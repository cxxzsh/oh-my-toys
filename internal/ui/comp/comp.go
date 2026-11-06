// Package comp draws the design system's components (ToolIcon, StatusPill,
// Switch, Button, Kbd, ToolHeader, MetricTile) with MyGo's widget bases,
// which have behavior and accessibility but no look of their own.
package comp

import (
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
	"github.com/cxxzsh/oh-my-toys/internal/ui/icons"
)

// CatColors returns a category's glyph and tile colors.
func CatColors(c *ui.Context, cat tool.Category) (fg, bg ui.Color) {
	p := ds.P(c)
	switch cat {
	case tool.CategorySystem:
		return p.CatOrange, p.CatOrangeBg
	case tool.CategoryFile:
		return p.CatBlue, p.CatBlueBg
	case tool.CategoryDev:
		return p.CatPurple, p.CatPurpleBg
	case tool.CategoryNetwork:
		return p.CatTeal, p.CatTealBg
	case tool.CategoryText:
		return p.CatPink, p.CatPinkBg
	case tool.CategoryMedia:
		return p.CatGreen, p.CatGreenBg
	case tool.CategoryProductivity:
		return p.CatIndigo, p.CatIndigoBg
	default:
		return p.CatSlate, p.CatSlateBg
	}
}

// IconSize names the ToolIcon sizes.
type IconSize int

// The sizes of tool icons.
const (
	Small  IconSize = iota // icon-sm 20, sidebar and tray rows
	Medium                 // icon-md 28, cards, rows, palette
	Large                  // icon-lg 48, tool page header
	Launch                 // the tray's launch grid: 36 (bundle.css)
)

// ToolIcon is a Lucide glyph on a tile of its category's color, with a
// success dot at the top right while the tool runs.
func ToolIcon(c *ui.Context, cat tool.Category, icon string, size IconSize, running bool) ui.Element {
	fg, bg := CatColors(c, cat)
	box, radius, glyph := float32(ds.IconSm), float32(ds.RadiusIconSm), float32(ds.IconSmGlyph)
	switch size {
	case Medium:
		box, radius, glyph = ds.IconMd, ds.RadiusIconMd, ds.IconMdGlyph
	case Large:
		box, radius, glyph = ds.IconLg, ds.RadiusIconLg, ds.IconLgGlyph
	case Launch:
		box, radius, glyph = ds.LaunchIcon, ds.LaunchIconR, ds.LaunchGlyph
	default: // Small, set above
	}
	return ui.Box(c).Size(box, box).Radius(radius).Background(bg).Center().Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(glyph, glyph).TextColor(fg)
		if running {
			p := ds.P(c)
			ui.Box(c).Absolute().Top(-3).Right(-3).Size(ds.RunDotSize, ds.RunDotSize).
				Radius(ds.RadiusPill).Background(p.Success).Border(2, p.BgElevated).Label("运行中")
		}
	})
}

// Glyph is an interface glyph in the color of the text (16px).
func Glyph(c *ui.Context, name string, size float32, color ui.Color) ui.Element {
	return ui.Icon(c, icons.Get(name)).Size(size, size).TextColor(color)
}

// Text is a text in a type style and color.
func Text(c *ui.Context, s string, st ds.TypeStyle, color ui.Color) ui.Element {
	return ds.Style(ui.Text(c, s), st).TextColor(color)
}

// Eyebrow is a small section label.
func Eyebrow(c *ui.Context, s string) ui.Element {
	return Text(c, s, ds.Eyebrow, ds.P(c).TextSecondary).SingleLine()
}

// Dot is a status dot: success while on, tertiary while off.
func Dot(c *ui.Context, on bool) ui.Element {
	p := ds.P(c)
	col := p.TextTertiary
	if on {
		col = p.Success
	}
	return ui.Box(c).Size(ds.DotSize, ds.DotSize).Radius(ds.RadiusPill).Background(col)
}

// Pill is a StatusPill: a dot and a short status.
func Pill(c *ui.Context, text string, on bool) ui.Element {
	p := ds.P(c)
	col := p.TextSecondary
	if on {
		col = p.TextPrimary
	}
	return ui.Row(c).Height(ds.PillHeight).Padding(0, 10, 0, 8).Gap(6).AlignItems(ui.Center).
		Radius(ds.RadiusPill).Background(p.FillControl).Children(func() {
		Dot(c, on)
		Text(c, text, ds.Callout, col).SingleLine().FontFeatures("tnum")
	})
}

// Switch is the design system's switch: 38×22, the knob easing in 150ms.
func Switch(c *ui.Context, on *bool, label string) ui.Element {
	p := ds.P(c)
	track := p.BorderControl
	if *on {
		track = p.AccentFill
	}
	sw := ui.SwitchBase(c, on).Label(label)
	x := float32(2)
	if *on {
		x = ds.SwitchWidth - ds.SwitchKnob - 2
	}
	x = sw.Animate("knob", x, 150*time.Millisecond)
	sw.Size(ds.SwitchWidth, ds.SwitchHeight).Radius(ds.RadiusPill).Background(track).Children(func() {
		ds.ApplyShadow(ui.Box(c).Absolute().Top(2).Left(x).Size(ds.SwitchKnob, ds.SwitchKnob).
			Radius(ds.RadiusPill).Background(ui.RGB(255, 255, 255)), ds.S(c).ShadowKnob)
	})
	return sw
}

// ButtonKind picks a button's look.
type ButtonKind int

// The looks of buttons.
const (
	Secondary ButtonKind = iota
	Primary
	Plain
	Danger
	DangerFill // a destructive primary action
)

// Button is a design-system button.
func Button(c *ui.Context, label string, kind ButtonKind) ui.Element {
	p := ds.P(c)
	bg, fg := p.FillControl, p.TextPrimary
	padX := float32(14)
	switch kind {
	case Primary:
		bg, fg = p.AccentFill, p.OnAccent
	case Plain:
		bg, fg, padX = ui.Transparent, p.Accent, 6
	case Danger:
		bg, fg, padX = ui.Transparent, p.Danger, 6
	case DangerFill:
		bg, fg = p.Danger, p.OnAccent
	default: // Secondary, set above
	}
	b := ui.ButtonBase(c).Label(label)
	if b.Hovered() && kind == Secondary {
		bg = p.FillSelected
	}
	return b.Height(ds.ControlHeight).Padding(0, padX).Radius(ds.RadiusMd).Background(bg).
		AlignItems(ui.Center).Gap(6).Children(func() {
		Text(c, label, ds.Headline, fg).SingleLine()
	})
}

// Kbd is a key cap.
func Kbd(c *ui.Context, keys string) ui.Element {
	p := ds.P(c)
	return ui.Row(c).Height(ds.KbdHeight).Padding(0, 5).Radius(ds.RadiusXs).Background(p.FillControl).
		AlignItems(ui.Center).Children(func() {
		Text(c, keys, ds.TypeStyle{Size: ds.Eyebrow.Size, LineHeight: ds.Eyebrow.Size, Weight: 400}, p.TextSecondary).SingleLine()
	})
}

// Shortcut spells an accelerator the platform's way: ⌥Space on macOS,
// Alt+Space elsewhere (README: 快捷键写法).
func Shortcut(acc string) string {
	if runtime.GOOS != "darwin" {
		return strings.ReplaceAll(acc, "CmdOrCtrl", "Ctrl")
	}
	return strings.NewReplacer("CmdOrCtrl+", "⌘", "Cmd+", "⌘", "Ctrl+", "⌃", "Alt+", "⌥", "Shift+", "⇧").Replace(acc)
}

// ToolHeader is a tool page's header: the large icon, name, summary, and
// what trailing puts on the right (a pill and a switch for switch tools).
func ToolHeader(c *ui.Context, m tool.Manifest, name, summary string, trailing func()) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
		ToolIcon(c, m.Category, m.Icon, Large, false)
		ui.Column(c).Grow(1).Gap(ds.HeaderGap).Children(func() {
			Text(c, name, ds.LargeTitle, p.TextPrimary).Role(ui.RoleHeading).Level(1)
			Text(c, summary, ds.Body, p.TextSecondary)
		})
		if trailing != nil {
			trailing()
		}
	})
}

// Card is a bg-card box with the card border and (light only) shadow.
func Card(c *ui.Context) ui.Element {
	p := ds.P(c)
	return ds.ApplyShadow(ui.Column(c).Radius(ds.RadiusLg).Background(p.BgCard).Border(1, p.BorderCard), ds.S(c).ShadowCard)
}

// MetricTile is a big number with its unit and a label.
func MetricTile(c *ui.Context, label, value, unit string) {
	p := ds.P(c)
	Card(c).Grow(1).Basis(0).Padding(ds.Space4).Gap(ds.Space1).Children(func() {
		Eyebrow(c, label)
		ui.Row(c).Gap(ds.Space1).AlignItems(ui.End).Children(func() {
			Text(c, value, ds.NumericXl, p.TextPrimary).FontFeatures("tnum").SingleLine()
			if unit != "" {
				Text(c, unit, ds.Callout, p.TextSecondary).Margin(0, 0, 6, 0)
			}
		})
	})
}

// Field is the design system's text field: fill-control, control-height,
// radius-md, a focus-ring border while it has the keyboard.
func Field(c *ui.Context, v *string, placeholder string) ui.Element {
	p := ds.P(c)
	in := ui.TextInputBase(c, v).Height(ds.ControlHeight).Padding(0, ds.FieldPadX).Radius(ds.RadiusMd).
		Background(p.FillControl).FontSize(ds.Body.Size).TextColor(p.TextPrimary).
		Placeholder(placeholder).PlaceholderColor(p.TextTertiary).Label(placeholder)
	if in.Focused() {
		in.Border(1, p.FocusRing)
	}
	return in
}
