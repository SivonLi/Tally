package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/internal/assessment"
	"tally/internal/store"
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
	a := &app{store: opened, dataDir: dir, language: "en", sidebarWidth: 260, batchSidebarWidth: 240}
	if _, err := opened.EnsureDefaultPlan(ctx); err != nil {
		t.Fatal(err)
	}
	a.reloadPlans(ctx)
	return a
}

func TestViewShowsDefaultPlan(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 960, 640)
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
	tt := ui.NewTester(a.view, 960, 640)
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
	tt := ui.NewTester(a.view, 960, 640)
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
	tt := ui.NewTester(a.view, 960, 640)
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
