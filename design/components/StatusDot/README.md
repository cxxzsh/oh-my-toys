# StatusDot

A 6pt dot (8pt large) for live state; pulsing means running right now.

**The view provides:** tone, whether it pulses, an accessible label when it stands alone.

- `live` + pulse: running (daemon, Keep Awake, auto-refresh). `accent`: selected sidebar row. `muted`: stopped. `warning`, `danger`: attention.
- The 1s ping stops under reduced motion and while the window is hidden.
