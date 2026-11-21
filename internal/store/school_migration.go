package store

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/school"
)

type migrationBatch struct {
	ID           int64
	AcademicYear string
	Grade        string
	CohortID     int64
	StageID      int64
	StageKey     string
}

func (s *Store) migrateSchoolData(ctx context.Context) error {
	version, err := s.schemaVersion(ctx)
	if err != nil {
		return err
	}
	if version >= SchemaVersion {
		return nil
	}
	if version < 4 {
		if err := s.preflightSchoolMigration(ctx); err != nil {
			return fmt.Errorf("迁移前检查失败: %w", err)
		}
		if err := s.renameLegacyTables(ctx); err != nil {
			return err
		}
		statements := seedSchoolData(schoolDataSchema())
		statements = append(statements, assessmentSchoolSchema()...)
		if _, err := s.db.Transaction(ctx, statements); err != nil {
			return err
		}
		if err := s.addStudentSourceColumns(ctx); err != nil {
			return err
		}
		if err := s.migrateLegacySchoolData(ctx); err != nil {
			return err
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (4, ?)`,
			s.now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if _, err := s.db.Transaction(ctx, examSchema()); err != nil {
		return err
	}
	if err := s.migrateLegacyExams(ctx); err != nil {
		return err
	}
	if err := s.ensureResultAssessmentColumn(ctx); err != nil {
		return err
	}
	if err := s.migrateResultRunsAssessment(ctx); err != nil {
		return err
	}
	if err := s.migrateLegacyResults(ctx); err != nil {
		return err
	}
	_, err = s.db.Execute(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
		SchemaVersion, s.now().Format(time.RFC3339Nano))
	return err
}

func (s *Store) preflightSchoolMigration(ctx context.Context) error {
	batches, err := s.migrationBatches(ctx)
	if err != nil {
		return err
	}
	studentClasses, err := s.firstTable(ctx, "legacy_student_classes", "student_classes")
	if err != nil {
		return err
	}
	classes, err := s.firstTable(ctx, "legacy_classes", "classes")
	if err != nil {
		return err
	}
	if studentClasses != "" && classes != "" {
		rows, err := s.db.Query(ctx, `SELECT sc.student_id, sc.class_id FROM `+studentClasses+` sc
			LEFT JOIN `+classes+` c ON c.id = sc.class_id
			WHERE c.id IS NULL LIMIT 5`)
		if err != nil {
			return err
		}
		if len(rows.Values) > 0 {
			studentID, _ := rows.Values[0][0].(int64)
			classID, _ := rows.Values[0][1].(int64)
			return fmt.Errorf("学生 %d 指向不存在的旧班级 %d", studentID, classID)
		}
	}
	teachers, err := s.firstTable(ctx, "legacy_teachers", "teachers")
	if err != nil {
		return err
	}
	if teachers != "" {
		rows, err := s.db.Query(ctx, `SELECT teacher_no, COUNT(*) FROM `+teachers+`
			WHERE teacher_no <> '' GROUP BY teacher_no HAVING COUNT(*) > 1 LIMIT 1`)
		if err != nil {
			return err
		}
		if len(rows.Values) > 0 {
			number, _ := rows.Values[0][0].(string)
			count, _ := rows.Values[0][1].(int64)
			return fmt.Errorf("工号 %q 对应 %d 条教师记录", number, count)
		}
	}
	groups, err := s.firstTable(ctx, "legacy_teaching_groups", "teaching_groups")
	if err != nil {
		return err
	}
	if groups != "" {
		rows, err := s.db.Query(ctx, `SELECT batch_id, subject, name, class_id FROM `+groups)
		if err != nil {
			return err
		}
		type groupKey struct {
			stage   int64
			subject string
			name    string
		}
		seen := map[groupKey]int64{}
		for _, row := range rows.Values {
			batchID, _ := row[0].(int64)
			subject, _ := row[1].(string)
			name, _ := row[2].(string)
			classID, _ := row[3].(int64)
			batch, ok := batches[batchID]
			if !ok {
				continue
			}
			key := groupKey{stage: batch.StageID, subject: subject, name: normalizeClassName(name)}
			if previous, ok := seen[key]; ok && previous != classID {
				return fmt.Errorf("阶段 %d 的学科 %s 存在同名教学班 %q，分别属于班级 %d 和 %d",
					batch.StageID, subject, name, previous, classID)
			}
			seen[key] = classID
		}
	}
	return nil
}

func (s *Store) firstTable(ctx context.Context, names ...string) (string, error) {
	for _, name := range names {
		exists, err := s.tableExists(ctx, name)
		if err != nil {
			return "", err
		}
		if exists {
			return name, nil
		}
	}
	return "", nil
}

func (s *Store) schemaVersion(ctx context.Context) (int, error) {
	rows, err := s.db.Query(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, nil
	}
	value, _ := rows.Values[0][0].(int64)
	return int(value), nil
}

func (s *Store) tableExists(ctx context.Context, name string) (bool, error) {
	rows, err := s.db.Query(ctx, `SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?`, name)
	if err != nil {
		return false, err
	}
	return len(rows.Values) > 0, nil
}

func (s *Store) columnExists(ctx context.Context, table, column string) (bool, error) {
	rows, err := s.db.Query(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, err
	}
	for _, row := range rows.Values {
		if len(row) > 1 {
			name, _ := row[1].(string)
			if name == column {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Store) renameLegacyTables(ctx context.Context) error {
	tables := []string{
		"classes", "student_classes", "teachers", "teaching_groups", "assignments",
		"import_sheets", "import_columns", "class_targets", "class_baselines",
		"stat_groups", "stat_group_members", "target_adjustments",
	}
	for _, table := range tables {
		exists, err := s.tableExists(ctx, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		legacy := "legacy_" + table
		legacyExists, err := s.tableExists(ctx, legacy)
		if err != nil {
			return err
		}
		if legacyExists {
			continue
		}
		if _, err := s.db.Execute(ctx, `ALTER TABLE `+table+` RENAME TO `+legacy); err != nil {
			return fmt.Errorf("重命名旧表 %s: %w", table, err)
		}
	}
	return nil
}

func (s *Store) addStudentSourceColumns(ctx context.Context) error {
	columns := []struct {
		name string
		sql  string
	}{
		{"created_at", `ALTER TABLE students ADD COLUMN created_at TEXT NOT NULL DEFAULT ''`},
		{"import_run_id", `ALTER TABLE students ADD COLUMN import_run_id INTEGER`},
		{"source_sheet", `ALTER TABLE students ADD COLUMN source_sheet TEXT NOT NULL DEFAULT ''`},
		{"source_row", `ALTER TABLE students ADD COLUMN source_row INTEGER NOT NULL DEFAULT 0`},
	}
	for _, column := range columns {
		exists, err := s.columnExists(ctx, "students", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := s.db.Execute(ctx, column.sql); err != nil {
			return err
		}
	}
	return nil
}

func assessmentSchoolSchema() []sqlite.Statement {
	return []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS class_targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			tier_key TEXT NOT NULL,
			target_count REAL NOT NULL,
			UNIQUE(batch_id, class_id, tier_key)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS class_baselines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			subject TEXT NOT NULL,
			average REAL NOT NULL,
			ratio REAL NOT NULL DEFAULT 0,
			UNIQUE(batch_id, class_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS stat_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			subject TEXT NOT NULL,
			name TEXT NOT NULL,
			lead_class_id INTEGER NOT NULL REFERENCES classes(id),
			target REAL NOT NULL DEFAULT 0,
			baseline_average REAL NOT NULL DEFAULT 0,
			baseline_ratio REAL NOT NULL DEFAULT 0,
			explicit INTEGER NOT NULL DEFAULT 0,
			UNIQUE(batch_id, subject, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS stat_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			stat_group_id INTEGER NOT NULL REFERENCES stat_groups(id) ON DELETE CASCADE,
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id),
			UNIQUE(stat_group_id, teaching_group_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS target_adjustments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			tier_key TEXT NOT NULL DEFAULT '',
			old_value REAL,
			new_value REAL,
			reason TEXT NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`},
	}
}

func (s *Store) migrateLegacySchoolData(ctx context.Context) error {
	batches, err := s.migrationBatches(ctx)
	if err != nil {
		return err
	}
	if len(batches) == 0 {
		return nil
	}
	classMap := map[int64]int64{}
	if err := s.migrateLegacyClasses(ctx, batches, classMap); err != nil {
		return err
	}
	if err := s.migrateLegacyTeachers(ctx); err != nil {
		return err
	}
	groupMap, err := s.migrateLegacyTeachingGroups(ctx, batches, classMap)
	if err != nil {
		return err
	}
	if err := s.migrateLegacyMemberships(ctx, batches, classMap, groupMap); err != nil {
		return err
	}
	if err := s.migrateLegacyAssignments(ctx, groupMap); err != nil {
		return err
	}
	if err := s.migrateLegacyAssessmentData(ctx, batches, classMap, groupMap); err != nil {
		return err
	}
	if err := s.migrateLegacyBaselines(ctx, batches, classMap); err != nil {
		return err
	}
	if err := s.migrateLegacyImports(ctx, batches); err != nil {
		return err
	}
	return s.initializeDataRevisions(ctx, batches)
}

func (s *Store) migrationBatches(ctx context.Context) (map[int64]migrationBatch, error) {
	rows, err := s.db.Query(ctx, `SELECT b.id, t.academic_year, b.grade
		FROM batches b JOIN terms t ON t.id = b.term_id ORDER BY b.id`)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]migrationBatch, len(rows.Values))
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		academicYear, _ := row[1].(string)
		grade, _ := row[2].(string)
		start, err := school.ParseAcademicYearStart(academicYear)
		if err != nil {
			return nil, fmt.Errorf("批次 %d 的学年无法解析: %w", id, err)
		}
		enrollment, err := school.EnrollmentYear(start, grade)
		if err != nil {
			return nil, fmt.Errorf("批次 %d 的年级无法解析: %w", id, err)
		}
		cohortRows, err := s.db.Query(ctx, `SELECT id FROM cohorts WHERE enrollment_year = ?`, enrollment)
		if err != nil {
			return nil, err
		}
		if len(cohortRows.Values) == 0 {
			return nil, fmt.Errorf("批次 %d 找不到对应的级", id)
		}
		cohortID, _ := cohortRows.Values[0][0].(int64)
		stageRows, err := s.db.Query(ctx, `SELECT id FROM cohort_stages
			WHERE cohort_id = ? AND academic_year_start = ?`, cohortID, start)
		if err != nil {
			return nil, err
		}
		if len(stageRows.Values) == 0 {
			return nil, fmt.Errorf("批次 %d 找不到对应的阶段", id)
		}
		stageID, _ := stageRows.Values[0][0].(int64)
		result[id] = migrationBatch{
			ID: id, AcademicYear: academicYear, Grade: grade,
			CohortID: cohortID, StageID: stageID, StageKey: grade,
		}
	}
	return result, nil
}

func (s *Store) migrateLegacyClasses(ctx context.Context, batches map[int64]migrationBatch, classMap map[int64]int64) error {
	exists, err := s.tableExists(ctx, "legacy_classes")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	rows, err := s.db.Query(ctx, `SELECT id, batch_id, name FROM legacy_classes ORDER BY id`)
	if err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	for _, row := range rows.Values {
		oldID, _ := row[0].(int64)
		batchID, _ := row[1].(int64)
		name, _ := row[2].(string)
		batch, ok := batches[batchID]
		if !ok {
			return fmt.Errorf("班级 %d 找不到批次 %d", oldID, batchID)
		}
		name = normalizeClassName(name)
		if name == "" {
			continue
		}
		if _, err := s.db.Execute(ctx, `INSERT INTO classes(cohort_id, name, created_at)
			VALUES (?, ?, ?) ON CONFLICT(cohort_id, name) DO NOTHING`, batch.CohortID, name, now); err != nil {
			return err
		}
		classRows, err := s.db.Query(ctx, `SELECT id FROM classes WHERE cohort_id = ? AND name = ?`,
			batch.CohortID, name)
		if err != nil {
			return err
		}
		if len(classRows.Values) == 0 {
			return fmt.Errorf("班级 %q 写入失败", name)
		}
		classID, _ := classRows.Values[0][0].(int64)
		classMap[oldID] = classID
		if _, err := s.db.Execute(ctx, `INSERT INTO class_stage_info(class_id, stage_id, status)
			VALUES (?, ?, 'active') ON CONFLICT(class_id, stage_id) DO NOTHING`, classID, batch.StageID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyTeachers(ctx context.Context) error {
	exists, err := s.tableExists(ctx, "legacy_teachers")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	now := s.now().Format(time.RFC3339Nano)
	rows, err := s.db.Query(ctx, `SELECT id, teacher_no, name FROM legacy_teachers ORDER BY id`)
	if err != nil {
		return err
	}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		number, _ := row[1].(string)
		name, _ := row[2].(string)
		if _, err := s.db.Execute(ctx, `INSERT INTO teachers(id, teacher_no, name, subject, created_at)
			VALUES (?, ?, ?, COALESCE((SELECT g.subject FROM legacy_assignments a
				JOIN legacy_teaching_groups g ON g.id = a.teaching_group_id
				WHERE a.teacher_id = ? LIMIT 1), ''), ?)
			ON CONFLICT(id) DO UPDATE SET teacher_no = excluded.teacher_no, name = excluded.name`,
			id, number, name, id, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyTeachingGroups(ctx context.Context, batches map[int64]migrationBatch, classMap map[int64]int64) (map[int64]int64, error) {
	groupMap := map[int64]int64{}
	exists, err := s.tableExists(ctx, "legacy_teaching_groups")
	if err != nil {
		return nil, err
	}
	if !exists {
		return groupMap, nil
	}
	rows, err := s.db.Query(ctx, `SELECT id, batch_id, subject, name FROM legacy_teaching_groups ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for _, row := range rows.Values {
		oldID, _ := row[0].(int64)
		batchID, _ := row[1].(int64)
		subject, _ := row[2].(string)
		name, _ := row[3].(string)
		batch, ok := batches[batchID]
		if !ok {
			return nil, fmt.Errorf("教学班 %d 找不到批次 %d", oldID, batchID)
		}
		name = normalizeClassName(name)
		if _, err := s.db.Execute(ctx, `INSERT INTO teaching_groups(stage_id, subject, name, kind)
			VALUES (?, ?, ?, 'admin') ON CONFLICT(stage_id, subject, name) DO NOTHING`,
			batch.StageID, subject, name); err != nil {
			return nil, err
		}
		groupRows, err := s.db.Query(ctx, `SELECT id FROM teaching_groups
			WHERE stage_id = ? AND subject = ? AND name = ?`, batch.StageID, subject, name)
		if err != nil {
			return nil, err
		}
		if len(groupRows.Values) == 0 {
			return nil, fmt.Errorf("教学班 %q 写入失败", name)
		}
		groupID, _ := groupRows.Values[0][0].(int64)
		groupMap[oldID] = groupID
	}
	return groupMap, nil
}

func (s *Store) migrateLegacyMemberships(ctx context.Context, batches map[int64]migrationBatch, classMap map[int64]int64, groupMap map[int64]int64) error {
	exists, err := s.tableExists(ctx, "legacy_student_classes")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	rows, err := s.db.Query(ctx, `SELECT sc.batch_id, sc.student_id, sc.class_id, sc.track,
		sc.elective, sc.japanese, sc.walking, sc.status, sc.total_score
		FROM legacy_student_classes sc ORDER BY sc.batch_id`)
	if err != nil {
		return err
	}
	type membershipKey struct {
		studentID int64
		stageID   int64
	}
	latest := map[membershipKey][]any{}
	seenCohort := map[[2]int64]bool{}
	for _, row := range rows.Values {
		batchID, _ := row[0].(int64)
		studentID, _ := row[1].(int64)
		batch, ok := batches[batchID]
		if !ok {
			continue
		}
		key := membershipKey{studentID: studentID, stageID: batch.StageID}
		latest[key] = row
		cohortKey := [2]int64{studentID, batch.CohortID}
		if !seenCohort[cohortKey] {
			seenCohort[cohortKey] = true
			if _, err := s.db.Execute(ctx, `INSERT INTO student_cohorts(student_id, cohort_id, status)
				VALUES (?, ?, 'active') ON CONFLICT(student_id, cohort_id) DO NOTHING`,
				studentID, batch.CohortID); err != nil {
				return err
			}
		}
	}
	for key, row := range latest {
		batchID, _ := row[0].(int64)
		oldClassID, _ := row[2].(int64)
		classID, ok := classMap[oldClassID]
		if !ok {
			return fmt.Errorf("学生 %d 的教学班找不到行政班 %d", key.studentID, oldClassID)
		}
		batch := batches[batchID]
		track, _ := row[3].(string)
		elective, _ := row[4].(string)
		japanese, _ := row[5].(int64)
		walking, _ := row[6].(int64)
		status, _ := row[7].(string)
		if _, err := s.db.Execute(ctx, `INSERT INTO student_stage_memberships(
			student_id, stage_id, class_id, status, walking, track, elective, japanese, total_score)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(student_id, stage_id) DO UPDATE SET class_id = excluded.class_id,
				status = excluded.status, walking = excluded.walking, track = excluded.track,
				elective = excluded.elective, japanese = excluded.japanese, total_score = excluded.total_score`,
			key.studentID, batch.StageID, classID, status, walking, track, elective, japanese, row[8]); err != nil {
			return err
		}
	}
	rows, err = s.db.Query(ctx, `SELECT m.student_id, m.stage_id, c.name
		FROM student_stage_memberships m JOIN classes c ON c.id = m.class_id`)
	if err != nil {
		return err
	}
	for _, row := range rows.Values {
		studentID, _ := row[0].(int64)
		stageID, _ := row[1].(int64)
		className, _ := row[2].(string)
		groups, err := s.db.Query(ctx, `SELECT id, subject FROM teaching_groups
			WHERE stage_id = ? AND name = ? ORDER BY subject`, stageID, normalizeClassName(className))
		if err != nil {
			return err
		}
		for _, group := range groups.Values {
			groupID, _ := group[0].(int64)
			subject, _ := group[1].(string)
			if _, err := s.db.Execute(ctx, `INSERT INTO student_teaching_groups(
				student_id, stage_id, subject, teaching_group_id) VALUES (?, ?, ?, ?)
				ON CONFLICT(student_id, stage_id, subject) DO UPDATE SET
					teaching_group_id = excluded.teaching_group_id`,
				studentID, stageID, subject, groupID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) migrateLegacyAssignments(ctx context.Context, groupMap map[int64]int64) error {
	exists, err := s.tableExists(ctx, "legacy_assignments")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	rows, err := s.db.Query(ctx, `SELECT teacher_id, teaching_group_id FROM legacy_assignments ORDER BY id`)
	if err != nil {
		return err
	}
	for _, row := range rows.Values {
		teacherID, _ := row[0].(int64)
		oldGroupID, _ := row[1].(int64)
		groupID, ok := groupMap[oldGroupID]
		if !ok {
			continue
		}
		if _, err := s.db.Execute(ctx, `INSERT INTO assignments(teacher_id, teaching_group_id)
			VALUES (?, ?) ON CONFLICT(teacher_id, teaching_group_id) DO NOTHING`,
			teacherID, groupID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyAssessmentData(ctx context.Context, batches map[int64]migrationBatch, classMap, groupMap map[int64]int64) error {
	if exists, err := s.tableExists(ctx, "legacy_class_targets"); err != nil {
		return err
	} else if exists {
		rows, err := s.db.Query(ctx, `SELECT batch_id, class_id, tier_key, target_count FROM legacy_class_targets`)
		if err != nil {
			return err
		}
		for _, row := range rows.Values {
			batchID, _ := row[0].(int64)
			oldClassID, _ := row[1].(int64)
			tier, _ := row[2].(string)
			classID, ok := classMap[oldClassID]
			if !ok {
				continue
			}
			if _, err := s.db.Execute(ctx, `INSERT INTO class_targets(batch_id, class_id, tier_key, target_count)
				VALUES (?, ?, ?, ?) ON CONFLICT(batch_id, class_id, tier_key) DO UPDATE SET
					target_count = excluded.target_count`, batchID, classID, tier, row[3]); err != nil {
				return err
			}
		}
	}
	if exists, err := s.tableExists(ctx, "legacy_class_baselines"); err != nil {
		return err
	} else if exists {
		rows, err := s.db.Query(ctx, `SELECT batch_id, class_id, subject, average, ratio FROM legacy_class_baselines`)
		if err != nil {
			return err
		}
		for _, row := range rows.Values {
			batchID, _ := row[0].(int64)
			oldClassID, _ := row[1].(int64)
			classID, ok := classMap[oldClassID]
			if !ok {
				continue
			}
			if _, err := s.db.Execute(ctx, `INSERT INTO class_baselines(batch_id, class_id, subject, average, ratio)
				VALUES (?, ?, ?, ?, ?) ON CONFLICT(batch_id, class_id, subject) DO UPDATE SET
					average = excluded.average, ratio = excluded.ratio`, batchID, classID, row[2], row[3], row[4]); err != nil {
				return err
			}
		}
	}
	if exists, err := s.tableExists(ctx, "legacy_stat_groups"); err != nil {
		return err
	} else if exists {
		rows, err := s.db.Query(ctx, `SELECT id, batch_id, subject, name, lead_class_id, target,
			baseline_average, baseline_ratio, explicit FROM legacy_stat_groups`)
		if err != nil {
			return err
		}
		for _, row := range rows.Values {
			oldClassID, _ := row[4].(int64)
			classID, ok := classMap[oldClassID]
			if !ok {
				continue
			}
			_, err := s.db.Execute(ctx, `INSERT INTO stat_groups(
				batch_id, subject, name, lead_class_id, target, baseline_average, baseline_ratio, explicit)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(batch_id, subject, name) DO UPDATE SET
					lead_class_id = excluded.lead_class_id, target = excluded.target,
					baseline_average = excluded.baseline_average, baseline_ratio = excluded.baseline_ratio,
					explicit = excluded.explicit`, row[1], row[2], row[3], classID, row[5], row[6], row[7], row[8])
			if err != nil {
				return err
			}
		}
		if exists, err := s.tableExists(ctx, "legacy_stat_group_members"); err != nil {
			return err
		} else if exists {
			members, err := s.db.Query(ctx, `SELECT stat_group_id, teaching_group_id FROM legacy_stat_group_members`)
			if err != nil {
				return err
			}
			for _, row := range members.Values {
				oldStatID, _ := row[0].(int64)
				oldGroupID, _ := row[1].(int64)
				newGroupID, ok := groupMap[oldGroupID]
				if !ok {
					continue
				}
				statRows, err := s.db.Query(ctx, `SELECT sg.id FROM legacy_stat_groups lsg
					JOIN stat_groups sg ON sg.batch_id = lsg.batch_id AND sg.subject = lsg.subject
						AND sg.name = lsg.name WHERE lsg.id = ?`, oldStatID)
				if err != nil {
					return err
				}
				if len(statRows.Values) == 0 {
					continue
				}
				statID, _ := statRows.Values[0][0].(int64)
				if _, err := s.db.Execute(ctx, `INSERT INTO stat_group_members(stat_group_id, teaching_group_id)
					VALUES (?, ?) ON CONFLICT(stat_group_id, teaching_group_id) DO NOTHING`, statID, newGroupID); err != nil {
					return err
				}
			}
		}
	}
	if exists, err := s.tableExists(ctx, "legacy_target_adjustments"); err != nil {
		return err
	} else if exists {
		rows, err := s.db.Query(ctx, `SELECT batch_id, class_id, tier_key, old_value, new_value,
			reason, created_by, created_at FROM legacy_target_adjustments`)
		if err != nil {
			return err
		}
		for _, row := range rows.Values {
			oldClassID, _ := row[1].(int64)
			classID, ok := classMap[oldClassID]
			if !ok {
				continue
			}
			if _, err := s.db.Execute(ctx, `INSERT INTO target_adjustments(
				batch_id, class_id, tier_key, old_value, new_value, reason, created_by, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, row[0], classID, row[2], row[3], row[4], row[5], row[6], row[7]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) migrateLegacyBaselines(ctx context.Context, batches map[int64]migrationBatch, classMap map[int64]int64) error {
	setIDs := map[int64]int64{}
	ensureSet := func(cohortID int64) (int64, error) {
		if id, ok := setIDs[cohortID]; ok {
			return id, nil
		}
		name := "历史入口成绩"
		if _, err := s.db.Execute(ctx, `INSERT INTO baseline_sets(cohort_id, name, source, kind, status, created_at)
			VALUES (?, ?, 'legacy_summary', 'zhongkao', 'active', ?)
			ON CONFLICT(cohort_id, name) DO NOTHING`, cohortID, name, s.now().Format(time.RFC3339Nano)); err != nil {
			return 0, err
		}
		rows, err := s.db.Query(ctx, `SELECT id FROM baseline_sets WHERE cohort_id = ? AND name = ?`, cohortID, name)
		if err != nil {
			return 0, err
		}
		if len(rows.Values) == 0 {
			return 0, fmt.Errorf("入口成绩数据集写入失败")
		}
		id, _ := rows.Values[0][0].(int64)
		setIDs[cohortID] = id
		return id, nil
	}
	if exists, err := s.tableExists(ctx, "legacy_class_baselines"); err != nil {
		return err
	} else if exists {
		rows, err := s.db.Query(ctx, `SELECT batch_id, class_id, subject, average, ratio FROM legacy_class_baselines`)
		if err != nil {
			return err
		}
		for _, row := range rows.Values {
			batchID, _ := row[0].(int64)
			oldClassID, _ := row[1].(int64)
			batch, ok := batches[batchID]
			if !ok {
				continue
			}
			classID, ok := classMap[oldClassID]
			if !ok {
				continue
			}
			setID, err := ensureSet(batch.CohortID)
			if err != nil {
				return err
			}
			subject, _ := row[2].(string)
			if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO baseline_subjects(baseline_set_id, subject, full_mark)
				VALUES (?, ?, ?)`, setID, subject, defaultFullMark(subject)); err != nil {
				return err
			}
			count := 0
			countRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM student_stage_memberships
				WHERE stage_id = ? AND class_id = ?`, batch.StageID, classID)
			if err != nil {
				return err
			}
			if len(countRows.Values) > 0 {
				value, _ := countRows.Values[0][0].(int64)
				count = int(value)
			}
			if _, err := s.db.Execute(ctx, `INSERT INTO baseline_aggregates(
				baseline_set_id, stage_id, scope_type, scope_id, subject, average, count, ratio)
				VALUES (?, ?, 'class', ?, ?, ?, ?, ?)
				ON CONFLICT(baseline_set_id, stage_id, scope_type, scope_id, subject) DO UPDATE SET
					average = excluded.average, count = excluded.count, ratio = excluded.ratio`,
				setID, batch.StageID, classID, subject, row[3], count, row[4]); err != nil {
				return err
			}
		}
	}
	if exists, err := s.tableExists(ctx, "grade_baselines"); err != nil {
		return err
	} else if exists {
		rows, err := s.db.Query(ctx, `SELECT batch_id, subject, average FROM grade_baselines`)
		if err != nil {
			return err
		}
		for _, row := range rows.Values {
			batchID, _ := row[0].(int64)
			batch, ok := batches[batchID]
			if !ok {
				continue
			}
			setID, err := ensureSet(batch.CohortID)
			if err != nil {
				return err
			}
			subject, _ := row[1].(string)
			if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO baseline_subjects(baseline_set_id, subject, full_mark)
				VALUES (?, ?, ?)`, setID, subject, defaultFullMark(subject)); err != nil {
				return err
			}
			count := 0
			countRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?`,
				batch.StageID)
			if err != nil {
				return err
			}
			if len(countRows.Values) > 0 {
				value, _ := countRows.Values[0][0].(int64)
				count = int(value)
			}
			if _, err := s.db.Execute(ctx, `INSERT INTO baseline_aggregates(
				baseline_set_id, stage_id, scope_type, scope_id, subject, average, count, ratio)
				VALUES (?, ?, 'stage', ?, ?, ?, ?, 0)
				ON CONFLICT(baseline_set_id, stage_id, scope_type, scope_id, subject) DO UPDATE SET
					average = excluded.average, count = excluded.count`,
				setID, batch.StageID, batch.StageID, subject, row[2], count); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) migrateLegacyImports(ctx context.Context, batches map[int64]migrationBatch) error {
	exists, err := s.tableExists(ctx, "imports")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	rows, err := s.db.Query(ctx, `SELECT id, batch_id, file_hash, relative_path, converted_path,
		row_count, status, created_at FROM imports ORDER BY id`)
	if err != nil {
		return err
	}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		batchID, _ := row[1].(int64)
		batch, ok := batches[batchID]
		if !ok {
			continue
		}
		hash, _ := row[2].(string)
		relative, _ := row[3].(string)
		converted, _ := row[4].(string)
		rowCount, _ := row[5].(int64)
		status, _ := row[6].(string)
		createdAt, _ := row[7].(string)
		if _, err := s.db.Execute(ctx, `INSERT INTO import_runs(
			id, scope_type, scope_id, data_kind, file_hash, stored_path, relative_path,
			converted_path, status, row_count, created_at)
			VALUES (?, 'stage', ?, 'legacy_workbook', ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING`, id, batch.StageID, hash, relative, relative,
			converted, status, rowCount, createdAt); err != nil {
			return err
		}
	}
	if exists, err := s.tableExists(ctx, "legacy_import_sheets"); err != nil {
		return err
	} else if exists {
		sheets, err := s.db.Query(ctx, `SELECT id, import_id, sheet_name, role, header_row,
			data_range, version_tag, is_current FROM legacy_import_sheets ORDER BY id`)
		if err != nil {
			return err
		}
		for _, row := range sheets.Values {
			id, _ := row[0].(int64)
			importID, _ := row[1].(int64)
			if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO import_sheets(
				id, import_run_id, sheet_name, role, header_row, data_range, version_tag, is_current)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, importID, row[2], row[3], row[4], row[5], row[6], row[7]); err != nil {
				return err
			}
		}
	}
	if exists, err := s.tableExists(ctx, "legacy_import_columns"); err != nil {
		return err
	} else if exists {
		columns, err := s.db.Query(ctx, `SELECT id, import_sheet_id, source_column, standard_field,
			merged_header, is_user_selected FROM legacy_import_columns ORDER BY id`)
		if err != nil {
			return err
		}
		for _, row := range columns.Values {
			if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO import_columns(
				id, import_sheet_id, source_column, standard_field, merged_header, is_user_selected)
				VALUES (?, ?, ?, ?, ?, ?)`, row[0], row[1], row[2], row[3], row[4], row[5]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) initializeDataRevisions(ctx context.Context, batches map[int64]migrationBatch) error {
	now := s.now().Format(time.RFC3339Nano)
	cohorts := map[int64]bool{}
	stages := map[int64]bool{}
	for _, batch := range batches {
		cohorts[batch.CohortID] = true
		stages[batch.StageID] = true
	}
	for id := range cohorts {
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO data_revisions(scope_type, scope_id, revision, updated_at)
			VALUES ('cohort', ?, 1, ?)`, id, now); err != nil {
			return err
		}
	}
	for id := range stages {
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO data_revisions(scope_type, scope_id, revision, updated_at)
			VALUES ('stage', ?, 1, ?)`, id, now); err != nil {
			return err
		}
	}
	for id := range batches {
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO data_revisions(scope_type, scope_id, revision, updated_at)
			VALUES ('batch', ?, 1, ?)`, id, now); err != nil {
			return err
		}
	}
	return nil
}

func defaultFullMark(subject string) float64 {
	switch subject {
	case "chinese", "math", "english", "语文", "数学", "英语":
		return 150
	default:
		return 100
	}
}

func (s *Store) ensureBatchStage(ctx context.Context, batch Batch) (migrationBatch, error) {
	start, err := school.ParseAcademicYearStart(batch.AcademicYear)
	if err != nil {
		return migrationBatch{}, err
	}
	enrollment, err := school.EnrollmentYear(start, batch.Grade)
	if err != nil {
		return migrationBatch{}, err
	}
	if _, err := s.db.Execute(ctx, `INSERT INTO cohorts(name, enrollment_year, graduation_year, status, created_at)
		VALUES (?, ?, ?, 'active', ?) ON CONFLICT(enrollment_year) DO NOTHING`,
		fmt.Sprintf("%d级", enrollment), enrollment, school.GraduationYear(enrollment),
		s.now().Format(time.RFC3339Nano)); err != nil {
		return migrationBatch{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM cohorts WHERE enrollment_year = ?`, enrollment)
	if err != nil {
		return migrationBatch{}, err
	}
	if len(rows.Values) == 0 {
		return migrationBatch{}, fmt.Errorf("级写入失败")
	}
	cohortID, _ := rows.Values[0][0].(int64)
	stage, err := s.EnsureCohortStage(ctx, cohortID, start, batch.Grade)
	if err != nil {
		return migrationBatch{}, err
	}
	return migrationBatch{
		ID: batch.ID, AcademicYear: batch.AcademicYear, Grade: batch.Grade,
		CohortID: cohortID, StageID: stage.ID, StageKey: batch.Grade,
	}, nil
}

func (s *Store) stageForBatch(ctx context.Context, batch Batch) (migrationBatch, error) {
	return s.ensureBatchStage(ctx, batch)
}

func migrationCounts(ctx context.Context, s *Store) (map[string]int, error) {
	tables := []string{
		"classes", "students", "student_cohorts", "student_stage_memberships",
		"teachers", "teaching_groups", "student_teaching_groups", "assignments",
		"baseline_aggregates",
	}
	counts := map[string]int{}
	for _, table := range tables {
		rows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM `+table)
		if err != nil {
			return nil, err
		}
		value := 0
		if len(rows.Values) > 0 {
			number, _ := rows.Values[0][0].(int64)
			value = int(number)
		}
		counts[table] = value
	}
	return counts, nil
}
