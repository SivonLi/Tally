package calc

import (
	"fmt"
	"math"
	"sort"

	"tally/internal/assessment"
	"tally/internal/formula"
)

// Calculate 按方案计算一个批次的全部结果。
func Calculate(input Input, options Options) (Result, error) {
	if err := input.Plan.Validate(); err != nil {
		return Result{}, err
	}
	if input.Grade == "" {
		return Result{}, fmt.Errorf("年级不能为空")
	}
	if input.ExamType == "" {
		return Result{}, fmt.Errorf("考试类型不能为空")
	}
	if len(input.Classes) == 0 {
		return Result{}, fmt.Errorf("班级不能为空")
	}
	if len(input.StatGroups) == 0 {
		return Result{}, fmt.Errorf("统计组不能为空")
	}
	if len(input.Students) == 0 {
		return Result{}, fmt.Errorf("学生不能为空")
	}

	classes, err := indexClasses(input.Classes)
	if err != nil {
		return Result{}, err
	}
	teachingGroups, err := indexTeachingGroups(input.TeachingGroups, classes)
	if err != nil {
		return Result{}, err
	}
	statGroups, err := indexStatGroups(input.StatGroups, teachingGroups, classes)
	if err != nil {
		return Result{}, err
	}
	targets, err := indexTargets(input.ClassTargets)
	if err != nil {
		return Result{}, err
	}
	if err := validateReferences(input, statGroups); err != nil {
		return Result{}, err
	}
	if err := validateStudents(input.Students, classes, teachingGroups); err != nil {
		return Result{}, err
	}

	excluded := make(map[string]bool, len(options.ExcludedCompletionClasses))
	for classID := range options.ExcludedCompletionClasses {
		excluded[classID] = true
	}
	issues, err := zeroTargetIssues(input, classes, statGroups, targets, excluded)
	if err != nil {
		return Result{}, err
	}
	if len(issues) > 0 {
		return Result{}, &ZeroTargetError{Issues: issues}
	}

	expressions, order, err := compileIndicators(input)
	if err != nil {
		return Result{}, err
	}
	rows := make([]Row, 0, len(input.StatGroups))
	for _, group := range input.StatGroups {
		row, err := calculateRow(input, group, classes, teachingGroups, targets, expressions, order, excluded)
		if err != nil {
			return Result{}, err
		}
		rows = append(rows, row)
	}
	if err := applyRanking(rows, input.Plan.Ranking); err != nil {
		return Result{}, err
	}
	personal, err := calculatePersonal(input, teachingGroups, rows)
	if err != nil {
		return Result{}, err
	}
	excludedIDs := make([]string, 0, len(excluded))
	for classID := range excluded {
		excludedIDs = append(excludedIDs, classID)
	}
	sort.Strings(excludedIDs)
	return Result{Rows: rows, Personal: personal, ExcludedCompletionClasses: excludedIDs}, nil
}

func indexClasses(classes []AdminClass) (map[string]AdminClass, error) {
	result := make(map[string]AdminClass, len(classes))
	for _, class := range classes {
		if class.ID == "" {
			return nil, fmt.Errorf("班级标识不能为空")
		}
		if _, exists := result[class.ID]; exists {
			return nil, fmt.Errorf("班级标识 %s 重复", class.ID)
		}
		result[class.ID] = class
	}
	return result, nil
}

func indexTeachingGroups(groups []TeachingGroup, classes map[string]AdminClass) (map[string]TeachingGroup, error) {
	result := make(map[string]TeachingGroup, len(groups))
	for _, group := range groups {
		if group.ID == "" {
			return nil, fmt.Errorf("教学班标识不能为空")
		}
		if _, exists := result[group.ID]; exists {
			return nil, fmt.Errorf("教学班标识 %s 重复", group.ID)
		}
		if _, ok := classes[group.AdminClassID]; !ok {
			return nil, fmt.Errorf("教学班 %s 引用了不存在的行政班 %s", group.ID, group.AdminClassID)
		}
		if group.Subject == "" {
			return nil, fmt.Errorf("教学班 %s 缺少学科", group.ID)
		}
		result[group.ID] = group
	}
	return result, nil
}

func indexStatGroups(groups []StatGroup, teachingGroups map[string]TeachingGroup, classes map[string]AdminClass) (map[string]StatGroup, error) {
	result := make(map[string]StatGroup, len(groups))
	for _, group := range groups {
		if group.ID == "" {
			return nil, fmt.Errorf("统计组标识不能为空")
		}
		if _, exists := result[group.ID]; exists {
			return nil, fmt.Errorf("统计组标识 %s 重复", group.ID)
		}
		if group.Subject == "" {
			return nil, fmt.Errorf("统计组 %s 缺少学科", group.ID)
		}
		if _, ok := classes[group.LeadClassID]; !ok {
			return nil, fmt.Errorf("统计组 %s 引用了不存在的承担统计职责班级 %s", group.ID, group.LeadClassID)
		}
		if len(group.MemberTeachingGroupIDs) == 0 {
			return nil, fmt.Errorf("统计组 %s 没有成员教学班", group.ID)
		}
		for _, memberID := range group.MemberTeachingGroupIDs {
			member, ok := teachingGroups[memberID]
			if !ok {
				return nil, fmt.Errorf("统计组 %s 引用了不存在的教学班 %s", group.ID, memberID)
			}
			if member.Subject != group.Subject {
				return nil, fmt.Errorf("统计组 %s 的成员教学班 %s 学科不一致", group.ID, memberID)
			}
		}
		result[group.ID] = group
	}
	return result, nil
}

func indexTargets(targets []ClassTarget) (map[string]ClassTarget, error) {
	result := make(map[string]ClassTarget, len(targets))
	for _, target := range targets {
		if target.ClassID == "" {
			return nil, fmt.Errorf("目标缺少班级标识")
		}
		if _, exists := result[target.ClassID]; exists {
			return nil, fmt.Errorf("班级 %s 的目标重复", target.ClassID)
		}
		result[target.ClassID] = target
	}
	return result, nil
}

func validateReferences(input Input, statGroups map[string]StatGroup) error {
	for _, group := range statGroups {
		reference, ok := input.References[group.Subject]
		if !ok {
			return fmt.Errorf("学科 %s 缺少年级均分、基数均分或者分数线", group.Subject)
		}
		if reference.GradeAverage <= 0 {
			return fmt.Errorf("学科 %s 的年级均分必须大于零", group.Subject)
		}
		if reference.GradeBaselineAverage <= 0 {
			return fmt.Errorf("学科 %s 的年级基数均分必须大于零", group.Subject)
		}
		if reference.Cutoff <= 0 && len(reference.Cutoffs) == 0 {
			return fmt.Errorf("学科 %s 的分数线必须大于零", group.Subject)
		}
		for track, cutoff := range reference.Cutoffs {
			if track == "" || cutoff <= 0 {
				return fmt.Errorf("学科 %s 的 %s 类分数线必须大于零", group.Subject, track)
			}
		}
		if group.BaselineAverage <= 0 && group.BaselineRatio <= 0 {
			return fmt.Errorf("统计组 %s 的基数均分必须大于零", group.ID)
		}
		if group.Target < 0 {
			return fmt.Errorf("统计组 %s 的目标不能为负数", group.ID)
		}
	}
	for _, assignment := range input.Assignments {
		if assignment.TeacherID == "" {
			return fmt.Errorf("任课关系缺少教师标识")
		}
		if _, ok := inputTeachingGroup(input.TeachingGroups, assignment.TeachingGroupID); !ok {
			return fmt.Errorf("任课关系引用了不存在的教学班 %s", assignment.TeachingGroupID)
		}
	}
	return nil
}

func inputTeachingGroup(groups []TeachingGroup, id string) (TeachingGroup, bool) {
	for _, group := range groups {
		if group.ID == id {
			return group, true
		}
	}
	return TeachingGroup{}, false
}

func validateStudents(students []Student, classes map[string]AdminClass, teachingGroups map[string]TeachingGroup) error {
	seen := map[string]bool{}
	for _, student := range students {
		if student.ID == "" {
			return fmt.Errorf("学生标识不能为空")
		}
		if seen[student.ID] {
			return fmt.Errorf("学生标识 %s 重复", student.ID)
		}
		seen[student.ID] = true
		if _, ok := classes[student.AdminClassID]; !ok {
			return fmt.Errorf("学生 %s 引用了不存在的行政班 %s", student.ID, student.AdminClassID)
		}
		for subject, groupID := range student.TeachingGroups {
			group, ok := teachingGroups[groupID]
			if !ok {
				return fmt.Errorf("学生 %s 引用了不存在的教学班 %s", student.ID, groupID)
			}
			if group.Subject != subject {
				return fmt.Errorf("学生 %s 的学科 %s 与教学班 %s 不一致", student.ID, subject, groupID)
			}
		}
	}
	return nil
}

func zeroTargetIssues(input Input, classes map[string]AdminClass, statGroups map[string]StatGroup, targets map[string]ClassTarget, excluded map[string]bool) ([]ZeroTargetIssue, error) {
	leadClasses := map[string]bool{}
	for _, group := range statGroups {
		leadClasses[group.LeadClassID] = true
	}
	var issues []ZeroTargetIssue
	for classID := range leadClasses {
		if excluded[classID] {
			if !input.Plan.Validation.ZeroTarget.AllowNotParticipating {
				return nil, fmt.Errorf("方案不允许班级 %s 不参与完成率计算", classID)
			}
			continue
		}
		target, ok := targets[classID]
		missing := []string{}
		base := 0.0
		if !ok {
			for _, tier := range input.Plan.Tiers {
				missing = append(missing, tier.Key)
			}
		} else {
			for _, tier := range input.Plan.Tiers {
				count, exists := target.Counts[tier.Key]
				if !exists || count == nil {
					missing = append(missing, tier.Key)
					continue
				}
				base += *count * tier.Weight
			}
		}
		if len(missing) > 0 || base == 0 {
			class := classes[classID]
			issues = append(issues, ZeroTargetIssue{
				ClassID:      classID,
				ClassName:    class.Name,
				AllTiers:     tierKeys(input.Plan.Tiers),
				MissingTiers: missing,
				ExplicitZero: len(missing) == 0 && base == 0,
			})
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].ClassID < issues[j].ClassID
	})
	return issues, nil
}

func tierKeys(tiers []assessment.TierConfig) []string {
	keys := make([]string, 0, len(tiers))
	for _, tier := range tiers {
		keys = append(keys, tier.Key)
	}
	return keys
}

func compileIndicators(input Input) (map[string]*formula.Expression, []string, error) {
	expressions := make(map[string]*formula.Expression, len(input.Plan.Indicators))
	dependencies := make(map[string][]string, len(input.Plan.Indicators))
	for _, indicator := range input.Plan.Indicators {
		source := effectiveFormula(input, indicator)
		expr, err := formula.Compile(source, formula.Options{CurrentIndicator: indicator.Key})
		if err != nil {
			return nil, nil, fmt.Errorf("指标 %s 的公式无法编译: %w", indicator.Key, err)
		}
		references, err := expr.IndicatorReferences()
		if err != nil {
			return nil, nil, fmt.Errorf("指标 %s 的公式无法读取依赖: %w", indicator.Key, err)
		}
		expressions[indicator.Key] = expr
		dependencies[indicator.Key] = references
	}

	state := map[string]int{}
	var order []string
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
			if _, ok := expressions[reference]; !ok {
				return fmt.Errorf("指标 %s 引用了不存在的指标 %s", key, reference)
			}
			if err := visit(reference); err != nil {
				return err
			}
		}
		state[key] = 2
		order = append(order, key)
		return nil
	}
	for _, indicator := range input.Plan.Indicators {
		if err := visit(indicator.Key); err != nil {
			return nil, nil, err
		}
	}
	return expressions, order, nil
}

func effectiveFormula(input Input, indicator assessment.IndicatorConfig) string {
	source := indicator.Formula
	for _, program := range input.Plan.Programs {
		if !programMatches(program.When, input) {
			continue
		}
		override, ok := program.IndicatorOverrides[indicator.Key]
		if ok && override.Formula != "" {
			source = override.Formula
		}
	}
	return source
}

func programMatches(when assessment.ProgramWhen, input Input) bool {
	if when.ExamType != "" && when.ExamType != input.ExamType {
		return false
	}
	if when.Grade != "" && when.Grade != input.Grade {
		return false
	}
	if when.Cohort != "" && when.Cohort != input.Cohort {
		return false
	}
	return true
}

func indicatorPoints(input Input) map[string]float64 {
	result := make(map[string]float64, len(input.Plan.Indicators))
	for _, indicator := range input.Plan.Indicators {
		result[indicator.Key] = indicator.Points
	}
	return result
}

func indicatorCaps(input Input) map[string]*assessment.CapConfig {
	result := make(map[string]*assessment.CapConfig, len(input.Plan.Indicators))
	for _, indicator := range input.Plan.Indicators {
		result[indicator.Key] = indicator.Cap
	}
	return result
}

func applyCap(value float64, cap *assessment.CapConfig) float64 {
	if cap == nil {
		return value
	}
	if cap.Min != nil && value < *cap.Min {
		value = *cap.Min
	}
	if cap.Max != nil && value > *cap.Max {
		value = *cap.Max
	}
	return value
}

func almostEqual(left, right float64) bool {
	return math.Abs(left-right) < 1e-9
}
