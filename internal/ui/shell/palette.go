package shell

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

type paletteState struct {
	open     bool
	query    string
	last     string
	searched bool
	results  []core.Result
	chosen   int
	input    ui.Handle
}

func (a *App) setUpPalette() {}

func (a *App) togglePalette() {
	if a.pal.open {
		a.pal.open = false
		a.redraw()
		return
	}
	a.showPalette()
}

// showPalette opens the command palette inside the main window.
func (a *App) showPalette() {
	if a.main != nil && !a.main.IsVisible() {
		a.openMain("")
	}
	a.pal = paletteState{open: true, input: a.pal.input}
	a.pal.input.Focus()
	a.redraw()
}

func (a *App) recentResults() []core.Result {
	tools := a.recentTools()
	if len(tools) == 0 {
		tools = a.reg.All()
		tools = tools[:min(len(tools), ds.PaletteMaxRows)]
	}
	out := make([]core.Result, 0, len(tools))
	for _, t := range tools {
		out = append(out, core.Result{Tool: t})
	}
	return out
}

func (a *App) search() {
	if a.pal.searched && a.pal.query == a.pal.last {
		return
	}
	t0 := time.Now()
	a.pal.searched, a.pal.last, a.pal.chosen = true, a.pal.query, 0
	if a.pal.query == "" {
		a.pal.results = a.recentResults()
		return
	}
	a.pal.results = a.reg.Index().Search(a.pal.query, msgsLang(), ds.PaletteMaxRows)
	a.metrics.keyChanged(time.Since(t0))
}

func (a *App) paletteKeys(c *ui.Context) {
	rs := a.pal.results
	if c.Shortcut(0, ui.KeyEscape) {
		if a.pal.query != "" {
			a.pal.query = ""
		} else {
			a.pal.open = false
		}
		return
	}
	if c.Shortcut(0, ui.KeyDown) && len(rs) > 0 {
		a.pal.chosen = min(a.pal.chosen+1, len(rs)-1)
	}
	if c.Shortcut(0, ui.KeyUp) && len(rs) > 0 {
		a.pal.chosen = max(a.pal.chosen-1, 0)
	}
	if a.pal.chosen >= len(rs) {
		return
	}
	r := rs[a.pal.chosen]
	if s, ok := r.Tool.(tool.Switch); ok && r.Action == nil && c.Shortcut(0, ui.KeySpace) {
		a.setSwitch(s, !s.Status().On)
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyEnter) {
		a.pal.open = false
		a.activate(r.Tool)
	} else if c.Shortcut(0, ui.KeyEnter) {
		a.pal.open = false
		a.runResult(r)
	}
}

func (a *App) paletteOverlay(c *ui.Context) {
	_, h := c.Size()
	ui.Box(c).Absolute().Top(h * ds.PaletteTopShare).Left(0).Right(0).AlignItems(ui.Center).Children(func() {
		a.paletteCard(c)
	})
}

func (a *App) paletteView(c *ui.Context) {
	a.theme(c)
	a.paletteCard(c)
}

func (a *App) paletteCard(c *ui.Context) {
	p := ds.P(c)
	a.search()
	a.paletteKeys(c)
	card := ui.Column(c).Width(ds.PaletteWidth).Radius(ds.RadiusLg).Background(p.BgPopover).
		Border(ds.LineWidth, p.LineControl).Clip()
	ds.ApplyShadow(card, ds.S(c).ShadowPopover)
	if card.PressedOutside() {
		a.pal.open = false
	}
	card.Children(func() {
		a.paletteInput(c)
		a.paletteList(c)
		a.paletteFoot(c)
	}).DrawOver(func(*ui.Painter, ui.Rect) { a.metrics.keyPainted() })
}

func (a *App) paletteInput(c *ui.Context) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.PaletteInputRow).Padding(0, ds.Space12, 0, ds.Space16).Gap(ds.Space10).
		AlignItems(ui.Center).BorderWidth(0, 0, ds.LineWidth, 0).BorderColor(p.Line).Children(func() {
		a.kit.Glyph(c, "search", ds.PaletteGlyph, p.TextMuted)
		in := ui.TextInputBase(c, &a.pal.query).Bind(&a.pal.input).Grow(1).Placeholder(i18n.PaletteHint()).
			PlaceholderColor(p.TextMuted).TextColor(p.TextStrong).Label(i18n.PaletteHint())
		ds.Style(in, ds.PaletteInput)
		comp.Kbd(c, "Esc")
	})
}

func (a *App) paletteList(c *ui.Context) {
	rs := a.pal.results
	ui.Column(c).FillWidth().Padding(ds.Space6).Children(func() {
		if len(rs) == 0 {
			a.paletteEmpty(c)
			return
		}
		section := ""
		for i, r := range rs {
			if s := a.sectionOf(r); s != section {
				section = s
				comp.Eyebrow(c, s).Padding(ds.Space6, ds.Space10, ds.Space4)
			}
			a.paletteRow(c, i, r)
		}
	})
}

func (a *App) paletteEmpty(c *ui.Context) {
	p := ds.P(c)
	msg := i18n.PaletteEmpty()
	if a.pal.query != "" {
		msg = i18n.NoMatches(a.pal.query)
	}
	comp.Text(c, msg, ds.Caption, p.TextMuted).Padding(ds.Space8, ds.Space10)
	b := ui.ButtonBase(c.Key("pal-settings")).Label(i18n.SearchSettings()).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.open(pageSettings)
	}
	b.FillWidth().Height(ds.PaletteRow).Padding(0, ds.Space10).Gap(ds.Space10).Radius(ds.RadiusSm).
		Background(p.AccentSoft).AlignItems(ui.Center).Children(func() {
		a.kit.Glyph(c, "settings", ds.Glyph, p.TextSecondary)
		comp.Text(c, i18n.SearchSettings(), ds.Body, p.TextStrong).Grow(1)
		comp.Value(c, "↵ "+i18n.Open(), p.AccentText)
	})
}

func (a *App) sectionOf(r core.Result) string {
	if r.Action != nil {
		return i18n.Actions()
	}
	return i18n.Tools()
}

func (a *App) paletteFoot(c *ui.Context) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.PaletteFootRow).Padding(0, ds.Space16).AlignItems(ui.Center).
		BorderWidth(ds.LineWidth, 0, 0, 0).BorderColor(p.Line).Children(func() {
		comp.Hint(c, "↑↓ "+i18n.HintChoose()+"  ·  ↵ "+i18n.Open()+"  ·  "+modKey("↵")+" "+i18n.HintRun())
		ui.Spacer(c)
		comp.Hint(c, i18n.ResultsCount(len(a.pal.results)))
	})
}

func (a *App) paletteRow(c *ui.Context, i int, r core.Result) {
	p := ds.P(c)
	m := r.Tool.Manifest()
	title := r.Title(msgs())
	chosen := i == a.pal.chosen
	row := ui.ButtonBase(c.Key(i)).Label(title).Selected(chosen).Cursor(ui.CursorDefault)
	if row.Clicked() {
		a.pal.open = false
		a.runResult(r)
	}
	fill := ui.Transparent
	if chosen {
		fill = p.AccentSoft
	} else if row.Hovered() {
		fill = p.FillControl
	}
	row.FillWidth().Height(ds.PaletteRow).Padding(0, ds.Space10).Gap(ds.Space10).Radius(ds.RadiusSm).
		Background(fill).AlignItems(ui.Center).Children(func() {
		if r.Action != nil {
			a.kit.Glyph(c, m.Icon, ds.Glyph, p.TextSecondary)
		} else {
			a.kit.ToolIcon(c, m, comp.ToolIconOpts{Size: comp.IconSmall, Running: core.IsRunning(r.Tool), Tint: true})
		}
		highlighted(c, title, r.Match, ds.Body, p.TextPrimary).SingleLine()
		comp.Text(c, a.paletteSub(r), ds.Caption, p.TextMuted).SingleLine().Ellipsis("…")
		ui.Spacer(c)
		a.rowTrailing(c, r, chosen)
	})
}

func (a *App) paletteSub(r core.Result) string {
	m := r.Tool.Manifest()
	if r.Action != nil {
		return m.Name(msgs())
	}
	if s, ok := r.Tool.(tool.Switch); ok && !m.Soon {
		return s.Status().Text
	}
	return categoryName(m.Category)
}

func (a *App) rowTrailing(c *ui.Context, r core.Result, chosen bool) {
	p := ds.P(c)
	if s, ok := r.Tool.(tool.Switch); ok && r.Action == nil && !r.Tool.Manifest().Soon {
		on := s.Status().On
		if comp.SwitchCompact(c, &on, r.Tool.Manifest().Name(msgs())).Changed() {
			a.setSwitch(s, on)
		}
		return
	}
	if r.Action != nil && r.Action.Shortcut != "" {
		comp.Hint(c, keys(r.Action.Shortcut))
		return
	}
	if !chosen {
		return
	}
	label := "↵ " + i18n.Open()
	if r.Action != nil {
		label = "↵ " + i18n.Run()
	}
	comp.Value(c, label, p.AccentText)
}

func highlighted(c *ui.Context, s string, m [2]int, st ds.TypeStyle, color ui.Color) ui.Element {
	runes := []rune(s)
	if m[0] >= m[1] || m[1] > len(runes) {
		return comp.Text(c, s, st, color)
	}
	p := ds.P(c)
	return ds.Style(ui.RichText(c,
		ui.Span{Text: string(runes[:m[0]])},
		ui.Span{Text: string(runes[m[0]:m[1]]), Weight: ds.Semibold, Color: p.TextStrong},
		ui.Span{Text: string(runes[m[1]:])},
	), st).TextColor(color)
}
