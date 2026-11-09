// Command e2e drives a running e2e build of the desktop app through MyGo's
// automation plugin, which sees the app as a screen reader does, and
// checks its tools end to end: the sidebar's accessible names, Keep Awake
// really holding a power assertion, Ports listing listeners, Color
// converting a color and the Traffic capture running through the proxy.
//
// The CI job first writes the Traffic settings the scenario starts from into
// the app's configuration folder, then drives the app:
//
//	go run ./dev/e2e -prepare "$XDG_CONFIG_HOME"   # or "$HOME/Library/Application Support", "%APPDATA%"
//	go build -tags e2e -o omt-e2e ./cmd/oh-my-toys
//	MYGO_AUTOMATION=/tmp/omt.sock ./omt-e2e &
//	go run ./dev/e2e -socket /tmp/omt.sock -out e2e-result.json -shots shots
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
)

// Waits of the run.
const (
	dialTries   = 100
	dialPause   = 100 * time.Millisecond
	idleTimeout = 5000 // milliseconds
	callTimeout = 30 * time.Second
	planPause   = 1500 * time.Millisecond
	powerPause  = 300 * time.Millisecond
)

// paramWindow is the protocol's parameter naming a window.
const paramWindow = "window"

// Protocol roles the driver looks for.
const (
	roleButton = "button"
	roleField  = "textField"
)

var errNoNode = errors.New("no such element")

type client struct {
	conn net.Conn
	r    *bufio.Reader
	n    int
}

func (c *client) call(method string, params any) (json.RawMessage, error) {
	// A deadline on every call keeps a wedged app from hanging the run; it
	// is longer than any idle wait the driver asks for.
	if err := c.conn.SetDeadline(time.Now().Add(callTimeout)); err != nil {
		return nil, err
	}
	c.n++
	line, err := json.Marshal(map[string]any{"id": c.n, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	answer, err := c.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var res struct {
		Result json.RawMessage
		Error  *struct{ Code, Message string }
	}
	if err := json.Unmarshal(answer, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, fmt.Errorf("%s: %s %s", method, res.Error.Code, res.Error.Message)
	}
	return res.Result, nil
}

type node struct {
	ID     string   `json:"id"`
	Parent string   `json:"parent"`
	Role   string   `json:"role"`
	Label  string   `json:"label"`
	Value  string   `json:"value"`
	States []string `json:"states"`
	Bounds *rect    `json:"bounds"`
}

func (c *client) treeErr(win int) ([]node, error) {
	r, err := c.call("tree", map[string]any{paramWindow: win})
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}
	var t struct{ Nodes []node }
	if err := json.Unmarshal(r, &t); err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}
	return t.Nodes, nil
}

func (c *client) tree(win int) []node {
	nodes, err := c.treeErr(win)
	if err != nil {
		log.Fatal(err)
	}
	return nodes
}

func (c *client) idle(win int) {
	if _, err := c.call("idle", map[string]any{paramWindow: win, "timeout": idleTimeout}); err != nil {
		log.Print(err)
	}
}

// press presses the first element of a role named by any of labels.
func (c *client) press(win int, role string, labels ...string) error {
	nodes, err := c.treeErr(win)
	if err != nil {
		return err
	}
	n, ok := findNode(nodes, role, labels)
	if !ok {
		return fmt.Errorf("%s %v: %w", role, labels, errNoNode)
	}
	_, err = c.call("act", map[string]any{paramWindow: win, "id": n.ID, paramAction: actionPress})
	c.idle(win)
	return err
}

func (c *client) setValue(win int, label, text string) error {
	nodes, err := c.treeErr(win)
	if err != nil {
		return err
	}
	n, ok := findNode(nodes, roleField, []string{label})
	if !ok {
		return fmt.Errorf("text field %q: %w", label, errNoNode)
	}
	_, err = c.call("act", map[string]any{paramWindow: win, "id": n.ID, paramAction: "setValue", "text": text})
	c.idle(win)
	return err
}

// findNode returns the first element of the role (any role when role is
// empty) whose label is one of labels.
func findNode(nodes []node, role string, labels []string) (node, bool) {
	for _, n := range nodes {
		if (role == "" || n.Role == role) && slices.Contains(labels, n.Label) {
			return n, true
		}
	}
	return node{}, false
}

func contains(nodes []node, s string) bool {
	for _, n := range nodes {
		if strings.Contains(n.Label, s) || strings.Contains(n.Value, s) {
			return true
		}
	}
	return false
}

// both is a message in English and Chinese, whichever the app shows.
func both(m func(i18n.Messages) string) []string {
	return []string{m(i18n.In("en")), m(i18n.In("zh-Hans"))}
}

func dial(socket string) *client {
	var (
		conn net.Conn
		err  error
	)
	var d net.Dialer
	for range dialTries {
		if conn, err = d.DialContext(context.Background(), "unix", socket); err == nil {
			return &client{conn: conn, r: bufio.NewReader(conn)}
		}
		time.Sleep(dialPause)
	}
	log.Fatal(err)
	return nil
}

func mainWindow(c *client) int {
	r, err := c.call("windows", nil)
	if err != nil {
		log.Fatal(err)
	}
	var w struct {
		Windows []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		}
	}
	if err := json.Unmarshal(r, &w); err != nil {
		log.Fatal(err)
	}
	for _, x := range w.Windows {
		if x.Title == "oh-my-toys" {
			return x.ID
		}
	}
	log.Fatalf("no main window in %+v", w.Windows)
	return 0
}

func main() {
	socket := flag.String("socket", "/tmp/omt.sock", "the app's MYGO_AUTOMATION socket")
	out := flag.String("out", "e2e-result.json", "where to write the results")
	prepare := flag.String("prepare", "", "write the Traffic settings the scenario starts from under this configuration folder, then exit")
	shots := flag.String("shots", "", "directory for the window captures of the Traffic scenario; empty takes none")
	flag.Parse()
	if *prepare != "" {
		if err := prepareTraffic(*prepare); err != nil {
			log.Fatal(err)
		}
		return
	}
	c := dial(*socket)
	win := mainWindow(c)
	c.idle(win)
	result := map[string]any{"os": runtime.GOOS + "-" + runtime.GOARCH}
	result["sidebar"] = sidebar(c.tree(win))
	result["awake"] = checkAwake(c, win)
	result["ports"] = checkPorts(c, win)
	result["color"] = checkColor(c, win)
	traffic, failures := runTraffic(c, win, *shots)
	result["traffic"] = traffic
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, b, 0o600); err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
	if len(failures) > 0 {
		log.Fatalf("traffic scenario: %d failure(s), see traffic.failures in %s", len(failures), *out)
	}
}

func sidebar(nodes []node) []string {
	var out []string
	for _, n := range nodes {
		if n.Role == roleButton && n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}

func checkAwake(c *client, win int) map[string]string {
	res := map[string]string{}
	name := both(i18n.Messages.AwakeName)
	if err := c.press(win, roleButton, name...); err != nil {
		res["error"] = err.Error()
		return res
	}
	if err := c.press(win, "switch", name...); err != nil {
		res["error"] = err.Error()
		return res
	}
	time.Sleep(powerPause)
	res["on"] = assertions()
	if err := c.press(win, "switch", name...); err != nil {
		res["error"] = err.Error()
	}
	time.Sleep(powerPause)
	res["off"] = assertions()
	return res
}

func checkPorts(c *client, win int) map[string]any {
	if err := c.press(win, roleButton, both(i18n.Messages.PortsName)...); err != nil {
		return map[string]any{"error": err.Error()}
	}
	time.Sleep(planPause)
	c.idle(win)
	t := c.tree(win)
	listening := contains(t, "listening") || contains(t, "在监听")
	return map[string]any{"summary_shown": listening}
}

func checkColor(c *client, win int) map[string]any {
	if err := c.press(win, roleButton, both(i18n.Messages.ColorName)...); err != nil {
		return map[string]any{"error": err.Error()}
	}
	for _, label := range both(i18n.Messages.ColorFieldHint) {
		if c.setValue(win, label, "#0071e3") == nil {
			break
		}
	}
	time.Sleep(planPause)
	c.idle(win)
	return map[string]any{"rgb_shown": contains(c.tree(win), "rgb(0, 113, 227)")}
}

// assertions lists the power assertions or inhibitors of the app.
func assertions() string {
	ctx := context.Background()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "pmset", "-g", "assertions")
	case "windows":
		cmd = exec.CommandContext(ctx, "powercfg", "/requests")
	default:
		cmd = exec.CommandContext(ctx, "systemd-inhibit", "--list")
	}
	b, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "oh-my-toys") || strings.Contains(l, "omt") {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	return strings.Join(lines, "\n")
}
