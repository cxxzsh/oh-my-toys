package tool

import (
	"errors"
	"testing"
	"time"
)

func TestParseHold(t *testing.T) {
	now := time.Date(2026, time.March, 9, 17, 42, 10, 0, time.UTC)
	cases := []struct {
		name string
		in   Input
		want time.Duration
		bad  bool
	}{
		{"neither: the settings used last", Input{}, 0, false},
		{"blank is neither", Input{PropFor: "  ", PropUntil: ""}, 0, false},
		{"a length", Input{PropFor: "90m"}, 90 * time.Minute, false},
		{"hours and minutes", Input{PropFor: "1h30m"}, 90 * time.Minute, false},
		{"the longest length", Input{PropFor: "99h59m"}, MaxHold, false},
		{"a second over the longest", Input{PropFor: "99h59m1s"}, 0, true},
		{"zero", Input{PropFor: "0"}, 0, true},
		{"negative", Input{PropFor: "-5m"}, 0, true},
		{"no unit", Input{PropFor: "90"}, 0, true},
		{"later today", Input{PropUntil: "18:30"}, 47*time.Minute + 50*time.Second, false},
		{"surrounding space", Input{PropUntil: " 18:30 "}, 47*time.Minute + 50*time.Second, false},
		{"already passed is tomorrow", Input{PropUntil: "09:00"}, 15*time.Hour + 17*time.Minute + 50*time.Second, false},
		{"this minute is tomorrow", Input{PropUntil: "17:42"}, 24*time.Hour - 10*time.Second, false},
		{"midnight", Input{PropUntil: "00:00"}, 6*time.Hour + 17*time.Minute + 50*time.Second, false},
		{"hour 24", Input{PropUntil: "24:00"}, 0, true},
		{"minute 60", Input{PropUntil: "18:60"}, 0, true},
		{"not a time", Input{PropUntil: "tea time"}, 0, true},
		{"both", Input{PropFor: "1h", PropUntil: "18:30"}, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseHold(c.in, now)
			if c.bad {
				var p *Problem
				if !errors.As(err, &p) || p.Text == "" || !errors.Is(err, ErrHold) {
					t.Fatalf("got %v, %v: want a problem with text that wraps ErrHold", got, err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Errorf("got %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

// TestParseHoldAcrossTheClockChange: a time of day is read on the wall
// clock of its day, so across a clock change the length is not a multiple
// of 24 hours.
func TestParseHoldAcrossTheClockChange(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no time zone data:", err)
	}
	cases := []struct {
		name  string
		now   time.Time
		until string
		want  time.Duration
	}{
		{"noon to noon over the change forward", time.Date(2026, time.March, 7, 12, 0, 0, 0, zone), "12:00", 23 * time.Hour},
		{"noon to noon after it", time.Date(2026, time.March, 8, 12, 0, 0, 0, zone), "12:00", 24 * time.Hour},
		{"night to morning over it", time.Date(2026, time.March, 8, 0, 30, 0, 0, zone), "08:30", 7 * time.Hour},
	}
	for _, c := range cases {
		got, err := ParseHold(Input{PropUntil: c.until}, c.now)
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}
