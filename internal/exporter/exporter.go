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
	Students    []store.Student
	Classes     []store.Class
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
	var overrideBoundaries []store.ResultRunBoundary
	for _, b := range input.Run.Boundaries {
		if b.Source == "override" {
			overrideBoundaries = append(overrideBoundaries, b)
		}
	}
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
	if len(overrideBoundaries) == 0 {
		baseRows = append(baseRows, []any{"分档边界", "按方案默认边界"})
	}
	if err := writeTable(workbook, "计算依据", baseRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if len(overrideBoundaries) > 0 {
		boundaryRows := [][]any{{"学科", "参评组数", "一档止于", "二档止于", "调整备注"}}
		for _, b := range overrideBoundaries {
			boundaryRows = append(boundaryRows, []any{
				subjectLabel(b.Subject), b.GroupCount, b.FirstRank, b.SecondRank, b.Reason,
			})
		}
		if err := writeTableAt(workbook, "计算依据", len(baseRows)+2, boundaryRows, headerStyle, bodyStyle); err != nil {
			return err
		}
		for _, col := range []struct {
			name  string
			width float64
		}{
			{"C", 14}, {"D", 14}, {"E", 28},
		} {
			if err := workbook.SetColWidth("计算依据", col.name, col.name, col.width); err != nil {
				return err
			}
		}
	}
	if err := workbook.SetColWidth("计算依据", "A", "A", 18); err != nil {
		return err
	}
	if err := workbook.SetColWidth("计算依据", "B", "B", 76); err != nil {
		return err
	}

	subjects := distinctSubjects(input.Rows)

	// 1. 班级汇总
	classSummaryRows := buildClassSummaryRows(input.Rows, subjects)
	if err := addSheet(workbook, "班级汇总", classSummaryRows, headerStyle, bodyStyle); err != nil {
		return err
	}

	// 2. 量化计算
	quantRows := [][]any{{
		"班级", "学科", "统计组", "任课教师", "参考人数", "班级均分", "年级均分", "基数均分",
		"基数比值", "均分提升率得分", "达线人数", "单科目标", "单科达线率得分", "完成率得分", "教学量化成绩",
	}}
	// 3. 排名赋分
	rankRows := [][]any{{"班级", "学科", "统计组", "任课教师", "教学量化成绩", "排名", "档次", "赋分"}}
	// 4. 均分提升率
	improvementRows := [][]any{{
		"班级", "学科", "统计组", "任课教师", "参考人数", "班级均分", "年级均分", "基数均分", "基数比值", "提升率未封顶值", "均分提升率得分",
	}}
	// 5. 达线率
	passRows := [][]any{{
		"班级", "学科", "统计组", "任课教师", "参考人数", "达线人数", "单科目标", "达线率未封顶值", "单科达线率得分",
	}}
	// 6. 结果明细
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
		improvementRows = append(improvementRows, []any{
			row.LeadClassName, subjectLabel(row.Subject), row.StatGroupName, teachers, row.ClassCount,
			row.ClassAverage, row.GradeAverage, row.BaselineAverage, row.BaselineRatio,
			row.ImprovementRaw, row.ImprovementScore,
		})
		passRows = append(passRows, []any{
			row.LeadClassName, subjectLabel(row.Subject), row.StatGroupName, teachers, row.ClassCount,
			row.PassCount, row.PassTarget, row.PassRaw, row.PassScore,
		})
		detailRows = append(detailRows, []any{row.LeadClassName, subjectLabel(row.Subject), row.StatGroupName, row.IndicatorJSON})
	}
	if err := addSheet(workbook, "量化计算", quantRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := addSheet(workbook, "排名赋分", rankRows, headerStyle, bodyStyle); err != nil {
		return err
	}

	// 7. 个人得分
	personalRows := [][]any{{"教师", "年级", "学科", "任教统计组赋分", "个人得分"}}
	for _, score := range input.Personal {
		personalRows = append(personalRows, []any{
			score.TeacherName, gradeText[score.Grade], subjectLabel(score.Subject), decodePoints(score.GroupPoints), score.Score,
		})
	}
	if err := addSheet(workbook, "个人得分", personalRows, headerStyle, bodyStyle); err != nil {
		return err
	}

	if err := addSheet(workbook, "均分提升率", improvementRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := addSheet(workbook, "达线率", passRows, headerStyle, bodyStyle); err != nil {
		return err
	}

	// 8. 完成率
	completionRows := buildCompletionRows(input.Rows)
	if err := addSheet(workbook, "完成率", completionRows, headerStyle, bodyStyle); err != nil {
		return err
	}

	// 9. 基础数据 (可选)
	if len(input.Students) > 0 {
		rosterRows := buildStudentRosterRows(input.Students, input.Classes, subjects)
		if err := addSheet(workbook, "基础数据", rosterRows, headerStyle, bodyStyle); err != nil {
			return err
		}
	}

	// 10. 结果明细
	if err := addSheet(workbook, "结果明细", detailRows, headerStyle, bodyStyle); err != nil {
		return err
	}

	// 设置列宽
	_ = workbook.SetColWidth("班级汇总", "A", "Z", 16)
	_ = workbook.SetColWidth("量化计算", "A", "O", 14)
	_ = workbook.SetColWidth("量化计算", "D", "D", 22)
	_ = workbook.SetColWidth("排名赋分", "A", "H", 14)
	_ = workbook.SetColWidth("排名赋分", "D", "D", 22)
	_ = workbook.SetColWidth("个人得分", "A", "E", 16)
	_ = workbook.SetColWidth("个人得分", "D", "D", 24)
	_ = workbook.SetColWidth("均分提升率", "A", "K", 14)
	_ = workbook.SetColWidth("均分提升率", "D", "D", 22)
	_ = workbook.SetColWidth("达线率", "A", "I", 14)
	_ = workbook.SetColWidth("达线率", "D", "D", 22)
	_ = workbook.SetColWidth("完成率", "A", "E", 16)
	if len(input.Students) > 0 {
		_ = workbook.SetColWidth("基础数据", "A", "Z", 14)
		_ = workbook.SetColWidth("基础数据", "B", "B", 12)
	}
	_ = workbook.SetColWidth("结果明细", "A", "C", 18)
	_ = workbook.SetColWidth("结果明细", "D", "D", 96)

	freezeSheets := []string{"班级汇总", "量化计算", "排名赋分", "个人得分", "均分提升率", "达线率", "完成率", "结果明细"}
	if len(input.Students) > 0 {
		freezeSheets = append(freezeSheets, "基础数据")
	}
	for _, sheet := range freezeSheets {
		_ = workbook.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
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

func distinctSubjects(rows []store.StoredResultRow) []string {
	order := []string{"chinese", "math", "english", "physics", "chemistry", "biology", "history", "geography", "politics"}
	seen := map[string]bool{}
	present := map[string]bool{}
	for _, row := range rows {
		present[row.Subject] = true
	}
	var res []string
	for _, s := range order {
		if present[s] && !seen[s] {
			seen[s] = true
			res = append(res, s)
		}
	}
	for _, row := range rows {
		if !seen[row.Subject] {
			seen[row.Subject] = true
			res = append(res, row.Subject)
		}
	}
	return res
}

type classExportSummary struct {
	ClassName       string
	CompletionScore float64
	HasCompletion   bool
	SubjectScores   map[string]float64
	SubjectTeachers map[string]string
	Total           float64
}

func buildClassSummaryRows(rows []store.StoredResultRow, subjects []string) [][]any {
	header := []any{"班级", "完成率得分"}
	for _, s := range subjects {
		header = append(header, subjectLabel(s)+"量化成绩", subjectLabel(s)+"任课教师")
	}
	header = append(header, "班级总分")

	classOrder := make([]string, 0)
	classMap := make(map[string]*classExportSummary)
	for _, row := range rows {
		item, exists := classMap[row.LeadClassName]
		if !exists {
			item = &classExportSummary{
				ClassName:       row.LeadClassName,
				SubjectScores:   make(map[string]float64),
				SubjectTeachers: make(map[string]string),
			}
			classMap[row.LeadClassName] = item
			classOrder = append(classOrder, row.LeadClassName)
		}
		if row.CompletionParticipate {
			item.CompletionScore = row.CompletionScore
			item.HasCompletion = true
		}
		item.SubjectScores[row.Subject] = row.Total
		item.SubjectTeachers[row.Subject] = decodeNames(row.TeacherNames)
	}

	result := [][]any{header}
	for _, name := range classOrder {
		item := classMap[name]
		total := 0.0
		var completionVal any = "-"
		if item.HasCompletion {
			total += item.CompletionScore
			completionVal = item.CompletionScore
		}
		row := []any{item.ClassName, completionVal}
		for _, s := range subjects {
			var scoreVal any = "-"
			if sc, ok := item.SubjectScores[s]; ok {
				scoreVal = sc
				total += sc
			}
			teacherVal := item.SubjectTeachers[s]
			row = append(row, scoreVal, teacherVal)
		}
		row = append(row, total)
		result = append(result, row)
	}
	return result
}

func buildCompletionRows(rows []store.StoredResultRow) [][]any {
	header := []any{"班级", "实际完成分", "目标基础分", "完成率未封顶值", "完成率得分"}
	result := [][]any{header}
	seen := map[string]bool{}
	for _, row := range rows {
		if !row.CompletionParticipate || seen[row.LeadClassName] {
			continue
		}
		seen[row.LeadClassName] = true
		result = append(result, []any{
			row.LeadClassName, row.CompletionActual, row.CompletionBase, row.CompletionRaw, row.CompletionScore,
		})
	}
	return result
}

func buildStudentRosterRows(students []store.Student, classes []store.Class, subjects []string) [][]any {
	classMap := map[int64]string{}
	for _, c := range classes {
		classMap[c.ID] = c.Name
	}
	trackNames := map[string]string{
		"physics": "物理类", "history": "历史类", "art": "艺体类", "comprehensive": "综合",
	}

	header := []any{"考号", "姓名", "班级", "选科类别"}
	for _, s := range subjects {
		header = append(header, subjectLabel(s))
	}
	header = append(header, "总分")
	result := [][]any{header}

	for _, st := range students {
		cName := classMap[st.ClassID]
		if cName == "" {
			cName = strconv.FormatInt(st.ClassID, 10)
		}
		track := trackNames[st.Track]
		if track == "" {
			track = st.Track
		}
		row := []any{st.StudentNo, st.Name, cName, track}
		for _, s := range subjects {
			if sc, ok := st.Scores[s]; ok {
				row = append(row, sc)
			} else {
				row = append(row, "")
			}
		}
		if st.TotalScore != nil {
			row = append(row, *st.TotalScore)
		} else {
			row = append(row, "")
		}
		result = append(result, row)
	}
	return result
}

func addSheet(workbook *excelize.File, name string, rows [][]any, headerStyle, bodyStyle int) error {
	if _, err := workbook.NewSheet(name); err != nil {
		return err
	}
	return writeTable(workbook, name, rows, headerStyle, bodyStyle)
}

func writeTable(workbook *excelize.File, sheet string, rows [][]any, headerStyle, bodyStyle int) error {
	return writeTableAt(workbook, sheet, 1, rows, headerStyle, bodyStyle)
}

func writeTableAt(workbook *excelize.File, sheet string, startRow int, rows [][]any, headerStyle, bodyStyle int) error {
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, startRow+rowIndex)
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
