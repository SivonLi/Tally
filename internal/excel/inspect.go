package excel

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// WorkbookInfo 描述一个工作簿的工作表、合并区域、公式和抽样值。
type WorkbookInfo struct {
	Path   string
	Sheets []SheetInfo
}

// SheetInfo 描述一个工作表的检查结果。
type SheetInfo struct {
	Name         string
	Merges       []string
	FormulaCount int
	Rows         int
	Cols         int
	Values       map[string]string
	Formulas     map[string]string
}

// InspectWorkbook 读取工作簿结构、合并区域、公式数量和抽样值。
func InspectWorkbook(path string) (WorkbookInfo, error) {
	workbook, err := Open(path)
	if err != nil {
		return WorkbookInfo{}, err
	}
	defer workbook.Close()
	info := WorkbookInfo{Path: path}
	for _, name := range workbook.GetSheetList() {
		sheet, err := inspectSheet(workbook, name)
		if err != nil {
			return WorkbookInfo{}, err
		}
		info.Sheets = append(info.Sheets, sheet)
	}
	if len(info.Sheets) == 0 {
		return WorkbookInfo{}, fmt.Errorf("工作簿没有工作表: %s", path)
	}
	return info, nil
}

func inspectSheet(workbook *excelize.File, name string) (SheetInfo, error) {
	rows, err := workbook.GetRows(name)
	if err != nil {
		return SheetInfo{}, err
	}
	sheet := SheetInfo{
		Name:     name,
		Rows:     len(rows),
		Values:   map[string]string{},
		Formulas: map[string]string{},
	}
	for _, row := range rows {
		if len(row) > sheet.Cols {
			sheet.Cols = len(row)
		}
	}
	dimension, err := workbook.GetSheetDimension(name)
	if err != nil {
		return SheetInfo{}, err
	}
	parts := strings.Split(dimension, ":")
	if len(parts) == 1 {
		parts = []string{parts[0], parts[0]}
	}
	if len(parts) != 2 {
		return SheetInfo{}, fmt.Errorf("工作表 %s 的数据范围无法识别: %s", name, dimension)
	}
	_, maxRow, err := excelize.CellNameToCoordinates(parts[0])
	if err != nil {
		return SheetInfo{}, err
	}
	maxCol, lastRow, err := excelize.CellNameToCoordinates(parts[1])
	if err != nil {
		return SheetInfo{}, err
	}
	if lastRow > maxRow {
		maxRow = lastRow
	}
	if maxRow > sheet.Rows {
		sheet.Rows = maxRow
	}
	if maxCol > sheet.Cols {
		sheet.Cols = maxCol
	}
	merges, err := workbook.GetMergeCells(name)
	if err != nil {
		return SheetInfo{}, err
	}
	for _, merge := range merges {
		sheet.Merges = append(sheet.Merges, merge.GetStartAxis()+":"+merge.GetEndAxis())
	}
	sort.Strings(sheet.Merges)
	for rowIndex := 1; rowIndex <= sheet.Rows; rowIndex++ {
		for colIndex := 1; colIndex <= sheet.Cols; colIndex++ {
			axis, err := excelize.CoordinatesToCellName(colIndex, rowIndex)
			if err != nil {
				return SheetInfo{}, err
			}
			value, err := workbook.GetCellValue(name, axis)
			if err != nil {
				return SheetInfo{}, err
			}
			if value != "" && rowIndex <= 10 && colIndex <= 10 {
				sheet.Values[axis] = value
			}
			formula, err := workbook.GetCellFormula(name, axis)
			if err != nil {
				return SheetInfo{}, err
			}
			if formula != "" {
				sheet.FormulaCount++
				if rowIndex <= 10 && colIndex <= 10 {
					sheet.Formulas[axis] = formula
				}
			}
		}
	}
	return sheet, nil
}

// CompareWorkbookInfo 比较两个工作簿的工作表、合并区域、公式和抽样值。
func CompareWorkbookInfo(expected, actual WorkbookInfo) error {
	if len(expected.Sheets) != len(actual.Sheets) {
		return fmt.Errorf("工作表数量不一致: %d 和 %d", len(expected.Sheets), len(actual.Sheets))
	}
	for i, expectedSheet := range expected.Sheets {
		actualSheet := actual.Sheets[i]
		if expectedSheet.Name != actualSheet.Name {
			return fmt.Errorf("工作表名称不一致: %q 和 %q", expectedSheet.Name, actualSheet.Name)
		}
		if strings.Join(expectedSheet.Merges, ",") != strings.Join(actualSheet.Merges, ",") {
			return fmt.Errorf("工作表 %s 的合并区域不一致", expectedSheet.Name)
		}
		if expectedSheet.FormulaCount != actualSheet.FormulaCount {
			return fmt.Errorf("工作表 %s 的公式数量不一致: %d 和 %d", expectedSheet.Name, expectedSheet.FormulaCount, actualSheet.FormulaCount)
		}
		for axis, expectedValue := range expectedSheet.Values {
			if actualValue := actualSheet.Values[axis]; actualValue != expectedValue {
				return fmt.Errorf("工作表 %s 的单元格 %s 不一致: %q 和 %q", expectedSheet.Name, axis, expectedValue, actualValue)
			}
		}
		for axis, expectedFormula := range expectedSheet.Formulas {
			if actualFormula := actualSheet.Formulas[axis]; actualFormula != expectedFormula {
				return fmt.Errorf("工作表 %s 的公式 %s 不一致: %q 和 %q", expectedSheet.Name, axis, expectedFormula, actualFormula)
			}
		}
	}
	return nil
}
