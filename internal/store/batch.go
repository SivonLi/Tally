package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/assessment"
	"tally/internal/calc"
)

// Batch 是一次考核的组织单位。
type Batch struct {
	ID                   int64
	TermID               int64
	AcademicYear         string
	Semester             string
	ExamType             string
	Grade                string
	Status               string
	CurrentPlanVersionID int64
	Revision             int
	CreatedAt            time.Time
}

// CreateBatchInput 是新建批次需要的字段。
type CreateBatchInput struct {
	AcademicYear string
	Semester     string
	ExamType     string
	Grade        string
}

// ImportStudent 是导入时的一名学生。
type ImportStudent struct {
	StudentNo   string
	Name        string
	ClassName   string
	Track       string
	Elective    string
	Status      string
	Japanese    bool
	Walking     bool
	TotalScore  *float64
	Scores      map[string]float64
	Flags       map[string]string
	SourceSheet string
	SourceRow   int
}

// ImportAssignment 是导入时的一条任课关系。
type ImportAssignment struct {
	ClassName   string
	Subject     string
	TeacherName string
	TeacherNo   string
	SourceSheet string
	SourceRow   int
}

// ImportTeacher 是导入时的一名教师。
type ImportTeacher struct {
	TeacherNo   string
	Name        string
	Subject     string
	SourceSheet string
	SourceRow   int
}

// ImportClassTarget 是一个行政班的分档目标人数。
type ImportClassTarget struct {
	ClassName string
	Counts    map[string]*float64
}

// ImportClassBaseline 是一个行政班某个学科的基数均分。
type ImportClassBaseline struct {
	ClassName   string
	Subject     string
	Average     float64
	Ratio       *float64
	SourceSheet string
	SourceRow   int
}

// ImportSubjectCutoff 是一个学科在某一类考生的分数线。
type ImportSubjectCutoff struct {
	Subject string
	Track   string
	Value   float64
}

// ImportTierCutoff 是一个总分档位在某一类考生中的分数线。
type ImportTierCutoff struct {
	TierKey string
	Track   string
	Value   float64
}

// ImportStatGroup 是显式配置的统计组，用于走班合并。
type ImportStatGroup struct {
	Name            string
	Subject         string
	LeadClassName   string
	MemberClasses   []string
	Target          float64
	BaselineAverage float64
}

// ImportSheet 记录一个被识别的工作表。
type ImportSheet struct {
	Name      string
	Role      string
	HeaderRow int
	Rows      int
}

// ImportReport 记录一次导入的来源与识别结果。
type ImportReport struct {
	FileHash      string
	Path          string
	StoredPath    string
	ConvertedPath string
	Format        string
	Conversion    string
	RowCount      int
	Sheets        []ImportSheet
	Warnings      []string
}

// ImportData 是一次导入解析出的全部数据，可以分多次导入同一批次。
type ImportData struct {
	Classes        []string
	Students       []ImportStudent
	Roster         []ImportRoster
	Teachers       []string
	TeacherList    []ImportTeacher
	Assignments    []ImportAssignment
	ClassTargets   []ImportClassTarget
	ClassBaselines []ImportClassBaseline
	SubjectCutoffs []ImportSubjectCutoff
	TierCutoffs    []ImportTierCutoff
	StatGroups     []ImportStatGroup
	GradeBaselines map[string]float64
}

// ImportIssue 是一条导入校验问题。Code 与 Args 用于界面按语言生成文案。
type ImportIssue struct {
	Severity string
	Row      int
	Column   string
	Code     string
	Args     []string
}

// StageImportRequest 是一次阶段数据导入请求。
type StageImportRequest struct {
	Kind    string
	Data    ImportData
	Report  ImportReport
	Mapping map[string]string
	Issues  []ImportIssue
}

// ImportPreview 是写入前的校验结果。
type ImportPreview struct {
	Kind     string
	New      int
	Update   int
	Skip     int
	Issues   []ImportIssue
	Blocking int
	Warnings int
}

// ImportRunDetail 是导入记录详情。
type ImportRunDetail struct {
	Run    ImportRun
	Issues []ImportIssue
}

// ImportSummary 是导入完成后可以显示的数量摘要。
type ImportSummary struct {
	Classes        int
	Students       int
	Teachers       int
	TeachingGroups int
	StatGroups     int
	Assignments    int
	ClassTargets   int
	SubjectCutoffs int
	TierCutoffs    int
	Baselines      int
	Inserted       int
	Updated        int
	Skipped        int
	Warnings       []string
}

// Class 是一个行政班。
type Class struct {
	ID   int64
	Name string
}

// Student 是一名学生及其在本批次的成绩与归属。
type Student struct {
	ID             int64
	StudentNo      string
	Name           string
	ClassID        int64
	Track          string
	Elective       string
	Japanese       bool
	Walking        bool
	Status         string
	TotalScore     *float64
	Scores         map[string]float64
	ScoreFlags     map[string]string
	TeachingGroups map[string]int64
}

// TeachingGroup 是一个学科教学班。
type TeachingGroup struct {
	ID      int64
	Subject string
	Name    string
	ClassID int64
}

// StatGroup 是单科达线率的计算单位。
type StatGroup struct {
	ID              int64
	Subject         string
	Name            string
	LeadClassID     int64
	MemberGroupIDs  []int64
	Target          float64
	BaselineAverage float64
	BaselineRatio   float64
}

// Assignment 是教师与教学班的任课关系。
type Assignment struct {
	TeacherID       int64
	TeacherName     string
	TeachingGroupID int64
}

// ClassTarget 是一个行政班的分档目标人数。
type ClassTarget struct {
	ClassID int64
	Counts  map[string]*float64
}

// SubjectCutoff 是一个学科在两类考生中的分数线。
type SubjectCutoff struct {
	Subject string
	Track   string
	Value   float64
}

// TierCutoff 是总分档位在两类考生中的分数线。
type TierCutoff struct {
	TierKey string
	Track   string
	Value   float64
}

// BatchData 是一个批次读取后可以直接用于核算的数据。
type BatchData struct {
	Batch                     Batch
	Classes                   []Class
	Students                  []Student
	TeachingGroups            []TeachingGroup
	StatGroups                []StatGroup
	Assignments               []Assignment
	ClassTargets              []ClassTarget
	ClassBaselines            map[string]map[string]float64
	ClassBaselineRatios       map[string]map[string]float64
	GradeBaselines            map[string]float64
	SubjectCutoffs            []SubjectCutoff
	TierCutoffs               []TierCutoff
	ExcludedCompletionClasses map[int64]bool
}

// ResultRun 是一次核算记录。
type ResultRun struct {
	ID            int64
	BatchID       int64
	PlanVersionID int64
	InputHash     string
	// InputSummary 是运行时的输入摘要 JSON，数据项标识对应哈希；
	// 旧版本运行没有摘要，为空。
	InputSummary string
	IsOfficial   bool
	CreatedAt    time.Time
}

// StoredResultRow 是一条保存下来的核算结果。
type StoredResultRow struct {
	ID                    int64
	StatGroupID           int64
	StatGroupName         string
	Subject               string
	LeadClassID           int64
	LeadClassName         string
	TeacherNames          string
	ClassCount            int
	ClassAverage          float64
	GradeAverage          float64
	BaselineAverage       float64
	BaselineRatio         float64
	ImprovementRaw        float64
	ImprovementScore      float64
	PassCount             int
	PassTarget            float64
	PassRaw               float64
	PassScore             float64
	CompletionActual      float64
	CompletionBase        float64
	CompletionRaw         float64
	CompletionParticipate bool
	CompletionScore       float64
	Total                 float64
	Rank                  int
	TierIndex             int
	Points                float64
	IndicatorJSON         string
}

// StoredPersonalScore 是一条保存下来的教师个人得分。
type StoredPersonalScore struct {
	TeacherID   int64
	TeacherName string
	Grade       string
	Subject     string
	GroupPoints string
	Score       float64
}

func batchSchema() []sqlite.Statement {
	return []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS terms (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			academic_year TEXT NOT NULL,
			semester TEXT NOT NULL,
			UNIQUE(academic_year, semester)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS batches (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			term_id INTEGER NOT NULL REFERENCES terms(id),
			exam_type TEXT NOT NULL,
			grade TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			current_plan_version_id INTEGER,
			revision INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			UNIQUE(term_id, exam_type, grade)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS imports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			file_hash TEXT NOT NULL,
			relative_path TEXT NOT NULL,
			converted_path TEXT NOT NULL DEFAULT '',
			file_format TEXT NOT NULL,
			conversion_tool TEXT NOT NULL DEFAULT '',
			conversion_report_json TEXT NOT NULL DEFAULT '{}',
			row_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'imported',
			created_at TEXT NOT NULL,
			UNIQUE(batch_id, file_hash)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_sheets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_id INTEGER NOT NULL REFERENCES imports(id) ON DELETE CASCADE,
			sheet_name TEXT NOT NULL,
			role TEXT NOT NULL,
			header_row INTEGER NOT NULL DEFAULT 0,
			data_range TEXT NOT NULL DEFAULT '',
			version_tag TEXT NOT NULL DEFAULT '',
			is_current INTEGER NOT NULL DEFAULT 1,
			UNIQUE(import_id, sheet_name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_columns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_sheet_id INTEGER NOT NULL REFERENCES import_sheets(id) ON DELETE CASCADE,
			source_column TEXT NOT NULL,
			standard_field TEXT NOT NULL,
			merged_header TEXT NOT NULL DEFAULT '',
			is_user_selected INTEGER NOT NULL DEFAULT 0,
			UNIQUE(import_sheet_id, source_column)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS classes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			name TEXT NOT NULL,
			headcount INTEGER NOT NULL DEFAULT 0,
			UNIQUE(batch_id, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL,
			name TEXT NOT NULL,
			UNIQUE(student_no)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS student_classes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			student_id INTEGER NOT NULL REFERENCES students(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			track TEXT NOT NULL DEFAULT '',
			elective TEXT NOT NULL DEFAULT '',
			japanese INTEGER NOT NULL DEFAULT 0,
			walking INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT '',
			total_score REAL,
			UNIQUE(batch_id, student_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS teachers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			teacher_no TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			UNIQUE(name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS teaching_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			subject TEXT NOT NULL,
			name TEXT NOT NULL,
			class_id INTEGER NOT NULL REFERENCES classes(id),
			UNIQUE(batch_id, subject, class_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS stat_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			subject TEXT NOT NULL,
			name TEXT NOT NULL,
			lead_class_id INTEGER NOT NULL REFERENCES classes(id),
			target REAL NOT NULL DEFAULT 0,
			baseline_average REAL NOT NULL DEFAULT 0,
			baseline_ratio REAL NOT NULL DEFAULT 0,
			explicit INTEGER NOT NULL DEFAULT 0,
			UNIQUE(batch_id, subject, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS stat_group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			stat_group_id INTEGER NOT NULL REFERENCES stat_groups(id) ON DELETE CASCADE,
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id),
			UNIQUE(stat_group_id, teaching_group_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			teacher_id INTEGER NOT NULL REFERENCES teachers(id),
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id),
			UNIQUE(teacher_id, teaching_group_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			student_id INTEGER NOT NULL REFERENCES students(id),
			subject TEXT NOT NULL,
			score REAL NOT NULL,
			flag TEXT NOT NULL DEFAULT '',
			UNIQUE(batch_id, student_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS class_targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			tier_key TEXT NOT NULL,
			target_count REAL NOT NULL,
			UNIQUE(batch_id, class_id, tier_key)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS class_baselines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			subject TEXT NOT NULL,
			average REAL NOT NULL,
			ratio REAL NOT NULL DEFAULT 0,
			UNIQUE(batch_id, class_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS grade_baselines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			subject TEXT NOT NULL,
			average REAL NOT NULL,
			UNIQUE(batch_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS subject_cutoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			subject TEXT NOT NULL,
			track TEXT NOT NULL,
			cutoff REAL NOT NULL,
			UNIQUE(batch_id, subject, track)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS tier_cutoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			tier_key TEXT NOT NULL,
			track TEXT NOT NULL,
			cutoff REAL NOT NULL,
			UNIQUE(batch_id, tier_key, track)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS target_adjustments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			tier_key TEXT NOT NULL DEFAULT '',
			old_value REAL,
			new_value REAL,
			reason TEXT NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS result_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER REFERENCES batches(id),
			assessment_id INTEGER REFERENCES assessments(id),
			plan_version_id INTEGER NOT NULL REFERENCES plan_versions(id),
			input_hash TEXT NOT NULL,
			is_official INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS result_rows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL REFERENCES result_runs(id) ON DELETE CASCADE,
			stat_group_id INTEGER NOT NULL,
			stat_group_name TEXT NOT NULL,
			subject TEXT NOT NULL,
			lead_class_id INTEGER NOT NULL,
			lead_class_name TEXT NOT NULL,
			teacher_names TEXT NOT NULL DEFAULT '',
			class_count INTEGER NOT NULL,
			class_average REAL NOT NULL,
			grade_average REAL NOT NULL,
			baseline_average REAL NOT NULL,
			baseline_ratio REAL NOT NULL,
			improvement_score REAL NOT NULL,
			pass_count INTEGER NOT NULL,
			pass_target REAL NOT NULL,
			pass_score REAL NOT NULL,
			completion_score REAL NOT NULL,
			total REAL NOT NULL,
			rank INTEGER NOT NULL,
			tier_index INTEGER NOT NULL,
			points REAL NOT NULL,
			indicator_values_json TEXT NOT NULL DEFAULT '{}'
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS exports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL REFERENCES result_runs(id),
			path TEXT NOT NULL,
			file_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS personal_scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL REFERENCES result_runs(id) ON DELETE CASCADE,
			teacher_id INTEGER NOT NULL,
			teacher_name TEXT NOT NULL,
			grade TEXT NOT NULL,
			subject TEXT NOT NULL,
			group_points_json TEXT NOT NULL DEFAULT '[]',
			score REAL NOT NULL
		)`},
		{SQL: `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
			VALUES (2, ?)`, Args: []any{time.Now().UTC().Format(time.RFC3339Nano)}},
	}
}

// CreateBatch 新建一个考核批次。
func (s *Store) CreateBatch(ctx context.Context, input CreateBatchInput) (Batch, error) {
	if input.AcademicYear == "" || input.Semester == "" {
		return Batch{}, fmt.Errorf("学年和学期不能为空")
	}
	if input.ExamType != "midterm" && input.ExamType != "final" && input.ExamType != "gaokao" {
		return Batch{}, fmt.Errorf("考试类型必须是 midterm、final 或 gaokao")
	}
	if input.Grade != "g1" && input.Grade != "g2" && input.Grade != "g3" {
		return Batch{}, fmt.Errorf("年级必须是 g1、g2 或 g3")
	}
	now := s.now().Format(time.RFC3339Nano)
	statements := []sqlite.Statement{
		{SQL: `INSERT INTO terms(academic_year, semester) VALUES (?, ?)
			ON CONFLICT(academic_year, semester) DO NOTHING`, Args: []any{input.AcademicYear, input.Semester}},
		{SQL: `INSERT INTO batches(term_id, exam_type, grade, status, revision, created_at)
			SELECT id, ?, ?, 'draft', 1, ? FROM terms WHERE academic_year = ? AND semester = ?
			ON CONFLICT(term_id, exam_type, grade) DO NOTHING`,
			Args: []any{input.ExamType, input.Grade, now, input.AcademicYear, input.Semester}},
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return Batch{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT b.id FROM batches b JOIN terms t ON t.id = b.term_id
		WHERE t.academic_year = ? AND t.semester = ? AND b.exam_type = ? AND b.grade = ?`,
		input.AcademicYear, input.Semester, input.ExamType, input.Grade)
	if err != nil {
		return Batch{}, err
	}
	if len(rows.Values) == 0 {
		return Batch{}, ErrNotFound
	}
	id, ok := rows.Values[0][0].(int64)
	if !ok {
		return Batch{}, fmt.Errorf("批次标识格式不正确")
	}
	batch, err := s.GetBatch(ctx, id)
	if err != nil {
		return Batch{}, err
	}
	if _, err := s.ensureBatchStage(ctx, batch); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

// GetBatch 按标识读取一个批次。
func (s *Store) GetBatch(ctx context.Context, id int64) (Batch, error) {
	rows, err := s.db.Query(ctx, `SELECT b.id, b.term_id, t.academic_year, t.semester, b.exam_type,
		b.grade, b.status, b.current_plan_version_id, b.revision, b.created_at
		FROM batches b JOIN terms t ON t.id = b.term_id WHERE b.id = ?`, id)
	if err != nil {
		return Batch{}, err
	}
	if len(rows.Values) == 0 {
		return Batch{}, ErrNotFound
	}
	return batchFromRow(rows.Values[0])
}

// ListBatches 返回全部批次，最近创建的在前。
func (s *Store) ListBatches(ctx context.Context) ([]Batch, error) {
	rows, err := s.db.Query(ctx, `SELECT b.id, b.term_id, t.academic_year, t.semester, b.exam_type,
		b.grade, b.status, b.current_plan_version_id, b.revision, b.created_at
		FROM batches b JOIN terms t ON t.id = b.term_id ORDER BY b.id DESC`)
	if err != nil {
		return nil, err
	}
	batches := make([]Batch, 0, len(rows.Values))
	for _, row := range rows.Values {
		batch, err := batchFromRow(row)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	return batches, nil
}

// SetBatchPlan 记录批次当前选用的方案版本。
func (s *Store) SetBatchPlan(ctx context.Context, batchID, versionID int64) error {
	version, err := s.GetVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if version.Status != "published" {
		return fmt.Errorf("正式核算只能选择已发布的方案版本")
	}
	_, err = s.db.Execute(ctx, `UPDATE batches SET current_plan_version_id = ?, revision = revision + 1
		WHERE id = ?`, versionID, batchID)
	return err
}

// MergeImport 把一次解析的数据合并到批次中，并让已有结果失效。
func (s *Store) MergeImport(ctx context.Context, batchID int64, data ImportData, report ImportReport) (ImportSummary, error) {
	batch, err := s.GetBatch(ctx, batchID)
	if err != nil {
		return ImportSummary{}, err
	}
	stage, err := s.stageForBatch(ctx, batch)
	if err != nil {
		return ImportSummary{}, err
	}
	runID, err := s.ensureImportRun(ctx, stage, report, data)
	if err != nil {
		return ImportSummary{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	for _, class := range data.Classes {
		if class == "" {
			continue
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO classes(cohort_id, name, created_at, import_run_id)
				VALUES (?, ?, ?, ?) ON CONFLICT(cohort_id, name) DO NOTHING`,
			Args: []any{stage.CohortID, normalizeClassName(class), now, runID},
		}, sqlite.Statement{
			SQL: `INSERT INTO class_stage_info(class_id, stage_id, status, import_run_id)
				SELECT id, ?, 'active', ? FROM classes WHERE cohort_id = ? AND name = ?
				ON CONFLICT(class_id, stage_id) DO NOTHING`,
			Args: []any{stage.StageID, runID, stage.CohortID, normalizeClassName(class)},
		})
	}
	teacherSubjects := map[string]map[string]bool{}
	classSubjects := map[string]map[string]bool{}
	for _, assignment := range data.Assignments {
		if assignment.TeacherName == "" || assignment.Subject == "" || assignment.ClassName == "" {
			continue
		}
		if teacherSubjects[assignment.TeacherName] == nil {
			teacherSubjects[assignment.TeacherName] = map[string]bool{}
		}
		teacherSubjects[assignment.TeacherName][assignment.Subject] = true
		if classSubjects[assignment.ClassName] == nil {
			classSubjects[assignment.ClassName] = map[string]bool{}
		}
		classSubjects[assignment.ClassName][assignment.Subject] = true
	}
	for _, teacher := range data.Teachers {
		if teacher == "" || teacherSubjects[teacher] != nil {
			continue
		}
		teacherSubjects[teacher] = map[string]bool{"": true}
	}
	for _, student := range data.Students {
		if student.StudentNo == "" || student.Name == "" || student.ClassName == "" {
			return ImportSummary{}, fmt.Errorf("学生 %q 缺少考号、姓名或者班级", student.Name)
		}
		className := normalizeClassName(student.ClassName)
		if classSubjects[student.ClassName] == nil {
			classSubjects[student.ClassName] = map[string]bool{}
		}
		for subject := range student.Scores {
			classSubjects[student.ClassName][subject] = true
		}
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO students(student_no, name, created_at, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(student_no) DO UPDATE SET
						name = excluded.name, import_run_id = excluded.import_run_id,
						source_sheet = excluded.source_sheet, source_row = excluded.source_row`,
				Args: []any{student.StudentNo, student.Name, now, runID, student.SourceSheet, student.SourceRow},
			},
			sqlite.Statement{
				SQL: `INSERT INTO student_cohorts(student_id, cohort_id, status, import_run_id, source_sheet, source_row)
					SELECT id, ?, 'active', ?, ?, ? FROM students WHERE student_no = ?
					ON CONFLICT(student_id, cohort_id) DO NOTHING`,
				Args: []any{stage.CohortID, runID, student.SourceSheet, student.SourceRow, student.StudentNo},
			},
			sqlite.Statement{
				SQL: `INSERT INTO student_stage_memberships(
					student_id, stage_id, class_id, status, walking, track, elective, japanese, total_score,
					import_run_id, source_sheet, source_row)
					SELECT s.id, ?, c.id, ?, ?, ?, ?, ?, ?, ?, ?, ? FROM students s, classes c
					WHERE s.student_no = ? AND c.cohort_id = ? AND c.name = ?
					ON CONFLICT(student_id, stage_id) DO UPDATE SET class_id = excluded.class_id,
						track = excluded.track, elective = excluded.elective, japanese = excluded.japanese,
						walking = excluded.walking, status = excluded.status, total_score = excluded.total_score,
						import_run_id = excluded.import_run_id, source_sheet = excluded.source_sheet,
						source_row = excluded.source_row`,
				Args: []any{stage.StageID, student.Status, boolInt(student.Walking), student.Track,
					student.Elective, boolInt(student.Japanese), optionalFloat(student.TotalScore),
					runID, student.SourceSheet, student.SourceRow, student.StudentNo, stage.CohortID, className},
			},
		)
		for subject, score := range student.Scores {
			flag := student.Flags[subject]
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO scores(batch_id, student_id, subject, score, flag)
					SELECT ?, id, ?, ?, ? FROM students WHERE student_no = ?
					ON CONFLICT(batch_id, student_id, subject) DO UPDATE SET score = excluded.score, flag = excluded.flag`,
				Args: []any{batchID, subject, score, flag, student.StudentNo},
			})
		}
	}
	for teacher, subjects := range teacherSubjects {
		for subject := range subjects {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO teachers(teacher_no, name, subject, created_at, import_run_id)
					SELECT '', ?, ?, ?, ? WHERE NOT EXISTS (
						SELECT 1 FROM teachers WHERE teacher_no = '' AND name = ? AND subject = ?)`,
				Args: []any{teacher, subject, now, runID, teacher, subject},
			})
		}
	}
	for className, subjects := range classSubjects {
		name := normalizeClassName(className)
		for subject := range subjects {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO teaching_groups(stage_id, subject, name, kind, import_run_id)
					VALUES (?, ?, ?, 'admin', ?) ON CONFLICT(stage_id, subject, name) DO NOTHING`,
				Args: []any{stage.StageID, subject, name, runID},
			})
		}
	}
	for _, assignment := range data.Assignments {
		if assignment.TeacherName == "" {
			continue
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO assignments(teacher_id, teaching_group_id, import_run_id, source_sheet, source_row)
				SELECT t.id, g.id, ?, ?, ? FROM teachers t, teaching_groups g
				WHERE t.teacher_no = '' AND t.name = ? AND t.subject = ?
					AND g.stage_id = ? AND g.subject = ? AND g.name = ?
				ON CONFLICT(teacher_id, teaching_group_id) DO NOTHING`,
			Args: []any{runID, assignment.SourceSheet, assignment.SourceRow,
				assignment.TeacherName, assignment.Subject, stage.StageID, assignment.Subject,
				normalizeClassName(assignment.ClassName)},
		})
	}
	for className, subjects := range classSubjects {
		name := normalizeClassName(className)
		for subject := range subjects {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO student_teaching_groups(
					student_id, stage_id, subject, teaching_group_id, import_run_id)
					SELECT m.student_id, m.stage_id, g.subject, g.id, ?
					FROM student_stage_memberships m
					JOIN classes c ON c.id = m.class_id
					JOIN teaching_groups g ON g.stage_id = m.stage_id AND g.name = c.name AND g.subject = ?
					WHERE m.stage_id = ? AND c.name = ?
					ON CONFLICT(student_id, stage_id, subject) DO UPDATE SET
						teaching_group_id = excluded.teaching_group_id, import_run_id = excluded.import_run_id`,
				Args: []any{runID, subject, stage.StageID, name},
			})
		}
	}
	for _, target := range data.ClassTargets {
		for tier, count := range target.Counts {
			if count == nil {
				continue
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO class_targets(batch_id, class_id, tier_key, target_count)
					SELECT ?, c.id, ?, ? FROM classes c WHERE c.cohort_id = ? AND c.name = ?
					ON CONFLICT(batch_id, class_id, tier_key) DO UPDATE SET target_count = excluded.target_count`,
				Args: []any{batchID, tier, *count, stage.CohortID, normalizeClassName(target.ClassName)},
			})
		}
	}
	for _, baseline := range data.ClassBaselines {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO class_baselines(batch_id, class_id, subject, average, ratio)
				SELECT ?, c.id, ?, ?, ? FROM classes c WHERE c.cohort_id = ? AND c.name = ?
				ON CONFLICT(batch_id, class_id, subject) DO UPDATE SET average = excluded.average,
					ratio = excluded.ratio`,
			Args: []any{batchID, baseline.Subject, baseline.Average, floatOrZero(baseline.Ratio),
				stage.CohortID, normalizeClassName(baseline.ClassName)},
		})
	}
	if len(data.ClassBaselines) > 0 || len(data.GradeBaselines) > 0 {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO baseline_sets(cohort_id, name, source, kind, status, created_at, import_run_id)
				VALUES (?, '入口成绩', 'import', 'zhongkao', 'active', ?, ?)
				ON CONFLICT(cohort_id, name) DO NOTHING`,
			Args: []any{stage.CohortID, now, runID},
		})
	}
	for _, baseline := range data.ClassBaselines {
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO baseline_subjects(baseline_set_id, subject, full_mark)
					SELECT id, ?, ? FROM baseline_sets WHERE cohort_id = ? AND name = '入口成绩'`,
				Args: []any{baseline.Subject, defaultFullMark(baseline.Subject), stage.CohortID},
			},
			sqlite.Statement{
				SQL: `INSERT INTO baseline_aggregates(
					baseline_set_id, stage_id, scope_type, scope_id, subject, average, count, ratio, import_run_id)
					SELECT b.id, ?, 'class', c.id, ?, ?, (
						SELECT COUNT(*) FROM student_stage_memberships m
						WHERE m.stage_id = ? AND m.class_id = c.id), ?, ?
					FROM baseline_sets b, classes c
					WHERE b.cohort_id = ? AND b.name = '入口成绩' AND c.cohort_id = ? AND c.name = ?
					ON CONFLICT(baseline_set_id, stage_id, scope_type, scope_id, subject) DO UPDATE SET
						average = excluded.average, count = excluded.count, ratio = excluded.ratio,
						import_run_id = excluded.import_run_id`,
				Args: []any{stage.StageID, baseline.Subject, baseline.Average, stage.StageID,
					floatOrZero(baseline.Ratio), runID, stage.CohortID, stage.CohortID,
					normalizeClassName(baseline.ClassName)},
			},
		)
	}
	for subject, average := range data.GradeBaselines {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO grade_baselines(batch_id, subject, average) VALUES (?, ?, ?)
				ON CONFLICT(batch_id, subject) DO UPDATE SET average = excluded.average`,
			Args: []any{batchID, subject, average},
		})
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO baseline_subjects(baseline_set_id, subject, full_mark)
					SELECT id, ?, ? FROM baseline_sets WHERE cohort_id = ? AND name = '入口成绩'`,
				Args: []any{subject, defaultFullMark(subject), stage.CohortID},
			},
			sqlite.Statement{
				SQL: `INSERT INTO baseline_aggregates(
					baseline_set_id, stage_id, scope_type, scope_id, subject, average, count, ratio, import_run_id)
					SELECT id, ?, 'stage', ?, ?, ?, (
						SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?), 0, ?
					FROM baseline_sets WHERE cohort_id = ? AND name = '入口成绩'
					ON CONFLICT(baseline_set_id, stage_id, scope_type, scope_id, subject) DO UPDATE SET
						average = excluded.average, count = excluded.count, import_run_id = excluded.import_run_id`,
				Args: []any{stage.StageID, stage.StageID, subject, average, stage.StageID, runID, stage.CohortID},
			},
		)
	}
	for _, cutoff := range data.SubjectCutoffs {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO subject_cutoffs(batch_id, subject, track, cutoff) VALUES (?, ?, ?, ?)
				ON CONFLICT(batch_id, subject, track) DO UPDATE SET cutoff = excluded.cutoff`,
			Args: []any{batchID, cutoff.Subject, cutoff.Track, cutoff.Value},
		})
	}
	for _, cutoff := range data.TierCutoffs {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO tier_cutoffs(batch_id, tier_key, track, cutoff) VALUES (?, ?, ?, ?)
				ON CONFLICT(batch_id, tier_key, track) DO UPDATE SET cutoff = excluded.cutoff`,
			Args: []any{batchID, cutoff.TierKey, cutoff.Track, cutoff.Value},
		})
	}
	for _, group := range data.StatGroups {
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT INTO stat_groups(batch_id, subject, name, lead_class_id, target, baseline_average, explicit)
					SELECT ?, ?, ?, c.id, ?, ?, 1 FROM classes c WHERE c.cohort_id = ? AND c.name = ?
					ON CONFLICT(batch_id, subject, name) DO UPDATE SET lead_class_id = excluded.lead_class_id,
						target = excluded.target, baseline_average = excluded.baseline_average, explicit = 1`,
				Args: []any{batchID, group.Subject, group.Name, group.Target, group.BaselineAverage,
					stage.CohortID, normalizeClassName(group.LeadClassName)},
			},
			sqlite.Statement{
				SQL: `DELETE FROM stat_group_members WHERE stat_group_id =
					(SELECT id FROM stat_groups WHERE batch_id = ? AND subject = ? AND name = ?)`,
				Args: []any{batchID, group.Subject, group.Name},
			},
		)
		for _, className := range group.MemberClasses {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO stat_group_members(stat_group_id, teaching_group_id)
					SELECT sg.id, g.id FROM stat_groups sg, teaching_groups g
					WHERE sg.batch_id = ? AND sg.subject = ? AND sg.name = ?
						AND g.stage_id = ? AND g.subject = ? AND g.name = ?
					ON CONFLICT(stat_group_id, teaching_group_id) DO NOTHING`,
				Args: []any{batchID, group.Subject, group.Name, stage.StageID, group.Subject, normalizeClassName(className)},
			})
		}
	}
	statements = append(statements,
		sqlite.Statement{
			SQL:  `DELETE FROM result_rows WHERE run_id IN (SELECT id FROM result_runs WHERE batch_id = ?)`,
			Args: []any{batchID},
		},
		sqlite.Statement{
			SQL:  `DELETE FROM result_runs WHERE batch_id = ?`,
			Args: []any{batchID},
		},
		sqlite.Statement{
			SQL:  `UPDATE batches SET status = 'imported', revision = revision + 1 WHERE id = ?`,
			Args: []any{batchID},
		},
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		if runID != 0 {
			_, _ = s.db.Execute(ctx, `DELETE FROM import_runs WHERE id = ?`, runID)
		}
		return ImportSummary{}, err
	}
	if _, err := s.db.Execute(ctx, `DELETE FROM stat_groups WHERE batch_id = ? AND explicit = 0`, batchID); err != nil {
		return ImportSummary{}, err
	}
	if err := s.rebuildDefaultStatGroups(ctx, batchID); err != nil {
		return ImportSummary{}, err
	}
	if report.FileHash != "" {
		if err := s.saveImportSheets(ctx, runID, report); err != nil {
			return ImportSummary{}, err
		}
	}
	summary, err := s.importSummary(ctx, batchID, data)
	if err != nil {
		return ImportSummary{}, err
	}
	summary.Warnings = append(summary.Warnings, report.Warnings...)
	return summary, nil
}

func (s *Store) ensureImportRun(ctx context.Context, stage migrationBatch, report ImportReport, data ImportData) (int64, error) {
	if report.FileHash == "" {
		return 0, nil
	}
	stored := report.StoredPath
	if stored == "" {
		stored = report.Path
	}
	if _, err := s.db.Execute(ctx, `INSERT INTO import_runs(
		scope_type, scope_id, data_kind, file_hash, stored_path, relative_path, converted_path,
		status, row_count, warning_count, created_at)
		VALUES ('stage', ?, 'workbook', ?, ?, ?, ?, 'imported', ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, data_kind, file_hash) DO UPDATE SET
			stored_path = excluded.stored_path, relative_path = excluded.relative_path,
			converted_path = excluded.converted_path, row_count = excluded.row_count,
			warning_count = excluded.warning_count, created_at = excluded.created_at`,
		stage.StageID, report.FileHash, stored, report.Path, report.ConvertedPath,
		report.RowCount, len(report.Warnings), s.now().Format(time.RFC3339Nano)); err != nil {
		return 0, err
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM import_runs
		WHERE scope_type = 'stage' AND scope_id = ? AND data_kind = 'workbook' AND file_hash = ?`,
		stage.StageID, report.FileHash)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, fmt.Errorf("导入记录写入失败")
	}
	id, _ := rows.Values[0][0].(int64)
	return id, nil
}

func (s *Store) saveImportSheets(ctx context.Context, runID int64, report ImportReport) error {
	if runID == 0 {
		return nil
	}
	statements := []sqlite.Statement{{SQL: `DELETE FROM import_sheets WHERE import_run_id = ?`, Args: []any{runID}}}
	for _, sheet := range report.Sheets {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_sheets(import_run_id, sheet_name, role, header_row, data_range, is_current)
				VALUES (?, ?, ?, ?, ?, 1)`,
			Args: []any{runID, sheet.Name, sheet.Role, sheet.HeaderRow, fmt.Sprintf("%d rows", sheet.Rows)},
		})
	}
	_, err := s.db.Transaction(ctx, statements)
	return err
}

func (s *Store) rebuildDefaultStatGroups(ctx context.Context, batchID int64) error {
	batch, err := s.GetBatch(ctx, batchID)
	if err != nil {
		return err
	}
	stage, err := s.stageForBatch(ctx, batch)
	if err != nil {
		return err
	}
	statements := []sqlite.Statement{
		{SQL: `INSERT INTO stat_groups(batch_id, subject, name, lead_class_id, target, baseline_average, baseline_ratio, explicit)
			SELECT ?, g.subject, g.name, c.id,
				COALESCE((SELECT ct.target_count FROM class_targets ct
					WHERE ct.batch_id = ? AND ct.class_id = c.id AND ct.tier_key = 'single_subject'),
					(SELECT ct.target_count FROM class_targets ct
						WHERE ct.batch_id = ? AND ct.class_id = c.id AND ct.tier_key = 'undergraduate'), 0),
				COALESCE((SELECT cb.average FROM class_baselines cb
					WHERE cb.batch_id = ? AND cb.class_id = c.id AND cb.subject = g.subject), 0),
				COALESCE((SELECT cb.ratio FROM class_baselines cb
					WHERE cb.batch_id = ? AND cb.class_id = c.id AND cb.subject = g.subject), 0),
				0
			FROM teaching_groups g
			JOIN classes c ON c.cohort_id = ? AND c.name = g.name
			WHERE g.stage_id = ?
			AND NOT EXISTS (SELECT 1 FROM stat_groups sg WHERE sg.batch_id = ? AND sg.subject = g.subject
				AND sg.name = g.name)
			AND NOT EXISTS (SELECT 1 FROM stat_group_members sgm
				JOIN stat_groups explicit_group ON explicit_group.id = sgm.stat_group_id
				WHERE explicit_group.batch_id = ? AND explicit_group.subject = g.subject
					AND explicit_group.explicit = 1 AND sgm.teaching_group_id = g.id)
			ON CONFLICT(batch_id, subject, name) DO NOTHING`,
			Args: []any{batchID, batchID, batchID, batchID, batchID, stage.CohortID, stage.StageID, batchID, batchID}},
		{SQL: `INSERT INTO stat_group_members(stat_group_id, teaching_group_id)
			SELECT sg.id, g.id FROM stat_groups sg, teaching_groups g
			WHERE sg.batch_id = ? AND sg.explicit = 0 AND g.stage_id = ?
				AND g.subject = sg.subject AND g.name = sg.name
			ON CONFLICT(stat_group_id, teaching_group_id) DO NOTHING`, Args: []any{batchID, stage.StageID}},
	}
	_, err = s.db.Transaction(ctx, statements)
	return err
}

func (s *Store) importSummary(ctx context.Context, batchID int64, data ImportData) (ImportSummary, error) {
	batch, err := s.GetBatch(ctx, batchID)
	if err != nil {
		return ImportSummary{}, err
	}
	stage, err := s.stageForBatch(ctx, batch)
	if err != nil {
		return ImportSummary{}, err
	}
	count := func(query string, args ...any) (int, error) {
		rows, err := s.db.Query(ctx, query, args...)
		if err != nil {
			return 0, err
		}
		if len(rows.Values) == 0 {
			return 0, nil
		}
		value, ok := rows.Values[0][0].(int64)
		if !ok {
			return 0, fmt.Errorf("统计数量格式不正确")
		}
		return int(value), nil
	}
	summary := ImportSummary{}
	if summary.Classes, err = count(`SELECT COUNT(*) FROM classes c
		JOIN class_stage_info i ON i.class_id = c.id WHERE i.stage_id = ?`, stage.StageID); err != nil {
		return summary, err
	}
	if summary.Students, err = count(`SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?`,
		stage.StageID); err != nil {
		return summary, err
	}
	if summary.Teachers, err = count(`SELECT COUNT(DISTINCT a.teacher_id) FROM assignments a
		JOIN teaching_groups g ON g.id = a.teaching_group_id WHERE g.stage_id = ?`, stage.StageID); err != nil {
		return summary, err
	}
	if summary.TeachingGroups, err = count(`SELECT COUNT(*) FROM teaching_groups WHERE stage_id = ?`,
		stage.StageID); err != nil {
		return summary, err
	}
	if summary.StatGroups, err = count(`SELECT COUNT(*) FROM stat_groups WHERE batch_id = ?`, batchID); err != nil {
		return summary, err
	}
	if summary.Assignments, err = count(`SELECT COUNT(*) FROM assignments a
		JOIN teaching_groups g ON g.id = a.teaching_group_id WHERE g.stage_id = ?`, stage.StageID); err != nil {
		return summary, err
	}
	if summary.ClassTargets, err = count(`SELECT COUNT(*) FROM class_targets WHERE batch_id = ?`, batchID); err != nil {
		return summary, err
	}
	if summary.SubjectCutoffs, err = count(`SELECT COUNT(*) FROM subject_cutoffs WHERE batch_id = ?`, batchID); err != nil {
		return summary, err
	}
	if summary.TierCutoffs, err = count(`SELECT COUNT(*) FROM tier_cutoffs WHERE batch_id = ?`, batchID); err != nil {
		return summary, err
	}
	if summary.Students == 0 {
		summary.Warnings = append(summary.Warnings, "没有导入学生成绩")
	}
	if summary.SubjectCutoffs == 0 {
		summary.Warnings = append(summary.Warnings, "没有导入学科分数线")
	}
	if summary.TierCutoffs == 0 {
		summary.Warnings = append(summary.Warnings, "没有导入总分档位分数线")
	}
	return summary, nil
}

// LoadBatchData 读取一个批次参与核算的全部数据。
func (s *Store) LoadBatchData(ctx context.Context, batchID int64) (BatchData, error) {
	batch, err := s.GetBatch(ctx, batchID)
	if err != nil {
		return BatchData{}, err
	}
	stage, err := s.stageForBatch(ctx, batch)
	if err != nil {
		return BatchData{}, err
	}
	data := BatchData{
		Batch:                     batch,
		ClassBaselines:            map[string]map[string]float64{},
		ClassBaselineRatios:       map[string]map[string]float64{},
		GradeBaselines:            map[string]float64{},
		ExcludedCompletionClasses: map[int64]bool{},
	}
	classesByID := map[int64]string{}
	rows, err := s.db.Query(ctx, `SELECT c.id, c.name FROM classes c
		JOIN class_stage_info i ON i.class_id = c.id WHERE i.stage_id = ? ORDER BY c.name`, stage.StageID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		name, _ := row[1].(string)
		classesByID[id] = name
		data.Classes = append(data.Classes, Class{ID: id, Name: name})
	}
	rows, err = s.db.Query(ctx, `SELECT st.id, st.student_no, st.name, m.class_id, m.track, m.elective,
		m.japanese, m.walking, m.status, m.total_score
		FROM student_stage_memberships m JOIN students st ON st.id = m.student_id
		WHERE m.stage_id = ? ORDER BY st.student_no`, stage.StageID)
	if err != nil {
		return BatchData{}, err
	}
	studentIndex := map[int64]int{}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		studentNo, _ := row[1].(string)
		name, _ := row[2].(string)
		classID, _ := row[3].(int64)
		track, _ := row[4].(string)
		elective, _ := row[5].(string)
		japanese, _ := row[6].(int64)
		walking, _ := row[7].(int64)
		status, _ := row[8].(string)
		total, _ := nullableFloat(row[9])
		studentIndex[id] = len(data.Students)
		data.Students = append(data.Students, Student{
			ID:             id,
			StudentNo:      studentNo,
			Name:           name,
			ClassID:        classID,
			Track:          track,
			Elective:       elective,
			Japanese:       japanese != 0,
			Walking:        walking != 0,
			Status:         status,
			TotalScore:     total,
			Scores:         map[string]float64{},
			ScoreFlags:     map[string]string{},
			TeachingGroups: map[string]int64{},
		})
	}
	rows, err = s.db.Query(ctx, `SELECT student_id, subject, score, flag FROM scores WHERE batch_id = ?`, batchID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		studentID, _ := row[0].(int64)
		subject, _ := row[1].(string)
		score, _ := row[2].(float64)
		flag, _ := row[3].(string)
		index, ok := studentIndex[studentID]
		if !ok {
			continue
		}
		data.Students[index].Scores[subject] = score
		data.Students[index].ScoreFlags[subject] = flag
	}
	rows, err = s.db.Query(ctx, `SELECT g.id, g.subject, g.name, COALESCE(c.id, 0)
		FROM teaching_groups g
		LEFT JOIN classes c ON c.cohort_id = ? AND c.name = g.name
		WHERE g.stage_id = ? ORDER BY g.subject, g.name`, stage.CohortID, stage.StageID)
	if err != nil {
		return BatchData{}, err
	}
	groupIndex := map[int64]int{}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		subject, _ := row[1].(string)
		name, _ := row[2].(string)
		classID, _ := row[3].(int64)
		groupIndex[id] = len(data.TeachingGroups)
		data.TeachingGroups = append(data.TeachingGroups, TeachingGroup{ID: id, Subject: subject, Name: name, ClassID: classID})
	}
	for i := range data.Students {
		for _, group := range data.TeachingGroups {
			if group.ClassID == data.Students[i].ClassID {
				data.Students[i].TeachingGroups[group.Subject] = group.ID
			}
		}
	}
	rows, err = s.db.Query(ctx, `SELECT student_id, subject, teaching_group_id
		FROM student_teaching_groups WHERE stage_id = ?`, stage.StageID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		studentID, _ := row[0].(int64)
		subject, _ := row[1].(string)
		groupID, _ := row[2].(int64)
		if index, ok := studentIndex[studentID]; ok {
			data.Students[index].TeachingGroups[subject] = groupID
		}
	}
	rows, err = s.db.Query(ctx, `SELECT sg.id, sg.subject, sg.name, sg.lead_class_id, sg.target, sg.baseline_average,
		sg.baseline_ratio
		FROM stat_groups sg WHERE sg.batch_id = ? ORDER BY sg.subject, sg.name`, batchID)
	if err != nil {
		return BatchData{}, err
	}
	statIndex := map[int64]int{}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		subject, _ := row[1].(string)
		name, _ := row[2].(string)
		leadClassID, _ := row[3].(int64)
		target, _ := row[4].(float64)
		baseline, _ := row[5].(float64)
		ratio, _ := row[6].(float64)
		statIndex[id] = len(data.StatGroups)
		data.StatGroups = append(data.StatGroups, StatGroup{
			ID:              id,
			Subject:         subject,
			Name:            name,
			LeadClassID:     leadClassID,
			Target:          target,
			BaselineAverage: baseline,
			BaselineRatio:   ratio,
		})
	}
	rows, err = s.db.Query(ctx, `SELECT stat_group_id, teaching_group_id FROM stat_group_members`)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		statID, _ := row[0].(int64)
		groupID, _ := row[1].(int64)
		index, ok := statIndex[statID]
		if !ok {
			continue
		}
		data.StatGroups[index].MemberGroupIDs = append(data.StatGroups[index].MemberGroupIDs, groupID)
	}
	rows, err = s.db.Query(ctx, `SELECT a.teacher_id, t.name, a.teaching_group_id
		FROM assignments a JOIN teachers t ON t.id = a.teacher_id
		JOIN teaching_groups g ON g.id = a.teaching_group_id WHERE g.stage_id = ?`, stage.StageID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		teacherID, _ := row[0].(int64)
		name, _ := row[1].(string)
		groupID, _ := row[2].(int64)
		data.Assignments = append(data.Assignments, Assignment{
			TeacherID:       teacherID,
			TeacherName:     name,
			TeachingGroupID: groupID,
		})
	}
	targetIndex := map[int64]int{}
	rows, err = s.db.Query(ctx, `SELECT class_id, tier_key, target_count FROM class_targets WHERE batch_id = ?`, batchID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		classID, _ := row[0].(int64)
		tier, _ := row[1].(string)
		value, _ := row[2].(float64)
		index, ok := targetIndex[classID]
		if !ok {
			index = len(data.ClassTargets)
			targetIndex[classID] = index
			data.ClassTargets = append(data.ClassTargets, ClassTarget{ClassID: classID, Counts: map[string]*float64{}})
		}
		copied := value
		data.ClassTargets[index].Counts[tier] = &copied
	}
	rows, err = s.db.Query(ctx, `SELECT class_id, tier_key, new_value FROM target_adjustments
		WHERE batch_id = ? ORDER BY id`, batchID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		classID, _ := row[0].(int64)
		tier, _ := row[1].(string)
		value, hasValue := nullableFloat(row[2])
		if tier == "" {
			data.ExcludedCompletionClasses[classID] = true
			continue
		}
		index, ok := targetIndex[classID]
		if !ok {
			index = len(data.ClassTargets)
			targetIndex[classID] = index
			data.ClassTargets = append(data.ClassTargets, ClassTarget{ClassID: classID, Counts: map[string]*float64{}})
		}
		if hasValue {
			copied := *value
			data.ClassTargets[index].Counts[tier] = &copied
		}
	}
	rows, err = s.db.Query(ctx, `SELECT class_id, subject, average, ratio FROM class_baselines WHERE batch_id = ?`, batchID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		classID, _ := row[0].(int64)
		subject, _ := row[1].(string)
		average, _ := row[2].(float64)
		ratio, _ := row[3].(float64)
		if data.ClassBaselines[subject] == nil {
			data.ClassBaselines[subject] = map[string]float64{}
		}
		data.ClassBaselines[subject][strconv.FormatInt(classID, 10)] = average
		if ratio > 0 {
			if data.ClassBaselineRatios[subject] == nil {
				data.ClassBaselineRatios[subject] = map[string]float64{}
			}
			data.ClassBaselineRatios[subject][strconv.FormatInt(classID, 10)] = ratio
		}
	}
	rows, err = s.db.Query(ctx, `SELECT subject, average FROM grade_baselines WHERE batch_id = ?`, batchID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		subject, _ := row[0].(string)
		average, _ := row[1].(float64)
		data.GradeBaselines[subject] = average
	}
	rows, err = s.db.Query(ctx, `SELECT subject, track, cutoff FROM subject_cutoffs WHERE batch_id = ?`, batchID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		subject, _ := row[0].(string)
		track, _ := row[1].(string)
		value, _ := row[2].(float64)
		data.SubjectCutoffs = append(data.SubjectCutoffs, SubjectCutoff{Subject: subject, Track: track, Value: value})
	}
	rows, err = s.db.Query(ctx, `SELECT tier_key, track, cutoff FROM tier_cutoffs WHERE batch_id = ?`, batchID)
	if err != nil {
		return BatchData{}, err
	}
	for _, row := range rows.Values {
		tier, _ := row[0].(string)
		track, _ := row[1].(string)
		value, _ := row[2].(float64)
		data.TierCutoffs = append(data.TierCutoffs, TierCutoff{TierKey: tier, Track: track, Value: value})
	}
	return data, nil
}

// AdjustTarget 记录一次目标补录。
func (s *Store) AdjustTarget(ctx context.Context, batchID, classID int64, tierKey string, oldValue, newValue *float64, reason, user string) error {
	if reason == "" {
		return fmt.Errorf("补录原因不能为空")
	}
	_, err := s.db.Execute(ctx, `INSERT INTO target_adjustments(batch_id, class_id, tier_key, old_value,
		new_value, reason, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		batchID, classID, tierKey, optionalFloat(oldValue), optionalFloat(newValue), reason, user, s.now().Format(time.RFC3339Nano))
	return err
}

// ExcludeClassFromCompletion 记录一个班级不参与完成率计算。
func (s *Store) ExcludeClassFromCompletion(ctx context.Context, batchID, classID int64, reason, user string) error {
	if reason == "" {
		return fmt.Errorf("不参与的原因不能为空")
	}
	_, err := s.db.Execute(ctx, `INSERT INTO target_adjustments(batch_id, class_id, tier_key, reason, created_by, created_at)
		VALUES (?, ?, '', ?, ?, ?)`, batchID, classID, reason, user, s.now().Format(time.RFC3339Nano))
	return err
}

// SaveRun 保存一次核算结果，并把批次标记为已核算。
func (s *Store) SaveRun(ctx context.Context, batchID, versionID int64, inputHash string, result calc.Result) (int64, error) {
	now := s.now().Format(time.RFC3339Nano)
	statements := []sqlite.Statement{
		{SQL: `UPDATE result_runs SET is_official = 0 WHERE batch_id = ?`, Args: []any{batchID}},
		{SQL: `INSERT INTO result_runs(batch_id, plan_version_id, input_hash, is_official, created_at)
			VALUES (?, ?, ?, 1, ?)`, Args: []any{batchID, versionID, inputHash, now}},
	}
	transaction, err := s.db.Transaction(ctx, statements)
	if err != nil {
		return 0, err
	}
	runID := transaction[1].LastInsertID
	rowStatements := make([]sqlite.Statement, 0, len(result.Rows))
	for _, row := range result.Rows {
		indicatorJSON, err := json.Marshal(row.IndicatorValues)
		if err != nil {
			return 0, err
		}
		rowStatements = append(rowStatements, sqlite.Statement{
			SQL: `INSERT INTO result_rows(run_id, stat_group_id, stat_group_name, subject, lead_class_id,
				lead_class_name, teacher_names, class_count, class_average, grade_average, baseline_average,
				baseline_ratio, improvement_score, pass_count, pass_target, pass_score, completion_score, total, rank, tier_index, points,
				indicator_values_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{runID, row.StatGroupID, row.StatGroupName, row.Subject, row.LeadClassID,
				row.LeadClassName, joinNames(row.TeacherNames), row.ClassCount, row.ClassAverage,
				row.GradeAverage, row.BaselineAverage, row.BaselineRatio, row.ImprovementScore,
				row.PassCount, row.PassTarget, row.PassScore, row.CompletionScore, row.Total, row.Rank,
				row.TierIndex, row.Points, string(indicatorJSON)},
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
	rowStatements = append(rowStatements, sqlite.Statement{
		SQL:  `UPDATE batches SET status = 'calculated', current_plan_version_id = ?, revision = revision + 1 WHERE id = ?`,
		Args: []any{versionID, batchID},
	})
	if _, err := s.db.Transaction(ctx, rowStatements); err != nil {
		return 0, err
	}
	return runID, nil
}

// LatestRun 返回批次最新的一次核算与结果行。
func (s *Store) LatestRun(ctx context.Context, batchID int64) (ResultRun, []StoredResultRow, []StoredPersonalScore, error) {
	rows, err := s.db.Query(ctx, `SELECT id, batch_id, plan_version_id, input_hash, is_official, created_at
		FROM result_runs WHERE batch_id = ? ORDER BY id DESC LIMIT 1`, batchID)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	if len(rows.Values) == 0 {
		return ResultRun{}, nil, nil, ErrNotFound
	}
	row := rows.Values[0]
	id, _ := row[0].(int64)
	batch, _ := row[1].(int64)
	version, _ := row[2].(int64)
	hash, _ := row[3].(string)
	official, _ := row[4].(int64)
	createdAt, err := parseTime(row[5])
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	run := ResultRun{ID: id, BatchID: batch, PlanVersionID: version, InputHash: hash, IsOfficial: official != 0, CreatedAt: createdAt}
	rows, err = s.db.Query(ctx, `SELECT stat_group_id, stat_group_name, subject, lead_class_id, lead_class_name,
		teacher_names, class_count, class_average, grade_average, baseline_average, baseline_ratio,
		improvement_score, pass_count, pass_target, pass_score, completion_score, total, rank, tier_index,
		points, indicator_values_json
		FROM result_rows WHERE run_id = ? ORDER BY subject, rank, stat_group_name`, id)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	result := make([]StoredResultRow, 0, len(rows.Values))
	for _, row := range rows.Values {
		item := StoredResultRow{}
		item.StatGroupID, _ = row[0].(int64)
		item.StatGroupName, _ = row[1].(string)
		item.Subject, _ = row[2].(string)
		item.LeadClassID, _ = row[3].(int64)
		item.LeadClassName, _ = row[4].(string)
		item.TeacherNames, _ = row[5].(string)
		count, _ := row[6].(int64)
		item.ClassCount = int(count)
		item.ClassAverage, _ = row[7].(float64)
		item.GradeAverage, _ = row[8].(float64)
		item.BaselineAverage, _ = row[9].(float64)
		item.BaselineRatio, _ = row[10].(float64)
		item.ImprovementScore, _ = row[11].(float64)
		pass, _ := row[12].(int64)
		item.PassCount = int(pass)
		item.PassTarget, _ = row[13].(float64)
		item.PassScore, _ = row[14].(float64)
		item.CompletionScore, _ = row[15].(float64)
		item.Total, _ = row[16].(float64)
		rank, _ := row[17].(int64)
		item.Rank = int(rank)
		tier, _ := row[18].(int64)
		item.TierIndex = int(tier)
		item.Points, _ = row[19].(float64)
		item.IndicatorJSON, _ = row[20].(string)
		result = append(result, item)
	}
	personalRows, err := s.db.Query(ctx, `SELECT teacher_id, teacher_name, grade, subject, group_points_json, score
		FROM personal_scores WHERE run_id = ? ORDER BY subject, teacher_name`, id)
	if err != nil {
		return ResultRun{}, nil, nil, err
	}
	personal := make([]StoredPersonalScore, 0, len(personalRows.Values))
	for _, row := range personalRows.Values {
		item := StoredPersonalScore{}
		item.TeacherID, _ = row[0].(int64)
		item.TeacherName, _ = row[1].(string)
		item.Grade, _ = row[2].(string)
		item.Subject, _ = row[3].(string)
		item.GroupPoints, _ = row[4].(string)
		item.Score, _ = row[5].(float64)
		personal = append(personal, item)
	}
	return run, result, personal, nil
}

// SaveExport 记录一次导出。
func (s *Store) SaveExport(ctx context.Context, runID int64, path, hash string) error {
	_, err := s.db.Execute(ctx, `INSERT INTO exports(run_id, path, file_hash, created_at) VALUES (?, ?, ?, ?)`,
		runID, path, hash, s.now().Format(time.RFC3339Nano))
	return err
}

// InputHash 计算一组方案与批次数据的稳定摘要。
func InputHash(plan assessment.Config, data BatchData) (string, error) {
	data.Batch.Status = ""
	data.Batch.Revision = 0
	data.Batch.CurrentPlanVersionID = 0
	data.Batch.CreatedAt = time.Time{}
	payload := struct {
		Plan assessment.Config
		Data BatchData
	}{Plan: plan, Data: data}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// 输入摘要里数据项的展示顺序。
var inputItemOrder = []string{
	"roster", "scores", "cutoffs", "targets", "groups",
	"baseline", "assignments", "classes", "exclusions",
}

// InputSummary 分别计算方案与每个数据项的哈希，保存在核算运行记录里，
// 之后按项对比找出哪些数据变化过。
func InputSummary(plan assessment.Config, data BatchData) (map[string]string, error) {
	summary := map[string]string{}
	hash := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(encoded)
		summary[key] = hex.EncodeToString(sum[:])
		return nil
	}
	if err := hash("plan", plan); err != nil {
		return nil, err
	}
	type rosterEntry struct {
		ID       int64  `json:"id"`
		No       string `json:"no"`
		Name     string `json:"name"`
		ClassID  int64  `json:"class_id"`
		Track    string `json:"track"`
		Elective string `json:"elective"`
		Japanese bool   `json:"japanese"`
		Walking  bool   `json:"walking"`
		Status   string `json:"status"`
	}
	type scoreEntry struct {
		ID      int64    `json:"id"`
		Subject string   `json:"subject"`
		Score   *float64 `json:"score"`
		Flag    string   `json:"flag"`
		Total   *float64 `json:"total"`
	}
	roster := make([]rosterEntry, 0, len(data.Students))
	scores := []scoreEntry{}
	for _, student := range data.Students {
		roster = append(roster, rosterEntry{
			ID: student.ID, No: student.StudentNo, Name: student.Name, ClassID: student.ClassID,
			Track: student.Track, Elective: student.Elective, Japanese: student.Japanese,
			Walking: student.Walking, Status: student.Status,
		})
		if student.TotalScore != nil {
			scores = append(scores, scoreEntry{ID: student.ID, Total: student.TotalScore})
		}
		subjects := make([]string, 0, len(student.Scores))
		for subject := range student.Scores {
			subjects = append(subjects, subject)
		}
		sort.Strings(subjects)
		for _, subject := range subjects {
			value := student.Scores[subject]
			scores = append(scores, scoreEntry{
				ID: student.ID, Subject: subject, Score: &value, Flag: student.ScoreFlags[subject],
			})
		}
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
	for _, group := range data.StatGroups {
		groupTargets = append(groupTargets, groupTarget{ID: group.ID, Target: group.Target})
		groupShapes = append(groupShapes, groupShape{
			ID: group.ID, Subject: group.Subject, Name: group.Name,
			Lead: group.LeadClassID, Members: group.MemberGroupIDs,
		})
	}
	excluded := make([]string, 0, len(data.ExcludedCompletionClasses))
	for classID := range data.ExcludedCompletionClasses {
		excluded = append(excluded, strconv.FormatInt(classID, 10))
	}
	sort.Strings(excluded)
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
	}
	keys := append([]string(nil), inputItemOrder...)
	keys = append(keys, "plan")
	for _, key := range keys {
		value := any(plan)
		if key != "plan" {
			value = items[key]
		}
		if err := hash(key, value); err != nil {
			return nil, err
		}
	}
	return summary, nil
}

// ChangedInputItems 对比运行时保存的输入摘要与当前摘要，
// 返回变化的数据项标识，按 inputItemOrder 排序。
// 保存的摘要为空时返回 nil，调用方回退到整体哈希对比。
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
	for key, value := range current {
		if key == "plan" {
			continue
		}
		if stored[key] != value {
			changed[key] = true
		}
	}
	result := make([]string, 0, len(changed))
	for _, key := range inputItemOrder {
		if changed[key] {
			result = append(result, key)
		}
	}
	return result, nil
}

func batchFromRow(row []any) (Batch, error) {
	id, ok := row[0].(int64)
	if !ok {
		return Batch{}, fmt.Errorf("批次标识格式不正确")
	}
	termID, _ := row[1].(int64)
	academicYear, _ := row[2].(string)
	semester, _ := row[3].(string)
	examType, _ := row[4].(string)
	grade, _ := row[5].(string)
	status, _ := row[6].(string)
	version, _ := nullableInt64(row[7])
	revision, _ := row[8].(int64)
	createdAt, err := parseTime(row[9])
	if err != nil {
		return Batch{}, err
	}
	return Batch{
		ID:                   id,
		TermID:               termID,
		AcademicYear:         academicYear,
		Semester:             semester,
		ExamType:             examType,
		Grade:                grade,
		Status:               status,
		CurrentPlanVersionID: version,
		Revision:             int(revision),
		CreatedAt:            createdAt,
	}, nil
}

func nullableFloat(value any) (*float64, bool) {
	if value == nil {
		return nil, false
	}
	number, ok := value.(float64)
	if !ok {
		return nil, false
	}
	return &number, true
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func optionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func floatOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func joinNames(names []string) string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	encoded, err := json.Marshal(sorted)
	if err != nil {
		return ""
	}
	return string(encoded)
}
