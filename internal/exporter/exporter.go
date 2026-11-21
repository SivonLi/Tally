// Package exporter 把核算结果写成学校可以核对的 xlsx 工作簿。
package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"tally/internal/excel"
	"tally/internal/store"
)

// Input 是一次导出需要的全部数据。
type Input struct {
	Path        string
	Batch       store.Batch
	Plan        store.PlanVersion
	Run         store.ResultRun
	Rows        []store.StoredResultRow
	Personal    []store.StoredPersonalScore
	GeneratedAt time.Time
}

// Write 写出结果工作簿，先写临时文件再替换目标文件。
func Write(input Input) error {
	if input.Path == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(input.Path), 0o755); err != nil {
		return fmt.Errorf("创建导出目录: %w", err)
	}
	workbook := excel.New()
	defer workbook.Close()
	defaultSheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetName(defaultSheet, "计算依据"); err != nil {
		return err
	}
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
		return err
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
		return err
	}
	generatedAt := input.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	typeText := map[string]string{"midterm": "期中", "final": "期末", "gaokao": "高考"}
	gradeText := map[string]string{"g1": "高一", "g2": "高二", "g3": "高三"}
	baseRows := [][]any{
		{"项目", "值"},
		{"学年学期", input.Batch.AcademicYear + " " + input.Batch.Semester},
		{"考试类型", typeText[input.Batch.ExamType]},
		{"年级", gradeText[input.Batch.Grade]},
		{"考核方案", input.Plan.Config.Name},
		{"方案版本", strconv.Itoa(input.Plan.Version)},
		{"方案版本标识", strconv.FormatInt(input.Plan.ID, 10)},
		{"输入摘要", input.Run.InputHash},
		{"核算时间", input.Run.CreatedAt.Local().Format("2006-01-02 15:04:05")},
		{"导出时间", generatedAt.Local().Format("2006-01-02 15:04:05")},
	}
	if err := writeTable(workbook, "计算依据", baseRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth("计算依据", "A", "A", 18); err != nil {
		return err
	}
	if err := workbook.SetColWidth("计算依据", "B", "B", 76); err != nil {
		return err
	}
	quantRows := [][]any{{
		"班级", "学科", "统计组", "任课教师", "参考人数", "班级均分", "年级均分", "基数均分",
		"基数比值", "均分提升率得分", "达线人数", "单科目标", "单科达线率得分", "完成率得分", "教学量化成绩",
	}}
	rankRows := [][]any{{"班级", "学科", "统计组", "任课教师", "教学量化成绩", "排名", "档次", "赋分"}}
	detailRows := [][]any{{"班级", "学科", "统计组", "指标明细"}}
	for _, row := range input.Rows {
		teachers := decodeNames(row.TeacherNames)
		quantRows = append(quantRows, []any{
			row.LeadClassName, subjectLabel(row.Subject), row.StatGroupName, teachers, row.ClassCount,
			row.ClassAverage, row.GradeAverage, row.BaselineAverage, row.BaselineRatio,
			row.ImprovementScore, row.PassCount, row.PassTarget, row.PassScore, row.CompletionScore, row.Total,
		})
		rankRows = append(rankRows, []any{
			row.LeadClassName, subjectLabel(row.Subject), row.StatGroupName, teachers, row.Total,
			row.Rank, levelLabel(row.TierIndex), row.Points,
		})
		detailRows = append(detailRows, []any{row.LeadClassName, subjectLabel(row.Subject), row.StatGroupName, row.IndicatorJSON})
	}
	if err := addSheet(workbook, "量化计算", quantRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := addSheet(workbook, "排名赋分", rankRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	personalRows := [][]any{{"教师", "年级", "学科", "任教统计组赋分", "个人得分"}}
	for _, score := range input.Personal {
		personalRows = append(personalRows, []any{
			score.TeacherName, gradeText[score.Grade], subjectLabel(score.Subject), decodePoints(score.GroupPoints), score.Score,
		})
	}
	if err := addSheet(workbook, "个人得分", personalRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := addSheet(workbook, "结果明细", detailRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth("量化计算", "A", "O", 14); err != nil {
		return err
	}
	if err := workbook.SetColWidth("量化计算", "D", "D", 22); err != nil {
		return err
	}
	if err := workbook.SetColWidth("排名赋分", "A", "H", 14); err != nil {
		return err
	}
	if err := workbook.SetColWidth("个人得分", "A", "E", 16); err != nil {
		return err
	}
	if err := workbook.SetColWidth("结果明细", "A", "D", 18); err != nil {
		return err
	}
	if err := workbook.SetColWidth("结果明细", "D", "D", 96); err != nil {
		return err
	}
	for _, sheet := range []string{"量化计算", "排名赋分", "个人得分", "结果明细"} {
		if err := workbook.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
			return err
		}
	}
	tempPath := strings.TrimSuffix(input.Path, filepath.Ext(input.Path)) + ".tmp.xlsx"
	if err := workbook.SaveAs(tempPath); err != nil {
		return fmt.Errorf("保存导出文件: %w", err)
	}
	if err := workbook.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, input.Path); err != nil {
		return fmt.Errorf("替换导出文件: %w", err)
	}
	return nil
}

func addSheet(workbook *excelize.File, name string, rows [][]any, headerStyle, bodyStyle int) error {
	if _, err := workbook.NewSheet(name); err != nil {
		return err
	}
	return writeTable(workbook, name, rows, headerStyle, bodyStyle)
}

func writeTable(workbook *excelize.File, sheet string, rows [][]any, headerStyle, bodyStyle int) error {
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if err != nil {
				return err
			}
			if err := workbook.SetCellValue(sheet, cell, value); err != nil {
				return err
			}
			style := bodyStyle
			if rowIndex == 0 {
				style = headerStyle
			}
			if err := workbook.SetCellStyle(sheet, cell, cell, style); err != nil {
				return err
			}
		}
	}
	return nil
}

func subjectLabel(code string) string {
	labels := map[string]string{
		"chinese": "语文", "math": "数学", "english": "英语", "physics": "物理",
		"chemistry": "化学", "biology": "生物", "history": "历史", "geography": "地理",
		"politics": "政治", "japanese": "日语",
	}
	if label, ok := labels[code]; ok {
		return label
	}
	return code
}

func levelLabel(index int) string {
	switch index {
	case 0:
		return "第一档"
	case 1:
		return "第二档"
	case 2:
		return "第三档"
	default:
		return "第" + strconv.Itoa(index+1) + "档"
	}
}

func decodeNames(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var names []string
	if err := json.Unmarshal([]byte(value), &names); err != nil {
		return value
	}
	return strings.Join(names, "、")
}

func decodePoints(value string) string {
	var points []float64
	if err := json.Unmarshal([]byte(value), &points); err != nil {
		return value
	}
	parts := make([]string, 0, len(points))
	for _, point := range points {
		parts = append(parts, strconv.FormatFloat(point, 'f', -1, 64))
	}
	return strings.Join(parts, "、")
}
