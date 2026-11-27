package calc

import (
	"fmt"
	"math"
	"sort"

	"tally/internal/assessment"
)

func applyRanking(rows []Row, ranking assessment.RankingConfig, overrides map[string]BoundaryOverride) ([]EffectiveBoundary, error) {
	groups := map[string][]int{}
	for i := range rows {
		key := rankGroupKey(rows[i], ranking.GroupBy)
		groups[key] = append(groups[key], i)
	}
	groupKeys := make([]string, 0, len(groups))
	for key := range groups {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)

	var staleIssues []BoundaryStaleIssue
	effective := make([]EffectiveBoundary, 0, len(groupKeys))
	groupBounds := make(map[string][2]int, len(groupKeys))

	for _, key := range groupKeys {
		indexes := groups[key]
		subject := rows[indexes[0]].Subject
		n := len(indexes)
		if override, ok := overrides[subject]; ok {
			if n != override.GroupCount || override.First < 1 || override.Second < override.First || override.Second > n {
				staleIssues = append(staleIssues, BoundaryStaleIssue{
					Subject:          subject,
					SavedGroupCount:  override.GroupCount,
					ActualGroupCount: n,
					First:            override.First,
					Second:           override.Second,
				})
				continue
			}
			groupBounds[key] = [2]int{override.First, override.Second}
			effective = append(effective, EffectiveBoundary{
				Subject:    subject,
				GroupCount: n,
				First:      override.First,
				Second:     override.Second,
				Source:     "override",
				Reason:     override.Reason,
			})
			continue
		}
		first, second, err := DefaultRankBoundaries(ranking.Boundary, subject, n)
		if err != nil {
			return nil, err
		}
		groupBounds[key] = [2]int{first, second}
		effective = append(effective, EffectiveBoundary{
			Subject:    subject,
			GroupCount: n,
			First:      first,
			Second:     second,
			Source:     "default",
		})
	}
	if len(staleIssues) > 0 {
		sort.Slice(staleIssues, func(i, j int) bool {
			return staleIssues[i].Subject < staleIssues[j].Subject
		})
		return nil, &BoundaryStaleError{Issues: staleIssues}
	}

	for _, key := range groupKeys {
		indexes := groups[key]
		bounds := groupBounds[key]
		first, second := bounds[0], bounds[1]
		sort.SliceStable(indexes, func(left, right int) bool {
			a := rows[indexes[left]]
			b := rows[indexes[right]]
			if almostEqual(a.Total, b.Total) {
				return a.StatGroupID < b.StatGroupID
			}
			return a.Total > b.Total
		})
		assignRanks(rows, indexes, ranking.TieMode)
		for _, index := range indexes {
			tier := 0
			switch {
			case rows[index].Rank <= first:
				tier = 0
			case rows[index].Rank <= second:
				tier = 1
			default:
				tier = 2
			}
			if tier >= len(ranking.Points) {
				return nil, fmt.Errorf("统计组 %s 的档次超出赋分范围", rows[index].StatGroupID)
			}
			rows[index].TierIndex = tier
			rows[index].Points = ranking.Points[tier]
		}
	}
	return effective, nil
}

func rankGroupKey(row Row, groupBy []string) string {
	switch {
	case len(groupBy) == 2 && groupBy[0] == "grade" && groupBy[1] == "subject":
		return row.Subject
	case len(groupBy) == 1 && groupBy[0] == "grade":
		return "grade"
	case len(groupBy) == 1 && groupBy[0] == "school":
		return "school"
	default:
		return row.Subject
	}
}

// DefaultRankBoundaries 按方案配置计算某学科在参评统计组数为 n 时的默认一档、二档截止名次。
func DefaultRankBoundaries(boundary assessment.BoundaryConfig, subject string, n int) (int, int, error) {
	return rankBoundaries(boundary, subject, n)
}

func rankBoundaries(boundary assessment.BoundaryConfig, subject string, n int) (int, int, error) {
	if n <= 0 {
		return 0, 0, fmt.Errorf("参评统计组数量必须大于零")
	}
	first := 0
	second := 0
	switch boundary.Mode {
	case "manual":
		found := false
		for _, item := range boundary.Manual {
			if item.Subject != subject {
				continue
			}
			if item.N != 0 && item.N != n {
				continue
			}
			first = item.First
			second = item.Second
			found = true
		}
		if !found {
			return 0, 0, fmt.Errorf("学科 %s 在参评数量 %d 时缺少人工分档边界", subject, n)
		}
	case "ratio":
		if n <= 2 {
			first = n
			second = n
		} else {
			first = n / 3
			if first < 1 {
				first = 1
			}
			second = int(math.Round(float64(2*n) / 3.0))
			if second < first {
				second = first
			}
			if second > n {
				second = n
			}
		}
		for _, override := range boundary.Overrides {
			if override.Subject == subject && override.N == n {
				first = override.First
				second = override.Second
			}
		}
	default:
		return 0, 0, fmt.Errorf("未知的分档模式 %q", boundary.Mode)
	}
	if first < 1 {
		first = 1
	}
	if second < first {
		second = first
	}
	if second > n {
		second = n
	}
	if first > n {
		first = n
	}
	return first, second, nil
}

func assignRanks(rows []Row, indexes []int, mode string) {
	switch mode {
	case "competition":
		position := 0
		for position < len(indexes) {
			end := position + 1
			for end < len(indexes) && almostEqual(rows[indexes[position]].Total, rows[indexes[end]].Total) {
				end++
			}
			rank := position + 1
			for i := position; i < end; i++ {
				rows[indexes[i]].Rank = rank
			}
			position = end
		}
	case "dense":
		rank := 0
		for i, index := range indexes {
			if i == 0 || !almostEqual(rows[indexes[i-1]].Total, rows[index].Total) {
				rank++
			}
			rows[index].Rank = rank
		}
	case "ordinal":
		for i, index := range indexes {
			rows[index].Rank = i + 1
		}
	default:
		for i, index := range indexes {
			rows[index].Rank = i + 1
		}
	}
}
