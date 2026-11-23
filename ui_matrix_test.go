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

// -------------------------------------------------------------
// 12 个 View 视图测试
// -------------------------------------------------------------

func TestViewMatrix_Shell(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if !tt.HasText(locales.NavOverview()) {
		t.Errorf("missing overview nav: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "view_shell")
}

func TestViewMatrix_Overview(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_overview")
}

func TestViewMatrix_Workbench(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_workbench")
}

func TestViewMatrix_Analysis(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAnalysis
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_analysis")
}

func TestViewMatrix_Quality(t *testing.T) {
	a := newTestApp(t)
	a.page = pageQuality
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_quality")
}

func TestViewMatrix_DataPages(t *testing.T) {
	a := newTestApp(t)
	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_datapages")
}

func TestViewMatrix_ExamPages(t *testing.T) {
	a := newTestApp(t)
	a.page = pageExams
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_exampages")
}

func TestViewMatrix_PlanPage(t *testing.T) {
	a := newTestApp(t)
	a.page = pagePlans
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_planpage")
}

func TestViewMatrix_PrepWizard(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	a.prepOpen = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_prepwizard")
}

func TestViewMatrix_ImportWizard(t *testing.T) {
	a := newTestApp(t)
	a.openImportWizard(importer.KindStudents)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_importwizard")
}

func TestViewMatrix_SettingsSchool(t *testing.T) {
	a := newTestApp(t)
	a.settingsPageID = "school"
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_settings_school")
}

func TestViewMatrix_SettingsSync(t *testing.T) {
	a := newTestApp(t)
	a.settingsPageID = "sync"
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	captureDualSnapshot(t, tt, "view_settings_sync")
}

// -------------------------------------------------------------
// 9 个 Modal 弹窗测试
// -------------------------------------------------------------

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
	captureDualSnapshot(t, tt, "modal_sync_conflict")
}
