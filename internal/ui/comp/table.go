package comp

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Column is a column of a DataTable.
type Column struct {
	Label string
	Share float32 // width share against the other columns
	End   bool    // right-aligned
}

// TableState is what a DataTable remembers between frames: the scroll
// position, the selected rows and the sort.
type TableState struct {
	list           ui.ListState
	anchor, cursor int // the selected rows run from anchor to cursor
	sortCol        int
	sorted, desc   bool
}

// Table describes a DataTable: a dense mono table for system data.
type Table struct {
	Cols []Column
	Text [][]string         // the text of every cell: sorting, copying
	Cell func(row, col int) // draws the content of a cell
	// Tone is a row's fill: ToneWarning for a conflict, ToneNeutral for none.
	Tone   func(row int) Tone
	Menu   func(m *ui.Menu, row int) // a row's native context menu
	Delete func(row int)             // Delete or Backspace on a row
}

// order returns the rows in display order.
func (s *TableState) order(text [][]string) []int {
	order := make([]int, len(text))
	for i := range order {
		order[i] = i
	}
	if !s.sorted {
		return order
	}
	slices.SortStableFunc(order, func(a, b int) int {
		r := compareCells(text[a][s.sortCol], text[b][s.sortCol])
		if s.desc {
			return -r
		}
		return r
	})
	return order
}

// compareCells orders numbers as numbers and anything else as text.
func compareCells(a, b string) int {
	x, errA := strconv.Atoi(strings.TrimPrefix(a, ":"))
	y, errB := strconv.Atoi(strings.TrimPrefix(b, ":"))
	if errA == nil && errB == nil {
		return cmp.Compare(x, y)
	}
	return strings.Compare(a, b)
}

func (s *TableState) selected(pos int) bool {
	return pos >= min(s.anchor, s.cursor) && pos <= max(s.anchor, s.cursor)
}

// DataTable draws the table: a header over a virtual list of rows. ↑↓ move
// the selection, Shift extends it, Mod+A selects all, Mod+C copies the
// selected rows as tab-separated text, Delete runs the row's destructive
// action. Clicking a header sorts by it.
func (k *Kit) DataTable(c *ui.Context, st *TableState, t Table) {
	p := ds.P(c)
	order := st.order(t.Text)
	ui.Column(c).Grow(1).FillWidth().Radius(ds.RadiusSm).Background(p.BgCard).Border(ds.LineWidth, p.Line).Clip().
		Children(func() {
			k.tableHeader(c, st, t.Cols)
			st.keys(c, t, order)
			ui.List(c.Key("table"), &st.list, len(order), func(pos int) {
				k.tableRow(c, st, t, order, pos)
			}).Grow(1).FillWidth()
		})
}

func (k *Kit) tableHeader(c *ui.Context, st *TableState, cols []Column) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.TableRow).Padding(0, ds.TableCellPad).AlignItems(ui.Center).
		BorderWidth(0, 0, ds.LineWidth, 0).BorderColor(p.Line).Children(func() {
		for i, col := range cols {
			h := ui.ButtonBase(c.Key(i)).Label(col.Label).Cursor(ui.CursorDefault)
			if h.Clicked() {
				st.sortBy(i)
			}
			just := ui.Start
			if col.End {
				just = ui.End
			}
			h.Grow(col.Share).Basis(0).MinWidth(0).Gap(ds.Space4).AlignItems(ui.Center).Justify(just)
			h.Children(func() {
				Text(c, strings.ToUpper(col.Label), ds.Eyebrow, p.TextMuted).SingleLine()
				if st.sorted && st.sortCol == i {
					k.sortGlyph(c, st.desc)
				}
			})
		}
	})
}

func (k *Kit) sortGlyph(c *ui.Context, desc bool) {
	g := k.Glyph(c, "expand_more", ds.GlyphSm, ds.P(c).TextMuted)
	if !desc {
		g.Rotate(180)
	}
}

func (s *TableState) sortBy(col int) {
	switch {
	case !s.sorted || s.sortCol != col:
		s.sorted, s.sortCol, s.desc = true, col, false
	case !s.desc:
		s.desc = true
	default:
		s.sorted = false
	}
}

func (k *Kit) tableRow(c *ui.Context, st *TableState, t Table, order []int, pos int) {
	p := ds.P(c)
	row := order[pos]
	b := ui.ButtonBase(c.Key(row)).Label(strings.Join(t.Text[row], ", ")).Selected(st.selected(pos)).
		Cursor(ui.CursorDefault)
	if b.Clicked() {
		st.cursor = pos
		if b.ClickModifiers()&ui.Shift == 0 {
			st.anchor = pos
		}
	}
	fill := ui.Transparent
	switch {
	case st.selected(pos):
		fill = p.AccentSoft
	case t.Tone != nil && t.Tone(row) == ToneWarning:
		fill = p.WarningSoft
	case b.Hovered():
		fill = p.FillControl
	}
	if t.Menu != nil {
		b.ContextMenu(func(m *ui.Menu) { t.Menu(m, row) })
	}
	b.FillWidth().Height(ds.TableRow).Padding(0, ds.TableCellPad).Background(fill).AlignItems(ui.Center)
	if pos > 0 {
		b.BorderWidth(ds.LineWidth, 0, 0, 0).BorderColor(p.LineHairline)
	}
	b.Children(func() {
		for i, col := range t.Cols {
			cell := ui.Row(c.Key(i)).Grow(col.Share).Basis(0).MinWidth(0).AlignItems(ui.Center)
			if col.End {
				cell.Justify(ui.End)
			}
			cell.Children(func() { t.Cell(row, i) })
		}
	})
}

// keys handles the table's keyboard while it or a row has the focus.
func (s *TableState) keys(c *ui.Context, t Table, order []int) {
	if len(order) == 0 || !c.FocusWithin(s.list.Handle) {
		return
	}
	step := 0
	switch {
	case c.Shortcut(0, ui.KeyDown), c.Shortcut(ui.Shift, ui.KeyDown):
		step = 1
	case c.Shortcut(0, ui.KeyUp), c.Shortcut(ui.Shift, ui.KeyUp):
		step = -1
	}
	if step != 0 {
		s.cursor = max(0, min(s.cursor+step, len(order)-1))
		if c.Modifiers()&ui.Shift == 0 {
			s.anchor = s.cursor
		}
		s.list.ScrollTo(s.cursor, ui.Center)
	}
	if c.Shortcut(ui.Cmd, ui.KeyA) {
		s.anchor, s.cursor = 0, len(order)-1
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) {
		mygo.Clipboard.WriteText(s.copyText(t.Text, order))
	}
	if t.Delete != nil && (c.Shortcut(0, ui.KeyDelete) || c.Shortcut(0, ui.KeyBackspace)) {
		t.Delete(order[min(s.cursor, len(order)-1)])
	}
}

// copyText is the selected rows as tab-separated text, a row per line.
func (s *TableState) copyText(text [][]string, order []int) string {
	var b strings.Builder
	for pos := range order {
		if !s.selected(pos) {
			continue
		}
		b.WriteString(strings.Join(text[order[pos]], "\t"))
		b.WriteByte('\n')
	}
	return b.String()
}
