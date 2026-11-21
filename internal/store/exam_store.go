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

	"tally/internal/assessment"
	"tally/internal/school"
)

var (
	// ErrExamExists 表示同一学年、学期、类型和名称的考试已经存在。
	ErrExamExists = errors.New("考试已经存在")
	// ErrExamCohort 表示考试没有指定的级。
	ErrExamCohort = errors.New("考试没有这个级")
	// ErrExamTrackMissing 表示报名名单无法确定选科类别。
	ErrExamTrackMissing = errors.New("报名名单缺少选科类别")
)

const (
	ExamTermFirst  = "autumn"
	ExamTermSecond = "spring"

	ExamMidterm = "midterm"
	ExamFinal   = "final"
	ExamGaokao  = "gaokao"
)

// Exam 是一场覆盖一个或者多个级的考试。
type Exam struct {
	ID                int64
	AcademicYearStart int
	Term              string
	ExamType          string
	Name              string
	ExamDate          string
	Sequence          int
	Status            string
	CreatedAt         time.Time
	Cohorts           []ExamCohort
}

// ExamCohort 是考试中的一个级与考试当时的阶段。
type ExamCohort struct {
	ID                int64
	ExamID            int64
	CohortID          int64
	StageID           int64
	GradeLevelAtExam  string
	CohortName        string
	EnrollmentYear    int
	GraduationYear    int
	RosterCount       int
	ScoreCount        int
	SubjectCount      int
	ClassTargetCount  int
	StatGroupCount    int
	AcademicYearStart int
}

// CreateExamInput 是新建考试需要的字段。
type CreateExamInput struct {
	AcademicYearStart int
	Term              string
	ExamType          string
	Name              string
	ExamDate          string
	Sequence          int
	CohortIDs         []int64
	SubjectMarks      map[int64]map[string]float64
}

// ExamSubject 是一场考试中一个级的科目满分。
type ExamSubject struct {
	ExamID   int64
	CohortID int64
	Subject  string
	FullMark float64
	Enabled  bool
}

// ImportRoster 是报名名单中的一行。
type ImportRoster struct {
	StudentNo   string
	Name        string
	ClassName   string
	ExamNo      string
	CandidateNo string
	Track       string
	Elective    string
	Status      string
	TotalScore  *float64
	SourceSheet string
	SourceRow   int
}

// ExamRosterEntry 是报名名单中的一名学生。
type ExamRosterEntry struct {
	ID          int64
	StudentID   int64
	StudentNo   string
	Name        string
	ClassID     int64
	ClassName   string
	ExamNo      string
	CandidateNo string
	Track       string
	Status      string
	TotalScore  *float64
	Subjects    []string
	SourceSheet string
	SourceRow   int
	ImportRunID int64
}

// ExamRosterFilter 是报名名单的查询条件。
type ExamRosterFilter struct {
	ExamID   int64
	CohortID int64
	Query    string
	Page     int
	PageSize int
}

// ExamRosterSummary 是生成报名名单后的数量。
type ExamRosterSummary struct {
	Students     int
	Inserted     int
	Existing     int
	MissingTrack int
}

// ExamScoreRow 是一条考试成绩。
type ExamScoreRow struct {
	StudentID   int64
	StudentNo   string
	Name        string
	ClassID     int64
	ClassName   string
	Subject     string
	Score       *float64
	Flag        string
	SourceSheet string
	SourceRow   int
	ImportRunID int64
}

// ExamScoreFilter 是成绩查询条件。
type ExamScoreFilter struct {
	ExamID   int64
	CohortID int64
	Subject  string
	ClassID  int64
	Query    string
	Page     int
	PageSize int
}

// ExamScoreSummary 是一个科目已录入成绩的数量与分布。
type ExamScoreSummary struct {
	Subject string
	Count   int
	Missing int
	Average float64
	Minimum float64
	Maximum float64
}

// ExamScoreBand 是成绩分布中的一个分数区间。
type ExamScoreBand struct {
	Subject string
	Lower   float64
	Upper   float64
	Count   int
}

// ExamClassAverage 是一个行政班在某科目的均分与人数。
type ExamClassAverage struct {
	Subject   string
	ClassID   int64
	ClassName string
	Count     int
	Average   float64
}

// ExamCutoffImpact 是一条分数线达到人数与占比。
type ExamCutoffImpact struct {
	Subject      string
	Track        string
	Cutoff       float64
	Rostered     int
	Reached      int
	ReachedRatio float64
}

// ExamQualitySummary 是一场考试中一个级的问题汇总。
type ExamQualitySummary struct {
	ExamID        int64
	ExamName      string
	CohortID      int64
	CohortName    string
	GradeLevel    string
	AcademicStart int
	Issues        []ExamQualityIssue
}

// ExamSubjectCutoff 是单科本科线。
type ExamSubjectCutoff struct {
	ID       int64
	ExamID   int64
	CohortID int64
	Subject  string
	Track    string
	Cutoff   float64
}

// ExamTierCutoff 是总分档位线。
type ExamTierCutoff struct {
	ID       int64
	ExamID   int64
	CohortID int64
	TierKey  string
	Track    string
	Cutoff   float64
}

// Assessment 是一场考试中一个级的本次考核。
type Assessment struct {
	ID           int64
	ExamCohortID int64
	PlanVersion  int64
	Status       string
	CurrentRunID int64
	Revision     int
}

// AssessmentClassTarget 是一个行政班某个档位的目标人数。
type AssessmentClassTarget struct {
	ClassID    int64
	ClassName  string
	TierKey    string
	Target     float64
	HasTarget  bool
	Adjustment string
}

// StatGroupMember 是合并组中的一个教学班。
type StatGroupMember struct {
	TeachingGroupID int64
	Subject         string
	Name            string
	ClassNames      string
	Students        int
}

// ExamStatGroup 是单科达线率的计算单位。
type ExamStatGroup struct {
	ID              int64
	AssessmentID    int64
	Subject         string
	Name            string
	LeadClassID     int64
	LeadClassName   string
	Target          *float64
	BaselineAverage float64
	BaselineRatio   float64
	BaselineSource  string
	Explicit        bool
	Members         []StatGroupMember
}

// StatGroupInput 是新建或者修改合并组需要的字段。
type StatGroupInput struct {
	ID              int64
	AssessmentID    int64
	Subject         string
	Name            string
	LeadClassID     int64
	Target          *float64
	MemberGroupIDs  []int64
	BaselineAverage float64
	BaselineRatio   float64
	BaselineSource  string
}

// ExamQualityIssue 是考试数据中的一条问题。
type ExamQualityIssue struct {
	Severity    string
	Code        string
	Row         int
	ImportRunID int64
	StudentNo   string
	Name        string
	ClassName   string
	Subject     string
	TierKey     string
	Args        []string
}

// ExamPrepStatus 是考试准备清单的数量。
type ExamPrepStatus struct {
	Roster     int
	Scores     int
	Cutoffs    int
	Targets    int
	StatGroups int
	Blocking   int
	Warnings   int
}

func examSchema() []sqlite.Statement {
	return []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS exams (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			academic_year_start INTEGER NOT NULL,
			term TEXT NOT NULL,
			exam_type TEXT NOT NULL,
			name TEXT NOT NULL,
			exam_date TEXT NOT NULL DEFAULT '',
			sequence INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'draft',
			created_at TEXT NOT NULL,
			UNIQUE(academic_year_start, term, exam_type, name)
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS exams_date ON exams(academic_year_start, exam_date)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_cohorts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_id INTEGER NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			stage_id INTEGER NOT NULL REFERENCES cohort_stages(id),
			grade_level_at_exam TEXT NOT NULL,
			UNIQUE(exam_id, cohort_id)
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS exam_cohorts_cohort ON exam_cohorts(cohort_id, exam_id)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_subjects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_id INTEGER NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			subject TEXT NOT NULL,
			full_mark REAL NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			UNIQUE(exam_id, cohort_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_rosters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_id INTEGER NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			student_id INTEGER NOT NULL REFERENCES students(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			exam_no TEXT NOT NULL DEFAULT '',
			candidate_no TEXT NOT NULL DEFAULT '',
			track TEXT NOT NULL DEFAULT '',
			total_score REAL,
			status TEXT NOT NULL DEFAULT 'active',
			subjects_json TEXT NOT NULL DEFAULT '[]',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(exam_id, cohort_id, student_id)
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS exam_rosters_no
			ON exam_rosters(exam_id, exam_no) WHERE exam_no <> ''`},
		{SQL: `CREATE INDEX IF NOT EXISTS exam_rosters_candidate
			ON exam_rosters(exam_id, candidate_no)`},
		{SQL: `CREATE INDEX IF NOT EXISTS exam_rosters_cohort
			ON exam_rosters(exam_id, cohort_id, class_id)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_id INTEGER NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			student_id INTEGER NOT NULL REFERENCES students(id),
			subject TEXT NOT NULL,
			score REAL,
			flag TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(exam_id, cohort_id, student_id, subject)
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS exam_scores_subject
			ON exam_scores(exam_id, cohort_id, subject)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_subject_cutoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_id INTEGER NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			subject TEXT NOT NULL,
			track TEXT NOT NULL,
			cutoff REAL NOT NULL,
			UNIQUE(exam_id, cohort_id, subject, track)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exam_tier_cutoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_id INTEGER NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			tier_key TEXT NOT NULL,
			track TEXT NOT NULL,
			cutoff REAL NOT NULL,
			UNIQUE(exam_id, cohort_id, tier_key, track)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exam_cohort_id INTEGER NOT NULL UNIQUE REFERENCES exam_cohorts(id) ON DELETE CASCADE,
			plan_version_id INTEGER,
			status TEXT NOT NULL DEFAULT 'draft',
			current_run_id INTEGER,
			revision INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS assessments_status ON assessments(status)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_class_targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			class_id INTEGER NOT NULL REFERENCES classes(id),
			tier_key TEXT NOT NULL,
			target_count REAL NOT NULL,
			UNIQUE(assessment_id, class_id, tier_key)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_stat_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			subject TEXT NOT NULL,
			name TEXT NOT NULL,
			lead_class_id INTEGER NOT NULL REFERENCES classes(id),
			baseline_average REAL NOT NULL DEFAULT 0,
			baseline_ratio REAL NOT NULL DEFAULT 0,
			baseline_source TEXT NOT NULL DEFAULT '',
			explicit INTEGER NOT NULL DEFAULT 0,
			UNIQUE(assessment_id, subject, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_stat_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			stat_group_id INTEGER NOT NULL REFERENCES assessment_stat_groups(id) ON DELETE CASCADE,
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id),
			UNIQUE(stat_group_id, teaching_group_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_stat_group_targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			stat_group_id INTEGER NOT NULL UNIQUE REFERENCES assessment_stat_groups(id) ON DELETE CASCADE,
			target_count REAL NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assessment_target_adjustments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			scope_type TEXT NOT NULL,
			scope_id INTEGER NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			reason TEXT NOT NULL,
			value REAL,
			created_at TEXT NOT NULL
		)`},
	}
}

// CreateExam 建立一场考试，并为参加的级写入阶段快照、科目满分和本次考核。
func (s *Store) CreateExam(ctx context.Context, input CreateExamInput) (Exam, error) {
	if input.AcademicYearStart < 1990 || input.AcademicYearStart > 3000 {
		return Exam{}, fmt.Errorf("学年超出允许范围")
	}
	switch input.Term {
	case ExamTermFirst, ExamTermSecond:
	default:
		return Exam{}, fmt.Errorf("学期必须是 autumn 或 spring")
	}
	switch input.ExamType {
	case ExamMidterm, ExamFinal, ExamGaokao:
	default:
		return Exam{}, fmt.Errorf("考试类型必须是 midterm、final 或 gaokao")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return Exam{}, fmt.Errorf("考试名称不能为空")
	}
	if _, err := time.Parse("2006-01-02", input.ExamDate); err != nil {
		return Exam{}, fmt.Errorf("考试日期必须是 2006-01-02 格式")
	}
	if len(input.CohortIDs) == 0 {
		return Exam{}, fmt.Errorf("至少选择一个级")
	}
	seen := map[int64]bool{}
	type cohortStage struct {
		cohort Cohort
		stage  CohortStage
	}
	stages := make([]cohortStage, 0, len(input.CohortIDs))
	for _, cohortID := range input.CohortIDs {
		if seen[cohortID] {
			continue
		}
		seen[cohortID] = true
		cohort, err := s.GetCohort(ctx, cohortID)
		if err != nil {
			return Exam{}, err
		}
		if cohort.Status != "active" {
			return Exam{}, fmt.Errorf("%s 已经归档", cohort.Name)
		}
		index := school.StageIndex(cohort.EnrollmentYear, input.AcademicYearStart)
		if index < school.StageFirst || index > school.StageThird {
			return Exam{}, fmt.Errorf("%s 在 %d 学年没有可用的年级阶段", cohort.Name, input.AcademicYearStart)
		}
		stage, err := s.GetCohortStage(ctx, cohort.ID, input.AcademicYearStart)
		if errors.Is(err, ErrNotFound) {
			stage, err = s.EnsureCohortStage(ctx, cohort.ID, input.AcademicYearStart, school.StageKey(index))
		}
		if err != nil {
			return Exam{}, err
		}
		stages = append(stages, cohortStage{cohort: cohort, stage: stage})
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM exams WHERE academic_year_start = ? AND term = ?
		AND exam_type = ? AND name = ?`, input.AcademicYearStart, input.Term, input.ExamType, input.Name)
	if err != nil {
		return Exam{}, err
	}
	if len(rows.Values) > 0 {
		return Exam{}, ErrExamExists
	}
	subjects, err := s.ListSubjects(ctx)
	if err != nil {
		return Exam{}, err
	}
	planVersionID, err := s.defaultPlanVersionID(ctx)
	if err != nil {
		return Exam{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	statements := []sqlite.Statement{{
		SQL: `INSERT INTO exams(academic_year_start, term, exam_type, name, exam_date, sequence, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'draft', ?)`,
		Args: []any{input.AcademicYearStart, input.Term, input.ExamType, input.Name, input.ExamDate, input.Sequence, now},
	}}
	for _, item := range stages {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO exam_cohorts(exam_id, cohort_id, stage_id, grade_level_at_exam)
				SELECT id, ?, ?, ? FROM exams
				WHERE academic_year_start = ? AND term = ? AND exam_type = ? AND name = ?`,
			Args: []any{item.cohort.ID, item.stage.ID, item.stage.GradeLevel,
				input.AcademicYearStart, input.Term, input.ExamType, input.Name},
		})
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assessments(exam_cohort_id, plan_version_id, status, revision, created_at)
				SELECT ec.id, ?, 'draft', 1, ? FROM exam_cohorts ec
				JOIN exams e ON e.id = ec.exam_id
				WHERE e.academic_year_start = ? AND e.term = ? AND e.exam_type = ? AND e.name = ?
				AND ec.cohort_id = ?`,
			Args: []any{planVersionID, now, input.AcademicYearStart, input.Term, input.ExamType, input.Name,
				item.cohort.ID},
		})
		for _, subject := range subjects {
			if !subject.Enabled {
				continue
			}
			fullMark := subject.FullMark
			if marks, ok := input.SubjectMarks[item.cohort.ID]; ok {
				if mark, ok := marks[subject.Key]; ok {
					if mark <= 0 {
						return Exam{}, fmt.Errorf("学科 %s 的满分必须大于零", subject.Key)
					}
					fullMark = mark
				}
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_subjects(exam_id, cohort_id, subject, full_mark, enabled)
					SELECT e.id, ?, ?, ?, 1 FROM exams e
					WHERE e.academic_year_start = ? AND e.term = ? AND e.exam_type = ? AND e.name = ?`,
				Args: []any{item.cohort.ID, subject.Key, fullMark,
					input.AcademicYearStart, input.Term, input.ExamType, input.Name},
			})
		}
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return Exam{}, err
	}
	rows, err = s.db.Query(ctx, `SELECT id FROM exams WHERE academic_year_start = ? AND term = ?
		AND exam_type = ? AND name = ?`, input.AcademicYearStart, input.Term, input.ExamType, input.Name)
	if err != nil {
		return Exam{}, err
	}
	if len(rows.Values) == 0 {
		return Exam{}, ErrNotFound
	}
	id, _ := rows.Values[0][0].(int64)
	return s.GetExam(ctx, id)
}

// ListExams 返回全部考试，最近的在前。
func (s *Store) ListExams(ctx context.Context) ([]Exam, error) {
	rows, err := s.db.Query(ctx, `SELECT id, academic_year_start, term, exam_type, name, exam_date,
		sequence, status, created_at FROM exams
		ORDER BY academic_year_start DESC, exam_date DESC, id DESC`)
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

// GetExam 读取一场考试及其参加的级。
func (s *Store) GetExam(ctx context.Context, id int64) (Exam, error) {
	rows, err := s.db.Query(ctx, `SELECT id, academic_year_start, term, exam_type, name, exam_date,
		sequence, status, created_at FROM exams WHERE id = ?`, id)
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
	exam.Cohorts, err = s.ListExamCohorts(ctx, id)
	if err != nil {
		return Exam{}, err
	}
	return exam, nil
}

// ListExamCohorts 返回一场考试中的全部级。
func (s *Store) ListExamCohorts(ctx context.Context, examID int64) ([]ExamCohort, error) {
	rows, err := s.db.Query(ctx, `SELECT ec.id, ec.exam_id, ec.cohort_id, ec.stage_id,
		ec.grade_level_at_exam, c.name, c.enrollment_year, c.graduation_year,
		(SELECT COUNT(*) FROM exam_rosters r WHERE r.exam_id = ec.exam_id AND r.cohort_id = ec.cohort_id),
		(SELECT COUNT(*) FROM exam_scores sc WHERE sc.exam_id = ec.exam_id AND sc.cohort_id = ec.cohort_id),
		(SELECT COUNT(*) FROM exam_subjects es WHERE es.exam_id = ec.exam_id AND es.cohort_id = ec.cohort_id AND es.enabled = 1),
		(SELECT COUNT(*) FROM assessments a JOIN assessment_class_targets t ON t.assessment_id = a.id
			WHERE a.exam_cohort_id = ec.id),
		(SELECT COUNT(*) FROM assessments a JOIN assessment_stat_groups g ON g.assessment_id = a.id
			WHERE a.exam_cohort_id = ec.id),
		cs.academic_year_start
		FROM exam_cohorts ec
		JOIN cohorts c ON c.id = ec.cohort_id
		JOIN cohort_stages cs ON cs.id = ec.stage_id
		WHERE ec.exam_id = ? ORDER BY c.enrollment_year DESC`, examID)
	if err != nil {
		return nil, err
	}
	cohorts := make([]ExamCohort, 0, len(rows.Values))
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		exam, _ := row[1].(int64)
		cohortID, _ := row[2].(int64)
		stageID, _ := row[3].(int64)
		grade, _ := row[4].(string)
		name, _ := row[5].(string)
		enrollment, _ := row[6].(int64)
		graduation, _ := row[7].(int64)
		roster, _ := row[8].(int64)
		scores, _ := row[9].(int64)
		subjects, _ := row[10].(int64)
		targets, _ := row[11].(int64)
		groups, _ := row[12].(int64)
		start, _ := row[13].(int64)
		cohorts = append(cohorts, ExamCohort{
			ID: id, ExamID: exam, CohortID: cohortID, StageID: stageID,
			GradeLevelAtExam: grade, CohortName: name, EnrollmentYear: int(enrollment),
			GraduationYear: int(graduation), RosterCount: int(roster), ScoreCount: int(scores),
			SubjectCount: int(subjects), ClassTargetCount: int(targets), StatGroupCount: int(groups),
			AcademicYearStart: int(start),
		})
	}
	return cohorts, nil
}

// GetExamCohort 读取一场考试中的一个级。
func (s *Store) GetExamCohort(ctx context.Context, examID, cohortID int64) (ExamCohort, error) {
	cohorts, err := s.ListExamCohorts(ctx, examID)
	if err != nil {
		return ExamCohort{}, err
	}
	for _, cohort := range cohorts {
		if cohort.CohortID == cohortID {
			return cohort, nil
		}
	}
	return ExamCohort{}, ErrExamCohort
}

// ListExamSubjects 返回一场考试中一个级的科目与满分。
func (s *Store) ListExamSubjects(ctx context.Context, examID, cohortID int64) ([]ExamSubject, error) {
	rows, err := s.db.Query(ctx, `SELECT exam_id, cohort_id, subject, full_mark, enabled
		FROM exam_subjects WHERE exam_id = ? AND cohort_id = ? ORDER BY subject`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	subjects := make([]ExamSubject, 0, len(rows.Values))
	for _, row := range rows.Values {
		exam, _ := row[0].(int64)
		cohort, _ := row[1].(int64)
		subject, _ := row[2].(string)
		mark, _ := row[3].(float64)
		enabled, _ := row[4].(int64)
		subjects = append(subjects, ExamSubject{
			ExamID: exam, CohortID: cohort, Subject: subject, FullMark: mark, Enabled: enabled != 0,
		})
	}
	return subjects, nil
}

// GenerateExamRoster 从当前阶段的组织数据生成报名名单。
func (s *Store) GenerateExamRoster(ctx context.Context, examID, cohortID int64, defaultTrack string) (ExamRosterSummary, error) {
	examCohort, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return ExamRosterSummary{}, err
	}
	if defaultTrack != "" {
		tracks, err := s.ListTracks(ctx)
		if err != nil {
			return ExamRosterSummary{}, err
		}
		valid := false
		for _, track := range tracks {
			if track.Key == defaultTrack && track.Enabled {
				valid = true
				break
			}
		}
		if !valid {
			return ExamRosterSummary{}, fmt.Errorf("选科类别 %q 不可用", defaultTrack)
		}
	}
	rows, err := s.db.Query(ctx, `SELECT m.student_id, st.student_no, st.name, m.class_id, m.track,
		m.elective, m.status, m.total_score
		FROM student_stage_memberships m JOIN students st ON st.id = m.student_id
		WHERE m.stage_id = ? ORDER BY st.student_no`, examCohort.StageID)
	if err != nil {
		return ExamRosterSummary{}, err
	}
	existingRows, err := s.db.Query(ctx, `SELECT student_id FROM exam_rosters WHERE exam_id = ? AND cohort_id = ?`,
		examID, cohortID)
	if err != nil {
		return ExamRosterSummary{}, err
	}
	existing := map[int64]bool{}
	for _, row := range existingRows.Values {
		id, _ := row[0].(int64)
		existing[id] = true
	}
	subjectsByStudent := map[int64][]string{}
	subjectRows, err := s.db.Query(ctx, `SELECT student_id, subject FROM student_teaching_groups
		WHERE stage_id = ? ORDER BY student_id, subject`, examCohort.StageID)
	if err != nil {
		return ExamRosterSummary{}, err
	}
	for _, row := range subjectRows.Values {
		studentID, _ := row[0].(int64)
		subject, _ := row[1].(string)
		subjectsByStudent[studentID] = append(subjectsByStudent[studentID], subject)
	}
	summary := ExamRosterSummary{Students: len(rows.Values)}
	var statements []sqlite.Statement
	for _, row := range rows.Values {
		studentID, _ := row[0].(int64)
		studentNo, _ := row[1].(string)
		name, _ := row[2].(string)
		classID, _ := row[3].(int64)
		track, _ := row[4].(string)
		status, _ := row[6].(string)
		if status == "" {
			status = "active"
		}
		if track == "" {
			track = defaultTrack
		}
		if track == "" {
			summary.MissingTrack++
		}
		if existing[studentID] {
			summary.Existing++
			continue
		}
		subjects := subjectsByStudent[studentID]
		subjectsJSON, err := json.Marshal(subjects)
		if err != nil {
			return ExamRosterSummary{}, err
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT OR IGNORE INTO exam_rosters(exam_id, cohort_id, student_id, class_id, track,
				total_score, status, subjects_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{examID, cohortID, studentID, classID, track, row[7], status, string(subjectsJSON)},
		})
		_ = studentNo
		_ = name
		summary.Inserted++
	}
	if summary.MissingTrack > 0 {
		return summary, fmt.Errorf("%w: %d 名学生", ErrExamTrackMissing, summary.MissingTrack)
	}
	if len(statements) > 0 {
		if _, err := s.db.Transaction(ctx, statements); err != nil {
			return ExamRosterSummary{}, err
		}
	}
	return summary, nil
}

// ListExamRoster 按考试、级和关键词查询报名名单。
func (s *Store) ListExamRoster(ctx context.Context, filter ExamRosterFilter) ([]ExamRosterEntry, int, error) {
	if filter.PageSize <= 0 {
		filter.PageSize = 200
	}
	if filter.Page < 0 {
		filter.Page = 0
	}
	where := []string{"r.exam_id = ?", "r.cohort_id = ?"}
	args := []any{filter.ExamID, filter.CohortID}
	if text := strings.TrimSpace(filter.Query); text != "" {
		where = append(where, "(st.student_no LIKE ? OR st.name LIKE ? OR r.exam_no LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}
	condition := strings.Join(where, " AND ")
	countRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM exam_rosters r
		JOIN students st ON st.id = r.student_id WHERE `+condition, args...)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	if len(countRows.Values) > 0 {
		value, _ := countRows.Values[0][0].(int64)
		total = int(value)
	}
	queryArgs := append(append([]any{}, args...), filter.PageSize, filter.Page*filter.PageSize)
	rows, err := s.db.Query(ctx, `SELECT r.id, r.student_id, st.student_no, st.name, r.class_id,
		c.name, r.exam_no, r.candidate_no, r.track, r.status, r.total_score, r.subjects_json,
		COALESCE(r.import_run_id, 0), r.source_sheet, r.source_row
		FROM exam_rosters r
		JOIN students st ON st.id = r.student_id
		JOIN classes c ON c.id = r.class_id
		WHERE `+condition+` ORDER BY st.student_no LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	entries := make([]ExamRosterEntry, 0, len(rows.Values))
	for _, row := range rows.Values {
		var subjects []string
		text, _ := row[11].(string)
		if text != "" {
			_ = json.Unmarshal([]byte(text), &subjects)
		}
		totalScore, err := nullableFloat64(row[10])
		if err != nil {
			return nil, 0, err
		}
		entry := ExamRosterEntry{}
		entry.ID, _ = row[0].(int64)
		entry.StudentID, _ = row[1].(int64)
		entry.StudentNo, _ = row[2].(string)
		entry.Name, _ = row[3].(string)
		entry.ClassID, _ = row[4].(int64)
		entry.ClassName, _ = row[5].(string)
		entry.ExamNo, _ = row[6].(string)
		entry.CandidateNo, _ = row[7].(string)
		entry.Track, _ = row[8].(string)
		entry.Status, _ = row[9].(string)
		entry.TotalScore = totalScore
		entry.Subjects = subjects
		entry.ImportRunID, _ = row[12].(int64)
		entry.SourceSheet, _ = row[13].(string)
		sourceRow, _ := row[14].(int64)
		entry.SourceRow = int(sourceRow)
		entries = append(entries, entry)
	}
	return entries, total, nil
}

// ListExamScores 按考试、级、学科、班级和关键词查询成绩。
func (s *Store) ListExamScores(ctx context.Context, filter ExamScoreFilter) ([]ExamScoreRow, int, error) {
	if filter.PageSize <= 0 {
		filter.PageSize = 200
	}
	if filter.Page < 0 {
		filter.Page = 0
	}
	where := []string{"sc.exam_id = ?", "sc.cohort_id = ?"}
	args := []any{filter.ExamID, filter.CohortID}
	if filter.Subject != "" {
		where = append(where, "sc.subject = ?")
		args = append(args, filter.Subject)
	}
	if filter.ClassID != 0 {
		where = append(where, "r.class_id = ?")
		args = append(args, filter.ClassID)
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		where = append(where, "(st.student_no LIKE ? OR st.name LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like)
	}
	condition := strings.Join(where, " AND ")
	countRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM exam_scores sc
		JOIN exam_rosters r ON r.exam_id = sc.exam_id AND r.cohort_id = sc.cohort_id AND r.student_id = sc.student_id
		JOIN students st ON st.id = sc.student_id WHERE `+condition, args...)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	if len(countRows.Values) > 0 {
		value, _ := countRows.Values[0][0].(int64)
		total = int(value)
	}
	queryArgs := append(append([]any{}, args...), filter.PageSize, filter.Page*filter.PageSize)
	rows, err := s.db.Query(ctx, `SELECT sc.student_id, st.student_no, st.name, r.class_id, c.name,
		sc.subject, sc.score, sc.flag, COALESCE(sc.import_run_id, 0), sc.source_sheet, sc.source_row
		FROM exam_scores sc
		JOIN exam_rosters r ON r.exam_id = sc.exam_id AND r.cohort_id = sc.cohort_id AND r.student_id = sc.student_id
		JOIN students st ON st.id = sc.student_id
		JOIN classes c ON c.id = r.class_id
		WHERE `+condition+` ORDER BY st.student_no, sc.subject LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ExamScoreRow, 0, len(rows.Values))
	for _, row := range rows.Values {
		score, err := nullableFloat64(row[6])
		if err != nil {
			return nil, 0, err
		}
		item := ExamScoreRow{}
		item.StudentID, _ = row[0].(int64)
		item.StudentNo, _ = row[1].(string)
		item.Name, _ = row[2].(string)
		item.ClassID, _ = row[3].(int64)
		item.ClassName, _ = row[4].(string)
		item.Subject, _ = row[5].(string)
		item.Score = score
		item.Flag, _ = row[7].(string)
		item.ImportRunID, _ = row[8].(int64)
		item.SourceSheet, _ = row[9].(string)
		sourceRow, _ := row[10].(int64)
		item.SourceRow = int(sourceRow)
		items = append(items, item)
	}
	return items, total, nil
}

// ExamScoreSummaries 返回一个级各科目的成绩数量与分布。
func (s *Store) ExamScoreSummaries(ctx context.Context, examID, cohortID int64) ([]ExamScoreSummary, error) {
	subjects, err := s.ListExamSubjects(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := make([]ExamScoreSummary, 0, len(subjects))
	for _, subject := range subjects {
		if !subject.Enabled {
			continue
		}
		rows, err := s.db.Query(ctx, `SELECT COUNT(sc.score), COALESCE(AVG(sc.score), 0),
			COALESCE(MIN(sc.score), 0), COALESCE(MAX(sc.score), 0)
			FROM exam_scores sc WHERE sc.exam_id = ? AND sc.cohort_id = ? AND sc.subject = ? AND sc.score IS NOT NULL`,
			examID, cohortID, subject.Subject)
		if err != nil {
			return nil, err
		}
		item := ExamScoreSummary{Subject: subject.Subject}
		if len(rows.Values) > 0 {
			count, _ := rows.Values[0][0].(int64)
			item.Count = int(count)
			item.Average, _ = rows.Values[0][1].(float64)
			item.Minimum, _ = rows.Values[0][2].(float64)
			item.Maximum, _ = rows.Values[0][3].(float64)
		}
		rosterRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM exam_rosters WHERE exam_id = ? AND cohort_id = ?`,
			examID, cohortID)
		if err != nil {
			return nil, err
		}
		if len(rosterRows.Values) > 0 {
			total, _ := rosterRows.Values[0][0].(int64)
			item.Missing = int(total) - item.Count
		}
		result = append(result, item)
	}
	return result, nil
}

// ExamScoreBands 返回每个学科按满分十等分统计的成绩分布。
func (s *Store) ExamScoreBands(ctx context.Context, examID, cohortID int64) ([]ExamScoreBand, error) {
	subjects, err := s.ListExamSubjects(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := make([]ExamScoreBand, 0, len(subjects)*10)
	for _, subject := range subjects {
		if !subject.Enabled || subject.FullMark <= 0 {
			continue
		}
		step := subject.FullMark / 10
		counts := make([]int, 10)
		rows, err := s.db.Query(ctx, `SELECT sc.score FROM exam_scores sc
			WHERE sc.exam_id = ? AND sc.cohort_id = ? AND sc.subject = ? AND sc.score IS NOT NULL`,
			examID, cohortID, subject.Subject)
		if err != nil {
			return nil, err
		}
		for _, row := range rows.Values {
			score, _ := row[0].(float64)
			index := int(score / step)
			if index < 0 {
				index = 0
			}
			if index > 9 {
				index = 9
			}
			counts[index]++
		}
		for index, count := range counts {
			result = append(result, ExamScoreBand{
				Subject: subject.Subject,
				Lower:   step * float64(index),
				Upper:   step * float64(index+1),
				Count:   count,
			})
		}
	}
	return result, nil
}

// ExamClassAverages 返回每个学科每个行政班的均分与人数。
func (s *Store) ExamClassAverages(ctx context.Context, examID, cohortID int64) ([]ExamClassAverage, error) {
	rows, err := s.db.Query(ctx, `SELECT sc.subject, r.class_id, c.name, COUNT(sc.score), AVG(sc.score)
		FROM exam_scores sc
		JOIN exam_rosters r ON r.exam_id = sc.exam_id AND r.cohort_id = sc.cohort_id
			AND r.student_id = sc.student_id
		JOIN classes c ON c.id = r.class_id
		WHERE sc.exam_id = ? AND sc.cohort_id = ? AND sc.score IS NOT NULL
		GROUP BY sc.subject, r.class_id, c.name ORDER BY sc.subject, c.name`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := make([]ExamClassAverage, 0, len(rows.Values))
	for _, row := range rows.Values {
		item := ExamClassAverage{}
		item.Subject, _ = row[0].(string)
		item.ClassID, _ = row[1].(int64)
		item.ClassName, _ = row[2].(string)
		count, _ := row[3].(int64)
		item.Count = int(count)
		item.Average, _ = row[4].(float64)
		result = append(result, item)
	}
	return result, nil
}

// ExamCutoffImpacts 返回每条单科分数线达到人数与同类考生中的占比。
func (s *Store) ExamCutoffImpacts(ctx context.Context, examID, cohortID int64) ([]ExamCutoffImpact, error) {
	cutoffs, err := s.ListExamSubjectCutoffs(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := make([]ExamCutoffImpact, 0, len(cutoffs))
	for _, cutoff := range cutoffs {
		rows, err := s.db.Query(ctx, `SELECT
			(SELECT COUNT(*) FROM exam_rosters r WHERE r.exam_id = ? AND r.cohort_id = ? AND r.track = ?),
			(SELECT COUNT(*) FROM exam_scores sc
				JOIN exam_rosters r ON r.exam_id = sc.exam_id AND r.cohort_id = sc.cohort_id
					AND r.student_id = sc.student_id
				WHERE sc.exam_id = ? AND sc.cohort_id = ? AND sc.subject = ? AND r.track = ?
					AND sc.score IS NOT NULL AND sc.score >= ?)`,
			examID, cohortID, cutoff.Track,
			examID, cohortID, cutoff.Subject, cutoff.Track, cutoff.Cutoff)
		if err != nil {
			return nil, err
		}
		item := ExamCutoffImpact{
			Subject: cutoff.Subject, Track: cutoff.Track, Cutoff: cutoff.Cutoff,
		}
		if len(rows.Values) > 0 {
			rostered, _ := rows.Values[0][0].(int64)
			reached, _ := rows.Values[0][1].(int64)
			item.Rostered = int(rostered)
			item.Reached = int(reached)
			if item.Rostered > 0 {
				item.ReachedRatio = float64(item.Reached) / float64(item.Rostered)
			}
		}
		result = append(result, item)
	}
	return result, nil
}

// ListExamQualitySummaries 返回每场考试每个级的数据质量问题。
func (s *Store) ListExamQualitySummaries(ctx context.Context) ([]ExamQualitySummary, error) {
	exams, err := s.ListExams(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ExamQualitySummary, 0, len(exams))
	for _, exam := range exams {
		for _, cohort := range exam.Cohorts {
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

// ListExamSubjectCutoffs 返回单科本科线。
func (s *Store) ListExamSubjectCutoffs(ctx context.Context, examID, cohortID int64) ([]ExamSubjectCutoff, error) {
	rows, err := s.db.Query(ctx, `SELECT id, exam_id, cohort_id, subject, track, cutoff
		FROM exam_subject_cutoffs WHERE exam_id = ? AND cohort_id = ? ORDER BY subject, track`,
		examID, cohortID)
	if err != nil {
		return nil, err
	}
	items := make([]ExamSubjectCutoff, 0, len(rows.Values))
	for _, row := range rows.Values {
		item := ExamSubjectCutoff{}
		item.ID, _ = row[0].(int64)
		item.ExamID, _ = row[1].(int64)
		item.CohortID, _ = row[2].(int64)
		item.Subject, _ = row[3].(string)
		item.Track, _ = row[4].(string)
		item.Cutoff, _ = row[5].(float64)
		items = append(items, item)
	}
	return items, nil
}

// ListExamTierCutoffs 返回总分档位线。
func (s *Store) ListExamTierCutoffs(ctx context.Context, examID, cohortID int64) ([]ExamTierCutoff, error) {
	rows, err := s.db.Query(ctx, `SELECT id, exam_id, cohort_id, tier_key, track, cutoff
		FROM exam_tier_cutoffs WHERE exam_id = ? AND cohort_id = ? ORDER BY tier_key, track`,
		examID, cohortID)
	if err != nil {
		return nil, err
	}
	items := make([]ExamTierCutoff, 0, len(rows.Values))
	for _, row := range rows.Values {
		item := ExamTierCutoff{}
		item.ID, _ = row[0].(int64)
		item.ExamID, _ = row[1].(int64)
		item.CohortID, _ = row[2].(int64)
		item.TierKey, _ = row[3].(string)
		item.Track, _ = row[4].(string)
		item.Cutoff, _ = row[5].(float64)
		items = append(items, item)
	}
	return items, nil
}

// AssessmentForExamCohort 读取或者建立一场考试中一个级的本次考核。
func (s *Store) AssessmentForExamCohort(ctx context.Context, examCohortID int64) (Assessment, error) {
	rows, err := s.db.Query(ctx, `SELECT id, exam_cohort_id, COALESCE(plan_version_id, 0),
		status, COALESCE(current_run_id, 0), revision FROM assessments WHERE exam_cohort_id = ?`,
		examCohortID)
	if err != nil {
		return Assessment{}, err
	}
	if len(rows.Values) == 0 {
		if _, err := s.db.Execute(ctx, `INSERT INTO assessments(exam_cohort_id, status, revision, created_at)
			VALUES (?, 'draft', 1, ?)`, examCohortID, s.now().Format(time.RFC3339Nano)); err != nil {
			return Assessment{}, err
		}
		return s.AssessmentForExamCohort(ctx, examCohortID)
	}
	return assessmentFromRow(rows.Values[0])
}

// ListClassTargets 返回本次考核的班级分档目标。
func (s *Store) ListClassTargets(ctx context.Context, assessmentID int64) ([]AssessmentClassTarget, error) {
	rows, err := s.db.Query(ctx, `SELECT t.class_id, c.name, t.tier_key, t.target_count
		FROM assessment_class_targets t JOIN classes c ON c.id = t.class_id
		WHERE t.assessment_id = ? ORDER BY c.name, t.tier_key`, assessmentID)
	if err != nil {
		return nil, err
	}
	items := make([]AssessmentClassTarget, 0, len(rows.Values))
	for _, row := range rows.Values {
		item := AssessmentClassTarget{HasTarget: true}
		item.ClassID, _ = row[0].(int64)
		item.ClassName, _ = row[1].(string)
		item.TierKey, _ = row[2].(string)
		item.Target, _ = row[3].(float64)
		items = append(items, item)
	}
	return items, nil
}

// SetClassTarget 写入一个班级一个档位的目标人数。
func (s *Store) SetClassTarget(ctx context.Context, assessmentID, classID int64, tierKey string, target float64) error {
	if target < 0 {
		return fmt.Errorf("目标人数不能小于零")
	}
	if strings.TrimSpace(tierKey) == "" {
		return fmt.Errorf("档位不能为空")
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
	if len(rows.Values) == 0 {
		return fmt.Errorf("班级不属于本次考核对应的阶段")
	}
	count, _ := rows.Values[0][0].(int64)
	if count == 0 {
		return fmt.Errorf("班级不属于本次考核对应的阶段")
	}
	_, err = s.db.Execute(ctx, `INSERT INTO assessment_class_targets(assessment_id, class_id, tier_key, target_count)
		VALUES (?, ?, ?, ?) ON CONFLICT(assessment_id, class_id, tier_key)
		DO UPDATE SET target_count = excluded.target_count`, assessmentID, classID, tierKey, target)
	return err
}

// ListExamStatGroups 返回本次考核的合并组。
func (s *Store) ListExamStatGroups(ctx context.Context, assessmentID int64) ([]ExamStatGroup, error) {
	rows, err := s.db.Query(ctx, `SELECT g.id, g.assessment_id, g.subject, g.name, g.lead_class_id,
		c.name, t.target_count, g.baseline_average, g.baseline_ratio, g.baseline_source, g.explicit
		FROM assessment_stat_groups g
		JOIN classes c ON c.id = g.lead_class_id
		LEFT JOIN assessment_stat_group_targets t ON t.stat_group_id = g.id
		WHERE g.assessment_id = ? ORDER BY g.subject, g.name`, assessmentID)
	if err != nil {
		return nil, err
	}
	groups := make([]ExamStatGroup, 0, len(rows.Values))
	for _, row := range rows.Values {
		group := ExamStatGroup{}
		group.ID, _ = row[0].(int64)
		group.AssessmentID, _ = row[1].(int64)
		group.Subject, _ = row[2].(string)
		group.Name, _ = row[3].(string)
		group.LeadClassID, _ = row[4].(int64)
		group.LeadClassName, _ = row[5].(string)
		group.Target, _ = nullableFloat64(row[6])
		group.BaselineAverage, _ = row[7].(float64)
		group.BaselineRatio, _ = row[8].(float64)
		group.BaselineSource, _ = row[9].(string)
		explicit, _ := row[10].(int64)
		group.Explicit = explicit != 0
		groups = append(groups, group)
	}
	for index := range groups {
		members, err := s.statGroupMembers(ctx, groups[index].ID)
		if err != nil {
			return nil, err
		}
		groups[index].Members = members
	}
	return groups, nil
}

// SaveStatGroup 新建或者修改一个合并组。
func (s *Store) SaveStatGroup(ctx context.Context, input StatGroupInput) (int64, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return 0, fmt.Errorf("合并组名称不能为空")
	}
	if len(input.MemberGroupIDs) == 0 {
		return 0, fmt.Errorf("合并组成员不能为空")
	}
	if input.Target != nil && *input.Target < 0 {
		return 0, fmt.Errorf("合并组目标不能小于零")
	}
	stageID, err := s.assessmentStage(ctx, input.AssessmentID)
	if err != nil {
		return 0, err
	}
	if input.ID != 0 {
		rows, err := s.db.Query(ctx, `SELECT id FROM assessment_stat_groups WHERE id = ? AND assessment_id = ?`,
			input.ID, input.AssessmentID)
		if err != nil {
			return 0, err
		}
		if len(rows.Values) == 0 {
			return 0, ErrNotFound
		}
	}
	subjects, err := s.subjectFullMarks(ctx)
	if err != nil {
		return 0, err
	}
	if _, ok := subjects[input.Subject]; !ok {
		return 0, fmt.Errorf("学科 %q 不存在", input.Subject)
	}
	leadRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM class_stage_info WHERE stage_id = ? AND class_id = ?`,
		stageID, input.LeadClassID)
	if err != nil {
		return 0, err
	}
	if len(leadRows.Values) == 0 {
		return 0, fmt.Errorf("承担统计职责的班级不属于当前阶段")
	}
	lead, _ := leadRows.Values[0][0].(int64)
	if lead == 0 {
		return 0, fmt.Errorf("承担统计职责的班级不属于当前阶段")
	}
	seen := map[int64]bool{}
	ids := make([]any, 0, len(input.MemberGroupIDs))
	for _, id := range input.MemberGroupIDs {
		if seen[id] {
			return 0, fmt.Errorf("合并组成员重复")
		}
		seen[id] = true
		rows, err := s.db.Query(ctx, `SELECT subject FROM teaching_groups WHERE id = ? AND stage_id = ?`,
			id, stageID)
		if err != nil {
			return 0, err
		}
		if len(rows.Values) == 0 {
			return 0, fmt.Errorf("教学班 %d 不属于当前阶段", id)
		}
		subject, _ := rows.Values[0][0].(string)
		if subject != input.Subject {
			return 0, fmt.Errorf("教学班学科与合并组学科不一致")
		}
		ids = append(ids, id)
	}
	rows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM student_teaching_groups
		WHERE stage_id = ? AND teaching_group_id IN (`+placeholders(len(ids))+`)`, append([]any{stageID}, ids...)...)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) > 0 {
		count, _ := rows.Values[0][0].(int64)
		if count == 0 {
			return 0, fmt.Errorf("合并组没有学生加入")
		}
	}
	var statements []sqlite.Statement
	if input.ID == 0 {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assessment_stat_groups(assessment_id, subject, name, lead_class_id,
				baseline_average, baseline_ratio, baseline_source, explicit)
				VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
			Args: []any{input.AssessmentID, input.Subject, input.Name, input.LeadClassID,
				input.BaselineAverage, input.BaselineRatio, input.BaselineSource},
		})
	} else {
		statements = append(statements, sqlite.Statement{
			SQL: `UPDATE assessment_stat_groups SET subject = ?, name = ?, lead_class_id = ?,
				baseline_average = ?, baseline_ratio = ?, baseline_source = ?, explicit = 1
				WHERE id = ? AND assessment_id = ?`,
			Args: []any{input.Subject, input.Name, input.LeadClassID, input.BaselineAverage,
				input.BaselineRatio, input.BaselineSource, input.ID, input.AssessmentID},
		})
		statements = append(statements, sqlite.Statement{
			SQL: `DELETE FROM assessment_stat_group_members WHERE stat_group_id = ?`, Args: []any{input.ID},
		})
	}
	for _, id := range input.MemberGroupIDs {
		var args []any
		if input.ID == 0 {
			args = []any{id}
		} else {
			args = []any{input.ID, id}
		}
		sql := `INSERT INTO assessment_stat_group_members(stat_group_id, teaching_group_id)
			SELECT id, ? FROM assessment_stat_groups WHERE assessment_id = ? AND subject = ? AND name = ?`
		if input.ID != 0 {
			sql = `INSERT INTO assessment_stat_group_members(stat_group_id, teaching_group_id)
				SELECT id, ? FROM assessment_stat_groups WHERE id = ?`
		}
		if input.ID == 0 {
			args = []any{id, input.AssessmentID, input.Subject, input.Name}
		}
		statements = append(statements, sqlite.Statement{SQL: sql, Args: args})
	}
	targetValue := 0.0
	if input.Target != nil {
		targetValue = *input.Target
	}
	targetSQL := `INSERT INTO assessment_stat_group_targets(stat_group_id, target_count)
		SELECT id, ? FROM assessment_stat_groups WHERE assessment_id = ? AND subject = ? AND name = ?
		ON CONFLICT(stat_group_id) DO UPDATE SET target_count = excluded.target_count`
	targetArgs := []any{targetValue, input.AssessmentID, input.Subject, input.Name}
	if input.ID == 0 {
		statements = append(statements, sqlite.Statement{SQL: targetSQL, Args: targetArgs})
	} else {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assessment_stat_group_targets(stat_group_id, target_count) VALUES (?, ?)
				ON CONFLICT(stat_group_id) DO UPDATE SET target_count = excluded.target_count`,
			Args: []any{input.ID, targetValue},
		})
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return 0, err
	}
	if input.ID != 0 {
		return input.ID, nil
	}
	rows, err = s.db.Query(ctx, `SELECT id FROM assessment_stat_groups WHERE assessment_id = ?
		AND subject = ? AND name = ?`, input.AssessmentID, input.Subject, input.Name)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, ErrNotFound
	}
	id, _ := rows.Values[0][0].(int64)
	return id, nil
}

// DeleteStatGroup 删除本次考核内的一个合并组。
func (s *Store) DeleteStatGroup(ctx context.Context, assessmentID, id int64) error {
	if _, err := s.assessmentStage(ctx, assessmentID); err != nil {
		return err
	}
	result, err := s.db.Execute(ctx, `DELETE FROM assessment_stat_groups WHERE id = ? AND assessment_id = ?`,
		id, assessmentID)
	if err != nil {
		return err
	}
	if result.Changes == 0 {
		return ErrNotFound
	}
	return nil
}

// ExamPrep 返回一份考试准备清单的数量和问题。
func (s *Store) ExamPrep(ctx context.Context, examID, cohortID int64) (ExamPrepStatus, []ExamQualityIssue, error) {
	cohort, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return ExamPrepStatus{}, nil, err
	}
	assessment, err := s.AssessmentForExamCohort(ctx, cohort.ID)
	if err != nil {
		return ExamPrepStatus{}, nil, err
	}
	cutoffs, err := s.ListExamSubjectCutoffs(ctx, examID, cohortID)
	if err != nil {
		return ExamPrepStatus{}, nil, err
	}
	targets, err := s.ListClassTargets(ctx, assessment.ID)
	if err != nil {
		return ExamPrepStatus{}, nil, err
	}
	groups, err := s.ListExamStatGroups(ctx, assessment.ID)
	if err != nil {
		return ExamPrepStatus{}, nil, err
	}
	status := ExamPrepStatus{
		Roster: cohort.RosterCount, Scores: cohort.ScoreCount, Cutoffs: len(cutoffs),
		Targets: len(targets), StatGroups: len(groups),
	}
	issues, err := s.ExamQualityIssues(ctx, examID, cohortID)
	if err != nil {
		return ExamPrepStatus{}, nil, err
	}
	for _, issue := range issues {
		if issue.Severity == "blocking" {
			status.Blocking++
		} else {
			status.Warnings++
		}
	}
	return status, issues, nil
}

// ExamQualityIssues 检查一场考试中一个级的成绩、分数线和目标。
func (s *Store) ExamQualityIssues(ctx context.Context, examID, cohortID int64) ([]ExamQualityIssue, error) {
	cohort, err := s.GetExamCohort(ctx, examID, cohortID)
	if err != nil {
		return nil, err
	}
	assessment, err := s.AssessmentForExamCohort(ctx, cohort.ID)
	if err != nil {
		return nil, err
	}
	var issues []ExamQualityIssue
	if cohort.RosterCount == 0 {
		issues = append(issues, ExamQualityIssue{Severity: "warning", Code: "no_roster"})
	}
	rows, err := s.db.Query(ctx, `SELECT st.student_no, st.name, r.track,
		COALESCE(r.import_run_id, 0), r.source_row,
		(SELECT COUNT(*) FROM exam_scores sc WHERE sc.exam_id = r.exam_id AND sc.cohort_id = r.cohort_id
			AND sc.student_id = r.student_id) AS score_count
		FROM exam_rosters r JOIN students st ON st.id = r.student_id
		WHERE r.exam_id = ? AND r.cohort_id = ? ORDER BY st.student_no`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	missing := 0
	for _, row := range rows.Values {
		studentNo, _ := row[0].(string)
		name, _ := row[1].(string)
		track, _ := row[2].(string)
		runID, _ := row[3].(int64)
		sourceRow, _ := row[4].(int64)
		count, _ := row[5].(int64)
		if track == "" {
			issues = append(issues, ExamQualityIssue{
				Severity: "warning", Code: "missing_track", StudentNo: studentNo, Name: name,
				ImportRunID: runID, Row: int(sourceRow),
			})
		}
		if count == 0 && missing < 200 {
			missing++
			issues = append(issues, ExamQualityIssue{
				Severity: "warning", Code: "missing_scores", StudentNo: studentNo, Name: name,
				ImportRunID: runID, Row: int(sourceRow),
			})
		}
	}
	scoreRows, err := s.db.Query(ctx, `SELECT st.student_no, st.name, sc.subject, sc.score, es.full_mark,
		COALESCE(sc.import_run_id, 0), sc.source_row
		FROM exam_scores sc
		JOIN students st ON st.id = sc.student_id
		JOIN exam_subjects es ON es.exam_id = sc.exam_id AND es.cohort_id = sc.cohort_id AND es.subject = sc.subject
		WHERE sc.exam_id = ? AND sc.cohort_id = ? AND sc.score IS NOT NULL AND sc.score > es.full_mark
		LIMIT 200`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	for _, row := range scoreRows.Values {
		studentNo, _ := row[0].(string)
		name, _ := row[1].(string)
		subject, _ := row[2].(string)
		score, _ := row[3].(float64)
		full, _ := row[4].(float64)
		runID, _ := row[5].(int64)
		sourceRow, _ := row[6].(int64)
		issues = append(issues, ExamQualityIssue{
			Severity: "blocking", Code: "score_out_of_range", StudentNo: studentNo,
			Name: name, Subject: subject, Args: []string{formatNumber(score), formatNumber(full)},
			ImportRunID: runID, Row: int(sourceRow),
		})
	}
	orphanRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM exam_scores sc
		LEFT JOIN exam_rosters r ON r.exam_id = sc.exam_id AND r.cohort_id = sc.cohort_id AND r.student_id = sc.student_id
		WHERE sc.exam_id = ? AND sc.cohort_id = ? AND r.id IS NULL`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	if len(orphanRows.Values) > 0 {
		count, _ := orphanRows.Values[0][0].(int64)
		if count > 0 {
			issues = append(issues, ExamQualityIssue{
				Severity: "blocking", Code: "score_without_roster", Args: []string{fmt.Sprintf("%d", count)},
			})
		}
	}
	tierKeys, err := s.assessmentTierKeys(ctx, assessment.ID)
	if err != nil {
		return nil, err
	}
	targets, err := s.ListClassTargets(ctx, assessment.ID)
	if err != nil {
		return nil, err
	}
	targetMap := map[string]float64{}
	for _, target := range targets {
		targetMap[fmt.Sprintf("%d\x00%s", target.ClassID, target.TierKey)] = target.Target
	}
	classRows, err := s.db.Query(ctx, `SELECT c.id, c.name FROM classes c
		JOIN class_stage_info i ON i.class_id = c.id WHERE i.stage_id = ? ORDER BY c.name`, cohort.StageID)
	if err != nil {
		return nil, err
	}
	for _, row := range classRows.Values {
		classID, _ := row[0].(int64)
		className, _ := row[1].(string)
		for _, tier := range tierKeys {
			if targetMap[fmt.Sprintf("%d\x00%s", classID, tier)] == 0 {
				issues = append(issues, ExamQualityIssue{
					Severity: "warning", Code: "zero_class_target", ClassName: className, TierKey: tier,
				})
			}
		}
	}
	groups, err := s.ListExamStatGroups(ctx, assessment.ID)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if len(group.Members) == 0 {
			issues = append(issues, ExamQualityIssue{
				Severity: "blocking", Code: "empty_stat_group", Subject: group.Subject,
				Args: []string{group.Name},
			})
		}
		if group.Target == nil || *group.Target == 0 {
			issues = append(issues, ExamQualityIssue{
				Severity: "warning", Code: "zero_stat_group_target", Subject: group.Subject,
				Args: []string{group.Name},
			})
		}
	}
	cutoffRows, err := s.db.Query(ctx, `SELECT es.subject FROM exam_subjects es
		WHERE es.exam_id = ? AND es.cohort_id = ? AND es.enabled = 1
		AND NOT EXISTS (SELECT 1 FROM exam_subject_cutoffs c
			WHERE c.exam_id = es.exam_id AND c.cohort_id = es.cohort_id AND c.subject = es.subject)`,
		examID, cohortID)
	if err != nil {
		return nil, err
	}
	for _, row := range cutoffRows.Values {
		subject, _ := row[0].(string)
		issues = append(issues, ExamQualityIssue{
			Severity: "warning", Code: "missing_cutoff", Subject: subject,
		})
	}
	return issues, nil
}

func (s *Store) assessmentStage(ctx context.Context, assessmentID int64) (int64, error) {
	rows, err := s.db.Query(ctx, `SELECT ec.stage_id FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id WHERE a.id = ?`, assessmentID)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, ErrNotFound
	}
	stageID, _ := rows.Values[0][0].(int64)
	return stageID, nil
}

func (s *Store) defaultPlanVersionID(ctx context.Context) (int64, error) {
	rows, err := s.db.Query(ctx, `SELECT v.id FROM plans p JOIN plan_versions v ON v.plan_id = p.id
		WHERE p.kind = 'system' AND v.status = 'published' ORDER BY v.version DESC LIMIT 1`)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, fmt.Errorf("没有可用的内置方案版本")
	}
	id, _ := rows.Values[0][0].(int64)
	return id, nil
}

func (s *Store) statGroupMembers(ctx context.Context, groupID int64) ([]StatGroupMember, error) {
	rows, err := s.db.Query(ctx, `SELECT g.id, g.subject, g.name,
		COALESCE((SELECT GROUP_CONCAT(c.name, '、') FROM student_teaching_groups stg
			JOIN student_stage_memberships m ON m.student_id = stg.student_id AND m.stage_id = stg.stage_id
			JOIN classes c ON c.id = m.class_id
			WHERE stg.teaching_group_id = g.id), ''),
		COALESCE((SELECT COUNT(*) FROM student_teaching_groups stg WHERE stg.teaching_group_id = g.id), 0)
		FROM assessment_stat_group_members m JOIN teaching_groups g ON g.id = m.teaching_group_id
		WHERE m.stat_group_id = ? ORDER BY g.subject, g.name`, groupID)
	if err != nil {
		return nil, err
	}
	members := make([]StatGroupMember, 0, len(rows.Values))
	for _, row := range rows.Values {
		member := StatGroupMember{}
		member.TeachingGroupID, _ = row[0].(int64)
		member.Subject, _ = row[1].(string)
		member.Name, _ = row[2].(string)
		member.ClassNames, _ = row[3].(string)
		students, _ := row[4].(int64)
		member.Students = int(students)
		members = append(members, member)
	}
	return members, nil
}

func (s *Store) assessmentTierKeys(ctx context.Context, assessmentID int64) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT COALESCE(plan_version_id, 0) FROM assessments WHERE id = ?`, assessmentID)
	if err != nil {
		return nil, err
	}
	if len(rows.Values) == 0 {
		return nil, ErrNotFound
	}
	versionID, _ := rows.Values[0][0].(int64)
	if versionID == 0 {
		published, err := s.db.Query(ctx, `SELECT id FROM plan_versions WHERE status = 'published'
			ORDER BY id DESC LIMIT 1`)
		if err != nil {
			return nil, err
		}
		if len(published.Values) > 0 {
			versionID, _ = published.Values[0][0].(int64)
		}
	}
	config := assessment.DefaultConfig()
	if versionID != 0 {
		version, err := s.GetVersion(ctx, versionID)
		if err != nil {
			return nil, err
		}
		config = version.Config
	}
	keys := make([]string, 0, len(config.Tiers))
	for _, tier := range config.Tiers {
		keys = append(keys, tier.Key)
	}
	return keys, nil
}

// AssessmentTierKeys 返回本次考核使用的方案档位。
func (s *Store) AssessmentTierKeys(ctx context.Context, assessmentID int64) ([]string, error) {
	return s.assessmentTierKeys(ctx, assessmentID)
}

func examFromRow(row []any) (Exam, error) {
	id, ok := row[0].(int64)
	if !ok {
		return Exam{}, fmt.Errorf("考试标识格式不正确")
	}
	start, _ := row[1].(int64)
	term, _ := row[2].(string)
	examType, _ := row[3].(string)
	name, _ := row[4].(string)
	date, _ := row[5].(string)
	sequence, _ := row[6].(int64)
	status, _ := row[7].(string)
	createdAt, err := parseTime(row[8])
	if err != nil {
		return Exam{}, err
	}
	return Exam{
		ID: id, AcademicYearStart: int(start), Term: term, ExamType: examType,
		Name: name, ExamDate: date, Sequence: int(sequence), Status: status, CreatedAt: createdAt,
	}, nil
}

func assessmentFromRow(row []any) (Assessment, error) {
	id, ok := row[0].(int64)
	if !ok {
		return Assessment{}, fmt.Errorf("本次考核标识格式不正确")
	}
	examCohortID, _ := row[1].(int64)
	planVersionID, _ := row[2].(int64)
	status, _ := row[3].(string)
	currentRunID, _ := row[4].(int64)
	revision, _ := row[5].(int64)
	return Assessment{
		ID: id, ExamCohortID: examCohortID, PlanVersion: planVersionID,
		Status: status, CurrentRunID: currentRunID, Revision: int(revision),
	}, nil
}

func nullableFloat64(value any) (*float64, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case float64:
		copied := typed
		return &copied, nil
	case int64:
		copied := float64(typed)
		return &copied, nil
	}
	return nil, fmt.Errorf("数值格式不正确")
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
