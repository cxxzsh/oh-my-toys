package traffic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/platform/terminal"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
)

// The shells Environment writes for. PowerShell is the shell of Windows.
const (
	shellSh         = "sh"
	shellPowerShell = "powershell"
	windowsOS       = "windows"
)

// noProxy are the hosts a program never sends through the proxy. They are
// fixed: CaptureConfig.Bypass names the hosts never decrypted, which is a
// different question, and a program told to skip them would skip the capture.
const noProxy = "localhost,127.0.0.1,::1"

// caBundleFile is the combined bundle, in the tool's CA folder.
const caBundleFile = "bundle.pem"

// systemCABundles are where the system keeps its CA bundle, in the order to
// look for them. Windows has none, so it gets no replacing variables.
var systemCABundles = []string{
	"/etc/ssl/cert.pem",
	"/etc/ssl/certs/ca-certificates.crt",
	"/etc/pki/tls/certs/ca-bundle.crt",
	"/etc/ssl/ca-bundle.pem",
}

// replacingCAVars are the variables that replace a program's CA bundle
// instead of adding to it, so they must name a bundle that still holds the
// system's certificates.
var replacingCAVars = []string{"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE", "GIT_SSL_CAINFO"}

// Environment is the set of variables that sends a program's traffic through
// the running proxy, and the script that sets them in a shell.
func (t *Tool) Environment() (tool.Environment, error) {
	run, err := t.liveProxy()
	if err != nil {
		return tool.Environment{}, err
	}
	vars, err := t.proxyVars(run)
	if err != nil {
		return tool.Environment{}, err
	}
	script, shell := renderScript(vars)
	return tool.Environment{Vars: vars, Script: script, Shell: shell}, nil
}

// OpenTerminal opens a terminal window with the proxy environment set, so a
// program started in it goes through the capture.
func (t *Tool) OpenTerminal(ctx context.Context) error {
	env, err := t.Environment()
	if err != nil {
		return err
	}
	vars := make([]terminal.Var, 0, len(env.Vars))
	for _, v := range env.Vars {
		vars = append(vars, terminal.Var{Name: v.Name, Value: v.Value})
	}
	return t.sys.openTerminal(ctx, vars)
}

// liveProxy returns the running proxy capture. Packet mode has no proxy to
// point a program at, and a stopped proxy has no address to point it at.
func (t *Tool) liveProxy() (*proxyRun, error) {
	t.mu.Lock()
	run, packets := t.proxy, t.packet
	t.mu.Unlock()
	switch {
	case packets != nil:
		return nil, tool.ErrUnsupportedHere
	case run == nil:
		return nil, tool.ErrNotRunning
	}
	return run, nil
}

// proxyVars are the variables of a program that goes through run. The root
// certificate is made here when it is missing, so that a program started
// before the person installs it has a file to point at; it is not trusted
// here. It holds opMu, as InstallCert does, because making a root is not safe
// beside a capture that makes one too.
func (t *Tool) proxyVars(run *proxyRun) ([]tool.Header, error) {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	a, err := t.root(rootMake)
	if err != nil {
		return nil, err
	}
	certVars, err := t.certVars(a)
	if err != nil {
		return nil, err
	}
	return append(proxyEnvVars("http://"+run.addr), certVars...), nil
}

// proxyEnvVars are the proxy variables for proxyURL, in the order the
// programs' documentation lists them. The lower-case twins are read by tools
// that ignore the upper-case names.
func proxyEnvVars(proxyURL string) []tool.Header {
	return []tool.Header{
		{Name: "HTTP_PROXY", Value: proxyURL},
		{Name: "HTTPS_PROXY", Value: proxyURL},
		{Name: "ALL_PROXY", Value: proxyURL},
		{Name: "http_proxy", Value: proxyURL},
		{Name: "https_proxy", Value: proxyURL},
		{Name: "all_proxy", Value: proxyURL},
		{Name: "NO_PROXY", Value: noProxy},
		{Name: "no_proxy", Value: noProxy},
	}
}

// certVars are the certificate variables for root a. NODE_EXTRA_CA_CERTS only
// adds to a program's trust, so it is always set, even when decryption is off
// and the person may switch it on later. The replacing variables are set only
// when a system bundle exists: pointing them at our root alone would break
// every other HTTPS site for the program.
func (t *Tool) certVars(a *ca.Authority) ([]tool.Header, error) {
	vars := []tool.Header{{Name: "NODE_EXTRA_CA_CERTS", Value: a.CertPath()}}
	system, ok, err := t.systemBundle()
	if err != nil {
		return nil, err
	}
	if !ok {
		return vars, nil
	}
	bundle, err := t.writeCABundle(system, a.CertPEM())
	if err != nil {
		return nil, err
	}
	for _, name := range replacingCAVars {
		vars = append(vars, tool.Header{Name: name, Value: bundle})
	}
	return vars, nil
}

// systemBundle returns the system's CA bundle, and false when the machine has
// none. A file that is missing is not an error: the next location is tried.
func (t *Tool) systemBundle() ([]byte, bool, error) {
	for _, path := range t.sys.caBundles {
		b, err := os.ReadFile(path) //nolint:gosec // the system's own CA bundle, at a fixed location.
		switch {
		case err == nil:
			return b, true, nil
		case errors.Is(err, fs.ErrNotExist):
		default:
			return nil, false, fmt.Errorf("read the system certificates: %w", err)
		}
	}
	return nil, false, nil
}

// writeCABundle writes the system bundle with our root appended to the CA
// folder, and returns its path. It is rewritten only when its bytes change,
// so a program's variables do not move between two Environment calls.
func (t *Tool) writeCABundle(system, root []byte) (string, error) {
	path := filepath.Join(t.caDir, caBundleFile)
	combined := combineBundles(system, root)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, combined) { //nolint:gosec // a file of the tool's own CA folder.
		return path, nil
	}
	if err := writeBytes(path, combined); err != nil {
		return "", fmt.Errorf("write the combined certificates: %w", err)
	}
	return path, nil
}

// combineBundles joins the system bundle and our root PEM. A bundle that does
// not end in a newline gets one first, so the root does not run into the last
// certificate.
func combineBundles(system, root []byte) []byte {
	out := make([]byte, 0, len(system)+len(root)+1)
	out = append(out, system...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(out, root...)
}

// renderScript writes vars as the shell of this system reads them, and names
// that shell.
func renderScript(vars []tool.Header) (string, string) {
	lines := make([]string, 0, len(vars))
	if runtime.GOOS == windowsOS {
		for _, v := range vars {
			lines = append(lines, "$env:"+v.Name+" = "+powerShellQuote(v.Value))
		}
		return strings.Join(lines, "\n"), shellPowerShell
	}
	for _, v := range vars {
		lines = append(lines, "export "+v.Name+"="+posixQuote(v.Value))
	}
	return strings.Join(lines, "\n"), shellSh
}

// posixQuote quotes s for a POSIX shell. Inside single quotes nothing is
// expanded, so the only character needing care is the quote itself.
func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// powerShellQuote quotes s for PowerShell, where a single quote inside
// single quotes is written twice.
func powerShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
