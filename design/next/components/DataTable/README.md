# DataTable

A dense mono table for system data: ports, processes, files.

**The view provides:** columns (label, width share, alignment), rows, the row's actions.

- Row `table-row` 26pt; header in `eyebrow` over a `line` rule; cells in `meta`; `line-hairline` between rows.
- **States.** Hover: `fill-control`. Selected: `accent-soft`. ↑↓ move, Shift extends, Mod+A selects all, Mod+C copies the selected rows as tab-separated text.
- Columns resize by dragging header edges and sort by clicking the header; widths and sort persist per tool.
- The Process column leads with a 16pt ProcessIcon (the OS icon, or the default user or system icon), `space-6` before the name.
- Row actions at the end: a square danger button, or a lock with Protected / System for rows the tool refuses to touch. Delete / ⌫ triggers the row's destructive action through the Dialog. Every row has a ContextMenu. Numbers are tabular.
