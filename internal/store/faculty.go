package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// ListTeachersWithOccupancy 返回全校教师库及其基于教学班与班主任任期的实时占用状态。
func (s *Store) ListTeachersWithOccupancy(ctx context.Context, query, statusFilter, subjectFilter string) ([]TeacherOccupancy, error) {
	var where []string
	var args []any
	if text := strings.TrimSpace(query); text != "" {
		where = append(where, "(t.name LIKE ? OR t.teacher_no LIKE ? OR t.primary_subject LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}
	if strings.TrimSpace(subjectFilter) != "" {
		where = append(where, "t.primary_subject = ?")
		args = append(args, strings.TrimSpace(subjectFilter))
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := s.db.Query(ctx, `SELECT t.id FROM teachers t`+whereSQL+` ORDER BY t.name, t.teacher_no`, args...)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows.Values))
	for _, row := range rows.Values {
		ids = append(ids, intValue(row[0]))
	}
	if len(ids) == 0 {
		return nil, nil
	}
	occMap, err := s.teacherOccupancyByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]TeacherOccupancy, 0, len(ids))
	for _, id := range ids {
		occ, ok := occMap[id]
		if !ok {
			continue
		}
		if statusFilter != "" && occ.Status != statusFilter {
			continue
		}
		result = append(result, occ)
	}
	return result, nil
}

// DetectCrossGradeWarnings 检测教师在其他在读年级是否已有活跃任课或班主任任职，返回非阻断提示。
func (s *Store) DetectCrossGradeWarnings(ctx context.Context, teacherID, targetCohortID int64) (*CrossGradeWarning, error) {
	occMap, err := s.teacherOccupancyByIDs(ctx, []int64{teacherID})
	if err != nil {
		return nil, err
	}
	occ, ok := occMap[teacherID]
	if !ok {
		return nil, ErrNotFound
	}
	var other []TeacherActiveAssignment
	for _, item := range occ.ActiveAssignments {
		if item.CohortID != targetCohortID {
			other = append(other, item)
		}
	}
	if len(other) == 0 {
		return nil, nil
	}
	return &CrossGradeWarning{
		TeacherID:       occ.TeacherID,
		TeacherNo:       occ.TeacherNo,
		TeacherName:     occ.Name,
		ExistingTargets: other,
	}, nil
}

func (s *Store) teacherOccupancyByIDs(ctx context.Context, teacherIDs []int64) (map[int64]TeacherOccupancy, error) {
	result := make(map[int64]TeacherOccupancy, len(teacherIDs))
	if len(teacherIDs) == 0 {
		return result, nil
	}
	inClause := placeholders(len(teacherIDs))
	args := make([]any, len(teacherIDs))
	for i, id := range teacherIDs {
		args[i] = id
	}

	tRows, err := s.db.Query(ctx, `SELECT id, teacher_no, name, primary_subject, status
		FROM teachers WHERE id IN (`+inClause+`)`, args...)
	if err != nil {
		return nil, err
	}
	accountStatus := make(map[int64]string, len(tRows.Values))
	for _, row := range tRows.Values {
		tid := intValue(row[0])
		st := stringValue(row[4])
		accountStatus[tid] = st
		occStatus := TeacherStatusIdle
		if st == TeacherStatusInactive {
			occStatus = TeacherStatusInactive
		}
		result[tid] = TeacherOccupancy{
			TeacherID:      tid,
			TeacherNo:      stringValue(row[1]),
			Name:           stringValue(row[2]),
			PrimarySubject: stringValue(row[3]),
			Status:         occStatus,
		}
	}

	// 查询活跃任课关系（所属年级未毕业且 assignment.status = 'active'）。
	taRows, err := s.db.Query(ctx, `SELECT ta.teacher_id, ta.cohort_id, co.enrollment_year, co.graduation_year,
		ta.phase, COALESCE(c.id, 0), COALESCE(c.name, tg.name), tg.id, tg.name, tg.subject, ta.role
		FROM teaching_assignments ta
		JOIN cohorts co ON co.id = ta.cohort_id
		JOIN teaching_groups tg ON tg.id = ta.teaching_group_id
		LEFT JOIN classes c ON c.id = tg.admin_class_id
		WHERE ta.teacher_id IN (`+inClause+`) AND ta.status = 'active' AND co.current_phase <> 'graduated'
		ORDER BY co.enrollment_year DESC, tg.subject, tg.name`, args...)
	if err != nil {
		return nil, err
	}
	cohortSets := make(map[int64]map[int64]bool, len(teacherIDs))
	for _, row := range taRows.Values {
		tid := intValue(row[0])
		occ, ok := result[tid]
		if !ok {
			continue
		}
		cid := intValue(row[1])
		occ.ActiveAssignments = append(occ.ActiveAssignments, TeacherActiveAssignment{
			CohortID:          cid,
			EnrollmentYear:    int(intValue(row[2])),
			GraduationYear:    int(intValue(row[3])),
			Phase:             stringValue(row[4]),
			ClassID:           intValue(row[5]),
			ClassName:         stringValue(row[6]),
			TeachingGroupID:   intValue(row[7]),
			TeachingGroupName: stringValue(row[8]),
			Subject:           stringValue(row[9]),
			Role:              stringValue(row[10]),
		})
		if cohortSets[tid] == nil {
			cohortSets[tid] = map[int64]bool{}
		}
		cohortSets[tid][cid] = true
		result[tid] = occ
	}

	// 查询活跃班主任任职。
	htRows, err := s.db.Query(ctx, `SELECT ht.teacher_id, ht.cohort_id, co.enrollment_year, co.graduation_year,
		ht.phase, c.id, c.name
		FROM class_head_teachers ht
		JOIN cohorts co ON co.id = ht.cohort_id
		JOIN classes c ON c.id = ht.class_id
		WHERE ht.teacher_id IN (`+inClause+`) AND ht.status = 'active' AND co.current_phase <> 'graduated'
		ORDER BY co.enrollment_year DESC, c.name`, args...)
	if err != nil {
		return nil, err
	}
	for _, row := range htRows.Values {
		tid := intValue(row[0])
		occ, ok := result[tid]
		if !ok {
			continue
		}
		cid := intValue(row[1])
		occ.ActiveAssignments = append(occ.ActiveAssignments, TeacherActiveAssignment{
			CohortID:       cid,
			EnrollmentYear: int(intValue(row[2])),
			GraduationYear: int(intValue(row[3])),
			Phase:          stringValue(row[4]),
			ClassID:        intValue(row[5]),
			ClassName:      stringValue(row[6]),
			Role:           "head_teacher",
		})
		if cohortSets[tid] == nil {
			cohortSets[tid] = map[int64]bool{}
		}
		cohortSets[tid][cid] = true
		result[tid] = occ
	}

	// 查询最近一次释放记录（用于全校空闲教师显示最近带班去向）。
	relRows, err := s.db.Query(ctx, `SELECT teacher_id, cohort_id, enrollment_year, phase, class_name, subject, end_reason, ended_at FROM (
		SELECT ta.teacher_id, ta.cohort_id, co.enrollment_year, ta.phase,
			COALESCE(c.name, tg.name) AS class_name, tg.subject, ta.end_reason, ta.ended_at
		FROM teaching_assignments ta
		JOIN cohorts co ON co.id = ta.cohort_id
		JOIN teaching_groups tg ON tg.id = ta.teaching_group_id
		LEFT JOIN classes c ON c.id = tg.admin_class_id
		WHERE ta.teacher_id IN (`+inClause+`) AND ta.status = 'ended' AND ta.ended_at IS NOT NULL
		UNION ALL
		SELECT ht.teacher_id, ht.cohort_id, co.enrollment_year, ht.phase,
			c.name AS class_name, '' AS subject, ht.end_reason, ht.ended_at
		FROM class_head_teachers ht
		JOIN cohorts co ON co.id = ht.cohort_id
		JOIN classes c ON c.id = ht.class_id
		WHERE ht.teacher_id IN (`+inClause+`) AND ht.status = 'ended' AND ht.ended_at IS NOT NULL
	) ORDER BY ended_at DESC`, append(append([]any(nil), args...), args...)...)
	if err != nil {
		return nil, err
	}
	for _, row := range relRows.Values {
		tid := intValue(row[0])
		occ, ok := result[tid]
		if !ok || occ.LastRelease != nil {
			continue
		}
		endedAt, err := parseTime(row[7])
		if err != nil {
			continue
		}
		occ.LastRelease = &TeacherLastRelease{
			CohortID:       intValue(row[1]),
			EnrollmentYear: int(intValue(row[2])),
			Phase:          stringValue(row[3]),
			ClassName:      stringValue(row[4]),
			Subject:        stringValue(row[5]),
			EndReason:      stringValue(row[6]),
			EndedAt:        endedAt,
		}
		result[tid] = occ
	}

	for tid, occ := range result {
		set := cohortSets[tid]
		for cid := range set {
			occ.ActiveCohorts = append(occ.ActiveCohorts, cid)
		}
		sort.Slice(occ.ActiveCohorts, func(i, j int) bool { return occ.ActiveCohorts[i] < occ.ActiveCohorts[j] })
		if accountStatus[tid] == TeacherStatusInactive {
			occ.Status = TeacherStatusInactive
		} else {
			switch len(occ.ActiveCohorts) {
			case 0:
				occ.Status = TeacherStatusIdle
			case 1:
				occ.Status = TeacherStatusActiveSingle
			default:
				occ.Status = TeacherStatusActiveMulti
			}
		}
		result[tid] = occ
	}
	return result, nil
}

// CreateTeacher 在全校师资库中新建一名教师。
func (s *Store) CreateTeacher(ctx context.Context, input ImportTeacher) (DataTeacher, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return DataTeacher{}, fmt.Errorf("教师姓名不能为空")
	}
	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `INSERT INTO teachers(
		teacher_no, name, primary_subject, status, note, source_sheet, source_row, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?, ?)`,
		strings.TrimSpace(input.TeacherNo), name, strings.TrimSpace(input.Subject),
		strings.TrimSpace(input.Note), input.SourceSheet, input.SourceRow, now, now)
	if err != nil {
		return DataTeacher{}, err
	}
	return DataTeacher{
		ID:        res.LastInsertID,
		TeacherNo: strings.TrimSpace(input.TeacherNo),
		Name:      name,
		Subject:   strings.TrimSpace(input.Subject),
		Status:    "active",
		Occupancy: TeacherStatusIdle,
	}, nil
}

// SetTeacherStatus 切换教师在职/停用状态（active 或 inactive）。
func (s *Store) SetTeacherStatus(ctx context.Context, teacherID int64, status string) error {
	if status != "active" && status != "inactive" {
		return fmt.Errorf("未知教师状态 %q", status)
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `UPDATE teachers SET status = ?, updated_at = ? WHERE id = ?`, status, now, teacherID)
	return err
}

// UpdateTeacher 更新教师工号、姓名、主教学科与备注等基本信息。
func (s *Store) UpdateTeacher(ctx context.Context, teacherID int64, input ImportTeacher) (DataTeacher, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return DataTeacher{}, fmt.Errorf("教师姓名不能为空")
	}
	teacherNo := strings.TrimSpace(input.TeacherNo)
	subject := strings.TrimSpace(input.Subject)
	note := strings.TrimSpace(input.Note)
	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `UPDATE teachers SET teacher_no = ?, name = ?, primary_subject = ?, note = ?, updated_at = ?
		WHERE id = ?`, teacherNo, name, subject, note, now, teacherID)
	if err != nil {
		return DataTeacher{}, err
	}
	if res.Changes == 0 {
		return DataTeacher{}, ErrNotFound
	}
	prof, err := s.GetTeacherProfile(ctx, teacherID)
	if err != nil {
		return DataTeacher{}, err
	}
	return prof.Teacher, nil
}

// AssignTeacherToGroup 为指定教师分配教学班任课关系，若跨年级在任则返回非阻断提示。
func (s *Store) AssignTeacherToGroup(ctx context.Context, teacherID, teachingGroupID int64, role string) (*CrossGradeWarning, error) {
	if role == "" {
		role = "primary"
	}
	gRows, err := s.db.Query(ctx, `SELECT cohort_id, phase FROM teaching_groups WHERE id = ?`, teachingGroupID)
	if err != nil {
		return nil, err
	}
	if len(gRows.Values) == 0 {
		return nil, ErrNotFound
	}
	cohortID := intValue(gRows.Values[0][0])
	phase := stringValue(gRows.Values[0][1])

	warning, err := s.DetectCrossGradeWarnings(ctx, teacherID, cohortID)
	if err != nil {
		return nil, err
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.Execute(ctx, `INSERT INTO teaching_assignments(
		teacher_id, cohort_id, phase, teaching_group_id, role, status, started_at)
		VALUES (?, ?, ?, ?, ?, 'active', ?)
		ON CONFLICT(teacher_id, teaching_group_id, role) WHERE status = 'active' DO NOTHING`,
		teacherID, cohortID, phase, teachingGroupID, role, now)
	if err != nil {
		return nil, err
	}
	return warning, nil
}

// EndTeachingAssignment 结束一条教学班任课关系（例如 mid_term_change 或 manual_release）。
func (s *Store) EndTeachingAssignment(ctx context.Context, assignmentID int64, reason string) error {
	if reason == "" {
		reason = "manual_release"
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `UPDATE teaching_assignments SET status = 'ended', ended_at = ?, end_reason = ?
		WHERE id = ? AND status = 'active'`, now, reason, assignmentID)
	return err
}

// SetClassHeadTeacher 为指定班级设置当前活跃班主任（自动结束原班主任任期）。
func (s *Store) SetClassHeadTeacher(ctx context.Context, classID, teacherID int64) (*CrossGradeWarning, error) {
	cRows, err := s.db.Query(ctx, `SELECT cohort_id, phase FROM classes WHERE id = ?`, classID)
	if err != nil {
		return nil, err
	}
	if len(cRows.Values) == 0 {
		return nil, ErrNotFound
	}
	cohortID := intValue(cRows.Values[0][0])
	phase := stringValue(cRows.Values[0][1])
	warning, err := s.DetectCrossGradeWarnings(ctx, teacherID, cohortID)
	if err != nil {
		return nil, err
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.Transaction(ctx, []sqlite.Statement{
		{
			SQL: `UPDATE class_head_teachers SET status = 'ended', ended_at = ?, end_reason = 'replaced'
				WHERE class_id = ? AND status = 'active'`,
			Args: []any{now, classID},
		},
		{
			SQL: `INSERT INTO class_head_teachers(class_id, teacher_id, cohort_id, phase, status, started_at)
				VALUES (?, ?, ?, ?, 'active', ?)`,
			Args: []any{classID, teacherID, cohortID, phase, now},
		},
	})
	if err != nil {
		return nil, err
	}
	return warning, nil
}

// ListClasses 返回一个级在某个阶段的行政班与人数、班主任。
func (s *Store) ListClasses(ctx context.Context, cohortID, stageID int64) ([]SchoolClass, error) {
	cohort, phase, err := s.resolveCohortPhaseScope(ctx, cohortID, stageID)
	if err != nil {
		return nil, err
	}
	yearStart := cohort.EnrollmentYear + PhaseIndex(phase) - 1
	rows, err := s.db.Query(ctx, `SELECT c.id, c.cohort_id, c.name, c.continued_from_class_id,
		c.track_hint, c.room, c.status,
		(SELECT COUNT(*) FROM students st WHERE st.current_class_id = c.id),
		COALESCE((SELECT t.name FROM class_head_teachers ht
			JOIN teachers t ON t.id = ht.teacher_id
			WHERE ht.class_id = c.id AND ht.status = 'active' LIMIT 1), ''),
		COALESCE((SELECT ht.teacher_id FROM class_head_teachers ht
			WHERE ht.class_id = c.id AND ht.status = 'active' LIMIT 1), 0)
		FROM classes c
		WHERE c.cohort_id = ? AND c.phase = ?
		ORDER BY c.name`, cohort.ID, phase)
	if err != nil {
		return nil, err
	}
	resolvedStageID := EncodeStageID(cohort.ID, phase)
	classes := make([]SchoolClass, 0, len(rows.Values))
	for _, row := range rows.Values {
		var continuedFrom *int64
		if cid, ok := nullableInt64(row[3]); ok {
			continuedFrom = &cid
		}
		studentCount := int(intValue(row[7]))
		if studentCount == 0 {
			// 历史阶段班级归档后 students.current_class_id 已升至新阶段，按考试快照补充人数展示。
			if snapCnt, qErr := s.countQuery(ctx, `SELECT COUNT(DISTINCT ep.student_id)
				FROM exam_participants ep WHERE ep.class_id = ?`, intValue(row[0])); qErr == nil && snapCnt > 0 {
				studentCount = snapCnt
			}
		}
		classes = append(classes, SchoolClass{
			ID:                   intValue(row[0]),
			CohortID:             intValue(row[1]),
			Name:                 stringValue(row[2]),
			StageID:              resolvedStageID,
			Stage:                phase,
			ContinuedFromClassID: continuedFrom,
			TrackHint:            stringValue(row[4]),
			Room:                 stringValue(row[5]),
			Status:               stringValue(row[6]),
			AcademicYear:         yearStart,
			Students:             studentCount,
			HeadTeacher:          stringValue(row[8]),
			HeadTeacherID:        intValue(row[9]),
		})
	}
	return classes, nil
}

// CreateClass 在指定级和阶段下手动新建行政班，并可选建立默认常规教学班。
func (s *Store) CreateClass(ctx context.Context, cohortID int64, phase, name, trackHint, room string, continuedFromClassID *int64) (SchoolClass, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return SchoolClass{}, err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return SchoolClass{}, ErrCohortGraduated
	}
	name = normalizeClassName(name)
	if name == "" {
		return SchoolClass{}, fmt.Errorf("班级名称不能为空")
	}
	if !ValidSchoolPhase(phase) {
		return SchoolClass{}, fmt.Errorf("无效阶段 %q", phase)
	}
	now := s.now().Format(time.RFC3339Nano)
	var contArg any
	if continuedFromClassID != nil && *continuedFromClassID > 0 {
		contArg = *continuedFromClassID
	}
	res, err := s.db.Execute(ctx, `INSERT INTO classes(
		cohort_id, phase, name, continued_from_class_id, track_hint, room, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'active', ?)`,
		cohortID, phase, name, contArg, strings.TrimSpace(trackHint), strings.TrimSpace(room), now)
	if err != nil {
		return SchoolClass{}, err
	}
	return SchoolClass{
		ID:                   res.LastInsertID,
		CohortID:             cohortID,
		Name:                 name,
		StageID:              EncodeStageID(cohortID, phase),
		Stage:                phase,
		ContinuedFromClassID: continuedFromClassID,
		TrackHint:            strings.TrimSpace(trackHint),
		Room:                 strings.TrimSpace(room),
		Status:               "active",
		AcademicYear:         cohort.EnrollmentYear + PhaseIndex(phase) - 1,
	}, nil
}

// DeleteClass 删除尚未关联任何学生、考试快照或衍生外键引用的空班级。
func (s *Store) DeleteClass(ctx context.Context, classID int64) error {
	for _, check := range []struct {
		query string
		label string
	}{
		{`SELECT COUNT(*) FROM students WHERE current_class_id = ?`, "学生"},
		{`SELECT COUNT(*) FROM exam_participants WHERE class_id = ?`, "考试快照"},
		{`SELECT COUNT(*) FROM classes WHERE continued_from_class_id = ?`, "后续阶段延续班级"},
		{`SELECT COUNT(*) FROM teaching_groups WHERE admin_class_id = ?`, "常规教学班"},
		{`SELECT COUNT(*) FROM assessment_stat_groups WHERE lead_class_id = ?`, "考核统计组"},
	} {
		cnt, err := s.countQuery(ctx, check.query, classID)
		if err != nil {
			return err
		}
		if cnt > 0 {
			return fmt.Errorf("%w: 仍存在关联的%s", ErrClassNotEmpty, check.label)
		}
	}
	_, err := s.db.Execute(ctx, `DELETE FROM classes WHERE id = ?`, classID)
	return err
}

// ListTeachers 返回指定阶段（当 stageID > 0）或全校的教师及任课统计。
func (s *Store) ListTeachers(ctx context.Context, stageID int64, query string) ([]DataTeacher, error) {
	cohortID, phase, hasStage := DecodeStageID(stageID)
	where := []string{"1=1"}
	var args []any
	if hasStage {
		where = append(where, `EXISTS (SELECT 1 FROM teaching_assignments ta
			WHERE ta.teacher_id = t.id AND ta.cohort_id = ? AND ta.phase = ?)`)
		args = append(args, cohortID, phase)
	}
	if text := strings.TrimSpace(query); text != "" {
		where = append(where, "(t.teacher_no LIKE ? OR t.name LIKE ? OR t.primary_subject LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}
	whereSQL := strings.Join(where, " AND ")

	var groupSubquery, assignSubquery string
	var subArgs []any
	if hasStage {
		groupSubquery = `(SELECT COUNT(DISTINCT ta.teaching_group_id) FROM teaching_assignments ta
			WHERE ta.teacher_id = t.id AND ta.cohort_id = ? AND ta.phase = ?)`
		assignSubquery = `(SELECT COUNT(*) FROM teaching_assignments ta
			WHERE ta.teacher_id = t.id AND ta.cohort_id = ? AND ta.phase = ?)`
		subArgs = append(subArgs, cohortID, phase, cohortID, phase)
	} else {
		groupSubquery = `(SELECT COUNT(DISTINCT ta.teaching_group_id) FROM teaching_assignments ta
			WHERE ta.teacher_id = t.id AND ta.status = 'active')`
		assignSubquery = `(SELECT COUNT(*) FROM teaching_assignments ta
			WHERE ta.teacher_id = t.id AND ta.status = 'active')`
	}
	allArgs := append(subArgs, args...)
	rows, err := s.db.Query(ctx, `SELECT t.id, t.teacher_no, t.name, t.primary_subject, t.status, `+
		groupSubquery+`, `+assignSubquery+`, COALESCE(t.import_run_id, 0), t.source_sheet, t.source_row
		FROM teachers t WHERE `+whereSQL+` ORDER BY t.name, t.teacher_no`, allArgs...)
	if err != nil {
		return nil, err
	}
	teachers := make([]DataTeacher, 0, len(rows.Values))
	ids := make([]int64, 0, len(rows.Values))
	for _, row := range rows.Values {
		id := intValue(row[0])
		ids = append(ids, id)
		teachers = append(teachers, DataTeacher{
			ID:          id,
			TeacherNo:   stringValue(row[1]),
			Name:        stringValue(row[2]),
			Subject:     stringValue(row[3]),
			Status:      stringValue(row[4]),
			Groups:      int(intValue(row[5])),
			Assignments: int(intValue(row[6])),
			ImportRunID: intValue(row[7]),
			SourceSheet: stringValue(row[8]),
			SourceRow:   int(intValue(row[9])),
		})
	}
	if len(ids) > 0 {
		occMap, err := s.teacherOccupancyByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range teachers {
			if occ, ok := occMap[teachers[i].ID]; ok {
				teachers[i].Occupancy = occ.Status
				teachers[i].ActiveSummary = formatOccupancySummary(occ)
			}
		}
	}
	return teachers, nil
}

func formatOccupancySummary(occ TeacherOccupancy) string {
	if occ.Status == TeacherStatusInactive {
		return "已停用"
	}
	if len(occ.ActiveAssignments) == 0 {
		if occ.LastRelease != nil {
			return fmt.Sprintf("最近带过 %s%s", formatCohortLabel(occ.LastRelease.EnrollmentYear), occ.LastRelease.ClassName)
		}
		return "空闲待命"
	}
	parts := make([]string, 0, len(occ.ActiveAssignments))
	for _, item := range occ.ActiveAssignments {
		if item.Role == "head_teacher" {
			parts = append(parts, fmt.Sprintf("%s %s班主任", formatCohortLabel(item.EnrollmentYear), item.ClassName))
		} else {
			parts = append(parts, fmt.Sprintf("%s %s", formatCohortLabel(item.EnrollmentYear), item.TeachingGroupName))
		}
	}
	return strings.Join(parts, "、")
}

// ListTeachingGroups 返回阶段下的教学班与任课教师、学生数量。
func (s *Store) ListTeachingGroups(ctx context.Context, stageID int64, query string) ([]DataTeachingGroup, error) {
	cohort, phase, err := s.resolveCohortPhaseScope(ctx, 0, stageID)
	if err != nil {
		return nil, err
	}
	where := " WHERE g.cohort_id = ? AND g.phase = ?"
	args := []any{cohort.ID, phase}
	if text := strings.TrimSpace(query); text != "" {
		where += ` AND (g.name LIKE ? OR g.subject LIKE ? OR EXISTS (
			SELECT 1 FROM teaching_assignments ta JOIN teachers t ON t.id = ta.teacher_id
			WHERE ta.teaching_group_id = g.id AND t.name LIKE ?))`
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}
	rows, err := s.db.Query(ctx, `SELECT g.id, g.cohort_id, g.phase, g.subject, g.name, g.kind, COALESCE(g.admin_class_id, 0), g.status,
		CASE
			WHEN g.kind = 'walking' THEN (SELECT COUNT(*) FROM teaching_group_students tgs WHERE tgs.teaching_group_id = g.id AND tgs.ended_at IS NULL)
			ELSE (SELECT COUNT(*) FROM students st WHERE st.current_class_id = g.admin_class_id)
		END AS student_cnt,
		COALESCE((SELECT GROUP_CONCAT(t.name, '、') FROM teaching_assignments ta
			JOIN teachers t ON t.id = ta.teacher_id
			WHERE ta.teaching_group_id = g.id AND (ta.status = 'active' OR g.status <> 'active')), ''),
		CASE
			WHEN g.kind = 'walking' THEN COALESCE((SELECT GROUP_CONCAT(DISTINCT c.name) FROM teaching_group_students tgs
				JOIN students st ON st.id = tgs.student_id
				JOIN classes c ON c.id = st.current_class_id
				WHERE tgs.teaching_group_id = g.id AND tgs.ended_at IS NULL), '')
			ELSE COALESCE((SELECT c.name FROM classes c WHERE c.id = g.admin_class_id), '')
		END AS class_names,
		COALESCE(g.import_run_id, 0), g.source_sheet, g.source_row
		FROM teaching_groups g`+where+` ORDER BY g.subject, g.name`, args...)
	if err != nil {
		return nil, err
	}
	groups := make([]DataTeachingGroup, 0, len(rows.Values))
	resolvedStageID := EncodeStageID(cohort.ID, phase)
	for _, row := range rows.Values {
		groups = append(groups, DataTeachingGroup{
			ID:           intValue(row[0]),
			CohortID:     intValue(row[1]),
			StageID:      resolvedStageID,
			Phase:        stringValue(row[2]),
			Subject:      stringValue(row[3]),
			Name:         stringValue(row[4]),
			Kind:         stringValue(row[5]),
			AdminClassID: intValue(row[6]),
			Status:       stringValue(row[7]),
			Students:     int(intValue(row[8])),
			Teachers:     stringValue(row[9]),
			ClassNames:   stringValue(row[10]),
			ImportRunID:  intValue(row[11]),
			SourceSheet:  stringValue(row[12]),
			SourceRow:    int(intValue(row[13])),
		})
	}
	return groups, nil
}

// CreateTeachingGroup 在指定级和阶段下新建教学班（admin 或 walking）。
func (s *Store) CreateTeachingGroup(ctx context.Context, cohortID int64, phase, subject, name, kind string, adminClassID int64) (DataTeachingGroup, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return DataTeachingGroup{}, err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return DataTeachingGroup{}, ErrCohortGraduated
	}
	if !ValidSchoolPhase(phase) {
		return DataTeachingGroup{}, fmt.Errorf("无效阶段 %q", phase)
	}
	subject = strings.TrimSpace(subject)
	name = strings.TrimSpace(name)
	if subject == "" || name == "" {
		return DataTeachingGroup{}, fmt.Errorf("教学班学科和名称不能为空")
	}
	if kind == "" {
		kind = "admin"
	}
	if kind != "admin" && kind != "walking" {
		return DataTeachingGroup{}, fmt.Errorf("无效教学班类型 %q", kind)
	}
	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `INSERT INTO teaching_groups(
		cohort_id, phase, subject, name, kind, admin_class_id, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'active', ?)`,
		cohortID, phase, subject, name, kind, nullableID(adminClassID), now)
	if err != nil {
		return DataTeachingGroup{}, err
	}
	return DataTeachingGroup{
		ID:           res.LastInsertID,
		CohortID:     cohortID,
		StageID:      EncodeStageID(cohortID, phase),
		Phase:        phase,
		Subject:      subject,
		Name:         name,
		Kind:         kind,
		AdminClassID: adminClassID,
		Status:       "active",
	}, nil
}

// AssignWalkingStudent 把学生加入走班教学班，同届同阶段同学科至多归属 1 个走班（应用层显式约束并闭合旧归属）。
func (s *Store) AssignWalkingStudent(ctx context.Context, teachingGroupID, studentID int64) error {
	gRows, err := s.db.Query(ctx, `SELECT cohort_id, phase, subject, kind FROM teaching_groups WHERE id = ?`, teachingGroupID)
	if err != nil {
		return err
	}
	if len(gRows.Values) == 0 {
		return ErrNotFound
	}
	cohortID := intValue(gRows.Values[0][0])
	phase := stringValue(gRows.Values[0][1])
	subject := stringValue(gRows.Values[0][2])
	kind := stringValue(gRows.Values[0][3])
	if kind != "walking" {
		return fmt.Errorf("仅走班教学班可显式写入 teaching_group_students")
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.Transaction(ctx, []sqlite.Statement{
		{
			SQL: `UPDATE teaching_group_students SET ended_at = ?
				WHERE student_id = ? AND cohort_id = ? AND phase = ? AND subject = ? AND ended_at IS NULL`,
			Args: []any{now, studentID, cohortID, phase, subject},
		},
		{
			SQL: `INSERT INTO teaching_group_students(teaching_group_id, student_id, cohort_id, phase, subject, started_at, ended_at)
				VALUES (?, ?, ?, ?, ?, ?, NULL)
				ON CONFLICT(teaching_group_id, student_id) DO UPDATE SET ended_at = NULL, started_at = excluded.started_at`,
			Args: []any{teachingGroupID, studentID, cohortID, phase, subject, now},
		},
	})
	return err
}

// ListCohortAssignments 返回指定年级与阶段下活跃的任课关系明细。
func (s *Store) ListCohortAssignments(ctx context.Context, cohortID int64, phase string) ([]TeachingAssignmentRow, error) {
	cohort, ph, err := s.resolveCohortPhaseScope(ctx, cohortID, EncodeStageID(cohortID, phase))
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT ta.id, ta.cohort_id, ta.phase, tg.id, tg.name, tg.kind,
		COALESCE(tg.admin_class_id, 0), COALESCE(c.name, tg.name), tg.subject,
		t.id, t.teacher_no, t.name, ta.role, ta.status
		FROM teaching_assignments ta
		JOIN teaching_groups tg ON tg.id = ta.teaching_group_id
		JOIN teachers t ON t.id = ta.teacher_id
		LEFT JOIN classes c ON c.id = tg.admin_class_id
		WHERE ta.cohort_id = ? AND ta.phase = ? AND ta.status = 'active'
		ORDER BY tg.kind, c.name, tg.subject, tg.name, t.name`, cohort.ID, ph)
	if err != nil {
		return nil, err
	}
	out := make([]TeachingAssignmentRow, 0, len(rows.Values))
	for _, r := range rows.Values {
		out = append(out, TeachingAssignmentRow{
			ID:                intValue(r[0]),
			CohortID:          intValue(r[1]),
			Phase:             stringValue(r[2]),
			TeachingGroupID:   intValue(r[3]),
			TeachingGroupName: stringValue(r[4]),
			GroupKind:         stringValue(r[5]),
			AdminClassID:      intValue(r[6]),
			ClassName:         stringValue(r[7]),
			Subject:           stringValue(r[8]),
			TeacherID:         intValue(r[9]),
			TeacherNo:         stringValue(r[10]),
			TeacherName:       stringValue(r[11]),
			Role:              stringValue(r[12]),
			Status:            stringValue(r[13]),
		})
	}
	return out, nil
}

// ListTeachingGroupStudents 返回走班教学班当前活跃在读学生列表。
func (s *Store) ListTeachingGroupStudents(ctx context.Context, teachingGroupID int64) ([]DataStudent, error) {
	rows, err := s.db.Query(ctx, `SELECT st.id, st.student_no, st.candidate_code, st.name, st.gender,
		st.cohort_id, COALESCE(st.current_class_id, 0), COALESCE(c.name, ''),
		st.current_track, st.elective_combo, st.foreign_lang, st.status,
		st.entry_total_score, st.previous_student_id,
		COALESCE(st.import_run_id, 0), st.source_sheet, st.source_row
		FROM teaching_group_students tgs
		JOIN students st ON st.id = tgs.student_id
		LEFT JOIN classes c ON c.id = st.current_class_id
		WHERE tgs.teaching_group_id = ? AND tgs.ended_at IS NULL
		ORDER BY st.student_no, st.id`, teachingGroupID)
	if err != nil {
		return nil, err
	}
	out := make([]DataStudent, 0, len(rows.Values))
	for _, r := range rows.Values {
		out = append(out, dataStudentFromRow(r, 0))
	}
	return out, nil
}

// RemoveWalkingStudent 把学生从走班教学班移出（闭合当前走班映射）。
func (s *Store) RemoveWalkingStudent(ctx context.Context, teachingGroupID, studentID int64) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `UPDATE teaching_group_students SET ended_at = ?
		WHERE teaching_group_id = ? AND student_id = ? AND ended_at IS NULL`,
		now, teachingGroupID, studentID)
	return err
}

func normalizeClassName(name string) string {
	return strings.TrimSpace(name)
}

func normalizeTrackValue(track string) string {
	switch strings.ToLower(strings.TrimSpace(track)) {
	case "physics", "物理", "物理类":
		return "physics"
	case "history", "历史", "历史类":
		return "history"
	case "common", "general", "通用", "全部", "all":
		return "common"
	default:
		return ""
	}
}
