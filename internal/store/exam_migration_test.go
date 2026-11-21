package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/assessment"
)

func legacyExamTables() []sqlite.Statement {
	return []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS terms (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			academic_year TEXT NOT NULL,
			semester TEXT NOT NULL,
			UNIQUE(academic_year, semester)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS batches (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			term_id INTEGER NOT NULL REFERENCES terms(id),
			exam_type TEXT NOT NULL,
			grade TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			current_plan_version_id INTEGER,
			revision INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			UNIQUE(term_id, exam_type, grade)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			student_id INTEGER NOT NULL,
			subject TEXT NOT NULL,
			score REAL NOT NULL,
			flag TEXT NOT NULL DEFAULT '',
			UNIQUE(batch_id, student_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS subject_cutoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			subject TEXT NOT NULL,
			track TEXT NOT NULL,
			cutoff REAL NOT NULL,
			UNIQUE(batch_id, subject, track)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS tier_cutoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			tier_key TEXT NOT NULL,
			track TEXT NOT NULL,
			cutoff REAL NOT NULL,
			UNIQUE(batch_id, tier_key, track)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS class_targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			class_id INTEGER NOT NULL,
			tier_key TEXT NOT NULL,
			target_count REAL NOT NULL,
			UNIQUE(batch_id, class_id, tier_key)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS stat_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			subject TEXT NOT NULL,
			name TEXT NOT NULL,
			lead_class_id INTEGER NOT NULL,
			target REAL NOT NULL DEFAULT 0,
			baseline_average REAL NOT NULL DEFAULT 0,
			baseline_ratio REAL NOT NULL DEFAULT 0,
			explicit INTEGER NOT NULL DEFAULT 0,
			UNIQUE(batch_id, subject, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS stat_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			stat_group_id INTEGER NOT NULL,
			teaching_group_id INTEGER NOT NULL,
			UNIQUE(stat_group_id, teaching_group_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS target_adjustments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			class_id INTEGER NOT NULL,
			tier_key TEXT NOT NULL DEFAULT '',
			old_value REAL,
			new_value REAL,
			reason TEXT NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS result_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			plan_version_id INTEGER NOT NULL,
			input_hash TEXT NOT NULL,
			is_official INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`},
	}
}

func TestVersionFourDatabaseMigratesExamAndResultLinks(t *testing.T) {
	ctx := context.Background()
	dir := migrationTestDir(t)
	path := filepath.Join(dir, "tally.db")
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	config := assessment.DefaultConfig()
	configJSON, hash, err := encodeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	statements := []sqlite.Statement{
		{SQL: `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`},
		{SQL: `INSERT INTO schema_migrations(version, applied_at) VALUES (1, ?), (2, ?), (3, ?), (4, ?)`,
			Args: []any{now, now, now, now}},
		{SQL: `CREATE TABLE students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE plans (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL,
			kind TEXT NOT NULL, base_plan_version_id INTEGER, description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL)`},
		{SQL: `CREATE TABLE plan_versions (id INTEGER PRIMARY KEY AUTOINCREMENT, plan_id INTEGER NOT NULL,
			version INTEGER NOT NULL, status TEXT NOT NULL, schema_version INTEGER NOT NULL,
			config_json TEXT NOT NULL, config_hash TEXT NOT NULL, parent_version_id INTEGER,
			notes TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, created_by TEXT NOT NULL DEFAULT '',
			published_at TEXT, published_by TEXT NOT NULL DEFAULT '')`},
		{SQL: `INSERT INTO plans(id, name, kind, description, created_at)
			VALUES (1, '默认方案', 'system', '', ?)`, Args: []any{now}},
		{SQL: `INSERT INTO plan_versions(id, plan_id, version, status, schema_version, config_json,
			config_hash, notes, created_at, published_at)
			VALUES (1, 1, 1, 'published', ?, ?, ?, '', ?, ?)`,
			Args: []any{config.SchemaVersion, configJSON, hash, now, now}},
	}
	statements = append(statements, cohortSchema()...)
	statements = append(statements, schoolDataSchema()...)
	statements = append(statements, legacyExamTables()...)
	statements = append(statements,
		sqlite.Statement{SQL: `INSERT INTO cohorts(id, name, enrollment_year, graduation_year, status, created_at)
			VALUES (1, '2026级', 2026, 2029, 'active', ?)`, Args: []any{now}},
		sqlite.Statement{SQL: `INSERT INTO cohort_stages(id, cohort_id, academic_year_start, grade_level, review_status)
			VALUES (1, 1, 2026, 'g1', 'confirmed')`},
		sqlite.Statement{SQL: `INSERT INTO classes(id, cohort_id, name, created_at) VALUES (1, 1, '1班', ?)`,
			Args: []any{now}},
		sqlite.Statement{SQL: `INSERT INTO class_stage_info(class_id, stage_id) VALUES (1, 1)`},
		sqlite.Statement{SQL: `INSERT INTO students(id, student_no, name) VALUES (1, '001', '甲')`},
		sqlite.Statement{SQL: `INSERT INTO student_stage_memberships(student_id, stage_id, class_id, track)
			VALUES (1, 1, 1, 'physics')`},
		sqlite.Statement{SQL: `INSERT INTO terms(id, academic_year, semester) VALUES (1, '2026-2027', 'autumn')`},
		sqlite.Statement{SQL: `INSERT INTO batches(id, term_id, exam_type, grade, status,
			current_plan_version_id, revision, created_at)
			VALUES (1, 1, 'midterm', 'g1', 'calculated', 1, 2, ?)`, Args: []any{now}},
		sqlite.Statement{SQL: `INSERT INTO scores(batch_id, student_id, subject, score) VALUES (1, 1, 'chinese', 90)`},
		sqlite.Statement{SQL: `INSERT INTO subject_cutoffs(batch_id, subject, track, cutoff)
			VALUES (1, 'chinese', 'physics', 80)`},
		sqlite.Statement{SQL: `INSERT INTO class_targets(batch_id, class_id, tier_key, target_count)
			VALUES (1, 1, 'undergraduate', 5)`},
		sqlite.Statement{SQL: `INSERT INTO result_runs(id, batch_id, plan_version_id, input_hash,
			is_official, created_at) VALUES (1, 1, 1, 'hash', 1, ?)`, Args: []any{now}},
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
	exams, err := opened.ListExams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exams) != 1 || len(exams[0].Cohorts) != 1 {
		t.Fatalf("exams = %+v", exams)
	}
	examCohort := exams[0].Cohorts[0]
	if examCohort.RosterCount != 1 || examCohort.ScoreCount != 1 || examCohort.ClassTargetCount != 1 {
		t.Fatalf("exam cohort = %+v", examCohort)
	}
	assessment, err := opened.AssessmentForExamCohort(ctx, examCohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.PlanVersion != 1 || assessment.Status != "calculated" || assessment.Revision != 2 {
		t.Fatalf("assessment = %+v", assessment)
	}
	if assessment.CurrentRunID != 1 {
		t.Fatalf("current run = %d", assessment.CurrentRunID)
	}
	rows, err := opened.db.Query(ctx, `SELECT assessment_id FROM result_runs WHERE id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) == 0 {
		t.Fatal("result run missing")
	}
	linked, _ := rows.Values[0][0].(int64)
	if linked != assessment.ID {
		t.Fatalf("result run assessment = %d, want %d", linked, assessment.ID)
	}
}

func TestVersionFourMigrationCarriesCutoffsTargetsAndStatGroups(t *testing.T) {
	ctx := context.Background()
	dir := migrationTestDir(t)
	path := filepath.Join(dir, "tally.db")
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	statements := []sqlite.Statement{
		{SQL: `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`},
		{SQL: `INSERT INTO schema_migrations(version, applied_at) VALUES (1, ?), (2, ?), (3, ?), (4, ?)`,
			Args: []any{now, now, now, now}},
		{SQL: `CREATE TABLE students (id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL UNIQUE, name TEXT NOT NULL)`},
		{SQL: `CREATE TABLE plans (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL,
			kind TEXT NOT NULL, base_plan_version_id INTEGER, description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL)`},
		{SQL: `CREATE TABLE plan_versions (id INTEGER PRIMARY KEY AUTOINCREMENT, plan_id INTEGER NOT NULL,
			version INTEGER NOT NULL, status TEXT NOT NULL, schema_version INTEGER NOT NULL,
			config_json TEXT NOT NULL, config_hash TEXT NOT NULL, parent_version_id INTEGER,
			notes TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, created_by TEXT NOT NULL DEFAULT '',
			published_at TEXT, published_by TEXT NOT NULL DEFAULT '')`},
	}
	statements = append(statements, cohortSchema()...)
	statements = append(statements, schoolDataSchema()...)
	statements = append(statements, legacyExamTables()...)
	statements = append(statements,
		sqlite.Statement{SQL: `INSERT INTO subjects("key", name, full_mark, sort_order, enabled)
			VALUES ('chinese', '语文', 150, 1, 1), ('math', '数学', 150, 2, 1)`},
		sqlite.Statement{SQL: `INSERT INTO tracks("key", name, sort_order, enabled)
			VALUES ('physics', '物理类', 1, 1), ('history', '历史类', 2, 1)`},
		sqlite.Statement{SQL: `INSERT INTO plans(id, name, kind, description, created_at)
			VALUES (1, '默认方案', 'system', '', ?)`, Args: []any{now}},
		sqlite.Statement{SQL: `INSERT INTO cohorts(id, name, enrollment_year, graduation_year, status, created_at)
			VALUES (1, '2026级', 2026, 2029, 'active', ?)`, Args: []any{now}},
		sqlite.Statement{SQL: `INSERT INTO cohort_stages(id, cohort_id, academic_year_start, grade_level, review_status)
			VALUES (1, 1, 2026, 'g1', 'confirmed')`},
		sqlite.Statement{SQL: `INSERT INTO classes(id, cohort_id, name, created_at)
			VALUES (1, 1, '1班', ?), (2, 1, '2班', ?)`, Args: []any{now, now}},
		sqlite.Statement{SQL: `INSERT INTO students(id, student_no, name)
			VALUES (1, '001', '甲'), (2, '002', '乙')`},
		sqlite.Statement{SQL: `INSERT INTO student_stage_memberships(student_id, stage_id, class_id, track)
			VALUES (1, 1, 1, 'physics'), (2, 1, 1, 'history')`},
		sqlite.Statement{SQL: `INSERT INTO teaching_groups(id, stage_id, subject, name)
			VALUES (10, 1, 'chinese', '1班'), (11, 1, 'chinese', '2班')`},
		sqlite.Statement{SQL: `INSERT INTO student_teaching_groups(student_id, stage_id, subject, teaching_group_id)
			VALUES (1, 1, 'chinese', 10), (1, 1, 'math', 10), (2, 1, 'chinese', 10)`},
		sqlite.Statement{SQL: `INSERT INTO terms(id, academic_year, semester) VALUES (1, '2026-2027', 'autumn')`},
		sqlite.Statement{SQL: `INSERT INTO batches(id, term_id, exam_type, grade, status,
			current_plan_version_id, revision, created_at)
			VALUES (1, 1, 'midterm', 'g1', 'calculated', NULL, 1, ?)`, Args: []any{now}},
		sqlite.Statement{SQL: `INSERT INTO scores(batch_id, student_id, subject, score, flag)
			VALUES (1, 1, 'chinese', 95, ''), (1, 1, 'math', 0, 'absent')`},
		sqlite.Statement{SQL: `INSERT INTO subject_cutoffs(batch_id, subject, track, cutoff)
			VALUES (1, 'chinese', 'physics', 85.5)`},
		sqlite.Statement{SQL: `INSERT INTO tier_cutoffs(batch_id, tier_key, track, cutoff)
			VALUES (1, 'undergraduate', 'physics', 480)`},
		sqlite.Statement{SQL: `INSERT INTO class_targets(batch_id, class_id, tier_key, target_count)
			VALUES (1, 1, 'undergraduate', 20), (1, 1, 'single_subject', 7)`},
		sqlite.Statement{SQL: `INSERT INTO stat_groups(id, batch_id, subject, name, lead_class_id,
			target, baseline_average, baseline_ratio, explicit)
			VALUES (30, 1, 'chinese', '语文合并组', 1, 12, 88.5, 1.05, 1)`},
		sqlite.Statement{SQL: `INSERT INTO stat_group_members(stat_group_id, teaching_group_id)
			VALUES (30, 10), (30, 11)`},
		sqlite.Statement{SQL: `INSERT INTO target_adjustments(batch_id, class_id, tier_key,
			old_value, new_value, reason, created_at)
			VALUES (1, 1, 'undergraduate', 20, 18, '学校调整', ?)`, Args: []any{now}},
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
	exams, err := opened.ListExams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exams) != 1 || len(exams[0].Cohorts) != 1 {
		t.Fatalf("exams = %+v", exams)
	}
	examCohort := exams[0].Cohorts[0]
	if examCohort.RosterCount != 2 || examCohort.ScoreCount != 2 ||
		examCohort.ClassTargetCount != 2 {
		t.Fatalf("exam cohort = %+v", examCohort)
	}
	subjects, err := opened.ListExamSubjects(ctx, exams[0].ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) == 0 {
		t.Fatal("no migrated exam subjects")
	}
	subjectCutoffs, err := opened.ListExamSubjectCutoffs(ctx, exams[0].ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjectCutoffs) != 1 || subjectCutoffs[0].Cutoff != 85.5 ||
		subjectCutoffs[0].Track != "physics" {
		t.Fatalf("subject cutoffs = %+v", subjectCutoffs)
	}
	tierCutoffs, err := opened.ListExamTierCutoffs(ctx, exams[0].ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tierCutoffs) != 1 || tierCutoffs[0].Cutoff != 480 {
		t.Fatalf("tier cutoffs = %+v", tierCutoffs)
	}
	roster, _, err := opened.ListExamRoster(ctx, ExamRosterFilter{ExamID: exams[0].ID, CohortID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 2 {
		t.Fatalf("roster = %+v", roster)
	}
	var subjectsList []string
	if err := json.Unmarshal([]byte(rosterSubjectsJSON(t, opened, exams[0].ID, 1, 1)), &subjectsList); err != nil {
		t.Fatalf("subjects json is not valid: %v", err)
	}
	if len(subjectsList) != 2 || subjectsList[0] != "chinese" || subjectsList[1] != "math" {
		t.Fatalf("subjects json = %v", subjectsList)
	}
	assessment, err := opened.AssessmentForExamCohort(ctx, examCohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := opened.ListClassTargets(ctx, assessment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %+v", targets)
	}
	groups, err := opened.ListExamStatGroups(ctx, assessment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	group := groups[0]
	if group.Name != "语文合并组" || group.LeadClassID != 1 || group.Target == nil ||
		*group.Target != 12 || group.BaselineAverage != 88.5 || len(group.Members) != 2 {
		t.Fatalf("group = %+v", group)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	exams, err = reopened.ListExams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exams) != 1 || len(exams[0].Cohorts) != 1 {
		t.Fatalf("exams after reopen = %+v", exams)
	}
	if exams[0].Cohorts[0].RosterCount != 2 || exams[0].Cohorts[0].ScoreCount != 2 ||
		exams[0].Cohorts[0].ClassTargetCount != 2 {
		t.Fatalf("exam cohort after reopen = %+v", exams[0].Cohorts[0])
	}
	assessment, err = reopened.AssessmentForExamCohort(ctx, exams[0].Cohorts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	groups, err = reopened.ListExamStatGroups(ctx, assessment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatalf("groups after reopen = %+v", groups)
	}
	adjustmentRows, err := reopened.db.Query(ctx, `SELECT COUNT(*) FROM assessment_target_adjustments
		WHERE assessment_id = ?`, assessment.ID)
	if err != nil {
		t.Fatal(err)
	}
	adjustments, _ := adjustmentRows.Values[0][0].(int64)
	if adjustments != 1 {
		t.Fatalf("target adjustments = %d", adjustments)
	}
}

func rosterSubjectsJSON(t *testing.T, s *Store, examID, cohortID, studentID int64) string {
	t.Helper()
	rows, err := s.db.Query(context.Background(), `SELECT subjects_json FROM exam_rosters
		WHERE exam_id = ? AND cohort_id = ? AND student_id = ?`, examID, cohortID, studentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) == 0 {
		t.Fatal("roster row missing")
	}
	text, _ := rows.Values[0][0].(string)
	return text
}
