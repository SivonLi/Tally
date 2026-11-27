package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

type importChange struct {
	TableName string
	RowID     int64
	Action    string
	Before    any
	After     any
}

// StoreImportFile 把外部工作簿复制到软件数据目录下 files/，按文件哈希去重存储。
func (s *Store) StoreImportFile(sourcePath, fileHash string) (string, error) {
	if s.rootDir == "" {
		return sourcePath, nil
	}
	ext := filepath.Ext(sourcePath)
	if ext == "" {
		ext = ".xlsx"
	}
	targetDir := filepath.Join(s.rootDir, "files")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	targetPath := filepath.Join(targetDir, fileHash+ext)
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	}
	in, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(targetPath)
		return "", err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(targetPath)
		return "", err
	}
	return targetPath, nil
}

// ValidateStageImport 在写入前校验阶段数据导入，返回新增/更新/跳过数量、阻断错误及非阻断提示（含教师跨年级兼任）。
func (s *Store) ValidateStageImport(ctx context.Context, cohortID, stageID int64, req StageImportRequest) (ImportPreview, error) {
	cohort, phase, err := s.resolveCohortPhaseScope(ctx, cohortID, stageID)
	if err != nil {
		return ImportPreview{}, err
	}
	preview := ImportPreview{Kind: req.Kind}
	preview.Issues = append(preview.Issues, req.Issues...)
	switch req.Kind {
	case "classes":
		err = s.validateClassImport(ctx, cohort, phase, req, &preview)
	case "students":
		err = s.validateRosterImport(ctx, cohort, phase, req, &preview)
	case "teachers":
		err = s.validateTeacherImport(ctx, cohort, req, &preview)
	case "assignments":
		err = s.validateAssignmentImport(ctx, cohort, phase, req, &preview)
	case "baselines":
		err = s.validateBaselineImport(ctx, cohort, phase, req, &preview)
	default:
		err = fmt.Errorf("未知的数据项 %q", req.Kind)
	}
	if err != nil {
		return ImportPreview{}, err
	}
	for _, issue := range preview.Issues {
		if issue.Severity == "blocking" {
			preview.Blocking++
		} else {
			preview.Warnings++
		}
	}
	return preview, nil
}

func (s *Store) validateClassImport(ctx context.Context, cohort Cohort, phase string, req StageImportRequest, preview *ImportPreview) error {
	existing, err := s.phaseClassIDMap(ctx, cohort.ID, phase)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for index, raw := range req.Data.Classes {
		name := normalizeClassName(raw)
		if name == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: index + 2, Column: "class_name", Code: "missing_field",
			})
			continue
		}
		if seen[name] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: index + 2, Column: "class_name", Code: "duplicate_class", Args: []string{name},
			})
			preview.Skip++
			continue
		}
		seen[name] = true
		if existing[name] > 0 {
			preview.Update++
		} else {
			preview.New++
		}
	}
	return nil
}

func (s *Store) validateRosterImport(ctx context.Context, cohort Cohort, phase string, req StageImportRequest, preview *ImportPreview) error {
	classes, err := s.phaseClassIDMap(ctx, cohort.ID, phase)
	if err != nil {
		return err
	}
	existingNos, err := s.cohortStudentNoSet(ctx, cohort.ID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	newClasses := map[string]bool{}
	for _, student := range req.Data.Students {
		if strings.TrimSpace(student.StudentNo) == "" && strings.TrimSpace(student.CandidateCode) == "" && strings.TrimSpace(student.Name) == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: student.SourceRow, Column: "student_no", Code: "missing_field",
			})
			continue
		}
		if strings.TrimSpace(student.Name) == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: student.SourceRow, Column: "student_name", Code: "missing_field",
			})
			continue
		}
		cname := normalizeClassName(student.ClassName)
		if cname == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: student.SourceRow, Column: "class_name", Code: "missing_field",
			})
			continue
		}
		if classes[cname] == 0 {
			newClasses[cname] = true
		}
		sno := strings.TrimSpace(student.StudentNo)
		key := sno
		if key == "" {
			key = strings.TrimSpace(student.CandidateCode) + "\x00" + strings.TrimSpace(student.Name) + "\x00" + cname
		}
		if seen[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: student.SourceRow, Column: "student_no", Code: "duplicate_student", Args: []string{sno},
			})
			preview.Skip++
			continue
		}
		seen[key] = true
		if existingNos[sno] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	if len(newClasses) > 0 {
		preview.Issues = append(preview.Issues, ImportIssue{
			Severity: "warning", Column: "class_name", Code: "classes_created", Args: []string{strconv.Itoa(len(newClasses))},
		})
	}
	return nil
}

func (s *Store) validateTeacherImport(ctx context.Context, cohort Cohort, req StageImportRequest, preview *ImportPreview) error {
	existingByNo, existingByNameSubj, existingByName, err := s.schoolTeacherMaps(ctx)
	if err != nil {
		return err
	}
	seenNo := map[string]string{}
	for _, teacher := range req.Data.TeacherList {
		name := strings.TrimSpace(teacher.Name)
		no := strings.TrimSpace(teacher.TeacherNo)
		subj := strings.TrimSpace(teacher.Subject)
		if name == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: teacher.SourceRow, Column: "teacher_name", Code: "missing_field",
			})
			continue
		}
		if no != "" {
			if prevName, ok := seenNo[no]; ok && prevName != name {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: teacher.SourceRow, Column: "teacher_no",
					Code: "duplicate_teacher_no", Args: []string{no},
				})
				continue
			}
			seenNo[no] = name
			if existingName, ok := existingByNo[no]; ok && existingName != name {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: teacher.SourceRow, Column: "teacher_no",
					Code: "teacher_no_conflict", Args: []string{no, existingName},
				})
				continue
			}
		}
		if _, ok := existingByNo[no]; ok && no != "" {
			preview.Update++
		} else if subj != "" && len(existingByNameSubj[name+"\x00"+subj]) > 0 {
			preview.Update++
		} else if subj == "" && len(existingByName[name]) > 0 {
			preview.Update++
		} else {
			preview.New++
		}
	}
	// 同步检查是否与其余在读年级形成跨年级在任提示。
	for _, teacher := range req.Data.TeacherList {
		tid := s.lookupTeacherID(ctx, strings.TrimSpace(teacher.TeacherNo), strings.TrimSpace(teacher.Name), strings.TrimSpace(teacher.Subject))
		if tid <= 0 {
			continue
		}
		if warn, err := s.DetectCrossGradeWarnings(ctx, tid, cohort.ID); err == nil && warn != nil {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: teacher.SourceRow, Column: "teacher_name",
				Code: "teacher_cross_grade", Args: []string{warn.TeacherName, formatWarningTargets(warn.ExistingTargets)},
			})
		}
	}
	return nil
}

func (s *Store) validateAssignmentImport(ctx context.Context, cohort Cohort, phase string, req StageImportRequest, preview *ImportPreview) error {
	classes, err := s.phaseClassIDMap(ctx, cohort.ID, phase)
	if err != nil {
		return err
	}
	subjects, err := s.subjectFullMarksMap(ctx)
	if err != nil {
		return err
	}

	existingAssignRows, err := s.db.Query(ctx, `SELECT tg.subject, tg.name, t.name
		FROM teaching_assignments ta
		JOIN teaching_groups tg ON tg.id = ta.teaching_group_id
		JOIN teachers t ON t.id = ta.teacher_id
		WHERE ta.cohort_id = ? AND ta.phase = ? AND ta.status = 'active'`, cohort.ID, phase)
	if err != nil {
		return err
	}
	existingSet := map[string]bool{}
	for _, r := range existingAssignRows.Values {
		existingSet[stringValue(r[0])+"\x00"+stringValue(r[1])+"\x00"+stringValue(r[2])] = true
	}

	seen := map[string]bool{}
	newClasses := map[string]bool{}
	warnedCrossGrade := map[int64]bool{}
	for _, a := range req.Data.Assignments {
		cname := normalizeClassName(a.ClassName)
		tname := strings.TrimSpace(a.TeacherName)
		tno := strings.TrimSpace(a.TeacherNo)
		if cname == "" || a.Subject == "" || tname == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: a.SourceRow, Column: "teacher_name", Code: "missing_field",
			})
			continue
		}
		if _, ok := subjects[a.Subject]; !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: a.SourceRow, Column: "subject", Code: "unknown_subject", Args: []string{a.Subject},
			})
			continue
		}
		if classes[cname] == 0 {
			newClasses[cname] = true
		}
		key := a.Subject + "\x00" + cname + "\x00" + tname
		if seen[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: a.SourceRow, Column: "teacher_name",
				Code: "duplicate_assignment", Args: []string{cname, a.Subject, tname},
			})
			preview.Skip++
			continue
		}
		seen[key] = true
		if existingSet[key] {
			preview.Update++
		} else {
			preview.New++
		}
		tid := s.lookupTeacherID(ctx, tno, tname, strings.TrimSpace(a.Subject))
		if tid > 0 && !warnedCrossGrade[tid] {
			warnedCrossGrade[tid] = true
			if warn, err := s.DetectCrossGradeWarnings(ctx, tid, cohort.ID); err == nil && warn != nil {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "warning", Row: a.SourceRow, Column: "teacher_name",
					Code: "teacher_cross_grade", Args: []string{warn.TeacherName, formatWarningTargets(warn.ExistingTargets)},
				})
			}
		}
	}
	if len(newClasses) > 0 {
		preview.Issues = append(preview.Issues, ImportIssue{
			Severity: "warning", Column: "class_name", Code: "classes_created", Args: []string{strconv.Itoa(len(newClasses))},
		})
	}
	return nil
}

func (s *Store) validateBaselineImport(ctx context.Context, cohort Cohort, phase string, req StageImportRequest, preview *ImportPreview) error {
	classes, err := s.phaseClassIDMap(ctx, cohort.ID, phase)
	if err != nil {
		return err
	}
	subjects, err := s.subjectFullMarksMap(ctx)
	if err != nil {
		return err
	}
	existingRows, err := s.db.Query(ctx, `SELECT ov.scope_id, ov.subject FROM cohort_baseline_overrides ov
		WHERE ov.cohort_id = ? AND ov.phase = ? AND ov.scope_type = 'class'`, cohort.ID, phase)
	if err != nil {
		return err
	}
	existingSet := map[string]bool{}
	for _, r := range existingRows.Values {
		existingSet[fmt.Sprintf("%d\x00%s", intValue(r[0]), stringValue(r[1]))] = true
	}

	seen := map[string]bool{}
	for _, b := range req.Data.ClassBaselines {
		cname := normalizeClassName(b.ClassName)
		cid := classes[cname]
		if cid == 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: b.SourceRow, Column: "class_name", Code: "unknown_class", Args: []string{b.ClassName},
			})
			continue
		}
		if _, ok := subjects[b.Subject]; !ok && b.Subject != "total" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: b.SourceRow, Column: "subject", Code: "unknown_subject", Args: []string{b.Subject},
			})
			continue
		}
		if b.Ratio != nil && *b.Ratio <= 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: b.SourceRow, Column: "ratio", Code: "ratio_not_positive",
			})
			continue
		}
		key := fmt.Sprintf("%d\x00%s", cid, b.Subject)
		if seen[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: b.SourceRow, Column: "subject",
				Code: "duplicate_baseline", Args: []string{b.ClassName, b.Subject},
			})
			preview.Skip++
			continue
		}
		seen[key] = true
		if existingSet[key] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	return nil
}

func (s *Store) beginImportRun(ctx context.Context, scopeType string, scopeID int64, kind string, report ImportReport, mapping map[string]string, preview ImportPreview, allowDuplicate bool) (int64, bool, error) {
	hash := strings.TrimSpace(report.FileHash)
	if hash == "" {
		hash = fmt.Sprintf("manual-%d", s.now().UnixNano())
	}
	rows, err := s.db.Query(ctx, `SELECT id, status FROM import_runs
		WHERE scope_type = ? AND scope_id = ? AND kind = ? AND file_hash = ? LIMIT 1`,
		scopeType, scopeID, kind, hash)
	if err != nil {
		return 0, false, err
	}
	mapJSON := "{}"
	if len(mapping) > 0 {
		if raw, mErr := json.Marshal(mapping); mErr == nil {
			mapJSON = string(raw)
		}
	}
	now := s.now().Format(time.RFC3339Nano)
	if len(rows.Values) > 0 {
		id := intValue(rows.Values[0][0])
		status := stringValue(rows.Values[0][1])
		if status == "applied" && !allowDuplicate {
			return 0, false, ErrDuplicateImport
		}
		_, err := s.db.Execute(ctx, `UPDATE import_runs SET relative_path = ?, stored_path = ?, converted_path = ?,
			file_format = ?, row_count = ?, inserted_count = ?, updated_count = ?, skipped_count = ?,
			blocking_count = ?, warning_count = ?, status = 'applied', mapping_json = ?, created_at = ?
			WHERE id = ?`,
			report.Path, report.StoredPath, report.ConvertedPath, report.Format, report.RowCount,
			preview.New, preview.Update, preview.Skip, preview.Blocking, preview.Warnings, mapJSON, now, id)
		if err != nil {
			return 0, false, err
		}
		return id, true, nil
	}

	res, err := s.db.Execute(ctx, `INSERT INTO import_runs(
		scope_type, scope_id, kind, file_hash, relative_path, stored_path, converted_path,
		file_format, row_count, inserted_count, updated_count, skipped_count, blocking_count, warning_count,
		status, mapping_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'applied', ?, ?)`,
		scopeType, scopeID, kind, hash, report.Path, report.StoredPath, report.ConvertedPath,
		report.Format, report.RowCount, preview.New, preview.Update, preview.Skip, preview.Blocking, preview.Warnings,
		mapJSON, now)
	if err != nil {
		return 0, false, err
	}
	return res.LastInsertID, false, nil
}

func (s *Store) deleteImportRun(ctx context.Context, runID int64) error {
	_, err := s.db.Execute(ctx, `DELETE FROM import_runs WHERE id = ?`, runID)
	return err
}

func (s *Store) importChangeStatements(runID int64, changes []importChange) []sqlite.Statement {
	statements := []sqlite.Statement{
		{SQL: `DELETE FROM import_changes WHERE import_run_id = ?`, Args: []any{runID}},
	}
	for _, ch := range changes {
		beforeJSON := mustJSONString(ch.Before)
		afterJSON := ""
		if ch.After != nil {
			afterJSON = mustJSONString(ch.After)
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_changes(import_run_id, table_name, row_id, action, before_json, after_json)
				VALUES (?, ?, ?, ?, ?, ?)`,
			Args: []any{runID, ch.TableName, ch.RowID, ch.Action, beforeJSON, afterJSON},
		})
	}
	return statements
}

func (s *Store) importIssueStatements(runID int64, issues []ImportIssue) []sqlite.Statement {
	statements := []sqlite.Statement{
		{SQL: `DELETE FROM import_issues WHERE import_run_id = ?`, Args: []any{runID}},
	}
	for _, iss := range issues {
		argsJSON := "[]"
		if len(iss.Args) > 0 {
			if raw, err := json.Marshal(iss.Args); err == nil {
				argsJSON = string(raw)
			}
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_issues(import_run_id, severity, row_num, col_name, code, args_json)
				VALUES (?, ?, ?, ?, ?, ?)`,
			Args: []any{runID, iss.Severity, iss.Row, iss.Column, iss.Code, argsJSON},
		})
	}
	return statements
}

func (s *Store) importSheetStatements(runID int64, report ImportReport) []sqlite.Statement {
	statements := []sqlite.Statement{
		{SQL: `DELETE FROM import_sheets WHERE import_run_id = ?`, Args: []any{runID}},
	}
	for _, sheet := range report.Sheets {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_sheets(import_run_id, sheet_name, role, header_row, data_range, is_current)
				VALUES (?, ?, ?, ?, ?, 1)
				ON CONFLICT(import_run_id, sheet_name) DO UPDATE SET role = excluded.role, header_row = excluded.header_row, data_range = excluded.data_range`,
			Args: []any{runID, sheet.Name, sheet.Role, sheet.HeaderRow, fmt.Sprintf("%d rows", sheet.Rows)},
		})
	}
	return statements
}

// CanUndoImport 判断导入记录是否支持安全撤销。
func (s *Store) CanUndoImport(ctx context.Context, runID int64) (bool, string, error) {
	detail, err := s.GetImportRun(ctx, runID)
	if err != nil {
		return false, "", err
	}
	if detail.Run.Status != "applied" && detail.Run.Status != "imported" {
		return false, "not_imported", nil
	}
	cnt, err := s.countQuery(ctx, `SELECT COUNT(*) FROM import_changes WHERE import_run_id = ?`, runID)
	if err != nil {
		return false, "", err
	}
	if cnt == 0 {
		return false, "no_changes", nil
	}
	newer, err := s.countQuery(ctx, `SELECT COUNT(*) FROM import_runs
		WHERE scope_type = ? AND scope_id = ? AND id > ? AND status = 'applied'`,
		detail.Run.ScopeType, detail.Run.ScopeID, runID)
	if err != nil {
		return false, "", err
	}
	if newer > 0 {
		return false, "newer_import", nil
	}
	return true, "", nil
}

// UndoImportRun 按逆序还原某次导入的所有数据库变更，并标记该次导入为 undone。
func (s *Store) UndoImportRun(ctx context.Context, runID int64) error {
	ok, reason, err := s.CanUndoImport(ctx, runID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w (%s)", ErrImportUndoBlocked, reason)
	}
	cRows, err := s.db.Query(ctx, `SELECT table_name, row_id, action, before_json
		FROM import_changes WHERE import_run_id = ? ORDER BY id DESC`, runID)
	if err != nil {
		return err
	}

	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	for _, r := range cRows.Values {
		tableName := stringValue(r[0])
		rowID := intValue(r[1])
		action := stringValue(r[2])
		var before map[string]any
		if raw := strings.TrimSpace(stringValue(r[3])); raw != "" {
			if err := json.Unmarshal([]byte(raw), &before); err != nil {
				return fmt.Errorf("解析撤销快照失败: %w", err)
			}
		}

		switch action {
		case "insert":
			statements = append(statements, s.undoInsertStatements(tableName, rowID, before, runID, now)...)
		case "update":
			if stmt, has := s.undoUpdateStatement(tableName, rowID, before); has {
				statements = append(statements, stmt)
			}
		}
	}

	statements = append(statements,
		sqlite.Statement{SQL: `UPDATE import_runs SET status = 'undone' WHERE id = ?`, Args: []any{runID}},
		auditLogStatement("undo_import", "import_run", runID, map[string]any{"changes": len(cRows.Values)}, now),
	)
	_, err = s.db.Transaction(ctx, statements)
	return err
}

func (s *Store) undoInsertStatements(tableName string, rowID int64, before map[string]any, runID int64, now string) []sqlite.Statement {
	switch tableName {
	case "classes":
		if rowID > 0 {
			return []sqlite.Statement{{SQL: `DELETE FROM classes WHERE id = ?`, Args: []any{rowID}}}
		}
		cohortID := intValue(before["cohort_id"])
		phase, _ := before["phase"].(string)
		name, _ := before["name"].(string)
		if cohortID > 0 && phase != "" && name != "" {
			return []sqlite.Statement{{
				SQL: `DELETE FROM classes WHERE import_run_id = ? AND cohort_id = ? AND phase = ? AND name = ?
					AND NOT EXISTS (SELECT 1 FROM students s WHERE s.current_class_id = classes.id)
					AND NOT EXISTS (SELECT 1 FROM exam_participants ep WHERE ep.class_id = classes.id)
					AND NOT EXISTS (SELECT 1 FROM teaching_groups tg WHERE tg.admin_class_id = classes.id)
					AND NOT EXISTS (SELECT 1 FROM assessment_stat_groups sg WHERE sg.lead_class_id = classes.id)
					AND NOT EXISTS (SELECT 1 FROM result_rows rr WHERE rr.lead_class_id = classes.id)`,
				Args: []any{runID, cohortID, phase, name},
			}}
		}
		return nil
	case "students":
		if rowID > 0 {
			return []sqlite.Statement{{SQL: `DELETE FROM students WHERE id = ?`, Args: []any{rowID}}}
		}
		return nil
	case "teachers":
		if rowID > 0 {
			return []sqlite.Statement{{SQL: `DELETE FROM teachers WHERE id = ?`, Args: []any{rowID}}}
		}
		name, _ := before["name"].(string)
		subj, _ := before["primary_subject"].(string)
		if name != "" {
			return []sqlite.Statement{{
				SQL: `DELETE FROM teachers WHERE import_run_id = ? AND teacher_no = '' AND name = ? AND primary_subject = ?
					AND NOT EXISTS (SELECT 1 FROM teaching_assignments ta WHERE ta.teacher_id = teachers.id)
					AND NOT EXISTS (SELECT 1 FROM class_head_teachers cht WHERE cht.teacher_id = teachers.id)
					AND NOT EXISTS (SELECT 1 FROM result_row_teachers rrt WHERE rrt.teacher_id = teachers.id)
					AND NOT EXISTS (SELECT 1 FROM personal_scores ps WHERE ps.teacher_id = teachers.id)`,
				Args: []any{runID, name, subj},
			}}
		}
		return nil
	case "teaching_groups":
		if rowID > 0 {
			return []sqlite.Statement{{SQL: `DELETE FROM teaching_groups WHERE id = ?`, Args: []any{rowID}}}
		}
		return nil
	case "teaching_assignments":
		if rowID > 0 {
			return []sqlite.Statement{{SQL: `DELETE FROM teaching_assignments WHERE id = ?`, Args: []any{rowID}}}
		}
		return nil
	case "cohort_baseline_overrides":
		if rowID > 0 {
			return []sqlite.Statement{{SQL: `DELETE FROM cohort_baseline_overrides WHERE id = ?`, Args: []any{rowID}}}
		}
		return []sqlite.Statement{{SQL: `DELETE FROM cohort_baseline_overrides WHERE import_run_id = ?`, Args: []any{runID}}}
	case "student_entry_scores":
		return []sqlite.Statement{{SQL: `DELETE FROM student_entry_scores WHERE import_run_id = ?`, Args: []any{runID}}}
	case "exam_participants":
		ecID := intValue(before["exam_cohort_id"])
		sid := intValue(before["student_id"])
		if ecID > 0 && sid > 0 {
			return []sqlite.Statement{{SQL: `DELETE FROM exam_participants WHERE exam_cohort_id = ? AND student_id = ?`, Args: []any{ecID, sid}}}
		}
		return nil
	case "exam_scores":
		epID := intValue(before["exam_participant_id"])
		subj, _ := before["subject"].(string)
		if epID > 0 && subj != "" {
			return []sqlite.Statement{{SQL: `DELETE FROM exam_scores WHERE exam_participant_id = ? AND subject = ?`, Args: []any{epID, subj}}}
		}
		return nil
	case "exam_cutoffs":
		ecID := intValue(before["exam_cohort_id"])
		kind, _ := before["kind"].(string)
		key, _ := before["key"].(string)
		track, _ := before["track"].(string)
		if ecID > 0 {
			return []sqlite.Statement{{
				SQL:  `DELETE FROM exam_cutoffs WHERE exam_cohort_id = ? AND kind = ? AND "key" = ? AND track = ?`,
				Args: []any{ecID, kind, key, track},
			}}
		}
		return nil
	case "assessment_class_targets":
		aid := intValue(before["assessment_id"])
		cid := intValue(before["class_id"])
		tier, _ := before["tier_key"].(string)
		if aid > 0 {
			return []sqlite.Statement{{
				SQL:  `DELETE FROM assessment_class_targets WHERE assessment_id = ? AND class_id = ? AND tier_key = ?`,
				Args: []any{aid, cid, tier},
			}}
		}
		return nil
	case "archived_assessments":
		aid := intValue(before["assessment_id"])
		if aid <= 0 {
			return nil
		}
		var stmts []sqlite.Statement
		summaryMarker, _ := before["input_summary_json"].(string)
		if summaryMarker != "" {
			stmts = append(stmts, sqlite.Statement{
				SQL:  `DELETE FROM result_runs WHERE assessment_id = ? AND input_summary_json = ?`,
				Args: []any{aid, summaryMarker},
			})
		}
		if prevOfficial := intValue(before["previous_official_run_id"]); prevOfficial > 0 {
			stmts = append(stmts, sqlite.Statement{
				SQL:  `UPDATE result_runs SET is_official = 1 WHERE id = ? AND assessment_id = ?`,
				Args: []any{prevOfficial, aid},
			})
		}
		hadArchive, _ := before["had_archive"].(bool)
		if hadArchive {
			prevImportRunID := intValue(before["prev_archive_import_run_id"])
			prevCreatedAt, _ := before["prev_archive_created_at"].(string)
			stmts = append(stmts, sqlite.Statement{
				SQL:  `UPDATE archived_assessments SET import_run_id = ?, created_at = ? WHERE assessment_id = ?`,
				Args: []any{prevImportRunID, prevCreatedAt, aid},
			})
		} else {
			stmts = append(stmts, sqlite.Statement{
				SQL:  `DELETE FROM archived_assessments WHERE assessment_id = ?`,
				Args: []any{aid},
			})
		}
		prevStatus, _ := before["prev_assessment_status"].(string)
		prevCurrentRunID := intValue(before["prev_current_run_id"])
		if prevStatus != "" {
			stmts = append(stmts, sqlite.Statement{
				SQL:  `UPDATE assessments SET status = ?, current_run_id = ?, revision = revision + 1 WHERE id = ?`,
				Args: []any{prevStatus, nullableID(prevCurrentRunID), aid},
			})
		}
		examID := intValue(before["exam_id"])
		prevExamStatus, _ := before["prev_exam_status"].(string)
		if examID > 0 && prevExamStatus != "" && prevExamStatus != "archived" {
			stmts = append(stmts, sqlite.Statement{
				SQL:  `UPDATE exams SET status = ? WHERE id = ? AND status <> 'archived'`,
				Args: []any{prevExamStatus, examID},
			})
		}
		stmts = append(stmts, bumpRevisionStatement(fmt.Sprintf("assessment:%d", aid), now))
		return stmts
	default:
		return nil
	}
}

func (s *Store) undoUpdateStatement(tableName string, rowID int64, before map[string]any) (sqlite.Statement, bool) {
	switch tableName {
	case "students":
		return sqlite.Statement{
			SQL:  `UPDATE students SET current_class_id = ?, current_track = ?, elective_combo = ?, status = ? WHERE id = ?`,
			Args: []any{nullableID(intValue(before["class_id"])), before["track"], before["elective"], before["status"], rowID},
		}, rowID > 0
	case "exam_cutoffs":
		ecID := intValue(before["exam_cohort_id"])
		kind, _ := before["kind"].(string)
		key, _ := before["key"].(string)
		track, _ := before["track"].(string)
		return sqlite.Statement{
			SQL:  `UPDATE exam_cutoffs SET cutoff = ? WHERE exam_cohort_id = ? AND kind = ? AND "key" = ? AND track = ?`,
			Args: []any{before["cutoff"], ecID, kind, key, track},
		}, ecID > 0
	case "cohort_baseline_overrides":
		return sqlite.Statement{
			SQL:  `UPDATE cohort_baseline_overrides SET average = ?, ratio = ? WHERE id = ?`,
			Args: []any{before["average"], before["ratio"], rowID},
		}, rowID > 0
	default:
		return sqlite.Statement{}, false
	}
}

// GetImportRun 读取一条导入记录及其校验问题与工作表详情。
func (s *Store) GetImportRun(ctx context.Context, runID int64) (ImportRunDetail, error) {
	rows, err := s.db.Query(ctx, `SELECT id, scope_type, scope_id, kind, file_hash, stored_path,
		relative_path, status, row_count, inserted_count, updated_count, skipped_count, blocking_count, warning_count, created_at
		FROM import_runs WHERE id = ?`, runID)
	if err != nil {
		return ImportRunDetail{}, err
	}
	if len(rows.Values) == 0 {
		return ImportRunDetail{}, ErrNotFound
	}
	run, err := importRunFromRow(rows.Values[0])
	if err != nil {
		return ImportRunDetail{}, err
	}
	run.Sheets, err = s.importRunSheets(ctx, run.ID)
	if err != nil {
		return ImportRunDetail{}, err
	}
	issues, err := s.ListImportIssues(ctx, run.ID)
	if err != nil {
		return ImportRunDetail{}, err
	}
	return ImportRunDetail{Run: run, Issues: issues}, nil
}

// ListImportIssues 读取某次导入的问题清单。
func (s *Store) ListImportIssues(ctx context.Context, runID int64) ([]ImportIssue, error) {
	rows, err := s.db.Query(ctx, `SELECT severity, row_num, col_name, code, args_json
		FROM import_issues WHERE import_run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	out := make([]ImportIssue, 0, len(rows.Values))
	for _, r := range rows.Values {
		var args []string
		_ = json.Unmarshal([]byte(stringValue(r[4])), &args)
		out = append(out, ImportIssue{
			Severity: stringValue(r[0]),
			Row:      int(intValue(r[1])),
			Column:   stringValue(r[2]),
			Code:     stringValue(r[3]),
			Args:     args,
		})
	}
	return out, nil
}

// ListImportRuns 返回某个阶段的导入记录。
func (s *Store) ListImportRuns(ctx context.Context, stageID int64) ([]ImportRun, error) {
	return s.queryImportRuns(ctx, `scope_type = 'cohort' AND scope_id = ?`, stageID)
}

// ListExamImportRuns 返回一场考试下某个级的导入记录。
func (s *Store) ListExamImportRuns(ctx context.Context, examCohortID int64) ([]ImportRun, error) {
	return s.queryImportRuns(ctx, `scope_type = 'exam' AND scope_id = ?`, examCohortID)
}

// ListAllImportRuns 返回全部导入记录，最新在前。
func (s *Store) ListAllImportRuns(ctx context.Context) ([]ImportRun, error) {
	return s.queryImportRuns(ctx, ``)
}

func (s *Store) queryImportRuns(ctx context.Context, where string, args ...any) ([]ImportRun, error) {
	whereSQL := ""
	if where != "" {
		whereSQL = " WHERE " + where
	}
	rows, err := s.db.Query(ctx, `SELECT id, scope_type, scope_id, kind, file_hash, stored_path,
		relative_path, status, row_count, inserted_count, updated_count, skipped_count, blocking_count, warning_count, created_at
		FROM import_runs`+whereSQL+` ORDER BY id DESC`, args...)
	if err != nil {
		return nil, err
	}
	runs := make([]ImportRun, 0, len(rows.Values))
	for _, r := range rows.Values {
		run, err := importRunFromRow(r)
		if err != nil {
			return nil, err
		}
		sheets, err := s.importRunSheets(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		run.Sheets = sheets
		runs = append(runs, run)
	}
	return runs, nil
}

func importRunFromRow(r []any) (ImportRun, error) {
	createdAt, err := parseTime(r[14])
	if err != nil {
		return ImportRun{}, err
	}
	return ImportRun{
		ID:           intValue(r[0]),
		ScopeType:    stringValue(r[1]),
		ScopeID:      intValue(r[2]),
		DataKind:     stringValue(r[3]),
		FileHash:     stringValue(r[4]),
		StoredPath:   stringValue(r[5]),
		RelativePath: stringValue(r[6]),
		Status:       stringValue(r[7]),
		RowCount:     int(intValue(r[8])),
		Inserted:     int(intValue(r[9])),
		Updated:      int(intValue(r[10])),
		Skipped:      int(intValue(r[11])),
		Blocking:     int(intValue(r[12])),
		Warnings:     int(intValue(r[13])),
		CreatedAt:    createdAt,
	}, nil
}

func (s *Store) importRunSheets(ctx context.Context, runID int64) ([]ImportSheetInfo, error) {
	rows, err := s.db.Query(ctx, `SELECT sheet_name, role, header_row, data_range, is_current
		FROM import_sheets WHERE import_run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	sheets := make([]ImportSheetInfo, 0, len(rows.Values))
	for _, r := range rows.Values {
		rowCnt := 0
		_, _ = fmt.Sscanf(stringValue(r[3]), "%d", &rowCnt)
		sheets = append(sheets, ImportSheetInfo{
			Name:      stringValue(r[0]),
			Role:      stringValue(r[1]),
			HeaderRow: int(intValue(r[2])),
			Rows:      rowCnt,
			Current:   intValue(r[4]) != 0,
		})
	}
	return sheets, nil
}

func (s *Store) cohortStudentNoSet(ctx context.Context, cohortID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT student_no FROM students WHERE cohort_id = ? AND student_no <> ''`, cohortID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows.Values))
	for _, r := range rows.Values {
		out[stringValue(r[0])] = true
	}
	return out, nil
}

func (s *Store) schoolTeacherMaps(ctx context.Context) (map[string]string, map[string][]int64, map[string][]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT id, teacher_no, name, primary_subject FROM teachers`)
	if err != nil {
		return nil, nil, nil, err
	}
	byNo := map[string]string{}
	byNameSubj := map[string][]int64{}
	byName := map[string][]int64{}
	for _, r := range rows.Values {
		id := intValue(r[0])
		no := stringValue(r[1])
		name := stringValue(r[2])
		subj := stringValue(r[3])
		if no != "" {
			byNo[no] = name
		}
		if name != "" {
			byName[name] = append(byName[name], id)
			if subj != "" {
				byNameSubj[name+"\x00"+subj] = append(byNameSubj[name+"\x00"+subj], id)
			}
		}
	}
	return byNo, byNameSubj, byName, nil
}

func (s *Store) lookupTeacherID(ctx context.Context, teacherNo, name, primarySubject string) int64 {
	if teacherNo != "" {
		rows, err := s.db.Query(ctx, `SELECT id FROM teachers WHERE teacher_no = ? LIMIT 1`, teacherNo)
		if err == nil && len(rows.Values) > 0 {
			return intValue(rows.Values[0][0])
		}
	}
	if name != "" {
		if primarySubject != "" {
			rows, err := s.db.Query(ctx, `SELECT id FROM teachers WHERE name = ? AND primary_subject = ? LIMIT 2`, name, primarySubject)
			if err == nil && len(rows.Values) == 1 {
				return intValue(rows.Values[0][0])
			}
			rows, err = s.db.Query(ctx, `SELECT id FROM teachers WHERE name = ? AND primary_subject = '' LIMIT 2`, name)
			if err == nil && len(rows.Values) == 1 {
				return intValue(rows.Values[0][0])
			}
			return 0
		}
		rows, err := s.db.Query(ctx, `SELECT id FROM teachers WHERE name = ? LIMIT 2`, name)
		if err == nil && len(rows.Values) == 1 {
			return intValue(rows.Values[0][0])
		}
	}
	return 0
}

func formatWarningTargets(targets []TeacherActiveAssignment) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		parts = append(parts, fmt.Sprintf("%s %s", formatCohortLabel(t.EnrollmentYear), t.ClassName))
	}
	return strings.Join(parts, "、")
}

func issueCodeSummary(issues []ImportIssue) string {
	var codes []string
	for _, iss := range issues {
		if iss.Severity == "blocking" {
			codes = append(codes, iss.Code)
		}
	}
	return strings.Join(codes, ",")
}

var _ = errors.Is
