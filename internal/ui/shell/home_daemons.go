package shell

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// showDaemons says whether Home leads with the Active Runs strip: on the
// whole catalog, and only while a switch tool is on.
func (a *App) showDaemons() bool {
	return a.homeCat == "" && a.pageFilter == "" && len(a.reg.Running()) > 0
}

// daemonStrip is the Active Runs row of Home: a card for each switch tool
// that is on.
func (a *App) daemonStrip(c *ui.Context) {
	running := a.reg.Running()
	ui.Column(c).FillWidth().Gap(ds.Space8).Children(func() {
		comp.Eyebrow(c, i18n.ActiveDaemons()).Padding(ds.Space12, 0, ds.Space8).Role(ui.RoleHeading).Level(2)
		ui.Row(c).FillWidth().Gap(ds.Space12).Children(func() {
			for _, sw := range running {
				a.daemonTile(c, sw)
			}
		})
	})
}

// daemonTile is one running switch tool on Home: its icon, name, live state,
// a timer bar when it has an end, and its actions.
func (a *App) daemonTile(c *ui.Context, sw tool.Switch) {
	p := ds.P(c)
	m := sw.Manifest()
	st := sw.Status()
	ui.Column(c.Key("daemon:"+m.ID)).MinWidth(ds.CardMinColumn).Grow(1).Padding(ds.Space12).Gap(ds.Space8).
		Radius(ds.RadiusMd).Background(p.BgCard).Border(ds.LineWidth, p.Line).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
			a.kit.ToolIcon(c, m, comp.ToolIconOpts{Size: comp.IconSmall, Tint: true})
			comp.Text(c, m.Name(msgs()), ds.Label, p.TextStrong).Grow(1).SingleLine().Ellipsis("...")
		})
		comp.Text(c, st.Text, ds.Numeric, p.LiveText).SingleLine()
		if left, total, ok := timeLeft(st); ok && total > 0 {
			comp.ProgressBar(c, float32(left)/float32(total))
		}
		ui.Row(c).FillWidth().Gap(ds.Space6).Justify(ui.End).Children(func() {
			a.daemonActions(c, sw, st)
		})
	})
}

// daemonActions are a running tool's extend, when a person can add to its
// end, and its stop.
func (a *App) daemonActions(c *ui.Context, sw tool.Switch, st tool.Status) {
	m := sw.Manifest()
	if timed, ok := sw.(tool.Timed); ok && canExtend(st) {
		extend := i18n.SwitchExtend(int(extendBy / time.Minute))
		if a.kit.Button(c.Key("daemon-extend:"+m.ID), extend, comp.ButtonOpts{Kind: comp.Ghost, Small: true, Square: true}).Clicked() {
			a.extend(timed)
		}
	}
	if a.kit.Button(c.Key("daemon-stop:"+m.ID), i18n.Stop(), comp.ButtonOpts{Kind: comp.Danger, Small: true}).Clicked() {
		a.setSwitch(sw, false)
	}
}

// quickRow is a switch tool's quick modes on its Home card: one mini-button
// for each start that has a short name, such as "1 hour".
func (a *App) quickRow(c *ui.Context, t tool.Tool) {
	m := t.Manifest()
	if _, ok := t.(tool.Switch); !ok || m.Soon {
		return
	}
	acts := quickActions(m)
	if len(acts) == 0 {
		return
	}
	ui.Row(c).FillWidth().Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
		for _, act := range acts {
			if a.kit.Button(c.Key("quick:"+m.ID+":"+act.ID), act.Short(msgs()), comp.ButtonOpts{Kind: comp.Ghost, Small: true, Square: true}).Clicked() {
				a.runAction(t, act)
			}
		}
	})
}

// quickActions are the actions a card offers as quick modes: those that
// start something and have a short name.
func quickActions(m tool.Manifest) []*tool.QuickAction {
	var out []*tool.QuickAction
	for i := range m.Actions {
		if act := &m.Actions[i]; !act.Stops && act.Short != nil {
			out = append(out, act)
		}
	}
	return out
}
