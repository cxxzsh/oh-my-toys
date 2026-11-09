//go:build darwin

package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// openCmd is the absolute path of the tool that opens a file in an app, so a
// `open` found through PATH cannot be run instead. terminalApp names the app
// that runs a `.command` file in a new window.
const (
	openCmd     = "/usr/bin/open"
	terminalApp = "Terminal"
	scriptName  = "session.command"

	// scriptMode keeps the variables private: only the person can read the
	// script that holds them.
	scriptMode = 0o700
)

// runner runs one command and returns when it has finished; tests pass a fake.
type runner func(ctx context.Context, name string, args ...string) error

// execRunner runs the command for real.
func execRunner(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run() //nolint:gosec // G204: name is a constant and args are a path this package made
}

// open writes the session script to a private folder and has Terminal run it.
func open(ctx context.Context, vars []Var) error { return openWith(ctx, vars, execRunner) }

// openWith is open with the command runner to test it through.
func openWith(ctx context.Context, vars []Var, r runner) error {
	if err := checkVars(vars); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "omt-terminal-")
	if err != nil {
		return fmt.Errorf("make terminal script folder: %w", err)
	}
	path := filepath.Join(dir, scriptName)
	script, err := darwinScript(vars, path)
	if err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	if err := os.WriteFile(path, []byte(script), scriptMode); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("write terminal script: %w", err)
	}
	if err := r(ctx, openCmd, "-a", terminalApp, path); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("open terminal: %w", err)
	}
	return nil
}
