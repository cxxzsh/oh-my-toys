# MainWindow

The one main window: a 48pt titlebar, a 220pt sidebar of tools, and a scrolling workbench for the selected tool. One layout on every platform.

**The view provides:** the tool list from the registry, the selected tool's page, the window's active state.

- **Size.** Opens at 1120×720 (`window-width`, `window-height`, 14:9), never smaller than 880×600 (`window-min-width`, `window-min-height`). It remembers size, position and selected tool between launches. Resizing never switches layout: only card grids change their column count (260pt minimum per column).
- **Frame.** `TitleBarStyle: TitleBarHidden`: the view draws the titlebar, the OS keeps the window controls, frame, corners, shadow and resize edges.
- **Material.** The sidebar sits on the window material (`Vibrancy`) tinted with `bg-sidebar-material`; titlebar and workbench are opaque (`bg-titlebar`, `bg-canvas`). Where `c.Vibrancy()` reports none, the sidebar is solid `bg-sidebar`.
- **Inactive window.** When another window has focus, accent glyphs and dots turn `text-muted` and titlebar text turns `text-muted`. Live dots keep pulsing: they are data.
- **Closing** the window does not quit: the app keeps running in the tray so switch tools stay on. Quit lives in the tray panel and the app menu.
- **Hide the sidebar** with View › Hide Sidebar (Mod+Shift+S); it never collapses on its own.
