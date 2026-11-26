package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"

	"tally/internal/exporter"
	"tally/internal/store"
	"tally/locales"
)

type studentProfileNavEntry struct {
	CohortID  int64
	StudentID int64
	Name      string
}

// invalidateProfileCaches 清空学生与教师档案缓存，在数据变更后调用以保证下次读取最新数据。
func (a *app) invalidateProfileCaches() {
	a.studentProfileCache = nil
	a.teacherProfileCache = nil
}

// openStudentProfile 异步加载并打开指定学生的三年完整档案页，优先命中本地缓存。
func (a *app) openStudentProfile(studentID int64) {
	a.openStudentProfileWithHistory(studentID, false)
}

func (a *app) openStudentProfilePrevious(prevCohortID, prevStudentID int64) {
	if prevStudentID <= 0 {
		return
	}
	if a.studentProfile.Student.ID > 0 {
		a.studentProfileStack = append(a.studentProfileStack, studentProfileNavEntry{
			CohortID:  a.studentProfile.Cohort.ID,
			StudentID: a.studentProfile.Student.ID,
			Name:      a.studentProfile.Student.Name,
		})
	}
	if prevCohortID > 0 && strconv.FormatInt(prevCohortID, 10) != a.selectedCohortID {
		a.selectedCohortID = strconv.FormatInt(prevCohortID, 10)
		a.selectedStageID = 0
		a.reloadSelectedCohort(context.Background())
	}
	a.openStudentProfileWithHistory(prevStudentID, true)
}

func (a *app) popStudentProfileStack(targetIdx int) {
	if targetIdx < 0 || targetIdx >= len(a.studentProfileStack) {
		a.studentProfileStack = nil
		a.studentProfileOpen = false
		return
	}
	entry := a.studentProfileStack[targetIdx]
	a.studentProfileStack = a.studentProfileStack[:targetIdx]
	if entry.CohortID > 0 && strconv.FormatInt(entry.CohortID, 10) != a.selectedCohortID {
		a.selectedCohortID = strconv.FormatInt(entry.CohortID, 10)
		a.selectedStageID = 0
		a.reloadSelectedCohort(context.Background())
	}
	a.openStudentProfileWithHistory(entry.StudentID, true)
}

func (a *app) closeStudentProfile() {
	if len(a.studentProfileStack) > 0 {
		a.popStudentProfileStack(len(a.studentProfileStack) - 1)
		return
	}
	a.studentProfileOpen = false
	a.studentProfileLoading = false
	a.studentProfileError = ""
	a.selectedStudentID = 0
	a.studentSelectedExamIdx = -1
	a.studentChartHover = -1
}

func (a *app) openStudentProfileWithHistory(studentID int64, keepStack bool) {
	if a.store == nil || studentID <= 0 {
		return
	}
	if !keepStack {
		a.studentProfileStack = nil
	}
	a.selectedStudentID = studentID
	a.studentSelectedExamIdx = -1
	a.studentChartHover = -1
	a.studentProfileError = ""
	a.dataError = ""

	if a.studentProfileCache != nil {
		if cached, ok := a.studentProfileCache[studentID]; ok {
			a.applyLoadedStudentProfile(cached)
			return
		}
	}

	a.studentProfile = store.StudentProfile{}
	a.studentProfileReqID++
	reqID := a.studentProfileReqID
	a.studentProfileOpen = true

	if a.win == nil {
		prof, err := a.store.GetStudentProfile(context.Background(), studentID)
		if err != nil {
			a.studentProfileLoading = false
			a.studentProfile = store.StudentProfile{}
			a.studentProfileError = err.Error()
			return
		}
		a.cacheStudentProfile(prof)
		a.applyLoadedStudentProfile(prof)
		return
	}

	a.studentProfileLoading = true
	a.win.Invalidate()
	go func(id int64, seq int64) {
		prof, err := a.store.GetStudentProfile(context.Background(), id)
		a.win.Update(func() {
			if seq != a.studentProfileReqID {
				return
			}
			a.studentProfileLoading = false
			if err != nil {
				a.studentProfile = store.StudentProfile{}
				a.studentProfileError = err.Error()
				return
			}
			a.cacheStudentProfile(prof)
			a.applyLoadedStudentProfile(prof)
		})
	}(studentID, reqID)
}

func (a *app) cacheStudentProfile(prof store.StudentProfile) {
	if prof.Student.ID <= 0 {
		return
	}
	if a.studentProfileCache == nil {
		a.studentProfileCache = map[int64]store.StudentProfile{}
	}
	a.studentProfileCache[prof.Student.ID] = prof
}

func (a *app) applyLoadedStudentProfile(prof store.StudentProfile) {
	a.studentProfile = prof
	a.studentProfileLoading = false
	a.studentProfileOpen = true
	a.studentSelectedExamIdx = -1
	a.studentChartHover = -1

	avail := a.studentProfileSubjects(prof)
	if len(avail) == 0 {
		a.studentChartSubjects = nil
		return
	}
	var kept []string
	availSet := map[string]bool{}
	for _, s := range avail {
		availSet[s] = true
	}
	for _, s := range a.studentChartSubjects {
		if availSet[s] {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		limit := min(len(avail), 3)
		kept = append(kept, avail[:limit]...)
	}
	a.studentChartSubjects = kept
}

func (a *app) studentProfileSubjects(prof store.StudentProfile) []string {
	seen := map[string]bool{}
	for _, es := range prof.EntryScores {
		if es.Subject != "" {
			seen[es.Subject] = true
		}
	}
	for _, ex := range prof.Exams {
		for subj := range ex.Scores {
			if subj != "" {
				seen[subj] = true
			}
		}
	}
	ordered := make([]string, 0, len(seen))
	for _, s := range a.subjects {
		if seen[s.Key] {
			ordered = append(ordered, s.Key)
			delete(seen, s.Key)
		}
	}
	var rest []string
	for k := range seen {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	return append(ordered, rest...)
}

func (a *app) openStudentEventModal() {
	st := a.studentProfile.Student
	a.studentEventClasses = nil
	if a.store != nil && st.ID > 0 {
		classes, err := a.store.ListClasses(context.Background(), st.CohortID, st.StageID)
		if err != nil {
			a.dataError = err.Error()
			return
		}
		a.studentEventClasses = classes
	}
	a.studentEventType = "class_changed"
	a.studentEventClassChoice = st.ClassName
	if a.studentEventClassChoice == "" {
		if len(a.studentEventClasses) > 0 {
			a.studentEventClassChoice = a.studentEventClasses[0].Name
		} else if len(a.classes) > 0 {
			a.studentEventClassChoice = a.classes[0].Name
		}
	}
	a.studentEventTrackChoice = st.Track
	if a.studentEventTrackChoice == "" {
		a.studentEventTrackChoice = "physics"
	}
	a.studentEventElective = st.Elective
	a.studentEventForeignLang = st.ForeignLang
	if a.studentEventForeignLang == "" {
		a.studentEventForeignLang = "english"
	}
	a.studentEventStatusChoice = st.Status
	if a.studentEventStatusChoice == "" {
		a.studentEventStatusChoice = "active"
	}
	a.studentEventNote = ""
	a.studentEventError = ""
	a.studentEventModalOpen = true
}

func (a *app) submitStudentEvent() {
	if a.store == nil || a.studentProfile.Student.ID <= 0 {
		return
	}
	st := a.studentProfile.Student
	input := store.AddStudentEventInput{
		EventType: a.studentEventType,
		Note:      strings.TrimSpace(a.studentEventNote),
	}
	switch a.studentEventType {
	case "class_changed":
		var targetClassID int64
		for _, cls := range a.studentEventClasses {
			if cls.Name == a.studentEventClassChoice {
				targetClassID = cls.ID
				break
			}
		}
		if targetClassID == 0 {
			for _, cls := range a.classes {
				if cls.Name == a.studentEventClassChoice {
					targetClassID = cls.ID
					break
				}
			}
		}
		if targetClassID == 0 {
			classes, err := a.store.ListClasses(context.Background(), st.CohortID, st.StageID)
			if err != nil {
				a.studentEventError = err.Error()
				a.studentEventModalOpen = true
				return
			}
			a.studentEventClasses = classes
			for _, cls := range classes {
				if cls.Name == a.studentEventClassChoice {
					targetClassID = cls.ID
					break
				}
			}
		}
		input.ToClassID = targetClassID
	case "track_changed":
		input.ToTrack = a.studentEventTrackChoice
		input.ToElective = strings.TrimSpace(a.studentEventElective)
		input.ToForeignLang = a.studentEventForeignLang
	case "status_changed":
		input.ToStatus = a.studentEventStatusChoice
	}

	if _, err := a.store.AddStudentEvent(context.Background(), st.ID, input); err != nil {
		a.studentEventError = err.Error()
		a.studentEventModalOpen = true
		return
	}
	a.studentEventModalOpen = false
	a.studentEventError = ""
	if a.studentProfileCache != nil {
		delete(a.studentProfileCache, st.ID)
	}
	a.reloadCohorts(context.Background())
	a.openStudentProfileWithHistory(st.ID, true)
	a.notifyMain(locales.StudentEventSavedToast())
}

func (a *app) studentProfileExportOptions() exporter.StudentProfileExportOptions {
	subjLabels := make(map[string]string, len(a.subjects))
	for _, s := range a.subjects {
		if s.Key != "" && s.Name != "" {
			subjLabels[s.Key] = s.Name
		}
	}
	trackLabels := make(map[string]string, len(a.tracks))
	for _, tr := range a.tracks {
		if tr.Key != "" && tr.Name != "" {
			trackLabels[tr.Key] = tr.Name
		}
	}
	return exporter.StudentProfileExportOptions{
		SubjectLabels: subjLabels,
		TrackLabels:   trackLabels,
	}
}

func (a *app) chooseStudentProfileExport() {
	prof := a.studentProfile
	if prof.Student.ID <= 0 {
		return
	}
	exportOpts := a.studentProfileExportOptions()
	cohortLabel := a.cohortName(prof.Cohort.EnrollmentYear)
	if i18n.Locale() == "en" {
		cohortLabel += " "
	}
	defaultName := sanitizeExportFilename(
		fmt.Sprintf(locales.StudentProfileExportFile(), cohortLabel, prof.Student.Name),
		"Student-Profile.xlsx",
	)
	if a.win == nil {
		path := filepath.Join(a.dataDir, defaultName)
		if err := exporter.ExportStudentProfileWithOptions(path, prof, exportOpts); err != nil {
			a.dataError = err.Error()
			return
		}
		a.notifyMain(fmt.Sprintf(locales.StudentProfileExportSavedToast(), path))
		return
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       locales.StudentProfileExportExcel(),
			DefaultPath: defaultName,
			Filters:     []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx"}}},
		})
		if err != nil || path == "" {
			return
		}
		err = exporter.ExportStudentProfileWithOptions(path, prof, exportOpts)
		a.win.Update(func() {
			if err != nil {
				a.dataError = err.Error()
				return
			}
			a.notifyMain(fmt.Sprintf(locales.StudentProfileExportSavedToast(), path))
		})
	}()
}

func (a *app) teacherProfileExportOptions() exporter.TeacherProfileExportOptions {
	subjLabels := make(map[string]string, len(a.subjects))
	for _, s := range a.subjects {
		if s.Key != "" && s.Name != "" {
			subjLabels[s.Key] = s.Name
		}
	}
	return exporter.TeacherProfileExportOptions{
		SubjectLabels: subjLabels,
	}
}

func (a *app) chooseTeacherProfileExport() {
	prof := a.teacherProfile
	if prof.Teacher.ID <= 0 {
		return
	}
	exportOpts := a.teacherProfileExportOptions()
	secondary := strings.TrimSpace(prof.Teacher.TeacherNo)
	if secondary == "" {
		subject := prof.Teacher.Subject
		if strings.TrimSpace(subject) == "" {
			subject = prof.Occupancy.PrimarySubject
		}
		secondary = strings.TrimSpace(a.subjectName(subject, ""))
	}
	if secondary == "" {
		secondary = "-"
	}
	defaultName := sanitizeExportFilename(
		fmt.Sprintf(locales.TeacherProfileExportFile(), prof.Teacher.Name, secondary),
		"Teacher-Profile.xlsx",
	)
	if a.win == nil {
		path := filepath.Join(a.dataDir, defaultName)
		if err := exporter.ExportTeacherProfileWithOptions(path, prof, exportOpts); err != nil {
			a.dataError = err.Error()
			return
		}
		a.notifyMain(fmt.Sprintf(locales.TeacherProfileExportSavedToast(), path))
		return
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       locales.TeacherProfileExportExcel(),
			DefaultPath: defaultName,
			Filters:     []mygo.FileFilter{{Name: "Excel", Extensions: []string{"xlsx"}}},
		})
		if err != nil || path == "" {
			return
		}
		err = exporter.ExportTeacherProfileWithOptions(path, prof, exportOpts)
		a.win.Update(func() {
			if err != nil {
				a.dataError = err.Error()
				return
			}
			a.notifyMain(fmt.Sprintf(locales.TeacherProfileExportSavedToast(), path))
		})
	}()
}

// openTeacherProfile 异步加载并打开指定教师的执教档案详情页，优先命中本地缓存。
func (a *app) openTeacherProfile(teacherID int64) {
	a.openTeacherProfileWithReturn(teacherID, 0)
}

func (a *app) openTeacherProfileFromCohort(teacherID, cohortID int64) {
	a.openTeacherProfileWithReturn(teacherID, cohortID)
	a.page = pageFaculty
}

func (a *app) openTeacherProfileWithReturn(teacherID, returnCohortID int64) {
	if a.store == nil || teacherID <= 0 {
		return
	}
	a.selectedTeacherID = teacherID
	a.teacherProfileReturnCohort = returnCohortID
	a.teacherChartHover = -1
	a.teacherProfileError = ""
	a.dataError = ""

	if a.teacherProfileCache != nil {
		if cached, ok := a.teacherProfileCache[teacherID]; ok {
			a.applyLoadedTeacherProfile(cached)
			return
		}
	}

	a.teacherProfile = store.TeacherProfile{}
	a.teacherProfileReqID++
	reqID := a.teacherProfileReqID
	a.teacherProfileOpen = true

	if a.win == nil {
		prof, err := a.store.GetTeacherProfile(context.Background(), teacherID)
		if err != nil {
			a.teacherProfileLoading = false
			a.teacherProfile = store.TeacherProfile{}
			a.teacherProfileError = err.Error()
			return
		}
		a.cacheTeacherProfile(prof)
		a.applyLoadedTeacherProfile(prof)
		return
	}

	a.teacherProfileLoading = true
	a.win.Invalidate()
	go func(id int64, seq int64) {
		prof, err := a.store.GetTeacherProfile(context.Background(), id)
		a.win.Update(func() {
			if seq != a.teacherProfileReqID {
				return
			}
			a.teacherProfileLoading = false
			if err != nil {
				a.teacherProfile = store.TeacherProfile{}
				a.teacherProfileError = err.Error()
				return
			}
			a.cacheTeacherProfile(prof)
			a.applyLoadedTeacherProfile(prof)
		})
	}(teacherID, reqID)
}

func (a *app) cacheTeacherProfile(prof store.TeacherProfile) {
	if prof.Teacher.ID <= 0 {
		return
	}
	if a.teacherProfileCache == nil {
		a.teacherProfileCache = map[int64]store.TeacherProfile{}
	}
	a.teacherProfileCache[prof.Teacher.ID] = prof
}

func (a *app) applyLoadedTeacherProfile(prof store.TeacherProfile) {
	a.teacherProfile = prof
	a.teacherProfileLoading = false
	a.teacherProfileOpen = true
	a.teacherChartHover = -1
	if a.teacherCohortExpanded == nil {
		a.teacherCohortExpanded = map[int64]bool{}
	}
	for _, ch := range prof.CohortHistories {
		if _, exists := a.teacherCohortExpanded[ch.CohortID]; !exists {
			a.teacherCohortExpanded[ch.CohortID] = ch.CurrentPhase != store.PhaseGraduated
		}
	}
}

func (a *app) closeTeacherProfile() {
	retCohort := a.teacherProfileReturnCohort
	a.teacherProfileOpen = false
	a.teacherProfileLoading = false
	a.teacherProfileError = ""
	a.selectedTeacherID = 0
	a.teacherProfileReturnCohort = 0
	a.teacherChartHover = -1
	if retCohort > 0 {
		a.openCohortDetail(retCohort)
		a.cohortDetailTab = 3
		a.page = pageCohorts
	}
}

func (a *app) openEditTeacherModal() {
	tc := a.teacherProfile.Teacher
	a.editTeacherNo = tc.TeacherNo
	a.editTeacherName = tc.Name
	a.editTeacherSubject = tc.Subject
	a.editTeacherNote = tc.Note
	a.editTeacherError = ""
	a.editTeacherOpen = true
}

func (a *app) submitEditTeacher() {
	if a.store == nil || a.teacherProfile.Teacher.ID <= 0 {
		return
	}
	tid := a.teacherProfile.Teacher.ID
	updated, err := a.store.UpdateTeacher(context.Background(), tid, store.ImportTeacher{
		TeacherNo: a.editTeacherNo,
		Name:      a.editTeacherName,
		Subject:   a.editTeacherSubject,
		Note:      a.editTeacherNote,
	})
	if err != nil {
		a.editTeacherError = err.Error()
		a.editTeacherOpen = true
		return
	}
	a.editTeacherOpen = false
	a.editTeacherError = ""
	if a.teacherProfileCache != nil {
		delete(a.teacherProfileCache, tid)
	}
	a.reloadTeacherDirectory(context.Background())
	a.reloadSchoolOverview(context.Background())
	a.openTeacherProfileWithReturn(tid, a.teacherProfileReturnCohort)
	a.notifyMain(fmt.Sprintf(locales.TeacherProfileUpdatedToast(), updated.Name))
}

func (a *app) toggleTeacherProfileStatus() {
	prof := a.teacherProfile
	if a.store == nil || prof.Teacher.ID <= 0 {
		return
	}
	nextStatus := "inactive"
	if prof.Teacher.Status == store.TeacherStatusInactive {
		nextStatus = "active"
	}
	if err := a.store.SetTeacherStatus(context.Background(), prof.Teacher.ID, nextStatus); err != nil {
		a.dataError = err.Error()
		return
	}
	if a.teacherProfileCache != nil {
		delete(a.teacherProfileCache, prof.Teacher.ID)
	}
	a.reloadTeacherDirectory(context.Background())
	a.reloadSchoolOverview(context.Background())
	a.openTeacherProfileWithReturn(prof.Teacher.ID, a.teacherProfileReturnCohort)
}

func (a *app) studentForeignLangName(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "japanese":
		return locales.SubjectJapanese()
	default:
		return locales.SubjectEnglish()
	}
}

func (a *app) studentEventTypeLabel(eventType string) (string, badgeTone) {
	switch eventType {
	case "enrolled":
		return locales.StudentEventTypeEnrolled(), toneAccent
	case "promoted":
		return locales.StudentEventTypePromoted(), toneSuccess
	case "class_changed":
		return locales.StudentEventTypeClassChanged(), toneWarning
	case "track_changed":
		return locales.StudentEventTypeTrackChanged(), toneAccent
	case "status_changed":
		return locales.StudentEventTypeStatusChanged(), toneDanger
	case "merged":
		return locales.StudentEventTypeMerged(), toneNeutral
	case "graduated":
		return locales.StudentEventTypeGraduated(), toneNeutral
	default:
		return eventType, toneNeutral
	}
}

func (a *app) formatStudentEventTransition(ev store.StudentEvent) string {
	var fromMap, toMap map[string]any
	if ev.FromValueJSON != "" && ev.FromValueJSON != "{}" {
		if err := json.Unmarshal([]byte(ev.FromValueJSON), &fromMap); err != nil {
			fromMap = nil
		}
	}
	if ev.ToValueJSON != "" && ev.ToValueJSON != "{}" {
		if err := json.Unmarshal([]byte(ev.ToValueJSON), &toMap); err != nil {
			toMap = nil
		}
	}
	strVal := func(m map[string]any, key string) string {
		if m == nil {
			return ""
		}
		if v, ok := m[key]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return ""
	}
	switch ev.EventType {
	case "class_changed":
		fromCls := dashOr(strVal(fromMap, "class_name"))
		toCls := dashOr(strVal(toMap, "class_name"))
		return fmt.Sprintf(locales.StudentEventTransition(), fromCls, toCls)
	case "track_changed":
		fromT := dashOr(a.trackName(strVal(fromMap, "track"), strVal(fromMap, "track")))
		toT := dashOr(a.trackName(strVal(toMap, "track"), strVal(toMap, "track")))
		if el := strVal(toMap, "elective"); el != "" {
			toT = toT + " · " + el
		}
		return fmt.Sprintf(locales.StudentEventTransition(), fromT, toT)
	case "status_changed":
		fromS := dashOr(a.statusName(strVal(fromMap, "status")))
		toS := dashOr(a.statusName(strVal(toMap, "status")))
		return fmt.Sprintf(locales.StudentEventTransition(), fromS, toS)
	case "enrolled":
		if cls := strVal(toMap, "class_name"); cls != "" {
			return cls
		}
	}
	return ""
}

func (a *app) teacherRoleLabel(role string) string {
	switch role {
	case "head_teacher":
		return locales.TeacherProfileRoleHead()
	case "assistant":
		return locales.TeacherProfileRoleAssistant()
	default:
		return locales.TeacherProfileRolePrimary()
	}
}

func (a *app) teacherReleaseReasonLabel(reason string) string {
	switch reason {
	case "graduated":
		return locales.TeacherProfileReleaseGraduated()
	case "replaced":
		return locales.TeacherProfileReleaseReplaced()
	case "reclassified":
		return locales.TeacherProfileReleaseReclassified()
	case "manual_release", "mid_term_change":
		return locales.TeacherProfileReleaseManual()
	default:
		return dashOr(reason)
	}
}
