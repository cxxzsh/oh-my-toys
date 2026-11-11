package shell

import (
	"fmt"
	"math"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

var tonalSteps = []struct {
	name string
	l    float64
}{
	{"50", 0.95},
	{"100", 0.89},
	{"200", 0.79},
	{"300", 0.67},
	{"400", 0.56},
	{"500", 0.46},
	{"600", 0.37},
	{"700", 0.27},
	{"800", 0.18},
	{"900", 0.11},
}

func (a *App) colorStudioView(c *ui.Context, t tool.Action, s *actionState, hex string, rows []tool.Row) {
	ui.Column(c).Grow(1).FillWidth().Gap(ds.Space12).Children(func() {
		a.colorHeroCard(c, t, s, hex)
		a.colorDetailRow(c, hex, rows)
	})
}

// colorDetailRow puts the format values beside the contrast previews, as two
// cards of equal height.
func (a *App) colorDetailRow(c *ui.Context, hex string, rows []tool.Row) {
	formats, contrasts := splitColorRows(rows)
	ui.Row(c).FillWidth().Gap(ds.Space12).AlignItems(ui.Stretch).Wrap().Children(func() {
		a.colorFormatsCard(c, formats)
		a.colorContrastCard(c, hex, contrasts)
	})
}

// splitColorRows separates the format rows, which carry a value to copy, from
// the contrast rows.
func splitColorRows(rows []tool.Row) (formats, contrasts []tool.Row) {
	for _, r := range rows {
		if r.Value != "" {
			formats = append(formats, r)
		} else {
			contrasts = append(contrasts, r)
		}
	}
	return formats, contrasts
}

func (a *App) colorFormatsCard(c *ui.Context, formats []tool.Row) {
	comp.Card(c).Grow(1).Basis(0).MinWidth(ds.CardMinColumn).FillHeight().Padding(ds.Space12).Gap(ds.Space8).Children(func() {
		comp.Eyebrow(c, i18n.ColorFormats())
		for _, r := range formats {
			a.colorFormatButton(c, r)
		}
	})
}

// colorFormatButton is one format; a click copies its value.
func (a *App) colorFormatButton(c *ui.Context, r tool.Row) {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key("fmt:" + r.Title)).Cursor(ui.CursorDefault)
	if b.Clicked() {
		mygo.Clipboard.WriteText(r.Value)
		c.Toast(i18n.Copied(r.Value))
	}
	fill := ui.Transparent
	if b.Hovered() {
		fill = p.FillControlHover
	}
	b.FillWidth().Height(ds.TableRow).Padding(0, ds.Space10).Radius(ds.RadiusSm).
		Background(fill).Border(ds.LineWidth, p.LineHairline).
		Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		comp.Text(c, r.Title, ds.Caption, p.TextMuted).SingleLine()
		ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
			comp.Value(c, r.Detail, p.TextPrimary)
			a.kit.Glyph(c, "content_copy", ds.GlyphSm, p.TextMuted)
		})
	})
}

func (a *App) colorContrastCard(c *ui.Context, hex string, contrasts []tool.Row) {
	p := ds.P(c)
	surfaces := []ui.Color{p.ContrastWhite, p.ContrastBlack}
	comp.Card(c).Grow(1).Basis(0).MinWidth(ds.CardMinColumn).FillHeight().Padding(ds.Space12).Gap(ds.Space8).Children(func() {
		comp.Eyebrow(c, i18n.ColorContrastCheck())
		for i, r := range contrasts {
			if i < len(surfaces) {
				contrastPreview(c, hex, r, surfaces[i])
			}
		}
	})
}

// contrastPreview draws the swatch's color as sample text on one surface, with
// the contrast verdict beneath it.
func contrastPreview(c *ui.Context, hex string, r tool.Row, surface ui.Color) {
	p := ds.P(c)
	tone := toneDetailColor(p, r.Tone)
	ui.Column(c.Key("cprev:" + r.Title)).FillWidth().Gap(ds.Space6).Children(func() {
		ui.Column(c).FillWidth().Background(surface).Border(ds.LineWidth, p.LineControl).
			Radius(ds.RadiusSm).Padding(ds.Space10, ds.Space12).Children(func() {
			comp.Text(c, i18n.ColorSampleText(), ds.Headline, ui.Hex(hex)).SingleLine()
		})
		ui.Row(c).FillWidth().Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(ds.Space8, ds.Space8).Radius(ds.RadiusFull).Background(tone).Shrink(0)
			comp.Text(c, r.Title, ds.Caption, p.TextSecondary)
			ui.Spacer(c)
			comp.Text(c, r.Detail, ds.Caption, tone)
		})
	})
}

func (a *App) colorHeroCard(c *ui.Context, t tool.Action, s *actionState, hex string) {
	p := ds.P(c)
	col := ui.Hex(hex)
	h, sat := hexHueSat(hex)
	comp.Card(c).FillWidth().Padding(ds.Space12).Gap(ds.Space12).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space16).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Row(c).Height(64).MinWidth(220).Padding(0, ds.Space16).Gap(ds.Space16).
				Radius(ds.RadiusSm).Background(col).Border(ds.LineWidth, p.LineControl).
				AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				comp.Text(c, "Aa  "+hex, ds.Headline, p.ContrastWhite)
				comp.Text(c, "Aa", ds.Headline, p.ContrastBlack)
			})
			ui.Column(c).Grow(1).MinWidth(260).Gap(ds.Space6).Children(func() {
				comp.Eyebrow(c, i18n.TonalScale())
				ui.Row(c).FillWidth().Gap(ds.Space4).Children(func() {
					for _, st := range tonalSteps {
						shade := hslHex(h, sat, st.l)
						a.tonalStepTile(c, t, s, st.name, shade)
					}
				})
			})
		})
	})
}

func (a *App) tonalStepTile(c *ui.Context, t tool.Action, s *actionState, step, hex string) {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key("tone:" + step)).Label(step + " " + hex).Tooltip(hex).Cursor(ui.CursorDefault)
	if b.Clicked() {
		mygo.Clipboard.WriteText(hex)
		c.Toast(i18n.Copied(hex))
		if _, ok := s.input["color"]; ok {
			a.setFieldAndReplan(t, s, "color", hex)
		}
	}
	b.Column().Grow(1).Basis(0).Gap(ds.Space2).AlignItems(ui.Center).Children(func() {
		ui.Box(c).FillWidth().Height(28).Radius(ds.RadiusXs).Background(ui.Hex(hex)).Border(ds.LineWidth, p.LineControl)
		comp.Text(c, step, ds.Kbd, p.TextMuted).SingleLine()
	})
}

func hexHueSat(hex string) (h, s float64) {
	if len(hex) != 7 || hex[0] != '#' {
		return 220, 0.8
	}
	rv, _ := strconv.ParseUint(hex[1:3], 16, 8)
	gv, _ := strconv.ParseUint(hex[3:5], 16, 8)
	bv, _ := strconv.ParseUint(hex[5:7], 16, 8)
	r, g, b := float64(rv)/255, float64(gv)/255, float64(bv)/255
	mx, mn := max(r, max(g, b)), min(r, min(g, b))
	d := mx - mn
	l := (mx + mn) / 2
	if d == 0 {
		return 0, 0
	}
	s = d / (1 - math.Abs(2*l-1))
	switch mx {
	case r:
		h = 60 * math.Mod((g-b)/d, 6)
	case g:
		h = 60 * ((b-r)/d + 2)
	default:
		h = 60 * ((r-g)/d + 4)
	}
	if h < 0 {
		h += 360
	}
	return h, s
}

func hslHex(h, s, l float64) string {
	ch := (1 - math.Abs(2*l-1)) * s
	x := ch * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - ch/2
	var r1, g1, b1 float64
	switch {
	case h < 60:
		r1, g1 = ch, x
	case h < 120:
		r1, g1 = x, ch
	case h < 180:
		g1, b1 = ch, x
	case h < 240:
		g1, b1 = x, ch
	case h < 300:
		r1, b1 = x, ch
	default:
		r1, b1 = ch, x
	}
	clampByte := func(v float64) uint8 { return uint8(math.Round(max(0, min(1, v)) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", clampByte(r1+m), clampByte(g1+m), clampByte(b1+m))
}
