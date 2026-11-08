package comp

import (
	"github.com/egoist/mygo/ui"
)

// Tabs is a row of tabs with an underline under the chosen one, the
// design system's look for a pane with several views. A click chooses a
// tab, as do the arrows, Home and End while one has the focus; label is
// the accessible name of the row. Ask the returned element Changed for a
// new choice.
func Tabs(c *ui.Context, selected *int, label string, options ...string) ui.Element {
	return ui.Tabs(c, selected, options...).Label(label)
}
