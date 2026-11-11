package comp

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// FieldOpts describe a text field besides its value.
type FieldOpts struct {
	Label       string // above the field, in label
	Placeholder string
	Help        string // below, in caption
	Error       string // below, in danger: the fix ("Use a number of 0 or more")
	Mono        bool   // patterns and paths
	Invalid     bool   // marks the border as an error, with no text
}

// TextField is a single-line input for a value a tool needs. It returns
// the input: ask it Changed. The field is not a pill, because its text can
// be long.
func TextField(c *ui.Context, v *string, o FieldOpts) ui.Element {
	p := ds.P(c)
	var in ui.Element
	ui.Column(c).FillWidth().Gap(ds.Space4).Children(func() {
		if o.Label != "" {
			Text(c, o.Label, ds.Label, p.TextPrimary)
		}
		in = fieldInput(c, v, o)
		switch {
		case o.Error != "":
			Text(c, o.Error, ds.Caption, p.Danger)
		case o.Help != "":
			Text(c, o.Help, ds.Caption, p.TextInactive)
		}
	})
	return in
}

func fieldInput(c *ui.Context, v *string, o FieldOpts) ui.Element {
	p := ds.P(c)
	style := ds.Body
	if o.Mono {
		style = ds.TypeStyle{Size: ds.Label.Size, LineHeight: ds.Label.LineHeight, Weight: ds.Regular, Mono: true}
	}
	in := ui.TextInputBase(c, v).Placeholder(o.Placeholder).PlaceholderColor(p.TextMuted).
		TextColor(p.TextPrimary).Label(firstOf(o.Label, o.Placeholder))
	ds.Style(in, style)
	in.FillWidth().Height(ds.ControlLg).Padding(0, ds.Space10).Radius(ds.RadiusSm).Background(p.FillControl)
	switch {
	case o.Error != "" || o.Invalid:
		in.Border(ds.LineWidth, p.Danger)
	case in.Focused():
		in.Background(p.BgCard).Border(ds.LineWidth, p.AccentStrong).Shadow(0, 0, 0, ds.FieldHaloWidth, p.AccentSoft)
	default:
		in.Border(ds.LineWidth, p.LineControl)
	}
	return in
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// SearchFieldOpts describe a search field.
type SearchFieldOpts struct {
	Placeholder string // what it filters
	Hint        string // the palette shortcut, at the end; "" for none
	Width       float32
}

// SearchField is the pill field that narrows a page as you type: a 14pt
// glyph, the text, the palette shortcut at the end. Esc clears it. It
// reports whether the shortcut hint was clicked.
func (k *Kit) SearchField(c *ui.Context, q *string, h *ui.Handle, o SearchFieldOpts) (hintClicked bool) {
	p := ds.P(c)
	box := ui.Row(c).Height(ds.ControlLg).Padding(0, ds.Space6, 0, ds.Space10).Gap(ds.Space6).
		Radius(ds.RadiusSm).Background(p.FillControl).Border(ds.LineWidth, p.LineControl).AlignItems(ui.Center)
	if o.Width > 0 {
		box.Width(o.Width).MinWidth(ds.SearchMinWidth).Shrink(1)
	} else {
		box.FillWidth()
	}
	box.Children(func() {
		k.Glyph(c, "search", ds.SearchGlyph, p.TextMuted)
		in := ui.TextInputBase(c, q).Bind(h).Grow(1).Placeholder(o.Placeholder).PlaceholderColor(p.TextMuted).
			TextColor(p.TextPrimary).Label(o.Placeholder)
		ds.Style(in, ds.Label)
		if o.Hint == "" {
			return
		}
		hint := ui.ButtonBase(c).Label(o.Hint).Cursor(ui.CursorDefault)
		hintClicked = hint.Clicked()
		hint.Children(func() { Kbd(c, o.Hint) })
	})
	if c.Focused(*h) {
		box.Border(ds.LineWidth, p.AccentStrong)
	}
	if *q != "" && c.Focused(*h) && c.Shortcut(0, ui.KeyEscape) {
		*q = ""
	}
	return hintClicked
}

// Select is a pop-up button for one choice out of a list: it shows the
// current value and opens a native menu with the current item checked.
// choose is called with the index of the item picked.
func (k *Kit) Select(c *ui.Context, options []string, selected int, choose func(int)) ui.Element {
	p := ds.P(c)
	b := ui.ButtonBase(c).Label(options[selected]).Cursor(ui.CursorDefault)
	fill := p.BgButton
	if b.Hovered() {
		fill = p.FillControlHover
	}
	b.Menu(func(m *ui.Menu) {
		for i, name := range options {
			if m.Item(name).Checked(i == selected).Chosen() {
				choose(i)
			}
		}
	})
	b.Height(ds.ControlMd).MinWidth(ds.SelectMinWidth).Padding(0, ds.Space6, 0, ds.Space10).Gap(ds.Space6).
		Radius(ds.RadiusSm).Background(fill).Border(ds.LineWidth, p.LineControl).AlignItems(ui.Center)
	ds.ApplyShadow(b, ds.S(c).ShadowXs)
	return b.Children(func() {
		sel := ds.Label
		sel.Weight = ds.Regular
		Text(c, options[selected], sel, p.TextStrong).SingleLine().Grow(1)
		k.Glyph(c, "unfold_more", ds.SelectGlyph, p.TextSecondary)
	})
}
