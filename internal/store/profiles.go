package store

import (
	"context"
	"encoding/json"
	"sort"
)

// GetStudentProfile 返回阶段 2 使用的学生完整三年档案（含中考入学成绩、变动事件、往届复读前溯与历次考试表现）。
func (s *Store) GetStudentProfile(ctx context.Context, studentID int64) (StudentProfile, error) {
	st, err := s.GetStudent(ctx, 0, studentID)
	if err != nil {
		return StudentProfile{}, err
	}
	cohort, err := s.GetCohort(ctx, st.CohortID)
	if err != nil {
		return StudentProfile{}, err
	}
	profile := StudentProfile{
		Student: st,
		Cohort:  cohort,
	}

	if st.PreviousStudentID != nil && *st.PreviousStudentID > 0 {
		if prevSt, pErr := s.GetStudent(ctx, 0, *st.PreviousStudentID); pErr == nil {
			profile.PreviousStudent = &prevSt
			if prevCoh, cErr := s.GetCohort(ctx, prevSt.CohortID); cErr == nil {
				profile.PreviousCohort = &prevCoh
			}
		}
	}

	// 入学基数各科成绩。
	esRows, err := s.db.Query(ctx, `SELECT subject, score, COALESCE(import_run_id, 0), source_sheet, source_row
		FROM student_entry_scores WHERE student_id = ? ORDER BY subject`, studentID)
	if err != nil {
		return StudentProfile{}, err
	}
	for _, r := range esRows.Values {
		profile.EntryScores = append(profile.EntryScores, StudentEntryScore{
			Subject:     stringValue(r[0]),
			Score:       floatValue(r[1]),
			ImportRunID: intValue(r[2]),
			SourceSheet: stringValue(r[3]),
			SourceRow:   int(intValue(r[4])),
		})
	}

	// 学籍与变动时间线。
	evRows, err := s.db.Query(ctx, `SELECT id, student_id, cohort_id, phase, event_type,
		from_value_json, to_value_json, exam_id, note, occurred_at
		FROM student_events WHERE student_id = ? ORDER BY occurred_at ASC, id ASC`, studentID)
	if err != nil {
		return StudentProfile{}, err
	}
	for _, r := range evRows.Values {
		t, err := parseTime(r[9])
		if err != nil {
			return StudentProfile{}, err
		}
		var exID *int64
		if v, ok := nullableInt64(r[7]); ok {
			exID = &v
		}
		profile.Events = append(profile.Events, StudentEvent{
			ID:            intValue(r[0]),
			StudentID:     intValue(r[1]),
			CohortID:      intValue(r[2]),
			Phase:         stringValue(r[3]),
			EventType:     stringValue(r[4]),
			FromValueJSON: stringValue(r[5]),
			ToValueJSON:   stringValue(r[6]),
			ExamID:        exID,
			Note:          stringValue(r[8]),
			OccurredAt:    t,
		})
	}

	track, err := s.QueryStudentTracking(ctx, st.CohortID, studentID, st.StudentNo)
	if err != nil {
		return StudentProfile{}, err
	}
	profile.Exams = track.Points
	return profile, nil
}

// GetTeacherProfile 返回阶段 2 使用的教师完整档案（实时全校占用、跨届个人定量得分轨迹与按届分组的带班历史）。
func (s *Store) GetTeacherProfile(ctx context.Context, teacherID int64) (TeacherProfile, error) {
	rows, err := s.db.Query(ctx, `SELECT id, teacher_no, name, primary_subject, status, note,
		COALESCE(import_run_id, 0), source_sheet, source_row
		FROM teachers WHERE id = ?`, teacherID)
	if err != nil {
		return TeacherProfile{}, err
	}
	if len(rows.Values) == 0 {
		return TeacherProfile{}, ErrNotFound
	}
	r := rows.Values[0]
	occMap, err := s.teacherOccupancyByIDs(ctx, []int64{teacherID})
	if err != nil {
		return TeacherProfile{}, err
	}
	occ := occMap[teacherID]
	dt := DataTeacher{
		ID:            teacherID,
		TeacherNo:     stringValue(r[1]),
		Name:          stringValue(r[2]),
		Subject:       stringValue(r[3]),
		Status:        stringValue(r[4]),
		Note:          stringValue(r[5]),
		Occupancy:     occ.Status,
		ActiveSummary: formatOccupancySummary(occ),
		ImportRunID:   intValue(r[6]),
		SourceSheet:   stringValue(r[7]),
		SourceRow:     int(intValue(r[8])),
	}

	// 跨届个人量化得分轨迹。
	psRows, err := s.db.Query(ctx, `SELECT e.id, e.name, e.exam_date, e.academic_year_start, e.term, e.exam_type,
		ec.cohort_id, co.enrollment_year, co.graduation_year, co.current_phase,
		ps.phase_at_exam, a.id, ps.subject, ps.score, ps.group_points_json, rr.id
		FROM personal_scores ps
		JOIN result_runs rr ON rr.id = ps.run_id AND rr.is_official = 1
		JOIN assessments a ON a.id = rr.assessment_id AND a.current_run_id = rr.id
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		JOIN cohorts co ON co.id = ps.cohort_id
		WHERE ps.teacher_id = ?
		ORDER BY e.academic_year_start ASC, e.exam_date ASC, e.sequence ASC, e.id ASC, ps.subject`, teacherID)
	if err != nil {
		return TeacherProfile{}, err
	}

	var crossScores []TeacherTrendPoint
	cohortMap := map[int64]*TeacherCohortHistory{}
	runRowsCache := map[int64][]StoredResultRow{}
	ensureCohortHistory := func(cid int64, enroll, grad int, curPhase string) *TeacherCohortHistory {
		if h, ok := cohortMap[cid]; ok {
			return h
		}
		h := &TeacherCohortHistory{
			CohortID:       cid,
			EnrollmentYear: enroll,
			GraduationYear: grad,
			CurrentPhase:   curPhase,
		}
		cohortMap[cid] = h
		return h
	}

	for _, pr := range psRows.Values {
		examID := intValue(pr[0])
		examName := stringValue(pr[1])
		examDate := stringValue(pr[2])
		yearStart := int(intValue(pr[3]))
		term := stringValue(pr[4])
		examType := stringValue(pr[5])
		cid := intValue(pr[6])
		enroll := int(intValue(pr[7]))
		grad := int(intValue(pr[8]))
		curPhase := stringValue(pr[9])
		phaseAtExam := stringValue(pr[10])
		assessID := intValue(pr[11])
		subj := stringValue(pr[12])
		score := floatValue(pr[13])
		runID := intValue(pr[15])
		var gp []float64
		_ = json.Unmarshal([]byte(stringValue(pr[14])), &gp)

		allRunRows, cached := runRowsCache[runID]
		if !cached && runID > 0 {
			loadedRows, _, rErr := s.assessmentRunRows(ctx, runID)
			if rErr != nil {
				return TeacherProfile{}, rErr
			}
			allRunRows = loadedRows
			runRowsCache[runID] = allRunRows
		}
		var classRows []StoredResultRow
		for _, row := range allRunRows {
			if row.Subject != subj {
				continue
			}
			for _, tid := range row.TeacherIDs {
				if tid == teacherID {
					classRows = append(classRows, row)
					break
				}
			}
		}

		crossScores = append(crossScores, TeacherTrendPoint{
			ExamID:            examID,
			AssessmentID:      assessID,
			ExamName:          examName,
			ExamDate:          examDate,
			AcademicYearStart: yearStart,
			Term:              term,
			ExamType:          examType,
			GradeLevel:        phaseAtExam,
			Subject:           subj,
			PersonalScore:     score,
			GroupPoints:       gp,
		})

		ch := ensureCohortHistory(cid, enroll, grad, curPhase)
		ch.Exams = append(ch.Exams, TeacherExamHistory{
			ExamID:            examID,
			AssessmentID:      assessID,
			ExamName:          examName,
			ExamType:          examType,
			AcademicYearStart: yearStart,
			Term:              term,
			ExamDate:          examDate,
			CohortID:          cid,
			EnrollmentYear:    enroll,
			PhaseAtExam:       phaseAtExam,
			Subject:           subj,
			Score:             score,
			GroupPoints:       gp,
			ClassRows:         classRows,
		})
	}

	// 补充教师历史上所有届别（含尚未出分的活跃/已结束任课关系与班主任履历）。
	histRows, err := s.db.Query(ctx, `SELECT ta.cohort_id, co.enrollment_year, co.graduation_year, co.current_phase,
		ta.phase, COALESCE(c.id, 0), COALESCE(c.name, tg.name), tg.id, tg.name, tg.subject, ta.role,
		ta.status, ta.end_reason, ta.started_at, COALESCE(ta.ended_at, '')
		FROM teaching_assignments ta
		JOIN cohorts co ON co.id = ta.cohort_id
		JOIN teaching_groups tg ON tg.id = ta.teaching_group_id
		LEFT JOIN classes c ON c.id = tg.admin_class_id
		WHERE ta.teacher_id = ?
		ORDER BY co.enrollment_year DESC, ta.phase, tg.subject, tg.name`, teacherID)
	if err != nil {
		return TeacherProfile{}, err
	}
	for _, hr := range histRows.Values {
		ch := ensureCohortHistory(intValue(hr[0]), int(intValue(hr[1])), int(intValue(hr[2])), stringValue(hr[3]))
		ch.Assignments = append(ch.Assignments, TeacherActiveAssignment{
			CohortID:          ch.CohortID,
			EnrollmentYear:    ch.EnrollmentYear,
			GraduationYear:    ch.GraduationYear,
			Phase:             stringValue(hr[4]),
			ClassID:           intValue(hr[5]),
			ClassName:         stringValue(hr[6]),
			TeachingGroupID:   intValue(hr[7]),
			TeachingGroupName: stringValue(hr[8]),
			Subject:           stringValue(hr[9]),
			Role:              stringValue(hr[10]),
			Status:            stringValue(hr[11]),
			EndReason:         stringValue(hr[12]),
			StartedAt:         stringValue(hr[13]),
			EndedAt:           stringValue(hr[14]),
		})
	}

	htRows, err := s.db.Query(ctx, `SELECT ht.cohort_id, co.enrollment_year, co.graduation_year, co.current_phase,
		ht.phase, c.id, c.name, ht.status, ht.end_reason, ht.started_at, COALESCE(ht.ended_at, '')
		FROM class_head_teachers ht
		JOIN cohorts co ON co.id = ht.cohort_id
		JOIN classes c ON c.id = ht.class_id
		WHERE ht.teacher_id = ?
		ORDER BY co.enrollment_year DESC, ht.phase, c.name`, teacherID)
	if err != nil {
		return TeacherProfile{}, err
	}
	for _, hr := range htRows.Values {
		ch := ensureCohortHistory(intValue(hr[0]), int(intValue(hr[1])), int(intValue(hr[2])), stringValue(hr[3]))
		ch.Assignments = append(ch.Assignments, TeacherActiveAssignment{
			CohortID:       ch.CohortID,
			EnrollmentYear: ch.EnrollmentYear,
			GraduationYear: ch.GraduationYear,
			Phase:          stringValue(hr[4]),
			ClassID:        intValue(hr[5]),
			ClassName:      stringValue(hr[6]),
			Role:           "head_teacher",
			Status:         stringValue(hr[7]),
			EndReason:      stringValue(hr[8]),
			StartedAt:      stringValue(hr[9]),
			EndedAt:        stringValue(hr[10]),
		})
	}

	histories := make([]TeacherCohortHistory, 0, len(cohortMap))
	for _, h := range cohortMap {
		histories = append(histories, *h)
	}
	sort.Slice(histories, func(i, j int) bool {
		return histories[i].EnrollmentYear > histories[j].EnrollmentYear
	})

	return TeacherProfile{
		Teacher:           dt,
		Occupancy:         occ,
		CrossCohortScores: crossScores,
		CohortHistories:   histories,
	}, nil
}

// QueryClassTrendChain 沿 classes.continued_from_class_id 串联班级高一至高三传承链，并返回整条传承链的历次考核成绩轨迹。
func (s *Store) QueryClassTrendChain(ctx context.Context, classID int64) ([]ClassChainNode, ClassTrendResult, error) {
	rows, err := s.db.Query(ctx, `SELECT id, cohort_id, phase, name, continued_from_class_id, status
		FROM classes WHERE cohort_id = (SELECT cohort_id FROM classes WHERE id = ?)`, classID)
	if err != nil {
		return nil, ClassTrendResult{}, err
	}
	if len(rows.Values) == 0 {
		return nil, ClassTrendResult{}, ErrNotFound
	}

	type clsRec struct {
		node ClassChainNode
	}
	byID := map[int64]ClassChainNode{}
	nextMap := map[int64]int64{}
	var cohortID int64
	for _, r := range rows.Values {
		cid := intValue(r[0])
		cohortID = intValue(r[1])
		var prev *int64
		if pv, ok := nullableInt64(r[4]); ok {
			prev = &pv
			nextMap[pv] = cid
		}
		htRows, _ := s.db.Query(ctx, `SELECT COALESCE(t.name, '') FROM class_head_teachers ht
			JOIN teachers t ON t.id = ht.teacher_id
			WHERE ht.class_id = ? ORDER BY ht.status = 'active' DESC, ht.id DESC LIMIT 1`, cid)
		htName := ""
		if len(htRows.Values) > 0 {
			htName = stringValue(htRows.Values[0][0])
		}
		sCnt, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM students WHERE current_class_id = ?`, cid)
		if sCnt == 0 {
			sCnt, _ = s.countQuery(ctx, `SELECT COUNT(DISTINCT student_id) FROM exam_participants WHERE class_id = ?`, cid)
		}
		byID[cid] = ClassChainNode{
			ClassID:              cid,
			CohortID:             cohortID,
			Phase:                stringValue(r[2]),
			Name:                 stringValue(r[3]),
			ContinuedFromClassID: prev,
			Status:               stringValue(r[5]),
			HeadTeacher:          htName,
			StudentCount:         sCnt,
		}
	}

	// 回溯至起点节点，再顺藤摸瓜走到最末节点。
	rootID := classID
	visited := map[int64]bool{rootID: true}
	for {
		cur, ok := byID[rootID]
		if !ok || cur.ContinuedFromClassID == nil || *cur.ContinuedFromClassID <= 0 || visited[*cur.ContinuedFromClassID] {
			break
		}
		rootID = *cur.ContinuedFromClassID
		visited[rootID] = true
	}

	var chain []ClassChainNode
	var chainClassIDs []int64
	curID := rootID
	seenForward := map[int64]bool{}
	for curID > 0 && !seenForward[curID] {
		seenForward[curID] = true
		node, ok := byID[curID]
		if !ok {
			break
		}
		chain = append(chain, node)
		chainClassIDs = append(chainClassIDs, node.ClassID)
		curID = nextMap[curID]
	}

	trend, err := s.queryClassTrendsByIDs(ctx, cohortID, chainClassIDs, chain[len(chain)-1].Name, nil, true)
	if err != nil {
		return nil, ClassTrendResult{}, err
	}
	return chain, trend, nil
}
