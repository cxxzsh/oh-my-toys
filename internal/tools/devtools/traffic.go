package devtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// TrafficID is the id of the made-up traffic tool.
const TrafficID = "fake-traffic"

// trafficTick is how often the fake records another flow while it runs,
// so that the inspector's Follow switch has something to follow.
const trafficTick = time.Second

// trafficNotify is how often a log wakes the page that watches it.
const trafficNotify = 100 * time.Millisecond

// cannedPrograms are the programs that open the made-up flows, in turn. The
// last one opens nothing, so a flow with no program shows too.
var cannedPrograms = []tool.Program{
	{PID: 4120, Name: "Safari", Path: "/Applications/Safari.app/Contents/MacOS/Safari"},
	{PID: 2231, Name: "Slack", Path: "/Applications/Slack.app/Contents/MacOS/Slack"},
	{PID: 9087, Name: "curl", Path: "/usr/bin/curl"},
	{},
}

// cannedProgram is the program that opened the made-up flow with the id.
func cannedProgram(id uint64) tool.Program {
	return cannedPrograms[id%uint64(len(cannedPrograms))]
}

// Traffic is a made-up traffic tool: canned flows and packets, settings in
// memory, and a certificate that comes and goes as it is installed and
// removed. It exists so that the inspector page can be run and looked at,
// and so that -dev has something of every kind.
type Traffic struct {
	mu      sync.Mutex
	cfg     tool.CaptureConfig
	on      bool
	stop    chan struct{}
	cert    tool.CertState
	created bool
	trusted bool
	proxy   *fakeLog
	packets *fakeLog
	rules   []tool.Rule
}

// NewTraffic makes the made-up traffic tool, with its canned capture.
func NewTraffic() *Traffic {
	t := &Traffic{
		cfg:     tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 9099, SystemProxy: true, Decrypt: true},
		proxy:   newFakeLog(),
		packets: newFakeLog(),
		rules:   exampleRules(),
	}
	seedTraffic(t.proxy, t.packets)
	return t
}

// Manifest describes the tool.
func (t *Traffic) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:       TrafficID,
		Name:     func(m i18n.Messages) string { return m.TrafficName() },
		Summary:  func(m i18n.Messages) string { return m.TrafficSummary() },
		Category: tool.CategoryNetwork,
		Kind:     tool.KindSwitch,
		Icon:     "lan",
		Keywords: []string{"traffic", "capture", "proxy", "抓包", "代理"},
	}
}

// Specs declares start, stop and status.
func (t *Traffic) Specs() []tool.Spec {
	return tool.SwitchSpecs(
		func(m i18n.Messages) string { return m.TrafficStart() },
		func(m i18n.Messages) string { return m.TrafficStop() },
		func(m i18n.Messages) string { return m.TrafficStatus() },
	)
}

// Start begins recording, and appends a flow every second while it runs.
func (t *Traffic) Start(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.on {
		return nil
	}
	t.on = true
	stop := make(chan struct{})
	t.stop = stop
	go t.record(stop)
	return nil
}

// Stop ends the recording.
func (t *Traffic) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.on {
		return nil
	}
	t.on = false
	close(t.stop)
	t.stop = nil
	return nil
}

// Status says whether it records, and what it has.
func (t *Traffic) Status() tool.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.on {
		return tool.Status{Text: i18n.StatusOff()}
	}
	log := t.logLocked()
	if t.cfg.Mode == tool.ModePackets {
		return tool.Status{On: true, Text: i18n.TrafficRunningPackets(t.cfg.Interface, len(log.Rows(tool.Filter{Source: tool.SourcePacket})))}
	}
	addr := fmt.Sprintf("127.0.0.1:%d", t.cfg.ProxyPort)
	return tool.Status{On: true, Text: i18n.TrafficRunningProxy(addr, len(log.Rows(tool.Filter{Source: tool.SourceProxy})))}
}

// Config is what the controls show.
func (t *Traffic) Config() tool.CaptureConfig {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cfg
}

// SetConfig keeps the settings; the next flow recorded follows the mode
// they hold, as the log of each mode is a log of its own.
func (t *Traffic) SetConfig(cfg tool.CaptureConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cfg = cfg
	return nil
}

// Log is the log of the mode in progress.
func (t *Traffic) Log() tool.FlowLog {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.logLocked()
}

// Environment is what a program needs to go through the made-up proxy. It
// fails as the real tool does when the proxy is not running, or in packet
// mode.
func (t *Traffic) Environment() (tool.Environment, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.proxyReadyLocked(); err != nil {
		return tool.Environment{}, err
	}
	return fakeEnvironment(t.cfg.ProxyPort), nil
}

// OpenTerminal opens nothing, as the made-up tool has no terminal to open;
// it fails as Environment does.
func (t *Traffic) OpenTerminal(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.proxyReadyLocked()
}

// proxyReadyLocked is nil while the made-up proxy runs, ErrUnsupportedHere
// in packet mode and ErrNotRunning otherwise; the caller holds mu.
func (t *Traffic) proxyReadyLocked() error {
	if t.cfg.Mode == tool.ModePackets {
		return tool.ErrUnsupportedHere
	}
	if !t.on {
		return tool.ErrNotRunning
	}
	return nil
}

// fakeEnvironment sets the proxy variables, as a POSIX shell reads them.
func fakeEnvironment(port int) tool.Environment {
	proxy := fmt.Sprintf("http://127.0.0.1:%d", port)
	vars := []tool.Header{
		{Name: "http_proxy", Value: proxy},
		{Name: "https_proxy", Value: proxy},
		{Name: "all_proxy", Value: proxy},
		{Name: "no_proxy", Value: "localhost,127.0.0.1"},
	}
	var script strings.Builder
	for _, v := range vars {
		fmt.Fprintf(&script, "export %s=%s\n", v.Name, v.Value)
	}
	return tool.Environment{Vars: vars, Script: script.String(), Shell: "sh"}
}

// logLocked picks the log of the mode in progress; the caller holds mu.
func (t *Traffic) logLocked() *fakeLog {
	if t.cfg.Mode == tool.ModePackets {
		return t.packets
	}
	return t.proxy
}

// Rules is the made-up rule book.
func (t *Traffic) Rules() []tool.Rule {
	t.mu.Lock()
	defer t.mu.Unlock()
	return copyRules(t.rules)
}

// SetRules keeps a book the engine would accept, and refuses the rest the
// way the real engine does, so that the editor's error path can be seen.
func (t *Traffic) SetRules(rules []tool.Rule) error {
	if err := checkRules(rules); err != nil {
		return err
	}
	t.mu.Lock()
	t.rules = copyRules(rules)
	t.mu.Unlock()
	return nil
}

// Cert is the state of the made-up certificate.
func (t *Traffic) Cert() (tool.CertState, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.created {
		return tool.CertState{}, nil
	}
	cs := t.cert
	cs.Exists, cs.Trusted = true, t.trusted
	cs.NotAfter = time.Now().AddDate(1, 0, 0)
	return cs, nil
}

// InstallCert makes the certificate and trusts it.
func (t *Traffic) InstallCert(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.created, t.trusted = true, true
	t.cert.Fingerprint = fakeFingerprint
	t.cert.Path = fakeCertPath()
	return nil
}

// RemoveCert stops trusting the certificate and forgets it.
func (t *Traffic) RemoveCert(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.created, t.trusted = false, false
	t.cert = tool.CertState{}
	return nil
}

// Replay records the flow with the id again, as a new flow.
func (t *Traffic) Replay(_ context.Context, id uint64) (uint64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f, ok := t.proxy.get(id)
	if !ok {
		return 0, fmt.Errorf("replay flow %d: not found", id)
	}
	f.ID, f.ReplayOf, f.Started, f.State = 0, id, time.Now(), tool.FlowDone
	return t.proxy.add(f), nil
}

// ExportHAR writes a HAR file of the flows asked for. The made-up tool
// writes the shape of one, not its bodies.
func (t *Traffic) ExportHAR(w io.Writer, ids []uint64) error {
	t.mu.Lock()
	flows := t.proxy.snapshot(ids)
	t.mu.Unlock()
	entries := make([]map[string]any, 0, len(flows))
	for _, f := range flows {
		entries = append(entries, map[string]any{
			"startedDateTime": f.Started.Format(time.RFC3339Nano),
			"request":         map[string]any{"method": f.Method, "url": f.URL},
			"response":        map[string]any{"status": f.Status},
		})
	}
	return json.NewEncoder(w).Encode(map[string]any{
		"log": map[string]any{"version": "1.2", "creator": map[string]string{"name": "oh-my-toys"}, "entries": entries},
	})
}

// Interfaces are the interfaces a made-up capture can read.
func (t *Traffic) Interfaces() ([]tool.NetInterface, error) {
	return []tool.NetInterface{
		{Name: "en0", Description: "Wi-Fi", Up: true},
		{Name: "en1", Description: "Ethernet", Up: true},
		{Name: "lo0", Description: "Loopback"},
	}, nil
}

// record appends a flow of the mode in progress every trafficTick until
// stop closes, so that a page can watch a log grow.
func (t *Traffic) record(stop <-chan struct{}) {
	tk := time.NewTicker(trafficTick)
	defer tk.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-tk.C:
			t.appendLive(now)
		}
	}
}

// appendLive records one more flow, of the kind the mode records.
func (t *Traffic) appendLive(now time.Time) {
	t.mu.Lock()
	mode := t.cfg.Mode
	log := t.logLocked()
	t.mu.Unlock()
	if mode == tool.ModePackets {
		log.add(packetAt(now, log.count()))
		return
	}
	log.add(liveFlowAt(now, log.count()))
}

// fakeLog is a log of canned flows that behaves as tool.FlowLog says: it
// is safe for any goroutine, its rows are a snapshot, and it wakes its
// watchers at most every trafficNotify.
type fakeLog struct {
	mu       sync.Mutex
	flows    []tool.Flow
	version  uint64
	watch    map[int]func()
	watchID  int
	dirty    chan struct{}
	notified time.Time
}

func newFakeLog() *fakeLog {
	l := &fakeLog{watch: map[int]func(){}, dirty: make(chan struct{}, 1)}
	go l.wake()
	return l
}

// Version changes whenever the log does.
func (l *fakeLog) Version() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.version
}

// Rows returns the flows the filter picks, oldest first.
func (l *fakeLog) Rows(f tool.Filter) []tool.FlowRow {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]tool.FlowRow, 0, len(l.flows))
	for _, flow := range l.flows {
		if trafficMatch(flow, f) {
			out = append(out, flow.FlowRow)
		}
	}
	return out
}

// Get returns a flow with everything it carried.
func (l *fakeLog) Get(id uint64) (tool.Flow, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.get(id)
}

// get is Get with the caller holding mu.
func (l *fakeLog) get(id uint64) (tool.Flow, bool) {
	for _, flow := range l.flows {
		if flow.ID == id {
			return flow, true
		}
	}
	return tool.Flow{}, false
}

// Clear forgets every flow.
func (l *fakeLog) Clear() {
	l.mu.Lock()
	l.flows, l.version = nil, l.version+1
	l.mu.Unlock()
	l.touch()
}

// OnChange calls fn after the log changes, off the UI thread, and at most
// every trafficNotify; stop ends it.
func (l *fakeLog) OnChange(fn func()) (stop func()) {
	l.mu.Lock()
	id := l.watchID
	l.watchID++
	l.watch[id] = fn
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		delete(l.watch, id)
		l.mu.Unlock()
	}
}

// add records a flow under the next id, and returns that id.
func (l *fakeLog) add(f tool.Flow) uint64 {
	l.mu.Lock()
	id := l.nextIDLocked()
	f.ID = id
	if f.Process == (tool.Program{}) {
		f.Process = cannedProgram(id)
	}
	l.flows = append(l.flows, f)
	l.version++
	l.mu.Unlock()
	l.touch()
	return id
}

// nextIDLocked is the id after the last flow's; the caller holds mu.
func (l *fakeLog) nextIDLocked() uint64 {
	var last uint64
	for _, f := range l.flows {
		last = max(last, f.ID)
	}
	return last + 1
}

// count is how many flows the log holds.
func (l *fakeLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.flows)
}

// Processes counts the flows by the program that opened them.
func (l *fakeLog) Processes() []tool.Facet {
	l.mu.Lock()
	defer l.mu.Unlock()
	return countFacets(l.flows, func(f tool.Flow) string { return f.Process.Name })
}

// Hosts counts the flows by host.
func (l *fakeLog) Hosts() []tool.Facet {
	l.mu.Lock()
	defer l.mu.Unlock()
	return countFacets(l.flows, func(f tool.Flow) string { return f.Host })
}

// snapshot copies the flows with the given ids, or all of them.
func (l *fakeLog) snapshot(ids []uint64) []tool.Flow {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(ids) == 0 {
		return slices.Clone(l.flows)
	}
	var out []tool.Flow
	for _, id := range ids {
		if f, ok := l.get(id); ok {
			out = append(out, f)
		}
	}
	return out
}

// touch wakes the watchers, at most every trafficNotify.
func (l *fakeLog) touch() {
	select {
	case l.dirty <- struct{}{}:
	default:
	}
}

// wake calls the watchers off the goroutine that changed the log, no more
// often than trafficNotify.
func (l *fakeLog) wake() {
	for range l.dirty {
		l.mu.Lock()
		wait := trafficNotify - time.Since(l.notified)
		l.mu.Unlock()
		if wait > 0 {
			time.Sleep(wait)
		}
		l.mu.Lock()
		l.notified = time.Now()
		fns := slices.Collect(maps.Values(l.watch))
		l.mu.Unlock()
		for _, fn := range fns {
			fn()
		}
	}
}

// countFacets counts the flows by the name that name gives them, grouping
// names ignoring case and keeping the first spelling; a flow with no name
// is left out. The most flows come first, then the names in order.
func countFacets(flows []tool.Flow, name func(tool.Flow) string) []tool.Facet {
	byKey := map[string]*tool.Facet{}
	for _, f := range flows {
		n := name(f)
		if n == "" {
			continue
		}
		key := strings.ToLower(n)
		if c, ok := byKey[key]; ok {
			c.Count++
			continue
		}
		byKey[key] = &tool.Facet{Name: n, Count: 1}
	}
	out := make([]tool.Facet, 0, len(byKey))
	for _, c := range byKey {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b tool.Facet) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// trafficMatch is what the made-up log's filters do: the same things the
// real one promises.
func trafficMatch(f tool.Flow, filter tool.Filter) bool {
	if f.Source != filter.Source {
		return false
	}
	if filter.Text != "" && !trafficText(f, strings.ToLower(filter.Text)) {
		return false
	}
	if len(filter.StatusClasses) > 0 && !slices.Contains(filter.StatusClasses, f.Status/100) {
		return false
	}
	if !trafficNamesMatch(f, filter) {
		return false
	}
	return !filter.Failed || f.State == tool.FlowFailed || f.State == tool.FlowBlocked
}

// trafficNamesMatch is what the made-up log's list filters do: the hosts,
// methods, media types and programs, each of which matches any of its
// entries.
func trafficNamesMatch(f tool.Flow, filter tool.Filter) bool {
	if len(filter.Hosts) > 0 && !slices.ContainsFunc(filter.Hosts, func(h string) bool {
		return strings.EqualFold(h, f.Host)
	}) {
		return false
	}
	if len(filter.Methods) > 0 && !slices.Contains(filter.Methods, f.Method) {
		return false
	}
	if len(filter.Types) > 0 && !slices.ContainsFunc(filter.Types, func(p string) bool {
		return strings.HasPrefix(f.Type, p)
	}) {
		return false
	}
	return len(filter.Processes) == 0 || slices.ContainsFunc(filter.Processes, func(p string) bool {
		return strings.EqualFold(p, f.Process.Name)
	})
}

// trafficText reports whether the lowered text is in the flow's URL, its
// header lines, a text body or its note.
func trafficText(f tool.Flow, text string) bool {
	if strings.Contains(strings.ToLower(f.URL), text) || strings.Contains(strings.ToLower(f.Info), text) {
		return true
	}
	for _, msg := range []tool.HTTPMessage{f.Request, f.Response} {
		if strings.Contains(strings.ToLower(string(msg.Body.Data)), text) {
			return true
		}
		for _, h := range msg.Headers {
			if strings.Contains(strings.ToLower(h.Name+": "+h.Value), text) {
				return true
			}
		}
	}
	return false
}
