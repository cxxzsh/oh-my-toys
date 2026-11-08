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

	text    string
	failed  bool
	methods [trafficMethodCount]bool
	classes [trafficClassCount]bool
	types   [trafficTypeCount]bool

	view       int // the flows, or the rule book
	rules      ruleView
	sel        int // the chosen row, -1 for none
	list       ui.ListState
	search     ui.Handle
	follow     bool
	splitW     float32 // the list's width beside the detail
	rulesSplit float32 // the rule list's width beside its form
	splitH     float32 // the list's height above the detail
	detailTab  int
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
		sel: -1, follow: true,
		splitW: ds.TrafficSplit, splitH: ds.TrafficSplitH,
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
	page.Children(func() {
		if st.saved != "" {
			c.Toast(i18n.TrafficExported(st.saved))
			st.saved = ""
		}
		a.trafficHead(c, t, st)
		a.trafficControls(c, t, st)
		a.trafficTabs(c, st)
		a.trafficProblem(c, st)
		if st.view == viewRules {
			a.rulesView(c, t, st)
			return
		}
		a.trafficFilterBar(c, t, st)
		a.trafficPanes(c, t, st)
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
	st.rows, st.fetched, st.sel = nil, false, -1
	st.detailID, st.detail, st.hasDetail = 0, tool.Flow{}, false
	st.list = ui.ListState{}
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

// trafficHead is the page's title, the state of the capture and what acts
// on it.
func (a *App) trafficHead(c *ui.Context, t tool.Traffic, st *trafficState) {
	m := t.Manifest()
	comp.PageHeader(c, comp.Header{
		Title:       m.Name(msgs()),
		Badge:       t.Status().Text,
		Description: m.Summary(msgs()),
		Meta:        func() { a.trafficActions(c, t, st) },
	})
}

// trafficActions starts and stops the capture, clears the log and saves a
// HAR file.
func (a *App) trafficActions(c *ui.Context, t tool.Traffic, st *trafficState) {
	if a.trafficBusy(st) {
		comp.Text(c, i18n.Working(), ds.Caption, ds.P(c).TextInactive)
	}
	on := t.Status().On
	opts := comp.ButtonOpts{Kind: comp.Primary, Small: true, Glyph: glyphPower, Disabled: a.trafficBusy(st)}
	label := i18n.SwitchStart()
	if on {
		label, opts = i18n.Stop(), comp.ButtonOpts{Kind: comp.Danger, Small: true, Disabled: a.trafficBusy(st)}
	}
	if a.kit.Button(c.Key("traffic-on"), label, opts).Clicked() {
		a.trafficSetOn(t, st, !on)
	}
	if a.kit.Button(c.Key("traffic-clear"), i18n.TrafficClear(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
		t.Log().Clear()
	}
	if t.Config().Mode == tool.ModeProxy &&
		a.kit.Button(c.Key("traffic-har"), i18n.TrafficExport(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
		a.trafficExport(t, st)
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
	list := func() { a.trafficListPane(c, t, st) }
	detail := func() { a.trafficDetailPane(c, t, st) }
	if w, _ := c.Size(); w >= ds.TrafficWide {
		comp.Split(c, &st.splitW, list, detail).Grow(1).FillWidth()
		return
	}
	comp.SplitVertical(c, &st.splitH, list, detail).Grow(1).FillWidth()
}

// sync reads the log again when it or the filter changed, and keeps the
// chosen flow and the detail in step with what is there now.
func (st *trafficState) sync() {
	st.frames++
	log := st.log
	f := st.narrowFilter(st.makeFilter())
	if !st.fetched || log.Version() != st.ver || !sameFilter(f, st.filter) {
		st.filter, st.ver, st.fetched = f, log.Version(), true
		st.rows = st.narrowRows(log.Rows(f))
	}
	if st.pending != 0 {
		st.selectID(st.pending)
		st.pending = 0
	}
	st.keepChoice()
}

// keepChoice keeps the choice by flow, and loads the chosen flow again
// while it is still arriving, so that a filter or a new arrival does not
// quietly show another flow's detail.
func (st *trafficState) keepChoice() {
	if st.detailID != 0 {
		if i := st.indexOf(st.detailID); i >= 0 {
			st.sel = i
		}
	}
	st.sel = max(-1, min(st.sel, len(st.rows)-1))
	if st.sel < 0 {
		st.hasDetail, st.detailID = false, 0
		return
	}
	row := st.rows[st.sel]
	open := row.State == tool.FlowOpen && st.detailVer != st.ver
	if st.detailID != row.ID || open {
		st.loadDetail(row.ID)
	}
}

// loadDetail takes a copy of a flow for the detail pane.
func (st *trafficState) loadDetail(id uint64) {
	f, ok := st.log.Get(id)
	if !ok {
		st.hasDetail, st.detailID = false, 0
		return
	}
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

// selectID chooses a flow and brings its row into view, once the rows
// hold it.
func (st *trafficState) selectID(id uint64) {
	if i := st.indexOf(id); i >= 0 {
		st.sel = i
		st.list.ScrollIntoView(i)
		return
	}
	st.pending = id
}

// sameFilter reports whether two filters pick the same flows.
func sameFilter(a, b tool.Filter) bool {
	return a.Source == b.Source && a.Text == b.Text && a.Failed == b.Failed &&
		slices.Equal(a.Hosts, b.Hosts) && slices.Equal(a.Methods, b.Methods) &&
		slices.Equal(a.Types, b.Types) && slices.Equal(a.StatusClasses, b.StatusClasses)
}

// portValid reports whether text is a port a proxy may listen on.
func portValid(text string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	return err == nil && n >= trafficPortMin && n <= trafficPortMax
}
