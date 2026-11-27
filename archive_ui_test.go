package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/internal/importer"
	"tally/internal/store"
)

func setFixedLocale(t *testing.T, tt *ui.Tester, tag string) {
	t.Helper()
	t.Setenv("MYGO_LOCALE", tag)
	i18n.SetLocaleFunc(func() string { return tag })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	if tt != nil {
		tt.SetLocale(tag)
	}
}

func TestArchiveImportEndToEndUIAndReadOnlyWorkbench(t *testing.T) {
	a := newTestApp(t)
	setFixedLocale(t, nil, "zh-Hans")
	addCohort(t, a, 2025)
	ctx := context.Background()
	cohort := a.selectedCohort()

	exam, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2025,
		Term:              store.ExamTermFirst,
		ExamType:          store.ExamMidterm,
		Name:              "高一上期中归档测试",
		ExamDate:          "2026-04-20",
		CohortIDs:         []int64{cohort.ID},
	})
	if err != nil {
		t.Fatalf("CreateExam: %v", err)
	}
	a.reloadExams(ctx)
	a.openExamForCohort(exam.ID, cohort.ID)
	a.reloadExamData(ctx)

	tt := ui.NewTester(a.view, 1280, 800)
	setFixedLocale(t, tt, "zh-Hans")
	settleTester(tt)

	// 1. 考试详情页应展示「导入归档结果」入口，点击后进入结果归档向导。
	if !tt.HasText("导入归档结果") {
		t.Fatalf("expected 导入归档结果 button on exam detail page, texts=%q", tt.Texts())
	}
	if err := tt.Click("导入归档结果"); err != nil {
		t.Fatalf("click 导入归档结果: %v", err)
	}
	settleTester(tt)
	if !a.importWizardActive() || a.importWizard.Kind != importer.KindArchivedResults {
		t.Fatalf("expected active archive import wizard, got %+v", a.importWizard)
	}

	// 快照：未导入文件时的归档向导空态（浅色/深色）。
	captureDualSnapshot(t, tt, "archive_import_empty")
	settleTester(tt)

	// 2. 先测试含阻断问题的工作簿：预览页展示问题且写入按钮禁用。
	issuesXlsx := filepath.Join("internal", "importer", "testdata", "archive_issues.xlsx")
	a.loadWizardFile(a.importWizard, issuesXlsx)
	settleTester(tt)
	if a.importWizard.Step != wizardStepValidate {
		t.Fatalf("expected validate step after loading archive workbook, got step=%d err=%q", a.importWizard.Step, a.importWizard.Error)
	}
	if a.importWizard.Preview.Blocking == 0 {
		t.Fatalf("expected blocking issues for archive_issues.xlsx, got preview=%+v", a.importWizard.Preview)
	}

	// 3. 加载规范合成脱敏工作簿：展示三张识别表（量化计算/排名赋分/个人得分）与结果行数。
	cleanXlsx := filepath.Join("internal", "importer", "testdata", "archive_long_standard.xlsx")
	a.loadWizardFile(a.importWizard, cleanXlsx)
	settleTester(tt)
	if a.importWizard.Error != "" {
		t.Fatalf("unexpected wizard error: %s", a.importWizard.Error)
	}
	if a.importWizard.Preview.Blocking > 0 {
		t.Fatalf("expected 0 blocking issues, got %+v", a.importWizard.Issues)
	}
	for _, want := range []string{"归档工作簿识别与预览", "3 张结果表", "3 条班级学科结果", "2 条个人得分", "量化计算", "排名赋分", "个人得分", "新增 5", "导入 5 条"} {
		if !tt.HasText(want) {
			t.Fatalf("missing archive preview text %q in %q", want, tt.Texts())
		}
	}

	// 快照：结果归档预览态（浅色/深色）。
	captureDualSnapshot(t, tt, "archive_import_preview")
	tt.SetDark(false)
	settleTester(tt)

	// 4. 如有提醒，先确认提醒并点击导入写入 ApplyArchivedResultImport。
	if a.importWizard.Preview.Warnings > 0 {
		a.importWizard.ConfirmedWarnings = true
		tt.Frame()
	}
	a.wizardApply()
	settleTester(tt)
	if a.importWizard.Error != "" {
		t.Fatalf("wizardApply error: %s", a.importWizard.Error)
	}
	if a.importWizard.Step != wizardStepResult || a.importWizard.ArchiveSummary == nil {
		t.Fatalf("expected result step with ArchiveSummary, got step=%d summary=%+v", a.importWizard.Step, a.importWizard.ArchiveSummary)
	}
	if !tt.HasText("已归档写入 3 条班级学科量化结果和 2 条教师个人得分") {
		t.Fatalf("missing archive result summary text in %q", tt.Texts())
	}

	// 5. 返回考试详情：应显示归档只读徽章与只读说明。
	a.returnFromImport()
	settleTester(tt)
	for _, want := range []string{"结果归档（只读）", "该考核以结果归档方式导入，仅作只读保存，不能试算或重新计算。", "查看量化结果"} {
		if !tt.HasText(want) {
			t.Fatalf("missing archived exam detail text %q in %q", want, tt.Texts())
		}
	}
	captureDualSnapshot(t, tt, "exam_detail_archived")
	tt.SetDark(false)
	settleTester(tt)

	// 6. 进入本次考核工作台：显示结果与「结果归档（只读）」，试算/正式计算应拒绝并给出明确中文提示。
	if err := tt.Click("查看量化结果"); err != nil {
		t.Fatalf("click 查看量化结果: %v", err)
	}
	settleTester(tt)
	if a.page != pageAssessments {
		t.Fatalf("expected pageAssessments, got %q", a.page)
	}
	if !tt.HasText("结果归档（只读）") {
		t.Fatalf("missing archived badge on workbench: %q", tt.Texts())
	}
	if a.assessmentOfficialStale {
		t.Fatalf("archived assessment should never be marked stale")
	}
	captureDualSnapshot(t, tt, "workbench_archived")
	tt.SetDark(false)
	settleTester(tt)

	// 验证点击归档结果行可以打开详情弹窗且无报错。
	if err := tt.Click("语文"); err != nil {
		t.Fatalf("click archived row 语文: %v", err)
	}
	settleTester(tt)
	if !a.resultDetailOpen {
		t.Fatalf("expected archived row click to open detail modal")
	}
	a.closeResultDetail()
	settleTester(tt)

	// 点击试算：调用 AssertAssessmentCalculable 并提示只读原因
	if err := tt.Click("开始试算"); err != nil {
		t.Fatalf("click 开始试算: %v", err)
	}
	settleTester(tt)
	if !strings.Contains(a.assessmentError, store.ErrAssessmentArchived.Error()) {
		t.Fatalf("expected archived error after trial click, got %q", a.assessmentError)
	}
	if !tt.HasText(store.ErrAssessmentArchived.Error()) {
		t.Fatalf("expected error notice rendered in UI: %q", tt.Texts())
	}

	// 清除提示后点击正式计算：同样拒绝
	a.assessmentError = ""
	tt.Frame()
	if err := tt.Click("正式计算"); err != nil {
		t.Fatalf("click 正式计算: %v", err)
	}
	settleTester(tt)
	if !strings.Contains(a.assessmentError, store.ErrAssessmentArchived.Error()) {
		t.Fatalf("expected archived error after calculate click, got %q", a.assessmentError)
	}

	// 7. 验证导入中心「本次考试数据」同样包含结果归档入口（含英文界面）。
	a.page = pageImports
	a.importCenterScope = 1
	a.importCenterExamID = exam.ID
	setFixedLocale(t, tt, "en")
	tt.Frame()
	if !tt.HasText("Result archive") {
		t.Fatalf("expected Result archive item in import center (en), got %q", tt.Texts())
	}
}

func TestArchivePreviewDoesNotCreateAssessmentAndLabelsLocalized(t *testing.T) {
	a := newTestApp(t)
	setFixedLocale(t, nil, "zh-Hans")
	addCohort(t, a, 2025)
	ctx := context.Background()
	cohort := a.selectedCohort()

	monthly, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2025,
		Term:              store.ExamTermFirst,
		ExamType:          store.ExamMonthly,
		Name:              "高一月考归档预览",
		ExamDate:          "2026-03-15",
		CohortIDs:         []int64{cohort.ID},
	})
	if err != nil {
		t.Fatalf("CreateExam: %v", err)
	}
	ec, err := a.store.GetExamCohort(ctx, monthly.ID, cohort.ID)
	if err != nil {
		t.Fatalf("GetExamCohort: %v", err)
	}
	if _, ok, findErr := a.store.FindAssessmentForExamCohort(ctx, ec.ID); findErr != nil || ok {
		t.Fatalf("expected no assessment row initially for monthly exam, got (%v, %v)", ok, findErr)
	}

	a.openExamImportWizard(importer.KindArchivedResults, monthly.ID, cohort.ID)
	cleanXlsx := filepath.Join("internal", "importer", "testdata", "archive_long_standard.xlsx")
	a.loadWizardFile(a.importWizard, cleanXlsx)
	if a.importWizard.Error != "" {
		t.Fatalf("loadWizardFile failed: %s", a.importWizard.Error)
	}
	if _, ok, findErr := a.store.FindAssessmentForExamCohort(ctx, ec.ID); findErr != nil || ok {
		t.Fatalf("previewing archive workbook should not create assessment row, got (%v, %v)", ok, findErr)
	}

	cols := []string{
		"class_count", "class_average", "grade_average", "baseline_average", "baseline_ratio",
		"improvement_raw", "improvement_score", "pass_count", "pass_target", "pass_raw", "pass_score",
		"completion_score", "quant_total_score", "stat_group", "rank", "points", "tier",
		"personal_score", "group_points",
	}
	for _, loc := range []string{"zh-Hans", "en"} {
		setFixedLocale(t, nil, loc)
		for _, col := range cols {
			label := a.importFieldLabel(col)
			if label == "" || label == col {
				t.Fatalf("locale %s: column %q not localized (got %q)", loc, col, label)
			}
		}
	}
}
