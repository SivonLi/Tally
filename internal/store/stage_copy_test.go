package store_test

import (
	"context"
	"testing"

	"tally/internal/store"
)

func TestCopyStageFromPreviousCopiesRelationsAndMarksReview(t *testing.T) {
	ctx := context.Background()
	s := openCohortStore(t)
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班", "2班"},
			Students: []store.ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "1班", SourceRow: 2},
				{StudentNo: "002", Name: "乙", ClassName: "2班", SourceRow: 3},
			},
		},
		Report: rosterReport("copy-roster"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Classes: []string{"1班", "2班"},
			Assignments: []store.ImportAssignment{
				{ClassName: "1班", Subject: "math", TeacherName: "李老师", SourceRow: 2},
				{ClassName: "2班", Subject: "chinese", TeacherName: "张老师", SourceRow: 3},
			},
		},
		Report: store.ImportReport{
			FileHash: "copy-assignments", Path: "assignments.xlsx", RowCount: 2,
			Sheets: []store.ImportSheet{{Name: "任课关系", Role: "assignments", HeaderRow: 1, Rows: 2}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	next, err := s.EnsureCohortStage(ctx, cohort.ID, 2027, "g2")
	if err != nil {
		t.Fatal(err)
	}
	summary, err := s.CopyStageFromPrevious(ctx, cohort.ID, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.FromStageID != stage.ID || summary.Classes != 2 ||
		summary.TeachingGroups != 2 || summary.Assignments != 2 || summary.Students != 2 {
		t.Fatalf("summary = %+v", summary)
	}
	copied, err := s.GetCohortStage(ctx, cohort.ID, 2027)
	if err != nil {
		t.Fatal(err)
	}
	if copied.ReviewStatus != "pending" || copied.CopiedFromStageID == nil ||
		*copied.CopiedFromStageID != stage.ID {
		t.Fatalf("copied stage = %+v", copied)
	}
	if err := s.ConfirmStageReview(ctx, copied.ID); err != nil {
		t.Fatal(err)
	}
	confirmed, err := s.GetCohortStage(ctx, cohort.ID, 2027)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.ReviewStatus != "confirmed" || confirmed.ConfirmedAt == nil {
		t.Fatalf("confirmed stage = %+v", confirmed)
	}
}
