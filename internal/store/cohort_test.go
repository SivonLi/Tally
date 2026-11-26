package store_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/school"
	"tally/internal/store"
)

func openCohortStore(t *testing.T) *store.Store {
	t.Helper()
	opened, _ := openCohortStorePath(t)
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
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
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

func TestLegacyDatabaseIsRejectedCleanly(t *testing.T) {
	ctx := context.Background()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), "..", "..", ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "legacy-reject-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	path := filepath.Join(dir, "legacy.db")
	rawDB, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.Execute(ctx, `CREATE TABLE batches (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(ctx, path); !errors.Is(err, store.ErrIncompatibleDatabase) {
		t.Fatalf("expected ErrIncompatibleDatabase, got %v", err)
	}
}

func TestStageCounts(t *testing.T) {
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
			Classes: []string{"1班"},
			Students: []store.ImportStudent{{
				StudentNo: "001",
				Name:      "甲",
				ClassName: "1班",
			}},
		},
		Report: store.ImportReport{FileHash: "cohort-test-students"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Assignments: []store.ImportAssignment{{
				ClassName:   "1班",
				Subject:     "chinese",
				TeacherName: "张老师",
			}},
		},
		Report: store.ImportReport{FileHash: "cohort-test-assign"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "baselines",
		Data: store.ImportData{
			ClassBaselines: []store.ImportClassBaseline{{
				ClassName: "1班",
				Subject:   "chinese",
				Average:   80,
			}},
		},
		Report: store.ImportReport{FileHash: "cohort-test-base"},
	}); err != nil {
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
