package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// SyncSettings 保存 WebDAV 云端同步与跨设备协作的非敏感配置。
// 密码等敏感数据保存在系统凭据库 (mygo.Secrets)，不保存在此表。
type SyncSettings struct {
	WebDAVURL       string
	Username        string
	RemoteDir       string
	DeviceID        string
	DeviceName      string
	EncryptEnabled  bool
	RemindOnExit    bool
	LastSyncTime    string
	LastSyncRev     int64
	LastPackageHash string
	LastSyncDevice  string
}

func syncSchema() []sqlite.Statement {
	return []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS sync_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			webdav_url TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			remote_dir TEXT NOT NULL DEFAULT 'Tally',
			device_id TEXT NOT NULL DEFAULT '',
			device_name TEXT NOT NULL DEFAULT '',
			encrypt_enabled INTEGER NOT NULL DEFAULT 1,
			remind_on_exit INTEGER NOT NULL DEFAULT 0,
			last_sync_time TEXT NOT NULL DEFAULT '',
			last_sync_rev INTEGER NOT NULL DEFAULT 0,
			last_package_hash TEXT NOT NULL DEFAULT '',
			last_sync_device TEXT NOT NULL DEFAULT ''
		)`},
	}
}

// GetSyncSettings 读取本地同步设置；首次调用时初始化默认记录与设备标识。
func (s *Store) GetSyncSettings(ctx context.Context) (SyncSettings, error) {
	rows, err := s.db.Query(ctx, `SELECT webdav_url, username, remote_dir, device_id, device_name,
		encrypt_enabled, remind_on_exit, last_sync_time, last_sync_rev, last_package_hash, last_sync_device
		FROM sync_settings WHERE id = 1`)
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
			return def, nil
		}
		return def, nil
	}

	row := rows.Values[0]
	webdavURL, _ := row[0].(string)
	username, _ := row[1].(string)
	remoteDir, _ := row[2].(string)
	deviceID, _ := row[3].(string)
	deviceName, _ := row[4].(string)
	encryptVal, _ := row[5].(int64)
	remindVal, _ := row[6].(int64)
	lastSyncTime, _ := row[7].(string)
	lastSyncRev, _ := row[8].(int64)
	lastPkgHash, _ := row[9].(string)
	lastSyncDevice, _ := row[10].(string)

	if remoteDir == "" {
		remoteDir = "Tally"
	}
	if deviceID == "" {
		deviceID = generateDeviceID()
	}

	return SyncSettings{
		WebDAVURL:       webdavURL,
		Username:        username,
		RemoteDir:       remoteDir,
		DeviceID:        deviceID,
		DeviceName:      deviceName,
		EncryptEnabled:  encryptVal != 0,
		RemindOnExit:    remindVal != 0,
		LastSyncTime:    lastSyncTime,
		LastSyncRev:     lastSyncRev,
		LastPackageHash: lastPkgHash,
		LastSyncDevice:  lastSyncDevice,
	}, nil
}

// SaveSyncSettings 保存或更新同步设置。
func (s *Store) SaveSyncSettings(ctx context.Context, settings SyncSettings) error {
	if settings.RemoteDir == "" {
		settings.RemoteDir = "Tally"
	}
	if settings.DeviceID == "" {
		settings.DeviceID = generateDeviceID()
	}
	encVal := 0
	if settings.EncryptEnabled {
		encVal = 1
	}
	remindVal := 0
	if settings.RemindOnExit {
		remindVal = 1
	}

	_, err := s.db.Execute(ctx, `INSERT INTO sync_settings (
		id, webdav_url, username, remote_dir, device_id, device_name,
		encrypt_enabled, remind_on_exit, last_sync_time, last_sync_rev, last_package_hash, last_sync_device
	) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		webdav_url = excluded.webdav_url,
		username = excluded.username,
		remote_dir = excluded.remote_dir,
		device_id = excluded.device_id,
		device_name = excluded.device_name,
		encrypt_enabled = excluded.encrypt_enabled,
		remind_on_exit = excluded.remind_on_exit,
		last_sync_time = excluded.last_sync_time,
		last_sync_rev = excluded.last_sync_rev,
		last_package_hash = excluded.last_package_hash,
		last_sync_device = excluded.last_sync_device`,
		settings.WebDAVURL, settings.Username, settings.RemoteDir,
		settings.DeviceID, settings.DeviceName, encVal, remindVal,
		settings.LastSyncTime, settings.LastSyncRev, settings.LastPackageHash, settings.LastSyncDevice,
	)
	return err
}

// GetDatabaseID 获取当前数据库全局唯一标识；若无则自动生成并持久化。
func (s *Store) GetDatabaseID(ctx context.Context) (string, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_settings WHERE key = 'database_id'`)
	if err != nil {
		return "", err
	}
	if len(rows.Values) > 0 {
		if val, ok := rows.Values[0][0].(string); ok && val != "" {
			return val, nil
		}
	}
	dbID := generateDeviceID()
	_, err = s.db.Execute(ctx, `INSERT INTO app_settings(key, value) VALUES('database_id', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, dbID)
	if err != nil {
		return "", err
	}
	return dbID, nil
}

// SetDatabaseID 强制指定数据库标识（例如拉取远端覆盖后对齐标识）。
func (s *Store) SetDatabaseID(ctx context.Context, id string) error {
	_, err := s.db.Execute(ctx, `INSERT INTO app_settings(key, value) VALUES('database_id', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, id)
	return err
}

// GetGlobalRevision 计算当前数据库的全局修订号与各批次/数据作用域的修订号映射。
func (s *Store) GetGlobalRevision(ctx context.Context) (int64, map[string]int64, error) {
	batchRevs := make(map[string]int64)
	var totalRev int64

	// 1. 查询 batches
	rows, err := s.db.Query(ctx, `SELECT id, revision FROM batches`)
	if err == nil {
		for _, row := range rows.Values {
			if id, ok := row[0].(int64); ok {
				rev, _ := row[1].(int64)
				key := fmt.Sprintf("batch-%d", id)
				batchRevs[key] = rev
				totalRev += rev
			}
		}
	}

	// 2. 查询 assessments
	rows, err = s.db.Query(ctx, `SELECT exam_cohort_id, revision FROM assessments`)
	if err == nil {
		for _, row := range rows.Values {
			if id, ok := row[0].(int64); ok {
				rev, _ := row[1].(int64)
				key := fmt.Sprintf("assessment-%d", id)
				batchRevs[key] = rev
				totalRev += rev
			}
		}
	}

	// 3. 查询 data_revisions
	rows, err = s.db.Query(ctx, `SELECT scope_type, scope_id, revision FROM data_revisions`)
	if err == nil {
		for _, row := range rows.Values {
			scopeType, _ := row[0].(string)
			scopeID, _ := row[1].(int64)
			rev, _ := row[2].(int64)
			key := fmt.Sprintf("data:%s:%d", scopeType, scopeID)
			batchRevs[key] = rev
			totalRev += rev
		}
	}

	// 4. cohorts 计数与最大ID
	rows, err = s.db.Query(ctx, `SELECT COUNT(*), COALESCE(MAX(id), 0) FROM cohorts`)
	if err == nil && len(rows.Values) > 0 {
		cnt, _ := rows.Values[0][0].(int64)
		maxID, _ := rows.Values[0][1].(int64)
		totalRev += cnt + maxID
		if cnt > 0 {
			batchRevs["cohorts"] = cnt + maxID
		}
	}

	// 5. exams 计数与最大ID
	rows, err = s.db.Query(ctx, `SELECT COUNT(*), COALESCE(MAX(id), 0) FROM exams`)
	if err == nil && len(rows.Values) > 0 {
		cnt, _ := rows.Values[0][0].(int64)
		maxID, _ := rows.Values[0][1].(int64)
		totalRev += cnt + maxID
		if cnt > 0 {
			batchRevs["exams"] = cnt + maxID
		}
	}

	// 6. 查询 plans 版本与方案计数
	rows, err = s.db.Query(ctx, `SELECT COUNT(*), COALESCE(MAX(version), 0) FROM plan_versions`)
	if err == nil && len(rows.Values) > 0 {
		cnt, _ := rows.Values[0][0].(int64)
		maxVer, _ := rows.Values[0][1].(int64)
		totalRev += cnt + maxVer
	}

	return totalRev, batchRevs, nil
}

func generateDeviceID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("dev-%d", time.Now().UnixNano()%1000000)
	}
	hostname, _ := os.Hostname()
	cleanHost := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, hostname)
	if len(cleanHost) > 12 {
		cleanHost = cleanHost[:12]
	}
	if cleanHost == "" {
		cleanHost = "device"
	}
	return fmt.Sprintf("%s-%s", cleanHost, hex.EncodeToString(b[:4]))
}
