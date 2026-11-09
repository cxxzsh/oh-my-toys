package shell

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

const trafficSettingsWidth = 540

// trafficSettings is the Settings dialog. It holds the capture's port and
// switches, the certificate and the capture of one program. It opens over the
// page, so the page keeps its layout.
func (a *App) trafficSettings(c *ui.Context, t tool.Traffic, st *trafficState) {
	if !st.settings {
		return
	}
	p := ds.P(c)
	ui.DialogBase(c, &st.settings, func(backdrop, panel ui.Element) {
		backdrop.Background(p.Scrim)
		ds.ApplyShadow(panel.Width(trafficSettingsWidth).Column().Radius(ds.RadiusMd).
			Background(p.BgPopover).Border(ds.LineWidth, p.LineControl).Clip(), ds.S(c).ShadowPopover)
		ui.Column(c).FillWidth().Padding(ds.Space20).Gap(ds.Space12).Children(func() {
			comp.Text(c, i18n.TrafficSettingsTitle(), ds.Headline, p.TextStrong)
			a.trafficProblem(c, st)
			a.trafficNote(c, t)
			a.trafficModeRow(c, t, st)
			a.trafficControls(c, t, st)
			a.trafficCapture(c, t, st)
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				done := comp.ButtonOpts{Kind: comp.Primary}
				if a.kit.Button(c.Key("traffic-settings-done"), i18n.TrafficSettingsDone(), done).Clicked() {
					st.settings = false
				}
			})
		})
	})
}

// trafficCapture copies the proxy environment as a script, so that a program
// ignoring the system proxy can still be captured. The terminal itself opens
// from the toolbar.
func (a *App) trafficCapture(c *ui.Context, t tool.Traffic, st *trafficState) {
	if t.Config().Mode != tool.ModeProxy {
		return
	}
	comp.Eyebrow(c, i18n.TrafficCaptureTitle())
	comp.Hint(c, i18n.TrafficCaptureHint())
	ui.Row(c).FillWidth().Gap(ds.Space8).Children(func() {
		copyEnv := comp.ButtonOpts{Kind: comp.Secondary, Small: true}
		if a.kit.Button(c.Key("traffic-env"), i18n.TrafficCopyEnv(), copyEnv).Clicked() {
			a.trafficCopyEnv(c, t, st)
		}
	})
}

// trafficModeRow picks between the proxy and the packet capture. It applies at
// once, as the rest of the mode's settings do.
func (a *App) trafficModeRow(c *ui.Context, t tool.Traffic, st *trafficState) {
	modes := []string{i18n.TrafficModeProxy(), i18n.TrafficModePackets()}
	ui.Row(c).FillWidth().Children(func() {
		if comp.Segmented(c.Key("traffic-mode"), &st.mode, modes, i18n.ModeLabel()).Changed() {
			a.trafficApplyNow(t, st)
		}
	})
}

// trafficOpenTerminal opens a terminal with the proxy environment. It runs off
// the UI thread because spawning a terminal can take a while.
func (a *App) trafficOpenTerminal(t tool.Traffic, st *trafficState) {
	st.begin()
	st.err = nil
	a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficTimeout, func(ctx context.Context) {
		a.tell(st, trafficOutcome{err: t.OpenTerminal(ctx)})
	})
}

// trafficCopyEnv copies the proxy environment as a script the user can paste
// into any shell. A stopped capture has no proxy to copy, so that error goes
// to the problem banner.
func (a *App) trafficCopyEnv(c *ui.Context, t tool.Traffic, st *trafficState) {
	env, err := t.Environment()
	if err != nil {
		st.record(err)
		return
	}
	mygo.Clipboard.WriteText(env.Script)
	c.Toast(i18n.TrafficEnvCopied())
}

// trafficSetScope records only the named programs, or everything when scope is
// empty. It keeps the rest of the capture's settings as they are.
func (a *App) trafficSetScope(t tool.Traffic, st *trafficState, scope []string) {
	cfg := t.Config()
	cfg.Scope = scope
	st.begin()
	st.err = nil
	a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficTimeout, func(context.Context) {
		a.tell(st, trafficOutcome{err: t.SetConfig(cfg)})
	})
}
