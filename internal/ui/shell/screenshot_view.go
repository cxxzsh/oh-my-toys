package shell

import (
	"fmt"
	"image"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// snapView is the content of one display's overlay window.
func (a *App) snapView(c *ui.Context, ov *snapOverlay) {
	s := a.snap
	if s == nil {
		return
	}
	a.theme(c)
	pal := ds.P(c)

	// Keys: whichever overlay is key handles them all the same.
	c.OnShortcut(0, ui.KeyEscape, a.endSnap)
	c.OnShortcut(ui.Cmd, ui.KeyS, a.snapSave)
	c.OnShortcut(0, ui.KeyEnter, a.snapCopy)

	// The surface covers the window and tracks the drag.
	surface := ui.Box(c).FillWidth().FillHeight()
	pressed := surface.Pressed()
	x, y, over := surface.PointerPosition()
	idx := slices.IndexFunc(s.overlays, func(o *snapOverlay) bool { return o == ov })
	if idx >= 0 {
		s.pressed[idx] = pressed
		a.snapTrack(s, ov, pressed, x, y, over)
	}

	surface.
		Draw(func(p *ui.Painter, r ui.Rect) { a.snapPaint(p, r, ov, s, pal) }).
		Children(func() { a.snapHud(c, ov, s, pal) })
}

// snapTrack folds this overlay's pointer state into the session's drag.
// The press edge comes from the overlay being pressed, the moving end
// from whichever overlay the pointer is over, so a drag crosses
// displays; the release edge comes from no overlay being pressed.
func (a *App) snapTrack(s *snapSession, ov *snapOverlay, pressed bool, x, y float32, over bool) {
	anyDown := slices.Contains(s.pressed, true)
	pos := image.Pt(int(x)+ov.shot.Bounds.Min.X, int(y)+ov.shot.Bounds.Min.Y)
	changed := false
	if pressed && !s.wasDown {
		s.anchor, s.dragging = pos, true
		s.sel = image.Rect(pos.X, pos.Y, pos.X, pos.Y)
		changed = true
	}
	if s.dragging && over {
		if next := image.Rect(s.anchor.X, s.anchor.Y, pos.X, pos.Y).Canon(); next != s.sel {
			s.sel, changed = next, true
		}
	}
	if !anyDown && s.wasDown {
		s.dragging = false
		if s.sel.Dx() < snapMinDrag || s.sel.Dy() < snapMinDrag {
			s.sel, changed = image.Rectangle{}, true
		}
	}
	s.wasDown = anyDown
	if changed {
		a.snapInvalidate()
	}
}

// snapPaint draws the screenshot, the dim, the selection and its size.
func (a *App) snapPaint(p *ui.Painter, r ui.Rect, ov *snapOverlay, s *snapSession, pal *ds.Palette) {
	b := ov.shot.Bounds
	w, h := ov.shot.Image.Bounds().Dx(), ov.shot.Image.Bounds().Dy()
	full := ui.Rect{W: float32(b.Dx()), H: float32(b.Dy())}

	// The picture: this display's pixels stretched over its points.
	p.ImageRegion(ov.bmp, ui.Rect{W: float32(w), H: float32(h)}, full, ui.ImageOptions{})

	// Dim everything but the selection: four rectangles around it.
	local := snapLocal(s.sel, b)
	if local.W <= 0 || local.H <= 0 {
		p.Fill(r, pal.Scrim, 0)
		return
	}
	a.snapDim(p, r, local, pal.Scrim)
	if s.sel.Empty() {
		return
	}
	p.Stroke(local, pal.Accent, 0, ds.LineWidth*2)
	a.snapTag(p, local, s.sel, pal)
}

// snapDim fills r minus sel with c.
func (a *App) snapDim(p *ui.Painter, r, sel ui.Rect, c ui.Color) {
	right, bottom := r.X+r.W, r.Y+r.H
	selRight, selBottom := sel.X+sel.W, sel.Y+sel.H
	p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: sel.Y - r.Y}, c, 0)
	p.Fill(ui.Rect{X: r.X, Y: selBottom, W: r.W, H: bottom - selBottom}, c, 0)
	p.Fill(ui.Rect{X: r.X, Y: sel.Y, W: sel.X - r.X, H: sel.H}, c, 0)
	p.Fill(ui.Rect{X: selRight, Y: sel.Y, W: right - selRight, H: sel.H}, c, 0)
}

// snapTag draws the selection's size at its top-left corner.
func (a *App) snapTag(p *ui.Painter, local ui.Rect, sel image.Rectangle, pal *ds.Palette) {
	text := snapSizeText(sel.Dx(), sel.Dy())
	tw := float32(len(text))*ds.Caption.Size*snapTagCharW + ds.Space16
	th := float32(ds.Space20)
	tag := ui.Rect{X: local.X + ds.Space4, Y: local.Y + ds.Space4, W: tw, H: th}
	p.Fill(tag, pal.BgPopover, ds.RadiusSm)
	p.Stroke(tag, pal.LineControl, ds.RadiusSm, ds.LineWidth)
	p.Text(tag.X+ds.Space8, tag.Y+(th-ds.Caption.Size)/2, text, ds.Caption.Size, pal.TextPrimary)
}

// snapHud draws the hint line at the bottom of the overlay.
func (a *App) snapHud(c *ui.Context, ov *snapOverlay, s *snapSession, pal *ds.Palette) {
	text := i18n.SnapHint()
	if s.flash != "" && time.Now().Before(s.flashTil) {
		text = s.flash
	}
	comp.Text(c, text, ds.Label, pal.TextSecondary).
		Absolute().
		Left(ds.Space16).
		Top(float32(ov.shot.Bounds.Dy()) - ds.Space16 - ds.Label.Size)
}

// snapLocal translates a global selection into this display's window
// coordinates, clamped to the window.
func snapLocal(sel image.Rectangle, bounds image.Rectangle) ui.Rect {
	part := sel.Intersect(bounds)
	if part.Empty() {
		return ui.Rect{}
	}
	return ui.Rect{
		X: float32(part.Min.X - bounds.Min.X), Y: float32(part.Min.Y - bounds.Min.Y),
		W: float32(part.Dx()), H: float32(part.Dy()),
	}
}

// snapSizeText is the "1234 × 567" tag on the selection.
func snapSizeText(w, h int) string {
	return fmt.Sprintf("%d × %d", w, h)
}

const (
	snapMinDrag  = 3   // smaller releases count as a click that clears
	snapTagCharW = 0.6 // average glyph width of the size tag, in ems
	snapFlashFor = 2500 * time.Millisecond
)
