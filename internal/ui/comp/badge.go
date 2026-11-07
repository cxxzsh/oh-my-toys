package comp

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Kbd is a keyboard shortcut written once with Mod and spelled by the
// caller in the user's key names (⌘K, Ctrl+K): bg-button, line-control,
// radius-xs.
func Kbd(c *ui.Context, keys string) ui.Element {
	p := ds.P(c)
	return ds.ApplyShadow(ui.Row(c).Padding(0, ds.Space4).Radius(ds.RadiusXs).Background(p.BgButton).
		Border(ds.LineWidth, p.LineControl).AlignItems(ui.Center).Children(func() {
		Text(c, keys, ds.Kbd, p.TextSecondary).SingleLine()
	}), ds.S(c).ShadowXs)
}

// Tone colours a badge, a dot or a status.
type Tone int

// The tones of the design system.
const (
	ToneNeutral Tone = iota
	ToneAccent
	ToneSuccess
	ToneWarning
	ToneDanger
)

func toneColors(p *ds.Palette, t Tone) (fill, ink, line ui.Color) {
	switch t {
	case ToneAccent:
		return p.AccentSoft, p.AccentText, p.AccentLine
	case ToneSuccess:
		return p.SuccessSoft, p.Success, ui.Transparent
	case ToneWarning:
		return p.WarningSoft, p.Warning, ui.Transparent
	case ToneDanger:
		return p.DangerSoft, p.Danger, ui.Transparent
	case ToneNeutral:
	}
	return p.FillControl, p.TextSecondary, ui.Transparent
}

// Badge is a mono pill for a count or a state beside a title or in a row.
// Status colour always comes with the word. A rimmed neutral badge sits
// beside page titles.
func Badge(c *ui.Context, text string, tone Tone) ui.Element {
	return badge(c, nil, "", text, tone, false)
}

// BadgeRimmed is a neutral badge with a line-hairline rim, for beside a
// page title.
func BadgeRimmed(c *ui.Context, text string) ui.Element {
	return badge(c, nil, "", text, ToneNeutral, true)
}

// BadgeGlyph is a badge with a 13pt glyph before the word, as the accent
// Active badge's check.
func (k *Kit) BadgeGlyph(c *ui.Context, glyph, text string, tone Tone) ui.Element {
	return badge(c, k, glyph, text, tone, false)
}

func badge(c *ui.Context, k *Kit, glyph, text string, tone Tone, rimmed bool) ui.Element {
	p := ds.P(c)
	fill, ink, line := toneColors(p, tone)
	if rimmed {
		line = p.LineHairline
	}
	b := ui.Row(c).Padding(ds.BadgePadY, ds.Space6).Gap(ds.Space4).Radius(ds.RadiusFull).Background(fill).
		AlignItems(ui.Center)
	if line != ui.Transparent {
		b.Border(ds.LineWidth, line)
	}
	return b.Children(func() {
		if k != nil && glyph != "" {
			k.Glyph(c, glyph, ds.GlyphSm, ink)
		}
		Text(c, text, ds.Badge, ink).SingleLine()
	})
}
