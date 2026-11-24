package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/importer"
	"tally/internal/store"
	"tally/locales"
)

func TestImportCenterMultiSheetDetectAndChecklist(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageImports
	a.importCenterTab = 0

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 1. 拖入/加载多工作表工作簿后，自动识别各工作表类型与必填列状态，并支持一键进入列映射。
	a.loadImportCenterWorkbook(guidedImportWorkbook(t))
	settleTester(tt)
	if len(a.importCenterSheets) < 2 {
		t.Fatalf("expected multi-sheet classifications, got %+v", a.importCenterSheets)
	}
	if !tt.HasText("学生名单") || !tt.HasText(locales.ImportCenterSheetReady()) {
		t.Fatalf("missing detected sheet info in %q", tt.Texts())
	}
	if err := tt.Click(locales.ImportCenterGoImport() + " 学生名单"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.importWizard == nil || a.importWizard.Kind != importer.KindStudents ||
		a.importWizard.Step != wizardStepMapping || a.importWizard.Sheet != "学生名单" {
		t.Fatalf("wizard after detected sheet click = %+v", a.importWizard)
	}
	a.closeImportWizard()
	settleTester(tt)

	// 2. 切换到「准备清单」页签，展示工作区准备清单并支持点击去导入。
	if err := tt.Click(locales.ImportCenterTabGuide()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.importCenterTab != 1 || !tt.HasText(locales.ImportCenterChecklistTitle()) {
		t.Fatalf("guide tab not rendered: tab=%d texts=%q", a.importCenterTab, tt.Texts())
	}
	if err := tt.Click(locales.ImportCenterGoImport() + " " + locales.PrepStudents()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.importWizard == nil || a.importWizard.Kind != importer.KindStudents {
		t.Fatalf("guide tab go-import wizard = %+v", a.importWizard)
	}
}

func TestImportCenterExamImportUndoAndReturnToOrigin(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	ctx := context.Background()
	cohort := a.selectedCohort()

	// 1. 从「学生」页点击导入按钮 -> 跳转到独立「数据导入」页并携带返回上下文 -> 导入完成后点击「完成并返回」回到学生页。
	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	if err := tt.Click(locales.ImportStudents()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageImports || !a.importWizardActive() || a.importReturn == nil || a.importReturn.Page != pageStudents {
		t.Fatalf("Import students did not jump with return context: page=%q ret=%+v", a.page, a.importReturn)
	}
	a.loadWizardFile(a.importWizard, guidedImportWorkbook(t))
	a.wizardValidate()
	a.wizardApply()
	settleTester(tt)
	if a.importWizard == nil || a.importWizard.Step != wizardStepResult {
		t.Fatalf("wizard did not complete: %+v", a.importWizard)
	}
	doneRet := fmt.Sprintf(locales.ImportCenterDoneReturn(), locales.NavStudents())
	if err := tt.Click(doneRet); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageStudents || a.importWizardActive() || a.importReturn != nil {
		t.Fatalf("Done & return did not restore students page: page=%q active=%v ret=%+v",
			a.page, a.importWizardActive(), a.importReturn)
	}

	// 2. 创建考试并导入分数线 -> 在「数据导入 -> 导入记录」查看详情并安全确认撤销该考试类导入。
	exam, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "期中考试", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportCutoffs,
		Data: store.ImportData{
			SubjectCutoffs: []store.ImportSubjectCutoff{{Subject: "chinese", Track: "physics", Value: 98}},
		},
		Report: store.ImportReport{
			FileHash: "exam-cutoff-undo-ui-1", Path: "cutoffs.xlsx", RowCount: 1,
			Sheets: []store.ImportSheet{{Name: "分数线", Role: "cutoffs", HeaderRow: 1, Rows: 1}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	a.reloadSelectedCohort(ctx)
	a.page = pageImports
	a.importCenterTab = 2
	settleTester(tt)

	if !tt.HasText("cutoffs.xlsx") {
		t.Fatalf("workspace import history missing exam import cutoffs.xlsx in %q", tt.Texts())
	}
	if err := tt.Click("Details " + locales.ExamKindCutoffs()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.importDetail == nil || a.importDetail.Run.ScopeType != "exam" {
		t.Fatalf("expected exam import detail, got %+v", a.importDetail)
	}
	if err := tt.Click(locales.ImportUndo()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.importUndoConfirmOpen {
		t.Fatal("undo confirmation dialog did not open for exam import")
	}
	if err := tt.Click(locales.ImportCenterUndoConfirmAction()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	cutoffs, err := a.store.ListExamSubjectCutoffs(ctx, exam.ID, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cutoffs) != 0 {
		t.Fatalf("expected exam subject cutoffs rolled back to 0, got %+v", cutoffs)
	}
}

func TestImportCenterDualSnapshots(t *testing.T) {
	useChineseUI(t)
	// Empty state (no workspace yet)
	aEmpty := newTestApp(t)
	aEmpty.page = pageImports
	ttEmpty := newChineseTester(t, aEmpty.view, 1280, 800)
	captureDualSnapshot(t, ttEmpty, "view_importcenter_empty")

	// Populated state (workspace with multi-sheet workbook detected)
	aPop := newPopulatedTestApp(t)
	aPop.page = pageImports
	aPop.importCenterTab = 0
	aPop.loadImportCenterWorkbook(guidedImportWorkbook(t))
	ttPop := newChineseTester(t, aPop.view, 1280, 800)
	captureDualSnapshot(t, ttPop, "view_importcenter_populated")
}
