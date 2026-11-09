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
// reason: nothing before the search field may change their position.
const (
	trafficRecordWidth = 96
	trafficModeWidth   = 184
	trafficScopeWidth  = 180
	trafficWarnWidth   = 14
	glyphTune          = "tune"
)

// trafficToolbar is the one row above the page, in a fixed order: record,
// mode, warning, search, then Settings. The flow list's own bar holds the
// count and the Clear and Export actions; the scope chip sits in the sidebar,
// where the programs are.
func (a *App) trafficToolbar(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		a.trafficRecord(c, t, st)
		a.trafficModeControl(c, t, st)
		a.trafficWarnCell(c, t)
		a.trafficSearchField(c, st)
		a.trafficSettingsButton(c, st)
	})
}

// trafficRecord starts and stops the capture. Both labels sit in one fixed
// cell, so the controls after it do not move when the label changes.
func (a *App) trafficRecord(c *ui.Context, t tool.Traffic, st *trafficState) {
	on := t.Status().On
	opts := comp.ButtonOpts{Kind: comp.Primary, Small: true, Glyph: glyphPower, Disabled: a.trafficBusy(st)}
	label := i18n.SwitchStart()
	if on {
		label, opts = i18n.Stop(), comp.ButtonOpts{Kind: comp.Danger, Small: true, Glyph: glyphPower, Disabled: a.trafficBusy(st)}
	}
	ui.Row(c).Width(trafficRecordWidth).Justify(ui.Center).Children(func() {
		if a.kit.Button(c.Key("traffic-on"), label, opts).Clicked() {
			a.trafficSetOn(t, st, !on)
		}
	})
}

// trafficModeControl picks between the proxy and the packet capture. It
// applies at once, as the rest of the mode's settings do.
func (a *App) trafficModeControl(c *ui.Context, t tool.Traffic, st *trafficState) {
	modes := []string{i18n.TrafficModeProxy(), i18n.TrafficModePackets()}
	ui.Row(c).Width(trafficModeWidth).Children(func() {
		if comp.Segmented(c.Key("traffic-mode"), &st.mode, modes, i18n.ModeLabel()).Changed() {
			a.trafficApplyNow(t, st)
		}
	})
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

// trafficCount shows how many flows the list holds. It sits in the list's bar,
// so it needs no fixed width: nothing after it has to keep its place.
func (a *App) trafficCount(c *ui.Context, st *trafficState) {
	comp.Value(c, i18n.TrafficFlowCount(len(st.rows)), ds.P(c).TextMuted).Ellipsis("…")
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

// trafficClearExport empties the log and saves it as a HAR file. Export is
// disabled rather than hidden outside the proxy, so Settings keeps its place.
func (a *App) trafficClearExport(c *ui.Context, t tool.Traffic, st *trafficState) {
	if a.kit.Button(c.Key("traffic-clear"), i18n.TrafficClear(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
		t.Log().Clear()
	}
	opts := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: t.Config().Mode != tool.ModeProxy}
	if a.kit.Button(c.Key("traffic-har"), i18n.TrafficExport(), opts).Clicked() {
		a.trafficExport(t, st)
	}
}

func (a *App) trafficSettingsButton(c *ui.Context, st *trafficState) {
	btn := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Glyph: glyphTune}
	if a.kit.Button(c.Key("traffic-settings"), i18n.TrafficSettings(), btn).Clicked() {
		st.settings = true
	}
}
