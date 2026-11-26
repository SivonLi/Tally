package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo/ui"

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
		ui.Column(c).FillWidth().Gap(t.Space(5)).Children(func() {
			hasActive := len(a.schoolOverview.ActiveCohorts) > 0
			pageHeading(c, p, locales.OverviewTitle(), locales.OverviewSchoolSubtitle(), func() {
				if labelledButton(c, locales.NewCohort(), iconPlus, !hasActive).Clicked() {
					a.openNewCohort()
				}
				if hasActive {
					if labelledButton(c, locales.OverviewQuickNewExam(), iconExams, true).Clicked() {
						a.navigateTo(pageExams)
						a.openNewExam()
					}
				}
			})

			a.overviewActiveCohortsSection(c, p)
			a.overviewTeacherOccupancySection(c, p)
			ui.Grid(c).FillWidth().
				ColumnTracks(ui.Fr(3), ui.Fr(2)).
				Gap(t.Space(4)).Children(func() {
				a.overviewRecentExamsCard(c, p)
				a.overviewRemindersCard(c, p)
			})
		})
	})
}

func (a *app) overviewActiveCohortsSection(c *ui.Context, p uiPalette) {
	t := p.theme
	cards := a.schoolOverview.ActiveCohorts
	if len(cards) == 0 {
		panel(c, p).FillWidth().Padding(t.Space(5)).Children(func() {
			emptyState(c, p, iconCohort, locales.OverviewActiveCohortsEmpty(), locales.OverviewActiveCohortsEmptyHint(), func() {
				if labelledButton(c, locales.CohortCreate(), iconPlus, true).Clicked() {
					a.openNewCohort()
				}
			})
		})
		if a.schoolOverview.GraduatedCount > 0 {
			a.overviewGraduatedBanner(c, p)
		}
		return
	}

	section(c, p, func() {
		sectionHeading(c, p, locales.OverviewActiveCohortsTitle(), len(cards))
		cols := min(len(cards), 3)
		ui.Grid(c).FillWidth().Columns(cols).Gap(t.Space(3)).Children(func() {
			for _, card := range cards {
				card := card
				a.overviewCohortCard(c, p, card)
			}
		})
		if a.schoolOverview.GraduatedCount > 0 {
			a.overviewGraduatedBanner(c, p)
		}
	})
}

func (a *app) overviewGraduatedBanner(c *ui.Context, p uiPalette) {
	t := p.theme
	text := fmt.Sprintf(locales.OverviewGraduatedArchiveHint(), formatInt(a.schoolOverview.GraduatedCount))
	row := linkRow(c, p, text, func() {
		ui.Icon(c, iconCohort).Size(16, 16).TextColor(p.muted).Shrink(0)
		ui.Text(c, text).FontSize(bodySize(t)).TextColor(p.inkSecondary).Grow(1).Shrink(1).MinWidth(0)
		ui.Icon(c, iconArrowRight).Size(15, 15).TextColor(p.muted).Shrink(0)
	})
	if row.Clicked() {
		a.cohortArchiveFilter = 1
		a.cohortDetailOpen = false
		a.navigateTo(pageCohorts)
	}
}

func (a *app) overviewCohortCard(c *ui.Context, p uiPalette, card store.CohortOverviewCard) {
	t := p.theme
	cohort := card.Cohort
	title := a.cohortName(cohort.EnrollmentYear)
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, title).FontSize(sectionSize(t)).FontWeight(600).
					TextColor(p.ink).SingleLine().Tooltip(title)
				ui.Text(c, fmt.Sprintf("%s · %s", a.classOfName(cohort.GraduationYear), a.academicYearLabel(cohort.CurrentAcademicYearStart))).
					FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
			})
			badge(c, p, a.stageName(cohort.CurrentPhase), toneAccent)
		})

		ui.Row(c).FillWidth().Wrap().Gap(t.Space(4)).Children(func() {
			metricInline(c, p, uint64(card.ClassCount), locales.ReadinessClasses())
			metricInline(c, p, uint64(card.ActiveStudents), locales.ReadinessStudents())
			metricInline(c, p, uint64(card.ExamCount), locales.OverviewRecentExamsTitle())
		})

		rule(c)

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			examText := locales.OverviewCohortNoExam()
			if card.LatestExamName != "" {
				examText = fmt.Sprintf(locales.OverviewCohortLatestExam(), card.LatestExamName)
			}
			ui.Text(c, examText).FontSize(captionSize(t)).TextColor(p.inkSecondary).
				Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(examText)
			if card.LatestExamID > 0 {
				if card.HasOfficialRun {
					badge(c, p, locales.OverviewLatestOfficial(), toneSuccess)
				} else {
					badge(c, p, locales.OverviewLatestPending(), toneNeutral)
				}
			}
		})

		ui.Row(c).FillWidth().Wrap().Gap(t.Space(2)).Children(func() {
			enterBtn := plainButton(c.Key("ov-enter-"+strconv.FormatInt(cohort.ID, 10)), locales.OverviewEnterCohort())
			if enterBtn.Clicked() {
				a.cohortDetailTab = 0
				a.openCohortDetail(cohort.ID)
				a.navigateTo(pageCohorts)
			}
			examBtn := textButton(c.Key("ov-exams-"+strconv.FormatInt(cohort.ID, 10)), locales.OverviewViewRecentExams())
			if examBtn.Clicked() {
				if card.LatestExamID > 0 {
					a.openExamForCohort(card.LatestExamID, cohort.ID)
				} else {
					a.selectedExamID = 0
					a.examCohortID = 0
					a.navigateTo(pageExams)
				}
			}
		})
	})
}

func (a *app) overviewTeacherOccupancySection(c *ui.Context, p uiPalette) {
	t := p.theme
	var activeTotal, singleCount, multiCount, idleCount, inactiveCount int
	for _, occ := range a.teacherOccupancy {
		if occ.Status == store.TeacherStatusInactive {
			inactiveCount++
			continue
		}
		activeTotal++
		switch occ.Status {
		case store.TeacherStatusActiveSingle:
			singleCount++
		case store.TeacherStatusActiveMulti:
			multiCount++
		case store.TeacherStatusIdle:
			idleCount++
		}
	}

	section(c, p, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			sectionHeading(c, p, locales.OverviewOccupancyTitle(), len(a.teacherOccupancy))
			ui.Spacer(c)
			if activeTotal > 0 {
				occupied := singleCount + multiCount
				ui.Text(c, fmt.Sprintf(locales.OverviewOccupancySummary(), formatInt(occupied), formatInt(idleCount))).
					FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
			}
		})
		if activeTotal > 0 {
			ratio := float64(singleCount+multiCount) / float64(activeTotal)
			sunbeam.Progress(c, ratio).Label(locales.OverviewOccupancyTitle())
		}
		ui.Row(c).FillWidth().Wrap().Gap(t.Space(2.5)).Children(func() {
			jump := func(filter string) {
				a.teacherOccupancyFilter = filter
				a.teacherProfileOpen = false
				a.navigateTo(pageFaculty)
			}
			if kpiCard(c, p, kpiCardProps{
				Key:       "occ-total",
				Title:     locales.OverviewOccupancyTotal(),
				Value:     formatInt(activeTotal),
				Clickable: true,
			}) {
				jump("")
			}
			if kpiCard(c, p, kpiCardProps{
				Key:       "occ-single",
				Title:     locales.OverviewOccupancySingle(),
				Value:     formatInt(singleCount),
				BadgeText: locales.OverviewOccupancySingle(),
				BadgeTone: toneSuccess,
				Clickable: true,
			}) {
				jump(store.TeacherStatusActiveSingle)
			}
			multiTone := toneNeutral
			if multiCount > 0 {
				multiTone = toneWarning
			}
			if kpiCard(c, p, kpiCardProps{
				Key:       "occ-multi",
				Title:     locales.OverviewOccupancyMulti(),
				Value:     formatInt(multiCount),
				BadgeText: locales.OverviewOccupancyMulti(),
				BadgeTone: multiTone,
				Clickable: true,
			}) {
				jump(store.TeacherStatusActiveMulti)
			}
			if kpiCard(c, p, kpiCardProps{
				Key:       "occ-idle",
				Title:     locales.OverviewOccupancyIdle(),
				Value:     formatInt(idleCount),
				BadgeText: locales.OverviewOccupancyIdle(),
				BadgeTone: toneAccent,
				Clickable: true,
			}) {
				jump(store.TeacherStatusIdle)
			}
			if kpiCard(c, p, kpiCardProps{
				Key:       "occ-inactive",
				Title:     locales.OverviewOccupancyInactive(),
				Value:     formatInt(inactiveCount),
				Clickable: true,
			}) {
				jump(store.TeacherStatusInactive)
			}
		})
	})
}

func (a *app) overviewRecentExamsCard(c *ui.Context, p uiPalette) {
	t := p.theme
	exams := a.schoolOverview.RecentExams
	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.OverviewRecentExamsTitle(), len(exams))
			ui.Spacer(c)
			if len(exams) > 0 {
				if textButton(c.Key("ov-all-exams"), locales.OverviewViewRecentExams()).Clicked() {
					a.selectedExamID = 0
					a.navigateTo(pageExams)
				}
			}
		})
		if len(exams) == 0 {
			smallEmptyState(c, p, locales.OverviewNoExams())
			return
		}
		limit := min(len(exams), 6)
		ui.Column(c).FillWidth().Gap(0).Children(func() {
			for idx := 0; idx < limit; idx++ {
				exam := exams[idx]
				if idx > 0 {
					rule(c)
				}
				title := a.examBatchLabel(exam)
				row := listRowButton(c.Key("ov-exam-"+strconv.FormatInt(exam.ID, 10)), title).
					Height(0).MinHeight(56).Padding(t.Space(2.5), t.Space(3)).AlignItems(ui.Center)
				row.Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, title).FontSize(bodySize(t)).FontWeight(500).
								TextColor(p.ink).SingleLine().Tooltip(title)
							ui.Textf(c, locales.ExamListMeta(), a.academicYearLabel(exam.AcademicYearStart),
								a.examTermName(exam.Term), exam.ExamDate).
								FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
						})
						if len(exam.Cohorts) > 0 {
							ui.Text(c, pluralCount(len(exam.Cohorts), locales.ExamCohortCountOne(), locales.ExamCohortCount())).
								FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
						}
						statusBadge(c, p, exam.Status, a.examStatusName(exam.Status))
					})
				})
				if row.Clicked() {
					a.openExamForCohort(exam.ID, 0)
				}
			}
		})
	})
}

type overviewAlertRow struct {
	Key    string
	Text   string
	Tone   badgeTone
	Action func()
}

func (a *app) overviewRemindersCard(c *ui.Context, p uiPalette) {
	t := p.theme
	var alerts []overviewAlertRow

	multiCount := 0
	for _, occ := range a.teacherOccupancy {
		if occ.Status == store.TeacherStatusActiveMulti {
			multiCount++
		}
	}
	if multiCount > 0 {
		alerts = append(alerts, overviewAlertRow{
			Key:  "cross-grade-teachers",
			Text: fmt.Sprintf(locales.OverviewReminderCrossGrade(), formatInt(multiCount)),
			Tone: toneWarning,
			Action: func() {
				a.teacherOccupancyFilter = store.TeacherStatusActiveMulti
				a.teacherProfileOpen = false
				a.navigateTo(pageFaculty)
			},
		})
	}

	for _, card := range a.schoolOverview.ActiveCohorts {
		card := card
		cname := a.cohortName(card.Cohort.EnrollmentYear)
		if card.BlockingIssues > 0 {
			alerts = append(alerts, overviewAlertRow{
				Key:  fmt.Sprintf("blk-%d", card.Cohort.ID),
				Text: fmt.Sprintf(locales.OverviewReminderCohortBlocking(), cname, formatInt(card.BlockingIssues)),
				Tone: toneDanger,
				Action: func() {
					a.qualitySeverity = 1
					a.navigateTo(pageQuality)
				},
			})
		}
		if card.ActiveStudents == 0 {
			alerts = append(alerts, overviewAlertRow{
				Key:  fmt.Sprintf("stu-%d", card.Cohort.ID),
				Text: fmt.Sprintf(locales.OverviewReminderCohortSetup(), cname, locales.CohortTabStudents()),
				Tone: toneWarning,
				Action: func() {
					a.cohortDetailTab = 2
					a.openCohortDetail(card.Cohort.ID)
					a.navigateTo(pageCohorts)
				},
			})
		}
		if card.AssignmentCount == 0 {
			alerts = append(alerts, overviewAlertRow{
				Key:  fmt.Sprintf("assign-%d", card.Cohort.ID),
				Text: fmt.Sprintf(locales.OverviewReminderCohortSetup(), cname, locales.CohortTabAssignments()),
				Tone: toneWarning,
				Action: func() {
					a.cohortDetailTab = 3
					a.openCohortDetail(card.Cohort.ID)
					a.navigateTo(pageCohorts)
				},
			})
		}
		if card.BaselineCount == 0 {
			alerts = append(alerts, overviewAlertRow{
				Key:  fmt.Sprintf("base-%d", card.Cohort.ID),
				Text: fmt.Sprintf(locales.OverviewReminderCohortSetup(), cname, locales.CohortTabBaselines()),
				Tone: toneWarning,
				Action: func() {
					a.cohortDetailTab = 4
					a.openCohortDetail(card.Cohort.ID)
					a.navigateTo(pageCohorts)
				},
			})
		}
		if card.LatestAssessmentID > 0 && !card.HasOfficialRun {
			assessID := card.LatestAssessmentID
			examName := card.LatestExamName
			alerts = append(alerts, overviewAlertRow{
				Key:  fmt.Sprintf("assess-%d", card.Cohort.ID),
				Text: fmt.Sprintf(locales.OverviewReminderCohortPendingAssess(), cname, examName),
				Tone: toneWarning,
				Action: func() {
					a.selectAssessment(assessID)
					a.navigateTo(pageAssessments)
				},
			})
		}
	}

	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.OverviewRemindersTitle(), len(alerts))
		if len(alerts) == 0 {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2.5)).Padding(t.Space(2), 0).Children(func() {
				ui.Icon(c, iconCircleCheck).Size(18, 18).TextColor(p.success).Shrink(0)
				ui.Text(c, locales.OverviewRemindersClear()).FontSize(bodySize(t)).
					TextColor(p.inkSecondary).Pretty().Grow(1).Shrink(1).MinWidth(0)
			})
			return
		}
		limit := min(len(alerts), 8)
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			for index := 0; index < limit; index++ {
				alert := alerts[index]
				if index > 0 {
					rule(c)
				}
				dotColor := p.warning
				if alert.Tone == toneDanger {
					dotColor = p.danger
				}
				row := linkRow(c, p, alert.Text, func() {
					ui.Box(c).Size(8, 8).Radius(4).Background(dotColor).Shrink(0)
					ui.Text(c, alert.Text).FontSize(bodySize(t)).TextColor(p.ink).
						Grow(1).Shrink(1).MinWidth(0).Pretty()
					ui.Icon(c, iconArrowRight).Size(15, 15).TextColor(p.muted).Shrink(0)
				})
				if row.Clicked() && alert.Action != nil {
					alert.Action()
				}
			}
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
				a.summaryMetricRow(c, p, locales.ReadinessClasses(), formatInt(a.stageCounts.Classes))
				a.summaryMetricRow(c, p, locales.StageReviewTeachingGroups(),
					formatInt(a.stageCounts.TeachingGroups))
				a.summaryMetricRow(c, p, locales.ReadinessAssignments(), formatInt(a.stageCounts.Assignments))
				a.summaryMetricRow(c, p, locales.ReadinessStudents(), formatInt(a.stageCounts.Students))
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
