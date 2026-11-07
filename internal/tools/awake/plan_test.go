package awake

import (
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// at is a time on Monday 9 March 2026 plus days.
func at(days, h, m int) time.Time {
	return time.Date(2026, time.March, 9+days, h, m, 0, 0, time.UTC)
}

// workdays is 09:00 to 12:00 on weekdays.
func workdays(display bool) tool.Schedule {
	return tool.Schedule{
		Enabled: true, Display: display,
		Segments: []tool.Segment{{Days: tool.Workdays, From: 9 * 60, To: 12 * 60}},
	}
}

// look sets the clock of the tool and lets the plan look at it, as the
// timer of the plan does each minute.
func look(tl *Tool, now time.Time) {
	tl.mu.Lock()
	tl.now = func() time.Time { return now }
	tl.reconcileLocked(now)
	tl.mu.Unlock()
}

func TestPlanHoldsInsideItsStretchOnly(t *testing.T) {
	tl, held, _ := newTestTool(t)
	tl.now = func() time.Time { return at(0, 8, 0) }
	tl.SetSchedule(workdays(true))
	if *held != 0 || tl.Status().On {
		t.Fatalf("held %d before the plan begins", *held)
	}

	look(tl, at(0, 9, 0))
	st := tl.Status()
	if *held != 1 || !st.On || st.Active != tool.ActivePlan {
		t.Fatalf("at 09:00: held %d, %+v", *held, st)
	}
	if !st.Since.Equal(at(0, 9, 0)) || !st.Until.Equal(at(0, 12, 0)) {
		t.Errorf("the stretch is %v to %v, want 09:00 to 12:00", st.Since, st.Until)
	}

	look(tl, at(0, 11, 59))
	if *held != 1 {
		t.Errorf("held %d a minute before the end: looking again must not take a second hold", *held)
	}
	look(tl, at(0, 12, 0))
	if *held != 0 || tl.Status().On {
		t.Errorf("at 12:00: held %d, %+v", *held, tl.Status())
	}
	look(tl, at(5, 10, 0)) // a Saturday
	if *held != 0 {
		t.Errorf("held %d on a Saturday", *held)
	}
}

// TestPlanStartedMidStretch: an app opened at 10:30 holds at once, and the
// stretch still begins at 09:00.
func TestPlanStartedMidStretch(t *testing.T) {
	tl, held, _ := newTestTool(t)
	tl.now = func() time.Time { return at(0, 10, 30) }
	tl.SetSchedule(workdays(true))
	if st := tl.Status(); *held != 1 || !st.Since.Equal(at(0, 9, 0)) {
		t.Fatalf("held %d, %+v", *held, st)
	}
}

// TestStopSkipsOnlyTheStretchItWasHolding: turning the tool off at 10:00
// keeps it off until 12:00, but tomorrow's stretch holds again.
func TestStopSkipsOnlyTheStretchItWasHolding(t *testing.T) {
	tl, held, _ := newTestTool(t)
	tl.now = func() time.Time { return at(0, 10, 0) }
	tl.SetSchedule(workdays(true))
	if err := tl.Stop(); err != nil {
		t.Fatal(err)
	}
	if *held != 0 || tl.Status().On {
		t.Fatalf("held %d after Stop", *held)
	}
	look(tl, at(0, 10, 1))
	look(tl, at(0, 11, 59))
	if *held != 0 {
		t.Errorf("held %d later in the stretch that was turned off", *held)
	}
	look(tl, at(1, 9, 0))
	if *held != 1 {
		t.Errorf("held %d at tomorrow's 09:00, want the plan back", *held)
	}
}

// TestPlanAndAPersonHoldTogether: the tool is awake while either holds, is
// named for the person's hold, and ends when the last of them ends.
func TestPlanAndAPersonHoldTogether(t *testing.T) {
	tl, held, _ := newTestTool(t)
	tl.now = func() time.Time { return at(0, 10, 0) }
	tl.SetSchedule(workdays(true))
	if err := tl.StartFor(t.Context(), time.Hour); err != nil {
		t.Fatal(err)
	}
	st := tl.Status()
	if *held != 2 || st.Active != idCustom {
		t.Fatalf("held %d, active %q: the person's hold is named", *held, st.Active)
	}
	if !st.Until.Equal(at(0, 12, 0)) {
		t.Errorf("ends at %v, want the later of 11:00 and 12:00", st.Until)
	}

	runAction(t, tl, idUntilOff)
	if st := tl.Status(); !st.Until.IsZero() {
		t.Errorf("until %v, want none: a hold with no end has none", st.Until)
	}

	tl.expire() // the person's hold ends by itself; here it ends all the same
	st = tl.Status()
	if *held != 1 || st.Active != tool.ActivePlan {
		t.Errorf("held %d, active %q: the plan goes on", *held, st.Active)
	}
}

func TestPlanDisplayFollowsThePlan(t *testing.T) {
	tl, held, displays := newTestTool(t)
	tl.now = func() time.Time { return at(0, 10, 0) }
	tl.SetSchedule(workdays(false))
	if len(*displays) != 1 || (*displays)[0] {
		t.Fatalf("display asked %v, want the display to sleep", *displays)
	}
	tl.SetSchedule(workdays(true))
	if *held != 1 || len(*displays) != 2 || !(*displays)[1] {
		t.Errorf("held %d, display asked %v: changing it asks again and lets go of the first", *held, *displays)
	}
}

func TestDisablingThePlanLetsGo(t *testing.T) {
	tl, held, _ := newTestTool(t)
	tl.now = func() time.Time { return at(0, 10, 0) }
	s := workdays(true)
	tl.SetSchedule(s)
	s.Enabled = false
	tl.SetSchedule(s)
	if *held != 0 || tl.Status().On {
		t.Errorf("held %d with the plan off", *held)
	}
	if tl.plan.wake != nil {
		t.Error("a plan that is off still wakes each minute")
	}
}

// TestScheduleIsCleaned: what the page gives may hold half-typed rows.
func TestScheduleIsCleaned(t *testing.T) {
	tl, _, _ := newTestTool(t)
	tl.SetSchedule(tool.Schedule{Enabled: true, Segments: []tool.Segment{{Days: 0, From: 1, To: 2}, {Days: tool.AllDays, From: 60, To: 120}}})
	got := tl.Schedule()
	if len(got.Segments) != 1 || got.Segments[0].From != 60 {
		t.Fatalf("schedule %+v: the segment with no day should be dropped", got)
	}
	got.Segments[0].From = 999
	if tl.Schedule().Segments[0].From != 60 {
		t.Error("Schedule hands out its own segments")
	}
}
