The tray icon: the four shapes of the app icon (circle, rounded square, triangle, plus) drawn as one single-ink glyph, 1.5pt strokes on an 18pt grid. The same glyph on every platform.

- `tray-black.svg` on a light tray; on macOS pass it as a template image and the system tints it.
- `tray-white.svg` on a dark tray.
- `-active` files fill the circle while any switch tool is on.

Ink is pure black or pure white, following the tray's theme, never the app theme or a colour. 18pt in a top bar, 16pt in a bottom taskbar.
