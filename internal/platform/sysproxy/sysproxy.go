// Package sysproxy saves, changes and restores the system HTTP proxy, so a
// local proxy can see the traffic of the machine. On macOS it drives
// networksetup for every network service that is up, on Windows it writes
// the Internet Settings registry key and tells running programs to re-read
// it, and on Linux it drives GNOME's gsettings schema. Every other system
// returns ErrUnsupported.
//
// Linux: not yet verified on a real system.
package sysproxy

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// ErrPermission is what Current, Enable and Restore return when the system
// refuses the change for want of authorisation. The caller may offer the
// system's own prompt and try again.
var ErrPermission = errors.New("changing the system proxy is not permitted")

// ErrUnsupported is what every function returns on a system whose proxy
// settings this package cannot change.
var ErrUnsupported = errors.New("the system proxy is not supported on this system")

// Snapshot is the system proxy settings before the app changed them. It
// is plain data that marshals to JSON, so it can be kept in a state file
// and restored after a crash. Each system fills the group of fields it
// uses and leaves the others empty, so one type serves every build.
type Snapshot struct {
	// Services is the macOS group: one entry per network service that was
	// up when the snapshot was taken.
	Services []Service `json:"services,omitempty"`
	// Windows is the WinINET group of the current user.
	Windows *WindowsSettings `json:"windows,omitempty"`
	// GNOME is the gsettings group of a GNOME desktop.
	GNOME *GNOMESettings `json:"gnome,omitempty"`
}

// Service is the proxy settings of one macOS network service.
type Service struct {
	Name  string    `json:"name"`
	HTTP  Proxy     `json:"http"`
	HTTPS Proxy     `json:"https"`
	Auto  AutoProxy `json:"auto"`
}

// Proxy is one manual proxy of a macOS network service. Port is 0 when the
// service had none, which is how the system stores a disabled proxy that
// was never configured.
type Proxy struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
}

// AutoProxy is the proxy auto-configuration URL of a macOS network
// service. A URL of "(null)" in the tool's output means none, which is
// stored as an empty string.
type AutoProxy struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty"`
}

// WindowsSettings is the WinINET proxy configuration of the current user.
// Each value records whether it existed at all, because deleting a value
// and writing a zero are not the same thing for the system.
type WindowsSettings struct {
	ProxyEnable   DWORD       `json:"proxy_enable"`
	ProxyServer   StringValue `json:"proxy_server"`
	ProxyOverride StringValue `json:"proxy_override"`
	AutoConfigURL StringValue `json:"auto_config_url"`
}

// DWORD is a registry number as it was before the app wrote it.
type DWORD struct {
	Present bool   `json:"present"`
	Number  uint32 `json:"number,omitempty"`
}

// StringValue is a registry string as it was before the app wrote it.
type StringValue struct {
	Present bool   `json:"present"`
	Text    string `json:"text,omitempty"`
}

// GNOMESettings is the proxy configuration of a GNOME desktop, read from
// and written to org.gnome.system.proxy.
type GNOMESettings struct {
	Mode  string     `json:"mode"`
	HTTP  GNOMEProxy `json:"http"`
	HTTPS GNOMEProxy `json:"https"`
}

// GNOMEProxy is the host and port one scheme of a GNOME proxy points at.
type GNOMEProxy struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
}

// Current returns the proxy settings the system has now.
func Current() (Snapshot, error) { return current() }

// Enable points the HTTP and HTTPS proxy of the system (on macOS, of every
// network service that is up) at hostport and returns the settings it
// replaced. It is safe to call twice: the second snapshot is then the
// app's own change, which is why the caller keeps the first one.
func Enable(hostport string) (Snapshot, error) { return enable(hostport) }

// Restore puts back what a snapshot holds. Restoring an empty snapshot does
// nothing.
func Restore(s Snapshot) error { return restore(s) }

// Points reports whether any part of the snapshot's proxy is enabled and
// pointing at hostport: on macOS the web or secure-web proxy of any service,
// on Windows either the http or the https part of ProxyServer, on GNOME
// either scheme in manual mode. One part is enough, which is the same test
// RestorePointing uses: a service where the person turned off only one of
// the two proxies still points at us through the other one.
func Points(s Snapshot, hostport string) bool { return points(s, hostport) }

// RestorePointing restores, of the services (or settings) in the snapshot,
// only those that still point at hostport in at least one part, and leaves
// the rest as the person has them now. A service that qualifies gets all of
// its saved fields back: web, secure web and the auto-proxy URL. It goes
// through every one even when one fails (a service that is gone is skipped)
// and returns the errors joined. It is what crash recovery uses, because
// after a crash a person may have switched the proxy off by hand on one
// service while others still point at the dead address.
func RestorePointing(s Snapshot, hostport string) error { return restorePointing(s, hostport) }

// parseHostPort splits a proxy address and checks the port a listener could
// bind. The system accepts a host without a port in some places; this
// package does not, because the proxy always listens on one.
func parseHostPort(hostport string) (string, int, error) {
	host, portText, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", 0, fmt.Errorf("parse proxy address %q: %w", hostport, err)
	}
	if host == "" {
		return "", 0, fmt.Errorf("parse proxy address %q: no host", hostport)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > maxPort {
		return "", 0, fmt.Errorf("parse proxy address %q: port %q is not a port", hostport, portText)
	}
	return host, port, nil
}

// maxPort is the largest port a socket can listen on.
const maxPort = 65535

// sameProxy reports whether a stored endpoint is wantHost:wantPort. Hosts
// compare without case because the system keeps whatever a person typed.
func sameProxy(host string, port int, wantHost string, wantPort int) bool {
	return port == wantPort && strings.EqualFold(strings.TrimSpace(host), wantHost)
}

// proxyServerPoints reports whether a WinINET ProxyServer value sends HTTP
// or HTTPS to wantHost:wantPort. The value is either one host:port for
// every protocol or a "scheme=host:port;..." list with entries such as
// http, https and socks; either declared part is enough, because a person
// may fix one of them by hand after a crash.
func proxyServerPoints(server string, wantHost string, wantPort int) bool {
	server = strings.TrimSpace(server)
	if server == "" {
		return false
	}
	if !strings.Contains(server, "=") {
		host, port, ok := splitEndpoint(server)
		return ok && sameProxy(host, port, wantHost, wantPort)
	}
	var httpSet, httpsSet bool
	for _, entry := range strings.Split(server, ";") {
		scheme, target, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		host, port, ok := splitEndpoint(target)
		switch strings.ToLower(strings.TrimSpace(scheme)) {
		case "http":
			httpSet = ok && sameProxy(host, port, wantHost, wantPort)
		case "https":
			httpsSet = ok && sameProxy(host, port, wantHost, wantPort)
		}
	}
	return httpSet || httpsSet
}

// splitEndpoint reads a host:port pair out of a proxy value, refusing the
// empty and the portless forms WinINET also accepts.
func splitEndpoint(endpoint string) (string, int, bool) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(endpoint))
	if err != nil || host == "" {
		return "", 0, false
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > maxPort {
		return "", 0, false
	}
	return host, port, true
}

// windowsSettingsPoint reports whether the saved WinINET values still send
// HTTP or HTTPS to wantHost:wantPort.
func windowsSettingsPoint(s WindowsSettings, wantHost string, wantPort int) bool {
	if !s.ProxyEnable.Present || s.ProxyEnable.Number == 0 {
		return false
	}
	return proxyServerPoints(s.ProxyServer.Text, wantHost, wantPort)
}

// gnomeSettingsPoint reports whether the saved GNOME values are in manual
// mode and one of the two schemes still sends to wantHost:wantPort.
func gnomeSettingsPoint(s GNOMESettings, wantHost string, wantPort int) bool {
	if s.Mode != manualMode {
		return false
	}
	return sameProxy(s.HTTP.Host, s.HTTP.Port, wantHost, wantPort) ||
		sameProxy(s.HTTPS.Host, s.HTTPS.Port, wantHost, wantPort)
}

// refusalPattern matches what a system tool prints when it refuses a change
// for want of authorisation, in the wordings macOS and GNOME use.
var refusalPattern = regexp.MustCompile(`(?i)not permitted|permission denied|unauthori[sz]ed|authori[sz]ation (is )?required|must be root|requires? administrator|not (allowed|authori[sz]ed) to`)

// commandFailed wraps a command that exited non-zero, naming it and keeping
// its first output line, and reports a refusal as ErrPermission so the
// caller can offer the system's own prompt.
func commandFailed(name string, out string, err error) error {
	if refusalPattern.MatchString(out) {
		return fmt.Errorf("%s: %w", name, ErrPermission)
	}
	if line := firstLine(out); line != "" {
		return fmt.Errorf("%s: %w: %s", name, err, line)
	}
	return fmt.Errorf("%s: %w", name, err)
}

// rollback keeps the first failure visible and also reports a failed
// restore, because a proxy left pointing at a dead port cuts the person off
// the network and both faults need fixing.
func rollback(cause, restoreErr error) error {
	if restoreErr == nil {
		return cause
	}
	return errors.Join(cause, fmt.Errorf("restore: %w", restoreErr))
}

// firstLine keeps an error short when the tool prints a whole manual.
func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

// The GNOME schema and the two modes the package writes.
const (
	gsettingsProgram = "gsettings"
	gnomeSchema      = "org.gnome.system.proxy"
	manualMode       = "manual"
	noneMode         = "none"
)

// The gsettings verbs and keys of the GNOME proxy.
const (
	gsettingsGet = "get"
	gsettingsSet = "set"
	settingHost  = "host"
	settingPort  = "port"
	settingMode  = "mode"
)

// gnomeEnableCommands is the command sequence that points GNOME at host and
// port. It turns the proxy off while the endpoints change, so no request
// is sent to a half-written setting.
func gnomeEnableCommands(host string, port int) [][]string {
	commands := [][]string{{gsettingsProgram, gsettingsSet, gnomeSchema, settingMode, noneMode}}
	commands = append(commands, gnomeProxyCommands(".http", GNOMEProxy{Host: host, Port: port})...)
	commands = append(commands, gnomeProxyCommands(".https", GNOMEProxy{Host: host, Port: port})...)
	return append(commands, []string{gsettingsProgram, gsettingsSet, gnomeSchema, settingMode, manualMode})
}

// gnomeRestoreCommands is the command sequence that puts a GNOME snapshot
// back: proxy off, the old endpoints, then the mode it had.
func gnomeRestoreCommands(s GNOMESettings) [][]string {
	commands := [][]string{{gsettingsProgram, gsettingsSet, gnomeSchema, settingMode, noneMode}}
	commands = append(commands, gnomeProxyCommands(".http", s.HTTP)...)
	commands = append(commands, gnomeProxyCommands(".https", s.HTTPS)...)
	if s.Mode == "" {
		return commands
	}
	return append(commands, []string{gsettingsProgram, gsettingsSet, gnomeSchema, settingMode, s.Mode})
}

// gnomeProxyCommands writes the host and port of one scheme.
func gnomeProxyCommands(suffix string, p GNOMEProxy) [][]string {
	schema := gnomeSchema + suffix
	return [][]string{
		{gsettingsProgram, gsettingsSet, schema, settingHost, p.Host},
		{gsettingsProgram, gsettingsSet, schema, settingPort, strconv.Itoa(p.Port)},
	}
}

// parseGSettingsString reads a gsettings string value, dropping the single
// quotes the tool prints around it.
func parseGSettingsString(out string) string {
	return strings.Trim(strings.TrimSpace(out), "'")
}

// parseGSettingsInt reads a gsettings number, which may carry a type name
// in front of it.
func parseGSettingsInt(out string) (int, error) {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0, fmt.Errorf("parse gsettings number %q: empty", out)
	}
	text := fields[len(fields)-1]
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("parse gsettings number %q: %w", out, err)
	}
	return n, nil
}
