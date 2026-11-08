package traffic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/platform/sysproxy"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/har"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/proxy"
)

// stateVersion is the version of sysproxy.json the tool writes.
const stateVersion = 1

// proxyState is what sysproxy.json holds while the capture has changed the
// system proxy: the settings it replaced, and the address it pointed them
// at. The address tells a leftover of a crash from a proxy the person set
// since.
type proxyState struct {
	Version  int               `json:"version"`
	Addr     string            `json:"addr"`
	Snapshot sysproxy.Snapshot `json:"snapshot"`
}

// startProxy runs the capture proxy. The order matters: the settings the
// system had are read first, the root certificate is made before anything
// listens, and the state file is written before the system proxy changes,
// so that a crash at any point can be undone.
func (t *Tool) startProxy(ctx context.Context, cfg tool.CaptureConfig, policy rootPolicy) error {
	var authority *ca.Authority
	var err error
	if cfg.Decrypt {
		if authority, err = t.root(policy); err != nil {
			return err
		}
	}
	var before *sysproxy.Snapshot
	if cfg.SystemProxy {
		now, err := t.systemProxyNow()
		if err != nil {
			return err
		}
		before = &now
	}
	where, err := chooseUpstream(cfg, before)
	if err != nil {
		return err
	}
	via, err := upstreamFor(where)
	if err != nil {
		return err
	}
	decrypting := cfg.Decrypt && authority != nil
	want := net.JoinHostPort(loopback, strconv.Itoa(cfg.ProxyPort))
	srv, err := proxy.New(proxy.Options{ //nolint:contextcheck // proxy.New takes no context: it only validates what it is given.
		Addr:          want,
		Authority:     authority,
		Decrypt:       decrypting,
		Bypass:        cfg.Bypass,
		Log:           t.proxylog,
		Rules:         t.rules,
		Dial:          via.dial,
		UpstreamProxy: via.proxy,
		RootCAs:       t.rootCAs,
	})
	if err != nil {
		return fmt.Errorf("make the proxy: %w", err)
	}
	// The capture outlives the call that started it, so it gets a context
	// of its own: the caller's ends when its action returns.
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if err := srv.Start(lifetime); err != nil {
		cancel()
		return err
	}
	run := &proxyRun{srv: srv, addr: srv.Addr(), decrypting: decrypting, cancel: cancel}
	// The capture counts as on from this moment: Status is what StopAll
	// reads before the app quits, and quitting while the system proxy is
	// being changed must still put it back.
	t.publish(run)
	if before != nil {
		if err := t.takeSystemProxy(run, *before); err != nil {
			t.unpublish(run)
			_ = srv.Stop()
			cancel()
			return err
		}
	}
	return nil
}

// publish makes a capture the one Status reports.
func (t *Tool) publish(run *proxyRun) {
	t.mu.Lock()
	t.proxy = run
	t.since = t.sys.now()
	t.note = ""
	t.mu.Unlock()
}

// unpublish takes a capture that could not be finished back down.
func (t *Tool) unpublish(run *proxyRun) {
	t.mu.Lock()
	if t.proxy == run {
		t.proxy, t.since = nil, time.Time{}
	}
	t.mu.Unlock()
}

// takeSystemProxy points the system proxy at the capture. The state file
// comes first, so that a crash after the change is undone at the next
// start: a change that fails keeps it, because recovery restores only the
// services that still point at the capture, and losing the file is what
// would really hurt. Once the change is made, the file is written again
// from the settings Enable actually replaced, so that a service that came
// up in between is restored too.
func (t *Tool) takeSystemProxy(run *proxyRun, before sysproxy.Snapshot) error {
	addr := run.addr
	if err := t.saveState(addr, before); err != nil {
		return err
	}
	replaced, err := t.sys.sysproxy.enable(addr)
	if err != nil {
		return systemError("point the system proxy at the capture", err)
	}
	if err := t.saveState(addr, mergeSnapshots(before, replaced)); err != nil {
		// The file of the first write is still on disk and still safe: a
		// failure to rewrite it must not undo a working capture.
		log.Printf("traffic: %v; the system proxy is restored from the first snapshot", err)
	}
	// Every service points at the capture now, but apps read the one of
	// the primary service, and a proxy app's virtual interface keeps its
	// own. The capture runs either way; the person is told that this
	// setting does not reach it, so that they can point a client at it.
	if effective, err := t.sys.sysproxy.effective(addr); err == nil && !effective {
		t.mu.Lock()
		run.ineffective = true
		t.mu.Unlock()
	}
	return nil
}

// mergeSnapshots combines the settings the system had before the change
// with the ones Enable reported it replaced: the second is what was
// actually changed, and the first keeps the services the second does not
// name.
func mergeSnapshots(before, changed sysproxy.Snapshot) sysproxy.Snapshot {
	merged := changed
	merged.Services = append([]sysproxy.Service(nil), changed.Services...)
	seen := make(map[string]struct{}, len(merged.Services))
	for _, service := range merged.Services {
		seen[service.Name] = struct{}{}
	}
	for _, service := range before.Services {
		if _, ok := seen[service.Name]; !ok {
			merged.Services = append(merged.Services, service)
		}
	}
	if merged.Windows == nil {
		merged.Windows = before.Windows
	}
	if merged.GNOME == nil {
		merged.GNOME = before.GNOME
	}
	return merged
}

// root is the root certificate the proxy signs with. A root that is there
// is used; one that is missing is made first when the policy allows it,
// and otherwise HTTPS is tunnelled, which is what removing the root leaves
// a running capture with until it is started again.
func (t *Tool) root(policy rootPolicy) (*ca.Authority, error) {
	a, err := ca.Load(t.caDir)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, ca.ErrNotFound) {
		return nil, fmt.Errorf("read the root certificate: %w", err)
	}
	if policy == rootRefuse {
		return nil, nil
	}
	a, err = ca.Create(t.caDir)
	if err != nil {
		return nil, fmt.Errorf("make the root certificate: %w", err)
	}
	return a, nil
}

// systemProxyNow reads the proxy settings the system has now.
func (t *Tool) systemProxyNow() (sysproxy.Snapshot, error) {
	s, err := t.sys.sysproxy.current()
	if err != nil {
		return sysproxy.Snapshot{}, systemError("read the system proxy", err)
	}
	return s, nil
}

// saveState writes the settings a running capture replaced.
func (t *Tool) saveState(addr string, s sysproxy.Snapshot) error {
	return writeJSON(t.path(stateFile), proxyState{Version: stateVersion, Addr: addr, Snapshot: s})
}

// loadState reads the settings a previous capture replaced. A missing file
// comes back as fs.ErrNotExist; a damaged one is moved aside and also
// reported as missing, so that a power cut cannot block every later action.
func (t *Tool) loadState() (proxyState, error) {
	path := t.path(stateFile)
	var st proxyState
	if err := readJSON(path, &st); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return proxyState{}, err
		}
		return proxyState{}, t.setAside(path, err)
	}
	if st.Addr == "" {
		return proxyState{}, t.setAside(path, fmt.Errorf("%s names no address", path))
	}
	return st, nil
}

// setAside moves a file the tool cannot use out of the way, keeping it for
// a look, and reports that there is nothing to read.
func (t *Tool) setAside(path string, cause error) error {
	broken := path + brokenSuffix
	log.Printf("traffic: %v; moved to %s", cause, broken)
	if err := os.Rename(path, broken); err != nil {
		return cause
	}
	return fs.ErrNotExist
}

// removeState forgets the settings a capture replaced, once they are back.
func (t *Tool) removeState() error {
	if err := os.Remove(t.path(stateFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", t.path(stateFile), err)
	}
	return nil
}

// restoreSystemProxy puts the system proxy back, and forgets the state
// file only once that worked: a restore that fails keeps it for the next
// try, because the system is still pointing at a proxy that is stopping.
func (t *Tool) restoreSystemProxy() error {
	st, err := t.loadState()
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := t.sys.sysproxy.restore(st.Snapshot); err != nil {
		return systemError("put the system proxy back", err)
	}
	return t.removeState()
}

// recover undoes the system proxy change a previous run left behind. It
// restores, of the saved settings, only the services that still point at
// the capture's address, and leaves the ones the person changed since. The
// state file is forgotten only when that worked; a failure keeps it, so a
// later try can finish the job.
func (t *Tool) recover() error {
	if t.running() {
		// A capture in progress wrote this state file itself, and gives
		// the settings back when it stops.
		return nil
	}
	st, err := t.loadState()
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := t.sys.sysproxy.restorePointing(st.Snapshot, st.Addr); err != nil {
		log.Printf("traffic: put the system proxy back: %v", err)
		return systemError("put the system proxy back", err)
	}
	return t.removeState()
}

// Replay sends the request of a flow again through the running proxy, so
// that the rules and the upstream apply as they did, and returns the ID of
// the new flow.
func (t *Tool) Replay(ctx context.Context, id uint64) (uint64, error) {
	t.mu.Lock()
	run := t.proxy
	t.mu.Unlock()
	if run == nil {
		return 0, ErrNotRunning
	}
	flow, ok := t.proxylog.Get(id)
	if !ok {
		return 0, fmt.Errorf("flow %d is no longer recorded", id)
	}
	req, err := proxy.Request(flow) //nolint:contextcheck // proxy.Request builds a request and takes no context.
	if err != nil {
		return 0, err
	}
	newID, err := run.srv.Do(ctx, req, id)
	if err != nil {
		return 0, fmt.Errorf("send flow %d again: %w", id, err)
	}
	return newID, nil
}

// ExportHAR writes the proxied flows of ids, or all of them when ids is
// empty, as a HAR 1.2 file.
func (t *Tool) ExportHAR(w io.Writer, ids []uint64) error {
	flows := t.flowsOf(ids)
	if err := har.Write(w, flows); err != nil {
		return fmt.Errorf("write the HAR file: %w", err)
	}
	return nil
}

// flowsOf reads the flows of ids, or every proxied flow when ids is empty,
// leaving out the ones the log has already evicted.
func (t *Tool) flowsOf(ids []uint64) []tool.Flow {
	if len(ids) == 0 {
		for _, row := range t.proxylog.Rows(tool.Filter{Source: tool.SourceProxy}) {
			ids = append(ids, row.ID)
		}
	}
	flows := make([]tool.Flow, 0, len(ids))
	for _, id := range ids {
		if flow, ok := t.proxylog.Get(id); ok {
			flows = append(flows, flow)
		}
	}
	return flows
}
