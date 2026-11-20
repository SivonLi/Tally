package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"tally/internal/store"
)

type examFixture struct {
	Store    *store.Store
	Path     string
	CohortID int64
	StageID  int64
	ExamID   int64
}

func newExamFixture(t *testing.T) examFixture {
	t.Helper()
	ctx := context.Background()
	s, path := openCohortStorePath(t)
	t.Cleanup(func() { s.Close() })
	if _, err := s.EnsureDefaultPlan(ctx); err != nil {
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
	dir := filepath.Dir(path)
	roster := store.ImportReport{
		FileHash: "roster", Path: "roster.xlsx",
		StoredPath: filepath.Join(dir, "files", "roster.xlsx"),
		RowCount:   2,
		Sheets:     []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 2}},
	}
	if _, err := s.MergeStageRoster(ctx, cohort.ID, stage.ID, store.ImportData{
		Students: []store.ImportStudent{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics", Status: "active", SourceSheet: "学生名单", SourceRow: 2},
			{StudentNo: "002", Name: "乙", ClassName: "1班", Track: "history", Status: "active", SourceSheet: "学生名单", SourceRow: 3},
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
			{ClassName: "1班", Subject: "chinese", TeacherName: "张老师", SourceSheet: "任课教师", SourceRow: 2},
			{ClassName: "1班", Subject: "math", TeacherName: "李老师", SourceSheet: "任课教师", SourceRow: 3},
		},
	}, assignments); err != nil {
		t.Fatal(err)
	}
	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "期中考试", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return examFixture{Store: s, Path: path, CohortID: cohort.ID, StageID: stage.ID, ExamID: exam.ID}
}

func (f examFixture) assessment(t *testing.T) store.Assessment {
	t.Helper()
	examCohort, err := f.Store.GetExamCohort(context.Background(), f.ExamID, f.CohortID)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := f.Store.AssessmentForExamCohort(context.Background(), examCohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	return assessment
}

func TestCreateExamWritesDefaultPlanVersionAndSubjectMarks(t *testing.T) {
	ctx := context.Background()
	s, _ := openCohortStorePath(t)
	t.Cleanup(func() { s.Close() })
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	defaultVersion, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamFinal,
		Name: "期末考试", ExamDate: "2026-12-20", CohortIDs: []int64{cohort.ID},
		SubjectMarks: map[int64]map[string]float64{
			cohort.ID: {"chinese": 160, "math": 120},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	examCohort, err := s.GetExamCohort(ctx, exam.ID, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := s.AssessmentForExamCohort(ctx, examCohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.PlanVersion != defaultVersion.ID {
		t.Fatalf("plan version = %d, want %d", assessment.PlanVersion, defaultVersion.ID)
	}
	subjects, err := s.ListExamSubjects(ctx, exam.ID, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	marks := map[string]float64{}
	for _, subject := range subjects {
		marks[subject.Subject] = subject.FullMark
	}
	if marks["chinese"] != 160 || marks["math"] != 120 {
		t.Fatalf("subject marks = %+v", marks)
	}
}

func TestExamRosterScoresAndQuality(t *testing.T) {
	ctx := context.Background()
	f := newExamFixture(t)
	summary, err := f.Store.GenerateExamRoster(ctx, f.ExamID, f.CohortID, "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Inserted != 2 {
		t.Fatalf("roster summary = %+v", summary)
	}
	tracks, err := f.Store.ListTracks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) == 0 {
		t.Fatal("no tracks")
	}
	assessment := f.assessment(t)
	tier := defaultTierKey(t, f.Store, assessment)
	target := 4.0
	groups, err := f.Store.ListTeachingGroups(ctx, f.StageID, "chinese")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) == 0 {
		t.Fatal("no chinese teaching group")
	}
	classes, err := f.Store.ListClasses(ctx, f.CohortID, f.StageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.SaveStatGroup(ctx, store.StatGroupInput{
		AssessmentID: assessment.ID, Subject: "chinese", Name: "语文合并组",
		LeadClassID: classes[0].ID, Target: &target,
		MemberGroupIDs: []int64{groups[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.Store.SetClassTarget(ctx, assessment.ID, classes[0].ID, tier, 4); err != nil {
		t.Fatal(err)
	}
	report := store.ImportReport{
		FileHash: "scores", Path: "scores.xlsx", RowCount: 2,
		Sheets: []store.ImportSheet{{Name: "成绩", Role: "scores", HeaderRow: 1, Rows: 2}},
	}
	_, err = f.Store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: f.ExamID, CohortID: f.CohortID, Kind: store.ExamImportScores,
		Report: report,
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 90}, Flags: map[string]string{}},
			{StudentNo: "002", Scores: map[string]float64{"chinese": 200}, Flags: map[string]string{}},
		}},
	})
	if !errors.Is(err, store.ErrImportBlocked) {
		t.Fatalf("apply error = %v", err)
	}
	target = 4
	imported, err := f.Store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: f.ExamID, CohortID: f.CohortID, Kind: store.ExamImportScores,
		Report: report,
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 90}, Flags: map[string]string{}},
			{StudentNo: "002", Scores: map[string]float64{"chinese": 88}, Flags: map[string]string{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if imported.Inserted != 2 {
		t.Fatalf("score summary = %+v", imported)
	}
	bands, err := f.Store.ExamScoreBands(ctx, f.ExamID, f.CohortID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bands) == 0 {
		t.Fatal("no score bands")
	}
	averages, err := f.Store.ExamClassAverages(ctx, f.ExamID, f.CohortID)
	if err != nil {
		t.Fatal(err)
	}
	if len(averages) != 1 || averages[0].Count != 2 {
		t.Fatalf("class averages = %+v", averages)
	}
	runs, err := f.Store.ListExamImportRuns(ctx, assessment.ExamCohortID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || len(runs[0].Sheets) != 1 {
		t.Fatalf("runs = %+v", runs)
	}
	detail, err := f.Store.GetImportRun(ctx, runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Run.Sheets) != 1 || detail.Run.Sheets[0].Name != "成绩" {
		t.Fatalf("detail = %+v", detail.Run)
	}
}

func defaultTierKey(t *testing.T, s *store.Store, assessment store.Assessment) string {
	t.Helper()
	keys, err := s.AssessmentTierKeys(context.Background(), assessment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) == 0 {
		t.Fatal("no tier keys")
	}
	return keys[0]
}

func TestStatGroupAndTargetStayInsideAssessment(t *testing.T) {
	ctx := context.Background()
	f := newExamFixture(t)
	assessment := f.assessment(t)
	classes, err := f.Store.ListClasses(ctx, f.CohortID, f.StageID)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := f.Store.ListTeachingGroups(ctx, f.StageID, "math")
	if err != nil {
		t.Fatal(err)
	}
	target := 3.0
	firstID, err := f.Store.SaveStatGroup(ctx, store.StatGroupInput{
		AssessmentID: assessment.ID, Subject: "math", Name: "数学合并组",
		LeadClassID: classes[0].ID, Target: &target,
		MemberGroupIDs: []int64{groups[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	other, otherCohortID, otherStageID := f.assessmentOther(t)
	otherClasses, err := f.Store.ListClasses(ctx, otherCohortID, otherStageID)
	if err != nil {
		t.Fatal(err)
	}
	otherGroups, err := f.Store.ListTeachingGroups(ctx, otherStageID, "math")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.SaveStatGroup(ctx, store.StatGroupInput{
		ID: firstID, AssessmentID: other.ID, Subject: "math", Name: "数学合并组",
		LeadClassID: otherClasses[0].ID, Target: &target,
		MemberGroupIDs: []int64{otherGroups[0].ID},
	}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross assessment save error = %v", err)
	}
	if err := f.Store.DeleteStatGroup(ctx, other.ID, firstID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross assessment delete error = %v", err)
	}
	if _, err := f.Store.SaveStatGroup(ctx, store.StatGroupInput{
		AssessmentID: other.ID, Subject: "math", Name: "数学合并组",
		LeadClassID: otherClasses[0].ID, Target: &target,
		MemberGroupIDs: []int64{otherGroups[0].ID},
	}); err != nil {
		t.Fatalf("independent group in another assessment: %v", err)
	}
}

func TestExamImportRecordsIssuesAndSourceRows(t *testing.T) {
	ctx := context.Background()
	f := newExamFixture(t)
	dir := filepath.Dir(f.Path)
	studentImport := store.ImportReport{
		FileHash: "stage-roster", Path: "students.xlsx",
		StoredPath: filepath.Join(dir, "files", "students.xlsx"),
		RowCount:   1,
		Sheets:     []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 1}},
	}
	if _, err := f.Store.MergeStageRoster(ctx, f.CohortID, f.StageID, store.ImportData{
		Students: []store.ImportStudent{
			{StudentNo: "003", Name: "丙", ClassName: "1班", Status: "active", SourceSheet: "学生名单", SourceRow: 2},
		},
	}, studentImport); err != nil {
		t.Fatal(err)
	}
	first := store.ImportReport{
		FileHash: "roster-a", Path: "roster-a.xlsx",
		StoredPath: filepath.Join(dir, "files", "roster-a.xlsx"),
		RowCount:   2,
		Sheets:     []store.ImportSheet{{Name: "报名名单", Role: "roster", HeaderRow: 1, Rows: 2}},
	}
	request := store.ExamImportRequest{
		ExamID: f.ExamID, CohortID: f.CohortID, Kind: store.ExamImportRoster,
		Report: first,
		Data: store.ImportData{Roster: []store.ImportRoster{
			{StudentNo: "001", Name: "甲", ClassName: "1班", Track: "physics",
				SourceSheet: "报名名单", SourceRow: 2},
			{StudentNo: "003", Name: "丙", ClassName: "1班",
				SourceSheet: "报名名单", SourceRow: 3},
		}},
	}
	if _, err := f.Store.ApplyExamImport(ctx, request); err != nil {
		t.Fatal(err)
	}
	entries, _, err := f.Store.ListExamRoster(ctx, store.ExamRosterFilter{ExamID: f.ExamID, CohortID: f.CohortID})
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]store.ExamRosterEntry{}
	for _, entry := range entries {
		sources[entry.StudentNo] = entry
	}
	if sources["003"].SourceSheet != "报名名单" || sources["003"].SourceRow != 3 {
		t.Fatalf("roster source = %+v", sources["003"])
	}
	runs, err := f.Store.ListExamImportRuns(ctx, f.assessment(t).ExamCohortID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %+v", runs)
	}
	issues, err := f.Store.ListImportIssues(ctx, runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Code != "missing_track" || issues[0].Row != 3 ||
		len(issues[0].Args) != 1 || issues[0].Args[0] != "003" {
		t.Fatalf("issues = %+v", issues)
	}
	if _, err := f.Store.ApplyExamImport(ctx, request); !errors.Is(err, store.ErrDuplicateImport) {
		t.Fatalf("duplicate import error = %v", err)
	}
	second := store.ImportReport{
		FileHash: "roster-b", Path: "roster-b.xlsx",
		StoredPath: filepath.Join(dir, "files", "roster-b.xlsx"),
		RowCount:   2,
		Sheets:     []store.ImportSheet{{Name: "核对名单", Role: "roster", HeaderRow: 1, Rows: 2}},
	}
	request.Report = second
	request.Data.Roster[1].SourceSheet = "核对名单"
	request.Data.Roster[1].SourceRow = 9
	if _, err := f.Store.ApplyExamImport(ctx, request); err != nil {
		t.Fatal(err)
	}
	entries, _, err = f.Store.ListExamRoster(ctx, store.ExamRosterFilter{ExamID: f.ExamID, CohortID: f.CohortID})
	if err != nil {
		t.Fatal(err)
	}
	sources = map[string]store.ExamRosterEntry{}
	for _, entry := range entries {
		sources[entry.StudentNo] = entry
	}
	if sources["003"].SourceSheet != "核对名单" || sources["003"].SourceRow != 9 {
		t.Fatalf("roster source after second import = %+v", sources["003"])
	}
	runs, err = f.Store.ListExamImportRuns(ctx, f.assessment(t).ExamCohortID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs after second import = %+v", runs)
	}
	issues, err = f.Store.ListImportIssues(ctx, runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Row != 9 {
		t.Fatalf("latest issues = %+v", issues)
	}
	scoreReport := store.ImportReport{
		FileHash: "scores-a", Path: "scores-a.xlsx",
		StoredPath: filepath.Join(dir, "files", "scores-a.xlsx"),
		RowCount:   1,
		Sheets:     []store.ImportSheet{{Name: "成绩单", Role: "scores", HeaderRow: 1, Rows: 1}},
	}
	if _, err := f.Store.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: f.ExamID, CohortID: f.CohortID, Kind: store.ExamImportScores,
		Report: scoreReport,
		Data: store.ImportData{Students: []store.ImportStudent{
			{StudentNo: "001", Scores: map[string]float64{"chinese": 105},
				Flags: map[string]string{}, SourceSheet: "成绩单", SourceRow: 7},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	scores, _, err := f.Store.ListExamScores(ctx, store.ExamScoreFilter{ExamID: f.ExamID, CohortID: f.CohortID})
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 1 || scores[0].SourceSheet != "成绩单" || scores[0].SourceRow != 7 {
		t.Fatalf("scores = %+v", scores)
	}
}

func (f examFixture) assessmentOther(t *testing.T) (store.Assessment, int64, int64) {
	t.Helper()
	ctx := context.Background()
	cohort, err := f.Store.CreateCohort(ctx, 2025, 2026)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := f.Store.GetCohortStage(ctx, cohort.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	roster := store.ImportReport{
		FileHash: "other-roster", Path: "other-roster.xlsx",
		StoredPath: filepath.Join("files", "other-roster.xlsx"),
		RowCount:   1,
		Sheets:     []store.ImportSheet{{Name: "学生名单", Role: "students", HeaderRow: 1, Rows: 1}},
	}
	if _, err := f.Store.MergeStageRoster(ctx, cohort.ID, stage.ID, store.ImportData{
		Students: []store.ImportStudent{
			{StudentNo: "101", Name: "丙", ClassName: "1班", Status: "active", SourceSheet: "学生名单", SourceRow: 2},
		},
	}, roster); err != nil {
		t.Fatal(err)
	}
	assignments := store.ImportReport{
		FileHash: "other-assignments", Path: "other-assignments.xlsx",
		StoredPath: filepath.Join("files", "other-assignments.xlsx"),
		RowCount:   1,
		Sheets:     []store.ImportSheet{{Name: "任课教师", Role: "teachers", HeaderRow: 1, Rows: 1}},
	}
	if _, err := f.Store.MergeStageAssignments(ctx, cohort.ID, stage.ID, store.ImportData{
		Assignments: []store.ImportAssignment{
			{ClassName: "1班", Subject: "math", TeacherName: "王老师", SourceSheet: "任课教师", SourceRow: 2},
		},
	}, assignments); err != nil {
		t.Fatal(err)
	}
	exam, err := f.Store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermSecond, ExamType: store.ExamFinal,
		Name: "期末考试", ExamDate: "2027-01-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	examCohort, err := f.Store.GetExamCohort(ctx, exam.ID, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := f.Store.AssessmentForExamCohort(ctx, examCohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	return assessment, cohort.ID, stage.ID
}
