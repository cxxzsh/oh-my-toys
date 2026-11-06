# SegmentedControl

分段控件：在 2 到 6 个互斥选项里选一个，例如防睡眠的时长、结果的分组方式。

- **使用方提供**：选项文字和当前选中项。
- 轨道 `fill-control`，圆角 `radius-md`，内边距 2px；选中段为 `fill-segment-selected` 滑块，`shadow-knob`，文字加粗。
- 选项文字简短：数字和中文单位之间空一格（`30 分钟`）。
- 选项超过 6 个，或文字长短差很多时，改用弹出菜单。
