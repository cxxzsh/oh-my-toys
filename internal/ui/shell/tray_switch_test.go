package shell

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// trayHeight is room enough for the tray panel with a switch tool in it.
const trayHeight = 600

func modeSwitchTray(t *testing.T, m *modeSwitch) *ui.Tester {
	t.Helper()
	a, _ := modeSwitchPage(t, m)
	return trayTester(a)
}

// trayTester shows the tray panel of an app.
func trayTester(a *App) *ui.Tester {
	tt := ui.NewTester(a.trayView, ds.TrayWidth, trayHeight)
	tt.Frame()
	return tt
}

// TestTrayChipsStartTheirModeAtOnce: the modes are on the tray whether the
// tool is off or on, and one click starts that mode: a person need not turn
// the tool on first and then change it.
func TestTrayChipsStartTheirModeAtOnce(t *testing.T) {
	for _, st := range []tool.Status{{}, {On: true, Active: "short", Since: time.Now()}} {
		m := &modeSwitch{st: st}
		tt := modeSwitchTray(t, m)
		if err := tt.Click(modeLongChip); err != nil {
			t.Fatalf("on=%v: %v: %q", st.On, err, tt.Texts())
		}
		if want := []string{"long"}; !slices.Equal(m.ran, want) {
			t.Errorf("on=%v: ran %v, want %v", st.On, m.ran, want)
		}
	}
}

// TestTrayChipsAreOnlyTheModesWithAShortName: the way to turn the tool off
// is its switch, and a mode with no short name is not a chip.
func TestTrayChipsAreOnlyTheModesWithAShortName(t *testing.T) {
	tt := modeSwitchTray(t, &modeSwitch{st: tool.Status{On: true, Active: "short", Since: time.Now()}})
	for _, title := range []string{modeShortTitle, modeLongTitle, modeStopTitle} {
		if tt.HasText(title) {
			t.Errorf("the long name %q is on the tray: %q", title, tt.Texts())
		}
	}
	for _, chip := range []string{modeShortChip, modeLongChip, i18n.SwitchCustom() + "…"} {
		if !tt.HasText(chip) {
			t.Errorf("no %q chip: %q", chip, tt.Texts())
		}
	}
}

// TestTrayExtendAddsToWhatIsLeft: extending a run that ends by itself
// starts one for what is left and a little more; a run with no end has
// nothing to extend.
func TestTrayExtendAddsToWhatIsLeft(t *testing.T) {
	label := i18n.SwitchExtend(int(extendBy / time.Minute))
	now := time.Now()
	m := &modeSwitch{st: tool.Status{On: true, Active: "short", Since: now, Until: now.Add(10 * time.Minute)}}
	tt := modeSwitchTray(t, m)
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	if len(m.lengths) != 1 || m.lengths[0] < 39*time.Minute || m.lengths[0] > 40*time.Minute {
		t.Errorf("extended for %v, want about 10m left + %v", m.lengths, extendBy)
	}

	tt = modeSwitchTray(t, &modeSwitch{st: tool.Status{On: true, Active: "long", Since: now}})
	if tt.HasText(label) {
		t.Error("a run with no end has an extend button")
	}
}

// TestExtendNeverRunsForever: the time left may have run out between the
// frame and the click, and a length of nothing means until turned off.
func TestExtendNeverRunsForever(t *testing.T) {
	m := &modeSwitch{st: tool.Status{On: true, Since: time.Now().Add(-time.Hour), Until: time.Now().Add(-time.Minute)}}
	a, _ := modeSwitchPage(t, m)
	a.extend(m)
	if len(m.lengths) != 1 || m.lengths[0] != extendBy {
		t.Errorf("extended for %v, want %v", m.lengths, extendBy)
	}
}

// TestTrayShowsWhenThePlanNextTurnsTheToolOn: a tool with a plan that is on
// says when it next starts, and says nothing while the plan holds it.
func TestTrayShowsWhenThePlanNextTurnsTheToolOn(t *testing.T) {
	now := time.Now()
	always := tool.Segment{Days: tool.AllDays, From: 0, To: 12 * 60}
	m := &modeSwitch{sched: tool.Schedule{Enabled: true, Segments: []tool.Segment{always}}}
	tt := modeSwitchTray(t, m)
	prefix := strings.SplitN(i18n.SwitchNextRun("\x00", ""), "\x00", 2)[0]
	if !slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.HasPrefix(s, prefix) }) {
		t.Errorf("no next-run line on a plan that is on: %q", tt.Texts())
	}

	m = &modeSwitch{sched: m.sched, st: tool.Status{On: true, Active: tool.ActivePlan, Since: now}}
	tt = modeSwitchTray(t, m)
	if slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.HasPrefix(s, prefix) }) {
		t.Errorf("a next-run line while the plan holds the tool on: %q", tt.Texts())
	}
}

// TestSwitchPageExtendsAndStopsFromTheStatusCard: a run in progress can be
// given more time, or ended, from the card that shows it.
func TestSwitchPageExtendsAndStopsFromTheStatusCard(t *testing.T) {
	now := time.Now()
	m := &modeSwitch{st: tool.Status{On: true, Active: "short", Since: now, Until: now.Add(time.Hour)}}
	_, tt := modeSwitchPage(t, m)
	if err := tt.Click(i18n.SwitchExtend(int(extendBy / time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(modeStopTitle); err != nil {
		t.Fatal(err)
	}
	if len(m.lengths) != 1 || m.lengths[0] < 89*time.Minute || m.lengths[0] > 90*time.Minute {
		t.Errorf("extended for %v, want about 1h left + %v", m.lengths, extendBy)
	}
	if want := []string{"off"}; !slices.Equal(m.ran, want) {
		t.Errorf("ran %v, want %v", m.ran, want)
	}
}

// TestDayCellsMarkTheTimeNow: a day's bar is the stretches the plan holds
// and those it does not, covers the whole day, and shows now in its place.
func TestDayCellsMarkTheTimeNow(t *testing.T) {
	cover := []tool.Range{{From: 9 * 60, To: 12 * 60}, {From: 22 * 60, To: tool.MinutesPerDay}}
	sum := func(cells []cell) (total int) {
		for _, c := range cells {
			total += c.minutes
		}
		return total
	}
	plain := dayCells(cover, notToday)
	want := []cell{{9 * 60, cellOff}, {3 * 60, cellOn}, {10 * 60, cellOff}, {2 * 60, cellOn}}
	if !slices.Equal(plain, want) {
		t.Fatalf("cells %v, want %v", plain, want)
	}
	for _, now := range []int{0, 9 * 60, 10 * 60, 12 * 60, tool.MinutesPerDay - 1} {
		cells := dayCells(cover, now)
		if sum(cells) != tool.MinutesPerDay {
			t.Errorf("now %d: the bar covers %d minutes, want a day", now, sum(cells))
		}
		before, marks := 0, 0
		for _, c := range cells {
			if c.kind == cellNow {
				marks++
				break
			}
			before += c.minutes
		}
		if marks != 1 || before != now {
			t.Errorf("now %d: mark after %d minutes (%d marks)", now, before, marks)
		}
	}
}

// TestPlanHoldIsNotExtended: a stretch the plan holds has the plan's end, so
// there is nothing to extend, while a person's run inside it can be; either
// way the other hold is said, on the page and on the tray.
func TestPlanHoldIsNotExtended(t *testing.T) {
	const note = "the plan also holds it until 18:00"
	label := i18n.SwitchExtend(int(extendBy / time.Minute))
	now := time.Now()
	byPlan := tool.Status{On: true, Active: tool.ActivePlan, Since: now, Until: now.Add(time.Hour)}
	byHand := tool.Status{On: true, Active: "short", Since: now, Until: now.Add(time.Hour), Note: note}
	for _, c := range []struct {
		name   string
		st     tool.Status
		extend bool
		note   bool
	}{{"plan only", byPlan, false, false}, {"hand and plan", byHand, true, true}} {
		_, page := modeSwitchPage(t, &modeSwitch{st: c.st})
		tray := modeSwitchTray(t, &modeSwitch{st: c.st})
		for where, tt := range map[string]*ui.Tester{"page": page, "tray": tray} {
			if got := tt.HasText(label); got != c.extend {
				t.Errorf("%s, %s: extend button %v, want %v", c.name, where, got, c.extend)
			}
			if got := tt.HasText(note); got != c.note {
				t.Errorf("%s, %s: note shown %v, want %v", c.name, where, got, c.note)
			}
		}
	}
}
