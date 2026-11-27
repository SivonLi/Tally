package importer

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"tally/internal/excel"
	"tally/internal/store"
)

// 结果归档导入的向导数据项与工作表角色常量。
const (
	KindArchivedResults = "archived_results"

	ArchiveRoleQuant    = "archive_quant"
	ArchiveRoleRank     = "archive_rank"
	ArchiveRolePersonal = "archive_personal"
)

// ArchiveSheetInfo 描述结果归档工作簿中单张工作表的角色识别结果。
type ArchiveSheetInfo struct {
	Name        string
	Role        string
	Format      string
	HeaderRow   int
	RowCount    int
	Score       int
	Recommended bool
	Headers     []string
}

// ArchiveInspectResult 汇总结果归档工作簿的识别、解析结果与校验问题。
type ArchiveInspectResult struct {
	Path          string
	FileHash      string
	Format        string
	ConvertedPath string
	Conversion    string
	Sheets        []ArchiveSheetInfo
	Data          store.ArchivedResultData
	Report        store.ImportReport
	Issues        []store.ImportIssue
}

type archiveHeaderCol struct {
	Index  int
	Top    string
	Bottom string
	Label  string
}

type archiveParsedSheet struct {
	Info     ArchiveSheetInfo
	Cols     []archiveHeaderCol
	DataRows [][]string
	DataNums []int // 对应 Excel 1-based 行号
}

// InspectArchiveWorkbook 读取结果归档工作簿，按表头识别量化计算、排名赋分与个人得分工作表，
// 展开多行表头与合并单元格（左上角取值），并解析为 ArchivedResultData 与 ImportIssue。
func InspectArchiveWorkbook(path string, options Options, subjects []store.Subject) (*ArchiveInspectResult, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	hash, err := fileHash(path)
	if err != nil {
		return nil, err
	}
	res := &ArchiveInspectResult{
		Path:     path,
		FileHash: hash,
		Format:   ext,
		Report: store.ImportReport{
			FileHash: hash,
			Path:     path,
			Format:   ext,
		},
	}

	readPath := path
	if strings.EqualFold(filepath.Ext(path), ".xls") {
		if options.ConversionDir == "" {
			return nil, fmt.Errorf("导入旧版文件需要转换目录")
		}
		converted, convErr := (excel.Converter{SofficePath: options.SofficePath}).Convert(path, options.ConversionDir)
		if convErr != nil {
			return nil, convErr
		}
		readPath = converted
		res.ConvertedPath = converted
		res.Conversion = "libreoffice"
		res.Report.ConvertedPath = converted
		res.Report.Conversion = "libreoffice"
	}

	wb, err := excel.Open(readPath)
	if err != nil {
		return nil, err
	}
	defer wb.Close()

	parsedSheets, err := readArchiveSheets(wb, subjects)
	if err != nil {
		return nil, err
	}

	bestByRole := map[string]int{}
	for idx, ps := range parsedSheets {
		if ps.Info.Role == "" {
			continue
		}
		curBest, exists := bestByRole[ps.Info.Role]
		if !exists || ps.Info.Score > parsedSheets[curBest].Info.Score {
			bestByRole[ps.Info.Role] = idx
		}
	}
	if len(bestByRole) == 0 {
		return nil, fmt.Errorf("工作簿 %s 没有识别到量化计算、排名赋分或个人得分工作表", filepath.Base(path))
	}

	for _, idx := range bestByRole {
		parsedSheets[idx].Info.Recommended = true
	}
	for _, ps := range parsedSheets {
		res.Sheets = append(res.Sheets, ps.Info)
	}

	var quantRows []parsedArchiveQuantRow
	var rankRows []parsedArchiveRankRow
	var issues []store.ImportIssue

	if qIdx, ok := bestByRole[ArchiveRoleQuant]; ok {
		qSheet := parsedSheets[qIdx]
		var qIssues []store.ImportIssue
		quantRows, qIssues = parseArchiveQuantSheet(qSheet, subjects)
		issues = append(issues, qIssues...)
		res.Report.Sheets = append(res.Report.Sheets, store.ImportSheet{
			Name:      qSheet.Info.Name,
			Role:      ArchiveRoleQuant,
			HeaderRow: qSheet.Info.HeaderRow,
			Rows:      len(quantRows),
		})
	}
	if rIdx, ok := bestByRole[ArchiveRoleRank]; ok {
		rSheet := parsedSheets[rIdx]
		var rIssues []store.ImportIssue
		rankRows, rIssues = parseArchiveRankSheet(rSheet, subjects)
		issues = append(issues, rIssues...)
		res.Report.Sheets = append(res.Report.Sheets, store.ImportSheet{
			Name:      rSheet.Info.Name,
			Role:      ArchiveRoleRank,
			HeaderRow: rSheet.Info.HeaderRow,
			Rows:      len(rankRows),
		})
	}
	hasRankSheet := false
	if _, ok := bestByRole[ArchiveRoleRank]; ok {
		hasRankSheet = true
	}

	mergedRows, mergeIssues := mergeArchiveResultRows(quantRows, rankRows, hasRankSheet)
	res.Data.Rows = mergedRows
	issues = append(issues, mergeIssues...)

	if pIdx, ok := bestByRole[ArchiveRolePersonal]; ok {
		pSheet := parsedSheets[pIdx]
		personal, pIssues := parseArchivePersonalSheet(pSheet, subjects)
		res.Data.Personal = personal
		issues = append(issues, pIssues...)
		res.Report.Sheets = append(res.Report.Sheets, store.ImportSheet{
			Name:      pSheet.Info.Name,
			Role:      ArchiveRolePersonal,
			HeaderRow: pSheet.Info.HeaderRow,
			Rows:      len(personal),
		})
	}

	if len(res.Data.Rows) == 0 && len(res.Data.Personal) == 0 {
		issues = append(issues, store.ImportIssue{
			Severity: "blocking",
			Row:      1,
			Column:   "class_name",
			Code:     "no_rows",
		})
	}

	res.Issues = issues
	res.Report.RowCount = len(res.Data.Rows) + len(res.Data.Personal)
	return res, nil
}

func readArchiveSheets(wb *excelize.File, subjects []store.Subject) ([]archiveParsedSheet, error) {
	sheetNames := wb.GetSheetList()
	if len(sheetNames) == 0 {
		return nil, fmt.Errorf("工作簿没有工作表")
	}
	out := make([]archiveParsedSheet, 0, len(sheetNames))
	for _, sheet := range sheetNames {
		rows, err := excel.ReadMergedRows(wb, sheet)
		if err != nil || len(rows) == 0 {
			out = append(out, archiveParsedSheet{Info: ArchiveSheetInfo{Name: sheet}})
			continue
		}
		cols, dataStart := extractArchiveHeader(rows)
		if len(cols) == 0 {
			out = append(out, archiveParsedSheet{Info: ArchiveSheetInfo{Name: sheet}})
			continue
		}
		var dataRows [][]string
		var dataNums []int
		for r := dataStart; r < len(rows); r++ {
			row := normalizeRowWidth(rows[r], len(cols))
			if archiveRowEmpty(row) {
				continue
			}
			dataRows = append(dataRows, row)
			dataNums = append(dataNums, r+1)
		}
		role, format, score := classifyArchiveSheet(sheet, cols, len(dataRows), subjects)
		headers := make([]string, 0, len(cols))
		for _, c := range cols {
			headers = append(headers, c.Label)
		}
		out = append(out, archiveParsedSheet{
			Info: ArchiveSheetInfo{
				Name:      sheet,
				Role:      role,
				Format:    format,
				HeaderRow: dataStart,
				RowCount:  len(dataRows),
				Score:     score,
				Headers:   headers,
			},
			Cols:     cols,
			DataRows: dataRows,
			DataNums: dataNums,
		})
	}
	return out, nil
}

func cleanArchiveCell(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", ""))
}

func normalizeRowWidth(row []string, width int) []string {
	out := make([]string, width)
	for i := 0; i < width && i < len(row); i++ {
		out[i] = cleanArchiveCell(row[i])
	}
	return out
}

func archiveRowEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// extractArchiveHeader 在前 15 行内定位单行或双行合并表头，并返回列定义与数据起始行索引。
func extractArchiveHeader(rows [][]string) ([]archiveHeaderCol, int) {
	limit := len(rows)
	if limit > 15 {
		limit = 15
	}
	width := 0
	for i := 0; i < len(rows); i++ {
		if len(rows[i]) > width {
			width = len(rows[i])
		}
	}
	if width == 0 {
		return nil, 0
	}

	headerKeywords := []string{
		"班级", "行政班", "学科", "科目", "统计组", "任课教师", "教师", "教师姓名",
		"教学量化成绩", "量化成绩", "综合得分", "均分提升率", "达线率", "完成率",
		"排名", "名次", "档次", "赋分", "个人得分", "期中得分", "期末得分",
		"参考人数", "班级均分", "年级均分", "基数", "达线人数", "单科目标",
		"语文", "数学", "英语", "外语", "物理", "化学", "生物", "历史", "地理", "政治",
	}

	bestRow := -1
	bestScore := 0
	for r := 0; r < limit; r++ {
		if isBannerRow(rows[r]) {
			continue
		}
		score := scoreArchiveHeaderRow(rows[r], headerKeywords)
		if score > bestScore {
			bestScore = score
			bestRow = r
		}
	}
	if bestRow < 0 || bestScore < 2 {
		return nil, 0
	}

	topRow := -1
	botRow := bestRow
	// 判断相邻下一行或上一行是否构成两行复合表头（如上行是「语文 / 三项指标」，下行是「任课教师 / 均分提升率 / 排名」）。
	if bestRow+1 < limit && isSubHeaderRow(rows[bestRow], rows[bestRow+1], headerKeywords) {
		topRow = bestRow
		botRow = bestRow + 1
	} else if bestRow > 0 && !isBannerRow(rows[bestRow-1]) && isSubHeaderRow(rows[bestRow-1], rows[bestRow], headerKeywords) {
		topRow = bestRow - 1
		botRow = bestRow
	}

	cols := make([]archiveHeaderCol, width)
	for c := 0; c < width; c++ {
		bot := ""
		if c < len(rows[botRow]) {
			bot = cleanArchiveCell(rows[botRow][c])
		}
		top := ""
		if topRow >= 0 && c < len(rows[topRow]) {
			top = cleanArchiveCell(rows[topRow][c])
		}
		label := bot
		switch {
		case top == "":
			label = bot
		case bot == "" || top == bot:
			label = top
		default:
			label = top + "-" + bot
		}
		cols[c] = archiveHeaderCol{
			Index:  c,
			Top:    top,
			Bottom: bot,
			Label:  label,
		}
	}
	return cols, botRow + 1
}

// isBannerRow 判断整行是否只是同一个跨列合并的大标题。
func isBannerRow(row []string) bool {
	distinct := map[string]bool{}
	for _, cell := range row {
		v := cleanArchiveCell(cell)
		if v != "" {
			distinct[v] = true
		}
	}
	return len(distinct) <= 1
}

func scoreArchiveHeaderRow(row []string, keywords []string) int {
	score := 0
	seen := map[string]bool{}
	for _, cell := range row {
		v := cleanArchiveCell(cell)
		if v == "" {
			continue
		}
		for _, kw := range keywords {
			if seen[kw] {
				continue
			}
			if strings.Contains(v, kw) {
				score++
				seen[kw] = true
				break
			}
		}
	}
	return score
}

// isSubHeaderRow 判断 child 行是否作为 parent 行的子表头行，而不是数据行。
func isSubHeaderRow(parent, child, keywords []string) bool {
	if isBannerRow(child) {
		return false
	}
	childDistinct := 0
	seen := map[string]bool{}
	for _, cell := range child {
		v := cleanArchiveCell(cell)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		childDistinct++
	}
	if childDistinct < 2 {
		return false
	}
	// 子表头行不应出现普通数字单元格，且至少命中 2 个表头关键词。
	metricHits := 0
	for _, cell := range child {
		v := cleanArchiveCell(cell)
		if v == "" {
			continue
		}
		if _, status := parseNumber(v); status == numberOK {
			return false
		}
		for _, kw := range keywords {
			if strings.Contains(v, kw) {
				metricHits++
				break
			}
		}
	}
	if metricHits < 2 {
		return false
	}
	// parent 行不能像数据行，且必须至少有两个非空单元格。
	parentNonEmpty := 0
	for _, cell := range parent {
		v := cleanArchiveCell(cell)
		if v == "" {
			continue
		}
		parentNonEmpty++
	}
	return parentNonEmpty >= 2
}

// classifyArchiveSheet 严格根据表头结构识别结果工作表角色，不依赖工作表顺序。
func classifyArchiveSheet(sheetName string, cols []archiveHeaderCol, rowCount int, subjects []store.Subject) (string, string, int) {
	hasStudentID := hasArchiveHeaderExact(cols, "考号", "学号", "考生号", "学籍号")
	if hasStudentID {
		return "", "", 0
	}

	// 1. 先检查个人得分表（长表或右侧汇总块）。
	if _, ok := findPersonalBlock(cols); ok {
		// 若同时是完整的排名/量化大表（含班级+教学量化成绩/均分提升率），不误判为个人得分。
		hasTotalMetric := hasArchiveHeaderContains(cols, "教学量化成绩", "量化成绩", "均分提升率", "均分提升率得分")
		if !hasTotalMetric {
			score := 40 + minInt(rowCount, 40)
			if strings.Contains(sheetName, "个人") {
				score += 40
			}
			score += archiveSheetNameAdjustment(sheetName)
			return ArchiveRolePersonal, "long", score
		}
	}

	hasClass := hasArchiveHeaderExact(cols, "班级", "行政班", "承担统计职责班级")
	if !hasClass {
		return "", "", 0
	}

	// 2. 长表格式（一行一个班级+学科）。
	hasSubjectCol := hasArchiveHeaderExact(cols, "学科", "科目")
	if hasSubjectCol {
		hasQuant := hasArchiveHeaderContains(cols, "教学量化成绩", "量化成绩", "综合得分") ||
			(hasArchiveHeaderContains(cols, "均分提升率") && hasArchiveHeaderContains(cols, "达线率"))
		hasRank := hasArchiveHeaderExact(cols, "排名", "名次")
		hasPoints := hasArchiveHeaderExact(cols, "赋分")
		hasTier := hasArchiveHeaderExact(cols, "档次", "分档")
		isRankSheetName := strings.Contains(sheetName, "排名") || strings.Contains(sheetName, "赋分")

		if hasQuant && !hasPoints && !isRankSheetName {
			score := 50 + minInt(rowCount, 40)
			if strings.Contains(sheetName, "量化") {
				score += 40
			}
			if strings.Contains(sheetName, "提升率") || strings.Contains(sheetName, "达线率") || strings.Contains(sheetName, "完成率") {
				score -= 30
			}
			score += archiveSheetNameAdjustment(sheetName)
			return ArchiveRoleQuant, "long", score
		}
		if (hasRank && hasPoints) || (hasRank && hasQuant) || (hasTier && hasPoints) || ((hasRank || hasTier) && isRankSheetName) {
			score := 60 + minInt(rowCount, 40)
			if isRankSheetName {
				score += 40
			}
			score += archiveSheetNameAdjustment(sheetName)
			return ArchiveRoleRank, "long", score
		}
		if hasQuant {
			score := 50 + minInt(rowCount, 40)
			if strings.Contains(sheetName, "量化") {
				score += 40
			}
			if strings.Contains(sheetName, "提升率") || strings.Contains(sheetName, "达线率") || strings.Contains(sheetName, "完成率") {
				score -= 30
			}
			score += archiveSheetNameAdjustment(sheetName)
			return ArchiveRoleQuant, "long", score
		}
	}

	// 3. 宽表格式（一行一个班级，横向重复多个学科块）。
	blocks := detectWideSubjectBlocks(cols, subjects)
	if len(blocks) > 0 {
		rankBlocks := 0
		pointsBlocks := 0
		quantBlocks := 0
		for _, blk := range blocks {
			if blk.RankCol >= 0 || blk.PointsCol >= 0 || blk.TierCol >= 0 {
				rankBlocks++
			}
			if blk.PointsCol >= 0 {
				pointsBlocks++
			}
			if blk.TotalCol >= 0 || (blk.ImprovementCol >= 0 && blk.PassCol >= 0) {
				quantBlocks++
			}
		}
		isRankSheetName := strings.Contains(sheetName, "排名") || strings.Contains(sheetName, "赋分")
		if quantBlocks > 0 && pointsBlocks == 0 && !isRankSheetName {
			score := 50 + quantBlocks*5 + minInt(rowCount, 40)
			if strings.Contains(sheetName, "量化") {
				score += 40
			}
			if strings.Contains(sheetName, "提升率") || strings.Contains(sheetName, "达线率") || strings.Contains(sheetName, "完成率") {
				score -= 30
			}
			score += archiveSheetNameAdjustment(sheetName)
			return ArchiveRoleQuant, "wide", score
		}
		if rankBlocks > 0 {
			score := 60 + rankBlocks*5 + minInt(rowCount, 40)
			if isRankSheetName {
				score += 40
			}
			score += archiveSheetNameAdjustment(sheetName)
			return ArchiveRoleRank, "wide", score
		}
		if quantBlocks > 0 {
			score := 50 + quantBlocks*5 + minInt(rowCount, 40)
			if strings.Contains(sheetName, "量化") {
				score += 40
			}
			if strings.Contains(sheetName, "提升率") || strings.Contains(sheetName, "达线率") || strings.Contains(sheetName, "完成率") {
				score -= 30
			}
			score += archiveSheetNameAdjustment(sheetName)
			return ArchiveRoleQuant, "wide", score
		}
	}

	return "", "", 0
}

func archiveSheetNameAdjustment(sheetName string) int {
	adj := 0
	if strings.Contains(sheetName, "新") || strings.Contains(sheetName, "修改") || strings.Contains(sheetName, "核对后") {
		adj += 15
	}
	if strings.Contains(sheetName, "旧") || strings.Contains(sheetName, "修改前") || strings.Contains(sheetName, "副本") || strings.Contains(sheetName, "备用") {
		adj -= 40
	}
	if strings.HasSuffix(strings.TrimSpace(sheetName), "2") {
		adj -= 20
	}
	return adj
}

func hasArchiveHeaderExact(cols []archiveHeaderCol, names ...string) bool {
	return findArchiveColExact(cols, names...) >= 0
}

func hasArchiveHeaderContains(cols []archiveHeaderCol, substrs ...string) bool {
	return findArchiveColContains(cols, substrs...) >= 0
}

func findArchiveColExact(cols []archiveHeaderCol, names ...string) int {
	for _, name := range names {
		for _, col := range cols {
			if col.Bottom == name || col.Label == name || col.Top == name {
				return col.Index
			}
		}
	}
	return -1
}

func findArchiveColContains(cols []archiveHeaderCol, substrs ...string) int {
	for _, sub := range substrs {
		for _, col := range cols {
			if sub != "" && (strings.Contains(col.Bottom, sub) || strings.Contains(col.Label, sub)) {
				return col.Index
			}
		}
	}
	return -1
}

type parsedArchiveQuantRow struct {
	Row      store.ImportArchivedResultRow
	HasTotal bool
}

type parsedArchiveRankRow struct {
	Row           store.ImportArchivedResultRow
	HasTotal      bool
	MissingPoints bool
}

type wideSubjectBlock struct {
	Subject        string
	TeacherCol     int
	ClassAvgCol    int
	GradeAvgCol    int
	BaseAvgCol     int
	BaseRatioCol   int
	ImprovementCol int
	PassCountCol   int
	PassTargetCol  int
	PassCol        int
	TotalCol       int
	RankCol        int
	TierCol        int
	PointsCol      int
}

func parseArchiveQuantSheet(sheet archiveParsedSheet, subjects []store.Subject) ([]parsedArchiveQuantRow, []store.ImportIssue) {
	if sheet.Info.Format == "wide" {
		rows, issues := parseArchiveWideSheet(sheet, subjects, false)
		out := make([]parsedArchiveQuantRow, len(rows))
		for i, r := range rows {
			out[i] = parsedArchiveQuantRow{Row: r.Row, HasTotal: r.HasTotal}
		}
		return out, issues
	}
	rows, issues := parseArchiveLongSheet(sheet, subjects, false)
	out := make([]parsedArchiveQuantRow, len(rows))
	for i, r := range rows {
		out[i] = parsedArchiveQuantRow{Row: r.Row, HasTotal: r.HasTotal}
	}
	return out, issues
}

func parseArchiveRankSheet(sheet archiveParsedSheet, subjects []store.Subject) ([]parsedArchiveRankRow, []store.ImportIssue) {
	if sheet.Info.Format == "wide" {
		return parseArchiveWideSheet(sheet, subjects, true)
	}
	return parseArchiveLongSheet(sheet, subjects, true)
}

func parseArchiveLongSheet(sheet archiveParsedSheet, subjects []store.Subject, expectRank bool) ([]parsedArchiveRankRow, []store.ImportIssue) {
	cols := sheet.Cols
	classCol := findArchiveColExact(cols, "班级", "行政班", "承担统计职责班级")
	subjCol := findArchiveColExact(cols, "学科", "科目")
	statGroupCol := findArchiveColExact(cols, "统计组", "统计组名称")
	teacherCol := findArchiveColExact(cols, "任课教师", "教师", "教师姓名", "授课教师")
	countCol := findArchiveColExact(cols, "参考人数", "班级人数", "实考人数", "人数")
	classAvgCol := findArchiveColExact(cols, "班级均分", "均分")
	gradeAvgCol := findArchiveColExact(cols, "年级均分")
	baseAvgCol := findArchiveColExact(cols, "基数均分", "入口均分", "基线均分")
	baseRatioCol := findArchiveColExact(cols, "基数比值", "基数")
	impRawCol := findArchiveColExact(cols, "提升率未封顶值")
	impScoreCol := findArchiveColExact(cols, "均分提升率得分", "均分提升率", "提升率得分")
	if impScoreCol < 0 {
		impScoreCol = findArchiveColContains(cols, "均分提升率")
	}
	passCountCol := findArchiveColExact(cols, "达线人数")
	passTargetCol := findArchiveColExact(cols, "单科目标", "统计组目标", "目标任务")
	passRawCol := findArchiveColExact(cols, "达线率未封顶值")
	passScoreCol := findArchiveColExact(cols, "单科达线率得分", "单科达线率", "达线率得分", "达线率")
	if passScoreCol < 0 {
		passScoreCol = findArchiveColContains(cols, "达线率")
	}
	compCol := findArchiveColExact(cols, "完成率得分", "班级目标完成率得分", "完成率")
	if compCol < 0 {
		compCol = findArchiveColContains(cols, "完成率")
	}
	totalCol := findArchiveColExact(cols, "教学量化成绩", "量化成绩", "综合得分", "总分")
	if totalCol < 0 {
		totalCol = findArchiveColContains(cols, "教学量化成绩", "量化成绩")
	}
	rankCol := findArchiveColExact(cols, "排名", "名次")
	tierCol := findArchiveColExact(cols, "档次", "分档")
	pointsCol := findArchiveColExact(cols, "赋分")

	var out []parsedArchiveRankRow
	var issues []store.ImportIssue

	for idx, values := range sheet.DataRows {
		rowNum := sheet.DataNums[idx]
		className := normalizeClassName(archiveVal(values, classCol))
		subjText := archiveVal(values, subjCol)
		if className == "" && strings.TrimSpace(subjText) == "" {
			continue
		}
		if (className != "" && skipSummaryRow(className)) || (strings.TrimSpace(subjText) != "" && skipSummaryRow(subjText)) {
			continue
		}
		if className == "" {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: rowNum, Column: "class_name", Code: "missing_field",
			})
			continue
		}
		if subjText == "" {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: rowNum, Column: "subject", Code: "missing_field",
			})
			continue
		}
		subjKey, ok := resolveArchiveSubject(subjText, subjects)
		if !ok {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: rowNum, Column: "subject", Code: "unknown_subject", Args: []string{subjText},
			})
			continue
		}

		row := store.ImportArchivedResultRow{
			ClassName:     className,
			Subject:       subjKey,
			StatGroupName: normalizeClassName(archiveVal(values, statGroupCol)),
			TeacherNames:  splitArchiveTeachers(archiveVal(values, teacherCol)),
			SourceRow:     rowNum,
		}
		if row.StatGroupName == "" {
			row.StatGroupName = className
		}

		blocked := false
		if v, present, ok := parseArchiveIntField(archiveVal(values, countCol), rowNum, "class_count", false, &issues); !ok {
			blocked = true
		} else if present {
			row.ClassCount = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, classAvgCol), rowNum, "class_average", false, &issues); !ok {
			blocked = true
		} else if present {
			row.ClassAverage = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, gradeAvgCol), rowNum, "grade_average", false, &issues); !ok {
			blocked = true
		} else if present {
			row.GradeAverage = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, baseAvgCol), rowNum, "baseline_average", false, &issues); !ok {
			blocked = true
		} else if present {
			row.BaselineAverage = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, baseRatioCol), rowNum, "baseline_ratio", false, &issues); !ok {
			blocked = true
		} else if present {
			row.BaselineRatio = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, impRawCol), rowNum, "improvement_raw", false, &issues); !ok {
			blocked = true
		} else if present {
			row.ImprovementRaw = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, impScoreCol), rowNum, "improvement_score", false, &issues); !ok {
			blocked = true
		} else if present {
			row.ImprovementScore = v
		}
		if v, present, ok := parseArchiveIntField(archiveVal(values, passCountCol), rowNum, "pass_count", true, &issues); !ok {
			blocked = true
		} else if present {
			row.PassCount = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, passTargetCol), rowNum, "pass_target", false, &issues); !ok {
			blocked = true
		} else if present {
			row.PassTarget = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, passRawCol), rowNum, "pass_raw", false, &issues); !ok {
			blocked = true
		} else if present {
			row.PassRaw = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, passScoreCol), rowNum, "pass_score", false, &issues); !ok {
			blocked = true
		} else if present {
			row.PassScore = v
		}
		if v, present, ok := parseArchiveFloatField(archiveVal(values, compCol), rowNum, "completion_score", false, &issues); !ok {
			blocked = true
		} else if present {
			row.CompletionParticipate = true
			row.CompletionScore = v
		}

		hasTotal := false
		if v, present, ok := parseArchiveFloatField(archiveVal(values, totalCol), rowNum, "quant_total_score", false, &issues); !ok {
			blocked = true
		} else if present {
			row.Total = v
			hasTotal = true
		}
		if v, present, ok := parseArchiveIntField(archiveVal(values, rankCol), rowNum, "rank", true, &issues); !ok {
			blocked = true
		} else if present {
			row.Rank = v
		}
		if tierCol >= 0 {
			tierText := archiveVal(values, tierCol)
			if tierText != "" && !isEmptyMarker(tierText) {
				if idxVal, valid := parseArchiveTier(tierText); valid {
					copied := idxVal
					row.TierIndex = &copied
				} else {
					issues = append(issues, store.ImportIssue{
						Severity: "warning", Row: rowNum, Column: "tier", Code: "archive_invalid_tier", Args: []string{tierText},
					})
				}
			}
		}
		missingPoints := false
		if v, present, ok := parseArchiveFloatField(archiveVal(values, pointsCol), rowNum, "points", true, &issues); !ok {
			blocked = true
		} else if present {
			row.Points = v
		} else if expectRank && pointsCol >= 0 && !blocked {
			missingPoints = true
		}

		if blocked {
			continue
		}
		out = append(out, parsedArchiveRankRow{Row: row, HasTotal: hasTotal, MissingPoints: missingPoints})
	}
	return out, issues
}

func detectWideSubjectBlocks(cols []archiveHeaderCol, subjects []store.Subject) []wideSubjectBlock {
	subjectForCol := func(c archiveHeaderCol) (string, bool) {
		if c.Top != "" {
			if key, ok := resolveArchiveSubject(c.Top, subjects); ok {
				return key, true
			}
		}
		if key, ok := resolveArchiveSubject(c.Bottom, subjects); ok {
			return key, true
		}
		if key, ok := resolveArchiveSubject(c.Label, subjects); ok {
			return key, true
		}
		return "", false
	}

	var blocks []wideSubjectBlock
	cur := wideSubjectBlock{
		TeacherCol: -1, ClassAvgCol: -1, GradeAvgCol: -1, BaseAvgCol: -1, BaseRatioCol: -1,
		ImprovementCol: -1, PassCountCol: -1, PassTargetCol: -1, PassCol: -1,
		TotalCol: -1, RankCol: -1, TierCol: -1, PointsCol: -1,
	}
	inBlock := false
	flush := func() {
		if !inBlock {
			return
		}
		if cur.TotalCol >= 0 || cur.ImprovementCol >= 0 || cur.PassCol >= 0 || cur.RankCol >= 0 || cur.PointsCol >= 0 {
			blocks = append(blocks, cur)
		}
		cur = wideSubjectBlock{
			TeacherCol: -1, ClassAvgCol: -1, GradeAvgCol: -1, BaseAvgCol: -1, BaseRatioCol: -1,
			ImprovementCol: -1, PassCountCol: -1, PassTargetCol: -1, PassCol: -1,
			TotalCol: -1, RankCol: -1, TierCol: -1, PointsCol: -1,
		}
		inBlock = false
	}

	for _, col := range cols {
		// 先检查是否是一个新学科块的起始列。
		if subj, ok := subjectForCol(col); ok {
			isPureSubjectName := isExactSubjectLabel(col.Bottom) || isExactSubjectLabel(col.Label) ||
				col.Bottom == "任课教师" || col.Bottom == "教师" || col.Bottom == "教师姓名"
			if (!inBlock && (isPureSubjectName || col.Top != "")) || (inBlock && cur.Subject != subj) {
				flush()
				cur.Subject = subj
				inBlock = true
				if isPureSubjectName {
					cur.TeacherCol = col.Index
					continue
				}
			}
		}
		if !inBlock {
			continue
		}

		label := col.Bottom
		if label == "" {
			label = col.Label
		}
		switch {
		case label == "任课教师" || label == "教师" || label == "教师姓名" || label == "授课教师":
			cur.TeacherCol = col.Index
		case strings.Contains(label, "提升率"):
			cur.ImprovementCol = col.Index
		case strings.Contains(label, "达线率"):
			cur.PassCol = col.Index
		case strings.Contains(label, "达线人数"):
			cur.PassCountCol = col.Index
		case strings.Contains(label, "单科目标") || strings.Contains(label, "目标"):
			cur.PassTargetCol = col.Index
		case strings.Contains(label, "教学量化成绩") || strings.Contains(label, "量化成绩") || strings.Contains(label, "综合得分"):
			cur.TotalCol = col.Index
		case label == "排名" || label == "名次":
			cur.RankCol = col.Index
		case label == "档次" || label == "分档":
			cur.TierCol = col.Index
		case label == "赋分":
			cur.PointsCol = col.Index
		case strings.Contains(label, "基") && !strings.Contains(label, "提升"):
			if strings.Contains(label, "均分") {
				cur.BaseAvgCol = col.Index
			} else {
				cur.BaseRatioCol = col.Index
			}
		case strings.Contains(label, "均"):
			if strings.Contains(label, "年级") {
				cur.GradeAvgCol = col.Index
			} else {
				cur.ClassAvgCol = col.Index
			}
		}
	}
	flush()
	return blocks
}

func isExactSubjectLabel(s string) bool {
	s = strings.TrimSpace(s)
	_, ok := subjectAliases[s]
	return ok
}

func parseArchiveWideSheet(sheet archiveParsedSheet, subjects []store.Subject, expectRank bool) ([]parsedArchiveRankRow, []store.ImportIssue) {
	cols := sheet.Cols
	blocks := detectWideSubjectBlocks(cols, subjects)
	firstBlockCol := len(cols)
	for _, blk := range blocks {
		for _, c := range []int{
			blk.TeacherCol, blk.ClassAvgCol, blk.GradeAvgCol, blk.BaseAvgCol, blk.BaseRatioCol,
			blk.ImprovementCol, blk.PassCountCol, blk.PassTargetCol, blk.PassCol,
			blk.TotalCol, blk.RankCol, blk.TierCol, blk.PointsCol,
		} {
			if c >= 0 && c < firstBlockCol {
				firstBlockCol = c
			}
		}
	}
	prefixCols := make([]archiveHeaderCol, 0, firstBlockCol)
	for _, c := range cols {
		if c.Index < firstBlockCol {
			prefixCols = append(prefixCols, c)
		}
	}

	classCol := findArchiveColExact(prefixCols, "班级", "行政班")
	countCol := findArchiveColContains(prefixCols, "参考人数", "班级人数")
	compCol := findArchiveColExact(prefixCols, "班级目标完成率得分", "完成率得分", "完成率")
	if compCol < 0 {
		// 历史工作簿里「班级目标基础分」紧邻的列常简写为「得分」。
		baseIdx := findArchiveColContains(prefixCols, "目标基础分")
		if baseIdx >= 0 && baseIdx+1 < len(prefixCols) && strings.Contains(prefixCols[baseIdx+1].Label, "得分") {
			compCol = prefixCols[baseIdx+1].Index
		} else {
			compCol = findArchiveColContains(prefixCols, "完成率得分")
		}
	}

	var out []parsedArchiveRankRow
	var issues []store.ImportIssue

	for idx, values := range sheet.DataRows {
		rowNum := sheet.DataNums[idx]
		className := normalizeClassName(archiveVal(values, classCol))
		if className == "" || skipSummaryRow(className) {
			continue
		}

		classCount := 0
		if v, present, ok := parseArchiveIntField(archiveVal(values, countCol), rowNum, "class_count", false, &issues); ok && present {
			classCount = v
		}
		compParticipate := false
		compScore := 0.0
		if v, present, ok := parseArchiveFloatField(archiveVal(values, compCol), rowNum, "completion_score", false, &issues); !ok {
			// 完成率字段异常时不阻断其他列，问题已记入 issues。
		} else if present {
			compParticipate = true
			compScore = v
		}

		for _, blk := range blocks {
			teacherRaw := archiveVal(values, blk.TeacherCol)
			impRaw := archiveVal(values, blk.ImprovementCol)
			passRaw := archiveVal(values, blk.PassCol)
			totalRaw := archiveVal(values, blk.TotalCol)
			rankRaw := archiveVal(values, blk.RankCol)
			tierRaw := archiveVal(values, blk.TierCol)
			pointsRaw := archiveVal(values, blk.PointsCol)

			// 某班级未开设该学科（例如物理类班级不考历史/地理/政治）或非主统计行时整块为空，跳过。
			if isEmptyMarker(impRaw) && isEmptyMarker(passRaw) && isEmptyMarker(totalRaw) &&
				isEmptyMarker(rankRaw) && isEmptyMarker(tierRaw) && isEmptyMarker(pointsRaw) {
				continue
			}

			row := store.ImportArchivedResultRow{
				ClassName:             className,
				Subject:               blk.Subject,
				StatGroupName:         className,
				TeacherNames:          splitArchiveTeachers(teacherRaw),
				ClassCount:            classCount,
				CompletionParticipate: compParticipate,
				CompletionScore:       compScore,
				SourceRow:             rowNum,
			}

			blocked := false
			if v, present, ok := parseArchiveFloatField(archiveVal(values, blk.ClassAvgCol), rowNum, "class_average", false, &issues); !ok {
				blocked = true
			} else if present {
				row.ClassAverage = v
			}
			if v, present, ok := parseArchiveFloatField(archiveVal(values, blk.GradeAvgCol), rowNum, "grade_average", false, &issues); !ok {
				blocked = true
			} else if present {
				row.GradeAverage = v
			}
			if v, present, ok := parseArchiveFloatField(archiveVal(values, blk.BaseAvgCol), rowNum, "baseline_average", false, &issues); !ok {
				blocked = true
			} else if present {
				row.BaselineAverage = v
			}
			if v, present, ok := parseArchiveFloatField(archiveVal(values, blk.BaseRatioCol), rowNum, "baseline_ratio", false, &issues); !ok {
				blocked = true
			} else if present {
				row.BaselineRatio = v
			}
			if v, present, ok := parseArchiveFloatField(impRaw, rowNum, "improvement_score", false, &issues); !ok {
				blocked = true
			} else if present {
				row.ImprovementScore = v
			}
			if v, present, ok := parseArchiveIntField(archiveVal(values, blk.PassCountCol), rowNum, "pass_count", true, &issues); !ok {
				blocked = true
			} else if present {
				row.PassCount = v
			}
			if v, present, ok := parseArchiveFloatField(archiveVal(values, blk.PassTargetCol), rowNum, "pass_target", false, &issues); !ok {
				blocked = true
			} else if present {
				row.PassTarget = v
			}
			if v, present, ok := parseArchiveFloatField(passRaw, rowNum, "pass_score", false, &issues); !ok {
				blocked = true
			} else if present {
				row.PassScore = v
			}
			hasTotal := false
			if v, present, ok := parseArchiveFloatField(totalRaw, rowNum, "quant_total_score", false, &issues); !ok {
				blocked = true
			} else if present {
				row.Total = v
				hasTotal = true
			}
			if v, present, ok := parseArchiveIntField(rankRaw, rowNum, "rank", true, &issues); !ok {
				blocked = true
			} else if present {
				row.Rank = v
			}
			if blk.TierCol >= 0 && tierRaw != "" && !isEmptyMarker(tierRaw) {
				if tierIdx, valid := parseArchiveTier(tierRaw); valid {
					copied := tierIdx
					row.TierIndex = &copied
				} else {
					issues = append(issues, store.ImportIssue{
						Severity: "warning", Row: rowNum, Column: "tier", Code: "archive_invalid_tier", Args: []string{tierRaw},
					})
				}
			}
			missingPoints := false
			if v, present, ok := parseArchiveFloatField(pointsRaw, rowNum, "points", true, &issues); !ok {
				blocked = true
			} else if present {
				row.Points = v
			} else if expectRank && blk.PointsCol >= 0 && !blocked {
				missingPoints = true
			}

			if blocked {
				continue
			}
			out = append(out, parsedArchiveRankRow{Row: row, HasTotal: hasTotal, MissingPoints: missingPoints})
		}
	}
	return out, issues
}

func mergeArchiveResultRows(quantRows []parsedArchiveQuantRow, rankRows []parsedArchiveRankRow, hasRankSheet bool) ([]store.ImportArchivedResultRow, []store.ImportIssue) {
	type mergedEntry struct {
		row            store.ImportArchivedResultRow
		hasTotal       bool
		mergedFromRank bool
		missingPoints  bool
		pointsRow      int
	}
	rowKey := func(r store.ImportArchivedResultRow) string {
		cn := normalizeClassName(r.ClassName)
		sg := normalizeClassName(r.StatGroupName)
		if sg == "" {
			sg = cn
		}
		return cn + "\x00" + r.Subject + "\x00" + sg
	}

	var order []string
	merged := map[string]*mergedEntry{}
	var extras []store.ImportArchivedResultRow

	for _, qr := range quantRows {
		k := rowKey(qr.Row)
		if _, exists := merged[k]; exists {
			// 同表内重复行保留下来，由 ValidateArchivedResultImport 报 archive_stat_group_duplicate 阻断。
			extras = append(extras, qr.Row)
			continue
		}
		hasTotal := qr.HasTotal || (qr.Row.Total != 0)
		merged[k] = &mergedEntry{row: qr.Row, hasTotal: hasTotal}
		order = append(order, k)
	}

	for _, rr := range rankRows {
		k := rowKey(rr.Row)
		entry, exists := merged[k]
		if !exists {
			copied := rr.Row
			pointsRow := 0
			if rr.MissingPoints {
				pointsRow = rr.Row.SourceRow
			}
			merged[k] = &mergedEntry{
				row: copied, hasTotal: rr.HasTotal, mergedFromRank: true,
				missingPoints: rr.MissingPoints, pointsRow: pointsRow,
			}
			order = append(order, k)
			continue
		}
		if entry.mergedFromRank {
			// 排名赋分表内部重复同一统计组，保留给校验层报 archive_stat_group_duplicate 阻断。
			extras = append(extras, rr.Row)
			continue
		}
		entry.mergedFromRank = true

		// 即使量化表已带部分排名或档次列，只要跨表主键唯一对应，就与排名赋分表互补合并。
		if rr.Row.Rank > 0 {
			entry.row.Rank = rr.Row.Rank
		}
		if rr.Row.TierIndex != nil {
			entry.row.TierIndex = rr.Row.TierIndex
		}
		if rr.Row.Points != 0 {
			entry.row.Points = rr.Row.Points
		}
		if rr.MissingPoints && entry.row.Points == 0 {
			entry.missingPoints = true
			entry.pointsRow = rr.Row.SourceRow
		} else if entry.row.Points != 0 {
			entry.missingPoints = false
		}
		if !entry.hasTotal && rr.HasTotal {
			entry.row.Total = rr.Row.Total
			entry.hasTotal = true
		} else if entry.row.Total == 0 && rr.Row.Total != 0 {
			entry.row.Total = rr.Row.Total
			entry.hasTotal = entry.hasTotal || rr.HasTotal
		}
		if len(entry.row.TeacherNames) == 0 && len(rr.Row.TeacherNames) > 0 {
			entry.row.TeacherNames = rr.Row.TeacherNames
		}
		if entry.row.ClassCount == 0 && rr.Row.ClassCount != 0 {
			entry.row.ClassCount = rr.Row.ClassCount
		}
		if entry.row.ClassAverage == 0 && rr.Row.ClassAverage != 0 {
			entry.row.ClassAverage = rr.Row.ClassAverage
		}
		if entry.row.GradeAverage == 0 && rr.Row.GradeAverage != 0 {
			entry.row.GradeAverage = rr.Row.GradeAverage
		}
		if entry.row.BaselineAverage == 0 && rr.Row.BaselineAverage != 0 {
			entry.row.BaselineAverage = rr.Row.BaselineAverage
		}
		if entry.row.BaselineRatio == 0 && rr.Row.BaselineRatio != 0 {
			entry.row.BaselineRatio = rr.Row.BaselineRatio
		}
		if entry.row.ImprovementRaw == 0 && rr.Row.ImprovementRaw != 0 {
			entry.row.ImprovementRaw = rr.Row.ImprovementRaw
		}
		if entry.row.ImprovementScore == 0 && rr.Row.ImprovementScore != 0 {
			entry.row.ImprovementScore = rr.Row.ImprovementScore
		}
		if entry.row.PassCount == 0 && rr.Row.PassCount != 0 {
			entry.row.PassCount = rr.Row.PassCount
		}
		if entry.row.PassTarget == 0 && rr.Row.PassTarget != 0 {
			entry.row.PassTarget = rr.Row.PassTarget
		}
		if entry.row.PassRaw == 0 && rr.Row.PassRaw != 0 {
			entry.row.PassRaw = rr.Row.PassRaw
		}
		if entry.row.PassScore == 0 && rr.Row.PassScore != 0 {
			entry.row.PassScore = rr.Row.PassScore
		}
		if !entry.row.CompletionParticipate && rr.Row.CompletionParticipate {
			entry.row.CompletionParticipate = true
			entry.row.CompletionScore = rr.Row.CompletionScore
		} else if entry.row.CompletionParticipate && entry.row.CompletionScore == 0 && rr.Row.CompletionParticipate && rr.Row.CompletionScore != 0 {
			entry.row.CompletionScore = rr.Row.CompletionScore
		}
	}

	out := make([]store.ImportArchivedResultRow, 0, len(order)+len(extras))
	var issues []store.ImportIssue
	for _, k := range order {
		entry := merged[k]
		row := entry.row
		if len(row.TeacherNames) == 0 {
			issues = append(issues, store.ImportIssue{
				Severity: "warning", Row: row.SourceRow, Column: "teacher_name",
				Code: "archive_missing_teacher", Args: []string{row.ClassName, row.Subject},
			})
		}
		if !entry.hasTotal && row.Total == 0 {
			issues = append(issues, store.ImportIssue{
				Severity: "warning", Row: row.SourceRow, Column: "quant_total_score",
				Code: "archive_missing_total", Args: []string{row.ClassName, row.Subject},
			})
		}
		if (hasRankSheet && row.Points == 0 && row.Rank > 0) || (entry.missingPoints && row.Points == 0) {
			warnRow := row.SourceRow
			if entry.pointsRow > 0 {
				warnRow = entry.pointsRow
			}
			issues = append(issues, store.ImportIssue{
				Severity: "warning", Row: warnRow, Column: "points",
				Code: "archive_missing_points", Args: []string{row.ClassName, row.Subject},
			})
		}
		out = append(out, row)
	}
	out = append(out, extras...)
	return out, issues
}

type personalBlock struct {
	SubjectCol     int
	TeacherCol     int
	ScoreCol       int
	GroupPointsCol int
}

// findPersonalBlock 从单表或历史多栏个人得分表中定位最终个人得分汇总块（优先无「班级」列的右侧汇总区）。
func findPersonalBlock(cols []archiveHeaderCol) (personalBlock, bool) {
	isSubjectHeader := func(s string) bool {
		return s == "学科" || s == "科目"
	}
	isTeacherHeader := func(s string) bool {
		return s == "教师" || s == "任课教师" || s == "教师姓名" || s == "授课教师"
	}
	isScoreHeader := func(s string) bool {
		return s == "个人得分" || s == "期中得分" || s == "期末得分" || s == "个人量化得分" || s == "得分"
	}

	var candidates []struct {
		blk      personalBlock
		priority int
	}

	for i, col := range cols {
		if !isScoreHeader(col.Bottom) && !isScoreHeader(col.Label) {
			continue
		}
		start := i - 5
		if start < 0 {
			start = 0
		}
		subjCol, teacherCol, gpCol, classCol := -1, -1, -1, -1
		for j := start; j < i; j++ {
			lbl := cols[j].Bottom
			if lbl == "" {
				lbl = cols[j].Label
			}
			switch {
			case isSubjectHeader(lbl):
				subjCol = cols[j].Index
			case isTeacherHeader(lbl):
				teacherCol = cols[j].Index
			case strings.Contains(lbl, "统计组赋分") || strings.Contains(lbl, "任教统计组赋分"):
				gpCol = cols[j].Index
			case lbl == "班级" || lbl == "行政班":
				classCol = cols[j].Index
			}
		}
		if subjCol >= 0 && teacherCol >= 0 {
			prio := 10
			if classCol < 0 {
				prio += 20
			}
			if col.Bottom == "个人得分" || col.Label == "个人得分" {
				prio += 5
			}
			candidates = append(candidates, struct {
				blk      personalBlock
				priority int
			}{
				blk: personalBlock{
					SubjectCol:     subjCol,
					TeacherCol:     teacherCol,
					ScoreCol:       col.Index,
					GroupPointsCol: gpCol,
				},
				priority: prio,
			})
		}
	}

	if len(candidates) == 0 {
		return personalBlock{}, false
	}
	best := candidates[0]
	for _, cand := range candidates[1:] {
		if cand.priority >= best.priority {
			best = cand
		}
	}
	return best.blk, true
}

func parseArchivePersonalSheet(sheet archiveParsedSheet, subjects []store.Subject) ([]store.ImportArchivedPersonalScore, []store.ImportIssue) {
	blk, ok := findPersonalBlock(sheet.Cols)
	if !ok {
		return nil, []store.ImportIssue{{
			Severity: "blocking", Row: sheet.Info.HeaderRow, Column: "personal_score", Code: "missing_field",
		}}
	}

	var out []store.ImportArchivedPersonalScore
	var issues []store.ImportIssue

	for idx, values := range sheet.DataRows {
		rowNum := sheet.DataNums[idx]
		teacherName := strings.TrimSpace(archiveVal(values, blk.TeacherCol))
		subjText := strings.TrimSpace(archiveVal(values, blk.SubjectCol))
		scoreText := strings.TrimSpace(archiveVal(values, blk.ScoreCol))
		if teacherName == "" && subjText == "" && scoreText == "" {
			continue
		}
		if (teacherName != "" && skipSummaryRow(teacherName)) || (subjText != "" && skipSummaryRow(subjText)) {
			continue
		}
		if teacherName == "" {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: rowNum, Column: "teacher_name", Code: "missing_field",
			})
			continue
		}
		if subjText == "" {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: rowNum, Column: "subject", Code: "missing_field",
			})
			continue
		}
		subjKey, ok := resolveArchiveSubject(subjText, subjects)
		if !ok {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: rowNum, Column: "subject", Code: "unknown_subject", Args: []string{subjText},
			})
			continue
		}
		if isEmptyMarker(scoreText) {
			issues = append(issues, store.ImportIssue{
				Severity: "blocking", Row: rowNum, Column: "personal_score", Code: "missing_field",
			})
			continue
		}
		score, present, valid := parseArchiveFloatField(scoreText, rowNum, "personal_score", true, &issues)
		if !valid || !present {
			continue
		}

		var groupPoints []float64
		if blk.GroupPointsCol >= 0 {
			gpRaw := strings.TrimSpace(archiveVal(values, blk.GroupPointsCol))
			if !isEmptyMarker(gpRaw) {
				parts := splitArchiveList(gpRaw)
				for _, part := range parts {
					num, status := parseNumber(part)
					if status != numberOK || num < 0 {
						issues = append(issues, store.ImportIssue{
							Severity: "warning", Row: rowNum, Column: "group_points", Code: "invalid_number", Args: []string{part},
						})
						continue
					}
					groupPoints = append(groupPoints, num)
				}
			}
		}

		out = append(out, store.ImportArchivedPersonalScore{
			TeacherName: teacherName,
			Subject:     subjKey,
			Score:       score,
			GroupPoints: groupPoints,
			SourceRow:   rowNum,
		})
	}
	return out, issues
}

func resolveArchiveSubject(text string, subjects []store.Subject) (string, bool) {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return "", false
	}
	if key, ok := subjectAliases[cleaned]; ok {
		return key, true
	}
	for _, s := range subjects {
		if strings.TrimSpace(s.Name) == cleaned || strings.TrimSpace(s.Key) == cleaned {
			return s.Key, true
		}
	}
	return "", false
}

func splitArchiveTeachers(raw string) []string {
	parts := splitArchiveList(raw)
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func splitArchiveList(raw string) []string {
	normalized := strings.NewReplacer("、", "|", ",", "|", "，", "|", "/", "|", ";", "|", "；", "|", " ", "|").Replace(raw)
	parts := strings.Split(normalized, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" && t != "-" && t != "—" {
			out = append(out, t)
		}
	}
	return out
}

func parseArchiveTier(text string) (int, bool) {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return 0, false
	}
	cleaned = strings.TrimPrefix(cleaned, "第")
	cleaned = strings.TrimSuffix(cleaned, "档")
	switch cleaned {
	case "1", "一":
		return 0, true
	case "2", "二":
		return 1, true
	case "3", "三":
		return 2, true
	case "4", "四":
		return 3, true
	case "5", "五":
		return 4, true
	}
	if n, err := strconv.Atoi(cleaned); err == nil && n >= 1 {
		return n - 1, true
	}
	return 0, false
}

func isEmptyMarker(text string) bool {
	t := strings.TrimSpace(text)
	return t == "" || t == "-" || t == "—" || t == "/"
}

func parseArchiveFloatField(raw string, row int, col string, nonNegative bool, issues *[]store.ImportIssue) (float64, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if isEmptyMarker(trimmed) {
		return 0, false, true
	}
	cleaned := strings.ReplaceAll(trimmed, ",", "")
	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || (nonNegative && val < 0) {
		*issues = append(*issues, store.ImportIssue{
			Severity: "blocking", Row: row, Column: col, Code: "invalid_number", Args: []string{trimmed},
		})
		return 0, false, false
	}
	return val, true, true
}

func parseArchiveIntField(raw string, row int, col string, nonNegative bool, issues *[]store.ImportIssue) (int, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if isEmptyMarker(trimmed) {
		return 0, false, true
	}
	cleaned := strings.ReplaceAll(trimmed, ",", "")
	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || (nonNegative && val < 0) {
		*issues = append(*issues, store.ImportIssue{
			Severity: "blocking", Row: row, Column: col, Code: "invalid_number", Args: []string{trimmed},
		})
		return 0, false, false
	}
	return int(val), true, true
}

func archiveVal(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[index])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
