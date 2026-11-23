package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/nav"

	"tally/internal/assessment"
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

// runOptionLabel 是一条核算记录在下拉里的文字：类型、时间、方案版本。
func (a *app) runOptionLabel(run store.AssessmentRunSummary) string {
	kind := locales.AssessmentRunTrial()
	if run.IsOfficial {
		kind = locales.AssessmentRunOfficial()
	}
	return fmt.Sprintf("%s · %s · %s", kind, formatTime(run.CreatedAt), a.labelForVersionID(run.PlanVersionID))
}

func (a *app) runOptions() []string {
	options := make([]string, 0, len(a.assessmentRuns))
	for _, run := range a.assessmentRuns {
		options = append(options, a.runOptionLabel(run))
	}
	return options
}

func (a *app) runIDForLabel(label string) (int64, bool) {
	for _, run := range a.assessmentRuns {
		if a.runOptionLabel(run) == label {
			return run.ID, true
		}
	}
	return 0, false
}

func (a *app) selectAssessmentRun(id int64) {
	a.assessmentRunChosen = true
	a.assessmentRunID = id
	a.loadAssessmentDetail(context.Background())
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
	a.assessmentRunChosen = false
	a.assessmentRunID = 0
	a.closeResultDetail()
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
	a.assessmentRuns, err = a.store.ListAssessmentRuns(ctx, a.selectedAssessmentID)
	if err != nil {
		a.assessmentError = err.Error()
		return
	}
	if !a.assessmentRunChosen || !a.runExists(a.assessmentRunID) {
		a.assessmentRunChosen = false
		a.assessmentRunID = a.defaultRunID()
	}
	a.closeResultDetail()
	if a.assessmentRunID == 0 {
		a.assessmentRun = store.ResultRun{}
		a.assessmentRows = nil
		a.assessmentPersonal = nil
		a.assessmentOfficialStale = false
		a.assessmentOfficialStaleItems = nil
		return
	}
	run, rows, personal, err := a.store.AssessmentRunDetail(ctx, a.selectedAssessmentID, a.assessmentRunID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			a.assessmentError = err.Error()
			return
		}
		a.assessmentRunChosen = false
		a.assessmentRunID = 0
		a.assessmentRuns = nil
		a.assessmentRun = store.ResultRun{}
		a.assessmentRows = nil
		a.assessmentPersonal = nil
		a.assessmentOfficialStale = false
		a.assessmentOfficialStaleItems = nil
		return
	}
	a.assessmentRun = run
	a.assessmentRows = rows
	a.assessmentPersonal = personal
	a.loadAssessmentStaleness(ctx)
}

// defaultRunID 默认显示的核算：最近一次正式核算，没有正式核算时取最近一次试算。
func (a *app) defaultRunID() int64 {
	for _, run := range a.assessmentRuns {
		if run.IsOfficial {
			return run.ID
		}
	}
	if len(a.assessmentRuns) > 0 {
		return a.assessmentRuns[0].ID
	}
	return 0
}

func (a *app) latestOfficialRun() store.AssessmentRunSummary {
	for _, run := range a.assessmentRuns {
		if run.IsOfficial {
			return run
		}
	}
	return store.AssessmentRunSummary{}
}

func (a *app) runExists(id int64) bool {
	for _, run := range a.assessmentRuns {
		if run.ID == id {
			return true
		}
	}
	return false
}

// loadAssessmentStaleness 对比当前数据与最新正式核算的输入摘要，
// 得到需要重新计算时的提示数据项。
func (a *app) loadAssessmentStaleness(ctx context.Context) {
	a.assessmentOfficialStale = false
	a.assessmentOfficialStaleItems = nil
	official := a.latestOfficialRun()
	if official.ID == 0 {
		return
	}
	var officialItems []string
	var officialStale bool
	var err error
	if official.ID == a.assessmentRun.ID {
		officialItems, officialStale, err = a.staleInputItems(ctx, a.assessmentRun)
	} else {
		detail, _, _, detailErr := a.store.AssessmentRunDetail(ctx, a.selectedAssessmentID, official.ID)
		if detailErr != nil {
			a.assessmentError = detailErr.Error()
			return
		}
		officialItems, officialStale, err = a.staleInputItems(ctx, detail)
	}
	if err != nil {
		a.assessmentError = err.Error()
		return
	}
	a.assessmentOfficialStale = officialStale
	a.assessmentOfficialStaleItems = officialItems
}

// staleInputItems 对比一次核算保存的输入摘要与当前数据，
// 返回变化的数据项与是否需要重新计算；
// 旧运行没有摘要时回退整体哈希对比，此时没有数据项清单。
func (a *app) staleInputItems(ctx context.Context, run store.ResultRun) ([]string, bool, error) {
	version, err := a.store.GetVersion(ctx, run.PlanVersionID)
	if err != nil {
		return nil, false, err
	}
	data, err := a.store.LoadAssessmentData(ctx, a.selectedAssessmentID)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(run.InputSummary) != "" {
		summary, err := store.InputSummary(version.Config, data)
		if err != nil {
			return nil, false, err
		}
		changed, err := store.ChangedInputItems(run.InputSummary, summary)
		if err != nil {
			return nil, false, err
		}
		return changed, len(changed) > 0, nil
	}
	hash, err := store.InputHash(version.Config, data)
	if err != nil {
		return nil, false, err
	}
	return nil, run.InputHash != hash, nil
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
			ui.Column(c).Fill().MinWidth(0).Clip().Padding(0, t.Space(4), 0, 0).Children(func() {
				a.assessmentList(c, p)
			})
		}, func() {
			ui.Column(c).Fill().MinWidth(0).Padding(0, 0, 0, t.Space(4)).Children(func() {
				a.assessmentDetail(c, p)
			})
		}).Grow(1)
	})
	a.zeroModal(c, p)
	a.resultDetailModal(c, p)
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
					row := listRowButton(c, a.assessmentOverviewLabel(item)).
						Height(0).MinHeight(t.Space(14)).Padding(t.Space(3), t.Space(3)).
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
	return fmt.Sprintf(locales.AssessmentOption(), item.CohortName, a.examTypeName(item.ExamType))
}

func (a *app) assessmentMissingItems(overview store.AssessmentOverview) []string {
	items := []struct {
		label string
		count int
	}{
		{locales.ExamTabRoster(), overview.RosterCount},
		{locales.ExamTabScores(), overview.ScoreCount},
		{locales.ExamTabCutoffs(), overview.CutoffCount},
		{locales.ExamTabTargets(), overview.TargetCount},
		{locales.ExamTabGroups(), overview.GroupCount},
	}
	missing := make([]string, 0, len(items))
	for _, item := range items {
		if item.count == 0 {
			missing = append(missing, item.label)
		}
	}
	return missing
}

func (a *app) assessmentReady(overview store.AssessmentOverview) bool {
	return len(a.assessmentMissingItems(overview)) == 0
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
	case !a.assessmentReady(*overview):
		stage = 0
	case overview.Status == "calculated" && !a.assessmentOfficialStale:
		stage = 2
	default:
		stage = 1
	}
	ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
				ui.Text(c, overview.ExamName).FontSize(titleSize(t)).FontWeight(600).
					LetterSpacing(-0.2).Role(ui.RoleHeading).Level(1)
				ui.Row(c).Gap(t.Space(1)).Children(func() {
					badge(c, p, a.examTypeName(overview.ExamType), toneAccent)
					badge(c, p, overview.CohortName, toneNeutral)
					badge(c, p, a.stageName(overview.GradeLevel), toneNeutral)
				})
				ui.Textf(c, locales.AssessmentDetailMeta(), a.academicYearLabel(overview.AcademicYearStart),
					a.examTermName(overview.Term), overview.ExamDate).FontSize(captionSize(t)).
					TextColor(p.muted).FontFeatures("tnum")
			})
			status, statusLabel := a.assessmentStatusDisplay(*overview)
			statusBadge(c, p, status, statusLabel)
		})
		a.assessmentSteps(c, p, stage)
		a.assessmentActions(c, p, *overview)
		if len(a.assessmentRows) == 0 {
			ui.Scroll(c).Grow(1).Shrink(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
					a.assessmentChecklist(c, p, *overview)
				})
			})
		} else {
			ui.Column(c).Grow(1).Shrink(1).MinHeight(0).Children(func() {
				a.assessmentResults(c, p)
			})
		}
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

// assessmentStatusDisplay 返回状态徽标的取值与文字；
// 已核算但数据已变化时显示「需要重新计算」。
func (a *app) assessmentStatusDisplay(overview store.AssessmentOverview) (string, string) {
	if overview.Status == "calculated" {
		if a.assessmentOfficialStale {
			return "stale", locales.AssessmentStatusStale()
		}
		return "calculated", locales.AssessmentStatusCalculated()
	}
	return "draft", locales.AssessmentStatusDraft()
}

// staleItemLabel 把输入摘要的数据项标识映射为界面名称。
func (a *app) staleItemLabel(key string) string {
	switch key {
	case "roster":
		return locales.ExamTabRoster()
	case "scores":
		return locales.ExamTabScores()
	case "cutoffs":
		return locales.ExamTabCutoffs()
	case "targets":
		return locales.ExamTabTargets()
	case "groups":
		return locales.ExamTabGroups()
	case "baseline":
		return locales.AssessmentItemBaseline()
	case "assignments":
		return locales.AssessmentItemAssignments()
	case "classes":
		return locales.AssessmentItemClasses()
	case "exclusions":
		return locales.AssessmentItemExclusions()
	default:
		return key
	}
}

// staleNoticeText 生成「需要重新计算」提示；
// 能定位到变化数据项时把名称列出来。
func (a *app) staleNoticeText() string {
	labels := make([]string, 0, len(a.assessmentOfficialStaleItems))
	for _, key := range a.assessmentOfficialStaleItems {
		labels = append(labels, a.staleItemLabel(key))
	}
	if len(labels) == 0 {
		return locales.AssessmentStaleNotice()
	}
	return fmt.Sprintf(locales.AssessmentStaleItems(), strings.Join(labels, locales.ListSeparator()))
}

// officialExportReady 判断能否导出：
// 存在最新正式核算，且它相对当前数据仍然新鲜。
func (a *app) officialExportReady() bool {
	return a.latestOfficialRun().ID != 0 && !a.assessmentOfficialStale
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
			row := listRowButton(c, item.Label).
				MinHeight(t.Space(9)).Padding(t.Space(2), t.Space(3)).
				Background(ui.Transparent).AlignItems(ui.Center)
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
				ui.Icon(c, iconArrowRight).FontSize(t.Space(4)).TextColor(p.muted)
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
		missing := a.assessmentMissingItems(overview)
		if len(missing) > 0 {
			notice(c, p, noticeWarning, fmt.Sprintf(locales.AssessmentPrepareRequired(),
				strings.Join(missing, locales.ListSeparator())))
		}
		if a.assessmentOfficialStale {
			notice(c, p, noticeWarning, a.staleNoticeText())
		}
		ui.Row(c).Wrap().AlignItems(ui.End).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Width(t.Space(56)).MinWidth(0).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.AssessmentPlanVersion()).FontSize(captionSize(t)).TextColor(p.muted)
				if len(a.published) == 0 {
					ui.Text(c, locales.BatchSelectVersion()).FontSize(bodySize(t)).TextColor(p.warning)
					return
				}
				selectField(c, "workbench-version", &a.calculateVersion, a.versionOptions(), locales.AssessmentPlanVersion()).
					Width(t.Space(56)).Label(locales.AssessmentPlanVersion())
			})
			ui.Spacer(c)
			ui.Row(c).Wrap().Gap(t.Space(2)).Children(func() {
				if labelledButton(c, fmt.Sprintf(locales.AssessmentFixExam(), overview.ExamName),
					iconExams, false).Clicked() {
					a.openExamTab(overview, 0)
				}
				trial := labelledButton(c, locales.AssessmentTrial(), iconCalculate, false)
				trial.Disabled(a.assessmentBusy || len(a.published) == 0 ||
					!a.assessmentReady(overview))
				if trial.Clicked() {
					a.startAssessmentTrial()
				}
				calculate := labelledButton(c, locales.AssessmentCalculate(), iconCalculate, true)
				calculate.Disabled(a.assessmentBusy || len(a.published) == 0 ||
					!a.assessmentReady(overview))
				if calculate.Clicked() {
					a.startAssessmentCalculate()
				}
				export := labelledButton(c, locales.AssessmentExport(), iconExport, false)
				export.Disabled(a.assessmentBusy || !a.officialExportReady())
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
	ui.Column(c).Grow(1).Shrink(1).MinHeight(0).Fill().Gap(t.Space(2)).Children(func() {
		a.assessmentResultsHeader(c, p)
		if len(a.assessmentRows) == 0 {
			ui.Text(c, locales.AssessmentNoResults()).FontSize(bodySize(t)).TextColor(p.muted)
			return
		}
		switch a.assessmentResultTab {
		case 1:
			a.assessmentClassResults(c, p)
		case 2:
			a.assessmentTeacherResults(c, p)
		default:
			a.assessmentSubjectResults(c, p)
		}
		a.consumeResultRowSelection()
	})
}

func (a *app) distinctAssessmentSubjects() []string {
	seen := map[string]bool{}
	order := []string{"chinese", "math", "english", "physics", "chemistry", "biology", "history", "geography", "politics"}
	present := map[string]bool{}
	for _, row := range a.assessmentRows {
		present[row.Subject] = true
	}
	var res []string
	for _, s := range order {
		if present[s] && !seen[s] {
			seen[s] = true
			res = append(res, s)
		}
	}
	for _, row := range a.assessmentRows {
		if !seen[row.Subject] {
			seen[row.Subject] = true
			res = append(res, row.Subject)
		}
	}
	return res
}

func (a *app) filteredAssessmentRows() []store.StoredResultRow {
	if a.assessmentSubjectFilter == "" || a.assessmentSubjectFilter == "all" {
		return a.assessmentRows
	}
	var res []store.StoredResultRow
	for _, row := range a.assessmentRows {
		if row.Subject == a.assessmentSubjectFilter {
			res = append(res, row)
		}
	}
	return res
}

func (a *app) assessmentSubjectResults(c *ui.Context, p uiPalette) {
	t := p.theme
	subjects := a.distinctAssessmentSubjects()
	if len(subjects) > 1 {
		options := []string{locales.AssessmentFilterAllSubjects()}
		for _, s := range subjects {
			options = append(options, locales.ColumnSubject()+": "+subjectLabel(s))
		}
		chosen := locales.AssessmentFilterAllSubjects()
		if a.assessmentSubjectFilter != "" && a.assessmentSubjectFilter != "all" {
			chosen = locales.ColumnSubject() + ": " + subjectLabel(a.assessmentSubjectFilter)
		}
		next := chosen
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			selectField(c, "workbench-subject-filter", &next, options, locales.AssessmentFilterAllSubjects()).
				Width(t.Space(40)).Label(locales.ColumnSubject())
		})
		if next != chosen {
			if next == locales.AssessmentFilterAllSubjects() {
				a.assessmentSubjectFilter = "all"
			} else {
				for _, s := range subjects {
					if locales.ColumnSubject()+": "+subjectLabel(s) == next {
						a.assessmentSubjectFilter = s
						break
					}
				}
			}
		}
	}
	rows := a.filteredAssessmentRows()
	a.resultTable.Key = func(row int) any {
		return fmt.Sprintf("%d:%s", rows[row].StatGroupID, rows[row].Subject)
	}
	a.resultTable.Selection = &a.resultSelection
	a.resultTable.RowHeight = t.Space(10)
	columns := []ui.TableColumn{
		{Title: locales.ColumnClass(), Width: t.Space(18)},
		{Title: locales.ColumnSubject(), Width: t.Space(20)},
		{Title: locales.ColumnStatGroup(), Width: t.Space(20)},
		{Title: locales.ColumnTeachers(), Width: 0},
		{Title: locales.ColumnTotal(), Width: t.Space(20), Align: ui.End},
		{Title: locales.ColumnRank(), Width: t.Space(18), Align: ui.End},
		{Title: locales.ColumnTier(), Width: t.Space(20)},
		{Title: locales.ColumnPoints(), Width: t.Space(18), Align: ui.End},
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
	}).Label(locales.AssessmentViewBySubject()).Grow(1).Shrink(1).MinHeight(0).Fill()
}

type classSummaryItem struct {
	ClassName       string
	CompletionScore float64
	HasCompletion   bool
	SubjectScores   map[string]float64
	SubjectTeachers map[string]string
	Total           float64
}

func (a *app) classSummaryData() ([]string, []classSummaryItem) {
	subjects := a.distinctAssessmentSubjects()
	classOrder := make([]string, 0)
	classMap := make(map[string]*classSummaryItem)
	for _, row := range a.assessmentRows {
		item, exists := classMap[row.LeadClassName]
		if !exists {
			item = &classSummaryItem{
				ClassName:       row.LeadClassName,
				SubjectScores:   make(map[string]float64),
				SubjectTeachers: make(map[string]string),
			}
			classMap[row.LeadClassName] = item
			classOrder = append(classOrder, row.LeadClassName)
		}
		if row.CompletionParticipate {
			item.CompletionScore = row.CompletionScore
			item.HasCompletion = true
		}
		item.SubjectScores[row.Subject] = row.Total
		item.SubjectTeachers[row.Subject] = decodeNames(row.TeacherNames)
	}
	items := make([]classSummaryItem, 0, len(classOrder))
	for _, name := range classOrder {
		item := *classMap[name]
		total := 0.0
		if item.HasCompletion {
			total += item.CompletionScore
		}
		for _, s := range subjects {
			total += item.SubjectScores[s]
		}
		item.Total = total
		items = append(items, item)
	}
	return subjects, items
}

func (a *app) assessmentClassResults(c *ui.Context, p uiPalette) {
	t := p.theme
	subjects, items := a.classSummaryData()
	if len(items) == 0 {
		smallEmptyState(c, p, locales.AssessmentNoResults())
		return
	}
	a.classResultTable.Key = func(row int) any {
		return items[row].ClassName
	}
	a.classResultTable.Selection = &a.classSelection
	a.classResultTable.RowHeight = t.Space(10)
	columns := []ui.TableColumn{
		{Title: locales.ColumnClass(), Width: t.Space(18)},
		{Title: locales.ColumnClassCompletionScore(), Width: t.Space(20), Align: ui.End},
	}
	for _, s := range subjects {
		columns = append(columns,
			ui.TableColumn{Title: subjectLabel(s), Width: t.Space(20), Align: ui.End},
			ui.TableColumn{Title: subjectLabel(s) + " " + locales.ColumnTeachers(), Width: t.Space(20)},
		)
	}
	columns = append(columns, ui.TableColumn{Title: locales.ColumnClassTotal(), Width: t.Space(20), Align: ui.End})
	rule(c)
	ui.Table(c, &a.classResultTable, columns, len(items), func(row, col int) {
		item := items[row]
		switch {
		case col == 0:
			ui.Text(c, item.ClassName).SingleLine()
		case col == 1:
			if item.HasCompletion {
				ui.Text(c, formatDecimal(item.CompletionScore, 1)).FontFeatures("tnum")
			} else {
				ui.Text(c, "-").TextColor(p.muted)
			}
		case col < len(columns)-1:
			subIdx := (col - 2) / 2
			isTeacher := (col-2)%2 == 1
			s := subjects[subIdx]
			if isTeacher {
				ui.Text(c, item.SubjectTeachers[s]).SingleLine()
			} else {
				if score, ok := item.SubjectScores[s]; ok {
					ui.Text(c, formatDecimal(score, 1)).FontFeatures("tnum")
				} else {
					ui.Text(c, "-").TextColor(p.muted)
				}
			}
		case col == len(columns)-1:
			ui.Text(c, formatDecimal(item.Total, 1)).FontFeatures("tnum")
		}
	}).Label(locales.AssessmentViewByClass()).Grow(1).Shrink(1).MinHeight(0).Fill()
}

func (a *app) assessmentTeacherResults(c *ui.Context, p uiPalette) {
	t := p.theme
	scores := a.assessmentPersonal
	if len(scores) == 0 {
		smallEmptyState(c, p, locales.AssessmentNoResults())
		return
	}
	a.teacherResultTable.Key = func(row int) any {
		return fmt.Sprintf("%s:%s", scores[row].TeacherName, scores[row].Subject)
	}
	a.teacherResultTable.Selection = &a.teacherSelection
	a.teacherResultTable.RowHeight = t.Space(10)
	columns := []ui.TableColumn{
		{Title: locales.ColumnTeacherName(), Width: t.Space(28)},
		{Title: locales.ColumnGrade(), Width: t.Space(18)},
		{Title: locales.ColumnSubject(), Width: t.Space(20)},
		{Title: locales.ColumnGroupPoints(), Width: 0},
		{Title: locales.ColumnPersonalScore(), Width: t.Space(24), Align: ui.End},
	}
	rule(c)
	ui.Table(c, &a.teacherResultTable, columns, len(scores), func(row, col int) {
		item := scores[row]
		switch col {
		case 0:
			ui.Text(c, item.TeacherName).SingleLine()
		case 1:
			ui.Text(c, gradeLabel(item.Grade)).SingleLine()
		case 2:
			ui.Text(c, subjectLabel(item.Subject)).SingleLine()
		case 3:
			ui.Text(c, formatPoints(item.GroupPoints)).SingleLine()
		case 4:
			ui.Text(c, formatDecimal(item.Score, 2)).FontFeatures("tnum")
		}
	}).Label(locales.AssessmentViewByTeacher()).Grow(1).Shrink(1).MinHeight(0).Fill()
}

func gradeLabel(grade string) string {
	switch grade {
	case "g1":
		return locales.GradeG1()
	case "g2":
		return locales.GradeG2()
	case "g3":
		return locales.GradeG3()
	default:
		return grade
	}
}

func formatPoints(value string) string {
	var points []float64
	if err := json.Unmarshal([]byte(value), &points); err != nil {
		return value
	}
	parts := make([]string, 0, len(points))
	for _, point := range points {
		parts = append(parts, strconv.FormatFloat(point, 'f', -1, 64))
	}
	return strings.Join(parts, locales.ListSeparator())
}

func (a *app) assessmentResultsHeader(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		sectionHeading(c, p, locales.AssessmentResults(), len(a.assessmentRows))
		if a.assessmentRun.ID != 0 {
			kind := locales.AssessmentTrialResults()
			tone := toneNeutral
			if a.assessmentRun.IsOfficial {
				kind = locales.AssessmentOfficialResults()
				tone = toneAccent
			}
			badge(c, p, kind, tone)
		}
		if len(a.assessmentRuns) > 1 {
			chosen := a.runOptionLabelForID(a.assessmentRunID)
			next := chosen
			selectField(c, "workbench-runs", &next, a.runOptions(), locales.AssessmentRunSelect()).
				Width(t.Space(40)).Label(locales.AssessmentRunSelect())
			if next != chosen {
				if id, ok := a.runIDForLabel(next); ok {
					a.selectAssessmentRun(id)
				}
			}
		}
		ui.Spacer(c)
		segmentedField(c, &a.assessmentResultTab, []string{
			locales.AssessmentViewBySubject(),
			locales.AssessmentViewByClass(),
			locales.AssessmentViewByTeacher(),
		}, locales.AssessmentViewMode())
		if a.assessmentRun.ID != 0 {
			ui.Textf(c, locales.AssessmentRunMeta(), formatTime(a.assessmentRun.CreatedAt)).
				FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
		}
	})
}

func (a *app) runOptionLabelForID(id int64) string {
	for _, run := range a.assessmentRuns {
		if run.ID == id {
			return a.runOptionLabel(run)
		}
	}
	return ""
}

// consumeResultRowSelection 消费行点击产生的选中：
// 选中在下一帧出现，这里打开对应行的详情并清空选中。
func (a *app) consumeResultRowSelection() {
	if a.resultDetailOpen {
		return
	}
	if a.resultSelection.Len() > 0 {
		for key := range a.resultSelection.All() {
			row := a.resultRowByKey(key)
			if row != nil {
				a.openResultDetail(*row)
			}
			break
		}
		a.resultSelection.Clear()
		return
	}
	if a.classSelection.Len() > 0 {
		for className := range a.classSelection.All() {
			for _, row := range a.assessmentRows {
				if row.LeadClassName == className {
					a.openResultDetail(row)
					break
				}
			}
			break
		}
		a.classSelection.Clear()
		return
	}
	if a.teacherSelection.Len() > 0 {
		for key := range a.teacherSelection.All() {
			teacherName, subject, _ := strings.Cut(key, ":")
			for _, row := range a.assessmentRows {
				if (subject == "" || row.Subject == subject) && strings.Contains(decodeNames(row.TeacherNames), teacherName) {
					a.openResultDetail(row)
					break
				}
			}
			break
		}
		a.teacherSelection.Clear()
		return
	}
}

func (a *app) resultRowByKey(key string) *store.StoredResultRow {
	idText, subject, ok := strings.Cut(key, ":")
	if !ok {
		return nil
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return nil
	}
	for index := range a.assessmentRows {
		row := &a.assessmentRows[index]
		if row.StatGroupID == id && row.Subject == subject {
			return row
		}
	}
	return nil
}

// resultDetail 是一条结果行详情弹窗的展示数据，
// 打开时同步查好，弹窗构建不再访问存储。
type resultDetail struct {
	Row       store.StoredResultRow
	PlanLabel string
	Formulas  map[string]string
	Group     store.ExamStatGroup
	Students  []store.ResultRowStudent
}

func (a *app) closeResultDetail() {
	a.resultDetailOpen = false
	a.resultDetail = resultDetail{}
	a.resultSelection.Clear()
	a.classSelection.Clear()
	a.teacherSelection.Clear()
}

// openResultDetail 为一条结果行收集详情并打开弹窗。
func (a *app) openResultDetail(row store.StoredResultRow) {
	a.resultSelection.Clear()
	a.classSelection.Clear()
	a.teacherSelection.Clear()
	if a.store == nil || a.assessmentRun.ID == 0 {
		return
	}
	ctx := context.Background()
	version, err := a.store.GetVersion(ctx, a.assessmentRun.PlanVersionID)
	if err != nil {
		a.assessmentError = err.Error()
		return
	}
	groups, err := a.store.ListExamStatGroups(ctx, a.selectedAssessmentID)
	if err != nil {
		a.assessmentError = err.Error()
		return
	}
	var group store.ExamStatGroup
	for _, item := range groups {
		if item.ID == row.StatGroupID {
			group = item
			break
		}
	}
	students, err := a.store.AssessmentRowStudents(ctx, a.assessmentRun.ID, row.StatGroupID, row.Subject)
	if err != nil {
		a.assessmentError = err.Error()
		return
	}
	a.resultDetail = resultDetail{
		Row:       row,
		PlanLabel: a.labelForVersionID(version.ID),
		Formulas:  assessment.EffectiveFormulas(version.Config, a.assessmentContext.Exam.ExamType, a.assessmentContext.Grade),
		Group:     group,
		Students:  students,
	}
	a.resultDetailOpen = true
}

// startAssessmentTrial 用当前考核数据试算：
// 结果按非正式保存，不改变考核状态与正式结果。
func (a *app) startAssessmentTrial() {
	a.runAssessmentCalculate(true)
}

func (a *app) startAssessmentCalculate() {
	a.runAssessmentCalculate(false)
}

func (a *app) runAssessmentCalculate(trial bool) {
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
	if trial {
		a.zeroMode = "trial"
	} else {
		a.zeroMode = "official"
	}
	work := func() (calc.Result, error) {
		ctx := context.Background()
		if trial {
			return pipeline.TrialAssessment(ctx, a.store, assessmentID, version.Version.ID)
		}
		return pipeline.CalculateAssessment(ctx, a.store, assessmentID, version.Version.ID)
	}
	run := func() {
		result, err := work()
		if a.win == nil {
			a.finishAssessmentCalculate(result, err, trial)
			return
		}
		a.win.Update(func() {
			a.finishAssessmentCalculate(result, err, trial)
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *app) finishAssessmentCalculate(result calc.Result, err error, trial bool) {
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
	if trial {
		a.assessmentMessage = fmt.Sprintf(locales.AssessmentTrialCalculated(), len(result.Rows), len(result.Personal))
	} else {
		a.assessmentMessage = fmt.Sprintf(locales.AssessmentCalculated(), len(result.Rows), len(result.Personal))
	}
	a.assessmentRunChosen = false
	a.reloadAssessments(context.Background())
}

func (a *app) chooseAssessmentExport() {
	name := "Tally.xlsx"
	if overview := a.selectedAssessmentOverview(); overview != nil {
		name = overview.CohortName + a.examTypeName(overview.ExamType) + ".xlsx"
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
	if modal(c, &open, modalProps{
		Title:      locales.ZeroTitle(),
		OkText:     locales.ZeroFill(),
		CancelText: locales.ZeroClose(),
		Width:      modalWidthWide,
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
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
			ui.Text(c, issue.ClassName).FontSize(bodySize(t)).FontWeight(600).Grow(1)
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
					Width(t.Space(36)).Label(name)
			})
		}
	})
}

type targetAdjustment struct {
	classID int64
	tier    string
	value   float64
}

// detailOperand 是指标明细里的一行操作数。
type detailOperand struct {
	Label string
	Value string
}

// resultDetailModal 是一条结果行的详情弹窗：
// 指标明细、计算依据、合并组与参与学生。
func (a *app) resultDetailModal(c *ui.Context, p uiPalette) {
	if !a.resultDetailOpen {
		return
	}
	t := p.theme
	detail := a.resultDetail
	open := true
	modal(c, &open, modalProps{
		Title:    locales.AssessmentDetailTitle(),
		Width:    modalWidthWide,
		NoFooter: true,
	}, func() {
		row := detail.Row
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			ui.Row(c).Gap(t.Space(1)).Children(func() {
				badge(c, p, row.LeadClassName, toneNeutral)
				badge(c, p, subjectLabel(row.Subject), toneNeutral)
				badge(c, p, row.StatGroupName, toneNeutral)
			})
			sectionHeading(c, p, locales.AssessmentDetailIndicators(), -1)
			rule(c)
			a.detailIndicator(c, p, locales.PlanIndicatorImprovement(), detail.Formulas["improvement"],
				[]detailOperand{
					{Label: locales.AssessmentDetailClassAverage(), Value: formatDecimal(row.ClassAverage, 1)},
					{Label: locales.AssessmentDetailGradeAverage(), Value: formatDecimal(row.GradeAverage, 1)},
					{Label: locales.AssessmentDetailBaselineRatio(), Value: formatRatio(row.BaselineRatio)},
				},
				formatDecimal(row.ImprovementRaw, 1), formatDecimal(row.ImprovementScore, 1))
			a.detailIndicator(c, p, locales.PlanIndicatorPass(), detail.Formulas["pass"],
				[]detailOperand{
					{Label: locales.AssessmentDetailPassCount(), Value: formatInt(row.PassCount)},
					{Label: locales.AssessmentDetailPassTarget(), Value: formatDecimal(row.PassTarget, 0)},
				},
				formatDecimal(row.PassRaw, 1), formatDecimal(row.PassScore, 1))
			if row.CompletionParticipate {
				a.detailIndicator(c, p, locales.PlanIndicatorCompletion(), detail.Formulas["completion"],
					[]detailOperand{
						{Label: locales.AssessmentDetailCompletionActual(), Value: formatDecimal(row.CompletionActual, 1)},
						{Label: locales.AssessmentDetailCompletionBase(), Value: formatDecimal(row.CompletionBase, 1)},
					},
					formatDecimal(row.CompletionRaw, 1), formatDecimal(row.CompletionScore, 1))
			} else {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, locales.PlanIndicatorCompletion()).FontSize(bodySize(t))
					ui.Spacer(c)
					ui.Text(c, locales.AssessmentDetailNoCompletion()).FontSize(bodySize(t)).TextColor(p.muted)
				})
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.AssessmentDetailBasis()).FontSize(captionSize(t)).TextColor(p.muted)
				ui.Spacer(c)
				ui.Text(c, detail.PlanLabel).FontSize(bodySize(t)).SingleLine()
			})
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.AssessmentDetailGroup()).FontSize(captionSize(t)).TextColor(p.muted)
				ui.Spacer(c)
				ui.Text(c, a.detailGroupMembers(detail)).FontSize(bodySize(t)).SingleLine()
			})
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.AssessmentDetailLeadClass()).FontSize(captionSize(t)).TextColor(p.muted)
				ui.Spacer(c)
				ui.Text(c, row.LeadClassName).FontSize(bodySize(t)).SingleLine()
			})
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.AssessmentDetailTeachers()).FontSize(captionSize(t)).TextColor(p.muted)
				ui.Spacer(c)
				ui.Text(c, decodeNames(row.TeacherNames)).FontSize(bodySize(t)).SingleLine()
			})
			sectionHeading(c, p, locales.AssessmentDetailStudents(), len(detail.Students))
			rule(c)
			ui.Scroll(c).MaxHeight(t.Space(48)).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
					for _, student := range detail.Students {
						student := student
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Text(c, student.StudentNo).FontSize(captionSize(t)).TextColor(p.muted).
								Width(t.Space(14)).FontFeatures("tnum")
							ui.Text(c, student.Name).FontSize(bodySize(t)).SingleLine()
						})
					}
				})
			})
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if plainButton(c, locales.AssessmentDetailClose()).Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		a.closeResultDetail()
	}
}

// detailIndicator 渲染一个指标的公式、操作数、未封顶值与得分。
func (a *app) detailIndicator(c *ui.Context, p uiPalette, name, formulaText string,
	operands []detailOperand, raw, score string) {
	t := p.theme
	panel(c, p).Background(p.panelMuted).Padding(t.Space(3)).Gap(t.Space(1.5)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, name).FontSize(bodySize(t)).FontWeight(600).Grow(1).SingleLine()
			ui.Text(c, locales.AssessmentDetailUncapped()).FontSize(captionSize(t)).TextColor(p.muted)
			ui.Text(c, raw).FontSize(bodySize(t)).FontFeatures("tnum")
			ui.Text(c, locales.AssessmentDetailScore()).FontSize(captionSize(t)).TextColor(p.muted)
			ui.Text(c, score).FontSize(bodySize(t)).FontWeight(600).FontFeatures("tnum")
		})
		if formulaText != "" {
			ui.Text(c, formulaText).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
		}
		for _, operand := range operands {
			operand := operand
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Text(c, operand.Label).FontSize(captionSize(t)).TextColor(p.muted).Grow(1).SingleLine()
				ui.Text(c, operand.Value).FontSize(captionSize(t)).FontFeatures("tnum")
			})
		}
	})
}

// detailGroupMembers 返回合并组名称与组内教学班名称。
func (a *app) detailGroupMembers(detail resultDetail) string {
	if detail.Group.ID == 0 {
		return detail.Row.StatGroupName
	}
	names := make([]string, 0, len(detail.Group.Members))
	for _, member := range detail.Group.Members {
		names = append(names, member.Name)
	}
	if len(names) == 0 {
		return detail.Group.Name
	}
	return detail.Group.Name + " (" + strings.Join(names, locales.ListSeparator()) + ")"
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
			if a.zeroMode == "trial" {
				result, err = pipeline.TrialAssessment(ctx, a.store, assessmentID, version.Version.ID)
			} else {
				result, err = pipeline.CalculateAssessment(ctx, a.store, assessmentID, version.Version.ID)
			}
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
	if a.zeroMode == "trial" {
		a.assessmentMessage = fmt.Sprintf(locales.AssessmentTrialCalculated(), len(result.Rows), len(result.Personal))
	} else {
		a.assessmentMessage = fmt.Sprintf(locales.AssessmentCalculated(), len(result.Rows), len(result.Personal))
	}
	a.assessmentRunChosen = false
	a.reloadAssessments(context.Background())
}

func (a *app) resultsPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavResults(), locales.ResultsSubtitle(), func() {
			export := labelledButton(c, locales.AssessmentExport(), iconExport, false)
			export.Disabled(a.assessmentBusy || !a.officialExportReady())
			if export.Clicked() {
				a.chooseAssessmentExport()
			}
		})
		if a.assessmentError != "" {
			notice(c, p, noticeDanger, a.assessmentError)
		}
		if a.assessmentOfficialStale {
			notice(c, p, noticeWarning, a.staleNoticeText())
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
		selectField(c, "workbench-results", &next, labels, locales.AssessmentResultsSelect()).Width(t.Space(72))
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
	a.resultDetailModal(c, p)
}
