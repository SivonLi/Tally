package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/assessment"
	"tally/internal/calc"
	"tally/internal/school"
)

var inputItemOrder = []string{
	"roster", "scores", "cutoffs", "targets",
	"groups", "baseline", "assignments", "classes", "exclusions", "boundaries",
}

// FindAssessmentForExamCohort 只读查询一场考试中某个级的考核记录，不存在时返回 false 而不自动建档。
func (s *Store) FindAssessmentForExamCohort(ctx context.Context, examCohortID int64) (Assessment, bool, error) {
	existing, err := s.findAssessmentByExamCohort(ctx, examCohortID)
	if err == nil {
		return existing, true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return Assessment{}, false, nil
	}
	return Assessment{}, false, err
}

// EnableAssessmentForExamCohort 为月考等尚未开启量化考核的考试按需建档，并初始化默认方案版本与常规统计组。
func (s *Store) EnableAssessmentForExamCohort(ctx context.Context, examCohortID int64) (Assessment, error) {
	assess, err := s.AssessmentForExamCohort(ctx, examCohortID)
	if err != nil {
		return Assessment{}, err
	}
	if assess.PlanVersion == 0 {
		if defaultPlan, pErr := s.EnsureDefaultPlan(ctx); pErr == nil && defaultPlan.ID > 0 {
			if _, uErr := s.db.Execute(ctx, `UPDATE assessments SET plan_version_id = ? WHERE id = ? AND COALESCE(plan_version_id, 0) = 0`,
				defaultPlan.ID, assess.ID); uErr != nil {
				return Assessment{}, uErr
			}
			assess.PlanVersion = defaultPlan.ID
		}
	}
	if err := s.EnsureAssessmentStatGroups(ctx, assess.ID); err != nil {
		return Assessment{}, err
	}
	return assess, nil
}

// AssessmentForExamCohort 返回一场考试中某个级的考核记录，不存在时按需创建。
func (s *Store) AssessmentForExamCohort(ctx context.Context, examCohortID int64) (Assessment, error) {
	if existing, err := s.findAssessmentByExamCohort(ctx, examCohortID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Assessment{}, err
	}
	ecRows, err := s.db.Query(ctx, `SELECT id FROM exam_cohorts WHERE id = ?`, examCohortID)
	if err != nil {
		return Assessment{}, err
	}
	if len(ecRows.Values) == 0 {
		return Assessment{}, ErrNotFound
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := s.db.Execute(ctx, `INSERT INTO assessments(exam_cohort_id, status, revision, created_at)
		VALUES (?, 'draft', 1, ?) ON CONFLICT(exam_cohort_id) DO NOTHING`, examCohortID, now); err != nil {
		return Assessment{}, err
	}
	return s.findAssessmentByExamCohort(ctx, examCohortID)
}

func (s *Store) findAssessmentByExamCohort(ctx context.Context, examCohortID int64) (Assessment, error) {
	rows, err := s.db.Query(ctx, `SELECT id, exam_cohort_id, COALESCE(plan_version_id, 0),
		status, COALESCE(current_run_id, 0), revision, created_at,
		EXISTS(SELECT 1 FROM archived_assessments aa WHERE aa.assessment_id = assessments.id)
		FROM assessments WHERE exam_cohort_id = ?`, examCohortID)
	if err != nil {
		return Assessment{}, err
	}
	if len(rows.Values) == 0 {
		return Assessment{}, ErrNotFound
	}
	createdAt, err := parseTime(rows.Values[0][6])
	if err != nil {
		return Assessment{}, err
	}
	return Assessment{
		ID:           intValue(rows.Values[0][0]),
		ExamCohortID: intValue(rows.Values[0][1]),
		PlanVersion:  intValue(rows.Values[0][2]),
		Status:       stringValue(rows.Values[0][3]),
		CurrentRunID: intValue(rows.Values[0][4]),
		Revision:     int(intValue(rows.Values[0][5])),
		Archived:     intValue(rows.Values[0][7]) != 0,
		CreatedAt:    createdAt,
	}, nil
}

// LoadAssessmentContext 读取考核关联的考试、级与阶段展示信息。
func (s *Store) LoadAssessmentContext(ctx context.Context, assessmentID int64) (AssessmentContext, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id, a.exam_cohort_id, COALESCE(a.plan_version_id, 0),
		a.status, COALESCE(a.current_run_id, 0), a.revision, a.created_at,
		e.id, e.academic_year_start, e.term, e.exam_type, e.name, e.exam_date, e.sequence, e.status, e.created_at,
		ec.id, ec.cohort_id, ec.phase_at_exam,
		c.id, c.enrollment_year, c.graduation_year, c.current_phase, c.current_academic_year_start, c.graduated_at, c.created_at,
		EXISTS(SELECT 1 FROM archived_assessments aa WHERE aa.assessment_id = a.id)
		FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		JOIN cohorts c ON c.id = ec.cohort_id
		WHERE a.id = ?`, assessmentID)
	if err != nil {
		return AssessmentContext{}, err
	}
	if len(rows.Values) == 0 {
		return AssessmentContext{}, ErrNotFound
	}
	r := rows.Values[0]
	assessCreated, err := parseTime(r[6])
	if err != nil {
		return AssessmentContext{}, err
	}
	examCreated, err := parseTime(r[15])
	if err != nil {
		return AssessmentContext{}, err
	}
	cohortGrad, err := nullableTime(r[24])
	if err != nil {
		return AssessmentContext{}, err
	}
	cohortCreated, err := parseTime(r[25])
	if err != nil {
		return AssessmentContext{}, err
	}

	phaseAtExam := stringValue(r[18])
	cohortPhase := stringValue(r[22])
	cohortStatus := "active"
	if cohortPhase == PhaseGraduated {
		cohortStatus = PhaseGraduated
	}
	cohortID := intValue(r[19])
	stagePhase := phaseForStage(phaseAtExam)
	examYear := int(intValue(r[8]))

	ac := AssessmentContext{
		Assessment: Assessment{
			ID:           intValue(r[0]),
			ExamCohortID: intValue(r[1]),
			PlanVersion:  intValue(r[2]),
			Status:       stringValue(r[3]),
			CurrentRunID: intValue(r[4]),
			Revision:     int(intValue(r[5])),
			Archived:     intValue(r[26]) != 0,
			CreatedAt:    assessCreated,
		},
		Exam: Exam{
			ID:                intValue(r[7]),
			AcademicYearStart: examYear,
			Term:              stringValue(r[9]),
			ExamType:          stringValue(r[10]),
			Name:              stringValue(r[11]),
			ExamDate:          stringValue(r[12]),
			Sequence:          int(intValue(r[13])),
			Status:            stringValue(r[14]),
			CreatedAt:         examCreated,
		},
		ExamCohort: ExamCohort{
			ID:                intValue(r[16]),
			ExamID:            intValue(r[7]),
			CohortID:          intValue(r[17]),
			StageID:           EncodeStageID(cohortID, stagePhase),
			GradeLevelAtExam:  phaseAtExam,
			CohortName:        formatCohortLabel(int(intValue(r[20]))),
			EnrollmentYear:    int(intValue(r[20])),
			GraduationYear:    int(intValue(r[21])),
			AcademicYearStart: examYear,
		},
		Cohort: Cohort{
			ID:                       cohortID,
			EnrollmentYear:           int(intValue(r[20])),
			GraduationYear:           int(intValue(r[21])),
			CurrentPhase:             cohortPhase,
			CurrentAcademicYearStart: int(intValue(r[23])),
			GraduatedAt:              cohortGrad,
			Status:                   cohortStatus,
			CreatedAt:                cohortCreated,
		},
		Stage: CohortStage{
			ID:                EncodeStageID(cohortID, stagePhase),
			CohortID:          cohortID,
			AcademicYearStart: examYear,
			GradeLevel:        stagePhase,
			ReviewStatus:      "confirmed",
		},
		AcademicYear: school.AcademicYearLabel(examYear),
		Grade:        phaseAtExam,
	}
	return ac, nil
}

// LoadAssessmentData 读取一次考核参与量化计算所需的全部只读快照、基数、目标、分数线、统计组与任课数据。
func (s *Store) LoadAssessmentData(ctx context.Context, assessmentID int64) (BatchData, error) {
	ac, err := s.LoadAssessmentContext(ctx, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	if err := s.EnsureAssessmentStatGroups(ctx, assessmentID); err != nil {
		return BatchData{}, err
	}
	cohortID := ac.ExamCohort.CohortID
	phase := phaseForStage(ac.ExamCohort.GradeLevelAtExam)

	data := BatchData{
		Batch: Batch{
			ID:                   assessmentID,
			AcademicYear:         ac.AcademicYear,
			Semester:             ac.Exam.Term,
			ExamType:             ac.Exam.ExamType,
			Grade:                ac.Grade,
			Status:               ac.Assessment.Status,
			CurrentPlanVersionID: ac.Assessment.PlanVersion,
			Revision:             ac.Assessment.Revision,
			CreatedAt:            ac.Assessment.CreatedAt,
		},
		ClassBaselines:            map[string]map[string]float64{},
		ClassBaselineRatios:       map[string]map[string]float64{},
		GradeBaselines:            map[string]float64{},
		ExcludedCompletionClasses: map[int64]bool{},
	}

	// 1. 行政班列表（按考试所在阶段 + 参试快照中引用的班级）。
	classRows, err := s.db.Query(ctx, `SELECT id, name FROM classes
		WHERE cohort_id = ? AND (phase = ? OR id IN (
			SELECT DISTINCT class_id FROM exam_participants WHERE exam_cohort_id = ? AND class_id IS NOT NULL
		)) ORDER BY name`, cohortID, phase, ac.ExamCohort.ID)
	if err != nil {
		return BatchData{}, err
	}
	for _, r := range classRows.Values {
		data.Classes = append(data.Classes, Class{ID: intValue(r[0]), Name: stringValue(r[1])})
	}

	// 2. 参试学生与考务快照。
	partRows, err := s.db.Query(ctx, `SELECT ep.student_id, st.student_no, st.name,
		COALESCE(ep.class_id, 0), ep.track_snapshot, ep.elective_snapshot,
		ep.foreign_lang_snapshot, ep.status_snapshot, ep.total_score
		FROM exam_participants ep
		JOIN students st ON st.id = ep.student_id
		WHERE ep.exam_cohort_id = ?
		ORDER BY st.student_no, ep.id`, ac.ExamCohort.ID)
	if err != nil {
		return BatchData{}, err
	}
	studentIndex := map[int64]int{}
	for _, r := range partRows.Values {
		sid := intValue(r[0])
		tot, _ := nullableFloat(r[8])
		fl := stringValue(r[6])
		studentIndex[sid] = len(data.Students)
		data.Students = append(data.Students, Student{
			ID:             sid,
			StudentNo:      stringValue(r[1]),
			Name:           stringValue(r[2]),
			ClassID:        intValue(r[3]),
			Track:          stringValue(r[4]),
			Elective:       stringValue(r[5]),
			Japanese:       fl == "japanese",
			Status:         stringValue(r[7]),
			TotalScore:     tot,
			Scores:         map[string]float64{},
			ScoreFlags:     map[string]string{},
			TeachingGroups: map[string]int64{},
		})
	}

	// 3. 教学班字典（常规班按 admin_class_id，走班按多数归属行政班推断默认参考班级）。
	tgRows, err := s.db.Query(ctx, `SELECT g.id, g.subject, g.name, g.kind, COALESCE(g.admin_class_id, 0),
		COALESCE((SELECT st.current_class_id FROM teaching_group_students tgs
			JOIN students st ON st.id = tgs.student_id
			WHERE tgs.teaching_group_id = g.id AND st.current_class_id IS NOT NULL
			GROUP BY st.current_class_id ORDER BY COUNT(*) DESC, st.current_class_id LIMIT 1), 0)
		FROM teaching_groups g WHERE g.cohort_id = ? AND g.phase = ? ORDER BY g.subject, g.name`, cohortID, phase)
	if err != nil {
		return BatchData{}, err
	}
	adminGroupByClassSubj := map[string]int64{}
	for _, r := range tgRows.Values {
		gid := intValue(r[0])
		subj := stringValue(r[1])
		name := stringValue(r[2])
		kind := stringValue(r[3])
		cid := intValue(r[4])
		if cid == 0 {
			cid = intValue(r[5])
		}
		if kind == "admin" && cid > 0 {
			adminGroupByClassSubj[fmt.Sprintf("%d\x00%s", cid, subj)] = gid
		}
		data.TeachingGroups = append(data.TeachingGroups, TeachingGroup{
			ID: gid, Subject: subj, Name: name, ClassID: cid,
		})
	}

	// 先填充学生常规行政教学班，再用显式走班覆盖，最后优先采用 exam_scores.teaching_group_id_snapshot！
	for idx := range data.Students {
		for _, tg := range data.TeachingGroups {
			if gid := adminGroupByClassSubj[fmt.Sprintf("%d\x00%s", data.Students[idx].ClassID, tg.Subject)]; gid > 0 {
				data.Students[idx].TeachingGroups[tg.Subject] = gid
			}
		}
	}
	walkRows, err := s.db.Query(ctx, `SELECT student_id, subject, teaching_group_id
		FROM teaching_group_students
		WHERE cohort_id = ? AND phase = ? AND ended_at IS NULL`, cohortID, phase)
	if err != nil {
		return BatchData{}, err
	}
	for _, r := range walkRows.Values {
		if idx, ok := studentIndex[intValue(r[0])]; ok {
			data.Students[idx].Walking = true
			data.Students[idx].TeachingGroups[stringValue(r[1])] = intValue(r[2])
		}
	}

	// 4. 成绩及考试教学班快照。
	scoreRows, err := s.db.Query(ctx, `SELECT ep.student_id, es.subject, es.score, es.flag, COALESCE(es.teaching_group_id_snapshot, 0)
		FROM exam_scores es
		JOIN exam_participants ep ON ep.id = es.exam_participant_id
		WHERE ep.exam_cohort_id = ? ORDER BY ep.student_id, es.subject`, ac.ExamCohort.ID)
	if err != nil {
		return BatchData{}, err
	}
	for _, r := range scoreRows.Values {
		idx, ok := studentIndex[intValue(r[0])]
		if !ok {
			continue
		}
		subj := stringValue(r[1])
		if snapTG := intValue(r[4]); snapTG > 0 {
			data.Students[idx].TeachingGroups[subj] = snapTG
		}
		val, present := nullableFloat(r[2])
		if present && val != nil {
			data.Students[idx].Scores[subj] = *val
			data.Students[idx].ScoreFlags[subj] = stringValue(r[3])
		}
	}

	// 5. 统计组、任课关系。
	groups, err := s.ListExamStatGroups(ctx, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	for _, g := range groups {
		target := 0.0
		if g.Target != nil {
			target = *g.Target
		}
		members := make([]int64, 0, len(g.Members))
		for _, m := range g.Members {
			members = append(members, m.TeachingGroupID)
		}
		data.StatGroups = append(data.StatGroups, StatGroup{
			ID:              g.ID,
			Subject:         g.Subject,
			Name:            g.Name,
			LeadClassID:     g.LeadClassID,
			MemberGroupIDs:  members,
			Target:          target,
			BaselineAverage: g.BaselineAverage,
			BaselineRatio:   g.BaselineRatio,
		})
	}

	assignRows, err := s.db.Query(ctx, `SELECT ta.teacher_id, t.name, ta.teaching_group_id
		FROM teaching_assignments ta
		JOIN teachers t ON t.id = ta.teacher_id
		WHERE ta.cohort_id = ? AND ta.phase = ?
		AND (ta.status = 'active' OR ta.end_reason = 'promoted' OR ta.end_reason = 'graduated')
		ORDER BY ta.id`, cohortID, phase)
	if err != nil {
		return BatchData{}, err
	}
	seenAssign := map[string]bool{}
	for _, r := range assignRows.Values {
		tid := intValue(r[0])
		tgID := intValue(r[2])
		key := fmt.Sprintf("%d\x00%d", tid, tgID)
		if seenAssign[key] {
			continue
		}
		seenAssign[key] = true
		data.Assignments = append(data.Assignments, Assignment{
			TeacherID:       tid,
			TeacherName:     stringValue(r[1]),
			TeachingGroupID: tgID,
		})
	}

	// 6. 班级目标及微调、剔除。
	targetIndex := map[int64]int{}
	tRows, err := s.db.Query(ctx, `SELECT class_id, tier_key, target_count, actual_count
		FROM assessment_class_targets WHERE assessment_id = ? ORDER BY class_id, tier_key`, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	for _, r := range tRows.Values {
		cid := intValue(r[0])
		idx, ok := targetIndex[cid]
		if !ok {
			idx = len(data.ClassTargets)
			targetIndex[cid] = idx
			data.ClassTargets = append(data.ClassTargets, ClassTarget{
				ClassID:      cid,
				Counts:       map[string]*float64{},
				ActualCounts: map[string]*float64{},
			})
		}
		tierKey := stringValue(r[1])
		if tv, hasTV := nullableFloat(r[2]); hasTV {
			data.ClassTargets[idx].Counts[tierKey] = tv
		}
		if av, hasAV := nullableFloat(r[3]); hasAV {
			data.ClassTargets[idx].ActualCounts[tierKey] = av
		}
	}

	adjRows, err := s.db.Query(ctx, `SELECT class_id, tier_key, adjusted_target, excluded
		FROM assessment_target_adjustments WHERE assessment_id = ? ORDER BY id`, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	for _, r := range adjRows.Values {
		cid := intValue(r[0])
		tierKey := stringValue(r[1])
		adjVal, hasAdj := nullableFloat(r[2])
		excluded := intValue(r[3]) != 0
		if excluded {
			data.ExcludedCompletionClasses[cid] = true
		}
		if tierKey != "" && hasAdj && adjVal != nil {
			idx, ok := targetIndex[cid]
			if !ok {
				idx = len(data.ClassTargets)
				targetIndex[cid] = idx
				data.ClassTargets = append(data.ClassTargets, ClassTarget{
					ClassID:      cid,
					Counts:       map[string]*float64{},
					ActualCounts: map[string]*float64{},
				})
			}
			copied := *adjVal
			data.ClassTargets[idx].Counts[tierKey] = &copied
		}
	}

	// 7. 基数（student_entry_scores 聚合 + cohort_baseline_overrides）。
	if err := s.fillCohortBaselines(ctx, cohortID, phase, &data); err != nil {
		return BatchData{}, err
	}

	// 8. 学科线与总分档位线。
	cRows, err := s.db.Query(ctx, `SELECT kind, "key", track, cutoff FROM exam_cutoffs
		WHERE exam_cohort_id = ? ORDER BY kind, "key", track`, ac.ExamCohort.ID)
	if err != nil {
		return BatchData{}, err
	}
	for _, r := range cRows.Values {
		kind := stringValue(r[0])
		key := stringValue(r[1])
		track := stringValue(r[2])
		val := floatValue(r[3])
		switch kind {
		case "subject":
			data.SubjectCutoffs = append(data.SubjectCutoffs, SubjectCutoff{Subject: key, Track: track, Value: val})
		case "tier":
			data.TierCutoffs = append(data.TierCutoffs, TierCutoff{TierKey: key, Track: track, Value: val})
		}
	}

	// 9. 本次考核按科微调的分档边界。
	overrides, err := s.ListAssessmentBoundaryOverrides(ctx, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	data.BoundaryOverrides = overrides
	return data, nil
}

func (s *Store) fillCohortBaselines(ctx context.Context, cohortID int64, phase string, data *BatchData) error {
	// 先按学生入学基数算出年级与各班基准。
	gRows, err := s.db.Query(ctx, `SELECT es.subject, AVG(es.score)
		FROM student_entry_scores es
		JOIN students st ON st.id = es.student_id
		WHERE st.cohort_id = ?
		GROUP BY es.subject`, cohortID)
	if err != nil {
		return err
	}
	for _, r := range gRows.Values {
		data.GradeBaselines[stringValue(r[0])] = floatValue(r[1])
	}

	cRows, err := s.db.Query(ctx, `SELECT c.id, es.subject, AVG(es.score)
		FROM student_entry_scores es
		JOIN students st ON st.id = es.student_id
		JOIN classes c ON c.id = st.current_class_id
		WHERE st.cohort_id = ? AND c.phase = ?
		GROUP BY c.id, es.subject`, cohortID, phase)
	if err != nil {
		return err
	}
	for _, r := range cRows.Values {
		cid := strconv.FormatInt(intValue(r[0]), 10)
		subj := stringValue(r[1])
		avg := floatValue(r[2])
		if data.ClassBaselines[subj] == nil {
			data.ClassBaselines[subj] = map[string]float64{}
		}
		data.ClassBaselines[subj][cid] = avg
		if ga := data.GradeBaselines[subj]; ga > 0 {
			if data.ClassBaselineRatios[subj] == nil {
				data.ClassBaselineRatios[subj] = map[string]float64{}
			}
			data.ClassBaselineRatios[subj][cid] = avg / ga
		}
	}

	// 解析同届任意历史阶段 class_id 沿 continued_from_class_id 传承链到当前阶段的 class_id 映射。
	classResolve, err := s.resolveCohortClassToPhaseMap(ctx, cohortID, phase)
	if err != nil {
		return err
	}

	// 应用覆盖表（grade 优先于计算年级均值，class 覆盖班级均分与基数比；当前阶段显式覆盖优先于继承历史覆盖）。
	ovRows, err := s.db.Query(ctx, `SELECT scope_type, scope_id, subject, average, ratio, phase
		FROM cohort_baseline_overrides WHERE cohort_id = ?
		ORDER BY scope_type DESC,
			CASE phase WHEN '' THEN 0 WHEN 'g1' THEN 1 WHEN 'g2' THEN 2 WHEN 'g3' THEN 3 ELSE 4 END ASC,
			id ASC`, cohortID)
	if err != nil {
		return err
	}
	targetPhaseIdx := PhaseIndex(phase)
	directApplied := map[string]bool{}
	for _, r := range ovRows.Values {
		st := stringValue(r[0])
		sid := intValue(r[1])
		subj := stringValue(r[2])
		avg := floatValue(r[3])
		ratioPtr, hasRatio := nullableFloat(r[4])
		ovPhase := stringValue(r[5])
		if ovPhase != "" && targetPhaseIdx > 0 && PhaseIndex(ovPhase) > targetPhaseIdx {
			continue
		}
		switch st {
		case "grade":
			data.GradeBaselines[subj] = avg
		case "class":
			resolvedID, ok := classResolve[sid]
			if !ok || resolvedID <= 0 {
				continue
			}
			isDirect := resolvedID == sid
			key := fmt.Sprintf("%d\x00%s", resolvedID, subj)
			if !isDirect && directApplied[key] {
				continue
			}
			if isDirect {
				directApplied[key] = true
			}
			cid := strconv.FormatInt(resolvedID, 10)
			if data.ClassBaselines[subj] == nil {
				data.ClassBaselines[subj] = map[string]float64{}
			}
			data.ClassBaselines[subj][cid] = avg
			var ratio float64
			if hasRatio && ratioPtr != nil && *ratioPtr > 0 {
				ratio = *ratioPtr
			} else if ga := data.GradeBaselines[subj]; ga > 0 {
				ratio = avg / ga
			}
			if ratio > 0 {
				if data.ClassBaselineRatios[subj] == nil {
					data.ClassBaselineRatios[subj] = map[string]float64{}
				}
				data.ClassBaselineRatios[subj][cid] = ratio
			}
		}
	}
	return nil
}

// resolveCohortClassToPhaseMap 将同一级下各阶段 class_id 沿 continued_from_class_id 链映射到 targetPhase 的 class_id。
func (s *Store) resolveCohortClassToPhaseMap(ctx context.Context, cohortID int64, targetPhase string) (map[int64]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT id, phase, continued_from_class_id FROM classes WHERE cohort_id = ?`, cohortID)
	if err != nil {
		return nil, err
	}
	phaseByID := make(map[int64]string, len(rows.Values))
	nextByID := make(map[int64]int64, len(rows.Values))
	for _, r := range rows.Values {
		id := intValue(r[0])
		phaseByID[id] = stringValue(r[1])
		if prevID, ok := nullableInt64(r[2]); ok && prevID > 0 {
			nextByID[prevID] = id
		}
	}
	resolved := make(map[int64]int64, len(phaseByID))
	for id, ph := range phaseByID {
		if ph == targetPhase {
			resolved[id] = id
			continue
		}
		cur := id
		visited := map[int64]bool{id: true}
		for {
			next, ok := nextByID[cur]
			if !ok || next <= 0 || visited[next] {
				break
			}
			visited[next] = true
			if phaseByID[next] == targetPhase {
				resolved[id] = next
				break
			}
			cur = next
		}
	}
	return resolved, nil
}

// SaveAssessmentRun 保存一次考核的核算运行（包含 result_rows、participants_json、result_row_teachers 与 personal_scores）。
func (s *Store) SaveAssessmentRun(ctx context.Context, assessmentID, versionID int64, inputHash string, inputSummary map[string]string, result calc.Result, official bool) (int64, error) {
	ac, err := s.LoadAssessmentContext(ctx, assessmentID)
	if err != nil {
		return 0, err
	}
	// 构建教师 ID -> 姓名快照映射。
	tRows, err := s.db.Query(ctx, `SELECT id, name FROM teachers`)
	if err != nil {
		return 0, err
	}
	teacherNameMap := make(map[int64]string, len(tRows.Values))
	for _, r := range tRows.Values {
		teacherNameMap[intValue(r[0])] = stringValue(r[1])
	}

	summaryText := ""
	if len(inputSummary) > 0 {
		raw, err := json.Marshal(inputSummary)
		if err != nil {
			return 0, err
		}
		summaryText = string(raw)
	}
	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	if official {
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE result_runs SET is_official = 0 WHERE assessment_id = ? AND is_official = 1`,
			Args: []any{assessmentID},
		})
	}
	runStmtIdx := len(statements)
	statements = append(statements, sqlite.Statement{
		SQL: `INSERT INTO result_runs(assessment_id, plan_version_id, input_hash, input_summary_json, is_official, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
		Args: []any{assessmentID, versionID, inputHash, summaryText, boolInt(official), now},
	})

	for _, row := range result.Rows {
		groupID, err := strconv.ParseInt(row.StatGroupID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("统计组标识 %q 格式不正确", row.StatGroupID)
		}
		var leadClass any
		if row.LeadClassID != "" {
			if parsed, err := strconv.ParseInt(row.LeadClassID, 10, 64); err == nil && parsed > 0 {
				leadClass = parsed
			}
		}
		indicatorsJSON, err := json.Marshal(row.IndicatorValues)
		if err != nil {
			return 0, err
		}
		partPayload := make([]ResultRowStudent, 0, len(row.Participants))
		for _, p := range row.Participants {
			partPayload = append(partPayload, ResultRowStudent{StudentNo: p.StudentNo, Name: p.Name})
		}
		participantsJSON, err := json.Marshal(partPayload)
		if err != nil {
			return 0, err
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO result_rows(
				run_id, stat_group_id, stat_group_name, subject, lead_class_id, lead_class_name,
				class_count, class_average, grade_average, baseline_average, baseline_ratio,
				improvement_raw, improvement_score, pass_count, pass_target, pass_raw, pass_score,
				completion_actual, completion_base, completion_raw, completion_participate, completion_score,
				total_score, rank, tier_index, points, participants_json, indicator_values_json)
				VALUES ((SELECT MAX(id) FROM result_runs WHERE assessment_id = ?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{
				assessmentID, groupID, row.StatGroupName, row.Subject, leadClass, row.LeadClassName,
				row.ClassCount, row.ClassAverage, row.GradeAverage, row.BaselineAverage, row.BaselineRatio,
				row.ImprovementRaw, row.ImprovementScore, row.PassCount, row.PassTarget, row.PassRaw, row.PassScore,
				row.CompletionActual, row.CompletionBase, row.CompletionRaw, boolInt(row.CompletionParticipating), row.CompletionScore,
				row.Total, row.Rank, row.TierIndex, row.Points, string(participantsJSON), string(indicatorsJSON),
			},
		})
		seenTIDs := map[int64]bool{}
		for _, tidStr := range row.TeacherIDs {
			tid, err := strconv.ParseInt(tidStr, 10, 64)
			if err != nil || tid <= 0 || seenTIDs[tid] {
				continue
			}
			seenTIDs[tid] = true
			tName := teacherNameMap[tid]
			if tName == "" && len(row.TeacherNames) > 0 {
				tName = row.TeacherNames[0]
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO result_row_teachers(result_row_id, teacher_id, teacher_name_snapshot)
					VALUES ((SELECT MAX(id) FROM result_rows), ?, ?) ON CONFLICT(result_row_id, teacher_id) DO NOTHING`,
				Args: []any{tid, tName},
			})
		}
		if len(seenTIDs) == 0 && len(row.TeacherNames) > 0 {
			for _, name := range row.TeacherNames {
				for tid, tName := range teacherNameMap {
					if tName == name && !seenTIDs[tid] {
						seenTIDs[tid] = true
						statements = append(statements, sqlite.Statement{
							SQL: `INSERT INTO result_row_teachers(result_row_id, teacher_id, teacher_name_snapshot)
								VALUES ((SELECT MAX(id) FROM result_rows), ?, ?) ON CONFLICT(result_row_id, teacher_id) DO NOTHING`,
							Args: []any{tid, tName},
						})
						break
					}
				}
			}
		}
	}

	for _, ps := range result.Personal {
		tid, err := strconv.ParseInt(ps.TeacherID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("教师标识 %q 格式不正确", ps.TeacherID)
		}
		gpJSON, err := json.Marshal(ps.GroupPoints)
		if err != nil {
			return 0, err
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO personal_scores(run_id, teacher_id, teacher_name_snapshot, cohort_id, phase_at_exam, subject, group_points_json, score)
				VALUES ((SELECT MAX(id) FROM result_runs WHERE assessment_id = ?), ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{assessmentID, tid, ps.TeacherName, ac.Cohort.ID, ac.Grade, ps.Subject, string(gpJSON), ps.Score},
		})
	}
	for _, b := range result.Boundaries {
		source := b.Source
		if source != "override" {
			source = "default"
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO result_run_boundaries(run_id, subject, group_count, first_rank, second_rank, source, reason)
				VALUES ((SELECT MAX(id) FROM result_runs WHERE assessment_id = ?), ?, ?, ?, ?, ?, ?)
				ON CONFLICT(run_id, subject) DO UPDATE SET
					group_count = excluded.group_count,
					first_rank = excluded.first_rank,
					second_rank = excluded.second_rank,
					source = excluded.source,
					reason = excluded.reason`,
			Args: []any{assessmentID, b.Subject, b.GroupCount, b.First, b.Second, source, b.Reason},
		})
	}
	if official {
		statements = append(statements,
			sqlite.Statement{
				SQL: `UPDATE assessments SET plan_version_id = ?, status = 'calculated',
					current_run_id = (SELECT MAX(id) FROM result_runs WHERE assessment_id = ?), revision = revision + 1 WHERE id = ?`,
				Args: []any{versionID, assessmentID, assessmentID},
			},
			sqlite.Statement{
				SQL:  `UPDATE exams SET status = 'assessed' WHERE id = ? AND status <> 'archived'`,
				Args: []any{ac.Exam.ID},
			},
			bumpRevisionStatement(fmt.Sprintf("assessment:%d", assessmentID), now),
			sqlite.Statement{
				SQL: `INSERT INTO audit_logs(action, target_type, target_id, payload_json, created_at)
					VALUES ('calculate_assessment', 'assessment', ?, json_object('run_id', (SELECT MAX(id) FROM result_runs WHERE assessment_id = ?), 'version_id', ?), ?)`,
				Args: []any{assessmentID, assessmentID, versionID, now},
			},
		)
	}
	results, err := s.db.Transaction(ctx, statements)
	if err != nil {
		return 0, err
	}
	return results[runStmtIdx].LastInsertID, nil
}

// ListAssessmentRuns 返回一次考核的历史核算记录，最新在前。
func (s *Store) ListAssessmentRuns(ctx context.Context, assessmentID int64) ([]AssessmentRunSummary, error) {
	rows, err := s.db.Query(ctx, `SELECT id, plan_version_id, input_hash, is_official, created_at
		FROM result_runs WHERE assessment_id = ? ORDER BY id DESC`, assessmentID)
	if err != nil {
		return nil, err
	}
	out := make([]AssessmentRunSummary, 0, len(rows.Values))
	for _, r := range rows.Values {
		createdAt, err := parseTime(r[4])
		if err != nil {
			return nil, err
		}
		out = append(out, AssessmentRunSummary{
			ID:            intValue(r[0]),
			PlanVersionID: intValue(r[1]),
			InputHash:     stringValue(r[2]),
			IsOfficial:    intValue(r[3]) != 0,
			CreatedAt:     createdAt,
		})
	}
	return out, nil
}

// LatestOfficialAssessmentRun 返回本次考核最新的正式核算结果、明细行与教师个人得分。
func (s *Store) LatestOfficialAssessmentRun(ctx context.Context, assessmentID int64) (ResultRun, []StoredResultRow, []StoredPersonalScore, error) {
	rows, err := s.db.Query(ctx, `SELECT id, assessment_id, plan_version_id, input_hash, input_summary_json, is_official, created_at
		FROM result_runs WHERE assessment_id = ? AND is_official = 1 ORDER BY id DESC LIMIT 1`, assessmentID)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	if len(rows.Values) == 0 {
		return ResultRun{}, nil, nil, ErrNotFound
	}
	run, err := assessmentRunFromRow(rows.Values[0])
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	boundaries, err := s.assessmentRunBoundaries(ctx, run.ID)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	run.Boundaries = boundaries
	resRows, personal, err := s.assessmentRunRows(ctx, run.ID)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	return run, resRows, personal, nil
}

// AssessmentRunDetail 读取指定某次核算运行（含试算）的明细行与教师个人得分。
func (s *Store) AssessmentRunDetail(ctx context.Context, assessmentID, runID int64) (ResultRun, []StoredResultRow, []StoredPersonalScore, error) {
	rows, err := s.db.Query(ctx, `SELECT id, assessment_id, plan_version_id, input_hash, input_summary_json, is_official, created_at
		FROM result_runs WHERE id = ? AND assessment_id = ? LIMIT 1`, runID, assessmentID)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	if len(rows.Values) == 0 {
		return ResultRun{}, nil, nil, ErrNotFound
	}
	run, err := assessmentRunFromRow(rows.Values[0])
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	boundaries, err := s.assessmentRunBoundaries(ctx, run.ID)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	run.Boundaries = boundaries
	resRows, personal, err := s.assessmentRunRows(ctx, run.ID)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	return run, resRows, personal, nil
}

func (s *Store) assessmentRunBoundaries(ctx context.Context, runID int64) ([]ResultRunBoundary, error) {
	rows, err := s.db.Query(ctx, `SELECT subject, group_count, first_rank, second_rank, source, reason
		FROM result_run_boundaries WHERE run_id = ? ORDER BY subject`, runID)
	if err != nil {
		return nil, err
	}
	out := make([]ResultRunBoundary, 0, len(rows.Values))
	for _, r := range rows.Values {
		out = append(out, ResultRunBoundary{
			Subject:    stringValue(r[0]),
			GroupCount: int(intValue(r[1])),
			FirstRank:  int(intValue(r[2])),
			SecondRank: int(intValue(r[3])),
			Source:     stringValue(r[4]),
			Reason:     stringValue(r[5]),
		})
	}
	return out, nil
}

func assessmentRunFromRow(row []any) (ResultRun, error) {
	createdAt, err := parseTime(row[6])
	if err != nil {
		return ResultRun{}, err
	}
	return ResultRun{
		ID:            intValue(row[0]),
		BatchID:       intValue(row[1]),
		PlanVersionID: intValue(row[2]),
		InputHash:     stringValue(row[3]),
		InputSummary:  stringValue(row[4]),
		IsOfficial:    intValue(row[5]) != 0,
		CreatedAt:     createdAt,
	}, nil
}

func (s *Store) assessmentRunRows(ctx context.Context, runID int64) ([]StoredResultRow, []StoredPersonalScore, error) {
	rRows, err := s.db.Query(ctx, `SELECT id, stat_group_id, stat_group_name, subject,
		COALESCE(lead_class_id, 0), lead_class_name, class_count, class_average, grade_average,
		baseline_average, baseline_ratio, improvement_raw, improvement_score,
		pass_count, pass_target, pass_raw, pass_score,
		completion_actual, completion_base, completion_raw, completion_participate, completion_score,
		total_score, rank, tier_index, points, participants_json, indicator_values_json
		FROM result_rows WHERE run_id = ? ORDER BY subject, rank, stat_group_name`, runID)
	if err != nil {
		return nil, nil, err
	}

	// 批量读取关联教师快照，避免 N+1 查询。
	tRows, err := s.db.Query(ctx, `SELECT rrt.result_row_id, rrt.teacher_id, rrt.teacher_name_snapshot
		FROM result_row_teachers rrt
		JOIN result_rows rr ON rr.id = rrt.result_row_id
		WHERE rr.run_id = ? ORDER BY rrt.teacher_name_snapshot`, runID)
	if err != nil {
		return nil, nil, err
	}
	rowTeacherNames := map[int64][]string{}
	rowTeacherIDs := map[int64][]int64{}
	for _, tr := range tRows.Values {
		rrID := intValue(tr[0])
		rowTeacherIDs[rrID] = append(rowTeacherIDs[rrID], intValue(tr[1]))
		rowTeacherNames[rrID] = append(rowTeacherNames[rrID], stringValue(tr[2]))
	}

	resultRows := make([]StoredResultRow, 0, len(rRows.Values))
	for _, r := range rRows.Values {
		rrID := intValue(r[0])
		names := rowTeacherNames[rrID]
		sort.Strings(names)
		tJSON := "[]"
		if len(names) > 0 {
			if encoded, err := json.Marshal(names); err == nil {
				tJSON = string(encoded)
			}
		}
		resultRows = append(resultRows, StoredResultRow{
			ID:                    rrID,
			StatGroupID:           intValue(r[1]),
			StatGroupName:         stringValue(r[2]),
			Subject:               stringValue(r[3]),
			LeadClassID:           intValue(r[4]),
			LeadClassName:         stringValue(r[5]),
			TeacherNames:          tJSON,
			TeacherIDs:            rowTeacherIDs[rrID],
			ClassCount:            int(intValue(r[6])),
			ClassAverage:          floatValue(r[7]),
			GradeAverage:          floatValue(r[8]),
			BaselineAverage:       floatValue(r[9]),
			BaselineRatio:         floatValue(r[10]),
			ImprovementRaw:        floatValue(r[11]),
			ImprovementScore:      floatValue(r[12]),
			PassCount:             int(intValue(r[13])),
			PassTarget:            floatValue(r[14]),
			PassRaw:               floatValue(r[15]),
			PassScore:             floatValue(r[16]),
			CompletionActual:      floatValue(r[17]),
			CompletionBase:        floatValue(r[18]),
			CompletionRaw:         floatValue(r[19]),
			CompletionParticipate: intValue(r[20]) != 0,
			CompletionScore:       floatValue(r[21]),
			Total:                 floatValue(r[22]),
			Rank:                  int(intValue(r[23])),
			TierIndex:             int(intValue(r[24])),
			Points:                floatValue(r[25]),
			ParticipantsJSON:      stringValue(r[26]),
			IndicatorJSON:         stringValue(r[27]),
		})
	}

	pRows, err := s.db.Query(ctx, `SELECT teacher_id, teacher_name_snapshot, cohort_id, phase_at_exam, subject, group_points_json, score
		FROM personal_scores WHERE run_id = ? ORDER BY score DESC, subject, teacher_name_snapshot`, runID)
	if err != nil {
		return nil, nil, err
	}
	personal := make([]StoredPersonalScore, 0, len(pRows.Values))
	for _, r := range pRows.Values {
		personal = append(personal, StoredPersonalScore{
			TeacherID:   intValue(r[0]),
			TeacherName: stringValue(r[1]),
			CohortID:    intValue(r[2]),
			Grade:       stringValue(r[3]),
			Subject:     stringValue(r[4]),
			GroupPoints: stringValue(r[5]),
			Score:       floatValue(r[6]),
		})
	}
	return resultRows, personal, nil
}

// AssessmentRowStudents 读取某条计算结果下参与核算的学生快照，并补充本次考核的成绩、分数线与达线状态。
func (s *Store) AssessmentRowStudents(ctx context.Context, runID, statGroupID int64, subject string) ([]ResultRowStudent, error) {
	rows, err := s.db.Query(ctx, `SELECT rr.participants_json, ec.id
		FROM result_rows rr
		JOIN result_runs rrun ON rrun.id = rr.run_id
		JOIN assessments a ON a.id = rrun.assessment_id
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		WHERE rr.run_id = ? AND rr.stat_group_id = ? AND rr.subject = ? LIMIT 1`, runID, statGroupID, subject)
	if err != nil {
		return nil, err
	}
	if len(rows.Values) == 0 {
		return nil, nil
	}
	var students []ResultRowStudent
	raw := stringValue(rows.Values[0][0])
	if raw == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(raw), &students); err != nil {
		return nil, err
	}
	ecID := intValue(rows.Values[0][1])

	cutRows, _ := s.db.Query(ctx, `SELECT track, cutoff FROM exam_cutoffs WHERE exam_cohort_id = ? AND kind = 'subject' AND "key" = ?`,
		ecID, subject)
	cutoffs := map[string]float64{}
	for _, cr := range cutRows.Values {
		cutoffs[stringValue(cr[0])] = floatValue(cr[1])
	}

	scRows, _ := s.db.Query(ctx, `SELECT st.student_no, st.id, ep.class_name_snapshot, ep.track_snapshot, es.score, es.flag
		FROM exam_participants ep
		JOIN students st ON st.id = ep.student_id
		LEFT JOIN exam_scores es ON es.exam_participant_id = ep.id AND es.subject = ?
		WHERE ep.exam_cohort_id = ?`, subject, ecID)
	type stuSnap struct {
		id        int64
		className string
		track     string
		score     *float64
		flag      string
	}
	snapByNo := map[string]stuSnap{}
	for _, sr := range scRows.Values {
		sc, has := nullableFloat(sr[4])
		if !has {
			sc = nil
		}
		snapByNo[stringValue(sr[0])] = stuSnap{
			id:        intValue(sr[1]),
			className: stringValue(sr[2]),
			track:     stringValue(sr[3]),
			score:     sc,
			flag:      stringValue(sr[5]),
		}
	}

	for i := range students {
		students[i].Position = i + 1
		if snap, ok := snapByNo[students[i].StudentNo]; ok {
			students[i].StudentID = snap.id
			students[i].ClassName = snap.className
			students[i].Track = snap.track
			students[i].Score = snap.score
			students[i].Flag = snap.flag
			if cutoff, ok := cutoffs[snap.track]; ok {
				students[i].Cutoff = cutoff
				if snap.score != nil && cutoff > 0 && *snap.score >= cutoff {
					students[i].Passed = true
				}
			}
		}
	}
	return students, nil
}

// EnsureAssessmentStatGroups 为未配置显式走班统计组的常规教学班补充默认 1:1 统计组，并同步目标与基数。
func (s *Store) EnsureAssessmentStatGroups(ctx context.Context, assessmentID int64) error {
	ac, err := s.LoadAssessmentContext(ctx, assessmentID)
	if err != nil {
		return err
	}
	cohortID := ac.ExamCohort.CohortID
	phase := phaseForStage(ac.ExamCohort.GradeLevelAtExam)

	// 取出当前可参考基数（优先使用该届入口基数）。
	data := BatchData{
		ClassBaselines:      map[string]map[string]float64{},
		ClassBaselineRatios: map[string]map[string]float64{},
		GradeBaselines:      map[string]float64{},
	}
	if err := s.fillCohortBaselines(ctx, cohortID, phase, &data); err != nil {
		return err
	}

	tgRows, err := s.db.Query(ctx, `SELECT g.id, g.subject, g.name, COALESCE(g.admin_class_id, 0)
		FROM teaching_groups g WHERE g.cohort_id = ? AND g.phase = ? ORDER BY g.subject, g.name`, cohortID, phase)
	if err != nil {
		return err
	}

	// 读取已有统计组，避免覆盖用户已确认或显式配置的组。
	existingMembersRows, err := s.db.Query(ctx, `SELECT sgm.teaching_group_id, asg.explicit
		FROM assessment_stat_group_members sgm
		JOIN assessment_stat_groups asg ON asg.id = sgm.stat_group_id
		WHERE asg.assessment_id = ?`, assessmentID)
	if err != nil {
		return err
	}
	explicitlyCovered := map[int64]bool{}
	anyCovered := map[int64]bool{}
	for _, r := range existingMembersRows.Values {
		tgID := intValue(r[0])
		anyCovered[tgID] = true
		if intValue(r[1]) != 0 {
			explicitlyCovered[tgID] = true
		}
	}

	// 读取考核分档目标（默认单科或本科目标）。
	classTargets := map[int64]float64{}
	ctRows, err := s.db.Query(ctx, `SELECT class_id, tier_key, target_count FROM assessment_class_targets
		WHERE assessment_id = ? AND target_count IS NOT NULL`, assessmentID)
	if err != nil {
		return err
	}
	for _, r := range ctRows.Values {
		cid := intValue(r[0])
		tk := stringValue(r[1])
		val := floatValue(r[2])
		if tk == "single_subject" {
			classTargets[cid] = val
		} else if tk == "undergraduate" {
			if _, ok := classTargets[cid]; !ok {
				classTargets[cid] = val
			}
		}
	}

	for _, r := range tgRows.Values {
		tgID := intValue(r[0])
		subj := stringValue(r[1])
		name := stringValue(r[2])
		leadCID := intValue(r[3])
		if explicitlyCovered[tgID] || anyCovered[tgID] {
			continue
		}
		var targetArg any
		if tv, ok := classTargets[leadCID]; ok {
			targetArg = tv
		}
		bAvg := 0.0
		bRatio := 0.0
		if leadCID > 0 {
			cidStr := strconv.FormatInt(leadCID, 10)
			if sm := data.ClassBaselines[subj]; sm != nil {
				bAvg = sm[cidStr]
			}
			if rm := data.ClassBaselineRatios[subj]; rm != nil {
				bRatio = rm[cidStr]
			}
		}
		res, err := s.db.Execute(ctx, `INSERT INTO assessment_stat_groups(
			assessment_id, subject, name, lead_class_id, target_count, baseline_average, baseline_ratio, baseline_source, explicit)
			VALUES (?, ?, ?, ?, ?, ?, ?, '', 0)
			ON CONFLICT(assessment_id, subject, name) DO UPDATE SET
				target_count = COALESCE(assessment_stat_groups.target_count, excluded.target_count)`,
			assessmentID, subj, name, nullableID(leadCID), targetArg, bAvg, bRatio)
		if err != nil {
			return err
		}
		sgID := res.LastInsertID
		if sgID == 0 {
			qRows, err := s.db.Query(ctx, `SELECT id FROM assessment_stat_groups WHERE assessment_id = ? AND subject = ? AND name = ?`,
				assessmentID, subj, name)
			if err != nil || len(qRows.Values) == 0 {
				continue
			}
			sgID = intValue(qRows.Values[0][0])
		}
		if _, err := s.db.Execute(ctx, `INSERT INTO assessment_stat_group_members(stat_group_id, teaching_group_id)
			VALUES (?, ?) ON CONFLICT(stat_group_id, teaching_group_id) DO NOTHING`, sgID, tgID); err != nil {
			return err
		}
	}
	return nil
}

// ListExamStatGroups 列出考核下的所有统计组及其成员教学班。
func (s *Store) ListExamStatGroups(ctx context.Context, assessmentID int64) ([]ExamStatGroupRow, error) {
	rows, err := s.db.Query(ctx, `SELECT g.id, g.assessment_id, g.subject, g.name, COALESCE(g.lead_class_id, 0),
		COALESCE(c.name, ''), g.target_count, g.baseline_average, g.baseline_ratio, g.baseline_source, g.explicit
		FROM assessment_stat_groups g
		LEFT JOIN classes c ON c.id = g.lead_class_id
		WHERE g.assessment_id = ? ORDER BY g.subject, g.name`, assessmentID)
	if err != nil {
		return nil, err
	}
	out := make([]ExamStatGroupRow, 0, len(rows.Values))
	for _, r := range rows.Values {
		sgID := intValue(r[0])
		target, _ := nullableFloat(r[6])
		mRows, err := s.db.Query(ctx, `SELECT m.teaching_group_id, tg.subject, tg.name, tg.kind,
			CASE WHEN tg.kind = 'walking' THEN (SELECT COUNT(*) FROM teaching_group_students tgs WHERE tgs.teaching_group_id = tg.id AND tgs.ended_at IS NULL)
			ELSE (SELECT COUNT(*) FROM students st WHERE st.current_class_id = tg.admin_class_id) END,
			CASE WHEN tg.kind = 'walking' THEN COALESCE((SELECT GROUP_CONCAT(DISTINCT c.name) FROM teaching_group_students tgs JOIN students st ON st.id = tgs.student_id JOIN classes c ON c.id = st.current_class_id WHERE tgs.teaching_group_id = tg.id AND tgs.ended_at IS NULL), '')
			ELSE COALESCE((SELECT c.name FROM classes c WHERE c.id = tg.admin_class_id), '') END
			FROM assessment_stat_group_members m
			JOIN teaching_groups tg ON tg.id = m.teaching_group_id
			WHERE m.stat_group_id = ? ORDER BY tg.name`, sgID)
		if err != nil {
			return nil, err
		}
		members := make([]ExamStatGroupMember, 0, len(mRows.Values))
		for _, mr := range mRows.Values {
			members = append(members, ExamStatGroupMember{
				TeachingGroupID: intValue(mr[0]),
				Subject:         stringValue(mr[1]),
				Name:            stringValue(mr[2]),
				Kind:            stringValue(mr[3]),
				StudentCount:    int(intValue(mr[4])),
				ClassNames:      stringValue(mr[5]),
			})
		}
		out = append(out, ExamStatGroupRow{
			ID:              sgID,
			AssessmentID:    intValue(r[1]),
			Subject:         stringValue(r[2]),
			Name:            stringValue(r[3]),
			LeadClassID:     intValue(r[4]),
			LeadClassName:   stringValue(r[5]),
			Target:          target,
			BaselineAverage: floatValue(r[7]),
			BaselineRatio:   floatValue(r[8]),
			BaselineSource:  stringValue(r[9]),
			Explicit:        intValue(r[10]) != 0,
			Members:         members,
		})
	}
	return out, nil
}

// SaveStatGroup 新增或更新一个显式统计组（如走班合并组），同时自动清理被吸纳教学班对应的默认 1:1 组。
func (s *Store) SaveStatGroup(ctx context.Context, input StatGroupInput) (int64, error) {
	assessmentID := input.AssessmentID
	subj := strings.TrimSpace(input.Subject)
	name := strings.TrimSpace(input.Name)
	if assessmentID <= 0 {
		return 0, ErrNotFound
	}
	if subj == "" || name == "" {
		return 0, fmt.Errorf("统计组学科和名称不能为空")
	}
	var sgID int64
	if input.ID > 0 {
		res, err := s.db.Execute(ctx, `UPDATE assessment_stat_groups
			SET subject = ?, name = ?, lead_class_id = ?, target_count = ?,
				baseline_average = ?, baseline_ratio = ?, baseline_source = ?, explicit = 1
			WHERE id = ? AND assessment_id = ?`,
			subj, name, nullableID(input.LeadClassID), optionalFloat(input.Target),
			input.BaselineAverage, input.BaselineRatio, input.BaselineSource, input.ID, assessmentID)
		if err != nil {
			return 0, err
		}
		if res.Changes == 0 {
			return 0, ErrNotFound
		}
		sgID = input.ID
	} else {
		res, err := s.db.Execute(ctx, `INSERT INTO assessment_stat_groups(
			assessment_id, subject, name, lead_class_id, target_count, baseline_average, baseline_ratio, baseline_source, explicit)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)
			ON CONFLICT(assessment_id, subject, name) DO UPDATE SET
				lead_class_id = excluded.lead_class_id,
				target_count = excluded.target_count,
				baseline_average = excluded.baseline_average,
				baseline_ratio = excluded.baseline_ratio,
				baseline_source = excluded.baseline_source,
				explicit = 1`,
			assessmentID, subj, name, nullableID(input.LeadClassID), optionalFloat(input.Target),
			input.BaselineAverage, input.BaselineRatio, input.BaselineSource)
		if err != nil {
			return 0, err
		}
		sgID = res.LastInsertID
		if sgID == 0 {
			rows, err := s.db.Query(ctx, `SELECT id FROM assessment_stat_groups WHERE assessment_id = ? AND subject = ? AND name = ?`,
				assessmentID, subj, name)
			if err != nil || len(rows.Values) == 0 {
				return 0, ErrNotFound
			}
			sgID = intValue(rows.Values[0][0])
		}
	}

	statements := []sqlite.Statement{
		{SQL: `DELETE FROM assessment_stat_group_members WHERE stat_group_id = ?`, Args: []any{sgID}},
	}
	for _, tgID := range input.MemberGroupIDs {
		statements = append(statements,
			sqlite.Statement{
				SQL: `DELETE FROM assessment_stat_group_members WHERE teaching_group_id = ?
					AND stat_group_id IN (SELECT id FROM assessment_stat_groups WHERE assessment_id = ? AND id <> ? AND explicit = 0)`,
				Args: []any{tgID, assessmentID, sgID},
			},
			sqlite.Statement{
				SQL: `INSERT INTO assessment_stat_group_members(stat_group_id, teaching_group_id)
					VALUES (?, ?) ON CONFLICT(stat_group_id, teaching_group_id) DO NOTHING`,
				Args: []any{sgID, tgID},
			},
		)
	}
	statements = append(statements, sqlite.Statement{
		SQL: `DELETE FROM assessment_stat_groups WHERE assessment_id = ? AND explicit = 0
			AND NOT EXISTS (SELECT 1 FROM assessment_stat_group_members m WHERE m.stat_group_id = assessment_stat_groups.id)`,
		Args: []any{assessmentID},
	})
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return 0, err
	}
	if err := s.markAssessmentStale(ctx, 0, assessmentID); err != nil {
		return 0, err
	}
	return sgID, nil
}

// DeleteStatGroup 删除一个统计组。
func (s *Store) DeleteStatGroup(ctx context.Context, assessmentID, groupID int64) error {
	res, err := s.db.Execute(ctx, `DELETE FROM assessment_stat_groups WHERE id = ? AND assessment_id = ?`, groupID, assessmentID)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return ErrNotFound
	}
	return s.markAssessmentStale(ctx, 0, assessmentID)
}

// AssessmentTierKeys 返回考核所属年级应展示的全部达线/目标档位键。
func (s *Store) AssessmentTierKeys(ctx context.Context, assessmentID int64) ([]string, error) {
	return s.assessmentTierKeys(ctx, assessmentID)
}

func (s *Store) assessmentTierKeys(ctx context.Context, assessmentID int64) ([]string, error) {
	plan, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		return nil, err
	}
	ac, err := s.LoadAssessmentContext(ctx, assessmentID)
	if err == nil && ac.Assessment.PlanVersion > 0 {
		if ver, vErr := s.GetVersion(ctx, ac.Assessment.PlanVersion); vErr == nil {
			plan = ver
		}
	}
	keys := make([]string, 0, len(plan.Config.Tiers))
	for _, tier := range plan.Config.Tiers {
		if tier.Key != "" {
			keys = append(keys, tier.Key)
		}
	}
	if len(keys) == 0 {
		keys = []string{"qingbei", "c9", "project985", "project211", "special_control", "undergraduate"}
	}
	return keys, nil
}

// ListClassTargets 返回本次考核已经录入的各班各档位目标。
func (s *Store) ListClassTargets(ctx context.Context, assessmentID int64) ([]AssessmentClassTarget, error) {
	rows, err := s.db.Query(ctx, `SELECT act.class_id, COALESCE(c.name, ''), act.tier_key, act.target_count
		FROM assessment_class_targets act
		LEFT JOIN classes c ON c.id = act.class_id
		WHERE act.assessment_id = ? AND act.target_count IS NOT NULL
		ORDER BY c.name, act.class_id, act.tier_key`, assessmentID)
	if err != nil {
		return nil, err
	}
	out := make([]AssessmentClassTarget, 0, len(rows.Values))
	for _, r := range rows.Values {
		out = append(out, AssessmentClassTarget{
			ClassID:   intValue(r[0]),
			ClassName: stringValue(r[1]),
			TierKey:   stringValue(r[2]),
			Target:    floatValue(r[3]),
		})
	}
	return out, nil
}

// SetClassTarget 设置或更新单个班级的某个档位目标人数。
func (s *Store) SetClassTarget(ctx context.Context, assessmentID, classID int64, tierKey string, target float64) error {
	_, err := s.db.Execute(ctx, `INSERT INTO assessment_class_targets(assessment_id, class_id, tier_key, target_count)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(assessment_id, class_id, tier_key) DO UPDATE SET target_count = excluded.target_count`,
		assessmentID, classID, tierKey, target)
	if err != nil {
		return err
	}
	return s.markAssessmentStale(ctx, 0, assessmentID)
}

// AdjustAssessmentTarget 微调一个班在某个档位的考核目标人数。
func (s *Store) AdjustAssessmentTarget(ctx context.Context, assessmentID, classID int64, tierKey string, value float64, reason string) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `INSERT INTO assessment_target_adjustments(
		assessment_id, class_id, tier_key, adjusted_target, excluded, reason, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(assessment_id, class_id, tier_key) DO UPDATE SET
			adjusted_target = excluded.adjusted_target,
			excluded = 0,
			reason = excluded.reason,
			updated_at = excluded.updated_at`,
		assessmentID, classID, tierKey, value, reason, now)
	if err != nil {
		return err
	}
	return s.markAssessmentStale(ctx, 0, assessmentID)
}

// ExcludeAssessmentClass 将某个班级标记为不参与本次考核完成率考核。
func (s *Store) ExcludeAssessmentClass(ctx context.Context, assessmentID, classID int64, reason string) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `INSERT INTO assessment_target_adjustments(
		assessment_id, class_id, tier_key, adjusted_target, excluded, reason, updated_at)
		VALUES (?, ?, 'undergraduate', NULL, 1, ?, ?)
		ON CONFLICT(assessment_id, class_id, tier_key) DO UPDATE SET
			excluded = 1,
			reason = excluded.reason,
			updated_at = excluded.updated_at`,
		assessmentID, classID, reason, now)
	if err != nil {
		return err
	}
	return s.markAssessmentStale(ctx, 0, assessmentID)
}

// ApplyZeroTargetBatch 批量处理 0 目标或剔除考核班级的配置项。
func (s *Store) ApplyZeroTargetBatch(ctx context.Context, assessmentID int64, items []ZeroTargetBatchItem, defaultReason string) error {
	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	for _, item := range items {
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			reason = strings.TrimSpace(defaultReason)
		}
		if reason == "" {
			return fmt.Errorf("请说明调整或不参评原因")
		}
		if item.Exclude {
			tierKey := item.TierKey
			if tierKey == "" {
				tierKey = "undergraduate"
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO assessment_target_adjustments(assessment_id, class_id, tier_key, adjusted_target, excluded, reason, updated_at)
					VALUES (?, ?, ?, NULL, 1, ?, ?)
					ON CONFLICT(assessment_id, class_id, tier_key) DO UPDATE SET excluded = 1, reason = excluded.reason, updated_at = excluded.updated_at`,
				Args: []any{assessmentID, item.ClassID, tierKey, reason, now},
			})
			continue
		}
		addTarget := func(tierKey string, val float64) {
			statements = append(statements,
				sqlite.Statement{
					SQL: `INSERT INTO assessment_class_targets(assessment_id, class_id, tier_key, target_count)
						VALUES (?, ?, ?, ?)
						ON CONFLICT(assessment_id, class_id, tier_key) DO UPDATE SET target_count = excluded.target_count`,
					Args: []any{assessmentID, item.ClassID, tierKey, val},
				},
				sqlite.Statement{
					SQL: `INSERT INTO assessment_target_adjustments(assessment_id, class_id, tier_key, adjusted_target, excluded, reason, updated_at)
						VALUES (?, ?, ?, ?, 0, ?, ?)
						ON CONFLICT(assessment_id, class_id, tier_key) DO UPDATE SET adjusted_target = excluded.adjusted_target, excluded = 0, reason = excluded.reason, updated_at = excluded.updated_at`,
					Args: []any{assessmentID, item.ClassID, tierKey, val, reason, now},
				},
			)
		}
		for tier, val := range item.Targets {
			addTarget(tier, val)
		}
		if item.Target != nil {
			tierKey := item.TierKey
			if tierKey == "" {
				tierKey = "undergraduate"
			}
			addTarget(tierKey, *item.Target)
		}
	}
	if len(statements) == 0 {
		return nil
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return err
	}
	return s.markAssessmentStale(ctx, 0, assessmentID)
}

// ListAssessmentBoundaryOverrides 返回本次考核已保存的按科分档边界微调列表，按学科排序。
func (s *Store) ListAssessmentBoundaryOverrides(ctx context.Context, assessmentID int64) ([]AssessmentBoundaryOverride, error) {
	rows, err := s.db.Query(ctx, `SELECT id, assessment_id, subject, group_count, first_rank, second_rank, reason, updated_at
		FROM assessment_boundary_overrides WHERE assessment_id = ? ORDER BY subject`, assessmentID)
	if err != nil {
		return nil, err
	}
	out := make([]AssessmentBoundaryOverride, 0, len(rows.Values))
	for _, r := range rows.Values {
		updatedAt, err := parseTime(r[7])
		if err != nil {
			return nil, err
		}
		out = append(out, AssessmentBoundaryOverride{
			ID:           intValue(r[0]),
			AssessmentID: intValue(r[1]),
			Subject:      stringValue(r[2]),
			GroupCount:   int(intValue(r[3])),
			FirstRank:    int(intValue(r[4])),
			SecondRank:   int(intValue(r[5])),
			Reason:       stringValue(r[6]),
			UpdatedAt:    updatedAt,
		})
	}
	return out, nil
}

// SaveAssessmentBoundaryOverrides 在事务内批量保存本次考核的按科分档边界微调，并记录审计日志与数据修订号。
func (s *Store) SaveAssessmentBoundaryOverrides(ctx context.Context, assessmentID int64, items []AssessmentBoundaryOverride) error {
	if err := s.AssertAssessmentCalculable(ctx, assessmentID); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	now := s.now().Format(time.RFC3339Nano)
	statements := make([]sqlite.Statement, 0, len(items)+3)
	subjects := make([]string, 0, len(items))
	for _, item := range items {
		subj := strings.TrimSpace(item.Subject)
		if subj == "" {
			return fmt.Errorf("分档边界微调的学科不能为空")
		}
		if item.GroupCount <= 0 {
			return fmt.Errorf("学科 %s 的参评统计组数必须大于零", subj)
		}
		if item.FirstRank < 1 || item.SecondRank < item.FirstRank || item.SecondRank > item.GroupCount {
			return fmt.Errorf("学科 %s 的分档边界须满足 1 <= 一档止于 <= 二档止于 <= %d", subj, item.GroupCount)
		}
		subjects = append(subjects, subj)
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assessment_boundary_overrides(
				assessment_id, subject, group_count, first_rank, second_rank, reason, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(assessment_id, subject) DO UPDATE SET
					group_count = excluded.group_count,
					first_rank = excluded.first_rank,
					second_rank = excluded.second_rank,
					reason = excluded.reason,
					updated_at = excluded.updated_at`,
			Args: []any{assessmentID, subj, item.GroupCount, item.FirstRank, item.SecondRank, strings.TrimSpace(item.Reason), now},
		})
	}
	payload, err := json.Marshal(map[string]any{"subjects": subjects})
	if err != nil {
		return err
	}
	statements = append(statements,
		bumpRevisionStatement(fmt.Sprintf("assessment:%d", assessmentID), now),
		sqlite.Statement{
			SQL: `INSERT INTO audit_logs(action, target_type, target_id, payload_json, created_at)
				VALUES ('save_boundary_overrides', 'assessment', ?, ?, ?)`,
			Args: []any{assessmentID, string(payload), now},
		},
		sqlite.Statement{
			SQL: `UPDATE assessments SET status = 'stale'
				WHERE id = ? AND status = 'calculated'
				AND NOT EXISTS (SELECT 1 FROM archived_assessments WHERE assessment_id = assessments.id)`,
			Args: []any{assessmentID},
		},
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return err
	}
	return nil
}

// ResetAssessmentBoundaryOverride 在事务内删除指定学科的分档边界微调（恢复方案默认），并记录审计日志与数据修订号。
func (s *Store) ResetAssessmentBoundaryOverride(ctx context.Context, assessmentID int64, subject string) error {
	if err := s.AssertAssessmentCalculable(ctx, assessmentID); err != nil {
		return err
	}
	subj := strings.TrimSpace(subject)
	if subj == "" {
		return fmt.Errorf("学科不能为空")
	}
	now := s.now().Format(time.RFC3339Nano)
	payload, err := json.Marshal(map[string]any{"subject": subj})
	if err != nil {
		return err
	}
	statements := []sqlite.Statement{
		{
			SQL:  `DELETE FROM assessment_boundary_overrides WHERE assessment_id = ? AND subject = ?`,
			Args: []any{assessmentID, subj},
		},
		bumpRevisionStatement(fmt.Sprintf("assessment:%d", assessmentID), now),
		{
			SQL: `INSERT INTO audit_logs(action, target_type, target_id, payload_json, created_at)
				VALUES ('reset_boundary_override', 'assessment', ?, ?, ?)`,
			Args: []any{assessmentID, string(payload), now},
		},
		{
			SQL: `UPDATE assessments SET status = 'stale'
				WHERE id = ? AND status = 'calculated'
				AND NOT EXISTS (SELECT 1 FROM archived_assessments WHERE assessment_id = assessments.id)`,
			Args: []any{assessmentID},
		},
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return err
	}
	return nil
}

// ResetAllAssessmentBoundaryOverrides 在事务内清空本次考核的全部按科分档边界微调，并记录审计日志与数据修订号。
func (s *Store) ResetAllAssessmentBoundaryOverrides(ctx context.Context, assessmentID int64) error {
	if err := s.AssertAssessmentCalculable(ctx, assessmentID); err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	statements := []sqlite.Statement{
		{
			SQL:  `DELETE FROM assessment_boundary_overrides WHERE assessment_id = ?`,
			Args: []any{assessmentID},
		},
		bumpRevisionStatement(fmt.Sprintf("assessment:%d", assessmentID), now),
		{
			SQL: `INSERT INTO audit_logs(action, target_type, target_id, payload_json, created_at)
				VALUES ('reset_all_boundary_overrides', 'assessment', ?, '{}', ?)`,
			Args: []any{assessmentID, now},
		},
		{
			SQL: `UPDATE assessments SET status = 'stale'
				WHERE id = ? AND status = 'calculated'
				AND NOT EXISTS (SELECT 1 FROM archived_assessments WHERE assessment_id = assessments.id)`,
			Args: []any{assessmentID},
		},
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return err
	}
	return nil
}

// FindPreviousAssessmentWithTargets 查找同级上一次已经配置过班级分档目标的考核。
func (s *Store) FindPreviousAssessmentWithTargets(ctx context.Context, assessmentID int64) (int64, error) {
	ac, err := s.LoadAssessmentContext(ctx, assessmentID)
	if err != nil {
		return 0, err
	}
	rows, err := s.db.Query(ctx, `SELECT a.id FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		WHERE ec.cohort_id = ? AND a.id <> ?
		AND EXISTS (SELECT 1 FROM assessment_class_targets act WHERE act.assessment_id = a.id)
		ORDER BY e.academic_year_start DESC, e.exam_date DESC, a.id DESC LIMIT 1`,
		ac.Cohort.ID, assessmentID)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, ErrNotFound
	}
	return intValue(rows.Values[0][0]), nil
}

// CopyAssessmentTargets 从另一次考核复制同一批班级的分档目标。
func (s *Store) CopyAssessmentTargets(ctx context.Context, fromAssessmentID, toAssessmentID int64) (int, error) {
	rows, err := s.db.Query(ctx, `SELECT class_id, tier_key, target_count FROM assessment_class_targets
		WHERE assessment_id = ?`, fromAssessmentID)
	if err != nil {
		return 0, err
	}
	var statements []sqlite.Statement
	copied := 0
	for _, r := range rows.Values {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assessment_class_targets(assessment_id, class_id, tier_key, target_count)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(assessment_id, class_id, tier_key) DO UPDATE SET target_count = excluded.target_count`,
			Args: []any{toAssessmentID, intValue(r[0]), stringValue(r[1]), optionalFloat(func() *float64 { v, _ := nullableFloat(r[2]); return v }())},
		})
		copied++
	}
	if len(statements) > 0 {
		if _, err := s.db.Transaction(ctx, statements); err != nil {
			return 0, err
		}
	}
	if err := s.markAssessmentStale(ctx, 0, toAssessmentID); err != nil {
		return 0, err
	}
	return copied, nil
}

// ListAssessmentOverview 返回全校所有已创建的考核列表摘要。
func (s *Store) ListAssessmentOverview(ctx context.Context) ([]AssessmentOverview, error) {
	return s.queryAssessmentOverviews(ctx, "", nil)
}

func (s *Store) queryAssessmentOverviews(ctx context.Context, whereSQL string, args []any) ([]AssessmentOverview, error) {
	if whereSQL != "" {
		whereSQL = " WHERE " + whereSQL
	}
	rows, err := s.db.Query(ctx, `SELECT a.id, ec.id, e.id, e.academic_year_start, e.term, e.exam_type,
		e.name, e.exam_date, c.id, c.enrollment_year, ec.phase_at_exam,
		a.status, COALESCE(a.plan_version_id, 0), COALESCE(a.current_run_id, 0),
		(SELECT COUNT(*) FROM exam_participants ep WHERE ep.exam_cohort_id = ec.id),
		(SELECT COUNT(*) FROM exam_scores es JOIN exam_participants ep ON ep.id = es.exam_participant_id WHERE ep.exam_cohort_id = ec.id),
		(SELECT COUNT(*) FROM exam_cutoffs x WHERE x.exam_cohort_id = ec.id AND x.kind = 'subject'),
		(SELECT COUNT(*) FROM exam_cutoffs x WHERE x.exam_cohort_id = ec.id AND x.kind = 'tier'),
		(SELECT COUNT(*) FROM assessment_class_targets t WHERE t.assessment_id = a.id),
		(SELECT COUNT(*) FROM assessment_stat_groups g WHERE g.assessment_id = a.id),
		EXISTS(SELECT 1 FROM archived_assessments aa WHERE aa.assessment_id = a.id)
		FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		JOIN cohorts c ON c.id = ec.cohort_id`+whereSQL+`
		ORDER BY e.academic_year_start DESC, e.exam_date DESC, a.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	out := make([]AssessmentOverview, 0, len(rows.Values))
	for _, r := range rows.Values {
		runID := intValue(r[13])
		out = append(out, AssessmentOverview{
			AssessmentID:      intValue(r[0]),
			ExamCohortID:      intValue(r[1]),
			ExamID:            intValue(r[2]),
			AcademicYearStart: int(intValue(r[3])),
			Term:              stringValue(r[4]),
			ExamType:          stringValue(r[5]),
			ExamName:          stringValue(r[6]),
			ExamDate:          stringValue(r[7]),
			CohortID:          intValue(r[8]),
			CohortName:        formatCohortLabel(int(intValue(r[9]))),
			GradeLevel:        stringValue(r[10]),
			Status:            stringValue(r[11]),
			PlanVersionID:     intValue(r[12]),
			CurrentRunID:      runID,
			HasOfficial:       runID > 0,
			Archived:          intValue(r[20]) != 0,
			RosterCount:       int(intValue(r[14])),
			ScoreCount:        int(intValue(r[15])),
			SubjectCutoffCnt:  int(intValue(r[16])),
			TierCutoffCnt:     int(intValue(r[17])),
			CutoffCount:       int(intValue(r[16])) + int(intValue(r[17])),
			TargetCount:       int(intValue(r[18])),
			GroupCount:        int(intValue(r[19])),
			StatGroupCount:    int(intValue(r[19])),
		})
	}
	return out, nil
}

// AssessmentRunExists 判断考核是否已有至少一次核算记录。
func (s *Store) AssessmentRunExists(ctx context.Context, assessmentID int64) (bool, error) {
	cnt, err := s.countQuery(ctx, `SELECT COUNT(*) FROM result_runs WHERE assessment_id = ?`, assessmentID)
	return cnt > 0, err
}

// SaveExport 记录考核结果工作簿的导出路径与文件哈希。
func (s *Store) SaveExport(ctx context.Context, runID int64, filePath, fileHash string) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `INSERT INTO export_records(run_id, file_path, file_hash, exported_at) VALUES (?, ?, ?, ?)`,
		runID, filePath, fileHash, now)
	return err
}

func (s *Store) markAssessmentStale(ctx context.Context, examCohortID int64, assessmentIDs ...int64) error {
	var targetIDs []int64
	targetIDs = append(targetIDs, assessmentIDs...)
	if examCohortID > 0 {
		if a, err := s.findAssessmentByExamCohort(ctx, examCohortID); err == nil {
			targetIDs = append(targetIDs, a.ID)
		}
	}
	for _, aid := range targetIDs {
		if aid <= 0 {
			continue
		}
		// 归档考核的结果只读，输入数据变化不会让它需要重新计算。
		if _, err := s.db.Execute(ctx, `UPDATE assessments SET status = 'stale'
			WHERE id = ? AND status = 'calculated'
			AND NOT EXISTS (SELECT 1 FROM archived_assessments WHERE assessment_id = assessments.id)`, aid); err != nil {
			return err
		}
	}
	return nil
}

// InputHash 计算方案与考核输入数据的确定性摘要哈希，忽略仅在保存运行后变化的状态和修订号。
func InputHash(plan assessment.Config, data BatchData) (string, error) {
	normalized := data
	normalized.Batch.Status = ""
	normalized.Batch.Revision = 0
	normalized.Batch.CurrentPlanVersionID = 0
	if len(normalized.BoundaryOverrides) > 0 {
		clean := make([]AssessmentBoundaryOverride, len(normalized.BoundaryOverrides))
		for i, o := range normalized.BoundaryOverrides {
			clean[i] = AssessmentBoundaryOverride{
				Subject:    o.Subject,
				GroupCount: o.GroupCount,
				FirstRank:  o.FirstRank,
				SecondRank: o.SecondRank,
				Reason:     o.Reason,
			}
		}
		sort.Slice(clean, func(i, j int) bool { return clean[i].Subject < clean[j].Subject })
		normalized.BoundaryOverrides = clean
	}
	type hashPayload struct {
		Plan assessment.Config
		Data BatchData
	}
	raw, err := json.Marshal(hashPayload{Plan: plan, Data: normalized})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// InputSummary 计算参与核算的十类数据的独立哈希映射，用于向用户精确提示哪一类数据有变动。
func InputSummary(plan assessment.Config, data BatchData) (map[string]string, error) {
	summary := make(map[string]string, len(inputItemOrder)+1)
	hash := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(encoded)
		summary[key] = hex.EncodeToString(sum[:])
		return nil
	}

	type rosterEntry struct {
		ID         int64  `json:"id"`
		StudentNo  string `json:"no"`
		Name       string `json:"name"`
		ClassID    int64  `json:"class"`
		Track      string `json:"track"`
		Status     string `json:"status"`
		Japanese   bool   `json:"jp"`
		TotalScore any    `json:"total"`
		Groups     any    `json:"groups"`
	}
	type scoreEntry struct {
		StudentID int64              `json:"id"`
		Scores    map[string]float64 `json:"scores"`
		Flags     map[string]string  `json:"flags"`
	}
	roster := make([]rosterEntry, 0, len(data.Students))
	scores := make([]scoreEntry, 0, len(data.Students))
	for _, st := range data.Students {
		roster = append(roster, rosterEntry{
			ID: st.ID, StudentNo: st.StudentNo, Name: st.Name, ClassID: st.ClassID,
			Track: st.Track, Status: st.Status, Japanese: st.Japanese,
			TotalScore: st.TotalScore, Groups: st.TeachingGroups,
		})
		scores = append(scores, scoreEntry{
			StudentID: st.ID, Scores: st.Scores, Flags: st.ScoreFlags,
		})
	}
	type groupTarget struct {
		ID     int64   `json:"id"`
		Target float64 `json:"target"`
	}
	type groupShape struct {
		ID      int64   `json:"id"`
		Subject string  `json:"subject"`
		Name    string  `json:"name"`
		Lead    int64   `json:"lead"`
		Members []int64 `json:"members"`
	}
	groupTargets := make([]groupTarget, 0, len(data.StatGroups))
	groupShapes := make([]groupShape, 0, len(data.StatGroups))
	for _, g := range data.StatGroups {
		groupTargets = append(groupTargets, groupTarget{ID: g.ID, Target: g.Target})
		groupShapes = append(groupShapes, groupShape{
			ID: g.ID, Subject: g.Subject, Name: g.Name, Lead: g.LeadClassID, Members: g.MemberGroupIDs,
		})
	}
	excluded := make([]string, 0, len(data.ExcludedCompletionClasses))
	for cid := range data.ExcludedCompletionClasses {
		excluded = append(excluded, strconv.FormatInt(cid, 10))
	}
	sort.Strings(excluded)

	type boundaryEntry struct {
		Subject    string `json:"subject"`
		GroupCount int    `json:"group_count"`
		FirstRank  int    `json:"first_rank"`
		SecondRank int    `json:"second_rank"`
		Reason     string `json:"reason"`
	}
	boundaries := make([]boundaryEntry, 0, len(data.BoundaryOverrides))
	for _, o := range data.BoundaryOverrides {
		boundaries = append(boundaries, boundaryEntry{
			Subject:    o.Subject,
			GroupCount: o.GroupCount,
			FirstRank:  o.FirstRank,
			SecondRank: o.SecondRank,
			Reason:     o.Reason,
		})
	}
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i].Subject < boundaries[j].Subject })

	items := map[string]any{
		"roster": roster,
		"scores": scores,
		"cutoffs": struct {
			Subject []SubjectCutoff `json:"subject"`
			Tier    []TierCutoff    `json:"tier"`
		}{Subject: data.SubjectCutoffs, Tier: data.TierCutoffs},
		"targets": struct {
			Class []ClassTarget `json:"class"`
			Group []groupTarget `json:"group"`
		}{Class: data.ClassTargets, Group: groupTargets},
		"groups": groupShapes,
		"baseline": struct {
			Class map[string]map[string]float64 `json:"class"`
			Ratio map[string]map[string]float64 `json:"ratio"`
			Grade map[string]float64            `json:"grade"`
		}{Class: data.ClassBaselines, Ratio: data.ClassBaselineRatios, Grade: data.GradeBaselines},
		"assignments": data.Assignments,
		"classes":     data.Classes,
		"exclusions":  excluded,
		"boundaries":  boundaries,
	}
	keys := append([]string(nil), inputItemOrder...)
	keys = append(keys, "plan")
	for _, key := range keys {
		val := any(plan)
		if key != "plan" {
			val = items[key]
		}
		if err := hash(key, val); err != nil {
			return nil, err
		}
	}
	return summary, nil
}

// ChangedInputItems 对比已保存的输入摘要和当前输入摘要，返回发生变化的数据项标识。
func ChangedInputItems(storedJSON string, current map[string]string) ([]string, error) {
	stored := map[string]string{}
	if strings.TrimSpace(storedJSON) != "" {
		if err := json.Unmarshal([]byte(storedJSON), &stored); err != nil {
			return nil, err
		}
	} else {
		return nil, nil
	}
	changed := map[string]bool{}
	for key, val := range current {
		if key == "plan" {
			continue
		}
		if stored[key] != val {
			changed[key] = true
		}
	}
	var out []string
	for _, key := range inputItemOrder {
		if changed[key] {
			out = append(out, key)
		}
	}
	return out, nil
}
