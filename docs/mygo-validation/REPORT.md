# MyGo 技术验证报告

> 状态：**Mac 部分已完成**，并已做出能日常使用的 Mac 版 v0.1（见第 2 节）；Windows / Linux 部分等 `TESTING-win-linux.md` 的结果回来后补进来（表里标“待测”的格子）。
> 日期：2026-11-06 · MyGo **v0.7.4**（锁定）· Go 1.27.3 · onevra v0.2026.17（只用了它的 Lucide 图标包）

## 结论速览

- **Mac 上 1–6 项必过项全部通过。** 其中两点只能靠人眼确认：拖动改大小时有没有闪烁、拼音候选框的位置。已列进文末“Mac 还需人工看一眼”。
- **Windows / Linux 还没测。** 从文档和代码看，有一处已知不能满足标准：Linux 托盘面板没法“贴着图标”弹出。原因是 AppIndicator 不告诉程序点击发生在哪里，不是 MyGo 的问题。Tauri 2 在 Linux 上用的也是 AppIndicator，同样做不到，所以这一条**不构成退回 Tauri 的理由**。
- **框架质量比预想的好：** 性能余量很大，打包和自动更新一次跑通，升到下一个未发布版本时代码零改动，issue 处理很快。
- **主要风险：**
  - 基本一人维护，提交大量由 AI 协作完成，节奏极快（3 周 12 个版本，一个月约 900 次提交）。
  - 仓库 `main` 上的文档跑在 v0.7.4 前面。
  - 托盘面板和命令面板有几处小毛病，都已绕过。
- **建议（暂定）：选 MyGo。** 前提是 Windows 和 Linux 清单里的 1–6 项通过。重点盯三件事：Wayland 下全局快捷键（门户）、Windows 和 Linux 的拼音候选框位置、Windows 的 Mica。
- **onevra：** 不用它的组件，按设计系统自己写（这个原型已经写了一套：组件约 360 行，外壳约 2,000 行）。它的 Lucide 图标包可以用，命令面板里“切换到英文输入法”的做法值得借鉴。

## 1. 环境

| | |
|---|---|
| 机器 | Apple M4，macOS 27.2（26B123） |
| 显示器 | 2560×1440，**1× 缩放**，144 Hz（所以高分屏用 2× 离屏渲染验证，见第 3 项） |
| 输入法 | 微信输入法（WeType）拼音；系统自带“简体拼音”也已启用 |
| 工具链 | Go 1.27.3（mise 安装在用户目录），`go tool mygo` 0.7.4（写在 `go.mod` 的 `tool` 里） |

## 2. 原型做了什么

代码在仓库根目录，**只用原生界面模式**，没有网页。目录按 CONTRIBUTING 的分层：

| 层 | 目录 | 说明 |
|---|---|---|
| 入口 | `cmd/oh-my-toys` | 桌面程序。唯一一处列出全部工具（`tools()`），新增工具只加一行。`-dev` 再加 500 个假工具和“崩溃测试”。命令行 `cmd/omt` 还没写 |
| 界面 | `internal/ui/{ds,comp,icons,shell}` | `ds`：设计令牌，由 `dev/gentokens` 从 `design/tokens.json` **生成**，界面代码里不出现色值；tokens 没给的组件尺寸（侧边栏行高 30 等）抄自设计系统的 `bundle.css`，集中在 `ds/metrics.go`。`comp`：ToolIcon、StatusPill、Switch、Button、Field、Kbd、ToolHeader、MetricTile、Card，基于 MyGo 的无样式基础控件，自带键盘和读屏支持。`shell`：主窗口（侧边栏、首页、工具页、设置）、托盘面板、命令面板。**界面不认识任何具体工具**：开关型工具用通用的开关页，操作型工具用通用的“填写 → 预览 → 执行”页 |
| 核心 | `internal/core` | 注册表、搜索索引（中英两种名字都索引，拼音、首字母、多音字、关键词）、运行器（后台执行、取消、超时、接住 panic）、状态变化事件、设置存储（钉住、最近使用、折叠状态、语言） |
| 工具契约 | `internal/tool` | 清单（Manifest）和两种契约：开关型 `Start / Stop / Status`，操作型 `Fields / Plan / Apply`（`Plan` 不改动任何东西）。见第 5.2 节 |
| 工具 | `internal/tools/<id>` | 防睡眠、批量重命名、找重复文件、端口、颜色，都已可用；`devtools` 只在 `-dev` 下注册 |
| 平台 | `internal/platform/{power,ports,trash}` | 防睡眠、各系统列端口（macOS `lsof`、Linux `ss`、Windows `netstat` + `tasklist`）、移到废纸篓 |
| 多语言 | `internal/i18n` | `en.json`（源语言，每条带说明）和 `zh-Hans.json`，`mygo generate` 生成带类型的函数 |
| 开发工具 | `dev/{gentokens,genicon,e2e}` | 生成令牌、生成 app 图标、端到端检查 |
| 测试 | 各包的 `_test.go` | 搜索、四个操作型工具（不开窗口）、离屏 2× 渲染、500 个工具下每次按键和滚动的帧耗时 |

运行：`go run ./cmd/oh-my-toys`（可选参数：`-dev`、`-material sidebar|glass|mica|none`、`-shortcut …`、`-exit-after-first-frame`、`-screenshots 目录`、`-update`）。

**Mac 版 v0.1（2026-11-06）**：`go tool mygo build` 打出 `oh-my-toys.app`（13 MB，临时签名）和 DMG（4 MB），装在 `/Applications`。可用：防睡眠（侧边栏、首页卡片、托盘、命令面板都能开关；1 小时、2 小时、直到关闭、只防系统睡眠）、端口（列出监听，按端口结束进程，先确认）、颜色（HEX / RGB / HSL 互转与对比度）、批量重命名（先预览再执行）、找重复文件（多余副本移到废纸篓）。界面跟随系统语言，也可在设置里切换。Windows 版用同一份代码交叉编译成 zip，尚未在 Windows 上实测。

截图在 `docs/mygo-validation/screenshots/`：

- `home-*.png`、`awake-*.png`、`palette-*.png`、`tray-*.png`、`settings-*.png` 等：程序用 `Window.CapturePage` 截的，中英文、浅色深色各一套。这种截法**不含**系统材质，所以截图时用了 `-material none`。
- `hidpi-2x-*.png`：2× 离屏渲染。
- `mac-real-*.jpg`：真实屏幕截图（只含本应用的窗口和面板），含材质和输入法组字。

## 3. 九项验证

图例：✅ 通过 · ⚠️ 通过但有问题或需人工补看 · ❌ 不通过 · ⏳ 待测

### 3.1 托盘快捷面板（必过）

| | Mac | Windows | Linux |
|---|---|---|---|
| 能弹出 | ✅ | ⏳ | ⏳（只能从 AppIndicator 菜单打开） |
| 贴着图标 | ✅ 在图标正下方居中 | ⏳（文档说任务栏在下方时向上弹） | ❌ 已知做不到，会出现在主屏中间 |
| 模板图标 | ✅ 黑色透明 PNG，菜单栏深色时系统自动染成白色 | ⏳ 程序按 `Tray.Dark()` 切白/黑两版 | ⏳ |
| GNOME 装扩展后显示 | — | — | ⏳ |
| 点外面关闭 | ✅ | ⏳ | ⏳ |
| 100ms 内弹出 | ✅ **17.9 / 20.7 / 20.7 / 20.8 ms**（4 次真实点击，从点击到面板第一帧画完）；脚本触发 17.4–27.8 ms | ⏳ | ⏳ |

截图：`screenshots/mac-real-tray-panel.jpg`、`tray-*.png`。

做法：面板用 `mygo.NewFlyout`，挂在托盘上（`Tray`），`Focusable`、`Shadow`。启动时就建好并隐藏，点击时只调用 `Show()`（参数 `-tray-reuse`）。面板只读工具的描述信息和运行状态。

遇到的问题和绕过办法：

1. **v0.7.4 没有原生界面的 `FitContent`。** `main` 上的文档有，但 v0.7.4 只支持网页窗口；`ui/fit.go` 是 v0.7.4 之后才加的。原型自己量内容高度，再调 `SetContentSize`（`internal/ui/shell/tray.go` 里的 `fitHeight`，约 20 行）。升级到下一版后可以删掉。
2. **复用面板时，点图标关不掉面板。** 按下图标时 MyGo 先因为失焦把面板收起，随后 `OnClick` 发现面板是隐藏的，又打开了它。文档里保证“点图标会关闭”的写法是每次 `Close` 再重建，复用写法不在保证范围内。绕过：收起后 400ms 内的点击不再打开（`toggleTray`）。可以给上游提 issue。
3. **Linux：** AppIndicator 只给菜单，不报告点击，也不报告图标位置。这是 AppIndicator 协议本身的限制，Tauri 2 的托盘在 Linux 上一样。可以把菜单里的“打开快捷面板”做成第一项；X11 下可以按鼠标位置摆放面板（Wayland 拿不到全局鼠标位置）。

### 3.2 全局快捷键呼出命令面板（必过）

| | Mac | Windows | X11 | KDE Plasma 6 Wayland | GNOME 48+ Wayland |
|---|---|---|---|---|---|
| 能呼出 | ✅ ⌥Space，在别的程序前台时也能弹出 | ⏳ | ⏳ | ⏳（走 XDG GlobalShortcuts 门户） | ⏳（同左；程序需要已安装） |
| 被占用时提示 | ⚠️ 只能发现**本程序内**的冲突（见下） | ⏳（`RegisterHotKey` 会失败，应能提示） | ⏳（`XGrabKey` BadAccess） | ⏳（由桌面弹框处理） | ⏳ |

做法：启动时依次试 `Alt+Space`、`Alt+Shift+Space`、`CmdOrCtrl+Shift+K`，用第一个成功的。失败的写进设置页的黄色提示：“快捷键被占用：…，已改用 …”。命令面板是 `Panel` 窗口（浮在所有程序之上，不抢走前台程序），启动时就建好并隐藏，失焦时隐藏。主窗口里按 ⌘K 也能打开。

问题：**macOS 的 Carbon 热键不会告诉程序“已被别的程序占用”**，两个程序都会收到。比如 Raycast 也用 ⌥Space 时，两边会同时弹出。这是系统限制：Tauri 的 `global-hotkey` 在 Mac 上用的也是 Carbon，结果一样。绕过：

- 设置里提供“录制快捷键”，录完让用户按一次确认能弹出；
- 读 `com.apple.symbolichotkeys`，避开系统已占用的组合；
- 首次启动默认用不太常见的组合，设计系统里的 ⌥Space 写成“推荐，可改”。

### 3.3 主窗口外观（必过）

| | Mac | Windows | Linux |
|---|---|---|---|
| 系统材质 | ✅ `-material sidebar`（默认，系统侧边栏材质）；✅ `-material glass`（`VibrancyGlass`，在 macOS 26 以上就是 `NSGlassEffectView`，Liquid Glass） | ⏳ `-material mica`（Windows 11 22H2 以上） | 没有系统材质，退回实色背景 ⏳ 好不好看 |
| 改窗口大小不闪烁 | ⚠️ 改大小后内容正确重排（首页从 3 列变 4 列）；闪不闪需要人眼看 | ⏳ | ⏳ |
| 高分屏清晰 | ✅ 2× 离屏渲染文字和图标锐利（`hidpi-2x-home-*.png`）；这台显示器本身是 1×，没法实机看 | ⏳ | ⏳ |
| 跟随系统深浅色 | ✅ 用 `Theme.SetSource` 切换，三个窗口实时跟着变（截图就是这样切出来的）；⚠️ 直接改系统外观没有实测（属于系统设置，验证中不做改动），见文末人工项 | ⏳ | ⏳ |

截图：

- `mac-real-main-sidebar-material-dark.jpg`：系统侧边栏材质
- `mac-real-main-liquid-glass.jpg`：Liquid Glass
- `home-*.png`、`awake-*.png`、`hidpi-2x-home-*.png`

说明：

- 任务里写的“glass 插件”（`plugins/glass`）是**窗口内**的 Liquid Glass，用 MyGo 自己的着色器画在窗口里已有内容之上，三个平台效果一样，适合浮在内容上的工具栏、面板。透出桌面的整窗材质是 `WindowOptions.Vibrancy`。原型用后者做侧边栏，这也是设计系统的要求。以后要给 Windows 和 Linux 也做出“玻璃感”，可以用 glass 插件。
- `CapturePage` 截图里看不到系统材质。透明区域会变成白色，所以程序截图时用实色。

### 3.4 中文输入（必过）

| | Mac 拼音（微信输入法） | Windows 微软拼音 | Linux fcitx5 | Linux ibus |
|---|---|---|---|---|
| 组字显示（带下划线的拼音） | ✅ 托盘搜索框、命令面板都正常 | ⏳ | ⏳ | ⏳ |
| 上屏 | ✅ 空格上屏，结果立即刷新，面板跟着变高或变矮 | ⏳ | ⏳ | ⏳ |
| 候选框位置 | ⚠️ 截图工具会过滤掉输入法进程的窗口，看不到；需要人眼看 | ⏳ | ⏳ | ⏳ |

截图：`mac-real-tray-pinyin-composing.jpg`、`mac-real-palette-pinyin-composing.jpg`。

MyGo 在 Mac 上通过 NSTextInputClient 接输入法，Linux 是 GtkIMMulticontext（同时支持 IBus 和 Fcitx），Windows 是 IMM32。

体验问题：组字时程序拿不到拼音，结果要等上屏后才变。这里的搜索本来就支持拼音和首字母，所以命令面板最好在获得焦点时切到英文输入，Raycast、Alfred 都这么做。onevra 的命令面板已经实现了 macOS 上的切换（`InputSource` 选项，用完会切回原来的输入法），可以照抄。

### 3.5 500 个工具下的性能（必过）

共 507 个工具（500 个假的 + 7 个真的），加上快捷动作，搜索约 1,100 条。

| 指标 | Mac 实测 | 标准 | 结论 |
|---|---|---|---|
| 搜索本身（`go test -bench`） | **37–77 µs/次**（10 种查询） | — | — |
| 命令面板每次按键（端到端：自动化插件发按键 → 帧画完 → 回应） | 中位 **6.2 ms**，p95 13.6 ms，最慢 20.4 ms（n=43，含进程间通信） | 50 ms | ✅ |
| 同上，程序内部从“发现查询变了”到“画完” | 0.14–4.9 ms | 50 ms | ✅ |
| 同上，离屏 CPU 渲染（任何平台的保守上限） | 中位 0.27 ms，p95 1.66 ms，最慢 4.0 ms（n=45） | 50 ms | ✅ |
| 同上，按 CONTRIBUTING 分层重构后复测（`go test ./internal/ui/shell/`） | 中位 0.45 ms，p95 0.94 ms，最慢 2.44 ms（n=28） | 50 ms | ✅ |
| 首页虚拟列表滚动（端到端，每步 120 DIP） | 中位 12.5 ms，p95 18.1 ms，最慢 28.6 ms（n=60，含进程间通信） | 流畅 | ✅ |
| 同上，每帧 CPU 渲染 | 中位 1.56 ms，p95 2.58 ms（n=120） | — | — |
| 同上，重构后复测 | 中位 1.96 ms，p95 2.38 ms（n=120） | — | — |
| 冷启动到第一帧 | **192–256 ms**（6 次，`-exit-after-first-frame`，进程总耗时 0.22–0.29 s）；打包后的 .app 首次启动 252 / 305 ms。其中读取注册表只占 3–6 ms | — | 记录 |
| 闲置内存 | phys_footprint **55 MB**（500 个工具）/ 51 MB（0 个工具）；RSS 108–135 MB | — | 记录 |
| 闲置 CPU | 5 次中 4 次在 0.00–0.2%；1 次 0.7%；另有 1 次 5%，没能复现，推测是鼠标停在窗口上触发了悬停重绘 | — | 记录 |

搜索覆盖：名字、全拼、首字母（`fsm` → 防睡眠）、多音字（`plcmm` → 批量重命名）、中英混合（`jsongsh` → JSON 格式化）、英文关键词（`caffein`、`monitor`）。见 `internal/core/search_test.go`。

### 3.6 后台运行与故障隔离（必过）

| | Mac | Windows | Linux |
|---|---|---|---|
| 防睡眠真正生效 | ✅ 打开时 `pmset -g assertions` 里本进程有 `PreventUserIdleSystemSleep` 和 `PreventUserIdleDisplaySleep`；关掉后消失；只剩托盘时照样生效（它不依附任何窗口） | ⏳ `powercfg /requests` | ⏳ `systemd-inhibit` |
| 界面构建中途 panic（在两层容器里面） | ✅ 只有这个工具页显示“崩溃测试出错了”，侧边栏、托盘、命令面板照常；点“重新加载”恢复 | ⏳ | ⏳ |
| 按钮动作 panic | ✅ 同上 | ⏳ | ⏳ |
| 后台 goroutine panic（用 `safe.Go` 启动） | ✅ 同上 | ⏳ | ⏳ |
| 裸 `go func(){ panic() }` | ❌（预期）整个进程退出 | — | — |

数据：`results/e2e-darwin-arm64.json` 里 `panic_isolated` 三项都是 true。这组数据来自分层重构之前的版本；重构后“崩溃测试”只在 `-dev` 下出现（输入 `plan` 会在预览时 panic，它的快捷动作会在执行时 panic），所有工具代码都经 `core.Runner` 执行。重构后又用自动化脚本确认了一次防睡眠：打开时 `pmset` 里有两条断言，名字是 “oh-my-toys: Keep Awake”，关掉后消失。

要点：

- **在构建中途 recover 后，MyGo 的构建器状态没有被弄坏**，后面的帧都正常。MyGo 的元素是带校验的句柄，每一轮构建重新生成，这大概是原因。但文档没有承诺这一点，建议给上游提一个测试用例或问一句。
- **Go 接不住“没有 recover 的 goroutine”里的 panic。** 这是语言本身的限制，换框架也一样，Tauri（Rust）的后台线程 panic 默认也会让进程退出。规则是：工具代码一律用 `safe.Go`。可以写一个 `go vet` 分析器，禁止 `tools/` 下直接写 `go`。第三方或高风险的工具放进**辅助进程**里跑，MyGo 支持把辅助可执行文件打进安装包（distribution.md 的 Helper executables 一节），崩了也只是那个进程退出。
- 小问题：`KeepAwake` 传进去的说明文字没有出现在 pmset 里（显示为 `named: ""`），不影响功能。

### 3.7 打包（记录）

在这台 Mac 上一条命令 `go tool mygo build -platform darwin/arm64,linux/amd64,linux/arm64,windows/amd64` 交叉编译四个平台，**用时约 27 秒**，不需要 cgo。

| 平台 | 产物 | 体积 | 安装和启动 |
|---|---|---|---|
| macOS arm64 | `oh-my-toys.app`（临时签名）、`.dmg` | .app 13 MB，DMG **3.9 MB**，更新包 tar.gz 5.4 MB | ✅ 能启动，菜单栏、托盘、面板都正常 |
| Linux amd64 / arm64 | `.deb`、tar.gz + `install.sh`、PKGBUILD、.desktop | .deb **5.6 MB** / 5.1 MB，可执行文件 13.6 MB | ⏳ |
| Windows amd64 | `oh-my-toys.exe`、tar.gz；**安装程序要用 NSIS**：在 Mac 上要先 `brew install makensis`，在 Windows 上 `mygo build` 会自动下载 | exe **14.2 MB**，tar.gz 5.8 MB | ⏳ |

**Windows 安装包和便携版（在 Mac 上打）：**

- 安装包 `oh-my-toys-setup-0.1.0-amd64.exe`（4.6 MB）：`brew install makensis` 后由 `mygo build` 生成。MyGo 的 NSIS 模板按用户安装到 `%LOCALAPPDATA%\Programs`（不需要管理员），建开始菜单快捷方式，在 `HKCU\…\Uninstall` 登记卸载项，所以出现在“应用和功能”里。注意：这台 Mac 的终端没有设置 `LANG`，NSIS 3.13 在这种情况下连最小的脚本都会以 `std::bad_alloc` 崩溃，设 `LANG=en_US.UTF-8` 后正常。CI 上要记得设置。
- 便携版 `oh-my-toys-0.1.0-windows-amd64-portable.zip`（6.2 MB）：`dev/portable` 把 exe 和标记文件 `portable` 放进同一个文件夹。程序启动时看 exe 旁边有没有这个文件（`core.PortableDir`，有表格测试）；有就用 `App.SetPath` 把 MyGo 的用户数据、缓存、日志目录都指到旁边的 `data\`，不写 `%APPDATA%`，也不自动更新，设置页显示“便携版”。在 Mac 上用同样的方式验证过：设置和窗口状态都写进了 `data/`，系统的应用数据目录没有变化。
- 两个文件都还没有在 Windows 上实测，见 `TESTING-win-linux.md`。
- 之前 app 图标放在 `resources/`，会被当作资源复制到 exe 旁边；已移到 `design/app-icon.png` 并在 `mygo.json` 里用 `icon` 指定。

**自动更新跑通了（Mac）：**

1. 生成密钥，签名构建 0.1.0，装到一个可写的目录；
2. 构建 0.1.1；
3. 带 `-update` 运行 0.1.0：检查更新 → 校验签名 → 安装，用时 **108 ms** → `App.Relaunch` → `Info.plist` 版本变成 0.1.1。

注意：`mygo build` 要求 `updates.url` 是 https，但程序运行时其实也接受本机回环地址的 http。所以测试改用了 `Updater.SetSource`，让程序直接从本地目录读更新文件。这也顺便验证了“自建更新服务”的接口。

另外：验证环境里，输出目录放在外接磁盘上时，`hdiutil attach` 报“权限被拒绝”，DMG 没做出来；换到系统盘就好了。判断是运行环境（沙箱）的限制，不是 MyGo 的问题。

### 3.8 无障碍与框架成熟度（记录）

**无障碍：**

- Mac：用 macOS 辅助功能 API（VoiceOver 读的就是这个）看到侧边栏每一项都是 `AXButton`，名字是中文工具名，搜索框是 `AXTextField`“搜索工具”。
- 自动化插件读到的元素树：98 个节点，**没有一个控件缺名字**。分组标题带 `expanded` 状态，选中项带 `checked`，开关的角色是 `switch`。
- ⚠️ VoiceOver 本身没有开起来实际听（它会接管键盘和声音），列进了人工项。
- 讲述人、Orca ⏳。

**升级一个小版本：**

- 原型基于 v0.7.4。下一版还没发布，所以直接升到 `main`（`v0.7.5-0.20261105183718-bc8a994455d5`，比 v0.7.4 多 203 次提交）：**代码零改动**，`go vet` 和 `go test` 全过。
- 反过来降到 v0.7.3：有 2 处编译错误（`PlaceholderColor` 那时还没有），而且 onevra 会被一起拉回 v0.2026.16。小版本之间新增 API 多、删改少。
- 有过一次大的破坏性改动：v0.3.0（10 月 8 日）把元素换成了带校验的句柄。上游为此提供了自动改代码的工具 `go tool mygo migrate-ui`。

**维护情况（2026-11-06 通过 `gh` 统计）：**

| | |
|---|---|
| 仓库 | egoist/mygo，2026-09-26 创建，MIT，4,788 星，285 fork |
| 发版节奏 | 3 周 12 个版本（v0.5.5 是 10-16，v0.7.4 是 11-01） |
| 提交 | 最近一个月约 910 次。几乎全部来自作者本人（751 + 23 次）；其他贡献者每人最多 16 次；绝大多数提交带 AI 协作者署名（“Co-authored-by: Claude …”） |
| issue | 共 343 个，17 个未关闭。外部用户提的 308 个中：首次回复中位 **0.9 小时**（n=131），关闭用时中位 **1.7 小时**、p75 13 小时 |
| Linux 相关 issue | 35 个，**未关闭 0 个** |

**读文档要注意：** `main` 上的文档比 v0.7.4 新。这次就碰到了三处 v0.7.4 里还没有的东西：原生界面的 `FitContent`、`Theme.InactiveAccent`、`ui.RoleSearch`。要按 tag 读，比如 `$(go env GOMODCACHE)/github.com/egoist/mygo@v0.7.4/docs` 或 `go doc`。

### 3.9 多语言（记录）

| | Mac | Windows | Linux |
|---|---|---|---|
| 中英文切换 | ✅ 跟随系统语言；设置页可选“跟随系统 / English / 简体中文”，主窗口、托盘面板、命令面板、菜单立刻换，不用重启（截图 `*-en-*.png`、`*-zh-Hans-*.png`） | ⏳ | ⏳ |
| 复数和数字 | ✅ ICU 复数（“Rename 1 File / Rename 3 Files”，中文没有复数形式），端口号用 `::group-off` 不加千分位，大小用 `::.0` 一位小数 | ⏳ | ⏳ |
| 英文最长文案 | ⚠️ 托盘格子里的 “Batch Rename”“Find Duplicates” 单行放不下，已改成最多两行；其它位置未见截断 | ⏳ | ⏳ |
| 命令行 | ⏳ `omt` 还没写；`--json` 不翻译的要求留到写命令行时实现 | ⏳ | ⏳ |

做法：`mygo.json` 里 `i18n.dir` 指向 `internal/i18n`，`mygo generate` 从 ARB 目录生成 `i18n.Messages` 的方法，每条消息一个。工具清单里的名字和说明存的是方法表达式（如 `i18n.Messages.AwakeName`），同一份清单能用任何语言读出来，所以搜索索引同时收中英两种名字。

发现的问题：

1. **MyGo 不提供“当前保存的是哪种语言”的查询**。`App.SetLocale` 会记住选择，但没有对应的读取方法，设置页无法显示“当前选的是跟随系统还是手动”。原型在自己的设置文件里另存一份。
2. **生成的代码用了 `init()`**（向 MyGo 注册目录），和 CONTRIBUTING 第 4 节“不用 `init()`”冲突。生成文件已从 lint 中排除，并在 `.golangci.yml` 里写明原因。
3. 用户可见的错误：工具返回 `tool.Problem`（带已翻译的文字），界面原样显示；其它错误只写日志，界面显示通用的“出错了”。

## 4. onevra 评估

| | 情况 |
|---|---|
| 是什么 | 基于 MyGo 的组件库（MIT），oneo-me 一人维护，2026-10-06 创建，277 星，15 个 issue，版本号是日期式（v0.2026.17），一个月 17 个版本。**写死依赖 MyGo v0.7.4** |
| 规模 | 组件约 3.6 万行，含窗口外壳（sidebar、plain）、命令面板、聊天、图表、树等 |
| Lucide 图标 | 1,879 个，每个图标一个函数，用时才解析，许可证文件齐全。**但没有按名字查找的接口**（文档明说不提供运行时注册表），而本项目的注册表是按名字（`"sun"`）引用图标，所以原型加了一张“名字 → 函数”的对照表（`icons/icons.go`） |
| 主题 | 只通过 MyGo 的 `ui.Theme` 换颜色，大约 15 个槽位（背景、表面、强调色……）。组件里还有 39 处写死的颜色，自带的是暖色系（赤陶色强调）。**表达不了本项目设计系统的约 40 个令牌**：fill-selected 和 fill-hover 要分开、bg-elevated、类别色、浅色才有的卡片阴影、侧边栏行高 30 / 分组标题 22 等 |
| 命令面板 | 有分组、关键词，`PreFiltered` 可以接本项目自己的拼音搜索，还有 macOS 输入法切换。但行样式、“工具 / 动作”两组的排法、匹配字加粗、右侧快捷键和“打开 ↩”这些要照设计改，只能通过主题改，改不到位 |
| 和 MyGo 的版本绑定 | 它跟 MyGo 同步发版。降级实验里 onevra 被一起拉动，而且它要的 purego、x/net 版本比 MyGo v0.7.4 测过的更新（Go 取较高版本），两个快速变化的依赖互相牵制 |

**建议：不用 onevra 的组件，按设计系统自己写；它的 Lucide 图标包可以继续用。**

- 理由一：设计系统要求精确到令牌，onevra 的主题接口太粗。
- 理由二：需要的组件不多（ToolIcon、Pill、Switch、Button、搜索框、侧边栏、卡片、面板），基于 MyGo 的无样式基础控件写，原型里 `comp/` 加 `ds/` 约 360 行就够了，键盘和读屏支持也是 MyGo 自带的。
- 理由三：少一个快速变化的依赖。

图标也可以用 onevra 的 `tools/lucidegen` 自己生成一份带名字查找表的包，从而彻底去掉这个依赖。可以借鉴的：命令面板里的输入法切换（`input_source_darwin.go`）、`PreFiltered` 的接口设计。

## 5. 风险和建议

**选 MyGo 的理由（Mac 实测）：**

- 性能余量是标准的 5–10 倍：按键 6 ms（标准 50 ms），托盘 20 ms（标准 100 ms）。
- 闲置 55 MB、几乎不占 CPU，一个 13 MB 的程序 0.2 秒出第一帧。
- 一套 Go 代码交叉编译四个平台；打包和自动更新一条命令；带签名的增量更新是内置的。
- 读屏、输入法、托盘面板、浮动面板、全局快捷键、防睡眠都是框架自带的，不用自己接系统 API。
- 工具注册表、500 个工具、故障隔离在这个结构里都很自然。

**风险和对策：**

| 风险 | 对策 |
|---|---|
| 一人维护、AI 大量参与、节奏极快 | 锁定版本；每次升级跑 `go test`、离屏测试和 `dev/e2e`；依赖面保持在 MyGo 本身（不加 onevra 组件）；关键行为（构建中途 panic、托盘复用）给上游补测试用例 |
| 文档跑在发布版前面 | 按 tag 读文档 |
| Linux 托盘不能贴着图标 | 用菜单项打开，接受这个差异（Tauri 一样） |
| Mac 全局快捷键冲突发现不了 | 快捷键录制 + 试按确认（Tauri 一样） |
| 进程内隔离接不住裸 goroutine 的 panic | 用 `safe.Go` + vet 规则；高风险工具放进辅助进程 |

**什么情况下退回 Tauri 2：** Windows 或 Linux 清单里出现必过项失败，而且绕不过去。最可能的几种：

- 某个输入法候选框位置错乱或不能上屏；
- Wayland 下全局快捷键门户在 KDE 或 GNOME 上不工作；
- Windows 改窗口大小时明显闪烁；
- Mica 出不来。

前两种在 Tauri 里由 WebKitGTK 和 WebView2 处理，可能更成熟，到时候要对比着测。

### 代码规范（CONTRIBUTING 第 4 节）和 MyGo 写法的冲突

原型带了一份按第 4 节配置的 `.golangci.yml`，`golangci-lint run` 结果为 0 个问题；`go vet`、`gofumpt`、`go test -race ./...` 都通过。逐条看：

| 规则 | 和 MyGo 有无冲突 | 处理 |
|---|---|---|
| 函数 ≤ 60 行、嵌套 ≤ 3 层 | MyGo 的界面是“一帧一次的 Go 函数”，用 `Children(func() { … })` 嵌套描述布局，一个页面很容易写长、写深。lint 的嵌套检查（`nestif`）只数 `if`，不数闭包嵌套，所以 lint 抓不到 | 按区块拆成小函数（如 `paletteInput / paletteList / paletteFoot`）；评审时要人工看闭包层数 |
| 不用 `init()`、不用包级变量 | `mygo generate` 生成的目录代码用 `init()` 注册 | 排除生成文件；自己的代码没有 `init()`：图标查找用 `switch`，托盘图标缓存放在 `App` 上 |
| 不要魔法数字 | 设计令牌之外，组件还有一些 `bundle.css` 里的固定尺寸 | 集中在 `internal/ui/ds/metrics.go`，每个都注明来自哪个 CSS 规则 |
| `//nolint` 必须写原因 | 图标表是一个 80 多分支的 `switch`，会超 `funlen` 和 `cyclop` | 一处 `//nolint:funlen,cyclop` 并写明原因；另外几处 `gosec` G304（读用户选的文件夹）也写了原因 |
| 分层（`depguard`） | 工具层不能直接用 MyGo；平台层包了 MyGo 的 `Power` 和 `Shell.TrashItem` | `.golangci.yml` 里写了各层的禁止导入；平台层允许导入 MyGo，这是对“平台层只用标准库”的例外，需要在 CONTRIBUTING 里写明 |
| 工具不能导入核心 | 清单和契约类型原本在核心层，工具要用它们 | 拆出 `internal/tool` 放在工具层之下。建议 CONTRIBUTING 第 1 节的分层表加上这一层 |
| 测试的分层 | 界面层的测试要用 `devtools` 造 500 个假工具 | 测试文件不检查 `depguard`，在配置里写明 |

## 6. Mac 还需人工看一眼（各 1 分钟）

1. 拖窗口右下角快速改大小，看有没有闪白或内容延迟一拍。（自动化工具在拖动过程中拿不到这个程序的辅助功能信息，没能逐帧截图。）
2. 拼音输入时，候选框是不是紧贴光标：托盘搜索框和命令面板各试一次。
3. 系统设置 → 外观里切换深浅色，程序不重启就跟着变。
4. 打开 VoiceOver（⌘F5），用 VO+方向键走一遍侧边栏，听名字和“已展开 / 已折叠”。

## 7. 待填（Windows / Linux 结果回来后）

- 第 3 节各表的 ⏳ 格子
- 各平台的 `results/e2e-*.json`
- 最终结论：选 MyGo / 退回 Tauri 2
