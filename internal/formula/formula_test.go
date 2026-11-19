package formula_test

import (
	"math"
	"strings"
	"testing"

	"tally/internal/assessment"
	"tally/internal/formula"
)

func mustCompile(t *testing.T, source string) *formula.Expression {
	t.Helper()
	expr, err := formula.Compile(source, formula.Options{})
	if err != nil {
		t.Fatalf("compile %q: %v", source, err)
	}
	return expr
}

func mustEval(t *testing.T, source string, env formula.Env) float64 {
	t.Helper()
	expr := mustCompile(t, source)
	value, err := expr.Eval(env)
	if err != nil {
		t.Fatalf("eval %q: %v", source, err)
	}
	number, ok := value.Number()
	if !ok {
		t.Fatalf("eval %q = %s", source, value.Format())
	}
	return number
}

func TestDefaultIndicatorFormulasCompile(t *testing.T) {
	for _, indicator := range assessment.DefaultConfig().Indicators {
		if _, err := formula.Compile(indicator.Formula, formula.Options{
			CurrentIndicator: indicator.Key,
		}); err != nil {
			t.Fatalf("%s: %v", indicator.Key, err)
		}
	}
}

func TestImprovementFormulaUsesClassGradeAndBaseline(t *testing.T) {
	source := assessment.DefaultConfig().Indicators[0].Formula
	env := formula.Env{
		"class": formula.Object(map[string]formula.Value{
			"average": formula.Object(map[string]formula.Value{
				"subject": formula.String("chinese"),
				"chinese": formula.Number(95),
			}),
		}),
		"grade": formula.Object(map[string]formula.Value{
			"average": formula.Object(map[string]formula.Value{
				"chinese": formula.Number(88),
			}),
		}),
		"baseline": formula.Object(map[string]formula.Value{
			"ratio": formula.Object(map[string]formula.Value{
				"chinese": formula.Number(1.08),
			}),
		}),
		"subject": formula.String("chinese"),
	}
	got := mustEval(t, source, env)
	want := 95.0 / 88.0 / 1.08
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPassFormulaUsesStatGroup(t *testing.T) {
	source := assessment.DefaultConfig().Indicators[1].Formula
	env := formula.Env{
		"statGroup": formula.Object(map[string]formula.Value{
			"passCount": formula.Number(49),
			"target":    formula.Number(11),
		}),
	}
	if got := mustEval(t, source, env); got != 49.0/11.0 {
		t.Fatalf("got %v", got)
	}
}

func TestGaokaoCompletionFormula(t *testing.T) {
	program := assessment.DefaultConfig().Programs[0]
	source := program.IndicatorOverrides["completion"].Formula
	env := formula.Env{
		"program": formula.Object(map[string]formula.Value{
			"targetCompletion":  formula.Number(0.8),
			"undergraduatePass": formula.Number(0.5),
		}),
	}
	if got := mustEval(t, source, env); math.Abs(got-0.68) > 1e-9 {
		t.Fatalf("got %v", got)
	}
}

func TestConditionalAndFunctions(t *testing.T) {
	source := `if(statGroup.target == 0, 0, round(max(statGroup.passCount / statGroup.target, 1) * 100))`
	env := formula.Env{
		"statGroup": formula.Object(map[string]formula.Value{
			"passCount": formula.Number(3),
			"target":    formula.Number(2),
		}),
	}
	if got := mustEval(t, source, env); got != 150 {
		t.Fatalf("got %v", got)
	}
}

func TestRuntimeDivisionByZeroIsAnError(t *testing.T) {
	expr := mustCompile(t, "statGroup.passCount / statGroup.target")
	_, err := expr.Eval(formula.Env{
		"statGroup": formula.Object(map[string]formula.Value{
			"passCount": formula.Number(1),
			"target":    formula.Number(0),
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "除数不能为零") {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingVariableReportsPosition(t *testing.T) {
	expr := mustCompile(t, "class.average[subject] / grade.average[subject]")
	_, err := expr.Eval(formula.Env{"subject": formula.String("chinese")})
	if err == nil || !strings.Contains(err.Error(), "第 1 行第 1 列") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompileRejectsUnknownVariable(t *testing.T) {
	if _, err := formula.Compile("class.mean", formula.Options{}); err == nil {
		t.Fatal("unknown variable accepted")
	}
}

func TestCompileRejectsUnknownFunction(t *testing.T) {
	if _, err := formula.Compile("sqrt(4)", formula.Options{}); err == nil {
		t.Fatal("unknown function accepted")
	}
}

func TestCompileRejectsLiteralDivisionByZero(t *testing.T) {
	if _, err := formula.Compile("1 / 0", formula.Options{}); err == nil {
		t.Fatal("literal division by zero accepted")
	}
}

func TestCompileRejectsSelfReference(t *testing.T) {
	if _, err := formula.Compile(`indicator["pass"] + 1`, formula.Options{CurrentIndicator: "pass"}); err == nil {
		t.Fatal("self reference accepted")
	}
}

func TestCompileRejectsTypeMismatch(t *testing.T) {
	if _, err := formula.Compile(`"a" + 1`, formula.Options{}); err == nil {
		t.Fatal("type mismatch accepted")
	}
}

func TestCompileRejectsNodeLimit(t *testing.T) {
	source := strings.Repeat("1 + ", 20) + "1"
	if _, err := formula.Compile(source, formula.Options{MaxNodes: 5}); err == nil {
		t.Fatal("node limit ignored")
	}
}

func TestCompileRejectsTrailingInput(t *testing.T) {
	if _, err := formula.Compile("1 2", formula.Options{}); err == nil {
		t.Fatal("trailing input accepted")
	}
}

func TestCompileRejectsSingleEquals(t *testing.T) {
	if _, err := formula.Compile("1 = 1", formula.Options{}); err == nil {
		t.Fatal("single equals accepted")
	}
}
