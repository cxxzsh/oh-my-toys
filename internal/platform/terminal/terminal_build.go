package terminal

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// validName is the shape of a variable name that every shell here accepts
// without quoting. Anything else is refused before a command line is built,
// so a name can never inject shell or PowerShell syntax.
var validName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// shellFallback is the shell a new window starts when the person has none
// set, on each system.
const (
	posixFallbackShell = "/bin/sh"
	darwinFallbackSh   = "/bin/zsh"
)

// checkVars refuses a name that is not a plain identifier, and a value a
// process environment cannot hold.
func checkVars(vars []Var) error {
	for _, v := range vars {
		if !validName.MatchString(v.Name) {
			return fmt.Errorf("invalid environment variable name %q", v.Name)
		}
		if strings.ContainsRune(v.Value, 0) {
			return fmt.Errorf("environment variable %s: value contains a NUL byte", v.Name)
		}
	}
	return nil
}

// shellQuote wraps s in single quotes so a POSIX shell reads it as one word
// with no expansion. An embedded quote closes the string, adds an escaped
// quote, and reopens it.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// exportCommands is one `export` statement per variable.
func exportCommands(vars []Var) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, "export "+v.Name+"="+shellQuote(v.Value))
	}
	return out
}

// darwinScript is the file Terminal runs in its new window. It moves out of
// the script's own folder and deletes the script and that folder before the
// shell starts, so nothing is left behind in the temporary directory. Then it
// starts the person's login shell with the variables set.
func darwinScript(vars []Var, self string) (string, error) {
	if err := checkVars(vars); err != nil {
		return "", err
	}
	lines := make([]string, 0, len(vars)+4)
	lines = append(lines,
		"#!/bin/sh",
		`cd -- "${HOME:-/}" 2>/dev/null`,
		"rm -f -- "+shellQuote(self),
		"rmdir -- "+shellQuote(filepath.Dir(self))+" 2>/dev/null",
	)
	lines = append(lines, exportCommands(vars)...)
	lines = append(lines, `exec "${SHELL:-`+darwinFallbackSh+`}" -l`)
	return strings.Join(lines, "\n") + "\n", nil
}

// windowsArgs are the arguments of cmd.exe that open a new console window
// running PowerShell with the variables set. Each value is sent as base64
// and decoded inside PowerShell: cmd.exe treats a double quote as a quote and
// expands %NAME%, so a value written out in the command line would be
// changed or cut short. Base64 holds only letters, digits and + / =, which
// neither cmd.exe nor PowerShell interprets.
func windowsArgs(vars []Var) ([]string, error) {
	if err := checkVars(vars); err != nil {
		return nil, err
	}
	sets := make([]string, 0, len(vars))
	for _, v := range vars {
		encoded := base64.StdEncoding.EncodeToString([]byte(v.Value))
		sets = append(sets, fmt.Sprintf(
			"$env:%s=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('%s'))",
			v.Name, encoded))
	}
	return []string{"/c", "start", "", "powershell.exe", "-NoExit", "-Command", strings.Join(sets, "; ")}, nil
}

// linuxTerminal is a terminal emulator and the flag that makes it run the
// rest of its command line. A nil flag means it takes the command directly.
type linuxTerminal struct {
	name string
	flag []string
}

// linuxTerminals are tried in this order; the first one on PATH is used.
var linuxTerminals = []linuxTerminal{
	{name: "x-terminal-emulator", flag: []string{"-e"}},
	{name: "gnome-terminal", flag: []string{"--"}},
	{name: "konsole", flag: []string{"-e"}},
	{name: "xfce4-terminal", flag: []string{"-x"}},
	{name: "kitty"},
	{name: "alacritty", flag: []string{"-e"}},
	{name: "xterm", flag: []string{"-e"}},
}

// posixScript is the command a Linux terminal runs: export the variables,
// then replace the shell with the person's shell.
func posixScript(vars []Var) string {
	cmds := append(exportCommands(vars), `exec "${SHELL:-`+posixFallbackShell+`}"`)
	return strings.Join(cmds, "; ")
}

// linuxCommand picks the first terminal emulator that has reports as present
// and returns its name and arguments. It fails with ErrUnsupported when none
// is present.
func linuxCommand(vars []Var, has func(name string) bool) (string, []string, error) {
	if err := checkVars(vars); err != nil {
		return "", nil, err
	}
	for _, t := range linuxTerminals {
		if has(t.name) {
			return t.name, slices.Concat(t.flag, []string{posixFallbackShell, "-c", posixScript(vars)}), nil
		}
	}
	return "", nil, ErrUnsupported
}
