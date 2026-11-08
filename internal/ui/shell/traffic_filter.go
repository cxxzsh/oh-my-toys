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

// The HTTP methods the filter offers, in the order of their chips. They
// are protocol tokens and read the same in every language.
const (
	methodGET = iota
	methodPOST
	methodPUT
	methodDELETE
	methodPATCH
	trafficMethodCount
)

// trafficClassCount is how many status chips the filter offers: no
// response, 2xx, 3xx, 4xx and 5xx.
const trafficClassCount = 5

// The media types the filter offers, in the order of their chips, and
// "other" for everything no other chip names.
const (
	typeJSON = iota
	typeHTML
	typeJS
	typeCSS
	typeImage
	typeOther
	trafficTypeCount
)

// trafficMethod is the name of a method chip.
func trafficMethod(i int) string {
	switch i {
	case methodGET:
		return "GET"
	case methodPOST:
		return "POST"
	case methodPUT:
		return "PUT"
	case methodDELETE:
		return "DELETE"
	case methodPATCH:
		return "PATCH"
	}
	return ""
}

// trafficClass is the status class a chip picks: 0 for no response, then
// 2xx to 5xx.
func trafficClass(i int) int {
	if i == 0 {
		return 0
	}
	return i + 1
}

// trafficClassLabel is the name of a status chip: "2xx", or the word for
// a flow that has no response yet.
func trafficClassLabel(i int) string {
	if i == 0 {
		return i18n.TrafficStatusNone()
	}
	return strconv.Itoa(trafficClass(i)) + "xx"
}

// trafficTypeLabel is the name of a type chip.
func trafficTypeLabel(i int) string {
	switch i {
	case typeJSON:
		return i18n.TrafficTypeJSON()
	case typeHTML:
		return i18n.TrafficTypeHTML()
	case typeJS:
		return i18n.TrafficTypeJS()
	case typeCSS:
		return i18n.TrafficTypeCSS()
	case typeImage:
		return i18n.TrafficTypeImage()
	case typeOther:
		return i18n.TrafficTypeOther()
	}
	return ""
}

// trafficTypePrefixes are the media type prefixes a type chip picks.
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

// makeFilter is the filter the chips and the search field ask for.
func (st *trafficState) makeFilter() tool.Filter {
	f := tool.Filter{
		Source: tool.FlowSource(st.mode),
		Text:   strings.TrimSpace(st.text),
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
	for i := range st.types {
		if st.types[i] && i != typeOther {
			f.Types = append(f.Types, trafficTypePrefixes(i)...)
		}
	}
	return f
}

// narrowFilter drops the type prefix list when "other" is chosen: the log
// can only pick by prefix, so the rows are narrowed here instead.
func (st *trafficState) narrowFilter(f tool.Filter) tool.Filter {
	if st.types[typeOther] {
		f.Types = nil
	}
	return f
}

// narrowRows keeps the rows "other" asks for: a media type no chip names.
func (st *trafficState) narrowRows(rows []tool.FlowRow) []tool.FlowRow {
	if !st.types[typeOther] {
		return rows
	}
	out := make([]tool.FlowRow, 0, len(rows))
	for _, r := range rows {
		if st.typeMatch(r.Type) {
			out = append(out, r)
		}
	}
	return out
}

// typeMatch reports whether a media type is one the chips chosen pick.
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

// clearFilter forgets the search text and every chip.
func (st *trafficState) clearFilter() {
	st.text, st.failed = "", false
	st.methods, st.classes, st.types = [trafficMethodCount]bool{}, [trafficClassCount]bool{}, [trafficTypeCount]bool{}
}

// trafficFilterBar is the search field, the chips and the Follow switch.
func (a *App) trafficFilterBar(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	ui.Column(c).FillWidth().Gap(ds.Space6).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
			a.kit.SearchField(c, &st.text, &st.search, comp.SearchFieldOpts{
				Placeholder: i18n.TrafficSearch(), Width: ds.SearchWidth,
			})
			ui.Spacer(c)
			comp.Text(c, i18n.TrafficFollow(), ds.Caption, p.TextSecondary)
			if comp.SwitchCompact(c.Key("traffic-follow"), &st.follow, i18n.TrafficFollow()).Changed() && st.follow {
				st.list.ScrollToEnd()
			}
		})
		a.trafficChips(c, t, st)
	})
}

// trafficChips are the filters, each group with its accessibly named label.
func (a *App) trafficChips(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Row(c).FillWidth().Gap(ds.Space6).AlignItems(ui.Center).Wrap().Children(func() {
		if t.Config().Mode == tool.ModeProxy {
			a.trafficMethodChips(c, st)
			a.trafficClassChips(c, st)
			a.trafficTypeChips(c, st)
		}
		if comp.Chip(c.Key("failed"), i18n.TrafficFilterFailed(), st.failed).Clicked() {
			st.failed = !st.failed
		}
		if !st.filterActive() {
			return
		}
		ui.Spacer(c)
		if a.kit.Button(c.Key("clear-filters"), i18n.TrafficClearFilters(), comp.ButtonOpts{Kind: comp.Ghost, Small: true}).Clicked() {
			st.clearFilter()
		}
	})
}

func (a *App) trafficMethodChips(c *ui.Context, st *trafficState) {
	comp.Eyebrow(c, i18n.TrafficMethods())
	for i := range st.methods {
		if comp.Chip(c.Key("method:"+trafficMethod(i)), trafficMethod(i), st.methods[i]).Clicked() {
			st.methods[i] = !st.methods[i]
		}
	}
}

func (a *App) trafficClassChips(c *ui.Context, st *trafficState) {
	comp.Eyebrow(c, i18n.StatusLabel())
	for i := range st.classes {
		if comp.Chip(c.Key("class:"+trafficClassLabel(i)), trafficClassLabel(i), st.classes[i]).Clicked() {
			st.classes[i] = !st.classes[i]
		}
	}
}

func (a *App) trafficTypeChips(c *ui.Context, st *trafficState) {
	comp.Eyebrow(c, i18n.TrafficTypes())
	for i := range st.types {
		if comp.Chip(c.Key("type:"+trafficTypeLabel(i)), trafficTypeLabel(i), st.types[i]).Clicked() {
			st.types[i] = !st.types[i]
		}
	}
}
