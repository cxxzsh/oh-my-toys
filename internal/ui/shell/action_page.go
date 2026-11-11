package shell

import (
	"context"
	"errors"
	"log"
	"maps"
	"slices"
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
	input       tool.Input
	plan        tool.Plan
	err         error
	busy        bool
	busyUntil   time.Time
	result      string
	cancel      context.CancelFunc
	version     int
	table       comp.TableState
	rows        ui.ListState
	filter      string
	filterInput ui.Handle
	selected    []bool
	portScope   string
	inputs      map[string]*string
	started     bool
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
			Meta: func() {
				cmd := buildActionCLI(t, s)
				comp.RiskBadge(c, actionRisk(t))
				a.kit.CLICopyPill(c.Key("cli"), cmd, i18n.CopyCommand(), i18n.Copied(cmd))
			},
		})
		a.fields(c, t, s)
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
	colorField, hasColor := hasColorField(fs)
	comp.Card(c).FillWidth().Padding(ds.Space10, ds.Space12).Gap(ds.Space8).Children(func() {
		a.fieldControls(c, t, s, fs, !hasColor || len(fs) > 1)
		if hasColor {
			a.colorSwatchRow(c, t, s, colorField)
		}
	})
}

func (a *App) fieldControls(c *ui.Context, t tool.Action, s *actionState, fs []core.Field, showRefresh bool) {
	ui.Row(c).FillWidth().Gap(ds.Space10).AlignItems(ui.End).Wrap().Children(func() {
		for _, f := range fs {
			a.fieldCell(c, t, s, f)
		}
		if len(s.plan.Rows) > 8 {
			a.kit.SearchField(c.Key("act-filter"), &s.filter, &s.filterInput, comp.SearchFieldOpts{
				Placeholder: i18n.FilterRows(),
				Width:       190,
			})
		}
		if showRefresh && a.kit.Button(c.Key("refresh"), i18n.Refresh(), comp.ButtonOpts{Kind: comp.Secondary}).Clicked() {
			a.replan(context.Background(), t, s)
		}
	})
}

// fieldCell gives a folder field room for its path, and every other field a
// narrower share of the row.
func (a *App) fieldCell(c *ui.Context, t tool.Action, s *actionState, f core.Field) {
	if isFolderField(f) {
		ui.Box(c.Key("box:" + f.Name)).Grow(2).Basis(260).MinWidth(180).Children(func() {
			a.field(c, t, s, f)
		})
		return
	}
	ui.Box(c.Key("box:" + f.Name)).Grow(1).Basis(150).MinWidth(140).Children(func() {
		a.field(c, t, s, f)
	})
}

// isFolderField reports whether a field takes a folder path.
func isFolderField(f core.Field) bool {
	return f.Kind == core.FieldPath
}

func actionRisk(t tool.Action) tool.Risk {
	for _, name := range []string{tool.ActionApply, tool.ActionPlan} {
		if spec, ok := core.SpecOf(t, name); ok {
			return spec.Risk
		}
	}
	return tool.RiskRead
}

// buildActionCLI is the omt command that runs the action with the current
// inputs, as the CLI copy pill shows it.
func buildActionCLI(t tool.Action, s *actionState) string {
	var b strings.Builder
	b.WriteString("omt " + t.Manifest().ID)
	for _, k := range slices.Sorted(maps.Keys(s.input)) {
		if v := s.input[k]; v != "" {
			b.WriteString(" --" + k + " " + quoteArg(v))
		}
	}
	if s.plan.ApplyLabel != "" {
		b.WriteString(" --apply")
	}
	return b.String()
}

// quoteArg quotes a flag value only when it holds a space, as the CLI copy
// pill shows it.
func quoteArg(v string) string {
	if !strings.ContainsAny(v, " \t") {
		return v
	}
	return shellQuote(v)
}

func (a *App) field(c *ui.Context, t tool.Action, s *actionState, f core.Field) {
	v, ok := s.inputs[f.Name]
	if !ok {
		v = new(string)
		s.inputs[f.Name] = v
	}
	if isFolderField(f) {
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
		if a.kit.Button(c.Key("choose"), i18n.ChooseFolder(), comp.ButtonOpts{Kind: comp.Secondary}).Clicked() {
			a.openFolderPicker(t, s, f.Name)
		}
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
			s.plan, s.err = plan, err
			s.resetSelection()
			s.finishBusy()
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
	plan := s.effectivePlan()
	if plan.Destructive {
		a.confirmPlan(t, s, plan)
		return
	}
	a.runApply(t, s, plan)
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

func (s *actionState) startBusy() {
	s.busy = true
	s.busyUntil = time.Time{}
}

// finishBusy keeps a completed blip visible briefly, including plans that
// finish before the UI has time to paint a frame.
func (s *actionState) finishBusy() {
	s.busy = false
	s.busyUntil = time.Now().Add(busyFlash)
}

const busyFlash = 250 * time.Millisecond

// showBusyLine stays visible while work runs and briefly after it finishes.
func (a *App) showBusyLine(c *ui.Context) bool {
	now := time.Now()
	show := false
	var wait time.Duration
	for _, s := range a.actions {
		if s.busy {
			show = true
			continue
		}
		left := s.busyUntil.Sub(now)
		if left > 0 {
			show = true
			if wait == 0 || left < wait {
				wait = left
			}
		}
	}
	if wait > 0 {
		c.After(wait)
	}
	return show
}

func (a *App) planView(c *ui.Context, t tool.Action, s *actionState) {
	indices := filterActionIndices(s.filter, s.plan.Rows)
	rows := rowsAt(s.plan.Rows, indices)
	switch {
	case len(s.plan.Rows) > 0 && s.plan.Rows[0].Process != nil:
		a.portsTable(c, t, s, rows)
	case len(s.plan.Columns) == 3 && len(rows) > 0:
		a.resultList(c, t, s, rows, indices)
	default:
		a.plainRows(c, t, s, rows, indices)
	}
}

func filterActionIndices(query string, rows []tool.Row) []int {
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]int, 0, len(rows))
	for i, r := range rows {
		if q == "" || strings.Contains(strings.ToLower(r.Title+" "+r.Detail+" "+strings.Join(r.Cells, " ")), q) {
			out = append(out, i)
		}
	}
	return out
}

func rowsAt(rows []tool.Row, indices []int) []tool.Row {
	out := make([]tool.Row, len(indices))
	for i, idx := range indices {
		out[i] = rows[idx]
	}
	return out
}

func (a *App) resultList(c *ui.Context, t tool.Action, s *actionState, rows []tool.Row, indices []int) {
	p := ds.P(c)
	checkable := hasSelectable(s.plan.Rows)
	text := resultTableText(rows, checkable)
	cols := resultTableCols(s.plan.Columns, checkable)
	ui.Column(c).Grow(1).FillWidth().Gap(ds.Space8).Children(func() {
		if checkable {
			a.selectionBar(c, s)
		}
		a.kit.DataTable(c, &s.table, comp.Table{
			Cols: cols,
			Text: text,
			Tone: func(row int) comp.Tone {
				if rows[row].Tone == tool.ToneWarning {
					return comp.ToneWarning
				}
				return comp.ToneNeutral
			},
			Cell: func(row, col int) { a.resultTableCell(c, s, rows[row], indices[row], col, checkable, p) },
		})
		a.resultFooter(c, t, s)
	})
}

func resultTableText(rows []tool.Row, checkable bool) [][]string {
	text := make([][]string, len(rows))
	for i, r := range rows {
		if checkable {
			text[i] = []string{"", r.Cells[0], "→", r.Cells[1], r.Cells[2]}
		} else {
			text[i] = []string{r.Cells[0], "→", r.Cells[1], r.Cells[2]}
		}
	}
	return text
}

func resultTableCols(headers []string, checkable bool) []comp.Column {
	cols := []comp.Column{
		{Label: headers[0], Share: 5},
		{Label: "", Share: 1},
		{Label: headers[1], Share: 5},
		{Label: headers[2], Share: 2, End: true},
	}
	if checkable {
		cols = append([]comp.Column{{Label: "", Share: 0.7}}, cols...)
	}
	return cols
}

func (a *App) resultTableCell(c *ui.Context, s *actionState, r tool.Row, origIdx, col int, checkable bool, p *ds.Palette) {
	if checkable {
		if col == 0 {
			a.rowCheckBox(c, s, origIdx)
			return
		}
		resultCell(c, r, col-1, p)
		return
	}
	resultCell(c, r, col, p)
}

func resultCell(c *ui.Context, r tool.Row, col int, p *ds.Palette) {
	switch col {
	case 0:
		if r.Cells[0] != r.Cells[1] {
			diffFrom(c, r, p)
			return
		}
		comp.Value(c, r.Cells[0], p.TextMuted).Ellipsis("…")
	case 1:
		comp.Value(c, "→", p.TextMuted)
	case 2:
		if r.Tone == tool.ToneWarning {
			comp.Value(c, r.Cells[1], p.Warning).Ellipsis("…")
			return
		}
		if r.Cells[0] == r.Cells[1] {
			comp.Value(c, r.Cells[1], p.TextStrong).Ellipsis("…")
			return
		}
		diffTo(c, r, p)
	case 3:
		tone := comp.ToneNeutral
		if r.Tone == tool.ToneWarning {
			tone = comp.ToneWarning
		}
		comp.Badge(c, r.Cells[2], tone)
	}
}

func (a *App) resultFooter(c *ui.Context, t tool.Action, s *actionState) {
	eff := s.effectivePlan()
	if s.plan.ApplyLabel == "" && s.result == "" {
		return
	}
	p := ds.P(c)
	ui.Row(c).FillWidth().Padding(ds.ResultFootPad, ds.TableCellPad).Gap(ds.Space12).AlignItems(ui.Center).
		Radius(ds.RadiusSm).Background(p.BgTitlebar).Border(ds.LineWidth, p.Line).Children(func() {
		cmd := buildActionCLI(t, s)
		comp.RiskBadge(c, actionRisk(t))
		text, ink := eff.Summary, p.TextSecondary
		if s.result != "" {
			text, ink = s.result, p.Success
		}
		comp.Text(c, text, ds.Caption, ink).Grow(1)
		a.kit.CLICopyPill(c.Key("footer-cli"), cmd, i18n.CopyCommand(), i18n.Copied(cmd))
		if eff.ApplyLabel != "" && !s.busy && s.err == nil {
			if a.kit.Button(c.Key("apply"), eff.ApplyLabel, comp.ButtonOpts{Kind: comp.Primary}).Clicked() {
				a.apply(t, s)
			}
		}
	})
}

func (a *App) plainRows(c *ui.Context, t tool.Action, s *actionState, rows []tool.Row, indices []int) {
	if len(rows) == 0 {
		a.actionEmptyCard(c, t, s)
		return
	}
	if s.plan.ApplyLabel == "" {
		if _, ok := parseHexSwatch(s.plan.Summary); ok {
			a.colorStudioView(c, t, s, strings.TrimSpace(s.plan.Summary), rows)
			return
		}
	}
	if hasGroups(s.plan.Rows) {
		a.groupedPlainRows(c, t, s, indices)
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
		if col, ok := swatchOf(r); ok {
			swatchBox(c, col, p)
		}
		comp.Value(c, r.Title, p.TextPrimary).Ellipsis("…").Shrink(1).MinWidth(0)
		ui.Spacer(c)
		plainDetail(c, r, detail)
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
