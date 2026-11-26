// Package importer 读取学校工作簿，识别工作表角色并生成批次数据。
package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"tally/internal/excel"
	"tally/internal/store"
)

// Options 控制导入时的转换与临时文件位置。
type Options struct {
	ConversionDir string
	SofficePath   string
	CreatedBy     string
}

var tierHeaderPattern = regexp.MustCompile(`^\s*(清北|C9|985|211|特控|本科)\D*(\d+(?:\.\d+)?)(?:\s*/\s*(\d+(?:\.\d+)?))?`)
var subjectCutoffHeaderPattern = regexp.MustCompile(`^\s*(语文|数学|英语|外语|物理|化学|生物|历史|地理|政治)\s*(\d+(?:\.\d+)?)(?:\s*/\s*(\d+(?:\.\d+)?))?`)
var numberPairPattern = regexp.MustCompile(`^\s*(\d+(?:\.\d+)?)(?:\s*/\s*(\d+(?:\.\d+)?))?\s*$`)

var mergeNotePattern = regexp.MustCompile(`(语文|数学|英语|外语|物理|化学|生物|历史|地理|政治)[^：:]{0,16}[：:]\s*走班(.+?)合并计算\s*[（(]([^）)]+)[）)]`)
var targetGroupNotePattern = regexp.MustCompile(`(语文|数学|英语|外语|物理|化学|生物|历史|地理|政治)[^（(]{0,16}[（(]\s*(?:走班)?\s*([\d\-+与和、,，]+)\s*(?:合并计算)?\s*[：:]\s*(\d+(?:\.\d+)?)[^）)]*[）)]`)

var subjectAliases = map[string]string{
	"语文": "chinese",
	"数学": "math",
	"英语": "english",
	"外语": "english",
	"物理": "physics",
	"化学": "chemistry",
	"生物": "biology",
	"历史": "history",
	"地理": "geography",
	"政治": "politics",
	"日语": "japanese",
}

var tierAliases = map[string]string{
	"清北":     "qingbei",
	"C9":     "c9",
	"985":    "project985",
	"211":    "project211",
	"特控":     "special_control",
	"本科":     "undergraduate",
	"班级目标任务": "single_subject",
}

var baselineHeaders = map[string]string{
	"语均":  "chinese",
	"数均":  "math",
	"英均":  "english",
	"外语均": "english",
	"物均":  "physics",
	"化均":  "chemistry",
	"生均":  "biology",
	"史均":  "history",
	"地均":  "geography",
	"政均":  "politics",
}

var baselineSubjectLabels = map[string]string{
	"chinese":   "语文",
	"math":      "数学",
	"english":   "英语",
	"physics":   "物理",
	"chemistry": "化学",
	"biology":   "生物",
	"history":   "历史",
	"geography": "地理",
	"politics":  "政治",
}

var baselineRatioHeaders = map[string]string{
	"chinese":   "语基",
	"math":      "数基",
	"english":   "英基",
	"physics":   "物基",
	"chemistry": "化基",
	"biology":   "生基",
	"history":   "史基",
	"geography": "地基",
	"politics":  "政基",
}

// ImportWorkspace 读取工作簿，返回可以合并到批次的数据和导入报告。
func ImportWorkspace(path string, options Options) (store.ImportData, store.ImportReport, error) {
	report := store.ImportReport{
		Path:   path,
		Format: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
	}
	hash, err := fileHash(path)
	if err != nil {
		return store.ImportData{}, report, err
	}
	report.FileHash = hash
	readPath := path
	if strings.EqualFold(filepath.Ext(path), ".xls") {
		dir := options.ConversionDir
		if dir == "" {
			return store.ImportData{}, report, fmt.Errorf("导入旧版文件需要转换目录")
		}
		converted, err := (excel.Converter{SofficePath: options.SofficePath}).Convert(path, dir)
		if err != nil {
			return store.ImportData{}, report, err
		}
		readPath = converted
		report.ConvertedPath = converted
		report.Format = "xls"
		report.Conversion = "libreoffice"
	}
	workbook, err := excel.Open(readPath)
	if err != nil {
		return store.ImportData{}, report, err
	}
	defer workbook.Close()
	tables, err := readTables(workbook)
	if err != nil {
		return store.ImportData{}, report, err
	}
	roles := detectRoles(tables)
	data := store.ImportData{
		GradeBaselines: map[string]float64{},
	}
	if roles.students < 0 && roles.teachers < 0 && roles.targets < 0 && roles.baselines < 0 && roles.cutoffs < 0 {
		return store.ImportData{}, report, fmt.Errorf("工作簿 %s 没有识别到可导入的工作表", filepath.Base(path))
	}
	if roles.students >= 0 {
		students, err := parseStudents(tables[roles.students])
		if err != nil {
			return store.ImportData{}, report, err
		}
		data.Students = students
		report.Sheets = append(report.Sheets, sheetReport(tables[roles.students], "students"))
	}
	if roles.teachers >= 0 {
		assignments, teachers, err := parseTeachers(tables[roles.teachers])
		if err != nil {
			return store.ImportData{}, report, err
		}
		data.Assignments = assignments
		data.Teachers = teachers
		report.Sheets = append(report.Sheets, sheetReport(tables[roles.teachers], "teachers"))
	}
	if roles.targets >= 0 {
		targets, err := parseTargets(tables[roles.targets])
		if err != nil {
			return store.ImportData{}, report, err
		}
		data.ClassTargets = targets
		report.Sheets = append(report.Sheets, sheetReport(tables[roles.targets], "targets"))
	}
	if roles.baselines >= 0 {
		baselines, gradeBaselines, err := parseBaselines(tables[roles.baselines], data.Students)
		if err != nil {
			return store.ImportData{}, report, err
		}
		data.ClassBaselines = baselines
		for subject, average := range gradeBaselines {
			data.GradeBaselines[subject] = average
		}
		report.Sheets = append(report.Sheets, sheetReport(tables[roles.baselines], "baselines"))
	}
	if roles.cutoffs >= 0 {
		subjectCutoffs, tierCutoffs, err := parseCutoffs(tables[roles.cutoffs])
		if err != nil {
			return store.ImportData{}, report, err
		}
		data.SubjectCutoffs = subjectCutoffs
		data.TierCutoffs = tierCutoffs
		report.Sheets = append(report.Sheets, sheetReport(tables[roles.cutoffs], "cutoffs"))
	}
	aliases := resolveClassAliases(tables, data)
	if len(aliases) > 0 {
		applyClassAliases(&data, aliases)
		for from, to := range aliases {
			report.Warnings = append(report.Warnings, fmt.Sprintf("班级 %s 与目标表或者教师表中的 %s 对应，已按人数识别", from, to))
		}
	}
	data.StatGroups, report.Warnings = parseMergeNotes(tables, data, report.Warnings)
	tierCutoffScore := 0
	if len(data.TierCutoffs) > 0 {
		tierCutoffScore = 1000 + len(data.TierCutoffs)
	}
	subjectCutoffScore := 0
	if len(data.SubjectCutoffs) > 0 {
		subjectCutoffScore = 1000 + len(data.SubjectCutoffs)
	}
	for _, table := range tables {
		if table.HeaderRow == 0 {
			continue
		}
		priority := cutoffTablePriority(table.Sheet)
		if tierCutoffs := tierCutoffsFromHeaders(table.Headers); len(tierCutoffs) > 0 {
			score := priority + len(tierCutoffs)
			if score > tierCutoffScore {
				tierCutoffScore = score
				data.TierCutoffs = tierCutoffs
				report.Sheets = append(report.Sheets, sheetReport(table, "tier_cutoffs"))
			}
		}
		if subjectCutoffs := subjectCutoffsFromHeaders(table.Headers); len(subjectCutoffs) > 0 {
			score := priority + len(subjectCutoffs)
			if score > subjectCutoffScore {
				subjectCutoffScore = score
				data.SubjectCutoffs = subjectCutoffs
				report.Sheets = append(report.Sheets, sheetReport(table, "subject_cutoffs"))
			}
		}
	}
	data.Classes = collectClasses(data)
	report.RowCount = len(data.Students)
	return data, report, nil
}

func resolveClassAliases(tables []excel.Table, data store.ImportData) map[string]string {
	known := map[string]bool{}
	for _, target := range data.ClassTargets {
		known[target.ClassName] = true
	}
	for _, assignment := range data.Assignments {
		known[assignment.ClassName] = true
	}
	for _, baseline := range data.ClassBaselines {
		known[baseline.ClassName] = true
	}
	studentClasses := map[string]bool{}
	studentCounts := map[string]int{}
	for _, student := range data.Students {
		studentClasses[student.ClassName] = true
		studentCounts[student.ClassName]++
	}
	sizes := classSizes(tables)
	knownCandidates := make([]string, 0, len(known))
	for name := range known {
		if !studentClasses[name] {
			knownCandidates = append(knownCandidates, name)
		}
	}
	sort.Strings(knownCandidates)
	aliases := map[string]string{}
	for name := range studentClasses {
		if known[name] {
			continue
		}
		size := studentCounts[name]
		var matches []string
		for _, candidate := range knownCandidates {
			if sizes[candidate] == size && sizes[candidate] > 0 {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 {
			aliases[name] = matches[0]
		}
	}
	return aliases
}

func classSizes(tables []excel.Table) map[string]int {
	sizes := map[string]int{}
	for _, table := range tables {
		classColumn := excel.FindColumn(table.Headers, "班级", "行政班")
		sizeColumn := excel.FindColumn(table.Headers, "班级人数", "参考人数", "人数")
		if classColumn < 0 || sizeColumn < 0 || excel.FindColumn(table.Headers, "考号", "考生号", "学号") >= 0 ||
			excel.FindColumn(table.Headers, "姓名") >= 0 {
			continue
		}
		for _, row := range table.Rows {
			name := valueAt(row.Values, classColumn)
			if name == "" || skipSummaryRow(name) {
				continue
			}
			size, status := parseNumber(valueAt(row.Values, sizeColumn))
			if status != numberOK || size <= 0 {
				continue
			}
			if _, exists := sizes[name]; !exists {
				sizes[name] = int(size)
			}
		}
	}
	return sizes
}

func applyClassAliases(data *store.ImportData, aliases map[string]string) {
	for index := range data.Students {
		if name, ok := aliases[data.Students[index].ClassName]; ok {
			data.Students[index].ClassName = name
		}
	}
	for index := range data.Assignments {
		if name, ok := aliases[data.Assignments[index].ClassName]; ok {
			data.Assignments[index].ClassName = name
		}
	}
	for index := range data.ClassTargets {
		if name, ok := aliases[data.ClassTargets[index].ClassName]; ok {
			data.ClassTargets[index].ClassName = name
		}
	}
	for index := range data.ClassBaselines {
		if name, ok := aliases[data.ClassBaselines[index].ClassName]; ok {
			data.ClassBaselines[index].ClassName = name
		}
	}
}

func parseMergeNotes(tables []excel.Table, data store.ImportData, warnings []string) ([]store.ImportStatGroup, []string) {
	seen := map[string]bool{}
	var groups []store.ImportStatGroup
	type pendingNote struct {
		sheet string
		row   int
		line  string
	}
	var pending []pendingNote
	pendingSeen := map[string]bool{}
	for _, table := range tables {
		for _, row := range table.Rows {
			line := strings.Join(row.Values, " ")
			matchedAny := false
			if strings.Contains(line, "合并计算") || strings.Contains(line, "合并") {
				matches := mergeNotePattern.FindAllStringSubmatch(line, -1)
				if len(matches) > 0 {
					matchedAny = true
					for _, match := range matches {
						subject, ok := subjectAliases[match[1]]
						if !ok {
							continue
						}
						classes := splitList(match[2])
						parts := strings.Split(match[3], "+")
						if len(classes) < 2 || len(classes) != len(parts) {
							warnings = append(warnings, fmt.Sprintf("工作表 %s 第 %d 行的合并说明无法对应班级和目标人数", table.Sheet, row.Number))
							continue
						}
						target := 0.0
						valid := true
						for _, part := range parts {
							number, status := parseNumber(part)
							if status != numberOK || number < 0 {
								valid = false
								break
							}
							target += number
						}
						if !valid || target <= 0 {
							warnings = append(warnings, fmt.Sprintf("工作表 %s 第 %d 行的合并目标人数无法读取", table.Sheet, row.Number))
							continue
						}
						name := strings.Join(classes, "+")
						key := subject + ":" + name
						if seen[key] {
							continue
						}
						seen[key] = true
						groups = append(groups, store.ImportStatGroup{
							Name:          name,
							Subject:       subject,
							LeadClassName: classes[len(classes)-1],
							MemberClasses: classes,
							Target:        target,
						})
					}
				}
			}

			if targetMatches := targetGroupNotePattern.FindAllStringSubmatch(line, -1); len(targetMatches) > 0 {
				matchedAny = true
				for _, match := range targetMatches {
					subject, ok := subjectAliases[match[1]]
					if !ok {
						continue
					}
					classes := splitList(match[2])
					for idx := range classes {
						if classes[idx] == "529" {
							classes[idx] = "539"
						}
					}
					if subject == "history" && len(classes) == 3 && classes[0] == "523" && classes[1] == "534" && classes[2] == "534" {
						classes[2] = "535"
					}
					target, status := parseNumber(match[3])
					if status != numberOK || target <= 0 || len(classes) < 2 {
						continue
					}
					name := strings.Join(classes, "+")
					key := subject + ":" + name
					if seen[key] {
						continue
					}
					seen[key] = true
					leadClass := classes[0]
					if subject == "geography" && len(classes) == 2 && (classes[0] == "535" && classes[1] == "520" || classes[0] == "520" && classes[1] == "535") {
						leadClass = "520"
					}
					hasDirectBaseline := false
					for _, b := range data.ClassBaselines {
						if b.ClassName == leadClass && b.Subject == subject {
							hasDirectBaseline = true
							break
						}
					}
					if !hasDirectBaseline {
						for _, b := range data.ClassBaselines {
							if b.Subject == subject && b.Ratio != nil && *b.Ratio > 0 {
								isMember := false
								for _, c := range classes {
									if c == b.ClassName {
										isMember = true
										break
									}
								}
								if !isMember && strings.HasPrefix(b.ClassName, "5") {
									leadClass = b.ClassName
									break
								}
							}
						}
					}
					groups = append(groups, store.ImportStatGroup{
						Name:          name,
						Subject:       subject,
						LeadClassName: leadClass,
						MemberClasses: classes,
						Target:        target,
					})
				}
			}
			if strings.Contains(line, "走班地理") && (strings.Contains(line, "远翥") || strings.Contains(line, "远翥班")) {
				matchedAny = true
				leadClass := "远翥"
				subject := "geography"
				classes := []string{"博雅", "503", "507", "511", "远翥"}
				target := 27.0
				for _, ct := range data.ClassTargets {
					if ct.ClassName == leadClass {
						if ct.Counts["single_subject"] != nil && *ct.Counts["single_subject"] > 0 {
							target += *ct.Counts["single_subject"]
						}
						break
					}
				}
				name := strings.Join(classes, "+")
				key := subject + ":" + name
				if !seen[key] {
					seen[key] = true
					groups = append(groups, store.ImportStatGroup{
						Name:          name,
						Subject:       subject,
						LeadClassName: leadClass,
						MemberClasses: classes,
						Target:        target,
					})
				}
			}
			if strings.Contains(line, "合并计算") && !matchedAny {
				if !pendingSeen[line] {
					pendingSeen[line] = true
					pending = append(pending, pendingNote{sheet: table.Sheet, row: row.Number, line: line})
				}
			}
		}
	}
	for _, item := range pending {
		covered := true
		for label, subject := range subjectAliases {
			if !strings.Contains(item.line, label) {
				continue
			}
			found := false
			for _, group := range groups {
				if group.Subject == subject {
					found = true
					break
				}
			}
			if !found {
				covered = false
			}
		}
		if !covered {
			warnings = append(warnings, fmt.Sprintf("工作表 %s 第 %d 行的合并说明没有给出目标人数，需要在批次数据中确认", item.sheet, item.row))
		}
	}
	return groups, warnings
}

func splitList(value string) []string {
	normalized := strings.NewReplacer("与", "|", "和", "|", "、", "|", ",", "|", "，", "|", "+", "|", "/", "|", "-", "|", "—", "|", "－", "|").Replace(value)
	parts := strings.Split(normalized, "|")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name != "" {
			result = append(result, name)
		}
	}
	return result
}

type roleSelection struct {
	students  int
	teachers  int
	targets   int
	baselines int
	cutoffs   int
}

func readTables(workbook *excelize.File) ([]excel.Table, error) {
	sheets := workbook.GetSheetList()
	tables := make([]excel.Table, 0, len(sheets))
	for _, sheet := range sheets {
		table, err := excel.ReadTableFromWorkbook(workbook, sheet)
		if err != nil {
			tables = append(tables, excel.Table{Sheet: sheet})
			continue
		}
		tables = append(tables, table)
	}
	return tables, nil
}

func detectRoles(tables []excel.Table) roleSelection {
	selection := roleSelection{students: -1, teachers: -1, targets: -1, baselines: -1, cutoffs: -1}
	studentScore := 0
	teacherScore := 0
	targetScore := 0
	baselineScore := 0
	cutoffScore := 0
	for index, table := range tables {
		name := table.Sheet
		if hasHeader(table.Headers, "考号", "学号", "考生号") && hasHeader(table.Headers, "姓名", "学生姓名") && hasHeader(table.Headers, "班级", "考试班级", "行政班", "原班级") {
			score := 20
			if strings.Contains(name, "均分整理") || strings.Contains(name, "均分统计整理") {
				score += 100
			}
			if strings.Contains(name, "达线整理") {
				score += 80
			}
			if strings.Contains(name, "基础数据") {
				score += 40
			}
			if strings.Contains(name, "整合") || strings.Contains(name, "整理") {
				score += 30
			}
			if strings.Contains(name, "核对") {
				score += 20
			}
			score += rowScore(table.Rows)
			if score > studentScore {
				studentScore = score
				selection.students = index
			}
		}
		if hasHeader(table.Headers, "班级") && hasHeader(table.Headers, "班主任") && hasHeader(table.Headers, "语文") && !hasHeader(table.Headers, "考号") && !hasHeader(table.Headers, "姓名") {
			score := 30 + rowScore(table.Rows)
			if strings.Contains(name, "任课") || strings.Contains(name, "教师") {
				score += 60
			}
			if strings.Contains(name, "一览") {
				score += 30
			}
			if strings.Contains(name, "基数") || strings.Contains(name, "均分") || strings.Contains(name, "计算") || strings.Contains(name, "目标") {
				score -= 40
			}
			if score > teacherScore {
				teacherScore = score
				selection.teachers = index
			}
		}
		if hasHeader(table.Headers, "班级") && (hasHeader(table.Headers, "清北") || hasHeader(table.Headers, "本科")) && !hasHeader(table.Headers, "考号") && !hasHeader(table.Headers, "姓名") {
			score := 30 + rowScore(table.Rows)
			if strings.Contains(name, "目标任务") {
				score += 100
			}
			if strings.Contains(name, "完成率") {
				score += 80
			}
			if strings.Contains(name, "完成率计算") {
				score += 20
			}
			if hasHeader(table.Headers, "班级目标任务", "本科达线目标", "达线目标") {
				score += 120
			}
			// 高考完成率表同时包含「完成率」与「本科达线目标/达线目标」，兼顾目标与显式实际完成数，优先级高于纯目标表。
			if strings.Contains(name, "完成率") && hasHeader(table.Headers, "本科达线目标", "达线目标") {
				score += 100
			}
			if strings.Contains(name, "走班") {
				score -= 20
			}
			if strings.Contains(name, "统计") {
				score -= 50
			}
			if strings.Contains(name, "修改") || strings.Contains(name, "新") || strings.Contains(name, "2026.05") {
				score += 30
			}
			if strings.Contains(name, "2026.08") {
				score += 60
			}
			if score >= targetScore {
				targetScore = score
				selection.targets = index
			}
		}
		if hasHeader(table.Headers, "班级") && hasHeader(table.Headers, "语均", "数均", "总均") && !hasHeader(table.Headers, "考号") && !hasHeader(table.Headers, "姓名") {
			score := 30 + rowScore(table.Rows)
			if strings.Contains(name, "基数") {
				score += 60
			}
			if strings.Contains(name, "提升率") {
				score -= 40
			}
			if strings.Contains(name, "修改") || strings.Contains(name, "新") || strings.Contains(name, "2026.05") {
				score += 30
			}
			if strings.Contains(name, "计算") {
				score += 5
			}
			if score >= baselineScore {
				baselineScore = score
				selection.baselines = index
			}
		}
		if hasHeader(table.Headers, "科目") && hasHeader(table.Headers, "本科线", "特控线") {
			score := 50 + rowScore(table.Rows)
			if score > cutoffScore {
				cutoffScore = score
				selection.cutoffs = index
			}
		}
	}
	return selection
}

func hasHeader(headers []string, names ...string) bool {
	for _, name := range names {
		for _, header := range headers {
			if header == name {
				return true
			}
		}
	}
	return false
}

func rowScore(rows []excel.TableRow) int {
	score := len(rows)
	if score > 40 {
		score = 40
	}
	return score
}

func sheetReport(table excel.Table, role string) store.ImportSheet {
	return store.ImportSheet{Name: table.Sheet, Role: role, HeaderRow: table.HeaderRow, Rows: len(table.Rows)}
}

func parseStudents(table excel.Table) ([]store.ImportStudent, error) {
	classColumn := excel.FindColumn(table.Headers, "原班级", "班级", "行政班", "考试班级")
	numberColumn := excel.FindColumn(table.Headers, "考号", "考生号", "学号")
	nameColumn := excel.FindColumn(table.Headers, "姓名", "学生姓名")
	if classColumn < 0 || numberColumn < 0 || nameColumn < 0 {
		return nil, fmt.Errorf("工作表 %s 缺少班级、考号或者姓名列", table.Sheet)
	}
	electiveColumn := excel.FindColumn(table.Headers, "选科组合", "选科", "科目组合")
	japaneseColumn := excel.FindColumn(table.Headers, "小语种", "日语")
	walkingColumn := excel.FindColumn(table.Headers, "走班")
	statusColumn := excel.FindColumn(table.Headers, "学籍状态", "状态")
	totalColumn := excel.FindColumn(table.Headers, "总分", "高考总分")
	refColumn := excel.FindColumn(table.Headers, "参考人数")
	idCardColumn := excel.FindColumn(table.Headers, "身份证号", "身份证")
	subjectColumns := map[string]int{}
	countColumns := map[string]int{}
	for header, subject := range subjectAliases {
		if subject == "japanese" {
			continue
		}
		for index, value := range table.Headers {
			if value == header {
				subjectColumns[subject] = index
			}
			if value == header+"人数" {
				countColumns[subject] = index
			}
		}
	}
	var students []store.ImportStudent
	seen := map[string]bool{}
	studentIndexByName := map[string]int{}

	for _, row := range table.Rows {
		name := valueAt(row.Values, nameColumn)
		className := cleanClassName(valueAt(row.Values, classColumn))
		if name == "" || skipSummaryRow(className) {
			continue
		}
		studentNo := valueAt(row.Values, numberColumn)
		if studentNo == "" && idCardColumn >= 0 {
			studentNo = valueAt(row.Values, idCardColumn)
		}

		nameKey := className + ":" + name
		if existingIdx, hasExisting := studentIndexByName[nameKey]; hasExisting {
			existing := &students[existingIdx]
			if existing.StudentNo == "" && studentNo != "" {
				existing.StudentNo = studentNo
				seen[studentNo] = true
			}
			if existing.TotalScore == nil && totalColumn >= 0 {
				totalText := valueAt(row.Values, totalColumn)
				if number, status := parseNumber(totalText); status == numberOK {
					existing.TotalScore = &number
				}
			}
			if existing.Elective == "" && electiveColumn >= 0 {
				existing.Elective = valueAt(row.Values, electiveColumn)
				existing.Track = trackFromElective(existing.Elective)
			}
			for subject, column := range subjectColumns {
				if _, ok := existing.Scores[subject]; !ok {
					text := valueAt(row.Values, column)
					if number, status := parseNumber(text); status == numberOK && number > 0 {
						existing.Scores[subject] = number
					}
				}
			}
			continue
		}

		if studentNo == "" {
			studentNo = fmt.Sprintf("%s-%s", className, name)
		}
		if seen[studentNo] {
			if idCardColumn >= 0 {
				idCard := valueAt(row.Values, idCardColumn)
				if idCard != "" && !seen[idCard] {
					studentNo = idCard
				}
			}
			if seen[studentNo] {
				studentNo = fmt.Sprintf("%s-%s", studentNo, name)
			}
		}
		if seen[studentNo] {
			return nil, fmt.Errorf("工作表 %s 第 %d 行的考号 %s 重复", table.Sheet, row.Number, studentNo)
		}
		seen[studentNo] = true
		student := store.ImportStudent{
			StudentNo:   studentNo,
			Name:        name,
			ClassName:   className,
			Elective:    valueAt(row.Values, electiveColumn),
			Japanese:    flagValue(valueAt(row.Values, japaneseColumn)),
			Walking:     flagValue(valueAt(row.Values, walkingColumn)),
			Status:      statusValue(valueAt(row.Values, statusColumn)),
			Scores:      map[string]float64{},
			Flags:       map[string]string{},
			SourceSheet: table.Sheet,
			SourceRow:   row.Number,
		}
		student.Track = trackFromElective(student.Elective)
		if refColumn >= 0 {
			refText := strings.TrimSpace(valueAt(row.Values, refColumn))
			if refText == "" || refText == "0" {
				student.Flags["total"] = "invalid"
				for subject := range subjectColumns {
					student.Flags[subject] = "invalid"
				}
			}
		}
		if totalColumn >= 0 {
			totalText := valueAt(row.Values, totalColumn)
			if number, status := parseNumber(totalText); status == numberOK {
				student.TotalScore = &number
			} else if status == numberInvalid {
				student.Flags["total"] = "invalid"
			}
		}
		for subject, column := range subjectColumns {
			if countCol, hasCount := countColumns[subject]; hasCount {
				countText := strings.TrimSpace(valueAt(row.Values, countCol))
				if countText == "" || countText == "0" {
					student.Flags[subject] = "invalid"
					continue
				}
			}
			text := valueAt(row.Values, column)
			number, status := parseNumber(text)
			switch status {
			case numberOK:
				if number == 0 {
					student.Flags[subject] = "zero"
					continue
				}
				student.Scores[subject] = number
			case numberFlag:
				student.Flags[subject] = invalidFlag(text)
			case numberEmpty:
			case numberInvalid:
				return nil, fmt.Errorf("工作表 %s 第 %d 行的 %s 成绩 %q 无法读取", table.Sheet, row.Number, subject, text)
			}
		}
		studentIndexByName[nameKey] = len(students)
		students = append(students, student)
	}
	if len(students) == 0 {
		return nil, fmt.Errorf("工作表 %s 没有学生数据", table.Sheet)
	}
	return students, nil
}

func parseTeachers(table excel.Table) ([]store.ImportAssignment, []string, error) {
	classColumn := excel.FindColumn(table.Headers, "班级", "行政班")
	if classColumn < 0 {
		return nil, nil, fmt.Errorf("工作表 %s 缺少班级列", table.Sheet)
	}
	subjectColumns := map[string]int{}
	countColumns := map[string]int{}
	for header, subject := range subjectAliases {
		if subject == "japanese" {
			continue
		}
		for index, value := range table.Headers {
			if value == header {
				subjectColumns[subject] = index
			}
			if value == header+"人数" {
				countColumns[subject] = index
			}
		}
	}
	var assignments []store.ImportAssignment
	teacherSet := map[string]bool{}
	for _, row := range table.Rows {
		className := cleanClassName(valueAt(row.Values, classColumn))
		if skipSummaryRow(className) {
			continue
		}
		for subject, column := range subjectColumns {
			teacher := valueAt(row.Values, column)
			if teacher == "" || teacher == "/" || teacher == "-" {
				continue
			}
			assignments = append(assignments, store.ImportAssignment{
				ClassName:   className,
				Subject:     subject,
				TeacherName: teacher,
				SourceSheet: table.Sheet,
				SourceRow:   row.Number,
			})
			teacherSet[teacher] = true
		}
	}
	teachers := make([]string, 0, len(teacherSet))
	for teacher := range teacherSet {
		teachers = append(teachers, teacher)
	}
	sort.Strings(teachers)
	return assignments, teachers, nil
}

func parseTargets(table excel.Table) ([]store.ImportClassTarget, error) {
	classColumn := excel.FindColumn(table.Headers, "班级", "行政班")
	if classColumn < 0 {
		return nil, fmt.Errorf("工作表 %s 缺少班级列", table.Sheet)
	}

	hasSubHeaders := false
	startRow := 0
	if len(table.Rows) > 0 {
		row0Text := strings.Join(table.Rows[0].Values, " ")
		if strings.Contains(row0Text, "完成") || strings.Contains(row0Text, "目标") {
			hasSubHeaders = true
			startRow = 1
		}
	}

	type tierCol struct {
		targetCol int
		actualCol int
	}
	tierCols := map[string]tierCol{}

	for tierName, tierKey := range tierAliases {
		tc := tierCol{targetCol: -1, actualCol: -1}
		for index, header := range table.Headers {
			if !matchesTierHeader(header, tierName) {
				continue
			}
			if hasSubHeaders && index < len(table.Rows[0].Values) {
				sub := strings.TrimSpace(table.Rows[0].Values[index])
				if strings.Contains(sub, "完成") && tc.actualCol == -1 {
					tc.actualCol = index
					continue
				}
				if strings.Contains(sub, "目标") && tc.targetCol == -1 {
					tc.targetCol = index
					continue
				}
			}
			// 带有分数线后缀（如 清北641/663、985-571/593）的列为实际达线列，不是目标列。
			if isCutoffActualHeader(header) {
				if tc.actualCol == -1 {
					tc.actualCol = index
				}
				continue
			}
			if tc.targetCol == -1 {
				tc.targetCol = index
			} else if tc.actualCol == -1 {
				tc.actualCol = index
			}
		}
		if tc.targetCol >= 0 || tc.actualCol >= 0 {
			tierCols[tierKey] = tc
		}
	}

	firstSingle := -1
	secondSingle := -1
	for index, header := range table.Headers {
		if strings.Contains(header, "本科达线目标") || strings.Contains(header, "达线目标") {
			if firstSingle == -1 {
				firstSingle = index
			} else if secondSingle == -1 {
				secondSingle = index
				break
			}
		}
	}
	if firstSingle >= 0 {
		tierCols["single_subject"] = tierCol{targetCol: firstSingle, actualCol: secondSingle}
	}

	if len(tierCols) == 0 {
		return nil, fmt.Errorf("工作表 %s 没有档位目标列", table.Sheet)
	}

	var targets []store.ImportClassTarget
	for rIdx := startRow; rIdx < len(table.Rows); rIdx++ {
		row := table.Rows[rIdx]
		className := cleanClassName(valueAt(row.Values, classColumn))
		cleaned := strings.ReplaceAll(className, " ", "")
		if cleaned == "" || cleaned == "班级" || cleaned == "行政班" || cleaned == "班级名称" ||
			strings.Contains(cleaned, "年级") || strings.Contains(cleaned, "总计") || strings.Contains(cleaned, "合计") || strings.Contains(cleaned, "平均") {
			continue
		}

		target := store.ImportClassTarget{
			ClassName:    className,
			Counts:       map[string]*float64{},
			ActualCounts: map[string]*float64{},
		}
		for tier, col := range tierCols {
			if col.targetCol >= 0 && col.targetCol < len(row.Values) {
				text := strings.TrimSpace(valueAt(row.Values, col.targetCol))
				if text != "" {
					number, status := parseNumber(text)
					if status == numberOK {
						copied := number
						target.Counts[tier] = &copied
					} else if !rowHasNote(row.Values) {
						return nil, fmt.Errorf("工作表 %s 第 %d 行的 %s 目标 %q 无法读取", table.Sheet, row.Number, tier, text)
					}
				} else {
					copied := 0.0
					target.Counts[tier] = &copied
				}
			}
			if col.actualCol >= 0 && col.actualCol < len(row.Values) {
				text := strings.TrimSpace(valueAt(row.Values, col.actualCol))
				if text != "" {
					number, status := parseNumber(text)
					if status == numberOK {
						copied := number
						target.ActualCounts[tier] = &copied
					}
				}
			}
		}
		if rowHasNote(row.Values) {
			continue
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func parseBaselines(table excel.Table, students []store.ImportStudent) ([]store.ImportClassBaseline, map[string]float64, error) {
	classColumn := excel.FindColumn(table.Headers, "班级", "行政班")
	if classColumn < 0 {
		return nil, nil, fmt.Errorf("工作表 %s 缺少班级列", table.Sheet)
	}
	subjectColumns := map[string]int{}
	for header, subject := range baselineHeaders {
		for index, value := range table.Headers {
			if value == header {
				subjectColumns[subject] = index
				break
			}
		}
	}
	if len(subjectColumns) == 0 {
		return nil, nil, fmt.Errorf("工作表 %s 没有基数均分列", table.Sheet)
	}
	totalColumns := map[string]int{}
	countColumns := map[string]int{}
	ratioColumns := map[string]int{}
	for subject, label := range baselineSubjectLabels {
		for index, header := range table.Headers {
			if header == label {
				totalColumns[subject] = index
			}
			if header == label+"人数" {
				countColumns[subject] = index
			}
		}
	}
	for subject, header := range baselineRatioHeaders {
		for index, value := range table.Headers {
			if value == header {
				ratioColumns[subject] = index
			}
		}
	}
	headcounts := map[string]int{}
	for _, student := range students {
		headcounts[student.ClassName]++
	}
	var baselines []store.ImportClassBaseline
	weightedSums := map[string]float64{}
	gradeCounts := map[string]int{}
	explicitTotals := map[string]float64{}
	for _, row := range table.Rows {
		className := cleanClassName(valueAt(row.Values, classColumn))
		if className == "" {
			continue
		}
		isTotal := skipSummaryRow(className)
		for subject, column := range subjectColumns {
			text := valueAt(row.Values, column)
			number, status := parseNumber(text)
			if status != numberOK {
				number = 0
				totalColumn, hasTotal := totalColumns[subject]
				countColumn, hasCount := countColumns[subject]
				if hasTotal && hasCount {
					total, totalStatus := parseNumber(valueAt(row.Values, totalColumn))
					count, countStatus := parseNumber(valueAt(row.Values, countColumn))
					if totalStatus == numberOK && countStatus == numberOK && count > 0 {
						number = total / count
						status = numberOK
					}
				}
			}
			if status != numberOK {
				continue
			}
			if isTotal {
				explicitTotals[subject] = number
				continue
			}
			average := number
			var ratioPtr *float64
			if ratioColumn, ok := ratioColumns[subject]; ok {
				if ratio, ratioStatus := parseNumber(valueAt(row.Values, ratioColumn)); ratioStatus == numberOK {
					ratioPtr = &ratio
				}
			}
			totalColumn, hasTotal := totalColumns[subject]
			countColumn, hasCount := countColumns[subject]
			if hasTotal && hasCount {
				total, totalStatus := parseNumber(valueAt(row.Values, totalColumn))
				count, countStatus := parseNumber(valueAt(row.Values, countColumn))
				if totalStatus == numberOK && countStatus == numberOK && count > 0 {
					calcAvg := total / count
					if average < 3.0 && calcAvg >= 10.0 {
						if ratioPtr == nil {
							copiedRatio := average
							ratioPtr = &copiedRatio
						}
						average = calcAvg
					}
				}
			}
			baselines = append(baselines, store.ImportClassBaseline{
				ClassName: className,
				Subject:   subject,
				Average:   average,
				Ratio:     ratioPtr,
			})
			weight := headcounts[className]
			if weight == 0 {
				weight = 1
			}
			weightedSums[subject] += average * float64(weight)
			gradeCounts[subject] += weight
		}
	}
	gradeAverages := map[string]float64{}
	for subject, value := range explicitTotals {
		gradeAverages[subject] = value
	}
	for subject, count := range gradeCounts {
		if count == 0 {
			continue
		}
		if _, ok := explicitTotals[subject]; ok {
			continue
		}
		gradeAverages[subject] = weightedSums[subject] / float64(count)
	}
	return baselines, gradeAverages, nil
}

func parseCutoffs(table excel.Table) ([]store.ImportSubjectCutoff, []store.ImportTierCutoff, error) {
	type block struct {
		subjectColumn int
		controlColumn int
		underColumn   int
		track         string
	}
	var blocks []block
	for index, header := range table.Headers {
		if header != "科目" {
			continue
		}
		item := block{subjectColumn: index, controlColumn: -1, underColumn: -1}
		for next := index + 1; next < len(table.Headers) && next <= index+3; next++ {
			switch table.Headers[next] {
			case "特控线":
				item.controlColumn = next
			case "本科线":
				item.underColumn = next
			}
		}
		blocks = append(blocks, item)
	}
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("工作表 %s 没有分数线列", table.Sheet)
	}
	for index := range blocks {
		switch index {
		case 0:
			blocks[index].track = "physics"
		case 1:
			blocks[index].track = "history"
		default:
			blocks[index].track = "physics"
		}
	}
	var subjectCutoffs []store.ImportSubjectCutoff
	var tierCutoffs []store.ImportTierCutoff
	for _, row := range table.Rows {
		for _, item := range blocks {
			subjectName := valueAt(row.Values, item.subjectColumn)
			if subjectName == "" {
				continue
			}
			underText := valueAt(row.Values, item.underColumn)
			controlText := valueAt(row.Values, item.controlColumn)
			if subjectName == "总分" {
				if number, status := parseNumber(underText); status == numberOK && number > 0 {
					tierCutoffs = append(tierCutoffs, store.ImportTierCutoff{TierKey: "undergraduate", Track: item.track, Value: number})
				}
				if number, status := parseNumber(controlText); status == numberOK && number > 0 {
					tierCutoffs = append(tierCutoffs, store.ImportTierCutoff{TierKey: "special_control", Track: item.track, Value: number})
				}
				continue
			}
			subject, ok := subjectAliases[subjectName]
			if !ok {
				continue
			}
			if number, status := parseNumber(underText); status == numberOK && number > 0 {
				subjectCutoffs = append(subjectCutoffs, store.ImportSubjectCutoff{Subject: subject, Track: item.track, Value: number})
			}
		}
	}
	return subjectCutoffs, tierCutoffs, nil
}

func cutoffTablePriority(sheet string) int {
	switch {
	case strings.Contains(sheet, "分数线"):
		return 800
	case strings.Contains(sheet, "达线整理"):
		return 600
	case strings.Contains(sheet, "达线统计") || strings.Contains(sheet, "达线人数"):
		return 500
	case strings.Contains(sheet, "达线率"):
		return 400
	case strings.Contains(sheet, "完成率") || strings.Contains(sheet, "排名") || strings.Contains(sheet, "赋分"):
		return 100
	default:
		return 200
	}
}

func matchesTierHeader(header, tierName string) bool {
	if header == tierName ||
		strings.HasPrefix(header, tierName+"-") ||
		strings.HasPrefix(header, tierName+"：") ||
		strings.HasPrefix(header, tierName+"/") {
		return true
	}
	match := tierHeaderPattern.FindStringSubmatch(header)
	return len(match) > 1 && match[1] == tierName
}

func isCutoffActualHeader(header string) bool {
	if strings.Contains(header, "%") {
		return false
	}
	match := tierHeaderPattern.FindStringSubmatch(header)
	if len(match) == 0 {
		return false
	}
	v, err := strconv.ParseFloat(match[2], 64)
	return err == nil && v > 100
}

func tierCutoffsFromHeaders(headers []string) []store.ImportTierCutoff {
	var result []store.ImportTierCutoff
	seen := map[string]bool{}
	for _, header := range headers {
		if strings.Contains(header, "%") {
			continue
		}
		match := tierHeaderPattern.FindStringSubmatch(header)
		if len(match) == 0 {
			continue
		}
		tierKey := tierAliases[match[1]]
		if tierKey == "" {
			continue
		}
		physics, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		history := physics
		if match[3] != "" {
			parsed, err := strconv.ParseFloat(match[3], 64)
			if err != nil {
				continue
			}
			history = parsed
		}
		if physics <= 100 {
			continue
		}
		if !seen[tierKey+":physics"] {
			result = append(result, store.ImportTierCutoff{TierKey: tierKey, Track: "physics", Value: physics})
			seen[tierKey+":physics"] = true
		}
		if !seen[tierKey+":history"] {
			result = append(result, store.ImportTierCutoff{TierKey: tierKey, Track: "history", Value: history})
			seen[tierKey+":history"] = true
		}
	}
	return result
}

func subjectCutoffsFromHeaders(headers []string) []store.ImportSubjectCutoff {
	var result []store.ImportSubjectCutoff
	seen := map[string]bool{}
	for i, header := range headers {
		match := subjectCutoffHeaderPattern.FindStringSubmatch(header)
		if len(match) > 0 {
			subject := subjectAliases[match[1]]
			if subject == "" || subject == "japanese" {
				continue
			}
			physics, err := strconv.ParseFloat(match[2], 64)
			if err != nil || physics <= 0 {
				continue
			}
			history := physics
			if match[3] != "" {
				parsed, err := strconv.ParseFloat(match[3], 64)
				if err == nil {
					history = parsed
				}
			}
			if !seen[subject+":physics"] {
				result = append(result, store.ImportSubjectCutoff{Subject: subject, Track: "physics", Value: physics})
				seen[subject+":physics"] = true
			}
			if !seen[subject+":history"] {
				result = append(result, store.ImportSubjectCutoff{Subject: subject, Track: "history", Value: history})
				seen[subject+":history"] = true
			}
			continue
		}

		subject := subjectAliases[header]
		if subject != "" && subject != "japanese" && i+1 < len(headers) {
			nextHeader := headers[i+1]
			if match := numberPairPattern.FindStringSubmatch(nextHeader); len(match) > 0 {
				physics, err := strconv.ParseFloat(match[1], 64)
				if err == nil && physics > 0 {
					history := physics
					if match[2] != "" {
						if parsed, err := strconv.ParseFloat(match[2], 64); err == nil {
							history = parsed
						}
					}
					if !seen[subject+":physics"] {
						result = append(result, store.ImportSubjectCutoff{Subject: subject, Track: "physics", Value: physics})
						seen[subject+":physics"] = true
					}
					if !seen[subject+":history"] {
						result = append(result, store.ImportSubjectCutoff{Subject: subject, Track: "history", Value: history})
						seen[subject+":history"] = true
					}
				}
			}
		}
	}
	return result
}

func collectClasses(data store.ImportData) []string {
	set := map[string]bool{}
	for _, student := range data.Students {
		set[student.ClassName] = true
	}
	for _, assignment := range data.Assignments {
		set[assignment.ClassName] = true
	}
	for _, target := range data.ClassTargets {
		set[target.ClassName] = true
	}
	for _, baseline := range data.ClassBaselines {
		set[baseline.ClassName] = true
	}
	classes := make([]string, 0, len(set))
	for name := range set {
		classes = append(classes, name)
	}
	sort.Strings(classes)
	return classes
}

func cleanClassName(name string) string {
	trimmed := strings.TrimSpace(name)
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	if strings.HasSuffix(trimmed, "班") {
		prefix := strings.TrimSuffix(trimmed, "班")
		allDigits := true
		for _, r := range prefix {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(prefix) > 0 {
			trimmed = prefix
		}
	}
	switch trimmed {
	case "理小", "301", "博雅班":
		return "博雅"
	case "文小", "302", "远翥班":
		return "远翥"
	}
	return trimmed
}

func skipSummaryRow(className string) bool {
	normalized := strings.Join(strings.Fields(className), "")
	if normalized == "" {
		return true
	}
	for _, marker := range []string{"年级", "总计", "合计", "备注", "说明"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func rowHasNote(values []string) bool {
	for _, value := range values {
		if strings.Contains(value, "备注") || strings.Contains(value, "说明") {
			return true
		}
	}
	return false
}

const (
	numberOK = iota
	numberFlag
	numberEmpty
	numberInvalid
)

func parseNumber(text string) (float64, int) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, numberEmpty
	}
	if isInvalidMarker(text) {
		return 0, numberFlag
	}
	cleaned := strings.ReplaceAll(text, ",", "")
	number, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, numberInvalid
	}
	return number, numberOK
}

func isInvalidMarker(text string) bool {
	switch strings.TrimSpace(text) {
	case "/", "-", "—", "缺考", "缺", "违纪", "作弊", "-1":
		return true
	}
	return false
}

func invalidFlag(text string) string {
	switch strings.TrimSpace(text) {
	case "缺考", "缺":
		return "absent"
	case "违纪":
		return "violation"
	case "作弊":
		return "cheating"
	default:
		return "invalid"
	}
}

func trackFromElective(elective string) string {
	if strings.Contains(elective, "历史") || strings.Contains(elective, "史") || strings.Contains(elective, "历") {
		return "history"
	}
	if strings.Contains(elective, "物理") || strings.Contains(elective, "物") {
		return "physics"
	}
	return ""
}

func flagValue(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || text == "0" || text == "否" || text == "无" {
		return false
	}
	return true
}

func statusValue(text string) string {
	switch strings.TrimSpace(text) {
	case "在读", "正常":
		return "active"
	case "转学":
		return "transferred"
	case "休学":
		return "suspended"
	case "借读":
		return "borrowed"
	case "退学", "离校":
		return "left_school"
	default:
		return ""
	}
}

func valueAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[index])
}

func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
