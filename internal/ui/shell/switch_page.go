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

func (a *App) switchPage(c *ui.Context, t tool.Switch) {
	m := t.Manifest()
	st := t.Status()
	if st.On {
		c.After(tick)
	}
	toggle := func() {
		on := st.On
		if comp.Switch(c, &on, m.Name(msgs())).Changed() {
			a.setSwitch(t, on)
		}
	}
	ui.Column(c).Grow(1).FillWidth().Padding(ds.Space16).Gap(ds.Space16).Children(func() {
		comp.PageHeader(c, comp.Header{
			Title: m.Name(msgs()), Badge: st.Text, Description: m.Summary(msgs()), Meta: toggle,
		})
		a.kit.SettingsList(c, []comp.SettingsRow{{
			Glyph: m.Icon, Title: m.Name(msgs()), Desc: st.Detail, Control: toggle,
		}})
		a.switchActions(c, t)
		ui.Spacer(c)
		a.kit.StatusBar(c, comp.StatusBarOpts{
			Title:  i18n.LiveTelemetry(),
			Counts: []comp.StatusCount{{Text: st.Text, Live: st.On}},
			Hints:  []string{modKey("K") + " " + i18n.SearchTools()},
		})
	})
}

func (a *App) switchActions(c *ui.Context, t tool.Switch) {
	m := t.Manifest()
	if len(m.Actions) == 0 {
		return
	}
	a.kit.Panel(c, comp.PanelOpts{Title: i18n.QuickActions(), Glyph: m.Icon}, func() {
		ui.Row(c).FillWidth().Gap(ds.Space8).Wrap().Children(func() {
			for i := range m.Actions {
				act := &m.Actions[i]
				if a.kit.Button(c.Key(act.ID), act.Title(msgs()), comp.ButtonOpts{Kind: comp.Secondary}).Clicked() {
					a.runAction(t, act)
				}
			}
		})
		comp.Text(c, i18n.SwitchHint(), ds.Caption, ds.P(c).TextInactive)
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
