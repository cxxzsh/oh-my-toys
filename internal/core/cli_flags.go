package core

import (
	"strconv"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// flagPlan is the command-line name of the plan_id input, as
// docs/agents.md writes it: apply --plan <id>.
const flagPlan = "plan"

const propPlanID = "plan_id"

type usageError struct{ text string }

func (e *usageError) Error() string { return e.text }

func parseFlags(msgs i18n.Messages, args []string, spec tool.Spec) (tool.Input, error) {
	in := tool.Input{}
	for len(args) > 0 {
		name, value, rest, err := takeFlag(msgs, args)
		if err != nil {
			return nil, err
		}
		prop, ok := flagProp(spec, name)
		if !ok {
			return nil, &usageError{text: msgs.CliUnknownFlag(name)}
		}
		if err := checkValue(msgs, prop, value); err != nil {
			return nil, err
		}
		in[prop.Name] = value
		args = rest
	}
	for _, p := range spec.Input.Properties {
		if p.Required && strings.TrimSpace(in[p.Name]) == "" {
			return nil, &usageError{text: msgs.CliMissingFlag(flagName(p.Name))}
		}
	}
	return in, nil
}

func takeFlag(msgs i18n.Messages, args []string) (name, value string, rest []string, err error) {
	raw := args[0]
	if !strings.HasPrefix(raw, "--") || raw == "--" {
		return "", "", nil, &usageError{text: msgs.CliUnknownFlag(raw)}
	}
	name, value, hasValue := strings.Cut(strings.TrimPrefix(raw, "--"), "=")
	name = strings.ReplaceAll(name, "-", "_")
	if hasValue {
		return name, value, args[1:], nil
	}
	if len(args) > 1 && !strings.HasPrefix(args[1], "--") {
		return name, args[1], args[2:], nil
	}
	return name, "true", args[1:], nil
}

func flagProp(spec tool.Spec, name string) (tool.Property, bool) {
	if name == flagPlan {
		name = propPlanID
	}
	for _, p := range spec.Input.Properties {
		if p.Name == name {
			return p, true
		}
	}
	return tool.Property{}, false
}

func flagName(prop string) string {
	if prop == propPlanID {
		return flagPlan
	}
	return strings.ReplaceAll(prop, "_", "-")
}

func checkValue(msgs i18n.Messages, p tool.Property, value string) error {
	switch p.Schema.Type {
	case tool.TypeBoolean:
		if value != "true" && value != "false" {
			return &usageError{text: msgs.CliBadValue(flagName(p.Name), value)}
		}
	case tool.TypeInteger, tool.TypeNumber:
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || outOfBounds(p.Schema, n) {
			return &usageError{text: msgs.CliBadValue(flagName(p.Name), value)}
		}
	case tool.TypeString:
		if len(p.Schema.Enum) > 0 && !contains(p.Schema.Enum, value) {
			return &usageError{text: msgs.CliBadValue(flagName(p.Name), value)}
		}
	default:
		return nil
	}
	return nil
}

func outOfBounds(s tool.Schema, n float64) bool {
	if s.Minimum != nil && n < *s.Minimum {
		return true
	}
	return s.Maximum != nil && n > *s.Maximum
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func (c CLI) listTools() {
	if c.JSON {
		writeJSON(c.Out, toolList(c.Registry.All(), c.Msgs))
		return
	}
	writef(c.Out, "%s\n", c.Msgs.CliUsage())
	for _, t := range c.Registry.All() {
		m := t.Manifest()
		writef(c.Out, "  %s\t%s\n", m.ID, m.Name(c.Msgs))
	}
}

func (c CLI) listActions(t tool.Tool) {
	m := t.Manifest()
	if c.JSON {
		writeJSON(c.Out, oneTool(t, c.Msgs))
		return
	}
	writef(c.Out, "%s\t%s\n", m.ID, m.Summary(c.Msgs))
	for _, s := range t.Specs() {
		writef(c.Out, "  %s\t%s\n", s.Name, s.Summary(c.Msgs))
		for _, p := range s.Input.Properties {
			writef(c.Out, "    --%s\n", flagName(p.Name))
		}
	}
}

type listedTool struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

func toolList(tools []tool.Tool, msgs i18n.Messages) map[string]any {
	out := make([]listedTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, oneTool(t, msgs))
	}
	return map[string]any{"tools": out}
}

func oneTool(t tool.Tool, msgs i18n.Messages) listedTool {
	m := t.Manifest()
	names := make([]string, 0, len(t.Specs()))
	for _, s := range t.Specs() {
		names = append(names, s.Name)
	}
	return listedTool{ID: m.ID, Name: m.Name(msgs), Actions: names}
}
