# TrayIcon

The app's icon in the system tray: the four shapes of the app icon as one single-ink glyph. One glyph on every platform; only the ink follows the tray it sits in.

**The view provides:** whether any switch tool is on.

- **Ink.** Black on a light tray, white on a dark one, following the system tray's theme, not the app theme. On macOS hand MyGo the black file as a template image and the system tints it.
- **Active.** While a switch tool is on, the circle fills (`-active` files).
- 18pt in a top menu bar, 16pt in a bottom taskbar. Tooltip: "oh-my-toys" or "oh-my-toys · Keep Awake on".
- Never the full-colour app icon here: it turns to mush at 16pt.
