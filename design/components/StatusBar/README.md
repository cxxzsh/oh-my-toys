# StatusBar

A 28pt telemetry strip at the bottom of a tool's workbench: live counts at the start, shortcut hints at the end.

**The view provides:** a title, an optional tag, up to four counts with tones, up to three shortcut hints.

- `meta` throughout; tones `accent` for counts, `live` for running state. Values are tabular and update in place.
- Only tools with live data have one (Ports, Keep Awake).
