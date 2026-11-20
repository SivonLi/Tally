package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/school"
)

// ErrCohortExists 表示相同入学年份的级已经存在。
var ErrCohortExists = errors.New("级已经存在")

// Cohort 是一个级及其届。
type Cohort struct {
	ID             int64
	Name           string
	EnrollmentYear int
	GraduationYear int
	Status         string
	CreatedAt      time.Time
}

// CohortStage 是级在一个学年中的阶段记录。
type CohortStage struct {
	ID                int64
	CohortID          int64
	AcademicYearStart int
	GradeLevel        string
	CopiedFromStageID *int64
	ReviewStatus      string
	ConfirmedAt       *time.Time
}

// StageCounts 是一个级在一个阶段中的基础数据数量。
type StageCounts struct {
	Classes        int
	Students       int
	Teachers       int
	Assignments    int
	TeachingGroups int
	Baselines      int
}

func cohortSchema() []sqlite.Statement {
	return []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS cohorts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			enrollment_year INTEGER NOT NULL,
			graduation_year INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS cohorts_name ON cohorts(name)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS cohorts_enrollment_year ON cohorts(enrollment_year)`},
		{SQL: `CREATE TABLE IF NOT EXISTS cohort_stages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cohort_id INTEGER NOT NULL REFERENCES cohorts(id),
			academic_year_start INTEGER NOT NULL,
			grade_level TEXT NOT NULL,
			copied_from_stage_id INTEGER,
			review_status TEXT NOT NULL DEFAULT 'confirmed',
			confirmed_at TEXT,
			UNIQUE(cohort_id, academic_year_start),
			UNIQUE(cohort_id, grade_level)
		)`},
		{SQL: `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
			VALUES (3, ?)`, Args: []any{time.Now().UTC().Format(time.RFC3339Nano)}},
	}
}

// deriveLegacyCohorts 从已有批次推导级与阶段，保留旧数据的上下文。
func (s *Store) deriveLegacyCohorts(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT t.academic_year, b.grade FROM batches b
		JOIN terms t ON t.id = b.term_id ORDER BY b.id`)
	if err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	seen := map[[2]int]bool{}
	for _, row := range rows.Values {
		academicYear, _ := row[0].(string)
		grade, _ := row[1].(string)
		start, err := school.ParseAcademicYearStart(academicYear)
		if err != nil {
			continue
		}
		enrollment, err := school.EnrollmentYear(start, grade)
		if err != nil {
			continue
		}
		if seen[[2]int{start, enrollment}] {
			continue
		}
		seen[[2]int{start, enrollment}] = true
		statements = append(statements,
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO cohorts(name, enrollment_year, graduation_year, status, created_at)
					VALUES (?, ?, ?, 'active', ?)`,
				Args: []any{fmt.Sprintf("%d级", enrollment), enrollment, school.GraduationYear(enrollment), now},
			},
			sqlite.Statement{
				SQL: `INSERT OR IGNORE INTO cohort_stages(cohort_id, academic_year_start, grade_level, review_status)
					SELECT id, ?, ?, 'confirmed' FROM cohorts WHERE enrollment_year = ?`,
				Args: []any{start, grade, enrollment},
			},
		)
	}
	if len(statements) == 0 {
		return nil
	}
	_, err = s.db.Transaction(ctx, statements)
	return err
}

// ListCohorts 返回全部级，入学年份新的在前。
func (s *Store) ListCohorts(ctx context.Context) ([]Cohort, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, enrollment_year, graduation_year, status, created_at
		FROM cohorts ORDER BY enrollment_year DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	cohorts := make([]Cohort, 0, len(rows.Values))
	for _, row := range rows.Values {
		cohort, err := cohortFromRow(row)
		if err != nil {
			return nil, err
		}
		cohorts = append(cohorts, cohort)
	}
	return cohorts, nil
}

// GetCohort 按标识读取一个级。
func (s *Store) GetCohort(ctx context.Context, id int64) (Cohort, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, enrollment_year, graduation_year, status, created_at
		FROM cohorts WHERE id = ?`, id)
	if err != nil {
		return Cohort{}, err
	}
	if len(rows.Values) == 0 {
		return Cohort{}, ErrNotFound
	}
	return cohortFromRow(rows.Values[0])
}

// CreateCohort 新建一个级，并为当前学年写入阶段记录。
func (s *Store) CreateCohort(ctx context.Context, enrollmentYear, currentAcademicYearStart int) (Cohort, error) {
	if err := school.ValidateEnrollmentYear(enrollmentYear, currentAcademicYearStart); err != nil {
		return Cohort{}, err
	}
	name := fmt.Sprintf("%d级", enrollmentYear)
	rows, err := s.db.Query(ctx, `SELECT id FROM cohorts WHERE enrollment_year = ?`, enrollmentYear)
	if err != nil {
		return Cohort{}, err
	}
	if len(rows.Values) > 0 {
		return Cohort{}, fmt.Errorf("%w: %s", ErrCohortExists, name)
	}
	result, err := s.db.Execute(ctx, `INSERT INTO cohorts(name, enrollment_year, graduation_year, status, created_at)
		VALUES (?, ?, ?, 'active', ?)`, name, enrollmentYear, school.GraduationYear(enrollmentYear),
		s.now().Format(time.RFC3339Nano))
	if err != nil {
		return Cohort{}, err
	}
	cohort, err := s.GetCohort(ctx, result.LastInsertID)
	if err != nil {
		return Cohort{}, err
	}
	index := school.StageIndex(enrollmentYear, currentAcademicYearStart)
	if index >= school.StageFirst && index <= school.StageThird {
		if _, err := s.EnsureCohortStage(ctx, cohort.ID, currentAcademicYearStart, school.StageKey(index)); err != nil {
			return Cohort{}, err
		}
	}
	return cohort, nil
}

// EnsureCohortStage 写入级在某个学年的阶段记录，已经存在时保持不变。
func (s *Store) EnsureCohortStage(ctx context.Context, cohortID int64, academicYearStart int, gradeLevel string) (CohortStage, error) {
	index, err := school.StageIndexForKey(gradeLevel)
	if err != nil {
		return CohortStage{}, err
	}
	if index < school.StageFirst || index > school.StageThird {
		return CohortStage{}, fmt.Errorf("%w: %s", school.ErrGradeLevel, gradeLevel)
	}
	if _, err := s.GetCohort(ctx, cohortID); err != nil {
		return CohortStage{}, err
	}
	_, err = s.db.Execute(ctx, `INSERT OR IGNORE INTO cohort_stages(
		cohort_id, academic_year_start, grade_level, review_status)
		VALUES (?, ?, ?, 'confirmed')`, cohortID, academicYearStart, gradeLevel)
	if err != nil {
		return CohortStage{}, err
	}
	return s.GetCohortStage(ctx, cohortID, academicYearStart)
}

// GetCohortStage 读取级在某个学年的阶段记录。
func (s *Store) GetCohortStage(ctx context.Context, cohortID int64, academicYearStart int) (CohortStage, error) {
	rows, err := s.db.Query(ctx, `SELECT id, cohort_id, academic_year_start, grade_level,
		copied_from_stage_id, review_status, confirmed_at
		FROM cohort_stages WHERE cohort_id = ? AND academic_year_start = ?`, cohortID, academicYearStart)
	if err != nil {
		return CohortStage{}, err
	}
	if len(rows.Values) == 0 {
		return CohortStage{}, ErrNotFound
	}
	return cohortStageFromRow(rows.Values[0])
}

// ListCohortStages 返回一个级的全部阶段记录，学年早的在前。
func (s *Store) ListCohortStages(ctx context.Context, cohortID int64) ([]CohortStage, error) {
	rows, err := s.db.Query(ctx, `SELECT id, cohort_id, academic_year_start, grade_level,
		copied_from_stage_id, review_status, confirmed_at
		FROM cohort_stages WHERE cohort_id = ? ORDER BY academic_year_start`, cohortID)
	if err != nil {
		return nil, err
	}
	stages := make([]CohortStage, 0, len(rows.Values))
	for _, row := range rows.Values {
		stage, err := cohortStageFromRow(row)
		if err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	return stages, nil
}

// CurrentAcademicYear 读取设置的当前学年，没有设置时使用 fallback。
func (s *Store) CurrentAcademicYear(ctx context.Context, fallback int) (int, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE key = 'current_academic_year_start'`)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return fallback, nil
	}
	text, _ := rows.Values[0][0].(string)
	value, err := strconv.Atoi(text)
	if err != nil {
		return fallback, nil
	}
	return value, nil
}

// SetAcademicYear 保存当前学年。
func (s *Store) SetAcademicYear(ctx context.Context, start int) error {
	if start < 1990 || start > 3000 {
		return fmt.Errorf("%w: %d", school.ErrAcademicYear, start)
	}
	_, err := s.db.Execute(ctx, `INSERT INTO app_settings(key, value) VALUES ('current_academic_year_start', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, strconv.Itoa(start))
	return err
}

// StageCounts 汇总一个级在一个阶段中的基础数据数量。
func (s *Store) StageCounts(ctx context.Context, cohortID int64, academicYearStart int, gradeLevel string) (StageCounts, error) {
	stage, err := s.GetCohortStage(ctx, cohortID, academicYearStart)
	if err != nil {
		return StageCounts{}, err
	}
	count := func(query string, args ...any) (int, error) {
		rows, err := s.db.Query(ctx, query, args...)
		if err != nil {
			return 0, err
		}
		if len(rows.Values) == 0 {
			return 0, nil
		}
		value, _ := rows.Values[0][0].(int64)
		return int(value), nil
	}
	classes, err := count(`SELECT COUNT(*) FROM class_stage_info WHERE stage_id = ?`, stage.ID)
	if err != nil {
		return StageCounts{}, err
	}
	students, err := count(`SELECT COUNT(*) FROM student_stage_memberships WHERE stage_id = ?`, stage.ID)
	if err != nil {
		return StageCounts{}, err
	}
	teachers, err := count(`SELECT COUNT(DISTINCT a.teacher_id) FROM assignments a
		JOIN teaching_groups g ON g.id = a.teaching_group_id WHERE g.stage_id = ?`, stage.ID)
	if err != nil {
		return StageCounts{}, err
	}
	assignments, err := count(`SELECT COUNT(*) FROM assignments a
		JOIN teaching_groups g ON g.id = a.teaching_group_id WHERE g.stage_id = ?`, stage.ID)
	if err != nil {
		return StageCounts{}, err
	}
	groups, err := count(`SELECT COUNT(*) FROM teaching_groups WHERE stage_id = ?`, stage.ID)
	if err != nil {
		return StageCounts{}, err
	}
	baselines, err := count(`SELECT COUNT(*) FROM baseline_aggregates WHERE stage_id = ?`, stage.ID)
	if err != nil {
		return StageCounts{}, err
	}
	return StageCounts{
		Classes:        classes,
		Students:       students,
		Teachers:       teachers,
		Assignments:    assignments,
		TeachingGroups: groups,
		Baselines:      baselines,
	}, nil
}

// StageClassNames 返回一个级在一个阶段中的行政班名称。
func (s *Store) StageClassNames(ctx context.Context, cohortID int64, academicYearStart int, gradeLevel string) ([]string, error) {
	stage, err := s.GetCohortStage(ctx, cohortID, academicYearStart)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT c.name FROM classes c
		JOIN class_stage_info i ON i.class_id = c.id
		WHERE i.stage_id = ? ORDER BY c.name`, stage.ID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows.Values))
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func cohortFromRow(row []any) (Cohort, error) {
	id, ok := row[0].(int64)
	if !ok {
		return Cohort{}, fmt.Errorf("级标识格式不正确")
	}
	name, _ := row[1].(string)
	enrollment, _ := row[2].(int64)
	graduation, _ := row[3].(int64)
	status, _ := row[4].(string)
	createdAt, err := parseTime(row[5])
	if err != nil {
		return Cohort{}, err
	}
	return Cohort{
		ID:             id,
		Name:           name,
		EnrollmentYear: int(enrollment),
		GraduationYear: int(graduation),
		Status:         status,
		CreatedAt:      createdAt,
	}, nil
}

func cohortStageFromRow(row []any) (CohortStage, error) {
	id, ok := row[0].(int64)
	if !ok {
		return CohortStage{}, fmt.Errorf("阶段标识格式不正确")
	}
	cohortID, _ := row[1].(int64)
	start, _ := row[2].(int64)
	grade, _ := row[3].(string)
	var copiedFrom *int64
	if value, ok := nullableInt64(row[4]); ok {
		copiedFrom = &value
	}
	review, _ := row[5].(string)
	confirmedAt, err := nullableTime(row[6])
	if err != nil {
		return CohortStage{}, err
	}
	return CohortStage{
		ID:                id,
		CohortID:          cohortID,
		AcademicYearStart: int(start),
		GradeLevel:        grade,
		CopiedFromStageID: copiedFrom,
		ReviewStatus:      review,
		ConfirmedAt:       confirmedAt,
	}, nil
}
