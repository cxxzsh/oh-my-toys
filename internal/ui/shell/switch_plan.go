package shell

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// The times a new span of a plan starts with.
const (
	newSpanFrom = "09:00"
	newSpanTo   = "18:00"
)

// removeSpan and addSpan are what the buttons of a plan read. The plus and
// the cross are marks, not words.
const removeSpan = "×"

// weekOrder is the days of the week as a span shows them, Monday first.
var weekOrder = [...]time.Weekday{
	time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday,
}

func weekdayLabel(d time.Weekday) string {
	switch d {
	case time.Monday:
		return i18n.WeekdayMon()
	case time.Tuesday:
		return i18n.WeekdayTue()
	case time.Wednesday:
		return i18n.WeekdayWed()
	case time.Thursday:
		return i18n.WeekdayThu()
	case time.Friday:
		return i18n.WeekdayFri()
	case time.Saturday:
		return i18n.WeekdaySat()
	default:
		return i18n.WeekdaySun()
	}
}

// weekdayName is the whole name of a day, for what a screen reader says of
// its toggle.
func weekdayName(d time.Weekday) string {
	return [...]func() string{
		time.Sunday: i18n.WeekdayNameSun, time.Monday: i18n.WeekdayNameMon, time.Tuesday: i18n.WeekdayNameTue,
		time.Wednesday: i18n.WeekdayNameWed, time.Thursday: i18n.WeekdayNameThu,
		time.Friday: i18n.WeekdayNameFri, time.Saturday: i18n.WeekdayNameSat,
	}[d]()
}

// planRow is a span of the day as a person types it. Its times may be half
// typed, so it is not a tool.Segment until both read.
type planRow struct {
	days     tool.Weekdays
	from, to string

	// last is the span this row last said whole. While the row is half
	// typed, the plan keeps it, so that editing a time does not take the
	// span out of the plan for the keystrokes in between.
	last    tool.Segment
	hasLast bool
}

func rowOf(seg tool.Segment) planRow {
	return planRow{days: seg.Days, from: tool.FormatClock(seg.From), to: tool.FormatClock(seg.To), last: seg, hasLast: true}
}

// segment is the span the row says, and whether it says one.
func (r planRow) segment() (tool.Segment, bool) {
	from, okFrom := tool.ParseClock(r.from)
	to, okTo := tool.ParseClock(r.to)
	seg := tool.Segment{Days: r.days, From: from, To: to}
	return seg, okFrom && okTo && seg.Valid()
}

// syncPlan reads the rows from the plan of the tool the first time, and
// takes out the row a click asked to remove.
func (f *switchForm) syncPlan(t tool.Scheduled) {
	if !f.loaded {
		for _, seg := range t.Schedule().Segments {
			f.rows = append(f.rows, rowOf(seg))
		}
		f.loaded = true
	}
	if f.drop > 0 && f.drop <= len(f.rows) {
		f.rows = slices.Delete(f.rows, f.drop-1, f.drop)
	}
	f.drop = 0
}

// segments are the spans of the plan: what each row says, or what it last
// said whole while it is half typed. allRead is false while any row is not
// a span.
func (f *switchForm) segments() (segs []tool.Segment, allRead bool) {
	segs, allRead = make([]tool.Segment, 0, len(f.rows)), true
	for i := range f.rows {
		r := &f.rows[i]
		if seg, ok := r.segment(); ok {
			r.last, r.hasLast = seg, true
		} else {
			allRead = false
		}
		if r.hasLast {
			segs = append(segs, r.last)
		}
	}
	return segs, allRead
}

// planTab is the plan of a tool: when it turns itself on and off. Every
// change is kept and applied at once, for the rows that are whole.
func (a *App) planTab(c *ui.Context, t tool.Scheduled, f *switchForm) {
	f.syncPlan(t)
	a.planDisplay(c, t)
	a.weekStrip(c, t.Schedule(), time.Now())
	if len(f.rows) == 0 {
		comp.Text(c, i18n.SwitchPlanEmpty(), ds.Caption, ds.P(c).TextInactive)
	} else {
		a.planRows(c, f)
	}
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		a.addSpanButton(c, f)
	})
	segs, allRead := f.segments()
	if cur := t.Schedule(); !slices.Equal(segs, cur.Segments) {
		cur.Segments = segs
		a.setSchedule(t, cur)
	}
	a.planNote(c, t.Schedule(), allRead)
}

// planSwitch turns the plan on and off.
func (a *App) planSwitch(c *ui.Context, t tool.Scheduled) {
	cur := t.Schedule()
	ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		on := cur.Enabled
		if comp.Switch(c.Key("plan-on"), &on, i18n.SwitchPlanOn()).Changed() {
			cur.Enabled = on
			a.setSchedule(t, cur)
		}
		comp.Text(c, i18n.SwitchPlanOn(), ds.Caption, ds.P(c).TextInactive)
	})
}

// planDisplay is the choice to keep the display on while the plan holds.
func (a *App) planDisplay(c *ui.Context, t tool.Scheduled) {
	cur := t.Schedule()
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		display := cur.Display
		if comp.SwitchCompact(c.Key("plan-display"), &display, i18n.SwitchPlanDisplay()).Changed() {
			cur.Display = display
			a.setSchedule(t, cur)
		}
		comp.Text(c, i18n.SwitchPlanDisplay(), ds.Body, ds.P(c).TextPrimary)
	})
}

// planRows lists the spans of the plan.
func (a *App) planRows(c *ui.Context, f *switchForm) {
	ui.Column(c.Key("plan-rows")).FillWidth().Gap(ds.Space8).Children(func() {
		for i := range f.rows {
			a.planRow(c, f, i)
		}
	})
}

func (a *App) planRow(c *ui.Context, f *switchForm, i int) {
	r := &f.rows[i]
	_, read := r.segment()
	ui.Row(c.Key(i)).FillWidth().Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
		for _, d := range weekOrder {
			r.days = a.dayToggle(c, i, d, r.days)
		}
		ui.Spacer(c)
		a.timeField(c, [2]int{i, 0}, &r.from, !read)
		comp.Text(c, "–", ds.Body, ds.P(c).TextInactive)
		a.timeField(c, [2]int{i, 1}, &r.to, !read)
		opts := comp.ButtonOpts{Kind: comp.Plain, Small: true, Tooltip: i18n.SwitchRemoveSpan()}
		if a.kit.Button(c.Key([2]int{i, 2}), removeSpan, opts).Clicked() {
			f.drop = i + 1
		}
	})
}

func (a *App) timeField(c *ui.Context, key [2]int, value *string, invalid bool) {
	ui.Column(c.Key(key)).Width(ds.SpanFieldWidth).Children(func() {
		comp.TextField(c, value, comp.FieldOpts{Placeholder: newSpanFrom, Mono: true, Invalid: invalid})
	})
}

// dayToggle is one day of the week of a span, on or off; it returns the
// days after a click.
func (a *App) dayToggle(c *ui.Context, row int, d time.Weekday, days tool.Weekdays) tool.Weekdays {
	p, label, on := ds.P(c), weekdayLabel(d), days.Has(d)
	b := ui.ButtonBase(c.Key([2]int{row, 10 + int(d)})).Label(weekdayName(d)).Selected(on).Cursor(ui.CursorDefault)
	if b.Clicked() {
		days, on = days.With(d, !on), !on
	}
	fill, ink := p.FillControl, p.TextInactive
	switch {
	case on:
		fill, ink = p.AccentSoft, p.AccentText
	case b.Hovered():
		fill = p.FillControlHover
	}
	b.Width(ds.DayToggleWidth).Height(ds.ControlMd).Radius(ds.RadiusSm).Background(fill).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		comp.Text(c, label, ds.Caption, ink).SingleLine()
	})
	return days
}

func (a *App) addSpanButton(c *ui.Context, f *switchForm) {
	opts := comp.ButtonOpts{Kind: comp.Secondary, Small: true}
	if a.kit.Button(c.Key("plan-add"), "+ "+i18n.SwitchAddSpan(), opts).Clicked() {
		f.rows = append(f.rows, planRow{days: tool.Workdays, from: newSpanFrom, to: newSpanTo})
	}
}

// planNote says what is wrong with the rows, or when the plan next turns
// the tool on.
func (a *App) planNote(c *ui.Context, cur tool.Schedule, allRead bool) {
	p := ds.P(c)
	if !allRead {
		comp.Text(c, i18n.SwitchSpanBad(), ds.Caption, p.Danger)
		return
	}
	if line, ok := nextRunText(cur, time.Now()); ok {
		comp.Text(c, line, ds.Caption, p.TextInactive)
	}
}

// nextRunText says when a plan next turns its tool on, as a line.
func nextRunText(cur tool.Schedule, now time.Time) (string, bool) {
	next, ok := cur.Next(now)
	if !ok {
		return "", false
	}
	return i18n.SwitchNextRun(weekdayLabel(next.Weekday()), tool.FormatClock(next.Hour()*60+next.Minute())), true
}

// setSchedule gives a tool its plan and keeps it for the next launch.
func (a *App) setSchedule(t tool.Scheduled, s tool.Schedule) {
	t.SetSchedule(s)
	a.settings.SetSchedule(t.Manifest().ID, t.Schedule())
	a.save()
}

// applySchedules gives each tool that runs by a plan the plan kept for it.
func (a *App) applySchedules() {
	for _, t := range a.reg.All() {
		if s, ok := t.(tool.Scheduled); ok {
			if saved, kept := a.settings.Schedules[t.Manifest().ID]; kept {
				s.SetSchedule(saved)
			}
		}
	}
}
