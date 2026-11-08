package tool

import (
	"slices"
	"testing"
	"time"
)

func clk(h, m int) int { return h*60 + m }

// at is a time on Monday 9 March 2026 (a Monday) plus days.
func at(days, h, m int) time.Time {
	return time.Date(2026, time.March, 9+days, h, m, 0, 0, time.UTC)
}

func TestSegmentValid(t *testing.T) {
	cases := []struct {
		name string
		seg  Segment
		want bool
	}{
		{"a working day", Segment{Workdays, clk(9, 0), clk(18, 0)}, true},
		{"past midnight", Segment{AllDays, clk(22, 0), clk(6, 0)}, true},
		{"to midnight", Segment{AllDays, clk(22, 0), 0}, true},
		{"no day", Segment{0, clk(9, 0), clk(18, 0)}, false},
		{"a day that does not exist", Segment{1 << 7, clk(9, 0), clk(18, 0)}, false},
		{"from equals to", Segment{AllDays, clk(9, 0), clk(9, 0)}, false},
		{"24:00", Segment{AllDays, clk(9, 0), MinutesPerDay}, false},
		{"negative", Segment{AllDays, -1, clk(9, 0)}, false},
	}
	for _, c := range cases {
		if got := c.seg.Valid(); got != c.want {
			t.Errorf("%s: Valid is %v, want %v", c.name, got, c.want)
		}
	}
}

func TestScheduleActive(t *testing.T) {
	work := Schedule{Enabled: true, Segments: []Segment{
		{Workdays, clk(9, 0), clk(12, 0)},
		{Workdays, clk(14, 0), clk(18, 0)},
	}}
	night := Schedule{Enabled: true, Segments: []Segment{{AllDays, clk(22, 0), clk(6, 0)}}}
	chain := Schedule{Enabled: true, Segments: []Segment{
		{AllDays, clk(9, 0), clk(12, 0)}, {AllDays, clk(11, 0), clk(14, 0)}, {AllDays, clk(14, 0), clk(15, 0)},
	}}
	friNight := Schedule{Enabled: true, Segments: []Segment{{1 << time.Friday, clk(22, 0), clk(6, 0)}}}
	cases := []struct {
		name       string
		s          Schedule
		now        time.Time
		start, end time.Time
		ok         bool
	}{
		{"in the morning", work, at(0, 10, 30), at(0, 9, 0), at(0, 12, 0), true},
		{"the first minute", work, at(0, 9, 0), at(0, 9, 0), at(0, 12, 0), true},
		{"the end is not in it", work, at(0, 12, 0), time.Time{}, time.Time{}, false},
		{"at lunch", work, at(0, 13, 0), time.Time{}, time.Time{}, false},
		{"in the afternoon", work, at(0, 14, 0), at(0, 14, 0), at(0, 18, 0), true},
		{"on a Saturday", work, at(5, 10, 0), time.Time{}, time.Time{}, false},
		{"off", Schedule{Segments: work.Segments}, at(0, 10, 0), time.Time{}, time.Time{}, false},
		{"before midnight", night, at(0, 23, 0), at(0, 22, 0), at(1, 6, 0), true},
		{"after midnight, in yesterday's run", night, at(1, 3, 0), at(0, 22, 0), at(1, 6, 0), true},
		{"in the morning after a night", night, at(1, 7, 0), time.Time{}, time.Time{}, false},
		{"overlapping runs are one", chain, at(0, 11, 30), at(0, 9, 0), at(0, 15, 0), true},
		{"touching runs are one", chain, at(0, 14, 30), at(0, 9, 0), at(0, 15, 0), true},
		{"Friday's night runs into Saturday", friNight, at(5, 2, 0), at(4, 22, 0), at(5, 6, 0), true},
		{"but Saturday's own night does not", friNight, at(5, 23, 0), time.Time{}, time.Time{}, false},
		{"no segments", Schedule{Enabled: true}, at(0, 10, 0), time.Time{}, time.Time{}, false},
		{"an invalid segment is ignored", Schedule{Enabled: true, Segments: []Segment{{AllDays, 600, 600}}}, at(0, 10, 0), time.Time{}, time.Time{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end, ok := c.s.Active(c.now)
			if ok != c.ok || !start.Equal(c.start) || !end.Equal(c.end) {
				t.Errorf("Active(%v) = %v, %v, %v; want %v, %v, %v", c.now, start, end, ok, c.start, c.end, c.ok)
			}
		})
	}
}

func TestScheduleNext(t *testing.T) {
	work := Schedule{Enabled: true, Segments: []Segment{
		{Workdays, clk(14, 0), clk(18, 0)},
		{Workdays, clk(9, 0), clk(12, 0)},
	}}
	cases := []struct {
		name string
		s    Schedule
		now  time.Time
		want time.Time
		ok   bool
	}{
		{"before the first", work, at(0, 7, 0), at(0, 9, 0), true},
		{"between the two", work, at(0, 13, 0), at(0, 14, 0), true},
		{"after the last, Friday", work, at(4, 19, 0), at(7, 9, 0), true},
		{"on the weekend", work, at(5, 10, 0), at(7, 9, 0), true},
		{"as one begins is the next one", work, at(0, 9, 0), at(0, 14, 0), true},
		{"off", Schedule{Segments: work.Segments}, at(0, 7, 0), time.Time{}, false},
		{"nothing valid", Schedule{Enabled: true, Segments: []Segment{{0, 600, 700}}}, at(0, 7, 0), time.Time{}, false},
		{"one day a week", Schedule{Enabled: true, Segments: []Segment{{1 << time.Monday, clk(9, 0), clk(10, 0)}}}, at(0, 11, 0), at(7, 9, 0), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := c.s.Next(c.now)
			if ok != c.ok || !got.Equal(c.want) {
				t.Errorf("Next(%v) = %v, %v; want %v, %v", c.now, got, ok, c.want, c.ok)
			}
		})
	}
}

// TestScheduleAcrossTheClockChange: times are on the wall clock, so the
// run of a plan over the night the clocks go forward ends at its wall time,
// an hour sooner by the clock on the wall.
func TestScheduleAcrossTheClockChange(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no time zone data:", err)
	}
	s := Schedule{Enabled: true, Segments: []Segment{{AllDays, clk(0, 30), clk(6, 0)}}}
	now := time.Date(2026, time.March, 8, 4, 0, 0, 0, zone) // after 02:00 became 03:00
	start, end, ok := s.Active(now)
	if !ok || end.Hour() != 6 || end.Sub(start) != 4*time.Hour+30*time.Minute {
		t.Fatalf("Active = %v, %v, %v: want a run of 4h30m real time ending at 06:00", start, end, ok)
	}
}

func TestScheduleClean(t *testing.T) {
	s := Schedule{Enabled: true, Segments: []Segment{{AllDays, 600, 600}, {AllDays, 600, 700}, {0, 600, 700}, {AllDays, 700, 800}}}
	got := s.Clean()
	if len(got.Segments) != 2 {
		t.Fatalf("%d segments, want 2: the ones with no length and no day are dropped", len(got.Segments))
	}
	for _, seg := range got.Segments {
		if !seg.Valid() {
			t.Errorf("kept an invalid segment %+v", seg)
		}
	}
}

func TestWeekdaysWith(t *testing.T) {
	w := Weekdays(0).With(time.Monday, true).With(time.Sunday, true)
	if !w.Has(time.Monday) || !w.Has(time.Sunday) || w.Has(time.Tuesday) {
		t.Fatalf("set %b", w)
	}
	if w = w.With(time.Monday, false); w.Has(time.Monday) {
		t.Fatalf("Monday not taken out: %b", w)
	}
}

func TestScheduleCoverage(t *testing.T) {
	night := Segment{Days: Weekdays(0).With(time.Friday, true), From: clk(22, 0), To: clk(6, 0)}
	work := Segment{Days: Workdays, From: clk(9, 0), To: clk(18, 0)}
	lunch := Segment{Days: Workdays, From: clk(12, 0), To: clk(19, 0)}
	s := Schedule{Segments: []Segment{night, work, lunch, {Days: 0, From: 1, To: 2}}}
	for _, c := range []struct {
		day  time.Weekday
		want []Range
	}{
		{time.Wednesday, []Range{{clk(9, 0), clk(19, 0)}}},
		{time.Friday, []Range{{clk(9, 0), clk(19, 0)}, {clk(22, 0), MinutesPerDay}}},
		{time.Saturday, []Range{{0, clk(6, 0)}}},
		{time.Sunday, nil},
	} {
		if got := s.Coverage(c.day); !slices.Equal(got, c.want) {
			t.Errorf("%v: %v, want %v", c.day, got, c.want)
		}
	}
}
