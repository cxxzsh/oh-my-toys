package core

import (
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestSwitchIPC(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "app.sock")
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	sw := &fakeSwitch{}
	reg, err := NewRegistry(sw)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ServeSwitch(ctx, reg, ln) }()

	st, err := CallSwitch(context.Background(), "unix", sock, "fake", tool.ActionStart, nil)
	if err != nil || !st.On || sw.started != 1 {
		t.Fatalf("start %+v %v started=%d", st, err, sw.started)
	}
	st, err = CallSwitch(context.Background(), "unix", sock, "fake", tool.ActionStatus, nil)
	if err != nil || !st.On {
		t.Fatalf("status %+v %v", st, err)
	}
	st, err = CallSwitch(context.Background(), "unix", sock, "fake", tool.ActionStop, nil)
	if err != nil || st.On || sw.on || sw.stopped != 1 {
		t.Fatalf("stop %+v %v switch=%+v", st, err, sw)
	}
}

func TestSwitchIPCMissing(t *testing.T) {
	_, err := CallSwitch(context.Background(), "unix", filepath.Join(t.TempDir(), "missing.sock"), "fake", tool.ActionStatus, nil)
	if !errors.Is(err, ErrNoApp) {
		t.Fatal(err)
	}
}

func TestCLIStartUsesTheApp(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "app.sock")
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	remote := &fakeSwitch{}
	reg, err := NewRegistry(remote)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ServeSwitch(ctx, reg, ln) }()

	prev := dialSwitch
	dialSwitch = func() (string, string, error) { return "unix", sock, nil }
	t.Cleanup(func() { dialSwitch = prev })

	local := &fakeSwitch{}
	cli, _, errb := newCLI(t, local)
	cli.Remote = true
	code := cli.Run(context.Background(), []string{"fake", tool.ActionStart, "--json"})
	if code != ExitOK {
		t.Fatal(code, errb)
	}
	if local.started != 0 || remote.started != 1 || !remote.on {
		t.Fatalf("local %+v remote %+v", local, remote)
	}
}

// timedSwitch is a fakeSwitch that can also run for a length of time.
type timedSwitch struct {
	fakeSwitch
	lengths []time.Duration
}

func (f *timedSwitch) Specs() []tool.Spec {
	return tool.TimedSwitchSpecs(i18n.Messages.About, i18n.Messages.About, i18n.Messages.About)
}

func (f *timedSwitch) StartFor(_ context.Context, d time.Duration) error {
	f.lengths = append(f.lengths, d)
	f.on = true
	return nil
}

func serveSwitch(t *testing.T, sw tool.Switch) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "app.sock")
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := NewRegistry(sw)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = ServeSwitch(ctx, reg, ln) }()
	return sock
}

// TestSwitchIPCStartForALength sends a length to the app, which runs the
// switch for it; a bad length comes back as the text of the problem, not
// as a failure of the connection.
func TestSwitchIPCStartForALength(t *testing.T) {
	sw := &timedSwitch{}
	sock := serveSwitch(t, sw)
	ctx := context.Background()

	if _, err := CallSwitch(ctx, "unix", sock, "fake", tool.ActionStart, tool.Input{tool.PropFor: "90m"}); err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{90 * time.Minute}; !slices.Equal(sw.lengths, want) {
		t.Fatalf("ran for %v, want %v", sw.lengths, want)
	}
	if _, err := CallSwitch(ctx, "unix", sock, "fake", tool.ActionStart, nil); err != nil || sw.started != 1 {
		t.Fatalf("start with no length: %v, started %d: it uses the settings used last", err, sw.started)
	}
	_, err := CallSwitch(ctx, "unix", sock, "fake", tool.ActionStart, tool.Input{tool.PropUntil: "tea time"})
	if err == nil || err.Error() != i18n.HoldClock() {
		t.Fatalf("a bad time of day: %v, want %q", err, i18n.HoldClock())
	}
	if len(sw.lengths) != 1 {
		t.Errorf("a bad input still started the switch: %v", sw.lengths)
	}
}

func TestIPCPlainSwitchNoLength(t *testing.T) {
	sw := &fakeSwitch{}
	sock := serveSwitch(t, sw)
	_, err := CallSwitch(context.Background(), "unix", sock, "fake", tool.ActionStart, tool.Input{tool.PropFor: "1h"})
	if err == nil || sw.started != 0 {
		t.Fatalf("err %v, started %d: a switch that cannot run for a length must not start without one", err, sw.started)
	}
}

// TestStatusNote: the other hold on a tool is part of
// its status, over IPC to the app and in the text and JSON of `omt status`.
func TestStatusNote(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "app.sock")
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	sw := &fakeSwitch{on: true, note: "the plan holds it too"}
	reg, err := NewRegistry(sw)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ServeSwitch(ctx, reg, ln) }()

	st, err := CallSwitch(context.Background(), "unix", sock, "fake", tool.ActionStatus, nil)
	if err != nil || st.Note != sw.note {
		t.Fatalf("status over IPC: %+v %v, want the note %q", st, err, sw.note)
	}
	for _, asJSON := range []bool{false, true} {
		cli, out, errb := newCLI(t, sw)
		args := []string{"fake", "status"}
		if asJSON {
			args = append(args, "--json")
		}
		if code := cli.Run(context.Background(), args); code != ExitOK {
			t.Fatal(code, errb)
		}
		if !strings.Contains(out.String(), sw.note) {
			t.Errorf("json=%v: no note in %q", asJSON, out.String())
		}
	}
}

// screenedSwitch is a timedSwitch that can also keep the display on.
type screenedSwitch struct {
	timedSwitch
	display bool
	started []bool // the display choice at each start
}

func (f *screenedSwitch) Specs() []tool.Spec {
	return tool.ScreenedSwitchSpecs(i18n.Messages.About, i18n.Messages.About, i18n.Messages.About)
}

func (f *screenedSwitch) Display() bool      { return f.display }
func (f *screenedSwitch) SetDisplay(on bool) { f.display = on }

func (f *screenedSwitch) Start(context.Context) error {
	f.started = append(f.started, f.display)
	f.on = true
	return nil
}

func (f *screenedSwitch) StartFor(_ context.Context, d time.Duration) error {
	f.started = append(f.started, f.display)
	f.lengths = append(f.lengths, d)
	f.on = true
	return nil
}

// TestStartChoosesTheDisplay: a start that says on or off makes it the
// tool's choice before the tool starts, with a length or without one, over
// IPC as on the command line; a word that is neither refuses the start, and a
// tool with no display to choose refuses it too.
func TestStartChoosesTheDisplay(t *testing.T) {
	sw := &screenedSwitch{display: true}
	sock := serveSwitch(t, sw)
	ctx := context.Background()

	if _, err := CallSwitch(ctx, "unix", sock, "fake", tool.ActionStart, tool.Input{tool.PropDisplay: "off", tool.PropFor: "1h"}); err != nil {
		t.Fatal(err)
	}
	if _, err := CallSwitch(ctx, "unix", sock, "fake", tool.ActionStart, tool.Input{tool.PropDisplay: "on"}); err != nil {
		t.Fatal(err)
	}
	if want := []bool{false, true}; !slices.Equal(sw.started, want) {
		t.Fatalf("display at each start %v, want %v", sw.started, want)
	}
	_, err := CallSwitch(ctx, "unix", sock, "fake", tool.ActionStart, tool.Input{tool.PropDisplay: "dim"})
	if err == nil || err.Error() != i18n.HoldDisplay() || len(sw.started) != 2 {
		t.Errorf("a bad choice: %v, %d starts", err, len(sw.started))
	}

	plain := &timedSwitch{}
	_, err = StartSwitch(ctx, plain, tool.Input{tool.PropDisplay: "off"}, time.Now())
	if err == nil || plain.started != 0 {
		t.Errorf("a tool with no display choice: %v, started %d", err, plain.started)
	}
}

// TestCLIStartTakesADisplayFlag: --display is a flag of a tool that can
// choose, and of no other.
func TestCLIStartTakesADisplayFlag(t *testing.T) {
	sw := &screenedSwitch{display: true}
	cli, _, errb := newCLI(t, sw)
	code := cli.Run(context.Background(), []string{"fake", "start", "--display", "off", "--for", "20ms", "--json"})
	if code != ExitOK || sw.display || len(sw.started) != 1 || sw.started[0] {
		t.Fatalf("code %d, display %v, starts %v: %s", code, sw.display, sw.started, errb)
	}
	plain, _, _ := newCLI(t, &timedSwitch{})
	if code := plain.Run(context.Background(), []string{"fake", "start", "--display", "off"}); code != ExitUsage {
		t.Errorf("a tool with no display choice took --display: exit %d", code)
	}
}

// fakeTraffic is a capture tool: the app draws it, and the command line
// must not reach it, because a capture changes the system proxy and can
// hold the secrets of the person's traffic.
type fakeTraffic struct{ fakeSwitch }

func (f *fakeTraffic) Manifest() tool.Manifest {
	return tool.Manifest{ID: "traffic", Name: i18n.Messages.AppName, Summary: i18n.Messages.About, Kind: tool.KindSwitch}
}

func (f *fakeTraffic) Config() tool.CaptureConfig                     { return tool.CaptureConfig{} }
func (f *fakeTraffic) SetConfig(tool.CaptureConfig) error             { return nil }
func (f *fakeTraffic) Log() tool.FlowLog                              { return nil }
func (f *fakeTraffic) Rules() []tool.Rule                             { return nil }
func (f *fakeTraffic) SetRules([]tool.Rule) error                     { return nil }
func (f *fakeTraffic) Cert() (tool.CertState, error)                  { return tool.CertState{}, nil }
func (f *fakeTraffic) InstallCert(context.Context) error              { return nil }
func (f *fakeTraffic) RemoveCert(context.Context) error               { return nil }
func (f *fakeTraffic) Interfaces() ([]tool.NetInterface, error)       { return nil, nil }
func (f *fakeTraffic) Replay(context.Context, uint64) (uint64, error) { return 0, nil }
func (f *fakeTraffic) ExportHAR(io.Writer, []uint64) error            { return nil }
func (f *fakeTraffic) Environment() (tool.Environment, error)         { return tool.Environment{}, nil }
func (f *fakeTraffic) OpenTerminal(context.Context) error             { return nil }

// TestSwitchIPCRefusesACapture: the command line and MCP are told no, and
// the capture does not start.
func TestSwitchIPCRefusesACapture(t *testing.T) {
	capturer := &fakeTraffic{}
	sock := serveSwitch(t, capturer)
	for _, action := range []string{tool.ActionStart, tool.ActionStop, tool.ActionStatus} {
		_, err := CallSwitch(context.Background(), "unix", sock, "traffic", action, nil)
		if err == nil {
			t.Fatalf("%s: the command line reached the capture", action)
		}
		if !strings.Contains(err.Error(), "app") {
			t.Errorf("%s: %v, want an error that says only the app runs it", action, err)
		}
	}
	if capturer.started != 0 || capturer.on || capturer.stopped != 0 {
		t.Errorf("the capture was acted on: started=%d on=%v stopped=%d", capturer.started, capturer.on, capturer.stopped)
	}
}
