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
	key     string
	page    string
	glyph   string
	label   string
	section string
	count   int
	running bool
	tool    tool.Tool
}

// navRows flattens the sidebar into rows of a virtual list: All Tools,
// the collapsible Pinned section, and each category's collapsible section
// with all tools of that category.
func (a *App) navRows() []navRow {
	rows := []navRow{{kind: navAll, key: pageHome, page: pageHome, glyph: "apps", label: i18n.AllTools()}}
	if pinned := a.pinnedTools(); len(pinned) > 0 {
		rows = append(rows, navRow{
			kind: navHead, label: i18n.Pinned(), section: sectionPinned, count: len(pinned),
		})
		if !a.settings.IsCollapsed(sectionPinned) {
			for _, t := range pinned {
				rows = append(rows, toNavTool("pin:", t))
			}
		}
	}
	for _, cat := range tool.Categories() {
		tools := a.reg.InCategory(cat)
		if len(tools) == 0 {
			continue
		}
		sec := string(cat)
		rows = append(rows, navRow{kind: navHead, label: categoryName(cat), section: sec, count: len(tools)})
		if !a.settings.IsCollapsed(sec) {
			for _, t := range tools {
				rows = append(rows, toNavTool("cat:", t))
			}
		}
	}
	return rows
}

func toNavTool(prefix string, t tool.Tool) navRow {
	m := t.Manifest()
	return navRow{
		kind: navTool, key: prefix + m.ID, page: m.ID, glyph: m.Icon,
		label: m.Name(msgs()), running: core.IsRunning(t), tool: t,
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

// sidebar is the docked column when the sidebar is pinned open.
func (a *App) sidebar(c *ui.Context) {
	p := ds.P(c)
	fill := p.BgSidebar
	if c.Vibrancy() {
		fill = p.BgSidebarMaterial
	}
	ui.Column(c).Width(ds.SidebarWidth).FillHeight().Padding(ds.Space12).Gap(ds.Space8).Background(fill).
		BorderWidth(0, ds.LineWidth, 0, 0).BorderColor(p.Line).Role(ui.RoleGroup).Label(i18n.Sidebar()).
		Children(func() { a.sidebarBody(c) })
}

// autoSidebar is the auto-collapsing sidebar when unpinned: hovering the
// left edge pops it out as a floating card, and moving the pointer out or
// choosing a tool collapses it again.
func (a *App) autoSidebar(c *ui.Context) {
	edge := ui.Box(c.Key("side-edge")).Absolute().Top(0).Bottom(0).Left(0).Width(ds.SidePeekTrigger)
	if edge.Hovered() && !a.sidePeek {
		a.sidePeek, a.sideEntered = true, true
	}
	if !a.sidePeek {
		return
	}
	p := ds.P(c)
	panel := ui.Column(c.Key("side-float")).Absolute().Top(ds.Space6).Bottom(ds.Space6).Left(ds.Space6).
		Width(ds.SidebarWidth).Padding(ds.Space12).Gap(ds.Space8).Radius(ds.RadiusMd).
		Background(p.BgPopover).Border(ds.LineWidth, p.LineControl).Role(ui.RoleGroup).Label(i18n.Sidebar())
	ds.ApplyShadow(panel, ds.S(c).ShadowPopover)
	if panel.Hovered() {
		a.sideEntered = true
	} else if (a.sideEntered && !edge.Hovered()) || panel.PressedOutside() {
		a.sidePeek, a.sideEntered = false, false
		c.Invalidate()
	}
	panel.Children(func() { a.sidebarBody(c) })
}

func (a *App) sidebarBody(c *ui.Context) {
	rows := a.navRows()
	a.sideList.Label = func(i int) string {
		if i < len(rows) {
			return rows[i].label
		}
		return ""
	}
	ui.List(c.Key("side"), &a.sideList, len(rows), func(i int) { a.sideRow(c, rows[i]) }).
		Grow(1).FillWidth().Gap(ds.Space2).Label(i18n.Tools())
	a.navItem(c, navRow{key: pageSettings, page: pageSettings, glyph: glyphSettings, label: i18n.Settings()})
	a.daemonCard(c)
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

// navItem is one sidebar row: glyph, name, inline pin button on tool rows,
// and a live dot for a running switch tool or an accent dot when selected.
func (a *App) navItem(c *ui.Context, r navRow) {
	p := ds.P(c)
	sel := a.page == r.page
	acc := r.label
	if r.running {
		acc = r.label + ", " + i18n.Running()
	}
	b := ui.ButtonBase(c.Key("nav:" + r.key)).Label(acc).Selected(sel).Cursor(ui.CursorDefault)
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
