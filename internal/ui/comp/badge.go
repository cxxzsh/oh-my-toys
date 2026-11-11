package comp

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Kbd is a keyboard shortcut written once with Mod and spelled by the
// caller in the user's key names (⌘K, Ctrl+K): bg-button, line-control,
// radius-xs.
func Kbd(c *ui.Context, keys string) ui.Element {
	return kbd(c, keys, ds.Kbd, ds.Space4)
}

// KbdLarge is a shortcut beside the button that does the same thing: a size
// up, so it reads with the button rather than as a footnote.
func KbdLarge(c *ui.Context, keys string) ui.Element {
	return kbd(c, keys, ds.KbdLg, ds.Space6)
}

func kbd(c *ui.Context, keys string, st ds.TypeStyle, pad float32) ui.Element {
	p := ds.P(c)
	return ds.ApplyShadow(ui.Row(c).Padding(ds.Space2, pad).Radius(ds.RadiusXs).Background(p.BgButton).
		Border(ds.LineWidth, p.LineControl).AlignItems(ui.Center).Children(func() {
		Text(c, keys, st, p.TextSecondary).SingleLine()
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
	b := ui.Row(c).Padding(ds.BadgePadY, ds.Space6).Gap(ds.Space4).Radius(ds.RadiusXs).Background(fill).
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

// RiskBadge draws a tool action's risk class (READ, SYSTEM, WRITE, PROCESS)
// as a compact mono tag with semantic colour and border.
func RiskBadge(c *ui.Context, risk tool.Risk) ui.Element {
	label, tone := riskLabel(risk)
	return badge(c, nil, "", label, tone, tone == ToneNeutral)
}

// riskLabel is the localized word of a risk class and its tone.
func riskLabel(risk tool.Risk) (string, Tone) {
	switch risk {
	case tool.RiskSystem:
		return i18n.RiskSystem(), ToneWarning
	case tool.RiskWrite:
		return i18n.RiskWrite(), ToneAccent
	case tool.RiskProcess:
		return i18n.RiskProcess(), ToneDanger
	case tool.RiskRead, "":
		return i18n.RiskRead(), ToneNeutral
	}
	return i18n.RiskRead(), ToneNeutral
}

// CLICopyPill draws the equivalent omt CLI command as a compact mono pill
// that copies the command to the clipboard when clicked.
func (k *Kit) CLICopyPill(c *ui.Context, cmd, label, toast string) ui.Element {
	p := ds.P(c)
	b := ui.ButtonBase(c).Label(label).Tooltip(cmd).Cursor(ui.CursorDefault)
	if b.Clicked() {
		mygo.Clipboard.WriteText(cmd)
		if toast != "" {
			c.Toast(toast)
		}
	}
	fill, edge := p.FillControl, p.LineControl
	if b.Hovered() {
		fill, edge = p.FillControlHover, p.LineHover
	}
	return b.Height(ds.ControlSm).MaxWidth(ds.CLIMaxWidth).Padding(0, ds.Space8).Gap(ds.Space6).
		Radius(ds.RadiusSm).Background(fill).Border(ds.LineWidth, edge).AlignItems(ui.Center).Children(func() {
		k.Glyph(c, "terminal", ds.GlyphSm, p.TextMuted)
		Value(c, cmd, p.TextSecondary).Ellipsis("…").Shrink(1).MinWidth(0)
		k.Glyph(c, "content_copy", ds.GlyphSm, p.TextMuted)
	})
}
