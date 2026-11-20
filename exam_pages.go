package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"tally/internal/importer"
	"tally/internal/store"
	"tally/locales"
)

func (a *app) examsPage(c *ui.Context, p uiPalette) {
	if a.examPrepOpen {
		a.examPrepPage(c, p)
		return
	}
	if a.store == nil {
		return
	}
	if a.selectedExamID != 0 {
		a.examDetailPage(c, p)
		return
	}
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavExams(), locales.ExamsPageSubtitle(), func() {
			if labelledButton(c, locales.ExamNew(), iconPlus, true).Clicked() {
				a.openNewExam()
			}
		})
		if a.examError != "" {
			notice(c, p, noticeDanger, a.examError)
		}
		if a.examMessage != "" {
			notice(c, p, noticeSuccess, a.examMessage)
		}
		if len(a.exams) == 0 {
			emptyState(c, p, iconExams, locales.ExamsEmpty(), locales.ExamsEmptyHint(), func() {
				if labelledButton(c, locales.ExamNew(), iconPlus, true).Clicked() {
					a.openNewExam()
				}
			})
			return
		}
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				for _, exam := range a.exams {
					exam := exam
					row := linkRow(c, p, a.examBatchLabel(exam), func() {
						ui.Column(c).Grow(1).Gap(0).Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, a.examBatchLabel(exam)).FontSize(bodySize(t)).Bold()
								badge(c, p, a.examTypeName(exam.ExamType), toneAccent)
							})
							ui.Textf(c, locales.ExamListMeta(), a.academicYearLabel(exam.AcademicYearStart),
								a.examTermName(exam.Term), exam.ExamDate).
								FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
						})
						ui.Column(c).Gap(0).AlignItems(ui.End).Children(func() {
							ui.Textf(c, locales.ExamCohortCount(), len(exam.Cohorts)).
								FontSize(bodySize(t)).FontFeatures("tnum")
							statusBadge(c, p, exam.Status, a.examStatusName(exam.Status))
						})
					})
					if row.Clicked() {
						a.selectExam(exam.ID)
					}
				}
			})
		})
	})
	a.newExamModal(c, p)
}

func (a *app) examDetailPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.exam.ID == 0 {
		a.selectedExamID = 0
		a.examsPage(c, p)
		return
	}
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			if ui.Button(c, "").Label(locales.Back()).Children(func() {
				ui.Icon(c, iconArrowLeft).FontSize(t.Space(4))
				ui.Text(c, locales.Back()).FontSize(bodySize(t))
			}).Clicked() {
				a.selectedExamID = 0
				a.exam = store.Exam{}
				a.examError = ""
				a.examMessage = ""
				return
			}
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Text(c, a.examBatchLabel(a.exam)).FontSize(titleSize(t)).Bold().
						Role(ui.RoleHeading).Level(1)
					badge(c, p, a.examTypeName(a.exam.ExamType), toneAccent)
				})
				ui.Textf(c, locales.ExamDetailMeta(), a.examAcademicYearLabel(),
					a.examTermName(a.exam.Term), a.exam.ExamDate).
					FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
			})
			ui.Row(c).Gap(t.Space(2)).Children(func() {
				if labelledButton(c, locales.ExamPrepOpen(), iconQuality, false).Clicked() {
					a.examPrepOpen = true
				}
				if labelledButton(c, locales.ExamNew(), iconPlus, false).Clicked() {
					a.openNewExam()
				}
			})
		})
		if len(a.exam.Cohorts) > 1 {
			labels := make([]string, 0, len(a.exam.Cohorts))
			for _, cohort := range a.exam.Cohorts {
				labels = append(labels, a.examCohortLabel(cohort))
			}
			chosen := ""
			for index, cohort := range a.exam.Cohorts {
				if cohort.CohortID == a.examCohortID {
					chosen = labels[index]
				}
			}
			next := chosen
			ui.Select(c, &next, labels).Label(locales.ExamCohortSelect()).Width(t.Space(72))
			if next != chosen {
				for index, label := range labels {
					if label == next {
						a.examCohortID = a.exam.Cohorts[index].CohortID
						a.reloadExamData(context.Background())
						break
					}
				}
			}
		}
		if a.examError != "" {
			notice(c, p, noticeDanger, a.examError)
		}
		if a.examMessage != "" {
			notice(c, p, noticeSuccess, a.examMessage)
		}
		a.examSummary(c, p)
		ui.Segmented(c, &a.examTab,
			locales.ExamTabRoster(), locales.ExamTabScores(), locales.ExamTabCutoffs(),
			locales.ExamTabTargets(), locales.ExamTabGroups(), locales.ExamTabQuality()).
			Label(locales.ExamViewLabel())
		switch a.examTab {
		case 1:
			a.examScoresTab(c, p)
		case 2:
			a.examCutoffsTab(c, p)
		case 3:
			a.examTargetsTab(c, p)
		case 4:
			a.examStatGroupsTab(c, p)
		case 5:
			a.examQualityTab(c, p)
		default:
			a.examRosterTab(c, p)
		}
	})
	a.newExamModal(c, p)
	a.statGroupModal(c, p)
}

func (a *app) examSummary(c *ui.Context, p uiPalette) {
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(3)).Gap(t.Space(2)).Children(func() {
		ui.Row(c).Wrap().Gap(t.Space(6)).Children(func() {
			metric(c, p, fmtInt(a.examCohort.RosterCount), locales.ExamTabRoster())
			metric(c, p, fmtInt(a.examCohort.ScoreCount), locales.ExamTabScores())
			metric(c, p, fmtInt(len(a.examSubjectCutoffs)+len(a.examTierCutoffs)), locales.ExamTabCutoffs())
			metric(c, p, fmtInt(a.examCohort.ClassTargetCount), locales.ExamTabTargets())
			metric(c, p, fmtInt(len(a.examStatGroups)), locales.ExamTabGroups())
			blocking, warnings := examIssueCounts(a.examQuality)
			metric(c, p, fmtInt(blocking), locales.ExamQualityBlocking())
			metric(c, p, fmtInt(warnings), locales.ExamQualityWarnings())
		})
	})
}

func examIssueCounts(issues []store.ExamQualityIssue) (int, int) {
	blocking := 0
	warnings := 0
	for _, issue := range issues {
		if issue.Severity == "blocking" {
			blocking++
		} else {
			warnings++
		}
	}
	return blocking, warnings
}

func (a *app) examRosterTab(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Grow(1).Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.End).Wrap().Gap(t.Space(2)).Children(func() {
			ui.Column(c).Width(t.Space(56)).Children(func() {
				next := a.examRosterQuery
				ui.SearchField(c, &next).Label(locales.ExamSearchRoster())
				if next != a.examRosterQuery {
					a.examRosterQuery = next
					a.reloadExamData(context.Background())
				}
			})
			trackLabels := make([]string, 0, len(a.tracks))
			trackCodes := make([]string, 0, len(a.tracks))
			for _, track := range a.tracks {
				if !track.Enabled {
					continue
				}
				trackLabels = append(trackLabels, a.trackName(track.Key, track.Name))
				trackCodes = append(trackCodes, track.Key)
			}
			chosenTrack := ""
			for index, code := range trackCodes {
				if code == a.examDefaultTrack {
					chosenTrack = trackLabels[index]
				}
			}
			if chosenTrack == "" && len(trackLabels) > 0 {
				chosenTrack = trackLabels[0]
			}
			nextTrack := chosenTrack
			ui.Select(c, &nextTrack, trackLabels).Label(locales.ExamDefaultTrack()).Width(t.Space(24))
			if nextTrack != chosenTrack {
				for index, label := range trackLabels {
					if label == nextTrack {
						a.examDefaultTrack = trackCodes[index]
						break
					}
				}
			}
			ui.Spacer(c)
			if labelledButton(c, locales.ExamGenerateRoster(), iconStudents, false).Clicked() {
				a.generateExamRoster()
			}
			if labelledButton(c, locales.ExamImportRoster(), iconImports, true).Clicked() {
				a.openExamImportWizard(importer.KindRoster, a.exam.ID, a.examCohort.CohortID)
			}
		})
		if len(a.examRoster) == 0 {
			emptyState(c, p, iconStudents, locales.ExamRosterEmpty(), locales.ExamRosterEmptyHint(), nil)
			return
		}
		ui.Textf(c, locales.DataRecordCount(), a.examRosterTotal).FontSize(captionSize(t)).
			TextColor(p.muted).FontFeatures("tnum")
		columns := []ui.TableColumn{
			{Title: locales.ColumnStudentNo(), Width: t.Space(18)},
			{Title: locales.ColumnName(), Width: t.Space(16)},
			{Title: locales.ColumnClass(), Width: t.Space(16)},
			{Title: locales.ImportFieldExamNo(), Width: t.Space(16)},
			{Title: locales.ColumnTrack(), Width: t.Space(16)},
			{Title: locales.ColumnStatus(), Width: 0},
		}
		ui.Table(c, &a.studentTable, columns, len(a.examRoster), func(row, col int) {
			entry := a.examRoster[row]
			switch col {
			case 0:
				ui.Text(c, entry.StudentNo).SingleLine().FontFeatures("tnum")
			case 1:
				ui.Text(c, entry.Name).SingleLine()
			case 2:
				ui.Text(c, entry.ClassName).SingleLine()
			case 3:
				ui.Text(c, entry.ExamNo).TextColor(p.muted).SingleLine().FontFeatures("tnum")
			case 4:
				ui.Text(c, a.trackName(entry.Track, entry.Track)).SingleLine()
			case 5:
				ui.Text(c, a.statusName(entry.Status)).TextColor(p.muted).SingleLine()
			}
		}).Label(locales.ExamTabRoster()).Grow(1).FillWidth()
	})
}

func (a *app) examScoresTab(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Grow(1).Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.End).Wrap().Gap(t.Space(2)).Children(func() {
			ui.Column(c).Width(t.Space(52)).Children(func() {
				next := a.examScoreQuery
				ui.SearchField(c, &next).Label(locales.ExamSearchScores())
				if next != a.examScoreQuery {
					a.examScoreQuery = next
					a.reloadExamData(context.Background())
				}
			})
			subjectLabels, subjectCodes := a.examSubjectLabels()
			chosenSubject := subjectLabels[0]
			for index, code := range subjectCodes {
				if code == a.examScoreSubject {
					chosenSubject = subjectLabels[index]
				}
			}
			nextSubject := chosenSubject
			ui.Select(c, &nextSubject, subjectLabels).Label(locales.DataFilterSubject()).Width(t.Space(24))
			if nextSubject != chosenSubject {
				for index, label := range subjectLabels {
					if label == nextSubject {
						a.examScoreSubject = subjectCodes[index]
						a.reloadExamData(context.Background())
						break
					}
				}
			}
			classLabels, classIDs := a.examClassLabels()
			chosenClass := classLabels[0]
			for index, id := range classIDs {
				if id == a.examScoreClass {
					chosenClass = classLabels[index]
				}
			}
			nextClass := chosenClass
			ui.Select(c, &nextClass, classLabels).Label(locales.DataFilterClass()).Width(t.Space(24))
			if nextClass != chosenClass {
				for index, label := range classLabels {
					if label == nextClass {
						a.examScoreClass = classIDs[index]
						a.reloadExamData(context.Background())
						break
					}
				}
			}
			ui.Spacer(c)
			if labelledButton(c, locales.ExamImportScores(), iconImports, true).Clicked() {
				a.openExamImportWizard(importer.KindScores, a.exam.ID, a.examCohort.CohortID)
			}
		})
		if len(a.examScoreSummaries) > 0 {
			ui.Row(c).Wrap().Gap(t.Space(4)).Children(func() {
				for _, summary := range a.examScoreSummaries {
					panel(c, p).Padding(t.Space(2)).Width(t.Space(34)).Gap(0).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Children(func() {
							ui.Text(c, a.subjectName(summary.Subject, summary.Subject)).
								FontSize(bodySize(t)).Bold().Grow(1)
							ui.Textf(c, locales.ExamAverage(), summary.Average).FontSize(captionSize(t)).
								TextColor(p.muted).FontFeatures("tnum")
						})
						ui.Textf(c, locales.ExamScoreMeta(), summary.Count, summary.Missing).
							FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
					})
				}
			})
		}
		a.examScoreStatistics(c, p)
		if len(a.examScores) == 0 {
			emptyState(c, p, iconExams, locales.ExamScoresEmpty(), locales.ExamScoresEmptyHint(), nil)
			return
		}
		ui.Textf(c, locales.DataRecordCount(), a.examScoresTotal).FontSize(captionSize(t)).
			TextColor(p.muted).FontFeatures("tnum")
		columns := []ui.TableColumn{
			{Title: locales.ColumnStudentNo(), Width: t.Space(18)},
			{Title: locales.ColumnName(), Width: t.Space(16)},
			{Title: locales.ColumnClass(), Width: t.Space(16)},
			{Title: locales.BaselineSubject(), Width: t.Space(16)},
			{Title: locales.ExamScore(), Width: t.Space(14), Align: ui.End},
			{Title: locales.ExamFlag(), Width: 0},
		}
		ui.Table(c, &a.baselineTable, columns, len(a.examScores), func(row, col int) {
			item := a.examScores[row]
			switch col {
			case 0:
				ui.Text(c, item.StudentNo).SingleLine().FontFeatures("tnum")
			case 1:
				ui.Text(c, item.Name).SingleLine()
			case 2:
				ui.Text(c, item.ClassName).SingleLine()
			case 3:
				ui.Text(c, a.subjectName(item.Subject, item.Subject)).SingleLine()
			case 4:
				if item.Score == nil {
					ui.Text(c, "-").TextColor(p.muted)
				} else {
					ui.Textf(c, "%.1f", *item.Score).FontFeatures("tnum")
				}
			case 5:
				if item.Flag == "" {
					ui.Text(c, locales.ExamFlagNone()).TextColor(p.muted)
				} else {
					badge(c, p, a.examFlagName(item.Flag), toneWarning)
				}
			}
		}).Label(locales.ExamTabScores()).Grow(1).FillWidth()
	})
}

func (a *app) examScoreStatistics(c *ui.Context, p uiPalette) {
	t := p.theme
	subject := a.examScoreSubject
	if subject == "" && len(a.examScoreSummaries) > 0 {
		for _, summary := range a.examScoreSummaries {
			if summary.Count == 0 {
				continue
			}
			for _, band := range a.examScoreBands {
				if band.Subject == summary.Subject {
					subject = summary.Subject
					break
				}
			}
			if subject != "" {
				break
			}
		}
		if subject == "" {
			subject = a.examScoreSummaries[0].Subject
		}
	}
	bands := make([]store.ExamScoreBand, 0, 10)
	for _, band := range a.examScoreBands {
		if band.Subject == subject {
			bands = append(bands, band)
		}
	}
	averages := make([]store.ExamClassAverage, 0, len(a.examClasses))
	for _, average := range a.examClassAverages {
		if average.Subject == subject {
			averages = append(averages, average)
		}
	}
	if len(bands) == 0 {
		return
	}
	panel(c, p).FillWidth().Padding(t.Space(3)).Gap(t.Space(2)).Children(func() {
		ui.Textf(c, locales.ExamScoreDistribution(), a.subjectName(subject, subject)).
			FontSize(bodySize(t)).Bold()
		labels := make([]string, 0, len(bands))
		values := make([]float64, 0, len(bands))
		for _, band := range bands {
			labels = append(labels, fmt.Sprintf("%.0f–%.0f", band.Lower, band.Upper))
			values = append(values, float64(band.Count))
		}
		x := chart.NewBand(labels...).Padding(0.25, 0.1)
		_, high := chart.Extent(values...)
		if high < 1 {
			high = 1
		}
		y := chart.NewLinear(0, high).Nice(4)
		chart.Plot(c, chart.Axes{Left: y.Ticks(4), Bottom: x.Ticks()},
			func(area ui.Element) {
				for index, band := range bands {
					bar := chart.Rect(c, x.Band(index), y.Span(0, float64(band.Count))).
						Background(p.accent).Radius(t.Space(0.5))
					bar.Label(labels[index] + " " + fmtInt(band.Count))
					bar.Tooltip(labels[index] + ": " + fmtInt(band.Count))
				}
			}).Label(locales.ExamScoreDistribution()).Height(t.Space(48)).Gap(t.Space(2))
		if len(averages) > 0 {
			ui.Divider(c)
			ui.Text(c, locales.ExamClassComparison()).FontSize(bodySize(t)).Bold()
			ui.Row(c).Wrap().Gap(t.Space(4)).Children(func() {
				for _, average := range averages {
					ui.Textf(c, "%s · %.1f (%d)", average.ClassName, average.Average, average.Count).
						FontSize(captionSize(t)).TextColor(p.inkSecondary).FontFeatures("tnum")
				}
			})
		}
	})
}

func (a *app) examFlagName(flag string) string {
	switch flag {
	case "absent":
		return locales.ExamFlagAbsent()
	case "violation":
		return locales.ExamFlagViolation()
	case "cheating":
		return locales.ExamFlagCheating()
	case "invalid":
		return locales.ExamFlagInvalid()
	default:
		return flag
	}
}

func (a *app) examCutoffsTab(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Grow(1).Gap(t.Space(3)).Children(func() {
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			if labelledButton(c, locales.ExamImportCutoffs(), iconImports, true).Clicked() {
				a.openExamImportWizard(importer.KindCutoffs, a.exam.ID, a.examCohort.CohortID)
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
				sectionHeading(c, p, locales.ExamSubjectCutoffs(), len(a.examSubjectCutoffs))
				if len(a.examSubjectCutoffs) == 0 {
					smallEmptyState(c, p, locales.ExamCutoffsEmpty())
				} else {
					for _, cutoff := range a.examSubjectCutoffs {
						ui.Row(c).FillWidth().Gap(t.Space(2)).Padding(t.Space(1.5), 0).Children(func() {
							ui.Text(c, a.subjectName(cutoff.Subject, cutoff.Subject)).Width(t.Space(24))
							ui.Text(c, a.trackName(cutoff.Track, cutoff.Track)).Width(t.Space(18)).
								TextColor(p.muted)
							ui.Textf(c, "%.1f", cutoff.Cutoff).FontFeatures("tnum")
						})
					}
				}
				sectionHeading(c, p, locales.ExamTierCutoffs(), len(a.examTierCutoffs))
				if len(a.examTierCutoffs) == 0 {
					smallEmptyState(c, p, locales.ExamCutoffsEmpty())
				} else {
					for _, cutoff := range a.examTierCutoffs {
						ui.Row(c).FillWidth().Gap(t.Space(2)).Padding(t.Space(1.5), 0).Children(func() {
							ui.Text(c, a.tierName(cutoff.TierKey)).Width(t.Space(24))
							ui.Text(c, a.trackName(cutoff.Track, cutoff.Track)).Width(t.Space(18)).
								TextColor(p.muted)
							ui.Textf(c, "%.1f", cutoff.Cutoff).FontFeatures("tnum")
						})
					}
				}
				sectionHeading(c, p, locales.ExamCutoffImpact(), len(a.examCutoffImpacts))
				if len(a.examCutoffImpacts) == 0 {
					smallEmptyState(c, p, locales.ExamCutoffsEmpty())
				} else {
					for _, impact := range a.examCutoffImpacts {
						ui.Row(c).FillWidth().Gap(t.Space(2)).Padding(t.Space(1.5), 0).Children(func() {
							ui.Text(c, a.subjectName(impact.Subject, impact.Subject)).Width(t.Space(24))
							ui.Text(c, a.trackName(impact.Track, impact.Track)).Width(t.Space(18)).
								TextColor(p.muted)
							ui.Textf(c, locales.ExamCutoffImpactValue(), impact.Cutoff,
								impact.Reached, impact.Rostered, impact.ReachedRatio*100).
								FontFeatures("tnum").TextColor(p.inkSecondary).Grow(1)
						})
					}
				}
			})
		})
	})
}

func (a *app) examTargetsTab(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Grow(1).Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, locales.ExamTargetsHint()).FontSize(captionSize(t)).TextColor(p.muted).Grow(1).
				Pretty()
			if labelledButton(c, locales.ExamImportTargets(), iconImports, false).Clicked() {
				a.openExamImportWizard(importer.KindTargets, a.exam.ID, a.examCohort.CohortID)
			}
			if labelledButton(c, locales.ExamSaveTargets(), iconCheck, true).Clicked() {
				a.saveExamTargets()
			}
		})
		if len(a.examClasses) == 0 || len(a.examTierKeys) == 0 {
			emptyState(c, p, iconBaseline, locales.ExamTargetsEmpty(), locales.ExamTargetsEmptyHint(), nil)
			return
		}
		columns := []ui.TableColumn{{Title: locales.ColumnClass(), Width: t.Space(18)}}
		for _, tier := range a.examTierKeys {
			columns = append(columns, ui.TableColumn{Title: a.tierName(tier), Width: t.Space(16), Align: ui.End})
		}
		ui.Table(c, &a.classTable, columns, len(a.examClasses), func(row, col int) {
			class := a.examClasses[row]
			if col == 0 {
				ui.Text(c, class.Name).SingleLine()
				return
			}
			tier := a.examTierKeys[col-1]
			value := a.examTargetDrafts[examTargetKey(class.ID, tier)]
			if value == nil {
				empty := 0.0
				value = &empty
			}
			ui.NumberInput(c, value, 0, 10000, 1).Label(class.Name + " " + a.tierName(tier))
		}).Label(locales.ExamTabTargets()).Grow(1).FillWidth()
	})
}

func (a *app) examStatGroupsTab(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Grow(1).Gap(t.Space(2)).Children(func() {
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			if labelledButton(c, locales.ExamNewStatGroup(), iconPlus, true).Clicked() {
				a.openStatGroupEditor(nil)
			}
		})
		if len(a.examStatGroups) == 0 {
			emptyState(c, p, iconGroups, locales.ExamGroupsEmpty(), locales.ExamGroupsEmptyHint(), nil)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				for _, group := range a.examStatGroups {
					group := group
					panel(c, p).FillWidth().Padding(t.Space(3)).Gap(t.Space(1)).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Text(c, group.Name).FontSize(bodySize(t)).Bold().Grow(1)
							badge(c, p, a.subjectName(group.Subject, group.Subject), toneAccent)
							if group.Target == nil || *group.Target == 0 {
								badge(c, p, locales.ExamTargetMissing(), toneWarning)
							}
						})
						ui.Textf(c, locales.ExamStatGroupMeta(), group.LeadClassName,
							statGroupTargetText(group)).FontSize(captionSize(t)).TextColor(p.muted)
						members := make([]string, 0, len(group.Members))
						for _, member := range group.Members {
							label := member.Name
							if member.ClassNames != "" {
								label += " · " + member.ClassNames
							}
							members = append(members, label)
						}
						ui.Text(c, strings.Join(members, locales.ListSeparator())).
							FontSize(bodySize(t)).TextColor(p.inkSecondary).Pretty()
						ui.Row(c).Gap(t.Space(2)).Children(func() {
							if ui.Button(c, locales.Edit()).Clicked() {
								a.openStatGroupEditor(&group)
							}
							if ui.Button(c, locales.Delete()).Clicked() {
								a.statGroupDeleteOpen = true
								a.statGroupDeleteID = group.ID
								a.statGroupDeleteName = group.Name
							}
						})
					})
				}
			})
		})
	})
	if a.statGroupDeleteOpen {
		title := fmt.Sprintf(locales.ExamStatGroupDeleteTitle(), a.statGroupDeleteName)
		switch ui.AlertDialog(c, &a.statGroupDeleteOpen, title, locales.ExamStatGroupDeleteBody(),
			locales.PlanCancel(), locales.Delete()) {
		case 1:
			a.deleteStatGroup(a.statGroupDeleteID)
		}
	}
}

func statGroupTargetText(group store.ExamStatGroup) string {
	if group.Target == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f", *group.Target)
}

func (a *app) examQualityTab(c *ui.Context, p uiPalette) {
	t := p.theme
	blocking, warnings := examIssueCounts(a.examQuality)
	ui.Column(c).Grow(1).Gap(t.Space(2)).Children(func() {
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			badge(c, p, fmt.Sprintf(locales.ExamQualityBlockingCount(), blocking), toneDanger)
			badge(c, p, fmt.Sprintf(locales.ExamQualityWarningCount(), warnings), toneWarning)
			ui.Spacer(c)
			if ui.Button(c, locales.ExamRevalidate()).Clicked() {
				a.reloadExamData(context.Background())
			}
		})
		if len(a.examQuality) == 0 {
			emptyState(c, p, iconQuality, locales.ExamQualityEmpty(), locales.ExamQualityEmptyHint(), nil)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
				for _, issue := range a.examQuality {
					issue := issue
					tone := toneWarning
					if issue.Severity == "blocking" {
						tone = toneDanger
					}
					ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(t.Space(2)).
						Padding(t.Space(1.5), 0).Children(func() {
						badge(c, p, a.examSeverityName(issue.Severity), tone)
						ui.Text(c, a.examQualityText(issue)).FontSize(bodySize(t)).Grow(1).Pretty()
					})
				}
			})
		})
	})
}

func (a *app) examSeverityName(severity string) string {
	if severity == "blocking" {
		return locales.ExamQualityBlocking()
	}
	return locales.ExamQualityWarnings()
}

func (a *app) examQualityText(issue store.ExamQualityIssue) string {
	arg := func(index int) string {
		if index < len(issue.Args) {
			return issue.Args[index]
		}
		return ""
	}
	switch issue.Code {
	case "no_roster":
		return locales.ExamIssueNoRoster()
	case "missing_track":
		return fmt.Sprintf(locales.ExamIssueMissingTrack(), issue.StudentNo, issue.Name)
	case "missing_scores":
		return fmt.Sprintf(locales.ExamIssueMissingScores(), issue.StudentNo, issue.Name)
	case "score_out_of_range":
		return fmt.Sprintf(locales.ExamIssueScoreOutOfRange(), issue.StudentNo, issue.Name,
			a.subjectName(issue.Subject, issue.Subject), arg(0), arg(1))
	case "score_without_roster":
		return fmt.Sprintf(locales.ExamIssueScoreWithoutRoster(), arg(0))
	case "zero_class_target":
		return fmt.Sprintf(locales.ExamIssueZeroClassTarget(), issue.ClassName, a.tierName(issue.TierKey))
	case "empty_stat_group":
		return fmt.Sprintf(locales.ExamIssueEmptyStatGroup(), arg(0))
	case "zero_stat_group_target":
		return fmt.Sprintf(locales.ExamIssueZeroStatGroupTarget(), arg(0))
	case "missing_cutoff":
		return fmt.Sprintf(locales.ExamIssueMissingCutoff(), a.subjectName(issue.Subject, issue.Subject))
	default:
		return issue.Code
	}
}

func (a *app) examPrepPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			if ui.Button(c, "").Label(locales.Back()).Children(func() {
				ui.Icon(c, iconArrowLeft).FontSize(t.Space(4))
				ui.Text(c, locales.Back()).FontSize(bodySize(t))
			}).Clicked() {
				a.examPrepOpen = false
			}
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Text(c, locales.ExamPrepTitle()).FontSize(titleSize(t)).Bold().
					Role(ui.RoleHeading).Level(1)
				ui.Text(c, locales.ExamPrepSubtitle()).FontSize(bodySize(t)).TextColor(p.muted)
			})
		})
		if a.selectedExamID == 0 {
			emptyState(c, p, iconExams, locales.ExamPrepNoExam(), locales.ExamPrepNoExamHint(), func() {
				if labelledButton(c, locales.ExamNew(), iconPlus, true).Clicked() {
					a.openNewExam()
				}
			})
			return
		}
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				steps := []struct {
					Kind  string
					Title string
					Body  string
					Count int
				}{
					{importer.KindRoster, locales.ExamTabRoster(), locales.ExamKindRosterPurpose(), a.examPrep.Roster},
					{importer.KindScores, locales.ExamTabScores(), locales.ExamKindScoresPurpose(), a.examPrep.Scores},
					{importer.KindCutoffs, locales.ExamTabCutoffs(), locales.ExamKindCutoffsPurpose(), a.examPrep.Cutoffs},
					{importer.KindTargets, locales.ExamTabTargets(), locales.ExamKindTargetsPurpose(), a.examPrep.Targets},
				}
				for index, step := range steps {
					step := step
					panel(c, p).FillWidth().Padding(t.Space(3)).Gap(t.Space(1)).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
							if step.Count > 0 {
								badge(c, p, fmtInt(index+1), toneSuccess)
							} else {
								badge(c, p, fmtInt(index+1), toneNeutral)
							}
							ui.Column(c).Grow(1).Gap(0).Children(func() {
								ui.Text(c, step.Title).FontSize(bodySize(t)).Bold()
								ui.Text(c, step.Body).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
							})
							if step.Count > 0 {
								badge(c, p, locales.ReadinessReady(), toneSuccess)
								if ui.Button(c, locales.View()).Clicked() {
									a.examTab = examKindTab(step.Kind)
									a.examPrepOpen = false
								}
								return
							}
							if labelledButton(c, locales.PrepImport(), iconImports, false).Clicked() {
								a.openExamImportWizard(step.Kind, a.exam.ID, a.examCohort.CohortID)
							}
						})
					})
				}
			})
		})
	})
	a.newExamModal(c, p)
}

func examKindTab(kind string) int {
	switch kind {
	case importer.KindRoster:
		return 0
	case importer.KindScores:
		return 1
	case importer.KindCutoffs:
		return 2
	case importer.KindTargets:
		return 3
	default:
		return 0
	}
}

func (a *app) newExamModal(c *ui.Context, p uiPalette) {
	if !a.newExamOpen {
		return
	}
	t := p.theme
	ui.Modal(c, &a.newExamOpen, func() {
		ui.Column(c).Width(t.Space(120)).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.ExamNewTitle()).FontSize(sectionSize(t)).Bold().
					Role(ui.RoleHeading).Level(2)
				ui.Text(c, locales.ExamNewSubtitle()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
			})
			ui.Form(c, func() {
				ui.Field(c, locales.SettingsAcademicYear(), func() {
					ui.NumberInput(c, &a.newExamYear, 1990, 3000, 1).Label(locales.SettingsAcademicYear())
				})
				ui.Field(c, locales.ExamTerm(), func() {
					labels := []string{locales.ExamTermFirst(), locales.ExamTermSecond()}
					chosen := 0
					if a.newExamTerm >= 0 && a.newExamTerm < len(labels) {
						chosen = a.newExamTerm
					}
					next := labels[chosen]
					ui.Select(c, &next, labels).Label(locales.ExamTerm())
					if next != labels[chosen] {
						for index, label := range labels {
							if label == next {
								a.newExamTerm = index
								break
							}
						}
					}
				})
				ui.Field(c, locales.ExamType(), func() {
					labels := []string{
						locales.ExamTypeMidterm(), locales.ExamTypeFinal(), locales.ExamTypeGaokao(),
					}
					chosen := 0
					if a.newExamType >= 0 && a.newExamType < len(labels) {
						chosen = a.newExamType
					}
					next := labels[chosen]
					ui.Select(c, &next, labels).Label(locales.ExamType())
					if next != labels[chosen] {
						for index, label := range labels {
							if label == next {
								a.newExamType = index
								break
							}
						}
					}
				})
				ui.Field(c, locales.ExamName(), func() {
					ui.TextInput(c, &a.newExamName).Label(locales.ExamName())
				})
				ui.Field(c, locales.ExamDate(), func() {
					ui.DateInput(c, &a.newExamDate).Label(locales.ExamDate())
				}).Description(locales.ExamDateHint())
			})
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.ExamCohorts()).FontSize(captionSize(t)).TextColor(p.muted)
				for _, cohort := range a.cohorts {
					cohort := cohort
					if a.newExamCohorts == nil {
						a.newExamCohorts = map[int64]bool{}
					}
					checked := a.newExamCohorts[cohort.ID]
					ui.Checkbox(c, &checked, a.cohortName(cohort.EnrollmentYear)+" · "+
						a.classOfName(cohort.GraduationYear))
					a.newExamCohorts[cohort.ID] = checked
					if !checked {
						continue
					}
					marks := a.newExamMarks[cohort.ID]
					if marks == nil {
						marks = map[string]float64{}
						a.newExamMarks[cohort.ID] = marks
					}
					ui.Row(c).Wrap().Gap(t.Space(2)).Padding(t.Space(1), 0).Children(func() {
						for _, subject := range a.subjects {
							if !subject.Enabled {
								continue
							}
							value := marks[subject.Key]
							if value == 0 {
								value = subject.FullMark
							}
							ui.Column(c).Width(t.Space(20)).Gap(0).Children(func() {
								ui.NumberInput(c, &value, 0.1, 10000, 1).
									Label(a.subjectName(subject.Key, subject.Name))
								ui.Text(c, a.subjectName(subject.Key, subject.Name)).
									FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
							})
							marks[subject.Key] = value
						}
					})
				}
			})
			if a.examError != "" {
				notice(c, p, noticeDanger, a.examError)
			}
			ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if ui.Button(c, locales.PlanCancel()).Clicked() {
					a.newExamOpen = false
					a.examError = ""
				}
				if ui.PrimaryButton(c, locales.ExamCreate()).Clicked() {
					a.createExam()
				}
			})
		})
	})
}

func (a *app) statGroupModal(c *ui.Context, p uiPalette) {
	if !a.statGroupOpen {
		return
	}
	t := p.theme
	ui.Modal(c, &a.statGroupOpen, func() {
		ui.Column(c).Width(t.Space(124)).Gap(t.Space(3)).Children(func() {
			ui.Text(c, locales.ExamStatGroupTitle()).FontSize(sectionSize(t)).Bold().
				Role(ui.RoleHeading).Level(2)
			ui.Form(c, func() {
				ui.Field(c, locales.ExamStatGroupSubject(), func() {
					labels := make([]string, 0)
					codes := make([]string, 0)
					chosen := ""
					for _, subject := range a.subjects {
						if !subject.Enabled || subject.Key == "japanese" {
							continue
						}
						if subject.Key == a.statGroupSubject {
							chosen = a.subjectName(subject.Key, subject.Name)
						}
						labels = append(labels, a.subjectName(subject.Key, subject.Name))
						codes = append(codes, subject.Key)
					}
					next := chosen
					ui.Select(c, &next, labels).Label(locales.ExamStatGroupSubject())
					if next != chosen {
						for index, label := range labels {
							if label == next {
								a.statGroupSubject = codes[index]
								break
							}
						}
					}
				})
				ui.Field(c, locales.ExamStatGroupName(), func() {
					ui.TextInput(c, &a.statGroupName).Label(locales.ExamStatGroupName())
				})
				ui.Field(c, locales.ExamStatGroupLead(), func() {
					labels := make([]string, 0, len(a.examClasses))
					chosen := ""
					for _, class := range a.examClasses {
						if class.ID == a.statGroupLeadClass {
							chosen = class.Name
						}
						labels = append(labels, class.Name)
					}
					next := chosen
					ui.Select(c, &next, labels).Label(locales.ExamStatGroupLead())
					if next != chosen {
						for _, class := range a.examClasses {
							if class.Name == next {
								a.statGroupLeadClass = class.ID
								break
							}
						}
					}
				})
				ui.Field(c, locales.ExamStatGroupTarget(), func() {
					ui.NumberInput(c, &a.statGroupTarget, 0, 10000, 1).
						Label(locales.ExamStatGroupTarget())
				})
			})
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.ExamStatGroupMembers()).FontSize(captionSize(t)).
					TextColor(p.muted)
				ui.Scroll(c).Height(t.Space(48)).Children(func() {
					ui.Column(c).FillWidth().Gap(0).Children(func() {
						for _, group := range a.examTeachingGroups {
							group := group
							if group.Subject != a.statGroupSubject {
								continue
							}
							if a.statGroupMembers == nil {
								a.statGroupMembers = map[int64]bool{}
							}
							checked := a.statGroupMembers[group.ID]
							ui.Checkbox(c, &checked, group.Name+" · "+group.ClassNames).
								Label(group.Name)
							a.statGroupMembers[group.ID] = checked
						}
					})
				})
			})
			if a.statGroupError != "" {
				notice(c, p, noticeDanger, a.statGroupError)
			}
			ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if ui.Button(c, locales.PlanCancel()).Clicked() {
					a.statGroupOpen = false
					a.statGroupError = ""
				}
				if ui.PrimaryButton(c, locales.Save()).Clicked() {
					a.saveStatGroup()
				}
			})
		})
	})
}
