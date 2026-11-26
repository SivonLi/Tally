package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// WorkspaceSummary 汇总指定级与阶段的基础体量、最近一次考核快照、质量问题与历次考试摘要。
func (s *Store) WorkspaceSummary(ctx context.Context, cohortID, stageID int64) (WorkspaceSummary, error) {
	cohort, phase, err := s.resolveCohortPhaseScope(ctx, cohortID, stageID)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	yearStart := cohort.EnrollmentYear + PhaseIndex(phase) - 1
	counts, err := s.StageCounts(ctx, cohort.ID, yearStart, phase)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	summary := WorkspaceSummary{
		CohortID:          cohort.ID,
		StageID:           EncodeStageID(cohort.ID, phase),
		AcademicYearStart: yearStart,
		GradeLevel:        phase,
		Counts:            counts,
	}

	stuRows, err := s.db.Query(ctx, `SELECT
		COALESCE(SUM(CASE WHEN status IN ('active','') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN current_track = 'physics' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN current_track = 'history' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status NOT IN ('active','') THEN 1 ELSE 0 END), 0)
		FROM students WHERE cohort_id = ?`, cohort.ID)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	if len(stuRows.Values) > 0 {
		r := stuRows.Values[0]
		summary.ActiveStudents = int(intValue(r[0]))
		summary.PhysicsStudents = int(intValue(r[1]))
		summary.HistoryStudents = int(intValue(r[2]))
		summary.InactiveStudents = int(intValue(r[3]))
	}

	bClassRows, err := s.db.Query(ctx, `SELECT COUNT(DISTINCT cid), COUNT(DISTINCT CASE WHEN subj <> 'total' THEN subj END) FROM (
		SELECT c.id AS cid, es.subject AS subj
		FROM student_entry_scores es
		JOIN students st ON st.id = es.student_id
		JOIN classes c ON c.id = st.current_class_id
		WHERE st.cohort_id = ? AND c.phase = ?
		UNION
		SELECT ov.scope_id AS cid, ov.subject AS subj
		FROM cohort_baseline_overrides ov WHERE ov.cohort_id = ? AND ov.scope_type = 'class'
	)`, cohort.ID, phase, cohort.ID)
	if err == nil && len(bClassRows.Values) > 0 {
		summary.BaselineCoveredClasses = int(intValue(bClassRows.Values[0][0]))
		summary.BaselineSubjectCount = int(intValue(bClassRows.Values[0][1]))
	}
	totRows, tErr := s.db.Query(ctx, `SELECT average FROM cohort_baseline_overrides
		WHERE cohort_id = ? AND scope_type = 'grade' AND subject = 'total' LIMIT 1`, cohort.ID)
	if tErr == nil && len(totRows.Values) > 0 {
		summary.GradeBaselineTotal = floatValue(totRows.Values[0][0])
	} else {
		entryRows, eErr := s.db.Query(ctx, `SELECT COUNT(DISTINCT current_class_id), COALESCE(AVG(entry_total_score), 0)
			FROM students WHERE cohort_id = ? AND entry_total_score IS NOT NULL AND entry_total_score > 0`, cohort.ID)
		if eErr == nil && len(entryRows.Values) > 0 && floatValue(entryRows.Values[0][1]) > 0 {
			if summary.BaselineCoveredClasses == 0 {
				summary.BaselineCoveredClasses = int(intValue(entryRows.Values[0][0]))
			}
			summary.GradeBaselineTotal = floatValue(entryRows.Values[0][1])
		}
	}

	overviews, err := s.listAssessmentOverviewForPhase(ctx, cohort.ID, phase)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	summary.ExamCount = len(overviews)

	exams := make([]WorkspaceExamSnapshot, 0, len(overviews))
	for index, ov := range overviews {
		snap := WorkspaceExamSnapshot{
			ExamID:       ov.ExamID,
			AssessmentID: ov.AssessmentID,
			ExamName:     ov.ExamName,
			ExamType:     ov.ExamType,
			Term:         ov.Term,
			ExamDate:     ov.ExamDate,
			Status:       ov.Status,
			RosterCount:  ov.RosterCount,
		}
		var runID, planVerID int64
		var inputHash string
		var hasRun bool
		if ov.CurrentRunID > 0 {
			rRows, rErr := s.db.Query(ctx, `SELECT id, input_hash, plan_version_id FROM result_runs WHERE id = ?`, ov.CurrentRunID)
			if rErr == nil && len(rRows.Values) > 0 {
				runID = intValue(rRows.Values[0][0])
				inputHash = stringValue(rRows.Values[0][1])
				planVerID = intValue(rRows.Values[0][2])
				hasRun = true
			}
		}
		var gradeAvg float64
		var rowCnt, personalCnt int
		if hasRun {
			snap.Calculated = true
			summary.CalculatedExamCount++
			aggRows, aErr := s.db.Query(ctx, `SELECT COALESCE(AVG(total_score), 0),
				COALESCE(AVG(CASE WHEN completion_participate = 1 THEN completion_score END), 0),
				COALESCE(MAX(grade_average), 0), COUNT(*)
				FROM result_rows WHERE run_id = ?`, runID)
			if aErr == nil && len(aggRows.Values) > 0 {
				snap.AverageTotal = floatValue(aggRows.Values[0][0])
				snap.AverageCompletion = floatValue(aggRows.Values[0][1])
				gradeAvg = floatValue(aggRows.Values[0][2])
				rowCnt = int(intValue(aggRows.Values[0][3]))
			}
			personalCnt, _ = s.countQuery(ctx, `SELECT COUNT(*) FROM personal_scores WHERE run_id = ?`, runID)
		}
		exams = append(exams, snap)

		_, issues, pErr := s.ExamPrep(ctx, ov.ExamID, cohort.ID)
		if pErr == nil {
			blk, wrn := 0, 0
			for _, iss := range issues {
				if iss.Severity == "blocking" {
					blk++
				} else {
					wrn++
				}
			}
			summary.BlockingIssues += blk
			summary.WarningIssues += wrn
			if index == 0 {
				stale := false
				if hasRun {
					stale, _ = s.isAssessmentRunStale(ctx, ov.AssessmentID, planVerID, inputHash)
				}
				summary.LatestAssessment = &WorkspaceAssessmentSnapshot{
					AssessmentID:       ov.AssessmentID,
					ExamID:             ov.ExamID,
					ExamName:           ov.ExamName,
					ExamType:           ov.ExamType,
					Term:               ov.Term,
					ExamDate:           ov.ExamDate,
					Status:             ov.Status,
					RosterCount:        ov.RosterCount,
					ScoreCount:         ov.ScoreCount,
					CutoffCount:        ov.SubjectCutoffCnt + ov.TierCutoffCnt,
					TargetCount:        ov.TargetCount,
					GroupCount:         ov.StatGroupCount,
					BlockingIssues:     blk,
					WarningIssues:      wrn,
					HasOfficialRun:     hasRun,
					OfficialStale:      stale,
					GradeAverage:       gradeAvg,
					ResultRowCount:     rowCnt,
					PersonalScoreCount: personalCnt,
				}
			}
		}
	}
	summary.Exams = exams
	return summary, nil
}

func (s *Store) listAssessmentOverviewForPhase(ctx context.Context, cohortID int64, phase string) ([]AssessmentOverview, error) {
	return s.queryAssessmentOverviews(ctx, `ec.cohort_id = ? AND ec.phase_at_exam = ?`, []any{cohortID, phase})
}

func (s *Store) isAssessmentRunStale(ctx context.Context, assessmentID, planVersionID int64, runHash string) (bool, error) {
	if runHash == "" || planVersionID <= 0 {
		return false, nil
	}
	ver, err := s.GetVersion(ctx, planVersionID)
	if err != nil {
		return false, nil
	}
	data, err := s.LoadAssessmentData(ctx, assessmentID)
	if err != nil {
		return false, err
	}
	curHash, err := InputHash(ver.Config, data)
	if err != nil {
		return false, err
	}
	return curHash != runHash, nil
}

// ListExamsByCohort 返回某个级参与的全部考试（cohortID 为 0 时返回全校所有考试）。
func (s *Store) ListExamsByCohort(ctx context.Context, cohortID int64) ([]Exam, error) {
	if cohortID == 0 {
		return s.ListExams(ctx)
	}
	rows, err := s.db.Query(ctx, `SELECT e.id, e.academic_year_start, e.term, e.exam_type, e.name, e.exam_date, e.sequence, e.status, e.created_at
		FROM exams e
		JOIN exam_cohorts ec ON ec.exam_id = e.id
		WHERE ec.cohort_id = ?
		ORDER BY e.academic_year_start DESC, e.exam_date DESC, e.id DESC`, cohortID)
	if err != nil {
		return nil, err
	}
	exams := make([]Exam, 0, len(rows.Values))
	for _, r := range rows.Values {
		exam, err := examFromRow(r)
		if err != nil {
			return nil, err
		}
		cohorts, err := s.loadExamCohorts(ctx, exam.ID, exam.AcademicYearStart)
		if err != nil {
			return nil, err
		}
		exam.Cohorts = cohorts
		exams = append(exams, exam)
	}
	return exams, nil
}

// ListAssessmentOverviewByCohort 返回某个级的全部考核摘要。
func (s *Store) ListAssessmentOverviewByCohort(ctx context.Context, cohortID int64) ([]AssessmentOverview, error) {
	return s.queryAssessmentOverviews(ctx, `ec.cohort_id = ?`, []any{cohortID})
}

// ListExamQualitySummaries 汇总全校每场考试的质量检查摘要。
func (s *Store) ListExamQualitySummaries(ctx context.Context) ([]ExamQualitySummary, error) {
	overviews, err := s.ListAssessmentOverview(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildExamQualitySummaries(ctx, overviews), nil
}

// ListExamQualitySummariesByCohort 汇总某个级每场考试的质量检查摘要。
func (s *Store) ListExamQualitySummariesByCohort(ctx context.Context, cohortID int64) ([]ExamQualitySummary, error) {
	overviews, err := s.ListAssessmentOverviewByCohort(ctx, cohortID)
	if err != nil {
		return nil, err
	}
	return s.buildExamQualitySummaries(ctx, overviews), nil
}

func (s *Store) buildExamQualitySummaries(ctx context.Context, overviews []AssessmentOverview) []ExamQualitySummary {
	out := make([]ExamQualitySummary, 0, len(overviews))
	for _, ov := range overviews {
		_, issues, err := s.ExamPrep(ctx, ov.ExamID, ov.CohortID)
		blk, wrn := 0, 0
		if err == nil {
			for _, iss := range issues {
				if iss.Severity == "blocking" {
					blk++
				} else {
					wrn++
				}
			}
		}
		out = append(out, ExamQualitySummary{
			ExamID:         ov.ExamID,
			ExamName:       ov.ExamName,
			ExamType:       ov.ExamType,
			Term:           ov.Term,
			ExamDate:       ov.ExamDate,
			CohortID:       ov.CohortID,
			CohortName:     ov.CohortName,
			GradeLevel:     ov.GradeLevel,
			Blocking:       blk,
			Warnings:       wrn,
			AssessmentID:   ov.AssessmentID,
			Status:         ov.Status,
			HasOfficialRun: ov.HasOfficial,
			Issues:         issues,
		})
	}
	return out
}

// ListWorkspaceImportRuns 返回指定级/阶段相关的基础数据与考试导入记录。
func (s *Store) ListWorkspaceImportRuns(ctx context.Context, cohortID, stageID int64) ([]ImportRun, error) {
	targetIDs := map[int64]bool{}
	if stageID > 0 {
		targetIDs[stageID] = true
	}
	if cohortID > 0 {
		targetIDs[cohortID] = true
		for _, ph := range []string{PhaseG1, PhaseG2, PhaseG3} {
			targetIDs[EncodeStageID(cohortID, ph)] = true
		}
	}
	ecRows, err := s.db.Query(ctx, `SELECT id FROM exam_cohorts WHERE cohort_id = ?`, cohortID)
	if err != nil {
		return nil, err
	}
	ecSet := map[int64]bool{}
	for _, r := range ecRows.Values {
		ecSet[intValue(r[0])] = true
	}

	all, err := s.ListAllImportRuns(ctx)
	if err != nil {
		return nil, err
	}
	var filtered []ImportRun
	for _, run := range all {
		switch run.ScopeType {
		case "school":
			filtered = append(filtered, run)
		case "cohort":
			if targetIDs[run.ScopeID] {
				filtered = append(filtered, run)
			}
		case "exam":
			if ecSet[run.ScopeID] {
				filtered = append(filtered, run)
			}
		}
	}
	return filtered, nil
}

// GetSchoolOverview 返回全校统一视角总览：全部在读年级态势卡、已毕业数量、全校近期考试与最新考核摘要。
func (s *Store) GetSchoolOverview(ctx context.Context) (SchoolOverview, error) {
	cohorts, err := s.ListCohorts(ctx)
	if err != nil {
		return SchoolOverview{}, err
	}
	exams, err := s.ListExams(ctx)
	if err != nil {
		return SchoolOverview{}, err
	}
	assessments, err := s.ListAssessmentOverview(ctx)
	if err != nil {
		return SchoolOverview{}, err
	}
	latestByCohort := make(map[int64]AssessmentOverview)
	examCountByCohort := make(map[int64]int)
	for _, ov := range assessments {
		examCountByCohort[ov.CohortID]++
		if _, exists := latestByCohort[ov.CohortID]; !exists {
			latestByCohort[ov.CohortID] = ov
		}
	}

	var out SchoolOverview
	out.RecentExams = exams
	out.LatestAssessments = assessments
	for _, cohort := range cohorts {
		if cohort.CurrentPhase == PhaseGraduated {
			out.GraduatedCount++
			continue
		}
		counts, err := s.StageCounts(ctx, cohort.ID, cohort.CurrentAcademicYearStart, cohort.CurrentPhase)
		if err != nil {
			return SchoolOverview{}, err
		}
		activeStudents, err := s.countQuery(ctx, `SELECT COUNT(*) FROM students
			WHERE cohort_id = ? AND status IN ('active','')`, cohort.ID)
		if err != nil {
			return SchoolOverview{}, err
		}
		card := CohortOverviewCard{
			Cohort:          cohort,
			ClassCount:      counts.Classes,
			ActiveStudents:  activeStudents,
			TeacherCount:    counts.Teachers,
			AssignmentCount: counts.Assignments,
			BaselineCount:   counts.Baselines,
			ExamCount:       examCountByCohort[cohort.ID],
		}
		if latest, ok := latestByCohort[cohort.ID]; ok {
			card.LatestExamID = latest.ExamID
			card.LatestExamName = latest.ExamName
			card.LatestExamDate = latest.ExamDate
			card.LatestExamStatus = latest.Status
			card.LatestAssessmentID = latest.AssessmentID
			card.LatestAssessmentStatus = latest.Status
			card.HasOfficialRun = latest.HasOfficial
			_, issues, pErr := s.ExamPrep(ctx, latest.ExamID, cohort.ID)
			if pErr == nil {
				for _, iss := range issues {
					if iss.Severity == "blocking" {
						card.BlockingIssues++
					}
				}
			}
		}
		out.ActiveCohorts = append(out.ActiveCohorts, card)
	}
	return out, nil
}

var _ = fmt.Sprint
var _ = strings.TrimSpace
var _ = json.Marshal
