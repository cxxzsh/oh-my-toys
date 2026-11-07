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

// sectionPinned names the pinned section in the collapsed list.
const sectionPinned = "pinned"

type navKind int

const (
	navAll navKind = iota
	navHead
	navTool
)

type navRow struct {
	kind    navKind
	page    string
	glyph   string
	label   string
	section string
	count   int
	running bool
	tool    tool.Tool
}

// navRows flattens the sidebar into rows of a virtual list: All Tools,
// the pinned tools, then each category's header and, when open, its tools.
// Pinned tools are not repeated in their category.
func (a *App) navRows() []navRow {
	rows := []navRow{{kind: navAll, page: pageHome, glyph: "apps", label: i18n.AllTools()}}
	if pinned := a.pinnedTools(); len(pinned) > 0 {
		rows = append(rows, navRow{
			kind: navHead, label: i18n.Pinned(), section: sectionPinned, count: len(pinned),
		})
		if !a.settings.IsCollapsed(sectionPinned) {
			for _, t := range pinned {
				rows = append(rows, toNavTool(t))
			}
		}
	}
	for _, cat := range tool.Categories() {
		rows = append(rows, a.categoryNavRows(cat)...)
	}
	return rows
}

func (a *App) categoryNavRows(cat tool.Category) []navRow {
	var tools []tool.Tool
	for _, t := range a.reg.InCategory(cat) {
		if !a.isPinned(t) {
			tools = append(tools, t)
		}
	}
	if len(tools) == 0 {
		return nil
	}
	sec := string(cat)
	rows := []navRow{{kind: navHead, label: categoryName(cat), section: sec, count: len(tools)}}
	if a.settings.IsCollapsed(sec) {
		return rows
	}
	for _, t := range tools {
		rows = append(rows, toNavTool(t))
	}
	return rows
}

func toNavTool(t tool.Tool) navRow {
	m := t.Manifest()
	return navRow{
		kind: navTool, page: m.ID, glyph: m.Icon, label: m.Name(msgs()),
		running: core.IsRunning(t), tool: t,
	}
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

// sidebar is the fixed 220pt column: the header with collapse toggle, the
// collapsible Pinned and category tool sections, Settings, and the daemon
// card pinned to the bottom.
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
	ui.Column(c).Width(ds.SidebarWidth).FillHeight().Padding(ds.Space12).Gap(ds.Space8).Background(fill).
		BorderWidth(0, ds.LineWidth, 0, 0).BorderColor(p.Line).Role(ui.RoleGroup).Label(i18n.Sidebar()).
		Children(func() {
			a.sidebarHeader(c)
			ui.List(c.Key("side"), &a.sideList, len(rows), func(i int) { a.sideRow(c, rows[i]) }).
				Grow(1).FillWidth().Gap(ds.Space2).Label(i18n.Tools())
			a.navItem(c, navRow{page: pageSettings, glyph: glyphSettings, label: i18n.Settings()})
			a.daemonCard(c)
		})
}

func (a *App) sidebarHeader(c *ui.Context) {
	p := ds.P(c)
	tip := i18n.HideSidebar() + " (" + modKey("⇧S") + ")"
	ui.Row(c).FillWidth().Padding(0, ds.Space4, 0, ds.Space8).AlignItems(ui.Center).Children(func() {
		comp.Eyebrow(c, i18n.Tools()).Grow(1)
		b := ui.ButtonBase(c.Key("hide-side")).Label(i18n.HideSidebar()).Tooltip(tip).Cursor(ui.CursorDefault)
		if b.Clicked() {
			a.toggleSidebar()
		}
		fill := ui.Transparent
		if b.Hovered() {
			fill = p.FillControl
		}
		b.Size(ds.ControlSm, ds.ControlSm).Radius(ds.RadiusFull).Background(fill).Center().Children(func() {
			a.kit.Glyph(c, "chevron_right", ds.GlyphSm, p.TextMuted).Rotate(ds.ChevronLeft)
		})
	})
}

func (a *App) sideRow(c *ui.Context, r navRow) {
	if r.kind == navHead {
		a.navHead(c, r)
		return
	}
	a.navItem(c, r)
}

func (a *App) navHead(c *ui.Context, r navRow) {
	p := ds.P(c)
	open := !a.settings.IsCollapsed(r.section)
	head := ui.ButtonBase(c.Key("head:" + r.section)).Label(r.label).Expanded(open).Cursor(ui.CursorDefault)
	if head.Clicked() {
		a.settings.SetCollapsed(r.section, open)
		a.save()
	}
	fill := ui.Transparent
	if head.Hovered() {
		fill = p.FillControl
	}
	head.FillWidth().Height(ds.SideHeadHeight).Margin(ds.Space4, 0, 0).Padding(0, ds.Space8).
		Gap(ds.Space4).Radius(ds.RadiusFull).Background(fill).AlignItems(ui.Center).Children(func() {
		g := a.kit.Glyph(c, "expand_more", ds.GlyphSm, p.TextMuted)
		if !open {
			g.Rotate(ds.ChevronClosed)
		}
		comp.Eyebrow(c, r.label).Grow(1)
		if r.count > 0 {
			comp.Hint(c, strconv.Itoa(r.count))
		}
	})
}

// navItem is one sidebar row: a 16pt glyph, the name, a pin button, and a
// live dot for a running switch tool or an accent dot when selected.
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
	hovered := b.Hovered()
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
	case hovered:
		fill, ink, glyphInk = p.FillControl, p.TextStrong, p.TextStrong
	}
	b.FillWidth().Height(ds.NavRow).Padding(0, ds.Space10).Gap(ds.Space8).Radius(ds.RadiusFull).
		Background(fill).AlignItems(ui.Center).Children(func() {
		a.kit.Glyph(c, r.glyph, ds.Glyph, glyphInk)
		comp.Text(c, r.label, style, ink).Grow(1).SingleLine().Ellipsis("…")
		if r.tool != nil && (a.isPinned(r.tool) || hovered) {
			a.pinButton(c, r.tool)
		}
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
	pin := i18n.Pin()
	if a.isPinned(t) {
		pin = i18n.Unpin()
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
