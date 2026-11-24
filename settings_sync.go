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
	a.notifySettings(a.syncMessage)
}

func (a *app) runSyncAsync(work func() func()) {
	if a.syncBusy {
		return
	}
	a.syncBusy = true
	a.syncError = ""
	a.syncMessage = ""
	a.invalidateSettings()
	if a.settingsWin == nil && a.win == nil {
		apply := work()
		a.syncBusy = false
		if apply != nil {
			apply()
		}
		a.invalidateSettings()
		return
	}
	go func() {
		apply := work()
		finish := func() {
			a.syncBusy = false
			if apply != nil {
				apply()
			}
			a.invalidateSettings()
		}
		if a.settingsWin != nil {
			a.settingsWin.Update(finish)
			return
		}
		if a.win != nil {
			a.win.Update(finish)
			return
		}
		finish()
	}()
}

func (a *app) testWebDAVConnection() {
	rawURL := strings.TrimSpace(a.syncSettings.WebDAVURL)
	if rawURL == "" {
		a.syncError = locales.SettingsSyncWebDAVURLRequired()
		a.syncMessage = ""
		a.invalidateSettings()
		return
	}
	username := a.syncSettings.Username
	password := a.syncWebDAVPassword
	a.runSyncAsync(func() func() {
		client, err := syncpkg.NewClient(rawURL, username, password)
		if err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.Ping(ctx); err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}
		return func() {
			a.syncMessage = locales.SettingsConnectionSuccess()
			a.syncError = ""
			a.notifySettings(a.syncMessage)
		}
	})
}

func (a *app) checkRemoteSync() {
	if a.store == nil {
		return
	}
	ctx := context.Background()
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
	cfg := a.syncSettings
	password := a.syncWebDAVPassword
	a.runSyncAsync(func() func() {
		client, err := syncpkg.NewClient(cfg.WebDAVURL, cfg.Username, password)
		if err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}
		reqCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		status, err := syncpkg.CheckRemote(reqCtx, client, cfg.RemoteDir, dbID, localRev, cfg.LastSyncRev, cfg.LastPackageHash, localBatches)
		if err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}
		return func() {
			a.syncRemoteStatus = status
			a.syncMessage = status.Message
			a.syncError = ""
			if status.Kind == syncpkg.StatusConflict {
				a.syncConflictOpen = true
			}
		}
	})
}

func (a *app) startSyncUpload() {
	if a.store == nil || a.syncBusy {
		return
	}
	ctx := context.Background()
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

	// 3. 在后台执行加密打包与 WebDAV 上传
	cfg := a.syncSettings
	webdavPass := a.syncWebDAVPassword
	pkgPass := a.syncPackagePassword
	filesDir := filepath.Join(a.dataDir, "files")
	baseManifest := syncpkg.SyncManifest{
		DatabaseID:     dbID,
		Revision:       localRev,
		BaseRevision:   cfg.LastSyncRev,
		DeviceID:       cfg.DeviceID,
		DeviceName:     cfg.DeviceName,
		BatchRevisions: batchRevs,
	}

	a.runSyncAsync(func() func() {
		client, err := syncpkg.NewClient(cfg.WebDAVURL, cfg.Username, webdavPass)
		if err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}
		reqCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		finalManifest, err := syncpkg.UploadSync(reqCtx, client, cfg.RemoteDir, dbBytes, filesDir, baseManifest, cfg.EncryptEnabled, pkgPass)
		if err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}
		return func() {
			// 4. 记录本地同步状态
			a.syncSettings.LastSyncTime = finalManifest.ExportedAt.Local().Format("2006-01-02 15:04:05")
			a.syncSettings.LastSyncRev = finalManifest.Revision
			a.syncSettings.LastPackageHash = finalManifest.PackageHash
			a.syncSettings.LastSyncDevice = a.syncSettings.DeviceName
			if a.store != nil {
				_ = a.store.SaveSyncSettings(context.Background(), a.syncSettings)
			}
			a.saveSyncSecrets()

			a.syncMessage = locales.SettingsSyncSuccess()
			a.syncError = ""
			a.notifySettings(a.syncMessage)
			a.syncRemoteStatus = &syncpkg.RemoteStatus{
				Kind:           syncpkg.StatusUpToDate,
				Message:        a.syncMessage,
				RemoteManifest: finalManifest,
			}
		}
	})
}

func (a *app) startSyncPull(force bool) {
	if a.store == nil || a.syncBusy {
		return
	}
	ctx := context.Background()
	var dbID string
	var localRev int64
	var localBatches map[string]int64
	if !force {
		var err error
		dbID, err = a.store.GetDatabaseID(ctx)
		if err != nil {
			a.syncError = err.Error()
			a.syncMessage = ""
			a.invalidateSettings()
			return
		}
		localRev, localBatches, err = a.store.GetGlobalRevision(ctx)
		if err != nil {
			a.syncError = err.Error()
			a.syncMessage = ""
			a.invalidateSettings()
			return
		}
	}
	cfg := a.syncSettings
	webdavPass := a.syncWebDAVPassword
	pkgPass := a.syncPackagePassword
	dataDir := a.dataDir
	filesDir := filepath.Join(dataDir, "files")

	a.runSyncAsync(func() func() {
		client, err := syncpkg.NewClient(cfg.WebDAVURL, cfg.Username, webdavPass)
		if err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}
		reqCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if !force {
			status, err := syncpkg.CheckRemote(reqCtx, client, cfg.RemoteDir, dbID, localRev, cfg.LastSyncRev, cfg.LastPackageHash, localBatches)
			if err != nil {
				return func() {
					a.syncError = err.Error()
					a.syncMessage = ""
				}
			}
			if status.Kind == syncpkg.StatusConflict {
				return func() {
					a.syncRemoteStatus = status
					a.syncConflictOpen = true
				}
			}
			if status.Kind == syncpkg.StatusUpToDate {
				return func() {
					a.syncRemoteStatus = status
					a.syncMessage = locales.SettingsSyncStatusUpToDate()
					a.syncError = ""
				}
			}
		}

		// 1. 创建临时解压目录并下载解密
		tempDir, err := os.MkdirTemp("", "tally-pull-*")
		if err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}
		tempDB := filepath.Join(tempDir, "tally.db")
		manifest, err := syncpkg.DownloadSync(reqCtx, client, cfg.RemoteDir, pkgPass, tempDB, filesDir)
		if err != nil {
			_ = os.RemoveAll(tempDir)
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}

		// 2. SQLite 完整性校验
		verifyDB, err := sqlite.Open(reqCtx, tempDB, sqlite.OpenOptions{ReadOnly: true})
		if err != nil {
			_ = os.RemoveAll(tempDir)
			return func() {
				a.syncError = fmt.Sprintf(locales.SettingsSyncOpenTempDBFailed(), err)
				a.syncMessage = ""
			}
		}
		rows, err := verifyDB.Query(reqCtx, "PRAGMA integrity_check")
		verifyDB.Close()
		if err != nil || len(rows.Values) == 0 || rows.Values[0][0] != "ok" {
			_ = os.RemoveAll(tempDir)
			return func() {
				a.syncError = locales.SettingsSyncIntegrityCheckFailed()
				a.syncMessage = ""
			}
		}

		tempData, err := os.ReadFile(tempDB)
		_ = os.RemoveAll(tempDir)
		if err != nil {
			return func() {
				a.syncError = err.Error()
				a.syncMessage = ""
			}
		}

		return func() {
			if a.store == nil {
				return
			}
			uiCtx := context.Background()
			// 3. 关闭本地数据库并自动备份当前数据
			if err := a.store.Close(); err != nil {
				a.syncError = err.Error()
				a.syncMessage = ""
				return
			}
			a.store = nil

			if _, err := store.CreateBackup(a.dataDir); err != nil {
				a.syncError = fmt.Sprintf(locales.SettingsSyncBackupFailed(), err)
				a.reopenStore()
				return
			}

			// 4. 替换本地 tally.db 并重新打开
			dbCurrent := filepath.Join(a.dataDir, "tally.db")
			if err := os.WriteFile(dbCurrent, tempData, 0o644); err != nil {
				a.syncError = err.Error()
				a.reopenStore()
				return
			}
			if !a.reopenStore() {
				return
			}

			// 5. 记录对齐同步状态
			_ = a.store.SetDatabaseID(uiCtx, manifest.DatabaseID)
			a.syncSettings.LastSyncTime = manifest.ExportedAt.Local().Format("2006-01-02 15:04:05")
			a.syncSettings.LastSyncRev = manifest.Revision
			a.syncSettings.LastPackageHash = manifest.PackageHash
			a.syncSettings.LastSyncDevice = manifest.DeviceName
			_ = a.store.SaveSyncSettings(uiCtx, a.syncSettings)
			a.saveSyncSecrets()

			// 6. 重新加载应用界面数据
			a.reloadPlans(uiCtx)
			a.reloadCohorts(uiCtx)
			a.reloadExams(uiCtx)
			a.reloadAssessments(uiCtx)
			a.reloadSchoolSettings(uiCtx)
			a.settingsForm = form.New(a.settingsFormValuesFromDrafts())

			a.syncMessage = locales.SettingsPullSuccess()
			a.syncError = ""
			a.notifySettings(a.syncMessage)
			a.syncRemoteStatus = &syncpkg.RemoteStatus{
				Kind:           syncpkg.StatusUpToDate,
				Message:        a.syncMessage,
				RemoteManifest: manifest,
			}
		}
	})
}

func (a *app) syncConflictDialog(c *ui.Context, p uiPalette) {
	if confirmDangerModal(c, p, "sync-conflict", &a.syncConflictOpen, locales.SettingsSyncConflictModalTitle(),
		locales.SettingsSyncConflictModalBody(),
		locales.SettingsSyncConflictCancel(), locales.SettingsSyncConflictOverwrite()) {
		a.startSyncPull(true)
		a.invalidateSettings()
	}
}

func (a *app) settingsSyncSection(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().MaxWidth(t.Space(170)).Gap(t.Space(5)).Children(func() {
		a.syncSettingsForm(c, func(*settingsFormValues) {
			form.Group(c, a.settingsForm, form.GroupProps{
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
					Layout: form.Vertical,
					Value:  a.syncSettings.Username,
				}, func() {
					textField(c, &a.syncSettings.Username, locales.SettingsWebDAVUsername(), "")
				})

				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "webdav-password",
					Label:  locales.SettingsWebDAVPassword(),
					Desc:   locales.SettingsWebDAVPasswordHint(),
					Layout: form.Vertical,
					Value:  a.syncWebDAVPassword,
				}, func() {
					passwordField(c, &a.syncWebDAVPassword, locales.SettingsWebDAVPassword(), "")
				})

				// 3. 远端目录与设备名称
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "remote-dir",
					Label:  locales.SettingsRemoteDir(),
					Desc:   locales.SettingsRemoteDirHint(),
					Layout: form.Vertical,
					Value:  a.syncSettings.RemoteDir,
				}, func() {
					textField(c, &a.syncSettings.RemoteDir, locales.SettingsRemoteDir(), "Tally")
				})

				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "device-name",
					Label:  locales.SettingsDeviceName(),
					Desc:   locales.SettingsDeviceNameHint(),
					Layout: form.Vertical,
					Value:  a.syncSettings.DeviceName,
				}, func() {
					textField(c, &a.syncSettings.DeviceName, locales.SettingsDeviceName(), "Office PC")
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
						Layout: form.Vertical,
						Value:  a.syncPackagePassword,
					}, func() {
						passwordField(c, &a.syncPackagePassword, locales.SettingsSyncPassword(), "")
					})
				}
			})

			// 按钮行: 保存与测试连接
			ui.Row(c).FillWidth().Wrap().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
				saveBtn := labelledButton(c, locales.SettingsSaveSync(), nil, true)
				saveBtn.Disabled(a.syncBusy)
				if saveBtn.Clicked() {
					a.saveSyncSettings()
				}
				testBtn := plainButton(c, locales.SettingsTestConnection())
				testBtn.Disabled(a.syncBusy)
				if testBtn.Clicked() {
					a.testWebDAVConnection()
				}
			})

			// 5. 同步状态卡片与操作
			form.Group(c, a.settingsForm, form.GroupProps{
				Title: locales.SettingsSyncStatusTitle(),
				Fixed: true,
				Extra: func() {
					checkBtn := plainButton(c, locales.SettingsSyncCheckRemote())
					checkBtn.Disabled(a.syncBusy)
					if checkBtn.Clicked() {
						a.checkRemoteSync()
					}
				},
			}, func() {
				ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
					if a.syncSettings.LastSyncTime != "" {
						ui.Text(c, fmt.Sprintf(locales.SettingsLastSyncTime(), a.syncSettings.LastSyncTime)).
							FontSize(captionSize(t)).TextColor(p.muted)
					}
					if a.syncSettings.LastSyncDevice != "" {
						ui.Text(c, fmt.Sprintf(locales.SettingsLastSyncDevice(), a.syncSettings.LastSyncDevice)).
							FontSize(captionSize(t)).TextColor(p.muted).SingleLine().Tooltip(a.syncSettings.LastSyncDevice)
					}

					if a.syncBusy {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Spinner(c).Label(locales.SettingsSyncBusy())
							ui.Text(c, locales.SettingsSyncBusy()).FontSize(bodySize(t)).TextColor(p.muted)
						})
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

					ui.Row(c).FillWidth().Wrap().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
						uploadBtn := labelledButton(c, locales.SettingsSyncNow(), iconSync, true)
						uploadBtn.Disabled(a.syncBusy)
						if uploadBtn.Clicked() {
							a.startSyncUpload()
						}
						pullBtn := plainButton(c, locales.SettingsPullNow())
						pullBtn.Disabled(a.syncBusy)
						if pullBtn.Clicked() {
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
