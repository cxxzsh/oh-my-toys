// Package comp draws the design system's components with MyGo's widget
// bases, which have behaviour and accessibility but no look of their own.
// Each file is one component, named as in design/components. A Kit holds
// what the components load once: the glyph set, the tool and app icon
// images and the default process icons.
package comp

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/design"
	"github.com/cxxzsh/oh-my-toys/internal/ui/icons"
)

// Kit is what the components share. Make one at start with NewKit; views
// only read it.
type Kit struct {
	glyphs        *icons.Set
	bitmaps       map[string]*ui.Bitmap
	processPlain  *ui.SVG
	processSystem *ui.SVG
}

const (
	appIconLight = "assets/app-icon/app-icon-light.png"
	appIconDark  = "assets/app-icon/app-icon-dark.png"
	toolIconDir  = "assets/tool-icons/"
)

// NewKit loads the glyphs, the images and the default process icons.
func NewKit() (*Kit, error) {
	glyphs, err := icons.Load()
	if err != nil {
		return nil, fmt.Errorf("load glyphs: %w", err)
	}
	k := &Kit{glyphs: glyphs, bitmaps: map[string]*ui.Bitmap{}}
	if err := k.loadBitmaps(); err != nil {
		return nil, err
	}
	if k.processPlain, err = parseSVG("assets/process-icon/process-default.svg"); err != nil {
		return nil, err
	}
	if k.processSystem, err = parseSVG("assets/process-icon/process-system.svg"); err != nil {
		return nil, err
	}
	return k, nil
}

func (k *Kit) loadBitmaps() error {
	names := []string{appIconLight, appIconDark}
	arts, err := design.Names(toolIconDir)
	if err != nil {
		return fmt.Errorf("list tool icons: %w", err)
	}
	for _, a := range arts {
		names = append(names, toolIconDir+a+".png")
	}
	for _, n := range names {
		data, err := design.Read(n)
		if err != nil {
			return err
		}
		b, err := ui.DecodeBitmap(data)
		if err != nil {
			return fmt.Errorf("decode %s: %w", n, err)
		}
		k.bitmaps[n] = b
	}
	return nil
}

func parseSVG(name string) (*ui.SVG, error) {
	data, err := design.Read(name)
	if err != nil {
		return nil, err
	}
	svg, err := ui.ParseSVG(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	return svg, nil
}

// HasGlyph reports whether a glyph name is in the design system's set.
func (k *Kit) HasGlyph(name string) bool { return k.glyphs.Has(name) }

// Glyph is an interface glyph filled with a text colour: a mask of the
// Material Symbols SVG at the given size.
func (k *Kit) Glyph(c *ui.Context, name string, size float32, color ui.Color) ui.Element {
	return ui.Icon(c, k.glyphs.Get(name)).Size(size, size).TextColor(color)
}

// AppIcon is the app icon at a size, light or dark by the theme.
func (k *Kit) AppIcon(c *ui.Context, size float32) ui.Element {
	name := appIconLight
	if c.Theme().Dark {
		name = appIconDark
	}
	return ui.Image(c, k.bitmaps[name]).Size(size, size)
}
