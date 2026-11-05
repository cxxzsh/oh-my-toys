package shell

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/core"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/ui/ds"
)

// trayRunning lists the switch tools that run, each with Stop; it shows
// nothing when none runs.
func (a *App) trayRunning(c *ui.Context) {
	running := a.reg.Running()
	if len(running) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(ds.Space1).Children(func() {
		comp.Eyebrow(c, i18n.Running()).Padding(0, ds.TrayInset)
		for _, t := range running {
			a.runRow(c, t)
		}
	})
}

func (a *App) runRow(c *ui.Context, t tool.Switch) {
	p := ds.P(c)
	m, st := t.Manifest(), t.Status()
	ui.Row(c.Key("run:" + m.ID)).FillWidth().Padding(ds.Space2).Gap(ds.Space2).Radius(ds.RadiusLg).
		Background(p.FillControl).AlignItems(ui.Center).Children(func() {
		comp.ToolIcon(c, m.Category, m.Icon, comp.Small, false)
		ui.Column(c).Grow(1).Children(func() {
			comp.Text(c, m.Name(msgs()), ds.Headline, p.TextPrimary)
			sub := st.Text
			if st.Detail != "" {
				sub = i18n.StatusWithDetail(st.Text, st.Detail)
			}
			comp.Text(c, sub, ds.Callout, p.TextSecondary).FontFeatures("tnum").SingleLine()
		})
		stop := ui.ButtonBase(c).Label(i18n.StopTool(m.Name(msgs())))
		if stop.Clicked() {
			a.setSwitch(t, false)
		}
		stop.Height(ds.RunRowButtonH).Padding(0, ds.RunRowButtonPadX).Radius(ds.RadiusMd).Background(p.BgElevated).
			AlignItems(ui.Center).Children(func() {
			comp.Text(c, i18n.Stop(), ds.CalloutStrong, p.TextPrimary)
		})
	})
}

// trayLaunch is the grid of pinned tools, then "All Tools".
func (a *App) trayLaunch(c *ui.Context) {
	tools := a.pinnedTools()
	tools = tools[:min(len(tools), ds.TrayMaxLaunch)]
	cells := len(tools) + 1
	ui.Column(c).FillWidth().Gap(ds.Space1).Children(func() {
		comp.Eyebrow(c, i18n.PinnedTools()).Padding(0, ds.TrayInset)
		for r := 0; r*ds.TrayColumns < cells; r++ {
			ui.Row(c.Key(r)).FillWidth().Gap(ds.TrayCellGap).Children(func() {
				for col := range ds.TrayColumns {
					a.launchCell(c, tools, r*ds.TrayColumns+col)
				}
			})
		}
	})
}

func (a *App) launchCell(c *ui.Context, tools []tool.Tool, i int) {
	switch {
	case i < len(tools):
		t := tools[i]
		m := t.Manifest()
		b := a.cell(c, m.ID, m.Name(msgs()), m.Category, m.Icon, core.IsRunning(t))
		if b.Clicked() {
			if _, ok := t.(tool.Switch); !ok {
				a.hideTray()
			}
			a.activate(t)
		}
		b.ContextMenu(func(menu *ui.Menu) { a.toolMenu(menu, t) })
	case i == len(tools):
		if a.cell(c, pageHome, i18n.AllTools(), tool.CategoryApp, "layout-grid", false).Clicked() {
			a.hideTray()
			a.openMain(pageHome)
		}
	default:
		ui.Box(c.Key(i)).Grow(1).Basis(0)
	}
}

// toolMenu is a tool's context menu: its quick actions, and its page.
func (a *App) toolMenu(menu *ui.Menu, t tool.Tool) {
	m := t.Manifest()
	for k := range m.Actions {
		act := &m.Actions[k]
		if menu.Item(act.Title(msgs())).Chosen() {
			a.runAction(t, act)
		}
	}
	menu.Separator()
	if menu.Item(i18n.OpenInMain()).Chosen() {
		a.hideTray()
		a.openMain(m.ID)
	}
}

func (a *App) cell(c *ui.Context, key, label string, cat tool.Category, icon string, running bool) ui.Element {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key(key)).Label(label)
	if running {
		b.Description(i18n.StatusOn())
	}
	bg := ui.Transparent
	if b.Hovered() {
		bg = p.FillHover
	}
	b.Grow(1).Basis(0).Column().Padding(ds.LaunchPadTop, ds.TrayCellGap, ds.Space2).Gap(ds.LaunchGap).
		Radius(ds.RadiusLg).Background(bg).AlignItems(ui.Center).Children(func() {
		comp.ToolIcon(c, cat, icon, comp.Launch, running)
		comp.Text(c, label, ds.Callout, p.TextPrimary).MaxLines(ds.LaunchLabelLines).Ellipsis("…").TextAlign(ui.Center)
	})
	return b
}

// trayResults is the tray's search: the same results as the palette's.
func (a *App) trayResults(c *ui.Context) {
	p := ds.P(c)
	rs := a.reg.Index().Search(a.trayQuery, msgsLang(), ds.PaletteMaxRows)
	ui.Column(c).FillWidth().Gap(ds.TrayCellGap).Children(func() {
		if len(rs) == 0 {
			comp.Text(c, i18n.NoTools(), ds.Callout, p.TextSecondary).Padding(ds.Space2)
		}
		for i, r := range rs {
			a.trayResult(c, i, r)
		}
	})
	if len(rs) > 0 && c.Shortcut(0, ui.KeyEnter) {
		a.hideTray()
		a.runResult(rs[0])
	}
}

func (a *App) trayResult(c *ui.Context, i int, r core.Result) {
	p := ds.P(c)
	title := r.Title(msgs())
	b := ui.ButtonBase(c.Key(i)).Label(title)
	if b.Clicked() {
		a.hideTray()
		a.runResult(r)
	}
	bg := ui.Transparent
	if b.Hovered() || i == 0 {
		bg = p.FillSelected
	}
	m := r.Tool.Manifest()
	b.FillWidth().Height(ds.TrayItemHeight).Padding(0, ds.TrayInset).Gap(ds.Space2).Radius(ds.RadiusSm).Background(bg).
		AlignItems(ui.Center).Children(func() {
		comp.ToolIcon(c, m.Category, m.Icon, comp.Small, false)
		highlighted(c, title, r.Match, ds.Body, p.TextPrimary).Grow(1).SingleLine()
	})
}

// runResult does what choosing a search result does: runs an action, or
// activates a tool.
func (a *App) runResult(r core.Result) {
	if r.Action != nil {
		a.runAction(r.Tool, r.Action)
		return
	}
	a.activate(r.Tool)
}
