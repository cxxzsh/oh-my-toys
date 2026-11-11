package shell

import (
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

func (s *actionState) resetSelection() {
	s.selected = make([]bool, len(s.plan.Rows))
	for i, r := range s.plan.Rows {
		s.selected[i] = r.Selectable
	}
}

func (s *actionState) ensureSelection() {
	if len(s.selected) != len(s.plan.Rows) {
		s.resetSelection()
	}
}

func (s *actionState) effectivePlan() tool.Plan {
	s.ensureSelection()
	if s.plan.Select != nil {
		return s.plan.Select(s.selected)
	}
	return s.plan
}

func hasSelectable(rows []tool.Row) bool {
	for _, r := range rows {
		if r.Selectable {
			return true
		}
	}
	return false
}

func hasGroups(rows []tool.Row) bool {
	for _, r := range rows {
		if r.Group != "" || r.Selectable {
			return true
		}
	}
	return false
}

func selectionStats(rows []tool.Row, sel []bool) (checked, total int) {
	for i, r := range rows {
		if !r.Selectable {
			continue
		}
		total++
		if i < len(sel) && sel[i] {
			checked++
		}
	}
	return checked, total
}

func (a *App) selectionBar(c *ui.Context, s *actionState) {
	s.ensureSelection()
	checked, total := selectionStats(s.plan.Rows, s.selected)
	if total == 0 {
		return
	}
	all := checked == total
	ui.Row(c).FillWidth().Padding(0, ds.Space4).Gap(ds.Space10).AlignItems(ui.Center).Children(func() {
		if a.kit.Checkbox(c.Key("sel-all"), &all, i18n.SelectAll(), false).Changed() {
			for i, r := range s.plan.Rows {
				if r.Selectable {
					s.selected[i] = all
				}
			}
		}
		comp.Hint(c, i18n.SelectedCount(strconv.Itoa(checked), strconv.Itoa(total)))
	})
}

func (a *App) rowCheckBox(c *ui.Context, s *actionState, origIdx int) {
	s.ensureSelection()
	if origIdx < 0 || origIdx >= len(s.plan.Rows) || !s.plan.Rows[origIdx].Selectable {
		return
	}
	p := ds.P(c)
	on := s.selected[origIdx]
	b := ui.ButtonBase(c.Key("chk")).Label(s.plan.Rows[origIdx].Title).Selected(on).Cursor(ui.CursorDefault)
	if b.Clicked() {
		s.selected[origIdx] = !on
	}
	box, edge := p.FillControl, p.LineHover
	if on {
		box, edge = p.AccentStrong, p.AccentStrong
	}
	b.Size(ds.CheckboxSize, ds.CheckboxSize).Radius(ds.RadiusXs).Background(box).Border(ds.LineWidth, edge).
		Center().Children(func() {
		if on {
			a.kit.Glyph(c, "check", ds.CheckTickSize, p.Thumb)
		}
	})
}

func (a *App) groupedPlainRows(c *ui.Context, t tool.Action, s *actionState, indices []int) {
	s.ensureSelection()
	ui.Column(c).Grow(1).FillWidth().Gap(ds.Space8).Children(func() {
		a.selectionBar(c, s)
		comp.Card(c).Grow(1).FillWidth().Clip().Children(func() {
			ui.List(c.Key("grows"), &s.rows, len(indices), func(pos int) {
				a.groupedRow(c, s, indices[pos])
			}).Grow(1).FillWidth().Padding(ds.Space8).Label(i18n.Preview())
		})
		a.resultFooter(c, t, s)
	})
}

func (a *App) groupedRow(c *ui.Context, s *actionState, idx int) {
	r := s.plan.Rows[idx]
	p := ds.P(c)
	ui.Column(c.Key(idx)).FillWidth().Gap(ds.Space4).Children(func() {
		if r.Group != "" {
			comp.Eyebrow(c, r.Group).Padding(ds.Space8, ds.TableCellPad, ds.Space2)
		}
		ui.Row(c).FillWidth().Height(ds.TableRow).Padding(0, ds.TableCellPad).Gap(ds.Space10).
			Radius(ds.RadiusXs).AlignItems(ui.Center).Children(func() {
			if r.Selectable {
				a.rowCheckBox(c, s, idx)
			} else {
				ui.Box(c).Size(ds.CheckboxSize, ds.CheckboxSize)
			}
			ink := p.TextPrimary
			if r.Selectable && !s.selected[idx] {
				ink = p.TextMuted
			}
			b := ui.ButtonBase(c.Key("copy")).Label(r.Title).Cursor(ui.CursorDefault)
			if b.Clicked() && r.Value != "" {
				mygo.Clipboard.WriteText(r.Value)
				c.Toast(i18n.Copied(r.Title))
			}
			b.Grow(1).MinWidth(0).Justify(ui.Start).AlignItems(ui.Center).Children(func() {
				comp.Value(c, r.Title, ink).Ellipsis("…").Shrink(1).MinWidth(0)
			})
			plainDetail(c, r, toneDetailColor(p, r.Tone))
		})
	})
}

func toneDetailColor(p *ds.Palette, t tool.Tone) ui.Color {
	switch t {
	case tool.ToneSuccess:
		return p.Success
	case tool.ToneWarning:
		return p.Warning
	case tool.ToneDanger:
		return p.Danger
	case tool.ToneNormal:
	}
	return p.TextSecondary
}
