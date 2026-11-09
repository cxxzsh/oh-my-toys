package shell

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// The methods the filter offers, in the order of their chips.
const (
	methodGET = iota
	methodPOST
	methodPUT
	methodDELETE
	methodHEAD
	methodOPTIONS
	methodPATCH
	trafficMethodCount
)

// trafficClassCount is the status chips: none (no response), then 2xx to 5xx.
const trafficClassCount = 5

// The media types the filter offers, in the order of their chips.
const (
	typeJSON = iota
	typeHTML
	typeJS
	typeCSS
	typeImage
	typeOther
	trafficTypeCount
)

// trafficFilterState is the filter that the search field and the Filter
// popover set. It is embedded in trafficState, so its fields belong to the page.
type trafficFilterState struct {
	text     string // the search text: the source of truth for the query tokens
	failed   bool   // "Failed only"
	methods  [trafficMethodCount]bool
	classes  [trafficClassCount]bool
	types    [trafficTypeCount]bool
	search   ui.Handle // the search input
	rowsText string    // the search text the rows were last narrowed with
	menuOpen bool      // the Filter popover
}

func trafficMethod(i int) string {
	return [trafficMethodCount]string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH"}[i]
}

// trafficClass maps a chip index to a status class: index 0 is class 0 (no
// response), index 1 is 2xx, and so on up to 5xx.
func trafficClass(i int) int {
	if i == 0 {
		return 0
	}
	return i + 1
}

func trafficClassLabel(i int) string {
	if i == 0 {
		return "none"
	}
	return strconv.Itoa(trafficClass(i)) + "xx"
}

func trafficTypeLabel(i int) string {
	return [trafficTypeCount]string{"JSON", "HTML", "JS", "CSS", "Image", "Other"}[i]
}

// trafficTypePrefixes are the media type prefixes a type chip matches.
func trafficTypePrefixes(i int) []string {
	switch i {
	case typeJSON:
		return []string{"application/json"}
	case typeHTML:
		return []string{"text/html"}
	case typeJS:
		return []string{"text/javascript", "application/javascript"}
	case typeCSS:
		return []string{"text/css"}
	case typeImage:
		return []string{"image/"}
	}
	return nil
}

// makeFilter is the part of the filter the log can pick: the popover's
// choices and the longest plain word. The query's tokens narrow the rows in
// narrowRows, so the sidebar's pick and the tokens AND together there.
func (st *trafficState) makeFilter() tool.Filter {
	f := tool.Filter{
		Source: tool.FlowSource(st.mode),
		Text:   parseQuery(st.text).logWord(),
		Failed: st.failed,
	}
	for i := range st.methods {
		if st.methods[i] {
			f.Methods = append(f.Methods, trafficMethod(i))
		}
	}
	for i := range st.classes {
		if st.classes[i] {
			f.StatusClasses = append(f.StatusClasses, trafficClass(i))
		}
	}
	if !st.types[typeOther] {
		for i := range st.types {
			if st.types[i] {
				f.Types = append(f.Types, trafficTypePrefixes(i)...)
			}
		}
	}
	return f
}

// narrowFilter drops the log's Types when "Other" is chosen: the log cannot
// say "not one of the known types", so narrowRows does it.
func (st *trafficState) narrowFilter(f tool.Filter) tool.Filter {
	if st.types[typeOther] {
		f.Types = nil
	}
	return f
}

// narrowRows keeps the rows that pass every token of the query and the
// "Other" media type choice. The log has already applied the popover's choices.
func (st *trafficState) narrowRows(rows []tool.FlowRow) []tool.FlowRow {
	q := parseQuery(st.text)
	out := make([]tool.FlowRow, 0, len(rows))
	for _, r := range rows {
		if q.rowMatches(r) && (!st.types[typeOther] || st.typeMatch(r.Type)) && st.wordsMatch(q.words, r.ID) {
			out = append(out, r)
		}
	}
	return out
}

func (st *trafficState) typeMatch(media string) bool {
	for i := range st.types {
		if !st.types[i] || i == typeOther {
			continue
		}
		for _, p := range trafficTypePrefixes(i) {
			if strings.HasPrefix(media, p) {
				return true
			}
		}
	}
	return st.types[typeOther] && media != "" && !knownType(media)
}

// knownType reports whether a chip names a media type.
func knownType(media string) bool {
	for i := range trafficTypeCount {
		for _, p := range trafficTypePrefixes(i) {
			if strings.HasPrefix(media, p) {
				return true
			}
		}
	}
	return false
}

// filterActive reports whether anything narrows the list.
func (st *trafficState) filterActive() bool {
	return strings.TrimSpace(st.text) != "" || st.failed ||
		anyTrue(st.methods[:]) || anyTrue(st.classes[:]) || anyTrue(st.types[:])
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

// clearFilter forgets the search text and every choice.
func (st *trafficState) clearFilter() {
	st.text, st.failed = "", false
	st.methods, st.classes, st.types = [trafficMethodCount]bool{}, [trafficClassCount]bool{}, [trafficTypeCount]bool{}
}

// trafficSearchField is the query field of the traffic page: one pill with a
// magnifier, the text, a clear button and the placeholder. The text is the
// source of truth for the filter, so the toolbar only decides where it sits.
func (a *App) trafficSearchField(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	box := ui.Row(c).Width(ds.SearchWidth).MinWidth(ds.SearchMinWidth).Shrink(1).Height(ds.ControlLg).
		Padding(0, ds.Space6, 0, ds.Space12).Gap(ds.Space6).Radius(ds.RadiusFull).Background(p.FillControl).
		Border(ds.LineWidth, p.LineHairline).AlignItems(ui.Center)
	box.Children(func() {
		a.kit.Glyph(c, "search", ds.SearchGlyph, p.TextMuted)
		in := ui.TextInputBase(c, &st.text).Bind(&st.search).Grow(1).Placeholder(i18n.TrafficSearch()).
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

// trafficFilterBar is the one row under the toolbar: the Filter button with
// its popover, the applied choices as chips, Failed only, and Follow at the end.
func (a *App) trafficFilterBar(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.ControlLg).Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
		if t.Config().Mode == tool.ModeProxy {
			a.trafficFilterButton(c, st)
			a.trafficAppliedChips(c, st)
		}
		ui.Spacer(c)
		if comp.Chip(c.Key("failed"), i18n.TrafficFilterFailed(), st.failed).Clicked() {
			st.failed = !st.failed
		}
		if st.filterActive() && a.kit.Button(c.Key("clear-filters"), i18n.TrafficClearFilters(), comp.ButtonOpts{Kind: comp.Ghost, Small: true}).Clicked() {
			st.clearFilter()
		}
		comp.Text(c, i18n.TrafficFollow(), ds.Caption, p.TextSecondary)
		if comp.SwitchCompact(c.Key("traffic-follow"), &st.follow, i18n.TrafficFollow()).Changed() && st.follow {
			st.list.ScrollToEnd()
		}
	})
}

// trafficFilterButton opens the popover with the method, status and type groups.
func (a *App) trafficFilterButton(c *ui.Context, st *trafficState) {
	btn := a.kit.Button(c.Key("filter-open"), i18n.TrafficFilterButton(), comp.ButtonOpts{Kind: comp.Secondary, Small: true, Glyph: "tune"})
	if btn.Clicked() {
		st.menuOpen = !st.menuOpen
	}
	ui.Popover(c, btn, &st.menuOpen, func() {
		ui.Column(c).Padding(ds.Space12, ds.Space12).Gap(ds.Space10).Children(func() {
			a.trafficMethodChips(c, st)
			a.trafficClassChips(c, st)
			a.trafficTypeChips(c, st)
		})
	})
}

// trafficAppliedChips shows what narrows the list: one chip per query token,
// which removes that token from the text when clicked, and one chip per
// choice in the popover, which clears it.
func (a *App) trafficAppliedChips(c *ui.Context, st *trafficState) {
	for _, tok := range parseQuery(st.text).tokens {
		if comp.Chip(c.Key(tok.start), st.text[tok.start:tok.end]+" ×", true).Clicked() {
			st.text = withoutToken(st.text, tok)
			return
		}
	}
	for i := range st.methods {
		if st.methods[i] && comp.Chip(c.Key("applied-method:"+trafficMethod(i)), trafficMethod(i)+" ×", true).Clicked() {
			st.methods[i] = false
		}
	}
	for i := range st.classes {
		if st.classes[i] && comp.Chip(c.Key("applied-class:"+trafficClassLabel(i)), trafficClassLabel(i)+" ×", true).Clicked() {
			st.classes[i] = false
		}
	}
	for i := range st.types {
		if st.types[i] && comp.Chip(c.Key("applied-type:"+trafficTypeLabel(i)), trafficTypeLabel(i)+" ×", true).Clicked() {
			st.types[i] = false
		}
	}
}

// trafficGroup is one labelled group of checkable chips in the popover.
func trafficGroup(c *ui.Context, title string, chips func()) {
	comp.Eyebrow(c, title)
	ui.Row(c).Gap(ds.Space6).AlignItems(ui.Center).Wrap().Children(chips)
}

func (a *App) trafficMethodChips(c *ui.Context, st *trafficState) {
	trafficGroup(c, i18n.TrafficMethods(), func() {
		for i := range st.methods {
			if comp.Chip(c.Key("method:"+trafficMethod(i)), trafficMethod(i), st.methods[i]).Clicked() {
				st.methods[i] = !st.methods[i]
			}
		}
	})
}

func (a *App) trafficClassChips(c *ui.Context, st *trafficState) {
	trafficGroup(c, i18n.StatusLabel(), func() {
		for i := range st.classes {
			if comp.Chip(c.Key("class:"+trafficClassLabel(i)), trafficClassLabel(i), st.classes[i]).Clicked() {
				st.classes[i] = !st.classes[i]
			}
		}
	})
}

func (a *App) trafficTypeChips(c *ui.Context, st *trafficState) {
	trafficGroup(c, i18n.TrafficTypes(), func() {
		for i := range st.types {
			if comp.Chip(c.Key("type:"+trafficTypeLabel(i)), trafficTypeLabel(i), st.types[i]).Clicked() {
				st.types[i] = !st.types[i]
			}
		}
	})
}
