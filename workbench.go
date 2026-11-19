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

func (a *app) batchView(c *ui.Context) {
	t := c.Theme()
	if a.batchSidebarWidth == 0 {
		a.batchSidebarWidth = 240
	}
	if len(a.batches) == 0 {
		ui.Column(c).Fill().Center().Gap(t.Space(3)).Children(func() {
			ui.Text(c, locales.BatchEmpty()).TextColor(t.TextMuted)
			if ui.PrimaryButton(c, locales.BatchNew()).Clicked() {
				a.openNewBatch()
			}
		})
		a.newBatchModal(c)
		return
	}
	ui.Split(c, &a.batchSidebarWidth, func() {
		a.batchSidebar(c)
	}, func() {
		a.batchDetails(c)
	}).Fill()
	a.newBatchModal(c)
	a.zeroModal(c)
}

func (a *app) batchSidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(t.Space(3)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, locales.BatchListTitle()).Bold()
			ui.Spacer(c)
			if ui.Button(c, locales.BatchNew()).Clicked() {
				a.openNewBatch()
			}
		})
		sidebar := ui.Sidebar(c, &a.selectedBatchID, func() {
			ui.SidebarSection(c, locales.BatchListTitle(), nil, func() {
				for _, batch := range a.batches {
					ui.SidebarItem(c, strconv.FormatInt(batch.ID, 10), nil, a.batchLabel(batch))
				}
			})
		}).Fill()
		if sidebar.Changed() {
			a.loadLatestRun(context.Background())
		}
	})
}

func (a *app) batchDetails(c *ui.Context) {
	t := c.Theme()
	batch, err := a.selectedBatchValue()
	if err != nil {
		ui.Text(c, locales.BatchSelectHint()).TextColor(t.TextMuted)
		return
	}
	ui.Column(c).Fill().Padding(t.Space(3)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, a.batchLabel(*batch)).FontSize(t.FontSize * 1.3).Bold()
			ui.Spacer(c)
			ui.Text(c, a.batchStatusLabel(batch.Status)).TextColor(t.TextMuted)
		})
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, locales.BatchPlanVersion()).TextColor(t.TextMuted)
			if len(a.published) == 0 {
				ui.Text(c, locales.BatchSelectVersion()).TextColor(t.Danger)
			} else {
				ui.Select(c, &a.calculateVersion, a.versionOptions()).Width(t.Space(46)).Label(locales.BatchPlanVersion())
			}
			if ui.Button(c, locales.BatchImport()).Label(locales.BatchImport()).Clicked() {
				a.chooseImportFile()
			}
			calculate := ui.PrimaryButton(c, locales.BatchCalculate())
			calculate.Disabled(a.batchBusy || len(a.published) == 0)
			if calculate.Clicked() {
				a.startCalculate()
			}
			export := ui.Button(c, locales.BatchExport())
			export.Disabled(a.batchBusy || len(a.resultRows) == 0)
			if export.Clicked() {
				a.chooseExportFile()
			}
		})
		if a.batchBusy {
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Spinner(c).Label(locales.BatchWorking())
				ui.Text(c, locales.BatchWorking()).TextColor(t.TextMuted)
			})
		}
		if a.batchError != "" {
			ui.Text(c, a.batchError).TextColor(t.Danger)
		}
		if a.batchMessage != "" {
			ui.Text(c, a.batchMessage).TextColor(t.TextMuted)
		}
		if a.importSummary != nil {
			a.importSummaryView(c)
		}
		ui.Divider(c)
		ui.Text(c, locales.BatchResults()).Bold()
		if len(a.resultRows) == 0 {
			ui.Text(c, locales.BatchNoResults()).TextColor(t.TextMuted)
			return
		}
		a.resultsTable(c)
	})
}

func (a *app) importSummaryView(c *ui.Context) {
	t := c.Theme()
	summary := a.importSummary
	ui.Column(c).Gap(t.Space(1)).Children(func() {
		ui.Text(c, fmt.Sprintf(locales.BatchImportSummary(), summary.Classes, summary.Students, summary.StatGroups))
		if len(summary.Warnings) == 0 {
			return
		}
		ui.Text(c, locales.BatchWarnings()).Bold()
		for _, warning := range summary.Warnings {
			ui.Text(c, "- "+warning).TextColor(t.TextMuted)
		}
	})
}

func (a *app) resultsTable(c *ui.Context) {
	a.resultTable.Key = func(row int) any {
		return fmt.Sprintf("%d:%s", a.resultRows[row].StatGroupID, a.resultRows[row].Subject)
	}
	columns := []ui.TableColumn{
		{Title: locales.ColumnClass(), Width: 100},
		{Title: locales.ColumnSubject(), Width: 80},
		{Title: locales.ColumnStatGroup(), Width: 120},
		{Title: locales.ColumnTeachers(), Width: 150},
		{Title: locales.ColumnTotal(), Width: 110, Align: ui.End},
		{Title: locales.ColumnRank(), Width: 70, Align: ui.End},
		{Title: locales.ColumnTier(), Width: 80},
		{Title: locales.ColumnPoints(), Width: 80, Align: ui.End},
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
			ui.Textf(c, "%.2f", item.Total)
		case 5:
			ui.Textf(c, "%d", item.Rank)
		case 6:
			ui.Text(c, levelLabel(item.TierIndex))
		case 7:
			ui.Textf(c, "%.0f", item.Points)
		}
	}).Grow(1).FillWidth()
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

func (a *app) newBatchModal(c *ui.Context) {
	if !a.newBatchOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c, &a.newBatchOpen, func() {
		ui.Column(c).Padding(t.Space(4)).Gap(t.Space(2)).Width(t.Space(110)).Children(func() {
			ui.Text(c, locales.BatchNew()).Bold()
			ui.Text(c, locales.BatchAcademicYear()).TextColor(t.TextMuted)
			ui.TextInput(c, &a.newBatchYear)
			ui.Text(c, locales.BatchSemester()).TextColor(t.TextMuted)
			ui.Select(c, &a.newBatchSemester, []string{locales.SemesterSpring(), locales.SemesterAutumn()})
			ui.Text(c, locales.BatchExamType()).TextColor(t.TextMuted)
			ui.Select(c, &a.newBatchExamType, []string{locales.ExamMidterm(), locales.ExamFinal(), locales.ExamGaokao()})
			ui.Text(c, locales.BatchGrade()).TextColor(t.TextMuted)
			ui.Select(c, &a.newBatchGrade, []string{locales.GradeG1(), locales.GradeG2(), locales.GradeG3()})
			if a.newBatchError != "" {
				ui.Text(c, a.newBatchError).TextColor(t.Danger)
			}
			ui.Row(c).Gap(t.Space(2)).Children(func() {
				if ui.PrimaryButton(c, locales.BatchCreate()).Clicked() {
					a.startCreateBatch()
				}
				if ui.Button(c, locales.BatchCancel()).Clicked() {
					a.newBatchOpen = false
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
	a.batchMessage = fmt.Sprintf(locales.BatchImportSummary(), summary.Classes, summary.Students, summary.StatGroups)
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

func (a *app) zeroModal(c *ui.Context) {
	if !a.zeroOpen {
		return
	}
	t := c.Theme()
	excludeClass := int64(0)
	fill := false
	ui.Modal(c, &a.zeroOpen, func() {
		ui.Column(c).Padding(t.Space(4)).Gap(t.Space(2)).Width(t.Space(180)).Children(func() {
			ui.Text(c, locales.ZeroTitle()).Bold()
			ui.Text(c, locales.ZeroIntro()).TextColor(t.TextMuted)
			for index := range a.zeroIssues {
				issue := a.zeroIssues[index]
				ui.Text(c, issue.ClassName).Bold()
				tiers := issue.MissingTiers
				if len(tiers) == 0 {
					tiers = issue.AllTiers
					ui.Text(c, locales.ZeroExplicitZero()).TextColor(t.TextMuted)
				}
				for _, tier := range tiers {
					key := issue.ClassID + ":" + tier
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Text(c, a.tierLabel(tier)).Width(t.Space(16))
						ui.NumberInput(c, a.zeroValues[key], 0, 100000, 1).Width(t.Space(20)).Label(a.tierLabel(tier))
					})
				}
				if ui.Button(c, locales.ZeroExclude()).Clicked() {
					excludeClass, _ = strconv.ParseInt(issue.ClassID, 10, 64)
				}
			}
			ui.Text(c, locales.ZeroReason()).TextColor(t.TextMuted)
			ui.TextInput(c, &a.zeroReason)
			if a.zeroError != "" {
				ui.Text(c, a.zeroError).TextColor(t.Danger)
			}
			ui.Row(c).Gap(t.Space(2)).Children(func() {
				if ui.PrimaryButton(c, locales.ZeroFill()).Clicked() {
					fill = true
				}
				if ui.Button(c, locales.ZeroClose()).Clicked() {
					a.zeroOpen = false
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
