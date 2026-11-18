## 改动内容

<!-- 一句话说明这个改动解决什么问题。 -->

## 验证

<!-- 贴出实际运行的命令与输出，不允许只写“已测试”。 -->

```sh
gofmt -l .
go vet ./...
go tool mygo vet
go tool mygo i18n check
go test -race ./...
go tool mygo build
```

## 检查清单

- [ ] 新增或修改的交互有 `ui.NewTester` 测试
- [ ] 新增文字同时写进 `locales/en.json` 与 `locales/zh-Hans.json`
- [ ] 界面代码里没有用户可见的字面量
- [ ] 没有删除、跳过或弱化测试
- [ ] `CONSTRAINTS.md` 的条目全部满足
- [ ] 涉及发布时，`mygo.json` 的版本与 `CHANGELOG.md` 已更新
