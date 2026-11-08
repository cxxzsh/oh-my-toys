//go:build darwin

package pcap

// The macOS parts that do not talk to libpcap: which interface the default
// route uses, and the one system prompt that can make the capture devices
// usable.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// The programs below are named by absolute path: a copy earlier on PATH
// must not be able to answer for them.
const (
	// routeCommand is how macOS reports the route a destination takes.
	routeCommand = "/sbin/route"
	// osascriptCommand runs AppleScript; the one below raises the system's
	// own administrator prompt.
	osascriptCommand = "/usr/bin/osascript"
)

// adminGroup is the group macOS puts the first user of the machine in, and
// the one the capture devices are handed to.
const adminGroup = "admin"

// bpfGrantCommand hands the capture devices to the administrators' group
// until the next restart. libpcap opens /dev/bpfN read-write and only falls
// back to read-only when that is refused (pcap-bpf.c, bpf_open), so group
// read alone would capture; group read-write is what Wireshark's ChmodBPF
// grants, and it leaves the preferred path working.
//
// It must not be widened to other users: reading these devices means
// reading every packet the machine sends and receives. macOS creates more
// /dev/bpfN as they are taken, and a device created after this runs starts
// root-only again, so the person may have to grant again later; Open
// reports that as ErrPermission.
const bpfGrantCommand = "chgrp " + adminGroup + " /dev/bpf* && chmod g+rw /dev/bpf*"

func defaultInterface() (string, error) {
	out, routeErr := exec.CommandContext(context.Background(), routeCommand, "-n", "get", "default").Output()
	if routeErr == nil {
		if name := routeCommandInterface(out); name != "" {
			return name, nil
		}
		routeErr = errors.New("route named no interface")
	}
	ifaces, err := list()
	if err != nil {
		return "", err
	}
	if name, ok := pickDefault(ifaces); ok {
		return name, nil
	}
	return "", fmt.Errorf("pcap: default interface: %w", routeErr)
}

// routeCommandInterface reads the interface line of `route -n get default`:
//
//	route to: default
//	destination: default
//	...
//	interface: en0
func routeCommandInterface(out []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		name, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "interface:")
		if !ok {
			continue
		}
		return strings.TrimSpace(name)
	}
	return ""
}

// grantScript is the AppleScript that raises the administrator prompt, runs
// command with those rights, and shows reason in the prompt.
func grantScript(command, reason string) string {
	return `do shell script "` + appleQuote(command) +
		`" with administrator privileges with prompt "` + appleQuote(reason) + `"`
}

// appleQuote escapes s for an AppleScript string literal, where a
// backslash and a double quote each need one of their own. Nothing that
// passes through it can end the literal.
func appleQuote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(escaped, `"`, `\"`)
}

func grant(ctx context.Context, reason string) error {
	script := grantScript(bpfGrantCommand, reason)
	out, err := exec.CommandContext(ctx, osascriptCommand, "-e", script).CombinedOutput() //nolint:gosec // G204: the script is one constant command plus a sentence appleQuote cannot break out of
	if err != nil {
		if cancelled(out) {
			return fmt.Errorf("pcap: grant: %w: the prompt was dismissed", ErrPermission)
		}
		if detail := strings.TrimSpace(string(out)); detail != "" {
			return fmt.Errorf("pcap: grant: %w: %s", err, detail)
		}
		return fmt.Errorf("pcap: grant: %w", err)
	}
	return nil
}

// cancelled reports the osascript failure that means the person refused:
// userCanceledErr is -128, which osascript prints with the message.
func cancelled(out []byte) bool {
	text := strings.ToLower(string(out))
	return strings.Contains(text, "user canceled") || strings.Contains(text, "-128")
}
