package shell

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// cfgDelay is how long typing in a field may rest before the capture is
// asked to use it: every change restarts a capture in progress.
const cfgDelay = 500 * time.Millisecond

// loadConfig fills the controls from what the tool has now.
func (st *trafficState) loadConfig(cfg tool.CaptureConfig) {
	if cfg.ProxyPort > 0 {
		st.port = strconv.Itoa(cfg.ProxyPort)
	}
	st.bypass = strings.Join(cfg.Bypass, ", ")
	st.upstream = cfg.Upstream
	st.systemProxy = cfg.SystemProxy
	st.decrypt = cfg.Decrypt
	st.mode = int(cfg.Mode)
	st.iface = cfg.Interface
}

// captureConfig is the tool's settings with what the fields hold now. ok
// is false while a field cannot be used, so nothing is applied yet.
func (st *trafficState) captureConfig(t tool.Traffic) (tool.CaptureConfig, bool) {
	cfg := t.Config()
	cfg.Mode = tool.CaptureMode(st.mode)
	cfg.SystemProxy, cfg.Decrypt = st.systemProxy, st.decrypt
	cfg.Bypass = splitList(st.bypass)
	cfg.Interface = st.iface
	up, ok := parseUpstream(st.upstream)
	st.upBad = !ok
	cfg.Upstream = up
	port, err := strconv.Atoi(strings.TrimSpace(st.port))
	if err == nil && port >= trafficPortMin && port <= trafficPortMax {
		cfg.ProxyPort = port
	}
	// An empty field keeps the port the tool had; anything else must be a
	// port it may listen on.
	st.portBad = st.port != "" && !portValid(st.port)
	return cfg, !st.portBad && ok
}

// trafficControls is the mode and its settings: the proxy's port, system
// proxy, decryption, bypass list and upstream proxy, or the interface
// packets are read from.
func (a *App) trafficControls(c *ui.Context, t tool.Traffic, st *trafficState) {
	a.trafficConfigDue(c, t, st)
	if t.Config().Mode == tool.ModePackets && !st.ifacesLoaded {
		a.trafficLoadInterfaces(t, st)
	}
	comp.Card(c).FillWidth().Padding(ds.Space12).Gap(ds.Space10).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space16).AlignItems(ui.End).Wrap().Children(func() {
			modes := []string{i18n.TrafficModeProxy(), i18n.TrafficModePackets()}
			if comp.Segmented(c.Key("traffic-mode"), &st.mode, modes, i18n.ModeLabel()).Changed() {
				a.trafficApplyNow(t, st)
			}
			if st.mode == int(tool.ModeProxy) {
				a.trafficProxyToggles(c, t, st)
				return
			}
			a.trafficPacketControls(c, t, st)
		})
		if st.mode == int(tool.ModeProxy) {
			a.trafficProxyFields(c, st)
		}
		a.trafficCertRow(c, t, st)
	})
}

// trafficProxyToggles are the proxy's port and the two switches, which
// apply at once.
func (a *App) trafficProxyToggles(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Box(c).Width(ds.SelectMinWidth).Children(func() {
		st.portBad = st.port != "" && !portValid(st.port)
		o := comp.FieldOpts{Label: i18n.TrafficPort(), Mono: true}
		if st.portBad {
			o.Error = i18n.TrafficPortBad(st.port)
		}
		if comp.TextField(c.Key("traffic-port"), &st.port, o).Changed() {
			st.markConfig()
		}
	})
	a.trafficSwitch(c, "traffic-sysproxy", &st.systemProxy, i18n.TrafficSystemProxy(), t, st)
	a.trafficSwitch(c, "traffic-decrypt", &st.decrypt, i18n.TrafficDecrypt(), t, st)
}

// trafficProxyFields are the two lists the proxy keeps, which apply once
// typing has rested.
func (a *App) trafficProxyFields(c *ui.Context, st *trafficState) {
	ui.Row(c).FillWidth().Gap(ds.Space12).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Grow(1).Basis(ds.SearchMinWidth).Children(func() {
			o := comp.FieldOpts{Label: i18n.TrafficBypass(), Placeholder: i18n.TrafficBypassHint(), Mono: true}
			if comp.TextField(c.Key("traffic-bypass"), &st.bypass, o).Changed() {
				st.markConfig()
			}
		})
		ui.Box(c).Grow(1).Basis(ds.SearchMinWidth).Children(func() {
			o := comp.FieldOpts{Label: i18n.TrafficUpstream(), Placeholder: i18n.TrafficUpstreamPlaceholder(), Help: i18n.TrafficUpstreamHint(), Mono: true}
			if _, ok := parseUpstream(st.upstream); !ok {
				o.Help, o.Error = "", i18n.TrafficUpstreamBad(st.upstream)
			}
			if comp.TextField(c.Key("traffic-upstream"), &st.upstream, o).Changed() {
				st.markConfig()
			}
		})
	})
}

// trafficSwitch is a labelled switch that applies at once, since there is
// nothing to type.
func (a *App) trafficSwitch(c *ui.Context, key string, v *bool, label string, t tool.Traffic, st *trafficState) {
	ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		comp.Text(c, label, ds.Label, ds.P(c).TextPrimary).SingleLine()
		if comp.Switch(c.Key(key), v, label).Changed() {
			a.trafficApplyNow(t, st)
		}
	})
}

// trafficPacketControls is the interface packets are read from.
func (a *App) trafficPacketControls(c *ui.Context, t tool.Traffic, st *trafficState) {
	if len(st.ifaces) == 0 {
		comp.Text(c, i18n.TrafficNoInterfaces(), ds.Caption, ds.P(c).TextInactive)
		return
	}
	names := make([]string, len(st.ifaces))
	chosen := 0
	for i, it := range st.ifaces {
		names[i] = it.Name
		if it.Description != "" {
			names[i] = it.Name + " · " + it.Description
		}
		if st.iface == it.Name {
			chosen = i
		}
	}
	ui.Column(c).Gap(ds.Space4).Children(func() {
		comp.Text(c, i18n.TrafficInterface(), ds.Label, ds.P(c).TextPrimary)
		a.kit.Select(c, names, chosen, func(i int) {
			st.iface = st.ifaces[i].Name
			a.trafficApplyNow(t, st)
		})
	})
}

// trafficLoadInterfaces asks the system for the interfaces a capture can
// read, off the UI thread: on some systems that loads a driver.
func (a *App) trafficLoadInterfaces(t tool.Traffic, st *trafficState) {
	st.ifacesLoaded = true // asked for once
	st.begin()
	a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficTimeout, func(context.Context) {
		list, err := t.Interfaces()
		a.tell(st, trafficOutcome{err: err, ifaces: list, hasIfaces: true})
	})
}

// markConfig asks for the fields to be applied once typing has rested.
func (st *trafficState) markConfig() {
	st.cfgDirty = true
	st.cfgDue = time.Now().Add(cfgDelay)
}

// trafficConfigDue applies what was typed once it has rested, and shows a
// field's error meanwhile.
func (a *App) trafficConfigDue(c *ui.Context, t tool.Traffic, st *trafficState) {
	if !st.cfgDirty || a.trafficBusy(st) {
		return
	}
	if wait := time.Until(st.cfgDue); wait > 0 {
		c.After(wait)
		return
	}
	a.trafficApplyNow(t, st)
}

// trafficApplyNow asks the tool for the settings the fields hold. The tool
// restarts a capture in progress, so it runs off the UI thread.
func (a *App) trafficApplyNow(t tool.Traffic, st *trafficState) {
	cfg, ok := st.captureConfig(t)
	st.cfgDirty = false
	if !ok {
		st.err = nil
		return
	}
	st.begin()
	st.err = nil
	a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficTimeout, func(context.Context) {
		a.tell(st, trafficOutcome{err: t.SetConfig(cfg)})
	})
}

// parseUpstream reads "http://host:port" or "socks5://host:port", and
// nothing else; an empty text goes direct.
func parseUpstream(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", true
	}
	u, err := url.Parse(text)
	if err != nil || (u.Scheme != "http" && u.Scheme != "socks5") {
		return "", false
	}
	if u.Host == "" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" {
		return "", false
	}
	return text, true
}

// splitList splits a list typed with commas, as Chinese input types them.
func splitList(text string) []string {
	text = strings.ReplaceAll(text, "\uff0c", ",")
	var out []string
	for _, part := range strings.Split(text, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
