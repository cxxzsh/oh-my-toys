# Dialog

A modal confirmation for an action that cannot be undone, shown after the tool's Plan and before Apply. One layout on every platform.

**The view provides:** a question title, one sentence of consequence, the affected items from the Plan, the verb and count for the confirm button, an optional "don't ask again".

- **Window-modal.** Dims the window with `scrim` and centres a `dialog-width` 400pt card: `bg-popover`, `radius-md`, `shadow-popover`, padding `space-20`. One at a time; the rest of the window ignores input.
- **Text.** Title in `headline`, a question naming the count ("Kill 3 dev processes?"); body in `label` says what is lost. Items in mono, at most 6, then "and 4 more".
- **Buttons.** End-aligned, `space-8` apart: Cancel first, the action last, at the trailing edge. The same order everywhere, so the action is always in the same place for everyone. The destructive action is `danger-solid`.
- **Defaults.** For destructive actions Cancel is the default and holds focus: ↵ and Esc both cancel; the action needs a click, or Tab then Space. For safe confirmations the action is the default.
- Never ask for actions that only change a view, and never stack a second dialog: errors after Apply show in the page.
