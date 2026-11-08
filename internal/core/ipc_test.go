package core

import (
	"context"
	"errors"
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
