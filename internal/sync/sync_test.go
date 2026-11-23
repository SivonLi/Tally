package sync

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPackageCreationAndExtractionUnencrypted(t *testing.T) {
	tempDir := t.TempDir()
	filesDir := filepath.Join(tempDir, "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "test.xlsx"), []byte("spreadsheet data"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbData := []byte("sqlite format 3 dummy db data for testing")
	baseManifest := SyncManifest{
		DatabaseID: "test-db-1",
		Revision:   10,
		DeviceID:   "mac-01",
		DeviceName: "Office",
	}

	pkgBytes, pkgName, manifest, rawFiles, err := CreatePackage(dbData, filesDir, baseManifest, false, "")
	if err != nil {
		t.Fatalf("CreatePackage failed: %v", err)
	}
	if !strings.HasSuffix(pkgName, ".tally") {
		t.Errorf("expected package name to end with .tally, got %s", pkgName)
	}
	if len(rawFiles) != 1 {
		t.Errorf("expected 1 raw file, got %d", len(rawFiles))
	}

	destDB := filepath.Join(tempDir, "extracted", "tally.db")
	if err := ExtractAndVerifyPackage(pkgBytes, manifest, "", destDB); err != nil {
		t.Fatalf("ExtractAndVerifyPackage failed: %v", err)
	}

	extracted, err := os.ReadFile(destDB)
	if err != nil {
		t.Fatal(err)
	}
	if string(extracted) != string(dbData) {
		t.Errorf("extracted DB content mismatch: got %q, want %q", extracted, dbData)
	}
}

func TestPackageCreationAndExtractionEncrypted(t *testing.T) {
	tempDir := t.TempDir()
	filesDir := filepath.Join(tempDir, "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "scores.xlsx"), []byte("encrypted student scores"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbData := []byte("sqlite database with student confidential data")
	baseManifest := SyncManifest{
		DatabaseID: "test-db-enc",
		Revision:   25,
		DeviceID:   "laptop-01",
		DeviceName: "Home PC",
	}
	password := "SecretPass123!"

	pkgBytes, pkgName, manifest, rawFiles, err := CreatePackage(dbData, filesDir, baseManifest, true, password)
	if err != nil {
		t.Fatalf("CreatePackage encrypted failed: %v", err)
	}
	_ = pkgName
	if !manifest.Encrypted {
		t.Error("manifest should indicate Encrypted = true")
	}
	if len(manifest.KDF.SaltHex) == 0 {
		t.Error("expected non-empty KDF salt")
	}

	// 1. 正确密码解密
	destDB := filepath.Join(tempDir, "extracted_enc", "tally.db")
	if err := ExtractAndVerifyPackage(pkgBytes, manifest, password, destDB); err != nil {
		t.Fatalf("ExtractAndVerifyPackage failed with correct password: %v", err)
	}
	extracted, err := os.ReadFile(destDB)
	if err != nil {
		t.Fatal(err)
	}
	if string(extracted) != string(dbData) {
		t.Errorf("decrypted content mismatch: got %q, want %q", extracted, dbData)
	}

	// 2. 错误密码解密应失败
	destDBWrong := filepath.Join(tempDir, "extracted_wrong", "tally.db")
	err = ExtractAndVerifyPackage(pkgBytes, manifest, "WrongPass!", destDBWrong)
	if err == nil {
		t.Error("expected decryption failure with wrong password, got nil")
	}

	// 3. 检查原始文件也已加密
	if len(rawFiles) != 1 {
		t.Fatalf("expected 1 raw file, got %d", len(rawFiles))
	}
	for fname, data := range rawFiles {
		if !strings.HasSuffix(fname, ".enc") {
			t.Errorf("expected encrypted filename to end with .enc, got %s", fname)
		}
		if string(data) == "encrypted student scores" {
			t.Error("raw file should be encrypted, not plaintext")
		}
	}
}

func TestCheckRemoteRules(t *testing.T) {
	ctx := context.Background()

	// 建立模拟 WebDAV 服务器
	var mu sync.Mutex
	storage := make(map[string][]byte)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		path := r.URL.Path
		switch r.Method {
		case "GET":
			if data, ok := storage[path]; ok {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	client, err := NewClient(ts.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}

	// 1. 远端为空
	status, err := CheckRemote(ctx, client, "Tally", "db1", 10, 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.Kind != StatusRemoteEmpty {
		t.Errorf("expected StatusRemoteEmpty, got %v", status.Kind)
	}

	// 2. 远端与本地一致
	remoteManifest := SyncManifest{
		DatabaseID: "db1",
		Revision:   10,
		ExportedAt: time.Now().UTC(),
		DeviceName: "Office",
	}
	storage["/Tally/latest.json"] = []byte(`{"database_id":"db1","revision":10,"package_hash":"abc"}`)

	status, err = CheckRemote(ctx, client, "Tally", "db1", 10, 10, "abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.Kind != StatusUpToDate {
		t.Errorf("expected StatusUpToDate, got %v", status.Kind)
	}

	// 3. 远端更新且来源修订号为本地修订号 -> CanPull
	storage["/Tally/latest.json"] = []byte(`{"database_id":"db1","revision":15,"base_revision":10,"package_hash":"def"}`)
	status, err = CheckRemote(ctx, client, "Tally", "db1", 10, 10, "abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.Kind != StatusCanPull {
		t.Errorf("expected StatusCanPull, got %v", status.Kind)
	}

	// 4. 本地比远端新 -> CanUpload
	status, err = CheckRemote(ctx, client, "Tally", "db1", 20, 15, "def", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.Kind != StatusCanUpload {
		t.Errorf("expected StatusCanUpload, got %v", status.Kind)
	}

	// 5. 两边均有不同修改 -> StatusConflict
	storage["/Tally/latest.json"] = []byte(`{
		"database_id":"db1",
		"revision":16,
		"base_revision":10,
		"batch_revisions":{"batch-1":2, "batch-2":1}
	}`)
	localBatches := map[string]int64{"batch-1": 1, "batch-2": 3}
	status, err = CheckRemote(ctx, client, "Tally", "db1", 18, 10, "oldhash", localBatches)
	if err != nil {
		t.Fatal(err)
	}
	if status.Kind != StatusConflict {
		t.Errorf("expected StatusConflict, got %v", status.Kind)
	}
	if len(status.ConflictedBatches) != 2 {
		t.Errorf("expected 2 conflicted batches, got %v", status.ConflictedBatches)
	}
	_ = remoteManifest
}

func TestEndToEndUploadAndDownload(t *testing.T) {
	ctx := context.Background()

	// 内存 WebDAV 模拟
	var mu sync.Mutex
	files := make(map[string][]byte)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		path := r.URL.Path
		switch r.Method {
		case "PROPFIND":
			if _, ok := files[path]; ok || path == "/Tally/" || path == "/Tally/packages/" || path == "/Tally/files/" {
				w.WriteHeader(http.StatusMultiStatus)
				_, _ = w.Write([]byte(`<D:multistatus xmlns:D="DAV:"/>`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
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

	client, err := NewClient(ts.URL, "teacher", "pass123")
	if err != nil {
		t.Fatal(err)
	}

	// 1. 上传
	tempDir := t.TempDir()
	filesDir := filepath.Join(tempDir, "files")
	_ = os.MkdirAll(filesDir, 0o755)
	_ = os.WriteFile(filepath.Join(filesDir, "roster.xlsx"), []byte("roster sheet content"), 0o644)

	dbBytes := []byte("database content for e2e sync test")
	baseManifest := SyncManifest{
		DatabaseID: "db-e2e",
		Revision:   5,
		DeviceID:   "office-mac",
		DeviceName: "办公室",
	}
	password := "SyncPassword!"

	uploadedManifest, err := UploadSync(ctx, client, "Tally", dbBytes, filesDir, baseManifest, true, password)
	if err != nil {
		t.Fatalf("UploadSync failed: %v", err)
	}
	if uploadedManifest.PackageHash == "" {
		t.Error("uploaded manifest missing package hash")
	}

	// 2. 下载并解密
	destDB := filepath.Join(tempDir, "downloaded", "tally.db")
	destFilesDir := filepath.Join(tempDir, "downloaded", "files")

	dlManifest, err := DownloadSync(ctx, client, "Tally", password, destDB, destFilesDir)
	if err != nil {
		t.Fatalf("DownloadSync failed: %v", err)
	}
	if dlManifest.Revision != 5 {
		t.Errorf("expected downloaded revision 5, got %d", dlManifest.Revision)
	}

	decryptedDB, err := os.ReadFile(destDB)
	if err != nil {
		t.Fatal(err)
	}
	if string(decryptedDB) != string(dbBytes) {
		t.Errorf("downloaded db mismatch: got %q, want %q", decryptedDB, dbBytes)
	}

	rawFileHash := SHA256String([]byte("roster sheet content"))
	downloadedRaw, err := os.ReadFile(filepath.Join(destFilesDir, rawFileHash+".xlsx"))
	if err != nil {
		t.Fatalf("reading downloaded raw file failed: %v", err)
	}
	if string(downloadedRaw) != "roster sheet content" {
		t.Errorf("downloaded raw file mismatch: got %q, want %q", downloadedRaw, "roster sheet content")
	}
}
