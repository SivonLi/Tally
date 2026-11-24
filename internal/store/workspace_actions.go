package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// ZeroTargetBatchItem 描述批量零目标处理中单个班级的补录或不参与决策。
type ZeroTargetBatchItem struct {
	ClassID int64
	Exclude bool
	Targets map[string]float64
	Reason  string
}

// ApplyZeroTargetBatch 在单个事务中批量写入多个班级的零目标补录或不参与记录。
func (s *Store) ApplyZeroTargetBatch(ctx context.Context, assessmentID int64, items []ZeroTargetBatchItem, defaultReason string) error {
	if len(items) == 0 {
		return nil
	}
	stageID, err := s.assessmentStage(ctx, assessmentID)
	if err != nil {
		return err
	}
	classRows, err := s.db.Query(ctx, `SELECT class_id FROM class_stage_info WHERE stage_id = ?`, stageID)
	if err != nil {
		return err
	}
	validClasses := map[int64]bool{}
	for _, row := range classRows.Values {
		validClasses[intValue(row[0])] = true
	}
	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	for _, item := range items {
		if !validClasses[item.ClassID] {
			return fmt.Errorf("班级不属于本次考核对应的阶段")
		}
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			reason = strings.TrimSpace(defaultReason)
		}
		if reason == "" {
			return fmt.Errorf("原因不能为空")
		}
		if item.Exclude {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO assessment_target_adjustments(assessment_id, scope_type,
					scope_id, subject, kind, reason, value, created_at)
					VALUES (?, 'class', ?, '', 'exclusion', ?, NULL, ?)`,
				Args: []any{assessmentID, item.ClassID, reason, now},
			})
			continue
		}
		tiers := make([]string, 0, len(item.Targets))
		for tier := range item.Targets {
			tiers = append(tiers, tier)
		}
		sort.Strings(tiers)
		for _, tier := range tiers {
			value := item.Targets[tier]
			if strings.TrimSpace(tier) == "" {
				return fmt.Errorf("档位不能为空")
			}
			if value < 0 {
				return fmt.Errorf("目标人数不能小于零")
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO assessment_target_adjustments(assessment_id, scope_type,
					scope_id, subject, kind, reason, value, created_at)
					VALUES (?, 'class', ?, ?, 'adjustment', ?, ?, ?)`,
				Args: []any{assessmentID, item.ClassID, tier, reason, value, now},
			})
		}
	}
	if len(statements) == 0 {
		return nil
	}
	_, err = s.db.Transaction(ctx, statements)
	return err
}

// FindPreviousAssessmentWithTargets 查找同一工作区（级）中最近一次已设置班级目标的考核标识。
func (s *Store) FindPreviousAssessmentWithTargets(ctx context.Context, assessmentID int64) (int64, error) {
	current, err := s.LoadAssessmentContext(ctx, assessmentID)
	if err != nil {
		return 0, err
	}
	rows, err := s.db.Query(ctx, `SELECT a.id FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		WHERE ec.cohort_id = ? AND a.id <> ?
			AND EXISTS (SELECT 1 FROM assessment_class_targets t WHERE t.assessment_id = a.id)
		ORDER BY CASE WHEN e.exam_date <= ? THEN 0 ELSE 1 END,
			e.academic_year_start DESC, e.exam_date DESC, e.id DESC
		LIMIT 1`, current.Cohort.ID, assessmentID, current.Exam.ExamDate)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, ErrNotFound
	}
	return intValue(rows.Values[0][0]), nil
}

// CopyAssessmentTargets 把来源考核的班级分档目标复制到目标考核，仅复制目标考核阶段内存在的行政班。
func (s *Store) CopyAssessmentTargets(ctx context.Context, fromAssessmentID, toAssessmentID int64) (int, error) {
	if fromAssessmentID == toAssessmentID {
		return 0, fmt.Errorf("来源考核与目标考核不能相同")
	}
	toStageID, err := s.assessmentStage(ctx, toAssessmentID)
	if err != nil {
		return 0, err
	}
	classRows, err := s.db.Query(ctx, `SELECT class_id FROM class_stage_info WHERE stage_id = ?`, toStageID)
	if err != nil {
		return 0, err
	}
	validClasses := map[int64]bool{}
	for _, row := range classRows.Values {
		validClasses[intValue(row[0])] = true
	}
	sourceTargets, err := s.ListClassTargets(ctx, fromAssessmentID)
	if err != nil {
		return 0, err
	}
	var statements []sqlite.Statement
	copied := 0
	for _, target := range sourceTargets {
		if !validClasses[target.ClassID] {
			continue
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assessment_class_targets(assessment_id, class_id, tier_key, target_count)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(assessment_id, class_id, tier_key)
				DO UPDATE SET target_count = excluded.target_count`,
			Args: []any{toAssessmentID, target.ClassID, target.TierKey, target.Target},
		})
		copied++
	}
	if len(statements) == 0 {
		return 0, nil
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return 0, err
	}
	return copied, nil
}

// UpdateExamRosterTrack 原地修正报名名单中一名学生的选科类别，并推进考试数据修订号。
func (s *Store) UpdateExamRosterTrack(ctx context.Context, examID, cohortID, studentID int64, track string) error {
	track = strings.TrimSpace(track)
	if track == "" {
		return fmt.Errorf("选科类别不能为空")
	}
	tracks, err := s.trackKeys(ctx)
	if err != nil {
		return err
	}
	if !tracks[track] {
		return fmt.Errorf("未启用的选科类别 %q", track)
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM exam_rosters
		WHERE exam_id = ? AND cohort_id = ? AND student_id = ?`, examID, cohortID, studentID)
	if err != nil {
		return err
	}
	if len(rows.Values) == 0 {
		return ErrNotFound
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.Transaction(ctx, []sqlite.Statement{
		{
			SQL:  `UPDATE exam_rosters SET track = ? WHERE exam_id = ? AND cohort_id = ? AND student_id = ?`,
			Args: []any{track, examID, cohortID, studentID},
		},
		{
			SQL: `INSERT INTO data_revisions(scope_type, scope_id, revision, updated_at)
				VALUES ('exam', ?, 1, ?)
				ON CONFLICT(scope_type, scope_id) DO UPDATE SET revision = revision + 1,
					updated_at = excluded.updated_at`,
			Args: []any{examID, now},
		},
	})
	return err
}

// UpdateExamScore 原地修正一名学生的单科成绩或异常标记，并推进考试数据修订号。
func (s *Store) UpdateExamScore(ctx context.Context, examID, cohortID, studentID int64, subject string, score *float64, flag string) error {
	subject = strings.TrimSpace(subject)
	flag = strings.TrimSpace(flag)
	if subject == "" {
		return fmt.Errorf("学科不能为空")
	}
	rosterRows, err := s.db.Query(ctx, `SELECT id FROM exam_rosters
		WHERE exam_id = ? AND cohort_id = ? AND student_id = ?`, examID, cohortID, studentID)
	if err != nil {
		return err
	}
	if len(rosterRows.Values) == 0 {
		return ErrNotFound
	}
	subjRows, err := s.db.Query(ctx, `SELECT full_mark FROM exam_subjects
		WHERE exam_id = ? AND cohort_id = ? AND subject = ? AND enabled = 1`,
		examID, cohortID, subject)
	if err != nil {
		return err
	}
	if len(subjRows.Values) == 0 {
		return fmt.Errorf("本次考试未启用学科 %q", subject)
	}
	fullMark := floatValue(subjRows.Values[0][0])
	if score != nil {
		if *score < 0 || (fullMark > 0 && *score > fullMark) {
			return fmt.Errorf("成绩 %.2f 超出范围 0..%.2f", *score, fullMark)
		}
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.Transaction(ctx, []sqlite.Statement{
		{
			SQL: `INSERT INTO exam_scores(exam_id, cohort_id, student_id, subject, score, flag, source_sheet, source_row)
				VALUES (?, ?, ?, ?, ?, ?, '', 0)
				ON CONFLICT(exam_id, cohort_id, student_id, subject) DO UPDATE SET
					score = excluded.score, flag = excluded.flag`,
			Args: []any{examID, cohortID, studentID, subject, optionalFloat(score), flag},
		},
		{
			SQL: `INSERT INTO data_revisions(scope_type, scope_id, revision, updated_at)
				VALUES ('exam', ?, 1, ?)
				ON CONFLICT(scope_type, scope_id) DO UPDATE SET revision = revision + 1,
					updated_at = excluded.updated_at`,
			Args: []any{examID, now},
		},
	})
	return err
}

// SofficePath 读取设置中保存的 LibreOffice soffice 路径，未设置时返回空字符串。
func (s *Store) SofficePath(ctx context.Context) (string, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE key = 'soffice_path'`)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 {
		return "", nil
	}
	return strings.TrimSpace(stringValue(rows.Values[0][0])), nil
}

// SetSofficePath 保存自定义 LibreOffice soffice 路径；传入空字符串时恢复自动检测。
func (s *Store) SetSofficePath(ctx context.Context, path string) error {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		_, err := s.db.Execute(ctx, `DELETE FROM app_settings WHERE key = 'soffice_path'`)
		return err
	}
	_, err := s.db.Execute(ctx, `INSERT INTO app_settings(key, value) VALUES ('soffice_path', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, cleaned)
	return err
}
