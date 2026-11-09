package terminal

import (
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
)

// hostileValue has every character that a quoting mistake would let a shell
// act on: quotes of both kinds, spaces, expansions, a command substitution,
// a newline and a percent sign.
const hostileValue = "it's \"quoted\" $HOME `id` %PATH% line\nnext"

func TestCheckVarsRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "1A", "A-B", "A=B", "A B", "$A", "A;rm", "X'Y", "a.b"} {
		if err := checkVars([]Var{{Name: name, Value: "v"}}); err == nil {
			t.Errorf("name %q accepted, want an error", name)
		}
	}
}

func TestCheckVarsAcceptsNames(t *testing.T) {
	for _, name := range []string{"A", "_x", "HTTP_PROXY", "a1_B2"} {
		if err := checkVars([]Var{{Name: name, Value: "v"}}); err != nil {
			t.Errorf("name %q: %v", name, err)
		}
	}
}

func TestCheckVarsRejectsNUL(t *testing.T) {
	if err := checkVars([]Var{{Name: "A", Value: "a\x00b"}}); err == nil {
		t.Fatal("a value with a NUL byte was accepted")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "''"},
		{"plain", "'plain'"},
		{"two words", "'two words'"},
		{"it's", `'it'\''s'`},
		{"$HOME `id` \"x\"", "'$HOME `id` \"x\"'"},
		{"a\nb", "'a\nb'"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDarwinScript(t *testing.T) {
	const self = "/tmp/omt-terminal-1/session.command"
	tests := []struct {
		name string
		vars []Var
		want string
	}{
		{
			name: "no variables",
			want: "#!/bin/sh\n" +
				"cd -- \"${HOME:-/}\" 2>/dev/null\n" +
				"rm -f -- '/tmp/omt-terminal-1/session.command'\n" +
				"rmdir -- '/tmp/omt-terminal-1' 2>/dev/null\n" +
				"exec \"${SHELL:-/bin/zsh}\" -l\n",
		},
		{
			name: "plain and spaced values",
			vars: []Var{{Name: "HTTPS_PROXY", Value: "http://127.0.0.1:8080"}, {Name: "NAME", Value: "a b"}},
			want: "#!/bin/sh\n" +
				"cd -- \"${HOME:-/}\" 2>/dev/null\n" +
				"rm -f -- '/tmp/omt-terminal-1/session.command'\n" +
				"rmdir -- '/tmp/omt-terminal-1' 2>/dev/null\n" +
				"export HTTPS_PROXY='http://127.0.0.1:8080'\n" +
				"export NAME='a b'\n" +
				"exec \"${SHELL:-/bin/zsh}\" -l\n",
		},
		{
			name: "hostile value is single-quoted",
			vars: []Var{{Name: "V", Value: hostileValue}},
			want: "#!/bin/sh\n" +
				"cd -- \"${HOME:-/}\" 2>/dev/null\n" +
				"rm -f -- '/tmp/omt-terminal-1/session.command'\n" +
				"rmdir -- '/tmp/omt-terminal-1' 2>/dev/null\n" +
				"export V='it'\\''s \"quoted\" $HOME `id` %PATH% line\nnext'\n" +
				"exec \"${SHELL:-/bin/zsh}\" -l\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := darwinScript(tt.vars, self)
			if err != nil {
				t.Fatalf("darwinScript: %v", err)
			}
			if got != tt.want {
				t.Fatalf("script =\n%s\nwant =\n%s", got, tt.want)
			}
		})
	}
}

func TestDarwinScriptRejectsBadName(t *testing.T) {
	if _, err := darwinScript([]Var{{Name: "A B", Value: "x"}}, "/tmp/s/session.command"); err == nil {
		t.Fatal("a bad name was accepted")
	}
}

func TestWindowsArgs(t *testing.T) {
	tests := []struct {
		name string
		vars []Var
		last string
	}{
		{name: "no variables", last: ""},
		{
			name: "one variable",
			vars: []Var{{Name: "A", Value: "B"}},
			last: "$env:A=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('Qg=='))",
		},
		{
			name: "two variables",
			vars: []Var{{Name: "A", Value: "B"}, {Name: "C", Value: "d e"}},
			last: "$env:A=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('Qg=='))" +
				"; $env:C=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('ZCBl'))",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := windowsArgs(tt.vars)
			if err != nil {
				t.Fatalf("windowsArgs: %v", err)
			}
			want := []string{"/c", "start", "", "powershell.exe", "-NoExit", "-Command", tt.last}
			if !slices.Equal(got, want) {
				t.Fatalf("args =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestWindowsArgsKeepsHostileValue checks that a value full of cmd.exe and
// PowerShell metacharacters reaches PowerShell intact: it is base64 in the
// command line and decodes back to the exact bytes.
func TestWindowsArgsKeepsHostileValue(t *testing.T) {
	got, err := windowsArgs([]Var{{Name: "V", Value: hostileValue}})
	if err != nil {
		t.Fatalf("windowsArgs: %v", err)
	}
	script := got[len(got)-1]
	const marker = "FromBase64String('"
	start := strings.Index(script, marker)
	if start < 0 {
		t.Fatalf("no base64 literal in %q", script)
	}
	rest := script[start+len(marker):]
	encoded, _, found := strings.Cut(rest, "'")
	if !found {
		t.Fatalf("unterminated base64 literal in %q", script)
	}
	if strings.ContainsAny(encoded, "\"%'`$ ") {
		t.Fatalf("encoded value %q holds a character cmd.exe or PowerShell would read", encoded)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != hostileValue {
		t.Fatalf("decoded %q, want %q", decoded, hostileValue)
	}
}

func TestWindowsArgsRejectsBadName(t *testing.T) {
	if _, err := windowsArgs([]Var{{Name: "A;calc", Value: "x"}}); err == nil {
		t.Fatal("a bad name was accepted")
	}
}

func TestLinuxCommand(t *testing.T) {
	vars := []Var{{Name: "A", Value: "b c"}}
	const script = "export A='b c'; exec \"${SHELL:-/bin/sh}\""
	tests := []struct {
		name      string
		installed []string
		wantName  string
		wantArgs  []string
	}{
		{
			name:      "x-terminal-emulator first",
			installed: []string{"xterm", "x-terminal-emulator", "gnome-terminal"},
			wantName:  "x-terminal-emulator",
			wantArgs:  []string{"-e", "/bin/sh", "-c", script},
		},
		{
			name:      "gnome-terminal takes --",
			installed: []string{"xterm", "gnome-terminal"},
			wantName:  "gnome-terminal",
			wantArgs:  []string{"--", "/bin/sh", "-c", script},
		},
		{
			name:      "konsole",
			installed: []string{"konsole", "alacritty"},
			wantName:  "konsole",
			wantArgs:  []string{"-e", "/bin/sh", "-c", script},
		},
		{
			name:      "xfce4-terminal runs the rest of the line",
			installed: []string{"xfce4-terminal", "kitty"},
			wantName:  "xfce4-terminal",
			wantArgs:  []string{"-x", "/bin/sh", "-c", script},
		},
		{
			name:      "kitty takes the command directly",
			installed: []string{"kitty", "xterm"},
			wantName:  "kitty",
			wantArgs:  []string{"/bin/sh", "-c", script},
		},
		{
			name:      "alacritty before xterm",
			installed: []string{"xterm", "alacritty"},
			wantName:  "alacritty",
			wantArgs:  []string{"-e", "/bin/sh", "-c", script},
		},
		{
			name:      "xterm last resort",
			installed: []string{"xterm"},
			wantName:  "xterm",
			wantArgs:  []string{"-e", "/bin/sh", "-c", script},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			has := func(name string) bool { return slices.Contains(tt.installed, name) }
			name, args, err := linuxCommand(vars, has)
			if err != nil {
				t.Fatalf("linuxCommand: %v", err)
			}
			if name != tt.wantName || !slices.Equal(args, tt.wantArgs) {
				t.Fatalf("got %s %q, want %s %q", name, args, tt.wantName, tt.wantArgs)
			}
		})
	}
}

func TestLinuxCommandNoTerminal(t *testing.T) {
	_, _, err := linuxCommand(nil, func(string) bool { return false })
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestLinuxCommandHostileValue(t *testing.T) {
	_, args, err := linuxCommand([]Var{{Name: "V", Value: hostileValue}}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("linuxCommand: %v", err)
	}
	want := "export V='it'\\''s \"quoted\" $HOME `id` %PATH% line\nnext'; exec \"${SHELL:-/bin/sh}\""
	if got := args[len(args)-1]; got != want {
		t.Fatalf("script = %q, want %q", got, want)
	}
}

func TestLinuxCommandRejectsBadName(t *testing.T) {
	_, _, err := linuxCommand([]Var{{Name: "A=B", Value: "x"}}, func(string) bool { return true })
	if err == nil {
		t.Fatal("a bad name was accepted")
	}
}
