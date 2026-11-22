package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/internal/store"
)

func TestLoadFromDataDir(t *testing.T) {
	dir := t.TempDir()
	a := &app{dataDir: dir, language: "en"}
	a.load()
	if a.loadError != "" {
		t.Fatal(a.loadError)
	}
	if a.store == nil {
		t.Fatal("store not opened")
	}
	t.Cleanup(func() {
		if err := a.store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if len(a.plans) != 1 {
		t.Fatalf("plans = %d", len(a.plans))
	}
	if _, err := os.Stat(filepath.Join(dir, "tally.db")); err != nil {
		t.Fatal(err)
	}
}

func TestAssessmentWorkflowAtMinimumSize(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{
		"Assessments", "Data", "Calculate", "Export", "Data for this assessment", "Export results",
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	for _, text := range []string{"Calculate", "Export results"} {
		box, ok := tt.Find(text)
		if !ok {
			t.Fatalf("missing box for %q", text)
		}
		if box.X < 0 || box.Y < 0 || box.X+box.W > 1024 || box.Y+box.H > 640 {
			t.Fatalf("%q is outside the window: %+v", text, box)
		}
	}
	if err := tt.Click("Calculate"); err != nil {
		t.Fatal(err)
	}
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}
	if len(a.assessmentRows) != 2 {
		t.Fatalf("result rows = %d", len(a.assessmentRows))
	}
}

func TestAssessmentExcludeClassFromZeroDialog(t *testing.T) {
	fixture := newAssessmentPageFixture(t, true)
	a := fixture.App
	a.startAssessmentCalculate()
	if !a.zeroOpen || len(a.zeroIssues) == 0 {
		t.Fatalf("zero dialog = %v issues=%d", a.zeroOpen, len(a.zeroIssues))
	}
	a.zeroReason = "本班不参与"
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Do not participate"); err != nil {
		t.Fatal(err)
	}
	if a.zeroError != "" {
		t.Fatal(a.zeroError)
	}
	if a.zeroOpen {
		t.Fatal("zero dialog still open")
	}
	if len(a.assessmentRows) == 0 {
		t.Fatal("missing results after exclusion")
	}
}

func TestAssessmentOpenExamData(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Open 期中考试 data"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageExams || a.selectedExamID != fixture.ExamID {
		t.Fatalf("page = %q exam = %d", a.page, a.selectedExamID)
	}
	if a.examCohortID != fixture.CohortID {
		t.Fatalf("cohort = %d", a.examCohortID)
	}
}

func TestAssessmentNewExamFromEmptyState(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("No exams yet") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("New exam"); err != nil {
		t.Fatal(err)
	}
	if !a.newExamOpen {
		t.Fatal("new exam dialog not open")
	}
	if a.newExamYear == 0 || a.newExamDate.IsZero() {
		t.Fatalf("defaults = %v %v", a.newExamYear, a.newExamDate)
	}
}

func TestLabelHelpers(t *testing.T) {
	a := newTestApp(t)
	i18n.SetLocaleFunc(func() string { return "en" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	if got := subjectLabel("chinese"); got != "Chinese" {
		t.Errorf("subjectLabel = %q", got)
	}
	if got := subjectLabel("unknown"); got != "unknown" {
		t.Errorf("subjectLabel unknown = %q", got)
	}
	for index, want := range []string{"Tier 1", "Tier 2", "Tier 3", "Tier 4"} {
		if got := levelLabel(index); got != want {
			t.Errorf("levelLabel(%d) = %q", index, got)
		}
	}
	if got := decodeNames(`["张老师","李老师"]`); got != "张老师、李老师" {
		t.Errorf("decodeNames = %q", got)
	}
	if got := decodeNames("raw"); got != "raw" {
		t.Errorf("decodeNames invalid = %q", got)
	}
	if got := decodeNames("  "); got != "" {
		t.Errorf("decodeNames empty = %q", got)
	}
	if got := a.semesterLabel("autumn"); got != "Autumn" {
		t.Errorf("semesterLabel = %q", got)
	}
	if got := a.semesterLabel("other"); got != "other" {
		t.Errorf("semesterLabel = %q", got)
	}
	if got := a.examLabel("final"); got != "Final" {
		t.Errorf("examLabel = %q", got)
	}
	if got := a.examLabel("gaokao"); got != "Gaokao" {
		t.Errorf("examLabel = %q", got)
	}
	if got := a.examLabel("other"); got != "other" {
		t.Errorf("examLabel = %q", got)
	}
	if got := a.gradeLabel("g2"); got != "Grade 11" {
		t.Errorf("gradeLabel = %q", got)
	}
	if got := a.gradeLabel("g3"); got != "Grade 12" {
		t.Errorf("gradeLabel = %q", got)
	}
	if got := a.gradeLabel("other"); got != "other" {
		t.Errorf("gradeLabel = %q", got)
	}
	if got := semesterCode("Autumn"); got != "autumn" {
		t.Errorf("semesterCode = %q", got)
	}
	if got := semesterCode("other"); got != "other" {
		t.Errorf("semesterCode = %q", got)
	}
	if got := examCode("Final"); got != "final" {
		t.Errorf("examCode = %q", got)
	}
	if got := examCode("Gaokao"); got != "gaokao" {
		t.Errorf("examCode = %q", got)
	}
	if got := examCode("other"); got != "other" {
		t.Errorf("examCode = %q", got)
	}
	if got := gradeCode("Grade 11"); got != "g2" {
		t.Errorf("gradeCode = %q", got)
	}
	if got := gradeCode("Grade 12"); got != "g3" {
		t.Errorf("gradeCode = %q", got)
	}
	if got := gradeCode("other"); got != "other" {
		t.Errorf("gradeCode = %q", got)
	}
	if got := a.tierLabel("unknown"); got != "unknown" {
		t.Errorf("tierLabel = %q", got)
	}
	if got := a.kindLabel("system"); got != "Built-in" {
		t.Errorf("kindLabel = %q", got)
	}
	if got := a.kindLabel("user"); got != "Derived" {
		t.Errorf("kindLabel = %q", got)
	}
}

func TestSelectedValuesAndMissingAssessment(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	a.selectedPlanID = "not-a-number"
	if a.selectedPlan() != nil {
		t.Fatal("expected nil plan")
	}
	a.reloadVersions(ctx)
	if a.loadError == "" {
		t.Fatal("expected parse error")
	}
	a.loadError = ""
	a.selectedPlanID = ""
	a.reloadVersions(ctx)
	if a.versions != nil || a.versionIndex != -1 {
		t.Fatalf("versions = %v index = %d", a.versions, a.versionIndex)
	}
	a.versionIndex = 5
	if a.selectedVersion() != nil {
		t.Fatal("expected nil version")
	}
	a.selectedAssessmentID = 999
	if a.selectedAssessmentOverview() != nil {
		t.Fatal("expected nil assessment")
	}
	if got := a.labelForVersionID(999); got != "" {
		t.Fatalf("label = %q", got)
	}
}

func TestAssessmentCalculateWithoutVersion(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.calculateVersion = "missing"
	a.startAssessmentCalculate()
	if a.assessmentError == "" {
		t.Fatal("expected version error")
	}
}

func TestAssessmentExportError(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.finishAssessmentExport("out.xlsx", os.ErrPermission)
	if a.assessmentError == "" {
		t.Fatal("expected export error")
	}
}

func TestAssessmentSelectionAndOverview(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	if len(a.assessments) != 1 {
		t.Fatalf("assessments = %d", len(a.assessments))
	}
	item := a.assessments[0]
	if item.AssessmentID != fixture.AssessmentID {
		t.Fatalf("assessment = %d", item.AssessmentID)
	}
	if got := a.assessmentOverviewLabel(item); got != "2026级 Midterm" {
		t.Fatalf("label = %q", got)
	}
	a.selectedAssessmentID = 0
	a.reloadAssessments(context.Background())
	if a.selectedAssessmentID != fixture.AssessmentID {
		t.Fatalf("selected = %d", a.selectedAssessmentID)
	}
	if a.assessmentContext.Exam.ID != fixture.ExamID {
		t.Fatalf("context exam = %d", a.assessmentContext.Exam.ID)
	}
}

func TestAssessmentDetailWithoutSelection(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("No exams yet") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	a.assessments = []store.AssessmentOverview{{AssessmentID: 1}}
	a.selectedAssessmentID = 0
	tt.Frame()
	if !tt.HasText("Select an exam to see what is ready.") {
		t.Fatalf("texts = %q", tt.Texts())
	}
}

func TestAssessmentOverviewSortsRecentFirst(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	ctx := context.Background()
	cohort := a.selectedCohort()
	exam, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermSecond, ExamType: store.ExamFinal,
		Name: "期末考试", ExamDate: "2027-01-12", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	a.reloadExams(ctx)
	a.reloadAssessments(ctx)
	if len(a.assessments) != 2 {
		t.Fatalf("assessments = %d", len(a.assessments))
	}
	if a.assessments[0].ExamID != exam.ID {
		t.Fatalf("first exam = %d", a.assessments[0].ExamID)
	}
}
