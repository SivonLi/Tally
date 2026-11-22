package main

import (
	"context"
	"log"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/form"

	"tally/internal/assessment"
	"tally/internal/calc"
	"tally/internal/school"
	"tally/internal/store"
	"tally/locales"
)

type app struct {
	win                 *mygo.Window
	dataDir             string
	language            string
	store               *store.Store
	plans               []store.Plan
	versions            []store.PlanVersion
	selectedPlanID      string
	versionIndex        int
	sidebarWidth        float32
	loadError           string
	planError           string
	planMessage         string
	planConfig          assessment.Config
	planConfigVersionID int64
	planConfigBaseline  string
	planSaveChoice      planSaveChoice
	planForm            *form.Form[struct{}]
	planFieldSeq        int
	dialogFormHost      *form.Form[struct{}]

	published                    []publishedVersion
	assessments                  []store.AssessmentOverview
	selectedAssessmentID         int64
	assessmentContext            store.AssessmentContext
	assessmentRun                store.ResultRun
	assessmentRows               []store.StoredResultRow
	assessmentPersonal           []store.StoredPersonalScore
	assessmentBusy               bool
	assessmentMessage            string
	assessmentError              string
	batchSidebarWidth            float32
	resultTable                  ui.ListState
	assessmentRuns               []store.AssessmentRunSummary
	assessmentRunID              int64
	assessmentRunChosen          bool
	assessmentOfficialStale      bool
	assessmentOfficialStaleItems []string
	resultSelection              ui.Selection[string]
	resultDetailOpen             bool
	resultDetail                 resultDetail
	calculateVersion             string
	zeroOpen                     bool
	zeroIssues                   []calc.ZeroTargetIssue
	zeroValues                   map[string]*float64
	zeroReason                   string
	zeroError                    string
	zeroMode                     string

	now                  func() time.Time
	page                 pageID
	systemAcademicYear   int
	academicYearStart    int
	cohorts              []store.Cohort
	cohortStages         []store.CohortStage
	selectedCohortID     string
	selectedStageID      int64
	cohortsCollapsed     bool
	stageCounts          store.StageCounts
	stageClasses         []string
	cohortSidebarWidth   float32
	newCohortOpen        bool
	newCohortYear        float64
	newCohortError       string
	settingsError        string
	dataQuery            string
	dataClassFilter      int64
	dataTrackFilter      string
	dataStatusFilter     string
	teacherQuery         string
	groupQuery           string
	groupSubjectFilter   string
	baselineQuery        string
	baselineClassFilter  int64
	importQuery          string
	dataBusy             bool
	dataError            string
	dataMessage          string
	classes              []store.SchoolClass
	students             []store.DataStudent
	studentsTotal        int
	studentTable         ui.ListState
	classTable           ui.ListState
	teacherTable         ui.ListState
	groupTable           ui.ListState
	baselineTable        ui.ListState
	selectedStudentID    int64
	studentDetail        store.DataStudent
	studentDetailOpen    bool
	teachers             []store.DataTeacher
	teachingGroups       []store.DataTeachingGroup
	baselines            []store.BaselineRow
	importRuns           []store.ImportRun
	subjects             []store.Subject
	tracks               []store.Track
	subjectDrafts        []store.Subject
	trackDrafts          []store.Track
	facultyTab           int
	backups              []store.BackupInfo
	restoreBackupName    string
	restoreOpen          bool
	importWizard         *importWizardState
	importRunID          int64
	importDetail         *store.ImportRunDetail
	prepOpen             bool
	stageReviewOpen      bool
	stageNoticeDismissed map[int64]bool

	exams                []store.Exam
	selectedExamID       int64
	examCohortID         int64
	exam                 store.Exam
	examCohort           store.ExamCohort
	examAssessment       store.Assessment
	examTierKeys         []string
	examClasses          []store.SchoolClass
	examTeachingGroups   []store.DataTeachingGroup
	examTab              int
	examRoster           []store.ExamRosterEntry
	examRosterTotal      int
	examScores           []store.ExamScoreRow
	examScoresTotal      int
	examScoreSummaries   []store.ExamScoreSummary
	examScoreBands       []store.ExamScoreBand
	examClassAverages    []store.ExamClassAverage
	examCutoffImpacts    []store.ExamCutoffImpact
	examSubjectCutoffs   []store.ExamSubjectCutoff
	examTierCutoffs      []store.ExamTierCutoff
	examTargets          []store.AssessmentClassTarget
	examStatGroups       []store.ExamStatGroup
	examQuality          []store.ExamQualityIssue
	examPrep             store.ExamPrepStatus
	examRosterQuery      string
	examScoreQuery       string
	examScoreSubject     string
	examScoreClass       int64
	examBusy             bool
	examError            string
	examMessage          string
	newExamOpen          bool
	newExamYear          float64
	newExamTerm          int
	newExamType          int
	newExamName          string
	newExamDate          time.Time
	newExamCohorts       map[int64]bool
	newExamMarks         map[int64]map[string]float64
	examDefaultTrack     string
	examTargetDrafts     map[string]*float64
	examPrepOpen         bool
	qualityExams         []qualityExamOverview
	qualityError         string
	qualitySeverity      int
	qualityImports       map[int64]string
	qualityImportOptions map[int64]string
	qualityDataItem      string
	qualityImportRun     int64
	qualityLoaded        bool
	lastPage             pageID
	statGroupOpen        bool
	statGroupID          int64
	statGroupSubject     string
	statGroupName        string
	statGroupLeadClass   int64
	statGroupTarget      float64
	statGroupMembers     map[int64]bool
	statGroupError       string
	statGroupDeleteOpen  bool
	statGroupDeleteID    int64
	statGroupDeleteName  string

	settingsWin    *mygo.Window
	settingsOpen   bool
	appearance     string
	settingsPageID string
	settingsForm   *form.Form[settingsFormValues]
}

func (a *app) load() {
	ctx := context.Background()
	dir := a.dataDir
	if dir == "" {
		var err error
		dir, err = mygo.App.Path(mygo.PathUserData)
		if err != nil {
			a.loadError = err.Error()
			return
		}
	}
	opened, err := store.Open(ctx, filepath.Join(dir, "tally.db"))
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.store = opened
	a.dataDir = dir
	a.systemAcademicYear = school.AcademicYearStart(a.nowValue())
	a.academicYearStart, err = opened.CurrentAcademicYear(ctx, a.systemAcademicYear)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	if _, err := opened.EnsureDefaultPlan(ctx); err != nil {
		a.loadError = err.Error()
		return
	}
	a.reloadPlans(ctx)
	a.reloadCohorts(ctx)
	a.reloadExams(ctx)
	a.reloadAssessments(ctx)
	a.reloadSchoolSettings(ctx)
	appearance, err := opened.Appearance(ctx)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.appearance = appearance
	if a.page == "" {
		a.page = pageOverview
	}
}

func (a *app) reloadPlans(ctx context.Context) {
	if a.store == nil {
		return
	}
	plans, err := a.store.ListPlans(ctx)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.plans = plans
	if len(plans) == 0 {
		a.selectedPlanID = ""
		a.versions = nil
		return
	}
	found := false
	for _, plan := range plans {
		if strconv.FormatInt(plan.ID, 10) == a.selectedPlanID {
			found = true
			break
		}
	}
	if !found {
		a.selectedPlanID = strconv.FormatInt(plans[0].ID, 10)
	}
	a.reloadVersions(ctx)
	a.reloadPublished(ctx)
}

func (a *app) reloadVersions(ctx context.Context) {
	if a.store == nil || a.selectedPlanID == "" {
		a.versions = nil
		a.versionIndex = -1
		return
	}
	planID, err := strconv.ParseInt(a.selectedPlanID, 10, 64)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	versions, err := a.store.ListVersions(ctx, planID)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.versions = versions
	if len(versions) == 0 {
		a.versionIndex = -1
		return
	}
	if a.versionIndex < 0 || a.versionIndex >= len(versions) {
		a.versionIndex = len(versions) - 1
	}
}

func (a *app) selectedPlan() *store.Plan {
	if a.selectedPlanID == "" {
		return nil
	}
	planID, err := strconv.ParseInt(a.selectedPlanID, 10, 64)
	if err != nil {
		return nil
	}
	for i := range a.plans {
		if a.plans[i].ID == planID {
			return &a.plans[i]
		}
	}
	return nil
}

func (a *app) selectedVersion() *store.PlanVersion {
	if a.versionIndex < 0 || a.versionIndex >= len(a.versions) {
		return nil
	}
	return &a.versions[a.versionIndex]
}

func (a *app) kindLabel(kind string) string {
	if kind == "system" {
		return locales.PlanKindSystem()
	}
	return locales.PlanKindUser()
}

func formatTime(value time.Time) string {
	return value.Local().Format("2006-01-02 15:04")
}

func main() {
	a := &app{}
	mygo.App.WhenReady(func() {
		a.language = mygo.App.Locale()
		a.load()
		if a.appearance == "" {
			a.appearance = store.AppearanceSystem
		}
		switch a.appearance {
		case store.AppearanceLight:
			mygo.Theme.SetSource(mygo.ThemeLight)
		case store.AppearanceDark:
			mygo.Theme.SetSource(mygo.ThemeDark)
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:                locales.AppTitle(),
			Width:                1280,
			Height:               800,
			MinWidth:             1024,
			MinHeight:            640,
			StateKey:             "tally",
			TitleBarStyle:        mygo.TitleBarHidden,
			TitleBarHeight:       int(topStripHeight),
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			Vibrancy:             mygo.VibrancySidebar,
			BackgroundColor:      "light-dark(#ffffff, #000000)",
			Content:              ui.View(a.view),
		})
		mygo.App.OnLocaleChanged(func(string) {
			if a.win != nil {
				a.win.SetTitle(locales.AppTitle())
			}
			a.invalidateSettings()
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			log.Println(err)
		}
	}
}
