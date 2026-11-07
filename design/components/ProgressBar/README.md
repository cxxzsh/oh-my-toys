# ProgressBar

A 6pt track for long work (checksums, duplicate scans), with a percentage and a mono status line.

**The view provides:** progress 0–100 or indeterminate, the current item.

- Track `sandbox-bg`, fill `sandbox-blue` → `sandbox-purple`, the source's one gradient: use it nowhere else.
- While it runs, the app icon in the Dock or taskbar shows the same progress, and the work keeps going if the window closes. Pair it with a Cancel button; never block the window.
