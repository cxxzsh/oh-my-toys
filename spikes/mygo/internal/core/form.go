package core

import (
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

// FieldKind says which control asks for a field.
type FieldKind int

// The kinds of fields a form has.
const (
	FieldText   FieldKind = iota // a line of text
	FieldPath                    // a folder or file, chosen in a dialog
	FieldNumber                  // a number, typed
	FieldToggle                  // on or off
	FieldChoice                  // one of a few values
)

// Field is one control of a form, in the language of the messages it was
// made with.
type Field struct {
	Name     string // the property of the input it fills
	Label    string
	Hint     string // the placeholder
	Kind     FieldKind
	Required bool
	Choices  []string
	Min, Max *float64
}

// Form describes the form that asks for an action's input: a field per
// property of its input schema, in order. The desktop app draws it; the
// command line will make its flags from the same schema.
func Form(spec tool.Spec, msgs i18n.Messages) []Field {
	fields := make([]Field, 0, len(spec.Input.Properties))
	for _, p := range spec.Input.Properties {
		fields = append(fields, Field{
			Name:     p.Name,
			Label:    say(p.Schema.Title, msgs),
			Hint:     say(p.Schema.Hint, msgs),
			Kind:     fieldKind(p.Schema),
			Required: p.Required,
			Choices:  p.Schema.Enum,
			Min:      p.Schema.Minimum,
			Max:      p.Schema.Maximum,
		})
	}
	return fields
}

func fieldKind(s tool.Schema) FieldKind {
	switch {
	case len(s.Enum) > 0:
		return FieldChoice
	case s.Type == tool.TypeString && s.Format == tool.FormatPath:
		return FieldPath
	case s.Type == tool.TypeInteger || s.Type == tool.TypeNumber:
		return FieldNumber
	case s.Type == tool.TypeBoolean:
		return FieldToggle
	default:
		return FieldText
	}
}

// say is a message in a language, or "" when there is none.
func say(m tool.Message, msgs i18n.Messages) string {
	if m == nil {
		return ""
	}
	return m(msgs)
}

// SpecOf returns a tool's action of a name, and whether it declares one.
func SpecOf(t tool.Tool, name string) (tool.Spec, bool) {
	for _, s := range t.Specs() {
		if s.Name == name {
			return s, true
		}
	}
	return tool.Spec{}, false
}
