package power

import (
	"sync/atomic"
	"testing"
	"time"
)

// slowSystem replaces the system call with one that waits for the test, as
// MyGo's does while the main thread is busy; it counts what it is asked
// and what is let go.
func slowSystem(t *testing.T) (answer chan struct{}, asked, released *atomic.Int32) {
	t.Helper()
	answer, asked, released = make(chan struct{}), new(atomic.Int32), new(atomic.Int32)
	old := system
	system = func(string, bool) func() {
		asked.Add(1)
		<-answer
		return func() { released.Add(1) }
	}
	t.Cleanup(func() { system = old })
	return answer, asked, released
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("never: %s", what)
}

// TestKeepAwakeDoesNotWaitForTheSystem: the caller holds a lock the main
// thread needs while it asks, so it must come back before the system
// answers, and so must the function that lets go.
func TestKeepAwakeDoesNotWaitForTheSystem(t *testing.T) {
	answer, asked, released := slowSystem(t)
	returned := make(chan func())
	go func() { returned <- KeepAwake("test", true) }()
	var release func()
	select {
	case release = <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("KeepAwake waits for the system")
	}
	eventually(t, "the system was asked", func() bool { return asked.Load() == 1 })

	done := make(chan struct{})
	go func() { release(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("release waits for the system")
	}
	if released.Load() != 0 {
		t.Fatal("let go before the system had answered")
	}
	close(answer)
	eventually(t, "let go once the system answered", func() bool { return released.Load() == 1 })
}

// TestReleaseLetsGoOnce: a second call of the function does nothing.
func TestReleaseLetsGoOnce(t *testing.T) {
	answer, _, released := slowSystem(t)
	close(answer)
	release := KeepAwake("test", false)
	release()
	release()
	eventually(t, "let go", func() bool { return released.Load() >= 1 })
	time.Sleep(20 * time.Millisecond)
	if got := released.Load(); got != 1 {
		t.Errorf("let go %d times, want once", got)
	}
}
