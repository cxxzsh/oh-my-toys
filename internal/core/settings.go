package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// Limits of the lists the settings keep.
const (
	MaxPinned = 8
	MaxRecent = 8
)

// Settings are what the app remembers between launches.
type Settings struct {
	Pinned []string `json:"pinned"`
	Recent []string `json:"recent"`
	// Collapsed lists the sidebar sections that were closed.
	Collapsed []string `json:"collapsed"`
	// Order is the tool IDs in the order the user dragged them into; tools
	// missing from it follow in registry order. Empty is registry order.
	Order []string `json:"order,omitempty"`
	// Sections is the sidebar sections (the pinned one and the categories)
	// in the order the user dragged them into. Empty is the default order.
	Sections []string `json:"sections,omitempty"`
	// Language is the language chosen in the settings; empty follows the
	// system.
	Language string `json:"language,omitempty"`
}

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
