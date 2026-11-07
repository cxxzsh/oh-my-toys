package shell

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

const (
	modeShortTitle = "Short run"
	modeLongTitle  = "Long run"
	modeStopTitle  = "Stop it"
	modeToolID     = "mode-switch"
)

// modeSwitch is a switch tool with two modes and a quick action that stops
// it. The test sets its status, and it records which actions ran.
type modeSwitch struct {
	st      tool.Status
	ran     []string
	lengths []time.Duration // what StartFor was asked
}

func titled(s string) tool.Message { return func(i18n.Messages) string { return s } }

func (m *modeSwitch) Manifest() tool.Manifest {
	act := func(id, title string, stops bool) tool.QuickAction {
		return tool.QuickAction{ID: id, Title: titled(title), Stops: stops, Run: func(context.Context) error {
			m.ran = append(m.ran, id)
			return nil
		}}
	}
	return tool.Manifest{
		ID: modeToolID, Name: titled("Mode switch"), Summary: titled("A switch with modes"),
		Kind: tool.KindSwitch, Icon: "bedtime_off", Category: tool.CategorySystem,
		Actions: []tool.QuickAction{act("short", modeShortTitle, false), act("long", modeLongTitle, false), act("off", modeStopTitle, true)},
	}
}

func (m *modeSwitch) Specs() []tool.Spec {
	s := titled("x")
	return tool.TimedSwitchSpecs(s, s, s)
}

func (m *modeSwitch) StartFor(_ context.Context, d time.Duration) error {
	m.lengths = append(m.lengths, d)
	return nil
}

func (m *modeSwitch) Start(context.Context) error { return nil }
func (m *modeSwitch) Stop() error                 { return nil }
func (m *modeSwitch) Status() tool.Status         { return m.st }

// modeSwitchPage opens the switch tool's page in a window of its own.
func modeSwitchPage(t *testing.T, m *modeSwitch) *ui.Tester {
	t.Helper()
	a := newTestApp(t, false)
	reg, err := core.NewRegistry(m)
	if err != nil {
		t.Fatal(err)
	}
	a.reg = reg
	a.open(modeToolID)
	tt := ui.NewTester(a.mainView, ds.WindowWidth, ds.WindowHeight)
	tt.Frame()
	return tt
}

// TestSwitchPageShowsTheRunByItsState checks what each state of a switch
// tool puts on its page: the way to turn it off only while it runs, and the
// end of a run only when it has one.
func TestSwitchPageShowsTheRunByItsState(t *testing.T) {
	now := time.Now()
	ends := now.Add(90 * time.Minute)
	cases := []struct {
		name     string
		st       tool.Status
		want     []string
		wantNone []string
	}{
		{"off", tool.Status{}, []string{modeShortTitle, modeLongTitle, i18n.SwitchPickPreset()}, []string{modeStopTitle}},
		{
			"timed",
			tool.Status{On: true, Active: "short", Since: now, Until: ends},
			[]string{modeStopTitle, ends.Format(time.TimeOnly), i18n.SwitchTimeLeft()},
			[]string{i18n.SwitchPickPreset()},
		},
		{
			"until turned off",
			tool.Status{On: true, Active: "long", Since: now},
			[]string{modeStopTitle, "∞", i18n.SwitchUntilOff()},
			[]string{i18n.SwitchTimeLeft(), i18n.SwitchPickPreset()},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tt := modeSwitchPage(t, &modeSwitch{st: c.st})
			for _, s := range c.want {
				if !tt.HasText(s) {
					t.Errorf("page lacks %q; it has %q", s, tt.Texts())
				}
			}
			for _, s := range c.wantNone {
				if tt.HasText(s) {
					t.Errorf("page has %q", s)
				}
			}
		})
	}
}

// TestSwitchPagePresetsRunTheirActions clicks a mode, then the stop button:
// each runs its own quick action and no other.
func TestSwitchPagePresetsRunTheirActions(t *testing.T) {
	m := &modeSwitch{st: tool.Status{On: true, Active: "short", Since: time.Now()}}
	tt := modeSwitchPage(t, m)
	for _, title := range []string{modeLongTitle, modeStopTitle} {
		if err := tt.Click(title); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"long", "off"}; !slices.Equal(m.ran, want) {
		t.Errorf("ran %v, want %v", m.ran, want)
	}
}

// typeInto clicks the input under a field's label and types into it.
func typeInto(t *testing.T, tt *ui.Tester, label, text string) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no %q on the page: %q", label, tt.Texts())
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H+ds.Space4+ds.ControlLg/2) // the input is under its label
	tt.Type(text)
	tt.Frame()
}

func customTab(t *testing.T, tt *ui.Tester) {
	t.Helper()
	if err := tt.Click(i18n.SwitchCustom()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

// TestSwitchCustomLengthStartsForThatLong types hours and minutes, which
// become one length, started by the button or by Enter.
func TestSwitchCustomLengthStartsForThatLong(t *testing.T) {
	m := &modeSwitch{}
	tt := modeSwitchPage(t, m)
	customTab(t, tt)
	if !tt.HasText(i18n.SwitchCustomHint()) {
		t.Errorf("nothing typed yet, but no hint: %q", tt.Texts())
	}
	typeInto(t, tt, i18n.SwitchHours(), "1")
	typeInto(t, tt, i18n.SwitchMinutes(), "30")
	if err := tt.Click(i18n.SwitchStart()); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter) // the minutes field still has the focus
	tt.Frame()
	want := []time.Duration{90 * time.Minute, 90 * time.Minute}
	if !slices.Equal(m.lengths, want) {
		t.Errorf("started for %v, want %v: once by the button, once by Enter", m.lengths, want)
	}
}

// TestSwitchCustomRefusesWhatIsNotALength: a mistake shows its fix and the
// button does nothing, but an empty form is not a mistake yet.
func TestSwitchCustomRefusesWhatIsNotALength(t *testing.T) {
	for _, bad := range []string{"x", "1.5", "-1", "100"} {
		t.Run(bad, func(t *testing.T) {
			m := &modeSwitch{}
			tt := modeSwitchPage(t, m)
			customTab(t, tt)
			typeInto(t, tt, i18n.SwitchHours(), bad)
			if !tt.HasText(i18n.SwitchBadLength()) {
				t.Errorf("hours %q: no fix shown: %q", bad, tt.Texts())
			}
			_ = tt.Click(i18n.SwitchStart())
			if len(m.lengths) != 0 {
				t.Errorf("hours %q: started for %v", bad, m.lengths)
			}
		})
	}
	m := &modeSwitch{}
	tt := modeSwitchPage(t, m)
	customTab(t, tt)
	typeInto(t, tt, i18n.SwitchMinutes(), "0")
	_ = tt.Click(i18n.SwitchStart())
	if len(m.lengths) != 0 || !tt.HasText(i18n.SwitchBadLength()) {
		t.Errorf("zero minutes: started for %v, texts %q", m.lengths, tt.Texts())
	}
}

// TestSwitchCustomTimeOfDay switches to a time of day: a bad one shows the
// fix, a good one starts for the time left until it.
func TestSwitchCustomTimeOfDay(t *testing.T) {
	m := &modeSwitch{}
	tt := modeSwitchPage(t, m)
	customTab(t, tt)
	if err := tt.Click(i18n.SwitchModeClock()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	typeInto(t, tt, i18n.SwitchClock(), "tea")
	if !tt.HasText(i18n.HoldClock()) {
		t.Fatalf("no fix for a bad time of day: %q", tt.Texts())
	}
	_ = tt.Click(i18n.SwitchStart())
	if len(m.lengths) != 0 {
		t.Fatalf("started for %v with a bad time of day", m.lengths)
	}

	m = &modeSwitch{}
	tt = modeSwitchPage(t, m)
	customTab(t, tt)
	_ = tt.Click(i18n.SwitchModeClock())
	tt.Frame()
	typeInto(t, tt, i18n.SwitchClock(), "23:59")
	if err := tt.Click(i18n.SwitchStart()); err != nil {
		t.Fatal(err)
	}
	if len(m.lengths) != 1 || m.lengths[0] <= 0 || m.lengths[0] > 24*time.Hour {
		t.Errorf("started for %v: want one length within the next day", m.lengths)
	}
}
