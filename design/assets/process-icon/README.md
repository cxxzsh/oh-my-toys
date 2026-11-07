Default process icons, for processes the OS has no real icon for. Single-ink masks on a 16pt grid, the size of the whole icon.

- `process-default.svg`: a command prompt `>_`, for the user's own processes (dev servers, scripts, plain binaries).
- `process-system.svg`: a server rack, for system services (processes the tool classes as System or Protected).

The app draws a `process-icon` square in `fill-control-hover` with `radius-xs`, then fills the mask with `text-secondary`; the black ink here is only the mask. They scale as a whole to 20pt (`process-icon-lg`).
