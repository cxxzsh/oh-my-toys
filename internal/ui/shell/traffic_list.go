package shell

import (
	"context"
	"strconv"
	"time"

	"github.com/egoist/mygo"
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
	cellHost
	cellPath
	cellStatus
	cellType
	cellSize
	cellDuration
	cellProtocol
	cellDestination
	cellInfo
	cellLength
)

// trafficColumn is one column of the list: its heading, the share of the
// width it takes, and the cell it draws.
type trafficColumn struct {
	label func() string
	share float32
	end   bool
	kind  int
}

// trafficColumns are the columns of the mode in progress: whole requests
// and responses for the proxy, one packet a row for the packet capture.
func (st *trafficState) trafficColumns() []trafficColumn {
	if tool.CaptureMode(st.mode) == tool.ModePackets {
		return []trafficColumn{
			{label: i18n.TrafficColID, share: trafficShareID, kind: cellID},
			{label: i18n.TrafficColTime, share: trafficShareTime, kind: cellTime},
			{label: i18n.TrafficColProtocol, share: trafficShareMethod, kind: cellProtocol},
			{label: i18n.TrafficColDestination, share: trafficShareHost, kind: cellDestination},
			{label: i18n.TrafficColInfo, share: trafficSharePath, kind: cellInfo},
			{label: i18n.TrafficColLength, share: trafficShareSize, end: true, kind: cellLength},
		}
	}
	return []trafficColumn{
		{label: i18n.TrafficColID, share: trafficShareID, kind: cellID},
		{label: i18n.TrafficColTime, share: trafficShareTime, kind: cellTime},
		{label: i18n.TrafficColMethod, share: trafficShareMethod, kind: cellMethod},
		{label: i18n.TrafficColHost, share: trafficShareHost, kind: cellHost},
		{label: i18n.TrafficColPath, share: trafficSharePath, kind: cellPath},
		{label: i18n.StatusLabel, share: trafficShareStatus, kind: cellStatus},
		{label: i18n.TrafficColType, share: trafficShareType, kind: cellType},
		{label: i18n.TrafficColSize, share: trafficShareSize, end: true, kind: cellSize},
		{label: i18n.TrafficColDuration, share: trafficShareDuration, end: true, kind: cellDuration},
	}
}

// How the list's width is shared out: the path and the host give way
// first, the number and the time keep their room.
const (
	trafficShareID       = 0.55
	trafficShareTime     = 1.15
	trafficShareMethod   = 1.15
	trafficShareHost     = 1.35
	trafficSharePath     = 1.65
	trafficShareStatus   = 1.45
	trafficShareType     = 0.8
	trafficShareSize     = 0.95
	trafficShareDuration = 0.85
)

// trafficListPane is the list with its column headings.
func (a *App) trafficListPane(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Column(c).FillHeight().FillWidth().Gap(ds.Space4).Children(func() {
		a.trafficColumnHead(c, st)
		a.trafficList(c, t, st)
	})
}

// trafficColumnHead is the row of labels above the list.
func (a *App) trafficColumnHead(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.ControlSm).Padding(0, ds.TableCellPad).Gap(ds.Space4).AlignItems(ui.Center).
		BorderWidth(0, 0, ds.LineWidth, 0).BorderColor(p.LineHairline).Children(func() {
		for i, col := range st.trafficColumns() {
			cell := ui.Row(c.Key(i)).Grow(col.share).Basis(0).MinWidth(0)
			if col.end {
				cell.Justify(ui.End)
			}
			cell.Children(func() {
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
	cols := st.trafficColumns()
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
	b.ContextMenu(func(m *ui.Menu) { a.trafficRowMenu(m, c, t, st, r) })
	b.FillWidth().Height(ds.TableRow).Padding(0, ds.TableCellPad).Gap(ds.Space4).AlignItems(ui.Center)
	switch {
	case chosen:
		b.Background(p.FillControl).BorderWidth(0, 0, 0, ds.FocusRingWidth).BorderColor(p.AccentStrong)
	case b.Hovered():
		b.Background(p.FillControl)
	}
	b.Children(func() {
		for j, col := range cols {
			cell := ui.Row(c.Key(j)).Grow(col.share).Basis(0).MinWidth(0).AlignItems(ui.Center)
			if col.end {
				cell.Justify(ui.End)
			}
			cell.Children(func() { a.trafficCell(c, r, col.kind) })
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
	case cellHost:
		comp.Value(c, r.Host, p.TextSecondary).Ellipsis("…").Shrink(1).MinWidth(0)
	case cellPath:
		comp.Value(c, r.Path, p.TextPrimary).Ellipsis("…").Shrink(1).MinWidth(0)
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
	}
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

// trafficRowMenu is what a person can do with one flow besides looking at
// it.
func (a *App) trafficRowMenu(m *ui.Menu, c *ui.Context, t tool.Traffic, st *trafficState, r tool.FlowRow) {
	if r.Source == tool.SourceProxy && m.Item(i18n.TrafficReplay()).Chosen() {
		a.trafficReplay(t, st, r.ID)
	}
	url := st.urlOf(r.ID)
	if m.Item(i18n.TrafficCopyURL()).Disabled(url == "").Chosen() {
		mygo.Clipboard.WriteText(url)
		c.Toast(i18n.Copied(url))
	}
	if m.Item(i18n.TrafficCopyRow()).Chosen() {
		mygo.Clipboard.WriteText(trafficRowText(r))
		c.Toast(i18n.Copied(trafficRowLabel(r)))
	}
	m.Separator()
	if m.Item(i18n.TrafficFilterHost()).Disabled(r.Host == "").Chosen() {
		st.text = r.Host
	}
	m.Separator()
	a.flowRuleMenu(m, t, st, r)
}

// urlOf returns a flow's whole URL, asking the log when the detail has not
// been read yet.
func (st *trafficState) urlOf(id uint64) string {
	if st.hasDetail && st.detailID == id && st.detail.URL != "" {
		return st.detail.URL
	}
	if st.log == nil {
		return ""
	}
	if f, ok := st.log.Get(id); ok {
		return f.URL
	}
	return ""
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
