package shell

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// pollEvery is how often waitUntil redraws and checks its condition.
const pollEvery = 5 * time.Millisecond

// testFrom is a proxied flow opened by the named program.
func testFrom(flow tool.Flow, name string) tool.Flow {
	flow.Process = tool.Program{Name: name}
	return flow
}

// waitUntil redraws the page until cond holds, or fails the test.
func waitUntil(t *testing.T, tt *ui.Tester, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(trafficWait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		tt.Frame()
		time.Sleep(pollEvery)
	}
}

// clickOK clicks label and fails the test when nothing answers to it.
func clickOK(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("click %q: %v", label, err)
	}
}

// TestTrafficSidebarNarrowsByProgram: a click on a program in the sidebar
// keeps the flows of that program and no others.
func TestTrafficSidebarNarrowsByProgram(t *testing.T) {
	f := newTestTraffic()
	f.proxy.add(testFrom(testGET("api.example.com", "/alpha-path", 200), "alpha-app"))
	f.proxy.add(testFrom(testGET("api.example.com", "/beta-path", 200), "beta-app"))
	tt := openTraffic(t, f)
	clickOK(t, tt, "beta-app")
	waitUntil(t, tt, func() bool { return !tt.HasText("/alpha-path") }, "the alpha flow to leave the list")
	if !tt.HasText("/beta-path") {
		t.Errorf("the beta flow is not shown: %q", tt.Texts())
	}
}

// TestTrafficScopeChipClears: a scope shows as a chip, and a click on the
// chip records every program again.
func TestTrafficScopeChipClears(t *testing.T) {
	f := newTestTraffic()
	f.cfg.Scope = []string{"alpha-app"}
	f.proxy.add(testFrom(testGET("api.example.com", "/alpha-path", 200), "alpha-app"))
	tt := openTraffic(t, f)
	label := i18n.TrafficScopeChip("alpha-app")
	if !tt.HasText(label) {
		t.Fatalf("the scope chip %q is not shown: %q", label, tt.Texts())
	}
	clickOK(t, tt, label)
	waitUntil(t, tt, func() bool { return len(f.Config().Scope) == 0 }, "the scope to clear")
}

// TestTrafficOpenTerminalRuns: with the proxy on, the terminal opens.
func TestTrafficOpenTerminalRuns(t *testing.T) {
	f := newTestTraffic()
	f.on = true
	tt := openTraffic(t, f)
	clickOK(t, tt, i18n.TrafficSettings())
	clickOK(t, tt, i18n.TrafficOpenTerminal())
	waitUntil(t, tt, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.terminals == 1
	}, "the terminal to open")
}

// TestTrafficOpenTerminalNeedsProxy: with the proxy off, the page says why
// the terminal did not open.
func TestTrafficOpenTerminalNeedsProxy(t *testing.T) {
	f := newTestTraffic()
	tt := openTraffic(t, f)
	clickOK(t, tt, i18n.TrafficSettings())
	clickOK(t, tt, i18n.TrafficOpenTerminal())
	waitUntil(t, tt, func() bool { return tt.HasText(i18n.TrafficErrNotRunningTitle()) }, "the not-running problem")
}

// TestTrafficGuideShowsOnEmptyLog: a new user with nothing captured sees the
// guide; a flow in the log takes it away.
func TestTrafficGuideShowsOnEmptyLog(t *testing.T) {
	f := newTestTraffic()
	tt := openTraffic(t, f)
	if !tt.HasText(i18n.TrafficGuideTitle()) {
		t.Errorf("the guide is not shown on an empty log: %q", tt.Texts())
	}
	withFlow := newTestTraffic()
	withFlow.proxy.add(testGET("api.example.com", "/x", 200))
	tt = openTraffic(t, withFlow)
	if tt.HasText(i18n.TrafficGuideTitle()) {
		t.Errorf("the guide is shown beside a flow: %q", tt.Texts())
	}
}

// TestTrafficToolbarDoesNotJump: the controls at the right of the toolbar
// stay where they are whatever the capture is doing, so a click lands where
// the eye expects after a start, a note or a scope.
func TestTrafficToolbarDoesNotJump(t *testing.T) {
	labels := []string{i18n.TrafficSettings()}
	states := map[string]func(f *testTraffic){
		"idle":    func(*testTraffic) {},
		"running": func(f *testTraffic) { f.on = true },
		"note": func(f *testTraffic) {
			f.on = true
			f.note = "apps read another service's proxy"
		},
		"scope": func(f *testTraffic) {
			f.on = true
			f.cfg.Scope = []string{"alpha-app"}
		},
	}
	var want map[string]ui.Rect
	for name, set := range states {
		f := newTestTraffic()
		set(f)
		tt := openTraffic(t, f)
		got := map[string]ui.Rect{}
		for _, label := range labels {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("%s: %q is not on the toolbar: %q", name, label, tt.Texts())
			}
			got[label] = r
		}
		if want == nil {
			want = got
			continue
		}
		for _, label := range labels {
			if got[label].X != want[label].X || got[label].Y != want[label].Y {
				t.Errorf("%s: %q moved from %v to %v", name, label, want[label], got[label])
			}
		}
	}
}

// wideURLWidth is the least width the URL column gets in a 1600 px window,
// where the list has the whole pane under the detail.
const wideURLWidth = 300

// TestTrafficProgramNameFullAtWideWidth checks, at 1600 px, that the Program
// column shows its name whole and is its fixed width. The column widths are
// read from where the headers start.
func TestTrafficProgramNameFullAtWideWidth(t *testing.T) {
	const path = "/api/v2/some/long/path/for/the/url"
	f := newTestTraffic()
	f.proxy.add(testFrom(testGET("www.example.com", path, 200), "Code Helper"))
	tt := openTrafficAt(t, f, 1600)
	waitUntil(t, tt, func() bool { return tt.HasText(path) }, "the URL row")
	// The sidebar lists the program too: a whole name shows twice, a cut one
	// only once.
	shown := 0
	for _, s := range tt.Texts() {
		if s == "Code Helper" {
			shown++
		}
	}
	if shown < 2 {
		t.Errorf("program name shown %d times, want the row and the sidebar", shown)
	}
	prog := headerX(t, tt, i18n.TrafficColProgram())
	started := headerX(t, tt, i18n.TrafficColTime())
	if d := started - prog - trafficWidthProgram; d < 0 || d > float32(ds.Space4) {
		t.Errorf("Program column is %v px, want %d px", started-prog, trafficWidthProgram)
	}
	// At this width every column is shown, so Type's header must be on the page.
	headerX(t, tt, i18n.TrafficColType())
	url := headerX(t, tt, i18n.TrafficColURL())
	status := headerX(t, tt, i18n.StatusLabel())
	if w := status - url; w < wideURLWidth {
		t.Errorf("URL column is %v px, want at least %d px", w, wideURLWidth)
	}
}

// headerX is where the column header label starts on the page.
func headerX(t *testing.T, tt *ui.Tester, label string) float32 {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("column header %q is not on the page", label)
	}
	return r.X
}

// TestTrafficDetailBesideListAtWideWindow checks that a window wide enough for
// side-by-side panes leaves the detail pane at least 400 px wide. Its empty
// state text starts at the pane's left edge, so the distance from there to the
// window's right edge bounds the pane's width from below.
func TestTrafficDetailBesideListAtWideWindow(t *testing.T) {
	const width = 2200
	const minDetail = 400
	tt := openTrafficAt(t, newTestTraffic(), width)
	r, ok := tt.Find(i18n.TrafficNoSelection())
	if !ok {
		t.Fatalf("detail empty state is not on the page")
	}
	if got := width - r.X; got < minDetail {
		t.Errorf("detail pane is about %v px wide, want at least %d px", got, minDetail)
	}
}

// TestTrafficURLKeepsMinimumAtNarrowWidth checks that a single flow in a
// 1024 px window keeps the URL column at least trafficURLMin wide, and that the
// host and path are shown whole in it.
func TestTrafficURLKeepsMinimumAtNarrowWidth(t *testing.T) {
	const host = "127.0.0.1:65409"
	const path = "/api/narrow"
	f := newTestTraffic()
	f.proxy.add(testFrom(testGET(host, path, 200), "curl"))
	tt := openTrafficAt(t, f, 1024)
	waitUntil(t, tt, func() bool { return tt.HasText(host) }, "the URL row")
	url, ok := tt.Find(host)
	if !ok {
		t.Fatalf("the URL cell does not show %q", host)
	}
	status := headerX(t, tt, i18n.StatusLabel())
	if w := status - url.X; w < trafficURLMin {
		t.Errorf("URL column is %v px, want at least %d px", w, trafficURLMin)
	}
}
