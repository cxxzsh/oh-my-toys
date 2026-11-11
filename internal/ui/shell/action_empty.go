package shell

import (
	"context"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

var presetSwatches = []string{
	"#2563eb", "#0f7647", "#d97706", "#e11d48",
	"#7c3aed", "#0891b2", "#0f172a", "#64748b",
}

func firstPathField(t tool.Action) (core.Field, bool) {
	spec, ok := core.SpecOf(t, tool.ActionPlan)
	if !ok {
		return core.Field{}, false
	}
	for _, f := range core.Form(spec, msgs()) {
		if f.Kind == core.FieldPath {
			return f, true
		}
	}
	return core.Field{}, false
}

func hasColorField(fs []core.Field) (string, bool) {
	for _, f := range fs {
		if f.Name == "color" {
			return f.Name, true
		}
	}
	return "", false
}

func (a *App) setFieldAndReplan(t tool.Action, s *actionState, name, val string) {
	v, ok := s.inputs[name]
	if !ok {
		v = new(string)
		s.inputs[name] = v
	}
	*v = val
	s.input[name] = val
	a.replan(context.Background(), t, s)
}

func (a *App) colorSwatchRow(c *ui.Context, t tool.Action, s *actionState, fieldName string) {
	p := ds.P(c)
	cur := s.input[fieldName]
	ui.Row(c).FillWidth().Gap(ds.Space6).AlignItems(ui.Center).Wrap().Children(func() {
		comp.Text(c, i18n.PresetSwatches(), ds.Small, p.TextMuted)
		for _, hex := range presetSwatches {
			a.swatchChip(c, t, s, fieldName, hex, cur == hex)
		}
	})
}

func (a *App) swatchChip(c *ui.Context, t tool.Action, s *actionState, fieldName, hex string, active bool) {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key("sw:" + hex)).Label(hex).Selected(active).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.setFieldAndReplan(t, s, fieldName, hex)
	}
	edge := p.LineControl
	if active {
		edge = p.AccentStrong
	} else if b.Hovered() {
		edge = p.LineHover
	}
	b.Height(ds.ControlSm).Padding(0, ds.Space8, 0, ds.Space6).Gap(ds.Space6).
		Radius(ds.RadiusXs).Background(p.BgButton).Border(ds.LineWidth, edge).AlignItems(ui.Center).
		Children(func() {
			ui.Box(c).Size(12, 12).Radius(2).Background(ui.Hex(hex)).Border(ds.LineWidth, p.LineControl)
			comp.Value(c, hex, p.TextSecondary)
		})
}

func (a *App) actionEmptyCard(c *ui.Context, t tool.Action, s *actionState) {
	p := ds.P(c)
	m := t.Manifest()
	spec, _ := core.SpecOf(t, tool.ActionPlan)
	fs := core.Form(spec, msgs())
	pathField, hasPath := firstPathField(t)
	colorField, hasColor := hasColorField(fs)

	title, titleInk := emptyTitle(m, s, p)
	hint := emptyHint(m, s, pathField, hasPath, colorField, hasColor)
	comp.Card(c).Grow(1).FillWidth().Justify(ui.Center).AlignItems(ui.Center).Padding(ds.Space28).Gap(ds.Space12).
		Children(func() {
			a.kit.ToolIcon(c, m, comp.ToolIconOpts{Size: comp.IconLarge, Tint: true})
			comp.Text(c, title, ds.Headline, titleInk).TextAlign(ui.Center)
			comp.Text(c, hint, ds.Caption, p.TextInactive).TextAlign(ui.Center)
			if hasPath {
				if a.kit.Button(c.Key("empty-choose"), i18n.ChooseFolder(), comp.ButtonOpts{Kind: comp.Primary, Glyph: "upload"}).Clicked() {
					a.openFolderPicker(t, s, pathField.Name)
				}
			}
			if hasColor {
				ui.Row(c).Gap(ds.Space6).Justify(ui.Center).Wrap().Children(func() {
					for _, hex := range presetSwatches {
						a.swatchChip(c, t, s, colorField, hex, s.input[colorField] == hex)
					}
				})
			}
		})
}

func emptyTitle(m tool.Manifest, s *actionState, p *ds.Palette) (string, ui.Color) {
	if s.result != "" {
		return s.result, p.Success
	}
	if s.plan.Summary != "" {
		return s.plan.Summary, p.TextStrong
	}
	return m.Name(msgs()), p.TextStrong
}

func emptyHint(m tool.Manifest, s *actionState, pf core.Field, hasPath bool, cf string, hasColor bool) string {
	if hasPath && s.input[pf.Name] == "" {
		return i18n.EmptyFolderHint()
	}
	if hasColor && s.input[cf] == "" {
		return i18n.EmptyColorHint()
	}
	return m.Summary(msgs())
}

func (a *App) openFolderPicker(t tool.Action, s *actionState, fieldName string) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Directory: true, Parent: a.main})
		if err != nil || len(paths) == 0 {
			return
		}
		a.Post(func() { a.setFieldAndReplan(t, s, fieldName, paths[0]) })
	}()
}

func (a *App) acceptDroppedPath(raw string) {
	dir := droppedFolder(raw)
	if dir == "" {
		return
	}
	act, field, ok := a.targetPathAction()
	if !ok {
		return
	}
	id := act.Manifest().ID
	a.Post(func() {
		a.open(id)
		s := a.action(id)
		s.started = true
		a.setFieldAndReplan(act, s, field.Name, dir)
	})
}

func droppedFolder(raw string) string {
	clean := filepath.Clean(raw)
	info, err := os.Stat(clean)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return clean
	}
	return filepath.Dir(clean)
}

func (a *App) targetPathAction() (tool.Action, core.Field, bool) {
	if cur := a.reg.Get(a.page); cur != nil {
		if act, ok := cur.(tool.Action); ok {
			if f, has := firstPathField(act); has {
				return act, f, true
			}
		}
	}
	for _, t := range a.jumpTools() {
		act, ok := t.(tool.Action)
		if !ok {
			continue
		}
		if f, has := firstPathField(act); has {
			return act, f, true
		}
	}
	return nil, core.Field{}, false
}
