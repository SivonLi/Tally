package store_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"tally/internal/school"
	"tally/internal/store"
)

func openCohortStore(t *testing.T) *store.Store {
	t.Helper()
	opened, _ := openCohortStorePath(t)
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return opened
}

func openCohortStorePath(t *testing.T) (*store.Store, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), "..", "..", ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "cohort-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})
	path := filepath.Join(dir, "tally.db")
	opened, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return opened, path
}

func TestCreateCohortAndStages(t *testing.T) {
	ctx := context.Background()
	s := openCohortStore(t)
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if cohort.Name != "2026级" || cohort.GraduationYear != 2029 || cohort.Status != "active" {
		t.Fatalf("cohort = %+v", cohort)
	}
	stages, err := s.ListCohortStages(ctx, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 1 || stages[0].GradeLevel != "g1" || stages[0].AcademicYearStart != 2026 {
		t.Fatalf("stages = %+v", stages)
	}
	if stages[0].ReviewStatus != "confirmed" {
		t.Fatalf("review status = %q", stages[0].ReviewStatus)
	}
	if _, err := s.CreateCohort(ctx, 2026, 2026); !errors.Is(err, store.ErrCohortExists) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := s.CreateCohort(ctx, 2027, 2026); !errors.Is(err, school.ErrEnrollmentYear) {
		t.Fatalf("future year error = %v", err)
	}
	second, err := s.EnsureCohortStage(ctx, cohort.ID, 2027, "g2")
	if err != nil {
		t.Fatal(err)
	}
	if second.GradeLevel != "g2" {
		t.Fatalf("stage = %+v", second)
	}
}

func TestCurrentAcademicYear(t *testing.T) {
	ctx := context.Background()
	s := openCohortStore(t)
	year, err := s.CurrentAcademicYear(ctx, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if year != 2026 {
		t.Fatalf("year = %d", year)
	}
	if err := s.SetAcademicYear(ctx, 2027); err != nil {
		t.Fatal(err)
	}
	year, err = s.CurrentAcademicYear(ctx, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if year != 2027 {
		t.Fatalf("year = %d", year)
	}
}

func TestLegacyBatchesProduceCohorts(t *testing.T) {
	ctx := context.Background()
	s, path := openCohortStorePath(t)
	if _, err := s.CreateBatch(ctx, store.CreateBatchInput{
		AcademicYear: "2025-2026",
		Semester:     "spring",
		ExamType:     "midterm",
		Grade:        "g1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { opened.Close() })
	cohorts, err := opened.ListCohorts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cohorts) != 1 {
		t.Fatalf("cohorts = %+v", cohorts)
	}
	if cohorts[0].EnrollmentYear != 2025 || cohorts[0].GraduationYear != 2028 {
		t.Fatalf("cohort = %+v", cohorts[0])
	}
	stages, err := opened.ListCohortStages(ctx, cohorts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 1 || stages[0].AcademicYearStart != 2025 || stages[0].GradeLevel != "g1" {
		t.Fatalf("stages = %+v", stages)
	}
}

func TestStageCounts(t *testing.T) {
	ctx := context.Background()
	s := openCohortStore(t)
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := s.CreateBatch(ctx, store.CreateBatchInput{
		AcademicYear: "2026-2027",
		Semester:     "autumn",
		ExamType:     "midterm",
		Grade:        "g1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.MergeImport(ctx, batch.ID, store.ImportData{
		Classes: []string{"1班"},
		Students: []store.ImportStudent{{
			StudentNo: "001",
			Name:      "甲",
			ClassName: "1班",
		}},
		Teachers: []string{"张老师"},
		Assignments: []store.ImportAssignment{{
			ClassName:   "1班",
			Subject:     "chinese",
			TeacherName: "张老师",
		}},
		ClassBaselines: []store.ImportClassBaseline{{
			ClassName: "1班",
			Subject:   "chinese",
			Average:   80,
		}},
	}, store.ImportReport{})
	if err != nil {
		t.Fatal(err)
	}
	counts, err := s.StageCounts(ctx, cohort.ID, 2026, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Classes != 1 || counts.Students != 1 || counts.Teachers != 1 ||
		counts.Assignments != 1 || counts.Baselines != 1 {
		t.Fatalf("counts = %+v", counts)
	}
	names, err := s.StageClassNames(ctx, cohort.ID, 2026, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "1班" {
		t.Fatalf("names = %+v", names)
	}
}
