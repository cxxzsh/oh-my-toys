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

type traySection int

const (
	traySwitches traySection = iota
	trayTools
	trayFooter
)

type trayItem struct {
	section traySection
	sw      tool.Switch
	tool    tool.Tool
	glyph   string
	label   string
	hint    string
	run     func()
}

func (a *App) trayItems() []trayItem {
	var out []trayItem
	for _, t := range a.reg.All() {
		if sw, ok := t.(tool.Switch); ok && !t.Manifest().Soon {
			out = append(out, trayItem{section: traySwitches, sw: sw})
		}
	}
	for _, t := range a.pinnedTools() {
		if _, ok := t.(tool.Switch); !ok {
			out = append(out, trayItem{section: trayTools, tool: t})
		}
	}
	out = append(out,
		trayItem{
			section: trayFooter, glyph: "open_in_new", label: i18n.OpenApp(), hint: modKey("O"),
			run: func() { a.hideTray(); a.openMain(pageHome) },
		},
		trayItem{
			section: trayFooter, glyph: glyphSettings, label: i18n.SettingsEllipsis(), hint: modKey(","),
			run: func() { a.hideTray(); a.openMain(pageSettings) },
		},
		trayItem{
			section: trayFooter, glyph: "power_settings_new", label: i18n.QuitApp(), hint: modKey("Q"),
			run: mygo.App.Quit,
		},
	)
	return out
}

func (a *App) trayKeys(c *ui.Context, items []trayItem) {
	if c.Shortcut(0, ui.KeyEscape) {
		a.hideTray()
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.hideTray()
		a.openMain(pageHome)
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		a.hideTray()
		a.openMain(pageSettings)
		return
	}
	if len(items) == 0 {
		return
	}
	if c.Shortcut(0, ui.KeyDown) {
		a.trayChosen = min(a.trayChosen+1, len(items)-1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		a.trayChosen = max(a.trayChosen-1, 0)
	}
	it := items[min(a.trayChosen, len(items)-1)]
	if it.sw != nil && (c.Shortcut(0, ui.KeySpace) || c.Shortcut(0, ui.KeyEnter)) {
		a.setSwitch(it.sw, !it.sw.Status().On)
		return
	}
	if c.Shortcut(0, ui.KeyEnter) {
		a.runTrayItem(it)
	}
}

func (a *App) runTrayItem(it trayItem) {
	if it.sw != nil {
		a.setSwitch(it.sw, !it.sw.Status().On)
		return
	}
	if it.tool != nil {
		a.hideTray()
		a.openMain(it.tool.Manifest().ID)
		return
	}
	if it.run != nil {
		it.run()
	}
}

func (a *App) trayBody(c *ui.Context, items []trayItem) {
	p := ds.P(c)
	sec := traySection(-1)
	for i, it := range items {
		if it.section != sec {
			if sec >= 0 {
				ui.Row(c).FillWidth().Padding(ds.Space6, ds.Space8).Children(func() {
					ui.Box(c).FillWidth().Height(ds.LineWidth).Background(p.Line)
				})
			}
			sec = it.section
			switch sec {
			case traySwitches:
				comp.Eyebrow(c, i18n.Switches()).Padding(ds.Space8, ds.Space8, ds.Space4)
			case trayTools:
				comp.Eyebrow(c, i18n.Tools()).Padding(ds.Space8, ds.Space8, ds.Space4)
			case trayFooter:
			}
		}
		a.trayRow(c, i, it)
	}
}

func (a *App) trayRow(c *ui.Context, i int, it trayItem) {
	chosen := i == a.trayChosen
	switch {
	case it.sw != nil:
		a.traySwitchRow(c, i, it.sw, chosen)
	case it.tool != nil:
		a.trayToolRow(c, i, it.tool, chosen)
	default:
		a.trayFooterRow(c, i, it, chosen)
	}
}

func (a *App) traySwitchRow(c *ui.Context, i int, sw tool.Switch, chosen bool) {
	p := ds.P(c)
	m, st := sw.Manifest(), sw.Status()
	row := ui.ButtonBase(c.Key(i)).Label(m.Name(msgs())).Selected(chosen).Cursor(ui.CursorDefault)
	if row.Clicked() {
		a.trayChosen = i
		a.setSwitch(sw, !st.On)
	}
	row.ContextMenu(func(menu *ui.Menu) { a.toolMenu(menu, sw) })
	row.FillWidth().MinHeight(ds.TrayRow).Padding(ds.Space4, ds.Space8).Gap(ds.Space8).Radius(ds.RadiusSm).
		Background(trayRowFill(p, row.Hovered(), chosen)).AlignItems(ui.Center).Children(func() {
		a.kit.ToolIcon(c, m, comp.ToolIconOpts{Size: comp.IconSmall, Running: st.On, Tint: true})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			comp.Text(c, m.Name(msgs()), ds.Label, p.TextPrimary).SingleLine().Ellipsis("…")
			ink := p.TextMuted
			if st.On {
				ink = p.LiveText
			}
			comp.Value(c, st.Text, ink)
		})
		on := st.On
		if comp.SwitchCompact(c, &on, m.Name(msgs())).Changed() {
			a.setSwitch(sw, on)
		}
	})
	if st.On && len(m.Actions) >= 3 {
		a.traySwitchPresets(c, sw)
	}
}

func (a *App) traySwitchPresets(c *ui.Context, sw tool.Switch) {
	m := sw.Manifest()
	ui.Row(c).FillWidth().Padding(0, ds.Space8, ds.Space8, ds.TraySubInset).Children(func() {
		sel := 2
		opts := []string{i18n.Awake1hShort(), i18n.Awake2hShort(), i18n.AwakeForeverShort()}
		if comp.Segmented(c.Key("preset:"+m.ID), &sel, opts, m.Name(msgs())).Changed() && sel < len(m.Actions) {
			a.runAction(sw, &m.Actions[sel])
		}
	})
}

func (a *App) trayToolRow(c *ui.Context, i int, t tool.Tool, chosen bool) {
	p := ds.P(c)
	m := t.Manifest()
	label := m.Name(msgs()) + "…"
	row := ui.ButtonBase(c.Key(i)).Label(label).Selected(chosen).Cursor(ui.CursorDefault)
	if row.Clicked() {
		a.hideTray()
		a.openMain(m.ID)
	}
	row.ContextMenu(func(menu *ui.Menu) { a.toolMenu(menu, t) })
	row.FillWidth().Height(ds.TrayRow).Padding(0, ds.Space8).Gap(ds.Space8).Radius(ds.RadiusSm).
		Background(trayRowFill(p, row.Hovered(), chosen)).AlignItems(ui.Center).Children(func() {
		a.kit.Glyph(c, m.Icon, ds.Glyph, p.TextSecondary)
		comp.Text(c, label, ds.Label, p.TextPrimary).Grow(1).SingleLine().Ellipsis("…")
		if core.IsRunning(t) {
			comp.StatusDot(c, comp.DotOpts{Tone: comp.DotLive, Pulse: true})
		}
	})
}

func (a *App) trayFooterRow(c *ui.Context, i int, it trayItem, chosen bool) {
	p := ds.P(c)
	row := ui.ButtonBase(c.Key(i)).Label(it.label).Selected(chosen).Cursor(ui.CursorDefault)
	if row.Clicked() {
		it.run()
	}
	row.FillWidth().Height(ds.TrayRow).Padding(0, ds.Space8).Gap(ds.Space8).Radius(ds.RadiusSm).
		Background(trayRowFill(p, row.Hovered(), chosen)).AlignItems(ui.Center).Children(func() {
		a.kit.Glyph(c, it.glyph, ds.Glyph, p.TextSecondary)
		comp.Text(c, it.label, ds.Label, p.TextPrimary).Grow(1).SingleLine()
		if it.hint != "" {
			comp.Hint(c, it.hint)
		}
	})
}

func trayRowFill(p *ds.Palette, hovered, chosen bool) ui.Color {
	switch {
	case chosen:
		return p.AccentSoft
	case hovered:
		return p.FillControl
	}
	return ui.Transparent
}

func (a *App) toolMenu(menu *ui.Menu, t tool.Tool) {
	m := t.Manifest()
	for k := range m.Actions {
		act := &m.Actions[k]
		if menu.Item(act.Title(msgs())).Chosen() {
			a.runAction(t, act)
		}
	}
	if len(m.Actions) > 0 {
		menu.Separator()
	}
	if menu.Item(i18n.OpenInMain()).Chosen() {
		a.hideTray()
		a.openMain(m.ID)
	}
}

func (a *App) runResult(r core.Result) {
	if r.Action != nil {
		a.runAction(r.Tool, r.Action)
		return
	}
	a.activate(r.Tool)
}
