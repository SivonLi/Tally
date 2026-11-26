package pipeline_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"tally/internal/assessment"
	"tally/internal/calc"
	"tally/internal/pipeline"
	"tally/internal/store"
)

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func openRegressionStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "regression.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return s
}

func fullCutoffs(chineseCutoff float64) ([]store.ImportSubjectCutoff, []store.ImportTierCutoff) {
	sub := []store.ImportSubjectCutoff{
		{Subject: "chinese", Track: "physics", Value: chineseCutoff},
		{Subject: "chinese", Track: "history", Value: chineseCutoff},
	}
	tiers := []store.ImportTierCutoff{
		{TierKey: "qingbei", Track: "physics", Value: 650},
		{TierKey: "qingbei", Track: "history", Value: 650},
		{TierKey: "c9", Track: "physics", Value: 600},
		{TierKey: "c9", Track: "history", Value: 600},
		{TierKey: "project985", Track: "physics", Value: 550},
		{TierKey: "project985", Track: "history", Value: 550},
		{TierKey: "project211", Track: "physics", Value: 500},
		{TierKey: "project211", Track: "history", Value: 500},
		{TierKey: "special_control", Track: "physics", Value: 450},
		{TierKey: "special_control", Track: "history", Value: 450},
		{TierKey: "undergraduate", Track: "physics", Value: 350},
		{TierKey: "undergraduate", Track: "history", Value: 350},
	}
	return sub, tiers
}

// TestHistoricalRegressionMidtermOverridesAndTopTwoAverage 覆盖高二/高三期中场景：
// 1. 方案按科边界覆盖（overrides）与并列排名（competition）跨越边界处理；
// 2. 多班教师个人得分 top_two_average（3 班取最高 2 班均值）。
func TestHistoricalRegressionMidtermOverridesAndTopTwoAverage(t *testing.T) {
	ctx := context.Background()
	s := openRegressionStore(t)

	defaultVer, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatalf("EnsureDefaultPlan: %v", err)
	}
	cfg := defaultVer.Config
	// 模拟 6 个统计组：默认 floor(6/3)=2，覆盖语文一档止于 3（类似 17 组中一档覆盖为 6）
	cfg.Ranking.Boundary.Overrides = []assessment.BoundaryOverride{
		{Subject: "chinese", N: 6, First: 3, Second: 4},
	}
	_, customDraft, err := s.CreatePlan(ctx, store.CreatePlanInput{
		Name:   "期中覆盖方案",
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	customVer, err := s.PublishVersion(ctx, customDraft.ID)
	if err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}

	cohort, err := s.CreateCohort(ctx, 2024, 2025)
	if err != nil {
		t.Fatalf("CreateCohort: %v", err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2025)
	if err != nil {
		t.Fatalf("GetCohortStage: %v", err)
	}

	classes := []string{"1班", "2班", "3班", "4班", "5班", "6班"}
	var students []store.ImportStudent
	var roster []store.ImportRoster
	var baselines []store.ImportClassBaseline
	var targets []store.ImportClassTarget

	// 构造 6 个班，每班 2 名学生，其中 1班/2班 分数相同（触发并列），3班 次高，4班/5班/6班 递减。
	// 教师甲教 1班(一档70)、4班(二档67)、6班(三档64) -> top_two_average = (70+67)/2 = 68.5。
	classScores := []struct {
		name  string
		chi   float64
		total float64
		base  float64
	}{
		{"1班", 120, 560, 100},
		{"2班", 120, 560, 100},
		{"3班", 110, 520, 100},
		{"4班", 100, 470, 100},
		{"5班", 90, 400, 100},
		{"6班", 70, 320, 100},
	}
	for idx, cs := range classScores {
		for k := 0; k < 2; k++ {
			sno := string(rune('A'+idx)) + string(rune('1'+k))
			students = append(students, store.ImportStudent{
				StudentNo: sno,
				Name:      "学生" + sno,
				ClassName: cs.name,
				Track:     "physics",
				Status:    "active",
				Scores:    map[string]float64{"chinese": cs.chi},
			})
			roster = append(roster, store.ImportRoster{
				StudentNo:  sno,
				Name:       "学生" + sno,
				ClassName:  cs.name,
				Track:      "physics",
				Status:     "active",
				TotalScore: fp(cs.total),
			})
		}
		baselines = append(baselines, store.ImportClassBaseline{ClassName: cs.name, Subject: "chinese", Average: cs.base})
		targets = append(targets, store.ImportClassTarget{
			ClassName: cs.name,
			Counts: map[string]*float64{
				"qingbei": fp(0), "c9": fp(0), "project985": fp(0),
				"project211": fp(0), "special_control": fp(1), "undergraduate": fp(1),
				"single_subject": fp(2),
			},
		})
	}
	assignments := []store.ImportAssignment{
		{ClassName: "1班", Subject: "chinese", TeacherName: "教师甲"},
		{ClassName: "2班", Subject: "chinese", TeacherName: "教师乙"},
		{ClassName: "3班", Subject: "chinese", TeacherName: "教师丙"},
		{ClassName: "4班", Subject: "chinese", TeacherName: "教师甲"},
		{ClassName: "5班", Subject: "chinese", TeacherName: "教师丁"},
		{ClassName: "6班", Subject: "chinese", TeacherName: "教师甲"},
	}

	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "students", Data: store.ImportData{Classes: classes, Students: students},
		Report: store.ImportReport{FileHash: "reg-mid-stu", Path: "stu.xlsx", RowCount: len(students)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments", Data: store.ImportData{Assignments: assignments},
		Report: store.ImportReport{FileHash: "reg-mid-asg", Path: "asg.xlsx", RowCount: len(assignments)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "baselines", Data: store.ImportData{ClassBaselines: baselines, GradeBaselines: map[string]float64{"chinese": 100}},
		Report: store.ImportReport{FileHash: "reg-mid-bas", Path: "bas.xlsx", RowCount: len(baselines)},
	}); err != nil {
		t.Fatal(err)
	}

	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2025, Term: store.ExamTermSecond, ExamType: store.ExamMidterm,
		Name: "期中合成回归", ExamDate: "2026-04-22", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	subCut, tierCut := fullCutoffs(85)
	for _, req := range []store.ExamImportRequest{
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportRoster, Data: store.ImportData{Roster: roster}, Report: store.ImportReport{FileHash: "reg-mid-r", Path: "r.xlsx", RowCount: len(roster)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportScores, Data: store.ImportData{Students: students}, Report: store.ImportReport{FileHash: "reg-mid-s", Path: "s.xlsx", RowCount: len(students)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportCutoffs, Data: store.ImportData{SubjectCutoffs: subCut, TierCutoffs: tierCut}, Report: store.ImportReport{FileHash: "reg-mid-c", Path: "c.xlsx", RowCount: len(subCut)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportTargets, Data: store.ImportData{ClassTargets: targets}, Report: store.ImportReport{FileHash: "reg-mid-t", Path: "t.xlsx", RowCount: len(targets)}},
	} {
		if _, err := s.ApplyExamImport(ctx, req); err != nil {
			t.Fatalf("ApplyExamImport %s: %v", req.Kind, err)
		}
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

	res, err := pipeline.CalculateAssessment(ctx, s, assess.ID, customVer.ID)
	if err != nil {
		t.Fatalf("CalculateAssessment: %v", err)
	}

	byClass := map[string]calc.Row{}
	for _, row := range res.Rows {
		byClass[row.LeadClassName] = row
	}
	// 1班、2班并列第 1，3班跳至第 3 名且因边界覆盖 First=3 仍属于一档（70分）。
	if byClass["1班"].Rank != 1 || byClass["2班"].Rank != 1 || byClass["3班"].Rank != 3 {
		t.Fatalf("unexpected competition ranks: 1班=%d, 2班=%d, 3班=%d", byClass["1班"].Rank, byClass["2班"].Rank, byClass["3班"].Rank)
	}
	if byClass["3班"].TierIndex != 0 || byClass["3班"].Points != 70 {
		t.Fatalf("expected 3班 in tier 0 with 70 points under override First=3, got tier=%d pts=%v", byClass["3班"].TierIndex, byClass["3班"].Points)
	}
	if byClass["4班"].Rank != 4 || byClass["4班"].TierIndex != 1 || byClass["4班"].Points != 67 {
		t.Fatalf("expected 4班 rank=4 tier=1 pts=67, got %+v", byClass["4班"])
	}
	if byClass["6班"].Rank != 6 || byClass["6班"].TierIndex != 2 || byClass["6班"].Points != 64 {
		t.Fatalf("expected 6班 rank=6 tier=2 pts=64, got %+v", byClass["6班"])
	}

	persByTeacher := map[string]calc.PersonalScore{}
	for _, p := range res.Personal {
		persByTeacher[p.TeacherName] = p
	}
	// 教师甲任教 1班(70)、4班(67)、6班(64)，取最高两班均值 (70+67)/2 = 68.5。
	if !almostEqual(persByTeacher["教师甲"].Score, 68.5, 1e-6) {
		t.Fatalf("expected 教师甲 top_two_average=68.5, got %+v", persByTeacher["教师甲"])
	}
	// 教师乙只任教 2班(70)，得 70。
	if !almostEqual(persByTeacher["教师乙"].Score, 70.0, 1e-6) {
		t.Fatalf("expected 教师乙 score=70.0, got %+v", persByTeacher["教师乙"])
	}
}

// TestHistoricalRegressionFinalWalkingMergeUncappedAndZeroScore 覆盖期末场景：
// 1. 走班合并统计组（政治 1班+2班 合并归 2班，分子相加、分母取统计组目标）与独立成班（历史 3班、4班 各自独立）；
// 2. 不封顶单科达线率 > 40（目标 1 人、达线 3 人 -> 120 分）；
// 3. 单科 0 分按未参考处理（不计入该科均分与人数，总分保留）。
func TestHistoricalRegressionFinalWalkingMergeUncappedAndZeroScore(t *testing.T) {
	ctx := context.Background()
	s := openRegressionStore(t)
	ver, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cohort, err := s.CreateCohort(ctx, 2025, 2025)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2025)
	if err != nil {
		t.Fatal(err)
	}

	classes := []string{"1班", "2班", "3班"}
	students := []store.ImportStudent{
		// 1班：政治走班 1 人达线（80），2班：政治本班 1 人达线（82）、1 人未达线（60）；
		// 1班有 1 人政治 0 分（应按未参考剔除），另有 1 人正常参考。
		{
			StudentNo: "S101", Name: "甲1", ClassName: "1班", Track: "history", Status: "active",
			Scores: map[string]float64{"politics": 80, "history": 75},
			Flags:  map[string]string{},
		},
		{
			StudentNo: "S102", Name: "甲2", ClassName: "1班", Track: "history", Status: "active",
			Scores: map[string]float64{"history": 75},
			Flags:  map[string]string{"politics": "zero"},
		},
		{
			StudentNo: "S201", Name: "乙1", ClassName: "2班", Track: "history", Status: "active",
			Scores: map[string]float64{"politics": 82, "history": 80},
		},
		{
			StudentNo: "S202", Name: "乙2", ClassName: "2班", Track: "history", Status: "active",
			Scores: map[string]float64{"politics": 60, "history": 65},
		},
		// 3班：历史独立成班，目标 1 人，3 人全部达线（达线率 = 3/1*40 = 120 > 40，不封顶）。
		{
			StudentNo: "S301", Name: "丙1", ClassName: "3班", Track: "history", Status: "active",
			Scores: map[string]float64{"history": 90},
		},
		{
			StudentNo: "S302", Name: "丙2", ClassName: "3班", Track: "history", Status: "active",
			Scores: map[string]float64{"history": 85},
		},
		{
			StudentNo: "S303", Name: "丙3", ClassName: "3班", Track: "history", Status: "active",
			Scores: map[string]float64{"history": 80},
		},
	}
	roster := []store.ImportRoster{
		{StudentNo: "S101", Name: "甲1", ClassName: "1班", Track: "history", Status: "active", TotalScore: fp(480)},
		{StudentNo: "S102", Name: "甲2", ClassName: "1班", Track: "history", Status: "active", TotalScore: fp(420)},
		{StudentNo: "S201", Name: "乙1", ClassName: "2班", Track: "history", Status: "active", TotalScore: fp(510)},
		{StudentNo: "S202", Name: "乙2", ClassName: "2班", Track: "history", Status: "active", TotalScore: fp(430)},
		{StudentNo: "S301", Name: "丙1", ClassName: "3班", Track: "history", Status: "active", TotalScore: fp(520)},
		{StudentNo: "S302", Name: "丙2", ClassName: "3班", Track: "history", Status: "active", TotalScore: fp(460)},
		{StudentNo: "S303", Name: "丙3", ClassName: "3班", Track: "history", Status: "active", TotalScore: fp(440)},
	}
	assignments := []store.ImportAssignment{
		{ClassName: "1班", Subject: "politics", TeacherName: "政治师A"},
		{ClassName: "2班", Subject: "politics", TeacherName: "政治师A"},
		{ClassName: "1班", Subject: "history", TeacherName: "历史师A"},
		{ClassName: "2班", Subject: "history", TeacherName: "历史师B"},
		{ClassName: "3班", Subject: "history", TeacherName: "历史师C"},
	}
	baselines := []store.ImportClassBaseline{
		{ClassName: "1班", Subject: "politics", Average: 70},
		{ClassName: "2班", Subject: "politics", Average: 70},
		{ClassName: "1班", Subject: "history", Average: 70},
		{ClassName: "2班", Subject: "history", Average: 70},
		{ClassName: "3班", Subject: "history", Average: 70},
	}
	targets := []store.ImportClassTarget{
		{ClassName: "1班", Counts: map[string]*float64{"qingbei": fp(0), "c9": fp(0), "project985": fp(0), "project211": fp(0), "special_control": fp(1), "undergraduate": fp(1), "single_subject": fp(1)}},
		{ClassName: "2班", Counts: map[string]*float64{"qingbei": fp(0), "c9": fp(0), "project985": fp(0), "project211": fp(0), "special_control": fp(1), "undergraduate": fp(1), "single_subject": fp(1)}},
		{ClassName: "3班", Counts: map[string]*float64{"qingbei": fp(0), "c9": fp(0), "project985": fp(0), "project211": fp(0), "special_control": fp(1), "undergraduate": fp(1), "single_subject": fp(1)}},
	}
	// 政治合并组目标为 4（不同于成员目标之和 1+1=2）。
	statGroups := []store.ImportStatGroup{
		{Name: "1班+2班", Subject: "politics", LeadClassName: "2班", MemberClasses: []string{"1班", "2班"}, Target: 4},
	}

	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "students", Data: store.ImportData{Classes: classes, Students: students},
		Report: store.ImportReport{FileHash: "reg-fin-stu", Path: "stu.xlsx", RowCount: len(students)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments", Data: store.ImportData{Assignments: assignments},
		Report: store.ImportReport{FileHash: "reg-fin-asg", Path: "asg.xlsx", RowCount: len(assignments)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "baselines", Data: store.ImportData{ClassBaselines: baselines, GradeBaselines: map[string]float64{"politics": 70, "history": 70}},
		Report: store.ImportReport{FileHash: "reg-fin-bas", Path: "bas.xlsx", RowCount: len(baselines)},
	}); err != nil {
		t.Fatal(err)
	}

	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2025, Term: store.ExamTermSecond, ExamType: store.ExamFinal,
		Name: "期末走班回归", ExamDate: "2026-07-07", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, tierCut := fullCutoffs(70)
	subCut := []store.ImportSubjectCutoff{
		{Subject: "politics", Track: "history", Value: 75},
		{Subject: "history", Track: "history", Value: 70},
	}
	for _, req := range []store.ExamImportRequest{
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportRoster, Data: store.ImportData{Roster: roster}, Report: store.ImportReport{FileHash: "reg-fin-r", Path: "r.xlsx", RowCount: len(roster)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportScores, Data: store.ImportData{Students: students}, Report: store.ImportReport{FileHash: "reg-fin-s", Path: "s.xlsx", RowCount: len(students)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportCutoffs, Data: store.ImportData{SubjectCutoffs: subCut, TierCutoffs: tierCut}, Report: store.ImportReport{FileHash: "reg-fin-c", Path: "c.xlsx", RowCount: len(subCut)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportTargets, Data: store.ImportData{ClassTargets: targets, StatGroups: statGroups}, Report: store.ImportReport{FileHash: "reg-fin-t", Path: "t.xlsx", RowCount: len(targets)}},
	} {
		if _, err := s.ApplyExamImport(ctx, req); err != nil {
			t.Fatalf("ApplyExamImport %s: %v", req.Kind, err)
		}
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
	existingGroups, err := s.ListExamStatGroups(ctx, assess.ID)
	if err != nil {
		t.Fatal(err)
	}
	var leadClass2ID int64
	var memberTGIDs []int64
	for _, g := range existingGroups {
		if g.Subject == "politics" && (g.LeadClassName == "1班" || g.LeadClassName == "2班") {
			if g.LeadClassName == "2班" && g.LeadClassID > 0 {
				leadClass2ID = g.LeadClassID
			}
			for _, m := range g.Members {
				memberTGIDs = append(memberTGIDs, m.TeachingGroupID)
			}
		}
	}
	if _, err := s.SaveStatGroup(ctx, store.StatGroupInput{
		AssessmentID:    assess.ID,
		Subject:         "politics",
		Name:            "1班+2班",
		LeadClassID:     leadClass2ID,
		MemberGroupIDs:  memberTGIDs,
		Target:          fp(4),
		BaselineAverage: 70,
	}); err != nil {
		t.Fatalf("SaveStatGroup: %v", err)
	}

	res, err := pipeline.CalculateAssessment(ctx, s, assess.ID, ver.ID)
	if err != nil {
		t.Fatalf("CalculateAssessment: %v", err)
	}

	politicsRows := []calc.Row{}
	historyRows := map[string]calc.Row{}
	for _, row := range res.Rows {
		switch row.Subject {
		case "politics":
			politicsRows = append(politicsRows, row)
		case "history":
			historyRows[row.LeadClassName] = row
		}
	}
	// 政治合并组应只产生 1 条结果行，归属 2班；有效人数 3 人（剔除了 0 分生 S102），达线 2 人（80、82），目标 4 -> pass = 2/4*40 = 20。
	if len(politicsRows) != 1 {
		t.Fatalf("expected 1 merged politics row, got %d", len(politicsRows))
	}
	pol := politicsRows[0]
	if pol.LeadClassName != "2班" || pol.ClassCount != 3 || pol.PassCount != 2 || !almostEqual(pol.PassTarget, 4, 1e-6) || !almostEqual(pol.PassScore, 20, 1e-6) {
		t.Fatalf("unexpected merged politics row: %+v", pol)
	}
	expectedAvg := (80.0 + 82.0 + 60.0) / 3.0
	if !almostEqual(pol.ClassAverage, expectedAvg, 1e-6) {
		t.Fatalf("expected politics avg %.4f (zero score excluded), got %.4f", expectedAvg, pol.ClassAverage)
	}

	// 历史应有 3 条独立行；3班 3 人达线 / 目标 1 人 * 40 = 120 分（不封顶）。
	if len(historyRows) != 3 {
		t.Fatalf("expected 3 independent history rows, got %d", len(historyRows))
	}
	if !almostEqual(historyRows["3班"].PassScore, 120.0, 1e-6) {
		t.Fatalf("expected uncapped 3班 history PassScore=120, got %.4f", historyRows["3班"].PassScore)
	}
}

// TestHistoricalRegressionGaokaoCompletionAndExplicitActuals 覆盖高考专项场景：
// 1. 显式 ActualCounts 免总分分数线；
// 2. 60% 目标任务完成率 + 40% 本科达线完成率公式验证。
func TestHistoricalRegressionGaokaoCompletionAndExplicitActuals(t *testing.T) {
	ctx := context.Background()
	s := openRegressionStore(t)
	ver, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cohort, err := s.CreateCohort(ctx, 2023, 2025)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2025)
	if err != nil {
		t.Fatal(err)
	}

	classes := []string{"1班", "2班"}
	students := []store.ImportStudent{
		{StudentNo: "G101", Name: "高1", ClassName: "1班", Track: "physics", Status: "active", Scores: map[string]float64{"chinese": 115}},
		{StudentNo: "G201", Name: "高2", ClassName: "2班", Track: "physics", Status: "active", Scores: map[string]float64{"chinese": 105}},
	}
	roster := []store.ImportRoster{
		{StudentNo: "G101", Name: "高1", ClassName: "1班", Track: "physics", Status: "active", TotalScore: fp(600)},
		{StudentNo: "G201", Name: "高2", ClassName: "2班", Track: "physics", Status: "active", TotalScore: fp(550)},
	}
	assignments := []store.ImportAssignment{
		{ClassName: "1班", Subject: "chinese", TeacherName: "语文师1"},
		{ClassName: "2班", Subject: "chinese", TeacherName: "语文师2"},
	}
	baselines := []store.ImportClassBaseline{
		{ClassName: "1班", Subject: "chinese", Average: 100},
		{ClassName: "2班", Subject: "chinese", Average: 100},
	}
	// 1班目标：985=2, 211=2, 本科=6 -> 目标分=2*30+2*20+6*8=148，总目标人数=10。
	// 1班实际完成：985=1, 211=3, 本科=8 -> 实际分=1*30+3*20+8*8=154，总完成人数=12。
	// 高考完成率得分 = (154/148*0.6 + 12/10*0.4) * 20 = (0.624324324 + 0.48) * 20 = 22.086486486...
	targets := []store.ImportClassTarget{
		{
			ClassName: "1班",
			Counts: map[string]*float64{
				"qingbei": fp(0), "c9": fp(0), "project985": fp(2),
				"project211": fp(2), "special_control": fp(0), "undergraduate": fp(6),
				"single_subject": fp(10),
			},
			ActualCounts: map[string]*float64{
				"qingbei": fp(0), "c9": fp(0), "project985": fp(1),
				"project211": fp(3), "special_control": fp(0), "undergraduate": fp(8),
			},
		},
		{
			ClassName: "2班",
			Counts: map[string]*float64{
				"qingbei": fp(0), "c9": fp(0), "project985": fp(0),
				"project211": fp(0), "special_control": fp(0), "undergraduate": fp(10),
				"single_subject": fp(10),
			},
			ActualCounts: map[string]*float64{
				"qingbei": fp(0), "c9": fp(0), "project985": fp(0),
				"project211": fp(0), "special_control": fp(0), "undergraduate": fp(5),
			},
		},
	}

	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "students", Data: store.ImportData{Classes: classes, Students: students},
		Report: store.ImportReport{FileHash: "reg-gk-stu", Path: "stu.xlsx", RowCount: len(students)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "assignments", Data: store.ImportData{Assignments: assignments},
		Report: store.ImportReport{FileHash: "reg-gk-asg", Path: "asg.xlsx", RowCount: len(assignments)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, cohort.ID, stage.ID, store.StageImportRequest{
		Kind: "baselines", Data: store.ImportData{ClassBaselines: baselines, GradeBaselines: map[string]float64{"chinese": 100}},
		Report: store.ImportReport{FileHash: "reg-gk-bas", Path: "bas.xlsx", RowCount: len(baselines)},
	}); err != nil {
		t.Fatal(err)
	}

	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2025, Term: store.ExamTermSecond, ExamType: store.ExamGaokao,
		Name: "高考专项合成回归", ExamDate: "2026-06-25", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 高考不传总分分数线（TierCutoffs 为空），仅传单科分数线。
	subCut := []store.ImportSubjectCutoff{{Subject: "chinese", Track: "physics", Value: 100}}
	for _, req := range []store.ExamImportRequest{
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportRoster, Data: store.ImportData{Roster: roster}, Report: store.ImportReport{FileHash: "reg-gk-r", Path: "r.xlsx", RowCount: len(roster)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportScores, Data: store.ImportData{Students: students}, Report: store.ImportReport{FileHash: "reg-gk-s", Path: "s.xlsx", RowCount: len(students)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportCutoffs, Data: store.ImportData{SubjectCutoffs: subCut}, Report: store.ImportReport{FileHash: "reg-gk-c", Path: "c.xlsx", RowCount: len(subCut)}},
		{ExamID: exam.ID, CohortID: cohort.ID, Kind: store.ExamImportTargets, Data: store.ImportData{ClassTargets: targets}, Report: store.ImportReport{FileHash: "reg-gk-t", Path: "t.xlsx", RowCount: len(targets)}},
	} {
		if _, err := s.ApplyExamImport(ctx, req); err != nil {
			t.Fatalf("ApplyExamImport %s: %v", req.Kind, err)
		}
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

	res, err := pipeline.CalculateAssessment(ctx, s, assess.ID, ver.ID)
	if err != nil {
		t.Fatalf("CalculateAssessment: %v", err)
	}
	byClass := map[string]calc.Row{}
	for _, r := range res.Rows {
		byClass[r.LeadClassName] = r
	}
	expectedC1 := (154.0/148.0*0.6 + 12.0/10.0*0.4) * 20.0
	if !almostEqual(byClass["1班"].CompletionScore, expectedC1, 1e-6) {
		t.Fatalf("expected 1班 gaokao completion=%.6f, got %.6f", expectedC1, byClass["1班"].CompletionScore)
	}
	// 2班：目标 10*8=80，实际 5*8=40 -> (40/80*0.6 + 5/10*0.4)*20 = 10.0
	if !almostEqual(byClass["2班"].CompletionScore, 10.0, 1e-6) {
		t.Fatalf("expected 2班 gaokao completion=10.0, got %.6f", byClass["2班"].CompletionScore)
	}
}
