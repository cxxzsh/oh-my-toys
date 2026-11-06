package core

import "sync"

// Events tells the front end that some state changed: a switch tool
// started or stopped, a tool failed. Tools get its Changed method when they
// are made, before the front end listens.
type Events struct {
	mu sync.Mutex
	fn func()
}

// Changed reports a change, from any goroutine.
func (e *Events) Changed() {
	e.mu.Lock()
	fn := e.fn
	e.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// OnChanged sets what Changed calls.
func (e *Events) OnChanged(fn func()) {
	e.mu.Lock()
	e.fn = fn
	e.mu.Unlock()
}
