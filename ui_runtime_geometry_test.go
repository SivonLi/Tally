package main

import (
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func useChineseUI(t *testing.T) {
	t.Helper()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
}

func newChineseTester(t *testing.T, view func(*ui.Context), width, height int) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(view, width, height)
	tt.SetLocale("zh-Hans")
	settleTester(tt)
	if tt.FramePending() {
		t.Fatalf("界面未稳定：%v", tt.Texts())
	}
	return tt
}

func failGeometry(t *testing.T, tt *ui.Tester, name, message string) {
	t.Helper()
	path := filepath.Join(".scratch", "ui-failures", strings.NewReplacer("/", "_", " ", "_").Replace(name)+".png")
	savePNG(t, path, tt.Image())
	t.Errorf("%s：%s（截图：%s）", name, message, path)
}

func rectRight(r ui.Rect) float32 {
	return r.X + r.W
}

func rectBottom(r ui.Rect) float32 {
	return r.Y + r.H
}

func findNode(tt *ui.Tester, role ui.Role, name string) (ui.AccessInfo, bool) {
	return tt.AccessByRole(role, name)
}

func contentNodes(tt *ui.Tester, minX, minY float32) []ui.AccessInfo {
	roles := map[ui.Role]bool{
		ui.RoleText:        true,
		ui.RoleButton:      true,
		ui.RoleTextField:   true,
		ui.RolePopUpButton: true,
		ui.RoleRadio:       true,
		ui.RoleRadioGroup:  true,
		ui.RoleSwitch:      true,
		ui.RoleStepper:     true,
		ui.RoleTable:       true,
		ui.RoleHeading:     true,
	}
	var nodes []ui.AccessInfo
	for _, info := range tt.AccessAll() {
		if info.Offscreen || info.Bounds.W <= 0 || info.Bounds.H <= 0 {
			continue
		}
		if !roles[info.Role] {
			continue
		}
		if info.Bounds.X < minX || info.Bounds.Y < minY {
			continue
		}
		nodes = append(nodes, info)
	}
	return nodes
}

func measureTextWidth(t *testing.T, text string, size float32) float32 {
	t.Helper()
	return measureTextWidthWithWeight(t, text, size, 0)
}

func measureTextWidthWithWeight(t *testing.T, text string, size float32, weight int) float32 {
	t.Helper()
	width := float32(-1)
	view := func(c *ui.Context) {
		applySunbeamTheme(c)
		if width < 0 {
			width, _ = c.MeasureText(0, ui.Span{Text: text, Font: c.Theme().Font, Size: size, Weight: weight})
		}
		ui.Text(c, "measure")
	}
	tt := ui.NewTester(view, 200, 80)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	if width < 0 {
		t.Fatalf("无法测量文字宽度：%q", text)
	}
	return width
}

func fillStudents(t *testing.T, a *app, count int) {
	t.Helper()
	if a.selectedStage() == nil {
		t.Fatal("没有选中学期")
	}
	if len(a.students) == 0 {
		t.Fatal("没有学生数据")
	}
	for index := 0; index < count; index++ {
		a.students = append(a.students, store.DataStudent{
			ID:        int64(10000 + index),
			StudentNo: fmt.Sprintf("%03d", index),
			Name:      "学生",
			ClassID:   a.students[0].ClassID,
			ClassName: a.students[0].ClassName,
			Track:     "physics",
			Status:    "active",
		})
	}
	a.studentsTotal = len(a.students)
}

func TestRuntimeScrollStaysInsideItsContainer(t *testing.T) {
	useChineseUI(t)

	t.Run("学生页", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageStudents
		fillStudents(t, a, 60)
		tt := newChineseTester(t, a.view, 1024, 640)

		heading, ok := findNode(tt, ui.RoleHeading, locales.NavStudents())
		if !ok {
			t.Fatalf("找不到学生页标题")
		}
		search, ok := findNode(tt, ui.RoleTextField, locales.DataSearchStudents())
		if !ok {
			t.Fatalf("找不到学生页搜索框")
		}
		filterBottom := max(rectBottom(heading.Bounds), rectBottom(search.Bounds))
		for _, name := range []string{
			locales.DataFilterClass(), locales.DataFilterTrack(), locales.DataFilterStatus(),
		} {
			if filter, ok := findNode(tt, ui.RolePopUpButton, name); ok {
				filterBottom = max(filterBottom, rectBottom(filter.Bounds))
			}
		}

		tt.Scroll(500, 450, 0, 200)
		settleTester(tt)

		table, ok := findNode(tt, ui.RoleTable, locales.NavStudents())
		if !ok {
			t.Fatalf("找不到学生表格")
		}
		if table.Bounds.Y < filterBottom {
			failGeometry(t, tt, "学生页滚动穿透", fmt.Sprintf(
				"表格顶边 %.1f 高于筛选栏底边 %.1f，侵入页头区域", table.Bounds.Y, filterBottom))
		}
		if rectBottom(table.Bounds) > 640 {
			failGeometry(t, tt, "学生页滚动穿透", fmt.Sprintf(
				"表格底边 %.1f 超出窗口高度 640", rectBottom(table.Bounds)))
		}
		if table.Bounds.Y < rectBottom(heading.Bounds) {
			failGeometry(t, tt, "学生页滚动穿透", fmt.Sprintf(
				"表格顶边 %.1f 与页头底边 %.1f 重叠", table.Bounds.Y, rectBottom(heading.Bounds)))
		}
	})

	settingsCases := []struct {
		name  string
		page  string
		title string
	}{
		{name: "设置云端同步", page: "sync", title: locales.SettingsSync()},
		{name: "设置学科", page: "subjects", title: locales.SettingsSubjects()},
	}
	for _, tc := range settingsCases {
		t.Run(tc.name, func(t *testing.T) {
			a := newPopulatedTestApp(t)
			a.settingsPageID = tc.page
			tt := newChineseTester(t, a.settingsView, 760, 620)

			tt.Scroll(500, 400, 0, 200)
			settleTester(tt)

			var contentScroll *ui.AccessInfo
			for _, info := range tt.AccessAll() {
				if info.Role == ui.RoleScroll && info.Bounds.X >= settingsSidebarWidth && info.Bounds.W > 0 && info.Bounds.H > 0 {
					copied := info
					contentScroll = &copied
					break
				}
			}
			if contentScroll == nil {
				failGeometry(t, tt, tc.name+"缺少滚动区", "找不到右侧内容滚动容器")
				return
			}
			if contentScroll.Bounds.Y < topStripHeight {
				failGeometry(t, tt, tc.name+"滚动穿透", fmt.Sprintf(
					"滚动区顶边 %.1f 高于页头底边 %d，侵入了页头区域",
					contentScroll.Bounds.Y, topStripHeight))
			}
			if rectBottom(contentScroll.Bounds) > 620 {
				failGeometry(t, tt, tc.name+"滚动溢出", fmt.Sprintf(
					"滚动区底边 %.1f 超出窗口高度 620", rectBottom(contentScroll.Bounds)))
			}
			if header, ok := findNode(tt, ui.RoleText, tc.title); !ok {
				failGeometry(t, tt, tc.name+"缺少标题", "滚动后找不到页头标题 "+tc.title)
			} else if rectBottom(header.Bounds) > contentScroll.Bounds.Y+1 {
				failGeometry(t, tt, tc.name+"标题重叠", fmt.Sprintf(
					"页头标题底边 %.1f 越过滚动区顶边 %.1f", rectBottom(header.Bounds), contentScroll.Bounds.Y))
			}
		})
	}

	t.Run("总览页班级表不被Scroll包裹且在容器内", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageCohorts
		tt := newChineseTester(t, a.view, 1024, 640)

		table, ok := findNode(tt, ui.RoleTable, locales.CohortClasses())
		if !ok {
			t.Fatalf("找不到总览班级表")
		}
		for _, info := range tt.AccessAll() {
			if info.Role == ui.RoleScroll && info.Bounds.X >= table.Bounds.X-20 && rectRight(info.Bounds) >= rectRight(table.Bounds) {
				failGeometry(t, tt, "总览班级表外层嵌套Scroll", fmt.Sprintf(
					"总览班级表外层存在 RoleScroll %.1f,%.1f (%.1fx%.1f)，形成嵌套滚动",
					info.Bounds.X, info.Bounds.Y, info.Bounds.W, info.Bounds.H))
			}
		}
		if rectBottom(table.Bounds) > 640 {
			failGeometry(t, tt, "总览班级表溢出窗口", fmt.Sprintf(
				"表格底边 %.1f 超出窗口高度 640", rectBottom(table.Bounds)))
		}

		tt.Scroll(table.Bounds.X+50, table.Bounds.Y+50, 0, 100)
		settleTester(tt)
		if rectBottom(table.Bounds) > 640 {
			failGeometry(t, tt, "总览班级表滚动后溢出窗口", fmt.Sprintf(
				"滚动后表格底边 %.1f 超出窗口高度 640", rectBottom(table.Bounds)))
		}
	})

	t.Run("本次考核结果表不被Scroll包裹且在容器内", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageAssessments
		tt := newChineseTester(t, a.view, 1024, 640)

		table, ok := findNode(tt, ui.RoleTable, locales.AssessmentViewBySubject())
		if !ok {
			t.Fatalf("找不到考核结果表")
		}
		for _, info := range tt.AccessAll() {
			if info.Role == ui.RoleScroll && info.Bounds.X >= table.Bounds.X-20 && rectRight(info.Bounds) >= rectRight(table.Bounds) {
				failGeometry(t, tt, "考核结果表外层嵌套Scroll", fmt.Sprintf(
					"考核结果表外层存在 RoleScroll %.1f,%.1f (%.1fx%.1f)，形成嵌套滚动",
					info.Bounds.X, info.Bounds.Y, info.Bounds.W, info.Bounds.H))
			}
		}
		if rectBottom(table.Bounds) > 640 {
			failGeometry(t, tt, "考核结果表溢出窗口", fmt.Sprintf(
				"表格底边 %.1f 超出窗口高度 640", rectBottom(table.Bounds)))
		}

		tt.Scroll(table.Bounds.X+50, table.Bounds.Y+50, 0, 100)
		settleTester(tt)
		if rectBottom(table.Bounds) > 640 {
			failGeometry(t, tt, "考核结果表滚动后溢出窗口", fmt.Sprintf(
				"滚动后表格底边 %.1f 超出窗口高度 640", rectBottom(table.Bounds)))
		}
	})
}

func TestRuntimeTwoLineListItemsFitTheirContent(t *testing.T) {
	useChineseUI(t)

	t.Run("考试记录双行高度", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageExams
		a.selectedExamID = 0
		tt := newChineseTester(t, a.view, 1280, 800)

		if len(a.exams) == 0 {
			t.Fatal("没有考试数据")
		}
		exam := a.exams[0]
		row, ok := findNode(tt, ui.RoleButton, a.examBatchLabel(exam))
		if !ok {
			t.Fatalf("找不到考试记录行：%q", a.examBatchLabel(exam))
		}
		if row.Bounds.H < 56 {
			failGeometry(t, tt, "考试记录双行高度", fmt.Sprintf(
				"考试记录行高 %.1f，低于双行列表项下限 56", row.Bounds.H))
		}
		if !tt.HasText(exam.Name) {
			failGeometry(t, tt, "考试记录双行高度", fmt.Sprintf("缺少主标题 %q", exam.Name))
		}
		if exam.ExamDate != "" && !tt.HasText(exam.ExamDate) {
			failGeometry(t, tt, "考试记录双行高度", fmt.Sprintf("缺少副标题日期 %q", exam.ExamDate))
		}
		status := a.examStatusName(exam.Status)
		if !tt.HasText(status) {
			failGeometry(t, tt, "考试记录双行高度", fmt.Sprintf("缺少状态标签 %q", status))
		}
	})

	t.Run("本次考核列表项高度", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageAssessments
		tt := newChineseTester(t, a.view, 1280, 800)
		if len(a.assessments) == 0 {
			t.Fatal("没有考核数据")
		}
		item := a.assessments[0]
		row, ok := findNode(tt, ui.RoleButton, a.assessmentOverviewLabel(item))
		if !ok {
			t.Fatalf("找不到考核列表项：%q", a.assessmentOverviewLabel(item))
		}
		if row.Bounds.H < 56 {
			failGeometry(t, tt, "考核列表双行高度", fmt.Sprintf(
				"考核列表项行高 %.1f，低于双行列表项下限 56", row.Bounds.H))
		}
	})

	t.Run("总览页近期考试项高度", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageOverview
		tt := newChineseTester(t, a.view, 1280, 800)
		if len(a.exams) == 0 {
			t.Fatal("没有考试数据")
		}
		exam := a.exams[0]
		row, ok := findNode(tt, ui.RoleButton, a.examBatchLabel(exam))
		if !ok {
			t.Fatalf("找不到总览考试记录行：%q", a.examBatchLabel(exam))
		}
		if row.Bounds.H < 56 {
			failGeometry(t, tt, "总览考试双行高度", fmt.Sprintf(
				"总览考试记录行高 %.1f，低于双行列表项下限 56", row.Bounds.H))
		}
	})
}

func TestRuntimeNumberStepperWidth(t *testing.T) {
	useChineseUI(t)

	t.Run("新建考试步进框宽度", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageExams
		a.openNewExam()
		tt := newChineseTester(t, a.view, 1280, 800)

		checkedSubjects := 0
		for _, subject := range a.subjects {
			if !subject.Enabled {
				continue
			}
			name := a.subjectName(subject.Key, subject.Name)
			field, ok := findNode(tt, ui.RoleTextField, name)
			if !ok {
				t.Fatalf("找不到学科满分步进框：%q", name)
			}
			var label ui.AccessInfo
			found := false
			for _, info := range tt.AccessAll() {
				if info.Role != ui.RoleText || info.Name != name {
					continue
				}
				if info.Bounds.Y < rectBottom(field.Bounds)-1 || info.Bounds.Y > rectBottom(field.Bounds)+40 {
					continue
				}
				if !found || info.Bounds.Y < label.Bounds.Y {
					label = info
					found = true
				}
			}
			if !found {
				t.Fatalf("找不到学科标签：%q", name)
			}
			// 学科标签与步进框同处一个固定宽度列，标签宽度就是步进框可用的列宽。
			if label.Bounds.W < 112 {
				failGeometry(t, tt, "新建考试步进框宽度", fmt.Sprintf(
					"学科 %q 的步进框列宽 %.1f，低于 112", name, label.Bounds.W))
			}
			value := strconv.FormatFloat(subject.FullMark, 'f', -1, 64)
			valueWidth := measureTextWidth(t, value, 16)
			if field.Bounds.W < valueWidth {
				failGeometry(t, tt, "新建考试步进框宽度", fmt.Sprintf(
					"学科 %q 的数字区域宽 %.1f，容纳不下 %q 所需的 %.1f", name, field.Bounds.W, value, valueWidth))
			}
			checkedSubjects++
		}
		if checkedSubjects == 0 {
			t.Fatal("没有启用的学科")
		}
	})

	t.Run("设置页学科满分步进框宽度", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.settingsPageID = "subjects"
		tt := newChineseTester(t, a.settingsView, 760, 620)

		field, ok := findNode(tt, ui.RoleTextField, locales.SubjectFullMark())
		if !ok {
			t.Fatalf("找不到设置学科满分输入框")
		}
		if field.Bounds.W < 112 {
			failGeometry(t, tt, "设置学科满分宽度", fmt.Sprintf(
				"设置学科满分输入框宽 %.1f，低于 112", field.Bounds.W))
		}
	})
}

func TestRuntimeSplitSpacingAndImportGap(t *testing.T) {
	useChineseUI(t)

	t.Run("本次考核分栏", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageAssessments
		tt := newChineseTester(t, a.view, 1280, 800)

		divider, ok := findNode(tt, ui.RoleSplitter, "")
		if !ok {
			t.Fatalf("找不到分栏分割线")
		}
		if len(a.assessments) == 0 {
			t.Fatal("没有考核数据")
		}
		row, ok := findNode(tt, ui.RoleButton, a.assessmentOverviewLabel(a.assessments[0]))
		if !ok {
			t.Fatalf("找不到左侧考试列表项")
		}
		if gap := divider.Bounds.X - rectRight(row.Bounds); gap < 16 {
			failGeometry(t, tt, "本次考核分栏间距", fmt.Sprintf(
				"左侧列表项右边界 %.1f 与分割线 %.1f 的间距为 %.1f，低于 16",
				rectRight(row.Bounds), divider.Bounds.X, gap))
		}
		labelWidth := measureTextWidth(t, row.Name, 16)
		if labelWidth > row.Bounds.W-24 {
			failGeometry(t, tt, "本次考核分栏间距", fmt.Sprintf(
				"左侧列表文字 %q 需要 %.1f，列表项仅提供 %.1f", row.Name, labelWidth, row.Bounds.W-24))
		}

		dividerRight := rectRight(divider.Bounds)
		minX := float32(-1)
		for _, info := range contentNodes(tt, dividerRight, divider.Bounds.Y) {
			if info.Bounds.X <= divider.Bounds.X {
				continue
			}
			if minX < 0 || info.Bounds.X < minX {
				minX = info.Bounds.X
			}
		}
		if minX >= 0 {
			if gap := minX - dividerRight; gap < 16 {
				failGeometry(t, tt, "本次考核分栏间距", fmt.Sprintf(
					"右侧详情最左内容位于 %.1f，与分割线右边界 %.1f 的间距为 %.1f，低于 16",
					minX, dividerRight, gap))
			}
		}
	})

	t.Run("导入记录留白", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageImports
		a.importCenterTab = 2
		if len(a.importRuns) < 4 {
			t.Fatalf("导入记录不足 4 条：%d", len(a.importRuns))
		}
		a.importRuns = a.importRuns[:4]
		tt := newChineseTester(t, a.view, 1280, 800)

		search, ok := findNode(tt, ui.RoleTextField, locales.DataSearchImports())
		if !ok {
			t.Fatalf("找不到导入记录搜索框")
		}
		searchBottom := rectBottom(search.Bounds)
		first, ok := findNode(tt, ui.RoleText, importFileLabel(a.importRuns[0]))
		if !ok || first.Bounds.Y < searchBottom {
			t.Fatalf("找不到第一条导入记录 %q", importFileLabel(a.importRuns[0]))
		}
		if gap := first.Bounds.Y - searchBottom; gap > 32 {
			failGeometry(t, tt, "导入记录留白", fmt.Sprintf(
				"搜索栏底边 %.1f 到第一条记录顶边 %.1f 的间距为 %.1f，超过 32",
				searchBottom, first.Bounds.Y, gap))
		}
	})

	t.Run("年级总览分栏", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageCohorts
		tt := newChineseTester(t, a.view, 1280, 800)

		divider, ok := findNode(tt, ui.RoleSplitter, "")
		if !ok {
			t.Fatalf("找不到年级分栏分割线")
		}
		dividerRight := rectRight(divider.Bounds)
		minRightX := float32(-1)
		for _, info := range contentNodes(tt, dividerRight, divider.Bounds.Y) {
			if info.Bounds.X <= divider.Bounds.X {
				continue
			}
			if minRightX < 0 || info.Bounds.X < minRightX {
				minRightX = info.Bounds.X
			}
		}
		if minRightX >= 0 {
			if gap := minRightX - dividerRight; gap < 16 {
				failGeometry(t, tt, "年级分栏间距", fmt.Sprintf(
					"右侧详情最左内容位于 %.1f，与分割线右边界 %.1f 的间距为 %.1f，低于 16",
					minRightX, dividerRight, gap))
			}
		}
	})
}

func TestRuntimeNoRepeatedAdjacentText(t *testing.T) {
	useChineseUI(t)

	cases := []struct {
		name string
		view func(*testing.T) *ui.Tester
	}{
		{
			name: "成绩与分数线详情页",
			view: func(t *testing.T) *ui.Tester {
				f := newExamPageFixture(t)
				f.App.page = pageExams
				f.App.selectExam(f.ExamID)
				return newChineseTester(t, f.App.view, 1280, 800)
			},
		},
		{
			name: "本次考核页",
			view: func(t *testing.T) *ui.Tester {
				a := newPopulatedTestApp(t)
				a.page = pageAssessments
				return newChineseTester(t, a.view, 1280, 800)
			},
		},
		{
			name: "导入记录页",
			view: func(t *testing.T) *ui.Tester {
				a := newPopulatedTestApp(t)
				a.page = pageImports
				return newChineseTester(t, a.view, 1280, 800)
			},
		},
		{
			name: "学生页",
			view: func(t *testing.T) *ui.Tester {
				a := newPopulatedTestApp(t)
				a.page = pageStudents
				return newChineseTester(t, a.view, 1280, 800)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.view(t)
			texts := tt.Texts()
			accessible := make(map[string]int)
			for _, info := range tt.AccessAll() {
				if info.Name != "" {
					accessible[info.Name]++
				}
			}
			var duplicates []string
			for index := 0; index < len(texts); {
				end := index + 1
				for end < len(texts) && texts[end] == texts[index] {
					end++
				}
				repeated := end - index
				if texts[index] != "" && repeated >= 3 &&
					repeated > accessible[texts[index]]+1 {
					duplicates = append(duplicates, fmt.Sprintf(
						"%q 连续出现 %d 次（无障碍节点 %d 个）",
						texts[index], repeated, accessible[texts[index]]))
				}
				index = end
			}
			if len(duplicates) > 0 {
				failGeometry(t, tt, tc.name+"重复文案", strings.Join(duplicates, "；"))
			}
		})
	}
}

func TestRuntimeSearchAndSelectWidthsAndSidebar(t *testing.T) {
	useChineseUI(t)

	t.Run("成绩与分数线详情页控件宽度", func(t *testing.T) {
		f := newExamPageFixture(t)
		f.App.page = pageExams
		f.App.selectExam(f.ExamID)
		f.App.examCohortID = f.CohortID
		f.App.reloadExamData(t.Context())
		tt := newChineseTester(t, f.App.view, 1280, 800)

		selectBox, ok := findNode(tt, ui.RolePopUpButton, locales.ExamDefaultTrack())
		if !ok {
			t.Fatalf("找不到选科下拉框")
		}
		if selectBox.Bounds.W < 128 {
			failGeometry(t, tt, "选科下拉框宽度", fmt.Sprintf(
				"选科下拉框宽度 %.1f，低于 128", selectBox.Bounds.W))
		}

		search, ok := findNode(tt, ui.RoleTextField, locales.ExamSearchRoster())
		if !ok {
			t.Fatalf("找不到报名名单搜索框")
		}
		placeholder := search.Placeholder
		if placeholder == "" {
			placeholder = search.Name
		}
		need := measureTextWidth(t, placeholder, 16)
		if search.Bounds.W < need {
			failGeometry(t, tt, "搜索框占位文字", fmt.Sprintf(
				"搜索框宽 %.1f，容纳不下占位文字 %q 所需的 %.1f", search.Bounds.W, placeholder, need))
		}
	})

	t.Run("学生页与数据页控件宽度", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageStudents
		tt := newChineseTester(t, a.view, 1280, 800)

		for _, name := range []string{locales.DataFilterTrack(), locales.DataFilterStatus()} {
			box, ok := findNode(tt, ui.RolePopUpButton, name)
			if !ok {
				t.Fatalf("找不到学生页下拉框：%q", name)
			}
			if box.Bounds.W < 128 {
				failGeometry(t, tt, "学生页下拉框宽度", fmt.Sprintf(
					"下拉框 %q 宽 %.1f，低于 128", name, box.Bounds.W))
			}
		}

		search, ok := findNode(tt, ui.RoleTextField, locales.DataSearchStudents())
		if !ok {
			t.Fatalf("找不到学生页搜索框")
		}
		need := measureTextWidth(t, locales.DataSearchStudents(), 16)
		if search.Bounds.W < need {
			failGeometry(t, tt, "学生页搜索框宽度", fmt.Sprintf(
				"搜索框宽 %.1f，容纳不下占位文字 %q 所需的 %.1f", search.Bounds.W, locales.DataSearchStudents(), need))
		}
	})

	t.Run("基线与导入搜索框占位文字宽度", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageBaseline
		tt := newChineseTester(t, a.view, 1280, 800)
		search, ok := findNode(tt, ui.RoleTextField, locales.DataSearchBaseline())
		if !ok {
			t.Fatalf("找不到基线搜索框")
		}
		need := measureTextWidth(t, locales.DataSearchBaseline(), 16)
		if search.Bounds.W < need {
			failGeometry(t, tt, "基线搜索框宽度", fmt.Sprintf(
				"搜索框宽 %.1f，容纳不下占位文字 %q 所需的 %.1f", search.Bounds.W, locales.DataSearchBaseline(), need))
		}

		a.page = pageImports
		a.importCenterTab = 2
		tt = newChineseTester(t, a.view, 1280, 800)
		search, ok = findNode(tt, ui.RoleTextField, locales.DataSearchImports())
		if !ok {
			t.Fatalf("找不到导入记录搜索框")
		}
		need = measureTextWidth(t, locales.DataSearchImports(), 16)
		if search.Bounds.W < need {
			failGeometry(t, tt, "导入记录搜索框宽度", fmt.Sprintf(
				"搜索框宽 %.1f，容纳不下占位文字 %q 所需的 %.1f", search.Bounds.W, locales.DataSearchImports(), need))
		}
	})

	t.Run("成绩页学科与班级下拉宽度", func(t *testing.T) {
		f := newExamPageFixture(t)
		f.App.page = pageExams
		f.App.selectExam(f.ExamID)
		f.App.examCohortID = f.CohortID
		f.App.examTab = 1 // 成绩与赋分
		f.App.reloadExamData(t.Context())
		tt := newChineseTester(t, f.App.view, 1280, 800)

		for _, name := range []string{locales.DataFilterSubject(), locales.DataFilterClass()} {
			box, ok := findNode(tt, ui.RolePopUpButton, name)
			if !ok {
				t.Fatalf("找不到成绩页下拉框：%q", name)
			}
			if box.Bounds.W < 128 {
				failGeometry(t, tt, "成绩页下拉框宽度", fmt.Sprintf(
					"下拉框 %q 宽 %.1f，低于 128", name, box.Bounds.W))
			}
		}
	})

	t.Run("最小窗口侧边栏完整性", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageOverview
		tt := newChineseTester(t, a.view, 1024, 640)

		var navScroll *ui.AccessInfo
		for _, info := range tt.AccessAll() {
			if info.Role == ui.RoleScroll && info.Bounds.X < sidebarWidth && info.Bounds.W > 0 && info.Bounds.H > 0 {
				copy := info
				navScroll = &copy
				break
			}
		}
		if navScroll == nil {
			t.Fatal("找不到侧边栏导航滚动容器")
		}

		bottomActions := map[string]bool{
			locales.NavHelp():     true,
			locales.NavSettings(): true,
		}
		for _, label := range []string{
			locales.NavOverview(), locales.NavDataImport(), locales.NavBaseData(),
			locales.NavExamHub(), locales.NavQuality(), locales.NavAnalysis(),
			locales.NavPlans(), locales.NavHelp(), locales.NavSettings(),
		} {
			item, ok := findNode(tt, ui.RoleButton, label)
			if !ok {
				failGeometry(t, tt, "侧边栏完整性", fmt.Sprintf("找不到导航项 %q", label))
				continue
			}
			if item.Offscreen {
				failGeometry(t, tt, "侧边栏完整性", fmt.Sprintf("导航项 %q 位于滚动区之外", label))
				continue
			}
			if item.Bounds.H != 36 {
				failGeometry(t, tt, "侧边栏完整性", fmt.Sprintf(
					"导航项 %q 高度 %.1f，低于完整行高 36", label, item.Bounds.H))
			}
			if item.Bounds.X < 0 || rectRight(item.Bounds) > sidebarWidth ||
				item.Bounds.Y < 0 || rectBottom(item.Bounds) > 640 {
				failGeometry(t, tt, "侧边栏完整性", fmt.Sprintf(
					"导航项 %q 边界 %+v 超出侧边栏或窗口", label, item.Bounds))
			}
			if !bottomActions[label] &&
				(item.Bounds.Y < navScroll.Bounds.Y-0.5 || rectBottom(item.Bounds) > rectBottom(navScroll.Bounds)+0.5) {
				failGeometry(t, tt, "侧边栏完整性", fmt.Sprintf(
					"导航项 %q（%.1f–%.1f）超出导航滚动区可视范围（%.1f–%.1f），出现半截行",
					label, item.Bounds.Y, rectBottom(item.Bounds), navScroll.Bounds.Y, rectBottom(navScroll.Bounds)))
			}
		}
	})
}

func TestRuntimeAnalysisAndWorkbenchRegression(t *testing.T) {
	t.Run("分析页无原始JSON教师名与未格式化占位符", func(t *testing.T) {
		useChineseUI(t)
		a := newPopulatedTestApp(t)
		a.page = pageAnalysis
		a.analysisTeacherName = ""
		tt := newChineseTester(t, a.view, 1280, 800)

		for _, text := range tt.Texts() {
			if strings.Contains(text, "%d") {
				failGeometry(t, tt, "分析页占位符", fmt.Sprintf("出现未格式化占位符：%q", text))
			}
			if strings.Contains(text, `["`) {
				failGeometry(t, tt, "分析页教师名JSON", fmt.Sprintf("出现原始 JSON 数组文案：%q", text))
			}
		}
	})

	for _, lang := range []string{"zh-Hans", "en"} {
		t.Run("分析页最小窗口页签不裁切_"+lang, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return lang })
			t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

			a := newPopulatedTestApp(t)
			a.page = pageAnalysis
			tt := ui.NewTester(a.view, 1024, 640)
			tt.SetLocale(lang)
			settleTester(tt)
			if tt.FramePending() {
				t.Fatalf("分析页未稳定 (%s)", lang)
			}

			for _, tab := range []string{
				locales.AnalysisTabTeacherTrend(),
				locales.AnalysisTabClassTrend(),
				locales.AnalysisTabSubjectComparison(),
				locales.AnalysisTabStudentTrack(),
				locales.AnalysisTabAssessmentComparison(),
			} {
				node, ok := findNode(tt, ui.RoleRadio, tab)
				if !ok {
					failGeometry(t, tt, "分析页签_"+lang, fmt.Sprintf("找不到页签 %q", tab))
					continue
				}
				if rectRight(node.Bounds) > 1024-pagePadding+1 {
					failGeometry(t, tt, "分析页签_"+lang, fmt.Sprintf(
						"页签 %q 右边界 %.1f 超出内容区 %.1f", tab, rectRight(node.Bounds), float32(1024-pagePadding)))
				}
			}
		})

		t.Run("工作台结果顶栏最小窗口不裁切_"+lang, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return lang })
			t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

			a := newPopulatedTestApp(t)
			a.page = pageAssessments
			tt := ui.NewTester(a.view, 1024, 640)
			tt.SetLocale(lang)
			settleTester(tt)
			if tt.FramePending() {
				t.Fatalf("工作台未稳定 (%s)", lang)
			}

			for _, tab := range []string{
				locales.AssessmentViewBySubject(),
				locales.AssessmentViewByClass(),
				locales.AssessmentViewByTeacher(),
			} {
				node, ok := findNode(tt, ui.RoleRadio, tab)
				if !ok {
					failGeometry(t, tt, "工作台结果页签_"+lang, fmt.Sprintf("找不到结果页签 %q", tab))
					continue
				}
				if rectRight(node.Bounds) > 1024-pagePadding+1 {
					failGeometry(t, tt, "工作台结果页签_"+lang, fmt.Sprintf(
						"结果页签 %q 右边界 %.1f 超出内容区 %.1f", tab, rectRight(node.Bounds), float32(1024-pagePadding)))
				}
			}
			badgeNode, ok := findNode(tt, ui.RoleText, locales.AssessmentOfficialResults())
			if !ok {
				failGeometry(t, tt, "工作台正式徽标_"+lang, "找不到正式结果徽标")
			} else if badgeNode.Bounds.W+1 < measureTextWidthWithWeight(t, locales.AssessmentOfficialResults(), captionSize(ui.Theme{FontSize: 14}), 500) {
				failGeometry(t, tt, "工作台正式徽标_"+lang, fmt.Sprintf(
					"徽标宽度 %.1f 小于文字所需宽度", badgeNode.Bounds.W))
			} else if rectRight(badgeNode.Bounds) > 1024-pagePadding+1 {
				failGeometry(t, tt, "工作台正式徽标_"+lang, fmt.Sprintf(
					"徽标右边界 %.1f 超出内容区 %.1f", rectRight(badgeNode.Bounds), float32(1024-pagePadding)))
			}
		})
	}
}

func TestRuntimeTabBarBoundaryAndSelection(t *testing.T) {
	useChineseUI(t)
	f := newExamPageFixture(t)
	f.App.page = pageExams
	f.App.selectExam(f.ExamID)
	f.App.examCohortID = f.CohortID
	f.App.reloadExamData(t.Context())
	tt := newChineseTester(t, f.App.view, 1280, 800)
	tt.SetDark(false)
	settleTester(tt)

	if _, ok := findNode(tt, ui.RoleRadioGroup, locales.ExamViewLabel()); !ok {
		t.Fatalf("找不到页签栏")
	}
	var selected *ui.AccessInfo
	var segWidths []float32
	for _, info := range tt.AccessAll() {
		if info.Role != ui.RoleRadio || !info.Segment {
			continue
		}
		segWidths = append(segWidths, info.Bounds.W)
		if info.Checked {
			copy := info
			selected = &copy
		}
	}
	if len(segWidths) == 0 {
		t.Fatalf("页签栏没有页签")
	}
	if selected == nil {
		t.Fatalf("页签栏没有选中项")
	}
	// Sunbeam 规范要求分段选择器的所有选项严格等宽，选中滑块移动时尺寸不变。
	for i, w := range segWidths {
		if math.Abs(float64(w-segWidths[0])) > 1 {
			failGeometry(t, tt, "页签栏等宽约束", fmt.Sprintf("第 %d 个页签宽度 %.1f 与首项 %.1f 不一致", i, w, segWidths[0]))
		}
	}

	palette := lightPalette(t)
	img := tt.Image()
	selectedBg := img.RGBAAt(int(selected.Bounds.X)+6, int(selected.Bounds.Y+selected.Bounds.H/2))
	if !nearColor(selectedBg, palette.sb.FlatThumb, 12) {
		failGeometry(t, tt, "页签选中滑块材质", fmt.Sprintf(
			"选中页签 %q 底色 #%02X%02X%02X 与 Sunbeam FlatThumb #%02X%02X%02X 不符",
			selected.Name, selectedBg.R, selectedBg.G, selectedBg.B,
			palette.sb.FlatThumb.R, palette.sb.FlatThumb.G, palette.sb.FlatThumb.B))
	}
}

func lightPalette(t *testing.T) uiPalette {
	t.Helper()
	var palette uiPalette
	view := func(c *ui.Context) {
		palette = uiPaletteFor(c)
		ui.Text(c, "palette")
	}
	tt := ui.NewTester(view, 120, 80)
	tt.SetDark(false)
	tt.Frame()
	return palette
}

func nearColor(got color.RGBA, want ui.Color, tolerance int) bool {
	diff := func(a, b uint8) int {
		if a > b {
			return int(a - b)
		}
		return int(b - a)
	}
	return diff(got.R, want.R) <= tolerance &&
		diff(got.G, want.G) <= tolerance &&
		diff(got.B, want.B) <= tolerance
}
