package importer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"tally/internal/assessment"
	"tally/internal/excel"
	"tally/internal/store"
)

// 可引导导入的数据项。
const (
	KindClasses     = "classes"
	KindStudents    = "students"
	KindTeachers    = "teachers"
	KindAssignments = "assignments"
	KindBaselines   = "baselines"
	KindRoster      = "roster"
	KindScores      = "scores"
	KindCutoffs     = "cutoffs"
	KindTargets     = "targets"
)

// 导入向导使用的标准字段。
const (
	FieldClassName     = "class_name"
	FieldStudentNo     = "student_no"
	FieldName          = "name"
	FieldTrack         = "track"
	FieldElective      = "elective"
	FieldStatus        = "status"
	FieldTotalScore    = "total_score"
	FieldWalking       = "walking"
	FieldJapanese      = "japanese"
	FieldTeacherName   = "teacher_name"
	FieldTeacherNo     = "teacher_no"
	FieldSubject       = "subject"
	FieldExamNo        = "exam_no"
	FieldCandidateNo   = "candidate_no"
	FieldCutoffSubject = "cutoff_subject"
	FieldCutoffTrack   = "cutoff_track"
	FieldCutoffTier    = "cutoff_tier"
	FieldCutoffValue   = "cutoff_value"
)

// Workbook 是已经读取到内存的工作簿。
type Workbook struct {
	Path          string
	FileHash      string
	Format        string
	ConvertedPath string
	Conversion    string
	Tables        []excel.Table
}

// Field 是导入向导要求用户映射的一个标准字段。
type Field struct {
	Key      string
	Required bool
}

// InspectWorkbook 读取工作簿的工作表、表头和预览行。
func InspectWorkbook(path string, options Options) (*Workbook, error) {
	report := store.ImportReport{
		Path:   path,
		Format: strings.TrimPrefix(strings.ToLower(extension(path)), "."),
	}
	hash, err := fileHash(path)
	if err != nil {
		return nil, err
	}
	report.FileHash = hash
	readPath := path
	if strings.EqualFold(extension(path), ".xls") {
		dir := options.ConversionDir
		if dir == "" {
			return nil, fmt.Errorf("导入旧版文件需要转换目录")
		}
		converted, err := (excel.Converter{}).Convert(path, dir)
		if err != nil {
			return nil, err
		}
		readPath = converted
		report.ConvertedPath = converted
		report.Format = "xls"
		report.Conversion = "libreoffice"
	}
	workbook, err := excel.Open(readPath)
	if err != nil {
		return nil, err
	}
	defer workbook.Close()
	tables, err := readTables(workbook)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("工作簿 %s 没有工作表", path)
	}
	return &Workbook{
		Path:          path,
		FileHash:      hash,
		Format:        report.Format,
		ConvertedPath: report.ConvertedPath,
		Conversion:    report.Conversion,
		Tables:        tables,
	}, nil
}

func extension(path string) string {
	index := strings.LastIndex(path, ".")
	if index < 0 {
		return ""
	}
	return path[index:]
}

// SheetNames 返回全部工作表名称。
func (w *Workbook) SheetNames() []string {
	names := make([]string, 0, len(w.Tables))
	for _, table := range w.Tables {
		names = append(names, table.Sheet)
	}
	return names
}

// Sheet 按名称读取一个工作表。
func (w *Workbook) Sheet(name string) (excel.Table, bool) {
	for _, table := range w.Tables {
		if table.Sheet == name {
			return table, true
		}
	}
	return excel.Table{}, false
}

// DetectSheet 按数据项判断工作表的匹配程度，返回 0 表示不匹配。
func DetectSheet(kind string, table excel.Table) int {
	headers := table.Headers
	score := 0
	switch kind {
	case KindClasses:
		if findHeader(headers, "班级名称", "班级", "行政班") >= 0 && findHeader(headers, "姓名", "学号", "考号") < 0 {
			score = 20
		}
	case KindStudents:
		if findHeader(headers, "学号", "考号", "考生号", "学籍号") >= 0 && findHeader(headers, "姓名") >= 0 {
			score = 20
		}
	case KindTeachers:
		if findHeader(headers, "教师姓名", "任课教师", "教师") >= 0 &&
			findHeader(headers, "学科", "科目", "工号", "教师工号") >= 0 &&
			findHeader(headers, "班级", "行政班") < 0 {
			score = 20
		} else if wideAssignmentTable(table) {
			score = 20
		}
	case KindAssignments:
		if findHeader(headers, "教师姓名", "任课教师", "教师") >= 0 &&
			findHeader(headers, "学科", "科目") >= 0 &&
			findHeader(headers, "班级", "行政班") >= 0 {
			score = 20
		} else if wideAssignmentTable(table) {
			score = 20
		}
	case KindBaselines:
		if findHeader(headers, "班级", "行政班") >= 0 && hasBaselineSubject(headers) {
			score = 20
		}
	case KindRoster:
		if findHeader(headers, "学号", "考号", "考生号", "学籍号") >= 0 && findHeader(headers, "姓名") >= 0 &&
			findHeader(headers, "班级", "行政班") >= 0 {
			score = 20
		}
	case KindScores:
		if findHeader(headers, "考号", "学号", "考生号", "学籍号") >= 0 && findHeader(headers, "姓名") >= 0 &&
			countSubjectHeaders(headers) >= 2 {
			score = 20
		}
	case KindCutoffs:
		if findHeader(headers, "科目") >= 0 && findHeader(headers, "本科线", "特控线", "分数线", "总分线") >= 0 {
			score = 20
		}
	case KindTargets:
		if findHeader(headers, "班级", "行政班") >= 0 && findHeader(headers, "档位", "清北", "C9", "985", "211", "特控", "本科") >= 0 {
			score = 20
		}
	}
	if score == 0 {
		return 0
	}
	switch kind {
	case KindScores, KindRoster:
		// 完整名单行数最多，用实际行数区分按班级拆分的表。
		score += len(table.Rows)
	default:
		score += rowScore(table.Rows)
	}
	switch {
	case kind == KindStudents && (strings.Contains(table.Sheet, "学生") || strings.Contains(table.Sheet, "名单")):
		score += 20
	case kind == KindTeachers && (strings.Contains(table.Sheet, "教师") || strings.Contains(table.Sheet, "任课")):
		score += 20
	case kind == KindAssignments && (strings.Contains(table.Sheet, "任课") || strings.Contains(table.Sheet, "排课")):
		score += 20
	case kind == KindBaselines && strings.Contains(table.Sheet, "基数"):
		score += 20
	case kind == KindClasses && (strings.Contains(table.Sheet, "班级") || strings.Contains(table.Sheet, "结构")):
		score += 20
	case kind == KindRoster && (strings.Contains(table.Sheet, "报名") || strings.Contains(table.Sheet, "名单")):
		score += 20
	case kind == KindScores && (table.Sheet == "总" || table.Sheet == "汇总" ||
		strings.Contains(table.Sheet, "总成绩")):
		score += 20
	case kind == KindCutoffs && strings.Contains(table.Sheet, "分数线"):
		score += 20
	case kind == KindTargets && strings.Contains(table.Sheet, "目标"):
		score += 20
	}
	return score
}

// countSubjectHeaders 统计表头里学科列的数量。
func countSubjectHeaders(headers []string) int {
	count := 0
	for _, header := range headers {
		if _, ok := subjectAliases[strings.TrimSpace(header)]; ok {
			count++
		}
	}
	return count
}

// wideAssignmentTable 判断是不是教师一览的宽表：一行一个班，每个学科一列。
func wideAssignmentTable(table excel.Table) bool {
	if findHeader(table.Headers, "班级", "行政班") < 0 {
		return false
	}
	if findHeader(table.Headers, "教师姓名", "任课教师", "教师", "学科", "科目") >= 0 {
		return false
	}
	if findHeader(table.Headers, "考号", "学号", "考生号") >= 0 {
		return false
	}
	subjects := 0
	for _, header := range table.Headers {
		if _, ok := subjectAliases[strings.TrimSpace(header)]; ok {
			subjects++
		}
	}
	return subjects >= 2
}

// BestSheet 返回数据项最匹配的工作表，找不到时返回空字符串。
func BestSheet(kind string, workbook *Workbook) string {
	best := ""
	bestScore := 0
	for _, table := range workbook.Tables {
		score := DetectSheet(kind, table)
		if score > bestScore {
			best = table.Sheet
			bestScore = score
		}
	}
	return best
}

func hasBaselineSubject(headers []string) bool {
	for _, header := range headers {
		cleaned := strings.TrimSpace(header)
		if cleaned == "" {
			continue
		}
		if _, ok := baselineHeaders[cleaned]; ok {
			return true
		}
		for _, subject := range baselineSubjectLabels {
			if cleaned == subject || cleaned == subject+"均分" || cleaned == subject+"平均分" {
				return true
			}
		}
	}
	return false
}

// DefaultTiers 返回内置方案的档位，用于模板和默认映射。
func DefaultTiers() []string {
	config := assessment.DefaultConfig()
	keys := make([]string, 0, len(config.Tiers))
	for _, tier := range config.Tiers {
		keys = append(keys, tier.Key)
	}
	return keys
}

// Fields 返回一个数据项在向导里需要映射的字段。
func Fields(kind string, subjects []store.Subject) []Field {
	return FieldsFor(kind, subjects, DefaultTiers())
}

// FieldsFor 返回一个数据项在向导里需要映射的字段，档位来自当前方案。
func FieldsFor(kind string, subjects []store.Subject, tiers []string) []Field {
	switch kind {
	case KindClasses:
		return []Field{{Key: FieldClassName, Required: true}}
	case KindStudents:
		return []Field{
			{Key: FieldStudentNo, Required: true},
			{Key: FieldName, Required: true},
			{Key: FieldClassName, Required: true},
			{Key: FieldTrack},
			{Key: FieldElective},
			{Key: FieldStatus},
			{Key: FieldTotalScore},
			{Key: FieldWalking},
			{Key: FieldJapanese},
		}
	case KindTeachers:
		return []Field{
			{Key: FieldTeacherName, Required: true},
			{Key: FieldTeacherNo},
			{Key: FieldSubject},
		}
	case KindAssignments:
		return []Field{
			{Key: FieldTeacherName, Required: true},
			{Key: FieldTeacherNo},
			{Key: FieldClassName, Required: true},
			{Key: FieldSubject, Required: true},
		}
	case KindBaselines:
		fields := []Field{{Key: FieldClassName, Required: true}}
		for _, subject := range subjects {
			if !subject.Enabled || subject.Key == "japanese" {
				continue
			}
			fields = append(fields,
				Field{Key: "subject:" + subject.Key},
				Field{Key: "ratio:" + subject.Key})
		}
		return fields
	case KindRoster:
		return []Field{
			{Key: FieldClassName, Required: true},
			{Key: FieldStudentNo, Required: true},
			{Key: FieldName, Required: true},
			{Key: FieldExamNo},
			{Key: FieldCandidateNo},
			{Key: FieldTrack},
			{Key: FieldElective},
			{Key: FieldStatus},
			{Key: FieldTotalScore},
		}
	case KindScores:
		fields := []Field{
			{Key: FieldStudentNo, Required: true},
			{Key: FieldName},
			{Key: FieldClassName},
			{Key: FieldTrack},
			{Key: FieldElective},
		}
		for _, subject := range subjects {
			if !subject.Enabled || subject.Key == "japanese" {
				continue
			}
			fields = append(fields, Field{Key: "score:" + subject.Key})
		}
		return fields
	case KindCutoffs:
		return []Field{
			{Key: FieldCutoffSubject},
			{Key: FieldCutoffTrack},
			{Key: FieldCutoffTier},
			{Key: FieldCutoffValue, Required: true},
		}
	case KindTargets:
		fields := []Field{{Key: FieldClassName, Required: true}}
		for _, tier := range tiers {
			fields = append(fields, Field{Key: "tier:" + tier})
		}
		return fields
	default:
		return nil
	}
}

// FieldsForTable 返回某个工作表在向导里需要映射的字段。教师一览的宽表
// 只有一个行政班列需要指定，学科列按表头自动识别。
func FieldsForTable(kind string, table excel.Table, subjects []store.Subject, tiers []string) []Field {
	if kind == KindAssignments && wideAssignmentTable(table) {
		return []Field{{Key: FieldClassName, Required: true}}
	}
	if kind == KindTeachers && wideAssignmentTable(table) {
		return nil
	}
	if kind == KindCutoffs && wideCutoffTable(table) {
		return nil
	}
	return FieldsFor(kind, subjects, tiers)
}

// AutoMapping 按表头同义词生成默认列名映射。
func AutoMapping(kind string, table excel.Table, subjects []store.Subject) map[string]string {
	return AutoMappingFor(kind, table, subjects, DefaultTiers())
}

// AutoMappingFor 按表头同义词生成默认列名映射，档位来自当前方案。
func AutoMappingFor(kind string, table excel.Table, subjects []store.Subject, tiers []string) map[string]string {
	mapping := map[string]string{}
	for _, field := range FieldsForTable(kind, table, subjects, tiers) {
		if header := matchHeader(field.Key, table.Headers, subjects); header != "" {
			mapping[field.Key] = header
		}
	}
	if kind == KindAssignments && wideAssignmentTable(table) {
		for _, subject := range subjects {
			if !subject.Enabled {
				continue
			}
			if header := matchHeader("subject:"+subject.Key, table.Headers, subjects); header != "" {
				mapping["subject:"+subject.Key] = header
			}
		}
	}
	return mapping
}

func matchHeader(key string, headers []string, subjects []store.Subject) string {
	names := fieldAliases(key)
	for _, name := range names {
		if index := findHeader(headers, name); index >= 0 {
			return headers[index]
		}
	}
	if strings.HasPrefix(key, "subject:") || strings.HasPrefix(key, "ratio:") {
		subjectKey := strings.TrimPrefix(strings.TrimPrefix(key, "subject:"), "ratio:")
		subject := subjectByKey(subjects, subjectKey)
		if subject == nil {
			return ""
		}
		prefix := ""
		if strings.HasPrefix(key, "ratio:") {
			prefix = "ratio"
		}
		// 入口成绩存班级均分，先匹配「语均」这类均分列，再回退到学科总名列。
		if prefix == "" {
			for _, header := range headers {
				name := strings.TrimSpace(header)
				if alias, ok := baselineHeaders[name]; ok && alias == subjectKey {
					return header
				}
				if name == subject.Name+"均" || name == subject.Name+"均分" || name == subject.Name+"平均分" {
					return header
				}
			}
		}
		for _, header := range headers {
			name := strings.TrimSpace(header)
			if name == subject.Name && prefix == "" {
				return header
			}
			if prefix == "ratio" {
				alias, hasAlias := baselineRatioHeaders[subjectKey]
				if name == subject.Name+"比值" || name == subject.Name+"基" ||
					(hasAlias && name == alias) {
					return header
				}
			}
		}
	}
	if strings.HasPrefix(key, "score:") {
		subjectKey := strings.TrimPrefix(key, "score:")
		subject := subjectByKey(subjects, subjectKey)
		if subject == nil {
			return ""
		}
		for _, header := range headers {
			name := strings.TrimSpace(header)
			if name == subject.Name || name == subject.Name+"成绩" || name == subject.Name+"分数" {
				return header
			}
			if alias, ok := subjectAliases[name]; ok && alias == subjectKey {
				return header
			}
		}
	}
	if strings.HasPrefix(key, "tier:") {
		tierKey := strings.TrimPrefix(key, "tier:")
		label := tierLabel(tierKey)
		if label == "" {
			return ""
		}
		for _, header := range headers {
			name := strings.TrimSpace(header)
			if name == label || strings.HasPrefix(name, label+"目标") ||
				strings.HasPrefix(name, label+"-") {
				return header
			}
			if alias, ok := tierAliases[name]; ok && alias == tierKey {
				return header
			}
		}
	}
	return ""
}

func fieldAliases(key string) []string {
	switch key {
	case FieldClassName:
		return []string{"班级名称", "行政班", "班级", "班"}
	case FieldStudentNo:
		return []string{"学号", "考号", "考生号", "学籍号"}
	case FieldName:
		return []string{"姓名", "学生姓名"}
	case FieldTrack:
		return []string{"选科类别", "类别", "科类"}
	case FieldElective:
		return []string{"选科组合", "选科", "科目组合"}
	case FieldStatus:
		return []string{"学籍状态", "状态", "学生状态"}
	case FieldTotalScore:
		return []string{"入口总分", "总分", "中考总分"}
	case FieldWalking:
		return []string{"是否走班", "走班"}
	case FieldJapanese:
		return []string{"日语", "小语种", "外语语种"}
	case FieldTeacherName:
		return []string{"教师姓名", "任课教师", "教师", "姓名"}
	case FieldTeacherNo:
		return []string{"教师工号", "工号", "教工号"}
	case FieldSubject:
		return []string{"学科", "科目", "任教学科"}
	case FieldExamNo:
		return []string{"考号", "考试号", "准考证号"}
	case FieldCandidateNo:
		return []string{"考生号", "报名号"}
	case FieldCutoffSubject:
		return []string{"科目", "学科"}
	case FieldCutoffTrack:
		return []string{"类别", "科类", "选科类别"}
	case FieldCutoffTier:
		return []string{"档位", "批次", "分数线类型"}
	case FieldCutoffValue:
		return []string{"分数线", "本科线", "分数"}
	default:
		return nil
	}
}

func tierLabel(key string) string {
	switch key {
	case "qingbei":
		return "清北"
	case "c9":
		return "C9"
	case "project985":
		return "985"
	case "project211":
		return "211"
	case "special_control":
		return "特控"
	case "undergraduate":
		return "本科"
	case "single_subject":
		return "单科"
	default:
		return key
	}
}

func findHeader(headers []string, names ...string) int {
	for _, name := range names {
		for index, header := range headers {
			if strings.TrimSpace(header) == name {
				return index
			}
		}
	}
	return -1
}

func subjectByKey(subjects []store.Subject, key string) *store.Subject {
	for index := range subjects {
		if subjects[index].Key == key {
			return &subjects[index]
		}
	}
	return nil
}

func subjectByName(subjects []store.Subject, text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	if key, ok := subjectAliases[text]; ok {
		return key, true
	}
	for _, subject := range subjects {
		if subject.Key == text || subject.Name == text {
			return subject.Key, true
		}
	}
	return "", false
}

// ParseSheet 按列名映射解析一个工作表。
func ParseSheet(workbook *Workbook, sheet, kind string, mapping map[string]string, subjects []store.Subject) (store.ImportData, []store.ImportIssue, error) {
	return ParseSheetFor(workbook, sheet, kind, mapping, subjects, DefaultTiers())
}

// ParseSheetFor 按列名映射解析一个工作表，档位来自当前方案。
func ParseSheetFor(workbook *Workbook, sheet, kind string, mapping map[string]string, subjects []store.Subject, tiers []string) (store.ImportData, []store.ImportIssue, error) {
	return ParseSheetForGrade(workbook, sheet, kind, mapping, subjects, tiers, "")
}

// ParseSheetForGrade 按列名映射解析一个工作表，并只保留指定年级的数据。
// 教师一览这类跨年级的工作簿用它把行归到当前阶段。
func ParseSheetForGrade(workbook *Workbook, sheet, kind string, mapping map[string]string, subjects []store.Subject, tiers []string, grade string) (store.ImportData, []store.ImportIssue, error) {
	table, ok := workbook.Sheet(sheet)
	if !ok {
		return store.ImportData{}, nil, fmt.Errorf("工作簿中没有工作表 %q", sheet)
	}
	switch kind {
	case KindClasses:
		return parseClassSheet(table, mapping)
	case KindStudents:
		return parseRosterSheet(table, mapping, subjects)
	case KindTeachers:
		return parseTeacherSheet(table, mapping, subjects, grade)
	case KindAssignments:
		return parseAssignmentSheet(table, mapping, subjects, grade)
	case KindBaselines:
		return parseBaselineSheet(table, mapping, subjects)
	case KindRoster:
		return parseExamRosterSheet(table, mapping, subjects)
	case KindScores:
		return parseExamScoreSheet(table, mapping, subjects)
	case KindCutoffs:
		if wideCutoffTable(table) {
			return parseWideCutoffSheet(table, subjects)
		}
		return parseExamCutoffSheet(table, mapping, subjects)
	case KindTargets:
		return parseExamTargetSheet(table, mapping, tiers)
	default:
		return store.ImportData{}, nil, fmt.Errorf("未知的数据项 %q", kind)
	}
}

func columnIndex(table excel.Table, mapping map[string]string, key string) int {
	header := mapping[key]
	if header == "" {
		return -1
	}
	for index, value := range table.Headers {
		if value == header {
			return index
		}
	}
	return -1
}

func invalidNumberIssue(row int, column, text string) store.ImportIssue {
	return store.ImportIssue{
		Severity: "blocking", Row: row, Column: column, Code: "invalid_number",
		Args: []string{text},
	}
}

func missingFieldIssue(row int, column string) store.ImportIssue {
	return store.ImportIssue{
		Severity: "blocking", Row: row, Column: column, Code: "missing_field",
	}
}

func parseClassSheet(table excel.Table, mapping map[string]string) (store.ImportData, []store.ImportIssue, error) {
	column := columnIndex(table, mapping, FieldClassName)
	if column < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少班级名称列", table.Sheet)
	}
	data := store.ImportData{}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	for _, row := range table.Rows {
		name := normalizeClassName(valueAt(row.Values, column))
		if name == "" {
			continue
		}
		if skipSummaryRow(name) {
			continue
		}
		if seen[name] {
			issues = append(issues, store.ImportIssue{
				Severity: "warning", Row: row.Number, Column: mapping[FieldClassName],
				Code: "duplicate_class", Args: []string{name},
			})
			continue
		}
		seen[name] = true
		data.Classes = append(data.Classes, name)
	}
	if len(data.Classes) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldClassName],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func parseRosterSheet(table excel.Table, mapping map[string]string, subjects []store.Subject) (store.ImportData, []store.ImportIssue, error) {
	numberColumn := columnIndex(table, mapping, FieldStudentNo)
	nameColumn := columnIndex(table, mapping, FieldName)
	classColumn := columnIndex(table, mapping, FieldClassName)
	if numberColumn < 0 || nameColumn < 0 || classColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少学号、姓名或者行政班列", table.Sheet)
	}
	data := store.ImportData{}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	classNames := map[string]bool{}
	columns := map[string]int{}
	for _, key := range []string{FieldTrack, FieldElective, FieldStatus, FieldTotalScore, FieldWalking, FieldJapanese} {
		if index := columnIndex(table, mapping, key); index >= 0 {
			columns[key] = index
		}
	}
	for _, row := range table.Rows {
		studentNo := strings.TrimSpace(valueAt(row.Values, numberColumn))
		name := strings.TrimSpace(valueAt(row.Values, nameColumn))
		className := normalizeClassName(valueAt(row.Values, classColumn))
		if studentNo == "" && name == "" && className == "" {
			continue
		}
		if skipSummaryRow(className) {
			continue
		}
		if studentNo == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldStudentNo]))
			continue
		}
		if name == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldName]))
			continue
		}
		if className == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldClassName]))
			continue
		}
		if seen[studentNo] {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldStudentNo],
				Code: "duplicate_student", Args: []string{studentNo},
			})
			continue
		}
		seen[studentNo] = true
		classNames[className] = true
		student := store.ImportStudent{
			StudentNo: studentNo, Name: name, ClassName: className,
			SourceSheet: table.Sheet, SourceRow: row.Number,
		}
		if index, ok := columns[FieldElective]; ok {
			student.Elective = valueAt(row.Values, index)
		}
		if index, ok := columns[FieldTrack]; ok {
			student.Track = trackFromText(valueAt(row.Values, index))
		}
		if student.Track == "" {
			student.Track = trackFromElective(student.Elective)
		}
		if index, ok := columns[FieldStatus]; ok {
			text := valueAt(row.Values, index)
			student.Status = statusValue(text)
			if text != "" && student.Status == "" {
				issues = append(issues, store.ImportIssue{
					Severity: "warning", Row: row.Number, Column: mapping[FieldStatus],
					Code: "unknown_status", Args: []string{text},
				})
			}
		}
		if student.Status == "" {
			student.Status = "active"
		}
		if index, ok := columns[FieldWalking]; ok {
			student.Walking = flagValue(valueAt(row.Values, index))
		}
		if index, ok := columns[FieldJapanese]; ok {
			student.Japanese = flagValue(valueAt(row.Values, index))
		}
		if index, ok := columns[FieldTotalScore]; ok {
			text := valueAt(row.Values, index)
			if text != "" {
				number, status := parseNumber(text)
				if status == numberOK {
					student.TotalScore = &number
				} else {
					issues = append(issues, invalidNumberIssue(row.Number, mapping[FieldTotalScore], text))
				}
			}
		}
		data.Students = append(data.Students, student)
	}
	names := make([]string, 0, len(classNames))
	for name := range classNames {
		names = append(names, name)
	}
	sort.Strings(names)
	data.Classes = names
	if len(data.Students) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldName],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func trackFromText(text string) string {
	text = strings.TrimSpace(text)
	if strings.Contains(text, "物理") {
		return "physics"
	}
	if strings.Contains(text, "历史") {
		return "history"
	}
	return ""
}

func parseTeacherSheet(table excel.Table, mapping map[string]string, subjects []store.Subject, grade string) (store.ImportData, []store.ImportIssue, error) {
	if wideAssignmentTable(table) {
		return parseWideTeacherSheet(table, subjects, grade)
	}
	nameColumn := columnIndex(table, mapping, FieldTeacherName)
	if nameColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少教师姓名列", table.Sheet)
	}
	noColumn := columnIndex(table, mapping, FieldTeacherNo)
	subjectColumn := columnIndex(table, mapping, FieldSubject)
	data := store.ImportData{}
	var issues []store.ImportIssue
	seenNo := map[string]string{}
	seenNameSubject := map[string]string{}
	for _, row := range table.Rows {
		name := strings.TrimSpace(valueAt(row.Values, nameColumn))
		teacherNo := strings.TrimSpace(valueAt(row.Values, noColumn))
		subjectText := ""
		if subjectColumn >= 0 {
			subjectText = strings.TrimSpace(valueAt(row.Values, subjectColumn))
		}
		if name == "" {
			if teacherNo != "" || subjectText != "" {
				issues = append(issues, missingFieldIssue(row.Number, mapping[FieldTeacherName]))
			}
			continue
		}
		if skipSummaryRow(name) {
			continue
		}
		subject := ""
		if subjectText != "" {
			key, ok := subjectByName(subjects, subjectText)
			if !ok {
				issues = append(issues, store.ImportIssue{
					Severity: "blocking", Row: row.Number, Column: mapping[FieldSubject],
					Code: "unknown_subject", Args: []string{subjectText},
				})
				continue
			}
			subject = key
		}
		if teacherNo == "" && subject == "" {
			column := mapping[FieldTeacherNo]
			if column == "" {
				column = mapping[FieldTeacherName]
			}
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: column,
				Code: "teacher_identity_missing", Args: []string{name},
			})
			continue
		}
		if teacherNo != "" {
			if previous, ok := seenNo[teacherNo]; ok && previous != name {
				issues = append(issues, store.ImportIssue{
					Severity: "blocking", Row: row.Number, Column: mapping[FieldTeacherNo],
					Code: "duplicate_teacher_no", Args: []string{teacherNo},
				})
				continue
			}
			seenNo[teacherNo] = name
		}
		key := name + "\x00" + subject
		if previous, ok := seenNameSubject[key]; ok {
			if previous != teacherNo {
				issues = append(issues, store.ImportIssue{
					Severity: "blocking", Row: row.Number, Column: mapping[FieldTeacherName],
					Code: "teacher_name_conflict", Args: []string{name, subjectText},
				})
			}
			continue
		}
		seenNameSubject[key] = teacherNo
		data.TeacherList = append(data.TeacherList, store.ImportTeacher{
			TeacherNo: teacherNo, Name: name, Subject: subject,
			SourceSheet: table.Sheet, SourceRow: row.Number,
		})
	}
	if len(data.TeacherList) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldTeacherName],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

// parseWideTeacherSheet 从教师一览的宽表里收集教师名单，学科来自列名。
func parseWideTeacherSheet(table excel.Table, subjects []store.Subject, grade string) (store.ImportData, []store.ImportIssue, error) {
	data := store.ImportData{}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	foundSubjectColumn := false
	gradeColumn := gradeColumnIndex(table)
	for _, subject := range subjects {
		if !subject.Enabled {
			continue
		}
		column := -1
		for index, header := range table.Headers {
			if strings.TrimSpace(header) == subject.Name {
				column = index
				break
			}
		}
		if column < 0 {
			continue
		}
		foundSubjectColumn = true
		for _, row := range table.Rows {
			if !rowInGrade(row, gradeColumn, grade) {
				continue
			}
			name := strings.TrimSpace(valueAt(row.Values, column))
			if name == "" || name == "/" || name == "-" || name == "—" {
				continue
			}
			key := name + "\x00" + subject.Key
			if seen[key] {
				continue
			}
			seen[key] = true
			data.TeacherList = append(data.TeacherList, store.ImportTeacher{
				Name: name, Subject: subject.Key,
				SourceSheet: table.Sheet, SourceRow: row.Number,
			})
		}
	}
	if !foundSubjectColumn || len(data.TeacherList) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: FieldTeacherName,
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func parseAssignmentSheet(table excel.Table, mapping map[string]string, subjects []store.Subject, grade string) (store.ImportData, []store.ImportIssue, error) {
	if wideAssignmentTable(table) {
		return parseWideAssignmentSheet(table, mapping, subjects, grade)
	}
	teacherColumn := columnIndex(table, mapping, FieldTeacherName)
	classColumn := columnIndex(table, mapping, FieldClassName)
	subjectColumn := columnIndex(table, mapping, FieldSubject)
	if teacherColumn < 0 || classColumn < 0 || subjectColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少教师、行政班或者学科列", table.Sheet)
	}
	noColumn := columnIndex(table, mapping, FieldTeacherNo)
	data := store.ImportData{}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	for _, row := range table.Rows {
		teacher := strings.TrimSpace(valueAt(row.Values, teacherColumn))
		className := normalizeClassName(valueAt(row.Values, classColumn))
		subjectText := strings.TrimSpace(valueAt(row.Values, subjectColumn))
		if teacher == "" && className == "" && subjectText == "" {
			continue
		}
		if skipSummaryRow(className) {
			continue
		}
		if teacher == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldTeacherName]))
			continue
		}
		if className == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldClassName]))
			continue
		}
		if subjectText == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldSubject]))
			continue
		}
		subject, ok := subjectByName(subjects, subjectText)
		if !ok {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldSubject],
				Code: "unknown_subject", Args: []string{subjectText},
			})
			continue
		}
		key := teacher + "\x00" + className + "\x00" + subject
		if seen[key] {
			issues = append(issues, store.ImportIssue{
				Severity: "warning", Row: row.Number, Column: mapping[FieldTeacherName],
				Code: "duplicate_assignment", Args: []string{teacher, className, subjectText},
			})
			continue
		}
		seen[key] = true
		assignment := store.ImportAssignment{
			ClassName: className, Subject: subject, TeacherName: teacher,
			SourceSheet: table.Sheet, SourceRow: row.Number,
		}
		if noColumn >= 0 {
			assignment.TeacherNo = strings.TrimSpace(valueAt(row.Values, noColumn))
		}
		data.Assignments = append(data.Assignments, assignment)
		data.Classes = append(data.Classes, className)
	}
	data.Classes = uniqueStrings(data.Classes)
	if len(data.Assignments) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldTeacherName],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

// parseWideAssignmentSheet 读取教师一览的宽表：每个学科一列，单元格是该班的任课教师。
func parseWideAssignmentSheet(table excel.Table, mapping map[string]string, subjects []store.Subject, grade string) (store.ImportData, []store.ImportIssue, error) {
	classColumn := columnIndex(table, mapping, FieldClassName)
	if classColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少行政班列", table.Sheet)
	}
	gradeColumn := gradeColumnIndex(table)
	type subjectColumn struct {
		subject string
		column  int
	}
	var columns []subjectColumn
	for _, subject := range subjects {
		if !subject.Enabled {
			continue
		}
		column := columnIndex(table, mapping, "subject:"+subject.Key)
		if column < 0 {
			continue
		}
		columns = append(columns, subjectColumn{subject: subject.Key, column: column})
	}
	if len(columns) == 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 没有可识别的学科列", table.Sheet)
	}
	data := store.ImportData{}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	teacherSeen := map[string]bool{}
	for _, row := range table.Rows {
		if !rowInGrade(row, gradeColumn, grade) {
			continue
		}
		className := normalizeClassName(valueAt(row.Values, classColumn))
		if className == "" || skipSummaryRow(className) {
			continue
		}
		for _, column := range columns {
			teacher := strings.TrimSpace(valueAt(row.Values, column.column))
			if teacher == "" || teacher == "/" || teacher == "-" || teacher == "—" {
				continue
			}
			key := teacher + "\x00" + className + "\x00" + column.subject
			if seen[key] {
				issues = append(issues, store.ImportIssue{
					Severity: "warning", Row: row.Number, Column: table.Headers[column.column],
					Code: "duplicate_assignment", Args: []string{teacher, className, subjectLabel(subjects, column.subject)},
				})
				continue
			}
			seen[key] = true
			data.Assignments = append(data.Assignments, store.ImportAssignment{
				ClassName: className, Subject: column.subject, TeacherName: teacher,
				SourceSheet: table.Sheet, SourceRow: row.Number,
			})
			data.Classes = append(data.Classes, className)
			teacherKey := teacher + "\x00" + column.subject
			if !teacherSeen[teacherKey] {
				teacherSeen[teacherKey] = true
				data.TeacherList = append(data.TeacherList, store.ImportTeacher{
					Name: teacher, Subject: column.subject,
					SourceSheet: table.Sheet, SourceRow: row.Number,
				})
			}
		}
	}
	data.Classes = uniqueStrings(data.Classes)
	if len(data.Assignments) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldClassName],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func subjectLabel(subjects []store.Subject, key string) string {
	for _, subject := range subjects {
		if subject.Key == key {
			return subject.Name
		}
	}
	return key
}

// gradeColumnIndex 返回教师一览里的年级列。
func gradeColumnIndex(table excel.Table) int {
	for index, header := range table.Headers {
		switch strings.TrimSpace(header) {
		case "年级", "级", "Grade", "Stage":
			return index
		}
	}
	return -1
}

// rowInGrade 判断教师一览的一行是否属于当前阶段。
func rowInGrade(row excel.TableRow, column int, grade string) bool {
	if column < 0 || grade == "" {
		return true
	}
	text := strings.TrimSpace(valueAt(row.Values, column))
	switch grade {
	case "g1":
		return strings.Contains(text, "高一") || text == "Grade 10" || text == "10"
	case "g2":
		return strings.Contains(text, "高二") || text == "Grade 11" || text == "11"
	case "g3":
		return strings.Contains(text, "高三") || text == "Grade 12" || text == "12"
	default:
		return true
	}
}

func parseBaselineSheet(table excel.Table, mapping map[string]string, subjects []store.Subject) (store.ImportData, []store.ImportIssue, error) {
	classColumn := columnIndex(table, mapping, FieldClassName)
	if classColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少行政班列", table.Sheet)
	}
	type subjectColumn struct {
		subject string
		average int
		ratio   int
	}
	var columns []subjectColumn
	for _, subject := range subjects {
		average := columnIndex(table, mapping, "subject:"+subject.Key)
		ratio := columnIndex(table, mapping, "ratio:"+subject.Key)
		if average < 0 && ratio < 0 {
			continue
		}
		if average < 0 {
			continue
		}
		columns = append(columns, subjectColumn{subject: subject.Key, average: average, ratio: ratio})
	}
	if len(columns) == 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 没有可导入的入口成绩列", table.Sheet)
	}
	data := store.ImportData{GradeBaselines: map[string]float64{}}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	for _, row := range table.Rows {
		className := normalizeClassName(valueAt(row.Values, classColumn))
		if className == "" {
			continue
		}
		if skipSummaryRow(className) {
			continue
		}
		for _, column := range columns {
			text := valueAt(row.Values, column.average)
			if text == "" {
				continue
			}
			number, status := parseNumber(text)
			if status != numberOK {
				issues = append(issues, invalidNumberIssue(row.Number, mapping["subject:"+column.subject], text))
				continue
			}
			key := className + "\x00" + column.subject
			if seen[key] {
				issues = append(issues, store.ImportIssue{
					Severity: "blocking", Row: row.Number, Column: mapping["subject:"+column.subject],
					Code: "duplicate_baseline", Args: []string{className, column.subject},
				})
				continue
			}
			seen[key] = true
			baseline := store.ImportClassBaseline{
				ClassName: className, Subject: column.subject, Average: number,
				SourceSheet: table.Sheet, SourceRow: row.Number,
			}
			if column.ratio >= 0 {
				ratioText := valueAt(row.Values, column.ratio)
				if ratioText != "" {
					if ratio, ratioStatus := parseNumber(ratioText); ratioStatus == numberOK {
						baseline.Ratio = &ratio
					} else {
						issues = append(issues, invalidNumberIssue(row.Number, mapping["ratio:"+column.subject], ratioText))
					}
				}
			}
			data.ClassBaselines = append(data.ClassBaselines, baseline)
		}
	}
	if len(data.ClassBaselines) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldClassName],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func parseExamRosterSheet(table excel.Table, mapping map[string]string, subjects []store.Subject) (store.ImportData, []store.ImportIssue, error) {
	numberColumn := columnIndex(table, mapping, FieldStudentNo)
	nameColumn := columnIndex(table, mapping, FieldName)
	classColumn := columnIndex(table, mapping, FieldClassName)
	if numberColumn < 0 || nameColumn < 0 || classColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少学号、姓名或者行政班列", table.Sheet)
	}
	columns := map[string]int{}
	for _, key := range []string{FieldExamNo, FieldCandidateNo, FieldTrack, FieldElective, FieldStatus, FieldTotalScore} {
		if index := columnIndex(table, mapping, key); index >= 0 {
			columns[key] = index
		}
	}
	data := store.ImportData{}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	for _, row := range table.Rows {
		studentNo := strings.TrimSpace(valueAt(row.Values, numberColumn))
		name := strings.TrimSpace(valueAt(row.Values, nameColumn))
		className := normalizeClassName(valueAt(row.Values, classColumn))
		if studentNo == "" && name == "" && className == "" {
			continue
		}
		if skipSummaryRow(className) {
			continue
		}
		if studentNo == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldStudentNo]))
			continue
		}
		if name == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldName]))
			continue
		}
		if className == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldClassName]))
			continue
		}
		if seen[studentNo] {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldStudentNo],
				Code: "duplicate_student", Args: []string{studentNo},
			})
			continue
		}
		seen[studentNo] = true
		entry := store.ImportRoster{
			StudentNo: studentNo, Name: name, ClassName: className,
			SourceSheet: table.Sheet, SourceRow: row.Number,
		}
		if index, ok := columns[FieldExamNo]; ok {
			entry.ExamNo = strings.TrimSpace(valueAt(row.Values, index))
		}
		if index, ok := columns[FieldCandidateNo]; ok {
			entry.CandidateNo = strings.TrimSpace(valueAt(row.Values, index))
		}
		if index, ok := columns[FieldTrack]; ok {
			entry.Track = trackFromText(valueAt(row.Values, index))
		}
		if index, ok := columns[FieldElective]; ok {
			entry.Elective = valueAt(row.Values, index)
			if entry.Track == "" {
				entry.Track = trackFromElective(entry.Elective)
			}
		}
		if index, ok := columns[FieldStatus]; ok {
			entry.Status = statusValue(valueAt(row.Values, index))
		}
		if index, ok := columns[FieldTotalScore]; ok {
			text := valueAt(row.Values, index)
			if text != "" {
				number, status := parseNumber(text)
				if status == numberOK {
					entry.TotalScore = &number
				} else {
					issues = append(issues, invalidNumberIssue(row.Number, mapping[FieldTotalScore], text))
				}
			}
		}
		data.Roster = append(data.Roster, entry)
	}
	if len(data.Roster) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldName],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func parseExamScoreSheet(table excel.Table, mapping map[string]string, subjects []store.Subject) (store.ImportData, []store.ImportIssue, error) {
	numberColumn := columnIndex(table, mapping, FieldStudentNo)
	if numberColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少学号或者考号列", table.Sheet)
	}
	nameColumn := columnIndex(table, mapping, FieldName)
	classColumn := columnIndex(table, mapping, FieldClassName)
	trackColumn := columnIndex(table, mapping, FieldTrack)
	electiveColumn := columnIndex(table, mapping, FieldElective)
	type subjectColumn struct {
		subject string
		index   int
	}
	var subjectColumns []subjectColumn
	for _, subject := range subjects {
		index := columnIndex(table, mapping, "score:"+subject.Key)
		if index >= 0 {
			subjectColumns = append(subjectColumns, subjectColumn{subject: subject.Key, index: index})
		}
	}
	if len(subjectColumns) == 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 没有可导入的成绩列", table.Sheet)
	}
	data := store.ImportData{}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	for _, row := range table.Rows {
		studentNo := strings.TrimSpace(valueAt(row.Values, numberColumn))
		name := strings.TrimSpace(valueAt(row.Values, nameColumn))
		className := normalizeClassName(valueAt(row.Values, classColumn))
		if studentNo == "" && name == "" && className == "" {
			continue
		}
		if skipSummaryRow(className) {
			continue
		}
		if studentNo == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldStudentNo]))
			continue
		}
		if seen[studentNo] {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldStudentNo],
				Code: "duplicate_student", Args: []string{studentNo},
			})
			continue
		}
		seen[studentNo] = true
		student := store.ImportStudent{
			StudentNo: studentNo, Name: name, ClassName: className,
			Scores: map[string]float64{}, Flags: map[string]string{},
			SourceSheet: table.Sheet, SourceRow: row.Number,
		}
		if electiveColumn >= 0 {
			student.Elective = valueAt(row.Values, electiveColumn)
		}
		if trackColumn >= 0 {
			student.Track = trackFromText(valueAt(row.Values, trackColumn))
		}
		if student.Track == "" {
			student.Track = trackFromElective(student.Elective)
		}
		for _, column := range subjectColumns {
			text := valueAt(row.Values, column.index)
			number, status := parseNumber(text)
			switch status {
			case numberOK:
				if number == 0 {
					student.Flags[column.subject] = "zero"
					continue
				}
				student.Scores[column.subject] = number
			case numberFlag:
				student.Flags[column.subject] = invalidFlag(text)
			case numberEmpty:
			case numberInvalid:
				issues = append(issues, invalidNumberIssue(row.Number,
					mapping["score:"+column.subject], text))
			}
		}
		data.Students = append(data.Students, student)
	}
	if len(data.Students) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldStudentNo],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func parseExamCutoffSheet(table excel.Table, mapping map[string]string, subjects []store.Subject) (store.ImportData, []store.ImportIssue, error) {
	valueColumn := columnIndex(table, mapping, FieldCutoffValue)
	if valueColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少分数线列", table.Sheet)
	}
	subjectColumn := columnIndex(table, mapping, FieldCutoffSubject)
	trackColumn := columnIndex(table, mapping, FieldCutoffTrack)
	tierColumn := columnIndex(table, mapping, FieldCutoffTier)
	data := store.ImportData{}
	var issues []store.ImportIssue
	seenSubject := map[string]bool{}
	seenTier := map[string]bool{}
	for _, row := range table.Rows {
		subjectText := strings.TrimSpace(valueAt(row.Values, subjectColumn))
		tierText := strings.TrimSpace(valueAt(row.Values, tierColumn))
		trackText := strings.TrimSpace(valueAt(row.Values, trackColumn))
		valueText := strings.TrimSpace(valueAt(row.Values, valueColumn))
		if subjectText == "" && tierText == "" && valueText == "" {
			continue
		}
		if valueText == "" {
			issues = append(issues, missingFieldIssue(row.Number, mapping[FieldCutoffValue]))
			continue
		}
		value, status := parseNumber(valueText)
		if status != numberOK || value <= 0 {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldCutoffValue],
				Code: "cutoff_not_positive", Args: []string{subjectText, tierText},
			})
			continue
		}
		track := trackFromText(trackText)
		if subjectText != "" {
			subject, ok := subjectByName(subjects, subjectText)
			if !ok {
				issues = append(issues, store.ImportIssue{
					Severity: "blocking", Row: row.Number, Column: mapping[FieldCutoffSubject],
					Code: "unknown_subject", Args: []string{subjectText},
				})
				continue
			}
			key := subject + "\x00" + track
			if seenSubject[key] {
				issues = append(issues, store.ImportIssue{
					Severity: "blocking", Row: row.Number, Column: mapping[FieldCutoffSubject],
					Code: "duplicate_cutoff", Args: []string{subjectText, trackText},
				})
				continue
			}
			seenSubject[key] = true
			data.SubjectCutoffs = append(data.SubjectCutoffs, store.ImportSubjectCutoff{
				Subject: subject, Track: track, Value: value,
			})
			continue
		}
		if tierText == "" {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldCutoffValue],
				Code: "missing_field", Args: []string{FieldCutoffSubject},
			})
			continue
		}
		tier := tierKeyFromText(tierText)
		if tier == "" {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldCutoffTier],
				Code: "unknown_tier", Args: []string{tierText},
			})
			continue
		}
		key := tier + "\x00" + track
		if seenTier[key] {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldCutoffTier],
				Code: "duplicate_cutoff", Args: []string{tierText, trackText},
			})
			continue
		}
		seenTier[key] = true
		data.TierCutoffs = append(data.TierCutoffs, store.ImportTierCutoff{
			TierKey: tier, Track: track, Value: value,
		})
	}
	if len(data.SubjectCutoffs) == 0 && len(data.TierCutoffs) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldCutoffValue],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

// wideCutoffTable 判断是不是横向排列的分数线表：多个「科目」列，每组后面跟本组各条线。
func wideCutoffTable(table excel.Table) bool {
	subjectColumns := 0
	lineColumns := 0
	for _, header := range table.Headers {
		switch strings.TrimSpace(header) {
		case "科目":
			subjectColumns++
		case "本科线", "特控线", "分数线", "总分线", "985线", "211线", "清北线", "C9线":
			lineColumns++
		}
	}
	return subjectColumns >= 2 && lineColumns >= 1
}

// tierKeyFromHeaderText 从「特控线」这类列名读出档位。
func tierKeyFromHeaderText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, tier := range assessment.DefaultConfig().Tiers {
		label := tierLabel(tier.Key)
		if label == "" {
			continue
		}
		if text == label || strings.HasPrefix(text, label) {
			return tier.Key
		}
	}
	return ""
}

// wideCutoffGroupTrack 从分组上方的「总分(物理)」行识别类别，识别不出时按科目推断。
func wideCutoffGroupTrack(table excel.Table, start, end int) string {
	for _, row := range table.LeadRows {
		for column := start; column < end; column++ {
			text := strings.TrimSpace(valueAt(row.Values, column))
			if text == "" {
				continue
			}
			if strings.Contains(text, "历史") {
				return "history"
			}
			if strings.Contains(text, "物理") {
				return "physics"
			}
		}
	}
	seen := map[string]bool{}
	for _, row := range table.Rows {
		seen[strings.TrimSpace(valueAt(row.Values, start))] = true
	}
	track := ""
	for name := range seen {
		if name == "历史" {
			track = "history"
		}
		if name == "物理" {
			track = "physics"
		}
	}
	return track
}

func parseWideCutoffSheet(table excel.Table, subjects []store.Subject) (store.ImportData, []store.ImportIssue, error) {
	type lineColumn struct {
		header string
		column int
		tier   string
	}
	type group struct {
		subjectColumn int
		track         string
		lines         []lineColumn
	}
	var groups []group
	subjectPositions := []int{}
	for index, header := range table.Headers {
		if strings.TrimSpace(header) == "科目" {
			subjectPositions = append(subjectPositions, index)
		}
	}
	for i, start := range subjectPositions {
		end := len(table.Headers)
		if i+1 < len(subjectPositions) {
			end = subjectPositions[i+1]
		}
		group := group{subjectColumn: start}
		for column := start + 1; column < end; column++ {
			tier := tierKeyFromHeaderText(table.Headers[column])
			if tier == "" {
				continue
			}
			group.lines = append(group.lines, lineColumn{header: table.Headers[column], column: column, tier: tier})
		}
		if len(group.lines) == 0 {
			continue
		}
		group.track = wideCutoffGroupTrack(table, start, end)
		if group.track == "" {
			return store.ImportData{}, nil, fmt.Errorf("工作表 %s 的分组没有标注类别，需要「总分(物理)」这样的分组行", table.Sheet)
		}
		groups = append(groups, group)
	}
	if len(groups) == 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 没有可识别的分数线分组", table.Sheet)
	}
	data := store.ImportData{}
	var issues []store.ImportIssue
	seenSubject := map[string]bool{}
	seenTier := map[string]bool{}
	enabledSubjects := map[string]bool{}
	for _, subject := range subjects {
		if subject.Enabled {
			enabledSubjects[subject.Key] = true
		}
	}
	for _, row := range table.Rows {
		for _, group := range groups {
			subjectText := strings.TrimSpace(valueAt(row.Values, group.subjectColumn))
			if subjectText == "" {
				continue
			}
			for _, line := range group.lines {
				text := strings.TrimSpace(valueAt(row.Values, line.column))
				if text == "" {
					continue
				}
				value, status := parseNumber(text)
				if status != numberOK {
					issues = append(issues, invalidNumberIssue(row.Number, line.header, text))
					continue
				}
				if value <= 0 {
					issues = append(issues, store.ImportIssue{
						Severity: "warning", Row: row.Number, Column: line.header,
						Code: "cutoff_zero", Args: []string{subjectText, group.track},
					})
					continue
				}
				if subjectText == "总分" {
					key := line.tier + "\x00" + group.track
					if seenTier[key] {
						issues = append(issues, store.ImportIssue{
							Severity: "blocking", Row: row.Number, Column: line.header,
							Code: "duplicate_cutoff", Args: []string{subjectText, group.track},
						})
						continue
					}
					seenTier[key] = true
					data.TierCutoffs = append(data.TierCutoffs, store.ImportTierCutoff{
						TierKey: line.tier, Track: group.track, Value: value,
					})
					continue
				}
				subject, ok := subjectByName(subjects, subjectText)
				if !ok || !enabledSubjects[subject] {
					issues = append(issues, store.ImportIssue{
						Severity: "warning", Row: row.Number, Column: table.Headers[group.subjectColumn],
						Code: "cutoff_subject_skipped", Args: []string{subjectText},
					})
					continue
				}
				key := subject + "\x00" + group.track
				if seenSubject[key] {
					issues = append(issues, store.ImportIssue{
						Severity: "blocking", Row: row.Number, Column: table.Headers[group.subjectColumn],
						Code: "duplicate_cutoff", Args: []string{subjectText, group.track},
					})
					continue
				}
				seenSubject[key] = true
				data.SubjectCutoffs = append(data.SubjectCutoffs, store.ImportSubjectCutoff{
					Subject: subject, Track: group.track, Value: value,
				})
			}
		}
	}
	if len(data.SubjectCutoffs) == 0 && len(data.TierCutoffs) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: "科目",
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func parseExamTargetSheet(table excel.Table, mapping map[string]string, tiers []string) (store.ImportData, []store.ImportIssue, error) {
	classColumn := columnIndex(table, mapping, FieldClassName)
	if classColumn < 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 缺少行政班列", table.Sheet)
	}
	type tierColumn struct {
		key   string
		index int
	}
	var columns []tierColumn
	for _, tier := range tiers {
		if index := columnIndex(table, mapping, "tier:"+tier); index >= 0 {
			columns = append(columns, tierColumn{key: tier, index: index})
		}
	}
	if len(columns) == 0 {
		return store.ImportData{}, nil, fmt.Errorf("工作表 %s 没有档位目标列", table.Sheet)
	}
	data := store.ImportData{}
	var issues []store.ImportIssue
	seen := map[string]bool{}
	for _, row := range table.Rows {
		className := normalizeClassName(valueAt(row.Values, classColumn))
		if className == "" {
			continue
		}
		if skipSummaryRow(className) {
			continue
		}
		if seen[className] {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: row.Number, Column: mapping[FieldClassName],
				Code: "duplicate_target", Args: []string{className},
			})
			continue
		}
		seen[className] = true
		target := store.ImportClassTarget{ClassName: className, Counts: map[string]*float64{}}
		for _, column := range columns {
			text := valueAt(row.Values, column.index)
			if text == "" {
				continue
			}
			number, status := parseNumber(text)
			if status != numberOK {
				issues = append(issues, invalidNumberIssue(row.Number,
					mapping["tier:"+column.key], text))
				continue
			}
			if number < 0 {
				issues = append(issues, store.ImportIssue{
					Severity: "blocking", Row: row.Number, Column: mapping["tier:"+column.key],
					Code: "negative_target", Args: []string{className, column.key},
				})
				continue
			}
			copied := number
			target.Counts[column.key] = &copied
		}
		data.ClassTargets = append(data.ClassTargets, target)
	}
	if len(data.ClassTargets) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking", Row: table.HeaderRow, Column: mapping[FieldClassName],
			Code: "no_rows",
		})
	}
	return data, issues, nil
}

func tierKeyFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, tier := range assessment.DefaultConfig().Tiers {
		label := tierLabel(tier.Key)
		if text == label {
			return tier.Key
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeClassName(name string) string {
	text := strings.TrimSpace(name)
	replacer := strings.NewReplacer("（", "(", "）", ")", "　", "")
	return replacer.Replace(text)
}

// SheetPreview 返回工作表前几行，用于向导展示。
func SheetPreview(table excel.Table, limit int) [][]string {
	if limit <= 0 || limit > len(table.Rows) {
		limit = len(table.Rows)
	}
	rows := make([][]string, 0, limit)
	for _, row := range table.Rows[:limit] {
		rows = append(rows, row.Values)
	}
	return rows
}

// MappingDescription 返回列名映射的中文说明，供导入记录和问题清单使用。
func MappingDescription(mapping map[string]string) string {
	keys := make([]string, 0, len(mapping))
	for key, header := range mapping {
		if header == "" {
			continue
		}
		keys = append(keys, key+"="+header)
	}
	sort.Strings(keys)
	return strings.Join(keys, "; ")
}

// TemplateHeaders 返回模板的第一行表头。
func TemplateHeaders(kind string, subjects []store.Subject) []string {
	headers := make([]string, 0)
	for _, field := range Fields(kind, subjects) {
		if strings.HasPrefix(field.Key, "ratio:") {
			continue
		}
		headers = append(headers, FieldHeader(field.Key, subjects))
	}
	return headers
}

// FieldHeader 返回字段在模板和界面上的中文表头。
func FieldHeader(key string, subjects []store.Subject) string {
	switch key {
	case FieldClassName:
		return "行政班"
	case FieldStudentNo:
		return "学号"
	case FieldName:
		return "姓名"
	case FieldTrack:
		return "选科类别"
	case FieldElective:
		return "选科组合"
	case FieldStatus:
		return "状态"
	case FieldTotalScore:
		return "入口总分"
	case FieldWalking:
		return "是否走班"
	case FieldJapanese:
		return "日语"
	case FieldTeacherName:
		return "教师姓名"
	case FieldTeacherNo:
		return "工号"
	case FieldSubject:
		return "学科"
	}
	if strings.HasPrefix(key, "subject:") {
		if subject := subjectByKey(subjects, strings.TrimPrefix(key, "subject:")); subject != nil {
			return subject.Name
		}
	}
	if strings.HasPrefix(key, "ratio:") {
		if subject := subjectByKey(subjects, strings.TrimPrefix(key, "ratio:")); subject != nil {
			return subject.Name + "比值"
		}
	}
	return key
}

// TemplateExample 返回模板的示例行。
func TemplateExample(kind string, subjects []store.Subject) []string {
	values := make([]string, 0)
	for _, field := range Fields(kind, subjects) {
		if strings.HasPrefix(field.Key, "ratio:") {
			continue
		}
		switch {
		case field.Key == FieldClassName:
			values = append(values, "1班")
		case field.Key == FieldStudentNo:
			values = append(values, "2026001")
		case field.Key == FieldName:
			values = append(values, "示例学生")
		case field.Key == FieldTrack:
			values = append(values, "物理类")
		case field.Key == FieldElective:
			values = append(values, "物理、化学、生物")
		case field.Key == FieldStatus:
			values = append(values, "在读")
		case field.Key == FieldTotalScore:
			values = append(values, "620.5")
		case field.Key == FieldWalking:
			values = append(values, "否")
		case field.Key == FieldJapanese:
			values = append(values, "否")
		case field.Key == FieldTeacherName:
			values = append(values, "示例教师")
		case field.Key == FieldTeacherNo:
			values = append(values, "T001")
		case field.Key == FieldSubject:
			values = append(values, "数学")
		case strings.HasPrefix(field.Key, "subject:"):
			values = append(values, "102.5")
		default:
			values = append(values, "")
		}
	}
	return values
}

// TemplateDescription 返回模板说明工作表的行。
func TemplateDescription(kind string, subjects []store.Subject) [][]string {
	rows := [][]string{{"字段", "是否必需", "说明"}}
	for _, field := range Fields(kind, subjects) {
		if strings.HasPrefix(field.Key, "ratio:") {
			continue
		}
		required := "否"
		if field.Required {
			required = "是"
		}
		rows = append(rows, []string{FieldHeader(field.Key, subjects), required, fieldDescription(field.Key, subjects)})
	}
	return rows
}

func fieldDescription(key string, subjects []store.Subject) string {
	switch key {
	case FieldClassName:
		return "行政班名称，同一阶段内不能重复"
	case FieldStudentNo:
		return "按文本保存，保留前导零"
	case FieldName:
		return "学生姓名"
	case FieldTrack:
		return "物理类或者历史类"
	case FieldElective:
		return "选科组合，例如物理、化学、生物"
	case FieldStatus:
		return "在读、转学、休学、借读或者退学"
	case FieldTotalScore:
		return "入口考试总分，可以留空"
	case FieldWalking:
		return "填写是或者否"
	case FieldJapanese:
		return "日语考生填写是"
	case FieldTeacherName:
		return "教师姓名"
	case FieldTeacherNo:
		return "教师工号，可以为空"
	case FieldSubject:
		return "学科名称，必须是设置中已经存在的学科"
	}
	if strings.HasPrefix(key, "subject:") {
		if subject := subjectByKey(subjects, strings.TrimPrefix(key, "subject:")); subject != nil {
			return subject.Name + "入口均分"
		}
	}
	return ""
}

// FormatNumber 按模板要求把分数写成文本。
func FormatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
