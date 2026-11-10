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
		comp.Card(c).Grow(1).FillWidth().Clip().Children(func() {
			ui.List(c.Key("crows"), &s.rows, len(rows), func(i int) { a.plainRow(c, rows[i]) }).
				Grow(1).FillWidth().Padding(ds.Space8).Label(i18n.Preview())
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
				comp.Text(c, "Aa  "+hex, ds.Headline, ui.Hex("#ffffff"))
				comp.Text(c, "Aa", ds.Headline, ui.Hex("#0b0f19"))
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
