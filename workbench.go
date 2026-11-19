package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tally/internal/calc"
	"tally/internal/importer"
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

func (a *app) reloadBatches(ctx context.Context) {
	if a.store == nil {
		return
	}
	batches, err := a.store.ListBatches(ctx)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.batches = batches
	found := false
	for _, batch := range batches {
		if strconv.FormatInt(batch.ID, 10) == a.selectedBatchID {
			found = true
			break
		}
	}
	if !found {
		if len(batches) > 0 {
			a.selectedBatchID = strconv.FormatInt(batches[0].ID, 10)
		} else {
			a.selectedBatchID = ""
		}
	}
	a.loadLatestRun(ctx)
}

func (a *app) loadLatestRun(ctx context.Context) {
	a.resultRows = nil
	a.personalScores = nil
	batch, err := a.selectedBatchValue()
	if err != nil {
		return
	}
	if batch.CurrentPlanVersionID != 0 {
		if label := a.labelForVersionID(batch.CurrentPlanVersionID); label != "" {
			a.calculateVersion = label
		}
	}
	_, rows, personal, err := a.store.LatestRun(ctx, batch.ID)
	if err != nil {
		return
	}
	a.resultRows = rows
	a.personalScores = personal
}

func (a *app) selectedBatchValue() (*store.Batch, error) {
	if a.selectedBatchID == "" {
		return nil, store.ErrNotFound
	}
	id, err := strconv.ParseInt(a.selectedBatchID, 10, 64)
	if err != nil {
		return nil, err
	}
	for index := range a.batches {
		if a.batches[index].ID == id {
			return &a.batches[index], nil
		}
	}
	return nil, store.ErrNotFound
}

func (a *app) batchPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.batchSidebarWidth == 0 {
		a.batchSidebarWidth = t.Space(64)
	}
	if a.batchResultsHeight == 0 {
		a.batchResultsHeight = t.Space(62)
	}
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.BatchWorkbenchTitle(), locales.BatchPageSubtitle(), func() {
			if labelledButton(c, locales.BatchNew(), iconPlus, true).Clicked() {
				a.openNewBatch()
			}
		})
		if len(a.batches) == 0 {
			emptyState(c, p, iconEmptyBatch, locales.BatchEmpty(), locales.BatchEmptyHint(), func() {
				if labelledButton(c, locales.BatchNew(), iconPlus, true).Clicked() {
					a.openNewBatch()
				}
			})
		} else {
			ui.Split(c, &a.batchSidebarWidth, func() {
				a.batchSidebar(c, p)
			}, func() {
				a.batchDetails(c, p)
			}).Grow(1)
		}
	})
	a.newBatchModal(c, p)
	a.zeroModal(c, p)
}

func (a *app) batchSidebar(c *ui.Context, p uiPalette) {
	changed := false
	listPanel(c, p, locales.BatchListPanelTitle(), len(a.batches), func() {
		sidebar := ui.Sidebar(c, &a.selectedBatchID, func() {
			for _, batch := range a.batches {
				ui.SidebarItem(c, strconv.FormatInt(batch.ID, 10), iconBatches, a.batchLabel(batch))
			}
		}).Label(locales.BatchListTitle()).Fill()
		if sidebar.Changed() {
			changed = true
		}
	})
	if changed {
		a.loadLatestRun(context.Background())
	}
}

func (a *app) batchDetails(c *ui.Context, p uiPalette) {
	batch, err := a.selectedBatchValue()
	if err != nil {
		ui.Column(c).Fill().Children(func() {
			emptyState(c, p, iconEmptyBatch, locales.BatchSelectHint(), "", nil)
		})
		return
	}
	t := p.theme
	ui.SplitVertical(c, &a.batchResultsHeight, func() {
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
				a.batchSummary(c, p, *batch)
				a.batchActions(c, p)
				if a.batchBusy {
					panel(c, p).Padding(t.Space(3)).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Spinner(c).Label(locales.BatchWorking())
							ui.Text(c, locales.BatchWorking()).TextColor(p.muted)
						})
					})
				}
				if a.batchError != "" {
					notice(c, p, noticeDanger, a.batchError)
				}
				if a.batchMessage != "" {
					notice(c, p, noticeSuccess, a.batchMessage)
				}
				if a.importSummary != nil {
					a.importSummaryView(c, p)
				}
			})
		})
	}, func() {
		a.batchResults(c, p)
	}).Fill()
}

func (a *app) batchSummary(c *ui.Context, p uiPalette, batch store.Batch) {
	t := p.theme
	panel(c, p).Padding(t.Space(4)).Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
				ui.Text(c, a.batchLabel(batch)).FontSize(t.Rem(1.35)).Bold().
					LetterSpacing(-0.2).Role(ui.RoleHeading).Level(2)
			})
			statusBadge(c, p, batch.Status, a.batchStatusLabel(batch.Status))
		})
		ui.Column(c).Gap(t.Space(2)).Children(func() {
			ui.Text(c, locales.BatchWorkflowTitle()).FontSize(t.Rem(0.8)).
				TextColor(p.muted).LetterSpacing(0.4)
			a.batchWorkflow(c, p, batch)
		})
	})
}

type workflowStep struct {
	label   string
	caption string
	done    bool
	current bool
}

func (a *app) batchWorkflow(c *ui.Context, p uiPalette, batch store.Batch) {
	t := p.theme
	imported := batch.Status != "draft" || a.importSummary != nil
	calculated := batch.Status == "calculated" || len(a.resultRows) > 0
	steps := []workflowStep{
		{label: locales.WorkflowImport()},
		{label: locales.WorkflowCalculate()},
		{label: locales.WorkflowExport()},
	}
	switch {
	case !imported:
		steps[0].current = true
		steps[0].caption = locales.WorkflowCurrent()
		steps[1].caption = locales.WorkflowPending()
		steps[2].caption = locales.WorkflowPending()
	case !calculated:
		steps[0].done = true
		steps[1].current = true
		steps[0].caption = locales.WorkflowDone()
		steps[1].caption = locales.WorkflowCurrent()
		steps[2].caption = locales.WorkflowPending()
	default:
		steps[0].done = true
		steps[1].done = true
		steps[2].current = true
		steps[0].caption = locales.WorkflowDone()
		steps[1].caption = locales.WorkflowDone()
		steps[2].caption = locales.WorkflowCurrent()
	}
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		for index, step := range steps {
			marker := ui.Box(c).Size(t.Space(7), t.Space(7)).Radius(t.Space(3.5)).Center()
			switch {
			case step.done:
				marker.Background(p.accent).Children(func() {
					ui.Icon(c, iconCheck).FontSize(t.Space(3.5)).TextColor(p.accentText)
				})
			case step.current:
				marker.Background(p.accentSoft).Border(1, p.accent).Children(func() {
					ui.Textf(c, "%d", index+1).FontSize(t.Rem(0.8)).Bold().TextColor(p.accent)
				})
			default:
				marker.Background(p.panelMuted).Border(1, p.border).Children(func() {
					ui.Textf(c, "%d", index+1).FontSize(t.Rem(0.8)).TextColor(p.muted)
				})
			}
			ui.Column(c).Gap(0).Children(func() {
				ui.Text(c, step.label).FontSize(t.Rem(0.86)).Bold()
				ui.Text(c, step.caption).FontSize(t.Rem(0.78)).TextColor(p.muted)
			})
			if index < len(steps)-1 {
				ui.Box(c).Height(1).Grow(1).Background(p.border).Margin(0, t.Space(2))
			}
		}
	})
}

func (a *app) batchActions(c *ui.Context, p uiPalette) {
	t := p.theme
	panel(c, p).Padding(t.Space(4)).Children(func() {
		ui.Row(c).Wrap().AlignItems(ui.End).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Width(t.Space(50)).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.BatchPlanVersion()).FontSize(t.Rem(0.8)).TextColor(p.muted)
				if len(a.published) == 0 {
					ui.Text(c, locales.BatchSelectVersion()).FontSize(t.Rem(0.82)).TextColor(p.danger)
				} else {
					ui.Select(c, &a.calculateVersion, a.versionOptions()).
						Width(t.Space(50)).Label(locales.BatchPlanVersion())
				}
			})
			ui.Spacer(c)
			if labelledButton(c, locales.BatchImport(), iconImport, false).Clicked() {
				a.chooseImportFile()
			}
			calculate := labelledButton(c, locales.BatchCalculate(), iconCalculate, true)
			calculate.Disabled(a.batchBusy || len(a.published) == 0)
			if calculate.Clicked() {
				a.startCalculate()
			}
			export := labelledButton(c, locales.BatchExport(), iconExport, false)
			export.Disabled(a.batchBusy || len(a.resultRows) == 0)
			if export.Clicked() {
				a.chooseExportFile()
			}
		})
	})
}

func (a *app) importSummaryView(c *ui.Context, p uiPalette) {
	t := p.theme
	summary := a.importSummary
	notice(c, p, noticeSuccess, fmt.Sprintf(locales.BatchImportSummary(), summary.Classes, summary.Students, summary.StatGroups))
	if len(summary.Warnings) == 0 {
		return
	}
	ui.Column(c).Padding(t.Space(3)).Gap(t.Space(1)).Background(p.warningSoft).
		Border(1, p.warning.Alpha(0.35)).Radius(t.Space(2)).Children(func() {
		ui.Text(c, locales.BatchWarnings()).FontSize(t.Rem(0.84)).Bold().TextColor(p.warning)
		for _, warning := range summary.Warnings {
			ui.Text(c, "- "+warning).FontSize(t.Rem(0.82)).TextColor(p.muted).Selectable()
		}
	})
}

func (a *app) batchResults(c *ui.Context, p uiPalette) {
	t := p.theme
	panel(c, p).Grow(1).Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.BatchResults(), len(a.resultRows))
			ui.Spacer(c)
			if len(a.personalScores) > 0 {
				ui.Textf(c, locales.BatchPersonalScores(), len(a.personalScores)).
					FontSize(t.Rem(0.8)).TextColor(p.muted)
			}
		})
		if len(a.resultRows) == 0 {
			smallEmptyState(c, p, locales.BatchNoResults())
			return
		}
		a.resultsTable(c, p)
	})
}

func (a *app) resultsTable(c *ui.Context, p uiPalette) {
	a.resultTable.Key = func(row int) any {
		return fmt.Sprintf("%d:%s", a.resultRows[row].StatGroupID, a.resultRows[row].Subject)
	}
	a.resultTable.RowHeight = p.theme.Space(8)
	columns := []ui.TableColumn{
		{Title: locales.ColumnClass(), Width: 80},
		{Title: locales.ColumnSubject(), Width: 60},
		{Title: locales.ColumnStatGroup(), Width: 88},
		{Title: locales.ColumnTeachers(), Width: 0},
		{Title: locales.ColumnTotal(), Width: 72, Align: ui.End},
		{Title: locales.ColumnRank(), Width: 46, Align: ui.End},
		{Title: locales.ColumnTier(), Width: 52},
		{Title: locales.ColumnPoints(), Width: 52, Align: ui.End},
	}
	ui.Table(c, &a.resultTable, columns, len(a.resultRows), func(row, col int) {
		item := a.resultRows[row]
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
			ui.Textf(c, "%.2f", item.Total).FontFeatures("tnum")
		case 5:
			ui.Textf(c, "%d", item.Rank).FontFeatures("tnum")
		case 6:
			ui.Text(c, levelLabel(item.TierIndex))
		case 7:
			ui.Textf(c, "%.0f", item.Points).FontFeatures("tnum")
		}
	}).Label(locales.BatchResults()).Grow(1).FillWidth()
}

func (a *app) openNewBatch() {
	if a.newBatchYear == "" {
		a.newBatchYear = "2025-2026"
	}
	if a.newBatchSemester == "" {
		a.newBatchSemester = locales.SemesterSpring()
	}
	if a.newBatchExamType == "" {
		a.newBatchExamType = locales.ExamMidterm()
	}
	if a.newBatchGrade == "" {
		a.newBatchGrade = locales.GradeG1()
	}
	a.newBatchError = ""
	a.newBatchOpen = true
}

func (a *app) newBatchModal(c *ui.Context, p uiPalette) {
	if !a.newBatchOpen {
		return
	}
	t := p.theme
	ui.Modal(c, &a.newBatchOpen, func() {
		ui.Column(c).Width(t.Space(120)).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.BatchNew()).FontSize(t.Rem(1.3)).Bold().
					Role(ui.RoleHeading).Level(2)
				ui.Text(c, locales.BatchNewSubtitle()).FontSize(t.Rem(0.9)).TextColor(p.muted).Pretty()
			})
			ui.Form(c, func() {
				ui.Field(c, locales.BatchAcademicYear(), func() {
					ui.TextInput(c, &a.newBatchYear)
				})
				ui.Field(c, locales.BatchSemester(), func() {
					ui.Select(c, &a.newBatchSemester, []string{locales.SemesterSpring(), locales.SemesterAutumn()})
				})
				ui.Field(c, locales.BatchExamType(), func() {
					ui.Select(c, &a.newBatchExamType, []string{locales.ExamMidterm(), locales.ExamFinal(), locales.ExamGaokao()})
				})
				ui.Field(c, locales.BatchGrade(), func() {
					ui.Select(c, &a.newBatchGrade, []string{locales.GradeG1(), locales.GradeG2(), locales.GradeG3()})
				})
			})
			if a.newBatchError != "" {
				notice(c, p, noticeDanger, a.newBatchError)
			}
			ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if ui.Button(c, locales.BatchCancel()).Clicked() {
					a.newBatchOpen = false
				}
				if ui.PrimaryButton(c, locales.BatchCreate()).Clicked() {
					a.startCreateBatch()
				}
			})
		})
	})
}

func (a *app) startCreateBatch() {
	input := store.CreateBatchInput{
		AcademicYear: strings.TrimSpace(a.newBatchYear),
		Semester:     semesterCode(a.newBatchSemester),
		ExamType:     examCode(a.newBatchExamType),
		Grade:        gradeCode(a.newBatchGrade),
	}
	work := func() (store.Batch, error) {
		ctx := context.Background()
		return a.store.CreateBatch(ctx, input)
	}
	if a.win == nil {
		batch, err := work()
		a.finishCreateBatch(batch, err)
		return
	}
	go func() {
		batch, err := work()
		a.win.Update(func() {
			a.finishCreateBatch(batch, err)
		})
	}()
}

func (a *app) finishCreateBatch(batch store.Batch, err error) {
	if err != nil {
		a.newBatchError = err.Error()
		return
	}
	a.newBatchOpen = false
	a.newBatchError = ""
	a.selectedBatchID = strconv.FormatInt(batch.ID, 10)
	a.reloadBatches(context.Background())
}

func (a *app) chooseImportFile() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:   locales.BatchImport(),
			Filters: []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx", "xls"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		if a.win == nil {
			a.startImport(paths[0])
			return
		}
		a.win.Update(func() {
			a.startImport(paths[0])
		})
	}()
}

func (a *app) startImport(path string) {
	batch, err := a.selectedBatchValue()
	if err != nil {
		a.batchError = err.Error()
		return
	}
	a.batchBusy = true
	a.batchError = ""
	work := func() (store.ImportSummary, error) {
		ctx := context.Background()
		summary, _, err := importer.ImportInto(ctx, a.store, batch.ID, path, importer.Options{
			ConversionDir: filepath.Join(a.dataDir, "converted"),
			CreatedBy:     "local",
		})
		return summary, err
	}
	run := func() {
		summary, err := work()
		if a.win == nil {
			a.finishImport(summary, err)
			return
		}
		a.win.Update(func() {
			a.finishImport(summary, err)
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *app) finishImport(summary store.ImportSummary, err error) {
	a.batchBusy = false
	if err != nil {
		a.batchError = err.Error()
		return
	}
	a.batchError = ""
	a.importSummary = &summary
	a.batchMessage = ""
	a.reloadBatches(context.Background())
}

func (a *app) startCalculate() {
	batch, err := a.selectedBatchValue()
	if err != nil {
		a.batchError = err.Error()
		return
	}
	version := a.publishedByLabel(a.calculateVersion)
	if version == nil {
		a.batchError = locales.BatchSelectVersion()
		return
	}
	a.batchBusy = true
	a.batchError = ""
	work := func() (calc.Result, error) {
		ctx := context.Background()
		return pipeline.CalculateBatch(ctx, a.store, batch.ID, version.Version.ID)
	}
	run := func() {
		result, err := work()
		if a.win == nil {
			a.finishCalculate(result, err)
			return
		}
		a.win.Update(func() {
			a.finishCalculate(result, err)
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *app) finishCalculate(result calc.Result, err error) {
	a.batchBusy = false
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
		a.batchError = err.Error()
		return
	}
	a.zeroOpen = false
	a.batchError = ""
	a.batchMessage = fmt.Sprintf(locales.BatchCalculated(), len(result.Rows), len(result.Personal))
	a.reloadBatches(context.Background())
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
	ui.Modal(c, &a.zeroOpen, func() {
		ui.Column(c).Width(t.Space(170)).Gap(t.Space(4)).Children(func() {
			ui.Row(c).AlignItems(ui.Start).Gap(t.Space(3)).Children(func() {
				ui.Box(c).Size(t.Space(11), t.Space(11)).Radius(t.Space(3)).
					Background(p.warningSoft).Center().Children(func() {
					ui.Icon(c, iconAlert).FontSize(t.Space(5)).TextColor(p.warning)
				})
				ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
					ui.Text(c, locales.ZeroTitle()).FontSize(t.Rem(1.3)).Bold().
						Role(ui.RoleHeading).Level(2)
					ui.Text(c, locales.ZeroIntro()).FontSize(t.Rem(0.9)).TextColor(p.muted).Pretty()
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
			ui.Field(c, locales.ZeroReason(), func() {
				ui.TextInput(c, &a.zeroReason)
			}).Error(a.zeroError)
			ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if ui.Button(c, locales.ZeroClose()).Clicked() {
					a.zeroOpen = false
				}
				if ui.PrimaryButton(c, locales.ZeroFill()).Clicked() {
					fill = true
				}
			})
		})
	})
	if excludeClass != 0 {
		a.startExcludeClass(excludeClass)
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
			ui.Text(c, issue.ClassName).FontSize(t.Rem(0.95)).Bold().Grow(1)
			if ui.Button(c, locales.ZeroExclude()).Clicked() {
				classID, _ := strconv.ParseInt(issue.ClassID, 10, 64)
				exclude(classID)
			}
		})
		tiers := issue.MissingTiers
		if len(tiers) == 0 {
			tiers = issue.AllTiers
			ui.Text(c, locales.ZeroExplicitZero()).FontSize(t.Rem(0.82)).TextColor(p.muted)
		}
		for _, tier := range tiers {
			key := issue.ClassID + ":" + tier
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Text(c, a.tierLabel(tier)).Width(t.Space(16))
				name := issue.ClassName + " " + a.tierLabel(tier)
				ui.NumberInput(c, a.zeroValues[key], 0, 100000, 1).
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
	a.runZeroWork(func(ctx context.Context, batchID int64) error {
		for _, item := range adjustments {
			value := item.value
			if err := a.store.AdjustTarget(ctx, batchID, item.classID, item.tier, nil, &value, reason, "local"); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *app) startExcludeClass(classID int64) {
	reason := strings.TrimSpace(a.zeroReason)
	if reason == "" {
		a.zeroError = locales.ZeroReasonRequired()
		return
	}
	a.runZeroWork(func(ctx context.Context, batchID int64) error {
		return a.store.ExcludeClassFromCompletion(ctx, batchID, classID, reason, "local")
	})
}

func (a *app) runZeroWork(work func(ctx context.Context, batchID int64) error) {
	batch, err := a.selectedBatchValue()
	if err != nil {
		a.zeroError = err.Error()
		return
	}
	version := a.publishedByLabel(a.calculateVersion)
	if version == nil {
		a.zeroError = locales.BatchSelectVersion()
		return
	}
	a.batchBusy = true
	a.zeroError = ""
	run := func() {
		ctx := context.Background()
		err := work(ctx, batch.ID)
		var result calc.Result
		if err == nil {
			result, err = pipeline.CalculateBatch(ctx, a.store, batch.ID, version.Version.ID)
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
	a.batchBusy = false
	if err != nil {
		if zero, ok := err.(*calc.ZeroTargetError); ok {
			a.zeroIssues = zero.Issues
			a.initZeroValues()
			a.zeroError = ""
			return
		}
		a.zeroError = err.Error()
		a.batchError = err.Error()
		return
	}
	a.zeroOpen = false
	a.zeroError = ""
	a.batchError = ""
	a.batchMessage = fmt.Sprintf(locales.BatchCalculated(), len(result.Rows), len(result.Personal))
	a.reloadBatches(context.Background())
}

func (a *app) chooseExportFile() {
	name := "Tally.xlsx"
	if batch, err := a.selectedBatchValue(); err == nil {
		name = a.batchLabel(*batch) + ".xlsx"
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       locales.BatchExport(),
			DefaultPath: name,
			Filters:     []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx"}}},
		})
		if err != nil || path == "" {
			return
		}
		if a.win == nil {
			a.startExport(path)
			return
		}
		a.win.Update(func() {
			a.startExport(path)
		})
	}()
}

func (a *app) startExport(path string) {
	batch, err := a.selectedBatchValue()
	if err != nil {
		a.batchError = err.Error()
		return
	}
	a.batchBusy = true
	a.batchError = ""
	run := func() {
		ctx := context.Background()
		err := pipeline.ExportLatest(ctx, a.store, batch.ID, path)
		if a.win == nil {
			a.finishExport(path, err)
			return
		}
		a.win.Update(func() {
			a.finishExport(path, err)
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *app) finishExport(path string, err error) {
	a.batchBusy = false
	if err != nil {
		a.batchError = err.Error()
		return
	}
	a.batchError = ""
	a.batchMessage = fmt.Sprintf(locales.BatchExported(), path)
}

func (a *app) batchLabel(batch store.Batch) string {
	return fmt.Sprintf("%s %s %s %s", batch.AcademicYear, a.semesterLabel(batch.Semester),
		a.gradeLabel(batch.Grade), a.examLabel(batch.ExamType))
}

func (a *app) batchStatusLabel(status string) string {
	switch status {
	case "draft":
		return locales.BatchStatusDraft()
	case "imported":
		return locales.BatchStatusImported()
	case "calculated":
		return locales.BatchStatusCalculated()
	default:
		return status
	}
}

func (a *app) semesterLabel(value string) string {
	switch value {
	case "spring":
		return locales.SemesterSpring()
	case "autumn":
		return locales.SemesterAutumn()
	default:
		return value
	}
}

func (a *app) examLabel(value string) string {
	switch value {
	case "midterm":
		return locales.ExamMidterm()
	case "final":
		return locales.ExamFinal()
	case "gaokao":
		return locales.ExamGaokao()
	default:
		return value
	}
}

func (a *app) gradeLabel(value string) string {
	switch value {
	case "g1":
		return locales.GradeG1()
	case "g2":
		return locales.GradeG2()
	case "g3":
		return locales.GradeG3()
	default:
		return value
	}
}

func semesterCode(label string) string {
	switch label {
	case locales.SemesterSpring():
		return "spring"
	case locales.SemesterAutumn():
		return "autumn"
	default:
		return label
	}
}

func examCode(label string) string {
	switch label {
	case locales.ExamMidterm():
		return "midterm"
	case locales.ExamFinal():
		return "final"
	case locales.ExamGaokao():
		return "gaokao"
	default:
		return label
	}
}

func gradeCode(label string) string {
	switch label {
	case locales.GradeG1():
		return "g1"
	case locales.GradeG2():
		return "g2"
	case locales.GradeG3():
		return "g3"
	default:
		return label
	}
}

func (a *app) tierLabel(key string) string {
	if version := a.publishedByLabel(a.calculateVersion); version != nil {
		for _, tier := range version.Version.Config.Tiers {
			if tier.Key == key {
				return tier.Label
			}
		}
	}
	switch key {
	case "qingbei":
		return locales.TierQingbei()
	case "c9":
		return locales.TierC9()
	case "project985":
		return locales.TierProject985()
	case "project211":
		return locales.TierProject211()
	case "special_control":
		return locales.TierSpecialControl()
	case "undergraduate":
		return locales.TierUndergraduate()
	}
	return key
}

func subjectLabel(code string) string {
	switch code {
	case "chinese":
		return locales.SubjectChinese()
	case "math":
		return locales.SubjectMath()
	case "english":
		return locales.SubjectEnglish()
	case "physics":
		return locales.SubjectPhysics()
	case "chemistry":
		return locales.SubjectChemistry()
	case "biology":
		return locales.SubjectBiology()
	case "history":
		return locales.SubjectHistory()
	case "geography":
		return locales.SubjectGeography()
	case "politics":
		return locales.SubjectPolitics()
	case "japanese":
		return locales.SubjectJapanese()
	}
	return code
}

func levelLabel(index int) string {
	switch index {
	case 0:
		return locales.LevelFirst()
	case 1:
		return locales.LevelSecond()
	case 2:
		return locales.LevelThird()
	default:
		return fmt.Sprintf(locales.LevelOther(), index+1)
	}
}

func decodeNames(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var names []string
	if err := json.Unmarshal([]byte(value), &names); err != nil {
		return value
	}
	return strings.Join(names, "、")
}
