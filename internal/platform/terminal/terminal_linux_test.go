//go:build linux

package terminal

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// TestOpenNeedsDisplay checks that no terminal is started without a display.
func TestOpenNeedsDisplay(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	called := false
	fake := func(context.Context, string, ...string) error {
		called = true
		return nil
	}
	err := openWith(context.Background(), nil, func(string) bool { return true }, fake)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if called {
		t.Fatal("a terminal was started without a display")
	}
}

// TestOpenAcceptsWaylandDisplay checks that a Wayland session counts as a display.
func TestOpenAcceptsWaylandDisplay(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	var got []string
	fake := func(_ context.Context, name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}
	if err := openWith(context.Background(), nil, func(n string) bool { return n == "xterm" }, fake); err != nil {
		t.Fatalf("openWith: %v", err)
	}
	if want := []string{"xterm", "-e", "/bin/sh", "-c", `exec "${SHELL:-/bin/sh}"`}; !slices.Equal(got, want) {
		t.Fatalf("ran %q, want %q", got, want)
	}
}

// TestOpenNoTerminal checks the error when no emulator is installed.
func TestOpenNoTerminal(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	fake := func(context.Context, string, ...string) error {
		t.Fatal("a terminal was started with none installed")
		return nil
	}
	err := openWith(context.Background(), nil, func(string) bool { return false }, fake)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// TestOpenWrapsStartFailure checks a start error is returned, not dropped.
func TestOpenWrapsStartFailure(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	boom := errors.New("boom")
	fake := func(context.Context, string, ...string) error { return boom }
	err := openWith(context.Background(), nil, func(string) bool { return true }, fake)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap boom", err)
	}
}

// TestOpenRejectsBadName checks a bad name is refused before anything starts.
func TestOpenRejectsBadName(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	fake := func(context.Context, string, ...string) error {
		t.Fatal("a terminal was started for a bad name")
		return nil
	}
	err := openWith(context.Background(), []Var{{Name: "A;B", Value: "x"}}, func(string) bool { return true }, fake)
	if err == nil {
		t.Fatal("a bad name was accepted")
	}
}
