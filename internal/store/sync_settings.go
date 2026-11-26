package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// SyncSettings 保存 WebDAV 云端同步与跨设备协作的非敏感配置。
// 密码等敏感数据保存在系统凭据库 (mygo.Secrets)，不保存在此表。
type SyncSettings struct {
	WebDAVURL       string `json:"webdav_url"`
	Username        string `json:"username"`
	RemoteDir       string `json:"remote_dir"`
	DeviceID        string `json:"device_id"`
	DeviceName      string `json:"device_name"`
	EncryptEnabled  bool   `json:"encrypt_enabled"`
	RemindOnExit    bool   `json:"remind_on_exit"`
	LastSyncTime    string `json:"last_sync_time"`
	LastSyncRev     int64  `json:"last_sync_rev"`
	LastPackageHash string `json:"last_package_hash"`
	LastSyncDevice  string `json:"last_sync_device"`
}

const (
	syncSettingsKey   = "settings"
	syncDatabaseIDKey = "database_id"
)

// GetSyncSettings 读取本地同步设置；首次调用时初始化默认记录与设备标识。
func (s *Store) GetSyncSettings(ctx context.Context) (SyncSettings, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM sync_state WHERE "key" = ?`, syncSettingsKey)
	if err != nil {
		return SyncSettings{}, err
	}
	if len(rows.Values) == 0 {
		devID := generateDeviceID()
		devName, _ := os.Hostname()
		if devName == "" {
			devName = "Office PC"
		}
		def := SyncSettings{
			RemoteDir:      "Tally",
			DeviceID:       devID,
			DeviceName:     devName,
			EncryptEnabled: true,
		}
		if err := s.SaveSyncSettings(ctx, def); err != nil {
			return SyncSettings{}, err
		}
		return def, nil
	}
	var settings SyncSettings
	raw := stringValue(rows.Values[0][0])
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return SyncSettings{}, fmt.Errorf("解析同步设置: %w", err)
		}
	}
	if settings.RemoteDir == "" {
		settings.RemoteDir = "Tally"
	}
	if settings.DeviceID == "" {
		settings.DeviceID = generateDeviceID()
	}
	return settings, nil
}

// SaveSyncSettings 保存或更新同步设置。
func (s *Store) SaveSyncSettings(ctx context.Context, settings SyncSettings) error {
	if settings.RemoteDir == "" {
		settings.RemoteDir = "Tally"
	}
	if settings.DeviceID == "" {
		settings.DeviceID = generateDeviceID()
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.Execute(ctx, `INSERT INTO sync_state("key", value, updated_at)
		VALUES (?, ?, ?) ON CONFLICT("key") DO UPDATE SET value = excluded.value,
		updated_at = excluded.updated_at`, syncSettingsKey, string(encoded), now)
	return err
}

// GetDatabaseID 获取当前数据库全局唯一标识；若无则自动生成并持久化。
func (s *Store) GetDatabaseID(ctx context.Context) (string, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE "key" = ?`, syncDatabaseIDKey)
	if err != nil {
		return "", err
	}
	if len(rows.Values) > 0 {
		if val := stringValue(rows.Values[0][0]); val != "" {
			return val, nil
		}
	}
	dbID := generateDeviceID()
	if err := s.SetDatabaseID(ctx, dbID); err != nil {
		return "", err
	}
	return dbID, nil
}

// SetDatabaseID 强制指定数据库标识（例如拉取远端覆盖后对齐标识）。
func (s *Store) SetDatabaseID(ctx context.Context, id string) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `INSERT INTO app_settings("key", value, updated_at)
		VALUES (?, ?, ?) ON CONFLICT("key") DO UPDATE SET value = excluded.value,
		updated_at = excluded.updated_at`, syncDatabaseIDKey, id, now)
	return err
}

// GetGlobalRevision 计算当前数据库的全局修订号与各作用域的修订号映射。
func (s *Store) GetGlobalRevision(ctx context.Context) (int64, map[string]int64, error) {
	scopeRevs := make(map[string]int64)
	var totalRev int64

	// 1. 考核版本计数
	rows, err := s.db.Query(ctx, `SELECT exam_cohort_id, revision FROM assessments`)
	if err != nil {
		return 0, nil, err
	}
	for _, row := range rows.Values {
		id := intValue(row[0])
		rev := intValue(row[1])
		scopeRevs[fmt.Sprintf("assessment-%d", id)] = rev
		totalRev += rev
	}

	// 2. 数据修订账本
	rows, err = s.db.Query(ctx, `SELECT scope_key, revision FROM data_revisions`)
	if err != nil {
		return 0, nil, err
	}
	for _, row := range rows.Values {
		scopeKey := stringValue(row[0])
		rev := intValue(row[1])
		scopeRevs[scopeKey] = rev
		totalRev += rev
	}

	// 3. 年级、考试与方案计数
	for _, item := range []struct {
		key   string
		query string
	}{
		{"cohorts", `SELECT COUNT(*), COALESCE(MAX(id), 0) FROM cohorts`},
		{"exams", `SELECT COUNT(*), COALESCE(MAX(id), 0) FROM exams`},
		{"plans", `SELECT COUNT(*), COALESCE(MAX(version), 0) FROM plan_versions`},
	} {
		qRows, err := s.db.Query(ctx, item.query)
		if err != nil {
			return 0, nil, err
		}
		if len(qRows.Values) > 0 {
			cnt := intValue(qRows.Values[0][0])
			maxVal := intValue(qRows.Values[0][1])
			totalRev += cnt + maxVal
			if cnt > 0 && item.key != "plans" {
				scopeRevs[item.key] = cnt + maxVal
			}
		}
	}

	// 4. 审计日志序列
	rows, err = s.db.Query(ctx, `SELECT COALESCE(MAX(id), 0) FROM audit_logs`)
	if err != nil {
		return 0, nil, err
	}
	if len(rows.Values) > 0 {
		totalRev += intValue(rows.Values[0][0])
	}

	return totalRev, scopeRevs, nil
}

func generateDeviceID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("dev-%d", time.Now().UnixNano()%1000000)
	}
	hostname, _ := os.Hostname()
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		hostname = "host"
	}
	if len(hostname) > 12 {
		hostname = hostname[:12]
	}
	return fmt.Sprintf("%s-%s", hostname, hex.EncodeToString(b)[:8])
}

// syncStateSet 是包内复用的简单 KV 写入 helper。
func (s *Store) syncStateSet(ctx context.Context, key, value string) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.Execute(ctx, `INSERT INTO sync_state("key", value, updated_at)
		VALUES (?, ?, ?) ON CONFLICT("key") DO UPDATE SET value = excluded.value,
		updated_at = excluded.updated_at`, key, value, now)
	return err
}

var _ = sqlite.Statement{}
