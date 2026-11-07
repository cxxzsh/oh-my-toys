package awake

import (
	"slices"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// planState is the plan of the tool and what it holds. Every field is
// guarded by the mutex of the Tool.
type planState struct {
	sched     tool.Schedule
	release   func()    // of the plan's hold; nil while the plan does not hold
	display   bool      // whether that hold keeps the display on
	since     time.Time // the stretch of the plan it holds for
	end       time.Time
	skipUntil time.Time // a person turned the tool off: the plan stays off until then
	wake      *time.Timer
}

// releaseLocked ends the plan's hold.
func (p *planState) releaseLocked() {
	if p.release != nil {
		p.release()
		p.release = nil
	}
}

// suspendLocked ends the plan's hold and keeps the plan off to the end of
// the stretch it was holding, so that turning the tool off does not turn
// it back on a minute later.
func (p *planState) suspendLocked() {
	if p.release != nil {
		p.releaseLocked()
		p.skipUntil = p.end
	}
}

// Schedule returns the plan.
func (t *Tool) Schedule() tool.Schedule {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.plan.sched
	s.Segments = slices.Clone(s.Segments)
	return s
}

// SetSchedule makes s the plan, without the segments that are not valid,
// and applies it at once.
func (t *Tool) SetSchedule(s tool.Schedule) {
	t.mu.Lock()
	t.plan.sched = s.Clean()
	t.plan.skipUntil = time.Time{}
	t.reconcileLocked(t.now())
	t.mu.Unlock()
	t.changed()
}

// reconcileLocked brings the plan's hold in line with the plan at now, and
// sets the next time to look.
func (t *Tool) reconcileLocked(now time.Time) {
	p := &t.plan
	start, end, in := p.sched.Active(now)
	if in && now.Before(p.skipUntil) {
		in = false
	}
	if !in {
		p.releaseLocked()
	} else {
		if p.release != nil && p.display != p.sched.Display {
			p.releaseLocked()
		}
		if p.release == nil {
			p.release = keepAwake(reason, p.sched.Display)
			p.display = p.sched.Display
		}
		p.since, p.end = start, end
	}
	t.armLocked(now)
}

// armLocked wakes the plan at the next minute, while the plan is on. A
// plan is read by the wall clock and begins and ends on whole minutes, so
// that is as late as it can look.
func (t *Tool) armLocked(now time.Time) {
	p := &t.plan
	if p.wake != nil {
		p.wake.Stop()
		p.wake = nil
	}
	if !p.sched.Enabled {
		return
	}
	next := now.Truncate(time.Minute).Add(time.Minute)
	p.wake = time.AfterFunc(next.Sub(now), t.wakePlan)
}

func (t *Tool) wakePlan() {
	t.mu.Lock()
	before := t.plan.release != nil
	t.reconcileLocked(t.now())
	moved := before != (t.plan.release != nil)
	t.mu.Unlock()
	if moved {
		t.changed()
	}
}
