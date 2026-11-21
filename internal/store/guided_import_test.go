package store_test

import (
	"context"
	"errors"
	"testing"

	"tally/internal/store"
)

func stageImportFixture(t *testing.T) (*store.Store, int64, int64) {
	t.Helper()
	ctx := context.Background()
	s, _ := openCohortStorePath(t)
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	return s, cohort.ID, stage.ID
}

func rosterReport(hash string) store.ImportReport {
	return store.ImportReport{
		FileHash: hash, Path: hash + ".xlsx", RowCount: 2,
		Sheets: []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 2}},
	}
}

func TestGuidedImportValidatesBeforeWriting(t *testing.T) {
	ctx := context.Background()
	s, cohortID, stageID := stageImportFixture(t)
	req := store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Name: "甲", ClassName: "1班", SourceRow: 2},
			{StudentNo: "001", Name: "乙", ClassName: "1班", SourceRow: 3},
		}},
		Report: rosterReport("blocked"),
	}
	preview, err := s.ValidateStageImport(ctx, cohortID, stageID, req)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Blocking != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if _, err := s.ApplyStageImport(ctx, cohortID, stageID, req); !errors.Is(err, store.ErrImportBlocked) {
		t.Fatalf("apply error = %v", err)
	}
	_, total, err := s.ListStudents(ctx, store.StudentFilter{StageID: stageID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("students written despite blocking issue: %d", total)
	}
}

func TestGuidedImportUndoRestoresPreviousValues(t *testing.T) {
	ctx := context.Background()
	s, cohortID, stageID := stageImportFixture(t)
	first := store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班"},
			Students: []store.ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "1班", SourceRow: 2},
				{StudentNo: "002", Name: "乙", ClassName: "1班", SourceRow: 3},
			},
		},
		Report: rosterReport("first"),
	}
	if _, err := s.ApplyStageImport(ctx, cohortID, stageID, first); err != nil {
		t.Fatal(err)
	}
	second := store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"2班"},
			Students: []store.ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "2班", SourceRow: 2},
			},
		},
		Report: rosterReport("second"),
	}
	if _, err := s.ApplyStageImport(ctx, cohortID, stageID, second); err != nil {
		t.Fatal(err)
	}
	students, _, err := s.ListStudents(ctx, store.StudentFilter{StageID: stageID, Query: "001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].ClassName != "2班" {
		t.Fatalf("students after update = %+v", students)
	}
	runs, err := s.ListImportRuns(ctx, stageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %+v", runs)
	}
	if err := s.UndoImportRun(ctx, runs[0].ID); err != nil {
		t.Fatal(err)
	}
	students, _, err = s.ListStudents(ctx, store.StudentFilter{StageID: stageID, Query: "001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].ClassName != "1班" {
		t.Fatalf("students after undo = %+v", students)
	}
	classes, err := s.ListClasses(ctx, cohortID, stageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 1 || classes[0].Name != "1班" {
		t.Fatalf("classes after undo = %+v", classes)
	}
	if err := s.UndoImportRun(ctx, runs[1].ID); err != nil {
		t.Fatal(err)
	}
	_, total, err := s.ListStudents(ctx, store.StudentFilter{StageID: stageID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("students after second undo = %d", total)
	}
	classes, err = s.ListClasses(ctx, cohortID, stageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 0 {
		t.Fatalf("classes after second undo = %+v", classes)
	}
}

func TestGuidedImportRejectsDuplicateFile(t *testing.T) {
	ctx := context.Background()
	s, cohortID, stageID := stageImportFixture(t)
	req := store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Name: "甲", ClassName: "1班", SourceRow: 2},
		}},
		Report: rosterReport("same"),
	}
	if _, err := s.ApplyStageImport(ctx, cohortID, stageID, req); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohortID, stageID, req); !errors.Is(err, store.ErrDuplicateImport) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestGuidedAssignmentImportBuildsTeachingGroups(t *testing.T) {
	ctx := context.Background()
	s, cohortID, stageID := stageImportFixture(t)
	roster := store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班"},
			Students: []store.ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "1班", SourceRow: 2},
			},
		},
		Report: rosterReport("roster"),
	}
	if _, err := s.ApplyStageImport(ctx, cohortID, stageID, roster); err != nil {
		t.Fatal(err)
	}
	assignments := store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Classes: []string{"1班"},
			Assignments: []store.ImportAssignment{
				{ClassName: "1班", Subject: "math", TeacherName: "李老师", SourceRow: 2},
			},
		},
		Report: store.ImportReport{
			FileHash: "assignments", Path: "assignments.xlsx", RowCount: 1,
			Sheets: []store.ImportSheet{{Name: "任课关系", Role: "assignments", HeaderRow: 1, Rows: 1}},
		},
	}
	summary, err := s.ApplyStageImport(ctx, cohortID, stageID, assignments)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TeachingGroups != 1 || summary.Assignments != 1 || summary.Teachers != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	groups, err := s.ListTeachingGroups(ctx, stageID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Students != 1 || groups[0].Teachers != "李老师" {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestGuidedBaselineImportRejectsUnknownClass(t *testing.T) {
	ctx := context.Background()
	s, cohortID, stageID := stageImportFixture(t)
	ratio := 1.1
	req := store.StageImportRequest{
		Kind: "baselines",
		Data: store.ImportData{ClassBaselines: []store.ImportClassBaseline{
			{ClassName: "9班", Subject: "chinese", Average: 100, Ratio: &ratio, SourceRow: 2},
		}},
		Report: store.ImportReport{
			FileHash: "baseline", Path: "baseline.xlsx", RowCount: 1,
			Sheets: []store.ImportSheet{{Name: "入口成绩", Role: "baselines", HeaderRow: 1, Rows: 1}},
		},
	}
	preview, err := s.ValidateStageImport(ctx, cohortID, stageID, req)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Blocking != 1 {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestGuidedBaselineImportUndoClearsStageRows(t *testing.T) {
	ctx := context.Background()
	s, cohortID, stageID := stageImportFixture(t)
	if _, err := s.ApplyStageImport(ctx, cohortID, stageID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班"},
			Students: []store.ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "1班", SourceRow: 2},
			},
		},
		Report: rosterReport("baseline-undo-roster"),
	}); err != nil {
		t.Fatal(err)
	}
	ratio := 1.1
	baseline := store.StageImportRequest{
		Kind: "baselines",
		Data: store.ImportData{ClassBaselines: []store.ImportClassBaseline{
			{ClassName: "1班", Subject: "chinese", Average: 100, Ratio: &ratio, SourceRow: 2},
		}},
		Report: store.ImportReport{
			FileHash: "baseline-undo", Path: "baseline.xlsx", RowCount: 1,
			Sheets: []store.ImportSheet{{Name: "入口成绩", Role: "baselines", HeaderRow: 1, Rows: 1}},
		},
	}
	if _, err := s.ApplyStageImport(ctx, cohortID, stageID, baseline); err != nil {
		t.Fatal(err)
	}
	counts, err := s.StageCounts(ctx, cohortID, 2026, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Baselines == 0 {
		t.Fatal("baseline rows were not written")
	}
	runs, err := s.ListImportRuns(ctx, stageID)
	if err != nil {
		t.Fatal(err)
	}
	ok, reason, err := s.CanUndoImport(ctx, runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("undo blocked: %s", reason)
	}
	if err := s.UndoImportRun(ctx, runs[0].ID); err != nil {
		t.Fatal(err)
	}
	counts, err = s.StageCounts(ctx, cohortID, 2026, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Baselines != 0 {
		t.Fatalf("baseline rows after undo = %d", counts.Baselines)
	}
}

func TestGuidedImportUndoRequiresChangeRecords(t *testing.T) {
	ctx := context.Background()
	s, cohortID, stageID := stageImportFixture(t)
	if _, err := s.MergeStageRoster(ctx, cohortID, stageID, store.ImportData{
		Students: []store.ImportStudent{
			{StudentNo: "001", Name: "甲", ClassName: "1班", SourceRow: 2},
		},
	}, rosterReport("legacy-roster")); err != nil {
		t.Fatal(err)
	}
	runs, err := s.ListImportRuns(ctx, stageID)
	if err != nil {
		t.Fatal(err)
	}
	ok, reason, err := s.CanUndoImport(ctx, runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok || reason != "no_changes" {
		t.Fatalf("ok = %v, reason = %q", ok, reason)
	}
	if err := s.UndoImportRun(ctx, runs[0].ID); !errors.Is(err, store.ErrImportUndoBlocked) {
		t.Fatalf("undo error = %v", err)
	}
}
