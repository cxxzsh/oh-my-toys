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
	seg.Track.Label(label).Height(ds.ControlMd).Padding(ds.Space2).Radius(ds.RadiusSm).
		Background(p.FillControl).Border(ds.LineWidth, p.LineHairline).AlignItems(ui.Center)
	seg.Track.Children(func() {
		for i, name := range options {
			chosen := i == *selected
			s := seg.Segment(i).Label(name).Cursor(ui.CursorDefault).Radius(ds.RadiusXs).AlignItems(ui.Center)
			style, ink, padX := ds.Caption, p.TextInactive, float32(ds.Space8)
			if chosen {
				style.Weight, ink, padX = ds.Semibold, p.TextStrong, ds.Space10
				s.Background(p.BgSelected).Border(ds.LineWidth, p.LineControl)
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
	style, ink, padX := ds.Caption, p.TextSecondary, float32(ds.Space10)
	style.Weight = ds.Label.Weight
	fill, edge := p.BgButton, p.LineControl
	if active {
		style.Weight, ink, padX = ds.Semibold, p.AccentText, ds.Space12
		fill, edge = p.AccentSoft, p.AccentLine
	} else if b.Hovered() {
		fill, ink = p.FillControlHover, p.TextStrong
	}
	b.Height(ds.ControlSm).Radius(ds.RadiusSm).AlignItems(ui.Center).Background(fill).Border(ds.LineWidth, edge)
	return b.Padding(0, padX).Children(func() { Text(c, label, style, ink).SingleLine() })
}
