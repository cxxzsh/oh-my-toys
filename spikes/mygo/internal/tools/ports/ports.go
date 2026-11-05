// Package ports shows which processes listen on TCP ports, and frees a
// port by ending the processes on it.
package ports

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/platform/ports"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

// ID is the tool's ID and command name.
const ID = "ports"

// fieldPort is the ID of the port field.
const fieldPort = "port"

// maxPort is the largest TCP port.
const maxPort = 65535

// errBadPort is a port field that is not a port number.
var errBadPort = errors.New("not a port number")

// Tool is Ports.
type Tool struct{}

// New makes the tool.
func New() *Tool { return &Tool{} }

// Manifest describes the tool.
func (*Tool) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:        ID,
		Name:      i18n.Messages.PortsName,
		Summary:   i18n.Messages.PortsSummary,
		Category:  tool.CategoryNetwork,
		Kind:      tool.KindAction,
		Icon:      "plug",
		Keywords:  []string{"port", "lsof", "netstat", "kill", "listen", "端口占用"},
		Platforms: []string{"macos", "windows", "linux"},
	}
}

// Fields asks for a port; empty lists every listening port.
func (*Tool) Fields() []tool.Field {
	return []tool.Field{{ID: fieldPort, Label: i18n.Messages.PortsField, Placeholder: i18n.Messages.PortsFieldHint}}
}

// Plan lists the listeners, of one port when the field names one. With a
// port, the plan ends its processes.
func (*Tool) Plan(ctx context.Context, in tool.Input) (tool.Plan, error) {
	port, err := parsePort(in[fieldPort])
	if err != nil {
		return tool.Plan{}, err
	}
	ls, err := ports.Listening(ctx)
	if err != nil {
		return tool.Plan{}, fmt.Errorf("list ports: %w", err)
	}
	if port > 0 {
		ls = only(ls, port)
	}
	p := tool.Plan{Summary: i18n.PortsListening(len(ls)), Rows: rows(ls)}
	if port > 0 && len(ls) > 0 {
		p.Summary = i18n.PortsOnPort(port, len(ls))
		p.ApplyLabel = i18n.PortsEnd(len(ls))
		p.Destructive = true
		p.Data = ls
	}
	return p, nil
}

// Apply ends the processes the plan lists.
func (*Tool) Apply(_ context.Context, p tool.Plan) (tool.Result, error) {
	ls, _ := p.Data.([]ports.Listener)
	var errs []error
	for _, l := range ls {
		if err := ports.Kill(l.PID); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return tool.Result{}, err
	}
	return tool.Result{Summary: i18n.PortsEnded(len(ls))}, nil
}

func parsePort(s string) (int, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), ":")
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > maxPort {
		return 0, &tool.Problem{Text: i18n.PortsBadPort(s), Err: fmt.Errorf("%q: %w", s, errBadPort)}
	}
	return n, nil
}

func only(ls []ports.Listener, port int) []ports.Listener {
	var out []ports.Listener
	for _, l := range ls {
		if l.Port == port {
			out = append(out, l)
		}
	}
	return out
}

func rows(ls []ports.Listener) []tool.Row {
	out := make([]tool.Row, 0, len(ls))
	for _, l := range ls {
		name := l.Process
		if name == "" {
			name = i18n.PortsUnknownProcess()
		}
		out = append(out, tool.Row{
			Title:  i18n.PortsRowTitle(l.Port, name),
			Detail: i18n.PortsRowDetail(l.PID, l.Address),
			Value:  strconv.Itoa(l.Port),
		})
	}
	return out
}
