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
	var (
		summaries []store.ExamQualitySummary
		runs      []store.ImportRun
		err       error
	)
	summaries, err = a.store.ListExamQualitySummaries(ctx)
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
	runs, err = a.store.ListAllImportRuns(ctx)
	if err != nil {
		a.qualityError = err.Error()
		return
	}
	a.qualityImports = map[int64]string{}
	for _, run := range runs {
		a.qualityImports[run.ID] = importFileLabel(run)
	}
	a.qualityImportOptions = a.qualityImportFilterOptions(overviews, runs)
	if a.qualityImportRun != 0 && a.qualityImportOptions[a.qualityImportRun] == "" {
		a.qualityImportRun = 0
	}
	a.qualityExams = overviews
	a.qualityLoaded = true
	a.qualityError = ""
}

// qualityImportFilterOptions 构造数据质量页的导入记录选项：
// 只列质量问题实际引用的记录，文件名重复时按数据项区分；
// 没有任何问题引用导入记录时返回 nil，页面上隐藏这个筛选。
func (a *app) qualityImportFilterOptions(overviews []qualityExamOverview, runs []store.ImportRun) map[int64]string {
	byID := make(map[int64]store.ImportRun, len(runs))
	for _, run := range runs {
		byID[run.ID] = run
	}
	referenced := map[int64]bool{}
	for _, overview := range overviews {
		for _, issue := range overview.Issues {
			if issue.ImportRunID > 0 {
				referenced[issue.ImportRunID] = true
			}
		}
	}
	if len(referenced) == 0 {
		return nil
	}
	labels := make(map[int64]string, len(referenced))
	counts := map[string]int{}
	for id := range referenced {
		labels[id] = importFileLabel(byID[id])
		counts[labels[id]]++
	}
	for id, label := range labels {
		if counts[label] > 1 {
			labels[id] = fmt.Sprintf(locales.LabelQualifier(), label, importKindName(byID[id].DataKind))
		}
	}
	return labels
}

type qualityListRowKind int

const (
	qualityRowExam qualityListRowKind = iota
	qualityRowGroup
	qualityRowIssue
)

type qualityListRow struct {
	Kind     qualityListRowKind
	Key      string
	Overview qualityExamOverview
	GroupKey string
	Count    int
	Issue    store.ExamQualityIssue
}

func buildQualityRows(visible []qualityExamOverview) []qualityListRow {
	rows := make([]qualityListRow, 0, len(visible)*4)
	for _, overview := range visible {
		rows = append(rows, qualityListRow{
			Kind:     qualityRowExam,
			Key:      fmt.Sprintf("exam:%d:%d", overview.ExamID, overview.CohortID),
			Overview: overview,
		})
		for _, group := range qualityIssueGroups(overview.Issues) {
			rows = append(rows, qualityListRow{
				Kind:     qualityRowGroup,
				Key:      fmt.Sprintf("group:%d:%d:%s", overview.ExamID, overview.CohortID, group.Key),
				Overview: overview,
				GroupKey: group.Key,
				Count:    len(group.Issues),
			})
			for idx, issue := range group.Issues {
				rows = append(rows, qualityListRow{
					Kind:     qualityRowIssue,
					Key:      fmt.Sprintf("issue:%d:%d:%s:%d:%s:%d", overview.ExamID, overview.CohortID, group.Key, idx, issue.Code, issue.Row),
					Overview: overview,
					Issue:    issue,
				})
			}
		}
	}
	return rows
}

func (a *app) qualityPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.store == nil {
		errorState(c, p, locales.StoreUnavailable(), locales.StoreUnavailableHint(), func() {
			a.loadError = ""
			a.load()
		}, func() {
			a.openSettings()
		})
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
		a.examHubSubnav(c, p, 2)
		pageHeading(c, p, locales.NavQuality(), locales.QualityPageSubtitle(), func() {
			if plainButton(c, locales.ExamRevalidate()).Clicked() {
				a.reloadQuality(context.Background())
				if a.qualityError == "" {
					a.notifyMain(locales.QualityRevalidatedToast())
				}
			}
		})
		if a.qualityError != "" {
			notice(c, p, noticeDanger, a.qualityError)
		}
		ui.Row(c).FillWidth().AlignItems(ui.End).Wrap().Gap(t.Space(2)).Children(func() {
			segmentedField(c, &a.qualitySeverity,
				[]string{locales.ExamQualityAll(), locales.ExamQualityBlocking(),
					locales.ExamQualityWarnings()}, locales.ExamQualitySeverity())
			a.qualityItemFilter(c, p)
			if a.qualityImportOptions != nil {
				a.qualityImportFilter(c, p)
			}
			ui.Spacer(c)
			badge(c, p, fmt.Sprintf(locales.ExamQualityBlockingCount(), blocking), toneDanger)
			badge(c, p, fmt.Sprintf(locales.ExamQualityWarningCount(), warnings), toneWarning)
		})
		if len(visible) == 0 {
			if a.qualitySeverity != 0 || a.qualityDataItem != "" || a.qualityImportRun != 0 {
				emptyState(c, p, iconQuality, locales.FilterEmptyTitle(), locales.FilterEmptyHint(), func() {
					if plainButton(c, locales.ClearFilters()).Clicked() {
						a.qualitySeverity = 0
						a.qualityDataItem = ""
						a.qualityImportRun = 0
					}
				})
				return
			}
			emptyState(c, p, iconQuality, locales.ExamQualityEmpty(), locales.ExamQualityEmptyHint(), nil)
			return
		}
		rows := buildQualityRows(visible)
		var openExam *qualityExamOverview
		var openIssueExam *qualityExamOverview
		var openIssue *store.ExamQualityIssue
		var fixExam *qualityExamOverview
		var fixIssue *store.ExamQualityIssue
		a.qualityListState.Key = func(row int) any { return rows[row].Key }
		ui.List(c, &a.qualityListState, len(rows), func(row int) {
			entry := rows[row]
			switch entry.Kind {
			case qualityRowExam:
				overview := entry.Overview
				ui.Column(c).FillWidth().Gap(t.Space(1)).Padding(t.Space(1), 0).Children(func() {
					rule(c)
					btn := linkRow(c, p, overview.ExamLabel, func() {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(0).Children(func() {
							ui.Text(c, overview.ExamLabel).FontSize(bodySize(t)).FontWeight(600).
								SingleLine().MinWidth(0).Shrink(1).Tooltip(overview.ExamLabel)
							ui.Textf(c, locales.ExamQualityCohortMeta(), overview.CohortName,
								a.stageName(overview.GradeLevel)).FontSize(captionSize(t)).
								TextColor(p.muted).SingleLine().MinWidth(0).Shrink(1)
						})
						ui.Text(c, locales.View()).FontSize(captionSize(t)).FontWeight(500).TextColor(p.ink).Shrink(0)
						ui.Icon(c, iconArrowRight).FontSize(t.Space(4)).TextColor(p.muted).Shrink(0)
					})
					if btn.Clicked() {
						copied := overview
						openExam = &copied
					}
				})
			case qualityRowGroup:
				ui.Column(c).FillWidth().Padding(t.Space(2), 0, t.Space(1), 0).Children(func() {
					sectionHeading(c, p, a.qualityDataItemName(entry.GroupKey), entry.Count)
				})
			case qualityRowIssue:
				overview := entry.Overview
				issue := entry.Issue
				var onFix func()
				if qualityIssueInlineFixable(issue) {
					onFix = func() {
						copiedOverview := overview
						copiedIssue := issue
						fixExam = &copiedOverview
						fixIssue = &copiedIssue
					}
				}
				a.qualityIssueRow(c, p, issue, func() {
					copiedOverview := overview
					copiedIssue := issue
					openIssueExam = &copiedOverview
					openIssue = &copiedIssue
				}, onFix)
			}
		}).Label(locales.NavQuality()).Grow(1).Shrink(1).MinHeight(0).Fill()
		if fixExam != nil && fixIssue != nil {
			a.openQualityFix(*fixExam, *fixIssue)
		} else if openExam != nil {
			a.openQualityExam(*openExam)
		} else if openIssueExam != nil && openIssue != nil {
			a.openQualityIssue(*openIssueExam, *openIssue)
		}
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
	selectField(c, "quality-data-item", &next, labels, locales.ExamQualityDataItem()).Width(t.Space(40))
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
	ids := make([]int64, 0, len(a.qualityImportOptions))
	for id := range a.qualityImportOptions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	labels := []string{locales.ExamQualityAllImports()}
	for _, id := range ids {
		labels = append(labels, a.qualityImportOptions[id])
	}
	chosen := locales.ExamQualityAllImports()
	for _, id := range ids {
		if id == a.qualityImportRun {
			chosen = a.qualityImportOptions[id]
			break
		}
	}
	next := chosen
	selectField(c, "quality-import", &next, labels, locales.ExamQualityImport()).Width(t.Space(40))
	if next != chosen {
		for index, label := range labels {
			if label == next {
				if index == 0 {
					a.qualityImportRun = 0
				} else {
					a.qualityImportRun = ids[index-1]
				}
				break
			}
		}
	}
}

func qualityIssueInlineFixable(issue store.ExamQualityIssue) bool {
	if issue.StudentNo == "" {
		return false
	}
	switch issue.Code {
	case "missing_track", "score_out_of_range", "missing_scores":
		return true
	default:
		return false
	}
}

func (a *app) qualityIssueRow(c *ui.Context, p uiPalette, issue store.ExamQualityIssue, open func(), onFix func()) {
	t := p.theme
	tone := toneWarning
	if issue.Severity == "blocking" {
		tone = toneDanger
	}
	label := a.examQualityText(issue)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		element := listRowButton(c, label).
			Height(0).MinHeight(t.Space(14)).Padding(t.Space(3), t.Space(3)).Grow(1).Shrink(1).MinWidth(0)
		element.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(t.Space(2)).Children(func() {
				badge(c, p, a.examSeverityName(issue.Severity), tone)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(0).Children(func() {
					ui.Text(c, label).FontSize(bodySize(t)).Pretty()
					if meta := a.qualityIssueMeta(issue); meta != "" {
						ui.Text(c, meta).FontSize(captionSize(t)).TextColor(p.muted).
							SingleLine().MinWidth(0).Shrink(1).Tooltip(meta)
					}
				})
				ui.Icon(c, iconArrowRight).FontSize(t.Space(4)).TextColor(p.muted).Shrink(0)
			})
		})
		if element.Clicked() {
			open()
		}
		if onFix != nil {
			fixBtn := labelledButton(c, locales.QualityFixInline(), iconEdit, false)
			fixBtn.Label(locales.QualityFixInline() + " " + issue.StudentNo)
			if fixBtn.Clicked() {
				onFix()
			}
		}
	})
}

func (a *app) openQualityFix(overview qualityExamOverview, issue store.ExamQualityIssue) {
	if a.store == nil || issue.StudentNo == "" {
		return
	}
	ctx := context.Background()
	entries, _, err := a.store.ListExamRoster(ctx, store.ExamRosterFilter{
		ExamID:   overview.ExamID,
		CohortID: overview.CohortID,
		Query:    issue.StudentNo,
		PageSize: 20,
	})
	if err != nil {
		a.qualityError = err.Error()
		return
	}
	var matched *store.ExamRosterEntry
	for idx := range entries {
		if entries[idx].StudentNo == issue.StudentNo {
			matched = &entries[idx]
			break
		}
	}
	if matched == nil {
		a.openQualityIssue(overview, issue)
		return
	}
	a.qualityFixExamID = overview.ExamID
	a.qualityFixCohortID = overview.CohortID
	a.qualityFixExamName = overview.ExamLabel
	a.qualityFixStudentID = matched.StudentID
	a.qualityFixStudentNo = matched.StudentNo
	a.qualityFixStudentName = matched.Name
	a.qualityFixIssue = issue
	a.qualityFixTrack = matched.Track
	if a.qualityFixTrack == "" {
		for _, tr := range a.tracks {
			if tr.Enabled {
				a.qualityFixTrack = tr.Key
				break
			}
		}
	}
	a.qualityFixSubject = issue.Subject
	if a.qualityFixSubject == "" {
		for _, sub := range a.subjects {
			if sub.Enabled {
				a.qualityFixSubject = sub.Key
				break
			}
		}
	}
	a.qualityFixScore = 0
	a.qualityFixFlag = ""
	a.qualityFixError = ""
	a.qualityFixOpen = true
}

func (a *app) qualityFixDrawer(c *ui.Context, p uiPalette) {
	if !a.qualityFixOpen {
		return
	}
	subtitle := fmt.Sprintf("%s · %s (%s)", a.qualityFixExamName, a.qualityFixStudentName, a.qualityFixStudentNo)
	sideDrawer(c, &a.qualityFixOpen, sideDrawerProps{
		Key:      "quality-fix",
		Title:    locales.QualityFixInline(),
		Subtitle: subtitle,
		Width:    460,
	}, func() {
		notice(c, p, noticeWarning, a.examQualityText(a.qualityFixIssue))
		if a.qualityFixError != "" {
			notice(c, p, noticeDanger, a.qualityFixError)
		}
		if a.qualityFixIssue.Code == "missing_track" {
			trackLabels := make([]string, 0, len(a.tracks))
			trackKeys := make([]string, 0, len(a.tracks))
			chosen := ""
			for _, tr := range a.tracks {
				if !tr.Enabled {
					continue
				}
				lbl := a.trackName(tr.Key, tr.Name)
				trackLabels = append(trackLabels, lbl)
				trackKeys = append(trackKeys, tr.Key)
				if tr.Key == a.qualityFixTrack {
					chosen = lbl
				}
			}
			if chosen == "" && len(trackLabels) > 0 {
				chosen = trackLabels[0]
				a.qualityFixTrack = trackKeys[0]
			}
			formFieldItem(c, p, locales.QualityFixTrackLabel(), "", false, func() {
				next := chosen
				selectField(c, "quality-fix-track", &next, trackLabels, locales.QualityFixTrackLabel()).FillWidth()
				if next != chosen {
					for idx, lbl := range trackLabels {
						if lbl == next {
							a.qualityFixTrack = trackKeys[idx]
							break
						}
					}
				}
			})
			return
		}
		subjLabels := make([]string, 0, len(a.subjects))
		subjKeys := make([]string, 0, len(a.subjects))
		chosenSubj := ""
		for _, sub := range a.subjects {
			if !sub.Enabled {
				continue
			}
			lbl := a.subjectName(sub.Key, sub.Name)
			subjLabels = append(subjLabels, lbl)
			subjKeys = append(subjKeys, sub.Key)
			if sub.Key == a.qualityFixSubject {
				chosenSubj = lbl
			}
		}
		if chosenSubj == "" && len(subjLabels) > 0 {
			chosenSubj = subjLabels[0]
			a.qualityFixSubject = subjKeys[0]
		}
		formFieldItem(c, p, locales.ColumnSubject(), "", false, func() {
			next := chosenSubj
			selectField(c, "quality-fix-subject", &next, subjLabels, locales.ColumnSubject()).FillWidth()
			if next != chosenSubj {
				for idx, lbl := range subjLabels {
					if lbl == next {
						a.qualityFixSubject = subjKeys[idx]
						break
					}
				}
			}
		})
		flagKeys := []string{"", "absent", "cheating", "void"}
		flagLabels := []string{"-", "absent", "cheating", "void"}
		chosenFlag := flagLabels[0]
		for idx, k := range flagKeys {
			if k == a.qualityFixFlag {
				chosenFlag = flagLabels[idx]
				break
			}
		}
		formFieldItem(c, p, locales.QualityFixFlagLabel(), "", false, func() {
			next := chosenFlag
			selectField(c, "quality-fix-flag", &next, flagLabels, locales.QualityFixFlagLabel()).FillWidth()
			if next != chosenFlag {
				for idx, lbl := range flagLabels {
					if lbl == next {
						a.qualityFixFlag = flagKeys[idx]
						break
					}
				}
			}
		})
		if a.qualityFixFlag == "" {
			formFieldItem(c, p, locales.QualityFixScoreLabel(), "", false, func() {
				numberField(c, &a.qualityFixScore, 0, 1000, 0.5, locales.QualityFixScoreLabel()).FillWidth()
			})
		}
	}, func() {
		if plainButton(c, locales.PlanCancel()).Clicked() {
			a.qualityFixOpen = false
		}
		if primaryButton(c, locales.Save()).Clicked() {
			a.saveQualityFix()
		}
	})
}

func (a *app) saveQualityFix() {
	if a.store == nil || a.qualityFixStudentID == 0 {
		return
	}
	ctx := context.Background()
	var err error
	if a.qualityFixIssue.Code == "missing_track" {
		err = a.store.UpdateExamRosterTrack(ctx, a.qualityFixExamID, a.qualityFixCohortID,
			a.qualityFixStudentID, a.qualityFixTrack)
	} else {
		var scorePtr *float64
		if a.qualityFixFlag == "" {
			v := a.qualityFixScore
			scorePtr = &v
		}
		err = a.store.UpdateExamScore(ctx, a.qualityFixExamID, a.qualityFixCohortID,
			a.qualityFixStudentID, a.qualityFixSubject, scorePtr, a.qualityFixFlag)
	}
	if err != nil {
		a.qualityFixError = err.Error()
		return
	}
	a.qualityFixOpen = false
	a.qualityFixError = ""
	a.notifyMain(locales.QualityFixSaved())
	a.reloadQuality(ctx)
	if a.selectedExamID == a.qualityFixExamID {
		a.reloadExamData(ctx)
	}
	a.reloadAssessments(ctx)
	a.reloadWorkspaceSummary(ctx)
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
	a.setExamSelectionState(overview.ExamID, overview.CohortID)
	a.examTab = 5
	a.page = pageExams
	a.lastPage = pageExams
	a.reloadExamData(context.Background())
}

func (a *app) openQualityIssue(overview qualityExamOverview, issue store.ExamQualityIssue) {
	if a.store == nil {
		return
	}
	a.setExamSelectionState(overview.ExamID, overview.CohortID)
	a.examTab = qualityIssueTab(issue.Code)
	if issue.StudentNo != "" {
		a.examRosterQuery = issue.StudentNo
		a.examScoreQuery = issue.StudentNo
	}
	a.page = pageExams
	a.lastPage = pageExams
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
