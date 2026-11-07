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
)

// reason is what the system lists as keeping the computer awake.
const reason = "oh-my-toys: Keep Awake"

// keepAwake asks the system to stay awake; tests replace it.
var keepAwake = power.KeepAwake

// Tool is Keep Awake. Its zero value is not usable; use New.
type Tool struct {
	mu      sync.Mutex
	release func()
	since   time.Time // when it started
	until   time.Time // zero while it runs until turned off
	timer   *time.Timer
	id      string        // the quick action in effect, or used last
	last    time.Duration // the length chosen last; 0 for until turned off
	display bool          // keep the display on too
	changed func()
}

// New makes the tool; changed is called, from any goroutine, when it
// starts or stops.
func New(changed func()) *Tool {
	return &Tool{id: idUntilOff, display: true, changed: changed}
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
// system may sleep; status only reads it.
func (t *Tool) Specs() []tool.Spec {
	return tool.SwitchSpecs(i18n.Messages.AwakeStartSummary, i18n.Messages.AwakeStopSummary, i18n.Messages.AwakeStatusSummary)
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

func (t *Tool) start(id string, d time.Duration, display bool) {
	t.mu.Lock()
	t.stopLocked()
	t.release = keepAwake(reason, display)
	t.id, t.last, t.display = id, d, display
	t.since, t.until = time.Now(), time.Time{}
	if d > 0 {
		t.until = t.since.Add(d)
		t.timer = time.AfterFunc(d, func() { _ = t.Stop() })
	}
	t.mu.Unlock()
	t.changed()
}

// Stop lets the computer sleep again.
func (t *Tool) Stop() error {
	t.mu.Lock()
	t.stopLocked()
	t.mu.Unlock()
	t.changed()
	return nil
}

func (t *Tool) stopLocked() {
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
// stays on.
func (t *Tool) Status() tool.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.release == nil {
		return tool.Status{Text: i18n.StatusOff()}
	}
	detail := i18n.AwakeDisplayOn()
	if !t.display {
		detail = i18n.AwakeDisplayMaySleep()
	}
	st := tool.Status{On: true, Detail: detail, Active: t.id, Since: t.since, Until: t.until}
	if t.until.IsZero() {
		st.Text = i18n.AwakeRunningUntilOff()
		return st
	}
	left := max(time.Until(t.until), 0).Round(time.Second)
	st.Text = i18n.AwakeRunningLeft(tool.Clock(left))
	return st
}
