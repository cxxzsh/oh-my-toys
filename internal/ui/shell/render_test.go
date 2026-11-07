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
