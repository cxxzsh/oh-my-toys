//go:build darwin

package sysproxy

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// runner runs one system command and returns everything it printed, also
// when it fails. Tests pass a fake so the command sequences are checked
// without touching the machine.
type runner func(name string, args ...string) (string, error)

// execRunner runs the command for real. The frozen API of this package
// takes no context, so there is none to stop it with.
func execRunner(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput() //nolint:noctx,gosec // the frozen API takes no context, and the program is a package constant
	return string(out), err
}

// networksetupPath is the absolute path of the tool, because an app started
// from the Finder does not inherit the shell's PATH.
const networksetupPath = "/usr/sbin/networksetup"

// The networksetup arguments that change a service, and the two words it
// uses for a proxy state.
const (
	listServices   = "-listallnetworkservices"
	getWebProxy    = "-getwebproxy"
	getSecureProxy = "-getsecurewebproxy"
	getAutoProxy   = "-getautoproxyurl"
	setWebProxy    = "-setwebproxy"
	setSecureProxy = "-setsecurewebproxy"
	setWebState    = "-setwebproxystate"
	setSecureState = "-setsecurewebproxystate"
	setAutoURL     = "-setautoproxyurl"
	setAutoState   = "-setautoproxystate"
	stateOn        = "on"
	stateOff       = "off"
)

// The keys networksetup prints when it reports a service's proxy.
const (
	fieldEnabled = "Enabled"
	fieldServer  = "Server"
	fieldPort    = "Port"
	fieldURL     = "URL"
)

// servicesLegend is the first line of -listallnetworkservices, which
// explains that a disabled service is marked with an asterisk.
const servicesLegend = "denotes that a network service is disabled"

// nullURL is what the tool prints for a service without an auto-proxy URL.
const nullURL = "(null)"

func current() (Snapshot, error) { return readServices(execRunner) }

func enable(hostport string) (Snapshot, error) { return enableServices(execRunner, hostport) }

func restore(s Snapshot) error { return restoreServices(execRunner, s) }

func restorePointing(s Snapshot, hostport string) error {
	return restorePointingServices(execRunner, s, hostport)
}

func points(s Snapshot, hostport string) bool { return pointsAt(s, hostport) }

func effective(hostport string) (bool, error) { return effectiveWith(execRunner, hostport) }

// scutilPath is the absolute path of the tool that reports the proxy the
// system hands to apps, which is the setting of the primary service.
const scutilPath = "/usr/sbin/scutil"

// The names scutil gives the manual proxy of each scheme, and the word it
// prints for an enabled one.
const (
	scutilHTTP  = "HTTP"
	scutilHTTPS = "HTTPS"
	scutilOn    = "1"
)

// effectiveWith asks scutil what apps would use and compares it with the
// proxy. A proxy auto-configuration file counts as not pointing at it: it
// may send a request anywhere, so the caller is told rather than promised.
func effectiveWith(r runner, hostport string) (bool, error) {
	out, err := r(scutilPath, "--proxy")
	if err != nil {
		return false, commandFailed(scutilPath, out, err)
	}
	host, port, err := parseHostPort(hostport)
	if err != nil {
		return false, err
	}
	settings := parseScutil(out)
	for _, scheme := range []string{scutilHTTP, scutilHTTPS} {
		if proxyPoints(dictionaryProxy(settings, scheme), host, port) {
			return true, nil
		}
	}
	return false, nil
}

// dictionaryProxy is one scheme's manual proxy in what scutil printed.
func dictionaryProxy(settings map[string]string, scheme string) Proxy {
	port, _ := strconv.Atoi(settings[scheme+"Port"])
	return Proxy{Enabled: settings[scheme+"Enable"] == scutilOn, Host: settings[scheme+"Proxy"], Port: port}
}

// parseScutil reads the flat key/value lines of scutil's dictionary. The
// nested lists it prints for exceptions and PAC addresses have values with
// braces or no value word, and they are skipped: only the proxy itself is
// asked for.
func parseScutil(out string) map[string]string {
	settings := make(map[string]string, 8)
	for _, line := range strings.Split(out, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), " : ")
		if !found || strings.ContainsAny(value, "{}<>") {
			continue
		}
		settings[key] = value
	}
	return settings
}

// readServices snapshots every network service that is up. A disabled
// service is left alone by Enable, so its settings need no saving.
func readServices(r runner) (Snapshot, error) {
	out, err := read(r, listServices)
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	for _, name := range parseServices(out) {
		svc, err := readService(r, name)
		if err != nil {
			return Snapshot{}, err
		}
		snap.Services = append(snap.Services, svc)
	}
	return snap, nil
}

// readService reads the three proxies of one service.
func readService(r runner, name string) (Service, error) {
	web, err := readProxySetting(r, getWebProxy, name)
	if err != nil {
		return Service{}, err
	}
	secure, err := readProxySetting(r, getSecureProxy, name)
	if err != nil {
		return Service{}, err
	}
	out, err := read(r, getAutoProxy, name)
	if err != nil {
		return Service{}, err
	}
	auto, err := parseAutoProxy(out)
	if err != nil {
		return Service{}, fmt.Errorf("read %s: %w", name, err)
	}
	return Service{Name: name, HTTP: web, HTTPS: secure, Auto: auto}, nil
}

func readProxySetting(r runner, query, name string) (Proxy, error) {
	out, err := read(r, query, name)
	if err != nil {
		return Proxy{}, err
	}
	p, err := parseProxy(out)
	if err != nil {
		return Proxy{}, fmt.Errorf("read %s: %w", name, err)
	}
	return p, nil
}

// enableServices snapshots the services, then points each one at host:port.
// On a failure it puts the snapshot back, because a proxy left pointing at
// a dead port cuts the person off the network.
func enableServices(r runner, hostport string) (Snapshot, error) {
	host, port, err := parseHostPort(hostport)
	if err != nil {
		return Snapshot{}, err
	}
	snap, err := readServices(r)
	if err != nil {
		return Snapshot{}, err
	}
	for _, svc := range snap.Services {
		if err := pointService(r, svc.Name, host, port); err != nil {
			return snap, rollback(err, restoreServices(r, snap))
		}
	}
	return snap, nil
}

// pointService sets both proxies of one service. The auto-proxy URL goes
// off first, because a PAC file overrides a manual proxy.
func pointService(r runner, name string, host string, port int) error {
	portText := strconv.Itoa(port)
	steps := [][]string{
		{setAutoState, name, stateOff},
		{setWebProxy, name, host, portText},
		{setSecureProxy, name, host, portText},
		{setWebState, name, stateOn},
		{setSecureState, name, stateOn},
	}
	for _, args := range steps {
		if err := set(r, args...); err != nil {
			return err
		}
	}
	return nil
}

// restoreServices puts every service of the snapshot back, skipping the ones
// that no longer exist and collecting the rest of the failures, so one
// unplugged adapter cannot make a Stop impossible.
func restoreServices(r runner, s Snapshot) error {
	return eachService(s.Services, func(svc Service) error {
		return restoreService(r, svc)
	})
}

// restorePointingServices restores only the services that still point at
// hostport: anything else is a change the person made after the crash and
// stays as it is.
func restorePointingServices(r runner, s Snapshot, hostport string) error {
	host, port, err := parseHostPort(hostport)
	if err != nil {
		return err
	}
	return eachService(s.Services, func(svc Service) error {
		current, err := readService(r, svc.Name)
		if err != nil {
			return err
		}
		if !servicePoints(current, host, port) {
			return nil
		}
		return restoreService(r, svc)
	})
}

// eachService runs fn for every service, skipping the ones that are gone
// and keeping the other failures.
func eachService(services []Service, fn func(Service) error) error {
	var errs []error
	for _, svc := range services {
		err := fn(svc)
		if err == nil || errors.Is(err, errServiceGone) {
			continue
		}
		errs = append(errs, fmt.Errorf("service %s: %w", svc.Name, err))
	}
	return errors.Join(errs...)
}

// restoreService puts one service back. It keeps going after a failure, so
// one bad command does not stop the rest of the service from being
// restored, and stops at once when the service is gone because every later
// command would fail the same way.
func restoreService(r runner, svc Service) error {
	var errs []error
	for _, args := range restoreSteps(svc) {
		err := set(r, args...)
		if err == nil {
			continue
		}
		if errors.Is(err, errServiceGone) {
			return err
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// restoreSteps is the command sequence that puts one service back.
func restoreSteps(svc Service) [][]string {
	steps := restoreProxySteps(setWebProxy, setWebState, svc.Name, svc.HTTP)
	steps = append(steps, restoreProxySteps(setSecureProxy, setSecureState, svc.Name, svc.HTTPS)...)
	return append(steps, restoreAutoSteps(svc.Name, svc.Auto)...)
}

// restoreProxySteps writes the old endpoint before the old state, because a
// set command also turns the proxy on. An empty endpoint is written too, as
// "" and 0: leaving it out would keep our own endpoint in the service's
// disabled slot, where it shows in System Settings and comes back when a
// person ticks the box. networksetup accepts that form and returns the
// service to Server empty, Port 0.
func restoreProxySteps(setCmd, stateCmd, name string, p Proxy) [][]string {
	host, port := p.Host, p.Port
	if host == "" || port <= 0 {
		host, port = "", 0
	}
	return [][]string{
		{setCmd, name, host, strconv.Itoa(port)},
		{stateCmd, name, stateWord(p.Enabled)},
	}
}

// restoreAutoSteps puts the PAC URL and its state back. An empty URL cannot
// be written, because networksetup rejects the empty string, and needs no
// writing: only the state can have moved while we ran, so it goes back to
// off and no PAC URL of ours is left behind.
func restoreAutoSteps(name string, a AutoProxy) [][]string {
	if a.URL == "" {
		return [][]string{{setAutoState, name, stateOff}}
	}
	return [][]string{
		{setAutoURL, name, a.URL},
		{setAutoState, name, stateWord(a.Enabled)},
	}
}

func stateWord(on bool) string {
	if on {
		return stateOn
	}
	return stateOff
}

// servicePoints reports whether one of the manual proxies of a service is
// on and pointing at wantHost:wantPort. Either one is enough: a person may
// untick one checkbox after a crash while the other still points at the dead
// proxy, and that service must still come back.
func servicePoints(svc Service, wantHost string, wantPort int) bool {
	return proxyPoints(svc.HTTP, wantHost, wantPort) || proxyPoints(svc.HTTPS, wantHost, wantPort)
}

// proxyPoints reports whether one proxy is on and points at wantHost.
func proxyPoints(p Proxy, wantHost string, wantPort int) bool {
	return p.Enabled && sameProxy(p.Host, p.Port, wantHost, wantPort)
}

// pointsAt reports whether any proxy of any service of the snapshot still
// points at hostport, which is the test RestorePointing uses.
func pointsAt(s Snapshot, hostport string) bool {
	host, port, err := parseHostPort(hostport)
	if err != nil {
		return false
	}
	for _, svc := range s.Services {
		if servicePoints(svc, host, port) {
			return true
		}
	}
	return false
}

// parseServices lists the services that are up, in the order networksetup
// prints them. The legend line and the disabled services marked with an
// asterisk are skipped.
func parseServices(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") || strings.Contains(line, servicesLegend) {
			continue
		}
		names = append(names, line)
	}
	return names
}

// parseProxy reads the output of -getwebproxy and -getsecurewebproxy.
func parseProxy(out string) (Proxy, error) {
	var (
		p           Proxy
		haveEnabled bool
	)
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case fieldEnabled:
			on, err := yesNo(value)
			if err != nil {
				return Proxy{}, err
			}
			p.Enabled, haveEnabled = on, true
		case fieldServer:
			p.Host = value
		case fieldPort:
			port, err := strconv.Atoi(value)
			if err != nil {
				return Proxy{}, fmt.Errorf("parse port %q: %w", value, err)
			}
			p.Port = port
		}
	}
	if !haveEnabled {
		return Proxy{}, fmt.Errorf("no %s field in %q", fieldEnabled, firstLine(out))
	}
	return p, nil
}

// parseAutoProxy reads the output of -getautoproxyurl.
func parseAutoProxy(out string) (AutoProxy, error) {
	var (
		auto        AutoProxy
		haveEnabled bool
	)
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case fieldURL:
			if value != nullURL {
				auto.URL = value
			}
		case fieldEnabled:
			on, err := yesNo(value)
			if err != nil {
				return AutoProxy{}, err
			}
			auto.Enabled, haveEnabled = on, true
		}
	}
	if !haveEnabled {
		return AutoProxy{}, fmt.Errorf("no %s field in %q", fieldEnabled, firstLine(out))
	}
	return auto, nil
}

// yesNo reads the Enabled value the tool prints.
func yesNo(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	}
	return false, fmt.Errorf("parse enabled %q: want yes or no", value)
}

// errServiceGone marks a network service that is no longer in the network
// database, such as a USB adapter unplugged since the snapshot.
var errServiceGone = errors.New("network service is gone")

// gonePattern matches the ways networksetup says it cannot find a service.
var gonePattern = regexp.MustCompile(`(?i)unable to find item|not a recognized network service`)

// serviceFailure names a failed networksetup call, and marks a missing
// service so the caller can skip it instead of failing the whole restore.
func serviceFailure(command, out string, err error) error {
	if gonePattern.MatchString(out) {
		return fmt.Errorf("%s: %w", command, errServiceGone)
	}
	return commandFailed(command, out, err)
}

// read runs a networksetup query and returns what it printed.
func read(r runner, args ...string) (string, error) {
	out, err := r(networksetupPath, args...)
	if err != nil {
		return "", serviceFailure("networksetup "+args[0], out, err)
	}
	return out, nil
}

// set runs a networksetup change.
func set(r runner, args ...string) error {
	out, err := r(networksetupPath, args...)
	if err != nil {
		return serviceFailure("networksetup "+args[0], out, err)
	}
	return nil
}
