package comp

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// ButtonKind picks a button's look.
type ButtonKind int

// The looks of buttons.
const (
	Secondary   ButtonKind = iota // bg-button with a rim: the default
	Primary                       // inverse: one per view
	Ghost                         // fill-control
	Plain                         // no fill until hovered: toolbars and the tray
	Danger                        // tinted: Kill in rows and pages
	DangerSolid                   // only a dialog's confirm
)

// ButtonOpts describe a button besides its label.
type ButtonOpts struct {
	Kind     ButtonKind
	Glyph    string // an optional 13pt glyph before the label
	Small    bool   // control-sm 20pt, inside panels
	Square   bool   // radius-xs: in-row actions of a table
	Disabled bool
	Tooltip  string // names an icon-only button
}

type buttonColors struct {
	fill, ink, line ui.Color
	weight          int
	raised          bool
}

func buttonLook(p *ds.Palette, k ButtonKind) buttonColors {
	switch k {
	case Primary:
		return buttonColors{fill: p.Inverse, ink: p.OnInverse, weight: ds.Semibold, raised: true}
	case Ghost:
		return buttonColors{fill: p.FillControl, ink: p.TextSecondary, weight: ds.Label.Weight}
	case Plain:
		return buttonColors{fill: ui.Transparent, ink: p.TextSecondary, weight: ds.Label.Weight}
	case Danger:
		return buttonColors{fill: p.DangerSoft, ink: p.Danger, line: p.DangerLine, weight: ds.Label.Weight}
	case DangerSolid:
		return buttonColors{fill: p.DangerSolid, ink: p.OnDanger, weight: ds.Semibold}
	case Secondary:
	}
	return buttonColors{fill: p.BgButton, ink: p.TextStrong, line: p.LineControl, weight: ds.Label.Weight, raised: true}
}

// hoverFill is the fill of a button under the pointer; ok is false where
// the kind dims instead.
func hoverFill(p *ds.Palette, k ButtonKind) (ui.Color, bool) {
	switch k {
	case Secondary, Ghost:
		return p.FillControlHover, true
	case Plain:
		return p.FillControl, true
	case Primary, Danger, DangerSolid:
	}
	return ui.Transparent, false
}

// Button is a pill push button whose label is a verb with its target. It
// returns the element: ask it Clicked. Give each button of a view its own
// key with c.Key.
func (k *Kit) Button(c *ui.Context, label string, o ButtonOpts) ui.Element {
	p := ds.P(c)
	look := buttonLook(p, o.Kind)
	b := ui.ButtonBase(c).Label(label).Cursor(ui.CursorDefault).Disabled(o.Disabled)
	if o.Tooltip != "" {
		b.Tooltip(o.Tooltip)
	}
	fill, opacity := look.fill, float32(1)
	hover, fills := hoverFill(p, o.Kind)
	switch {
	case o.Disabled:
		opacity = ds.OpacityDisabled
	case b.Pressed() && fills:
		fill = p.FillControlPressed
	case b.Hovered() && fills:
		fill = hover
	case b.Hovered():
		opacity = ds.OpacityHover
	}
	height, style := float32(ds.ControlMd), ds.Label
	if o.Small {
		height, style = ds.ControlSm, ds.Caption
	}
	style.Weight = look.weight
	radius, padX := float32(ds.RadiusSm), float32(ds.Space10)
	if o.Square {
		radius, padX = ds.RadiusXs, ds.Space8
	}
	b.Height(height).Padding(0, padX).Gap(ds.Space4).Radius(radius).Background(fill).Opacity(opacity).
		AlignItems(ui.Center).Justify(ui.Center)
	if label == "" {
		b.Width(height).Padding(0)
	}
	if look.line != ui.Transparent {
		b.Border(ds.LineWidth, look.line)
	}
	if look.raised {
		ds.ApplyShadow(b, ds.S(c).ShadowXs)
	}
	return b.Children(func() {
		if o.Glyph != "" {
			k.Glyph(c, o.Glyph, glyphSize(label), look.ink)
		}
		if label != "" {
			Text(c, label, style, look.ink).SingleLine()
		}
	})
}

// glyphSize is the glyph of a button: 13pt beside a label, 16pt alone.
func glyphSize(label string) float32 {
	if label == "" {
		return ds.Glyph
	}
	return ds.GlyphSm
}
