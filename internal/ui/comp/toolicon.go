package comp

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// IconSize names the ToolIcon sizes.
type IconSize int

// The sizes of tool icons.
const (
	IconSmall  IconSize = iota // 20pt, 5pt corners: palette and tray rows
	IconMedium                 // 28pt, 7pt corners: cards
	IconLarge                  // 48pt, 11pt corners: the tool page header
)

// ToolIconOpts describe a tool icon besides its tool.
type ToolIconOpts struct {
	Size    IconSize
	Running bool // a 10pt live badge (6pt on the small size)
	Tint    bool // the category's colours instead of the monochrome tile
}

type iconMetrics struct{ box, radius, glyph float32 }

func metricsOf(s IconSize) iconMetrics {
	switch s {
	case IconMedium:
		return iconMetrics{ds.IconMd, ds.RadiusIconMd, ds.GlyphMedium}
	case IconLarge:
		return iconMetrics{ds.IconLg, ds.RadiusIconLg, ds.GlyphLarge}
	case IconSmall:
	}
	return iconMetrics{ds.IconSm, ds.RadiusIconSm, ds.GlyphSmall}
}

// ToolIcon is the tile that stands for a tool: its icon art when its
// manifest names one, otherwise its glyph on a monochrome tile, or on its
// category's tint.
func (k *Kit) ToolIcon(c *ui.Context, m tool.Manifest, o ToolIconOpts) ui.Element {
	p := ds.P(c)
	mt := metricsOf(o.Size)
	tile := ui.Box(c).Size(mt.box, mt.box).Radius(mt.radius).Center().Shrink(0)
	if o.Tint {
		ink, fill := CategoryColors(c, m.Category)
		tile.Background(fill)
		return k.finishTile(c, tile, o, func() { k.Glyph(c, m.Icon, mt.glyph, ink) })
	}
	tile.Gradient(p.IconTile, p.IconTileEnd, 180).Border(ds.LineWidth, p.LineHairline)
	return k.finishTile(c, tile, o, func() { k.Glyph(c, m.Icon, mt.glyph, p.IconInk) })
}

func (k *Kit) finishTile(c *ui.Context, tile ui.Element, o ToolIconOpts, face func()) ui.Element {
	p := ds.P(c)
	return tile.Children(func() {
		face()
		if !o.Running {
			return
		}
		size, ring, off := float32(ds.RunBadge), float32(ds.RunBadgeRing), float32(-3)
		if o.Size == IconSmall {
			size, ring, off = ds.RunBadgeSmall, ds.LineWidth, -2
		}
		ui.Box(c).Absolute().Top(off).Right(off).Size(size, size).Radius(ds.RadiusFull).Background(p.Live).
			Border(ring, p.BgCanvas)
	})
}
