package shell

import (
	"errors"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// trafficProblemText turns a failure into what a person reads: what
// happened, in plain words, and the steps that fix it. The error's own
// text is never the headline; it goes to the log.
func trafficProblemText(err error) (title, steps string) {
	switch {
	case errors.Is(err, tool.ErrPermission):
		return i18n.TrafficErrPermissionTitle(), i18n.TrafficErrPermissionSteps()
	case errors.Is(err, tool.ErrDriverMissing):
		return i18n.TrafficErrDriverTitle(), i18n.TrafficErrDriverSteps()
	case errors.Is(err, tool.ErrPortInUse):
		return i18n.TrafficErrPortTitle(), i18n.TrafficErrPortSteps()
	case errors.Is(err, tool.ErrCertNotTrusted):
		return i18n.TrafficErrCertTitle(), i18n.TrafficErrCertSteps()
	case errors.Is(err, tool.ErrUnsupportedHere):
		return i18n.TrafficErrUnsupportedTitle(), i18n.TrafficErrUnsupportedSteps()
	case errors.Is(err, tool.ErrNotRunning):
		return i18n.TrafficErrNotRunningTitle(), i18n.TrafficErrNotRunningSteps()
	}
	return i18n.TrafficErrTitle(), i18n.TrafficErrDetail()
}

// trafficProblem is the banner under the toolbar that explains the last
// failure and how to get past it.
func (a *App) trafficProblem(c *ui.Context, st *trafficState) {
	if st.err == nil {
		return
	}
	p := ds.P(c)
	title, steps := trafficProblemText(st.err)
	comp.Card(c).FillWidth().Padding(ds.Space12).Gap(ds.Space4).Background(p.DangerSoft).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
			comp.StatusDot(c, comp.DotOpts{Tone: comp.DotDanger})
			comp.Text(c, title, ds.Headline, p.TextStrong).Grow(1).Shrink(1).MinWidth(0)
			if a.kit.Button(c.Key("traffic-dismiss"), i18n.TrafficDismiss(), comp.ButtonOpts{Kind: comp.Plain, Small: true}).Clicked() {
				st.err = nil
			}
		})
		comp.Text(c, steps, ds.Body, p.TextPrimary)
	})
}
