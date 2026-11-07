# SearchField

The titlebar filter: a pill field that narrows the current page as you type.

**The view provides:** the placeholder (what it filters), the query.

- `search-width` 256pt (160pt minimum), `control-lg` 28pt, `fill-control` with `line-hairline`, 14pt search glyph, the palette shortcut at the end.
- Mod+F focuses it; Esc clears, a second Esc leaves. Filters on every keystroke, after IME composition commits.
- Clicking the shortcut hint opens the CommandPalette.
