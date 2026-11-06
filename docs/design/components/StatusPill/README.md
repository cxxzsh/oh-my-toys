# StatusPill

一个小胶囊，表示工具当前在不在运行，可附带一句状态。

- **使用方提供**：开 / 关，以及一句文字（`运行中`、`运行中 · 剩余 2:29:10`、`已关闭`）。
- 底色 `fill-control`，圆角 `radius-pill`，高 22px；文字 `callout`。开启时圆点为 `success`、文字为 `text-primary`；关闭时圆点 `text-tertiary`、文字 `text-secondary`。
- 圆点永远配文字，不能只靠颜色表示状态。
- 用在首页工具卡片底部、工具页页头右侧。错误不用它表示，错误用文字加 `danger`。
