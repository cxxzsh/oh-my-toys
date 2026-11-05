package core

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

// Failure is a tool's last recovered panic or error.
type Failure struct {
	Where string // "view", "action", "background"
	Value string
	Stack string
	At    time.Time
}

// Runner runs tools' code so that one tool's failure stays inside it: a
// panic is recovered and kept as the tool's failure, which its page shows,
// while the app and the other tools go on. Background work runs on
// goroutines the runner owns, under a context it can cancel.
//
// Go cannot recover a panic on a goroutine that defers no recover, so a
// tool's background work goes through Go, never a bare go statement.
type Runner struct {
	mu       sync.Mutex
	failures map[string]*Failure
	changed  func()
}

// NewRunner makes a runner; changed is called, from any goroutine, when a
// failure is kept or cleared.
func NewRunner(changed func()) *Runner {
	return &Runner{failures: map[string]*Failure{}, changed: changed}
}

// Failed returns a tool's last failure, or nil.
func (r *Runner) Failed(tool string) *Failure {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failures[tool]
}

// Clear forgets a tool's failure.
func (r *Runner) Clear(tool string) {
	r.mu.Lock()
	delete(r.failures, tool)
	r.mu.Unlock()
	r.notify()
}

// Do runs fn, keeping a panic as the tool's failure. It reports whether fn
// returned normally.
func (r *Runner) Do(tool, where string, fn func()) (ok bool) {
	defer func() {
		if v := recover(); v != nil {
			r.record(tool, where, fmt.Sprint(v), string(debug.Stack()))
			ok = false
		}
	}()
	fn()
	return true
}

// Go runs fn on a goroutine under ctx with a timeout (none when 0),
// keeping a panic as the tool's failure. The returned function cancels it.
func (r *Runner) Go(ctx context.Context, tool string, timeout time.Duration, fn func(ctx context.Context)) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}
	go func() {
		defer cancel()
		r.Do(tool, "background", func() { fn(ctx) })
	}()
	return cancel
}

func (r *Runner) record(tool, where, value, stack string) {
	f := &Failure{Where: where, Value: value, Stack: stack, At: time.Now()}
	r.mu.Lock()
	r.failures[tool] = f
	r.mu.Unlock()
	log.Printf("tool %s failed in %s: %s\n%s", tool, where, value, stack)
	r.notify()
}

func (r *Runner) notify() {
	if r.changed != nil {
		r.changed()
	}
}
