package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tally/internal/importer"
	"tally/internal/store"
	"tally/locales"
)

// importsPage 是独立的「数据导入」中心页，统一承载：
// 1. 新建导入（文件拖拽/选择 + 多 Sheet 自动分类 + 快捷数据类型入口）
// 2. 准备清单（从总览迁移出的基础数据就绪清单）
// 3. 导入记录（工作区级基础数据与考试类导入的详情查看与安全撤销）
func (a *app) importsPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
		if a.importReturn != nil {
			ctxText := fmt.Sprintf(locales.ImportCenterReturnContext(), a.importReturn.Label)
			retLabel := fmt.Sprintf(locales.ImportCenterReturnButton(), a.importReturn.Label)
			if returnContextBanner(c, p, ctxText, retLabel) {
				a.returnFromImport()
				return
			}
		}
		pageHeading(c, p, locales.ImportCenterTitle(), locales.ImportCenterSubtitle(), nil)
		if !a.dataStageReady() {
			a.noStageState(c, p, iconImports)
			return
		}
		if a.dataError != "" {
			notice(c, p, noticeDanger, a.dataError)
		}
		if a.dataMessage != "" {
			notice(c, p, noticeSuccess, a.dataMessage)
		}
		tabs := []string{
			locales.ImportCenterTabNew(),
			locales.ImportCenterTabGuide(),
			locales.ImportCenterTabHistory(),
		}
		if a.importCenterTab < 0 || a.importCenterTab >= len(tabs) {
			a.importCenterTab = 0
		}
		ui.Row(c).Children(func() {
			segmentedField(c, &a.importCenterTab, tabs, locales.ImportCenterTitle())
		})
		ui.Column(c).Grow(1).Shrink(1).MinHeight(0).FillWidth().Children(func() {
			switch a.importCenterTab {
			case 1:
				a.importCenterGuideTab(c, p)
			case 2:
				a.importCenterHistoryTab(c, p)
			default:
				a.importCenterNewTab(c, p)
			}
		})
	})
}

func (a *app) importCenterNewTab(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.importCenterScope < 0 || a.importCenterScope > 1 {
		a.importCenterScope = 0
	}
	scope := "stage"
	if a.importCenterScope == 1 {
		scope = "exam"
	}
	if scope == "exam" && a.importCenterExamID == 0 && len(a.exams) > 0 {
		a.importCenterExamID = a.exams[0].ID
	}

	ui.Scroll(c.Key("import-center-new")).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Padding(0, 0, t.Space(3), 0).Children(func() {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				prevScope := a.importCenterScope
				segmentedField(c, &a.importCenterScope, []string{
					locales.ImportCenterScopeStage(),
					locales.ImportCenterScopeExam(),
				}, locales.ImportCenterTitle())
				if a.importCenterScope != prevScope && a.importCenterWorkbook != nil {
					a.reclassifyCenterWorkbook()
				}
				if a.importCenterScope == 1 {
					if len(a.exams) == 0 {
						if labelledButton(c, locales.OverviewQuickNewExam(), iconPlus, false).Clicked() {
							a.navigateTo(pageExams)
							a.openNewExam()
						}
					} else {
						labels := make([]string, 0, len(a.exams))
						chosen := ""
						for _, exam := range a.exams {
							lbl := a.examBatchLabel(exam)
							labels = append(labels, lbl)
							if exam.ID == a.importCenterExamID {
								chosen = lbl
							}
						}
						if chosen == "" && len(labels) > 0 {
							chosen = labels[0]
							a.importCenterExamID = a.exams[0].ID
						}
						next := chosen
						selectField(c, "import-center-exam", &next, labels, locales.NavExamHub()).Width(t.Space(64))
						if next != chosen {
							for idx, lbl := range labels {
								if lbl == next {
									a.importCenterExamID = a.exams[idx].ID
									break
								}
							}
						}
					}
				}
			})

			if a.importCenterScope == 0 && !a.stageEditable() {
				notice(c, p, noticeInfo, locales.StageReadOnly())
			}
			if a.importCenterError != "" {
				notice(c, p, noticeDanger, a.importCenterError)
			}
			if showDelayedBusy(c, "import-center-reading", a.importCenterReading) {
				ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Spinner(c).Label(locales.ImportWizardReadingFile())
					ui.Text(c, locales.ImportWizardReadingFile()).FontSize(bodySize(t)).TextColor(p.muted)
				})
			}

			canDrop := (a.importCenterScope == 0 && a.stageEditable()) ||
				(a.importCenterScope == 1 && a.importCenterExamID != 0 && a.selectedCohort() != nil)
			if canDrop {
				clicked, dropped := fileDropZone(c, p, fileDropZoneProps{
					Key:         "import-center-drop",
					Title:       locales.ImportCenterDropTitle(),
					ActiveTitle: locales.ImportCenterDropActive(),
					Hint:        locales.ImportCenterDropHint(),
					ButtonLabel: locales.ImportCenterBrowseFile(),
				})
				if len(dropped) > 0 {
					a.loadImportCenterWorkbook(dropped[0])
				} else if clicked {
					a.chooseImportCenterFile()
				}
			}

			if a.importCenterWorkbook != nil && len(a.importCenterSheets) > 0 {
				a.importCenterDetectedSheets(c, p)
			}

			a.importCenterKindGrid(c, p, scope)
		})
	})
}

func (a *app) chooseImportCenterFile() {
	if a.importCenterReading {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: locales.ImportWizardChooseFile(),
			Filters: []mygo.FileFilter{{
				Name:       "Excel / CSV",
				Extensions: []string{"xlsx", "xls", "csv"},
			}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		if a.win == nil {
			a.loadImportCenterWorkbook(path)
			return
		}
		a.win.Update(func() {
			a.importCenterReading = true
			a.importCenterError = ""
			a.importCenterFilePath = path
			a.importCenterFileName = filepath.Base(path)
		})
		wb, inspectErr := importer.InspectWorkbook(path, importer.Options{
			ConversionDir: filepath.Join(a.dataDir, "converted"),
			SofficePath:   a.sofficePath,
		})
		a.win.Update(func() {
			a.importCenterReading = false
			a.applyImportCenterWorkbook(path, wb, inspectErr)
		})
	}()
}

func (a *app) loadImportCenterWorkbook(path string) {
	a.importCenterError = ""
	wb, err := importer.InspectWorkbook(path, importer.Options{
		ConversionDir: filepath.Join(a.dataDir, "converted"),
		SofficePath:   a.sofficePath,
	})
	a.applyImportCenterWorkbook(path, wb, err)
}

func (a *app) applyImportCenterWorkbook(path string, wb *importer.Workbook, err error) {
	if err != nil {
		a.importCenterError = err.Error()
		a.importCenterWorkbook = nil
		a.importCenterSheets = nil
		return
	}
	a.importCenterWorkbook = wb
	a.importCenterFilePath = path
	a.importCenterFileName = filepath.Base(path)
	a.importCenterError = ""
	a.reclassifyCenterWorkbook()
}

func (a *app) reclassifyCenterWorkbook() {
	if a.importCenterWorkbook == nil {
		a.importCenterSheets = nil
		return
	}
	scope := "stage"
	tiers := importer.DefaultTiers()
	if a.importCenterScope == 1 {
		scope = "exam"
		if len(a.examTierKeys) > 0 {
			tiers = a.examTierKeys
		}
	}
	a.importCenterSheets = importer.ClassifyWorkbookSheets(
		a.importCenterWorkbook, scope, a.subjects, tiers, 3,
	)
}

func (a *app) importCenterDetectedSheets(c *ui.Context, p uiPalette) {
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Icon(c, iconSpreadsheet).Size(18, 18).TextColor(p.ink).Shrink(0)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
				ui.Textf(c, locales.ImportCenterSheetsDetected(), formatInt(len(a.importCenterSheets))).
					FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink).SingleLine()
				ui.Text(c, a.importCenterFileName).FontSize(captionSize(t)).TextColor(p.muted).
					SingleLine().TruncateMiddle().Tooltip(a.importCenterFilePath)
			})
			if textButton(c, locales.Close()).Clicked() {
				a.importCenterWorkbook = nil
				a.importCenterSheets = nil
				a.importCenterFilePath = ""
				a.importCenterFileName = ""
			}
		})
		primaryUsed := false
		for idx, sheet := range a.importCenterSheets {
			sheet := sheet
			if idx > 0 {
				rule(c)
			}
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Padding(t.Space(1.5), 0).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(50)).Gap(t.Space(1)).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Wrap().Gap(t.Space(2)).Children(func() {
						ui.Text(c, sheet.Sheet).FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink).SingleLine()
						if sheet.Kind != "" {
							badge(c, p, importKindName(sheet.Kind), toneAccent)
						} else {
							badge(c, p, locales.ImportCenterSheetUnrecognized(), toneNeutral)
						}
						if sheet.Recommended {
							badge(c, p, locales.ImportCenterSheetRecommended(), toneSuccess)
						}
						ui.Textf(c, locales.ImportCenterSheetRows(), formatInt(sheet.RowCount)).
							FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
					})
					if sheet.Kind != "" {
						if len(sheet.MissingRequired) == 0 {
							ui.Text(c, locales.ImportCenterSheetReady()).FontSize(captionSize(t)).TextColor(p.success)
						} else {
							missing := make([]string, 0, len(sheet.MissingRequired))
							for _, key := range sheet.MissingRequired {
								missing = append(missing, a.importFieldLabel(key))
							}
							ui.Textf(c, locales.ImportCenterSheetMissingCols(),
								strings.Join(missing, locales.ListSeparator())).
								FontSize(captionSize(t)).TextColor(p.warning).Pretty()
						}
					}
				})
				if sheet.Kind != "" {
					isPrimary := sheet.Recommended && !primaryUsed
					if isPrimary {
						primaryUsed = true
					}
					btn := labelledButton(c, locales.ImportCenterGoImport(), iconArrowRight, isPrimary)
					btn.Label(locales.ImportCenterGoImport() + " " + sheet.Sheet)
					if btn.Clicked() {
						a.startWizardFromDetectedSheet(sheet)
					}
				}
			})
		}
	})
}

func (a *app) startWizardFromDetectedSheet(sheet importer.SheetClassification) {
	if a.importCenterWorkbook == nil || sheet.Kind == "" {
		return
	}
	var wizard *importWizardState
	if a.importCenterScope == 1 {
		cohort := a.selectedCohort()
		if cohort == nil || a.importCenterExamID == 0 {
			return
		}
		wizard = &importWizardState{
			Kind:         sheet.Kind,
			Step:         wizardStepMapping,
			ExamID:       a.importCenterExamID,
			ExamCohortID: cohort.ID,
			Workbook:     a.importCenterWorkbook,
			FilePath:     a.importCenterFilePath,
			FileName:     a.importCenterFileName,
			Sheet:        sheet.Sheet,
			Mapping:      cloneStringMap(sheet.Mapping),
		}
	} else {
		if !a.stageEditable() {
			a.dataError = locales.StageReadOnly()
			return
		}
		wizard = &importWizardState{
			Kind:     sheet.Kind,
			Step:     wizardStepMapping,
			Workbook: a.importCenterWorkbook,
			FilePath: a.importCenterFilePath,
			FileName: a.importCenterFileName,
			Sheet:    sheet.Sheet,
			Mapping:  cloneStringMap(sheet.Mapping),
		}
	}
	a.importWizard = wizard
}

func cloneStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (a *app) importCenterKindGrid(c *ui.Context, p uiPalette, scope string) {
	t := p.theme
	kinds := []string{
		importer.KindClasses,
		importer.KindStudents,
		importer.KindTeachers,
		importer.KindAssignments,
		importer.KindBaselines,
	}
	if scope == "exam" {
		kinds = []string{
			importer.KindRoster,
			importer.KindScores,
			importer.KindCutoffs,
			importer.KindTargets,
		}
	}
	editable := (scope == "stage" && a.stageEditable()) ||
		(scope == "exam" && a.importCenterExamID != 0 && a.selectedCohort() != nil)

	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(2)).Children(func() {
		sectionHeading(c, p, locales.ImportStepItem(), len(kinds))
		for idx, kind := range kinds {
			kind := kind
			if idx > 0 {
				rule(c)
			}
			count := a.importKindCount(kind)
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Padding(t.Space(1.5), 0).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(50)).Gap(t.Space(0.5)).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Text(c, importKindName(kind)).FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink)
						if count > 0 {
							badge(c, p, locales.ImportCenterChecklistReady(), toneSuccess)
						} else {
							badge(c, p, locales.ImportCenterChecklistMissing(), toneNeutral)
						}
					})
					ui.Text(c, importKindPurpose(kind)).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
				})
				btn := labelledButton(c, locales.ImportCenterGoImport(), iconArrowRight, false)
				btn.Label(locales.ImportCenterGoImport() + " " + importKindName(kind))
				btn.Disabled(!editable)
				if btn.Clicked() {
					if scope == "exam" {
						a.openExamImportWizard(kind, a.importCenterExamID, a.selectedCohort().ID)
					} else {
						a.openImportWizard(kind)
					}
				}
			})
		}
	})
}

func (a *app) importCenterGuideTab(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	steps := a.prepSteps()
	done := 0
	next := -1
	for idx, step := range steps {
		if step.Count > 0 {
			done++
		} else if next < 0 {
			next = idx
		}
	}
	editable := a.stageEditable()
	hint := locales.PrepSubtitle()
	if cohort != nil {
		hint = fmt.Sprintf(locales.ImportCenterChecklistHint(), a.cohortName(cohort.EnrollmentYear))
	}

	ui.Scroll(c.Key("import-center-guide")).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Padding(0, 0, t.Space(3), 0).Children(func() {
			panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
						sectionHeading(c, p, locales.ImportCenterChecklistTitle(), -1)
						ui.Text(c, hint).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
					})
					ui.Textf(c, locales.ReadinessProgress(), formatInt(done), formatInt(len(steps))).
						FontSize(bodySize(t)).TextColor(p.inkSecondary).FontFeatures("tnum").Shrink(0)
				})
				percent := float32(0)
				if len(steps) > 0 {
					percent = float32(done) / float32(len(steps)) * 100
				}
				progressBar(c, p, percent, locales.ReadinessTitle()).FillWidth()
				if !editable {
					notice(c, p, noticeInfo, locales.StageReadOnly())
				}
				for idx, step := range steps {
					idx := idx
					step := step
					rule(c)
					ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Padding(t.Space(2), 0).Children(func() {
						ui.Textf(c, "%d", idx+1).FontSize(captionSize(t)).
							TextColor(p.muted).FontFeatures("tnum").Width(t.Space(4))
						ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(48)).Gap(t.Space(0.5)).Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, step.Title).FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink)
								if step.Count > 0 {
									badge(c, p, locales.ImportCenterChecklistReady(), toneSuccess)
								} else {
									badge(c, p, locales.ImportCenterChecklistMissing(), toneWarning)
								}
							})
							ui.Text(c, step.Body).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
						})
						if step.Count > 0 {
							ui.Text(c, formatInt(step.Count)).FontSize(captionSize(t)).
								TextColor(p.muted).FontFeatures("tnum")
							viewBtn := plainButton(c, locales.PrepView())
							viewBtn.Label(locales.PrepView() + " " + step.Title)
							if viewBtn.Clicked() {
								a.openBaseDataTabForPage(step.Page)
							}
						}
						if editable {
							primary := step.Count == 0 && idx == next
							impBtn := labelledButton(c, locales.ImportCenterGoImport(), iconImport, primary)
							impBtn.Label(locales.ImportCenterGoImport() + " " + step.Title)
							if impBtn.Clicked() {
								a.openImportWizard(step.Kind)
							}
						}
					})
				}
			})
		})
	})
}

func (a *app) openBaseDataTabForPage(page pageID) {
	switch page {
	case pageCohorts:
		a.cohortDetailTab = 1
	case pageStudents:
		a.cohortDetailTab = 2
	case pageFaculty:
		a.cohortDetailTab = 3
	case pageGroups:
		a.cohortDetailTab = 1
	case pageBaseline:
		a.cohortDetailTab = 4
	default:
		a.navigateTo(page)
		return
	}
	if a.selectedCohort() != nil {
		a.cohortDetailOpen = true
	}
	a.navigateTo(pageCohorts)
}

func (a *app) importCenterHistoryTab(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(2)).Children(func() {
		a.importFilters(c, p)
		runs := a.visibleImportRuns()
		if len(runs) == 0 {
			if strings.TrimSpace(a.importQuery) != "" {
				emptyState(c, p, iconImports, locales.FilterEmptyTitle(), locales.FilterEmptyHint(), func() {
					if plainButton(c, locales.ClearFilters()).Clicked() {
						a.importQuery = ""
					}
				})
				return
			}
			emptyState(c, p, iconImports, locales.ImportsEmpty(), locales.ImportCenterHistoryEmpty(), func() {
				if labelledButton(c, locales.ImportCenterTabNew(), iconImport, true).Clicked() {
					a.importCenterTab = 0
				}
			})
			return
		}
		ui.Row(c).Grow(1).Shrink(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Gap(t.Space(3)).Children(func() {
			selectedRunID := int64(0)
			a.importsListState.Key = func(row int) any { return runs[row].ID }
			ui.List(c, &a.importsListState, len(runs), func(row int) {
				run := runs[row]
				fileLabel := importFileLabel(run)
				ui.Column(c).FillWidth().Gap(0).Children(func() {
					if row > 0 {
						rule(c)
					}
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).
						Padding(t.Space(2), 0).Children(func() {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, fileLabel).FontSize(bodySize(t)).
									FontWeight(500).SingleLine().TruncateMiddle().
									Shrink(1).MinWidth(0).Tooltip(fileLabel)
								badge(c, p, importKindName(run.DataKind), toneNeutral)
								if run.Status == "undone" {
									badge(c, p, locales.ImportStatusUndone(), toneNeutral)
								}
							})
							ui.Textf(c, locales.ImportRunMeta(), formatTime(run.CreatedAt),
								formatInt(run.RowCount)).FontSize(captionSize(t)).
								TextColor(p.muted).FontFeatures("tnum").SingleLine().MinWidth(0)
						})
						if run.Warnings > 0 {
							ui.Text(c, pluralCount(run.Warnings, locales.ImportWarningCountOne(), locales.ImportWarningCount())).
								FontSize(captionSize(t)).TextColor(p.warning).Shrink(0)
						}
						detailButton := plainButton(c, locales.ImportViewDetail()).
							Label(locales.ImportViewDetail() + " " + importKindName(run.DataKind))
						if detailButton.Clicked() {
							selectedRunID = run.ID
						}
					})
				})
			}).Label(locales.NavImports()).Grow(1).Shrink(1).MinHeight(0).MinWidth(0).Fill()
			if selectedRunID != 0 {
				a.selectImportRun(selectedRunID)
			}
			if a.importDetail != nil {
				ui.Column(c.Key("import-detail-panel")).Width(t.Space(84)).Shrink(0).FillHeight().MinHeight(0).
					Transition(drawerTransition).Children(func() {
					a.importDetailPanel(c, p)
				})
			}
		})
	})
}

func (a *app) importDetailPanel(c *ui.Context, p uiPalette) {
	detail := a.importDetail
	if detail == nil {
		return
	}
	t := p.theme
	canUndo := a.importCanUndo[detail.Run.ID]
	reason := a.importUndoReason[detail.Run.ID]
	listPanel(c, p, locales.ImportDetailTitle(), len(detail.Issues), func() {
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				a.detailRow(c, p, locales.ImportDetailKind(), importKindName(detail.Run.DataKind))
				a.detailPathRow(c, p, locales.ImportDetailFile(), importFileLabel(detail.Run))
				a.detailRow(c, p, locales.ImportDetailStatus(),
					importStatusName(detail.Run.Status))
				a.detailRow(c, p, locales.ImportDetailRows(), formatInt(detail.Run.RowCount))
				if detail.Run.RelativePath != "" {
					a.detailPathRow(c, p, locales.ImportSourcePath(), detail.Run.RelativePath)
				}
				for _, sheet := range detail.Run.Sheets {
					ui.Textf(c, locales.ImportSheetRow(), sheet.Name, sheet.Role, sheet.Rows).
						FontSize(captionSize(t)).TextColor(p.inkSecondary)
				}
				rule(c)
				if len(detail.Issues) == 0 {
					ui.Text(c, locales.ImportWizardNoIssues()).FontSize(bodySize(t)).
						TextColor(p.muted)
				}
				for _, issue := range detail.Issues {
					issue := issue
					ui.Column(c).FillWidth().Gap(0).Children(func() {
						ui.Text(c, a.importIssueText(issue)).FontSize(bodySize(t)).Pretty()
						ui.Textf(c, locales.ImportIssueLocation(), issue.Row,
							a.importColumnName(issue.Column)).FontSize(captionSize(t)).
							TextColor(p.muted).FontFeatures("tnum")
					})
				}
				rule(c)
				button := labelledButton(c, locales.ImportUndo(), iconUndo, false)
				button.Disabled(!canUndo)
				if button.Clicked() {
					a.importUndoConfirmOpen = true
				}
				if !canUndo {
					text := locales.ImportUndoNotImported()
					if reason == "newer_import" {
						text = locales.ImportUndoBlocked()
					} else if reason == "no_changes" {
						text = locales.ImportUndoNoChanges()
					}
					ui.Text(c, text).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
				}
				if plainButton(c, locales.Close()).Clicked() {
					a.importRunID = 0
					a.importDetail = nil
				}
			})
		})
	})
}

func (a *app) importUndoConfirmModal(c *ui.Context, p uiPalette) {
	if !a.importUndoConfirmOpen && a.importDetail == nil {
		return
	}
	name := ""
	if a.importDetail != nil {
		name = importFileLabel(a.importDetail.Run)
	}
	body := fmt.Sprintf(locales.ImportCenterUndoConfirmBody(), name)
	if confirmDangerModal(c, p, "import-undo-confirm", &a.importUndoConfirmOpen,
		locales.ImportCenterUndoConfirmTitle(), body,
		locales.PlanCancel(), locales.ImportCenterUndoConfirmAction()) {
		a.undoSelectedImport()
	}
}

func (a *app) importFilters(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).FillWidth().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
		ui.Column(c).Width(t.Space(72)).Children(func() {
			nextQuery := a.importQuery
			searchField(c, &nextQuery, locales.DataSearchImports(), locales.DataSearchImports())
			if nextQuery != a.importQuery {
				a.importQuery = nextQuery
			}
		})
		ui.Spacer(c)
		ui.Text(c, pluralCount(len(a.visibleImportRuns()), locales.DataRecordCountOne(), locales.DataRecordCount())).
			FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
	})
}

func (a *app) visibleImportRuns() []store.ImportRun {
	query := strings.ToLower(strings.TrimSpace(a.importQuery))
	if query == "" {
		return a.importRuns
	}
	runs := make([]store.ImportRun, 0, len(a.importRuns))
	for _, run := range a.importRuns {
		text := strings.ToLower(strings.Join([]string{
			importFileLabel(run), run.RelativePath, run.StoredPath, run.DataKind,
		}, " "))
		for _, sheet := range run.Sheets {
			text += " " + strings.ToLower(sheet.Name+" "+sheet.Role)
		}
		if strings.Contains(text, query) {
			runs = append(runs, run)
		}
	}
	return runs
}
