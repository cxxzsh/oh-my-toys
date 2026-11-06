// Package mcp serves the tools over the Model Context Protocol on stdio.
// Read actions and switch tools are exposed. Actions that change files or
// end processes are not, until a person can confirm them.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

const version = "0.1.0"

// Server speaks MCP for one registry. Switch calls go to the running desktop
// app when it is listening, and otherwise run in this process.
type Server struct {
	Registry *core.Registry
}

// Run serves until the client disconnects.
func (s Server) Run(ctx context.Context) error {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "oh-my-toys", Version: version}, &mcpsdk.ServerOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	for _, t := range s.Registry.All() {
		s.add(srv, t)
	}
	defer stopLocal(s.Registry)
	return srv.Run(ctx, &mcpsdk.StdioTransport{})
}

func (s Server) add(srv *mcpsdk.Server, t tool.Tool) {
	id := t.Manifest().ID
	for _, spec := range t.Specs() {
		if !exposed(spec) {
			continue
		}
		srv.AddTool(mcpTool(id, spec), s.handler(t, spec))
	}
}

func exposed(spec tool.Spec) bool {
	switch spec.Name {
	case tool.ActionPlan, tool.ActionStart, tool.ActionStop, tool.ActionStatus:
		return true
	default:
		return false
	}
}

func mcpTool(id string, spec tool.Spec) *mcpsdk.Tool {
	msgs := i18n.In("en")
	ro := spec.Risk == tool.RiskRead
	dest := spec.Risk == tool.RiskWrite || spec.Risk == tool.RiskProcess
	closed := false
	return &mcpsdk.Tool{
		Name:         id + "_" + spec.Name,
		Description:  spec.Summary(msgs),
		InputSchema:  core.JSONSchema(spec.Input, msgs),
		OutputSchema: core.JSONSchema(spec.Output, msgs),
		Annotations: &mcpsdk.ToolAnnotations{
			ReadOnlyHint:    ro,
			DestructiveHint: &dest,
			IdempotentHint:  spec.Idempotent,
			OpenWorldHint:   &closed,
		},
	}
}

func (s Server) handler(t tool.Tool, spec tool.Spec) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		out, err := s.call(ctx, t, spec, arguments(req))
		if err != nil {
			return toolError(err), nil
		}
		return structured(out), nil
	}
}

func (s Server) call(ctx context.Context, t tool.Tool, spec tool.Spec, in tool.Input) (any, error) {
	switch spec.Name {
	case tool.ActionPlan:
		act, ok := t.(tool.Action)
		if !ok {
			return nil, fmt.Errorf("%s has no plan", t.Manifest().ID)
		}
		p, err := act.Plan(ctx, in)
		if err != nil {
			return nil, err
		}
		return planValue(p), nil
	case tool.ActionStart, tool.ActionStop, tool.ActionStatus:
		return s.switchCall(ctx, t, spec.Name)
	default:
		return nil, fmt.Errorf("action %s is not exposed", spec.Name)
	}
}

func (s Server) switchCall(ctx context.Context, t tool.Tool, action string) (any, error) {
	id := t.Manifest().ID
	if network, address, err := core.SwitchDial(); err == nil {
		st, err := core.CallSwitch(ctx, network, address, id, action)
		if err == nil {
			return statusValue(st), nil
		}
		if !errors.Is(err, core.ErrNoApp) {
			return nil, err
		}
	}
	sw, ok := t.(tool.Switch)
	if !ok {
		return nil, fmt.Errorf("%s is not a switch", id)
	}
	if err := runLocal(ctx, sw, action); err != nil {
		return nil, err
	}
	return statusValue(sw.Status()), nil
}

func runLocal(ctx context.Context, sw tool.Switch, action string) error {
	switch action {
	case tool.ActionStart:
		return sw.Start(ctx)
	case tool.ActionStop:
		return sw.Stop()
	default:
		return nil
	}
}

func stopLocal(reg *core.Registry) {
	for _, t := range reg.All() {
		sw, ok := t.(tool.Switch)
		if ok && sw.Status().On {
			_ = sw.Stop()
		}
	}
}

func arguments(req *mcpsdk.CallToolRequest) tool.Input {
	if req == nil || req.Params == nil {
		return tool.Input{}
	}
	raw, err := json.Marshal(req.Params.Arguments)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return tool.Input{}
	}
	vals := map[string]any{}
	if err := json.Unmarshal(raw, &vals); err != nil {
		return tool.Input{}
	}
	in := tool.Input{}
	for k, v := range vals {
		in[k] = fmt.Sprint(v)
	}
	return in
}

func planValue(p tool.Plan) map[string]any {
	changes := make([]map[string]any, len(p.Rows))
	for i, r := range p.Rows {
		changes[i] = map[string]any{
			"title": r.Title, "detail": r.Detail, "value": r.Value, "tone": tool.ToneName(r.Tone),
		}
	}
	return map[string]any{
		"summary": p.Summary, "changes": changes,
		"apply_label": p.ApplyLabel, "destructive": p.Destructive,
	}
}

func statusValue(st tool.Status) map[string]any {
	return map[string]any{"on": st.On, "text": st.Text, "detail": st.Detail}
}

func structured(v any) *mcpsdk.CallToolResult {
	b, _ := json.Marshal(v)
	return &mcpsdk.CallToolResult{
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: string(b)}},
		StructuredContent: v,
	}
}

func toolError(err error) *mcpsdk.CallToolResult {
	var problem *tool.Problem
	text := err.Error()
	if errors.As(err, &problem) && problem.Text != "" {
		text = problem.Text
	}
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: text}},
	}
}
