package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"tally/internal/store"
)

func TestSavedViewsCRUD(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "tally.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	// 1. List on empty
	views, err := st.ListViews(ctx, "analysis")
	if err != nil {
		t.Fatalf("ListViews on empty: %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("expected 0 views, got %d", len(views))
	}

	// 2. SaveView insert
	id1, err := st.SaveView(ctx, store.SavedView{
		Page:        "analysis",
		Name:        "高一语文趋势",
		FiltersJSON: `{"subject":"chinese","tab":"teacher"}`,
	})
	if err != nil {
		t.Fatalf("SaveView 1: %v", err)
	}
	if id1 <= 0 {
		t.Fatalf("expected positive ID, got %d", id1)
	}

	id2, err := st.SaveView(ctx, store.SavedView{
		Page:        "analysis",
		Name:        "数学班级对比",
		FiltersJSON: `{"subject":"math","tab":"class"}`,
	})
	if err != nil {
		t.Fatalf("SaveView 2: %v", err)
	}
	if id2 <= 0 {
		t.Fatalf("expected positive ID, got %d", id2)
	}

	// 3. ListViews
	views, err = st.ListViews(ctx, "analysis")
	if err != nil {
		t.Fatalf("ListViews after insert: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(views))
	}

	// 4. Update view by ID
	_, err = st.SaveView(ctx, store.SavedView{
		ID:          id1,
		Page:        "analysis",
		Name:        "高一语文趋势（更新）",
		FiltersJSON: `{"subject":"chinese","tab":"teacher","updated":true}`,
	})
	if err != nil {
		t.Fatalf("Update view: %v", err)
	}

	views, err = st.ListViews(ctx, "analysis")
	if err != nil {
		t.Fatalf("ListViews after update: %v", err)
	}
	found := false
	for _, v := range views {
		if v.ID == id1 && v.Name == "高一语文趋势（更新）" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("updated view not found in views: %+v", views)
	}

	// 5. DeleteView
	if err := st.DeleteView(ctx, id1); err != nil {
		t.Fatalf("DeleteView: %v", err)
	}
	views, err = st.ListViews(ctx, "analysis")
	if err != nil {
		t.Fatalf("ListViews after delete: %v", err)
	}
	if len(views) != 1 || views[0].ID != id2 {
		t.Fatalf("expected only view 2 left, got %+v", views)
	}
}
