package flowlog

import (
	"slices"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// cachedRows is the last result of Rows, kept so that the next frame of
// the inspector costs nothing.
type cachedRows struct {
	version uint64
	filter  tool.Filter
	rows    []tool.FlowRow
}

// Rows returns the rows of the flows f picks, oldest first. The slice is a
// snapshot: the caller may keep it, and the log never changes it.
func (l *Log) Rows(f tool.Filter) []tool.FlowRow {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cache != nil && l.cache.version == l.version && sameFilter(l.cache.filter, f) {
		return l.cache.rows
	}
	rows := l.collect(f)
	l.cache = &cachedRows{version: l.version, filter: copyFilter(f), rows: rows}
	return rows
}

// collect matches every flow of the log against f.
func (l *Log) collect(f tool.Filter) []tool.FlowRow {
	c := newCriteria(f)
	rows := make([]tool.FlowRow, 0, l.order.Len())
	for e := l.order.Front(); e != nil; e = e.Next() {
		entry := e.Value.(*flowEntry)
		if c.matches(&entry.flow) {
			rows = append(rows, entry.flow.FlowRow)
		}
	}
	return rows
}

// Get returns the flow of id with everything it carried, or false once it
// has been evicted. The flow is a copy: the caller may change it.
func (l *Log) Get(id uint64) (tool.Flow, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.byID[id]
	if !ok {
		return tool.Flow{}, false
	}
	return cloneFlow(e.flow), true
}

// sameFilter reports whether two filters pick the same flows. The lists are
// compared in order: a reordered list misses the cache and matches again.
func sameFilter(a, b tool.Filter) bool {
	return a.Source == b.Source &&
		a.Text == b.Text &&
		a.Failed == b.Failed &&
		slices.Equal(a.Hosts, b.Hosts) &&
		slices.Equal(a.Methods, b.Methods) &&
		slices.Equal(a.Types, b.Types) &&
		slices.Equal(a.StatusClasses, b.StatusClasses)
}

// copyFilter copies the lists of f, so a caller that reuses its filter
// cannot change a cached key behind the log's back.
func copyFilter(f tool.Filter) tool.Filter {
	return tool.Filter{
		Source:        f.Source,
		Text:          f.Text,
		Hosts:         slices.Clone(f.Hosts),
		Methods:       slices.Clone(f.Methods),
		Types:         slices.Clone(f.Types),
		StatusClasses: slices.Clone(f.StatusClasses),
		Failed:        f.Failed,
	}
}

// cloneFlow copies the slices of f deeply enough that the caller cannot
// reach anything the log holds.
func cloneFlow(f tool.Flow) tool.Flow {
	out := f
	out.RuleHits = slices.Clone(f.RuleHits)
	out.Request = cloneMessage(f.Request)
	out.Response = cloneMessage(f.Response)
	if f.Packet != nil {
		out.Packet = clonePacket(f.Packet)
	}
	return out
}

func cloneMessage(m tool.HTTPMessage) tool.HTTPMessage {
	return tool.HTTPMessage{
		Headers: slices.Clone(m.Headers),
		Body: tool.Body{
			Type:      m.Body.Type,
			Data:      slices.Clone(m.Body.Data),
			Size:      m.Body.Size,
			Truncated: m.Body.Truncated,
		},
	}
}

func clonePacket(p *tool.PacketDetail) *tool.PacketDetail {
	out := &tool.PacketDetail{Raw: slices.Clone(p.Raw)}
	out.Layers = make([]tool.Layer, len(p.Layers))
	for i, layer := range p.Layers {
		out.Layers[i] = tool.Layer{Name: layer.Name, Fields: slices.Clone(layer.Fields)}
	}
	return out
}
