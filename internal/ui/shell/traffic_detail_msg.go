package shell

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// detailSide is which message of a proxied flow a pane shows.
type detailSide int

const (
	sideRequest detailSide = iota
	sideResponse
)

// The tabs of a message pane. tabNone means the person has not picked one
// yet, so the pane opens on its default.
const (
	tabNone = iota
	tabHeaders
	tabQuery
	tabBody
	tabRaw
	tabSummary
)

// headersShare is the most of the detail pane the header lines of a
// message take before they scroll: the body keeps the rest.
const headersShare = 40

// detailMemo is one tab's text of a flow, made once when the flow is shown.
type detailMemo struct {
	text  string
	kind  comp.BodyKind
	pairs []tool.Header
}

// memoKey names a memo: which flow, which side of it, which tab.
type memoKey struct {
	id   uint64
	side detailSide
	tab  int
}

// trafficExchange is a proxied flow's request and response: side by side
// when the detail pane is wide enough, one above the other when it is not.
func (a *App) trafficExchange(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	request := func() { a.trafficPane(c, st, sideRequest) }
	response := func() { a.trafficPane(c, st, sideResponse) }
	if a.trafficDetailWidth(c, st) >= ds.TrafficDetailWide {
		ui.Row(c).Grow(1).FillWidth().Gap(ds.Space12).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(request)
			ui.Column(c).Width(ds.LineWidth).FillHeight().Background(p.LineHairline)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(response)
		})
		return
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().FillHeight().Gap(ds.Space12).Children(func() {
			ui.Column(c).Grow(1).FillWidth().MinHeight(ds.TrafficPaneMinH).Children(request)
			ui.Column(c).Grow(1).FillWidth().MinHeight(ds.TrafficPaneMinH).Children(response)
		})
	})
}

// trafficPane is one message of a flow under its own tab strip.
func (a *App) trafficPane(c *ui.Context, st *trafficState, side detailSide) {
	kinds := paneTabs(st.detail, side)
	labels := make([]string, len(kinds))
	for i, kind := range kinds {
		labels[i] = paneTabLabel(kind)
	}
	sel := tabIndex(kinds, st.paneTab[side], defaultTab(messageOf(st.detail, side)))
	title, key := i18n.TrafficTabRequest(), "request-tabs"
	if side == sideResponse {
		title, key = i18n.TrafficTabResponse(), "response-tabs"
	}
	if comp.Tabs(c.Key(key), &sel, title, labels...).Changed() {
		st.paneTab[side] = kinds[sel]
	}
	a.trafficPaneView(c, st, side, kinds[sel])
}

// paneTabs are the tabs a message pane offers for the flow. The query is
// only offered when the URL has one, and the summary only on the request.
func paneTabs(f tool.Flow, side detailSide) []int {
	if side == sideResponse {
		return []int{tabHeaders, tabBody, tabRaw}
	}
	kinds := []int{tabHeaders}
	if strings.Contains(f.URL, "?") {
		kinds = append(kinds, tabQuery)
	}
	return append(kinds, tabBody, tabRaw, tabSummary)
}

// paneTabLabel names one message tab.
func paneTabLabel(tab int) string {
	switch tab {
	case tabHeaders:
		return i18n.TrafficHeaders()
	case tabQuery:
		return i18n.TrafficTabQuery()
	case tabBody:
		return i18n.TrafficBody()
	case tabRaw:
		return i18n.TrafficTabRaw()
	case tabSummary:
		return i18n.TrafficTabSummary()
	}
	return ""
}

// tabIndex is where a tab strip stands: on the tab the person picked when
// it is offered, on the default otherwise.
func tabIndex(kinds []int, picked, def int) int {
	for _, want := range []int{picked, def} {
		if i := slices.Index(kinds, want); i >= 0 {
			return i
		}
	}
	return 0
}

// defaultTab is the tab a pane opens on: the body when the message has one,
// its headers otherwise.
func defaultTab(m tool.HTTPMessage) int {
	if m.Body.Size > 0 || len(m.Body.Data) > 0 {
		return tabBody
	}
	return tabHeaders
}

// messageOf is the message of the flow on one side.
func messageOf(f tool.Flow, side detailSide) tool.HTTPMessage {
	if side == sideResponse {
		return f.Response
	}
	return f.Request
}

// trafficPaneView is the chosen tab of a message.
func (a *App) trafficPaneView(c *ui.Context, st *trafficState, side detailSide, tab int) {
	switch tab {
	case tabHeaders:
		a.trafficHeaders(c, messageOf(st.detail, side).Headers)
	case tabQuery:
		a.trafficQuery(c, st)
	case tabBody:
		a.trafficBodyTab(c, st, side)
	case tabRaw:
		a.trafficRaw(c, st, side)
	case tabSummary:
		a.trafficFacts(c, st, st.detail)
	}
}

// trafficQuery is the URL's query parameters as name and value lines.
func (a *App) trafficQuery(c *ui.Context, st *trafficState) {
	pairs := st.memoFor(sideRequest, tabQuery).pairs
	if len(pairs) == 0 {
		comp.Text(c, i18n.TrafficNoQuery(), ds.Caption, ds.P(c).TextInactive)
		return
	}
	a.trafficHeaders(c, pairs)
}

// trafficBodyTab is a message's body with its media type and size above it.
func (a *App) trafficBodyTab(c *ui.Context, st *trafficState, side detailSide) {
	m := messageOf(st.detail, side)
	memo := st.memoFor(side, tabBody)
	if memo.kind != comp.BodyEmpty {
		comp.Text(c, i18n.TrafficBodyMeta(m.Body.Type, trafficSize(m.Body.Size)), ds.Caption, ds.P(c).TextInactive)
	}
	a.trafficBody(c, m.Body, memo)
}

// trafficRaw is a message as it went over the wire.
func (a *App) trafficRaw(c *ui.Context, st *trafficState, side detailSide) {
	a.kit.CodeView(c, st.memoFor(side, tabRaw).text, comp.CodeViewOpts{
		Label: i18n.TrafficTabRaw(), Copy: i18n.CopyValue(),
		Capped: i18n.TrafficBodyCapped(trafficSize(ds.CodeMaxBytes)),
	})
}

// trafficHeaders is a table of name and value lines: a message's headers,
// or a query's parameters. It scrolls when there are many.
func (a *App) trafficHeaders(c *ui.Context, lines []tool.Header) {
	if len(lines) == 0 {
		return
	}
	p := ds.P(c)
	_, windowH := c.Size()
	height := min(float32(len(lines))*ds.TableRow+2*ds.Space4, windowH*headersShare/100)
	ui.ScrollBoth(c).FillWidth().Height(height).Shrink(0).Padding(ds.Space4).Radius(ds.RadiusXs).
		Background(p.FillControl).Selectable().Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			for i, h := range lines {
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
func (a *App) trafficBody(c *ui.Context, b tool.Body, memo detailMemo) {
	p := ds.P(c)
	note := ""
	if b.Truncated {
		note = i18n.TrafficBodyTruncated(trafficSize(b.Size))
	}
	opts := comp.CodeViewOpts{Label: i18n.TrafficBody(), Note: note, Capped: i18n.TrafficBodyCapped(trafficSize(ds.CodeMaxBytes))}
	switch memo.kind {
	case comp.BodyEmpty:
		comp.Text(c, i18n.TrafficNoBody(), ds.Caption, p.TextInactive)
	case comp.BodyImage:
		comp.Text(c, i18n.TrafficImageBody(b.Type, trafficSize(b.Size)), ds.Caption, p.TextInactive)
	case comp.BodyBinary:
		comp.Text(c, i18n.TrafficBinaryBody(trafficSize(b.Size), b.Type), ds.Caption, p.TextInactive)
		opts.Copy = i18n.CopyValue()
		a.kit.CodeView(c, memo.text, opts)
	default:
		opts.Copy = i18n.CopyValue()
		a.kit.CodeView(c, memo.text, opts)
	}
}

// memoFor is one tab's text of the flow in the detail pane, made the first
// time it is shown and kept until another flow is loaded.
func (st *trafficState) memoFor(side detailSide, tab int) detailMemo {
	key := memoKey{id: st.detailID, side: side, tab: tab}
	if m, ok := st.memo[key]; ok {
		return m
	}
	m := buildMemo(st.detail, side, tab)
	if st.memo == nil {
		st.memo = map[memoKey]detailMemo{}
	}
	st.memo[key] = m
	return m
}

// buildMemo makes one tab's text of a flow.
func buildMemo(f tool.Flow, side detailSide, tab int) detailMemo {
	switch tab {
	case tabBody:
		text, kind := comp.FormatBody(messageOf(f, side).Body)
		return detailMemo{text: text, kind: kind}
	case tabRaw:
		return detailMemo{text: rawText(f, side)}
	case tabQuery:
		return detailMemo{pairs: queryPairs(f.URL)}
	}
	return detailMemo{}
}

// rawText is a message as it went over the wire: its first line, its
// header lines, a blank line, and its body.
func rawText(f tool.Flow, side detailSide) string {
	var b strings.Builder
	if side == sideRequest {
		b.WriteString(strings.Join([]string{f.Method, f.Path, f.Proto}, " "))
	} else {
		b.WriteString(statusLine(f))
	}
	m := messageOf(f, side)
	for _, h := range m.Headers {
		b.WriteString("\n" + h.Name + ": " + h.Value)
	}
	b.WriteString("\n\n")
	b.Write(m.Body.Data)
	return b.String()
}

// statusLine is a response's first line: its protocol, status and reason.
func statusLine(f tool.Flow) string {
	if f.Status == 0 {
		return f.Proto
	}
	return strings.TrimSpace(f.Proto + " " + strconv.Itoa(f.Status) + " " + f.Info)
}

// queryPairs are a URL's query parameters in the order they were sent, each
// name and value unescaped where it can be.
func queryPairs(rawURL string) []tool.Header {
	u, err := url.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return nil
	}
	var pairs []tool.Header
	for _, part := range strings.Split(u.RawQuery, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		pairs = append(pairs, tool.Header{Name: unescape(name), Value: unescape(value)})
	}
	return pairs
}

// unescape decodes a query component, or returns it as it is when it does
// not decode.
func unescape(s string) string {
	if v, err := url.QueryUnescape(s); err == nil {
		return v
	}
	return s
}
