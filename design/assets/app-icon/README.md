The oh-my-toys app icon: a circle, a rounded square, a triangle and a plus, in the System, Files, Media and Developer category hues.

- `app-icon-light.png`: the primary icon, everywhere the OS shows the app (Dock, taskbar, Start menu, launchers, installers). Build each platform's icon file (`.icns`, `.ico`, PNG set) from it with `mygo build`.
- `app-icon-dark.png`: glowing shapes on an obsidian tile. Titlebar icon in the dark theme.
- `app-icon-mono.png`: white outlines on black, for monochrome contexts. The tray uses the Tray icon glyph instead.

1024×1024 PNG. Never recolour, crop the tile, or redraw the shapes.

`app-icon-light-128.png` and `app-icon-dark-128.png` are the 128×128 copies the app draws in its titlebar and tray (20 to 22 pt, so 128 px is sharp at 5×). They are the only icons the app embeds: decoding a 1024×1024 bitmap costs 4 MB of memory, and the app held two. Make them again from the 1024 files with `sips -Z 128` (macOS) or any resampler when the icon changes.
