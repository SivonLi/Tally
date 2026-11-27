package exporter

import (
	"encoding/json"
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
		{"考试名称", "考试日期", "班级", "学科", "统计组", "班级均分", "年级均分", "基数均分", "基数比值", "提升率得分", "达线率得分", "完成率得分", "量化总分", "名次", "档次", "赋分"},
	}
	for _, pt := range data.Points {
		for _, r := range pt.Rows {
			detailRows = append(detailRows, []any{
				pt.ExamName, pt.ExamDate, r.LeadClassName, r.Subject, r.StatGroupName,
				r.ClassAverage, r.GradeAverage, r.BaselineAverage, r.BaselineRatio,
				r.ImprovementScore, r.PassRateScore,
				r.CompletionScore, r.Total, r.Rank, r.Tier, r.Points,
			})
		}
	}
	if err := writeTable(workbook, detailSheet, detailRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth(detailSheet, "A", "P", 16)

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
		{"考试名称", "考试日期", "学年学期", "学科", "任课教师", "班级均分", "年级均分", "基数均分", "基数比值", "提升率得分", "达线率得分", "完成率得分", "总分", "名次", "档位", "赋分"},
	}

	for _, pt := range data.Points {
		for _, r := range pt.Rows {
			rows = append(rows, []any{
				pt.ExamName, pt.ExamDate, pt.AcademicYear + " " + pt.Semester,
				r.Subject, strings.Join(r.TeacherNames, "、"),
				r.ClassAverage, r.GradeAverage, r.BaselineAverage, r.BaselineRatio,
				r.ImprovementScore, r.PassRateScore,
				r.CompletionScore, r.Total, r.Rank, r.Tier, r.Points,
			})
		}
	}

	if err := writeTable(workbook, "班级趋势", rows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth("班级趋势", "A", "P", 16)

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
		{"名次", "班级", "任课教师", "统计组", "班级均分", "年级均分", "基数均分", "基数比值", "提升率得分", "达线率得分", "完成率得分", "教学量化总分", "档位", "赋分"},
	}

	for _, g := range data.Groups {
		rows = append(rows, []any{
			g.Rank, g.LeadClassName, strings.Join(g.TeacherNames, "、"), g.StatGroupName,
			g.ClassAverage, g.GradeAverage, g.BaselineAverage, g.BaselineRatio,
			g.ImprovementScore, g.PassRateScore,
			g.CompletionScore, g.Total, g.Tier, g.Points,
		})
	}

	if err := writeTable(workbook, "学科对比", rows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth("学科对比", "A", "N", 16)

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

// StudentProfileExportOptions 配置学生三年档案导出时各类代码字段的可读映射。
type StudentProfileExportOptions struct {
	PhaseLabels       map[string]string
	TrackLabels       map[string]string
	ForeignLangLabels map[string]string
	StatusLabels      map[string]string
	EventTypeLabels   map[string]string
	SubjectLabels     map[string]string
}

var (
	defaultPhaseLabels = map[string]string{
		"g1":        "高一",
		"g2":        "高二",
		"g3":        "高三",
		"gaokao":    "高考",
		"graduated": "已毕业",
	}
	defaultTrackLabels = map[string]string{
		"physics": "物理类",
		"history": "历史类",
	}
	defaultForeignLangLabels = map[string]string{
		"english":  "英语",
		"japanese": "日语",
	}
	defaultStatusLabels = map[string]string{
		"active":      "在读",
		"transferred": "转出",
		"suspended":   "休学",
		"borrowed":    "借读",
		"left_school": "离校",
		"graduated":   "已毕业",
	}
	defaultEventTypeLabels = map[string]string{
		"enrolled":       "入学建档",
		"promoted":       "年级升级",
		"class_changed":  "转班调整",
		"track_changed":  "选科调整",
		"status_changed": "状态变更",
		"merged":         "档案合并",
		"graduated":      "毕业归档",
	}
)

func lookupMappedLabel(code string, custom, defaults map[string]string) string {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return "-"
	}
	key := strings.ToLower(trimmed)
	if custom != nil {
		if label, ok := custom[trimmed]; ok && label != "" {
			return label
		}
		if label, ok := custom[key]; ok && label != "" {
			return label
		}
	}
	if label, ok := defaults[key]; ok && label != "" {
		return label
	}
	return trimmed
}

func (o StudentProfileExportOptions) phaseLabel(code string) string {
	return lookupMappedLabel(code, o.PhaseLabels, defaultPhaseLabels)
}

func (o StudentProfileExportOptions) trackLabel(code string) string {
	return lookupMappedLabel(code, o.TrackLabels, defaultTrackLabels)
}

func (o StudentProfileExportOptions) foreignLangLabel(code string) string {
	return lookupMappedLabel(code, o.ForeignLangLabels, defaultForeignLangLabels)
}

func (o StudentProfileExportOptions) statusLabel(code string) string {
	return lookupMappedLabel(code, o.StatusLabels, defaultStatusLabels)
}

func (o StudentProfileExportOptions) eventTypeLabel(code string) string {
	return lookupMappedLabel(code, o.EventTypeLabels, defaultEventTypeLabels)
}

func (o StudentProfileExportOptions) subjectLabel(code string) string {
	return lookupSubjectLabel(code, o.SubjectLabels)
}

// lookupSubjectLabel 依次尝试调用方提供的名称映射与内置学科名称。
func lookupSubjectLabel(code string, custom map[string]string) string {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return "-"
	}
	if custom != nil {
		if label, ok := custom[trimmed]; ok && label != "" {
			return label
		}
		if label, ok := custom[strings.ToLower(trimmed)]; ok && label != "" {
			return label
		}
	}
	return subjectLabel(trimmed)
}

func dashIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func rankOrDash(rank int) any {
	if rank <= 0 {
		return "-"
	}
	return rank
}

func formatStudentEventPayload(rawJSON string, opts StudentProfileExportOptions) string {
	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" || trimmed == "{}" {
		return "-"
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}
	if len(payload) == 0 {
		return "-"
	}
	strVal := func(key string) string {
		v, ok := payload[key]
		if !ok || v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
	var parts []string
	if v := strVal("phase"); v != "" {
		parts = append(parts, opts.phaseLabel(v))
	}
	if v := strVal("class_name"); v != "" {
		parts = append(parts, v)
	} else if cid := strVal("class_id"); cid != "" && cid != "0" {
		parts = append(parts, "班级#"+cid)
	}
	if v := strVal("track"); v != "" {
		parts = append(parts, opts.trackLabel(v))
	}
	if v := strVal("elective"); v != "" {
		parts = append(parts, v)
	}
	if v := strVal("foreign_lang"); v != "" {
		parts = append(parts, opts.foreignLangLabel(v))
	}
	if v := strVal("status"); v != "" {
		parts = append(parts, opts.statusLabel(v))
	}
	if v := strVal("mode"); v != "" {
		switch v {
		case "direct":
			parts = append(parts, "原班直升")
		case "reclassify":
			parts = append(parts, "选科重分班")
		default:
			parts = append(parts, v)
		}
	}
	if v := strVal("student_no"); v != "" {
		parts = append(parts, "学号 "+v)
	}
	if v := strVal("name"); v != "" {
		parts = append(parts, v)
	}
	if v := strVal("previous_student_id"); v != "" && v != "0" {
		parts = append(parts, "前档 #"+v)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " · ")
	}
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %v", k, payload[k]))
	}
	return strings.Join(parts, " · ")
}

// ExportStudentProfile 导出学生三年完整档案工作簿（基本档案与入学基数、历次考试明细、学籍异动时间线）。
func ExportStudentProfile(path string, profile store.StudentProfile) error {
	return ExportStudentProfileWithOptions(path, profile)
}

// ExportStudentProfileWithOptions 支持传入自定义名称映射，导出可读性更强的学生三年完整档案工作簿。
func ExportStudentProfileWithOptions(path string, profile store.StudentProfile, opts ...StudentProfileExportOptions) error {
	if path == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建导出目录: %w", err)
	}
	var opt StudentProfileExportOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	workbook := excel.New()
	defer workbook.Close()

	headerStyle, bodyStyle, err := createStyles(workbook)
	if err != nil {
		return err
	}

	defaultSheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetName(defaultSheet, "档案概览"); err != nil {
		return err
	}

	entryTotal := "-"
	if profile.Student.TotalScore != nil {
		entryTotal = fmt.Sprintf("%.1f", *profile.Student.TotalScore)
	}
	prevInfo := "无"
	if profile.PreviousStudent != nil && profile.PreviousCohort != nil {
		prevInfo = fmt.Sprintf("%d级 (%s)", profile.PreviousCohort.EnrollmentYear, profile.PreviousStudent.Name)
	}
	overviewRows := [][]any{
		{"项目", "内容"},
		{"姓名", profile.Student.Name},
		{"学号", dashIfEmpty(profile.Student.StudentNo)},
		{"考号", dashIfEmpty(profile.Student.CandidateCode)},
		{"所属年级", fmt.Sprintf("%d级", profile.Cohort.EnrollmentYear)},
		{"当前班级", dashIfEmpty(profile.Student.ClassName)},
		{"选科方向", opt.trackLabel(profile.Student.Track)},
		{"选考组合", dashIfEmpty(profile.Student.Elective)},
		{"外语语种", opt.foreignLangLabel(profile.Student.ForeignLang)},
		{"学籍状态", opt.statusLabel(profile.Student.Status)},
		{"入学总分", entryTotal},
		{"往届前溯档案", prevInfo},
		{"导出时间", time.Now().Format("2006-01-02 15:04:05")},
	}
	if len(profile.EntryScores) > 0 {
		overviewRows = append(overviewRows, []any{}, []any{"入学单科", "分数"})
		for _, es := range profile.EntryScores {
			overviewRows = append(overviewRows, []any{opt.subjectLabel(es.Subject), es.Score})
		}
	}
	if err := writeTable(workbook, "档案概览", overviewRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth("档案概览", "A", "B", 24); err != nil {
		return err
	}

	// Sheet 2: 历次考试明细
	examSheet := "历次考试明细"
	if _, err := workbook.NewSheet(examSheet); err != nil {
		return err
	}
	subjectMap := map[string]bool{}
	for _, pt := range profile.Exams {
		for s := range pt.Scores {
			subjectMap[s] = true
		}
	}
	subjects := make([]string, 0, len(subjectMap))
	for s := range subjectMap {
		subjects = append(subjects, s)
	}
	sort.Strings(subjects)

	examHeader := []any{"阶段", "考试名称", "考试日期", "考时班级", "总分", "年级排名", "年级人数", "类别排名", "类别人数", "班级排名"}
	for _, s := range subjects {
		examHeader = append(examHeader, opt.subjectLabel(s))
	}
	examRows := [][]any{examHeader}
	for _, pt := range profile.Exams {
		row := []any{
			opt.phaseLabel(pt.PhaseAtExam), pt.ExamName, dashIfEmpty(pt.ExamDate), dashIfEmpty(pt.ClassName),
			pt.TotalScore, rankOrDash(pt.GradeRank), rankOrDash(pt.TotalStudents),
			rankOrDash(pt.TrackRank), rankOrDash(pt.TrackTotalCount), rankOrDash(pt.ClassRank),
		}
		for _, s := range subjects {
			if val, ok := pt.Scores[s]; ok {
				row = append(row, val)
			} else {
				row = append(row, "-")
			}
		}
		examRows = append(examRows, row)
	}
	if err := writeTable(workbook, examSheet, examRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth(examSheet, "A", "Z", 15); err != nil {
		return err
	}

	// Sheet 3: 学籍与分班异动
	eventSheet := "学籍异动记录"
	if _, err := workbook.NewSheet(eventSheet); err != nil {
		return err
	}
	eventRows := [][]any{{"发生时间", "阶段", "异动类型", "变更前", "变更后", "备注"}}
	for _, ev := range profile.Events {
		eventRows = append(eventRows, []any{
			ev.OccurredAt.Local().Format("2006-01-02 15:04"),
			opt.phaseLabel(ev.Phase),
			opt.eventTypeLabel(ev.EventType),
			formatStudentEventPayload(ev.FromValueJSON, opt),
			formatStudentEventPayload(ev.ToValueJSON, opt),
			dashIfEmpty(ev.Note),
		})
	}
	if err := writeTable(workbook, eventSheet, eventRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth(eventSheet, "A", "F", 22); err != nil {
		return err
	}

	return saveAtomic(workbook, path)
}

// TeacherProfileExportOptions 配置教师执教档案导出时各类代码字段的可读映射。
type TeacherProfileExportOptions struct {
	PhaseLabels      map[string]string
	StatusLabels     map[string]string
	DutyStatusLabels map[string]string
	RoleLabels       map[string]string
	EndReasonLabels  map[string]string
	SubjectLabels    map[string]string
}

var (
	defaultTeacherStatusLabels = map[string]string{
		"active":        "在职",
		"inactive":      "已停用",
		"idle":          "空闲",
		"active_single": "单届在任",
		"active_multi":  "跨届兼任",
	}
	defaultTeacherDutyStatusLabels = map[string]string{
		"active": "在任",
		"ended":  "已结束",
	}
	defaultTeacherRoleLabels = map[string]string{
		"primary":      "主讲",
		"assistant":    "协助",
		"head_teacher": "班主任",
	}
	defaultTeacherEndReasonLabels = map[string]string{
		"graduated":       "毕业封存",
		"replaced":        "更换任课",
		"reclassified":    "选科重分班",
		"manual_release":  "手动释放",
		"mid_term_change": "期中调整",
	}
	defaultTermLabels = map[string]string{
		"autumn": "第一学期",
		"spring": "第二学期",
	}
	defaultExamTypeLabels = map[string]string{
		"monthly": "月考",
		"midterm": "期中考试",
		"final":   "期末考试",
		"mock":    "模拟考试",
		"gaokao":  "高考",
	}
)

func (o TeacherProfileExportOptions) phaseLabel(code string) string {
	return lookupMappedLabel(code, o.PhaseLabels, defaultPhaseLabels)
}

func (o TeacherProfileExportOptions) statusLabel(code string) string {
	return lookupMappedLabel(code, o.StatusLabels, defaultTeacherStatusLabels)
}

func (o TeacherProfileExportOptions) dutyStatusLabel(code string) string {
	return lookupMappedLabel(code, o.DutyStatusLabels, defaultTeacherDutyStatusLabels)
}

func (o TeacherProfileExportOptions) roleLabel(code string) string {
	return lookupMappedLabel(code, o.RoleLabels, defaultTeacherRoleLabels)
}

func (o TeacherProfileExportOptions) endReasonLabel(code string) string {
	return lookupMappedLabel(code, o.EndReasonLabels, defaultTeacherEndReasonLabels)
}

func (o TeacherProfileExportOptions) subjectLabel(code string) string {
	return lookupSubjectLabel(code, o.SubjectLabels)
}

// teacherTermText 把学年起始年与学期代码组合为「2026-2027 第一学期」。
func teacherTermText(yearStart int, term string) string {
	parts := make([]string, 0, 2)
	if yearStart > 0 {
		parts = append(parts, fmt.Sprintf("%d-%d", yearStart, yearStart+1))
	}
	if label := lookupMappedLabel(term, nil, defaultTermLabels); label != "-" {
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

// teacherExamTypeLabel 把考试类型代码转为可读名称。
func teacherExamTypeLabel(code string) string {
	return lookupMappedLabel(code, nil, defaultExamTypeLabels)
}

// cohortYearText 把入学年份格式化为「2026级」，未知年份显示占位符。
func cohortYearText(year int) string {
	if year <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d级", year)
}

// planVersionCell 仅在有方案版本号时写出数值，否则用占位符。
func planVersionCell(id int64) any {
	if id <= 0 {
		return "-"
	}
	return id
}

func formatScore(value float64) string {
	return fmt.Sprintf("%.1f", value)
}

func formatPointList(points []float64) string {
	if len(points) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(points))
	for _, point := range points {
		parts = append(parts, formatScore(point))
	}
	return strings.Join(parts, "、")
}

func teacherDutyHeader() []any {
	return []any{"届别", "阶段", "班级", "教学班", "学科", "角色", "职责状态", "开始时间", "结束时间", "结束原因"}
}

func teacherDutyRow(asg store.TeacherActiveAssignment, opt TeacherProfileExportOptions) []any {
	return []any{
		cohortYearText(asg.EnrollmentYear),
		opt.phaseLabel(asg.Phase),
		dashIfEmpty(asg.ClassName),
		dashIfEmpty(asg.TeachingGroupName),
		opt.subjectLabel(asg.Subject),
		opt.roleLabel(asg.Role),
		opt.dutyStatusLabel(asg.Status),
		dashIfEmpty(asg.StartedAt),
		dashIfEmpty(asg.EndedAt),
		opt.endReasonLabel(asg.EndReason),
	}
}

// teacherClassResultHeader 是跨届核算明细与考核明细共用的班级结果表头。
func teacherClassResultHeader() []any {
	return []any{
		"届别", "考试名称", "统计组", "班级", "学科", "班级均分", "年级均分", "基数均分", "基数比值",
		"提升率得分", "达线率得分", "完成率得分", "量化总分", "名次", "档次", "赋分",
	}
}

func teacherTrendResultRow(cohortText, examName string, row store.TeacherTrendRow, opt TeacherProfileExportOptions) []any {
	return []any{
		cohortText,
		dashIfEmpty(examName),
		dashIfEmpty(row.StatGroupName),
		dashIfEmpty(row.LeadClassName),
		opt.subjectLabel(row.Subject),
		formatScore(row.ClassAverage),
		formatScore(row.GradeAverage),
		formatScore(row.BaselineAverage),
		fmt.Sprintf("%.2f", row.BaselineRatio),
		formatScore(row.ImprovementScore),
		formatScore(row.PassRateScore),
		formatScore(row.CompletionScore),
		formatScore(row.Total),
		rankOrDash(row.Rank),
		dashIfEmpty(row.Tier),
		formatScore(row.Points),
	}
}

func teacherHistoryResultRow(cohortText, examName string, row store.StoredResultRow, opt TeacherProfileExportOptions) []any {
	return []any{
		cohortText,
		dashIfEmpty(examName),
		dashIfEmpty(row.StatGroupName),
		dashIfEmpty(row.LeadClassName),
		opt.subjectLabel(row.Subject),
		formatScore(row.ClassAverage),
		formatScore(row.GradeAverage),
		formatScore(row.BaselineAverage),
		fmt.Sprintf("%.2f", row.BaselineRatio),
		formatScore(row.ImprovementScore),
		formatScore(row.PassScore),
		formatScore(row.CompletionScore),
		formatScore(row.Total),
		rankOrDash(row.Rank),
		levelLabel(row.TierIndex),
		formatScore(row.Points),
	}
}

// teacherReleaseText 组合最近一次结束任教的届别、班级、学科与释放原因。
func teacherReleaseText(release *store.TeacherLastRelease, opt TeacherProfileExportOptions) string {
	if release == nil {
		return "-"
	}
	parts := make([]string, 0, 6)
	if label := cohortYearText(release.EnrollmentYear); label != "-" {
		parts = append(parts, label)
	}
	if value := strings.TrimSpace(release.ClassName); value != "" {
		parts = append(parts, value)
	}
	if label := opt.subjectLabel(release.Subject); label != "-" {
		parts = append(parts, label)
	}
	if label := opt.phaseLabel(release.Phase); label != "-" {
		parts = append(parts, label)
	}
	if label := opt.endReasonLabel(release.EndReason); label != "-" {
		parts = append(parts, label)
	}
	if !release.EndedAt.IsZero() {
		parts = append(parts, release.EndedAt.Local().Format("2006-01-02 15:04"))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " · ")
}

// teacherActiveCohortText 汇总在任职责涉及的届别。
func teacherActiveCohortText(assignments []store.TeacherActiveAssignment) string {
	seen := map[int]bool{}
	labels := make([]string, 0, len(assignments))
	for _, asg := range assignments {
		if asg.EnrollmentYear <= 0 || seen[asg.EnrollmentYear] {
			continue
		}
		seen[asg.EnrollmentYear] = true
		labels = append(labels, cohortYearText(asg.EnrollmentYear))
	}
	if len(labels) == 0 {
		return "-"
	}
	return strings.Join(labels, "、")
}

// teacherAcademicTerm 优先使用跨届得分记录里已可读的学年学期，缺失时由起始年与学期代码组合。
func teacherAcademicTerm(academicYear, semester string, yearStart int, term string) string {
	text := strings.TrimSpace(strings.TrimSpace(academicYear) + " " + strings.TrimSpace(semester))
	if text != "" {
		return text
	}
	return teacherTermText(yearStart, term)
}

// ExportTeacherProfile 导出教师执教档案工作簿（基本档案与实时占用、跨届个人得分、按届带班历史、考核明细）。
func ExportTeacherProfile(path string, profile store.TeacherProfile) error {
	return ExportTeacherProfileWithOptions(path, profile)
}

// ExportTeacherProfileWithOptions 支持传入自定义名称映射，导出可读性更强的教师执教档案工作簿。
func ExportTeacherProfileWithOptions(path string, profile store.TeacherProfile, opts ...TeacherProfileExportOptions) error {
	if path == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建导出目录: %w", err)
	}
	var opt TeacherProfileExportOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	workbook := excel.New()
	defer workbook.Close()

	headerStyle, bodyStyle, err := createStyles(workbook)
	if err != nil {
		return err
	}

	defaultSheet := workbook.GetSheetName(0)
	if err := workbook.SetSheetName(defaultSheet, "档案概览"); err != nil {
		return err
	}

	teacher := profile.Teacher
	occ := profile.Occupancy
	primarySubject := strings.TrimSpace(teacher.Subject)
	if primarySubject == "" {
		primarySubject = occ.PrimarySubject
	}

	// Sheet 1: 基本档案与实时占用
	overviewRows := [][]any{
		{"项目", "内容"},
		{"工号", dashIfEmpty(teacher.TeacherNo)},
		{"姓名", dashIfEmpty(teacher.Name)},
		{"主教学科", opt.subjectLabel(primarySubject)},
		{"备注", dashIfEmpty(teacher.Note)},
		{"教师状态", opt.statusLabel(teacher.Status)},
		{"占用状态", opt.statusLabel(occ.Status)},
		{"在任届别", teacherActiveCohortText(occ.ActiveAssignments)},
		{"在任职责数", len(occ.ActiveAssignments)},
		{"累计执教届数", len(profile.CohortHistories)},
		{"累计考核次数", len(profile.CrossCohortScores)},
		{"最近释放", teacherReleaseText(occ.LastRelease, opt)},
		{"导出时间", time.Now().Format("2006-01-02 15:04:05")},
	}
	if len(occ.ActiveAssignments) > 0 {
		overviewRows = append(overviewRows, []any{}, teacherDutyHeader())
		for _, asg := range occ.ActiveAssignments {
			overviewRows = append(overviewRows, teacherDutyRow(asg, opt))
		}
	}
	if err := writeTable(workbook, "档案概览", overviewRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth("档案概览", "A", "J", 18); err != nil {
		return err
	}

	// 跨届得分记录按考核 ID 关联所属届别，与档案页趋势图使用同一关联方式。
	yearByAssessment := map[int64]int{}
	for _, ch := range profile.CohortHistories {
		for _, ex := range ch.Exams {
			yearByAssessment[ex.AssessmentID] = ch.EnrollmentYear
		}
	}

	// Sheet 2: 跨届个人得分趋势与统计组明细
	trendSheet := "跨届得分趋势"
	if _, err := workbook.NewSheet(trendSheet); err != nil {
		return err
	}
	trendRows := [][]any{
		{"届别", "阶段", "考试名称", "考试日期", "学年学期", "学科", "方案版本", "个人得分", "统计组数", "统计组得分"},
	}
	for _, pt := range profile.CrossCohortScores {
		trendRows = append(trendRows, []any{
			cohortYearText(yearByAssessment[pt.AssessmentID]),
			opt.phaseLabel(pt.GradeLevel),
			dashIfEmpty(pt.ExamName),
			dashIfEmpty(pt.ExamDate),
			teacherAcademicTerm(pt.AcademicYear, pt.Semester, pt.AcademicYearStart, pt.Term),
			opt.subjectLabel(pt.Subject),
			planVersionCell(pt.PlanVersionID),
			formatScore(pt.PersonalScore),
			len(pt.Rows),
			formatPointList(pt.GroupPoints),
		})
	}
	for _, pt := range profile.CrossCohortScores {
		if len(pt.Rows) == 0 {
			continue
		}
		trendRows = append(trendRows, []any{}, teacherClassResultHeader())
		cohortText := cohortYearText(yearByAssessment[pt.AssessmentID])
		for _, row := range pt.Rows {
			trendRows = append(trendRows, teacherTrendResultRow(cohortText, pt.ExamName, row, opt))
		}
	}
	if err := writeTable(workbook, trendSheet, trendRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth(trendSheet, "A", "P", 16); err != nil {
		return err
	}

	// Sheet 3: 按届带班历史
	historySheet := "按届带班历史"
	if _, err := workbook.NewSheet(historySheet); err != nil {
		return err
	}
	historyRows := [][]any{{"届别", "毕业年份", "当前阶段", "任课记录数", "考核记录数"}}
	for _, ch := range profile.CohortHistories {
		graduationYear := any("-")
		if ch.GraduationYear > 0 {
			graduationYear = ch.GraduationYear
		}
		historyRows = append(historyRows, []any{
			cohortYearText(ch.EnrollmentYear),
			graduationYear,
			opt.phaseLabel(ch.CurrentPhase),
			len(ch.Assignments),
			len(ch.Exams),
		})
	}
	for _, ch := range profile.CohortHistories {
		if len(ch.Assignments) == 0 {
			continue
		}
		historyRows = append(historyRows, []any{}, teacherDutyHeader())
		for _, asg := range ch.Assignments {
			historyRows = append(historyRows, teacherDutyRow(asg, opt))
		}
	}
	if err := writeTable(workbook, historySheet, historyRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth(historySheet, "A", "J", 18); err != nil {
		return err
	}

	// Sheet 4: 考核明细，包含每场考试的班级结果行
	detailSheet := "考核明细"
	if _, err := workbook.NewSheet(detailSheet); err != nil {
		return err
	}
	detailRows := [][]any{
		{"届别", "阶段", "考试名称", "考试类型", "考试日期", "学年学期", "学科", "个人得分", "统计组数", "统计组得分"},
	}
	for _, ch := range profile.CohortHistories {
		for _, ex := range ch.Exams {
			detailRows = append(detailRows, []any{
				cohortYearText(ch.EnrollmentYear),
				opt.phaseLabel(ex.PhaseAtExam),
				dashIfEmpty(ex.ExamName),
				teacherExamTypeLabel(ex.ExamType),
				dashIfEmpty(ex.ExamDate),
				teacherTermText(ex.AcademicYearStart, ex.Term),
				opt.subjectLabel(ex.Subject),
				formatScore(ex.Score),
				len(ex.ClassRows),
				formatPointList(ex.GroupPoints),
			})
		}
	}
	for _, ch := range profile.CohortHistories {
		for _, ex := range ch.Exams {
			if len(ex.ClassRows) == 0 {
				continue
			}
			detailRows = append(detailRows, []any{}, teacherClassResultHeader())
			cohortText := cohortYearText(ch.EnrollmentYear)
			for _, row := range ex.ClassRows {
				detailRows = append(detailRows, teacherHistoryResultRow(cohortText, ex.ExamName, row, opt))
			}
		}
	}
	if err := writeTable(workbook, detailSheet, detailRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	if err := workbook.SetColWidth(detailSheet, "A", "P", 16); err != nil {
		return err
	}

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

// ExportExamScores 导出单场考试的成绩明细、总分排名及班级学科均分工作簿。
func ExportExamScores(path string, exam store.Exam, ec store.ExamCohort, roster []store.ExamRosterEntry, scores []store.ExamScoreRow, avgs []store.ExamClassAverage) error {
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
	if err := workbook.SetSheetName(defaultSheet, "总分与排名"); err != nil {
		return err
	}
	rosterRows := [][]any{
		{"考试名称", exam.Name, "届别", ec.CohortName, "阶段", ec.GradeLevelAtExam},
		{},
		{"学号", "考号", "姓名", "班级", "选科", "总分", "年排", "类排", "班排"},
	}
	for _, r := range roster {
		rosterRows = append(rosterRows, []any{
			r.StudentNo, r.CandidateNo, r.Name, r.ClassName, r.Track,
			exportOptFloat(r.TotalScore), exportOptInt(r.GradeRank), exportOptInt(r.TrackRank), exportOptInt(r.ClassRank),
		})
	}
	if err := writeTable(workbook, "总分与排名", rosterRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth("总分与排名", "A", "I", 16)

	scoreSheet := "单科成绩明细"
	_, _ = workbook.NewSheet(scoreSheet)
	scoreRows := [][]any{
		{"学号", "姓名", "班级", "学科", "成绩", "标记", "科年排", "科班排"},
	}
	for _, s := range scores {
		scoreRows = append(scoreRows, []any{
			s.StudentNo, s.Name, s.ClassName, s.Subject,
			exportOptFloat(s.Score), s.Flag, exportOptInt(s.SubjectGradeRank), exportOptInt(s.SubjectClassRank),
		})
	}
	if err := writeTable(workbook, scoreSheet, scoreRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth(scoreSheet, "A", "H", 16)

	avgSheet := "班级均分汇总"
	_, _ = workbook.NewSheet(avgSheet)
	avgRows := [][]any{
		{"学科", "班级", "有效人数", "班级均分"},
	}
	for _, avg := range avgs {
		avgRows = append(avgRows, []any{
			avg.Subject, avg.ClassName, avg.Count, fmt.Sprintf("%.2f", avg.Average),
		})
	}
	if err := writeTable(workbook, avgSheet, avgRows, headerStyle, bodyStyle); err != nil {
		return err
	}
	_ = workbook.SetColWidth(avgSheet, "A", "D", 18)

	return saveAtomic(workbook, path)
}

func exportOptFloat(v *float64) any {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.1f", *v)
}

func exportOptInt(v *int) any {
	if v == nil {
		return ""
	}
	return *v
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
