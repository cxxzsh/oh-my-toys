package comp

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Switch is the on/off control of a switch tool or of a setting that
// applies at once: 38×22, the knob sliding in 150ms. The label is its
// accessible name.
func Switch(c *ui.Context, on *bool, label string) ui.Element {
	return drawSwitch(c, on, label, ds.SwitchWidth, ds.SwitchHeight)
}

// SwitchCompact is the 30×18 switch of the tray panel and the palette.
func SwitchCompact(c *ui.Context, on *bool, label string) ui.Element {
	return drawSwitch(c, on, label, ds.SwitchCompactW, ds.SwitchCompactH)
}

func drawSwitch(c *ui.Context, on *bool, label string, w, h float32) ui.Element {
	p := ds.P(c)
	track, edge := p.FillControlHover, p.LineHover
	if *on {
		track, edge = p.AccentStrong, p.AccentStrong
	}
	knob := h - 2*ds.SwitchInset
	sw := ui.SwitchBase(c, on).Label(label).Cursor(ui.CursorDefault)
	x := float32(ds.SwitchInset)
	if *on {
		x = w - knob - ds.SwitchInset
	}
	x = sw.Animate("knob", x, ds.EaseDuration)
	return sw.Size(w, h).Radius(ds.RadiusFull).Background(track).Border(ds.LineWidth, edge).Children(func() {
		ds.ApplyShadow(ui.Box(c).Absolute().Top(ds.SwitchInset).Left(x).Size(knob, knob).
			Radius(ds.RadiusFull).Background(p.Thumb), ds.S(c).ShadowThumb)
	})
}

// Checkbox is an independent option inside a tool panel: a 14pt box and
// its label, the whole label the hit target. A muted checkbox is for
// advanced options.
func (k *Kit) Checkbox(c *ui.Context, v *bool, label string, muted bool) ui.Element {
	p := ds.P(c)
	box, edge := p.FillControl, p.LineHover
	if *v {
		box, edge = p.AccentStrong, p.AccentStrong
	}
	ink := p.TextPrimary
	if muted {
		ink = p.TextInactive
	}
	cb := ui.CheckboxBase(c, v).Label(label).Cursor(ui.CursorDefault)
	style := ds.Label
	style.Weight = ds.Regular
	return cb.Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(ds.CheckboxSize, ds.CheckboxSize).Radius(ds.RadiusXs).Background(box).
			Border(ds.LineWidth, edge).Center().Children(func() {
			if *v {
				k.Glyph(c, "check", ds.CheckTickSize, p.Thumb)
			}
		})
		Text(c, label, style, ink).SingleLine()
	})
}
