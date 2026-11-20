package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/egoist/mygo/ui"

	"tally/internal/assessment"
	"tally/internal/store"
	"tally/locales"
)

func newTestApp(t *testing.T) *app {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "ui-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})
	ctx := context.Background()
	opened, err := store.Open(ctx, filepath.Join(dir, "tally.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	a := &app{
		store:              opened,
		dataDir:            dir,
		language:           "en",
		sidebarWidth:       260,
		batchSidebarWidth:  240,
		page:               pagePlans,
		systemAcademicYear: 2026,
		academicYearStart:  2026,
	}
	if _, err := opened.EnsureDefaultPlan(ctx); err != nil {
		t.Fatal(err)
	}
	a.reloadPlans(ctx)
	a.reloadCohorts(ctx)
	a.reloadSchoolSettings(ctx)
	a.reloadExams(ctx)
	return a
}

func TestViewShowsDefaultPlan(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{"Assessment plans", "Built-in", "Versions", "Published"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestSidebarChangesPlan(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	config := assessment.DefaultConfig()
	config.Name = "派生方案"
	config.Ranking.TieMode = "dense"
	plan, _, err := a.store.CreatePlan(ctx, store.CreatePlanInput{
		Name:          config.Name,
		Kind:          "user",
		Description:   "从默认方案派生",
		BaseVersionID: a.versions[0].ID,
		Config:        config,
	})
	if err != nil {
		t.Fatal(err)
	}
	a.reloadPlans(ctx)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	if err := tt.Click("派生方案"); err != nil {
		t.Fatal(err)
	}
	if a.selectedPlanID != strconv.FormatInt(plan.ID, 10) {
		t.Fatalf("selected plan = %q", a.selectedPlanID)
	}
	if len(a.versions) != 1 || a.versions[0].Status != "draft" {
		t.Fatalf("versions = %+v", a.versions)
	}
}

func TestPublishAndRetireVersion(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	config := assessment.DefaultConfig()
	config.Name = "派生方案"
	config.Ranking.TieMode = "dense"
	plan, draft, err := a.store.CreatePlan(ctx, store.CreatePlanInput{
		Name:          config.Name,
		Kind:          "user",
		Description:   "从默认方案派生",
		BaseVersionID: a.versions[0].ID,
		Config:        config,
	})
	if err != nil {
		t.Fatal(err)
	}
	a.reloadPlans(ctx)
	a.selectedPlanID = strconv.FormatInt(plan.ID, 10)
	a.versionIndex = 0
	a.reloadVersions(ctx)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	if err := tt.Click("Publish"); err != nil {
		t.Fatal(err)
	}
	version, err := a.store.GetVersion(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if version.Status != "published" {
		t.Fatalf("status = %q", version.Status)
	}
	tt.Frame()
	if err := tt.Click("Retire"); err != nil {
		t.Fatal(err)
	}
	version, err = a.store.GetVersion(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if version.Status != "retired" {
		t.Fatalf("status = %q", version.Status)
	}
}

func TestLanguageSwitch(t *testing.T) {
	a := newTestApp(t)
	i18n.SetLocaleFunc(func() string { return a.language })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	tt := ui.NewTester(a.view, 1280, 800)
	if err := tt.Click(i18n.Autonym("zh-Hans")); err != nil {
		t.Fatal(err)
	}
	if a.language != "zh-Hans" {
		t.Fatalf("language = %q", a.language)
	}
	if !tt.HasText("考核方案") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if err := tt.Click(i18n.Autonym("en")); err != nil {
		t.Fatal(err)
	}
	if a.language != "en" {
		t.Fatalf("language = %q", a.language)
	}
	if !tt.HasText("Assessment plans") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestNavigationSwitchesPages(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Assessments"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageAssessments {
		t.Fatalf("page = %q", a.page)
	}
	if !tt.HasText("No assessment batches yet.") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Assessment plans"); err != nil {
		t.Fatal(err)
	}
	if a.page != pagePlans {
		t.Fatalf("page = %q", a.page)
	}
}

func TestEmptyBatchStateAtMinimumSize(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAssessments
	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("No assessment batches yet.") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if err := tt.Click("New batch"); err != nil {
		t.Fatal(err)
	}
	if !a.newBatchOpen {
		t.Fatal("new batch dialog not open")
	}
}

func TestDarkThemeRendersPlanPage(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.SetDark(true)
	tt.Frame()
	for _, text := range []string{"Assessment plans", "Built-in", "Versions"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestPlanPageAtMinimumSize(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{"Built-in", "Versions", "Update this plan", "Save as new plan"} {
		box, ok := tt.Find(text)
		if !ok {
			t.Fatalf("missing box for %q", text)
		}
		if box.X < 0 || box.Y < 0 || box.X+box.W > 1024 || box.Y+box.H > 640 {
			t.Fatalf("%q is outside the window: %+v", text, box)
		}
	}
}

func TestUpdateSystemPlanFromEditor(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Update this plan"); err != nil {
		t.Fatal(err)
	}
	if !a.planEditOpen {
		t.Fatal("editor not open")
	}
	config := a.versions[a.versionIndex].Config
	config.Ranking.TieMode = "dense"
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	a.planEditDraft = string(data)
	a.savePlanUpdate()
	if a.planEditError != "" {
		t.Fatal(a.planEditError)
	}
	if a.planEditOpen {
		t.Fatal("editor still open")
	}
	if a.loadError != "" || a.planError != "" {
		t.Fatalf("loadError=%q planError=%q", a.loadError, a.planError)
	}
	if want := fmt.Sprintf(locales.PlanUpdateSaved(), 2); a.planMessage != want {
		t.Fatalf("message = %q", a.planMessage)
	}
	latest, err := a.store.LatestPublished(context.Background(), a.versions[a.versionIndex].PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != 2 || latest.Status != "published" {
		t.Fatalf("latest = %+v", latest)
	}
	if latest.Config.Ranking.TieMode != "dense" {
		t.Fatalf("tie mode = %q", latest.Config.Ranking.TieMode)
	}
}

func TestSaveAsNewPlanFromEditor(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Save as new plan"); err != nil {
		t.Fatal(err)
	}
	if !a.planEditOpen {
		t.Fatal("editor not open")
	}
	a.planEditName = "Derived plan"
	a.savePlanAsNew()
	if a.planEditError != "" {
		t.Fatal(a.planEditError)
	}
	if a.planEditOpen {
		t.Fatal("editor still open")
	}
	if a.loadError != "" || a.planError != "" {
		t.Fatalf("loadError=%q planError=%q", a.loadError, a.planError)
	}
	if len(a.plans) != 2 {
		t.Fatalf("plans = %+v", a.plans)
	}
	selected := a.selectedPlan()
	if selected == nil || selected.Name != "Derived plan" || selected.Kind != "user" {
		t.Fatalf("selected = %+v", selected)
	}
	if len(a.versions) != 1 || a.versions[0].Status != "draft" {
		t.Fatalf("versions = %+v", a.versions)
	}
	if want := fmt.Sprintf(locales.PlanSavedAsNew(), "Derived plan", 1); a.planMessage != want {
		t.Fatalf("message = %q", a.planMessage)
	}
}

func TestRetireSystemPlanKeepsPlanPageUsable(t *testing.T) {
	a := newTestApp(t)
	a.retireSelected()
	if a.planError != locales.PlanSystemCannotRetire() {
		t.Fatalf("planError = %q", a.planError)
	}
	if a.loadError != "" {
		t.Fatalf("loadError = %q", a.loadError)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Assessment plans") {
		t.Fatalf("texts = %q", tt.Texts())
	}
}

func TestLoadRecoversSystemPlanWithoutPublishedVersion(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "recover-")
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
	base, err := opened.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, `UPDATE plan_versions SET status = 'retired' WHERE id = ?`, base.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	a := &app{dataDir: dir, language: "en"}
	a.load()
	if a.loadError != "" {
		t.Fatal(a.loadError)
	}
	t.Cleanup(func() {
		if err := a.store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if len(a.plans) != 1 {
		t.Fatalf("plans = %+v", a.plans)
	}
	versions, err := a.store.ListVersions(ctx, a.plans[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %+v", versions)
	}
	if versions[1].Status != "published" {
		t.Fatalf("versions = %+v", versions)
	}
	latest, err := a.store.LatestPublished(ctx, a.plans[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != versions[1].ID {
		t.Fatalf("latest = %+v", latest)
	}
}
