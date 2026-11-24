package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"tally/internal/calc"
	"tally/internal/store"
)

func TestAnalysisQueries(t *testing.T) {
	ctx := context.Background()
	s, path := openCohortStorePath(t)
	dir := filepath.Dir(path)

	defaultPlan, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatalf("EnsureDefaultPlan: %v", err)
	}

	// 1. 创建级与阶段
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatalf("CreateCohort: %v", err)
	}
	stage, err := s.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatalf("GetCohortStage: %v", err)
	}

	// 2. 导入学生名单与任课教师
	roster := store.ImportReport{
		FileHash: "roster", Path: "roster.xlsx",
		StoredPath: filepath.Join(dir, "files", "roster.xlsx"),
		RowCount:   2,
		Sheets:     []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 2}},
	}
	if _, err := s.MergeStageRoster(ctx, cohort.ID, stage.ID, store.ImportData{
		Students: []store.ImportStudent{
			{StudentNo: "001", Name: "张三", ClassName: "1班", Track: "physics", Status: "active", SourceSheet: "学生名单", SourceRow: 2},
			{StudentNo: "002", Name: "李四", ClassName: "1班", Track: "history", Status: "active", SourceSheet: "学生名单", SourceRow: 3},
		},
	}, roster); err != nil {
		t.Fatal(err)
	}

	assignments := store.ImportReport{
		FileHash: "assignments", Path: "assignments.xlsx",
		StoredPath: filepath.Join(dir, "files", "assignments.xlsx"),
		RowCount:   2,
		Sheets:     []store.ImportSheet{{Name: "任课教师", Role: "teachers", HeaderRow: 1, Rows: 2}},
	}
	if _, err := s.MergeStageAssignments(ctx, cohort.ID, stage.ID, store.ImportData{
		Assignments: []store.ImportAssignment{
			{ClassName: "1班", Subject: "chinese", TeacherName: "李老师", SourceSheet: "任课教师", SourceRow: 2},
			{ClassName: "1班", Subject: "math", TeacherName: "王老师", SourceSheet: "任课教师", SourceRow: 3},
		},
	}, assignments); err != nil {
		t.Fatal(err)
	}

	// 3. 创建两场考试
	exam1, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "高一期中考试", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatalf("CreateExam 1: %v", err)
	}

	exam2, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamFinal,
		Name: "高一期末考试", ExamDate: "2027-01-15", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatalf("CreateExam 2: %v", err)
	}

	examCohorts1, err := s.ListExamCohorts(ctx, exam1.ID)
	if err != nil || len(examCohorts1) == 0 {
		t.Fatalf("ListExamCohorts 1: %v", err)
	}
	examCohorts2, err := s.ListExamCohorts(ctx, exam2.ID)
	if err != nil || len(examCohorts2) == 0 {
		t.Fatalf("ListExamCohorts 2: %v", err)
	}

	assess1, err := s.AssessmentForExamCohort(ctx, examCohorts1[0].ID)
	if err != nil {
		t.Fatalf("Assessment 1: %v", err)
	}
	assess2, err := s.AssessmentForExamCohort(ctx, examCohorts2[0].ID)
	if err != nil {
		t.Fatalf("Assessment 2: %v", err)
	}

	// 4. 保存两场考核计算结果
	calcResult1 := calc.Result{
		Rows: []calc.Row{
			{
				StatGroupID:      "1",
				StatGroupName:    "1班语文",
				LeadClassID:      "1",
				LeadClassName:    "1班",
				Subject:          "chinese",
				TeacherNames:     []string{"李老师"},
				ClassAverage:     88.5,
				GradeAverage:     85.0,
				ImprovementScore: 38.0,
				PassScore:        39.0,
				CompletionScore:  19.0,
				Total:            96.0,
				Rank:             1,
				TierIndex:        1,
				Points:           70.0,
			},
		},
		Personal: []calc.PersonalScore{
			{
				TeacherID:   "1",
				TeacherName: "李老师",
				Grade:       "g1",
				Subject:     "chinese",
				Score:       96.0,
			},
		},
	}
	_, err = s.SaveAssessmentRun(ctx, assess1.ID, defaultPlan.ID, "hash1", nil, calcResult1, true)
	if err != nil {
		t.Fatalf("SaveAssessmentRun 1: %v", err)
	}

	calcResult2 := calc.Result{
		Rows: []calc.Row{
			{
				StatGroupID:      "2",
				StatGroupName:    "1班语文",
				LeadClassID:      "1",
				LeadClassName:    "1班",
				Subject:          "chinese",
				TeacherNames:     []string{"李老师"},
				ClassAverage:     90.0,
				GradeAverage:     86.0,
				ImprovementScore: 39.0,
				PassScore:        40.0,
				CompletionScore:  19.5,
				Total:            98.5,
				Rank:             1,
				TierIndex:        1,
				Points:           70.0,
			},
		},
		Personal: []calc.PersonalScore{
			{
				TeacherID:   "1",
				TeacherName: "李老师",
				Grade:       "g1",
				Subject:     "chinese",
				Score:       98.5,
			},
		},
	}
	_, err = s.SaveAssessmentRun(ctx, assess2.ID, defaultPlan.ID, "hash2", nil, calcResult2, true)
	if err != nil {
		t.Fatalf("SaveAssessmentRun 2: %v", err)
	}

	// 5. 测试 QueryTeacherTrends
	teacherTrends, err := s.QueryTeacherTrends(ctx, cohort.ID, "李老师", nil)
	if err != nil {
		t.Fatalf("QueryTeacherTrends: %v", err)
	}
	if len(teacherTrends.Points) != 2 {
		t.Fatalf("expected 2 trend points, got %d", len(teacherTrends.Points))
	}
	if teacherTrends.TeacherName != "李老师" || teacherTrends.Subject != "chinese" {
		t.Fatalf("unexpected teacher trend: %+v", teacherTrends)
	}
	if teacherTrends.Points[0].PersonalScore != 96.0 || teacherTrends.Points[1].PersonalScore != 98.5 {
		t.Fatalf("unexpected scores: %v, %v", teacherTrends.Points[0].PersonalScore, teacherTrends.Points[1].PersonalScore)
	}
	if len(teacherTrends.Points[0].Rows) != 1 || teacherTrends.Points[0].Rows[0].StatGroupName != "1班语文" {
		t.Fatalf("expected teacher trend row parsed from JSON teacher_names, got %+v", teacherTrends.Points[0].Rows)
	}

	// 6. 测试 QueryClassTrends
	classTrends, err := s.QueryClassTrends(ctx, cohort.ID, 1, "1班", nil)
	if err != nil {
		t.Fatalf("QueryClassTrends: %v", err)
	}
	if len(classTrends.Points) != 2 {
		t.Fatalf("expected 2 class trend points, got %d", len(classTrends.Points))
	}
	if classTrends.ClassName != "1班" {
		t.Fatalf("unexpected class trend name: %s", classTrends.ClassName)
	}
	if len(classTrends.Points[0].Rows) != 1 || len(classTrends.Points[0].Rows[0].TeacherNames) != 1 || classTrends.Points[0].Rows[0].TeacherNames[0] != "李老师" {
		t.Fatalf("expected clean decoded TeacherNames in class trend, got %+v", classTrends.Points[0].Rows)
	}

	// 7. 测试 QuerySubjectComparison
	subjComp, err := s.QuerySubjectComparison(ctx, assess1.ID, "chinese")
	if err != nil {
		t.Fatalf("QuerySubjectComparison: %v", err)
	}
	if subjComp.GroupCount != 1 || len(subjComp.Groups) != 1 {
		t.Fatalf("expected 1 group in subject comparison, got %d", subjComp.GroupCount)
	}
	if subjComp.Groups[0].Total != 96.0 {
		t.Fatalf("unexpected total score: %f", subjComp.Groups[0].Total)
	}
	if len(subjComp.Groups[0].TeacherNames) != 1 || subjComp.Groups[0].TeacherNames[0] != "李老师" {
		t.Fatalf("expected clean decoded TeacherNames in subject comparison, got %+v", subjComp.Groups[0].TeacherNames)
	}

	// 8. 测试 QueryAssessmentComparison
	assessComp, err := s.QueryAssessmentComparison(ctx, []int64{assess1.ID, assess2.ID})
	if err != nil {
		t.Fatalf("QueryAssessmentComparison: %v", err)
	}
	if len(assessComp.Items) != 2 {
		t.Fatalf("expected 2 comparison items, got %d", len(assessComp.Items))
	}
	if assessComp.PlanDifferent {
		t.Fatalf("expected plans to be identical, got PlanDifferent = true")
	}
}
