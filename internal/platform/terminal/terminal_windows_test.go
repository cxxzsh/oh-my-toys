//go:build windows

package terminal

import (
	"context"
	"slices"
	"testing"
)

// TestOpenWithPassesArgs checks the interpreter and the arguments reach the runner.
func TestOpenWithPassesArgs(t *testing.T) {
	var gotName string
	var gotArgs []string
	fake := func(_ context.Context, name string, args ...string) error {
		gotName, gotArgs = name, args
		return nil
	}
	const tool = `C:\Windows\System32\cmd.exe`
	if err := openWith(context.Background(), []Var{{Name: "A", Value: "B"}}, tool, fake); err != nil {
		t.Fatalf("openWith: %v", err)
	}
	if gotName != tool || gotArgs[0] != "/c" || gotArgs[3] != "powershell.exe" {
		t.Fatalf("ran %s %q", gotName, gotArgs)
	}
	if !slices.Contains(gotArgs, "-NoExit") {
		t.Fatalf("args %q lack -NoExit", gotArgs)
	}
}
