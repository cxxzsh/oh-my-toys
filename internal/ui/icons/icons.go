// Package icons looks the design system's interface glyphs up by name. The
// glyphs are Material Symbols Outlined SVGs, embedded in the app; tool
// manifests refer to them by their snake_case names, and an unknown name
// falls back to Fallback.
package icons

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/design"
)

// Fallback is the glyph of a name the set does not have.
const Fallback = "apps"

const glyphDir = "assets/glyphs"

// Set holds the parsed glyphs. Parse them once, at start, not in a view.
type Set struct {
	byName map[string]*ui.SVG
}

// Load parses every glyph of the design system.
func Load() (*Set, error) {
	names, err := design.Names(glyphDir)
	if err != nil {
		return nil, fmt.Errorf("list glyphs: %w", err)
	}
	s := &Set{byName: make(map[string]*ui.SVG, len(names))}
	for _, name := range names {
		data, err := design.Read(glyphDir + "/" + name + ".svg")
		if err != nil {
			return nil, fmt.Errorf("read glyph %s: %w", name, err)
		}
		svg, err := ui.ParseSVG(data)
		if err != nil {
			return nil, fmt.Errorf("parse glyph %s: %w", name, err)
		}
		s.byName[name] = svg
	}
	if s.byName[Fallback] == nil {
		return nil, fmt.Errorf("glyph %s is missing", Fallback)
	}
	return s, nil
}

// Has reports whether the set has a glyph of the name.
func (s *Set) Has(name string) bool {
	_, ok := s.byName[name]
	return ok
}

// Get returns the glyph of a name, or the fallback glyph.
func (s *Set) Get(name string) *ui.SVG {
	if g, ok := s.byName[name]; ok {
		return g
	}
	return s.byName[Fallback]
}
