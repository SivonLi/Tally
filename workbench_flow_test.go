package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/xuri/excelize/v2"

	"tally/internal/store"
)

func writeImportWorkbook(t *testing.T, path string) {
	t.Helper()
	workbook := excelize.NewFile()
	t.Cleanup(func() {
		if err := workbook.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
	})
	sheets := map[string][][]any{
		"成绩": {
			{"考号", "姓名", "班级", "选科", "总分", "语文", "数学"},
			{"1001", "学生甲", "A", "物理", 180, 90, 90},
			{"1002", "学生乙", "B", "历史", 175, 88, 87},
		},
		"任课教师": {
			{"班级", "班主任", "语文", "数学"},
			{"A", "班主任甲", "教师甲", "教师乙"},
			{"B", "班主任乙", "教师丙", "教师丁"},
		},
		"目标任务": {
			{"班级", "清北", "C9", "985", "211", "特控", "本科"},
			{"A", 1, 1, 1, 1, 1, 1},
			{"B", 1, 1, 1, 1, 1, 1},
		},
		"基数": {
			{"班级", "语均", "数均", "总均"},
			{"A", 90, 90, 180},
			{"B", 88, 87, 175},
			{"年级总计", 89, 88.5, 177.5},
		},
		"分数线": {
			{"科目", "特控线", "本科线", "科目", "特控线", "本科线"},
			{"语文", 90, 60, "语文", 90, 60},
			{"数学", 90, 60, "数学", 90, 60},
			{"总分", 500, 400, "总分", 500, 400},
		},
		"说明": {
			{"备注：546班政治走班和鹭宁班政治合并计算"},
		},
	}
	defaultSheet := workbook.GetSheetName(0)
	for name, rows := range sheets {
		if name != defaultSheet {
			if _, err := workbook.NewSheet(name); err != nil {
				t.Fatal(err)
			}
		}
		for rowIndex, row := range rows {
			cell, err := excelize.CoordinatesToCellName(1, rowIndex+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := workbook.SetSheetRow(name, cell, &row); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := workbook.DeleteSheet(defaultSheet); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFromDataDir(t *testing.T) {
	dir := t.TempDir()
	a := &app{dataDir: dir, language: "en"}
	a.load()
	if a.loadError != "" {
		t.Fatal(a.loadError)
	}
	if a.store == nil {
		t.Fatal("store not opened")
	}
	t.Cleanup(func() {
		if err := a.store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if len(a.plans) != 1 {
		t.Fatalf("plans = %d", len(a.plans))
	}
	if _, err := os.Stat(filepath.Join(dir, "tally.db")); err != nil {
		t.Fatal(err)
	}
}

func TestStartImportWorkbook(t *testing.T) {
	a := newTestApp(t)
	batch := seedBatch(t, a, false)
	path := filepath.Join(a.dataDir, "import.xlsx")
	writeImportWorkbook(t, path)
	a.startImport(path)
	if a.batchError != "" {
		t.Fatal(a.batchError)
	}
	if a.batchBusy {
		t.Fatal("import still busy")
	}
	if a.importSummary == nil {
		t.Fatal("missing import summary")
	}
	if a.importSummary.Students != 8 {
		t.Fatalf("students = %d", a.importSummary.Students)
	}
	a.selectedBatchID = strconv.FormatInt(batch.ID, 10)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Imported 2 classes, 8 students, 4 stat groups.") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestStartCalculateAndExport(t *testing.T) {
	a := newTestApp(t)
	seedBatch(t, a, false)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}
	if a.batchError != "" {
		t.Fatal(a.batchError)
	}
	if a.batchBusy {
		t.Fatal("calculate still busy")
	}
	if len(a.resultRows) != 2 {
		t.Fatalf("result rows = %d", len(a.resultRows))
	}
	if len(a.personalScores) != 2 {
		t.Fatalf("personal = %d", len(a.personalScores))
	}
	tt.Frame()
	for _, text := range []string{"A", "B", "Chinese", "Teacher A"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
	path := filepath.Join(a.dataDir, "export.xlsx")
	a.startExport(path)
	if a.batchError != "" {
		t.Fatal(a.batchError)
	}
	if a.batchBusy {
		t.Fatal("export still busy")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if a.batchMessage != "Exported to "+path {
		t.Fatalf("message = %q", a.batchMessage)
	}
}

func TestBatchWorkflowAtMinimumSize(t *testing.T) {
	a := newTestApp(t)
	seedBatch(t, a, false)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{"Workflow", "Import", "Calculation", "Export", "Import workbook", "Calculate", "Export results"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
	for _, text := range []string{"Calculate", "Export results"} {
		box, ok := tt.Find(text)
		if !ok {
			t.Fatalf("missing box for %q", text)
		}
		if box.X < 0 || box.Y < 0 || box.X+box.W > 1024 || box.Y+box.H > 640 {
			t.Fatalf("%q is outside the window: %+v", text, box)
		}
	}
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}
	if a.batchError != "" {
		t.Fatal(a.batchError)
	}
	if len(a.resultRows) != 2 {
		t.Fatalf("result rows = %d", len(a.resultRows))
	}
}

func TestStartExcludeClass(t *testing.T) {
	a := newTestApp(t)
	seedBatch(t, a, true)
	a.startCalculate()
	if !a.zeroOpen || len(a.zeroIssues) == 0 {
		t.Fatalf("zero dialog = %v issues=%d", a.zeroOpen, len(a.zeroIssues))
	}
	a.zeroReason = "本班不参与"
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Do not participate"); err != nil {
		t.Fatal(err)
	}
	if a.zeroError != "" {
		t.Fatal(a.zeroError)
	}
	if a.zeroOpen {
		t.Fatal("zero dialog still open")
	}
	if len(a.resultRows) == 0 {
		t.Fatal("missing results after exclusion")
	}
}

func TestOpenNewBatchDefaults(t *testing.T) {
	a := newTestApp(t)
	a.openNewBatch()
	if !a.newBatchOpen {
		t.Fatal("dialog not open")
	}
	if a.newBatchYear == "" || a.newBatchSemester == "" || a.newBatchExamType == "" || a.newBatchGrade == "" {
		t.Fatalf("defaults = %q %q %q %q", a.newBatchYear, a.newBatchSemester, a.newBatchExamType, a.newBatchGrade)
	}
}

func TestLabelHelpers(t *testing.T) {
	a := newTestApp(t)
	i18n.SetLocaleFunc(func() string { return "en" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	if got := subjectLabel("chinese"); got != "Chinese" {
		t.Errorf("subjectLabel = %q", got)
	}
	if got := subjectLabel("unknown"); got != "unknown" {
		t.Errorf("subjectLabel unknown = %q", got)
	}
	for index, want := range []string{"Tier 1", "Tier 2", "Tier 3", "Tier 4"} {
		if got := levelLabel(index); got != want {
			t.Errorf("levelLabel(%d) = %q", index, got)
		}
	}
	if got := decodeNames(`["张老师","李老师"]`); got != "张老师、李老师" {
		t.Errorf("decodeNames = %q", got)
	}
	if got := decodeNames("raw"); got != "raw" {
		t.Errorf("decodeNames invalid = %q", got)
	}
	if got := decodeNames("  "); got != "" {
		t.Errorf("decodeNames empty = %q", got)
	}
	if got := a.batchStatusLabel("draft"); got != "Draft" {
		t.Errorf("batchStatusLabel = %q", got)
	}
	if got := a.batchStatusLabel("imported"); got != "Imported" {
		t.Errorf("batchStatusLabel = %q", got)
	}
	if got := a.batchStatusLabel("calculated"); got != "Calculated" {
		t.Errorf("batchStatusLabel = %q", got)
	}
	if got := a.batchStatusLabel("other"); got != "other" {
		t.Errorf("batchStatusLabel = %q", got)
	}
	if got := a.semesterLabel("autumn"); got != "Autumn" {
		t.Errorf("semesterLabel = %q", got)
	}
	if got := a.semesterLabel("other"); got != "other" {
		t.Errorf("semesterLabel = %q", got)
	}
	if got := a.examLabel("final"); got != "Final" {
		t.Errorf("examLabel = %q", got)
	}
	if got := a.examLabel("gaokao"); got != "Gaokao" {
		t.Errorf("examLabel = %q", got)
	}
	if got := a.examLabel("other"); got != "other" {
		t.Errorf("examLabel = %q", got)
	}
	if got := a.gradeLabel("g2"); got != "Grade 11" {
		t.Errorf("gradeLabel = %q", got)
	}
	if got := a.gradeLabel("g3"); got != "Grade 12" {
		t.Errorf("gradeLabel = %q", got)
	}
	if got := a.gradeLabel("other"); got != "other" {
		t.Errorf("gradeLabel = %q", got)
	}
	if got := semesterCode("Autumn"); got != "autumn" {
		t.Errorf("semesterCode = %q", got)
	}
	if got := semesterCode("other"); got != "other" {
		t.Errorf("semesterCode = %q", got)
	}
	if got := examCode("Final"); got != "final" {
		t.Errorf("examCode = %q", got)
	}
	if got := examCode("Gaokao"); got != "gaokao" {
		t.Errorf("examCode = %q", got)
	}
	if got := examCode("other"); got != "other" {
		t.Errorf("examCode = %q", got)
	}
	if got := gradeCode("Grade 11"); got != "g2" {
		t.Errorf("gradeCode = %q", got)
	}
	if got := gradeCode("Grade 12"); got != "g3" {
		t.Errorf("gradeCode = %q", got)
	}
	if got := gradeCode("other"); got != "other" {
		t.Errorf("gradeCode = %q", got)
	}
	if got := a.tierLabel("unknown"); got != "unknown" {
		t.Errorf("tierLabel = %q", got)
	}
	if got := a.kindLabel("system"); got != "Built-in" {
		t.Errorf("kindLabel = %q", got)
	}
	if got := a.kindLabel("user"); got != "Derived" {
		t.Errorf("kindLabel = %q", got)
	}
	if got := a.statusLabel("published"); got != "Published" {
		t.Errorf("statusLabel = %q", got)
	}
	if got := a.statusLabel("retired"); got != "Retired" {
		t.Errorf("statusLabel = %q", got)
	}
	if got := a.statusLabel("other"); got != "other" {
		t.Errorf("statusLabel = %q", got)
	}
}

func TestSelectedValuesAndMissingBatch(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	a.selectedPlanID = "not-a-number"
	if a.selectedPlan() != nil {
		t.Fatal("expected nil plan")
	}
	a.reloadVersions(ctx)
	if a.loadError == "" {
		t.Fatal("expected parse error")
	}
	a.loadError = ""
	a.selectedPlanID = ""
	a.reloadVersions(ctx)
	if a.versions != nil || a.versionIndex != -1 {
		t.Fatalf("versions = %v index = %d", a.versions, a.versionIndex)
	}
	a.versionIndex = 5
	if a.selectedVersion() != nil {
		t.Fatal("expected nil version")
	}
	a.selectedBatchID = ""
	if _, err := a.selectedBatchValue(); err == nil {
		t.Fatal("expected missing batch error")
	}
	if got := a.labelForVersionID(999); got != "" {
		t.Fatalf("label = %q", got)
	}
}

func TestImportFailureKeepsError(t *testing.T) {
	a := newTestApp(t)
	seedBatch(t, a, false)
	a.startImport(filepath.Join(a.dataDir, "missing.xlsx"))
	if a.batchError == "" {
		t.Fatal("expected import error")
	}
	if a.batchBusy {
		t.Fatal("import still busy")
	}
}

func TestCalculateWithoutVersion(t *testing.T) {
	a := newTestApp(t)
	seedBatch(t, a, false)
	a.calculateVersion = "missing"
	a.startCalculate()
	if a.batchError == "" {
		t.Fatal("expected version error")
	}
}

func TestFinishExportError(t *testing.T) {
	a := newTestApp(t)
	a.finishExport("out.xlsx", os.ErrPermission)
	if a.batchError == "" {
		t.Fatal("expected export error")
	}
}

func TestFinishImportError(t *testing.T) {
	a := newTestApp(t)
	a.finishImport(store.ImportSummary{}, os.ErrInvalid)
	if a.batchError == "" {
		t.Fatal("expected import error")
	}
}
