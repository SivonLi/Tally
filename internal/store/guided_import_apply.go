package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// ApplyStageImport 校验并写入一个阶段数据导入，同时保存变更记录和问题清单。
func (s *Store) ApplyStageImport(ctx context.Context, cohortID, stageID int64, req StageImportRequest) (ImportSummary, error) {
	stage, err := s.stageByID(ctx, cohortID, stageID)
	if err != nil {
		return ImportSummary{}, err
	}
	preview, err := s.ValidateStageImport(ctx, cohortID, stageID, req)
	if err != nil {
		return ImportSummary{}, err
	}
	if preview.Blocking > 0 {
		return ImportSummary{}, fmt.Errorf("%w: %s", ErrImportBlocked, issueCodeSummary(preview.Issues))
	}
	runID, reused, err := s.beginStageImportRun(ctx, stage, req, preview)
	if err != nil {
		return ImportSummary{}, err
	}
	var statements []sqlite.Statement
	var changes []importChange
	switch req.Kind {
	case "classes":
		statements, changes, err = s.classImportStatements(ctx, stage, req, runID)
	case "students":
		statements, changes, err = s.rosterImportStatements(ctx, stage, req, runID)
	case "teachers":
		statements, changes, err = s.teacherImportStatements(ctx, stage, req, runID)
	case "assignments":
		statements, changes, err = s.assignmentImportStatements(ctx, stage, req, runID)
	case "baselines":
		statements, changes, err = s.baselineImportStatements(ctx, stage, req, runID)
	default:
		err = fmt.Errorf("未知的数据项 %q", req.Kind)
	}
	if err != nil {
		if !reused {
			s.deleteImportRun(ctx, runID)
		}
		return ImportSummary{}, err
	}
	statements = append(statements, s.importChangeStatements(runID, changes)...)
	statements = append(statements, s.importIssueStatements(runID, preview.Issues)...)
	statements = append(statements, s.importSheetStatements(runID, req.Report)...)
	now := s.now().Format(time.RFC3339Nano)
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
				VALUES ('import', 'stage', ?, ?, ?, 'local')`,
			Args: []any{stageID, fmt.Sprintf(`{"kind":%q,"run":%d}`, req.Kind, runID), now},
		},
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		if !reused {
			s.deleteImportRun(ctx, runID)
		}
		return ImportSummary{}, err
	}
	summary := ImportSummary{
		Inserted: preview.New, Updated: preview.Update, Skipped: preview.Skip,
	}
	for _, issue := range preview.Issues {
		if issue.Severity != "warning" {
			continue
		}
		summary.Warnings = append(summary.Warnings, issue.Code)
	}
	s.fillImportSummary(ctx, stageID, &summary)
	return summary, nil
}

func (s *Store) beginStageImportRun(ctx context.Context, stage migrationBatch, req StageImportRequest, preview ImportPreview) (int64, bool, error) {
	hash := req.Report.FileHash
	if hash == "" {
		hash = fmt.Sprintf("manual-%d", s.now().UnixNano())
	}
	rows, err := s.db.Query(ctx, `SELECT id, status FROM import_runs
		WHERE scope_type = 'stage' AND scope_id = ? AND data_kind = ? AND file_hash = ?`,
		stage.StageID, req.Kind, hash)
	if err != nil {
		return 0, false, err
	}
	mapping := ""
	if len(req.Mapping) != 0 {
		encoded, err := json.Marshal(req.Mapping)
		if err != nil {
			return 0, false, err
		}
		mapping = string(encoded)
	}
	stored := req.Report.StoredPath
	if stored == "" {
		stored = req.Report.Path
	}
	now := s.now().Format(time.RFC3339Nano)
	if len(rows.Values) > 0 {
		id, _ := rows.Values[0][0].(int64)
		status, _ := rows.Values[0][1].(string)
		if status == "imported" {
			return 0, false, ErrDuplicateImport
		}
		if _, err := s.db.Execute(ctx, `UPDATE import_runs SET status = 'imported', stored_path = ?,
			relative_path = ?, converted_path = ?, mapping_json = ?, row_count = ?,
			error_count = ?, warning_count = ?, created_at = ? WHERE id = ?`,
			stored, req.Report.Path, req.Report.ConvertedPath, mapping, req.Report.RowCount,
			preview.Blocking, preview.Warnings, now, id); err != nil {
			return 0, false, err
		}
		if _, err := s.db.Execute(ctx, `DELETE FROM import_changes WHERE import_run_id = ?`, id); err != nil {
			return 0, false, err
		}
		if _, err := s.db.Execute(ctx, `DELETE FROM import_issues WHERE import_run_id = ?`, id); err != nil {
			return 0, false, err
		}
		return id, true, nil
	}
	result, err := s.db.Execute(ctx, `INSERT INTO import_runs(
		scope_type, scope_id, data_kind, file_hash, stored_path, relative_path, converted_path,
		mapping_json, status, row_count, error_count, warning_count, created_at)
		VALUES ('stage', ?, ?, ?, ?, ?, ?, ?, 'imported', ?, ?, ?, ?)`,
		stage.StageID, req.Kind, hash, stored, req.Report.Path, req.Report.ConvertedPath,
		mapping, req.Report.RowCount, preview.Blocking, preview.Warnings, now)
	if err != nil {
		return 0, false, err
	}
	return result.LastInsertID, false, nil
}

func (s *Store) deleteImportRun(ctx context.Context, runID int64) {
	if runID == 0 {
		return
	}
	_, _ = s.db.Execute(ctx, `DELETE FROM import_runs WHERE id = ?`, runID)
}

func (s *Store) classImportStatements(ctx context.Context, stage migrationBatch, req StageImportRequest, runID int64) ([]sqlite.Statement, []importChange, error) {
	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	var changes []importChange
	seen := map[string]bool{}
	for _, raw := range req.Data.Classes {
		name := normalizeClassName(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		rows, err := s.db.Query(ctx, `SELECT id FROM classes WHERE cohort_id = ? AND name = ?`,
			stage.CohortID, name)
		if err != nil {
			return nil, nil, err
		}
		if len(rows.Values) == 0 {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO classes(cohort_id, name, created_at, import_run_id)
					VALUES (?, ?, ?, ?)`,
				Args: []any{stage.CohortID, name, now, runID},
			})
			changes = append(changes, importChange{Entity: "class", Action: "insert", Before: map[string]any{
				"cohort_id": stage.CohortID, "name": name,
			}})
		}
		rows, err = s.db.Query(ctx, `SELECT 1 FROM class_stage_info i JOIN classes c ON c.id = i.class_id
			WHERE c.cohort_id = ? AND c.name = ? AND i.stage_id = ?`, stage.CohortID, name, stage.StageID)
		if err != nil {
			return nil, nil, err
		}
		if len(rows.Values) == 0 {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO class_stage_info(class_id, stage_id, status, import_run_id)
					SELECT id, ?, 'active', ? FROM classes WHERE cohort_id = ? AND name = ?`,
				Args: []any{stage.StageID, runID, stage.CohortID, name},
			})
			changes = append(changes, importChange{Entity: "class_stage", Action: "insert", Before: map[string]any{
				"cohort_id": stage.CohortID, "name": name, "stage_id": stage.StageID,
			}})
		}
	}
	return statements, changes, nil
}

func (s *Store) rosterImportStatements(ctx context.Context, stage migrationBatch, req StageImportRequest, runID int64) ([]sqlite.Statement, []importChange, error) {
	now := s.now().Format(time.RFC3339Nano)
	classes, err := s.stageClassMap(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	students, err := s.studentRowMap(ctx)
	if err != nil {
		return nil, nil, err
	}
	members, err := s.membershipMap(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	cohorts, err := s.studentCohortSet(ctx, stage.CohortID)
	if err != nil {
		return nil, nil, err
	}
	classNames := append([]string(nil), req.Data.Classes...)
	for _, student := range req.Data.Students {
		classNames = append(classNames, student.ClassName)
	}
	var statements []sqlite.Statement
	var changes []importChange
	for _, raw := range classNames {
		name := normalizeClassName(raw)
		if name == "" || classes[name] != 0 {
			continue
		}
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO classes(cohort_id, name, created_at, import_run_id)
					VALUES (?, ?, ?, ?)`,
				Args: []any{stage.CohortID, name, now, runID},
			},
			sqlite.Statement{
				SQL: `INSERT INTO class_stage_info(class_id, stage_id, status, import_run_id)
					SELECT id, ?, 'active', ? FROM classes WHERE cohort_id = ? AND name = ?`,
				Args: []any{stage.StageID, runID, stage.CohortID, name},
			})
		changes = append(changes,
			importChange{Entity: "class", Action: "insert", Before: map[string]any{
				"cohort_id": stage.CohortID, "name": name,
			}},
			importChange{Entity: "class_stage", Action: "insert", Before: map[string]any{
				"cohort_id": stage.CohortID, "name": name, "stage_id": stage.StageID,
			}})
		classes[name] = -1
	}
	seen := map[string]bool{}
	for _, student := range req.Data.Students {
		if student.StudentNo == "" || student.Name == "" || seen[student.StudentNo] {
			continue
		}
		seen[student.StudentNo] = true
		className := normalizeClassName(student.ClassName)
		status := student.Status
		if status == "" {
			status = "active"
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO students(student_no, name, created_at, import_run_id, source_sheet, source_row)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(student_no) DO UPDATE SET name = excluded.name,
					import_run_id = excluded.import_run_id, source_sheet = excluded.source_sheet,
					source_row = excluded.source_row`,
			Args: []any{student.StudentNo, student.Name, now, runID, student.SourceSheet, student.SourceRow},
		})
		if before, ok := students[student.StudentNo]; ok {
			changes = append(changes, importChange{Entity: "student", Action: "update", Before: before})
		} else {
			changes = append(changes, importChange{Entity: "student", Action: "insert", Before: map[string]any{
				"student_no": student.StudentNo,
			}})
		}
		if !cohorts[student.StudentNo] {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO student_cohorts(student_id, cohort_id, status, import_run_id, source_sheet, source_row)
					SELECT id, ?, ?, ?, ?, ? FROM students WHERE student_no = ?
					ON CONFLICT(student_id, cohort_id) DO NOTHING`,
				Args: []any{stage.CohortID, status, runID, student.SourceSheet,
					student.SourceRow, student.StudentNo},
			})
			changes = append(changes, importChange{Entity: "student_cohort", Action: "insert", Before: map[string]any{
				"student_no": student.StudentNo, "cohort_id": stage.CohortID,
			}})
		}
		statements = append(statements, sqlite.Statement{
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
			Args: []any{stage.StageID, status, boolInt(student.Walking), student.Track,
				student.Elective, boolInt(student.Japanese), optionalFloat(student.TotalScore),
				runID, student.SourceSheet, student.SourceRow, student.StudentNo,
				stage.CohortID, className},
		})
		if before, ok := members[student.StudentNo]; ok {
			changes = append(changes, importChange{Entity: "student_membership", Action: "update", Before: before})
		} else {
			changes = append(changes, importChange{Entity: "student_membership", Action: "insert", Before: map[string]any{
				"student_no": student.StudentNo, "stage_id": stage.StageID,
			}})
		}
	}
	return statements, changes, nil
}

func (s *Store) teacherImportStatements(ctx context.Context, stage migrationBatch, req StageImportRequest, runID int64) ([]sqlite.Statement, []importChange, error) {
	now := s.now().Format(time.RFC3339Nano)
	teachers, err := s.teacherRowMap(ctx)
	if err != nil {
		return nil, nil, err
	}
	var statements []sqlite.Statement
	var changes []importChange
	for _, teacher := range req.Data.TeacherList {
		key := teacher.TeacherNo
		if key == "" {
			key = teacher.Name + "\x00" + teacher.Subject
		}
		if _, ok := teachers[key]; ok {
			continue
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO teachers(teacher_no, name, subject, created_at, import_run_id, source_sheet, source_row)
				SELECT ?, ?, ?, ?, ?, ?, ? WHERE NOT EXISTS (
					SELECT 1 FROM teachers WHERE ` + teacherMatch(teacher) + `)`,
			Args: append([]any{teacher.TeacherNo, teacher.Name, teacher.Subject, now, runID,
				teacher.SourceSheet, teacher.SourceRow}, teacherMatchArgs(teacher)...),
		})
		changes = append(changes, importChange{Entity: "teacher", Action: "insert", Before: map[string]any{
			"teacher_no": teacher.TeacherNo, "name": teacher.Name, "subject": teacher.Subject,
		}})
		teachers[key] = map[string]any{}
	}
	return statements, changes, nil
}

func teacherMatch(teacher ImportTeacher) string {
	if teacher.TeacherNo != "" {
		return "teacher_no = ?"
	}
	return "teacher_no = '' AND name = ? AND subject = ?"
}

func teacherMatchArgs(teacher ImportTeacher) []any {
	if teacher.TeacherNo != "" {
		return []any{teacher.TeacherNo}
	}
	return []any{teacher.Name, teacher.Subject}
}

func (s *Store) assignmentImportStatements(ctx context.Context, stage migrationBatch, req StageImportRequest, runID int64) ([]sqlite.Statement, []importChange, error) {
	now := s.now().Format(time.RFC3339Nano)
	classes, err := s.stageClassMap(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	teachers, err := s.teacherRowMap(ctx)
	if err != nil {
		return nil, nil, err
	}
	groups, err := s.groupRowMap(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	assignments, err := s.assignmentKeySet(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	mappings, err := s.studentTeachingMap(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	var statements []sqlite.Statement
	var changes []importChange
	for _, raw := range req.Data.Classes {
		name := normalizeClassName(raw)
		if name == "" || classes[name] != 0 {
			continue
		}
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO classes(cohort_id, name, created_at, import_run_id)
					VALUES (?, ?, ?, ?)`,
				Args: []any{stage.CohortID, name, now, runID},
			},
			sqlite.Statement{
				SQL: `INSERT INTO class_stage_info(class_id, stage_id, status, import_run_id)
					SELECT id, ?, 'active', ? FROM classes WHERE cohort_id = ? AND name = ?`,
				Args: []any{stage.StageID, runID, stage.CohortID, name},
			})
		changes = append(changes,
			importChange{Entity: "class", Action: "insert", Before: map[string]any{
				"cohort_id": stage.CohortID, "name": name,
			}},
			importChange{Entity: "class_stage", Action: "insert", Before: map[string]any{
				"cohort_id": stage.CohortID, "name": name, "stage_id": stage.StageID,
			}})
		classes[name] = -1
	}
	seen := map[string]bool{}
	for _, assignment := range req.Data.Assignments {
		className := normalizeClassName(assignment.ClassName)
		if className == "" || assignment.Subject == "" || assignment.TeacherName == "" {
			continue
		}
		teacherKey := assignment.TeacherNo
		if teacherKey == "" {
			teacherKey = assignment.TeacherName + "\x00" + assignment.Subject
		}
		if _, ok := teachers[teacherKey]; !ok {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO teachers(teacher_no, name, subject, created_at, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, ?, ?, ?, ?)
					ON CONFLICT DO NOTHING`,
				Args: []any{assignment.TeacherNo, assignment.TeacherName, assignment.Subject,
					now, runID, assignment.SourceSheet, assignment.SourceRow},
			})
			changes = append(changes, importChange{Entity: "teacher", Action: "insert", Before: map[string]any{
				"teacher_no": assignment.TeacherNo, "name": assignment.TeacherName,
				"subject": assignment.Subject,
			}})
			teachers[teacherKey] = map[string]any{}
		}
		groupKey := assignment.Subject + "\x00" + className
		if _, ok := groups[groupKey]; !ok {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO teaching_groups(
					stage_id, subject, name, kind, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, 'admin', ?, ?, ?)
					ON CONFLICT(stage_id, subject, name) DO NOTHING`,
				Args: []any{stage.StageID, assignment.Subject, className, runID,
					assignment.SourceSheet, assignment.SourceRow},
			})
			changes = append(changes, importChange{Entity: "teaching_group", Action: "insert", Before: map[string]any{
				"stage_id": stage.StageID, "subject": assignment.Subject, "name": className,
			}})
			groups[groupKey] = map[string]any{}
		}
		key := assignment.TeacherName + "\x00" + className + "\x00" + assignment.Subject
		if assignments[key] || seen[key] {
			continue
		}
		seen[key] = true
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assignments(teacher_id, teaching_group_id, import_run_id, source_sheet, source_row)
				SELECT t.id, g.id, ?, ?, ? FROM teachers t, teaching_groups g
				WHERE ` + teacherMatchFor("t", assignment) + `
					AND g.stage_id = ? AND g.subject = ? AND g.name = ?
				ON CONFLICT(teacher_id, teaching_group_id) DO NOTHING`,
			Args: append([]any{runID, assignment.SourceSheet, assignment.SourceRow},
				append(teacherMatchArgsFor(assignment), stage.StageID, assignment.Subject, className)...),
		})
		changes = append(changes, importChange{Entity: "assignment", Action: "insert", Before: map[string]any{
			"teacher_no": assignment.TeacherNo, "teacher_name": assignment.TeacherName,
			"subject": assignment.Subject, "stage_id": stage.StageID, "group_name": className,
		}})
		statements = append(statements, s.studentTeachingStatements(ctx, stage, className,
			assignment.Subject, runID, mappings, &changes)...)
	}
	return statements, changes, nil
}

func teacherMatchFor(alias string, assignment ImportAssignment) string {
	if assignment.TeacherNo != "" {
		return alias + ".teacher_no = ?"
	}
	return alias + ".teacher_no = '' AND " + alias + ".name = ? AND " + alias + ".subject = ?"
}

func teacherMatchArgsFor(assignment ImportAssignment) []any {
	if assignment.TeacherNo != "" {
		return []any{assignment.TeacherNo}
	}
	return []any{assignment.TeacherName, assignment.Subject}
}

func (s *Store) studentTeachingStatements(ctx context.Context, stage migrationBatch, className, subject string, runID int64, mappings map[string]int64, changes *[]importChange) []sqlite.Statement {
	rows, err := s.db.Query(ctx, `SELECT s.student_no FROM students s
		JOIN student_stage_memberships m ON m.student_id = s.id
		JOIN classes c ON c.id = m.class_id
		WHERE m.stage_id = ? AND c.name = ?`, stage.StageID, className)
	if err != nil {
		return nil
	}
	var statements []sqlite.Statement
	for _, row := range rows.Values {
		studentNo, _ := row[0].(string)
		if studentNo == "" {
			continue
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO student_teaching_groups(
				student_id, stage_id, subject, teaching_group_id, import_run_id)
				SELECT m.student_id, m.stage_id, g.subject, g.id, ?
				FROM student_stage_memberships m
				JOIN classes c ON c.id = m.class_id
				JOIN teaching_groups g ON g.stage_id = m.stage_id AND g.name = c.name AND g.subject = ?
				JOIN students s ON s.id = m.student_id
				WHERE m.stage_id = ? AND c.name = ? AND s.student_no = ?
				ON CONFLICT(student_id, stage_id, subject) DO UPDATE SET
					teaching_group_id = excluded.teaching_group_id, import_run_id = excluded.import_run_id`,
			Args: []any{runID, subject, stage.StageID, className, studentNo},
		})
		key := studentNo + "\x00" + subject
		if before, ok := mappings[key]; ok {
			*changes = append(*changes, importChange{Entity: "student_teaching_group", Action: "update",
				Before: map[string]any{"student_no": studentNo, "stage_id": stage.StageID,
					"subject": subject, "teaching_group_id": before}})
		} else {
			*changes = append(*changes, importChange{Entity: "student_teaching_group", Action: "insert",
				Before: map[string]any{"student_no": studentNo, "stage_id": stage.StageID, "subject": subject}})
		}
	}
	return statements
}

func (s *Store) baselineImportStatements(ctx context.Context, stage migrationBatch, req StageImportRequest, runID int64) ([]sqlite.Statement, []importChange, error) {
	now := s.now().Format(time.RFC3339Nano)
	subjects, err := s.subjectFullMarks(ctx)
	if err != nil {
		return nil, nil, err
	}
	sets, err := s.baselineSetMap(ctx, stage.CohortID)
	if err != nil {
		return nil, nil, err
	}
	existing, err := s.baselineAggregateMap(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	stageExisting, err := s.stageBaselineAggregateMap(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	counts, err := s.classStudentCounts(ctx, stage.StageID)
	if err != nil {
		return nil, nil, err
	}
	var statements []sqlite.Statement
	var changes []importChange
	if sets["入口成绩"] == 0 {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO baseline_sets(cohort_id, name, source, kind, status, created_at, import_run_id)
				VALUES (?, '入口成绩', 'import', 'zhongkao', 'active', ?, ?)`,
			Args: []any{stage.CohortID, now, runID},
		})
	}
	seen := map[string]bool{}
	for _, baseline := range req.Data.ClassBaselines {
		className := normalizeClassName(baseline.ClassName)
		subject := baseline.Subject
		key := className + "\x00" + subject
		if seen[key] {
			continue
		}
		seen[key] = true
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO baseline_subjects(baseline_set_id, subject, full_mark)
					SELECT id, ?, ? FROM baseline_sets WHERE cohort_id = ? AND name = '入口成绩'`,
				Args: []any{subject, subjects[subject], stage.CohortID},
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
				Args: []any{stage.StageID, subject, baseline.Average, stage.StageID,
					floatOrZero(baseline.Ratio), runID, baseline.SourceSheet, baseline.SourceRow,
					stage.CohortID, stage.CohortID, className},
			})
		if before, ok := existing[key]; ok {
			changes = append(changes, importChange{Entity: "baseline_aggregate", Action: "update", Before: before})
		} else {
			changes = append(changes, importChange{Entity: "baseline_aggregate", Action: "insert", Before: map[string]any{
				"cohort_id": stage.CohortID, "stage_id": stage.StageID, "class_name": className,
				"subject": subject,
			}})
		}
	}
	for subject, average := range weightedBaselineAverages(req.Data.ClassBaselines, counts) {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO baseline_subjects(baseline_set_id, subject, full_mark)
				SELECT id, ?, ? FROM baseline_sets WHERE cohort_id = ? AND name = '入口成绩'
				ON CONFLICT(baseline_set_id, subject) DO NOTHING`,
			Args: []any{subject, subjects[subject], stage.CohortID},
		}, sqlite.Statement{
			SQL: `INSERT INTO baseline_aggregates(
				baseline_set_id, stage_id, scope_type, scope_id, subject, average, count, ratio,
				import_run_id, source_sheet, source_row)
				SELECT b.id, ?, 'stage', ?, ?, ?, (
					SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?), 0, ?, '', 0
				FROM baseline_sets b WHERE b.cohort_id = ? AND b.name = '入口成绩'
				ON CONFLICT(baseline_set_id, stage_id, scope_type, scope_id, subject) DO UPDATE SET
					average = excluded.average, count = excluded.count,
					import_run_id = excluded.import_run_id`,
			Args: []any{stage.StageID, stage.StageID, subject, average, stage.StageID, runID, stage.CohortID},
		})
		if before, ok := stageExisting[subject]; ok {
			changes = append(changes, importChange{Entity: "baseline_stage_aggregate", Action: "update", Before: before})
		} else {
			changes = append(changes, importChange{Entity: "baseline_stage_aggregate", Action: "insert", Before: map[string]any{
				"stage_id": stage.StageID, "subject": subject,
			}})
		}
	}
	return statements, changes, nil
}

func weightedBaselineAverages(baselines []ImportClassBaseline, counts map[string]int) map[string]float64 {
	sums := map[string]float64{}
	weights := map[string]int{}
	for _, baseline := range baselines {
		weight := counts[normalizeClassName(baseline.ClassName)]
		if weight <= 0 {
			weight = 1
		}
		sums[baseline.Subject] += baseline.Average * float64(weight)
		weights[baseline.Subject] += weight
	}
	result := map[string]float64{}
	for subject, weight := range weights {
		if weight == 0 {
			continue
		}
		result[subject] = sums[subject] / float64(weight)
	}
	return result
}

type importChange struct {
	Entity string
	Action string
	Before map[string]any
}

func (s *Store) importChangeStatements(runID int64, changes []importChange) []sqlite.Statement {
	statements := make([]sqlite.Statement, 0, len(changes))
	for _, change := range changes {
		encoded, err := json.Marshal(change.Before)
		if err != nil {
			continue
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_changes(import_run_id, entity, entity_id, action, before_json, after_json)
				VALUES (?, ?, 0, ?, ?, '{}')`,
			Args: []any{runID, change.Entity, change.Action, string(encoded)},
		})
	}
	return statements
}

func (s *Store) importIssueStatements(runID int64, issues []ImportIssue) []sqlite.Statement {
	statements := make([]sqlite.Statement, 0, len(issues))
	for _, issue := range issues {
		encoded, err := json.Marshal(issue.Args)
		if err != nil {
			encoded = []byte("[]")
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_issues(import_run_id, severity, row_number, column_name, code, message, resolved)
				VALUES (?, ?, ?, ?, ?, ?, 0)`,
			Args: []any{runID, issue.Severity, issue.Row, issue.Column, issue.Code, string(encoded)},
		})
	}
	return statements
}

func (s *Store) importSheetStatements(runID int64, report ImportReport) []sqlite.Statement {
	statements := []sqlite.Statement{{SQL: `DELETE FROM import_sheets WHERE import_run_id = ?`, Args: []any{runID}}}
	seen := map[string]bool{}
	for _, sheet := range report.Sheets {
		if seen[sheet.Name] {
			continue
		}
		seen[sheet.Name] = true
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_sheets(import_run_id, sheet_name, role, header_row, data_range, is_current)
				VALUES (?, ?, ?, ?, ?, 1)`,
			Args: []any{runID, sheet.Name, sheet.Role, sheet.HeaderRow,
				fmt.Sprintf("%d rows", sheet.Rows)},
		})
	}
	return statements
}

func (s *Store) fillImportSummary(ctx context.Context, stageID int64, summary *ImportSummary) {
	rows, err := s.db.Query(ctx, `SELECT
		(SELECT COUNT(*) FROM class_stage_info WHERE stage_id = ?),
		(SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?),
		(SELECT COUNT(DISTINCT a.teacher_id) FROM assignments a
			JOIN teaching_groups g ON g.id = a.teaching_group_id WHERE g.stage_id = ?),
		(SELECT COUNT(*) FROM teaching_groups WHERE stage_id = ?),
		(SELECT COUNT(*) FROM assignments a JOIN teaching_groups g ON g.id = a.teaching_group_id
			WHERE g.stage_id = ?),
		(SELECT COUNT(*) FROM baseline_aggregates WHERE stage_id = ?)`,
		stageID, stageID, stageID, stageID, stageID, stageID)
	if err != nil || len(rows.Values) == 0 {
		return
	}
	classes, _ := rows.Values[0][0].(int64)
	students, _ := rows.Values[0][1].(int64)
	teachers, _ := rows.Values[0][2].(int64)
	groups, _ := rows.Values[0][3].(int64)
	assignments, _ := rows.Values[0][4].(int64)
	baselines, _ := rows.Values[0][5].(int64)
	summary.Classes = int(classes)
	summary.Students = int(students)
	summary.Teachers = int(teachers)
	summary.TeachingGroups = int(groups)
	summary.Assignments = int(assignments)
	summary.Baselines = int(baselines)
}

func (s *Store) stageClassMap(ctx context.Context, stageID int64) (map[string]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT c.name, c.id FROM classes c
		JOIN class_stage_info i ON i.class_id = c.id WHERE i.stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]int64{}
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		id, _ := row[1].(int64)
		result[name] = id
	}
	return result, nil
}

func (s *Store) classStudentCounts(ctx context.Context, stageID int64) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `SELECT c.name, COUNT(*) FROM student_stage_memberships m
		JOIN classes c ON c.id = m.class_id WHERE m.stage_id = ? GROUP BY c.name`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]int{}
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		count, _ := row[1].(int64)
		result[name] = int(count)
	}
	return result, nil
}

func (s *Store) studentRowMap(ctx context.Context) (map[string]map[string]any, error) {
	rows, err := s.db.Query(ctx, `SELECT student_no, name, import_run_id, source_sheet, source_row FROM students`)
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]any{}
	for _, row := range rows.Values {
		no, _ := row[0].(string)
		result[no] = map[string]any{
			"student_no": no, "name": stringValue(row[1]), "import_run_id": intValue(row[2]),
			"source_sheet": stringValue(row[3]), "source_row": intValue(row[4]),
		}
	}
	return result, nil
}

func (s *Store) membershipMap(ctx context.Context, stageID int64) (map[string]map[string]any, error) {
	rows, err := s.db.Query(ctx, `SELECT s.student_no, m.class_id, m.status, m.walking, m.track,
		m.elective, m.japanese, m.total_score, m.import_run_id, m.source_sheet, m.source_row
		FROM student_stage_memberships m JOIN students s ON s.id = m.student_id WHERE m.stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]any{}
	for _, row := range rows.Values {
		no, _ := row[0].(string)
		result[no] = map[string]any{
			"student_no": no, "stage_id": stageID, "class_id": intValue(row[1]), "status": stringValue(row[2]),
			"walking": intValue(row[3]), "track": stringValue(row[4]), "elective": stringValue(row[5]),
			"japanese": intValue(row[6]), "total_score": floatPointerValue(row[7]),
			"import_run_id": intValue(row[8]), "source_sheet": stringValue(row[9]),
			"source_row": intValue(row[10]),
		}
	}
	return result, nil
}

func (s *Store) studentCohortSet(ctx context.Context, cohortID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT s.student_no FROM students s
		JOIN student_cohorts c ON c.student_id = s.id WHERE c.cohort_id = ?`, cohortID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		no, _ := row[0].(string)
		result[no] = true
	}
	return result, nil
}

func (s *Store) teacherRowMap(ctx context.Context) (map[string]map[string]any, error) {
	rows, err := s.db.Query(ctx, `SELECT teacher_no, name, subject FROM teachers`)
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]any{}
	for _, row := range rows.Values {
		no, _ := row[0].(string)
		name, _ := row[1].(string)
		subject, _ := row[2].(string)
		key := no
		if key == "" {
			key = name + "\x00" + subject
		}
		result[key] = map[string]any{"teacher_no": no, "name": name, "subject": subject}
	}
	return result, nil
}

func (s *Store) groupRowMap(ctx context.Context, stageID int64) (map[string]map[string]any, error) {
	rows, err := s.db.Query(ctx, `SELECT subject, name FROM teaching_groups WHERE stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]any{}
	for _, row := range rows.Values {
		subject, _ := row[0].(string)
		name, _ := row[1].(string)
		result[subject+"\x00"+name] = map[string]any{
			"stage_id": stageID, "subject": subject, "name": name,
		}
	}
	return result, nil
}

func (s *Store) studentTeachingMap(ctx context.Context, stageID int64) (map[string]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT s.student_no, g.subject, g.id
		FROM student_teaching_groups g JOIN students s ON s.id = g.student_id WHERE g.stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]int64{}
	for _, row := range rows.Values {
		no, _ := row[0].(string)
		subject, _ := row[1].(string)
		id, _ := row[2].(int64)
		result[no+"\x00"+subject] = id
	}
	return result, nil
}

func (s *Store) baselineSetMap(ctx context.Context, cohortID int64) (map[string]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT name, id FROM baseline_sets WHERE cohort_id = ?`, cohortID)
	if err != nil {
		return nil, err
	}
	result := map[string]int64{}
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		id, _ := row[1].(int64)
		result[name] = id
	}
	return result, nil
}

func (s *Store) baselineAggregateMap(ctx context.Context, stageID int64) (map[string]map[string]any, error) {
	rows, err := s.db.Query(ctx, `SELECT c.cohort_id, c.name, a.subject, a.average, a.count, a.ratio,
		a.import_run_id, a.source_sheet, a.source_row
		FROM baseline_aggregates a JOIN classes c ON c.id = a.scope_id
		WHERE a.stage_id = ? AND a.scope_type = 'class'`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]any{}
	for _, row := range rows.Values {
		cohortID := intValue(row[0])
		name, _ := row[1].(string)
		subject, _ := row[2].(string)
		result[name+"\x00"+subject] = map[string]any{
			"cohort_id": cohortID, "stage_id": stageID, "class_name": name, "subject": subject,
			"average": floatValue(row[3]), "count": intValue(row[4]), "ratio": floatValue(row[5]),
			"import_run_id": intValue(row[6]), "source_sheet": stringValue(row[7]),
			"source_row": intValue(row[8]),
		}
	}
	return result, nil
}

func (s *Store) stageBaselineAggregateMap(ctx context.Context, stageID int64) (map[string]map[string]any, error) {
	rows, err := s.db.Query(ctx, `SELECT subject, average, count, import_run_id, source_sheet, source_row
		FROM baseline_aggregates WHERE stage_id = ? AND scope_type = 'stage'`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]any{}
	for _, row := range rows.Values {
		subject, _ := row[0].(string)
		result[subject] = map[string]any{
			"stage_id": stageID, "subject": subject,
			"average": floatValue(row[1]), "count": intValue(row[2]),
			"import_run_id": intValue(row[3]), "source_sheet": stringValue(row[4]),
			"source_row": intValue(row[5]),
		}
	}
	return result, nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func intValue(value any) int64 {
	switch item := value.(type) {
	case int64:
		return item
	case int:
		return int64(item)
	case float64:
		return int64(item)
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch item := value.(type) {
	case float64:
		return item
	case int64:
		return float64(item)
	default:
		return 0
	}
}

func floatPointerValue(value any) any {
	if value == nil {
		return nil
	}
	return floatValue(value)
}
