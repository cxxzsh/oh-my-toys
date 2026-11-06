package ports

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ssUser matches a process of ss's users column: ("name",pid=123,fd=4).
var ssUser = regexp.MustCompile(`\("([^"]*)",pid=(\d+)`)

// listening reads `ss -ltnpH`: state, queues, local address, peer, users.
// Processes of other users show without a name or PID unless run as root.
func listening(ctx context.Context) ([]Listener, error) {
	out, err := exec.CommandContext(ctx, "ss", "-ltnpH").Output()
	if err != nil {
		return nil, fmt.Errorf("run ss: %w", err)
	}
	var ls []Listener
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		ls = append(ls, parseSSLine(sc.Text())...)
	}
	return ls, nil
}

const ssLocalColumn = 3

func parseSSLine(line string) []Listener {
	f := strings.Fields(line)
	if len(f) <= ssLocalColumn {
		return nil
	}
	host, port, ok := splitHostPort(f[ssLocalColumn])
	if !ok {
		return nil
	}
	users := ssUser.FindAllStringSubmatch(line, -1)
	if len(users) == 0 {
		return []Listener{{Port: port, Address: host}}
	}
	ls := make([]Listener, 0, len(users))
	for _, u := range users {
		pid, _ := strconv.Atoi(u[2])
		ls = append(ls, Listener{Port: port, Address: host, PID: pid, Process: u[1]})
	}
	return ls
}
