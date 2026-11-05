// Package awake keeps the computer from sleeping, for a while or until
// turned off.
package awake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/platform/power"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

// ID is the tool's ID and command name.
const ID = "awake"

// reason is what the system lists as keeping the computer awake.
const reason = "oh-my-toys: Keep Awake"

// Tool is Keep Awake. Its zero value is not usable; use New.
type Tool struct {
	mu      sync.Mutex
	release func()
	until   time.Time // zero while it runs until turned off
	timer   *time.Timer
	last    time.Duration // the length chosen last; 0 for until turned off
	display bool          // keep the display on too
	changed func()
}

// New makes the tool; changed is called, from any goroutine, when it
// starts or stops.
func New(changed func()) *Tool {
	return &Tool{display: true, changed: changed}
}

// Manifest describes the tool.
func (t *Tool) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:        ID,
		Name:      i18n.Messages.AwakeName,
		Summary:   i18n.Messages.AwakeSummary,
		Category:  tool.CategorySystem,
		Kind:      tool.KindSwitch,
		Icon:      "sun",
		Keywords:  []string{"caffeinate", "keep awake", "amphetamine", "sleep", "不睡眠", "咖啡因"},
		Platforms: []string{"macos", "windows", "linux"},
		Actions: []tool.QuickAction{
			{ID: "1h", Title: i18n.Messages.AwakeFor1h, Shortcut: "Alt+1", Run: t.runFor(time.Hour, true)},
			{ID: "2h", Title: i18n.Messages.AwakeFor2h, Run: t.runFor(2*time.Hour, true)},
			{ID: "on", Title: i18n.Messages.AwakeUntilOff, Run: t.runFor(0, true)},
			{ID: "system", Title: i18n.Messages.AwakeSystemOnly, Run: t.runFor(0, false)},
			{ID: "off", Title: i18n.Messages.AwakeTurnOff, Run: func(context.Context) error { return t.Stop() }},
		},
	}
}

func (t *Tool) runFor(d time.Duration, display bool) func(context.Context) error {
	return func(context.Context) error {
		t.start(d, display)
		return nil
	}
}

// Start keeps the computer awake with the settings used last.
func (t *Tool) Start(context.Context) error {
	t.mu.Lock()
	d, display := t.last, t.display
	t.mu.Unlock()
	t.start(d, display)
	return nil
}

func (t *Tool) start(d time.Duration, display bool) {
	t.mu.Lock()
	t.stopLocked()
	t.release = power.KeepAwake(reason, display)
	t.last, t.display, t.until = d, display, time.Time{}
	if d > 0 {
		t.until = time.Now().Add(d)
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
	if t.until.IsZero() {
		return tool.Status{On: true, Text: i18n.AwakeRunningUntilOff(), Detail: detail}
	}
	left := max(time.Until(t.until), 0).Round(time.Second)
	return tool.Status{On: true, Text: i18n.AwakeRunningLeft(clock(left)), Detail: detail}
}

// clock writes a duration as 2:29:10.
func clock(d time.Duration) string {
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}
