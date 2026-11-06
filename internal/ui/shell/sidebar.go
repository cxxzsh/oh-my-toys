package shell

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// sectionPinned names the pinned section in the collapsed list.
const sectionPinned = "pinned"

// sideSearchLimit is how many tools the sidebar's search lists.
const sideSearchLimit = 60

type sideKind int

const (
	sideAll sideKind = iota
	sideHead
	sideTool
)

type sideRow struct {
	kind    sideKind
	label   string
	section string
	tool    tool.Tool
	count   int
}

// sideRows flattens the sidebar into rows of a virtual list: all tools,
// the pinned tools, then each category's header and, when open, its tools.
// Pinned tools are not repeated in their category.
func (a *App) sideRows() []sideRow {
	if a.sideQuery != "" {
		return a.sideMatches()
	}
	rows := []sideRow{{kind: sideAll, label: i18n.AllTools()}}
	if len(a.settings.Pinned) > 0 {
		rows = append(rows, sideRow{kind: sideHead, label: i18n.Pinned(), section: sectionPinned})
		if !a.settings.IsCollapsed(sectionPinned) {
			for _, t := range a.pinnedTools() {
				rows = append(rows, sideRow{kind: sideTool, tool: t})
			}
		}
	}
	for _, cat := range tool.Categories() {
		rows = append(rows, a.categoryRows(cat)...)
	}
	return rows
}

func (a *App) categoryRows(cat tool.Category) []sideRow {
	var tools []tool.Tool
	for _, t := range a.reg.InCategory(cat) {
		if !a.isPinned(t) {
			tools = append(tools, t)
		}
	}
	if len(tools) == 0 {
		return nil
	}
	rows := []sideRow{{kind: sideHead, label: categoryName(cat), section: string(cat), count: len(tools)}}
	if a.settings.IsCollapsed(string(cat)) {
		return rows
	}
	for _, t := range tools {
		rows = append(rows, sideRow{kind: sideTool, tool: t})
	}
	return rows
}

func (a *App) sideMatches() []sideRow {
	var rows []sideRow
	seen := map[string]bool{}
	for _, r := range a.reg.Index().Search(a.sideQuery, msgsLang(), sideSearchLimit) {
		id := r.Tool.Manifest().ID
		if !seen[id] {
			seen[id] = true
			rows = append(rows, sideRow{kind: sideTool, tool: r.Tool})
		}
	}
	return rows
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

func (a *App) isPinned(t tool.Tool) bool {
	id := t.Manifest().ID
	for _, p := range a.settings.Pinned {
		if p == id {
			return true
		}
	}
	return false
}

func (a *App) sidebar(c *ui.Context) {
	p := ds.P(c)
	bar := c.TitleBar()
	ui.Box(c).FillWidth().Height(max(ds.ToolbarHeight, bar.Height)).DragWindow()
	ui.Column(c).FillWidth().Padding(0, ds.SidePadX, ds.SideSearchGap).Children(func() {
		a.searchField(c, &a.sideQuery, &a.sideInput, i18n.SearchTools(), modKey("K"))
	})
	rows := a.sideRows()
	a.sideList.Label = func(i int) string {
		if i >= len(rows) {
			return ""
		}
		if rows[i].tool != nil {
			return rows[i].tool.Manifest().Name(msgs())
		}
		return rows[i].label
	}
	ui.List(c.Key("side"), &a.sideList, len(rows), func(i int) { a.sideRow(c, rows[i]) }).
		Grow(1).Padding(0, ds.SidePadX).Gap(ds.SideItemGap).Label(i18n.Tools()).Children(func() {
		if len(rows) == 0 {
			comp.Text(c, i18n.NoTools(), ds.Callout, p.TextSecondary).Padding(ds.Space2)
		}
	})
	ui.Column(c).FillWidth().Padding(ds.Space1, ds.SidePadX, ds.Space3).Children(func() {
		a.sideItem(c, pageSettings, i18n.Settings(), tool.CategoryApp, "settings", false)
	})
}

func (a *App) sideRow(c *ui.Context, r sideRow) {
	switch r.kind {
	case sideAll:
		a.sideItem(c, pageHome, r.label, tool.CategoryApp, "layout-grid", false)
	case sideHead:
		a.sideHead(c, r)
	case sideTool:
		m := r.tool.Manifest()
		a.sideItem(c, m.ID, m.Name(msgs()), m.Category, m.Icon, core.IsRunning(r.tool))
	}
}

func (a *App) sideItem(c *ui.Context, page, label string, cat tool.Category, icon string, running bool) {
	p := ds.P(c)
	sel := a.page == page
	item := ui.ButtonBase(c.Key(page)).Label(label).Selected(sel)
	bg := ui.Transparent
	switch {
	case sel:
		bg = p.FillSelected
	case item.Hovered():
		bg = p.FillHover
	}
	if item.Clicked() {
		a.open(page)
		if page == pageHome {
			a.homeCat = ""
		}
	}
	style := ds.Body
	if sel {
		style = ds.Headline
	}
	item.FillWidth().Height(ds.SideItemHeight).Padding(0, ds.Space2).Gap(ds.Space2).Radius(ds.RadiusSm).
		Background(bg).AlignItems(ui.Center).Children(func() {
		comp.ToolIcon(c, cat, icon, comp.Small, false)
		comp.Text(c, label, style, p.TextPrimary).Grow(1).SingleLine().Ellipsis("…")
		if running {
			comp.Dot(c, true).Label(i18n.StatusOn())
		}
	})
}

func (a *App) sideHead(c *ui.Context, r sideRow) {
	p := ds.P(c)
	open := !a.settings.IsCollapsed(r.section)
	head := ui.ButtonBase(c.Key("head:" + r.section)).Label(r.label).Expanded(open)
	if head.Clicked() {
		a.settings.SetCollapsed(r.section, open)
		a.save()
	}
	head.FillWidth().Height(ds.SideHeadHeight).Margin(ds.Space3, ds.Space1, ds.SideItemGap).Padding(0, ds.Space1).
		Gap(ds.Space1).AlignItems(ui.Center).Children(func() {
		g := comp.Glyph(c, "chevron-down", ds.SideHeadGlyph, p.TextSecondary)
		if !open {
			g.Rotate(ds.ChevronClosed)
		}
		comp.Eyebrow(c, r.label).Grow(1)
		if r.count > 0 {
			comp.Text(c, strconv.Itoa(r.count), ds.Count, p.TextTertiary)
		}
	})
}

// searchField is the design system's search box: a glyph, the text, and a
// key hint on the right. Escape clears it.
func (a *App) searchField(c *ui.Context, q *string, h *ui.Handle, placeholder, hint string) {
	p := ds.P(c)
	box := ui.Row(c).FillWidth().Height(ds.ControlHeight).Padding(0, ds.SearchPadEnd, 0, ds.Space2).Gap(ds.SearchGap).
		Radius(ds.RadiusMd).Background(p.FillControl).AlignItems(ui.Center).Children(func() {
		comp.Glyph(c, "search", ds.SearchGlyph, p.TextSecondary)
		ui.TextInputBase(c, q).Bind(h).Grow(1).FontSize(ds.Body.Size).Placeholder(placeholder).
			PlaceholderColor(p.TextTertiary).TextColor(p.TextPrimary).Label(placeholder)
		if hint != "" && *q == "" {
			comp.Kbd(c, hint)
		}
	})
	if c.Focused(*h) {
		box.Border(1, p.FocusRing)
	}
	if *q != "" && c.Shortcut(0, ui.KeyEscape) {
		*q = ""
	}
}
