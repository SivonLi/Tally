package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func TestOverviewLiveLedgerPopulatedAndNavigation(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageOverview
	a.reloadSchoolOverview(context.Background())

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 验证总览驾驶舱显示在读年级态势卡、教师占用统计、近期考试与待办提醒。
	for _, want := range []string{
		locales.OverviewActiveCohortsTitle(),
		locales.OverviewOccupancyTitle(),
		locales.OverviewOccupancySingle(),
		locales.OverviewRecentExamsTitle(),
		locales.OverviewRemindersTitle(),
		locales.OverviewEnterCohort(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("overview ledger missing %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("Continue setup") {
		t.Fatalf("legacy onboarding button should not appear on overview: %q", tt.Texts())
	}

	// 点击「单届在任」占用 KPI 卡片跳转到教师档案并预选筛选状态。
	if err := tt.Click(locales.OverviewOccupancySingle()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageFaculty || a.teacherOccupancyFilter != store.TeacherStatusActiveSingle {
		t.Fatalf("clicking Single cohort KPI -> page=%q filter=%q", a.page, a.teacherOccupancyFilter)
	}

	// 回到总览，点击年级态势卡「进入年级档案」跳转到年级详情页。
	a.navigateTo(pageOverview)
	settleTester(tt)
	if err := tt.Click(locales.OverviewEnterCohort()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageCohorts || !a.cohortDetailOpen {
		t.Fatalf("clicking Enter cohort -> page=%q detailOpen=%v", a.page, a.cohortDetailOpen)
	}
}

func TestOverviewMissingDataAlertsJumpToImportWithReturn(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageOverview
	a.reloadSchoolOverview(context.Background())

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	wantReminder := fmt.Sprintf(locales.OverviewReminderCohortSetup(), a.cohortName(2026), locales.CohortTabStudents())
	if !tt.HasText(wantReminder) {
		t.Fatalf("expected missing students reminder %q in %q", wantReminder, tt.Texts())
	}
	if err := tt.Click(wantReminder); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageCohorts || !a.cohortDetailOpen || a.cohortDetailTab != 2 {
		t.Fatalf("clicking missing students reminder -> page=%q detail=%v tab=%d",
			a.page, a.cohortDetailOpen, a.cohortDetailTab)
	}
}
