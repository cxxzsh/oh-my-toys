package shell

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// The media types the type chips pick, in the order of the chips.
const (
	typeJSON = iota
	typeForm
	typeXML
	typeJS
	typeCSS
	typeDocument
	typeMedia
	typeOther
	trafficTypeCount
)

// The status chips are 2xx to 5xx, in order, so the chip at index i is the
// hundreds trafficStatusFirst+i.
const (
	trafficStatusCount = 4
	trafficStatusFirst = 2
)

// trafficStatusLabels are the labels of the status chips.
var trafficStatusLabels = [trafficStatusCount]string{"2xx", "3xx", "4xx", "5xx"}

// trafficFilterState is the filter that the chip strip and the search field
// set. It is embedded in trafficState, so its fields belong to the page.
type trafficFilterState struct {
	text      string // the search text: the source of truth for the query tokens
	failed    bool   // the Failed chip, which joins the status group
	classes   [trafficStatusCount]bool
	types     [trafficTypeCount]bool
	search    ui.Handle    // the search input
	rowsText  string       // the search text the rows were last narrowed with
	rowsChips trafficChips // the chips the rows were last narrowed with
}

// trafficChips are the chip choices. Being a value, two of them compare, so
// the page can tell when the rows must be narrowed again.
type trafficChips struct {
	types   [trafficTypeCount]bool
	classes [trafficStatusCount]bool
	failed  bool
}

// chips returns the current chip choices.
func (st *trafficState) chips() trafficChips {
	return trafficChips{types: st.types, classes: st.classes, failed: st.failed}
}

// trafficClass is the hundreds of the status chip at index i: 2 for 2xx.
func trafficClass(i int) int {
	return trafficStatusFirst + i
}

// trafficTypeLabel is the label of the type chip at index i.
func trafficTypeLabel(i int) string {
	switch i {
	case typeJSON:
		return i18n.TrafficTypeJSON()
	case typeForm:
		return i18n.TrafficTypeForm()
	case typeXML:
		return i18n.TrafficTypeXML()
	case typeJS:
		return i18n.TrafficTypeJS()
	case typeCSS:
		return i18n.TrafficTypeCSS()
	case typeDocument:
		return i18n.TrafficTypeDocument()
	case typeMedia:
		return i18n.TrafficTypeMedia()
	}
	return i18n.TrafficTypeOther()
}

// trafficTypePrefixes are the media type prefixes a type chip matches. The
// "Other" chip has none: it matches what the others do not.
func trafficTypePrefixes(i int) []string {
	switch i {
	case typeJSON:
		return []string{"application/json"}
	case typeForm:
		return []string{"application/x-www-form-urlencoded", "multipart/form-data"}
	case typeXML:
		return []string{"application/xml", "text/xml"}
	case typeJS:
		return []string{"text/javascript", "application/javascript"}
	case typeCSS:
		return []string{"text/css"}
	case typeDocument:
		return []string{"text/html", "application/pdf"}
	case typeMedia:
		return []string{"image/", "video/", "audio/"}
	}
	return nil
}

// makeFilter is the part of the filter the log can pick: the longest plain
// word of the search text. The chips and the query's tokens narrow the rows
// in narrowRows, so the sidebar's pick and them all AND together there.
func (st *trafficState) makeFilter() tool.Filter {
	return tool.Filter{
		Source: tool.FlowSource(st.mode),
		Text:   parseQuery(st.text).logWord(),
	}
}

// narrowRows keeps the rows that pass every token of the query and the chips.
func (st *trafficState) narrowRows(rows []tool.FlowRow) []tool.FlowRow {
	q := parseQuery(st.text)
	out := make([]tool.FlowRow, 0, len(rows))
	for _, r := range rows {
		if q.rowMatches(r) && st.chipsMatch(r) && st.wordsMatch(q.words, r.ID) {
			out = append(out, r)
		}
	}
	return out
}

// chipsMatch reports whether a row passes the chips. Chips of one group OR
// together, and the type group and the status group AND together.
func (st *trafficState) chipsMatch(r tool.FlowRow) bool {
	if anyTrue(st.types[:]) && !st.typeMatch(r.Type) {
		return false
	}
	return !st.statusChipsOn() || st.statusMatch(r)
}

// statusMatch reports whether a row is one of the status chips picked, or
// failed when the Failed chip is on.
func (st *trafficState) statusMatch(r tool.FlowRow) bool {
	if st.failed && (r.State == tool.FlowFailed || r.State == tool.FlowBlocked) {
		return true
	}
	for i, on := range st.classes {
		if on && r.Status/100 == trafficClass(i) {
			return true
		}
	}
	return false
}

// typeMatch reports whether a media type is one of the type chips picked.
func (st *trafficState) typeMatch(media string) bool {
	for i, on := range st.types {
		if on && trafficTypeMatches(i, media) {
			return true
		}
	}
	return false
}

// trafficTypeMatches reports whether a media type belongs to the chip at i.
// "Other" takes any media type that no other chip names.
func trafficTypeMatches(i int, media string) bool {
	if i == typeOther {
		return media != "" && !knownType(media)
	}
	for _, p := range trafficTypePrefixes(i) {
		if strings.HasPrefix(media, p) {
			return true
		}
	}
	return false
}

// knownType reports whether a type chip other than "Other" names a media type.
func knownType(media string) bool {
	for i := range typeOther {
		for _, p := range trafficTypePrefixes(i) {
			if strings.HasPrefix(media, p) {
				return true
			}
		}
	}
	return false
}

// statusChipsOn reports whether a status chip or Failed is picked.
func (st *trafficState) statusChipsOn() bool {
	return st.failed || anyTrue(st.classes[:])
}

// chipsOn reports whether any chip is picked.
func (st *trafficState) chipsOn() bool {
	return st.statusChipsOn() || anyTrue(st.types[:])
}

// filterActive reports whether anything narrows the list.
func (st *trafficState) filterActive() bool {
	return strings.TrimSpace(st.text) != "" || st.chipsOn()
}

// anyTrue reports whether any of flags is set.
func anyTrue(flags []bool) bool {
	for _, on := range flags {
		if on {
			return true
		}
	}
	return false
}

// clearChips forgets every chip, and keeps the search text.
func (st *trafficState) clearChips() {
	st.failed = false
	st.classes = [trafficStatusCount]bool{}
	st.types = [trafficTypeCount]bool{}
}

// clearFilter forgets the search text and every chip.
func (st *trafficState) clearFilter() {
	st.text = ""
	st.clearChips()
}

// trafficSearchField is the query field of the traffic page: one pill with a
// magnifier, the text, a clear button and the placeholder. The text is the
// source of truth for the filter, so the bar only decides where it sits.
func (a *App) trafficSearchField(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	box := ui.Row(c).MinWidth(trafficSearchMinWidth).MaxWidth(trafficSearchMaxWidth).Grow(1).Height(ds.ControlLg).
		Padding(0, ds.Space6, 0, ds.Space10).Gap(ds.Space6).Radius(ds.RadiusSm).Background(p.FillControl).
		Border(ds.LineWidth, p.LineControl).AlignItems(ui.Center)
	box.Children(func() {
		a.kit.Glyph(c, "search", ds.SearchGlyph, p.TextMuted)
		in := ui.TextInputBase(c, &st.text).Bind(&st.search).Grow(1).Disabled(st.view == viewRules).Placeholder(i18n.TrafficSearch()).
			PlaceholderColor(p.TextMuted).TextColor(p.TextPrimary).Label(i18n.TrafficSearch())
		ds.Style(in, ds.Label)
		if st.text != "" && ui.ButtonBase(c.Key("search-clear")).Label("×").Cursor(ui.CursorDefault).Clicked() {
			st.text = ""
		}
	})
	if st.text != "" && c.Focused(st.search) && c.Shortcut(0, ui.KeyEscape) {
		st.text = ""
	}
	if c.Focused(st.search) {
		box.Border(ds.LineWidth, p.AccentStrong)
	}
}

// trafficFilterBar is the chip row under the toolbar: All, the type group, the
// status group with Failed, then one chip per query token, which removes that
// token. The chips wrap onto a new line when they do not fit, so none is cut off.
func (a *App) trafficFilterBar(c *ui.Context, st *trafficState) {
	ui.Row(c).FillWidth().Wrap().GapX(ds.Space6).GapY(ds.Space6).AlignItems(ui.Center).Children(func() {
		if comp.Chip(c.Key("all"), i18n.All(), !st.chipsOn()).Clicked() {
			st.clearChips()
		}
		for i := range st.types {
			if comp.Chip(c.Key("type:"+trafficTypeLabel(i)), trafficTypeLabel(i), st.types[i]).Clicked() {
				st.types[i] = !st.types[i]
			}
		}
		ui.Row(c).Width(ds.LineWidth).Height(ds.Glyph).Background(ds.P(c).LineHairline)
		for i := range st.classes {
			if comp.Chip(c.Key("class:"+trafficStatusLabels[i]), trafficStatusLabels[i], st.classes[i]).Clicked() {
				st.classes[i] = !st.classes[i]
			}
		}
		if comp.Chip(c.Key("failed"), i18n.TrafficFilterFailed(), st.failed).Clicked() {
			st.failed = !st.failed
		}
		a.trafficAppliedChips(c, st)
	})
}

// trafficAppliedChips shows one chip per query token. Clicking a chip removes
// that token from the search text.
func (a *App) trafficAppliedChips(c *ui.Context, st *trafficState) {
	for _, tok := range parseQuery(st.text).tokens {
		if comp.Chip(c.Key(tok.start), st.text[tok.start:tok.end]+" ×", true).Clicked() {
			st.text = withoutToken(st.text, tok)
			return
		}
	}
}
