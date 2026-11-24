package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo/ui"

	"tally/internal/school"
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
		return locales.NavFaculty()
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
		ctx := context.Background()
		switch page {
		case pageAnalysis:
			a.reloadAnalysis(ctx)
		case pageQuality:
			a.reloadQuality(ctx)
		case pageOverview:
			a.reloadWorkspaceSummary(ctx)
		}
	})
}

// openDataImport 记录当前来源上下文并跳转到独立「数据导入」页，按需预选导入类型与考试。
func (a *app) openDataImport(kind string, examID, cohortID int64) {
	if examID == 0 && kind != "" && !a.stageEditable() {
		a.dataError = locales.StageReadOnly()
		return
	}
	a.guardNavigation(func() {
		origin := a.normalizedPage()
		if origin != pageImports {
			a.importReturn = &importReturnContext{
				Page:        origin,
				BaseDataTab: a.baseDataTab,
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
	a.reloadSelectedCohort(ctx)
	if ret == nil {
		return
	}
	a.page = ret.Page
	a.baseDataTab = ret.BaseDataTab
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
	a.newExamModal(c, p)
	a.statGroupModal(c, p)
	a.statGroupDeleteDialog(c, p)
	a.zeroModal(c, p)
	a.resultDetailModal(c, p)
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

// sidebar 是 Sunbeam Flat 材质的暖纸左侧栏，顶部为工作区切换（空态直接显示新建按钮），
// 中部为精简后的工作区核心菜单与工具分区，底部固定帮助与设置。
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
		a.workspaceSwitcher(c, p)
		ui.Scroll(c.Key("nav")).Grow(1).Shrink(1).MinHeight(0).
			Margin(0, -t.Space(2)).Padding(0, t.Space(2)).
			Gap(0).Children(func() {
			a.navSection(c, p, locales.NavGroupWorkspace())
			a.navItem(c, p, pageOverview, iconHome, locales.NavOverview(), 0)
			a.navItem(c, p, pageImports, iconImport, locales.NavDataImport(), 0)
			a.navItem(c, p, pageBaseData, iconCohort, locales.NavBaseData(), 0)
			a.navItem(c, p, pageAssessments, iconBatches, locales.NavExamHub(), 0)
			a.navItem(c, p, pageQuality, iconQuality, locales.NavQuality(), a.workspaceSummary.BlockingIssues)
			a.navItem(c, p, pageAnalysis, iconAnalysis, locales.NavAnalysis(), 0)

			ui.Box(c).Height(t.Space(2))
			a.navSection(c, p, locales.NavGroupTools())
			a.navItem(c, p, pagePlans, iconPlans, locales.NavPlans(), 0)
		})
		rule(c)
		ui.Column(c).Gap(0).Padding(t.Space(0.5), 0, 0, 0).Children(func() {
			a.sidebarAction(c, p, "help", iconHelp, locales.NavHelp(), func() {
				a.navigateTo(pageHelp)
			})
			a.sidebarAction(c, p, "settings", iconSettings, locales.NavSettings(), func() {
				a.openSettings()
			})
		})
	})
}

// workspaceSwitcher 是侧栏顶部的工作区（年级）选择器：
// 无工作区时直接展示「新建工作区」按钮；有工作区时显示当前年级与阶段，下拉列表采用单对勾结构并在底部提供「新建工作区」。
func (a *app) workspaceSwitcher(c *ui.Context, p uiPalette) {
	t := p.theme
	if len(a.cohorts) == 0 {
		btn := ui.ButtonBase(c.Key("workspace-create")).FillWidth().Height(40).
			PaddingX(12).Gap(8).Radius(sunbeam.RadiusXL).
			Background(p.panel).Border(1, p.borderStrong).
			AlignItems(ui.Center).Justify(ui.Center).
			Cursor(ui.CursorPointer).FocusRing(false).
			Label(locales.WorkspaceCreate()).TextColor(p.ink)
		switch {
		case btn.Pressed():
			btn.Background(p.sb.FlatFacePressed)
		case btn.Hovered():
			btn.Background(p.hover)
		}
		btn.Children(func() {
			ui.Icon(c, iconPlus).Size(16, 16).TextColor(p.ink).Shrink(0)
			ui.Text(c, locales.WorkspaceCreate()).FontSize(bodySize(t)).FontWeight(500).SingleLine()
		})
		if btn.Clicked() {
			a.openNewCohort()
		}
		return
	}

	selectedCohort := a.selectedCohort()
	displayStage := a.displayStage()
	chosen := ""
	if selectedCohort != nil {
		chosen = "cohort-" + strconv.FormatInt(selectedCohort.ID, 10)
	}
	sel := ui.SelectBase(c.Key("workspace"), &chosen)
	trigger := sel.Trigger.FillWidth().MinHeight(48).Padding(6, 10).Gap(8).
		Radius(sunbeam.RadiusXL).AlignItems(ui.Center).
		Background(p.panel).Border(1, p.border).
		Cursor(ui.CursorPointer).FocusRing(false).Label(locales.WorkspaceSwitchLabel())
	switch {
	case sel.Open():
		trigger.Border(1, p.sb.Ring)
	case trigger.Hovered():
		trigger.Background(p.panelHover).Border(1, p.borderStrong)
	}
	trigger.Children(func() {
		name := locales.WorkspaceEmpty()
		sub := ""
		if selectedCohort != nil {
			name = a.cohortName(selectedCohort.EnrollmentYear)
			stageText := a.stageName(displayStage.GradeLevel)
			yearText := school.AcademicYearLabel(displayStage.AcademicYearStart)
			if stageText != "" && yearText != "" {
				sub = fmt.Sprintf("%s · %s", stageText, yearText)
			}
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, name).FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink).
				SingleLine().Tooltip(name)
			if sub != "" {
				ui.Text(c, sub).FontSize(captionSize(t)).TextColor(p.muted).
					SingleLine().Tooltip(sub)
			}
		})
		ui.Icon(c, iconUpDown).Size(14, 14).TextColor(p.muted).Shrink(0)
	})

	sel.Popup(func(popup ui.Element) {
		popup.Key("workspace-switcher-popup")
		sunbeam.PanelSurface(c, popup)
		popup.Margin(6, 0, 0, 0).Padding(6).Gap(4).
			MinWidth(sidebarWidth - 16).MaxWidth(sidebarWidth + 60).
			Radius(sunbeam.RadiusXL).Transition(popupTransition).AttachedOrigin()
		sunbeam.Layer(sunbeam.Raised, func() {
			ui.Scroll(c).MaxHeight(360).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					ui.Text(c, locales.WorkspaceLabel()).FontSize(captionSize(t)).FontWeight(500).
						TextColor(p.muted).Padding(4, 8, 2, 8).SingleLine()
					for _, cohort := range a.cohorts {
						cohort := cohort
						name := a.cohortName(cohort.EnrollmentYear)
						gradeHint := a.stageIndexName(school.StageIndex(cohort.EnrollmentYear, a.academicYearStart))
						selected := selectedCohort != nil && cohort.ID == selectedCohort.ID
						item := sel.Item("cohort-" + strconv.FormatInt(cohort.ID, 10)).Label(name).
							Height(36).PaddingX(10).Gap(8).
							Radius(sunbeam.RadiusLG).AlignItems(ui.Center).
							TextColor(p.ink)
						if item.Highlighted() {
							item.Background(p.hover)
						}
						item.Children(func() {
							ui.Text(c, name).FontSize(bodySize(t)).FontWeight(500).TextColor(p.ink).
								SingleLine().Grow(1).Shrink(1).MinWidth(0)
							ui.Text(c, gradeHint).FontSize(captionSize(t)).TextColor(p.muted).
								SingleLine().Shrink(0)
							if selected {
								ui.Icon(c, iconCheck).Size(14, 14).TextColor(p.ink).Shrink(0)
							}
						})
					}
					if len(a.cohortStages) > 1 {
						ui.Box(c).Height(2)
						rule(c)
						ui.Text(c, locales.StageSelectLabel()).FontSize(captionSize(t)).FontWeight(500).
							TextColor(p.muted).Padding(4, 8, 2, 8).SingleLine()
						labels, ids := a.stageOptions()
						for index, label := range labels {
							label, id := label, ids[index]
							selected := id == a.selectedStageID
							item := sel.Item("stage-" + strconv.FormatInt(id, 10)).Label(label).
								Height(34).PaddingX(10).Gap(8).
								Radius(sunbeam.RadiusLG).AlignItems(ui.Center).
								TextColor(p.inkSecondary)
							if item.Highlighted() {
								item.Background(p.hover)
							}
							item.Children(func() {
								ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.inkSecondary).
									Grow(1).Shrink(1).MinWidth(0).SingleLine()
								if selected {
									ui.Icon(c, iconCheck).Size(14, 14).TextColor(p.ink).Shrink(0)
								}
							})
						}
					}
					ui.Box(c).Height(2)
					rule(c)
					createItem := sel.Item("workspace-create-new").Label(locales.WorkspaceCreate()).
						Height(36).PaddingX(10).Gap(8).
						Radius(sunbeam.RadiusLG).AlignItems(ui.Center).
						TextColor(p.ink)
					if createItem.Highlighted() {
						createItem.Background(p.hover)
					}
					createItem.Children(func() {
						ui.Icon(c, iconPlus).Size(14, 14).TextColor(p.ink).Shrink(0)
						ui.Text(c, locales.WorkspaceCreate()).FontSize(bodySize(t)).FontWeight(500).
							TextColor(p.ink).SingleLine()
					})
				})
			})
		})
	})

	if !trigger.Changed() {
		return
	}
	switch {
	case chosen == "workspace-create-new":
		a.openNewCohort()
	case strings.HasPrefix(chosen, "cohort-"):
		targetID, err := strconv.ParseInt(strings.TrimPrefix(chosen, "cohort-"), 10, 64)
		if err != nil {
			return
		}
		if selectedCohort != nil && selectedCohort.ID == targetID {
			return
		}
		for _, cohort := range a.cohorts {
			if cohort.ID != targetID {
				continue
			}
			cohort := cohort
			a.guardNavigation(func() {
				a.switchWorkspaceCohort(cohort.ID)
				a.notifyMain(fmt.Sprintf(locales.WorkspaceSwitchedToast(), a.cohortName(cohort.EnrollmentYear)))
			})
			return
		}
	case strings.HasPrefix(chosen, "stage-"):
		stageID := strings.TrimPrefix(chosen, "stage-")
		a.guardNavigation(func() {
			a.selectStageByID(stageID)
		})
	}
}

// selectStageByID 按标识切换当前工作区的学年阶段。
func (a *app) selectStageByID(id string) {
	for _, stage := range a.cohortStages {
		if strconv.FormatInt(stage.ID, 10) != id {
			continue
		}
		if stage.ID == a.selectedStageID {
			return
		}
		a.selectedStageID = stage.ID
		a.reloadSelectedCohort(context.Background())
		return
	}
}

// navActiveFor 判断指定侧栏顶层入口在当前路由下是否处于激活状态。
func (a *app) navActiveFor(item pageID) bool {
	if a.importWizardActive() {
		return item == pageImports
	}
	current := a.normalizedPage()
	switch item {
	case pageBaseData:
		switch current {
		case pageBaseData, pageCohorts, pageStudents, pageFaculty, pageGroups, pageBaseline:
			return true
		}
		return false
	case pageAssessments:
		switch current {
		case pageAssessments, pageExams, pageResults:
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

// baseDataPage 聚合「基础数据」的 5 个子页签（班级与阶段 / 学生 / 教师与任课 / 教学班 / 入口基数），
// 替代原先分散在左侧栏的 5 个独立菜单。
func (a *app) baseDataPage(c *ui.Context, p uiPalette) {
	t := p.theme
	labels := []string{
		locales.BaseDataTabClasses(),
		locales.BaseDataTabStudents(),
		locales.BaseDataTabFaculty(),
		locales.BaseDataTabGroups(),
		locales.BaseDataTabBaselines(),
	}
	if a.baseDataTab < 0 || a.baseDataTab >= len(labels) {
		a.baseDataTab = 0
	}
	ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
		segmentedField(c, &a.baseDataTab, labels, locales.NavBaseData())
		ui.Column(c).Grow(1).Shrink(1).MinHeight(0).FillWidth().Children(func() {
			switch a.baseDataTab {
			case 1:
				a.studentsPage(c, p)
			case 2:
				a.facultyPage(c, p)
			case 3:
				a.groupsPage(c, p)
			case 4:
				a.baselinePage(c, p)
			default:
				a.cohortPage(c, p)
			}
		})
	})
}

func (a *app) pageContent(c *ui.Context, p uiPalette) {
	page := a.normalizedPage()
	if page != a.lastPage {
		if page == pageQuality {
			a.reloadQuality(context.Background())
		} else if page == pageOverview {
			a.reloadWorkspaceSummary(context.Background())
		}
		a.lastPage = page
	}
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
	case pageBaseData:
		a.baseDataPage(c, p)
	case pageCohorts:
		a.cohortPage(c, p)
	case pageStudents:
		a.studentsPage(c, p)
	case pageFaculty:
		a.facultyPage(c, p)
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
