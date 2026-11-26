package pipeline_test

import (
	"context"
	"path/filepath"
	"testing"

	"tally/internal/calc"
	"tally/internal/pipeline"
	"tally/internal/store"
)

func fp(v float64) *float64 {
	return &v
}

func setupPipelineTestAssessment(t *testing.T, class2Counts, class3Counts map[string]*float64) (*store.Store, int64, int64, map[string]int64) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "tally.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	ver, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班", "2班", "3班"},
			Students: []store.ImportStudent{
				{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", Status: "active"},
				{StudentNo: "002", Name: "乙", ClassName: "2班", Track: "physics", Status: "active"},
				{StudentNo: "003", Name: "丙", ClassName: "3班", Track: "physics", Status: "active"},
			},
		},
		Report: store.ImportReport{FileHash: "p-stu", Path: "stu.xlsx", RowCount: 3},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Assignments: []store.ImportAssignment{
				{ClassName: "1班", Subject: "chinese", TeacherName: "张老师"},
				{ClassName: "2班", Subject: "chinese", TeacherName: "李老师"},
				{ClassName: "3班", Subject: "chinese", TeacherName: "王老师"},
			},
		},
		Report: store.ImportReport{FileHash: "p-assign", Path: "assign.xlsx", RowCount: 3},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "baselines",
		Data: store.ImportData{
			ClassBaselines: []store.ImportClassBaseline{
				{ClassName: "1班", Subject: "chinese", Average: 95},
				{ClassName: "2班", Subject: "chinese", Average: 90},
				{ClassName: "3班", Subject: "chinese", Average: 85},
			},
			GradeBaselines: map[string]float64{"chinese": 90},
		},
		Report: store.ImportReport{FileHash: "p-base", Path: "base.xlsx", RowCount: 3},
	}); err != nil {
		t.Fatal(err)
	}

	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "管道测试期中", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportRoster,
		Report: store.ImportReport{FileHash: "p-roster", Path: "roster.xlsx", RowCount: 3},
		Data: store.ImportData{Roster: []store.ImportRoster{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", TotalScore: fp(140)},
			{StudentNo: "002", Name: "乙", ClassName: "2班", Track: "physics", TotalScore: fp(120)},
			{StudentNo: "003", Name: "丙", ClassName: "3班", Track: "physics", TotalScore: fp(100)},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportScores,
		Report: store.ImportReport{FileHash: "p-scores", Path: "scores.xlsx", RowCount: 3},
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 100}},
			{StudentNo: "002", Scores: map[string]float64{"chinese": 90}},
			{StudentNo: "003", Scores: map[string]float64{"chinese": 80}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportCutoffs,
		Report: store.ImportReport{FileHash: "p-cutoffs", Path: "cutoffs.xlsx", RowCount: 2},
		Data: store.ImportData{
			SubjectCutoffs: []store.ImportSubjectCutoff{{Subject: "chinese", Track: "physics", Value: 60}},
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
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportTargets,
		Report: store.ImportReport{FileHash: "p-targets", Path: "targets.xlsx", RowCount: 3},
		Data: store.ImportData{ClassTargets: []store.ImportClassTarget{
			{ClassName: "1班", Counts: map[string]*float64{
				"qingbei": fp(0), "c9": fp(0), "project985": fp(0),
				"project211": fp(1), "special_control": fp(1), "undergraduate": fp(1),
				"single_subject": fp(2),
			}},
			{ClassName: "2班", Counts: class2Counts},
			{ClassName: "3班", Counts: class3Counts},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	ec, err := s.GetExamCohort(ctx, exam.ID, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	assess, err := s.AssessmentForExamCohort(ctx, ec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAssessmentStatGroups(ctx, assess.ID); err != nil {
		t.Fatal(err)
	}
	classes, err := s.ListClasses(ctx, cohort.ID, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	classIDs := make(map[string]int64, len(classes))
	for _, c := range classes {
		classIDs[c.Name] = c.ID
	}
	return s, assess.ID, ver.ID, classIDs
}

func TestCalculateAssessmentConsecutiveOfficialRuns(t *testing.T) {
	ctx := context.Background()
	normalCounts := map[string]*float64{
		"qingbei": fp(0), "c9": fp(0), "project985": fp(0),
		"project211": fp(1), "special_control": fp(1), "undergraduate": fp(1),
	}
	s, assessID, verID, _ := setupPipelineTestAssessment(t, normalCounts, normalCounts)

	if _, err := pipeline.CalculateAssessment(ctx, s, assessID, verID); err != nil {
		t.Fatalf("CalculateAssessment 1: %v", err)
	}
	if _, err := pipeline.CalculateAssessment(ctx, s, assessID, verID); err != nil {
		t.Fatalf("CalculateAssessment 2: %v", err)
	}
	runs, err := s.ListAssessmentRuns(ctx, assessID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %+v", runs)
	}
	if !runs[0].IsOfficial || runs[1].IsOfficial {
		t.Fatalf("expected only newest run official, got %+v", runs)
	}
}

func TestCalculateAssessmentWithExcludedAndZeroUndergraduateClasses(t *testing.T) {
	ctx := context.Background()
	// 2班：非零高优目标（special_control=2）但本科目标为 0（零本科目标班级）
	class2ZeroUndergrad := map[string]*float64{
		"qingbei": fp(0), "c9": fp(0), "project985": fp(0),
		"project211": fp(0), "special_control": fp(2), "undergraduate": fp(0),
	}
	// 3班：六档全为 0，先验证走方案 zeroTargetRule 触发 ZeroTargetError，再验证排除后顺利出分。
	class3AllZero := map[string]*float64{
		"qingbei": fp(0), "c9": fp(0), "project985": fp(0),
		"project211": fp(0), "special_control": fp(0), "undergraduate": fp(0),
	}
	s, assessID, verID, classIDs := setupPipelineTestAssessment(t, class2ZeroUndergrad, class3AllZero)

	// 未排除 3班 时应交由 calc 的 zeroTargetRule 返回 *calc.ZeroTargetError，而非提前被 missing_stat_target 卡死。
	_, err := pipeline.CalculateAssessment(ctx, s, assessID, verID)
	zeroErr, ok := err.(*calc.ZeroTargetError)
	if !ok || len(zeroErr.Issues) != 1 || zeroErr.Issues[0].ClassName != "3班" {
		t.Fatalf("expected ZeroTargetError for 3班, got %T (%v)", err, err)
	}

	// 将 3班 设为不参评完成率（ExcludeAssessmentClass）后再次正式核算，应顺利出分不卡死。
	if err := s.ExcludeAssessmentClass(ctx, assessID, classIDs["3班"], "艺体班不参评"); err != nil {
		t.Fatalf("ExcludeAssessmentClass: %v", err)
	}
	res, err := pipeline.CalculateAssessment(ctx, s, assessID, verID)
	if err != nil {
		t.Fatalf("CalculateAssessment with excluded & zero-undergrad classes failed: %v", err)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("expected 3 result rows, got %d", len(res.Rows))
	}
	byClass := map[string]calc.Row{}
	for _, r := range res.Rows {
		byClass[r.LeadClassName] = r
	}
	if byClass["2班"].PassScore != 0 || byClass["2班"].CompletionScore <= 0 {
		t.Fatalf("expected 2班 PassScore=0 and positive CompletionScore, got %+v", byClass["2班"])
	}
	if byClass["3班"].CompletionParticipating || byClass["3班"].CompletionScore != 0 || byClass["3班"].PassScore != 0 {
		t.Fatalf("expected 3班 excluded with CompletionScore=0 PassScore=0, got %+v", byClass["3班"])
	}
	if byClass["1班"].PassScore <= 0 || byClass["1班"].CompletionScore <= 0 {
		t.Fatalf("expected 1班 normal scores, got %+v", byClass["1班"])
	}
}
