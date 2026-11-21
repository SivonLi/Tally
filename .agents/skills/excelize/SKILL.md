---
name: excelize
description: 使用 excelize 读取和写入 xlsx 工作簿，处理多工作表、样式、合并单元格、冻结窗格、筛选和打印布局。用于本项目的 Excel 导入导出；不处理旧版 xls，也不负责界面表格控件。
---

# Excelize

## 适用场景

- 读取学校提供的 xlsx 文件，识别工作表、表头、合并单元格和单元格类型。
- 生成考核结果 xlsx，设置多工作表、字体、填充、边框、对齐、数字格式、列宽、冻结窗格、筛选和打印布局。
- 不处理 `.xls`。遇到旧文件时要求用户另存为 `.xlsx`，或者单独引入只读 `.xls` 的库。
- 界面里的表格显示与选择使用 MyGo 的 `ui.Table`、`ui.CellSelect`，不要用 excelize 代替界面控件。

## 版本与源码

以 `go.mod` 固定的版本为准：

```sh
go list -m -f '{{.Version}}' github.com/xuri/excelize/v2
go list -m -f '{{.Dir}}' github.com/xuri/excelize/v2
```

本地材料：

- 模块自带的 `README_zh.md`。
- `go doc github.com/xuri/excelize/v2`。
- 官方参考：https://pkg.go.dev/github.com/xuri/excelize/v2

## 本项目约定

- 统一通过 `tally/internal/excel` 打开和创建工作簿，不在视图函数里直接操作文件。
- 读取大表时用 `Rows` 逐行读取；普通规模的表格用 `GetRows`。
- 输出先写到同目录的临时文件，校验后再替换目标文件，避免写到一半留下损坏文件。
- 学号、身份证号等按文本写入，保留前导零。
- 日期、百分比、分数使用 `NumFmt` 或 `CustomNumFmt`，不要把格式拼进字符串。
- 样式通过 `NewStyle` 生成样式 ID，再用 `SetCellStyle` 应用；同一个样式复用同一个 ID。
- 合并单元格只合并显示区域，数据写在左上角单元格。
- 导入只读学校原始文件，不修改源文件。

## 主要入口

完整的函数签名和样式字段见 [references/api.md](references/api.md)。

| 任务 | 函数 |
|---|---|
| 打开与创建 | `excelize.OpenFile`、`excelize.OpenReader`、`excelize.NewFile` |
| 多工作表 | `NewSheet`、`GetSheetList`、`GetSheetMap`、`SetSheetName`、`CopySheet`、`MoveSheet`、`DeleteSheet`、`SetActiveSheet` |
| 读取 | `GetRows`、`Rows`、`Cols`、`GetCellValue`、`GetCellType`、`GetCellFormula`、`GetMergeCells` |
| 写入 | `SetCellValue`、`SetSheetRow`、`MergeCell`、`SetColWidth`、`SetRowHeight` |
| 样式 | `NewStyle`、`SetCellStyle`、`GetCellStyle`、`SetConditionalFormat` |
| 冻结与筛选 | `SetPanes`、`AutoFilter`、`AddTable` |
| 打印 | `SetPageLayout`、`SetHeaderFooter`、`InsertPageBreak` |
| 保存 | `SaveAs`、`Write`、`Close` |

## 常见错误

- `Font.Family` 是字体名称，例如「宋体」「微软雅黑」。
- `SetCellStyle` 接收的是 `NewStyle` 返回的整数 ID，不是 `*Style`。
- 样式 ID 只在当前工作簿有效，跨工作簿不能复用。
- 工作表名称最长 31 个字符，不能包含 `: \ / ? * [ ]`。
- `GetRows` 返回格式化后的字符串；需要区分数字、日期、公式时使用 `GetCellType` 和 `GetCellFormula`。
- 公式以 `=` 开头写入；读取公式用 `GetCellFormula`。
- 打开复杂模板再保存时，未支持的对象可能丢失。用学校真实模板验证后再交付。

## 验证

```sh
go test ./internal/excel
go test -race ./...
go tool mygo build
```

构建后检查应用包体积不超过 30 MB。新增依赖让包体积增长超过 1 MB 时，在提交说明里写明增长来源。
