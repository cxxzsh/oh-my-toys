package tool

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Weekdays is a set of days of the week. Bit n is time.Weekday n, so Sunday
// is bit 0.
type Weekdays uint8

// The sets of days a plan is usually made for.
const (
	AllDays  Weekdays = 1<<7 - 1
	Workdays Weekdays = 1<<time.Monday | 1<<time.Tuesday | 1<<time.Wednesday | 1<<time.Thursday | 1<<time.Friday
)

// Has reports whether d is in the set.
func (w Weekdays) Has(d time.Weekday) bool { return w&(1<<d) != 0 }

// With returns the set with d added, or taken out when on is false.
func (w Weekdays) With(d time.Weekday, on bool) Weekdays {
	if on {
		return w | 1<<d
	}
	return w &^ (1 << d)
}

// MinutesPerDay is the length of a day on the clock.
const MinutesPerDay = 24 * 60

// Segment is a span of the day, on some days of the week. From and To are
// minutes after midnight. A segment whose To is not after its From runs past
// midnight into the next day, and belongs to the day it begins on.
type Segment struct {
	Days Weekdays `json:"days"`
	From int      `json:"from"`
	To   int      `json:"to"`
}

// Valid reports whether the segment names at least one day and two
// different times of a day.
func (s Segment) Valid() bool {
	return s.Days != 0 && s.Days&^AllDays == 0 && inDay(s.From) && inDay(s.To) && s.From != s.To
}

func inDay(minutes int) bool { return minutes >= 0 && minutes < MinutesPerDay }

// ParseClock reads a time of day such as 9:05 or 18:30, in 24 hours, as
// minutes after midnight.
func ParseClock(text string) (minutes int, ok bool) {
	at, err := time.Parse(clockLayout, strings.TrimSpace(text))
	if err != nil {
		return 0, false
	}
	return at.Hour()*60 + at.Minute(), true
}

// FormatClock writes minutes after midnight as a time of day: 09:05.
func FormatClock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// Schedule is the plan by which a switch tool turns itself on and off: it
// is on while the time falls in any of its segments. Segments that overlap
// or touch make one stretch.
type Schedule struct {
	Enabled  bool      `json:"enabled"`
	Display  bool      `json:"display"` // keep the display on too
	Segments []Segment `json:"segments"`
}

// Equal reports whether two schedules are the same plan.
func (s Schedule) Equal(o Schedule) bool {
	return s.Enabled == o.Enabled && s.Display == o.Display && slices.Equal(s.Segments, o.Segments)
}

// Clean returns the schedule without its invalid segments.
func (s Schedule) Clean() Schedule {
	kept := make([]Segment, 0, len(s.Segments))
	for _, seg := range s.Segments {
		if seg.Valid() {
			kept = append(kept, seg)
		}
	}
	s.Segments = kept
	return s
}

// span is one run of a segment on one day.
type span struct{ start, end time.Time }

// spanOn is the run of s that begins on the day offset days from t, if its
// days include that one. Times are read on the wall clock of their day, so
// a run across a clock change is an hour longer or shorter.
func (s Segment) spanOn(t time.Time, offset int) (span, bool) {
	day := t.AddDate(0, 0, offset)
	if !s.Days.Has(day.Weekday()) {
		return span{}, false
	}
	end := clockOn(day, s.To)
	if s.To <= s.From {
		end = clockOn(day.AddDate(0, 0, 1), s.To)
	}
	return span{clockOn(day, s.From), end}, true
}

func clockOn(day time.Time, minutes int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), minutes/60, minutes%60, 0, 0, day.Location())
}

// spansAround are the runs that begin yesterday, today or tomorrow: those
// that can hold t, or touch one that does.
func (s Schedule) spansAround(t time.Time) []span {
	var out []span
	for _, seg := range s.Segments {
		if !seg.Valid() {
			continue
		}
		for offset := -1; offset <= 1; offset++ {
			if sp, ok := seg.spanOn(t, offset); ok {
				out = append(out, sp)
			}
		}
	}
	return out
}

// Active reports the stretch of the plan that holds t: when it began and
// when it ends. Runs that overlap or touch are one stretch, which is
// followed across a day boundary and no further.
func (s Schedule) Active(t time.Time) (start, end time.Time, ok bool) {
	if !s.Enabled {
		return start, end, false
	}
	spans := s.spansAround(t)
	i := slices.IndexFunc(spans, func(sp span) bool { return !t.Before(sp.start) && t.Before(sp.end) })
	if i < 0 {
		return start, end, false
	}
	start, end = spans[i].start, spans[i].end
	for grown := true; grown; {
		grown = false
		for _, sp := range spans {
			if sp.start.Before(start) && !sp.end.Before(start) {
				start, grown = sp.start, true
			}
			if sp.end.After(end) && !sp.start.After(end) {
				end, grown = sp.end, true
			}
		}
	}
	return start, end, true
}

// nextDays is how far ahead Next looks: every segment recurs within a week.
const nextDays = 7

// Next is when a run of the plan next begins after t, for a line such as
// "next: Monday 09:00". It is not ok when the plan is off or has no valid
// segment.
func (s Schedule) Next(t time.Time) (next time.Time, ok bool) {
	if !s.Enabled {
		return next, false
	}
	for _, seg := range s.Segments {
		if !seg.Valid() {
			continue
		}
		for offset := 0; offset <= nextDays; offset++ {
			sp, has := seg.spanOn(t, offset)
			if has && sp.start.After(t) {
				if !ok || sp.start.Before(next) {
					next, ok = sp.start, true
				}
				break
			}
		}
	}
	return next, ok
}
