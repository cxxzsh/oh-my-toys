package shell

import (
	"image"
	"image/color"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// snapTool is the annotation tool in hand; none lets the pointer move the
// selection again.
type snapTool int

// The annotation tools, in the order the toolbar lists them.
const (
	snapToolNone snapTool = iota
	snapToolArrow
	snapToolMosaic
	snapToolText
	snapToolErase
)

// snapMark is one annotation of the session, in global points. A text mark
// keeps its anchor in a and its words in text.
type snapMark struct {
	tool snapTool
	a, b image.Point
	text string
}

const (
	snapMarkWidth  = 3.0  // points, the shaft of an arrow
	snapMarkHead   = 14.0 // points, the head of an arrow, along its shaft
	snapMarkSpread = 0.5  // how far the head's edges open, over its length
	snapMarkText   = 16.0 // points, the size of a label
	snapMosaicCell = 8    // points, one square of a mosaic
	snapMarkMin    = 6.0  // points, a mark smaller than this is dropped
	snapTextCharW  = 0.55 // average glyph width of a label, in ems
	snapTextLineH  = 1.4  // a label's line height, over its size
)

// snapMarkColor is the red annotations are drawn in, on screen and in the
// file.
var snapMarkColor = color.RGBA{R: 0xE5, G: 0x3E, B: 0x3E, A: 0xFF}

// snapPaintMarks draws the session's marks that touch one display, clipped
// to the selection as the exported picture is, and the one being drawn with
// its box outlined.
func (a *App) snapPaintMarks(p *ui.Painter, ov *snapOverlay, s *snapSession) {
	clip := snapLocal(s.sel, ov.shot.Bounds)
	if clip.W <= 0 || clip.H <= 0 {
		return
	}
	c := ui.RGBA(snapMarkColor.R, snapMarkColor.G, snapMarkColor.B, 1)
	p.Clip(clip, 0, func() {
		for _, m := range s.marks {
			a.snapPaintMark(p, ov, m, c, false)
		}
		if s.pending != nil {
			a.snapPaintMark(p, ov, *s.pending, c, true)
		}
	})
}

// snapPaintMark draws one mark in one display's points; a mark being drawn
// also outlines the box it covers.
func (a *App) snapPaintMark(p *ui.Painter, ov *snapOverlay, m snapMark, c ui.Color, pending bool) {
	o := ov.shot.Bounds.Min
	at := func(pt image.Point) (float32, float32) {
		return float32(pt.X - o.X), float32(pt.Y - o.Y)
	}
	switch m.tool {
	case snapToolNone, snapToolErase:
	case snapToolArrow:
		for _, ln := range snapArrowLines(m.a, m.b) {
			x0, y0 := at(ln[0])
			x1, y1 := at(ln[1])
			p.StrokePolyline([][2]float32{{x0, y0}, {x1, y1}}, snapMarkWidth, c)
		}
	case snapToolMosaic:
		a.snapPaintMosaic(p, ov, m)
		if pending {
			p.Stroke(snapLocal(snapMarkRect(m), ov.shot.Bounds), c, 0, ds.LineWidth)
		}
	case snapToolText:
		x, y := at(m.a)
		p.Text(x, y, m.text, snapMarkText, c)
	}
}

// snapPaintMosaic covers the mark's rectangle with the colors of its own
// pixels, one square at a time.
func (a *App) snapPaintMosaic(p *ui.Painter, ov *snapOverlay, m snapMark) {
	r := snapMarkRect(m).Intersect(ov.shot.Bounds)
	o := ov.shot.Bounds.Min
	for y := r.Min.Y; y < r.Max.Y; y += snapMosaicCell {
		for x := r.Min.X; x < r.Max.X; x += snapMosaicCell {
			_, _, c := snapSample(ov, image.Pt(x+snapMosaicCell/2, y+snapMosaicCell/2))
			w := min(snapMosaicCell, r.Max.X-x)
			h := min(snapMosaicCell, r.Max.Y-y)
			p.Fill(ui.Rect{
				X: float32(x - o.X), Y: float32(y - o.Y),
				W: float32(w), H: float32(h),
			}, ui.RGBA(c.R, c.G, c.B, 1), 0)
		}
	}
}

// snapMarkRect is the box a mark covers: a text's anchor has no box.
func snapMarkRect(m snapMark) image.Rectangle {
	return image.Rect(m.a.X, m.a.Y, m.b.X, m.b.Y).Canon()
}

// snapArrowLines breaks an arrow into the lines that draw it: the shaft
// ends where the head begins, and the head's two edges meet at the tip.
func snapArrowLines(a, b image.Point) [3][2]image.Point {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	d := math.Hypot(dx, dy)
	if d == 0 {
		return [3][2]image.Point{}
	}
	ux, uy := dx/d, dy/d
	head := min(snapMarkHead, d)
	tipX, tipY := float64(b.X), float64(b.Y)
	baseX, baseY := tipX-ux*head, tipY-uy*head
	round := func(x, y float64) image.Point {
		return image.Pt(int(math.Round(x)), int(math.Round(y)))
	}
	return [3][2]image.Point{
		{{a.X, a.Y}, round(baseX, baseY)},
		{round(tipX, tipY), round(baseX-uy*head*snapMarkSpread, baseY+ux*head*snapMarkSpread)},
		{round(tipX, tipY), round(baseX+uy*head*snapMarkSpread, baseY-ux*head*snapMarkSpread)},
	}
}

// snapAction is one change to the marks: undo reverses it, redo makes it
// again.
type snapAction struct {
	mark  snapMark
	erase bool // the action took the mark away
	at    int  // where the mark sat in the list
}

// addMark keeps a mark, remembering how it went in.
func (s *snapSession) addMark(m snapMark) {
	s.marks = append(s.marks, m)
	s.history = append(s.history, snapAction{mark: m, at: len(s.marks) - 1})
	s.redone = nil
}

// eraseAt takes the mark at an index away, remembering how it came out.
func (s *snapSession) eraseAt(i int) {
	if i < 0 || i >= len(s.marks) {
		return
	}
	m := s.marks[i]
	s.marks = append(s.marks[:i], s.marks[i+1:]...)
	s.history = append(s.history, snapAction{mark: m, erase: true, at: i})
	s.redone = nil
}

// eraseMarks rubs out every mark the eraser touches at p. It reports
// whether any went.
func (s *snapSession) eraseMarks(p image.Point) bool {
	gone := false
	for i := len(s.marks) - 1; i >= 0; i-- {
		if snapMarkHit(s.marks[i], p) {
			s.eraseAt(i)
			gone = true
		}
	}
	return gone
}

// snapMarkHit reports whether p touches a mark: its box for a mosaic, its
// line for an arrow, the space its words take for a label.
func snapMarkHit(m snapMark, p image.Point) bool {
	switch m.tool {
	case snapToolMosaic:
		return p.In(snapMarkRect(m))
	case snapToolArrow:
		return snapNearLine(m.a, m.b, p, snapEraseReach)
	case snapToolText:
		return p.In(snapTextBounds(m))
	case snapToolNone, snapToolErase:
	}
	return false
}

// snapTextBounds is the space a label's words take, estimated from their
// count, which is all the mark keeps.
func snapTextBounds(m snapMark) image.Rectangle {
	w := int(float64(len([]rune(m.text))) * snapMarkText * snapTextCharW)
	h := int(math.Round(snapMarkText * snapTextLineH))
	return image.Rect(m.a.X, m.a.Y, m.a.X+w, m.a.Y+h)
}

// snapNearLine reports whether p is within reach of the line from a to b.
func snapNearLine(a, b, p image.Point, reach float64) bool {
	ax, ay := float64(a.X), float64(a.Y)
	bx, by := float64(b.X), float64(b.Y)
	px, py := float64(p.X), float64(p.Y)
	dx, dy := bx-ax, by-ay
	d := dx*dx + dy*dy
	t := 0.0
	if d > 0 {
		t = ((px-ax)*dx + (py-ay)*dy) / d
		t = math.Max(0, math.Min(1, t))
	}
	cx, cy := ax+t*dx, ay+t*dy
	return math.Hypot(px-cx, py-cy) <= reach
}

// undo reverses the last change, or lets go of the mark being drawn. It
// reports whether there was one.
func (s *snapSession) undo() bool {
	if s.pending != nil {
		s.pending, s.dragging, s.mode = nil, false, snapGripNone
		return true
	}
	if len(s.history) == 0 {
		return false
	}
	a := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	a.revert(s)
	s.redone = append(s.redone, a)
	return true
}

// redo makes the last change again. It reports whether there was one.
func (s *snapSession) redo() bool {
	if len(s.redone) == 0 {
		return false
	}
	a := s.redone[len(s.redone)-1]
	s.redone = s.redone[:len(s.redone)-1]
	a.apply(s)
	s.history = append(s.history, a)
	return true
}

// apply makes the change again, revert takes it back.
func (a snapAction) apply(s *snapSession) {
	if a.erase {
		s.takeAt(a.at)
		return
	}
	s.putAt(a.at, a.mark)
}

func (a snapAction) revert(s *snapSession) {
	if a.erase {
		s.putAt(a.at, a.mark)
		return
	}
	s.takeAt(a.at)
}

// putAt is a mark at an index.
func (s *snapSession) putAt(at int, m snapMark) {
	at = clamp(at, 0, len(s.marks))
	s.marks = append(s.marks, snapMark{})
	copy(s.marks[at+1:], s.marks[at:])
	s.marks[at] = m
}

// takeAt takes the mark at an index away without remembering it.
func (s *snapSession) takeAt(at int) {
	if at < 0 || at >= len(s.marks) {
		return
	}
	s.marks = append(s.marks[:at], s.marks[at+1:]...)
}

// canUndo reports whether there is a change to take back.
func (s *snapSession) canUndo() bool { return s.pending != nil || len(s.history) > 0 }

// canRedo reports whether undo took back a change that can come again.
func (s *snapSession) canRedo() bool { return len(s.redone) > 0 }

// commitMark keeps the mark just drawn, unless it is too small to mean
// anything.
func (s *snapSession) commitMark() {
	m := s.pending
	s.pending = nil
	if m == nil {
		return
	}
	switch m.tool {
	case snapToolNone, snapToolText, snapToolErase:
	case snapToolArrow:
		dx, dy := float64(m.b.X-m.a.X), float64(m.b.Y-m.a.Y)
		if math.Hypot(dx, dy) < snapMarkMin {
			return
		}
	case snapToolMosaic:
		r := snapMarkRect(*m)
		if float64(r.Dx()) < snapMarkMin || float64(r.Dy()) < snapMarkMin {
			return
		}
		m.a, m.b = r.Min, r.Max
	}
	s.addMark(*m)
}

// snapDrawMarks draws the session's marks into the exported image, whose
// top left pixel is the selection's.
func snapDrawMarks(img *image.RGBA, marks []snapMark, sel image.Rectangle, scale float64) {
	for _, m := range marks {
		snapDrawMark(img, m, sel, scale)
	}
}

func snapDrawMark(img *image.RGBA, m snapMark, sel image.Rectangle, scale float64) {
	px := func(p image.Point) image.Point {
		return image.Pt(
			int(math.Round(float64(p.X-sel.Min.X)*scale)),
			int(math.Round(float64(p.Y-sel.Min.Y)*scale)))
	}
	switch m.tool {
	case snapToolNone, snapToolErase:
	case snapToolArrow:
		width := int(math.Round(snapMarkWidth * scale))
		for _, ln := range snapArrowLines(m.a, m.b) {
			stampLine(img, px(ln[0]), px(ln[1]), width)
		}
	case snapToolMosaic:
		snapDrawMosaic(img, m, sel, scale)
	case snapToolText:
		snapDrawText(img, px(m.a), m.text, snapMarkText*scale)
	}
}

// snapDrawMosaic pixelates the mark's box with the image's own pixels,
// read before each square is written.
func snapDrawMosaic(img *image.RGBA, m snapMark, sel image.Rectangle, scale float64) {
	cell := int(math.Round(snapMosaicCell * scale))
	if cell < 1 {
		cell = 1
	}
	for y := m.a.Y; y < m.b.Y; y += snapMosaicCell {
		for x := m.a.X; x < m.b.X; x += snapMosaicCell {
			sx := int(math.Round(float64(x-sel.Min.X) * scale))
			sy := int(math.Round(float64(y-sel.Min.Y) * scale))
			c := img.RGBAAt(clamp(sx, img.Rect.Min.X, img.Rect.Max.X-1),
				clamp(sy, img.Rect.Min.Y, img.Rect.Max.Y-1))
			w := min(cell, int(math.Round(float64(min(snapMosaicCell, m.b.X-x))*scale)))
			h := min(cell, int(math.Round(float64(min(snapMosaicCell, m.b.Y-y))*scale)))
			fillRect(img, image.Rect(sx, sy, sx+w, sy+h), c)
		}
	}
}

// fillRect paints a rectangle of img, clipped to it.
func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Rect)
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// stampLine paints a line of squares, so it keeps its width at any angle.
func stampLine(img *image.RGBA, a, b image.Point, width int) {
	if width < 1 {
		width = 1
	}
	steps := max(abs(b.X-a.X), abs(b.Y-a.Y))
	if steps == 0 {
		fillRect(img, image.Rect(a.X, a.Y, a.X+width, a.Y+width), snapMarkColor)
		return
	}
	for i := 0; i <= steps; i++ {
		x := a.X + (b.X-a.X)*i/steps
		y := a.Y + (b.Y-a.Y)*i/steps
		fillRect(img, image.Rect(x, y, x+width, y+width), snapMarkColor)
	}
}

// snapFont parses a static TrueType font once for rasterizing text labels into
// exported screenshot bitmaps (x/image/font/opentype does not interpolate gvar).
var snapFont = sync.OnceValues(func() (*opentype.Font, error) {
	return opentype.Parse(gomedium.TTF)
})

// snapDrawText writes a label into the image with its top left at at.
func snapDrawText(img *image.RGBA, at image.Point, s string, size float64) {
	f, err := snapFont()
	if err != nil {
		return
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return
	}
	defer func() { _ = face.Close() }()
	m := face.Metrics()
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(snapMarkColor),
		Face: face,
		Dot:  fixed.P(at.X, at.Y+int(m.Ascent.Round())),
	}
	d.DrawString(s)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
