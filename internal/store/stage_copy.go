package store

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// StageCopySummary 是一次阶段复制的结果。
type StageCopySummary struct {
	FromStageID    int64
	Classes        int
	TeachingGroups int
	Assignments    int
	Students       int
}

// CopyStageFromPrevious 把上一个阶段组织关系复制到新阶段，并标记待核对。
func (s *Store) CopyStageFromPrevious(ctx context.Context, cohortID, toStageID int64) (StageCopySummary, error) {
	if _, err := s.stageByID(ctx, cohortID, toStageID); err != nil {
		return StageCopySummary{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM cohort_stages
		WHERE cohort_id = ? AND academic_year_start < (
			SELECT academic_year_start FROM cohort_stages WHERE id = ?
		) ORDER BY academic_year_start DESC LIMIT 1`,
		cohortID, toStageID)
	if err != nil {
		return StageCopySummary{}, err
	}
	if len(rows.Values) == 0 {
		return StageCopySummary{}, nil
	}
	fromStageID, _ := rows.Values[0][0].(int64)
	if fromStageID == 0 || fromStageID == toStageID {
		return StageCopySummary{}, nil
	}
	now := s.now().Format(time.RFC3339Nano)
	statements := []sqlite.Statement{
		{
			SQL: `INSERT INTO class_stage_info(class_id, stage_id, head_teacher_id, room, status)
				SELECT i.class_id, ?, i.head_teacher_id, i.room, i.status FROM class_stage_info i
				WHERE i.stage_id = ?
				ON CONFLICT(class_id, stage_id) DO NOTHING`,
			Args: []any{toStageID, fromStageID},
		},
		{
			SQL: `INSERT INTO teaching_groups(stage_id, subject, name, kind)
				SELECT ?, g.subject, g.name, g.kind FROM teaching_groups g
				WHERE g.stage_id = ?
				ON CONFLICT(stage_id, subject, name) DO NOTHING`,
			Args: []any{toStageID, fromStageID},
		},
		{
			SQL: `INSERT INTO assignments(teacher_id, teaching_group_id, role)
				SELECT a.teacher_id, target.id, a.role FROM assignments a
				JOIN teaching_groups source ON source.id = a.teaching_group_id
				JOIN teaching_groups target ON target.stage_id = ?
					AND target.subject = source.subject AND target.name = source.name
				WHERE source.stage_id = ?
				ON CONFLICT(teacher_id, teaching_group_id) DO NOTHING`,
			Args: []any{toStageID, fromStageID},
		},
		{
			SQL: `INSERT INTO student_stage_memberships(
				student_id, stage_id, class_id, status, walking, track, elective, japanese, total_score)
				SELECT m.student_id, ?, m.class_id, m.status, m.walking, m.track, m.elective,
					m.japanese, m.total_score
				FROM student_stage_memberships m
				JOIN class_stage_info i ON i.class_id = m.class_id AND i.stage_id = ?
				WHERE m.stage_id = ?
				ON CONFLICT(student_id, stage_id) DO NOTHING`,
			Args: []any{toStageID, toStageID, fromStageID},
		},
		{
			SQL: `INSERT INTO student_teaching_groups(student_id, stage_id, subject, teaching_group_id)
				SELECT g.student_id, ?, g.subject, target.id
				FROM student_teaching_groups g
				JOIN teaching_groups source ON source.id = g.teaching_group_id
				JOIN teaching_groups target ON target.stage_id = ?
					AND target.subject = source.subject AND target.name = source.name
				WHERE g.stage_id = ?
				ON CONFLICT(student_id, stage_id, subject) DO NOTHING`,
			Args: []any{toStageID, toStageID, fromStageID},
		},
		{
			SQL: `UPDATE cohort_stages SET copied_from_stage_id = ?, review_status = 'pending',
				confirmed_at = NULL WHERE id = ?`,
			Args: []any{fromStageID, toStageID},
		},
		{
			SQL: `INSERT INTO audit_log(action, entity, entity_id, detail_json, created_at, operator)
				VALUES ('copy_stage', 'cohort_stage', ?, ?, ?, 'local')`,
			Args: []any{toStageID, fmt.Sprintf(`{"from":%d}`, fromStageID), now},
		},
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return StageCopySummary{}, err
	}
	summary := StageCopySummary{FromStageID: fromStageID}
	counts, err := s.db.Query(ctx, `SELECT
		(SELECT COUNT(*) FROM class_stage_info WHERE stage_id = ?),
		(SELECT COUNT(*) FROM teaching_groups WHERE stage_id = ?),
		(SELECT COUNT(*) FROM assignments a JOIN teaching_groups g ON g.id = a.teaching_group_id
			WHERE g.stage_id = ?),
		(SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?)`,
		toStageID, toStageID, toStageID, toStageID)
	if err != nil {
		return summary, err
	}
	if len(counts.Values) > 0 {
		summary.Classes = int(intValue(counts.Values[0][0]))
		summary.TeachingGroups = int(intValue(counts.Values[0][1]))
		summary.Assignments = int(intValue(counts.Values[0][2]))
		summary.Students = int(intValue(counts.Values[0][3]))
	}
	return summary, nil
}

// ConfirmStageReview 把阶段标记为已经核对。
func (s *Store) ConfirmStageReview(ctx context.Context, stageID int64) error {
	_, err := s.db.Execute(ctx, `UPDATE cohort_stages SET review_status = 'confirmed',
		confirmed_at = ? WHERE id = ?`, s.now().Format(time.RFC3339Nano), stageID)
	return err
}
