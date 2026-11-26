package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"

	"tally/internal/exporter"
	"tally/internal/store"
	"tally/locales"
)

type analysisViewFilters struct {
	Tab         int    `json:"tab"`
	TeacherName string `json:"teacherName,omitempty"`
	ClassID     int64  `json:"classId,omitempty"`
	Subject     string `json:"subject,omitempty"`
	StudentID   int64  `json:"studentId,omitempty"`
}

func (a *app) ensureAnalysisCohort(ctx context.Context) *store.Cohort {
	if cohort := a.selectedCohort(); cohort != nil {
		return cohort
	}
	if len(a.cohorts) == 0 {
		return nil
	}
	targetID := a.cohorts[0].ID
	for _, co := range a.cohorts {
		if co.CurrentPhase != store.PhaseGraduated {
			targetID = co.ID
			break
		}
	}
	a.switchWorkspaceCohort(targetID)
	return a.selectedCohort()
}

func (a *app) reloadAnalysis(ctx context.Context) {
	if a.store == nil {
		return
	}
	a.analysisError = ""
	a.loadSavedViews(ctx)

	cohort := a.ensureAnalysisCohort(ctx)
	if cohort == nil {
		return
	}

	switch a.analysisTabIndex {
	case 0:
		a.queryTeacherTrend(ctx)
	case 1:
		a.queryClassTrend(ctx)
	case 2:
		a.querySubjectComparison(ctx)
	case 3:
		a.queryStudentTracking(ctx)
	case 4:
		a.queryAssessmentComparison(ctx)
	}
}

func (a *app) loadSavedViews(ctx context.Context) {
	if a.store == nil {
		return
	}
	views, err := a.store.ListViews(ctx, "analysis")
	if err != nil {
		a.analysisError = err.Error()
		return
	}
	a.analysisSavedViews = views
}

func (a *app) queryTeacherTrend(ctx context.Context) {
	cohort := a.selectedCohort()
	if cohort == nil || a.analysisTeacherName == "" {
		a.analysisTeacherResult = store.TeacherTrendResult{TeacherName: a.analysisTeacherName}
		return
	}
	res, err := a.store.QueryTeacherTrends(ctx, cohort.ID, a.analysisTeacherName, nil)
	if err != nil {
		a.analysisTeacherResult = store.TeacherTrendResult{}
		a.analysisError = err.Error()
		return
	}
	a.analysisTeacherResult = res
}

func (a *app) queryClassTrend(ctx context.Context) {
	cohort := a.selectedCohort()
	if cohort == nil || a.analysisClassID <= 0 {
		a.analysisClassResult = store.ClassTrendResult{ClassID: a.analysisClassID}
		return
	}
	res, err := a.store.QueryClassTrends(ctx, cohort.ID, a.analysisClassID, "", nil)
	if err != nil {
		a.analysisClassResult = store.ClassTrendResult{}
		a.analysisError = err.Error()
		return
	}
	a.analysisClassResult = res
}

func (a *app) querySubjectComparison(ctx context.Context) {
	if a.selectedAssessmentID <= 0 {
		a.analysisSubjectResult = store.SubjectComparisonResult{Subject: a.analysisSubject}
		return
	}
	res, err := a.store.QuerySubjectComparison(ctx, a.selectedAssessmentID, a.analysisSubject)
	if err != nil {
		a.analysisSubjectResult = store.SubjectComparisonResult{}
		a.analysisError = err.Error()
		return
	}
	a.analysisSubjectResult = res
}

func (a *app) queryStudentTracking(ctx context.Context) {
	cohort := a.selectedCohort()
	if cohort == nil || a.analysisStudentID <= 0 {
		a.analysisStudentResult = store.StudentTrackResult{StudentID: a.analysisStudentID}
		return
	}
	res, err := a.store.QueryStudentTracking(ctx, cohort.ID, a.analysisStudentID, "")
	if err != nil {
		a.analysisStudentResult = store.StudentTrackResult{}
		a.analysisError = err.Error()
		return
	}
	a.analysisStudentResult = res
}

func (a *app) queryAssessmentComparison(ctx context.Context) {
	ids := make([]int64, 0, len(a.analysisComparisonAssessmentIDs))
	for id, checked := range a.analysisComparisonAssessmentIDs {
		if checked {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		// 默认挑选最近 2 个考核
		for i := 0; i < len(a.assessments) && len(ids) < 2; i++ {
			ids = append(ids, a.assessments[i].AssessmentID)
			if a.analysisComparisonAssessmentIDs == nil {
				a.analysisComparisonAssessmentIDs = map[int64]bool{}
			}
			a.analysisComparisonAssessmentIDs[a.assessments[i].AssessmentID] = true
		}
	}
	if len(ids) == 0 {
		a.analysisComparisonResult = store.AssessmentComparisonResult{}
		return
	}
	res, err := a.store.QueryAssessmentComparison(ctx, ids)
	if err != nil {
		a.analysisComparisonResult = store.AssessmentComparisonResult{}
		a.analysisError = err.Error()
		return
	}
	a.analysisComparisonResult = res
}

func (a *app) saveCurrentView(ctx context.Context, name string) {
	if a.store == nil || name == "" {
		return
	}
	filters := analysisViewFilters{
		Tab:         a.analysisTabIndex,
		TeacherName: a.analysisTeacherName,
		ClassID:     a.analysisClassID,
		Subject:     a.analysisSubject,
		StudentID:   a.analysisStudentID,
	}
	raw, err := json.Marshal(filters)
	if err != nil {
		a.analysisError = err.Error()
		return
	}
	id, err := a.store.SaveView(ctx, store.SavedView{
		Page:        "analysis",
		Name:        name,
		FiltersJSON: string(raw),
	})
	if err != nil {
		a.analysisError = err.Error()
		return
	}
	a.analysisSelectedViewID = id
	a.analysisMessage = locales.AnalysisViewSaved()
	a.notifyMain(a.analysisMessage)
	a.loadSavedViews(ctx)
}

func (a *app) applySavedView(ctx context.Context, view store.SavedView) {
	if view.FiltersJSON == "" {
		return
	}
	var filters analysisViewFilters
	if err := json.Unmarshal([]byte(view.FiltersJSON), &filters); err != nil {
		return
	}
	a.analysisTabIndex = filters.Tab
	a.analysisTeacherName = filters.TeacherName
	a.analysisClassID = filters.ClassID
	a.analysisSubject = filters.Subject
	a.analysisStudentID = filters.StudentID
	a.reloadAnalysis(ctx)
}

func (a *app) deleteSavedView(ctx context.Context, id int64) {
	if a.store == nil || id <= 0 {
		return
	}
	if err := a.store.DeleteView(ctx, id); err != nil {
		a.analysisError = err.Error()
		return
	}
	a.analysisSelectedViewID = 0
	a.notifyMain(locales.AnalysisViewDeleted())
	a.loadSavedViews(ctx)
}

func (a *app) chooseAnalysisExport() {
	cohort := a.selectedCohort()
	cohortName := ""
	if cohort != nil {
		cohortName = a.cohortName(cohort.EnrollmentYear)
		if i18n.Locale() == "en" {
			cohortName += " "
		}
	}
	defaultName := locales.AnalysisReportDefaultFileName()
	switch a.analysisTabIndex {
	case 0:
		defaultName = fmt.Sprintf(locales.AnalysisExportTeacherTrend(), cohortName, a.analysisTeacherName)
	case 1:
		clsName := a.analysisClassResult.ClassName
		if clsName == "" {
			clsName = locales.ColumnClass()
		}
		defaultName = fmt.Sprintf(locales.AnalysisExportClassTrend(), cohortName, clsName)
	case 2:
		subjName := a.subjectName(a.analysisSubject, a.analysisSubject)
		if subjName == "" {
			subjName = locales.ColumnSubject()
		}
		defaultName = fmt.Sprintf(locales.AnalysisExportSubjectComparison(), cohortName, subjName)
	case 3:
		stuName := a.analysisStudentResult.StudentName
		if stuName == "" {
			stuName = locales.NavStudents()
		}
		defaultName = fmt.Sprintf(locales.AnalysisExportStudentTracking(), cohortName, stuName)
	case 4:
		defaultName = fmt.Sprintf(locales.AnalysisExportAssessmentComparison(), cohortName)
	}

	defaultName = sanitizeExportFilename(defaultName, locales.AnalysisReportDefaultFileName())
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       locales.AnalysisExport(),
			DefaultPath: defaultName,
			Filters:     []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx"}}},
		})
		if err != nil || path == "" {
			return
		}
		if a.win == nil {
			a.performAnalysisExport(path)
			return
		}
		a.win.Update(func() {
			a.performAnalysisExport(path)
		})
	}()
}

func (a *app) performAnalysisExport(path string) {
	var err error
	switch a.analysisTabIndex {
	case 0:
		err = exporter.ExportTeacherTrend(path, a.analysisTeacherResult)
	case 1:
		err = exporter.ExportClassTrend(path, a.analysisClassResult)
	case 2:
		err = exporter.ExportSubjectComparison(path, a.analysisSubjectResult)
	case 3:
		err = exporter.ExportStudentTracking(path, a.analysisStudentResult)
	case 4:
		err = exporter.ExportAssessmentComparison(path, a.analysisComparisonResult)
	}
	if err != nil {
		a.analysisError = err.Error()
		return
	}
	a.analysisMessage = fmt.Sprintf(locales.AnalysisExportSuccess(), filepath.Base(path))
	a.notifyMain(a.analysisMessage)
}
