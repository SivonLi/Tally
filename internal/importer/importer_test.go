package importer

import (
	"testing"

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
	if geography.Name != "卓越+鹭宁" || geography.LeadClassName != "卓越" || geography.Target != 18 {
		t.Errorf("geography = %+v", geography)
	}
	if len(geography.MemberClasses) != 2 || geography.MemberClasses[0] != "卓越" || geography.MemberClasses[1] != "鹭宁" {
		t.Errorf("geography members = %v", geography.MemberClasses)
	}
	politics := bySubject["politics"]
	if politics.Name != "546+鹭宁" || politics.LeadClassName != "546" || politics.Target != 14 {
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
