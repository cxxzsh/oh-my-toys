// Package ds is the oh-my-toys design system for MyGo native UI. Every value
// comes from tokens_gen.go, which cmd/gentokens generates from
// design/tokens.json; component metrics that the tokens file does not name
// (row heights, inner icon sizes) are in metrics.go, copied from the design
// system's components/bundle.css.
package ds

//go:generate go run ../../../dev/gentokens ../../../design/tokens.json tokens_gen.go

import (
	"runtime"

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
	t.Background = p.BgWindow
	t.Surface = p.FillControl
	t.SurfaceHover = p.FillHover
	t.SurfacePressed = p.FillSelected
	t.Border = p.Separator
	t.Text = p.TextPrimary
	t.TextMuted = p.TextSecondary
	t.Accent = p.AccentFill
	t.AccentText = p.OnAccent
	t.Danger = p.Danger
	t.Warning = p.Warning
	t.Success = p.Success
	t.Focus = p.FocusRing
	t.Selection = p.Accent.Alpha(0.25)
	t.Radius = RadiusMd
	t.Spacing = Space1
	t.FontSize = Body.Size
	if runtime.GOOS == "windows" {
		t.FontSize = 14 // README: Windows body text is 14px
	}
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
		e = e.Font(MonoFont())
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

// MonoFont is the first family of the tokens' mono stack the platform has.
func MonoFont() string {
	switch runtime.GOOS {
	case "darwin":
		return "SF Mono"
	case "windows":
		return "Cascadia Mono"
	}
	return "monospace"
}

// Weights not in the type scale.
const (
	Regular = 400
	Bold    = 700
)

// Styles derived from the scale: a count beside a section's title, and
// callout text in the headline's weight, as a small button's.
var (
	Count         = TypeStyle{Size: Eyebrow.Size, LineHeight: Eyebrow.LineHeight, Weight: Regular}
	CalloutStrong = TypeStyle{Size: Callout.Size, LineHeight: Callout.LineHeight, Weight: Headline.Weight}
)
