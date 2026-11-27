package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"tally/internal/importer"
	"tally/internal/store"
	"tally/locales"
)

func (a *app) reloadDataPages(ctx context.Context) {
	a.classes = nil
	a.students = nil
	a.studentsTotal = 0
	a.teachers = nil
	a.teachingGroups = nil
	a.baselines = nil
	a.importRuns = nil
	a.studentDetailOpen = false
	if a.store == nil {
		return
	}
	var err error
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	if cohort != nil && stage != nil {
		a.importRuns, err = a.store.ListWorkspaceImportRuns(ctx, cohort.ID, stage.ID)
	} else {
		a.importRuns, err = a.store.ListAllImportRuns(ctx)
	}
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.refreshImportUndoStates(ctx)
	if cohort == nil || stage == nil {
		return
	}
	classes, err := a.store.ListClasses(ctx, cohort.ID, stage.ID)
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.classes = classes
	a.reloadStudents(ctx)
	teachers, err := a.store.ListTeachers(ctx, stage.ID, a.subjectQuery(a.teacherQuery))
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.teachers = teachers
	groups, err := a.store.ListTeachingGroups(ctx, stage.ID, a.subjectQuery(a.groupQuery))
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.teachingGroups = groups
	baselines, err := a.store.ListBaselines(ctx, stage.ID, a.baselineClassFilter,
		a.subjectQuery(a.baselineQuery))
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.baselines = baselines
}

func (a *app) reloadStudents(ctx context.Context) {
	stage := a.selectedStage()
	if stage == nil || a.store == nil {
		a.students = nil
		a.studentsTotal = 0
		return
	}
	pageSize := normalizePageSize(a.studentPageSize)
	a.studentPageSize = pageSize
	if a.studentPage < 0 {
		a.studentPage = 0
	}
	students, total, err := a.store.ListStudents(ctx, store.StudentFilter{
		StageID:  stage.ID,
		ClassID:  a.dataClassFilter,
		Track:    a.dataTrackFilter,
		Status:   a.dataStatusFilter,
		Query:    a.dataQuery,
		Page:     a.studentPage,
		PageSize: pageSize,
	})
	if err != nil {
		a.dataError = err.Error()
		a.students = nil
		a.studentsTotal = 0
		return
	}
	if total > 0 && len(students) == 0 && a.studentPage > 0 {
		a.studentPage = (total - 1) / pageSize
		students, total, err = a.store.ListStudents(ctx, store.StudentFilter{
			StageID:  stage.ID,
			ClassID:  a.dataClassFilter,
			Track:    a.dataTrackFilter,
			Status:   a.dataStatusFilter,
			Query:    a.dataQuery,
			Page:     a.studentPage,
			PageSize: pageSize,
		})
		if err != nil {
			a.dataError = err.Error()
			a.students = nil
			a.studentsTotal = 0
			return
		}
	}
	a.students = students
	a.studentsTotal = total
	if a.selectedStudentID == 0 {
		a.studentDetailOpen = false
	}
	if a.selectedStudentID != 0 {
		detail, err := a.store.GetStudent(ctx, stage.ID, a.selectedStudentID)
		if err == nil {
			a.studentDetail = detail
			a.studentDetailOpen = true
		}
	}
}

func (a *app) reloadSchoolSettings(ctx context.Context) {
	if a.store == nil {
		return
	}
	subjects, err := a.store.ListSubjects(ctx)
	if err != nil {
		a.settingsError = err.Error()
		return
	}
	a.subjects = subjects
	a.subjectDrafts = append([]store.Subject(nil), subjects...)
	tracks, err := a.store.ListTracks(ctx)
	if err != nil {
		a.settingsError = err.Error()
		return
	}
	a.tracks = tracks
	a.trackDrafts = append([]store.Track(nil), tracks...)
	backups, err := store.ListBackups(a.dataDir)
	if err != nil {
		a.settingsError = err.Error()
		return
	}
	a.backups = backups
}

func (a *app) chooseStudentImportFile() {
	a.chooseWorkbookImport(locales.ImportStudentsTitle(), a.importStudentFile)
}

func (a *app) chooseAssignmentImportFile() {
	a.chooseWorkbookImport(locales.ImportAssignmentsTitle(), a.importAssignmentFile)
}

func (a *app) chooseBaselineImportFile() {
	a.chooseWorkbookImport(locales.ImportBaselinesTitle(), a.importBaselineFile)
}

func (a *app) chooseWorkbookImport(title string, action func(string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:   title,
			Filters: []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx", "xls"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		if a.win == nil {
			action(paths[0])
			return
		}
		a.win.Update(func() {
			action(paths[0])
		})
	}()
}

func (a *app) importStudentFile(path string) {
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	if cohort == nil || stage == nil {
		a.dataError = locales.ImportNoStage()
		return
	}
	a.dataBusy = true
	a.dataError = ""
	a.dataMessage = ""
	work := func() (store.ImportSummary, error) {
		ctx := context.Background()
		data, report, err := importer.ImportWorkspace(path, importer.Options{
			ConversionDir: filepath.Join(a.dataDir, "converted"),
			SofficePath:   a.sofficePath,
		})
		if err != nil {
			return store.ImportSummary{}, err
		}
		stored, err := a.store.StoreImportFile(path, report.FileHash)
		if err != nil {
			return store.ImportSummary{}, err
		}
		report.StoredPath = stored
		return a.store.MergeStageRoster(ctx, cohort.ID, stage.ID, data, report)
	}
	finish := func(summary store.ImportSummary, err error) {
		a.dataBusy = false
		if err != nil {
			a.dataError = err.Error()
			return
		}
		a.dataError = ""
		a.dataMessage = fmt.Sprintf(locales.ImportStudentsDone(), summary.Students, summary.Classes)
		a.notifyMain(a.dataMessage)
		ctx := context.Background()
		a.reloadCohorts(ctx)
		a.reloadDataPages(ctx)
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

func (a *app) importAssignmentFile(path string) {
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	if cohort == nil || stage == nil {
		a.dataError = locales.ImportNoStage()
		return
	}
	a.dataBusy = true
	a.dataError = ""
	a.dataMessage = ""
	work := func() (store.ImportSummary, error) {
		ctx := context.Background()
		data, report, err := importer.ImportWorkspace(path, importer.Options{
			ConversionDir: filepath.Join(a.dataDir, "converted"),
			SofficePath:   a.sofficePath,
		})
		if err != nil {
			return store.ImportSummary{}, err
		}
		stored, err := a.store.StoreImportFile(path, report.FileHash)
		if err != nil {
			return store.ImportSummary{}, err
		}
		report.StoredPath = stored
		return a.store.MergeStageAssignments(ctx, cohort.ID, stage.ID, data, report)
	}
	finish := func(summary store.ImportSummary, err error) {
		a.dataBusy = false
		if err != nil {
			a.dataError = err.Error()
			return
		}
		a.dataError = ""
		a.dataMessage = fmt.Sprintf(locales.ImportAssignmentsDone(), summary.Teachers,
			summary.TeachingGroups, summary.Assignments)
		a.notifyMain(a.dataMessage)
		ctx := context.Background()
		a.reloadCohorts(ctx)
		a.reloadDataPages(ctx)
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

func (a *app) importBaselineFile(path string) {
	cohort := a.selectedCohort()
	stage := a.selectedStage()
	if cohort == nil || stage == nil {
		a.dataError = locales.ImportNoStage()
		return
	}
	a.dataBusy = true
	a.dataError = ""
	a.dataMessage = ""
	work := func() (store.ImportSummary, error) {
		ctx := context.Background()
		data, report, err := importer.ImportWorkspace(path, importer.Options{
			ConversionDir: filepath.Join(a.dataDir, "converted"),
			SofficePath:   a.sofficePath,
		})
		if err != nil {
			return store.ImportSummary{}, err
		}
		stored, err := a.store.StoreImportFile(path, report.FileHash)
		if err != nil {
			return store.ImportSummary{}, err
		}
		report.StoredPath = stored
		return a.store.MergeStageBaselines(ctx, cohort.ID, stage.ID, data, report)
	}
	finish := func(summary store.ImportSummary, err error) {
		a.dataBusy = false
		if err != nil {
			a.dataError = err.Error()
			return
		}
		a.dataError = ""
		a.dataMessage = fmt.Sprintf(locales.ImportBaselinesDone(), summary.Baselines)
		a.notifyMain(a.dataMessage)
		ctx := context.Background()
		a.reloadCohorts(ctx)
		a.reloadDataPages(ctx)
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

func (a *app) refreshImportUndoStates(ctx context.Context) {
	if a.store == nil {
		return
	}
	if a.importCanUndo == nil {
		a.importCanUndo = map[int64]bool{}
	}
	if a.importUndoReason == nil {
		a.importUndoReason = map[int64]string{}
	}
	clear(a.importCanUndo)
	clear(a.importUndoReason)
	for _, run := range a.importRuns {
		canUndo, why, err := a.store.CanUndoImport(ctx, run.ID)
		if err == nil {
			a.importCanUndo[run.ID] = canUndo
			a.importUndoReason[run.ID] = why
		}
	}
	if a.importDetail != nil {
		id := a.importDetail.Run.ID
		if _, seen := a.importCanUndo[id]; !seen {
			canUndo, why, err := a.store.CanUndoImport(ctx, id)
			if err == nil {
				a.importCanUndo[id] = canUndo
				a.importUndoReason[id] = why
			}
		}
	}
}

func (a *app) selectImportRun(id int64) {
	if a.store == nil {
		return
	}
	ctx := context.Background()
	detail, err := a.store.GetImportRun(ctx, id)
	if err != nil {
		a.dataError = err.Error()
		return
	}
	a.importRunID = id
	a.importDetail = &detail
	if a.importCanUndo == nil {
		a.importCanUndo = map[int64]bool{}
	}
	if a.importUndoReason == nil {
		a.importUndoReason = map[int64]string{}
	}
	canUndo, why, uErr := a.store.CanUndoImport(ctx, id)
	if uErr == nil {
		a.importCanUndo[id] = canUndo
		a.importUndoReason[id] = why
	}
	a.dataError = ""
}

func (a *app) undoSelectedImport() {
	if a.store == nil || a.importRunID == 0 {
		return
	}
	if err := a.store.UndoImportRun(context.Background(), a.importRunID); err != nil {
		if errors.Is(err, store.ErrImportUndoBlocked) {
			a.dataError = locales.ImportUndoBlocked()
			return
		}
		a.dataError = err.Error()
		return
	}
	a.dataMessage = locales.ImportUndoDone()
	a.notifyMain(locales.ImportCenterUndoSuccess())
	a.importRunID = 0
	a.importDetail = nil
	ctx := context.Background()
	a.reloadCohorts(ctx)
	a.reloadDataPages(ctx)
	a.reloadExams(ctx)
	a.reloadAssessments(ctx)
	a.qualityLoaded = false
	a.reloadWorkspaceSummary(ctx)
}

func (a *app) classFilterLabels() (labels []string, ids []int64) {
	labels = append(labels, locales.DataAllClasses())
	ids = append(ids, 0)
	for _, class := range a.classes {
		labels = append(labels, class.Name)
		ids = append(ids, class.ID)
	}
	return labels, ids
}

func (a *app) trackLabels() []string {
	labels := []string{locales.DataAllTracks()}
	for _, track := range a.tracks {
		labels = append(labels, a.trackName(track.Key, track.Name))
	}
	return labels
}

func (a *app) trackName(key, fallback string) string {
	switch key {
	case "physics":
		return locales.TrackPhysics()
	case "history":
		return locales.TrackHistory()
	default:
		if fallback != "" {
			return fallback
		}
		return key
	}
}

func (a *app) statusLabels() []string {
	return []string{
		locales.DataAllStatuses(),
		locales.StudentStatusActive(),
		locales.StudentStatusTransferred(),
		locales.StudentStatusSuspended(),
		locales.StudentStatusBorrowed(),
		locales.StudentStatusLeft(),
	}
}

func (a *app) statusName(status string) string {
	switch status {
	case "transferred":
		return locales.StudentStatusTransferred()
	case "suspended":
		return locales.StudentStatusSuspended()
	case "borrowed":
		return locales.StudentStatusBorrowed()
	case "left_school":
		return locales.StudentStatusLeft()
	case "graduated":
		return locales.StudentStatusGraduated()
	default:
		return locales.StudentStatusActive()
	}
}

func (a *app) subjectName(key, fallback string) string {
	for _, subject := range a.subjects {
		if subject.Key == key && subject.Name != "" {
			return subject.Name
		}
	}
	if fallback != "" && fallback != key {
		return fallback
	}
	if label := subjectLabel(key); label != key {
		return label
	}
	return key
}

func (a *app) subjectQuery(query string) string {
	text := strings.TrimSpace(query)
	if text == "" {
		return query
	}
	for _, subject := range a.subjects {
		if a.subjectName(subject.Key, subject.Name) == text {
			return subject.Key
		}
	}
	return query
}
