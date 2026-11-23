package main

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"tally/internal/calc"
	"tally/internal/importer"
	"tally/internal/store"
	syncpkg "tally/internal/sync"
	"tally/locales"
)

func settleTester(tt *ui.Tester) {
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
}

func savePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png %s: %v", path, err)
	}
}

func captureDualSnapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	settleTester(tt)

	dir := filepath.Join("testdata", "snapshots")

	// 1. Light Mode
	tt.SetDark(false)
	settleTester(tt)
	imgLight := tt.Image()
	if imgLight == nil || imgLight.Bounds().Empty() {
		t.Fatalf("snapshot %s: light mode image is empty", name)
	}
	savePNG(t, filepath.Join(dir, name+"_light.png"), imgLight)

	// 2. Dark Mode
	tt.SetDark(true)
	settleTester(tt)
	imgDark := tt.Image()
	if imgDark == nil || imgDark.Bounds().Empty() {
		t.Fatalf("snapshot %s: dark mode image is empty", name)
	}
	savePNG(t, filepath.Join(dir, name+"_dark.png"), imgDark)

	// 3. Assert visual difference between dark and light
	pt := image.Pt(imgLight.Bounds().Min.X+20, imgLight.Bounds().Min.Y+20)
	cLight := imgLight.RGBAAt(pt.X, pt.Y)
	cDark := imgDark.RGBAAt(pt.X, pt.Y)
	lumLight := int(cLight.R) + int(cLight.G) + int(cLight.B)
	lumDark := int(cDark.R) + int(cDark.G) + int(cDark.B)
	if lumDark >= lumLight && cLight == cDark {
		t.Errorf("snapshot %s: dark mode is identical to light mode at %v", name, pt)
	}
}

// newPopulatedTestApp 创建预填充完整业务数据（学届、阶段、名册、教师、成绩、分数线、目标、核算结果、多维分析、质量监测）的应用实例。
func newPopulatedTestApp(t *testing.T) *app {
	t.Helper()
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	ctx := context.Background()

	// 重新加载选中学届与阶段数据，以获取真实 stageCounts
	a.reloadSelectedCohort(ctx)
	a.startAssessmentCalculate()
	if a.assessmentError != "" {
		t.Fatalf("calculate assessment: %v", a.assessmentError)
	}

	// 注入多维分析数据
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
					},
				},
			},
		},
	}

	// 注入教学质量监测数据
	a.qualityExams = []qualityExamOverview{
		{
			ExamID:     fixture.ExamID,
			ExamLabel:  "高一期中考试",
			CohortID:   fixture.CohortID,
			CohortName: "2026 届",
			GradeLevel: "g1",
			Issues: []store.ExamQualityIssue{
				{Severity: "warning", Code: "missing_score", ClassName: "2班", Subject: "math"},
			},
		},
	}
	a.qualityLoaded = true

	// 注入 WebDAV 云同步就绪状态
	a.syncSettings.WebDAVURL = "https://dav.example.com/dav/"
	a.syncSettings.Username = "teacher_admin"
	a.syncSettings.RemoteDir = "Tally"
	a.syncSettings.DeviceName = "教师工作站-A1"
	a.syncSettings.EncryptEnabled = true
	a.syncSettings.LastSyncTime = "2026-11-23 12:00:00"
	a.syncSettings.LastSyncDevice = "教师工作站-A1"
	a.syncRemoteStatus = &syncpkg.RemoteStatus{
		Kind:    syncpkg.StatusUpToDate,
		Message: locales.SettingsSyncStatusUpToDate(),
	}

	return a
}

// =============================================================
// 12 个 View 视图测试：无数据状态 (Empty)
// =============================================================

func TestViewMatrix_Shell_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_shell_empty")
	captureDualSnapshot(t, tt, "view_shell")
}

func TestViewMatrix_Overview_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_overview_empty")
	captureDualSnapshot(t, tt, "view_overview")
}

func TestViewMatrix_Workbench_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_workbench_empty")
	captureDualSnapshot(t, tt, "view_workbench")
}

func TestViewMatrix_Analysis_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAnalysis
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_analysis_empty")
	captureDualSnapshot(t, tt, "view_analysis")
}

func TestViewMatrix_Quality_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pageQuality
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_quality_empty")
	captureDualSnapshot(t, tt, "view_quality")
}

func TestViewMatrix_DataPages_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_datapages_empty")
	captureDualSnapshot(t, tt, "view_datapages")
}

func TestViewMatrix_ExamPages_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pageExams
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_exampages_empty")
	captureDualSnapshot(t, tt, "view_exampages")
}

func TestViewMatrix_PlanPage_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pagePlans
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_planpage_empty")
	captureDualSnapshot(t, tt, "view_planpage")
}

func TestViewMatrix_PrepWizard_Empty(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	a.prepOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_prepwizard_empty")
	captureDualSnapshot(t, tt, "view_prepwizard")
}

func TestViewMatrix_ImportWizard_Empty(t *testing.T) {
	a := newTestApp(t)
	a.openImportWizard(importer.KindStudents)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_importwizard_empty")
	captureDualSnapshot(t, tt, "view_importwizard")
}

func TestViewMatrix_SettingsSchool_Empty(t *testing.T) {
	a := newTestApp(t)
	a.settingsPageID = "school"
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_settings_school_empty")
	captureDualSnapshot(t, tt, "view_settings_school")
}

func TestViewMatrix_SettingsSync_Empty(t *testing.T) {
	a := newTestApp(t)
	a.settingsPageID = "sync"
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_settings_sync_empty")
	captureDualSnapshot(t, tt, "view_settings_sync")
}

// =============================================================
// 12 个 View 视图测试：有数据状态 (Populated)
// =============================================================

func TestViewMatrix_Shell_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_shell_populated")
}

func TestViewMatrix_Overview_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_overview_populated")
}

func TestViewMatrix_Workbench_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_workbench_populated")
}

func TestViewMatrix_Analysis_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageAnalysis
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_analysis_populated")
}

func TestViewMatrix_Quality_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageQuality
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_quality_populated")
}

func TestViewMatrix_DataPages_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_datapages_populated")
}

func TestViewMatrix_ExamPages_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageExams
	if len(a.exams) > 0 {
		a.selectExam(a.exams[0].ID)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_exampages_populated")
}

func TestViewMatrix_PlanPage_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pagePlans
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_planpage_populated")
}

func TestViewMatrix_PrepWizard_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageOverview
	a.prepOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_prepwizard_populated")
}

func TestViewMatrix_ImportWizard_WithData(t *testing.T) {
	a := newTestApp(t)
	a.openImportWizard(importer.KindStudents)
	a.loadWizardFile(a.importWizard, guidedImportWorkbook(t))
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_importwizard_populated")
}

func TestViewMatrix_SettingsSchool_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.settingsPageID = "school"
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_settings_school_populated")
}

func TestViewMatrix_SettingsSync_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.settingsPageID = "sync"
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_settings_sync_populated")
}

// =============================================================
// 9 个 Modal 弹窗测试：空/初始状态
// =============================================================

func TestModalMatrix_NewCohort(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	a.newCohortOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.NewCohortTitle()) {
		t.Errorf("missing new cohort modal title: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_new_cohort_empty")
	captureDualSnapshot(t, tt, "modal_new_cohort")
}

func TestModalMatrix_StageReview(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	_, err := a.store.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	a.reloadCohorts(ctx)

	a.page = pageOverview
	a.stageReviewOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.StageReviewOpenPrep()) {
		t.Errorf("missing stage review modal text: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_stage_review_empty")
	captureDualSnapshot(t, tt, "modal_stage_review")
}

func TestModalMatrix_NewExam(t *testing.T) {
	a := newTestApp(t)
	a.page = pageExams
	a.newExamOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.ExamNewTitle()) {
		t.Errorf("missing new exam modal title: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_new_exam_empty")
	captureDualSnapshot(t, tt, "modal_new_exam")
}

func TestModalMatrix_StatGroup(t *testing.T) {
	a := newTestApp(t)
	a.page = pageExams
	a.selectedExamID = 1
	a.exam = store.Exam{
		ID:                1,
		Name:              "2026 期中考试",
		AcademicYearStart: 2026,
		Term:              "first",
		ExamType:          "midterm",
	}
	a.statGroupOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.ExamStatGroupTitle()) {
		t.Errorf("missing stat group modal title: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_stat_group_empty")
	captureDualSnapshot(t, tt, "modal_stat_group")
}

func TestModalMatrix_Zero(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAssessments
	a.zeroOpen = true
	a.zeroIssues = []calc.ZeroTargetIssue{
		{
			ClassID:      "1",
			ClassName:    "高一(1)班",
			MissingTiers: []string{"本一"},
		},
	}
	a.initZeroValues()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.ZeroTitle()) {
		t.Errorf("missing zero modal title: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_zero_empty")
	captureDualSnapshot(t, tt, "modal_zero")
}

func TestModalMatrix_ResultDetail(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAssessments
	a.resultDetailOpen = true
	a.resultDetail = resultDetail{
		Row: store.StoredResultRow{
			Subject:       "语文",
			StatGroupName: "高一(1)班",
			LeadClassName: "高一(1)班",
			TeacherNames:  "张老师",
			Total:         92.5,
			Rank:          1,
		},
		PlanLabel: "默认方案",
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.AssessmentDetailTitle()) {
		t.Errorf("missing result detail modal title: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_result_detail_empty")
	captureDualSnapshot(t, tt, "modal_result_detail")
}

func TestModalMatrix_AnalysisSaveView(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAnalysis
	a.analysisSaveViewOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.AnalysisSaveCurrentView()) {
		t.Errorf("missing analysis save view modal title: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_analysis_save_view_empty")
	captureDualSnapshot(t, tt, "modal_analysis_save_view")
}

func TestModalMatrix_PlanSave(t *testing.T) {
	a := newTestApp(t)
	a.page = pagePlans
	a.planSaveChoice = planSaveAsking
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.PlanSaveTitle()) {
		t.Errorf("missing plan save modal title: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_plan_save_empty")
	captureDualSnapshot(t, tt, "modal_plan_save")
}

func TestModalMatrix_SyncConflict(t *testing.T) {
	a := newTestApp(t)
	a.settingsPageID = "sync"
	a.syncConflictOpen = true
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.SettingsSyncConflictModalTitle()) {
		t.Errorf("missing sync conflict modal title: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "modal_sync_conflict_empty")
	captureDualSnapshot(t, tt, "modal_sync_conflict")
}

// =============================================================
// 9 个 Modal 弹窗测试：有业务数据填充状态 (Populated)
// =============================================================

func TestModalMatrix_NewCohort_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageOverview
	a.newCohortOpen = true
	a.newCohortYear = 2027
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_new_cohort_populated")
}

func TestModalMatrix_StageReview_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageOverview
	a.stageReviewOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_stage_review_populated")
}

func TestModalMatrix_NewExam_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageExams
	a.newExamOpen = true
	a.newExamName = "高一第一学期期中质量调研"
	a.newExamDate = time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_new_exam_populated")
}

func TestModalMatrix_StatGroup_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageExams
	if len(a.exams) > 0 {
		a.selectedExamID = a.exams[0].ID
		a.exam = a.exams[0]
	}
	a.statGroupOpen = true
	a.statGroupName = "高一物理走班创新组"
	a.statGroupTarget = 45
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_stat_group_populated")
}

func TestModalMatrix_Zero_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageAssessments
	a.zeroOpen = true
	a.zeroIssues = []calc.ZeroTargetIssue{
		{
			ClassID:      "1",
			ClassName:    "高一(1)班",
			MissingTiers: []string{"特控", "本科"},
		},
		{
			ClassID:      "2",
			ClassName:    "高一(2)班",
			MissingTiers: []string{"清北"},
		},
	}
	a.initZeroValues()
	val := 15.0
	a.zeroValues["1:本科"] = &val
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_zero_populated")
}

func TestModalMatrix_ResultDetail_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageAssessments
	a.resultDetailOpen = true
	a.resultDetail = resultDetail{
		Row: store.StoredResultRow{
			Subject:          "语文",
			StatGroupName:    "高一(1)班",
			LeadClassName:    "高一(1)班",
			TeacherNames:     "张老师",
			ClassAverage:     92.5,
			GradeAverage:     88.0,
			ImprovementScore: 104.5,
			PassScore:        95.0,
			CompletionScore:  90.0,
			Total:            96.5,
			Rank:             1,
		},
		PlanLabel: "教学成绩量化考核方案",
		Group: store.ExamStatGroup{
			Name:          "高一(1)班",
			LeadClassName: "1班",
		},
		Students: []store.ResultRowStudent{
			{Position: 1, StudentNo: "001", Name: "甲"},
			{Position: 2, StudentNo: "002", Name: "乙"},
		},
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_result_detail_populated")
}

func TestModalMatrix_AnalysisSaveView_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageAnalysis
	a.analysisSaveViewOpen = true
	a.analysisSaveViewName = "语文教学质量跟踪与趋势对比"
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_analysis_save_view_populated")
}

func TestModalMatrix_PlanSave_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pagePlans
	a.planSaveChoice = planSaveAsking
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_plan_save_populated")
}

func TestModalMatrix_SyncConflict_WithData(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.settingsPageID = "sync"
	a.syncConflictOpen = true
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "modal_sync_conflict_populated")
}
