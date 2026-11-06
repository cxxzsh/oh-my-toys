# SettingsList

分组设置列表：一张卡片里的若干设置行，行与行之间用细线分隔。参考 Mac 系统设置。

- **使用方提供**：分组标题和每行的图标、名字、说明（可选）、右侧控件。
- 行：16px 线条图标（`text-secondary`）、名字 `headline`、说明 `callout` / `text-secondary`，右侧是 Switch、SegmentedControl，或"当前值 ›"。最小行高 `row-height`，行间 1px `separator`。
- 分组标题在卡片上方，用 `eyebrow`。
- 一行只放一个控件。说明写后果和例外（`手动睡眠和合盖仍照常生效`）。
