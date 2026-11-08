package shell

import (
	"slices"
	"time"

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

// traySwitchRow is a switch tool in the tray: the row with its switch, and
// under it how it is running and how to start it another way.
func (a *App) traySwitchRow(c *ui.Context, i int, sw tool.Switch, chosen bool) {
	p := ds.P(c)
	m, st := sw.Manifest(), sw.Status()
	if st.On {
		c.After(tick)
	}
	row := ui.ButtonBase(c.Key(i)).Label(m.Name(msgs())).Selected(chosen).Cursor(ui.CursorDefault)
	if row.Clicked() {
		a.trayChosen = i
		a.hideTray()
		a.openMain(m.ID)
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
	a.traySwitchBody(c, sw, st)
}

// traySwitchBody is under a switch's row: the time left of a run, the ways
// to start it, and when its plan next turns it on.
func (a *App) traySwitchBody(c *ui.Context, sw tool.Switch, st tool.Status) {
	left, total, timed := timeLeft(st)
	chips := hasChips(sw)
	next, planned := trayPlanLine(sw, st)
	if !timed && !chips && !planned {
		return
	}
	ui.Column(c).FillWidth().Gap(ds.Space6).Padding(ds.Space2, ds.Space8, ds.Space8, ds.TraySubInset).Children(func() {
		if timed {
			comp.ProgressBar(c, float32(left)/float32(total))
		}
		if chips {
			a.trayChips(c, sw, st)
		}
		if planned {
			comp.Text(c, next, ds.Caption, ds.P(c).TextInactive)
		}
	})
}

// hasChips reports whether a tool has anything to show as a chip.
func hasChips(sw tool.Switch) bool {
	if _, ok := sw.(tool.Timed); ok {
		return true
	}
	return slices.ContainsFunc(sw.Manifest().Actions, func(act tool.QuickAction) bool { return act.Short != nil })
}

// trayChips are the ways to start a tool in one click, the one in effect
// marked; then a way to the page for a length of the person's own, and a
// way to give a run that ends by itself more time.
func (a *App) trayChips(c *ui.Context, sw tool.Switch, st tool.Status) {
	m := sw.Manifest()
	ui.Row(c).FillWidth().Gap(ds.Space4).Wrap().Children(func() {
		for i := range m.Actions {
			act := &m.Actions[i]
			if act.Short != nil && a.trayChip(c, act.ID, act.Short(msgs()), st.On && st.Active == act.ID) {
				a.runAction(sw, act)
			}
		}
		timed, ok := sw.(tool.Timed)
		if !ok {
			return
		}
		if a.trayChip(c, "custom", i18n.SwitchCustom()+"…", false) {
			a.hideTray()
			a.openMain(m.ID)
		}
		if canExtend(st) && a.trayChip(c, "extend", i18n.SwitchExtend(int(extendBy/time.Minute)), false) {
			a.extend(timed)
		}
	})
}

// trayChip is one way to start a tool; it reports a click.
func (a *App) trayChip(c *ui.Context, key, label string, active bool) bool {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key(key)).Label(label).Selected(active).Cursor(ui.CursorDefault)
	clicked := b.Clicked()
	style, ink, fill, line := ds.Caption, p.TextSecondary, p.FillControl, p.LineHairline
	switch {
	case active:
		style.Weight, ink, fill, line = ds.Semibold, p.AccentText, p.AccentSoft, p.AccentLine
	case b.Hovered():
		fill = p.FillControlHover
	}
	b.Height(ds.ControlSm).Padding(0, ds.Space8).Radius(ds.RadiusSm).Background(fill).
		Border(ds.LineWidth, line).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		comp.Text(c, label, style, ink).SingleLine()
	})
	return clicked
}

// trayPlanLine says when the plan of a tool next turns it on, unless the
// plan holds it on now or is off; the note, when there is one, instead.
func trayPlanLine(sw tool.Switch, st tool.Status) (text string, ok bool) {
	if st.Note != "" {
		return st.Note, true
	}
	sched, has := sw.(tool.Scheduled)
	if !has || st.Active == tool.ActivePlan {
		return "", false
	}
	return nextRunText(sched.Schedule(), time.Now())
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
