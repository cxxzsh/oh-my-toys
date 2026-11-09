package flowlog

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// flowOf is a proxy flow to host with method, opened by the program named
// proc; an empty proc is a flow with no known program.
func flowOf(host, method, proc string) tool.Flow {
	return tool.Flow{
		FlowRow: tool.FlowRow{
			Source:  tool.SourceProxy,
			Method:  method,
			Host:    host,
			State:   tool.FlowDone,
			Process: tool.Program{Name: proc},
		},
	}
}

// facetsOf builds facets from alternating names and counts, as text.
func facetsOf(pairs ...string) []tool.Facet {
	out := make([]tool.Facet, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		n, err := strconv.Atoi(pairs[i+1])
		if err != nil {
			panic(err)
		}
		out = append(out, tool.Facet{Name: pairs[i], Count: n})
	}
	return out
}

func TestFacets(t *testing.T) {
	tests := []struct {
		name      string
		flows     []tool.Flow
		processes []tool.Facet
		hosts     []tool.Facet
	}{
		{
			name:      "empty log",
			processes: facetsOf(),
			hosts:     facetsOf(),
		},
		{
			name: "most flows first, then by name",
			flows: []tool.Flow{
				flowOf("b.test:443", "GET", "curl"),
				flowOf("a.test:443", "GET", "Safari"),
				flowOf("a.test:443", "GET", "curl"),
				flowOf("c.test:443", "GET", "Slack"),
			},
			processes: facetsOf("curl", "2", "Safari", "1", "Slack", "1"),
			hosts:     facetsOf("a.test:443", "2", "b.test:443", "1", "c.test:443", "1"),
		},
		{
			name: "names group ignoring case and keep the first spelling",
			flows: []tool.Flow{
				flowOf("Example.com:443", "GET", "Safari"),
				flowOf("example.COM:443", "GET", "safari"),
				flowOf("EXAMPLE.com:443", "GET", "SAFARI"),
			},
			processes: facetsOf("Safari", "3"),
			hosts:     facetsOf("Example.com:443", "3"),
		},
		{
			name: "flows with no program are left out of programs, not hosts",
			flows: []tool.Flow{
				flowOf("a.test:443", "GET", ""),
				flowOf("a.test:443", "GET", "curl"),
				flowOf("", "GET", ""),
			},
			processes: facetsOf("curl", "1"),
			hosts:     facetsOf("a.test:443", "2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(100, 1<<20)
			for _, f := range tt.flows {
				l.Put(f)
			}
			if got := l.Processes(); !reflect.DeepEqual(got, tt.processes) {
				t.Fatalf("Processes() = %+v, want %+v", got, tt.processes)
			}
			if got := l.Hosts(); !reflect.DeepEqual(got, tt.hosts) {
				t.Fatalf("Hosts() = %+v, want %+v", got, tt.hosts)
			}
		})
	}
}

func TestFacetsFollowEvictionAndClear(t *testing.T) {
	l := New(3, 1<<20)
	l.Put(flowOf("a.test:443", "GET", "curl"))
	l.Put(flowOf("b.test:443", "GET", "Safari"))
	l.Put(flowOf("b.test:443", "GET", "Safari"))
	// The fourth flow pushes the first, the only curl flow, out of the log.
	l.Put(flowOf("c.test:443", "GET", "Slack"))
	if got, want := l.Processes(), facetsOf("Safari", "2", "Slack", "1"); !reflect.DeepEqual(got, want) {
		t.Fatalf("after eviction Processes() = %+v, want %+v", got, want)
	}
	if got, want := l.Hosts(), facetsOf("b.test:443", "2", "c.test:443", "1"); !reflect.DeepEqual(got, want) {
		t.Fatalf("after eviction Hosts() = %+v, want %+v", got, want)
	}
	l.Clear()
	if got := l.Processes(); len(got) != 0 {
		t.Fatalf("after Clear Processes() = %+v, want none", got)
	}
	if got := l.Hosts(); len(got) != 0 {
		t.Fatalf("after Clear Hosts() = %+v, want none", got)
	}
}

func TestFacetsFollowUpdate(t *testing.T) {
	l := New(10, 1<<20)
	id := l.Put(flowOf("a.test:443", "GET", ""))
	f, _ := l.Get(id)
	f.Process = tool.Program{Name: "curl"}
	f.Host = "b.test:443"
	l.Put(f)
	if got, want := l.Processes(), facetsOf("curl", "1"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Processes() = %+v, want %+v", got, want)
	}
	if got, want := l.Hosts(), facetsOf("b.test:443", "1"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Hosts() = %+v, want %+v", got, want)
	}
}

func TestFacetsAreCopies(t *testing.T) {
	l := New(10, 1<<20)
	l.Put(flowOf("a.test:443", "GET", "curl"))
	got := l.Processes()
	got[0].Count = 99
	if again := l.Processes(); again[0].Count != 1 {
		t.Fatalf("caller changed the log's facets: %+v", again)
	}
}

// TestProcessesSurviveUpdate: a flow that is replaced, or read back, keeps
// the program that opened it.
func TestProcessesSurviveUpdate(t *testing.T) {
	l := New(10, 1<<20)
	id := l.Put(flowOf("a.test:443", "GET", "curl"))
	f, ok := l.Get(id)
	if !ok || f.Process.Name != "curl" {
		t.Fatalf("Get after Put: %+v, %v", f.Process, ok)
	}
	f.Info = "done"
	l.Put(f)
	rows := l.Rows(tool.Filter{Source: tool.SourceProxy})
	if len(rows) != 1 || rows[0].Process.Name != "curl" {
		t.Fatalf("Rows after update: %+v", rows)
	}
	if f, _ := l.Get(id); f.Process.Name != "curl" || f.Info != "done" {
		t.Fatalf("Get after update: %+v", f)
	}
}

func TestFilterProcesses(t *testing.T) {
	l := New(100, 1<<20)
	l.Put(flowOf("a.test:443", "GET", "Safari"))
	l.Put(flowOf("a.test:443", "POST", "curl"))
	l.Put(flowOf("b.test:443", "GET", "Slack"))
	l.Put(flowOf("b.test:443", "GET", ""))
	l.Put(tool.Flow{FlowRow: tool.FlowRow{Source: tool.SourcePacket, Process: tool.Program{Name: "Safari"}}})

	tests := []struct {
		name   string
		filter tool.Filter
		want   []uint64
	}{
		{"empty matches all proxy flows", tool.Filter{Source: tool.SourceProxy}, []uint64{1, 2, 3, 4}},
		{"ignores case", tool.Filter{Source: tool.SourceProxy, Processes: []string{"SAFARI"}}, []uint64{1}},
		{"any of", tool.Filter{Source: tool.SourceProxy, Processes: []string{"curl", "slack"}}, []uint64{2, 3}},
		{"unknown program matches none", tool.Filter{Source: tool.SourceProxy, Processes: []string{"Mail"}}, nil},
		{"with host", tool.Filter{Source: tool.SourceProxy, Processes: []string{"Safari", "Slack"}, Hosts: []string{"b.test"}}, []uint64{3}},
		{"with method", tool.Filter{Source: tool.SourceProxy, Processes: []string{"curl", "Safari"}, Methods: []string{"post"}}, []uint64{2}},
		{"with host and method, no match", tool.Filter{Source: tool.SourceProxy, Processes: []string{"Slack"}, Hosts: []string{"a.test"}, Methods: []string{"GET"}}, nil},
		{"packet source is separate", tool.Filter{Source: tool.SourcePacket, Processes: []string{"safari"}}, []uint64{5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idsOf(l.Rows(tt.filter))
			if len(got) != len(tt.want) || (len(got) > 0 && !reflect.DeepEqual(got, tt.want)) {
				t.Fatalf("IDs %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFilterProcessesCacheKey: Rows caches by filter, so a cached result
// must not answer for another program filter.
func TestFilterProcessesCacheKey(t *testing.T) {
	l := New(10, 1<<20)
	l.Put(flowOf("a.test:443", "GET", "Safari"))
	l.Put(flowOf("a.test:443", "GET", "curl"))
	if got := idsOf(l.Rows(tool.Filter{Source: tool.SourceProxy, Processes: []string{"Safari"}})); !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("Safari IDs %v", got)
	}
	if got := idsOf(l.Rows(tool.Filter{Source: tool.SourceProxy, Processes: []string{"curl"}})); !reflect.DeepEqual(got, []uint64{2}) {
		t.Fatalf("curl IDs %v", got)
	}
}
