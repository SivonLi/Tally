package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/xuri/excelize/v2"

	"tally/internal/store"
	"tally/locales"
)

type assessmentPageFixture struct {
	App          *app
	AssessmentID int64
	ExamID       int64
	CohortID     int64
	LeadClassID  int64
}

func newAssessmentPageFixture(t *testing.T, missingTarget bool) assessmentPageFixture {
	t.Helper()
	ctx := context.Background()
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	exam, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "期中考试", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportRoster,
		Report: store.ImportReport{
			FileHash: "roster", Path: "roster.xlsx", RowCount: 3,
			StoredPath: filepath.Join(a.dataDir, "files", "roster.xlsx"),
			Sheets:     []store.ImportSheet{{Name: "报名名单", Role: "roster", HeaderRow: 1, Rows: 3}},
		},
		Data: store.ImportData{Roster: []store.ImportRoster{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", TotalScore: scorePointer(140)},
			{StudentNo: "002", Name: "乙", ClassName: "1班", Track: "physics", TotalScore: scorePointer(120)},
			{StudentNo: "003", Name: "丙", ClassName: "2班", Track: "physics", TotalScore: scorePointer(100)},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportScores,
		Report: store.ImportReport{
			FileHash: "scores", Path: "scores.xlsx", RowCount: 3,
			StoredPath: filepath.Join(a.dataDir, "files", "scores.xlsx"),
			Sheets:     []store.ImportSheet{{Name: "成绩", Role: "scores", HeaderRow: 1, Rows: 3}},
		},
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 100, "math": 90}, Flags: map[string]string{}},
			{StudentNo: "002", Scores: map[string]float64{"chinese": 90, "math": 80}, Flags: map[string]string{}},
			{StudentNo: "003", Scores: map[string]float64{"chinese": 80, "math": 70}, Flags: map[string]string{}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportCutoffs,
		Report: store.ImportReport{
			FileHash: "cutoffs", Path: "cutoffs.xlsx", RowCount: 2,
			StoredPath: filepath.Join(a.dataDir, "files", "cutoffs.xlsx"),
			Sheets:     []store.ImportSheet{{Name: "分数线", Role: "cutoffs", HeaderRow: 1, Rows: 2}},
		},
		Data: store.ImportData{
			SubjectCutoffs: []store.ImportSubjectCutoff{
				{Subject: "chinese", Track: "physics", Value: 60},
				{Subject: "math", Track: "physics", Value: 60},
			},
			TierCutoffs: []store.ImportTierCutoff{
				{TierKey: "qingbei", Track: "physics", Value: 130},
				{TierKey: "c9", Track: "physics", Value: 120},
				{TierKey: "project985", Track: "physics", Value: 110},
				{TierKey: "project211", Track: "physics", Value: 100},
				{TierKey: "special_control", Track: "physics", Value: 90},
				{TierKey: "undergraduate", Track: "physics", Value: 1},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	leadCounts := map[string]*float64{
		"qingbei": scorePointer(0), "c9": scorePointer(0), "project985": scorePointer(0),
		"project211": scorePointer(1), "special_control": scorePointer(1),
		"undergraduate": scorePointer(1),
	}
	if missingTarget {
		delete(leadCounts, "qingbei")
	}
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportTargets,
		Report: store.ImportReport{
			FileHash: "targets", Path: "targets.xlsx", RowCount: 2,
			StoredPath: filepath.Join(a.dataDir, "files", "targets.xlsx"),
			Sheets:     []store.ImportSheet{{Name: "目标", Role: "targets", HeaderRow: 1, Rows: 2}},
		},
		Data: store.ImportData{ClassTargets: []store.ImportClassTarget{
			{ClassName: "1班", Counts: leadCounts},
			{ClassName: "2班", Counts: map[string]*float64{
				"qingbei": scorePointer(0), "c9": scorePointer(0), "project985": scorePointer(0),
				"project211": scorePointer(1), "special_control": scorePointer(1),
				"undergraduate": scorePointer(1),
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	classes, err := a.store.ListClasses(ctx, cohort.ID, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]int64{}
	for _, class := range classes {
		byName[class.Name] = class.ID
	}
	assessment, err := a.store.AssessmentForExamCohort(ctx, mustExamCohortID(t, a.store, exam.ID, stage.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.EnsureAssessmentStatGroups(ctx, assessment.ID); err != nil {
		t.Fatal(err)
	}
	a.reloadCohorts(ctx)
	a.reloadExams(ctx)
	a.reloadAssessments(ctx)
	a.page = pageAssessments
	return assessmentPageFixture{
		App: a, AssessmentID: assessment.ID, ExamID: exam.ID, CohortID: cohort.ID,
		LeadClassID: byName["1班"],
	}
}

func scorePointer(value float64) *float64 {
	return &value
}

func TestAssessmentPageShowsWorkflow(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	tt := ui.NewTester(fixture.App.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{
		"Assessments", "期中考试", "Midterm", "2026级", "Grade 10",
		"Data", "Data for this assessment", "Roster", "Scores", "Cutoffs", "Targets",
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Calculate final results"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if fixture.App.assessmentError != "" {
		t.Fatal(fixture.App.assessmentError)
	}
	if len(fixture.App.assessmentRows) != 2 {
		t.Fatalf("rows = %d", len(fixture.App.assessmentRows))
	}
	if len(fixture.App.assessmentPersonal) != 2 {
		t.Fatalf("personal = %d", len(fixture.App.assessmentPersonal))
	}
	for _, text := range []string{"Results", "Chinese", "Math", "张老师"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestAssessmentCalculateRequiresPreparation(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	ctx := context.Background()
	groups, err := a.store.ListExamStatGroups(ctx, fixture.AssessmentID)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range groups {
		if err := a.store.DeleteStatGroup(ctx, fixture.AssessmentID, group.ID); err != nil {
			t.Fatal(err)
		}
	}
	a.reloadAssessments(ctx)
	overview := a.selectedAssessmentOverview()
	if overview == nil {
		t.Fatal("no assessment selected")
	}
	if a.assessmentReady(*overview) {
		t.Fatal("assessment is ready without stat groups")
	}
	if got := a.assessmentMissingItems(*overview); len(got) != 1 || got[0] != "Merged groups" {
		t.Fatalf("missing items = %q", got)
	}
	tt := ui.NewTester(a.view, 1280, 1000)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Before calculation, prepare: Merged groups.") {
		t.Fatalf("texts = %q", tt.Texts())
	}
}

func TestAssessmentExportWritesWorkbook(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.startAssessmentCalculate()
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}
	path := filepath.Join(a.dataDir, "result.xlsx")
	a.startAssessmentExport(path)
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer workbook.Close()
	for _, sheet := range []string{"计算依据", "量化计算", "排名赋分", "个人得分", "结果明细"} {
		found := false
		for _, name := range workbook.GetSheetList() {
			if name == sheet {
				found = true
			}
		}
		if !found {
			t.Errorf("missing sheet %q in %v", sheet, workbook.GetSheetList())
		}
	}
}

func TestAssessmentZeroTargetFill(t *testing.T) {
	fixture := newAssessmentPageFixture(t, true)
	a := fixture.App
	a.startAssessmentCalculate()
	if !a.zeroOpen || len(a.zeroIssues) == 0 {
		t.Fatalf("zero dialog = %v issues=%d", a.zeroOpen, len(a.zeroIssues))
	}
	tt := ui.NewTester(a.view, 1280, 1000)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Fill missing targets") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	for _, issue := range a.zeroIssues {
		for _, tier := range issue.MissingTiers {
			value := 0.0
			a.zeroValues[issue.ClassID+":"+tier] = &value
		}
	}
	a.zeroReason = "test"
	tt.Frame()
	if err := tt.Click("Fill and recalculate"); err != nil {
		t.Fatal(err)
	}
	if a.zeroOpen {
		t.Fatalf("zero dialog still open: %s", a.zeroError)
	}
	if len(a.assessmentRows) != 2 {
		t.Fatalf("result rows = %d", len(a.assessmentRows))
	}
}

func TestAssessmentTrialKeepsOfficialState(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.startAssessmentTrial()
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}
	if len(a.assessmentRuns) != 1 || a.assessmentRuns[0].IsOfficial {
		t.Fatalf("runs = %+v", a.assessmentRuns)
	}
	overview := a.selectedAssessmentOverview()
	if overview == nil || overview.Status != "draft" {
		t.Fatalf("status = %+v", overview)
	}
	if a.officialExportReady() {
		t.Fatal("export ready without an official run")
	}
	tt := ui.NewTester(a.view, 1280, 1000)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{"Test results", "Test calculation finished"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestAssessmentStaleMarksOfficialRun(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.startAssessmentCalculate()
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}
	if a.assessmentOfficialStale {
		t.Fatal("official run stale right after calculation")
	}
	if !a.officialExportReady() {
		t.Fatal("export not ready after calculation")
	}
	ctx := context.Background()
	cohort := a.selectedCohort()
	if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: fixture.ExamID, CohortID: cohort.ID, Kind: store.ExamImportScores,
		Report: store.ImportReport{
			FileHash: "scores-v2", Path: "scores-v2.xlsx", RowCount: 3,
			StoredPath: filepath.Join(a.dataDir, "files", "scores-v2.xlsx"),
			Sheets:     []store.ImportSheet{{Name: "成绩", Role: "scores", HeaderRow: 1, Rows: 3}},
		},
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 95, "math": 88}, Flags: map[string]string{}},
			{StudentNo: "002", Scores: map[string]float64{"chinese": 85, "math": 75}, Flags: map[string]string{}},
			{StudentNo: "003", Scores: map[string]float64{"chinese": 75, "math": 65}, Flags: map[string]string{}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	a.reloadAssessments(ctx)
	if !a.assessmentOfficialStale {
		t.Fatal("official run not stale after data change")
	}
	if len(a.assessmentOfficialStaleItems) != 1 || a.assessmentOfficialStaleItems[0] != "scores" {
		t.Fatalf("stale items = %q", a.assessmentOfficialStaleItems)
	}
	if a.officialExportReady() {
		t.Fatal("export ready while results are stale")
	}
	tt := ui.NewTester(a.view, 1280, 1000)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{"Needs recalculation", "Data has changed: Scores"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestAssessmentRowClickOpensDetail(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.startAssessmentCalculate()
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}
	tt := ui.NewTester(a.view, 1280, 1000)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Chinese"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.resultDetailOpen {
		t.Fatalf("detail not open, selection = %d", a.resultSelection.Len())
	}
	detail := a.resultDetail
	if detail.Row.Subject != "chinese" {
		t.Fatalf("row = %+v", detail.Row)
	}
	if len(detail.Students) == 0 || len(detail.Formulas) == 0 {
		t.Fatalf("detail = %+v", detail)
	}
	for _, text := range []string{
		"Result details", "Indicator breakdown", "Class average", "Participating students", "甲",
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestAssessmentRunSelectorSwitchesRuns(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.startAssessmentTrial()
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}
	a.startAssessmentCalculate()
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}
	if len(a.assessmentRuns) != 2 {
		t.Fatalf("runs = %+v", a.assessmentRuns)
	}
	if !a.assessmentRun.IsOfficial {
		t.Fatalf("displayed run = %+v", a.assessmentRun)
	}
	tt := ui.NewTester(a.view, 1280, 1000)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Final results") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Result version"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	trialLabel := a.runOptionLabelForID(a.assessmentRuns[1].ID)
	if err := tt.Click(trialLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.assessmentRun.IsOfficial {
		t.Fatal("still on the official run after switching")
	}
	if !tt.HasText("Test results") {
		t.Fatalf("texts = %q", tt.Texts())
	}
}

func TestAssessmentMultiDimensionalViews(t *testing.T) {
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.startAssessmentCalculate()
	if a.assessmentError != "" {
		t.Fatal(a.assessmentError)
	}

	tt := ui.NewTester(a.view, 1280, 1000)
	tt.SetLocale("en")
	tt.Frame()

	// 1. 默认视图：学科排名
	if a.assessmentResultTab != 0 {
		t.Fatalf("expected default tab 0, got %d", a.assessmentResultTab)
	}
	if !tt.HasText("Subject Rankings") {
		t.Fatalf("missing 'Subject Rankings' in %q", tt.Texts())
	}

	// 2. 切换到班级汇总视图
	a.assessmentResultTab = 1
	tt.Frame()
	if !tt.HasText("Class Summary") {
		t.Fatalf("missing 'Class Summary' in %q", tt.Texts())
	}
	// 点击班级行
	if len(a.assessmentRows) > 0 {
		className := a.assessmentRows[0].LeadClassName
		if err := tt.Click(className); err == nil {
			tt.Frame()
			if !a.resultDetailOpen {
				t.Fatalf("expected result detail to open on class row click, got false")
			}
			a.closeResultDetail()
		}
	}

	// 3. 切换到教师得分视图
	a.assessmentResultTab = 2
	tt.Frame()
	if !tt.HasText("Teacher Scores") {
		t.Fatalf("missing 'Teacher Scores' in %q", tt.Texts())
	}
	// 点击教师行
	if len(a.assessmentPersonal) > 0 {
		teacherName := a.assessmentPersonal[0].TeacherName
		if err := tt.Click(teacherName); err == nil {
			tt.Frame()
			if !a.resultDetailOpen {
				t.Fatalf("expected result detail to open on teacher row click, got false")
			}
			a.closeResultDetail()
		}
	}
}

func TestResultDetailUsesSingleContentClose(t *testing.T) {
	a := newTestApp(t)
	a.page = pageAssessments
	a.resultDetailOpen = true
	a.resultDetail = resultDetail{
		Row: store.StoredResultRow{
			Subject:       "chinese",
			StatGroupName: "高一(1)班",
			LeadClassName: "高一(1)班",
			TeacherNames:  "张老师",
		},
		PlanLabel: "默认方案",
	}

	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)
	if _, ok := tt.AccessByRole(ui.RoleButton, "Cancel"); ok {
		t.Fatal("result detail still shows a footer cancel button")
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, locales.AssessmentDetailClose()); !ok {
		t.Fatalf("missing content close button in %q", tt.Texts())
	}
	if err := tt.Click(locales.AssessmentDetailClose()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.resultDetailOpen {
		t.Fatal("content close button did not close the result detail")
	}
}
