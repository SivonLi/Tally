package assessment

import (
	"fmt"
	"strconv"
	"strings"
)

// FormulaShape 是指标公式的一个可选形状：
// 界面上按形状选择，公式文本由形状生成后写回配置。
type FormulaShape struct {
	Code    string
	Formula string
}

// IndicatorFormulaShapes 返回某项指标可选的公式形状。
func IndicatorFormulaShapes(key string) []FormulaShape {
	switch key {
	case "improvement":
		return []FormulaShape{
			{Code: "class_grade_baseline",
				Formula: "class.average[subject] / grade.average[subject] / baseline.ratio[subject]"},
			{Code: "class_grade",
				Formula: "class.average[subject] / grade.average[subject]"},
			{Code: "class_baseline",
				Formula: "class.average[subject] / baseline.average[subject]"},
		}
	case "pass":
		return []FormulaShape{
			{Code: "pass_target", Formula: "statGroup.passCount / statGroup.target"},
			{Code: "pass_members", Formula: "statGroup.passCount / statGroup.memberCount"},
		}
	case "completion":
		return []FormulaShape{
			{Code: "tier_weighted", Formula: "tier.actual / tier.base"},
		}
	}
	return nil
}

// FormulaShapeFor 返回与公式文本匹配的形状代码，
// 忽略空白差异；没有匹配的形状时返回空串。
func FormulaShapeFor(key, formula string) string {
	target := normalizeFormula(formula)
	for _, shape := range IndicatorFormulaShapes(key) {
		if normalizeFormula(shape.Formula) == target {
			return shape.Code
		}
	}
	return ""
}

// FormulaForShape 返回某个形状的公式文本；
// 形状不存在时返回空串。
func FormulaForShape(key, code string) string {
	for _, shape := range IndicatorFormulaShapes(key) {
		if shape.Code == code {
			return shape.Formula
		}
	}
	return ""
}

func normalizeFormula(formula string) string {
	return strings.ReplaceAll(formula, " ", "")
}

// BoundaryRatioFormulas 按「把参评数量均分为 parts 份」
// 生成一档与二档边界表达式。
func BoundaryRatioFormulas(parts int) (string, string) {
	if parts < 2 {
		parts = 2
	}
	first := fmt.Sprintf("floor(n / %d)", parts)
	if parts == 2 {
		return first, "round(n / 2)"
	}
	return first, fmt.Sprintf("round(%d * n / %d)", parts-1, parts)
}

// BoundaryRatioParts 从一对边界表达式反推均等份数；
// 不符合均等形状时返回 0。
func BoundaryRatioParts(first, second string) int {
	for parts := 2; parts <= 10; parts++ {
		gotFirst, gotSecond := BoundaryRatioFormulas(parts)
		if normalizeFormula(gotFirst) == normalizeFormula(first) &&
			normalizeFormula(gotSecond) == normalizeFormula(second) {
			return parts
		}
	}
	return 0
}

// CompletionWeights 是完成率覆盖公式「目标完成×权重 + 本科达线×权重」的两个权重。
type CompletionWeights struct {
	TargetCompletion float64
	Undergraduate    float64
}

// ParseCompletionWeights 解析完成率覆盖公式的加权合成形状；
// 不符合形状时 ok 为 false。
func ParseCompletionWeights(formula string) (CompletionWeights, bool) {
	rest, ok := cutPrefix(normalizeFormula(formula), "program.targetCompletion*")
	if !ok {
		return CompletionWeights{}, false
	}
	weightText, remainder, ok := cutAt(rest, "+")
	if !ok {
		return CompletionWeights{}, false
	}
	targetCompletion, err := strconv.ParseFloat(weightText, 64)
	if err != nil {
		return CompletionWeights{}, false
	}
	undergraduateText, ok := cutPrefix(remainder, "program.undergraduatePass*")
	if !ok {
		return CompletionWeights{}, false
	}
	undergraduate, err := strconv.ParseFloat(undergraduateText, 64)
	if err != nil {
		return CompletionWeights{}, false
	}
	return CompletionWeights{TargetCompletion: targetCompletion, Undergraduate: undergraduate}, true
}

// CompletionWeightsFormula 生成完成率覆盖公式的加权合成形状。
func CompletionWeightsFormula(targetCompletion, undergraduate float64) string {
	return "program.targetCompletion * " + FormatWeight(targetCompletion) +
		" + program.undergraduatePass * " + FormatWeight(undergraduate)
}

// FormatWeight 把权重写成不带多余零的数字文本。
func FormatWeight(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func cutPrefix(s, prefix string) (string, bool) {
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

func cutAt(s, separator string) (string, string, bool) {
	index := strings.Index(s, separator)
	if index < 0 {
		return "", "", false
	}
	return s[:index], s[index+len(separator):], true
}
