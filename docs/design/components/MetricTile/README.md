# MetricTile

一个大数字磁贴：左上角小标签、大号数字和单位、下方一行补充。借鉴 Surge 的仪表盘。

- **使用方提供**：标签（`剩余时间`）、数值、单位、补充说明。
- 标签 `eyebrow`、`text-secondary`；数字 `numeric-xl`，等宽数字；单位 `callout`、`text-secondary`，与数字基线对齐，间隔 `space-1`；补充 `callout`。
- `bg-card` 卡片，内边距 `space-4` / `space-5`。一行 2 到 3 个，间距 `space-3`。
- 只放用户最关心的一个数，不放图表装饰。数字会变化时保持等宽，避免跳动。
