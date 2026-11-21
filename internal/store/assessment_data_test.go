package store

import (
	"context"
	"path/filepath"
	"testing"

	"tally/internal/calc"
)

type assessmentFixture struct {
	Store        *Store
	ExamID       int64
	CohortID     int64
	AssessmentID int64
	VersionID    int64
}

func openAssessmentFixture(t *testing.T) assessmentFixture {
	t.Helper()
	ctx := context.Background()
	opened, err := Open(ctx, filepath.Join(t.TempDir(), "tally.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	version, err := opened.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cohort, err := opened.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := opened.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ApplyStageImport(ctx, cohort.ID, stage.ID, StageImportRequest{
		Kind: "students",
		Data: ImportData{
			Classes: []string{"1班", "2班"},
			Students: []ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", Status: "active"},
				{StudentNo: "002", Name: "乙", ClassName: "1班", Track: "physics", Status: "active"},
				{StudentNo: "003", Name: "丙", ClassName: "2班", Track: "physics", Status: "active"},
			},
		},
		Report: ImportReport{
			FileHash: "students", StoredPath: filepath.Join(t.TempDir(), "students.xlsx"), RowCount: 3,
			Sheets: []ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 3}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ApplyStageImport(ctx, cohort.ID, stage.ID, StageImportRequest{
		Kind: "assignments",
		Data: ImportData{
			Classes: []string{"1班", "2班"},
			Assignments: []ImportAssignment{
				{ClassName: "1班", Subject: "chinese", TeacherName: "张老师"},
				{ClassName: "2班", Subject: "chinese", TeacherName: "李老师"},
			},
		},
		Report: ImportReport{
			FileHash: "teachers", StoredPath: filepath.Join(t.TempDir(), "teachers.xlsx"), RowCount: 2,
			Sheets: []ImportSheet{{Name: "任课教师", Role: "teachers", HeaderRow: 1, Rows: 2}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ApplyStageImport(ctx, cohort.ID, stage.ID, StageImportRequest{
		Kind: "baselines",
		Data: ImportData{
			ClassBaselines: []ImportClassBaseline{
				{ClassName: "1班", Subject: "chinese", Average: 90},
				{ClassName: "2班", Subject: "chinese", Average: 88},
			},
			GradeBaselines: map[string]float64{"chinese": 89},
		},
		Report: ImportReport{
			FileHash: "baselines", StoredPath: filepath.Join(t.TempDir(), "baselines.xlsx"), RowCount: 2,
			Sheets: []ImportSheet{{Name: "入口成绩", Role: "baselines", HeaderRow: 1, Rows: 2}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	exam, err := opened.CreateExam(ctx, CreateExamInput{
		AcademicYearStart: 2026, Term: ExamTermFirst, ExamType: ExamMidterm,
		Name: "期中考试", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ApplyExamImport(ctx, ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: ExamImportRoster,
		Report: ImportReport{
			FileHash: "roster", StoredPath: filepath.Join(t.TempDir(), "roster.xlsx"), RowCount: 3,
			Sheets: []ImportSheet{{Name: "报名名单", Role: "roster", HeaderRow: 1, Rows: 3}},
		},
		Data: ImportData{Roster: []ImportRoster{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", TotalScore: floatPointer(140)},
			{StudentNo: "002", Name: "乙", ClassName: "1班", Track: "physics", TotalScore: floatPointer(120)},
			{StudentNo: "003", Name: "丙", ClassName: "2班", Track: "physics", TotalScore: floatPointer(100)},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ApplyExamImport(ctx, ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: ExamImportScores,
		Report: ImportReport{
			FileHash: "scores", StoredPath: filepath.Join(t.TempDir(), "scores.xlsx"), RowCount: 3,
			Sheets: []ImportSheet{{Name: "成绩", Role: "scores", HeaderRow: 1, Rows: 3}},
		},
		Data: ImportData{Students: []ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 100}, Flags: map[string]string{}},
			{StudentNo: "002", Scores: map[string]float64{"chinese": 90}, Flags: map[string]string{}},
			{StudentNo: "003", Scores: map[string]float64{"chinese": 80}, Flags: map[string]string{}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ApplyExamImport(ctx, ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: ExamImportCutoffs,
		Report: ImportReport{
			FileHash: "cutoffs", StoredPath: filepath.Join(t.TempDir(), "cutoffs.xlsx"), RowCount: 2,
			Sheets: []ImportSheet{{Name: "分数线", Role: "cutoffs", HeaderRow: 1, Rows: 2}},
		},
		Data: ImportData{
			SubjectCutoffs: []ImportSubjectCutoff{{Subject: "chinese", Track: "physics", Value: 60}},
			TierCutoffs: []ImportTierCutoff{
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
	if _, err := opened.ApplyExamImport(ctx, ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: ExamImportTargets,
		Report: ImportReport{
			FileHash: "targets", StoredPath: filepath.Join(t.TempDir(), "targets.xlsx"), RowCount: 2,
			Sheets: []ImportSheet{{Name: "目标", Role: "targets", HeaderRow: 1, Rows: 2}},
		},
		Data: ImportData{ClassTargets: []ImportClassTarget{
			{ClassName: "1班", Counts: map[string]*float64{
				"qingbei": floatPointer(0), "c9": floatPointer(0), "project985": floatPointer(0),
				"project211": floatPointer(1), "special_control": floatPointer(1),
				"undergraduate": floatPointer(1),
			}},
			{ClassName: "2班", Counts: map[string]*float64{
				"qingbei": floatPointer(0), "c9": floatPointer(0), "project985": floatPointer(0),
				"project211": floatPointer(1), "special_control": floatPointer(1),
				"undergraduate": floatPointer(1),
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	examCohort, err := opened.GetExamCohort(ctx, exam.ID, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := opened.AssessmentForExamCohort(ctx, examCohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.EnsureAssessmentStatGroups(ctx, assessment.ID); err != nil {
		t.Fatal(err)
	}
	return assessmentFixture{
		Store: opened, ExamID: exam.ID, CohortID: cohort.ID,
		AssessmentID: assessment.ID, VersionID: version.ID,
	}
}

func floatPointer(value float64) *float64 {
	return &value
}

func TestLoadAssessmentDataBuildsCalcInput(t *testing.T) {
	fixture := openAssessmentFixture(t)
	ctx := context.Background()
	data, err := fixture.Store.LoadAssessmentData(ctx, fixture.AssessmentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Classes) != 2 || len(data.Students) != 3 {
		t.Fatalf("classes=%d students=%d", len(data.Classes), len(data.Students))
	}
	if len(data.SubjectCutoffs) != 1 || len(data.TierCutoffs) != 6 {
		t.Fatalf("cutoffs = %d/%d", len(data.SubjectCutoffs), len(data.TierCutoffs))
	}
	if len(data.ClassTargets) != 2 || len(data.StatGroups) != 2 {
		t.Fatalf("targets=%d groups=%d", len(data.ClassTargets), len(data.StatGroups))
	}
	if data.GradeBaselines["chinese"] < 88 || data.GradeBaselines["chinese"] > 90 {
		t.Fatalf("grade baseline = %v", data.GradeBaselines["chinese"])
	}
	if data.Students[0].Scores["chinese"] != 100 {
		t.Fatalf("student scores = %+v", data.Students[0].Scores)
	}
}

func TestSaveAssessmentRunAndLatest(t *testing.T) {
	fixture := openAssessmentFixture(t)
	ctx := context.Background()
	result := calc.Result{
		Rows: []calc.Row{{
			StatGroupID: "1", StatGroupName: "1班", Subject: "chinese", LeadClassID: "1",
			LeadClassName: "1班", ClassCount: 2, ClassAverage: 95, Total: 88, Rank: 1,
			IndicatorValues: map[string]float64{"improvement": 5},
		}},
		Personal: []calc.PersonalScore{{
			TeacherID: "1", TeacherName: "张老师", Grade: "g1", Subject: "chinese", Score: 88,
		}},
	}
	runID, err := fixture.Store.SaveAssessmentRun(ctx, fixture.AssessmentID, fixture.VersionID, "hash", result)
	if err != nil {
		t.Fatal(err)
	}
	run, rows, personal, err := fixture.Store.LatestAssessmentRun(ctx, fixture.AssessmentID)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID != runID || len(rows) != 1 || len(personal) != 1 {
		t.Fatalf("run=%+v rows=%d personal=%d", run, len(rows), len(personal))
	}
	if rows[0].Total != 88 || personal[0].TeacherName != "张老师" {
		t.Fatalf("rows = %+v personal=%+v", rows, personal)
	}
	context, err := fixture.Store.LoadAssessmentContext(ctx, fixture.AssessmentID)
	if err != nil {
		t.Fatal(err)
	}
	if context.Assessment.Status != "calculated" || context.Assessment.CurrentRunID != runID {
		t.Fatalf("assessment = %+v", context.Assessment)
	}
}

func TestAssessmentTargetAdjustmentAndExclusion(t *testing.T) {
	fixture := openAssessmentFixture(t)
	ctx := context.Background()
	data, err := fixture.Store.LoadAssessmentData(ctx, fixture.AssessmentID)
	if err != nil {
		t.Fatal(err)
	}
	classID := data.Classes[0].ID
	if err := fixture.Store.AdjustAssessmentTarget(ctx, fixture.AssessmentID, classID, "qingbei", 2, "补录"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Store.ExcludeAssessmentClass(ctx, fixture.AssessmentID, data.Classes[1].ID, "不参与"); err != nil {
		t.Fatal(err)
	}
	updated, err := fixture.Store.LoadAssessmentData(ctx, fixture.AssessmentID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.ExcludedCompletionClasses[data.Classes[1].ID] {
		t.Fatal("class not excluded")
	}
	found := false
	for _, target := range updated.ClassTargets {
		if target.ClassID == classID && target.Counts["qingbei"] != nil && *target.Counts["qingbei"] == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %+v", updated.ClassTargets)
	}
}

func TestEnsureAssessmentStatGroupsFromTeachingGroups(t *testing.T) {
	fixture := openAssessmentFixture(t)
	ctx := context.Background()
	groups, err := fixture.Store.ListExamStatGroups(ctx, fixture.AssessmentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d", len(groups))
	}
	for _, group := range groups {
		if len(group.Members) != 1 {
			t.Fatalf("members = %+v", group.Members)
		}
		if group.Target == nil {
			t.Fatalf("missing target for %s", group.Name)
		}
	}
}
