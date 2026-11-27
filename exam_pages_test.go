package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/internal/store"
)

type examPageFixture struct {
	App      *app
	ExamID   int64
	CohortID int64
}

func newExamPageFixture(t *testing.T) examPageFixture {
	t.Helper()
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	ctx := context.Background()
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	exam, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "期中考试", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	roster := store.ImportReport{
		FileHash: "roster", Path: "roster.xlsx", RowCount: 3,
		StoredPath: filepath.Join(a.dataDir, "files", "roster.xlsx"),
		Sheets:     []store.ImportSheet{{Name: "报名名单", Role: "roster", HeaderRow: 1, Rows: 3}},
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportRoster,
		Report: roster,
		Data: store.ImportData{Roster: []store.ImportRoster{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", SourceSheet: "报名名单", SourceRow: 2},
			{StudentNo: "002", Name: "乙", ClassName: "1班", Track: "history", SourceSheet: "报名名单", SourceRow: 3},
			{StudentNo: "003", Name: "丙", ClassName: "2班", SourceSheet: "报名名单", SourceRow: 4},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	report := store.ImportReport{
		FileHash: "scores", Path: "scores.xlsx", RowCount: 3,
		StoredPath: filepath.Join(a.dataDir, "files", "scores.xlsx"),
		Sheets:     []store.ImportSheet{{Name: "成绩", Role: "scores", HeaderRow: 1, Rows: 3}},
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportScores,
		Report: report,
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 120, "math": 130},
				Flags: map[string]string{}, SourceSheet: "成绩", SourceRow: 2},
			{StudentNo: "002", Scores: map[string]float64{"chinese": 90},
				Flags: map[string]string{"math": "absent"}, SourceSheet: "成绩", SourceRow: 3},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	a.reloadCohorts(ctx)
	a.reloadExams(ctx)
	a.reloadDataPages(ctx)
	assessment, err := a.store.AssessmentForExamCohort(ctx, mustExamCohortID(t, a.store, exam.ID, stage.ID))
	if err != nil {
		t.Fatal(err)
	}
	classes, err := a.store.ListClasses(ctx, cohort.ID, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetClassTarget(ctx, assessment.ID, classes[0].ID, "undergraduate", 2); err != nil {
		t.Fatal(err)
	}
	return examPageFixture{App: a, ExamID: exam.ID, CohortID: cohort.ID}
}

func mustExamCohortID(t *testing.T, s *store.Store, examID, stageID int64) int64 {
	t.Helper()
	exam, err := s.GetExam(context.Background(), examID)
	if err != nil {
		t.Fatal(err)
	}
	for _, cohort := range exam.Cohorts {
		if cohort.StageID == stageID {
			return cohort.ID
		}
	}
	t.Fatalf("exam %d has no cohort at stage %d", examID, stageID)
	return 0
}

func (f examPageFixture) open(t *testing.T, page pageID) *ui.Tester {
	t.Helper()
	f.App.page = page
	f.App.selectExam(f.ExamID)
	f.App.examCohortID = f.CohortID
	f.App.reloadExamData(context.Background())
	tt := ui.NewTester(f.App.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	return tt
}

func TestExamPagesShowRosterScoresAndStatistics(t *testing.T) {
	f := newExamPageFixture(t)
	tt := f.open(t, pageExams)
	for _, text := range []string{"期中考试", "Midterm", "Roster", "Scores", "001", "002"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	f.App.examTab = 1
	tt.Frame()
	if !tt.HasText("Score distribution and class averages") {
		t.Fatalf("missing collapsible stats toggle in %q", tt.Texts())
	}
	if err := tt.Click("Score distribution and class averages"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	for _, text := range []string{"120.0", "90.0", "Absent", "Score distribution: 语文"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if !tt.HasText("1班 average 105.0 (2 students)") {
		t.Fatalf("texts = %q", tt.Texts())
	}
}

func TestExamTargetsSaveAndStatGroupDeleteConfirm(t *testing.T) {
	f := newExamPageFixture(t)
	tt := f.open(t, pageExams)
	f.App.examTab = 3
	tt.Frame()
	if err := tt.Click("Save targets"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Class targets saved.") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	f.App.examTab = 4
	tt.Frame()
	if err := tt.Click("New merged group"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	f.App.statGroupName = "语文合并组"
	tt.Frame()
	if err := tt.Click("1班 (1班)"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Merged group saved.") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Delete “语文合并组”?") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.App.statGroupDeleteOpen {
		t.Fatal("delete confirmation stayed open")
	}
}

func TestExamQualityPageGroupsAndNavigates(t *testing.T) {
	f := newExamPageFixture(t)
	f.App.page = pageOverview
	tt := f.open(t, pageQuality)
	for _, text := range []string{"Data quality", "Warnings", "Cutoffs", "Roster"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("All data items"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Scores"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Scores") || tt.HasText("Cutoffs") {
		t.Fatalf("texts after filter = %q", tt.Texts())
	}
	f.App.qualityDataItem = ""
	tt.Frame()
	if err := tt.Click("All data items"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Roster"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("丙 (003) has no scores."); err != nil {
		t.Fatal(err)
	}
	if f.App.page != pageExams || f.App.examTab != 1 || f.App.examScoreQuery != "003" {
		t.Fatalf("page=%q tab=%d query=%q", f.App.page, f.App.examTab, f.App.examRosterQuery)
	}
}

func TestExamPrepChecklistUsesStoredCounts(t *testing.T) {
	f := newExamPageFixture(t)
	ctx := context.Background()
	assessment, err := f.App.store.AssessmentForExamCohort(ctx, mustExamCohortID(t, f.App.store, f.ExamID, f.App.selectedStage().ID))
	if err != nil {
		t.Fatal(err)
	}
	classes, err := f.App.store.ListClasses(ctx, f.CohortID, f.App.selectedStage().ID)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := f.App.store.ListTeachingGroups(ctx, f.App.selectedStage().ID, "chinese")
	if err != nil {
		t.Fatal(err)
	}
	target := 3.0
	if _, err := f.App.store.SaveStatGroup(ctx, store.StatGroupInput{
		AssessmentID: assessment.ID, Subject: "chinese", Name: "语文合并组",
		LeadClassID: classes[0].ID, Target: &target,
		MemberGroupIDs: []int64{groups[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	f.App.selectExam(f.ExamID)
	f.App.examCohortID = f.CohortID
	f.App.reloadExamData(ctx)
	f.App.examPrepOpen = true
	f.App.page = pageExams
	tt := ui.NewTester(f.App.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	if f.App.examPrep.StatGroups != 1 || f.App.examPrep.Roster != 3 || f.App.examPrep.Scores != 4 {
		t.Fatalf("prep = %+v", f.App.examPrep)
	}
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	if f.App.examPrepOpen {
		t.Fatal("preparation page stayed open")
	}
}

func TestNewExamDateInputAndMarks(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	a.page = pageExams
	tt := ui.NewTester(a.view, 1280, 1200)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("New exam"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"Create an exam", "Exam date", "2026 cohort (Class of 2029)"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	selected := false
	for _, focused := range []string{"Exam date"} {
		if tt.Focused(focused) {
			selected = true
		}
	}
	_ = selected
	if a.newExamDate.IsZero() {
		t.Fatal("exam date is zero")
	}
	if a.newExamDate.Format(time.DateOnly) != a.nowValue().Format(time.DateOnly) {
		t.Fatalf("exam date = %s", a.newExamDate.Format(time.DateOnly))
	}
	if len(a.newExamMarks) != len(a.cohorts) {
		t.Fatalf("marks = %+v", a.newExamMarks)
	}
}

func TestExamCohortSwitchAndQualityImportFilter(t *testing.T) {
	f := newExamPageFixture(t)
	ctx := context.Background()
	other, err := f.App.store.CreateCohort(ctx, 2025, 2026)
	if err != nil {
		t.Fatal(err)
	}
	multi, err := f.App.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamFinal,
		Name: "期末考试", ExamDate: "2026-12-20", CohortIDs: []int64{other.ID, f.CohortID},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.App.reloadExams(ctx)
	f.App.selectExam(multi.ID)
	f.App.page = pageExams
	tt := ui.NewTester(f.App.view, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Cohort") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	exam, err := f.App.store.GetExam(ctx, multi.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exam.Cohorts) != 2 {
		t.Fatalf("cohorts = %+v", exam.Cohorts)
	}
	current := f.App.examCohortLabel(exam.Cohorts[0])
	target := f.App.examCohortLabel(exam.Cohorts[1])
	if err := tt.Click(current); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(target); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.App.examCohort.CohortID != exam.Cohorts[1].CohortID {
		t.Fatalf("cohort = %+v", f.App.examCohort)
	}
	f.App.navigateTo(pageQuality)
	f.App.qualityImportRun = 0
	tt.Frame()
	if len(f.App.qualityImportOptions) == 0 {
		t.Fatalf("import options = %+v", f.App.qualityImportOptions)
	}
	if err := tt.Click("All import records"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	firstLabel := ""
	for _, label := range f.App.qualityImportOptions {
		firstLabel = label
		break
	}
	if err := tt.Click(firstLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.App.qualityImportRun == 0 {
		t.Fatal("import filter did not change")
	}
	for _, overview := range f.App.visibleQuality() {
		for _, issue := range overview.Issues {
			if issue.ImportRunID != f.App.qualityImportRun {
				t.Fatalf("issue from another import: %+v", issue)
			}
		}
	}
}

// 考试数据没有导入记录引用时，数据质量页不显示导入记录筛选。
func TestQualityImportFilterHiddenWithoutReferencedRuns(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	ctx := context.Background()
	cohort := a.selectedCohort()
	_, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "期中考试", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	a.reloadExams(ctx)
	a.page = pageQuality
	tt := ui.NewTester(a.view, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	if a.qualityImportOptions != nil {
		t.Fatalf("import options = %+v", a.qualityImportOptions)
	}
	if tt.HasText("All import records") {
		t.Fatalf("import filter shown: %q", tt.Texts())
	}
}

func TestExamPagesAtMinimumSizeInChineseAndDarkTheme(t *testing.T) {
	f := newExamPageFixture(t)
	f.App.page = pageExams
	f.App.selectExam(f.ExamID)
	f.App.examCohortID = f.CohortID
	f.App.reloadExamData(context.Background())
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	tt := ui.NewTester(f.App.view, 1024, 640)
	tt.SetLocale("zh-Hans")
	tt.SetDark(true)
	tt.Frame()
	for _, text := range []string{"报名名单", "成绩", "分数线", "目标", "合并组", "数据质量"} {
		box, ok := tt.Find(text)
		if !ok {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
		if box.X < 0 || box.Y < 0 || box.X+box.W > 1024 || box.Y+box.H > 640 {
			t.Fatalf("%q is outside the window: %+v", text, box)
		}
	}
	f.App.examTab = 1
	tt.Frame()
	if err := tt.Click("分数分布与班级均分"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !tt.HasText("成绩分布") {
		t.Fatalf("texts = %q", tt.Texts())
	}
}
