// Package store 使用 SQLite 保存考核方案、方案版本和核算结果。
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/assessment"
)

// ErrNotFound 表示查询的方案或者版本不存在。
var ErrNotFound = errors.New("记录不存在")

// ErrSystemPlanRetire 表示内置方案不允许停用。
var ErrSystemPlanRetire = errors.New("system plan cannot be retired")

// SchemaVersion 是当前应用写入的数据库版本。
const SchemaVersion = 5

// Store 是一个打开的本地数据库。
type Store struct {
	db   *sqlite.DB
	root string
	now  func() time.Time
}

// Plan 是一项考核方案。
type Plan struct {
	ID                int64
	Name              string
	Kind              string
	BasePlanVersionID int64
	Description       string
	CreatedAt         time.Time
}

// PlanVersion 是一个方案版本。
type PlanVersion struct {
	ID              int64
	PlanID          int64
	Version         int
	Status          string
	SchemaVersion   int
	Config          assessment.Config
	ConfigJSON      string
	ConfigHash      string
	ParentVersionID int64
	Notes           string
	CreatedAt       time.Time
	CreatedBy       string
	PublishedAt     *time.Time
	PublishedBy     string
}

// CreatePlanInput 是新建方案需要的字段。
type CreatePlanInput struct {
	Name          string
	Kind          string
	Description   string
	BaseVersionID int64
	Config        assessment.Config
	CreatedBy     string
}

// CreateDraftInput 是在已有方案下新建草稿需要的字段。
type CreateDraftInput struct {
	PlanID        int64
	BaseVersionID int64
	Config        assessment.Config
	Notes         string
	CreatedBy     string
}

// Open 打开数据库并执行迁移。
func Open(ctx context.Context, path string) (*Store, error) {
	root := ""
	if path != ":memory:" {
		root = filepath.Dir(path)
		pending, err := needsSchoolMigration(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("检查数据库版本: %w", err)
		}
		if !pending {
			root = ""
		}
	}
	backup := ""
	if root != "" {
		var err error
		backup, err = BackupBeforeOpen(root)
		if err != nil {
			return nil, fmt.Errorf("备份现有数据库: %w", err)
		}
	}
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, root: filepath.Dir(path), now: func() time.Time { return time.Now().UTC() }}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		if backup != "" {
			if restoreErr := restoreBackup(root, filepath.Base(backup), false); restoreErr != nil {
				return nil, fmt.Errorf("%w；恢复迁移前备份失败: %v", err, restoreErr)
			}
			return nil, fmt.Errorf("%w；数据库已经恢复到迁移前状态", err)
		}
		return nil, err
	}
	return store, nil
}

func needsSchoolMigration(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return false, nil
	}
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{ReadOnly: true})
	if err != nil {
		return false, err
	}
	defer db.Close()
	rows, err := db.Query(ctx, `SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`)
	if err != nil {
		return false, err
	}
	if len(rows.Values) == 0 {
		return true, nil
	}
	rows, err = db.Query(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`)
	if err != nil {
		return false, err
	}
	version := 0
	if len(rows.Values) > 0 {
		value, _ := rows.Values[0][0].(int64)
		version = int(value)
	}
	if version > SchemaVersion {
		return false, fmt.Errorf("数据库版本 %d 高于应用支持的版本 %d", version, SchemaVersion)
	}
	return version < SchemaVersion, nil
}

// Close 关闭数据库。
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`},
		{SQL: `INSERT OR IGNORE INTO schema_migrations(version, applied_at)
			VALUES (1, ?)`, Args: []any{s.now().Format(time.RFC3339Nano)}},
		{SQL: `CREATE TABLE IF NOT EXISTS plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			kind TEXT NOT NULL,
			base_plan_version_id INTEGER,
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS plan_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plan_id INTEGER NOT NULL,
			version INTEGER NOT NULL,
			status TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			config_json TEXT NOT NULL,
			config_hash TEXT NOT NULL,
			parent_version_id INTEGER,
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			published_at TEXT,
			published_by TEXT NOT NULL DEFAULT ''
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS plan_versions_plan_version
			ON plan_versions(plan_id, version)`},
		{SQL: `CREATE INDEX IF NOT EXISTS plan_versions_status
			ON plan_versions(plan_id, status)`},
	}
	statements = append(statements, batchSchema()...)
	statements = append(statements, cohortSchema()...)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return err
	}
	if err := s.deriveLegacyCohorts(ctx); err != nil {
		return err
	}
	return s.migrateSchoolData(ctx)
}

// EnsureDefaultPlan 在首次启动时写入内置默认方案，返回最新的已发布版本。
func (s *Store) EnsureDefaultPlan(ctx context.Context) (PlanVersion, error) {
	rows, err := s.db.Query(ctx, `SELECT id FROM plans WHERE kind = 'system' ORDER BY id LIMIT 1`)
	if err != nil {
		return PlanVersion{}, err
	}
	if len(rows.Values) == 0 {
		config := assessment.DefaultConfig()
		_, version, err := s.createPlan(ctx, CreatePlanInput{
			Name:        config.Name,
			Kind:        "system",
			Description: config.Description,
			Config:      config,
		}, true)
		if err != nil {
			return PlanVersion{}, err
		}
		return version, nil
	}
	planID, ok := rows.Values[0][0].(int64)
	if !ok {
		return PlanVersion{}, fmt.Errorf("默认方案标识格式不正确")
	}
	version, err := s.LatestPublished(ctx, planID)
	if err == nil {
		return version, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return PlanVersion{}, err
	}
	return s.restoreSystemPlan(ctx, planID)
}

// ListPlans 返回全部方案。
func (s *Store) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, kind, base_plan_version_id, description, created_at
		FROM plans ORDER BY kind, name, id`)
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

// ListVersions 返回一个方案的全部版本。
func (s *Store) ListVersions(ctx context.Context, planID int64) ([]PlanVersion, error) {
	rows, err := s.db.Query(ctx, `SELECT id, plan_id, version, status, schema_version, config_json,
		config_hash, parent_version_id, notes, created_at, created_by, published_at, published_by
		FROM plan_versions WHERE plan_id = ? ORDER BY version`, planID)
	if err != nil {
		return nil, err
	}
	versions := make([]PlanVersion, 0, len(rows.Values))
	for _, row := range rows.Values {
		version, err := versionFromRow(row)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

// GetVersion 按标识读取一个方案版本。
func (s *Store) GetVersion(ctx context.Context, id int64) (PlanVersion, error) {
	rows, err := s.db.Query(ctx, `SELECT id, plan_id, version, status, schema_version, config_json,
		config_hash, parent_version_id, notes, created_at, created_by, published_at, published_by
		FROM plan_versions WHERE id = ?`, id)
	if err != nil {
		return PlanVersion{}, err
	}
	if len(rows.Values) == 0 {
		return PlanVersion{}, ErrNotFound
	}
	return versionFromRow(rows.Values[0])
}

// LatestPublished 返回一个方案最新的已发布版本。
func (s *Store) LatestPublished(ctx context.Context, planID int64) (PlanVersion, error) {
	rows, err := s.db.Query(ctx, `SELECT id, plan_id, version, status, schema_version, config_json,
		config_hash, parent_version_id, notes, created_at, created_by, published_at, published_by
		FROM plan_versions WHERE plan_id = ? AND status = 'published'
		ORDER BY version DESC LIMIT 1`, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	if len(rows.Values) == 0 {
		return PlanVersion{}, ErrNotFound
	}
	return versionFromRow(rows.Values[0])
}

// CreatePlan 新建一个派生方案，版本一为草稿。
func (s *Store) CreatePlan(ctx context.Context, input CreatePlanInput) (Plan, PlanVersion, error) {
	return s.createPlan(ctx, input, false)
}

func (s *Store) createPlan(ctx context.Context, input CreatePlanInput, publish bool) (Plan, PlanVersion, error) {
	if input.Kind != "system" && input.Kind != "user" {
		return Plan{}, PlanVersion{}, fmt.Errorf("方案类型必须是 system 或 user")
	}
	if input.Name == "" {
		return Plan{}, PlanVersion{}, fmt.Errorf("方案名称不能为空")
	}
	if err := validateConfig(input.Config); err != nil {
		return Plan{}, PlanVersion{}, err
	}
	if input.BaseVersionID != 0 {
		if _, err := s.GetVersion(ctx, input.BaseVersionID); err != nil {
			return Plan{}, PlanVersion{}, fmt.Errorf("来源版本: %w", err)
		}
	}
	configJSON, configHash, err := encodeConfig(input.Config)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	now := s.now()
	planResult, err := s.db.Execute(ctx, `INSERT INTO plans(name, kind, base_plan_version_id, description, created_at)
		VALUES (?, ?, ?, ?, ?)`, input.Name, input.Kind, nullableID(input.BaseVersionID), input.Description, now.Format(time.RFC3339Nano))
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	planID := planResult.LastInsertID
	status := "draft"
	var publishedAt any
	publishedBy := ""
	if publish {
		status = "published"
		publishedAt = now.Format(time.RFC3339Nano)
		publishedBy = input.CreatedBy
	}
	versionResult, err := s.db.Execute(ctx, `INSERT INTO plan_versions(
		plan_id, version, status, schema_version, config_json, config_hash, parent_version_id, notes,
		created_at, created_by, published_at, published_by)
		VALUES (?, 1, ?, ?, ?, ?, ?, '', ?, ?, ?, ?)`,
		planID, status, input.Config.SchemaVersion, configJSON, configHash,
		nullableID(input.BaseVersionID), now.Format(time.RFC3339Nano), input.CreatedBy, publishedAt, publishedBy)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	plan, err := s.getPlan(ctx, planID)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	version, err := s.GetVersion(ctx, versionResult.LastInsertID)
	if err != nil {
		return Plan{}, PlanVersion{}, err
	}
	return plan, version, nil
}

// CreateDraft 在一个已有方案下新建草稿版本。
func (s *Store) CreateDraft(ctx context.Context, input CreateDraftInput) (PlanVersion, error) {
	if err := validateConfig(input.Config); err != nil {
		return PlanVersion{}, err
	}
	_, err := s.getPlan(ctx, input.PlanID)
	if err != nil {
		return PlanVersion{}, err
	}
	if input.BaseVersionID != 0 {
		base, err := s.GetVersion(ctx, input.BaseVersionID)
		if err != nil {
			return PlanVersion{}, fmt.Errorf("来源版本: %w", err)
		}
		if base.PlanID != input.PlanID {
			return PlanVersion{}, fmt.Errorf("来源版本不属于当前方案")
		}
	}
	configJSON, configHash, err := encodeConfig(input.Config)
	if err != nil {
		return PlanVersion{}, err
	}
	result, err := s.db.Execute(ctx, `INSERT INTO plan_versions(
		plan_id, version, status, schema_version, config_json, config_hash, parent_version_id, notes,
		created_at, created_by, published_at, published_by)
		VALUES (?, (SELECT COALESCE(MAX(version), 0) + 1 FROM plan_versions WHERE plan_id = ?),
		'draft', ?, ?, ?, ?, ?, ?, ?, NULL, '')`,
		input.PlanID, input.PlanID, input.Config.SchemaVersion, configJSON, configHash,
		nullableID(input.BaseVersionID), input.Notes, s.now().Format(time.RFC3339Nano), input.CreatedBy)
	if err != nil {
		return PlanVersion{}, err
	}
	return s.GetVersion(ctx, result.LastInsertID)
}

// Publish 发布一个草稿版本。
func (s *Store) Publish(ctx context.Context, id int64, user string) error {
	version, err := s.GetVersion(ctx, id)
	if err != nil {
		return err
	}
	if version.Status != "draft" {
		return fmt.Errorf("只有草稿版本可以发布")
	}
	if err := validateConfig(version.Config); err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.Execute(ctx, `UPDATE plan_versions
		SET status = 'published', published_at = ?, published_by = ?
		WHERE id = ? AND status = 'draft'`, now, user, id)
	return err
}

// UpdatePlan 在当前方案下追加一个版本。内置方案直接发布，派生方案保存为草稿。
func (s *Store) UpdatePlan(ctx context.Context, planID int64, config assessment.Config, notes, user string) (PlanVersion, error) {
	plan, err := s.getPlan(ctx, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	if err := validateConfig(config); err != nil {
		return PlanVersion{}, err
	}
	parentID, err := s.latestVersionID(ctx, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	configJSON, configHash, err := encodeConfig(config)
	if err != nil {
		return PlanVersion{}, err
	}
	status := "draft"
	var publishedAt any
	publishedBy := ""
	if plan.Kind == "system" {
		status = "published"
		publishedAt = s.now().Format(time.RFC3339Nano)
		publishedBy = user
	}
	now := s.now().Format(time.RFC3339Nano)
	results, err := s.db.Transaction(ctx, []sqlite.Statement{
		{
			SQL:  `UPDATE plans SET name = ?, description = ? WHERE id = ?`,
			Args: []any{config.Name, config.Description, planID},
		},
		{
			SQL: `INSERT INTO plan_versions(
				plan_id, version, status, schema_version, config_json, config_hash, parent_version_id, notes,
				created_at, created_by, published_at, published_by)
				VALUES (?, (SELECT COALESCE(MAX(version), 0) + 1 FROM plan_versions WHERE plan_id = ?),
				?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{
				planID, planID, status, config.SchemaVersion, configJSON, configHash,
				nullableID(parentID), notes, now, user, publishedAt, publishedBy,
			},
		},
	})
	if err != nil {
		return PlanVersion{}, err
	}
	return s.GetVersion(ctx, results[1].LastInsertID)
}

// Retire 停用一个已发布版本。
func (s *Store) Retire(ctx context.Context, id int64) error {
	version, err := s.GetVersion(ctx, id)
	if err != nil {
		return err
	}
	plan, err := s.getPlan(ctx, version.PlanID)
	if err != nil {
		return err
	}
	if plan.Kind == "system" {
		return ErrSystemPlanRetire
	}
	if version.Status != "published" {
		return fmt.Errorf("只有已发布版本可以停用")
	}
	_, err = s.db.Execute(ctx, `UPDATE plan_versions SET status = 'retired' WHERE id = ? AND status = 'published'`, id)
	return err
}

func (s *Store) restoreSystemPlan(ctx context.Context, planID int64) (PlanVersion, error) {
	versions, err := s.ListVersions(ctx, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	config := assessment.DefaultConfig()
	if len(versions) > 0 {
		config = versions[len(versions)-1].Config
	}
	return s.UpdatePlan(ctx, planID, config, "", "")
}

func (s *Store) latestVersionID(ctx context.Context, planID int64) (int64, error) {
	rows, err := s.db.Query(ctx, `SELECT id FROM plan_versions WHERE plan_id = ? ORDER BY version DESC LIMIT 1`, planID)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, nil
	}
	id, ok := rows.Values[0][0].(int64)
	if !ok {
		return 0, fmt.Errorf("方案版本标识格式不正确")
	}
	return id, nil
}

func (s *Store) getPlan(ctx context.Context, id int64) (Plan, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, kind, base_plan_version_id, description, created_at
		FROM plans WHERE id = ?`, id)
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
		return err
	}
	return config.ValidateFormulas()
}

func encodeConfig(config assessment.Config) (string, string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(data)
	return string(data), hex.EncodeToString(sum[:]), nil
}

func planFromRow(row []any) (Plan, error) {
	id, ok := row[0].(int64)
	if !ok {
		return Plan{}, fmt.Errorf("方案标识格式不正确")
	}
	name, _ := row[1].(string)
	kind, _ := row[2].(string)
	base, _ := nullableInt64(row[3])
	description, _ := row[4].(string)
	createdAt, err := parseTime(row[5])
	if err != nil {
		return Plan{}, err
	}
	return Plan{ID: id, Name: name, Kind: kind, BasePlanVersionID: base, Description: description, CreatedAt: createdAt}, nil
}

func versionFromRow(row []any) (PlanVersion, error) {
	id, ok := row[0].(int64)
	if !ok {
		return PlanVersion{}, fmt.Errorf("方案版本标识格式不正确")
	}
	planID, _ := row[1].(int64)
	versionNumber, _ := row[2].(int64)
	status, _ := row[3].(string)
	schemaVersion, _ := row[4].(int64)
	configJSON, _ := row[5].(string)
	configHash, _ := row[6].(string)
	parentID, _ := nullableInt64(row[7])
	notes, _ := row[8].(string)
	createdAt, err := parseTime(row[9])
	if err != nil {
		return PlanVersion{}, err
	}
	createdBy, _ := row[10].(string)
	publishedAt, err := nullableTime(row[11])
	if err != nil {
		return PlanVersion{}, err
	}
	publishedBy, _ := row[12].(string)
	config, err := assessment.Decode([]byte(configJSON))
	if err != nil {
		return PlanVersion{}, fmt.Errorf("方案版本 %d 的配置无法读取: %w", id, err)
	}
	return PlanVersion{
		ID:              id,
		PlanID:          planID,
		Version:         int(versionNumber),
		Status:          status,
		SchemaVersion:   int(schemaVersion),
		Config:          config,
		ConfigJSON:      configJSON,
		ConfigHash:      configHash,
		ParentVersionID: parentID,
		Notes:           notes,
		CreatedAt:       createdAt,
		CreatedBy:       createdBy,
		PublishedAt:     publishedAt,
		PublishedBy:     publishedBy,
	}, nil
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullableInt64(value any) (int64, bool) {
	if value == nil {
		return 0, true
	}
	id, ok := value.(int64)
	return id, ok
}

func parseTime(value any) (time.Time, error) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("时间格式不正确")
	}
	return time.Parse(time.RFC3339Nano, text)
}

func nullableTime(value any) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
