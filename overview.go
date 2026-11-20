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
					if labelledButton(c, locales.NewCohort(), iconPlus, false).Clicked() {
						a.openNewCohort()
					}
				}
			}
			pageHeading(c, p, locales.OverviewTitle(), locales.OverviewSubtitle(), actions)
			if len(a.cohorts) == 0 {
				emptyState(c, p, iconCohort, locales.OverviewNoCohort(), "", func() {
					if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
						a.openNewCohort()
					}
				})
				return
			}
			a.stageReviewBanner(c, p)
			a.overviewHero(c, p)
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(t.Space(4)).Children(func() {
				a.readinessPanel(c, p)
				a.recentExamsPanel(c, p)
			})
			a.gaokaoPanel(c, p)
			a.dataSummaryPanel(c, p)
		})
	})
	a.newCohortModal(c, p)
	a.stageReviewModal(c, p)
}

func (a *app) overviewHero(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	if cohort == nil {
		return
	}
	stage := a.displayStage()
	ready, total := a.readiness()
	context := a.cohortName(cohort.EnrollmentYear) + " · " +
		a.stageName(stage.GradeLevel) + " · " + a.classOfName(cohort.GraduationYear)
	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
				ui.Text(c, context).FontSize(titleSize(t)).Bold().LetterSpacing(-0.2)
				ui.Text(c, a.academicYearLabel(stage.AcademicYearStart)).FontSize(bodySize(t)).
					TextColor(p.muted)
			})
			a.stageSelect(c, p, stage)
			if labelledButton(c, locales.OverviewContinue(), iconArrowRight, true).Clicked() {
				a.prepOpen = true
			}
		})
		ui.Divider(c)
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			ui.Textf(c, locales.ReadinessProgress(), ready, total).
				FontSize(bodySize(t)).TextColor(p.inkSecondary)
			ui.Progress(c, float64(ready)/float64(total)).
				TextColor(p.accent).Background(p.panelMuted).Grow(1).
				Label(locales.ReadinessTitle())
			ui.Textf(c, "%d/%d", ready, total).FontSize(captionSize(t)).
				TextColor(p.muted).FontFeatures("tnum")
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
		ui.Row(c).AlignItems(ui.Start).Gap(t.Space(2)).Children(func() {
			ui.Icon(c, iconAlert).FontSize(t.Space(4)).TextColor(p.warning)
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Textf(c, locales.StageAdvanceTitle(),
					a.academicYearLabel(stage.AcademicYearStart)).FontSize(bodySize(t)).Bold()
				ui.Textf(c, locales.StageAdvanceBody(), a.stageName(stage.GradeLevel),
					a.stageCounts.Classes, a.stageCounts.TeachingGroups, a.stageCounts.Assignments).
					FontSize(bodySize(t)).TextColor(p.inkSecondary).Pretty()
			})
			ui.Row(c).Gap(t.Space(2)).Children(func() {
				if ui.Button(c, locales.StageReviewConfirm()).Clicked() {
					if err := a.store.ConfirmStageReview(context.Background(), stage.ID); err != nil {
						a.settingsError = err.Error()
						return
					}
					a.reloadSelectedCohort(context.Background())
				}
				if ui.Button(c, locales.StageReviewView()).Clicked() {
					a.stageReviewOpen = true
				}
				if ui.Button(c, locales.StageReviewLater()).Clicked() {
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
	if !a.stageReviewOpen {
		return
	}
	stage := a.displayStage()
	if stage.ID == 0 {
		a.stageReviewOpen = false
		return
	}
	t := p.theme
	ui.Modal(c, &a.stageReviewOpen, func() {
		ui.Column(c).Width(t.Space(112)).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Text(c, fmt.Sprintf(locales.StageReviewTitle(), a.copiedStageLabel(stage))).
					FontSize(sectionSize(t)).Bold().Role(ui.RoleHeading).Level(2)
				ui.Text(c, locales.StageReviewBody()).FontSize(bodySize(t)).
					TextColor(p.muted).Pretty()
			})
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				a.detailRow(c, p, locales.ReadinessClasses(), fmtInt(a.stageCounts.Classes))
				a.detailRow(c, p, locales.StageReviewTeachingGroups(),
					fmtInt(a.stageCounts.TeachingGroups))
				a.detailRow(c, p, locales.ReadinessAssignments(), fmtInt(a.stageCounts.Assignments))
				a.detailRow(c, p, locales.ReadinessStudents(), fmtInt(a.stageCounts.Students))
			})
			ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if ui.Button(c, locales.Close()).Clicked() {
					a.stageReviewOpen = false
				}
				if ui.PrimaryButton(c, locales.StageReviewOpenPrep()).Clicked() {
					a.stageReviewOpen = false
					a.prepOpen = true
				}
			})
		})
	})
}

func (a *app) copiedStageLabel(stage store.CohortStage) string {
	if stage.CopiedFromStageID == nil {
		return a.stageName(stage.GradeLevel) + " · " + a.academicYearLabel(stage.AcademicYearStart)
	}
	for _, previous := range a.cohortStages {
		if previous.ID != *stage.CopiedFromStageID {
			continue
		}
		return a.stageName(previous.GradeLevel) + " · " +
			a.academicYearLabel(previous.AcademicYearStart)
	}
	return a.stageName(stage.GradeLevel) + " · " + a.academicYearLabel(stage.AcademicYearStart)
}

func (a *app) stageSelect(c *ui.Context, p uiPalette, stage store.CohortStage) {
	t := p.theme
	labels, ids := a.stageOptions()
	if len(labels) == 0 {
		ui.Text(c, a.stageName(stage.GradeLevel)).FontSize(bodySize(t)).TextColor(p.muted)
		return
	}
	chosen := ""
	for index, id := range ids {
		if id == a.selectedStageID {
			chosen = labels[index]
			break
		}
	}
	if chosen == "" {
		caption := fmt.Sprintf(locales.StageOption(), a.stageName(stage.GradeLevel),
			school.AcademicYearLabel(stage.AcademicYearStart))
		for _, label := range labels {
			if label == caption {
				chosen = label
				break
			}
		}
	}
	next := chosen
	ui.Select(c, &next, labels).Label(locales.StageSelectLabel()).Width(t.Space(46))
	if next != chosen {
		a.selectStage(next)
	}
}

func (a *app) readinessPanel(c *ui.Context, p uiPalette) {
	t := p.theme
	ready, total := a.readiness()
	panel(c, p).Grow(1).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			sectionHeading(c, p, locales.ReadinessTitle(), -1)
			ui.Spacer(c)
			ui.Textf(c, "%d/%d", ready, total).FontSize(captionSize(t)).
				TextColor(p.muted).FontFeatures("tnum")
		})
		a.readinessRow(c, p, locales.ReadinessClasses(), a.stageCounts.Classes)
		a.readinessRow(c, p, locales.ReadinessStudents(), a.stageCounts.Students)
		a.readinessRow(c, p, locales.ReadinessTeachers(), a.stageCounts.Teachers)
		a.readinessRow(c, p, locales.ReadinessAssignments(), a.stageCounts.Assignments)
		a.readinessRow(c, p, locales.ReadinessBaseline(), a.stageCounts.Baselines)
	})
}

func (a *app) readinessRow(c *ui.Context, p uiPalette, label string, count int) {
	t := p.theme
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).
		Padding(t.Space(1.5), 0).Children(func() {
		ui.Text(c, label).FontSize(bodySize(t)).Grow(1)
		if count > 0 {
			badge(c, p, locales.ReadinessReady(), toneSuccess)
			return
		}
		badge(c, p, locales.ReadinessMissing(), toneNeutral)
	})
}

func (a *app) recentExamsPanel(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	stage := a.displayStage()
	panel(c, p).Width(t.Space(92)).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.OverviewRecentExams(), -1)
		count := 0
		for _, exam := range a.exams {
			examCohort, ok := examMatchesStage(exam, cohort, stage)
			if !ok {
				continue
			}
			count++
			if count > 6 {
				break
			}
			exam := exam
			row := linkRow(c, p, a.examBatchLabel(exam), func() {
				ui.Column(c).Grow(1).Gap(0).Children(func() {
					ui.Text(c, a.examBatchLabel(exam)).FontSize(bodySize(t)).SingleLine()
					ui.Textf(c, locales.ExamListMeta(), a.academicYearLabel(exam.AcademicYearStart),
						a.examTermName(exam.Term), exam.ExamDate).FontSize(captionSize(t)).
						TextColor(p.muted).SingleLine()
				})
				statusBadge(c, p, exam.Status, a.examStatusName(exam.Status))
			})
			if row.Clicked() {
				a.page = pageExams
				a.selectExam(exam.ID)
				a.examCohortID = examCohort.CohortID
				a.reloadExamData(context.Background())
			}
		}
		if count == 0 {
			ui.Text(c, locales.OverviewNoExams()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
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

func (a *app) dataSummaryPanel(c *ui.Context, p uiPalette) {
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.OverviewDataSummary(), -1)
		ui.Text(c, locales.OverviewNoData()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
	})
}

func (a *app) newCohortModal(c *ui.Context, p uiPalette) {
	if !a.newCohortOpen {
		return
	}
	t := p.theme
	ui.Modal(c, &a.newCohortOpen, func() {
		ui.Column(c).Width(t.Space(112)).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.NewCohortTitle()).FontSize(sectionSize(t)).Bold().
					Role(ui.RoleHeading).Level(2)
				ui.Text(c, locales.NewCohortSubtitle()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
			})
			ui.Form(c, func() {
				ui.Field(c, locales.EnrollmentYear(), func() {
					ui.NumberInput(c, &a.newCohortYear, 1990, float64(a.academicYearStart), 1).
						Label(locales.EnrollmentYear())
				}).Description(fmt.Sprintf(locales.CohortYearRange(), 1990, a.academicYearStart))
			})
			year := int(a.newCohortYear)
			index := school.StageIndex(year, a.academicYearStart)
			ui.Text(c, fmt.Sprintf(locales.CohortPreview(), year, a.stageIndexName(index),
				school.GraduationYear(year))).FontSize(bodySize(t)).TextColor(p.inkSecondary)
			if a.newCohortError != "" {
				notice(c, p, noticeDanger, a.newCohortError)
			}
			ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if ui.Button(c, locales.PlanCancel()).Clicked() {
					a.newCohortOpen = false
				}
				if ui.PrimaryButton(c, locales.CohortCreate()).Clicked() {
					a.createCohort()
				}
			})
		})
	})
}

func (a *app) cohortPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.cohortSidebarWidth == 0 {
		a.cohortSidebarWidth = t.Space(58)
	}
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavCohorts(), locales.CohortPageSubtitle(), func() {
			classButton := ui.Button(c, locales.ImportClasses())
			classButton.Disabled(!a.stageEditable())
			if classButton.Clicked() {
				a.openImportWizard(importer.KindClasses)
			}
			if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
				a.openNewCohort()
			}
		})
		if len(a.cohorts) == 0 {
			emptyState(c, p, iconCohort, locales.OverviewNoCohort(), "", nil)
		} else {
			if a.selectedStage() != nil && !a.stageEditable() {
				notice(c, p, noticeInfo, locales.StageReadOnly())
			}
			ui.Split(c, &a.cohortSidebarWidth, func() {
				a.cohortSidebar(c, p)
			}, func() {
				a.cohortDetails(c, p)
			}).Grow(1)
		}
	})
	a.newCohortModal(c, p)
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
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(4)).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
					ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
						ui.Text(c, a.cohortName(cohort.EnrollmentYear)).FontSize(sectionSize(t)).Bold()
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
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
				sectionHeading(c, p, locales.CohortStageHistory(), len(a.cohortStages))
				if len(a.cohortStages) == 0 {
					ui.Text(c, locales.CohortNoStages()).FontSize(bodySize(t)).
						TextColor(p.muted).Pretty()
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
					ui.Divider(c)
				}
			})
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
				sectionHeading(c, p, locales.CohortClasses(), len(a.classes))
				if len(a.classes) == 0 {
					ui.Text(c, locales.CohortNoClasses()).FontSize(bodySize(t)).
						TextColor(p.muted).Pretty()
					return
				}
				columns := []ui.TableColumn{
					{Title: locales.ColumnClass(), Width: t.Space(24)},
					{Title: locales.ReadinessStudents(), Width: 0, Align: ui.End},
					{Title: locales.ClassHeadTeacher(), Width: 0},
				}
				ui.Table(c, &a.classTable, columns, len(a.classes), func(row, col int) {
					class := a.classes[row]
					switch col {
					case 0:
						ui.Text(c, class.Name).SingleLine()
					case 1:
						ui.Textf(c, "%d", class.Students).FontFeatures("tnum")
					case 2:
						if class.HeadTeacher == "" {
							ui.Text(c, "-").TextColor(p.muted)
						} else {
							ui.Text(c, class.HeadTeacher).SingleLine()
						}
					}
				}).Label(locales.CohortClasses()).Height(t.Space(64))
			})
		})
	})
}

func (a *app) settingsPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(t.Space(190)).Gap(t.Space(6)).Children(func() {
			pageHeading(c, p, locales.NavSettings(), locales.SettingsSubtitle(), nil)
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(4)).Children(func() {
				sectionHeading(c, p, locales.SettingsSchool(), -1)
				ui.Field(c, locales.SettingsAcademicYear(), func() {
					a.academicYearSelect(c, p)
				}).Description(locales.SettingsAcademicYearHint())
			})
			if a.settingsError != "" {
				notice(c, p, noticeDanger, a.settingsError)
			}
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
				sectionHeading(c, p, locales.SettingsSubjects(), len(a.subjectDrafts))
				ui.Text(c, locales.SettingsSubjectsHint()).FontSize(bodySize(t)).
					TextColor(p.muted).Pretty()
				for index := range a.subjectDrafts {
					subject := &a.subjectDrafts[index]
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Text(c, a.subjectName(subject.Key, subject.Name)).FontSize(captionSize(t)).
							TextColor(p.muted).Width(t.Space(16))
						ui.TextInput(c, &subject.Name).Label(locales.SubjectName()).Grow(1)
						ui.NumberInput(c, &subject.FullMark, 0, 300, 1).
							Label(locales.SubjectFullMark()).Width(t.Space(18))
						ui.Switch(c, &subject.Enabled).Label(locales.SubjectEnabled())
					})
				}
			})
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
				sectionHeading(c, p, locales.SettingsTracks(), len(a.trackDrafts))
				ui.Text(c, locales.SettingsTracksHint()).FontSize(bodySize(t)).
					TextColor(p.muted).Pretty()
				for index := range a.trackDrafts {
					track := &a.trackDrafts[index]
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Text(c, track.Key).FontSize(captionSize(t)).TextColor(p.muted).
							Width(t.Space(16))
						ui.TextInput(c, &track.Name).Label(locales.TrackName()).Grow(1)
						ui.Switch(c, &track.Enabled).Label(locales.TrackEnabled())
					})
				}
				ui.Row(c).Children(func() {
					if labelledButton(c, locales.SettingsSaveSchool(), nil, true).Clicked() {
						a.saveSchoolSettings()
					}
				})
			})
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(4)).Children(func() {
				sectionHeading(c, p, locales.SettingsData(), -1)
				ui.Column(c).Gap(t.Space(1)).Children(func() {
					ui.Text(c, locales.SettingsDataDir()).FontSize(captionSize(t)).TextColor(p.muted)
					ui.Text(c, a.dataDir).FontSize(bodySize(t)).Selectable().SingleLine()
				})
				ui.Divider(c)
				sectionHeading(c, p, locales.SettingsBackups(), len(a.backups))
				ui.Row(c).Gap(t.Space(2)).Children(func() {
					if labelledButton(c, locales.SettingsBackupNow(), iconImports, false).Clicked() {
						a.backupNow()
					}
				})
				if len(a.backups) == 0 {
					ui.Text(c, locales.SettingsBackupsEmpty()).FontSize(bodySize(t)).
						TextColor(p.muted)
				}
				for _, backup := range a.backups {
					backup := backup
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).
						Padding(t.Space(2), 0).Children(func() {
						ui.Column(c).Grow(1).Gap(0).Children(func() {
							ui.Text(c, formatTime(backup.CreatedAt)).FontSize(bodySize(t)).
								FontFeatures("tnum")
							ui.Textf(c, locales.SettingsBackupSize(), float64(backup.Size)/(1024*1024)).
								FontSize(captionSize(t)).TextColor(p.muted)
						})
						if ui.Button(c, locales.SettingsRestore()).Clicked() {
							a.restoreBackupName = backup.Name
							a.restoreOpen = true
						}
					})
					ui.Divider(c)
				}
				ui.Divider(c)
				sectionHeading(c, p, locales.SettingsAppearance(), -1)
				ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
					ui.Text(c, locales.LanguageLabel()).FontSize(bodySize(t)).Grow(1)
					a.languageSwitch(c)
				})
			})
		})
	})
	if ui.AlertDialog(c, &a.restoreOpen, locales.SettingsRestoreTitle(),
		fmt.Sprintf(locales.SettingsRestoreConfirm(), a.restoreBackupName),
		locales.PlanCancel(), locales.SettingsRestore()) == 1 {
		name := a.restoreBackupName
		a.restoreBackupName = ""
		a.restoreBackup(name)
	}
}

func (a *app) academicYearSelect(c *ui.Context, p uiPalette) {
	t := p.theme
	years := make([]int, 0, 7)
	for year := a.systemAcademicYear - 3; year <= a.systemAcademicYear+3; year++ {
		years = append(years, year)
	}
	labels := make([]string, 0, len(years))
	chosen := a.academicYearLabel(a.academicYearStart)
	for _, year := range years {
		labels = append(labels, a.academicYearLabel(year))
	}
	if !containsString(labels, chosen) {
		labels = append(labels, chosen)
	}
	next := chosen
	ui.Select(c, &next, labels).Label(locales.SettingsAcademicYear()).Width(t.Space(52))
	if next != chosen {
		start, err := school.ParseAcademicYearStart(next)
		if err == nil {
			a.setAcademicYear(start)
		}
	}
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
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
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
			panel(c, p).Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
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
		ui.Text(c, label).FontSize(bodySize(t)).Bold()
		ui.Text(c, body).FontSize(bodySize(t)).TextColor(p.inkSecondary).Pretty()
	})
}

func (a *app) placeholderPage(c *ui.Context, p uiPalette, title, subtitle string, icon *ui.SVG, empty string) {
	t := p.theme
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(6)).Children(func() {
			pageHeading(c, p, title, subtitle, nil)
			panel(c, p).FillWidth().Padding(t.Space(8)).Children(func() {
				emptyState(c, p, icon, locales.NoDataYet(), empty, nil)
			})
		})
	})
}

func (a *app) resultsPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavResults(), locales.ResultsSubtitle(), nil)
		batch, err := a.selectedBatchValue()
		if err != nil {
			emptyState(c, p, iconResults, locales.ResultsEmpty(), "", nil)
			return
		}
		if len(a.resultRows) == 0 {
			emptyState(c, p, iconResults, locales.ResultsEmpty(), "", nil)
			return
		}
		ui.Column(c).Grow(1).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, a.batchLabel(*batch), len(a.resultRows))
			a.batchResults(c, p)
		})
	})
}
