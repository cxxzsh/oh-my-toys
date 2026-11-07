# Chip

A filter pill above a table or grid; exactly one is active, and the first is always All.

**The view provides:** labels with counts, the active filter.

- Height `control-md`. Rest: `bg-card`, `line-control`, `text-inactive`, `shadow-xs`. Active: `inverse` / `on-inverse`, 600. Hover: `text-strong`.
- Counts come from the data ("Dev (3)") and update live. One tab stop; ←→ change the filter.
