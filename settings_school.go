package main

import (
	"context"
	"fmt"
	"path/filepath"

	"tally/internal/store"
	"tally/locales"
)

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
	a.reloadBatches(ctx)
	a.reloadCohorts(ctx)
	a.reloadSchoolSettings(ctx)
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
