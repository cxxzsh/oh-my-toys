package packets

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// errRead is the failure a fake source reports instead of EOF.
var errRead = errors.New("interface went away")

// fakeSource is a pcap.Source the tests drive: it hands out the packets it
// was given, then either returns its error or waits for Close, the way a
// real capture waits for its next packet.
type fakeSource struct {
	mu       sync.Mutex
	packets  []pcap.Packet
	err      error
	block    bool
	closes   int
	closed   chan struct{}
	closeOne sync.Once
	started  chan struct{}
	startOne sync.Once
}

func newFakeSource(packets ...pcap.Packet) *fakeSource {
	return &fakeSource{
		packets: packets,
		err:     io.EOF,
		closed:  make(chan struct{}),
		started: make(chan struct{}),
	}
}

func (s *fakeSource) Read() (pcap.Packet, error) {
	s.startOne.Do(func() { close(s.started) })
	s.mu.Lock()
	if len(s.packets) > 0 {
		packet := s.packets[0]
		s.packets = s.packets[1:]
		s.mu.Unlock()
		return packet, nil
	}
	block, err := s.block, s.err
	s.mu.Unlock()
	if block {
		<-s.closed
		return pcap.Packet{}, io.EOF
	}
	return pcap.Packet{}, err
}

func (s *fakeSource) Close() error {
	s.mu.Lock()
	s.closes++
	s.mu.Unlock()
	s.closeOne.Do(func() { close(s.closed) })
	return nil
}

func (s *fakeSource) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closes
}

// fakeRecorder keeps the flows Run recorded, safe for the race detector.
type fakeRecorder struct {
	mu    sync.Mutex
	flows []tool.Flow
}

func (r *fakeRecorder) Put(f tool.Flow) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flows = append(r.flows, f)
	return uint64(len(r.flows))
}

func (r *fakeRecorder) snapshot() []tool.Flow {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]tool.Flow(nil), r.flows...)
}

func TestRunRecordsEveryPacket(t *testing.T) {
	frame := v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, nil))
	src := newFakeSource(
		pcap.Packet{Time: testTime, Link: pcap.LinkEthernet, Data: frame, Length: len(frame)},
		pcap.Packet{Time: testTime, Link: pcap.LinkRaw, Data: frame, Length: len(frame)},
	)
	rec := &fakeRecorder{}
	if err := Run(context.Background(), src, rec); err != nil {
		t.Fatalf("Run returned %v, want nil after EOF", err)
	}
	flows := rec.snapshot()
	if len(flows) != 2 {
		t.Fatalf("recorded %d flows, want 2", len(flows))
	}
	for _, flow := range flows {
		if flow.Source != tool.SourcePacket {
			t.Errorf("Source = %v, want packet", flow.Source)
		}
		if flow.Packet == nil {
			t.Error("a packet flow without its detail")
		}
	}
	if got := src.closeCount(); got != 1 {
		t.Errorf("source closed %d times, want once", got)
	}
}

func TestRunReturnsReadError(t *testing.T) {
	src := newFakeSource()
	src.err = errRead
	err := Run(context.Background(), src, &fakeRecorder{})
	if !errors.Is(err, errRead) {
		t.Fatalf("Run returned %v, want the read error", err)
	}
	if got := src.closeCount(); got != 1 {
		t.Errorf("source closed %d times, want once", got)
	}
}

func TestRunCancelUnblocksRead(t *testing.T) {
	src := newFakeSource()
	src.block = true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, src, &fakeRecorder{}) }()

	<-src.started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want the context error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
	if got := src.closeCount(); got != 1 {
		t.Errorf("source closed %d times, want once", got)
	}
}

func TestRunWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src := newFakeSource()
	src.block = true
	err := Run(ctx, src, &fakeRecorder{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want the context error", err)
	}
	if got := src.closeCount(); got != 1 {
		t.Errorf("source closed %d times, want once", got)
	}
}
