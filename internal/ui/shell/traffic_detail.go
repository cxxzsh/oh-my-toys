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

// The views of the detail pane. Which of them are offered depends on the
// mode: a proxied flow has a request and a response, a packet has layers
// and bytes.
const (
	tabOverview = iota
	tabRequest
	tabResponse
	tabLayers
	tabHex
)

// detailTabs are the tabs of the mode in progress.
func detailTabs(mode tool.CaptureMode) []int {
	if mode == tool.ModePackets {
		return []int{tabLayers, tabHex}
	}
	return []int{tabOverview, tabRequest, tabResponse}
}

// detailTabLabel names one tab.
func detailTabLabel(tab int) string {
	switch tab {
	case tabOverview:
		return i18n.TrafficTabOverview()
	case tabRequest:
		return i18n.TrafficTabRequest()
	case tabResponse:
		return i18n.TrafficTabResponse()
	case tabLayers:
		return i18n.TrafficTabLayers()
	case tabHex:
		return i18n.TrafficTabHex()
	}
	return ""
}

// headersShare is the most of the detail pane the header lines of a
// message take before they scroll: the body keeps the rest.
const headersShare = 40

// trafficDetailPane is what the chosen flow carried.
func (a *App) trafficDetailPane(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	comp.Card(c).FillHeight().FillWidth().Clip().Padding(ds.Space12).Gap(ds.Space8).Children(func() {
		if !st.hasDetail {
			comp.Text(c, i18n.TrafficNoSelection(), ds.Caption, p.TextInactive)
			return
		}
		a.trafficDetailHead(c, t, st)
		a.trafficDetailTabs(c, st)
	})
}

// trafficDetailHead is the flow's number and line, and the way to send its
// request again.
func (a *App) trafficDetailHead(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	f := st.detail
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		comp.BadgeRimmed(c, "#"+strconv.FormatUint(f.ID, 10))
		comp.Text(c, trafficDetailLine(f), ds.Label, p.TextStrong).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		ui.Spacer(c)
		if f.Source != tool.SourceProxy {
			return
		}
		fromFlow := a.kit.Button(c.Key("detail-rule"), i18n.TrafficNewRule(), comp.ButtonOpts{Kind: comp.Secondary, Small: true})
		fromFlow.Menu(func(m *ui.Menu) { a.flowRuleMenu(m, t, st, f.FlowRow) })
		opts := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: a.trafficBusy(st)}
		if a.kit.Button(c.Key("detail-replay"), i18n.TrafficReplay(), opts).Clicked() {
			a.trafficReplay(t, st, f.ID)
		}
	})
}

// trafficDetailLine is a flow in one line: the request for a proxied flow,
// what the packet is for a packet.
func trafficDetailLine(f tool.Flow) string {
	if f.Source == tool.SourcePacket {
		return strings.Join([]string{f.Proto, f.Host, f.Info}, " ")
	}
	return strings.TrimSpace(f.Method + " " + f.URL)
}

// trafficDetailTabs chooses between the views of the flow.
func (a *App) trafficDetailTabs(c *ui.Context, st *trafficState) {
	kinds := detailTabs(tool.CaptureMode(st.mode))
	labels := make([]string, len(kinds))
	for i, kind := range kinds {
		labels[i] = detailTabLabel(kind)
	}
	st.detailTab = max(0, min(st.detailTab, len(kinds)-1))
	comp.Tabs(c.Key("detail-tabs"), &st.detailTab, i18n.TrafficDetail(), labels...)
	a.trafficDetailView(c, st, kinds[st.detailTab])
}

// trafficDetailView is the chosen view of the flow.
func (a *App) trafficDetailView(c *ui.Context, st *trafficState, tab int) {
	f := st.detail
	switch tab {
	case tabOverview:
		a.trafficOverview(c, st, f)
	case tabRequest:
		a.trafficMessage(c, f.Request)
	case tabResponse:
		a.trafficMessage(c, f.Response)
	case tabLayers:
		a.trafficLayers(c, f)
	case tabHex:
		a.trafficHex(c, f)
	}
}

// trafficOverview is everything the flow is, in a scroll so that a long
// URL and a list of rules both fit.
func (a *App) trafficOverview(c *ui.Context, st *trafficState, f tool.Flow) {
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

// trafficFact is one labelled value of the overview. A value that is not
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

// trafficMessage is one side of a flow: its header lines over its body.
func (a *App) trafficMessage(c *ui.Context, m tool.HTTPMessage) {
	ui.Column(c).Grow(1).FillWidth().Gap(ds.Space6).Children(func() {
		a.trafficHeaders(c, m.Headers)
		a.trafficBody(c, m.Body)
	})
}

// trafficHeaders are a message's header lines, in the order they were
// sent; they scroll when a message carries many.
func (a *App) trafficHeaders(c *ui.Context, headers []tool.Header) {
	if len(headers) == 0 {
		return
	}
	p := ds.P(c)
	_, windowH := c.Size()
	height := min(float32(len(headers))*ds.TableRow+2*ds.Space4, windowH*headersShare/100)
	ui.ScrollBoth(c).FillWidth().Height(height).Shrink(0).Padding(ds.Space4).Radius(ds.RadiusXs).
		Background(p.FillControl).Selectable().Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			for i, h := range headers {
				ui.Row(c.Key(i)).FillWidth().Height(ds.TableRow).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
					comp.Value(c, h.Name, p.TextSecondary).NoWrap()
					ui.Text(c, h.Value).NoWrap().TextColor(p.TextPrimary)
				})
			}
		})
	})
}

// trafficBody is a message's body: pretty JSON, text as it is, a form's
// pairs, the name of an image, or the bytes of anything else.
func (a *App) trafficBody(c *ui.Context, b tool.Body) {
	p := ds.P(c)
	text, kind := comp.FormatBody(b)
	note := ""
	if b.Truncated {
		note = i18n.TrafficBodyTruncated(trafficSize(b.Size))
	}
	opts := comp.CodeViewOpts{Label: i18n.TrafficBody(), Note: note, Capped: i18n.TrafficBodyCapped(trafficSize(ds.CodeMaxBytes))}
	switch kind {
	case comp.BodyEmpty:
		comp.Text(c, i18n.TrafficNoBody(), ds.Caption, p.TextInactive)
	case comp.BodyImage:
		comp.Text(c, i18n.TrafficImageBody(b.Type, trafficSize(b.Size)), ds.Caption, p.TextInactive)
	case comp.BodyBinary:
		comp.Text(c, i18n.TrafficBinaryBody(trafficSize(b.Size), b.Type), ds.Caption, p.TextInactive)
		opts.Copy = i18n.CopyValue()
		a.kit.CodeView(c, text, opts)
	default:
		opts.Copy = i18n.CopyValue()
		a.kit.CodeView(c, text, opts)
	}
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
						for j, field := range layer.Fields {
							a.trafficFact(c, field.Name, field.Value)
							_ = j
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
