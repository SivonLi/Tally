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
	if !a.workspaceSummaryLoaded && a.selectedCohort() != nil && a.selectedStage() != nil {
		a.reloadWorkspaceSummary(context.Background())
	}
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(5)).Children(func() {
			cohort := a.selectedCohort()
			var actions func()
			subtitle := locales.WorkspaceEmptyHint()
			if cohort != nil {
				subtitle = fmt.Sprintf(locales.OverviewLedgerSubtitle(), a.cohortName(cohort.EnrollmentYear))
				actions = func() {
					if labelledButton(c, locales.OverviewQuickNewExam(), iconPlus, false).Clicked() {
						a.navigateTo(pageExams)
						a.openNewExam()
					}
					if labelledButton(c, locales.OverviewQuickImport(), iconImport, true).Clicked() {
						a.openDataImport("", 0, 0)
					}
				}
			}
			pageHeading(c, p, locales.OverviewTitle(), subtitle, actions)
			if cohort == nil {
				emptyState(c, p, iconCohort, locales.WorkspaceEmpty(), locales.WorkspaceEmptyHint(), func() {
					if labelledButton(c, locales.WorkspaceCreate(), iconPlus, true).Clicked() {
						a.openNewCohort()
					}
				})
				return
			}
			a.overviewContextStrip(c, p, cohort)
			a.stageReviewBanner(c, p)
			a.overviewMetricsGrid(c, p)
			ui.Grid(c).FillWidth().
				ColumnTracks(ui.Fr(3).Min(t.Space(92)), ui.Fr(2).Min(t.Space(72))).
				Gap(t.Space(4)).Children(func() {
				a.overviewLatestAssessmentCard(c, p)
				a.overviewAlertsCard(c, p)
			})
			a.overviewExamTimelineCard(c, p)
			a.gaokaoPanel(c, p)
		})
	})
}

func (a *app) overviewContextStrip(c *ui.Context, p uiPalette, cohort *store.Cohort) {
	t := p.theme
	stage := a.displayStage()
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		ui.Textf(c, locales.CohortContext(), a.cohortName(cohort.EnrollmentYear),
			a.stageName(stage.GradeLevel),
			a.classOfName(cohort.GraduationYear)).
			FontSize(bodySize(t)).FontWeight(500).TextColor(p.inkSecondary).SingleLine().
			Shrink(1).MinWidth(0)
		if !a.stageEditable() && a.selectedStage() != nil {
			badge(c, p, locales.StageReadOnly(), toneNeutral)
		}
		ui.Spacer(c)
		ui.Text(c, a.academicYearLabel(stage.AcademicYearStart)).
			FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
	})
}

func (a *app) overviewMetricsGrid(c *ui.Context, p uiPalette) {
	t := p.theme
	ws := a.workspaceSummary
	stage := a.displayStage()

	studentSub := fmt.Sprintf(locales.OverviewMetricStudentsSub(),
		formatInt(ws.ActiveStudents), formatInt(ws.PhysicsStudents), formatInt(ws.HistoryStudents))
	studentBadge := ""
	if ws.InactiveStudents > 0 {
		studentBadge = fmt.Sprintf(locales.OverviewMetricStudentsInactive(), formatInt(ws.InactiveStudents))
	}

	teacherSub := fmt.Sprintf(locales.OverviewMetricTeachersSub(),
		formatInt(ws.Counts.Assignments), formatInt(ws.Counts.TeachingGroups))

	baselineVal := formatInt(ws.Counts.Baselines)
	if ws.Counts.Classes > 0 {
		baselineVal = fmt.Sprintf("%s/%s", formatInt(ws.BaselineCoveredClasses), formatInt(ws.Counts.Classes))
	}
	baselineSub := fmt.Sprintf(locales.OverviewMetricBaselinesSub(),
		formatInt(ws.BaselineCoveredClasses), formatInt(ws.Counts.Classes), formatInt(ws.BaselineSubjectCount))
	if ws.GradeBaselineTotal > 0 {
		baselineSub = fmt.Sprintf(locales.OverviewMetricBaselinesAvg(), strconv.FormatFloat(ws.GradeBaselineTotal, 'f', 1, 64))
	}
	baselineBadge := ""
	baselineTone := toneNeutral
	if ws.Counts.Classes > 0 && ws.BaselineCoveredClasses >= ws.Counts.Classes {
		baselineBadge = locales.ImportCenterChecklistReady()
		baselineTone = toneSuccess
	} else if ws.Counts.Baselines == 0 {
		baselineBadge = locales.ImportCenterChecklistMissing()
		baselineTone = toneWarning
	}

	classSub := fmt.Sprintf(locales.StageOption(), a.stageName(stage.GradeLevel),
		school.AcademicYearLabel(stage.AcademicYearStart))

	ui.Grid(c).FillWidth().
		ColumnTracks(
			ui.Fr(1).Min(t.Space(42)),
			ui.Fr(1).Min(t.Space(42)),
			ui.Fr(1).Min(t.Space(42)),
			ui.Fr(1).Min(t.Space(42)),
		).Gap(t.Space(3)).Children(func() {
		if kpiCard(c, p, kpiCardProps{
			Key:       "overview-classes",
			Title:     locales.OverviewMetricClasses(),
			Value:     formatInt(ws.Counts.Classes),
			Subtitle:  classSub,
			Clickable: true,
		}) {
			a.baseDataTab = 0
			a.navigateTo(pageBaseData)
		}
		if kpiCard(c, p, kpiCardProps{
			Key:       "overview-students",
			Title:     locales.OverviewMetricStudents(),
			Value:     formatInt(ws.Counts.Students),
			Subtitle:  studentSub,
			BadgeText: studentBadge,
			BadgeTone: toneNeutral,
			Clickable: true,
		}) {
			a.baseDataTab = 1
			a.navigateTo(pageBaseData)
		}
		if kpiCard(c, p, kpiCardProps{
			Key:       "overview-teachers",
			Title:     locales.OverviewMetricTeachers(),
			Value:     formatInt(ws.Counts.Teachers),
			Subtitle:  teacherSub,
			Clickable: true,
		}) {
			a.baseDataTab = 2
			a.navigateTo(pageBaseData)
		}
		if kpiCard(c, p, kpiCardProps{
			Key:       "overview-baselines",
			Title:     locales.OverviewMetricBaselines(),
			Value:     baselineVal,
			Subtitle:  baselineSub,
			BadgeText: baselineBadge,
			BadgeTone: baselineTone,
			Clickable: true,
		}) {
			a.baseDataTab = 4
			a.navigateTo(pageBaseData)
		}
	})
}

func (a *app) overviewLatestAssessmentCard(c *ui.Context, p uiPalette) {
	t := p.theme
	snap := a.workspaceSummary.LatestAssessment
	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.OverviewLatestAssessment(), -1)
			ui.Spacer(c)
			if snap != nil {
				switch {
				case snap.HasOfficialRun && snap.OfficialStale:
					badge(c, p, locales.AssessmentStatusStale(), toneWarning)
				case snap.HasOfficialRun:
					badge(c, p, locales.OverviewLatestOfficial(), toneSuccess)
				default:
					badge(c, p, locales.OverviewLatestPending(), toneNeutral)
				}
			}
		})
		if snap == nil {
			smallEmptyState(c, p, locales.OverviewNoExams())
			ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
				if labelledButton(c, locales.OverviewQuickNewExam(), iconPlus, false).Clicked() {
					a.navigateTo(pageExams)
					a.openNewExam()
				}
			})
			return
		}
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			ui.Text(c, snap.ExamName).FontSize(sectionSize(t)).FontWeight(600).
				TextColor(p.ink).SingleLine().Tooltip(snap.ExamName)
			ui.Textf(c, locales.ExamListMeta(), a.academicYearLabel(a.displayStage().AcademicYearStart),
				a.examTermName(snap.Term), snap.ExamDate).
				FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
		})
		if snap.HasOfficialRun {
			ui.Row(c).FillWidth().Wrap().Gap(t.Space(6)).Padding(t.Space(1), 0).Children(func() {
				metric(c, p, formatInt(snap.ResultRowCount), locales.AssessmentResults())
				metric(c, p, formatInt(snap.PersonalScoreCount), locales.ColumnPersonalScore())
				if snap.GradeAverage > 0 {
					metric(c, p, strconv.FormatFloat(snap.GradeAverage, 'f', 1, 64), locales.AssessmentDetailGradeAverage())
				}
				if snap.RosterCount > 0 {
					metric(c, p, formatInt(snap.RosterCount), locales.OverviewMetricStudents())
				}
			})
			if snap.OfficialStale {
				notice(c, p, noticeWarning, locales.OverviewLatestStale())
			}
		} else {
			ui.Text(c, locales.OverviewLatestPending()).FontSize(bodySize(t)).TextColor(p.muted)
		}
		ui.Row(c).FillWidth().Wrap().Gap(t.Space(2)).Padding(t.Space(1), 0, 0, 0).Children(func() {
			if labelledButton(c, locales.OverviewOpenWorkbench(), iconBatches, false).Clicked() {
				a.selectAssessment(snap.AssessmentID)
				a.navigateTo(pageAssessments)
			}
			if snap.HasOfficialRun {
				if labelledButton(c, locales.OverviewViewResults(), iconResults, false).Clicked() {
					a.selectAssessment(snap.AssessmentID)
					a.navigateTo(pageResults)
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

func (a *app) overviewAlertsCard(c *ui.Context, p uiPalette) {
	t := p.theme
	ws := a.workspaceSummary
	var alerts []overviewAlertRow
	if ws.BlockingIssues > 0 {
		alerts = append(alerts, overviewAlertRow{
			Key:  "blocking",
			Text: fmt.Sprintf(locales.OverviewAlertBlocking(), formatInt(ws.BlockingIssues)),
			Tone: toneDanger,
			Action: func() {
				a.qualitySeverity = 1
				a.navigateTo(pageQuality)
			},
		})
	}
	if ws.WarningIssues > 0 {
		alerts = append(alerts, overviewAlertRow{
			Key:  "warning",
			Text: fmt.Sprintf(locales.OverviewAlertWarning(), formatInt(ws.WarningIssues)),
			Tone: toneWarning,
			Action: func() {
				a.qualitySeverity = 2
				a.navigateTo(pageQuality)
			},
		})
	}
	if ws.LatestAssessment != nil && ws.LatestAssessment.OfficialStale {
		assessID := ws.LatestAssessment.AssessmentID
		alerts = append(alerts, overviewAlertRow{
			Key:  "stale",
			Text: locales.OverviewLatestStale(),
			Tone: toneWarning,
			Action: func() {
				a.selectAssessment(assessID)
				a.navigateTo(pageAssessments)
			},
		})
	}
	if ws.Counts.Students == 0 {
		alerts = append(alerts, overviewAlertRow{
			Key:  "missing-students",
			Text: locales.OverviewAlertNoRoster(),
			Tone: toneWarning,
			Action: func() {
				a.openDataImport(importer.KindStudents, 0, 0)
			},
		})
	}
	if ws.Counts.Assignments == 0 {
		alerts = append(alerts, overviewAlertRow{
			Key:  "missing-assignments",
			Text: locales.OverviewAlertNoAssignments(),
			Tone: toneWarning,
			Action: func() {
				a.openDataImport(importer.KindAssignments, 0, 0)
			},
		})
	}
	if ws.Counts.Baselines == 0 {
		alerts = append(alerts, overviewAlertRow{
			Key:  "missing-baselines",
			Text: locales.OverviewAlertNoBaselines(),
			Tone: toneWarning,
			Action: func() {
				a.openDataImport(importer.KindBaselines, 0, 0)
			},
		})
	}

	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.OverviewAlertsTitle(), len(alerts))
		if len(alerts) == 0 {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2.5)).Padding(t.Space(2), 0).Children(func() {
				ui.Icon(c, iconCircleCheck).Size(18, 18).TextColor(p.success).Shrink(0)
				ui.Text(c, locales.OverviewAlertsClear()).FontSize(bodySize(t)).
					TextColor(p.inkSecondary).Pretty().Grow(1).Shrink(1).MinWidth(0)
			})
			return
		}
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			for index, alert := range alerts {
				alert := alert
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

func (a *app) overviewExamTimelineCard(c *ui.Context, p uiPalette) {
	t := p.theme
	ws := a.workspaceSummary
	panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.OverviewExamTimeline(), -1)
			ui.Spacer(c)
			ui.Textf(c, locales.OverviewExamCountSub(), formatInt(ws.CalculatedExamCount), formatInt(ws.ExamCount)).
				FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
		})
		if len(ws.Exams) == 0 {
			smallEmptyState(c, p, locales.OverviewNoExams())
			return
		}
		limit := min(len(ws.Exams), 6)
		ui.Column(c).FillWidth().Gap(0).Children(func() {
			for idx := 0; idx < limit; idx++ {
				exam := ws.Exams[idx]
				if idx > 0 {
					rule(c)
				}
				title := fmt.Sprintf(locales.LabelQualifier(), exam.ExamName, a.examTypeName(exam.ExamType))
				row := listRowButton(c.Key("ov-exam-"+strconv.FormatInt(exam.ExamID, 10)), title).
					Height(0).MinHeight(t.Space(14)).Padding(t.Space(3), t.Space(3)).AlignItems(ui.Center)
				row.Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, title).FontSize(bodySize(t)).FontWeight(500).
								TextColor(p.ink).SingleLine().Tooltip(title)
							ui.Textf(c, locales.ExamListMeta(), a.academicYearLabel(a.displayStage().AcademicYearStart),
								a.examTermName(exam.Term), exam.ExamDate).
								FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
						})
						if exam.RosterCount > 0 {
							metricInline(c, p, uint64(exam.RosterCount), locales.OverviewMetricStudents())
						}
						if exam.Calculated {
							ui.Text(c, strconv.FormatFloat(exam.AverageTotal, 'f', 2, 64)).
								FontSize(bodySize(t)).FontWeight(600).FontFeatures("tnum").
								TextColor(p.ink).Shrink(0)
							badge(c, p, locales.OverviewLatestOfficial(), toneSuccess)
						} else {
							badge(c, p, locales.OverviewLatestPending(), toneNeutral)
						}
					})
				})
				if row.Clicked() {
					if exam.Calculated && exam.AssessmentID > 0 {
						a.selectAssessment(exam.AssessmentID)
						a.navigateTo(pageAssessments)
					} else {
						a.selectExam(exam.ExamID)
						a.navigateTo(pageExams)
					}
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
				a.openDataImport(importer.KindClasses, 0, 0)
			}
			if len(a.cohorts) > 0 {
				if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
					a.openNewCohort()
				}
			}
		})
		if len(a.cohorts) == 0 {
			emptyState(c, p, iconCohort, locales.WorkspaceEmpty(), locales.WorkspaceEmptyHint(), func() {
				if labelledButton(c, locales.WorkspaceCreate(), iconPlus, true).Clicked() {
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
