package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// StartSwitch carries out a start action on a switch tool. A start whose
// input asks for a length or a time of day (tool.ParseHold) runs that long,
// and the length is returned; any other start uses the settings used last,
// and returns 0. An input that says whether to keep the display on
// (tool.ParseDisplay) first makes that the tool's choice, as the switch on
// its page does.
func StartSwitch(ctx context.Context, sw tool.Switch, in tool.Input, now time.Time) (time.Duration, error) {
	d, err := tool.ParseHold(in, now)
	if err != nil {
		return 0, err
	}
	if err := chooseDisplay(sw, in); err != nil {
		return 0, err
	}
	if d == 0 {
		return 0, sw.Start(ctx)
	}
	timed, ok := sw.(tool.Timed)
	if !ok {
		return 0, fmt.Errorf("%s cannot run for a set time", sw.Manifest().ID)
	}
	return d, timed.StartFor(ctx, d)
}

// chooseDisplay gives a tool the display choice a start's input makes.
func chooseDisplay(sw tool.Switch, in tool.Input) error {
	on, given, err := tool.ParseDisplay(in)
	if err != nil || !given {
		return err
	}
	screened, ok := sw.(tool.Screened)
	if !ok {
		return fmt.Errorf("%s cannot choose whether the display stays on", sw.Manifest().ID)
	}
	screened.SetDisplay(on)
	return nil
}

// problemText is what a person should read of an error: the text of a
// problem with their input, else the error itself.
func problemText(err error) string {
	var p *tool.Problem
	if errors.As(err, &p) && p.Text != "" {
		return p.Text
	}
	return err.Error()
}
