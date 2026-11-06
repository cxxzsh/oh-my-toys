package core

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func title(m i18n.Messages) string { return m.FieldFolder() }

func validSpec() tool.Spec {
	folder := tool.Schema{Type: tool.TypeString, Format: tool.FormatPath, Title: title}
	return tool.PlanSpec(title, tool.Schema{Type: tool.TypeObject, Properties: []tool.Property{
		{Name: "folder", Schema: folder, Required: true},
		{Name: "port", Schema: tool.Schema{Type: tool.TypeInteger, Title: title, Minimum: tool.Bounds(1), Maximum: tool.Bounds(9)}},
		{Name: "mode", Schema: tool.Schema{Type: tool.TypeString, Title: title, Enum: []string{"a", "b"}}},
		{Name: "dry_run", Schema: tool.Schema{Type: tool.TypeBoolean, Title: title}},
	}})
}

func TestValidate(t *testing.T) {
	if err := Validate(validSpec()); err != nil {
		t.Fatalf("valid spec: %v", err)
	}
	broken := map[string]func(*tool.Spec){
		"name":         func(s *tool.Spec) { s.Name = "Plan It" },
		"summary":      func(s *tool.Spec) { s.Summary = nil },
		"risk":         func(s *tool.Spec) { s.Risk = "dangerous" },
		"input type":   func(s *tool.Spec) { s.Input.Type = tool.TypeString },
		"bounds":       func(s *tool.Spec) { s.Input.Properties[1].Schema.Minimum = tool.Bounds(10) },
		"format":       func(s *tool.Spec) { s.Input.Properties[1].Schema.Format = tool.FormatPath },
		"enum":         func(s *tool.Spec) { s.Input.Properties[3].Schema.Enum = []string{"x"} },
		"duplicate":    func(s *tool.Spec) { s.Input.Properties[1].Name = "folder" },
		"no title":     func(s *tool.Spec) { s.Input.Properties[0].Schema.Title = nil },
		"no items":     func(s *tool.Spec) { s.Output.Properties[1].Schema.Items = nil },
		"unknown type": func(s *tool.Spec) { s.Input.Properties[0].Schema.Type = "date" },
	}
	for name, breakIt := range broken {
		s := validSpec()
		s.Input.Properties = slices.Clone(s.Input.Properties)
		s.Output.Properties = slices.Clone(s.Output.Properties)
		breakIt(&s)
		if err := Validate(s); !errors.Is(err, errInvalidSpec) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestJSONSchema(t *testing.T) {
	js := JSONSchema(validSpec().Input, i18n.In("en"))
	b, err := json.Marshal(js)
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Type       string                                   `json:"type"`
		Required   []string                                 `json:"required"`
		Properties map[string]struct{ Type, Format string } `json:"properties"`
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Type != "object" || !slices.Equal(back.Required, []string{"folder"}) {
		t.Errorf("schema %s", b)
	}
	if p := back.Properties["folder"]; p.Type != "string" || p.Format != "path" {
		t.Errorf("folder %+v", p)
	}
}

func TestForm(t *testing.T) {
	fs := Form(validSpec(), i18n.In("zh-Hans"))
	kinds := make([]FieldKind, 0, len(fs))
	for _, f := range fs {
		kinds = append(kinds, f.Kind)
	}
	if want := []FieldKind{FieldPath, FieldNumber, FieldChoice, FieldToggle}; !slices.Equal(kinds, want) {
		t.Errorf("kinds %v, want %v", kinds, want)
	}
	if fs[0].Label != "文件夹" || !fs[0].Required || fs[1].Required {
		t.Errorf("fields %+v", fs)
	}
}
