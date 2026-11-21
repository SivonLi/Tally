package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/nav"

	"tally/internal/calc"
	"tally/internal/pipeline"
	"tally/internal/store"
	"tally/locales"
)

type publishedVersion struct {
	PlanName string
	Version  store.PlanVersion
}

func (a *app) reloadPublished(ctx context.Context) {
	a.published = nil
	if a.store == nil {
		return
	}
	for _, plan := range a.plans {
		versions, err := a.store.ListVersions(ctx, plan.ID)
		if err != nil {
			a.loadError = err.Error()
			return
		}
		for _, version := range versions {
			if version.Status == "published" {
				a.published = append(a.published, publishedVersion{PlanName: plan.Name, Version: version})
			}
		}
	}
	sort.Slice(a.published, func(i, j int) bool {
		if a.published[i].PlanName != a.published[j].PlanName {
			return a.published[i].PlanName < a.published[j].PlanName
		}
		return a.published[i].Version.Version < a.published[j].Version.Version
	})
	if a.publishedByLabel(a.calculateVersion) == nil && len(a.published) > 0 {
		a.calculateVersion = a.versionLabel(a.published[0])
	}
}

func (a *app) versionLabel(item publishedVersion) string {
	return fmt.Sprintf("%s v%d", item.PlanName, item.Version.Version)
}

func (a *app) versionOptions() []string {
	options := make([]string, 0, len(a.published))
	for _, item := range a.published {
		options = append(options, a.versionLabel(item))
	}
	return options
}

func (a *app) publishedByLabel(label string) *publishedVersion {
	for index := range a.published {
		if a.versionLabel(a.published[index]) == label {
			return &a.published[index]
		}
	}
	return nil
}

func (a *app) labelForVersionID(id int64) string {
	for _, item := range a.published {
		if item.Version.ID == id {
			return a.versionLabel(item)
		}
	}
	return ""
}

func (a *app) reloadAssessments(ctx context.Context) {
	if a.store == nil {
		return
	}
	items, err := a.store.ListAssessmentOverview(ctx)
	if err != nil {
		a.assessmentError = err.Error()
		return
	}
	a.assessments = items
	found := false
	for _, item := range items {
		if item.AssessmentID == a.selectedAssessmentID {
			found = true
			break
		}
	}
	if !found {
		a.selectedAssessmentID = 0
		a.assessmentRows = nil
		a.assessmentPersonal = nil
		if len(items) > 0 {
			a.selectAssessment(items[0].AssessmentID)
		}
		return
	}
	if a.selectedAssessmentID != 0 {
		a.loadAssessmentDetail(ctx)
	}
}

func (a *app) selectedAssessmentOverview() *store.AssessmentOverview {
	for index := range a.assessments {
		if a.assessments[index].AssessmentID == a.selectedAssessmentID {
			return &a.assessments[index]
		}
	}
	return nil
}

func (a *app) selectAssessment(id int64) {
	a.selectedAssessmentID = id
	a.assessmentError = ""
	a.assessmentMessage = ""
	a.loadAssessmentDetail(context.Background())
}

func (a *app) loadAssessmentDetail(ctx context.Context) {
	if a.store == nil || a.selectedAssessmentID == 0 {
		return
	}
	detail, err := a.store.LoadAssessmentContext(ctx, a.selectedAssessmentID)
	if err != nil {
		a.assessmentError = err.Error()
		return
	}
	a.assessmentContext = detail
	if detail.Assessment.PlanVersion != 0 {
		if label := a.labelForVersionID(detail.Assessment.PlanVersion); label != "" {
			a.calculateVersion = label
		}
	}
	run, rows, personal, err := a.store.LatestAssessmentRun(ctx, a.selectedAssessmentID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			a.assessmentError = err.Error()
			return
		}
		a.assessmentRun = store.ResultRun{}
		a.assessmentRows = nil
		a.assessmentPersonal = nil
		return
	}
	a.assessmentRun = run
	a.assessmentRows = rows
	a.assessmentPersonal = personal
}

func (a *app) assessmentLabel() string {
	detail := a.assessmentContext
	if detail.Assessment.ID == 0 {
		return ""
	}
	return fmt.Sprintf("%s · %s · %s", a.examBatchLabel(detail.Exam),
		a.cohortName(detail.Cohort.EnrollmentYear), a.stageName(detail.Stage.GradeLevel))
}

func (a *app) assessmentPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.AssessmentTitle(), locales.AssessmentSubtitle(), func() {
			if labelledButton(c, locales.AssessmentNewExam(), iconPlus, true).Clicked() {
				a.page = pageExams
				a.openNewExam()
			}
		})
		if a.assessmentError != "" {
			notice(c, p, noticeDanger, a.assessmentError)
		}
		if len(a.assessments) == 0 {
			emptyState(c, p, iconEmptyBatch, locales.AssessmentEmpty(), locales.AssessmentEmptyHint(), func() {
				if labelledButton(c, locales.AssessmentNewExam(), iconPlus, true).Clicked() {
					a.page = pageExams
					a.openNewExam()
				}
			})
			return
		}
		ui.Split(c, &a.batchSidebarWidth, func() {
			a.assessmentList(c, p)
		}, func() {
			a.assessmentDetail(c, p)
		}).Grow(1)
	})
	a.zeroModal(c, p)
	a.newExamModal(c, p)
}

func (a *app) assessmentList(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.batchSidebarWidth == 0 {
		a.batchSidebarWidth = t.Space(64)
	}
	listPanel(c, p, locales.AssessmentListTitle(), len(a.assessments), func() {
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
				for _, item := range a.assessments {
					item := item
					selected := item.AssessmentID == a.selectedAssessmentID
					background := ui.Transparent
					if selected {
						background = p.accentSoft
					}
					row := textButton(c, a.assessmentOverviewLabel(item)).FillWidth().
						Padding(t.Space(2), t.Space(2.5)).Radius(t.Space(1.5)).
						Background(background).AlignItems(ui.Start)
					row.Children(func() {
						ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
							ui.Text(c, a.assessmentOverviewLabel(item)).FontSize(bodySize(t)).
								FontWeight(500).SingleLine()
							ui.Textf(c, locales.AssessmentListMeta(), a.academicYearLabel(item.AcademicYearStart),
								a.examTermName(item.Term),
								fmt.Sprintf(locales.AssessmentStudentCount(), formatInt(item.RosterCount))).
								FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
						})
					})
					if row.Clicked() {
						a.selectAssessment(item.AssessmentID)
					}
				}
			})
		})
	})
}

func (a *app) assessmentOverviewLabel(item store.AssessmentOverview) string {
	return fmt.Sprintf("%s · %s", a.examTypeName(item.ExamType), item.CohortName)
}

func (a *app) assessmentDetail(c *ui.Context, p uiPalette) {
	overview := a.selectedAssessmentOverview()
	if overview == nil {
		emptyState(c, p, iconEmptyBatch, locales.AssessmentSelectHint(), "", nil)
		return
	}
	t := p.theme
	stage := 0
	switch {
	case overview.RosterCount == 0:
		stage = 0
	case overview.ScoreCount == 0 || overview.CutoffCount == 0 || overview.TargetCount == 0 || overview.GroupCount == 0:
		stage = 0
	case overview.Status == "calculated":
		stage = 2
	default:
		stage = 1
	}
	ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
				ui.Text(c, a.assessmentLabel()).FontSize(titleSize(t)).Bold().
					LetterSpacing(-0.2).Role(ui.RoleHeading).Level(1)
				ui.Textf(c, locales.AssessmentDetailMeta(), a.academicYearLabel(overview.AcademicYearStart),
					a.examTermName(overview.Term), overview.ExamDate).FontSize(captionSize(t)).
					TextColor(p.muted).FontFeatures("tnum")
			})
			statusBadge(c, p, overview.Status, a.assessmentStatusLabel(overview.Status))
		})
		a.assessmentSteps(c, p, stage)
		a.assessmentActions(c, p, *overview)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
				a.assessmentChecklist(c, p, *overview)
				a.assessmentResults(c, p)
			})
		})
	})
}

func (a *app) assessmentSteps(c *ui.Context, p uiPalette, current int) {
	items := []nav.StepItem{
		{Title: locales.AssessmentStepPrepare()},
		{Title: locales.AssessmentStepCalculate()},
		{Title: locales.AssessmentStepExport()},
	}
	nav.Steps(c, nav.StepsProps{Items: items, Current: current})
}

func (a *app) assessmentStatusLabel(status string) string {
	switch status {
	case "calculated":
		return locales.AssessmentStatusCalculated()
	default:
		return locales.AssessmentStatusDraft()
	}
}

func (a *app) assessmentChecklist(c *ui.Context, p uiPalette, overview store.AssessmentOverview) {
	t := p.theme
	items := []struct {
		Label string
		Count int
		Meta  string
		Tab   int
	}{
		{locales.ExamTabRoster(), overview.RosterCount,
			fmt.Sprintf(locales.AssessmentCountStudents(), formatInt(overview.RosterCount)), 0},
		{locales.ExamTabScores(), overview.ScoreCount,
			fmt.Sprintf(locales.AssessmentCountScores(), formatInt(overview.ScoreCount)), 1},
		{locales.ExamTabCutoffs(), overview.CutoffCount,
			fmt.Sprintf(locales.AssessmentCountCutoffs(), formatInt(overview.CutoffCount)), 2},
		{locales.ExamTabTargets(), overview.TargetCount,
			fmt.Sprintf(locales.AssessmentCountTargets(), formatInt(overview.TargetCount)), 3},
		{locales.ExamTabGroups(), overview.GroupCount,
			fmt.Sprintf(locales.AssessmentCountGroups(), formatInt(overview.GroupCount)), 4},
	}
	ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			sectionHeading(c, p, locales.AssessmentChecklistTitle(), -1)
		})
		rule(c)
		for _, item := range items {
			item := item
			missing := item.Count == 0
			row := textButton(c, item.Label+" "+item.Meta).FillWidth().
				Padding(t.Space(2), 0).Radius(t.Space(1.5)).Background(ui.Transparent).
				AlignItems(ui.Center)
			row.Children(func() {
				if missing {
					ui.Icon(c, iconAlert).FontSize(t.Space(4)).TextColor(p.warning)
				} else {
					ui.Icon(c, iconCheck).FontSize(t.Space(4)).TextColor(p.accent)
				}
				ui.Text(c, item.Label).FontSize(bodySize(t)).Grow(1).SingleLine().
					TextAlign(ui.Start)
				ui.Text(c, item.Meta).FontSize(captionSize(t)).
					TextColor(p.muted).SingleLine().FontFeatures("tnum")
				ui.Icon(c, iconArrowRight).FontSize(t.Space(3.5)).TextColor(p.muted)
			})
			if row.Clicked() {
				a.openExamTab(overview, item.Tab)
			}
			rule(c)
		}
	})
}

// openExamTab 跳到这场考试的对应页签，让准备清单可以直接补数据。
func (a *app) openExamTab(overview store.AssessmentOverview, tab int) {
	a.page = pageExams
	a.selectExam(overview.ExamID)
	a.examCohortID = overview.CohortID
	a.examTab = tab
	a.reloadExamData(context.Background())
}

func (a *app) assessmentActions(c *ui.Context, p uiPalette, overview store.AssessmentOverview) {
	t := p.theme
	ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
		rule(c)
		ui.Row(c).Wrap().AlignItems(ui.End).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Width(t.Space(56)).MinWidth(0).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.AssessmentPlanVersion()).FontSize(captionSize(t)).TextColor(p.muted)
				if len(a.published) == 0 {
					ui.Text(c, locales.BatchSelectVersion()).FontSize(bodySize(t)).TextColor(p.warning)
					return
				}
				selectField(c, &a.calculateVersion, a.versionOptions(), locales.AssessmentPlanVersion()).
					Width(t.Space(56)).Label(locales.AssessmentPlanVersion())
			})
			ui.Spacer(c)
			ui.Row(c).Wrap().Gap(t.Space(2)).Children(func() {
				if labelledButton(c, locales.AssessmentOpenExam(), iconExams, false).Clicked() {
					a.openExamTab(overview, 0)
				}
				calculate := labelledButton(c, locales.AssessmentCalculate(), iconCalculate, true)
				calculate.Disabled(a.assessmentBusy || len(a.published) == 0)
				if calculate.Clicked() {
					a.startAssessmentCalculate()
				}
				export := labelledButton(c, locales.AssessmentExport(), iconExport, false)
				export.Disabled(a.assessmentBusy || len(a.assessmentRows) == 0)
				if export.Clicked() {
					a.chooseAssessmentExport()
				}
			})
		})
		if a.assessmentBusy {
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Spinner(c).Label(locales.BatchWorking())
				ui.Text(c, locales.BatchWorking()).FontSize(bodySize(t)).TextColor(p.muted)
			})
		}
		if a.assessmentMessage != "" {
			notice(c, p, noticeSuccess, a.assessmentMessage)
		}
		if len(a.assessmentRows) == 0 {
			ui.Text(c, locales.AssessmentNotCalculated()).FontSize(bodySize(t)).
				TextColor(p.muted).Pretty()
		}
	})
}

func (a *app) assessmentResults(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			sectionHeading(c, p, locales.AssessmentResults(), len(a.assessmentRows))
			ui.Spacer(c)
			if a.assessmentRun.ID != 0 {
				ui.Textf(c, locales.AssessmentRunMeta(), formatTime(a.assessmentRun.CreatedAt)).
					FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
			}
		})
		if len(a.assessmentRows) == 0 {
			ui.Text(c, locales.AssessmentNoResults()).FontSize(bodySize(t)).TextColor(p.muted)
			return
		}
		rows := a.assessmentRows
		a.resultTable.Key = func(row int) any {
			return fmt.Sprintf("%d:%s", rows[row].StatGroupID, rows[row].Subject)
		}
		a.resultTable.RowHeight = t.Space(8)
		columns := []ui.TableColumn{
			{Title: locales.ColumnClass(), Width: t.Space(18)},
			{Title: locales.ColumnSubject(), Width: t.Space(14)},
			{Title: locales.ColumnStatGroup(), Width: t.Space(20)},
			{Title: locales.ColumnTeachers(), Width: 0},
			{Title: locales.ColumnTotal(), Width: t.Space(16), Align: ui.End},
			{Title: locales.ColumnRank(), Width: t.Space(12), Align: ui.End},
			{Title: locales.ColumnTier(), Width: t.Space(12)},
			{Title: locales.ColumnPoints(), Width: t.Space(12), Align: ui.End},
		}
		rule(c)
		ui.Table(c, &a.resultTable, columns, len(rows), func(row, col int) {
			item := rows[row]
			switch col {
			case 0:
				ui.Text(c, item.LeadClassName).SingleLine()
			case 1:
				ui.Text(c, subjectLabel(item.Subject)).SingleLine()
			case 2:
				ui.Text(c, item.StatGroupName).SingleLine()
			case 3:
				ui.Text(c, decodeNames(item.TeacherNames)).SingleLine()
			case 4:
				ui.Text(c, formatDecimal(item.Total, 1)).FontFeatures("tnum")
			case 5:
				ui.Text(c, formatInt(item.Rank)).FontFeatures("tnum")
			case 6:
				ui.Text(c, levelLabel(item.TierIndex))
			case 7:
				ui.Text(c, formatDecimal(item.Points, 1)).FontFeatures("tnum")
			}
		}).Label(locales.AssessmentResults()).FillWidth()
	})
}

func (a *app) startAssessmentCalculate() {
	if a.store == nil || a.selectedAssessmentID == 0 {
		return
	}
	version := a.publishedByLabel(a.calculateVersion)
	if version == nil {
		a.assessmentError = locales.BatchSelectVersion()
		return
	}
	assessmentID := a.selectedAssessmentID
	a.assessmentBusy = true
	a.assessmentError = ""
	work := func() (calc.Result, error) {
		ctx := context.Background()
		return pipeline.CalculateAssessment(ctx, a.store, assessmentID, version.Version.ID)
	}
	run := func() {
		result, err := work()
		if a.win == nil {
			a.finishAssessmentCalculate(result, err)
			return
		}
		a.win.Update(func() {
			a.finishAssessmentCalculate(result, err)
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *app) finishAssessmentCalculate(result calc.Result, err error) {
	a.assessmentBusy = false
	if err != nil {
		if zero, ok := err.(*calc.ZeroTargetError); ok {
			a.zeroIssues = zero.Issues
			a.zeroOpen = true
			a.zeroError = ""
			if a.zeroReason == "" {
				a.zeroReason = locales.ZeroDefaultReason()
			}
			a.initZeroValues()
			return
		}
		a.assessmentError = err.Error()
		return
	}
	a.zeroOpen = false
	a.assessmentError = ""
	a.assessmentMessage = fmt.Sprintf(locales.AssessmentCalculated(), len(result.Rows), len(result.Personal))
	a.reloadAssessments(context.Background())
}

func (a *app) chooseAssessmentExport() {
	name := "Tally.xlsx"
	if overview := a.selectedAssessmentOverview(); overview != nil {
		name = a.assessmentOverviewLabel(*overview) + ".xlsx"
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       locales.AssessmentExport(),
			DefaultPath: name,
			Filters:     []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx"}}},
		})
		if err != nil || path == "" {
			return
		}
		if a.win == nil {
			a.startAssessmentExport(path)
			return
		}
		a.win.Update(func() {
			a.startAssessmentExport(path)
		})
	}()
}

func (a *app) startAssessmentExport(path string) {
	if a.store == nil || a.selectedAssessmentID == 0 {
		return
	}
	assessmentID := a.selectedAssessmentID
	a.assessmentBusy = true
	a.assessmentError = ""
	run := func() {
		ctx := context.Background()
		err := pipeline.ExportAssessment(ctx, a.store, assessmentID, path)
		if a.win == nil {
			a.finishAssessmentExport(path, err)
			return
		}
		a.win.Update(func() {
			a.finishAssessmentExport(path, err)
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *app) finishAssessmentExport(path string, err error) {
	a.assessmentBusy = false
	if err != nil {
		a.assessmentError = err.Error()
		return
	}
	a.assessmentError = ""
	a.assessmentMessage = fmt.Sprintf(locales.BatchExported(), path)
}

func (a *app) initZeroValues() {
	if a.zeroValues == nil {
		a.zeroValues = map[string]*float64{}
	}
	for _, issue := range a.zeroIssues {
		tiers := issue.MissingTiers
		if len(tiers) == 0 {
			tiers = issue.AllTiers
		}
		for _, tier := range tiers {
			key := issue.ClassID + ":" + tier
			if a.zeroValues[key] == nil {
				value := 0.0
				a.zeroValues[key] = &value
			}
		}
	}
}

func (a *app) zeroModal(c *ui.Context, p uiPalette) {
	if !a.zeroOpen {
		return
	}
	t := p.theme
	excludeClass := int64(0)
	fill := false
	open := true
	if feedback.Modal(c, &open, feedback.ModalProps{
		Title:      locales.ZeroTitle(),
		OkText:     locales.ZeroFill(),
		CancelText: locales.ZeroClose(),
		Width:      t.Space(170),
	}, func() {
		ui.Column(c).Width(t.Space(170)).Gap(t.Space(4)).Children(func() {
			ui.Row(c).AlignItems(ui.Start).Gap(t.Space(3)).Children(func() {
				ui.Box(c).Size(t.Space(11), t.Space(11)).Radius(t.Space(3)).
					Background(p.warningSoft).Center().Children(func() {
					ui.Icon(c, iconAlert).FontSize(t.Space(5)).TextColor(p.warning)
				})
				ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
					ui.Text(c, locales.ZeroIntro()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
				})
			})
			ui.Scroll(c).MaxHeight(t.Space(72)).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
					for index := range a.zeroIssues {
						a.zeroIssueCard(c, p, index, func(classID int64) {
							excludeClass = classID
						})
					}
				})
			})
			a.dialogField(c, "zero-reason", locales.ZeroReason(), "", func() {
				textField(c, &a.zeroReason, locales.ZeroReason(), "")
			})
			if a.zeroError != "" {
				notice(c, p, noticeDanger, a.zeroError)
			}
		})
	}) {
		fill = true
	}
	if !open {
		a.zeroOpen = false
	}
	if excludeClass != 0 {
		a.startExcludeAssessmentClass(excludeClass)
	}
	if fill {
		a.startFillZero()
	}
}

func (a *app) zeroIssueCard(c *ui.Context, p uiPalette, index int, exclude func(int64)) {
	t := p.theme
	issue := a.zeroIssues[index]
	panel(c, p).Background(p.panelMuted).Padding(t.Space(3)).Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, issue.ClassName).FontSize(bodySize(t)).Bold().Grow(1)
			if plainButton(c, locales.ZeroExclude()).Clicked() {
				classID, _ := strconv.ParseInt(issue.ClassID, 10, 64)
				exclude(classID)
			}
		})
		tiers := issue.MissingTiers
		if len(tiers) == 0 {
			tiers = issue.AllTiers
			ui.Text(c, locales.ZeroExplicitZero()).FontSize(captionSize(t)).TextColor(p.muted)
		}
		for _, tier := range tiers {
			key := issue.ClassID + ":" + tier
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Text(c, a.tierLabel(tier)).Width(t.Space(16))
				name := issue.ClassName + " " + a.tierLabel(tier)
				numberField(c, a.zeroValues[key], 0, 100000, 1, key).
					Width(t.Space(22)).Label(name)
			})
		}
	})
}

type targetAdjustment struct {
	classID int64
	tier    string
	value   float64
}

func (a *app) startFillZero() {
	reason := strings.TrimSpace(a.zeroReason)
	if reason == "" {
		a.zeroError = locales.ZeroReasonRequired()
		return
	}
	var adjustments []targetAdjustment
	for _, issue := range a.zeroIssues {
		classID, err := strconv.ParseInt(issue.ClassID, 10, 64)
		if err != nil {
			a.zeroError = err.Error()
			return
		}
		tiers := issue.MissingTiers
		if len(tiers) == 0 {
			tiers = issue.AllTiers
		}
		for _, tier := range tiers {
			value := 0.0
			if pointer := a.zeroValues[issue.ClassID+":"+tier]; pointer != nil {
				value = *pointer
			}
			adjustments = append(adjustments, targetAdjustment{classID: classID, tier: tier, value: value})
		}
	}
	a.runZeroWork(func(ctx context.Context, assessmentID int64) error {
		for _, item := range adjustments {
			if err := a.store.AdjustAssessmentTarget(ctx, assessmentID, item.classID, item.tier, item.value, reason); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *app) startExcludeAssessmentClass(classID int64) {
	reason := strings.TrimSpace(a.zeroReason)
	if reason == "" {
		a.zeroError = locales.ZeroReasonRequired()
		return
	}
	a.runZeroWork(func(ctx context.Context, assessmentID int64) error {
		return a.store.ExcludeAssessmentClass(ctx, assessmentID, classID, reason)
	})
}

func (a *app) runZeroWork(work func(ctx context.Context, assessmentID int64) error) {
	if a.selectedAssessmentID == 0 {
		a.zeroError = locales.AssessmentSelectHint()
		return
	}
	version := a.publishedByLabel(a.calculateVersion)
	if version == nil {
		a.zeroError = locales.BatchSelectVersion()
		return
	}
	assessmentID := a.selectedAssessmentID
	a.assessmentBusy = true
	a.zeroError = ""
	run := func() {
		ctx := context.Background()
		err := work(ctx, assessmentID)
		var result calc.Result
		if err == nil {
			result, err = pipeline.CalculateAssessment(ctx, a.store, assessmentID, version.Version.ID)
		}
		if a.win == nil {
			a.finishZeroWork(result, err)
			return
		}
		a.win.Update(func() {
			a.finishZeroWork(result, err)
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *app) finishZeroWork(result calc.Result, err error) {
	a.assessmentBusy = false
	if err != nil {
		if zero, ok := err.(*calc.ZeroTargetError); ok {
			a.zeroIssues = zero.Issues
			a.initZeroValues()
			a.zeroError = ""
			return
		}
		a.zeroError = err.Error()
		a.assessmentError = err.Error()
		return
	}
	a.zeroOpen = false
	a.zeroError = ""
	a.assessmentError = ""
	a.assessmentMessage = fmt.Sprintf(locales.AssessmentCalculated(), len(result.Rows), len(result.Personal))
	a.reloadAssessments(context.Background())
}

func (a *app) resultsPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavResults(), locales.ResultsSubtitle(), func() {
			export := labelledButton(c, locales.AssessmentExport(), iconExport, false)
			export.Disabled(a.assessmentBusy || len(a.assessmentRows) == 0)
			if export.Clicked() {
				a.chooseAssessmentExport()
			}
		})
		if a.assessmentError != "" {
			notice(c, p, noticeDanger, a.assessmentError)
		}
		if a.assessmentMessage != "" {
			notice(c, p, noticeSuccess, a.assessmentMessage)
		}
		if len(a.assessments) == 0 {
			emptyState(c, p, iconResults, locales.ResultsEmpty(), locales.ResultsEmptyHint(), func() {
				if labelledButton(c, locales.AssessmentNewExam(), iconPlus, true).Clicked() {
					a.page = pageExams
					a.openNewExam()
				}
			})
			return
		}
		labels := make([]string, 0, len(a.assessments))
		chosen := ""
		for _, item := range a.assessments {
			label := a.assessmentOverviewLabel(item)
			labels = append(labels, label)
			if item.AssessmentID == a.selectedAssessmentID {
				chosen = label
			}
		}
		next := chosen
		selectField(c, &next, labels, locales.AssessmentResultsSelect()).Width(t.Space(72))
		if next != chosen {
			for _, item := range a.assessments {
				if a.assessmentOverviewLabel(item) == next {
					a.selectAssessment(item.AssessmentID)
					break
				}
			}
		}
		if len(a.assessmentRows) == 0 {
			emptyState(c, p, iconResults, locales.AssessmentNoResults(), locales.AssessmentNoResultsHint(), func() {
				if labelledButton(c, locales.AssessmentOpenAssessment(), iconBatches, false).Clicked() {
					a.page = pageAssessments
				}
			})
			return
		}
		a.assessmentResults(c, p)
	})
}
