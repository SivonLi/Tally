package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tally/internal/store"
)

func TestStageRosterImportAndQueries(t *testing.T) {
	ctx := context.Background()
	s, path := openCohortStorePath(t)
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	report := store.ImportReport{
		FileHash:   "abc",
		Path:       "student-list.xlsx",
		StoredPath: filepath.Join(filepath.Dir(path), "files", "abc.xlsx"),
		RowCount:   3,
		Sheets:     []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 3}},
	}
	summary, err := s.MergeStageRoster(ctx, cohort.ID, stage.ID, store.ImportData{
		Classes: []string{"1班", "2班"},
		Students: []store.ImportStudent{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", Status: "active", SourceSheet: "学生名单", SourceRow: 2},
			{StudentNo: "002", Name: "乙", ClassName: "1班", Track: "history", SourceSheet: "学生名单", SourceRow: 3},
			{StudentNo: "003", Name: "丙", ClassName: "2班", Status: "transferred", SourceSheet: "学生名单", SourceRow: 4},
		},
	}, report)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Students != 3 || summary.Classes != 2 {
		t.Fatalf("summary = %+v", summary)
	}
	classes, err := s.ListClasses(ctx, cohort.ID, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 2 {
		t.Fatalf("classes = %+v", classes)
	}
	students, total, err := s.ListStudents(ctx, store.StudentFilter{StageID: stage.ID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(students) != 3 {
		t.Fatalf("students = %+v total = %d", students, total)
	}
	filtered, total, err := s.ListStudents(ctx, store.StudentFilter{
		StageID: stage.ID,
		Query:   "002",
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(filtered) != 1 || filtered[0].Name != "乙" {
		t.Fatalf("filtered = %+v total = %d", filtered, total)
	}
	if filtered[0].SourceSheet != "学生名单" || filtered[0].SourceRow != 3 {
		t.Fatalf("source = %+v", filtered[0])
	}
	transferred, total, err := s.ListStudents(ctx, store.StudentFilter{
		StageID: stage.ID,
		Status:  "transferred",
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(transferred) != 1 || transferred[0].StudentNo != "003" {
		t.Fatalf("transferred = %+v total = %d", transferred, total)
	}
	if _, err := s.MergeStageRoster(ctx, cohort.ID, stage.ID, store.ImportData{
		Students: []store.ImportStudent{
			{StudentNo: "001", Name: "甲", ClassName: "1班"},
		},
	}, report); err != nil {
		t.Fatal(err)
	}
	_, total, err = s.ListStudents(ctx, store.StudentFilter{StageID: stage.ID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("duplicate import produced %d students", total)
	}
	runs, err := s.ListImportRuns(ctx, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || len(runs[0].Sheets) != 1 || runs[0].Sheets[0].Name != "学生名单" {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestStoreImportFileByHash(t *testing.T) {
	s, path := openCohortStorePath(t)
	dir := filepath.Dir(path)
	source := filepath.Join(dir, "source.xlsx")
	if err := os.WriteFile(source, []byte("workbook"), 0o644); err != nil {
		t.Fatal(err)
	}
	stored, err := s.StoreImportFile(source, "content-hash")
	if err != nil {
		t.Fatal(err)
	}
	if stored != filepath.Join(dir, "files", "content-hash.xlsx") {
		t.Fatalf("stored = %q", stored)
	}
	data, err := os.ReadFile(stored)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "workbook" {
		t.Fatalf("data = %q", data)
	}
	if _, err := s.StoreImportFile(source, "content-hash"); err != nil {
		t.Fatal(err)
	}
}

func TestBackupListsAndPrunes(t *testing.T) {
	ctx := context.Background()
	s, path := openCohortStorePath(t)
	if _, err := s.CreateCohort(ctx, 2026, 2026); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	backup, err := store.CreateBackup(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(backup, "tally.db")); err != nil {
		t.Fatal(err)
	}
	backups, err := store.ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) == 0 {
		t.Fatal("no backups")
	}
}

func TestStageAssignmentAndBaselineImports(t *testing.T) {
	ctx := context.Background()
	s, path := openCohortStorePath(t)
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	roster := store.ImportReport{
		FileHash: "roster", Path: "roster.xlsx",
		StoredPath: filepath.Join(dir, "files", "roster.xlsx"),
		RowCount:   2,
		Sheets:     []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 2}},
	}
	if _, err := s.MergeStageRoster(ctx, cohort.ID, stage.ID, store.ImportData{
		Students: []store.ImportStudent{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Status: "active", SourceSheet: "学生名单", SourceRow: 2},
			{StudentNo: "002", Name: "乙", ClassName: "1班", Status: "active", SourceSheet: "学生名单", SourceRow: 3},
		},
	}, roster); err != nil {
		t.Fatal(err)
	}
	assignmentReport := store.ImportReport{
		FileHash: "assignments", Path: "assignments.xlsx",
		StoredPath: filepath.Join(dir, "files", "assignments.xlsx"),
		RowCount:   2,
		Sheets:     []store.ImportSheet{{Name: "任课教师", Role: "teachers", HeaderRow: 1, Rows: 2}},
	}
	summary, err := s.MergeStageAssignments(ctx, cohort.ID, stage.ID, store.ImportData{
		Assignments: []store.ImportAssignment{
			{ClassName: "1班", Subject: "chinese", TeacherName: "张老师", SourceSheet: "任课教师", SourceRow: 2},
			{ClassName: "1班", Subject: "math", TeacherName: "李老师", SourceSheet: "任课教师", SourceRow: 3},
		},
	}, assignmentReport)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Teachers != 2 || summary.TeachingGroups != 2 || summary.Assignments != 2 {
		t.Fatalf("summary = %+v", summary)
	}
	teachers, err := s.ListTeachers(ctx, stage.ID, "张")
	if err != nil {
		t.Fatal(err)
	}
	if len(teachers) != 1 || teachers[0].Subject != "chinese" || teachers[0].ImportRunID == 0 {
		t.Fatalf("teachers = %+v", teachers)
	}
	groups, err := s.ListTeachingGroups(ctx, stage.ID, "math")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Students != 2 || groups[0].Teachers != "李老师" {
		t.Fatalf("groups = %+v", groups)
	}
	students, _, err := s.ListStudents(ctx, store.StudentFilter{StageID: stage.ID, Query: "001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 {
		t.Fatalf("students = %+v", students)
	}
	student, err := s.GetStudent(ctx, stage.ID, students[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(student.TeachingGroups) != 2 {
		t.Fatalf("teaching groups = %+v", student.TeachingGroups)
	}
	baselineReport := store.ImportReport{
		FileHash: "baseline", Path: "baseline.xlsx",
		StoredPath: filepath.Join(dir, "files", "baseline.xlsx"),
		RowCount:   1,
		Sheets:     []store.ImportSheet{{Name: "基数", Role: "baselines", HeaderRow: 1, Rows: 1}},
	}
	ratio := 1.2
	summary, err = s.MergeStageBaselines(ctx, cohort.ID, stage.ID, store.ImportData{
		ClassBaselines: []store.ImportClassBaseline{
			{ClassName: "1班", Subject: "chinese", Average: 82.5, Ratio: &ratio, SourceSheet: "基数", SourceRow: 2},
		},
		GradeBaselines: map[string]float64{"chinese": 75},
	}, baselineReport)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Baselines != 2 {
		t.Fatalf("summary = %+v", summary)
	}
	classes, err := s.ListClasses(ctx, cohort.ID, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	baselines, err := s.ListBaselines(ctx, stage.ID, classes[0].ID, "chinese")
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines) != 1 || baselines[0].Average != 82.5 || baselines[0].Count != 2 ||
		baselines[0].SourceSheet != "基数" || baselines[0].SourceRow != 2 {
		t.Fatalf("baselines = %+v", baselines)
	}
	runs, err := s.ListImportRuns(ctx, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("runs = %+v", runs)
	}
}
