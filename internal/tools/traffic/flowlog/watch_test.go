package flowlog

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// drain empties the buffered calls of a test subscriber.
func drain(calls chan struct{}) {
	for {
		select {
		case <-calls:
		default:
			return
		}
	}
}

func TestOnChangeCoalesces(t *testing.T) {
	log := New(100, 1<<20)
	var mu sync.Mutex
	var calls []time.Time
	stop := log.OnChange(func() {
		mu.Lock()
		calls = append(calls, time.Now())
		mu.Unlock()
	})
	defer stop()

	for range 10 {
		log.Put(sizedFlow(1, "burst"))
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(3 * notifyInterval)

	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 2 {
		t.Fatalf("%d calls for one burst of ten changes", len(calls))
	}
	if len(calls) > 6 {
		t.Fatalf("%d calls for one burst of ten changes", len(calls))
	}
	for i := 1; i < len(calls); i++ {
		if gap := calls[i].Sub(calls[i-1]); gap < notifyInterval-time.Millisecond {
			t.Fatalf("two calls %v apart", gap)
		}
	}
}

func TestOnChangeStops(t *testing.T) {
	baseline := runtime.NumGoroutine()
	log := New(10, 1<<20)
	calls := make(chan struct{}, 16)
	stop := log.OnChange(func() { calls <- struct{}{} })

	log.Put(sizedFlow(1, "one"))
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("no call after a change")
	}

	stop()
	stop()

	// A call already running may still arrive; give it its time, then make
	// sure nothing more comes.
	time.Sleep(3 * notifyInterval)
	drain(calls)
	log.Put(sizedFlow(1, "two"))
	select {
	case <-calls:
		t.Fatal("called after stop")
	case <-time.After(3 * notifyInterval):
	}

	// The log's goroutine ends with its last subscriber.
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > baseline {
		t.Fatalf("%d goroutines, %d before the log", n, baseline)
	}
}

func TestOnChangeStopFromCallback(t *testing.T) {
	log := New(10, 1<<20)
	done := make(chan struct{})
	var once sync.Once
	var stop func()
	stop = log.OnChange(func() {
		stop()
		once.Do(func() { close(done) })
	})
	log.Put(sizedFlow(1, "one"))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the callback did not run")
	}
}

func TestOnChangeKeepsTheOtherSubscribers(t *testing.T) {
	log := New(10, 1<<20)
	first := make(chan struct{}, 8)
	second := make(chan struct{}, 8)
	stopFirst := log.OnChange(func() { first <- struct{}{} })
	stopSecond := log.OnChange(func() { second <- struct{}{} })
	defer stopSecond()

	log.Put(sizedFlow(1, "one"))
	for name, calls := range map[string]chan struct{}{"first": first, "second": second} {
		select {
		case <-calls:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s subscriber was not called", name)
		}
	}

	stopFirst()
	time.Sleep(3 * notifyInterval)
	drain(first)
	log.Put(sizedFlow(1, "two"))
	select {
	case <-second:
	case <-time.After(2 * time.Second):
		t.Fatal("the second subscriber lost its calls")
	}
	select {
	case <-first:
		t.Fatal("the stopped subscriber was called")
	case <-time.After(notifyInterval):
	}
}
