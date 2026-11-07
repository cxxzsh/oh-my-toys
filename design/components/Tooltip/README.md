# Tooltip

A short label that names an icon-only control and its shortcut.

**The view provides:** the label and, if it has one, the shortcut.

- After the pointer rests 600ms, below the control, 4pt away; hides on leave, click or key press. Moving to the next control of the same group shows its tooltip at once.
- `bg-popover`, `line-control` rim, `radius-xs`, `shadow-popover`; label in `caption`, shortcut as a mono hint.
- Only for controls without a visible label; never help text. The label is also the accessible name.
