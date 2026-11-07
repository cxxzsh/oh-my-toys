# ToolCard

A tool on the Home grid: icon, name, one-line summary, and an inline control or a way in.

**The view provides:** the tool manifest (name, summary, icon, category, kind), its state, pinned state.

- Min height 132pt, padding `space-16`, `radius-md`, `line`; hover: `line-hover` + `shadow-sm`. The grid fits as many 260pt columns as the window allows, gutter `space-12`.
- **Switch tools** end with their Switch and a mono status. **Action tools** end with the last run ("Renamed 42 files · yesterday") and an "Open ›" text link.
- The pin glyph (filled `accent` when pinned) adds the tool to the tray panel.
- Click anywhere but the control opens the tool; ↵ on a focused card does the same; Space toggles a switch tool.
