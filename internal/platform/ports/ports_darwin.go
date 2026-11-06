package ports

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// listening reads `lsof -Fpcn`: a "p<pid>" line, a "c<command>" line, then
// an "n<address>" line per socket of that process.
func listening(ctx context.Context) ([]Listener, error) {
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn").Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("run lsof: %w", err)
	}
	return parseLsof(out), nil
}

func parseLsof(out []byte) []Listener {
	var (
		ls      []Listener
		pid     int
		command string
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
		case 'c':
			command = line[1:]
		case 'n':
			if host, port, ok := splitHostPort(line[1:]); ok {
				ls = append(ls, Listener{Port: port, Address: host, PID: pid, Process: command})
			}
		}
	}
	return ls
}
