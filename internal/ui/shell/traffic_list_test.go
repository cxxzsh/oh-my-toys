package shell

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// kindsOf lists the cells of the columns, in order.
func kindsOf(cols []trafficColumn) []int {
	kinds := make([]int, 0, len(cols))
	for _, col := range cols {
		kinds = append(kinds, col.kind)
	}
	return kinds
}

// TestTrafficDropOrder: as the list narrows, the columns go one at a time in
// trafficDropOrder, and the column the list cannot fit is the one dropped.
func TestTrafficDropOrder(t *testing.T) {
	st := newTrafficState()
	cols := st.trafficColumns(1e6)
	for _, kind := range trafficDropOrder {
		want := slices.DeleteFunc(kindsOf(cols), func(k int) bool { return k == kind })
		cols = st.trafficColumns(trafficNeed(cols) - 1)
		if got := kindsOf(cols); !slices.Equal(got, want) {
			t.Fatalf("narrowing past %v dropped %v, want %v", kind, got, want)
		}
	}
}

// TestTrafficKeyColumnsStay: no width drops #, Method, Status or the URL.
func TestTrafficKeyColumnsStay(t *testing.T) {
	st := newTrafficState()
	got := kindsOf(st.trafficColumns(0))
	want := []int{cellID, cellMethod, cellStatus, cellURL}
	if !slices.Equal(got, want) {
		t.Errorf("columns at zero width are %v, want %v", got, want)
	}
}

func TestMethodColor(t *testing.T) {
	p := &ds.Light
	cases := map[string]ui.Color{
		"GET":    p.Success,
		"POST":   p.AccentText,
		"PUT":    p.Warning,
		"DELETE": p.Danger,
		"PATCH":  p.TextPrimary,
	}
	for method, want := range cases {
		if got := methodColor(p, method); got != want {
			t.Errorf("%s colour %v, want %v", method, got, want)
		}
	}
}

// TestRowTextFollowsStatus: the cached text of a row changes when the flow
// does, so the list never shows a status from before the response came.
func TestRowTextFollowsStatus(t *testing.T) {
	st := newTrafficState()
	r := tool.FlowRow{ID: 7, State: tool.FlowOpen}
	if got := st.textOf(r).status; got != i18n.TrafficOpen() {
		t.Errorf("open flow shows %q, want %q", got, i18n.TrafficOpen())
	}
	r.State, r.Status = tool.FlowDone, 200
	if got := st.textOf(r).status; got != "200" {
		t.Errorf("answered flow shows %q, want 200", got)
	}
}

// TestSelectNewestFollowsArrivals: Auto select chooses each new flow until a
// person picks a row, and then leaves the choice alone.
func TestSelectNewestFollowsArrivals(t *testing.T) {
	st := newTrafficState()
	st.rows = []tool.FlowRow{{ID: 1}, {ID: 2}}
	st.selectNewest()
	if st.sel != 1 {
		t.Fatalf("auto select chose %d, want the newest row 1", st.sel)
	}
	st.rows = append(st.rows, tool.FlowRow{ID: 3})
	st.selectNewest()
	if st.sel != 2 {
		t.Fatalf("auto select chose %d after an arrival, want 2", st.sel)
	}
	st.picked = true
	st.rows = append(st.rows, tool.FlowRow{ID: 4})
	st.selectNewest()
	if st.sel != 2 {
		t.Errorf("auto select moved a picked row to %d, want 2", st.sel)
	}
}
