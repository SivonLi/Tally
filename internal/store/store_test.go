package store_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"tally/internal/assessment"
	"tally/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), "..", "..", ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "store-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(dir, "tally.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return s
}

func TestEnsureDefaultPlanIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	first, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "published" || first.Version != 1 || first.Config.SchemaVersion != 1 {
		t.Fatalf("version = %+v", first)
	}
	second, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.PlanID != second.PlanID {
		t.Fatalf("default plan duplicated: %d %d", first.ID, second.ID)
	}
	plans, err := s.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Kind != "system" {
		t.Fatalf("plans = %+v", plans)
	}
}

func TestDerivedPlanVersions(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := assessment.DefaultConfig()
	config.Name = "并列不跳名次"
	config.Ranking.TieMode = "dense"
	plan, draft, err := s.CreatePlan(ctx, store.CreatePlanInput{
		Name:          config.Name,
		Kind:          "user",
		Description:   "从默认方案派生",
		BaseVersionID: base.ID,
		Config:        config,
		CreatedBy:     "教务处",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != "user" || plan.BasePlanVersionID != base.ID {
		t.Fatalf("plan = %+v", plan)
	}
	if draft.Status != "draft" || draft.Version != 1 {
		t.Fatalf("draft = %+v", draft)
	}
	if err := s.Publish(ctx, draft.ID, "教务处"); err != nil {
		t.Fatal(err)
	}
	published, err := s.GetVersion(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != "published" || published.PublishedAt == nil {
		t.Fatalf("published = %+v", published)
	}

	config.Ranking.TieMode = "ordinal"
	second, err := s.CreateDraft(ctx, store.CreateDraftInput{
		PlanID:        plan.ID,
		BaseVersionID: published.ID,
		Config:        config,
		Notes:         "改为顺序名次",
		CreatedBy:     "教务处",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 || second.Status != "draft" {
		t.Fatalf("second = %+v", second)
	}
	if err := s.Publish(ctx, second.ID, "教务处"); err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestPublished(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != second.ID {
		t.Fatalf("latest = %+v", latest)
	}
	if err := s.Retire(ctx, published.ID); err != nil {
		t.Fatal(err)
	}
	versions, err := s.ListVersions(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Status != "retired" || versions[1].Status != "published" {
		t.Fatalf("versions = %+v", versions)
	}
}

func TestCreatePlanRejectsInvalidFormula(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := assessment.DefaultConfig()
	config.Indicators[0].Formula = "class.missing"
	_, _, err = s.CreatePlan(ctx, store.CreatePlanInput{
		Name:          "无效方案",
		Kind:          "user",
		BaseVersionID: base.ID,
		Config:        config,
	})
	if err == nil {
		t.Fatal("invalid formula accepted")
	}
	plans, err := s.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 {
		t.Fatalf("plans = %+v", plans)
	}
}

func TestPublishRejectsPublishedVersion(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(ctx, base.ID, "教务处"); err == nil {
		t.Fatal("published version accepted")
	}
}
