package store_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"

	"tally/internal/calc"
	"tally/internal/store"
)

// newArchiveFixture 建立一个已存在的年级、考试与考核记录，返回考试下该级的考核标识。
func newArchiveFixture(t *testing.T, s *store.Store) (store.Cohort, store.ExamCohort, store.Assessment) {
	t.Helper()
	ctx := context.Background()
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatalf("CreateCohort: %v", err)
	}
	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "高一期中", ExamDate: "2026-04-21", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatalf("CreateExam: %v", err)
	}
	ec, err := s.GetExamCohort(ctx, exam.ID, cohort.ID)
	if err != nil {
		t.Fatalf("GetExamCohort: %v", err)
	}
	assess, err := s.AssessmentForExamCohort(ctx, ec.ID)
	if err != nil {
		t.Fatalf("AssessmentForExamCohort: %v", err)
	}
	return cohort, ec, assess
}

func archiveSampleData() store.ArchivedResultData {
	tier0, tier1 := 0, 1
	return store.ArchivedResultData{
		Rows: []store.ImportArchivedResultRow{
			{
				ClassName: "卓越", Subject: "chinese", StatGroupName: "卓越",
				TeacherNames: []string{"赵老师"}, ClassCount: 24,
				ClassAverage: 102.65, GradeAverage: 86.01, BaselineRatio: 1.1614,
				ImprovementScore: 41.10, PassCount: 9, PassScore: 36.66,
				CompletionScore: 12.26, Total: 90.03,
				Rank: 6, TierIndex: &tier0, Points: 70, SourceRow: 2,
			},
			{
				ClassName: "卓越", Subject: "math", StatGroupName: "卓越",
				TeacherNames: []string{"孙老师"}, ClassCount: 24,
				ClassAverage: 110.30, GradeAverage: 90.11, BaselineRatio: 1.2512,
				ImprovementScore: 62.37, PassCount: 10, PassScore: 38.33,
				CompletionScore: 12.26, Total: 112.97,
				Rank: 2, TierIndex: &tier1, Points: 67, SourceRow: 2,
			},
		},
		Personal: []store.ImportArchivedPersonalScore{
			{TeacherName: "赵老师", Subject: "chinese", Score: 70, GroupPoints: []float64{70}, SourceRow: 2},
			{TeacherName: "孙老师", Subject: "math", Score: 70, GroupPoints: []float64{67, 70}, SourceRow: 24},
		},
	}
}

func TestApplyArchivedResultImportWritesRunRowsAndScores(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	cohort, ec, assess := newArchiveFixture(t, s)
	data := archiveSampleData()

	summary, err := s.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess.ID,
		Data:         data,
		Report: store.ImportReport{
			FileHash: "archive-hash-1", Path: "archive.xlsx", RowCount: 4,
			Sheets: []store.ImportSheet{
				{Name: "排名赋分", Role: "archive_results", HeaderRow: 1, Rows: 2},
				{Name: "个人得分", Role: "archive_personal", HeaderRow: 1, Rows: 2},
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyArchivedResultImport: %v", err)
	}
	if summary.ResultRows != 2 || summary.PersonalScores != 2 || summary.Teachers != 2 {
		t.Fatalf("summary = %+v", summary)
	}

	isArchived, err := s.AssessmentArchived(ctx, assess.ID)
	if err != nil {
		t.Fatalf("AssessmentArchived: %v", err)
	}
	if !isArchived {
		t.Fatal("assessment 未标记为归档模式")
	}

	run, rows, personal, err := s.LatestOfficialAssessmentRun(ctx, assess.ID)
	if err != nil {
		t.Fatalf("LatestOfficialAssessmentRun: %v", err)
	}
	if !run.IsOfficial {
		t.Fatal("归档运行应为正式版本")
	}
	if !strings.Contains(run.InputSummary, `"mode":"archived"`) {
		t.Fatalf("输入摘要未标注归档模式: %s", run.InputSummary)
	}
	if run.InputHash != store.ArchivedInputHash(data) {
		t.Fatalf("输入哈希与归档数据内容不一致")
	}
	if len(rows) != 2 || len(personal) != 2 {
		t.Fatalf("rows=%d personal=%d", len(rows), len(personal))
	}
	for _, row := range rows {
		if row.LeadClassName != "卓越" || row.Points == 0 || row.Rank == 0 {
			t.Fatalf("归档行内容不正确: %+v", row)
		}
		if len(row.TeacherIDs) != 1 {
			t.Fatalf("归档行缺少教师关联: %+v", row)
		}
	}

	// 教师档案可以读到归档结果。
	teachers, err := s.ListTeachersWithOccupancy(ctx, "赵老师", "", "")
	if err != nil {
		t.Fatalf("ListTeachersWithOccupancy: %v", err)
	}
	var zhaoID int64
	for _, teacher := range teachers {
		if teacher.Name == "赵老师" {
			zhaoID = teacher.TeacherID
		}
	}
	if zhaoID == 0 {
		t.Fatal("归档导入未建立教师档案")
	}
	profile, err := s.GetTeacherProfile(ctx, zhaoID)
	if err != nil {
		t.Fatalf("GetTeacherProfile: %v", err)
	}
	if len(profile.CrossCohortScores) != 1 {
		t.Fatalf("教师档案跨届得分 = %+v", profile.CrossCohortScores)
	}
	if profile.CrossCohortScores[0].PersonalScore != 70 {
		t.Fatalf("教师档案得分 = %v", profile.CrossCohortScores[0].PersonalScore)
	}
	if len(profile.CohortHistories) == 0 || len(profile.CohortHistories[0].Exams) == 0 {
		t.Fatalf("教师档案带班历史为空: %+v", profile.CohortHistories)
	}
	if profile.CohortHistories[0].CohortID != cohort.ID {
		t.Fatalf("带班历史届别 = %d，期望 %d", profile.CohortHistories[0].CohortID, cohort.ID)
	}
	if len(profile.CohortHistories[0].Exams[0].ClassRows) != 1 {
		t.Fatalf("带班历史班级行 = %+v", profile.CohortHistories[0].Exams[0].ClassRows)
	}

	// 审计日志与导入账本沿用现有模式（2 条结果行 + 2 条个人得分 = 4）。
	runs, err := s.ListExamImportRuns(ctx, ec.ID)
	if err != nil {
		t.Fatalf("ListExamImportRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].DataKind != store.ExamImportArchivedResults {
		t.Fatalf("导入记录 = %+v", runs)
	}
	if runs[0].Inserted != 4 || runs[0].Blocking != 0 {
		t.Fatalf("导入记录计数 = %+v", runs[0])
	}
}

func TestApplyArchivedResultImportRejectsCalculationAndStale(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	_, ec, assess := newArchiveFixture(t, s)
	if _, err := s.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess.ID,
		Data:         archiveSampleData(),
		Report:       store.ImportReport{FileHash: "archive-hash-guard", Path: "archive.xlsx", RowCount: 4},
	}); err != nil {
		t.Fatalf("ApplyArchivedResultImport: %v", err)
	}

	// 试算与正式计算入口都必须拒绝归档考核。
	if err := s.AssertAssessmentCalculable(ctx, assess.ID); err == nil {
		t.Fatal("归档考核不应允许重新计算")
	} else if !strings.Contains(err.Error(), "归档") {
		t.Fatalf("拒绝原因不明确: %v", err)
	}

	// 再次导入归档结果视为新的归档版本，旧记录不再正式。
	if _, err := s.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess.ID,
		Data:         archiveSampleData(),
		Report:       store.ImportReport{FileHash: "archive-hash-guard-2", Path: "archive2.xlsx", RowCount: 4},
	}); err != nil {
		t.Fatalf("second ApplyArchivedResultImport: %v", err)
	}
	runs, err := s.ListAssessmentRuns(ctx, assess.ID)
	if err != nil {
		t.Fatalf("ListAssessmentRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("核算记录数量 = %d", len(runs))
	}
	official := 0
	for _, run := range runs {
		if run.IsOfficial {
			official++
		}
	}
	if official != 1 {
		t.Fatalf("正式记录数量 = %d", official)
	}

	// 导入考试数据引起的陈旧标记必须跳过归档考核。
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: ec.ExamID, CohortID: ec.CohortID, Kind: store.ExamImportCutoffs,
		Data: store.ImportData{
			SubjectCutoffs: []store.ImportSubjectCutoff{{Subject: "chinese", Track: "physics", Value: 98}},
		},
		Report: store.ImportReport{FileHash: "archive-stale-cutoff", Path: "cutoffs.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatalf("ApplyExamImport: %v", err)
	}
	ac, err := s.LoadAssessmentContext(ctx, assess.ID)
	if err != nil {
		t.Fatalf("LoadAssessmentContext: %v", err)
	}
	if ac.Assessment.Status == "stale" {
		t.Fatalf("归档考核被标为 stale: %+v", ac.Assessment)
	}
}

func TestApplyArchivedResultImportIsAtomic(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atomic.db")
	s, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer func() {
		if closeErr := s.Close(); closeErr != nil {
			t.Errorf("close store: %v", closeErr)
		}
	}()
	if _, err := s.EnsureDefaultPlan(ctx); err != nil {
		t.Fatalf("EnsureDefaultPlan: %v", err)
	}
	cohort, ec, assess := newArchiveFixture(t, s)

	// 1. 前置校验阻断时，不应写入任何核算记录或孤儿 import_runs。
	badData := archiveSampleData()
	badData.Personal[0].Subject = "unknown_subject"
	if _, err := s.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess.ID,
		Data:         badData,
		Report:       store.ImportReport{FileHash: "archive-atomic-precheck", Path: "archive.xlsx", RowCount: 4},
	}); err == nil {
		t.Fatal("未知学科应阻断归档导入")
	}
	if runs, err := s.ListExamImportRuns(ctx, ec.ID); err != nil {
		t.Fatalf("ListExamImportRuns: %v", err)
	} else if len(runs) != 0 {
		t.Fatalf("前置校验失败后残留 import_runs: %+v", runs)
	}

	// 2. 通过 ValidateArchivedResultImport 后，在 SQLite 事务执行期触发约束失败，验证真实事务回滚。
	rawDB, err := sqlite.Open(ctx, dbPath, sqlite.OpenOptions{})
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	defer func() {
		if closeErr := rawDB.Close(); closeErr != nil {
			t.Errorf("close rawDB: %v", closeErr)
		}
	}()
	if _, err := rawDB.Execute(ctx, `CREATE TRIGGER fail_archived_insert
		BEFORE INSERT ON archived_assessments
		BEGIN
			SELECT RAISE(ABORT, 'simulated archive tx failure');
		END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	validData := archiveSampleData()
	if _, err := s.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess.ID,
		Data:         validData,
		Report:       store.ImportReport{FileHash: "archive-atomic-tx", Path: "archive.xlsx", RowCount: 4},
	}); err == nil {
		t.Fatal("事务执行期触发约束失败应返回错误")
	}

	runs, err := s.ListAssessmentRuns(ctx, assess.ID)
	if err != nil {
		t.Fatalf("ListAssessmentRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("事务回滚后仍残留 result_runs: %+v", runs)
	}
	isArchived, err := s.AssessmentArchived(ctx, assess.ID)
	if err != nil {
		t.Fatalf("AssessmentArchived: %v", err)
	}
	if isArchived {
		t.Fatal("事务回滚后不应留下归档标记")
	}
	importRuns, err := s.ListExamImportRuns(ctx, ec.ID)
	if err != nil {
		t.Fatalf("ListExamImportRuns: %v", err)
	}
	if len(importRuns) != 0 {
		t.Fatalf("事务回滚后仍残留 import_runs: %+v", importRuns)
	}
	classes, err := s.ListClasses(ctx, cohort.ID, ec.StageID)
	if err != nil {
		t.Fatalf("ListClasses: %v", err)
	}
	if len(classes) != 0 {
		t.Fatalf("事务回滚后仍残留 classes: %+v", classes)
	}
	teachers, err := s.ListTeachersWithOccupancy(ctx, "", "", "")
	if err != nil {
		t.Fatalf("ListTeachersWithOccupancy: %v", err)
	}
	if len(teachers) != 0 {
		t.Fatalf("事务回滚后仍残留 teachers: %+v", teachers)
	}
}

func TestValidateArchivedResultImportReportsIssues(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	_, _, assess := newArchiveFixture(t, s)
	data := archiveSampleData()
	// 工作簿里的班级可能还没有档案，按名称补建后给出提示，不阻断导入。
	data.Rows = append(data.Rows, store.ImportArchivedResultRow{
		ClassName: "新班级", Subject: "chinese", StatGroupName: "新班级",
		TeacherNames: []string{"无名"}, ClassCount: 10, Rank: 1, SourceRow: 9,
	})
	// 同一班级同一学科的同一统计组重复出现时阻断写入。
	data.Rows = append(data.Rows, store.ImportArchivedResultRow{
		ClassName: "卓越", Subject: "chinese", StatGroupName: "卓越", Rank: 3, SourceRow: 10,
	})
	// 个人得分缺少教师姓名时阻断写入。
	data.Personal = append(data.Personal, store.ImportArchivedPersonalScore{
		TeacherName: "  ", Subject: "chinese", Score: 60, SourceRow: 12,
	})
	preview, err := s.ValidateArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess.ID,
		Data:         data,
		Report:       store.ImportReport{FileHash: "archive-validate", Path: "archive.xlsx"},
	})
	if err != nil {
		t.Fatalf("ValidateArchivedResultImport: %v", err)
	}
	if preview.Blocking != 2 {
		t.Fatalf("阻断数量 = %d（问题：%+v）", preview.Blocking, preview.Issues)
	}
	var codes []string
	for _, issue := range preview.Issues {
		codes = append(codes, issue.Code)
	}
	if !containsString(codes, "archive_stat_group_duplicate") || !containsString(codes, "missing_field") {
		t.Fatalf("问题码 = %v", codes)
	}
	if !containsString(codes, "classes_created") {
		t.Fatalf("缺少新建班级提示: %v", codes)
	}
}

func TestArchiveRunIsIndependentOfOfficialCalculation(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	_, ec, assess := newArchiveFixture(t, s)
	plan, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 先有一次常规正式核算，再导入归档结果：归档运行成为唯一正式版本。
	if _, err := s.SaveAssessmentRun(ctx, assess.ID, plan.ID, "regular-hash", nil, calc.Result{
		Rows: []calc.Row{{
			StatGroupID: "1", StatGroupName: "卓越", Subject: "chinese",
			LeadClassName: "卓越", Total: 80, Rank: 1, TierIndex: 0, Points: 70,
		}},
	}, true); err != nil {
		t.Fatalf("SaveAssessmentRun: %v", err)
	}
	if _, err := s.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess.ID,
		Data:         archiveSampleData(),
		Report:       store.ImportReport{FileHash: "archive-after-regular", Path: "archive.xlsx", RowCount: 4},
	}); err != nil {
		t.Fatalf("ApplyArchivedResultImport: %v", err)
	}
	runs, err := s.ListAssessmentRuns(ctx, assess.ID)
	if err != nil {
		t.Fatalf("ListAssessmentRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("核算记录数量 = %d", len(runs))
	}
	if !runs[0].IsOfficial || runs[1].IsOfficial {
		t.Fatalf("正式记录不唯一: %+v", runs)
	}
	if _, err := s.GetExamCohort(ctx, ec.ExamID, ec.CohortID); err != nil {
		t.Fatalf("GetExamCohort: %v", err)
	}
}

func TestUndoArchivedResultImport(t *testing.T) {
	ctx := context.Background()

	// 场景 1：首次归档导入 -> CanUndoImport == true -> UndoImportRun -> 全部归档数据、新建班级与教师、考核与考试状态回滚。
	dbPath1 := filepath.Join(t.TempDir(), "undo1.db")
	s1, err := store.Open(ctx, dbPath1)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer func() {
		if closeErr := s1.Close(); closeErr != nil {
			t.Errorf("close s1: %v", closeErr)
		}
	}()
	if _, err := s1.EnsureDefaultPlan(ctx); err != nil {
		t.Fatalf("EnsureDefaultPlan: %v", err)
	}
	cohort1, ec1, assess1 := newArchiveFixture(t, s1)
	summary1, err := s1.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess1.ID,
		Data:         archiveSampleData(),
		Report:       store.ImportReport{FileHash: "undo-archive-1", Path: "archive.xlsx", RowCount: 4},
	})
	if err != nil {
		t.Fatalf("ApplyArchivedResultImport: %v", err)
	}
	canUndo, reason, err := s1.CanUndoImport(ctx, summary1.ImportRunID)
	if err != nil {
		t.Fatalf("CanUndoImport: %v", err)
	}
	if !canUndo {
		t.Fatalf("expected CanUndoImport=true, got false (%s)", reason)
	}
	if err := s1.UndoImportRun(ctx, summary1.ImportRunID); err != nil {
		t.Fatalf("UndoImportRun: %v", err)
	}
	if archived, err := s1.AssessmentArchived(ctx, assess1.ID); err != nil {
		t.Fatalf("AssessmentArchived: %v", err)
	} else if archived {
		t.Fatal("撤销归档导入后不应仍处于归档态")
	}
	if err := s1.AssertAssessmentCalculable(ctx, assess1.ID); err != nil {
		t.Fatalf("撤销归档后应恢复可计算: %v", err)
	}
	if runs, err := s1.ListAssessmentRuns(ctx, assess1.ID); err != nil {
		t.Fatalf("ListAssessmentRuns: %v", err)
	} else if len(runs) != 0 {
		t.Fatalf("撤销归档后 result_runs 应为空，实际 %+v", runs)
	}
	if classes, err := s1.ListClasses(ctx, cohort1.ID, ec1.StageID); err != nil {
		t.Fatalf("ListClasses: %v", err)
	} else if len(classes) != 0 {
		t.Fatalf("撤销归档后新建班级应被删除，实际 %+v", classes)
	}
	if teachers, err := s1.ListTeachersWithOccupancy(ctx, "", "", ""); err != nil {
		t.Fatalf("ListTeachersWithOccupancy: %v", err)
	} else if len(teachers) != 0 {
		t.Fatalf("撤销归档后新建教师应被删除，实际 %+v", teachers)
	}
	ac1, err := s1.LoadAssessmentContext(ctx, assess1.ID)
	if err != nil {
		t.Fatalf("LoadAssessmentContext: %v", err)
	}
	if ac1.Assessment.Status != "draft" || ac1.Assessment.CurrentRunID != 0 {
		t.Fatalf("撤销归档后考核状态未恢复: %+v", ac1.Assessment)
	}
	if ac1.Exam.Status != "draft" {
		t.Fatalf("撤销归档后考试状态未恢复为 draft，实际 %q", ac1.Exam.Status)
	}
	detail1, err := s1.GetImportRun(ctx, summary1.ImportRunID)
	if err != nil {
		t.Fatalf("GetImportRun: %v", err)
	}
	if detail1.Run.Status != "undone" {
		t.Fatalf("import_run 状态期望 undone，实际 %q", detail1.Run.Status)
	}

	rawDB1, err := sqlite.Open(ctx, dbPath1, sqlite.OpenOptions{})
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	defer func() {
		if closeErr := rawDB1.Close(); closeErr != nil {
			t.Errorf("close rawDB1: %v", closeErr)
		}
	}()
	for _, table := range []string{"result_runs", "result_rows", "result_row_teachers", "personal_scores", "archived_assessments"} {
		cntRows, qErr := rawDB1.Query(ctx, "SELECT COUNT(*) FROM "+table)
		if qErr != nil {
			t.Fatalf("query %s: %v", table, qErr)
		}
		if cnt := cntRows.Values[0][0].(int64); cnt != 0 {
			t.Fatalf("撤销后表 %s 仍残留 %d 条记录", table, cnt)
		}
	}

	// 场景 2：已有正式计算结果后再归档覆盖 -> 撤销归档 -> 恢复原正式计算 is_official=1 与 current_run_id；
	// 以及再次归档覆盖 -> 撤销第二次归档 -> 恢复第一次归档记录。
	s2 := openL1Store(t)
	_, _, assess2 := newArchiveFixture(t, s2)
	plan, err := s2.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatalf("EnsureDefaultPlan: %v", err)
	}
	regularRunID, err := s2.SaveAssessmentRun(ctx, assess2.ID, plan.ID, "regular-official-hash", nil, calc.Result{
		Rows: []calc.Row{{
			StatGroupID: "1", StatGroupName: "卓越", Subject: "chinese",
			LeadClassName: "卓越", Total: 82, Rank: 1, TierIndex: 0, Points: 70,
		}},
	}, true)
	if err != nil {
		t.Fatalf("SaveAssessmentRun: %v", err)
	}

	sumArch1, err := s2.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess2.ID,
		Data:         archiveSampleData(),
		Report:       store.ImportReport{FileHash: "undo-overwrite-1", Path: "archive1.xlsx", RowCount: 4},
	})
	if err != nil {
		t.Fatalf("first ApplyArchivedResultImport: %v", err)
	}
	sumArch2, err := s2.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess2.ID,
		Data:         archiveSampleData(),
		Report:       store.ImportReport{FileHash: "undo-overwrite-2", Path: "archive2.xlsx", RowCount: 4},
	})
	if err != nil {
		t.Fatalf("second ApplyArchivedResultImport: %v", err)
	}

	// 撤销第二次归档覆盖，应恢复第一次归档为正式且保持归档态。
	if err := s2.UndoImportRun(ctx, sumArch2.ImportRunID); err != nil {
		t.Fatalf("undo second archive import: %v", err)
	}
	if archived, err := s2.AssessmentArchived(ctx, assess2.ID); err != nil {
		t.Fatalf("AssessmentArchived after undo 2: %v", err)
	} else if !archived {
		t.Fatal("撤销覆盖归档后应恢复原归档状态")
	}
	latestArch1, _, _, err := s2.LatestOfficialAssessmentRun(ctx, assess2.ID)
	if err != nil {
		t.Fatalf("LatestOfficialAssessmentRun after undo 2: %v", err)
	}
	if latestArch1.ID == regularRunID || !latestArch1.IsOfficial {
		t.Fatalf("撤销第二次归档后应恢复第一次归档记录为正式，实际 %+v", latestArch1)
	}

	// 继续撤销第一次归档，应恢复最初的常规正式计算结果。
	if err := s2.UndoImportRun(ctx, sumArch1.ImportRunID); err != nil {
		t.Fatalf("undo first archive import: %v", err)
	}
	if archived, err := s2.AssessmentArchived(ctx, assess2.ID); err != nil {
		t.Fatalf("AssessmentArchived after undo 1: %v", err)
	} else if archived {
		t.Fatal("撤销全部归档后不应仍处于归档态")
	}
	restoredRun, restoredRows, restoredPersonal, err := s2.LatestOfficialAssessmentRun(ctx, assess2.ID)
	if err != nil {
		t.Fatalf("LatestOfficialAssessmentRun after undo 1: %v", err)
	}
	if restoredRun.ID != regularRunID || !restoredRun.IsOfficial || len(restoredRows) != 1 || len(restoredPersonal) != 0 {
		t.Fatalf("未恢复原常规正式计算结果: run=%+v rows=%d personal=%d", restoredRun, len(restoredRows), len(restoredPersonal))
	}
	ac2, err := s2.LoadAssessmentContext(ctx, assess2.ID)
	if err != nil {
		t.Fatalf("LoadAssessmentContext after undo 1: %v", err)
	}
	if ac2.Assessment.Status != "calculated" || ac2.Assessment.CurrentRunID != regularRunID {
		t.Fatalf("撤销归档后未恢复原考核状态与 current_run_id: %+v", ac2.Assessment)
	}

	// 场景 3：首次归档 -> 撤销 -> 常规正式计算 -> 重导同一文件哈希（复用 runID）-> 再次撤销，
	// 验证 import_changes 旧快照已被清理，不会覆盖回最早的 draft 状态。
	s3 := openL1Store(t)
	_, _, assess3 := newArchiveFixture(t, s3)
	plan3, err := s3.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatalf("EnsureDefaultPlan s3: %v", err)
	}
	firstArch3, err := s3.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess3.ID,
		Data:         archiveSampleData(),
		Report:       store.ImportReport{FileHash: "undo-reimport-same-hash", Path: "same.xlsx", RowCount: 4},
	})
	if err != nil {
		t.Fatalf("s3 first ApplyArchivedResultImport: %v", err)
	}
	if err := s3.UndoImportRun(ctx, firstArch3.ImportRunID); err != nil {
		t.Fatalf("s3 first UndoImportRun: %v", err)
	}
	regRun3, err := s3.SaveAssessmentRun(ctx, assess3.ID, plan3.ID, "reg-after-undo-hash", nil, calc.Result{
		Rows: []calc.Row{{
			StatGroupID: "1", StatGroupName: "卓越", Subject: "chinese",
			LeadClassName: "卓越", Total: 85, Rank: 1, TierIndex: 0, Points: 70,
		}},
	}, true)
	if err != nil {
		t.Fatalf("s3 SaveAssessmentRun: %v", err)
	}
	reArch3, err := s3.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
		AssessmentID: assess3.ID,
		Data:         archiveSampleData(),
		Report:       store.ImportReport{FileHash: "undo-reimport-same-hash", Path: "same.xlsx", RowCount: 4},
	})
	if err != nil {
		t.Fatalf("s3 re-import same hash: %v", err)
	}
	if reArch3.ImportRunID != firstArch3.ImportRunID {
		t.Fatalf("expected reused import_run_id=%d, got %d", firstArch3.ImportRunID, reArch3.ImportRunID)
	}
	if err := s3.UndoImportRun(ctx, reArch3.ImportRunID); err != nil {
		t.Fatalf("s3 second UndoImportRun: %v", err)
	}
	ac3, err := s3.LoadAssessmentContext(ctx, assess3.ID)
	if err != nil {
		t.Fatalf("LoadAssessmentContext s3: %v", err)
	}
	if ac3.Assessment.Status != "calculated" || ac3.Assessment.CurrentRunID != regRun3 {
		t.Fatalf("重导同一文件再撤销后状态被旧快照污染: got status=%q current_run_id=%d, want calculated/%d",
			ac3.Assessment.Status, ac3.Assessment.CurrentRunID, regRun3)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
