package ports

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// listening reads `lsof -Fpcun`: pid, command, uid, then each socket's
// address; a second call reads each process's executable path (`-d txt`).
func listening(ctx context.Context) ([]Listener, error) {
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcun").Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("run lsof: %w", err)
	}
	ls := parseLsof(out)
	paths := textPaths(ctx, ls)
	for i := range ls {
		ls[i].Path = paths[ls[i].PID]
		ls[i].System = ls[i].System || isSystemPath(ls[i].Path)
	}
	return ls, nil
}

func parseLsof(out []byte) []Listener {
	var (
		ls      []Listener
		pid     int
		command string
		system  bool
	)
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
			system = false
		case 'c':
			command = line[1:]
		case 'u':
			uid, _ := strconv.Atoi(line[1:])
			system = uid == 0
		case 'n':
			if host, port, ok := splitHostPort(line[1:]); ok {
				ls = append(ls, Listener{Port: port, Address: host, PID: pid, Process: command, System: system})
			}
		}
	}
	return ls
}

func textPaths(ctx context.Context, ls []Listener) map[int]string {
	paths := map[int]string{}
	if len(ls) == 0 {
		return paths
	}
	var pids []string
	for _, l := range ls {
		if _, seen := paths[l.PID]; !seen && l.PID > 0 {
			paths[l.PID] = ""
			pids = append(pids, strconv.Itoa(l.PID))
		}
	}
	out, _ := exec.CommandContext(ctx, "lsof", "-nP", "-a", "-p", strings.Join(pids, ","), "-d", "txt", "-Fpn").Output() //nolint:gosec // G204: pids are formatted integers
	var pid int
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "p"):
			pid, _ = strconv.Atoi(line[1:])
		case strings.HasPrefix(line, "n") && paths[pid] == "":
			paths[pid] = line[1:]
		}
	}
	return paths
}
