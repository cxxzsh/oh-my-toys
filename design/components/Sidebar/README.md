# Sidebar

The fixed 220pt column that lists the tools, with the daemon status pinned to the bottom.

**The view provides:** the tools in registry order, their running state, the selection.

- Over the window material with `bg-sidebar-material`; solid `bg-sidebar` without material. `line` on the trailing edge, padding `space-12`, rows `space-4` apart.
- Eyebrow in `eyebrow`, `text-muted`: "Tools" in the main view, "Settings" in the settings scope.
- **Keyboard.** One tab stop. ↑↓ move the selection and show the tool at once; Home and End jump; typing a letter jumps to the next tool starting with it. Mod+1…9 select the first nine tools from anywhere.
- **Context menu** on a row: Open, Pin to Home / Unpin, Show in Tray / Hide from Tray.
- Never hard-code a tool name: the registry is the source.
