package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

// assertNoOverflowInScrollable 断言所有可见节点横向绝不溢出窗口，且所有滚动/列表/表格容器自身边界完全位于窗口内。
func assertNoOverflowInScrollable(t *testing.T, tt *ui.Tester, width, height float32) {
	t.Helper()
	for _, info := range tt.AccessAll() {
		if info.Offscreen || info.Bounds.W <= 0 || info.Bounds.H <= 0 {
			continue
		}
		box := info.Bounds
		if box.X < -0.5 || box.X+box.W > width+0.5 {
			t.Errorf("%v %q 横向溢出窗口（宽 %.0f）：%+v", info.Role, info.Name, width, box)
		}
		if info.Role == ui.RoleScroll || info.Role == ui.RoleList || info.Role == ui.RoleTable {
			if box.Y < -0.5 || box.Y+box.H > height+0.5 {
				t.Errorf("滚动容器 %v %q 垂直溢出窗口（高 %.0f）：%+v", info.Role, info.Name, height, box)
			}
		}
	}
}

// TestBreakUI_ExamRosterDoesNotPanicAfterStudentTableKey 回归 #1：
// 学生页为 studentTable 绑定 Key 后，切到行数更多的考试报名名单表不得因复用 ListState 触发越界 panic。
func TestBreakUI_ExamRosterDoesNotPanicAfterStudentTableKey(t *testing.T) {
	useChineseUI(t)
	f := newExamPageFixture(t)
	a := f.App
	a.page = pageStudents
	a.students = a.students[:1]
	a.studentsTotal = 1

	tt := ui.NewTester(a.view, 1024, 640)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	a.page = pageExams
	a.selectExam(f.ExamID)
	a.examCohortID = f.CohortID
	a.reloadExamData(context.Background())
	// 造出比 a.students 更多行的报名名单
	for i := 0; i < 12; i++ {
		a.examRoster = append(a.examRoster, store.ExamRosterEntry{
			StudentID: int64(5000 + i),
			StudentNo: fmt.Sprintf("2026%04d", i),
			Name:      "Aleksandra Wiśniewska-Kowalczyk",
			ClassName: "高一实验创新一班",
			ExamNo:    fmt.Sprintf("E%06d", i),
			Track:     "physics",
			Status:    "active",
		})
	}
	a.examRosterTotal = len(a.examRoster)
	settleTester(tt)

	if _, ok := findNode(tt, ui.RoleTable, locales.ExamTabRoster()); !ok {
		t.Fatalf("切换后找不到报名名单表")
	}
}

// TestBreakUI_PaginationSwitchPageAndPageSize 回归 #2：
// 验证学生表超过每页条数时的翻页与切换每页条数交互，不再硬截断于 200 条。
func TestBreakUI_PaginationSwitchPageAndPageSize(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	stage := a.selectedStage()
	cohort := a.selectedCohort()
	if stage == nil || cohort == nil {
		t.Fatal("missing cohort/stage")
	}
	// 写入 65 名学生，配合 50 条/页测试多页翻页
	students := make([]store.ImportStudent, 0, 65)
	for i := 1; i <= 65; i++ {
		students = append(students, store.ImportStudent{
			StudentNo: fmt.Sprintf("2026%03d", i),
			Name:      fmt.Sprintf("学生%03d", i),
			ClassName: "1班",
			Track:     "physics",
			Status:    "active",
		})
	}
	if _, err := a.store.ApplyStageImport(context.Background(), cohort.ID, stage.ID, store.StageImportRequest{
		Kind:   "students",
		Data:   store.ImportData{Students: students},
		Report: store.ImportReport{FileHash: "pagination-test-hash", Path: "students.xlsx"},
	}); err != nil {
		t.Fatalf("import students: %v", err)
	}

	a.studentPage = 0
	a.studentPageSize = 50
	a.reloadStudents(context.Background())
	if a.studentsTotal != 65 || len(a.students) != 50 {
		t.Fatalf("page 0: total=%d len=%d", a.studentsTotal, len(a.students))
	}

	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	if !tt.HasText("Showing 1–50 of 65") {
		t.Fatalf("missing page 1 summary in %q", tt.Texts())
	}
	if err := tt.Click("Next"); err != nil {
		t.Fatalf("click Next: %v", err)
	}
	settleTester(tt)
	if a.studentPage != 1 || len(a.students) != 15 {
		t.Fatalf("after Next: page=%d len=%d", a.studentPage, len(a.students))
	}
	if !tt.HasText("Showing 51–65 of 65") {
		t.Fatalf("missing page 2 summary in %q", tt.Texts())
	}
}

// TestBreakUI_ExamScoresMinWindowFitsTableAndCollapsible 回归 #3：
// 1024×640 最小窗口下，成绩页签默认折叠统计分布，下方表格保持清晰可视高度不溢出窗口。
func TestBreakUI_ExamScoresMinWindowFitsTableAndCollapsible(t *testing.T) {
	useChineseUI(t)
	f := newExamPageFixture(t)
	a := f.App
	a.page = pageExams
	a.selectExam(f.ExamID)
	a.examCohortID = f.CohortID
	a.reloadExamData(context.Background())
	a.examTab = 1
	a.examScoresStatsOpen = false

	tt := newChineseTester(t, a.view, 1024, 640)
	table, ok := findNode(tt, ui.RoleTable, locales.ExamTabScores())
	if !ok {
		t.Fatal("1024×640 下找不到成绩表")
	}
	if table.Bounds.H < 160 {
		t.Fatalf("成绩表可用高度过小：%.1f", table.Bounds.H)
	}
	if rectBottom(table.Bounds) > 640 {
		t.Fatalf("成绩表底边 %.1f 超出 640 窗口", rectBottom(table.Bounds))
	}
}

// TestBreakUI_WorstCaseLongTextAndNarrowLayouts 回归 #4 #5 #6 #7 #8 #9 #10 #11 #15 #17 #19：
// 使用 CATALOG.md 真实极端数据在 1024×640（主窗口）与 640×440（设置窗口）下做双语边界断言。
func TestBreakUI_WorstCaseLongTextAndNarrowLayouts(t *testing.T) {
	longName := "Aleksandra Wiśniewska-Kowalczyk"
	longExam := "2026-2027学年第一学期高一重点实验班联合教学质量诊断性期中大联考（含走班赋分）"
	longURL := "https://dav.northwind-industries-holdings.example.com/workspaces/school/grade10/tally-sync/latest"
	longTeachers := []string{
		"Christopher Alexander Montgomery III",
		"Konstantin Oberhauser-Wettstein",
		"Seán O'Brien-Ó Súilleabháin",
		"María José de la Cruz y Fernández",
	}

	for _, lang := range []string{"zh-Hans", "en"} {
		t.Run("分析页极长数据与多卡对比_"+lang, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return lang })
			t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

			a := newPopulatedTestApp(t)
			a.page = pageAnalysis
			a.analysisSavedViews = []store.SavedView{{ID: 1, Name: "Q3 Board Deck — FINAL (revised) v12"}}
			a.analysisSelectedViewID = 1
			a.analysisTeacherResult = store.TeacherTrendResult{
				TeacherName:          longName,
				Subject:              "chinese",
				AveragePersonalScore: 95.4,
				LatestPersonalScore:  128.6,
				Points: []store.TeacherTrendPoint{{
					ExamName:      longExam,
					ExamDate:      "2026-11-20",
					PersonalScore: 128.6,
					Rows: []store.TeacherTrendRow{{
						LeadClassName:    "高一物理强基实验创新12班",
						Subject:          "chinese",
						ClassAverage:     112.4,
						BaselineRatio:    1.1234,
						ImprovementScore: 44.5,
						PassRateScore:    42.8,
						CompletionScore:  22.3,
						Total:            109.6,
						Rank:             1284,
						TierIndex:        1,
					}},
				}},
			}
			a.analysisSubjectResult = store.SubjectComparisonResult{
				ExamName:     longExam,
				Subject:      "chinese",
				GroupCount:   12,
				AverageScore: 98.0,
				Groups: []store.SubjectComparisonGroup{{
					Rank:            1,
					LeadClassName:   "高一实验12班",
					TeacherNames:    longTeachers,
					ClassAverage:    112.4,
					BaselineRatio:   1.0856,
					PassRateScore:   41.2,
					CompletionScore: 21.5,
					Total:           1284.5,
				}},
			}
			a.analysisComparisonResult = store.AssessmentComparisonResult{
				Items: make([]store.AssessmentComparisonItem, 6),
			}
			for i := range a.analysisComparisonResult.Items {
				a.analysisComparisonResult.Items[i] = store.AssessmentComparisonItem{
					ExamName:         fmt.Sprintf("%s #%d", longExam, i+1),
					ExamDate:         "2026-11-20",
					AcademicYear:     "2026-2027",
					Semester:         "First",
					ParticipantCount: 1284,
					GroupCount:       135,
					AverageScore:     96.5,
				}
			}

			for _, tab := range []int{0, 2, 4} {
				a.analysisTabIndex = tab
				tt := ui.NewTester(a.view, 1024, 640)
				tt.SetLocale(lang)
				settleTester(tt)
				assertNoOverflowInScrollable(t, tt, 1024, 640)
			}
		})

		t.Run("设置窗口最小尺寸640x440_"+lang, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return lang })
			t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

			a := newPopulatedTestApp(t)
			a.syncSettings.WebDAVURL = longURL
			a.syncSettings.Username = "bartholomew.fitzgerald@northwind-industries-holdings.example.com"
			a.syncError = "Request failed: " + longURL
			a.dataDir = "/Users/bartholomew.fitzgerald/Library/Application Support/Northwind-Industries/Tally-Workspace/production-data"

			for _, page := range []string{"subjects", "data", "sync"} {
				a.settingsPageID = page
				tt := ui.NewTester(a.settingsView, 640, 440)
				tt.SetLocale(lang)
				settleTester(tt)
				assertNoOverflowInScrollable(t, tt, 640, 440)
			}
		})
	}
}

// TestBreakUI_EmptyErrorAndSingleCountStates 回归 #12 #21 #22 #23：
// 覆盖启动失败恢复态、空库/筛选无结果态、以及英文数量为 1 的单数文案。
func TestBreakUI_EmptyErrorAndSingleCountStates(t *testing.T) {
	t.Run("全局加载失败提供重试与设置入口", func(t *testing.T) {
		a := newTestApp(t)
		a.loadError = "disk I/O error: database locked"
		tt := ui.NewTester(a.view, 1024, 640)
		tt.SetLocale("en")
		settleTester(tt)

		if !tt.HasText("Retry") || !tt.HasText("Open settings") {
			t.Fatalf("error state missing recovery actions: %q", tt.Texts())
		}
	})

	t.Run("筛选无结果与无数据区分并提供清空筛选", func(t *testing.T) {
		a := newPopulatedTestApp(t)
		a.page = pageStudents
		a.dataQuery = "不存在的极端查询词"
		a.reloadStudents(context.Background())

		tt := ui.NewTester(a.view, 1024, 640)
		tt.SetLocale("en")
		settleTester(tt)

		if !tt.HasText("No matching records") || !tt.HasText("Clear filters") {
			t.Fatalf("missing filter empty state in %q", tt.Texts())
		}
		if err := tt.Click("Clear filters"); err != nil {
			t.Fatalf("click Clear filters: %v", err)
		}
		settleTester(tt)
		if a.dataQuery != "" || len(a.students) == 0 {
			t.Fatalf("Clear filters did not restore students: query=%q len=%d", a.dataQuery, len(a.students))
		}
	})

	t.Run("英文单数计数语法正确", func(t *testing.T) {
		i18n.SetLocaleFunc(func() string { return "en" })
		t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

		if got := pluralCount(1, locales.DataStudentCountOne(), locales.DataStudentCount()); got != "1 student" {
			t.Fatalf("got %q, want %q", got, "1 student")
		}
		if got := pluralCount(1284, locales.DataStudentCountOne(), locales.DataStudentCount()); got != "1,284 students" {
			t.Fatalf("got %q, want %q", got, "1,284 students")
		}
	})
}

// TestBreakUI_NameAndDescLengthValidationAndFilenameSanitization 回归 #26：
// 校验 80/500 字符上限与导出文件名清洗截断。
func TestBreakUI_NameAndDescLengthValidationAndFilenameSanitization(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "en" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })

	a := newPopulatedTestApp(t)
	a.openNewExam()
	a.newExamName = strings.Repeat("高", 81)
	a.createExam()
	if a.examError == "" || !strings.Contains(a.examError, "80") {
		t.Fatalf("expected 80-char error for exam name, got %q", a.examError)
	}

	a.planConfig.Name = strings.Repeat("P", 81)
	a.savePlan(false)
	if a.planError == "" || !strings.Contains(a.planError, "80") {
		t.Fatalf("expected 80-char error for plan name, got %q", a.planError)
	}

	a.planConfig.Name = "Valid Plan"
	a.planConfig.Description = strings.Repeat("D", 501)
	a.savePlan(false)
	if a.planError == "" || !strings.Contains(a.planError, "500") {
		t.Fatalf("expected 500-char error for plan description, got %q", a.planError)
	}

	a.zeroReason = strings.Repeat("R", 501)
	a.startFillZero()
	if a.zeroError == "" || !strings.Contains(a.zeroError, "500") {
		t.Fatalf("expected 500-char error for zero reason, got %q", a.zeroError)
	}

	clean := sanitizeExportFilename("2026/07:09*高三?最终版<v12>/"+strings.Repeat("甲", 100)+".xlsx", "Tally.xlsx")
	if strings.ContainsAny(clean, `/\:*?"<>|`) {
		t.Fatalf("dirty chars left in filename: %q", clean)
	}
	if !strings.HasSuffix(clean, ".xlsx") {
		t.Fatalf("lost extension: %q", clean)
	}
	if len([]rune(strings.TrimSuffix(clean, ".xlsx"))) > maxExportFileBaseRunes {
		t.Fatalf("filename not truncated: %q", clean)
	}
}

// TestBreakUI_HugeVirtualListPerformance 回归 #24：
// 验证导入向导校验问题与数据质量页面对 1,000+ 条问题使用虚拟化 ui.List，渲染保持流畅稳定。
func TestBreakUI_HugeVirtualListPerformance(t *testing.T) {
	useChineseUI(t)
	a := newPopulatedTestApp(t)
	issues := make([]store.ExamQualityIssue, 0, 1200)
	for i := 0; i < 1200; i++ {
		issues = append(issues, store.ExamQualityIssue{
			Severity:  "warning",
			Code:      "missing_scores",
			StudentNo: fmt.Sprintf("2026%04d", i),
			Name:      "Aleksandra Wiśniewska-Kowalczyk",
			Row:       i + 2,
		})
	}
	a.qualityExams = []qualityExamOverview{{
		ExamID:     1,
		ExamLabel:  "2026春季高一全科综合测评",
		CohortID:   1,
		CohortName: "2026级",
		GradeLevel: "g1",
		Issues:     issues,
	}}
	a.qualityLoaded = true
	a.page = pageQuality

	start := time.Now()
	tt := newChineseTester(t, a.view, 1024, 640)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("1200 quality rows rendered too slowly: %v", elapsed)
	}
	assertNoOverflowInScrollable(t, tt, 1024, 640)
}
