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
