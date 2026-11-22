package exporter_test

import (
	"path/filepath"
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
