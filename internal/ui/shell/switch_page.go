package shell

import (
	"context"
	"log"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// switchPage is the page of a switch tool: its state and switch in the
// header, its status, and its quick actions as buttons.
func (a *App) switchPage(c *ui.Context, t tool.Switch) {
	p := ds.P(c)
	m := t.Manifest()
	st := t.Status()
	if st.On {
		c.After(tick) // countdowns
	}
	a.header(c, m, func() {
		comp.Pill(c, st.Text, st.On)
		on := st.On
		if comp.Switch(c, &on, m.Name(msgs())).Changed() {
			a.setSwitch(t, on)
		}
	})
	ui.Row(c).FillWidth().Gap(ds.Space3).Children(func() {
		comp.MetricTile(c, i18n.StatusLabel(), st.Text, "")
		if st.Detail != "" {
			comp.MetricTile(c, i18n.ModeLabel(), st.Detail, "")
		}
	})
	if len(m.Actions) == 0 {
		return
	}
	comp.Card(c).FillWidth().Padding(ds.Space4).Gap(ds.Space3).Children(func() {
		comp.Eyebrow(c, i18n.QuickActions())
		ui.Row(c).FillWidth().Gap(ds.Space2).Wrap().Children(func() {
			for i := range m.Actions {
				act := &m.Actions[i]
				if comp.Button(c.Key(act.ID), act.Title(msgs()), comp.Secondary).Clicked() {
					a.runAction(t, act)
				}
			}
		})
		comp.Text(c, i18n.SwitchHint(), ds.Callout, p.TextSecondary)
	})
}

func (a *App) setSwitch(t tool.Switch, on bool) {
	id := t.Manifest().ID
	a.opt.Runner.Do(id, "action", func() {
		var err error
		if on {
			err = t.Start(context.Background())
		} else {
			err = t.Stop()
		}
		if err != nil {
			log.Printf("%s: %v", id, err)
		}
	})
}
