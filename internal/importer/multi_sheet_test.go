package importer

import (
	"testing"

	"tally/internal/excel"
	"tally/internal/store"
)

func TestClassifyWorkbookSheets(t *testing.T) {
	subjects := []store.Subject{
		{Key: "chinese", Name: "语文", FullMark: 150, Enabled: true},
		{Key: "math", Name: "数学", FullMark: 150, Enabled: true},
		{Key: "english", Name: "英语", FullMark: 150, Enabled: true},
	}
	tiers := []string{"qingbei", "undergraduate"}
	makeRows := func(n int, vals []string) []excel.TableRow {
		rows := make([]excel.TableRow, n)
		for i := range rows {
			rows[i] = excel.TableRow{Number: i + 2, Values: vals}
		}
		return rows
	}

	wb := &Workbook{
		Path: "midterm.xlsx",
		Tables: []excel.Table{
			{
				Sheet:   "总成绩",
				Headers: []string{"考号", "姓名", "班级", "语文", "数学", "英语"},
				Rows:    makeRows(15, []string{"2026001", "甲", "1班", "120", "130", "125"}),
			},
			{
				Sheet:   "分数线",
				Headers: []string{"科目", "类别", "本科线"},
				Rows:    makeRows(3, []string{"语文", "物理类", "95"}),
			},
			{
				Sheet:   "目标任务",
				Headers: []string{"班级", "清北", "本科"},
				Rows:    makeRows(2, []string{"1班", "2", "30"}),
			},
			{
				Sheet:   "说明",
				Headers: []string{"备注"},
				Rows:    makeRows(1, []string{"测试备注"}),
			},
		},
	}

	classified := ClassifyWorkbookSheets(wb, "exam", subjects, tiers, 5)
	if len(classified) != 4 {
		t.Fatalf("expected 4 sheets, got %d", len(classified))
	}
	if classified[0].Kind != KindScores || !classified[0].Recommended || len(classified[0].PreviewRows) != 5 {
		t.Fatalf("unexpected scores sheet classification: %+v", classified[0])
	}
	if len(classified[0].MissingRequired) != 0 {
		t.Fatalf("expected 0 missing required for scores, got %v", classified[0].MissingRequired)
	}
	if classified[1].Kind != KindCutoffs || !classified[1].Recommended {
		t.Fatalf("unexpected cutoffs sheet classification: %+v", classified[1])
	}
	if classified[2].Kind != KindTargets || !classified[2].Recommended {
		t.Fatalf("unexpected targets sheet classification: %+v", classified[2])
	}
	if classified[3].Kind != "" || classified[3].Recommended {
		t.Fatalf("expected unknown sheet unclassified, got %+v", classified[3])
	}
}
