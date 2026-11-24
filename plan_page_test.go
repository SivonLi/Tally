package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/locales"
)

// scrollPlanTo 滚动方案页，直到指定文字出现在窗口内。
func scrollPlanTo(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	for range 20 {
		if box, ok := tt.Find(label); ok &&
			box.X > 0 && box.Y > 0 && box.X < 1280 && box.Y < 800 && box.W > 0 && box.H > 0 {
			return
		}
		tt.Scroll(640, 400, 0, 500)
		tt.Frame()
	}
	t.Fatalf("%q is not visible in the plan page", label)
}

func TestPlanFormulaShapeSelector(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{
		"Class average ÷ grade average ÷ baseline ratio",
		"Students at or above the cutoff ÷ target",
		"Weighted actual ÷ weighted target base",
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	scrollPlanTo(t, tt, "Class average ÷ grade average ÷ baseline ratio")
	if err := tt.Click("Class average ÷ grade average ÷ baseline ratio"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Class average ÷ grade average"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := a.planConfig.Indicators[0].Formula; got != "class.average[subject] / grade.average[subject]" {
		t.Fatalf("formula = %q", got)
	}
}

func TestPlanBoundaryPartsField(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	boundary := &a.planConfig.Ranking.Boundary
	if boundary.Mode != "ratio" {
		t.Fatalf("boundary mode = %q", boundary.Mode)
	}
	if !tt.HasText("Equal parts") || !tt.HasText("integer part of count ÷ 3") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	scrollPlanTo(t, tt, "Equal parts")
	if tt.HasText("The boundary expressions are custom") {
		t.Fatalf("custom echo shown for the default boundary: %q", tt.Texts())
	}
	boundary.First = "ceil(n / 3)"
	tt.Frame()
	if !tt.HasText("The boundary expressions are custom") {
		t.Fatalf("missing custom echo: %q", tt.Texts())
	}
	if !tt.HasText("ceil(n / 3)") {
		t.Fatalf("missing echo of the custom expression: %q", tt.Texts())
	}
}

func TestPlanCompletionOverrideField(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	for _, text := range []string{
		"Blend completion and undergraduate pass",
		"Completion weight",
		"Undergraduate pass weight",
		"Completion × 0.6 + undergraduate pass × 0.4",
	} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	scrollPlanTo(t, tt, "Blend completion and undergraduate pass")
	if err := tt.Click("Blend completion and undergraduate pass"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("No override"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.planConfig.Programs) == 0 {
		t.Fatal("default program missing")
	}
	program := a.planConfig.Programs[0]
	if _, exists := program.IndicatorOverrides["completion"]; exists {
		t.Fatalf("completion override still present: %+v", program.IndicatorOverrides)
	}
}

func TestPlanInlineRowsUseStandardControlHeight(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1280, 1000)
	tt.SetLocale("en")
	settleTester(tt)
	scrollPlanTo(t, tt, locales.PlanTierLabel())

	want := map[string]int{
		locales.PlanTierLabel():       1,
		locales.PlanTierWeightLabel(): 1,
		locales.PlanRemoveRow():       1,
		locales.PlanOverrideSubject(): 1,
		locales.PlanOverrideCount():   1,
		locales.PlanOverrideFirst():   1,
		locales.PlanOverrideSecond():  1,
	}
	seen := map[string]int{}
	for _, info := range tt.AccessAll() {
		switch info.Role {
		case ui.RoleButton, ui.RoleTextField, ui.RoleComboBox, ui.RolePopUpButton:
		default:
			continue
		}
		_, ok := want[info.Name]
		if !ok || info.Bounds.W <= 0 || info.Bounds.H <= 0 {
			continue
		}
		seen[info.Name]++
		if info.Bounds.H != 40 {
			t.Errorf("%s height = %v, want 40", info.Name, info.Bounds.H)
		}
	}
	for name, count := range want {
		if seen[name] < count {
			t.Errorf("%s controls = %d, want at least %d", name, seen[name], count)
		}
	}
}
