// Package color converts a color between HEX, RGB and HSL, and checks the
// contrast of text in it on white and on black.
package color

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

// ID is the tool's ID and command name.
const ID = "color"

const fieldColor = "color"

// WCAG thresholds for normal text.
const (
	contrastAA  = 4.5
	contrastAAA = 7
)

var (
	hexColor = regexp.MustCompile(`^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	rgbColor = regexp.MustCompile(`^(?:rgba?\()?\s*(\d{1,3})\s*[, ]\s*(\d{1,3})\s*[, ]\s*(\d{1,3})\s*(?:[,/][^)]*)?\)?$`)
	hslColor = regexp.MustCompile(`^hsla?\(\s*(\d{1,3}(?:\.\d+)?)\s*[, ]\s*(\d{1,3}(?:\.\d+)?)%\s*[, ]\s*(\d{1,3}(?:\.\d+)?)%\s*(?:[,/][^)]*)?\)$`)
)

// errUnknownColor is input no format reads.
var errUnknownColor = errors.New("unknown color format")

// RGB is a color of 8-bit channels.
type RGB struct{ R, G, B uint8 }

// Tool is Color.
type Tool struct{}

// New makes the tool.
func New() *Tool { return &Tool{} }

// Manifest describes the tool.
func (*Tool) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:        ID,
		Name:      i18n.Messages.ColorName,
		Summary:   i18n.Messages.ColorSummary,
		Category:  tool.CategoryDev,
		Kind:      tool.KindAction,
		Icon:      "pipette",
		Keywords:  []string{"color", "colour", "hex", "rgb", "hsl", "contrast", "wcag", "取色", "对比度"},
		Platforms: []string{"macos", "windows", "linux"},
	}
}

// Specs declares the tool's one action: a plan that shows a color in
// every format, which reads its input and changes nothing. There is
// nothing to apply.
func (*Tool) Specs() []tool.Spec {
	return []tool.Spec{
		tool.PlanSpec(i18n.Messages.ColorPlanSummary, tool.Schema{Type: tool.TypeObject, Properties: []tool.Property{{
			Name:     fieldColor,
			Schema:   tool.Schema{Type: tool.TypeString, Title: i18n.Messages.ColorField, Hint: i18n.Messages.ColorFieldHint},
			Required: true,
		}}}),
	}
}

// Plan shows the color in every format and its contrast; it changes
// nothing, so it has nothing to apply.
func (*Tool) Plan(_ context.Context, in tool.Input) (tool.Plan, error) {
	s := strings.TrimSpace(in[fieldColor])
	if s == "" {
		return tool.Plan{}, nil
	}
	c, err := Parse(s)
	if err != nil {
		return tool.Plan{}, &tool.Problem{Text: i18n.ColorBadInput(s), Err: err}
	}
	h, sat, l := c.HSL()
	hex := c.Hex()
	return tool.Plan{
		Summary: hex,
		Rows: []tool.Row{
			{Title: "HEX", Detail: hex, Value: hex},
			{Title: "RGB", Detail: c.String(), Value: c.String()},
			{Title: "HSL", Detail: fmt.Sprintf("hsl(%.0f, %.0f%%, %.0f%%)", h, sat, l), Value: fmt.Sprintf("hsl(%.0f, %.0f%%, %.0f%%)", h, sat, l)},
			contrastRow(i18n.ColorOnWhite(), Contrast(c, RGB{255, 255, 255})),
			contrastRow(i18n.ColorOnBlack(), Contrast(c, RGB{})),
		},
	}, nil
}

// Apply has nothing to do: the plan only shows information.
func (*Tool) Apply(context.Context, tool.Plan) (tool.Result, error) {
	return tool.Result{}, nil
}

func contrastRow(title string, ratio float64) tool.Row {
	verdict, tone := i18n.ColorFailsAA(), tool.ToneDanger
	switch {
	case ratio >= contrastAAA:
		verdict, tone = i18n.ColorPassesAAA(), tool.ToneSuccess
	case ratio >= contrastAA:
		verdict, tone = i18n.ColorPassesAA(), tool.ToneSuccess
	}
	return tool.Row{Title: title, Detail: i18n.ColorRatio(ratio, verdict), Tone: tone}
}

// Parse reads #rgb, #rrggbb, rgb(r, g, b), "r, g, b" and hsl(h, s%, l%).
func Parse(s string) (RGB, error) {
	if m := hexColor.FindStringSubmatch(s); m != nil {
		return parseHex(m[1]), nil
	}
	if m := rgbColor.FindStringSubmatch(s); m != nil {
		return parseRGB(m[1:])
	}
	if m := hslColor.FindStringSubmatch(strings.ToLower(s)); m != nil {
		h, _ := strconv.ParseFloat(m[1], 64)
		sat, _ := strconv.ParseFloat(m[2], 64)
		l, _ := strconv.ParseFloat(m[3], 64)
		return FromHSL(h, sat, l), nil
	}
	return RGB{}, fmt.Errorf("%q: %w", s, errUnknownColor)
}

func parseHex(h string) RGB {
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var ch [3]uint8
	for i := range ch {
		v, _ := strconv.ParseUint(h[2*i:2*i+2], 16, 8) // two hex digits: at most 255
		ch[i] = uint8(v)
	}
	return RGB{ch[0], ch[1], ch[2]}
}

func parseRGB(parts []string) (RGB, error) {
	var ch [3]uint8
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 8) // fails above 255
		if err != nil {
			return RGB{}, fmt.Errorf("channel %q: %w", p, errUnknownColor)
		}
		ch[i] = uint8(n)
	}
	return RGB{ch[0], ch[1], ch[2]}, nil
}

// Hex is the color as #rrggbb.
func (c RGB) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// String is the color as rgb(r, g, b).
func (c RGB) String() string { return fmt.Sprintf("rgb(%d, %d, %d)", c.R, c.G, c.B) }

// HSL is the color's hue in degrees, and its saturation and lightness in
// percent.
func (c RGB) HSL() (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	l = (hi + lo) / 2
	d := hi - lo
	if d == 0 {
		return 0, 0, l * 100
	}
	s = d / (1 - math.Abs(2*l-1))
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s * 100, l * 100
}

// FromHSL makes a color of a hue in degrees and a saturation and lightness
// in percent.
func FromHSL(h, s, l float64) RGB {
	s, l = s/100, l/100
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	ch := func(v float64) uint8 { return uint8(math.Round((v + m) * 255)) }
	return RGB{ch(r), ch(g), ch(b)}
}

// Contrast is the WCAG contrast ratio of two colors, from 1 to 21.
func Contrast(a, b RGB) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

func luminance(c RGB) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}
