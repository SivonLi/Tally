package importer_test

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"tally/internal/importer"
	"tally/internal/store"
)

func guidedWorkbook(t *testing.T, dir string) string {
	t.Helper()
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", "学生名单"); err != nil {
		t.Fatal(err)
	}
	students := [][]any{
		{"学号", "姓名", "行政班", "选科组合", "状态", "入口总分"},
		{"0012", "甲", "1班", "物理、化学、生物", "在读", "620.5"},
		{"0013", "乙", "2班", "历史、政治、地理", "转学", "590"},
	}
	for index, row := range students {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetSheetRow("学生名单", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := workbook.NewSheet("任课关系"); err != nil {
		t.Fatal(err)
	}
	assignments := [][]any{
		{"教师姓名", "工号", "行政班", "学科"},
		{"李老师", "T01", "1班", "数学"},
	}
	for index, row := range assignments {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := workbook.SetSheetRow("任课关系", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := workbook.NewSheet("入口成绩"); err != nil {
		t.Fatal(err)
	}
	baselines := [][]any{
		{"行政班", "语文", "数学", "语文比值", "数学比值"},
		{"1班", "102.5", "98", "1.05", "1.1"},
	}
	for index, row := range baselines {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := workbook.SetSheetRow("入口成绩", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "guided.xlsx")
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGuidedWorkbookDetectionAndMapping(t *testing.T) {
	dir := t.TempDir()
	path := guidedWorkbook(t, dir)
	workbook, err := importer.InspectWorkbook(path, importer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(workbook.SheetNames()) != 3 {
		t.Fatalf("sheets = %+v", workbook.SheetNames())
	}
	if got := importer.BestSheet(importer.KindStudents, workbook); got != "学生名单" {
		t.Fatalf("student sheet = %q", got)
	}
	if got := importer.BestSheet(importer.KindAssignments, workbook); got != "任课关系" {
		t.Fatalf("assignment sheet = %q", got)
	}
	if got := importer.BestSheet(importer.KindBaselines, workbook); got != "入口成绩" {
		t.Fatalf("baseline sheet = %q", got)
	}
	subjects := []store.Subject{
		{Key: "chinese", Name: "语文", FullMark: 150, Enabled: true},
		{Key: "math", Name: "数学", FullMark: 150, Enabled: true},
	}
	table, ok := workbook.Sheet("学生名单")
	if !ok {
		t.Fatal("missing student sheet")
	}
	mapping := importer.AutoMapping(importer.KindStudents, table, subjects)
	if mapping[importer.FieldStudentNo] != "学号" ||
		mapping[importer.FieldClassName] != "行政班" {
		t.Fatalf("mapping = %+v", mapping)
	}
	data, issues, err := importer.ParseSheet(workbook, "学生名单", importer.KindStudents, mapping, subjects)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if len(data.Students) != 2 || data.Students[0].StudentNo != "0012" ||
		data.Students[0].Track != "physics" || data.Students[1].Status != "transferred" {
		t.Fatalf("students = %+v", data.Students)
	}
	baselineTable, ok := workbook.Sheet("入口成绩")
	if !ok {
		t.Fatal("missing baseline sheet")
	}
	baselineMapping := importer.AutoMapping(importer.KindBaselines, baselineTable, subjects)
	baselineData, issues, err := importer.ParseSheet(workbook, "入口成绩",
		importer.KindBaselines, baselineMapping, subjects)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("baseline issues = %+v", issues)
	}
	if len(baselineData.ClassBaselines) != 2 {
		t.Fatalf("baselines = %+v", baselineData.ClassBaselines)
	}
	if baselineData.ClassBaselines[0].Ratio == nil {
		t.Fatalf("baseline ratio missing: %+v", baselineData.ClassBaselines[0])
	}
}

func TestGuidedTemplateMatchesFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "template.xlsx")
	subjects := []store.Subject{
		{Key: "chinese", Name: "语文", FullMark: 150, Enabled: true},
		{Key: "math", Name: "数学", FullMark: 150, Enabled: true},
	}
	if err := importer.WriteTemplate(importer.KindStudents, subjects, path); err != nil {
		t.Fatal(err)
	}
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer workbook.Close()
	if got := workbook.GetSheetList(); len(got) != 2 || got[0] != "数据" || got[1] != "说明" {
		t.Fatalf("sheets = %+v", got)
	}
	rows, err := workbook.GetRows("数据")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	want := importer.TemplateHeaders(importer.KindStudents, subjects)
	if len(rows[0]) != len(want) {
		t.Fatalf("headers = %+v want %+v", rows[0], want)
	}
	for index := range want {
		if rows[0][index] != want[index] {
			t.Fatalf("header %d = %q want %q", index, rows[0][index], want[index])
		}
	}
}

func TestGuidedTeacherNeedsNumberOrSubject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "teachers.xlsx")
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", "教师名单"); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{
		{"教师姓名", "工号", "学科"},
		{"李老师", "", ""},
	}
	for index, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetSheetRow("教师名单", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	inspect, err := importer.InspectWorkbook(path, importer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	table, ok := inspect.Sheet("教师名单")
	if !ok {
		t.Fatal("missing teacher sheet")
	}
	mapping := importer.AutoMapping(importer.KindTeachers, table, nil)
	data, issues, err := importer.ParseSheet(inspect, "教师名单", importer.KindTeachers, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.TeacherList) != 0 {
		t.Fatalf("teachers = %+v", data.TeacherList)
	}
	found := false
	for _, issue := range issues {
		if issue.Code == "teacher_identity_missing" && issue.Severity == "blocking" {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestGuidedWideTeacherSheet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overview.xlsx")
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", "2026年春学期"); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{
		{"年级", "类型", "选科", "班级", "班主任", "语文", "数学", "英语", "物理", "化学"},
		{"高一", "精英", "物化生", "537", "韩锋", "王丽", "程胜", "赵丽", "韩锋", "王栋"},
		{"高一", "精英", "物化生", "545", "王磊", "苗劲", "王桃英", "刘爱英", "陈凯", "张晓燕"},
		{"高一", "重点", "物化生", "546", "宋强", "", "王丽", "李丽芳", "呼颖慧", "暴海霞"},
		{"高二", "精英", "物化生", "601", "张老师", "李老师", "", "", "", ""},
	}
	for index, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetSheetRow("2026年春学期", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	inspect, err := importer.InspectWorkbook(path, importer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := importer.BestSheet(importer.KindAssignments, inspect); got != "2026年春学期" {
		t.Fatalf("assignment sheet = %q", got)
	}
	table, ok := inspect.Sheet("2026年春学期")
	if !ok {
		t.Fatal("missing sheet")
	}
	subjects := []store.Subject{
		{Key: "chinese", Name: "语文", Enabled: true},
		{Key: "math", Name: "数学", Enabled: true},
		{Key: "english", Name: "英语", Enabled: true},
		{Key: "physics", Name: "物理", Enabled: true},
		{Key: "chemistry", Name: "化学", Enabled: true},
	}
	fields := importer.FieldsForTable(importer.KindAssignments, table, subjects, nil)
	if len(fields) != 1 || fields[0].Key != importer.FieldClassName {
		t.Fatalf("fields = %+v", fields)
	}
	mapping := importer.AutoMappingFor(importer.KindAssignments, table, subjects, nil)
	if mapping[importer.FieldClassName] != "班级" {
		t.Fatalf("mapping = %+v", mapping)
	}
	data, issues, err := importer.ParseSheetFor(inspect, "2026年春学期", importer.KindAssignments, mapping, subjects, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if len(data.Assignments) != 15 {
		t.Fatalf("assignments = %+v", data.Assignments)
	}
	if len(data.TeacherList) != 15 {
		t.Fatalf("teachers = %+v", data.TeacherList)
	}
	if data.Assignments[0].ClassName != "537" || data.Assignments[0].Subject != "chinese" ||
		data.Assignments[0].TeacherName != "王丽" {
		t.Fatalf("first assignment = %+v", data.Assignments[0])
	}
	if len(data.Classes) != 4 {
		t.Fatalf("classes = %+v", data.Classes)
	}
	filtered, issues, err := importer.ParseSheetForGrade(inspect, "2026年春学期",
		importer.KindAssignments, mapping, subjects, nil, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 || len(filtered.Assignments) != 14 || len(filtered.Classes) != 3 {
		t.Fatalf("filtered = %+v issues = %+v", filtered.Assignments, issues)
	}
	other, _, err := importer.ParseSheetForGrade(inspect, "2026年春学期",
		importer.KindAssignments, mapping, subjects, nil, "g2")
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Assignments) != 1 || other.Assignments[0].ClassName != "601" {
		t.Fatalf("other grade = %+v", other.Assignments)
	}
}
