package shell

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// trafficTestID is the id of the tool the inspector tests open.
const trafficTestID = "test-traffic"

// trafficWait bounds how long a test waits for a background action to
// report back through the page.
const trafficWait = 2 * time.Second

// testTraffic is a traffic tool the tests drive: canned flows and packets,
// settings in memory, and failures it returns on demand.
type testTraffic struct {
	mu        sync.Mutex
	cfg       tool.CaptureConfig
	on        bool
	cert      tool.CertState
	startErr  error
	proxy     *testLog
	packets   *testLog
	replays   []uint64
	harIDs    []uint64
	rules     []tool.Rule
	rulesErr  error
	ruleBooks [][]tool.Rule
	note      string

	// termErr is what OpenTerminal fails with, when a test sets it.
	termErr error
	// terminals counts the terminals a test has opened.
	terminals int
}

func newTestTraffic() *testTraffic {
	return &testTraffic{
		cfg:     tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 9099},
		proxy:   newTestLog(),
		packets: newTestLog(),
	}
}

func (f *testTraffic) Manifest() tool.Manifest {
	return tool.Manifest{
		ID: trafficTestID, Name: titled("Traffic"), Summary: titled("Capture and inspect"),
		Kind: tool.KindSwitch, Category: tool.CategoryNetwork, Icon: "lan",
	}
}

func (f *testTraffic) Specs() []tool.Spec {
	s := titled("x")
	return tool.SwitchSpecs(s, s, s)
}

func (f *testTraffic) Start(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	f.on = true
	return nil
}

func (f *testTraffic) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.on = false
	return nil
}

func (f *testTraffic) Status() tool.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.on {
		return tool.Status{On: true, Text: i18n.StatusOn(), Note: f.note}
	}
	return tool.Status{Text: i18n.StatusOff(), Note: f.note}
}

func (f *testTraffic) Config() tool.CaptureConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cfg
}

func (f *testTraffic) SetConfig(cfg tool.CaptureConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cfg = cfg
	return nil
}

func (f *testTraffic) Log() tool.FlowLog {
	if f.Config().Mode == tool.ModePackets {
		return f.packets
	}
	return f.proxy
}

func (f *testTraffic) Rules() []tool.Rule {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneRules(f.rules)
}

func (f *testTraffic) SetRules(rules []tool.Rule) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rulesErr != nil {
		return f.rulesErr
	}
	f.rules = cloneRules(rules)
	f.ruleBooks = append(f.ruleBooks, cloneRules(rules))
	return nil
}

func (f *testTraffic) Cert() (tool.CertState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cert, nil
}

func (f *testTraffic) InstallCert(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cert = tool.CertState{Exists: true, Trusted: true, Fingerprint: "a1b2c3d4e5f6", Path: "/tmp/ca.pem"}
	return nil
}

func (f *testTraffic) RemoveCert(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cert = tool.CertState{}
	return nil
}

func (f *testTraffic) Replay(_ context.Context, id uint64) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replays = append(f.replays, id)
	flow, ok := f.proxy.get(id)
	if !ok {
		return 0, fmt.Errorf("flow %d: not found", id)
	}
	flow.ID, flow.ReplayOf = 0, id
	return f.proxy.add(flow), nil
}

func (f *testTraffic) ExportHAR(w io.Writer, ids []uint64) error {
	f.mu.Lock()
	f.harIDs = ids
	f.mu.Unlock()
	_, err := io.WriteString(w, "{}\n")
	return err
}

func (f *testTraffic) Interfaces() ([]tool.NetInterface, error) {
	return []tool.NetInterface{
		{Name: "en0", Description: "Wi-Fi", Up: true},
		{Name: "en1", Description: "Ethernet", Up: true},
	}, nil
}

// testLog is a log the tests fill, with the filter the contract promises.
type testLog struct {
	mu    sync.Mutex
	flows []tool.Flow
	next  uint64
	ver   uint64
}

func newTestLog() *testLog { return &testLog{} }

func (l *testLog) add(f tool.Flow) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next++
	f.ID = l.next
	l.flows = append(l.flows, f)
	l.ver++
	return f.ID
}

func (l *testLog) Version() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ver
}

func (l *testLog) Rows(f tool.Filter) []tool.FlowRow {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []tool.FlowRow
	for _, flow := range l.flows {
		if testMatch(flow, f) {
			out = append(out, flow.FlowRow)
		}
	}
	return out
}

func (l *testLog) Get(id uint64) (tool.Flow, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.get(id)
}

func (l *testLog) get(id uint64) (tool.Flow, bool) {
	for _, flow := range l.flows {
		if flow.ID == id {
			return flow, true
		}
	}
	return tool.Flow{}, false
}

func (l *testLog) Clear() {
	l.mu.Lock()
	l.flows, l.ver = nil, l.ver+1
	l.mu.Unlock()
}

func (l *testLog) OnChange(func()) func() { return func() {} }

// Processes counts the flows of each program, the way the contract orders them.
func (l *testLog) Processes() []tool.Facet {
	l.mu.Lock()
	defer l.mu.Unlock()
	counts := map[string]int{}
	for _, f := range l.flows {
		if f.Process.Name != "" {
			counts[f.Process.Name]++
		}
	}
	return testFacets(counts)
}

// Hosts counts the flows to each host.
func (l *testLog) Hosts() []tool.Facet {
	l.mu.Lock()
	defer l.mu.Unlock()
	counts := map[string]int{}
	for _, f := range l.flows {
		if f.Host != "" {
			counts[f.Host]++
		}
	}
	return testFacets(counts)
}

// testFacets orders counts most flows first, then by name.
func testFacets(counts map[string]int) []tool.Facet {
	out := make([]tool.Facet, 0, len(counts))
	for name, n := range counts {
		out = append(out, tool.Facet{Name: name, Count: n})
	}
	slices.SortFunc(out, func(a, b tool.Facet) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// Environment is the proxy environment of a running proxy, as a shell script.
func (f *testTraffic) Environment() (tool.Environment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.on {
		return tool.Environment{}, tool.ErrNotRunning
	}
	const proxy = "http://127.0.0.1:9099"
	vars := []tool.Header{{Name: "http_proxy", Value: proxy}, {Name: "https_proxy", Value: proxy}}
	script := "export http_proxy=" + proxy + " https_proxy=" + proxy
	return tool.Environment{Vars: vars, Script: script, Shell: "/bin/zsh"}, nil
}

// OpenTerminal counts a terminal opened with the proxy, or fails as the
// test asks.
func (f *testTraffic) OpenTerminal(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.on {
		return tool.ErrNotRunning
	}
	if f.termErr != nil {
		return f.termErr
	}
	f.terminals++
	return nil
}

// testMatch is the filter the contract describes, for the tests' log.
func testMatch(f tool.Flow, filter tool.Filter) bool {
	if f.Source != filter.Source || !testMatchText(f, filter.Text) || !testMatchFields(f, filter) {
		return false
	}
	return !filter.Failed || f.State == tool.FlowFailed || f.State == tool.FlowBlocked
}

// testMatchText is the free-text part of the filter: URL, info and bodies.
func testMatchText(f tool.Flow, text string) bool {
	if text == "" {
		return true
	}
	all := f.URL + " " + f.Info + " " + string(f.Request.Body.Data) + " " + string(f.Response.Body.Data)
	return strings.Contains(strings.ToLower(all), strings.ToLower(text))
}

// testMatchFields is the list-valued part of the filter, one field each.
func testMatchFields(f tool.Flow, filter tool.Filter) bool {
	if len(filter.Hosts) > 0 && !slices.Contains(filter.Hosts, f.Host) {
		return false
	}
	if len(filter.Methods) > 0 && !slices.Contains(filter.Methods, f.Method) {
		return false
	}
	if len(filter.Types) > 0 && !slices.ContainsFunc(filter.Types, func(p string) bool { return strings.HasPrefix(f.Type, p) }) {
		return false
	}
	if len(filter.StatusClasses) > 0 && !slices.Contains(filter.StatusClasses, f.Status/100) {
		return false
	}
	return len(filter.Processes) == 0 || slices.ContainsFunc(filter.Processes, func(n string) bool {
		return strings.EqualFold(n, f.Process.Name)
	})
}

// testGET is a proxied GET that answered JSON.
func testGET(host, path string, status int) tool.Flow {
	row := tool.FlowRow{
		Source: tool.SourceProxy, Started: time.Now(), Method: "GET", Host: host, Path: path,
		Proto: "h2", Status: status, Type: "application/json", Size: 12, State: tool.FlowDone,
		Duration: time.Second,
	}
	return tool.Flow{
		FlowRow: row, URL: "https://" + host + path,
		Response: tool.HTTPMessage{
			Headers: []tool.Header{{Name: "Content-Type", Value: "application/json"}},
			Body:    tool.Body{Type: "application/json", Data: []byte(`{"name":"Widget"}`), Size: 18},
		},
	}
}

// testPOST is a proxied POST.
func testPOST(host, path string) tool.Flow {
	f := testGET(host, path, 201)
	f.Method = "POST"
	return f
}

// testPacket is a packet row.
func testPacket(info string) tool.Flow {
	row := tool.FlowRow{
		Source: tool.SourcePacket, Started: time.Now(), Host: "93.184.216.34",
		Proto: "TCP", Info: info, Size: 54, State: tool.FlowDone,
	}
	return tool.Flow{FlowRow: row, Packet: &tool.PacketDetail{
		Layers: []tool.Layer{{Name: "TCP", Fields: []tool.Header{{Name: "Flags", Value: "SYN"}}}},
		Raw:    []byte("sniffed"),
	}}
}

// openTraffic opens the inspector of f in a window of its own.
func openTraffic(t *testing.T, f *testTraffic) *ui.Tester {
	t.Helper()
	return openTrafficAt(t, f, ds.WindowWidth)
}

// openTrafficAt opens the inspector of f in a window of the given width.
func openTrafficAt(t *testing.T, f *testTraffic, width int) *ui.Tester {
	t.Helper()
	a := newTestApp(t, false)
	reg, err := core.NewRegistry(f)
	if err != nil {
		t.Fatal(err)
	}
	a.reg = reg
	a.open(trafficTestID)
	tt := ui.NewTester(a.mainView, width, 1000)
	tt.Frame()
	return tt
}

// waitText draws frames until the page shows s, or fails the test.
func waitText(t *testing.T, tt *ui.Tester, s string) {
	t.Helper()
	deadline := time.Now().Add(trafficWait)
	for time.Now().Before(deadline) {
		if tt.HasText(s) {
			return
		}
		time.Sleep(time.Millisecond)
		tt.Frame()
	}
	t.Fatalf("the page never showed %q: %q", s, tt.Texts())
}

// TestTrafficPageRowsAndDetail: the flows are listed, and choosing one
// shows what it carried.
func TestTrafficPageRowsAndDetail(t *testing.T) {
	f := newTestTraffic()
	flow := testGET("api.example.com", "/v1/items", 200)
	flow.ID = f.proxy.add(flow)
	f.proxy.add(testPOST("api.example.com", "/v1/orders"))
	tt := openTraffic(t, f)
	if !tt.HasText("api.example.com") || !tt.HasText("/v1/orders") {
		t.Fatalf("the rows are not there: %q", tt.Texts())
	}
	if err := tt.Click(trafficRowLabel(flow.FlowRow)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("https://api.example.com/v1/items") {
		t.Errorf("the detail does not show the URL: %q", tt.Texts())
	}
	tt.Frame()
	if !tt.HasText(`"name": "Widget"`) {
		t.Errorf("the response is not shown pretty-printed: %q", tt.Texts())
	}
}

// TestTrafficPageVirtualises: a log of fifty thousand flows draws a frame
// in the time of one, because only the rows in view are built.
func TestTrafficPageVirtualises(t *testing.T) {
	long := newTestTraffic()
	for i := range 50000 {
		long.proxy.add(testGET(fmt.Sprintf("h%d.example.com", i), "/x", 200))
	}
	short := newTestTraffic()
	for i := range 20 {
		short.proxy.add(testGET(fmt.Sprintf("s%d.example.com", i), "/x", 200))
	}
	longTT := openTraffic(t, long)
	if longTT.HasText("h40000.example.com") {
		t.Error("a row far below the view was built")
	}
	if !longTT.HasText("h49999.example.com") {
		t.Errorf("the list does not follow its end: %q", longTT.Texts())
	}
	big, small := trafficFrameTime(longTT), trafficFrameTime(openTraffic(t, short))
	t.Logf("a frame of 50000 flows: %v; of 20 flows: %v", big, small)
	if big > 3*small+100*time.Millisecond {
		t.Errorf("50000 flows take %v a frame against %v for 20: the rows are not virtualised", big, small)
	}
}

// trafficFrameTime is the median of five frames of a page.
func trafficFrameTime(tt *ui.Tester) time.Duration {
	frames := make([]time.Duration, 0, 5)
	for range 5 {
		start := time.Now()
		tt.Frame()
		frames = append(frames, time.Since(start))
	}
	slices.Sort(frames)
	return frames[len(frames)/2]
}

// TestTrafficFiltersNarrow: the Failed chip and the search field each take
// flows out of the list, and All puts every flow back. The chips sit in one
// row under the toolbar.
func TestTrafficFiltersNarrow(t *testing.T) {
	f := newTestTraffic()
	f.proxy.add(testGET("api.example.com", "/get", 200))
	f.proxy.add(testPOST("api.example.com", "/post"))
	f.proxy.add(testGET("www.example.com", "/docs", 200))
	broken := testGET("www.example.com", "/broken", 500)
	broken.State = tool.FlowFailed
	f.proxy.add(broken)
	tt := openTraffic(t, f)
	tt.SetSize(1600, 800) // narrower, the chip row scrolls and hides Failed
	if !tt.HasText("/post") || !tt.HasText("/docs") || !tt.HasText("/broken") {
		t.Fatalf("the rows are not there: %q", tt.Texts())
	}
	if err := tt.Click(i18n.TrafficFilterFailed()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("/docs") || !tt.HasText("/broken") {
		t.Errorf("the failed filter did not narrow the list: %q", tt.Texts())
	}
	if err := tt.Click(i18n.All()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(i18n.TrafficSearch()); err != nil {
		t.Fatal(err)
	}
	tt.Type("docs")
	tt.Frame()
	if tt.HasText("/post") || !tt.HasText("/docs") {
		t.Errorf("the search did not narrow the list: %q", tt.Texts())
	}
}

// TestTrafficSearchMethodPOST: a method:POST token in the search field keeps
// only the POST rows.
func TestTrafficSearchMethodPOST(t *testing.T) {
	f := newTestTraffic()
	f.proxy.add(testGET("api.example.com", "/get", 200))
	f.proxy.add(testPOST("www.example.com", "/post"))
	tt := openTraffic(t, f)
	if err := tt.Click(i18n.TrafficSearch()); err != nil {
		t.Fatal(err)
	}
	tt.Type("method:POST")
	tt.Frame()
	if tt.HasText("/get") || !tt.HasText("/post") {
		t.Errorf("method:POST did not narrow to POST rows: %q", tt.Texts())
	}
}

// TestTrafficModeSwitchesColumns: packets are rows of their own, with the
// protocol and what the packet is.
func TestTrafficModeSwitchesColumns(t *testing.T) {
	f := newTestTraffic()
	f.proxy.add(testGET("api.example.com", "/get", 200))
	packet := testPacket("SYN 52114 → 443")
	packet.ID = f.packets.add(packet)
	tt := openTraffic(t, f)
	if err := tt.Click(i18n.TrafficSettings()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(i18n.TrafficModePackets()); err != nil {
		t.Fatal(err)
	}
	waitText(t, tt, i18n.TrafficColProtocol())
	if !tt.HasText("SYN 52114 → 443") {
		t.Errorf("the packet is not listed: %q", tt.Texts())
	}
	if tt.HasText("/get") {
		t.Errorf("a proxied flow is still listed in packet mode: %q", tt.Texts())
	}
	if err := tt.Click(trafficRowLabel(packet.FlowRow)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Flags") {
		t.Errorf("the packet's layers are not shown: %q", tt.Texts())
	}
	if err := tt.Click(i18n.TrafficTabHex()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("00000000") {
		t.Errorf("the packet's bytes are not shown: %q", tt.Texts())
	}
}

// TestTrafficProblemGuides: each failure is explained in words, and the
// error's own text is never the headline.
func TestTrafficProblemGuides(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		title string
		steps string
	}{
		{"permission", fmt.Errorf("open /dev/bpf0: %w", tool.ErrPermission), i18n.TrafficErrPermissionTitle(), i18n.TrafficErrPermissionSteps()},
		{"driver", fmt.Errorf("load wpcap: %w", tool.ErrDriverMissing), i18n.TrafficErrDriverTitle(), i18n.TrafficErrDriverSteps()},
		{"port", fmt.Errorf("listen 127.0.0.1:9099: %w", tool.ErrPortInUse), i18n.TrafficErrPortTitle(), i18n.TrafficErrPortSteps()},
		{"certificate", fmt.Errorf("handshake: %w", tool.ErrCertNotTrusted), i18n.TrafficErrCertTitle(), i18n.TrafficErrCertSteps()},
		{"unsupported", fmt.Errorf("grant: %w", tool.ErrUnsupportedHere), i18n.TrafficErrUnsupportedTitle(), i18n.TrafficErrUnsupportedSteps()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestTraffic()
			f.startErr = tt.err
			page := openTraffic(t, f)
			if err := page.Click(i18n.SwitchStart()); err != nil {
				t.Fatal(err)
			}
			waitText(t, page, tt.title)
			if !page.HasText(tt.steps) {
				t.Errorf("the steps are not shown: %q", page.Texts())
			}
			if page.HasText(tt.err.Error()) {
				t.Errorf("the raw error is shown: %q", page.Texts())
			}
		})
	}
}

// TestTrafficFollowKeepsEnd: the list follows what arrives until a person
// scrolls up, as a log does.
func TestTrafficFollowKeepsEnd(t *testing.T) {
	f := newTestTraffic()
	for range 40 {
		f.proxy.add(testGET("api.example.com", "/x", 200))
	}
	tt := openTraffic(t, f)
	last := testGET("api.example.com", "/arrived", 200)
	last.ID = f.proxy.add(last)
	tt.Frame()
	if !tt.HasText("/arrived") {
		t.Fatalf("the list does not follow its end: %q", tt.Texts())
	}
	row, ok := tt.Find(trafficRowLabel(last.FlowRow))
	if !ok {
		t.Fatal("the last row is not there to scroll from")
	}
	for range 3 {
		tt.Scroll(row.X+row.W/2, row.Y+row.H/2, 0, -300)
		tt.Frame()
	}
	later := testGET("api.example.com", "/later", 200)
	f.proxy.add(later)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find(trafficRowLabel(later.FlowRow)); ok {
		t.Errorf("the list followed after scrolling up: %q", tt.Texts())
	}
}

// TestTrafficFieldsCheckInput: a port or an upstream proxy that cannot be
// used says so, and is not applied.
func TestTrafficFieldsCheckInput(t *testing.T) {
	f := newTestTraffic()
	tt := openTraffic(t, f)
	if err := tt.Click(i18n.TrafficSettings()); err != nil {
		t.Fatal(err)
	}
	clickField(t, tt, i18n.TrafficPort())
	tt.Type("9")
	tt.Frame()
	if !hasPortError(tt) {
		t.Errorf("no port error: %q", tt.Texts())
	}
	clickField(t, tt, i18n.TrafficUpstream())
	tt.Type("ftp://example.com:21")
	tt.Frame()
	if !hasUpstreamError(tt) {
		t.Errorf("no upstream error: %q", tt.Texts())
	}
	if got := f.Config().Upstream; got != "" {
		t.Errorf("an unusable upstream was applied: %q", got)
	}
}

// clickField puts the pointer in the text field under label: the label
// above it is the first element that shows that name.
func clickField(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	box, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no field labelled %q", label)
	}
	tt.ClickAt(box.X+box.W/2, box.Y+box.H+ds.Space4+ds.ControlLg/2)
}

// hasPortError reports whether the page shows the port field's error.
func hasPortError(tt *ui.Tester) bool {
	for _, s := range tt.Texts() {
		if strings.Contains(s, "1024 to 65535") {
			return true
		}
	}
	return false
}

// hasUpstreamError reports whether the page shows the upstream field's
// error.
func hasUpstreamError(tt *ui.Tester) bool {
	for _, s := range tt.Texts() {
		if strings.Contains(s, "socks5://host:port") {
			return true
		}
	}
	return false
}

// TestTrafficPageShowsTheStatusNote: what the tool says beside its state,
// as a system proxy that apps do not read, is in the Settings dialog.
func TestTrafficPageShowsTheStatusNote(t *testing.T) {
	f := newTestTraffic()
	f.note = "apps read another service's proxy"
	tt := openTraffic(t, f)
	if err := tt.Click(i18n.TrafficSettings()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(f.note) {
		t.Errorf("the page does not show %q", f.note)
	}
}
