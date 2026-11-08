// Package awake keeps the computer from sleeping, for a while or until
// turned off.
package awake

import (
	"context"
	"sync"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/platform/power"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// ID is the tool's ID and command name.
const ID = "awake"

// The IDs of the quick actions.
const (
	idHalfHour   = "30m"
	idHour       = "1h"
	idTwoHours   = "2h"
	idFourHours  = "4h"
	idEightHours = "8h"
	idUntilOff   = "on"
	idOff        = "off"
	idCustom     = "custom" // a length or time of day the person typed, which has no quick action
)

// reason is what the system lists as keeping the computer awake.
const reason = "oh-my-toys: Keep Awake"

// keepAwake asks the system to stay awake; tests replace it.
var keepAwake = power.KeepAwake

// Tool is Keep Awake. Its zero value is not usable; use New. It holds the
// computer awake for two reasons that do not know of each other: a person
// started it (a quick action, a length), or its plan says it is time. It is
// awake while either holds.
type Tool struct {
	mu      sync.Mutex
	now     func() time.Time // the clock; tests replace it
	changed func()

	release func() // of the person's hold; nil when they have none
	since   time.Time
	until   time.Time // zero while it runs until turned off
	timer   *time.Timer
	id      string        // the quick action in effect, or used last
	last    time.Duration // the length chosen last; 0 for until turned off
	display bool          // keep the display on too

	plan planState
}

// New makes the tool; changed is called, from any goroutine, when it
// starts or stops.
func New(changed func()) *Tool {
	return &Tool{
		id: idUntilOff, display: true, changed: changed, now: time.Now,
		plan: planState{sched: tool.Schedule{Display: true}},
	}
}

// Manifest describes the tool.
func (t *Tool) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:        ID,
		Name:      i18n.Messages.AwakeName,
		Summary:   i18n.Messages.AwakeSummary,
		Category:  tool.CategorySystem,
		Kind:      tool.KindSwitch,
		Icon:      "bedtime_off",
		Art:       "keep-awake",
		Keywords:  []string{"caffeinate", "keep awake", "amphetamine", "sleep", "不睡眠", "咖啡因"},
		Platforms: []string{"macos", "windows", "linux"},
		Actions: []tool.QuickAction{
			t.preset(idHalfHour, 30*time.Minute, i18n.Messages.AwakeFor30m, i18n.Messages.Awake30mShort),
			{
				ID: idHour, Title: i18n.Messages.AwakeFor1h, Short: i18n.Messages.Awake1hShort, Length: time.Hour, Tray: true,
				Shortcut: "Alt+1", Run: t.runFor(idHour, time.Hour),
			},
			{
				ID: idTwoHours, Title: i18n.Messages.AwakeFor2h, Short: i18n.Messages.Awake2hShort, Length: 2 * time.Hour,
				Tray: true, Run: t.runFor(idTwoHours, 2*time.Hour),
			},
			t.preset(idFourHours, 4*time.Hour, i18n.Messages.AwakeFor4h, i18n.Messages.Awake4hShort),
			t.preset(idEightHours, 8*time.Hour, i18n.Messages.AwakeFor8h, i18n.Messages.Awake8hShort),
			{
				ID: idUntilOff, Title: i18n.Messages.AwakeUntilOff, Short: i18n.Messages.AwakeForeverShort, Tray: true,
				Run: t.runFor(idUntilOff, 0),
			},
			{ID: idOff, Title: i18n.Messages.AwakeTurnOff, Stops: true, Run: func(context.Context) error { return t.Stop() }},
		},
	}
}

// Specs declares the tool's actions: start and stop change whether the
// system may sleep, and start may say for how long; status only reads it.
func (t *Tool) Specs() []tool.Spec {
	return tool.ScreenedSwitchSpecs(i18n.Messages.AwakeStartSummary, i18n.Messages.AwakeStopSummary, i18n.Messages.AwakeStatusSummary)
}

func (t *Tool) runFor(id string, d time.Duration) func(context.Context) error {
	return func(context.Context) error {
		t.start(id, d)
		return nil
	}
}

// preset is a quick action that keeps the computer awake for a length.
func (t *Tool) preset(id string, d time.Duration, title, short tool.Message) tool.QuickAction {
	return tool.QuickAction{ID: id, Title: title, Short: short, Length: d, Run: t.runFor(id, d)}
}

// Start keeps the computer awake with the settings used last.
func (t *Tool) Start(context.Context) error {
	t.mu.Lock()
	id, d := t.id, t.last
	t.mu.Unlock()
	t.start(id, d)
	return nil
}

// StartFor keeps the computer awake for d.
func (t *Tool) StartFor(_ context.Context, d time.Duration) error {
	t.start(idCustom, d)
	return nil
}

// Display says whether a person's hold keeps the display on too.
func (t *Tool) Display() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.display
}

// SetDisplay chooses whether a person's hold keeps the display on too. It
// is the choice of the next hold, and of the one in progress, which keeps
// its length.
func (t *Tool) SetDisplay(on bool) {
	t.mu.Lock()
	t.display = on
	if old := t.release; old != nil {
		t.release = keepAwake(reason, on)
		old()
	}
	t.mu.Unlock()
	t.changed()
}

func (t *Tool) start(id string, d time.Duration) {
	t.mu.Lock()
	t.stopHoldLocked()
	t.release = keepAwake(reason, t.display)
	t.id, t.last = id, d
	t.since, t.until = t.now(), time.Time{}
	if d > 0 {
		t.until = t.since.Add(d)
		t.timer = time.AfterFunc(d, t.expire)
	}
	t.mu.Unlock()
	t.changed()
}

// expire ends a person's hold that has run its length. The plan, if it
// holds, goes on.
func (t *Tool) expire() {
	t.mu.Lock()
	t.stopHoldLocked()
	t.reconcileLocked(t.now())
	t.mu.Unlock()
	t.changed()
}

// Stop lets the computer sleep again, whatever holds it. A plan that holds
// stays off to the end of its stretch, and goes on at the next.
func (t *Tool) Stop() error {
	t.mu.Lock()
	t.stopHoldLocked()
	t.plan.suspendLocked()
	t.mu.Unlock()
	t.changed()
	return nil
}

func (t *Tool) stopHoldLocked() {
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	if t.release != nil {
		t.release()
		t.release = nil
	}
}

// Status says whether it runs, for how long still, and whether the display
// stays on. A person's hold is what it names, with its own start and end;
// the plan's, when it is the only one. When both hold, the plan's end is in
// the note, so the countdown is never the plan's while a person asked for
// less.
func (t *Tool) Status() tool.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	hand, plan := t.release != nil, t.plan.release != nil
	if !hand && !plan {
		return tool.Status{Text: i18n.StatusOff()}
	}
	st := tool.Status{On: true, Active: t.id, Since: t.since, Until: t.until}
	display := hand && t.display
	if plan {
		display = display || t.plan.display
	}
	if !hand {
		st.Active, st.Since, st.Until = tool.ActivePlan, t.plan.since, t.plan.end
	} else if plan {
		st.Note = i18n.AwakePlanAlso(t.plan.end.Format(clockLayout))
	}
	st.Detail = displayDetail(display)
	st.Text = t.statusText(hand, st.Until)
	return st
}

// clockLayout is how the note writes a time of day.
const clockLayout = "15:04"

func (t *Tool) statusText(hand bool, until time.Time) string {
	if until.IsZero() {
		return i18n.AwakeRunningUntilOff()
	}
	left := tool.Clock(max(until.Sub(t.now()), 0).Round(time.Second))
	if hand {
		return i18n.AwakeRunningLeft(left)
	}
	return i18n.AwakeRunningPlan(left)
}

func displayDetail(on bool) string {
	if on {
		return i18n.AwakeDisplayOn()
	}
	return i18n.AwakeDisplayMaySleep()
}
