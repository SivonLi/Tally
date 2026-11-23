package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func TestAnalysisPageTabs(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAnalysis
	a.language = "zh-Hans"

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	tt.Frame()

	// 确认顶栏与各维度选项卡文字存在
	for _, text := range []string{
		locales.NavAnalysis(),
		locales.AnalysisTabTeacherTrend(),
		locales.AnalysisTabClassTrend(),
		locales.AnalysisTabSubjectComparison(),
		locales.AnalysisTabStudentTrack(),
		locales.AnalysisTabAssessmentComparison(),
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing text %q in analysis page", text)
		}
	}

	// 切换到班级趋势
	if err := tt.Click(locales.AnalysisTabClassTrend()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.analysisTabIndex != 1 {
		t.Fatalf("expected analysisTabIndex 1, got %d", a.analysisTabIndex)
	}

	// 切换到学科对比
	if err := tt.Click(locales.AnalysisTabSubjectComparison()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.analysisTabIndex != 2 {
		t.Fatalf("expected analysisTabIndex 2, got %d", a.analysisTabIndex)
	}

	// 切换到学生跟踪
	if err := tt.Click(locales.AnalysisTabStudentTrack()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.analysisTabIndex != 3 {
		t.Fatalf("expected analysisTabIndex 3, got %d", a.analysisTabIndex)
	}

	// 切换到考核对比
	if err := tt.Click(locales.AnalysisTabAssessmentComparison()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.analysisTabIndex != 4 {
		t.Fatalf("expected analysisTabIndex 4, got %d", a.analysisTabIndex)
	}
}

func TestAnalysisSavedViewFlow(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	a.page = pageAnalysis
	a.language = "zh-Hans"

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	tt.Frame()

	// 保存视图
	a.analysisTabIndex = 2
	a.analysisSubject = "chinese"
	a.saveCurrentView(ctx, "语文学科分析")

	if a.analysisSelectedViewID <= 0 {
		t.Fatalf("expected saved view ID > 0, got %d", a.analysisSelectedViewID)
	}
	if len(a.analysisSavedViews) != 1 {
		t.Fatalf("expected 1 saved view, got %d", len(a.analysisSavedViews))
	}

	// 切换回 tab 0 后恢复
	a.analysisTabIndex = 0
	a.applySavedView(ctx, a.analysisSavedViews[0])
	if a.analysisTabIndex != 2 || a.analysisSubject != "chinese" {
		t.Fatalf("saved view restoration failed: tab=%d, subject=%s", a.analysisTabIndex, a.analysisSubject)
	}

	// 删除视图
	savedID := a.analysisSelectedViewID
	a.deleteSavedView(ctx, savedID)
	if len(a.analysisSavedViews) != 0 {
		t.Fatalf("expected 0 saved views after deletion, got %d", len(a.analysisSavedViews))
	}
}

func TestAnalysisDeleteViewRequiresConfirmation(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	a.page = pageAnalysis
	a.language = "zh-Hans"

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	a.saveCurrentView(ctx, "语文学科分析")
	tt.Frame()
	if a.analysisSelectedViewID <= 0 || len(a.analysisSavedViews) != 1 {
		t.Fatalf("saved view setup: id=%d views=%d", a.analysisSelectedViewID, len(a.analysisSavedViews))
	}

	if err := tt.Click(locales.AnalysisDeleteView()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.analysisDeleteViewOpen {
		t.Fatal("delete confirmation did not open")
	}
	title := fmt.Sprintf(locales.AnalysisDeleteViewTitle(), "语文学科分析")
	if !tt.HasText(title) {
		t.Fatalf("missing %q in %q", title, tt.Texts())
	}
	if len(a.analysisSavedViews) != 1 {
		t.Fatalf("view deleted before confirmation: %d", len(a.analysisSavedViews))
	}

	if err := tt.Click(locales.AnalysisCancelBtn()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.analysisDeleteViewOpen || len(a.analysisSavedViews) != 1 {
		t.Fatalf("cancel changed the saved view: open=%v views=%d",
			a.analysisDeleteViewOpen, len(a.analysisSavedViews))
	}

	if err := tt.Click(locales.AnalysisDeleteView()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	confirm := fmt.Sprintf(locales.AnalysisDeleteViewConfirm(), "语文学科分析")
	if err := tt.Click(confirm); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.analysisDeleteViewOpen || len(a.analysisSavedViews) != 0 {
		t.Fatalf("confirmed delete did not run: open=%v views=%d",
			a.analysisDeleteViewOpen, len(a.analysisSavedViews))
	}
}

func TestAnalysisExportExecution(t *testing.T) {
	a := newTestApp(t)
	tmpDir := t.TempDir()

	// 测试 5 种维度的导出执行与结果文件生成
	a.analysisTabIndex = 0
	a.analysisTeacherResult = store.TeacherTrendResult{
		TeacherName:          "张三",
		AveragePersonalScore: 88.5,
		Points: []store.TeacherTrendPoint{
			{
				ExamName:      "期中考试",
				ExamDate:      "2026-11-01",
				PersonalScore: 88.5,
			},
		},
	}
	p0 := filepath.Join(tmpDir, "teacher.xlsx")
	a.performAnalysisExport(p0)
	if fi, err := os.Stat(p0); err != nil || fi.Size() == 0 {
		t.Fatalf("failed to export teacher trend: %v", err)
	}

	a.analysisTabIndex = 1
	a.analysisClassResult = store.ClassTrendResult{
		ClassName: "高一(1)班",
		Points: []store.ClassTrendPoint{
			{
				ExamName: "期中考试",
				ExamDate: "2026-11-01",
			},
		},
	}
	p1 := filepath.Join(tmpDir, "class.xlsx")
	a.performAnalysisExport(p1)
	if fi, err := os.Stat(p1); err != nil || fi.Size() == 0 {
		t.Fatalf("failed to export class trend: %v", err)
	}

	a.analysisTabIndex = 2
	a.analysisSubjectResult = store.SubjectComparisonResult{
		Subject: "chinese",
		Groups: []store.SubjectComparisonGroup{
			{
				LeadClassName: "高一(1)班",
				Total:         85.0,
			},
		},
	}
	p2 := filepath.Join(tmpDir, "subject.xlsx")
	a.performAnalysisExport(p2)
	if fi, err := os.Stat(p2); err != nil || fi.Size() == 0 {
		t.Fatalf("failed to export subject comparison: %v", err)
	}

	a.analysisTabIndex = 3
	a.analysisStudentResult = store.StudentTrackResult{
		StudentName: "李四",
		Points: []store.StudentTrackPoint{
			{
				ExamName:   "期中考试",
				TotalScore: 610.5,
			},
		},
	}
	p3 := filepath.Join(tmpDir, "student.xlsx")
	a.performAnalysisExport(p3)
	if fi, err := os.Stat(p3); err != nil || fi.Size() == 0 {
		t.Fatalf("failed to export student tracking: %v", err)
	}

	a.analysisTabIndex = 4
	a.analysisComparisonResult = store.AssessmentComparisonResult{
		Items: []store.AssessmentComparisonItem{
			{
				ExamName:     "期中考试",
				AverageScore: 82.5,
			},
			{
				ExamName:     "期末考试",
				AverageScore: 84.0,
			},
		},
	}
	p4 := filepath.Join(tmpDir, "comparison.xlsx")
	a.performAnalysisExport(p4)
	if fi, err := os.Stat(p4); err != nil || fi.Size() == 0 {
		t.Fatalf("failed to export assessment comparison: %v", err)
	}
}

func TestAnalysisTabsRenderWithData(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	a.page = pageAnalysis
	a.language = "zh-Hans"

	// 准备样本数据
	a.analysisTeacherResult = store.TeacherTrendResult{
		TeacherName:          "张老师",
		Subject:              "chinese",
		AveragePersonalScore: 92.4,
		LatestPersonalScore:  95.0,
		HasMultiplePlans:     true,
		Points: []store.TeacherTrendPoint{
			{
				ExamName:      "高一上期中",
				ExamDate:      "2026-11-01",
				PersonalScore: 89.8,
				Rows: []store.TeacherTrendRow{
					{
						StatGroupName:    "高一(1)班语文",
						Subject:          "chinese",
						ClassAverage:     92.0,
						GradeAverage:     88.0,
						ImprovementScore: 104.5,
						PassRateScore:    95.0,
						CompletionScore:  90.0,
						Total:            96.5,
						Rank:             1,
					},
				},
			},
			{
				ExamName:      "高一上期末",
				ExamDate:      "2027-01-15",
				PersonalScore: 95.0,
			},
		},
	}

	a.analysisClassResult = store.ClassTrendResult{
		ClassID:                1,
		ClassName:              "高一(1)班",
		AverageCompletionScore: 94.2,
		Points: []store.ClassTrendPoint{
			{
				ExamName:        "高一上期中",
				ExamDate:        "2026-11-01",
				CompletionScore: 94.2,
				Rows: []store.ClassTrendRow{
					{
						Subject:          "chinese",
						ClassAverage:     92.0,
						GradeAverage:     88.0,
						ImprovementScore: 104.5,
						PassRateScore:    95.0,
						CompletionScore:  90.0,
						Total:            96.5,
						Rank:             1,
						TeacherNames:     []string{"张老师"},
					},
				},
			},
		},
	}

	a.analysisSubjectResult = store.SubjectComparisonResult{
		Subject:            "chinese",
		ExamName:           "高一上期中",
		GroupCount:         2,
		AverageScore:       90.0,
		AverageImprovement: 102.0,
		AveragePassRate:    95.0,
		Groups: []store.SubjectComparisonGroup{
			{
				LeadClassName:   "高一(1)班",
				ClassAverage:    92.0,
				PassRateScore:   96.0,
				CompletionScore: 94.0,
				Total:           95.0,
				Rank:            1,
				TeacherNames:    []string{"张老师"},
			},
			{
				LeadClassName:   "高一(2)班",
				ClassAverage:    88.0,
				PassRateScore:   92.0,
				CompletionScore: 88.0,
				Total:           85.0,
				Rank:            2,
				TeacherNames:    []string{"李老师"},
			},
		},
	}

	a.analysisStudentResult = store.StudentTrackResult{
		StudentID:   1,
		StudentName: "王小明",
		StudentNo:   "2026001",
		ClassName:   "高一(1)班",
		Track:       "physics",
		Points: []store.StudentTrackPoint{
			{
				ExamName:      "高一上期中",
				ExamDate:      "2026-11-01",
				TotalScore:    620.5,
				GradeRank:     15,
				TotalStudents: 800,
				Scores: map[string]float64{
					"chinese": 115.0,
					"math":    125.0,
					"english": 130.0,
				},
			},
		},
	}

	a.analysisComparisonResult = store.AssessmentComparisonResult{
		PlanDifferent: true,
		Items: []store.AssessmentComparisonItem{
			{
				ExamName:           "高一上期中",
				ExamDate:           "2026-11-01",
				AcademicYear:       "2026-2027",
				Semester:           "第一学期",
				ParticipantCount:   800,
				GroupCount:         18,
				AverageScore:       88.5,
				AverageClass:       88.5,
				AverageImprovement: 101.2,
				PassTotal:          650,
			},
			{
				ExamName:           "高一上期末",
				ExamDate:           "2027-01-15",
				AcademicYear:       "2026-2027",
				Semester:           "第一学期",
				ParticipantCount:   795,
				GroupCount:         18,
				AverageScore:       90.2,
				AverageClass:       90.2,
				AverageImprovement: 103.0,
				PassTotal:          670,
			},
		},
	}

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")

	for tab := 0; tab < 5; tab++ {
		a.analysisTabIndex = tab
		tt.Frame()
	}

	// 触发 reloadAnalysis 各分支
	for tab := 0; tab < 5; tab++ {
		a.analysisTabIndex = tab
		a.reloadAnalysis(ctx)
	}
}
