package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
	"tally/internal/store"
	syncpkg "tally/internal/sync"
)

func TestSettingsSyncPageUI(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.settingsView, 800, 700)
	tt.SetLocale("en")
	tt.Frame()

	// 切换到 Cloud Sync 页面
	if err := tt.Click("Cloud Sync"); err != nil {
		t.Fatalf("clicking Cloud Sync failed: %v", err)
	}
	tt.Frame()

	if a.settingsPageID != "sync" {
		t.Fatalf("expected settingsPageID = sync, got %q", a.settingsPageID)
	}

	for _, text := range []string{
		"WebDAV Server URL",
		"Username",
		"Password",
		"Remote Directory",
		"Device Name",
		"Package Encryption",
		"Sync Password",
		"Test Connection",
		"Sync Status",
		"Upload to Cloud",
		"Pull from Cloud",
	} {
		if !tt.HasText(text) {
			t.Errorf("expected text %q on sync settings page", text)
		}
	}
}

func TestAppSyncOperationsWithMockWebDAV(t *testing.T) {
	// 建立模拟 WebDAV 服务器
	var mu sync.Mutex
	files := make(map[string][]byte)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		path := r.URL.Path
		switch r.Method {
		case "PROPFIND":
			if _, ok := files[path]; ok || path == "/Tally/" || path == "/Tally/packages/" || path == "/Tally/files/" || path == "/" {
				w.WriteHeader(http.StatusMultiStatus)
				_, _ = w.Write([]byte(`<D:multistatus xmlns:D="DAV:"/>`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case "OPTIONS":
			w.Header().Set("DAV", "1, 2")
			w.WriteHeader(http.StatusOK)
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			files[path] = b
			w.WriteHeader(http.StatusCreated)
		case "GET":
			if b, ok := files[path]; ok {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(b)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case "MOVE":
			dest := r.Header.Get("Destination")
			dest = strings.TrimPrefix(dest, "http://"+r.Host)
			if b, ok := files[path]; ok {
				files[dest] = b
				delete(files, path)
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case "DELETE":
			delete(files, path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	a := newTestApp(t)
	a.syncSettings = store.SyncSettings{
		WebDAVURL:      ts.URL + "/Tally/",
		Username:       "testuser",
		RemoteDir:      "Tally",
		DeviceID:       "test-dev",
		DeviceName:     "Test PC",
		EncryptEnabled: true,
	}
	a.syncWebDAVPassword = "webdavpass"
	a.syncPackagePassword = "syncpass"

	// 1. 测试保存
	a.saveSyncSettings()
	if a.syncError != "" {
		t.Fatalf("saveSyncSettings failed: %v", a.syncError)
	}

	// 2. 测试 Ping
	a.testWebDAVConnection()
	if a.syncError != "" {
		t.Fatalf("testWebDAVConnection failed: %v", a.syncError)
	}

	// 3. 测试上传
	a.startSyncUpload()
	if a.syncError != "" {
		t.Fatalf("startSyncUpload failed: %v", a.syncError)
	}
	if a.syncSettings.LastSyncRev == 0 {
		t.Error("expected LastSyncRev > 0 after upload")
	}

	// 4. 测试检查远端
	a.checkRemoteSync()
	if a.syncError != "" {
		t.Fatalf("checkRemoteSync failed: %v", a.syncError)
	}
	if a.syncRemoteStatus == nil || a.syncRemoteStatus.Kind != syncpkg.StatusUpToDate {
		t.Errorf("expected StatusUpToDate after fresh upload, got %v", a.syncRemoteStatus)
	}

	// 5. 模拟另一个实例拉取
	a2 := newTestApp(t)
	a2.syncSettings = store.SyncSettings{
		WebDAVURL:      ts.URL + "/Tally/",
		Username:       "testuser",
		RemoteDir:      "Tally",
		DeviceID:       "home-dev",
		DeviceName:     "Home PC",
		EncryptEnabled: true,
	}
	a2.syncWebDAVPassword = "webdavpass"
	a2.syncPackagePassword = "syncpass"

	a2.startSyncPull(true)
	if a2.syncError != "" {
		t.Fatalf("a2 startSyncPull failed: %v", a2.syncError)
	}
	if a2.syncSettings.LastSyncRev != a.syncSettings.LastSyncRev {
		t.Errorf("a2 rev mismatch: got %d, want %d", a2.syncSettings.LastSyncRev, a.syncSettings.LastSyncRev)
	}

	// 6. 模拟冲突场景
	// a 端修改数据
	_, err := a.store.CreateCohort(context.Background(), 2030, 2030)
	if err != nil {
		t.Fatal(err)
	}
	// a 上传新版本
	a.startSyncUpload()
	if a.syncError != "" {
		t.Fatalf("a second upload failed: %v", a.syncError)
	}

	// a2 在本地也做了修改（导致分叉）
	_, err = a2.store.CreateCohort(context.Background(), 2031, 2031)
	if err != nil {
		t.Fatal(err)
	}

	// a2 检查远端应检测到 conflict
	a2.checkRemoteSync()
	if a2.syncRemoteStatus == nil || a2.syncRemoteStatus.Kind != syncpkg.StatusConflict {
		t.Errorf("expected StatusConflict on a2, got %v", a2.syncRemoteStatus)
	}
	if !a2.syncConflictOpen {
		t.Error("expected syncConflictOpen to be true on conflict")
	}

	// 验证覆盖覆盖拉取 (force pull with auto backup)
	a2.startSyncPull(true)
	if a2.syncError != "" {
		t.Fatalf("a2 force pull failed: %v", a2.syncError)
	}
	// 验证 a2 本地备份增加
	backups, err := store.ListBackups(filepath.Dir(filepath.Join(a2.dataDir, "tally.db")))
	if err != nil || len(backups) == 0 {
		t.Errorf("expected auto backup before overwrite, got %v", backups)
	}
}
