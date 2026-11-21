package excel_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"tally/internal/excel"
)

func testDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), "..", "..", ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "excel-inspect-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})
	return dir
}

func createWorkbook(t *testing.T, dir string) string {
	t.Helper()
	workbook := excel.New()
	t.Cleanup(func() {
		if err := workbook.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
	})
	if _, err := workbook.NewSheet("成绩"); err != nil {
		t.Fatal(err)
	}
	for _, cell := range []struct {
		sheet string
		axis  string
		value any
	}{
		{"成绩", "A1", "2026 春期中成绩"},
		{"成绩", "A2", "班级"},
		{"成绩", "B2", "姓名"},
		{"成绩", "C2", "考号"},
		{"成绩", "D2", "语文"},
		{"成绩", "E2", "数学"},
		{"成绩", "A3", "一班"},
		{"成绩", "B3", "甲"},
		{"成绩", "C3", "00123"},
		{"成绩", "D3", 95},
		{"成绩", "E3", 88},
		{"成绩", "A4", "二班"},
		{"成绩", "B4", "乙"},
		{"成绩", "C4", "00124"},
		{"成绩", "D4", 90},
		{"成绩", "E4", 92},
	} {
		if err := workbook.SetCellValue(cell.sheet, cell.axis, cell.value); err != nil {
			t.Fatal(err)
		}
	}
	for _, cell := range []struct {
		axis    string
		formula string
	}{
		{"F3", "D3+E3"},
		{"F4", "D4+E4"},
	} {
		if err := workbook.SetCellFormula("成绩", cell.axis, cell.formula); err != nil {
			t.Fatal(err)
		}
	}
	if err := workbook.MergeCell("成绩", "A1", "F1"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetSheetDimension("成绩", "A1:F4"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "inspect.xlsx")
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInspectWorkbook(t *testing.T) {
	path := createWorkbook(t, testDirectory(t))
	info, err := excel.InspectWorkbook(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Sheets) != 2 {
		t.Fatalf("sheets = %+v", info.Sheets)
	}
	var sheet excel.SheetInfo
	for _, item := range info.Sheets {
		if item.Name == "成绩" {
			sheet = item
		}
	}
	if len(sheet.Merges) != 1 || sheet.Merges[0] != "A1:F1" {
		t.Fatalf("merges = %v", sheet.Merges)
	}
	if sheet.FormulaCount != 2 {
		t.Fatalf("formula count = %d", sheet.FormulaCount)
	}
	if sheet.Values["C3"] != "00123" {
		t.Fatalf("C3 = %q", sheet.Values["C3"])
	}
	if sheet.Formulas["F3"] != "D3+E3" {
		t.Fatalf("F3 formula = %q", sheet.Formulas["F3"])
	}
}

func TestReadSheetDetectsHeader(t *testing.T) {
	path := createWorkbook(t, testDirectory(t))
	data, err := excel.ReadSheet(path, "成绩")
	if err != nil {
		t.Fatal(err)
	}
	if data.HeaderRow != 2 {
		t.Fatalf("header row = %d", data.HeaderRow)
	}
	if len(data.Headers) != 5 || data.Headers[0] != "班级" || data.Headers[4] != "数学" {
		t.Fatalf("headers = %v", data.Headers)
	}
	if len(data.Rows) != 2 {
		t.Fatalf("rows = %v", data.Rows)
	}
	if data.Rows[0][2] != "00123" {
		t.Fatalf("student number = %q", data.Rows[0][2])
	}
}

func TestFindSofficeRejectsExplicitMissingPath(t *testing.T) {
	t.Setenv("TALLY_SOFFICE", filepath.Join(testDirectory(t), "missing-soffice"))
	if _, err := excel.FindSoffice(); err == nil {
		t.Fatal("missing soffice accepted")
	}
}
