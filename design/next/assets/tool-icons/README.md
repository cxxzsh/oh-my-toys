One icon per shipped tool, in a light and a dark version (240×240 PNG): Keep Awake (an eye with an ON pill), Ports (a socket with a port pill), Batch Rename (a pencil), Find Duplicates (two overlapping tiles), Color (an eyedropper with a toggle).

- Light files: a near-white tile (`icon-tile` → `icon-tile-end`) with a single `icon-ink` (#18181b) glyph. Use them in the light theme.
- Dark files: a near-black tile (#131316 → #101012) with a white glyph. Use them in the dark theme.
- Show them through ToolIcon `src`. New tools follow the same recipe: one line glyph, 24-grid strokes, an optional state pill along the bottom.
