package assessment

import "testing"

func TestFormulaShapeForDefaultFormulas(t *testing.T) {
	config := DefaultConfig()
	for _, indicator := range config.Indicators {
		got := FormulaShapeFor(indicator.Key, indicator.Formula)
		if got == "" {
			t.Errorf("indicator %s: default formula %q does not match any shape", indicator.Key, indicator.Formula)
		}
	}
}

func TestFormulaShapeForIgnoresWhitespace(t *testing.T) {
	if got := FormulaShapeFor("improvement", "class.average[subject]/grade.average[subject]/baseline.ratio[subject]"); got != "class_grade_baseline" {
		t.Fatalf("got %q", got)
	}
	if got := FormulaShapeFor("pass", "statGroup.passCount / statGroup.target"); got != "pass_target" {
		t.Fatalf("got %q", got)
	}
}

func TestFormulaShapeForCustomFormula(t *testing.T) {
	if got := FormulaShapeFor("improvement", "class.count[subject] / 2"); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := FormulaForShape("pass", "missing"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestFormulaForShapeRoundTrip(t *testing.T) {
	for key, shapes := range map[string][]FormulaShape{
		"improvement": IndicatorFormulaShapes("improvement"),
		"pass":        IndicatorFormulaShapes("pass"),
		"completion":  IndicatorFormulaShapes("completion"),
	} {
		for _, shape := range shapes {
			if got := FormulaShapeFor(key, shape.Formula); got != shape.Code {
				t.Errorf("%s %s: round trip got %q", key, shape.Code, got)
			}
		}
	}
}

func TestBoundaryRatioFormulas(t *testing.T) {
	cases := []struct {
		parts  int
		first  string
		second string
	}{
		{2, "floor(n / 2)", "round(n / 2)"},
		{3, "floor(n / 3)", "round(2 * n / 3)"},
		{4, "floor(n / 4)", "round(3 * n / 4)"},
	}
	for _, item := range cases {
		first, second := BoundaryRatioFormulas(item.parts)
		if first != item.first || second != item.second {
			t.Errorf("parts %d = %q, %q", item.parts, first, second)
		}
		if got := BoundaryRatioParts(first, second); got != item.parts {
			t.Errorf("parts from %q %q = %d", first, second, got)
		}
	}
}

func TestBoundaryRatioPartsMismatch(t *testing.T) {
	if got := BoundaryRatioParts("floor(n / 3)", "round(n / 2)"); got != 0 {
		t.Fatalf("got %d", got)
	}
	if got := BoundaryRatioParts("floor(n/4)", "round(3*n/4)"); got != 4 {
		t.Fatalf("compact form got %d", got)
	}
}

func TestParseCompletionWeightsRoundTrip(t *testing.T) {
	weights, ok := ParseCompletionWeights("program.targetCompletion * 0.6 + program.undergraduatePass * 0.4")
	if !ok {
		t.Fatal("default gaokao override does not parse")
	}
	if weights.TargetCompletion != 0.6 || weights.Undergraduate != 0.4 {
		t.Fatalf("weights = %+v", weights)
	}
	formula := CompletionWeightsFormula(weights.TargetCompletion, weights.Undergraduate)
	if formula != "program.targetCompletion * 0.6 + program.undergraduatePass * 0.4" {
		t.Fatalf("formula = %q", formula)
	}
	again, ok := ParseCompletionWeights(formula)
	if !ok || again != weights {
		t.Fatalf("round trip = %+v %v", again, ok)
	}
}

func TestParseCompletionWeightsMismatch(t *testing.T) {
	for _, formula := range []string{
		"",
		"program.targetCompletion * 0.6",
		"program.targetCompletion * 0.6 + program.undergraduatePass",
		"program.undergraduatePass * 0.4 + program.targetCompletion * 0.6",
		"tier.actual / tier.base",
	} {
		if _, ok := ParseCompletionWeights(formula); ok {
			t.Errorf("%q parsed unexpectedly", formula)
		}
	}
}
