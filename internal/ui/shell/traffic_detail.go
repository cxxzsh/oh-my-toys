package shell

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// The views of a packet's detail pane: its protocol layers and its bytes.
// A proxied flow shows its request and response instead; see
// traffic_detail_msg.go.
const (
	tabLayers = iota
	tabHex
)

// detailTabLabel names one packet tab.
func detailTabLabel(tab int) string {
	switch tab {
	case tabLayers:
		return i18n.TrafficTabLayers()
	case tabHex:
		return i18n.TrafficTabHex()
	}
	return ""
}

// trafficDetailPane is what the chosen flow carried.
func (a *App) trafficDetailPane(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	comp.Card(c).FillHeight().FillWidth().Clip().Padding(ds.Space12).Gap(ds.Space8).Children(func() {
		if !st.hasDetail {
			comp.Text(c, i18n.TrafficNoSelection(), ds.Caption, p.TextInactive)
			return
		}
		a.trafficDetailHead(c, t, st)
		if tool.CaptureMode(st.mode) == tool.ModePackets {
			a.trafficPacketTabs(c, st)
			return
		}
		a.trafficExchange(c, st)
	})
}

// trafficDetailWidth is the width the detail pane takes: its share of the
// panes beside the list, or all of them when the detail goes under the list.
func (a *App) trafficDetailWidth(c *ui.Context, st *trafficState) float32 {
	w := a.trafficPanesWidth(c, st)
	if w >= ds.TrafficWide {
		return w * (100 - st.splitP) / 100
	}
	return w
}

// trafficDetailHead is the flow's number and summary line, and for a proxied
// flow its time, size and the ways to act on the request.
func (a *App) trafficDetailHead(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	f := st.detail
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		comp.BadgeRimmed(c, "#"+strconv.FormatUint(f.ID, 10)).Shrink(0)
		if f.Source == tool.SourceProxy {
			a.trafficSummaryLine(c, f)
		} else {
			comp.Text(c, trafficDetailLine(f), ds.Label, p.TextStrong).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		}
		ui.Spacer(c)
		if f.Source != tool.SourceProxy {
			return
		}
		comp.Text(c, trafficTook(f.Duration), ds.Caption, p.TextInactive).NoWrap()
		comp.Text(c, trafficSize(f.Size), ds.Caption, p.TextInactive).NoWrap()
		a.trafficActions(c, t, st)
	})
}

// trafficSummaryLine is a proxied flow in one line: its method, its URL and
// its status.
func (a *App) trafficSummaryLine(c *ui.Context, f tool.Flow) {
	p := ds.P(c)
	comp.Text(c, f.Method, ds.Label, methodColor(p, f.Method)).NoWrap()
	comp.Text(c, f.URL, ds.Label, p.TextStrong).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(ds.Space4).Children(func() {
		a.trafficStatus(c, f.FlowRow, trafficStatusText(f))
	})
}

// trafficActions are the buttons on a proxied flow: a new rule from it, and
// its replay.
func (a *App) trafficActions(c *ui.Context, t tool.Traffic, st *trafficState) {
	f := st.detail
	fromFlow := a.kit.Button(c.Key("detail-rule"), i18n.TrafficNewRule(), comp.ButtonOpts{Kind: comp.Secondary, Small: true})
	fromFlow.Menu(func(m *ui.Menu) { a.flowRuleMenu(m, t, st, f.FlowRow) })
	opts := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: a.trafficBusy(st)}
	if a.kit.Button(c.Key("detail-replay"), i18n.TrafficReplay(), opts).Clicked() {
		a.trafficReplay(t, st, f.ID)
	}
}

// trafficDetailLine is what a packet is for, in one line.
func trafficDetailLine(f tool.Flow) string {
	return strings.Join([]string{f.Proto, f.Host, f.Info}, " ")
}

// trafficPacketTabs chooses between a packet's layers and its bytes.
func (a *App) trafficPacketTabs(c *ui.Context, st *trafficState) {
	kinds := []int{tabLayers, tabHex}
	labels := make([]string, len(kinds))
	for i, kind := range kinds {
		labels[i] = detailTabLabel(kind)
	}
	st.detailTab = max(0, min(st.detailTab, len(kinds)-1))
	comp.Tabs(c.Key("detail-tabs"), &st.detailTab, i18n.TrafficDetail(), labels...)
	if kinds[st.detailTab] == tabHex {
		a.trafficHex(c, st.detail)
		return
	}
	a.trafficLayers(c, st.detail)
}

// trafficStatusText is a flow's status as words when it has no code.
func trafficStatusText(f tool.Flow) string {
	switch {
	case f.State == tool.FlowBlocked:
		return i18n.TrafficBlocked()
	case f.State == tool.FlowFailed:
		return i18n.TrafficFailed()
	case f.Status > 0:
		return strconv.Itoa(f.Status) + " " + f.Info
	case f.State == tool.FlowOpen:
		return i18n.TrafficOpen()
	}
	return ""
}

// trafficFacts is everything the flow is, in a scroll so that a long URL and
// a list of rules both fit. It is the summary tab of the request.
func (a *App) trafficFacts(c *ui.Context, st *trafficState, f tool.Flow) {
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Gap(ds.Space4).Children(func() {
			a.trafficFact(c, i18n.TrafficURL(), f.URL)
			a.trafficFact(c, i18n.TrafficColMethod(), f.Method)
			a.trafficFact(c, i18n.StatusLabel(), trafficStatusText(f))
			a.trafficFact(c, i18n.TrafficColProtocol(), f.Proto)
			a.trafficFact(c, i18n.TrafficTLS(), f.TLSVersion)
			a.trafficFact(c, i18n.TrafficServer(), f.ServerAddr)
			a.trafficFact(c, i18n.TrafficStarted(), f.Started.Format("15:04:05.000"))
			a.trafficFact(c, i18n.TrafficColDuration(), trafficTook(f.Duration))
			a.trafficFact(c, i18n.TrafficColSize(), trafficSize(f.Size))
			a.trafficRuleHits(c, st, f.RuleHits)
			if f.ReplayOf != 0 {
				a.trafficFact(c, i18n.TrafficReplay(), i18n.TrafficReplayOf(strconv.FormatUint(f.ReplayOf, 10)))
			}
		})
	})
}

// trafficFact is one labelled value of the summary. A value that is not
// there is left out rather than shown empty.
func (a *App) trafficFact(c *ui.Context, label, value string) {
	if value == "" {
		return
	}
	p := ds.P(c)
	mono := ds.Body
	mono.Mono = true
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Start).Children(func() {
		comp.Text(c, label, ds.Caption, p.TextInactive).Width(ds.SelectMinWidth).Shrink(0)
		comp.Text(c, value, mono, p.TextPrimary).Grow(1).Shrink(1).MinWidth(0).Selectable()
	})
}

// trafficLayers is a packet's protocol layers as a tree: each layer, and
// its fields under it.
func (a *App) trafficLayers(c *ui.Context, f tool.Flow) {
	p := ds.P(c)
	if f.Packet == nil {
		return
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Gap(ds.Space6).Children(func() {
			for i, layer := range f.Packet.Layers {
				ui.Column(c.Key(i)).FillWidth().Gap(ds.Space2).Children(func() {
					ui.Row(c).Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
						a.kit.Glyph(c, "expand_more", ds.GlyphSm, p.TextMuted)
						comp.Text(c, layer.Name, ds.Section, p.TextStrong)
					})
					ui.Column(c).FillWidth().Padding(0, 0, 0, ds.Space16).
						BorderWidth(0, 0, 0, ds.LineWidth).BorderColor(p.LineHairline).Children(func() {
						for _, field := range layer.Fields {
							a.trafficFact(c, field.Name, field.Value)
						}
					})
				})
			}
		})
	})
}

// trafficHex is a packet's bytes as a hex dump.
func (a *App) trafficHex(c *ui.Context, f tool.Flow) {
	if f.Packet == nil {
		return
	}
	a.kit.CodeView(c, comp.HexDump(f.Packet.Raw), comp.CodeViewOpts{
		Label: i18n.TrafficTabHex(), Copy: i18n.CopyValue(),
	})
}
