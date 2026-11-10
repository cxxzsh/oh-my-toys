package shell

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

func (a *App) sidebarWidth() float32 {
	switch {
	case a.sideHidden:
		return 0
	case a.sideCompact:
		return ds.SidebarCompactW
	default:
		return ds.SidebarWidth
	}
}

func (a *App) sidebarCompactButton(c *ui.Context) {
	p := ds.P(c)
	glyph, label := "left_panel_close", i18n.RailCompact()
	if a.sideCompact {
		glyph, label = "left_panel_open", i18n.RailExpand()
	}
	b := ui.ButtonBase(c.Key("compact-side")).Label(label).Tooltip(label).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.sideCompact = !a.sideCompact
	}
	fill, ink := ui.Transparent, p.TextMuted
	if b.Hovered() {
		fill, ink = p.FillControl, p.TextStrong
	}
	b.Size(ds.ControlSm, ds.ControlSm).Radius(ds.RadiusXs).Background(fill).Center().Children(func() {
		a.kit.Glyph(c, glyph, ds.GlyphSm, ink)
	})
}

func (a *App) compactRail(c *ui.Context, fill ui.Color) {
	p := ds.P(c)
	ui.Column(c).Width(ds.SidebarCompactW).FillHeight().Padding(ds.Space8, ds.Space6).Gap(ds.Space6).
		Background(fill).BorderWidth(0, ds.LineWidth, 0, 0).BorderColor(p.Line).AlignItems(ui.Center).
		Role(ui.RoleGroup).Label(i18n.Sidebar()).Children(func() {
		a.compactRailGlyph(c, pageHome, "apps", i18n.AllTools())
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
				for _, t := range a.jumpTools() {
					a.compactRailTool(c, t)
				}
			})
		})
		a.compactRailGlyph(c, pageSettings, glyphSettings, i18n.Settings())
	})
}

func (a *App) compactRailGlyph(c *ui.Context, page, glyph, label string) {
	p := ds.P(c)
	sel := a.page == page
	b := ui.ButtonBase(c.Key("cnav:" + page)).Label(label).Tooltip(label).Selected(sel).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.open(page)
	}
	fill, ink := ui.Transparent, p.TextSecondary
	switch {
	case sel:
		fill, ink = p.BgSelected, p.Accent
		b.BorderWidth(0, 0, 0, ds.SelectionBarW).BorderColor(p.AccentStrong)
	case b.Hovered():
		fill, ink = p.FillControl, p.TextStrong
	}
	b.Size(36, 34).Radius(ds.RadiusSm).Background(fill).Center().Children(func() {
		a.kit.Glyph(c, glyph, ds.Glyph, ink)
	})
}

func (a *App) compactRailTool(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	m := t.Manifest()
	sel := a.page == m.ID
	name := m.Name(msgs())
	b := ui.ButtonBase(c.Key("ctool:" + m.ID)).Label(name).Tooltip(name).Selected(sel).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.open(m.ID)
	}
	fill := ui.Transparent
	switch {
	case sel:
		fill = p.BgSelected
		b.BorderWidth(0, 0, 0, ds.SelectionBarW).BorderColor(p.AccentStrong)
	case b.Hovered():
		fill = p.FillControl
	}
	b.Size(38, 34).Radius(ds.RadiusSm).Background(fill).Center().Children(func() {
		a.kit.ToolIcon(c, m, comp.ToolIconOpts{Size: comp.IconSmall, Running: core.IsRunning(t), Tint: true})
	})
}
