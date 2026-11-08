package traffic

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/platform/sysproxy"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// priorSettings is a system proxy a person had before the capture: the
// settings a snapshot holds on macOS.
func priorSettings() sysproxy.Snapshot {
	return sysproxy.Snapshot{Services: []sysproxy.Service{{
		Name:  "Wi-Fi",
		HTTP:  sysproxy.Proxy{Enabled: true, Host: "10.0.0.9", Port: 3128},
		HTTPS: sysproxy.Proxy{Enabled: true, Host: "10.0.0.9", Port: 3128},
	}}}
}

// proxyConfig is the settings of a capture proxy on a free port.
func proxyConfig(t *testing.T, systemProxy bool) tool.CaptureConfig {
	t.Helper()
	return tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: freePort(t), SystemProxy: systemProxy}
}

func TestStartSavesTheStateBeforeItChangesTheSystemProxy(t *testing.T) {
	tl, fake := testTool(t)
	fake.snap = withService(priorSettings(), "Thunderbolt", 9090)
	fake.before = withService(priorSettings(), "USB LAN", 8080)
	cfg := proxyConfig(t, true)
	want := fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort)

	var duringEnable proxyState
	var stateThere bool
	fake.onEnable = func() {
		state, err := tl.loadState()
		duringEnable, stateThere = state, err == nil
	}
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })

	if !stateThere {
		t.Fatal("the system proxy was changed before the state file was written")
	}
	if names := serviceNames(duringEnable.Snapshot); len(names) != 2 {
		t.Errorf("the state file held %v while Enable ran, want the settings the system had", names)
	}
	calls := fake.callsSoFar()
	if len(calls) < 2 || calls[0] != "current" || calls[1] != "enable "+want {
		t.Errorf("calls %v, want current then enable %s first", calls, want)
	}
	state, err := tl.loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if state.Addr != want {
		t.Errorf("state address %q, want %q", state.Addr, want)
	}
	wantNames := []string{"Wi-Fi", "USB LAN", "Thunderbolt"}
	if names := serviceNames(state.Snapshot); !slices.Equal(names, wantNames) {
		t.Errorf("state services %v, want %v: what Enable replaced and what the system had", names, wantNames)
	}
	if st := tl.Status(); !st.On {
		t.Errorf("status %+v, want on", st)
	}
}

// withService is a snapshot with one more service that points at a proxy.
func withService(s sysproxy.Snapshot, name string, port int) sysproxy.Snapshot {
	proxy := sysproxy.Proxy{Enabled: true, Host: "10.0.0.9", Port: port}
	s.Services = append([]sysproxy.Service(nil), s.Services...)
	s.Services = append(s.Services, sysproxy.Service{Name: name, HTTP: proxy, HTTPS: proxy})
	return s
}

// serviceNames is the names of the services a snapshot holds, in order.
func serviceNames(s sysproxy.Snapshot) []string {
	names := make([]string, len(s.Services))
	for i, service := range s.Services {
		names[i] = service.Name
	}
	return names
}

func TestStopRestoresThenForgetsTheState(t *testing.T) {
	tl, fake := testTool(t)
	fake.snap = priorSettings()
	if err := tl.SetConfig(proxyConfig(t, true)); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := tl.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	calls := fake.callsSoFar()
	if calls[len(calls)-1] != "restore" {
		t.Errorf("calls %v, want the last one to restore the system proxy", calls)
	}
	if len(fake.restored) != 1 || fake.restored[0].Services[0].HTTP.Port != 3128 {
		t.Errorf("restored %+v, want the snapshot of the settings the system had", fake.restored)
	}
	if _, err := os.Stat(tl.path(stateFile)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want the state file gone", stateFile, err)
	}
	if st := tl.Status(); st.On {
		t.Errorf("status %+v, want off", st)
	}
	if err := tl.Stop(); err != nil {
		t.Errorf("Stop of a stopped capture: %v", err)
	}
}

func TestStopWithoutACaptureDoesNothing(t *testing.T) {
	tl, fake := testTool(t)
	if err := tl.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if calls := fake.callsSoFar(); len(calls) != 0 {
		t.Errorf("calls %v, want none", calls)
	}
}

func TestStartWhenTheSystemProxyCannotBeChanged(t *testing.T) {
	tl, fake := testTool(t)
	fake.enableErr = sysproxy.ErrPermission
	cfg := proxyConfig(t, true)
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	err := tl.Start(context.Background())
	if !errors.Is(err, tool.ErrPermission) {
		t.Errorf("Start: %v, want tool.ErrPermission", err)
	}
	if !errors.Is(err, sysproxy.ErrPermission) {
		t.Errorf("Start: %v, want the system's refusal kept in the chain", err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort)
	state, statErr := tl.loadState()
	if statErr != nil {
		t.Fatalf("state file: %v, want it kept: recovery only restores what still points at the capture", statErr)
	}
	if state.Addr != addr {
		t.Errorf("state address %q, want %q", state.Addr, addr)
	}
	if st := tl.Status(); st.On {
		t.Errorf("status %+v, want off", st)
	}
	if dialProbeTo(t, addr) {
		t.Errorf("the proxy still listens on %s", addr)
	}
}

// dialProbe is how long a test waits for a connection it expects to fail.
const dialProbe = 200 * time.Millisecond

// dialProbeTo reports whether something listens on addr.
func dialProbeTo(t *testing.T, addr string) bool {
	t.Helper()
	dialer := &net.Dialer{Timeout: dialProbe}
	conn, err := dialer.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func TestStopKeepsTheStateWhenTheRestoreFails(t *testing.T) {
	tl, fake := testTool(t)
	fake.snap = priorSettings()
	if err := tl.SetConfig(proxyConfig(t, true)); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.restoreErr = sysproxy.ErrPermission
	err := tl.Stop()
	if !errors.Is(err, tool.ErrPermission) {
		t.Errorf("Stop: %v, want tool.ErrPermission", err)
	}
	if _, statErr := os.Stat(tl.path(stateFile)); statErr != nil {
		t.Errorf("stat %s: %v, want the state file kept for the next try", stateFile, statErr)
	}
	if st := tl.Status(); !st.On {
		t.Errorf("status %+v, want the capture still on: stopping it would cut the person off", st)
	}

	fake.restoreErr = nil
	if err := tl.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if _, statErr := os.Stat(tl.path(stateFile)); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want the state file gone", stateFile, statErr)
	}
}

func TestPortInUsePassesThrough(t *testing.T) {
	tl, fake := testTool(t)
	held, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = held.Close() }()
	cfg := tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: held.Addr().(*net.TCPAddr).Port, SystemProxy: true}
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	err = tl.Start(context.Background())
	if !errors.Is(err, tool.ErrPortInUse) {
		t.Fatalf("Start: %v, want tool.ErrPortInUse", err)
	}
	if fake.seen("enable " + held.Addr().String()) {
		t.Error("the system proxy was changed although the port was taken")
	}
	if _, statErr := os.Stat(tl.path(stateFile)); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want no state file", stateFile, statErr)
	}
}

func TestStartAgainReplacesTheCapture(t *testing.T) {
	tl, _ := testTool(t)
	first := proxyConfig(t, false)
	if err := tl.SetConfig(first); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	second := proxyConfig(t, false)
	if err := tl.SetConfig(second); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })
	run, ok := tl.runningProxy()
	if !ok {
		t.Fatal("no capture runs")
	}
	if run.addr != fmt.Sprintf("127.0.0.1:%d", second.ProxyPort) {
		t.Errorf("capture on %s, want the new port %d", run.addr, second.ProxyPort)
	}
	if dialProbeTo(t, fmt.Sprintf("127.0.0.1:%d", first.ProxyPort)) {
		t.Errorf("the old port %d still listens", first.ProxyPort)
	}
}

// runningProxy is the capture in progress, for a test that has to look at
// it.
func (t *Tool) runningProxy() (*proxyRun, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.proxy, t.proxy != nil
}

func TestRecoverRestoresWhatStillPointsAtTheCapture(t *testing.T) {
	tl, fake := testTool(t)
	if err := tl.saveState("127.0.0.1:9099", priorSettings()); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	if err := tl.Recover(); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if fake.pointingAddr != "127.0.0.1:9099" {
		t.Errorf("restore pointing for %q, want the address the state file names", fake.pointingAddr)
	}
	if len(fake.pointing) != 1 || fake.pointing[0].Services[0].HTTP.Port != 3128 {
		t.Errorf("restore pointing given %+v, want the saved settings", fake.pointing)
	}
	if fake.seen("points") {
		t.Error("recovery asks Points first; it must let RestorePointing decide per service")
	}
	if _, err := os.Stat(tl.path(stateFile)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want the state file gone", stateFile, err)
	}
}

func TestRecoverWithoutStateTouchesNothing(t *testing.T) {
	tl, fake := testTool(t)
	if err := tl.Recover(); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if calls := fake.callsSoFar(); len(calls) != 0 {
		t.Errorf("calls %v, want none: nothing was changed before", calls)
	}
}

func TestRecoverKeepsTheStateWhenTheRestoreFails(t *testing.T) {
	tl, fake := testTool(t)
	fake.pointingErr = sysproxy.ErrUnsupported
	if err := tl.saveState("127.0.0.1:9099", priorSettings()); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	err := tl.Recover()
	if !errors.Is(err, tool.ErrUnsupportedHere) {
		t.Errorf("Recover: %v, want tool.ErrUnsupportedHere", err)
	}
	if _, statErr := os.Stat(tl.path(stateFile)); statErr != nil {
		t.Errorf("stat %s: %v, want the state file kept", stateFile, statErr)
	}
}

func TestReplayNeedsARunningCapture(t *testing.T) {
	tl, _ := testTool(t)
	if _, err := tl.Replay(context.Background(), 1); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Replay: %v, want ErrNotRunning", err)
	}
}

// TestTheCaptureCountsAsOnWhileTheSystemProxyChanges: Status is what
// StopAll reads before it stops a tool, so a capture that is still changing
// the system proxy has to report itself on already. Otherwise quitting the
// app leaves the system proxy pointing at a dead port.
func TestTheCaptureCountsAsOnWhileTheSystemProxyChanges(t *testing.T) {
	tl, fake := testTool(t)
	cfg := proxyConfig(t, true)
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	fake.onEnable = func() {
		close(entered)
		<-release
	}
	started := make(chan error, 1)
	go func() { started <- tl.Start(context.Background()) }()
	select {
	case <-entered:
	case <-time.After(captureWait):
		t.Fatal("the system proxy was never changed")
	}
	if st := tl.Status(); !st.On {
		t.Error("Status says off while the system proxy is being changed")
	}
	// What StopAll does: Stop waits for the action in flight, then puts the
	// system proxy back.
	stopped := make(chan error, 1)
	go func() { stopped <- tl.Stop() }()
	close(release)
	if err := <-started; err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := <-stopped; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !fake.seen("restore") {
		t.Error("the system proxy was not put back after the start")
	}
	if _, err := os.Stat(tl.path(stateFile)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want the state file gone", stateFile, err)
	}
}

// TestRecoverMovesADamagedStateFileAside: a file a power cut left empty
// must not make every later action fail.
func TestRecoverMovesADamagedStateFileAside(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "half written", body: `{"addr":"127.0.0.1:9099","snapshot":{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { checkDamagedStateFile(t, tt.body) })
	}
}

// checkDamagedStateFile leaves a state file the tool cannot read behind,
// and checks that recovery moves it aside and a capture still starts.
func checkDamagedStateFile(t *testing.T, body string) {
	t.Helper()
	tl, _ := testTool(t)
	if err := os.MkdirAll(tl.dir, folderMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := tl.path(stateFile)
	if err := os.WriteFile(path, []byte(body), fileMode); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := tl.Recover(); err != nil {
		t.Fatalf("Recover: %v, want the damaged file moved aside, not an error", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want it moved aside", stateFile, err)
	}
	if _, err := os.Stat(path + brokenSuffix); err != nil {
		t.Errorf("stat %s%s: %v, want the damaged file kept for a look", stateFile, brokenSuffix, err)
	}
	if err := tl.SetConfig(proxyConfig(t, false)); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := tl.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestStartRefusesAPacOnlySystemProxy: a snapshot whose only proxy is a
// PAC file cannot be followed, so the capture must ask for an explicit
// upstream rather than going direct behind the person's back.
func TestStartRefusesAPacOnlySystemProxy(t *testing.T) {
	tl, fake := testTool(t)
	fake.snap = sysproxy.Snapshot{Services: []sysproxy.Service{{
		Name: "Wi-Fi",
		Auto: sysproxy.AutoProxy{Enabled: true, URL: "http://pac.example/proxy.pac"},
	}}}
	cfg := proxyConfig(t, true)
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	err := tl.Start(context.Background())
	if !errors.Is(err, ErrBadConfig) {
		t.Fatalf("Start: %v, want ErrBadConfig: the capture cannot follow a PAC", err)
	}
	if fake.seenPrefix("enable ") {
		t.Error("the system proxy was changed although the capture refused to start")
	}
	if st := tl.Status(); st.On {
		t.Errorf("status %+v, want off", st)
	}
	if _, statErr := os.Stat(tl.path(stateFile)); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want no state file", stateFile, statErr)
	}

	cfg.Upstream = "http://10.0.0.2:3128"
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start with an explicit upstream: %v", err)
	}
	if err := tl.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
