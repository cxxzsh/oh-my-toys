package shell

import (
	"fmt"
	"image/png"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/core"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tools/devtools"
)

// fakeCount is how many made-up tools the tests register, as -dev does.
const fakeCount = 500

func newTestApp(t *testing.T, dark bool) *App {
	t.Helper()
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
		img := ui.Render(newTestApp(t, dark).mainView, 1100, 720, 2)
		if b := img.Bounds(); b.Dx() != 2200 {
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
	tt := ui.NewTester(a.paletteView, 600, 480)
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
	tt := ui.NewTester(a.mainView, 1100, 720)
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
