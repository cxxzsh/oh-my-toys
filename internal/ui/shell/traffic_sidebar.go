package shell

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

const (
	// trafficHostLimit caps the host list: a busy log can name thousands of
	// hosts, and the busiest ones are what the sidebar is for.
	trafficHostLimit = 200
)

// refreshFacets reads the programs and hosts of the log again, but only when
// the log has changed. Counting the whole log on every frame would be wasted
// work on a page that re-renders often.
func (st *trafficState) refreshFacets() {
	if st.log == nil {
		return
	}
	ver := st.log.Version()
	if st.facetSet && ver == st.facetVer {
		return
	}
	st.facetVer, st.facetSet = ver, true
	st.procs, st.hosts = st.log.Processes(), st.log.Hosts()
}

// withFacets narrows a filter to the program and host picked in the sidebar.
func (st *trafficState) withFacets(f tool.Filter) tool.Filter {
	if st.procSel != "" {
		f.Processes = []string{st.procSel}
	}
	if st.hostSel != "" {
		f.Hosts = []string{st.hostSel}
	}
	return f
}

// trafficSidebar is the sources column beside the flow list: the scope chip
// when one is set, then the programs and hosts of the log. A click narrows the
// list to one of them; clicking it again widens it back.
func (a *App) trafficSidebar(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	ui.Column(c).Width(ds.TrafficSourcesWidth).FillHeight().Padding(ds.Space8).Gap(ds.Space4).
		Background(p.FillControl).Radius(ds.RadiusMd).Clip().Children(func() {
		a.trafficScope(c, t, st)
		comp.Eyebrow(c, i18n.TrafficPrograms())
		if a.trafficAllRow(c, i18n.TrafficAllPrograms()) {
			st.procSel = ""
		}
		for _, f := range st.procs {
			a.trafficProgramRow(c, t, st, f)
		}
		comp.Eyebrow(c, i18n.TrafficHosts())
		if a.trafficAllRow(c, i18n.TrafficAllHosts()) {
			st.hostSel = ""
		}
		a.trafficHostRows(c, st)
	})
}

// trafficAllRow is the entry that clears a selection. It is drawn plain: the
// unfiltered state is the default and needs no highlight, so only a chosen
// program or host is tinted. It reports a click.
func (a *App) trafficAllRow(c *ui.Context, label string) bool {
	return trafficRowButton(c, "traffic-facet:"+label, label, "", false).Clicked()
}

// trafficProgramRow is one program. Its menu holds the actions that make
// sense for a program: showing only it, or recording only it.
func (a *App) trafficProgramRow(c *ui.Context, t tool.Traffic, st *trafficState, f tool.Facet) {
	active := st.procSel == f.Name
	row := trafficRowButton(c, "traffic-prog:"+f.Name, f.Name, strconv.Itoa(f.Count), active)
	if row.Clicked() {
		st.procSel = trafficToggle(st.procSel, f.Name)
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(i18n.TrafficShowOnly()).Chosen() {
			st.procSel = f.Name
		}
		if m.Item(i18n.TrafficCaptureOnly()).Chosen() {
			a.trafficSetScope(t, st, []string{f.Name})
		}
	})
}

// trafficHostRows lists the busiest hosts. Past the limit a hint says how
// many were left out.
func (a *App) trafficHostRows(c *ui.Context, st *trafficState) {
	hosts := st.hosts
	if len(hosts) > trafficHostLimit {
		hosts = hosts[:trafficHostLimit]
	}
	for _, h := range hosts {
		row := trafficRowButton(c, "traffic-host:"+h.Name, h.Name, strconv.Itoa(h.Count), st.hostSel == h.Name)
		if row.Clicked() {
			st.hostSel = trafficToggle(st.hostSel, h.Name)
		}
	}
	if len(st.hosts) > len(hosts) {
		comp.Hint(c, i18n.TrafficHostsMore(strconv.Itoa(len(hosts)), strconv.Itoa(len(st.hosts))))
	}
}

// trafficRowButton draws one sidebar row: the name on the left, the count on
// the right, and a tint while it is the selection.
func trafficRowButton(c *ui.Context, key, name, count string, active bool) ui.Element {
	p := ds.P(c)
	bg := ui.Color{}
	if active {
		bg = p.AccentSoft
	}
	b := ui.ButtonBase(c.Key(key)).FillWidth().Height(ds.ControlMd).Padding(ds.Space6).
		Radius(ds.RadiusSm).Background(bg).Cursor(ui.CursorDefault).Selected(active)
	return b.Children(func() {
		ui.Row(c).FillWidth().Height(ds.ControlMd).AlignItems(ui.Center).Gap(ds.Space6).Children(func() {
			comp.Text(c, name, ds.Caption, p.TextPrimary).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if count != "" {
				comp.Value(c, count, p.TextMuted)
			}
		})
	})
}

// trafficToggle returns the name to select: the same name clears the selection.
func trafficToggle(cur, name string) string {
	if cur == name {
		return ""
	}
	return name
}
