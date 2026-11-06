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
	vib := c.Vibrancy()
	if vib {
		c.Root().Background(ui.Transparent) // the material shows through the sidebar
	} else {
		c.Root().Background(p.BgWindow)
	}
	c.OnShortcut(ui.Cmd, ui.KeyK, a.showPalette)
	c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.page = pageSettings })
	c.OnShortcut(ui.Cmd, ui.KeyW, a.main.Hide)
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		side := ui.Column(c).Width(ds.SidebarWidth).BorderWidth(0, 1, 0, 0).BorderColor(p.Separator).
			Role(ui.RoleGroup).Label(i18n.Sidebar())
		if !vib {
			side.Background(p.BgSidebar)
		}
		side.Children(func() { a.sidebar(c) })
		ui.Column(c).Grow(1).Background(p.BgWindow).Children(func() { a.content(c) })
	}).DrawOver(func(*ui.Painter, ui.Rect) { a.metrics.firstFrame(a.opt.OnFirstFrame) })
}

func (a *App) content(c *ui.Context) {
	a.toolbar(c)
	switch a.page {
	case pageHome:
		a.home(c)
	case pageSettings:
		a.scrollPage(c, func() { a.settingsPage(c) })
	default:
		a.toolPage(c)
	}
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

func (a *App) toolbar(c *ui.Context) {
	p := ds.P(c)
	bar := c.TitleBar()
	ui.Row(c).FillWidth().Height(max(ds.ToolbarHeight, bar.Height)).Padding(0, ds.Space5+bar.Right, 0, ds.Space5).
		Gap(ds.Space3).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Separator).DragWindow().Children(func() {
		comp.Text(c, a.title(), ds.Title, p.TextPrimary).Grow(1).SingleLine().Role(ui.RoleHeading).Level(1)
		if n := len(a.reg.Running()); n > 0 {
			comp.Pill(c, i18n.ToolsRunning(n), true)
		}
	})
}

// scrollPage lays a page out at the content's width, at most
// content-max-width, scrolling.
func (a *App) scrollPage(c *ui.Context, body func()) {
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(ds.Space6, ds.Space8).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(ds.ContentMaxWidth).Gap(ds.Space6).Children(body)
		})
	})
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
		a.scrollPage(c, func() { a.failure(c, t, f) })
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
		a.scrollPage(c, func() { a.soonPage(c, m) })
		return
	}
	switch t := t.(type) {
	case tool.Switch:
		a.scrollPage(c, func() { a.switchPage(c, t) })
	case tool.Action:
		a.actionPage(c, t)
	}
}

func (a *App) header(c *ui.Context, m tool.Manifest, trailing func()) {
	comp.ToolHeader(c, m, m.Name(msgs()), m.Summary(msgs()), trailing)
}

func (a *App) soonPage(c *ui.Context, m tool.Manifest) {
	p := ds.P(c)
	a.header(c, m, nil)
	comp.Card(c).FillWidth().Padding(ds.Space4).Gap(ds.Space2).Children(func() {
		comp.Text(c, i18n.ComingSoon(), ds.Headline, p.TextPrimary)
		comp.Text(c, i18n.ComingSoonDetail(), ds.Body, p.TextSecondary)
	})
}

func (a *App) failure(c *ui.Context, t tool.Tool, f *core.Failure) {
	p := ds.P(c)
	m := t.Manifest()
	a.header(c, m, nil)
	comp.Card(c).FillWidth().Padding(ds.Space4).Gap(ds.Space3).Children(func() {
		ui.Row(c).Gap(ds.Space2).AlignItems(ui.Center).Children(func() {
			comp.Glyph(c, "triangle-alert", ds.Glyph, p.Danger)
			comp.Text(c, i18n.ToolFailed(m.Name(msgs())), ds.Headline, p.TextPrimary)
		})
		comp.Text(c, f.Value, ds.Mono, p.Danger).Selectable()
		ui.Row(c).Gap(ds.Space2).Children(func() {
			if comp.Button(c, i18n.Reload(), comp.Primary).Clicked() {
				delete(a.actions, m.ID)
				a.opt.Runner.Clear(m.ID)
			}
			if comp.Button(c, i18n.CopyDetails(), comp.Secondary).Clicked() {
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
