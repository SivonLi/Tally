package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"tally/internal/importer"
	"tally/internal/store"
	"tally/locales"
)

func (a *app) dataStageReady() bool {
	return a.selectedCohort() != nil && a.selectedStage() != nil
}

func (a *app) noStageState(c *ui.Context, p uiPalette, icon *ui.SVG) {
	title := locales.DataNoCohort()
	body := locales.DataNoCohortHint()
	if a.selectedCohort() != nil {
		title = locales.DataNoStage()
		body = locales.DataNoStageHint()
	}
	emptyState(c, p, icon, title, body, func() {
		if labelledButton(c, locales.NewCohort(), iconPlus, true).Clicked() {
			a.openNewCohort()
		}
	})
}

func (a *app) studentsPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavStudents(), locales.StudentsPageSubtitle(), func() {
			button := labelledButton(c, locales.ImportStudents(), iconImports, false)
			button.Disabled(a.dataBusy || !a.stageEditable())
			if button.Clicked() {
				a.openImportWizard(importer.KindStudents)
			}
		})
		if !a.dataStageReady() {
			a.noStageState(c, p, iconStudents)
			return
		}
		if !a.stageEditable() {
			notice(c, p, noticeInfo, locales.StageReadOnly())
		}
		a.studentFilters(c, p)
		if a.dataError != "" {
			notice(c, p, noticeDanger, a.dataError)
		}
		if a.dataBusy {
			notice(c, p, noticeInfo, locales.ImportBusy())
		}
		if a.dataMessage != "" {
			notice(c, p, noticeSuccess, a.dataMessage)
		}
		ui.Row(c).Grow(1).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Grow(1).Gap(t.Space(2)).Children(func() {
				a.studentTable.RowHeight = t.Space(10)
				selected := -1
				for index, student := range a.students {
					if student.ID == a.selectedStudentID {
						selected = index
						break
					}
				}
				if len(a.students) == 0 {
					emptyState(c, p, iconStudents, locales.StudentsEmpty(), locales.StudentsEmptyHint(), func() {
						if labelledButton(c, locales.ImportStudents(), iconImports, true).Clicked() {
							a.openImportWizard(importer.KindStudents)
						}
					})
					return
				}
				a.studentTable.Selected = &selected
				a.studentTable.Key = func(row int) any { return a.students[row].ID }
				table := ui.Table(c, &a.studentTable, studentColumns(t), len(a.students), func(row, col int) {
					student := a.students[row]
					switch col {
					case 0:
						ui.Text(c, student.StudentNo).SingleLine().FontFeatures("tnum")
					case 1:
						ui.Text(c, student.Name).SingleLine()
					case 2:
						ui.Text(c, student.ClassName).SingleLine()
					case 3:
						ui.Text(c, a.trackName(student.Track, student.Track)).SingleLine()
					case 4:
						ui.Text(c, a.statusName(student.Status)).SingleLine()
					case 5:
						if student.TotalScore == nil {
							ui.Text(c, "-").TextColor(p.muted)
						} else {
							ui.Text(c, formatDecimal(*student.TotalScore, 1)).FontFeatures("tnum")
						}
					}
				}).Label(locales.NavStudents()).Grow(1).FillWidth()
				if table.Changed() && selected >= 0 {
					a.selectStudent(a.students[selected].ID)
				}
			})
			if a.studentDetailOpen {
				a.studentPanel(c, p)
			}
		})
	})
}

func studentColumns(t ui.Theme) []ui.TableColumn {
	return []ui.TableColumn{
		{Title: locales.ColumnStudentNo(), Width: t.Space(18)},
		{Title: locales.ColumnName(), Width: t.Space(28)},
		{Title: locales.ColumnClass(), Width: t.Space(24)},
		{Title: locales.ColumnTrack(), Width: t.Space(20)},
		{Title: locales.ColumnStatus(), Width: t.Space(20)},
		{Title: locales.ColumnStudentTotal(), Width: 0, Align: ui.End},
	}
}

func (a *app) studentFilters(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
		ui.Column(c).Width(t.Space(56)).Gap(0).Children(func() {
			nextQuery := a.dataQuery
			searchField(c, &nextQuery, locales.DataSearchStudents(), locales.DataSearchStudents())
			if nextQuery != a.dataQuery {
				a.dataQuery = nextQuery
				a.selectedStudentID = 0
				a.reloadStudents(context.Background())
			}
		})
		labels, ids := a.classFilterLabels()
		chosenClass := labels[0]
		for index, id := range ids {
			if id == a.dataClassFilter {
				chosenClass = labels[index]
				break
			}
		}
		nextClass := chosenClass
		selectField(c, "students-class", &nextClass, labels, locales.DataFilterClass()).Width(t.Space(40))
		if nextClass != chosenClass {
			for index, label := range labels {
				if label == nextClass {
					a.dataClassFilter = ids[index]
					break
				}
			}
			a.selectedStudentID = 0
			a.reloadStudents(context.Background())
		}
		trackLabels := a.trackLabels()
		chosenTrack := trackLabels[0]
		for index, track := range a.tracks {
			if track.Key == a.dataTrackFilter {
				chosenTrack = trackLabels[index+1]
				break
			}
		}
		nextTrack := chosenTrack
		selectField(c, "students-track", &nextTrack, trackLabels, locales.DataFilterTrack()).Width(t.Space(24))
		if nextTrack != chosenTrack {
			a.dataTrackFilter = ""
			for index, track := range a.tracks {
				if trackLabels[index+1] == nextTrack {
					a.dataTrackFilter = track.Key
					break
				}
			}
			a.selectedStudentID = 0
			a.reloadStudents(context.Background())
		}
		statusLabels := a.statusLabels()
		statusCodes := []string{"", "active", "transferred", "suspended", "borrowed", "left_school"}
		chosenStatus := statusLabels[0]
		for index, code := range statusCodes {
			if code == a.dataStatusFilter {
				chosenStatus = statusLabels[index]
				break
			}
		}
		nextStatus := chosenStatus
		selectField(c, "students-status", &nextStatus, statusLabels, locales.DataFilterStatus()).Width(t.Space(24))
		if nextStatus != chosenStatus {
			for index, label := range statusLabels {
				if label == nextStatus {
					a.dataStatusFilter = statusCodes[index]
					break
				}
			}
			a.selectedStudentID = 0
			a.reloadStudents(context.Background())
		}
		ui.Spacer(c)
		ui.Textf(c, locales.DataStudentCount(), formatInt(a.studentsTotal)).FontSize(captionSize(t)).
			TextColor(p.muted).FontFeatures("tnum")
	})
}

func (a *app) selectStudent(id int64) {
	a.selectedStudentID = id
	stage := a.selectedStage()
	if stage == nil || a.store == nil {
		return
	}
	detail, err := a.store.GetStudent(context.Background(), stage.ID, id)
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.studentDetail = detail
	a.studentDetailOpen = true
}

func (a *app) studentPanel(c *ui.Context, p uiPalette) {
	t := p.theme
	student := a.studentDetail
	listPanel(c, p, locales.StudentDetailTitle(), 0, func() {
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
				ui.Column(c).Gap(0).Children(func() {
					ui.Text(c, student.Name).FontSize(sectionSize(t)).FontWeight(600)
					ui.Text(c, student.StudentNo).FontSize(captionSize(t)).TextColor(p.muted).
						FontFeatures("tnum")
				})
				a.detailRow(c, p, locales.ColumnClass(), student.ClassName)
				a.detailRow(c, p, locales.ColumnTrack(), a.trackName(student.Track, student.Track))
				a.detailRow(c, p, locales.ColumnStatus(), a.statusName(student.Status))
				if student.TotalScore != nil {
					a.detailRow(c, p, locales.ColumnTotal(), formatDecimal(*student.TotalScore, 1))
				}
				rule(c)
				ui.Text(c, locales.StudentTeachingGroups()).FontSize(captionSize(t)).
					TextColor(p.muted)
				if len(student.TeachingGroups) == 0 {
					ui.Text(c, locales.StudentNoGroups()).FontSize(bodySize(t)).
						TextColor(p.muted).Pretty()
				}
				for _, group := range student.TeachingGroups {
					parts := strings.SplitN(group, ":", 2)
					label := group
					if len(parts) == 2 {
						label = a.subjectName(parts[0], parts[0]) + " · " + parts[1]
					}
					ui.Text(c, label).FontSize(bodySize(t))
				}
				rule(c)
				ui.Text(c, locales.StudentSource()).FontSize(captionSize(t)).TextColor(p.muted)
				ui.Text(c, a.studentSource(student)).FontSize(bodySize(t)).
					TextColor(p.inkSecondary).Selectable().Pretty()
				if plainButton(c, locales.StudentCloseDetail()).Clicked() {
					a.studentDetailOpen = false
					a.selectedStudentID = 0
				}
			})
		})
	})
}

func (a *app) detailRow(c *ui.Context, p uiPalette, label, value string) {
	t := p.theme
	ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted).Width(t.Space(20))
		ui.Text(c, value).FontSize(bodySize(t)).Grow(1).SingleLine()
	})
}

func (a *app) studentSource(student store.DataStudent) string {
	if student.ImportRunID == 0 {
		return locales.StudentSourceManual()
	}
	if student.SourceSheet == "" {
		return fmt.Sprintf(locales.StudentSourceRun(), student.ImportRunID)
	}
	return fmt.Sprintf(locales.StudentSourceSheet(), student.SourceSheet, student.SourceRow)
}

func (a *app) facultyPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavFaculty(), locales.FacultyPageSubtitle(), func() {
			teacherButton := plainButton(c, locales.ImportTeachers())
			teacherButton.Disabled(a.dataBusy || !a.stageEditable())
			if teacherButton.Clicked() {
				a.openImportWizard(importer.KindTeachers)
			}
			button := labelledButton(c, locales.ImportAssignments(), iconImports, false)
			button.Disabled(a.dataBusy || !a.stageEditable())
			if button.Clicked() {
				a.openImportWizard(importer.KindAssignments)
			}
		})
		if !a.dataStageReady() {
			a.noStageState(c, p, iconFaculty)
			return
		}
		if !a.stageEditable() {
			notice(c, p, noticeInfo, locales.StageReadOnly())
		}
		if a.dataError != "" {
			notice(c, p, noticeDanger, a.dataError)
		}
		if a.dataBusy {
			notice(c, p, noticeInfo, locales.ImportBusy())
		}
		if a.dataMessage != "" {
			notice(c, p, noticeSuccess, a.dataMessage)
		}
		ui.Row(c).Children(func() {
			segmentedField(c, &a.facultyTab,
				[]string{locales.FacultyTabTeachers(), locales.FacultyTabAssignments()},
				locales.FacultyViewLabel())
		})
		a.facultyFilters(c, p)
		if a.facultyTab == 1 {
			a.teachingGroupTable(c, p, locales.FacultyAssignmentsTitle())
			return
		}
		if len(a.teachers) == 0 {
			emptyState(c, p, iconFaculty, locales.FacultyEmpty(), locales.FacultyEmptyHint(), nil)
			return
		}
		a.teacherTable.RowHeight = t.Space(10)
		columns := []ui.TableColumn{
			{Title: locales.ColumnName(), Width: t.Space(24)},
			{Title: locales.TeacherNo(), Width: t.Space(18)},
			{Title: locales.TeacherSubject(), Width: t.Space(18)},
			{Title: locales.TeacherGroups(), Width: 0, Align: ui.End},
			{Title: locales.TeacherAssignments(), Width: 0, Align: ui.End},
			{Title: locales.ColumnSource(), Width: t.Space(28)},
		}
		ui.Table(c, &a.teacherTable, columns, len(a.teachers), func(row, col int) {
			teacher := a.teachers[row]
			switch col {
			case 0:
				ui.Text(c, teacher.Name).SingleLine()
			case 1:
				ui.Text(c, teacher.TeacherNo).TextColor(p.muted).SingleLine().FontFeatures("tnum")
			case 2:
				ui.Text(c, a.subjectName(teacher.Subject, teacher.Subject)).SingleLine()
			case 3:
				ui.Text(c, formatInt(teacher.Groups)).FontFeatures("tnum")
			case 4:
				ui.Text(c, formatInt(teacher.Assignments)).FontFeatures("tnum")
			case 5:
				ui.Text(c, dataSourceLabel(teacher.ImportRunID, teacher.SourceSheet, teacher.SourceRow)).
					TextColor(p.muted).SingleLine()
			}
		}).Label(locales.NavFaculty()).Grow(1).FillWidth()
	})
}

func (a *app) facultyFilters(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
		ui.Column(c).Width(t.Space(56)).Children(func() {
			if a.facultyTab == 1 {
				nextQuery := a.groupQuery
				searchField(c, &nextQuery, locales.DataSearchGroups(), locales.DataSearchGroups())
				if nextQuery != a.groupQuery {
					a.groupQuery = nextQuery
					a.reloadDataPages(context.Background())
				}
				return
			}
			nextQuery := a.teacherQuery
			searchField(c, &nextQuery, locales.DataSearchFaculty(), locales.DataSearchFaculty())
			if nextQuery != a.teacherQuery {
				a.teacherQuery = nextQuery
				a.reloadDataPages(context.Background())
			}
		})
		ui.Spacer(c)
		if a.facultyTab == 1 {
			ui.Textf(c, locales.DataRecordCount(), formatInt(len(a.teachingGroups))).FontSize(captionSize(t)).
				TextColor(p.muted).FontFeatures("tnum")
		} else {
			ui.Textf(c, locales.DataRecordCount(), formatInt(len(a.teachers))).FontSize(captionSize(t)).
				TextColor(p.muted).FontFeatures("tnum")
		}
	})
}

func (a *app) teachingGroupTable(c *ui.Context, p uiPalette, label string) {
	if len(a.teachingGroups) == 0 {
		emptyState(c, p, iconGroups, locales.GroupsEmpty(), locales.GroupsEmptyHint(), nil)
		return
	}
	a.groupTableWith(c, p, label, a.teachingGroups)
}

func (a *app) groupsPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavGroups(), locales.GroupsPageSubtitle(), nil)
		if !a.dataStageReady() {
			a.noStageState(c, p, iconGroups)
			return
		}
		if a.dataError != "" {
			notice(c, p, noticeDanger, a.dataError)
		}
		if a.dataBusy {
			notice(c, p, noticeInfo, locales.ImportBusy())
		}
		if a.dataMessage != "" {
			notice(c, p, noticeSuccess, a.dataMessage)
		}
		if len(a.teachingGroups) == 0 {
			emptyState(c, p, iconGroups, locales.GroupsEmpty(), locales.GroupsEmptyHint(), nil)
			return
		}
		a.groupFilters(c, p)
		groups := a.visibleTeachingGroups()
		if len(groups) == 0 {
			emptyState(c, p, iconGroups, locales.GroupsEmpty(), locales.GroupsEmptyHint(), nil)
			return
		}
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Textf(c, locales.GroupsCount(), formatInt(len(groups))).FontSize(bodySize(t)).
				TextColor(p.muted)
		})
		a.groupTableWith(c, p, locales.NavGroups(), groups)
	})
}

func (a *app) groupFilters(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
		ui.Column(c).Width(t.Space(56)).Children(func() {
			nextQuery := a.groupQuery
			ui.SearchField(c, &nextQuery).Label(locales.DataSearchGroups())
			if nextQuery != a.groupQuery {
				a.groupQuery = nextQuery
				a.reloadDataPages(context.Background())
			}
		})
		labels := []string{locales.DataAllSubjects()}
		codes := []string{""}
		chosen := labels[0]
		for _, subject := range a.subjects {
			labels = append(labels, a.subjectName(subject.Key, subject.Name))
			codes = append(codes, subject.Key)
			if subject.Key == a.groupSubjectFilter {
				chosen = a.subjectName(subject.Key, subject.Name)
			}
		}
		next := chosen
		selectField(c, "groups-subject", &next, labels, locales.DataFilterSubject()).Width(t.Space(40))
		if next != chosen {
			for index, label := range labels {
				if label == next {
					a.groupSubjectFilter = codes[index]
					break
				}
			}
		}
	})
}

func (a *app) visibleTeachingGroups() []store.DataTeachingGroup {
	if a.groupSubjectFilter == "" {
		return a.teachingGroups
	}
	groups := make([]store.DataTeachingGroup, 0, len(a.teachingGroups))
	for _, group := range a.teachingGroups {
		if group.Subject == a.groupSubjectFilter {
			groups = append(groups, group)
		}
	}
	return groups
}

func (a *app) groupTableWith(c *ui.Context, p uiPalette, label string, groups []store.DataTeachingGroup) {
	t := p.theme
	a.groupTable.RowHeight = t.Space(10)
	columns := []ui.TableColumn{
		{Title: locales.GroupSubject(), Width: t.Space(18)},
		{Title: locales.GroupName(), Width: t.Space(20)},
		{Title: locales.GroupClasses(), Width: 0},
		{Title: locales.GroupTeachers(), Width: 0},
		{Title: locales.GroupStudents(), Width: t.Space(18), Align: ui.End},
		{Title: locales.ColumnSource(), Width: t.Space(28)},
	}
	ui.Table(c, &a.groupTable, columns, len(groups), func(row, col int) {
		group := groups[row]
		switch col {
		case 0:
			ui.Text(c, a.subjectName(group.Subject, group.Subject)).SingleLine()
		case 1:
			ui.Text(c, group.Name).SingleLine()
		case 2:
			ui.Text(c, group.ClassNames).TextColor(p.muted).SingleLine()
		case 3:
			ui.Text(c, group.Teachers).TextColor(p.muted).SingleLine()
		case 4:
			ui.Text(c, formatInt(group.Students)).FontFeatures("tnum")
		case 5:
			ui.Text(c, dataSourceLabel(group.ImportRunID, group.SourceSheet, group.SourceRow)).
				TextColor(p.muted).SingleLine()
		}
	}).Label(label).Grow(1).FillWidth()
}

func (a *app) baselinePage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavBaseline(), locales.BaselinePageSubtitle(), func() {
			button := labelledButton(c, locales.ImportBaselines(), iconImports, false)
			button.Disabled(a.dataBusy || !a.stageEditable())
			if button.Clicked() {
				a.openImportWizard(importer.KindBaselines)
			}
		})
		if !a.dataStageReady() {
			a.noStageState(c, p, iconBaseline)
			return
		}
		if !a.stageEditable() {
			notice(c, p, noticeInfo, locales.StageReadOnly())
		}
		if a.dataError != "" {
			notice(c, p, noticeDanger, a.dataError)
		}
		if a.dataBusy {
			notice(c, p, noticeInfo, locales.ImportBusy())
		}
		if a.dataMessage != "" {
			notice(c, p, noticeSuccess, a.dataMessage)
		}
		a.baselineFilters(c, p)
		if len(a.baselines) == 0 {
			emptyState(c, p, iconBaseline, locales.BaselineEmpty(), locales.BaselineEmptyHint(), nil)
			return
		}
		a.baselineTable.RowHeight = t.Space(10)
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
				ui.Text(c, item.ClassName).SingleLine()
			case 1:
				ui.Text(c, a.subjectName(item.Subject, item.Subject)).SingleLine()
			case 2:
				ui.Text(c, formatDecimal(item.Average, 2)).FontFeatures("tnum")
			case 3:
				ui.Text(c, formatInt(item.Count)).FontFeatures("tnum")
			case 4:
				ui.Text(c, formatRatio(item.Ratio)).FontFeatures("tnum")
			case 5:
				ui.Text(c, item.SetName).TextColor(p.muted).SingleLine()
			case 6:
				ui.Text(c, dataSourceLabel(item.ImportRunID, item.SourceSheet, item.SourceRow)).
					TextColor(p.muted).SingleLine()
			}
		}).Label(locales.NavBaseline()).Grow(1).FillWidth()
	})
}

func (a *app) baselineFilters(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
		ui.Column(c).Width(t.Space(56)).Children(func() {
			nextQuery := a.baselineQuery
			searchField(c, &nextQuery, locales.DataSearchBaseline(), locales.DataSearchBaseline())
			if nextQuery != a.baselineQuery {
				a.baselineQuery = nextQuery
				a.reloadDataPages(context.Background())
			}
		})
		labels, ids := a.classFilterLabels()
		chosen := labels[0]
		for index, id := range ids {
			if id == a.baselineClassFilter {
				chosen = labels[index]
				break
			}
		}
		next := chosen
		selectField(c, "baseline-class", &next, labels, locales.DataFilterClass()).Width(t.Space(40))
		if next != chosen {
			for index, label := range labels {
				if label == next {
					a.baselineClassFilter = ids[index]
					a.reloadDataPages(context.Background())
					break
				}
			}
		}
		ui.Spacer(c)
		ui.Textf(c, locales.DataRecordCount(), formatInt(len(a.baselines))).FontSize(captionSize(t)).
			TextColor(p.muted).FontFeatures("tnum")
	})
}

func (a *app) importsPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavImports(), locales.ImportsPageSubtitle(), nil)
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
		a.importFilters(c, p)
		runs := a.visibleImportRuns()
		if len(runs) == 0 {
			emptyState(c, p, iconImports, locales.ImportsEmpty(), locales.ImportsEmptyHint(), nil)
			return
		}
		ui.Row(c).Grow(1).Gap(t.Space(3)).Children(func() {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).FillWidth().Gap(0).Dividers(1, p.border).Children(func() {
					for _, run := range runs {
						run := run
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).
							Padding(t.Space(3), 0).Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
								ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
									ui.Text(c, importFileLabel(run)).FontSize(bodySize(t)).
										FontWeight(500).SingleLine().Shrink(1).MinWidth(0)
									badge(c, p, importKindName(run.DataKind), toneNeutral)
									if run.Status == "undone" {
										badge(c, p, locales.ImportStatusUndone(), toneNeutral)
									}
								})
								ui.Textf(c, locales.ImportRunMeta(), formatTime(run.CreatedAt),
									formatInt(run.RowCount)).FontSize(captionSize(t)).
									TextColor(p.muted).FontFeatures("tnum").SingleLine()
							})
							if run.Warnings > 0 {
								ui.Textf(c, locales.ImportWarningCount(), formatInt(run.Warnings)).
									FontSize(captionSize(t)).TextColor(p.warning).Shrink(0)
							}
							detailButton := plainButton(c, locales.ImportViewDetail()).
								Label(locales.ImportViewDetail() + " " + importKindName(run.DataKind))
							if detailButton.Clicked() {
								a.selectImportRun(run.ID)
							}
						})
					}
				})
			})
			if a.importDetail != nil {
				ui.Column(c).Width(t.Space(84)).Children(func() {
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
	canUndo := false
	reason := ""
	if a.store != nil && detail.Run.ScopeType == "stage" {
		value, why, err := a.store.CanUndoImport(context.Background(), detail.Run.ID)
		if err != nil {
			a.dataError = err.Error()
		} else {
			canUndo = value
			reason = why
		}
	}
	listPanel(c, p, locales.ImportDetailTitle(), len(detail.Issues), func() {
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				a.detailRow(c, p, locales.ImportDetailKind(), importKindName(detail.Run.DataKind))
				a.detailRow(c, p, locales.ImportDetailFile(), importFileLabel(detail.Run))
				a.detailRow(c, p, locales.ImportDetailStatus(),
					importStatusName(detail.Run.Status))
				a.detailRow(c, p, locales.ImportDetailRows(), formatInt(detail.Run.RowCount))
				if detail.Run.RelativePath != "" {
					a.detailRow(c, p, locales.ImportSourcePath(), detail.Run.RelativePath)
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
				button := labelledButton(c, locales.ImportUndo(), iconAlert, false)
				button.Disabled(!canUndo)
				if button.Clicked() {
					a.undoSelectedImport()
				}
				if !canUndo {
					text := locales.ImportUndoNotImported()
					if reason == "newer_import" {
						text = locales.ImportUndoBlocked()
					} else if reason == "no_changes" {
						text = locales.ImportUndoNoChanges()
					} else if detail.Run.ScopeType == "exam" {
						text = locales.ImportUndoExam()
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

func importStatusName(status string) string {
	switch status {
	case "undone":
		return locales.ImportStatusUndone()
	default:
		return locales.ImportStatusImported()
	}
}

func (a *app) importFilters(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).FillWidth().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
		ui.Column(c).Width(t.Space(56)).Children(func() {
			nextQuery := a.importQuery
			searchField(c, &nextQuery, locales.DataSearchImports(), locales.DataSearchImports())
			if nextQuery != a.importQuery {
				a.importQuery = nextQuery
			}
		})
		ui.Spacer(c)
		ui.Textf(c, locales.DataRecordCount(), formatInt(len(a.visibleImportRuns()))).
			FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
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

func dataSourceLabel(runID int64, sheet string, row int) string {
	if runID == 0 {
		return locales.StudentSourceManual()
	}
	if sheet == "" {
		return fmt.Sprintf(locales.StudentSourceRun(), runID)
	}
	return fmt.Sprintf(locales.StudentSourceSheet(), sheet, row)
}

func importFileLabel(run store.ImportRun) string {
	if run.RelativePath != "" {
		parts := strings.Split(strings.ReplaceAll(run.RelativePath, "\\", "/"), "/")
		return parts[len(parts)-1]
	}
	if run.StoredPath != "" {
		parts := strings.Split(strings.ReplaceAll(run.StoredPath, "\\", "/"), "/")
		if base := parts[len(parts)-1]; !hashFileName(base) {
			return base
		}
	}
	// 归档文件按内容哈希命名，显示它对读者没有意义，改用工作表名。
	for _, sheet := range run.Sheets {
		if sheet.Name != "" {
			return sheet.Name
		}
	}
	return importKindName(run.DataKind)
}

// hashFileName 判断文件名是否只是 64 位内容哈希，例如归档后的导入文件。
func hashFileName(name string) bool {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if len(base) != 64 {
		return false
	}
	for _, char := range base {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}
