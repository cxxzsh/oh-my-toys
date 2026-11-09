package shell

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// trafficBottom is the bar under the flow list: how many flows it shows, the
// switch that selects each new flow as it arrives, then Clear and Export HAR.
// Export is disabled rather than hidden outside the proxy, so the bar keeps
// its place.
func (a *App) trafficBottom(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.ControlSm).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		comp.Value(c, trafficCountText(st), p.TextMuted).Ellipsis("…")
		ui.Row(c).Grow(1)
		comp.Text(c, i18n.TrafficAutoSelect(), ds.Label, p.TextPrimary).SingleLine()
		comp.Switch(c.Key("traffic-auto"), &st.autoSelect, i18n.TrafficAutoSelect())
		if a.kit.Button(c.Key("traffic-clear"), i18n.TrafficClear(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
			t.Log().Clear()
		}
		opts := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: t.Config().Mode != tool.ModeProxy}
		if a.kit.Button(c.Key("traffic-har"), i18n.TrafficExport(), opts).Clicked() {
			a.trafficExport(t, st)
		}
	})
}

// trafficCountText says how many flows the list shows, and how many the log
// holds when a filter or a sources pick narrows the list.
func trafficCountText(st *trafficState) string {
	if st.total == len(st.rows) {
		return i18n.TrafficFlowCount(len(st.rows))
	}
	return i18n.TrafficFlowCountOf(len(st.rows), st.total)
}
