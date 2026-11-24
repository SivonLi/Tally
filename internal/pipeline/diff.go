package pipeline

import (
	"context"
	"fmt"
	"math"
	"sort"

	"tally/internal/store"
)

const diffEpsilon = 1e-6

// RowDiff 记录同一统计组与学科在两次核算运行（如正式 vs 试算）间的得分、排名、档次与赋分差异。
// RankDelta 为 BaseRank - CompareRank，正值表示对比运行名次上升。
type RowDiff struct {
	StatGroupID   int64
	StatGroupName string
	Subject       string
	LeadClassName string
	TeacherNames  string
	Presence      string
	BaseTotal     float64
	CompareTotal  float64
	TotalDelta    float64
	BaseRank      int
	CompareRank   int
	RankDelta     int
	BaseTier      int
	CompareTier   int
	TierChanged   bool
	BasePoints    float64
	ComparePoints float64
	PointsDelta   float64
}

// PersonalDiff 记录同一教师在两次核算运行间的个人得分差异。
type PersonalDiff struct {
	TeacherID    int64
	TeacherName  string
	Grade        string
	Subject      string
	Presence     string
	BaseScore    float64
	CompareScore float64
	ScoreDelta   float64
}

// RunDiff 汇总两次核算运行的完整对比结果。
type RunDiff struct {
	BaseRun              store.ResultRun
	CompareRun           store.ResultRun
	Rows                 []RowDiff
	Personal             []PersonalDiff
	ChangedRowCount      int
	ChangedTierCount     int
	ChangedPersonalCount int
}

// DiffAssessmentRuns 读取同一考核下的两次核算运行并计算差异。
func DiffAssessmentRuns(ctx context.Context, storage *store.Store, assessmentID, baseRunID, compareRunID int64) (RunDiff, error) {
	baseRun, baseRows, basePersonal, err := storage.AssessmentRunDetail(ctx, assessmentID, baseRunID)
	if err != nil {
		return RunDiff{}, err
	}
	compareRun, compareRows, comparePersonal, err := storage.AssessmentRunDetail(ctx, assessmentID, compareRunID)
	if err != nil {
		return RunDiff{}, err
	}
	return DiffRuns(baseRun, baseRows, basePersonal, compareRun, compareRows, comparePersonal), nil
}

// DiffRuns 纯函数对比两次核算结果行与教师个人得分，生成结构化差异。
func DiffRuns(baseRun store.ResultRun, baseRows []store.StoredResultRow, basePersonal []store.StoredPersonalScore,
	compareRun store.ResultRun, compareRows []store.StoredResultRow, comparePersonal []store.StoredPersonalScore) RunDiff {
	diff := RunDiff{
		BaseRun:    baseRun,
		CompareRun: compareRun,
	}

	rowKey := func(r store.StoredResultRow) string {
		return fmt.Sprintf("%d\x00%s", r.StatGroupID, r.Subject)
	}
	baseRowMap := make(map[string]store.StoredResultRow, len(baseRows))
	for _, r := range baseRows {
		baseRowMap[rowKey(r)] = r
	}
	compareRowMap := make(map[string]store.StoredResultRow, len(compareRows))
	for _, r := range compareRows {
		compareRowMap[rowKey(r)] = r
	}

	seenRow := map[string]bool{}
	var rows []RowDiff
	for _, b := range baseRows {
		k := rowKey(b)
		seenRow[k] = true
		if c, ok := compareRowMap[k]; ok {
			item := RowDiff{
				StatGroupID:   b.StatGroupID,
				StatGroupName: b.StatGroupName,
				Subject:       b.Subject,
				LeadClassName: b.LeadClassName,
				TeacherNames:  b.TeacherNames,
				Presence:      "both",
				BaseTotal:     b.Total,
				CompareTotal:  c.Total,
				TotalDelta:    c.Total - b.Total,
				BaseRank:      b.Rank,
				CompareRank:   c.Rank,
				RankDelta:     b.Rank - c.Rank,
				BaseTier:      b.TierIndex,
				CompareTier:   c.TierIndex,
				TierChanged:   b.TierIndex != c.TierIndex,
				BasePoints:    b.Points,
				ComparePoints: c.Points,
				PointsDelta:   c.Points - b.Points,
			}
			if item.TierChanged {
				diff.ChangedTierCount++
			}
			if math.Abs(item.TotalDelta) > diffEpsilon || item.RankDelta != 0 || item.TierChanged || math.Abs(item.PointsDelta) > diffEpsilon {
				diff.ChangedRowCount++
			}
			rows = append(rows, item)
		} else {
			diff.ChangedRowCount++
			rows = append(rows, RowDiff{
				StatGroupID:   b.StatGroupID,
				StatGroupName: b.StatGroupName,
				Subject:       b.Subject,
				LeadClassName: b.LeadClassName,
				TeacherNames:  b.TeacherNames,
				Presence:      "only_base",
				BaseTotal:     b.Total,
				TotalDelta:    -b.Total,
				BaseRank:      b.Rank,
				BaseTier:      b.TierIndex,
				TierChanged:   true,
				BasePoints:    b.Points,
				PointsDelta:   -b.Points,
			})
		}
	}
	for _, c := range compareRows {
		k := rowKey(c)
		if seenRow[k] {
			continue
		}
		diff.ChangedRowCount++
		rows = append(rows, RowDiff{
			StatGroupID:   c.StatGroupID,
			StatGroupName: c.StatGroupName,
			Subject:       c.Subject,
			LeadClassName: c.LeadClassName,
			TeacherNames:  c.TeacherNames,
			Presence:      "only_compare",
			CompareTotal:  c.Total,
			TotalDelta:    c.Total,
			CompareRank:   c.Rank,
			CompareTier:   c.TierIndex,
			TierChanged:   true,
			ComparePoints: c.Points,
			PointsDelta:   c.Points,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Subject != rows[j].Subject {
			return rows[i].Subject < rows[j].Subject
		}
		if rows[i].CompareRank != rows[j].CompareRank {
			if rows[i].CompareRank == 0 {
				return false
			}
			if rows[j].CompareRank == 0 {
				return true
			}
			return rows[i].CompareRank < rows[j].CompareRank
		}
		return rows[i].StatGroupName < rows[j].StatGroupName
	})
	diff.Rows = rows

	personalKey := func(p store.StoredPersonalScore) string {
		return fmt.Sprintf("%s\x00%s\x00%s", p.Grade, p.Subject, p.TeacherName)
	}
	comparePersonalMap := make(map[string]store.StoredPersonalScore, len(comparePersonal))
	for _, p := range comparePersonal {
		comparePersonalMap[personalKey(p)] = p
	}
	seenPersonal := map[string]bool{}
	var personal []PersonalDiff
	for _, b := range basePersonal {
		k := personalKey(b)
		seenPersonal[k] = true
		if c, ok := comparePersonalMap[k]; ok {
			delta := c.Score - b.Score
			if math.Abs(delta) > diffEpsilon {
				diff.ChangedPersonalCount++
			}
			personal = append(personal, PersonalDiff{
				TeacherID:    b.TeacherID,
				TeacherName:  b.TeacherName,
				Grade:        b.Grade,
				Subject:      b.Subject,
				Presence:     "both",
				BaseScore:    b.Score,
				CompareScore: c.Score,
				ScoreDelta:   delta,
			})
		} else {
			diff.ChangedPersonalCount++
			personal = append(personal, PersonalDiff{
				TeacherID:   b.TeacherID,
				TeacherName: b.TeacherName,
				Grade:       b.Grade,
				Subject:     b.Subject,
				Presence:    "only_base",
				BaseScore:   b.Score,
				ScoreDelta:  -b.Score,
			})
		}
	}
	for _, c := range comparePersonal {
		k := personalKey(c)
		if seenPersonal[k] {
			continue
		}
		diff.ChangedPersonalCount++
		personal = append(personal, PersonalDiff{
			TeacherID:    c.TeacherID,
			TeacherName:  c.TeacherName,
			Grade:        c.Grade,
			Subject:      c.Subject,
			Presence:     "only_compare",
			CompareScore: c.Score,
			ScoreDelta:   c.Score,
		})
	}
	sort.Slice(personal, func(i, j int) bool {
		if personal[i].Subject != personal[j].Subject {
			return personal[i].Subject < personal[j].Subject
		}
		return personal[i].TeacherName < personal[j].TeacherName
	})
	diff.Personal = personal
	return diff
}
