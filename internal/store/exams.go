package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/school"
)

// CreateExam 新建一场考试及其覆盖的年级和考试学科设置。月考（monthly）按需创建考核，其余考试自动预建考核记录。
func (s *Store) CreateExam(ctx context.Context, input CreateExamInput) (Exam, error) {
	if input.AcademicYearStart < 1900 || input.AcademicYearStart > 2999 {
		return Exam{}, fmt.Errorf("学年起始年份超出范围")
	}
	if input.Term != ExamTermFirst && input.Term != ExamTermSecond {
		return Exam{}, fmt.Errorf("学期类型无效 %q", input.Term)
	}
	switch input.ExamType {
	case ExamMonthly, ExamMidterm, ExamFinal, ExamMock, ExamGaokao:
	default:
		return Exam{}, fmt.Errorf("考试类型无效 %q", input.ExamType)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Exam{}, fmt.Errorf("考试名称不能为空")
	}
	if len(input.CohortIDs) == 0 {
		return Exam{}, fmt.Errorf("至少选择一个参加考试的级")
	}
	seq := input.Sequence
	if seq <= 0 {
		seq = 1
	}

	subjects, err := s.ListSubjects(ctx)
	if err != nil {
		return Exam{}, err
	}
	defaultPlan, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		return Exam{}, err
	}

	type cohortPrep struct {
		cohortID int64
		phase    string
		subJSON  string
	}
	seenCohort := map[int64]bool{}
	var preps []cohortPrep
	for _, cid := range input.CohortIDs {
		if seenCohort[cid] {
			continue
		}
		seenCohort[cid] = true
		cohort, err := s.GetCohort(ctx, cid)
		if err != nil {
			return Exam{}, err
		}
		phase := cohort.CurrentPhase
		if idx := school.StageIndex(cohort.EnrollmentYear, input.AcademicYearStart); idx >= 1 && idx <= 3 {
			phase = school.StageKey(idx)
		} else if phase == PhaseGraduated {
			if input.ExamType == ExamGaokao {
				phase = ExamGaokao
			} else {
				phase = PhaseG3
			}
		}
		var list []ExamSubject
		marks := input.SubjectMarks[cid]
		for _, subj := range subjects {
			if !subj.Enabled && marks == nil {
				continue
			}
			fullMark := subj.FullMark
			if override, ok := marks[subj.Key]; ok && override > 0 {
				fullMark = override
			} else if marks != nil {
				continue
			}
			list = append(list, ExamSubject{
				Subject:  subj.Key,
				FullMark: fullMark,
				Enabled:  true,
			})
		}
		rawSubs, err := json.Marshal(list)
		if err != nil {
			return Exam{}, err
		}
		preps = append(preps, cohortPrep{
			cohortID: cid,
			phase:    phase,
			subJSON:  string(rawSubs),
		})
	}

	dupRows, err := s.db.Query(ctx, `SELECT id FROM exams WHERE academic_year_start = ? AND term = ? AND exam_type = ? AND name = ?`,
		input.AcademicYearStart, input.Term, input.ExamType, name)
	if err != nil {
		return Exam{}, err
	}
	if len(dupRows.Values) > 0 {
		return Exam{}, ErrExamExists
	}

	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `INSERT INTO exams(academic_year_start, term, exam_type, name, exam_date, sequence, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'draft', ?)`,
		input.AcademicYearStart, input.Term, input.ExamType, name, strings.TrimSpace(input.ExamDate), seq, now)
	if err != nil {
		return Exam{}, err
	}
	examID := res.LastInsertID

	var statements []sqlite.Statement
	for _, prep := range preps {
		statements = append(statements, sqlite.Statement{
			SQL:  `INSERT INTO exam_cohorts(exam_id, cohort_id, phase_at_exam, subjects_json) VALUES (?, ?, ?, ?)`,
			Args: []any{examID, prep.cohortID, prep.phase, prep.subJSON},
		})
		if input.ExamType != ExamMonthly {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO assessments(exam_cohort_id, plan_version_id, status, revision, created_at)
					SELECT id, ?, 'draft', 1, ? FROM exam_cohorts WHERE exam_id = ? AND cohort_id = ?`,
				Args: []any{defaultPlan.ID, now, examID, prep.cohortID},
			})
		}
	}
	statements = append(statements, bumpRevisionStatement(fmt.Sprintf("exam:%d", examID), now))
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return Exam{}, err
	}
	return s.GetExam(ctx, examID)
}

// GetExam 读取单场考试及其覆盖的全部级详情。
func (s *Store) GetExam(ctx context.Context, examID int64) (Exam, error) {
	rows, err := s.db.Query(ctx, `SELECT id, academic_year_start, term, exam_type, name, exam_date, sequence, status, created_at
		FROM exams WHERE id = ?`, examID)
	if err != nil {
		return Exam{}, err
	}
	if len(rows.Values) == 0 {
		return Exam{}, ErrNotFound
	}
	exam, err := examFromRow(rows.Values[0])
	if err != nil {
		return Exam{}, err
	}
	cohorts, err := s.loadExamCohorts(ctx, exam.ID, exam.AcademicYearStart)
	if err != nil {
		return Exam{}, err
	}
	exam.Cohorts = cohorts
	return exam, nil
}

// ListExams 返回全部考试列表，按学年、时间倒序排列。
func (s *Store) ListExams(ctx context.Context) ([]Exam, error) {
	rows, err := s.db.Query(ctx, `SELECT id, academic_year_start, term, exam_type, name, exam_date, sequence, status, created_at
		FROM exams ORDER BY academic_year_start DESC, exam_date DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	exams := make([]Exam, 0, len(rows.Values))
	for _, row := range rows.Values {
		exam, err := examFromRow(row)
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

// ListExamCohorts 返回一场考试下所有参考级及其阶段与统计信息。
func (s *Store) ListExamCohorts(ctx context.Context, examID int64) ([]ExamCohort, error) {
	return s.loadExamCohorts(ctx, examID, 0)
}

// GetExamCohort 读取一场考试中指定级的考核单元配置。
func (s *Store) GetExamCohort(ctx context.Context, examID, cohortID int64) (ExamCohort, error) {
	rows, err := s.db.Query(ctx, `SELECT ec.id, ec.exam_id, ec.cohort_id, ec.phase_at_exam,
		c.enrollment_year, c.graduation_year, ec.subjects_json, e.academic_year_start,
		(SELECT COUNT(*) FROM exam_participants ep WHERE ep.exam_cohort_id = ec.id),
		(SELECT COUNT(*) FROM exam_scores es JOIN exam_participants ep ON ep.id = es.exam_participant_id WHERE ep.exam_cohort_id = ec.id),
		COALESCE((SELECT COUNT(*) FROM assessment_class_targets act JOIN assessments a ON a.id = act.assessment_id WHERE a.exam_cohort_id = ec.id), 0),
		COALESCE((SELECT COUNT(*) FROM assessment_stat_groups asg JOIN assessments a ON a.id = asg.assessment_id WHERE a.exam_cohort_id = ec.id), 0)
		FROM exam_cohorts ec
		JOIN cohorts c ON c.id = ec.cohort_id
		JOIN exams e ON e.id = ec.exam_id
		WHERE ec.exam_id = ? AND ec.cohort_id = ?`, examID, cohortID)
	if err != nil {
		return ExamCohort{}, err
	}
	if len(rows.Values) == 0 {
		return ExamCohort{}, ErrExamCohort
	}
	return examCohortFromRow(rows.Values[0]), nil
}

func (s *Store) loadExamCohorts(ctx context.Context, examID int64, academicYearStart int) ([]ExamCohort, error) {
	rows, err := s.db.Query(ctx, `SELECT ec.id, ec.exam_id, ec.cohort_id, ec.phase_at_exam,
		c.enrollment_year, c.graduation_year, ec.subjects_json, e.academic_year_start,
		(SELECT COUNT(*) FROM exam_participants ep WHERE ep.exam_cohort_id = ec.id),
		(SELECT COUNT(*) FROM exam_scores es JOIN exam_participants ep ON ep.id = es.exam_participant_id WHERE ep.exam_cohort_id = ec.id),
		COALESCE((SELECT COUNT(*) FROM assessment_class_targets act JOIN assessments a ON a.id = act.assessment_id WHERE a.exam_cohort_id = ec.id), 0),
		COALESCE((SELECT COUNT(*) FROM assessment_stat_groups asg JOIN assessments a ON a.id = asg.assessment_id WHERE a.exam_cohort_id = ec.id), 0)
		FROM exam_cohorts ec
		JOIN cohorts c ON c.id = ec.cohort_id
		JOIN exams e ON e.id = ec.exam_id
		WHERE ec.exam_id = ? ORDER BY c.enrollment_year DESC`, examID)
	if err != nil {
		return nil, err
	}
	cohorts := make([]ExamCohort, 0, len(rows.Values))
	for _, row := range rows.Values {
		ec := examCohortFromRow(row)
		if ec.AcademicYearStart == 0 {
			ec.AcademicYearStart = academicYearStart
		}
		cohorts = append(cohorts, ec)
	}
	return cohorts, nil
}

func examFromRow(row []any) (Exam, error) {
	createdAt, err := parseTime(row[8])
	if err != nil {
		return Exam{}, err
	}
	return Exam{
		ID:                intValue(row[0]),
		AcademicYearStart: int(intValue(row[1])),
		Term:              stringValue(row[2]),
		ExamType:          stringValue(row[3]),
		Name:              stringValue(row[4]),
		ExamDate:          stringValue(row[5]),
		Sequence:          int(intValue(row[6])),
		Status:            stringValue(row[7]),
		CreatedAt:         createdAt,
	}, nil
}

func examCohortFromRow(row []any) ExamCohort {
	cohortID := intValue(row[2])
	phase := stringValue(row[3])
	enrollYear := int(intValue(row[4]))
	var subs []ExamSubject
	_ = json.Unmarshal([]byte(stringValue(row[6])), &subs)
	enabledCnt := 0
	for _, sub := range subs {
		if sub.Enabled {
			enabledCnt++
		}
	}
	return ExamCohort{
		ID:                intValue(row[0]),
		ExamID:            intValue(row[1]),
		CohortID:          cohortID,
		StageID:           EncodeStageID(cohortID, phaseForStage(phase)),
		GradeLevelAtExam:  phase,
		CohortName:        formatCohortLabel(enrollYear),
		EnrollmentYear:    enrollYear,
		GraduationYear:    int(intValue(row[5])),
		AcademicYearStart: int(intValue(row[7])),
		RosterCount:       int(intValue(row[8])),
		ScoreCount:        int(intValue(row[9])),
		SubjectCount:      enabledCnt,
		ClassTargetCount:  int(intValue(row[10])),
		StatGroupCount:    int(intValue(row[11])),
	}
}

func phaseForStage(phase string) string {
	if phase == ExamGaokao || phase == PhaseGraduated {
		return PhaseG3
	}
	return phase
}

// ListExamSubjects 返回考试下指定年级的科目及满分配置。
func (s *Store) ListExamSubjects(ctx context.Context, examID, cohortID int64) ([]ExamSubject, error) {
	rows, err := s.db.Query(ctx, `SELECT subjects_json FROM exam_cohorts WHERE exam_id = ? AND cohort_id = ?`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	if len(rows.Values) == 0 {
		return nil, ErrExamCohort
	}
	var subs []ExamSubject
	if err := json.Unmarshal([]byte(stringValue(rows.Values[0][0])), &subs); err != nil {
		return nil, err
	}
	for i := range subs {
		subs[i].ExamID = examID
		subs[i].CohortID = cohortID
	}
	return subs, nil
}

// GenerateExamRoster 从年级在籍学生名单自动生成本次考试的考务快照与参考名单；defaultTrack 用于补充尚未分科的默认类别。
func (s *Store) GenerateExamRoster(ctx context.Context, examID, cohortID int64, defaultTrack string) (ExamRosterSummary, error) {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return ExamRosterSummary{}, err
	}
	stuRows, err := s.db.Query(ctx, `SELECT st.id, COALESCE(st.current_class_id, 0), COALESCE(c.name, ''),
		st.current_track, st.elective_combo, st.foreign_lang, st.status, st.candidate_code
		FROM students st
		LEFT JOIN classes c ON c.id = st.current_class_id
		WHERE st.cohort_id = ? AND st.status IN ('active','suspended','borrowed')
		ORDER BY st.student_no, st.id`, cohortID)
	if err != nil {
		return ExamRosterSummary{}, err
	}
	existingRows, err := s.db.Query(ctx, `SELECT student_id FROM exam_participants WHERE exam_cohort_id = ?`, ec.ID)
	if err != nil {
		return ExamRosterSummary{}, err
	}
	existing := make(map[int64]bool, len(existingRows.Values))
	for _, r := range existingRows.Values {
		existing[intValue(r[0])] = true
	}

	normDefaultTrack := normalizeTrackValue(defaultTrack)
	summary := ExamRosterSummary{Students: len(stuRows.Values)}
	var statements []sqlite.Statement
	for _, r := range stuRows.Values {
		sid := intValue(r[0])
		classID := intValue(r[1])
		className := stringValue(r[2])
		track := stringValue(r[3])
		elective := stringValue(r[4])
		if track == "" {
			track = trackFromElective(elective)
		}
		if track == "" && normDefaultTrack != "" {
			track = normDefaultTrack
		}
		if track == "" {
			summary.MissingTrack++
		}
		if existing[sid] {
			summary.Existing++
			continue
		}
		summary.Inserted++
		fl := stringValue(r[5])
		if fl == "" {
			fl = "english"
		}
		status := stringValue(r[6])
		if status == "" {
			status = "active"
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO exam_participants(
				exam_cohort_id, student_id, class_id, class_name_snapshot,
				track_snapshot, elective_snapshot, foreign_lang_snapshot, status_snapshot, candidate_no, is_valid_for_calc)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{ec.ID, sid, nullableID(classID), className, track, elective, fl, status, stringValue(r[7]), boolInt(isParticipantValidStatus(status))},
		})
	}
	if len(statements) > 0 {
		if _, err := s.db.Transaction(ctx, statements); err != nil {
			return ExamRosterSummary{}, err
		}
	}
	return summary, nil
}

// AddExamRosterStudents 把成绩单中不在报名名单的考生补建为学生并加入本场考试报名名单。
func (s *Store) AddExamRosterStudents(ctx context.Context, examCohort ExamCohort, entries []ImportStudent) (int, int, error) {
	roster, err := s.examRosterUserIndex(ctx, examCohort)
	if err != nil {
		return 0, 0, err
	}
	phase := phaseForStage(examCohort.GradeLevelAtExam)
	classes, err := s.phaseClassIDMap(ctx, examCohort.CohortID, phase)
	if err != nil {
		return 0, 0, err
	}
	now := s.now().Format(time.RFC3339Nano)
	added := 0
	skipped := 0

	for _, entry := range entries {
		studentNo := strings.TrimSpace(entry.StudentNo)
		name := strings.TrimSpace(entry.Name)
		if studentNo == "" || name == "" {
			skipped++
			continue
		}
		if _, ok := roster.byStudentNo[studentNo]; ok {
			continue
		}
		className := normalizeClassName(entry.ClassName)
		if className == "" {
			skipped++
			continue
		}

		// 查找学生档案或新建档案。
		studentID, _, matchErr := s.MatchStudent(ctx, examCohort.CohortID, phase, studentNo, entry.CandidateCode, name, className)
		classID := classes[className]
		track := normalizeTrackValue(entry.Track)
		if track == "" {
			track = trackFromElective(entry.Elective)
		}
		fl := "english"
		if entry.Japanese {
			fl = "japanese"
		}
		status := normalizeStudentStatus(entry.Status)

		if errors.Is(matchErr, ErrNotFound) {
			if classID == 0 {
				createdClass, err := s.CreateClass(ctx, examCohort.CohortID, phase, className, "", "", nil)
				if err != nil {
					return added, skipped, err
				}
				classID = createdClass.ID
				classes[className] = classID
			}
			res, err := s.db.Execute(ctx, `INSERT INTO students(
				cohort_id, student_no, candidate_code, name, gender, current_class_id, current_track,
				elective_combo, foreign_lang, status, source_sheet, source_row, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				examCohort.CohortID, studentNo, strings.TrimSpace(entry.CandidateCode), name,
				strings.TrimSpace(entry.Gender), classID, track, strings.TrimSpace(entry.Elective),
				fl, status, entry.SourceSheet, entry.SourceRow, now, now)
			if err != nil {
				return added, skipped, err
			}
			studentID = res.LastInsertID
		} else if matchErr != nil {
			skipped++
			continue
		} else if classID == 0 {
			// 读取已有班级快照。
			stuRows, err := s.db.Query(ctx, `SELECT COALESCE(current_class_id, 0), current_track, elective_combo, foreign_lang, status FROM students WHERE id = ?`, studentID)
			if err == nil && len(stuRows.Values) > 0 {
				classID = intValue(stuRows.Values[0][0])
				if track == "" {
					track = stringValue(stuRows.Values[0][1])
				}
				if entry.Elective == "" {
					entry.Elective = stringValue(stuRows.Values[0][2])
				}
				if stringValue(stuRows.Values[0][3]) != "" {
					fl = stringValue(stuRows.Values[0][3])
				}
				if entry.Status == "" {
					status = stringValue(stuRows.Values[0][4])
				}
			}
		}

		_, err = s.db.Execute(ctx, `INSERT INTO exam_participants(
			exam_cohort_id, student_id, class_id, class_name_snapshot,
			track_snapshot, elective_snapshot, foreign_lang_snapshot, status_snapshot, candidate_no,
			total_score, is_valid_for_calc, source_sheet, source_row)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(exam_cohort_id, student_id) DO NOTHING`,
			examCohort.ID, studentID, nullableID(classID), className, track, strings.TrimSpace(entry.Elective),
			fl, status, strings.TrimSpace(entry.CandidateCode), optionalFloat(entry.TotalScore), boolInt(isParticipantValidStatus(status)),
			entry.SourceSheet, entry.SourceRow)
		if err != nil {
			return added, skipped, err
		}
		roster.byStudentNo[studentNo] = ExamRosterEntry{
			StudentID: studentID,
			StudentNo: studentNo,
			Name:      name,
			ClassID:   classID,
			ClassName: className,
			Track:     track,
		}
		added++
	}
	return added, skipped, nil
}

// ListExamRoster 分页读取一场考试中指定级的参考名单与总分排名快照。
func (s *Store) ListExamRoster(ctx context.Context, filter ExamRosterFilter) ([]ExamRosterEntry, int, error) {
	if filter.PageSize <= 0 {
		filter.PageSize = 200
	}
	if filter.Page < 0 {
		filter.Page = 0
	}
	ec, err := s.GetExamCohort(ctx, filter.ExamID, filter.CohortID)
	if err != nil {
		return nil, 0, err
	}
	where := []string{"ep.exam_cohort_id = ?"}
	args := []any{ec.ID}
	if text := strings.TrimSpace(filter.Query); text != "" {
		where = append(where, "(st.student_no LIKE ? OR ep.candidate_no LIKE ? OR st.candidate_code LIKE ? OR st.name LIKE ? OR ep.class_name_snapshot LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like, like, like, like)
	}
	cond := strings.Join(where, " AND ")
	total, err := s.countQuery(ctx, `SELECT COUNT(*) FROM exam_participants ep
		JOIN students st ON st.id = ep.student_id WHERE `+cond, args...)
	if err != nil {
		return nil, 0, err
	}

	qArgs := append(append([]any{}, args...), filter.PageSize, filter.Page*filter.PageSize)
	rows, err := s.db.Query(ctx, `SELECT ep.id, ep.student_id, st.student_no, st.name,
		COALESCE(ep.class_id, 0), ep.class_name_snapshot, ep.exam_no, COALESCE(NULLIF(ep.candidate_no, ''), st.candidate_code),
		ep.track_snapshot, ep.elective_snapshot, ep.foreign_lang_snapshot, ep.status_snapshot,
		ep.total_score, ep.grade_rank, ep.grade_total_count, ep.track_rank, ep.track_total_count,
		ep.class_rank, ep.class_total_count,
		COALESCE(ep.import_run_id, 0), ep.source_sheet, ep.source_row
		FROM exam_participants ep
		JOIN students st ON st.id = ep.student_id
		WHERE `+cond+` ORDER BY st.student_no, ep.id LIMIT ? OFFSET ?`, qArgs...)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ExamRosterEntry, 0, len(rows.Values))
	for _, r := range rows.Values {
		totalScore, _ := nullableFloat(r[12])
		elective := stringValue(r[9])
		items = append(items, ExamRosterEntry{
			ID:              intValue(r[0]),
			StudentID:       intValue(r[1]),
			StudentNo:       stringValue(r[2]),
			Name:            stringValue(r[3]),
			ClassID:         intValue(r[4]),
			ClassName:       stringValue(r[5]),
			ExamNo:          stringValue(r[6]),
			CandidateNo:     stringValue(r[7]),
			Track:           stringValue(r[8]),
			Elective:        elective,
			ForeignLang:     stringValue(r[10]),
			Status:          stringValue(r[11]),
			TotalScore:      totalScore,
			GradeRank:       nullableIntPtr(r[13]),
			GradeTotalCount: nullableIntPtr(r[14]),
			TrackRank:       nullableIntPtr(r[15]),
			TrackTotalCount: nullableIntPtr(r[16]),
			ClassRank:       nullableIntPtr(r[17]),
			ClassTotalCount: nullableIntPtr(r[18]),
			Subjects:        electiveSubjects(elective),
			ImportRunID:     intValue(r[19]),
			SourceSheet:     stringValue(r[20]),
			SourceRow:       int(intValue(r[21])),
		})
	}
	return items, total, nil
}

// ListExamScores 按条件分页读取单科成绩及单科排名快照。
func (s *Store) ListExamScores(ctx context.Context, filter ExamScoreFilter) ([]ExamScoreRow, int, error) {
	if filter.PageSize <= 0 {
		filter.PageSize = 200
	}
	if filter.Page < 0 {
		filter.Page = 0
	}
	ec, err := s.GetExamCohort(ctx, filter.ExamID, filter.CohortID)
	if err != nil {
		return nil, 0, err
	}
	where := []string{"ep.exam_cohort_id = ?"}
	args := []any{ec.ID}
	if filter.Subject != "" {
		where = append(where, "es.subject = ?")
		args = append(args, filter.Subject)
	}
	if filter.ClassID != 0 {
		where = append(where, "ep.class_id = ?")
		args = append(args, filter.ClassID)
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		where = append(where, "(st.student_no LIKE ? OR st.name LIKE ? OR ep.class_name_snapshot LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}
	cond := strings.Join(where, " AND ")
	total, err := s.countQuery(ctx, `SELECT COUNT(*) FROM exam_scores es
		JOIN exam_participants ep ON ep.id = es.exam_participant_id
		JOIN students st ON st.id = ep.student_id
		WHERE `+cond, args...)
	if err != nil {
		return nil, 0, err
	}
	qArgs := append(append([]any{}, args...), filter.PageSize, filter.Page*filter.PageSize)
	rows, err := s.db.Query(ctx, `SELECT ep.student_id, st.student_no, st.name,
		COALESCE(ep.class_id, 0), ep.class_name_snapshot, es.subject, es.score, es.flag,
		es.teaching_group_id_snapshot, es.subject_grade_rank, es.subject_class_rank,
		COALESCE(es.import_run_id, 0), es.source_sheet, es.source_row
		FROM exam_scores es
		JOIN exam_participants ep ON ep.id = es.exam_participant_id
		JOIN students st ON st.id = ep.student_id
		WHERE `+cond+` ORDER BY st.student_no, es.subject LIMIT ? OFFSET ?`, qArgs...)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ExamScoreRow, 0, len(rows.Values))
	for _, r := range rows.Values {
		score, _ := nullableFloat(r[6])
		var tgID *int64
		if v, ok := nullableInt64(r[8]); ok {
			tgID = &v
		}
		items = append(items, ExamScoreRow{
			StudentID:             intValue(r[0]),
			StudentNo:             stringValue(r[1]),
			Name:                  stringValue(r[2]),
			ClassID:               intValue(r[3]),
			ClassName:             stringValue(r[4]),
			Subject:               stringValue(r[5]),
			Score:                 score,
			Flag:                  stringValue(r[7]),
			TeachingGroupSnapshot: tgID,
			SubjectGradeRank:      nullableIntPtr(r[9]),
			SubjectClassRank:      nullableIntPtr(r[10]),
			ImportRunID:           intValue(r[11]),
			SourceSheet:           stringValue(r[12]),
			SourceRow:             int(intValue(r[13])),
		})
	}
	return items, total, nil
}

// ExamScoreSummaries 汇总一场考试中各个科目的实考人数、缺考与均分极值。
func (s *Store) ExamScoreSummaries(ctx context.Context, examID, cohortID int64) ([]ExamScoreSummary, error) {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT es.subject,
		COALESCE(SUM(CASE WHEN es.score IS NOT NULL AND es.score > 0 AND es.flag = '' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN es.score IS NULL OR es.score <= 0 OR es.flag <> '' THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(CASE WHEN es.score IS NOT NULL AND es.score > 0 AND es.flag = '' THEN es.score END), 0),
		COALESCE(MIN(CASE WHEN es.score IS NOT NULL AND es.score > 0 AND es.flag = '' THEN es.score END), 0),
		COALESCE(MAX(CASE WHEN es.score IS NOT NULL AND es.score > 0 AND es.flag = '' THEN es.score END), 0)
		FROM exam_scores es
		JOIN exam_participants ep ON ep.id = es.exam_participant_id
		WHERE ep.exam_cohort_id = ?
		GROUP BY es.subject ORDER BY es.subject`, ec.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ExamScoreSummary, 0, len(rows.Values))
	for _, r := range rows.Values {
		out = append(out, ExamScoreSummary{
			Subject: stringValue(r[0]),
			Count:   int(intValue(r[1])),
			Missing: int(intValue(r[2])),
			Average: floatValue(r[3]),
			Minimum: floatValue(r[4]),
			Maximum: floatValue(r[5]),
		})
	}
	return out, nil
}

// ExamScoreBands 统计所有已录入科目的成绩分段人数分布。
func (s *Store) ExamScoreBands(ctx context.Context, examID, cohortID int64) ([]ExamScoreBand, error) {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	subs, _ := s.ListExamSubjects(ctx, examID, cohortID)
	fullMarkMap := map[string]float64{}
	for _, sub := range subs {
		if sub.FullMark > 0 {
			fullMarkMap[sub.Subject] = sub.FullMark
		}
	}
	rows, err := s.db.Query(ctx, `SELECT es.subject, es.score FROM exam_scores es
		JOIN exam_participants ep ON ep.id = es.exam_participant_id
		WHERE ep.exam_cohort_id = ? AND es.score IS NOT NULL AND es.score > 0 AND es.flag = ''
		ORDER BY es.subject`, ec.ID)
	if err != nil {
		return nil, err
	}
	bySubj := map[string][]float64{}
	var subjOrder []string
	for _, r := range rows.Values {
		subj := stringValue(r[0])
		if _, exists := bySubj[subj]; !exists {
			subjOrder = append(subjOrder, subj)
		}
		bySubj[subj] = append(bySubj[subj], floatValue(r[1]))
	}
	var bands []ExamScoreBand
	for _, subj := range subjOrder {
		fullMark := 100.0
		if fm := fullMarkMap[subj]; fm > 0 {
			fullMark = fm
		}
		step := 10.0
		if fullMark >= 140 {
			step = 15.0
		}
		buckets := int(math.Ceil(fullMark / step))
		if buckets <= 0 {
			buckets = 10
		}
		counts := make([]int, buckets)
		for _, val := range bySubj[subj] {
			idx := int(val / step)
			if idx >= buckets {
				idx = buckets - 1
			}
			if idx < 0 {
				idx = 0
			}
			counts[idx]++
		}
		for i := 0; i < buckets; i++ {
			lower := float64(i) * step
			upper := math.Min(fullMark, lower+step)
			bands = append(bands, ExamScoreBand{
				Subject: subj,
				Lower:   lower,
				Upper:   upper,
				Count:   counts[i],
			})
		}
	}
	return bands, nil
}

// ExamClassAverages 返回各行政班在考试各科目的均分。
func (s *Store) ExamClassAverages(ctx context.Context, examID, cohortID int64) ([]ExamClassAverage, error) {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT es.subject, COALESCE(ep.class_id, 0), ep.class_name_snapshot,
		COUNT(*), AVG(es.score)
		FROM exam_scores es
		JOIN exam_participants ep ON ep.id = es.exam_participant_id
		WHERE ep.exam_cohort_id = ? AND es.score IS NOT NULL AND es.score > 0 AND es.flag = ''
		GROUP BY es.subject, ep.class_id, ep.class_name_snapshot
		ORDER BY es.subject, ep.class_name_snapshot`, ec.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ExamClassAverage, 0, len(rows.Values))
	for _, r := range rows.Values {
		out = append(out, ExamClassAverage{
			Subject:   stringValue(r[0]),
			ClassID:   intValue(r[1]),
			ClassName: stringValue(r[2]),
			Count:     int(intValue(r[3])),
			Average:   floatValue(r[4]),
		})
	}
	return out, nil
}

// ListExamSubjectCutoffs 读取考试各科分数线。
func (s *Store) ListExamSubjectCutoffs(ctx context.Context, examID, cohortID int64) ([]SubjectCutoff, error) {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT "key", track, cutoff FROM exam_cutoffs
		WHERE exam_cohort_id = ? AND kind = 'subject' ORDER BY "key", track`, ec.ID)
	if err != nil {
		return nil, err
	}
	out := make([]SubjectCutoff, 0, len(rows.Values))
	for _, r := range rows.Values {
		v := floatValue(r[2])
		out = append(out, SubjectCutoff{
			Subject: stringValue(r[0]),
			Track:   stringValue(r[1]),
			Value:   v,
			Cutoff:  v,
		})
	}
	return out, nil
}

// ListExamTierCutoffs 读取考试总分各档位分数线。
func (s *Store) ListExamTierCutoffs(ctx context.Context, examID, cohortID int64) ([]TierCutoff, error) {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT "key", track, cutoff FROM exam_cutoffs
		WHERE exam_cohort_id = ? AND kind = 'tier' ORDER BY "key", track`, ec.ID)
	if err != nil {
		return nil, err
	}
	out := make([]TierCutoff, 0, len(rows.Values))
	for _, r := range rows.Values {
		v := floatValue(r[2])
		out = append(out, TierCutoff{
			TierKey: stringValue(r[0]),
			Track:   stringValue(r[1]),
			Value:   v,
			Cutoff:  v,
		})
	}
	return out, nil
}

// ExamCutoffImpacts 计算各科各选科分数线下的达线情况。
func (s *Store) ExamCutoffImpacts(ctx context.Context, examID, cohortID int64) ([]ExamCutoffImpact, error) {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	cutoffs, err := s.ListExamSubjectCutoffs(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	out := make([]ExamCutoffImpact, 0, len(cutoffs))
	for _, c := range cutoffs {
		rows, err := s.db.Query(ctx, `SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN es.score >= ? THEN 1 ELSE 0 END), 0)
			FROM exam_scores es
			JOIN exam_participants ep ON ep.id = es.exam_participant_id
			WHERE ep.exam_cohort_id = ? AND es.subject = ? AND ep.track_snapshot = ?
			AND ep.is_valid_for_calc = 1 AND es.score IS NOT NULL AND es.score > 0 AND es.flag = ''`,
			c.Value, ec.ID, c.Subject, c.Track)
		if err != nil {
			return nil, err
		}
		rostered, reached := 0, 0
		if len(rows.Values) > 0 {
			rostered = int(intValue(rows.Values[0][0]))
			reached = int(intValue(rows.Values[0][1]))
		}
		ratio := 0.0
		if rostered > 0 {
			ratio = float64(reached) / float64(rostered)
		}
		out = append(out, ExamCutoffImpact{
			Subject:      c.Subject,
			Track:        c.Track,
			Cutoff:       c.Value,
			Rostered:     rostered,
			Reached:      reached,
			ReachedRatio: ratio,
		})
	}
	return out, nil
}

// UpdateExamRosterTrack 修改考生在单场考试内的选科类别，并原子重算排名快照。
func (s *Store) UpdateExamRosterTrack(ctx context.Context, examID, cohortID, studentID int64, track string) error {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return err
	}
	norm := normalizeTrackValue(track)
	if norm == "" {
		return fmt.Errorf("无效选科类别 %q", track)
	}
	if _, err := s.db.Execute(ctx, `UPDATE exam_participants SET track_snapshot = ?
		WHERE exam_cohort_id = ? AND student_id = ?`, norm, ec.ID, studentID); err != nil {
		return err
	}
	if err := s.recomputeExamCohortRanks(ctx, ec.ID); err != nil {
		return err
	}
	return s.markAssessmentStale(ctx, ec.ID)
}

// UpdateExamScore 修改单个考生的单科成绩或标记，并原子重算该考试的排名快照。
func (s *Store) UpdateExamScore(ctx context.Context, examID, cohortID, studentID int64, subject string, score *float64, flag string) error {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return err
	}
	if score != nil {
		fullMark := 150.0
		foundSubj := false
		if subs, sErr := s.ListExamSubjects(ctx, examID, cohortID); sErr == nil {
			for _, sub := range subs {
				if sub.Subject == subject {
					fullMark = sub.FullMark
					foundSubj = true
					break
				}
			}
		}
		if !foundSubj {
			if marks, mErr := s.subjectFullMarksMap(ctx); mErr == nil {
				if fm, ok := marks[subject]; ok {
					fullMark = fm
				}
			}
		}
		if *score < 0 || *score > fullMark {
			return fmt.Errorf("成绩 %.2f 超出学科满分 %.0f 范围", *score, fullMark)
		}
	}
	pRows, err := s.db.Query(ctx, `SELECT id, COALESCE(class_id, 0) FROM exam_participants
		WHERE exam_cohort_id = ? AND student_id = ?`, ec.ID, studentID)
	if err != nil {
		return err
	}
	if len(pRows.Values) == 0 {
		return ErrNotFound
	}
	partID := intValue(pRows.Values[0][0])
	classID := intValue(pRows.Values[0][1])
	phase := phaseForStage(ec.GradeLevelAtExam)
	tgID := s.resolveStudentSubjectTeachingGroup(ctx, cohortID, phase, studentID, classID, subject)

	if _, err := s.db.Execute(ctx, `INSERT INTO exam_scores(
		exam_participant_id, subject, score, flag, teaching_group_id_snapshot)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(exam_participant_id, subject) DO UPDATE SET
			score = excluded.score,
			flag = excluded.flag,
			teaching_group_id_snapshot = COALESCE(excluded.teaching_group_id_snapshot, exam_scores.teaching_group_id_snapshot)`,
		partID, subject, optionalFloat(score), strings.TrimSpace(flag), nullableID(tgID)); err != nil {
		return err
	}
	if err := s.recomputeExamCohortRanks(ctx, ec.ID); err != nil {
		return err
	}
	return s.markAssessmentStale(ctx, ec.ID)
}

// RecomputeExamRanks 预计算一场考试中某届的总分与全年级/全选科/全班/单科排名快照（同事务写入）。
func (s *Store) RecomputeExamRanks(ctx context.Context, examID, cohortID int64) error {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return err
	}
	return s.recomputeExamCohortRanks(ctx, ec.ID)
}

func (s *Store) recomputeExamCohortRanks(ctx context.Context, examCohortID int64) error {
	pRows, err := s.db.Query(ctx, `SELECT id, COALESCE(class_id, 0), track_snapshot, status_snapshot, total_score, is_valid_for_calc
		FROM exam_participants WHERE exam_cohort_id = ? ORDER BY id`, examCohortID)
	if err != nil {
		return err
	}
	if len(pRows.Values) == 0 {
		return nil
	}
	type partInfo struct {
		id       int64
		classID  int64
		track    string
		valid    bool
		total    *float64
		scoreSum float64
		scoreCnt int
	}
	parts := make(map[int64]*partInfo, len(pRows.Values))
	partOrder := make([]int64, 0, len(pRows.Values))
	for _, r := range pRows.Values {
		id := intValue(r[0])
		tot, _ := nullableFloat(r[4])
		valid := intValue(r[5]) != 0 && isParticipantValidStatus(stringValue(r[3]))
		parts[id] = &partInfo{
			id:      id,
			classID: intValue(r[1]),
			track:   stringValue(r[2]),
			valid:   valid,
			total:   tot,
		}
		partOrder = append(partOrder, id)
	}

	sRows, err := s.db.Query(ctx, `SELECT es.id, es.exam_participant_id, es.subject, es.score, es.flag
		FROM exam_scores es
		JOIN exam_participants ep ON ep.id = es.exam_participant_id
		WHERE ep.exam_cohort_id = ?`, examCohortID)
	if err != nil {
		return err
	}
	type scoreItem struct {
		id      int64
		partID  int64
		subject string
		score   float64
		classID int64
		valid   bool
	}
	var allScores []scoreItem
	var resetScores []int64
	for _, r := range sRows.Values {
		sid := intValue(r[0])
		pid := intValue(r[1])
		subj := stringValue(r[2])
		val, hasVal := nullableFloat(r[3])
		flag := stringValue(r[4])
		p := parts[pid]
		if hasVal && val != nil && *val > 0 && flag == "" && p != nil {
			p.scoreSum += *val
			p.scoreCnt++
			if p.valid {
				allScores = append(allScores, scoreItem{
					id: sid, partID: pid, subject: subj, score: *val, classID: p.classID, valid: true,
				})
				continue
			}
		}
		resetScores = append(resetScores, sid)
	}

	// 若未单独提供导入总分且已有各科成绩，按各科有效成绩累计 total_score。
	var validTotals []*partInfo
	for _, pid := range partOrder {
		p := parts[pid]
		if p.total == nil && p.scoreCnt > 0 {
			sum := p.scoreSum
			p.total = &sum
		}
		if p.valid && p.total != nil && *p.total > 0 {
			validTotals = append(validTotals, p)
		}
	}

	gradeRanks, gradeTotal := competitionRanksAny(validTotals, func(p *partInfo) int64 { return p.id }, func(p *partInfo) float64 { return *p.total })
	trackGroups := map[string][]*partInfo{}
	classGroups := map[int64][]*partInfo{}
	for _, p := range validTotals {
		if p.track != "" {
			trackGroups[p.track] = append(trackGroups[p.track], p)
		}
		if p.classID > 0 {
			classGroups[p.classID] = append(classGroups[p.classID], p)
		}
	}
	trackRanks := map[int64]int{}
	trackTotals := map[int64]int{}
	for _, group := range trackGroups {
		rMap, tot := competitionRanksAny(group, func(p *partInfo) int64 { return p.id }, func(p *partInfo) float64 { return *p.total })
		for pid, rk := range rMap {
			trackRanks[pid] = rk
			trackTotals[pid] = tot
		}
	}
	classRanks := map[int64]int{}
	classTotals := map[int64]int{}
	for _, group := range classGroups {
		rMap, tot := competitionRanksAny(group, func(p *partInfo) int64 { return p.id }, func(p *partInfo) float64 { return *p.total })
		for pid, rk := range rMap {
			classRanks[pid] = rk
			classTotals[pid] = tot
		}
	}

	var statements []sqlite.Statement
	for _, pid := range partOrder {
		p := parts[pid]
		var gr, gt, tr, tt, cr, ct any
		if rk, ok := gradeRanks[pid]; ok {
			gr, gt = rk, gradeTotal
		}
		if rk, ok := trackRanks[pid]; ok {
			tr, tt = rk, trackTotals[pid]
		}
		if rk, ok := classRanks[pid]; ok {
			cr, ct = rk, classTotals[pid]
		}
		statements = append(statements, sqlite.Statement{
			SQL: `UPDATE exam_participants
				SET total_score = ?, grade_rank = ?, grade_total_count = ?,
					track_rank = ?, track_total_count = ?, class_rank = ?, class_total_count = ?
				WHERE id = ?`,
			Args: []any{optionalFloat(p.total), gr, gt, tr, tt, cr, ct, pid},
		})
	}

	// 单科年级排名与班级排名。
	bySubject := map[string][]scoreItem{}
	for _, item := range allScores {
		bySubject[item.subject] = append(bySubject[item.subject], item)
	}
	subjGradeRank := map[int64]int{}
	subjClassRank := map[int64]int{}
	for _, group := range bySubject {
		ptrs := make([]*scoreItem, len(group))
		byClass := map[int64][]*scoreItem{}
		for i := range group {
			ptrs[i] = &group[i]
			if group[i].classID > 0 {
				byClass[group[i].classID] = append(byClass[group[i].classID], &group[i])
			}
		}
		gMap, _ := competitionRanksAny(ptrs, func(item *scoreItem) int64 { return item.id }, func(item *scoreItem) float64 { return item.score })
		for id, rk := range gMap {
			subjGradeRank[id] = rk
		}
		for _, cGroup := range byClass {
			cMap, _ := competitionRanksAny(cGroup, func(item *scoreItem) int64 { return item.id }, func(item *scoreItem) float64 { return item.score })
			for id, rk := range cMap {
				subjClassRank[id] = rk
			}
		}
	}
	for _, item := range allScores {
		var cr any
		if rk, ok := subjClassRank[item.id]; ok {
			cr = rk
		}
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE exam_scores SET subject_grade_rank = ?, subject_class_rank = ? WHERE id = ?`,
			Args: []any{subjGradeRank[item.id], cr, item.id},
		})
	}
	for _, sid := range resetScores {
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE exam_scores SET subject_grade_rank = NULL, subject_class_rank = NULL WHERE id = ?`,
			Args: []any{sid},
		})
	}

	if len(validTotals) > 0 || len(allScores) > 0 {
		statements = append(statements, sqlite.Statement{
			SQL: `UPDATE exams SET status = 'scored'
				WHERE id = (SELECT exam_id FROM exam_cohorts WHERE id = ?) AND status = 'draft'`,
			Args: []any{examCohortID},
		})
	}

	_, err = s.db.Transaction(ctx, statements)
	return err
}

// ArchiveExam 将考试状态更新为已归档并推进修订号。
func (s *Store) ArchiveExam(ctx context.Context, examID int64) error {
	if _, err := s.GetExam(ctx, examID); err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Transaction(ctx, []sqlite.Statement{
		{
			SQL:  `UPDATE exams SET status = 'archived' WHERE id = ?`,
			Args: []any{examID},
		},
		bumpRevisionStatement(fmt.Sprintf("exam:%d", examID), now),
		auditLogStatement("archive_exam", "exam", examID, map[string]any{"status": "archived"}, now),
	})
	return err
}

// ListExamRosterDrift 对比参考名单快照与当前学生主档，找出班级或选科发生变化的考生。
func (s *Store) ListExamRosterDrift(ctx context.Context, examID, cohortID int64) ([]ExamRosterDriftItem, error) {
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT ep.student_id, st.student_no, st.name,
		COALESCE(ep.class_id, 0), COALESCE(NULLIF(ep.class_name_snapshot, ''), COALESCE(ec_cls.name, '')),
		ep.track_snapshot, ep.elective_snapshot,
		COALESCE(st.current_class_id, 0), COALESCE(st_cls.name, ''),
		st.current_track, st.elective_combo
		FROM exam_participants ep
		JOIN students st ON st.id = ep.student_id
		LEFT JOIN classes ec_cls ON ec_cls.id = ep.class_id
		LEFT JOIN classes st_cls ON st_cls.id = st.current_class_id
		WHERE ep.exam_cohort_id = ?
		ORDER BY st.student_no, ep.id`, ec.ID)
	if err != nil {
		return nil, err
	}
	var out []ExamRosterDriftItem
	for _, r := range rows.Values {
		examCID := intValue(r[3])
		examClass := strings.TrimSpace(stringValue(r[4]))
		examTrack := normalizeTrackValue(stringValue(r[5]))
		examElective := strings.TrimSpace(stringValue(r[6]))
		curCID := intValue(r[7])
		curClass := strings.TrimSpace(stringValue(r[8]))
		curTrack := normalizeTrackValue(stringValue(r[9]))
		curElective := strings.TrimSpace(stringValue(r[10]))

		classChanged := false
		if examClass != "" && normalizeClassName(examClass) != normalizeClassName(curClass) {
			classChanged = true
		} else if examCID > 0 && curCID > 0 && examCID != curCID {
			classChanged = true
		}

		trackChanged := false
		if examTrack != "" && examTrack != curTrack {
			trackChanged = true
		} else if examElective != "" && examElective != curElective {
			trackChanged = true
		}

		if !classChanged && !trackChanged {
			continue
		}
		out = append(out, ExamRosterDriftItem{
			StudentID:     intValue(r[0]),
			StudentNo:     stringValue(r[1]),
			StudentName:   stringValue(r[2]),
			ExamClassID:   examCID,
			ExamClassName: examClass,
			ExamTrack:     examTrack,
			ExamElective:  examElective,
			CurClassID:    curCID,
			CurClassName:  curClass,
			CurTrack:      curTrack,
			CurElective:   curElective,
			ClassChanged:  classChanged,
			TrackChanged:  trackChanged,
		})
	}
	return out, nil
}

// SyncExamRosterToStudents 将参考名单中变更的班级和选科同步更新至学生主档，并记录学籍异动时间线事件。
func (s *Store) SyncExamRosterToStudents(ctx context.Context, examID, cohortID int64) (int, error) {
	drifts, err := s.ListExamRosterDrift(ctx, examID, cohortID)
	if err != nil || len(drifts) == 0 {
		return 0, err
	}
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return 0, err
	}
	phase := phaseForStage(ec.GradeLevelAtExam)
	classMap, err := s.phaseClassIDMap(ctx, cohortID, phase)
	if err != nil {
		return 0, err
	}

	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	for _, item := range drifts {
		nextClassID := item.CurClassID
		if item.ClassChanged {
			if item.ExamClassID > 0 {
				nextClassID = item.ExamClassID
			} else if cid := classMap[normalizeClassName(item.ExamClassName)]; cid > 0 {
				nextClassID = cid
			}
		}
		nextTrack := item.CurTrack
		if item.ExamTrack != "" {
			nextTrack = item.ExamTrack
		}
		nextElective := item.CurElective
		if item.ExamElective != "" {
			nextElective = item.ExamElective
		}
		statements = append(statements, sqlite.Statement{
			SQL: `UPDATE students
				SET current_class_id = ?, current_track = ?, elective_combo = ?, updated_at = ?
				WHERE id = ?`,
			Args: []any{nullableID(nextClassID), nextTrack, nextElective, now, item.StudentID},
		})
		if item.ClassChanged {
			beforeJSON, _ := json.Marshal(map[string]any{"class_id": item.CurClassID, "class_name": item.CurClassName})
			afterJSON, _ := json.Marshal(map[string]any{"class_id": nextClassID, "class_name": item.ExamClassName})
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO student_events(student_id, cohort_id, exam_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
					VALUES (?, ?, ?, ?, 'class_changed', ?, ?, '', ?)`,
				Args: []any{item.StudentID, cohortID, examID, phase, string(beforeJSON), string(afterJSON), now},
			})
		}
		if item.TrackChanged {
			beforeJSON, _ := json.Marshal(map[string]any{"track": item.CurTrack, "elective": item.CurElective})
			afterJSON, _ := json.Marshal(map[string]any{"track": nextTrack, "elective": nextElective})
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO student_events(student_id, cohort_id, exam_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
					VALUES (?, ?, ?, ?, 'track_changed', ?, ?, '', ?)`,
				Args: []any{item.StudentID, cohortID, examID, phase, string(beforeJSON), string(afterJSON), now},
			})
		}
	}
	statements = append(statements,
		bumpRevisionStatement(fmt.Sprintf("cohort:%d", cohortID), now),
		auditLogStatement("sync_exam_roster", "exam_cohort", ec.ID, map[string]any{"synced": len(drifts)}, now),
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return 0, err
	}
	return len(drifts), nil
}

func competitionRanksAny[T any](items []T, idFn func(T) int64, scoreFn func(T) float64) (map[int64]int, int) {
	sorted := append([]T(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		return scoreFn(sorted[i]) > scoreFn(sorted[j])
	})
	ranks := make(map[int64]int, len(sorted))
	for i, item := range sorted {
		rank := i + 1
		if i > 0 && math.Abs(scoreFn(sorted[i-1])-scoreFn(item)) < 1e-9 {
			rank = ranks[idFn(sorted[i-1])]
		}
		ranks[idFn(item)] = rank
	}
	return ranks, len(sorted)
}

// ExamQualityIssues 返回一场考试中某个年级的质量检查结果。
func (s *Store) ExamQualityIssues(ctx context.Context, examID, cohortID int64) ([]ExamQualityIssue, error) {
	_, issues, err := s.ExamPrep(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// ExamPrep 返回考试某个年级的考前就绪检查与质量问题详情。
func (s *Store) ExamPrep(ctx context.Context, examID, cohortID int64) (ExamPrepSummary, []ExamQualityIssue, error) {
	exam, err := s.GetExam(ctx, examID)
	if err != nil {
		return ExamPrepSummary{}, nil, err
	}
	ec, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return ExamPrepSummary{}, nil, err
	}
	phase := phaseForStage(ec.GradeLevelAtExam)
	classCnt, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM classes WHERE cohort_id = ? AND phase = ?`, cohortID, phase)
	missingTrack, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM exam_participants WHERE exam_cohort_id = ? AND track_snapshot = ''`, ec.ID)
	scoreCnt, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM exam_scores es JOIN exam_participants ep ON ep.id = es.exam_participant_id WHERE ep.exam_cohort_id = ?`, ec.ID)

	assessment, err := s.findAssessmentByExamCohort(ctx, ec.ID)
	statGroupCnt := 0
	targetCnt := 0
	zeroTarget := 0
	if err == nil {
		statGroupCnt, _ = s.countQuery(ctx, `SELECT COUNT(*) FROM assessment_stat_groups WHERE assessment_id = ?`, assessment.ID)
		targetCnt, _ = s.countQuery(ctx, `SELECT COUNT(*) FROM assessment_class_targets WHERE assessment_id = ?`, assessment.ID)
		zeroTarget, _ = s.countQuery(ctx, `SELECT COUNT(*) FROM assessment_class_targets act
			WHERE act.assessment_id = ? AND COALESCE(act.target_count, 0) = 0
			AND NOT EXISTS (SELECT 1 FROM assessment_target_adjustments adj
				WHERE adj.assessment_id = act.assessment_id AND adj.class_id = act.class_id AND adj.excluded = 1)`,
			assessment.ID)
	}

	var issues []ExamQualityIssue
	if ec.RosterCount == 0 {
		issues = append(issues, ExamQualityIssue{Severity: "blocking", Code: "no_roster"})
	}
	if missingTrack > 0 {
		mtRows, _ := s.db.Query(ctx, `SELECT st.student_no, st.name, ep.source_row, COALESCE(ep.import_run_id, 0)
			FROM exam_participants ep JOIN students st ON st.id = ep.student_id
			WHERE ep.exam_cohort_id = ? AND ep.track_snapshot = ''
			ORDER BY st.student_no`, ec.ID)
		for _, r := range mtRows.Values {
			issues = append(issues, ExamQualityIssue{
				Severity:    "blocking",
				Code:        "missing_track",
				StudentNo:   stringValue(r[0]),
				Name:        stringValue(r[1]),
				Row:         int(intValue(r[2])),
				ImportRunID: intValue(r[3]),
			})
		}
	}
	msRows, _ := s.db.Query(ctx, `SELECT st.student_no, st.name, ep.source_row, COALESCE(ep.import_run_id, 0)
		FROM exam_participants ep
		JOIN students st ON st.id = ep.student_id
		WHERE ep.exam_cohort_id = ?
		AND NOT EXISTS (SELECT 1 FROM exam_scores es WHERE es.exam_participant_id = ep.id)
		ORDER BY st.student_no LIMIT 200`, ec.ID)
	for _, r := range msRows.Values {
		issues = append(issues, ExamQualityIssue{
			Severity:    "warning",
			Code:        "missing_scores",
			StudentNo:   stringValue(r[0]),
			Name:        stringValue(r[1]),
			Row:         int(intValue(r[2])),
			ImportRunID: intValue(r[3]),
		})
	}
	examSubjects, _ := s.ListExamSubjects(ctx, examID, cohortID)
	fullMarks := map[string]float64{}
	for _, sub := range examSubjects {
		if sub.Enabled {
			fullMarks[sub.Subject] = sub.FullMark
		}
	}
	if len(fullMarks) > 0 {
		scRows, _ := s.db.Query(ctx, `SELECT st.student_no, st.name, es.subject, es.score, COALESCE(es.import_run_id, 0), es.source_row
			FROM exam_scores es
			JOIN exam_participants ep ON ep.id = es.exam_participant_id
			JOIN students st ON st.id = ep.student_id
			WHERE ep.exam_cohort_id = ? AND es.score IS NOT NULL
			ORDER BY st.student_no, es.subject`, ec.ID)
		outOfRange := 0
		for _, r := range scRows.Values {
			if outOfRange >= 200 {
				break
			}
			subj := stringValue(r[2])
			full, ok := fullMarks[subj]
			if !ok {
				continue
			}
			sc := floatValue(r[3])
			if sc < 0 || sc > full {
				outOfRange++
				issues = append(issues, ExamQualityIssue{
					Severity:    "blocking",
					Code:        "score_out_of_range",
					StudentNo:   stringValue(r[0]),
					Name:        stringValue(r[1]),
					Subject:     subj,
					Args:        []string{fmt.Sprintf("%g", sc), fmt.Sprintf("%g", full)},
					ImportRunID: intValue(r[4]),
					Row:         int(intValue(r[5])),
				})
			}
		}
	}
	if assessment.ID > 0 {
		if zeroTarget > 0 {
			ztRows, _ := s.db.Query(ctx, `SELECT act.class_id, COALESCE(c.name, ''), act.tier_key
				FROM assessment_class_targets act
				LEFT JOIN classes c ON c.id = act.class_id
				WHERE act.assessment_id = ? AND COALESCE(act.target_count, 0) = 0
				AND NOT EXISTS (SELECT 1 FROM assessment_target_adjustments adj
					WHERE adj.assessment_id = act.assessment_id AND adj.class_id = act.class_id AND adj.excluded = 1)`,
				assessment.ID)
			for _, r := range ztRows.Values {
				issues = append(issues, ExamQualityIssue{
					Severity:  "warning",
					Code:      "zero_class_target",
					ClassID:   intValue(r[0]),
					ClassName: stringValue(r[1]),
					TierKey:   stringValue(r[2]),
				})
			}
		}
		if groups, gErr := s.ListExamStatGroups(ctx, assessment.ID); gErr == nil {
			for _, group := range groups {
				if len(group.Members) == 0 {
					issues = append(issues, ExamQualityIssue{
						Severity: "blocking",
						Code:     "empty_stat_group",
						Subject:  group.Subject,
						Args:     []string{group.Name},
					})
				}
				if group.Target == nil || *group.Target == 0 {
					issues = append(issues, ExamQualityIssue{
						Severity: "warning",
						Code:     "zero_stat_group_target",
						Subject:  group.Subject,
						Args:     []string{group.Name},
					})
				}
			}
		}
	}
	existingSubjectCutoffs := map[string]bool{}
	cutRows, _ := s.db.Query(ctx, `SELECT DISTINCT "key" FROM exam_cutoffs WHERE exam_cohort_id = ? AND kind = 'subject'`, ec.ID)
	for _, r := range cutRows.Values {
		existingSubjectCutoffs[stringValue(r[0])] = true
	}
	for _, sub := range examSubjects {
		if sub.Enabled && !existingSubjectCutoffs[sub.Subject] {
			issues = append(issues, ExamQualityIssue{
				Severity: "warning",
				Code:     "missing_cutoff",
				Subject:  sub.Subject,
			})
		}
	}

	cutoffCnt, _ := s.countQuery(ctx, `SELECT COUNT(*) FROM exam_cutoffs WHERE exam_cohort_id = ?`, ec.ID)
	return ExamPrepSummary{
		ExamID:       examID,
		ExamName:     exam.Name,
		CohortID:     cohortID,
		CohortName:   ec.CohortName,
		GradeLevel:   ec.GradeLevelAtExam,
		RosterCount:  ec.RosterCount,
		Roster:       ec.RosterCount,
		Scores:       scoreCnt,
		Cutoffs:      cutoffCnt,
		Targets:      targetCnt,
		ClassCount:   classCnt,
		GroupCount:   statGroupCnt,
		StatGroups:   statGroupCnt,
		TargetCount:  targetCnt,
		SubjectCount: ec.SubjectCount,
		MissingTrack: missingTrack,
		ZeroTarget:   zeroTarget,
		Issues:       issues,
	}, issues, nil
}

// -----------------------------------------------------------------------------
// Internal Exam Helpers
// -----------------------------------------------------------------------------

type rosterIndex struct {
	byStudentNo map[string]ExamRosterEntry
	byCandCode  map[string]ExamRosterEntry
	byNameClass map[string]ExamRosterEntry
	ambigName   map[string]bool
}

func (r rosterIndex) find(studentNo string) (ExamRosterEntry, bool) {
	entry, ok := r.byStudentNo[strings.TrimSpace(studentNo)]
	return entry, ok
}

func (r rosterIndex) findMulti(studentNo, candidateCode, name, className string) (ExamRosterEntry, bool) {
	if s := strings.TrimSpace(studentNo); s != "" {
		if e, ok := r.byStudentNo[s]; ok {
			return e, true
		}
	}
	if c := strings.TrimSpace(candidateCode); c != "" {
		if e, ok := r.byCandCode[c]; ok {
			return e, true
		}
	}
	if n := strings.TrimSpace(name); n != "" {
		if cn := normalizeClassName(className); cn != "" {
			key := n + "\x00" + cn
			if !r.ambigName[key] {
				if e, ok := r.byNameClass[key]; ok {
					return e, true
				}
			}
		}
	}
	return ExamRosterEntry{}, false
}

func (s *Store) examRosterUserIndex(ctx context.Context, ec ExamCohort) (rosterIndex, error) {
	idx := rosterIndex{
		byStudentNo: map[string]ExamRosterEntry{},
		byCandCode:  map[string]ExamRosterEntry{},
		byNameClass: map[string]ExamRosterEntry{},
		ambigName:   map[string]bool{},
	}
	rows, err := s.db.Query(ctx, `SELECT ep.id, ep.student_id, st.student_no, COALESCE(NULLIF(ep.candidate_no, ''), st.candidate_code), st.name,
		COALESCE(ep.class_id, 0), ep.class_name_snapshot, ep.track_snapshot
		FROM exam_participants ep
		JOIN students st ON st.id = ep.student_id
		WHERE ep.exam_cohort_id = ?`, ec.ID)
	if err != nil {
		return idx, err
	}
	for _, r := range rows.Values {
		entry := ExamRosterEntry{
			ID:          intValue(r[0]),
			StudentID:   intValue(r[1]),
			StudentNo:   stringValue(r[2]),
			CandidateNo: stringValue(r[3]),
			Name:        stringValue(r[4]),
			ClassID:     intValue(r[5]),
			ClassName:   stringValue(r[6]),
			Track:       stringValue(r[7]),
		}
		if entry.StudentNo != "" {
			idx.byStudentNo[entry.StudentNo] = entry
		}
		if entry.CandidateNo != "" {
			idx.byCandCode[entry.CandidateNo] = entry
		}
		if entry.Name != "" && entry.ClassName != "" {
			key := entry.Name + "\x00" + normalizeClassName(entry.ClassName)
			if _, dup := idx.byNameClass[key]; dup {
				idx.ambigName[key] = true
			} else {
				idx.byNameClass[key] = entry
			}
		}
	}
	return idx, nil
}

func (s *Store) resolveStudentSubjectTeachingGroup(ctx context.Context, cohortID int64, phase string, studentID, classID int64, subject string) int64 {
	if !ValidSchoolPhase(phase) || subject == "" {
		return 0
	}
	// 优先走班归属，其次行政班常规教学班。
	if studentID > 0 {
		wRows, err := s.db.Query(ctx, `SELECT teaching_group_id FROM teaching_group_students
			WHERE student_id = ? AND cohort_id = ? AND phase = ? AND subject = ? AND ended_at IS NULL LIMIT 1`,
			studentID, cohortID, phase, subject)
		if err == nil && len(wRows.Values) > 0 {
			return intValue(wRows.Values[0][0])
		}
	}
	if classID > 0 {
		aRows, err := s.db.Query(ctx, `SELECT id FROM teaching_groups
			WHERE cohort_id = ? AND phase = ? AND subject = ? AND kind = 'admin' AND admin_class_id = ? LIMIT 1`,
			cohortID, phase, subject, classID)
		if err == nil && len(aRows.Values) > 0 {
			return intValue(aRows.Values[0][0])
		}
	}
	return 0
}

func (s *Store) phaseClassIDMap(ctx context.Context, cohortID int64, phase string) (map[string]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name FROM classes WHERE cohort_id = ? AND phase = ?`, cohortID, phase)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows.Values))
	for _, r := range rows.Values {
		out[normalizeClassName(stringValue(r[1]))] = intValue(r[0])
	}
	return out, nil
}

func isParticipantValidStatus(status string) bool {
	switch status {
	case "", "active":
		return true
	default:
		return false
	}
}

func normalizeStudentStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active", "在籍", "在读":
		return "active"
	case "transferred", "transferred_out", "转出":
		return "transferred"
	case "suspended", "休学":
		return "suspended"
	case "borrowed", "借读":
		return "borrowed"
	case "left_school", "退学", "离校":
		return "left_school"
	case "graduated", "毕业":
		return "graduated"
	default:
		return "active"
	}
}

func trackFromElective(elective string) string {
	if strings.Contains(elective, "物") {
		return "physics"
	}
	if strings.Contains(elective, "历") || strings.Contains(elective, "史") {
		return "history"
	}
	return ""
}

func electiveSubjects(elective string) []string {
	var subjects []string
	for _, item := range []struct {
		token string
		key   string
	}{
		{"物", "physics"},
		{"化", "chemistry"},
		{"生", "biology"},
		{"政", "politics"},
		{"历", "history"},
		{"史", "history"},
		{"地", "geography"},
	} {
		if strings.Contains(elective, item.token) {
			dup := false
			for _, s := range subjects {
				if s == item.key {
					dup = true
					break
				}
			}
			if !dup {
				subjects = append(subjects, item.key)
			}
		}
	}
	return subjects
}

func nullableIntPtr(v any) *int {
	if n, ok := nullableInt64(v); ok {
		iv := int(n)
		return &iv
	}
	return nil
}
