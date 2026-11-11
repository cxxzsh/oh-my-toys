package shell

import (
	"context"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

const (
	scopeAll     = ""
	scopeDev     = "dev"
	scopeExposed = "exposed"
	scopeSystem  = "system"
)

func matchPortScope(r tool.Row, scope string) bool {
	switch scope {
	case scopeDev:
		port, _ := strconv.Atoi(r.Value)
		return port >= 3000 && port <= 9999
	case scopeExposed:
		return len(r.Cells) > 3 && isExposedBind(r.Cells[3])
	case scopeSystem:
		return r.Process != nil && r.Process.System
	default:
		return true
	}
}

func filterByPortScope(rows []tool.Row, scope string) []tool.Row {
	if scope == scopeAll {
		return rows
	}
	var out []tool.Row
	for _, r := range rows {
		if matchPortScope(r, scope) {
			out = append(out, r)
		}
	}
	return out
}

func (a *App) portScopeBar(c *ui.Context, s *actionState, allRows []tool.Row) {
	count := func(sc string) int { return len(filterByPortScope(allRows, sc)) }
	chip := func(sc, label string) {
		txt := label + " (" + strconv.Itoa(count(sc)) + ")"
		if comp.Chip(c.Key("pscope:"+sc), txt, s.portScope == sc).Clicked() {
			s.portScope = sc
		}
	}
	ui.Row(c).FillWidth().Gap(ds.Space6).Wrap().Children(func() {
		chip(scopeAll, i18n.All())
		chip(scopeDev, i18n.ScopeDevPorts())
		chip(scopeExposed, i18n.ScopeExposed())
		chip(scopeSystem, i18n.Protected())
	})
}

func (a *App) portsTable(c *ui.Context, t tool.Action, s *actionState, allRows []tool.Row) {
	rows := filterByPortScope(allRows, s.portScope)
	p := ds.P(c)
	text := make([][]string, len(rows))
	for i, r := range rows {
		text[i] = r.Cells
	}
	cols := []comp.Column{
		{Label: i18n.ColPort(), Share: 2},
		{Label: i18n.ColProcess(), Share: 3},
		{Label: i18n.ColPID(), Share: 1.5},
		{Label: i18n.ColAddress(), Share: 4},
		{Label: i18n.ColAction(), Share: 2, End: true},
	}
	table := func() {
		ui.Column(c).Grow(1).FillHeight().FillWidth().Gap(ds.Space8).Children(func() {
			a.portScopeBar(c, s, allRows)
			a.kit.DataTable(c, &s.table, comp.Table{
				Cols: cols,
				Text: text,
				Cell: func(row, col int) { a.portsCell(c, t, s, rows[row], col, p) },
				Menu: func(m *ui.Menu, row int) { a.portsRowMenu(m, t, s, rows[row]) },
				Delete: func(row int) {
					if rows[row].Process != nil && !rows[row].Process.System {
						a.killPortRow(t, s, rows[row])
					}
				},
			})
		})
	}
	if a.workbenchWidth(c) < ds.PortsWide {
		table()
		return
	}
	ui.Row(c).Grow(1).FillWidth().Gap(ds.Space12).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(table)
		a.portsInspector(c, t, s, rows)
	})
}

func (a *App) portsCell(c *ui.Context, t tool.Action, s *actionState, r tool.Row, col int, p *ds.Palette) {
	switch col {
	case 0:
		ui.Row(c).Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
			comp.Text(c, r.Cells[0], ds.Badge, p.AccentText).FontFeatures(ds.Tabular)
			comp.StatusDot(c, comp.DotOpts{Tone: comp.DotLive})
		})
	case 1:
		ui.Row(c).Gap(ds.ProcessGap).MinWidth(0).AlignItems(ui.Center).Children(func() {
			if r.Process != nil {
				a.kit.ProcessIcon(c, a.procs, *r.Process)
			}
			comp.Value(c, r.Cells[1], p.TextStrong).Ellipsis("…").Shrink(1).MinWidth(0)
		})
	case 2:
		comp.Value(c, r.Cells[2], p.TextMuted).Ellipsis("…")
	case 3:
		ui.Row(c).Gap(ds.Space6).AlignItems(ui.Center).MinWidth(0).Children(func() {
			comp.Value(c, r.Cells[3], p.TextMuted).Ellipsis("…").Shrink(1).MinWidth(0)
			bindBadge(c, r.Cells[3])
		})
	case 4:
		if r.Process != nil && r.Process.System {
			ui.Row(c).Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
				a.kit.Glyph(c, "lock", ds.GlyphSm, p.TextMuted)
				comp.Value(c, i18n.Protected(), p.TextMuted)
			})
			return
		}
		if a.kit.Button(c.Key("kill:"+r.Value+":"+r.Cells[2]), i18n.Kill(), comp.ButtonOpts{
			Kind: comp.Danger, Small: true, Square: true,
		}).Clicked() {
			a.killPortRow(t, s, r)
		}
	}
}

// bindBadge marks a listener that other machines can reach as exposed.
func bindBadge(c *ui.Context, addr string) {
	if isExposedBind(addr) {
		comp.Badge(c, i18n.BindExposed(), comp.ToneWarning).Shrink(0)
		return
	}
	comp.Badge(c, i18n.BindLocal(), comp.ToneSuccess).Shrink(0)
}

// isExposedBind reports whether an address listens on every interface.
func isExposedBind(addr string) bool {
	a := strings.TrimSpace(addr)
	if strings.HasPrefix(a, "::1") {
		return false
	}
	return strings.HasPrefix(a, "0.0.0.0") || strings.HasPrefix(a, "*") || strings.HasPrefix(a, "[::]") || strings.HasPrefix(a, "::")
}

// portsInspector shows the process behind the selected row, with its
// command to run again and its end action.
func (a *App) portsInspector(c *ui.Context, t tool.Action, s *actionState, rows []tool.Row) {
	text := make([][]string, len(rows))
	for i, r := range rows {
		text[i] = r.Cells
	}
	idx := s.table.ActiveRow(text)
	if idx < 0 || idx >= len(rows) {
		return
	}
	r := rows[idx]
	p := ds.P(c)
	comp.Card(c).Width(ds.InspectorWidth).FillHeight().Padding(ds.Space12).Gap(ds.Space10).Children(func() {
		comp.Text(c, i18n.InspectorTitle(), ds.Caption, p.TextMuted)
		ui.Row(c).Gap(ds.ProcessGap).AlignItems(ui.Center).MinWidth(0).Children(func() {
			if r.Process != nil {
				a.kit.ProcessIcon(c, a.procs, *r.Process)
			}
			comp.Value(c, r.Cells[1], p.TextStrong).Ellipsis("…").Shrink(1).MinWidth(0)
			comp.Badge(c, r.Cells[0], comp.ToneAccent)
		})
		inspectorLine(c, i18n.ColPID(), r.Cells[2], p.TextPrimary)
		ui.Row(c).Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
			comp.Text(c, i18n.ColAddress(), ds.Caption, p.TextMuted)
			comp.Value(c, r.Cells[3], p.TextPrimary).Ellipsis("…").Shrink(1).MinWidth(0)
		})
		bindBadge(c, r.Cells[3])
		a.kit.CLICopyPill(c.Key("inspector-cli"), "omt "+t.Manifest().ID+" --port "+r.Value, i18n.CopyCommand(), i18n.Copied("omt "+t.Manifest().ID+" --port "+r.Value))
		inspectorEnd(c, a, t, s, r)
	})
}

// inspectorLine draws one mono key-value line of the inspector.
func inspectorLine(c *ui.Context, key, value string, ink ui.Color) {
	ui.Row(c).Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
		comp.Text(c, key, ds.Caption, ds.P(c).TextMuted)
		comp.Value(c, value, ink).FontFeatures(ds.Tabular)
	})
}

// inspectorEnd draws the locked row for a protected process, or the end button.
func inspectorEnd(c *ui.Context, a *App, t tool.Action, s *actionState, r tool.Row) {
	p := ds.P(c)
	if r.Process != nil && r.Process.System {
		ui.Row(c).Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
			a.kit.Glyph(c, "lock", ds.GlyphSm, p.TextMuted)
			comp.Value(c, i18n.Protected(), p.TextMuted)
		})
		return
	}
	if a.kit.Button(c.Key("inspector-end"), i18n.EndSelectedProcess(), comp.ButtonOpts{Kind: comp.Danger}).Clicked() {
		a.killPortRow(t, s, r)
	}
}

func (a *App) killPortRow(t tool.Action, s *actionState, r tool.Row) {
	pid, _ := strconv.Atoi(r.Cells[2])
	a.askDialog(comp.DialogOpts{
		Title:       i18n.KillProcessTitle(r.Cells[1], pid),
		Description: i18n.KillConsequence(),
		Items:       []comp.DialogItem{{Process: r.Process, Text: r.Cells[1] + "  " + r.Cells[0] + "  pid " + r.Cells[2]}},
		Cancel:      i18n.Cancel(),
		Confirm:     i18n.PortsEnd(1),
		Destructive: true,
		OnConfirm: func() {
			a.opt.Runner.Go(context.Background(), t.Manifest().ID, planTimeout, func(ctx context.Context) {
				plan, err := t.Plan(ctx, tool.Input{"port": r.Value})
				if err == nil && plan.Destructive {
					_, _ = t.Apply(ctx, plan)
				}
				a.Post(func() { a.replan(context.WithoutCancel(ctx), t, s) })
			})
		},
	})
}

func (a *App) portsRowMenu(m *ui.Menu, t tool.Action, s *actionState, r tool.Row) {
	if m.Item(i18n.CopyPort()).Chosen() {
		mygo.Clipboard.WriteText(r.Value)
	}
	if m.Item(i18n.CopyPID()).Chosen() {
		mygo.Clipboard.WriteText(r.Cells[2])
	}
	if m.Item(i18n.CopyAddress()).Chosen() {
		mygo.Clipboard.WriteText(r.Cells[3])
	}
	m.Separator()
	if m.Item(i18n.KillProcess()).Disabled(r.Process != nil && r.Process.System).Chosen() {
		a.killPortRow(t, s, r)
	}
}
