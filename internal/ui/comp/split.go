package comp

import (
	"github.com/egoist/mygo/ui"
)

// Split lays first and second side by side, with a line between them that
// a person drags, or moves with the arrows while it has the focus. size is
// first's width in points; a split too narrow for it gives first less and
// keeps size for when there is room again.
func Split(c *ui.Context, size *float32, first, second func()) ui.Element {
	return ui.Split(c, size, first, second)
}

// SplitPercent is Split with p the share of the room first takes, from 0
// to 100, so that both panes grow and shrink with the window.
func SplitPercent(c *ui.Context, p *float32, first, second func()) ui.Element {
	return ui.SplitPercent(c, p, first, second)
}

// SplitVertical is Split with first above second: size is its height.
func SplitVertical(c *ui.Context, size *float32, first, second func()) ui.Element {
	return ui.SplitVertical(c, size, first, second)
}
