package importer

import (
	"sort"

	"tally/internal/excel"
	"tally/internal/store"
)

// SheetClassification 描述工作簿内单张工作表的自动识别角色、预览数据与列映射状态。
type SheetClassification struct {
	Sheet           string
	Kind            string
	Score           int
	Recommended     bool
	RowCount        int
	Headers         []string
	PreviewRows     [][]string
	Mapping         map[string]string
	MissingRequired []string
}

// ClassifyWorkbookSheets 一次性识别工作簿内所有工作表的数据角色、默认列映射、缺失必需列及前 N 行预览。
// scope 可选 "stage"（基础数据）、"exam"（考试数据）或 "all"（全部九类数据）。
func ClassifyWorkbookSheets(workbook *Workbook, scope string, subjects []store.Subject, tiers []string, previewLimit int) []SheetClassification {
	if workbook == nil || len(workbook.Tables) == 0 {
		return nil
	}
	if previewLimit <= 0 {
		previewLimit = 10
	}
	kinds := classificationKinds(scope)
	results := make([]SheetClassification, 0, len(workbook.Tables))
	bestByKind := map[string]int{}
	bestIndexByKind := map[string]int{}

	for _, table := range workbook.Tables {
		bestKind := ""
		bestScore := 0
		for _, kind := range kinds {
			score := DetectSheet(kind, table)
			if score > bestScore {
				bestScore = score
				bestKind = kind
			}
		}
		item := SheetClassification{
			Sheet:       table.Sheet,
			Kind:        bestKind,
			Score:       bestScore,
			RowCount:    len(table.Rows),
			Headers:     append([]string(nil), table.Headers...),
			PreviewRows: previewTableRows(table, previewLimit),
		}
		if bestKind != "" {
			item.Mapping = AutoMappingFor(bestKind, table, subjects, tiers)
			item.MissingRequired = missingRequiredFields(bestKind, table, item.Mapping, subjects, tiers)
			if bestScore > bestByKind[bestKind] {
				bestByKind[bestKind] = bestScore
				bestIndexByKind[bestKind] = len(results)
			}
		}
		results = append(results, item)
	}
	for _, idx := range bestIndexByKind {
		results[idx].Recommended = true
	}
	return results
}

func classificationKinds(scope string) []string {
	switch scope {
	case "stage":
		return []string{KindAssignments, KindBaselines, KindStudents, KindTeachers, KindClasses}
	case "exam":
		// 成绩表通常也含学号/姓名/班级，优先于报名名单参与比对，分数相同时先匹配更具体的成绩表。
		return []string{KindScores, KindCutoffs, KindTargets, KindRoster}
	default:
		return []string{
			KindScores, KindCutoffs, KindTargets, KindAssignments,
			KindBaselines, KindRoster, KindStudents, KindTeachers, KindClasses,
		}
	}
}

func previewTableRows(table excel.Table, limit int) [][]string {
	if len(table.Rows) == 0 || limit <= 0 {
		return nil
	}
	count := len(table.Rows)
	if count > limit {
		count = limit
	}
	rows := make([][]string, 0, count)
	for i := 0; i < count; i++ {
		values := append([]string(nil), table.Rows[i].Values...)
		rows = append(rows, values)
	}
	return rows
}

func missingRequiredFields(kind string, table excel.Table, mapping map[string]string, subjects []store.Subject, tiers []string) []string {
	fields := FieldsForTable(kind, table, subjects, tiers)
	var missing []string
	for _, field := range fields {
		if field.Required && mapping[field.Key] == "" {
			missing = append(missing, field.Key)
		}
	}
	sort.Strings(missing)
	return missing
}
