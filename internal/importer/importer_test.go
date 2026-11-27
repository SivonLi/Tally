package importer

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"tally/internal/excel"
	"tally/internal/store"
)

func TestParseMergeNotesReadsEveryNoteInARow(t *testing.T) {
	tables := []excel.Table{{
		Sheet: "本科达线率计算",
		Rows: []excel.TableRow{{
			Number: 27,
			Values: []string{"地理张丽芬：走班卓越与鹭宁合并计算（14+4），政治王玲燕：走班546与鹭宁合并计算（0+14）"},
		}},
	}}
	groups, warnings := parseMergeNotes(tables, store.ImportData{}, nil)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	bySubject := map[string]store.ImportStatGroup{}
	for _, group := range groups {
		bySubject[group.Subject] = group
	}
	geography := bySubject["geography"]
	if geography.Name != "卓越+鹭宁" || geography.LeadClassName != "鹭宁" || geography.Target != 18 {
		t.Errorf("geography = %+v", geography)
	}
	if len(geography.MemberClasses) != 2 || geography.MemberClasses[0] != "卓越" || geography.MemberClasses[1] != "鹭宁" {
		t.Errorf("geography members = %v", geography.MemberClasses)
	}
	politics := bySubject["politics"]
	if politics.Name != "546+鹭宁" || politics.LeadClassName != "鹭宁" || politics.Target != 14 {
		t.Errorf("politics = %+v", politics)
	}
}

func TestParseMergeNotesWarnsWithoutNumbers(t *testing.T) {
	tables := []excel.Table{{
		Sheet: "均分计算",
		Rows: []excel.TableRow{{
			Number: 25,
			Values: []string{"备注：546班政治走班和鹭宁班政治合并计算"},
		}},
	}}
	groups, warnings := parseMergeNotes(tables, store.ImportData{}, nil)
	if len(groups) != 0 {
		t.Fatalf("groups = %+v", groups)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestResolveClassAliasesByHeadcount(t *testing.T) {
	data := store.ImportData{
		ClassTargets: []store.ImportClassTarget{
			{ClassName: "卓越"},
			{ClassName: "鹭宁"},
		},
	}
	for index := 0; index < 24; index++ {
		data.Students = append(data.Students, store.ImportStudent{ClassName: "800"})
	}
	for index := 0; index < 16; index++ {
		data.Students = append(data.Students, store.ImportStudent{ClassName: "900"})
	}
	tables := []excel.Table{{
		Sheet:   "均分计算",
		Headers: []string{"班级", "班级人数"},
		Rows: []excel.TableRow{
			{Number: 2, Values: []string{"卓越", "24"}},
			{Number: 3, Values: []string{"鹭宁", "16"}},
		},
	}}
	aliases := resolveClassAliases(tables, data)
	if aliases["800"] != "卓越" {
		t.Errorf("800 = %q", aliases["800"])
	}
	if aliases["900"] != "鹭宁" {
		t.Errorf("900 = %q", aliases["900"])
	}
}

func TestParseStudentsTreatsZeroAsMissingSubject(t *testing.T) {
	table := excel.Table{
		Sheet:   "学生成绩",
		Headers: []string{"班级", "考号", "姓名", "总分", "语文", "数学"},
		Rows: []excel.TableRow{{
			Number: 2,
			Values: []string{"A", "001", "Student", "200", "0", "100"},
		}},
	}
	students, err := parseStudents(table)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 {
		t.Fatalf("students = %+v", students)
	}
	if _, ok := students[0].Scores["chinese"]; ok {
		t.Errorf("chinese score should be missing: %+v", students[0].Scores)
	}
	if students[0].Flags["chinese"] != "zero" {
		t.Errorf("flags = %+v", students[0].Flags)
	}
	if students[0].Scores["math"] != 100 {
		t.Errorf("math = %v", students[0].Scores["math"])
	}
}

func TestParseTargetsDistinguishesCutoffActualFromTargets(t *testing.T) {
	table := excel.Table{
		Sheet: "期末完成率计算",
		Headers: []string{
			"班级", "清北640/660", "C9-610/630", "985-590/570", "211-560/530", "特控-460/475", "本科-345/385",
			"清北", "C9-2%", "985-3%", "211-7%", "特控-27%", "本科",
		},
		Rows: []excel.TableRow{{
			Number: 9,
			Values: []string{"1班", "2", "3", "9", "7", "16", "1", "1", "11", "16", "5", "0", "0"},
		}},
	}
	targets, err := parseTargets(table)
	if err != nil {
		t.Fatalf("parseTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	got := targets[0]
	if got.Counts["qingbei"] == nil || *got.Counts["qingbei"] != 1 ||
		got.Counts["c9"] == nil || *got.Counts["c9"] != 11 ||
		got.Counts["project985"] == nil || *got.Counts["project985"] != 16 ||
		got.Counts["project211"] == nil || *got.Counts["project211"] != 5 {
		t.Fatalf("unexpected target counts: %+v", got.Counts)
	}
	if got.ActualCounts["qingbei"] == nil || *got.ActualCounts["qingbei"] != 2 ||
		got.ActualCounts["c9"] == nil || *got.ActualCounts["c9"] != 3 ||
		got.ActualCounts["project985"] == nil || *got.ActualCounts["project985"] != 9 ||
		got.ActualCounts["project211"] == nil || *got.ActualCounts["project211"] != 7 ||
		got.ActualCounts["special_control"] == nil || *got.ActualCounts["special_control"] != 16 ||
		got.ActualCounts["undergraduate"] == nil || *got.ActualCounts["undergraduate"] != 1 {
		t.Fatalf("unexpected actual counts: %+v", got.ActualCounts)
	}
}

func TestImportWorkspacePrefersTargetSheetAndPassOrganizeCutoffs(t *testing.T) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("close workbook: %v", err)
		}
	}()

	// 1. 达线整理表（合成 8 条总分线：无清北/C9）
	f.SetSheetName("Sheet1", "期末总分、单科达线整理")
	if err := f.SetSheetRow("期末总分、单科达线整理", "A1", &[]any{
		"班级", "选科", "考号", "姓名", "总分",
		"清北/", "C9-/", "985-600/580", "211-550/530", "特控-460/470", "本科-350/380",
		"语文", "85/85",
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("期末总分、单科达线整理", "A2", &[]any{
		"1班", "物理", "1001", "学生甲", 600, 0, 0, 1, 0, 0, 0, 100, 1,
	}); err != nil {
		t.Fatal(err)
	}

	// 2. 完成率计算表（含旧表头 12 条总分线，且无班级目标任务列）
	if _, err := f.NewSheet("期末完成率计算"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("期末完成率计算", "A1", &[]any{
		"班级", "清北650/660", "C9-620/630", "985-580/590", "211-520/530", "特控-480/490", "本科-370/390",
		"清北", "C9-2%", "985-3%", "211-7%", "特控-27%", "本科",
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("期末完成率计算", "A2", &[]any{
		"1班", 0, 0, 9, 7, 16, 1, 1, 11, 16, 5, 0, 0,
	}); err != nil {
		t.Fatal(err)
	}

	// 3. 目标任务表（含班级目标任务列）
	if _, err := f.NewSheet("高考目标任务"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("高考目标任务", "A1", &[]any{
		"班级", "清北", "C9-2%", "985-3%", "211-7%", "特控-27%", "本科", "班级目标任务",
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("高考目标任务", "A2", &[]any{
		"1班", 1, 11, 16, 5, 0, 0, 33,
	}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "synthetic_qimo.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	data, _, err := ImportWorkspace(path, Options{})
	if err != nil {
		t.Fatalf("ImportWorkspace: %v", err)
	}
	if len(data.TierCutoffs) != 8 {
		t.Fatalf("expected 8 tier cutoffs from 达线整理, got %d: %+v", len(data.TierCutoffs), data.TierCutoffs)
	}
	for _, tc := range data.TierCutoffs {
		if tc.TierKey == "project985" && tc.Track == "physics" && tc.Value != 600 {
			t.Fatalf("expected 985 physics cutoff 600 from 达线整理, got %v", tc.Value)
		}
	}
	if len(data.ClassTargets) != 1 || data.ClassTargets[0].Counts["single_subject"] == nil || *data.ClassTargets[0].Counts["single_subject"] != 33 {
		t.Fatalf("expected single_subject target 33 from 高考目标任务, got %+v", data.ClassTargets)
	}
}
