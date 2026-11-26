package excel

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// TableRow 是工作表的一行数据，Number 是原始行号。
type TableRow struct {
	Number int
	Values []string
}

// Table 是一个展开合并单元格后的工作表。
type Table struct {
	Sheet     string
	HeaderRow int
	Headers   []string
	Rows      []TableRow
	LeadRows  []TableRow
}

// ReadTable 读取工作表，展开合并单元格，在前十五行内寻找表头。
func ReadTable(path, sheet string) (Table, error) {
	workbook, err := Open(path)
	if err != nil {
		return Table{}, err
	}
	defer workbook.Close()
	return ReadTableFromWorkbook(workbook, sheet)
}

// ReadMergedRows 读取工作表的全部行，展开合并单元格并保留原始行序与空行。
// 合并区域内的每个单元格都取合并区域左上角的值。
func ReadMergedRows(workbook *excelize.File, sheet string) ([][]string, error) {
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	return expandMerges(workbook, sheet, rows)
}

// ReadTableFromWorkbook 从已打开的工作簿读取工作表。
func ReadTableFromWorkbook(workbook *excelize.File, sheet string) (Table, error) {
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		return Table{}, err
	}
	rows, err = expandMerges(workbook, sheet, rows)
	if err != nil {
		return Table{}, err
	}
	headerIndex := findHeaderRow(rows)
	if headerIndex < 0 {
		return Table{}, fmt.Errorf("工作表 %s 的前十五行没有找到表头", sheet)
	}
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	headers := make([]string, width)
	for i := 0; i < width; i++ {
		value := cellValue(rows, headerIndex, i)
		if value == "" && headerIndex > 0 {
			value = cellValue(rows, headerIndex-1, i)
		}
		headers[i] = value
	}
	table := Table{Sheet: sheet, HeaderRow: headerIndex + 1, Headers: headers}
	for i := 0; i < headerIndex; i++ {
		values := make([]string, width)
		for column := 0; column < width; column++ {
			values[column] = cellValue(rows, i, column)
		}
		table.LeadRows = append(table.LeadRows, TableRow{Number: i + 1, Values: values})
	}
	for i := headerIndex + 1; i < len(rows); i++ {
		values := make([]string, width)
		for column := 0; column < width; column++ {
			values[column] = cellValue(rows, i, column)
		}
		if rowEmpty(values) {
			continue
		}
		table.Rows = append(table.Rows, TableRow{Number: i + 1, Values: values})
	}
	return table, nil
}

func expandMerges(workbook *excelize.File, sheet string, rows [][]string) ([][]string, error) {
	merges, err := workbook.GetMergeCells(sheet)
	if err != nil {
		return nil, err
	}
	for _, merge := range merges {
		startCol, startRow, err := excelize.CellNameToCoordinates(merge.GetStartAxis())
		if err != nil {
			return nil, err
		}
		endCol, endRow, err := excelize.CellNameToCoordinates(merge.GetEndAxis())
		if err != nil {
			return nil, err
		}
		value := cellValue(rows, startRow-1, startCol-1)
		for row := startRow - 1; row < endRow; row++ {
			for len(rows) <= row {
				rows = append(rows, nil)
			}
			for len(rows[row]) <= endCol-1 {
				rows[row] = append(rows[row], "")
			}
			for column := startCol - 1; column < endCol; column++ {
				rows[row][column] = value
			}
		}
	}
	return rows, nil
}

func cellValue(rows [][]string, row, column int) string {
	if row < 0 || row >= len(rows) || column < 0 || column >= len(rows[row]) {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(rows[row][column], "\n", ""))
}

// FindColumn 返回第一个名称完全匹配或者包含关键字的列，找不到时返回 -1。
func FindColumn(headers []string, names ...string) int {
	for _, name := range names {
		for index, header := range headers {
			if header == name {
				return index
			}
		}
	}
	for _, name := range names {
		for index, header := range headers {
			if name != "" && strings.Contains(header, name) {
				return index
			}
		}
	}
	return -1
}
