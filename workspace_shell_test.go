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
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 无年级时总览展示创建年级空状态按钮，点击打开新建弹窗。
	if !tt.HasText(locales.CohortCreate()) {
		t.Fatalf("empty state missing %q in %q", locales.CohortCreate(), tt.Texts())
	}
	if err := tt.Click(locales.CohortCreate()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.newCohortOpen {
		t.Fatal("clicking Create cohort did not open cohort modal")
	}
	a.newCohortOpen = false
	settleTester(tt)

	// 创建两个年级（2026级与2025级），验证全校统一导航下各自档案详情切换与总览并存展示。
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
	a.reloadExams(ctx)
	a.reloadAssessments(ctx)
	settleTester(tt)
	if !tt.HasText(a.cohortName(2026)) || !tt.HasText(a.cohortName(2025)) {
		t.Fatalf("overview should show both active cohorts: %q", tt.Texts())
	}
	if len(a.exams) != 2 || len(a.assessments) != 2 {
		t.Fatalf("school-wide exams/assessments should include both cohorts: exams=%d assess=%d",
			len(a.exams), len(a.assessments))
	}

	// 分别进入 2026 级与 2025 级详情，验证两级考试与学生数据隔离。
	a.openCohortDetail(c2026)
	if len(a.cohortExams) != 1 || a.cohortExams[0].Name != "2026-Midterm" {
		t.Fatalf("2026 cohortExams = %+v", a.cohortExams)
	}
	a.openCohortDetail(c2025.ID)
	if len(a.cohortExams) != 1 || a.cohortExams[0].Name != "2025-Final" {
		t.Fatalf("2025 cohortExams = %+v", a.cohortExams)
	}
}

func TestBaseDataTabsAndImportReturnContext(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	cID := a.selectedCohort().ID

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 点击侧栏「年级档案」并进入 2026 级详情，切换「学生档案」页签。
	if err := tt.Click(locales.NavCohorts()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageCohorts {
		t.Fatalf("page = %q, want %q", a.page, pageCohorts)
	}
	if err := tt.Click(a.cohortName(2026)); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if err := tt.Click(locales.CohortTabStudents()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.cohortDetailTab != 2 {
		t.Fatalf("cohortDetailTab = %d, want 2", a.cohortDetailTab)
	}

	// 从年级详情跳转至数据导入页并携带返回上下文，点击返回条回到原页面与原页签。
	a.openDataImport("students", 0, cID)
	settleTester(tt)
	if a.page != pageImports || !a.importWizardActive() || a.importReturn == nil {
		t.Fatalf("import return not initialized: page=%q active=%v ret=%+v",
			a.page, a.importWizardActive(), a.importReturn)
	}
	retBtn := "Back to " + locales.NavCohorts()
	if err := tt.Click(retBtn); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageCohorts || a.cohortDetailTab != 2 || a.importWizardActive() || a.importReturn != nil {
		t.Fatalf("returnFromImport state = page:%q tab:%d active:%v ret:%+v",
			a.page, a.cohortDetailTab, a.importWizardActive(), a.importReturn)
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
