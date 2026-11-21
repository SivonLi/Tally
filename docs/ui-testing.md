# 界面测试

## 适用范围

验证 `build/` 里构建好的应用，或者界面改动之后的回归。界面的控件位置和文字会变化，本文只说明工具怎么用，不记录具体按钮。

当前工具只适用于 macOS 上的产物；Windows 产物需要在 Windows 环境另外测试。

## 怎么测

一次完整测试的流程：

```sh
go tool mygo build
scripts/ui-test/app.sh snapshot
scripts/ui-test/app.sh start
# 用 ax.js 读取界面、点击、输入、选择文件、选择保存位置、截图和自动检查
scripts/ui-test/app.sh restore --start
```

`go tool mygo build` 的产物在 `build/darwin-arm64/Tally.app`。

`snapshot` 保存用户数据和语言偏好，`restore` 把它们恢复。每次测试都要先执行 `snapshot`，测试结束都要执行 `restore`。

测试结束后检查：

- 界面显示的内容与数据库或者导出文件一致。
- 导出文件能被 excelize 重新打开。
- 应用重启后数据仍在。
- 没有出现错误提示，或者“处理中”长时间不消失。

## 用什么测

Computer Use 的 device 通道需要先在 Settings → Integrations → Devices 打开，并在新会话中生效。未开启时使用 macOS 辅助功能接口驱动真实窗口，工具是系统自带的 `osascript`：

```sh
osascript -l JavaScript scripts/ui-test/ax.js <命令>
```

不需要安装依赖。第一次使用时，给运行命令的终端（T3 Code、Terminal 或者 VS Code）辅助功能权限：系统设置 → 隐私与安全性 → 辅助功能。

`ax.js` 读取控件角色、标题、描述、值与位置，也负责截图。它不依赖视觉识别来定位控件，窗口被遮挡或者不在前台也能读取。

device 通道开启后，可以先用 `device_screenshot` 截图确认整体外观，再用 `ax.js` 完成精确操作。

## 读取界面

```sh
# 所有窗口：名称、类型、尺寸
osascript -l JavaScript scripts/ui-test/ax.js windows

# 完整控件树：索引、角色、标题、描述、值、是否可用、位置与尺寸
osascript -l JavaScript scripts/ui-test/ax.js dump

# 只列可读文字，适合快速确认当前页面
osascript -l JavaScript scripts/ui-test/ax.js texts

# 限制深度和节点数，避免长列表太慢
osascript -l JavaScript scripts/ui-test/ax.js dump --depth 3 --limit 300
```

角色在界面改版后一般不变（`AXButton`、`AXTextField`、`AXStaticText`、`AXPopUpButton`、`AXRadioButton`、`AXTable`、`AXRow`、`AXCell`）。文字会随语言和界面调整变化，所以先 `dump` 看当前角色和文字，再用它们查找。

`dump` 给每个节点一个索引；没有可读文字的图标按钮用 `press-index` 点击：

```sh
osascript -l JavaScript scripts/ui-test/ax.js press-index 23
```

## 截图检查

截图用来发现控件树读不出来的视觉问题：文字对比度不足、文字被截断、内容被遮挡、空状态只有一条提示、间距不一致。定位仍然使用控件树，截图上的坐标不参与点击。

### 一次截图加自动检查

```sh
scripts/ui-test/app.sh visual "01-批次空状态-zh"
```

图片和控件快照写入 `.scratch/ui-test/screens/01-批次空状态-zh.png` 和同名 JSON 文件。系统文件对话框使用 `--dialog`：

```sh
scripts/ui-test/app.sh visual "06-导入文件对话框-zh" --dialog
```

应用自己绘制的模态弹窗仍在主窗口里，直接按主窗口截图。

`app.sh visual` 调用 `ax.js visual`，依次执行：

1. 激活 Tally，等待界面稳定，按窗口编号截图。
2. 把当前窗口里的静态文字、角色和位置写入同名 JSON 文件。
3. 调用 `scripts/ui-test/visualcheck.swift` 检查画面是否接近纯色、文字框是否落在窗口内、每段静态文字的估算对比度。
4. 输出截图路径、控件快照路径和对比度候选。

对比度按 WCAG 的亮度公式估算，普通文字使用 4.5，高度达到 22 点的文字使用 3.0。渐变背景、图标文字、半透明遮罩和禁用状态可能产生误报，候选结果必须对照截图确认。

`shot` 只截图，不运行检查，适合补充单个状态的证据：

```sh
osascript -l JavaScript scripts/ui-test/ax.js shot "/绝对路径/.scratch/ui-test/screens/弹窗.png"
osascript -l JavaScript scripts/ui-test/ax.js shot --dialog "/绝对路径/.scratch/ui-test/screens/文件对话框.png"
```

`shot` 通过窗口编号捕获，不受其他窗口遮挡影响。截图前会激活 Tally，标题栏显示为活动状态。Retina 屏幕上的图片是 2 倍像素。窗口编号由 `scripts/ui-test/windowid.swift` 查询，使用系统自带的 `swift`，需要 Xcode Command Line Tools。

第一次使用截图时，给终端“屏幕录制”权限：系统设置 → 隐私与安全性 → 屏幕录制。没有权限时图片会是空白、纯色或者只有桌面背景，`visual` 会报告纯色检查失败。

查看图片时，Codex 使用 `view_image` 打开 PNG；其他环境用预览或者任意图片查看器打开。按 100% 比例查看文字边缘、选中行和状态徽标。

### 每轮固定截取的状态

按页面状态截取，不按按钮截取：

1. 每个主要页面的默认状态。
2. 空状态、错误状态和加载状态。
3. 列表或者表格内容很多时的状态，包括滚动到中部和底部。
4. 每个模态弹窗、确认框和文件对话框。
5. 中文和英文各一遍。
6. 默认窗口大小和最小窗口 900×620 各一遍。
7. 有选中行、禁用控件和输入错误的页面。

文件名使用 `<两位序号>-<状态>-<语言>.png`，例如 `03-方案版本表-选中行-zh.png`。序号按截图顺序递增，状态按页面用途命名，不使用按钮文字。

### 检查清单

- 文字是否被截断、重叠，或者出现不该有的省略号。
- 控件是否越界、被遮挡、超出窗口边界。
- 表格的列宽、表头、长文本和滚动条是否正常。
- 空状态、错误状态和加载状态是否只有一条提示和大片空白。
- 次要文字、禁用按钮和状态徽标的对比度是否足够。
- 选中行的文字与选中背景对比度是否足够。
- 工具栏、卡片和表单的间距与边缘位置是否一致。
- 窗口最小尺寸 900×620 下是否仍然可用。
- 中文和英文切换后是否出现溢出。
- 焦点环、选中状态和悬停状态是否可见。
- 图标和文字在高分屏上是否模糊。
- 滚动到列表底部时最后一行是否完整可见。

## 点击与输入

```sh
# 按角色和文字点击；文字可以来自标题、描述或者值
osascript -l JavaScript scripts/ui-test/ax.js press AXButton "开始核算"

# 按角色和文字写入文本字段
osascript -l JavaScript scripts/ui-test/ax.js set AXTextField "学年" "2026-2027"

# 发送按键：36 是 Return，53 是 Escape
osascript -l JavaScript scripts/ui-test/ax.js key 36

# 输入一串文字
osascript -l JavaScript scripts/ui-test/ax.js type "abc"
```

`press` 和 `set` 找到第一个匹配元素后就停止，不会遍历整棵长列表。`dump` 和 `texts` 会遍历整棵控件树，结果列表很长时使用 `--depth` 或 `--limit`。

弹窗里的控件在同一棵窗口树里，`press` 可以直接找到。文件对话框是独立窗口，读取时加 `--dialog`：

```sh
osascript -l JavaScript scripts/ui-test/ax.js dump --dialog --depth 2
```

## 语言

`press` 和 `set` 按文字查找，测试前先固定语言：

```sh
defaults write cn.ac.yo.tally AppleLanguages -array en
scripts/ui-test/app.sh restart
```

中文使用 `zh-Hans`。界面上的语言切换写入同一个偏好域。`snapshot` 保存原来的偏好，`restore` 会恢复，测试不需要手动改回。

## 文件怎么选

`pick` 处理打开文件对话框：

```sh
osascript -l JavaScript scripts/ui-test/ax.js pick "/绝对路径/工作簿.xlsx"
```

它按以下步骤执行：

1. 找到类型为对话框的窗口，不依赖窗口标题，中英文都可以。
2. 发送 `Command+Shift+G` 打开“前往文件夹”。
3. 把完整路径写入路径输入框并回车。
4. 点击对话框的第二个按钮（“取消”之后的那个，即“打开”或者“Open”）。

路径必须是绝对路径。`private-data/` 里的工作簿只用于读取，导入不会修改源文件。

## 导出路径怎么选

`save` 处理保存对话框：

```sh
mkdir -p .scratch/ui-test/export
osascript -l JavaScript scripts/ui-test/ax.js save "/Users/.../Tally/.scratch/ui-test/export/结果.xlsx"
```

它把路径分成目录和文件名：先跳转到目录，再把文件名写进文件名输入框，最后点击第二个按钮（“保存”或者“Save”）。

导出文件只写到 `.scratch/ui-test/export/`，不要写到桌面、下载目录或者仓库根目录。`.scratch/` 已经在 `.gitignore` 中。

## 打开、退出与重启

```sh
scripts/ui-test/app.sh start
scripts/ui-test/app.sh stop
scripts/ui-test/app.sh restart
scripts/ui-test/app.sh status
```

`start` 使用 `open build/darwin-arm64/Tally.app`，等待主窗口出现后再多等 2 秒。不要使用 `open -n`，它会启动多个实例，两个实例同时写一个数据库会互相干扰。

`stop` 先请求应用正常退出，10 秒后再强制结束。强制结束会丢失窗口位置，必要时用 `snapshot` 和 `restore` 恢复。

重启后第一帧可能还没有构建完，先等待再读取：

```sh
scripts/ui-test/app.sh restart
osascript -l JavaScript scripts/ui-test/ax.js texts
```

## 无副作用测试

应用的数据和偏好位置：

| 内容 | 位置 |
|---|---|
| 数据库、导入转换文件、窗口状态、语言文件 | `~/Library/Application Support/Tally/` |
| 应用偏好（包括 `AppleLanguages`） | 偏好域 `cn.ac.yo.tally` |

`snapshot` 复制整个数据目录，并导出偏好域。`restore` 在应用退出后删除当前数据目录，再放回快照内容，并恢复偏好域。

规则：

- 测试开始前先退出应用，再执行 `snapshot`；应用在运行时 `snapshot` 会拒绝执行。
- 测试期间只保留一个实例，不要让其他人同时操作这台机器上的 Tally。
- 截图和文件对话框会把 Tally 切到前台，测试期间不要同时操作这台电脑。
- 导入使用 `private-data/` 的原始工作簿，只读，不会改动源文件。
- 导出、临时文件、日志全部写入 `.scratch/ui-test/`，禁止写入 `/tmp`。
- 不修改系统语言。语言切换只改应用偏好域，`restore` 会恢复。
- 需要制造测试状态时（例如先发布一个版本）可以直接修改数据库，`restore` 会还原。
- 测试结束执行 `restore --start`，把数据库、偏好和窗口状态恢复，再重新打开应用。
- 恢复后的数据库是测试前的样子。如果测试前就存在缺陷，恢复后仍会出现，这是预期结果。

## 常见问题

- `不能获得 ... 无效的索引`：窗口顺序变了，或者面板已经关闭。重新执行 `windows`，等待状态稳定后再操作。
- 文件对话框读取很慢：不要对对话框执行完整的 `dump`。对话框里有大量文件行，只读取顶层按钮和文本框，或者直接使用 `pick` 和 `save`。
- `press` 找不到元素：文字可能随语言变化。先用 `texts` 或者 `dump` 看当前文字，确认语言后再按文字查找。
- 长列表导致命令很慢：`dump` 和 `texts` 加 `--depth` 与 `--limit`；`press` 和 `set` 已经会在找到元素后停止。
- 应用启动后第一帧没有内容：`app.sh start` 已经等待 2 秒；手动启动时同样先等待再读取。
- 提示缺少辅助功能权限：到系统设置里给终端授权，然后重新执行命令。
- `visual` 报告纯色检查失败：给终端“屏幕录制”权限，退出并重新打开终端，再执行一次。
- `visual` 报告大量对比度候选：先确认候选文字是否位于渐变、图片或者半透明背景上，再对照截图判断。

## 测试记录

每个缺陷记录复现步骤、实际结果、预期结果、涉及的文件和行号。视觉缺陷附上 `.scratch/ui-test/screens/` 下的截图路径。不要记录按钮坐标，坐标会随窗口大小和界面改版变化。

## 扩展工具

`ax.js` 使用 JavaScript for Automation，入口是 `run(argv)`。所有命令都在 `main(argv)` 里，返回的字符串由 `osascript` 输出。`visual` 调用 `visualcheck.swift` 分析截图和控件位置，自动检查只给出候选，最终判断需要查看图片。修改检查逻辑后运行 `swift scripts/ui-test/visualcheck.swift --selftest`，确认像素坐标和对比度公式仍然正确。新增命令时按同样的方式解析参数、查找元素、执行动作，不要把窗口标题、按钮文字或者控件坐标写成固定值。
