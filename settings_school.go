package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"tally/internal/excel"
	"tally/internal/store"
	"tally/locales"
)

func (a *app) refreshSofficeStatus() {
	a.sofficeStatus = excel.ResolveSoffice(strings.TrimSpace(a.sofficeDraft))
	a.sofficeChecked = true
}

func (a *app) chooseSofficeExecutable() {
	settingsWin := a.settingsWin
	mainWin := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:  locales.SettingsSofficePathLabel(),
			Parent: settingsWin,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		chosen := paths[0]
		status := excel.ResolveSoffice(strings.TrimSpace(chosen))
		apply := func() {
			a.sofficeDraft = chosen
			a.sofficeStatus = status
			a.sofficeChecked = true
			a.invalidateSettings()
		}
		switch {
		case settingsWin != nil:
			settingsWin.Update(apply)
		case mainWin != nil:
			mainWin.Update(apply)
		default:
			apply()
		}
	}()
}

func (a *app) saveSofficePath() {
	cleaned := strings.TrimSpace(a.sofficeDraft)
	if a.store != nil {
		if err := a.store.SetSofficePath(context.Background(), cleaned); err != nil {
			a.settingsError = err.Error()
			a.invalidateSettings()
			return
		}
	}
	a.sofficePath = cleaned
	a.sofficeDraft = cleaned
	a.settingsError = ""
	a.refreshSofficeStatus()
	a.notifySettings(locales.SettingsSofficeSaved())
	a.invalidateSettings()
}

func (a *app) saveSchoolSettings() {
	if a.store == nil {
		return
	}
	ctx := context.Background()
	for index, subject := range a.subjectDrafts {
		if err := a.store.UpdateSubject(ctx, subject); err != nil {
			a.settingsError = fmt.Sprintf(locales.SettingsSaveSubjectFailed(), index+1, err)
			return
		}
	}
	for index, track := range a.trackDrafts {
		if err := a.store.UpdateTrack(ctx, track); err != nil {
			a.settingsError = fmt.Sprintf(locales.SettingsSaveTrackFailed(), index+1, err)
			return
		}
	}
	a.settingsError = ""
	a.reloadSchoolSettings(ctx)
	a.reloadDataPages(ctx)
	a.notifySettings(locales.SettingsSchoolSaved())
}

func (a *app) backupNow() {
	if a.store == nil {
		return
	}
	ctx := context.Background()
	if err := a.store.Close(); err != nil {
		a.settingsError = err.Error()
		return
	}
	a.store = nil
	if _, err := store.CreateBackup(a.dataDir); err != nil {
		a.settingsError = err.Error()
		a.reopenStore()
		return
	}
	if !a.reopenStore() {
		return
	}
	a.settingsError = ""
	a.reloadSchoolSettings(ctx)
	a.notifySettings(locales.SettingsBackupCreated())
}

func (a *app) restoreBackup(name string) {
	if a.store == nil || name == "" {
		return
	}
	ctx := context.Background()
	if err := a.store.Close(); err != nil {
		a.settingsError = err.Error()
		return
	}
	a.store = nil
	if err := store.RestoreBackup(a.dataDir, name); err != nil {
		a.settingsError = err.Error()
		a.reopenStore()
		return
	}
	if !a.reopenStore() {
		return
	}
	a.restoreOpen = false
	a.restoreBackupName = ""
	a.settingsError = ""
	a.reloadPlans(ctx)
	a.reloadCohorts(ctx)
	a.reloadExams(ctx)
	a.reloadAssessments(ctx)
	a.reloadSchoolSettings(ctx)
	if soffice, err := a.store.SofficePath(ctx); err != nil {
		a.settingsError = err.Error()
	} else {
		a.sofficePath = soffice
		a.sofficeDraft = soffice
		a.refreshSofficeStatus()
	}
	a.notifySettings(locales.SettingsBackupRestored())
}

func (a *app) reopenStore() bool {
	opened, err := store.Open(context.Background(), filepath.Join(a.dataDir, "tally.db"))
	if err != nil {
		a.loadError = err.Error()
		return false
	}
	a.store = opened
	return true
}
