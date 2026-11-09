package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
)

// The Traffic settings the scenario starts from. The proxy listens on
// trafficProxyPort; the system proxy and certificate decryption stay off so
// the runner's network is never touched.
const (
	// The driver's copy of internal/core/portable.go: a regular file named
	// portableMarker next to the executable makes the app keep its data in
	// the portableData folder beside it. The driver keeps its own copy so it
	// does not link the app's packages.
	portableMarker = "portable"
	portableData   = "data"
	trafficFolder  = "traffic"
	configFileName = "config.json"
	// paletteKey opens the app's command palette; Escape closes it again.
	paletteKey     = "CmdOrCtrl+K"
	escapeKey      = "Escape"
	trafficConfig  = `{"Mode":0,"ProxyPort":19099,"SystemProxy":false,"Decrypt":false,"Bypass":null,"Interface":"","Upstream":""}` + "\n"
	trafficProxy   = "http://127.0.0.1:19099"
	actionPress    = "press"
	paramAction    = "action"
	stateExpanded  = "expanded"
	stateChecked   = "checked"
	langCount      = 2 // English and Chinese: the languages both(...) returns
	readHeaderWait = 10 * time.Second
	linuxOS        = "linux"
	darwinOS       = "darwin"
	// openWait bounds the whole open step across its routes: a route that
	// cannot work must not hold the run for a poll deadline each.
	openWait = 40 * time.Second
	// curlRefused is curl's exit code for a failed connection: what the proxy
	// port answers when the app read another port from its settings.
	curlRefused = 7
)

// Waits and file modes of the Traffic scenario.
const (
	trafficWait = 20 * time.Second
	// slowWait is the deadline on Linux: its runner draws in software, so a
	// frame takes seconds there.
	slowWait    = 60 * time.Second
	trafficPoll = 500 * time.Millisecond
	curlWait    = 30 * time.Second
	privateDir  = 0o700
	privateFile = 0o600
	shotDir     = 0o755
)

// Names the scenario looks for.
const (
	helloPath      = "/hello"
	helloBody      = "hello"
	curlUnix       = "curl"
	curlWindows    = "curl.exe"
	sidebarProgram = "curl"
	flowPrefix     = "#"
	noProgramQuery = "program:nothing"
	shotIdle       = "traffic-idle.png"
	shotFlows      = "traffic-flows.png"
	shotFilter     = "traffic-filter.png"
	shotSettings   = "traffic-settings.png"
)

var errTimeout = errors.New("timed out")

// rect is an element's bounds in window coordinates.
type rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// want is an element the toolbar must show, found by one of its labels.
type want struct {
	name   string
	role   string
	labels []string
}

// prepareTraffic makes the app at exe portable, then writes the Traffic
// settings the scenario starts from into the data folder beside it. The
// driver writes there directly, so the app reads exactly these settings
// without the driver reproducing the platform's application data lookup.
func prepareTraffic(exe string) error {
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	dir := filepath.Dir(resolved)
	if err := os.WriteFile(filepath.Join(dir, portableMarker), nil, privateFile); err != nil {
		return err
	}
	cfg := filepath.Join(dir, portableData, trafficFolder)
	if err := os.MkdirAll(cfg, privateDir); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfg, configFileName), []byte(trafficConfig), privateFile)
}

// pollUntil calls check every interval until it reports true, fails, or
// timeout passes. It returns how long the wait took.
func pollUntil(timeout, interval time.Duration, check func() (bool, error)) (time.Duration, error) {
	start := time.Now()
	for {
		ok, err := check()
		if err != nil || ok {
			return time.Since(start), err
		}
		if time.Since(start) >= timeout {
			return time.Since(start), errTimeout
		}
		time.Sleep(interval)
	}
}

// missingAll names the wants no element of the tree satisfies.
func missingAll(nodes []node, wants []want) []string {
	var missing []string
	for _, w := range wants {
		if _, ok := findNode(nodes, w.role, w.labels); !ok {
			missing = append(missing, w.name)
		}
	}
	return missing
}

// settingsX is the x of the Traffic Settings button, which must not move
// when the capture starts.
func settingsX(nodes []node) (float64, error) {
	n, ok := findNode(nodes, roleButton, both(i18n.Messages.TrafficSettings))
	if !ok || n.Bounds == nil {
		return 0, fmt.Errorf("traffic settings button: %w", errNoNode)
	}
	return n.Bounds.X, nil
}

// matchFlow reports whether a flow row for the request to want is listed.
func matchFlow(nodes []node, want string) bool {
	for _, n := range nodes {
		if n.Role == roleButton && strings.HasPrefix(n.Label, flowPrefix) && strings.Contains(n.Label, want) {
			return true
		}
	}
	return false
}

// flowRows lists the labels of the flow rows shown.
func flowRows(nodes []node) []string {
	var rows []string
	for _, n := range nodes {
		if n.Role == roleButton && strings.HasPrefix(n.Label, flowPrefix) {
			rows = append(rows, n.Label)
		}
	}
	return rows
}

// programRow finds the sidebar row of the curl program, whose label is the
// program's name, case aside.
func programRow(nodes []node) (node, bool) {
	for _, n := range nodes {
		if n.Role == roleButton && strings.HasPrefix(strings.ToLower(n.Label), sidebarProgram) {
			return n, true
		}
	}
	return node{}, false
}

// curlName is the curl binary on this system; Windows ships it as curl.exe.
func curlName() (string, error) {
	name := curlUnix
	if runtime.GOOS == "windows" {
		name = curlWindows
	}
	if _, err := exec.LookPath(name); err != nil {
		return "", fmt.Errorf("the traffic scenario needs %s on PATH: %w", name, err)
	}
	return name, nil
}

// errorTitles are the banner titles the Traffic tool shows for a failure;
// seeing one after an action means the action failed.
func errorTitles() []string {
	titles := []func(i18n.Messages) string{
		i18n.Messages.TrafficErrTitle,
		i18n.Messages.TrafficErrPermissionTitle,
		i18n.Messages.TrafficErrDriverTitle,
		i18n.Messages.TrafficErrPortTitle,
		i18n.Messages.TrafficErrCertTitle,
		i18n.Messages.TrafficErrUnsupportedTitle,
		i18n.Messages.TrafficErrNotRunningTitle,
	}
	out := make([]string, 0, len(titles)*langCount)
	for _, m := range titles {
		out = append(out, both(m)...)
	}
	return out
}

// trafficRun carries the observations and failures of one scenario run.
type trafficRun struct {
	c        *client
	win      int
	shots    string
	addr     string
	obs      map[string]any
	waits    map[string]string
	failures []string
}

// runTraffic runs the Traffic scenario against the app's main window. shots
// is the directory for the window captures; empty takes none.
func runTraffic(c *client, win int, shots string) (map[string]any, []string) {
	r := &trafficRun{c: c, win: win, shots: shots, obs: map[string]any{}, waits: map[string]string{}}
	r.obs["window_content"] = windowContent(c, win)
	r.run()
	r.obs["waits"] = r.waits
	r.obs["failures"] = r.failures
	return r.obs, r.failures
}

// windowContent is the content size of window win, so each runner's record
// shows the window the sidebar was laid out in. It is nil when the window
// list does not answer.
func windowContent(c *client, win int) any {
	r, err := c.call("windows", nil)
	if err != nil {
		return nil
	}
	var w struct {
		Windows []struct {
			ID            int   `json:"id"`
			ContentBounds *rect `json:"contentBounds"`
		}
	}
	if json.Unmarshal(r, &w) != nil {
		return nil
	}
	for _, x := range w.Windows {
		if x.ID == win {
			return x.ContentBounds
		}
	}
	return nil
}

// run executes the steps in order. A failed step stops the run, after
// which the capture is stopped on a best-effort basis so the app is left
// as it was started.
func (r *trafficRun) run() {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"open", r.open},
		{"guide", r.checkGuide},
		{"toolbar", r.toolbar},
		{"start", r.start},
		{"flows", r.flows},
		{"filter", r.filter},
		{"settings", r.settings},
		{"stop", r.stop},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			r.failures = append(r.failures, s.name+": "+err.Error())
			_ = r.c.press(r.win, roleButton, both(i18n.Messages.Stop)...)
			return
		}
	}
}

// waitBudget is the deadline of one poll. Linux draws in software and takes
// seconds per frame, so it gets the longer deadline.
func waitBudget() time.Duration {
	if runtime.GOOS == linuxOS {
		return slowWait
	}
	return trafficWait
}

// wait polls the check for the named step and records how long it took.
func (r *trafficRun) wait(name string, check func() (bool, error)) error {
	elapsed, err := pollUntil(waitBudget(), trafficPoll, check)
	r.waits[name] = elapsed.String()
	return err
}

// shot captures the window to name in the shots directory, if one was given.
func (r *trafficRun) shot(name string) error {
	if r.shots == "" {
		return nil
	}
	r.c.idle(r.win)
	res, err := r.c.call("capture", map[string]any{paramWindow: r.win})
	if err != nil {
		return err
	}
	var img struct {
		PNG string `json:"png"`
	}
	if err := json.Unmarshal(res, &img); err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(img.PNG)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(r.shots, shotDir); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.shots, name), data, privateFile)
}

// errConfigNotRead is the failure when the app did not read the prepared
// settings.
var errConfigNotRead = errors.New("config not read: the app is not using the prepared data folder")

// menu chooses an item of the application menu by the labels along path, as
// the menu bar does. The protocol's menu call takes no window.
func (c *client) menu(path []string) error {
	_, err := c.call("menu", map[string]any{"path": path})
	return err
}

// open reaches the Traffic tool and records which route worked. The sidebar
// may fold the Traffic row out of the tree (a short window clips it), so the
// routes run in order: the sidebar row, the Network header that unfolds it,
// the app's menu bar, the macOS keyboard shortcut, and the command palette.
// They share one deadline, so a route that cannot work does not hold the run.
func (r *trafficRun) open() error {
	deadline := time.Now().Add(openWait)
	routes := []openRoute{
		{"sidebar", func() error {
			return r.c.press(r.win, roleButton, both(i18n.Messages.TrafficName)...)
		}},
		{"network-header", r.openFromNetwork},
		{"menu", r.openFromMenu},
	}
	if runtime.GOOS == darwinOS {
		routes = append(routes, openRoute{"keyboard", r.openFromKeyboard})
	}
	routes = append(routes, openRoute{"palette", r.openFromPalette})
	var failures []string
	for _, rt := range routes {
		if time.Now().After(deadline) {
			failures = append(failures, rt.name+": out of time")
			break
		}
		err := rt.fn()
		if err == nil {
			r.obs["open_path"] = rt.name
			return nil
		}
		failures = append(failures, rt.name+": "+err.Error())
	}
	r.obs["open_failures"] = failures
	r.obs["open_tree"] = r.treeSnapshot()
	return fmt.Errorf("open: no route reached Traffic: %s", strings.Join(failures, "; "))
}

// openRoute is one way to reach the Traffic tool, with its name in the JSON.
type openRoute struct {
	name string
	fn   func() error
}

// openFromMenu chooses Tools > Traffic from the app's menu bar. It does not
// depend on how much of the sidebar the window shows.
func (r *trafficRun) openFromMenu() error {
	for _, tools := range both(i18n.Messages.MenuTools) {
		for _, name := range both(i18n.Messages.TrafficName) {
			if err := r.c.menu([]string{tools, name}); err == nil {
				return nil
			}
		}
	}
	return errors.New("no Tools > Traffic menu item was chosen")
}

// treeSnapshot is the window's full tree as roles and labels, for a failure
// that must show what was on screen.
func (r *trafficRun) treeSnapshot() []string {
	nodes, err := r.c.treeErr(r.win)
	if err != nil {
		return []string{"tree: " + err.Error()}
	}
	return fullTree(nodes)
}

// openFromNetwork unfolds the Network category and presses the Traffic row.
func (r *trafficRun) openFromNetwork() error {
	if err := r.pressHeader(both(i18n.Messages.CategoryNetwork)); err != nil {
		return err
	}
	return r.c.press(r.win, roleButton, both(i18n.Messages.TrafficName)...)
}

// pressHeader unfolds the sidebar category whose header starts with one of
// labels. A category that is already expanded is left alone: pressing its
// header would fold it again.
func (r *trafficRun) pressHeader(labels []string) error {
	nodes, err := r.c.treeErr(r.win)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if n.Role != roleButton || !hasPrefixOf(n.Label, labels) {
			continue
		}
		if slices.Contains(n.States, stateExpanded) {
			return nil
		}
		return r.c.pressNode(r.win, n.ID)
	}
	return fmt.Errorf("sidebar category %v: %w", labels, errNoNode)
}

// openFromPalette types the tool's name into the command palette and presses
// the result. Each language's name is tried in turn, since the palette
// matches the names of the app's current language.
func (r *trafficRun) openFromPalette() error {
	// The palette may be a button that opens its field, so this press is optional.
	_ = r.c.press(r.win, roleButton, both(i18n.Messages.PaletteHint)...)
	return r.openPaletteResult()
}

// openFromKeyboard opens the command palette with its shortcut, then runs the
// same search. The shortcut is tried first on macOS, where the button path
// was seen to leave the palette open.
func (r *trafficRun) openFromKeyboard() error {
	if err := r.sendKey(paletteKey); err != nil {
		return err
	}
	r.c.idle(r.win)
	return r.openPaletteResult()
}

// openPaletteResult searches the palette for the Traffic name in each language
// and presses the result.
func (r *trafficRun) openPaletteResult() error {
	var last error
	for _, name := range both(i18n.Messages.TrafficName) {
		if err := r.setField(both(i18n.Messages.PaletteHint), name); err != nil {
			last = err
			continue
		}
		if err := r.c.press(r.win, roleButton, both(i18n.Messages.TrafficName)...); err != nil {
			last = err
			continue
		}
		return r.closePalette()
	}
	return last
}

// closePalette presses Escape in case the palette is still open, then polls
// until its search field is gone, so the toolbar is read from the page.
func (r *trafficRun) closePalette() error {
	if err := r.sendKey(escapeKey); err != nil {
		return err
	}
	return r.wait("palette_closed", func() (bool, error) {
		nodes, err := r.c.treeErr(r.win)
		if err != nil {
			return false, err
		}
		r.c.idle(r.win)
		return !hasLabel(nodes, both(i18n.Messages.PaletteHint)), nil
	})
}

// sendKey presses a key chord in the window through the input method, so the
// driver reaches the same shortcut a person would.
func (r *trafficRun) sendKey(chord string) error {
	_, err := r.c.call("input", map[string]any{paramWindow: r.win, "events": []map[string]string{{"type": "key", "key": chord}}})
	return err
}

// hasLabel reports whether a node with one of labels is in nodes.
func hasLabel(nodes []node, labels []string) bool {
	for _, n := range nodes {
		if slices.Contains(labels, n.Label) {
			return true
		}
	}
	return false
}

// fullTree lists every node's role and label, for a failure that must show
// what was on screen.
func fullTree(nodes []node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Role+":"+n.Label)
	}
	return out
}

// checkGuide fails when the first-run guide ticks its system-proxy step: the
// app then read SystemProxy=true from settings the driver did not prepare.
// The guide shows only on a first run, so its absence passes.
func (r *trafficRun) checkGuide() error {
	nodes, err := r.c.treeErr(r.win)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if slices.Contains(both(i18n.Messages.TrafficGuideProxy), n.Label) && slices.Contains(n.States, stateChecked) {
			return errConfigNotRead
		}
	}
	return nil
}

// pressNode presses the node with id, for rows a label lookup cannot tell
// apart.
func (c *client) pressNode(win int, id string) error {
	_, err := c.call("act", map[string]any{paramWindow: win, "id": id, paramAction: actionPress})
	c.idle(win)
	return err
}

// hasPrefixOf reports whether s starts with one of prefixes.
func hasPrefixOf(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// labelList lists the labels of the buttons and fields in nodes, so a failed
// wait can say what was on screen.
func labelList(nodes []node) []string {
	var out []string
	for _, n := range nodes {
		if (n.Role == roleButton || n.Role == roleField) && n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}

// toolbarWants are the controls the Traffic page must show before a capture.
func toolbarWants() []want {
	return []want{
		{"record", roleButton, both(i18n.Messages.SwitchStart)},
		{"mode", "", both(i18n.Messages.TrafficModeProxy)},
		{"search", roleField, both(i18n.Messages.TrafficSearch)},
		{"settings", roleButton, both(i18n.Messages.TrafficSettings)},
	}
}

// toolbar waits for the Traffic page to show its controls, then captures it.
func (r *trafficRun) toolbar() error {
	var seen []node
	err := r.wait("toolbar", func() (bool, error) {
		nodes, err := r.c.treeErr(r.win)
		if err != nil {
			return false, err
		}
		r.c.idle(r.win)
		seen = nodes
		return len(missingAll(nodes, toolbarWants())) == 0, nil
	})
	if err != nil {
		r.obs["toolbar_tree"] = fullTree(seen)
		return fmt.Errorf("toolbar lacks %v, saw %q: %w", missingAll(seen, toolbarWants()), labelList(seen), err)
	}
	r.obs["toolbar_found"] = len(toolbarWants())
	if err := r.shot(shotIdle); err != nil {
		return err
	}
	return nil
}

// start presses Start and checks that the toolbar does not move while the
// capture runs.
func (r *trafficRun) start() error {
	nodes, err := r.c.treeErr(r.win)
	if err != nil {
		return err
	}
	before, err := settingsX(nodes)
	if err != nil {
		return err
	}
	r.obs["settings_x_before"] = before
	if err := r.c.press(r.win, roleButton, both(i18n.Messages.SwitchStart)...); err != nil {
		return err
	}
	err = r.wait("start", func() (bool, error) {
		nodes, err := r.c.treeErr(r.win)
		if err != nil {
			return false, err
		}
		r.c.idle(r.win)
		_, ok := findNode(nodes, roleButton, both(i18n.Messages.Stop))
		return ok, nil
	})
	if err != nil {
		return err
	}
	nodes, err = r.c.treeErr(r.win)
	if err != nil {
		return err
	}
	after, err := settingsX(nodes)
	if err != nil {
		return err
	}
	r.obs["settings_x_after"] = after
	if before != after {
		return fmt.Errorf("the settings button moved from x=%v to x=%v when the capture started", before, after)
	}
	return nil
}

// flows sends one request through the proxy with the real curl and checks
// that its flow shows up in the list.
func (r *trafficRun) flows() error {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(helloBody)) }),
		ReadHeaderTimeout: readHeaderWait,
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()
	body, err := r.curl("http://" + ln.Addr().String() + helloPath)
	if err != nil {
		return err
	}
	r.obs["curl_body"] = body
	r.addr = ln.Addr().String() + helloPath
	err = r.wait("flow", func() (bool, error) {
		nodes, err := r.c.treeErr(r.win)
		if err != nil {
			return false, err
		}
		r.c.idle(r.win)
		r.obs["rows_seen"] = flowRows(nodes)
		return matchFlow(nodes, r.addr), nil
	})
	if err != nil {
		return err
	}
	return r.shot(shotFlows)
}

// curl fetches url through the proxy and returns the body it got back.
func (r *trafficRun) curl(url string) (string, error) {
	bin, err := curlName()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), curlWait)
	defer cancel()
	//nolint:gosec // curl is the fixed binary found on PATH; the URL is our own loopback server.
	out, err := exec.CommandContext(ctx, bin, "-s", "-x", trafficProxy, url).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == curlRefused {
			return "", fmt.Errorf("%w: nothing listens on %s: %w", errConfigNotRead, trafficProxy, err)
		}
		return "", fmt.Errorf("curl through the proxy: %w", err)
	}
	got := strings.TrimSpace(string(out))
	if got != helloBody {
		return got, fmt.Errorf("curl got %q through the proxy, want %q", got, helloBody)
	}
	return got, nil
}

// filter selects the curl program, then narrows the list with a search that
// matches nothing and clears it again.
func (r *trafficRun) filter() error {
	nodes, err := r.c.treeErr(r.win)
	if err != nil {
		return err
	}
	row, ok := programRow(nodes)
	if !ok {
		return fmt.Errorf("sidebar program row %q: %w", sidebarProgram, errNoNode)
	}
	if err := r.c.pressNode(r.win, row.ID); err != nil {
		return err
	}
	if err := r.expectFlow("flow after program", true); err != nil {
		return err
	}
	if err := r.setField(both(i18n.Messages.TrafficSearch), noProgramQuery); err != nil {
		return err
	}
	if err := r.expectFlow("flow hidden by search", false); err != nil {
		return err
	}
	if err := r.setField(both(i18n.Messages.TrafficSearch), ""); err != nil {
		return err
	}
	if err := r.expectFlow("flow back after clearing search", true); err != nil {
		return err
	}
	return r.shot(shotFilter)
}

// setField types text into the search field, trying each label in turn.
func (r *trafficRun) setField(labels []string, text string) error {
	var err error
	for _, label := range labels {
		if err = r.c.setValue(r.win, label, text); err == nil {
			r.c.idle(r.win)
			return nil
		}
	}
	return err
}

// expectFlow waits until the flow row of the request is listed (or gone).
func (r *trafficRun) expectFlow(name string, shown bool) error {
	return r.wait(name, func() (bool, error) {
		nodes, err := r.c.treeErr(r.win)
		if err != nil {
			return false, err
		}
		r.c.idle(r.win)
		return matchFlow(nodes, r.addr) == shown, nil
	})
}

// settings opens Traffic Settings and checks the capture section, copies the
// proxy environment, and closes the dialog. Open Terminal is never pressed:
// it would open a terminal window.
func (r *trafficRun) settings() error {
	if err := r.c.press(r.win, roleButton, both(i18n.Messages.TrafficSettings)...); err != nil {
		return err
	}
	err := r.wait("settings", func() (bool, error) {
		nodes, err := r.c.treeErr(r.win)
		if err != nil {
			return false, err
		}
		r.c.idle(r.win)
		_, term := findNode(nodes, roleButton, both(i18n.Messages.TrafficOpenTerminal))
		_, env := findNode(nodes, roleButton, both(i18n.Messages.TrafficCopyEnv))
		return term && env, nil
	})
	if err != nil {
		return err
	}
	if err := r.c.press(r.win, roleButton, both(i18n.Messages.TrafficCopyEnv)...); err != nil {
		return err
	}
	if err := r.checkNoError(); err != nil {
		return err
	}
	if err := r.shot(shotSettings); err != nil {
		return err
	}
	return r.c.press(r.win, roleButton, both(i18n.Messages.TrafficSettingsDone)...)
}

// checkNoError fails when the Traffic tool shows an error banner.
func (r *trafficRun) checkNoError() error {
	nodes, err := r.c.treeErr(r.win)
	if err != nil {
		return err
	}
	r.c.idle(r.win)
	for _, title := range errorTitles() {
		for _, n := range nodes {
			if n.Label == title {
				return fmt.Errorf("the Traffic tool shows the error %q", title)
			}
		}
	}
	return nil
}

// stop presses Stop and waits until Start is back.
func (r *trafficRun) stop() error {
	if err := r.c.press(r.win, roleButton, both(i18n.Messages.Stop)...); err != nil {
		return err
	}
	return r.wait("stop", func() (bool, error) {
		nodes, err := r.c.treeErr(r.win)
		if err != nil {
			return false, err
		}
		r.c.idle(r.win)
		_, ok := findNode(nodes, roleButton, both(i18n.Messages.SwitchStart))
		return ok, nil
	})
}
