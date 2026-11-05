// Package ports lists the TCP ports processes listen on, and ends those
// processes, with each system's own command: lsof on macOS, ss on Linux,
// netstat and tasklist on Windows.
package ports

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Listener is a process listening on a TCP port.
type Listener struct {
	Port    int
	Address string // "*", "127.0.0.1", "[::1]"
	PID     int
	Process string
}

// Listening returns the listening TCP ports, by port then PID, each port
// and process once.
func Listening(ctx context.Context) ([]Listener, error) {
	ls, err := listening(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(ls, func(a, b Listener) int {
		return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.PID, b.PID), strings.Compare(a.Address, b.Address))
	})
	return slices.CompactFunc(ls, func(a, b Listener) bool { return a.Port == b.Port && a.PID == b.PID }), nil
}

// Kill ends a process.
func Kill(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := p.Kill(); err != nil {
		return fmt.Errorf("end process %d: %w", pid, err)
	}
	return nil
}

// splitHostPort splits "127.0.0.1:8080", "[::1]:5432" or "*:3000" at the
// last colon.
func splitHostPort(s string) (string, int, bool) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return "", 0, false
	}
	port, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return "", 0, false
	}
	return s[:i], port, true
}
