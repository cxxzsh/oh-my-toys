# Titlebar

The 48pt titlebar the view draws under the OS window controls: app icon and name, breadcrumb, the page filter, view controls.

**The view provides:** the breadcrumb (tool, then sub-page), the filter placeholder, up to two trailing controls.

- **Window controls are the OS's.** macOS puts the traffic lights at the start, Windows and Linux put minimise, maximise and close at the end. Never draw or move them; lay the titlebar out inside the room `c.TitleBar()` leaves: start padding `max(space-16, TitleBar().Left)`, end padding `max(space-16, TitleBar().Right)`, height `max(titlebar-height, TitleBar().Height)`. Everything else is identical on every platform.
- **Content.** 18pt app icon, app name in `headline` at -0.025em, `›` breadcrumb in `label` (current crumb `text-strong` 600), the SearchField centred, the theme SegmentedControl at the end.
- **Dragging.** The row calls `DragWindow()`: empty space drags, double-click zooms or maximises as the OS does. Controls inside it do not drag.
- **Narrow windows.** The breadcrumb truncates first, then the search field shrinks to 160pt; trailing controls never move.
- **Inactive.** Text turns `text-muted`.
