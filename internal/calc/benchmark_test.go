package calc

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"tally/internal/assessment"
)

// Test10KCalculationPerformance 验证 10,000 行级（10,000 名学生全科 90,000 条成绩）单次核算用时 ≤ 1 秒。
func Test10KCalculationPerformance(t *testing.T) {
	const classCount = 20
	const studentCount = 10000
	subjects := []string{"chinese", "math", "english", "physics", "chemistry", "biology", "politics", "history", "geography"}

	plan := assessment.DefaultConfig()

	classes := make([]AdminClass, 0, classCount)
	for i := 1; i <= classCount; i++ {
		classes = append(classes, AdminClass{
			ID:    fmt.Sprintf("C%d", i),
			Name:  fmt.Sprintf("高一(%d)班", i),
			Grade: "g1",
		})
	}

	teachingGroups := make([]TeachingGroup, 0, classCount*len(subjects))
	statGroups := make([]StatGroup, 0, classCount*len(subjects))
	for _, s := range subjects {
		for i := 1; i <= classCount; i++ {
			cID := fmt.Sprintf("C%d", i)
			tgID := fmt.Sprintf("TG_%s_%d", s, i)
			sgID := fmt.Sprintf("SG_%s_%d", s, i)
			teachingGroups = append(teachingGroups, TeachingGroup{
				ID:           tgID,
				Name:         fmt.Sprintf("高一(%d)班%s", i, s),
				Subject:      s,
				AdminClassID: cID,
			})
			statGroups = append(statGroups, StatGroup{
				ID:                     sgID,
				Name:                   fmt.Sprintf("高一(%d)班%s", i, s),
				Subject:                s,
				LeadClassID:            cID,
				MemberTeachingGroupIDs: []string{tgID},
				Target:                 25,
				BaselineAverage:        85.0,
			})
		}
	}

	targets := make([]ClassTarget, 0, classCount)
	for i := 1; i <= classCount; i++ {
		cID := fmt.Sprintf("C%d", i)
		counts := make(map[string]*float64, len(plan.Tiers))
		for _, tier := range plan.Tiers {
			val := 0.0
			if tier.Key == "undergraduate" {
				val = 30.0
			}
			counts[tier.Key] = floatPointer(val)
		}
		targets = append(targets, ClassTarget{
			ClassID: cID,
			Counts:  counts,
		})
	}

	r := rand.New(rand.NewSource(42))
	students := make([]Student, 0, studentCount)
	for i := 1; i <= studentCount; i++ {
		cIdx := (i % classCount) + 1
		cID := fmt.Sprintf("C%d", cIdx)
		scores := make(map[string]float64, len(subjects))
		tgMap := make(map[string]string, len(subjects))
		for _, s := range subjects {
			scores[s] = 40.0 + r.Float64()*60.0
			tgMap[s] = fmt.Sprintf("TG_%s_%d", s, cIdx)
		}
		students = append(students, Student{
			ID:             fmt.Sprintf("S%d", i),
			Name:           fmt.Sprintf("学生%d", i),
			AdminClassID:   cID,
			Registered:     true,
			Scores:         scores,
			TeachingGroups: tgMap,
		})
	}

	refs := make(map[string]SubjectReference, len(subjects))
	for _, s := range subjects {
		refs[s] = SubjectReference{
			GradeAverage:         80.0,
			GradeBaselineAverage: 80.0,
			Cutoff:               60.0,
		}
	}

	input := Input{
		Grade:          "g1",
		ExamType:       "midterm",
		Cohort:         "2026",
		Plan:           plan,
		Classes:        classes,
		TeachingGroups: teachingGroups,
		StatGroups:     statGroups,
		ClassTargets:   targets,
		Students:       students,
		References:     refs,
	}

	start := time.Now()
	output, err := Calculate(input, Options{})
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(output.Rows) != len(statGroups) {
		t.Fatalf("expected %d rows, got %d", len(statGroups), len(output.Rows))
	}

	limit := 1 * time.Second
	if raceDetectorActive {
		limit = 5 * time.Second
	}
	t.Logf("10,000 students (%d scores) calculation took %v (limit: %v)", studentCount*len(subjects), duration, limit)
	if duration > limit {
		t.Fatalf("calculation performance exceeded %v limit: took %v", limit, duration)
	}
}

func Benchmark10KCalculation(b *testing.B) {
	const classCount = 20
	const studentCount = 10000
	subjects := []string{"chinese", "math", "english", "physics", "chemistry", "biology", "politics", "history", "geography"}

	plan := assessment.DefaultConfig()
	classes := make([]AdminClass, 0, classCount)
	for i := 1; i <= classCount; i++ {
		classes = append(classes, AdminClass{
			ID:    fmt.Sprintf("C%d", i),
			Name:  fmt.Sprintf("高一(%d)班", i),
			Grade: "g1",
		})
	}

	teachingGroups := make([]TeachingGroup, 0, classCount*len(subjects))
	statGroups := make([]StatGroup, 0, classCount*len(subjects))
	for _, s := range subjects {
		for i := 1; i <= classCount; i++ {
			cID := fmt.Sprintf("C%d", i)
			tgID := fmt.Sprintf("TG_%s_%d", s, i)
			sgID := fmt.Sprintf("SG_%s_%d", s, i)
			teachingGroups = append(teachingGroups, TeachingGroup{
				ID:           tgID,
				Name:         fmt.Sprintf("高一(%d)班%s", i, s),
				Subject:      s,
				AdminClassID: cID,
			})
			statGroups = append(statGroups, StatGroup{
				ID:                     sgID,
				Name:                   fmt.Sprintf("高一(%d)班%s", i, s),
				Subject:                s,
				LeadClassID:            cID,
				MemberTeachingGroupIDs: []string{tgID},
				Target:                 25,
				BaselineAverage:        85.0,
			})
		}
	}

	targets := make([]ClassTarget, 0, classCount)
	for i := 1; i <= classCount; i++ {
		cID := fmt.Sprintf("C%d", i)
		counts := make(map[string]*float64, len(plan.Tiers))
		for _, tier := range plan.Tiers {
			val := 0.0
			if tier.Key == "undergraduate" {
				val = 30.0
			}
			counts[tier.Key] = floatPointer(val)
		}
		targets = append(targets, ClassTarget{
			ClassID: cID,
			Counts:  counts,
		})
	}

	r := rand.New(rand.NewSource(42))
	students := make([]Student, 0, studentCount)
	for i := 1; i <= studentCount; i++ {
		cIdx := (i % classCount) + 1
		cID := fmt.Sprintf("C%d", cIdx)
		scores := make(map[string]float64, len(subjects))
		tgMap := make(map[string]string, len(subjects))
		for _, s := range subjects {
			scores[s] = 40.0 + r.Float64()*60.0
			tgMap[s] = fmt.Sprintf("TG_%s_%d", s, cIdx)
		}
		students = append(students, Student{
			ID:             fmt.Sprintf("S%d", i),
			Name:           fmt.Sprintf("学生%d", i),
			AdminClassID:   cID,
			Registered:     true,
			Scores:         scores,
			TeachingGroups: tgMap,
		})
	}

	refs := make(map[string]SubjectReference, len(subjects))
	for _, s := range subjects {
		refs[s] = SubjectReference{
			GradeAverage:         80.0,
			GradeBaselineAverage: 80.0,
			Cutoff:               60.0,
		}
	}

	input := Input{
		Grade:          "g1",
		ExamType:       "midterm",
		Cohort:         "2026",
		Plan:           plan,
		Classes:        classes,
		TeachingGroups: teachingGroups,
		StatGroups:     statGroups,
		ClassTargets:   targets,
		Students:       students,
		References:     refs,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Calculate(input, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}
