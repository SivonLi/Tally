package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/school"
)

// migrateLegacyExams 把旧批次的考试、报名、成绩、分数线和目标写入新的考试结构。
func (s *Store) migrateLegacyExams(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT b.id, t.academic_year, t.semester, b.exam_type, b.grade
		FROM batches b JOIN terms t ON t.id = b.term_id ORDER BY b.id`)
	if err != nil {
		return err
	}
	if len(rows.Values) == 0 {
		return nil
	}
	subjects, err := s.ListSubjects(ctx)
	if err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	examIDs := map[string]int64{}
	batchExamCohorts := map[int64]int64{}
	for _, row := range rows.Values {
		batchID, _ := row[0].(int64)
		academicYear, _ := row[1].(string)
		semester, _ := row[2].(string)
		examType, _ := row[3].(string)
		grade, _ := row[4].(string)
		start, err := school.ParseAcademicYearStart(academicYear)
		if err != nil {
			return fmt.Errorf("批次 %d 的学年无法解析: %w", batchID, err)
		}
		enrollment, err := school.EnrollmentYear(start, grade)
		if err != nil {
			return fmt.Errorf("批次 %d 的年级无法解析: %w", batchID, err)
		}
		cohortRows, err := s.db.Query(ctx, `SELECT id FROM cohorts WHERE enrollment_year = ?`, enrollment)
		if err != nil {
			return err
		}
		if len(cohortRows.Values) == 0 {
			return fmt.Errorf("批次 %d 找不到对应的级", batchID)
		}
		cohortID, _ := cohortRows.Values[0][0].(int64)
		stageRows, err := s.db.Query(ctx, `SELECT id, grade_level FROM cohort_stages
			WHERE cohort_id = ? AND academic_year_start = ?`, cohortID, start)
		if err != nil {
			return err
		}
		if len(stageRows.Values) == 0 {
			return fmt.Errorf("批次 %d 找不到对应的阶段", batchID)
		}
		stageID, _ := stageRows.Values[0][0].(int64)
		gradeLevel, _ := stageRows.Values[0][1].(string)
		term := normalizeLegacyTerm(semester)
		name := legacyExamName(examType)
		key := fmt.Sprintf("%d\x00%s\x00%s\x00%s", start, term, examType, name)
		examID, ok := examIDs[key]
		if !ok {
			rows, err := s.db.Query(ctx, `SELECT id FROM exams WHERE academic_year_start = ? AND term = ?
				AND exam_type = ? AND name = ?`, start, term, examType, name)
			if err != nil {
				return err
			}
			if len(rows.Values) > 0 {
				examID, _ = rows.Values[0][0].(int64)
			} else {
				result, err := s.db.Execute(ctx, `INSERT INTO exams(academic_year_start, term, exam_type,
					name, exam_date, sequence, status, created_at)
					VALUES (?, ?, ?, ?, '', 0, 'imported', ?)`, start, term, examType, name, now)
				if err != nil {
					return err
				}
				examID = result.LastInsertID
			}
			examIDs[key] = examID
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO exam_cohorts(exam_id, cohort_id, stage_id,
			grade_level_at_exam) VALUES (?, ?, ?, ?)`, examID, cohortID, stageID, gradeLevel); err != nil {
			return err
		}
		examCohortRows, err := s.db.Query(ctx, `SELECT id FROM exam_cohorts WHERE exam_id = ? AND cohort_id = ?`,
			examID, cohortID)
		if err != nil {
			return err
		}
		if len(examCohortRows.Values) == 0 {
			continue
		}
		examCohortID, _ := examCohortRows.Values[0][0].(int64)
		batchExamCohorts[batchID] = examCohortID
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO assessments(exam_cohort_id, plan_version_id,
			status, revision, created_at)
			SELECT ?, b.current_plan_version_id,
				CASE WHEN b.status = 'calculated' THEN 'calculated' ELSE 'draft' END,
				CASE WHEN b.revision > 0 THEN b.revision ELSE 1 END, ?
			FROM batches b WHERE b.id = ?`, examCohortID, now, batchID); err != nil {
			return err
		}
		for _, subject := range subjects {
			if !subject.Enabled {
				continue
			}
			if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO exam_subjects(exam_id, cohort_id, subject,
				full_mark, enabled) VALUES (?, ?, ?, ?, 1)`,
				examID, cohortID, subject.Key, subject.FullMark); err != nil {
				return err
			}
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO class_stage_info(class_id, stage_id, status)
			SELECT DISTINCT m.class_id, m.stage_id, 'active' FROM student_stage_memberships m
			WHERE m.stage_id = ?`, stageID); err != nil {
			return err
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO exam_rosters(exam_id, cohort_id, student_id,
			class_id, track, total_score, status, subjects_json)
			SELECT ?, ?, m.student_id, m.class_id, m.track, m.total_score,
				CASE WHEN m.status = '' THEN 'active' ELSE m.status END,
				COALESCE((SELECT json_group_array(subject) FROM (
					SELECT g.subject FROM student_teaching_groups g
					WHERE g.student_id = m.student_id AND g.stage_id = m.stage_id ORDER BY g.subject)), '[]')
			FROM student_stage_memberships m WHERE m.stage_id = ?`,
			examID, cohortID, stageID); err != nil {
			return err
		}
	}
	if err := s.migrateLegacyExamScores(ctx, batchExamCohorts); err != nil {
		return err
	}
	if err := s.migrateLegacyExamAssessment(ctx, batchExamCohorts, now); err != nil {
		return err
	}
	for batchID, examCohortID := range batchExamCohorts {
		if err := s.ensureAssessmentStatGroups(ctx, batchID, examCohortID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyExamScores(ctx context.Context, batchExamCohorts map[int64]int64) error {
	for batchID, examCohortID := range batchExamCohorts {
		rows, err := s.db.Query(ctx, `SELECT exam_id, cohort_id FROM exam_cohorts WHERE id = ?`, examCohortID)
		if err != nil {
			return err
		}
		if len(rows.Values) == 0 {
			continue
		}
		examID, _ := rows.Values[0][0].(int64)
		cohortID, _ := rows.Values[0][1].(int64)
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO exam_scores(exam_id, cohort_id, student_id,
			subject, score, flag)
			SELECT ?, ?, sc.student_id, sc.subject, sc.score, sc.flag FROM scores sc
			JOIN exam_rosters r ON r.exam_id = ? AND r.cohort_id = ? AND r.student_id = sc.student_id
			WHERE sc.batch_id = ?`, examID, cohortID, examID, cohortID, batchID); err != nil {
			return err
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO exam_subject_cutoffs(exam_id, cohort_id,
			subject, track, cutoff)
			SELECT ?, ?, subject, track, cutoff FROM subject_cutoffs WHERE batch_id = ?`,
			examID, cohortID, batchID); err != nil {
			return err
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO exam_tier_cutoffs(exam_id, cohort_id,
			tier_key, track, cutoff)
			SELECT ?, ?, tier_key, track, cutoff FROM tier_cutoffs WHERE batch_id = ?`,
			examID, cohortID, batchID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyExamAssessment(ctx context.Context, batchExamCohorts map[int64]int64, now string) error {
	for batchID, examCohortID := range batchExamCohorts {
		rows, err := s.db.Query(ctx, `SELECT id FROM assessments WHERE exam_cohort_id = ?`, examCohortID)
		if err != nil {
			return err
		}
		if len(rows.Values) == 0 {
			continue
		}
		assessmentID, _ := rows.Values[0][0].(int64)
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO assessment_class_targets(assessment_id,
			class_id, tier_key, target_count)
			SELECT ?, class_id, tier_key, target_count FROM class_targets WHERE batch_id = ?`,
			assessmentID, batchID); err != nil {
			return err
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO assessment_stat_groups(assessment_id,
			subject, name, lead_class_id, baseline_average, baseline_ratio, explicit)
			SELECT ?, subject, name, lead_class_id, baseline_average, baseline_ratio, explicit
			FROM stat_groups WHERE batch_id = ?`, assessmentID, batchID); err != nil {
			return err
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO assessment_stat_group_targets(stat_group_id, target_count)
			SELECT g.id, sg.target FROM assessment_stat_groups g
			JOIN stat_groups sg ON sg.subject = g.subject AND sg.name = g.name AND sg.batch_id = ?
			WHERE g.assessment_id = ?`, batchID, assessmentID); err != nil {
			return err
		}
		if _, err := s.db.Execute(ctx, `INSERT OR IGNORE INTO assessment_stat_group_members(stat_group_id,
			teaching_group_id)
			SELECT g.id, m.teaching_group_id FROM assessment_stat_groups g
			JOIN stat_groups sg ON sg.subject = g.subject AND sg.name = g.name AND sg.batch_id = ?
			JOIN stat_group_members m ON m.stat_group_id = sg.id
			WHERE g.assessment_id = ?`, batchID, assessmentID); err != nil {
			return err
		}
		if _, err := s.db.Execute(ctx, `INSERT INTO assessment_target_adjustments(assessment_id, scope_type,
			scope_id, subject, kind, reason, value, created_at)
			SELECT ?, 'class', class_id, tier_key, 'adjustment', reason, new_value, created_at
			FROM target_adjustments ta WHERE ta.batch_id = ?
			AND NOT EXISTS (SELECT 1 FROM assessment_target_adjustments a
				WHERE a.assessment_id = ? AND a.scope_type = 'class' AND a.scope_id = ta.class_id
				AND a.subject = ta.tier_key AND a.kind = 'adjustment' AND a.reason = ta.reason
				AND a.created_at = ta.created_at)`, assessmentID, batchID, assessmentID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureAssessmentStatGroups(ctx context.Context, batchID, examCohortID int64) error {
	rows, err := s.db.Query(ctx, `SELECT a.id, ec.stage_id, ec.cohort_id FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id WHERE a.exam_cohort_id = ?`, examCohortID)
	if err != nil {
		return err
	}
	if len(rows.Values) == 0 {
		return nil
	}
	assessmentID, _ := rows.Values[0][0].(int64)
	stageID, _ := rows.Values[0][1].(int64)
	cohortID, _ := rows.Values[0][2].(int64)
	statements := []sqlite.Statement{
		{SQL: `INSERT OR IGNORE INTO assessment_stat_groups(assessment_id, subject, name, lead_class_id,
			baseline_average, baseline_ratio, baseline_source, explicit)
			SELECT ?, g.subject, g.name,
				c.id,
				COALESCE((SELECT cb.average FROM class_baselines cb
					WHERE cb.batch_id = ? AND cb.class_id = c.id AND cb.subject = g.subject), 0),
				COALESCE((SELECT cb.ratio FROM class_baselines cb
					WHERE cb.batch_id = ? AND cb.class_id = c.id AND cb.subject = g.subject), 0),
				'legacy', 0
			FROM teaching_groups g
			JOIN classes c ON c.id = COALESCE(
				(SELECT m.class_id FROM student_teaching_groups stg
					JOIN student_stage_memberships m ON m.student_id = stg.student_id AND m.stage_id = stg.stage_id
					WHERE stg.teaching_group_id = g.id AND m.stage_id = g.stage_id
					ORDER BY m.class_id LIMIT 1),
				(SELECT c2.id FROM classes c2 WHERE c2.cohort_id = ? AND c2.name = g.name LIMIT 1))
			WHERE g.stage_id = ?
			AND NOT EXISTS (SELECT 1 FROM assessment_stat_groups sg
				WHERE sg.assessment_id = ? AND sg.subject = g.subject AND sg.name = g.name)
			AND NOT EXISTS (SELECT 1 FROM assessment_stat_group_members m
				JOIN assessment_stat_groups eg ON eg.id = m.stat_group_id
				WHERE eg.assessment_id = ? AND eg.explicit = 1 AND m.teaching_group_id = g.id)`,
			Args: []any{assessmentID, batchID, batchID, cohortID, stageID, assessmentID, assessmentID}},
		{SQL: `INSERT OR IGNORE INTO assessment_stat_group_members(stat_group_id, teaching_group_id)
			SELECT sg.id, g.id FROM assessment_stat_groups sg, teaching_groups g
			WHERE sg.assessment_id = ? AND sg.explicit = 0 AND g.stage_id = ?
				AND g.subject = sg.subject AND g.name = sg.name`,
			Args: []any{assessmentID, stageID}},
		{SQL: `INSERT OR IGNORE INTO assessment_stat_group_targets(stat_group_id, target_count)
			SELECT sg.id, COALESCE(
				(SELECT ct.target_count FROM assessment_class_targets ct
					WHERE ct.assessment_id = ? AND ct.class_id = sg.lead_class_id AND ct.tier_key = 'single_subject'),
				(SELECT ct.target_count FROM assessment_class_targets ct
					WHERE ct.assessment_id = ? AND ct.class_id = sg.lead_class_id AND ct.tier_key = 'undergraduate'),
				0)
			FROM assessment_stat_groups sg
			WHERE sg.assessment_id = ? AND sg.explicit = 0`,
			Args: []any{assessmentID, assessmentID, assessmentID}},
	}
	_, err = s.db.Transaction(ctx, statements)
	return err
}

func (s *Store) ensureResultAssessmentColumn(ctx context.Context) error {
	exists, err := s.columnExists(ctx, "result_runs", "assessment_id")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := s.db.Execute(ctx, `ALTER TABLE result_runs ADD COLUMN assessment_id INTEGER`); err != nil {
		return fmt.Errorf("增加结果运行的本次考核关联: %w", err)
	}
	if _, err := s.db.Execute(ctx, `CREATE INDEX IF NOT EXISTS result_runs_assessment
		ON result_runs(assessment_id, is_official)`); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateLegacyResults(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT r.id, r.batch_id, r.plan_version_id, r.input_hash,
		r.is_official, r.created_at, t.academic_year, t.semester, b.exam_type, b.grade
		FROM result_runs r
		JOIN batches b ON b.id = r.batch_id
		JOIN terms t ON t.id = b.term_id
		WHERE r.assessment_id IS NULL`)
	if err != nil {
		return err
	}
	for _, row := range rows.Values {
		runID, _ := row[0].(int64)
		start, err := school.ParseAcademicYearStart(stringValue(row[6]))
		if err != nil {
			continue
		}
		enrollment, err := school.EnrollmentYear(start, stringValue(row[9]))
		if err != nil {
			continue
		}
		term := normalizeLegacyTerm(stringValue(row[7]))
		examType := stringValue(row[8])
		name := legacyExamName(examType)
		assessmentRows, err := s.db.Query(ctx, `SELECT a.id FROM assessments a
			JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
			JOIN exams e ON e.id = ec.exam_id
			JOIN cohorts c ON c.id = ec.cohort_id
			WHERE c.enrollment_year = ? AND e.academic_year_start = ? AND e.term = ?
				AND e.exam_type = ? AND e.name = ?
			ORDER BY a.id LIMIT 1`, enrollment, start, term, examType, name)
		if err != nil {
			return err
		}
		if len(assessmentRows.Values) == 0 {
			continue
		}
		assessmentID, _ := assessmentRows.Values[0][0].(int64)
		if _, err := s.db.Execute(ctx, `UPDATE result_runs SET assessment_id = ? WHERE id = ?`,
			assessmentID, runID); err != nil {
			return err
		}
		official, _ := row[4].(int64)
		if official != 0 {
			if _, err := s.db.Execute(ctx, `UPDATE assessments SET current_run_id = ?,
				plan_version_id = COALESCE(plan_version_id, ?), status = 'calculated'
				WHERE id = ?`, runID, row[2], assessmentID); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeLegacyTerm(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(text, "spring"), strings.Contains(text, "second"), strings.Contains(value, "下"),
		strings.Contains(value, "第二"), strings.Contains(value, "春"):
		return ExamTermSecond
	default:
		return ExamTermFirst
	}
}

func legacyExamName(examType string) string {
	switch examType {
	case ExamFinal:
		return "期末考试"
	case ExamGaokao:
		return "高考专项"
	default:
		return "期中考试"
	}
}
