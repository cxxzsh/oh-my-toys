package shell

import (
	"slices"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// TestSavedPresetStartsItsLengthAndGoes: a length typed in is saved as a
// preset, shown after the tool's own, started by one click, and removed
// by its cross; the settings follow each step.
func TestSavedPresetStartsItsLengthAndGoes(t *testing.T) {
	m := &modeSwitch{}
	a, tt := modeSwitchPage(t, m)
	typeInto(t, tt, i18n.SwitchHours(), "1")
	typeInto(t, tt, i18n.SwitchMinutes(), "30")
	if err := tt.Click(i18n.SwitchSavePreset()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	want := []time.Duration{90 * time.Minute}
	if got := a.settings.Presets(modeToolID); !slices.Equal(got, want) {
		t.Fatalf("saved %v, want %v", got, want)
	}
	name := i18n.SwitchLengthBoth(1, 30)
	if err := tt.Click(name); err != nil {
		t.Fatalf("no tile for the saved length: %v: %q", err, tt.Texts())
	}
	if !slices.Equal(m.lengths, want) {
		t.Errorf("the tile started %v, want %v", m.lengths, want)
	}

	if err := tt.Click(removeSpan); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := a.settings.Presets(modeToolID); len(got) != 0 || tt.HasText(name) {
		t.Errorf("after remove: saved %v, tile shown %v", got, tt.HasText(name))
	}
}

// TestSavedPresetNeedsALengthTheToolLacks: a length the tool has a mode for
// is not saved a second time, and nothing is saved from an empty form or a
// time of day.
func TestSavedPresetNeedsALengthTheToolLacks(t *testing.T) {
	m := &modeSwitch{}
	a, tt := modeSwitchPage(t, m)
	_ = tt.Click(i18n.SwitchSavePreset())
	typeInto(t, tt, i18n.SwitchMinutes(), "30") // the length of the fake's own mode
	_ = tt.Click(i18n.SwitchSavePreset())
	if got := a.settings.Presets(modeToolID); len(got) != 0 {
		t.Fatalf("saved %v from an empty form and from a length the tool has", got)
	}

	if err := tt.Click(i18n.SwitchModeClock()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText(i18n.SwitchSavePreset()) {
		t.Error("a time of day can be saved as a preset")
	}
}

// TestTrayShowsTwoSavedPresets: the tray has room for the modes it names and
// the first two lengths a person saved, and runs one for its length.
func TestTrayShowsTwoSavedPresets(t *testing.T) {
	m := &modeSwitch{}
	a, _ := modeSwitchPage(t, m)
	for _, d := range []time.Duration{45 * time.Minute, 3 * time.Hour, 5 * time.Hour} {
		a.settings.AddPreset(modeToolID, d)
	}
	tt := trayTester(a)
	for d, shown := range map[time.Duration]bool{45 * time.Minute: true, 3 * time.Hour: true, 5 * time.Hour: false} {
		if got := tt.HasText(lengthName(d)); got != shown {
			t.Errorf("%v on the tray: %v, want %v", d, got, shown)
		}
	}
	if err := tt.Click(lengthName(45 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{45 * time.Minute}; !slices.Equal(m.lengths, want) {
		t.Errorf("started %v, want %v", m.lengths, want)
	}
}

// TestRunsForNamesOnlyAPersonsOwnLength: the saved tile is marked while a
// run of its length is in progress, but not while the tool's own mode runs
// for it, nor the plan.
func TestRunsForNamesOnlyAPersonsOwnLength(t *testing.T) {
	m := &modeSwitch{}
	now := time.Now()
	run := func(active string, length time.Duration) tool.Status {
		return tool.Status{On: true, Active: active, Since: now, Until: now.Add(length)}
	}
	for _, c := range []struct {
		name string
		st   tool.Status
		want bool
	}{
		{"typed length", run("custom", 90*time.Minute), true},
		{"another length", run("custom", time.Hour), false},
		{"a mode of the tool", run("short", 90*time.Minute), false},
		{"the plan", run(tool.ActivePlan, 90*time.Minute), false},
		{"no end", tool.Status{On: true, Active: "custom", Since: now}, false},
	} {
		if got := runsFor(c.st, m, 90*time.Minute); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
