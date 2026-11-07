# CommandPalette

A keyboard-first launcher over the main window: type to find a tool, an action or a setting, then run it.

**The view provides:** the query, ranked results grouped as Tools, Actions, Settings, the highlighted row.

- **Open** with Mod+K anywhere in the main window. It floats inside the window, `palette-width` 600pt, top edge at 20% of the window height, centred; no scrim. `radius-lg`, `bg-popover-material`, `line-control` rim, `shadow-popover`.
- **Input.** 48pt row, 18pt search glyph, query in `palette-input`, an Esc hint at the end. IME composition shows inline; results update when it commits.
- **Results.** At most 8 rows visible, then scrolls. `palette-row` 36pt, `radius-sm`, inset `space-6`. Each row: a 20pt ToolIcon or a 16pt glyph, title with matched letters in `text-strong` 600, a `text-muted` context, and at the end the run hint or a live control (a switch tool shows its Switch, toggled with Space). The highlighted row is `accent-soft`, its hint `accent-text`.
- **Keys.** ↑↓ move, ↵ opens or runs, Mod+↵ runs a tool's primary action without opening it, Esc clears the query, then closes. Actions with a risk above safe show their Plan first.
- **Empty.** "No matches for “query”" in `text-muted`, plus a Search settings row.
