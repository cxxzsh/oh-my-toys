package main

import (
	"encoding/json"
	"testing"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/core"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

// TestDeclarations checks every tool the app registers, the made-up ones
// of -dev too: each action is valid, renders as JSON Schema in both
// languages, and action and switch tools have the actions they must.
func TestDeclarations(t *testing.T) {
	for _, tl := range tools(&core.Events{}, true) {
		id := tl.Manifest().ID
		names := map[string]tool.Spec{}
		for _, s := range tl.Specs() {
			if err := core.Validate(s); err != nil {
				t.Errorf("%s: %v", id, err)
			}
			if _, dup := names[s.Name]; dup {
				t.Errorf("%s: action %q twice", id, s.Name)
			}
			names[s.Name] = s
			for _, lang := range core.Languages() {
				if _, err := json.Marshal(core.JSONSchema(s.Input, i18n.In(lang))); err != nil {
					t.Errorf("%s %s: %v", id, s.Name, err)
				}
				if s.Summary(i18n.In(lang)) == "" {
					t.Errorf("%s %s: no %s summary", id, s.Name, lang)
				}
			}
		}
		checkKind(t, tl, names)
	}
}

func checkKind(t *testing.T, tl tool.Tool, names map[string]tool.Spec) {
	t.Helper()
	id := tl.Manifest().ID
	switch tl.(type) {
	case tool.Switch:
		for _, n := range []string{tool.ActionStart, tool.ActionStop, tool.ActionStatus} {
			if _, ok := names[n]; !ok {
				t.Errorf("%s: switch tool without %q", id, n)
			}
		}
	case tool.Action:
		plan, ok := names[tool.ActionPlan]
		if !ok || plan.Risk != tool.RiskRead || !plan.Idempotent {
			t.Errorf("%s: a plan must exist, read only and idempotent: %+v", id, plan)
		}
		if apply, ok := names[tool.ActionApply]; ok && (apply.Risk == tool.RiskRead || apply.Idempotent) {
			t.Errorf("%s: an apply changes something and is not idempotent: %+v", id, apply)
		}
	}
}

// TestRisks pins the risk of each real tool's actions to what it does:
// Color only reads, Ports ends processes, Rename and Find Duplicates
// change files, Keep Awake changes whether the system sleeps.
func TestRisks(t *testing.T) {
	want := map[string]map[string]tool.Risk{
		"awake":  {"start": tool.RiskSystem, "stop": tool.RiskSystem, "status": tool.RiskRead},
		"color":  {"plan": tool.RiskRead},
		"ports":  {"plan": tool.RiskRead, "apply": tool.RiskProcess},
		"rename": {"plan": tool.RiskRead, "apply": tool.RiskWrite},
		"dupes":  {"plan": tool.RiskRead, "apply": tool.RiskWrite},
	}
	for _, tl := range tools(&core.Events{}, false) {
		id := tl.Manifest().ID
		got := map[string]tool.Risk{}
		for _, s := range tl.Specs() {
			got[s.Name] = s.Risk
		}
		if len(got) != len(want[id]) {
			t.Errorf("%s: actions %v, want %v", id, got, want[id])
		}
		for name, risk := range want[id] {
			if got[name] != risk {
				t.Errorf("%s %s: risk %q, want %q", id, name, got[name], risk)
			}
		}
	}
}
