package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Limits of the lists the settings keep.
const (
	MaxPinned = 8
	MaxRecent = 8
)

// Settings are what the app remembers between launches.
type Settings struct {
	// Version is the version of the file, stamped on every save.
	Version int      `json:"version,omitempty"`
	Pinned  []string `json:"pinned"`
	Recent  []string `json:"recent"`
	// Collapsed lists the sidebar sections that were closed.
	Collapsed []string `json:"collapsed"`
	// Order is the tool IDs in the order the user dragged them into; tools
	// missing from it follow in registry order. Empty is registry order.
	Order []string `json:"order,omitempty"`
	// Sections is the sidebar sections (the pinned one and the categories)
	// in the order the user dragged them into. Empty is the default order.
	Sections []string `json:"sections,omitempty"`
	// Schedules are the plans of the tools that run by a plan, by tool ID.
	Schedules map[string]tool.Schedule `json:"schedules,omitempty"`
	// Displays is whether the tools that can keep the display on do, by tool
	// ID. A tool not in it keeps its own default.
	Displays map[string]bool `json:"displays,omitempty"`
	// SavedPresets are the lengths a person saved as presets, in minutes, by
	// tool ID.
	SavedPresets map[string][]int `json:"presets,omitempty"`
	// Language is the language chosen in the settings; empty follows the
	// system.
	Language string `json:"language,omitempty"`
	// Theme is the appearance chosen in the settings; empty follows the
	// system.
	Theme string `json:"theme,omitempty"`
	// ScreenshotDir is where screenshots are saved. Empty means
	// ~/Pictures/oh-my-toys.
	ScreenshotDir string `json:"screenshotDir,omitempty"`
}

// SetSchedule remembers the plan of a tool.
func (v *Settings) SetSchedule(id string, s tool.Schedule) {
	if v.Schedules == nil {
		v.Schedules = map[string]tool.Schedule{}
	}
	s.Segments = slices.Clone(s.Segments)
	v.Schedules[id] = s
}

// SetDisplay remembers whether a tool keeps the display on.
func (v *Settings) SetDisplay(id string, on bool) {
	if v.Displays == nil {
		v.Displays = map[string]bool{}
	}
	v.Displays[id] = on
}

// Presets are the lengths a person saved for a tool, in the order they were
// saved.
func (v *Settings) Presets(id string) []time.Duration {
	out := make([]time.Duration, len(v.SavedPresets[id]))
	for i, minutes := range v.SavedPresets[id] {
		out[i] = time.Duration(minutes) * time.Minute
	}
	return out
}

// AddPreset saves a length, in whole minutes, as a preset of a tool. A length
// already saved, or no length, is not added again.
func (v *Settings) AddPreset(id string, d time.Duration) {
	minutes := int(d / time.Minute)
	if minutes <= 0 || slices.Contains(v.SavedPresets[id], minutes) {
		return
	}
	if v.SavedPresets == nil {
		v.SavedPresets = map[string][]int{}
	}
	v.SavedPresets[id] = append(v.SavedPresets[id], minutes)
}

// RemovePreset takes a length out of the presets saved for a tool.
func (v *Settings) RemovePreset(id string, d time.Duration) {
	left := slices.DeleteFunc(slices.Clone(v.SavedPresets[id]), func(m int) bool { return m == int(d/time.Minute) })
	if len(left) > 0 {
		v.SavedPresets[id] = left
	} else {
		delete(v.SavedPresets, id)
	}
}

// SettingsVersion is the version of the settings file the app writes. A
// file without one was written before the app kept a version, and still
// reads: a change to how a field means something can then migrate the old
// value instead of guessing what it was.
const SettingsVersion = 1

// Store keeps settings in a JSON file.
type Store struct {
	path string
}

// NewStore keeps settings in the file settings.json of dir.
func NewStore(dir string) *Store {
	return &Store{path: filepath.Join(dir, "settings.json")}
}

// Load reads the settings, or returns def when there are none yet.
func (s *Store) Load(def Settings) (Settings, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return def, nil
	}
	if err != nil {
		return def, fmt.Errorf("read settings: %w", err)
	}
	var out Settings
	if err := json.Unmarshal(b, &out); err != nil {
		return def, fmt.Errorf("parse %s: %w", s.path, err)
	}
	return out, nil
}

// Save writes the settings, replacing the file in one step.
func (s *Store) Save(v Settings) error {
	v.Version = SettingsVersion
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace settings: %w", err)
	}
	return nil
}

// TogglePin pins a tool, or unpins a pinned one; a full list stays as it is.
func (v *Settings) TogglePin(id string) {
	if i := slices.Index(v.Pinned, id); i >= 0 {
		v.Pinned = slices.Delete(v.Pinned, i, i+1)
		return
	}
	if len(v.Pinned) < MaxPinned {
		v.Pinned = append(v.Pinned, id)
	}
}

// Used puts a tool first among the recent ones.
func (v *Settings) Used(id string) {
	v.Recent = slices.DeleteFunc(v.Recent, func(r string) bool { return r == id })
	v.Recent = slices.Insert(v.Recent, 0, id)
	v.Recent = v.Recent[:min(len(v.Recent), MaxRecent)]
}

// IsCollapsed reports whether a sidebar section is closed.
func (v *Settings) IsCollapsed(section string) bool {
	return slices.Contains(v.Collapsed, section)
}

// SetCollapsed opens or closes a sidebar section.
func (v *Settings) SetCollapsed(section string, closed bool) {
	v.Collapsed = slices.DeleteFunc(v.Collapsed, func(s string) bool { return s == section })
	if closed {
		v.Collapsed = append(v.Collapsed, section)
	}
}

// Reset puts the sidebar back to how a first launch shows it: the default
// pinned tools, no custom order, every section open. The language stays.
func (v *Settings) Reset(def Settings) {
	v.Pinned = slices.Clone(def.Pinned)
	v.Order = nil
	v.Sections = nil
	v.Collapsed = nil
}

// MoveWithin moves the tools at rows, which index group, to before index
// to of group, and records the result in Order: only the relative order
// inside the group changes, so tools of other groups stay where they are.
func (v *Settings) MoveWithin(group []string, rows []int, to int) {
	moved := MoveItems(group, rows, to)
	if slices.Equal(moved, group) {
		return
	}
	v.Order = mergeOrder(v.Order, group, moved)
}

// Ordered returns tools in the user's order: those named in Order first,
// in that order, then the rest as given.
func (v *Settings) Ordered(ids []string) []string { return sortByRank(ids, v.Order) }

// OrderedSections returns the sidebar sections in the user's order.
func (v *Settings) OrderedSections(ids []string) []string { return sortByRank(ids, v.Sections) }

// MoveSection moves the section at index from of sections to before index
// to, and records the whole order, so later sections keep their places.
func (v *Settings) MoveSection(sections []string, from, to int) {
	moved := MoveItems(sections, []int{from}, to)
	if !slices.Equal(moved, sections) {
		v.Sections = moved
	}
}

// sortByRank orders ids as rank lists them; ids it does not list follow,
// in the order given.
func sortByRank(ids, rank []string) []string {
	pos := make(map[string]int, len(rank))
	for i, id := range rank {
		pos[id] = i
	}
	out := slices.Clone(ids)
	slices.SortStableFunc(out, func(a, b string) int {
		ra, oka := pos[a]
		rb, okb := pos[b]
		switch {
		case oka && okb:
			return ra - rb
		case oka:
			return -1
		case okb:
			return 1
		}
		return 0
	})
	return out
}

// mergeOrder returns an order that keeps every ID of old, puts the IDs of
// group in the order of moved, and leaves other IDs where they were.
func mergeOrder(old, group, moved []string) []string {
	base := slices.Clone(old)
	for _, id := range group {
		if !slices.Contains(base, id) {
			base = append(base, id)
		}
	}
	next := 0
	for i, id := range base {
		if slices.Contains(group, id) {
			base[i] = moved[next]
			next++
		}
	}
	return base
}

// MoveItems moves the items at rows, in order, to before index to.
func MoveItems(items []string, rows []int, to int) []string {
	taken := make([]bool, len(items))
	moved := make([]string, 0, len(rows))
	for _, r := range rows {
		if r >= 0 && r < len(items) && !taken[r] {
			taken[r] = true
			moved = append(moved, items[r])
		}
	}
	rest := make([]string, 0, len(items)-len(moved))
	insert := to
	for i, it := range items {
		if taken[i] {
			if i < to {
				insert--
			}
			continue
		}
		rest = append(rest, it)
	}
	insert = min(max(insert, 0), len(rest))
	out := slices.Clone(rest[:insert])
	out = append(out, moved...)
	return append(out, rest[insert:]...)
}
