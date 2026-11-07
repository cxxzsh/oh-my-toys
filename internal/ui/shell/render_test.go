package shell

import (
	"fmt"
	"image/png"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/devtools"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// fakeCount is how many made-up tools the tests register, as -dev does.
const fakeCount = 500

func newTestApp(t *testing.T, dark bool) *App {
	t.Helper()
	if err := ds.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	events := &core.Events{}
	reg, err := core.NewRegistry(devtools.Fakes(fakeCount, events.Changed)...)
	if err != nil {
		t.Fatal(err)
	}
	a := New(Options{
		Registry: reg, Runner: core.NewRunner(nil), Events: events,
		Store: core.NewStore(t.TempDir()), StartedAt: time.Now(),
	})
	a.forceDark = &dark
	return a
}

func dist(ds []time.Duration) string {
	s := slices.Clone(ds)
	slices.Sort(s)
	m := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	return fmt.Sprintf("n=%d median %.2f ms, p95 %.2f ms, max %.2f ms", len(s), m(s[len(s)/2]), m(s[len(s)*95/100]), m(s[len(s)-1]))
}

// TestRenderHiDPI draws the main window at scale 2, in light and dark, and
// writes the images when OMT_SHOTS names a directory.
func TestRenderHiDPI(t *testing.T) {
	for _, dark := range []bool{false, true} {
		img := ui.Render(newTestApp(t, dark).mainView, ds.WindowWidth, ds.WindowHeight, 2)
		if b := img.Bounds(); b.Dx() != 2*ds.WindowWidth {
			t.Errorf("width %d", b.Dx())
		}
		dir := os.Getenv("OMT_SHOTS")
		if dir == "" {
			continue
		}
		f, err := os.Create(fmt.Sprintf("%s/hidpi-2x-home-dark-%v.png", dir, dark))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Error(err)
		}
		_ = f.Close()
	}
}

// TestPaletteKeystroke types into the palette one letter at a time with
// 500 tools and times each keystroke: the search, the build, the layout
// and the frame painted on the CPU, slower than the GPU the app uses.
func TestPaletteKeystroke(t *testing.T) {
	a := newTestApp(t, false)
	tt := ui.NewTester(a.paletteView, ds.PaletteWidth, 480)
	a.pal.input.Focus()
	tt.Frame()
	var keys []time.Duration
	for _, q := range []string{"yasuotupian", "monitor", "jiankong", "ys"} {
		a.pal.query = ""
		tt.Frame()
		for _, r := range q {
			t0 := time.Now()
			tt.Type(string(r))
			keys = append(keys, time.Since(t0))
		}
		if len(a.pal.results) == 0 {
			t.Errorf("%q: no results", q)
		}
	}
	t.Logf("palette keystroke (CPU frame): %s", dist(keys))
}

// TestHomeScroll scrolls the home page's virtual list of 500 tools and
// times each frame on the CPU.
func TestHomeScroll(t *testing.T) {
	a := newTestApp(t, false)
	tt := ui.NewTester(a.mainView, ds.WindowWidth, ds.WindowHeight)
	tt.Frame()
	frames := make([]time.Duration, 0, 120)
	for i := range 120 {
		dy := float32(120)
		if i >= 60 {
			dy = -dy
		}
		t0 := time.Now()
		tt.Scroll(700, 400, 0, dy)
		frames = append(frames, time.Since(t0))
	}
	t.Logf("home scroll frame (CPU): %s", dist(frames))
}

// TestDestructiveDialogDefaultsToCancel checks the Dialog spec: for a
// destructive action Cancel is first and holds focus, so Enter cancels
// and the action needs an explicit click.
func TestDestructiveDialogDefaultsToCancel(t *testing.T) {
	a := newTestApp(t, false)
	confirmed := false
	a.askDialog(comp.DialogOpts{
		Title:       "Kill 3 processes?",
		Description: "Unsaved work is lost.",
		Cancel:      "Cancel",
		Confirm:     "Kill 3 Processes",
		Destructive: true,
		OnConfirm:   func() { confirmed = true },
	})
	tt := ui.NewTester(a.mainView, ds.WindowWidth, ds.WindowHeight)
	tt.Frame()
	cancel, okC := tt.Find("Cancel")
	confirm, okK := tt.Find("Kill 3 Processes")
	if !okC || !okK || cancel.X >= confirm.X {
		t.Errorf("Cancel %v (ok=%v) is not before Confirm %v (ok=%v)", cancel, okC, confirm, okK)
	}
	tt.Key(0, ui.KeyEnter)
	if a.dialogOpen || confirmed {
		t.Errorf("Enter on a destructive dialog: open=%v confirmed=%v", a.dialogOpen, confirmed)
	}
	a.dialogOpen = true
	tt.Frame()
	if err := tt.Click("Kill 3 Processes"); err != nil {
		t.Fatal(err)
	}
	if a.dialogOpen || !confirmed {
		t.Errorf("Click confirm: open=%v confirmed=%v", a.dialogOpen, confirmed)
	}
}

// TestTrayNavigation checks the TrayPanel spec: it opens with the first
// row highlighted and Down moves the highlight.
func TestTrayNavigation(t *testing.T) {
	a := newTestApp(t, false)
	tt := ui.NewTester(a.trayView, ds.TrayWidth, 480)
	tt.Frame()
	if a.trayChosen != 0 {
		t.Errorf("initial row %d", a.trayChosen)
	}
	tt.Key(0, ui.KeyDown)
	if a.trayChosen != 1 {
		t.Errorf("after Down: row %d", a.trayChosen)
	}
}

// TestSidebarAndPaletteShortcuts checks Mod+Shift+S hiding the sidebar
// and Mod+K opening the command palette inside the main window.
func TestSidebarAndPaletteShortcuts(t *testing.T) {
	a := newTestApp(t, false)
	tt := ui.NewTester(a.mainView, ds.WindowWidth, ds.WindowHeight)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyS)
	if !a.sideHidden {
		t.Error("Mod+Shift+S did not hide the sidebar")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	if !a.pal.open {
		t.Error("Mod+K did not open the palette")
	}
	tt.Key(0, ui.KeyEscape)
	if a.pal.open {
		t.Error("Escape did not close the palette")
	}
}

// TestSidebarPinAndAutoCollapse checks unpinning the sidebar, hovering the
// left edge to pop it out, moving out to auto-collapse it, and pinning it
// back open.
func TestSidebarPinAndAutoCollapse(t *testing.T) {
	a := newTestApp(t, false)
	tt := ui.NewTester(a.mainView, ds.WindowWidth, ds.WindowHeight)
	tt.Frame()
	if err := tt.Click("Hide Sidebar"); err != nil {
		t.Fatal(err)
	}
	if !a.sideHidden || a.sidePeek {
		t.Fatalf("after unpin: hidden=%v peek=%v", a.sideHidden, a.sidePeek)
	}
	tt.Move(4, 200)
	tt.Frame()
	if !a.sidePeek {
		t.Fatal("hovering left edge did not pop out the sidebar")
	}
	tt.Move(100, 200)
	tt.Frame()
	if !a.sidePeek {
		t.Fatal("moving inside floating sidebar collapsed it")
	}
	tt.Move(500, 200)
	tt.Frame()
	if a.sidePeek {
		t.Fatal("moving out of floating sidebar did not auto-collapse it")
	}
	tt.Move(4, 200)
	tt.Frame()
	if err := tt.Click("Show Sidebar"); err != nil {
		t.Fatal(err)
	}
	if a.sideHidden {
		t.Fatal("clicking Pin did not dock the sidebar")
	}
}

// dragRow presses the sidebar row labeled from, moves the pointer in steps
// to the row labeled to, and releases it there. Labels also name the cards
// of the home page, so only rows inside the sidebar's width count.
func dragRow(t *testing.T, tt *ui.Tester, from, to string) {
	t.Helper()
	a, okA := tt.Find(from)
	b, okB := tt.Find(to)
	if !okA || !okB || a.X > ds.SidebarWidth || b.X > ds.SidebarWidth {
		t.Fatalf("rows %q (%+v) and %q (%+v) are not in the sidebar", from, a, to, b)
	}
	x, y0, y1 := a.X+a.W/2, a.Y+a.H/2, b.Y+b.H/4 // the upper half of the target: before it
	tt.Press(x, y0)
	for i := 1; i <= 10; i++ {
		tt.Move(x, y0+(y1-y0)*float32(i)/10)
		tt.Frame()
	}
	tt.Release(x, y1)
	tt.Frame()
}

// TestSidebarDragReordersWithinGroup drags a tool above its neighbour in
// the same category, which changes the order, and onto a tool of another
// category, which changes nothing.
func TestSidebarDragReordersWithinGroup(t *testing.T) {
	a := newTestApp(t, false)
	cat := tool.Categories()[0]
	same := a.orderedInCategory(cat)
	if len(same) < 3 {
		t.Fatalf("category %s has %d tools", cat, len(same))
	}
	first, second := same[0].Manifest(), same[1].Manifest()
	tt := ui.NewTester(a.mainView, ds.WindowWidth, ds.WindowHeight)
	tt.Frame()
	dragRow(t, tt, second.Name(msgs()), first.Name(msgs()))
	got := a.orderedInCategory(cat)
	if got[0].Manifest().ID != second.ID || got[1].Manifest().ID != first.ID {
		t.Errorf("after dragging %s above %s: %s, %s", second.ID, first.ID, got[0].Manifest().ID, got[1].Manifest().ID)
	}
	if len(a.settings.Order) == 0 {
		t.Error("the new order was not recorded")
	}
	a.resetSidebar()
	if a.settings.Order != nil {
		t.Error("reset kept the custom order")
	}
}

// TestSidebarDragAcrossGroupsChangesNothing drops a tool of the first
// category into the second one's rows, and a header row: the order stays.
func TestSidebarDragAcrossGroupsChangesNothing(t *testing.T) {
	a := newTestApp(t, false)
	nav := a.navRows()
	firstTool, otherTool, header := -1, -1, -1
	for i, r := range nav {
		switch {
		case r.kind == navHead && header < 0:
			header = i
		case r.kind == navTool && r.group == string(tool.Categories()[0]) && firstTool < 0:
			firstTool = i
		case r.kind == navTool && r.group == string(tool.Categories()[1]) && otherTool < 0:
			otherTool = i
		}
	}
	a.reorderSidebar([]int{firstTool}, otherTool+1)  // into another group
	a.reorderSidebar([]int{firstTool, otherTool}, 0) // rows of two groups
	a.reorderSidebar([]int{header}, firstTool)       // a header
	if len(a.settings.Order) != 0 {
		t.Errorf("a drop outside the group changed the order: %v", a.settings.Order)
	}
}

// sectionIDs lists the section headers of the sidebar in order.
func sectionIDs(a *App) []string {
	var ids []string
	for _, r := range a.navRows() {
		if r.kind == navHead {
			ids = append(ids, r.section)
		}
	}
	return ids
}

// TestSidebarDragMovesWholeSection drags a category header above the one
// before it: the section, with its tools, changes places. Reset puts it
// back.
func TestSidebarDragMovesWholeSection(t *testing.T) {
	a := newTestApp(t, false)
	for _, cat := range tool.Categories() {
		a.settings.SetCollapsed(string(cat), true) // headers next to each other
	}
	before := sectionIDs(a)
	if len(before) < 3 {
		t.Fatalf("sections %v", before)
	}
	second, first := categoryName(tool.Category(before[1])), categoryName(tool.Category(before[0]))
	tt := ui.NewTester(a.mainView, ds.WindowWidth, ds.WindowHeight)
	tt.Frame()
	dragRow(t, tt, second, first)
	after := sectionIDs(a)
	if after[0] != before[1] || after[1] != before[0] || after[2] != before[2] {
		t.Errorf("sections %v after dragging %v above %v", after, before[1], before[0])
	}
	a.resetSidebar()
	if got := sectionIDs(a); !slices.Equal(got, before) {
		t.Errorf("after reset: %v, want %v", got, before)
	}
}
