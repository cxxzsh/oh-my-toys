# Button

A pill push button whose label is a verb with its target ("Kill All Dev Ports (3)", "Rename 3 files").

**The view provides:** the label, an optional 13pt glyph, the action, enabled state.

- **Sizes.** `control-md` 24pt; `control-sm` 20pt inside panels. Label in `label`.
- **Kinds.** `primary` (`inverse` / `on-inverse`, one per view) · `secondary` (`bg-button`, `line-control`, `shadow-xs`) · `ghost` (`fill-control`) · `plain` (no fill until hovered: icon buttons in toolbars and the tray) · `danger` (tinted: Kill in rows and pages) · `danger-solid` (`danger` / `on-danger`: only a Dialog's confirm). Square corners (`radius-xs`) only for in-row actions in a DataTable.
- **States.** Hover: secondary and ghost go `fill-control-hover`, primary drops to `opacity-hover`. Pressed: `fill-control-pressed`. Keyboard focus: 2pt `accent-strong` ring, 2pt offset. Disabled: `opacity-disabled`.
- **Pointer.** The arrow cursor; the pointing hand is for text links only.
- **Keyboard.** Space activates the focused button; ↵ the view's default button; Esc triggers Cancel where there is one.
- Never "OK" or "Submit". … only when a dialog or picker follows ("Custom…", "Choose Folder…").
