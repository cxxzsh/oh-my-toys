package devtools

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// TestFakeTrafficLogFilters checks the made-up log's rows against the
// filter the contract describes, which the inspector relies on.
func TestFakeTrafficLogFilters(t *testing.T) {
	l := newFakeLog()
	l.add(tool.Flow{FlowRow: tool.FlowRow{
		Source: tool.SourceProxy, Method: "GET", Host: "api.example.com", Path: "/items",
		Status: 200, Type: "application/json", State: tool.FlowDone,
	}})
	l.add(tool.Flow{FlowRow: tool.FlowRow{
		Source: tool.SourceProxy, Method: "POST", Host: "www.example.com", Path: "/form",
		Status: 500, Type: "text/html", State: tool.FlowDone,
	}})
	l.add(tool.Flow{FlowRow: tool.FlowRow{
		Source: tool.SourceProxy, Method: "GET", Host: "down.example.com", Path: "/health",
		State: tool.FlowFailed, Info: "connection refused",
	}})
	tests := []struct {
		name   string
		filter tool.Filter
		want   []uint64
	}{
		{"everything of the source", tool.Filter{}, []uint64{1, 2, 3}},
		{"a method", tool.Filter{Methods: []string{"POST"}}, []uint64{2}},
		{"a status class", tool.Filter{StatusClasses: []int{5}}, []uint64{2}},
		{"no response", tool.Filter{StatusClasses: []int{0}}, []uint64{3}},
		{"a type prefix", tool.Filter{Types: []string{"text/"}}, []uint64{2}},
		{"a host", tool.Filter{Hosts: []string{"www.example.com"}}, []uint64{2}},
		{"text", tool.Filter{Text: "REFUSED"}, []uint64{3}},
		{"failures", tool.Filter{Failed: true}, []uint64{3}},
		{"the other source", tool.Filter{Source: tool.SourcePacket}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []uint64
			for _, row := range l.Rows(tt.filter) {
				got = append(got, row.ID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("rows %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFakeTrafficLogChanges: a new flow changes the version, and the rows
// are a snapshot that later flows do not change.
func TestFakeTrafficLogChanges(t *testing.T) {
	l := newFakeLog()
	l.add(tool.Flow{FlowRow: tool.FlowRow{Source: tool.SourceProxy, Host: "a.example.com"}})
	before := l.Version()
	rows := l.Rows(tool.Filter{})
	l.add(tool.Flow{FlowRow: tool.FlowRow{Source: tool.SourceProxy, Host: "b.example.com"}})
	if l.Version() == before {
		t.Error("the version did not change")
	}
	if len(rows) != 1 {
		t.Errorf("the snapshot changed: %d rows, want 1", len(rows))
	}
	if got := len(l.Rows(tool.Filter{})); got != 2 {
		t.Errorf("%d rows after adding one, want 2", got)
	}
}

// TestFakeTrafficCertCycles: installing the certificate trusts it, and
// removing it forgets it.
func TestFakeTrafficCertCycles(t *testing.T) {
	tr := NewTraffic()
	if cs, err := tr.Cert(); err != nil || cs.Exists {
		t.Fatalf("a new tool has a certificate: %+v, %v", cs, err)
	}
	if err := tr.InstallCert(context.Background()); err != nil {
		t.Fatal(err)
	}
	cs, _ := tr.Cert()
	if !cs.Exists || !cs.Trusted || cs.Fingerprint == "" || cs.Path == "" {
		t.Errorf("after installing: %+v", cs)
	}
	if err := tr.RemoveCert(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cs, _ := tr.Cert(); cs.Exists || cs.Trusted {
		t.Errorf("after removing: %+v", cs)
	}
}

// TestFakeTrafficLogFollowsMode: each mode has a log of its own, and the
// capture appends to the one in progress.
func TestFakeTrafficLogFollowsMode(t *testing.T) {
	tr := NewTraffic()
	proxy := tr.Log()
	if err := tr.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Stop() })
	tr.appendLive(time.Now())
	if tr.Log().Version() == 0 {
		t.Error("the proxy log did not change")
	}
	cfg := tr.Config()
	cfg.Mode = tool.ModePackets
	if err := tr.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	packets := tr.Log()
	if packets == proxy {
		t.Fatal("both modes share one log")
	}
	tr.appendLive(time.Now())
	if packets.Version() == 0 {
		t.Error("the packet log did not change")
	}
	if got := tr.Status().On; !got {
		t.Error("the capture stopped on its own")
	}
}

// seedKinds says which kinds of flow a made-up capture holds.
type seedKinds struct {
	failed, blocked, tunnel, replay, upgrade, json bool
}

// seedKindsOf reads the kinds of flow out of a made-up capture.
func seedKindsOf(flows []tool.Flow) seedKinds {
	var k seedKinds
	for _, f := range flows {
		switch {
		case f.State == tool.FlowFailed:
			k.failed = true
		case f.State == tool.FlowBlocked:
			k.blocked = true
		case f.Proto == "tunnel":
			k.tunnel = true
		case f.ReplayOf != 0:
			k.replay = true
		case f.Status == 101:
			k.upgrade = true
		case f.Type == mediaJSON:
			k.json = true
		}
	}
	return k
}

// TestFakeTrafficSeeds: the made-up capture holds the kinds a person looks
// for, so the page has something of each to show.
func TestFakeTrafficSeeds(t *testing.T) {
	tr := NewTraffic()
	flows := tr.proxy.snapshot(nil)
	if len(flows) < 100 {
		t.Fatalf("%d flows, want a few hundred", len(flows))
	}
	kinds := seedKindsOf(flows)
	if !kinds.failed || !kinds.blocked || !kinds.tunnel || !kinds.replay || !kinds.upgrade || !kinds.json {
		t.Errorf("canned flows: %+v, want one of every kind", kinds)
	}
	if len(tr.packets.Rows(tool.Filter{Source: tool.SourcePacket})) < 10 {
		t.Error("no canned packets")
	}
}
