package store

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

func TestLegacyDatabaseMigratesSchoolData(t *testing.T) {
	ctx := context.Background()
	dir := migrationTestDir(t)
	path := filepath.Join(dir, "tally.db")
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	statements := []sqlite.Statement{
		{SQL: `CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`},
		{SQL: `INSERT INTO schema_migrations(version, applied_at) VALUES (1, ?), (3, ?)`,
			Args: []any{now, now}},
	}
	statements = append(statements, batchSchema()...)
	statements = append(statements, cohortSchema()...)
	statements = append(statements,
		sqlite.Statement{
			SQL:  `INSERT INTO terms(id, academic_year, semester) VALUES (1, '2026-2027', 'autumn')`,
			Args: nil,
		},
		sqlite.Statement{
			SQL: `INSERT INTO batches(id, term_id, exam_type, grade, status, revision, created_at)
				VALUES (1, 1, 'midterm', 'g1', 'imported', 1, ?)`, Args: []any{now},
		},
		sqlite.Statement{
			SQL:  `INSERT INTO classes(id, batch_id, name, headcount) VALUES (1, 1, '1班', 2), (2, 1, '2班', 1)`,
			Args: nil,
		},
		sqlite.Statement{
			SQL:  `INSERT INTO students(id, student_no, name) VALUES (1, '001', '甲'), (2, '002', '乙')`,
			Args: nil,
		},
		sqlite.Statement{
			SQL: `INSERT INTO student_classes(batch_id, student_id, class_id, track, status)
				VALUES (1, 1, 1, 'physics', 'active'), (1, 2, 1, 'history', 'active')`,
			Args: nil,
		},
		sqlite.Statement{
			SQL:  `INSERT INTO teachers(id, teacher_no, name) VALUES (10, '', '张老师')`,
			Args: nil,
		},
		sqlite.Statement{
			SQL: `INSERT INTO teaching_groups(id, batch_id, subject, name, class_id)
				VALUES (20, 1, 'chinese', '1班', 1), (21, 1, 'math', '1班', 1), (22, 1, 'chinese', '2班', 2)`,
			Args: nil,
		},
		sqlite.Statement{
			SQL:  `INSERT INTO assignments(teacher_id, teaching_group_id) VALUES (10, 20), (10, 22)`,
			Args: nil,
		},
		sqlite.Statement{
			SQL:  `INSERT INTO class_baselines(batch_id, class_id, subject, average, ratio) VALUES (1, 1, 'chinese', 82.5, 1.1)`,
			Args: nil,
		},
		sqlite.Statement{
			SQL:  `INSERT INTO grade_baselines(batch_id, subject, average) VALUES (1, 'chinese', 75)`,
			Args: nil,
		},
		sqlite.Statement{
			SQL: `INSERT INTO imports(batch_id, file_hash, relative_path, file_format, row_count, status, created_at)
				VALUES (1, 'hash-1', 'history/2026/student.xlsx', 'xlsx', 2, 'imported', ?)`, Args: []any{now},
		},
		sqlite.Statement{
			SQL: `INSERT INTO import_sheets(import_id, sheet_name, role, header_row, data_range, is_current)
				VALUES (1, '学生名单', 'students', 1, '2 rows', 1)`, Args: nil,
		},
	)
	if _, err := db.Transaction(ctx, statements); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { opened.Close() })
	cohorts, err := opened.ListCohorts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cohorts) != 1 || cohorts[0].EnrollmentYear != 2026 {
		t.Fatalf("cohorts = %+v", cohorts)
	}
	stage, err := opened.GetCohortStage(ctx, cohorts[0].ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	classes, err := opened.ListClasses(ctx, cohorts[0].ID, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 2 || classes[0].Students != 2 || classes[1].Students != 0 {
		t.Fatalf("classes = %+v", classes)
	}
	students, total, err := opened.ListStudents(ctx, StudentFilter{StageID: stage.ID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(students) != 2 || students[0].StudentNo != "001" {
		t.Fatalf("students = %+v total = %d", students, total)
	}
	teachers, err := opened.ListTeachers(ctx, stage.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(teachers) != 1 || teachers[0].Subject != "chinese" || teachers[0].Assignments != 2 {
		t.Fatalf("teachers = %+v", teachers)
	}
	groups, err := opened.ListTeachingGroups(ctx, stage.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 {
		t.Fatalf("groups = %+v", groups)
	}
	baselines, err := opened.ListBaselines(ctx, stage.ID, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines) != 1 || baselines[0].Average != 82.5 || baselines[0].Count != 2 {
		t.Fatalf("baselines = %+v", baselines)
	}
	runs, err := opened.ListImportRuns(ctx, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || len(runs[0].Sheets) != 1 {
		t.Fatalf("runs = %+v", runs)
	}
	counts, err := migrationCounts(ctx, opened)
	if err != nil {
		t.Fatal(err)
	}
	if counts["classes"] != 2 || counts["students"] != 2 || counts["teachers"] != 1 ||
		counts["teaching_groups"] != 3 || counts["assignments"] != 2 {
		t.Fatalf("counts = %+v", counts)
	}
	data, err := opened.LoadBatchData(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Classes) != 2 || len(data.Students) != 2 || len(data.TeachingGroups) != 3 {
		t.Fatalf("batch data = %+v", data)
	}
	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) == 0 {
		t.Fatal("migration did not create a backup")
	}
	if _, err := os.Stat(filepath.Join(backups[0].Path, "tally.db")); err != nil {
		t.Fatal(err)
	}
}

func TestOpenBacksUpOnlyWhenMigrationRuns(t *testing.T) {
	ctx := context.Background()
	dir := migrationTestDir(t)
	path := filepath.Join(dir, "tally.db")
	opened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("new database created %d backups", len(backups))
	}
	opened, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	backups, err = ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("reopening migrated database created %d backups", len(backups))
	}
}

func TestMigrationFailureRestoresDatabase(t *testing.T) {
	ctx := context.Background()
	dir := migrationTestDir(t)
	path := filepath.Join(dir, "tally.db")
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	statements := []sqlite.Statement{
		{SQL: `CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`},
		{SQL: `INSERT INTO schema_migrations(version, applied_at) VALUES (1, ?), (3, ?)`,
			Args: []any{now, now}},
	}
	statements = append(statements, batchSchema()...)
	statements = append(statements, cohortSchema()...)
	statements = append(statements,
		sqlite.Statement{SQL: `INSERT INTO terms(id, academic_year, semester) VALUES (1, '2026-2027', 'autumn')`},
		sqlite.Statement{SQL: `INSERT INTO batches(id, term_id, exam_type, grade, status, revision, created_at)
			VALUES (1, 1, 'midterm', 'g1', 'imported', 1, ?)`, Args: []any{now}},
		sqlite.Statement{SQL: `INSERT INTO classes(id, batch_id, name, headcount) VALUES (1, 1, '1班', 0), (2, 1, '   ', 1)`},
		sqlite.Statement{SQL: `INSERT INTO students(id, student_no, name) VALUES (1, '001', '甲')`},
		sqlite.Statement{SQL: `INSERT INTO student_classes(batch_id, student_id, class_id, status)
			VALUES (1, 1, 2, 'active')`},
	)
	if _, err := db.Transaction(ctx, statements); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); err == nil {
		t.Fatal("migration with an unnamed class succeeded")
	}
	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %d", len(backups))
	}
	restored, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	rows, err := restored.Query(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name IN ('classes', 'legacy_classes', 'teachers', 'legacy_teachers')
		ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(rows.Values))
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		names = append(names, name)
	}
	if len(names) != 2 || names[0] != "classes" || names[1] != "teachers" {
		t.Fatalf("tables after failed migration = %v", names)
	}
	versionRows, err := restored.Query(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`)
	if err != nil {
		t.Fatal(err)
	}
	version, _ := versionRows.Values[0][0].(int64)
	if version != 3 {
		t.Fatalf("schema version after failed migration = %d", version)
	}
}

func migrationTestDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	scratch := filepath.Join(filepath.Dir(file), "..", "..", ".scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(scratch, "migration-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})
	return dir
}
