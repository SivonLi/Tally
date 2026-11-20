package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// Subject 是一个学科设置。
type Subject struct {
	Key      string
	Name     string
	FullMark float64
	Sort     int
	Enabled  bool
}

// Track 是一个选科类别。
type Track struct {
	Key     string
	Name    string
	Sort    int
	Enabled bool
}

// GradeLevel 是阶段名称。
type GradeLevel struct {
	Key  string
	Name string
	Sort int
}

// SchoolClass 是一个级中的行政班。
type SchoolClass struct {
	ID           int64
	CohortID     int64
	Name         string
	StageID      int64
	Stage        string
	AcademicYear int
	Students     int
	HeadTeacher  string
}

// DataStudent 是数据页面显示的学生。
type DataStudent struct {
	ID             int64
	StudentNo      string
	Name           string
	CohortID       int64
	StageID        int64
	ClassID        int64
	ClassName      string
	Track          string
	Elective       string
	Status         string
	Walking        bool
	Japanese       bool
	TotalScore     *float64
	TeachingGroups []string
	ImportRunID    int64
	SourceSheet    string
	SourceRow      int
}

// StudentFilter 是学生列表的查询条件。
type StudentFilter struct {
	StageID  int64
	ClassID  int64
	Track    string
	Status   string
	Query    string
	Page     int
	PageSize int
}

// DataTeacher 是数据页面显示的教师。
type DataTeacher struct {
	ID          int64
	TeacherNo   string
	Name        string
	Subject     string
	Groups      int
	Assignments int
	ImportRunID int64
	SourceSheet string
	SourceRow   int
}

// DataTeachingGroup 是数据页面显示的教学班。
type DataTeachingGroup struct {
	ID          int64
	StageID     int64
	Subject     string
	Name        string
	Kind        string
	Students    int
	Teachers    string
	ClassNames  string
	ImportRunID int64
	SourceSheet string
	SourceRow   int
}

// BaselineRow 是入口成绩汇总。
type BaselineRow struct {
	BaselineSetID int64
	SetName       string
	ClassID       int64
	ClassName     string
	Subject       string
	Average       float64
	Count         int
	Ratio         float64
	ImportRunID   int64
	SourceSheet   string
	SourceRow     int
}

// ImportRun 是一条导入记录。
type ImportRun struct {
	ID           int64
	ScopeType    string
	ScopeID      int64
	DataKind     string
	FileHash     string
	StoredPath   string
	RelativePath string
	Status       string
	RowCount     int
	Warnings     int
	CreatedAt    time.Time
	Sheets       []ImportSheetInfo
}

// ImportSheetInfo 是导入记录中的一个工作表。
type ImportSheetInfo struct {
	Name      string
	Role      string
	HeaderRow int
	Rows      int
	Current   bool
}

func schoolDataSchema() []sqlite.Statement {
	return []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS subjects (
			"key" TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			full_mark REAL NOT NULL DEFAULT 150,
			sort_order INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS tracks (
			"key" TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS grade_levels (
			"key" TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS classes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			name TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(cohort_id, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS class_stage_info (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			class_id INTEGER NOT NULL REFERENCES classes(id),
			stage_id INTEGER NOT NULL REFERENCES cohort_stages(id),
			head_teacher_id INTEGER,
			room TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(class_id, stage_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS student_cohorts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL REFERENCES students(id),
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			graduation_year_override INTEGER,
			status TEXT NOT NULL DEFAULT 'active',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(student_id, cohort_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS student_stage_memberships (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL REFERENCES students(id),
			stage_id INTEGER NOT NULL REFERENCES cohort_stages(id),
			class_id INTEGER NOT NULL REFERENCES classes(id),
			status TEXT NOT NULL DEFAULT '',
			walking INTEGER NOT NULL DEFAULT 0,
			track TEXT NOT NULL DEFAULT '',
			elective TEXT NOT NULL DEFAULT '',
			japanese INTEGER NOT NULL DEFAULT 0,
			total_score REAL,
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(student_id, stage_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS teachers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			teacher_no TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS teachers_no
			ON teachers(teacher_no) WHERE teacher_no <> ''`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS teachers_name_subject
			ON teachers(name, subject) WHERE teacher_no = ''`},
		{SQL: `CREATE TABLE IF NOT EXISTS teaching_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			stage_id INTEGER NOT NULL REFERENCES cohort_stages(id),
			subject TEXT NOT NULL,
			name TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT 'admin',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(stage_id, subject, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS student_teaching_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL REFERENCES students(id),
			stage_id INTEGER NOT NULL REFERENCES cohort_stages(id),
			subject TEXT NOT NULL,
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id),
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(student_id, stage_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			teacher_id INTEGER NOT NULL REFERENCES teachers(id),
			teaching_group_id INTEGER NOT NULL REFERENCES teaching_groups(id),
			role TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(teacher_id, teaching_group_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS baseline_sets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			name TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(cohort_id, name)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS baseline_subjects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			baseline_set_id INTEGER NOT NULL REFERENCES baseline_sets(id),
			subject TEXT NOT NULL,
			full_mark REAL NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			UNIQUE(baseline_set_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS baseline_scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			baseline_set_id INTEGER NOT NULL REFERENCES baseline_sets(id),
			student_id INTEGER NOT NULL REFERENCES students(id),
			subject TEXT NOT NULL,
			score REAL,
			flag TEXT NOT NULL DEFAULT '',
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(baseline_set_id, student_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS baseline_aggregates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			baseline_set_id INTEGER NOT NULL REFERENCES baseline_sets(id),
			stage_id INTEGER NOT NULL REFERENCES cohort_stages(id),
			scope_type TEXT NOT NULL,
			scope_id INTEGER NOT NULL,
			subject TEXT NOT NULL,
			average REAL NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			ratio REAL NOT NULL DEFAULT 0,
			import_run_id INTEGER,
			source_sheet TEXT NOT NULL DEFAULT '',
			source_row INTEGER NOT NULL DEFAULT 0,
			UNIQUE(baseline_set_id, stage_id, scope_type, scope_id, subject)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope_type TEXT NOT NULL,
			scope_id INTEGER NOT NULL,
			data_kind TEXT NOT NULL DEFAULT '',
			file_hash TEXT NOT NULL,
			stored_path TEXT NOT NULL DEFAULT '',
			relative_path TEXT NOT NULL DEFAULT '',
			converted_path TEXT NOT NULL DEFAULT '',
			mapping_json TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'imported',
			row_count INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			warning_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			UNIQUE(scope_type, scope_id, data_kind, file_hash)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_sheets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_run_id INTEGER NOT NULL REFERENCES import_runs(id) ON DELETE CASCADE,
			sheet_name TEXT NOT NULL,
			role TEXT NOT NULL,
			header_row INTEGER NOT NULL DEFAULT 0,
			data_range TEXT NOT NULL DEFAULT '',
			version_tag TEXT NOT NULL DEFAULT '',
			is_current INTEGER NOT NULL DEFAULT 1,
			UNIQUE(import_run_id, sheet_name)
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
		{SQL: `CREATE TABLE IF NOT EXISTS import_issues (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_run_id INTEGER NOT NULL REFERENCES import_runs(id) ON DELETE CASCADE,
			severity TEXT NOT NULL,
			row_number INTEGER NOT NULL DEFAULT 0,
			column_name TEXT NOT NULL DEFAULT '',
			code TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL,
			resolved INTEGER NOT NULL DEFAULT 0
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS import_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			import_run_id INTEGER NOT NULL REFERENCES import_runs(id) ON DELETE CASCADE,
			entity TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			action TEXT NOT NULL,
			before_json TEXT NOT NULL DEFAULT '{}',
			after_json TEXT NOT NULL DEFAULT '{}'
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS data_revisions (
			scope_type TEXT NOT NULL,
			scope_id INTEGER NOT NULL,
			revision INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(scope_type, scope_id)
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			entity TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			operator TEXT NOT NULL DEFAULT ''
		)`},
	}
}

var defaultSubjects = []Subject{
	{Key: "chinese", Name: "语文", FullMark: 150, Sort: 1, Enabled: true},
	{Key: "math", Name: "数学", FullMark: 150, Sort: 2, Enabled: true},
	{Key: "english", Name: "英语", FullMark: 150, Sort: 3, Enabled: true},
	{Key: "physics", Name: "物理", FullMark: 100, Sort: 4, Enabled: true},
	{Key: "chemistry", Name: "化学", FullMark: 100, Sort: 5, Enabled: true},
	{Key: "biology", Name: "生物", FullMark: 100, Sort: 6, Enabled: true},
	{Key: "history", Name: "历史", FullMark: 100, Sort: 7, Enabled: true},
	{Key: "geography", Name: "地理", FullMark: 100, Sort: 8, Enabled: true},
	{Key: "politics", Name: "政治", FullMark: 100, Sort: 9, Enabled: true},
}

func seedSchoolData(statements []sqlite.Statement) []sqlite.Statement {
	for _, subject := range defaultSubjects {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT OR IGNORE INTO subjects("key", name, full_mark, sort_order, enabled)
				VALUES (?, ?, ?, ?, ?)`,
			Args: []any{subject.Key, subject.Name, subject.FullMark, subject.Sort, boolInt(subject.Enabled)},
		})
	}
	tracks := []Track{
		{Key: "physics", Name: "物理类", Sort: 1, Enabled: true},
		{Key: "history", Name: "历史类", Sort: 2, Enabled: true},
	}
	for _, track := range tracks {
		statements = append(statements, sqlite.Statement{
			SQL:  `INSERT OR IGNORE INTO tracks("key", name, sort_order, enabled) VALUES (?, ?, ?, ?)`,
			Args: []any{track.Key, track.Name, track.Sort, boolInt(track.Enabled)},
		})
	}
	levels := []GradeLevel{
		{Key: "g1", Name: "高一", Sort: 1},
		{Key: "g2", Name: "高二", Sort: 2},
		{Key: "g3", Name: "高三", Sort: 3},
	}
	for _, level := range levels {
		statements = append(statements, sqlite.Statement{
			SQL:  `INSERT OR IGNORE INTO grade_levels("key", name, sort_order) VALUES (?, ?, ?)`,
			Args: []any{level.Key, level.Name, level.Sort},
		})
	}
	return statements
}

// ListSubjects 返回全部学科。
func (s *Store) ListSubjects(ctx context.Context) ([]Subject, error) {
	rows, err := s.db.Query(ctx, `SELECT "key", name, full_mark, sort_order, enabled FROM subjects
		ORDER BY sort_order, "key"`)
	if err != nil {
		return nil, err
	}
	subjects := make([]Subject, 0, len(rows.Values))
	for _, row := range rows.Values {
		key, _ := row[0].(string)
		name, _ := row[1].(string)
		mark, _ := row[2].(float64)
		order, _ := row[3].(int64)
		enabled, _ := row[4].(int64)
		subjects = append(subjects, Subject{
			Key: key, Name: name, FullMark: mark, Sort: int(order), Enabled: enabled != 0,
		})
	}
	return subjects, nil
}

// ListTracks 返回全部选科类别。
func (s *Store) ListTracks(ctx context.Context) ([]Track, error) {
	rows, err := s.db.Query(ctx, `SELECT "key", name, sort_order, enabled FROM tracks ORDER BY sort_order, "key"`)
	if err != nil {
		return nil, err
	}
	tracks := make([]Track, 0, len(rows.Values))
	for _, row := range rows.Values {
		key, _ := row[0].(string)
		name, _ := row[1].(string)
		order, _ := row[2].(int64)
		enabled, _ := row[3].(int64)
		tracks = append(tracks, Track{Key: key, Name: name, Sort: int(order), Enabled: enabled != 0})
	}
	return tracks, nil
}

// ListGradeLevels 返回阶段名称。
func (s *Store) ListGradeLevels(ctx context.Context) ([]GradeLevel, error) {
	rows, err := s.db.Query(ctx, `SELECT "key", name, sort_order FROM grade_levels ORDER BY sort_order, "key"`)
	if err != nil {
		return nil, err
	}
	levels := make([]GradeLevel, 0, len(rows.Values))
	for _, row := range rows.Values {
		key, _ := row[0].(string)
		name, _ := row[1].(string)
		order, _ := row[2].(int64)
		levels = append(levels, GradeLevel{Key: key, Name: name, Sort: int(order)})
	}
	return levels, nil
}

// UpdateSubject 修改学科名称、满分、排序与启用状态。
func (s *Store) UpdateSubject(ctx context.Context, subject Subject) error {
	if subject.Key == "" || subject.Name == "" {
		return fmt.Errorf("学科代码和名称不能为空")
	}
	result, err := s.db.Execute(ctx, `UPDATE subjects SET name = ?, full_mark = ?, enabled = ? WHERE "key" = ?`,
		subject.Name, subject.FullMark, boolInt(subject.Enabled), subject.Key)
	if err != nil {
		return err
	}
	if result.Changes == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateTrack 修改选科类别名称与启用状态。
func (s *Store) UpdateTrack(ctx context.Context, track Track) error {
	if track.Key == "" || track.Name == "" {
		return fmt.Errorf("选科类别代码和名称不能为空")
	}
	result, err := s.db.Execute(ctx, `UPDATE tracks SET name = ?, enabled = ? WHERE "key" = ?`,
		track.Name, boolInt(track.Enabled), track.Key)
	if err != nil {
		return err
	}
	if result.Changes == 0 {
		return ErrNotFound
	}
	return nil
}

// ListClasses 返回当前阶段的行政班与人数。
func (s *Store) ListClasses(ctx context.Context, cohortID, stageID int64) ([]SchoolClass, error) {
	rows, err := s.db.Query(ctx, `SELECT c.id, c.cohort_id, c.name,
		COALESCE((SELECT COUNT(*) FROM student_stage_memberships m
			WHERE m.class_id = c.id AND m.stage_id = ?), 0),
		COALESCE((SELECT t.name FROM teachers t
			WHERE t.id = (SELECT head_teacher_id FROM class_stage_info i
				WHERE i.class_id = c.id AND i.stage_id = ?)), '')
		FROM classes c
		JOIN class_stage_info i ON i.class_id = c.id AND i.stage_id = ?
		WHERE c.cohort_id = ? ORDER BY c.name`, stageID, stageID, stageID, cohortID)
	if err != nil {
		return nil, err
	}
	classes := make([]SchoolClass, 0, len(rows.Values))
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		cohort, _ := row[1].(int64)
		name, _ := row[2].(string)
		students, _ := row[3].(int64)
		head, _ := row[4].(string)
		classes = append(classes, SchoolClass{
			ID: id, CohortID: cohort, Name: name, StageID: stageID,
			Students: int(students), HeadTeacher: head,
		})
	}
	return classes, nil
}

// ListStudents 按阶段、班级、选科、状态和关键词查询学生。
func (s *Store) ListStudents(ctx context.Context, filter StudentFilter) ([]DataStudent, int, error) {
	if filter.StageID == 0 {
		return nil, 0, nil
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 200
	}
	if filter.Page < 0 {
		filter.Page = 0
	}
	where := []string{"m.stage_id = ?"}
	args := []any{filter.StageID}
	if filter.ClassID != 0 {
		where = append(where, "m.class_id = ?")
		args = append(args, filter.ClassID)
	}
	if filter.Track != "" {
		where = append(where, "m.track = ?")
		args = append(args, filter.Track)
	}
	if filter.Status != "" {
		where = append(where, "m.status = ?")
		args = append(args, filter.Status)
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		where = append(where, "(st.student_no LIKE ? OR st.name LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like)
	}
	condition := strings.Join(where, " AND ")
	countRows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM student_stage_memberships m
		JOIN students st ON st.id = m.student_id WHERE `+condition, args...)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	if len(countRows.Values) > 0 {
		value, _ := countRows.Values[0][0].(int64)
		total = int(value)
	}
	queryArgs := append(append([]any{}, args...), filter.PageSize, filter.Page*filter.PageSize)
	rows, err := s.db.Query(ctx, `SELECT st.id, st.student_no, st.name, c.cohort_id, m.stage_id,
		m.class_id, c.name, m.track, m.elective, m.status, m.walking, m.japanese, m.total_score,
		COALESCE(m.import_run_id, 0), m.source_sheet, m.source_row
		FROM student_stage_memberships m
		JOIN students st ON st.id = m.student_id
		JOIN classes c ON c.id = m.class_id
		WHERE `+condition+` ORDER BY st.student_no LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	students := make([]DataStudent, 0, len(rows.Values))
	for _, row := range rows.Values {
		student, err := dataStudentFromRow(row)
		if err != nil {
			return nil, 0, err
		}
		students = append(students, student)
	}
	if len(students) > 0 {
		ids := make([]any, len(students))
		index := map[int64]int{}
		for i, student := range students {
			ids[i] = student.ID
			index[student.ID] = i
		}
		groupRows, err := s.db.Query(ctx, `SELECT stg.student_id, g.subject, g.name
			FROM student_teaching_groups stg JOIN teaching_groups g ON g.id = stg.teaching_group_id
			WHERE stg.stage_id = ? AND stg.student_id IN (`+placeholders(len(ids))+`)
			ORDER BY g.subject, g.name`, append([]any{filter.StageID}, ids...)...)
		if err != nil {
			return nil, 0, err
		}
		for _, row := range groupRows.Values {
			id, _ := row[0].(int64)
			subject, _ := row[1].(string)
			name, _ := row[2].(string)
			if at, ok := index[id]; ok {
				students[at].TeachingGroups = append(students[at].TeachingGroups, subject+":"+name)
			}
		}
	}
	return students, total, nil
}

func dataStudentFromRow(row []any) (DataStudent, error) {
	id, _ := row[0].(int64)
	studentNo, _ := row[1].(string)
	name, _ := row[2].(string)
	cohortID, _ := row[3].(int64)
	stageID, _ := row[4].(int64)
	classID, _ := row[5].(int64)
	className, _ := row[6].(string)
	track, _ := row[7].(string)
	elective, _ := row[8].(string)
	status, _ := row[9].(string)
	walking, _ := row[10].(int64)
	japanese, _ := row[11].(int64)
	total, err := nullableFloatValue(row[12])
	if err != nil {
		return DataStudent{}, err
	}
	runID, _ := row[13].(int64)
	sheet, _ := row[14].(string)
	sourceRow, _ := row[15].(int64)
	return DataStudent{
		ID: id, StudentNo: studentNo, Name: name, CohortID: cohortID,
		StageID: stageID, ClassID: classID, ClassName: className, Track: track,
		Elective: elective, Status: status, Walking: walking != 0, Japanese: japanese != 0,
		TotalScore: total, ImportRunID: runID, SourceSheet: sheet, SourceRow: int(sourceRow),
	}, nil
}

// GetStudent 读取一名学生在某个阶段的完整数据。
func (s *Store) GetStudent(ctx context.Context, stageID, studentID int64) (DataStudent, error) {
	rows, err := s.db.Query(ctx, `SELECT st.id, st.student_no, st.name, c.cohort_id, m.stage_id,
		m.class_id, c.name, m.track, m.elective, m.status, m.walking, m.japanese, m.total_score,
		COALESCE(m.import_run_id, 0), m.source_sheet, m.source_row
		FROM student_stage_memberships m
		JOIN students st ON st.id = m.student_id
		JOIN classes c ON c.id = m.class_id
		WHERE m.stage_id = ? AND m.student_id = ?`, stageID, studentID)
	if err != nil {
		return DataStudent{}, err
	}
	if len(rows.Values) == 0 {
		return DataStudent{}, ErrNotFound
	}
	student, err := dataStudentFromRow(rows.Values[0])
	if err != nil {
		return DataStudent{}, err
	}
	groups, err := s.db.Query(ctx, `SELECT g.subject, g.name FROM student_teaching_groups stg
		JOIN teaching_groups g ON g.id = stg.teaching_group_id
		WHERE stg.stage_id = ? AND stg.student_id = ? ORDER BY g.subject, g.name`,
		stageID, studentID)
	if err != nil {
		return DataStudent{}, err
	}
	for _, row := range groups.Values {
		subject, _ := row[0].(string)
		name, _ := row[1].(string)
		student.TeachingGroups = append(student.TeachingGroups, subject+":"+name)
	}
	return student, nil
}

// ListTeachers 返回教师与任课数量。
func (s *Store) ListTeachers(ctx context.Context, stageID int64, query string) ([]DataTeacher, error) {
	where := ""
	args := []any{}
	if stageID != 0 {
		where = ` WHERE EXISTS (SELECT 1 FROM assignments a
			JOIN teaching_groups g ON g.id = a.teaching_group_id
			WHERE a.teacher_id = t.id AND g.stage_id = ?)`
		args = append(args, stageID)
	}
	if text := strings.TrimSpace(query); text != "" {
		clause := "(t.name LIKE ? OR t.teacher_no LIKE ? OR t.subject LIKE ?)"
		if where == "" {
			where = " WHERE " + clause
		} else {
			where += " AND " + clause
		}
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}
	rows, err := s.db.Query(ctx, `SELECT t.id, t.teacher_no, t.name, t.subject,
		(SELECT COUNT(DISTINCT a.teaching_group_id) FROM assignments a
			WHERE a.teacher_id = t.id AND (? = 0 OR EXISTS (
				SELECT 1 FROM teaching_groups g WHERE g.id = a.teaching_group_id AND g.stage_id = ?))),
		(SELECT COUNT(*) FROM assignments a
			WHERE a.teacher_id = t.id AND (? = 0 OR EXISTS (
				SELECT 1 FROM teaching_groups g WHERE g.id = a.teaching_group_id AND g.stage_id = ?))),
		COALESCE(t.import_run_id, 0), t.source_sheet, t.source_row
		FROM teachers t`+where+` ORDER BY t.name`, append([]any{stageID, stageID, stageID, stageID}, args...)...)
	if err != nil {
		return nil, err
	}
	teachers := make([]DataTeacher, 0, len(rows.Values))
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		number, _ := row[1].(string)
		name, _ := row[2].(string)
		subject, _ := row[3].(string)
		groups, _ := row[4].(int64)
		assignments, _ := row[5].(int64)
		runID, _ := row[6].(int64)
		sheet, _ := row[7].(string)
		sourceRow, _ := row[8].(int64)
		teachers = append(teachers, DataTeacher{
			ID: id, TeacherNo: number, Name: name, Subject: subject,
			Groups: int(groups), Assignments: int(assignments),
			ImportRunID: runID, SourceSheet: sheet, SourceRow: int(sourceRow),
		})
	}
	return teachers, nil
}

// ListTeachingGroups 返回阶段中的教学班。
func (s *Store) ListTeachingGroups(ctx context.Context, stageID int64, query string) ([]DataTeachingGroup, error) {
	args := []any{stageID}
	where := " WHERE g.stage_id = ?"
	if text := strings.TrimSpace(query); text != "" {
		where += " AND (g.name LIKE ? OR g.subject LIKE ?)"
		like := "%" + text + "%"
		args = append(args, like, like)
	}
	rows, err := s.db.Query(ctx, `SELECT g.id, g.stage_id, g.subject, g.name, g.kind,
		(SELECT COUNT(*) FROM student_teaching_groups stg WHERE stg.teaching_group_id = g.id),
		COALESCE((SELECT GROUP_CONCAT(t.name, '、') FROM assignments a
			JOIN teachers t ON t.id = a.teacher_id WHERE a.teaching_group_id = g.id), ''),
		COALESCE((SELECT GROUP_CONCAT(DISTINCT c.name) FROM student_teaching_groups stg
			JOIN student_stage_memberships m ON m.student_id = stg.student_id AND m.stage_id = stg.stage_id
			JOIN classes c ON c.id = m.class_id WHERE stg.teaching_group_id = g.id), ''),
		COALESCE(g.import_run_id, 0), g.source_sheet, g.source_row
		FROM teaching_groups g`+where+` ORDER BY g.subject, g.name`, args...)
	if err != nil {
		return nil, err
	}
	groups := make([]DataTeachingGroup, 0, len(rows.Values))
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		stage, _ := row[1].(int64)
		subject, _ := row[2].(string)
		name, _ := row[3].(string)
		kind, _ := row[4].(string)
		students, _ := row[5].(int64)
		teachers, _ := row[6].(string)
		classes, _ := row[7].(string)
		runID, _ := row[8].(int64)
		sheet, _ := row[9].(string)
		sourceRow, _ := row[10].(int64)
		groups = append(groups, DataTeachingGroup{
			ID: id, StageID: stage, Subject: subject, Name: name, Kind: kind,
			Students: int(students), Teachers: teachers, ClassNames: classes,
			ImportRunID: runID, SourceSheet: sheet, SourceRow: int(sourceRow),
		})
	}
	return groups, nil
}

// ListBaselines 返回阶段的入口成绩汇总。
func (s *Store) ListBaselines(ctx context.Context, stageID, classID int64, query string) ([]BaselineRow, error) {
	where := "a.stage_id = ? AND a.scope_type = 'class'"
	args := []any{stageID}
	if classID != 0 {
		where += " AND a.scope_id = ?"
		args = append(args, classID)
	}
	if text := strings.TrimSpace(query); text != "" {
		where += " AND (c.name LIKE ? OR a.subject LIKE ? OR b.name LIKE ?)"
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}
	rows, err := s.db.Query(ctx, `SELECT b.id, b.name, a.scope_id, c.name, a.subject, a.average, a.count,
		a.ratio, COALESCE(a.import_run_id, 0), a.source_sheet, a.source_row
		FROM baseline_aggregates a
		JOIN baseline_sets b ON b.id = a.baseline_set_id
		LEFT JOIN classes c ON c.id = a.scope_id
		WHERE `+where+`
		ORDER BY c.name, a.subject`, args...)
	if err != nil {
		return nil, err
	}
	items := make([]BaselineRow, 0, len(rows.Values))
	for _, row := range rows.Values {
		setID, _ := row[0].(int64)
		setName, _ := row[1].(string)
		classID, _ := row[2].(int64)
		className, _ := row[3].(string)
		subject, _ := row[4].(string)
		average, _ := row[5].(float64)
		count, _ := row[6].(int64)
		ratio, _ := row[7].(float64)
		runID, _ := row[8].(int64)
		sheet, _ := row[9].(string)
		sourceRow, _ := row[10].(int64)
		items = append(items, BaselineRow{
			BaselineSetID: setID, SetName: setName, ClassID: classID,
			ClassName: className, Subject: subject, Average: average,
			Count: int(count), Ratio: ratio,
			ImportRunID: runID, SourceSheet: sheet, SourceRow: int(sourceRow),
		})
	}
	return items, nil
}

// ListImportRuns 返回某个级或阶段的导入记录。
func (s *Store) ListImportRuns(ctx context.Context, stageID int64) ([]ImportRun, error) {
	rows, err := s.db.Query(ctx, `SELECT id, scope_type, scope_id, data_kind, file_hash,
		stored_path, relative_path, status, row_count, warning_count, created_at
		FROM import_runs WHERE scope_type = 'stage' AND scope_id = ?
		ORDER BY id DESC`, stageID)
	if err != nil {
		return nil, err
	}
	runs := make([]ImportRun, 0, len(rows.Values))
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		scopeType, _ := row[1].(string)
		scopeID, _ := row[2].(int64)
		kind, _ := row[3].(string)
		hash, _ := row[4].(string)
		stored, _ := row[5].(string)
		relative, _ := row[6].(string)
		status, _ := row[7].(string)
		rowCount, _ := row[8].(int64)
		warnings, _ := row[9].(int64)
		createdAt, err := parseTime(row[10])
		if err != nil {
			return nil, err
		}
		runs = append(runs, ImportRun{
			ID: id, ScopeType: scopeType, ScopeID: scopeID, DataKind: kind,
			FileHash: hash, StoredPath: stored, RelativePath: relative,
			Status: status, RowCount: int(rowCount), Warnings: int(warnings),
			CreatedAt: createdAt,
		})
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

// ListExamImportRuns 返回一场考试中某个级的导入记录。
func (s *Store) ListExamImportRuns(ctx context.Context, examCohortID int64) ([]ImportRun, error) {
	rows, err := s.db.Query(ctx, `SELECT id, scope_type, scope_id, data_kind, file_hash,
		stored_path, relative_path, status, row_count, warning_count, created_at
		FROM import_runs WHERE scope_type = 'exam' AND scope_id = ?
		ORDER BY id DESC`, examCohortID)
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

// ListAllImportRuns 返回基础数据与考试数据的全部导入记录。
func (s *Store) ListAllImportRuns(ctx context.Context) ([]ImportRun, error) {
	rows, err := s.db.Query(ctx, `SELECT id, scope_type, scope_id, data_kind, file_hash,
		stored_path, relative_path, status, row_count, warning_count, created_at
		FROM import_runs ORDER BY id DESC`)
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

func (s *Store) importRunSheets(ctx context.Context, runID int64) ([]ImportSheetInfo, error) {
	rows, err := s.db.Query(ctx, `SELECT sheet_name, role, header_row, data_range, is_current
		FROM import_sheets WHERE import_run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	sheets := make([]ImportSheetInfo, 0, len(rows.Values))
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		role, _ := row[1].(string)
		header, _ := row[2].(int64)
		rangeText, _ := row[3].(string)
		current, _ := row[4].(int64)
		rowCount := 0
		fmt.Sscanf(rangeText, "%d", &rowCount)
		sheets = append(sheets, ImportSheetInfo{
			Name: name, Role: role, HeaderRow: int(header), Rows: rowCount, Current: current != 0,
		})
	}
	return sheets, nil
}

func nullableFloatValue(value any) (*float64, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case float64:
		return &v, nil
	case int64:
		f := float64(v)
		return &f, nil
	default:
		return nil, fmt.Errorf("数值格式不正确")
	}
}

func normalizeClassName(name string) string {
	text := strings.TrimSpace(name)
	replacer := strings.NewReplacer("（", "(", "）", ")", "　", "")
	return replacer.Replace(text)
}
