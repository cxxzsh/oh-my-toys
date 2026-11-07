package comp

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Card is a bg-card box with its line, radius-md and, in light, shadow-xs.
func Card(c *ui.Context) ui.Element {
	p := ds.P(c)
	return ds.ApplyShadow(ui.Column(c).Radius(ds.RadiusMd).Background(p.BgCard).Border(ds.LineWidth, p.Line),
		ds.S(c).ShadowXs)
}

// PanelOpts describe a panel's head.
type PanelOpts struct {
	Title   string
	Glyph   string // drawn in accent
	Actions func() // at the end of the head: at most one primary button
}

// Panel is a titled card that groups one tool's controls or one preview.
// Never nest panels.
func (k *Kit) Panel(c *ui.Context, o PanelOpts, body func()) ui.Element {
	p := ds.P(c)
	return Card(c).FillWidth().Padding(ds.Space12).Gap(ds.Space10).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
			if o.Glyph != "" {
				k.Glyph(c, o.Glyph, ds.Glyph, p.Accent)
			}
			Text(c, o.Title, ds.Section, p.TextStrong).SingleLine().Role(ui.RoleHeading).Level(2)
			ui.Spacer(c)
			if o.Actions != nil {
				o.Actions()
			}
		})
		body()
	})
}

// SettingsRow is one row of a SettingsList.
type SettingsRow struct {
	Glyph   string
	Title   string
	Desc    string // one line
	Control func() // a Switch, SegmentedControl, Select, or a chevron
}

// SettingsList is grouped settings rows in a card: glyph, title,
// description, and the control at the end. Settings apply at once.
func (k *Kit) SettingsList(c *ui.Context, rows []SettingsRow) ui.Element {
	p := ds.P(c)
	return Card(c).FillWidth().Clip().Children(func() {
		for i, r := range rows {
			row := ui.Row(c.Key(i)).FillWidth().MinHeight(ds.SettingsRow).Padding(ds.Space8, ds.Space16).
				Gap(ds.Space12).AlignItems(ui.Center)
			if i > 0 {
				row.BorderWidth(ds.LineWidth, 0, 0, 0).BorderColor(p.LineHairline)
			}
			row.Children(func() { k.settingsRow(c, r) })
		}
	})
}

func (k *Kit) settingsRow(c *ui.Context, r SettingsRow) {
	p := ds.P(c)
	if r.Glyph != "" {
		k.Glyph(c, r.Glyph, ds.Glyph, p.TextSecondary)
	}
	ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
		Text(c, r.Title, ds.Body, p.TextPrimary)
		if r.Desc != "" {
			Text(c, r.Desc, ds.Caption, p.TextInactive)
		}
	})
	if r.Control != nil {
		r.Control()
	}
}

// Header describes a tool page's top: the heading with a count badge, one
// line of description, mono meta and the filter row.
type Header struct {
	Title       string
	Badge       string // beside the title: "6 Listening"
	Description string
	Meta        func() // mono values at the end of the top row
	Filters     func() // chips at the start of the second row
	Hint        string // at the end of the second row: "Auto-refresh 2s"
}

// PageHeader is the top of a tool's workbench, closed by a line rule.
func PageHeader(c *ui.Context, h Header) {
	ui.Column(c).FillWidth().Gap(ds.Space12).Padding(0, 0, ds.Space12).
		BorderWidth(0, 0, ds.LineWidth, 0).BorderColor(ds.P(c).Line).Children(func() {
		headerTop(c, h)
		if h.Filters == nil && h.Hint == "" {
			return
		}
		ui.Row(c).FillWidth().Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
			if h.Filters != nil {
				h.Filters()
			}
			ui.Spacer(c)
			if h.Hint != "" {
				Hint(c, h.Hint)
			}
		})
	})
}

func headerTop(c *ui.Context, h Header) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Gap(ds.Space12).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(ds.HeaderMetaGap).Children(func() {
			ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
				Text(c, h.Title, ds.Title, p.TextStrong).SingleLine().Role(ui.RoleHeading).Level(1)
				if h.Badge != "" {
					BadgeRimmed(c, h.Badge)
				}
			})
			Text(c, h.Description, ds.Caption, p.TextInactive)
		})
		if h.Meta != nil {
			ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(h.Meta)
		}
	})
}

// StatusCount is a count of a StatusBar: accent, or live while running.
type StatusCount struct {
	Text string
	Live bool
}

// StatusBarOpts describe a StatusBar.
type StatusBarOpts struct {
	Title  string
	Tag    string        // "Poll: 2.0s"
	Counts []StatusCount // up to four
	Hints  []string      // up to three shortcut hints
}

// StatusBar is the 28pt telemetry strip at the bottom of a tool's
// workbench: live counts at the start, shortcut hints at the end.
func (k *Kit) StatusBar(c *ui.Context, o StatusBarOpts) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.StatusbarHeight).Padding(0, ds.Space10).Gap(ds.Space12).AlignItems(ui.Center).
		Radius(ds.RadiusSm).Background(p.BgTitlebar).Border(ds.LineWidth, p.Line).Shrink(0).Children(func() {
		ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
			k.Glyph(c, "terminal", ds.GlyphSm, p.Accent)
			Text(c, o.Title, ds.Headline, p.TextStrong).SingleLine()
			if o.Tag != "" {
				ui.Row(c).Padding(0, ds.Space4).Radius(ds.RadiusXs).Background(p.FillControlHover).Children(func() {
					Value(c, o.Tag, p.TextSecondary)
				})
			}
			for _, n := range o.Counts {
				ink := p.AccentText
				if n.Live {
					ink = p.LiveText
				}
				Value(c, n.Text, ink)
			}
		})
		ui.Spacer(c)
		for _, h := range o.Hints {
			Hint(c, h)
		}
	})
}

// DaemonCard is the sidebar footer: name, state, version and pid, all
// mono. It reports a click.
func DaemonCard(c *ui.Context, name, state, version, pid string, running bool) ui.Element {
	p := ds.P(c)
	stateInk, tone := p.LiveText, DotLive
	if !running {
		stateInk, tone = p.TextMuted, DotMuted
	}
	b := ui.ButtonBase(c).Label(name + ", " + state).Cursor(ui.CursorDefault)
	ds.ApplyShadow(b.FillWidth().Column().Padding(ds.DaemonPad).Gap(ds.Space8).Radius(ds.RadiusMd).
		Background(p.BgCard).Border(ds.LineWidth, p.LineHairline), ds.S(c).ShadowXs)
	return b.Children(func() {
		ui.Row(c).FillWidth().Padding(0, ds.Space4).AlignItems(ui.Center).Children(func() {
			StatusDot(c, DotOpts{Tone: tone, Pulse: running})
			Text(c, name, ds.Numeric, p.TextStrong).SingleLine().Margin(0, 0, 0, ds.Space8)
			ui.Spacer(c)
			Text(c, state, ds.Badge, stateInk).SingleLine()
		})
		ui.Row(c).FillWidth().Padding(0, ds.Space4).Children(func() {
			Value(c, version, p.TextMuted)
			ui.Spacer(c)
			Value(c, pid, p.TextMuted)
		})
	})
}
