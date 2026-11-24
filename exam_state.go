package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tally/internal/store"
	"tally/locales"
)

func (a *app) reloadExams(ctx context.Context) {
	if a.store == nil {
		return
	}
	exams, err := a.store.ListExams(ctx)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.exams = exams
	if a.selectedExamID != 0 {
		a.reloadExamData(ctx)
	}
}

func (a *app) selectedExam() *store.Exam {
	if a.selectedExamID == 0 {
		return nil
	}
	return &a.exam
}

func (a *app) selectExam(id int64) {
	a.selectedExamID = id
	a.examCohortID = 0
	a.examTab = 0
	a.examRosterQuery = ""
	a.examRosterPage = 0
	a.examScoreQuery = ""
	a.examScoreSubject = ""
	a.examScoreClass = 0
	a.examScorePage = 0
	a.reloadExamData(context.Background())
}

func (a *app) clearExamLoadedData() {
	a.examRoster = nil
	a.examRosterTotal = 0
	a.examScores = nil
	a.examScoresTotal = 0
	a.examScoreSummaries = nil
	a.examScoreBands = nil
	a.examClassAverages = nil
	a.examCutoffImpacts = nil
	a.examSubjectCutoffs = nil
	a.examTierCutoffs = nil
	a.examTargets = nil
	a.examStatGroups = nil
	a.examQuality = nil
	a.examPrep = store.ExamPrepStatus{}
}

func (a *app) reloadExamData(ctx context.Context) {
	if a.store == nil || a.selectedExamID == 0 {
		return
	}
	a.qualityLoaded = false
	exam, err := a.store.GetExam(ctx, a.selectedExamID)
	if err != nil {
		a.clearExamLoadedData()
		a.examError = err.Error()
		return
	}
	a.exam = exam
	if len(exam.Cohorts) == 0 {
		a.examCohort = store.ExamCohort{}
		a.clearExamLoadedData()
		return
	}
	found := false
	for _, cohort := range exam.Cohorts {
		if cohort.CohortID == a.examCohortID {
			found = true
			break
		}
	}
	if !found {
		a.examCohortID = exam.Cohorts[0].CohortID
	}
	for _, cohort := range exam.Cohorts {
		if cohort.CohortID == a.examCohortID {
			a.examCohort = cohort
			break
		}
	}
	assessment, err := a.store.AssessmentForExamCohort(ctx, a.examCohort.ID)
	if err != nil {
		a.clearExamLoadedData()
		a.examError = err.Error()
		return
	}
	a.examAssessment = assessment
	tiers, err := a.store.AssessmentTierKeys(ctx, assessment.ID)
	if err != nil {
		a.clearExamLoadedData()
		a.examError = err.Error()
		return
	}
	a.examTierKeys = tiers
	a.examClasses, err = a.store.ListClasses(ctx, a.examCohort.CohortID, a.examCohort.StageID)
	if err != nil {
		a.clearExamLoadedData()
		a.examError = err.Error()
		return
	}
	groups, err := a.store.ListTeachingGroups(ctx, a.examCohort.StageID, "")
	if err != nil {
		a.clearExamLoadedData()
		a.examError = err.Error()
		return
	}
	a.examTeachingGroups = groups
	rosterPageSize := normalizePageSize(a.examRosterPageSize)
	a.examRosterPageSize = rosterPageSize
	if a.examRosterPage < 0 {
		a.examRosterPage = 0
	}
	a.examRoster, a.examRosterTotal, err = a.store.ListExamRoster(ctx, store.ExamRosterFilter{
		ExamID: a.exam.ID, CohortID: a.examCohort.CohortID, Query: a.examRosterQuery,
		Page: a.examRosterPage, PageSize: rosterPageSize,
	})
	if err != nil {
		a.clearExamLoadedData()
		a.examError = err.Error()
		return
	}
	if a.examRosterTotal > 0 && len(a.examRoster) == 0 && a.examRosterPage > 0 {
		a.examRosterPage = (a.examRosterTotal - 1) / rosterPageSize
		a.examRoster, a.examRosterTotal, err = a.store.ListExamRoster(ctx, store.ExamRosterFilter{
			ExamID: a.exam.ID, CohortID: a.examCohort.CohortID, Query: a.examRosterQuery,
			Page: a.examRosterPage, PageSize: rosterPageSize,
		})
		if err != nil {
			a.clearExamLoadedData()
			a.examError = err.Error()
			return
		}
	}
	scorePageSize := normalizePageSize(a.examScorePageSize)
	a.examScorePageSize = scorePageSize
	if a.examScorePage < 0 {
		a.examScorePage = 0
	}
	a.examScores, a.examScoresTotal, err = a.store.ListExamScores(ctx, store.ExamScoreFilter{
		ExamID: a.exam.ID, CohortID: a.examCohort.CohortID, Query: a.examScoreQuery,
		Subject: a.examScoreSubject, ClassID: a.examScoreClass,
		Page: a.examScorePage, PageSize: scorePageSize,
	})
	if err != nil {
		a.clearExamLoadedData()
		a.examError = err.Error()
		return
	}
	if a.examScoresTotal > 0 && len(a.examScores) == 0 && a.examScorePage > 0 {
		a.examScorePage = (a.examScoresTotal - 1) / scorePageSize
		a.examScores, a.examScoresTotal, err = a.store.ListExamScores(ctx, store.ExamScoreFilter{
			ExamID: a.exam.ID, CohortID: a.examCohort.CohortID, Query: a.examScoreQuery,
			Subject: a.examScoreSubject, ClassID: a.examScoreClass,
			Page: a.examScorePage, PageSize: scorePageSize,
		})
		if err != nil {
			a.clearExamLoadedData()
			a.examError = err.Error()
			return
		}
	}
	a.examScoreSummaries, err = a.store.ExamScoreSummaries(ctx, a.exam.ID, a.examCohort.CohortID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examScoreBands, err = a.store.ExamScoreBands(ctx, a.exam.ID, a.examCohort.CohortID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examClassAverages, err = a.store.ExamClassAverages(ctx, a.exam.ID, a.examCohort.CohortID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examCutoffImpacts, err = a.store.ExamCutoffImpacts(ctx, a.exam.ID, a.examCohort.CohortID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examSubjectCutoffs, err = a.store.ListExamSubjectCutoffs(ctx, a.exam.ID, a.examCohort.CohortID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examTierCutoffs, err = a.store.ListExamTierCutoffs(ctx, a.exam.ID, a.examCohort.CohortID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examTargets, err = a.store.ListClassTargets(ctx, assessment.ID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examStatGroups, err = a.store.ListExamStatGroups(ctx, assessment.ID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examQuality, err = a.store.ExamQualityIssues(ctx, a.exam.ID, a.examCohort.CohortID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examPrep, _, err = a.store.ExamPrep(ctx, a.exam.ID, a.examCohort.CohortID)
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examTargetDrafts = map[string]*float64{}
	existing := map[string]float64{}
	for _, target := range a.examTargets {
		existing[examTargetKey(target.ClassID, target.TierKey)] = target.Target
	}
	for _, class := range a.examClasses {
		for _, tier := range a.examTierKeys {
			value := existing[examTargetKey(class.ID, tier)]
			copied := value
			a.examTargetDrafts[examTargetKey(class.ID, tier)] = &copied
		}
	}
	if a.examDefaultTrack == "" {
		for _, track := range a.tracks {
			if track.Enabled {
				a.examDefaultTrack = track.Key
				break
			}
		}
	}
	a.examError = ""
}

func examTargetKey(classID int64, tier string) string {
	return fmt.Sprintf("%d\x00%s", classID, tier)
}

func (a *app) openNewExam() {
	a.newExamOpen = true
	a.newExamYear = float64(a.academicYearStart)
	a.newExamTerm = 0
	a.newExamType = 0
	a.newExamName = ""
	now := a.nowValue()
	a.newExamDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	a.newExamCohorts = map[int64]bool{}
	a.newExamMarks = map[int64]map[string]float64{}
	for _, cohort := range a.cohorts {
		marks := map[string]float64{}
		for _, subject := range a.subjects {
			if subject.Enabled {
				marks[subject.Key] = subject.FullMark
			}
		}
		a.newExamMarks[cohort.ID] = marks
	}
	if cohort := a.selectedCohort(); cohort != nil {
		a.newExamCohorts[cohort.ID] = true
	}
}

func (a *app) createExam() {
	if a.store == nil {
		return
	}
	name := strings.TrimSpace(a.newExamName)
	if name == "" {
		a.examError = locales.ExamNameRequired()
		return
	}
	if msg := validateRuneLength(name, locales.ExamName(), maxNameRunes); msg != "" {
		a.examError = msg
		return
	}
	if a.newExamDate.IsZero() {
		a.examError = locales.ExamDateRequired()
		return
	}
	var cohortIDs []int64
	for id, selected := range a.newExamCohorts {
		if selected {
			cohortIDs = append(cohortIDs, id)
		}
	}
	if len(cohortIDs) == 0 {
		a.examError = locales.ExamCohortRequired()
		return
	}
	exam, err := a.store.CreateExam(context.Background(), store.CreateExamInput{
		AcademicYearStart: int(a.newExamYear),
		Term:              examTermCode(a.newExamTerm),
		ExamType:          examTypeCode(a.newExamType),
		Name:              name,
		ExamDate:          a.newExamDate.Format("2006-01-02"),
		CohortIDs:         cohortIDs,
		SubjectMarks:      a.newExamMarks,
	})
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.newExamOpen = false
	a.examError = ""
	a.examMessage = locales.ExamCreated()
	a.notifyMain(a.examMessage)
	a.reloadExams(context.Background())
	a.selectExam(exam.ID)
}

func examTermCode(index int) string {
	if index == 1 {
		return store.ExamTermSecond
	}
	return store.ExamTermFirst
}

func examTypeCode(index int) string {
	switch index {
	case 1:
		return store.ExamFinal
	case 2:
		return store.ExamGaokao
	default:
		return store.ExamMidterm
	}
}

func (a *app) examTermName(code string) string {
	if code == store.ExamTermSecond {
		return locales.ExamTermSecond()
	}
	return locales.ExamTermFirst()
}

func (a *app) examTypeName(code string) string {
	switch code {
	case store.ExamFinal:
		return locales.ExamTypeFinal()
	case store.ExamGaokao:
		return locales.ExamTypeGaokao()
	default:
		return locales.ExamTypeMidterm()
	}
}

func (a *app) examStatusName(code string) string {
	switch code {
	case "imported":
		return locales.ExamStatusImported()
	default:
		return locales.ExamStatusDraft()
	}
}

func (a *app) tierName(key string) string {
	switch key {
	case "qingbei":
		return locales.TierQingbei()
	case "c9":
		return locales.TierC9()
	case "project985":
		return locales.TierProject985()
	case "project211":
		return locales.TierProject211()
	case "special_control":
		return locales.TierSpecialControl()
	case "undergraduate":
		return locales.TierUndergraduate()
	case "single_subject":
		return locales.TierSingleSubject()
	default:
		return key
	}
}

func (a *app) examCohortLabel(cohort store.ExamCohort) string {
	return fmt.Sprintf(locales.CohortContext(), cohort.CohortName,
		a.stageName(cohort.GradeLevelAtExam), a.classOfName(cohort.GraduationYear))
}

func (a *app) generateExamRoster() {
	if a.store == nil || a.selectedExamID == 0 {
		return
	}
	a.examBusy = true
	summary, err := a.store.GenerateExamRoster(context.Background(), a.exam.ID,
		a.examCohort.CohortID, a.examDefaultTrack)
	a.examBusy = false
	if err != nil {
		a.examError = err.Error()
		return
	}
	a.examError = ""
	a.examMessage = fmt.Sprintf(locales.ExamRosterGenerated(), summary.Inserted)
	a.notifyMain(a.examMessage)
	a.reloadExamData(context.Background())
}

func (a *app) saveExamTargets() {
	if a.store == nil || a.examAssessment.ID == 0 {
		return
	}
	for _, class := range a.examClasses {
		for _, tier := range a.examTierKeys {
			value := a.examTargetDrafts[examTargetKey(class.ID, tier)]
			if value == nil {
				continue
			}
			if err := a.store.SetClassTarget(context.Background(), a.examAssessment.ID,
				class.ID, tier, *value); err != nil {
				a.examError = err.Error()
				return
			}
		}
	}
	a.examError = ""
	a.examMessage = locales.ExamTargetsSaved()
	a.notifyMain(a.examMessage)
	a.reloadExamData(context.Background())
}

func (a *app) openStatGroupEditor(group *store.ExamStatGroup) {
	a.statGroupOpen = true
	a.statGroupError = ""
	a.statGroupMembers = map[int64]bool{}
	if group != nil {
		a.statGroupID = group.ID
		a.statGroupSubject = group.Subject
		a.statGroupName = group.Name
		a.statGroupLeadClass = group.LeadClassID
		if group.Target != nil {
			a.statGroupTarget = *group.Target
		}
		for _, member := range group.Members {
			a.statGroupMembers[member.TeachingGroupID] = true
		}
		return
	}
	a.statGroupID = 0
	a.statGroupSubject = ""
	for _, subject := range a.subjects {
		if subject.Enabled && subject.Key != "japanese" {
			a.statGroupSubject = subject.Key
			break
		}
	}
	a.statGroupName = ""
	a.statGroupLeadClass = 0
	if len(a.examClasses) > 0 {
		a.statGroupLeadClass = a.examClasses[0].ID
	}
	a.statGroupTarget = 0
}

func (a *app) saveStatGroup() {
	if a.store == nil {
		return
	}
	if msg := validateRuneLength(a.statGroupName, locales.ExamStatGroupName(), maxNameRunes); msg != "" {
		a.statGroupError = msg
		return
	}
	var members []int64
	for id, selected := range a.statGroupMembers {
		if selected {
			members = append(members, id)
		}
	}
	target := a.statGroupTarget
	_, err := a.store.SaveStatGroup(context.Background(), store.StatGroupInput{
		ID: a.statGroupID, AssessmentID: a.examAssessment.ID, Subject: a.statGroupSubject,
		Name: a.statGroupName, LeadClassID: a.statGroupLeadClass, Target: &target,
		MemberGroupIDs: members,
	})
	if err != nil {
		a.statGroupError = err.Error()
		return
	}
	a.statGroupOpen = false
	a.statGroupError = ""
	a.examMessage = locales.ExamStatGroupSaved()
	a.notifyMain(a.examMessage)
	a.reloadExamData(context.Background())
}

func (a *app) deleteStatGroup(id int64) {
	if a.store == nil {
		return
	}
	if err := a.store.DeleteStatGroup(context.Background(), a.examAssessment.ID, id); err != nil {
		a.examError = err.Error()
		return
	}
	a.examMessage = locales.ExamStatGroupDeleted()
	a.notifyMain(a.examMessage)
	a.reloadExamData(context.Background())
}

func (a *app) examClassLabels() ([]string, []int64) {
	labels := []string{locales.DataAllClasses()}
	ids := []int64{0}
	for _, class := range a.examClasses {
		labels = append(labels, class.Name)
		ids = append(ids, class.ID)
	}
	return labels, ids
}

func (a *app) examSubjectLabels() ([]string, []string) {
	labels := []string{locales.DataAllSubjects()}
	codes := []string{""}
	for _, subject := range a.subjects {
		if !subject.Enabled {
			continue
		}
		labels = append(labels, a.subjectName(subject.Key, subject.Name))
		codes = append(codes, subject.Key)
	}
	return labels, codes
}

func (a *app) examAcademicYearLabel() string {
	return a.academicYearLabel(a.exam.AcademicYearStart)
}

func (a *app) examCohortStageName() string {
	return a.stageName(a.examCohort.GradeLevelAtExam)
}

func (a *app) examBatchLabel(exam store.Exam) string {
	return fmt.Sprintf(locales.LabelQualifier(), exam.Name, a.examTypeName(exam.ExamType))
}
