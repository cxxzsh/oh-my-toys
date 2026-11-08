package shell

import (
	"image"
	"image/color"
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

func TestSnapHit(t *testing.T) {
	sel := image.Rect(100, 100, 200, 180)
	bar := image.Rect(100, 188, 200, 220)
	if g := snapHit(sel, bar, image.Pt(150, 200)); g != snapGripBar {
		t.Fatalf("toolbar = %v", g)
	}
	if g := snapHit(sel, bar, image.Pt(100, 100)); g != snapGripNW {
		t.Fatalf("corner = %v", g)
	}
	if g := snapHit(sel, bar, image.Pt(150, 100)); g != snapGripN {
		t.Fatalf("edge = %v", g)
	}
	if g := snapHit(sel, bar, image.Pt(150, 140)); g != snapGripMove {
		t.Fatalf("inside = %v", g)
	}
	if g := snapHit(sel, bar, image.Pt(10, 10)); g != snapGripNone {
		t.Fatalf("outside = %v", g)
	}
}

func TestSnapResizeAndMove(t *testing.T) {
	origin := image.Rect(10, 20, 40, 50)
	if got := snapResize(origin, snapGripSE, image.Pt(80, 90)); got != image.Rect(10, 20, 80, 90) {
		t.Fatalf("resize = %v", got)
	}
	// Dragging a corner past the opposite one canonicalizes.
	if got := snapResize(origin, snapGripNW, image.Pt(100, 100)); got != image.Rect(40, 50, 100, 100) {
		t.Fatalf("flipped resize = %v", got)
	}
	limit := image.Rect(0, 0, 100, 100)
	if got := snapMove(origin, image.Pt(10, 20), image.Pt(90, 90), limit); got != image.Rect(70, 70, 100, 100) {
		t.Fatalf("clamped move = %v", got)
	}
}

func TestSnapBarAndLoupe(t *testing.T) {
	sel := image.Rect(10, 10, 80, 40)
	limit := image.Rect(0, 0, 200, 100)
	bar := snapBarPlace(sel, limit, 50, 20)
	if bar.Min.Y != 48 || bar.Max.X != sel.Max.X {
		t.Fatalf("bar below and right-aligned = %v", bar)
	}
	// No room below: the bar goes above the selection.
	low := image.Rect(10, 70, 80, 95)
	bar = snapBarPlace(low, limit, 50, 20)
	if bar.Max.Y > low.Min.Y {
		t.Fatalf("bar above = %v", bar)
	}
	screen := image.Rect(0, 0, 400, 300)
	box := snapLoupeBox(image.Pt(380, 280), screen)
	if box.Max.X > screen.Max.X || box.Max.Y > screen.Max.Y || box.Min.X < 0 || box.Min.Y < 0 {
		t.Fatalf("loupe escaped %v", box)
	}
	if snapHex(color.RGBA{R: 1, G: 2, B: 255}) != "#0102FF" {
		t.Fatal(snapHex(color.RGBA{R: 1, G: 2, B: 255}))
	}
}

func TestSnapPathLabel(t *testing.T) {
	if got := snapPathLabel("/Users/return/Pictures/oh-my-toys", "/Users/return"); got != "~/Pictures/oh-my-toys" {
		t.Fatalf("under home = %q", got)
	}
	if got := snapPathLabel("/tmp/shots", "/Users/return"); got != "/tmp/shots" {
		t.Fatalf("outside home = %q", got)
	}
}
