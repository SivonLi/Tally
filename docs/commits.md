# 提交规范

提交信息使用 Conventional Commits。`type` 与 `scope` 是固定英文标记，其余内容全部使用简体中文。

## 格式

```text
<type>(<scope>): <subject>

<body>

<footer>
```

- `type`：`feat`、`fix`、`refactor`、`docs`、`test`、`chore`、`perf`、`ci`
- `scope`：可选，受影响的模块，小写英文，例如 `ui`、`i18n`、`updates`、`release`、`repo`
- `subject`：简体中文，动词开头，说明这次提交完成的事情；不超过 50 个字符；结尾不加句号
- `body`：可选，简体中文，说明原因、影响与验证方式；每行不超过 72 个字符宽度，一个汉字按两个字符计算
- `footer`：可选，关键字用 `BREAKING CHANGE` 或 `Closes`，说明部分用简体中文

## 规则

- 一次提交只做一件事，不混合格式化、重构与功能改动
- 提交前通过 `CONSTRAINTS.md` 的全部门槛
- 不提交 `build/`、`dist/`、`private-data/`、密钥、token 与 `.scratch/`
- 主题写结果，不写“更新文件”“修改代码”这类没有信息量的内容
- 主题、正文与脚注说明不写英文句子；英文只用于 `type`、`scope`、标识符与命令
- 主题不使用表情符号，不在结尾加句号
- 已推送的提交不改写历史
- 版本号与 `CHANGELOG.md` 的改动可以单独提交；标签由发布工作流创建，不手动打标签

## 依赖更新

Dependabot 自动生成的提交标题由工具产生，可能包含英文。自动合并使用 squash，最终进入 `test` 的提交标题由工作流指定为中文。人工合并 major 依赖 PR 时同样使用 squash，并在合并标题中写中文说明。

## 示例

```text
feat(ui): 增加中英文切换
fix(updates): 修正镜像地址改写
docs(release): 补充 gh-proxy 镜像说明
test(i18n): 覆盖中文与英文文案
chore(repo): 初始化项目骨架
ci(release): 分平台构建更新包
```

带正文与脚注的提交：

```text
feat(updates): 增加中国镜像渠道

GitHub 直连失败时切换到 gh-proxy.com，清单与文件地址按同一
规则改写，版本、大小与签名保持原样。

Closes #12
```
