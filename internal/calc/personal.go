package calc

import (
	"fmt"
	"math"
	"sort"
)

type personalGroup struct {
	points      float64
	memberCount int
}

func calculatePersonal(input Input, teachingGroups map[string]TeachingGroup, rows []Row) ([]PersonalScore, error) {
	rowsByStatGroup := make(map[string]Row, len(rows))
	for _, row := range rows {
		rowsByStatGroup[row.StatGroupID] = row
	}
	statGroupsByTeachingGroup := map[string][]string{}
	for _, group := range input.StatGroups {
		for _, memberID := range group.MemberTeachingGroupIDs {
			statGroupsByTeachingGroup[memberID] = append(statGroupsByTeachingGroup[memberID], group.ID)
		}
	}

	type key struct {
		teacherID string
		subject   string
	}
	groups := map[key]map[string]personalGroup{}
	names := map[key]string{}
	for _, assignment := range input.Assignments {
		teachingGroup, ok := teachingGroups[assignment.TeachingGroupID]
		if !ok {
			return nil, fmt.Errorf("任课关系引用了不存在的教学班 %s", assignment.TeachingGroupID)
		}
		itemKey := key{teacherID: assignment.TeacherID, subject: teachingGroup.Subject}
		if groups[itemKey] == nil {
			groups[itemKey] = map[string]personalGroup{}
			names[itemKey] = assignment.TeacherName
		}
		for _, statGroupID := range statGroupsByTeachingGroup[assignment.TeachingGroupID] {
			row, ok := rowsByStatGroup[statGroupID]
			if !ok {
				return nil, fmt.Errorf("统计组 %s 没有计算结果", statGroupID)
			}
			groups[itemKey][statGroupID] = personalGroup{
				points:      row.Points,
				memberCount: row.MemberCount,
			}
		}
	}

	result := make([]PersonalScore, 0, len(groups))
	for itemKey, groupMap := range groups {
		items := make([]personalGroup, 0, len(groupMap))
		for _, item := range groupMap {
			items = append(items, item)
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].points > items[j].points
		})
		score, err := personalScore(input.Plan.Personal.Mode, items)
		if err != nil {
			return nil, err
		}
		points := make([]float64, len(items))
		for i, item := range items {
			points[i] = item.points
		}
		result = append(result, PersonalScore{
			TeacherID:   itemKey.teacherID,
			TeacherName: names[itemKey],
			Grade:       input.Grade,
			Subject:     itemKey.subject,
			GroupPoints: points,
			Score:       math.Round(score*10) / 10,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Subject != result[j].Subject {
			return result[i].Subject < result[j].Subject
		}
		if result[i].TeacherName != result[j].TeacherName {
			return result[i].TeacherName < result[j].TeacherName
		}
		return result[i].TeacherID < result[j].TeacherID
	})
	return result, nil
}

func personalScore(mode string, items []personalGroup) (float64, error) {
	if len(items) == 0 {
		return 0, nil
	}
	switch mode {
	case "top_two_average":
		total := items[0].points
		count := 1
		if len(items) > 1 {
			total += items[1].points
			count = 2
		}
		return total / float64(count), nil
	case "average_all":
		total := 0.0
		for _, item := range items {
			total += item.points
		}
		return total / float64(len(items)), nil
	case "top_one":
		return items[0].points, nil
	case "lowest_one":
		return items[len(items)-1].points, nil
	case "weighted_by_count":
		total := 0.0
		weight := 0
		for _, item := range items {
			total += item.points * float64(item.memberCount)
			weight += item.memberCount
		}
		if weight == 0 {
			return 0, fmt.Errorf("个人得分的权重为零")
		}
		return total / float64(weight), nil
	default:
		return 0, fmt.Errorf("未知的个人得分方式 %q", mode)
	}
}
