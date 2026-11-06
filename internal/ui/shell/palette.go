package shell

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

type paletteState struct {
	query    string
	last     string
	searched bool
	results  []core.Result
	chosen   int
	input    ui.Handle
}

// setUpPalette makes the command palette once, hidden: a panel over every
// app, which takes the keyboard without activating oh-my-toys, and hides
// rather than closes, so it shows again at once.
func (a *App) setUpPalette() {
	a.palette = mygo.NewWindow(mygo.WindowOptions{
		Title:         i18n.PaletteTitle(),
		Panel:         true,
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		CornerRadius:  ds.RadiusXl,
		Width:         ds.PaletteWidth,
		Height:        ds.PaletteFirstHeight,
		DisableResize: true,
		Content:       ui.View(a.paletteView),
	})
	a.palette.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			a.palette.Hide()
		}
	})
	a.palette.OnBlur(a.palette.Hide)
}

func (a *App) togglePalette() {
	if a.palette.IsVisible() {
		a.palette.Hide()
		return
	}
	a.showPalette()
}

// showPalette shows the palette on the display with the pointer, a quarter
// of the way down, with an empty query.
func (a *App) showPalette() {
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	w, _ := a.palette.Size()
	a.palette.SetPosition(d.WorkArea.X+(d.WorkArea.Width-w)/2, d.WorkArea.Y+d.WorkArea.Height/ds.PaletteTopFraction)
	a.pal = paletteState{input: a.pal.input}
	a.pal.input.Focus()
	a.palette.Show()
}

// recentResults is what an empty query shows.
func (a *App) recentResults() []core.Result {
	tools := a.recentTools()
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
	if a.pal.query != "" && c.Shortcut(0, ui.KeyEscape) {
		a.pal.query = ""
	}
	if c.Shortcut(0, ui.KeyDown) {
		a.pal.chosen = min(a.pal.chosen+1, len(rs)-1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		a.pal.chosen = max(a.pal.chosen-1, 0)
	}
	if a.pal.chosen >= len(rs) {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyEnter) {
		a.palette.Hide()
		a.openMain(rs[a.pal.chosen].Tool.Manifest().ID)
	} else if c.Shortcut(0, ui.KeyEnter) {
		a.palette.Hide()
		a.runResult(rs[a.pal.chosen])
	}
}

func (a *App) paletteView(c *ui.Context) {
	a.theme(c)
	p := ds.P(c)
	c.Root().Background(p.BgElevated)
	a.search()
	a.paletteKeys(c)
	box := ui.Column(c).FillWidth().Children(func() {
		a.paletteInput(c)
		a.paletteList(c)
		a.paletteFoot(c)
	}).DrawOver(func(*ui.Painter, ui.Rect) { a.metrics.keyPainted() })
	fitHeight(c, a.palette, box)
}

func (a *App) paletteInput(c *ui.Context) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.PaletteInputH).Padding(0, ds.Space4).Gap(ds.PaletteGap).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Separator).Children(func() {
		comp.Glyph(c, "search", ds.PaletteGlyph, p.TextSecondary)
		ui.TextInputBase(c, &a.pal.query).Bind(&a.pal.input).Grow(1).FontSize(ds.PaletteInputPx).
			Placeholder(i18n.PaletteHint()).PlaceholderColor(p.TextTertiary).TextColor(p.TextPrimary).Label(i18n.PaletteHint())
		comp.Kbd(c, "esc")
	})
}

func (a *App) paletteList(c *ui.Context) {
	p := ds.P(c)
	rs := a.pal.results
	ui.Column(c).FillWidth().Padding(ds.PaletteListPad).Children(func() {
		if len(rs) == 0 {
			msg := i18n.NoResults()
			if a.pal.query == "" {
				msg = i18n.PaletteEmpty()
			}
			comp.Text(c, msg, ds.Callout, p.TextSecondary).Padding(ds.PaletteGap)
		}
		section := ""
		for i, r := range rs {
			if s := a.sectionOf(r); s != section {
				section = s
				comp.Eyebrow(c, s).Padding(ds.Space2, ds.PaletteGap, ds.Space1)
			}
			a.paletteRow(c, i, r)
		}
	})
}

func (a *App) sectionOf(r core.Result) string {
	switch {
	case a.pal.query == "":
		return i18n.Recent()
	case r.Action != nil:
		return i18n.Actions()
	}
	return i18n.Tools()
}

func (a *App) paletteFoot(c *ui.Context) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.PaletteFootH).Padding(0, ds.Space4).Gap(ds.Space4).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Separator).Children(func() {
		hint := func(k, label string) {
			ui.Row(c).Gap(ds.Space1).AlignItems(ui.Center).Children(func() {
				comp.Kbd(c, k)
				comp.Text(c, label, ds.Callout, p.TextSecondary)
			})
		}
		hint("↑↓", i18n.HintChoose())
		hint("↩", i18n.HintRun())
		hint(modKey("↩"), i18n.HintOpenInMain())
		ui.Spacer(c)
		if a.shortcut != "" {
			comp.Text(c, i18n.HintSummon(keys(a.shortcut)), ds.Callout, p.TextTertiary)
		}
	})
}

func (a *App) paletteRow(c *ui.Context, i int, r core.Result) {
	p := ds.P(c)
	m := r.Tool.Manifest()
	title := r.Title(msgs())
	chosen := i == a.pal.chosen
	row := ui.ButtonBase(c.Key(i)).Label(title).Selected(chosen)
	if row.Clicked() {
		a.palette.Hide()
		a.runResult(r)
	}
	bg := ui.Transparent
	if chosen {
		bg = p.FillSelected
	}
	row.FillWidth().Height(ds.PaletteRowH).Padding(0, ds.PaletteGap).Gap(ds.PaletteGap).Radius(ds.RadiusMd).
		Background(bg).AlignItems(ui.Center).Children(func() {
		comp.ToolIcon(c, m.Category, m.Icon, comp.Medium, core.IsRunning(r.Tool))
		highlighted(c, title, r.Match, ds.Body, p.TextPrimary).SingleLine()
		if r.Action == nil {
			comp.Text(c, categoryName(m.Category), ds.Callout, p.TextSecondary)
		}
		ui.Spacer(c)
		a.rowHint(c, r, chosen)
	})
}

func (a *App) rowHint(c *ui.Context, r core.Result, chosen bool) {
	p := ds.P(c)
	if r.Action != nil && r.Action.Shortcut != "" {
		comp.Kbd(c, keys(r.Action.Shortcut))
		return
	}
	if !chosen {
		return
	}
	label := i18n.Open()
	if r.Action != nil {
		label = i18n.Run()
	} else if _, ok := r.Tool.(tool.Switch); ok {
		label = i18n.TurnOnOff()
	}
	ui.Row(c).Gap(ds.Space1).AlignItems(ui.Center).Children(func() {
		comp.Text(c, label, ds.Callout, p.TextSecondary)
		comp.Kbd(c, "↩")
	})
}

// highlighted shows a title with the runes the query matched in bold.
func highlighted(c *ui.Context, s string, m [2]int, st ds.TypeStyle, color ui.Color) ui.Element {
	runes := []rune(s)
	if m[0] >= m[1] || m[1] > len(runes) {
		return comp.Text(c, s, st, color)
	}
	return ds.Style(ui.RichText(c,
		ui.Span{Text: string(runes[:m[0]])},
		ui.Span{Text: string(runes[m[0]:m[1]]), Weight: ds.Bold},
		ui.Span{Text: string(runes[m[1]:])},
	), st).TextColor(color)
}
