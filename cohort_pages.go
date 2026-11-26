package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo/ui"

	"tally/internal/importer"
	"tally/internal/store"
	"tally/locales"
)

func (a *app) cohortsHubPage(c *ui.Context, p uiPalette) {
	if a.cohortDetailOpen && a.selectedCohort() != nil {
		if a.studentProfileOpen {
			a.studentProfilePage(c, p)
			return
		}
		a.cohortDetailPage(c, p)
		return
	}
	a.cohortsListPage(c, p)
}

func (a *app) cohortsListPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.cohortArchiveFilter < 0 || a.cohortArchiveFilter > 1 {
		a.cohortArchiveFilter = 0
	}
	var filtered []store.Cohort
	for _, co := range a.cohorts {
		isGrad := co.CurrentPhase == store.PhaseGraduated
		if a.cohortArchiveFilter == 1 && isGrad {
			filtered = append(filtered, co)
		} else if a.cohortArchiveFilter == 0 && !isGrad {
			filtered = append(filtered, co)
		}
	}

	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			pageHeading(c, p, locales.NavCohorts(), locales.CohortListSubtitle(), func() {
				if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
					a.openNewCohort()
				}
			})

			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				segmentedField(c, &a.cohortArchiveFilter, []string{
					locales.CohortFilterActive(),
					locales.CohortFilterGraduated(),
				}, locales.NavCohorts())
				ui.Spacer(c)
				ui.Text(c, pluralCount(len(filtered), locales.ExamCohortCountOne(), locales.ExamCohortCount())).
					FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
			})

			if a.cohortArchiveFilter == 1 {
				notice(c, p, noticeInfo, locales.CohortRestoreHint())
			}

			if len(filtered) == 0 {
				title, hint := locales.CohortEmptyActive(), locales.CohortEmptyActiveHint()
				var action func()
				if a.cohortArchiveFilter == 1 {
					title, hint = locales.CohortEmptyGraduated(), locales.CohortEmptyGraduatedHint()
				} else {
					action = func() {
						if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
							a.openNewCohort()
						}
					}
				}
				panel(c, p).FillWidth().Padding(t.Space(5)).Children(func() {
					emptyState(c, p, iconCohort, title, hint, action)
				})
				return
			}

			cols := min(len(filtered), 3)
			ui.Grid(c).FillWidth().Columns(cols).Gap(t.Space(3)).Children(func() {
				for _, cohort := range filtered {
					cohort := cohort
					a.cohortArchiveCard(c, p, cohort)
				}
			})
		})
	})
}

func (a *app) cohortArchiveCard(c *ui.Context, p uiPalette, cohort store.Cohort) {
	t := p.theme
	title := a.cohortName(cohort.EnrollmentYear)
	cardTone := toneAccent
	if cohort.CurrentPhase == store.PhaseGraduated {
		cardTone = toneNeutral
	}
	var cardCard *store.CohortOverviewCard
	for i := range a.schoolOverview.ActiveCohorts {
		if a.schoolOverview.ActiveCohorts[i].Cohort.ID == cohort.ID {
			cardCard = &a.schoolOverview.ActiveCohorts[i]
			break
		}
	}

	btn := ui.ButtonBase(c.Key("cohort-card-"+strconv.FormatInt(cohort.ID, 10))).
		FillWidth().Padding(t.Space(4)).Radius(sunbeam.Radius2XL).
		Background(p.panel).Border(1, p.border).Cursor(ui.CursorPointer).
		Label(title).TextColor(p.ink)
	switch {
	case btn.Pressed():
		btn.Background(p.sb.FlatFacePressed).Border(1, p.borderStrong)
	case btn.Hovered():
		btn.Background(p.panelHover).Border(1, p.borderStrong)
	}
	btn.Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(2.5)).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, title).FontSize(sectionSize(t)).FontWeight(600).
						TextColor(p.ink).SingleLine().Tooltip(title)
					ui.Text(c, fmt.Sprintf("%s · %s", a.classOfName(cohort.GraduationYear), a.academicYearLabel(cohort.CurrentAcademicYearStart))).
						FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
				})
				badge(c, p, a.stageName(cohort.CurrentPhase), cardTone)
			})
			if cardCard != nil {
				ui.Row(c).FillWidth().Wrap().Gap(t.Space(4)).Children(func() {
					metricInline(c, p, uint64(cardCard.ClassCount), locales.ReadinessClasses())
					metricInline(c, p, uint64(cardCard.ActiveStudents), locales.ReadinessStudents())
					metricInline(c, p, uint64(cardCard.ExamCount), locales.OverviewRecentExamsTitle())
				})
			} else if cohort.GraduatedAt != nil {
				ui.Text(c, fmt.Sprintf(locales.CohortGraduatedAtLabel(), formatTime(*cohort.GraduatedAt))).
					FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
			}
		})
	})
	if btn.Clicked() {
		a.cohortDetailTab = 0
		a.openCohortDetail(cohort.ID)
	}
}

func (a *app) cohortDetailPage(c *ui.Context, p uiPalette) {
	t := p.theme
	cohort := a.selectedCohort()
	if cohort == nil {
		a.cohortDetailOpen = false
		a.cohortsListPage(c, p)
		return
	}
	cohortTitle := a.cohortName(cohort.EnrollmentYear)

	ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
		crumbChosen := 1
		if ui.Breadcrumbs(c, []string{locales.CohortDetailBackToList(), cohortTitle}, &crumbChosen).
			Label(locales.NavCohorts()).Changed() {
			if crumbChosen == 0 {
				a.cohortDetailOpen = false
				a.dataError = ""
				a.dataMessage = ""
				return
			}
		}

		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			if backButton(c, p).Clicked() {
				a.cohortDetailOpen = false
				a.dataError = ""
				a.dataMessage = ""
				return
			}
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Text(c, cohortTitle).FontSize(titleSize(t)).FontWeight(500).
						Role(ui.RoleHeading).Level(1).SingleLine()
					phaseTone := toneAccent
					if cohort.CurrentPhase == store.PhaseGraduated {
						phaseTone = toneNeutral
					}
					badge(c, p, a.stageName(cohort.CurrentPhase), phaseTone)
					if cohort.CurrentPhase == store.PhaseGraduated {
						badge(c, p, locales.StageGraduated(), toneNeutral)
					} else {
						badge(c, p, locales.CohortActive(), toneSuccess)
					}
				})
				ui.Text(c, fmt.Sprintf("%s · %s", a.classOfName(cohort.GraduationYear), a.academicYearLabel(cohort.CurrentAcademicYearStart))).
					FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
			})

			if len(a.cohortStages) > 0 {
				labels, ids := a.stageOptions()
				chosenLabel := labels[0]
				for idx, id := range ids {
					if id == a.selectedStageID {
						chosenLabel = labels[idx]
						break
					}
				}
				nextLabel := chosenLabel
				selectField(c, "cohort-phase-switch", &nextLabel, labels, locales.CohortPhaseSwitcher()).
					Width(t.Space(48))
				if nextLabel != chosenLabel {
					for idx, lbl := range labels {
						if lbl == nextLabel {
							a.selectedStageID = ids[idx]
							a.dataClassFilter = 0
							a.baselineClassFilter = 0
							a.reloadSelectedCohort(context.Background())
							break
						}
					}
				}
			}
		})

		if cohort.CurrentPhase == store.PhaseGraduated {
			notice(c, p, noticeInfo, locales.CohortArchiveNotice())
		} else if a.selectedStage() != nil && !a.stageEditable() {
			notice(c, p, noticeInfo, locales.StageReadOnly())
		}
		a.stageReviewBanner(c, p)

		if a.dataError != "" {
			notice(c, p, noticeDanger, a.dataError)
		}
		if a.dataMessage != "" {
			notice(c, p, noticeSuccess, a.dataMessage)
		}

		tabLabels := []string{
			locales.CohortTabOverview(),
			locales.CohortTabClasses(),
			locales.CohortTabStudents(),
			locales.CohortTabAssignments(),
			locales.CohortTabBaselines(),
			locales.CohortTabExams(),
		}
		if a.cohortDetailTab < 0 || a.cohortDetailTab >= len(tabLabels) {
			a.cohortDetailTab = 0
		}
		ui.Tabs(c, &a.cohortDetailTab, tabLabels...).Label(locales.CohortTabGroupLabel())
		rule(c)

		ui.Column(c).Grow(1).Shrink(1).MinHeight(0).FillWidth().Children(func() {
			switch a.cohortDetailTab {
			case 1:
				a.cohortClassesGroupsTab(c, p, cohort)
			case 2:
				a.cohortStudentsTab(c, p, cohort)
			case 3:
				a.cohortAssignmentsTab(c, p, cohort)
			case 4:
				a.cohortBaselinesTab(c, p, cohort)
			case 5:
				a.cohortExamsTab(c, p, cohort)
			default:
				a.cohortOverviewLifecycleTab(c, p, cohort)
			}
		})
	})
}

func (a *app) cohortOverviewLifecycleTab(c *ui.Context, p uiPalette, cohort *store.Cohort) {
	t := p.theme
	ui.Scroll(c.Key("cohort-tab-overview")).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			// 3年生命周期进度条（高一 → 高二 → 高三 → 高考 → 毕业归档）
			panel(c, p).FillWidth().Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					sectionHeading(c, p, locales.CohortLifecycleTitle(), -1)
					ui.Spacer(c)
					if cohort.CurrentPhase != store.PhaseGraduated {
						if cohort.CurrentPhase == store.PhaseG1 || cohort.CurrentPhase == store.PhaseG2 {
							if labelledButton(c, locales.CohortPromoteAction(), iconArrowRight, true).Clicked() {
								a.openPromoteCohortModal(cohort)
							}
						}
						if plainButton(c, locales.CohortGraduateAction()).Clicked() {
							a.openGraduateCohortModal(cohort)
						}
					} else if cohort.GraduatedAt != nil {
						badge(c, p, fmt.Sprintf(locales.CohortGraduatedAtLabel(), formatTime(*cohort.GraduatedAt)), toneNeutral)
					}
				})

				progressVal := 0.25
				stepIdx := 0
				switch cohort.CurrentPhase {
				case store.PhaseG1:
					progressVal = 0.25
					stepIdx = 0
				case store.PhaseG2:
					progressVal = 0.50
					stepIdx = 1
				case store.PhaseG3:
					progressVal = 0.75
					stepIdx = 2
				case store.PhaseGraduated:
					progressVal = 1.0
					stepIdx = 4
				}
				sunbeam.Progress(c, progressVal).Label(locales.CohortLifecycleTitle())

				steps := []string{
					locales.StageG1(),
					locales.StageG2(),
					locales.StageG3(),
					locales.CohortStepGaokao(),
					locales.StageGraduated(),
				}
				ui.Row(c).FillWidth().Wrap().Gap(t.Space(2)).Children(func() {
					for i, sLabel := range steps {
						tone := toneNeutral
						if i < stepIdx {
							tone = toneSuccess
						} else if i == stepIdx {
							tone = toneAccent
						}
						badge(c, p, sLabel, tone)
					}
				})
			})

			ui.Grid(c).FillWidth().Columns(5).Gap(t.Space(2.5)).Children(func() {
				kpiCard(c, p, kpiCardProps{
					Key:   "co-kpi-classes",
					Title: locales.ReadinessClasses(),
					Value: formatInt(a.stageCounts.Classes),
				})
				kpiCard(c, p, kpiCardProps{
					Key:   "co-kpi-students",
					Title: locales.ReadinessStudents(),
					Value: formatInt(a.stageCounts.Students),
				})
				kpiCard(c, p, kpiCardProps{
					Key:   "co-kpi-teachers",
					Title: locales.ReadinessTeachers(),
					Value: formatInt(a.stageCounts.Teachers),
				})
				kpiCard(c, p, kpiCardProps{
					Key:   "co-kpi-baselines",
					Title: locales.ReadinessBaseline(),
					Value: formatInt(a.stageCounts.Baselines),
				})
				kpiCard(c, p, kpiCardProps{
					Key:   "co-kpi-exams",
					Title: locales.CohortTabExams(),
					Value: formatInt(len(a.cohortExams)),
				})
			})
		})
	})
}

func (a *app) cohortClassesGroupsTab(c *ui.Context, p uiPalette, cohort *store.Cohort) {
	t := p.theme
	editable := a.stageEditable()
	ui.Scroll(c.Key("cohort-tab-classes")).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(5)).Children(func() {
			// 行政班管理
			section(c, p, func() {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					sectionHeading(c, p, locales.CohortClasses(), len(a.classes))
					ui.Spacer(c)
					impBtn := plainButton(c, locales.ImportClasses())
					impBtn.Disabled(!editable)
					if impBtn.Clicked() {
						a.openDataImport(importer.KindClasses, 0, cohort.ID)
					}
					newBtn := labelledButton(c, locales.CohortNewClass(), iconPlus, true)
					newBtn.Disabled(!editable)
					if newBtn.Clicked() {
						a.newClassOpen = true
						a.newClassName = ""
						a.newClassTrack = ""
						a.newClassRoom = ""
						a.newClassError = ""
					}
				})
				if len(a.classes) == 0 {
					smallEmptyState(c, p, locales.CohortNoClasses())
				} else {
					prevPhase := prevStagePhase(a.selectedStage())
					for _, cls := range a.classes {
						cls := cls
						contLabel := "-"
						if cls.ContinuedFromClassID != nil {
							if name, ok := a.prevPhaseClasses[*cls.ContinuedFromClassID]; ok {
								if prevPhase != "" {
									contLabel = a.stageName(prevPhase) + " " + name
								} else {
									contLabel = name
								}
							} else {
								contLabel = "#" + strconv.FormatInt(*cls.ContinuedFromClassID, 10)
							}
						}
						panel(c, p).FillWidth().Padding(t.Space(3), t.Space(4)).Children(func() {
							ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
								ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
									ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
										ui.Text(c, cls.Name).FontSize(bodySize(t)).FontWeight(600).SingleLine()
										if cls.TrackHint != "" {
											badge(c, p, a.trackName(cls.TrackHint, cls.TrackHint), toneNeutral)
										}
										if cls.ContinuedFromClassID != nil {
											badge(c, p, locales.CohortClassContinuedFrom()+": "+contLabel, toneAccent)
										}
									})
									htLabel := dashOr(cls.HeadTeacher)
									ui.Text(c, fmt.Sprintf("%s: %s · %s: %s",
										locales.ClassHeadTeacher(), htLabel,
										locales.ReadinessStudents(), formatInt(cls.Students))).
										FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
								})
								ui.Row(c).Wrap().Shrink(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
									if plainButton(c.Key("cls-trend-"+strconv.FormatInt(cls.ID, 10)), locales.CohortClassTrendBtn()).Clicked() {
										a.openClassTrendModal(cls.ID)
									}
									htBtn := plainButton(c.Key("cls-ht-"+strconv.FormatInt(cls.ID, 10)), locales.CohortClassSetHeadBtn())
									htBtn.Disabled(!editable)
									if htBtn.Clicked() {
										a.openHeadTeacherModal(cls)
									}
									delBtn := dangerButton(c.Key("cls-del-"+strconv.FormatInt(cls.ID, 10)), locales.CohortClassDeleteBtn())
									delBtn.Disabled(!editable)
									if delBtn.Clicked() {
										a.deleteCohortClass(cls)
									}
								})
							})
						})
					}
				}
			})

			rule(c)

			// 走班教学班管理
			var walkingGroups []store.DataTeachingGroup
			for _, g := range a.teachingGroups {
				if g.Kind == "walking" {
					walkingGroups = append(walkingGroups, g)
				}
			}
			section(c, p, func() {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					sectionHeading(c, p, locales.CohortWalkingGroupsTitle(), len(walkingGroups))
					ui.Spacer(c)
					newWalk := labelledButton(c, locales.CohortNewWalkingGroup(), iconPlus, false)
					newWalk.Disabled(!editable)
					if newWalk.Clicked() {
						a.newWalkingGroupOpen = true
						a.newWalkingName = ""
						a.newWalkingSubject = ""
						a.newWalkingError = ""
					}
				})
				if len(walkingGroups) == 0 {
					smallEmptyState(c, p, locales.GroupsEmpty())
				} else {
					for _, g := range walkingGroups {
						g := g
						panel(c, p).FillWidth().Padding(t.Space(3), t.Space(4)).Children(func() {
							ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
								ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
									ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
										ui.Text(c, g.Name).FontSize(bodySize(t)).FontWeight(600).SingleLine()
										badge(c, p, a.subjectName(g.Subject, g.Subject), toneAccent)
									})
									ui.Text(c, fmt.Sprintf("%s: %s · %s: %s",
										locales.GroupTeachers(), dashOr(g.Teachers),
										locales.GroupStudents(), formatInt(g.Students))).
										FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
								})
								if plainButton(c.Key("walk-stu-"+strconv.FormatInt(g.ID, 10)), locales.CohortWalkingManageStudents()).Clicked() {
									a.openWalkingStudentsModal(g)
								}
							})
						})
					}
				}
			})
		})
	})
}

func prevStagePhase(stage *store.CohortStage) string {
	if stage == nil {
		return ""
	}
	switch stage.GradeLevel {
	case store.PhaseG2:
		return store.PhaseG1
	case store.PhaseG3:
		return store.PhaseG2
	default:
		return ""
	}
}

func (a *app) cohortStudentsTab(c *ui.Context, p uiPalette, cohort *store.Cohort) {
	t := p.theme
	editable := a.stageEditable()
	ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.CohortTabStudents(), a.studentsTotal)
			ui.Spacer(c)
			impBtn := plainButton(c, locales.ImportStudents())
			impBtn.Disabled(a.dataBusy || !editable)
			if impBtn.Clicked() {
				a.openDataImport(importer.KindStudents, 0, cohort.ID)
			}
			addBtn := labelledButton(c, locales.CohortNewStudent(), iconPlus, true)
			addBtn.Disabled(!editable)
			if addBtn.Clicked() {
				a.newStudentOpen = true
				a.newStudentNo = ""
				a.newStudentName = ""
				a.newStudentClassChoice = ""
				a.newStudentTrackChoice = ""
				a.newStudentEntryTotal = 0
				a.newStudentError = ""
			}
		})
		a.studentFilters(c, p)
		if len(a.students) == 0 {
			hasFilter := strings.TrimSpace(a.dataQuery) != "" || a.dataClassFilter != 0 ||
				a.dataTrackFilter != "" || a.dataStatusFilter != ""
			if hasFilter {
				emptyState(c, p, iconStudents, locales.FilterEmptyTitle(), locales.FilterEmptyHint(), func() {
					if plainButton(c, locales.ClearFilters()).Clicked() {
						a.dataQuery = ""
						a.dataClassFilter = 0
						a.dataTrackFilter = ""
						a.dataStatusFilter = ""
						a.studentPage = 0
						a.reloadStudents(context.Background())
					}
				})
				return
			}
			emptyState(c, p, iconStudents, locales.StudentsEmpty(), locales.StudentsEmptyHint(), func() {
				btn := labelledButton(c, locales.ImportStudents(), iconImports, true)
				btn.Disabled(a.dataBusy || !editable)
				if btn.Clicked() {
					a.openDataImport(importer.KindStudents, 0, cohort.ID)
				}
			})
			return
		}
		a.studentTable.RowHeight = tableRowHeight(t)
		selected := -1
		for index, student := range a.students {
			if student.ID == a.selectedStudentID {
				selected = index
				break
			}
		}
		a.studentTable.Selected = &selected
		a.studentTable.Key = func(row int) any { return a.students[row].ID }
		table := ui.Table(c, &a.studentTable, studentColumns(t), len(a.students), func(row, col int) {
			student := a.students[row]
			switch col {
			case 0:
				cellText(c, p, student.StudentNo).FontFeatures("tnum")
			case 1:
				cellText(c, p, student.Name)
			case 2:
				cellText(c, p, student.ClassName)
			case 3:
				cellText(c, p, a.trackName(student.Track, student.Track))
			case 4:
				cellText(c, p, a.statusName(student.Status))
			case 5:
				if student.TotalScore == nil {
					ui.Text(c, "-").TextColor(p.muted)
				} else {
					ui.Text(c, formatDecimal(*student.TotalScore, 1)).FontFeatures("tnum")
				}
			}
		}).Label(locales.NavStudents()).Grow(1).Shrink(1).MinHeight(0).Fill()
		if table.Changed() && selected >= 0 {
			a.openStudentProfile(a.students[selected].ID)
		}
		paginationBar(c, p, "cohort-students", &a.studentPage, &a.studentPageSize, a.studentsTotal, func() {
			a.selectedStudentID = 0
			a.reloadStudents(context.Background())
		})
	})
}

func (a *app) cohortAssignmentsTab(c *ui.Context, p uiPalette, cohort *store.Cohort) {
	t := p.theme
	editable := a.stageEditable()
	ui.Scroll(c.Key("cohort-tab-assignments")).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(5)).Children(func() {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				sectionHeading(c, p, locales.CohortAssignAdminTitle(), len(a.cohortAssignments))
				ui.Spacer(c)
				impBtn := plainButton(c, locales.ImportAssignments())
				impBtn.Disabled(!editable)
				if impBtn.Clicked() {
					a.openDataImport(importer.KindAssignments, 0, cohort.ID)
				}
				assignBtn := labelledButton(c, locales.CohortAssignBtn(), iconPlus, true)
				assignBtn.Disabled(!editable || len(a.teachingGroups) == 0)
				if assignBtn.Clicked() {
					a.openAssignTeacherModal(0)
				}
			})

			if len(a.teachingGroups) == 0 {
				smallEmptyState(c, p, locales.GroupsEmpty())
				return
			}

			// 行政班 × 学科矩阵（展示常规教学班及其当前任课教师）
			occStatusByID := make(map[int64]string, len(a.teacherOccupancy))
			for _, occ := range a.teacherOccupancy {
				occStatusByID[occ.TeacherID] = occ.Status
			}
			assignByGroup := map[int64][]store.TeachingAssignmentRow{}
			for _, row := range a.cohortAssignments {
				assignByGroup[row.TeachingGroupID] = append(assignByGroup[row.TeachingGroupID], row)
			}

			var adminGroups []store.DataTeachingGroup
			var walkGroups []store.DataTeachingGroup
			for _, g := range a.teachingGroups {
				if g.Kind == "walking" {
					walkGroups = append(walkGroups, g)
				} else {
					adminGroups = append(adminGroups, g)
				}
			}

			for _, g := range adminGroups {
				g := g
				rows := assignByGroup[g.ID]
				panel(c, p).FillWidth().Padding(t.Space(3), t.Space(4)).Children(func() {
					ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, dashOr(g.ClassNames)).FontSize(bodySize(t)).FontWeight(600).SingleLine()
								badge(c, p, a.subjectName(g.Subject, g.Subject), toneAccent)
							})
							if len(rows) > 0 {
								ui.Row(c).Wrap().AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
									for _, ar := range rows {
										ar := ar
										if a.assignmentTeacherLink(c, p, "assign-t-"+strconv.FormatInt(ar.ID, 10), ar.TeacherName, occStatusByID[ar.TeacherID]).Clicked() {
											a.openTeacherProfileFromCohort(ar.TeacherID, cohort.ID)
										}
									}
								})
							} else {
								ui.Text(c, dashOr(g.Teachers)).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
							}
						})
						ui.Row(c).Wrap().Shrink(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
							for _, ar := range rows {
								ar := ar
								relBtn := plainButton(c.Key("unassign-"+strconv.FormatInt(ar.ID, 10)),
									ar.TeacherName+" · "+locales.CohortUnassignBtn())
								relBtn.Disabled(!editable)
								if relBtn.Clicked() {
									a.endCohortAssignment(ar.ID)
								}
							}
							addBtn := plainButton(c.Key("assign-grp-"+strconv.FormatInt(g.ID, 10)), locales.CohortAssignBtn())
							addBtn.Disabled(!editable)
							if addBtn.Clicked() {
								a.openAssignTeacherModal(g.ID)
							}
						})
					})
				})
			}

			if len(walkGroups) > 0 {
				rule(c)
				sectionHeading(c, p, locales.CohortAssignWalkingTitle(), len(walkGroups))
				for _, g := range walkGroups {
					g := g
					rows := assignByGroup[g.ID]
					panel(c, p).FillWidth().Padding(t.Space(3), t.Space(4)).Children(func() {
						ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
							ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
								ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
									ui.Text(c, g.Name).FontSize(bodySize(t)).FontWeight(600).SingleLine()
									badge(c, p, a.subjectName(g.Subject, g.Subject), toneAccent)
								})
								if len(rows) > 0 {
									ui.Row(c).Wrap().AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
										for _, ar := range rows {
											ar := ar
											if a.assignmentTeacherLink(c, p, "assign-wt-"+strconv.FormatInt(ar.ID, 10), ar.TeacherName, occStatusByID[ar.TeacherID]).Clicked() {
												a.openTeacherProfileFromCohort(ar.TeacherID, cohort.ID)
											}
										}
									})
								} else {
									ui.Text(c, dashOr(g.Teachers)).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
								}
							})
							ui.Row(c).Wrap().Shrink(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
								for _, ar := range rows {
									ar := ar
									relBtn := plainButton(c.Key("unassign-w-"+strconv.FormatInt(ar.ID, 10)),
										ar.TeacherName+" · "+locales.CohortUnassignBtn())
									relBtn.Disabled(!editable)
									if relBtn.Clicked() {
										a.endCohortAssignment(ar.ID)
									}
								}
								addBtn := plainButton(c.Key("assign-wgrp-"+strconv.FormatInt(g.ID, 10)), locales.CohortAssignBtn())
								addBtn.Disabled(!editable)
								if addBtn.Clicked() {
									a.openAssignTeacherModal(g.ID)
								}
							})
						})
					})
				}
			}
		})
	})
}

func (a *app) cohortBaselinesTab(c *ui.Context, p uiPalette, cohort *store.Cohort) {
	t := p.theme
	editable := a.stageEditable()
	ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.CohortTabBaselines(), len(a.baselines))
			ui.Spacer(c)
			editBtn := plainButton(c, locales.CohortBaselineEditBtn())
			editBtn.Disabled(!editable)
			if editBtn.Clicked() {
				a.openBaselineEditModal()
			}
			impBtn := labelledButton(c, locales.ImportBaselines(), iconImports, false)
			impBtn.Disabled(a.dataBusy || !editable)
			if impBtn.Clicked() {
				a.openDataImport(importer.KindBaselines, 0, cohort.ID)
			}
		})
		a.baselineFilters(c, p)
		if len(a.baselines) == 0 {
			if strings.TrimSpace(a.baselineQuery) != "" || a.baselineClassFilter != 0 {
				emptyState(c, p, iconBaseline, locales.FilterEmptyTitle(), locales.FilterEmptyHint(), func() {
					if plainButton(c, locales.ClearFilters()).Clicked() {
						a.baselineQuery = ""
						a.baselineClassFilter = 0
						a.reloadDataPages(context.Background())
					}
				})
				return
			}
			emptyState(c, p, iconBaseline, locales.BaselineEmpty(), locales.BaselineEmptyHint(), func() {
				btn := labelledButton(c, locales.ImportBaselines(), iconImports, true)
				btn.Disabled(a.dataBusy || !editable)
				if btn.Clicked() {
					a.openDataImport(importer.KindBaselines, 0, cohort.ID)
				}
			})
			return
		}
		a.baselineTable.RowHeight = tableRowHeight(t)
		a.baselineTable.Key = func(row int) any {
			item := a.baselines[row]
			return fmt.Sprintf("%s:%s:%s", item.ClassName, item.Subject, item.SetName)
		}
		columns := []ui.TableColumn{
			{Title: locales.ColumnClass(), Width: t.Space(18)},
			{Title: locales.BaselineSubject(), Width: t.Space(20)},
			{Title: locales.BaselineAverage(), Width: 0, Align: ui.End},
			{Title: locales.BaselineCount(), Width: t.Space(18), Align: ui.End},
			{Title: locales.BaselineRatio(), Width: t.Space(18), Align: ui.End},
			{Title: locales.BaselineSet(), Width: 0},
			{Title: locales.ColumnSource(), Width: t.Space(28)},
		}
		ui.Table(c, &a.baselineTable, columns, len(a.baselines), func(row, col int) {
			item := a.baselines[row]
			switch col {
			case 0:
				cellText(c, p, item.ClassName)
			case 1:
				cellText(c, p, a.subjectName(item.Subject, item.Subject))
			case 2:
				ui.Text(c, formatDecimal(item.Average, 2)).FontFeatures("tnum")
			case 3:
				ui.Text(c, formatInt(item.Count)).FontFeatures("tnum")
			case 4:
				ui.Text(c, formatRatio(item.Ratio)).FontFeatures("tnum")
			case 5:
				mutedCellText(c, p, item.SetName)
			case 6:
				mutedCellText(c, p, dataSourceLabel(item.ImportRunID, item.SourceSheet, item.SourceRow))
			}
		}).Label(locales.NavBaseline()).Grow(1).Shrink(1).MinHeight(0).Fill()
	})
}

func (a *app) cohortExamsTab(c *ui.Context, p uiPalette, cohort *store.Cohort) {
	t := p.theme
	assessByExam := map[int64]store.AssessmentOverview{}
	for _, ov := range a.cohortAssessments {
		assessByExam[ov.ExamID] = ov
	}

	ui.Scroll(c.Key("cohort-tab-exams")).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				sectionHeading(c, p, locales.CohortTabExams(), len(a.cohortExams))
				ui.Spacer(c)
				if cohort.CurrentPhase != store.PhaseGraduated {
					if labelledButton(c, locales.ExamNew(), iconPlus, false).Clicked() {
						a.navigateTo(pageExams)
						a.openNewExam()
					}
				}
			})
			if len(a.cohortExams) == 0 {
				smallEmptyState(c, p, locales.CohortExamsEmpty())
				return
			}
			for _, exam := range a.cohortExams {
				exam := exam
				phaseAtExam := ""
				rosterCount := 0
				for _, ec := range exam.Cohorts {
					if ec.CohortID == cohort.ID {
						phaseAtExam = ec.GradeLevelAtExam
						rosterCount = ec.RosterCount
						break
					}
				}
				ov, hasOv := assessByExam[exam.ID]
				title := a.examBatchLabel(exam)
				row := linkRow(c, p, title, func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Text(c, title).FontSize(bodySize(t)).FontWeight(600).SingleLine().Tooltip(title)
							if phaseAtExam != "" {
								badge(c, p, a.stageName(phaseAtExam), toneAccent)
							}
						})
						ui.Text(c, fmt.Sprintf("%s · %s · %s",
							a.academicYearLabel(exam.AcademicYearStart), a.examTermName(exam.Term), dashOr(exam.ExamDate))).
							FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
					})
					if rosterCount > 0 {
						metricInline(c, p, uint64(rosterCount), locales.OverviewMetricStudents())
					}
					statusBadge(c, p, exam.Status, a.examStatusName(exam.Status))
					if hasOv {
						if ov.HasOfficial {
							badge(c, p, locales.OverviewLatestOfficial(), toneSuccess)
						} else {
							badge(c, p, locales.OverviewLatestPending(), toneNeutral)
						}
					}
				})
				if row.Clicked() {
					a.openExamForCohort(exam.ID, cohort.ID)
				}
			}
		})
	})
}

// -----------------------------------------------------------------------------
// Lifecycle & Entity Modals (Promote, Graduate, Class, Head Teacher, Walking, Assign, Student, Baseline, Teacher)
// -----------------------------------------------------------------------------

func (a *app) openPromoteCohortModal(cohort *store.Cohort) {
	if cohort == nil {
		return
	}
	nextPhase := store.PhaseG2
	if cohort.CurrentPhase == store.PhaseG2 {
		nextPhase = store.PhaseG3
	}
	a.promoteOpen = true
	a.promoteToPhase = nextPhase
	a.promoteYear = float64(cohort.CurrentAcademicYearStart + 1)
	a.promoteMode = store.PromoteDirect
	a.promoteError = ""
}

func (a *app) promoteCohortModal(c *ui.Context, p uiPalette) {
	cohort := a.selectedCohort()
	if a.promoteOpen && cohort == nil {
		a.promoteOpen = false
	}
	t := p.theme
	title := locales.CohortPromoteAction()
	if cohort != nil {
		title = fmt.Sprintf(locales.CohortPromoteTitle(), a.cohortName(cohort.EnrollmentYear))
	}
	wasOpen := a.promoteOpen
	confirmed := modal(c, &a.promoteOpen, modalProps{
		Key:        "promote-cohort",
		Title:      title,
		OkText:     locales.CohortPromoteConfirm(),
		CancelText: locales.PlanCancel(),
	}, func() {
		if cohort == nil {
			return
		}
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			ui.Text(c, locales.CohortPromoteSubtitle()).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
			a.detailRow(c, p, locales.CohortPromoteTargetPhase(), a.stageName(a.promoteToPhase))
			a.dialogField(c, "promote-year", locales.CohortPromoteTargetYear(),
				a.academicYearLabel(int(a.promoteYear)), func() {
					numberField(c, &a.promoteYear, 1990, 2100, 1, locales.CohortPromoteTargetYear())
				})
			a.dialogField(c, "promote-mode", locales.CohortPromoteModeLabel(), "", func() {
				modeLabels := []string{
					locales.CohortPromoteModeDirect(),
					locales.CohortPromoteModeReclassify(),
				}
				chosen := modeLabels[0]
				if a.promoteMode == store.PromoteReclassify {
					chosen = modeLabels[1]
				}
				next := chosen
				sunbeam.RadioGroup(c, &next, modeLabels...).Label(locales.CohortPromoteModeLabel())
				if next == modeLabels[1] {
					a.promoteMode = store.PromoteReclassify
				} else {
					a.promoteMode = store.PromoteDirect
				}
			})
			if a.promoteMode == store.PromoteReclassify {
				notice(c, p, noticeInfo, locales.CohortPromoteModeReclassifyDesc())
			} else {
				notice(c, p, noticeInfo, locales.CohortPromoteModeDirectDesc())
			}
			if a.promoteError != "" {
				notice(c, p, noticeDanger, a.promoteError)
			}
		})
	})
	if confirmed && cohort != nil {
		res, err := a.store.PromoteCohort(context.Background(), cohort.ID, store.PromoteInput{
			ToPhase:             a.promoteToPhase,
			ToAcademicYearStart: int(a.promoteYear),
			Mode:                a.promoteMode,
		})
		if err != nil {
			a.promoteError = err.Error()
			a.promoteOpen = true
			return
		}
		a.promoteOpen = false
		a.promoteError = ""
		a.selectedStageID = 0
		a.reloadCohorts(context.Background())
		a.notifyMain(fmt.Sprintf(locales.CohortPromoteDoneToast(),
			a.cohortName(cohort.EnrollmentYear), a.stageName(res.ToPhase)))
	} else if wasOpen && !a.promoteOpen {
		a.promoteError = ""
	}
}

func (a *app) openGraduateCohortModal(cohort *store.Cohort) {
	if cohort == nil || a.store == nil {
		return
	}
	preview, err := a.store.PreviewGraduateCohort(context.Background(), cohort.ID)
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.graduatePreview = preview
	a.graduateError = ""
	a.graduateOpen = true
}

func (a *app) graduateCohortModal(c *ui.Context, p uiPalette) {
	cohort := a.selectedCohort()
	if a.graduateOpen && cohort == nil {
		a.graduateOpen = false
	}
	t := p.theme
	title := locales.CohortGraduateAction()
	if cohort != nil {
		title = fmt.Sprintf(locales.CohortGraduateTitle(), a.cohortName(cohort.EnrollmentYear))
	}
	wasOpen := a.graduateOpen
	confirmed := modal(c, &a.graduateOpen, modalProps{
		Key:        "graduate-cohort",
		Title:      title,
		Width:      modalWidthWide,
		OkText:     locales.CohortGraduateConfirm(),
		CancelText: locales.PlanCancel(),
	}, func() {
		pv := a.graduatePreview
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			notice(c, p, noticeWarning, locales.CohortGraduateWarning())
			a.summaryMetricRow(c, p, locales.CohortGraduateImpactStudents(), formatInt(pv.ActiveStudents))
			a.summaryMetricRow(c, p, locales.CohortGraduateImpactExams(), formatInt(pv.ExamCount))
			rule(c)

			releasedNames := make([]string, 0, len(pv.ReleasedTeachers))
			for _, tc := range pv.ReleasedTeachers {
				releasedNames = append(releasedNames, tc.Name)
			}
			releasedText := locales.CohortGraduateNoneAffected()
			if len(releasedNames) > 0 {
				releasedText = strings.Join(releasedNames, locales.ListSeparator())
			}
			ui.Text(c, fmt.Sprintf(locales.CohortGraduateReleasedIdle(), formatInt(pv.ReleasedToIdleCount))).
				FontSize(bodySize(t)).FontWeight(600)
			ui.Text(c, releasedText).FontSize(captionSize(t)).TextColor(p.inkSecondary).Pretty()

			crossNames := make([]string, 0, len(pv.CrossCohortTeachers))
			for _, tc := range pv.CrossCohortTeachers {
				crossNames = append(crossNames, tc.Name)
			}
			crossText := locales.CohortGraduateNoneAffected()
			if len(crossNames) > 0 {
				crossText = strings.Join(crossNames, locales.ListSeparator())
			}
			ui.Text(c, fmt.Sprintf(locales.CohortGraduateStillCross(), formatInt(pv.StillActiveCrossCohortCount))).
				FontSize(bodySize(t)).FontWeight(600)
			ui.Text(c, crossText).FontSize(captionSize(t)).TextColor(p.inkSecondary).Pretty()

			if a.graduateError != "" {
				notice(c, p, noticeDanger, a.graduateError)
			}
		})
	})
	if confirmed && cohort != nil {
		res, err := a.store.GraduateCohort(context.Background(), cohort.ID)
		if err != nil {
			a.graduateError = err.Error()
			a.graduateOpen = true
			return
		}
		a.graduateOpen = false
		a.graduateError = ""
		a.selectedStageID = 0
		a.reloadCohorts(context.Background())
		a.notifyMain(fmt.Sprintf(locales.CohortGraduateDoneToast(),
			a.cohortName(cohort.EnrollmentYear), formatInt(res.ReleasedToIdle)))
	} else if wasOpen && !a.graduateOpen {
		a.graduateError = ""
	}
}

func (a *app) newClassModal(c *ui.Context, p uiPalette) {
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	t := p.theme
	wasOpen := a.newClassOpen
	confirmed := modal(c, &a.newClassOpen, modalProps{
		Key:        "new-class",
		Title:      locales.CohortNewClass(),
		OkText:     locales.Save(),
		CancelText: locales.PlanCancel(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			a.dialogForm(c, func() {
				a.dialogField(c, "class-name", locales.CohortClassName(), "", func() {
					textField(c, &a.newClassName, locales.CohortClassName(), locales.CohortClassName())
				})
				a.dialogField(c, "class-track", locales.CohortClassTrackHint(), "", func() {
					labels := []string{locales.DataAllTracks(), locales.TrackPhysics(), locales.TrackHistory()}
					codes := []string{"", "physics", "history"}
					chosen := labels[0]
					for i, code := range codes {
						if code == a.newClassTrack {
							chosen = labels[i]
						}
					}
					next := chosen
					selectField(c, "new-class-track", &next, labels, locales.CohortClassTrackHint())
					for i, lbl := range labels {
						if lbl == next {
							a.newClassTrack = codes[i]
						}
					}
				})
				a.dialogField(c, "class-room", locales.CohortClassRoom(), "", func() {
					textField(c, &a.newClassRoom, locales.CohortClassRoom(), locales.CohortClassRoom())
				})
			})
			if a.newClassError != "" {
				notice(c, p, noticeDanger, a.newClassError)
			}
		})
	})
	if confirmed && cohort != nil && stage != nil {
		created, err := a.store.CreateClass(context.Background(), cohort.ID, stage.GradeLevel,
			a.newClassName, a.newClassTrack, a.newClassRoom, nil)
		if err != nil {
			a.newClassError = err.Error()
			a.newClassOpen = true
			return
		}
		a.newClassOpen = false
		a.newClassError = ""
		a.reloadCohorts(context.Background())
		a.notifyMain(fmt.Sprintf(locales.CohortClassCreatedToast(), created.Name))
	} else if wasOpen && !a.newClassOpen {
		a.newClassError = ""
	}
}

func (a *app) deleteCohortClass(cls store.SchoolClass) {
	if a.store == nil {
		return
	}
	if err := a.store.DeleteClass(context.Background(), cls.ID); err != nil {
		if errors.Is(err, store.ErrClassNotEmpty) {
			a.dataError = err.Error()
			return
		}
		a.dataError = err.Error()
		return
	}
	a.dataError = ""
	a.reloadCohorts(context.Background())
	a.notifyMain(fmt.Sprintf(locales.CohortClassDeletedToast(), cls.Name))
}

func (a *app) formatTeacherPickerLabel(occ store.TeacherOccupancy) string {
	var tag string
	switch occ.Status {
	case store.TeacherStatusInactive:
		tag = locales.TeacherTagInactive()
	case store.TeacherStatusIdle:
		tag = locales.TeacherTagIdle()
	case store.TeacherStatusActiveMulti:
		tag = locales.TeacherTagCrossGrade()
	default:
		if len(occ.ActiveAssignments) > 0 {
			tag = fmt.Sprintf(locales.TeacherTagActiveCohort(), a.cohortName(occ.ActiveAssignments[0].EnrollmentYear))
		} else {
			tag = locales.TeacherTagIdle()
		}
	}
	return fmt.Sprintf("%s %s", occ.Name, tag)
}

func (a *app) teacherByPickerLabel(label string) *store.TeacherOccupancy {
	for i := range a.teacherOccupancy {
		if a.formatTeacherPickerLabel(a.teacherOccupancy[i]) == label {
			return &a.teacherOccupancy[i]
		}
	}
	return nil
}

func (a *app) openHeadTeacherModal(cls store.SchoolClass) {
	a.reloadTeacherDirectory(context.Background())
	a.headTeacherClassID = cls.ID
	a.headTeacherClassName = cls.Name
	a.headTeacherID = cls.HeadTeacherID
	a.headTeacherChoice = ""
	a.headTeacherWarning = nil
	a.headTeacherError = ""
	for _, occ := range a.teacherOccupancy {
		if occ.TeacherID == cls.HeadTeacherID {
			a.headTeacherChoice = a.formatTeacherPickerLabel(occ)
			break
		}
	}
	if a.headTeacherChoice == "" && len(a.teacherOccupancy) > 0 {
		a.headTeacherChoice = a.formatTeacherPickerLabel(a.teacherOccupancy[0])
		a.headTeacherID = a.teacherOccupancy[0].TeacherID
	}
	a.refreshHeadTeacherWarning()
	a.headTeacherOpen = true
}

func (a *app) refreshHeadTeacherWarning() {
	a.headTeacherWarning = nil
	cohort := a.selectedCohort()
	if cohort == nil || a.store == nil || a.headTeacherID <= 0 {
		return
	}
	warn, err := a.store.DetectCrossGradeWarnings(context.Background(), a.headTeacherID, cohort.ID)
	if err == nil {
		a.headTeacherWarning = warn
	}
}

func (a *app) formatCrossGradeWarningText(w *store.CrossGradeWarning) string {
	if w == nil {
		return ""
	}
	seen := map[string]bool{}
	var targets []string
	for _, item := range w.ExistingTargets {
		lbl := fmt.Sprintf("%s %s", a.cohortName(item.EnrollmentYear), item.ClassName)
		if !seen[lbl] {
			seen[lbl] = true
			targets = append(targets, lbl)
		}
	}
	return fmt.Sprintf(locales.CohortCrossGradeWarning(), w.TeacherName, strings.Join(targets, locales.ListSeparator()))
}

func (a *app) headTeacherModal(c *ui.Context, p uiPalette) {
	t := p.theme
	wasOpen := a.headTeacherOpen
	confirmed := modal(c, &a.headTeacherOpen, modalProps{
		Key:        "head-teacher",
		Title:      fmt.Sprintf(locales.CohortHeadTeacherModalTitle(), a.headTeacherClassName),
		OkText:     locales.Save(),
		CancelText: locales.PlanCancel(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			labels := make([]string, 0, len(a.teacherOccupancy))
			for _, occ := range a.teacherOccupancy {
				if occ.Status == store.TeacherStatusInactive {
					continue
				}
				labels = append(labels, a.formatTeacherPickerLabel(occ))
			}
			prev := a.headTeacherChoice
			a.dialogField(c, "ht-teacher", locales.CohortTeacherSelectLabel(), "", func() {
				selectField(c, "ht-teacher-sel", &a.headTeacherChoice, labels, locales.CohortTeacherSelectLabel())
			})
			if a.headTeacherChoice != prev {
				if occ := a.teacherByPickerLabel(a.headTeacherChoice); occ != nil {
					a.headTeacherID = occ.TeacherID
					a.refreshHeadTeacherWarning()
				}
			}
			if a.headTeacherWarning != nil {
				notice(c, p, noticeWarning, a.formatCrossGradeWarningText(a.headTeacherWarning))
			}
			if a.headTeacherError != "" {
				notice(c, p, noticeDanger, a.headTeacherError)
			}
		})
	})
	if confirmed {
		if occ := a.teacherByPickerLabel(a.headTeacherChoice); occ != nil {
			a.headTeacherID = occ.TeacherID
		}
		if a.headTeacherID <= 0 {
			a.headTeacherError = locales.SelectNoOptions()
			a.headTeacherOpen = true
			return
		}
		warn, err := a.store.SetClassHeadTeacher(context.Background(), a.headTeacherClassID, a.headTeacherID)
		if err != nil {
			a.headTeacherError = err.Error()
			a.headTeacherOpen = true
			return
		}
		a.headTeacherOpen = false
		a.headTeacherError = ""
		a.reloadCohorts(context.Background())
		msg := fmt.Sprintf(locales.CohortHeadTeacherSavedToast(), a.headTeacherClassName)
		if warn != nil {
			msg = a.formatCrossGradeWarningText(warn)
		}
		a.notifyMain(msg)
	} else if wasOpen && !a.headTeacherOpen {
		a.headTeacherError = ""
	}
}

func (a *app) openClassTrendModal(classID int64) {
	if a.store == nil {
		return
	}
	nodes, trend, err := a.store.QueryClassTrendChain(context.Background(), classID)
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.classTrendNodes = nodes
	a.classTrendResult = trend
	a.classTrendError = ""
	a.classTrendOpen = true
}

func (a *app) classTrendModal(c *ui.Context, p uiPalette) {
	t := p.theme
	title := fmt.Sprintf(locales.CohortClassTrendTitle(), a.classTrendResult.ClassName)
	modal(c, &a.classTrendOpen, modalProps{
		Key:      "class-trend-chain",
		Title:    title,
		Width:    modalWidthWide,
		NoFooter: true,
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			if len(a.classTrendNodes) > 0 {
				ui.Text(c, locales.CohortClassTrendChainLabel()).FontSize(captionSize(t)).TextColor(p.muted)
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					for i, n := range a.classTrendNodes {
						if i > 0 {
							ui.Icon(c, iconArrowRight).Size(14, 14).TextColor(p.muted)
						}
						badge(c, p, fmt.Sprintf("%s %s (%s)", a.stageName(n.Phase), n.Name, formatInt(n.StudentCount)), toneAccent)
					}
				})
			}
			rule(c)
			pts := a.classTrendResult.Points
			if len(pts) == 0 {
				smallEmptyState(c, p, locales.CohortClassTrendNoData())
			} else {
				a.classTrendTable.RowHeight = tableRowHeight(t)
				a.classTrendTable.Key = func(row int) any { return pts[row].AssessmentID }
				cols := []ui.TableColumn{
					{Title: locales.ExamName(), Width: 0},
					{Title: locales.ExamDate(), Width: t.Space(28)},
					{Title: locales.AnalysisCompletionScore(), Width: t.Space(26), Align: ui.End},
				}
				ui.Table(c, &a.classTrendTable, cols, len(pts), func(row, col int) {
					pt := pts[row]
					switch col {
					case 0:
						cellText(c, p, pt.ExamName)
					case 1:
						mutedCellText(c, p, pt.ExamDate)
					case 2:
						ui.Text(c, formatDecimal(pt.CompletionScore, 2)).FontFeatures("tnum")
					}
				}).Label(title).FillWidth().Height(t.Space(56))
			}
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if plainButton(c, locales.Close()).Clicked() {
					a.classTrendOpen = false
				}
			})
		})
	})
}

func (a *app) newWalkingGroupModal(c *ui.Context, p uiPalette) {
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	t := p.theme
	wasOpen := a.newWalkingGroupOpen
	confirmed := modal(c, &a.newWalkingGroupOpen, modalProps{
		Key:        "new-walking-group",
		Title:      locales.CohortNewWalkingGroup(),
		OkText:     locales.Save(),
		CancelText: locales.PlanCancel(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			subjLabels := make([]string, 0, len(a.subjects))
			subjKeys := make([]string, 0, len(a.subjects))
			chosen := ""
			for _, s := range a.subjects {
				if !s.Enabled {
					continue
				}
				lbl := a.subjectName(s.Key, s.Name)
				subjLabels = append(subjLabels, lbl)
				subjKeys = append(subjKeys, s.Key)
				if s.Key == a.newWalkingSubject {
					chosen = lbl
				}
			}
			if chosen == "" && len(subjLabels) > 0 {
				chosen = subjLabels[0]
				a.newWalkingSubject = subjKeys[0]
			}
			a.dialogForm(c, func() {
				a.dialogField(c, "walk-subj", locales.GroupSubject(), "", func() {
					next := chosen
					selectField(c, "walk-subj-sel", &next, subjLabels, locales.GroupSubject())
					for i, lbl := range subjLabels {
						if lbl == next {
							a.newWalkingSubject = subjKeys[i]
						}
					}
				})
				a.dialogField(c, "walk-name", locales.CohortWalkingGroupName(), "", func() {
					textField(c, &a.newWalkingName, locales.CohortWalkingGroupName(), locales.CohortWalkingGroupName())
				})
			})
			if a.newWalkingError != "" {
				notice(c, p, noticeDanger, a.newWalkingError)
			}
		})
	})
	if confirmed && cohort != nil && stage != nil {
		created, err := a.store.CreateTeachingGroup(context.Background(), cohort.ID, stage.GradeLevel,
			a.newWalkingSubject, a.newWalkingName, "walking", 0)
		if err != nil {
			a.newWalkingError = err.Error()
			a.newWalkingGroupOpen = true
			return
		}
		a.newWalkingGroupOpen = false
		a.newWalkingError = ""
		a.reloadCohorts(context.Background())
		a.notifyMain(fmt.Sprintf(locales.CohortWalkingGroupCreatedToast(), created.Name))
	} else if wasOpen && !a.newWalkingGroupOpen {
		a.newWalkingError = ""
	}
}

func (a *app) openWalkingStudentsModal(g store.DataTeachingGroup) {
	if a.store == nil {
		return
	}
	ctx := context.Background()
	list, err := a.store.ListTeachingGroupStudents(ctx, g.ID)
	if err != nil {
		a.dataError = err.Error()
		return
	}
	stageID := g.StageID
	if stageID == 0 {
		if st := a.selectedStage(); st != nil {
			stageID = st.ID
		}
	}
	var candidates []store.DataStudent
	if stageID > 0 || g.CohortID > 0 {
		const candidateBatchSize = 10000
		for page := 0; ; page++ {
			batch, total, sErr := a.store.ListStudents(ctx, store.StudentFilter{
				CohortID: g.CohortID,
				StageID:  stageID,
				Phase:    g.Phase,
				Status:   "active",
				Page:     page,
				PageSize: candidateBatchSize,
			})
			if sErr != nil {
				a.dataError = sErr.Error()
				return
			}
			candidates = append(candidates, batch...)
			if len(candidates) >= total || len(batch) == 0 {
				break
			}
		}
	}
	a.walkingGroupID = g.ID
	a.walkingGroupName = g.Name
	a.walkingStudents = list
	a.walkingCandidateStudents = candidates
	a.walkingAddStudentChoice = ""
	a.walkingStudentsError = ""
	a.walkingStudentsOpen = true
}

func (a *app) walkingStudentsModal(c *ui.Context, p uiPalette) {
	t := p.theme
	modal(c, &a.walkingStudentsOpen, modalProps{
		Key:      "walking-students",
		Title:    fmt.Sprintf(locales.CohortWalkingStudentsTitle(), a.walkingGroupName),
		Width:    modalWidthWide,
		NoFooter: true,
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			editable := a.stageEditable()
			if editable {
				inGroup := map[int64]bool{}
				for _, s := range a.walkingStudents {
					inGroup[s.ID] = true
				}
				var candidateLabels []string
				candidateIDs := map[string]int64{}
				for _, s := range a.walkingCandidateStudents {
					if inGroup[s.ID] {
						continue
					}
					lbl := fmt.Sprintf("%s (%s · %s)", s.Name, dashOr(s.StudentNo), dashOr(s.ClassName))
					candidateLabels = append(candidateLabels, lbl)
					candidateIDs[lbl] = s.ID
				}
				if a.walkingAddStudentChoice == "" && len(candidateLabels) > 0 {
					a.walkingAddStudentChoice = candidateLabels[0]
				}
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					selectField(c, "walk-add-stu", &a.walkingAddStudentChoice, candidateLabels, locales.CohortWalkingAddStudent()).
						Grow(1).Shrink(1).MinWidth(0)
					addBtn := primaryButton(c, locales.CohortWalkingAddStudent())
					addBtn.Disabled(len(candidateLabels) == 0)
					if addBtn.Clicked() {
						if sid := candidateIDs[a.walkingAddStudentChoice]; sid > 0 {
							if err := a.store.AssignWalkingStudent(context.Background(), a.walkingGroupID, sid); err != nil {
								a.walkingStudentsError = err.Error()
							} else {
								if list, lErr := a.store.ListTeachingGroupStudents(context.Background(), a.walkingGroupID); lErr != nil {
									a.walkingStudentsError = lErr.Error()
								} else {
									a.walkingStudents = list
									a.walkingStudentsError = ""
								}
								a.walkingAddStudentChoice = ""
								a.reloadSelectedCohort(context.Background())
							}
						}
					}
				})
			}
			if a.walkingStudentsError != "" {
				notice(c, p, noticeDanger, a.walkingStudentsError)
			}
			rule(c)
			if len(a.walkingStudents) == 0 {
				smallEmptyState(c, p, locales.StudentsEmpty())
			} else {
				for _, s := range a.walkingStudents {
					s := s
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Text(c, fmt.Sprintf("%s · %s (%s)", s.Name, dashOr(s.StudentNo), dashOr(s.ClassName))).
							FontSize(bodySize(t)).Grow(1).Shrink(1).MinWidth(0).SingleLine()
						if editable {
							if plainButton(c.Key("walk-rm-"+strconv.FormatInt(s.ID, 10)), locales.CohortWalkingRemoveStudent()).Clicked() {
								if err := a.store.RemoveWalkingStudent(context.Background(), a.walkingGroupID, s.ID); err != nil {
									a.walkingStudentsError = err.Error()
								} else {
									if list, lErr := a.store.ListTeachingGroupStudents(context.Background(), a.walkingGroupID); lErr != nil {
										a.walkingStudentsError = lErr.Error()
									} else {
										a.walkingStudents = list
										a.walkingStudentsError = ""
									}
									a.reloadSelectedCohort(context.Background())
								}
							}
						}
					})
				}
			}
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if plainButton(c, locales.Close()).Clicked() {
					a.walkingStudentsOpen = false
				}
			})
		})
	})
}

func (a *app) openAssignTeacherModal(groupID int64) {
	a.reloadTeacherDirectory(context.Background())
	a.assignGroupID = groupID
	a.assignGroupChoice = ""
	for _, g := range a.teachingGroups {
		if groupID == 0 || g.ID == groupID {
			a.assignGroupID = g.ID
			a.assignGroupChoice = a.formatGroupChoiceLabel(g)
			break
		}
	}
	a.assignTeacherChoice = ""
	for _, occ := range a.teacherOccupancy {
		if occ.Status != store.TeacherStatusInactive {
			a.assignTeacherChoice = a.formatTeacherPickerLabel(occ)
			break
		}
	}
	a.assignError = ""
	a.refreshAssignWarning()
	a.assignTeacherOpen = true
}

func (a *app) formatGroupChoiceLabel(g store.DataTeachingGroup) string {
	return fmt.Sprintf("%s · %s", a.subjectName(g.Subject, g.Subject), g.Name)
}

func (a *app) refreshAssignWarning() {
	a.assignWarning = nil
	cohort := a.selectedCohort()
	occ := a.teacherByPickerLabel(a.assignTeacherChoice)
	if cohort == nil || occ == nil || a.store == nil {
		return
	}
	warn, err := a.store.DetectCrossGradeWarnings(context.Background(), occ.TeacherID, cohort.ID)
	if err == nil {
		a.assignWarning = warn
	}
}

func (a *app) assignTeacherModal(c *ui.Context, p uiPalette) {
	t := p.theme
	wasOpen := a.assignTeacherOpen
	confirmed := modal(c, &a.assignTeacherOpen, modalProps{
		Key:        "assign-teacher",
		Title:      locales.CohortAssignModalTitle(),
		OkText:     locales.CohortAssignConfirm(),
		CancelText: locales.PlanCancel(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			grpLabels := make([]string, 0, len(a.teachingGroups))
			grpMap := map[string]int64{}
			for _, g := range a.teachingGroups {
				lbl := a.formatGroupChoiceLabel(g)
				grpLabels = append(grpLabels, lbl)
				grpMap[lbl] = g.ID
			}
			tLabels := make([]string, 0, len(a.teacherOccupancy))
			for _, occ := range a.teacherOccupancy {
				if occ.Status != store.TeacherStatusInactive {
					tLabels = append(tLabels, a.formatTeacherPickerLabel(occ))
				}
			}
			a.dialogForm(c, func() {
				a.dialogField(c, "assign-grp", locales.CohortAssignGroupLabel(), "", func() {
					selectField(c, "assign-grp-sel", &a.assignGroupChoice, grpLabels, locales.CohortAssignGroupLabel())
					if gid := grpMap[a.assignGroupChoice]; gid > 0 {
						a.assignGroupID = gid
					}
				})
				prevT := a.assignTeacherChoice
				a.dialogField(c, "assign-teacher", locales.CohortTeacherSelectLabel(), "", func() {
					selectField(c, "assign-t-sel", &a.assignTeacherChoice, tLabels, locales.CohortTeacherSelectLabel())
				})
				if a.assignTeacherChoice != prevT {
					a.refreshAssignWarning()
				}
			})
			if a.assignWarning != nil {
				notice(c, p, noticeWarning, a.formatCrossGradeWarningText(a.assignWarning))
			}
			if a.assignError != "" {
				notice(c, p, noticeDanger, a.assignError)
			}
		})
	})
	if confirmed {
		occ := a.teacherByPickerLabel(a.assignTeacherChoice)
		if occ == nil || a.assignGroupID <= 0 {
			a.assignError = locales.SelectNoOptions()
			a.assignTeacherOpen = true
			return
		}
		warn, err := a.store.AssignTeacherToGroup(context.Background(), occ.TeacherID, a.assignGroupID, "primary")
		if err != nil {
			a.assignError = err.Error()
			a.assignTeacherOpen = true
			return
		}
		a.assignTeacherOpen = false
		a.assignError = ""
		a.reloadCohorts(context.Background())
		msg := fmt.Sprintf(locales.CohortAssignSavedToast(), occ.Name)
		if warn != nil {
			msg = a.formatCrossGradeWarningText(warn)
		}
		a.notifyMain(msg)
	} else if wasOpen && !a.assignTeacherOpen {
		a.assignError = ""
	}
}

func (a *app) endCohortAssignment(assignmentID int64) {
	if a.store == nil || assignmentID <= 0 {
		return
	}
	if err := a.store.EndTeachingAssignment(context.Background(), assignmentID, "manual_release"); err != nil {
		a.dataError = err.Error()
		return
	}
	a.reloadCohorts(context.Background())
	a.notifyMain(locales.CohortUnassignDoneToast())
}

func (a *app) newStudentModal(c *ui.Context, p uiPalette) {
	cohort := a.selectedCohort()
	t := p.theme
	wasOpen := a.newStudentOpen
	confirmed := modal(c, &a.newStudentOpen, modalProps{
		Key:        "new-student",
		Title:      locales.CohortNewStudent(),
		OkText:     locales.Save(),
		CancelText: locales.PlanCancel(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			classLabels := make([]string, 0, len(a.classes))
			for _, cls := range a.classes {
				classLabels = append(classLabels, cls.Name)
			}
			if a.newStudentClassChoice == "" && len(classLabels) > 0 {
				a.newStudentClassChoice = classLabels[0]
			}
			trackLabels := []string{locales.TrackPhysics(), locales.TrackHistory()}
			trackCodes := []string{"physics", "history"}
			chosenTrack := trackLabels[0]
			if a.newStudentTrackChoice == "history" {
				chosenTrack = trackLabels[1]
			}
			a.dialogForm(c, func() {
				a.dialogField(c, "stu-no", locales.ColumnStudentNo(), "", func() {
					textField(c, &a.newStudentNo, locales.ColumnStudentNo(), locales.ColumnStudentNo())
				})
				a.dialogField(c, "stu-name", locales.ColumnName(), "", func() {
					textField(c, &a.newStudentName, locales.ColumnName(), locales.ColumnName())
				})
				a.dialogField(c, "stu-class", locales.ColumnClass(), "", func() {
					selectField(c, "stu-class-sel", &a.newStudentClassChoice, classLabels, locales.ColumnClass())
				})
				a.dialogField(c, "stu-track", locales.ColumnTrack(), "", func() {
					next := chosenTrack
					selectField(c, "stu-track-sel", &next, trackLabels, locales.ColumnTrack())
					for i, lbl := range trackLabels {
						if lbl == next {
							a.newStudentTrackChoice = trackCodes[i]
						}
					}
				})
				a.dialogField(c, "stu-entry", locales.StudentProfileEntryTotal(), "", func() {
					numberField(c, &a.newStudentEntryTotal, 0, 1000, 1, locales.StudentProfileEntryTotal())
				})
			})
			if a.newStudentError != "" {
				notice(c, p, noticeDanger, a.newStudentError)
			}
		})
	})
	if confirmed && cohort != nil {
		var totalPtr *float64
		if a.newStudentEntryTotal > 0 {
			v := a.newStudentEntryTotal
			totalPtr = &v
		}
		track := a.newStudentTrackChoice
		if track == "" {
			track = "physics"
		}
		created, err := a.store.CreateStudent(context.Background(), cohort.ID, store.ImportStudent{
			StudentNo:  a.newStudentNo,
			Name:       a.newStudentName,
			ClassName:  a.newStudentClassChoice,
			Track:      track,
			Status:     "active",
			TotalScore: totalPtr,
		})
		if err != nil {
			a.newStudentError = err.Error()
			a.newStudentOpen = true
			return
		}
		a.newStudentOpen = false
		a.newStudentError = ""
		a.reloadCohorts(context.Background())
		a.notifyMain(fmt.Sprintf(locales.CohortStudentCreatedToast(), created.Name))
	} else if wasOpen && !a.newStudentOpen {
		a.newStudentError = ""
	}
}

func (a *app) openBaselineEditModal() {
	a.baselineEditOpen = true
	a.baselineEditScope = "class"
	a.baselineEditClassChoice = ""
	if len(a.classes) > 0 {
		a.baselineEditClassChoice = a.classes[0].Name
	}
	a.baselineEditSubjectChoice = ""
	for _, s := range a.subjects {
		if s.Enabled {
			a.baselineEditSubjectChoice = a.subjectName(s.Key, s.Name)
			break
		}
	}
	a.baselineEditAverage = 0
	a.baselineEditRatio = 1.0
	a.baselineEditError = ""
}

func (a *app) baselineEditModal(c *ui.Context, p uiPalette) {
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	t := p.theme
	wasOpen := a.baselineEditOpen
	confirmed := modal(c, &a.baselineEditOpen, modalProps{
		Key:        "baseline-edit",
		Title:      locales.CohortBaselineModalTitle(),
		OkText:     locales.Save(),
		CancelText: locales.PlanCancel(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			scopeLabels := []string{locales.CohortBaselineScopeClass(), locales.CohortBaselineScopeGrade()}
			scopeChosen := scopeLabels[0]
			if a.baselineEditScope == "grade" {
				scopeChosen = scopeLabels[1]
			}
			classLabels := make([]string, 0, len(a.classes))
			for _, cls := range a.classes {
				classLabels = append(classLabels, cls.Name)
			}
			subjLabels := make([]string, 0, len(a.subjects)+1)
			subjKeys := map[string]string{}
			for _, s := range a.subjects {
				if s.Enabled {
					lbl := a.subjectName(s.Key, s.Name)
					subjLabels = append(subjLabels, lbl)
					subjKeys[lbl] = s.Key
				}
			}
			a.dialogForm(c, func() {
				a.dialogField(c, "bl-scope", locales.CohortBaselineScopeLabel(), "", func() {
					next := scopeChosen
					selectField(c, "bl-scope-sel", &next, scopeLabels, locales.CohortBaselineScopeLabel())
					if next == scopeLabels[1] {
						a.baselineEditScope = "grade"
					} else {
						a.baselineEditScope = "class"
					}
				})
				if a.baselineEditScope == "class" {
					a.dialogField(c, "bl-class", locales.ColumnClass(), "", func() {
						selectField(c, "bl-class-sel", &a.baselineEditClassChoice, classLabels, locales.ColumnClass())
					})
				}
				a.dialogField(c, "bl-subj", locales.BaselineSubject(), "", func() {
					selectField(c, "bl-subj-sel", &a.baselineEditSubjectChoice, subjLabels, locales.BaselineSubject())
				})
				a.dialogField(c, "bl-avg", locales.CohortBaselineAverageLabel(), "", func() {
					numberField(c, &a.baselineEditAverage, 0, 1000, 0.5, locales.CohortBaselineAverageLabel())
				})
				a.dialogField(c, "bl-ratio", locales.CohortBaselineRatioLabel(), "", func() {
					numberField(c, &a.baselineEditRatio, 0, 5, 0.01, locales.CohortBaselineRatioLabel())
				})
			})
			if a.baselineEditError != "" {
				notice(c, p, noticeDanger, a.baselineEditError)
			}
		})
	})
	if confirmed && cohort != nil && stage != nil {
		var scopeID int64
		if a.baselineEditScope == "class" {
			for _, cls := range a.classes {
				if cls.Name == a.baselineEditClassChoice {
					scopeID = cls.ID
					break
				}
			}
		}
		subjKey := a.baselineEditSubjectChoice
		for _, s := range a.subjects {
			if a.subjectName(s.Key, s.Name) == a.baselineEditSubjectChoice {
				subjKey = s.Key
				break
			}
		}
		var ratioPtr *float64
		if a.baselineEditRatio > 0 {
			r := a.baselineEditRatio
			ratioPtr = &r
		}
		if err := a.store.UpsertBaselineOverride(context.Background(), cohort.ID, stage.GradeLevel,
			a.baselineEditScope, scopeID, subjKey, a.baselineEditAverage, ratioPtr); err != nil {
			a.baselineEditError = err.Error()
			a.baselineEditOpen = true
			return
		}
		a.baselineEditOpen = false
		a.baselineEditError = ""
		a.reloadCohorts(context.Background())
		a.notifyMain(locales.CohortBaselineSavedToast())
	} else if wasOpen && !a.baselineEditOpen {
		a.baselineEditError = ""
	}
}

func (a *app) newTeacherModal(c *ui.Context, p uiPalette) {
	t := p.theme
	wasOpen := a.newTeacherOpen
	confirmed := modal(c, &a.newTeacherOpen, modalProps{
		Key:        "new-teacher",
		Title:      locales.TeachersNewModalTitle(),
		OkText:     locales.Save(),
		CancelText: locales.PlanCancel(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			subjLabels := make([]string, 0, len(a.subjects))
			subjMap := map[string]string{}
			chosen := ""
			for _, s := range a.subjects {
				if s.Enabled {
					lbl := a.subjectName(s.Key, s.Name)
					subjLabels = append(subjLabels, lbl)
					subjMap[lbl] = s.Key
					if s.Key == a.newTeacherSubject {
						chosen = lbl
					}
				}
			}
			if chosen == "" && len(subjLabels) > 0 {
				chosen = subjLabels[0]
				a.newTeacherSubject = subjMap[chosen]
			}
			a.dialogForm(c, func() {
				a.dialogField(c, "tc-no", locales.TeacherNo(), "", func() {
					textField(c, &a.newTeacherNo, locales.TeacherNo(), locales.TeacherNo())
				})
				a.dialogField(c, "tc-name", locales.ColumnName(), "", func() {
					textField(c, &a.newTeacherName, locales.ColumnName(), locales.ColumnName())
				})
				a.dialogField(c, "tc-subj", locales.TeacherSubject(), "", func() {
					next := chosen
					selectField(c, "tc-subj-sel", &next, subjLabels, locales.TeacherSubject())
					if key := subjMap[next]; key != "" {
						a.newTeacherSubject = key
					}
				})
			})
			if a.newTeacherError != "" {
				notice(c, p, noticeDanger, a.newTeacherError)
			}
		})
	})
	if confirmed {
		created, err := a.store.CreateTeacher(context.Background(), store.ImportTeacher{
			TeacherNo: a.newTeacherNo,
			Name:      a.newTeacherName,
			Subject:   a.newTeacherSubject,
		})
		if err != nil {
			a.newTeacherError = err.Error()
			a.newTeacherOpen = true
			return
		}
		a.newTeacherOpen = false
		a.newTeacherError = ""
		a.reloadTeacherDirectory(context.Background())
		a.reloadSchoolOverview(context.Background())
		a.notifyMain(fmt.Sprintf(locales.TeachersCreatedToast(), created.Name))
	} else if wasOpen && !a.newTeacherOpen {
		a.newTeacherError = ""
	}
}

func (a *app) assignmentTeacherLink(c *ui.Context, p uiPalette, key, name, occStatus string) ui.Element {
	t := p.theme
	isMulti := occStatus == store.TeacherStatusActiveMulti
	tip := name
	if isMulti {
		tip = name + " · " + locales.TeachersFilterActiveMulti()
	}
	btn := ui.ButtonBase(c.Key(key)).Height(t.Space(6)).Padding(0, t.Space(2)).Radius(t.Space(3)).
		Center().Cursor(ui.CursorPointer).Label(name).Tooltip(tip).TextColor(p.info)
	switch {
	case btn.Pressed():
		btn.Background(p.sb.FlatFacePressed)
	case btn.Hovered():
		btn.Background(p.infoSoft)
	default:
		btn.Background(p.fill)
	}
	btn.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
			if isMulti {
				ui.Box(c).Size(6, 6).Radius(3).Background(p.warning).Shrink(0)
			}
			ui.Text(c, name).FontSize(captionSize(t)).FontWeight(500).SingleLine()
			if isMulti {
				ui.Text(c, locales.TeachersFilterActiveMulti()).FontSize(captionSize(t)).
					TextColor(p.muted).SingleLine()
			}
			ui.Icon(c, iconExternalLink).Size(12, 12)
		})
	})
	return btn
}

func (a *app) teachersDirectoryPage(c *ui.Context, p uiPalette) {
	if a.teacherProfileOpen && a.selectedTeacherID > 0 {
		a.teacherProfilePage(c, p)
		return
	}
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.TeachersDirectoryTitle(), locales.TeachersDirectorySubtitle(), func() {
			if plainButton(c, locales.ImportTeachers()).Clicked() {
				a.openDataImport(importer.KindTeachers, 0, 0)
			}
			if labelledButton(c, locales.TeachersNewTeacher(), iconPlus, len(a.teacherOccupancy) == 0).Clicked() {
				a.newTeacherNo = ""
				a.newTeacherName = ""
				a.newTeacherSubject = ""
				a.newTeacherError = ""
				a.newTeacherOpen = true
			}
		})

		if a.dataError != "" {
			notice(c, p, noticeDanger, a.dataError)
		}
		if a.dataMessage != "" {
			notice(c, p, noticeSuccess, a.dataMessage)
		}

		// 状态计数（基于全校师资实时占用）
		var singleCnt, multiCnt, idleCnt, inactiveCnt int
		for _, occ := range a.teacherOccupancy {
			switch occ.Status {
			case store.TeacherStatusActiveSingle:
				singleCnt++
			case store.TeacherStatusActiveMulti:
				multiCnt++
			case store.TeacherStatusIdle:
				idleCnt++
			case store.TeacherStatusInactive:
				inactiveCnt++
			}
		}

		filterPillBar(c, p, &a.teacherOccupancyFilter, []filterPillOption{
			{Key: "", Label: locales.TeachersFilterAll(), Count: len(a.teacherOccupancy), HasCount: true},
			{Key: store.TeacherStatusActiveSingle, Label: locales.TeachersFilterActiveSingle(), Count: singleCnt, HasCount: true},
			{Key: store.TeacherStatusActiveMulti, Label: locales.TeachersFilterActiveMulti(), Count: multiCnt, HasCount: true},
			{Key: store.TeacherStatusIdle, Label: locales.TeachersFilterIdle(), Count: idleCnt, HasCount: true},
			{Key: store.TeacherStatusInactive, Label: locales.TeachersFilterInactive(), Count: inactiveCnt, HasCount: true},
		}, locales.TeachersOccupancyCol())

		filtered := a.filteredTeacherOccupancy()
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
			ui.Column(c).Width(t.Space(72)).Children(func() {
				searchField(c, &a.teacherQuery, locales.DataSearchFaculty(), locales.DataSearchFaculty())
			})
			subjLabels := []string{locales.DataAllSubjects()}
			subjCodes := []string{""}
			chosenSubj := subjLabels[0]
			for _, s := range a.subjects {
				if !s.Enabled {
					continue
				}
				lbl := a.subjectName(s.Key, s.Name)
				subjLabels = append(subjLabels, lbl)
				subjCodes = append(subjCodes, s.Key)
				if s.Key == a.teacherSubjectFilter {
					chosenSubj = lbl
				}
			}
			nextSubj := chosenSubj
			selectField(c, "teachers-dir-subj", &nextSubj, subjLabels, locales.DataFilterSubject()).Width(t.Space(40))
			if nextSubj != chosenSubj {
				for idx, lbl := range subjLabels {
					if lbl == nextSubj {
						a.teacherSubjectFilter = subjCodes[idx]
						break
					}
				}
			}
			ui.Spacer(c)
			ui.Text(c, pluralCount(len(filtered), locales.DataRecordCountOne(), locales.DataRecordCount())).
				FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
		})

		if len(filtered) == 0 {
			hasFilter := strings.TrimSpace(a.teacherQuery) != "" || a.teacherOccupancyFilter != "" || a.teacherSubjectFilter != ""
			if hasFilter {
				emptyState(c, p, iconFaculty, locales.FilterEmptyTitle(), locales.FilterEmptyHint(), func() {
					if plainButton(c, locales.ClearFilters()).Clicked() {
						a.teacherQuery = ""
						a.teacherOccupancyFilter = ""
						a.teacherSubjectFilter = ""
					}
				})
				return
			}
			emptyState(c, p, iconFaculty, locales.FacultyEmpty(), locales.FacultyEmptyHint(), func() {
				if labelledButton(c, locales.TeachersNewTeacher(), iconPlus, true).Clicked() {
					a.newTeacherNo = ""
					a.newTeacherName = ""
					a.newTeacherSubject = ""
					a.newTeacherError = ""
					a.newTeacherOpen = true
				}
			})
			return
		}

		a.teacherDirectoryTable.RowHeight = tableRowHeight(t)
		selected := -1
		for i, occ := range filtered {
			if occ.TeacherID == a.selectedTeacherID {
				selected = i
				break
			}
		}
		a.teacherDirectoryTable.Selected = &selected
		a.teacherDirectoryTable.Key = func(row int) any { return filtered[row].TeacherID }
		columns := []ui.TableColumn{
			{Title: locales.ColumnName(), Width: t.Space(28)},
			{Title: locales.TeacherNo(), Width: t.Space(22)},
			{Title: locales.TeacherSubject(), Width: t.Space(20)},
			{Title: locales.TeachersOccupancyCol(), Width: t.Space(28)},
			{Title: locales.TeachersAssignmentsCol(), Width: 0},
		}
		table := ui.Table(c, &a.teacherDirectoryTable, columns, len(filtered), func(row, col int) {
			occ := filtered[row]
			switch col {
			case 0:
				cellText(c, p, occ.Name)
			case 1:
				mutedCellText(c, p, dashOr(occ.TeacherNo)).FontFeatures("tnum")
			case 2:
				cellText(c, p, a.subjectName(occ.PrimarySubject, occ.PrimarySubject))
			case 3:
				lbl, tone := a.teacherOccupancyBadge(occ.Status)
				badge(c, p, lbl, tone)
			case 4:
				mutedCellText(c, p, a.teacherDutiesSummary(occ))
			}
		}).Label(locales.TeachersDirectoryTitle()).Grow(1).Shrink(1).MinHeight(0).Fill()
		if table.Changed() && selected >= 0 {
			a.openTeacherProfile(filtered[selected].TeacherID)
		}
	})
}

func (a *app) filteredTeacherOccupancy() []store.TeacherOccupancy {
	q := strings.ToLower(strings.TrimSpace(a.teacherQuery))
	qSubj := a.subjectQuery(a.teacherQuery)
	out := make([]store.TeacherOccupancy, 0, len(a.teacherOccupancy))
	for _, occ := range a.teacherOccupancy {
		if a.teacherOccupancyFilter != "" && occ.Status != a.teacherOccupancyFilter {
			continue
		}
		if a.teacherSubjectFilter != "" && occ.PrimarySubject != a.teacherSubjectFilter {
			continue
		}
		if q != "" {
			subjLabel := strings.ToLower(a.subjectName(occ.PrimarySubject, occ.PrimarySubject))
			matched := strings.Contains(strings.ToLower(occ.Name), q) ||
				strings.Contains(strings.ToLower(occ.TeacherNo), q) ||
				strings.Contains(strings.ToLower(occ.PrimarySubject), q) ||
				strings.Contains(subjLabel, q) ||
				(qSubj != "" && strings.Contains(strings.ToLower(occ.PrimarySubject), strings.ToLower(qSubj)))
			if !matched {
				continue
			}
		}
		out = append(out, occ)
	}
	return out
}

func (a *app) teacherOccupancyBadge(status string) (string, badgeTone) {
	switch status {
	case store.TeacherStatusActiveSingle:
		return locales.TeachersFilterActiveSingle(), toneSuccess
	case store.TeacherStatusActiveMulti:
		return locales.TeachersFilterActiveMulti(), toneWarning
	case store.TeacherStatusIdle:
		return locales.TeachersFilterIdle(), toneAccent
	default:
		return locales.TeachersFilterInactive(), toneNeutral
	}
}

func (a *app) teacherDutiesSummary(occ store.TeacherOccupancy) string {
	if len(occ.ActiveAssignments) > 0 {
		parts := make([]string, 0, len(occ.ActiveAssignments))
		for _, item := range occ.ActiveAssignments {
			if item.Role == "head_teacher" {
				parts = append(parts, fmt.Sprintf("%s %s (%s)", a.cohortName(item.EnrollmentYear), item.ClassName, locales.ClassHeadTeacher()))
			} else {
				parts = append(parts, fmt.Sprintf("%s %s", a.cohortName(item.EnrollmentYear), item.TeachingGroupName))
			}
		}
		return strings.Join(parts, locales.ListSeparator())
	}
	if occ.LastRelease != nil {
		return fmt.Sprintf("%s %s", a.cohortName(occ.LastRelease.EnrollmentYear), occ.LastRelease.ClassName)
	}
	return "-"
}
