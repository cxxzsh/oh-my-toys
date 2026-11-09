package flowlog

import (
	"slices"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// facetCount is how many flows of the log carry one name, and the spelling
// the first of them gave it.
type facetCount struct {
	name  string
	count int
}

// facetTally counts the flows of the log by a name, such as a host or a
// program. Names are grouped by their lower case, so a name written in
// another case joins the count of the one already there.
type facetTally map[string]*facetCount

// add moves the count of name by delta, up or down, and forgets a name
// that no flow carries any more. An empty name is no name and is left out.
func (t facetTally) add(name string, delta int) {
	if name == "" {
		return
	}
	key := strings.ToLower(name)
	c, ok := t[key]
	if !ok {
		if delta <= 0 {
			return
		}
		c = &facetCount{name: name}
		t[key] = c
	}
	c.count += delta
	if c.count <= 0 {
		delete(t, key)
	}
}

// facets lists the counts, most flows first and then by name.
func (t facetTally) facets() []tool.Facet {
	out := make([]tool.Facet, 0, len(t))
	for _, c := range t {
		out = append(out, tool.Facet{Name: c.name, Count: c.count})
	}
	slices.SortFunc(out, func(a, b tool.Facet) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// facetSnapshot is the listed programs and hosts of the log as they were at
// version.
type facetSnapshot struct {
	version   uint64
	processes []tool.Facet
	hosts     []tool.Facet
}

// tally counts f in (delta 1) or out (delta -1) of the programs and hosts
// the log lists.
func (l *Log) tally(f *tool.Flow, delta int) {
	l.processes.add(f.Process.Name, delta)
	l.hosts.add(f.Host, delta)
}

// Processes returns the programs that opened the flows of the log, and how
// many flows each opened, most flows first, then by name. Flows with no
// known program are left out.
func (l *Log) Processes() []tool.Facet {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.facetsLocked().processes)
}

// Hosts returns the hosts of the flows of the log with their counts, most
// flows first, then by name. Hosts are grouped ignoring case.
func (l *Log) Hosts() []tool.Facet {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.facetsLocked().hosts)
}

// facetsLocked returns the listed programs and hosts, sorted once per
// version of the log; the caller holds mu.
func (l *Log) facetsLocked() *facetSnapshot {
	if l.facets == nil || l.facets.version != l.version {
		l.facets = &facetSnapshot{
			version:   l.version,
			processes: l.processes.facets(),
			hosts:     l.hosts.facets(),
		}
	}
	return l.facets
}
