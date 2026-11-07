# TrayPanel

The quick panel that opens from the tray icon: every switch tool with its control, the tools pinned to the tray, and the way back to the main window. The same panel on every platform.

**The view provides:** switch tools with state and time left, the pinned tools, the app actions.

- **Window.** A borderless flyout window, `tray-width` 320pt, height fits content up to 70% of the screen, then scrolls. `radius-lg` 16pt, popover material tinted `bg-popover-material` (solid `bg-popover` without material), `line-control` rim; the OS gives it its shadow.
- **Position** is the only thing that follows the OS: it opens next to the tray icon, below it when the icon is in a top bar (macOS), above it when the icon is in a bottom taskbar (Windows, most Linux desktops), 6pt away, end-aligned to the icon.
- **Content, top to bottom.** Header (app icon, name, Settings and Open buttons) · Switches: a row per switch tool with a compact Switch and a mono status; an on switch with options shows them under its row · Tools: pinned tools, … on those that open a window · Footer: Open oh-my-toys, Settings…, Quit oh-my-toys.
- **Rows.** `tray-row` 32pt, `radius-sm`. Hover: `fill-control`. Keyboard highlight: `accent-soft`.
- **Behaviour.** Left click on the tray icon opens it; right click shows a native menu (Open oh-my-toys, Quit). Closes on Esc, focus loss, and after running a tool action; toggling a switch keeps it open. 120ms fade in, nothing else.
- **Keyboard.** Opens with the first row highlighted. ↑↓ move, Space toggles, ↵ runs, ←→ change a segmented option.
