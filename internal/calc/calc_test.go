package calc

import (
	"math"
	"strings"
	"testing"

	"tally/internal/assessment"
)

func floatPointer(value float64) *float64 {
	return &value
}

func baseInput() Input {
	target := func(undergraduate float64) ClassTarget {
		return ClassTarget{
			ClassID: "C1",
			Counts: map[string]*float64{
				"qingbei":         floatPointer(0),
				"c9":              floatPointer(0),
				"project985":      floatPointer(0),
				"project211":      floatPointer(0),
				"special_control": floatPointer(0),
				"undergraduate":   floatPointer(undergraduate),
			},
		}
	}
	targets := []ClassTarget{target(10), target(10), target(10)}
	targets[0].ClassID = "C1"
	targets[1].ClassID = "C2"
	targets[2].ClassID = "C3"
	return Input{
		Grade:    "g1",
		ExamType: "midterm",
		Cohort:   "2026",
		Plan:     assessment.DefaultConfig(),
		Classes: []AdminClass{
			{ID: "C1", Name: "一班", Grade: "g1"},
			{ID: "C2", Name: "二班", Grade: "g1"},
			{ID: "C3", Name: "三班", Grade: "g1"},
		},
		TeachingGroups: []TeachingGroup{
			{ID: "T1", Name: "一班语文", Subject: "chinese", AdminClassID: "C1"},
			{ID: "T2", Name: "二班语文", Subject: "chinese", AdminClassID: "C2"},
			{ID: "T3", Name: "三班语文", Subject: "chinese", AdminClassID: "C3"},
		},
		StatGroups: []StatGroup{
			{ID: "SG1", Name: "一班语文", Subject: "chinese", LeadClassID: "C1", MemberTeachingGroupIDs: []string{"T1"}, Target: 10, BaselineAverage: 90},
			{ID: "SG2", Name: "二班语文", Subject: "chinese", LeadClassID: "C2", MemberTeachingGroupIDs: []string{"T2"}, Target: 10, BaselineAverage: 90},
			{ID: "SG3", Name: "三班语文", Subject: "chinese", LeadClassID: "C3", MemberTeachingGroupIDs: []string{"T3"}, Target: 10, BaselineAverage: 90},
		},
		Students: []Student{
			{ID: "S1", Name: "甲", AdminClassID: "C1", Registered: true, Scores: map[string]float64{"chinese": 100}, TeachingGroups: map[string]string{"chinese": "T1"}},
			{ID: "S2", Name: "乙", AdminClassID: "C1", Registered: true, Scores: map[string]float64{"chinese": 100}, TeachingGroups: map[string]string{"chinese": "T1"}},
			{ID: "S3", Name: "丙", AdminClassID: "C2", Registered: true, Scores: map[string]float64{"chinese": 90}, TeachingGroups: map[string]string{"chinese": "T2"}},
			{ID: "S4", Name: "丁", AdminClassID: "C2", Registered: true, Scores: map[string]float64{"chinese": 90}, TeachingGroups: map[string]string{"chinese": "T2"}},
			{ID: "S5", Name: "戊", AdminClassID: "C3", Registered: true, Scores: map[string]float64{"chinese": 80}, TeachingGroups: map[string]string{"chinese": "T3"}},
			{ID: "S6", Name: "己", AdminClassID: "C3", Registered: true, Scores: map[string]float64{"chinese": 80}, TeachingGroups: map[string]string{"chinese": "T3"}},
		},
		ClassTargets: targets,
		ActualTierCounts: map[string]map[string]float64{
			"C1": {"undergraduate": 10},
			"C2": {"undergraduate": 10},
			"C3": {"undergraduate": 10},
		},
		Assignments: []Assignment{
			{TeacherID: "TCH", TeacherName: "教师甲", TeachingGroupID: "T1"},
			{TeacherID: "TCH", TeacherName: "教师甲", TeachingGroupID: "T2"},
			{TeacherID: "TCH", TeacherName: "教师甲", TeachingGroupID: "T3"},
		},
		References: map[string]SubjectReference{
			"chinese": {GradeAverage: 90, GradeBaselineAverage: 90, Cutoff: 60},
		},
	}
}

func rowByStatGroup(t *testing.T, result Result, id string) Row {
	t.Helper()
	for _, row := range result.Rows {
		if row.StatGroupID == id {
			return row
		}
	}
	t.Fatalf("stat group %s not found", id)
	return Row{}
}

func TestCalculateThreeIndicatorsAndRanking(t *testing.T) {
	result, err := Calculate(baseInput(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	first := rowByStatGroup(t, result, "SG1")
	if math.Abs(first.ImprovementScore-100.0/90.0*40) > 1e-9 {
		t.Fatalf("improvement = %v", first.ImprovementScore)
	}
	if math.Abs(first.PassScore-8) > 1e-9 {
		t.Fatalf("pass = %v", first.PassScore)
	}
	if math.Abs(first.CompletionScore-20) > 1e-9 {
		t.Fatalf("completion = %v", first.CompletionScore)
	}
	if first.Rank != 1 || first.TierIndex != 0 || first.Points != 70 {
		t.Fatalf("first rank = %d tier = %d points = %v", first.Rank, first.TierIndex, first.Points)
	}
	second := rowByStatGroup(t, result, "SG2")
	if second.Rank != 2 || second.TierIndex != 1 || second.Points != 67 {
		t.Fatalf("second rank = %d tier = %d points = %v", second.Rank, second.TierIndex, second.Points)
	}
	third := rowByStatGroup(t, result, "SG3")
	if third.Rank != 3 || third.TierIndex != 2 || third.Points != 64 {
		t.Fatalf("third rank = %d tier = %d points = %v", third.Rank, third.TierIndex, third.Points)
	}
}

func TestPersonalScoreUsesTopTwoGroups(t *testing.T) {
	result, err := Calculate(baseInput(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Personal) != 1 {
		t.Fatalf("personal = %+v", result.Personal)
	}
	if result.Personal[0].Score != 68.5 {
		t.Fatalf("score = %v", result.Personal[0].Score)
	}
}

func TestCompetitionAndDenseTieModes(t *testing.T) {
	input := baseInput()
	input.Students[2].Scores["chinese"] = 100
	input.Students[3].Scores["chinese"] = 100

	result, err := Calculate(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows[0].Rank != 1 || result.Rows[1].Rank != 1 {
		t.Fatalf("competition ranks = %d %d", result.Rows[0].Rank, result.Rows[1].Rank)
	}
	if result.Rows[2].Rank != 3 || result.Rows[2].Points != 64 {
		t.Fatalf("competition third = %d %v", result.Rows[2].Rank, result.Rows[2].Points)
	}

	input.Plan.Ranking.TieMode = "dense"
	result, err = Calculate(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows[2].Rank != 2 || result.Rows[2].Points != 67 {
		t.Fatalf("dense third = %d %v", result.Rows[2].Rank, result.Rows[2].Points)
	}
}

func TestBoundaryRatioAndOverrides(t *testing.T) {
	config := assessment.DefaultConfig().Ranking.Boundary
	first, second, err := rankBoundaries(config, "chinese", 17)
	if err != nil {
		t.Fatal(err)
	}
	if first != 6 || second != 11 {
		t.Fatalf("chinese n=17 = %d %d", first, second)
	}
	first, second, err = rankBoundaries(config, "chemistry", 18)
	if err != nil {
		t.Fatal(err)
	}
	if first != 6 || second != 12 {
		t.Fatalf("chemistry n=18 = %d %d", first, second)
	}
	first, second, err = rankBoundaries(config, "math", 2)
	if err != nil {
		t.Fatal(err)
	}
	if first != 2 || second != 2 {
		t.Fatalf("math n=2 = %d %d", first, second)
	}
}

func TestManualBoundary(t *testing.T) {
	config := assessment.BoundaryConfig{
		Mode: "manual",
		Manual: []assessment.ManualBoundary{
			{Subject: "chinese", N: 10, First: 3, Second: 7},
		},
	}
	first, second, err := rankBoundaries(config, "chinese", 10)
	if err != nil {
		t.Fatal(err)
	}
	if first != 3 || second != 7 {
		t.Fatalf("manual = %d %d", first, second)
	}
}

func TestZeroTargetBlocksAndAllowsExclusion(t *testing.T) {
	input := baseInput()
	input.ClassTargets = input.ClassTargets[:2]
	_, err := Calculate(input, Options{})
	if err == nil {
		t.Fatal("zero target accepted")
	}
	zero, ok := err.(*ZeroTargetError)
	if !ok || len(zero.Issues) != 1 || zero.Issues[0].ClassID != "C3" {
		t.Fatalf("err = %#v", err)
	}

	result, err := Calculate(input, Options{
		ExcludedCompletionClasses: map[string]bool{"C3": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	excluded := rowByStatGroup(t, result, "SG3")
	if excluded.CompletionParticipating {
		t.Fatal("excluded class still participates")
	}
	if excluded.CompletionScore != 0 {
		t.Fatalf("completion score = %v", excluded.CompletionScore)
	}
	if excluded.Total != excluded.ImprovementScore+excluded.PassScore {
		t.Fatalf("total = %v", excluded.Total)
	}
}

func TestMergedStatGroupSumsPassCount(t *testing.T) {
	input := Input{
		Grade:    "g1",
		ExamType: "midterm",
		Plan:     assessment.DefaultConfig(),
		Classes: []AdminClass{
			{ID: "C1", Name: "一班", Grade: "g1"},
			{ID: "C2", Name: "二班", Grade: "g1"},
		},
		TeachingGroups: []TeachingGroup{
			{ID: "T1", Name: "一班语文", Subject: "chinese", AdminClassID: "C1"},
			{ID: "T2", Name: "二班语文", Subject: "chinese", AdminClassID: "C2"},
		},
		StatGroups: []StatGroup{
			{ID: "SG", Name: "语文合并组", Subject: "chinese", LeadClassID: "C1", MemberTeachingGroupIDs: []string{"T1", "T2"}, Target: 10, BaselineAverage: 90},
		},
		Students: []Student{
			{ID: "S1", AdminClassID: "C1", Registered: true, Scores: map[string]float64{"chinese": 100}, TeachingGroups: map[string]string{"chinese": "T1"}},
			{ID: "S2", AdminClassID: "C2", Registered: true, Scores: map[string]float64{"chinese": 80}, TeachingGroups: map[string]string{"chinese": "T2"}},
		},
		ClassTargets: []ClassTarget{{
			ClassID: "C1",
			Counts: map[string]*float64{
				"qingbei":         floatPointer(0),
				"c9":              floatPointer(0),
				"project985":      floatPointer(0),
				"project211":      floatPointer(0),
				"special_control": floatPointer(0),
				"undergraduate":   floatPointer(10),
			},
		}},
		ActualTierCounts: map[string]map[string]float64{"C1": {"undergraduate": 10}},
		Assignments: []Assignment{
			{TeacherID: "A", TeacherName: "教师甲", TeachingGroupID: "T1"},
			{TeacherID: "B", TeacherName: "教师乙", TeachingGroupID: "T2"},
		},
		References: map[string]SubjectReference{
			"chinese": {GradeAverage: 90, GradeBaselineAverage: 90, Cutoff: 60},
		},
	}
	result, err := Calculate(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d", len(result.Rows))
	}
	row := result.Rows[0]
	if row.LeadClassID != "C1" || row.PassCount != 2 || row.PassTarget != 10 {
		t.Fatalf("row = %+v", row)
	}
	if row.MemberCount != 2 {
		t.Fatalf("member count = %d", row.MemberCount)
	}
	if len(result.Personal) != 2 {
		t.Fatalf("personal = %+v", result.Personal)
	}
}

func TestGaokaoCompletionProgram(t *testing.T) {
	input := baseInput()
	input.ExamType = "gaokao"
	for i := range input.ClassTargets {
		input.ClassTargets[i].Counts["qingbei"] = floatPointer(10)
	}
	for i := range input.ActualTierCounts {
		input.ActualTierCounts[i]["qingbei"] = 5
	}
	result, err := Calculate(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	row := rowByStatGroup(t, result, "SG1")
	targetCompletion := float64(5*50+10*8) / float64(10*50+10*8)
	undergraduate := 10.0 / 10.0
	want := (targetCompletion*0.6 + undergraduate*0.4) * 20
	if math.Abs(row.CompletionScore-want) > 1e-9 {
		t.Fatalf("completion = %v want %v", row.CompletionScore, want)
	}
}

func TestCapAppliesAfterPoints(t *testing.T) {
	input := baseInput()
	max := 41.0
	input.Plan.Indicators[0].Cap = &assessment.CapConfig{Max: &max}
	result, err := Calculate(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	row := rowByStatGroup(t, result, "SG1")
	if row.ImprovementScore != 41 {
		t.Fatalf("improvement = %v", row.ImprovementScore)
	}
}

func TestIndicatorDependencyOrder(t *testing.T) {
	input := baseInput()
	for i := range input.Plan.Indicators {
		switch input.Plan.Indicators[i].Key {
		case "improvement":
			input.Plan.Indicators[i].Formula = "1"
		case "completion":
			input.Plan.Indicators[i].Formula = `indicator["improvement"] / 40`
		}
	}
	result, err := Calculate(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	row := rowByStatGroup(t, result, "SG1")
	if row.CompletionScore != 20 {
		t.Fatalf("completion = %v", row.CompletionScore)
	}
}

func TestIndicatorCycleIsRejected(t *testing.T) {
	input := baseInput()
	for i := range input.Plan.Indicators {
		switch input.Plan.Indicators[i].Key {
		case "improvement":
			input.Plan.Indicators[i].Formula = `indicator["pass"]`
		case "pass":
			input.Plan.Indicators[i].Formula = `indicator["improvement"]`
		}
	}
	_, err := Calculate(input, Options{})
	if err == nil || !strings.Contains(err.Error(), "循环引用") {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidStudentsAreExcluded(t *testing.T) {
	input := baseInput()
	input.Students = append(input.Students,
		Student{ID: "S7", AdminClassID: "C3", Registered: true, Flags: []string{"absent"}, Scores: map[string]float64{"chinese": 100}, TeachingGroups: map[string]string{"chinese": "T3"}},
		Student{ID: "S8", AdminClassID: "C3", Registered: false, Scores: map[string]float64{"chinese": 100}, TeachingGroups: map[string]string{"chinese": "T3"}},
		Student{ID: "S9", AdminClassID: "C3", Registered: true, Status: "transferred", Scores: map[string]float64{"chinese": 100}, TeachingGroups: map[string]string{"chinese": "T3"}},
	)
	result, err := Calculate(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	row := rowByStatGroup(t, result, "SG3")
	if row.ClassCount != 2 {
		t.Fatalf("count = %d", row.ClassCount)
	}
}
