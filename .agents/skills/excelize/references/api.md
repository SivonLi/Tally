# Excelize API 入口

以 `go.mod` 固定的版本为准，下面以 v2.11.1 的函数签名为依据。调用前用 `go doc` 核对当前版本。

## 打开与创建

```go
func NewFile(opts ...Options) *File
func OpenFile(filename string, opts ...Options) (*File, error)
func OpenReader(r io.Reader, opts ...Options) (*File, error)
func (f *File) Close() error
func (f *File) SaveAs(name string, opts ...Options) error
func (f *File) Write(w io.Writer, opts ...Options) error
```

## 多工作表

```go
func (f *File) NewSheet(sheet string) (int, error)
func (f *File) GetSheetList() []string
func (f *File) GetSheetMap() map[int]string
func (f *File) SetSheetName(source, target string) error
func (f *File) CopySheet(from, to int) error
func (f *File) MoveSheet(source, target string) error
func (f *File) DeleteSheet(sheet string) error
func (f *File) SetActiveSheet(index int)
```

## 读取

```go
func (f *File) GetRows(sheet string, opts ...Options) ([][]string, error)
func (f *File) Rows(sheet string) (*Rows, error)
func (f *File) Cols(sheet string) (*Cols, error)
func (f *File) GetCellValue(sheet, cell string, opts ...Options) (string, error)
func (f *File) GetCellType(sheet, cell string) (CellType, error)
func (f *File) GetCellFormula(sheet, cell string) (string, error)
func (f *File) GetMergeCells(sheet string, withoutValues ...bool) ([]MergeCell, error)
func (f *File) GetCellStyle(sheet, cell string) (int, error)
```

`Rows` 是流式读取，遍历完要调用 `Close`。普通规模的数据用 `GetRows` 更直接。

## 写入

```go
func (f *File) SetCellValue(sheet, cell string, value interface{}) error
func (f *File) SetSheetRow(sheet, cell string, slice interface{}) error
func (f *File) MergeCell(sheet, topLeftCell, bottomRightCell string) error
func (f *File) SetColWidth(sheet, startCol, endCol string, width float64) error
func (f *File) SetRowHeight(sheet string, row int, height float64) error
func (f *File) SetPanes(sheet string, panes *Panes) error
func (f *File) AutoFilter(sheet, rangeRef string, opts []AutoFilterOptions) error
func (f *File) AddTable(sheet string, table *Table) error
```

冻结首行：

```go
f.SetPanes("成绩", &excelize.Panes{
    Freeze:      true,
    YSplit:      1,
    TopLeftCell: "A2",
})
```

## 样式

```go
func (f *File) NewStyle(style *Style) (int, error)
func (f *File) SetCellStyle(sheet, topLeftCell, bottomRightCell string, styleID int) error
func (f *File) SetConditionalFormat(sheet, rangeRef string, opts []ConditionalFormatOptions) error
func (f *File) AddDataValidation(sheet string, dv *DataValidation) error
```

`Style` 的字段：

```go
type Style struct {
    Border        []Border
    Fill          Fill
    Font          *Font
    Alignment     *Alignment
    Protection    *Protection
    NumFmt        int
    DecimalPlaces *int
    CustomNumFmt  *string
    NegRed        bool
}
```

常用字段：

- `Font`：`Bold`、`Italic`、`Underline`、`Strike`、`Family`、`Size`、`Color`、`VertAlign`。
- `Fill`：`Type`、`Pattern`、`Color`、`Transparency`。
- `Border`：`Type`、`Color`、`Style`，四条边分别放在切片里。
- `Alignment`：`Horizontal`、`Vertical`、`WrapText`、`TextRotation`、`ShrinkToFit`、`Indent`。
- `Protection`：`Locked`、`Hidden`。
- `NumFmt`、`CustomNumFmt`、`DecimalPlaces`：数字格式。

## 打印与视图

```go
func (f *File) SetPageLayout(sheet string, opts *PageLayoutOptions) error
func (f *File) SetHeaderFooter(sheet string, opts *HeaderFooterOptions) error
func (f *File) InsertPageBreak(sheet, cell string) error
func (f *File) SetSheetView(sheet string, viewIndex int, opts *ViewOptions) error
```

## 坐标与日期

```go
func ColumnNameToNumber(name string) (int, error)
func ColumnNumberToName(num int) (string, error)
func CellNameToCoordinates(cell string) (int, int, error)
func CoordinatesToCellName(col, row int, abs ...bool) (string, error)
func ExcelDateToTime(excelDate float64, use1904Format bool) (time.Time, error)
```

## 限制

- 只支持 `.xlsx`、`.xlsm`、`.xltx`、`.xltm`，不支持 `.xls`。
- 工作表名称最长 31 个字符，不能包含 `: \ / ? * [ ]`。
- 样式 ID 只在当前工作簿有效。
- 复杂模板里的未支持对象在重新保存时可能丢失，必须用真实模板验证。
- 许可证为 BSD-3-Clause，分发时保留版权声明。
