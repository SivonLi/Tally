package exporter_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"tally/internal/exporter"
	"tally/internal/store"
)

func TestExportAnalysis(t *testing.T) {
	dir := t.TempDir()

	// 1. Teacher Trend
	teacherPath := filepath.Join(dir, "teacher_trend.xlsx")
	err := exporter.ExportTeacherTrend(teacherPath, store.TeacherTrendResult{
		TeacherName:          "李老师",
		Subject:              "chinese",
		AveragePersonalScore: 97.25,
		LatestPersonalScore:  98.5,
		Points: []store.TeacherTrendPoint{
			{
				ExamName:      "期中考试",
				ExamDate:      "2026-11-05",
				AcademicYear:  "2026-2027",
				Semester:      "第一学期",
				PlanVersionID: 1,
				PersonalScore: 96.0,
				Rows: []store.TeacherTrendRow{
					{
						StatGroupName:    "501班语文",
						LeadClassName:    "501班",
						Subject:          "chinese",
						ClassAverage:     88.5,
						GradeAverage:     85.0,
						ImprovementScore: 38.0,
						PassRateScore:    39.0,
						CompletionScore:  19.0,
						Total:            96.0,
						Rank:             1,
						Tier:             "第1档",
						Points:           70.0,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExportTeacherTrend: %v", err)
	}
	assertWorkbookValid(t, teacherPath)

	// 2. Class Trend
	classPath := filepath.Join(dir, "class_trend.xlsx")
	err = exporter.ExportClassTrend(classPath, store.ClassTrendResult{
		ClassName:              "501班",
		AverageCompletionScore: 19.5,
		Points: []store.ClassTrendPoint{
			{
				ExamName:        "期中考试",
				ExamDate:        "2026-11-05",
				AcademicYear:    "2026-2027",
				Semester:        "第一学期",
				CompletionScore: 19.5,
				Rows: []store.ClassTrendRow{
					{
						Subject:          "chinese",
						StatGroupName:    "501班语文",
						ClassAverage:     88.5,
						GradeAverage:     85.0,
						ImprovementScore: 38.0,
						PassRateScore:    39.0,
						CompletionScore:  19.5,
						Total:            96.5,
						Rank:             1,
						Tier:             "第1档",
						Points:           70.0,
						TeacherNames:     []string{"李老师"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExportClassTrend: %v", err)
	}
	assertWorkbookValid(t, classPath)

	// 3. Subject Comparison
	subjectPath := filepath.Join(dir, "subject_comp.xlsx")
	err = exporter.ExportSubjectComparison(subjectPath, store.SubjectComparisonResult{
		ExamName:           "期中考试",
		Subject:            "chinese",
		GroupCount:         1,
		AverageScore:       96.5,
		AverageImprovement: 38.0,
		AveragePassRate:    39.0,
		Groups: []store.SubjectComparisonGroup{
			{
				StatGroupName:    "501班语文",
				LeadClassName:    "501班",
				TeacherNames:     []string{"李老师"},
				ClassAverage:     88.5,
				GradeAverage:     85.0,
				ImprovementScore: 38.0,
				PassRateScore:    39.0,
				CompletionScore:  19.5,
				Total:            96.5,
				Rank:             1,
				Tier:             "第1档",
				Points:           70.0,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExportSubjectComparison: %v", err)
	}
	assertWorkbookValid(t, subjectPath)

	// 4. Student Tracking
	studentPath := filepath.Join(dir, "student_track.xlsx")
	err = exporter.ExportStudentTracking(studentPath, store.StudentTrackResult{
		StudentID:   1,
		StudentNo:   "2026001",
		StudentName: "张三",
		ClassName:   "501班",
		Track:       "physics",
		Points: []store.StudentTrackPoint{
			{
				ExamName:      "期中考试",
				ExamDate:      "2026-11-05",
				AcademicYear:  "2026-2027",
				Semester:      "第一学期",
				TotalScore:    610.5,
				GradeRank:     5,
				TotalStudents: 960,
				Scores: map[string]float64{
					"chinese": 115.0,
					"math":    130.0,
					"english": 125.0,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExportStudentTracking: %v", err)
	}
	assertWorkbookValid(t, studentPath)

	// 5. Assessment Comparison
	assessPath := filepath.Join(dir, "assess_comp.xlsx")
	err = exporter.ExportAssessmentComparison(assessPath, store.AssessmentComparisonResult{
		PlanDifferent: false,
		Items: []store.AssessmentComparisonItem{
			{
				ExamName:             "高一期中",
				ExamDate:             "2026-11-05",
				CohortEntryYear:      2026,
				AcademicYear:         "2026-2027",
				Semester:             "第一学期",
				PlanVersionID:        1,
				ParticipantCount:     965,
				GroupCount:           135,
				AverageScore:         91.2,
				AverageClass:         86.4,
				AverageImprovement:   35.6,
				PassTotal:            640,
				OfficialRunCreatedAt: time.Now(),
			},
			{
				ExamName:             "高一期末",
				ExamDate:             "2027-01-15",
				CohortEntryYear:      2026,
				AcademicYear:         "2026-2027",
				Semester:             "第一学期",
				PlanVersionID:        1,
				ParticipantCount:     962,
				GroupCount:           135,
				AverageScore:         92.8,
				AverageClass:         87.1,
				AverageImprovement:   36.2,
				PassTotal:            655,
				OfficialRunCreatedAt: time.Now(),
			},
		},
	})
	if err != nil {
		t.Fatalf("ExportAssessmentComparison: %v", err)
	}
	assertWorkbookValid(t, assessPath)

	// 6. Student 3-Year Profile
	profilePath := filepath.Join(dir, "student_profile.xlsx")
	totalEntry := 632.5
	err = exporter.ExportStudentProfile(profilePath, store.StudentProfile{
		Cohort: store.Cohort{EnrollmentYear: 2026, GraduationYear: 2029},
		Student: store.DataStudent{
			StudentNo:   "2026001",
			Name:        "张三",
			ClassName:   "1班",
			Track:       "physics",
			ForeignLang: "english",
			Status:      "active",
			TotalScore:  &totalEntry,
		},
		EntryScores: []store.StudentEntryScore{
			{Subject: "chinese", Score: 118},
		},
		Exams: []store.StudentTrackPoint{
			{
				ExamName:    "高一期中",
				ExamDate:    "2026-11-05",
				PhaseAtExam: "g1",
				ClassName:   "1班",
				TotalScore:  610.5,
				GradeRank:   5,
				Scores:      map[string]float64{"chinese": 115},
			},
		},
		Events: []store.StudentEvent{
			{
				Phase:         "g1",
				EventType:     "enrolled",
				FromValueJSON: "{}",
				ToValueJSON:   `{"class_name":"1班"}`,
				Note:          "入学建档",
				OccurredAt:    time.Now(),
			},
		},
	})
	if err != nil {
		t.Fatalf("ExportStudentProfile: %v", err)
	}
	assertWorkbookValid(t, profilePath)

	// 验证学生档案导出包含可读中文标签且不直接泄漏原始数据库枚举与 JSON。
	pf, err := excelize.OpenFile(profilePath)
	if err != nil {
		t.Fatalf("excelize.OpenFile(profilePath): %v", err)
	}
	defer pf.Close()
	allCells := make(map[string]bool)
	for _, sheet := range pf.GetSheetList() {
		rows, err := pf.GetRows(sheet)
		if err != nil {
			t.Fatalf("GetRows(%s): %v", sheet, err)
		}
		for _, row := range rows {
			for _, cell := range row {
				allCells[cell] = true
			}
		}
	}
	for _, want := range []string{"物理类", "英语", "在读", "高一", "入学建档"} {
		if !allCells[want] {
			t.Fatalf("student profile export missing human-readable label %q", want)
		}
	}
	for _, forbidden := range []string{"physics", "english", "g1", "enrolled", "{}", `{"class_name":"1班"}`} {
		if allCells[forbidden] {
			t.Fatalf("student profile export leaked raw code/JSON %q", forbidden)
		}
	}
}

func TestExportTeacherProfile(t *testing.T) {
	dir := t.TempDir()
	if err := exporter.ExportTeacherProfile("", store.TeacherProfile{}); err == nil {
		t.Fatal("expected error for empty export path")
	}

	path := filepath.Join(dir, "teacher_profile.xlsx")
	profile := store.TeacherProfile{
		Teacher: store.DataTeacher{
			ID:        7,
			TeacherNo: "T-0007",
			Name:      "李老师",
			Subject:   "chinese",
			Status:    "active",
			Note:      "语文备课组长",
		},
		Occupancy: store.TeacherOccupancy{
			TeacherID:      7,
			TeacherNo:      "T-0007",
			Name:           "李老师",
			PrimarySubject: "chinese",
			Status:         "active_multi",
			ActiveCohorts:  []int64{25, 26},
			ActiveAssignments: []store.TeacherActiveAssignment{
				{
					CohortID: 25, EnrollmentYear: 2025, GraduationYear: 2027, Phase: "g2",
					ClassID: 3, ClassName: "1班", TeachingGroupID: 9, TeachingGroupName: "1班语文",
					Subject: "chinese", Role: "primary", Status: "active", StartedAt: "2026-09-01",
				},
				{
					CohortID: 26, EnrollmentYear: 2026, GraduationYear: 2028, Phase: "g1",
					ClassID: 4, ClassName: "2班", Subject: "chinese", Role: "head_teacher",
					Status: "active", StartedAt: "2026-09-01",
				},
			},
			LastRelease: &store.TeacherLastRelease{
				CohortID: 24, EnrollmentYear: 2024, Phase: "graduated", ClassName: "3班",
				Subject: "chinese", EndReason: "graduated", EndedAt: time.Now(),
			},
		},
		CrossCohortScores: []store.TeacherTrendPoint{
			{
				ExamID: 1, AssessmentID: 101, ExamName: "高二期中", ExamDate: "2026-11-05",
				AcademicYearStart: 2026, Term: "autumn", ExamType: "midterm", GradeLevel: "g2",
				Subject: "chinese", GroupPoints: []float64{96.5, 88}, PlanVersionID: 3, PersonalScore: 96.5,
				Rows: []store.TeacherTrendRow{{
					StatGroupName: "1班语文", LeadClassName: "1班", Subject: "chinese",
					ClassAverage: 108.5, GradeAverage: 102, BaselineAverage: 100, BaselineRatio: 1.085,
					ImprovementScore: 103, PassRateScore: 96, CompletionScore: 92,
					Total: 97.8, Rank: 1, Tier: "第1档", Points: 96.5,
				}},
			},
			{
				ExamID: 2, AssessmentID: 202, ExamName: "高一期末", ExamDate: "2027-01-15",
				AcademicYearStart: 2026, Term: "spring", ExamType: "final", GradeLevel: "g1",
				Subject: "chinese", GroupPoints: []float64{91.2}, PlanVersionID: 3, PersonalScore: 91.2,
				Rows: []store.TeacherTrendRow{{
					StatGroupName: "2班语文", LeadClassName: "2班", Subject: "chinese",
					ClassAverage: 101.5, GradeAverage: 98, BaselineAverage: 96, BaselineRatio: 1.057,
					ImprovementScore: 99, PassRateScore: 90, CompletionScore: 88,
					Total: 92.3, Rank: 2, Tier: "第2档", Points: 91.2,
				}},
			},
		},
		CohortHistories: []store.TeacherCohortHistory{
			{
				CohortID: 25, EnrollmentYear: 2025, GraduationYear: 2027, CurrentPhase: "g2",
				Assignments: []store.TeacherActiveAssignment{
					{
						CohortID: 25, EnrollmentYear: 2025, GraduationYear: 2027, Phase: "g1",
						ClassID: 3, ClassName: "1班", TeachingGroupID: 8, TeachingGroupName: "1班语文",
						Subject: "chinese", Role: "assistant", Status: "ended", EndReason: "replaced",
						StartedAt: "2025-09-01", EndedAt: "2026-03-01",
					},
					{
						CohortID: 25, EnrollmentYear: 2025, GraduationYear: 2027, Phase: "g2",
						ClassID: 3, ClassName: "1班", TeachingGroupID: 9, TeachingGroupName: "1班语文",
						Subject: "chinese", Role: "primary", Status: "active", StartedAt: "2026-03-01",
					},
				},
				Exams: []store.TeacherExamHistory{{
					ExamID: 1, AssessmentID: 101, ExamName: "高二期中", ExamType: "midterm",
					AcademicYearStart: 2026, Term: "autumn", ExamDate: "2026-11-05",
					CohortID: 25, EnrollmentYear: 2025, PhaseAtExam: "g2", Subject: "chinese",
					Score: 96.5, GroupPoints: []float64{96.5},
					ClassRows: []store.StoredResultRow{{
						StatGroupName: "1班语文", LeadClassName: "1班", Subject: "chinese",
						ClassAverage: 108.5, GradeAverage: 102, BaselineAverage: 100, BaselineRatio: 1.085,
						ImprovementScore: 103, PassScore: 96, CompletionScore: 92,
						Total: 97.8, Rank: 1, TierIndex: 0, Points: 96.5,
					}},
				}},
			},
			{
				CohortID: 26, EnrollmentYear: 2026, GraduationYear: 2028, CurrentPhase: "g1",
				Assignments: []store.TeacherActiveAssignment{{
					CohortID: 26, EnrollmentYear: 2026, GraduationYear: 2028, Phase: "g1",
					ClassID: 4, ClassName: "2班", Subject: "chinese", Role: "head_teacher",
					Status: "active", StartedAt: "2026-09-01",
				}},
				Exams: []store.TeacherExamHistory{{
					ExamID: 2, AssessmentID: 202, ExamName: "高一期末", ExamType: "final",
					AcademicYearStart: 2026, Term: "spring", ExamDate: "2027-01-15",
					CohortID: 26, EnrollmentYear: 2026, PhaseAtExam: "g1", Subject: "chinese",
					Score: 91.2, GroupPoints: []float64{91.2},
				}},
			},
		},
	}
	if err := exporter.ExportTeacherProfile(path, profile); err != nil {
		t.Fatalf("ExportTeacherProfile: %v", err)
	}
	assertWorkbookValid(t, path)

	workbook, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("excelize.OpenFile(%s): %v", path, err)
	}
	defer workbook.Close()
	if sheets := workbook.GetSheetList(); len(sheets) != 4 {
		t.Fatalf("teacher profile sheets = %v, want 4 sheets", sheets)
	}
	cells := workbookCells(t, workbook)
	var joined strings.Builder
	for cell := range cells {
		joined.WriteString(cell)
		joined.WriteString("\n")
	}
	for _, want := range []string{
		"在职", "跨届兼任", "主讲", "协助", "班主任", "在任", "已结束", "更换任课",
		"高二", "高一", "已毕业", "毕业封存", "语文", "期中考试", "期末考试",
		"第一学期", "第二学期", "第一档", "第2档",
	} {
		if !strings.Contains(joined.String(), want) {
			t.Fatalf("teacher profile export missing human-readable label %q", want)
		}
	}
	for _, forbidden := range []string{
		"active", "active_multi", "inactive", "primary", "assistant", "head_teacher",
		"ended", "replaced", "graduated", "chinese", "g1", "g2", "midterm", "final",
		"autumn", "spring",
	} {
		if cells[forbidden] {
			t.Fatalf("teacher profile export leaked raw code %q", forbidden)
		}
	}

	// 空档案也必须生成四个工作表的有效工作簿，缺失字段用「-」占位。
	emptyPath := filepath.Join(dir, "teacher_profile_empty.xlsx")
	if err := exporter.ExportTeacherProfile(emptyPath, store.TeacherProfile{}); err != nil {
		t.Fatalf("ExportTeacherProfile(empty): %v", err)
	}
	assertWorkbookValid(t, emptyPath)
	emptyBook, err := excelize.OpenFile(emptyPath)
	if err != nil {
		t.Fatalf("excelize.OpenFile(%s): %v", emptyPath, err)
	}
	defer emptyBook.Close()
	if sheets := emptyBook.GetSheetList(); len(sheets) != 4 {
		t.Fatalf("empty teacher profile sheets = %v, want 4 sheets", sheets)
	}
	if !workbookCells(t, emptyBook)["-"] {
		t.Fatal("empty teacher profile export missing dash placeholder")
	}
}

func workbookCells(t *testing.T, workbook *excelize.File) map[string]bool {
	t.Helper()
	cells := make(map[string]bool)
	for _, sheet := range workbook.GetSheetList() {
		rows, err := workbook.GetRows(sheet)
		if err != nil {
			t.Fatalf("GetRows(%s): %v", sheet, err)
		}
		for _, row := range rows {
			for _, cell := range row {
				cells[cell] = true
			}
		}
	}
	return cells
}

func assertWorkbookValid(t *testing.T, path string) {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("excelize.OpenFile(%s): %v", path, err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		t.Fatalf("file %s has no sheets", path)
	}
}
