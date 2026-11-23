package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/form"

	"tally/internal/store"
	syncpkg "tally/internal/sync"
	"tally/locales"
)

type secretStore interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
}

type systemSecrets struct{}

func (systemSecrets) Get(key string) ([]byte, error) {
	return mygo.Secrets.Get(key)
}

func (systemSecrets) Set(key string, value []byte) error {
	return mygo.Secrets.Set(key, value)
}

type memorySecrets struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func newMemorySecrets() *memorySecrets {
	return &memorySecrets{m: make(map[string][]byte)}
}

func (m *memorySecrets) Get(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if val, ok := m.m[key]; ok {
		return val, nil
	}
	return nil, mygo.ErrSecretNotFound
}

func (m *memorySecrets) Set(key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = make(map[string][]byte)
	}
	m.m[key] = value
	return nil
}

func (a *app) secretsStore() secretStore {
	if a.secrets == nil {
		if testing.Testing() {
			a.secrets = newMemorySecrets()
		} else {
			a.secrets = systemSecrets{}
		}
	}
	return a.secrets
}

func (a *app) loadSyncSecrets() {
	if pass, err := a.secretsStore().Get("webdav-password"); err == nil {
		a.syncWebDAVPassword = string(pass)
	}
	if pass, err := a.secretsStore().Get("sync-password"); err == nil {
		a.syncPackagePassword = string(pass)
	}
}

func (a *app) saveSyncSecrets() {
	if a.syncWebDAVPassword != "" {
		_ = a.secretsStore().Set("webdav-password", []byte(a.syncWebDAVPassword))
	}
	if a.syncPackagePassword != "" {
		_ = a.secretsStore().Set("sync-password", []byte(a.syncPackagePassword))
	}
}

func (a *app) saveSyncSettings() {
	if a.store == nil {
		return
	}
	ctx := context.Background()
	if err := a.store.SaveSyncSettings(ctx, a.syncSettings); err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	a.saveSyncSecrets()
	a.syncMessage = locales.SettingsSyncSaved()
	a.syncError = ""
	a.invalidateSettings()
}

func (a *app) testWebDAVConnection() {
	rawURL := strings.TrimSpace(a.syncSettings.WebDAVURL)
	if rawURL == "" {
		a.syncError = locales.SettingsSyncWebDAVURLRequired()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	client, err := syncpkg.NewClient(rawURL, a.syncSettings.Username, a.syncWebDAVPassword)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	a.syncMessage = locales.SettingsConnectionSuccess()
	a.syncError = ""
	a.invalidateSettings()
}

func (a *app) checkRemoteSync() {
	if a.store == nil {
		return
	}
	client, err := syncpkg.NewClient(a.syncSettings.WebDAVURL, a.syncSettings.Username, a.syncWebDAVPassword)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbID, err := a.store.GetDatabaseID(ctx)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	localRev, localBatches, err := a.store.GetGlobalRevision(ctx)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}

	status, err := syncpkg.CheckRemote(ctx, client, a.syncSettings.RemoteDir, dbID, localRev, a.syncSettings.LastSyncRev, a.syncSettings.LastPackageHash, localBatches)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}

	a.syncRemoteStatus = status
	a.syncMessage = status.Message
	a.syncError = ""
	if status.Kind == syncpkg.StatusConflict {
		a.syncConflictOpen = true
	}
	a.invalidateSettings()
}

func (a *app) startSyncUpload() {
	if a.store == nil {
		return
	}
	client, err := syncpkg.NewClient(a.syncSettings.WebDAVURL, a.syncSettings.Username, a.syncWebDAVPassword)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 获取全局标识与版本
	dbID, err := a.store.GetDatabaseID(ctx)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	localRev, batchRevs, err := a.store.GetGlobalRevision(ctx)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}

	// 2. 刷新 SQLite WAL 缓存并创建本地备份快照
	if err := a.store.Close(); err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	a.store = nil

	backupDir, err := store.CreateBackup(a.dataDir)
	if !a.reopenStore() {
		return
	}
	if err != nil {
		a.syncError = fmt.Sprintf(locales.SettingsSyncCreateSnapshotFailed(), err)
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}

	dbSnapshotPath := filepath.Join(backupDir, "tally.db")
	dbBytes, err := os.ReadFile(dbSnapshotPath)
	if err != nil {
		a.syncError = fmt.Sprintf(locales.SettingsSyncReadSnapshotFailed(), err)
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}

	// 3. 构建同步元数据与执行上传
	baseManifest := syncpkg.SyncManifest{
		DatabaseID:     dbID,
		Revision:       localRev,
		BaseRevision:   a.syncSettings.LastSyncRev,
		DeviceID:       a.syncSettings.DeviceID,
		DeviceName:     a.syncSettings.DeviceName,
		BatchRevisions: batchRevs,
	}
	filesDir := filepath.Join(a.dataDir, "files")

	finalManifest, err := syncpkg.UploadSync(ctx, client, a.syncSettings.RemoteDir, dbBytes, filesDir, baseManifest, a.syncSettings.EncryptEnabled, a.syncPackagePassword)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}

	// 4. 记录本地同步状态
	a.syncSettings.LastSyncTime = finalManifest.ExportedAt.Local().Format("2006-01-02 15:04:05")
	a.syncSettings.LastSyncRev = finalManifest.Revision
	a.syncSettings.LastPackageHash = finalManifest.PackageHash
	a.syncSettings.LastSyncDevice = a.syncSettings.DeviceName
	_ = a.store.SaveSyncSettings(ctx, a.syncSettings)
	a.saveSyncSecrets()

	a.syncMessage = locales.SettingsSyncSuccess()
	a.syncError = ""
	a.syncRemoteStatus = &syncpkg.RemoteStatus{
		Kind:           syncpkg.StatusUpToDate,
		Message:        a.syncMessage,
		RemoteManifest: finalManifest,
	}
	a.invalidateSettings()
}

func (a *app) startSyncPull(force bool) {
	if a.store == nil {
		return
	}
	client, err := syncpkg.NewClient(a.syncSettings.WebDAVURL, a.syncSettings.Username, a.syncWebDAVPassword)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !force {
		dbID, err := a.store.GetDatabaseID(ctx)
		if err == nil {
			localRev, localBatches, err := a.store.GetGlobalRevision(ctx)
			if err == nil {
				status, err := syncpkg.CheckRemote(ctx, client, a.syncSettings.RemoteDir, dbID, localRev, a.syncSettings.LastSyncRev, a.syncSettings.LastPackageHash, localBatches)
				if err == nil {
					a.syncRemoteStatus = status
					if status.Kind == syncpkg.StatusConflict {
						a.syncConflictOpen = true
						a.invalidateSettings()
						return
					}
					if status.Kind == syncpkg.StatusUpToDate {
						a.syncMessage = locales.SettingsSyncStatusUpToDate()
						a.syncError = ""
						a.invalidateSettings()
						return
					}
				}
			}
		}
	}

	// 1. 创建临时解压目录
	tempDir, err := os.MkdirTemp("", "tally-pull-*")
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	defer os.RemoveAll(tempDir)

	tempDB := filepath.Join(tempDir, "tally.db")
	filesDir := filepath.Join(a.dataDir, "files")

	// 2. 下载并解压解密
	manifest, err := syncpkg.DownloadSync(ctx, client, a.syncSettings.RemoteDir, a.syncPackagePassword, tempDB, filesDir)
	if err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}

	// 3. SQLite 完整性校验
	verifyDB, err := sqlite.Open(ctx, tempDB, sqlite.OpenOptions{ReadOnly: true})
	if err != nil {
		a.syncError = fmt.Sprintf(locales.SettingsSyncOpenTempDBFailed(), err)
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	rows, err := verifyDB.Query(ctx, "PRAGMA integrity_check")
	verifyDB.Close()
	if err != nil || len(rows.Values) == 0 || rows.Values[0][0] != "ok" {
		a.syncError = locales.SettingsSyncIntegrityCheckFailed()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}

	// 4. 关闭本地数据库并自动备份当前数据
	if err := a.store.Close(); err != nil {
		a.syncError = err.Error()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	a.store = nil

	if _, err := store.CreateBackup(a.dataDir); err != nil {
		a.syncError = fmt.Sprintf(locales.SettingsSyncBackupFailed(), err)
		a.reopenStore()
		a.invalidateSettings()
		return
	}

	// 5. 替换本地 tally.db 并重新打开
	dbCurrent := filepath.Join(a.dataDir, "tally.db")
	tempData, err := os.ReadFile(tempDB)
	if err != nil {
		a.syncError = err.Error()
		a.reopenStore()
		a.invalidateSettings()
		return
	}
	if err := os.WriteFile(dbCurrent, tempData, 0o644); err != nil {
		a.syncError = err.Error()
		a.reopenStore()
		a.invalidateSettings()
		return
	}

	if !a.reopenStore() {
		return
	}

	// 6. 记录对齐同步状态
	_ = a.store.SetDatabaseID(ctx, manifest.DatabaseID)
	a.syncSettings.LastSyncTime = manifest.ExportedAt.Local().Format("2006-01-02 15:04:05")
	a.syncSettings.LastSyncRev = manifest.Revision
	a.syncSettings.LastPackageHash = manifest.PackageHash
	a.syncSettings.LastSyncDevice = manifest.DeviceName
	_ = a.store.SaveSyncSettings(ctx, a.syncSettings)
	a.saveSyncSecrets()

	// 7. 重新加载应用界面数据
	a.reloadPlans(ctx)
	a.reloadCohorts(ctx)
	a.reloadExams(ctx)
	a.reloadAssessments(ctx)
	a.reloadSchoolSettings(ctx)
	a.settingsForm = form.New(a.settingsFormValuesFromDrafts())

	a.syncMessage = locales.SettingsPullSuccess()
	a.syncError = ""
	a.syncRemoteStatus = &syncpkg.RemoteStatus{
		Kind:           syncpkg.StatusUpToDate,
		Message:        a.syncMessage,
		RemoteManifest: manifest,
	}
	a.invalidateSettings()
}

func (a *app) syncConflictDialog(c *ui.Context, p uiPalette) {
	if ui.AlertDialog(c, &a.syncConflictOpen, locales.SettingsSyncConflictModalTitle(),
		locales.SettingsSyncConflictModalBody(),
		locales.SettingsSyncConflictCancel(), locales.SettingsSyncConflictOverwrite()) == 1 {
		a.syncConflictOpen = false
		a.startSyncPull(true)
		a.invalidateSettings()
	}
}

func (a *app) settingsSyncSection(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().MaxWidth(t.Space(170)).Gap(t.Space(5)).Children(func() {
		a.syncSettingsForm(c, func(*settingsFormValues) {
			form.Group(c, a.settingsForm, form.GroupProps{
				Title: locales.SettingsSync(),
				Desc:  locales.SettingsSyncHint(),
				Fixed: true,
			}, func() {
				// 1. WebDAV 服务器地址
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "webdav-url",
					Label:  locales.SettingsWebDAVURL(),
					Desc:   locales.SettingsWebDAVURLHint(),
					Layout: form.Vertical,
					Value:  a.syncSettings.WebDAVURL,
				}, func() {
					textField(c, &a.syncSettings.WebDAVURL, locales.SettingsWebDAVURL(), "https://dav.example.com/dav/")
				})

				// 内网 HTTP 提示
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.syncSettings.WebDAVURL)), "http://") {
					notice(c, p, noticeWarning, locales.SettingsInsecureHTTPWarning())
				}

				// 2. 账号与密码
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "webdav-username",
					Label:  locales.SettingsWebDAVUsername(),
					Layout: form.Horizontal,
					Value:  a.syncSettings.Username,
				}, func() {
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
							textField(c, &a.syncSettings.Username, locales.SettingsWebDAVUsername(), "")
						})
					})
				})

				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "webdav-password",
					Label:  locales.SettingsWebDAVPassword(),
					Desc:   locales.SettingsWebDAVPasswordHint(),
					Layout: form.Horizontal,
					Value:  a.syncWebDAVPassword,
				}, func() {
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
							passwordField(c, &a.syncWebDAVPassword, locales.SettingsWebDAVPassword(), "")
						})
					})
				})

				// 3. 远端目录与设备名称
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "remote-dir",
					Label:  locales.SettingsRemoteDir(),
					Desc:   locales.SettingsRemoteDirHint(),
					Layout: form.Horizontal,
					Value:  a.syncSettings.RemoteDir,
				}, func() {
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
							textField(c, &a.syncSettings.RemoteDir, locales.SettingsRemoteDir(), "Tally")
						})
					})
				})

				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "device-name",
					Label:  locales.SettingsDeviceName(),
					Desc:   locales.SettingsDeviceNameHint(),
					Layout: form.Horizontal,
					Value:  a.syncSettings.DeviceName,
				}, func() {
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
							textField(c, &a.syncSettings.DeviceName, locales.SettingsDeviceName(), "Office PC")
						})
					})
				})

				// 4. 数据包加密开关与同步密码
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "encrypt-sync",
					Label:  locales.SettingsEncryptSync(),
					Desc:   locales.SettingsEncryptSyncHint(),
					Layout: form.Horizontal,
					Value:  a.syncSettings.EncryptEnabled,
				}, func() {
					switchField(c, &a.syncSettings.EncryptEnabled, locales.SettingsEncryptSync())
				})

				if a.syncSettings.EncryptEnabled {
					form.Field(c, a.settingsForm, form.FieldProps{
						Name:   "sync-password",
						Label:  locales.SettingsSyncPassword(),
						Desc:   locales.SettingsSyncPasswordHint(),
						Layout: form.Horizontal,
						Value:  a.syncPackagePassword,
					}, func() {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
								passwordField(c, &a.syncPackagePassword, locales.SettingsSyncPassword(), "")
							})
						})
					})
				}
			})

			// 按钮行: 保存与测试连接
			ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
				if labelledButton(c, locales.SettingsSaveSync(), nil, true).Clicked() {
					a.saveSyncSettings()
				}
				if plainButton(c, locales.SettingsTestConnection()).Clicked() {
					a.testWebDAVConnection()
				}
			})

			// 5. 同步状态卡片与操作
			form.Group(c, a.settingsForm, form.GroupProps{
				Title: locales.SettingsSyncStatusTitle(),
				Fixed: true,
				Extra: func() {
					if plainButton(c, locales.SettingsSyncCheckRemote()).Clicked() {
						a.checkRemoteSync()
					}
				},
			}, func() {
				ui.Column(c).Gap(t.Space(2)).Children(func() {
					if a.syncSettings.LastSyncTime != "" {
						ui.Text(c, fmt.Sprintf(locales.SettingsLastSyncTime(), a.syncSettings.LastSyncTime)).
							FontSize(captionSize(t)).TextColor(p.muted)
					}
					if a.syncSettings.LastSyncDevice != "" {
						ui.Text(c, fmt.Sprintf(locales.SettingsLastSyncDevice(), a.syncSettings.LastSyncDevice)).
							FontSize(captionSize(t)).TextColor(p.muted)
					}

					if a.syncRemoteStatus != nil {
						switch a.syncRemoteStatus.Kind {
						case syncpkg.StatusUpToDate:
							notice(c, p, noticeSuccess, locales.SettingsSyncStatusUpToDate())
						case syncpkg.StatusCanPull:
							notice(c, p, noticeInfo, locales.SettingsSyncStatusCanPull())
						case syncpkg.StatusCanUpload:
							notice(c, p, noticeInfo, locales.SettingsSyncStatusCanPush())
						case syncpkg.StatusConflict:
							notice(c, p, noticeDanger, locales.SettingsSyncStatusConflict())
						}
					}

					ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
						if labelledButton(c, locales.SettingsSyncNow(), iconSync, true).Clicked() {
							a.startSyncUpload()
						}
						if plainButton(c, locales.SettingsPullNow()).Clicked() {
							a.startSyncPull(false)
						}
					})
				})
			})
		})

		if a.syncMessage != "" {
			notice(c, p, noticeSuccess, a.syncMessage)
		}
		if a.syncError != "" {
			notice(c, p, noticeDanger, a.syncError)
		}
	})
}
