package comp

import (
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// CodeViewOpts describe what a CodeView says besides its text.
type CodeViewOpts struct {
	Label string // accessible name of the text
	Note  string // a caption above the text: a truncated or binary body
	// Capped is shown when the text is longer than the view lays out.
	Capped string
	Copy   string // a copy button with this label; empty for none
}

// CodeView is a monospace block a person reads and copies: no wrap,
// scrolled both ways, selectable. It lays out at most ds.CodeMaxBytes of
// the text, with Capped shown when there is more, while the copy button
// copies all of it, so a large body never holds up a frame.
func (k *Kit) CodeView(c *ui.Context, text string, o CodeViewOpts) ui.Element {
	p := ds.P(c)
	shown, capped := cutText(text, ds.CodeMaxBytes)
	box := ui.Column(c).Grow(1).FillWidth().Gap(ds.Space4)
	box.Children(func() {
		k.codeHead(c, text, o, capped)
		code := ds.Small
		code.Mono = true
		ui.ScrollBoth(c).Grow(1).FillWidth().Padding(ds.Space8).Radius(ds.RadiusXs).
			Background(p.FillControl).Border(ds.LineWidth, p.LineHairline).Label(o.Label).
			Children(func() {
				ds.Style(ui.Text(c, shown).NoWrap().Selectable(), code).TextColor(p.TextPrimary)
			})
	})
	return box
}

// codeHead is the line above the text: what it is, what was left out, and
// the way to copy all of it.
func (k *Kit) codeHead(c *ui.Context, text string, o CodeViewOpts, capped bool) {
	p := ds.P(c)
	if o.Note == "" && o.Copy == "" && !capped {
		return
	}
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		if o.Note != "" {
			Text(c, o.Note, ds.Caption, p.TextInactive)
		}
		ui.Spacer(c)
		if capped && o.Capped != "" {
			Text(c, o.Capped, ds.Caption, p.TextInactive).SingleLine()
		}
		if o.Copy == "" {
			return
		}
		if k.Button(c.Key("copy"), o.Copy, ButtonOpts{Kind: Ghost, Small: true, Glyph: "content_copy"}).Clicked() {
			mygo.Clipboard.WriteText(text)
		}
	})
}

// cutText returns the text up to n bytes, on a character boundary, and
// whether it was cut.
func cutText(text string, n int) (string, bool) {
	if len(text) <= n {
		return text, false
	}
	for n > 0 && !utf8.RuneStart(text[n]) {
		n--
	}
	return text[:n], true
}
