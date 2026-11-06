package shell

import (
	"context"
	"errors"
	"log"
	"maps"
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

// actionState is an action tool's page: what was typed, the plan worked
// out for it in the background, and what applying it did.
type actionState struct {
	input   tool.Input
	plan    tool.Plan
	err     error
	busy    bool
	result  string
	cancel  context.CancelFunc
	version int
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

// actionPage is the page of an action tool: its fields, the plan of what
// would happen, and the button that applies it.
func (a *App) actionPage(c *ui.Context, t tool.Action) {
	m := t.Manifest()
	s := a.action(m.ID)
	if !s.started {
		s.started = true
		a.replan(context.Background(), t, s)
	}
	ui.Column(c).Grow(1).FillWidth().Padding(ds.Space6, ds.Space8).Gap(ds.Space4).Children(func() {
		a.header(c, m, nil)
		a.fields(c, t, s)
		a.planSummary(c, t, s)
		a.planRows(c, s)
	})
}

// fields draws the form of the tool's plan, generated from the input
// schema it declares.
func (a *App) fields(c *ui.Context, t tool.Action, s *actionState) {
	spec, ok := core.SpecOf(t, tool.ActionPlan)
	if !ok {
		return
	}
	fs := core.Form(spec, msgs())
	if len(fs) == 0 {
		return
	}
	comp.Card(c).FillWidth().Padding(ds.Space4).Gap(ds.Space3).Children(func() {
		for _, f := range fs {
			a.field(c, t, s, f)
		}
	})
}

// field draws one field of the form: a folder chooser for a path, a text
// field for anything else, numbers included, which the tool checks.
func (a *App) field(c *ui.Context, t tool.Action, s *actionState, f core.Field) {
	p := ds.P(c)
	v, ok := s.inputs[f.Name]
	if !ok {
		v = new(string)
		s.inputs[f.Name] = v
	}
	ui.Row(c.Key(f.Name)).FillWidth().Gap(ds.Space3).AlignItems(ui.Center).Children(func() {
		comp.Text(c, f.Label, ds.Headline, p.TextPrimary).Width(ds.FieldLabelWidth)
		if f.Kind == core.FieldPath {
			a.folderField(c, t, s, f, v)
			return
		}
		in := comp.Field(c, v, f.Hint).Grow(1)
		if in.Changed() {
			s.input[f.Name] = *v
			a.replan(context.Background(), t, s)
		}
	})
}

func (a *App) folderField(c *ui.Context, t tool.Action, s *actionState, f core.Field, v *string) {
	p := ds.P(c)
	shown, color := *v, p.TextPrimary
	if shown == "" {
		shown, color = f.Hint, p.TextTertiary
	}
	comp.Text(c, shown, ds.Body, color).Grow(1).SingleLine().Ellipsis("…")
	if !comp.Button(c, i18n.ChooseFolder(), comp.Secondary).Clicked() {
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
}

// replan works the plan out again in the background, for the input as it
// is now; a plan of an older input is dropped when it comes.
func (a *App) replan(ctx context.Context, t tool.Action, s *actionState) {
	if s.cancel != nil {
		s.cancel()
	}
	s.version++
	version, in := s.version, maps.Clone(s.input)
	s.busy, s.result = true, ""
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

func (a *App) planSummary(c *ui.Context, t tool.Action, s *actionState) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Gap(ds.Space3).AlignItems(ui.Center).MinHeight(ds.ControlHeight).Children(func() {
		switch {
		case s.busy:
			comp.Text(c, i18n.Working(), ds.Callout, p.TextSecondary).Grow(1)
		case s.err != nil:
			comp.Text(c, problem(s.err), ds.Callout, p.Danger).Grow(1)
		case s.result != "":
			comp.Text(c, s.result, ds.Callout, p.Success).Grow(1)
		default:
			comp.Text(c, s.plan.Summary, ds.Callout, p.TextSecondary).Grow(1)
		}
		if comp.Button(c, i18n.Refresh(), comp.Plain).Clicked() {
			a.replan(context.Background(), t, s)
		}
		if s.plan.ApplyLabel != "" && !s.busy && s.err == nil {
			kind := comp.Primary
			if s.plan.Destructive {
				kind = comp.DangerFill
			}
			if comp.Button(c, s.plan.ApplyLabel, kind).Clicked() {
				a.apply(t, s)
			}
		}
	})
}

// problem is the message of an error: a tool's own words for a problem
// with the input, or a general one, the details going to the log.
func problem(err error) string {
	var pr *tool.Problem
	if errors.As(err, &pr) {
		return pr.Text
	}
	log.Print(err)
	return i18n.PlanFailed()
}

// apply asks first when the plan changes or removes something, then
// applies it in the background and plans again.
func (a *App) apply(t tool.Action, s *actionState) {
	plan, id := s.plan, t.Manifest().ID
	s.busy = true
	a.opt.Runner.Go(context.Background(), id, planTimeout, func(ctx context.Context) {
		if plan.Destructive && !confirm(a.main, plan) {
			a.Post(func() { s.busy = false })
			return
		}
		res, err := t.Apply(ctx, plan)
		a.Post(func() {
			// The apply's context ends with it; the new plan outlives it.
			a.replan(context.WithoutCancel(ctx), t, s)
			s.result, s.err = res.Summary, err
		})
	})
}

func confirm(parent *mygo.Window, plan tool.Plan) bool {
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:  parent,
		Type:    mygo.MessageWarning,
		Message: plan.ApplyLabel + "?",
		Detail:  plan.Summary,
		Buttons: []string{plan.ApplyLabel, i18n.Cancel()},
	})
	return err == nil && res.Button == 0
}

func (a *App) planRows(c *ui.Context, s *actionState) {
	p := ds.P(c)
	rows := s.plan.Rows
	if len(rows) == 0 {
		return
	}
	comp.Card(c).Grow(1).FillWidth().Clip().Children(func() {
		ui.List(c.Key("rows"), &s.rows, len(rows), func(i int) { a.planRow(c, rows[i]) }).
			Grow(1).FillWidth().Padding(ds.Space2).Label(i18n.Preview())
		comp.Text(c, i18n.ClickToCopy(), ds.Callout, p.TextTertiary).Padding(0, ds.Space4, ds.Space2)
	})
}

func (a *App) planRow(c *ui.Context, r tool.Row) {
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
	row := ui.ButtonBase(c).Label(r.Title)
	if row.Clicked() && r.Value != "" {
		mygo.Clipboard.WriteText(r.Value)
		c.Toast(i18n.Copied(r.Value))
	}
	bg := ui.Transparent
	if row.Hovered() {
		bg = p.FillHover
	}
	row.FillWidth().Padding(ds.Space2, ds.Space3).Gap(ds.Space3).Radius(ds.RadiusSm).Background(bg).AlignItems(ui.Center).Children(func() {
		comp.Text(c, r.Title, ds.Body, p.TextPrimary).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		ui.Spacer(c)
		comp.Text(c, r.Detail, ds.Callout, detail).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
	})
}
