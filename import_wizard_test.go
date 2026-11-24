package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/xuri/excelize/v2"

	"tally/internal/importer"
	"tally/internal/store"
)

func guidedImportWorkbook(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", "学生名单"); err != nil {
		t.Fatal(err)
	}
	students := [][]any{
		{"学号", "姓名", "行政班", "选科组合", "状态", "入口总分"},
		{"0012", "甲", "1班", "物理、化学、生物", "在读", "620.5"},
		{"0013", "乙", "2班", "历史、政治、地理", "转学", "590"},
	}
	for index, row := range students {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetSheetRow("学生名单", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := workbook.NewSheet("任课关系"); err != nil {
		t.Fatal(err)
	}
	assignments := [][]any{
		{"教师姓名", "工号", "行政班", "学科"},
		{"李老师", "T01", "1班", "数学"},
	}
	for index, row := range assignments {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetSheetRow("任课关系", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := workbook.NewSheet("入口成绩"); err != nil {
		t.Fatal(err)
	}
	baselines := [][]any{
		{"行政班", "语文", "数学", "语文比值", "数学比值"},
		{"1班", "102.5", "98", "1.05", "1.1"},
	}
	for index, row := range baselines {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetSheetRow("入口成绩", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "guided.xlsx")
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGuidedImportWizardRunsFromFileToResult(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	a.openImportWizard(importer.KindStudents)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{"Guided import", "Choose a workbook"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	a.loadWizardFile(a.importWizard, guidedImportWorkbook(t))
	tt.Frame()
	if !tt.HasText("学生名单") || !tt.HasText("2 rows · 6 columns") {
		t.Fatalf("sheet step texts = %q", tt.Texts())
	}
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"Student number", "Class", "0012"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Check the data"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("New 2") || !tt.HasText("Blocking 0") {
		t.Fatalf("validation texts = %q", tt.Texts())
	}
	if err := tt.Click("I have reviewed the warnings (1)."); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Import 2 records"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Import finished") || !tt.HasText("Imported 2 students in 2 classes.") {
		t.Fatalf("result texts = %q", tt.Texts())
	}
	_, total, err := a.store.ListStudents(context.Background(), store.StudentFilter{
		StageID: a.selectedStage().ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("students = %d", total)
	}
	if err := tt.Click("Continue with Teaching assignments"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.importWizard.Kind != importer.KindAssignments || a.importWizard.Workbook == nil ||
		a.importWizard.Sheet != "任课关系" || a.importWizard.Step != wizardStepSheet {
		t.Fatalf("wizard after continue = %+v", a.importWizard)
	}
	if !tt.HasText("任课关系") || !tt.HasText("Teaching assignments") {
		t.Fatalf("continued wizard texts = %q", tt.Texts())
	}
}

// manySheetsWorkbook 生成 count 张工作表的工作簿，第一张是学生名单。
func manySheetsWorkbook(t *testing.T, count int) string {
	t.Helper()
	dir := t.TempDir()
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetRow("Sheet1", "A1", &[]any{"学号", "姓名", "行政班"}); err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= count; i++ {
		if _, err := workbook.NewSheet(fmt.Sprintf("S%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "many-sheets.xlsx")
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

// 工作表很多时，下拉只露出一部分，其余靠滚动到达并能选中。
func TestGuidedImportSheetDropdownScrollsLongList(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	a.openImportWizard(importer.KindStudents)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	a.loadWizardFile(a.importWizard, manySheetsWorkbook(t, 24))
	tt.Frame()
	first := a.importWizard.Sheet
	if first == "" {
		t.Fatal("no sheet auto selected")
	}
	if err := tt.Click(first); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	last := "S24"
	lastBox, ok := tt.Find(last)
	if !ok {
		t.Fatalf("dropdown missing the last option: %q", tt.Texts())
	}
	if lastBox.W > 0 && lastBox.H > 0 {
		t.Fatal("the last sheet shows without scrolling")
	}
	box, ok := tt.Find(first)
	if !ok {
		t.Fatalf("dropdown option missing: %q", tt.Texts())
	}
	t.Log("trigger:", box)
	s02, _ := tt.Find("S02")
	t.Log("S02 before:", s02)
	t.Log("S24 before:", lastBox)
	tt.Scroll(box.X+box.W/2, box.Y+40, 0, 800)
	tt.Frame()
	s02, _ = tt.Find("S02")
	s24, _ := tt.Find(last)
	t.Log("S02 after:", s02)
	t.Log("S24 after:", s24)
	if lastBox, ok = tt.Find(last); !ok || lastBox.W <= 0 || lastBox.H <= 0 {
		t.Fatalf("last sheet still hidden after scroll: %+v %v", lastBox, ok)
	}
	if err := tt.Click(last); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.importWizard.Sheet != last {
		t.Fatalf("sheet = %q", a.importWizard.Sheet)
	}
}

func TestGuidedImportWizardBackFromValidationClosesWizard(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	a.openImportWizard(importer.KindStudents)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	a.loadWizardFile(a.importWizard, guidedImportWorkbook(t))
	tt.Frame()
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Check the data"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.importWizard.Step != wizardStepValidate {
		t.Fatalf("step = %d", a.importWizard.Step)
	}
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.importWizard != nil {
		t.Fatal("wizard is still open after Back")
	}
	if !tt.HasText("Overview") {
		t.Fatalf("texts after back = %q", tt.Texts())
	}
}

func TestGuidedImportMappingShowsSubjectNames(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a.openImportWizard(importer.KindBaselines)
	a.loadWizardFile(a.importWizard, guidedImportWorkbook(t))
	a.importWizard.Step = wizardStepMapping
	tt.Frame()
	for _, text := range []string{"语文", "语文比值", "数学", "数学比值", "102.5"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestStageReviewBannerShowsChanges(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.setAcademicYear(2027)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Entered 2027-2028 academic year") || !tt.HasText("View changes") {
		t.Fatalf("banner texts = %q", tt.Texts())
	}
	if err := tt.Click("View changes"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"Copied from Grade 10 · 2026-2027", "Open preparation list"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Open preparation list"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.prepOpen || a.stageReviewOpen {
		t.Fatalf("prepOpen = %v, stageReviewOpen = %v", a.prepOpen, a.stageReviewOpen)
	}
	if !tt.HasText("Prepare cohort data") {
		t.Fatalf("prep texts = %q", tt.Texts())
	}
}

func TestStageReviewBannerConfirmsStage(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.setAcademicYear(2027)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Confirm review"); err != nil {
		t.Fatal(err)
	}
	stage := a.selectedStage()
	if stage == nil || stage.ReviewStatus != "confirmed" || stage.ConfirmedAt == nil {
		t.Fatalf("stage = %+v", stage)
	}
	tt.Frame()
	if tt.HasText("View changes") {
		t.Fatalf("banner still visible: %q", tt.Texts())
	}
}

func TestStageReviewLaterDismissesBanner(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.setAcademicYear(2027)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Review later"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("View changes") {
		t.Fatalf("banner still visible: %q", tt.Texts())
	}
	stage := a.selectedStage()
	if stage == nil || stage.ReviewStatus != "pending" {
		t.Fatalf("stage = %+v", stage)
	}
}

func TestOverviewContinueOpensPreparation(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	a.prepOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if !a.prepOpen || !tt.HasText("Prepare cohort data") {
		t.Fatalf("prepOpen = %v, texts = %q", a.prepOpen, tt.Texts())
	}
	if err := tt.Click("Import Class structure"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.importWizard == nil || a.importWizard.Kind != importer.KindClasses {
		t.Fatalf("wizard = %+v", a.importWizard)
	}
}

func TestGuidedImportSheetCanBeChanged(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	a.openImportWizard(importer.KindStudents)
	a.loadWizardFile(a.importWizard, guidedImportWorkbook(t))
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if a.importWizard.Sheet != "学生名单" {
		t.Fatalf("sheet = %q", a.importWizard.Sheet)
	}
	if err := tt.Click("学生名单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("任课关系"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.importWizard.Sheet != "任课关系" {
		t.Fatalf("sheet after change = %q", a.importWizard.Sheet)
	}
}

func TestImportRecordDetailAndUndo(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.page = pageImports
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Details Entry scores"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Import record") || !tt.HasText("Undo this import") {
		t.Fatalf("detail texts = %q", tt.Texts())
	}
	if err := tt.Click("Undo this import"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("The import was undone.") {
		t.Fatalf("undo texts = %q", tt.Texts())
	}
	counts, err := a.store.StageCounts(context.Background(), a.selectedCohort().ID,
		a.selectedStage().AcademicYearStart, a.selectedStage().GradeLevel)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Baselines != 0 {
		t.Fatalf("baselines after undo = %d", counts.Baselines)
	}
}

func TestHistoricalStageIsReadOnly(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.setAcademicYear(2027)
	for _, stage := range a.cohortStages {
		if stage.GradeLevel == "g1" {
			a.selectedStageID = stage.ID
			break
		}
	}
	a.reloadSelectedCohort(context.Background())
	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("This is a historical stage. Its data is read-only.") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Import students"); err != nil {
		t.Fatal(err)
	}
	if a.importWizard != nil {
		t.Fatal("historical stage opened the import wizard")
	}
	for _, stage := range a.cohortStages {
		if stage.GradeLevel == "g2" {
			a.selectedStageID = stage.ID
			break
		}
	}
	a.reloadSelectedCohort(context.Background())
	tt.Frame()
	if err := tt.Click("Import students"); err != nil {
		t.Fatal(err)
	}
	if a.importWizard == nil || a.importWizard.Kind != importer.KindStudents {
		t.Fatalf("wizard = %+v", a.importWizard)
	}
}

func TestCohortStageHistoryShowsReviewState(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.setAcademicYear(2027)
	a.page = pageCohorts
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Needs review") || !tt.HasText("Confirmed") {
		t.Fatalf("stage badges = %q", tt.Texts())
	}
}

func TestGuidedImportRequiresWarningConfirmation(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	a.openImportWizard(importer.KindAssignments)
	a.loadWizardFile(a.importWizard, guidedImportWorkbook(t))
	a.wizardValidate()
	if a.importWizard.Preview.Warnings == 0 || a.importWizard.Preview.Blocking != 0 {
		t.Fatalf("preview = %+v", a.importWizard.Preview)
	}
	if a.importWizard.ConfirmedWarnings {
		t.Fatal("warnings confirmed before the user reviewed them")
	}
	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("en")
	tt.Frame()
	label := "I have reviewed the warnings (1)."
	box, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing warning confirmation in %q", tt.Texts())
	}
	if box.X < 0 || box.Y < 0 || box.X+box.W > 1024 || box.Y+box.H > 640 {
		t.Fatalf("warning confirmation is outside the window: %+v", box)
	}
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.importWizard.ConfirmedWarnings {
		t.Fatal("warning confirmation did not toggle")
	}
	if err := tt.Click("Import 1 records"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.importWizard.Summary == nil {
		t.Fatalf("summary = %+v, error = %q", a.importWizard.Summary, a.importWizard.Error)
	}
}
