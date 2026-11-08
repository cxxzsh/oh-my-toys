package shell

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// clickFlows opens the inspector on a book of a few flows and returns the
// tester with the flows in place.
func clickFlows(t *testing.T) (*testTraffic, *ui.Tester, []tool.Flow) {
	t.Helper()
	f := newTestTraffic()
	flows := make([]tool.Flow, 4)
	for i := range flows {
		flows[i] = testGET("api.example.com", "/flow"+string(rune('a'+i)), 200)
		flows[i].ID = f.proxy.add(flows[i])
	}
	return f, openTraffic(t, f), flows
}

// TestTrafficSecondClickSelects: after one flow is chosen, clicking
// another chooses it too, and the detail follows.
func TestTrafficSecondClickSelects(t *testing.T) {
	_, tt, flows := clickFlows(t)
	if err := tt.Click(trafficRowLabel(flows[0].FlowRow)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("https://api.example.com/flowa") {
		t.Fatalf("the first click did not show the first flow: %q", tt.Texts())
	}
	if err := tt.Click(trafficRowLabel(flows[2].FlowRow)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("https://api.example.com/flowc") {
		t.Errorf("the second click did not choose the third flow: %q", tt.Texts())
	}
}

// TestTrafficClickSelectsAsFlowsArrive: a flow arriving between the
// clicks does not pull the choice back to the flow shown before.
func TestTrafficClickSelectsAsFlowsArrive(t *testing.T) {
	f, tt, flows := clickFlows(t)
	if err := tt.Click(trafficRowLabel(flows[1].FlowRow)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	fresh := testGET("api.example.com", "/newest", 200)
	f.proxy.add(fresh)
	tt.Frame()
	if err := tt.Click(trafficRowLabel(flows[3].FlowRow)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("https://api.example.com/flowd") {
		t.Errorf("a flow arriving between the clicks kept the old choice: %q", tt.Texts())
	}
}

// TestTrafficRowClickKeepsTheTab: clicking rows never leaves the Flows
// tab for the rule book.
func TestTrafficRowClickKeepsTheTab(t *testing.T) {
	_, tt, flows := clickFlows(t)
	for _, f := range flows {
		if err := tt.Click(trafficRowLabel(f.FlowRow)); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !tt.HasText(i18n.TrafficRulesHelp()) {
			continue // still on the flows tab, as it must be
		}
		t.Fatalf("a click on %s switched to the Rules tab", f.Path)
	}
}
