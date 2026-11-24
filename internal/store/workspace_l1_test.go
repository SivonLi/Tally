package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"tally/internal/calc"
	"tally/internal/store"
)

func openL1Store(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "tally.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.EnsureDefaultPlan(ctx); err != nil {
		t.Fatalf("ensure default plan: %v", err)
	}
	return s
}

func seedCohortBaseData(t *testing.T, s *store.Store, year int, hashPrefix string) (store.Cohort, store.CohortStage) {
	t.Helper()
	ctx := context.Background()
	cohort, err := s.CreateCohort(ctx, year, 2026)
	if err != nil {
		t.Fatalf("create cohort %d: %v", year, err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatalf("get stage %d: %v", year, err)
	}
	ent1, ent2, ent3 := 540.0, 500.0, 520.0
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Students: []store.ImportStudent{
				{StudentNo: hashPrefix + "01", Name: "甲", ClassName: "1班", Track: "physics", Status: "active", TotalScore: &ent1, SourceRow: 2},
				{StudentNo: hashPrefix + "02", Name: "乙", ClassName: "1班", Track: "history", Status: "active", TotalScore: &ent2, SourceRow: 3},
				{StudentNo: hashPrefix + "03", Name: "丙", ClassName: "2班", Track: "physics", Status: "transferred", TotalScore: &ent3, SourceRow: 4},
			},
		},
		Report: store.ImportReport{
			FileHash: hashPrefix + "-students",
			Path:     hashPrefix + "-students.xlsx",
			RowCount: 3,
			Sheets:   []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 3}},
		},
	}); err != nil {
		t.Fatalf("import students: %v", err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Assignments: []store.ImportAssignment{
				{ClassName: "1班", Subject: "chinese", TeacherName: "张老师", SourceRow: 2},
				{ClassName: "2班", Subject: "chinese", TeacherName: "李老师", SourceRow: 3},
			},
		},
		Report: store.ImportReport{
			FileHash: hashPrefix + "-assignments",
			Path:     hashPrefix + "-assignments.xlsx",
			RowCount: 2,
			Sheets:   []store.ImportSheet{{Name: "任课", Role: "assignments", HeaderRow: 1, Rows: 2}},
		},
	}); err != nil {
		t.Fatalf("import assignments: %v", err)
	}
	ratio1 := 1.05
	ratio2 := 0.95
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "baselines",
		Data: store.ImportData{
			ClassBaselines: []store.ImportClassBaseline{
				{ClassName: "1班", Subject: "chinese", Average: 95, Ratio: &ratio1, SourceRow: 2},
				{ClassName: "2班", Subject: "chinese", Average: 85, Ratio: &ratio2, SourceRow: 3},
			},
			GradeBaselines: map[string]float64{"chinese": 90, "total": 520},
		},
		Report: store.ImportReport{
			FileHash: hashPrefix + "-baselines",
			Path:     hashPrefix + "-baselines.xlsx",
			RowCount: 2,
			Sheets:   []store.ImportSheet{{Name: "基数", Role: "baselines", HeaderRow: 1, Rows: 2}},
		},
	}); err != nil {
		t.Fatalf("import baselines: %v", err)
	}
	return cohort, stage
}

func TestWorkspaceSummaryAndCohortFilters(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	c1, st1 := seedCohortBaseData(t, s, 2026, "c1")
	c2, _ := seedCohortBaseData(t, s, 2025, "c2")

	exam1, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "2026级期中", ExamDate: "2026-11-10", CohortIDs: []int64{c1.ID},
	})
	if err != nil {
		t.Fatalf("create exam1: %v", err)
	}
	exam2, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "2025级期中", ExamDate: "2026-11-12", CohortIDs: []int64{c2.ID},
	})
	if err != nil {
		t.Fatalf("create exam2: %v", err)
	}
	if _, err := s.GenerateExamRoster(ctx, exam1.ID, c1.ID, "physics"); err != nil {
		t.Fatalf("generate roster: %v", err)
	}
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam1.ID, CohortID: c1.ID, Kind: store.ExamImportScores,
		Data: store.ImportData{
			Students: []store.ImportStudent{
				{StudentNo: "c101", Name: "甲", Scores: map[string]float64{"chinese": 120}, SourceRow: 2},
				{StudentNo: "c102", Name: "乙", Scores: map[string]float64{"chinese": 88}, SourceRow: 3},
			},
		},
		Report: store.ImportReport{
			FileHash: "exam1-scores", Path: "scores.xlsx", RowCount: 2,
			Sheets: []store.ImportSheet{{Name: "成绩", Role: "scores", HeaderRow: 1, Rows: 2}},
		},
	}); err != nil {
		t.Fatalf("import exam1 scores: %v", err)
	}
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam1.ID, CohortID: c1.ID, Kind: store.ExamImportCutoffs,
		Data: store.ImportData{
			SubjectCutoffs: []store.ImportSubjectCutoff{
				{Subject: "chinese", Track: "physics", Value: 100},
				{Subject: "chinese", Track: "history", Value: 95},
			},
		},
		Report: store.ImportReport{
			FileHash: "exam1-cutoffs", Path: "cutoffs.xlsx", RowCount: 2,
			Sheets: []store.ImportSheet{{Name: "分数线", Role: "cutoffs", HeaderRow: 1, Rows: 2}},
		},
	}); err != nil {
		t.Fatalf("import exam1 cutoffs: %v", err)
	}

	// 验证工作区隔离查询
	examsC1, err := s.ListExamsByCohort(ctx, c1.ID)
	if err != nil || len(examsC1) != 1 || examsC1[0].ID != exam1.ID {
		t.Fatalf("ListExamsByCohort(c1) = %+v, err=%v", examsC1, err)
	}
	examsAll, err := s.ListExamsByCohort(ctx, 0)
	if err != nil || len(examsAll) != 2 {
		t.Fatalf("ListExamsByCohort(0) len=%d, err=%v", len(examsAll), err)
	}
	overviewsC2, err := s.ListAssessmentOverviewByCohort(ctx, c2.ID)
	if err != nil || len(overviewsC2) != 1 || overviewsC2[0].ExamID != exam2.ID {
		t.Fatalf("ListAssessmentOverviewByCohort(c2) = %+v, err=%v", overviewsC2, err)
	}
	qualityC1, err := s.ListExamQualitySummariesByCohort(ctx, c1.ID)
	if err != nil {
		t.Fatalf("ListExamQualitySummariesByCohort(c1): %v", err)
	}
	for _, q := range qualityC1 {
		if q.CohortID != c1.ID {
			t.Fatalf("unexpected cohort %d in c1 quality", q.CohortID)
		}
	}
	runsC1, err := s.ListWorkspaceImportRuns(ctx, c1.ID, st1.ID)
	if err != nil {
		t.Fatalf("ListWorkspaceImportRuns(c1): %v", err)
	}
	// c1 有 3 条基础导入 + 2 条考试导入 = 5 条
	if len(runsC1) != 5 {
		t.Fatalf("expected 5 workspace import runs for c1, got %d", len(runsC1))
	}

	// 保存一次正式核算并验证 WorkspaceSummary 与结果学生得分/达线
	ec1, err := s.GetExamCohort(ctx, exam1.ID, c1.ID)
	if err != nil {
		t.Fatalf("get exam cohort: %v", err)
	}
	assess1, err := s.AssessmentForExamCohort(ctx, ec1.ID)
	if err != nil {
		t.Fatalf("assessment: %v", err)
	}
	planVer, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	batchData, err := s.LoadAssessmentData(ctx, assess1.ID)
	if err != nil {
		t.Fatalf("load data: %v", err)
	}
	hash, err := store.InputHash(planVer.Config, batchData)
	if err != nil {
		t.Fatalf("input hash: %v", err)
	}
	summaryHash, err := store.InputSummary(planVer.Config, batchData)
	if err != nil {
		t.Fatalf("input summary: %v", err)
	}
	runID, err := s.SaveAssessmentRun(ctx, assess1.ID, planVer.ID, hash, summaryHash, calc.Result{
		Rows: []calc.Row{
			{
				StatGroupID: "1", StatGroupName: "1班", Subject: "chinese",
				LeadClassID: "1", LeadClassName: "1班", TeacherNames: []string{"张老师"},
				GradeAverage: 104, Total: 85.5, CompletionParticipating: true, CompletionScore: 18,
				Rank: 1, TierIndex: 0, Points: 70,
				Participants: []calc.Participant{
					{StudentNo: "c101", Name: "甲"},
					{StudentNo: "c102", Name: "乙"},
				},
			},
		},
		Personal: []calc.PersonalScore{
			{TeacherID: "1", TeacherName: "张老师", Grade: "g1", Subject: "chinese", GroupPoints: []float64{70}, Score: 70},
		},
	}, true)
	if err != nil {
		t.Fatalf("save assessment run: %v", err)
	}

	ws, err := s.WorkspaceSummary(ctx, c1.ID, st1.ID)
	if err != nil {
		t.Fatalf("WorkspaceSummary: %v", err)
	}
	if ws.Counts.Classes != 2 || ws.Counts.Students != 3 || ws.ActiveStudents != 2 || ws.InactiveStudents != 1 {
		t.Fatalf("unexpected student counts: %+v", ws)
	}
	if ws.PhysicsStudents != 2 || ws.HistoryStudents != 1 || ws.BaselineCoveredClasses != 2 || ws.BaselineSubjectCount != 1 || ws.GradeBaselineTotal != 520 {
		t.Fatalf("unexpected breakdown: %+v", ws)
	}
	if ws.ExamCount != 1 || ws.CalculatedExamCount != 1 || ws.LatestAssessment == nil {
		t.Fatalf("unexpected exam summary: %+v", ws)
	}
	if !ws.LatestAssessment.HasOfficialRun || ws.LatestAssessment.OfficialStale || ws.LatestAssessment.ResultRowCount != 1 || ws.LatestAssessment.PersonalScoreCount != 1 {
		t.Fatalf("unexpected latest assessment: %+v", ws.LatestAssessment)
	}

	// 验证 AssessmentRowStudents 返回真实单科成绩与达线判定
	rowStudents, err := s.AssessmentRowStudents(ctx, runID, 1, "chinese")
	if err != nil {
		t.Fatalf("AssessmentRowStudents: %v", err)
	}
	if len(rowStudents) != 2 {
		t.Fatalf("expected 2 row students, got %d", len(rowStudents))
	}
	if rowStudents[0].Score == nil || *rowStudents[0].Score != 120 || !rowStudents[0].Passed || rowStudents[0].Cutoff != 100 {
		t.Fatalf("student 0 expected 120 passed (cutoff 100), got %+v", rowStudents[0])
	}
	if rowStudents[1].Score == nil || *rowStudents[1].Score != 88 || rowStudents[1].Passed || rowStudents[1].Cutoff != 95 {
		t.Fatalf("student 1 expected 88 not passed (cutoff 95), got %+v", rowStudents[1])
	}

	// 原地修改一名学生成绩后，WorkspaceSummary 应识别到 OfficialStale = true
	rosterEntries, _, err := s.ListExamRoster(ctx, store.ExamRosterFilter{ExamID: exam1.ID, CohortID: c1.ID, PageSize: 20})
	if err != nil || len(rosterEntries) == 0 {
		t.Fatalf("list roster: %v", err)
	}
	newScore := 130.0
	if err := s.UpdateExamScore(ctx, exam1.ID, c1.ID, rosterEntries[0].StudentID, "chinese", &newScore, ""); err != nil {
		t.Fatalf("UpdateExamScore: %v", err)
	}
	wsAfter, err := s.WorkspaceSummary(ctx, c1.ID, st1.ID)
	if err != nil {
		t.Fatalf("WorkspaceSummary after update: %v", err)
	}
	if !wsAfter.LatestAssessment.OfficialStale {
		t.Fatalf("expected OfficialStale=true after score update")
	}
}

func TestCopyAssessmentTargetsAndZeroBatch(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	c1, st1 := seedCohortBaseData(t, s, 2026, "t1")

	exam1, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "期中", ExamDate: "2026-11-10", CohortIDs: []int64{c1.ID},
	})
	if err != nil {
		t.Fatalf("create exam1: %v", err)
	}
	exam2, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamFinal,
		Name: "期末", ExamDate: "2027-01-10", CohortIDs: []int64{c1.ID},
	})
	if err != nil {
		t.Fatalf("create exam2: %v", err)
	}
	ec1, _ := s.GetExamCohort(ctx, exam1.ID, c1.ID)
	ec2, _ := s.GetExamCohort(ctx, exam2.ID, c1.ID)
	a1, _ := s.AssessmentForExamCohort(ctx, ec1.ID)
	a2, _ := s.AssessmentForExamCohort(ctx, ec2.ID)
	classes, err := s.ListClasses(ctx, c1.ID, st1.ID)
	if err != nil || len(classes) < 2 {
		t.Fatalf("list classes: %v", err)
	}
	if err := s.SetClassTarget(ctx, a1.ID, classes[0].ID, "undergraduate", 30); err != nil {
		t.Fatalf("set target 1: %v", err)
	}
	if err := s.SetClassTarget(ctx, a1.ID, classes[1].ID, "undergraduate", 20); err != nil {
		t.Fatalf("set target 2: %v", err)
	}

	prevID, err := s.FindPreviousAssessmentWithTargets(ctx, a2.ID)
	if err != nil || prevID != a1.ID {
		t.Fatalf("FindPreviousAssessmentWithTargets = %d, err=%v", prevID, err)
	}
	copied, err := s.CopyAssessmentTargets(ctx, prevID, a2.ID)
	if err != nil || copied != 2 {
		t.Fatalf("CopyAssessmentTargets = %d, err=%v", copied, err)
	}
	targets, err := s.ListClassTargets(ctx, a2.ID)
	if err != nil || len(targets) != 2 {
		t.Fatalf("ListClassTargets(a2) len=%d, err=%v", len(targets), err)
	}

	// 测试批量零目标补录与排除
	if err := s.ApplyZeroTargetBatch(ctx, a2.ID, []store.ZeroTargetBatchItem{
		{ClassID: classes[0].ID, Targets: map[string]float64{"special_control": 5, "undergraduate": 35}},
		{ClassID: classes[1].ID, Exclude: true, Reason: "实验小班不参评"},
	}, "统一补齐"); err != nil {
		t.Fatalf("ApplyZeroTargetBatch: %v", err)
	}
	data, err := s.LoadAssessmentData(ctx, a2.ID)
	if err != nil {
		t.Fatalf("LoadAssessmentData: %v", err)
	}
	if !data.ExcludedCompletionClasses[classes[1].ID] {
		t.Fatalf("expected class %d excluded from completion", classes[1].ID)
	}
	// 无原因应报错
	if err := s.ApplyZeroTargetBatch(ctx, a2.ID, []store.ZeroTargetBatchItem{
		{ClassID: classes[0].ID, Exclude: true},
	}, ""); err == nil {
		t.Fatalf("expected error when reason is empty")
	}
}

func TestInlineExamFixesAndSofficeSettings(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	c1, _ := seedCohortBaseData(t, s, 2026, "f1")
	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "纠错测试", ExamDate: "2026-11-10", CohortIDs: []int64{c1.ID},
	})
	if err != nil {
		t.Fatalf("create exam: %v", err)
	}
	if _, err := s.GenerateExamRoster(ctx, exam.ID, c1.ID, "physics"); err != nil {
		t.Fatalf("generate roster: %v", err)
	}
	roster, _, err := s.ListExamRoster(ctx, store.ExamRosterFilter{ExamID: exam.ID, CohortID: c1.ID, PageSize: 10})
	if err != nil || len(roster) == 0 {
		t.Fatalf("list roster: %v", err)
	}
	sid := roster[0].StudentID
	if err := s.UpdateExamRosterTrack(ctx, exam.ID, c1.ID, sid, "history"); err != nil {
		t.Fatalf("UpdateExamRosterTrack: %v", err)
	}
	if err := s.UpdateExamRosterTrack(ctx, exam.ID, c1.ID, sid, "nonexistent"); err == nil {
		t.Fatalf("expected error for invalid track")
	}
	over := 999.0
	if err := s.UpdateExamScore(ctx, exam.ID, c1.ID, sid, "chinese", &over, ""); err == nil {
		t.Fatalf("expected error for out-of-range score")
	}
	okScore := 112.5
	if err := s.UpdateExamScore(ctx, exam.ID, c1.ID, sid, "chinese", &okScore, ""); err != nil {
		t.Fatalf("UpdateExamScore valid: %v", err)
	}
	if err := s.UpdateExamScore(ctx, exam.ID, c1.ID, sid, "chinese", nil, "absent"); err != nil {
		t.Fatalf("UpdateExamScore absent: %v", err)
	}

	// LibreOffice 路径设置
	got, err := s.SofficePath(ctx)
	if err != nil || got != "" {
		t.Fatalf("initial SofficePath = %q, err=%v", got, err)
	}
	if err := s.SetSofficePath(ctx, "/opt/homebrew/bin/soffice"); err != nil {
		t.Fatalf("SetSofficePath: %v", err)
	}
	got, err = s.SofficePath(ctx)
	if err != nil || got != "/opt/homebrew/bin/soffice" {
		t.Fatalf("SofficePath = %q, err=%v", got, err)
	}
	if err := s.SetSofficePath(ctx, ""); err != nil {
		t.Fatalf("clear SofficePath: %v", err)
	}
	got, err = s.SofficePath(ctx)
	if err != nil || got != "" {
		t.Fatalf("cleared SofficePath = %q, err=%v", got, err)
	}
}

func TestExamImportUndoAndReimport(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	c1, _ := seedCohortBaseData(t, s, 2026, "u1")
	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "撤销测试", ExamDate: "2026-11-10", CohortIDs: []int64{c1.ID},
	})
	if err != nil {
		t.Fatalf("create exam: %v", err)
	}
	ec, _ := s.GetExamCohort(ctx, exam.ID, c1.ID)
	assess, _ := s.AssessmentForExamCohort(ctx, ec.ID)

	// 1. 导入报名名单
	rosterReq := store.ExamImportRequest{
		ExamID: exam.ID, CohortID: c1.ID, Kind: store.ExamImportRoster,
		Data: store.ImportData{
			Roster: []store.ImportRoster{
				{StudentNo: "u101", Name: "甲", ClassName: "1班", Track: "physics", SourceRow: 2},
			},
		},
		Report: store.ImportReport{
			FileHash: "undo-roster-1", Path: "roster1.xlsx", RowCount: 1,
			Sheets: []store.ImportSheet{{Name: "报名", Role: "roster", HeaderRow: 1, Rows: 1}},
		},
	}
	if _, err := s.ApplyExamImport(ctx, rosterReq); err != nil {
		t.Fatalf("import roster: %v", err)
	}
	// 2. 导入分数线与目标
	cutoffReq1 := store.ExamImportRequest{
		ExamID: exam.ID, CohortID: c1.ID, Kind: store.ExamImportCutoffs,
		Data: store.ImportData{
			SubjectCutoffs: []store.ImportSubjectCutoff{{Subject: "chinese", Track: "physics", Value: 100}},
			TierCutoffs:    []store.ImportTierCutoff{{TierKey: "undergraduate", Track: "physics", Value: 450}},
		},
		Report: store.ImportReport{
			FileHash: "undo-cutoffs-1", Path: "cutoffs1.xlsx", RowCount: 2,
			Sheets: []store.ImportSheet{{Name: "分数线", Role: "cutoffs", HeaderRow: 1, Rows: 2}},
		},
	}
	if _, err := s.ApplyExamImport(ctx, cutoffReq1); err != nil {
		t.Fatalf("import cutoffs1: %v", err)
	}
	cutoffReq2 := store.ExamImportRequest{
		ExamID: exam.ID, CohortID: c1.ID, Kind: store.ExamImportCutoffs,
		Data: store.ImportData{
			SubjectCutoffs: []store.ImportSubjectCutoff{{Subject: "chinese", Track: "physics", Value: 105}},
		},
		Report: store.ImportReport{
			FileHash: "undo-cutoffs-2", Path: "cutoffs2.xlsx", RowCount: 1,
			Sheets: []store.ImportSheet{{Name: "分数线", Role: "cutoffs", HeaderRow: 1, Rows: 1}},
		},
	}
	if _, err := s.ApplyExamImport(ctx, cutoffReq2); err != nil {
		t.Fatalf("import cutoffs2: %v", err)
	}
	val25 := 25.0
	targetReq := store.ExamImportRequest{
		ExamID: exam.ID, CohortID: c1.ID, Kind: store.ExamImportTargets,
		Data: store.ImportData{
			ClassTargets: []store.ImportClassTarget{
				{ClassName: "1班", Counts: map[string]*float64{"undergraduate": &val25}},
			},
		},
		Report: store.ImportReport{
			FileHash: "undo-targets-1", Path: "targets1.xlsx", RowCount: 1,
			Sheets: []store.ImportSheet{{Name: "目标", Role: "targets", HeaderRow: 1, Rows: 1}},
		},
	}
	if _, err := s.ApplyExamImport(ctx, targetReq); err != nil {
		t.Fatalf("import targets: %v", err)
	}

	runs, err := s.ListExamImportRuns(ctx, ec.ID)
	if err != nil || len(runs) != 4 {
		t.Fatalf("ListExamImportRuns len=%d, err=%v", len(runs), err)
	}
	// 最新一条可撤销，旧条被阻断
	canLatest, _, err := s.CanUndoImport(ctx, runs[0].ID)
	if err != nil || !canLatest {
		t.Fatalf("expected latest exam import undoable, got %v err=%v", canLatest, err)
	}
	canOlder, reason, err := s.CanUndoImport(ctx, runs[1].ID)
	if err != nil || canOlder || reason != "newer_import" {
		t.Fatalf("expected older blocked by newer_import, got %v %q %v", canOlder, reason, err)
	}
	if err := s.UndoImportRun(ctx, runs[1].ID); !errors.Is(err, store.ErrImportUndoBlocked) {
		t.Fatalf("expected ErrImportUndoBlocked, got %v", err)
	}

	// 依次撤销 targets -> cutoffs2（恢复 100） -> cutoffs1（删除） -> roster（删除）
	if err := s.UndoImportRun(ctx, runs[0].ID); err != nil {
		t.Fatalf("undo targets: %v", err)
	}
	targets, _ := s.ListClassTargets(ctx, assess.ID)
	if len(targets) != 0 {
		t.Fatalf("expected targets deleted after undo, got %+v", targets)
	}
	if err := s.UndoImportRun(ctx, runs[1].ID); err != nil {
		t.Fatalf("undo cutoffs2: %v", err)
	}
	subjCutoffs, _ := s.ListExamSubjectCutoffs(ctx, exam.ID, c1.ID)
	if len(subjCutoffs) != 1 || subjCutoffs[0].Cutoff != 100 {
		t.Fatalf("expected chinese cutoff restored to 100, got %+v", subjCutoffs)
	}
	if err := s.UndoImportRun(ctx, runs[2].ID); err != nil {
		t.Fatalf("undo cutoffs1: %v", err)
	}
	subjCutoffs, _ = s.ListExamSubjectCutoffs(ctx, exam.ID, c1.ID)
	if len(subjCutoffs) != 0 {
		t.Fatalf("expected cutoffs empty after undo, got %+v", subjCutoffs)
	}
	if err := s.UndoImportRun(ctx, runs[3].ID); err != nil {
		t.Fatalf("undo roster: %v", err)
	}
	rosterAfter, total, _ := s.ListExamRoster(ctx, store.ExamRosterFilter{ExamID: exam.ID, CohortID: c1.ID, PageSize: 10})
	if total != 0 || len(rosterAfter) != 0 {
		t.Fatalf("expected roster empty after undo, got %d", total)
	}

	// 撤销后允许重新导入同一哈希文件
	if _, err := s.ApplyExamImport(ctx, rosterReq); err != nil {
		t.Fatalf("re-import undone roster should succeed: %v", err)
	}
}
