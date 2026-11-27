package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func newMultiGroupAssessmentFixture(t *testing.T) *app {
	t.Helper()
	ctx := context.Background()
	a := newTestApp(t)
	addCohort(t, a, 2026)
	cohort := a.selectedCohort()
	stage := a.selectedStage()

	if _, err := a.store.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班", "2班", "3班"},
			Students: []store.ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", Status: "active"},
				{StudentNo: "002", Name: "乙", ClassName: "2班", Track: "physics", Status: "active"},
				{StudentNo: "003", Name: "丙", ClassName: "3班", Track: "physics", Status: "active"},
			},
		},
		Report: store.ImportReport{FileHash: "mg-stu", Path: "stu.xlsx", RowCount: 3},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Assignments: []store.ImportAssignment{
				{ClassName: "1班", Subject: "chinese", TeacherName: "张老师"},
				{ClassName: "2班", Subject: "chinese", TeacherName: "李老师"},
				{ClassName: "3班", Subject: "chinese", TeacherName: "王老师"},
			},
		},
		Report: store.ImportReport{FileHash: "mg-assign", Path: "assign.xlsx", RowCount: 3},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "baselines",
		Data: store.ImportData{
			ClassBaselines: []store.ImportClassBaseline{
				{ClassName: "1班", Subject: "chinese", Average: 95},
				{ClassName: "2班", Subject: "chinese", Average: 90},
				{ClassName: "3班", Subject: "chinese", Average: 85},
			},
			GradeBaselines: map[string]float64{"chinese": 90},
		},
		Report: store.ImportReport{FileHash: "mg-base", Path: "base.xlsx", RowCount: 3},
	}); err != nil {
		t.Fatal(err)
	}

	exam, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "三组期中", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportRoster,
		Report: store.ImportReport{FileHash: "mg-roster", Path: "roster.xlsx", RowCount: 3},
		Data: store.ImportData{Roster: []store.ImportRoster{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", TotalScore: scorePointer(140)},
			{StudentNo: "002", Name: "乙", ClassName: "2班", Track: "physics", TotalScore: scorePointer(120)},
			{StudentNo: "003", Name: "丙", ClassName: "3班", Track: "physics", TotalScore: scorePointer(100)},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportScores,
		Report: store.ImportReport{FileHash: "mg-scores", Path: "scores.xlsx", RowCount: 3},
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 100}, Flags: map[string]string{}},
			{StudentNo: "002", Scores: map[string]float64{"chinese": 90}, Flags: map[string]string{}},
			{StudentNo: "003", Scores: map[string]float64{"chinese": 80}, Flags: map[string]string{}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportCutoffs,
		Report: store.ImportReport{FileHash: "mg-cutoffs", Path: "cutoffs.xlsx", RowCount: 2},
		Data: store.ImportData{
			SubjectCutoffs: []store.ImportSubjectCutoff{{Subject: "chinese", Track: "physics", Value: 60}},
			TierCutoffs: []store.ImportTierCutoff{
				{TierKey: "qingbei", Track: "physics", Value: 130},
				{TierKey: "c9", Track: "physics", Value: 120},
				{TierKey: "project985", Track: "physics", Value: 110},
				{TierKey: "project211", Track: "physics", Value: 100},
				{TierKey: "special_control", Track: "physics", Value: 90},
				{TierKey: "undergraduate", Track: "physics", Value: 1},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	counts := map[string]*float64{
		"qingbei": scorePointer(0), "c9": scorePointer(0), "project985": scorePointer(0),
		"project211": scorePointer(1), "special_control": scorePointer(1), "undergraduate": scorePointer(1),
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportTargets,
		Report: store.ImportReport{FileHash: "mg-targets", Path: "targets.xlsx", RowCount: 3},
		Data: store.ImportData{ClassTargets: []store.ImportClassTarget{
			{ClassName: "1班", Counts: counts},
			{ClassName: "2班", Counts: counts},
			{ClassName: "3班", Counts: counts},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	assess, err := a.store.AssessmentForExamCohort(ctx, mustExamCohortID(t, a.store, exam.ID, stage.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.EnsureAssessmentStatGroups(ctx, assess.ID); err != nil {
		t.Fatal(err)
	}
	a.reloadCohorts(ctx)
	a.reloadExams(ctx)
	a.reloadAssessments(ctx)
	return a
}

func typeIntoField(t *testing.T, tt *ui.Tester, label, value string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("click field %q: %v", label, err)
	}
	tt.Command("select_all")
	tt.Key(0, ui.KeyBackspace)
	tt.Key(0, ui.KeyBackspace)
	tt.Type(value)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	tt.Frame()
}

func TestWorkbenchBoundaryOverridesFlowAndLocales(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	a := newMultiGroupAssessmentFixture(t)
	a.page = pageAssessments

	tt := ui.NewTester(a.view, 1280, 820)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	// 1. 打开分档边界弹窗并验证 3 组预填（默认 1, 2, 第三档 3-3）
	if err := tt.Click(locales.BoundaryAdjustButton()); err != nil {
		t.Fatalf("click adjust boundaries: %v", err)
	}
	settleTester(tt)
	if !a.boundaryState().open || len(a.boundaryState().rows) == 0 {
		t.Fatal("expected boundary modal open with prefilled rows")
	}
	if _, ok := tt.Find(fmt.Sprintf(locales.BoundaryThirdTierRange(), 3, 3)); !ok {
		t.Fatal("expected initial third tier range 3-3")
	}

	// 2. 通过真实点击输入制造无效边界 (first=3 > second=2)，验证保存被拦阻
	firstLabel := subjectLabel("chinese") + " " + locales.BoundaryFirstRank()
	secondLabel := subjectLabel("chinese") + " " + locales.BoundarySecondRank()
	reasonLabel := subjectLabel("chinese") + " " + locales.BoundaryReason()

	typeIntoField(t, tt, firstLabel, "3")
	typeIntoField(t, tt, secondLabel, "2")
	if err := tt.Click(locales.BoundarySave()); err != nil {
		t.Fatalf("click save invalid boundaries: %v", err)
	}
	settleTester(tt)
	if !a.boundaryState().open || a.boundaryState().errorText == "" {
		t.Fatal("expected invalid boundary (first > second) to block saving and show error")
	}

	// 3. 修正为有效微调 (first=2, second=2) 并验证第三档实时联动，保存后关闭并显示徽标
	typeIntoField(t, tt, firstLabel, "2")
	typeIntoField(t, tt, secondLabel, "2")
	typeIntoField(t, tt, reasonLabel, "语文前两名接近")
	if _, ok := tt.Find(fmt.Sprintf(locales.BoundaryThirdTierRange(), 3, 3)); !ok {
		t.Fatal("expected third tier range updated to 3-3")
	}
	if err := tt.Click(locales.BoundarySave()); err != nil {
		t.Fatalf("click save valid boundaries: %v", err)
	}
	settleTester(tt)
	if a.boundaryState().open {
		t.Fatalf("expected modal closed after save, err=%q", a.boundaryState().errorText)
	}
	if _, ok := tt.Find(fmt.Sprintf(locales.BoundaryOverrideCountBadge(), 1)); !ok {
		t.Fatal("expected override count badge visible on workbench")
	}

	// 4. 第 1 次正式计算（带微调），并在结果详情中回显生效边界与来源
	if err := tt.Click(locales.AssessmentCalculate()); err != nil {
		t.Fatalf("click calculate run 1: %v", err)
	}
	settleTester(tt)
	if len(a.assessmentRows) == 0 {
		t.Fatal("expected calculated assessment rows for run 1")
	}
	run1ID := a.assessmentRun.ID
	a.openResultDetail(a.assessmentRows[0])
	settleTester(tt)
	if _, ok := tt.Find(locales.AssessmentDetailBoundary()); !ok {
		t.Fatal("expected effective boundary row in result detail modal")
	}
	if _, ok := tt.Find(locales.AssessmentDetailBoundarySourceOverride()); !ok {
		t.Fatal("expected override source in result detail modal")
	}
	if _, ok := tt.Find("语文前两名接近"); !ok {
		t.Fatal("expected override reason in result detail modal")
	}
	a.closeResultDetail()
	settleTester(tt)

	// 5. 切换英文语言，单科恢复默认后触发 stale，并执行第 2 次正式计算（默认边界）
	t.Setenv("MYGO_LOCALE", "en")
	tt.SetLocale("en")
	settleTester(tt)
	if err := tt.Click(locales.BoundaryAdjustButton()); err != nil {
		t.Fatalf("click adjust boundaries in en: %v", err)
	}
	settleTester(tt)
	if err := tt.Click(subjectLabel("chinese") + " " + locales.BoundaryResetSubject()); err != nil {
		t.Fatalf("click single subject reset: %v", err)
	}
	settleTester(tt)
	if a.boundaryState().overrideCount != 0 {
		t.Fatalf("expected overrideCount = 0 after single reset, got %d", a.boundaryState().overrideCount)
	}
	if err := tt.Click(locales.BoundaryCancel()); err != nil {
		t.Fatalf("close boundary modal: %v", err)
	}
	settleTester(tt)
	if !a.assessmentOfficialStale {
		t.Fatal("expected assessment marked stale after single reset")
	}
	if err := tt.Click(locales.AssessmentCalculate()); err != nil {
		t.Fatalf("click calculate run 2: %v", err)
	}
	settleTester(tt)

	// 6. 切换回历史运行 1，验证结果详情还原运行 1 的微调边界与备注
	a.selectAssessmentRun(run1ID)
	settleTester(tt)
	a.openResultDetail(a.assessmentRows[0])
	settleTester(tt)
	if _, ok := tt.Find(locales.AssessmentDetailBoundarySourceOverride()); !ok {
		t.Fatal("expected historical run 1 to restore override boundary source")
	}
	if _, ok := tt.Find("语文前两名接近"); !ok {
		t.Fatal("expected historical run 1 to restore override reason")
	}
	a.closeResultDetail()
	settleTester(tt)

	// 7. 全部恢复默认按钮验证
	ctx := context.Background()
	if err := a.store.SaveAssessmentBoundaryOverrides(ctx, a.selectedAssessmentID, []store.AssessmentBoundaryOverride{
		{Subject: "chinese", GroupCount: 3, FirstRank: 1, SecondRank: 3, Reason: "temp"},
	}); err != nil {
		t.Fatal(err)
	}
	a.reloadAssessments(ctx)
	tt.Frame()
	if err := tt.Click(locales.BoundaryAdjustButton()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if err := tt.Click(locales.BoundaryResetAll()); err != nil {
		t.Fatalf("click reset all: %v", err)
	}
	settleTester(tt)
	if a.boundaryState().overrideCount != 0 {
		t.Fatalf("expected overrideCount = 0 after reset all, got %d", a.boundaryState().overrideCount)
	}
}

func TestWorkbenchBoundaryArchivedHidesEntry(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	ctx := context.Background()
	tier0 := 0
	if _, err := a.store.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: fixture.AssessmentID,
		Data: store.ArchivedResultData{
			Rows: []store.ImportArchivedResultRow{
				{
					ClassName: "1班", Subject: "chinese", StatGroupName: "1班",
					TeacherNames: []string{"张老师"}, ClassCount: 2,
					ClassAverage: 95, GradeAverage: 90, BaselineRatio: 1.0,
					ImprovementScore: 30, PassCount: 2, PassScore: 40,
					CompletionScore: 20, Total: 90, Rank: 1, TierIndex: &tier0, Points: 10, SourceRow: 2,
				},
			},
			Personal: []store.ImportArchivedPersonalScore{
				{TeacherName: "张老师", Subject: "chinese", Score: 10, GroupPoints: []float64{10}, SourceRow: 2},
			},
		},
		Report: store.ImportReport{
			FileHash: "arch-ui-test", Path: "archive.xlsx", RowCount: 2,
			Sheets: []store.ImportSheet{
				{Name: "排名赋分", Role: "archive_results", HeaderRow: 1, Rows: 1},
				{Name: "个人得分", Role: "archive_personal", HeaderRow: 1, Rows: 1},
			},
		},
	}); err != nil {
		t.Fatalf("ApplyArchivedResultImport: %v", err)
	}
	a.reloadAssessments(ctx)
	a.page = pageAssessments

	tt := ui.NewTester(a.view, 1280, 820)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if _, ok := tt.Find(locales.BoundaryAdjustButton()); ok {
		t.Fatal("expected archived read-only assessment to hide boundary adjust button")
	}
}

func TestWorkbenchBoundaryStaleInterception(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.page = pageAssessments
	ctx := context.Background()

	if err := a.store.SaveAssessmentBoundaryOverrides(ctx, a.selectedAssessmentID, []store.AssessmentBoundaryOverride{
		{Subject: "chinese", GroupCount: 5, FirstRank: 2, SecondRank: 4, Reason: "组数已变化"},
	}); err != nil {
		t.Fatal(err)
	}
	a.reloadAssessments(ctx)

	tt := ui.NewTester(a.view, 1280, 820)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if err := tt.Click(locales.AssessmentCalculate()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.assessmentError == "" {
		t.Fatal("expected BoundaryStaleError message on workbench")
	}
	if _, ok := tt.Find(locales.BoundaryReviewButton()); !ok {
		t.Fatal("expected review boundaries shortcut button on workbench")
	}

	if err := tt.Click(locales.BoundaryReviewButton()); err != nil {
		t.Fatalf("click review boundaries: %v", err)
	}
	settleTester(tt)
	if !a.boundaryState().open {
		t.Fatal("expected boundary modal to open from review shortcut")
	}
	if _, ok := tt.Find(locales.BoundaryStatusStale()); !ok {
		t.Fatal("expected stale status badge inside boundary modal")
	}
	if err := tt.Click(subjectLabel("chinese") + " " + locales.BoundaryKeepCurrentCount()); err != nil {
		t.Fatalf("click keep current count: %v", err)
	}
	settleTester(tt)
	if err := tt.Click(locales.BoundarySave()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if err := tt.Click(locales.AssessmentCalculate()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.assessmentError != "" || len(a.assessmentRows) == 0 {
		t.Fatalf("expected calculation to succeed after reviewing boundary, err=%q", a.assessmentError)
	}
}

func TestWorkbenchBoundaryModalSnapshots(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")

	aEmpty := newTestApp(t)
	aEmpty.page = pageAssessments
	aEmpty.boundaryState().open = true
	ttEmpty := ui.NewTester(aEmpty.view, 1280, 820)
	ttEmpty.SetLocale("zh-Hans")
	captureDualSnapshot(t, ttEmpty, "modal_boundary_overrides_empty")

	aPop := newPopulatedTestApp(t)
	aPop.page = pageAssessments
	ctx := context.Background()
	if err := aPop.store.SaveAssessmentBoundaryOverrides(ctx, aPop.selectedAssessmentID, []store.AssessmentBoundaryOverride{
		{Subject: "chinese", GroupCount: 1, FirstRank: 1, SecondRank: 1, Reason: "前两名分差接近"},
	}); err != nil {
		t.Fatal(err)
	}
	aPop.reloadAssessments(ctx)
	aPop.openBoundaryModal()

	ttPop := ui.NewTester(aPop.view, 1280, 820)
	ttPop.SetLocale("zh-Hans")
	captureDualSnapshot(t, ttPop, "modal_boundary_overrides_populated")
}
