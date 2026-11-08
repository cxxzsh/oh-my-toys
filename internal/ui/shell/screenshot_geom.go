package shell

import (
	"fmt"
	"image"
	"image/color"
)

// snapGrip is where the pointer is on a committed selection, or how a
// drag in progress changes it.
type snapGrip int

const (
	snapGripNone snapGrip = iota
	snapGripNew           // a drag that draws a new rectangle
	snapGripMove
	snapGripN
	snapGripS
	snapGripE
	snapGripW
	snapGripNE
	snapGripNW
	snapGripSE
	snapGripSW
	snapGripBar   // the toolbar; a press there is not a selection
	snapGripMark  // drawing an annotation inside the selection
	snapGripErase // rubbing annotations out
)

const (
	snapHandle     = 8 // how close to an edge counts as a resize, in points
	snapBarW       = 400
	snapBarH       = 40
	snapBarGap     = 8
	snapLoupeN     = 11 // source pixels on a side
	snapLoupeCell  = 10 // points per source pixel in the loupe
	snapLoupeGap   = 20
	snapLoupeLabel = 18

	snapTextWidth  = 220 // points, the field a label is typed in
	snapEraseReach = 6   // points from an arrow the eraser still takes it

	snapNudge    = 1  // points an arrow key moves the selection
	snapNudgeBig = 10 // points with Shift, to cross a gap quickly
)

// snapHit reports what a press at p would do. The toolbar wins, then
// corners, then edges, then the inside of the selection.
func snapHit(sel, bar image.Rectangle, p image.Point) snapGrip {
	if !bar.Empty() && p.In(bar) {
		return snapGripBar
	}
	if sel.Empty() {
		return snapGripNone
	}
	nw, ne := sel.Min, image.Pt(sel.Max.X, sel.Min.Y)
	sw, se := image.Pt(sel.Min.X, sel.Max.Y), sel.Max
	switch {
	case near(p, nw):
		return snapGripNW
	case near(p, ne):
		return snapGripNE
	case near(p, sw):
		return snapGripSW
	case near(p, se):
		return snapGripSE
	case onEdge(p.Y, sel.Min.Y, p.X, sel.Min.X, sel.Max.X):
		return snapGripN
	case onEdge(p.Y, sel.Max.Y, p.X, sel.Min.X, sel.Max.X):
		return snapGripS
	case onEdge(p.X, sel.Min.X, p.Y, sel.Min.Y, sel.Max.Y):
		return snapGripW
	case onEdge(p.X, sel.Max.X, p.Y, sel.Min.Y, sel.Max.Y):
		return snapGripE
	case p.In(sel):
		return snapGripMove
	default:
		return snapGripNone
	}
}

func near(p, q image.Point) bool {
	dx, dy := p.X-q.X, p.Y-q.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx <= snapHandle && dy <= snapHandle
}

func onEdge(v, edge, along, lo, hi int) bool {
	d := v - edge
	if d < 0 {
		d = -d
	}
	return d <= snapHandle && along >= lo && along <= hi
}

// snapResize moves the edges grip names. The opposite edges stay.
func snapResize(origin image.Rectangle, g snapGrip, p image.Point) image.Rectangle {
	r := origin
	if g == snapGripN || g == snapGripNW || g == snapGripNE {
		r.Min.Y = p.Y
	}
	if g == snapGripS || g == snapGripSW || g == snapGripSE {
		r.Max.Y = p.Y
	}
	if g == snapGripW || g == snapGripNW || g == snapGripSW {
		r.Min.X = p.X
	}
	if g == snapGripE || g == snapGripNE || g == snapGripSE {
		r.Max.X = p.X
	}
	return r.Canon()
}

// snapMove shifts origin by to-from and keeps it inside limit.
func snapMove(origin image.Rectangle, from, to image.Point, limit image.Rectangle) image.Rectangle {
	r := origin.Add(to.Sub(from))
	if r.Dx() > limit.Dx() || r.Dy() > limit.Dy() {
		return r
	}
	if r.Min.X < limit.Min.X {
		r = r.Add(image.Pt(limit.Min.X-r.Min.X, 0))
	}
	if r.Max.X > limit.Max.X {
		r = r.Add(image.Pt(limit.Max.X-r.Max.X, 0))
	}
	if r.Min.Y < limit.Min.Y {
		r = r.Add(image.Pt(0, limit.Min.Y-r.Min.Y))
	}
	if r.Max.Y > limit.Max.Y {
		r = r.Add(image.Pt(0, limit.Max.Y-r.Max.Y))
	}
	return r
}

// snapBarPlace puts a w×h toolbar just under the selection, its right edge
// on the selection's, or above it when the bottom edge has no room, and
// clamps it to limit.
func snapBarPlace(sel, limit image.Rectangle, w, h int) image.Rectangle {
	x := sel.Max.X - w
	y := sel.Max.Y + snapBarGap
	if y+h > limit.Max.Y {
		y = sel.Min.Y - snapBarGap - h
	}
	if y < limit.Min.Y {
		y = limit.Min.Y
	}
	if x < limit.Min.X {
		x = limit.Min.X
	}
	if x+w > limit.Max.X {
		x = limit.Max.X - w
	}
	return image.Rect(x, y, x+w, y+h)
}

// snapWindowAt is the window under p in front of the others, which a click
// takes as the selection, or the empty rectangle when no window is there.
// The frames come front to back, as they are stacked on screen.
func snapWindowAt(frames []image.Rectangle, p image.Point) image.Rectangle {
	for _, f := range frames {
		if p.In(f) {
			return f
		}
	}
	return image.Rectangle{}
}

// snapLoupeBox is the magnified square near at, flipped so it stays
// inside bounds. The label sits just under the square.
func snapLoupeBox(at image.Point, bounds image.Rectangle) image.Rectangle {
	size := snapLoupeN * snapLoupeCell
	x, y := at.X+snapLoupeGap, at.Y+snapLoupeGap
	if x+size > bounds.Max.X {
		x = at.X - snapLoupeGap - size
	}
	if y+size+snapLoupeLabel > bounds.Max.Y {
		y = at.Y - snapLoupeGap - size - snapLoupeLabel
	}
	if x+size > bounds.Max.X {
		x = bounds.Max.X - size
	}
	if y+size > bounds.Max.Y {
		y = bounds.Max.Y - size
	}
	if x < bounds.Min.X {
		x = bounds.Min.X
	}
	if y < bounds.Min.Y {
		y = bounds.Min.Y
	}
	return image.Rect(x, y, x+size, y+size)
}

// snapHex is the color as #RRGGBB.
func snapHex(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}
