package ports

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// Columns of `netstat -ano`: protocol, local, foreign, state, PID.
const (
	netstatColumns = 5
	netstatLocal   = 1
	netstatState   = 3
	netstatPID     = 4
)

// listening reads `netstat -ano` for the listening sockets and `tasklist`
// for the names of their processes.
func listening(ctx context.Context) ([]Listener, error) {
	out, err := hidden(exec.CommandContext(ctx, "netstat", "-ano", "-p", "TCP")).Output()
	if err != nil {
		return nil, fmt.Errorf("run netstat: %w", err)
	}
	v6, _ := hidden(exec.CommandContext(ctx, "netstat", "-ano", "-p", "TCPv6")).Output()
	names := processNames(ctx)
	var ls []Listener
	for _, b := range [][]byte{out, v6} {
		sc := bufio.NewScanner(bytes.NewReader(b))
		for sc.Scan() {
			if l, ok := parseNetstatLine(sc.Text()); ok {
				l.Process = names[l.PID]
				ls = append(ls, l)
			}
		}
	}
	return ls, nil
}

func parseNetstatLine(line string) (Listener, bool) {
	f := strings.Fields(line)
	if len(f) != netstatColumns || f[netstatState] != "LISTENING" {
		return Listener{}, false
	}
	host, port, ok := splitHostPort(f[netstatLocal])
	if !ok {
		return Listener{}, false
	}
	pid, _ := strconv.Atoi(f[netstatPID])
	return Listener{Port: port, Address: host, PID: pid}, true
}

// processNames maps PIDs to image names with `tasklist /FO CSV /NH`.
func processNames(ctx context.Context) map[int]string {
	names := map[int]string{}
	out, err := hidden(exec.CommandContext(ctx, "tasklist", "/FO", "CSV", "/NH")).Output()
	if err != nil {
		return names
	}
	rows, _ := csv.NewReader(bytes.NewReader(out)).ReadAll()
	for _, r := range rows {
		if len(r) > 1 {
			if pid, err := strconv.Atoi(r[1]); err == nil {
				names[pid] = r[0]
			}
		}
	}
	return names
}

// hidden keeps a console window from flashing up for the command.
func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}
