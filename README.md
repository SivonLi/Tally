# Tally

用 [MyGo](https://mygo.run) 构建的桌面应用。界面由 MyGo 的 `ui` 包绘制，纯 Go，不依赖 cgo 与前端构建工具。

## 环境

- Go 1.27.4（版本固定见 `.mise.toml`，用 `mise install` 安装）
- macOS 开发环境，`mygo doctor` 检查全部依赖

```sh
eval "$(mise activate zsh)"
go tool mygo doctor
```

## 开发

```sh
go tool mygo dev        # 启动开发版本，改 Go 文件后自动重建
go test -race ./...     # 运行测试
go tool mygo build      # 打包，产物在 build/
```

## 目录

| 路径 | 内容 |
|---|---|
| `main.go` | 应用状态与视图函数 |
| `main_test.go` | `ui.NewTester` 驱动的无窗口界面测试 |
| `locales/` | 中文与英文文案目录，`catalog_gen.go` 由 CLI 生成 |
| `mygo.json` | 应用名称、标识、版本、输出目录、语言目录 |
| `resources/` | 图标等随应用分发的资源 |
| `private-data/` | 本地的需求文档与表格，不进版本库 |
| `docs/` | 发布流程等工程文档 |

## 约定

开发规则见 `AGENTS.md`，质量门槛见 `CONSTRAINTS.md`，提交规范见 `docs/commits.md`，发布流程见 `docs/release.md`。

功能分支从 `test` 切出，合并回 `test` 后发布预发布版本；`main` 只接受来自 `test` 的合并，打 `vX.Y.Z` 标签发布正式版本。

## 许可证

MIT，见 `LICENSE`。随仓库包含的第三方组件与它们的许可证见 `NOTICE`。
