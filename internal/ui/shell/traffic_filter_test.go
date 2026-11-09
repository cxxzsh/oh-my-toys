package shell

import (
	"reflect"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// filterRows are two flows, one per program and host, that the query tests
// narrow.
func filterRows() []tool.FlowRow {
	return []tool.FlowRow{
		{ID: 1, Method: "POST", Host: "api.example.com", Process: tool.Program{Name: "Safari"}, Status: 404, Type: "application/json"},
		{ID: 2, Method: "GET", Host: "cdn.example.net", Process: tool.Program{Name: "Chrome"}, Status: 200, Type: "text/html"},
	}
}

// ids lists the IDs of rows, for comparing the result of a narrowing.
func ids(rows []tool.FlowRow) []uint64 {
	out := make([]uint64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// TestNarrowRowsByToken: each token keeps the rows it names, and the tokens
// of one query all have to hold.
func TestNarrowRowsByToken(t *testing.T) {
	tests := []struct {
		text string
		want []uint64
	}{
		{text: "", want: []uint64{1, 2}},
		{text: "program:safari", want: []uint64{1}},
		{text: "program:Chrome", want: []uint64{2}},
		{text: "method:get", want: []uint64{2}},
		{text: "method:POST,GET", want: []uint64{1, 2}},
		{text: "status:4xx", want: []uint64{1}},
		{text: "status:none", want: []uint64{}},
		{text: "host:*.example.net", want: []uint64{2}},
		{text: "type:html", want: []uint64{2}},
		{text: "type:json program:Chrome", want: []uint64{}},
		{text: "host:api.example.com type:json status:4xx", want: []uint64{1}},
	}
	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			st := &trafficState{text: tc.text}
			if got := ids(st.narrowRows(filterRows())); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("narrowRows(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestHostFacetAndTokenIntersect: the sidebar's host goes to the log and the
// host token narrows the rows the log returns, so both name the flows kept.
// When they name different hosts, nothing is left.
func TestHostFacetAndTokenIntersect(t *testing.T) {
	rows := filterRows()
	st := &trafficState{text: "host:api.example.com", hostSel: "api.example.com"}
	f := st.withFacets(st.makeFilter())
	if !reflect.DeepEqual(f.Hosts, []string{"api.example.com"}) {
		t.Fatalf("log filter Hosts = %v, want the sidebar host", f.Hosts)
	}
	if got := ids(st.narrowRows([]tool.FlowRow{rows[0]})); !reflect.DeepEqual(got, []uint64{1}) {
		t.Errorf("same host named twice: got %v, want [1]", got)
	}

	st.hostSel = "cdn.example.net"
	f = st.withFacets(st.makeFilter())
	if !reflect.DeepEqual(f.Hosts, []string{"cdn.example.net"}) {
		t.Fatalf("log filter Hosts = %v, want the sidebar host", f.Hosts)
	}
	if got := st.narrowRows([]tool.FlowRow{rows[1]}); len(got) != 0 {
		t.Errorf("different hosts: got %d rows, want none", len(got))
	}
}

// TestMakeFilterFromChoices: the popover's choices go to the log, and the
// plain words become the log's text.
func TestMakeFilterFromChoices(t *testing.T) {
	st := &trafficState{text: "login status:4xx program:Safari"}
	st.methods[1] = true // POST
	st.classes[3] = true // 4xx
	f := st.makeFilter()
	if !reflect.DeepEqual(f.Methods, []string{"POST"}) {
		t.Errorf("Methods = %v, want [POST]", f.Methods)
	}
	if !reflect.DeepEqual(f.StatusClasses, []int{4}) {
		t.Errorf("StatusClasses = %v, want [4]", f.StatusClasses)
	}
	if f.Text != "login" {
		t.Errorf("Text = %q, want %q", f.Text, "login")
	}
}

// TestFilterActive: a query, a choice or Failed only narrows the list; the
// empty filter does not.
func TestFilterActive(t *testing.T) {
	if (&trafficState{}).filterActive() {
		t.Error("the empty filter is active")
	}
	if !(&trafficState{text: "  host:x  "}).filterActive() {
		t.Error("a query is not active")
	}
	if !(&trafficState{failed: true}).filterActive() {
		t.Error("Failed only is not active")
	}
}
