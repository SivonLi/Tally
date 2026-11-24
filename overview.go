package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"

	"tally/internal/importer"
	"tally/internal/school"
	"tally/internal/store"
	"tally/locales"
)

func (a *app) overviewPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.prepOpen {
		a.prepPage(c, p)
		return
	}
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(6)).Children(func() {
			var actions func()
			if len(a.cohorts) > 0 {
				actions = func() {
					if labelledButton(c, locales.OverviewContinue(), iconArrowRight, true).Clicked() {
						a.prepOpen = true
					}
					if labelledButton(c, locales.NewCohort(), iconPlus, false).Clicked() {
						a.openNewCohort()
					}
				}
			}
			pageHeading(c, p, locales.OverviewTitle(), locales.OverviewSubtitle(), actions)
			if len(a.cohorts) == 0 {
				emptyState(c, p, iconCohort, locales.OverviewNoCohort(), locales.OverviewNoCohortHint(), func() {
					if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
						a.openNewCohort()
					}
				})
				return
			}
			a.stageReviewBanner(c, p)
			a.overviewHero(c, p)
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(t.Space(7)).Children(func() {
				a.readinessPanel(c, p)
				a.recentExamsPanel(c, p)
			})
			a.gaokaoPanel(c, p)
		})
	})
}

func (a *app) overviewHero(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	if cohort == nil {
		return
	}
	ready, total := a.readiness()
	section(c, p, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Textf(c, locales.CohortContext(), a.cohortName(cohort.EnrollmentYear),
				a.stageName(a.displayStage().GradeLevel),
				a.classOfName(cohort.GraduationYear)).
				FontSize(bodySize(t)).FontWeight(500).TextColor(p.inkSecondary).SingleLine().
				Shrink(1).MinWidth(0)
			ui.Spacer(c)
			ui.Text(c, a.academicYearLabel(a.displayStage().AcademicYearStart)).
				FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
		})
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(4)).Children(func() {
			percent := float32(0)
			if total > 0 {
				percent = float32(ready) / float32(total) * 100
			}
			progressBar(c, p, percent, "").Grow(1)
			ui.Textf(c, locales.ReadinessProgress(), formatInt(ready), formatInt(total)).
				FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
		})
	})
}

func (a *app) stageReviewBanner(c *ui.Context, p uiPalette) {
	stage := a.displayStage()
	if stage.ReviewStatus != "pending" || a.stageNoticeDismissed[stage.ID] {
		return
	}
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(3)).Gap(t.Space(2)).
		Background(p.warningSoft).Border(1, p.warning.Alpha(0.35)).Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Start).Gap(t.Space(3)).Children(func() {
			ui.Row(c).Grow(1).Shrink(1).MinWidth(t.Space(60)).AlignItems(ui.Start).Gap(t.Space(2)).Children(func() {
				ui.Icon(c, iconAlert).FontSize(t.Space(4)).TextColor(p.warning).Shrink(0)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(0).Children(func() {
					ui.Textf(c, locales.StageAdvanceTitle(),
						a.academicYearLabel(stage.AcademicYearStart)).FontSize(bodySize(t)).FontWeight(600)
					ui.Textf(c, locales.StageAdvanceBody(), a.stageName(stage.GradeLevel),
						a.stageCounts.Classes, a.stageCounts.TeachingGroups, a.stageCounts.Assignments).
						FontSize(bodySize(t)).TextColor(p.inkSecondary).Pretty()
				})
			})
			ui.Row(c).Wrap().Shrink(0).Gap(t.Space(2)).Children(func() {
				if plainButton(c, locales.StageReviewConfirm()).Clicked() {
					if err := a.store.ConfirmStageReview(context.Background(), stage.ID); err != nil {
						a.settingsError = err.Error()
						return
					}
					a.reloadSelectedCohort(context.Background())
					a.notifyMain(locales.StageReviewConfirmedToast())
				}
				if plainButton(c, locales.StageReviewView()).Clicked() {
					a.stageReviewOpen = true
				}
				if plainButton(c, locales.StageReviewLater()).Clicked() {
					if a.stageNoticeDismissed == nil {
						a.stageNoticeDismissed = map[int64]bool{}
					}
					a.stageNoticeDismissed[stage.ID] = true
				}
			})
		})
	})
}

func (a *app) stageReviewModal(c *ui.Context, p uiPalette) {
	stage := a.displayStage()
	if a.stageReviewOpen && stage.ID == 0 {
		a.stageReviewOpen = false
	}
	t := p.theme
	confirmed := modal(c, &a.stageReviewOpen, modalProps{
		Key:        "stage-review",
		Title:      fmt.Sprintf(locales.StageReviewTitle(), a.copiedStageLabel(stage)),
		OkText:     locales.StageReviewOpenPrep(),
		CancelText: locales.Close(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			ui.Text(c, locales.StageReviewBody()).FontSize(bodySize(t)).
				TextColor(p.muted).Pretty()
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				a.detailRow(c, p, locales.ReadinessClasses(), formatInt(a.stageCounts.Classes))
				a.detailRow(c, p, locales.StageReviewTeachingGroups(),
					formatInt(a.stageCounts.TeachingGroups))
				a.detailRow(c, p, locales.ReadinessAssignments(), formatInt(a.stageCounts.Assignments))
				a.detailRow(c, p, locales.ReadinessStudents(), formatInt(a.stageCounts.Students))
			})
		})
	})
	if confirmed {
		a.stageReviewOpen = false
		a.prepOpen = true
	}
}

func (a *app) copiedStageLabel(stage store.CohortStage) string {
	if stage.CopiedFromStageID == nil {
		return fmt.Sprintf(locales.StageOption(), a.stageName(stage.GradeLevel),
			a.academicYearLabel(stage.AcademicYearStart))
	}
	for _, previous := range a.cohortStages {
		if previous.ID != *stage.CopiedFromStageID {
			continue
		}
		return fmt.Sprintf(locales.StageOption(), a.stageName(previous.GradeLevel),
			a.academicYearLabel(previous.AcademicYearStart))
	}
	return fmt.Sprintf(locales.StageOption(), a.stageName(stage.GradeLevel),
		a.academicYearLabel(stage.AcademicYearStart))
}

func (a *app) readinessPanel(c *ui.Context, p uiPalette) {
	t := p.theme
	ready, total := a.readiness()
	ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(3)).Children(func() {
		rule(c)
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.ReadinessTitle(), -1)
			ui.Spacer(c)
			ui.Text(c, formatInt(ready)+"/"+formatInt(total)).FontSize(captionSize(t)).
				TextColor(p.muted).FontFeatures("tnum")
		})
		steps := []readinessStep{
			{locales.ReadinessClasses(), a.stageCounts.Classes, pageCohorts},
			{locales.ReadinessStudents(), a.stageCounts.Students, pageStudents},
			{locales.ReadinessAssignments(), a.stageCounts.Assignments, pageFaculty},
			{locales.ReadinessBaseline(), a.stageCounts.Baselines, pageBaseline},
		}
		items := make([]actionListItem, 0, len(steps))
		for _, step := range steps {
			meta := locales.ReadinessMissing()
			if step.Count > 0 {
				meta = locales.ReadinessReady()
			}
			items = append(items, actionListItem{
				Key: step.Label, Label: step.Label, Extra: meta,
			})
		}
		clicked := actionList(c, p, items, false)
		for _, step := range steps {
			if step.Label == clicked {
				a.page = step.Page
			}
		}
	})
}

type readinessStep struct {
	Label string
	Count int
	Page  pageID
}

func (a *app) recentExamsPanel(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	stage := a.displayStage()
	count := 0
	for _, exam := range a.exams {
		if _, ok := examMatchesStage(exam, cohort, stage); ok {
			count++
		}
	}
	ui.Column(c).Width(t.Space(84)).MinWidth(0).Gap(t.Space(3)).Children(func() {
		rule(c)
		sectionHeading(c, p, locales.OverviewRecentExams(), -1)
		if count == 0 {
			emptyState(c, p, iconExams, locales.ExamsEmpty(), locales.OverviewNoExams(), func() {
				if labelledButton(c, locales.AssessmentNewExam(), iconPlus, false).Clicked() {
					a.page = pageExams
					a.openNewExam()
				}
			})
			return
		}
		shown := 0
		for _, exam := range a.exams {
			examCohort, ok := examMatchesStage(exam, cohort, stage)
			if !ok {
				continue
			}
			if shown >= 4 {
				break
			}
			shown++
			exam := exam
			title := a.examBatchLabel(exam)
			row := listRowButton(c, title).
				Height(0).MinHeight(t.Space(14)).Padding(t.Space(3), t.Space(3)).
				AlignItems(ui.Center)
			row.Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(0).Children(func() {
					ui.Text(c, title).FontSize(bodySize(t)).SingleLine().MinWidth(0).Shrink(1).
						Tooltip(title).TextAlign(ui.Start)
					ui.Textf(c, locales.ExamListMeta(), a.academicYearLabel(exam.AcademicYearStart),
						a.examTermName(exam.Term), exam.ExamDate).FontSize(captionSize(t)).
						TextColor(p.muted).SingleLine().MinWidth(0).Shrink(1)
				})
				ui.Text(c, a.examStatusName(exam.Status)).FontSize(captionSize(t)).
					TextColor(p.muted).Shrink(0)
			})
			if row.Clicked() {
				a.page = pageExams
				a.selectExam(exam.ID)
				a.examCohortID = examCohort.CohortID
				a.reloadExamData(context.Background())
			}
		}
	})
}

func examMatchesStage(exam store.Exam, cohort *store.Cohort, stage store.CohortStage) (store.ExamCohort, bool) {
	if cohort == nil {
		return store.ExamCohort{}, false
	}
	for _, item := range exam.Cohorts {
		if item.CohortID == cohort.ID && item.GradeLevelAtExam == stage.GradeLevel &&
			exam.AcademicYearStart == stage.AcademicYearStart {
			return item, true
		}
	}
	return store.ExamCohort{}, false
}

func batchMatchesStage(batch store.Batch, stage store.CohortStage) bool {
	start, err := school.ParseAcademicYearStart(batch.AcademicYear)
	if err != nil {
		return false
	}
	return start == stage.AcademicYearStart && batch.Grade == stage.GradeLevel
}

func (a *app) academicYearLabelForBatch(batch store.Batch) string {
	start, err := school.ParseAcademicYearStart(batch.AcademicYear)
	if err != nil {
		return batch.AcademicYear
	}
	return a.academicYearLabel(start)
}

func (a *app) gaokaoPanel(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	if cohort == nil || cohort.Status != "active" {
		return
	}
	if cohort.GraduationYear != a.nowValue().Year() {
		return
	}
	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, fmt.Sprintf(locales.GaokaoTitle(), cohort.GraduationYear), -1)
		ui.Text(c, locales.GaokaoHint()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
		ui.Row(c).Children(func() {
			if labelledButton(c, locales.GaokaoOpen(), iconBatches, false).Clicked() {
				a.page = pageExams
				a.openNewExam()
				a.newExamType = 2
				a.newExamName = locales.ExamTypeGaokao()
			}
		})
	})
}

func (a *app) newCohortModal(c *ui.Context, p uiPalette) {
	t := p.theme
	wasOpen := a.newCohortOpen
	confirmed := modal(c, &a.newCohortOpen, modalProps{
		Key:        "new-cohort",
		Title:      locales.NewCohortTitle(),
		OkText:     locales.CohortCreate(),
		CancelText: locales.PlanCancel(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			ui.Text(c, locales.NewCohortSubtitle()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
			a.dialogForm(c, func() {
				a.dialogField(c, "enrollment-year", locales.EnrollmentYear(),
					fmt.Sprintf(locales.CohortYearRange(), 1990, a.academicYearStart), func() {
						numberField(c, &a.newCohortYear, 1990, float64(a.academicYearStart), 1,
							locales.EnrollmentYear())
					})
			})
			year := int(a.newCohortYear)
			index := school.StageIndex(year, a.academicYearStart)
			ui.Text(c, fmt.Sprintf(locales.CohortPreview(), year, a.stageIndexName(index),
				school.GraduationYear(year))).FontSize(bodySize(t)).TextColor(p.inkSecondary)
			if a.newCohortError != "" {
				notice(c, p, noticeDanger, a.newCohortError)
			}
		})
	})
	if confirmed {
		a.createCohort()
	} else if wasOpen && !a.newCohortOpen {
		a.newCohortError = ""
	}
}

func (a *app) cohortPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.cohortSidebarWidth == 0 {
		a.cohortSidebarWidth = t.Space(58)
	}
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavCohorts(), locales.CohortPageSubtitle(), func() {
			classButton := plainButton(c, locales.ImportClasses())
			classButton.Disabled(!a.stageEditable())
			if classButton.Clicked() {
				a.openImportWizard(importer.KindClasses)
			}
			if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
				a.openNewCohort()
			}
		})
		if len(a.cohorts) == 0 {
			emptyState(c, p, iconCohort, locales.OverviewNoCohort(), locales.OverviewNoCohortHint(), func() {
				if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
					a.openNewCohort()
				}
			})
		} else {
			if a.selectedStage() != nil && !a.stageEditable() {
				notice(c, p, noticeInfo, locales.StageReadOnly())
			}
			ui.Split(c, &a.cohortSidebarWidth, func() {
				ui.Column(c).Fill().MinWidth(0).Clip().Padding(0, t.Space(4), 0, 0).Children(func() {
					a.cohortSidebar(c, p)
				})
			}, func() {
				ui.Column(c).Fill().MinWidth(0).Padding(0, 0, 0, t.Space(4)).Children(func() {
					a.cohortDetails(c, p)
				})
			}).Grow(1)
		}
	})
}

func (a *app) cohortSidebar(c *ui.Context, p uiPalette) {
	changed := false
	listPanel(c, p, locales.CohortListTitle(), len(a.cohorts), func() {
		sidebar := ui.Sidebar(c, &a.selectedCohortID, func() {
			for _, cohort := range a.cohorts {
				ui.SidebarItem(c, strconv.FormatInt(cohort.ID, 10), iconCohort,
					a.cohortName(cohort.EnrollmentYear))
			}
		}).Label(locales.CohortListTitle()).Fill()
		if sidebar.Changed() {
			changed = true
		}
	})
	if changed {
		a.selectedStageID = 0
		a.reloadSelectedCohort(context.Background())
	}
}

func (a *app) cohortDetails(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	if cohort == nil {
		ui.Column(c).Fill().Children(func() {
			emptyState(c, p, iconCohort, locales.CohortSelectHint(), "", nil)
		})
		return
	}
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		section(c, p, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
					ui.Text(c, a.cohortName(cohort.EnrollmentYear)).FontSize(sectionSize(t)).FontWeight(600)
					ui.Text(c, a.classOfName(cohort.GraduationYear)).FontSize(bodySize(t)).
						TextColor(p.muted)
				})
				badge(c, p, locales.CohortActive(), toneSuccess)
			})
			ui.Row(c).Gap(t.Space(7)).Children(func() {
				metric(c, p, strconv.Itoa(cohort.EnrollmentYear), locales.EnrollmentYear())
				metric(c, p, strconv.Itoa(cohort.GraduationYear), locales.CohortGraduation())
				metric(c, p, strconv.Itoa(len(a.cohortStages)), locales.CohortStageHistory())
				metric(c, p, strconv.Itoa(a.stageCounts.Students), locales.ReadinessStudents())
			})
		})
		sectionRule(c, p, func() {
			sectionHeading(c, p, locales.CohortStageHistory(), len(a.cohortStages))
			if len(a.cohortStages) == 0 {
				smallEmptyState(c, p, locales.CohortNoStages())
				return
			}
			for _, stage := range a.cohortStages {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).
					Padding(t.Space(2), 0).Children(func() {
					ui.Text(c, a.stageName(stage.GradeLevel)).FontSize(bodySize(t)).Width(t.Space(18))
					ui.Text(c, a.academicYearLabel(stage.AcademicYearStart)).FontSize(bodySize(t)).
						TextColor(p.inkSecondary).Grow(1)
					if stage.ReviewStatus == "pending" {
						badge(c, p, locales.CohortPending(), toneWarning)
					} else {
						badge(c, p, locales.CohortConfirmed(), toneSuccess)
					}
				})
				rule(c)
			}
		})
		ui.Column(c).Grow(1).Shrink(1).MinHeight(0).Gap(t.Space(3)).Children(func() {
			rule(c)
			sectionHeading(c, p, locales.CohortClasses(), len(a.classes))
			if len(a.classes) == 0 {
				smallEmptyState(c, p, locales.CohortNoClasses())
				return
			}
			a.classTable.RowHeight = tableRowHeight(t)
			a.classTable.Key = func(row int) any { return a.classes[row].ID }
			columns := []ui.TableColumn{
				{Title: locales.ColumnClass(), Width: t.Space(24)},
				{Title: locales.ReadinessStudents(), Width: 0, Align: ui.End},
				{Title: locales.ClassHeadTeacher(), Width: 0},
			}
			ui.Table(c, &a.classTable, columns, len(a.classes), func(row, col int) {
				class := a.classes[row]
				switch col {
				case 0:
					cellText(c, p, class.Name)
				case 1:
					ui.Text(c, formatInt(class.Students)).FontFeatures("tnum")
				case 2:
					cellText(c, p, class.HeadTeacher)
				}
			}).Label(locales.CohortClasses()).Grow(1).Shrink(1).MinHeight(0).Fill()
		})
	})
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (a *app) helpPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(t.Space(190)).Gap(t.Space(6)).Children(func() {
			pageHeading(c, p, locales.NavHelp(), locales.HelpSubtitle(), nil)
			section(c, p, func() {
				sectionHeading(c, p, locales.HelpImportGuide(), -1)
				ui.Text(c, locales.HelpImportGuideHint()).FontSize(bodySize(t)).
					TextColor(p.muted).Pretty()
				a.helpRow(c, p, locales.HelpStudents(), locales.HelpStudentsRequirement())
				a.helpRow(c, p, locales.HelpAssignments(), locales.HelpAssignmentsRequirement())
				a.helpRow(c, p, locales.HelpBaseline(), locales.HelpBaselineRequirement())
				a.helpRow(c, p, locales.HelpScores(), locales.HelpScoresRequirement())
				a.helpRow(c, p, locales.HelpCutoffs(), locales.HelpCutoffsRequirement())
				a.helpRow(c, p, locales.HelpTargets(), locales.HelpTargetsRequirement())
			})
			sectionRule(c, p, func() {
				sectionHeading(c, p, locales.HelpGlossary(), -1)
				a.helpRow(c, p, locales.HelpTermCohort(), locales.HelpTermCohortBody())
				a.helpRow(c, p, locales.HelpTermClassOf(), locales.HelpTermClassOfBody())
				a.helpRow(c, p, locales.HelpTermStage(), locales.HelpTermStageBody())
				a.helpRow(c, p, locales.HelpTermYear(), locales.HelpTermYearBody())
				a.helpRow(c, p, locales.HelpTermBaseline(), locales.HelpTermBaselineBody())
				a.helpRow(c, p, locales.HelpTermMerged(), locales.HelpTermMergedBody())
			})
		})
	})
}

func (a *app) helpRow(c *ui.Context, p uiPalette, label, body string) {
	t := p.theme
	ui.Column(c).FillWidth().Gap(0).Padding(t.Space(2), 0).Children(func() {
		ui.Text(c, label).FontSize(bodySize(t)).FontWeight(600)
		ui.Text(c, body).FontSize(bodySize(t)).TextColor(p.inkSecondary).Pretty()
	})
}
