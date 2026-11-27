package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/store"
)

// syncDevice 模拟一台本地桌面设备（独立数据目录、真实 SQLite 库、files/ 附件与本地同步状态）。
type syncDevice struct {
	name            string
	deviceID        string
	dataDir         string
	filesDir        string
	dbPath          string
	dbID            string
	lastSyncRev     int64
	lastPackageHash string
}

func newSyncDevice(t *testing.T, name, deviceID string) *syncDevice {
	t.Helper()
	dataDir := t.TempDir()
	filesDir := filepath.Join(dataDir, "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		t.Fatalf("创建设备 %s files 目录失败: %v", name, err)
	}
	dbPath := filepath.Join(dataDir, "tally.db")

	ctx := context.Background()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("打开设备 %s SQLite 数据库失败: %v", name, err)
	}
	dbID, err := st.GetDatabaseID(ctx)
	if err != nil {
		_ = st.Close()
		t.Fatalf("获取设备 %s DatabaseID 失败: %v", name, err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关闭设备 %s SQLite 数据库失败: %v", name, err)
	}

	return &syncDevice{
		name:     name,
		deviceID: deviceID,
		dataDir:  dataDir,
		filesDir: filesDir,
		dbPath:   dbPath,
		dbID:     dbID,
	}
}

func (d *syncDevice) openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), d.dbPath)
	if err != nil {
		t.Fatalf("打开设备 %s store 失败: %v", d.name, err)
	}
	return st
}

func (d *syncDevice) readRevisionState(t *testing.T) (string, int64, map[string]int64) {
	t.Helper()
	ctx := context.Background()
	st := d.openStore(t)
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("关闭设备 %s store 失败: %v", d.name, err)
		}
	}()
	dbID, err := st.GetDatabaseID(ctx)
	if err != nil {
		t.Fatalf("读取 DatabaseID 失败: %v", err)
	}
	rev, batches, err := st.GetGlobalRevision(ctx)
	if err != nil {
		t.Fatalf("读取 GlobalRevision 失败: %v", err)
	}
	d.dbID = dbID
	return dbID, rev, batches
}

func (d *syncDevice) addCohort(t *testing.T, enrollmentYear, academicYearStart int) {
	t.Helper()
	ctx := context.Background()
	st := d.openStore(t)
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("关闭设备 %s store 失败: %v", d.name, err)
		}
	}()
	if _, err := st.CreateCohort(ctx, enrollmentYear, academicYearStart); err != nil {
		t.Fatalf("设备 %s 创建年级 %d 失败: %v", d.name, enrollmentYear, err)
	}
}

func (d *syncDevice) listCohorts(t *testing.T) []int {
	t.Helper()
	ctx := context.Background()
	st := d.openStore(t)
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("关闭设备 %s store 失败: %v", d.name, err)
		}
	}()
	cohorts, err := st.ListCohorts(ctx)
	if err != nil {
		t.Fatalf("设备 %s 查询年级失败: %v", d.name, err)
	}
	years := make([]int, 0, len(cohorts))
	for _, c := range cohorts {
		years = append(years, c.EnrollmentYear)
	}
	return years
}

func (d *syncDevice) writeAttachment(t *testing.T, filename string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(d.filesDir, filename), content, 0o644); err != nil {
		t.Fatalf("写入附件 %s 失败: %v", filename, err)
	}
}

func (d *syncDevice) snapshotDBBytes(t *testing.T) []byte {
	t.Helper()
	backupDir, err := store.CreateBackup(d.dataDir)
	if err != nil {
		t.Fatalf("创建设备 %s 快照备份失败: %v", d.name, err)
	}
	dbBytes, err := os.ReadFile(filepath.Join(backupDir, "tally.db"))
	if err != nil {
		t.Fatalf("读取设备 %s 快照字节失败: %v", d.name, err)
	}
	return dbBytes
}

func (d *syncDevice) checkRemote(t *testing.T, client *Client, remoteDir string) *RemoteStatus {
	t.Helper()
	dbID, localRev, batches := d.readRevisionState(t)
	status, err := CheckRemote(context.Background(), client, remoteDir, dbID, localRev, d.lastSyncRev, d.lastPackageHash, batches)
	if err != nil {
		t.Fatalf("设备 %s CheckRemote 失败: %v", d.name, err)
	}
	return status
}

func (d *syncDevice) upload(t *testing.T, client *Client, remoteDir, password string) *SyncManifest {
	t.Helper()
	dbID, localRev, batches := d.readRevisionState(t)
	dbBytes := d.snapshotDBBytes(t)
	manifest, err := UploadSync(context.Background(), client, remoteDir, dbBytes, d.filesDir, SyncManifest{
		DatabaseID:     dbID,
		Revision:       localRev,
		BaseRevision:   d.lastSyncRev,
		DeviceID:       d.deviceID,
		DeviceName:     d.name,
		BatchRevisions: batches,
	}, true, password)
	if err != nil {
		t.Fatalf("设备 %s UploadSync 失败: %v", d.name, err)
	}
	d.lastSyncRev = manifest.Revision
	d.lastPackageHash = manifest.PackageHash
	return manifest
}

func verifySQLiteIntegrity(t *testing.T, dbPath string) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, dbPath, sqlite.OpenOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("打开 SQLite 做完整性检查失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("关闭校验 SQLite 连接失败: %v", err)
		}
	}()
	rows, err := db.Query(ctx, "PRAGMA integrity_check")
	if err != nil || len(rows.Values) == 0 || rows.Values[0][0] != "ok" {
		t.Fatalf("SQLite 完整性检查未通过: err=%v, rows=%v", err, rows.Values)
	}
}

func (d *syncDevice) pullAndApply(t *testing.T, client *Client, remoteDir, password string) (*SyncManifest, string) {
	t.Helper()
	ctx := context.Background()
	tempDir := t.TempDir()
	tempDB := filepath.Join(tempDir, "tally.db")

	manifest, err := DownloadSync(ctx, client, remoteDir, password, tempDB, d.filesDir)
	if err != nil {
		t.Fatalf("设备 %s DownloadSync 失败: %v", d.name, err)
	}
	verifySQLiteIntegrity(t, tempDB)

	tempBytes, err := os.ReadFile(tempDB)
	if err != nil {
		t.Fatalf("读取临时解压数据库失败: %v", err)
	}

	// 替换前先自动备份当前数据库。
	backupDir, err := store.CreateBackup(d.dataDir)
	if err != nil {
		t.Fatalf("拉取替换前备份失败: %v", err)
	}
	if err := os.WriteFile(d.dbPath, tempBytes, 0o644); err != nil {
		t.Fatalf("写入目标数据库失败: %v", err)
	}

	st := d.openStore(t)
	if err := st.SetDatabaseID(ctx, manifest.DatabaseID); err != nil {
		_ = st.Close()
		t.Fatalf("对齐 DatabaseID 失败: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关闭对齐后数据库失败: %v", err)
	}

	d.dbID = manifest.DatabaseID
	d.lastSyncRev = manifest.Revision
	d.lastPackageHash = manifest.PackageHash
	return manifest, backupDir
}

func TestRFC4918ProtocolAndErrors(t *testing.T) {
	srv := newRFC4918Server("teacher", "dav-secret")
	defer srv.Close()
	ctx := context.Background()

	// 1. Ping: PROPFIND 401 时禁止被免鉴权的 OPTIONS 掩盖。
	badClient, err := NewClient(srv.URL(), "teacher", "wrong-pass")
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	if err := badClient.Ping(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("错误密码 Ping 应返回 ErrUnauthorized，实际: %v", err)
	}

	// 2. Ping: PROPFIND 405 时自动回退到 OPTIONS 并返回成功。
	srv.propfindMethodNotAllowed = true
	client, err := NewClient(srv.URL(), "teacher", "dav-secret")
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("PROPFIND 405 回退 OPTIONS 应成功，实际: %v", err)
	}
	srv.propfindMethodNotAllowed = false
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("PROPFIND 正常 Ping 应成功，实际: %v", err)
	}

	// 3. EnsureDir: 递归建目录与 405 已存在幂等。
	if err := client.EnsureDir(ctx, "Tally/nested/packages"); err != nil {
		t.Fatalf("首次 EnsureDir 失败: %v", err)
	}
	if !srv.hasDir("/Tally/nested/packages") {
		t.Fatal("服务端应已创建 /Tally/nested/packages")
	}
	// 再次执行相同路径，服务端对已存在目录返回 405，客户端应幂等成功。
	if err := client.EnsureDir(ctx, "Tally/nested/packages"); err != nil {
		t.Fatalf("重复 EnsureDir (405 幂等) 失败: %v", err)
	}

	// 4. 403 / 507 / 404 错误语义传递。
	srv.forbiddenPaths["/Tally/locked"] = true
	if err := client.EnsureDir(ctx, "Tally/locked/sub"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("403 目录应返回 ErrForbidden，实际: %v", err)
	}
	delete(srv.forbiddenPaths, "/Tally/locked")

	srv.insufficientStorage = true
	if err := client.EnsureDir(ctx, "Tally/quota-dir"); !errors.Is(err, ErrInsufficientStorage) {
		t.Fatalf("507 MKCOL 应返回 ErrInsufficientStorage，实际: %v", err)
	}
	if err := client.Put(ctx, "Tally/big.bin", []byte("123")); !errors.Is(err, ErrInsufficientStorage) {
		t.Fatalf("507 PUT 应返回 ErrInsufficientStorage，实际: %v", err)
	}
	srv.insufficientStorage = false

	if _, err := client.Get(ctx, "Tally/missing.json"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 GET 应返回 ErrNotFound，实际: %v", err)
	}

	// 5. Move: 校验 Destination 完整 URL 与 Overwrite: T 请求头。
	if err := client.Put(ctx, "Tally/latest.json", []byte(`{"revision":1}`)); err != nil {
		t.Fatalf("写入初始 latest.json 失败: %v", err)
	}
	if err := client.Put(ctx, "Tally/latest.json.tmp", []byte(`{"revision":2}`)); err != nil {
		t.Fatalf("写入 latest.json.tmp 失败: %v", err)
	}
	if err := client.Move(ctx, "Tally/latest.json.tmp", "Tally/latest.json"); err != nil {
		t.Fatalf("Move 覆盖失败: %v", err)
	}
	got, err := client.Get(ctx, "Tally/latest.json")
	if err != nil || string(got) != `{"revision":2}` {
		t.Fatalf("Move 后读取结果异常: got=%q, err=%v", got, err)
	}
	var sawMove bool
	for _, req := range srv.snapshotRequests() {
		if req.Method == "MOVE" {
			sawMove = true
			if req.Header.Get("Overwrite") != "T" {
				t.Fatalf("MOVE 缺少 Overwrite: T 请求头，实际: %q", req.Header.Get("Overwrite"))
			}
			if !strings.HasSuffix(req.Header.Get("Destination"), "/Tally/latest.json") {
				t.Fatalf("MOVE Destination 请求头格式异常: %q", req.Header.Get("Destination"))
			}
		}
	}
	if !sawMove {
		t.Fatal("未观测到 MOVE 请求")
	}
}

func TestUploadRollbackAndTempCleanup(t *testing.T) {
	srv := newRFC4918Server("teacher", "dav-secret")
	defer srv.Close()
	client, err := NewClient(srv.URL(), "teacher", "dav-secret")
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	ctx := context.Background()

	// 1. 回读校验哈希不匹配时，应自动删除远端数据包且不写入 latest.json。
	corruptedOnce := false
	srv.onGet = func(reqPath string, data []byte) ([]byte, int) {
		if strings.HasPrefix(reqPath, "/Tally/packages/") && !corruptedOnce {
			corruptedOnce = true
			tampered := append([]byte(nil), data...)
			tampered[len(tampered)-1] ^= 0xFF
			return tampered, 0
		}
		return nil, 0
	}
	_, err = UploadSync(ctx, client, "Tally", []byte("db-bytes"), "", SyncManifest{
		DatabaseID: "db-1",
		Revision:   1,
		DeviceID:   "dev-a",
		DeviceName: "设备 A",
	}, true, "pass-123")
	if err == nil {
		t.Fatal("回读哈希不匹配时 UploadSync 必须失败")
	}
	if srv.hasFile("/Tally/latest.json") {
		t.Fatal("回读失败后不得生成 latest.json")
	}
	srv.mu.Lock()
	for p := range srv.files {
		if strings.HasPrefix(p, "/Tally/packages/") {
			srv.mu.Unlock()
			t.Fatalf("回读失败后远端数据包 %s 应已回滚删除", p)
		}
	}
	srv.mu.Unlock()
	srv.onGet = nil

	// 2. MOVE 更新 latest.json 失败时，应清理 latest.json.tmp，且远端仍指向旧版本。
	if err := client.Put(ctx, "Tally/latest.json", []byte(`{"revision":1}`)); err != nil {
		t.Fatalf("预置旧 latest.json 失败: %v", err)
	}
	srv.onMove = func(srcPath, dstPath string) int {
		return 500
	}
	_, err = UploadSync(ctx, client, "Tally", []byte("db-bytes-v2"), "", SyncManifest{
		DatabaseID: "db-1",
		Revision:   2,
		DeviceID:   "dev-a",
		DeviceName: "设备 A",
	}, true, "pass-123")
	if err == nil {
		t.Fatal("MOVE 失败时 UploadSync 必须报错")
	}
	if srv.hasFile("/Tally/latest.json.tmp") {
		t.Fatal("MOVE 失败后应清理残留的 latest.json.tmp")
	}
	oldLatest, ok := srv.getRawFile("/Tally/latest.json")
	if !ok || string(oldLatest) != `{"revision":1}` {
		t.Fatalf("MOVE 失败后远端 latest.json 应保持旧版本，实际: %q", oldLatest)
	}
}

func TestTwoDeviceSyncAndConflictRegression(t *testing.T) {
	srv := newRFC4918Server("teacher", "dav-secret")
	defer srv.Close()

	client, err := NewClient(srv.URL(), "teacher", "dav-secret")
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	const remoteDir = "Tally"
	const syncPass = "SchoolSync@2026!"

	devA := newSyncDevice(t, "办公室电脑", "office-mac")
	devB := newSyncDevice(t, "家里电脑", "home-pc")

	// a) 设备 A 录入 2024 级与原始 Excel 附件，首次加密上传。
	devA.addCohort(t, 2024, 2024)
	attContent := []byte("PK-dummy-xlsx-content-for-2024-midterm")
	attHash := SHA256String(attContent)
	devA.writeAttachment(t, "2024_midterm.xlsx", attContent)

	if st := devA.checkRemote(t, client, remoteDir); st.Kind != StatusRemoteEmpty {
		t.Fatalf("首次上传前远端应为空，实际: %v", st.Kind)
	}
	mA1 := devA.upload(t, client, remoteDir, syncPass)
	if !srv.hasFile("/Tally/files/" + attHash + ".xlsx.enc") {
		t.Fatal("远端应存在加密后的原始附件")
	}

	// b) 设备 B 首次拉取解密、校验明文 SHA-256 与 SQLite 完整性并成功恢复。
	if st := devB.checkRemote(t, client, remoteDir); st.Kind != StatusCanPull {
		t.Fatalf("设备 B 首次检查应为 StatusCanPull，实际: %v", st.Kind)
	}
	devB.pullAndApply(t, client, remoteDir, syncPass)

	cohortsB := devB.listCohorts(t)
	if len(cohortsB) != 1 || cohortsB[0] != 2024 {
		t.Fatalf("设备 B 拉取后年级数据不匹配: %v", cohortsB)
	}
	gotAttB, err := os.ReadFile(filepath.Join(devB.filesDir, attHash+".xlsx"))
	if err != nil || string(gotAttB) != string(attContent) {
		t.Fatalf("设备 B 恢复附件内容不匹配: err=%v, got=%q", err, gotAttB)
	}

	// c) 无变更重复同步：两边均应识别为 StatusUpToDate，不重复上传。
	if st := devA.checkRemote(t, client, remoteDir); st.Kind != StatusUpToDate {
		t.Fatalf("设备 A 无变更应为 StatusUpToDate，实际: %v", st.Kind)
	}
	if st := devB.checkRemote(t, client, remoteDir); st.Kind != StatusUpToDate {
		t.Fatalf("设备 B 无变更应为 StatusUpToDate，实际: %v", st.Kind)
	}

	// d) 设备 B 新增数据单向更新上传 -> 设备 A 增量拉取同步。
	devB.addCohort(t, 2025, 2025)
	att2Content := []byte("PK-dummy-xlsx-content-for-2025-entry")
	att2Hash := SHA256String(att2Content)
	devB.writeAttachment(t, "2025_entry.xlsx", att2Content)

	// e) 同步前先验证「本地修订号领先于远端」时的提示与保护 (StatusCanUpload)。
	if st := devB.checkRemote(t, client, remoteDir); st.Kind != StatusCanUpload {
		t.Fatalf("设备 B 修改后领先于远端应为 StatusCanUpload，实际: %v", st.Kind)
	}
	mB2 := devB.upload(t, client, remoteDir, syncPass)
	if mB2.BaseRevision != mA1.Revision {
		t.Fatalf("设备 B 上传的 BaseRevision 应为 %d，实际: %d", mA1.Revision, mB2.BaseRevision)
	}

	if st := devA.checkRemote(t, client, remoteDir); st.Kind != StatusCanPull {
		t.Fatalf("设备 A 检查远端应为 StatusCanPull，实际: %v", st.Kind)
	}
	devA.pullAndApply(t, client, remoteDir, syncPass)
	cohortsA := devA.listCohorts(t)
	if len(cohortsA) != 2 {
		t.Fatalf("设备 A 增量拉取后应有 2 个年级，实际: %v", cohortsA)
	}
	if _, err := os.ReadFile(filepath.Join(devA.filesDir, att2Hash+".xlsx")); err != nil {
		t.Fatalf("设备 A 增量拉取后缺少新附件: %v", err)
	}

	// f) 设备 A 与设备 B 各自离线修改产生修订号分叉（冲突检测）。
	// 设备 A 离线修改 1 个年级，设备 B 离线修改 2 个年级后先上传，导致设备 A 本地修订号落后但已相对上次同步发生改动。
	devA.addCohort(t, 2023, 2025)
	devB.addCohort(t, 2022, 2025)
	devB.addCohort(t, 2021, 2025)
	devB.upload(t, client, remoteDir, syncPass)

	// 设备 A 检查远端：尽管远端 Revision > 本地 Revision 且远端 BaseRevision == lastSyncRev，
	// 由于设备 A 本地也已修改，必须检测为 StatusConflict，禁止静默覆盖！
	conflictStatus := devA.checkRemote(t, client, remoteDir)
	if conflictStatus.Kind != StatusConflict {
		t.Fatalf("双端各自离线修改分叉时应返回 StatusConflict，实际: %v", conflictStatus.Kind)
	}
	if len(conflictStatus.ConflictedBatches) == 0 {
		t.Fatal("冲突状态应列出存在差异的批次/作用域")
	}

	// 用户选择「取消，保留本地」：设备 A 数据保持包含 2023 级（共 3 个年级），不被远端覆盖。
	cohortsABeforeResolve := devA.listCohorts(t)
	if len(cohortsABeforeResolve) != 3 {
		t.Fatalf("保留本地时设备 A 应保持 3 个年级，实际: %v", cohortsABeforeResolve)
	}

	// 用户显式选择「使用远端的数据」：先自动备份本地数据库（保留 2023 级），再用远端覆盖恢复（共 4 个年级）。
	_, backupDir := devA.pullAndApply(t, client, remoteDir, syncPass)
	cohortsAAfterResolve := devA.listCohorts(t)
	if len(cohortsAAfterResolve) != 4 {
		t.Fatalf("选择使用远端后设备 A 应有 4 个年级，实际: %v", cohortsAAfterResolve)
	}
	// 验证自动备份的本地数据库可随时打开并仍保留冲突前的 3 个年级。
	backupStore, err := store.Open(context.Background(), filepath.Join(backupDir, "tally.db"))
	if err != nil {
		t.Fatalf("打开冲突前备份数据库失败: %v", err)
	}
	backupCohorts, err := backupStore.ListCohorts(context.Background())
	if closeErr := backupStore.Close(); closeErr != nil {
		t.Fatalf("关闭备份数据库失败: %v", closeErr)
	}
	if err != nil || len(backupCohorts) != 3 {
		t.Fatalf("冲突前备份应完整保留 3 个年级，实际: len=%d, err=%v", len(backupCohorts), err)
	}

	// g) 补充冲突分支：双端同修订号分叉 (localRev == remoteRev) 与本地领先但远端亦被修改 (localRev > remoteRev)。
	devA.addCohort(t, 2020, 2025)
	devB.addCohort(t, 2019, 2025)
	devB.upload(t, client, remoteDir, syncPass)
	// 此时 devA 与 devB 均从上次同步点各自新增 1 个年级，修订号相同但内容已分叉。
	if st := devA.checkRemote(t, client, remoteDir); st.Kind != StatusConflict {
		t.Fatalf("双端同修订号分叉时应返回 StatusConflict，实际: %v", st.Kind)
	}

	// devA 继续新增 1 个年级使其 localRev > remoteRev，但 remoteRev != lastSyncRev。
	devA.addCohort(t, 2018, 2025)
	if st := devA.checkRemote(t, client, remoteDir); st.Kind != StatusConflict {
		t.Fatalf("本地领先且远端亦相对上次同步点有改动时应返回 StatusConflict，实际: %v", st.Kind)
	}

	// 用户显式选择「保留本地」并上传覆盖远端，随后 devB 拉取恢复双端一致。
	devA.upload(t, client, remoteDir, syncPass)
	if st := devB.checkRemote(t, client, remoteDir); st.Kind != StatusCanPull {
		t.Fatalf("devA 显式保留本地并上传后 devB 应可拉取，实际: %v", st.Kind)
	}
	devB.pullAndApply(t, client, remoteDir, syncPass)
	if len(devB.listCohorts(t)) != len(devA.listCohorts(t)) {
		t.Fatalf("冲突闭环恢复后双端年级数应一致: devA=%d, devB=%d", len(devA.listCohorts(t)), len(devB.listCohorts(t)))
	}
}

func TestCrossDatabaseIDConflictProtection(t *testing.T) {
	srv := newRFC4918Server("teacher", "dav-secret")
	defer srv.Close()
	client, err := NewClient(srv.URL(), "teacher", "dav-secret")
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	const remoteDir = "Tally"
	const syncPass = "SchoolSync@2026!"

	// 设备 A 创建 2 个年级并上传（Revision 随年级增长，BaseRevision=0；手动构造与设备 B 相同 Revision 场景）。
	devA := newSyncDevice(t, "设备A", "dev-a")
	devA.addCohort(t, 2024, 2024)
	devA.addCohort(t, 2025, 2025)
	mA := devA.upload(t, client, remoteDir, syncPass)

	// 独立建库的设备 B 从未同步过 (lastSyncRev=0)，且本地已建立 1 个年级 (localBatches 非空)。
	devB := newSyncDevice(t, "设备B", "dev-b")
	devB.addCohort(t, 2023, 2024)
	_, revB, batchesB := devB.readRevisionState(t)

	// 即使远端 BaseRevision 恰好等于 devB 的 localRev，跨 DatabaseID 也不得静默覆盖。
	rawLatest, ok := srv.getRawFile("/Tally/latest.json")
	if !ok {
		t.Fatal("远端缺少 latest.json")
	}
	var mAdjusted SyncManifest
	if err := json.Unmarshal(rawLatest, &mAdjusted); err != nil {
		t.Fatalf("解析 latest.json 失败: %v", err)
	}
	mAdjusted.BaseRevision = revB
	mAdjusted.Revision = revB + 4
	adjustedBytes, err := json.Marshal(mAdjusted)
	if err != nil {
		t.Fatalf("序列化 latest.json 失败: %v", err)
	}
	srv.setRawFile("/Tally/latest.json", adjustedBytes)

	status, err := CheckRemote(context.Background(), client, remoteDir, devB.dbID, revB, 0, "", batchesB)
	if err != nil {
		t.Fatalf("CheckRemote 失败: %v", err)
	}
	if status.Kind != StatusConflict {
		t.Fatalf("跨 DatabaseID 且本地已有业务数据时应返回 StatusConflict，实际: %v (mA.Rev=%d)", status.Kind, mA.Revision)
	}

	// 新设备即便 localBatches 为空，但若在本地修改/新增了考核方案 (localRev > 2)，同样必须拦截为 StatusConflict。
	devC := newSyncDevice(t, "设备C", "dev-c")
	statusC, err := CheckRemote(context.Background(), client, remoteDir, devC.dbID, 3, 0, "", nil)
	if err != nil {
		t.Fatalf("CheckRemote 失败: %v", err)
	}
	if statusC.Kind != StatusConflict {
		t.Fatalf("跨 DatabaseID 且 localRev > 2 时应返回 StatusConflict，实际: %v", statusC.Kind)
	}
}

func TestEncryptionSecurityAndTamperProtection(t *testing.T) {
	srv := newRFC4918Server("teacher", "dav-secret")
	defer srv.Close()
	client, err := NewClient(srv.URL(), "teacher", "dav-secret")
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	ctx := context.Background()
	const remoteDir = "Tally"
	const rightPass = "RightSyncPass!99"

	devA := newSyncDevice(t, "设备A", "dev-a")
	devA.addCohort(t, 2024, 2024)
	attContent := []byte("PK-dummy-xlsx-secret-attachment")
	attHash := SHA256String(attContent)
	devA.writeAttachment(t, "secret_scores.xlsx", attContent)
	manifest := devA.upload(t, client, remoteDir, rightPass)

	// 准备设备 B 本地已有数据库与备份保护。
	devB := newSyncDevice(t, "设备B", "dev-b")
	devB.addCohort(t, 2023, 2024)
	origBytesB, err := os.ReadFile(devB.dbPath)
	if err != nil {
		t.Fatalf("读取设备 B 原始库失败: %v", err)
	}

	// 1. 错误同步密码拒绝解密，不写入目标临时文件，不破坏本地库。
	badTempDB := filepath.Join(t.TempDir(), "tally.db")
	if _, err := DownloadSync(ctx, client, remoteDir, "WrongPass!", badTempDB, devB.filesDir); err == nil {
		t.Fatal("错误同步密码必须拒绝解密")
	}
	if _, err := os.Stat(badTempDB); !os.IsNotExist(err) {
		t.Fatal("解密失败时不得产出临时数据库文件")
	}

	// 2. 篡改 latest.json 中的 PackageHash，拦截损坏包。
	rawLatest, ok := srv.getRawFile("/Tally/latest.json")
	if !ok {
		t.Fatal("远端缺少 latest.json")
	}
	var tamperedManifest SyncManifest
	if err := json.Unmarshal(rawLatest, &tamperedManifest); err != nil {
		t.Fatalf("解析 latest.json 失败: %v", err)
	}
	tamperedManifest.PackageHash = strings.Repeat("0", 64)
	tamperedLatestBytes, err := json.Marshal(tamperedManifest)
	if err != nil {
		t.Fatalf("序列化篡改 manifest 失败: %v", err)
	}
	srv.setRawFile("/Tally/latest.json", tamperedLatestBytes)
	if _, err := DownloadSync(ctx, client, remoteDir, rightPass, badTempDB, devB.filesDir); err == nil {
		t.Fatal("latest.json 哈希被篡改时必须拦截下载")
	}
	// 还原 latest.json。
	srv.setRawFile("/Tally/latest.json", rawLatest)

	// 3. 篡改远端加密附件密文时，DownloadSync 必须阻断报错且不写入损坏附件。
	attRemotePath := "/Tally/files/" + attHash + ".xlsx.enc"
	origAttEnc, ok := srv.getRawFile(attRemotePath)
	if !ok {
		t.Fatal("远端缺少加密附件")
	}
	tamperedAttEnc := append([]byte(nil), origAttEnc...)
	tamperedAttEnc[len(tamperedAttEnc)-1] ^= 0xFF
	srv.setRawFile(attRemotePath, tamperedAttEnc)
	if _, err := DownloadSync(ctx, client, remoteDir, rightPass, filepath.Join(t.TempDir(), "tally.db"), devB.filesDir); err == nil {
		t.Fatal("远端加密附件被篡改时 DownloadSync 必须报错拦截")
	}
	if _, err := os.Stat(filepath.Join(devB.filesDir, attHash+".xlsx")); !os.IsNotExist(err) {
		t.Fatal("被篡改的附件不得写入本地 files 目录")
	}
	srv.setRawFile(attRemotePath, origAttEnc)

	// 4. 篡改 zip 包内的密文与 manifest（重新计算 latest.json 的 PackageHash 绕过外层校验，测试内层 AEAD + SHA-256 校验）。
	pkgRemotePath := "/Tally/packages/" + manifest.PackageName
	origPkg, ok := srv.getRawFile(pkgRemotePath)
	if !ok {
		t.Fatal("远端缺少数据包")
	}
	tamperedPkg := rewriteZipEntry(t, origPkg, "tally.db.enc", func(b []byte) []byte {
		out := append([]byte(nil), b...)
		out[len(out)-1] ^= 0x55
		return out
	})
	srv.setRawFile(pkgRemotePath, tamperedPkg)
	var m2 SyncManifest
	if err := json.Unmarshal(rawLatest, &m2); err != nil {
		t.Fatalf("解析 latest.json 失败: %v", err)
	}
	m2.PackageHash = SHA256String(tamperedPkg)
	m2Bytes, err := json.Marshal(m2)
	if err != nil {
		t.Fatalf("序列化 m2 失败: %v", err)
	}
	srv.setRawFile("/Tally/latest.json", m2Bytes)

	if _, err := DownloadSync(ctx, client, remoteDir, rightPass, badTempDB, devB.filesDir); err == nil {
		t.Fatal("密文被篡改时必须被 AES-GCM 认证标签拦截")
	}

	// 5. 确认设备 B 本地数据库完好无损。
	curBytesB, err := os.ReadFile(devB.dbPath)
	if err != nil || !bytes.Equal(origBytesB, curBytesB) {
		t.Fatal("异常数据包不得覆盖或破坏本地数据库")
	}
	verifySQLiteIntegrity(t, devB.dbPath)
}

func rewriteZipEntry(t *testing.T, zipBytes []byte, targetName string, mutator func([]byte) []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("打开 zip 失败: %v", err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("读取 zip 条目 %s 失败: %v", f.Name, err)
		}
		var data bytes.Buffer
		_, readErr := data.ReadFrom(rc)
		if closeErr := rc.Close(); closeErr != nil {
			t.Fatalf("关闭 zip 条目 %s 失败: %v", f.Name, closeErr)
		}
		if readErr != nil {
			t.Fatalf("复制 zip 条目 %s 失败: %v", f.Name, readErr)
		}

		content := data.Bytes()
		if f.Name == targetName {
			content = mutator(content)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatalf("写入 zip 条目 %s 失败: %v", f.Name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("写入 zip 条目内容 %s 失败: %v", f.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip writer 失败: %v", err)
	}
	return buf.Bytes()
}
