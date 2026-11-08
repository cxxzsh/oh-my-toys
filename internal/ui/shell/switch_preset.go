package shell

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// savedPresetsInTray is how many of the lengths a person saved the tray
// shows, which has room for a few chips.
const savedPresetsInTray = 2

// lengthName writes a saved length: "30 min", "2 h", "1 h 30 min".
func lengthName(d time.Duration) string {
	hours, minutes := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case hours == 0:
		return i18n.SwitchLengthMinutes(minutes)
	case minutes == 0:
		return i18n.SwitchLengthHours(hours)
	}
	return i18n.SwitchLengthBoth(hours, minutes)
}

// firstN is the first n of s, or all of it when it has fewer.
func firstN[T any](s []T, n int) []T { return s[:min(n, len(s))] }

// presetNote says why a length typed in cannot be saved: the tool has a
// preset of it already. Without it the button only looks dead.
func (a *App) presetNote(c *ui.Context, t tool.Switch, f *switchForm, d time.Duration) {
	if f.mode == holdLength && d > 0 && a.hasLength(t, d) {
		comp.Text(c, i18n.SwitchPresetExists(), ds.Caption, ds.P(c).TextInactive)
	}
}

// savedTiles are the lengths a person saved as presets, as tiles with a
// button that removes each.
func (a *App) savedTiles(c *ui.Context, t tool.Timed, st tool.Status) {
	id := t.Manifest().ID
	for _, d := range a.settings.Presets(id) {
		key := lengthName(d)
		ui.Row(c.Key("saved:" + key)).Width(ds.PresetBasis).Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
			a.presetTile(c, "preset:"+key, key, runsFor(st, t, d), 0, func() { a.startFor(t, d) })
			opts := comp.ButtonOpts{Kind: comp.Plain, Small: true, Tooltip: i18n.SwitchRemovePreset()}
			if a.kit.Button(c.Key("drop:"+key), removeSpan, opts).Clicked() {
				a.removePreset(id, d)
			}
		})
	}
}

// runsFor reports whether the run in progress is one a person started for
// length d, and none of the tool's own modes.
func runsFor(st tool.Status, t tool.Switch, d time.Duration) bool {
	if !st.On || st.Until.IsZero() || st.Active == tool.ActivePlan {
		return false
	}
	if st.Until.Sub(st.Since).Round(time.Minute) != d {
		return false
	}
	return !slices.ContainsFunc(t.Manifest().Actions, func(act tool.QuickAction) bool { return act.ID == st.Active })
}

// hasLength reports whether the tool already has a preset of length d: one
// of its own modes, or one a person saved.
func (a *App) hasLength(t tool.Switch, d time.Duration) bool {
	m := t.Manifest()
	if slices.ContainsFunc(m.Actions, func(act tool.QuickAction) bool { return act.Length == d }) {
		return true
	}
	return slices.Contains(a.settings.Presets(m.ID), d)
}

// savePreset keeps a length as a preset of the tool.
func (a *App) savePreset(t tool.Switch, d time.Duration) {
	a.settings.AddPreset(t.Manifest().ID, d)
	a.save()
}

func (a *App) removePreset(id string, d time.Duration) {
	a.settings.RemovePreset(id, d)
	a.save()
}
