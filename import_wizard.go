package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tally/internal/excel"
	"tally/internal/importer"
	"tally/internal/store"
	"tally/locales"
)

const (
	wizardStepItem = iota
	wizardStepFile
	wizardStepSheet
	wizardStepMapping
	wizardStepValidate
	wizardStepResult
)

type importWizardState struct {
	Kind              string
	Step              int
	ExamID            int64
	ExamCohortID      int64
	Workbook          *importer.Workbook
	FilePath          string
	FileName          string
	Sheet             string
	Mapping           map[string]string
	Data              store.ImportData
	Report            store.ImportReport
	Issues            []store.ImportIssue
	Preview           store.ImportPreview
	ConfirmedWarnings bool
	Error             string
	Busy              bool
	Reading           bool
	Summary           *store.ImportSummary
	ArchiveInspect    *importer.ArchiveInspectResult
	ArchiveSummary    *store.ArchivedImportSummary
	RosterSkipped     int
}

func (a *app) importWizardActive() bool {
	return a.importWizard != nil
}

func (a *app) openImportWizard(kind string) {
	if kind != "" && !a.stageEditable() {
		a.dataError = locales.StageReadOnly()
		return
	}
	step := wizardStepItem
	if kind != "" {
		step = wizardStepFile
	}
	a.importWizard = &importWizardState{Kind: kind, Step: step}
}

func (a *app) openExamImportWizard(kind string, examID, cohortID int64) {
	if a.importWizard != nil {
		return
	}
	a.importWizard = &importWizardState{
		Kind: kind, Step: wizardStepFile, ExamID: examID, ExamCohortID: cohortID,
	}
}

func (a *app) closeImportWizard() {
	a.importWizard = nil
}

func (a *app) wizardExam() bool {
	return a.importWizard != nil && a.importWizard.ExamID != 0
}

func (a *app) wizardTierKeys() []string {
	if a.wizardExam() && len(a.examTierKeys) > 0 {
		return a.examTierKeys
	}
	return importer.DefaultTiers()
}

func (a *app) wizardFields() []importer.Field {
	if a.importWizard == nil {
		return nil
	}
	if a.importWizard.Workbook != nil && a.importWizard.Sheet != "" {
		if table, ok := a.importWizard.Workbook.Sheet(a.importWizard.Sheet); ok {
			return importer.FieldsForTable(a.importWizard.Kind, table, a.subjects, a.wizardTierKeys())
		}
	}
	return importer.FieldsFor(a.importWizard.Kind, a.subjects, a.wizardTierKeys())
}

func (a *app) wizardSelectKind(kind string) {
	a.importWizard.Kind = kind
	a.importWizard.Step = wizardStepFile
	a.importWizard.Workbook = nil
	a.importWizard.Error = ""
}

// wizardGrade 返回当前导入所属的阶段年级。考试导入没有阶段，返回空。
func (a *app) wizardGrade() string {
	if a.importWizard == nil || a.importWizard.ExamID != 0 {
		return ""
	}
	return a.displayStage().GradeLevel
}

func (a *app) wizardContinueKind(kind string) {
	wizard := a.importWizard
	if wizard == nil {
		return
	}
	wizard.Kind = kind
	wizard.Step = wizardStepSheet
	wizard.Data = store.ImportData{}
	wizard.Preview = store.ImportPreview{}
	wizard.Issues = nil
	wizard.Summary = nil
	wizard.ConfirmedWarnings = false
	wizard.Error = ""
	if wizard.Workbook == nil {
		wizard.Step = wizardStepFile
		return
	}
	wizard.Sheet = importer.BestSheet(kind, wizard.Workbook)
	if wizard.Sheet == "" {
		wizard.Mapping = nil
		return
	}
	table, ok := wizard.Workbook.Sheet(wizard.Sheet)
	if !ok {
		wizard.Mapping = nil
		return
	}
	wizard.Mapping = importer.AutoMappingFor(kind, table, a.subjects, a.wizardTierKeys())
}

var errArchiveMissingStage = errors.New("archive missing stage")

func inspectAndValidateArchive(ctx context.Context, st *store.Store, opts importer.Options, subjects []store.Subject, examID, examCohortID int64, path string) (*importer.ArchiveInspectResult, store.ImportPreview, error) {
	inspectRes, err := importer.InspectArchiveWorkbook(path, opts, subjects)
	if err != nil {
		return nil, store.ImportPreview{}, err
	}
	if st == nil || examID == 0 || examCohortID == 0 {
		return inspectRes, store.ImportPreview{}, errArchiveMissingStage
	}
	preview, valErr := st.ValidateArchivedResultImportForExamCohort(ctx, examID, examCohortID, store.ArchivedResultImportRequest{
		Data:   inspectRes.Data,
		Report: inspectRes.Report,
		Issues: inspectRes.Issues,
	})
	if valErr != nil {
		return inspectRes, store.ImportPreview{}, valErr
	}
	return inspectRes, preview, nil
}

func (a *app) wizardChooseFile() {
	wizard := a.importWizard
	if wizard == nil || wizard.Reading {
		return
	}
	win := a.win
	st := a.store
	kind := wizard.Kind
	examID := wizard.ExamID
	examCohortID := wizard.ExamCohortID
	subjects := append([]store.Subject(nil), a.subjects...)
	opts := importer.Options{
		ConversionDir: filepath.Join(a.dataDir, "converted"),
		SofficePath:   a.sofficePath,
	}
	dialogTitle := locales.ImportWizardChooseFile()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:   dialogTitle,
			Filters: []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx", "xls"}}},
		})
		if err != nil || len(paths) == 0 || win == nil {
			return
		}
		path := paths[0]
		win.Update(func() {
			if a.importWizard != wizard {
				return
			}
			wizard.Reading = true
			wizard.Error = ""
			wizard.FilePath = path
			wizard.FileName = filepath.Base(path)
		})
		if kind == importer.KindArchivedResults {
			inspectRes, preview, inspectErr := inspectAndValidateArchive(context.Background(), st, opts, subjects, examID, examCohortID, path)
			win.Update(func() {
				if a.importWizard != wizard {
					return
				}
				wizard.Reading = false
				a.applyWizardArchiveInspectResult(wizard, path, inspectRes, preview, inspectErr)
			})
			return
		}
		workbook, inspectErr := importer.InspectWorkbook(path, opts)
		win.Update(func() {
			if a.importWizard != wizard {
				return
			}
			wizard.Reading = false
			a.applyWizardWorkbook(wizard, path, workbook, inspectErr)
		})
	}()
}

func (a *app) loadWizardFile(wizard *importWizardState, path string) {
	if wizard == nil || wizard.Reading {
		return
	}
	win := a.win
	st := a.store
	kind := wizard.Kind
	examID := wizard.ExamID
	examCohortID := wizard.ExamCohortID
	subjects := append([]store.Subject(nil), a.subjects...)
	opts := importer.Options{
		ConversionDir: filepath.Join(a.dataDir, "converted"),
		SofficePath:   a.sofficePath,
	}
	if win == nil {
		wizard.Error = ""
		if kind == importer.KindArchivedResults {
			inspectRes, preview, err := inspectAndValidateArchive(context.Background(), st, opts, subjects, examID, examCohortID, path)
			a.applyWizardArchiveInspectResult(wizard, path, inspectRes, preview, err)
			return
		}
		workbook, err := importer.InspectWorkbook(path, opts)
		a.applyWizardWorkbook(wizard, path, workbook, err)
		return
	}
	wizard.Reading = true
	wizard.Error = ""
	wizard.FilePath = path
	wizard.FileName = filepath.Base(path)
	go func() {
		if kind == importer.KindArchivedResults {
			inspectRes, preview, inspectErr := inspectAndValidateArchive(context.Background(), st, opts, subjects, examID, examCohortID, path)
			win.Update(func() {
				if a.importWizard != wizard {
					return
				}
				wizard.Reading = false
				a.applyWizardArchiveInspectResult(wizard, path, inspectRes, preview, inspectErr)
			})
			return
		}
		workbook, inspectErr := importer.InspectWorkbook(path, opts)
		win.Update(func() {
			if a.importWizard != wizard {
				return
			}
			wizard.Reading = false
			a.applyWizardWorkbook(wizard, path, workbook, inspectErr)
		})
	}()
}

func (a *app) applyWizardArchiveInspectResult(wizard *importWizardState, path string, inspectRes *importer.ArchiveInspectResult, preview store.ImportPreview, err error) {
	if wizard == nil {
		return
	}
	if err != nil && !errors.Is(err, errArchiveMissingStage) {
		wizard.Error = localizeErrorMessage(err.Error())
		return
	}
	wizard.Workbook = nil
	wizard.Sheet = ""
	wizard.Mapping = nil
	wizard.Data = store.ImportData{}
	wizard.Summary = nil
	wizard.ArchiveSummary = nil
	wizard.ConfirmedWarnings = false
	wizard.FilePath = path
	wizard.FileName = filepath.Base(path)
	wizard.ArchiveInspect = inspectRes
	if inspectRes != nil {
		wizard.Report = inspectRes.Report
		wizard.Issues = inspectRes.Issues
	}
	if errors.Is(err, errArchiveMissingStage) {
		wizard.Error = locales.ImportNoStage()
		return
	}
	wizard.Error = ""
	wizard.Preview = preview
	wizard.Issues = preview.Issues
	wizard.Step = wizardStepValidate
}

func (a *app) applyWizardWorkbook(wizard *importWizardState, path string, workbook *importer.Workbook, err error) {
	if wizard == nil {
		return
	}
	if err != nil {
		wizard.Error = err.Error()
		return
	}
	wizard.Workbook = workbook
	wizard.FilePath = path
	wizard.FileName = filepath.Base(path)
	wizard.Sheet = importer.BestSheet(wizard.Kind, workbook)
	wizard.Mapping = nil
	wizard.Data = store.ImportData{}
	wizard.Preview = store.ImportPreview{}
	wizard.Issues = nil
	wizard.Summary = nil
	wizard.ConfirmedWarnings = false
	if wizard.Sheet != "" {
		if table, ok := workbook.Sheet(wizard.Sheet); ok {
			wizard.Mapping = importer.AutoMappingFor(wizard.Kind, table, a.subjects, a.wizardTierKeys())
		}
	}
	wizard.Step = wizardStepSheet
}

func (a *app) wizardChooseSheet(sheet string) {
	wizard := a.importWizard
	if wizard == nil || wizard.Workbook == nil {
		return
	}
	wizard.Sheet = sheet
	table, ok := wizard.Workbook.Sheet(sheet)
	if !ok {
		wizard.Error = locales.ImportWizardSheetMissing()
		return
	}
	wizard.Mapping = importer.AutoMappingFor(wizard.Kind, table, a.subjects, a.wizardTierKeys())
	wizard.Error = ""
}

func (a *app) wizardParse() bool {
	wizard := a.importWizard
	if wizard == nil || wizard.Workbook == nil {
		return false
	}
	table, ok := wizard.Workbook.Sheet(wizard.Sheet)
	if !ok {
		wizard.Error = locales.ImportWizardSheetMissing()
		return false
	}
	data, issues, err := importer.ParseSheetForGrade(wizard.Workbook, wizard.Sheet, wizard.Kind,
		wizard.Mapping, a.subjects, a.wizardTierKeys(), a.wizardGrade())
	if err != nil {
		wizard.Error = err.Error()
		return false
	}
	wizard.Data = data
	wizard.Issues = issues
	wizard.Report = store.ImportReport{
		FileHash: wizard.Workbook.FileHash,
		Path:     wizard.FilePath,
		Format:   wizard.Workbook.Format,
		RowCount: len(table.Rows),
		Sheets: []store.ImportSheet{{
			Name: wizard.Sheet, Role: wizard.Kind,
			HeaderRow: table.HeaderRow, Rows: len(table.Rows),
		}},
	}
	if wizard.Workbook.ConvertedPath != "" {
		wizard.Report.ConvertedPath = wizard.Workbook.ConvertedPath
		wizard.Report.Conversion = wizard.Workbook.Conversion
	}
	wizard.Error = ""
	return true
}

func (a *app) wizardValidate() {
	wizard := a.importWizard
	if wizard == nil || !a.wizardParse() {
		return
	}
	var preview store.ImportPreview
	var err error
	if wizard.ExamID != 0 {
		preview, err = a.store.ValidateExamImport(context.Background(), store.ExamImportRequest{
			ExamID: wizard.ExamID, CohortID: wizard.ExamCohortID, Kind: examImportKind(wizard.Kind),
			Data: wizard.Data, Report: wizard.Report, Mapping: wizard.Mapping, Issues: wizard.Issues,
		})
	} else {
		cohort := a.selectedCohort()
		stage := a.selectedStage()
		if cohort == nil || stage == nil {
			wizard.Error = locales.ImportNoStage()
			return
		}
		preview, err = a.store.ValidateStageImport(context.Background(), cohort.ID, stage.ID,
			store.StageImportRequest{
				Kind: wizard.Kind, Data: wizard.Data, Report: wizard.Report,
				Mapping: wizard.Mapping, Issues: wizard.Issues,
			})
	}
	if err != nil {
		wizard.Error = err.Error()
		return
	}
	wizard.Preview = preview
	wizard.Issues = preview.Issues
	wizard.ConfirmedWarnings = false
	wizard.Error = ""
	wizard.Step = wizardStepValidate
}

// wizardZeroScoreCount 统计成绩数据里按未参加处理的单科 0 分数。
func wizardZeroScoreCount(wizard *importWizardState) int {
	count := 0
	for _, student := range wizard.Data.Students {
		for _, flag := range student.Flags {
			if flag == "zero" {
				count++
			}
		}
	}
	return count
}

// wizardUnknownRosterCount 统计报名名单之外的成绩行数。
func (a *app) wizardUnknownRosterCount(wizard *importWizardState) int {
	count := 0
	for _, issue := range wizard.Issues {
		if issue.Code == "unknown_roster" {
			count++
		}
	}
	return count
}

// wizardUnknownRosterEntries 从成绩数据里取出报名名单之外的考生。
func (a *app) wizardUnknownRosterEntries(wizard *importWizardState) []store.ImportStudent {
	unknown := map[string]bool{}
	for _, issue := range wizard.Issues {
		if issue.Code == "unknown_roster" && len(issue.Args) > 0 {
			unknown[issue.Args[0]] = true
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	entries := []store.ImportStudent{}
	for _, student := range wizard.Data.Students {
		if unknown[student.StudentNo] {
			entries = append(entries, student)
		}
	}
	return entries
}

// wizardSkipUnknownRoster 把报名名单之外的行从本次导入中移除并重新校验。
func (a *app) wizardSkipUnknownRoster(wizard *importWizardState) {
	entries := a.wizardUnknownRosterEntries(wizard)
	if len(entries) == 0 {
		return
	}
	unknown := map[string]bool{}
	for _, entry := range entries {
		unknown[entry.StudentNo] = true
	}
	kept := wizard.Data.Students[:0]
	for _, student := range wizard.Data.Students {
		if !unknown[student.StudentNo] {
			kept = append(kept, student)
		}
	}
	wizard.Data.Students = kept
	wizard.RosterSkipped = len(entries)
	a.wizardValidate()
}

// wizardAddUnknownRoster 把报名名单之外的考生补建并加入本场考试报名名单，然后重新校验。
func (a *app) wizardAddUnknownRoster(wizard *importWizardState) {
	entries := a.wizardUnknownRosterEntries(wizard)
	if len(entries) == 0 {
		return
	}
	exam, err := a.store.GetExamCohort(context.Background(), wizard.ExamID, wizard.ExamCohortID)
	if err != nil {
		wizard.Error = err.Error()
		return
	}
	if _, _, err := a.store.AddExamRosterStudents(context.Background(), exam, entries); err != nil {
		wizard.Error = err.Error()
		return
	}
	a.reloadExamData(context.Background())
	a.wizardValidate()
}

// wizardExportIssues 把当前问题清单保存为 Excel。
func (a *app) wizardExportIssues(wizard *importWizardState) {
	if len(wizard.Issues) == 0 {
		return
	}
	win := a.win
	title := locales.ImportWizardExportIssues()
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       title,
			DefaultPath: "tally-import-issues.xlsx",
			Filters:     []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx"}}},
		})
		if err != nil || path == "" {
			return
		}
		if win == nil {
			a.writeWizardIssues(wizard, path)
			return
		}
		win.Update(func() {
			if a.importWizard != wizard {
				return
			}
			a.writeWizardIssues(wizard, path)
		})
	}()
}

func (a *app) writeWizardIssues(wizard *importWizardState, path string) {
	header := []string{
		locales.ImportIssueColItem(), locales.ImportIssueColFile(), locales.ImportIssueColSheet(),
		locales.ImportIssueColRow(), locales.ImportIssueColColumn(), locales.ImportIssueColSeverity(),
		locales.ImportIssueColIssue(),
	}
	rows := make([][]string, 0, len(wizard.Issues))
	for _, issue := range wizard.Issues {
		severity := locales.SeverityWarning()
		if issue.Severity == "blocking" {
			severity = locales.SeverityBlocking()
		}
		row := ""
		if issue.Row > 0 {
			row = strconv.Itoa(issue.Row)
		}
		rows = append(rows, []string{
			importKindName(wizard.Kind), wizard.FileName, wizard.Sheet, row,
			a.importColumnName(issue.Column), severity, a.importIssueText(issue),
		})
	}
	if err := importer.WriteIssueList(path, header, rows); err != nil {
		wizard.Error = err.Error()
		return
	}
	a.notifyMain(fmt.Sprintf(locales.ImportWizardIssuesExported(), filepath.Base(path)))
}
func (a *app) wizardApply() {
	wizard := a.importWizard
	if wizard == nil || wizard.Preview.Blocking > 0 ||
		(wizard.Preview.Warnings > 0 && !wizard.ConfirmedWarnings) || wizard.Busy {
		return
	}
	wizard.Busy = true
	if wizard.Kind == importer.KindArchivedResults {
		a.wizardApplyArchive(wizard)
		return
	}
	win := a.win
	st := a.store
	filePath := wizard.FilePath
	report := wizard.Report
	examID := wizard.ExamID
	examCohortID := wizard.ExamCohortID
	kind := wizard.Kind
	data := wizard.Data
	mapping := wizard.Mapping
	issues := wizard.Issues
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	noStageMsg := locales.ImportNoStage()

	work := func() (store.ImportSummary, string, error) {
		stored, err := st.StoreImportFile(filePath, report.FileHash)
		if err != nil {
			return store.ImportSummary{}, "", err
		}
		reqReport := report
		reqReport.StoredPath = stored
		if examID != 0 {
			sum, applyErr := st.ApplyExamImport(context.Background(), store.ExamImportRequest{
				ExamID: examID, CohortID: examCohortID, Kind: examImportKind(kind),
				Data: data, Report: reqReport, Mapping: mapping, Issues: issues,
			})
			return sum, stored, applyErr
		}
		if cohort == nil || stage == nil {
			return store.ImportSummary{}, stored, errors.New(noStageMsg)
		}
		sum, applyErr := st.ApplyStageImport(context.Background(), cohort.ID, stage.ID,
			store.StageImportRequest{
				Kind: kind, Data: data, Report: reqReport,
				Mapping: mapping, Issues: issues,
			})
		return sum, stored, applyErr
	}
	finish := func(summary store.ImportSummary, stored string, err error) {
		wizard.Busy = false
		if stored != "" {
			wizard.Report.StoredPath = stored
		}
		if errors.Is(err, store.ErrDuplicateImport) {
			wizard.Error = locales.ImportWizardAlreadyImported()
			return
		}
		if err != nil {
			wizard.Error = err.Error()
			return
		}
		wizard.Summary = &summary
		wizard.Error = ""
		wizard.Step = wizardStepResult
		ctx := context.Background()
		a.reloadCohorts(ctx)
		if examID != 0 {
			a.reloadExamData(ctx)
		} else {
			a.reloadDataPages(ctx)
		}
		a.reloadExams(ctx)
		a.reloadAssessments(ctx)
		a.reloadWorkspaceSummary(ctx)
	}
	if win == nil {
		summary, stored, err := work()
		finish(summary, stored, err)
		return
	}
	go func() {
		summary, stored, err := work()
		win.Update(func() {
			if a.importWizard != wizard {
				return
			}
			finish(summary, stored, err)
		})
	}()
}

func (a *app) wizardApplyArchive(wizard *importWizardState) {
	win := a.win
	st := a.store
	inspect := wizard.ArchiveInspect
	filePath := wizard.FilePath
	examID := wizard.ExamID
	examCohortID := wizard.ExamCohortID
	noStageMsg := locales.ImportNoStage()

	work := func() (store.ArchivedImportSummary, error) {
		if inspect == nil || st == nil {
			return store.ArchivedImportSummary{}, errors.New(noStageMsg)
		}
		report := inspect.Report
		stored, err := st.StoreImportFile(filePath, report.FileHash)
		if err != nil {
			return store.ArchivedImportSummary{}, err
		}
		report.StoredPath = stored
		ctx := context.Background()
		ec, err := st.GetExamCohort(ctx, examID, examCohortID)
		if err != nil {
			return store.ArchivedImportSummary{}, err
		}
		assess, err := st.AssessmentForExamCohort(ctx, ec.ID)
		if err != nil {
			return store.ArchivedImportSummary{}, err
		}
		return st.ApplyArchivedResultImport(ctx, store.ArchivedResultImportRequest{
			AssessmentID: assess.ID,
			Data:         inspect.Data,
			Report:       report,
			Issues:       inspect.Issues,
		})
	}
	finish := func(summary store.ArchivedImportSummary, err error) {
		wizard.Busy = false
		if errors.Is(err, store.ErrDuplicateImport) {
			wizard.Error = locales.ImportWizardAlreadyImported()
			return
		}
		if err != nil {
			wizard.Error = localizeErrorMessage(err.Error())
			return
		}
		wizard.ArchiveSummary = &summary
		wizard.Error = ""
		wizard.Step = wizardStepResult
		ctx := context.Background()
		a.reloadCohorts(ctx)
		if examID != 0 {
			a.reloadExamData(ctx)
		}
		a.reloadExams(ctx)
		a.reloadAssessments(ctx)
		a.reloadWorkspaceSummary(ctx)
	}
	if win == nil {
		summary, err := work()
		finish(summary, err)
		return
	}
	go func() {
		summary, err := work()
		win.Update(func() {
			if a.importWizard != wizard {
				return
			}
			finish(summary, err)
		})
	}()
}

func (a *app) wizardDownloadTemplate() {
	wizard := a.importWizard
	if wizard == nil {
		return
	}
	win := a.win
	title := locales.ImportWizardDownloadTemplate()
	defaultPath := "tally-" + wizard.Kind + ".xlsx"
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       title,
			DefaultPath: defaultPath,
			Filters:     []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx"}}},
		})
		if err != nil || path == "" {
			return
		}
		if win == nil {
			a.writeWizardTemplate(wizard, path)
			return
		}
		win.Update(func() {
			if a.importWizard != wizard {
				return
			}
			a.writeWizardTemplate(wizard, path)
		})
	}()
}

func (a *app) writeWizardTemplate(wizard *importWizardState, path string) {
	if wizard == nil {
		return
	}
	if err := importer.WriteTemplate(wizard.Kind, a.subjects, path); err != nil {
		wizard.Error = err.Error()
		return
	}
	wizard.Error = ""
	a.notifyMain(fmt.Sprintf(locales.ImportWizardTemplateSaved(), filepath.Base(path)))
}

func (a *app) importWizardPage(c *ui.Context, p uiPalette) {
	wizard := a.importWizard
	if wizard == nil {
		return
	}
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			if backButton(c, p).Clicked() {
				if a.importReturn != nil {
					a.returnFromImport()
					return
				}
				a.closeImportWizard()
			}
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Text(c, locales.ImportWizardTitle()).FontSize(titleSize(t)).FontWeight(600).
					Level(1).Role(ui.RoleHeading)
				if wizard.Kind != "" {
					ui.Text(c, importKindName(wizard.Kind)).FontSize(bodySize(t)).TextColor(p.muted)
				}
			})
		})
		a.wizardSteps(c, p, wizard.Step)
		if wizard.Error != "" {
			notice(c, p, noticeDanger, wizard.Error)
		}
		if showDelayedBusy(c, "wizard-reading", wizard.Reading) {
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Spinner(c).Label(locales.ImportWizardReadingFile())
				ui.Text(c, locales.ImportWizardReadingFile()).FontSize(bodySize(t)).TextColor(p.muted)
			})
		}
		if showDelayedBusy(c, "wizard-busy", wizard.Busy) {
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Spinner(c).Label(locales.ImportBusy())
				ui.Text(c, locales.ImportBusy()).FontSize(bodySize(t)).TextColor(p.muted)
			})
		}
		switch wizard.Step {
		case wizardStepItem:
			a.wizardItemStep(c, p)
		case wizardStepFile:
			a.wizardFileStep(c, p, wizard)
		case wizardStepSheet:
			a.wizardSheetStep(c, p, wizard)
		case wizardStepMapping:
			a.wizardMappingStep(c, p, wizard)
		case wizardStepValidate:
			a.wizardValidateStep(c, p, wizard)
		default:
			a.wizardResultStep(c, p, wizard)
		}
	})
}

func (a *app) wizardSteps(c *ui.Context, p uiPalette, step int) {
	if a.importWizard != nil && a.importWizard.Kind == importer.KindArchivedResults {
		labels := []string{
			locales.ImportStepItem(), locales.ImportStepFile(),
			locales.ImportStepValidate(), locales.ImportStepResult(),
		}
		mapped := step
		switch step {
		case wizardStepValidate:
			mapped = 2
		case wizardStepResult:
			mapped = 3
		}
		stepsBar(c, p, labels, mapped)
		return
	}
	labels := []string{
		locales.ImportStepItem(), locales.ImportStepFile(), locales.ImportStepSheet(),
		locales.ImportStepMapping(), locales.ImportStepValidate(), locales.ImportStepResult(),
	}
	stepsBar(c, p, labels, step)
}

func (a *app) wizardItemStep(c *ui.Context, p uiPalette) {
	t := p.theme
	kinds := []string{
		importer.KindClasses, importer.KindStudents, importer.KindTeachers,
		importer.KindAssignments, importer.KindBaselines,
	}
	if a.wizardExam() {
		kinds = []string{importer.KindRoster, importer.KindScores,
			importer.KindCutoffs, importer.KindTargets, importer.KindArchivedResults}
	}
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
			if a.wizardExam() {
				if exam := a.selectedExam(); exam != nil {
					ui.Text(c, exam.Name).FontSize(captionSize(t)).TextColor(p.muted)
				}
			} else if cohort := a.selectedCohort(); cohort != nil {
				stage := a.displayStage()
				ui.Textf(c, locales.CohortContext(), a.cohortName(cohort.EnrollmentYear),
					a.stageName(stage.GradeLevel), a.classOfName(cohort.GraduationYear)).
					FontSize(captionSize(t)).TextColor(p.muted)
			}
			for _, kind := range kinds {
				kind := kind
				rule(c)
				ui.Column(c).FillWidth().Padding(t.Space(3), 0).Gap(t.Space(1)).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Column(c).Grow(1).Gap(0).Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, importKindName(kind)).FontSize(bodySize(t)).FontWeight(600)
								if a.importKindCount(kind) > 0 {
									badge(c, p, locales.ReadinessReady(), toneNeutral)
								} else {
									badge(c, p, locales.ReadinessMissing(), toneNeutral)
								}
							})
							ui.Text(c, importKindPurpose(kind)).FontSize(captionSize(t)).
								TextColor(p.muted).Pretty()
						})
						if labelledButton(c, locales.PrepImport(), iconArrowRight, false).Clicked() {
							a.wizardSelectKind(kind)
						}
					})
				})
			}
		})
	})
}

func (a *app) wizardFileStep(c *ui.Context, p uiPalette, wizard *importWizardState) {
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Text(c, importKindPurpose(wizard.Kind)).FontSize(bodySize(t)).
			TextColor(p.inkSecondary).Pretty()
		if cols := a.importKindColumns(wizard.Kind); cols != "" {
			ui.Text(c, cols).FontSize(captionSize(t)).
				TextColor(p.muted).Pretty()
		}
		clicked, dropped := fileDropZone(c, p, fileDropZoneProps{
			Key:         "wizard-file-drop",
			Title:       locales.ImportCenterDropTitle(),
			ActiveTitle: locales.ImportCenterDropActive(),
			Hint:        locales.ImportCenterDropHint(),
			ButtonLabel: locales.ImportCenterBrowseFile(),
		})
		if len(dropped) > 0 && !wizard.Reading {
			a.loadWizardFile(wizard, dropped[0])
		} else if clicked && !wizard.Reading {
			a.wizardChooseFile()
		}
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			choose := labelledButton(c, locales.ImportWizardChooseFile(), iconImports, true)
			choose.Disabled(wizard.Reading)
			if choose.Clicked() {
				a.wizardChooseFile()
			}
			if wizard.Kind != importer.KindArchivedResults {
				if plainButton(c, locales.ImportWizardDownloadTemplate()).Clicked() {
					a.wizardDownloadTemplate()
				}
			}
		})
		if wizard.FileName != "" {
			shown := fmt.Sprintf(locales.ImportWizardSelectedFile(), wizard.FileName)
			ui.Text(c, shown).FontSize(captionSize(t)).TextColor(p.muted).
				SingleLine().TruncateMiddle().MinWidth(0).Tooltip(wizard.FilePath)
		}
	})
}

func (a *app) wizardSheetStep(c *ui.Context, p uiPalette, wizard *importWizardState) {
	t := p.theme
	if wizard.Workbook == nil {
		a.wizardFileStep(c, p, wizard)
		return
	}
	names := wizard.Workbook.SheetNames()
	chosen := wizard.Sheet
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Text(c, locales.ImportWizardSheetHint()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
		next := chosen
		selectField(c, "wizard-sheet", &next, names, locales.ImportWizardSheetLabel()).Width(t.Space(72))
		if next != chosen {
			a.wizardChooseSheet(next)
		}
		if table, ok := wizard.Workbook.Sheet(wizard.Sheet); ok {
			ui.Textf(c, locales.ImportWizardSheetRows(), formatInt(len(table.Rows)), formatInt(len(table.Headers))).
				FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
			headers := table.Headers
			if len(headers) > 12 {
				headers = headers[:12]
			}
			ui.Text(c, strings.Join(headers, locales.ListSeparator())).FontSize(captionSize(t)).
				TextColor(p.inkSecondary)
		}
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			button := labelledButton(c, locales.Next(), iconArrowRight, true)
			button.Disabled(wizard.Sheet == "")
			if button.Clicked() {
				wizard.Step = wizardStepMapping
			}
		})
	})
}

func (a *app) wizardMappingStep(c *ui.Context, p uiPalette, wizard *importWizardState) {
	if wizard.Workbook == nil {
		wizard.Step = wizardStepFile
		return
	}
	t := p.theme
	table, ok := wizard.Workbook.Sheet(wizard.Sheet)
	if !ok {
		wizard.Error = locales.ImportWizardSheetMissing()
		return
	}
	if wizard.Mapping == nil {
		wizard.Mapping = importer.AutoMappingFor(wizard.Kind, table, a.subjects, a.wizardTierKeys())
	}
	fields := a.wizardFields()
	missing := 0
	for _, field := range fields {
		if field.Required && wizard.Mapping[field.Key] == "" {
			missing++
		}
	}
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
			if len(fields) == 0 {
				ui.Text(c, locales.ImportWizardAutoColumns()).FontSize(bodySize(t)).
					TextColor(p.muted).Pretty()
			}
			for _, field := range fields {
				field := field
				ui.Row(c).FillWidth().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
					label := a.importFieldLabel(field.Key)
					if field.Required {
						label += " *"
					}
					ui.Column(c).Width(t.Space(40)).Gap(0).Children(func() {
						ui.Text(c, label).FontSize(bodySize(t)).SingleLine()
						ui.Text(c, importFieldSample(field.Key, table, wizard.Mapping, a.subjects)).
							FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
					})
					labels := append([]string{locales.ImportWizardUnmapped()}, table.Headers...)
					chosen := wizard.Mapping[field.Key]
					if chosen == "" {
						chosen = labels[0]
					}
					next := chosen
					selectField(c, "wizard-map-"+field.Key, &next, labels, label).Grow(1)
					if next != chosen {
						if next == labels[0] {
							delete(wizard.Mapping, field.Key)
						} else {
							wizard.Mapping[field.Key] = next
						}
					}
				})
			}
			if missing > 0 {
				notice(c, p, noticeWarning, fmt.Sprintf(locales.ImportWizardMissingColumns(), formatInt(missing)))
			}
			ui.Row(c).Gap(t.Space(2)).Children(func() {
				button := labelledButton(c, locales.ImportWizardCheck(), iconArrowRight, true)
				button.Disabled(missing > 0)
				if button.Clicked() {
					a.wizardValidate()
				}
			})
		})
	})
}

func (a *app) wizardValidateStep(c *ui.Context, p uiPalette, wizard *importWizardState) {
	t := p.theme
	panel(c, p).FillWidth().Grow(1).Shrink(1).MinHeight(0).Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		if wizard.Kind == importer.KindArchivedResults && wizard.ArchiveInspect != nil {
			a.archivePreviewPanel(c, p, wizard.ArchiveInspect)
		}
		ui.Row(c).Gap(t.Space(2)).Wrap().Children(func() {
			badge(c, p, fmt.Sprintf(locales.ImportWizardNew(), formatInt(wizard.Preview.New)), toneAccent)
			badge(c, p, fmt.Sprintf(locales.ImportWizardUpdate(), formatInt(wizard.Preview.Update)), toneSuccess)
			badge(c, p, fmt.Sprintf(locales.ImportWizardSkip(), formatInt(wizard.Preview.Skip)), toneNeutral)
			badge(c, p, fmt.Sprintf(locales.ImportWizardBlocking(), formatInt(wizard.Preview.Blocking)), toneDanger)
			badge(c, p, fmt.Sprintf(locales.ImportWizardWarnings(), formatInt(wizard.Preview.Warnings)), toneWarning)
		})
		if zero := wizardZeroScoreCount(wizard); zero > 0 {
			ui.Textf(c, locales.ImportWizardZeroScores(), zero).FontSize(captionSize(t)).
				TextColor(p.muted).Pretty()
		}
		if wizard.RosterSkipped > 0 {
			ui.Textf(c, locales.ImportWizardRosterSkipped(), wizard.RosterSkipped).FontSize(captionSize(t)).
				TextColor(p.muted).Pretty()
		}
		if len(wizard.Issues) == 0 {
			smallEmptyState(c, p, locales.ImportWizardNoIssues())
		} else {
			a.wizardIssuesList.Key = func(row int) any {
				issue := wizard.Issues[row]
				return fmt.Sprintf("%d:%s:%d:%s", row, issue.Code, issue.Row, issue.Column)
			}
			ui.List(c, &a.wizardIssuesList, len(wizard.Issues), func(row int) {
				issue := wizard.Issues[row]
				tone := p.warning
				if issue.Severity == "blocking" {
					tone = p.danger
				}
				ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(t.Space(2)).Padding(t.Space(1), 0).Children(func() {
					ui.Box(c).Width(t.Space(1)).Height(t.Space(4)).Radius(t.Space(1)).
						Shrink(0).Background(tone)
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(0).Children(func() {
						ui.Text(c, a.importIssueText(issue)).FontSize(bodySize(t)).Pretty()
						if issue.Row > 0 {
							ui.Textf(c, locales.ImportIssueLocation(), issue.Row,
								a.importColumnName(issue.Column)).FontSize(captionSize(t)).
								TextColor(p.muted).FontFeatures("tnum")
						}
					})
				})
			}).Label(locales.ImportStepValidate()).Grow(1).Shrink(1).MinHeight(0).Fill()
			if wizard.Kind == importer.KindScores && a.wizardUnknownRosterCount(wizard) > 0 {
				ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
					ui.Row(c).Gap(t.Space(2)).Wrap().Children(func() {
						if plainButton(c, fmt.Sprintf(locales.ImportWizardAddToRoster(),
							formatInt(a.wizardUnknownRosterCount(wizard)))).Clicked() {
							a.wizardAddUnknownRoster(wizard)
						}
						if plainButton(c, fmt.Sprintf(locales.ImportWizardSkipRows(),
							formatInt(a.wizardUnknownRosterCount(wizard)))).Clicked() {
							a.wizardSkipUnknownRoster(wizard)
						}
					})
				})
			}
		}
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			if wizard.Kind == importer.KindArchivedResults {
				if plainButton(c, locales.Back()).Clicked() {
					wizard.Step = wizardStepFile
				}
			} else if plainButton(c, locales.ImportWizardBackToMapping()).Clicked() {
				wizard.Step = wizardStepMapping
			}
			if len(wizard.Issues) > 0 {
				if plainButton(c, locales.ImportWizardExportIssues()).Clicked() {
					a.wizardExportIssues(wizard)
				}
			}
			if wizard.Preview.Warnings > 0 {
				checkboxField(c, &wizard.ConfirmedWarnings,
					fmt.Sprintf(locales.ImportWizardConfirmWarnings(), formatInt(wizard.Preview.Warnings)))
			}
			importCount := wizard.Preview.New + wizard.Preview.Update
			button := labelledButton(c,
				fmt.Sprintf(locales.ImportWizardImportRows(), formatInt(importCount)),
				iconCheck, true)
			button.Disabled(wizard.Preview.Blocking > 0 || wizard.Busy ||
				(wizard.Preview.Warnings > 0 && !wizard.ConfirmedWarnings))
			if button.Clicked() {
				a.wizardApply()
			}
		})
	})
}

func archiveSheetRoleLabel(role string) string {
	switch role {
	case importer.ArchiveRoleQuant:
		return locales.ArchiveSheetRoleQuant()
	case importer.ArchiveRoleRank:
		return locales.ArchiveSheetRoleRank()
	case importer.ArchiveRolePersonal:
		return locales.ArchiveSheetRolePersonal()
	default:
		return locales.ArchiveSheetRoleOther()
	}
}

func (a *app) archivePreviewPanel(c *ui.Context, p uiPalette, inspect *importer.ArchiveInspectResult) {
	t := p.theme
	matchedSheets := 0
	for _, s := range inspect.Sheets {
		if s.Role != "" && s.Recommended {
			matchedSheets++
		}
	}
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Wrap().Gap(t.Space(2)).Children(func() {
			ui.Text(c, locales.ArchivePreviewTitle()).FontSize(bodySize(t)).FontWeight(600)
			badge(c, p, fmt.Sprintf(locales.ArchivePreviewSheetsBadge(), formatInt(matchedSheets)), toneAccent)
			badge(c, p, fmt.Sprintf(locales.ArchivePreviewResultRowsBadge(), formatInt(len(inspect.Data.Rows))), toneNeutral)
			badge(c, p, fmt.Sprintf(locales.ArchivePreviewPersonalBadge(), formatInt(len(inspect.Data.Personal))), toneNeutral)
		})
		for _, s := range inspect.Sheets {
			if s.Role == "" || !s.Recommended {
				continue
			}
			fmtLabel := locales.ArchiveFormatLong()
			if s.Format == "wide" {
				fmtLabel = locales.ArchiveFormatWide()
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				badge(c, p, archiveSheetRoleLabel(s.Role), toneSuccess)
				ui.Textf(c, locales.ArchivePreviewSheetMeta(), s.Name, fmtLabel, formatInt(s.RowCount)).
					FontSize(captionSize(t)).TextColor(p.inkSecondary).FontFeatures("tnum")
			})
		}
		if len(inspect.Data.Rows) > 0 {
			ui.Text(c, locales.ArchivePreviewRowTitle()).FontSize(captionSize(t)).FontWeight(600).TextColor(p.muted)
			limit := min(len(inspect.Data.Rows), 4)
			for i := 0; i < limit; i++ {
				r := inspect.Data.Rows[i]
				tierText := "-"
				if r.TierIndex != nil {
					tierText = fmt.Sprintf("%d", *r.TierIndex+1)
				}
				line := fmt.Sprintf("%s · %s · %.2f · #%d · %s · %.2f",
					r.ClassName, a.subjectName(r.Subject, r.Subject), r.Total, r.Rank, tierText, r.Points)
				ui.Text(c, line).FontSize(captionSize(t)).TextColor(p.inkSecondary).FontFeatures("tnum").SingleLine()
			}
		}
		if len(inspect.Data.Personal) > 0 {
			ui.Text(c, locales.ArchivePreviewPersonalTitle()).FontSize(captionSize(t)).FontWeight(600).TextColor(p.muted)
			limit := min(len(inspect.Data.Personal), 4)
			for i := 0; i < limit; i++ {
				ps := inspect.Data.Personal[i]
				line := fmt.Sprintf("%s · %s · %.2f",
					ps.TeacherName, a.subjectName(ps.Subject, ps.Subject), ps.Score)
				ui.Text(c, line).FontSize(captionSize(t)).TextColor(p.inkSecondary).FontFeatures("tnum").SingleLine()
			}
		}
	})
}

func (a *app) wizardResultStep(c *ui.Context, p uiPalette, wizard *importWizardState) {
	t := p.theme
	summary := wizard.Summary
	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Box(c.Key("wizard-result-icon")).Transition(wizardDoneEnter).Children(func() {
				ui.Icon(c, iconCheck).FontSize(t.Space(5)).TextColor(p.success)
			})
			ui.Text(c, locales.ImportWizardFinished()).FontSize(sectionSize(t)).FontWeight(600)
		})
		if wizard.ArchiveSummary != nil {
			ui.Textf(c, locales.ImportResultArchived(),
				formatInt(wizard.ArchiveSummary.ResultRows),
				formatInt(wizard.ArchiveSummary.PersonalScores)).
				FontSize(bodySize(t)).Pretty()
		} else if summary != nil {
			ui.Text(c, importResultText(wizard.Kind, summary)).FontSize(bodySize(t)).Pretty()
		}
		ui.Row(c).Wrap().Gap(t.Space(2)).Children(func() {
			if a.importReturn != nil {
				retText := fmt.Sprintf(locales.ImportCenterDoneReturn(), a.importReturn.Label)
				if labelledButton(c, retText, iconArrowLeft, true).Clicked() {
					a.returnFromImport()
					return
				}
			}
			if plainButton(c, locales.ImportWizardViewData()).Clicked() {
				a.importReturn = nil
				if wizard.Kind == importer.KindArchivedResults && a.examAssessment.ID != 0 {
					a.selectAssessment(a.examAssessment.ID)
				}
				a.page = importKindPage(wizard.Kind)
				a.closeImportWizard()
			}
			if next := nextImportKind(wizard.Kind); next != "" {
				if labelledButton(c, fmt.Sprintf(locales.ImportWizardNextKind(), importKindName(next)),
					iconArrowRight, a.importReturn == nil).Clicked() {
					a.wizardContinueKind(next)
				}
			}
			if plainButton(c, locales.ImportWizardDone()).Clicked() {
				if a.importReturn != nil {
					a.returnFromImport()
					return
				}
				a.closeImportWizard()
			}
		})
	})
}

func importKindName(kind string) string {
	switch kind {
	case importer.KindClasses:
		return locales.ImportKindClasses()
	case importer.KindStudents:
		return locales.ImportKindStudents()
	case importer.KindTeachers:
		return locales.ImportKindTeachers()
	case importer.KindAssignments:
		return locales.ImportKindAssignments()
	case importer.KindBaselines:
		return locales.ImportKindBaselines()
	case importer.KindRoster:
		return locales.ExamKindRoster()
	case importer.KindScores:
		return locales.ExamKindScores()
	case importer.KindCutoffs:
		return locales.ExamKindCutoffs()
	case importer.KindTargets:
		return locales.ExamKindTargets()
	case importer.KindArchivedResults, store.ExamImportArchivedResults:
		return locales.ExamKindArchivedResults()
	case store.ExamImportRoster, store.ExamImportScores, store.ExamImportCutoffs, store.ExamImportTargets:
		switch kind {
		case store.ExamImportRoster:
			return locales.ExamKindRoster()
		case store.ExamImportScores:
			return locales.ExamKindScores()
		case store.ExamImportCutoffs:
			return locales.ExamKindCutoffs()
		default:
			return locales.ExamKindTargets()
		}
	default:
		return kind
	}
}

func (a *app) importKindCount(kind string) int {
	switch kind {
	case importer.KindClasses:
		return a.stageCounts.Classes
	case importer.KindStudents:
		return a.stageCounts.Students
	case importer.KindTeachers:
		return a.stageCounts.Teachers
	case importer.KindAssignments:
		return a.stageCounts.Assignments
	case importer.KindBaselines:
		return a.stageCounts.Baselines
	case importer.KindRoster:
		return a.examCohort.RosterCount
	case importer.KindScores:
		return a.examCohort.ScoreCount
	case importer.KindCutoffs:
		return len(a.examSubjectCutoffs) + len(a.examTierCutoffs)
	case importer.KindTargets:
		return a.examCohort.ClassTargetCount
	case importer.KindArchivedResults:
		if a.examAssessment.ID != 0 && a.examAssessment.ExamCohortID == a.examCohort.ID && a.examAssessment.Archived {
			return 1
		}
		if a.importCenterExamID != 0 {
			if cohort := a.selectedCohort(); cohort != nil {
				for _, ov := range a.assessments {
					if ov.ExamID == a.importCenterExamID && ov.CohortID == cohort.ID && ov.Archived {
						return 1
					}
				}
			}
		}
		return 0
	default:
		return 0
	}
}

func importKindPurpose(kind string) string {
	switch kind {
	case importer.KindClasses:
		return locales.ImportKindClassesPurpose()
	case importer.KindStudents:
		return locales.ImportKindStudentsPurpose()
	case importer.KindTeachers:
		return locales.ImportKindTeachersPurpose()
	case importer.KindAssignments:
		return locales.ImportKindAssignmentsPurpose()
	case importer.KindBaselines:
		return locales.ImportKindBaselinesPurpose()
	case importer.KindRoster:
		return locales.ExamKindRosterPurpose()
	case importer.KindScores:
		return locales.ExamKindScoresPurpose()
	case importer.KindCutoffs:
		return locales.ExamKindCutoffsPurpose()
	case importer.KindTargets:
		return locales.ExamKindTargetsPurpose()
	case importer.KindArchivedResults:
		return locales.ExamKindArchivedResultsPurpose()
	default:
		return ""
	}
}

func importKindPage(kind string) pageID {
	switch kind {
	case importer.KindClasses:
		return pageCohorts
	case importer.KindStudents:
		return pageStudents
	case importer.KindTeachers, importer.KindAssignments:
		return pageFaculty
	case importer.KindBaselines:
		return pageBaseline
	case importer.KindRoster, importer.KindScores, importer.KindCutoffs, importer.KindTargets:
		return pageExams
	case importer.KindArchivedResults:
		return pageAssessments
	default:
		return pageImports
	}
}

func nextImportKind(kind string) string {
	switch kind {
	case importer.KindClasses:
		return importer.KindStudents
	case importer.KindStudents:
		return importer.KindAssignments
	case importer.KindTeachers:
		return importer.KindAssignments
	case importer.KindAssignments:
		return importer.KindBaselines
	case importer.KindRoster:
		return importer.KindScores
	case importer.KindScores:
		return importer.KindCutoffs
	case importer.KindCutoffs:
		return importer.KindTargets
	default:
		return ""
	}
}

func examImportKind(kind string) string {
	switch kind {
	case importer.KindRoster:
		return store.ExamImportRoster
	case importer.KindScores:
		return store.ExamImportScores
	case importer.KindCutoffs:
		return store.ExamImportCutoffs
	case importer.KindTargets:
		return store.ExamImportTargets
	case importer.KindArchivedResults:
		return store.ExamImportArchivedResults
	default:
		return ""
	}
}

func (a *app) importKindColumns(kind string) string {
	names := make([]string, 0)
	for _, field := range importer.FieldsFor(kind, a.subjects, a.wizardTierKeys()) {
		if field.Required {
			names = append(names, a.importFieldLabel(field.Key))
		}
	}
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf(locales.ImportWizardRequiredColumns(), strings.Join(names, locales.ListSeparator()))
}

func (a *app) importFieldLabel(key string) string {
	switch key {
	case importer.FieldClassName:
		return locales.ImportFieldClassName()
	case importer.FieldStudentNo:
		return locales.ImportFieldStudentNo()
	case importer.FieldName:
		return locales.ImportFieldName()
	case importer.FieldTrack:
		return locales.ImportFieldTrack()
	case importer.FieldElective:
		return locales.ImportFieldElective()
	case importer.FieldStatus:
		return locales.ImportFieldStatus()
	case importer.FieldTotalScore:
		return locales.ImportFieldTotalScore()
	case importer.FieldWalking:
		return locales.ImportFieldWalking()
	case importer.FieldJapanese:
		return locales.ImportFieldJapanese()
	case importer.FieldTeacherName:
		return locales.ImportFieldTeacherName()
	case importer.FieldTeacherNo:
		return locales.ImportFieldTeacherNo()
	case importer.FieldSubject:
		return locales.ImportFieldSubject()
	case importer.FieldExamNo:
		return locales.ImportFieldExamNo()
	case importer.FieldCandidateNo:
		return locales.ImportFieldCandidateNo()
	case importer.FieldCutoffSubject:
		return locales.ImportFieldCutoffSubject()
	case importer.FieldCutoffTrack:
		return locales.ImportFieldCutoffTrack()
	case importer.FieldCutoffTier:
		return locales.ImportFieldCutoffTier()
	case importer.FieldCutoffValue:
		return locales.ImportFieldCutoffValue()
	case "tier":
		return locales.ImportFieldTier()
	case "points":
		return locales.ImportFieldPoints()
	case "rank":
		return locales.ImportFieldRank()
	case "quant_total_score":
		return locales.ImportFieldQuantTotalScore()
	case "personal_score":
		return locales.ImportFieldPersonalScore()
	case "stat_group":
		return locales.ImportFieldStatGroup()
	case "class_count":
		return locales.ImportFieldClassCount()
	case "class_average":
		return locales.ImportFieldClassAverage()
	case "grade_average":
		return locales.ImportFieldGradeAverage()
	case "baseline_average":
		return locales.ImportFieldBaselineAverage()
	case "baseline_ratio":
		return locales.ImportFieldBaselineRatio()
	case "improvement_raw":
		return locales.ImportFieldImprovementRaw()
	case "improvement_score":
		return locales.ImportFieldImprovementScore()
	case "pass_count":
		return locales.ImportFieldPassCount()
	case "pass_target":
		return locales.ImportFieldPassTarget()
	case "pass_raw":
		return locales.ImportFieldPassRaw()
	case "pass_score":
		return locales.ImportFieldPassScore()
	case "completion_score":
		return locales.ImportFieldCompletionScore()
	case "group_points":
		return locales.ImportFieldGroupPoints()
	}
	if strings.HasPrefix(key, "subject:") {
		subject := strings.TrimPrefix(key, "subject:")
		return a.subjectName(subject, subject)
	}
	if strings.HasPrefix(key, "ratio:") {
		subject := strings.TrimPrefix(key, "ratio:")
		return fmt.Sprintf(locales.ImportFieldRatio(), a.subjectName(subject, subject))
	}
	if strings.HasPrefix(key, "score:") {
		subject := strings.TrimPrefix(key, "score:")
		return a.subjectName(subject, subject)
	}
	if strings.HasPrefix(key, "tier:") {
		return a.tierName(strings.TrimPrefix(key, "tier:"))
	}
	if key == "ratio" {
		return locales.ImportFieldRatioGeneric()
	}
	return key
}

func (a *app) importColumnName(column string) string {
	return a.importFieldLabel(column)
}

func importFieldSample(key string, table excel.Table, mapping map[string]string, subjects []store.Subject) string {
	header := mapping[key]
	if header == "" {
		return ""
	}
	index := -1
	for position, value := range table.Headers {
		if value == header {
			index = position
			break
		}
	}
	if index < 0 || len(table.Rows) == 0 || index >= len(table.Rows[0].Values) {
		return ""
	}
	return table.Rows[0].Values[index]
}

func (a *app) importIssueText(issue store.ImportIssue) string {
	arg := func(index int) string {
		if index < len(issue.Args) {
			return issue.Args[index]
		}
		return ""
	}
	numberArg := func(index int) int {
		value, err := strconv.Atoi(arg(index))
		if err != nil {
			return 0
		}
		return value
	}
	subjectOrTier := func(text string) string {
		if label := a.tierLabel(text); label != text {
			return label
		}
		return a.subjectName(text, text)
	}
	switch issue.Code {
	case "missing_field":
		return fmt.Sprintf(locales.ImportIssueMissingField(), a.importFieldLabel(issue.Column))
	case "duplicate_class":
		return fmt.Sprintf(locales.ImportIssueDuplicateClass(), arg(0))
	case "duplicate_student":
		return fmt.Sprintf(locales.ImportIssueDuplicateStudent(), arg(0))
	case "unknown_status":
		return fmt.Sprintf(locales.ImportIssueUnknownStatus(), arg(0))
	case "invalid_number":
		return fmt.Sprintf(locales.ImportIssueInvalidNumber(), arg(0))
	case "no_rows":
		return locales.ImportIssueNoRows()
	case "duplicate_teacher_no":
		return fmt.Sprintf(locales.ImportIssueDuplicateTeacherNo(), arg(0))
	case "teacher_name_conflict":
		return fmt.Sprintf(locales.ImportIssueTeacherNameConflict(), arg(0), arg(1))
	case "teacher_no_conflict":
		return fmt.Sprintf(locales.ImportIssueTeacherNoConflict(), arg(0), arg(1))
	case "teacher_identity_missing":
		return fmt.Sprintf(locales.ImportIssueTeacherIdentity(), arg(0))
	case "unknown_subject":
		return fmt.Sprintf(locales.ImportIssueUnknownSubject(), arg(0))
	case "duplicate_assignment":
		return fmt.Sprintf(locales.ImportIssueDuplicateAssignment(), arg(0), arg(1), arg(2))
	case "classes_created":
		return fmt.Sprintf(locales.ImportIssueClassesCreated(), formatInt(numberArg(0)))
	case "unknown_class":
		return fmt.Sprintf(locales.ImportIssueUnknownClass(), arg(0))
	case "score_out_of_range":
		return fmt.Sprintf(locales.ImportIssueScoreOutOfRange(), arg(0),
			a.subjectName(arg(1), arg(1)), arg(2))
	case "ratio_not_positive":
		return locales.ImportIssueRatioNotPositive()
	case "duplicate_baseline":
		return fmt.Sprintf(locales.ImportIssueDuplicateBaseline(), arg(0),
			a.subjectName(arg(1), arg(1)))
	case "unknown_student":
		return fmt.Sprintf(locales.ImportIssueUnknownStudent(), arg(0))
	case "duplicate_exam_no":
		return fmt.Sprintf(locales.ImportIssueDuplicateExamNo(), arg(0))
	case "missing_track":
		return fmt.Sprintf(locales.ImportIssueMissingTrack(), arg(0))
	case "unknown_roster":
		return fmt.Sprintf(locales.ImportIssueUnknownRoster(), arg(0))
	case "no_scores":
		return fmt.Sprintf(locales.ImportIssueNoScores(), arg(0))
	case "unknown_exam_subject":
		return fmt.Sprintf(locales.ImportIssueUnknownExamSubject(), a.subjectName(arg(0), arg(0)))
	case "unknown_track":
		return fmt.Sprintf(locales.ImportIssueUnknownTrack(), a.trackName(arg(0), arg(0)))
	case "cutoff_not_positive":
		return fmt.Sprintf(locales.ImportIssueCutoffNotPositive(), subjectOrTier(arg(0)),
			a.trackName(arg(1), arg(1)))
	case "duplicate_cutoff":
		return fmt.Sprintf(locales.ImportIssueDuplicateCutoff(),
			subjectOrTier(arg(0)), a.trackName(arg(1), arg(1)))
	case "cutoff_zero":
		return fmt.Sprintf(locales.ImportIssueCutoffZero(), arg(0), a.trackName(arg(1), arg(1)))
	case "cutoff_subject_skipped":
		return fmt.Sprintf(locales.ImportIssueCutoffSubjectSkipped(), arg(0))
	case "unknown_tier":
		return fmt.Sprintf(locales.ImportIssueUnknownTier(), a.tierLabel(arg(0)))
	case "negative_target":
		return fmt.Sprintf(locales.ImportIssueNegativeTarget(), arg(0), a.tierLabel(arg(1)))
	case "duplicate_target":
		return fmt.Sprintf(locales.ImportIssueDuplicateTarget(), arg(0))
	case "archive_invalid_tier":
		return fmt.Sprintf(locales.ImportIssueArchiveInvalidTier(), arg(0))
	case "archive_missing_points":
		return fmt.Sprintf(locales.ImportIssueArchiveMissingPoints(), arg(0), a.subjectName(arg(1), arg(1)))
	case "archive_missing_total":
		return fmt.Sprintf(locales.ImportIssueArchiveMissingTotal(), arg(0), a.subjectName(arg(1), arg(1)))
	case "archive_missing_teacher":
		return fmt.Sprintf(locales.ImportIssueArchiveMissingTeacher(), arg(0), a.subjectName(arg(1), arg(1)))
	case "archive_missing_rank":
		return fmt.Sprintf(locales.ImportIssueArchiveMissingRank(), arg(0), a.subjectName(arg(1), arg(1)))
	case "archive_stat_group_duplicate":
		return fmt.Sprintf(locales.ImportIssueArchiveStatGroupDup(), arg(0), a.subjectName(arg(1), arg(1)))
	case "archive_personal_duplicate":
		return fmt.Sprintf(locales.ImportIssueArchivePersonalDup(), arg(0), a.subjectName(arg(1), arg(1)))
	default:
		return issue.Code
	}
}

func importResultText(kind string, summary *store.ImportSummary) string {
	switch kind {
	case importer.KindClasses:
		return fmt.Sprintf(locales.ImportResultClasses(), formatInt(summary.Inserted+summary.Updated),
			formatInt(summary.Classes))
	case importer.KindStudents:
		return fmt.Sprintf(locales.ImportResultStudents(), formatInt(summary.Inserted+summary.Updated),
			formatInt(summary.Classes))
	case importer.KindTeachers:
		return fmt.Sprintf(locales.ImportResultTeachers(), formatInt(summary.Inserted+summary.Updated),
			formatInt(summary.Teachers))
	case importer.KindAssignments:
		return fmt.Sprintf(locales.ImportResultAssignments(), formatInt(summary.Inserted+summary.Updated),
			formatInt(summary.TeachingGroups), formatInt(summary.Assignments))
	case importer.KindBaselines:
		return fmt.Sprintf(locales.ImportResultBaselines(), formatInt(summary.Inserted+summary.Updated),
			formatInt(summary.Baselines))
	case importer.KindRoster:
		return fmt.Sprintf(locales.ImportResultRoster(), formatInt(summary.Inserted+summary.Updated),
			formatInt(summary.Students))
	case importer.KindScores:
		return fmt.Sprintf(locales.ImportResultScores(), formatInt(summary.Inserted+summary.Updated))
	case importer.KindCutoffs:
		return fmt.Sprintf(locales.ImportResultCutoffs(), formatInt(summary.SubjectCutoffs),
			formatInt(summary.TierCutoffs))
	case importer.KindTargets:
		return fmt.Sprintf(locales.ImportResultTargets(), formatInt(summary.ClassTargets))
	default:
		return ""
	}
}
