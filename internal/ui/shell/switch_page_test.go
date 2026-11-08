package shell

import (
	"context"
	"slices"
	"strings"
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
	sched   tool.Schedule   // the plan; SetSchedule keeps what it is given
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

func (m *modeSwitch) Schedule() tool.Schedule     { return m.sched }
func (m *modeSwitch) SetSchedule(s tool.Schedule) { m.sched = s.Clean() }

func (m *modeSwitch) StartFor(_ context.Context, d time.Duration) error {
	m.lengths = append(m.lengths, d)
	return nil
}

func (m *modeSwitch) Start(context.Context) error { return nil }
func (m *modeSwitch) Stop() error                 { return nil }
func (m *modeSwitch) Status() tool.Status         { return m.st }

// modeSwitchPage opens the switch tool's page in a window of its own.
func modeSwitchPage(t *testing.T, m *modeSwitch) (*App, *ui.Tester) {
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
	return a, tt
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
			_, tt := modeSwitchPage(t, &modeSwitch{st: c.st})
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
	_, tt := modeSwitchPage(t, m)
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
	_, tt := modeSwitchPage(t, m)
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
			_, tt := modeSwitchPage(t, m)
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
	_, tt := modeSwitchPage(t, m)
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
	_, tt := modeSwitchPage(t, m)
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
	_, tt = modeSwitchPage(t, m)
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

func planTabOf(t *testing.T, tt *ui.Tester) {
	t.Helper()
	if err := tt.Click(i18n.SwitchPlan()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

func workSpan() tool.Segment {
	return tool.Segment{Days: tool.Workdays, From: 9 * 60, To: 12 * 60}
}

// TestSwitchPlanShowsTheSpansOfTheTool: the rows are the plan the tool
// has, and the page says when it next turns on. The times typed in are not
// among the texts of a page, so the days and that line are what is checked.
func TestSwitchPlanShowsTheSpansOfTheTool(t *testing.T) {
	m := &modeSwitch{sched: tool.Schedule{Enabled: true, Segments: []tool.Segment{workSpan()}}}
	_, tt := modeSwitchPage(t, m)
	planTabOf(t, tt)
	next := strings.SplitN(i18n.SwitchNextRun("\x00", ""), "\x00", 2)[0]
	found := false
	for _, s := range tt.Texts() {
		found = found || strings.HasPrefix(s, next)
	}
	if !found {
		t.Errorf("no %q line on a plan that is on: %q", next, tt.Texts())
	}
	for _, want := range []string{i18n.WeekdayMon(), i18n.WeekdaySun()} {
		if !tt.HasText(want) {
			t.Errorf("no %q toggle on the plan", want)
		}
	}
	if tt.HasText(i18n.SwitchPlanEmpty()) {
		t.Error("an empty plan's hint is shown over a plan with a span")
	}
}

// TestSwitchPlanAddsASpanAndKeepsIt: a new span is Monday to Friday, 09:00
// to 18:00; the tool gets it and the settings keep it for the next launch.
func TestSwitchPlanAddsASpanAndKeepsIt(t *testing.T) {
	m := &modeSwitch{}
	a, tt := modeSwitchPage(t, m)
	planTabOf(t, tt)
	if !tt.HasText(i18n.SwitchPlanEmpty()) {
		t.Fatalf("no hint on an empty plan: %q", tt.Texts())
	}
	if err := tt.Click("+ " + i18n.SwitchAddSpan()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	want := []tool.Segment{{Days: tool.Workdays, From: 9 * 60, To: 18 * 60}}
	if !slices.Equal(m.sched.Segments, want) {
		t.Errorf("the tool's plan is %+v, want %+v", m.sched.Segments, want)
	}
	if got := a.settings.Schedules[modeToolID].Segments; !slices.Equal(got, want) {
		t.Errorf("kept plan is %+v, want %+v", got, want)
	}
	saved, err := a.opt.Store.Load(core.Settings{})
	if err != nil || !slices.Equal(saved.Schedules[modeToolID].Segments, want) {
		t.Errorf("plan on disk is %+v (%v): it must survive a restart", saved.Schedules[modeToolID], err)
	}
}

// TestSwitchPlanTakesManySpans: a week with a different plan each day needs
// more spans than a handful, and the button never stops adding them.
func TestSwitchPlanTakesManySpans(t *testing.T) {
	const spans = 20
	m := &modeSwitch{}
	_, tt := modeSwitchPage(t, m)
	planTabOf(t, tt)
	for range spans {
		if err := tt.Click("+ " + i18n.SwitchAddSpan()); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if got := len(m.sched.Segments); got != spans {
		t.Errorf("the plan has %d spans after adding %d", got, spans)
	}
}

// TestSwitchPlanHalfTypedTimeKeepsTheSpan: a time being edited is not a
// time yet, and until it is, the plan keeps the span as it was.
func TestSwitchPlanHalfTypedTimeKeepsTheSpan(t *testing.T) {
	m := &modeSwitch{sched: tool.Schedule{Enabled: true, Segments: []tool.Segment{workSpan()}}}
	_, tt := modeSwitchPage(t, m)
	planTabOf(t, tt)
	if err := tt.Click("09:00"); err != nil {
		t.Fatal(err)
	}
	tt.Type("9")
	tt.Frame()
	if !slices.Equal(m.sched.Segments, []tool.Segment{workSpan()}) {
		t.Errorf("plan %+v: a half typed time took the span out of the plan", m.sched.Segments)
	}
	if !tt.HasText(i18n.SwitchSpanBad()) {
		t.Errorf("no error for a time that does not read: %q", tt.Texts())
	}
}

func TestSwitchPlanTogglesADayAndRemovesASpan(t *testing.T) {
	m := &modeSwitch{sched: tool.Schedule{Enabled: true, Segments: []tool.Segment{workSpan()}}}
	_, tt := modeSwitchPage(t, m)
	planTabOf(t, tt)
	if err := tt.Click(i18n.WeekdaySat()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := m.sched.Segments; len(got) != 1 || !got[0].Days.Has(time.Saturday) || !got[0].Days.Has(time.Monday) {
		t.Fatalf("after Saturday: %+v", got)
	}
	if err := tt.Click(removeSpan); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(m.sched.Segments) != 0 {
		t.Errorf("after removing it: %+v", m.sched.Segments)
	}
	if !tt.HasText(i18n.SwitchPlanEmpty()) {
		t.Error("no hint once the last span is gone")
	}
}

func TestSwitchPlanSwitchTurnsThePlanOnAndOff(t *testing.T) {
	m := &modeSwitch{sched: tool.Schedule{Segments: []tool.Segment{workSpan()}}}
	_, tt := modeSwitchPage(t, m)
	planTabOf(t, tt)
	if err := tt.Click(i18n.SwitchPlanOn()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !m.sched.Enabled {
		t.Fatalf("the plan is still off: %+v", m.sched)
	}
	if err := tt.Click(i18n.SwitchPlanOn()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.sched.Enabled || len(m.sched.Segments) != 1 {
		t.Errorf("turning it off lost the spans: %+v", m.sched)
	}
}

// TestSavedPlansGoBackToTheirTools is the app opening: each tool that runs
// by a plan gets the one kept for it.
func TestSavedPlansGoBackToTheirTools(t *testing.T) {
	m := &modeSwitch{}
	a, _ := modeSwitchPage(t, m)
	a.settings.SetSchedule(modeToolID, tool.Schedule{Enabled: true, Segments: []tool.Segment{workSpan()}})
	a.applySchedules()
	if !m.sched.Enabled || !slices.Equal(m.sched.Segments, []tool.Segment{workSpan()}) {
		t.Errorf("the tool's plan is %+v", m.sched)
	}
}
