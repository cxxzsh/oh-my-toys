package flowlog

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// TestConcurrentUse runs writers, readers and a clearer at once, the way
// the capture and the inspector do; the race detector checks the rest.
func TestConcurrentUse(t *testing.T) {
	log := New(32, 4096)
	defer log.OnChange(func() {})()
	var lastID atomic.Uint64
	var wg sync.WaitGroup

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				f := sizedFlow(64, "writer")
				if i%3 == 0 {
					// Replacing a flow that may be gone is a no-op.
					f.ID = lastID.Load()
				}
				lastID.Store(log.Put(f))
			}
		}()
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 300 {
			readAndCount(t, log)
		}
	}()
	go func() {
		defer wg.Done()
		for range 50 {
			log.Clear()
		}
	}()

	wg.Wait()
	if rows := log.Rows(tool.Filter{}); len(rows) > 32 {
		t.Fatalf("%d rows past the capacity", len(rows))
	}
}

// readAndCount is what one frame of the inspector does, counting the flows
// without building the rows for it.
func readAndCount(t *testing.T, log *Log) {
	t.Helper()
	_ = log.Rows(tool.Filter{Source: tool.SourceProxy, Text: "writer"})
	_ = log.Version()
	if n := log.Len(); n > 32 {
		t.Errorf("Len %d past the capacity", n)
	}
}
