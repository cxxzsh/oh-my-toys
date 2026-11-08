package shell

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

func (a *App) mainView(c *ui.Context) {
	a.theme(c)
	p := ds.P(c)
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(p.BgCanvas)
	}
	a.mainShortcuts(c)
	ui.Column(c).Fill().Children(func() {
		a.titlebar(c)
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
			if !a.sideHidden {
				a.sidebar(c)
			}
			ui.Column(c).Grow(1).FillHeight().Background(p.BgCanvas).Children(func() { a.workbench(c) })
			if a.sideHidden {
				a.autoSidebar(c)
			}
		})
	}).DrawOver(func(*ui.Painter, ui.Rect) { a.metrics.firstFrame(a.opt.OnFirstFrame) })
	if a.pal.open {
		a.paletteOverlay(c)
	}
	if a.dialogOpen && a.dialog != nil {
		a.kit.Dialog(c, &a.dialogOpen, *a.dialog)
	}
	if a.showBusyLine(c) {
		comp.ProgressBusy(c)
	}
}

func (a *App) mainShortcuts(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyK, a.showPalette)
	c.OnShortcut(ui.Cmd, ui.KeyF, a.showPalette)
	c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.open(pageSettings) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyS, a.toggleSidebar)
	c.OnShortcut(ui.Cmd, ui.KeyW, a.main.Hide)
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
	all := a.reg.All()
	for i := range min(len(all), ds.SidebarMaxJump) {
		id := all[i].Manifest().ID
		c.OnShortcut(ui.Cmd, ui.Key1+ui.Key(i), func() { a.open(id) })
	}
}

// titlebar is the 48pt strip under the OS window controls: app icon and
// name, breadcrumb, the page filter, the theme segmented control.
func (a *App) titlebar(c *ui.Context) {
	p := ds.P(c)
	bar := c.TitleBar()
	left, right := float32(ds.Space16), float32(ds.Space16)
	if bar.Left > 0 {
		left = max(left, bar.Left+ds.Space12)
	}
	if bar.Right > 0 {
		right = max(right, bar.Right+ds.Space12)
	}
	ui.Row(c).FillWidth().Height(max(ds.TitlebarHeight, bar.Height)).Padding(0, right, 0, left).
		Gap(ds.Space12).AlignItems(ui.Center).Background(p.BgTitlebar).
		BorderWidth(0, 0, ds.LineWidth, 0).BorderColor(p.Line).DragWindow().Children(func() {
		a.breadcrumbs(c)
		a.paletteTrigger(c)
		ui.Row(c).Grow(1).Basis(0).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			a.themeSegment(c)
		})
	})
}

func (a *App) paletteTrigger(c *ui.Context) {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key("pal-trigger")).Label(i18n.PaletteHint()).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.showPalette()
	}
	fill := p.FillControl
	if b.Hovered() {
		fill = p.FillControlHover
	}
	b.Width(ds.SearchWidth).MinWidth(ds.SearchMinWidth).Shrink(1).Height(ds.ControlLg).
		Padding(0, ds.Space6, 0, ds.Space12).Gap(ds.Space6).Radius(ds.RadiusFull).
		Background(fill).Border(ds.LineWidth, p.LineHairline).AlignItems(ui.Center).Children(func() {
		a.kit.Glyph(c, "search", ds.SearchGlyph, p.TextMuted)
		comp.Text(c, i18n.PaletteHint(), ds.Label, p.TextMuted).Grow(1).SingleLine().TextAlign(ui.Start)
		comp.Kbd(c, modKey("K"))
	})
}

func (a *App) breadcrumbs(c *ui.Context) {
	p := ds.P(c)
	ink := p.TextStrong
	if !a.active() {
		ink = p.TextMuted
	}
	appStyle := ds.Headline
	appStyle.Tracking = -ds.TitleTracking
	crumbStyle := ds.Label
	crumbStyle.Weight = ds.Semibold
	ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		a.sidebarPinButton(c)
		a.kit.AppIcon(c, ds.TitlebarGlyph)
		comp.Text(c, i18n.AppName(), appStyle, ink).SingleLine()
		comp.Text(c, "›", ds.Label, p.TextMuted).SingleLine()
		comp.Text(c, a.title(), crumbStyle, ink).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
	})
}

func (a *App) sidebarPinButton(c *ui.Context) {
	p := ds.P(c)
	pinned := !a.sideHidden
	glyph, label, color := glyphPin, i18n.ShowSidebar(), p.TextMuted
	if pinned {
		glyph, label, color = glyphPinned, i18n.HideSidebar(), p.Accent
		if !a.active() {
			color = p.TextMuted
		}
	}
	tip := label + " (" + modKey("⇧S") + ")"
	b := ui.ButtonBase(c.Key("pin-side")).Label(label).Tooltip(tip).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.toggleSidebar()
	}
	fill := ui.Transparent
	if b.Hovered() {
		fill = p.FillControl
		if !pinned {
			color = p.TextStrong
		}
	}
	b.Size(ds.ControlSm, ds.ControlSm).Radius(ds.RadiusFull).Background(fill).Center().Children(func() {
		a.kit.Glyph(c, glyph, ds.GlyphSm, color)
	})
}

func (a *App) themeSegment(c *ui.Context) {
	src := mygo.Theme.Source()
	sel := 0
	switch src {
	case mygo.ThemeDark:
		sel = 1
	case mygo.ThemeLight:
		sel = 2
	case mygo.ThemeSystem, "":
	}
	opts := []string{i18n.ThemeAuto(), i18n.ThemeDark(), i18n.ThemeLight()}
	if comp.Segmented(c.Key("bar-theme"), &sel, opts, i18n.Appearance()).Changed() {
		sources := []mygo.ThemeSource{mygo.ThemeSystem, mygo.ThemeDark, mygo.ThemeLight}
		a.setTheme(sources[sel])
	}
}

func (a *App) workbench(c *ui.Context) {
	switch a.page {
	case pageHome:
		a.home(c)
	case pageSettings:
		a.scrollWorkbench(c, func() { a.settingsPage(c) })
	default:
		a.toolPage(c)
	}
}

func (a *App) scrollWorkbench(c *ui.Context, body func()) {
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(ds.Space16).Gap(ds.Space16).Children(body)
	})
}

func (a *App) title() string {
	switch a.page {
	case pageHome:
		if a.homeCat != "" {
			return categoryName(a.homeCat)
		}
		return i18n.AllTools()
	case pageSettings:
		return i18n.Settings()
	}
	if t := a.reg.Get(a.page); t != nil {
		return t.Manifest().Name(msgs())
	}
	return ""
}

// toolPage shows a tool by its kind. A panic in the page is kept as the
// tool's failure: the next frame shows it, and nothing else changes.
func (a *App) toolPage(c *ui.Context) {
	t := a.reg.Get(a.page)
	if t == nil {
		a.page = pageHome
		return
	}
	id := t.Manifest().ID
	if f := a.opt.Runner.Failed(id); f != nil {
		a.scrollWorkbench(c, func() { a.failure(c, t, f) })
		return
	}
	ui.Column(c.Key("page:" + id)).Grow(1).FillWidth().Children(func() {
		if !a.opt.Runner.Do(id, "view", func() { a.toolBody(c, t) }) {
			c.Invalidate()
		}
	})
}

func (a *App) toolBody(c *ui.Context, t tool.Tool) {
	m := t.Manifest()
	if m.Soon {
		a.scrollWorkbench(c, func() { a.soonPage(c, m) })
		return
	}
	if m.Kind == tool.KindCapture {
		a.capturePage(c, t)
		return
	}
	if tr, ok := t.(tool.Traffic); ok {
		a.trafficPage(c, tr)
		return
	}
	switch t := t.(type) {
	case tool.Switch:
		a.switchPage(c, t)
	case tool.Action:
		a.actionPage(c, t)
	}
}

func (a *App) soonPage(c *ui.Context, m tool.Manifest) {
	p := ds.P(c)
	comp.PageHeader(c, comp.Header{Title: m.Name(msgs()), Description: m.Summary(msgs())})
	comp.Card(c).FillWidth().Padding(ds.Space16).Gap(ds.Space8).Children(func() {
		comp.Text(c, i18n.ComingSoon(), ds.Headline, p.TextStrong)
		comp.Text(c, i18n.ComingSoonDetail(), ds.Body, p.TextSecondary)
	})
}

func (a *App) failure(c *ui.Context, t tool.Tool, f *core.Failure) {
	p := ds.P(c)
	m := t.Manifest()
	comp.PageHeader(c, comp.Header{Title: m.Name(msgs()), Description: m.Summary(msgs())})
	comp.Card(c).FillWidth().Padding(ds.Space16).Gap(ds.Space12).Children(func() {
		ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
			comp.StatusDot(c, comp.DotOpts{Tone: comp.DotDanger, Large: true})
			comp.Text(c, i18n.ToolFailed(m.Name(msgs())), ds.Headline, p.TextStrong)
		})
		comp.Value(c, f.Value, p.Danger).Selectable()
		ui.Row(c).Gap(ds.Space8).Children(func() {
			if a.kit.Button(c.Key("reload"), i18n.Reload(), comp.ButtonOpts{Kind: comp.Primary}).Clicked() {
				delete(a.actions, m.ID)
				a.opt.Runner.Clear(m.ID)
			}
			if a.kit.Button(c.Key("copy"), i18n.CopyDetails(), comp.ButtonOpts{Kind: comp.Secondary}).Clicked() {
				mygo.Clipboard.WriteText(f.Value + "\n\n" + f.Stack)
			}
		})
	})
}

func categoryName(c tool.Category) string {
	switch c {
	case tool.CategorySystem:
		return i18n.CategorySystem()
	case tool.CategoryFile:
		return i18n.CategoryFile()
	case tool.CategoryDev:
		return i18n.CategoryDev()
	case tool.CategoryNetwork:
		return i18n.CategoryNetwork()
	case tool.CategoryText:
		return i18n.CategoryText()
	case tool.CategoryMedia:
		return i18n.CategoryMedia()
	case tool.CategoryProductivity:
		return i18n.CategoryProductivity()
	case tool.CategoryApp:
		return i18n.CategoryApp()
	}
	return string(c)
}
