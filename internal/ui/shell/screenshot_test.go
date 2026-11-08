package shell

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSnapLocal(t *testing.T) {
	bounds := image.Rect(100, 50, 400, 350) // a secondary display
	sel := image.Rect(150, 100, 500, 400)   // crosses into the next display
	got := snapLocal(sel, bounds)
	want := ui.Rect{X: 50, Y: 50, W: 250, H: 250}
	if got != want {
		t.Fatalf("snapLocal = %v, want %v", got, want)
	}
	if outside := snapLocal(image.Rect(0, 0, 10, 10), bounds); outside.W != 0 {
		t.Fatalf("selection off the display = %v, want empty", outside)
	}
}

func TestPixelRect(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 50)
	// 2x display: 200x100 pixels.
	got := pixelRect(image.Rect(10, 10, 90, 40), bounds, 2)
	want := image.Rect(20, 20, 180, 80)
	if got != want {
		t.Fatalf("pixelRect = %v, want %v", got, want)
	}
	// Rounds outward on fractional pixels.
	got = pixelRect(image.Rect(10, 10, 11, 11), bounds, 1.5)
	want = image.Rect(15, 15, 17, 17)
	if got != want {
		t.Fatalf("pixelRect fractional = %v, want %v", got, want)
	}
}

func TestSnapTrack(t *testing.T) {
	a := &App{}
	s := &snapSession{pressed: []bool{false, false}}
	ov := &snapOverlay{shot: SnapShot{Bounds: image.Rect(0, 0, 100, 100)}}

	// Press begins the drag at the pointer.
	s.pressed[0] = true
	a.snapTrack(s, ov, true, 10, 10, true)
	if s.sel != image.Rect(10, 10, 10, 10) {
		t.Fatalf("sel after press = %v", s.sel)
	}
	// Dragging updates the end, canonicalized.
	a.snapTrack(s, ov, true, 5, 40, true)
	if s.sel != image.Rect(5, 10, 10, 40) {
		t.Fatalf("sel after drag = %v", s.sel)
	}
	// Release with a real selection keeps it.
	s.pressed[0] = false
	a.snapTrack(s, ov, false, 5, 40, true)
	if s.sel != image.Rect(5, 10, 10, 40) || s.dragging {
		t.Fatalf("after release sel=%v dragging=%v", s.sel, s.dragging)
	}
	// A click-sized release clears the selection.
	s.pressed[0] = true
	a.snapTrack(s, ov, true, 50, 50, true)
	s.pressed[0] = false
	a.snapTrack(s, ov, false, 51, 51, true)
	if !s.sel.Empty() {
		t.Fatalf("click should clear, sel=%v", s.sel)
	}
}

func TestSnapFileName(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 8, 15, 4, 5, 0, time.Local)
	if got := snapFileName(dir, now); got != "oh-my-toys-20261108-150405.png" {
		t.Fatalf("snapFileName = %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "oh-my-toys-20261108-150405.png"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := snapFileName(dir, now); got != "oh-my-toys-20261108-150405-1.png" {
		t.Fatalf("taken name = %q", got)
	}
}
