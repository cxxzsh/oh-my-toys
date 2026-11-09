# Icon

Interface glyphs: Material Symbols Outlined, weight 400, unfilled, optical size 20. The app embeds the SVGs of the Glyphs asset group and never loads icons from the network. The same glyphs on every platform.

**The view provides:** the glyph name (snake_case, as in the Glyphs group) and the text token to fill it with.

- **Sizes.** 16pt (`glyph`) in rows, fields and the sidebar; 13pt (`glyph-sm`) in buttons and badges; 18pt in the palette input; 29pt on a 48pt ToolIcon. Always from the 20pt optical size.
- **Colour.** Filled with the text token of its context (`text-secondary` in rows, `text-inactive` on unselected nav). Only the selected nav glyph and panel title glyphs take `accent`. Disabled: same token at `opacity-disabled`.
- **A tool's glyph** is declared in its manifest by name; an unknown name falls back to `apps`.

| Glyph | Used for |
|---|---|
| `search` | Titlebar filter, palette input |
| `tune` | Settings › General |
| `palette` | Settings › Appearance |
| `contrast` | Appearance row |
| `dark_mode` | Dark Mode switch tool |
| `language` | Language row |
| `keyboard` | Shortcuts; keyboard hints |
| `memory` | Daemons & Tools |
| `smart_toy` | MCP & Agent API |
| `lan` | Ports |
| `bedtime_off` | Keep Awake |
| `drive_file_rename` | Batch Rename |
| `content_copy` | Find Duplicates; Copy actions |
| `colorize` | Color |
| `upload` | Import |
| `settings` | Settings button |
| `open_in_new` | Open the main window |
| `power_settings_new` | Quit; Kill All; Launch at login |
| `lock` | Protected and system rows |
| `terminal` | StatusBar |
| `preview` | Live Tool Preview panel |
| `check` | Active badge |
| `unfold_more` | Select |
| `expand_more` | Disclosure |
| `chevron_right` | Settings row that opens a sub-page |
| `keep` | Pin a tool (keep-filled when pinned) |
| `apps` | Fallback for a tool without a known glyph; Home |
| `left_panel_open` | Traffic: show the sources |
| `left_panel_close` | Traffic: hide the sources |
