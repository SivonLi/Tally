package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

type qualityExamOverview struct {
	ExamID     int64
	ExamLabel  string
	CohortID   int64
	CohortName string
	GradeLevel string
	Issues     []store.ExamQualityIssue
}

type qualityGroup struct {
	Key    string
	Issues []store.ExamQualityIssue
}

func (a *app) reloadQuality(ctx context.Context) {
	if a.store == nil {
		return
	}
	summaries, err := a.store.ListExamQualitySummaries(ctx)
	if err != nil {
		a.qualityError = err.Error()
		return
	}
	overviews := make([]qualityExamOverview, 0, len(summaries))
	for _, summary := range summaries {
		overviews = append(overviews, qualityExamOverview{
			ExamID: summary.ExamID, ExamLabel: summary.ExamName, CohortID: summary.CohortID,
			CohortName: summary.CohortName, GradeLevel: summary.GradeLevel, Issues: summary.Issues,
		})
	}
	runs, err := a.store.ListAllImportRuns(ctx)
	if err != nil {
		a.qualityError = err.Error()
		return
	}
	a.qualityImports = map[int64]string{}
	for _, run := range runs {
		a.qualityImports[run.ID] = importFileLabel(run)
	}
	a.qualityExams = overviews
	a.qualityLoaded = true
	a.qualityError = ""
}

func (a *app) qualityPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.store == nil {
		return
	}
	if !a.qualityLoaded {
		a.reloadQuality(context.Background())
	}
	visible := a.visibleQuality()
	blocking, warnings := 0, 0
	for _, overview := range visible {
		for _, issue := range overview.Issues {
			if issue.Severity == "blocking" {
				blocking++
			} else {
				warnings++
			}
		}
	}
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavQuality(), locales.QualityPageSubtitle(), func() {
			if plainButton(c, locales.ExamRevalidate()).Clicked() {
				a.reloadQuality(context.Background())
			}
		})
		if a.qualityError != "" {
			notice(c, p, noticeDanger, a.qualityError)
		}
		ui.Row(c).AlignItems(ui.End).Wrap().Gap(t.Space(2)).Children(func() {
			segmentedField(c, &a.qualitySeverity,
				[]string{locales.ExamQualityAll(), locales.ExamQualityBlocking(),
					locales.ExamQualityWarnings()}, locales.ExamQualitySeverity())
			a.qualityItemFilter(c, p)
			a.qualityImportFilter(c, p)
			ui.Spacer(c)
			badge(c, p, fmt.Sprintf(locales.ExamQualityBlockingCount(), blocking), toneDanger)
			badge(c, p, fmt.Sprintf(locales.ExamQualityWarningCount(), warnings), toneWarning)
		})
		if len(visible) == 0 {
			emptyState(c, p, iconQuality, locales.ExamQualityEmpty(), locales.ExamQualityEmptyHint(), nil)
			return
		}
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
				for _, overview := range visible {
					overview := overview
					ui.Column(c).FillWidth().Gap(t.Space(2)).Padding(t.Space(1), 0).
						Children(func() {
							rule(c)
							row := linkRow(c, p, overview.ExamLabel, func() {
								ui.Column(c).Grow(1).Gap(0).Children(func() {
									ui.Text(c, overview.ExamLabel).FontSize(bodySize(t)).FontWeight(600)
									ui.Textf(c, locales.ExamQualityCohortMeta(), overview.CohortName,
										a.stageName(overview.GradeLevel)).FontSize(captionSize(t)).
										TextColor(p.muted)
								})
								if plainButton(c, locales.View()).Clicked() {
									a.openQualityExam(overview)
								}
							})
							if row.Clicked() {
								a.openQualityExam(overview)
							}
							for _, group := range qualityIssueGroups(overview.Issues) {
								group := group
								sectionHeading(c, p, a.qualityDataItemName(group.Key), len(group.Issues))
								for _, issue := range group.Issues {
									issue := issue
									a.qualityIssueRow(c, p, issue, func() {
										a.openQualityIssue(overview, issue)
									})
								}
							}
						})
				}
			})
		})
	})
}

func (a *app) qualityItemFilter(c *ui.Context, p uiPalette) {
	t := p.theme
	keys := []string{"", "roster", "scores", "cutoffs", "targets", "groups"}
	labels := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			labels = append(labels, locales.ExamQualityAllItems())
			continue
		}
		labels = append(labels, a.qualityDataItemName(key))
	}
	chosen := labels[0]
	for index, key := range keys {
		if key == a.qualityDataItem {
			chosen = labels[index]
		}
	}
	next := chosen
	selectField(c, "quality-data-item", &next, labels, locales.ExamQualityDataItem()).Width(t.Space(32))
	if next != chosen {
		for index, label := range labels {
			if label == next {
				a.qualityDataItem = keys[index]
				break
			}
		}
	}
}

func (a *app) qualityImportFilter(c *ui.Context, p uiPalette) {
	t := p.theme
	ids := []int64{0}
	for id := range a.qualityImports {
		ids = append(ids, id)
	}
	sort.Slice(ids[1:], func(i, j int) bool { return ids[i+1] < ids[j+1] })
	labels := []string{locales.ExamQualityAllImports()}
	for _, id := range ids[1:] {
		labels = append(labels, a.qualityImports[id])
	}
	chosen := labels[0]
	for index, id := range ids {
		if id == a.qualityImportRun {
			chosen = labels[index]
		}
	}
	next := chosen
	selectField(c, "quality-import", &next, labels, locales.ExamQualityImport()).Width(t.Space(40))
	if next != chosen {
		for index, label := range labels {
			if label == next {
				a.qualityImportRun = ids[index]
				break
			}
		}
	}
}

func (a *app) qualityIssueRow(c *ui.Context, p uiPalette, issue store.ExamQualityIssue, open func()) {
	t := p.theme
	tone := toneWarning
	if issue.Severity == "blocking" {
		tone = toneDanger
	}
	label := a.examQualityText(issue)
	element := textButton(c, label).FillWidth().
		Padding(t.Space(1.5), t.Space(2)).Radius(t.Space(1.5)).
		Background(ui.Transparent)
	element.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(t.Space(2)).Children(func() {
			badge(c, p, a.examSeverityName(issue.Severity), tone)
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Text(c, label).FontSize(bodySize(t)).Pretty()
				if meta := a.qualityIssueMeta(issue); meta != "" {
					ui.Text(c, meta).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
				}
			})
			ui.Icon(c, iconArrowRight).FontSize(t.Space(3.5)).TextColor(p.muted)
		})
	})
	if element.Clicked() {
		open()
	}
}

func (a *app) qualityIssueMeta(issue store.ExamQualityIssue) string {
	parts := make([]string, 0, 4)
	switch issue.Code {
	case "missing_track", "missing_scores", "score_out_of_range":
		// 学号已在标签里，meta 不再重复。
	case "zero_class_target":
		// 班级名已在标签里，meta 不再重复。
	default:
		if issue.StudentNo != "" {
			parts = append(parts, issue.StudentNo)
		}
		if issue.ClassName != "" {
			parts = append(parts, issue.ClassName)
		}
	}
	if issue.ImportRunID != 0 {
		if label := a.qualityImports[issue.ImportRunID]; label != "" {
			parts = append(parts, label)
		}
	}
	if issue.Row > 0 {
		parts = append(parts, fmt.Sprintf(locales.ExamQualityRow(), issue.Row))
	}
	result := ""
	for index, part := range parts {
		if index > 0 {
			result += locales.ListSeparator()
		}
		result += part
	}
	return result
}

func qualityIssueGroups(issues []store.ExamQualityIssue) []qualityGroup {
	order := []string{"roster", "scores", "cutoffs", "targets", "groups"}
	grouped := map[string][]store.ExamQualityIssue{}
	for _, issue := range issues {
		key := qualityIssueKey(issue)
		grouped[key] = append(grouped[key], issue)
	}
	result := make([]qualityGroup, 0, len(order))
	for _, key := range order {
		if len(grouped[key]) == 0 {
			continue
		}
		result = append(result, qualityGroup{Key: key, Issues: grouped[key]})
	}
	return result
}

func qualityIssueKey(issue store.ExamQualityIssue) string {
	switch issue.Code {
	case "no_roster", "missing_track", "missing_scores":
		return "roster"
	case "score_out_of_range", "score_without_roster":
		return "scores"
	case "missing_cutoff":
		return "cutoffs"
	case "zero_class_target":
		return "targets"
	case "empty_stat_group", "zero_stat_group_target":
		return "groups"
	default:
		return "scores"
	}
}

func (a *app) qualityDataItemName(key string) string {
	switch key {
	case "roster":
		return locales.ExamKindRoster()
	case "scores":
		return locales.ExamKindScores()
	case "cutoffs":
		return locales.ExamKindCutoffs()
	case "targets":
		return locales.ExamKindTargets()
	case "groups":
		return locales.ExamTabGroups()
	default:
		return key
	}
}

func (a *app) openQualityExam(overview qualityExamOverview) {
	if a.store == nil {
		return
	}
	a.page = pageExams
	a.selectExam(overview.ExamID)
	a.examCohortID = overview.CohortID
	a.examTab = 5
	a.reloadExamData(context.Background())
}

func (a *app) openQualityIssue(overview qualityExamOverview, issue store.ExamQualityIssue) {
	if a.store == nil {
		return
	}
	a.page = pageExams
	a.selectExam(overview.ExamID)
	a.examCohortID = overview.CohortID
	a.examTab = qualityIssueTab(issue.Code)
	if issue.StudentNo != "" {
		a.examRosterQuery = issue.StudentNo
		a.examScoreQuery = issue.StudentNo
	}
	a.reloadExamData(context.Background())
}

func qualityIssueTab(code string) int {
	switch code {
	case "missing_scores", "score_out_of_range", "score_without_roster":
		return 1
	case "missing_cutoff":
		return 2
	case "zero_class_target":
		return 3
	case "empty_stat_group", "zero_stat_group_target":
		return 4
	default:
		return 0
	}
}

func (a *app) visibleQuality() []qualityExamOverview {
	severity := ""
	switch a.qualitySeverity {
	case 1:
		severity = "blocking"
	case 2:
		severity = "warning"
	}
	result := make([]qualityExamOverview, 0, len(a.qualityExams))
	for _, overview := range a.qualityExams {
		issues := make([]store.ExamQualityIssue, 0, len(overview.Issues))
		for _, issue := range overview.Issues {
			if severity != "" && issue.Severity != severity {
				continue
			}
			if a.qualityDataItem != "" && qualityIssueKey(issue) != a.qualityDataItem {
				continue
			}
			if a.qualityImportRun != 0 && issue.ImportRunID != a.qualityImportRun {
				continue
			}
			issues = append(issues, issue)
		}
		if len(issues) == 0 {
			continue
		}
		overview.Issues = issues
		result = append(result, overview)
	}
	return result
}
