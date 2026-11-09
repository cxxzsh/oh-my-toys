package traffic

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/platform/procnet"
	"github.com/cxxzsh/oh-my-toys/internal/platform/terminal"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
)

// unknownPort is the client port the test owner fake cannot name.
const unknownPort = 6666

// systemPEM stands in for the system's certificates in a test bundle.
const systemPEM = "-----BEGIN CERTIFICATE-----\nSYSTEM\n-----END CERTIFICATE-----\n"

// varValue returns the value of a variable in vars, and whether it is set.
func varValue(vars []tool.Header, name string) (string, bool) {
	for _, v := range vars {
		if v.Name == name {
			return v.Value, true
		}
	}
	return "", false
}

// proxyAddrOf is the address the running proxy listens on.
func proxyAddrOf(tl *Tool) string {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.proxy.addr
}

// toHeaders converts terminal variables to the tool's headers, for lookup.
func toHeaders(vars []terminal.Var) []tool.Header {
	out := make([]tool.Header, 0, len(vars))
	for _, v := range vars {
		out = append(out, tool.Header{Name: v.Name, Value: v.Value})
	}
	return out
}

// startCaptureWith starts a proxy capture with cfg on a fresh tool whose
// machine has the system CA bundle files bundles.
func startCaptureWith(t *testing.T, bundles []string, cfg tool.CaptureConfig) (*Tool, *fakeSystem) {
	t.Helper()
	fake := &fakeSystem{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), caBundles: bundles}
	tl := newTool(t.TempDir(), fake.system())
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })
	return tl, fake
}

func TestEnvironmentNeedsARunningProxy(t *testing.T) {
	tl, _ := testTool(t)
	if _, err := tl.Environment(); !errors.Is(err, tool.ErrNotRunning) {
		t.Errorf("Environment = %v, want tool.ErrNotRunning", err)
	}
	if err := tl.OpenTerminal(context.Background()); !errors.Is(err, tool.ErrNotRunning) {
		t.Errorf("OpenTerminal = %v, want tool.ErrNotRunning", err)
	}
}

func TestEnvironmentInPacketModeIsUnsupported(t *testing.T) {
	tl, _ := testTool(t)
	tl.mu.Lock()
	tl.packet = &packetRun{}
	tl.mu.Unlock()
	if _, err := tl.Environment(); !errors.Is(err, tool.ErrUnsupportedHere) {
		t.Errorf("Environment = %v, want tool.ErrUnsupportedHere", err)
	}
	if err := tl.OpenTerminal(context.Background()); !errors.Is(err, tool.ErrUnsupportedHere) {
		t.Errorf("OpenTerminal = %v, want tool.ErrUnsupportedHere", err)
	}
}

func TestEnvironmentVariableOrderAndValues(t *testing.T) {
	cfg := proxyConfig(t, false)
	cfg.Bypass = []string{"*.corp.example"}
	tl, _ := startCaptureWith(t, nil, cfg)

	env, err := tl.Environment()
	if err != nil {
		t.Fatalf("Environment: %v", err)
	}
	want := []string{
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY",
		"http_proxy", "https_proxy", "all_proxy",
		"NO_PROXY", "no_proxy",
		"NODE_EXTRA_CA_CERTS",
	}
	if len(env.Vars) != len(want) {
		t.Fatalf("got %d variables, want %d: %+v", len(env.Vars), len(want), env.Vars)
	}
	for i, name := range want {
		if env.Vars[i].Name != name {
			t.Errorf("variable %d = %s, want %s", i, env.Vars[i].Name, name)
		}
	}
	proxyURL := "http://" + proxyAddrOf(tl)
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		if got, _ := varValue(env.Vars, name); got != proxyURL {
			t.Errorf("%s = %q, want %q", name, got, proxyURL)
		}
	}
	// The bypass list names hosts never decrypted; it must not reach NO_PROXY.
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		if got, _ := varValue(env.Vars, name); got != noProxy {
			t.Errorf("%s = %q, want %q", name, got, noProxy)
		}
	}
}

func TestEnvironmentAlwaysSetsTheRootPath(t *testing.T) {
	tl, fake := startCaptureWith(t, nil, proxyConfig(t, false))
	env, err := tl.Environment()
	if err != nil {
		t.Fatalf("Environment: %v", err)
	}
	a, err := ca.Load(tl.caDir)
	if err != nil {
		t.Fatalf("the root was not made: %v", err)
	}
	if got, _ := varValue(env.Vars, "NODE_EXTRA_CA_CERTS"); got != a.CertPath() {
		t.Errorf("NODE_EXTRA_CA_CERTS = %q, want %q", got, a.CertPath())
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.installed) != 0 {
		t.Errorf("Environment trusted the root: %v", fake.installed)
	}
}

func TestEnvironmentOmitsReplacingVarsWithoutASystemBundle(t *testing.T) {
	tl, _ := startCaptureWith(t, nil, proxyConfig(t, false))
	env, err := tl.Environment()
	if err != nil {
		t.Fatalf("Environment: %v", err)
	}
	for _, name := range replacingCAVars {
		if got, ok := varValue(env.Vars, name); ok {
			t.Errorf("%s = %q without a system bundle; it must be absent", name, got)
		}
	}
}

func TestEnvironmentCombinesTheSystemBundleAndOurRoot(t *testing.T) {
	system := filepath.Join(t.TempDir(), "system.pem")
	if err := os.WriteFile(system, []byte(systemPEM), 0o644); err != nil {
		t.Fatal(err)
	}
	tl, _ := startCaptureWith(t, []string{system}, proxyConfig(t, false))
	env, err := tl.Environment()
	if err != nil {
		t.Fatalf("Environment: %v", err)
	}
	bundle := filepath.Join(tl.caDir, caBundleFile)
	for _, name := range replacingCAVars {
		if got, _ := varValue(env.Vars, name); got != bundle {
			t.Errorf("%s = %q, want %q", name, got, bundle)
		}
	}
	data, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatalf("read the combined bundle: %v", err)
	}
	a, err := ca.Load(tl.caDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "SYSTEM") {
		t.Errorf("combined bundle lacks the system certificates:\n%s", data)
	}
	if !strings.Contains(string(data), string(a.CertPEM())) {
		t.Errorf("combined bundle lacks our root")
	}
	if runtime.GOOS != windowsOS {
		info, err := os.Stat(bundle)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != fileMode {
			t.Errorf("bundle mode = %o, want %o", perm, fileMode)
		}
	}
}

func TestCABundleIsRewrittenWhenTheSystemBundleChanges(t *testing.T) {
	system := filepath.Join(t.TempDir(), "system.pem")
	if err := os.WriteFile(system, []byte(systemPEM), 0o644); err != nil {
		t.Fatal(err)
	}
	tl, _ := startCaptureWith(t, []string{system}, proxyConfig(t, false))
	if _, err := tl.Environment(); err != nil {
		t.Fatalf("Environment: %v", err)
	}
	if err := os.WriteFile(system, []byte("-----BEGIN CERTIFICATE-----\nNEWER\n-----END CERTIFICATE-----\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Environment(); err != nil {
		t.Fatalf("Environment after the change: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tl.caDir, caBundleFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "NEWER") || strings.Contains(string(data), "SYSTEM") {
		t.Errorf("bundle was not rewritten for the new system bundle:\n%s", data)
	}
}

func TestSystemBundleReadErrorIsReported(t *testing.T) {
	// A directory is not a bundle: reading it fails, and that must not be
	// mistaken for a machine with no bundle.
	tl, _ := startCaptureWith(t, []string{t.TempDir()}, proxyConfig(t, false))
	if _, err := tl.Environment(); err == nil {
		t.Error("Environment succeeded although the system bundle could not be read")
	}
}

func TestScriptQuotesValues(t *testing.T) {
	if got := posixQuote("it's"); got != `'it'\''s'` {
		t.Errorf("posixQuote = %s", got)
	}
	if got := powerShellQuote("it's"); got != `'it''s'` {
		t.Errorf("powerShellQuote = %s", got)
	}
	script, shell := renderScript([]tool.Header{{Name: "X", Value: "a'b"}})
	if runtime.GOOS == windowsOS {
		if script != "$env:X = 'a''b'" || shell != shellPowerShell {
			t.Errorf("script = %q (%s)", script, shell)
		}
		return
	}
	if script != `export X='a'\''b'` || shell != shellSh {
		t.Errorf("script = %q (%s)", script, shell)
	}
}

func TestOpenTerminalPassesTheProxyVariables(t *testing.T) {
	tl, fake := startCaptureWith(t, nil, proxyConfig(t, false))

	if err := tl.OpenTerminal(context.Background()); err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}
	fake.mu.Lock()
	vars := fake.terminalVars
	fake.mu.Unlock()
	if got, _ := varValue(toHeaders(vars), "HTTP_PROXY"); got != "http://"+proxyAddrOf(tl) {
		t.Errorf("terminal HTTP_PROXY = %q", got)
	}
}

func TestScopeSurvivesARestart(t *testing.T) {
	tl, fake := testTool(t)
	cfg := proxyConfig(t, false)
	cfg.Scope = []string{"Chrome", "Safari"}
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	again := newTool(filepath.Dir(tl.dir), fake.system())
	if got := again.Config().Scope; len(got) != 2 || got[0] != "Chrome" {
		t.Errorf("Scope after a restart = %q", got)
	}
}

func TestScopeRejectsABlankEntry(t *testing.T) {
	tl, _ := testTool(t)
	cfg := proxyConfig(t, false)
	cfg.Scope = []string{"Chrome", "  "}
	if err := tl.SetConfig(cfg); !errors.Is(err, ErrBadConfig) {
		t.Errorf("SetConfig = %v, want ErrBadConfig", err)
	}
}

func TestProgramHookNamesTheOwnerOfAConnection(t *testing.T) {
	tl, fake := testTool(t)
	fake.owner = func(local, _ netip.AddrPort) (procnet.Process, error) {
		if local.Port() == unknownPort {
			return procnet.Process{}, procnet.ErrNotFound
		}
		return procnet.Process{PID: 7, Name: "curl", Path: "/usr/bin/curl"}, nil
	}
	hook := tl.programHook("127.0.0.1:9099")
	if hook == nil {
		t.Fatal("programHook returned nil for a valid address")
	}
	got := hook(netip.MustParseAddrPort("127.0.0.1:5555"))
	if got != (tool.Program{PID: 7, Name: "curl", Path: "/usr/bin/curl"}) {
		t.Errorf("program = %+v", got)
	}
	miss := hook(netip.MustParseAddrPort("127.0.0.1:6666"))
	if miss != (tool.Program{}) {
		t.Errorf("unknown connection named %+v", miss)
	}
}

func TestProgramHookIsNilForABadAddress(t *testing.T) {
	tl, _ := testTool(t)
	if hook := tl.programHook("not an address"); hook != nil {
		t.Error("programHook returned a hook for a bad address")
	}
}
