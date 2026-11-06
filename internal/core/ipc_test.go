package core

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

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

	st, err := CallSwitch(context.Background(), "unix", sock, "fake", tool.ActionStart)
	if err != nil || !st.On || sw.started != 1 {
		t.Fatalf("start %+v %v started=%d", st, err, sw.started)
	}
	st, err = CallSwitch(context.Background(), "unix", sock, "fake", tool.ActionStatus)
	if err != nil || !st.On {
		t.Fatalf("status %+v %v", st, err)
	}
	st, err = CallSwitch(context.Background(), "unix", sock, "fake", tool.ActionStop)
	if err != nil || st.On || sw.on || sw.stopped != 1 {
		t.Fatalf("stop %+v %v switch=%+v", st, err, sw)
	}
}

func TestSwitchIPCMissing(t *testing.T) {
	_, err := CallSwitch(context.Background(), "unix", filepath.Join(t.TempDir(), "missing.sock"), "fake", tool.ActionStatus)
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
