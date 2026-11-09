//go:build windows

package terminal

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// cmdName is the console interpreter that starts the new window. It is named
// by its path in the system directory, never through PATH: a tool planted
// earlier on PATH could run in place of it.
const cmdName = "cmd.exe"

// runner runs one command and returns when it has finished; tests pass a fake.
type runner func(ctx context.Context, name string, args ...string) error

// runCommand runs the command for real, with no console window of its own.
func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: name is built from the system directory and args are validated
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// open starts PowerShell in a new console window with vars set.
func open(ctx context.Context, vars []Var) error {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("find the system directory: %w", err)
	}
	return openWith(ctx, vars, filepath.Join(dir, cmdName), runCommand)
}

// openWith is open with the interpreter path and the runner to test it through.
func openWith(ctx context.Context, vars []Var, tool string, r runner) error {
	args, err := windowsArgs(vars)
	if err != nil {
		return err
	}
	if err := r(ctx, tool, args...); err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	return nil
}
