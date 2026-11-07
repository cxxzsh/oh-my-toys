Interface glyphs: Material Symbols Outlined, weight 400, fill 0, grade 0, optical size 20, one SVG per glyph, copied unchanged from google/material-design-icons at commit 10ecc14790612ca8fd164e3f4ffe9c16fc50d5b7 (`symbols/web/<name>/materialsymbolsoutlined/<name>_20px.svg`). Licence: Apache-2.0, in `LICENSE.txt`.

- **Format.** One `<path>`, `viewBox="0 -960 960 960"`, no fill set (black). The app draws them as a mask and fills them with the current text token, so one file serves every theme and state. On this page they show black.
- **Names.** The file name is the Material Symbols name in snake_case (`drive_file_rename`, `power_settings_new`). Tool manifests and code refer to glyphs by this name; never by a Lucide or SF Symbols name.
- **Filled variant.** Only `keep-filled.svg` (a pinned tool). Every other state is expressed with colour, not fill.
- **Adding a glyph.** Take the `_20px.svg` of the outlined style at the same commit, keep the name, add a row to the Icon component's table. Avoid legacy aliases (`file_upload`, `drive_file_rename_outline`): they come in an old 20×20 format without a viewBox.
