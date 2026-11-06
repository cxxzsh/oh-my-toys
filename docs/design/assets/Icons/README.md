# Icons

工具图形来自开源图标库 Lucide v0.460.0（ISC 许可，Lucide Contributors；部分来自 Feather，MIT 许可），许可全文见本组的 `LICENSE-lucide.txt`，分发应用时需要一并附上。24×24 网格、线宽 2、圆头圆角。文件里的墨色是 `#1d1d1f`（菜单栏图标是 `#000000`），因为图片无法继承文字颜色；在代码里使用时改成 `currentColor` 或对应 token。

| 文件 | 工具 | Lucide 名 | 类别 · 颜色 |
| --- | --- | --- | --- |
| `tool-awake.svg` | 防睡眠 | `sun` | 系统 · `cat-orange` 画在 `cat-orange-bg` 上 |
| `tool-ports.svg` | 端口 | `plug` | 网络 · `cat-teal` 画在 `cat-teal-bg` 上 |
| `tool-rename.svg` | 批量重命名 | `text-cursor-input` | 文件 · `cat-blue` 画在 `cat-blue-bg` 上 |
| `tool-dupes.svg` | 找重复文件 | `files` | 文件 · `cat-blue` 画在 `cat-blue-bg` 上 |
| `tool-color.svg` | 颜色 | `pipette` | 开发 · `cat-purple` 画在 `cat-purple-bg` 上 |
| `menubar.svg` | 菜单栏 / 托盘图标，平时（空心） | 自绘 | 单色；Mac 作模板图，Windows 出白 / 黑两版 |
| `menubar-active.svg` | 菜单栏 / 托盘图标，有工具在运行（圆、方、三角变实心） | 自绘 | 同上 |

- 新工具从 Lucide 里选一个还没被其它工具用过的图形；同一类别的工具颜色相同，只靠图形区分。
- 图形放在方块里时约占边长的 60–70%（20px 方块里 14px，28px 里 18px，48px 里 28px）。
- 菜单栏图标是自绘的，20×20 网格、线宽 1.6，形状来自 app 图标的四个小玩具。
