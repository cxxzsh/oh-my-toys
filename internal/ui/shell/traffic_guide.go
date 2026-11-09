package shell

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// glyphDone marks a guide step that is already true.
const glyphDone = "check"

// trafficGuideDue reports whether the list should give way to the first-run
// guide: proxy mode, nothing captured yet, and no filter or sidebar pick that
// would explain an empty list.
func trafficGuideDue(t tool.Traffic, st *trafficState) bool {
	return t.Config().Mode == tool.ModeProxy && st.fetched && len(st.rows) == 0 &&
		!st.filterActive() && st.procSel == "" && st.hostSel == ""
}

// guideStep is one step of the guide. Its action draws the control that
// finishes the step; a step that is done shows a tick and no action.
type guideStep struct {
	text   string
	done   bool
	action func(c *ui.Context, t tool.Traffic, st *trafficState)
}

// trafficGuide walks a new user through the three steps that make a capture
// work. Each step ticks itself once its condition holds, and an open step
// offers the control that does it.
func (a *App) trafficGuide(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	steps := []guideStep{
		{i18n.TrafficGuideCert(), st.cert.Trusted, a.trafficGuideCertAction},
		{i18n.TrafficGuideProxy(), t.Config().SystemProxy, a.trafficGuideProxyAction},
		{i18n.TrafficGuideStart(), t.Status().On, a.trafficGuideStartAction},
	}
	comp.Card(c).FillWidth().Padding(ds.Space16).Gap(ds.Space10).Children(func() {
		comp.Text(c, i18n.TrafficGuideTitle(), ds.Headline, p.TextStrong)
		comp.Text(c, i18n.TrafficGuideBody(), ds.Label, p.TextSecondary)
		for _, s := range steps {
			a.trafficGuideStep(c, t, st, s)
		}
	})
}

// trafficGuideStep draws one step: a tick or an open circle, the text, and
// the action while the step is open.
func (a *App) trafficGuideStep(c *ui.Context, t tool.Traffic, st *trafficState, s guideStep) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		if s.done {
			a.kit.Glyph(c, glyphDone, ds.Space16, p.Accent)
		} else {
			comp.StatusDot(c, comp.DotOpts{Tone: comp.DotMuted})
		}
		comp.Text(c, s.text, ds.Label, p.TextPrimary).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if !s.done {
			s.action(c, t, st)
		}
	})
}

// trafficGuideCertAction installs the certificate by the same path the
// Settings button takes.
func (a *App) trafficGuideCertAction(c *ui.Context, t tool.Traffic, st *trafficState) {
	if a.kit.Button(c.Key("guide-cert"), i18n.TrafficCertInstall(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
		a.trafficCertAction(t, st, false)
	}
}

// trafficGuideProxyAction is the system proxy switch. The hint names the
// other way in: a program started through the proxy needs no system setting.
func (a *App) trafficGuideProxyAction(c *ui.Context, t tool.Traffic, st *trafficState) {
	a.trafficSwitch(c, "guide-sysproxy", &st.systemProxy, i18n.TrafficSystemProxy(), t, st)
}

// trafficGuideStartAction starts the capture off the UI thread.
func (a *App) trafficGuideStartAction(c *ui.Context, t tool.Traffic, st *trafficState) {
	opts := comp.ButtonOpts{Kind: comp.Primary, Small: true, Glyph: glyphPower, Disabled: a.trafficBusy(st)}
	if a.kit.Button(c.Key("guide-start"), i18n.SwitchStart(), opts).Clicked() {
		a.trafficSetOn(t, st, true)
	}
}
