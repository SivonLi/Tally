package excel

import (
	"fmt"
	"strings"
)

// SheetData 是从工作表读出的表头和数据行。
type SheetData struct {
	Name      string
	HeaderRow int
	Headers   []string
	Rows      [][]string
}

// ReadSheet 读取一个工作表，并在前十五行内寻找表头。
func ReadSheet(path, name string) (SheetData, error) {
	workbook, err := Open(path)
	if err != nil {
		return SheetData{}, err
	}
	defer workbook.Close()
	rows, err := workbook.GetRows(name)
	if err != nil {
		return SheetData{}, err
	}
	headerRow := findHeaderRow(rows)
	if headerRow < 0 {
		return SheetData{}, fmt.Errorf("工作表 %s 的前十五行没有找到表头", name)
	}
	headers := make([]string, len(rows[headerRow]))
	for i, value := range rows[headerRow] {
		headers[i] = strings.TrimSpace(value)
	}
	data := make([][]string, 0, len(rows)-headerRow-1)
	for _, row := range rows[headerRow+1:] {
		if rowEmpty(row) {
			continue
		}
		data = append(data, row)
	}
	return SheetData{Name: name, HeaderRow: headerRow + 1, Headers: headers, Rows: data}, nil
}

func findHeaderRow(rows [][]string) int {
	keywords := []string{
		"班级", "姓名", "考号", "科目", "任课教师", "目标", "基数", "本科", "达线", "赋分",
		"语文", "数学", "英语", "外语", "清北", "班主任", "特控线", "本科线",
	}
	best := -1
	bestScore := 0
	limit := len(rows)
	if limit > 15 {
		limit = 15
	}
	for i := 0; i < limit; i++ {
		score := 0
		seen := map[string]bool{}
		for _, value := range rows[i] {
			for _, keyword := range keywords {
				if seen[keyword] {
					continue
				}
				if strings.Contains(value, keyword) {
					score++
					seen[keyword] = true
					break
				}
			}
		}
		if score > bestScore {
			bestScore = score
			best = i
		}
	}
	if bestScore < 2 {
		return -1
	}
	return best
}

func rowEmpty(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
