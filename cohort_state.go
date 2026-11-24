package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"tally/internal/school"
	"tally/internal/store"
	"tally/locales"
)

func (a *app) nowValue() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

func (a *app) reloadCohorts(ctx context.Context) {
	if a.store == nil {
		return
	}
	cohorts, err := a.store.ListCohorts(ctx)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.cohorts = cohorts
	for _, cohort := range cohorts {
		index := school.StageIndex(cohort.EnrollmentYear, a.academicYearStart)
		if index < school.StageFirst || index > school.StageThird {
			continue
		}
		_, err := a.store.GetCohortStage(ctx, cohort.ID, a.academicYearStart)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			a.loadError = err.Error()
			return
		}
		newStage := errors.Is(err, store.ErrNotFound)
		stage, err := a.store.EnsureCohortStage(ctx, cohort.ID, a.academicYearStart, school.StageKey(index))
		if err != nil {
			a.loadError = err.Error()
			return
		}
		if newStage {
			if _, err := a.store.CopyStageFromPrevious(ctx, cohort.ID, stage.ID); err != nil {
				a.loadError = err.Error()
				return
			}
		}
	}
	if a.selectedCohort() == nil {
		a.selectedCohortID = ""
		a.selectedStageID = 0
		for _, cohort := range cohorts {
			index := school.StageIndex(cohort.EnrollmentYear, a.academicYearStart)
			if index >= school.StageFirst && index <= school.StageThird {
				a.selectedCohortID = strconv.FormatInt(cohort.ID, 10)
				break
			}
		}
		if a.selectedCohortID == "" && len(cohorts) > 0 {
			a.selectedCohortID = strconv.FormatInt(cohorts[0].ID, 10)
		}
	}
	a.reloadSelectedCohort(ctx)
}

func (a *app) reloadSelectedCohort(ctx context.Context) {
	a.cohortStages = nil
	a.stageCounts = store.StageCounts{}
	a.stageClasses = nil
	cohort := a.selectedCohort()
	if cohort == nil || a.store == nil {
		return
	}
	stages, err := a.store.ListCohortStages(ctx, cohort.ID)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.cohortStages = stages
	if a.selectedStageID == 0 {
		index := school.StageIndex(cohort.EnrollmentYear, a.academicYearStart)
		if index >= school.StageFirst && index <= school.StageThird {
			if stage, err := a.store.GetCohortStage(ctx, cohort.ID, a.academicYearStart); err == nil {
				a.selectedStageID = stage.ID
			}
		}
	}
	stage := a.selectedStage()
	if stage == nil {
		return
	}
	counts, err := a.store.StageCounts(ctx, cohort.ID, stage.AcademicYearStart, stage.GradeLevel)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.stageCounts = counts
	classes, err := a.store.StageClassNames(ctx, cohort.ID, stage.AcademicYearStart, stage.GradeLevel)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.stageClasses = classes
	a.reloadDataPages(ctx)
}

func (a *app) selectedCohort() *store.Cohort {
	if a.selectedCohortID == "" {
		return nil
	}
	id, err := strconv.ParseInt(a.selectedCohortID, 10, 64)
	if err != nil {
		return nil
	}
	for index := range a.cohorts {
		if a.cohorts[index].ID == id {
			return &a.cohorts[index]
		}
	}
	return nil
}

func (a *app) selectedStage() *store.CohortStage {
	if a.selectedStageID == 0 {
		return nil
	}
	for index := range a.cohortStages {
		if a.cohortStages[index].ID == a.selectedStageID {
			return &a.cohortStages[index]
		}
	}
	return nil
}

func (a *app) stageEditable() bool {
	stage := a.selectedStage()
	return stage != nil && stage.AcademicYearStart == a.academicYearStart
}

// displayStage 返回顶栏和总览使用的阶段。没有手工选择历史阶段时按当前学年推导。
func (a *app) displayStage() store.CohortStage {
	if stage := a.selectedStage(); stage != nil {
		return *stage
	}
	cohort := a.selectedCohort()
	if cohort == nil {
		return store.CohortStage{}
	}
	index := school.StageIndex(cohort.EnrollmentYear, a.academicYearStart)
	return store.CohortStage{
		CohortID:          cohort.ID,
		AcademicYearStart: a.academicYearStart,
		GradeLevel:        school.StageKey(index),
	}
}

func (a *app) stageOptions() (labels []string, ids []int64) {
	for _, stage := range a.cohortStages {
		labels = append(labels, fmt.Sprintf(locales.StageOption(), a.stageName(stage.GradeLevel),
			school.AcademicYearLabel(stage.AcademicYearStart)))
		ids = append(ids, stage.ID)
	}
	return labels, ids
}

func (a *app) selectCohort(label string) {
	for _, cohort := range a.cohorts {
		if a.cohortName(cohort.EnrollmentYear) != label {
			continue
		}
		a.selectedCohortID = strconv.FormatInt(cohort.ID, 10)
		a.selectedStageID = 0
		a.reloadSelectedCohort(context.Background())
		return
	}
}

func (a *app) selectStage(label string) {
	labels, ids := a.stageOptions()
	for index, option := range labels {
		if option != label {
			continue
		}
		a.selectedStageID = ids[index]
		a.reloadSelectedCohort(context.Background())
		return
	}
}

func (a *app) setAcademicYear(start int) {
	if a.store == nil {
		return
	}
	if err := a.store.SetAcademicYear(context.Background(), start); err != nil {
		a.settingsError = err.Error()
		return
	}
	a.settingsError = ""
	a.academicYearStart = start
	a.selectedStageID = 0
	a.reloadCohorts(context.Background())
	a.notifySettings(locales.SettingsAcademicYearSaved())
}

func (a *app) openNewCohort() {
	a.newCohortOpen = true
	a.newCohortYear = float64(a.academicYearStart)
	a.newCohortError = ""
}

func (a *app) createCohort() {
	year := int(a.newCohortYear)
	cohort, err := a.store.CreateCohort(context.Background(), year, a.academicYearStart)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrCohortExists):
			a.newCohortError = fmt.Sprintf(locales.CohortExists(), year)
		case errors.Is(err, school.ErrEnrollmentYear):
			a.newCohortError = fmt.Sprintf(locales.CohortYearRange(), 1990, a.academicYearStart)
		default:
			a.newCohortError = err.Error()
		}
		return
	}
	a.newCohortOpen = false
	a.newCohortError = ""
	a.selectedCohortID = strconv.FormatInt(cohort.ID, 10)
	a.selectedStageID = 0
	a.reloadCohorts(context.Background())
	a.page = pageOverview
	a.notifyMain(fmt.Sprintf(locales.CohortCreatedToast(), a.cohortName(cohort.EnrollmentYear)))
}

func (a *app) cohortName(enrollmentYear int) string {
	return fmt.Sprintf(locales.CohortName(), enrollmentYear)
}

func (a *app) classOfName(graduationYear int) string {
	return fmt.Sprintf(locales.ClassOfName(), graduationYear)
}

func (a *app) academicYearLabel(start int) string {
	return fmt.Sprintf(locales.AcademicYearValue(), start, start+1)
}

func (a *app) stageName(gradeLevel string) string {
	index, err := school.StageIndexForKey(gradeLevel)
	if err != nil {
		return gradeLevel
	}
	return a.stageIndexName(index)
}

func (a *app) stageIndexName(index int) string {
	switch index {
	case school.StageFirst:
		return locales.StageG1()
	case school.StageSecond:
		return locales.StageG2()
	case school.StageThird:
		return locales.StageG3()
	case school.StageGraduated:
		return locales.StageGraduated()
	default:
		return locales.StageUpcoming()
	}
}

func (a *app) readiness() (ready, total int) {
	items := []bool{
		a.stageCounts.Classes > 0,
		a.stageCounts.Students > 0,
		a.stageCounts.Assignments > 0,
		a.stageCounts.Baselines > 0,
	}
	for _, item := range items {
		if item {
			ready++
		}
	}
	return ready, len(items)
}

func (a *app) nextSetupPage() pageID {
	switch {
	case a.stageCounts.Classes == 0:
		return pageCohorts
	case a.stageCounts.Students == 0:
		return pageStudents
	case a.stageCounts.Assignments == 0:
		return pageFaculty
	case a.stageCounts.Baselines == 0:
		return pageBaseline
	default:
		return pageAssessments
	}
}
