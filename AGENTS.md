# Tally

用 MyGo 构建的桌面应用，原生界面由 `ui` 包绘制，纯 Go，无 cgo，无前端。模块名 `tally`，Go 版本见 `go.mod`。

## 每个任务开始时先选技能

开始任何任务的第一步：按下面的表选出技能，读取对应的 `SKILL.md`，再动手。没有匹配项时读 `mygo-usage`。这一步不允许省略，也不允许跳过技能直接改代码。

| 任务 | 技能 |
|---|---|
| 新增或修改界面、选控件、检查页面是否用了该用的控件 | `mygo-components` |
| 窗口、视图函数、状态与身份、`ui.NewTester` 测试 | `mygo-usage` |
| 对话框、通知、菜单、托盘、剪贴板、全局快捷键、文件监听、电源、网络 | `mygo-desktop` |
| 过渡、弹簧、关键帧、手势动画以及动画测试 | `mygo-motion` |
| 自绘、画布、着色器、路径与文字绘制 | `mygo-drawing` |
| 版本升级、崩溃修复、API 行为不确定 | `mygo-maintenance` |
| 需要 Go 没有的系统能力，用 Swift、Kotlin、Rust 或 Zig 实现 | `mygo-native-modules` |
| 新需求、规格、任务拆分、需求澄清 | `spec-driven-development`、`planning-and-task-breakdown`、`interview-me` |
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

选好技能后用它规定的流程执行。技能与本次任务冲突时，以本文件为准。

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

本地参考资料放在 `private-data/`：需求文档、需求表格一类的 Word 与 Excel 文件，不进版本库。开始一个需求前先看这个目录里有没有对应文件，有就先读完再动手。

## 界面语言

应用必须支持中文与英文切换，两种语言都要完整。

- 所有用户可见文字来自 `locales/en.json` 与 `locales/zh-Hans.json`，禁止在界面代码里写死。控件自带的字符串（按钮标题、菜单项、提示）同样适用。
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
- 测试版本的版本号是 `<基础版本>-test.<运行号>`，标签是 `test-<运行号>`；正式版本标签是 `vX.Y.Z`。测试版本通过 `releases.atom` 找到运行号最大的标签，再读取该标签的清单。
- 渠道默认自动：先请求 GitHub，网络失败或超时后切镜像。设置里允许固定使用某一个渠道。
- 更新说明取自 `CHANGELOG.md` 对应版本的小节。
- 发布流程见 `docs/release.md`。

## 分支与提交

- `test`：测试版本发布分支。功能分支从 `test` 切出，合回 `test` 后由 CI 发布预发布版本。
- `main`：正式版本发布分支。只在发布时从 `test` 合并，推送 `vX.Y.Z` 标签触发正式发布。
- 不直接向 `main` 提交；发布标签必须指向 `main` 上的提交。
- 提交信息按 `docs/commits.md` 的规范写，正文使用简体中文。一次提交只做一件事。
- 版本号只改 `mygo.json` 的 `version`，同时补 `CHANGELOG.md` 的小节，最后才打标签。

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
