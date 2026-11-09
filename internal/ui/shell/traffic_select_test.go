package shell

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// addFlow records a flow in the test log and returns it with its ID.
func addFlow(f *testTraffic, flow tool.Flow) tool.Flow {
	flow.ID = f.proxy.add(flow)
	return flow
}

// detailShows reports whether the detail pane shows the URL of a flow. The
// list shows URLs without their scheme, so the scheme-qualified text is only
// in the detail.
func detailShows(tt *ui.Tester, url string) bool {
	return tt.HasText("https://" + url)
}

// clickText clicks the element whose text is s, then draws a frame.
func clickText(t *testing.T, tt *ui.Tester, s string) {
	t.Helper()
	if err := tt.Click(s); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

// search narrows the list by typing text into the search field.
func search(t *testing.T, tt *ui.Tester, text string) {
	t.Helper()
	clickText(t, tt, i18n.TrafficSearch())
	tt.Type(text)
	tt.Frame()
}

// TestTrafficFilterKeepsChosenFlow: a filter that still holds the chosen flow
// keeps it chosen, and its detail stays on screen.
func TestTrafficFilterKeepsChosenFlow(t *testing.T) {
	f := newTestTraffic()
	addFlow(f, testGET("one.example.com", "/one", 200))
	second := addFlow(f, testGET("two.example.com", "/second", 200))
	addFlow(f, testGET("two.example.com", "/third", 200))
	tt := openTraffic(t, f)
	clickText(t, tt, trafficRowLabel(second.FlowRow))
	search(t, tt, "two.example")
	if !detailShows(tt, "two.example.com/second") {
		t.Errorf("the filter lost the chosen flow's detail: %q", tt.Texts())
	}
}

// TestTrafficFilterExcludesUnpickedFlow: with Auto select on and no row picked
// by hand, a filter that drops the chosen flow chooses the newest flow left.
func TestTrafficFilterExcludesUnpickedFlow(t *testing.T) {
	f := newTestTraffic()
	addFlow(f, testGET("one.example.com", "/one", 200))
	addFlow(f, testGET("two.example.com", "/two", 200))
	tt := openTraffic(t, f)
	if !detailShows(tt, "two.example.com/two") {
		t.Fatalf("auto select did not choose the newest flow: %q", tt.Texts())
	}
	search(t, tt, "one.example")
	if !detailShows(tt, "one.example.com/one") {
		t.Errorf("the newest flow left was not chosen: %q", tt.Texts())
	}
}

// TestTrafficFilterExcludesPickedFlow: a row picked by hand is cleared when a
// filter drops it, and the hint takes its place.
func TestTrafficFilterExcludesPickedFlow(t *testing.T) {
	f := newTestTraffic()
	addFlow(f, testGET("one.example.com", "/one", 200))
	two := addFlow(f, testGET("two.example.com", "/two", 200))
	tt := openTraffic(t, f)
	clickText(t, tt, trafficRowLabel(two.FlowRow))
	search(t, tt, "one.example")
	if !tt.HasText(i18n.TrafficNoSelection()) {
		t.Errorf("the hint is not shown after the picked flow was filtered out: %q", tt.Texts())
	}
	if detailShows(tt, "two.example.com/two") {
		t.Errorf("the filtered-out flow is still in the detail: %q", tt.Texts())
	}
}

// TestTrafficArrivalKeepsChosenFlow: a new flow does not move a flow that a
// person picked by hand.
func TestTrafficArrivalKeepsChosenFlow(t *testing.T) {
	f := newTestTraffic()
	one := addFlow(f, testGET("one.example.com", "/one", 200))
	addFlow(f, testGET("two.example.com", "/two", 200))
	tt := openTraffic(t, f)
	clickText(t, tt, trafficRowLabel(one.FlowRow))
	addFlow(f, testGET("two.example.com", "/three", 200))
	tt.Frame()
	if !detailShows(tt, "one.example.com/one") {
		t.Errorf("an arrival moved the chosen flow: %q", tt.Texts())
	}
}
