# 项目技能索引

`AGENTS.md` 的技能表是任务路由的唯一来源。新增或者调整技能时，先更新技能表，再更新本文件。

## 技能来源

| 技能 | 来源 | 说明 |
|---|---|---|
| `mygo-components` | `transmit-bug/mygo-skills` | MyGo 控件与布局 |
| `mygo-desktop` | `transmit-bug/mygo-skills` | MyGo 桌面服务 |
| `mygo-drawing` | `transmit-bug/mygo-skills` | MyGo 自绘与画布 |
| `mygo-motion` | `transmit-bug/mygo-skills` | MyGo 动画与手势 |
| `mygo-usage` | `transmit-bug/mygo-skills` | MyGo 窗口、视图、状态与测试 |
| `mygo-automation` | MyGo 官方插件 | 外部自动化测试、驱动运行中的应用、获取控件树与截图 |
| `mygo-web` | `transmit-bug/mygo-skills` | 当前项目没有网页窗口，暂不使用 |
| `mygo-maintenance` | MyGo 自带模板 | 版本升级、崩溃修复、API 行为确认 |
| `mygo-native-modules` | MyGo 自带模板 | Swift、Kotlin、Rust、Zig 原生模块 |
| `excelize` | 项目本地 | xlsx 读取、写入、多工作表、样式与打印布局 |
| `sunbeam` | `AstraFoundry/design` | Sunbeam 视觉规范：暖纸墨色、明黄主动作、Flat/Raised 材质分层、同心圆角与动效曲线 |

`skills-lock.json` 记录前六个从 `transmit-bug/mygo-skills` 安装的技能。`mygo-maintenance` 和 `mygo-native-modules` 来自 MyGo 模板，`excelize` 与 `sunbeam` 是项目本地接入的技能，不在锁定文件中。

## 使用规则

- 处理 Excel 导入导出、xlsx 读写、多工作表、样式、合并单元格、冻结窗格和打印布局时，先读 `excelize/SKILL.md`。
- 处理界面、窗口、动画、自绘、桌面服务时，按 `AGENTS.md` 的技能表选择对应技能。
- 新增项目本地技能时，在 `AGENTS.md` 技能表和本文件同时登记；需要锁定来源时再更新 `skills-lock.json`。
