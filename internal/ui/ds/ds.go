// Package ds is the oh-my-toys design system for MyGo native UI. Every value
// comes from tokens_gen.go, which dev/gentokens generates from
// design/tokens.json; component metrics that the tokens file does not name
// are in metrics.go. One design runs on every platform: nothing here
// branches on the operating system.
package ds

//go:generate go run ../../../dev/gentokens ../../../design/tokens.json tokens_gen.go

import (
	"github.com/egoist/mygo/ui"
)

// P returns the palette of the theme the view is building with.
func P(c *ui.Context) *Palette {
	if c.Theme().Dark {
		return &Dark
	}
	return &Light
}

// S returns the shadows of the theme the view is building with.
func S(c *ui.Context) *Shadows {
	if c.Theme().Dark {
		return &DarkShadows
	}
	return &LightShadows
}

// MyGoTheme maps the tokens onto MyGo's own theme, so the built-in widgets
// (text inputs, scroll bars, focus rings, tooltips) follow the design
// system too.
func MyGoTheme(dark bool) *ui.Theme {
	base, p := ui.LightTheme(), &Light
	if dark {
		base, p = ui.DarkTheme(), &Dark
	}
	t := *base
	t.Background = p.BgCanvas
	t.Surface = p.FillControl
	t.SurfaceHover = p.FillControlHover
	t.SurfacePressed = p.FillControlPressed
	t.Border = p.Line
	t.Text = p.TextPrimary
	t.TextMuted = p.TextSecondary
	t.Accent = p.AccentStrong
	t.AccentText = ui.RGB(255, 255, 255)
	t.Danger = p.Danger
	t.Warning = p.Warning
	t.Success = p.Success
	t.Focus = p.AccentStrong
	t.Selection = p.Selection
	t.Inverse = p.BgPopover
	t.InverseText = p.TextPrimary
	t.Radius = RadiusSm
	t.Spacing = Space4
	t.FontSize = Body.Size
	t.Font = FontSans
	return &t
}

// Style applies a type style of the scale to a text element.
func Style(e ui.Element, s TypeStyle) ui.Element {
	e = e.FontSize(s.Size).FontWeight(s.Weight)
	if s.LineHeight > 0 && s.Size > 0 {
		e = e.LineHeight(s.LineHeight / s.Size)
	}
	if s.Tracking != 0 {
		e = e.LetterSpacing(s.Tracking * s.Size)
	}
	if s.Mono {
		e = e.Font(FontMono)
	}
	return e
}

// ApplyShadow applies a shadow token (all of its layers) to an element.
func ApplyShadow(e ui.Element, layers []Shadow) ui.Element {
	for _, l := range layers {
		e = e.Shadow(l.X, l.Y, l.Blur, l.Spread, l.Color)
	}
	return e
}

// Font weights that the type scale does not name on its own: the weight a
// match is set in, and the plain weight of a value beside a title.
const (
	Regular  = 400
	Semibold = 600
	Bold     = 700
)

// Tabular is the font feature that gives digits one width, for timers,
// counters and table cells.
const Tabular = "tnum"
