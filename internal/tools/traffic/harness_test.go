package traffic

import (
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/platform/sysproxy"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// fakeSystem is a machine whose every call the tool makes outside its own
// package is answered by the test: nothing reaches the system proxy, the
// trust store or a real interface.
type fakeSystem struct {
	mu    sync.Mutex
	calls []string

	snap       sysproxy.Snapshot // what Current returns
	before     sysproxy.Snapshot // what Enable says it replaced
	currentErr error
	enableErr  error
	restoreErr error
	restored   []sysproxy.Snapshot

	// What RestorePointing is given, and whether it refuses.
	pointing     []sysproxy.Snapshot
	pointingAddr string
	pointingErr  error

	// What apps would use, and whether reading it fails. A capture takes
	// the system proxy for apps unless a test says it does not.
	noEffect     bool
	effectiveErr error

	trusted    bool
	trustErr   error
	untrustErr error
	installed  []string
	untrusted  []string

	open        func(string) (pcap.Source, error)
	ifaces      []pcap.Interface
	ifaceErr    error
	defName     string
	defErr      error
	grant       func(context.Context, string) error
	granted     int
	grantReason string

	now      time.Time
	onEnable func() // run inside Enable, before it answers
}

// record notes one call, in the order it happened.
func (f *fakeSystem) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

// callsSoFar is the calls recorded so far.
func (f *fakeSystem) callsSoFar() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// seen reports whether a call was recorded.
func (f *fakeSystem) seen(call string) bool {
	for _, c := range f.callsSoFar() {
		if c == call {
			return true
		}
	}
	return false
}

// seenPrefix reports whether a call starting with prefix was recorded.
func (f *fakeSystem) seenPrefix(prefix string) bool {
	for _, c := range f.callsSoFar() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// clock is the time the tool sees.
func (f *fakeSystem) clock() time.Time {
	if f.now.IsZero() {
		return time.Now()
	}
	return f.now
}

// system is the seam the tool is built over.
func (f *fakeSystem) system() system {
	return system{
		sysproxy: proxySettings{
			current: func() (sysproxy.Snapshot, error) {
				f.record("current")
				if f.currentErr != nil {
					return sysproxy.Snapshot{}, f.currentErr
				}
				return f.snap, nil
			},
			enable: func(hostport string) (sysproxy.Snapshot, error) {
				f.record("enable " + hostport)
				if f.onEnable != nil {
					f.onEnable()
				}
				if f.enableErr != nil {
					return sysproxy.Snapshot{}, f.enableErr
				}
				return f.before, nil
			},
			restore: func(s sysproxy.Snapshot) error {
				f.record("restore")
				f.mu.Lock()
				f.restored = append(f.restored, s)
				f.mu.Unlock()
				return f.restoreErr
			},
			restorePointing: func(s sysproxy.Snapshot, hostport string) error {
				f.record("restore pointing " + hostport)
				f.mu.Lock()
				f.pointing = append(f.pointing, s)
				f.pointingAddr = hostport
				f.mu.Unlock()
				return f.pointingErr
			},
			effective: func(string) (bool, error) {
				f.record("effective")
				return !f.noEffect, f.effectiveErr
			},
		},
		trust: trustStore{
			install: func(certPEM []byte) error {
				f.record("trust install")
				f.mu.Lock()
				f.installed = append(f.installed, string(certPEM))
				f.mu.Unlock()
				return f.trustErr
			},
			remove: func(certPEM []byte) error {
				f.record("trust remove")
				f.mu.Lock()
				f.untrusted = append(f.untrusted, string(certPEM))
				f.mu.Unlock()
				return f.untrustErr
			},
			trusted: func([]byte) (bool, error) {
				f.record("trusted")
				return f.trusted, f.trustErr
			},
		},
		pcap: packetSource{
			open: func(name string) (pcap.Source, error) {
				f.record("open " + name)
				if f.open == nil {
					return nil, pcap.ErrUnsupported
				}
				return f.open(name)
			},
			interfaces: func() ([]pcap.Interface, error) {
				f.record("interfaces")
				return f.ifaces, f.ifaceErr
			},
			defaultOne: func() (string, error) {
				f.record("default")
				return f.defName, f.defErr
			},
			grant: func(ctx context.Context, reason string) error {
				f.record("grant")
				f.mu.Lock()
				f.granted++
				f.grantReason = reason
				f.mu.Unlock()
				if f.grant == nil {
					return nil
				}
				return f.grant(ctx, reason)
			},
		},
		now: f.clock,
	}
}

// testTool is a tool over a fake machine, in a folder of its own.
func testTool(t *testing.T) (*Tool, *fakeSystem) {
	t.Helper()
	fake := &fakeSystem{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	return newTool(t.TempDir(), fake.system()), fake
}

// freePort is a loopback port nothing listens on, which the test then
// hands to the capture.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

// fakeSource delivers the packets it is given, and ends like a closed
// capture does.
type fakeSource struct {
	packets chan pcap.Packet
	closed  chan struct{}
	once    sync.Once
}

// newFakeSource is a source with room for the packets a test pushes.
func newFakeSource() *fakeSource {
	return &fakeSource{packets: make(chan pcap.Packet, 8), closed: make(chan struct{})}
}

func (s *fakeSource) Read() (pcap.Packet, error) {
	select {
	case p := <-s.packets:
		return p, nil
	case <-s.closed:
		return pcap.Packet{}, io.EOF
	}
}

func (s *fakeSource) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

// waitRows waits until the log holds at least want finished rows of source,
// and returns the snapshot it read. A row appears when its request goes out,
// and its body is written only when the response finishes.
func waitRows(t *testing.T, log tool.FlowLog, source tool.FlowSource, want int) []tool.FlowRow {
	t.Helper()
	deadline := time.Now().Add(captureWait)
	for {
		rows := log.Rows(tool.Filter{Source: source})
		if len(rows) >= want && allFinished(rows[:want]) {
			return rows
		}
		if time.Now().After(deadline) {
			t.Fatalf("the log holds %d rows, want %d finished", len(rows), want)
		}
		time.Sleep(pollEvery)
	}
}

// allFinished reports whether none of the rows is still open.
func allFinished(rows []tool.FlowRow) bool {
	for _, row := range rows {
		if row.State == tool.FlowOpen {
			return false
		}
	}
	return true
}

// waitFlow waits until the log holds the flow and it is no longer open, and
// returns it with its response.
func waitFlow(t *testing.T, tl *Tool, id uint64) tool.Flow {
	t.Helper()
	deadline := time.Now().Add(captureWait)
	for {
		if flow, ok := tl.Log().Get(id); ok && flow.State != tool.FlowOpen {
			return flow
		}
		if time.Now().After(deadline) {
			flow, ok := tl.Log().Get(id)
			if !ok {
				t.Fatalf("flow %d is not recorded", id)
			}
			t.Fatalf("flow %d is still open (%d)", id, flow.State)
		}
		time.Sleep(pollEvery)
	}
}

// waitOff waits until the capture reports itself off, and returns its
// status then.
func waitOff(t *testing.T, tl *Tool) tool.Status {
	t.Helper()
	deadline := time.Now().Add(captureWait)
	for {
		if st := tl.Status(); !st.On {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatal("the capture is still on")
		}
		time.Sleep(pollEvery)
	}
}

// How long a test waits for a capture to record or to end, and how often
// it looks.
const (
	captureWait = 5 * time.Second
	pollEvery   = 5 * time.Millisecond
)
