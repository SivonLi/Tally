package main

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/importer"
	"tally/locales"
)

func TestOverviewLiveLedgerPopulatedAndNavigation(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageOverview
	a.reloadWorkspaceSummary(context.Background())

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 验证总览显示真实台账 KPI、最近一次考核快照与历次考试时间轴，而非旧版「准备情况」引导页。
	for _, want := range []string{
		locales.OverviewMetricClasses(),
		locales.OverviewMetricStudents(),
		locales.OverviewMetricTeachers(),
		locales.OverviewMetricBaselines(),
		locales.OverviewLatestAssessment(),
		locales.OverviewLatestOfficial(),
		locales.OverviewExamTimeline(),
		locales.OverviewQuickImport(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("overview ledger missing %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("Continue setup") {
		t.Fatalf("legacy onboarding button should not appear on overview: %q", tt.Texts())
	}

	// 点击「学生总数」KPI 卡片跳转到基础数据-学生页签。
	if err := tt.Click(locales.OverviewMetricStudents()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageBaseData || a.baseDataTab != 1 {
		t.Fatalf("clicking Students KPI -> page=%q tab=%d, want base_data/1", a.page, a.baseDataTab)
	}

	// 回到总览，点击「去导入数据」应携带返回总览上下文跳转到独立数据导入页。
	a.navigateTo(pageOverview)
	settleTester(tt)
	if err := tt.Click(locales.OverviewQuickImport()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageImports || a.importReturn == nil || a.importReturn.Page != pageOverview {
		t.Fatalf("clicking Import data -> page=%q return=%+v", a.page, a.importReturn)
	}

	// 回到总览，点击「进入考核工作台」跳转到考核页。
	a.returnFromImport()
	settleTester(tt)
	if a.page != pageOverview {
		t.Fatalf("returnFromImport did not restore overview: %q", a.page)
	}
	if err := tt.Click(locales.OverviewOpenWorkbench()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageAssessments {
		t.Fatalf("clicking Open workbench -> page=%q, want %q", a.page, pageAssessments)
	}
}

func TestOverviewMissingDataAlertsJumpToImportWithReturn(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	a.reloadWorkspaceSummary(context.Background())

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	if !tt.HasText(locales.OverviewAlertNoRoster()) {
		t.Fatalf("expected missing roster alert in %q", tt.Texts())
	}
	if err := tt.Click(locales.OverviewAlertNoRoster()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageImports || !a.importWizardActive() || a.importWizard.Kind != importer.KindStudents {
		t.Fatalf("clicking missing roster alert -> page=%q active=%v wizard=%+v",
			a.page, a.importWizardActive(), a.importWizard)
	}
	if a.importReturn == nil || a.importReturn.Page != pageOverview {
		t.Fatalf("expected import return context to overview, got %+v", a.importReturn)
	}
}
