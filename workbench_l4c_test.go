package main

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func TestL4CCopyPreviousExamTargets(t *testing.T) {
	f := newExamPageFixture(t)
	ctx := context.Background()

	// 给第一场考试设置目标
	ec1, err := f.App.store.GetExamCohort(ctx, f.ExamID, f.CohortID)
	if err != nil {
		t.Fatal(err)
	}
	a1, err := f.App.store.AssessmentForExamCohort(ctx, ec1.ID)
	if err != nil {
		t.Fatal(err)
	}
	classes, err := f.App.store.ListClasses(ctx, f.CohortID, ec1.StageID)
	if err != nil || len(classes) == 0 {
		t.Fatalf("classes = %+v err=%v", classes, err)
	}
	if err := f.App.store.SetClassTarget(ctx, a1.ID, classes[0].ID, "undergraduate", 35); err != nil {
		t.Fatal(err)
	}

	// 新建第二场考试并切换到目标页签，点击「复制上期目标」
	exam2, err := f.App.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamFinal,
		Name: "期末考试", ExamDate: "2027-01-15", CohortIDs: []int64{f.CohortID},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.App.reloadExams(ctx)
	f.App.selectExam(exam2.ID)
	f.App.examTab = 3
	f.App.page = pageExams

	tt := ui.NewTester(f.App.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	if err := tt.Click(locales.WorkbenchCopyPrevTargets()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if len(f.App.examTargets) == 0 {
		t.Fatalf("expected targets copied from previous exam, got 0")
	}
}

func TestL4CRunCompareAndDrilldownDrawer(t *testing.T) {
	a := newPopulatedTestApp(t)
	// 再运行一次试算，产生 2 条核算版本用于对比
	a.startAssessmentTrial()
	if len(a.assessmentRuns) < 2 {
		t.Fatalf("expected >=2 runs after trial, got %d", len(a.assessmentRuns))
	}
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 1. 打开「对比试算/正式」右侧抽屉
	if err := tt.Click(locales.WorkbenchCompareRuns()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.runCompareOpen || a.runCompareDiff == nil {
		t.Fatalf("runCompareDrawer failed to open: open=%v diff=%v", a.runCompareOpen, a.runCompareDiff)
	}
	if !tt.HasText(locales.WorkbenchCompareTitle()) {
		t.Fatalf("missing compare title in %q", tt.Texts())
	}
	if err := tt.Click(locales.Close()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.runCompareOpen {
		t.Fatal("runCompareDrawer did not close")
	}

	// 2. 打开结果行穿透抽屉，验证学生单科实考成绩与达线状态
	if len(a.assessmentRows) == 0 {
		t.Fatal("missing assessment rows")
	}
	a.openResultDetail(a.assessmentRows[0])
	settleTester(tt)
	if !a.resultDetailOpen || len(a.resultDetail.Students) == 0 {
		t.Fatalf("resultDetail drawer did not load students: open=%v count=%d",
			a.resultDetailOpen, len(a.resultDetail.Students))
	}
	if !tt.HasText(locales.AssessmentDetailTitle()) {
		t.Fatalf("missing detail title in %q", tt.Texts())
	}
	if err := tt.Click(locales.AssessmentDetailClose()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.resultDetailOpen {
		t.Fatal("resultDetail drawer did not close")
	}
}

func TestL4CQualityInlineFixDrawer(t *testing.T) {
	f := newExamPageFixture(t)
	ctx := context.Background()
	f.App.page = pageQuality
	f.App.qualityLoaded = false
	f.App.reloadQuality(ctx)

	tt := ui.NewTester(f.App.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	// 丙 (003) 有一条 missing_scores 提醒，原地修正抽屉可为其补录语文成绩并自动复检。
	fixLabel := locales.QualityFixInline() + " 003"
	if err := tt.Click(fixLabel); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !f.App.qualityFixOpen || f.App.qualityFixStudentNo != "003" {
		t.Fatalf("qualityFixDrawer did not open for 003: open=%v no=%q",
			f.App.qualityFixOpen, f.App.qualityFixStudentNo)
	}
	f.App.qualityFixSubject = "chinese"
	f.App.qualityFixScore = 108
	f.App.qualityFixFlag = ""
	if err := tt.Click(locales.Save()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if f.App.qualityFixOpen || f.App.qualityFixError != "" {
		t.Fatalf("saveQualityFix failed: open=%v err=%q", f.App.qualityFixOpen, f.App.qualityFixError)
	}
	// 验证 003 的 missing_scores 提醒已消除
	for _, ov := range f.App.qualityExams {
		for _, iss := range ov.Issues {
			if iss.StudentNo == "003" && iss.Code == "missing_scores" {
				t.Fatalf("expected 003 missing_scores resolved after inline score fix, still found: %+v", iss)
			}
		}
	}
}
