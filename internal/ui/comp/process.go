package comp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// ProcessIcons fetches the OS's icons for executables in the background
// and caches them for the session. A plain binary that only gets the OS's
// generic executable icon keeps the design system's default icon.
type ProcessIcons struct {
	mu      sync.Mutex
	icons   map[string]*ui.Bitmap // nil means the OS had no real icon
	generic [][]byte
	genDone bool
	changed func()
}

// NewProcessIcons makes the cache; changed is called when an icon arrives.
func NewProcessIcons(changed func()) *ProcessIcons {
	return &ProcessIcons{icons: map[string]*ui.Bitmap{}, changed: changed}
}

// ProcessIcon is the 16pt icon in front of a process name: the program's
// own icon from the OS when it has a real one, otherwise the default user
// or system icon.
func (k *Kit) ProcessIcon(c *ui.Context, cache *ProcessIcons, p tool.Process) {
	k.processIcon(c, cache, p, ds.ProcessIcon)
}

// ProcessIconLarge is the 20pt process icon of the palette and headers.
func (k *Kit) ProcessIconLarge(c *ui.Context, cache *ProcessIcons, p tool.Process) {
	k.processIcon(c, cache, p, ds.ProcessIconLg)
}

func (k *Kit) processIcon(c *ui.Context, cache *ProcessIcons, proc tool.Process, size float32) {
	if bmp := cache.lookup(proc.Path); bmp != nil {
		ui.Image(c, bmp).Size(size, size)
		return
	}
	p := ds.P(c)
	svg := k.processPlain
	if proc.System {
		svg = k.processSystem
	}
	ui.Box(c).Size(size, size).Radius(ds.RadiusXs).Background(p.FillControlHover).Center().Shrink(0).
		Children(func() { ui.Icon(c, svg).Size(size, size).TextColor(p.TextSecondary) })
}

func (s *ProcessIcons) lookup(path string) *ui.Bitmap {
	if s == nil || path == "" {
		return nil
	}
	s.mu.Lock()
	bmp, seen := s.icons[path]
	if !seen {
		s.icons[path] = nil
	}
	s.mu.Unlock()
	if !seen {
		go s.fetch(path)
	}
	return bmp
}

const iconPixels = ds.ProcessIconLg * ds.TrayIconScale

func (s *ProcessIcons) fetch(path string) {
	var bmp *ui.Bitmap
	for _, p := range candidates(path) {
		raw, err := mygo.Shell.FileIcon(p, iconPixels)
		if err != nil || len(raw) == 0 || s.isGeneric(raw) {
			continue
		}
		if b, err := ui.DecodeBitmap(raw); err == nil {
			bmp = b
			break
		}
	}
	if bmp == nil {
		return
	}
	s.mu.Lock()
	s.icons[path] = bmp
	s.mu.Unlock()
	if s.changed != nil {
		s.changed()
	}
}

// isGeneric reports whether raw is the icon the OS gives a plain executable
// or a plain file without an icon of its own.
func (s *ProcessIcons) isGeneric(raw []byte) bool {
	for _, g := range s.genericIcons() {
		if len(g) > 0 && bytes.Equal(raw, g) {
			return true
		}
	}
	return false
}

func (s *ProcessIcons) genericIcons() [][]byte {
	s.mu.Lock()
	done, gen := s.genDone, s.generic
	s.mu.Unlock()
	if done {
		return gen
	}
	if exe, err := os.Executable(); err == nil {
		if b, err := mygo.Shell.FileIcon(exe, iconPixels); err == nil {
			gen = append(gen, b)
		}
	}
	if f, err := os.CreateTemp("", "omt-icon-*"); err == nil {
		name := f.Name()
		_ = f.Close()
		if b, err := mygo.Shell.FileIcon(name, iconPixels); err == nil {
			gen = append(gen, b)
		}
		_ = os.Remove(name)
	}
	s.mu.Lock()
	if !s.genDone {
		s.genDone, s.generic = true, gen
	}
	gen = s.generic
	s.mu.Unlock()
	return gen
}

// candidates are the paths whose icons stand for an executable: the binary
// itself, then the nearest enclosing .app bundle when it sits inside one.
func candidates(path string) []string {
	out := []string{path}
	for d := filepath.Dir(path); len(d) > 1 && d != filepath.Dir(d); d = filepath.Dir(d) {
		if strings.HasSuffix(d, ".app") {
			return append(out, d)
		}
	}
	return out
}
