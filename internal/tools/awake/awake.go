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
	idHour       = "1h"
	idTwoHours   = "2h"
	idUntilOff   = "on"
	idSystemOnly = "system"
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
			{ID: idHour, Title: i18n.Messages.AwakeFor1h, Shortcut: "Alt+1", Run: t.runFor(idHour, time.Hour, true)},
			{ID: idTwoHours, Title: i18n.Messages.AwakeFor2h, Run: t.runFor(idTwoHours, 2*time.Hour, true)},
			{ID: idUntilOff, Title: i18n.Messages.AwakeUntilOff, Run: t.runFor(idUntilOff, 0, true)},
			{ID: idSystemOnly, Title: i18n.Messages.AwakeSystemOnly, Run: t.runFor(idSystemOnly, 0, false)},
			{ID: idOff, Title: i18n.Messages.AwakeTurnOff, Stops: true, Run: func(context.Context) error { return t.Stop() }},
		},
	}
}

// Specs declares the tool's actions: start and stop change whether the
// system may sleep, and start may say for how long; status only reads it.
func (t *Tool) Specs() []tool.Spec {
	return tool.TimedSwitchSpecs(i18n.Messages.AwakeStartSummary, i18n.Messages.AwakeStopSummary, i18n.Messages.AwakeStatusSummary)
}

func (t *Tool) runFor(id string, d time.Duration, display bool) func(context.Context) error {
	return func(context.Context) error {
		t.start(id, d, display)
		return nil
	}
}

// Start keeps the computer awake with the settings used last.
func (t *Tool) Start(context.Context) error {
	t.mu.Lock()
	id, d, display := t.id, t.last, t.display
	t.mu.Unlock()
	t.start(id, d, display)
	return nil
}

// StartFor keeps the computer awake for d, with the display setting used
// last.
func (t *Tool) StartFor(_ context.Context, d time.Duration) error {
	t.mu.Lock()
	display := t.display
	t.mu.Unlock()
	t.start(idCustom, d, display)
	return nil
}

func (t *Tool) start(id string, d time.Duration, display bool) {
	t.mu.Lock()
	t.stopHoldLocked()
	t.release = keepAwake(reason, display)
	t.id, t.last, t.display = id, d, display
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
// stays on. A person's hold is what it names; the plan's, when it is the
// only one.
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
		st.Until = lastEnd(hand, t.until, t.plan.end)
		st.Since = firstStart(hand, t.since, t.plan.since)
		if !hand {
			st.Active = tool.ActivePlan
		}
	}
	st.Detail = displayDetail(display)
	st.Text = t.statusText(hand, st.Until)
	return st
}

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

// lastEnd is when the last of the holds ends; zero when one never does.
func lastEnd(hand bool, handEnd, planEnd time.Time) time.Time {
	if hand && handEnd.IsZero() {
		return time.Time{}
	}
	if hand && handEnd.After(planEnd) {
		return handEnd
	}
	return planEnd
}

// firstStart is when the first of the holds began.
func firstStart(hand bool, handStart, planStart time.Time) time.Time {
	if hand && handStart.Before(planStart) {
		return handStart
	}
	return planStart
}
