package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// StoreImportFile 把导入文件按内容哈希复制到数据目录，返回副本路径。
func (s *Store) StoreImportFile(path, hash string) (string, error) {
	if path == "" || hash == "" {
		return "", fmt.Errorf("导入文件和哈希不能为空")
	}
	extension := strings.ToLower(filepath.Ext(path))
	if extension == "" {
		extension = ".xlsx"
	}
	dir := filepath.Join(s.root, "files")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, hash+extension)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	source, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer source.Close()
	temp := target + ".tmp"
	output, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(output, source); err != nil {
		output.Close()
		os.Remove(temp)
		return "", err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		os.Remove(temp)
		return "", err
	}
	if err := output.Close(); err != nil {
		os.Remove(temp)
		return "", err
	}
	if err := os.Rename(temp, target); err != nil {
		return "", err
	}
	return target, nil
}

// MergeStageRoster 把学生名单写入一个级与阶段，不创建考核批次。
func (s *Store) MergeStageRoster(ctx context.Context, cohortID, stageID int64, data ImportData, report ImportReport) (ImportSummary, error) {
	stage, err := s.stageByID(ctx, cohortID, stageID)
	if err != nil {
		return ImportSummary{}, err
	}
	runID, err := s.ensureImportRun(ctx, stage, report, data)
	if err != nil {
		return ImportSummary{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	names := map[string]bool{}
	for _, class := range data.Classes {
		if class != "" {
			names[class] = true
		}
	}
	for _, student := range data.Students {
		if student.ClassName != "" {
			names[student.ClassName] = true
		}
	}
	var statements []sqlite.Statement
	for className := range names {
		name := normalizeClassName(className)
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO classes(cohort_id, name, created_at, import_run_id)
					VALUES (?, ?, ?, ?) ON CONFLICT(cohort_id, name) DO NOTHING`,
				Args: []any{cohortID, name, now, runID},
			},
			sqlite.Statement{
				SQL: `INSERT INTO class_stage_info(class_id, stage_id, status, import_run_id)
					SELECT id, ?, 'active', ? FROM classes WHERE cohort_id = ? AND name = ?
					ON CONFLICT(class_id, stage_id) DO NOTHING`,
				Args: []any{stageID, runID, cohortID, name},
			},
		)
	}
	for _, student := range data.Students {
		if student.StudentNo == "" || student.Name == "" || student.ClassName == "" {
			return ImportSummary{}, fmt.Errorf("学生 %q 缺少学号、姓名或者班级", student.Name)
		}
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO students(student_no, name, created_at, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, ?, ?, ?)
					ON CONFLICT(student_no) DO UPDATE SET name = excluded.name,
						import_run_id = excluded.import_run_id, source_sheet = excluded.source_sheet,
						source_row = excluded.source_row`,
				Args: []any{student.StudentNo, student.Name, now, runID, student.SourceSheet, student.SourceRow},
			},
			sqlite.Statement{
				SQL: `INSERT INTO student_cohorts(student_id, cohort_id, status, import_run_id, source_sheet, source_row)
					SELECT id, ?, 'active', ?, ?, ? FROM students WHERE student_no = ?
					ON CONFLICT(student_id, cohort_id) DO NOTHING`,
				Args: []any{cohortID, runID, student.SourceSheet, student.SourceRow, student.StudentNo},
			},
			sqlite.Statement{
				SQL: `INSERT INTO student_stage_memberships(
					student_id, stage_id, class_id, status, walking, track, elective, japanese, total_score,
					import_run_id, source_sheet, source_row)
					SELECT s.id, ?, c.id, ?, ?, ?, ?, ?, ?, ?, ?, ? FROM students s, classes c
					WHERE s.student_no = ? AND c.cohort_id = ? AND c.name = ?
					ON CONFLICT(student_id, stage_id) DO UPDATE SET class_id = excluded.class_id,
						status = excluded.status, walking = excluded.walking, track = excluded.track,
						elective = excluded.elective, japanese = excluded.japanese,
						total_score = excluded.total_score, import_run_id = excluded.import_run_id,
						source_sheet = excluded.source_sheet, source_row = excluded.source_row`,
				Args: []any{stageID, student.Status, boolInt(student.Walking), student.Track,
					student.Elective, boolInt(student.Japanese), optionalFloat(student.TotalScore),
					runID, student.SourceSheet, student.SourceRow, student.StudentNo,
					cohortID, normalizeClassName(student.ClassName)},
			},
		)
	}
	statements = append(statements,
		sqlite.Statement{
			SQL: `INSERT INTO data_revisions(scope_type, scope_id, revision, updated_at)
				VALUES ('stage', ?, 1, ?)
				ON CONFLICT(scope_type, scope_id) DO UPDATE SET revision = revision + 1,
					updated_at = excluded.updated_at`,
			Args: []any{stageID, now},
		},
		sqlite.Statement{
			SQL: `INSERT INTO audit_log(action, entity, entity_id, detail_json, created_at, operator)
				VALUES ('import', 'stage', ?, '{}', ?, 'local')`,
			Args: []any{stageID, now},
		},
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		if runID != 0 {
			_, _ = s.db.Execute(ctx, `DELETE FROM import_runs WHERE id = ?`, runID)
		}
		return ImportSummary{}, err
	}
	if runID != 0 {
		if err := s.saveImportSheets(ctx, runID, report); err != nil {
			return ImportSummary{}, err
		}
	}
	rows, err := s.db.Query(ctx, `SELECT
		(SELECT COUNT(*) FROM class_stage_info WHERE stage_id = ?),
		(SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?)`, stageID, stageID)
	if err != nil {
		return ImportSummary{}, err
	}
	summary := ImportSummary{Warnings: report.Warnings}
	if len(rows.Values) > 0 {
		classes, _ := rows.Values[0][0].(int64)
		students, _ := rows.Values[0][1].(int64)
		summary.Classes = int(classes)
		summary.Students = int(students)
	}
	return summary, nil
}

// MergeStageAssignments 把教师、教学班与任课关系写入一个级与阶段。
func (s *Store) MergeStageAssignments(ctx context.Context, cohortID, stageID int64, data ImportData, report ImportReport) (ImportSummary, error) {
	stage, err := s.stageByID(ctx, cohortID, stageID)
	if err != nil {
		return ImportSummary{}, err
	}
	runID, err := s.ensureImportRun(ctx, stage, report, data)
	if err != nil {
		return ImportSummary{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	names := map[string]bool{}
	type sourceRef struct {
		sheet string
		row   int
	}
	for _, class := range data.Classes {
		if class != "" {
			names[class] = true
		}
	}
	teacherSubjects := map[string]map[string]bool{}
	classSubjects := map[string]map[string]bool{}
	teacherSources := map[string]map[string]sourceRef{}
	classSources := map[string]map[string]sourceRef{}
	for index, assignment := range data.Assignments {
		if assignment.TeacherName == "" || assignment.Subject == "" || assignment.ClassName == "" {
			return ImportSummary{}, fmt.Errorf("任课关系第 %d 条缺少教师、学科或者班级", index+1)
		}
		names[assignment.ClassName] = true
		if teacherSubjects[assignment.TeacherName] == nil {
			teacherSubjects[assignment.TeacherName] = map[string]bool{}
			teacherSources[assignment.TeacherName] = map[string]sourceRef{}
		}
		teacherSubjects[assignment.TeacherName][assignment.Subject] = true
		if _, ok := teacherSources[assignment.TeacherName][assignment.Subject]; !ok {
			teacherSources[assignment.TeacherName][assignment.Subject] = sourceRef{
				sheet: assignment.SourceSheet, row: assignment.SourceRow,
			}
		}
		if classSubjects[assignment.ClassName] == nil {
			classSubjects[assignment.ClassName] = map[string]bool{}
			classSources[assignment.ClassName] = map[string]sourceRef{}
		}
		classSubjects[assignment.ClassName][assignment.Subject] = true
		if _, ok := classSources[assignment.ClassName][assignment.Subject]; !ok {
			classSources[assignment.ClassName][assignment.Subject] = sourceRef{
				sheet: assignment.SourceSheet, row: assignment.SourceRow,
			}
		}
	}
	for _, teacher := range data.Teachers {
		if teacher == "" || teacherSubjects[teacher] != nil {
			continue
		}
		teacherSubjects[teacher] = map[string]bool{"": true}
	}
	var statements []sqlite.Statement
	for className := range names {
		name := normalizeClassName(className)
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO classes(cohort_id, name, created_at, import_run_id)
					VALUES (?, ?, ?, ?) ON CONFLICT(cohort_id, name) DO NOTHING`,
				Args: []any{cohortID, name, now, runID},
			},
			sqlite.Statement{
				SQL: `INSERT INTO class_stage_info(class_id, stage_id, status, import_run_id)
					SELECT id, ?, 'active', ? FROM classes WHERE cohort_id = ? AND name = ?
					ON CONFLICT(class_id, stage_id) DO NOTHING`,
				Args: []any{stageID, runID, cohortID, name},
			},
		)
	}
	for teacher, subjects := range teacherSubjects {
		for subject := range subjects {
			source := teacherSources[teacher][subject]
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO teachers(
					teacher_no, name, subject, created_at, import_run_id, source_sheet, source_row)
					SELECT '', ?, ?, ?, ?, ?, ? WHERE NOT EXISTS (
						SELECT 1 FROM teachers WHERE teacher_no = '' AND name = ? AND subject = ?)`,
				Args: []any{teacher, subject, now, runID, source.sheet, source.row, teacher, subject},
			})
		}
	}
	for className, subjects := range classSubjects {
		name := normalizeClassName(className)
		for subject := range subjects {
			source := classSources[className][subject]
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO teaching_groups(
					stage_id, subject, name, kind, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, 'admin', ?, ?, ?)
					ON CONFLICT(stage_id, subject, name) DO NOTHING`,
				Args: []any{stageID, subject, name, runID, source.sheet, source.row},
			})
		}
	}
	for _, assignment := range data.Assignments {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assignments(teacher_id, teaching_group_id, import_run_id, source_sheet, source_row)
				SELECT t.id, g.id, ?, ?, ? FROM teachers t, teaching_groups g
				WHERE t.teacher_no = '' AND t.name = ? AND t.subject = ?
					AND g.stage_id = ? AND g.subject = ? AND g.name = ?
				ON CONFLICT(teacher_id, teaching_group_id) DO NOTHING`,
			Args: []any{runID, assignment.SourceSheet, assignment.SourceRow,
				assignment.TeacherName, assignment.Subject, stageID, assignment.Subject,
				normalizeClassName(assignment.ClassName)},
		})
	}
	for className, subjects := range classSubjects {
		name := normalizeClassName(className)
		for subject := range subjects {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO student_teaching_groups(
					student_id, stage_id, subject, teaching_group_id, import_run_id)
					SELECT m.student_id, m.stage_id, g.subject, g.id, ?
					FROM student_stage_memberships m
					JOIN classes c ON c.id = m.class_id
					JOIN teaching_groups g ON g.stage_id = m.stage_id AND g.name = c.name AND g.subject = ?
					WHERE m.stage_id = ? AND c.name = ?
					ON CONFLICT(student_id, stage_id, subject) DO UPDATE SET
						teaching_group_id = excluded.teaching_group_id, import_run_id = excluded.import_run_id`,
				Args: []any{runID, subject, stageID, name},
			})
		}
	}
	statements = append(statements,
		sqlite.Statement{
			SQL: `INSERT INTO data_revisions(scope_type, scope_id, revision, updated_at)
				VALUES ('stage', ?, 1, ?)
				ON CONFLICT(scope_type, scope_id) DO UPDATE SET revision = revision + 1,
					updated_at = excluded.updated_at`,
			Args: []any{stageID, now},
		},
		sqlite.Statement{
			SQL: `INSERT INTO audit_log(action, entity, entity_id, detail_json, created_at, operator)
				VALUES ('import', 'stage', ?, '{}', ?, 'local')`,
			Args: []any{stageID, now},
		},
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		if runID != 0 {
			_, _ = s.db.Execute(ctx, `DELETE FROM import_runs WHERE id = ?`, runID)
		}
		return ImportSummary{}, err
	}
	if runID != 0 {
		if err := s.saveImportSheets(ctx, runID, report); err != nil {
			return ImportSummary{}, err
		}
	}
	rows, err := s.db.Query(ctx, `SELECT
		(SELECT COUNT(DISTINCT a.teacher_id) FROM assignments a
			JOIN teaching_groups g ON g.id = a.teaching_group_id WHERE g.stage_id = ?),
		(SELECT COUNT(*) FROM teaching_groups WHERE stage_id = ?),
		(SELECT COUNT(*) FROM assignments a JOIN teaching_groups g ON g.id = a.teaching_group_id
			WHERE g.stage_id = ?),
		(SELECT COUNT(*) FROM class_stage_info WHERE stage_id = ?),
		(SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?)`,
		stageID, stageID, stageID, stageID, stageID)
	if err != nil {
		return ImportSummary{}, err
	}
	summary := ImportSummary{Warnings: report.Warnings}
	if len(rows.Values) > 0 {
		teachers, _ := rows.Values[0][0].(int64)
		groups, _ := rows.Values[0][1].(int64)
		assignments, _ := rows.Values[0][2].(int64)
		classes, _ := rows.Values[0][3].(int64)
		students, _ := rows.Values[0][4].(int64)
		summary.Teachers = int(teachers)
		summary.TeachingGroups = int(groups)
		summary.Assignments = int(assignments)
		summary.Classes = int(classes)
		summary.Students = int(students)
	}
	return summary, nil
}

// MergeStageBaselines 把入口成绩汇总写入一个级与阶段。
func (s *Store) MergeStageBaselines(ctx context.Context, cohortID, stageID int64, data ImportData, report ImportReport) (ImportSummary, error) {
	stage, err := s.stageByID(ctx, cohortID, stageID)
	if err != nil {
		return ImportSummary{}, err
	}
	if len(data.ClassBaselines) == 0 && len(data.GradeBaselines) == 0 {
		return ImportSummary{}, fmt.Errorf("工作簿中没有入口成绩")
	}
	runID, err := s.ensureImportRun(ctx, stage, report, data)
	if err != nil {
		return ImportSummary{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	for _, baseline := range data.ClassBaselines {
		rows, err := s.db.Query(ctx, `SELECT id FROM classes WHERE cohort_id = ? AND name = ?`,
			cohortID, normalizeClassName(baseline.ClassName))
		if err != nil {
			return ImportSummary{}, err
		}
		if len(rows.Values) == 0 {
			return ImportSummary{}, fmt.Errorf("入口成绩中的班级 %q 不存在", baseline.ClassName)
		}
	}
	statements := []sqlite.Statement{
		{
			SQL: `INSERT INTO baseline_sets(cohort_id, name, source, kind, status, created_at, import_run_id)
				VALUES (?, '入口成绩', 'import', 'zhongkao', 'active', ?, ?)
				ON CONFLICT(cohort_id, name) DO UPDATE SET import_run_id = excluded.import_run_id`,
			Args: []any{cohortID, now, runID},
		},
	}
	for _, baseline := range data.ClassBaselines {
		name := normalizeClassName(baseline.ClassName)
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO baseline_subjects(baseline_set_id, subject, full_mark)
					SELECT id, ?, ? FROM baseline_sets WHERE cohort_id = ? AND name = '入口成绩'`,
				Args: []any{baseline.Subject, defaultFullMark(baseline.Subject), cohortID},
			},
			sqlite.Statement{
				SQL: `INSERT INTO baseline_aggregates(
					baseline_set_id, stage_id, scope_type, scope_id, subject, average, count, ratio,
					import_run_id, source_sheet, source_row)
					SELECT b.id, ?, 'class', c.id, ?, ?, (
						SELECT COUNT(*) FROM student_stage_memberships m
						WHERE m.stage_id = ? AND m.class_id = c.id), ?, ?, ?, ?
					FROM baseline_sets b, classes c
					WHERE b.cohort_id = ? AND b.name = '入口成绩' AND c.cohort_id = ? AND c.name = ?
					ON CONFLICT(baseline_set_id, stage_id, scope_type, scope_id, subject) DO UPDATE SET
						average = excluded.average, count = excluded.count, ratio = excluded.ratio,
						import_run_id = excluded.import_run_id, source_sheet = excluded.source_sheet,
						source_row = excluded.source_row`,
				Args: []any{stageID, baseline.Subject, baseline.Average, stageID,
					floatOrZero(baseline.Ratio), runID, baseline.SourceSheet, baseline.SourceRow,
					cohortID, cohortID, name},
			},
		)
	}
	for subject, average := range data.GradeBaselines {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO baseline_subjects(baseline_set_id, subject, full_mark)
				SELECT id, ?, ? FROM baseline_sets WHERE cohort_id = ? AND name = '入口成绩'
				ON CONFLICT(baseline_set_id, subject) DO NOTHING`,
			Args: []any{subject, defaultFullMark(subject), cohortID},
		}, sqlite.Statement{
			SQL: `INSERT INTO baseline_aggregates(
				baseline_set_id, stage_id, scope_type, scope_id, subject, average, count, ratio,
				import_run_id, source_sheet, source_row)
				SELECT b.id, ?, 'stage', ?, ?, ?, (
					SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?), 0, ?, ?, 0
				FROM baseline_sets b WHERE b.cohort_id = ? AND b.name = '入口成绩'
				ON CONFLICT(baseline_set_id, stage_id, scope_type, scope_id, subject) DO UPDATE SET
					average = excluded.average, count = excluded.count, import_run_id = excluded.import_run_id,
					source_sheet = excluded.source_sheet`,
			Args: []any{stageID, stageID, subject, average, stageID, runID, "", cohortID},
		})
	}
	statements = append(statements,
		sqlite.Statement{
			SQL: `INSERT INTO data_revisions(scope_type, scope_id, revision, updated_at)
				VALUES ('stage', ?, 1, ?)
				ON CONFLICT(scope_type, scope_id) DO UPDATE SET revision = revision + 1,
					updated_at = excluded.updated_at`,
			Args: []any{stageID, now},
		},
		sqlite.Statement{
			SQL: `INSERT INTO audit_log(action, entity, entity_id, detail_json, created_at, operator)
				VALUES ('import', 'stage', ?, '{}', ?, 'local')`,
			Args: []any{stageID, now},
		},
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		if runID != 0 {
			_, _ = s.db.Execute(ctx, `DELETE FROM import_runs WHERE id = ?`, runID)
		}
		return ImportSummary{}, err
	}
	if runID != 0 {
		if err := s.saveImportSheets(ctx, runID, report); err != nil {
			return ImportSummary{}, err
		}
	}
	rows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM baseline_aggregates WHERE stage_id = ?`, stageID)
	if err != nil {
		return ImportSummary{}, err
	}
	summary := ImportSummary{Warnings: report.Warnings}
	if len(rows.Values) > 0 {
		count, _ := rows.Values[0][0].(int64)
		summary.Baselines = int(count)
	}
	return summary, nil
}

func (s *Store) stageByID(ctx context.Context, cohortID, stageID int64) (migrationBatch, error) {
	rows, err := s.db.Query(ctx, `SELECT cs.id, cs.academic_year_start, cs.grade_level
		FROM cohort_stages cs WHERE cs.id = ? AND cs.cohort_id = ?`, stageID, cohortID)
	if err != nil {
		return migrationBatch{}, err
	}
	if len(rows.Values) == 0 {
		return migrationBatch{}, ErrNotFound
	}
	id, _ := rows.Values[0][0].(int64)
	start, _ := rows.Values[0][1].(int64)
	grade, _ := rows.Values[0][2].(string)
	return migrationBatch{
		ID: id, AcademicYear: fmt.Sprintf("%d", start), Grade: grade,
		CohortID: cohortID, StageID: id, StageKey: grade,
	}, nil
}
