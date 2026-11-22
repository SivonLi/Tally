package assessment

import (
	"fmt"

	"tally/internal/formula"
)

// ValidateFormulas 编译方案中的全部公式并检查指标依赖。
func (c Config) ValidateFormulas() error {
	keys := make(map[string]bool, len(c.Indicators))
	for _, indicator := range c.Indicators {
		keys[indicator.Key] = true
	}
	dependencies := map[string][]string{}
	for _, indicator := range c.Indicators {
		expr, err := formula.Compile(indicator.Formula, formula.Options{CurrentIndicator: indicator.Key})
		if err != nil {
			return fmt.Errorf("指标 %s 的公式无法编译: %w", indicator.Key, err)
		}
		references, err := expr.IndicatorReferences()
		if err != nil {
			return fmt.Errorf("指标 %s 的公式无法读取依赖: %w", indicator.Key, err)
		}
		for _, reference := range references {
			if !keys[reference] {
				return fmt.Errorf("指标 %s 引用了不存在的指标 %s", indicator.Key, reference)
			}
		}
		dependencies[indicator.Key] = references
	}
	for _, program := range c.Programs {
		for key, override := range program.IndicatorOverrides {
			if override.Formula == "" {
				continue
			}
			expr, err := formula.Compile(override.Formula, formula.Options{CurrentIndicator: key})
			if err != nil {
				return fmt.Errorf("专项规则中的指标 %s 公式无法编译: %w", key, err)
			}
			references, err := expr.IndicatorReferences()
			if err != nil {
				return fmt.Errorf("专项规则中的指标 %s 公式无法读取依赖: %w", key, err)
			}
			for _, reference := range references {
				if !keys[reference] {
					return fmt.Errorf("专项规则中的指标 %s 引用了不存在的指标 %s", key, reference)
				}
			}
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(key string) error {
		switch state[key] {
		case 1:
			return fmt.Errorf("指标 %s 存在循环引用", key)
		case 2:
			return nil
		}
		state[key] = 1
		for _, reference := range dependencies[key] {
			if err := visit(reference); err != nil {
				return err
			}
		}
		state[key] = 2
		return nil
	}
	for _, indicator := range c.Indicators {
		if err := visit(indicator.Key); err != nil {
			return err
		}
	}
	return nil
}

// EffectiveFormulas 返回各项指标在给定考试类型与级下实际使用的公式，
// 匹配条件的专项规则覆盖指标公式。
func EffectiveFormulas(c Config, examType, grade string) map[string]string {
	result := make(map[string]string, len(c.Indicators))
	for _, indicator := range c.Indicators {
		source := indicator.Formula
		for _, program := range c.Programs {
			if !programMatches(program.When, examType, grade) {
				continue
			}
			if override, ok := program.IndicatorOverrides[indicator.Key]; ok && override.Formula != "" {
				source = override.Formula
			}
		}
		result[indicator.Key] = source
	}
	return result
}

func programMatches(when ProgramWhen, examType, grade string) bool {
	if when.ExamType != "" && when.ExamType != examType {
		return false
	}
	if when.Grade != "" && when.Grade != grade {
		return false
	}
	return when.Cohort == ""
}
