package store

import (
	"context"
	"strings"
)

// WorkspaceAssessmentSnapshot 是当前工作区最近一次考试/考核在总览台账上的数据快照。
type WorkspaceAssessmentSnapshot struct {
	AssessmentID       int64
	ExamID             int64
	ExamName           string
	ExamType           string
	Term               string
	ExamDate           string
	Status             string
	RosterCount        int
	ScoreCount         int
	CutoffCount        int
	TargetCount        int
	GroupCount         int
	BlockingIssues     int
	WarningIssues      int
	HasOfficialRun     bool
	OfficialStale      bool
	GradeAverage       float64
	ResultRowCount     int
	PersonalScoreCount int
}

// WorkspaceExamSnapshot 是当前工作区历次考试在总览时间线与走势微图中的数据摘要。
type WorkspaceExamSnapshot struct {
	ExamID            int64
	AssessmentID      int64
	ExamName          string
	ExamType          string
	Term              string
	ExamDate          string
	Status            string
	RosterCount       int
	Calculated        bool
	AverageTotal      float64
	AverageCompletion float64
}

// WorkspaceSummary 聚合一个工作区（级与阶段）的全部总览实时台账数据。
type WorkspaceSummary struct {
	CohortID               int64
	StageID                int64
	AcademicYearStart      int
	GradeLevel             string
	Counts                 StageCounts
	ActiveStudents         int
	PhysicsStudents        int
	HistoryStudents        int
	InactiveStudents       int
	BaselineCoveredClasses int
	BaselineSubjectCount   int
	GradeBaselineTotal     float64
	ExamCount              int
	CalculatedExamCount    int
	BlockingIssues         int
	WarningIssues          int
	LatestAssessment       *WorkspaceAssessmentSnapshot
	Exams                  []WorkspaceExamSnapshot
}

// WorkspaceSummary 汇总指定级与阶段的基础体量、最近一次考核快照、质量问题与历次考试摘要。
func (s *Store) WorkspaceSummary(ctx context.Context, cohortID, stageID int64) (WorkspaceSummary, error) {
	stage, err := s.cohortStageByID(ctx, cohortID, stageID)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	counts, err := s.StageCounts(ctx, cohortID, stage.AcademicYearStart, stage.GradeLevel)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	summary := WorkspaceSummary{
		CohortID:          cohortID,
		StageID:           stage.ID,
		AcademicYearStart: stage.AcademicYearStart,
		GradeLevel:        stage.GradeLevel,
		Counts:            counts,
	}

	studentRows, err := s.db.Query(ctx, `SELECT
		COALESCE(SUM(CASE WHEN status = 'active' OR status = '' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN track = 'physics' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN track = 'history' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status <> '' AND status <> 'active' THEN 1 ELSE 0 END), 0)
		FROM student_stage_memberships WHERE stage_id = ?`, stage.ID)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	if len(studentRows.Values) > 0 {
		row := studentRows.Values[0]
		summary.ActiveStudents = int(intValue(row[0]))
		summary.PhysicsStudents = int(intValue(row[1]))
		summary.HistoryStudents = int(intValue(row[2]))
		summary.InactiveStudents = int(intValue(row[3]))
	}

	baselineRows, err := s.db.Query(ctx, `SELECT COUNT(DISTINCT scope_id), COUNT(DISTINCT subject)
		FROM baseline_aggregates WHERE stage_id = ? AND scope_type = 'class'`, stage.ID)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	if len(baselineRows.Values) > 0 {
		summary.BaselineCoveredClasses = int(intValue(baselineRows.Values[0][0]))
		summary.BaselineSubjectCount = int(intValue(baselineRows.Values[0][1]))
	}

	gradeBaseRows, err := s.db.Query(ctx, `SELECT a.average FROM baseline_aggregates a
		JOIN baseline_sets b ON b.id = a.baseline_set_id
		WHERE a.stage_id = ? AND a.scope_type = 'stage' AND a.subject = 'total'
		ORDER BY CASE WHEN b.status = 'active' THEN 0 ELSE 1 END, a.id DESC LIMIT 1`, stage.ID)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	if len(gradeBaseRows.Values) > 0 {
		summary.GradeBaselineTotal = floatValue(gradeBaseRows.Values[0][0])
	} else {
		entranceRows, err := s.db.Query(ctx, `SELECT COALESCE(AVG(total_score), 0)
			FROM student_stage_memberships
			WHERE stage_id = ? AND total_score IS NOT NULL AND total_score > 0`, stage.ID)
		if err != nil {
			return WorkspaceSummary{}, err
		}
		if len(entranceRows.Values) > 0 {
			summary.GradeBaselineTotal = floatValue(entranceRows.Values[0][0])
		}
	}

	overviews, err := s.listAssessmentOverviewForStage(ctx, cohortID, stage.ID)
	if err != nil {
		return WorkspaceSummary{}, err
	}
	summary.ExamCount = len(overviews)
	summary.Exams = make([]WorkspaceExamSnapshot, 0, len(overviews))

	for idx, item := range overviews {
		issues, err := s.ExamQualityIssues(ctx, item.ExamID, cohortID)
		if err != nil {
			return WorkspaceSummary{}, err
		}
		blocking, warnings := 0, 0
		for _, issue := range issues {
			if issue.Severity == "blocking" {
				blocking++
			} else {
				warnings++
			}
		}
		summary.BlockingIssues += blocking
		summary.WarningIssues += warnings

		runID, runHash, planVersionID, hasOfficial, err := s.latestOfficialRunMeta(ctx, item.AssessmentID)
		if err != nil {
			return WorkspaceSummary{}, err
		}
		var avgTotal, avgCompletion, gradeAvg float64
		var rowCount, personalCount int
		if hasOfficial {
			summary.CalculatedExamCount++
			avgTotal, avgCompletion, gradeAvg, rowCount, personalCount, err = s.runAggregateStats(ctx, runID)
			if err != nil {
				return WorkspaceSummary{}, err
			}
		}
		summary.Exams = append(summary.Exams, WorkspaceExamSnapshot{
			ExamID:            item.ExamID,
			AssessmentID:      item.AssessmentID,
			ExamName:          item.ExamName,
			ExamType:          item.ExamType,
			Term:              item.Term,
			ExamDate:          item.ExamDate,
			Status:            item.Status,
			RosterCount:       item.RosterCount,
			Calculated:        hasOfficial,
			AverageTotal:      avgTotal,
			AverageCompletion: avgCompletion,
		})

		if idx == 0 {
			stale := false
			if hasOfficial {
				stale, err = s.isAssessmentRunStale(ctx, item.AssessmentID, planVersionID, runHash)
				if err != nil {
					return WorkspaceSummary{}, err
				}
			}
			snap := WorkspaceAssessmentSnapshot{
				AssessmentID:       item.AssessmentID,
				ExamID:             item.ExamID,
				ExamName:           item.ExamName,
				ExamType:           item.ExamType,
				Term:               item.Term,
				ExamDate:           item.ExamDate,
				Status:             item.Status,
				RosterCount:        item.RosterCount,
				ScoreCount:         item.ScoreCount,
				CutoffCount:        item.CutoffCount,
				TargetCount:        item.TargetCount,
				GroupCount:         item.GroupCount,
				BlockingIssues:     blocking,
				WarningIssues:      warnings,
				HasOfficialRun:     hasOfficial,
				OfficialStale:      stale,
				GradeAverage:       gradeAvg,
				ResultRowCount:     rowCount,
				PersonalScoreCount: personalCount,
			}
			summary.LatestAssessment = &snap
		}
	}
	return summary, nil
}

func (s *Store) cohortStageByID(ctx context.Context, cohortID, stageID int64) (CohortStage, error) {
	rows, err := s.db.Query(ctx, `SELECT id, cohort_id, academic_year_start, grade_level,
		copied_from_stage_id, review_status, confirmed_at
		FROM cohort_stages WHERE cohort_id = ? AND id = ?`, cohortID, stageID)
	if err != nil {
		return CohortStage{}, err
	}
	if len(rows.Values) == 0 {
		return CohortStage{}, ErrNotFound
	}
	return cohortStageFromRow(rows.Values[0])
}

func (s *Store) listAssessmentOverviewForStage(ctx context.Context, cohortID, stageID int64) ([]AssessmentOverview, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id, e.id, e.name, e.exam_type, e.term, e.exam_date,
		e.academic_year_start, c.id, c.name, ec.grade_level_at_exam, c.graduation_year, ec.id,
		a.status,
		(SELECT COUNT(*) FROM exam_rosters r WHERE r.exam_id = e.id AND r.cohort_id = c.id),
		(SELECT COUNT(*) FROM exam_scores sc WHERE sc.exam_id = e.id AND sc.cohort_id = c.id AND sc.score IS NOT NULL),
		(SELECT COUNT(*) FROM assessment_class_targets t WHERE t.assessment_id = a.id),
		(SELECT COUNT(*) FROM assessment_stat_groups g WHERE g.assessment_id = a.id),
		(SELECT COUNT(*) FROM exam_subject_cutoffs sc WHERE sc.exam_id = e.id AND sc.cohort_id = c.id)
			+ (SELECT COUNT(*) FROM exam_tier_cutoffs tc WHERE tc.exam_id = e.id AND tc.cohort_id = c.id)
		FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		JOIN cohorts c ON c.id = ec.cohort_id
		WHERE ec.cohort_id = ? AND ec.stage_id = ?
		ORDER BY e.academic_year_start DESC, e.exam_date DESC, e.id DESC`, cohortID, stageID)
	if err != nil {
		return nil, err
	}
	return assessmentOverviewsFromRows(rows.Values), nil
}

func (s *Store) latestOfficialRunMeta(ctx context.Context, assessmentID int64) (runID int64, inputHash string, planVersionID int64, ok bool, err error) {
	rows, err := s.db.Query(ctx, `SELECT id, input_hash, plan_version_id
		FROM result_runs WHERE assessment_id = ? AND is_official = 1
		ORDER BY id DESC LIMIT 1`, assessmentID)
	if err != nil {
		return 0, "", 0, false, err
	}
	if len(rows.Values) == 0 {
		return 0, "", 0, false, nil
	}
	row := rows.Values[0]
	return intValue(row[0]), stringValue(row[1]), intValue(row[2]), true, nil
}

func (s *Store) runAggregateStats(ctx context.Context, runID int64) (avgTotal, avgCompletion, gradeAvg float64, rowCount, personalCount int, err error) {
	rows, err := s.db.Query(ctx, `SELECT COUNT(*),
		COALESCE(AVG(total), 0),
		COALESCE(AVG(CASE WHEN completion_participate = 1 THEN completion_score END), 0),
		COALESCE(AVG(grade_average), 0)
		FROM result_rows WHERE run_id = ?`, runID)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	if len(rows.Values) > 0 {
		row := rows.Values[0]
		rowCount = int(intValue(row[0]))
		avgTotal = floatValue(row[1])
		avgCompletion = floatValue(row[2])
		gradeAvg = floatValue(row[3])
	}
	pRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM personal_scores WHERE run_id = ?`, runID)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	if len(pRows.Values) > 0 {
		personalCount = int(intValue(pRows.Values[0][0]))
	}
	return avgTotal, avgCompletion, gradeAvg, rowCount, personalCount, nil
}

func (s *Store) isAssessmentRunStale(ctx context.Context, assessmentID, planVersionID int64, runHash string) (bool, error) {
	if strings.TrimSpace(runHash) == "" || planVersionID == 0 {
		return false, nil
	}
	version, err := s.GetVersion(ctx, planVersionID)
	if err != nil {
		return false, err
	}
	data, err := s.LoadAssessmentData(ctx, assessmentID)
	if err != nil {
		return false, err
	}
	hash, err := InputHash(version.Config, data)
	if err != nil {
		return false, err
	}
	return hash != runHash, nil
}

// ListExamsByCohort 返回指定工作区（级）参加的全部考试；cohortID 为 0 时返回全校考试。
func (s *Store) ListExamsByCohort(ctx context.Context, cohortID int64) ([]Exam, error) {
	if cohortID == 0 {
		return s.ListExams(ctx)
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT e.id, e.academic_year_start, e.term, e.exam_type,
		e.name, e.exam_date, e.sequence, e.status, e.created_at
		FROM exams e
		JOIN exam_cohorts ec ON ec.exam_id = e.id
		WHERE ec.cohort_id = ?
		ORDER BY e.academic_year_start DESC, e.exam_date DESC, e.id DESC`, cohortID)
	if err != nil {
		return nil, err
	}
	exams := make([]Exam, 0, len(rows.Values))
	for _, row := range rows.Values {
		exam, err := examFromRow(row)
		if err != nil {
			return nil, err
		}
		exams = append(exams, exam)
	}
	for index := range exams {
		cohorts, err := s.ListExamCohorts(ctx, exams[index].ID)
		if err != nil {
			return nil, err
		}
		exams[index].Cohorts = cohorts
	}
	return exams, nil
}

// ListAssessmentOverviewByCohort 返回指定工作区（级）的考核概况；cohortID 为 0 时返回全校。
func (s *Store) ListAssessmentOverviewByCohort(ctx context.Context, cohortID int64) ([]AssessmentOverview, error) {
	if cohortID == 0 {
		return s.ListAssessmentOverview(ctx)
	}
	rows, err := s.db.Query(ctx, `SELECT a.id, e.id, e.name, e.exam_type, e.term, e.exam_date,
		e.academic_year_start, c.id, c.name, ec.grade_level_at_exam, c.graduation_year, ec.id,
		a.status,
		(SELECT COUNT(*) FROM exam_rosters r WHERE r.exam_id = e.id AND r.cohort_id = c.id),
		(SELECT COUNT(*) FROM exam_scores sc WHERE sc.exam_id = e.id AND sc.cohort_id = c.id AND sc.score IS NOT NULL),
		(SELECT COUNT(*) FROM assessment_class_targets t WHERE t.assessment_id = a.id),
		(SELECT COUNT(*) FROM assessment_stat_groups g WHERE g.assessment_id = a.id),
		(SELECT COUNT(*) FROM exam_subject_cutoffs sc WHERE sc.exam_id = e.id AND sc.cohort_id = c.id)
			+ (SELECT COUNT(*) FROM exam_tier_cutoffs tc WHERE tc.exam_id = e.id AND tc.cohort_id = c.id)
		FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		JOIN cohorts c ON c.id = ec.cohort_id
		WHERE ec.cohort_id = ?
		ORDER BY e.academic_year_start DESC, e.exam_date DESC, e.id DESC, c.name`, cohortID)
	if err != nil {
		return nil, err
	}
	return assessmentOverviewsFromRows(rows.Values), nil
}

func assessmentOverviewsFromRows(values [][]any) []AssessmentOverview {
	items := make([]AssessmentOverview, 0, len(values))
	for _, row := range values {
		items = append(items, AssessmentOverview{
			AssessmentID:      intValue(row[0]),
			ExamID:            intValue(row[1]),
			ExamName:          stringValue(row[2]),
			ExamType:          stringValue(row[3]),
			Term:              stringValue(row[4]),
			ExamDate:          stringValue(row[5]),
			AcademicYearStart: int(intValue(row[6])),
			CohortID:          intValue(row[7]),
			CohortName:        stringValue(row[8]),
			GradeLevel:        stringValue(row[9]),
			GraduationYear:    int(intValue(row[10])),
			ExamCohortID:      intValue(row[11]),
			Status:            stringValue(row[12]),
			RosterCount:       int(intValue(row[13])),
			ScoreCount:        int(intValue(row[14])),
			TargetCount:       int(intValue(row[15])),
			GroupCount:        int(intValue(row[16])),
			CutoffCount:       int(intValue(row[17])),
		})
	}
	return items
}

// ListExamQualitySummariesByCohort 返回指定工作区（级）的考试数据质量问题；cohortID 为 0 时返回全校。
func (s *Store) ListExamQualitySummariesByCohort(ctx context.Context, cohortID int64) ([]ExamQualitySummary, error) {
	if cohortID == 0 {
		return s.ListExamQualitySummaries(ctx)
	}
	exams, err := s.ListExamsByCohort(ctx, cohortID)
	if err != nil {
		return nil, err
	}
	result := make([]ExamQualitySummary, 0, len(exams))
	for _, exam := range exams {
		for _, cohort := range exam.Cohorts {
			if cohort.CohortID != cohortID {
				continue
			}
			issues, err := s.ExamQualityIssues(ctx, exam.ID, cohort.CohortID)
			if err != nil {
				return nil, err
			}
			if len(issues) == 0 {
				continue
			}
			result = append(result, ExamQualitySummary{
				ExamID: exam.ID, ExamName: exam.Name, CohortID: cohort.CohortID,
				CohortName: cohort.CohortName, GradeLevel: cohort.GradeLevelAtExam,
				AcademicStart: exam.AcademicYearStart, Issues: issues,
			})
		}
	}
	return result, nil
}

// ListWorkspaceImportRuns 返回指定工作区（级与阶段）的全部基础数据与考试数据导入记录，按时间倒序排列。
// 当 stageID 为 0 时返回该级全部阶段及考试的导入记录；cohortID 也为 0 时返回全校全部记录。
func (s *Store) ListWorkspaceImportRuns(ctx context.Context, cohortID, stageID int64) ([]ImportRun, error) {
	if cohortID == 0 && stageID == 0 {
		return s.ListAllImportRuns(ctx)
	}
	var query string
	var args []any
	if stageID != 0 {
		query = `SELECT id, scope_type, scope_id, data_kind, file_hash,
			stored_path, relative_path, status, row_count, warning_count, created_at
			FROM import_runs
			WHERE (scope_type = 'stage' AND scope_id = ?)
				OR (scope_type = 'exam' AND scope_id IN (
					SELECT id FROM exam_cohorts WHERE cohort_id = ? AND stage_id = ?))
			ORDER BY id DESC`
		args = []any{stageID, cohortID, stageID}
	} else {
		query = `SELECT id, scope_type, scope_id, data_kind, file_hash,
			stored_path, relative_path, status, row_count, warning_count, created_at
			FROM import_runs
			WHERE (scope_type = 'stage' AND scope_id IN (
					SELECT id FROM cohort_stages WHERE cohort_id = ?))
				OR (scope_type = 'exam' AND scope_id IN (
					SELECT id FROM exam_cohorts WHERE cohort_id = ?))
			ORDER BY id DESC`
		args = []any{cohortID, cohortID}
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	runs := make([]ImportRun, 0, len(rows.Values))
	for _, row := range rows.Values {
		run, err := importRunFromRow(row)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	for index := range runs {
		sheets, err := s.importRunSheets(ctx, runs[index].ID)
		if err != nil {
			return nil, err
		}
		runs[index].Sheets = sheets
	}
	return runs, nil
}
