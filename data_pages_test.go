package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/internal/store"
)

func seedStageData(t *testing.T, a *app) {
	t.Helper()
	ctx := context.Background()
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	if cohort == nil || stage == nil {
		t.Fatal("no cohort or stage selected")
	}
	dir := filepath.Join(a.dataDir, "files")
	roster := store.ImportReport{
		FileHash: "roster", Path: "roster.xlsx", StoredPath: filepath.Join(dir, "roster.xlsx"),
		RowCount: 3,
		Sheets:   []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 3}},
	}
	if _, err := a.store.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班", "2班"},
			Students: []store.ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", Status: "active", SourceSheet: "学生名单", SourceRow: 2},
				{StudentNo: "002", Name: "乙", ClassName: "1班", Track: "history", Status: "active", SourceSheet: "学生名单", SourceRow: 3},
				{StudentNo: "003", Name: "丙", ClassName: "2班", Track: "history", Status: "transferred", SourceSheet: "学生名单", SourceRow: 4},
			},
		},
		Report: roster,
	}); err != nil {
		t.Fatal(err)
	}
	assignments := store.ImportReport{
		FileHash: "assignments", Path: "assignments.xlsx", StoredPath: filepath.Join(dir, "assignments.xlsx"),
		RowCount: 2,
		Sheets:   []store.ImportSheet{{Name: "任课教师", Role: "teachers", HeaderRow: 1, Rows: 2}},
	}
	if _, err := a.store.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Classes: []string{"1班"},
			Assignments: []store.ImportAssignment{
				{ClassName: "1班", Subject: "chinese", TeacherName: "张老师", SourceSheet: "任课教师", SourceRow: 2},
				{ClassName: "1班", Subject: "math", TeacherName: "李老师", SourceSheet: "任课教师", SourceRow: 3},
			},
		},
		Report: assignments,
	}); err != nil {
		t.Fatal(err)
	}
	ratio := 1.1
	baseline := store.ImportReport{
		FileHash: "baseline", Path: "baseline.xlsx", StoredPath: filepath.Join(dir, "baseline.xlsx"),
		RowCount: 3,
		Sheets:   []store.ImportSheet{{Name: "基数", Role: "baselines", HeaderRow: 1, Rows: 3}},
	}
	if _, err := a.store.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "baselines",
		Data: store.ImportData{
			ClassBaselines: []store.ImportClassBaseline{
				{ClassName: "1班", Subject: "chinese", Average: 82.5, Ratio: &ratio, SourceSheet: "基数", SourceRow: 2},
				{ClassName: "1班", Subject: "math", Average: 90, Ratio: &ratio, SourceSheet: "基数", SourceRow: 3},
				{ClassName: "2班", Subject: "chinese", Average: 70, Ratio: &ratio, SourceSheet: "基数", SourceRow: 4},
			},
		},
		Report: baseline,
	}); err != nil {
		t.Fatal(err)
	}
	a.reloadCohorts(ctx)
	a.reloadDataPages(ctx)
}

func TestStudentPageSearchAndFilters(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{"001", "002", "003", "甲", "丙"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	a.dataQuery = "002"
	a.reloadStudents(context.Background())
	tt.Frame()
	if a.dataQuery != "002" {
		t.Fatalf("query = %q", a.dataQuery)
	}
	if !tt.HasText("002") || tt.HasText("001") || tt.HasText("003") {
		t.Fatalf("texts after search = %q", tt.Texts())
	}
	a.dataQuery = ""
	a.reloadStudents(context.Background())
	tt.Frame()
	if a.dataQuery != "" {
		t.Fatalf("query after escape = %q", a.dataQuery)
	}
	if err := tt.Click("All classes"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("2班"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("003") || tt.HasText("001") {
		t.Fatalf("texts after class filter = %q", tt.Texts())
	}
	if err := tt.Click("All statuses"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Transferred"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("003") {
		t.Fatalf("texts after status filter = %q", tt.Texts())
	}
}

func TestStageDataPagesShowSources(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	a.page = pageFaculty
	tt.Frame()
	for _, text := range []string{"张老师", "李老师", "任课教师"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Teaching assignments"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"数学", "李老师"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	a.page = pageGroups
	tt.Frame()
	if !tt.HasText("数学") || !tt.HasText("李老师") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	a.groupQuery = "语文"
	a.reloadDataPages(context.Background())
	tt.Frame()
	if !tt.HasText("张老师") || tt.HasText("李老师") {
		t.Fatalf("texts after group search = %q", tt.Texts())
	}
	a.page = pageBaseline
	tt.Frame()
	for _, text := range []string{"82.50", "90.00", "基数"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	a.page = pageImports
	tt.Frame()
	for _, text := range []string{"roster.xlsx", "assignments.xlsx", "baseline.xlsx"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	a.importQuery = "assignments"
	tt.Frame()
	if !tt.HasText("assignments.xlsx") || tt.HasText("roster.xlsx") {
		t.Fatalf("texts after import search = %q", tt.Texts())
	}
}

func TestDataPagesAtMinimumSizeAndChinese(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.page = pageStudents
	a.language = "zh-Hans"
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	for _, text := range []string{"学生", "姓名", "001"} {
		box, ok := tt.Find(text)
		if !ok {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
		if box.X < 0 || box.Y < 0 || box.X+box.W > 1024 || box.Y+box.H > 640 {
			t.Fatalf("%q is outside the window: %+v", text, box)
		}
	}
}

func TestStudentPageInDarkTheme(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.SetDark(true)
	tt.Frame()
	for _, text := range []string{"Students", "001", "002", "003"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestRestoreBackupUsesAlertDialog(t *testing.T) {
	a := newTestApp(t)
	a.backupNow()
	if a.settingsError != "" {
		t.Fatal(a.settingsError)
	}
	if len(a.backups) == 0 {
		t.Fatal("no backups")
	}
	a.page = pageSettings
	tt := ui.NewTester(a.view, 1280, 1600)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Restore"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.restoreOpen {
		t.Fatal("restore confirmation did not open")
	}
	if !tt.HasText("Restore this backup?") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.restoreOpen {
		t.Fatal("restore confirmation stayed open")
	}
}
