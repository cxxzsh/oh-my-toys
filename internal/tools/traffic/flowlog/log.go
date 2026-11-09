// Package flowlog keeps what a traffic tool recorded: the rows the
// inspector lists and the flows it opens. One Log holds the flows of one
// source; the proxy writes to one and the packet reader to another.
//
// Put runs on the capture goroutines and Rows on the frame's, so Rows
// keeps its last result per (version, filter): asking again while nothing
// changed returns the same slice, and matching runs only after a change.
package flowlog

import (
	"container/list"
	"sync"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Log is the flows a traffic tool recorded, safe for use from any
// goroutine.
//
// IDs start at 1 and are never reused, not even after Clear, so a row the
// inspector holds still names the flow it showed. Put takes the flow it is
// given, bodies included; the caller must not write to those slices again.
// The log keeps one goroutine while a subscriber exists, and it ends when
// the last one stops.
type Log struct {
	mu           sync.Mutex
	capacity     int
	maxBodyBytes int64
	nextID       uint64
	order        *list.List
	byID         map[uint64]*flowEntry
	bodyBytes    int64
	version      uint64
	cache        *cachedRows
	processes    facetTally
	hosts        facetTally
	facets       *facetSnapshot
	subscribers  []*subscriber
	wake         chan struct{}
	quit         chan struct{}
	watching     bool
}

// flowEntry is one flow of the log, with its place in the order and the
// space its bodies take.
type flowEntry struct {
	flow  tool.Flow
	bytes int64
	elem  *list.Element
}

// New makes an empty log that keeps maxFlows flows and maxBodyBytes bytes
// of bodies and packet bytes, evicting the oldest first. A maxFlows below
// 1 is taken as 1, and a maxBodyBytes below 0 as 0, which keeps no body.
func New(maxFlows int, maxBodyBytes int64) *Log {
	if maxFlows < 1 {
		maxFlows = 1
	}
	if maxBodyBytes < 0 {
		maxBodyBytes = 0
	}
	return &Log{
		capacity:     maxFlows,
		maxBodyBytes: maxBodyBytes,
		order:        list.New(),
		byID:         make(map[uint64]*flowEntry, maxFlows),
		processes:    facetTally{},
		hosts:        facetTally{},
		wake:         make(chan struct{}, 1),
	}
}

// Put records f. With f.ID == 0 it assigns the next ID and appends the
// flow; otherwise it replaces the flow of that ID, and does nothing when
// that flow has been evicted. It returns the ID of the flow.
func (l *Log) Put(f tool.Flow) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.version++
	l.trim(&f)
	if f.ID == 0 {
		l.nextID++
		f.ID = l.nextID
		l.evict(l.append(f))
	} else if written := l.replace(f); written != nil {
		l.evict(written)
	}
	l.notify()
	return f.ID
}

// append adds f, whose ID is already fixed, as the newest flow.
func (l *Log) append(f tool.Flow) *flowEntry {
	e := &flowEntry{flow: f, bytes: bodyBytes(f)}
	l.tally(&f, 1)
	e.elem = l.order.PushBack(e)
	l.byID[f.ID] = e
	l.bodyBytes += e.bytes
	return e
}

// replace swaps the flow of f.ID for f and returns its entry, or nil when
// the flow is gone.
func (l *Log) replace(f tool.Flow) *flowEntry {
	// A caller may hand in an ID of its own; assignment must not later
	// reuse it.
	if f.ID >= l.nextID {
		l.nextID = f.ID
	}
	e, ok := l.byID[f.ID]
	if !ok {
		return nil
	}
	size := bodyBytes(f)
	l.tally(&e.flow, -1)
	l.tally(&f, 1)
	l.bodyBytes += size - e.bytes
	e.flow = f
	e.bytes = size
	return e
}

// trim drops every body of f when the flow alone is bigger than the whole
// body budget, because keeping a part of it would evict every other flow
// and still not fit. The bodies that lost their bytes say they did.
func (l *Log) trim(f *tool.Flow) {
	if bodyBytes(*f) <= l.maxBodyBytes {
		return
	}
	dropBody(&f.Request.Body)
	dropBody(&f.Response.Body)
	if f.Packet != nil && len(f.Packet.Raw) > 0 {
		// The packet detail is the caller's until now, so copy it before
		// cutting its bytes.
		packet := *f.Packet
		packet.Raw = nil
		f.Packet = &packet
	}
}

// dropBody forgets the bytes of b, keeping its size and type.
func dropBody(b *tool.Body) {
	if len(b.Data) == 0 {
		return
	}
	b.Data = nil
	b.Truncated = true
}

// evict brings the log back inside its limits, oldest first, but never
// drops the flow just written: a flow the person is looking at stays.
func (l *Log) evict(written *flowEntry) {
	for l.order.Len() > l.capacity {
		victim := l.oldest(written)
		if victim == nil {
			return
		}
		l.remove(victim)
	}
	for l.bodyBytes > l.maxBodyBytes {
		victim := l.oldest(written)
		if victim == nil {
			return
		}
		l.remove(victim)
	}
}

// oldest returns the oldest flow but skip, or nil when there is none.
func (l *Log) oldest(skip *flowEntry) *flowEntry {
	for e := l.order.Front(); e != nil; e = e.Next() {
		entry := e.Value.(*flowEntry)
		if entry != skip {
			return entry
		}
	}
	return nil
}

// remove drops e from the log and from the body budget.
func (l *Log) remove(e *flowEntry) {
	l.order.Remove(e.elem)
	delete(l.byID, e.flow.ID)
	l.tally(&e.flow, -1)
	l.bodyBytes -= e.bytes
	l.version++
}

// Version changes whenever the log does, so a reader asks for the rows
// again only when it has changed.
func (l *Log) Version() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.version
}

// Len returns how many flows the log holds now, for a status line that must
// not build the rows to count them.
func (l *Log) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}

// Clear forgets every flow. The IDs of those flows are not handed out
// again.
func (l *Log) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.processes = facetTally{}
	l.hosts = facetTally{}
	l.order.Init()
	l.byID = make(map[uint64]*flowEntry, l.capacity)
	l.bodyBytes = 0
	l.cache = nil
	l.version++
	l.notify()
}

// bodyBytes counts what a flow adds to the body budget: both bodies and
// the bytes of a packet.
func bodyBytes(f tool.Flow) int64 {
	n := int64(len(f.Request.Body.Data) + len(f.Response.Body.Data))
	if f.Packet != nil {
		n += int64(len(f.Packet.Raw))
	}
	return n
}
