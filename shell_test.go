package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func addCohort(t *testing.T, a *app, year int) {
	t.Helper()
	a.openNewCohort()
	a.newCohortYear = float64(year)
	a.createCohort()
	if a.newCohortError != "" {
		t.Fatal(a.newCohortError)
	}
}

func TestOverviewEmptyStateAndCohortCreation(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("No cohorts yet. Create a cohort to get started.") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("New cohort"); err != nil {
		t.Fatal(err)
	}
	if !a.newCohortOpen {
		t.Fatal("new cohort dialog not open")
	}
	tt.Frame()
	if err := tt.Click("Create cohort"); err != nil {
		t.Fatal(err)
	}
	if a.newCohortOpen {
		t.Fatal("new cohort dialog still open")
	}
	if len(a.cohorts) != 1 || a.cohorts[0].EnrollmentYear != 2026 {
		t.Fatalf("cohorts = %+v", a.cohorts)
	}
	if a.selectedCohort() == nil {
		t.Fatal("no selected cohort")
	}
	tt.Frame()
	for _, text := range []string{
		"2026 cohort · Grade 10 · Class of 2029",
		"2026-2027 academic year",
		"0 of 5 data items ready",
	} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
	if len(a.cohortStages) != 1 || a.cohortStages[0].GradeLevel != "g1" {
		t.Fatalf("stages = %+v", a.cohortStages)
	}
}

func TestStageAdvancesWithAcademicYear(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	addCohort(t, a, 2026)
	a.setAcademicYear(2027)
	if a.settingsError != "" {
		t.Fatal(a.settingsError)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{
		"2026 cohort · Grade 11 · Class of 2029",
		"2027-2028 academic year",
	} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
	if len(a.cohortStages) != 2 {
		t.Fatalf("stages = %+v", a.cohortStages)
	}
	year, err := a.store.CurrentAcademicYear(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if year != 2027 {
		t.Fatalf("saved year = %d", year)
	}
}

func TestCohortSwitcher(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	addCohort(t, a, 2025)
	addCohort(t, a, 2026)
	if got := a.selectedCohort(); got == nil || got.EnrollmentYear != 2026 {
		t.Fatalf("selected cohort = %+v", got)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Current cohort"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("2025 cohort"); err != nil {
		t.Fatal(err)
	}
	cohort := a.selectedCohort()
	if cohort == nil || cohort.EnrollmentYear != 2025 {
		t.Fatalf("selected cohort = %+v", cohort)
	}
	tt.Frame()
	if !tt.HasText("2025 cohort · Grade 11 · Class of 2028") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestCohortSwitcherHiddenWithoutCohorts(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if _, ok := tt.Find("Current cohort"); ok {
		t.Error("the cohort switcher shows without a cohort")
	}
	if !tt.HasText("No cohorts yet. Create a cohort to get started.") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestStageHistorySwitching(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	addCohort(t, a, 2026)
	a.setAcademicYear(2027)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Current cohort"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Grade 10 · 2026-2027"); err != nil {
		t.Fatal(err)
	}
	stage := a.selectedStage()
	if stage == nil || stage.GradeLevel != "g1" {
		t.Fatalf("stage = %+v", stage)
	}
	tt.Frame()
	if !tt.HasText("2026 cohort · Grade 10 · Class of 2029") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if !tt.HasText("2026-2027 academic year") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestSettingsWindowCategories(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{
		"School and academic year",
		"Subjects",
		"Data",
		"Language and appearance",
	} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Language and appearance"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Appearance") || !tt.HasText("Language") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

// 设置窗口与主窗口一致：内容区纯黑，左侧分类使用系统材质。
func TestSettingsWindowUsesBlackAndMaterial(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("en")
	tt.SetDark(true)
	tt.SetVibrancy(true)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	img := tt.Image()
	if got := img.RGBAAt(600, 400); got.R != 0 || got.G != 0 || got.B != 0 {
		t.Fatalf("content background = %v, want black", got)
	}
	if _, ok := tt.Find("Subjects"); !ok {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestAppearanceSwitchPersists(t *testing.T) {
	a := newTestApp(t)
	a.settingsPageID = "appearance"
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Dark"); err != nil {
		t.Fatal(err)
	}
	if a.appearance != store.AppearanceDark {
		t.Fatalf("appearance = %q", a.appearance)
	}
	saved, err := a.store.Appearance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if saved != store.AppearanceDark {
		t.Fatalf("saved appearance = %q", saved)
	}
	tt.Frame()
	if err := tt.Click("Light"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("System"); err != nil {
		t.Fatal(err)
	}
}

func TestSidebarNavigationToDataPages(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Students"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageStudents {
		t.Fatalf("page = %q", a.page)
	}
	if !tt.HasText("Create a cohort first") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	settings := ui.NewTester(a.settingsView, 760, 620)
	settings.SetLocale("en")
	settings.Frame()
	if !settings.HasText("School and academic year") {
		t.Errorf("texts = %q", settings.Texts())
	}
	settings.Frame()
	if err := settings.Click("Data"); err != nil {
		t.Fatal(err)
	}
	settings.Frame()
	if !settings.HasText(a.dataDir) {
		t.Errorf("texts = %q", settings.Texts())
	}
	if err := tt.Click("Help"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageHelp {
		t.Fatalf("page = %q", a.page)
	}
	if !tt.HasText("Import guide") || !tt.HasText("Merged group") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestSettingsChangesAcademicYear(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("2026-2027 academic year"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("2027-2028 academic year"); err != nil {
		t.Fatal(err)
	}
	if a.academicYearStart != 2027 {
		t.Fatalf("academic year = %d", a.academicYearStart)
	}
	if a.settingsError != "" {
		t.Fatal(a.settingsError)
	}
}

func TestShellAtMinimumSize(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	addCohort(t, a, 2026)
	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{
		"Overview",
		"2026 cohort · Grade 10 · Class of 2029",
		"Continue setup",
		"Readiness",
	} {
		box, ok := tt.Find(text)
		if !ok {
			t.Fatalf("missing box for %q", text)
		}
		if box.W <= 0 || box.H <= 0 {
			t.Fatalf("%q has no visible area: %+v", text, box)
		}
		if box.X < 0 || box.Y < 0 || box.X+box.W > 1024 || box.Y+box.H > 640 {
			t.Fatalf("%q is outside the window: %+v", text, box)
		}
	}
}

func TestDarkThemeShell(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	addCohort(t, a, 2026)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.SetDark(true)
	tt.Frame()
	for _, text := range []string{
		"Overview",
		"2026 cohort · Grade 10 · Class of 2029",
		"Readiness",
	} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestDuplicateCohortShowsError(t *testing.T) {
	a := newTestApp(t)
	a.page = pageOverview
	addCohort(t, a, 2026)
	a.openNewCohort()
	a.newCohortYear = 2026
	a.createCohort()
	if a.newCohortError == "" {
		t.Fatal("duplicate cohort was accepted")
	}
	if want := fmt.Sprintf(locales.In("en").CohortExists(), 2026); a.newCohortError != want {
		t.Fatalf("error = %q", a.newCohortError)
	}
}

func TestLoadDefaultsToOverviewAndDerivesLegacyCohort(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "shell-load-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})
	ctx := context.Background()
	path := filepath.Join(dir, "tally.db")
	opened, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.CreateBatch(ctx, store.CreateBatchInput{
		AcademicYear: "2026-2027",
		Semester:     "autumn",
		ExamType:     "midterm",
		Grade:        "g1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	a := &app{dataDir: dir, language: "en"}
	a.now = func() time.Time { return time.Date(2026, time.November, 19, 12, 0, 0, 0, time.Local) }
	a.load()
	if a.loadError != "" {
		t.Fatal(a.loadError)
	}
	t.Cleanup(func() {
		if err := a.store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if a.page != pageOverview {
		t.Fatalf("page = %q", a.page)
	}
	if a.academicYearStart != 2026 {
		t.Fatalf("academic year = %d", a.academicYearStart)
	}
	if len(a.cohorts) != 1 || a.cohorts[0].EnrollmentYear != 2026 {
		t.Fatalf("cohorts = %+v", a.cohorts)
	}
	if len(a.cohortStages) != 1 || a.cohortStages[0].GradeLevel != "g1" {
		t.Fatalf("stages = %+v", a.cohortStages)
	}
}
