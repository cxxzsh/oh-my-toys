package shell

import (
	"context"
	"slices"
	"strconv"
	"time"

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
	cellType
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
			{label: i18n.TrafficColID, width: trafficWidthID, kind: cellID},
			{label: i18n.TrafficColTime, width: trafficWidthTime, kind: cellTime},
			{label: i18n.TrafficColProtocol, width: trafficWidthProtocol, kind: cellProtocol},
			{label: i18n.TrafficColDestination, share: trafficShareDestination, kind: cellDestination},
			{label: i18n.TrafficColInfo, share: trafficShareInfo, kind: cellInfo},
			{label: i18n.TrafficColLength, width: trafficWidthLength, end: true, kind: cellLength},
		}, width)
	}
	return trafficFit([]trafficColumn{
		{label: i18n.TrafficColID, width: trafficWidthID, kind: cellID},
		{label: i18n.TrafficColProgram, width: trafficWidthProgram, kind: cellProgram},
		{label: i18n.TrafficColTime, width: trafficWidthTime, kind: cellTime},
		{label: i18n.TrafficColMethod, width: trafficWidthMethod, kind: cellMethod},
		{label: i18n.TrafficColURL, share: trafficShareURL, kind: cellURL},
		{label: i18n.TrafficColType, width: trafficWidthType, kind: cellType},
		{label: i18n.StatusLabel, width: trafficWidthStatus, kind: cellStatus},
		{label: i18n.TrafficColSize, width: trafficWidthSize, end: true, kind: cellSize},
		{label: i18n.TrafficColDuration, width: trafficWidthDuration, end: true, kind: cellDuration},
	}, width)
}

// trafficListWidth is the width the list gets: its share of the panes beside
// the detail, or all of them when the detail sits under the list.
func (a *App) trafficListWidth(c *ui.Context, st *trafficState) float32 {
	panes := a.trafficPanesWidth(c)
	if panes >= ds.TrafficWide {
		return panes * st.splitP / percentScale
	}
	return panes
}

// trafficFit drops the columns that do not fit in width, in the order of
// trafficDropOrder. The Program, the URL and the Status are never dropped.
func trafficFit(cols []trafficColumn, width float32) []trafficColumn {
	for _, kind := range trafficDropOrder {
		if trafficNeed(cols) <= width {
			break
		}
		cols = slices.DeleteFunc(cols, func(col trafficColumn) bool { return col.kind == kind })
	}
	return cols
}

// trafficNeed is the width the columns ask for: their fixed widths, a minimum
// for each flexible column, and the gap and padding that go with each.
func trafficNeed(cols []trafficColumn) float32 {
	need := 2 * float32(ds.TableCellPad)
	for _, col := range cols {
		if col.width > 0 {
			need += col.width
		} else {
			need += trafficMinFlex
		}
		need += float32(ds.Space4)
	}
	return need
}

// Widths of the fixed columns, in pixels. The URL takes what is left after
// them, and the Program keeps a whole program name.
const (
	trafficWidthID       = 50
	trafficWidthProgram  = 110
	trafficWidthTime     = 90
	trafficWidthMethod   = 70
	trafficWidthType     = 70
	trafficWidthStatus   = 70
	trafficWidthSize     = 90
	trafficWidthDuration = 80
	trafficWidthProtocol = 72
	trafficWidthLength   = 88
)

// Shares of the flexible columns. The URL is the only flexible column of the
// proxy list; the packet capture splits its room between two columns.
const (
	trafficShareURL         = 1.0
	trafficShareDestination = 1.35
	trafficShareInfo        = 1.65
)

// trafficMinFlex is the least room a flexible column needs before a column is
// dropped to make space for it.
const trafficMinFlex = 120

// percentScale turns the list's percent split into a fraction of the panes.
const percentScale = 100

// trafficDropOrder lists the columns dropped first when the list is narrow.
var trafficDropOrder = []int{cellType, cellTime}

// trafficListPane is the list with its column headings.
func (a *App) trafficListPane(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Column(c).FillHeight().FillWidth().Gap(ds.Space4).Children(func() {
		a.trafficListBar(c, t, st)
		a.trafficColumnHead(c, st)
		a.trafficList(c, t, st)
	})
}

// trafficListBar is the strip above the list: the count of flows on the left,
// and Clear and Export HAR on the right. The toolbar keeps only the capture
// controls, so a long label cannot push Settings off the window.
func (a *App) trafficListBar(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		a.trafficCount(c, st)
		ui.Row(c).Grow(1)
		a.trafficClearExport(c, t, st)
	})
}

// trafficColumnHead is the row of labels above the list.
func (a *App) trafficColumnHead(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.ControlSm).Padding(0, ds.TableCellPad).Gap(ds.Space4).AlignItems(ui.Center).
		BorderWidth(0, 0, ds.LineWidth, 0).BorderColor(p.LineHairline).Children(func() {
		for i, col := range st.trafficColumns(a.trafficListWidth(c, st)) {
			col.frame(c, i).Children(func() {
				comp.Text(c, col.label(), ds.Eyebrow, p.TextMuted).SingleLine().Ellipsis("…")
			})
		}
	})
}

// trafficList is the virtual list of flows: only the rows in view are
// built, so a log of fifty thousand flows costs a frame no more than a
// log of twenty.
func (a *App) trafficList(c *ui.Context, t tool.Traffic, st *trafficState) {
	if len(st.rows) == 0 {
		a.trafficListEmpty(c, t, st)
		return
	}
	st.list.Selected = &st.sel
	st.list.Key = func(i int) any { return st.rows[i].ID }
	st.list.Label = func(i int) string { return trafficRowLabel(st.rows[i]) }
	st.list.FollowEnd = st.follow
	cols := st.trafficColumns(a.trafficListWidth(c, st))
	ui.List(c.Key("flows"), &st.list, len(st.rows), func(i int) { a.trafficRow(c, t, st, cols, i) }).
		Grow(1).FillWidth()
	// Scrolling up is how a person stops following the end; the list
	// reports it once it has settled.
	if st.follow && st.frames > 2 && !st.list.AtEnd() {
		st.follow = false
	}
}

// trafficRow is one flow, the chosen one in the design system's accent.
func (a *App) trafficRow(c *ui.Context, t tool.Traffic, st *trafficState, cols []trafficColumn, i int) {
	p := ds.P(c)
	r := st.rows[i]
	chosen := i == st.sel
	b := ui.ButtonBase(c.Key(r.ID)).Label(trafficRowLabel(r)).Cursor(ui.CursorDefault)
	if b.Clicked() {
		st.sel = i
	}
	b.ContextMenu(func(m *ui.Menu) { a.trafficFlowMenu(m, c, t, st, r) })
	b.FillWidth().Height(ds.TableRow).Padding(0, ds.TableCellPad).Gap(ds.Space4).Background(p.BgCard).
		AlignItems(ui.Center)
	if chosen {
		b.BorderWidth(0, 0, 0, ds.FocusRingWidth).BorderColor(p.AccentStrong)
	} else if b.Hovered() {
		b.Background(p.FillControl)
	}
	b.Children(func() {
		if chosen {
			listSelection(c)
		}
		for j, col := range cols {
			col.frame(c, j).Children(func() { a.trafficCell(c, r, col.kind) })
		}
	})
}

// trafficCell draws one cell of a flow.
func (a *App) trafficCell(c *ui.Context, r tool.FlowRow, kind int) {
	p := ds.P(c)
	switch kind {
	case cellID:
		comp.Value(c, strconv.FormatUint(r.ID, 10), p.TextMuted).Ellipsis("…")
	case cellTime:
		comp.Value(c, r.Started.Format(time.TimeOnly), p.TextMuted).Ellipsis("…")
	case cellMethod:
		comp.Value(c, r.Method, p.TextPrimary).Ellipsis("…")
	case cellURL:
		trafficURL(c, r)
	case cellStatus:
		a.trafficStatus(c, r)
	case cellType:
		comp.Value(c, shortType(r.Type), p.TextSecondary).Ellipsis("…").Shrink(1)
	case cellSize:
		comp.Value(c, trafficSize(r.Size), p.TextSecondary).Ellipsis("…")
	case cellDuration:
		comp.Value(c, trafficTook(r.Duration), p.TextMuted).Ellipsis("…")
	case cellProtocol:
		comp.Value(c, r.Proto, p.TextSecondary).Ellipsis("…")
	case cellDestination:
		comp.Value(c, r.Host, p.TextPrimary).Ellipsis("…").Shrink(1).MinWidth(0)
	case cellInfo:
		comp.Value(c, r.Info, p.TextSecondary).Ellipsis("…").Shrink(1).MinWidth(0)
	case cellLength:
		comp.Value(c, trafficSize(r.Size), p.TextMuted).Ellipsis("…")
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

// trafficStatus is the state of a flow: its status in the colour of what
// it means, or the word for a failure, a block or a response still on its
// way. A rule that changed the flow is marked before it.
func (a *App) trafficStatus(c *ui.Context, r tool.FlowRow) {
	if r.Ruled {
		a.kit.Glyph(c, "tune", ds.GlyphSm, ds.P(c).TextMuted)
	}
	switch r.State {
	case tool.FlowBlocked:
		comp.Badge(c, i18n.TrafficBlocked(), comp.ToneDanger)
	case tool.FlowFailed:
		comp.Badge(c, i18n.TrafficFailed(), comp.ToneDanger)
	case tool.FlowDone:
		comp.Badge(c, strconv.Itoa(r.Status), statusTone(r.Status))
	case tool.FlowOpen:
		if r.Status > 0 {
			comp.Badge(c, strconv.Itoa(r.Status), statusTone(r.Status))
			return
		}
		comp.Badge(c, i18n.TrafficOpen(), comp.ToneAccent)
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
