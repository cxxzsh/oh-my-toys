package shell

import (
	"context"
	"log"
	"time"

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
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(ds.Space16).Gap(ds.Space16).Children(func() {
				comp.PageHeader(c, comp.Header{
					Title: m.Name(msgs()), Badge: st.Text, Description: m.Summary(msgs()), Meta: toggle,
				})
				a.switchHero(c, t, st)
				a.switchModes(c, t, st)
			})
		})
		ui.Column(c).FillWidth().Padding(0, ds.Space16, ds.Space16).Children(func() {
			a.kit.StatusBar(c, comp.StatusBarOpts{
				Title:  i18n.LiveTelemetry(),
				Counts: []comp.StatusCount{{Text: st.Text, Live: st.On}},
				Hints:  []string{modKey("K") + " " + i18n.SearchTools()},
			})
		})
	})
}

// switchHero is the state at a glance: the dot, the countdown and what is
// left of it on the left, and when it started and ends on the right.
func (a *App) switchHero(c *ui.Context, t tool.Switch, st tool.Status) {
	comp.Card(c).FillWidth().Padding(ds.Space20).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space20).AlignItems(ui.Stretch).Wrap().Children(func() {
			a.heroCountdown(c, st)
			if st.On {
				a.heroFacts(c, t, st)
			}
		})
	})
}

func (a *App) heroCountdown(c *ui.Context, st tool.Status) {
	p := ds.P(c)
	ink, tone, state := p.TextMuted, comp.DotMuted, i18n.StatusOff()
	if st.On {
		ink, tone, state = p.LiveText, comp.DotLive, i18n.StatusOn()
	}
	ui.Column(c).Grow(1).Basis(ds.FactsMinWidth).Gap(ds.Space6).Children(func() {
		ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
			comp.StatusDot(c, comp.DotOpts{Tone: tone, Pulse: st.On, Large: true})
			comp.Text(c, state, ds.Headline, p.TextStrong)
			if st.Detail != "" && st.On {
				comp.Text(c, "· "+st.Detail, ds.Caption, p.TextInactive)
			}
		})
		big := ds.Title
		big.Size, big.LineHeight, big.Mono, big.Tracking = ds.CountdownSize, ds.CountdownLine, true, 0
		comp.Text(c, countdownText(st), big, ink).SingleLine()
		comp.Text(c, countdownCaption(st), ds.Caption, p.TextInactive)
		if left, total, ok := timeLeft(st); ok {
			comp.ProgressBar(c, float32(left)/float32(total))
		}
	})
}

// timeLeft is how long a timed run still has and how long it was given;
// ok is false when it is off or has no end.
func timeLeft(st tool.Status) (left, total time.Duration, ok bool) {
	if !st.On || st.Until.IsZero() {
		return 0, 0, false
	}
	total = st.Until.Sub(st.Since)
	left = max(time.Until(st.Until), 0)
	return left, total, total > 0
}

func countdownText(st tool.Status) string {
	if left, _, ok := timeLeft(st); ok {
		return tool.Clock(left.Round(time.Second))
	}
	if st.On {
		return "∞"
	}
	return "--:--:--"
}

func countdownCaption(st tool.Status) string {
	switch {
	case !st.On:
		return i18n.SwitchPickPreset()
	case st.Until.IsZero():
		return i18n.SwitchUntilOff()
	}
	return i18n.SwitchTimeLeft()
}

// heroFacts are the clock times of the run, in mono.
func (a *App) heroFacts(c *ui.Context, t tool.Switch, st tool.Status) {
	p := ds.P(c)
	ends := i18n.SwitchUntilOff()
	if !st.Until.IsZero() {
		ends = st.Until.Format(time.TimeOnly)
	}
	ui.Column(c).Grow(1).Basis(ds.FactsMinWidth).Padding(0, 0, 0, ds.Space20).
		BorderWidth(0, 0, 0, ds.LineWidth).BorderColor(p.LineHairline).Children(func() {
		a.valueRow(c, i18n.SwitchStarted(), st.Since.Format(time.TimeOnly))
		a.valueRow(c, i18n.SwitchEnds(), ends)
		a.valueRow(c, i18n.SwitchRunning(), tool.Clock(time.Since(st.Since).Round(time.Second)))
		a.heroActions(c, t, st)
	})
}

// extendBy is how much time the extend button adds to a run.
const extendBy = 30 * time.Minute

// The tabs of a switch tool's page.
const (
	tabModes = iota
	tabPlan
)

// switchModes is how to start the tool: its presets and, when it can, a
// length of the person's own; and on a tab of its own the plan, when it
// has one.
func (a *App) switchModes(c *ui.Context, t tool.Switch, st tool.Status) {
	f := a.switchForm(t.Manifest().ID)
	sched, planned := t.(tool.Scheduled)
	if !planned {
		f.tab = tabModes
	}
	comp.Card(c).FillWidth().Padding(ds.Space12).Gap(ds.Space10).Children(func() {
		if planned {
			a.modeTabs(c, sched, f)
		}
		if f.tab == tabPlan {
			a.planTab(c, sched, f)
			return
		}
		a.presetTiles(c, t, st)
		if timed, ok := t.(tool.Timed); ok {
			a.customForm(c, timed, f)
		}
	})
}

// modeTabs picks between the ways to start the tool and its plan; the
// switch that turns the plan on is beside the tabs while the plan shows.
func (a *App) modeTabs(c *ui.Context, t tool.Scheduled, f *switchForm) {
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		names := []string{i18n.SwitchModes(), i18n.SwitchPlan()}
		comp.Segmented(c.Key("modes-tab"), &f.tab, names, i18n.SwitchModes())
		ui.Spacer(c)
		if f.tab == tabPlan {
			a.planSwitch(c, t)
		}
	})
}

// presetTiles lists a tool's modes as tiles, the one in effect marked.
func (a *App) presetTiles(c *ui.Context, t tool.Switch, st tool.Status) {
	m := t.Manifest()
	ui.Row(c).FillWidth().Gap(ds.Space8).Wrap().Children(func() {
		for i := range m.Actions {
			if act := &m.Actions[i]; !act.Stops {
				a.presetTile(c, t, act, st.On && st.Active == act.ID)
			}
		}
	})
}

// heroActions are what a person can do to a run in progress: give it more
// time, when it ends by itself, or end it.
func (a *App) heroActions(c *ui.Context, t tool.Switch, st tool.Status) {
	ui.Row(c).Gap(ds.Space8).Padding(ds.Space8, 0, 0).Children(func() {
		if timed, ok := t.(tool.Timed); ok && !st.Until.IsZero() {
			label := i18n.SwitchExtend(int(extendBy / time.Minute))
			if a.kit.Button(c.Key("extend"), label, comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
				a.extend(timed)
			}
		}
		a.stopButton(c, t)
	})
}

// extend gives a timed run extendBy more than it has left.
func (a *App) extend(t tool.Timed) {
	if until := t.Status().Until; !until.IsZero() {
		a.startFor(t, max(time.Until(until), 0)+extendBy)
	}
}

func (a *App) stopButton(c *ui.Context, t tool.Switch) {
	m := t.Manifest()
	for i := range m.Actions {
		act := &m.Actions[i]
		if !act.Stops {
			continue
		}
		opts := comp.ButtonOpts{Kind: comp.Danger, Small: true, Glyph: "power_settings_new"}
		if a.kit.Button(c.Key(act.ID), act.Title(msgs()), opts).Clicked() {
			a.runAction(t, act)
		}
		return
	}
}

func (a *App) presetTile(c *ui.Context, t tool.Switch, act *tool.QuickAction, active bool) {
	p := ds.P(c)
	title := act.Title(msgs())
	b := ui.ButtonBase(c.Key("preset:" + act.ID)).Label(title).Selected(active).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.runAction(t, act)
	}
	style, ink, fill, line := ds.Label, p.TextPrimary, p.FillControl, p.LineHairline
	switch {
	case active:
		style.Weight, ink, fill, line = ds.Semibold, p.AccentText, p.AccentSoft, p.AccentLine
	case b.Hovered():
		fill = p.FillControlHover
	}
	b.Grow(1).Basis(ds.PresetBasis).Height(ds.PresetHeight).Padding(0, ds.Space12).Gap(ds.Space8).
		Radius(ds.RadiusSm).Background(fill).Border(ds.LineWidth, line).AlignItems(ui.Center).Children(func() {
		comp.Text(c, title, style, ink).Grow(1).SingleLine().Ellipsis("…")
		if active {
			a.kit.Glyph(c, "check", ds.Glyph, p.AccentText)
		}
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

// startFor starts a timed switch tool for d.
func (a *App) startFor(t tool.Timed, d time.Duration) {
	id := t.Manifest().ID
	a.opt.Runner.Do(id, "action", func() {
		if err := t.StartFor(context.Background(), d); err != nil {
			log.Printf("%s: %v", id, err)
		}
	})
}
