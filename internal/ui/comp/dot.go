package comp

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// DotTone is what a status dot says.
type DotTone int

// The states a dot shows.
const (
	DotLive    DotTone = iota // running: daemon, Keep Awake, auto-refresh
	DotAccent                 // the selected sidebar row
	DotMuted                  // stopped
	DotWarning                // attention
	DotDanger
)

// DotOpts describe a status dot.
type DotOpts struct {
	Tone  DotTone
	Pulse bool   // running right now: a 1s ping
	Large bool   // 8pt instead of 6pt
	Label string // the accessible name of a dot that stands alone
}

// pingStart is the share of the ping after which it has faded out.
const pingShare = 0.75

// StatusDot is a 6pt dot (8pt large) for live state. The ping stops under
// reduced motion.
func StatusDot(c *ui.Context, o DotOpts) ui.Element {
	p := ds.P(c)
	col := p.Live
	switch o.Tone {
	case DotAccent:
		col = p.Accent
	case DotMuted:
		col = p.TextMuted
	case DotWarning:
		col = p.Warning
	case DotDanger:
		col = p.Danger
	case DotLive:
	}
	size := float32(ds.Dot)
	if o.Large {
		size = ds.DotLarge
	}
	dot := ui.Box(c).Size(size, size).Radius(ds.RadiusFull).Background(col).Shrink(0)
	if o.Label != "" {
		dot.Label(o.Label)
	}
	if !o.Pulse || c.Preferences().ReduceMotion {
		return dot
	}
	t := min(dot.Loop("ping", ds.PingDuration, ui.Linear)/pingShare, 1)
	scale := 1 + (ds.StatusDotPing-1)*t
	return dot.Children(func() {
		ui.Box(c).Absolute().Top(0).Left(0).Size(size, size).Radius(ds.RadiusFull).Background(col).
			Scale(scale, scale).Opacity(pingShare * (1 - t)).PassThrough()
	})
}

// CategoryColors is a category's label and fill: cat-<id> on cat-<id>-soft.
func CategoryColors(c *ui.Context, cat tool.Category) (ink, fill ui.Color) {
	p := ds.P(c)
	switch cat {
	case tool.CategorySystem:
		return p.CatSystem, p.CatSystemSoft
	case tool.CategoryFile:
		return p.CatFile, p.CatFileSoft
	case tool.CategoryDev:
		return p.CatDev, p.CatDevSoft
	case tool.CategoryNetwork:
		return p.CatNetwork, p.CatNetworkSoft
	case tool.CategoryText:
		return p.CatText, p.CatTextSoft
	case tool.CategoryMedia:
		return p.CatMedia, p.CatMediaSoft
	case tool.CategoryProductivity:
		return p.CatProductivity, p.CatProductivitySoft
	case tool.CategoryApp:
	}
	return p.CatApp, p.CatAppSoft
}

// CategoryBadge is a tool's category as an uppercase pastel pill.
func CategoryBadge(c *ui.Context, cat tool.Category, label string) ui.Element {
	ink, fill := CategoryColors(c, cat)
	style := ds.Caption
	style.Weight, style.Tracking = ds.Bold, ds.CategoryTracking
	return ui.Row(c).Height(ds.CategoryHeight).Padding(0, ds.Space8).Radius(ds.RadiusFull).Background(fill).
		AlignItems(ui.Center).Children(func() {
		Text(c, strings.ToUpper(label), style, ink).SingleLine()
	})
}

// ProgressBar is a 6pt track for long work; fraction is 0 to 1. The
// gradient is the design system's one: nowhere else.
func ProgressBar(c *ui.Context, fraction float32) ui.Element {
	p := ds.P(c)
	return ui.Box(c).FillWidth().Height(ds.ProgressHeight).Radius(ds.RadiusFull).Background(p.SandboxBg).Clip().
		Children(func() {
			ui.Box(c).WidthPercent(max(0, min(fraction, 1))*ds.PercentFull).FillHeight().
				Gradient(p.SandboxBlue, p.SandboxPurple, 90)
		})
}

// busyShare is how much of the top strip the sliding segment covers, in percent.
const busyShare = 30

// BusySweep is how long the segment takes to cross the window.
const BusySweep = 900 * time.Millisecond

// ProgressBusy is the indeterminate strip on the window's top edge. Light
// uses the accent, which is dark enough to read on the pale titlebar; dark
// keeps the cyan streak, which glows on it.
// ProgressBusy is the indeterminate strip on the window's top edge. Light
// uses the accent with a soft start so the tail is visible on the pale
// titlebar; dark keeps the cyan streak that glows on it.
func ProgressBusy(c *ui.Context) ui.Element {
	p := ds.P(c)
	var ink, from ui.Color
	if c.Theme().Dark {
		ink = p.SandboxCyan
		from = ink.Alpha(0)
	} else {
		ink = p.Accent
		from = ink.Alpha(0.15)
	}
	line := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(ds.BusyLine).Clip()
	at := line.Loop("busy", BusySweep, ui.Linear) * (ds.PercentFull - busyShare)
	return line.Children(func() {
		ui.Box(c).Absolute().Top(0).Bottom(0).LeftPercent(at).WidthPercent(busyShare).
			Gradient(from, ink, 90)
	})
}
