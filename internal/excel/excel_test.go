package excel_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xuri/excelize/v2"

	"tally/internal/excel"
)

func TestCreateStyledWorkbook(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), "..", "..", ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "excel-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})

	f := excel.New()
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
	})

	if _, err := f.NewSheet("成绩"); err != nil {
		t.Fatal(err)
	}
	for _, cell := range []struct {
		axis  string
		value any
	}{
		{"A1", "姓名"},
		{"B1", "分数"},
		{"A2", "张三"},
		{"B2", 95},
	} {
		if err := f.SetCellValue("成绩", cell.axis, cell.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.MergeCell("成绩", "D1", "E1"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("成绩", "A", "B", 14); err != nil {
		t.Fatal(err)
	}
	if err := f.SetRowHeight("成绩", 1, 24); err != nil {
		t.Fatal(err)
	}
	if err := f.SetPanes("成绩", &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2"}); err != nil {
		t.Fatal(err)
	}
	if err := f.AutoFilter("成绩", "A1:B2", nil); err != nil {
		t.Fatal(err)
	}
	style, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Family: "宋体", Size: 12, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"4472C4"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    []excelize.Border{{Type: "left", Color: "000000", Style: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("成绩", "A1", "B2", style); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "workbook.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	reopened, err := excel.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("close reopened workbook: %v", err)
		}
	})

	sheets := reopened.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("sheets = %v", sheets)
	}
	got := map[string]bool{}
	for _, name := range sheets {
		got[name] = true
	}
	if !got["Sheet1"] || !got["成绩"] {
		t.Fatalf("sheets = %v", sheets)
	}
	value, err := reopened.GetCellValue("成绩", "A2")
	if err != nil {
		t.Fatal(err)
	}
	if value != "张三" {
		t.Fatalf("A2 = %q", value)
	}
	merges, err := reopened.GetMergeCells("成绩")
	if err != nil {
		t.Fatal(err)
	}
	if len(merges) != 1 || merges[0].GetStartAxis() != "D1" || merges[0].GetEndAxis() != "E1" {
		t.Fatalf("merges = %v", merges)
	}
	styleID, err := reopened.GetCellStyle("成绩", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if styleID == 0 {
		t.Fatal("style was not preserved")
	}
}
