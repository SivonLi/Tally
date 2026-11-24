package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// ErrImportUndoBlocked 表示导入之后还有新的导入，撤销会覆盖后续修改。
var ErrImportUndoBlocked = errors.New("此导入之后还有新的导入，不能撤销")

// CanUndoImport 判断一条导入记录是否可以撤销，返回不能撤销的原因代码。
func (s *Store) CanUndoImport(ctx context.Context, runID int64) (bool, string, error) {
	detail, err := s.GetImportRun(ctx, runID)
	if err != nil {
		return false, "", err
	}
	if detail.Run.Status != "imported" {
		return false, "not_imported", nil
	}
	changes, err := s.db.Query(ctx, `SELECT 1 FROM import_changes WHERE import_run_id = ? LIMIT 1`, runID)
	if err != nil {
		return false, "", err
	}
	if len(changes.Values) == 0 {
		return false, "no_changes", nil
	}
	if detail.Run.ScopeType != "stage" && detail.Run.ScopeType != "exam" {
		return false, "unsupported_scope", nil
	}
	rows, err := s.db.Query(ctx, `SELECT 1 FROM import_runs
		WHERE scope_type = ? AND scope_id = ? AND id > ? AND status = 'imported' LIMIT 1`,
		detail.Run.ScopeType, detail.Run.ScopeID, runID)
	if err != nil {
		return false, "", err
	}
	if len(rows.Values) > 0 {
		return false, "newer_import", nil
	}
	return true, "", nil
}

// GetImportRun 读取一条导入记录及其问题。
func (s *Store) GetImportRun(ctx context.Context, runID int64) (ImportRunDetail, error) {
	rows, err := s.db.Query(ctx, `SELECT id, scope_type, scope_id, data_kind, file_hash, stored_path,
		relative_path, status, row_count, warning_count, created_at
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
	sheetRows, err := s.db.Query(ctx, `SELECT sheet_name, role, header_row, data_range, is_current
		FROM import_sheets WHERE import_run_id = ? ORDER BY id`, runID)
	if err != nil {
		return ImportRunDetail{}, err
	}
	for _, row := range sheetRows.Values {
		name, _ := row[0].(string)
		role, _ := row[1].(string)
		header, _ := row[2].(int64)
		rangeText, _ := row[3].(string)
		current, _ := row[4].(int64)
		rowCount := 0
		fmt.Sscanf(rangeText, "%d", &rowCount)
		run.Sheets = append(run.Sheets, ImportSheetInfo{
			Name: name, Role: role, HeaderRow: int(header), Rows: rowCount, Current: current != 0,
		})
	}
	issues, err := s.ListImportIssues(ctx, runID)
	if err != nil {
		return ImportRunDetail{}, err
	}
	return ImportRunDetail{Run: run, Issues: issues}, nil
}

// ListImportIssues 读取一条导入记录的问题清单。
func (s *Store) ListImportIssues(ctx context.Context, runID int64) ([]ImportIssue, error) {
	rows, err := s.db.Query(ctx, `SELECT severity, row_number, column_name, code, message
		FROM import_issues WHERE import_run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	issues := make([]ImportIssue, 0, len(rows.Values))
	for _, row := range rows.Values {
		issue := ImportIssue{}
		issue.Severity, _ = row[0].(string)
		issue.Row = int(intValue(row[1]))
		issue.Column, _ = row[2].(string)
		issue.Code, _ = row[3].(string)
		args, _ := row[4].(string)
		if args != "" {
			_ = json.Unmarshal([]byte(args), &issue.Args)
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

// UndoImportRun 反向应用一条导入记录的变更（支持基础数据与考试数据导入）。
func (s *Store) UndoImportRun(ctx context.Context, runID int64) error {
	detail, err := s.GetImportRun(ctx, runID)
	if err != nil {
		return err
	}
	run := detail.Run
	if run.ScopeType != "stage" && run.ScopeType != "exam" {
		return fmt.Errorf("不支持撤销作用域 %q 的导入", run.ScopeType)
	}
	if run.Status != "imported" {
		return fmt.Errorf("%w: 导入状态为 %s", ErrImportUndoBlocked, run.Status)
	}
	changeRows, err := s.db.Query(ctx, `SELECT 1 FROM import_changes WHERE import_run_id = ? LIMIT 1`, runID)
	if err != nil {
		return err
	}
	if len(changeRows.Values) == 0 {
		return fmt.Errorf("%w: 没有变更记录", ErrImportUndoBlocked)
	}
	rows, err := s.db.Query(ctx, `SELECT 1 FROM import_runs
		WHERE scope_type = ? AND scope_id = ? AND id > ? AND status = 'imported' LIMIT 1`,
		run.ScopeType, run.ScopeID, runID)
	if err != nil {
		return err
	}
	if len(rows.Values) > 0 {
		return ErrImportUndoBlocked
	}
	changes, err := s.importChanges(ctx, runID)
	if err != nil {
		return err
	}
	revScopeID := run.ScopeID
	if run.ScopeType == "exam" {
		examRows, err := s.db.Query(ctx, `SELECT exam_id FROM exam_cohorts WHERE id = ?`, run.ScopeID)
		if err != nil {
			return err
		}
		if len(examRows.Values) > 0 {
			revScopeID = intValue(examRows.Values[0][0])
		}
	}
	statements := make([]sqlite.Statement, 0, len(changes)+3)
	for index := len(changes) - 1; index >= 0; index-- {
		statement, err := importUndoStatement(changes[index])
		if err != nil {
			return err
		}
		statements = append(statements, statement)
	}
	now := s.now().Format(time.RFC3339Nano)
	statements = append(statements,
		sqlite.Statement{SQL: `UPDATE import_runs SET status = 'undone' WHERE id = ?`, Args: []any{runID}},
		sqlite.Statement{
			SQL: `INSERT INTO data_revisions(scope_type, scope_id, revision, updated_at)
				VALUES (?, ?, 1, ?)
				ON CONFLICT(scope_type, scope_id) DO UPDATE SET revision = revision + 1,
					updated_at = excluded.updated_at`,
			Args: []any{run.ScopeType, revScopeID, now},
		},
		sqlite.Statement{
			SQL: `INSERT INTO audit_log(action, entity, entity_id, detail_json, created_at, operator)
				VALUES ('undo_import', ?, ?, ?, ?, 'local')`,
			Args: []any{run.ScopeType, run.ScopeID, fmt.Sprintf(`{"run":%d}`, runID), now},
		},
	)
	_, err = s.db.Transaction(ctx, statements)
	return err
}

func (s *Store) importChanges(ctx context.Context, runID int64) ([]importChangeRecord, error) {
	rows, err := s.db.Query(ctx, `SELECT entity, action, before_json FROM import_changes
		WHERE import_run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	changes := make([]importChangeRecord, 0, len(rows.Values))
	for _, row := range rows.Values {
		change := importChangeRecord{}
		change.Entity, _ = row[0].(string)
		change.Action, _ = row[1].(string)
		encoded, _ := row[2].(string)
		change.Before = map[string]any{}
		if encoded != "" {
			if err := json.Unmarshal([]byte(encoded), &change.Before); err != nil {
				return nil, err
			}
		}
		changes = append(changes, change)
	}
	return changes, nil
}

type importChangeRecord struct {
	Entity string
	Action string
	Before map[string]any
}

func importUndoStatement(change importChangeRecord) (sqlite.Statement, error) {
	if change.Action == "update" {
		return importRestoreStatement(change)
	}
	return importDeleteStatement(change)
}

func importDeleteStatement(change importChangeRecord) (sqlite.Statement, error) {
	before := change.Before
	switch change.Entity {
	case "class":
		return sqlite.Statement{
			SQL:  `DELETE FROM classes WHERE cohort_id = ? AND name = ?`,
			Args: []any{intValue(before["cohort_id"]), stringValue(before["name"])},
		}, nil
	case "class_stage":
		return sqlite.Statement{
			SQL: `DELETE FROM class_stage_info WHERE stage_id = ? AND class_id = (
				SELECT id FROM classes WHERE cohort_id = ? AND name = ?)`,
			Args: []any{intValue(before["stage_id"]), intValue(before["cohort_id"]),
				stringValue(before["name"])},
		}, nil
	case "student":
		return sqlite.Statement{
			SQL:  `DELETE FROM students WHERE student_no = ?`,
			Args: []any{stringValue(before["student_no"])},
		}, nil
	case "student_cohort":
		return sqlite.Statement{
			SQL: `DELETE FROM student_cohorts WHERE cohort_id = ? AND student_id = (
				SELECT id FROM students WHERE student_no = ?)`,
			Args: []any{intValue(before["cohort_id"]), stringValue(before["student_no"])},
		}, nil
	case "student_membership":
		return sqlite.Statement{
			SQL: `DELETE FROM student_stage_memberships WHERE stage_id = ? AND student_id = (
				SELECT id FROM students WHERE student_no = ?)`,
			Args: []any{intValue(before["stage_id"]), stringValue(before["student_no"])},
		}, nil
	case "teacher":
		if no := stringValue(before["teacher_no"]); no != "" {
			return sqlite.Statement{SQL: `DELETE FROM teachers WHERE teacher_no = ?`, Args: []any{no}}, nil
		}
		return sqlite.Statement{
			SQL:  `DELETE FROM teachers WHERE teacher_no = '' AND name = ? AND subject = ?`,
			Args: []any{stringValue(before["name"]), stringValue(before["subject"])},
		}, nil
	case "teaching_group":
		return sqlite.Statement{
			SQL: `DELETE FROM teaching_groups WHERE stage_id = ? AND subject = ? AND name = ?`,
			Args: []any{intValue(before["stage_id"]), stringValue(before["subject"]),
				stringValue(before["name"])},
		}, nil
	case "assignment":
		if no := stringValue(before["teacher_no"]); no != "" {
			return sqlite.Statement{
				SQL: `DELETE FROM assignments WHERE teaching_group_id = (
					SELECT id FROM teaching_groups WHERE stage_id = ? AND subject = ? AND name = ?)
					AND teacher_id = (SELECT id FROM teachers WHERE teacher_no = ?)`,
				Args: []any{intValue(before["stage_id"]), stringValue(before["subject"]),
					stringValue(before["group_name"]), no},
			}, nil
		}
		return sqlite.Statement{
			SQL: `DELETE FROM assignments WHERE teaching_group_id = (
				SELECT id FROM teaching_groups WHERE stage_id = ? AND subject = ? AND name = ?)
				AND teacher_id = (SELECT id FROM teachers WHERE teacher_no = '' AND name = ? AND subject = ?)`,
			Args: []any{intValue(before["stage_id"]), stringValue(before["subject"]),
				stringValue(before["group_name"]), stringValue(before["teacher_name"]),
				stringValue(before["subject"])},
		}, nil
	case "student_teaching_group":
		return sqlite.Statement{
			SQL: `DELETE FROM student_teaching_groups WHERE stage_id = ? AND subject = ? AND student_id = (
				SELECT id FROM students WHERE student_no = ?)`,
			Args: []any{intValue(before["stage_id"]), stringValue(before["subject"]),
				stringValue(before["student_no"])},
		}, nil
	case "baseline_aggregate":
		return sqlite.Statement{
			SQL: `DELETE FROM baseline_aggregates WHERE stage_id = ? AND scope_type = 'class'
				AND subject = ? AND scope_id = (
					SELECT id FROM classes WHERE cohort_id = ? AND name = ? LIMIT 1)`,
			Args: []any{intValue(before["stage_id"]), stringValue(before["subject"]),
				intValue(before["cohort_id"]), stringValue(before["class_name"])},
		}, nil
	case "baseline_stage_aggregate":
		return sqlite.Statement{
			SQL: `DELETE FROM baseline_aggregates WHERE stage_id = ? AND scope_type = 'stage'
				AND subject = ?`,
			Args: []any{intValue(before["stage_id"]), stringValue(before["subject"])},
		}, nil
	case "exam_roster":
		return sqlite.Statement{
			SQL: `DELETE FROM exam_rosters WHERE exam_id = ? AND cohort_id = ? AND student_id = ?`,
			Args: []any{intValue(before["exam_id"]), intValue(before["cohort_id"]),
				intValue(before["student_id"])},
		}, nil
	case "exam_score":
		return sqlite.Statement{
			SQL: `DELETE FROM exam_scores WHERE exam_id = ? AND cohort_id = ? AND student_id = ? AND subject = ?`,
			Args: []any{intValue(before["exam_id"]), intValue(before["cohort_id"]),
				intValue(before["student_id"]), stringValue(before["subject"])},
		}, nil
	case "exam_subject_cutoff":
		return sqlite.Statement{
			SQL: `DELETE FROM exam_subject_cutoffs WHERE exam_id = ? AND cohort_id = ? AND subject = ? AND track = ?`,
			Args: []any{intValue(before["exam_id"]), intValue(before["cohort_id"]),
				stringValue(before["subject"]), stringValue(before["track"])},
		}, nil
	case "exam_tier_cutoff":
		return sqlite.Statement{
			SQL: `DELETE FROM exam_tier_cutoffs WHERE exam_id = ? AND cohort_id = ? AND tier_key = ? AND track = ?`,
			Args: []any{intValue(before["exam_id"]), intValue(before["cohort_id"]),
				stringValue(before["tier_key"]), stringValue(before["track"])},
		}, nil
	case "exam_class_target":
		return sqlite.Statement{
			SQL: `DELETE FROM assessment_class_targets WHERE assessment_id = ? AND class_id = ? AND tier_key = ?`,
			Args: []any{intValue(before["assessment_id"]), intValue(before["class_id"]),
				stringValue(before["tier_key"])},
		}, nil
	default:
		return sqlite.Statement{}, fmt.Errorf("未知的导入变更实体 %q", change.Entity)
	}
}

func importRestoreStatement(change importChangeRecord) (sqlite.Statement, error) {
	before := change.Before
	switch change.Entity {
	case "student":
		return sqlite.Statement{
			SQL: `UPDATE students SET name = ?, import_run_id = ?, source_sheet = ?, source_row = ?
				WHERE student_no = ?`,
			Args: []any{stringValue(before["name"]), intValue(before["import_run_id"]),
				stringValue(before["source_sheet"]), intValue(before["source_row"]),
				stringValue(before["student_no"])},
		}, nil
	case "student_membership":
		return sqlite.Statement{
			SQL: `UPDATE student_stage_memberships SET class_id = ?, status = ?, walking = ?,
				track = ?, elective = ?, japanese = ?, total_score = ?, import_run_id = ?,
				source_sheet = ?, source_row = ?
				WHERE stage_id = ? AND student_id = (SELECT id FROM students WHERE student_no = ?)`,
			Args: []any{intValue(before["class_id"]), stringValue(before["status"]),
				intValue(before["walking"]), stringValue(before["track"]),
				stringValue(before["elective"]), intValue(before["japanese"]),
				before["total_score"], intValue(before["import_run_id"]),
				stringValue(before["source_sheet"]), intValue(before["source_row"]),
				intValue(before["stage_id"]), stringValue(before["student_no"])},
		}, nil
	case "student_teaching_group":
		return sqlite.Statement{
			SQL: `UPDATE student_teaching_groups SET teaching_group_id = ?, import_run_id = 0
				WHERE stage_id = ? AND subject = ? AND student_id = (
					SELECT id FROM students WHERE student_no = ?)`,
			Args: []any{intValue(before["teaching_group_id"]), intValue(before["stage_id"]),
				stringValue(before["subject"]), stringValue(before["student_no"])},
		}, nil
	case "baseline_aggregate":
		return sqlite.Statement{
			SQL: `UPDATE baseline_aggregates SET average = ?, count = ?, ratio = ?,
				import_run_id = ?, source_sheet = ?, source_row = ?
				WHERE stage_id = ? AND scope_type = 'class' AND subject = ? AND scope_id = (
					SELECT id FROM classes WHERE cohort_id = ? AND name = ? LIMIT 1)`,
			Args: []any{floatValue(before["average"]), intValue(before["count"]),
				floatValue(before["ratio"]), intValue(before["import_run_id"]),
				stringValue(before["source_sheet"]), intValue(before["source_row"]),
				intValue(before["stage_id"]), stringValue(before["subject"]),
				intValue(before["cohort_id"]),
				stringValue(before["class_name"])},
		}, nil
	case "baseline_stage_aggregate":
		return sqlite.Statement{
			SQL: `UPDATE baseline_aggregates SET average = ?, count = ?, import_run_id = ?,
				source_sheet = ?, source_row = ?
				WHERE stage_id = ? AND scope_type = 'stage' AND subject = ?`,
			Args: []any{floatValue(before["average"]), intValue(before["count"]),
				intValue(before["import_run_id"]), stringValue(before["source_sheet"]),
				intValue(before["source_row"]), intValue(before["stage_id"]),
				stringValue(before["subject"])},
		}, nil
	case "exam_roster":
		return sqlite.Statement{
			SQL: `UPDATE exam_rosters SET class_id = ?, exam_no = ?, candidate_no = ?, track = ?,
				total_score = ?, status = ?, subjects_json = ?, import_run_id = ?,
				source_sheet = ?, source_row = ?
				WHERE exam_id = ? AND cohort_id = ? AND student_id = ?`,
			Args: []any{intValue(before["class_id"]), stringValue(before["exam_no"]),
				stringValue(before["candidate_no"]), stringValue(before["track"]),
				before["total_score"], stringValue(before["status"]),
				stringValue(before["subjects_json"]), intValue(before["import_run_id"]),
				stringValue(before["source_sheet"]), intValue(before["source_row"]),
				intValue(before["exam_id"]), intValue(before["cohort_id"]),
				intValue(before["student_id"])},
		}, nil
	case "exam_score":
		return sqlite.Statement{
			SQL: `UPDATE exam_scores SET score = ?, flag = ?, import_run_id = ?,
				source_sheet = ?, source_row = ?
				WHERE exam_id = ? AND cohort_id = ? AND student_id = ? AND subject = ?`,
			Args: []any{before["score"], stringValue(before["flag"]),
				intValue(before["import_run_id"]), stringValue(before["source_sheet"]),
				intValue(before["source_row"]), intValue(before["exam_id"]),
				intValue(before["cohort_id"]), intValue(before["student_id"]),
				stringValue(before["subject"])},
		}, nil
	case "exam_subject_cutoff":
		return sqlite.Statement{
			SQL: `UPDATE exam_subject_cutoffs SET cutoff = ?
				WHERE exam_id = ? AND cohort_id = ? AND subject = ? AND track = ?`,
			Args: []any{floatValue(before["cutoff"]), intValue(before["exam_id"]),
				intValue(before["cohort_id"]), stringValue(before["subject"]),
				stringValue(before["track"])},
		}, nil
	case "exam_tier_cutoff":
		return sqlite.Statement{
			SQL: `UPDATE exam_tier_cutoffs SET cutoff = ?
				WHERE exam_id = ? AND cohort_id = ? AND tier_key = ? AND track = ?`,
			Args: []any{floatValue(before["cutoff"]), intValue(before["exam_id"]),
				intValue(before["cohort_id"]), stringValue(before["tier_key"]),
				stringValue(before["track"])},
		}, nil
	case "exam_class_target":
		return sqlite.Statement{
			SQL: `UPDATE assessment_class_targets SET target_count = ?
				WHERE assessment_id = ? AND class_id = ? AND tier_key = ?`,
			Args: []any{floatValue(before["target_count"]), intValue(before["assessment_id"]),
				intValue(before["class_id"]), stringValue(before["tier_key"])},
		}, nil
	default:
		return sqlite.Statement{}, fmt.Errorf("未知的导入变更实体 %q", change.Entity)
	}
}

func importRunFromRow(row []any) (ImportRun, error) {
	run := ImportRun{}
	run.ID = intValue(row[0])
	run.ScopeType, _ = row[1].(string)
	run.ScopeID = intValue(row[2])
	run.DataKind, _ = row[3].(string)
	run.FileHash, _ = row[4].(string)
	run.StoredPath, _ = row[5].(string)
	run.RelativePath, _ = row[6].(string)
	run.Status, _ = row[7].(string)
	run.RowCount = int(intValue(row[8]))
	run.Warnings = int(intValue(row[9]))
	created, _ := row[10].(string)
	if parsed, err := time.Parse(time.RFC3339Nano, created); err == nil {
		run.CreatedAt = parsed
	}
	return run, nil
}
