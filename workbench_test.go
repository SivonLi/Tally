package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/xuri/excelize/v2"

	"tally/internal/calc"
	"tally/internal/pipeline"
	"tally/internal/store"
)

func seedBatch(t *testing.T, a *app, missingTarget bool) store.Batch {
	t.Helper()
	ctx := context.Background()
	batch, err := a.store.CreateBatch(ctx, store.CreateBatchInput{
		AcademicYear: "2025-2026",
		Semester:     "spring",
		ExamType:     "midterm",
		Grade:        "g1",
	})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]float64{
		"qingbei":         0,
		"c9":              0,
		"project985":      0,
		"project211":      1,
		"special_control": 1,
		"undergraduate":   1,
		"single_subject":  3,
	}
	var classTargets []store.ImportClassTarget
	for _, name := range []string{"A", "B"} {
		target := store.ImportClassTarget{ClassName: name, Counts: map[string]*float64{}}
		for tier, value := range counts {
			if missingTarget && name == "A" && tier == "qingbei" {
				continue
			}
			copied := value
			target.Counts[tier] = &copied
		}
		classTargets = append(classTargets, target)
	}
	data := store.ImportData{
		Classes:      []string{"A", "B"},
		ClassTargets: classTargets,
		Assignments: []store.ImportAssignment{
			{ClassName: "A", Subject: "chinese", TeacherName: "Teacher A"},
			{ClassName: "B", Subject: "chinese", TeacherName: "Teacher B"},
		},
		Teachers: []string{"Teacher A", "Teacher B"},
		ClassBaselines: []store.ImportClassBaseline{
			{ClassName: "A", Subject: "chinese", Average: 90},
			{ClassName: "B", Subject: "chinese", Average: 90},
		},
		GradeBaselines: map[string]float64{"chinese": 90},
		SubjectCutoffs: []store.ImportSubjectCutoff{
			{Subject: "chinese", Track: "physics", Value: 60},
		},
		TierCutoffs: []store.ImportTierCutoff{
			{TierKey: "qingbei", Track: "physics", Value: 130},
			{TierKey: "c9", Track: "physics", Value: 120},
			{TierKey: "project985", Track: "physics", Value: 110},
			{TierKey: "project211", Track: "physics", Value: 100},
			{TierKey: "special_control", Track: "physics", Value: 90},
			{TierKey: "undergraduate", Track: "physics", Value: 1},
		},
	}
	for _, name := range []string{"A", "B"} {
		for index, score := range []float64{100, 90, 80} {
			total := []float64{140, 120, 100}[index]
			data.Students = append(data.Students, store.ImportStudent{
				StudentNo:  name + strconv.Itoa(index+1),
				Name:       name + " Student " + strconv.Itoa(index+1),
				ClassName:  name,
				Track:      "physics",
				TotalScore: &total,
				Scores:     map[string]float64{"chinese": score},
				Flags:      map[string]string{},
			})
		}
	}
	if _, err := a.store.MergeImport(ctx, batch.ID, data, store.ImportReport{}); err != nil {
		t.Fatal(err)
	}
	a.reloadBatches(ctx)
	a.selectedBatchID = strconv.FormatInt(batch.ID, 10)
	a.loadLatestRun(ctx)
	return batch
}

func TestBatchPageShowsNewBatch(t *testing.T) {
	a := newTestApp(t)
	a.tab = 1
	tt := ui.NewTester(a.view, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("New batch"); err != nil {
		t.Fatal(err)
	}
	if !a.newBatchOpen {
		t.Fatal("new batch dialog not open")
	}
	tt.Frame()
	if err := tt.Click("Create batch"); err != nil {
		t.Fatal(err)
	}
	if len(a.batches) != 1 {
		t.Fatalf("batches = %d", len(a.batches))
	}
	tt.Frame()
	for _, text := range []string{"Batches", "2025-2026 Spring Grade 1 Midterm", "Import workbook", "Calculate"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestCalculateAndExportBatch(t *testing.T) {
	a := newTestApp(t)
	batch := seedBatch(t, a, false)
	version := a.publishedByLabel(a.calculateVersion)
	if version == nil {
		t.Fatal("no published version")
	}
	result, err := pipeline.CalculateBatch(context.Background(), a.store, batch.ID, version.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d", len(result.Rows))
	}
	if len(result.Personal) != 2 {
		t.Fatalf("personal = %d", len(result.Personal))
	}
	path := filepath.Join(a.dataDir, "result.xlsx")
	if err := pipeline.ExportLatest(context.Background(), a.store, batch.ID, path); err != nil {
		t.Fatal(err)
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

func TestZeroTargetDialogAndFill(t *testing.T) {
	a := newTestApp(t)
	batch := seedBatch(t, a, true)
	ctx := context.Background()
	version := a.publishedByLabel(a.calculateVersion)
	_, err := pipeline.CalculateBatch(ctx, a.store, batch.ID, version.Version.ID)
	if err == nil {
		t.Fatal("expected zero target error")
	}
	zeroError, ok := err.(*calc.ZeroTargetError)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	a.finishCalculate(calc.Result{}, zeroError)
	if !a.zeroOpen || len(a.zeroIssues) == 0 {
		t.Fatalf("zero dialog = %v issues=%d", a.zeroOpen, len(a.zeroIssues))
	}
	a.tab = 1
	tt := ui.NewTester(a.view, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Fill missing targets") {
		t.Errorf("texts = %q", tt.Texts())
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
	if len(a.resultRows) != 2 {
		t.Fatalf("result rows = %d", len(a.resultRows))
	}
}
