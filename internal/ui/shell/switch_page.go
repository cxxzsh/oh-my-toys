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
	ui.Column(c).Grow(1).FillWidth().Padding(ds.Space16).Gap(ds.Space16).Children(func() {
		comp.PageHeader(c, comp.Header{
			Title: m.Name(msgs()), Badge: st.Text, Description: m.Summary(msgs()), Meta: toggle,
		})
		a.switchHero(c, st)
		a.switchModes(c, t, st)
		ui.Spacer(c)
		a.kit.StatusBar(c, comp.StatusBarOpts{
			Title:  i18n.LiveTelemetry(),
			Counts: []comp.StatusCount{{Text: st.Text, Live: st.On}},
			Hints:  []string{modKey("K") + " " + i18n.SearchTools()},
		})
	})
}

// switchHero is the state at a glance: the dot, the countdown and what is
// left of it on the left, and when it started and ends on the right.
func (a *App) switchHero(c *ui.Context, st tool.Status) {
	comp.Card(c).FillWidth().Padding(ds.Space20).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space20).AlignItems(ui.Stretch).Wrap().Children(func() {
			a.heroCountdown(c, st)
			if st.On {
				a.heroFacts(c, st)
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
func (a *App) heroFacts(c *ui.Context, st tool.Status) {
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
	})
}

// switchModes is how to start the tool: the presets, and for a tool that
// can run for a set time, a custom length too, on tabs. The quick action
// that turns it off is at the end of the head.
func (a *App) switchModes(c *ui.Context, t tool.Switch, st tool.Status) {
	f := a.switchForm(t.Manifest().ID)
	timed, isTimed := t.(tool.Timed)
	opts := comp.PanelOpts{Title: i18n.SwitchModes(), Glyph: "tune"}
	if st.On {
		opts.Actions = func() { a.stopButton(c, t) }
	}
	a.kit.Panel(c, opts, func() {
		if isTimed {
			tabs := []string{i18n.SwitchPresets(), i18n.SwitchCustom()}
			ui.Row(c).AlignItems(ui.Center).Children(func() { comp.Segmented(c.Key("modes-tab"), &f.tab, tabs, i18n.SwitchModes()) })
		}
		if isTimed && f.tab == tabCustom {
			a.customTab(c, timed, f)
			return
		}
		a.presetTiles(c, t, st)
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
	comp.Text(c, i18n.SwitchHint(), ds.Caption, ds.P(c).TextInactive)
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
