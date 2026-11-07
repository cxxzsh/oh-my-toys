package shell

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// The tabs of the modes panel of a switch tool.
const (
	tabPresets = iota
	tabCustom
)

// The ways to say how long a custom run lasts.
const (
	holdLength = iota // hours and minutes
	holdClock         // a time of day
)

// notALength is what the length of a run is set to when its fields hold
// something other than whole numbers: any text ParseHold refuses.
const notALength = "?"

// switchForm is what a person has typed into a switch tool's page. It
// lives as long as the app, so a tool's page keeps its tab and its fields.
type switchForm struct {
	tab     int
	mode    int // holdLength or holdClock
	hours   string
	minutes string
	clock   string
}

func (a *App) switchForm(id string) *switchForm {
	f, ok := a.switchForms[id]
	if !ok {
		f = &switchForm{}
		a.switchForms[id] = f
	}
	return f
}

// input is the input of the start the form asks for. blank is true while
// nothing is typed in, which is not yet a mistake.
func (f *switchForm) input() (in tool.Input, blank bool) {
	if f.mode == holdClock {
		text := strings.TrimSpace(f.clock)
		return tool.Input{tool.PropUntil: text}, text == ""
	}
	h, m := strings.TrimSpace(f.hours), strings.TrimSpace(f.minutes)
	if h == "" && m == "" {
		return nil, true
	}
	if !wholeNumber(h) || !wholeNumber(m) {
		return tool.Input{tool.PropFor: notALength}, false
	}
	return tool.Input{tool.PropFor: zeroIfEmpty(h) + "h" + zeroIfEmpty(m) + "m"}, false
}

// wholeNumber reports whether s is digits only, or empty.
func wholeNumber(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return !unicode.IsDigit(r) }) < 0
}

func zeroIfEmpty(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// holdAsk is how long the form asks to run from now, or what is wrong
// with it.
type holdAsk struct {
	d     time.Duration
	blank bool
	err   string
}

func (f *switchForm) ask(now time.Time) holdAsk {
	in, blank := f.input()
	if blank {
		return holdAsk{blank: true}
	}
	d, err := tool.ParseHold(in, now)
	if err != nil {
		return holdAsk{err: f.errorText(err)}
	}
	return holdAsk{d: d}
}

// errorText says what is wrong in the words of the field the person used.
func (f *switchForm) errorText(err error) string {
	if f.mode == holdLength {
		return i18n.SwitchBadLength()
	}
	var p *tool.Problem
	if errors.As(err, &p) {
		return p.Text
	}
	return err.Error()
}

// customTab is the form of a custom run: a length or a time of day, what
// it comes to, and the button that starts it.
func (a *App) customTab(c *ui.Context, t tool.Timed, f *switchForm) {
	h := f.ask(time.Now())
	modes := []string{i18n.SwitchModeLength(), i18n.SwitchModeClock()}
	ui.Row(c).AlignItems(ui.Center).Children(func() { comp.Segmented(c.Key("custom-mode"), &f.mode, modes, i18n.SwitchCustom()) })
	submitted := false
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.End).Children(func() {
		if f.mode == holdClock {
			submitted = a.clockField(c, f)
		} else {
			submitted = a.lengthFields(c, f)
		}
		ui.Spacer(c)
		ok := h.d > 0
		opts := comp.ButtonOpts{Kind: comp.Primary, Disabled: !ok}
		if a.kit.Button(c.Key("custom-start"), i18n.SwitchStart(), opts).Clicked() && ok {
			a.startFor(t, h.d)
		}
	})
	if submitted && h.d > 0 {
		a.startFor(t, h.d)
	}
	a.customPreview(c, h)
}

func (a *App) lengthFields(c *ui.Context, f *switchForm) (submitted bool) {
	for _, field := range []struct {
		key, label string
		value      *string
	}{
		{"custom-hours", i18n.SwitchHours(), &f.hours},
		{"custom-minutes", i18n.SwitchMinutes(), &f.minutes},
	} {
		ui.Column(c.Key(field.key)).Width(ds.CustomFieldWidth).Children(func() {
			in := comp.TextField(c, field.value, comp.FieldOpts{Label: field.label, Placeholder: "0", Mono: true})
			submitted = submitted || in.Submitted()
		})
	}
	return submitted
}

func (a *App) clockField(c *ui.Context, f *switchForm) (submitted bool) {
	ui.Column(c.Key("custom-clock")).Width(ds.ClockFieldWidth).Children(func() {
		in := comp.TextField(c, &f.clock, comp.FieldOpts{Label: i18n.SwitchClock(), Placeholder: "18:30", Mono: true})
		submitted = in.Submitted()
	})
	return submitted
}

// customPreview says what the form comes to, or what is wrong with it.
func (a *App) customPreview(c *ui.Context, h holdAsk) {
	p := ds.P(c)
	switch {
	case h.err != "":
		comp.Text(c, h.err, ds.Caption, p.Danger)
	case h.blank:
		comp.Text(c, i18n.SwitchCustomHint(), ds.Caption, p.TextInactive)
	default:
		end := time.Now().Add(h.d).Format(time.TimeOnly)
		comp.Text(c, i18n.SwitchEndsAt(end, tool.Clock(h.d.Round(time.Second))), ds.Caption, p.TextInactive)
	}
}
