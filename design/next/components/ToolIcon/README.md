# ToolIcon

The tile that stands for a tool: its icon asset, or a glyph on a monochrome or category-tinted tile.

**The view provides:** the tool's icon asset (light and dark) or glyph, its category, running state.

- 20pt / 5pt corners (palette, tray), 28pt / 7pt (cards), 48pt / 11pt (tool page header).
- The monochrome tile (`icon-tile` → `icon-tile-end`, one `icon-ink`) is how the shipped tool icons are drawn; tint by category only where tools of several categories sit together (palette, tray).
- A running switch tool gets a 10pt `live` badge (6pt on the 20pt size). Light assets in the light theme, dark in dark.
