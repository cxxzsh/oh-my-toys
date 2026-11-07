package comp

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Text is a text in a type style and colour.
func Text(c *ui.Context, s string, st ds.TypeStyle, color ui.Color) ui.Element {
	return ds.Style(ui.Text(c, s), st).TextColor(color)
}

// Eyebrow is a section label: mono, uppercase, text-muted.
func Eyebrow(c *ui.Context, s string) ui.Element {
	return Text(c, strings.ToUpper(s), ds.Eyebrow, ds.P(c).TextMuted).SingleLine()
}

// Hint is a short mono hint in text-muted: a shortcut in a footer, a count.
func Hint(c *ui.Context, s string) ui.Element {
	return Text(c, s, ds.Kbd, ds.P(c).TextMuted).SingleLine().FontFeatures(ds.Tabular)
}

// Value is a mono value in a colour: a pid, a status, a path.
func Value(c *ui.Context, s string, color ui.Color) ui.Element {
	return Text(c, s, ds.Meta, color).SingleLine().FontFeatures(ds.Tabular)
}
