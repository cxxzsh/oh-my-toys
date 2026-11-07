package comp

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Segmented is two to four mutually exclusive options in a pill track; the
// chosen one rises as a white thumb. Ask the returned element Changed.
func Segmented(c *ui.Context, selected *int, options []string, label string) ui.Element {
	p := ds.P(c)
	seg := ui.SegmentedBase(c, selected, len(options))
	seg.Track.Label(label).Height(ds.ControlMd).Padding(ds.Space2).Radius(ds.RadiusFull).
		Background(p.FillControl).Border(ds.LineWidth, p.LineHairline).AlignItems(ui.Center)
	seg.Track.Children(func() {
		for i, name := range options {
			chosen := i == *selected
			s := seg.Segment(i).Label(name).Cursor(ui.CursorDefault).Radius(ds.RadiusFull).AlignItems(ui.Center)
			style, ink, padX := ds.Caption, p.TextInactive, float32(ds.Space8)
			if chosen {
				style.Weight, ink, padX = ds.Semibold, p.OnThumb, ds.Space10
				s.Background(p.Thumb)
				ds.ApplyShadow(s, ds.S(c).ShadowXs)
			}
			s.Padding(0, padX).Children(func() { Text(c, name, style, ink).SingleLine() })
		}
	})
	return seg.Track
}

// Chip is a filter pill above a table or grid; exactly one is active. The
// count comes from the data.
func Chip(c *ui.Context, label string, active bool) ui.Element {
	p := ds.P(c)
	b := ui.ButtonBase(c).Label(label).Selected(active).Cursor(ui.CursorDefault)
	style, ink, padX := ds.Caption, p.TextInactive, float32(ds.Space10)
	style.Weight = ds.Label.Weight
	b.Height(ds.ControlMd).Radius(ds.RadiusFull).AlignItems(ui.Center)
	if active {
		style.Weight, ink, padX = ds.Semibold, p.OnInverse, ds.Space12
		b.Background(p.Inverse)
	} else {
		if b.Hovered() {
			ink = p.TextStrong
		}
		b.Background(p.BgCard).Border(ds.LineWidth, p.LineControl)
		ds.ApplyShadow(b, ds.S(c).ShadowXs)
	}
	return b.Padding(0, padX).Children(func() { Text(c, label, style, ink).SingleLine() })
}
