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
		return tool.Status{On: true, Text: i18n.StatusOn()}
	}
	return tool.Status{Text: i18n.StatusOff()}
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

// testMatch is the filter the contract describes, for the tests' log.
func testMatch(f tool.Flow, filter tool.Filter) bool {
	if f.Source != filter.Source {
		return false
	}
	if filter.Text != "" {
		text := strings.ToLower(filter.Text)
		if !strings.Contains(strings.ToLower(f.URL+" "+f.Info+" "+string(f.Request.Body.Data)+" "+string(f.Response.Body.Data)), text) {
			return false
		}
	}
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
	return !filter.Failed || f.State == tool.FlowFailed || f.State == tool.FlowBlocked
}

// testGET is a proxied GET that answered JSON.
func testGET(host, path string, status int) tool.Flow {
	row := tool.FlowRow{
		Source: tool.SourceProxy, Started: time.Now(), Method: "GET", Host: host, Path: path,
		Proto: "h2", Status: status, Type: "application/json", Size: 12, State: tool.FlowDone,
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
	a := newTestApp(t, false)
	reg, err := core.NewRegistry(f)
	if err != nil {
		t.Fatal(err)
	}
	a.reg = reg
	a.open(trafficTestID)
	tt := ui.NewTester(a.mainView, ds.WindowWidth, 1000)
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
	if err := tt.Click(i18n.TrafficTabResponse()); err != nil {
		t.Fatal(err)
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

// TestTrafficFiltersNarrow: a method chip and the search field each take
// flows out of the list.
func TestTrafficFiltersNarrow(t *testing.T) {
	f := newTestTraffic()
	f.proxy.add(testGET("api.example.com", "/get", 200))
	f.proxy.add(testPOST("api.example.com", "/post"))
	f.proxy.add(testGET("www.example.com", "/docs", 200))
	tt := openTraffic(t, f)
	if !tt.HasText("/post") || !tt.HasText("/docs") {
		t.Fatalf("the rows are not there: %q", tt.Texts())
	}
	if err := tt.Click("POST"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("/docs") || !tt.HasText("/post") {
		t.Errorf("the method chip did not narrow the list: %q", tt.Texts())
	}
	if err := tt.Click(i18n.TrafficClearFilters()); err != nil {
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

// TestTrafficModeSwitchesColumns: packets are rows of their own, with the
// protocol and what the packet is.
func TestTrafficModeSwitchesColumns(t *testing.T) {
	f := newTestTraffic()
	f.proxy.add(testGET("api.example.com", "/get", 200))
	packet := testPacket("SYN 52114 → 443")
	packet.ID = f.packets.add(packet)
	tt := openTraffic(t, f)
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
	if tt.HasText("/later") {
		t.Errorf("the list followed after scrolling up: %q", tt.Texts())
	}
}

// TestTrafficFieldsCheckInput: a port or an upstream proxy that cannot be
// used says so, and is not applied.
func TestTrafficFieldsCheckInput(t *testing.T) {
	f := newTestTraffic()
	tt := openTraffic(t, f)
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
