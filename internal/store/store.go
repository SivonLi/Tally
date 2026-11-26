// Package store 把考核方案、年级组织、师生档案、考试成绩与核算结果保存在 SQLite 中。
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/assessment"
	"tally/internal/school"
)

// SchemaVersion 是 Tally v2 单一真理源数据库结构的当前版本。
const SchemaVersion = 1

var (
	// ErrNotFound 表示请求的记录不存在。
	ErrNotFound = errors.New("记录不存在")
	// ErrIncompatibleDatabase 表示当前数据库属于旧版或不兼容版本，需新建干净库。
	ErrIncompatibleDatabase = errors.New("检测到不兼容的旧版数据库结构，请新建或重置数据库")
	// ErrCohortExists 表示同一个入学年份的级已经存在。
	ErrCohortExists = errors.New("这个入学年份的级已经存在")
	// ErrCohortGraduated 表示已毕业归档的年级不允许再执行变更操作。
	ErrCohortGraduated = errors.New("该年级已毕业封存，不能继续变更")
	// ErrInvalidPhaseTransition 表示升学目标阶段不合法。
	ErrInvalidPhaseTransition = errors.New("不合法的年级阶段流转")
	// ErrSystemPlanRetire 表示内置默认考核方案不能停用。
	ErrSystemPlanRetire = errors.New("系统默认考核方案不能停用")
	// ErrPlanNameEmpty 表示方案名称不能为空。
	ErrPlanNameEmpty = errors.New("方案名称不能为空")
	// ErrExamExists 表示同一学年、学期、类型和名称的考试已经存在。
	ErrExamExists = errors.New("考试已经存在")
	// ErrExamCohort 表示考试没有指定的级。
	ErrExamCohort = errors.New("考试没有这个级")
	// ErrExamTrackMissing 表示报名名单无法确定选科类别。
	ErrExamTrackMissing = errors.New("报名名单缺少选科类别")
	// ErrDuplicateImport 表示同一个文件已经导入过同一类数据。
	ErrDuplicateImport = errors.New("该文件已经导入过这类数据")
	// ErrImportBlocked 表示导入存在阻断问题，不能写入。
	ErrImportBlocked = errors.New("导入存在阻断问题")
	// ErrImportUndoBlocked 表示导入之后还有新的导入，撤销会覆盖后续修改。
	ErrImportUndoBlocked = errors.New("此导入之后还有新的导入，不能撤销")
	// ErrClassNotEmpty 表示班级仍有关联学生或考试快照，不能删除。
	ErrClassNotEmpty = errors.New("班级下已有学生或考试记录，不能直接删除")
	// ErrTeacherInUse 表示教师仍在任课、担任班主任或已有历史得分，不能删除。
	ErrTeacherInUse = errors.New("教师已有任课、班主任或历史量化记录，建议改为停用")
	// ErrStudentMerge 表示学生合并参数无效。
	ErrStudentMerge = errors.New("无法合并指定的学生档案")
)

const (
	academicYearSettingKey = "academic_year_start"
	sofficePathSettingKey  = "soffice_path"
	systemPlanName         = "教学成绩量化考核方案"
)

// Store 封装学校数据、考试成绩、考核方案与核算结果的 SQLite 存储。
type Store struct {
	db      *sqlite.DB
	rootDir string
	now     func() time.Time
}

// Open 打开或初始化 Tally SQLite 数据库。检测到旧版或不兼容库结构时直接返回 ErrIncompatibleDatabase。
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("数据库路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	s := &Store{
		db:      db,
		rootDir: filepath.Dir(path),
		now:     func() time.Time { return time.Now().UTC() },
	}
	if _, err := db.Execute(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("启用外键约束: %w (关闭失败: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("启用外键约束: %w", err)
	}
	if err := s.initOrValidateSchema(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w (关闭数据库失败: %v)", err, closeErr)
		}
		return nil, err
	}
	if _, err := s.EnsureDefaultPlan(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("初始化默认方案: %w (关闭失败: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("初始化默认方案: %w", err)
	}
	return s, nil
}

// Close 关闭底层数据库连接。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) initOrValidateSchema(ctx context.Context) error {
	tableRows, err := s.db.Query(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return fmt.Errorf("检查数据库表结构: %w", err)
	}
	tables := make(map[string]bool, len(tableRows.Values))
	for _, row := range tableRows.Values {
		tables[stringValue(row[0])] = true
	}

	// 检测到任何旧版核心表或非干净旧库时直接拒绝打开，不做旧库迁移。
	for _, legacyTable := range []string{
		"batches", "terms", "imports", "scores", "cohort_stages",
		"class_stage_info", "student_cohorts", "student_stage_memberships",
		"student_teaching_groups", "baseline_sets", "baseline_aggregates",
		"exam_rosters", "exam_subjects", "sync_settings", "exports",
	} {
		if tables[legacyTable] {
			return fmt.Errorf("%w: 发现旧版表 %s，请新建干净数据库", ErrIncompatibleDatabase, legacyTable)
		}
	}

	if tables["schema_migrations"] {
		verRows, err := s.db.Query(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`)
		if err != nil {
			return fmt.Errorf("读取数据库版本: %w", err)
		}
		current := 0
		if len(verRows.Values) > 0 {
			current = int(intValue(verRows.Values[0][0]))
		}
		if current != SchemaVersion {
			return fmt.Errorf("%w: 数据库版本为 v%d，当前仅支持全新 v%d，请新建干净数据库",
				ErrIncompatibleDatabase, current, SchemaVersion)
		}
		// Schema v1 的建表语句全部带 IF NOT EXISTS，同版本数据库重复执行只会补齐后续
		// 追加的表与索引，已有数据保持不变。
		if _, err := s.db.Transaction(ctx, schemaV1Statements()); err != nil {
			return fmt.Errorf("补齐 Schema v1 结构: %w", err)
		}
		return s.seedDefaults(ctx)
	}

	if len(tables) > 0 {
		return fmt.Errorf("%w: 检测到未知数据库结构，请新建干净数据库", ErrIncompatibleDatabase)
	}

	now := s.now().Format(time.RFC3339Nano)
	statements := append([]sqlite.Statement(nil), schemaV1Statements()...)
	statements = append(statements, sqlite.Statement{
		SQL:  `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
		Args: []any{SchemaVersion, now},
	})
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return fmt.Errorf("初始化 Schema v1: %w", err)
	}
	return s.seedDefaults(ctx)
}

func (s *Store) seedDefaults(ctx context.Context) error {
	now := s.now().Format(time.RFC3339Nano)
	defaultSubjects := []Subject{
		{Key: "chinese", Name: "语文", FullMark: 150, Sort: 1, Enabled: true},
		{Key: "math", Name: "数学", FullMark: 150, Sort: 2, Enabled: true},
		{Key: "english", Name: "英语", FullMark: 150, Sort: 3, Enabled: true},
		{Key: "japanese", Name: "日语", FullMark: 150, Sort: 4, Enabled: true},
		{Key: "physics", Name: "物理", FullMark: 100, Sort: 5, Enabled: true},
		{Key: "chemistry", Name: "化学", FullMark: 100, Sort: 6, Enabled: true},
		{Key: "biology", Name: "生物", FullMark: 100, Sort: 7, Enabled: true},
		{Key: "history", Name: "历史", FullMark: 100, Sort: 8, Enabled: true},
		{Key: "politics", Name: "政治", FullMark: 100, Sort: 9, Enabled: true},
		{Key: "geography", Name: "地理", FullMark: 100, Sort: 10, Enabled: true},
	}
	defaultTracks := []Track{
		{Key: "physics", Name: "物理类", Sort: 1, Enabled: true},
		{Key: "history", Name: "历史类", Sort: 2, Enabled: true},
	}
	var statements []sqlite.Statement
	for _, subject := range defaultSubjects {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO subjects("key", name, full_mark, sort_order, enabled)
				VALUES (?, ?, ?, ?, ?) ON CONFLICT("key") DO NOTHING`,
			Args: []any{subject.Key, subject.Name, subject.FullMark, subject.Sort, boolInt(subject.Enabled)},
		})
	}
	for _, track := range defaultTracks {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO tracks("key", name, sort_order, enabled)
				VALUES (?, ?, ?, ?) ON CONFLICT("key") DO NOTHING`,
			Args: []any{track.Key, track.Name, track.Sort, boolInt(track.Enabled)},
		})
	}
	_ = now
	_, err := s.db.Transaction(ctx, statements)
	return err
}

// UpdateSubject 更新单个学科的名称、满分、排序与启用状态。
func (s *Store) UpdateSubject(ctx context.Context, subject Subject) error {
	key := strings.TrimSpace(subject.Key)
	name := strings.TrimSpace(subject.Name)
	if key == "" || name == "" {
		return fmt.Errorf("学科标识与名称不能为空")
	}
	if subject.FullMark <= 0 {
		return fmt.Errorf("学科满分必须大于零")
	}
	_, err := s.db.Execute(ctx, `INSERT INTO subjects("key", name, full_mark, sort_order, enabled)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT("key") DO UPDATE SET
			name = excluded.name,
			full_mark = excluded.full_mark,
			sort_order = excluded.sort_order,
			enabled = excluded.enabled`,
		key, name, subject.FullMark, subject.Sort, boolInt(subject.Enabled))
	return err
}

// UpdateTrack 更新单个选科类别的名称、排序与启用状态。
func (s *Store) UpdateTrack(ctx context.Context, track Track) error {
	key := strings.TrimSpace(track.Key)
	name := strings.TrimSpace(track.Name)
	if key == "" || name == "" {
		return fmt.Errorf("选科标识与名称不能为空")
	}
	_, err := s.db.Execute(ctx, `INSERT INTO tracks("key", name, sort_order, enabled)
		VALUES (?, ?, ?, ?)
		ON CONFLICT("key") DO UPDATE SET
			name = excluded.name,
			sort_order = excluded.sort_order,
			enabled = excluded.enabled`,
		key, name, track.Sort, boolInt(track.Enabled))
	return err
}

// CurrentAcademicYear 返回设置里的当前学年起始年份；尚未配置时返回 fallback。
func (s *Store) CurrentAcademicYear(ctx context.Context, fallback int) (int, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE "key" = ?`, academicYearSettingKey)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		if fallback > 0 {
			return fallback, nil
		}
		return school.AcademicYearStart(s.now()), nil
	}
	year, err := strconv.Atoi(stringValue(rows.Values[0][0]))
	if err != nil {
		return 0, fmt.Errorf("当前学年设置格式不正确: %w", err)
	}
	return year, nil
}

// AcademicYear 返回设置里的当前学年起始年份，没有设置时按当前时间推导。
func (s *Store) AcademicYear(ctx context.Context) (int, error) {
	return s.CurrentAcademicYear(ctx, school.AcademicYearStart(s.now()))
}

// SetAcademicYear 保存当前学年起始年份。
func (s *Store) SetAcademicYear(ctx context.Context, start int) error {
	if start < 1900 || start > 2999 {
		return fmt.Errorf("学年起始年份超出范围")
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `INSERT INTO app_settings("key", value, updated_at)
		VALUES (?, ?, ?) ON CONFLICT("key") DO UPDATE SET value = excluded.value,
		updated_at = excluded.updated_at`, academicYearSettingKey, strconv.Itoa(start), now)
	return err
}

// SofficePath 返回设置的 LibreOffice 可执行文件路径。
func (s *Store) SofficePath(ctx context.Context) (string, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE "key" = ?`, sofficePathSettingKey)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 {
		return "", nil
	}
	return stringValue(rows.Values[0][0]), nil
}

// SetSofficePath 保存 LibreOffice 可执行文件路径。
func (s *Store) SetSofficePath(ctx context.Context, path string) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `INSERT INTO app_settings("key", value, updated_at)
		VALUES (?, ?, ?) ON CONFLICT("key") DO UPDATE SET value = excluded.value,
		updated_at = excluded.updated_at`, sofficePathSettingKey, strings.TrimSpace(path), now)
	return err
}

// ListSubjects 返回学科配置列表。
func (s *Store) ListSubjects(ctx context.Context) ([]Subject, error) {
	rows, err := s.db.Query(ctx, `SELECT "key", name, full_mark, sort_order, enabled FROM subjects ORDER BY sort_order, "key"`)
	if err != nil {
		return nil, err
	}
	result := make([]Subject, 0, len(rows.Values))
	for _, row := range rows.Values {
		result = append(result, Subject{
			Key:      stringValue(row[0]),
			Name:     stringValue(row[1]),
			FullMark: floatValue(row[2]),
			Sort:     int(intValue(row[3])),
			Enabled:  intValue(row[4]) != 0,
		})
	}
	return result, nil
}

// SaveSubjects 更新并保存学科配置。
func (s *Store) SaveSubjects(ctx context.Context, subjects []Subject) error {
	statements := []sqlite.Statement{{SQL: `DELETE FROM subjects`}}
	for index, subject := range subjects {
		key := strings.TrimSpace(subject.Key)
		name := strings.TrimSpace(subject.Name)
		if key == "" || name == "" {
			continue
		}
		fullMark := subject.FullMark
		if fullMark <= 0 {
			fullMark = 100
		}
		sortOrder := subject.Sort
		if sortOrder == 0 {
			sortOrder = index + 1
		}
		statements = append(statements, sqlite.Statement{
			SQL:  `INSERT INTO subjects("key", name, full_mark, sort_order, enabled) VALUES (?, ?, ?, ?, ?)`,
			Args: []any{key, name, fullMark, sortOrder, boolInt(subject.Enabled)},
		})
	}
	_, err := s.db.Transaction(ctx, statements)
	return err
}

// ListTracks 返回选科类别配置列表。
func (s *Store) ListTracks(ctx context.Context) ([]Track, error) {
	rows, err := s.db.Query(ctx, `SELECT "key", name, sort_order, enabled FROM tracks ORDER BY sort_order, "key"`)
	if err != nil {
		return nil, err
	}
	result := make([]Track, 0, len(rows.Values))
	for _, row := range rows.Values {
		result = append(result, Track{
			Key:     stringValue(row[0]),
			Name:    stringValue(row[1]),
			Sort:    int(intValue(row[2])),
			Enabled: intValue(row[3]) != 0,
		})
	}
	return result, nil
}

// SaveTracks 更新并保存选科类别配置。
func (s *Store) SaveTracks(ctx context.Context, tracks []Track) error {
	statements := []sqlite.Statement{{SQL: `DELETE FROM tracks`}}
	for index, track := range tracks {
		key := strings.TrimSpace(track.Key)
		name := strings.TrimSpace(track.Name)
		if key == "" || name == "" {
			continue
		}
		sortOrder := track.Sort
		if sortOrder == 0 {
			sortOrder = index + 1
		}
		statements = append(statements, sqlite.Statement{
			SQL:  `INSERT INTO tracks("key", name, sort_order, enabled) VALUES (?, ?, ?, ?)`,
			Args: []any{key, name, sortOrder, boolInt(track.Enabled)},
		})
	}
	_, err := s.db.Transaction(ctx, statements)
	return err
}

// EnsureDefaultPlan 确保默认考核方案存在且至少有一个已发布版本。
func (s *Store) EnsureDefaultPlan(ctx context.Context) (PlanVersion, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, scope, is_system, created_at FROM plans WHERE is_system = 1 LIMIT 1`)
	if err != nil {
		return PlanVersion{}, err
	}
	if len(rows.Values) > 0 {
		plan, err := planFromRow(rows.Values[0])
		if err != nil {
			return PlanVersion{}, err
		}
		published, err := s.LatestPublished(ctx, plan.ID)
		if err == nil {
			return published, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return PlanVersion{}, err
		}
		return s.restoreSystemPlan(ctx, plan.ID)
	}
	cfg := assessment.DefaultConfig()
	encoded, hash, err := encodeConfig(cfg)
	if err != nil {
		return PlanVersion{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.Execute(ctx, `INSERT INTO plans(name, scope, is_system, created_at) VALUES (?, 'school', 1, ?)`,
		systemPlanName, now)
	if err != nil {
		return PlanVersion{}, err
	}
	planID := result.LastInsertID
	verResult, err := s.db.Execute(ctx, `INSERT INTO plan_versions(
		plan_id, version, status, config_json, note, created_by,
		parent_version_id, change_summary, config_hash, published_at, created_at)
		VALUES (?, 1, 'published', ?, '默认量化考核方案', 'system', NULL, '内置默认方案', ?, ?, ?)`,
		planID, encoded, hash, now, now)
	if err != nil {
		return PlanVersion{}, err
	}
	return s.GetVersion(ctx, verResult.LastInsertID)
}

func (s *Store) restoreSystemPlan(ctx context.Context, planID int64) (PlanVersion, error) {
	cfg := assessment.DefaultConfig()
	latestID, err := s.latestVersionID(ctx, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	if latestID > 0 {
		if latest, lErr := s.GetVersion(ctx, latestID); lErr == nil {
			cfg = latest.Config
		}
	}
	encoded, hash, err := encodeConfig(cfg)
	if err != nil {
		return PlanVersion{}, err
	}
	verRows, err := s.db.Query(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM plan_versions WHERE plan_id = ?`, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	nextVer := 1
	if len(verRows.Values) > 0 {
		nextVer = int(intValue(verRows.Values[0][0]))
	}
	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `INSERT INTO plan_versions(
		plan_id, version, status, config_json, note, created_by,
		parent_version_id, change_summary, config_hash, published_at, created_at)
		VALUES (?, ?, 'published', ?, '恢复默认方案', 'system', ?, '自动恢复默认方案', ?, ?, ?)`,
		planID, nextVer, encoded, nullableID(latestID), hash, now, now)
	if err != nil {
		return PlanVersion{}, err
	}
	return s.GetVersion(ctx, res.LastInsertID)
}

// ListPlans 返回所有考核方案。
func (s *Store) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, scope, is_system, created_at FROM plans ORDER BY is_system DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	plans := make([]Plan, 0, len(rows.Values))
	for _, row := range rows.Values {
		plan, err := planFromRow(row)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// CreatePlan 新建一个自定义考核方案并创建初始草稿版本。
func (s *Store) CreatePlan(ctx context.Context, input CreatePlanInput) (Plan, PlanVersion, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Plan{}, PlanVersion{}, ErrPlanNameEmpty
	}
	if err := validateConfig(input.Config); err != nil {
		return Plan{}, PlanVersion{}, err
	}
	encoded, hash, err := encodeConfig(input.Config)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	scope := "school"
	if input.Kind != "" {
		scope = input.Kind
	}
	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `INSERT INTO plans(name, scope, is_system, created_at) VALUES (?, ?, 0, ?)`,
		name, scope, now)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	plan, err := s.getPlan(ctx, res.LastInsertID)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	plan.Kind = input.Kind
	plan.Description = input.Description
	plan.BasePlanVersionID = input.BaseVersionID
	verRes, err := s.db.Execute(ctx, `INSERT INTO plan_versions(
		plan_id, version, status, config_json, note, created_by,
		parent_version_id, change_summary, config_hash, published_at, created_at)
		VALUES (?, 1, 'draft', ?, ?, ?, ?, ?, ?, NULL, ?)`,
		plan.ID, encoded, input.Description, input.CreatedBy, nullableID(input.BaseVersionID), "新建方案", hash, now)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	ver, err := s.GetVersion(ctx, verRes.LastInsertID)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	return plan, ver, nil
}

// UpdatePlan 为已有方案追加一个新版本：系统默认方案直接追加已发布版本，自定义方案追加草稿版本。
func (s *Store) UpdatePlan(ctx context.Context, planID int64, config assessment.Config, summary, createdBy string) (PlanVersion, error) {
	plan, err := s.getPlan(ctx, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	if err := validateConfig(config); err != nil {
		return PlanVersion{}, err
	}
	encoded, hash, err := encodeConfig(config)
	if err != nil {
		return PlanVersion{}, err
	}
	parentID, err := s.latestVersionID(ctx, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	verRows, err := s.db.Query(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM plan_versions WHERE plan_id = ?`, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	nextVer := 1
	if len(verRows.Values) > 0 {
		nextVer = int(intValue(verRows.Values[0][0]))
	}
	now := s.now().Format(time.RFC3339Nano)
	status := "draft"
	var pubAt any
	if plan.IsSystem {
		status = "published"
		pubAt = now
	}
	res, err := s.db.Execute(ctx, `INSERT INTO plan_versions(
		plan_id, version, status, config_json, note, created_by,
		parent_version_id, change_summary, config_hash, published_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		planID, nextVer, status, encoded, summary, createdBy, nullableID(parentID), summary, hash, pubAt, now)
	if err != nil {
		return PlanVersion{}, err
	}
	return s.GetVersion(ctx, res.LastInsertID)
}

// Publish 发布指定方案版本。
func (s *Store) Publish(ctx context.Context, id int64, _ string) error {
	_, err := s.PublishVersion(ctx, id)
	return err
}

// CreateDraft 为考核方案创建一个草稿版本。
func (s *Store) CreateDraft(ctx context.Context, input CreateDraftInput) (PlanVersion, error) {
	if _, err := s.getPlan(ctx, input.PlanID); err != nil {
		return PlanVersion{}, err
	}
	if err := validateConfig(input.Config); err != nil {
		return PlanVersion{}, err
	}
	encoded, hash, err := encodeConfig(input.Config)
	if err != nil {
		return PlanVersion{}, err
	}
	parentID := input.BaseVersionID
	if parentID <= 0 {
		var lErr error
		parentID, lErr = s.latestVersionID(ctx, input.PlanID)
		if lErr != nil {
			return PlanVersion{}, lErr
		}
	}
	verRows, err := s.db.Query(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM plan_versions WHERE plan_id = ?`, input.PlanID)
	if err != nil {
		return PlanVersion{}, err
	}
	nextVer := 1
	if len(verRows.Values) > 0 {
		nextVer = int(intValue(verRows.Values[0][0]))
	}
	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `INSERT INTO plan_versions(
		plan_id, version, status, config_json, note, created_by,
		parent_version_id, change_summary, config_hash, published_at, created_at)
		VALUES (?, ?, 'draft', ?, ?, ?, ?, ?, ?, NULL, ?)`,
		input.PlanID, nextVer, encoded, input.Notes, input.CreatedBy, nullableID(parentID), input.Notes, hash, now)
	if err != nil {
		return PlanVersion{}, err
	}
	return s.GetVersion(ctx, res.LastInsertID)
}

// PublishVersion 发布一个草稿版本。
func (s *Store) PublishVersion(ctx context.Context, id int64) (PlanVersion, error) {
	version, err := s.GetVersion(ctx, id)
	if err != nil {
		return PlanVersion{}, err
	}
	if version.Status != "draft" {
		return PlanVersion{}, fmt.Errorf("只能发布草稿版本的方案")
	}
	if err := validateConfig(version.Config); err != nil {
		return PlanVersion{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := s.db.Execute(ctx, `UPDATE plan_versions SET status = 'published', published_at = ? WHERE id = ?`, now, id); err != nil {
		return PlanVersion{}, err
	}
	return s.GetVersion(ctx, id)
}

// Retire 停用非系统方案版本。
func (s *Store) Retire(ctx context.Context, id int64) error {
	version, err := s.GetVersion(ctx, id)
	if err != nil {
		return err
	}
	plan, err := s.getPlan(ctx, version.PlanID)
	if err != nil {
		return err
	}
	if plan.IsSystem {
		return ErrSystemPlanRetire
	}
	_, err = s.db.Execute(ctx, `UPDATE plan_versions SET status = 'retired' WHERE id = ?`, id)
	return err
}

// ListVersions 返回某个方案的全部版本（按版本号升序）。
func (s *Store) ListVersions(ctx context.Context, planID int64) ([]PlanVersion, error) {
	rows, err := s.db.Query(ctx, `SELECT id, plan_id, version, status, config_json, note, created_by,
		parent_version_id, change_summary, config_hash, published_at, created_at
		FROM plan_versions WHERE plan_id = ? ORDER BY version ASC`, planID)
	if err != nil {
		return nil, err
	}
	result := make([]PlanVersion, 0, len(rows.Values))
	for _, row := range rows.Values {
		ver, err := versionFromRow(row)
		if err != nil {
			return nil, err
		}
		result = append(result, ver)
	}
	return result, nil
}

// GetVersion 读取单个方案版本。
func (s *Store) GetVersion(ctx context.Context, id int64) (PlanVersion, error) {
	rows, err := s.db.Query(ctx, `SELECT id, plan_id, version, status, config_json, note, created_by,
		parent_version_id, change_summary, config_hash, published_at, created_at
		FROM plan_versions WHERE id = ?`, id)
	if err != nil {
		return PlanVersion{}, err
	}
	if len(rows.Values) == 0 {
		return PlanVersion{}, ErrNotFound
	}
	return versionFromRow(rows.Values[0])
}

// LatestPublished 返回某个方案最新已发布版本。
func (s *Store) LatestPublished(ctx context.Context, planID int64) (PlanVersion, error) {
	rows, err := s.db.Query(ctx, `SELECT id, plan_id, version, status, config_json, note, created_by,
		parent_version_id, change_summary, config_hash, published_at, created_at
		FROM plan_versions WHERE plan_id = ? AND status = 'published' ORDER BY version DESC LIMIT 1`, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	if len(rows.Values) == 0 {
		return PlanVersion{}, ErrNotFound
	}
	return versionFromRow(rows.Values[0])
}

func (s *Store) latestVersionID(ctx context.Context, planID int64) (int64, error) {
	rows, err := s.db.Query(ctx, `SELECT id FROM plan_versions WHERE plan_id = ? ORDER BY version DESC LIMIT 1`, planID)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, nil
	}
	return intValue(rows.Values[0][0]), nil
}

func (s *Store) getPlan(ctx context.Context, id int64) (Plan, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, scope, is_system, created_at FROM plans WHERE id = ?`, id)
	if err != nil {
		return Plan{}, err
	}
	if len(rows.Values) == 0 {
		return Plan{}, ErrNotFound
	}
	return planFromRow(rows.Values[0])
}

func validateConfig(config assessment.Config) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("方案配置无效: %w", err)
	}
	if err := config.ValidateFormulas(); err != nil {
		return fmt.Errorf("方案公式无效: %w", err)
	}
	return nil
}

func encodeConfig(config assessment.Config) (string, string, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	return string(raw), hex.EncodeToString(sum[:]), nil
}

func planFromRow(row []any) (Plan, error) {
	createdAt, err := parseTime(row[4])
	if err != nil {
		return Plan{}, err
	}
	scope := stringValue(row[2])
	isSys := intValue(row[3]) != 0
	kind := scope
	if isSys {
		kind = "system"
	} else if kind == "school" {
		kind = "user"
	}
	return Plan{
		ID:        intValue(row[0]),
		Name:      stringValue(row[1]),
		Kind:      kind,
		Scope:     scope,
		IsSystem:  isSys,
		CreatedAt: createdAt,
	}, nil
}

func versionFromRow(row []any) (PlanVersion, error) {
	rawConfig := stringValue(row[4])
	var config assessment.Config
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		return PlanVersion{}, fmt.Errorf("解析方案配置 JSON: %w", err)
	}
	publishedAt, err := nullableTime(row[10])
	if err != nil {
		return PlanVersion{}, err
	}
	createdAt, err := parseTime(row[11])
	if err != nil {
		return PlanVersion{}, err
	}
	parentID, _ := nullableInt64(row[7])
	return PlanVersion{
		ID:              intValue(row[0]),
		PlanID:          intValue(row[1]),
		Version:         int(intValue(row[2])),
		Status:          stringValue(row[3]),
		Config:          config,
		Note:            stringValue(row[5]),
		CreatedBy:       stringValue(row[6]),
		ParentVersionID: parentID,
		ChangeSummary:   stringValue(row[8]),
		ConfigHash:      stringValue(row[9]),
		PublishedAt:     publishedAt,
		CreatedAt:       createdAt,
	}, nil
}

// -----------------------------------------------------------------------------
// Shared SQL / Value Helpers
// -----------------------------------------------------------------------------

func nullableID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func nullableInt64(value any) (int64, bool) {
	if value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func parseTime(value any) (time.Time, error) {
	raw := stringValue(value)
	if raw == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q", raw)
}

func nullableTime(value any) (*time.Time, error) {
	if value == nil || stringValue(value) == "" {
		return nil, nil
	}
	parsed, err := parseTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func nullableFloat(value any) (*float64, bool) {
	if value == nil {
		return nil, false
	}
	switch v := value.(type) {
	case float64:
		copied := v
		return &copied, true
	case int64:
		copied := float64(v)
		return &copied, true
	default:
		return nil, false
	}
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

func intValue(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func bumpRevisionStatement(scopeKey, now string) sqlite.Statement {
	return sqlite.Statement{
		SQL: `INSERT INTO data_revisions(scope_key, revision, updated_at)
			VALUES (?, 1, ?)
			ON CONFLICT(scope_key) DO UPDATE SET revision = revision + 1, updated_at = excluded.updated_at`,
		Args: []any{scopeKey, now},
	}
}

func auditLogStatement(action, targetType string, targetID int64, payload any, now string) sqlite.Statement {
	raw := "{}"
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			raw = string(b)
		}
	}
	return sqlite.Statement{
		SQL:  `INSERT INTO audit_logs(action, target_type, target_id, payload_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		Args: []any{action, targetType, targetID, raw, now},
	}
}
