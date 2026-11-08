package shell

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// displayOption is the choice to keep the display on, apart from how long
// the tool runs: every mode below it, and the run in progress, follow it.
func (a *App) displayOption(c *ui.Context, t tool.Screened) {
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		on := t.Display()
		if comp.SwitchCompact(c.Key("display"), &on, i18n.SwitchDisplay()).Changed() {
			a.setDisplay(t, on)
		}
		comp.Text(c, i18n.SwitchDisplay(), ds.Body, ds.P(c).TextPrimary)
	})
}

// setDisplay gives a tool the display choice and keeps it for the next
// launch.
func (a *App) setDisplay(t tool.Screened, on bool) {
	t.SetDisplay(on)
	a.settings.SetDisplay(t.Manifest().ID, on)
	a.save()
}

// applyDisplays gives each tool that can keep the display on the choice
// kept for it.
func (a *App) applyDisplays() {
	for _, t := range a.reg.All() {
		if s, ok := t.(tool.Screened); ok {
			if on, kept := a.settings.Displays[t.Manifest().ID]; kept {
				s.SetDisplay(on)
			}
		}
	}
}
