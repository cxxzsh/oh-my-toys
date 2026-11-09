package shell

import (
	"context"
	"runtime"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// The cells a row of the list can show.
const (
	cellID = iota
	cellTime
	cellMethod
	cellURL
	cellStatus
	cellSize
	cellDuration
	cellProtocol
	cellDestination
	cellInfo
	cellLength
	cellProgram
)

// trafficColumn is one column of the list: its heading, its width, and the
// cell it draws. A column with a width keeps that many pixels; the others
// share what is left, in proportion to their share.
type trafficColumn struct {
	label func() string
	width float32
	share float32
	end   bool
	kind  int
}

// frame is the box a column's cell sits in. The heading and every row use the
// same box, so the labels line up with the values under them.
func (col trafficColumn) frame(c *ui.Context, key int) ui.Element {
	cell := ui.Row(c.Key(key)).AlignItems(ui.Center)
	if col.width > 0 {
		cell.Width(col.width)
	} else {
		cell.Grow(col.share).Basis(0).MinWidth(0)
	}
	if col.end {
		cell.Justify(ui.End)
	}
	return cell
}

// trafficColumns are the columns of the mode in progress, cut down to what
// fits a list of the given width: whole requests and responses for the
// proxy, one packet a row for the packet capture.
func (st *trafficState) trafficColumns(width float32) []trafficColumn {
	if tool.CaptureMode(st.mode) == tool.ModePackets {
		return trafficFit([]trafficColumn{
			{label: i18n.TrafficColID, width: ds.TrafficColID, kind: cellID},
			{label: i18n.TrafficColTime, width: ds.TrafficColTime, kind: cellTime},
			{label: i18n.TrafficColProtocol, width: ds.TrafficColProtocol, kind: cellProtocol},
			{label: i18n.TrafficColDestination, share: trafficShareDestination, kind: cellDestination},
			{label: i18n.TrafficColInfo, share: trafficShareInfo, kind: cellInfo},
			{label: i18n.TrafficColLength, width: ds.TrafficColLength, end: true, kind: cellLength},
		}, width)
	}
	return trafficFit([]trafficColumn{
		{label: i18n.TrafficColID, width: ds.TrafficColID, kind: cellID},
		{label: i18n.TrafficColMethod, width: ds.TrafficColMethod, kind: cellMethod},
		{label: i18n.StatusLabel, width: ds.TrafficColStatus, kind: cellStatus},
		{label: i18n.TrafficColURL, share: trafficShareURL, kind: cellURL},
		{label: i18n.TrafficColProgram, width: ds.TrafficColProgram, kind: cellProgram},
		{label: i18n.TrafficColTime, width: ds.TrafficColTime, kind: cellTime},
		{label: i18n.TrafficColDuration, width: ds.TrafficColDuration, end: true, kind: cellDuration},
		{label: i18n.TrafficColSize, width: ds.TrafficColSize, end: true, kind: cellSize},
	}, width)
}

// trafficListWidth is the width the list gets: its share of the panes beside
// the detail, or all of them when the detail sits under the list.
func (a *App) trafficListWidth(c *ui.Context, st *trafficState) float32 {
	panes := a.trafficPanesWidth(c, st)
	if panes >= ds.TrafficWide {
		return panes * st.splitP / percentScale
	}
	return panes
}

// trafficFit drops the columns that do not fit in width, in the order of
// trafficDropOrder, until the URL has its minimum. #, Method, Status and the
// URL are never dropped.
func trafficFit(cols []trafficColumn, width float32) []trafficColumn {
	for _, kind := range trafficDropOrder {
		if trafficNeed(cols) <= width {
			break
		}
		cols = slices.DeleteFunc(cols, func(col trafficColumn) bool { return col.kind == kind })
	}
	return cols
}

// trafficNeed is the width the columns ask for: their fixed widths, the URL's
// minimum for the flexible column, and the gap and padding that go with each.
func trafficNeed(cols []trafficColumn) float32 {
	need := 2 * float32(ds.TableCellPad)
	for _, col := range cols {
		if col.width > 0 {
			need += col.width
		} else {
			need += ds.TrafficURLMin
		}
		need += float32(ds.Space4)
	}
	return need
}

// Shares of the flexible columns. The URL is the only flexible column of the
// proxy list; the packet capture splits its room between two columns.
const (
	trafficShareURL         = 1.0
	trafficShareDestination = 1.35
	trafficShareInfo        = 1.65
)

// percentScale turns the list's percent split into a fraction of the panes.
const percentScale = 100

// trafficDropOrder lists the columns dropped first when the list is narrow,
// least needed first. #, Method, Status and the URL are never dropped.
var trafficDropOrder = []int{cellSize, cellTime, cellDuration, cellProgram}

// trafficListPane is the list with its column headings. The columns are
// worked out once here and shared by the headings and the rows.
func (a *App) trafficListPane(c *ui.Context, t tool.Traffic, st *trafficState) {
	cols := st.trafficColumns(a.trafficListWidth(c, st))
	ui.Column(c).FillHeight().FillWidth().Gap(ds.Space4).Children(func() {
		trafficColumnHead(c, cols)
		a.trafficList(c, t, st, cols)
		a.trafficBottom(c, t, st)
	})
}

// trafficColumnHead is the row of labels above the list, in the columns the
// list draws.
func trafficColumnHead(c *ui.Context, cols []trafficColumn) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.ControlSm).Padding(0, ds.TableCellPad).Gap(ds.Space4).AlignItems(ui.Center).
		BorderWidth(0, 0, ds.LineWidth, 0).BorderColor(p.LineHairline).Children(func() {
		for i, col := range cols {
			col.frame(c, i).Children(func() {
				comp.Text(c, col.label(), ds.Eyebrow, p.TextMuted).SingleLine().Ellipsis("…")
			})
		}
	})
}

// trafficList is the virtual list of flows: only the rows in view are
// built, so a log of fifty thousand flows costs a frame no more than a
// log of twenty.
func (a *App) trafficList(c *ui.Context, t tool.Traffic, st *trafficState, cols []trafficColumn) {
	if len(st.rows) == 0 {
		a.trafficListEmpty(c, t, st)
		return
	}
	st.list.Selected = &st.sel
	if st.list.Key == nil {
		st.list.Key = func(i int) any { return st.rows[i].ID }
		st.list.Label = func(i int) string { return trafficRowLabel(st.rows[i]) }
	}
	st.list.FollowEnd = st.follow
	lst := ui.List(c.Key("flows"), &st.list, len(st.rows), func(i int) { a.trafficRow(c, t, st, cols, i) }).
		Grow(1).FillWidth()
	st.listKeys(lst)
	if lst.Changed() {
		st.picked = true
		st.setSel(st.sel)
	}
	// Scrolling up is how a person stops following the end; the list
	// reports it once it has settled.
	if st.follow && st.frames > 2 && !st.list.AtEnd() {
		st.follow = false
	}
}

// trafficRow is one flow. The chosen one carries the design system's accent
// and selection fill; a failed one carries a red mark on its left edge.
func (a *App) trafficRow(c *ui.Context, t tool.Traffic, st *trafficState, cols []trafficColumn, i int) {
	p := ds.P(c)
	r := st.rows[i]
	txt := st.textOf(r)
	chosen := i == st.sel
	b := ui.ButtonBase(c.Key(r.ID)).Label(txt.label).Cursor(ui.CursorDefault)
	if b.Clicked() {
		st.choose(i)
	}
	b.ContextMenu(func(m *ui.Menu) { a.trafficFlowMenu(m, c, t, st, r) })
	b.FillWidth().Height(ds.TableRow).Padding(0, ds.TableCellPad).Gap(ds.Space4).Background(p.BgCard).
		AlignItems(ui.Center)
	// The marker's width is always kept, so a marked row does not shift its
	// text; only its colour changes.
	b.BorderWidth(0, 0, 0, ds.FocusRingWidth).BorderColor(ui.Transparent)
	switch {
	case chosen:
		b.BorderColor(p.AccentStrong)
	case r.State == tool.FlowFailed:
		b.BorderColor(p.Danger)
	}
	if !chosen && b.Hovered() {
		b.Background(p.FillControl)
	}
	b.Children(func() {
		if chosen {
			listSelection(c)
		}
		for j, col := range cols {
			col.frame(c, j).Children(func() { a.trafficCell(c, r, txt, col.kind) })
		}
	})
}

// trafficCell draws one cell of a flow. The text of the cell comes from txt,
// which is made once for the flow rather than on every frame.
func (a *App) trafficCell(c *ui.Context, r tool.FlowRow, txt rowText, kind int) {
	p := ds.P(c)
	switch kind {
	case cellID:
		comp.Value(c, txt.id, p.TextMuted).Ellipsis("…")
	case cellTime:
		comp.Value(c, txt.time, p.TextMuted).Ellipsis("…")
	case cellMethod:
		comp.Value(c, r.Method, methodColor(p, r.Method)).Ellipsis("…")
	case cellURL:
		trafficURL(c, r)
	case cellStatus:
		a.trafficStatus(c, r, txt.status)
	case cellSize:
		comp.Value(c, txt.size, p.TextSecondary).Ellipsis("…")
	case cellDuration:
		comp.Value(c, txt.took, p.TextMuted).Ellipsis("…")
	case cellProtocol:
		comp.Value(c, r.Proto, p.TextSecondary).Ellipsis("…")
	case cellDestination:
		comp.Value(c, r.Host, p.TextPrimary).Ellipsis("…").Shrink(1).MinWidth(0)
	case cellInfo:
		comp.Value(c, r.Info, p.TextSecondary).Ellipsis("…").Shrink(1).MinWidth(0)
	case cellLength:
		comp.Value(c, txt.size, p.TextMuted).Ellipsis("…")
	case cellProgram:
		comp.Value(c, r.Process.Name, p.TextSecondary).Ellipsis("…").Shrink(1).MinWidth(0)
	}
}

// trafficURL draws a request's host and its path as one line. The host keeps
// its colour; the path says less, so it is muted and gives way first.
func trafficURL(c *ui.Context, r tool.FlowRow) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Clip().AlignItems(ui.Center).Children(func() {
		comp.Value(c, r.Host, p.TextPrimary).Ellipsis("…").Shrink(1).MinWidth(0)
		comp.Value(c, r.Path, p.TextMuted).Ellipsis("…").Shrink(2).MinWidth(0)
	})
}

// trafficStatus is the state of a flow: its status text in the colour of
// what it means, or the word for a failure, a block or a response still on
// its way. A rule that changed the flow is marked before it.
func (a *App) trafficStatus(c *ui.Context, r tool.FlowRow, text string) {
	if r.Ruled {
		a.kit.Glyph(c, "tune", ds.GlyphSm, ds.P(c).TextMuted)
	}
	switch r.State {
	case tool.FlowBlocked, tool.FlowFailed:
		comp.Badge(c, text, comp.ToneDanger)
	case tool.FlowDone:
		comp.Badge(c, text, statusTone(r.Status))
	case tool.FlowOpen:
		if r.Status > 0 {
			comp.Badge(c, text, statusTone(r.Status))
			return
		}
		comp.Badge(c, text, comp.ToneAccent)
	}
}

// trafficListEmpty says that nothing is there, and what to do next.
func (a *App) trafficListEmpty(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	comp.Card(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).Grow(1).FillWidth().Justify(ui.Center).AlignItems(ui.Center).Gap(ds.Space6).
			Padding(ds.Space20).Children(func() {
			comp.Text(c, st.emptyTitle(), ds.Headline, p.TextStrong)
			comp.Text(c, st.emptyNext(t), ds.Caption, p.TextInactive).TextAlign(ui.Center)
			if st.filterActive() {
				if a.kit.Button(c.Key("empty-clear"), i18n.TrafficClearFilters(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
					st.clearFilter()
				}
			}
		})
	})
}

// emptyTitle says why the list has nothing.
func (st *trafficState) emptyTitle() string {
	if st.filterActive() {
		return i18n.TrafficEmptyFiltered()
	}
	return i18n.TrafficEmpty()
}

// emptyNext says what to do about it.
func (st *trafficState) emptyNext(t tool.Traffic) string {
	switch {
	case st.filterActive():
		return ""
	case t.Config().Mode == tool.ModePackets:
		return i18n.TrafficEmptyPackets()
	}
	return i18n.TrafficEmptyProxy()
}

// trafficReplay sends a request again and shows the new flow, off the UI
// thread: the capture's rules and the network are in the way.
func (a *App) trafficReplay(t tool.Traffic, st *trafficState, id uint64) {
	if t == nil || a.trafficBusy(st) {
		return
	}
	st.begin()
	st.err = nil
	a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficTimeout, func(ctx context.Context) {
		newID, err := t.Replay(ctx, id)
		a.tell(st, trafficOutcome{err: err, replay: newID})
	})
}

// methodColor is the colour of a request's method: the common verbs read at
// a glance, and the others stay in the text colour.
func methodColor(p *ds.Palette, method string) ui.Color {
	switch method {
	case "GET":
		return p.Success
	case "POST":
		return p.AccentText
	case "PUT":
		return p.Warning
	case "DELETE":
		return p.Danger
	}
	return p.TextPrimary
}

// choose makes row i the choice, brings it into view, and marks it as picked
// by hand, so that Auto select stops taking the choice away.
func (st *trafficState) choose(i int) {
	i = max(0, min(i, len(st.rows)-1))
	st.setSel(i)
	st.picked = true
	st.list.ScrollIntoView(i)
}

// listKeys handles the keys the list leaves to the page: Esc clears the
// choice. On macOS the list scrolls by a page on PageUp and PageDown instead
// of moving the choice, so the page moves it here.
func (st *trafficState) listKeys(lst ui.Element) {
	switch {
	case lst.Shortcut(0, ui.KeyEscape):
		st.setSel(-1)
		st.picked = false
	case runtime.GOOS == "darwin" && lst.Shortcut(0, ui.KeyPageDown):
		st.choose(max(0, st.sel) + st.pageRows())
	case runtime.GOOS == "darwin" && lst.Shortcut(0, ui.KeyPageUp):
		st.choose(st.sel - st.pageRows())
	}
}

// pageRows is how many rows a page key moves the choice: the rows in view.
func (st *trafficState) pageRows() int {
	first, last := st.list.Visible()
	return max(1, last-first)
}

// selectNewest chooses the newest flow as it arrives, while Auto select is on
// and no row was picked by hand. Each flow is chosen once, so an Esc that
// clears the choice stays cleared until the next flow arrives.
func (st *trafficState) selectNewest() {
	if !st.autoSelect || st.picked || len(st.rows) == 0 {
		return
	}
	last := len(st.rows) - 1
	if st.rows[last].ID == st.autoID {
		return
	}
	st.autoID = st.rows[last].ID
	st.setSel(last)
}
