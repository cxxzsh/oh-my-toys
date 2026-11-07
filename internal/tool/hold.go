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
	PropFor   = "for"
	PropUntil = "until"
)

// clockLayout is a time of day in 24 hours: 18:30.
const clockLayout = "15:04"

// ErrHold marks a timed start whose length or time of day is not valid.
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
