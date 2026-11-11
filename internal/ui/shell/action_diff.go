package shell

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// splitCommonAffix splits two strings at their common rune prefix and their
// common rune suffix, which never overlap. delMid is what from has and to
// lacks; addMid is what to has and from lacks.
func splitCommonAffix(from, to string) (prefix, delMid, addMid, suffix string) {
	a, b := []rune(from), []rune(to)
	n := min(len(a), len(b))
	head := 0
	for head < n && a[head] == b[head] {
		head++
	}
	tail := 0
	for tail < n-head && a[len(a)-1-tail] == b[len(b)-1-tail] {
		tail++
	}
	return string(a[:head]), string(a[head : len(a)-tail]), string(b[head : len(b)-tail]), string(a[len(a)-tail:])
}

// diffFrom draws the old value of a changed row, marking what goes away.
func diffFrom(c *ui.Context, r tool.Row, p *ds.Palette) {
	prefix, delMid, _, suffix := splitCommonAffix(r.Cells[0], r.Cells[1])
	spans := []ui.Span{{Text: prefix, Color: p.TextSecondary}}
	if delMid != "" {
		spans = append(spans, ui.Span{Text: delMid, Color: p.Danger, Weight: ds.Semibold})
	}
	spans = append(spans, ui.Span{Text: suffix, Color: p.TextSecondary})
	ds.Style(ui.RichText(c, spans...), ds.Meta).Ellipsis("…").Shrink(1).MinWidth(0)
}

// diffTo draws the new value of a changed row, marking what is added.
func diffTo(c *ui.Context, r tool.Row, p *ds.Palette) {
	prefix, _, addMid, suffix := splitCommonAffix(r.Cells[0], r.Cells[1])
	spans := []ui.Span{{Text: prefix, Color: p.TextPrimary}}
	if addMid != "" {
		spans = append(spans, ui.Span{Text: addMid, Color: p.Success, Weight: ds.Semibold})
	}
	spans = append(spans, ui.Span{Text: suffix, Color: p.TextPrimary})
	ds.Style(ui.RichText(c, spans...), ds.Meta).Ellipsis("…").Shrink(1).MinWidth(0)
}

// swatchOf returns the colour a row names, from its value or its detail.
func swatchOf(r tool.Row) (ui.Color, bool) {
	if col, ok := parseHexSwatch(r.Value); ok {
		return col, true
	}
	return parseHexSwatch(r.Detail)
}

// parseHexSwatch reads "#" and six hex digits, as a colour.
func parseHexSwatch(s string) (ui.Color, bool) {
	t := strings.TrimSpace(s)
	if len(t) != 7 || t[0] != '#' {
		return ui.Transparent, false
	}
	for i := 1; i < len(t); i++ {
		if !isHexDigit(t[i]) {
			return ui.Transparent, false
		}
	}
	return ui.Hex(t), true
}

func isHexDigit(b byte) bool {
	return ('0' <= b && b <= '9') || ('a' <= b && b <= 'f') || ('A' <= b && b <= 'F')
}

// swatchBox draws a crisp 16 px colour chip, outlined so light colours show.
func swatchBox(c *ui.Context, col ui.Color, p *ds.Palette) {
	ui.Box(c).Width(16).Height(16).Shrink(0).Radius(ds.RadiusXs).Background(col).
		Border(ds.LineWidth, p.LineControl)
}

// dotFor maps a row tone to a status dot; normal rows have none.
func dotFor(t tool.Tone) (comp.DotTone, bool) {
	switch t {
	case tool.ToneSuccess:
		return comp.DotLive, true
	case tool.ToneWarning:
		return comp.DotWarning, true
	case tool.ToneDanger:
		return comp.DotDanger, true
	case tool.ToneNormal:
	}
	return comp.DotMuted, false
}

// plainDetail draws the detail of a plain row, with a status dot when the
// row carries a tone.
func plainDetail(c *ui.Context, r tool.Row, ink ui.Color) {
	value := func() {
		comp.Value(c, r.Detail, ink).Ellipsis("…").Shrink(1).MinWidth(0)
	}
	dot, ok := dotFor(r.Tone)
	if !ok {
		value()
		return
	}
	ui.Row(c).Gap(ds.Space6).AlignItems(ui.Center).Shrink(1).MinWidth(0).Children(func() {
		comp.StatusDot(c, comp.DotOpts{Tone: dot})
		value()
	})
}
