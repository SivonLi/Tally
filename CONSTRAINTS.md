# Tally 质量标准

每次改动都必须满足这里的全部条件。CI 按同样的顺序执行，任何人不得在本地跳过其中一条。

## 必须通过

- `gofmt -l .` 没有输出
- `go vet ./...` 没有输出
- `go tool mygo vet` 没有输出（检查原生 UI 的构建期与元素生命周期）
- `go tool mygo i18n check` 不报告缺失或多余的键
- `go test -race ./...` 全部通过
- `go mod tidy` 之后 `go.mod` 与 `go.sum` 没有变化
- `GOOS=linux GOARCH=amd64 go build -o /dev/null .` 通过
- `GOOS=windows GOARCH=amd64 go build -o /dev/null .` 通过

## 测试

- 模块根目录（应用代码）的语句覆盖率不低于 65%，`locales` 下的生成代码不计入
- 每个交互都必须有对应的 `ui.NewTester` 测试：点击、输入、快捷键、菜单项、上下文菜单
- 不允许删除、跳过或弱化测试来维持覆盖率；覆盖率下降时补测试
- 跳过的测试必须写明原因并引用 issue 编号
- 不得用 mock 或替身替换 `ui` 包的行为；测试直接驱动真实的视图函数
- 修复 bug 时先写一个能复现该 bug 的失败测试
- 测试必须固定界面语言，不依赖运行机器的系统语言

## 体积

- macOS 应用包 `Tally.app` 不超过 12 MB（当前 9.8 MB）
- 新增依赖让应用包增长超过 1 MB 时，在提交说明里写明增长来源与必要性

## 代码

- 不使用 `_ = err` 丢弃错误
- 不用 panic 处理可以预期的失败
- 新增的导出标识符必须有文档注释
- 不保留注释掉的代码、调试输出和占位 TODO
- 不在界面构建函数里做阻塞的 I/O 或耗时计算；结果通过 `Window.Update` 回到界面线程

## 界面

- 选控件前先查 `mygo-components` 中的需求与控件对照表
- 手工拼布局时，在代码注释里写明查过哪些控件以及它们为什么不适用
- 元素与部件只在当前构建期有效；跨构建只保存 `ui.Handle` 与普通数据
- 有状态的控件必须有稳定的 `Key` 或 `Bind` 身份
- 每个可交互控件都有可访问名称，用控件自己的 `Label` 或等价属性提供
- 键盘可以到达每一个可交互控件，焦点顺序与构建顺序一致

## 语言

- 界面代码里不出现用户可见的字面量，全部来自 `locales/`
- `en.json` 与 `zh-Hans.json` 的键完全一致，没有漏翻的条目
- 新增文字同时补两种语言，不允许只加一种

## 更新

- 发布构建必须带 `MYGO_UPDATER_PRIVATE_KEY`，产出 `update-<target>.json`、归档与 `SHA256SUMS`
- 更新签名校验不允许关闭
- 发布前用上一版的应用实际完成一次更新，确认两个渠道都能取到新版

## 发布

- `test` 分支只运行质量检查，不创建测试版本
- 合并到 `test` 的 PR 必须通过 `gates`，`gates` 包含三个平台的交叉编译
- 合并到 `main` 的 PR 必须通过 `gates`、`windows-build` 与 `macos-build`
- 合并到 `main` 的 PR 必须同时更新 `mygo.json` 的 `version` 与 `CHANGELOG.md` 的对应小节
- 合并到 `main` 的版本号必须严格高于最近一次正式发布版本；重复或更低的版本会让发布工作流终止
- Dependabot 的 `patch` 更新与 GitHub Actions 的 `minor` 更新必须通过 `gates` 与 `windows-build` 后才能自动合并，`major` 更新必须人工处理，PR 会自动请求审核人，合并使用 squash 与中文标题
