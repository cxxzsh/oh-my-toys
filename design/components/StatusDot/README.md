# StatusDot

A 6pt dot (8pt large) for live state. It is static: a dot that animated would make the window draw every frame for as long as it shows, and a running tool shows one for hours.

**The view provides:** tone, an accessible label when it stands alone.

- `live`: running (daemon, Keep Awake, auto-refresh). `accent`: selected sidebar row. `muted`: stopped. `warning`, `danger`: attention.
