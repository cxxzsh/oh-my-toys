package shell

import (
	"context"
	"errors"
	"log"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// planTimeout bounds a plan, such as hashing a large folder.
const planTimeout = 10 * time.Minute

type actionState struct {
	input   tool.Input
	plan    tool.Plan
	err     error
	busy    bool
	busyAt  time.Time // when the current busy run began
	result  string
	cancel  context.CancelFunc
	version int
	table   comp.TableState
	rows    ui.ListState
	inputs  map[string]*string
	started bool
}

func (a *App) action(id string) *actionState {
	s, ok := a.actions[id]
	if !ok {
		s = &actionState{input: tool.Input{}, inputs: map[string]*string{}}
		a.actions[id] = s
	}
	return s
}

func (a *App) actionPage(c *ui.Context, t tool.Action) {
	m := t.Manifest()
	s := a.action(m.ID)
	if !s.started {
		s.started = true
		a.replan(context.Background(), t, s)
	}
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		a.replan(context.Background(), t, s)
	}
	ui.Column(c).Grow(1).FillWidth().Padding(ds.Space16).Gap(ds.Space16).Children(func() {
		comp.PageHeader(c, comp.Header{
			Title:       m.Name(msgs()),
			Badge:       s.plan.Summary,
			Description: m.Summary(msgs()),
		})
		a.fields(c, t, s)
		a.busyBar(c, s)
		a.planView(c, t, s)
		a.actionStatus(c, m, s)
	})
}

func (a *App) fields(c *ui.Context, t tool.Action, s *actionState) {
	spec, ok := core.SpecOf(t, tool.ActionPlan)
	if !ok {
		return
	}
	fs := core.Form(spec, msgs())
	if len(fs) == 0 {
		return
	}
	a.kit.Panel(c, comp.PanelOpts{
		Title: t.Manifest().Name(msgs()),
		Glyph: t.Manifest().Icon,
		Actions: func() {
			if a.kit.Button(c.Key("refresh"), i18n.Refresh(), comp.ButtonOpts{Kind: comp.Ghost, Small: true}).Clicked() {
				a.replan(context.Background(), t, s)
			}
		},
	}, func() {
		for _, f := range fs {
			a.field(c, t, s, f)
		}
	})
}

func (a *App) field(c *ui.Context, t tool.Action, s *actionState, f core.Field) {
	v, ok := s.inputs[f.Name]
	if !ok {
		v = new(string)
		s.inputs[f.Name] = v
	}
	if f.Kind == core.FieldPath {
		a.folderField(c, t, s, f, v)
		return
	}
	errText := ""
	if s.err != nil {
		errText = problem(s.err)
	}
	in := comp.TextField(c.Key(f.Name), v, comp.FieldOpts{
		Label:       f.Label,
		Placeholder: f.Hint,
		Error:       errText,
		Mono:        true,
	})
	if in.Changed() {
		s.input[f.Name] = *v
		a.replan(context.Background(), t, s)
	}
}

func (a *App) folderField(c *ui.Context, t tool.Action, s *actionState, f core.Field, v *string) {
	p := ds.P(c)
	ui.Row(c.Key(f.Name)).FillWidth().Gap(ds.Space8).AlignItems(ui.End).Children(func() {
		ui.Box(c).Grow(1).Children(func() {
			in := comp.TextField(c, v, comp.FieldOpts{Label: f.Label, Placeholder: f.Hint, Mono: true})
			if in.Changed() {
				s.input[f.Name] = *v
				a.replan(context.Background(), t, s)
			}
		})
		if !a.kit.Button(c.Key("choose"), i18n.ChooseFolder(), comp.ButtonOpts{Kind: comp.Secondary}).Clicked() {
			return
		}
		go func() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Directory: true, Parent: a.main})
			if err != nil || len(paths) == 0 {
				return
			}
			a.Post(func() {
				*v = paths[0]
				s.input[f.Name] = paths[0]
				a.replan(context.Background(), t, s)
			})
		}()
	})
	_ = p
}

func (a *App) replan(ctx context.Context, t tool.Action, s *actionState) {
	if s.cancel != nil {
		s.cancel()
	}
	s.version++
	version, in := s.version, maps.Clone(s.input)
	s.startBusy()
	s.result = ""
	id := t.Manifest().ID
	s.cancel = a.opt.Runner.Go(ctx, id, planTimeout, func(ctx context.Context) {
		plan, err := t.Plan(ctx, in)
		if errors.Is(err, context.Canceled) {
			return
		}
		a.Post(func() {
			if s.version != version {
				return
			}
			s.plan, s.err, s.busy = plan, err, false
		})
	})
}

func problem(err error) string {
	var pr *tool.Problem
	if errors.As(err, &pr) {
		return pr.Text
	}
	log.Print(err)
	return i18n.PlanFailed()
}

func (a *App) apply(t tool.Action, s *actionState) {
	if s.plan.Destructive {
		a.confirmPlan(t, s, s.plan)
		return
	}
	a.runApply(t, s, s.plan)
}

func (a *App) confirmPlan(t tool.Action, s *actionState, plan tool.Plan) {
	items := make([]comp.DialogItem, 0, len(plan.Rows))
	for _, r := range plan.Rows {
		items = append(items, comp.DialogItem{Process: r.Process, Text: r.Title + "  " + r.Detail})
	}
	more := ""
	if extra := len(items) - ds.DialogMaxItems; extra > 0 {
		more = i18n.AndMore(extra)
	}
	a.askDialog(comp.DialogOpts{
		Title:       plan.ApplyLabel + "?",
		Description: plan.Summary,
		Items:       items,
		MoreText:    more,
		Cancel:      i18n.Cancel(),
		Confirm:     plan.ApplyLabel,
		Destructive: true,
		OnConfirm:   func() { a.runApply(t, s, plan) },
	})
}

func (a *App) runApply(t tool.Action, s *actionState, plan tool.Plan) {
	id := t.Manifest().ID
	s.startBusy()
	a.opt.Runner.Go(context.Background(), id, planTimeout, func(ctx context.Context) {
		res, err := t.Apply(ctx, plan)
		a.Post(func() {
			a.replan(context.WithoutCancel(ctx), t, s)
			s.result, s.err = res.Summary, err
		})
	})
}

// busyDelay is how long work must run before the page admits it is busy:
// a plan that finishes sooner, as a quick refresh does, shows no bar.
const busyDelay = 400 * time.Millisecond

func (s *actionState) startBusy() {
	if !s.busy {
		s.busyAt = time.Now()
	}
	s.busy = true
}

// busyBar shows the indeterminate bar once the work has run past
// busyDelay, and asks for a later frame while it is still waiting.
func (a *App) busyBar(c *ui.Context, s *actionState) {
	if !s.busy {
		return
	}
	if wait := busyDelay - time.Since(s.busyAt); wait > 0 {
		c.After(wait)
		return
	}
	comp.ProgressBusy(c)
}

func (a *App) planView(c *ui.Context, t tool.Action, s *actionState) {
	rows := a.filterRows(s.plan.Rows)
	switch {
	case len(rows) > 0 && rows[0].Process != nil:
		a.portsTable(c, t, s, rows)
	case len(s.plan.Columns) == 3 && len(rows) > 0:
		a.resultList(c, t, s, rows)
	default:
		a.plainRows(c, t, s, rows)
	}
}

func (a *App) filterRows(rows []tool.Row) []tool.Row {
	q := strings.ToLower(strings.TrimSpace(a.pageFilter))
	if q == "" {
		return rows
	}
	var out []tool.Row
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Title+" "+r.Detail), q) {
			out = append(out, r)
		}
	}
	return out
}

func (a *App) portsTable(c *ui.Context, t tool.Action, s *actionState, rows []tool.Row) {
	p := ds.P(c)
	text := make([][]string, len(rows))
	for i, r := range rows {
		text[i] = r.Cells
	}
	cols := []comp.Column{
		{Label: i18n.ColPort(), Share: 2},
		{Label: i18n.ColProcess(), Share: 3},
		{Label: i18n.ColPID(), Share: 2},
		{Label: i18n.ColAddress(), Share: 3},
		{Label: i18n.ColAction(), Share: 2, End: true},
	}
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
	case 2, 3:
		comp.Value(c, r.Cells[col], p.TextMuted).Ellipsis("…")
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

func (a *App) resultList(c *ui.Context, t tool.Action, s *actionState, rows []tool.Row) {
	p := ds.P(c)
	text := make([][]string, len(rows))
	for i, r := range rows {
		text[i] = []string{r.Cells[0], "→", r.Cells[1], r.Cells[2]}
	}
	cols := []comp.Column{
		{Label: s.plan.Columns[0], Share: 5},
		{Label: "", Share: 1},
		{Label: s.plan.Columns[1], Share: 5},
		{Label: s.plan.Columns[2], Share: 2, End: true},
	}
	ui.Column(c).Grow(1).FillWidth().Gap(ds.Space8).Children(func() {
		a.kit.DataTable(c, &s.table, comp.Table{
			Cols: cols,
			Text: text,
			Tone: func(row int) comp.Tone {
				if rows[row].Tone == tool.ToneWarning {
					return comp.ToneWarning
				}
				return comp.ToneNeutral
			},
			Cell: func(row, col int) { resultCell(c, rows[row], col, p) },
		})
		a.resultFooter(c, t, s)
	})
}

func resultCell(c *ui.Context, r tool.Row, col int, p *ds.Palette) {
	switch col {
	case 0:
		comp.Value(c, r.Cells[0], p.TextMuted).Ellipsis("…")
	case 1:
		comp.Value(c, "→", p.TextMuted)
	case 2:
		ink := p.TextStrong
		if r.Tone == tool.ToneWarning {
			ink = p.Warning
		}
		comp.Value(c, r.Cells[1], ink).Ellipsis("…")
	case 3:
		tone := comp.ToneNeutral
		if r.Tone == tool.ToneWarning {
			tone = comp.ToneWarning
		}
		comp.Badge(c, r.Cells[2], tone)
	}
}

func (a *App) resultFooter(c *ui.Context, t tool.Action, s *actionState) {
	if s.plan.ApplyLabel == "" && s.result == "" {
		return
	}
	p := ds.P(c)
	ui.Row(c).FillWidth().Padding(ds.ResultFootPad, ds.TableCellPad).Gap(ds.Space12).AlignItems(ui.Center).
		Radius(ds.RadiusSm).Background(p.BgTitlebar).Border(ds.LineWidth, p.Line).Children(func() {
		text, ink := s.plan.Summary, p.TextSecondary
		if s.result != "" {
			text, ink = s.result, p.Success
		}
		comp.Text(c, text, ds.Caption, ink).Grow(1)
		if s.plan.ApplyLabel != "" && !s.busy && s.err == nil {
			if a.kit.Button(c.Key("apply"), s.plan.ApplyLabel, comp.ButtonOpts{Kind: comp.Primary}).Clicked() {
				a.apply(t, s)
			}
		}
	})
}

func (a *App) plainRows(c *ui.Context, t tool.Action, s *actionState, rows []tool.Row) {
	if len(rows) == 0 {
		return
	}
	comp.Card(c).Grow(1).FillWidth().Clip().Children(func() {
		ui.List(c.Key("rows"), &s.rows, len(rows), func(i int) { a.plainRow(c, rows[i]) }).
			Grow(1).FillWidth().Padding(ds.Space8).Label(i18n.Preview())
		a.resultFooter(c, t, s)
	})
}

func (a *App) plainRow(c *ui.Context, r tool.Row) {
	p := ds.P(c)
	detail := p.TextSecondary
	switch r.Tone {
	case tool.ToneSuccess:
		detail = p.Success
	case tool.ToneWarning:
		detail = p.Warning
	case tool.ToneDanger:
		detail = p.Danger
	case tool.ToneNormal:
	}
	b := ui.ButtonBase(c).Label(r.Title).Cursor(ui.CursorDefault)
	if b.Clicked() && r.Value != "" {
		mygo.Clipboard.WriteText(r.Value)
		c.Toast(i18n.Copied(r.Value))
	}
	b.ContextMenu(func(m *ui.Menu) {
		val := r.Value
		if val == "" {
			val = r.Detail
		}
		if m.Item(i18n.CopyValue()).Chosen() {
			mygo.Clipboard.WriteText(val)
		}
	})
	fill := ui.Transparent
	if b.Hovered() {
		fill = p.FillControl
	}
	b.FillWidth().Height(ds.TableRow).Padding(0, ds.TableCellPad).Gap(ds.Space12).Radius(ds.RadiusXs).
		Background(fill).AlignItems(ui.Center).Children(func() {
		comp.Value(c, r.Title, p.TextPrimary).Ellipsis("…").Shrink(1).MinWidth(0)
		ui.Spacer(c)
		comp.Value(c, r.Detail, detail).Ellipsis("…").Shrink(1).MinWidth(0)
	})
}

func (a *App) actionStatus(c *ui.Context, m tool.Manifest, s *actionState) {
	if len(s.plan.Rows) == 0 || s.plan.Rows[0].Process == nil {
		return
	}
	counts := []comp.StatusCount{{Text: s.plan.Summary}}
	for _, sw := range a.reg.Running() {
		counts = append(counts, comp.StatusCount{Text: sw.Manifest().Name(msgs()), Live: true})
	}
	a.kit.StatusBar(c, comp.StatusBarOpts{
		Title:  i18n.LiveTelemetry(),
		Counts: counts,
		Hints:  []string{modKey("K") + " " + i18n.SearchTools(), modKey("R") + " " + i18n.Refresh()},
	})
	_ = m
}
