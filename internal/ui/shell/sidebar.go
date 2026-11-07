package shell

import (
	"fmt"
	"os"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

type navRow struct {
	page    string
	glyph   string
	label   string
	running bool
	tool    tool.Tool
}

func (a *App) navRows() []navRow {
	tools := a.reg.All()
	rows := make([]navRow, 0, 2+len(tools))
	rows = append(rows, navRow{page: pageHome, glyph: "apps", label: i18n.AllTools()})
	for _, t := range tools {
		m := t.Manifest()
		rows = append(rows, navRow{
			page: m.ID, glyph: m.Icon, label: m.Name(msgs()),
			running: core.IsRunning(t), tool: t,
		})
	}
	rows = append(rows, navRow{page: pageSettings, glyph: glyphSettings, label: i18n.Settings()})
	return rows
}

func (a *App) isPinned(t tool.Tool) bool {
	id := t.Manifest().ID
	for _, p := range a.settings.Pinned {
		if p == id {
			return true
		}
	}
	return false
}

func (a *App) pinnedTools() []tool.Tool {
	var out []tool.Tool
	for _, id := range a.settings.Pinned {
		if t := a.reg.Get(id); t != nil {
			out = append(out, t)
		}
	}
	return out
}

// sidebar is the fixed 220pt column: the eyebrow, the tools in registry
// order, and the daemon card pinned to the bottom.
func (a *App) sidebar(c *ui.Context) {
	p := ds.P(c)
	fill := p.BgSidebar
	if c.Vibrancy() {
		fill = p.BgSidebarMaterial
	}
	rows := a.navRows()
	a.sideList.Label = func(i int) string {
		if i < len(rows) {
			return rows[i].label
		}
		return ""
	}
	ui.Column(c).Width(ds.SidebarWidth).FillHeight().Padding(ds.Space12).Gap(ds.Space12).Background(fill).
		BorderWidth(0, ds.LineWidth, 0, 0).BorderColor(p.Line).Role(ui.RoleGroup).Label(i18n.Sidebar()).
		Children(func() {
			comp.Eyebrow(c, i18n.Tools()).Padding(ds.Space4, ds.Space8, 0)
			ui.List(c.Key("side"), &a.sideList, len(rows), func(i int) { a.navItem(c, rows[i]) }).
				Grow(1).FillWidth().Gap(ds.Space4).Label(i18n.Tools())
			a.daemonCard(c)
		})
}

// navItem is one sidebar row: a 16pt glyph, the name, and a live dot for
// a running switch tool or an accent dot when selected.
func (a *App) navItem(c *ui.Context, r navRow) {
	p := ds.P(c)
	sel := a.page == r.page
	acc := r.label
	if r.running {
		acc = r.label + ", " + i18n.Running()
	}
	b := ui.ButtonBase(c.Key("nav:" + r.page)).Label(acc).Selected(sel).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.open(r.page)
		if r.page == pageHome {
			a.homeCat = ""
		}
	}
	if r.tool != nil {
		b.ContextMenu(func(m *ui.Menu) { a.sidebarToolMenu(m, r.tool) })
	}
	fill, ink, glyphInk, style := ui.Transparent, p.TextInactive, p.TextInactive, ds.Label
	switch {
	case sel:
		fill, ink, glyphInk = p.BgSelected, p.TextStrong, p.Accent
		style.Weight = ds.Semibold
		if !a.active() {
			glyphInk = p.TextMuted
		}
		b.Border(ds.LineWidth, p.LineHairline)
		ds.ApplyShadow(b, ds.S(c).ShadowXs)
	case b.Pressed():
		fill, ink, glyphInk = p.FillControlPressed, p.TextStrong, p.TextStrong
	case b.Hovered():
		fill, ink, glyphInk = p.FillControl, p.TextStrong, p.TextStrong
	}
	b.FillWidth().Height(ds.NavRow).Padding(0, ds.Space10).Gap(ds.Space8).Radius(ds.RadiusFull).
		Background(fill).AlignItems(ui.Center).Children(func() {
		a.kit.Glyph(c, r.glyph, ds.Glyph, glyphInk)
		comp.Text(c, r.label, style, ink).Grow(1).SingleLine().Ellipsis("…")
		a.navTrailing(c, r, sel)
	})
}

func (a *App) navTrailing(c *ui.Context, r navRow, sel bool) {
	switch {
	case r.running:
		comp.StatusDot(c, comp.DotOpts{Tone: comp.DotLive, Pulse: true, Label: i18n.Running()})
	case sel:
		tone := comp.DotAccent
		if !a.active() {
			tone = comp.DotMuted
		}
		comp.StatusDot(c, comp.DotOpts{Tone: tone})
	}
}

func (a *App) sidebarToolMenu(m *ui.Menu, t tool.Tool) {
	id := t.Manifest().ID
	if m.Item(i18n.Open()).Chosen() {
		a.open(id)
	}
	pin := i18n.ShowInTray()
	if a.isPinned(t) {
		pin = i18n.HideFromTray()
	}
	if m.Item(pin).Chosen() {
		a.settings.TogglePin(id)
		a.save()
	}
}

func (a *App) daemonCard(c *ui.Context) {
	ver := mygo.App.Version()
	if ver == "" {
		ver = devVersion
	}
	pid := os.Getpid()
	card := comp.DaemonCard(c, i18n.AppName(), i18n.Running(), "v"+ver, fmt.Sprintf("pid: %d", pid), true)
	if card.Clicked() {
		a.open(pageSettings)
	}
	card.ContextMenu(func(m *ui.Menu) {
		if m.Item(i18n.CopyPID()).Chosen() {
			mygo.Clipboard.WriteText(strconv.Itoa(pid))
		}
	})
}
