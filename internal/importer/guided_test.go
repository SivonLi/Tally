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
