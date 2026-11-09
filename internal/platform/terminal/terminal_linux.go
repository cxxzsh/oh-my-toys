//go:build linux

package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// runner starts one command and returns without waiting for it to finish.
type runner func(ctx context.Context, name string, args ...string) error

// open starts the first terminal emulator found on PATH. A terminal needs a
// display to draw on, so a session with none is refused up front.
func open(ctx context.Context, vars []Var) error {
	return openWith(ctx, vars, hasCommand, startCommand)
}

// openWith is open with the lookup and the starter to test it through.
func openWith(ctx context.Context, vars []Var, has func(string) bool, r runner) error {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return ErrUnsupported
	}
	name, args, err := linuxCommand(vars, has)
	if err != nil {
		return err
	}
	if err := r(ctx, name, args...); err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	return nil
}

// hasCommand reports whether name is on PATH.
func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// startCommand starts the terminal and leaves it running: the window belongs
// to the person, so the call returns once it has started. The context does
// not stop it, because a context cancelled after this call must not close
// the window.
func startCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(context.WithoutCancel(ctx), name, args...) //nolint:gosec // G204: name is one of the listed emulators and args are validated
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the process when the window closes, so it does not linger as a
	// zombie for the life of the app.
	go func() { _ = cmd.Wait() }()
	return nil
}
