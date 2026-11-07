# ThemeCard

A selectable card for a palette preset in Settings › Appearance.

**The view provides:** name, subtitle, avatar colours, palette colours, footer, accent hex, selection.

- Three across with `space-12` gutters (fewer as the window narrows). Rest: `line`, `shadow-xs`; hover: `line-hover`, `shadow-sm`; selected: 2pt `accent-strong` border, accent Active badge.
- The avatar and strip show the preset's own colours; everything else stays in system tokens. The grid is one tab stop: arrows move, ↵ applies.
