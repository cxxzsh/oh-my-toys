package flowlog

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// sizedFlow is a proxy flow whose response body has size bytes, so that the
// body budget can be counted, and the note names it.
func sizedFlow(size int, info string) tool.Flow {
	f := tool.Flow{
		FlowRow: tool.FlowRow{
			Source: tool.SourceProxy,
			Method: "GET",
			Host:   "example.com:443",
			State:  tool.FlowDone,
			Info:   info,
		},
	}
	f.Response.Body = tool.Body{
		Type: "text/plain",
		Data: bytes.Repeat([]byte("x"), size),
		Size: int64(size),
	}
	return f
}

// idsOf lists the IDs of rows, which is what an eviction order shows.
func idsOf(rows []tool.FlowRow) []uint64 {
	ids := make([]uint64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

func assertIDs(t *testing.T, rows []tool.FlowRow, want ...uint64) {
	t.Helper()
	got := idsOf(rows)
	if len(got) != len(want) {
		t.Fatalf("IDs %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("IDs %v, want %v", got, want)
		}
	}
}

func TestImplementsFlowLog(t *testing.T) {
	l := New(4, 1<<20)
	var log tool.FlowLog = l
	if id := l.Put(sizedFlow(1, "one")); id != 1 {
		t.Fatalf("first ID %d", id)
	}
	if log.Version() == 0 {
		t.Fatal("version did not change")
	}
	if rows := log.Rows(tool.Filter{}); len(rows) != 1 || rows[0].ID != 1 {
		t.Fatalf("rows %+v", rows)
	}
	if f, ok := log.Get(1); !ok || f.Info != "one" {
		t.Fatalf("get %+v, %v", f, ok)
	}
}

func TestPutAssignsIDsInOrder(t *testing.T) {
	log := New(10, 1<<20)
	for i, want := range []uint64{1, 2, 3} {
		if got := log.Put(sizedFlow(1, fmt.Sprintf("flow %d", i))); got != want {
			t.Fatalf("ID %d, want %d", got, want)
		}
	}
	assertIDs(t, log.Rows(tool.Filter{}), 1, 2, 3)
}

func TestPutReplacesByID(t *testing.T) {
	log := New(10, 1<<20)
	for _, info := range []string{"one", "two", "three"} {
		log.Put(sizedFlow(1, info))
	}
	version := log.Version()
	replaced := sizedFlow(2, "changed")
	replaced.ID = 2
	if id := log.Put(replaced); id != 2 {
		t.Fatalf("ID %d", id)
	}
	assertIDs(t, log.Rows(tool.Filter{}), 1, 2, 3)
	if f, ok := log.Get(2); !ok || f.Info != "changed" || len(f.Response.Body.Data) != 2 {
		t.Fatalf("get %+v, %v", f, ok)
	}
	if log.Version() == version {
		t.Fatal("version did not change")
	}
}

func TestPutAfterEvictionDoesNothing(t *testing.T) {
	log := New(1, 1<<20)
	log.Put(sizedFlow(1, "gone"))
	log.Put(sizedFlow(1, "kept"))
	late := sizedFlow(1, "late")
	late.ID = 1
	if id := log.Put(late); id != 1 {
		t.Fatalf("ID %d", id)
	}
	assertIDs(t, log.Rows(tool.Filter{}), 2)
	if _, ok := log.Get(1); ok {
		t.Fatal("a flow came back after eviction")
	}
}

func TestEvictsOldestByCapacity(t *testing.T) {
	log := New(2, 1<<20)
	for i := range 4 {
		log.Put(sizedFlow(1, fmt.Sprintf("flow %d", i)))
	}
	assertIDs(t, log.Rows(tool.Filter{}), 3, 4)
	if _, ok := log.Get(2); ok {
		t.Fatal("evicted flow still there")
	}
}

func TestEvictsToBodyBudget(t *testing.T) {
	log := New(10, 15)
	log.Put(sizedFlow(10, "first"))
	log.Put(sizedFlow(10, "second"))
	assertIDs(t, log.Rows(tool.Filter{}), 2)

	// One flow bigger than the whole budget keeps its row, without bodies.
	log.Put(sizedFlow(20, "huge"))
	assertIDs(t, log.Rows(tool.Filter{}), 2, 3)
	f, ok := log.Get(3)
	if !ok {
		t.Fatal("the flow just written was evicted")
	}
	if len(f.Response.Body.Data) != 0 || !f.Response.Body.Truncated || f.Response.Body.Size != 20 {
		t.Fatalf("body %+v", f.Response.Body)
	}
}

func TestClearForgetsFlowsButNotIDs(t *testing.T) {
	log := New(10, 1<<20)
	log.Put(sizedFlow(1, "one"))
	log.Put(sizedFlow(1, "two"))
	version := log.Version()
	log.Clear()
	if rows := log.Rows(tool.Filter{}); len(rows) != 0 {
		t.Fatalf("rows %+v", rows)
	}
	if log.Version() == version {
		t.Fatal("clear did not change the version")
	}
	if id := log.Put(sizedFlow(1, "after")); id != 3 {
		t.Fatalf("ID %d reused after clear", id)
	}
}

func TestGetReturnsACopy(t *testing.T) {
	log := New(10, 1<<20)
	f := sizedFlow(4, "copy")
	f.Request.Headers = []tool.Header{{Name: "X-Test", Value: "before"}}
	f.Request.Body = tool.Body{Type: "text/plain", Data: []byte("body")}
	f.Packet = &tool.PacketDetail{
		Raw:    []byte{1, 2, 3},
		Layers: []tool.Layer{{Name: "IP", Fields: []tool.Header{{Name: "ttl", Value: "64"}}}},
	}
	id := log.Put(f)
	got, ok := log.Get(id)
	if !ok {
		t.Fatal("the flow is missing")
	}
	got.Request.Body.Data[0] = 'X'
	got.Response.Body.Data[0] = 'X'
	got.Request.Headers[0].Value = "after"
	got.Packet.Raw[0] = 9
	got.Packet.Layers[0].Fields[0].Value = "1"

	again, _ := log.Get(id)
	if again.Request.Body.Data[0] != 'b' || again.Response.Body.Data[0] != 'x' {
		t.Fatal("the body changed behind the log's back")
	}
	if again.Request.Headers[0].Value != "before" || again.Packet.Raw[0] != 1 {
		t.Fatal("the flow changed behind the log's back")
	}
	if again.Packet.Layers[0].Fields[0].Value != "64" {
		t.Fatal("a packet layer changed behind the log's back")
	}
}

// TestBudgetKeepsTheFlowBeingWritten checks that replacing the oldest flow
// with a bigger one evicts a newer flow instead, so the person never loses
// the flow they were just looking at.
func TestBudgetKeepsTheFlowBeingWritten(t *testing.T) {
	log := New(10, 10)
	log.Put(sizedFlow(4, "first"))
	log.Put(sizedFlow(4, "second"))
	log.Put(sizedFlow(4, "third"))
	assertIDs(t, log.Rows(tool.Filter{}), 2, 3)

	bigger := sizedFlow(8, "second")
	bigger.ID = 2
	log.Put(bigger)
	assertIDs(t, log.Rows(tool.Filter{}), 2)
	if f, ok := log.Get(3); ok {
		t.Fatalf("the newer flow stayed: %+v", f)
	}
}

// packetFlow is a packet whose bytes count toward the body budget.
func packetFlow(raw int) tool.Flow {
	return tool.Flow{
		FlowRow: tool.FlowRow{Source: tool.SourcePacket, Host: "8.8.8.8:53", Proto: "DNS"},
		Packet:  &tool.PacketDetail{Raw: bytes.Repeat([]byte{0}, raw)},
	}
}

func TestPacketBytesCountTowardTheBudget(t *testing.T) {
	log := New(10, 5)
	log.Put(packetFlow(4))
	log.Put(packetFlow(4))
	assertIDs(t, log.Rows(tool.Filter{Source: tool.SourcePacket}), 2)

	// A packet over the whole budget keeps its row, without its bytes.
	log.Put(packetFlow(8))
	assertIDs(t, log.Rows(tool.Filter{Source: tool.SourcePacket}), 2, 3)
	f, ok := log.Get(3)
	if !ok || f.Packet == nil || len(f.Packet.Raw) != 0 {
		t.Fatalf("packet %+v, %v", f.Packet, ok)
	}
}

func TestLenFollowsTheLog(t *testing.T) {
	log := New(2, 1<<20)
	if n := log.Len(); n != 0 {
		t.Fatalf("Len %d of an empty log", n)
	}
	log.Put(sizedFlow(1, "one"))
	log.Put(sizedFlow(1, "two"))
	if n := log.Len(); n != 2 {
		t.Fatalf("Len %d after two flows", n)
	}
	log.Put(sizedFlow(1, "three"))
	if n := log.Len(); n != 2 {
		t.Fatalf("Len %d after eviction", n)
	}
	log.Clear()
	if n := log.Len(); n != 0 {
		t.Fatalf("Len %d after clear", n)
	}
}
