package shell

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// rowMenu opens the context menu of one row and chooses the item named label.
func rowMenu(t *testing.T, tt *ui.Tester, row tool.FlowRow, label string) {
	t.Helper()
	if err := tt.RightClick(trafficRowLabel(row)); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(label); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

// proxyFlow is a captured flow of the Safari program, with its ID set.
func proxyFlow(f *testTraffic) tool.Flow {
	flow := testGET("api.example.com", "/v1/items", 200)
	flow.Process = tool.Program{Name: "Safari"}
	flow.ID = f.proxy.add(flow)
	return flow
}

// TestRowMenuReplay: Repeat sends the flow back through the tool with its ID.
func TestRowMenuReplay(t *testing.T) {
	f := newTestTraffic()
	flow := proxyFlow(f)
	tt := openTraffic(t, f)
	rowMenu(t, tt, flow.FlowRow, i18n.TrafficReplay())
	deadline := time.Now().Add(trafficWait)
	for !slices.Contains(f.replays, flow.ID) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		tt.Frame()
	}
	if !slices.Contains(f.replays, flow.ID) {
		t.Fatalf("replays = %v, want %d", f.replays, flow.ID)
	}
}

// TestRowMenuBlockHostOnce: Block this host appends one enabled block rule for
// the host, and a second choice adds no duplicate.
func TestRowMenuBlockHostOnce(t *testing.T) {
	f := newTestTraffic()
	flow := proxyFlow(f)
	tt := openTraffic(t, f)
	rowMenu(t, tt, flow.FlowRow, i18n.TrafficMenuBlockHost())
	rowMenu(t, tt, flow.FlowRow, i18n.TrafficMenuBlockHost())

	var blocks []tool.Rule
	for _, r := range f.Rules() {
		if r.Kind == tool.RuleBlock {
			blocks = append(blocks, r)
		}
	}
	if len(blocks) != 1 {
		t.Fatalf("block rules = %d, want 1: %+v", len(blocks), blocks)
	}
	r := blocks[0]
	if !r.Enabled || r.Name != "Block api.example.com" || r.Match.Pattern != "*://api.example.com/*" {
		t.Fatalf("block rule = %+v", r)
	}
}

// TestRowMenuDoNotDecryptOnce: Do not decrypt adds the host to the bypass list
// once.
func TestRowMenuDoNotDecryptOnce(t *testing.T) {
	f := newTestTraffic()
	flow := proxyFlow(f)
	tt := openTraffic(t, f)
	rowMenu(t, tt, flow.FlowRow, i18n.TrafficMenuDecrypt())
	rowMenu(t, tt, flow.FlowRow, i18n.TrafficMenuDecrypt())
	if got := f.Config().Bypass; !slices.Equal(got, []string{"api.example.com"}) {
		t.Fatalf("Bypass = %v, want [api.example.com]", got)
	}
}

// TestRowMenuCaptureOnlyProgram: Capture only this program scopes the capture
// to the flow's program.
func TestRowMenuCaptureOnlyProgram(t *testing.T) {
	f := newTestTraffic()
	flow := proxyFlow(f)
	tt := openTraffic(t, f)
	rowMenu(t, tt, flow.FlowRow, i18n.TrafficMenuCaptureProgram())
	if got := f.Config().Scope; !slices.Equal(got, []string{"Safari"}) {
		t.Fatalf("Scope = %v, want [Safari]", got)
	}
}
