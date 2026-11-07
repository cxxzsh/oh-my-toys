package shell

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

type homeKind int

const (
	homeHeader homeKind = iota
	homeEyebrow
	homeCards
)

type homeRow struct {
	kind  homeKind
	label string
	tools []tool.Tool
}

func (a *App) filteredTools(tools []tool.Tool) []tool.Tool {
	q := strings.ToLower(strings.TrimSpace(a.pageFilter))
	if q == "" {
		return tools
	}
	var out []tool.Tool
	for _, t := range tools {
		m := t.Manifest()
		if strings.Contains(strings.ToLower(m.Name(msgs())), q) ||
			strings.Contains(strings.ToLower(m.Summary(msgs())), q) {
			out = append(out, t)
		}
	}
	return out
}

func (a *App) homeRows(width float32) []homeRow {
	cols := columns(width, ds.CardMinColumn, ds.Space12)
	rows := []homeRow{{kind: homeHeader}}
	if a.homeCat == "" {
		rows = appendSection(rows, i18n.Pinned(), a.filteredTools(a.pinnedTools()), cols)
	}
	for _, cat := range tool.Categories() {
		if a.homeCat == "" || cat == a.homeCat {
			rows = appendSection(rows, categoryName(cat), a.filteredTools(a.reg.InCategory(cat)), cols)
		}
	}
	return rows
}

func appendSection(rows []homeRow, label string, tools []tool.Tool, cols int) []homeRow {
	if len(tools) == 0 {
		return rows
	}
	rows = append(rows, homeRow{kind: homeEyebrow, label: label})
	for i := 0; i < len(tools); i += cols {
		rows = append(rows, homeRow{kind: homeCards, tools: tools[i:min(i+cols, len(tools))]})
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

func (a *App) workbenchWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	if !a.sideHidden {
		w -= ds.SidebarWidth
	}
	return max(ds.CardMinColumn, w-2*ds.Space16)
}

func (a *App) home(c *ui.Context) {
	rows := a.homeRows(a.workbenchWidth(c))
	ui.List(c.Key("home"), &a.homeList, len(rows), func(i int) { a.homeRow(c, rows[i]) }).
		Grow(1).FillWidth().Padding(ds.Space16).Label(i18n.AllTools())
}

func (a *App) homeRow(c *ui.Context, r homeRow) {
	switch r.kind {
	case homeHeader:
		ui.Box(c).FillWidth().Padding(0, 0, ds.Space12).Children(func() {
			comp.PageHeader(c, comp.Header{
				Title:       a.title(),
				Badge:       fmt.Sprintf("%d", len(a.reg.All())),
				Description: i18n.PaletteEmpty(),
				Filters:     func() { a.chips(c) },
			})
		})
	case homeEyebrow:
		comp.Eyebrow(c, r.label).Padding(ds.Space12, 0, ds.Space8).Role(ui.RoleHeading).Level(2)
	case homeCards:
		a.cardRow(c, r.tools)
	}
}

func (a *App) cardRow(c *ui.Context, tools []tool.Tool) {
	cols := columns(a.workbenchWidth(c), ds.CardMinColumn, ds.Space12)
	ui.Row(c).FillWidth().Gap(ds.Space12).AlignItems(ui.Stretch).Padding(0, 0, ds.Space12).Children(func() {
		for i := range cols {
			ui.Column(c.Key(i)).Grow(1).Basis(0).Children(func() {
				if i < len(tools) {
					a.toolCard(c, tools[i])
				}
			})
		}
	})
}

func (a *App) chips(c *ui.Context) {
	ui.Row(c).Gap(ds.Space6).Wrap().Children(func() {
		a.chip(c, "", i18n.All(), len(a.reg.All()))
		for _, cat := range tool.Categories() {
			if n := len(a.reg.InCategory(cat)); n > 0 {
				a.chip(c, cat, categoryName(cat), n)
			}
		}
	})
}

func (a *App) chip(c *ui.Context, cat tool.Category, label string, n int) {
	text := fmt.Sprintf("%s (%d)", label, n)
	if comp.Chip(c.Key("chip:"+string(cat)), text, a.homeCat == cat).Clicked() {
		a.homeCat = cat
		a.homeList.ScrollTo(0, ui.Start)
	}
}

// toolCard is a tool on the Home grid: icon, name, one-line summary, and
// a Switch for switch tools or an "Open ›" link for action tools.
func (a *App) toolCard(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	m := t.Manifest()
	card := ui.ButtonBase(c.Key("card:" + m.ID)).Label(m.Name(msgs())).Description(m.Summary(msgs())).
		Cursor(ui.CursorDefault)
	if card.Clicked() {
		a.open(m.ID)
	}
	card.ContextMenu(func(menu *ui.Menu) { a.sidebarToolMenu(menu, t) })
	edge, shadow := p.Line, ds.S(c).ShadowXs
	if card.Hovered() {
		edge, shadow = p.LineHover, ds.S(c).ShadowSm
	}
	ds.ApplyShadow(card.FillWidth().Column().MinHeight(ds.CardMinHeight).Padding(ds.Space16).Gap(ds.Space10).
		Radius(ds.RadiusMd).Background(p.BgCard).Border(ds.LineWidth, edge).Justify(ui.SpaceBetween).
		AlignItems(ui.Start), shadow)
	card.Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space10).AlignItems(ui.Center).Children(func() {
			a.kit.ToolIcon(c, m, comp.ToolIconOpts{Size: comp.IconMedium})
			comp.Text(c, m.Name(msgs()), ds.Headline, p.TextStrong).Grow(1).SingleLine().Ellipsis("…")
			a.pinButton(c, t)
		})
		comp.Text(c, m.Summary(msgs()), ds.Caption, p.TextInactive).FillWidth().TextAlign(ui.Start).Grow(1).MaxLines(2)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() { a.cardFoot(c, t) })
	})
}

func (a *App) cardFoot(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	m := t.Manifest()
	if s, ok := t.(tool.Switch); ok && !m.Soon {
		st := s.Status()
		ink := p.TextMuted
		if st.On {
			ink = p.LiveText
		}
		comp.Value(c, st.Text, ink)
		ui.Spacer(c)
		on := st.On
		if comp.Switch(c, &on, m.Name(msgs())).Changed() {
			a.setSwitch(s, on)
		}
		return
	}
	comp.CategoryBadge(c, m.Category, categoryName(m.Category))
	ui.Spacer(c)
	label := i18n.Open() + " ›"
	if m.Soon {
		label = i18n.ComingSoon()
	}
	comp.Text(c, label, ds.Label, p.Accent).Cursor(ui.CursorPointer)
}

func (a *App) pinButton(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	glyph, label, color := glyphPin, i18n.Pin(), p.TextMuted
	if a.isPinned(t) {
		glyph, label, color = glyphPinned, i18n.Unpin(), p.Accent
		if !a.active() {
			color = p.TextMuted
		}
	}
	pin := ui.ButtonBase(c).Label(label).Tooltip(label).Cursor(ui.CursorDefault)
	if pin.Clicked() {
		a.settings.TogglePin(t.Manifest().ID)
		a.save()
	}
	pin.Children(func() { a.kit.Glyph(c, glyph, ds.SearchGlyph, color) })
}
