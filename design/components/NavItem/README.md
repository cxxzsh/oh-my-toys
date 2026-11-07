# NavItem

One sidebar row: glyph, tool name and an optional trailing badge, hint or live dot.

**The view provides:** glyph name, label, selected and running state, an optional count or shortcut hint.

- Height `nav-row` 28pt, pill (`radius-full`), padding `space-10`, label in `label`.
- **States.** Rest: `text-inactive`. Hover: `fill-control`, `text-strong`. Pressed: `fill-control-pressed`. Selected: `bg-selected`, `line-hairline` rim, `shadow-xs`, label 600, glyph `accent`, an `accent` dot when nothing else trails. Selected in an inactive window: glyph and dot `text-muted`.
- **Trailing.** One of: a Badge (`6 active`), a mono hint (`24`), a pulsing `live` dot for a running switch tool.
- Accessible name: label plus state ("Keep Awake, running").
