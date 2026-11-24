package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/locales"
)

func TestSettingsSofficePathSaveAndDetect(t *testing.T) {
	t.Setenv("TALLY_SOFFICE", "")
	a := newTestApp(t)
	a.settingsPageID = "data"
	a.refreshSofficeStatus()

	fakeDir := t.TempDir()
	fakeSoffice := filepath.Join(fakeDir, "soffice")
	if err := os.WriteFile(fakeSoffice, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	missingSoffice := filepath.Join(fakeDir, "missing-soffice")

	tt := ui.NewTester(a.settingsView, 760, 620)
	tt.SetLocale("en")
	settleTester(tt)

	if !tt.HasText(locales.SettingsSofficeTitle()) {
		t.Fatalf("missing soffice section title in %q", tt.Texts())
	}

	// 检测无效自定义路径，展示未检测到提示。
	a.sofficeDraft = missingSoffice
	tt.Frame()
	if err := tt.Click(locales.SettingsSofficeTest()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.sofficeStatus.Found {
		t.Fatalf("expected missing soffice status, got %+v", a.sofficeStatus)
	}
	if !tt.HasText(locales.SettingsSofficeStatusMissing()) {
		t.Fatalf("missing status prompt not rendered: %q", tt.Texts())
	}

	// 填入有效自定义路径，先检测再保存，验证 store 持久化与 Toast 提示。
	a.sofficeDraft = fakeSoffice
	tt.Frame()
	if err := tt.Click(locales.SettingsSofficeTest()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	wantReady := fmt.Sprintf(locales.SettingsSofficeStatusReady(), locales.SettingsSofficeSourceCustom(), fakeSoffice)
	if !a.sofficeStatus.Found || a.sofficeStatus.Source != "configured" || !tt.HasText(wantReady) {
		t.Fatalf("expected ready custom soffice (%q), status=%+v texts=%q", wantReady, a.sofficeStatus, tt.Texts())
	}

	if err := tt.Click(locales.Save()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !tt.HasText(locales.SettingsSofficeSaved()) {
		t.Fatalf("missing saved toast in %q", tt.Texts())
	}
	ctx := context.Background()
	persisted, err := a.store.SofficePath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted != fakeSoffice || a.sofficePath != fakeSoffice {
		t.Fatalf("persisted=%q sofficePath=%q, want %q", persisted, a.sofficePath, fakeSoffice)
	}

	// 清空路径后保存，验证持久化清空并回退自动探测。
	a.sofficeDraft = ""
	tt.Frame()
	if err := tt.Click(locales.Save()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	persisted, err = a.store.SofficePath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted != "" || a.sofficePath != "" || a.sofficeStatus.Source != "auto" {
		t.Fatalf("cleared soffice state: persisted=%q sofficePath=%q status=%+v", persisted, a.sofficePath, a.sofficeStatus)
	}

	// 验证备份与恢复同步还原 sofficePath 及检测状态。
	a.sofficeDraft = fakeSoffice
	a.saveSofficePath()
	a.backupNow()
	if len(a.backups) == 0 {
		t.Fatalf("expected at least one backup after backupNow, err=%q", a.settingsError)
	}
	backupName := a.backups[0].Name
	a.sofficeDraft = ""
	a.saveSofficePath()
	if a.sofficePath != "" {
		t.Fatalf("expected cleared sofficePath before restore, got %q", a.sofficePath)
	}
	a.restoreBackup(backupName)
	if a.sofficePath != fakeSoffice || a.sofficeDraft != fakeSoffice || !a.sofficeStatus.Found || a.sofficeStatus.Source != "configured" {
		t.Fatalf("restoreBackup did not sync soffice state: path=%q draft=%q status=%+v",
			a.sofficePath, a.sofficeDraft, a.sofficeStatus)
	}
}

func TestPlanSaveBarDirtyBadgeAndDiscard(t *testing.T) {
	a := newTestApp(t)
	a.page = pagePlans

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	if a.planConfigDirty() {
		t.Fatal("expected clean plan immediately after load")
	}
	if tt.HasText(locales.UnsavedGuardTitle()) || tt.HasText(locales.UnsavedGuardDiscard()) {
		t.Fatalf("unexpected dirty indicator on clean plan: %q", tt.Texts())
	}
	if !tt.HasText(locales.PlanSaveHint()) {
		t.Fatalf("missing clean save hint in %q", tt.Texts())
	}

	version := a.selectedVersion()
	if version == nil || len(a.planConfig.Indicators) == 0 || len(a.planConfig.Tiers) == 0 {
		t.Fatalf("expected seeded plan with indicators and tiers, version=%v", version)
	}
	origDesc := a.planConfig.Description
	origIndicatorPoints := a.planConfig.Indicators[0].Points
	origTierWeight := a.planConfig.Tiers[0].Weight

	// 仅修改切片内部元素，验证深拷贝生效：不污染已保存版本且能触发脏状态与放弃还原。
	a.planConfig.Indicators[0].Points = origIndicatorPoints + 5
	a.planConfig.Tiers[0].Weight = origTierWeight + 0.5
	tt.Frame()

	if version.Config.Indicators[0].Points != origIndicatorPoints || version.Config.Tiers[0].Weight != origTierWeight {
		t.Fatalf("mutating draft corrupted saved version config: indicator=%v tier=%v",
			version.Config.Indicators[0].Points, version.Config.Tiers[0].Weight)
	}
	if !a.planConfigDirty() {
		t.Fatal("expected planConfigDirty() = true after slice element mutation")
	}
	if !tt.HasText(locales.UnsavedGuardTitle()) || !tt.HasText(locales.UnsavedGuardDiscard()) {
		t.Fatalf("missing dirty badge or discard button after slice mutation: %q", tt.Texts())
	}

	a.planConfig.Description = origDesc + " (modified)"
	a.planError = "validation notice"
	tt.Frame()

	if err := tt.Click(locales.UnsavedGuardDiscard()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)

	if a.planConfigDirty() ||
		a.planConfig.Description != origDesc ||
		a.planConfig.Indicators[0].Points != origIndicatorPoints ||
		a.planConfig.Tiers[0].Weight != origTierWeight ||
		a.planError != "" {
		t.Fatalf("discard did not reset draft: dirty=%v desc=%q ind=%v tier=%v err=%q",
			a.planConfigDirty(), a.planConfig.Description,
			a.planConfig.Indicators[0].Points, a.planConfig.Tiers[0].Weight, a.planError)
	}
	if tt.HasText(locales.UnsavedGuardTitle()) || !tt.HasText(locales.PlanSaveHint()) {
		t.Fatalf("save bar did not return to clean state: %q", tt.Texts())
	}
}

func TestAnalysisPageEmptyWorkspaceState(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAnalysis

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	if !tt.HasText(locales.WorkspaceEmpty()) || !tt.HasText(locales.WorkspaceCreate()) {
		t.Fatalf("missing empty workspace state in analysis page: %q", tt.Texts())
	}
	if tt.HasText(locales.AnalysisTabTeacherTrend()) {
		t.Fatalf("unexpected analysis tabs without workspace: %q", tt.Texts())
	}

	if err := tt.Click(locales.WorkspaceCreate()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.newCohortOpen || !tt.HasText(locales.NewCohortTitle()) {
		t.Fatalf("clicking WorkspaceCreate did not open new cohort modal: open=%v texts=%q", a.newCohortOpen, tt.Texts())
	}
}
