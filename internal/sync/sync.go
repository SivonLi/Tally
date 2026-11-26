package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RemoteStatusKind 表示本地与远端的对比状态。
type RemoteStatusKind int

const (
	StatusUpToDate RemoteStatusKind = iota
	StatusCanUpload
	StatusCanPull
	StatusConflict
	StatusRemoteEmpty
)

// SyncManifest 是 latest.json 与数据包内 manifest.json 的数据模型。
type SyncManifest struct {
	FormatVersion  int              `json:"format_version"`
	DatabaseID     string           `json:"database_id"`
	Revision       int64            `json:"revision"`
	BaseRevision   int64            `json:"base_revision"`
	DeviceID       string           `json:"device_id"`
	DeviceName     string           `json:"device_name"`
	ExportedAt     time.Time        `json:"exported_at"`
	PackageName    string           `json:"package_name"`
	PackageHash    string           `json:"package_hash"`
	Encrypted      bool             `json:"encrypted"`
	KDF            KDFParams        `json:"kdf,omitempty"`
	Files          []SyncFileEntry  `json:"files"`
	BatchRevisions map[string]int64 `json:"batch_revisions,omitempty"`
}

// KDFParams 包含 scrypt 密钥派生参数。
type KDFParams struct {
	Algo    string `json:"algo"` // "scrypt"
	N       int    `json:"n"`
	R       int    `json:"r"`
	P       int    `json:"p"`
	SaltHex string `json:"salt_hex"`
}

// SyncFileEntry 描述数据包或远端 files/ 目录下的单个文件。
type SyncFileEntry struct {
	Path          string `json:"path"` // 例如 "tally.db" 或 "files/<hash>.xlsx"
	Size          int64  `json:"size"`
	PlaintextHash string `json:"plaintext_hash"`
	Encrypted     bool   `json:"encrypted"`
	NonceHex      string `json:"nonce_hex,omitempty"`
}

// RemoteStatus 包含检测远端版本后的结果。
type RemoteStatus struct {
	Kind              RemoteStatusKind
	Message           string
	RemoteManifest    *SyncManifest
	LocalRevision     int64
	LastSyncRevision  int64
	ConflictedBatches []string
}

// CreatePackage 生成数据包 (.tally zip) 以及需要上传到 files/ 的原始文件字节映射。
func CreatePackage(
	dbPlaintext []byte,
	filesDir string,
	baseManifest SyncManifest,
	encrypt bool,
	password string,
) (pkgBytes []byte, pkgName string, finalManifest SyncManifest, rawFilesToUpload map[string][]byte, err error) {
	if encrypt && len(password) == 0 {
		return nil, "", baseManifest, nil, errors.New("开启加密时同步密码不能为空")
	}

	rawFilesToUpload = make(map[string][]byte)
	var key []byte
	var salt []byte
	kdf := KDFParams{
		Algo: "scrypt",
		N:    ScryptN,
		R:    ScryptR,
		P:    ScryptP,
	}

	if encrypt {
		salt, err = GenerateRandomBytes(SaltLen)
		if err != nil {
			return nil, "", baseManifest, nil, fmt.Errorf("生成盐值失败: %w", err)
		}
		kdf.SaltHex = hex.EncodeToString(salt)
		key, err = DeriveKey(password, salt)
		if err != nil {
			return nil, "", baseManifest, nil, fmt.Errorf("派生密钥失败: %w", err)
		}
	}

	// 1. 处理 tally.db
	dbHash := SHA256String(dbPlaintext)
	var dbEntry SyncFileEntry
	var dbPackageData []byte
	var dbPackageFilename string

	if encrypt {
		nonce, err := GenerateRandomBytes(NonceLen)
		if err != nil {
			return nil, "", baseManifest, nil, err
		}
		encBlock, err := EncryptBlock(dbPlaintext, key, nonce)
		if err != nil {
			return nil, "", baseManifest, nil, fmt.Errorf("加密数据库快照失败: %w", err)
		}
		dbPackageData = encBlock
		dbPackageFilename = "tally.db.enc"
		dbEntry = SyncFileEntry{
			Path:          "tally.db",
			Size:          int64(len(dbPlaintext)),
			PlaintextHash: dbHash,
			Encrypted:     true,
			NonceHex:      hex.EncodeToString(nonce),
		}
	} else {
		dbPackageData = dbPlaintext
		dbPackageFilename = "tally.db"
		dbEntry = SyncFileEntry{
			Path:          "tally.db",
			Size:          int64(len(dbPlaintext)),
			PlaintextHash: dbHash,
			Encrypted:     false,
		}
	}

	filesList := []SyncFileEntry{dbEntry}

	// 2. 处理 files/ 目录下的原始文件
	if filesDir != "" {
		entries, err := os.ReadDir(filesDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, "", baseManifest, nil, fmt.Errorf("读取原始文件目录失败: %w", err)
		}
		seenHashes := make(map[string]bool)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			fpath := filepath.Join(filesDir, e.Name())
			rawPlaintext, err := os.ReadFile(fpath)
			if err != nil {
				return nil, "", baseManifest, nil, fmt.Errorf("读取原始文件 %s 失败: %w", e.Name(), err)
			}
			rawHash := SHA256String(rawPlaintext)
			if seenHashes[rawHash] {
				continue
			}
			seenHashes[rawHash] = true
			relPath := "files/" + rawHash + ".xlsx"

			if encrypt {
				nonce, err := GenerateRandomBytes(NonceLen)
				if err != nil {
					return nil, "", baseManifest, nil, err
				}
				encBlock, err := EncryptBlock(rawPlaintext, key, nonce)
				if err != nil {
					return nil, "", baseManifest, nil, fmt.Errorf("加密原始文件 %s 失败: %w", e.Name(), err)
				}
				rawFilesToUpload[rawHash+".xlsx.enc"] = encBlock
				filesList = append(filesList, SyncFileEntry{
					Path:          relPath,
					Size:          int64(len(rawPlaintext)),
					PlaintextHash: rawHash,
					Encrypted:     true,
					NonceHex:      hex.EncodeToString(nonce),
				})
			} else {
				rawFilesToUpload[rawHash+".xlsx"] = rawPlaintext
				filesList = append(filesList, SyncFileEntry{
					Path:          relPath,
					Size:          int64(len(rawPlaintext)),
					PlaintextHash: rawHash,
					Encrypted:     false,
				})
			}
		}
	}

	// 3. 构造数据包文件名: Tally-<deviceId>-r<rev>-<time>.tally
	now := time.Now().UTC()
	safeDevID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, baseManifest.DeviceID)
	if safeDevID == "" {
		safeDevID = "device"
	}
	pkgName = fmt.Sprintf("Tally-%s-r%d-%s.tally", safeDevID, baseManifest.Revision, now.Format("20060102-150405"))

	finalManifest = baseManifest
	finalManifest.FormatVersion = 1
	finalManifest.PackageName = pkgName
	finalManifest.ExportedAt = now
	finalManifest.Encrypted = encrypt
	if encrypt {
		finalManifest.KDF = kdf
	}
	finalManifest.Files = filesList

	// 4. 打包为 zip
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	// 写入 manifest.json
	manifestJSON, err := json.MarshalIndent(finalManifest, "", "  ")
	if err != nil {
		return nil, "", baseManifest, nil, err
	}
	mw, err := zw.Create("manifest.json")
	if err != nil {
		return nil, "", baseManifest, nil, err
	}
	if _, err := mw.Write(manifestJSON); err != nil {
		return nil, "", baseManifest, nil, err
	}

	// 写入 tally.db 或 tally.db.enc
	dw, err := zw.Create(dbPackageFilename)
	if err != nil {
		return nil, "", baseManifest, nil, err
	}
	if _, err := dw.Write(dbPackageData); err != nil {
		return nil, "", baseManifest, nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, "", baseManifest, nil, err
	}

	pkgBytes = zipBuf.Bytes()
	finalManifest.PackageHash = SHA256String(pkgBytes)

	return pkgBytes, pkgName, finalManifest, rawFilesToUpload, nil
}

// ExtractAndVerifyPackage 解压数据包，解密 tally.db 并写入 destDBPath，返回解压出的明文校验哈希。
func ExtractAndVerifyPackage(pkgBytes []byte, manifest SyncManifest, password string, destDBPath string) error {
	zr, err := zip.NewReader(bytes.NewReader(pkgBytes), int64(len(pkgBytes)))
	if err != nil {
		return fmt.Errorf("读取数据包 zip 失败: %w", err)
	}

	var dbFile *zip.File
	var manifestFile *zip.File
	for _, f := range zr.File {
		switch f.Name {
		case "manifest.json":
			manifestFile = f
		case "tally.db", "tally.db.enc":
			dbFile = f
		}
	}
	if dbFile == nil {
		return errors.New("数据包缺少数据库文件")
	}

	// 校验数据包内 manifest
	if manifestFile != nil {
		rc, err := manifestFile.Open()
		if err != nil {
			return fmt.Errorf("打开数据包内 manifest.json 失败: %w", err)
		}
		var internalManifest SyncManifest
		decErr := json.NewDecoder(rc).Decode(&internalManifest)
		_ = rc.Close()
		if decErr != nil {
			return fmt.Errorf("解析数据包内 manifest.json 失败: %w", decErr)
		}
		if internalManifest.FormatVersion != 1 {
			return errors.New("不支持的数据包格式版本")
		}
	}

	rc, err := dbFile.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("读取数据库数据失败: %w", err)
	}

	// 查找 tally.db 的元数据
	var dbMeta *SyncFileEntry
	for _, fe := range manifest.Files {
		if fe.Path == "tally.db" {
			feCopy := fe
			dbMeta = &feCopy
			break
		}
	}

	var plaintext []byte
	if manifest.Encrypted || dbFile.Name == "tally.db.enc" {
		if len(password) == 0 {
			return errors.New("远端数据包已加密，请输入同步密码")
		}
		salt, err := hex.DecodeString(manifest.KDF.SaltHex)
		if err != nil || len(salt) == 0 {
			return errors.New("无效的加密盐值")
		}
		key, err := DeriveKey(password, salt)
		if err != nil {
			return fmt.Errorf("派生解密密钥失败: %w", err)
		}
		if dbMeta == nil || dbMeta.NonceHex == "" {
			return errors.New("缺少数据库文件的 nonce 记录")
		}
		nonce, err := hex.DecodeString(dbMeta.NonceHex)
		if err != nil {
			return errors.New("无效的 nonce 记录")
		}
		plaintext, err = DecryptBlock(data, key, nonce, dbMeta.PlaintextHash)
		if err != nil {
			return err
		}
	} else {
		plaintext = data
		if dbMeta != nil && dbMeta.PlaintextHash != "" {
			if SHA256String(plaintext) != dbMeta.PlaintextHash {
				return ErrHashMismatch
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(destDBPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destDBPath, plaintext, 0o644)
}

// CheckRemote 检查远端 WebDAV 服务器上的同步状态与版本先后关系。
func CheckRemote(
	ctx context.Context,
	client *Client,
	remoteDir string,
	localDBID string,
	localRev int64,
	lastSyncRev int64,
	lastSyncHash string,
	localBatches map[string]int64,
) (*RemoteStatus, error) {
	remoteDir = strings.Trim(remoteDir, "/")
	latestPath := remoteDir + "/latest.json"

	latestBytes, err := client.Get(ctx, latestPath)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &RemoteStatus{
				Kind:             StatusRemoteEmpty,
				Message:          "远端暂无同步数据，可进行首次上传",
				LocalRevision:    localRev,
				LastSyncRevision: lastSyncRev,
			}, nil
		}
		return nil, err
	}

	var remoteManifest SyncManifest
	if err := json.Unmarshal(latestBytes, &remoteManifest); err != nil {
		return nil, fmt.Errorf("解析远端 latest.json 失败: %w", err)
	}

	status := &RemoteStatus{
		RemoteManifest:   &remoteManifest,
		LocalRevision:    localRev,
		LastSyncRevision: lastSyncRev,
	}

	// 规则 1: 远端修订号等于本地修订号且哈希相同（或本地自上次同步未做修改）
	if remoteManifest.Revision == localRev {
		if localRev == lastSyncRev || (lastSyncHash != "" && remoteManifest.PackageHash == lastSyncHash) {
			status.Kind = StatusUpToDate
			status.Message = fmt.Sprintf("已是最新，上次同步: %s，来自 %s",
				remoteManifest.ExportedAt.Local().Format("01月02日 15:04"), remoteManifest.DeviceName)
			return status, nil
		}
	}

	// 规则 2: 远端修订号大于本地修订号，且满足以下任一条件可直接拉取：
	// - 远端直接基于本地当前修订号修改 (BaseRevision == localRev)
	// - 本地自上次同步后未修改 (localRev == lastSyncRev)
	// - 本地为首次连接远端的空库 (从未同步且无业务批次数据)
	isFreshEmptyLocal := lastSyncRev == 0 && lastSyncHash == "" && len(localBatches) == 0
	if remoteManifest.Revision > localRev {
		if remoteManifest.BaseRevision == localRev || localRev == lastSyncRev || isFreshEmptyLocal {
			status.Kind = StatusCanPull
			status.Message = fmt.Sprintf("远端有新版本 (修订号 r%d)，来自 %s，可直接拉取",
				remoteManifest.Revision, remoteManifest.DeviceName)
			return status, nil
		}
	}

	// 规则 3: 远端修订号小于本地修订号，且远端等于上次同步修订号或哈希相同
	if remoteManifest.Revision < localRev {
		if remoteManifest.Revision == lastSyncRev || (lastSyncHash != "" && remoteManifest.PackageHash == lastSyncHash) {
			status.Kind = StatusCanUpload
			status.Message = fmt.Sprintf("本地有未同步的修改 (本地 r%d > 远端 r%d)，可直接上传",
				localRev, remoteManifest.Revision)
			return status, nil
		}
	}

	// 规则 4: 修订号不一致且两端均有修改，判定为冲突
	status.Kind = StatusConflict
	status.Message = "检测到并发修改冲突：远端与本地在上次同步后均有改动"

	// 分析冲突的批次
	var conflicts []string
	for k, rRev := range remoteManifest.BatchRevisions {
		if lRev, ok := localBatches[k]; ok {
			if lRev != rRev {
				conflicts = append(conflicts, k)
			}
		}
	}
	status.ConflictedBatches = conflicts
	return status, nil
}

// UploadSync 执行完整的 7 步上传流程。
func UploadSync(
	ctx context.Context,
	client *Client,
	remoteDir string,
	dbPlaintext []byte,
	filesDir string,
	baseManifest SyncManifest,
	encrypt bool,
	password string,
) (*SyncManifest, error) {
	remoteDir = strings.Trim(remoteDir, "/")

	// 1. 确保远端各层目录已创建
	if err := client.EnsureDir(ctx, remoteDir); err != nil {
		return nil, fmt.Errorf("确保远端根目录失败: %w", err)
	}
	if err := client.EnsureDir(ctx, remoteDir+"/packages"); err != nil {
		return nil, fmt.Errorf("确保远端 packages 目录失败: %w", err)
	}
	if err := client.EnsureDir(ctx, remoteDir+"/files"); err != nil {
		return nil, fmt.Errorf("确保远端 files 目录失败: %w", err)
	}

	// 2. 生成快照与数据包
	pkgBytes, pkgName, finalManifest, rawFiles, err := CreatePackage(
		dbPlaintext, filesDir, baseManifest, encrypt, password,
	)
	if err != nil {
		return nil, fmt.Errorf("创建数据包失败: %w", err)
	}

	// 3. 上传本地新增的原始文件
	for relName, data := range rawFiles {
		remoteFilePath := remoteDir + "/files/" + relName
		exists, err := client.Stat(ctx, remoteFilePath)
		if err == nil && exists {
			continue // 远端已存在，无需重复上传
		}
		if err := client.Put(ctx, remoteFilePath, data); err != nil {
			return nil, fmt.Errorf("上传原始文件 %s 失败: %w", relName, err)
		}
	}

	// 4. 上传数据包
	pkgRemotePath := remoteDir + "/packages/" + pkgName
	if err := client.Put(ctx, pkgRemotePath, pkgBytes); err != nil {
		return nil, fmt.Errorf("上传数据包失败: %w", err)
	}

	// 5. 回读数据包并校验哈希
	verifyBytes, err := client.Get(ctx, pkgRemotePath)
	if err != nil {
		_ = client.Delete(ctx, pkgRemotePath)
		return nil, fmt.Errorf("校验远端数据包下载失败: %w", err)
	}
	if SHA256String(verifyBytes) != finalManifest.PackageHash {
		_ = client.Delete(ctx, pkgRemotePath)
		return nil, errors.New("远端数据包校验哈希不匹配，已自动回滚删除")
	}

	// 6. 把 latest.json 写入临时文件，再用 MOVE 原子重命名
	latestBytes, err := json.MarshalIndent(finalManifest, "", "  ")
	if err != nil {
		return nil, err
	}
	tmpLatest := remoteDir + "/latest.json.tmp"
	finalLatest := remoteDir + "/latest.json"

	if err := client.Put(ctx, tmpLatest, latestBytes); err != nil {
		_ = client.Delete(ctx, tmpLatest)
		return nil, fmt.Errorf("上传临时指针文件失败: %w", err)
	}
	if err := client.Move(ctx, tmpLatest, finalLatest); err != nil {
		_ = client.Delete(ctx, tmpLatest)
		return nil, fmt.Errorf("远端原子更新指针文件失败: %w", err)
	}

	// 7. 返回同步完成元数据
	return &finalManifest, nil
}

// DownloadSync 下载远端数据包与缺失的原始文件，解密并写入 tempDBPath 与 filesDir。
func DownloadSync(
	ctx context.Context,
	client *Client,
	remoteDir string,
	password string,
	tempDBPath string,
	localFilesDir string,
) (*SyncManifest, error) {
	remoteDir = strings.Trim(remoteDir, "/")

	// 1. 读取 latest.json
	latestBytes, err := client.Get(ctx, remoteDir+"/latest.json")
	if err != nil {
		return nil, fmt.Errorf("读取远端 latest.json 失败: %w", err)
	}
	var manifest SyncManifest
	if err := json.Unmarshal(latestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("解析远端 latest.json 失败: %w", err)
	}

	// 2. 下载数据包
	pkgRemotePath := remoteDir + "/packages/" + manifest.PackageName
	pkgBytes, err := client.Get(ctx, pkgRemotePath)
	if err != nil {
		return nil, fmt.Errorf("下载远端数据包失败: %w", err)
	}

	// 3. 校验数据包哈希
	if SHA256String(pkgBytes) != manifest.PackageHash {
		return nil, errors.New("远端数据包哈希与 latest.json 记录不匹配")
	}

	// 4. 解密并解压 tally.db
	if err := ExtractAndVerifyPackage(pkgBytes, manifest, password, tempDBPath); err != nil {
		return nil, fmt.Errorf("解压或解密数据包失败: %w", err)
	}

	// 5. 补齐本地缺少的原始文件
	if localFilesDir != "" {
		if err := os.MkdirAll(localFilesDir, 0o755); err != nil {
			return nil, err
		}

		var key []byte
		if manifest.Encrypted {
			salt, _ := hex.DecodeString(manifest.KDF.SaltHex)
			key, _ = DeriveKey(password, salt)
		}

		for _, fe := range manifest.Files {
			if !strings.HasPrefix(fe.Path, "files/") {
				continue
			}
			localTarget := filepath.Join(localFilesDir, filepath.Base(fe.Path))
			if _, err := os.Stat(localTarget); err == nil {
				// 本地已存在，校验哈希
				curData, err := os.ReadFile(localTarget)
				if err == nil && SHA256String(curData) == fe.PlaintextHash {
					continue
				}
			}

			// 从远端下载
			remoteFilePath := remoteDir + "/" + fe.Path
			if fe.Encrypted {
				remoteFilePath += ".enc"
			}
			remoteData, err := client.Get(ctx, remoteFilePath)
			if err != nil {
				continue // 缺失的个别历史文件不阻断数据库主流程
			}

			var plain []byte
			if fe.Encrypted {
				if len(key) == 0 {
					continue
				}
				nonce, err := hex.DecodeString(fe.NonceHex)
				if err != nil {
					continue
				}
				plain, err = DecryptBlock(remoteData, key, nonce, fe.PlaintextHash)
				if err != nil {
					continue
				}
			} else {
				plain = remoteData
				if SHA256String(plain) != fe.PlaintextHash {
					continue
				}
			}

			_ = os.WriteFile(localTarget, plain, 0o644)
		}
	}

	return &manifest, nil
}
