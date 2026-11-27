package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo/ui"

	"tally/locales"
)

type pageID string

const (
	pageOverview    pageID = "overview"
	pageBaseData    pageID = "base_data"
	pageCohorts     pageID = "cohorts"
	pageStudents    pageID = "students"
	pageFaculty     pageID = "faculty"
	pageGroups      pageID = "groups"
	pageBaseline    pageID = "baseline"
	pageExams       pageID = "exams"
	pageQuality     pageID = "quality"
	pageImports     pageID = "imports"
	pageAssessments pageID = "assessments"
	pageResults     pageID = "results"
	pageAnalysis    pageID = "analysis"
	pagePlans       pageID = "plans"
	pageHelp        pageID = "help"
)

// importReturnContext 记录从某页面跳转到独立「数据导入」页时的来源上下文，用于导入后一键返回。
type importReturnContext struct {
	Page        pageID
	BaseDataTab int
	ExamID      int64
	ExamTab     int
	Label       string
}

// sidebarWidth 是左侧栏的固定宽度。
const sidebarWidth = 232

// topStripHeight 是窗口顶部留给拖动与标题的高度。
const topStripHeight = 40

// pagePadding 是页面内容距窗口两侧与底部的距离。
const pagePadding = 28

func (a *app) normalizedPage() pageID {
	if a.page == "" {
		return pageOverview
	}
	return a.page
}

// pageDisplayLabel 返回页面用于返回条与提示语的本地化名称。
func (a *app) pageDisplayLabel(page pageID) string {
	switch page {
	case pageBaseData:
		return locales.NavBaseData()
	case pageCohorts:
		return locales.NavCohorts()
	case pageStudents:
		return locales.NavStudents()
	case pageFaculty:
		return locales.NavTeachers()
	case pageGroups:
		return locales.NavGroups()
	case pageBaseline:
		return locales.NavBaseline()
	case pageExams:
		return locales.NavExams()
	case pageQuality:
		return locales.NavQuality()
	case pageImports:
		return locales.NavDataImport()
	case pageAssessments:
		return locales.NavExamHub()
	case pageResults:
		return locales.NavResults()
	case pageAnalysis:
		return locales.NavAnalysis()
	case pagePlans:
		return locales.NavPlans()
	case pageHelp:
		return locales.NavHelp()
	default:
		return locales.NavOverview()
	}
}

// hasUnsavedChanges 汇总当前页面的未保存草稿状态，供导航切换与工作区切换统一守卫。
func (a *app) hasUnsavedChanges() bool {
	return a.planConfigDirty()
}

// guardNavigation 在离开含未保存草稿的页面前弹出三态保护确认；无脏数据时直接执行。
func (a *app) guardNavigation(action func()) {
	if action == nil {
		return
	}
	if !a.hasUnsavedChanges() {
		action()
		return
	}
	a.pendingNavAction = action
	a.unsavedGuardOpen = true
}

// navigateTo 统一执行侧栏及页内主页面跳转，并经过未保存保护检查。
func (a *app) navigateTo(page pageID) {
	a.guardNavigation(func() {
		if page != pageImports {
			a.importWizard = nil
			a.importReturn = nil
		}
		a.page = page
		a.lastPage = page
		ctx := context.Background()
		switch page {
		case pageAnalysis:
			a.reloadAnalysis(ctx)
		case pageQuality:
			a.reloadQuality(ctx)
		case pageOverview:
			a.reloadSchoolOverview(ctx)
		case pageCohorts:
			a.reloadCohorts(ctx)
		case pageFaculty:
			a.reloadTeacherDirectory(ctx)
		case pageExams:
			a.reloadExams(ctx)
		case pageAssessments:
			a.reloadAssessments(ctx)
		}
	})
}

// openDataImport 记录当前来源上下文并跳转到独立「数据导入」页，按需预选导入类型与考试。
func (a *app) openDataImport(kind string, examID, cohortID int64) {
	if examID == 0 && kind != "" && a.selectedCohort() != nil && !a.stageEditable() {
		a.dataError = locales.StageReadOnly()
		return
	}
	a.guardNavigation(func() {
		origin := a.normalizedPage()
		if origin != pageImports {
			a.importReturn = &importReturnContext{
				Page:        origin,
				BaseDataTab: a.cohortDetailTab,
				ExamID:      a.selectedExamID,
				ExamTab:     a.examTab,
				Label:       a.pageDisplayLabel(origin),
			}
		}
		a.page = pageImports
		a.importCenterTab = 0
		if examID != 0 {
			a.importCenterScope = 1
			a.importCenterExamID = examID
			a.importWizard = nil
			a.openExamImportWizard(kind, examID, cohortID)
			return
		}
		a.importCenterScope = 0
		if cohortID != 0 {
			a.selectedCohortID = strconv.FormatInt(cohortID, 10)
			a.reloadSelectedCohort(context.Background())
		} else if a.selectedCohort() == nil && len(a.cohorts) > 0 {
			for _, co := range a.cohorts {
				if co.CurrentPhase != "graduated" {
					a.selectedCohortID = strconv.FormatInt(co.ID, 10)
					break
				}
			}
			if a.selectedCohortID == "" {
				a.selectedCohortID = strconv.FormatInt(a.cohorts[0].ID, 10)
			}
			a.reloadSelectedCohort(context.Background())
		}
		if kind != "" {
			a.openImportWizard(kind)
		}
	})
}

// returnFromImport 关闭导入向导并恢复进入「数据导入」前的页面与上下文，同时刷新工作区数据。
func (a *app) returnFromImport() {
	ret := a.importReturn
	a.importWizard = nil
	a.importReturn = nil
	ctx := context.Background()
	a.reloadSchoolOverview(ctx)
	if a.selectedCohort() != nil {
		a.reloadSelectedCohort(ctx)
	}
	if ret == nil {
		return
	}
	a.page = ret.Page
	a.cohortDetailTab = ret.BaseDataTab
	if ret.ExamID != 0 {
		a.selectedExamID = ret.ExamID
		a.examTab = ret.ExamTab
		a.reloadExamData(ctx)
	}
	if a.page == pageQuality {
		a.reloadQuality(ctx)
	}
}

// topStrip 返回窗口顶部需要避开的竖直空间。
func topStrip(c *ui.Context) float32 {
	return max(c.TitleBar().Height, topStripHeight)
}

// pageInsets 返回页面内容需要的上、右、下、左内边距。
func pageInsets(c *ui.Context) (top, right, bottom, left float32) {
	return topStrip(c) + 12, pagePadding, pagePadding, pagePadding
}

func (a *app) view(c *ui.Context) {
	p := uiPaletteFor(c)
	page := ui.Row(c).Fill().AlignItems(ui.Stretch).Background(p.canvas)
	page.Children(func() {
		a.sidebar(c, p)
		ui.Box(c).Width(1).FillHeight().Background(p.border).Shrink(0)
		top, right, bottom, left := pageInsets(c)
		ui.Column(c).Grow(1).Shrink(1).MinHeight(0).MinWidth(0).FillHeight().Background(p.canvas).
			Padding(top, right, bottom, left).Role(ui.RoleMain).Children(func() {
			if a.loadError != "" {
				errorState(c, p, locales.PlanLoadError(), a.loadError, func() {
					a.loadError = ""
					a.load()
				}, func() {
					a.openSettings()
				})
				return
			}
			a.pageContent(c, p)
		})
		ui.Box(c.Key("title-strip")).Absolute().Top(0).Right(0).Left(sidebarWidth).
			Height(topStrip(c)).DragWindow()
	})
	a.newCohortModal(c, p)
	a.stageReviewModal(c, p)
	a.promoteCohortModal(c, p)
	a.graduateCohortModal(c, p)
	a.newClassModal(c, p)
	a.headTeacherModal(c, p)
	a.classTrendModal(c, p)
	a.newWalkingGroupModal(c, p)
	a.walkingStudentsModal(c, p)
	a.assignTeacherModal(c, p)
	a.newStudentModal(c, p)
	a.studentEventModal(c, p)
	a.editTeacherModal(c, p)
	a.baselineEditModal(c, p)
	a.newTeacherModal(c, p)
	a.newExamModal(c, p)
	a.statGroupModal(c, p)
	a.statGroupDeleteDialog(c, p)
	a.zeroModal(c, p)
	a.resultDetailModal(c, p)
	a.runCompareDrawer(c, p)
	a.qualityFixDrawer(c, p)
	a.analysisSaveViewModal(c, p)
	a.analysisDeleteViewDialog(c, p)
	a.planSaveDialog(c, p)
	a.importUndoConfirmModal(c, p)
	a.renderUnsavedGuard(c, p)
	if a.mainToast != "" {
		c.Toast(a.mainToast)
		a.mainToast = ""
	}
	sunbeam.Toasts(c, locales.Close())
}

func (a *app) renderUnsavedGuard(c *ui.Context, p uiPalette) {
	if !a.unsavedGuardOpen && a.pendingNavAction == nil {
		return
	}
	act := unsavedGuardModal(c, p, "unsaved-nav", &a.unsavedGuardOpen, "", "")
	switch act {
	case unsavedGuardStay:
		a.pendingNavAction = nil
	case unsavedGuardDiscard:
		a.loadPlanConfig()
		pending := a.pendingNavAction
		a.pendingNavAction = nil
		if pending != nil {
			pending()
		}
	case unsavedGuardSave:
		a.savePlan(true)
		if a.planError == "" {
			if a.planMessage != "" {
				a.notifyMain(a.planMessage)
				a.planMessage = ""
			}
			pending := a.pendingNavAction
			a.pendingNavAction = nil
			if pending != nil {
				pending()
			}
		} else {
			a.pendingNavAction = nil
		}
	default:
		if !a.unsavedGuardOpen {
			a.pendingNavAction = nil
		}
	}
}

// sidebar 是 Sunbeam Flat 材质的暖纸左侧栏，采用全校统一导航（无单级工作区锁定）：
// 主栏 5 项：总览、年级档案、教师档案、考试与考核、考核方案；底部固定 3 项：导入记录、设置与同步、帮助。
func (a *app) sidebar(c *ui.Context, p uiPalette) {
	t := p.theme
	top := t.Space(3)
	if c.TitleBar().Left > 0 {
		top = topStrip(c) + t.Space(1)
	}
	side := ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().
		Background(p.panelMuted).
		Padding(top, t.Space(3), t.Space(2), t.Space(3)).
		Gap(t.Space(2)).DragWindow()
	side.Children(func() {
		ui.Row(c).Padding(t.Space(1), 10, t.Space(2), 10).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, iconMark).Size(20, 20).TextColor(p.ink).Shrink(0)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(0).Children(func() {
				ui.Text(c, locales.AppTitle()).FontSize(sectionSize(t)).FontWeight(600).
					TextColor(p.ink).SingleLine()
				ui.Text(c, locales.AppTagline()).FontSize(captionSize(t)).
					TextColor(p.muted).SingleLine()
			})
		})
		ui.Scroll(c.Key("nav")).Grow(1).Shrink(1).MinHeight(0).
			Margin(0, -t.Space(2)).Padding(0, t.Space(2)).
			Gap(0).Children(func() {
			a.navSection(c, p, locales.NavGroupMain())
			a.navItem(c, p, pageOverview, iconHome, locales.NavOverview(), 0)
			a.navItem(c, p, pageCohorts, iconCohort, locales.NavCohorts(), 0)
			a.navItem(c, p, pageFaculty, iconFaculty, locales.NavTeachers(), 0)
			a.navItem(c, p, pageAssessments, iconBatches, locales.NavExamHub(), a.schoolBlockingAlertCount())
			a.navItem(c, p, pagePlans, iconPlans, locales.NavPlans(), 0)
		})
		rule(c)
		ui.Column(c).Gap(0).Padding(t.Space(0.5), 0, 0, 0).Children(func() {
			a.navItem(c, p, pageImports, iconImports, locales.NavImports(), 0)
			a.sidebarAction(c, p, "settings", iconSettings, locales.NavSettingsSync(), func() {
				a.openSettings()
			})
			a.navItem(c, p, pageHelp, iconHelp, locales.NavHelp(), 0)
		})
	})
}

// schoolBlockingAlertCount 汇总全校在读年级最新考试的阻断问题数量，供侧栏「考试与考核」角标展示。
func (a *app) schoolBlockingAlertCount() int {
	total := 0
	for _, card := range a.schoolOverview.ActiveCohorts {
		total += card.BlockingIssues
	}
	if total == 0 {
		total = a.workspaceSummary.BlockingIssues
	}
	return total
}

// navActiveFor 判断指定侧栏顶层入口在当前路由下是否处于激活状态。
func (a *app) navActiveFor(item pageID) bool {
	if a.importWizardActive() {
		return item == pageImports
	}
	current := a.normalizedPage()
	switch item {
	case pageCohorts:
		switch current {
		case pageCohorts, pageBaseData, pageStudents, pageGroups, pageBaseline:
			return true
		}
		return false
	case pageAssessments:
		switch current {
		case pageAssessments, pageExams, pageResults, pageQuality, pageAnalysis:
			return true
		}
		return false
	default:
		return current == item
	}
}

// navSection 是导航里的分组标题。
func (a *app) navSection(c *ui.Context, p uiPalette, title string) {
	ui.Text(c, title).FontSize(captionSize(p.theme)).FontWeight(500).TextColor(p.muted).
		LineHeight(1.2).
		Padding(2, 10, 4, 10).
		SingleLine()
}

// navItem 是导航里的一个页面入口。遵循 Sunbeam 原则：选中项用暖灰底与深墨图标文字，
// 可选右侧异常计数角标采用安静圆点/等宽数字，不将明黄分散到侧栏。
func (a *app) navItem(c *ui.Context, p uiPalette, page pageID, icon *ui.SVG, label string, alertCount int) {
	selected := a.navActiveFor(page)
	b := ui.ButtonBase(c.Key(string(page))).Label(label).Height(36).
		PaddingX(10).Gap(10).
		Radius(sunbeam.RadiusLG).AlignItems(ui.Center).Justify(ui.Start).
		Cursor(ui.CursorPointer).FocusRing(false)
	background := ui.Transparent
	switch {
	case selected:
		background = p.fill
	case b.Pressed():
		background = p.sb.FlatFacePressed
	case b.Hovered():
		background = p.hover
	}
	b.Background(background)
	color := p.inkSecondary
	iconColor := p.muted
	weight := 400
	if selected {
		color = p.ink
		iconColor = p.ink
		weight = 500
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(iconColor).Shrink(0)
		ui.Text(c, label).FontSize(bodySize(p.theme)).FontWeight(weight).TextColor(color).
			SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if alertCount > 0 {
			ui.Row(c).Height(20).PaddingX(7).Radius(10).Gap(4).
				AlignItems(ui.Center).Background(p.dangerSoft).Shrink(0).Children(func() {
				ui.Box(c).Size(5, 5).Radius(3).Background(p.danger).Shrink(0)
				ui.Text(c, formatInt(alertCount)).FontSize(12).FontWeight(600).
					FontFeatures("tnum").TextColor(p.danger).SingleLine()
			})
		}
	})
	if b.Clicked() {
		if page == pageCohorts {
			a.cohortDetailOpen = false
			a.studentProfileOpen = false
			a.studentProfileStack = nil
			a.selectedStudentID = 0
		}
		if page == pageFaculty {
			a.teacherProfileOpen = false
			a.teacherProfileReturnCohort = 0
			a.selectedTeacherID = 0
		}
		a.navigateTo(page)
	}
}

func (a *app) sidebarAction(c *ui.Context, p uiPalette, key string, icon *ui.SVG, label string, action func()) {
	t := p.theme
	button := ui.ButtonBase(c.Key(key)).Label(label).Height(36).
		Padding(0, 10).Gap(10).Radius(sunbeam.RadiusLG).
		AlignItems(ui.Center).Justify(ui.Start).
		Cursor(ui.CursorPointer).FocusRing(false)
	switch {
	case button.Pressed():
		button.Background(p.sb.FlatFacePressed)
	case button.Hovered():
		button.Background(p.hover)
	}
	button.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(p.muted)
		ui.Text(c, label).FontSize(bodySize(t)).TextColor(p.inkSecondary)
	})
	if button.Clicked() {
		action()
	}
}

// examHubSubnav 在「考试与考核」一级导航下的子视图（考试列表 / 量化考核 / 数据质量 / 多维分析）顶部提供统一切换条。
func (a *app) examHubSubnav(c *ui.Context, p uiPalette, active int) {
	labels := []string{
		locales.ExamHubTabExams(),
		locales.ExamHubTabWorkbench(),
		locales.ExamHubTabQuality(),
		locales.ExamHubTabAnalysis(),
	}
	chosen := active
	next := chosen
	ui.Row(c).FillWidth().Children(func() {
		segmentedField(c, &next, labels, locales.NavExamHub())
	})
	if next != chosen {
		switch next {
		case 0:
			a.navigateTo(pageExams)
		case 1:
			a.navigateTo(pageAssessments)
		case 2:
			a.navigateTo(pageQuality)
		case 3:
			a.navigateTo(pageAnalysis)
		}
	}
}

func (a *app) pageContent(c *ui.Context, p uiPalette) {
	page := a.normalizedPage()
	if a.importWizardActive() {
		if a.importReturn != nil {
			t := p.theme
			ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
				ctxText := fmt.Sprintf(locales.ImportCenterReturnContext(), importKindName(a.importWizard.Kind))
				retLabel := fmt.Sprintf(locales.ImportCenterReturnButton(), a.importReturn.Label)
				if returnContextBanner(c, p, ctxText, retLabel) {
					a.returnFromImport()
					return
				}
				ui.Column(c).Grow(1).Shrink(1).MinHeight(0).FillWidth().Children(func() {
					a.importWizardPage(c, p)
				})
			})
			return
		}
		a.importWizardPage(c, p)
		return
	}
	switch page {
	case pageBaseData, pageCohorts:
		a.cohortsHubPage(c, p)
	case pageStudents:
		a.studentsPage(c, p)
	case pageFaculty:
		a.teachersDirectoryPage(c, p)
	case pageGroups:
		a.groupsPage(c, p)
	case pageBaseline:
		a.baselinePage(c, p)
	case pageExams:
		a.examsPage(c, p)
	case pageQuality:
		a.qualityPage(c, p)
	case pageImports:
		a.importsPage(c, p)
	case pageAssessments:
		a.assessmentPage(c, p)
	case pageResults:
		a.resultsPage(c, p)
	case pageAnalysis:
		a.analysisPage(c, p)
	case pagePlans:
		a.planPage(c, p)
	case pageHelp:
		a.helpPage(c, p)
	default:
		a.overviewPage(c, p)
	}
}
