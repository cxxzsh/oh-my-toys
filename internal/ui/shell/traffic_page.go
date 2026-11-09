package shell

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// How long the inspector waits for a traffic tool. Starting and stopping a
// capture is quick; installing a certificate waits for a person to type a
// password.
const (
	trafficTimeout       = 30 * time.Second
	trafficPromptTimeout = 5 * time.Minute
	trafficCertAge       = 10 * time.Second
)

// The ports a capture proxy may listen on: below 1024 the system asks for
// an administrator's password.
const (
	trafficPortMin = 1024
	trafficPortMax = 65535
)

// glyphPower is the glyph of the button that starts and stops a capture.
const glyphPower = "power_settings_new"

// glyphSourcesOpen is the glyph that shows the sources, and
// glyphSourcesClose the glyph that hides them.
const (
	glyphSourcesOpen  = "left_panel_open"
	glyphSourcesClose = "left_panel_close"
)

// listSplitPercent is the flow list's share of the width beside the detail.
// The list carries more columns than the detail, so it takes the larger
// share; both panes grow with the window.
const listSplitPercent = 58

// listSplitVPercent is the flow list's share of the height above the detail
// when the two stack; the detail takes the rest.
const listSplitVPercent = 40

// trafficKey names the inspector's state on its page element.
type trafficKey struct{}

// trafficState is what the inspector keeps from frame to frame. It lives
// on the page element, so it survives a redraw, and starts empty again
// after the app restarts.
type trafficState struct {
	started bool
	stop    func() // ends the log subscription

	rows    []tool.FlowRow // the snapshot of the log the list shows
	log     tool.FlowLog   // the log the rows came from: one per mode
	ver     uint64         // the log version rows were read at
	filter  tool.Filter    // the filter rows were read with
	fetched bool           // rows have been read at least once
	frames  int            // frames drawn, to tell a list that has settled
	pending uint64         // a flow to select once the rows have it
	saved   string         // a HAR file saved, to say so once

	trafficFilterState

	view       int // the flows, or the rule book
	sources    int // the sources column: by width, or shown or hidden by a person
	total      int // the flows the log holds, before any filter narrows them
	rules      ruleView
	sel        int    // the chosen row, -1 for none
	selID      uint64 // the chosen flow; sel is its row
	list       ui.ListState
	follow     bool
	splitP     float32 // the list's share of the width beside the detail, percent
	rulesSplit float32 // the rule list's width beside its form
	splitV     float32 // the list's share of the height above the detail, percent
	detailTab  int
	paneTab    [2]int // the tab each side of a flow last showed, indexed by detailSide; zero until picked
	memo       map[memoKey]detailMemo
	detail     tool.Flow
	detailID   uint64
	detailVer  uint64
	hasDetail  bool

	port        string
	bypass      string
	upstream    string
	systemProxy bool
	decrypt     bool
	mode        int
	portBad     bool
	upBad       bool
	cfgDirty    bool
	cfgDue      time.Time
	iface       string

	ifaces       []tool.NetInterface
	ifacesLoaded bool

	cert      tool.CertState
	certKnown bool
	certAt    time.Time

	// The work in flight, and what it reported: a capture, a certificate,
	// an interface list or a replay runs off the UI thread, so it tells the
	// page through here rather than touching it.
	mu       sync.Mutex
	inFlight int
	outcomes []trafficOutcome

	err error // the last failure, which the banner explains

	// settings is whether the Settings dialog is open.
	settings bool

	// procs and hosts are the sidebar's facets, read again only when the log changes.
	procs    []tool.Facet
	hosts    []tool.Facet
	facetVer uint64
	facetSet bool

	// procSel and hostSel are the program and host the list narrows to; empty means all.
	procSel string
	hostSel string

	// Auto select and the row text cache: see selectNewest and textOf.
	autoSelect bool
	picked     bool
	autoID     uint64
	texts      map[uint64]rowText
}

// trafficOutcome is what one piece of background work reported.
type trafficOutcome struct {
	err       error
	sent      []tool.Rule // the rule book the tool was given
	cert      tool.CertState
	hasCert   bool
	ifaces    []tool.NetInterface
	hasIfaces bool
	replay    uint64
	saved     string
}

// begin marks one piece of work in flight, which holds the controls back.
func (st *trafficState) begin() {
	st.mu.Lock()
	st.inFlight++
	st.mu.Unlock()
}

// tell hands what background work found to the page, and asks for a frame.
func (a *App) tell(st *trafficState, o trafficOutcome) {
	st.mu.Lock()
	st.outcomes = append(st.outcomes, o)
	st.mu.Unlock()
	a.Post(nil)
}

// drain applies what background work found, on the UI thread.
func (a *App) drain(st *trafficState) {
	st.mu.Lock()
	outcomes := st.outcomes
	st.outcomes, st.inFlight = nil, max(0, st.inFlight-len(outcomes))
	st.mu.Unlock()
	for _, o := range outcomes {
		st.apply(o)
	}
}

// apply is one outcome, in the order it arrived.
func (st *trafficState) apply(o trafficOutcome) {
	if o.sent != nil {
		st.rulesDone(o)
		return
	}
	if o.hasCert {
		st.cert, st.certKnown, st.certAt = o.cert, true, time.Now()
	}
	if o.hasIfaces {
		st.ifaces, st.ifacesLoaded = o.ifaces, true
	}
	if o.saved != "" {
		st.saved = o.saved
	}
	if o.replay != 0 {
		st.pending = o.replay
	}
	if o.err == nil && o.hasCert {
		return
	}
	st.record(o.err)
}

// newTrafficState is the state a page starts with: the list following its
// end, no row chosen, and the split at the design system's sizes.
func newTrafficState() trafficState {
	return trafficState{
		sel: -1, follow: true, autoSelect: true,
		splitP: listSplitPercent, splitV: listSplitVPercent,
		rulesSplit: ds.TrafficRulesList,
	}
}

// trafficPage draws the inspector of any tool that captures traffic. The
// page names no tool: everything it shows comes from the contract.
func (a *App) trafficPage(c *ui.Context, t tool.Traffic) {
	id := t.Manifest().ID
	page := ui.Column(c.Key("traffic:" + id)).Grow(1).FillWidth().Padding(ds.Space16).Gap(ds.Space12)
	st := ui.Local(page, trafficKey{}, newTrafficState)
	a.drain(st)
	a.trafficBegin(t, st)
	a.trafficLog(t, st)
	a.rulesDue(c, t, st)
	st.sync()
	st.refreshFacets()
	page.Children(func() {
		if st.saved != "" {
			c.Toast(i18n.TrafficExported(st.saved))
			st.saved = ""
		}
		a.trafficToolbar(c, t, st)
		if !st.settings {
			a.trafficProblem(c, st)
		}
		a.trafficSettings(c, t, st)
		if st.view == viewRules {
			a.rulesView(c, t, st)
			return
		}
		a.trafficFilterBar(c, st)
		a.trafficWorkspace(c, t, st)
	})
}

// trafficBegin reads the tool's settings and its certificate, once for
// the life of the page.
func (a *App) trafficBegin(t tool.Traffic, st *trafficState) {
	if st.started {
		return
	}
	st.started = true
	st.loadConfig(t.Config())
	a.loadRules(t, st)
	a.trafficLoadCert(context.Background(), t, st)
}

// trafficLog follows the log of the mode in progress: each mode has a log
// of its own, so a change of mode ends the old subscription, drops what
// was read from it and subscribes to the new one.
func (a *App) trafficLog(t tool.Traffic, st *trafficState) {
	log := t.Log()
	if sameLog(log, st.log) {
		return
	}
	if st.stop != nil {
		st.stop()
		st.stop = nil
	}
	st.log = log
	st.rows, st.fetched = nil, false
	st.sel, st.selID = -1, 0
	st.detailID, st.detail, st.hasDetail = 0, tool.Flow{}, false
	st.list = ui.ListState{}
	st.picked, st.autoID = false, 0
	clear(st.texts)
	a.trafficWatch(t, st, log)
}

// trafficWatch redraws the page when the log changes. The log calls this
// off the UI thread, so the page check runs on the main thread, and a page
// that has been left costs nothing.
func (a *App) trafficWatch(t tool.Traffic, st *trafficState, log tool.FlowLog) {
	id := t.Manifest().ID
	st.stop = log.OnChange(func() {
		if a.main == nil {
			return
		}
		a.main.Update(func() {
			if a.page == id {
				a.redraw()
			}
		})
	})
}

// sameLog reports whether two handles are the same log. Logs are pointers
// in practice; one that cannot be compared is never taken as the same, so
// the page reads it again rather than showing another log's rows.
func sameLog(a, b tool.FlowLog) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if !reflect.TypeOf(a).Comparable() {
		return false
	}
	return a == b
}

// trafficNote is what the capture asks the person to know beyond its
// state: a system proxy that apps will not use, for one.
func (a *App) trafficNote(c *ui.Context, t tool.Traffic) {
	if note := t.Status().Note; note != "" {
		comp.Text(c, note, ds.Caption, ds.P(c).TextInactive).Selectable()
	}
}

// trafficBusy reports whether the tool is doing something long enough to
// hold a control back.
func (a *App) trafficBusy(st *trafficState) bool {
	return st.inFlight > 0
}

// trafficSetOn starts or stops the capture off the UI thread: a capture
// waits on the system's prompts and sockets, which must not hold a frame.
func (a *App) trafficSetOn(t tool.Traffic, st *trafficState, on bool) {
	st.begin()
	st.err = nil
	a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficTimeout, func(ctx context.Context) {
		var err error
		if on {
			err = t.Start(ctx)
		} else {
			err = t.Stop()
		}
		a.tell(st, trafficOutcome{err: err})
	})
}

// record keeps a failure for the banner and the log, or clears it when the
// action that failed has been retried successfully.
func (st *trafficState) record(err error) {
	st.err = err
	if err != nil {
		log.Printf("traffic: %v", err)
	}
}

// trafficExport saves the flows the list shows as a HAR file, chosen in
// the system's own save dialog.
func (a *App) trafficExport(t tool.Traffic, st *trafficState) {
	ids := make([]uint64, len(st.rows))
	for i, r := range st.rows {
		ids[i] = r.ID
	}
	name := trafficFileName(time.Now())
	st.begin()
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      a.main,
			Title:       i18n.TrafficExport(),
			DefaultPath: filepath.Join(homeDir(), name),
			Filters:     []mygo.FileFilter{{Name: "HAR", Extensions: []string{"har"}}},
		})
		if err != nil || path == "" {
			a.tell(st, trafficOutcome{})
			return
		}
		a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficTimeout, func(context.Context) {
			err := writeHAR(t, path, ids)
			a.tell(st, trafficOutcome{err: err, saved: path})
		})
	}()
}

// writeHAR writes the flows with the given ids to path.
func writeHAR(t tool.Traffic, path string, ids []uint64) error {
	f, err := os.Create(path) //nolint:gosec // the path is the person's choice in the save dialog
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := t.ExportHAR(f, ids); err != nil {
		_ = f.Close()
		return fmt.Errorf("export to %s: %w", path, err)
	}
	return f.Close()
}

// trafficFileName names an export after the moment it was made.
func trafficFileName(now time.Time) string {
	return "traffic-" + now.Format("20060102-150405") + ".har"
}

// trafficPanes are the list and the detail, side by side when the window
// is wide and one above the other when it is not.
func (a *App) trafficPanes(c *ui.Context, t tool.Traffic, st *trafficState) {
	list := func() {
		if trafficGuideDue(t, st) {
			a.trafficGuide(c, t, st)
			return
		}
		a.trafficListPane(c, t, st)
	}
	detail := func() { a.trafficDetailPane(c, t, st) }
	if len(st.rows) == 0 && !st.hasDetail {
		// With no flow to show, the empty detail box would only repeat the
		// empty list: the list or the guide takes the whole pane instead.
		ui.Column(c).Grow(1).FillWidth().FillHeight().Children(list)
		return
	}
	if w := a.trafficPanesWidth(c, st); w >= ds.TrafficWide {
		comp.SplitPercent(c, &st.splitP, list, detail).Grow(1).FillWidth()
		return
	}
	ui.SplitVerticalPercent(c, &st.splitV, list, detail).Grow(1).FillWidth()
}

// The states of the sources column: by width until a person chooses, then
// shown or hidden as chosen.
const (
	sourcesAuto   = iota // open on a wide page
	sourcesOpen          // shown by a person
	sourcesClosed        // hidden by a person
)

// trafficWorkspace places the sources column beside the list and the detail
// when it is shown. The column keeps its fixed width; the panes take the rest.
func (a *App) trafficWorkspace(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Row(c).Grow(1).FillWidth().Gap(ds.Space12).AlignItems(ui.Stretch).Children(func() {
		if a.trafficSourcesShown(c, st) {
			a.trafficSidebar(c, t, st)
		}
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
			a.trafficPanes(c, t, st)
		})
	})
}

// trafficPanesWidth is the width the list and the detail share: the window
// less the app sidebar, and less the sources column when it is shown.
func (a *App) trafficPanesWidth(c *ui.Context, st *trafficState) float32 {
	w := a.workbenchWidth(c)
	if a.trafficSourcesShown(c, st) {
		w -= ds.TrafficSourcesWidth + ds.Space12
	}
	return w
}

// trafficSourcesShown reports whether the sources column is drawn. A pick made
// with the toggle wins; until one is made, the column opens on a wide page.
func (a *App) trafficSourcesShown(c *ui.Context, st *trafficState) bool {
	if st.sources == sourcesAuto {
		return a.workbenchWidth(c) >= ds.TrafficSourcesMin
	}
	return st.sources == sourcesOpen
}

// sync reads the log again when it or the filter changed, and keeps the
// chosen flow and the detail in step with what is there now.
func (st *trafficState) sync() {
	st.frames++
	log := st.log
	f := st.withFacets(st.makeFilter())
	if !st.fetched || log.Version() != st.ver || !sameFilter(f, st.filter) || st.rowsText != st.text || st.rowsChips != st.chips() {
		st.filter, st.ver, st.fetched, st.rowsText, st.rowsChips = f, log.Version(), true, st.text, st.chips()
		st.rows = st.narrowRows(log.Rows(f))
		st.total = len(st.rows)
		if st.filterActive() || st.procSel != "" || st.hostSel != "" {
			st.total = len(log.Rows(tool.Filter{Source: f.Source}))
		}
	}
	if st.pending != 0 {
		st.selectID(st.pending)
		st.pending = 0
		st.picked = true
	}
	st.followChoice()
	st.selectNewest()
	st.keepChoice()
}

// keepChoice loads the chosen flow's detail, and loads it again while the
// flow is still arriving, so that a filter or a new arrival does not quietly
// show another flow's detail.
func (st *trafficState) keepChoice() {
	if st.sel < 0 {
		st.hasDetail, st.detailID = false, 0
		return
	}
	row := st.rows[st.sel]
	// The detail is loaded again once the log changed and the row differs
	// from the flow the detail holds, so a flow that was still arriving shows
	// its response when it completes.
	stale := st.detailVer != st.ver && st.detailChanged(row)
	if st.detailID != row.ID || stale {
		st.loadDetail(row.ID)
	}
}

// detailChanged reports whether a row differs from the flow the detail holds
// in the fields the list shows: state, status, size or duration.
func (st *trafficState) detailChanged(r tool.FlowRow) bool {
	d := st.detail.FlowRow
	return r.State != d.State || r.Status != d.Status || r.Size != d.Size || r.Duration != d.Duration
}

// loadDetail takes a copy of a flow for the detail pane.
func (st *trafficState) loadDetail(id uint64) {
	f, ok := st.log.Get(id)
	if !ok {
		st.hasDetail, st.detailID = false, 0
		return
	}
	clear(st.memo) // the texts of the last flow do not outlive it
	st.detail, st.detailID, st.detailVer, st.hasDetail = f, id, st.ver, true
}

// indexOf returns the row that shows a flow, or -1.
func (st *trafficState) indexOf(id uint64) int {
	for i, r := range st.rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// followChoice finds the chosen flow among the rows now, so that a filter or
// a new arrival moves the choice with its flow. A flow the rows no longer
// hold is let go, and Auto select may choose the newest row again.
func (st *trafficState) followChoice() {
	if st.selID == 0 {
		st.sel = -1
		return
	}
	if i := st.indexOf(st.selID); i >= 0 {
		st.sel = i
		return
	}
	st.setSel(-1)
	st.autoID = 0
}

// setSel chooses row i and the flow it shows. An index with no row clears the
// choice.
func (st *trafficState) setSel(i int) {
	if i < 0 || i >= len(st.rows) {
		st.sel, st.selID = -1, 0
		return
	}
	st.sel, st.selID = i, st.rows[i].ID
}

// selectID chooses a flow and brings its row into view, once the rows hold it.
func (st *trafficState) selectID(id uint64) {
	if i := st.indexOf(id); i >= 0 {
		st.setSel(i)
		st.list.ScrollIntoView(i)
		return
	}
	st.pending = id
}

// sameFilter reports whether two filters pick the same flows.
func sameFilter(a, b tool.Filter) bool {
	return a.Text == b.Text && a.Failed == b.Failed &&
		a.Source == b.Source && slices.Equal(a.Processes, b.Processes) &&
		slices.Equal(a.Hosts, b.Hosts) && slices.Equal(a.Methods, b.Methods) &&
		slices.Equal(a.Types, b.Types) && slices.Equal(a.StatusClasses, b.StatusClasses)
}

// portValid reports whether text is a port a proxy may listen on.
func portValid(text string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	return err == nil && n >= trafficPortMin && n <= trafficPortMax
}
