// Package traffic records network traffic and lets a person look through
// it, rewrite it and send it again. It is the tool of docs/traffic.md: it
// owns the system proxy and the root certificate, keeps the settings and
// the rule book on disk, and drives the proxy, the packet reader, the log
// and the HAR export of its sub-packages.
package traffic

import (
	"context"
	"crypto/x509"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/flowlog"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/proxy"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/rules"
)

// ID is the tool's ID.
const ID = "traffic"

// The IDs of the quick actions.
const (
	idOn  = "on"
	idOff = "off"
)

// What each log keeps: enough to look through a session without letting a
// long capture fill the memory of the machine.
const (
	proxyFlows      = 5000
	packetFlows     = 50000
	proxyBodyBytes  = 256 << 20
	packetBodyBytes = 256 << 20
)

// loopback is where the proxy listens, and the only address it may use.
const loopback = "127.0.0.1"

// Tool is the traffic capture. Its zero value is not usable; use New.
type Tool struct {
	dir   string // the tool's own folder
	caDir string // the root certificate and its key

	sys       system
	proxylog  *flowlog.Log
	packetlog *flowlog.Log
	rules     *rules.Engine
	// rootCAs verifies the servers a capture reaches; nil means the
	// system's roots. Only a test sets another pool.
	rootCAs *x509.CertPool

	// opMu serialises the long actions - starting, stopping and applying
	// settings - so that two of them never change the system at once.
	opMu sync.Mutex
	// rulesMu serialises setting the rule book and saving it, which the
	// editor does on every keystroke.
	rulesMu sync.Mutex

	// mu guards what Status reads, and is never held across I/O.
	mu     sync.Mutex
	cfg    tool.CaptureConfig
	proxy  *proxyRun
	packet *packetRun
	since  time.Time
	note   string // the failure that ended the last capture, if any
}

// The tool is what the inspector draws.
var _ tool.Traffic = (*Tool)(nil)

// rootPolicy says whether a capture may make the root certificate, or has
// to do without one it cannot find.
type rootPolicy int

const (
	// rootRefuse leaves HTTPS tunnelled when there is no root: removing the
	// root must not make another secret behind the person's back.
	rootRefuse rootPolicy = iota
	// rootMake makes a root when there is none, which is what starting a
	// capture does.
	rootMake
)

// proxyRun is a running proxy-mode capture.
type proxyRun struct {
	srv        *proxy.Server
	addr       string
	decrypting bool
	cancel     context.CancelFunc
}

// New makes the tool, keeping its state under dir/traffic.
func New(dir string) *Tool { return newTool(dir, realSystem()) }

// newTool makes a tool over the calls it makes outside its own package,
// so that a test can replace them.
func newTool(dir string, sys system) *Tool {
	t := &Tool{
		dir:       filepath.Join(dir, trafficDir),
		sys:       sys,
		proxylog:  flowlog.New(proxyFlows, proxyBodyBytes),
		packetlog: flowlog.New(packetFlows, packetBodyBytes),
		rules:     rules.New(),
		cfg:       defaultConfig(),
	}
	t.caDir = filepath.Join(t.dir, caFolder)
	t.cfg = t.loadConfig()
	t.loadRules()
	return t
}

// Manifest describes the tool.
func (t *Tool) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:        ID,
		Name:      i18n.Messages.TrafficName,
		Summary:   i18n.Messages.TrafficSummary,
		Category:  tool.CategoryNetwork,
		Kind:      tool.KindSwitch,
		Icon:      "lan",
		Keywords:  []string{"capture", "proxy", "sniff", "packet", schemeHTTPProxy, "https", "mitm", "wireshark", "charles", "zhuabao", "抓包"},
		Platforms: []string{"macos", "windows", "linux"},
		Actions: []tool.QuickAction{
			{ID: idOn, Title: i18n.Messages.TrafficStart, Tray: true, Run: t.Start},
			{ID: idOff, Title: i18n.Messages.TrafficStop, Stops: true, Run: func(context.Context) error { return t.Stop() }},
		},
	}
}

// Specs declares the tool's actions: starting and stopping a capture
// changes the system proxy or reads the network, and status only reports.
func (t *Tool) Specs() []tool.Spec {
	return tool.SwitchSpecs(i18n.Messages.TrafficStart, i18n.Messages.TrafficStop, i18n.Messages.TrafficStatus)
}

// Config returns the settings the next capture uses.
func (t *Tool) Config() tool.CaptureConfig {
	t.mu.Lock()
	defer t.mu.Unlock()
	c := t.cfg
	c.Bypass = slices.Clone(c.Bypass)
	return c
}

// Log returns the log of the mode in Config: packets and proxied flows are
// kept apart, so a change of mode changes the log.
func (t *Tool) Log() tool.FlowLog {
	if t.Config().Mode == tool.ModePackets {
		return t.packetlog
	}
	return t.proxylog
}

// Start begins a capture with the settings used last, after putting back
// the system proxy a crashed run left behind.
func (t *Tool) Start(ctx context.Context) error {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	if err := t.recover(); err != nil {
		return err
	}
	if err := t.stopCapture(); err != nil {
		return err
	}
	return t.startCapture(ctx, t.Config(), rootMake)
}

// Stop ends the capture. The system proxy is put back first: a proxy left
// pointing at a dead port cuts the person off the network. Stopping a
// capture that does not run does nothing.
func (t *Tool) Stop() error {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	return t.stopCapture()
}

// Recover puts back the system proxy a previous run left behind, when the
// system still points at the capture's dead address. It is called at
// launch, before any capture starts, and does nothing when there is no
// leftover.
func (t *Tool) Recover() error {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	return t.recover()
}

// Status says whether a capture runs, what it has seen so far and why the
// last one ended. It reads no system settings and waits on no network, so
// the UI may call it every frame.
func (t *Tool) Status() tool.Status {
	t.mu.Lock()
	proxyOn, packets, since, note := t.proxy, t.packet, t.since, t.note
	t.mu.Unlock()

	switch {
	case proxyOn != nil:
		return tool.Status{
			On: true, Since: since, Note: note,
			Text:   i18n.TrafficRunningProxy(proxyOn.addr, t.proxylog.Len()),
			Detail: decryptDetail(proxyOn),
		}
	case packets != nil:
		return tool.Status{
			On: true, Since: since, Note: note,
			Text: i18n.TrafficRunningPackets(packets.iface, t.packetlog.Len()),
		}
	default:
		return tool.Status{Text: i18n.StatusOff(), Note: note}
	}
}

// decryptDetail says whether a running proxy opens HTTPS, or nothing when
// it tunnels it.
func decryptDetail(run *proxyRun) string {
	if run.decrypting {
		return i18n.TrafficDecrypt()
	}
	return ""
}

// running reports whether a capture is on.
func (t *Tool) running() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.proxy != nil || t.packet != nil
}

// startCapture starts the capture of the mode in cfg, making the root
// certificate first when the policy allows it and the config opens HTTPS.
func (t *Tool) startCapture(ctx context.Context, cfg tool.CaptureConfig, policy rootPolicy) error {
	if cfg.Mode == tool.ModePackets {
		return t.startPackets(ctx, cfg)
	}
	return t.startProxy(ctx, cfg, policy)
}

// stopCapture restores the system proxy and ends the capture in progress.
// A restore that fails leaves the capture running: stopping the proxy
// while the system points at it would cut the person off the network.
func (t *Tool) stopCapture() error {
	if err := t.restoreSystemProxy(); err != nil {
		return err
	}
	t.mu.Lock()
	run, packets := t.proxy, t.packet
	t.mu.Unlock()
	if run != nil {
		if err := run.srv.Stop(); err != nil {
			return err
		}
		run.cancel()
	}
	if packets != nil {
		packets.stop()
	}
	t.mu.Lock()
	t.proxy, t.packet, t.since = nil, nil, time.Time{}
	t.mu.Unlock()
	return nil
}
