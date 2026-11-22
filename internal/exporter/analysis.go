package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"tally/internal/excel"
	"tally/internal/store"
)

// ExportTeacherTrend 导出教师趋势分析工作簿。
func ExportTeacherTrend(path string, data store.TeacherTrendResult) error {
	if path == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建导出目录: %w", err)
	}

	workbook := excel.New()
	defer workbook.Close()

	headerStyle, bodyStyle, err := createStyles(workbook)
	if err != nil {
		return err
	}

	defaultSheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetName(defaultSheet, "教师趋势汇总"); err != nil {
		return err
	}

	summaryRows := [][]any{
		{"教师姓名", data.TeacherName},
		{"任教学科", data.Subject},
		{"历次平均得分", fmt.Sprintf("%.2f", data.AveragePersonalScore)},
		{"最近一次得分", fmt.Sprintf("%.2f", data.LatestPersonalScore)},
		{"方案是否跨版本", boolText(data.HasMultiplePlans)},
		{"导出时间", time.Now().Format("2006-01-02 15:04:05")},
		{},
		{"考试名称", "考试日期", "学年学期", "方案版本", "个人总分", "任教组数"},
	}

	for _, pt := range data.Points {
		summaryRows = append(summaryRows, []any{
			pt.ExamName, pt.ExamDate, pt.AcademicYear + " " + pt.Semester,
			pt.PlanVersionID, pt.PersonalScore, len(pt.Rows),
		})
	}

	if err := writeTable(workbook, "教师趋势汇总", summaryRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth("教师趋势汇总", "A", "F", 20)

	// 明细 Sheet
	detailSheet := "历次明细"
	_, _ = workbook.NewSheet(detailSheet)
	detailRows := [][]any{
		{"考试名称", "考试日期", "班级", "学科", "统计组", "班级均分", "年级均分", "提升率得分", "达线率得分", "完成率得分", "量化总分", "名次", "档次", "赋分"},
	}
	for _, pt := range data.Points {
		for _, r := range pt.Rows {
			detailRows = append(detailRows, []any{
				pt.ExamName, pt.ExamDate, r.LeadClassName, r.Subject, r.StatGroupName,
				r.ClassAverage, r.GradeAverage, r.ImprovementScore, r.PassRateScore,
				r.CompletionScore, r.Total, r.Rank, r.Tier, r.Points,
			})
		}
	}
	if err := writeTable(workbook, detailSheet, detailRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth(detailSheet, "A", "N", 16)

	return saveAtomic(workbook, path)
}

// ExportClassTrend 导出班级趋势分析工作簿。
func ExportClassTrend(path string, data store.ClassTrendResult) error {
	if path == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建导出目录: %w", err)
	}

	workbook := excel.New()
	defer workbook.Close()

	headerStyle, bodyStyle, err := createStyles(workbook)
	if err != nil {
		return err
	}

	defaultSheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetName(defaultSheet, "班级趋势"); err != nil {
		return err
	}

	rows := [][]any{
		{"班级名称", data.ClassName},
		{"平均完成率得分", fmt.Sprintf("%.2f", data.AverageCompletionScore)},
		{"导出时间", time.Now().Format("2006-01-02 15:04:05")},
		{},
		{"考试名称", "考试日期", "学年学期", "学科", "任课教师", "班级均分", "年级均分", "提升率得分", "达线率得分", "完成率得分", "总分", "名次", "档位", "赋分"},
	}

	for _, pt := range data.Points {
		for _, r := range pt.Rows {
			rows = append(rows, []any{
				pt.ExamName, pt.ExamDate, pt.AcademicYear + " " + pt.Semester,
				r.Subject, strings.Join(r.TeacherNames, "、"),
				r.ClassAverage, r.GradeAverage, r.ImprovementScore, r.PassRateScore,
				r.CompletionScore, r.Total, r.Rank, r.Tier, r.Points,
			})
		}
	}

	if err := writeTable(workbook, "班级趋势", rows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth("班级趋势", "A", "N", 16)

	return saveAtomic(workbook, path)
}

// ExportSubjectComparison 导出学科横向对比工作簿。
func ExportSubjectComparison(path string, data store.SubjectComparisonResult) error {
	if path == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建导出目录: %w", err)
	}

	workbook := excel.New()
	defer workbook.Close()

	headerStyle, bodyStyle, err := createStyles(workbook)
	if err != nil {
		return err
	}

	defaultSheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetName(defaultSheet, "学科对比"); err != nil {
		return err
	}

	rows := [][]any{
		{"考试名称", data.ExamName},
		{"学科", data.Subject},
		{"参评组数", data.GroupCount},
		{"平均量化得分", fmt.Sprintf("%.2f", data.AverageScore)},
		{"平均提升率得分", fmt.Sprintf("%.2f", data.AverageImprovement)},
		{"平均达线率得分", fmt.Sprintf("%.2f", data.AveragePassRate)},
		{"导出时间", time.Now().Format("2006-01-02 15:04:05")},
		{},
		{"名次", "班级", "任课教师", "统计组", "班级均分", "年级均分", "提升率得分", "达线率得分", "完成率得分", "教学量化总分", "档位", "赋分"},
	}

	for _, g := range data.Groups {
		rows = append(rows, []any{
			g.Rank, g.LeadClassName, strings.Join(g.TeacherNames, "、"), g.StatGroupName,
			g.ClassAverage, g.GradeAverage, g.ImprovementScore, g.PassRateScore,
			g.CompletionScore, g.Total, g.Tier, g.Points,
		})
	}

	if err := writeTable(workbook, "学科对比", rows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth("学科对比", "A", "L", 16)

	return saveAtomic(workbook, path)
}

// ExportStudentTracking 导出学生历次成绩追踪工作簿。
func ExportStudentTracking(path string, data store.StudentTrackResult) error {
	if path == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建导出目录: %w", err)
	}

	workbook := excel.New()
	defer workbook.Close()

	headerStyle, bodyStyle, err := createStyles(workbook)
	if err != nil {
		return err
	}

	defaultSheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetName(defaultSheet, "学生跟踪"); err != nil {
		return err
	}

	// 汇总所有出现过的学科
	subjectMap := map[string]bool{}
	for _, pt := range data.Points {
		for s := range pt.Scores {
			subjectMap[s] = true
		}
	}
	subjects := make([]string, 0, len(subjectMap))
	for s := range subjectMap {
		subjects = append(subjects, s)
	}
	sort.Strings(subjects)

	headerRow := []any{"考试名称", "考试日期", "学年学期", "总分", "全级名次", "总人数"}
	for _, s := range subjects {
		headerRow = append(headerRow, s)
	}

	rows := [][]any{
		{"学生姓名", data.StudentName},
		{"学号", data.StudentNo},
		{"班级", data.ClassName},
		{"选科方向", data.Track},
		{"导出时间", time.Now().Format("2006-01-02 15:04:05")},
		{},
		headerRow,
	}

	for _, pt := range data.Points {
		row := []any{
			pt.ExamName, pt.ExamDate, pt.AcademicYear + " " + pt.Semester,
			pt.TotalScore, pt.GradeRank, pt.TotalStudents,
		}
		for _, s := range subjects {
			val, ok := pt.Scores[s]
			if ok {
				row = append(row, val)
			} else {
				row = append(row, "-")
			}
		}
		rows = append(rows, row)
	}

	if err := writeTable(workbook, "学生跟踪", rows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth("学生跟踪", "A", "Z", 15)

	return saveAtomic(workbook, path)
}

// ExportAssessmentComparison 导出 2~5 次考核并排对比工作簿。
func ExportAssessmentComparison(path string, data store.AssessmentComparisonResult) error {
	if path == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建导出目录: %w", err)
	}

	workbook := excel.New()
	defer workbook.Close()

	headerStyle, bodyStyle, err := createStyles(workbook)
	if err != nil {
		return err
	}

	defaultSheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetName(defaultSheet, "考核并排对比"); err != nil {
		return err
	}

	headerRow := []any{"考核指标"}
	for _, item := range data.Items {
		headerRow = append(headerRow, item.ExamName)
	}

	diffText := "否"
	if data.PlanDifferent {
		diffText = "是（跨方案版本，综合量化分请以各期单独标准为准）"
	}

	rows := [][]any{
		{"跨方案版本", diffText},
		{"导出时间", time.Now().Format("2006-01-02 15:04:05")},
		{},
		headerRow,
	}

	metricRows := []struct {
		Label string
		Fn    func(item store.AssessmentComparisonItem) any
	}{
		{"考试日期", func(item store.AssessmentComparisonItem) any { return item.ExamDate }},
		{"学年学期", func(item store.AssessmentComparisonItem) any { return item.AcademicYear + " " + item.Semester }},
		{"入学年级", func(item store.AssessmentComparisonItem) any { return fmt.Sprintf("%d级", item.CohortEntryYear) }},
		{"方案版本", func(item store.AssessmentComparisonItem) any { return item.PlanVersionID }},
		{"参考人数", func(item store.AssessmentComparisonItem) any { return item.ParticipantCount }},
		{"参评组数", func(item store.AssessmentComparisonItem) any { return item.GroupCount }},
		{"平均教学总分", func(item store.AssessmentComparisonItem) any { return fmt.Sprintf("%.2f", item.AverageScore) }},
		{"平均班级均分", func(item store.AssessmentComparisonItem) any { return fmt.Sprintf("%.2f", item.AverageClass) }},
		{"平均提升率得分", func(item store.AssessmentComparisonItem) any { return fmt.Sprintf("%.2f", item.AverageImprovement) }},
		{"本科达线总人次", func(item store.AssessmentComparisonItem) any { return item.PassTotal }},
	}

	for _, m := range metricRows {
		r := []any{m.Label}
		for _, item := range data.Items {
			r = append(r, m.Fn(item))
		}
		rows = append(rows, r)
	}

	if err := writeTable(workbook, "考核并排对比", rows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth("考核并排对比", "A", "F", 24)

	return saveAtomic(workbook, path)
}

func createStyles(workbook *excelize.File) (int, int, error) {
	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DDEBF7"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "B4C6E7", Style: 1},
			{Type: "right", Color: "B4C6E7", Style: 1},
			{Type: "top", Color: "B4C6E7", Style: 1},
			{Type: "bottom", Color: "B4C6E7", Style: 1},
		},
	})
	if err != nil {
		return 0, 0, err
	}
	bodyStyle, err := workbook.NewStyle(&excelize.Style{
		Border: []excelize.Border{
			{Type: "left", Color: "D9E2F3", Style: 1},
			{Type: "right", Color: "D9E2F3", Style: 1},
			{Type: "top", Color: "D9E2F3", Style: 1},
			{Type: "bottom", Color: "D9E2F3", Style: 1},
		},
	})
	if err != nil {
		return 0, 0, err
	}
	return headerStyle, bodyStyle, nil
}

func saveAtomic(workbook *excelize.File, path string) error {
	tempPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".tmp.xlsx"
	if err := workbook.SaveAs(tempPath); err != nil {
		return fmt.Errorf("保存导出文件: %w", err)
	}
	if err := workbook.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("替换导出文件: %w", err)
	}
	return nil
}

func boolText(v bool) string {
	if v {
		return "是"
	}
	return "否"
}
