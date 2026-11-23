package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// assertAccessibleWithin 检查可访问元素都在窗口范围内，并跳过滚动区外的元素。
func assertAccessibleWithin(t *testing.T, tt *ui.Tester, width, height float32) {
	t.Helper()
	for _, info := range tt.AccessAll() {
		if info.Offscreen || info.Bounds.W <= 0 || info.Bounds.H <= 0 {
			continue
		}
		box := info.Bounds
		if box.X < -0.5 || box.Y < -0.5 || box.X+box.W > width+0.5 || box.Y+box.H > height+0.5 {
			t.Errorf("%v %q 超出窗口：%+v", info.Role, info.Name, box)
		}
	}
}

func assertTableRowHeight(t *testing.T, state *ui.ListState, name string) {
	t.Helper()
	if state.RowHeight != 40 {
		t.Fatalf("%s 行高 = %v，期望 40", name, state.RowHeight)
	}
}

func TestThirdBatchNavigationHeights(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.page = pageOverview
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	tt.Frame()
	nav, ok := tt.AccessByRole(ui.RoleButton, "Overview")
	if !ok {
		t.Fatal("找不到主导航 Overview")
	}
	if nav.Bounds.H != 36 {
		t.Fatalf("主导航行高 = %v，期望 36", nav.Bounds.H)
	}

	a.settingsPageID = "sync"
	settings := ui.NewTester(a.settingsView, 760, 620)
	settings.SetLocale("en")
	settings.Frame()
	item, ok := settings.AccessByRole(ui.RoleButton, "Subjects")
	if !ok {
		t.Fatal("找不到设置导航 Subjects")
	}
	if item.Bounds.H != 36 {
		t.Fatalf("设置导航行高 = %v，期望 36", item.Bounds.H)
	}
}

func TestThirdBatchTableRowHeights(t *testing.T) {
	a := newPopulatedTestApp(t)
	render := func(page pageID) *ui.Tester {
		a.page = page
		tt := ui.NewTester(a.view, 1280, 800)
		tt.SetLocale("en")
		tt.Frame()
		return tt
	}

	render(pageCohorts)
	assertTableRowHeight(t, &a.classTable, "总览班级表")

	render(pageStudents)
	assertTableRowHeight(t, &a.studentTable, "学生表")

	a.facultyTab = 0
	render(pageFaculty)
	assertTableRowHeight(t, &a.teacherTable, "教师表")
	a.facultyTab = 1
	render(pageFaculty)
	assertTableRowHeight(t, &a.groupTable, "教学组表")

	render(pageBaseline)
	assertTableRowHeight(t, &a.baselineTable, "基线表")

	for tab, name := range map[int]string{0: "学科结果表", 1: "班级结果表", 2: "教师结果表"} {
		a.assessmentResultTab = tab
		render(pageAssessments)
		switch tab {
		case 0:
			assertTableRowHeight(t, &a.resultTable, name)
		case 1:
			assertTableRowHeight(t, &a.classResultTable, name)
		case 2:
			assertTableRowHeight(t, &a.teacherResultTable, name)
		}
	}

	a.selectedExamID = a.exams[0].ID
	for tab, name := range map[int]string{0: "考生表", 1: "成绩表", 3: "目标表"} {
		a.examTab = tab
		render(pageExams)
		switch tab {
		case 0:
			assertTableRowHeight(t, &a.studentTable, name)
		case 1:
			assertTableRowHeight(t, &a.baselineTable, name)
		case 3:
			assertTableRowHeight(t, &a.classTable, name)
		}
	}
}

func TestThirdBatchPlanControlHeights(t *testing.T) {
	a := newTestApp(t)
	a.page = pagePlans
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	field, ok := tt.AccessByRole(ui.RoleTextField, "Plan name")
	if !ok {
		t.Fatal("找不到 Plan name 输入框")
	}
	if field.Bounds.H != 36 {
		t.Fatalf("Plan name 高度 = %v，期望 36", field.Bounds.H)
	}
	save, ok := tt.AccessByRole(ui.RoleButton, "Save")
	if !ok {
		t.Fatal("找不到页面级 Save 按钮")
	}
	if save.Bounds.H != 44 {
		t.Fatalf("页面级 Save 高度 = %v，期望 44", save.Bounds.H)
	}
}
