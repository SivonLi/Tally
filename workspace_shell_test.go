package main

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func TestWorkspaceSwitcherEmptyAndSwitchIsolation(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 无工作区时侧栏顶部直接展示「新建工作区」按钮，点击打开新建弹窗。
	if !tt.HasText(locales.WorkspaceCreate()) {
		t.Fatalf("empty state missing %q in %q", locales.WorkspaceCreate(), tt.Texts())
	}
	if err := tt.Click(locales.WorkspaceCreate()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.newCohortOpen {
		t.Fatal("clicking New workspace did not open cohort modal")
	}
	a.newCohortOpen = false
	settleTester(tt)

	// 创建两个工作区（2026级与2025级），各自挂一场不同考试，验证工作区切换严格隔离。
	ctx := context.Background()
	addCohort(t, a, 2026)
	c2026 := a.selectedCohort().ID
	seedStageData(t, a)
	if _, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "2026-Midterm", ExamDate: "2026-11-10", CohortIDs: []int64{c2026},
	}); err != nil {
		t.Fatal(err)
	}

	c2025, err := a.store.CreateCohort(ctx, 2025, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamFinal,
		Name: "2025-Final", ExamDate: "2026-12-20", CohortIDs: []int64{c2025.ID},
	}); err != nil {
		t.Fatal(err)
	}

	a.reloadCohorts(ctx)
	a.switchWorkspaceCohort(c2026)
	if len(a.exams) != 1 || a.exams[0].Name != "2026-Midterm" {
		t.Fatalf("2026 workspace exams = %+v, want only 2026-Midterm", a.exams)
	}
	if len(a.assessments) != 1 || a.assessments[0].ExamName != "2026-Midterm" {
		t.Fatalf("2026 workspace assessments = %+v", a.assessments)
	}

	// 通过侧栏工作区下拉切换到 2025 级。
	settleTester(tt)
	if err := tt.Click(locales.WorkspaceSwitchLabel()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !tt.HasText(a.cohortName(2025)) {
		t.Fatalf("workspace dropdown missing 2025 cohort: %q", tt.Texts())
	}
	if err := tt.Click(a.cohortName(2025)); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.selectedCohort() == nil || a.selectedCohort().ID != c2025.ID {
		t.Fatalf("selected cohort after switch = %+v, want %d", a.selectedCohort(), c2025.ID)
	}
	if len(a.exams) != 1 || a.exams[0].Name != "2025-Final" {
		t.Fatalf("2025 workspace exams = %+v, want only 2025-Final", a.exams)
	}
	if len(a.assessments) != 1 || a.assessments[0].ExamName != "2025-Final" {
		t.Fatalf("2025 workspace assessments = %+v", a.assessments)
	}
}

func TestBaseDataTabsAndImportReturnContext(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 点击侧栏「基础数据」并切换子页签。
	if err := tt.Click(locales.NavBaseData()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageBaseData {
		t.Fatalf("page = %q, want %q", a.page, pageBaseData)
	}
	if err := tt.Click(locales.BaseDataTabStudents()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.baseDataTab != 1 {
		t.Fatalf("baseDataTab = %d, want 1", a.baseDataTab)
	}

	// 从基础数据跳转至数据导入页并携带返回上下文，点击返回条回到原页面与原页签。
	a.openDataImport("students", 0, 0)
	settleTester(tt)
	if a.page != pageImports || !a.importWizardActive() || a.importReturn == nil {
		t.Fatalf("import return not initialized: page=%q active=%v ret=%+v",
			a.page, a.importWizardActive(), a.importReturn)
	}
	retBtn := "Back to " + locales.NavBaseData()
	if err := tt.Click(retBtn); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageBaseData || a.baseDataTab != 1 || a.importWizardActive() || a.importReturn != nil {
		t.Fatalf("returnFromImport state = page:%q tab:%d active:%v ret:%+v",
			a.page, a.baseDataTab, a.importWizardActive(), a.importReturn)
	}
}

func TestUnsavedPlanNavigationGuard(t *testing.T) {
	a := newTestApp(t)
	a.page = pagePlans
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 制造未保存的考核方案改动。
	origDesc := a.planConfig.Description
	a.planConfig.Description = origDesc + " (dirty)"
	tt.Frame()
	if !a.hasUnsavedChanges() {
		t.Fatal("expected hasUnsavedChanges() = true after modifying planConfig")
	}

	// 尝试点击总览，必须弹出未保存保护框而不是直接跳转。
	if err := tt.Click(locales.NavOverview()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.unsavedGuardOpen || a.page != pagePlans {
		t.Fatalf("guard did not intercept navigation: open=%v page=%q", a.unsavedGuardOpen, a.page)
	}

	// 回车触发默认聚焦的「留在当前页」。
	tt.Key(0, ui.KeyEnter)
	settleTester(tt)
	if a.unsavedGuardOpen || a.page != pagePlans {
		t.Fatalf("Stay here failed: open=%v page=%q", a.unsavedGuardOpen, a.page)
	}

	// 再次尝试切换并选择「放弃修改」，恢复草稿并完成跳转。
	if err := tt.Click(locales.NavOverview()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if err := tt.Click(locales.UnsavedGuardDiscard()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageOverview || a.planConfig.Description != origDesc || a.hasUnsavedChanges() {
		t.Fatalf("Discard failed: page=%q desc=%q dirty=%v", a.page, a.planConfig.Description, a.hasUnsavedChanges())
	}
}
