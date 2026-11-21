package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"tally/internal/assessment"
)

func openSystemPlanStore(t *testing.T) *Store {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), "..", "..", ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "system-plan-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})
	opened, err := Open(context.Background(), filepath.Join(dir, "tally.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return opened
}

func TestRetireRejectsSystemPlan(t *testing.T) {
	ctx := context.Background()
	s := openSystemPlanStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, base.ID); !errors.Is(err, ErrSystemPlanRetire) {
		t.Fatalf("Retire error = %v", err)
	}
	current, err := s.GetVersion(ctx, base.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "published" {
		t.Fatalf("status = %q", current.Status)
	}
}

func TestUpdatePlanAppendsPublishedSystemVersion(t *testing.T) {
	ctx := context.Background()
	s := openSystemPlanStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := base.Config
	config.Description = "学校调整后的规则"
	config.Ranking.TieMode = "dense"
	updated, err := s.UpdatePlan(ctx, base.PlanID, config, "调整并列规则", "教务处")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "published" || updated.Version != base.Version+1 {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.ParentVersionID != base.ID || updated.PublishedAt == nil {
		t.Fatalf("updated = %+v", updated)
	}
	previous, err := s.GetVersion(ctx, base.ID)
	if err != nil {
		t.Fatal(err)
	}
	if previous.Status != "published" {
		t.Fatalf("previous status = %q", previous.Status)
	}
	latest, err := s.LatestPublished(ctx, base.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != updated.ID {
		t.Fatalf("latest = %+v", latest)
	}
}

func TestUpdatePlanCreatesDraftForDerivedPlan(t *testing.T) {
	ctx := context.Background()
	s := openSystemPlanStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := assessment.DefaultConfig()
	config.Name = "派生方案"
	plan, draft, err := s.CreatePlan(ctx, CreatePlanInput{
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
	if err := s.Publish(ctx, draft.ID, "教务处"); err != nil {
		t.Fatal(err)
	}
	published, err := s.GetVersion(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	config.Ranking.TieMode = "ordinal"
	updated, err := s.UpdatePlan(ctx, plan.ID, config, "改为顺序名次", "教务处")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "draft" || updated.Version != published.Version+1 {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.ParentVersionID != published.ID {
		t.Fatalf("parent = %d", updated.ParentVersionID)
	}
	current, err := s.LatestPublished(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != published.ID {
		t.Fatalf("published changed to %+v", current)
	}
}

func TestUpdatePlanRejectsInvalidConfigWithoutWritingVersion(t *testing.T) {
	ctx := context.Background()
	s := openSystemPlanStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := base.Config
	config.Indicators[0].Formula = "class.missing"
	if _, err := s.UpdatePlan(ctx, base.PlanID, config, "", "教务处"); err == nil {
		t.Fatal("invalid config accepted")
	}
	versions, err := s.ListVersions(ctx, base.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("versions = %+v", versions)
	}
}

func TestEnsureDefaultPlanRestoresMissingPublishedVersion(t *testing.T) {
	ctx := context.Background()
	s := openSystemPlanStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := base.Config
	config.Description = "更新后的说明"
	updated, err := s.UpdatePlan(ctx, base.PlanID, config, "", "教务处")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Execute(ctx, `UPDATE plan_versions SET status = 'retired' WHERE plan_id = ?`, base.PlanID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != "published" || restored.Version != updated.Version+1 {
		t.Fatalf("restored = %+v", restored)
	}
	if restored.Config.Description != config.Description {
		t.Fatalf("description = %q", restored.Config.Description)
	}
	versions, err := s.ListVersions(ctx, base.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("versions = %+v", versions)
	}
	if versions[0].Status != "retired" || versions[1].Status != "retired" {
		t.Fatalf("versions = %+v", versions)
	}
}

func TestEnsureDefaultPlanRestoresFactoryConfigWithoutVersions(t *testing.T) {
	ctx := context.Background()
	s := openSystemPlanStore(t)
	base, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Execute(ctx, `DELETE FROM plan_versions WHERE plan_id = ?`, base.PlanID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != "published" || restored.Version != 1 {
		t.Fatalf("restored = %+v", restored)
	}
	if !reflect.DeepEqual(restored.Config, assessment.DefaultConfig()) {
		t.Fatalf("config = %+v", restored.Config)
	}
}
