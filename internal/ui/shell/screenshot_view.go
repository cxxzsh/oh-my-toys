package shell

import (
	"fmt"
	"image"
	"image/color"
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
	if s.text != nil && s.text.commit {
		s.text.commit = false
		a.snapCommitText()
	}
	a.theme(c)
	pal := ds.P(c)
	c.OnShortcut(0, ui.KeyEscape, a.snapCancel)
	c.OnShortcut(ui.Cmd, ui.KeyS, a.snapSave)
	if s.text == nil {
		c.OnShortcut(0, ui.KeyEnter, a.snapReturn)
	}

	a.snapToolKeys(c, s)
	if !s.dragging && !s.sel.Empty() {
		a.snapNudge(c)
		s.bar = snapBarPlace(s.sel, s.barLimit(), snapBarW, snapBarH)
	} else if s.dragging {
		s.bar = image.Rectangle{}
	}

	surface := ui.Box(c).FillWidth().FillHeight().Cursor(ui.CursorCrosshair).
		HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputPointerDown || ev.Button != 1 {
				return false
			}
			a.snapCancel()
			return true
		})
	pressed := surface.Pressed()
	x, y, over := surface.PointerPosition()
	idx := slices.IndexFunc(s.overlays, func(o *snapOverlay) bool { return o == ov })
	if idx >= 0 {
		s.pressed[idx] = pressed
		a.snapTrack(s, ov, pressed, x, y, over)
		if over {
			surface.Cursor(snapCursor(s, image.Pt(int(x)+ov.shot.Bounds.Min.X, int(y)+ov.shot.Bounds.Min.Y)))
		}
	}
	if surface.DoubleClicked() && a.snapInside(ov, x, y) {
		a.snapCopy()
		return
	}

	surface.
		Draw(func(p *ui.Painter, r ui.Rect) { a.snapPaint(p, r, ov, s, pal) }).
		Children(func() {
			a.snapBar(c, ov, s, pal)
			a.snapHud(c, ov, s, pal)
			a.snapTextEditor(c, ov, s, pal)
		})
}

// snapToolKeys puts an annotation tool in hand from the keyboard: a, m or
// t, and the same key again puts it away.
func (a *App) snapToolKeys(c *ui.Context, s *snapSession) {
	if s.text != nil || s.dragging {
		return
	}
	switch {
	case c.Shortcut(0, ui.KeyA):
		a.chooseSnapTool(s, snapToolArrow)
	case c.Shortcut(0, ui.KeyM):
		a.chooseSnapTool(s, snapToolMosaic)
	case c.Shortcut(0, ui.KeyT):
		a.chooseSnapTool(s, snapToolText)
	}
}

// snapNudge moves the selection with the arrow keys, a point at a time, or
// ten with Shift, so it can line up with a pixel. Each combination is asked
// for once: asking consumes the key.
func (a *App) snapNudge(c *ui.Context) {
	s := a.snap
	var d image.Point
	switch {
	case c.Shortcut(0, ui.KeyLeft):
		d.X = -snapNudge
	case c.Shortcut(0, ui.KeyRight):
		d.X = snapNudge
	case c.Shortcut(0, ui.KeyUp):
		d.Y = -snapNudge
	case c.Shortcut(0, ui.KeyDown):
		d.Y = snapNudge
	case c.Shortcut(ui.Shift, ui.KeyLeft):
		d.X = -snapNudgeBig
	case c.Shortcut(ui.Shift, ui.KeyRight):
		d.X = snapNudgeBig
	case c.Shortcut(ui.Shift, ui.KeyUp):
		d.Y = -snapNudgeBig
	case c.Shortcut(ui.Shift, ui.KeyDown):
		d.Y = snapNudgeBig
	default:
		return
	}
	s.sel = snapMove(s.sel, image.Point{}, d, s.barLimit())
}

func (a *App) snapInside(ov *snapOverlay, x, y float32) bool {
	s := a.snap
	if s == nil || s.sel.Empty() {
		return false
	}
	p := image.Pt(int(x)+ov.shot.Bounds.Min.X, int(y)+ov.shot.Bounds.Min.Y)
	return p.In(s.sel)
}

// snapTrack folds this overlay's pointer into the session. A press on the
// toolbar is ignored. A press on a handle resizes, one inside the
// selection moves it, and one outside draws a new rectangle.
func (a *App) snapTrack(s *snapSession, ov *snapOverlay, pressed bool, x, y float32, over bool) {
	anyDown := slices.Contains(s.pressed, true)
	pos := image.Pt(int(x)+ov.shot.Bounds.Min.X, int(y)+ov.shot.Bounds.Min.Y)
	if over {
		s.hover, s.hoverOn = pos, true
	}
	changed := false
	if pressed && !s.wasDown {
		changed = a.snapPress(s, pos)
	}
	if s.dragging && over && a.snapDrag(s, pos) {
		changed = true
	}
	if !anyDown && s.wasDown {
		s.dragging = false
		switch {
		case s.mode == snapGripMark:
			s.commitMark()
		case s.mode == snapGripNew && (s.sel.Dx() < snapMinDrag || s.sel.Dy() < snapMinDrag):
			s.sel = s.pressWin
		}
		s.pressWin = image.Rectangle{}
		s.mode = snapGripNone
		changed = true
	}
	s.wasDown = anyDown
	if changed {
		a.snapInvalidate()
	}
}

func (a *App) snapPress(s *snapSession, pos image.Point) bool {
	if s.text != nil {
		a.snapCommitText()
	}
	hit := snapHit(s.sel, s.bar, pos)
	if hit == snapGripBar {
		s.mode = snapGripBar
		return false
	}
	if hit == snapGripNone {
		s.mode, s.anchor, s.dragging = snapGripNew, pos, true
		s.sel = image.Rect(pos.X, pos.Y, pos.X, pos.Y)
		s.pressWin = snapWindowAt(s.wins, pos)
		return true
	}
	if s.tool != snapToolNone && pos.In(s.sel) {
		if s.tool == snapToolText {
			s.text = &snapText{at: pos}
			s.mode = snapGripNone
			a.registerSnapKeys()
			return true
		}
		s.pending = &snapMark{tool: s.tool, a: pos, b: pos}
		s.mode, s.dragging = snapGripMark, true
		return true
	}
	s.mode, s.origin, s.anchor, s.dragging = hit, s.sel, pos, true
	return true
}

// snapDrag applies the drag in progress. It reports whether the
// selection changed.
func (a *App) snapDrag(s *snapSession, pos image.Point) bool {
	var next image.Rectangle
	switch s.mode {
	case snapGripNew:
		next = image.Rect(s.anchor.X, s.anchor.Y, pos.X, pos.Y).Canon()
	case snapGripMove:
		next = snapMove(s.origin, s.anchor, pos, s.limit())
	case snapGripMark:
		if s.pending == nil {
			return false
		}
		s.pending.b = pos
		return true
	case snapGripN, snapGripS, snapGripE, snapGripW, snapGripNE, snapGripNW, snapGripSE, snapGripSW:
		next = snapResize(s.origin, s.mode, pos)
	default:
		return false
	}
	if next == s.sel {
		return false
	}
	s.sel = next
	return true
}

func (s *snapSession) limit() image.Rectangle {
	if len(s.overlays) == 0 {
		return image.Rectangle{}
	}
	b := s.overlays[0].shot.Bounds
	for _, ov := range s.overlays[1:] {
		b = b.Union(ov.shot.Bounds)
	}
	return b
}

func (s *snapSession) barLimit() image.Rectangle {
	if s.sel.Empty() {
		return image.Rectangle{}
	}
	if ov := s.overlayAt(image.Pt(s.sel.Max.X-1, s.sel.Max.Y-1)); ov != nil {
		return ov.shot.Bounds
	}
	return s.limit()
}

func snapCursor(s *snapSession, p image.Point) ui.Cursor {
	g := s.mode
	if !s.dragging {
		g = snapHit(s.sel, s.bar, p)
	}
	switch g {
	case snapGripMove:
		return ui.CursorMove
	case snapGripN, snapGripS:
		return ui.CursorResizeNS
	case snapGripE, snapGripW:
		return ui.CursorResizeEW
	case snapGripNW, snapGripSE:
		return ui.CursorResizeNWSE
	case snapGripNE, snapGripSW:
		return ui.CursorResizeNESW
	case snapGripBar:
		return ui.CursorDefault
	default:
		return ui.CursorCrosshair
	}
}

// snapPaint draws the frozen picture, the dim, the selection and, while
// aiming, the loupe.
func (a *App) snapPaint(p *ui.Painter, r ui.Rect, ov *snapOverlay, s *snapSession, pal *ds.Palette) {
	b := ov.shot.Bounds
	w, h := ov.shot.Image.Bounds().Dx(), ov.shot.Image.Bounds().Dy()
	p.ImageRegion(ov.bmp, ui.Rect{W: float32(w), H: float32(h)}, ui.Rect{W: float32(b.Dx()), H: float32(b.Dy())}, ui.ImageOptions{})
	local := snapLocal(s.sel, b)
	if local.W <= 0 || local.H <= 0 {
		p.Fill(r, pal.Scrim, 0)
		a.snapHoverWindow(p, ov, s, pal)
		a.snapLoupeIf(p, ov, s, pal)
		return
	}
	a.snapDim(p, r, local, pal.Scrim)
	a.snapPaintMarks(p, ov, s)
	a.snapSelection(p, local, s, pal)
	a.snapLoupeIf(p, ov, s, pal)
}

func (a *App) snapSelection(p *ui.Painter, local ui.Rect, s *snapSession, pal *ds.Palette) {
	if s.sel.Empty() {
		return
	}
	p.Stroke(local, pal.Accent, 0, ds.LineWidth*2)
	a.snapTag(p, local, snapSizeText(s.sel.Dx(), s.sel.Dy()), pal)
	if !s.dragging || s.mode != snapGripNew {
		snapHandles(p, local, pal)
	}
}

func (a *App) snapLoupeIf(p *ui.Painter, ov *snapOverlay, s *snapSession, pal *ds.Palette) {
	aiming := s.sel.Empty() || (s.dragging && s.mode == snapGripNew)
	if s.hoverOn && aiming && pIn(s.hover, ov.shot.Bounds) {
		a.snapLoupe(p, ov, s.hover, pal)
	}
}

func pIn(p image.Point, r image.Rectangle) bool {
	return p.X >= r.Min.X && p.X < r.Max.X && p.Y >= r.Min.Y && p.Y < r.Max.Y
}

func snapHandles(p *ui.Painter, local ui.Rect, pal *ds.Palette) {
	pts := [8]ui.Point{
		{X: local.X, Y: local.Y},
		{X: local.X + local.W/2, Y: local.Y},
		{X: local.X + local.W, Y: local.Y},
		{X: local.X, Y: local.Y + local.H/2},
		{X: local.X + local.W, Y: local.Y + local.H/2},
		{X: local.X, Y: local.Y + local.H},
		{X: local.X + local.W/2, Y: local.Y + local.H},
		{X: local.X + local.W, Y: local.Y + local.H},
	}
	const n = float32(7)
	for _, pt := range pts {
		box := ui.Rect{X: pt.X - n/2, Y: pt.Y - n/2, W: n, H: n}
		p.Fill(box, pal.BgPopover, ds.RadiusXs)
		p.Stroke(box, pal.Accent, ds.RadiusXs, ds.LineWidth)
	}
}

// snapLoupe magnifies the pixels around at and names the center color.
func (a *App) snapLoupe(p *ui.Painter, ov *snapOverlay, at image.Point, pal *ds.Palette) {
	box := snapLoupeBox(at, ov.shot.Bounds)
	px, py, col := snapSample(ov, at)
	half := snapLoupeN / 2
	src := ui.Rect{X: float32(px - half), Y: float32(py - half), W: snapLoupeN, H: snapLoupeN}
	dst := ui.Rect{X: float32(box.Min.X - ov.shot.Bounds.Min.X), Y: float32(box.Min.Y - ov.shot.Bounds.Min.Y), W: float32(box.Dx()), H: float32(box.Dy())}
	p.Fill(dst, pal.BgPopover, ds.RadiusSm)
	p.ImageRegion(ov.bmp, src, dst, ui.ImageOptions{})
	p.Stroke(dst, pal.TextPrimary, ds.RadiusSm, ds.LineWidth)
	cell := float32(snapLoupeCell)
	mid := ui.Rect{X: dst.X + cell*float32(half), Y: dst.Y + cell*float32(half), W: cell, H: cell}
	p.Stroke(mid, pal.Accent, 0, ds.LineWidth*2)
	label := ui.Rect{X: dst.X, Y: dst.Y + dst.H + ds.Space4, W: dst.W, H: snapLoupeLabel}
	p.Fill(label, pal.BgPopover, ds.RadiusXs)
	p.Text(label.X+ds.Space8, label.Y+ds.Space2, snapHex(col), ds.Caption.Size, pal.TextPrimary)
}

// snapSample is the color and pixel of global, clamped to the picture.
func snapSample(ov *snapOverlay, global image.Point) (px, py int, c color.RGBA) {
	b := ov.shot.Bounds
	img := ov.shot.Image
	ib := img.Bounds()
	px = int(float64(global.X-b.Min.X) * ov.pixelsPerPoint())
	py = int(float64(global.Y-b.Min.Y) * ov.pixelsPerPoint())
	px = clamp(px, 0, ib.Dx()-1)
	py = clamp(py, 0, ib.Dy()-1)
	return px, py, img.RGBAAt(ib.Min.X+px, ib.Min.Y+py)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// snapBar is the toolbar under a committed selection, its right edge on the
// selection's, as Snipaste puts it. It hangs on that edge by its own width,
// so it stays snug in every language.
func (a *App) snapBar(c *ui.Context, ov *snapOverlay, s *snapSession, pal *ds.Palette) {
	if s.dragging || s.sel.Empty() || s.bar.Empty() || !pIn(s.bar.Min, ov.shot.Bounds) {
		return
	}
	ui.Row(c).Gap(ds.Space8).Padding(ds.Space8).
		Background(pal.BgPopover).Radius(ds.RadiusMd).
		Border(ds.LineWidth, pal.LineControl).
		Absolute().
		Right(float32(ov.shot.Bounds.Max.X - s.bar.Max.X)).
		Top(float32(s.bar.Min.Y - ov.shot.Bounds.Min.Y)).
		Children(func() {
			a.snapToolButtons(c, s)
			if a.kit.Button(c.Key("snap-copy"), i18n.CopyValue(), comp.ButtonOpts{Kind: comp.Primary, Small: true}).Clicked() {
				a.snapCopy()
			}
			if a.kit.Button(c.Key("snap-save"), i18n.SnapSave(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
				a.snapSave()
			}
			if a.kit.Button(c.Key("snap-cancel"), i18n.Cancel(), comp.ButtonOpts{Kind: comp.Ghost, Small: true}).Clicked() {
				a.endSnap()
			}
		})
}

// snapToolButtons are the annotation tools, each a glyph that stays lit
// while its tool is in hand.
func (a *App) snapToolButtons(c *ui.Context, s *snapSession) {
	tools := []struct {
		key   string
		glyph string
		tool  snapTool
		label string
	}{
		{"snap-tool-arrow", "north_east", snapToolArrow, i18n.SnapArrow()},
		{"snap-tool-mosaic", "blur_on", snapToolMosaic, i18n.SnapMosaic()},
		{"snap-tool-text", "text_fields", snapToolText, i18n.SnapText()},
	}
	for _, t := range tools {
		kind := comp.Ghost
		if s.tool == t.tool {
			kind = comp.Primary
		}
		if a.kit.Button(c.Key(t.key), "", comp.ButtonOpts{Kind: kind, Small: true, Glyph: t.glyph, Tooltip: t.label}).Clicked() {
			a.chooseSnapTool(s, t.tool)
		}
	}
}

// chooseSnapTool puts a tool in hand, or puts it away when it is already
// the one in hand.
func (a *App) chooseSnapTool(s *snapSession, tool snapTool) {
	s.text = nil
	if s.tool == tool {
		s.tool = snapToolNone
	} else {
		s.tool = tool
	}
	a.snapInvalidate()
}

// snapTextEditor is the field a label is typed in, on the spot it will
// stay.
func (a *App) snapTextEditor(c *ui.Context, ov *snapOverlay, s *snapSession, pal *ds.Palette) {
	t := s.text
	if t == nil || !pIn(t.at, ov.shot.Bounds) {
		return
	}
	ui.Row(c).Absolute().
		Left(float32(t.at.X-ov.shot.Bounds.Min.X)).
		Top(float32(t.at.Y-ov.shot.Bounds.Min.Y)).
		Padding(ds.Space4).Radius(ds.RadiusSm).
		Background(pal.BgPopover).Border(ds.LineWidth, pal.Accent).
		Children(func() {
			ui.TextInputBase(c, &t.value).Width(snapTextWidth).Focus().
				HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter {
						t.commit = true
						return true
					}
					return false
				})
		})
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

// snapTag draws a size in a box at the top left of what it measures, just
// above it, with the text centered in the box; it sits inside when there
// is no room above.
func (a *App) snapTag(p *ui.Painter, local ui.Rect, text string, pal *ds.Palette) {
	txtW, _ := p.MeasureText(0, ui.Span{Text: text, Size: ds.Caption.Size})
	tagW := txtW + ds.Space16
	th := float32(ds.Space20)
	y := local.Y - th - ds.Space4
	if y < ds.Space4 {
		y = local.Y + ds.Space4
	}
	tag := ui.Rect{X: local.X, Y: y, W: tagW, H: th}
	p.Fill(tag, pal.BgPopover, ds.RadiusSm)
	p.Stroke(tag, pal.LineControl, ds.RadiusSm, ds.LineWidth)
	p.Text(tag.X+(tag.W-txtW)/2, tag.Y+(th-ds.Caption.Size)/2, text, ds.Caption.Size, pal.TextPrimary)
}

// snapHoverWindow outlines the window a click would take as the
// selection.
func (a *App) snapHoverWindow(p *ui.Painter, ov *snapOverlay, s *snapSession, pal *ds.Palette) {
	if s.dragging || !s.hoverOn || !pIn(s.hover, ov.shot.Bounds) {
		return
	}
	win := snapWindowAt(s.wins, s.hover)
	if win.Empty() {
		return
	}
	local := snapLocal(win, ov.shot.Bounds)
	p.Fill(local, pal.AccentSoft, 0)
	p.Stroke(local, pal.Accent, 0, ds.LineWidth*2)
	a.snapTag(p, local, snapSizeText(win.Dx(), win.Dy()), pal)
}

// snapHud draws the hint while aiming, or a flash after a save.
func (a *App) snapHud(c *ui.Context, ov *snapOverlay, s *snapSession, pal *ds.Palette) {
	text := ""
	switch {
	case s.flash != "" && time.Now().Before(s.flashTil):
		text = s.flash
	case s.sel.Empty() && !s.dragging:
		text = i18n.SnapHint()
	default:
		return
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
	snapMinDrag  = 3 // smaller releases of a new drag count as a click
	snapFlashFor = 2500 * time.Millisecond
)
