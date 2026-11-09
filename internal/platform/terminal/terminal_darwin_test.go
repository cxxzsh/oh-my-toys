//go:build darwin

package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestOpenWritesPrivateScript checks the script is created 0700 in a private
// folder, lists the variables, and is handed to Terminal. The runner stands in
// for `open`, so no window is launched.
func TestOpenWritesPrivateScript(t *testing.T) {
	vars := []Var{{Name: "HTTP_PROXY", Value: "http://127.0.0.1:8080"}, {Name: "T", Value: "it's"}}
	var (
		gotName string
		gotArgs []string
		path    string
		info    os.FileInfo
		body    []byte
	)
	fake := func(_ context.Context, name string, args ...string) error {
		gotName, gotArgs = name, args
		path = args[len(args)-1]
		var err error
		if info, err = os.Stat(path); err != nil {
			t.Errorf("stat script: %v", err)
		}
		body, err = os.ReadFile(path)
		if err != nil {
			t.Errorf("read script: %v", err)
		}
		return nil
	}
	if err := openWith(context.Background(), vars, fake); err != nil {
		t.Fatalf("openWith: %v", err)
	}
	if gotName != openCmd || !slices.Equal(gotArgs[:2], []string{"-a", terminalApp}) {
		t.Fatalf("ran %s %q, want %s -a %s <file>", gotName, gotArgs, openCmd, terminalApp)
	}
	if filepath.Base(path) != scriptName {
		t.Fatalf("script name = %q, want %q", filepath.Base(path), scriptName)
	}
	if mode := info.Mode().Perm(); mode != scriptMode {
		t.Errorf("script mode = %o, want %o", mode, scriptMode)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat folder: %v", err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o700 {
		t.Errorf("folder mode = %o, want 0700", mode)
	}
	for _, want := range []string{"export HTTP_PROXY='http://127.0.0.1:8080'", "export T='it'\\''s'"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("script lacks %q:\n%s", want, body)
		}
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
}

// TestOpenRemovesFolderOnFailure checks that a script which Terminal never
// receives does not stay behind in the temporary directory.
func TestOpenRemovesFolderOnFailure(t *testing.T) {
	var path string
	fake := func(_ context.Context, _ string, args ...string) error {
		path = args[len(args)-1]
		return errors.New("no terminal")
	}
	if err := openWith(context.Background(), nil, fake); err == nil {
		t.Fatal("a failed open was reported as success")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("folder still exists after failure: %v", err)
	}
}

// TestOpenRejectsBadNameBeforeWriting checks nothing is written for a bad name.
func TestOpenRejectsBadNameBeforeWriting(t *testing.T) {
	called := false
	fake := func(context.Context, string, ...string) error {
		called = true
		return nil
	}
	if err := openWith(context.Background(), []Var{{Name: "A B", Value: "x"}}, fake); err == nil {
		t.Fatal("a bad name was accepted")
	}
	if called {
		t.Fatal("the opener ran for a bad name")
	}
}
