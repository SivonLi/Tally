package calc

import (
	"fmt"
	"math"
	"sort"

	"tally/internal/assessment"
	"tally/internal/formula"
)

func calculateRow(input Input, group StatGroup, classes map[string]AdminClass, teachingGroups map[string]TeachingGroup, targets map[string]ClassTarget, expressions map[string]*formula.Expression, order []string, excluded map[string]bool) (Row, error) {
	memberSet := make(map[string]bool, len(group.MemberTeachingGroupIDs))
	for _, memberID := range group.MemberTeachingGroupIDs {
		memberSet[memberID] = true
	}
	count := 0
	sum := 0.0
	passCount := 0
	var participants []Participant
	reference := input.References[group.Subject]
	for _, student := range input.Students {
		if !studentValid(student, input.Plan.Data) {
			continue
		}
		teachingGroupID, ok, err := resolveStudentTeachingGroup(student, group, teachingGroups, memberSet)
		if err != nil {
			return Row{}, err
		}
		if !ok {
			continue
		}
		score, ok := student.Scores[group.Subject]
		if !ok {
			continue
		}
		count++
		sum += score
		participants = append(participants, Participant{StudentNo: student.ID, Name: student.Name})
		cutoff := reference.Cutoff
		if value, ok := reference.Cutoffs[student.Track]; ok {
			cutoff = value
		}
		if cutoff <= 0 {
			return Row{}, fmt.Errorf("学生 %s 的 %s 类分数线缺失", student.ID, student.Track)
		}
		if score >= cutoff {
			passCount++
		}
		_ = teachingGroupID
	}
	if count == 0 {
		return Row{}, fmt.Errorf("统计组 %s 没有可计算的有效成绩", group.ID)
	}
	classAverage := sum / float64(count)
	baselineRatio := group.BaselineRatio
	if baselineRatio <= 0 {
		baselineRatio = group.BaselineAverage / reference.GradeBaselineAverage
	}

	completionParticipating := !excluded[group.LeadClassID]
	target := ClassTarget{ClassID: group.LeadClassID, Counts: map[string]*float64{}}
	if completionParticipating {
		var ok bool
		target, ok = targets[group.LeadClassID]
		if !ok {
			return Row{}, fmt.Errorf("班级 %s 缺少完成率目标", group.LeadClassID)
		}
	}
	actualCounts := input.ActualTierCounts[group.LeadClassID]
	actual := 0.0
	base := 0.0
	tierActualValues := map[string]formula.Value{}
	tierTargetValues := map[string]formula.Value{}
	if completionParticipating {
		var err error
		actual, base, tierActualValues, tierTargetValues, err = weightedTiers(input.Plan.Tiers, target, actualCounts)
		if err != nil {
			return Row{}, err
		}
		if base == 0 {
			return Row{}, fmt.Errorf("班级 %s 的目标基础分为零", group.LeadClassID)
		}
	} else {
		for _, tier := range input.Plan.Tiers {
			tierActualValues[tier.Key] = formula.Number(actualCounts[tier.Key])
			tierTargetValues[tier.Key] = formula.Number(0)
		}
	}
	targetCompletion := 0.0
	undergraduatePass := 0.0
	if completionParticipating {
		targetCompletion = actual / base
		undergraduatePass = undergraduatePassRatio(input.Plan.Tiers, target, actualCounts)
	}

	env := formula.Env{
		"subject": formula.String(group.Subject),
		"class": formula.Object(map[string]formula.Value{
			"average": formula.Object(map[string]formula.Value{
				group.Subject: formula.Number(classAverage),
			}),
			"count": formula.Object(map[string]formula.Value{
				group.Subject: formula.Number(float64(count)),
			}),
		}),
		"grade": formula.Object(map[string]formula.Value{
			"average": formula.Object(map[string]formula.Value{
				group.Subject: formula.Number(reference.GradeAverage),
			}),
		}),
		"baseline": formula.Object(map[string]formula.Value{
			"average": formula.Object(map[string]formula.Value{
				group.Subject: formula.Number(group.BaselineAverage),
			}),
			"ratio": formula.Object(map[string]formula.Value{
				group.Subject: formula.Number(baselineRatio),
			}),
		}),
		"statGroup": formula.Object(map[string]formula.Value{
			"passCount":   formula.Number(float64(passCount)),
			"target":      formula.Number(group.Target),
			"memberCount": formula.Number(float64(len(group.MemberTeachingGroupIDs))),
		}),
		"tier": formula.Object(map[string]formula.Value{
			"actual": formula.Number(actual),
			"base":   formula.Number(base),
			"count":  formula.Object(tierActualValues),
			"target": formula.Object(tierTargetValues),
		}),
		"program": formula.Object(map[string]formula.Value{
			"targetCompletion":  formula.Number(targetCompletion),
			"undergraduatePass": formula.Number(undergraduatePass),
		}),
	}

	points := indicatorPoints(input)
	caps := indicatorCaps(input)
	values := map[string]float64{}
	rawValues := map[string]float64{}
	for _, key := range order {
		if key == "completion" && !completionParticipating {
			values[key] = 0
			rawValues[key] = 0
			continue
		}
		env["indicator"] = formula.Object(numberObject(values))
		value, err := expressions[key].Eval(env)
		if err != nil {
			return Row{}, fmt.Errorf("统计组 %s 的指标 %s 计算失败: %w", group.ID, key, err)
		}
		number, ok := value.Number()
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return Row{}, fmt.Errorf("统计组 %s 的指标 %s 返回值不是有限数值", group.ID, key)
		}
		rawValues[key] = number * points[key]
		values[key] = applyCap(rawValues[key], caps[key])
	}

	row := Row{
		StatGroupID:             group.ID,
		StatGroupName:           group.Name,
		Subject:                 group.Subject,
		LeadClassID:             group.LeadClassID,
		LeadClassName:           classes[group.LeadClassID].Name,
		ClassAverage:            classAverage,
		ClassCount:              count,
		GradeAverage:            reference.GradeAverage,
		BaselineAverage:         group.BaselineAverage,
		BaselineRatio:           baselineRatio,
		ImprovementRaw:          rawValues["improvement"],
		ImprovementScore:        values["improvement"],
		PassCount:               passCount,
		PassTarget:              group.Target,
		PassRaw:                 rawValues["pass"],
		PassScore:               values["pass"],
		CompletionActual:        actual,
		CompletionBase:          base,
		CompletionRaw:           rawValues["completion"],
		CompletionScore:         values["completion"],
		CompletionParticipating: completionParticipating,
		MemberCount:             len(group.MemberTeachingGroupIDs),
		Participants:            participants,
		IndicatorValues:         values,
	}
	if !completionParticipating {
		row.CompletionScore = 0
		row.IndicatorValues["completion"] = 0
	}
	row.TeacherNames = teacherNames(input.Assignments, group)
	row.Total = row.ImprovementScore + row.PassScore
	if row.CompletionParticipating {
		row.Total += row.CompletionScore
	}
	return row, nil
}

func studentValid(student Student, data assessment.DataConfig) bool {
	for _, status := range data.ExcludeStatus {
		if student.Status == status {
			return false
		}
	}
	for _, flag := range student.Flags {
		for _, excluded := range data.ExcludeFlags {
			if flag == excluded {
				return false
			}
		}
	}
	switch data.ValidStudent {
	case "registered_and_scored":
		return student.Registered && len(student.Scores) > 0
	case "registered":
		return student.Registered
	case "scored":
		return len(student.Scores) > 0
	default:
		return false
	}
}

// IsValidStudent 判断一名学生是否进入本批次的有效考生集合。
func IsValidStudent(student Student, data assessment.DataConfig) bool {
	return studentValid(student, data)
}

func resolveStudentTeachingGroup(student Student, group StatGroup, teachingGroups map[string]TeachingGroup, memberSet map[string]bool) (string, bool, error) {
	if groupID, ok := student.TeachingGroups[group.Subject]; ok {
		if memberSet[groupID] {
			return groupID, true, nil
		}
		return "", false, nil
	}
	var match string
	for memberID := range memberSet {
		member := teachingGroups[memberID]
		if member.AdminClassID != student.AdminClassID {
			continue
		}
		if match != "" {
			return "", false, fmt.Errorf("学生 %s 在统计组 %s 中缺少教学班归属，无法判断", student.ID, group.ID)
		}
		match = memberID
	}
	if match == "" {
		return "", false, nil
	}
	return match, true, nil
}

func weightedTiers(tiers []assessment.TierConfig, target ClassTarget, actualCounts map[string]float64) (float64, float64, map[string]formula.Value, map[string]formula.Value, error) {
	actualValues := make(map[string]formula.Value, len(tiers))
	targetValues := make(map[string]formula.Value, len(tiers))
	actual := 0.0
	base := 0.0
	for _, tier := range tiers {
		actualCount := actualCounts[tier.Key]
		targetCount, ok := target.Counts[tier.Key]
		if !ok || targetCount == nil {
			return 0, 0, nil, nil, fmt.Errorf("班级 %s 缺少档位 %s 的目标", target.ClassID, tier.Key)
		}
		actual += actualCount * tier.Weight
		base += *targetCount * tier.Weight
		actualValues[tier.Key] = formula.Number(actualCount)
		targetValues[tier.Key] = formula.Number(*targetCount)
	}
	return actual, base, actualValues, targetValues, nil
}

func undergraduatePassRatio(tiers []assessment.TierConfig, target ClassTarget, actualCounts map[string]float64) float64 {
	var undergraduate assessment.TierConfig
	found := false
	for _, tier := range tiers {
		if tier.Key == "undergraduate" {
			undergraduate = tier
			found = true
			break
		}
	}
	if !found {
		return 0
	}
	targetCount, ok := target.Counts[undergraduate.Key]
	if !ok || targetCount == nil {
		return 0
	}
	if *targetCount == 0 {
		return 0
	}
	return actualCounts[undergraduate.Key] / *targetCount
}

func numberObject(values map[string]float64) map[string]formula.Value {
	result := make(map[string]formula.Value, len(values))
	for key, value := range values {
		result[key] = formula.Number(value)
	}
	return result
}

func teacherNames(assignments []Assignment, group StatGroup) []string {
	memberSet := make(map[string]bool, len(group.MemberTeachingGroupIDs))
	for _, memberID := range group.MemberTeachingGroupIDs {
		memberSet[memberID] = true
	}
	seen := map[string]bool{}
	var names []string
	for _, assignment := range assignments {
		if !memberSet[assignment.TeachingGroupID] {
			continue
		}
		if assignment.TeacherName == "" || seen[assignment.TeacherName] {
			continue
		}
		seen[assignment.TeacherName] = true
		names = append(names, assignment.TeacherName)
	}
	sort.Strings(names)
	return names
}
