package shell

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Fixed widths of the toolbar's cells. A cell keeps its place whatever it
// shows, so the bar does not jump when a capture starts, stops, gets a note
// or narrows to a program. The controls on the right stay put for the same
// reason: nothing before them may change their position.
const (
	trafficRecordWidth    = 96
	trafficScopeWidth     = 180
	trafficWarnWidth      = 14
	trafficSearchMinWidth = 220
	trafficSearchMaxWidth = 360
)

// trafficToolbar is the one row above the page, in a fixed order: the sources
// toggle, record, warning, status text, search, Rules, Terminal, then
// Settings. The flow list's bottom bar holds the count and the Clear and
// Export actions.
func (a *App) trafficToolbar(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		a.trafficSourcesToggle(c, st)
		a.trafficRecord(c, t, st)
		a.trafficWarnCell(c, t)
		a.trafficStatusText(c, t)
		a.trafficSearchField(c, st)
		a.trafficRulesToggle(c, st)
		a.trafficTerminalButton(c, t, st)
		a.trafficSettingsButton(c, st)
	})
}

// trafficRecord starts and stops the capture. Both labels sit in one fixed
// cell, so the controls after it do not move when the label changes. Stopped,
// the button is filled like a primary action; running, it is plain.
func (a *App) trafficRecord(c *ui.Context, t tool.Traffic, st *trafficState) {
	on, busy := t.Status().On, a.trafficBusy(st)
	label, opacity := i18n.SwitchStart(), float32(1)
	if on {
		label = i18n.Stop()
	}
	if busy {
		opacity = ds.OpacityDisabled
	}
	p := ds.P(c)
	fill, ink, edge := p.Inverse, p.OnInverse, p.Inverse
	if on {
		fill, ink, edge = p.BgButton, p.TextPrimary, p.LineControl
	}
	b := ui.ButtonBase(c.Key("traffic-on")).Label(label).Cursor(ui.CursorDefault).Disabled(busy)
	b.Height(ds.ControlSm).Padding(0, ds.Space10).Gap(ds.Space6).Radius(ds.RadiusSm).Background(fill)
	b.Border(ds.LineWidth, edge).Opacity(opacity).AlignItems(ui.Center)
	ui.Row(c).Width(trafficRecordWidth).Children(func() {
		if b.Children(func() {
			trafficRecordDot(c, on)
			comp.Text(c, label, ds.Caption, ink).SingleLine()
		}).Clicked() {
			a.trafficSetOn(t, st, !on)
		}
	})
}

// trafficRecordDot is the dot inside the record button: solid red while the
// capture runs, a white dot on the filled Start button while it is stopped.
func trafficRecordDot(c *ui.Context, on bool) {
	if on {
		comp.StatusDot(c, comp.DotOpts{Tone: comp.DotDanger})
		return
	}
	ui.Box(c).Size(ds.Dot, ds.Dot).Radius(ds.RadiusFull).Background(ds.P(c).OnInverse).Shrink(0)
}

// trafficWarnCell holds a dot with the status note as its tooltip. The cell
// is always drawn at the same width, so a note appearing does not shift the
// bar.
func (a *App) trafficWarnCell(c *ui.Context, t tool.Traffic) {
	note := t.Status().Note
	ui.Row(c).Width(trafficWarnWidth).AlignItems(ui.Center).Children(func() {
		if note == "" {
			return
		}
		comp.StatusDot(c, comp.DotOpts{Tone: comp.DotWarning}).Tooltip(note)
	})
}

// trafficStatusText shows what the capture is doing in muted text. It takes
// the free width of the bar and cuts its end short, never the controls.
func (a *App) trafficStatusText(c *ui.Context, t tool.Traffic) {
	ui.Row(c).Grow(1).Clip().AlignItems(ui.Center).Children(func() {
		comp.Value(c, t.Status().Text, ds.P(c).TextMuted).Ellipsis("…")
	})
}

// trafficRulesToggle switches the page between the flow list and the rules.
// It is a chip, so it reads as a mode that is on while the rules show.
func (a *App) trafficRulesToggle(c *ui.Context, st *trafficState) {
	on := st.view == viewRules
	if comp.Chip(c.Key("traffic-rules"), i18n.TrafficViewRules(), on).Clicked() {
		st.view = viewFlows
		if !on {
			st.view = viewRules
		}
	}
}

// trafficTerminalButton opens a terminal that uses the proxy. It is off while
// a start or stop is being applied.
func (a *App) trafficTerminalButton(c *ui.Context, t tool.Traffic, st *trafficState) {
	opts := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: a.trafficBusy(st)}
	if a.kit.Button(c.Key("traffic-terminal"), i18n.TrafficTerminal(), opts).Clicked() {
		a.trafficOpenTerminal(t, st)
	}
}

// trafficSourcesToggle shows or hides the sources column. Until a person
// chooses, the column opens by itself on a wide page.
func (a *App) trafficSourcesToggle(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	shown := a.trafficSourcesShown(c, st)
	label, glyph := i18n.TrafficSourcesShow(), glyphSourcesOpen
	if shown {
		label, glyph = i18n.TrafficSourcesHide(), glyphSourcesClose
	}
	btn := ui.ButtonBase(c.Key("traffic-sources")).Label(label).Cursor(ui.CursorDefault)
	btn.Tooltip(label)
	btn.Width(ds.ControlSm).Height(ds.ControlSm).Radius(ds.RadiusFull).Background(ui.Transparent)
	btn.AlignItems(ui.Center).Justify(ui.Center)
	if btn.Children(func() {
		a.kit.Glyph(c, glyph, ds.Glyph, p.TextSecondary)
	}).Clicked() {
		st.sources = sourcesClosed
		if !shown {
			st.sources = sourcesOpen
		}
	}
}

// trafficScope is the chip of the programs being captured. The slot is kept
// even when no scope is set. Clicking the chip records everything again.
func (a *App) trafficScope(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Row(c).Width(trafficScopeWidth).Clip().AlignItems(ui.Center).Children(func() {
		scope := t.Config().Scope
		if len(scope) == 0 {
			return
		}
		label := i18n.TrafficScopeChip(strings.Join(scope, ", "))
		if comp.Chip(c, label, true).Tooltip(i18n.TrafficScopeClear()).Clicked() {
			a.trafficSetScope(t, st, nil)
		}
	})
}

// trafficSettingsButton is the gear at the far right. It is icon-only, so it
// takes its name from its label, which assistive technology reads, and shows
// the same words as its tooltip.
func (a *App) trafficSettingsButton(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	gear := ui.ButtonBase(c.Key("traffic-settings")).Label(i18n.TrafficSettings()).Cursor(ui.CursorDefault)
	gear.Tooltip(i18n.TrafficSettings())
	gear.Width(ds.ControlSm).Height(ds.ControlSm).Radius(ds.RadiusFull).Background(ui.Transparent)
	gear.AlignItems(ui.Center).Justify(ui.Center)
	if gear.Children(func() {
		a.kit.Glyph(c, glyphSettings, ds.Glyph, p.TextSecondary)
	}).Clicked() {
		st.settings = true
	}
}
