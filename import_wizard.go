package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/nav"

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
	Summary           *store.ImportSummary
	Started           time.Time
	Duration          time.Duration
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

func (a *app) wizardChooseFile() {
	wizard := a.importWizard
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:   locales.ImportWizardChooseFile(),
			Filters: []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx", "xls"}}},
		})
		if err != nil || len(paths) == 0 || a.win == nil {
			return
		}
		a.win.Update(func() {
			a.loadWizardFile(wizard, paths[0])
		})
	}()
}

func (a *app) loadWizardFile(wizard *importWizardState, path string) {
	if wizard == nil {
		return
	}
	wizard.Error = ""
	workbook, err := importer.InspectWorkbook(path, importer.Options{
		ConversionDir: filepath.Join(a.dataDir, "converted"),
	})
	if err != nil {
		wizard.Error = err.Error()
		return
	}
	wizard.Workbook = workbook
	wizard.FilePath = path
	wizard.FileName = filepath.Base(path)
	wizard.Sheet = importer.BestSheet(wizard.Kind, workbook)
	wizard.Mapping = nil
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

func (a *app) wizardApply() {
	wizard := a.importWizard
	if wizard == nil || wizard.Preview.Blocking > 0 ||
		(wizard.Preview.Warnings > 0 && !wizard.ConfirmedWarnings) || wizard.Busy {
		return
	}
	wizard.Busy = true
	wizard.Started = time.Now()
	work := func() (store.ImportSummary, error) {
		stored, err := a.store.StoreImportFile(wizard.FilePath, wizard.Report.FileHash)
		if err != nil {
			return store.ImportSummary{}, err
		}
		wizard.Report.StoredPath = stored
		if wizard.ExamID != 0 {
			return a.store.ApplyExamImport(context.Background(), store.ExamImportRequest{
				ExamID: wizard.ExamID, CohortID: wizard.ExamCohortID, Kind: examImportKind(wizard.Kind),
				Data: wizard.Data, Report: wizard.Report, Mapping: wizard.Mapping, Issues: wizard.Issues,
			})
		}
		cohort := a.selectedCohort()
		stage := a.selectedStage()
		if cohort == nil || stage == nil {
			return store.ImportSummary{}, errors.New(locales.ImportNoStage())
		}
		return a.store.ApplyStageImport(context.Background(), cohort.ID, stage.ID,
			store.StageImportRequest{
				Kind: wizard.Kind, Data: wizard.Data, Report: wizard.Report,
				Mapping: wizard.Mapping, Issues: wizard.Issues,
			})
	}
	finish := func(summary store.ImportSummary, err error) {
		wizard.Busy = false
		if errors.Is(err, store.ErrDuplicateImport) {
			wizard.Error = locales.ImportWizardAlreadyImported()
			return
		}
		if err != nil {
			wizard.Error = err.Error()
			return
		}
		wizard.Summary = &summary
		wizard.Duration = time.Since(wizard.Started).Round(time.Millisecond)
		wizard.Error = ""
		wizard.Step = wizardStepResult
		ctx := context.Background()
		a.reloadCohorts(ctx)
		if wizard.ExamID != 0 {
			a.reloadExamData(ctx)
		} else {
			a.reloadDataPages(ctx)
		}
	}
	if a.win == nil {
		summary, err := work()
		finish(summary, err)
		return
	}
	go func() {
		summary, err := work()
		a.win.Update(func() {
			finish(summary, err)
		})
	}()
}

func (a *app) wizardDownloadTemplate() {
	wizard := a.importWizard
	if wizard == nil {
		return
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       locales.ImportWizardDownloadTemplate(),
			DefaultPath: "tally-" + wizard.Kind + ".xlsx",
			Filters:     []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx"}}},
		})
		if err != nil || path == "" {
			return
		}
		if a.win == nil {
			a.writeWizardTemplate(wizard, path)
			return
		}
		a.win.Update(func() {
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
}

func (a *app) importWizardPage(c *ui.Context, p uiPalette) {
	wizard := a.importWizard
	if wizard == nil {
		return
	}
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			if textButton(c, locales.Back()).Children(func() {
				ui.Icon(c, iconArrowLeft).FontSize(t.Space(4))
				ui.Text(c, locales.Back()).FontSize(bodySize(t))
			}).Clicked() {
				a.closeImportWizard()
			}
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Text(c, locales.ImportWizardTitle()).FontSize(titleSize(t)).Bold().
					Level(1).Role(ui.RoleHeading)
				if wizard.Kind != "" {
					ui.Text(c, importKindName(wizard.Kind)).FontSize(bodySize(t)).TextColor(p.muted)
				}
			})
		})
		a.wizardSteps(c, p)
		if wizard.Error != "" {
			notice(c, p, noticeDanger, wizard.Error)
		}
		if wizard.Busy {
			notice(c, p, noticeInfo, locales.ImportBusy())
		}
		switch wizard.Step {
		case wizardStepItem:
			a.wizardItemStep(c, p)
		case wizardStepFile:
			a.wizardFileStep(c, p)
		case wizardStepSheet:
			a.wizardSheetStep(c, p)
		case wizardStepMapping:
			a.wizardMappingStep(c, p)
		case wizardStepValidate:
			a.wizardValidateStep(c, p)
		default:
			a.wizardResultStep(c, p)
		}
	})
}

func (a *app) wizardSteps(c *ui.Context, p uiPalette) {
	labels := []string{
		locales.ImportStepItem(), locales.ImportStepFile(), locales.ImportStepSheet(),
		locales.ImportStepMapping(), locales.ImportStepValidate(), locales.ImportStepResult(),
	}
	items := make([]nav.StepItem, 0, len(labels))
	for _, label := range labels {
		items = append(items, nav.StepItem{Title: label})
	}
	nav.Steps(c, nav.StepsProps{Items: items, Current: a.importWizard.Step})
}

func (a *app) wizardItemStep(c *ui.Context, p uiPalette) {
	t := p.theme
	kinds := []string{
		importer.KindClasses, importer.KindStudents, importer.KindTeachers,
		importer.KindAssignments, importer.KindBaselines,
	}
	if a.wizardExam() {
		kinds = []string{importer.KindRoster, importer.KindScores,
			importer.KindCutoffs, importer.KindTargets}
	}
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
			if a.wizardExam() {
				if exam := a.selectedExam(); exam != nil {
					ui.Text(c, exam.Name).FontSize(captionSize(t)).TextColor(p.muted)
				}
			} else if cohort := a.selectedCohort(); cohort != nil {
				stage := a.displayStage()
				ui.Text(c, a.cohortName(cohort.EnrollmentYear)+" · "+
					a.stageName(stage.GradeLevel)+" · "+
					a.classOfName(cohort.GraduationYear)).
					FontSize(captionSize(t)).TextColor(p.muted)
			}
			for _, kind := range kinds {
				kind := kind
				rule(c)
				ui.Column(c).FillWidth().Padding(t.Space(3), 0).Gap(t.Space(1)).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Column(c).Grow(1).Gap(0).Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, importKindName(kind)).FontSize(bodySize(t)).Bold()
								if a.importKindCount(kind) > 0 {
									badge(c, p, locales.ReadinessReady(), toneNeutral)
								} else {
									badge(c, p, locales.ReadinessMissing(), toneNeutral)
								}
							})
							ui.Text(c, importKindPurpose(kind)).FontSize(captionSize(t)).
								TextColor(p.muted).Pretty()
						})
						button := labelledButton(c, locales.ImportWizardStart(), iconArrowRight, false)
						button.Label(locales.ImportWizardStart() + " " + importKindName(kind))
						if button.Clicked() {
							a.wizardSelectKind(kind)
						}
					})
				})
			}
		})
	})
}

func (a *app) wizardFileStep(c *ui.Context, p uiPalette) {
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Text(c, importKindPurpose(a.importWizard.Kind)).FontSize(bodySize(t)).
			TextColor(p.inkSecondary).Pretty()
		ui.Text(c, a.importKindColumns(a.importWizard.Kind)).FontSize(captionSize(t)).
			TextColor(p.muted).Pretty()
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			if labelledButton(c, locales.ImportWizardChooseFile(), iconImports, true).Clicked() {
				a.wizardChooseFile()
			}
			if plainButton(c, locales.ImportWizardDownloadTemplate()).Clicked() {
				a.wizardDownloadTemplate()
			}
		})
		if a.importWizard.FileName != "" {
			ui.Textf(c, locales.ImportWizardSelectedFile(), a.importWizard.FileName).
				FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
		}
	})
}

func (a *app) wizardSheetStep(c *ui.Context, p uiPalette) {
	wizard := a.importWizard
	t := p.theme
	if wizard.Workbook == nil {
		a.wizardFileStep(c, p)
		return
	}
	names := wizard.Workbook.SheetNames()
	chosen := wizard.Sheet
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Text(c, locales.ImportWizardSheetHint()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
		next := chosen
		selectField(c, &next, names, locales.ImportWizardSheetLabel()).Width(t.Space(70))
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
			ui.Text(c, strings.Join(headers, " · ")).FontSize(captionSize(t)).
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

func (a *app) wizardMappingStep(c *ui.Context, p uiPalette) {
	wizard := a.importWizard
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
					selectField(c, &next, labels, label).Grow(1)
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

func (a *app) wizardValidateStep(c *ui.Context, p uiPalette) {
	wizard := a.importWizard
	t := p.theme
	panel(c, p).FillWidth().Grow(1).Shrink(1).MinHeight(0).Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).Gap(t.Space(2)).Wrap().Children(func() {
			badge(c, p, fmt.Sprintf(locales.ImportWizardNew(), formatInt(wizard.Preview.New)), toneAccent)
			badge(c, p, fmt.Sprintf(locales.ImportWizardUpdate(), formatInt(wizard.Preview.Update)), toneSuccess)
			badge(c, p, fmt.Sprintf(locales.ImportWizardSkip(), formatInt(wizard.Preview.Skip)), toneNeutral)
			badge(c, p, fmt.Sprintf(locales.ImportWizardBlocking(), formatInt(wizard.Preview.Blocking)), toneDanger)
			badge(c, p, fmt.Sprintf(locales.ImportWizardWarnings(), formatInt(wizard.Preview.Warnings)), toneWarning)
		})
		if len(wizard.Issues) == 0 {
			ui.Text(c, locales.ImportWizardNoIssues()).FontSize(bodySize(t)).TextColor(p.muted)
		} else {
			ui.Scroll(c).Grow(1).Shrink(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
					for _, issue := range wizard.Issues {
						issue := issue
						tone := p.warning
						if issue.Severity == "blocking" {
							tone = p.danger
						}
						ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(t.Space(2)).Children(func() {
							ui.Box(c).Width(t.Space(1)).Height(t.Space(4)).Radius(t.Space(0.5)).
								Background(tone)
							ui.Column(c).Grow(1).Gap(0).Children(func() {
								ui.Text(c, a.importIssueText(issue)).FontSize(bodySize(t)).Pretty()
								if issue.Row > 0 {
									ui.Textf(c, locales.ImportIssueLocation(), issue.Row,
										a.importColumnName(issue.Column)).FontSize(captionSize(t)).
										TextColor(p.muted).FontFeatures("tnum")
								}
							})
						})
					}
				})
			})
		}
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			if plainButton(c, locales.ImportWizardBackToMapping()).Clicked() {
				wizard.Step = wizardStepMapping
			}
			if wizard.Preview.Warnings > 0 {
				checkboxField(c, &wizard.ConfirmedWarnings,
					fmt.Sprintf(locales.ImportWizardConfirmWarnings(), formatInt(wizard.Preview.Warnings)))
			}
			button := labelledButton(c,
				fmt.Sprintf(locales.ImportWizardImportRows(), formatInt(wizard.Preview.New+wizard.Preview.Update)),
				iconCheck, true)
			button.Disabled(wizard.Preview.Blocking > 0 || wizard.Busy ||
				(wizard.Preview.Warnings > 0 && !wizard.ConfirmedWarnings))
			if button.Clicked() {
				a.wizardApply()
			}
		})
	})
}

func (a *app) wizardResultStep(c *ui.Context, p uiPalette) {
	wizard := a.importWizard
	t := p.theme
	summary := wizard.Summary
	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Icon(c, iconCheck).FontSize(t.Space(5)).TextColor(p.success)
			ui.Text(c, locales.ImportWizardFinished()).FontSize(sectionSize(t)).Bold()
		})
		if summary != nil {
			ui.Text(c, importResultText(wizard.Kind, summary)).FontSize(bodySize(t)).Pretty()
			if wizard.Duration > 0 {
				ui.Textf(c, locales.ImportWizardElapsed(), wizard.Duration).
					FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
			}
		}
		ui.Row(c).Wrap().Gap(t.Space(2)).Children(func() {
			if plainButton(c, locales.ImportWizardViewData()).Clicked() {
				a.page = importKindPage(wizard.Kind)
				a.closeImportWizard()
			}
			if next := nextImportKind(wizard.Kind); next != "" {
				if labelledButton(c, fmt.Sprintf(locales.ImportWizardNextKind(), importKindName(next)),
					iconArrowRight, true).Clicked() {
					a.wizardContinueKind(next)
				}
			}
			if plainButton(c, locales.ImportWizardDone()).Clicked() {
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
		return fmt.Sprintf(locales.ImportIssueScoreOutOfRange(), arg(0), arg(1))
	case "ratio_not_positive":
		return locales.ImportIssueRatioNotPositive()
	case "duplicate_baseline":
		return fmt.Sprintf(locales.ImportIssueDuplicateBaseline(), arg(0), arg(1))
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
		return fmt.Sprintf(locales.ImportIssueUnknownExamSubject(), arg(0))
	case "unknown_track":
		return fmt.Sprintf(locales.ImportIssueUnknownTrack(), arg(0))
	case "cutoff_not_positive":
		return fmt.Sprintf(locales.ImportIssueCutoffNotPositive(), arg(0), arg(1))
	case "duplicate_cutoff":
		return fmt.Sprintf(locales.ImportIssueDuplicateCutoff(), arg(0), arg(1))
	case "unknown_tier":
		return fmt.Sprintf(locales.ImportIssueUnknownTier(), arg(0))
	case "negative_target":
		return fmt.Sprintf(locales.ImportIssueNegativeTarget(), arg(0), arg(1))
	case "duplicate_target":
		return fmt.Sprintf(locales.ImportIssueDuplicateTarget(), arg(0))
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
