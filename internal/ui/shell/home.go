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

type homeKind int

const (
	homeChips homeKind = iota
	homeEyebrow
	homeCards
	homeRows
)

type homeRow struct {
	kind  homeKind
	label string
	tools []tool.Tool
}

// homeRows lays the home page out as rows of a virtual list, so that
// hundreds of tools build only the rows in view.
func (a *App) homeRows(width float32) []homeRow {
	cardCols := columns(width, ds.CardMinWidth, ds.Space3)
	rowCols := columns(width, ds.DenseMinWidth, ds.Space2)
	rows := []homeRow{{kind: homeChips}}
	if a.homeCat == "" {
		rows = appendSection(rows, i18n.Pinned(), homeCards, a.pinnedTools(), cardCols)
		rows = appendSection(rows, i18n.Recent(), homeRows, a.recentTools(), rowCols)
	}
	for _, cat := range tool.Categories() {
		if a.homeCat == "" || cat == a.homeCat {
			rows = appendSection(rows, categoryName(cat), homeRows, a.reg.InCategory(cat), rowCols)
		}
	}
	return rows
}

func appendSection(rows []homeRow, label string, kind homeKind, tools []tool.Tool, cols int) []homeRow {
	if len(tools) == 0 {
		return rows
	}
	rows = append(rows, homeRow{kind: homeEyebrow, label: label})
	for i := 0; i < len(tools); i += cols {
		rows = append(rows, homeRow{kind: kind, tools: tools[i:min(i+cols, len(tools))]})
	}
	return rows
}

func columns(width, minWidth, gap float32) int {
	return max(1, int((width+gap)/(minWidth+gap)))
}

func (a *App) recentTools() []tool.Tool {
	var out []tool.Tool
	for _, id := range a.settings.Recent {
		if t := a.reg.Get(id); t != nil {
			out = append(out, t)
		}
	}
	return out
}

func (a *App) contentWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	return min(w-ds.SidebarWidth, ds.ContentMaxWidth+2*ds.Space8) - 2*ds.Space8
}

func (a *App) home(c *ui.Context) {
	rows := a.homeRows(a.contentWidth(c))
	ui.List(c.Key("home"), &a.homeList, len(rows), func(i int) { a.homeRow(c, rows[i]) }).
		Grow(1).FillWidth().Padding(ds.Space6, ds.Space8).Label(i18n.AllTools())
}

func (a *App) homeRow(c *ui.Context, r homeRow) {
	switch r.kind {
	case homeChips:
		a.chips(c)
	case homeEyebrow:
		comp.Eyebrow(c, r.label).Padding(ds.Space6, 0, ds.Space2).Role(ui.RoleHeading).Level(2)
	case homeCards:
		a.gridRow(c, r.tools, ds.CardMinWidth, ds.Space3, a.toolCard)
	case homeRows:
		a.gridRow(c, r.tools, ds.DenseMinWidth, ds.Space2, a.toolRow)
	}
}

// gridRow lays tools out in equal columns; a short last row keeps the
// columns' width.
func (a *App) gridRow(c *ui.Context, tools []tool.Tool, minWidth, gap float32, cell func(*ui.Context, tool.Tool)) {
	cols := columns(a.contentWidth(c), minWidth, gap)
	ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Stretch).Padding(0, 0, gap).Children(func() {
		for i := range cols {
			ui.Column(c.Key(i)).Grow(1).Basis(0).Children(func() {
				if i < len(tools) {
					cell(c, tools[i])
				}
			})
		}
	})
}

func (a *App) chips(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(ds.ChipGap).Wrap().Children(func() {
		a.chip(c, "", i18n.All(), len(a.reg.All()))
		for _, cat := range tool.Categories() {
			if n := len(a.reg.InCategory(cat)); n > 0 {
				a.chip(c, cat, categoryName(cat), n)
			}
		}
	})
}

func (a *App) chip(c *ui.Context, cat tool.Category, label string, n int) {
	p := ds.P(c)
	sel := a.homeCat == cat
	b := ui.ButtonBase(c.Key("chip:" + string(cat))).Label(label).Selected(sel)
	if b.Clicked() {
		a.homeCat = cat
		a.homeList.ScrollTo(0, ui.Start)
	}
	bg, fg, count := p.FillControl, p.TextPrimary, p.TextTertiary
	if sel {
		bg, fg, count = p.TextPrimary, p.BgWindow, p.BgWindow.Alpha(ds.ChipCountAlpha)
	}
	b.Height(ds.ChipHeight).Padding(0, ds.ChipPadX).Gap(ds.ChipGap).Radius(ds.RadiusPill).Background(bg).
		AlignItems(ui.Center).Children(func() {
		comp.Text(c, label, ds.Callout, fg).SingleLine()
		comp.Text(c, strconv.Itoa(n), ds.Callout, count).SingleLine()
	})
}

// toolCard is a pinned tool: a switch tool shows its status and switch,
// any other tool "Open ›".
func (a *App) toolCard(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	m := t.Manifest()
	card := comp.Card(c.Key("card:" + m.ID)).FillWidth().MinHeight(ds.CardMinHeight).Padding(ds.Space4).Gap(ds.Space3)
	if card.Hovered() {
		ds.ApplyShadow(card, ds.S(c).ShadowPopover)
	}
	if card.Clicked() {
		a.open(m.ID)
	}
	card.Label(m.Name(msgs())).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space3).AlignItems(ui.Center).Children(func() {
			comp.ToolIcon(c, m.Category, m.Icon, comp.Medium, false)
			comp.Text(c, m.Name(msgs()), ds.Headline, p.TextPrimary).Grow(1).SingleLine().Ellipsis("…")
			a.pinButton(c, t)
		})
		comp.Text(c, m.Summary(msgs()), ds.Callout, p.TextSecondary).Grow(1).MaxLines(2)
		ui.Row(c).FillWidth().MinHeight(ds.PillHeight).Gap(ds.Space2).AlignItems(ui.Center).Children(func() {
			a.cardFoot(c, t)
		})
	})
}

func (a *App) cardFoot(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	if s, ok := t.(tool.Switch); ok && !t.Manifest().Soon {
		st := s.Status()
		comp.Pill(c, st.Text, st.On)
		ui.Spacer(c)
		on := st.On
		if comp.Switch(c, &on, t.Manifest().Name(msgs())).Changed() {
			a.setSwitch(s, on)
		}
		return
	}
	ui.Spacer(c)
	label := i18n.Open()
	if t.Manifest().Soon {
		label = i18n.ComingSoon()
	}
	ui.Row(c).Gap(ds.OpenGap).AlignItems(ui.Center).Children(func() {
		comp.Text(c, label, ds.Callout, p.TextSecondary)
		comp.Glyph(c, "chevron-right", ds.SmallGlyph, p.TextSecondary)
	})
}

func (a *App) pinButton(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	label, color := i18n.Pin(), p.TextTertiary
	if a.isPinned(t) {
		label, color = i18n.Unpin(), p.Accent
	}
	pin := ui.ButtonBase(c).Label(label).Tooltip(label)
	if pin.Clicked() {
		a.settings.TogglePin(t.Manifest().ID)
		a.save()
	}
	pin.Children(func() { comp.Glyph(c, "pin", ds.SmallGlyph, color) })
}

// toolRow is a compact tool in the home page's grid.
func (a *App) toolRow(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	m := t.Manifest()
	row := ui.ButtonBase(c.Key("row:" + m.ID)).Label(m.Name(msgs())).Description(m.Summary(msgs()))
	bg := p.BgCard
	if row.Hovered() {
		bg = p.BgElevated
	}
	if row.Clicked() {
		a.open(m.ID)
	}
	ds.ApplyShadow(row, ds.S(c).ShadowCard)
	row.FillWidth().Padding(ds.ToolRowPadY, ds.Space3).Gap(ds.Space3).Radius(ds.RadiusLg).Background(bg).
		Border(1, p.BorderCard).AlignItems(ui.Center).Children(func() {
		comp.ToolIcon(c, m.Category, m.Icon, comp.Medium, core.IsRunning(t))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			comp.Text(c, m.Name(msgs()), ds.Headline, p.TextPrimary).SingleLine().Ellipsis("…")
			comp.Text(c, m.Summary(msgs()), ds.Callout, p.TextSecondary).SingleLine().Ellipsis("…")
		})
		a.pinButton(c, t)
	})
}
