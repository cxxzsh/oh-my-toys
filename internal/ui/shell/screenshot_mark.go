package shell

import (
	"image"
	"image/color"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/design"
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
)

// snapMarkColor is the red annotations are drawn in, on screen and in the
// file.
var snapMarkColor = color.RGBA{R: 0xE5, G: 0x3E, B: 0x3E, A: 0xFF}

// snapPaintMarks draws the session's marks that touch one display.
func (a *App) snapPaintMarks(p *ui.Painter, ov *snapOverlay, s *snapSession) {
	c := ui.RGBA(snapMarkColor.R, snapMarkColor.G, snapMarkColor.B, 1)
	for _, m := range s.marks {
		a.snapPaintMark(p, ov, m, c)
	}
	if s.pending != nil {
		a.snapPaintMark(p, ov, *s.pending, c)
	}
}

// snapPaintMark draws one mark in one display's points.
func (a *App) snapPaintMark(p *ui.Painter, ov *snapOverlay, m snapMark, c ui.Color) {
	o := ov.shot.Bounds.Min
	at := func(pt image.Point) (float32, float32) {
		return float32(pt.X - o.X), float32(pt.Y - o.Y)
	}
	switch m.tool {
	case snapToolNone:
	case snapToolArrow:
		for _, ln := range snapArrowLines(m.a, m.b) {
			x0, y0 := at(ln[0])
			x1, y1 := at(ln[1])
			p.StrokePolyline([][2]float32{{x0, y0}, {x1, y1}}, snapMarkWidth, c)
		}
	case snapToolMosaic:
		a.snapPaintMosaic(p, ov, m)
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

// commitMark keeps the mark just drawn, unless it is too small to mean
// anything.
func (s *snapSession) commitMark() {
	m := s.pending
	s.pending = nil
	if m == nil {
		return
	}
	switch m.tool {
	case snapToolNone, snapToolText:
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
	s.marks = append(s.marks, *m)
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
	case snapToolNone:
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

// snapFont parses the interface font once, for the labels written into
// screenshots.
var snapFont = sync.OnceValues(func() (*opentype.Font, error) {
	data, err := design.Read("fonts/Geist-Variable.ttf")
	if err != nil {
		return nil, err
	}
	return opentype.Parse(data)
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
