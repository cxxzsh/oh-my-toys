package core

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

// actionName is the form of an action's or a property's name: lower-case
// English words joined by underscores, as MCP tool names and JSON keys use.
var actionName = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// errInvalidSpec is a declaration that breaks the rules of docs/agents.md.
var errInvalidSpec = errors.New("invalid action declaration")

// JSONSchema writes a schema as JSON Schema, its texts in a language: the
// inputSchema and outputSchema of the MCP tools, and the shape of --json.
func JSONSchema(s tool.Schema, msgs i18n.Messages) map[string]any {
	out := map[string]any{"type": string(s.Type)}
	put := func(key, v string) {
		if v != "" {
			out[key] = v
		}
	}
	put("format", s.Format)
	put("title", say(s.Title, msgs))
	put("description", say(s.Description, msgs))
	if hint := say(s.Hint, msgs); hint != "" {
		out["examples"] = []string{hint}
	}
	if len(s.Enum) > 0 {
		out["enum"] = s.Enum
	}
	if s.Minimum != nil {
		out["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		out["maximum"] = *s.Maximum
	}
	if s.Items != nil {
		out["items"] = JSONSchema(*s.Items, msgs)
	}
	if s.Type == tool.TypeObject {
		props, required := objectSchema(s, msgs)
		out["properties"], out["additionalProperties"] = props, false
		if len(required) > 0 {
			out["required"] = required
		}
	}
	return out
}

func objectSchema(s tool.Schema, msgs i18n.Messages) (map[string]any, []string) {
	props, required := map[string]any{}, []string{}
	for _, p := range s.Properties {
		props[p.Name] = JSONSchema(p.Schema, msgs)
		if p.Required {
			required = append(required, p.Name)
		}
	}
	return props, required
}

// Validate checks an action's declaration: a name, a summary, a known
// risk, object input and output, and schemas of the subset.
func Validate(s tool.Spec) error {
	switch {
	case !actionName.MatchString(s.Name):
		return fmt.Errorf("action %q: name: %w", s.Name, errInvalidSpec)
	case s.Summary == nil:
		return fmt.Errorf("action %q: no summary: %w", s.Name, errInvalidSpec)
	case !slices.Contains(tool.Risks(), s.Risk):
		return fmt.Errorf("action %q: risk %q: %w", s.Name, s.Risk, errInvalidSpec)
	case s.Input.Type != tool.TypeObject || s.Output.Type != tool.TypeObject:
		return fmt.Errorf("action %q: input and output must be objects: %w", s.Name, errInvalidSpec)
	}
	if err := validSchema(s.Input); err != nil {
		return fmt.Errorf("action %q input: %w", s.Name, err)
	}
	if err := validSchema(s.Output); err != nil {
		return fmt.Errorf("action %q output: %w", s.Name, err)
	}
	return nil
}

func validSchema(s tool.Schema) error {
	switch s.Type {
	case tool.TypeObject:
		return validProperties(s.Properties)
	case tool.TypeArray:
		if s.Items == nil {
			return fmt.Errorf("an array without items: %w", errInvalidSpec)
		}
		return validSchema(*s.Items)
	case tool.TypeString, tool.TypeBoolean:
	case tool.TypeInteger, tool.TypeNumber:
		if s.Minimum != nil && s.Maximum != nil && *s.Minimum > *s.Maximum {
			return fmt.Errorf("minimum above maximum: %w", errInvalidSpec)
		}
	default:
		return fmt.Errorf("type %q: %w", s.Type, errInvalidSpec)
	}
	if s.Format != "" && (s.Format != tool.FormatPath || s.Type != tool.TypeString) {
		return fmt.Errorf("format %q on %s: %w", s.Format, s.Type, errInvalidSpec)
	}
	if len(s.Enum) > 0 && s.Type != tool.TypeString {
		return fmt.Errorf("an enum of %s: %w", s.Type, errInvalidSpec)
	}
	return nil
}

func validProperties(props []tool.Property) error {
	seen := map[string]bool{}
	for _, p := range props {
		if !actionName.MatchString(p.Name) || seen[p.Name] {
			return fmt.Errorf("property %q: %w", p.Name, errInvalidSpec)
		}
		seen[p.Name] = true
		if p.Schema.Title == nil {
			return fmt.Errorf("property %q has no title: %w", p.Name, errInvalidSpec)
		}
		if err := validSchema(p.Schema); err != nil {
			return fmt.Errorf("property %q: %w", p.Name, err)
		}
	}
	return nil
}
