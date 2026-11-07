# ProcessIcon

The icon in front of a process name, so a row is recognised before it is read: the program's own icon from the OS when it has a real one, otherwise one of two default icons.

**The view provides:** the process's executable path, its display name, and whether the tool classes it as a system process.

- **Source.** `mygo.Shell.FileIcon(path, size)`, the icon the file manager shows, requested at `process-icon` (16pt) or `process-icon-lg` (20pt) times the display scale. The same call on every platform.
- **Which path, in order.** 1. The executable itself, when the OS has a real icon for it (an app's main binary, a Windows .exe with its own icon). 2. The app the executable belongs to: on macOS the nearest enclosing `.app` bundle (a helper inside `Foo.app/Contents/` shows Foo's icon); on Linux the desktop entry whose `Exec` names it. 3. Otherwise a **default icon**.
- **Generic icons are not icons.** Plain binaries (`node`, `postgres`, `vite`, `go-api`) only get the OS's generic executable icon. Compare against it (fetched once at start from a known plain binary) and use the default icon instead.
- **Default icons** (Process icon asset group). `process-default`, a command prompt `>_`, for the user's own processes. `process-system`, a server rack, for system services: processes the tool classes as System or Protected (owned by root, SYSTEM or a service account, or run from the OS's own directories). Drawn on a 16pt grid: a `process-icon` square, `radius-xs`, `fill-control-hover`, with the SVG as a mask filled with `text-secondary`. At 20pt the whole icon scales. Neutral on purpose: colour in the app means category or status, never a process.
- **Drawing.** OS icons are drawn as they come: no mask, tint, border or shadow, in both themes. Decorative: hidden from screen readers, since the name follows.
- **Loading.** Never on the frame: rows show the default icon, the OS icon is fetched for visible rows only, cached by executable path for the session, and swapped in when ready, without animation. A failed or empty fetch keeps the default icon.
- **Where.** DataTable Process column (16pt), Dialog item lists (16pt), the command palette and a process's detail header (20pt).
