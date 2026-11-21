package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"tally/locales"
)

type pageID string

const (
	pageOverview    pageID = "overview"
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

// sidebarWidth 与 MyGo Music 的侧栏一致。
const sidebarWidth = 232

// topStripHeight 是窗口顶部留给拖动与标题的高度。
const topStripHeight = 40

// pagePadding 是页面内容距窗口两侧与底部的距离，与 MyGo Music 的分页内边距一致。
const pagePadding = 28

func (a *app) normalizedPage() pageID {
	if a.page == "" {
		return pageOverview
	}
	return a.page
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
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	page := ui.Row(c).Fill().AlignItems(ui.Stretch)
	if !c.Vibrancy() {
		page.Background(p.canvas)
	}
	page.Children(func() {
		a.sidebar(c, p)
		top, right, bottom, left := pageInsets(c)
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(p.canvas).
			Padding(top, right, bottom, left).Role(ui.RoleMain).Children(func() {
			if a.loadError != "" {
				notice(c, p, noticeDanger, locales.PlanLoadError()+": "+a.loadError)
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
	a.zeroModal(c, p)
}

// sidebar 是唯一的导航入口，顶部为级切换，底部为帮助与设置。
func (a *app) sidebar(c *ui.Context, p uiPalette) {
	t := p.theme
	top := float32(14)
	if c.TitleBar().Left > 0 {
		top = topStrip(c) + 4
	}
	side := ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Padding(top, 14, 14, 14).
		Gap(t.Space(1)).DragWindow()
	if !c.Vibrancy() {
		side.Background(p.panelMuted)
	}
	side.Children(func() {
		a.cohortSwitcher(c, p)
		ui.Scroll(c.Key("nav")).Grow(1).Margin(0, -6).Padding(0, 6).Gap(t.Space(0.5)).Children(func() {
			a.navItem(c, p, pageOverview, iconHome, locales.NavOverview())
			a.navSection(c, p, locales.NavGroupData())
			a.navItem(c, p, pageCohorts, iconCohort, locales.NavCohorts())
			a.navItem(c, p, pageStudents, iconStudents, locales.NavStudents())
			a.navItem(c, p, pageFaculty, iconFaculty, locales.NavFaculty())
			a.navItem(c, p, pageGroups, iconGroups, locales.NavGroups())
			a.navItem(c, p, pageBaseline, iconBaseline, locales.NavBaseline())
			a.navItem(c, p, pageExams, iconExams, locales.NavExams())
			a.navItem(c, p, pageQuality, iconQuality, locales.NavQuality())
			a.navItem(c, p, pageImports, iconImports, locales.NavImports())
			a.navSection(c, p, locales.NavGroupAssessment())
			a.navItem(c, p, pageAssessments, iconBatches, locales.NavAssessments())
			a.navItem(c, p, pageResults, iconResults, locales.NavResults())
			a.navSection(c, p, locales.NavGroupAnalysis())
			a.navItem(c, p, pageAnalysis, iconAnalysis, locales.NavAnalysis())
			a.navItem(c, p, pagePlans, iconPlans, locales.NavPlans())
		})
		rule(c)
		ui.Column(c).Gap(t.Space(0.5)).Padding(t.Space(1.5), 0, 0, 0).Children(func() {
			a.sidebarAction(c, p, iconHelp, locales.NavHelp(), func() {
				a.page = pageHelp
			})
			a.sidebarAction(c, p, iconSettings, locales.NavSettings(), func() {
				a.openSettings()
			})
		})
	})
}

// cohortSwitcher 是侧栏顶部的年级切换，形态与 MyGo Music 的来源切换一致：
// 一个轻量下拉，菜单里列出全部年级，当前年级下面接着它的阶段。
// 没有年级时整块不显示。
func (a *app) cohortSwitcher(c *ui.Context, p uiPalette) {
	if len(a.cohorts) == 0 {
		return
	}
	const height = float32(24)
	chosen := a.selectedCohortID
	sel := ui.SelectBase(c.Key("cohort"), &chosen)
	trigger := sel.Trigger.Height(height).PaddingX(7).Gap(4).Radius(7).AlignItems(ui.Center).
		FocusRing(false).Label(locales.CohortSelectLabel())
	if left := c.TitleBar().Left; left > 0 {
		trigger.Absolute().Top((topStrip(c) - height) / 2).Left(left + 12).
			MaxWidth(sidebarWidth - left - 22)
	} else {
		trigger.AlignSelf(ui.Start).Margin(0, 0, 4, -7).MaxWidth(sidebarWidth - 28)
	}
	color := p.muted
	if trigger.Hovered() || sel.Open() {
		trigger.Background(p.hover)
		color = p.ink
	}
	trigger.Children(func() {
		name := ""
		if cohort := a.selectedCohort(); cohort != nil {
			name = a.cohortName(cohort.EnrollmentYear)
		}
		ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(color).
			SingleLine().Shrink(1).MinWidth(0)
		ui.Icon(c, iconUpDown).Size(11, 11).TextColor(p.muted)
	})
	selectedCohort := a.selectedCohort()
	sel.Popup(func(panel ui.Element) {
		panel.Margin(6, 0, 0, 0).Padding(5).Gap(1).MinWidth(180).MaxWidth(sidebarWidth+60).
			Radius(12).Background(p.panel).Border(0.5, p.border).
			Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.35))
		ui.Scroll(c).MaxHeight(320).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				for _, cohort := range a.cohorts {
					cohort := cohort
					name := a.cohortName(cohort.EnrollmentYear)
					selected := selectedCohort != nil && cohort.ID == selectedCohort.ID
					item := sel.Item("cohort-" + strconv.FormatInt(cohort.ID, 10)).Label(name).
						Height(30).PaddingX(8).Gap(8).Radius(7).AlignItems(ui.Center).
						TextColor(p.ink)
					if item.Highlighted() {
						item.Background(p.fill)
					}
					item.Children(func() {
						checkOpacity := float32(0)
						if selected {
							checkOpacity = 1
						}
						ui.Icon(c, iconCheck).Size(14, 14).TextColor(p.ink).Shrink(0).
							Opacity(checkOpacity)
						ui.Text(c, name).FontSize(13).TextColor(p.ink).SingleLine().
							Shrink(1).MinWidth(0)
					})
					if !selected {
						continue
					}
					a.stageMenuItems(c, p, sel)
				}
			})
		})
	})
	if !trigger.Changed() {
		return
	}
	switch {
	case strings.HasPrefix(chosen, "cohort-"):
		for _, cohort := range a.cohorts {
			if strconv.FormatInt(cohort.ID, 10) != strings.TrimPrefix(chosen, "cohort-") {
				continue
			}
			if selectedCohort == nil || cohort.ID != selectedCohort.ID {
				a.selectCohort(a.cohortName(cohort.EnrollmentYear))
			}
			return
		}
	case strings.HasPrefix(chosen, "stage-"):
		a.selectStageByID(strings.TrimPrefix(chosen, "stage-"))
	}
}

// stageMenuItems 在当前年级下面列出它的阶段。
func (a *app) stageMenuItems(c *ui.Context, p uiPalette, sel ui.SelectParts[string]) {
	labels, ids := a.stageOptions()
	for index, label := range labels {
		label, id := label, ids[index]
		selected := id == a.selectedStageID
		item := sel.Item("stage-" + strconv.FormatInt(id, 10)).Label(label).
			Height(28).PaddingStart(30).PaddingEnd(8).Gap(8).Radius(7).AlignItems(ui.Center).
			TextColor(p.inkSecondary)
		if item.Highlighted() {
			item.Background(p.fill)
		}
		item.Children(func() {
			ui.Text(c, label).FontSize(12.5).TextColor(p.inkSecondary).Grow(1).
				Shrink(1).MinWidth(0).SingleLine()
			if selected {
				ui.Icon(c, iconCheck).Size(13, 13).TextColor(p.accent)
			}
		})
	}
}

// selectStageByID 按标识切换当前年级的阶段。
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

// navSection 是导航里的分组标题。
func (a *app) navSection(c *ui.Context, p uiPalette, title string) {
	ui.Text(c, title).FontSize(captionSize(p.theme)).FontWeight(600).TextColor(p.ink).
		Padding(p.theme.Space(2.5), p.theme.Space(2.5), p.theme.Space(0.5), p.theme.Space(2.5)).
		SingleLine()
}

// navItem 是导航里的一个页面入口。选中项用浅底色和强调色图标标记，
// 与 MyGo Music 的侧栏条目一致。
func (a *app) navItem(c *ui.Context, p uiPalette, page pageID, icon *ui.SVG, label string) {
	selected := a.normalizedPage() == page
	b := ui.ButtonBase(c.Key(string(page))).Label(label).Height(34).PaddingX(10).Gap(10).
		Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	background := ui.Transparent
	switch {
	case selected:
		background = p.fill
	case b.Hovered():
		background = p.hover
	}
	b.Background(b.AnimateColor("bg", background, ui.Animation{Duration: 120 * time.Millisecond}))
	color := p.ink
	iconColor := p.muted
	if selected {
		iconColor = p.accent
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(iconColor)
		ui.Text(c, label).FontSize(13.5).FontWeight(500).TextColor(color).SingleLine()
	})
	if b.Clicked() {
		a.page = page
	}
}

func (a *app) sidebarAction(c *ui.Context, p uiPalette, icon *ui.SVG, label string, action func()) {
	t := p.theme
	button := ui.ButtonBase(c.Key(label)).Label(label).Height(t.Space(8)).
		Padding(0, t.Space(2.5)).Gap(t.Space(2)).Radius(t.Space(2)).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if button.Hovered() {
		button.Background(p.hover)
	}
	button.Children(func() {
		ui.Icon(c, icon).FontSize(t.Space(4)).TextColor(p.muted)
		ui.Text(c, label).FontSize(bodySize(t)).TextColor(p.ink)
	})
	if button.Clicked() {
		action()
	}
}

func (a *app) pageContent(c *ui.Context, p uiPalette) {
	page := a.normalizedPage()
	if page != a.lastPage {
		if page == pageQuality {
			a.reloadQuality(context.Background())
		}
		a.lastPage = page
	}
	if a.importWizardActive() {
		a.importWizardPage(c, p)
		return
	}
	switch page {
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
