package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/school"
)

// PhaseIndex 返回阶段对应的 1-based 序号（g1=1, g2=2, g3=3, graduated=4）。
func PhaseIndex(phase string) int {
	switch phase {
	case PhaseG1:
		return 1
	case PhaseG2:
		return 2
	case PhaseG3:
		return 3
	case PhaseGraduated:
		return 4
	default:
		return 0
	}
}

// ValidSchoolPhase 判断阶段是否为在读年级阶段（g1/g2/g3）。
func ValidSchoolPhase(phase string) bool {
	return phase == PhaseG1 || phase == PhaseG2 || phase == PhaseG3
}

// EncodeStageID 把 (cohortID, phase) 编码为阶段 1 界面使用的确定性标识。
func EncodeStageID(cohortID int64, phase string) int64 {
	idx := PhaseIndex(phase)
	if cohortID <= 0 || idx < 1 || idx > 3 {
		return 0
	}
	return cohortID*10 + int64(idx)
}

// DecodeStageID 从阶段 1 界面标识还原 (cohortID, phase)。
func DecodeStageID(stageID int64) (int64, string, bool) {
	if stageID <= 0 {
		return 0, "", false
	}
	cohortID := stageID / 10
	idx := int(stageID % 10)
	if cohortID <= 0 || idx < 1 || idx > 3 {
		return 0, "", false
	}
	return cohortID, school.StageKey(idx), true
}

// CreateCohort 新建一个按入学年份命名的级，并确定当前所在阶段。
func (s *Store) CreateCohort(ctx context.Context, enrollmentYear, currentAcademicYearStart int) (Cohort, error) {
	if err := school.ValidateEnrollmentYear(enrollmentYear, currentAcademicYearStart); err != nil {
		return Cohort{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM cohorts WHERE enrollment_year = ?`, enrollmentYear)
	if err != nil {
		return Cohort{}, err
	}
	if len(rows.Values) > 0 {
		return Cohort{}, ErrCohortExists
	}

	index := school.StageIndex(enrollmentYear, currentAcademicYearStart)
	phase := PhaseG1
	var graduatedAt *string
	now := s.now().Format(time.RFC3339Nano)
	academicStart := currentAcademicYearStart
	switch {
	case index <= school.StageFirst:
		phase = PhaseG1
		academicStart = enrollmentYear
	case index == school.StageSecond:
		phase = PhaseG2
		academicStart = enrollmentYear + 1
	case index == school.StageThird:
		phase = PhaseG3
		academicStart = enrollmentYear + 2
	default:
		phase = PhaseGraduated
		academicStart = enrollmentYear + 2
		g := now
		graduatedAt = &g
	}
	graduationYear := enrollmentYear + 3

	var gradArg any
	if graduatedAt != nil {
		gradArg = *graduatedAt
	}
	res, err := s.db.Execute(ctx, `INSERT INTO cohorts(
		enrollment_year, graduation_year, current_phase, current_academic_year_start, graduated_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		enrollmentYear, graduationYear, phase, academicStart, gradArg, now)
	if err != nil {
		return Cohort{}, err
	}
	id := res.LastInsertID
	if err := s.setStageReviewState(ctx, id, phase, "confirmed", 0); err != nil {
		return Cohort{}, err
	}
	auditStmt := auditLogStatement("create_cohort", "cohort", id, map[string]any{
		"enrollment_year": enrollmentYear,
		"current_phase":   phase,
	}, now)
	if _, err := s.db.Execute(ctx, auditStmt.SQL, auditStmt.Args...); err != nil {
		return Cohort{}, err
	}
	return s.GetCohort(ctx, id)
}

// ListCohorts 返回全部级，按入学年份从近到远排列。
func (s *Store) ListCohorts(ctx context.Context) ([]Cohort, error) {
	rows, err := s.db.Query(ctx, `SELECT id, enrollment_year, graduation_year, current_phase,
		current_academic_year_start, graduated_at, created_at
		FROM cohorts ORDER BY enrollment_year DESC`)
	if err != nil {
		return nil, err
	}
	cohorts := make([]Cohort, 0, len(rows.Values))
	for _, row := range rows.Values {
		cohort, err := cohortFromRow(row)
		if err != nil {
			return nil, err
		}
		cohorts = append(cohorts, cohort)
	}
	return cohorts, nil
}

// GetCohort 读取单个级。
func (s *Store) GetCohort(ctx context.Context, id int64) (Cohort, error) {
	rows, err := s.db.Query(ctx, `SELECT id, enrollment_year, graduation_year, current_phase,
		current_academic_year_start, graduated_at, created_at
		FROM cohorts WHERE id = ?`, id)
	if err != nil {
		return Cohort{}, err
	}
	if len(rows.Values) == 0 {
		return Cohort{}, ErrNotFound
	}
	return cohortFromRow(rows.Values[0])
}

func cohortFromRow(row []any) (Cohort, error) {
	graduatedAt, err := nullableTime(row[5])
	if err != nil {
		return Cohort{}, err
	}
	createdAt, err := parseTime(row[6])
	if err != nil {
		return Cohort{}, err
	}
	phase := stringValue(row[3])
	status := "active"
	if phase == PhaseGraduated {
		status = PhaseGraduated
	}
	enroll := int(intValue(row[1]))
	return Cohort{
		ID:                       intValue(row[0]),
		Name:                     formatCohortLabel(enroll),
		EnrollmentYear:           enroll,
		GraduationYear:           int(intValue(row[2])),
		CurrentPhase:             phase,
		CurrentAcademicYearStart: int(intValue(row[4])),
		GraduatedAt:              graduatedAt,
		Status:                   status,
		CreatedAt:                createdAt,
	}, nil
}

// PromoteCohort 执行年级升学流转（支持模式 A 原班直升与模式 B 选科重分班）。
func (s *Store) PromoteCohort(ctx context.Context, cohortID int64, input PromoteInput) (PromoteResult, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return PromoteResult{}, err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return PromoteResult{}, ErrCohortGraduated
	}
	toPhase := input.ToPhase
	if !ValidSchoolPhase(toPhase) || PhaseIndex(toPhase) <= PhaseIndex(cohort.CurrentPhase) {
		return PromoteResult{}, fmt.Errorf("%w: %s -> %s", ErrInvalidPhaseTransition, cohort.CurrentPhase, toPhase)
	}
	mode := input.Mode
	if mode == "" {
		mode = PromoteDirect
	}
	if mode != PromoteDirect && mode != PromoteReclassify {
		return PromoteResult{}, fmt.Errorf("未知升学模式 %q", mode)
	}
	toYear := input.ToAcademicYearStart
	if toYear <= 0 {
		toYear = cohort.EnrollmentYear + PhaseIndex(toPhase) - 1
	}

	fromPhase := cohort.CurrentPhase
	now := s.now().Format(time.RFC3339Nano)

	var statements []sqlite.Statement
	statements = append(statements, sqlite.Statement{
		SQL:  `UPDATE cohorts SET current_phase = ?, current_academic_year_start = ? WHERE id = ?`,
		Args: []any{toPhase, toYear, cohortID},
	})

	if mode == PromoteDirect {
		fromRows, err := s.db.Query(ctx, `SELECT id, name FROM classes WHERE cohort_id = ? AND phase = ? AND status = 'active' ORDER BY id`, cohortID, fromPhase)
		if err != nil {
			return PromoteResult{}, err
		}
		toRows, err := s.db.Query(ctx, `SELECT id, name, continued_from_class_id FROM classes WHERE cohort_id = ? AND phase = ?`, cohortID, toPhase)
		if err != nil {
			return PromoteResult{}, err
		}
		type existingTargetClass struct {
			id            int64
			continuedFrom *int64
		}
		toByName := make(map[string]existingTargetClass, len(toRows.Values))
		for _, r := range toRows.Values {
			var cf *int64
			if v, ok := nullableInt64(r[2]); ok {
				cf = &v
			}
			toByName[normalizeClassName(stringValue(r[1]))] = existingTargetClass{
				id:            intValue(r[0]),
				continuedFrom: cf,
			}
		}

		// 1. 按班级粒度校验/回填 continued_from_class_id，缺失的新建目标阶段延续班。
		for _, r := range fromRows.Values {
			oldID := intValue(r[0])
			name := normalizeClassName(stringValue(r[1]))
			if tc, exists := toByName[name]; exists {
				if tc.continuedFrom != nil && *tc.continuedFrom != oldID {
					return PromoteResult{}, fmt.Errorf("目标阶段班级 %s 已关联其他来源班级 (%d != %d)", name, *tc.continuedFrom, oldID)
				}
				if tc.continuedFrom == nil {
					statements = append(statements, sqlite.Statement{
						SQL:  `UPDATE classes SET continued_from_class_id = ? WHERE id = ? AND continued_from_class_id IS NULL`,
						Args: []any{oldID, tc.id},
					})
				}
				continue
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO classes(cohort_id, phase, name, continued_from_class_id, track_hint, room, status, import_run_id, source_sheet, source_row, created_at)
					SELECT cohort_id, ?, name, id, track_hint, room, 'active', import_run_id, source_sheet, source_row, ?
					FROM classes WHERE id = ?`,
				Args: []any{toPhase, now, oldID},
			})
		}
		// 2. 在新阶段补建缺失的班主任职责（目标班已有活跃班主任时保留原配置）。
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO class_head_teachers(class_id, teacher_id, cohort_id, phase, status, started_at)
				SELECT new_c.id, ht.teacher_id, ht.cohort_id, ?, 'active', ?
				FROM class_head_teachers ht
				JOIN classes old_c ON old_c.id = ht.class_id
				JOIN classes new_c ON new_c.continued_from_class_id = old_c.id
				WHERE ht.cohort_id = ? AND ht.phase = ? AND ht.status = 'active' AND new_c.phase = ?
				AND NOT EXISTS (
					SELECT 1 FROM class_head_teachers existing_ht
					WHERE existing_ht.class_id = new_c.id AND existing_ht.status = 'active'
				)`,
			Args: []any{toPhase, now, cohortID, fromPhase, toPhase},
		})
		// 3. 在新阶段补建缺失的行政常规教学班。
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO teaching_groups(cohort_id, phase, subject, name, kind, admin_class_id, status, import_run_id, source_sheet, source_row, created_at)
				SELECT old_tg.cohort_id, ?, old_tg.subject, old_tg.name, 'admin', new_c.id, 'active', old_tg.import_run_id, old_tg.source_sheet, old_tg.source_row, ?
				FROM teaching_groups old_tg
				JOIN classes new_c ON new_c.continued_from_class_id = old_tg.admin_class_id
				WHERE old_tg.cohort_id = ? AND old_tg.phase = ? AND old_tg.kind = 'admin' AND old_tg.status = 'active' AND new_c.phase = ?
				ON CONFLICT(cohort_id, phase, subject, name) DO UPDATE SET
					admin_class_id = COALESCE(teaching_groups.admin_class_id, excluded.admin_class_id)`,
			Args: []any{toPhase, now, cohortID, fromPhase, toPhase},
		})
		// 4. 在新阶段补建缺失的常规教学班任课关系。
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO teaching_assignments(teacher_id, cohort_id, phase, teaching_group_id, role, status, started_at, import_run_id, source_sheet, source_row)
				SELECT ta.teacher_id, ta.cohort_id, ?, new_tg.id, ta.role, 'active', ?, ta.import_run_id, ta.source_sheet, ta.source_row
				FROM teaching_assignments ta
				JOIN teaching_groups old_tg ON old_tg.id = ta.teaching_group_id
				JOIN teaching_groups new_tg ON new_tg.cohort_id = old_tg.cohort_id AND new_tg.phase = ?
					AND new_tg.subject = old_tg.subject AND new_tg.name = old_tg.name
				WHERE ta.cohort_id = ? AND ta.phase = ? AND ta.status = 'active' AND old_tg.kind = 'admin'
				AND NOT EXISTS (
					SELECT 1 FROM teaching_assignments existing_ta
					WHERE existing_ta.teaching_group_id = new_tg.id AND existing_ta.role = ta.role AND existing_ta.status = 'active'
				)
				ON CONFLICT(teacher_id, teaching_group_id, role) WHERE status = 'active' DO NOTHING`,
			Args: []any{toPhase, now, toPhase, cohortID, fromPhase},
		})
		// 5. 先记录学生升学事件，再把当前班级指针切到新阶段班级。
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
					SELECT st.id, st.cohort_id, ?, 'promoted',
						json_object('phase', ?, 'class_id', st.current_class_id, 'class_name', COALESCE(old_c.name, '')),
						json_object('phase', ?, 'class_id', new_c.id, 'class_name', new_c.name, 'mode', 'direct'),
						'原班直升', ?
					FROM students st
					LEFT JOIN classes old_c ON old_c.id = st.current_class_id
					JOIN classes new_c ON new_c.continued_from_class_id = st.current_class_id AND new_c.phase = ?
					WHERE st.cohort_id = ? AND st.status NOT IN ('graduated','transferred','transferred_out','left_school')`,
				Args: []any{toPhase, fromPhase, toPhase, now, toPhase, cohortID},
			},
			sqlite.Statement{
				SQL: `UPDATE students SET current_class_id = (
						SELECT new_c.id FROM classes new_c
						WHERE new_c.continued_from_class_id = students.current_class_id AND new_c.phase = ?
					), updated_at = ?
					WHERE cohort_id = ?
					AND EXISTS (SELECT 1 FROM classes new_c WHERE new_c.continued_from_class_id = students.current_class_id AND new_c.phase = ?)
					AND status NOT IN ('graduated','transferred','transferred_out','left_school')`,
				Args: []any{toPhase, now, cohortID, toPhase},
			},
		)
	} else if mode == PromoteReclassify {
		// 模式 B（选科重分班）：记录升学事件、清空当前分班等待重新导入，归档旧班级与任课。
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
					SELECT st.id, st.cohort_id, ?, 'promoted',
						json_object('phase', ?, 'class_id', st.current_class_id),
						json_object('phase', ?, 'mode', 'reclassify'),
						'选科重分班', ?
					FROM students st
					WHERE st.cohort_id = ? AND st.status NOT IN ('graduated','transferred','transferred_out','left_school')`,
				Args: []any{toPhase, fromPhase, toPhase, now, cohortID},
			},
			sqlite.Statement{
				SQL: `UPDATE students SET current_class_id = NULL, updated_at = ?
					WHERE cohort_id = ? AND status NOT IN ('graduated','transferred','transferred_out','left_school')`,
				Args: []any{now, cohortID},
			},
		)
	}

	// 归档旧阶段班级、教学班并释放旧阶段任课与班主任占用（end_reason = 'promoted'）。
	statements = append(statements,
		sqlite.Statement{
			SQL: `UPDATE teaching_assignments SET status = 'ended', ended_at = ?, end_reason = 'promoted'
				WHERE cohort_id = ? AND phase = ? AND status = 'active'`,
			Args: []any{now, cohortID, fromPhase},
		},
		sqlite.Statement{
			SQL: `UPDATE class_head_teachers SET status = 'ended', ended_at = ?, end_reason = 'promoted'
				WHERE cohort_id = ? AND phase = ? AND status = 'active'`,
			Args: []any{now, cohortID, fromPhase},
		},
		sqlite.Statement{
			SQL: `UPDATE teaching_group_students SET ended_at = ?
				WHERE cohort_id = ? AND phase = ? AND ended_at IS NULL`,
			Args: []any{now, cohortID, fromPhase},
		},
		sqlite.Statement{
			SQL: `UPDATE teaching_groups SET status = 'archived'
				WHERE cohort_id = ? AND phase = ? AND status = 'active'`,
			Args: []any{cohortID, fromPhase},
		},
		sqlite.Statement{
			SQL: `UPDATE classes SET status = 'archived'
				WHERE cohort_id = ? AND phase = ? AND status = 'active'`,
			Args: []any{cohortID, fromPhase},
		},
		bumpRevisionStatement(fmt.Sprintf("cohort:%d", cohortID), now),
		auditLogStatement("promote_cohort", "cohort", cohortID, map[string]any{
			"from_phase": fromPhase,
			"to_phase":   toPhase,
			"to_year":    toYear,
			"mode":       mode,
		}, now),
	)

	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return PromoteResult{}, err
	}

	counts, err := s.StageCounts(ctx, cohortID, toYear, toPhase)
	if err != nil {
		return PromoteResult{}, err
	}
	headCnt, err := s.countQuery(ctx, `SELECT COUNT(*) FROM class_head_teachers WHERE cohort_id = ? AND phase = ? AND status = 'active'`, cohortID, toPhase)
	if err != nil {
		return PromoteResult{}, err
	}
	res := PromoteResult{
		CohortID:              cohortID,
		FromPhase:             fromPhase,
		ToPhase:               toPhase,
		ToAcademicYearStart:   toYear,
		Mode:                  mode,
		ClassesCreated:        counts.Classes,
		TeachingGroupsCreated: counts.TeachingGroups,
		AssignmentsCarried:    counts.Assignments,
		HeadTeachersCarried:   headCnt,
		StudentsUpdated:       counts.Students,
		StageCopySummary: StageCopySummary{
			FromStageID:    EncodeStageID(cohortID, fromPhase),
			ToStageID:      EncodeStageID(cohortID, toPhase),
			Classes:        counts.Classes,
			Students:       counts.Students,
			TeachingGroups: counts.TeachingGroups,
			Assignments:    counts.Assignments,
		},
	}
	return res, nil
}

// PreviewGraduateCohort 统计年级毕业封存前的影响摘要：将封存学生数、考试数、释放变空闲的教师及仍跨级在任教师。
func (s *Store) PreviewGraduateCohort(ctx context.Context, cohortID int64) (GraduatePreview, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return GraduatePreview{}, err
	}
	activeStudents, err := s.countQuery(ctx, `SELECT COUNT(*) FROM students
		WHERE cohort_id = ? AND status IN ('active','suspended','borrowed')`, cohortID)
	if err != nil {
		return GraduatePreview{}, err
	}
	examCount, err := s.countQuery(ctx, `SELECT COUNT(*) FROM exam_cohorts WHERE cohort_id = ?`, cohortID)
	if err != nil {
		return GraduatePreview{}, err
	}

	teacherRows, err := s.db.Query(ctx, `SELECT DISTINCT teacher_id FROM (
		SELECT teacher_id FROM teaching_assignments WHERE cohort_id = ? AND status = 'active'
		UNION
		SELECT teacher_id FROM class_head_teachers WHERE cohort_id = ? AND status = 'active'
	) ORDER BY teacher_id`, cohortID, cohortID)
	if err != nil {
		return GraduatePreview{}, err
	}
	var teacherIDs []int64
	for _, row := range teacherRows.Values {
		if id := intValue(row[0]); id > 0 {
			teacherIDs = append(teacherIDs, id)
		}
	}

	preview := GraduatePreview{
		CohortID:              cohortID,
		EnrollmentYear:        cohort.EnrollmentYear,
		GraduationYear:        cohort.GraduationYear,
		CurrentPhase:          cohort.CurrentPhase,
		ActiveStudents:        activeStudents,
		ExamCount:             examCount,
		TotalTeachersAffected: len(teacherIDs),
	}
	if len(teacherIDs) == 0 {
		return preview, nil
	}

	occupancies, err := s.teacherOccupancyByIDs(ctx, teacherIDs)
	if err != nil {
		return GraduatePreview{}, err
	}
	for _, tid := range teacherIDs {
		occ, ok := occupancies[tid]
		if !ok {
			continue
		}
		hasOther := false
		for _, cid := range occ.ActiveCohorts {
			if cid != cohortID {
				hasOther = true
				break
			}
		}
		if hasOther {
			preview.StillActiveCrossCohortCount++
			preview.CrossCohortTeachers = append(preview.CrossCohortTeachers, occ)
		} else {
			preview.ReleasedToIdleCount++
			preview.ReleasedTeachers = append(preview.ReleasedTeachers, occ)
		}
	}
	return preview, nil
}

// GraduateCohort 原子执行年级毕业封存并释放仅在当前年级任教的教师占用。
func (s *Store) GraduateCohort(ctx context.Context, cohortID int64) (GraduateResult, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return GraduateResult{}, err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return GraduateResult{}, ErrCohortGraduated
	}
	preview, err := s.PreviewGraduateCohort(ctx, cohortID)
	if err != nil {
		return GraduateResult{}, err
	}

	gradClasses, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM classes WHERE cohort_id = ? AND status <> 'graduated'`, cohortID)
	gradGroups, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM teaching_groups WHERE cohort_id = ? AND status <> 'graduated'`, cohortID)
	gradStudents, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM students WHERE cohort_id = ? AND status IN ('active','suspended','borrowed')`, cohortID)
	endedAssign, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM teaching_assignments WHERE cohort_id = ? AND status = 'active'`, cohortID)
	endedHeads, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM class_head_teachers WHERE cohort_id = ? AND status = 'active'`, cohortID)

	nowTime := s.now()
	now := nowTime.Format(time.RFC3339Nano)

	statements := []sqlite.Statement{
		{
			SQL:  `UPDATE cohorts SET current_phase = 'graduated', graduated_at = ? WHERE id = ?`,
			Args: []any{now, cohortID},
		},
		{
			SQL:  `UPDATE classes SET status = 'graduated' WHERE cohort_id = ?`,
			Args: []any{cohortID},
		},
		{
			SQL:  `UPDATE teaching_groups SET status = 'graduated' WHERE cohort_id = ?`,
			Args: []any{cohortID},
		},
		{
			SQL: `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
				SELECT id, cohort_id, ?, 'graduated',
					json_object('status', status, 'phase', ?),
					json_object('status', 'graduated', 'phase', 'graduated'),
					'年级毕业封存', ?
				FROM students WHERE cohort_id = ? AND status IN ('active','suspended','borrowed')`,
			Args: []any{cohort.CurrentPhase, cohort.CurrentPhase, now, cohortID},
		},
		{
			SQL: `UPDATE students SET status = 'graduated', updated_at = ?
				WHERE cohort_id = ? AND status IN ('active','suspended','borrowed')`,
			Args: []any{now, cohortID},
		},
		{
			SQL: `UPDATE teaching_assignments SET status = 'ended', ended_at = ?, end_reason = 'graduated'
				WHERE cohort_id = ? AND status = 'active'`,
			Args: []any{now, cohortID},
		},
		{
			SQL: `UPDATE class_head_teachers SET status = 'ended', ended_at = ?, end_reason = 'graduated'
				WHERE cohort_id = ? AND status = 'active'`,
			Args: []any{now, cohortID},
		},
		{
			SQL: `UPDATE teaching_group_students SET ended_at = ?
				WHERE cohort_id = ? AND ended_at IS NULL`,
			Args: []any{now, cohortID},
		},
		bumpRevisionStatement(fmt.Sprintf("cohort:%d", cohortID), now),
		auditLogStatement("graduate_cohort", "cohort", cohortID, map[string]any{
			"enrollment_year":    cohort.EnrollmentYear,
			"graduated_students": gradStudents,
			"released_to_idle":   preview.ReleasedToIdleCount,
		}, now),
	}

	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return GraduateResult{}, err
	}
	return GraduateResult{
		CohortID:          cohortID,
		GraduatedAt:       nowTime,
		GraduatedClasses:  gradClasses,
		GraduatedGroups:   gradGroups,
		GraduatedStudents: gradStudents,
		EndedAssignments:  endedAssign,
		EndedHeadTeachers: endedHeads,
		ReleasedToIdle:    preview.ReleasedToIdleCount,
	}, nil
}

// -----------------------------------------------------------------------------
// Stage 1 UI Compatibility Layer (Backed strictly by Schema v1 + app_settings)
// -----------------------------------------------------------------------------

type stageReviewMeta struct {
	AcademicYearStart int    `json:"academic_year_start"`
	ReviewStatus      string `json:"review_status"`
	CopiedFromStageID int64  `json:"copied_from_stage_id,omitempty"`
	ConfirmedAt       string `json:"confirmed_at,omitempty"`
}

func stageMetaKey(cohortID int64, phase string) string {
	return fmt.Sprintf("stage_meta:%d:%s", cohortID, phase)
}

func (s *Store) getStageReviewState(ctx context.Context, cohortID int64, phase string) (stageReviewMeta, bool, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE "key" = ?`, stageMetaKey(cohortID, phase))
	if err != nil {
		return stageReviewMeta{}, false, err
	}
	if len(rows.Values) == 0 {
		return stageReviewMeta{}, false, nil
	}
	var meta stageReviewMeta
	if raw := stringValue(rows.Values[0][0]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			return stageReviewMeta{}, false, err
		}
	}
	if meta.ReviewStatus == "" {
		meta.ReviewStatus = "confirmed"
	}
	return meta, true, nil
}

func (s *Store) setStageReviewState(ctx context.Context, cohortID int64, phase, reviewStatus string, copiedFromStageID int64) error {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return err
	}
	idx := PhaseIndex(phase)
	if idx < 1 || idx > 3 {
		return nil
	}
	confirmedAt := ""
	if reviewStatus == "confirmed" {
		confirmedAt = s.now().Format(time.RFC3339Nano)
	}
	meta := stageReviewMeta{
		AcademicYearStart: cohort.EnrollmentYear + idx - 1,
		ReviewStatus:      reviewStatus,
		CopiedFromStageID: copiedFromStageID,
		ConfirmedAt:       confirmedAt,
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.Execute(ctx, `INSERT INTO app_settings("key", value, updated_at)
		VALUES (?, ?, ?) ON CONFLICT("key") DO UPDATE SET value = excluded.value,
		updated_at = excluded.updated_at`, stageMetaKey(cohortID, phase), string(encoded), now)
	return err
}

// GetCohortStage 读取级在某个学年对应的阶段视图。
func (s *Store) GetCohortStage(ctx context.Context, cohortID int64, academicYearStart int) (CohortStage, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return CohortStage{}, err
	}
	index := school.StageIndex(cohort.EnrollmentYear, academicYearStart)
	if index < school.StageFirst || index > school.StageThird {
		return CohortStage{}, ErrNotFound
	}
	phase := school.StageKey(index)
	_, hasMeta, err := s.getStageReviewState(ctx, cohortID, phase)
	if err != nil {
		return CohortStage{}, err
	}
	hasData, err := s.phaseHasRecords(ctx, cohortID, phase)
	if err != nil {
		return CohortStage{}, err
	}
	currentIdx := PhaseIndex(cohort.CurrentPhase)
	if index > currentIdx && !hasMeta && !hasData {
		return CohortStage{}, ErrNotFound
	}
	return s.buildCohortStageView(ctx, cohort, phase, academicYearStart)
}

// EnsureCohortStage 确保级在指定学年阶段已初始化，并返回该阶段视图。
func (s *Store) EnsureCohortStage(ctx context.Context, cohortID int64, academicYearStart int, gradeLevel string) (CohortStage, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return CohortStage{}, err
	}
	if !ValidSchoolPhase(gradeLevel) {
		return CohortStage{}, fmt.Errorf("%w: %s", school.ErrGradeLevel, gradeLevel)
	}
	meta, hasMeta, err := s.getStageReviewState(ctx, cohortID, gradeLevel)
	if err != nil {
		return CohortStage{}, err
	}
	if !hasMeta {
		if err := s.setStageReviewState(ctx, cohortID, gradeLevel, "confirmed", 0); err != nil {
			return CohortStage{}, err
		}
		meta.ReviewStatus = "confirmed"
	}
	_ = meta
	return s.buildCohortStageView(ctx, cohort, gradeLevel, academicYearStart)
}

// ListCohortStages 返回一个级已存在的所有阶段视图（按高一到高三排序）。
func (s *Store) ListCohortStages(ctx context.Context, cohortID int64) ([]CohortStage, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return nil, err
	}
	maxIdx := PhaseIndex(cohort.CurrentPhase)
	if maxIdx > 3 {
		maxIdx = 3
	}
	var stages []CohortStage
	for idx := 1; idx <= 3; idx++ {
		phase := school.StageKey(idx)
		_, hasMeta, err := s.getStageReviewState(ctx, cohortID, phase)
		if err != nil {
			return nil, err
		}
		hasData, err := s.phaseHasRecords(ctx, cohortID, phase)
		if err != nil {
			return nil, err
		}
		if idx <= maxIdx || hasMeta || hasData {
			stage, err := s.buildCohortStageView(ctx, cohort, phase, cohort.EnrollmentYear+idx-1)
			if err != nil {
				return nil, err
			}
			stages = append(stages, stage)
		}
	}
	return stages, nil
}

// ConfirmStageReview 把阶段标记为已核对。
func (s *Store) ConfirmStageReview(ctx context.Context, stageID int64) error {
	cohortID, phase, ok := DecodeStageID(stageID)
	if !ok {
		return ErrNotFound
	}
	meta, _, err := s.getStageReviewState(ctx, cohortID, phase)
	if err != nil {
		return err
	}
	return s.setStageReviewState(ctx, cohortID, phase, "confirmed", meta.CopiedFromStageID)
}

// CopyStageFromPrevious 兼容阶段 1 界面切换学年自动升学：调用 PromoteCohort(Mode=PromoteDirect) 延续上一阶段结构并标记待核对。
func (s *Store) CopyStageFromPrevious(ctx context.Context, cohortID, toStageID int64) (StageCopySummary, error) {
	cID, toPhase, ok := DecodeStageID(toStageID)
	if !ok || cID != cohortID {
		return StageCopySummary{}, ErrNotFound
	}
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return StageCopySummary{}, err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return StageCopySummary{}, nil
	}
	if PhaseIndex(toPhase) <= PhaseIndex(cohort.CurrentPhase) {
		return StageCopySummary{}, nil
	}
	fromStageID := EncodeStageID(cohortID, cohort.CurrentPhase)
	res, err := s.PromoteCohort(ctx, cohortID, PromoteInput{
		ToPhase:             toPhase,
		ToAcademicYearStart: cohort.EnrollmentYear + PhaseIndex(toPhase) - 1,
		Mode:                PromoteDirect,
	})
	if err != nil {
		return StageCopySummary{}, err
	}
	if err := s.setStageReviewState(ctx, cohortID, toPhase, "pending", fromStageID); err != nil {
		return StageCopySummary{}, err
	}
	return res.StageCopySummary, nil
}

// StageCounts 汇总一个级在某个阶段下的班级、学生、教师、教学班、任课与基数数量。
func (s *Store) StageCounts(ctx context.Context, cohortID int64, academicYearStart int, gradeLevel string) (StageCounts, error) {
	if _, err := s.GetCohort(ctx, cohortID); err != nil {
		return StageCounts{}, err
	}
	phase := gradeLevel
	if !ValidSchoolPhase(phase) {
		return StageCounts{}, nil
	}
	var counts StageCounts
	var err error
	if counts.Classes, err = s.countQuery(ctx, `SELECT COUNT(*) FROM classes WHERE cohort_id = ? AND phase = ?`, cohortID, phase); err != nil {
		return StageCounts{}, err
	}
	if counts.Students, err = s.countQuery(ctx, `SELECT COUNT(*) FROM students st
		JOIN classes c ON c.id = st.current_class_id
		WHERE st.cohort_id = ? AND c.phase = ?`, cohortID, phase); err != nil {
		return StageCounts{}, err
	}
	if counts.Students == 0 {
		// 若学生尚未分配或跨阶段查看，尝试从考试快照统计。
		if examStuds, qErr := s.countQuery(ctx, `SELECT COUNT(DISTINCT ep.student_id)
			FROM exam_participants ep
			JOIN exam_cohorts ec ON ec.id = ep.exam_cohort_id
			WHERE ec.cohort_id = ? AND ec.phase_at_exam = ?`, cohortID, phase); qErr == nil && examStuds > 0 {
			counts.Students = examStuds
		}
	}
	if counts.Teachers, err = s.countQuery(ctx, `SELECT COUNT(DISTINCT teacher_id) FROM teaching_assignments
		WHERE cohort_id = ? AND phase = ?`, cohortID, phase); err != nil {
		return StageCounts{}, err
	}
	if counts.TeachingGroups, err = s.countQuery(ctx, `SELECT COUNT(*) FROM teaching_groups
		WHERE cohort_id = ? AND phase = ?`, cohortID, phase); err != nil {
		return StageCounts{}, err
	}
	if counts.Assignments, err = s.countQuery(ctx, `SELECT COUNT(*) FROM teaching_assignments
		WHERE cohort_id = ? AND phase = ?`, cohortID, phase); err != nil {
		return StageCounts{}, err
	}
	entryCount, err := s.countQuery(ctx, `SELECT COUNT(*) FROM student_entry_scores es
		JOIN students st ON st.id = es.student_id WHERE st.cohort_id = ?`, cohortID)
	if err != nil {
		return StageCounts{}, err
	}
	overrideCount, err := s.countQuery(ctx, `SELECT COUNT(*) FROM cohort_baseline_overrides WHERE cohort_id = ?`, cohortID)
	if err != nil {
		return StageCounts{}, err
	}
	counts.Baselines = entryCount + overrideCount
	_ = academicYearStart
	return counts, nil
}

// StageClassNames 返回级在某个阶段下的班级名称列表。
func (s *Store) StageClassNames(ctx context.Context, cohortID int64, academicYearStart int, gradeLevel string) ([]string, error) {
	if !ValidSchoolPhase(gradeLevel) {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT name FROM classes WHERE cohort_id = ? AND phase = ? ORDER BY name`,
		cohortID, gradeLevel)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows.Values))
	for _, row := range rows.Values {
		names = append(names, stringValue(row[0]))
	}
	_ = academicYearStart
	return names, nil
}

func (s *Store) buildCohortStageView(ctx context.Context, cohort Cohort, phase string, academicYearStart int) (CohortStage, error) {
	meta, hasMeta, err := s.getStageReviewState(ctx, cohort.ID, phase)
	if err != nil {
		return CohortStage{}, err
	}
	reviewStatus := "confirmed"
	var copiedFrom *int64
	var confirmedAt *time.Time
	if hasMeta {
		reviewStatus = meta.ReviewStatus
		if meta.CopiedFromStageID > 0 {
			v := meta.CopiedFromStageID
			copiedFrom = &v
		}
		if meta.ConfirmedAt != "" {
			if parsed, pErr := parseTime(meta.ConfirmedAt); pErr == nil {
				confirmedAt = &parsed
			}
		}
	}
	if academicYearStart <= 0 {
		academicYearStart = cohort.EnrollmentYear + PhaseIndex(phase) - 1
	}
	return CohortStage{
		ID:                EncodeStageID(cohort.ID, phase),
		CohortID:          cohort.ID,
		AcademicYearStart: academicYearStart,
		GradeLevel:        phase,
		CopiedFromStageID: copiedFrom,
		ReviewStatus:      reviewStatus,
		ConfirmedAt:       confirmedAt,
	}, nil
}

func (s *Store) phaseHasRecords(ctx context.Context, cohortID int64, phase string) (bool, error) {
	cCnt, err := s.countQuery(ctx, `SELECT COUNT(*) FROM classes WHERE cohort_id = ? AND phase = ?`, cohortID, phase)
	if err != nil {
		return false, err
	}
	if cCnt > 0 {
		return true, nil
	}
	eCnt, err := s.countQuery(ctx, `SELECT COUNT(*) FROM exam_cohorts WHERE cohort_id = ? AND phase_at_exam = ?`, cohortID, phase)
	if err != nil {
		return false, err
	}
	return eCnt > 0, nil
}

func (s *Store) resolveCohortPhaseScope(ctx context.Context, cohortID, stageID int64) (Cohort, string, error) {
	if stageID > 0 {
		decodedCohort, phase, ok := DecodeStageID(stageID)
		if !ok {
			return Cohort{}, "", ErrNotFound
		}
		if cohortID > 0 && cohortID != decodedCohort {
			return Cohort{}, "", ErrNotFound
		}
		cohort, err := s.GetCohort(ctx, decodedCohort)
		if err != nil {
			return Cohort{}, "", err
		}
		return cohort, phase, nil
	}
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return Cohort{}, "", err
	}
	phase := cohort.CurrentPhase
	if phase == PhaseGraduated {
		phase = PhaseG3
	}
	return cohort, phase, nil
}

func (s *Store) countQuery(ctx context.Context, query string, args ...any) (int, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, nil
	}
	return int(intValue(rows.Values[0][0])), nil
}

func formatCohortLabel(enrollmentYear int) string {
	return strconv.Itoa(enrollmentYear) + "级"
}

func normalizePhaseText(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "g1", "高一":
		return PhaseG1
	case "g2", "高二":
		return PhaseG2
	case "g3", "高三":
		return PhaseG3
	case "graduated", "已毕业":
		return PhaseGraduated
	default:
		return phase
	}
}

var _ = errors.Is
