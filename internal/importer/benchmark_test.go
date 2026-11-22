package importer

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"tally/internal/store"
)

// Test10KImportPerformance 验证万行成绩工作簿解析与导入用时 ≤ 3 秒。
func Test10KImportPerformance(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "scores_10k.xlsx")

	// 生成 10,000 行成绩表
	f := excelize.NewFile()
	sheet := "成绩库"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"学号", "姓名", "班级", "选科", "语文", "数学", "外语", "物理", "化学", "生物", "总分"}
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for row := 2; row <= 10001; row++ {
		cIdx := (row % 20) + 1
		rowStr := strconv.Itoa(row)
		_ = f.SetCellValue(sheet, "A"+rowStr, "STU"+rowStr)
		_ = f.SetCellValue(sheet, "B"+rowStr, "学生"+rowStr)
		_ = f.SetCellValue(sheet, "C"+rowStr, fmt.Sprintf("高一(%d)班", cIdx))
		_ = f.SetCellValue(sheet, "D"+rowStr, "物化生")
		_ = f.SetCellValue(sheet, "E"+rowStr, 105.5)
		_ = f.SetCellValue(sheet, "F"+rowStr, 110.0)
		_ = f.SetCellValue(sheet, "G"+rowStr, 115.5)
		_ = f.SetCellValue(sheet, "H"+rowStr, 85.0)
		_ = f.SetCellValue(sheet, "I"+rowStr, 88.5)
		_ = f.SetCellValue(sheet, "J"+rowStr, 90.0)
		_ = f.SetCellValue(sheet, "K"+rowStr, 594.5)
	}

	if err := f.SaveAs(path); err != nil {
		t.Fatalf("failed to create 10k test workbook: %v", err)
	}
	_ = f.Close()

	subjects := []store.Subject{
		{Key: "chinese", Name: "语文", Enabled: true},
		{Key: "math", Name: "数学", Enabled: true},
		{Key: "english", Name: "外语", Enabled: true},
		{Key: "physics", Name: "物理", Enabled: true},
		{Key: "chemistry", Name: "化学", Enabled: true},
		{Key: "biology", Name: "生物", Enabled: true},
	}

	// 性能计时开始
	start := time.Now()
	wb, err := InspectWorkbook(path, Options{})
	if err != nil {
		t.Fatalf("InspectWorkbook failed: %v", err)
	}
	table, ok := wb.Sheet(sheet)
	if !ok {
		t.Fatalf("sheet %s not found", sheet)
	}
	mapping := AutoMapping(KindScores, table, subjects)
	data, issues, err := ParseSheet(wb, sheet, KindScores, mapping, subjects)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("ParseSheet failed: %v", err)
	}
	if len(issues) > 0 {
		t.Logf("ParseSheet produced %d issues", len(issues))
	}
	if len(data.Students) != 10000 {
		t.Fatalf("expected 10,000 parsed students, got %d", len(data.Students))
	}

	limit := 3 * time.Second
	if raceDetectorActive {
		limit = 15 * time.Second
	}
	t.Logf("10,000 rows import and parsing took %v (limit: %v)", duration, limit)
	if duration > limit {
		t.Fatalf("10K rows import exceeded %v constraint: took %v", limit, duration)
	}
}
