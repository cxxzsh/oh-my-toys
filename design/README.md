oh-my-toys is a desktop toolbox for macOS, Windows and Linux: one main window, a tray panel, and many small tools. A tool is either a **switch** (Keep Awake: on, off, a status) or an **action** (Batch Rename, Find Duplicates: plan, preview, apply). One rule shapes the interface: **the application is the box; the tools are the heroes.** The chrome is quiet; colour, data and motion belong to the tool.

The app is written once in Go and drawn by MyGo, so **one design runs everywhere**: the same fonts, sizes, colours, layouts and button order on every computer. The look is a precision instrument studio: warm alabaster neutrals in light, obsidian carbon in dark, IBM Plex Sans for words, IBM Plex Mono for every technical value, crisp 1pt hairlines, and machined 6pt controls. `tokens.json` feeds `dev/gentokens`; every size is in points (MyGo's DIPs, scaled by the display).

## One design, every platform

Everything in this system is identical on macOS, Windows and Linux. Only what the operating system owns differs, and the app never imitates it:

| Owned by the OS | What the app does |
|---|---|
| Window controls (traffic lights at the start on macOS; minimise, maximise, close at the end elsewhere), frame, corners, shadow, resize edges | `TitleBarStyle: TitleBarHidden`; lay the titlebar out inside the room `c.TitleBar()` reports |
| Tray position (top menu bar on macOS, bottom taskbar elsewhere) | The same TrayPanel opens next to the icon, below or above it |
| Tray ink (light or dark tray) | Black or white tray glyph |
| Modifier key (⌘ on macOS, Ctrl elsewhere) | Shortcuts are defined once with `Mod`; Kbd labels use the user's key names |
| Menus (context menus, the macOS menu bar) | Native menus via MyGo; the system sets content and order |
| Window material (vibrancy, Mica, none) | `Vibrancy` with a solid fallback: `bg-sidebar`, `bg-popover` |
| Chinese and other scripts not in IBM Plex | Fallback to the system's CJK font (PingFang SC, Microsoft YaHei UI, Noto Sans CJK SC) |

Anything not in this table must not branch on the platform.

## Content fundamentals

- **Say what happens, to how many.** Buttons pair a verb with its target: "Kill All Dev Ports (3)", "Rename 3 files", "Import .json". Never "OK", "Submit" or "Done".
- **One wording everywhere.** Settings, Quit, Launch at login, Keep running in the background, tray. No per-platform strings.
- **Casing.** Tool names, sidebar rows, menu items and button labels are Title Case as the tool registers them ("Keep Awake", "Kill All Dev Ports"). Descriptions, options and statuses are sentence case ("Prevent display sleep", "Running · 1:24:10 left"). Uppercase only through `eyebrow`.
- **Status reads like telemetry.** Short and factual, mono when it is a value: "6 Listening", "Auto-refresh 2s", "pid: 4092".
- **Ellipsis** when a dialog, picker or separate window follows ("Kill Process…", "Choose Folder…").
- **One locale at a time.** Every visible string comes from i18n (`en`, `zh-Hans`), with ICU plurals.
- **No emoji, no exclamation marks, no first person.** The app reports state.
- **Errors say the fix.** "Use a number of 0 or more", not "Invalid input".

## Visual foundations

### Themes and materials
- Auto (default) follows the system appearance live; Dark and Light pin it. The tray panel follows the same setting; the tray glyph follows the tray instead.
- The sidebar sits on the window material tinted `bg-sidebar-material`; the tray panel and command palette on popover material tinted `bg-popover-material`. Titlebar and workbench are opaque (`bg-titlebar`, `bg-canvas`). Without material, the solid twins `bg-sidebar` and `bg-popover`.

### Surfaces and depth
| Layer | Light | Dark | Token |
|---|---|---|---|
| Workbench | #fbfbfd | #121318 | `bg-canvas` |
| Titlebar | #f5f5f7 | #16171d | `bg-titlebar` |
| Sidebar (solid) | #f2f2f7 | #16171d | `bg-sidebar` |
| Cards, panels, tables | #ffffff | white 5% | `bg-card` |
| Selected sidebar row | #ffffff | white 15% | `bg-selected` |
| Tray, palette, dialog (solid) | #ffffff | #1e1f25 | `bg-popover` |

- Borders: `line` for dividers and card edges, `line-control` for controls and popovers, `line-hairline` for faint rims and row separators, `line-hover` under the pointer.
- In-window shadows exist only in light: `shadow-xs` under raised pills, `shadow-sm` / `shadow-md` for selected or hovered cards. `shadow-popover` lifts overlays drawn inside the window (palette, dialog, tooltip). Separate OS windows get their shadow from the system.

### Colour
- **One accent**, fixed for the brand: `accent` #0066cc light, #7aa2f7 dark, for the selected nav glyph and dot, panel title glyphs and text links. `accent-strong` draws selection borders, switch and checkbox fills and the focus ring. `accent-soft` highlights the keyboard row in the palette and tray.
- **Inverse for the primary action.** The one primary button and the active chip use `inverse` / `on-inverse`.
- **Status.** `success` / `live` for running or passing, `warning` for conflicts, `danger` for destructive actions; always with a word or glyph.
- **Categories.** Eight fixed pairs colour tools by category, never by tool: `cat-system`, `cat-file`, `cat-dev`, `cat-network`, `cat-text`, `cat-media`, `cat-productivity`, `cat-app`, each on its `-soft` fill. The app icon is built from the System, Files, Media and Developer hues.
- **Sandbox.** `sandbox-*` is a fixed Tokyo Night palette for the theme preview dock only.
- **Text.** `text-strong` headings and values, `text-primary` body, `text-secondary` descriptions, `text-inactive` unselected navigation, `text-muted` hints only.

### Typography
- **Two bundled fonts, the same on every computer.** IBM Plex Sans (`--font-sans`) for words; IBM Plex Mono (`--font-mono`) for anything a machine produced: ports, pids, paths, hex values, versions, counts, shortcuts, table cells, badges. Both are variable-weight TrueType in `fonts/` (SIL OFL 1.1, licences beside them), embedded with `//go:embed` and registered at start with `ui.RegisterFont(data, "IBM Plex Sans")` and `ui.RegisterFont(data, "IBM Plex Mono")`; set IBM Plex Sans as the theme `Font`.
- **Chinese** has no glyphs in either font and falls back, character by character, to the system's CJK font listed in the family stacks.
- **Scale.** Sans: `title` 17/22 bold at -0.025em, `headline` 13/16 semibold, `section` 12/16 bold, `body` 13/18, `label` 12/16 medium, `caption` 11/16, `small` 10/14, `palette-input` 15/20. Mono: `eyebrow` 10/14 semibold caps at 0.05em, `meta` 10/14, `badge` 10/14 semibold, `kbd` 10/14, `numeric` 11/14 semibold.
- Timers, counters and meters use tabular figures. Nothing below 10pt; people who need bigger text use the system's display scaling, which MyGo applies.

### Layout and window sizing
- 4pt module with 2pt half-steps: `space-2` … `space-28`.
- The main window opens at 1120×720 (14:9), minimum 880×600, and restores its last frame. Resizing never switches layout: the sidebar stays 220pt, the workbench takes the rest, card grids fit as many 260pt columns as there is room for, tables gain width.
- Workbench padding `space-16`, sections `space-16` apart, gutters `space-12`.
- Controls: `control-sm` 20pt in panels, `control-md` 24pt for buttons, chips, Select and segmented controls, `control-lg` 28pt for fields. Rows: sidebar 28, table 26, settings 44, tray 32, palette 36.

### Shape
- `radius-sm` (6pt) on buttons, segmented controls, sidebar rows, Select, search and text fields; `radius-full` on switches, filter chips and status dots.
- `radius-md` (8pt) on cards, panels and settings groups; `radius-lg` (12pt) on the tray panel, command palette and dialogs; `radius-xs` (4pt) on badges, Kbd, tooltips and in-row actions.
- Tool icon tiles: 20/5pt, 28/7pt, 48/11pt.

### States
In this order of precedence:
- **Disabled:** `opacity-disabled` (0.45), no hover, skipped by Tab.
- **Pressed:** `fill-control-pressed` while the mouse button is down.
- **Hover:** `fill-control-hover` on filled controls, `fill-control` on rows, `text-strong` on inactive text, `line-hover` on cards. Never required to discover a control.
- **Selected:** the raised white pill (`bg-selected`, `thumb`) in navigation and segments, `inverse` in chips, a 2pt `accent-strong` border on cards, `accent-soft` on table and palette rows.
- **Keyboard focus:** a 2pt `accent-strong` ring at 2pt offset, after keyboard input only.
- **Inactive window:** accent glyphs and dots turn `text-muted`, titlebar text turns `text-muted`; selection stays visible.

### Motion
150ms `cubic-bezier(0.4, 0, 0.2, 1)` for colour, border and knob changes; 120ms fade for the tray panel, palette and tooltips. Nothing slides, bounces or loops: a looping animation keeps the window drawing every frame, so a live dot is static. Reduced motion removes the fades; a hidden window stops all animation.

## Interaction

- **Keyboard first.** Tab moves between control groups; arrow keys move inside a group (sidebar, chips, segmented controls, tables, grids). ↵ runs the default action, Space toggles, Esc cancels or closes the topmost overlay.
- **Shortcuts,** defined once with `Mod`: Mod+K palette · Mod+F filter · Mod+, settings · Mod+1…9 tools · Mod+R refresh · Mod+Shift+S sidebar · Mod+W close the window (the app keeps running) · Mod+Q quit.
- **Pointer.** The arrow cursor on every control; the pointing hand only on text links; resize cursors on table column edges.
- **Right-click** opens a native ContextMenu on every row, card and value. Values (pids, paths, hex, ports) are selectable and copyable.
- **Drag and drop.** Files and folders dropped on the window go to the tool that accepts them; the drop target shows a 2pt `accent-strong` border on `accent-soft`.
- **Dialogs** put Cancel first and the action at the trailing edge, on every platform. Destructive actions always go Plan → Dialog → Apply, with Cancel as the default button.
- **Long work** never blocks the window; it shows a ProgressBar in the page and on the app's Dock or taskbar icon, and ends with a system notification if the window is not in front.

## Iconography

- **Interface glyphs:** Material Symbols Outlined, weight 400, unfilled, optical size 20: the SVGs of the Glyphs asset group, pinned to one commit of google/material-design-icons and embedded in the app (Apache-2.0). Drawn as masks filled with the current text token: 16pt in rows, 13pt in buttons and badges, 18pt in the palette. Referred to by their snake_case names everywhere, tool manifests included.
- **Tool icons:** the Tool icons group, a light and a dark file per tool: a soft square tile with one line glyph and a small state pill.
- **App icon:** `app-icon-light` for the Dock, taskbar, Start menu and installers; `app-icon-dark` in the dark titlebar; `app-icon-mono` for monochrome contexts. Never recoloured or redrawn.
- **Tray icon:** the Tray icon group, one glyph of the four shapes, black or white by tray theme; the circle fills while a switch tool is on.
- No emoji anywhere.

## Accessibility

- Every control has an accessible name (its label, or its tooltip for icon-only controls) and reports its state ("Keep Awake, on, 1:24:10 left") to the screen reader. Dots and colours never carry state alone.
- Respect the system: reduced motion, reduced transparency, increased contrast (borders go to `line-hover`), display scaling.
- Pairs kept exact from the source that miss AA: `text-muted` light (3.15:1 on `bg-canvas`, 2.92:1 on `bg-sidebar`): hints only. `success` light on `success-soft` (2.79:1) and `live-text` light (3.77:1): bold, always with a word. Light category labels for System, Media, Network and Text (3.9–4.3:1): 11pt bold caps only. The white tick on dark `accent-strong` (2.5:1): the filled box carries the state.
