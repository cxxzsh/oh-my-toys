# TextField

文本输入框，含搜索框和出错状态。

- **使用方提供**：标签、占位文字、值、出错信息（如有）。
- 底色 `fill-control`，无边框，圆角 `radius-md`，高 `control-height`；聚焦时 1px `focus-ring` 边加淡色光晕；出错时 1px `danger` 边，下方 `danger` 色说明，前面带警告图标。
- 标签放在上方，`callout`、`text-secondary`。文件名、正则、端口号用 `mono` 字体。
- 出错信息说清楚哪里不对，并给出正确的例子。在提交时校验，不在每次按键时报错；用户一改内容就清掉错误。
