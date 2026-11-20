package main

import (
	"context"

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
	pageSettings    pageID = "settings"
	pageHelp        pageID = "help"
)

func (a *app) normalizedPage() pageID {
	if a.page == "" {
		return pageOverview
	}
	return a.page
}

func (a *app) view(c *ui.Context) {
	p := uiPaletteFor(c)
	t := p.theme
	ui.Column(c).Fill().Background(p.canvas).Children(func() {
		a.shellBar(c, p)
		ui.Row(c).Grow(1).Children(func() {
			a.sidebar(c, p)
			ui.Box(c).Width(1).FillHeight().Background(p.border)
			ui.Column(c).Grow(1).FillHeight().Padding(t.Space(7), t.Space(8)).Children(func() {
				if a.loadError != "" {
					ui.Scroll(c).Fill().Children(func() {
						ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
							notice(c, p, noticeDanger, locales.PlanLoadError()+": "+a.loadError)
						})
					})
					return
				}
				a.pageContent(c, p)
			}).Role(ui.RoleMain)
		})
	})
}

func (a *app) shellBar(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Background(p.panel).Children(func() {
		ui.Row(c).Height(t.Space(14)).Padding(0, t.Space(8)).
			AlignItems(ui.Center).Gap(t.Space(4)).Children(func() {
			a.cohortContext(c, p)
			ui.Spacer(c)
			a.languageSwitch(c)
		})
		ui.Divider(c)
	}).Role(ui.RoleBanner)
}

func (a *app) cohortContext(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	if cohort == nil {
		ui.Text(c, locales.NoCohort()).FontSize(bodySize(t)).TextColor(p.muted)
		return
	}
	labels := make([]string, 0, len(a.cohorts))
	for _, item := range a.cohorts {
		labels = append(labels, a.cohortName(item.EnrollmentYear))
	}
	chosen := a.cohortName(cohort.EnrollmentYear)
	next := chosen
	ui.Select(c, &next, labels).Label(locales.CohortSelectLabel()).Width(t.Space(52))
	if next != chosen {
		a.selectCohort(next)
		cohort = a.selectedCohort()
	}
	if cohort == nil {
		return
	}
	stage := a.displayStage()
	context := a.cohortName(cohort.EnrollmentYear) + " · " +
		a.stageName(stage.GradeLevel) + " · " + a.classOfName(cohort.GraduationYear)
	ui.Column(c).Gap(0).Children(func() {
		ui.Text(c, context).FontSize(bodySize(t)).Bold().SingleLine()
		ui.Text(c, a.academicYearLabel(stage.AcademicYearStart)).FontSize(captionSize(t)).
			TextColor(p.muted).SingleLine()
	})
}

func (a *app) sidebar(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Width(t.Space(58)).FillHeight().Background(p.panelMuted).Children(func() {
		a.brand(c, p)
		ui.Divider(c)
		a.sidebarElement(c, func() {
			ui.SidebarSection(c, locales.NavGroupWorkbench(), nil, func() {
				ui.SidebarItem(c, string(pageOverview), iconHome, locales.NavOverview())
			})
			ui.SidebarSection(c, locales.NavGroupData(), nil, func() {
				ui.SidebarItem(c, string(pageCohorts), iconCohort, locales.NavCohorts())
				ui.SidebarItem(c, string(pageStudents), iconStudents, locales.NavStudents())
				ui.SidebarItem(c, string(pageFaculty), iconFaculty, locales.NavFaculty())
				ui.SidebarItem(c, string(pageGroups), iconGroups, locales.NavGroups())
				ui.SidebarItem(c, string(pageBaseline), iconBaseline, locales.NavBaseline())
				ui.SidebarItem(c, string(pageExams), iconExams, locales.NavExams())
				ui.SidebarItem(c, string(pageQuality), iconQuality, locales.NavQuality())
				ui.SidebarItem(c, string(pageImports), iconImports, locales.NavImports())
			})
			ui.SidebarSection(c, locales.NavGroupAssessment(), nil, func() {
				ui.SidebarItem(c, string(pageAssessments), iconBatches, locales.NavAssessments())
				ui.SidebarItem(c, string(pageResults), iconResults, locales.NavResults())
			})
			ui.SidebarSection(c, locales.NavGroupAnalysis(), nil, func() {
				ui.SidebarItem(c, string(pageAnalysis), iconAnalysis, locales.NavAnalysis())
			})
			ui.SidebarSection(c, locales.NavGroupPlans(), nil, func() {
				ui.SidebarItem(c, string(pagePlans), iconPlans, locales.NavPlans())
			})
		}).Grow(1).Label(locales.MainNavigation())
		ui.Divider(c)
		a.sidebarElement(c, func() {
			ui.SidebarItem(c, string(pageSettings), iconSettings, locales.NavSettings())
			ui.SidebarItem(c, string(pageHelp), iconHelp, locales.NavHelp())
		}).Label(locales.NavGroupSupport())
	})
}

func (a *app) sidebarElement(c *ui.Context, body func()) ui.Element {
	selected := string(a.normalizedPage())
	element := ui.Sidebar(c, &selected, body)
	if selected != string(a.normalizedPage()) {
		a.page = pageID(selected)
	}
	return element
}

func (a *app) brand(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).Padding(t.Space(5), t.Space(5)).AlignItems(ui.Center).Gap(t.Space(2.5)).
		Children(func() {
			ui.Box(c).Size(t.Space(9), t.Space(9)).Radius(t.Space(1.5)).Background(p.accent).
				Center().Children(func() {
				ui.Icon(c, iconMark).FontSize(t.Space(5.5)).TextColor(p.accentText)
			})
			ui.Column(c).Gap(0).Children(func() {
				ui.Text(c, locales.AppTitle()).FontSize(sectionSize(t)).Bold().LetterSpacing(-0.2)
				ui.Text(c, locales.AppTagline()).FontSize(captionSize(t)).TextColor(p.muted)
			})
		})
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
		a.batchPage(c, p)
	case pageResults:
		a.resultsPage(c, p)
	case pageAnalysis:
		a.placeholderPage(c, p, locales.NavAnalysis(), locales.AnalysisPageSubtitle(),
			iconAnalysis, locales.AnalysisEmpty())
	case pagePlans:
		a.planPage(c, p)
	case pageSettings:
		a.settingsPage(c, p)
	case pageHelp:
		a.helpPage(c, p)
	default:
		a.overviewPage(c, p)
	}
}
