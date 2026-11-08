package flowlog

import (
	"sync"
	"sync/atomic"
	"time"
)

// notifyInterval is the least time between two calls of a subscriber, so
// that a burst of Put calls wakes the inspector once.
const notifyInterval = 100 * time.Millisecond

// subscriber is one OnChange registration.
type subscriber struct {
	fn      func()
	stopped atomic.Bool
}

// OnChange calls fn after the log changes, off the caller's goroutine, at
// most every 100 ms: the first change is passed on at once and the ones
// during the next 100 ms are one call. stop ends the calls and is safe to
// call more than once. The log owns one goroutine while a subscriber
// exists and ends it when the last stop runs, so there is nothing to close.
func (l *Log) OnChange(fn func()) (stop func()) {
	s := &subscriber{fn: fn}
	var once sync.Once
	stop = func() {
		once.Do(func() { l.unsubscribe(s) })
	}
	if fn == nil {
		return stop
	}
	l.subscribe(s)
	return stop
}

func (l *Log) subscribe(s *subscriber) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.subscribers = append(l.subscribers, s)
	if l.watching {
		return
	}
	// A wake left from changes nobody listened to must not call fn at once.
	select {
	case <-l.wake:
	default:
	}
	l.quit = make(chan struct{})
	l.watching = true
	go l.watch(l.quit)
}

func (l *Log) unsubscribe(s *subscriber) {
	s.stopped.Store(true)
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, sub := range l.subscribers {
		if sub == s {
			l.subscribers = append(l.subscribers[:i], l.subscribers[i+1:]...)
			break
		}
	}
	if len(l.subscribers) > 0 || !l.watching {
		return
	}
	close(l.quit)
	l.watching = false
}

// watch is the one goroutine a log with subscribers owns. It passes a
// change on, then waits out the interval during which further changes are
// one call, and ends with its quit channel.
func (l *Log) watch(quit <-chan struct{}) {
	for {
		select {
		case <-quit:
			return
		case <-l.wake:
		}
		l.deliver()
		cooldown := time.NewTimer(notifyInterval)
		select {
		case <-quit:
			cooldown.Stop()
			return
		case <-cooldown.C:
		}
	}
}

// deliver calls every subscriber that has not stopped, outside the lock so
// that a subscriber may read the log.
func (l *Log) deliver() {
	l.mu.Lock()
	subs := make([]*subscriber, len(l.subscribers))
	copy(subs, l.subscribers)
	l.mu.Unlock()
	for _, s := range subs {
		if !s.stopped.Load() {
			s.fn()
		}
	}
}

// notify wakes the watcher, never waiting for it: a burst of changes is one
// wake. It runs with the lock held, which keeps the wake channel and the
// watcher's life in step.
func (l *Log) notify() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}
