package shell

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// cellKind is what a stretch of a day's bar shows.
type cellKind int

const (
	cellOff cellKind = iota // the plan does not hold it
	cellOn                  // the plan holds it
	cellNow                 // a mark at the time now
)

// cell is a stretch of a day's bar, as long as its minutes.
type cell struct {
	minutes int
	kind    cellKind
}

// notToday is the minute of a day that is not today: no mark is drawn.
const notToday = -1

// dayCells splits a day into the stretches the plan holds and those it does
// not, in order, and puts a mark at the minute now, unless it is notToday.
func dayCells(cover []tool.Range, now int) []cell {
	var cells []cell
	at := 0
	add := func(to int, kind cellKind) {
		if to > at {
			cells = append(cells, cell{to - at, kind})
			at = to
		}
	}
	for _, r := range cover {
		add(r.From, cellOff)
		add(r.To, cellOn)
	}
	add(tool.MinutesPerDay, cellOff)
	return markNow(cells, now)
}

// markNow splits the cell that holds the minute now to put the mark in.
func markNow(cells []cell, now int) []cell {
	if now == notToday {
		return cells
	}
	out := make([]cell, 0, len(cells)+2)
	start := 0
	for _, c := range cells {
		end := start + c.minutes
		if now >= start && now < end {
			if now > start {
				out = append(out, cell{now - start, c.kind})
			}
			out = append(out, cell{kind: cellNow})
			c = cell{end - now, c.kind}
		}
		out = append(out, c)
		start = end
	}
	return out
}

// weekStrip draws the plan across the week: a bar for each day, which is
// filled where the plan holds the tool, with a mark at the time now.
func (a *App) weekStrip(c *ui.Context, cur tool.Schedule, now time.Time) {
	ui.Column(c.Key("plan-week")).FillWidth().Gap(ds.Space4).Label(i18n.SwitchPlanWeek()).Children(func() {
		for _, d := range weekOrder {
			a.weekDay(c, cur, d, now)
		}
	})
}

func (a *App) weekDay(c *ui.Context, cur tool.Schedule, d time.Weekday, now time.Time) {
	p := ds.P(c)
	mark := notToday
	if now.Weekday() == d {
		mark = now.Hour()*60 + now.Minute()
	}
	ui.Row(c.Key(int(d))).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		ink := p.TextInactive
		if mark != notToday {
			ink = p.TextStrong
		}
		comp.Text(c, weekdayLabel(d), ds.Caption, ink).Width(ds.WeekLabelWidth).SingleLine()
		fill := p.Accent
		if !cur.Enabled {
			fill = p.TextInactive
		}
		ui.Row(c).Grow(1).Height(ds.WeekBarHeight).Radius(ds.RadiusFull).Background(p.FillControl).Clip().
			Children(func() {
				for i, cl := range dayCells(cur.Coverage(d), mark) {
					cellBox(c.Key(i), cl, fill, p.TextStrong)
				}
			})
	})
}

func cellBox(c *ui.Context, cl cell, fill, markInk ui.Color) {
	b := ui.Box(c).FillHeight()
	switch cl.kind {
	case cellNow:
		b.Width(ds.NowMarkWidth).Background(markInk)
	case cellOn:
		b.Grow(float32(cl.minutes)).Basis(0).Background(fill)
	case cellOff:
		b.Grow(float32(cl.minutes)).Basis(0)
	}
}
