# MyGo 技术验证：Windows 和 Linux 测试清单

Mac 上的项目已经测完（见 `REPORT.md`）。下面这些只能在 Windows 和 Linux 上跑。每一步都写了：怎么做、看什么、记什么。测完把**结果表**和 `results/` 里生成的 JSON 发回来，再填进报告。

大约用时：Windows 40 分钟；每个 Linux 环境 30 分钟。

---

## 0. 准备

### 测试环境（建议，可以按实际情况调整）

| 编号 | 系统 | 桌面 / 会话 | 输入法 | 覆盖的验证项 |
|---|---|---|---|---|
| W1 | Windows 11 24H2 或更新（x64），**最好是真机**，至少一块 150% 或 200% 缩放的屏幕 | — | 微软拼音 | 1–8 |
| L1 | Ubuntu 24.04 / 25.10，GNOME 48+ | **Wayland**（默认） | ibus（系统自带的智能拼音） | 1–8 |
| L2 | 同 L1，登录时选 “Ubuntu on Xorg” | **X11** | ibus | 2、4 |
| L3 | Fedora 42 KDE 或 Kubuntu 25.10 | **KDE Plasma 6 Wayland** | **fcitx5**（fcitx5-chinese-addons） | 1、2、4、8 |

虚拟机可以用（MyGo 在没有 GPU 时用 CPU 画）。但 L1 的“缩放下清晰度”和 W1 的 Mica 最好用真机看。

### 装工具

- Go **1.27.3** 或更新：https://go.dev/dl/ 。装好后 `go version` 要显示 go1.27.3+。
- Git。
- Linux 额外装：`sudo apt install libgtk-3-0 libayatana-appindicator3-1 socat`（Fedora：`sudo dnf install gtk3 libayatana-appindicator-gtk3 socat`）。
- GNOME（L1）额外装 AppIndicator 扩展：`sudo apt install gnome-shell-extension-appindicator`，注销再登录，然后在“扩展”里打开 **AppIndicator and KStatusNotifierItem Support**。

### 取代码

```bash
git clone <本仓库> oh-my-toys && cd oh-my-toys
git checkout main
go test ./...
```

`go test` 应该全部 `ok`。它会打印两行数字（在 `-v` 下可见），顺手记下来：

```bash
go test ./internal/ui/shell/ -run 'TestPaletteKeystroke|TestHomeScroll' -v
```

记：`palette keystroke (CPU frame): … median … p95 …` 和 `home scroll frame (CPU): …`。

---

## 1. 自动部分（先跑，最省事）

这一步用 MyGo 的自动化插件（读的是屏幕阅读器看到的同一棵元素树）测：侧边栏的无障碍名称、防睡眠是否真的生效、端口能否列出监听、颜色能否换算。按键和滚动的耗时已由上面的 `go test` 测过。**屏幕要亮着、不锁屏**：MyGo 的帧跟着显示器刷新，屏幕睡眠时界面不更新，脚本会超时。

**Windows（PowerShell）：**

```powershell
go build -tags e2e -o omt-e2e.exe ./cmd/oh-my-toys
$env:MYGO_AUTOMATION = "$env:TEMP\omt.sock"
Start-Process .\omt-e2e.exe
go run ./dev/e2e -socket "$env:TEMP\omt.sock" -out results\e2e-windows-amd64.json
```

**Linux：**

```bash
go build -tags e2e -o omt-e2e ./cmd/oh-my-toys
MYGO_AUTOMATION=/tmp/omt.sock ./omt-e2e &
go run ./dev/e2e -socket /tmp/omt.sock -out results/e2e-linux-$(uname -m)-$XDG_SESSION_TYPE.json
```

看：终端最后打印的 JSON。`awake.on` 应能看到本程序（Windows 是 `powercfg /requests` 里的 `omt-e2e.exe`，Linux 是 `systemd-inhibit --list` 里的条目），`awake.off` 应为空；`ports.summary_shown` 和 `color.rgb_shown` 应为 true。Linux 下 `awake.on` 为空时，另开终端运行 `gnome-session-inhibit --list` 或看 KDE 的“电源管理”，并记下。

跑完关掉程序（托盘图标右键 → 退出，或 Linux 下 `pkill omt-e2e`）。**把生成的 JSON 文件发回来。**

> 如果卡在 `dial … connect` 或报错，记下完整报错。Windows 需要 1803 以后的版本才有 Unix 套接字。

---

## 2. 手动部分

先构建普通版本并运行：

```bash
go build -o omt ./cmd/oh-my-toys        # Windows: go build -o omt.exe ./cmd/oh-my-toys
./omt                                   # Windows: .\omt.exe
./omt -dev                              # 再加 500 个假工具和“崩溃测试”，设置页显示实测数字
```

终端会打印以 `[metric]` 开头的行，下面要记其中一些。用 `-dev` 启动时，主窗口 **设置** 页（侧边栏最下面）的“实测”一节也显示同样的数字。

### 验证 1：托盘快捷面板（必过）

| 步骤 | 看什么 | 记什么 |
|---|---|---|
| 1. 找托盘图标（四个小玩具：圆、方、三角、加号） | Windows：任务栏右下通知区（可能在 ^ 溢出区里）；Linux：顶栏右侧 | 能不能看到；图标颜色在深色/浅色任务栏上是否看得清 |
| 2. **Windows**：左键单击图标 | 面板贴着图标**向上**弹出，圆角、有阴影 | 是否贴着图标；终端里 `tray_open_ms=` 的值（点 5 次，记 5 个） |
| 2. **Linux**：单击图标 | 会弹出一个菜单（AppIndicator 只支持菜单），点“打开快捷面板” | 面板出现在哪里（MyGo 的已知限制：Linux 拿不到图标位置，面板会在主屏中间）；`tray_open_ms=` |
| 3. 点面板外面任意处 | 面板关闭 | 是否关闭 |
| 4. Windows：面板打开时再点一次图标 | 面板关闭，**不应**马上又打开 | 是否正确 |
| 5. Windows：右键图标 | 出现系统菜单（打开快捷面板 / 打开主窗口 / 退出） | 是否出现 |
| 6. 面板里点“防睡眠”格子 | 格子右上出现绿点，“运行中”一节出现；托盘图标变成实心 | 是否同步；图标是否变实心 |
| 7. L1 GNOME：没装 AppIndicator 扩展时启动一次 | 终端是否打印 `tray:` 错误，程序是否还能用 | 记下行为 |

**通过标准**：能弹出；点外面会关；`tray_open_ms` 都 < 100。

### 验证 2：全局快捷键呼出命令面板（必过）

程序启动时会依次尝试 `Alt+Space`、`Alt+Shift+Space`、`Ctrl+Shift+K`，用第一个成功的。终端第二行 `[metric] shortcut="…" failed=[…]` 说明用了哪个、哪些失败及原因。

| 步骤 | 看什么 | 记什么 |
|---|---|---|
| 1. 看终端那一行 | | 用了哪个快捷键；`failed` 里的内容 |
| 2. 切到别的程序（比如浏览器），按该快捷键 | 命令面板在屏幕上方 1/4 处出现，输入框有光标 | 是否出现；从按下到出现是否感觉“立刻” |
| 3. 再按一次 | 面板消失 | |
| 4. 故意制造冲突：关掉程序，用 `./omt -shortcut "Alt+Tab,Ctrl+Shift+K"`（Windows 上 Alt+Tab 被系统占用）重新启动 | 终端 `failed` 里应列出 `Alt+Tab`，设置页“命令面板”一节应出现黄色“快捷键被占用……已改用 Ctrl+Shift+K” | 是否提示 |
| 5. **Linux Wayland（L1、L3）**：Wayland 下由桌面通过门户（portal）绑定快捷键，需要程序“已安装”：先完成下面“验证 7”里的 `.deb` 安装，再从应用菜单启动 oh-my-toys | 第一次可能弹出系统对话框让你确认或改键 | 是否弹框；确认后快捷键是否生效；对话框截图 |
| 6. **Linux X11（L2）**：直接运行 `./omt` | 同第 2 步 | |

**通过标准**：五种环境（Windows、X11、KDE Wayland、GNOME Wayland）都能呼出；被占用时有提示。

### 验证 3：主窗口外观（必过）

| 步骤 | 看什么 | 记什么 |
|---|---|---|
| 1. Windows：默认运行（`-material mica`） | 侧边栏透出 Mica（带一点桌面壁纸的颜色），右侧内容区是实色 | 截一张整窗图 |
| 2. Windows：`./omt.exe -material none` | 侧边栏变成实色灰 | 截图对比 |
| 3. Linux：直接运行（Linux 没有系统材质，自动用实色） | 侧边栏是实色 `#ebebee`（浅色）/ `#1e1e21`（深色），观感是否“好看” | 截图 |
| 4. 拖窗口右下角快速放大缩小 10 秒 | 有没有闪白、黑边、内容拉伸或延迟一拍 | 有 / 没有，严重程度；最好录一段屏 |
| 5. 把窗口拖到 150%/200% 缩放的屏幕上 | 文字和图标是否清晰（不是放大后的模糊） | 截一张局部放大图 |
| 6. 切换系统深色/浅色（Windows：设置 → 个性化 → 颜色；GNOME：设置 → 外观；KDE：系统设置 → 全局主题） | 程序**不重启**就跟着变，托盘面板和命令面板也跟着变 | 是否跟随；变得是否及时 |

### 验证 4：中文输入（必过）

在三个地方各试一次：主窗口侧边栏的搜索框、托盘面板的搜索框、命令面板。

| 步骤 | 看什么 | 记什么 |
|---|---|---|
| 1. 切到中文输入法（Windows：微软拼音；L1/L2：ibus 智能拼音；L3：fcitx5 拼音），点搜索框，打 `fangshuimian` | 输入框里出现带下划线的拼音（组字中）；候选框**紧贴光标下方** | 候选框位置对不对（截图）；有没有出现在屏幕左上角或窗口外 |
| 2. 按空格选第一个候选 | 输入框变成中文，下面出现搜索结果 | 是否上屏；结果是否刷新 |
| 3. 用数字键选第 2、3 个候选，用退格删字 | 正常 | |
| 4. 中文输入法下直接打 `fsm` 回车 | 有的输入法会把 `fsm` 原样上屏 | 记录行为 |
| 5. 命令面板在“组字中”时 | 已知：MyGo 在组字结束前不把拼音交给程序，所以结果在上屏后才变（Mac 也一样） | 确认是否一致 |

**通过标准**：四种输入法都能上屏、候选框位置正确。

### 验证 5：500 个工具下的性能（必过）

`go test` 已经测了按键和滚动。手动再补几项（用 `./omt -dev` 启动，才有 500 个工具）：

| 步骤 | 怎么测 | 记什么 |
|---|---|---|
| 1. 冷启动 | 重启电脑后第一次运行：`./omt -exit-after-first-frame`，再连续运行 5 次 | 6 个 `first_frame_ms=` 值 |
| 2. 闲置内存 | 启动后不动 30 秒。Windows：任务管理器“详细信息”里的“内存（专用工作集）”；Linux：`ps -o rss= -p $(pgrep -x omt)`（KB） | 数字 |
| 3. 闲置 CPU | 不动、鼠标不放在窗口上，60 秒。Windows：任务管理器 CPU 列；Linux：`pidstat -p $(pgrep -x omt) 60 1` 或 `top -p` | 平均 CPU% |
| 4. 手感 | 首页按住滚轮快速上下滚；命令面板连续快速打字 | 是否掉帧、卡顿 |

### 验证 6：后台运行与故障隔离（必过）

| 步骤 | 看什么 | 记什么 |
|---|---|---|
| 1. 打开防睡眠，关掉主窗口（只剩托盘），等 10 分钟以上（比系统设置的“睡眠/关屏时间”长） | 电脑不睡眠、不关屏 | 是否生效 |
| 2. Windows：管理员 PowerShell 运行 `powercfg /requests` | `SYSTEM` 和 `DISPLAY` 下有 omt.exe | 截图或复制文字 |
| 3. Linux：`systemd-inhibit --list` | 有 oh-my-toys / omt 的条目 | 复制文字 |
| 4. 关掉防睡眠后再查一次 | 条目消失 | |
| 5. 用 `./omt -dev` 启动，侧边栏 → 开发 → 崩溃测试，在输入框里输入 `plan` | 只有这个页面显示“崩溃测试出错了”，点“重新加载”恢复；托盘、命令面板、防睡眠都正常 | 是否隔离 |
| 6. 命令面板里执行“崩溃测试：在动作里 panic” | 程序不退出，崩溃测试页显示错误 | 是否隔离 |

### 验证 7：打包（记录即可）

在各自系统上构建安装包（Windows 第一次会自动下载 NSIS）：

```bash
go tool mygo build -o build-pkg
```

| 平台 | 产物（在 `build-pkg/<os>-<arch>/`） | 怎么装 | 记什么 |
|---|---|---|---|
| Windows | `oh-my-toys-setup-0.1.0-amd64.exe` | 双击安装（未签名，SmartScreen 会拦，点“更多信息 → 仍要运行”） | 安装包大小；装好后开始菜单里能否启动；安装目录大小（`%LOCALAPPDATA%\Programs\oh-my-toys`）；卸载是否干净 |
| Linux（Debian/Ubuntu） | `oh-my-toys_0.1.0_amd64.deb` | `sudo apt install ./oh-my-toys_0.1.0_amd64.deb` | 大小；应用菜单里能否找到并启动；依赖是否装全 |
| Linux（Fedora） | `oh-my-toys-0.1.0-linux-amd64.tar.gz` + `install.sh` | `./install.sh`（装到 `~/.local`） | 同上 |

**Windows 便携版**：`oh-my-toys-0.1.0-windows-amd64-portable.zip`（或用 `go run ./dev/portable -exe <oh-my-toys.exe> -out <zip>` 自己打包）。

| 步骤 | 看什么 | 记什么 |
|---|---|---|
| 1. 解压到 U 盘或任意文件夹，双击 `oh-my-toys.exe` | 能启动；设置页“版本”写 “0.1.0 · 便携版” | 是否显示便携版 |
| 2. 钉住一个工具、换一次语言，退出 | 解压出的文件夹里出现 `data\settings.json`；`%APPDATA%\oh-my-toys` 和 `%LOCALAPPDATA%\oh-my-toys` 里没有新文件 | 是否只写在 `data\` |
| 3. 用 `-update` 参数启动 | 终端打印 `[update] skipped: a portable app does not update itself` | 是否跳过 |
| 4. 删掉文件夹 | 开始菜单、“应用和功能”、注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall` 里都没有它的痕迹 | 是否干净 |

**自动更新（可选，Windows 最值得试）**：

```bash
go tool mygo keygen -o ./keys                       # 生成一对测试密钥
# 把 keys/mygo-update.pub 的内容填进 mygo.json 的 updates.publicKey（只在本地改，不要提交）
MYGO_UPDATER_PRIVATE_KEY="$(cat keys/mygo-update.key)" go tool mygo build -o v010
# 安装 v010 里的安装包（Windows）或用 install.sh 装（Linux；.deb 装的版本按设计不能自更新）
# 把 mygo.json 的 version 改成 0.1.1：
MYGO_UPDATER_PRIVATE_KEY="$(cat keys/mygo-update.key)" go tool mygo build -o v011 -skip-installer
# 运行已安装的 0.1.0，指向 v011 的产物目录：
OMT_UPDATE_DIR=$PWD/v011/<os>-<arch> "<已安装的 oh-my-toys 路径>" -update
```

PowerShell 设置环境变量用 `$env:OMT_UPDATE_DIR="..."`。看：终端出现 `[update] 0.1.0 → 0.1.1, installing` 和 `installed in …, relaunching`，程序重启后设置页底部版本变成 0.1.1。记：成功与否、耗时、报错。

### 验证 8：无障碍（记录即可）

| 步骤 | 记什么 |
|---|---|
| Windows：Ctrl+Win+Enter 打开讲述人。点侧边栏，按 Tab / 上下键移动 | 能否读出“所有工具”“防睡眠”等名称；分组（“系统”）是否读出展开/折叠；开关是否读出“开/关” |
| Windows：用 Accessibility Insights 或 Inspect.exe 看一下树（可选） | 截图 |
| Linux：Super+Alt+S 打开 Orca（GNOME）。同样操作 | 同上 |
| Linux：`accerciser`（可选） | 截图 |

---

### 验证 9：多语言（记录即可）

| 步骤 | 看什么 | 记什么 |
|---|---|---|
| 1. 系统语言是中文时启动 | 界面是简体中文；系统语言是英文时是英文 | 是否跟随 |
| 2. 设置 → 语言，依次点 English、简体中文、跟随系统 | 主窗口、托盘面板、命令面板、托盘右键菜单、菜单栏都立刻换语言，不用重启 | 有没有漏翻的地方 |
| 3. 批量重命名：选一个只有 1 个匹配文件的文件夹，再选一个有多个的 | 英文按钮写 “Rename 1 File” / “Rename 3 Files”，中文写“重命名 3 个文件” | 单复数是否正确 |
| 4. 英文下看托盘面板、首页卡片、命令面板 | 英文更长，文字有没有被截断、挤出边框或重叠 | 截图 |

## 3. 结果表（复制这一段，填好发回来）

```
环境编号：W1 / L1 / L2 / L3
系统和版本：
缩放比例：
是否虚拟机：

[自动部分] JSON 文件：已附上 / 失败（报错：）

[1 托盘] 能弹出：  贴着图标：  点外面关闭：  再点图标关闭：  tray_open_ms（5 次）：
         模板/深浅图标看得清：  运行时变实心：  GNOME 无扩展时的行为：
[2 快捷键] 用了哪个：  failed：  能呼出：  被占用时有提示：  Wayland 门户弹框：
[3 外观] Mica/实色截图：  拖动闪烁：  高分屏清晰：  跟随系统深浅色：
[4 输入法] 输入法名称：  组字显示：  候选框位置正确：  上屏后结果刷新：
[5 性能] first_frame_ms ×6：  闲置内存：  闲置 CPU：  滚动/打字手感：
         go test 的两行数字：
[6 后台] 防睡眠 10 分钟生效：  powercfg/systemd-inhibit 输出：  三处 panic 隔离：  不受保护的 goroutine：
[7 打包] 安装包大小：  能装能启动：  安装后大小：  卸载干净：  自动更新：
[8 无障碍] 读屏能读出侧边栏：  按钮/开关状态：
[9 多语言] 跟随系统：  手动切换：  单复数：  英文布局：
其它问题和截图：
```
