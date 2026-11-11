package shell

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

type directCmd struct {
	timed tool.Timed
	arg   string
	flag  string
	dur   time.Duration
}

func (d directCmd) cli() string {
	return "omt " + d.timed.Manifest().ID + " " + tool.ActionStart + " --" + d.flag + " " + d.arg
}

func parseDirectCommand(reg *core.Registry, query string, now time.Time) (directCmd, bool) {
	head, rest, ok := splitLastWord(strings.TrimSpace(query))
	if !ok {
		return directCmd{}, false
	}
	timed, found := matchTimedTool(reg, head)
	if !found {
		return directCmd{}, false
	}
	prop := tool.PropFor
	if strings.Contains(rest, ":") {
		prop = tool.PropUntil
	}
	dur, err := tool.ParseHold(tool.Input{prop: rest}, now)
	if err != nil || dur <= 0 {
		return directCmd{}, false
	}
	return directCmd{timed: timed, arg: rest, flag: prop, dur: dur}, true
}

func splitLastWord(s string) (head, tail string, ok bool) {
	idx := strings.LastIndexAny(s, " \t")
	if idx <= 0 {
		return "", "", false
	}
	head, tail = strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
	return head, tail, head != "" && tail != ""
}

func matchTimedTool(reg *core.Registry, name string) (tool.Timed, bool) {
	q := strings.ToLower(name)
	for _, t := range reg.All() {
		timed, ok := t.(tool.Timed)
		if !ok || t.Manifest().Soon {
			continue
		}
		m := t.Manifest()
		if strings.EqualFold(m.ID, q) || strings.EqualFold(m.Name(msgs()), q) {
			return timed, true
		}
	}
	return nil, false
}

func (a *App) drawDirectCommand(c *ui.Context, d directCmd) {
	p := ds.P(c)
	m := d.timed.Manifest()
	label := m.Name(msgs()) + " · " + d.arg
	comp.Eyebrow(c, i18n.DirectCommand()).Padding(ds.Space4, ds.Space10)
	row := ui.ButtonBase(c.Key("pal-direct")).Label(label).Cursor(ui.CursorDefault)
	if row.Clicked() {
		a.pal.open = false
		a.startFor(d.timed, d.dur)
	}
	row.FillWidth().Height(ds.PaletteRow).Padding(0, ds.Space10).Gap(ds.Space10).
		Radius(ds.RadiusSm).Background(p.AccentSoft).Border(ds.LineWidth, p.AccentLine).
		AlignItems(ui.Center).Children(func() {
		a.kit.ToolIcon(c, m, comp.ToolIconOpts{Size: comp.IconSmall, Tint: true})
		comp.Text(c, label, ds.Body, p.TextStrong).SingleLine()
		comp.RiskBadge(c, tool.RiskSystem)
		ui.Spacer(c)
		comp.Value(c, d.cli(), p.AccentText).Ellipsis("…")
	})
}

func (a *App) tryDirectKey(c *ui.Context) bool {
	if len(a.pal.results) > 0 || !c.Shortcut(0, ui.KeyEnter) {
		return false
	}
	d, ok := parseDirectCommand(a.reg, a.pal.query, time.Now())
	if !ok {
		return false
	}
	a.pal.open = false
	a.startFor(d.timed, d.dur)
	return true
}
