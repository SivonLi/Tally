package exporter_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"tally/internal/assessment"
	"tally/internal/exporter"
	"tally/internal/store"
)

func TestWriteAllStandardSheets(t *testing.T) {
	tempDir := t.TempDir()
	outPath := filepath.Join(tempDir, "test_standard_export.xlsx")

	totalScore1 := 560.5
	totalScore2 := 580.0

	input := exporter.Input{
		Path: outPath,
		Batch: store.Batch{
			AcademicYear: "2024-2025",
			Semester:     "first",
			ExamType:     "midterm",
			Grade:        "g2",
		},
		Plan: store.PlanVersion{
			ID:      1,
			Version: 1,
			Config: assessment.Config{
				Name: "2024秋季高二期中考核方案",
			},
		},
		Run: store.ResultRun{
			ID:        10,
			InputHash: "abcdef123456",
			CreatedAt: time.Now(),
		},
		Rows: []store.StoredResultRow{
			{
				LeadClassName:         "高二(1)班",
				Subject:               "chinese",
				StatGroupID:           101,
				StatGroupName:         "高二(1)班语文",
				TeacherNames:          `["张三"]`,
				ClassCount:            50,
				ClassAverage:          105.5,
				GradeAverage:          102.0,
				BaselineAverage:       100.0,
				BaselineRatio:         1.02,
				ImprovementRaw:        28.5,
				ImprovementScore:      28.5,
				PassCount:             35,
				PassTarget:            30,
				PassRaw:               38.0,
				PassScore:             38.0,
				CompletionActual:      88.0,
				CompletionBase:        80.0,
				CompletionRaw:         22.0,
				CompletionScore:       20.0,
				CompletionParticipate: true,
				Total:                 86.5,
				Rank:                  1,
				TierIndex:             0,
				Points:                10.0,
				IndicatorJSON:         `{"mock": true}`,
			},
			{
				LeadClassName:         "高二(1)班",
				Subject:               "math",
				StatGroupID:           102,
				StatGroupName:         "高二(1)班数学",
				TeacherNames:          `["李四"]`,
				ClassCount:            50,
				ClassAverage:          110.0,
				GradeAverage:          105.0,
				BaselineAverage:       102.0,
				BaselineRatio:         1.01,
				ImprovementRaw:        27.0,
				ImprovementScore:      27.0,
				PassCount:             32,
				PassTarget:            30,
				PassRaw:               36.0,
				PassScore:             36.0,
				CompletionActual:      88.0,
				CompletionBase:        80.0,
				CompletionRaw:         22.0,
				CompletionScore:       20.0,
				CompletionParticipate: true,
				Total:                 83.0,
				Rank:                  2,
				TierIndex:             0,
				Points:                9.5,
				IndicatorJSON:         `{"mock": true}`,
			},
			{
				LeadClassName:         "高二(2)班",
				Subject:               "chinese",
				StatGroupID:           201,
				StatGroupName:         "高二(2)班语文",
				TeacherNames:          `["王五"]`,
				ClassCount:            48,
				ClassAverage:          101.0,
				GradeAverage:          102.0,
				BaselineAverage:       99.0,
				BaselineRatio:         1.00,
				ImprovementRaw:        25.0,
				ImprovementScore:      25.0,
				PassCount:             28,
				PassTarget:            28,
				PassRaw:               34.0,
				PassScore:             34.0,
				CompletionActual:      75.0,
				CompletionBase:        80.0,
				CompletionRaw:         18.5,
				CompletionScore:       18.5,
				CompletionParticipate: true,
				Total:                 77.5,
				Rank:                  3,
				TierIndex:             1,
				Points:                8.0,
				IndicatorJSON:         `{"mock": true}`,
			},
		},
		Personal: []store.StoredPersonalScore{
			{
				TeacherName: "张三",
				Grade:       "g2",
				Subject:     "chinese",
				GroupPoints: `[10.0]`,
				Score:       10.0,
			},
			{
				TeacherName: "李四",
				Grade:       "g2",
				Subject:     "math",
				GroupPoints: `[9.5]`,
				Score:       9.5,
			},
		},
		Classes: []store.Class{
			{ID: 1, Name: "高二(1)班"},
			{ID: 2, Name: "高二(2)班"},
		},
		Students: []store.Student{
			{
				ID:         1001,
				StudentNo:  "20230101",
				Name:       "学生甲",
				ClassID:    1,
				Track:      "physics",
				TotalScore: &totalScore1,
				Scores: map[string]float64{
					"chinese": 110,
					"math":    120,
				},
			},
			{
				ID:         1002,
				StudentNo:  "20230201",
				Name:       "学生乙",
				ClassID:    2,
				Track:      "history",
				TotalScore: &totalScore2,
				Scores: map[string]float64{
					"chinese": 108,
					"math":    115,
				},
			},
		},
		GeneratedAt: time.Now(),
	}

	if err := exporter.Write(input); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	wb, err := excelize.OpenFile(outPath)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer wb.Close()

	expectedSheets := []string{
		"计算依据", "班级汇总", "量化计算", "排名赋分", "个人得分",
		"均分提升率", "达线率", "完成率", "基础数据", "结果明细",
	}

	sheetMap := map[string]bool{}
	for _, name := range wb.GetSheetList() {
		sheetMap[name] = true
	}

	for _, sheet := range expectedSheets {
		if !sheetMap[sheet] {
			t.Errorf("missing expected sheet: %s in %v", sheet, wb.GetSheetList())
		}
	}

	// 检查班级汇总
	classRows, err := wb.GetRows("班级汇总")
	if err != nil {
		t.Fatalf("GetRows(班级汇总) failed: %v", err)
	}
	if len(classRows) < 3 {
		t.Fatalf("expected at least 3 rows in 班级汇总, got %d", len(classRows))
	}
	if classRows[0][0] != "班级" || classRows[0][1] != "完成率得分" {
		t.Errorf("unexpected header in 班级汇总: %v", classRows[0])
	}
	if classRows[1][0] != "高二(1)班" {
		t.Errorf("expected first row class '高二(1)班', got %s", classRows[1][0])
	}

	// 检查均分提升率
	impRows, err := wb.GetRows("均分提升率")
	if err != nil {
		t.Fatalf("GetRows(均分提升率) failed: %v", err)
	}
	if len(impRows) != 4 { // 1 header + 3 rows
		t.Fatalf("expected 4 rows in 均分提升率, got %d", len(impRows))
	}

	// 检查完成率
	compRows, err := wb.GetRows("完成率")
	if err != nil {
		t.Fatalf("GetRows(完成率) failed: %v", err)
	}
	if len(compRows) != 3 { // 1 header + 2 classes
		t.Fatalf("expected 3 rows in 完成率, got %d", len(compRows))
	}

	// 检查基础数据
	rosterRows, err := wb.GetRows("基础数据")
	if err != nil {
		t.Fatalf("GetRows(基础数据) failed: %v", err)
	}
	if len(rosterRows) != 3 { // 1 header + 2 students
		t.Fatalf("expected 3 rows in 基础数据, got %d", len(rosterRows))
	}
	if rosterRows[1][1] != "学生甲" || rosterRows[1][2] != "高二(1)班" {
		t.Errorf("unexpected student row: %v", rosterRows[1])
	}
}

func TestWriteBoundaryOverridesInBasisSheet(t *testing.T) {
	tempDir := t.TempDir()
	baseInput := exporter.Input{
		Batch: store.Batch{AcademicYear: "2024-2025", Semester: "autumn", ExamType: "midterm", Grade: "g2"},
		Plan:  store.PlanVersion{ID: 1, Version: 1, Config: assessment.Config{Name: "测试方案"}},
		Run: store.ResultRun{
			ID:        1,
			InputHash: "hash",
			CreatedAt: time.Now(),
			Boundaries: []store.ResultRunBoundary{
				{Subject: "chinese", GroupCount: 6, FirstRank: 3, SecondRank: 5, Source: "override", Reason: "前3名同分接近"},
				{Subject: "math", GroupCount: 6, FirstRank: 2, SecondRank: 4, Source: "default"},
			},
		},
		GeneratedAt: time.Now(),
	}

	// 有微调时应输出微调清单表
	withOverridePath := filepath.Join(tempDir, "with_override.xlsx")
	baseInput.Path = withOverridePath
	if err := exporter.Write(baseInput); err != nil {
		t.Fatalf("Write with override: %v", err)
	}
	wb1, err := excelize.OpenFile(withOverridePath)
	if err != nil {
		t.Fatal(err)
	}
	defer wb1.Close()
	rows1, err := wb1.GetRows("计算依据")
	if err != nil {
		t.Fatal(err)
	}
	foundOverride := false
	for _, r := range rows1 {
		if len(r) >= 5 && r[0] == "语文" && r[1] == "6" && r[2] == "3" && r[3] == "5" && r[4] == "前3名同分接近" {
			foundOverride = true
		}
	}
	if !foundOverride {
		t.Fatalf("expected chinese override row in 计算依据 sheet, got %+v", rows1)
	}

	// 无微调时应写明按方案默认边界
	defaultPath := filepath.Join(tempDir, "default_only.xlsx")
	baseInput.Path = defaultPath
	baseInput.Run.Boundaries = []store.ResultRunBoundary{
		{Subject: "math", GroupCount: 6, FirstRank: 2, SecondRank: 4, Source: "default"},
	}
	if err := exporter.Write(baseInput); err != nil {
		t.Fatalf("Write default only: %v", err)
	}
	wb2, err := excelize.OpenFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	defer wb2.Close()
	rows2, err := wb2.GetRows("计算依据")
	if err != nil {
		t.Fatal(err)
	}
	foundDefault := false
	for _, r := range rows2 {
		if len(r) >= 2 && r[0] == "分档边界" && r[1] == "按方案默认边界" {
			foundDefault = true
		}
	}
	if !foundDefault {
		t.Fatalf("expected default boundary notice in 计算依据 sheet, got %+v", rows2)
	}
}
