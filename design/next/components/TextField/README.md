# TextField

A single-line input for values a tool needs: rename patterns, a port number, a path.

**The view provides:** label, value, placeholder, help or error text; mono for patterns and paths.

- `control-lg` 28pt, `radius-sm` (fields are not pills: their text can be long), `fill-control`, no border at rest. Label above in `label`; help below in `caption`, `text-inactive`.
- **Focus.** `bg-card`, 1pt `accent-strong` border, 3pt `accent-soft` halo. **Error.** 1pt `danger` border, the fix below in `danger` ("Use a number of 0 or more").
- Validate on commit (blur or ↵). Selected text uses `selection`; standard edit shortcuts and the system's edit menu work.
- A path field ends with a "Choose…" button that opens the system file dialog.
