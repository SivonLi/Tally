package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// TeacherTrendPoint 记录教师在单场考试/考核中的得分明细。
type TeacherTrendPoint struct {
	ExamID        int64             `json:"examId"`
	AssessmentID  int64             `json:"assessmentId"`
	ExamName      string            `json:"examName"`
	ExamDate      string            `json:"examDate"`
	AcademicYear  string            `json:"academicYear"`
	Semester      string            `json:"semester"`
	PlanVersionID int64             `json:"planVersionId"`
	PersonalScore float64           `json:"personalScore"`
	Rows          []TeacherTrendRow `json:"rows"`
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
	ExamID        int64              `json:"examId"`
	ExamName      string             `json:"examName"`
	ExamDate      string             `json:"examDate"`
	AcademicYear  string             `json:"academicYear"`
	Semester      string             `json:"semester"`
	TotalScore    float64            `json:"totalScore"`
	GradeRank     int                `json:"gradeRank"`
	TotalStudents int                `json:"totalStudents"`
	Scores        map[string]float64 `json:"scores"`
	Flags         map[string]string  `json:"flags"`
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
	result := TeacherTrendResult{TeacherName: teacherName}
	if teacherName == "" {
		return result, nil
	}

	query := `SELECT a.id, e.id, e.name, e.exam_date, e.academic_year_start, e.term, a.plan_version_id, rr.id
		FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		LEFT JOIN result_runs rr ON rr.assessment_id = a.id AND rr.is_official = 1
		WHERE ec.cohort_id = ? AND rr.id IS NOT NULL`
	args := []any{cohortID}
	if len(examIDs) > 0 {
		placeholders := make([]string, len(examIDs))
		for i, id := range examIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += fmt.Sprintf(" AND e.id IN (%s)", strings.Join(placeholders, ","))
	}
	query += " ORDER BY e.exam_date ASC, e.id ASC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return result, err
	}

	planVersionSet := map[int64]bool{}
	totalPersonal := 0.0
	personalCount := 0

	for _, row := range rows.Values {
		assessID, _ := row[0].(int64)
		examID, _ := row[1].(int64)
		examName, _ := row[2].(string)
		examDate, _ := row[3].(string)
		acadStart, _ := row[4].(int64)
		term, _ := row[5].(string)
		planVerID, _ := row[6].(int64)
		runID, _ := row[7].(int64)

		acadYear := fmt.Sprintf("%d-%d", acadStart, acadStart+1)

		if planVerID > 0 {
			planVersionSet[planVerID] = true
		}

		point := TeacherTrendPoint{
			ExamID:        examID,
			AssessmentID:  assessID,
			ExamName:      examName,
			ExamDate:      examDate,
			AcademicYear:  acadYear,
			Semester:      term,
			PlanVersionID: planVerID,
		}

		// 查询教师个人量化总分
		psRows, err := s.db.Query(ctx, `SELECT score, subject FROM personal_scores
			WHERE run_id = ? AND teacher_name = ? LIMIT 1`, runID, teacherName)
		if err == nil && len(psRows.Values) > 0 {
			sc, _ := psRows.Values[0][0].(float64)
			subj, _ := psRows.Values[0][1].(string)
			point.PersonalScore = sc
			totalPersonal += sc
			personalCount++
			if result.Subject == "" && subj != "" {
				result.Subject = subj
			}
		}

		// 查询教师任课统计组行
		rRows, err := s.db.Query(ctx, `SELECT stat_group_id, stat_group_name, lead_class_name, subject,
			class_average, grade_average, baseline_average, baseline_ratio, improvement_score, pass_score, completion_score,
			total, rank, tier_index, points, teacher_names
			FROM result_rows WHERE run_id = ? AND (teacher_names LIKE ? OR teacher_names = ?)
			ORDER BY subject, stat_group_name`,
			runID, "%"+teacherName+"%", teacherName)
		if err == nil {
			for _, r := range rRows.Values {
				teachers, _ := r[15].(string)
				// 严格确认姓名在任课教师列表中
				found := false
				for _, name := range strings.Split(teachers, "、") {
					if strings.TrimSpace(name) == teacherName {
						found = true
						break
					}
				}
				if !found {
					continue
				}

				sgID, _ := r[0].(int64)
				sgName, _ := r[1].(string)
				leadClass, _ := r[2].(string)
				subj, _ := r[3].(string)
				clsAvg, _ := r[4].(float64)
				grdAvg, _ := r[5].(float64)
				baseAvg, _ := r[6].(float64)
				baseRatio, _ := r[7].(float64)
				impSc, _ := r[8].(float64)
				passSc, _ := r[9].(float64)
				compSc, _ := r[10].(float64)
				tot, _ := r[11].(float64)
				rankVal, _ := r[12].(int64)
				tierVal, _ := r[13].(int64)
				ptsVal, _ := r[14].(float64)

				if result.Subject == "" && subj != "" {
					result.Subject = subj
				}

				tierIndex := int(tierVal)
				tierStr := fmt.Sprintf("%d", tierIndex)

				point.Rows = append(point.Rows, TeacherTrendRow{
					StatGroupID:      sgID,
					StatGroupName:    sgName,
					LeadClassName:    leadClass,
					Subject:          subj,
					ClassAverage:     clsAvg,
					GradeAverage:     grdAvg,
					BaselineAverage:  baseAvg,
					BaselineRatio:    baseRatio,
					ImprovementScore: impSc,
					PassRateScore:    passSc,
					CompletionScore:  compSc,
					Total:            tot,
					Rank:             int(rankVal),
					TierIndex:        tierIndex,
					Tier:             tierStr,
					Points:           ptsVal,
				})
			}
		}

		if point.PersonalScore > 0 || len(point.Rows) > 0 {
			result.Points = append(result.Points, point)
		}
	}

	if personalCount > 0 {
		result.AveragePersonalScore = totalPersonal / float64(personalCount)
		result.LatestPersonalScore = result.Points[len(result.Points)-1].PersonalScore
	}
	result.HasMultiplePlans = len(planVersionSet) > 1

	return result, nil
}

// QueryClassTrends 查询指定班级在某届（级）历次考核中的表现趋势。
func (s *Store) QueryClassTrends(ctx context.Context, cohortID int64, classID int64, className string, examIDs []int64) (ClassTrendResult, error) {
	result := ClassTrendResult{ClassID: classID, ClassName: className}
	if classID <= 0 && className == "" {
		return result, nil
	}

	// 补全班级信息
	if className == "" {
		cRows, err := s.db.Query(ctx, `SELECT name FROM classes WHERE id = ?`, classID)
		if err == nil && len(cRows.Values) > 0 {
			className, _ = cRows.Values[0][0].(string)
			result.ClassName = className
		}
	}

	query := `SELECT a.id, e.id, e.name, e.exam_date, e.academic_year_start, e.term, rr.id
		FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		LEFT JOIN result_runs rr ON rr.assessment_id = a.id AND rr.is_official = 1
		WHERE ec.cohort_id = ? AND rr.id IS NOT NULL`
	args := []any{cohortID}
	if len(examIDs) > 0 {
		placeholders := make([]string, len(examIDs))
		for i, id := range examIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += fmt.Sprintf(" AND e.id IN (%s)", strings.Join(placeholders, ","))
	}
	query += " ORDER BY e.exam_date ASC, e.id ASC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return result, err
	}

	totalComp := 0.0
	compCount := 0

	for _, row := range rows.Values {
		assessID, _ := row[0].(int64)
		examID, _ := row[1].(int64)
		examName, _ := row[2].(string)
		examDate, _ := row[3].(string)
		acadStart, _ := row[4].(int64)
		term, _ := row[5].(string)
		runID, _ := row[6].(int64)

		acadYear := fmt.Sprintf("%d-%d", acadStart, acadStart+1)

		point := ClassTrendPoint{
			ExamID:          examID,
			AssessmentID:    assessID,
			ExamName:        examName,
			ExamDate:        examDate,
			AcademicYear:    acadYear,
			Semester:        term,
			CompletionScore: 0,
		}

		rRows, err := s.db.Query(ctx, `SELECT subject, stat_group_name, class_average, grade_average,
			baseline_average, baseline_ratio, improvement_score, pass_score, completion_score, total, rank, tier_index, points, teacher_names
			FROM result_rows WHERE run_id = ? AND (lead_class_id = ? OR lead_class_name = ?)
			ORDER BY subject`,
			runID, classID, className)
		if err == nil {
			for _, r := range rRows.Values {
				subj, _ := r[0].(string)
				sgName, _ := r[1].(string)
				clsAvg, _ := r[2].(float64)
				grdAvg, _ := r[3].(float64)
				baseAvg, _ := r[4].(float64)
				baseRatio, _ := r[5].(float64)
				impSc, _ := r[6].(float64)
				passSc, _ := r[7].(float64)
				compSc, _ := r[8].(float64)
				tot, _ := r[9].(float64)
				rankVal, _ := r[10].(int64)
				tierVal, _ := r[11].(int64)
				ptsVal, _ := r[12].(float64)
				tNames, _ := r[13].(string)

				if point.CompletionScore == 0 && compSc > 0 {
					point.CompletionScore = compSc
					totalComp += compSc
					compCount++
				}

				teachers := make([]string, 0)
				for _, t := range strings.Split(tNames, "、") {
					t = strings.TrimSpace(t)
					if t != "" {
						teachers = append(teachers, t)
					}
				}

				tierIndex := int(tierVal)
				tierStr := fmt.Sprintf("%d", tierIndex)

				point.Rows = append(point.Rows, ClassTrendRow{
					Subject:          subj,
					StatGroupName:    sgName,
					ClassAverage:     clsAvg,
					GradeAverage:     grdAvg,
					BaselineAverage:  baseAvg,
					BaselineRatio:    baseRatio,
					ImprovementScore: impSc,
					PassRateScore:    passSc,
					CompletionScore:  compSc,
					Total:            tot,
					Rank:             int(rankVal),
					TierIndex:        tierIndex,
					Tier:             tierStr,
					Points:           ptsVal,
					TeacherNames:     teachers,
				})
			}
		}

		if len(point.Rows) > 0 {
			result.Points = append(result.Points, point)
		}
	}

	if compCount > 0 {
		result.AverageCompletionScore = totalComp / float64(compCount)
	}

	return result, nil
}

// QuerySubjectComparison 查询单场考核中特定学科的所有教学组横向对比。
func (s *Store) QuerySubjectComparison(ctx context.Context, assessmentID int64, subject string) (SubjectComparisonResult, error) {
	result := SubjectComparisonResult{
		AssessmentID: assessmentID,
		Subject:      subject,
		TierCounts:   map[string]int{},
	}

	run, rows, _, err := s.LatestOfficialAssessmentRun(ctx, assessmentID)
	if err != nil {
		return result, err
	}
	if run.ID == 0 {
		return result, nil
	}

	// 读取考试名称
	exRows, err := s.db.Query(ctx, `SELECT e.name FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		JOIN exams e ON e.id = ec.exam_id
		WHERE a.id = ?`, assessmentID)
	if err == nil && len(exRows.Values) > 0 {
		result.ExamName, _ = exRows.Values[0][0].(string)
	}

	scoreSum := 0.0
	impSum := 0.0
	passSum := 0.0

	for _, row := range rows {
		if subject != "" && row.Subject != subject {
			continue
		}

		teachers := make([]string, 0)
		for _, t := range strings.Split(row.TeacherNames, "、") {
			t = strings.TrimSpace(t)
			if t != "" {
				teachers = append(teachers, t)
			}
		}

		scoreSum += row.Total
		impSum += row.ImprovementScore
		passSum += row.PassScore
		tierStr := fmt.Sprintf("%d", row.TierIndex)
		if row.TierIndex > 0 {
			result.TierCounts[tierStr]++
		}

		result.Groups = append(result.Groups, SubjectComparisonGroup{
			StatGroupID:      row.StatGroupID,
			StatGroupName:    row.StatGroupName,
			LeadClassName:    row.LeadClassName,
			TeacherNames:     teachers,
			ClassAverage:     row.ClassAverage,
			GradeAverage:     row.GradeAverage,
			BaselineAverage:  row.BaselineAverage,
			BaselineRatio:    row.BaselineRatio,
			ImprovementScore: row.ImprovementScore,
			PassRateScore:    row.PassScore,
			CompletionScore:  row.CompletionScore,
			Total:            row.Total,
			Rank:             row.Rank,
			TierIndex:        row.TierIndex,
			Tier:             tierStr,
			Points:           row.Points,
		})
	}

	result.GroupCount = len(result.Groups)
	if result.GroupCount > 0 {
		result.AverageScore = scoreSum / float64(result.GroupCount)
		result.AverageImprovement = impSum / float64(result.GroupCount)
		result.AveragePassRate = passSum / float64(result.GroupCount)
	}

	sort.Slice(result.Groups, func(i, j int) bool {
		return result.Groups[i].Total > result.Groups[j].Total
	})

	return result, nil
}

// QueryStudentTracking 查询指定学生在历次考试中的成绩与名次变化。
func (s *Store) QueryStudentTracking(ctx context.Context, cohortID int64, studentID int64, studentNo string) (StudentTrackResult, error) {
	result := StudentTrackResult{StudentID: studentID, StudentNo: studentNo}
	if studentID <= 0 && studentNo == "" {
		return result, nil
	}

	// 补齐学生信息
	var stuQuery string
	var stuArgs []any
	if studentID > 0 {
		stuQuery = `SELECT s.id, s.student_no, s.name FROM students s WHERE s.id = ?`
		stuArgs = []any{studentID}
	} else {
		stuQuery = `SELECT s.id, s.student_no, s.name FROM students s WHERE s.student_no = ?`
		stuArgs = []any{studentNo}
	}
	sRows, err := s.db.Query(ctx, stuQuery, stuArgs...)
	if err == nil && len(sRows.Values) > 0 {
		result.StudentID, _ = sRows.Values[0][0].(int64)
		result.StudentNo, _ = sRows.Values[0][1].(string)
		result.StudentName, _ = sRows.Values[0][2].(string)
	}
	studentID = result.StudentID

	// 获取此学生参加的历次考试
	query := `SELECT DISTINCT e.id, e.name, e.exam_date, e.academic_year_start, e.term
		FROM exam_scores es
		JOIN exams e ON e.id = es.exam_id
		WHERE es.student_id = ? AND es.cohort_id = ?
		ORDER BY e.exam_date ASC, e.id ASC`
	eRows, err := s.db.Query(ctx, query, studentID, cohortID)
	if err != nil {
		return result, err
	}

	for _, eRow := range eRows.Values {
		examID, _ := eRow[0].(int64)
		examName, _ := eRow[1].(string)
		examDate, _ := eRow[2].(string)
		acadStart, _ := eRow[3].(int64)
		term, _ := eRow[4].(string)

		acadYear := fmt.Sprintf("%d-%d", acadStart, acadStart+1)

		point := StudentTrackPoint{
			ExamID:       examID,
			ExamName:     examName,
			ExamDate:     examDate,
			AcademicYear: acadYear,
			Semester:     term,
			Scores:       map[string]float64{},
			Flags:        map[string]string{},
		}

		// 查询该生单科成绩
		scRows, err := s.db.Query(ctx, `SELECT subject, score, flag FROM exam_scores
			WHERE exam_id = ? AND student_id = ?`, examID, studentID)
		if err == nil {
			for _, sc := range scRows.Values {
				subj, _ := sc[0].(string)
				scoreVal, _ := sc[1].(float64)
				flag, _ := sc[2].(string)
				point.Scores[subj] = scoreVal
				if flag != "" {
					point.Flags[subj] = flag
				}
				if flag != "invalid" && flag != "absent" && flag != "cheating" {
					point.TotalScore += scoreVal
				}
			}
		}

		// 计算该生在全级的名次
		rankRows, err := s.db.Query(ctx, `SELECT student_id, SUM(score) as total
			FROM exam_scores
			WHERE exam_id = ? AND cohort_id = ? AND flag NOT IN ('invalid', 'absent', 'cheating')
			GROUP BY student_id
			ORDER BY total DESC`, examID, cohortID)
		if err == nil {
			point.TotalStudents = len(rankRows.Values)
			for idx, r := range rankRows.Values {
				sid, _ := r[0].(int64)
				if sid == studentID {
					point.GradeRank = idx + 1
					break
				}
			}
		}

		result.Points = append(result.Points, point)
	}

	// 查找班级与选科
	clsRows, err := s.db.Query(ctx, `SELECT c.name, er.track FROM exam_rosters er
		JOIN classes c ON c.id = er.class_id
		WHERE er.student_id = ? AND er.cohort_id = ?
		ORDER BY er.id DESC LIMIT 1`, studentID, cohortID)
	if err == nil && len(clsRows.Values) > 0 {
		result.ClassName, _ = clsRows.Values[0][0].(string)
		result.Track, _ = clsRows.Values[0][1].(string)
	}

	return result, nil
}

// QueryAssessmentComparison 查询 2~5 个考核批次的并排对比结果。
func (s *Store) QueryAssessmentComparison(ctx context.Context, assessmentIDs []int64) (AssessmentComparisonResult, error) {
	result := AssessmentComparisonResult{
		SubjectAverages: map[string][]float64{},
	}
	if len(assessmentIDs) == 0 {
		return result, nil
	}

	planVersionHashes := map[int64]string{}

	for _, aID := range assessmentIDs {
		run, rows, _, err := s.LatestOfficialAssessmentRun(ctx, aID)
		if err != nil {
			return result, err
		}

		infoRows, err := s.db.Query(ctx, `SELECT a.id, e.id, e.name, e.exam_date, c.enrollment_year,
			e.academic_year_start, e.term, a.plan_version_id, COALESCE(pv.config_hash, '')
			FROM assessments a
			JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
			JOIN exams e ON e.id = ec.exam_id
			JOIN cohorts c ON c.id = ec.cohort_id
			LEFT JOIN plan_versions pv ON pv.id = a.plan_version_id
			WHERE a.id = ?`, aID)
		if err != nil {
			return result, err
		}
		if len(infoRows.Values) == 0 {
			continue
		}

		row0 := infoRows.Values[0]
		assessID, _ := row0[0].(int64)
		examID, _ := row0[1].(int64)
		examName, _ := row0[2].(string)
		examDate, _ := row0[3].(string)
		cohortYear, _ := row0[4].(int64)
		acadStart, _ := row0[5].(int64)
		term, _ := row0[6].(string)
		pvID, _ := row0[7].(int64)
		cfgHash, _ := row0[8].(string)

		acadYear := fmt.Sprintf("%d-%d", acadStart, acadStart+1)

		if pvID > 0 {
			planVersionHashes[pvID] = cfgHash
		}

		// 统计参与考生人数
		pCount := 0
		pRows, err := s.db.Query(ctx, `SELECT COUNT(DISTINCT student_id)
			FROM exam_scores WHERE exam_id = ? AND cohort_id = (
				SELECT cohort_id FROM exam_cohorts ec
				JOIN assessments a ON a.exam_cohort_id = ec.id
				WHERE a.id = ?
			)`, examID, aID)
		if err == nil && len(pRows.Values) > 0 {
			v, _ := pRows.Values[0][0].(int64)
			pCount = int(v)
		}

		scoreSum := 0.0
		classSum := 0.0
		impSum := 0.0
		passSum := 0

		subjectGroupSums := map[string]float64{}
		subjectGroupCounts := map[string]int{}

		for _, r := range rows {
			scoreSum += r.Total
			classSum += r.ClassAverage
			impSum += r.ImprovementScore
			passSum += r.PassCount

			subjectGroupSums[r.Subject] += r.Total
			subjectGroupCounts[r.Subject]++
		}

		item := AssessmentComparisonItem{
			AssessmentID:         assessID,
			ExamID:               examID,
			ExamName:             examName,
			ExamDate:             examDate,
			CohortEntryYear:      int(cohortYear),
			AcademicYear:         acadYear,
			Semester:             term,
			PlanVersionID:        pvID,
			PlanConfigHash:       cfgHash,
			ParticipantCount:     pCount,
			GroupCount:           len(rows),
			PassTotal:            passSum,
			OfficialRunCreatedAt: run.CreatedAt,
		}

		if len(rows) > 0 {
			item.AverageScore = scoreSum / float64(len(rows))
			item.AverageClass = classSum / float64(len(rows))
			item.AverageImprovement = impSum / float64(len(rows))
		}

		result.Items = append(result.Items, item)

		for subj, sum := range subjectGroupSums {
			cnt := subjectGroupCounts[subj]
			avg := 0.0
			if cnt > 0 {
				avg = sum / float64(cnt)
			}
			result.SubjectAverages[subj] = append(result.SubjectAverages[subj], avg)
		}
	}

	if len(planVersionHashes) > 1 {
		result.PlanDifferent = true
	}

	return result, nil
}
