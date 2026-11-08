// Package core holds what the desktop app and the command line share above
// the tools: the registry, the search index, the runner that keeps one
// tool's failure inside that tool, change events and the settings store.
package core

import (
	"fmt"
	"log"
	"runtime"
	"slices"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Registry holds the tools of one program, in the order they were given.
type Registry struct {
	tools []tool.Tool
	byID  map[string]tool.Tool
	index *Index
}

// NewRegistry registers tools, in order, leaving out those that do not run
// on this platform. IDs must be unique.
func NewRegistry(tools ...tool.Tool) (*Registry, error) {
	r := &Registry{byID: map[string]tool.Tool{}}
	platform := platformName()
	for _, t := range tools {
		m := t.Manifest()
		if len(m.Platforms) > 0 && !slices.Contains(m.Platforms, platform) {
			continue
		}
		if _, dup := r.byID[m.ID]; dup {
			return nil, fmt.Errorf("duplicate tool id %q", m.ID)
		}
		r.tools = append(r.tools, t)
		r.byID[m.ID] = t
	}
	r.index = newIndex(r.tools)
	return r, nil
}

// All returns every tool.
func (r *Registry) All() []tool.Tool { return r.tools }

// Get returns a tool by its ID, or nil.
func (r *Registry) Get(id string) tool.Tool { return r.byID[id] }

// Index returns the search index of the tools.
func (r *Registry) Index() *Index { return r.index }

// InCategory returns the tools of a category.
func (r *Registry) InCategory(c tool.Category) []tool.Tool {
	var out []tool.Tool
	for _, t := range r.tools {
		if t.Manifest().Category == c {
			out = append(out, t)
		}
	}
	return out
}

// Running returns the switch tools that are on.
func (r *Registry) Running() []tool.Switch {
	var out []tool.Switch
	for _, t := range r.tools {
		if s, ok := t.(tool.Switch); ok && s.Status().On {
			out = append(out, s)
		}
	}
	return out
}

// IsRunning reports whether a tool is a switch tool that is on.
func IsRunning(t tool.Tool) bool {
	s, ok := t.(tool.Switch)
	return ok && s.Status().On
}

// StopAll stops every switch tool that is on, so that a tool which changed
// the system leaves it as it was when the app quits: a proxy left pointing
// at a dead port cuts the person off the network. A tool that fails to stop
// is logged, and the others still stop.
func (r *Registry) StopAll() {
	for _, t := range r.tools {
		stopSafely(t)
	}
}

// stopSafely stops one tool when it is on, keeping a panic or a failure
// inside it: the tools after it still stop, and the system a tool changed
// is still put back.
func stopSafely(t tool.Tool) {
	id := ""
	defer func() {
		if v := recover(); v != nil {
			log.Printf("%s: stop: %v", id, v)
		}
	}()
	s, ok := t.(tool.Switch)
	if !ok {
		return
	}
	id = t.Manifest().ID
	if !s.Status().On {
		return
	}
	if err := s.Stop(); err != nil {
		log.Printf("%s: stop: %v", id, err)
	}
}

func platformName() string {
	if runtime.GOOS == "darwin" {
		return "macos"
	}
	return runtime.GOOS
}
