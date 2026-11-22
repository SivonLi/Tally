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

	"tally/internal/calc"
	"tally/internal/school"
)

// AssessmentContext 是一场考试中一个级的考核及其展示信息。
type AssessmentContext struct {
	Assessment   Assessment
	Exam         Exam
	ExamCohort   ExamCohort
	Cohort       Cohort
	Stage        CohortStage
	AcademicYear string
	Grade        string
}

// LoadAssessmentContext 读取本次考核关联的考试、级与阶段。
func (s *Store) LoadAssessmentContext(ctx context.Context, assessmentID int64) (AssessmentContext, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id, a.exam_cohort_id, COALESCE(a.plan_version_id, 0),
		a.status, COALESCE(a.current_run_id, 0), a.revision,
		e.id, e.academic_year_start, e.term, e.exam_type, e.name, e.exam_date,
		e.status, e.created_at,
		ec.id, ec.cohort_id, ec.stage_id, ec.grade_level_at_exam,
		c.id, c.enrollment_year, c.graduation_year, c.status, c.created_at,
		st.id, st.cohort_id, st.academic_year_start, st.grade_level, st.review_status
		FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		JOIN cohorts c ON c.id = ec.cohort_id
		JOIN cohort_stages st ON st.id = ec.stage_id
		WHERE a.id = ?`, assessmentID)
	if err != nil {
		return AssessmentContext{}, err
	}
	if len(rows.Values) == 0 {
		return AssessmentContext{}, ErrNotFound
	}
	row := rows.Values[0]
	createdAt, err := parseTime(row[13])
	if err != nil {
		return AssessmentContext{}, err
	}
	cohortCreated, err := parseTime(row[22])
	if err != nil {
		return AssessmentContext{}, err
	}
	context := AssessmentContext{}
	context.Assessment = Assessment{
		ID:           intValue(row[0]),
		ExamCohortID: intValue(row[1]),
		PlanVersion:  intValue(row[2]),
		Status:       stringValue(row[3]),
		CurrentRunID: intValue(row[4]),
		Revision:     int(intValue(row[5])),
	}
	context.Exam = Exam{
		ID:                intValue(row[6]),
		AcademicYearStart: int(intValue(row[7])),
		Term:              stringValue(row[8]),
		ExamType:          stringValue(row[9]),
		Name:              stringValue(row[10]),
		ExamDate:          stringValue(row[11]),
		Status:            stringValue(row[12]),
		CreatedAt:         createdAt,
	}
	context.ExamCohort = ExamCohort{
		ID:               intValue(row[14]),
		ExamID:           context.Exam.ID,
		CohortID:         intValue(row[15]),
		StageID:          intValue(row[16]),
		GradeLevelAtExam: stringValue(row[17]),
	}
	context.Cohort = Cohort{
		ID:             intValue(row[18]),
		EnrollmentYear: int(intValue(row[19])),
		GraduationYear: int(intValue(row[20])),
		Status:         stringValue(row[21]),
		CreatedAt:      cohortCreated,
	}
	context.Stage = CohortStage{
		ID:                intValue(row[23]),
		CohortID:          intValue(row[24]),
		AcademicYearStart: int(intValue(row[25])),
		GradeLevel:        stringValue(row[26]),
		ReviewStatus:      stringValue(row[27]),
	}
	context.AcademicYear = school.AcademicYearLabel(context.Exam.AcademicYearStart)
	context.Grade = context.Stage.GradeLevel
	return context, nil
}

// LoadAssessmentData 读取本次考核的核算输入。
func (s *Store) LoadAssessmentData(ctx context.Context, assessmentID int64) (BatchData, error) {
	context, err := s.LoadAssessmentContext(ctx, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	data := BatchData{
		Batch: Batch{
			AcademicYear: context.AcademicYear,
			Semester:     context.Exam.Term,
			ExamType:     context.Exam.ExamType,
			Grade:        context.Grade,
		},
		ClassBaselines:            map[string]map[string]float64{},
		ClassBaselineRatios:       map[string]map[string]float64{},
		GradeBaselines:            map[string]float64{},
		ExcludedCompletionClasses: map[int64]bool{},
	}
	stageID := context.ExamCohort.StageID
	classesByID := map[int64]string{}
	rows, err := s.db.Query(ctx, `SELECT c.id, c.name FROM classes c
		JOIN class_stage_info i ON i.class_id = c.id WHERE i.stage_id = ? ORDER BY c.name`, stageID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		id := intValue(row[0])
		name := stringValue(row[1])
		classesByID[id] = name
		data.Classes = append(data.Classes, Class{ID: id, Name: name})
	}
	rows, err = s.db.Query(ctx, `SELECT st.id, st.student_no, st.name, r.class_id,
		COALESCE(NULLIF(r.track, ''), m.track, ''), COALESCE(m.elective, ''),
		COALESCE(m.japanese, 0), COALESCE(m.walking, 0),
		COALESCE(NULLIF(m.status, ''), NULLIF(r.status, ''), 'active'), r.total_score
		FROM exam_rosters r
		JOIN students st ON st.id = r.student_id
		LEFT JOIN student_stage_memberships m ON m.student_id = r.student_id AND m.stage_id = ?
		WHERE r.exam_id = ? AND r.cohort_id = ?
		ORDER BY st.student_no`, stageID, context.Exam.ID, context.ExamCohort.CohortID)
	if err != nil {
		return BatchData{}, err
	}
	studentIndex := map[int64]int{}
	for _, row := range rows.Values {
		id := intValue(row[0])
		total, hasTotal := nullableFloat(row[9])
		var totalScore *float64
		if hasTotal {
			totalScore = total
		}
		studentIndex[id] = len(data.Students)
		data.Students = append(data.Students, Student{
			ID:             id,
			StudentNo:      stringValue(row[1]),
			Name:           stringValue(row[2]),
			ClassID:        intValue(row[3]),
			Track:          stringValue(row[4]),
			Elective:       stringValue(row[5]),
			Japanese:       intValue(row[6]) != 0,
			Walking:        intValue(row[7]) != 0,
			Status:         stringValue(row[8]),
			TotalScore:     totalScore,
			Scores:         map[string]float64{},
			ScoreFlags:     map[string]string{},
			TeachingGroups: map[string]int64{},
		})
	}
	rows, err = s.db.Query(ctx, `SELECT student_id, subject, score, flag FROM exam_scores
		WHERE exam_id = ? AND cohort_id = ? ORDER BY student_id, subject`,
		context.Exam.ID, context.ExamCohort.CohortID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		index, ok := studentIndex[intValue(row[0])]
		if !ok {
			continue
		}
		score, present := nullableFloat(row[2])
		if !present || score == nil {
			continue
		}
		subject := stringValue(row[1])
		data.Students[index].Scores[subject] = *score
		data.Students[index].ScoreFlags[subject] = stringValue(row[3])
	}
	rows, err = s.db.Query(ctx, `SELECT g.id, g.subject, g.name,
		COALESCE((SELECT m.class_id FROM student_teaching_groups stg
			JOIN student_stage_memberships m ON m.student_id = stg.student_id AND m.stage_id = stg.stage_id
			WHERE stg.teaching_group_id = g.id
			GROUP BY m.class_id ORDER BY COUNT(*) DESC, m.class_id LIMIT 1),
			(SELECT c.id FROM classes c WHERE c.cohort_id = ? AND c.name = g.name LIMIT 1), 0)
		FROM teaching_groups g WHERE g.stage_id = ? ORDER BY g.subject, g.name`,
		context.ExamCohort.CohortID, stageID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		data.TeachingGroups = append(data.TeachingGroups, TeachingGroup{
			ID:      intValue(row[0]),
			Subject: stringValue(row[1]),
			Name:    stringValue(row[2]),
			ClassID: intValue(row[3]),
		})
	}
	for index := range data.Students {
		for _, group := range data.TeachingGroups {
			if group.ClassID == data.Students[index].ClassID {
				data.Students[index].TeachingGroups[group.Subject] = group.ID
			}
		}
	}
	rows, err = s.db.Query(ctx, `SELECT student_id, subject, teaching_group_id
		FROM student_teaching_groups WHERE stage_id = ?`, stageID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		if index, ok := studentIndex[intValue(row[0])]; ok {
			data.Students[index].TeachingGroups[stringValue(row[1])] = intValue(row[2])
		}
	}
	groups, err := s.ListExamStatGroups(ctx, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	for _, group := range groups {
		target := 0.0
		if group.Target != nil {
			target = *group.Target
		}
		members := make([]int64, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, member.TeachingGroupID)
		}
		data.StatGroups = append(data.StatGroups, StatGroup{
			ID:              group.ID,
			Subject:         group.Subject,
			Name:            group.Name,
			LeadClassID:     group.LeadClassID,
			MemberGroupIDs:  members,
			Target:          target,
			BaselineAverage: group.BaselineAverage,
			BaselineRatio:   group.BaselineRatio,
		})
	}
	rows, err = s.db.Query(ctx, `SELECT a.teacher_id, t.name, a.teaching_group_id
		FROM assignments a JOIN teachers t ON t.id = a.teacher_id
		JOIN teaching_groups g ON g.id = a.teaching_group_id
		WHERE g.stage_id = ? ORDER BY a.id`, stageID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		data.Assignments = append(data.Assignments, Assignment{
			TeacherID:       intValue(row[0]),
			TeacherName:     stringValue(row[1]),
			TeachingGroupID: intValue(row[2]),
		})
	}
	targetIndex := map[int64]int{}
	rows, err = s.db.Query(ctx, `SELECT class_id, tier_key, target_count FROM assessment_class_targets
		WHERE assessment_id = ? ORDER BY class_id, tier_key`, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		classID := intValue(row[0])
		index, ok := targetIndex[classID]
		if !ok {
			index = len(data.ClassTargets)
			targetIndex[classID] = index
			data.ClassTargets = append(data.ClassTargets, ClassTarget{ClassID: classID, Counts: map[string]*float64{}})
		}
		value := floatValue(row[2])
		data.ClassTargets[index].Counts[stringValue(row[1])] = &value
	}
	rows, err = s.db.Query(ctx, `SELECT scope_type, scope_id, subject, kind, value
		FROM assessment_target_adjustments WHERE assessment_id = ? ORDER BY id`, assessmentID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		if stringValue(row[0]) != "class" {
			continue
		}
		classID := intValue(row[1])
		subject := stringValue(row[2])
		kind := stringValue(row[3])
		value, hasValue := nullableFloat(row[4])
		if kind == "exclusion" {
			data.ExcludedCompletionClasses[classID] = true
			continue
		}
		if subject == "" || !hasValue || value == nil {
			continue
		}
		index, ok := targetIndex[classID]
		if !ok {
			index = len(data.ClassTargets)
			targetIndex[classID] = index
			data.ClassTargets = append(data.ClassTargets, ClassTarget{ClassID: classID, Counts: map[string]*float64{}})
		}
		copied := *value
		data.ClassTargets[index].Counts[subject] = &copied
	}
	rows, err = s.db.Query(ctx, `SELECT a.scope_type, a.scope_id, a.subject, a.average, a.ratio
		FROM baseline_aggregates a JOIN baseline_sets b ON b.id = a.baseline_set_id
		WHERE a.stage_id = ? AND b.status = 'active' ORDER BY a.id`, stageID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		subject := stringValue(row[2])
		average := floatValue(row[3])
		ratio := floatValue(row[4])
		switch stringValue(row[0]) {
		case "class":
			if data.ClassBaselines[subject] == nil {
				data.ClassBaselines[subject] = map[string]float64{}
			}
			data.ClassBaselines[subject][strconv.FormatInt(intValue(row[1]), 10)] = average
			if ratio > 0 {
				if data.ClassBaselineRatios[subject] == nil {
					data.ClassBaselineRatios[subject] = map[string]float64{}
				}
				data.ClassBaselineRatios[subject][strconv.FormatInt(intValue(row[1]), 10)] = ratio
			}
		case "stage":
			data.GradeBaselines[subject] = average
		}
	}
	rows, err = s.db.Query(ctx, `SELECT subject, track, cutoff FROM exam_subject_cutoffs
		WHERE exam_id = ? AND cohort_id = ? ORDER BY subject, track`,
		context.Exam.ID, context.ExamCohort.CohortID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		data.SubjectCutoffs = append(data.SubjectCutoffs, SubjectCutoff{
			Subject: stringValue(row[0]), Track: stringValue(row[1]), Value: floatValue(row[2]),
		})
	}
	rows, err = s.db.Query(ctx, `SELECT tier_key, track, cutoff FROM exam_tier_cutoffs
		WHERE exam_id = ? AND cohort_id = ? ORDER BY tier_key, track`,
		context.Exam.ID, context.ExamCohort.CohortID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		data.TierCutoffs = append(data.TierCutoffs, TierCutoff{
			TierKey: stringValue(row[0]), Track: stringValue(row[1]), Value: floatValue(row[2]),
		})
	}
	return data, nil
}

// SaveAssessmentRun 保存一次核算结果。official 为 true 时是正式结果：
// 旧的正式运行清零，本次考核状态改为已核算；为 false 时是试算结果，
// 不影响本次考核的状态和当前正式结果。
func (s *Store) SaveAssessmentRun(ctx context.Context, assessmentID, versionID int64, inputHash string, inputSummary map[string]string, result calc.Result, official bool) (int64, error) {
	summaryJSON, err := json.Marshal(inputSummary)
	if err != nil {
		return 0, err
	}
	now := s.now().Format(time.RFC3339Nano)
	officialFlag := 0
	if official {
		officialFlag = 1
	}
	statements := []sqlite.Statement{}
	if official {
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE result_runs SET is_official = 0 WHERE assessment_id = ?`,
			Args: []any{assessmentID},
		})
	}
	statements = append(statements, sqlite.Statement{
		SQL: `INSERT INTO result_runs(batch_id, assessment_id, plan_version_id, input_hash,
			input_summary_json, is_official, created_at)
			VALUES (NULL, ?, ?, ?, ?, ?, ?)`,
		Args: []any{assessmentID, versionID, inputHash, string(summaryJSON), officialFlag, now},
	})
	transaction, err := s.db.Transaction(ctx, statements)
	if err != nil {
		return 0, err
	}
	runID := transaction[len(transaction)-1].LastInsertID
	rowStatements := make([]sqlite.Statement, 0, len(result.Rows)+len(result.Personal)+1)
	for _, row := range result.Rows {
		indicatorJSON, err := json.Marshal(row.IndicatorValues)
		if err != nil {
			return 0, err
		}
		participating := 0
		if row.CompletionParticipating {
			participating = 1
		}
		rowStatements = append(rowStatements, sqlite.Statement{
			SQL: `INSERT INTO result_rows(run_id, stat_group_id, stat_group_name, subject, lead_class_id,
				lead_class_name, teacher_names, class_count, class_average, grade_average, baseline_average,
				baseline_ratio, improvement_raw, improvement_score, pass_count, pass_target, pass_raw,
				pass_score, completion_actual, completion_base, completion_raw, completion_participate,
				completion_score, total, rank, tier_index, points, indicator_values_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{runID, row.StatGroupID, row.StatGroupName, row.Subject, row.LeadClassID,
				row.LeadClassName, joinNames(row.TeacherNames), row.ClassCount, row.ClassAverage,
				row.GradeAverage, row.BaselineAverage, row.BaselineRatio, row.ImprovementRaw,
				row.ImprovementScore, row.PassCount, row.PassTarget, row.PassRaw, row.PassScore,
				row.CompletionActual, row.CompletionBase, row.CompletionRaw, participating,
				row.CompletionScore, row.Total, row.Rank, row.TierIndex, row.Points, string(indicatorJSON)},
		})
	}
	for _, score := range result.Personal {
		pointsJSON, err := json.Marshal(score.GroupPoints)
		if err != nil {
			return 0, err
		}
		teacherID, err := strconv.ParseInt(score.TeacherID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("教师标识 %q 格式不正确", score.TeacherID)
		}
		rowStatements = append(rowStatements, sqlite.Statement{
			SQL: `INSERT INTO personal_scores(run_id, teacher_id, teacher_name, grade, subject, group_points_json, score)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{runID, teacherID, score.TeacherName, score.Grade, score.Subject, string(pointsJSON), score.Score},
		})
	}
	for _, row := range result.Rows {
		for position, participant := range row.Participants {
			rowStatements = append(rowStatements, sqlite.Statement{
				SQL: `INSERT INTO result_row_students(run_id, stat_group_id, subject, position,
					student_no, name) VALUES (?, ?, ?, ?, ?, ?)`,
				Args: []any{runID, row.StatGroupID, row.Subject, position,
					participant.StudentNo, participant.Name},
			})
		}
	}
	if official {
		rowStatements = append(rowStatements, sqlite.Statement{
			SQL: `UPDATE assessments SET status = 'calculated', plan_version_id = ?, current_run_id = ?,
				revision = revision + 1 WHERE id = ?`,
			Args: []any{versionID, runID, assessmentID},
		})
	}
	if _, err := s.db.Transaction(ctx, rowStatements); err != nil {
		return 0, err
	}
	return runID, nil
}

// AssessmentRunSummary 是一次核算的概要，供本次考核详情选择查看哪一份结果。
type AssessmentRunSummary struct {
	ID            int64
	PlanVersionID int64
	IsOfficial    bool
	CreatedAt     time.Time
	RowCount      int
}

// ListAssessmentRuns 返回本次考核的全部核算，最近的在前。
func (s *Store) ListAssessmentRuns(ctx context.Context, assessmentID int64) ([]AssessmentRunSummary, error) {
	rows, err := s.db.Query(ctx, `SELECT r.id, r.plan_version_id, r.is_official, r.created_at,
		(SELECT COUNT(*) FROM result_rows rr WHERE rr.run_id = r.id)
		FROM result_runs r WHERE r.assessment_id = ? ORDER BY r.id DESC`, assessmentID)
	if err != nil {
		return nil, err
	}
	runs := make([]AssessmentRunSummary, 0, len(rows.Values))
	for _, row := range rows.Values {
		createdAt, err := parseTime(row[3])
		if err != nil {
			return nil, err
		}
		runs = append(runs, AssessmentRunSummary{
			ID:            intValue(row[0]),
			PlanVersionID: intValue(row[1]),
			IsOfficial:    intValue(row[2]) != 0,
			CreatedAt:     createdAt,
			RowCount:      int(intValue(row[4])),
		})
	}
	return runs, nil
}

// ensureRunDetailColumns 为核算运行与结果行补上输入摘要、未封顶数值
// 和参与学生名单的列与表。
func (s *Store) ensureRunDetailColumns(ctx context.Context) error {
	exists, err := s.columnExists(ctx, "result_runs", "input_summary_json")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := s.db.Execute(ctx, `ALTER TABLE result_runs
			ADD COLUMN input_summary_json TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("增加结果运行的输入摘要: %w", err)
		}
	}
	columns := []struct {
		name string
		sql  string
	}{
		{"improvement_raw", `ALTER TABLE result_rows ADD COLUMN improvement_raw REAL NOT NULL DEFAULT 0`},
		{"pass_raw", `ALTER TABLE result_rows ADD COLUMN pass_raw REAL NOT NULL DEFAULT 0`},
		{"completion_actual", `ALTER TABLE result_rows ADD COLUMN completion_actual REAL NOT NULL DEFAULT 0`},
		{"completion_base", `ALTER TABLE result_rows ADD COLUMN completion_base REAL NOT NULL DEFAULT 0`},
		{"completion_raw", `ALTER TABLE result_rows ADD COLUMN completion_raw REAL NOT NULL DEFAULT 0`},
		{"completion_participate", `ALTER TABLE result_rows ADD COLUMN completion_participate INTEGER NOT NULL DEFAULT 0`},
	}
	for _, column := range columns {
		exists, err := s.columnExists(ctx, "result_rows", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := s.db.Execute(ctx, column.sql); err != nil {
			return fmt.Errorf("增加结果行的 %s 列: %w", column.name, err)
		}
	}
	if _, err := s.db.Execute(ctx, `CREATE TABLE IF NOT EXISTS result_row_students (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER NOT NULL,
		stat_group_id INTEGER NOT NULL,
		subject TEXT NOT NULL,
		position INTEGER NOT NULL,
		student_no TEXT NOT NULL,
		name TEXT NOT NULL
	)`); err != nil {
		return err
	}
	_, err = s.db.Execute(ctx, `CREATE INDEX IF NOT EXISTS result_row_students_row
		ON result_row_students(run_id, stat_group_id, subject)`)
	return err
}

// LatestOfficialAssessmentRun 返回本次考核最新的正式核算与结果行。
func (s *Store) LatestOfficialAssessmentRun(ctx context.Context, assessmentID int64) (ResultRun, []StoredResultRow, []StoredPersonalScore, error) {
	rows, err := s.db.Query(ctx, `SELECT id, COALESCE(batch_id, 0), plan_version_id, input_hash,
		COALESCE(input_summary_json, ''), is_official, created_at
		FROM result_runs WHERE assessment_id = ? AND is_official = 1
		ORDER BY id DESC LIMIT 1`, assessmentID)
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
	result, personal, err := s.assessmentRunRows(ctx, run.ID)
	return run, result, personal, err
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

// AssessmentRunDetail 返回指定核算的记录、结果行与个人得分。
func (s *Store) AssessmentRunDetail(ctx context.Context, assessmentID, runID int64) (ResultRun, []StoredResultRow, []StoredPersonalScore, error) {
	rows, err := s.db.Query(ctx, `SELECT id, COALESCE(batch_id, 0), plan_version_id, input_hash,
		COALESCE(input_summary_json, ''), is_official, created_at
		FROM result_runs WHERE assessment_id = ? AND id = ?`, assessmentID, runID)
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
	result, personal, err := s.assessmentRunRows(ctx, run.ID)
	return run, result, personal, err
}

func (s *Store) assessmentRunRows(ctx context.Context, runID int64) ([]StoredResultRow, []StoredPersonalScore, error) {
	resultRows, err := s.db.Query(ctx, `SELECT id, stat_group_id, stat_group_name, subject, lead_class_id,
		lead_class_name, teacher_names, class_count, class_average, grade_average, baseline_average,
		baseline_ratio, improvement_raw, improvement_score, pass_count, pass_target, pass_raw,
		pass_score, completion_actual, completion_base, completion_raw, completion_participate,
		completion_score, total, rank, tier_index, points, indicator_values_json
		FROM result_rows WHERE run_id = ? ORDER BY subject, rank, stat_group_name`, runID)
	if err != nil {
		return nil, nil, err
	}
	result := make([]StoredResultRow, 0, len(resultRows.Values))
	for _, row := range resultRows.Values {
		result = append(result, StoredResultRow{
			ID:                    intValue(row[0]),
			StatGroupID:           intValue(row[1]),
			StatGroupName:         stringValue(row[2]),
			Subject:               stringValue(row[3]),
			LeadClassID:           intValue(row[4]),
			LeadClassName:         stringValue(row[5]),
			TeacherNames:          stringValue(row[6]),
			ClassCount:            int(intValue(row[7])),
			ClassAverage:          floatValue(row[8]),
			GradeAverage:          floatValue(row[9]),
			BaselineAverage:       floatValue(row[10]),
			BaselineRatio:         floatValue(row[11]),
			ImprovementRaw:        floatValue(row[12]),
			ImprovementScore:      floatValue(row[13]),
			PassCount:             int(intValue(row[14])),
			PassTarget:            floatValue(row[15]),
			PassRaw:               floatValue(row[16]),
			PassScore:             floatValue(row[17]),
			CompletionActual:      floatValue(row[18]),
			CompletionBase:        floatValue(row[19]),
			CompletionRaw:         floatValue(row[20]),
			CompletionParticipate: intValue(row[21]) != 0,
			CompletionScore:       floatValue(row[22]),
			Total:                 floatValue(row[23]),
			Rank:                  int(intValue(row[24])),
			TierIndex:             int(intValue(row[25])),
			Points:                floatValue(row[26]),
			IndicatorJSON:         stringValue(row[27]),
		})
	}
	personalRows, err := s.db.Query(ctx, `SELECT teacher_id, teacher_name, grade, subject,
		group_points_json, score FROM personal_scores WHERE run_id = ?
		ORDER BY subject, teacher_name`, runID)
	if err != nil {
		return nil, nil, err
	}
	personal := make([]StoredPersonalScore, 0, len(personalRows.Values))
	for _, row := range personalRows.Values {
		personal = append(personal, StoredPersonalScore{
			TeacherID:   intValue(row[0]),
			TeacherName: stringValue(row[1]),
			Grade:       stringValue(row[2]),
			Subject:     stringValue(row[3]),
			GroupPoints: stringValue(row[4]),
			Score:       floatValue(row[5]),
		})
	}
	return result, personal, nil
}

// ResultRowStudent 是参与一条结果行计算的学生快照。
type ResultRowStudent struct {
	Position  int
	StudentNo string
	Name      string
}

// AssessmentRowStudents 返回一条结果行参与计算的学生名单。
func (s *Store) AssessmentRowStudents(ctx context.Context, runID, statGroupID int64, subject string) ([]ResultRowStudent, error) {
	rows, err := s.db.Query(ctx, `SELECT position, student_no, name FROM result_row_students
		WHERE run_id = ? AND stat_group_id = ? AND subject = ?
		ORDER BY position`, runID, statGroupID, subject)
	if err != nil {
		return nil, err
	}
	students := make([]ResultRowStudent, 0, len(rows.Values))
	for _, row := range rows.Values {
		students = append(students, ResultRowStudent{
			Position:  int(intValue(row[0])),
			StudentNo: stringValue(row[1]),
			Name:      stringValue(row[2]),
		})
	}
	return students, nil
}

// AdjustAssessmentTarget 记录一次目标补录，后续核算按补录值计算。
func (s *Store) AdjustAssessmentTarget(ctx context.Context, assessmentID, classID int64, tierKey string, value float64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("补录原因不能为空")
	}
	if value < 0 {
		return fmt.Errorf("目标人数不能小于零")
	}
	stageID, err := s.assessmentStage(ctx, assessmentID)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM class_stage_info WHERE stage_id = ? AND class_id = ?`,
		stageID, classID)
	if err != nil {
		return err
	}
	if len(rows.Values) == 0 || intValue(rows.Values[0][0]) == 0 {
		return fmt.Errorf("班级不属于本次考核对应的阶段")
	}
	_, err = s.db.Execute(ctx, `INSERT INTO assessment_target_adjustments(assessment_id, scope_type,
		scope_id, subject, kind, reason, value, created_at) VALUES (?, 'class', ?, ?, 'adjustment', ?, ?, ?)`,
		assessmentID, classID, tierKey, reason, value, s.now().Format(time.RFC3339Nano))
	return err
}

// ExcludeAssessmentClass 记录一个班级不参与完成率计算。
func (s *Store) ExcludeAssessmentClass(ctx context.Context, assessmentID, classID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("不参与的原因不能为空")
	}
	if _, err := s.assessmentStage(ctx, assessmentID); err != nil {
		return err
	}
	_, err := s.db.Execute(ctx, `INSERT INTO assessment_target_adjustments(assessment_id, scope_type,
		scope_id, subject, kind, reason, value, created_at) VALUES (?, 'class', ?, '', 'exclusion', ?, NULL, ?)`,
		assessmentID, classID, reason, s.now().Format(time.RFC3339Nano))
	return err
}

// EnsureAssessmentStatGroups 为本次考核补齐默认合并组。
func (s *Store) EnsureAssessmentStatGroups(ctx context.Context, assessmentID int64) error {
	stageID, err := s.assessmentStage(ctx, assessmentID)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM assessment_stat_groups WHERE assessment_id = ?`, assessmentID)
	if err != nil {
		return err
	}
	if len(rows.Values) > 0 && intValue(rows.Values[0][0]) > 0 {
		return nil
	}
	rows, err = s.db.Query(ctx, `SELECT g.id, g.subject, g.name,
		COALESCE((SELECT m.class_id FROM student_teaching_groups stg
			JOIN student_stage_memberships m ON m.student_id = stg.student_id AND m.stage_id = stg.stage_id
			WHERE stg.teaching_group_id = g.id
			GROUP BY m.class_id ORDER BY COUNT(*) DESC, m.class_id LIMIT 1),
			(SELECT c.id FROM classes c WHERE c.cohort_id = ? AND c.name = g.name LIMIT 1), 0)
		FROM teaching_groups g WHERE g.stage_id = ? ORDER BY g.subject, g.name`, stageID, stageID)
	if err != nil {
		return err
	}
	if len(rows.Values) == 0 {
		return nil
	}
	var statements []sqlite.Statement
	for _, row := range rows.Values {
		subject := stringValue(row[1])
		name := stringValue(row[2])
		leadClass := intValue(row[3])
		if leadClass == 0 {
			continue
		}
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO assessment_stat_groups(assessment_id, subject, name,
					lead_class_id, baseline_average, baseline_ratio, baseline_source, explicit)
					SELECT ?, ?, ?, ?,
						COALESCE((SELECT a.average FROM baseline_aggregates a
							JOIN baseline_sets b ON b.id = a.baseline_set_id
							WHERE a.stage_id = ? AND a.subject = ? AND a.scope_type = 'class'
								AND a.scope_id = ? AND b.status = 'active' ORDER BY a.id LIMIT 1),
							COALESCE((SELECT a.average FROM baseline_aggregates a
								JOIN baseline_sets b ON b.id = a.baseline_set_id
								WHERE a.stage_id = ? AND a.subject = ? AND a.scope_type = 'stage'
									AND b.status = 'active' ORDER BY a.id LIMIT 1), 0)),
						COALESCE((SELECT a.ratio FROM baseline_aggregates a
							JOIN baseline_sets b ON b.id = a.baseline_set_id
							WHERE a.stage_id = ? AND a.subject = ? AND a.scope_type = 'class'
								AND a.scope_id = ? AND b.status = 'active' ORDER BY a.id LIMIT 1), 0),
						'baseline', 0`,
				Args: []any{assessmentID, subject, name, leadClass,
					stageID, subject, leadClass, stageID, subject, stageID, subject, leadClass},
			},
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO assessment_stat_group_members(stat_group_id, teaching_group_id)
					SELECT sg.id, g.id FROM assessment_stat_groups sg, teaching_groups g
					WHERE sg.assessment_id = ? AND sg.explicit = 0 AND sg.subject = ?
						AND sg.name = ? AND g.stage_id = ? AND g.subject = sg.subject AND g.name = sg.name`,
				Args: []any{assessmentID, subject, name, stageID},
			},
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO assessment_stat_group_targets(stat_group_id, target_count)
					SELECT sg.id, COALESCE(
						(SELECT t.target_count FROM assessment_class_targets t
							WHERE t.assessment_id = ? AND t.class_id = sg.lead_class_id AND t.tier_key = 'single_subject'),
						(SELECT t.target_count FROM assessment_class_targets t
							WHERE t.assessment_id = ? AND t.class_id = sg.lead_class_id AND t.tier_key = 'undergraduate'),
						0)
					FROM assessment_stat_groups sg
					WHERE sg.assessment_id = ? AND sg.explicit = 0`,
				Args: []any{assessmentID, assessmentID, assessmentID},
			})
	}
	_, err = s.db.Transaction(ctx, statements)
	return err
}

// AssessmentRunExists 判断本次考核是否已有核算结果。
func (s *Store) AssessmentRunExists(ctx context.Context, assessmentID int64) (bool, error) {
	rows, err := s.db.Query(ctx, `SELECT 1 FROM result_runs WHERE assessment_id = ? LIMIT 1`, assessmentID)
	if err != nil {
		return false, err
	}
	return len(rows.Values) > 0, nil
}

// ErrAssessmentNotCalculated 表示本次考核还没有核算结果。
var ErrAssessmentNotCalculated = errors.New("本次考核还没有核算结果")

// AssessmentOverview 是考核列表中的一项，带考试、级与准备数量。
type AssessmentOverview struct {
	AssessmentID      int64
	ExamID            int64
	ExamName          string
	ExamType          string
	Term              string
	ExamDate          string
	AcademicYearStart int
	CohortID          int64
	CohortName        string
	GradeLevel        string
	GraduationYear    int
	ExamCohortID      int64
	Status            string
	RosterCount       int
	ScoreCount        int
	TargetCount       int
	GroupCount        int
	CutoffCount       int
	Blocking          int
	Warnings          int
}

// ListAssessmentOverview 返回全部考试与级的考核概况，最近的考试在前。
func (s *Store) ListAssessmentOverview(ctx context.Context) ([]AssessmentOverview, error) {
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
		ORDER BY e.academic_year_start DESC, e.exam_date DESC, e.id DESC, c.name`)
	if err != nil {
		return nil, err
	}
	items := make([]AssessmentOverview, 0, len(rows.Values))
	for _, row := range rows.Values {
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
	return items, nil
}
