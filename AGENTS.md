# Tally

用 MyGo 构建的桌面应用，原生界面由 `ui` 包绘制，纯 Go，无 cgo，无前端。模块名 `tally`，Go 版本见 `go.mod`。

## 每个任务开始时先选技能

开始任何任务的第一步：按下面的表选出技能，读取对应的 `SKILL.md`，再动手。没有匹配项时读 `mygo-usage`。这一步不允许省略，也不允许跳过技能直接改代码。

| 任务 | 技能 |
|---|---|
| 新增或修改界面、选控件、检查页面是否用了该用的控件 | `mygo-components`、`sunbeam` |
| 窗口、视图函数、状态与身份、`ui.NewTester` 测试 | `mygo-usage` |
| 外部自动化测试、驱动运行中的应用、获取控件树与截图 | `mygo-automation` |
| 对话框、通知、菜单、托盘、剪贴板、全局快捷键、文件监听、电源、网络 | `mygo-desktop` |
| 过渡、弹簧、关键帧、手势动画以及动画测试 | `mygo-motion` |
| 自绘、画布、着色器、路径与文字绘制 | `mygo-drawing` |
| 版本升级、崩溃修复、API 行为不确定 | `mygo-maintenance` |
| 需要 Go 没有的系统能力，用 Swift、Kotlin、Rust 或 Zig 实现 | `mygo-native-modules` |
| Excel 导入导出、xlsx 读写、多工作表、样式、合并单元格、打印布局 | `excelize` |
| 新需求、规格、任务划分、需求澄清 | `spec-driven-development`、`planning-and-task-breakdown`、`interview-me` |
| 实现功能、分片提交 | `incremental-implementation`、`test-driven-development` |
| 代码审查、简化实现 | `code-review-and-quality`、`code-simplification` |
| 分支、提交、标签、变更日志 | `git-workflow-and-versioning` |
| 更新更严格的质量数字、维护 `CONSTRAINTS.md` | `constraint-driven-development` |
| 写文档、记录架构决策 | `documentation-and-adrs` |
| 处理用户数据、权限、外部输入 | `security-and-hardening` |
| 日志、指标、崩溃诊断 | `observability-and-instrumentation` |
| 高风险改动、不可逆操作、陌生代码 | `doubt-driven-development` |
| 发布前检查、打标签、发布 | `shipping-and-launch` |
| 核对 API 与文档 | `source-driven-development` |

`mygo-web` 当前不适用：项目没有网页窗口。加入网页窗口时再引用它。

项目技能来源和锁定范围见 `.agents/skills/README.md`。

选好技能后用它规定的流程执行。技能与本次任务冲突时，以本文件为准。

## Waza 工程习惯

Waza 的八个通用技能已全局安装，任务匹配时先读对应的 `SKILL.md`：

- `think`：需求澄清、方案设计、实现前的计划。
- `ui`：界面视觉方向与设计；MyGo 控件选择仍先用 `mygo-components`。
- `check`：任务完成后的审查、合并前检查、发布。
- `hunt`：缺陷、回归、异常行为，先确认根因再修改。
- `write`：中文或英文文案润色。
- `learn`：研究陌生领域、整理资料。
- `read`：读取 URL 或 PDF。
- `health`：检查 agent 配置与项目规则。

同一任务同时匹配 Waza 与 Agent Skills 时，使用 Waza。完整工程规则见 https://github.com/tw93/Waza/blob/main/docs/ai-coding-rules.md。

任务按下面的阶段执行：

- 需求：说明背景、目标与验收标准；新增功能或开关前先确认；每片工作保持最小可用。
- 规则：`AGENTS.md` 只放代码无法表达的非显然规则与被拒绝的方案；必须执行的规则交给 CI 或检查脚本。
- 代码：写代码前说明改动范围与影响；每个功能完成后运行验证；多个 agent 并行时各自使用独立 worktree，只提交自己的改动。
- 调试：先确认根因；两次修复无效后增加日志查看真实数据；三次失败后清理上下文，重述问题、目标与已尝试的做法。
- 审查：写代码与审查分开；审查最多三轮；删除、计费、发布这类不可逆操作由人工确认。
- 发布：每次提交说明改了什么、如何验证、如何回退；面向用户的改动先给用户或小范围试用。
- 复盘：生产缺陷与用户报告必须有回归测试；修复一类问题，不只修一个位置。

## 查文档

MyGo 的源码与文档随模块下载到本地，优先读本地版本，它和 `go.mod` 固定的版本一致：

```sh
MYGO=$(go list -m -f '{{.Dir}}' github.com/egoist/mygo)
```

- 组件目录：`$MYGO/docs/ui/README.md`，中文版在 `$MYGO/docs-zh/ui/`
- UI 规则：`$MYGO/docs/ui/views.md`、`layout.md`、`testing.md`、`windows.md`、`accessibility.md`
- 语言目录与切换：`$MYGO/docs/i18n.md`
- 自动更新：`$MYGO/docs/updates.md`
- 桌面服务：`$MYGO/docs/native.md`
- 接口签名以源码为准，用 `go doc github.com/egoist/mygo/ui.<名称>`

需求阶段文档的入口是 `docs/README.md`。

- `docs/requirements-analysis.md`：业务口径与确认状态，默认值的唯一来源。
- `docs/system-design.md`：当前实现的系统结构、模块职责、运行流程、真实数据库结构、界面状态与限制。
- `docs/assessment-plan.md`：考核方案库、版本状态、完整配置结构、公式规则与核算时选择方案。
- `docs/design-roadmap.md`：后续阶段、目标数据模型、历史数据、同步与迁移。
- `docs/historical-data-checklist.md`：历史数据的目录层级、工作簿与工作表角色、读取规则。

开始需求或者实现前先读 `docs/README.md`。`private-data/` 只放原始 Excel、Word、PDF 和考试目录的说明文件，不复制规则。开始一个需求前先看这个目录里有没有对应文件，有就先读完再动手。

计算规则、公式、指标分值、档位权重、排名并列、分档边界、合并组算法和零目标处理都属于考核方案配置。计算代码只实现通用引擎，默认值从方案版本读取，不在代码里固定另一套口径。

## 界面语言

应用必须支持中文与英文切换，两种语言都要完整。

- 所有用户可见文字来自 `locales/en.json` 与 `locales/zh-Hans.json`，禁止在界面代码里固定写入。控件自带的字符串（按钮标题、菜单项、提示）同样适用。
- `en.json` 是源语言，键必须齐全；新增文字先写 `en.json`，再补 `zh-Hans.json`。`mygo i18n check` 检查两边键是否一致。
- `locales/catalog_gen.go` 由 `go tool mygo generate` 生成，和 catalog 一起提交。改了 `locales/*.json` 就重新生成。
- 切换语言调用 `mygo.App.SetLocale("zh-Hans")` 或 `mygo.App.SetLocale("en")`；跟随系统用 `SetLocale("")`。启动时用 `mygo.App.Locale()` 初始化界面上的选择状态。
- 只设置一次的内容（窗口标题、自建菜单项的标签）必须在 `mygo.App.OnLocaleChanged` 里重新设置；视图函数里构建的文字每帧自动跟随。
- 测试必须固定语言：`ui.NewTester` 用 `SetLocale`，目录文本用 `t.Setenv("MYGO_LOCALE", tag)`。不允许依赖运行机器的系统语言。

## 自动更新

更新走两个渠道：GitHub Releases 与中国镜像，两者读取同一批发布文件。

- `mygo.json` 的 `updates.github` 指向主渠道，`updates.publicKey` 是签名公钥，私钥只放在 CI 的 `MYGO_UPDATER_PRIVATE_KEY` 和密码管理器里。
- 中国镜像是 `gh-proxy.com` 对 GitHub Release 的实时代理，不单独存放文件。镜像地址是 GitHub 地址加 `https://gh-proxy.com/` 前缀，例如 `https://gh-proxy.com/github.com/<owner>/<repo>/releases/latest/download/update-<target>.json`。
- 镜像通过 `mygo.Updater.SetSource` 接入：清单从镜像地址读取，清单里以 `https://github.com/` 开头的文件地址改写为 `https://gh-proxy.com/github.com/` 开头；版本、大小、签名保持原样。签名校验不允许关闭或跳过。
- 正式版本标签是 `vX.Y.Z`，由发布工作流创建。当前不发布测试版本。
- 渠道默认自动：先请求 GitHub，网络失败或超时后切镜像。设置里允许固定使用某一个渠道。
- 更新说明取自 `CHANGELOG.md` 对应版本的小节。
- 发布流程见 `docs/release.md`。

## 分支与提交

- `test`：日常开发分支。功能分支从 `test` 切出，通过 PR 合回；文档和小修复允许直接推送 `test`，推送后仍会运行质量检查。
- `main`：正式版本分支，也是默认分支。每次从 `test` 向 `main` 开 PR，PR 内更新 `mygo.json` 的版本与 `CHANGELOG.md`，合并后自动发布正式版本。
- 不直接向 `main` 提交；发布标签由 `release.yml` 在发布时创建，指向 `main` 上的提交。
- `main` 已从初始提交创建并受分支保护；第一次正式发布时从 `test` 向 `main` 开 PR。
- `test` 与 `main` 都启用分支保护：禁止强制推送与删除；`test` 合并前必须通过 `gates`，`main` 还必须通过 `windows-build` 与 `macos-build`。
- 管理员直接推送 `test` 会绕过必需检查，推送后必须等待质量检查成功；需要强制检查时改走 PR。
- `test` 上 Dependabot 的 `patch` 更新与 GitHub Actions 的 `minor` 更新在检查通过后自动合并；`major` 更新保持人工处理，PR 会自动请求 `SivonLi` 审核，合并使用 squash 与中文标题，`main` 不使用自动合并。
- `main` 的必需批准数为 0，`.github/CODEOWNERS` 只声明归属，不强制审核，避免单人开发被自审规则阻塞。
- 提交信息按 `docs/commits.md` 的规范写，正文使用简体中文。一次提交只做一件事。
- 版本号只改 `mygo.json` 的 `version`，同时补 `CHANGELOG.md` 的小节；新版本必须高于最近一次正式发布版本，不手动打标签。

## 命令

```sh
go tool mygo dev          # 开发，改 Go 文件后自动重建并重启
go tool mygo generate     # 从 locales/ 生成 catalog_gen.go
go tool mygo i18n check   # 检查各语言目录的键是否齐全
go test -race ./...       # 测试
go tool mygo vet          # 原生 UI 生命周期检查
go tool mygo build        # 打包，产物在 build/
go tool mygo doctor       # 检查开发环境
```

Go 由 mise 管理，版本写在 `.mise.toml`：

```sh
eval "$(mise activate zsh)"
```

## 界面与自动化测试

测试流程与规范遵循 `docs/testing.md`：

- **无头测试 (ui.NewTester)**：每个 View 页面与 Modal 弹窗都必须编写专属的 `ui.NewTester` 测试。
- **双态双色快照 (tt.Image)**：每个 View 页面与 Modal 弹窗必须同时覆盖无数据状态（Empty）与有数据状态（Populated），并在虚拟时钟推进稳定后分别生成浅色（`tt.SetDark(false)`）与深色（`tt.SetDark(true)`）两份快照，落盘至 `testdata/snapshots/` 进行视觉比对与防退化断言。
- **自动化插件 (mygo-automation)**：端到端与黑盒自动化使用 MyGo 官方插件 `plugins/automation`（通过 `//go:build automation` 隔离）。外部操作统一使用 `mygo-automation` 命令行工具通过 Unix Domain Socket 驱动，严禁引入特定平台的脆弱无障碍脚本。

## 完成前

按 `CONSTRAINTS.md` 逐条核对。没有当场跑出的结果，不允许声称完成。

```sh
gofmt -l .
go vet ./...
go tool mygo vet
go tool mygo i18n check
go test -race ./...
go tool mygo build
```
