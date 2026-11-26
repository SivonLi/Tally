package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"tally/internal/school"
)

// decodeTeacherNames 解析数据库中保存的任课教师名单（兼容 JSON 数组与顿号分隔文本）。
func decodeTeacherNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		names := make([]string, 0, len(parsed))
		for _, item := range parsed {
			item = strings.TrimSpace(item)
			if item != "" {
				names = append(names, item)
			}
		}
		return names
	}
	parts := strings.Split(raw, "、")
	names := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item != "" {
			names = append(names, item)
		}
	}
	return names
}

// TeacherTrendPoint 记录教师在单场考试/考核中的得分明细。
type TeacherTrendPoint struct {
	ExamID            int64             `json:"examId"`
	AssessmentID      int64             `json:"assessmentId"`
	ExamName          string            `json:"examName"`
	ExamDate          string            `json:"examDate"`
	AcademicYear      string            `json:"academicYear"`
	Semester          string            `json:"semester"`
	AcademicYearStart int               `json:"academicYearStart,omitempty"`
	Term              string            `json:"term,omitempty"`
	ExamType          string            `json:"examType,omitempty"`
	GradeLevel        string            `json:"gradeLevel,omitempty"`
	Subject           string            `json:"subject,omitempty"`
	GroupPoints       []float64         `json:"groupPoints,omitempty"`
	PlanVersionID     int64             `json:"planVersionId"`
	PersonalScore     float64           `json:"personalScore"`
	Rows              []TeacherTrendRow `json:"rows"`
}

// TeacherTrendRow 教师在某个教学/统计组中的考核结果。
type TeacherTrendRow struct {
	StatGroupID      int64   `json:"statGroupId"`
	StatGroupName    string  `json:"statGroupName"`
	LeadClassName    string  `json:"leadClassName"`
	Subject          string  `json:"subject"`
	ClassAverage     float64 `json:"classAverage"`
	GradeAverage     float64 `json:"gradeAverage"`
	BaselineAverage  float64 `json:"baselineAverage"`
	BaselineRatio    float64 `json:"baselineRatio"`
	ImprovementScore float64 `json:"improvementScore"`
	PassRateScore    float64 `json:"passRateScore"`
	CompletionScore  float64 `json:"completionScore"`
	Total            float64 `json:"total"`
	Rank             int     `json:"rank"`
	TierIndex        int     `json:"tierIndex"`
	Tier             string  `json:"tier"`
	Points           float64 `json:"points"`
}

// TeacherTrendResult 包含教师在历次考试中的变化趋势。
type TeacherTrendResult struct {
	TeacherName          string              `json:"teacherName"`
	Subject              string              `json:"subject"`
	Points               []TeacherTrendPoint `json:"points"`
	AveragePersonalScore float64             `json:"averagePersonalScore"`
	LatestPersonalScore  float64             `json:"latestPersonalScore"`
	HasMultiplePlans     bool                `json:"hasMultiplePlans"`
}

// ClassTrendPoint 记录班级在单场考试/考核中的得分明细。
type ClassTrendPoint struct {
	ExamID          int64           `json:"examId"`
	AssessmentID    int64           `json:"assessmentId"`
	ExamName        string          `json:"examName"`
	ExamDate        string          `json:"examDate"`
	AcademicYear    string          `json:"academicYear"`
	Semester        string          `json:"semester"`
	CompletionScore float64         `json:"completionScore"`
	Rows            []ClassTrendRow `json:"rows"`
}

// ClassTrendRow 班级在特定学科的结果。
type ClassTrendRow struct {
	Subject          string   `json:"subject"`
	StatGroupName    string   `json:"statGroupName"`
	ClassAverage     float64  `json:"classAverage"`
	GradeAverage     float64  `json:"gradeAverage"`
	BaselineAverage  float64  `json:"baselineAverage"`
	BaselineRatio    float64  `json:"baselineRatio"`
	ImprovementScore float64  `json:"improvementScore"`
	PassRateScore    float64  `json:"passRateScore"`
	CompletionScore  float64  `json:"completionScore"`
	Total            float64  `json:"total"`
	Rank             int      `json:"rank"`
	TierIndex        int      `json:"tierIndex"`
	Tier             string   `json:"tier"`
	Points           float64  `json:"points"`
	TeacherNames     []string `json:"teacherNames"`
}

// ClassTrendResult 包含班级在历次考试中的变化趋势。
type ClassTrendResult struct {
	ClassID                int64             `json:"classId"`
	ClassName              string            `json:"className"`
	Points                 []ClassTrendPoint `json:"points"`
	AverageCompletionScore float64           `json:"averageCompletionScore"`
}

// SubjectComparisonGroup 单个统计组在学科对比中的数据。
type SubjectComparisonGroup struct {
	StatGroupID      int64    `json:"statGroupId"`
	StatGroupName    string   `json:"statGroupName"`
	LeadClassName    string   `json:"leadClassName"`
	TeacherNames     []string `json:"teacherNames"`
	ClassAverage     float64  `json:"classAverage"`
	GradeAverage     float64  `json:"gradeAverage"`
	BaselineAverage  float64  `json:"baselineAverage"`
	BaselineRatio    float64  `json:"baselineRatio"`
	ImprovementScore float64  `json:"improvementScore"`
	PassRateScore    float64  `json:"passRateScore"`
	CompletionScore  float64  `json:"completionScore"`
	Total            float64  `json:"total"`
	Rank             int      `json:"rank"`
	TierIndex        int      `json:"tierIndex"`
	Tier             string   `json:"tier"`
	Points           float64  `json:"points"`
}

// SubjectComparisonResult 单次考核内同科各班/统计组的横向对比。
type SubjectComparisonResult struct {
	AssessmentID       int64                    `json:"assessmentId"`
	ExamName           string                   `json:"examName"`
	Subject            string                   `json:"subject"`
	GroupCount         int                      `json:"groupCount"`
	AverageScore       float64                  `json:"averageScore"`
	AverageImprovement float64                  `json:"averageImprovement"`
	AveragePassRate    float64                  `json:"averagePassRate"`
	TierCounts         map[string]int           `json:"tierCounts"`
	Groups             []SubjectComparisonGroup `json:"groups"`
}

// StudentTrackPoint 单场考试中学生的成绩与排名快照。
type StudentTrackPoint struct {
	ExamID          int64              `json:"examId"`
	ExamName        string             `json:"examName"`
	ExamDate        string             `json:"examDate"`
	AcademicYear    string             `json:"academicYear"`
	Semester        string             `json:"semester"`
	PhaseAtExam     string             `json:"phaseAtExam,omitempty"`
	ClassName       string             `json:"className,omitempty"`
	TotalScore      float64            `json:"totalScore"`
	GradeRank       int                `json:"gradeRank"`
	TotalStudents   int                `json:"totalStudents"`
	TrackRank       int                `json:"trackRank,omitempty"`
	TrackTotalCount int                `json:"trackTotalCount,omitempty"`
	ClassRank       int                `json:"classRank,omitempty"`
	ClassTotalCount int                `json:"classTotalCount,omitempty"`
	Scores          map[string]float64 `json:"scores"`
	Flags           map[string]string  `json:"flags"`
	SubjectRanks    map[string]int     `json:"subjectRanks,omitempty"`
}

// StudentTrackResult 学生在历次考试中的成绩追踪。
type StudentTrackResult struct {
	StudentID   int64               `json:"studentId"`
	StudentNo   string              `json:"studentNo"`
	StudentName string              `json:"studentName"`
	ClassName   string              `json:"className"`
	Track       string              `json:"track"`
	Points      []StudentTrackPoint `json:"points"`
}

// AssessmentComparisonItem 一次考核的核心汇总。
type AssessmentComparisonItem struct {
	AssessmentID         int64     `json:"assessmentId"`
	ExamID               int64     `json:"examId"`
	ExamName             string    `json:"examName"`
	ExamDate             string    `json:"examDate"`
	CohortEntryYear      int       `json:"cohortEntryYear"`
	AcademicYear         string    `json:"academicYear"`
	Semester             string    `json:"semester"`
	PlanVersionID        int64     `json:"planVersionId"`
	PlanConfigHash       string    `json:"planConfigHash"`
	ParticipantCount     int       `json:"participantCount"`
	GroupCount           int       `json:"groupCount"`
	AverageScore         float64   `json:"averageScore"`
	AverageClass         float64   `json:"averageClass"`
	AverageImprovement   float64   `json:"averageImprovement"`
	PassTotal            int       `json:"passTotal"`
	OfficialRunCreatedAt time.Time `json:"officialRunCreatedAt"`
}

// AssessmentComparisonResult 2~5 次考核并排对比结果。
type AssessmentComparisonResult struct {
	Items           []AssessmentComparisonItem `json:"items"`
	PlanDifferent   bool                       `json:"planDifferent"`
	SubjectAverages map[string][]float64       `json:"subjectAverages"`
}

// QueryTeacherTrends 查询指定教师在某届（级）历次考核中的表现趋势。
func (s *Store) QueryTeacherTrends(ctx context.Context, cohortID int64, teacherName string, examIDs []int64) (TeacherTrendResult, error) {
	teacherName = strings.TrimSpace(teacherName)
	result := TeacherTrendResult{TeacherName: teacherName}
	if teacherName == "" {
		return result, nil
	}
	where := []string{"ps.cohort_id = ?", "ps.teacher_name_snapshot = ?", "rr.is_official = 1", "a.current_run_id = rr.id"}
	args := []any{cohortID, teacherName}
	if len(examIDs) > 0 {
		where = append(where, "e.id IN ("+placeholders(len(examIDs))+")")
		for _, id := range examIDs {
			args = append(args, id)
		}
	}
	rows, err := s.db.Query(ctx, `SELECT e.id, a.id, e.name, e.exam_date, e.academic_year_start, e.term,
		rr.id, rr.plan_version_id, ps.subject, ps.score, ps.teacher_id
		FROM personal_scores ps
		JOIN result_runs rr ON rr.id = ps.run_id
		JOIN assessments a ON a.id = rr.assessment_id
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY e.academic_year_start ASC, e.exam_date ASC, e.sequence ASC, e.id ASC`, args...)
	if err != nil {
		return result, err
	}

	planIDs := map[int64]bool{}
	var sum float64
	for _, r := range rows.Values {
		examID := intValue(r[0])
		assessID := intValue(r[1])
		runID := intValue(r[6])
		planID := intValue(r[7])
		planIDs[planID] = true
		subj := stringValue(r[8])
		score := floatValue(r[9])
		teacherID := intValue(r[10])
		if result.Subject == "" {
			result.Subject = subj
		}
		sum += score

		rrRows, err := s.db.Query(ctx, `SELECT rr.stat_group_id, rr.stat_group_name, rr.lead_class_name, rr.subject,
			rr.class_average, rr.grade_average, rr.baseline_average, rr.baseline_ratio,
			rr.improvement_score, rr.pass_score, rr.completion_score, rr.total_score,
			rr.rank, rr.tier_index, rr.points
			FROM result_rows rr
			JOIN result_row_teachers rrt ON rrt.result_row_id = rr.id
			WHERE rr.run_id = ? AND rrt.teacher_id = ?
			ORDER BY rr.subject, rr.stat_group_name`, runID, teacherID)
		if err != nil {
			return result, err
		}
		var trendRows []TeacherTrendRow
		for _, rr := range rrRows.Values {
			tierIdx := int(intValue(rr[13]))
			trendRows = append(trendRows, TeacherTrendRow{
				StatGroupID:      intValue(rr[0]),
				StatGroupName:    stringValue(rr[1]),
				LeadClassName:    stringValue(rr[2]),
				Subject:          stringValue(rr[3]),
				ClassAverage:     floatValue(rr[4]),
				GradeAverage:     floatValue(rr[5]),
				BaselineAverage:  floatValue(rr[6]),
				BaselineRatio:    floatValue(rr[7]),
				ImprovementScore: floatValue(rr[8]),
				PassRateScore:    floatValue(rr[9]),
				CompletionScore:  floatValue(rr[10]),
				Total:            floatValue(rr[11]),
				Rank:             int(intValue(rr[12])),
				TierIndex:        tierIdx,
				Tier:             tierLabel(tierIdx),
				Points:           floatValue(rr[14]),
			})
		}

		yearStart := int(intValue(r[4]))
		term := stringValue(r[5])
		result.Points = append(result.Points, TeacherTrendPoint{
			ExamID:            examID,
			AssessmentID:      assessID,
			ExamName:          stringValue(r[2]),
			ExamDate:          stringValue(r[3]),
			AcademicYear:      school.AcademicYearLabel(yearStart),
			Semester:          term,
			AcademicYearStart: yearStart,
			Term:              term,
			Subject:           subj,
			PlanVersionID:     planID,
			PersonalScore:     score,
			Rows:              trendRows,
		})
	}
	if len(result.Points) > 0 {
		result.AveragePersonalScore = sum / float64(len(result.Points))
		result.LatestPersonalScore = result.Points[len(result.Points)-1].PersonalScore
	}
	result.HasMultiplePlans = len(planIDs) > 1
	return result, nil
}

// QueryClassTrends 查询指定班级在某届（级）历次考核中的完成率及各科指标趋势。
func (s *Store) QueryClassTrends(ctx context.Context, cohortID int64, classID int64, className string, examIDs []int64) (ClassTrendResult, error) {
	classIDs := []int64{}
	if classID > 0 {
		classIDs = append(classIDs, classID)
	}
	return s.queryClassTrendsByIDs(ctx, cohortID, classIDs, className, examIDs, false)
}

func (s *Store) queryClassTrendsByIDs(ctx context.Context, cohortID int64, classIDs []int64, className string, examIDs []int64, strictByID bool) (ClassTrendResult, error) {
	resolvedID := int64(0)
	if len(classIDs) > 0 {
		resolvedID = classIDs[len(classIDs)-1]
	}
	result := ClassTrendResult{ClassID: resolvedID, ClassName: normalizeClassName(className)}
	if result.ClassName == "" && resolvedID > 0 {
		rows, err := s.db.Query(ctx, `SELECT name FROM classes WHERE id = ?`, resolvedID)
		if err == nil && len(rows.Values) > 0 {
			result.ClassName = stringValue(rows.Values[0][0])
		}
	}

	where := []string{"ec.cohort_id = ?", "rr.is_official = 1", "a.current_run_id = rr.id"}
	args := []any{cohortID}
	if len(examIDs) > 0 {
		where = append(where, "e.id IN ("+placeholders(len(examIDs))+")")
		for _, id := range examIDs {
			args = append(args, id)
		}
	}

	var classMatchSQL string
	if len(classIDs) > 0 {
		classMatchSQL = "rrow.lead_class_id IN (" + placeholders(len(classIDs)) + ")"
		for _, id := range classIDs {
			args = append(args, id)
		}
		if !strictByID && result.ClassName != "" {
			classMatchSQL = "(" + classMatchSQL + " OR rrow.lead_class_name = ?)"
			args = append(args, result.ClassName)
		}
	} else if result.ClassName != "" {
		classMatchSQL = "rrow.lead_class_name = ?"
		args = append(args, result.ClassName)
	} else {
		return result, nil
	}
	where = append(where, classMatchSQL)

	rows, err := s.db.Query(ctx, `SELECT e.id, a.id, e.name, e.exam_date, e.academic_year_start, e.term,
		rrow.id, rrow.subject, rrow.stat_group_name, rrow.class_average, rrow.grade_average,
		rrow.baseline_average, rrow.baseline_ratio, rrow.improvement_score, rrow.pass_score,
		rrow.completion_score, rrow.total_score, rrow.rank, rrow.tier_index, rrow.points
		FROM result_rows rrow
		JOIN result_runs rr ON rr.id = rrow.run_id
		JOIN assessments a ON a.id = rr.assessment_id
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY e.academic_year_start ASC, e.exam_date ASC, e.sequence ASC, e.id ASC, rrow.subject`, args...)
	if err != nil {
		return result, err
	}

	pointIndex := map[int64]int{}
	for _, r := range rows.Values {
		assessID := intValue(r[1])
		idx, ok := pointIndex[assessID]
		if !ok {
			idx = len(result.Points)
			pointIndex[assessID] = idx
			result.Points = append(result.Points, ClassTrendPoint{
				ExamID:       intValue(r[0]),
				AssessmentID: assessID,
				ExamName:     stringValue(r[2]),
				ExamDate:     stringValue(r[3]),
				AcademicYear: school.AcademicYearLabel(int(intValue(r[4]))),
				Semester:     stringValue(r[5]),
			})
		}
		rrID := intValue(r[6])
		tNames := s.loadResultRowTeacherNames(ctx, rrID)
		tierIdx := int(intValue(r[18]))
		compScore := floatValue(r[15])
		row := ClassTrendRow{
			Subject:          stringValue(r[7]),
			StatGroupName:    stringValue(r[8]),
			ClassAverage:     floatValue(r[9]),
			GradeAverage:     floatValue(r[10]),
			BaselineAverage:  floatValue(r[11]),
			BaselineRatio:    floatValue(r[12]),
			ImprovementScore: floatValue(r[13]),
			PassRateScore:    floatValue(r[14]),
			CompletionScore:  compScore,
			Total:            floatValue(r[16]),
			Rank:             int(intValue(r[17])),
			TierIndex:        tierIdx,
			Tier:             tierLabel(tierIdx),
			Points:           floatValue(r[19]),
			TeacherNames:     tNames,
		}
		result.Points[idx].Rows = append(result.Points[idx].Rows, row)
	}

	var compSum float64
	compCount := 0
	for i := range result.Points {
		if len(result.Points[i].Rows) == 0 {
			continue
		}
		var ptSum float64
		for _, row := range result.Points[i].Rows {
			ptSum += row.CompletionScore
		}
		result.Points[i].CompletionScore = ptSum / float64(len(result.Points[i].Rows))
		compSum += result.Points[i].CompletionScore
		compCount++
	}
	if compCount > 0 {
		result.AverageCompletionScore = compSum / float64(compCount)
	}
	return result, nil
}

// QuerySubjectComparison 查询单次考核某个学科下全部统计组的横向对比。
func (s *Store) QuerySubjectComparison(ctx context.Context, assessmentID int64, subject string) (SubjectComparisonResult, error) {
	ac, err := s.LoadAssessmentContext(ctx, assessmentID)
	if err != nil {
		return SubjectComparisonResult{}, err
	}
	res := SubjectComparisonResult{
		AssessmentID: assessmentID,
		ExamName:     ac.Exam.Name,
		Subject:      subject,
		TierCounts:   map[string]int{},
	}
	run, rows, _, err := s.LatestOfficialAssessmentRun(ctx, assessmentID)
	if err != nil {
		return res, nil
	}
	_ = run

	var sumTotal, sumImp, sumPass float64
	for _, r := range rows {
		if subject != "" && r.Subject != subject {
			continue
		}
		tLabel := tierLabel(r.TierIndex)
		res.TierCounts[tLabel]++
		sumTotal += r.Total
		sumImp += r.ImprovementScore
		sumPass += r.PassScore
		res.Groups = append(res.Groups, SubjectComparisonGroup{
			StatGroupID:      r.StatGroupID,
			StatGroupName:    r.StatGroupName,
			LeadClassName:    r.LeadClassName,
			TeacherNames:     decodeTeacherNames(r.TeacherNames),
			ClassAverage:     r.ClassAverage,
			GradeAverage:     r.GradeAverage,
			BaselineAverage:  r.BaselineAverage,
			BaselineRatio:    r.BaselineRatio,
			ImprovementScore: r.ImprovementScore,
			PassRateScore:    r.PassScore,
			CompletionScore:  r.CompletionScore,
			Total:            r.Total,
			Rank:             r.Rank,
			TierIndex:        r.TierIndex,
			Tier:             tLabel,
			Points:           r.Points,
		})
	}
	res.GroupCount = len(res.Groups)
	if res.GroupCount > 0 {
		res.AverageScore = sumTotal / float64(res.GroupCount)
		res.AverageImprovement = sumImp / float64(res.GroupCount)
		res.AveragePassRate = sumPass / float64(res.GroupCount)
	}
	return res, nil
}

// QueryStudentTracking 查询学生在所属年级历次考试的成绩与全年级/选科/班级排名走势。
func (s *Store) QueryStudentTracking(ctx context.Context, cohortID int64, studentID int64, studentNo string) (StudentTrackResult, error) {
	var stuRow []any
	if studentID > 0 {
		rows, err := s.db.Query(ctx, `SELECT st.id, st.student_no, st.name, COALESCE(c.name, ''), st.current_track
			FROM students st LEFT JOIN classes c ON c.id = st.current_class_id
			WHERE st.id = ?`, studentID)
		if err != nil {
			return StudentTrackResult{}, err
		}
		if len(rows.Values) > 0 {
			stuRow = rows.Values[0]
		}
	} else if strings.TrimSpace(studentNo) != "" {
		rows, err := s.db.Query(ctx, `SELECT st.id, st.student_no, st.name, COALESCE(c.name, ''), st.current_track
			FROM students st LEFT JOIN classes c ON c.id = st.current_class_id
			WHERE st.cohort_id = ? AND st.student_no = ? LIMIT 1`, cohortID, strings.TrimSpace(studentNo))
		if err != nil {
			return StudentTrackResult{}, err
		}
		if len(rows.Values) > 0 {
			stuRow = rows.Values[0]
		}
	}
	if stuRow == nil {
		return StudentTrackResult{}, nil
	}
	sid := intValue(stuRow[0])
	out := StudentTrackResult{
		StudentID:   sid,
		StudentNo:   stringValue(stuRow[1]),
		StudentName: stringValue(stuRow[2]),
		ClassName:   stringValue(stuRow[3]),
		Track:       stringValue(stuRow[4]),
	}

	pRows, err := s.db.Query(ctx, `SELECT ep.id, e.id, e.name, e.exam_date, e.academic_year_start, e.term,
		ec.phase_at_exam, ep.class_name_snapshot, ep.total_score,
		ep.grade_rank, ep.grade_total_count, ep.track_rank, ep.track_total_count,
		ep.class_rank, ep.class_total_count
		FROM exam_participants ep
		JOIN exam_cohorts ec ON ec.id = ep.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		WHERE ep.student_id = ?
		ORDER BY e.academic_year_start ASC, e.exam_date ASC, e.sequence ASC, e.id ASC`, sid)
	if err != nil {
		return StudentTrackResult{}, err
	}
	for _, r := range pRows.Values {
		epID := intValue(r[0])
		pt := StudentTrackPoint{
			ExamID:          intValue(r[1]),
			ExamName:        stringValue(r[2]),
			ExamDate:        stringValue(r[3]),
			AcademicYear:    school.AcademicYearLabel(int(intValue(r[4]))),
			Semester:        stringValue(r[5]),
			PhaseAtExam:     stringValue(r[6]),
			ClassName:       stringValue(r[7]),
			TotalScore:      floatValue(r[8]),
			GradeRank:       int(intValue(r[9])),
			TotalStudents:   int(intValue(r[10])),
			TrackRank:       int(intValue(r[11])),
			TrackTotalCount: int(intValue(r[12])),
			ClassRank:       int(intValue(r[13])),
			ClassTotalCount: int(intValue(r[14])),
			Scores:          map[string]float64{},
			Flags:           map[string]string{},
			SubjectRanks:    map[string]int{},
		}
		scRows, err := s.db.Query(ctx, `SELECT subject, score, flag, subject_grade_rank
			FROM exam_scores WHERE exam_participant_id = ?`, epID)
		if err != nil {
			return StudentTrackResult{}, err
		}
		for _, sr := range scRows.Values {
			subj := stringValue(sr[0])
			if val, ok := nullableFloat(sr[1]); ok && val != nil {
				pt.Scores[subj] = *val
			}
			if fl := stringValue(sr[2]); fl != "" {
				pt.Flags[subj] = fl
			}
			if rk, ok := nullableInt64(sr[3]); ok && rk > 0 {
				pt.SubjectRanks[subj] = int(rk)
			}
		}
		out.Points = append(out.Points, pt)
	}
	return out, nil
}

// QueryAssessmentComparison 对比 2~5 次考核的整体均分、达线与方案版本差异。
func (s *Store) QueryAssessmentComparison(ctx context.Context, assessmentIDs []int64) (AssessmentComparisonResult, error) {
	res := AssessmentComparisonResult{SubjectAverages: map[string][]float64{}}
	if len(assessmentIDs) == 0 {
		return res, nil
	}
	planHashes := map[string]bool{}
	subjSeries := map[string][]float64{}
	for idx, aid := range assessmentIDs {
		ac, err := s.LoadAssessmentContext(ctx, aid)
		if err != nil {
			continue
		}
		run, rows, _, err := s.LatestOfficialAssessmentRun(ctx, aid)
		if err != nil {
			continue
		}
		ver, err := s.GetVersion(ctx, run.PlanVersionID)
		if err != nil {
			continue
		}
		planHashes[ver.ConfigHash] = true

		partCnt, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM exam_participants WHERE exam_cohort_id = ?`, ac.ExamCohort.ID)
		var sumTotal, sumClass, sumImp float64
		passTotal := 0
		bySubj := map[string][]float64{}
		for _, row := range rows {
			sumTotal += row.Total
			sumClass += row.ClassAverage
			sumImp += row.ImprovementScore
			passTotal += row.PassCount
			bySubj[row.Subject] = append(bySubj[row.Subject], row.ClassAverage)
		}
		n := len(rows)
		avgTotal, avgClass, avgImp := 0.0, 0.0, 0.0
		if n > 0 {
			avgTotal = sumTotal / float64(n)
			avgClass = sumClass / float64(n)
			avgImp = sumImp / float64(n)
		}
		for subj, vals := range bySubj {
			if _, ok := subjSeries[subj]; !ok {
				subjSeries[subj] = make([]float64, len(assessmentIDs))
			}
			var sSum float64
			for _, v := range vals {
				sSum += v
			}
			subjSeries[subj][idx] = sSum / float64(len(vals))
		}
		res.Items = append(res.Items, AssessmentComparisonItem{
			AssessmentID:         aid,
			ExamID:               ac.Exam.ID,
			ExamName:             ac.Exam.Name,
			ExamDate:             ac.Exam.ExamDate,
			CohortEntryYear:      ac.Cohort.EnrollmentYear,
			AcademicYear:         ac.AcademicYear,
			Semester:             ac.Exam.Term,
			PlanVersionID:        run.PlanVersionID,
			PlanConfigHash:       ver.ConfigHash,
			ParticipantCount:     partCnt,
			GroupCount:           n,
			AverageScore:         avgTotal,
			AverageClass:         avgClass,
			AverageImprovement:   avgImp,
			PassTotal:            passTotal,
			OfficialRunCreatedAt: run.CreatedAt,
		})
	}
	res.PlanDifferent = len(planHashes) > 1
	res.SubjectAverages = subjSeries
	return res, nil
}

func (s *Store) loadResultRowTeacherNames(ctx context.Context, resultRowID int64) []string {
	rows, err := s.db.Query(ctx, `SELECT teacher_name_snapshot FROM result_row_teachers
		WHERE result_row_id = ? ORDER BY teacher_name_snapshot`, resultRowID)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(rows.Values))
	for _, r := range rows.Values {
		if n := stringValue(r[0]); n != "" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func tierLabel(tierIndex int) string {
	switch tierIndex {
	case 1:
		return "一档"
	case 2:
		return "二档"
	case 3:
		return "三档"
	default:
		return fmt.Sprintf("%d档", tierIndex)
	}
}
