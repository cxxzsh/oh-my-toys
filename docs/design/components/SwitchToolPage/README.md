# SwitchToolPage

开关型工具的页面模板，以防睡眠为例：页头带总开关，下面是状态指标和设置。

- **结构**：ToolHeader（右侧 StatusPill + Switch）→ MetricTile 一行（运行时显示剩余时间等）→ SettingsList 若干组。
- 关闭时指标磁贴显示静态说明（`未运行`），不隐藏，避免页面跳动。
- 开关型工具同时出现在托盘面板的磁贴里；两边状态必须同步。
