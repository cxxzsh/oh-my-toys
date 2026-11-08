package tool

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
)

// MaxHold is the longest a run may be asked to last.
const MaxHold = 99*time.Hour + 59*time.Minute

// The properties of the input of a timed start.
const (
	PropFor     = "for"
	PropUntil   = "until"
	PropDisplay = "display"
)

// clockLayout is a time of day in 24 hours: 18:30.
const clockLayout = "15:04"

// ErrHold marks a timed start whose length, time of day or display is not
// valid.
var ErrHold = errors.New("invalid hold")

// ParseHold works out how long a start asks to run from its input: a
// length such as 90m (a Go duration), or the next time of day such as
// 18:30, which is tomorrow's when today's has passed. It returns 0 and no
// error when the input asks for neither, which is a start with the
// settings used last. A bad input is a *Problem that wraps ErrHold.
func ParseHold(in Input, now time.Time) (time.Duration, error) {
	length, clock := strings.TrimSpace(in[PropFor]), strings.TrimSpace(in[PropUntil])
	switch {
	case length == "" && clock == "":
		return 0, nil
	case length != "" && clock != "":
		return 0, holdProblem(i18n.HoldEither(), "both")
	case length != "":
		return holdLength(length)
	}
	return holdUntil(clock, now)
}

func holdLength(text string) (time.Duration, error) {
	d, err := time.ParseDuration(text)
	if err != nil || d <= 0 || d > MaxHold {
		return 0, holdProblem(i18n.HoldLength(), text)
	}
	return d, nil
}

func holdUntil(text string, now time.Time) (time.Duration, error) {
	at, err := time.Parse(clockLayout, text)
	if err != nil {
		return 0, holdProblem(i18n.HoldClock(), text)
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), at.Hour(), at.Minute(), 0, 0, now.Location())
	if !end.After(now) {
		end = end.AddDate(0, 0, 1)
	}
	return end.Sub(now), nil
}

func holdProblem(text, value string) error {
	return &Problem{Text: text, Err: fmt.Errorf("%w: %q", ErrHold, value)}
}

// ParseDisplay reads whether a start asks to keep the display on: on or off
// (also true, false, yes, no, 1, 0). given is false when the input does not
// say, which leaves the choice made last. A bad input is a *Problem that
// wraps ErrHold.
func ParseDisplay(in Input) (on, given bool, err error) {
	text := strings.ToLower(strings.TrimSpace(in[PropDisplay]))
	switch text {
	case "":
		return false, false, nil
	case "on", "true", "yes", "1":
		return true, true, nil
	case "off", "false", "no", "0":
		return false, true, nil
	}
	return false, false, holdProblem(i18n.HoldDisplay(), text)
}
