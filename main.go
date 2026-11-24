package main

import (
	"context"
	"log"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tally/internal/assessment"
	"tally/internal/calc"
	"tally/internal/importer"
	"tally/internal/pipeline"
	"tally/internal/school"
	"tally/internal/store"
	"tally/internal/sync"
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
	planFieldSeq        int

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
	assessmentResultTab          int
	assessmentSubjectFilter      string
	resultTable                  ui.ListState
	classResultTable             ui.ListState
	teacherResultTable           ui.ListState
	assessmentRuns               []store.AssessmentRunSummary
	assessmentRunID              int64
	assessmentRunChosen          bool
	assessmentOfficialStale      bool
	assessmentOfficialStaleItems []string
	resultSelection              ui.Selection[string]
	classSelection               ui.Selection[string]
	teacherSelection             ui.Selection[string]
	resultDetailOpen             bool
	resultDetail                 resultDetail
	calculateVersion             string
	zeroOpen                     bool
	zeroIssues                   []calc.ZeroTargetIssue
	zeroValues                   map[string]*float64
	zeroExcluded                 map[string]bool
	zeroReason                   string
	zeroError                    string
	zeroMode                     string
	runCompareOpen               bool
	runCompareBaseID             int64
	runCompareTargetID           int64
	runCompareOnlyChanged        bool
	runCompareTab                int
	runCompareDiff               *pipeline.RunDiff
	runCompareError              string
	qualityFixOpen               bool
	qualityFixExamID             int64
	qualityFixCohortID           int64
	qualityFixExamName           string
	qualityFixStudentID          int64
	qualityFixStudentNo          string
	qualityFixStudentName        string
	qualityFixIssue              store.ExamQualityIssue
	qualityFixTrack              string
	qualityFixSubject            string
	qualityFixScore              float64
	qualityFixFlag               string
	qualityFixError              string

	analysisTabIndex                int
	analysisTeacherName             string
	analysisClassID                 int64
	analysisSubject                 string
	analysisStudentID               int64
	analysisSelectedExamIDs         map[int64]bool
	analysisComparisonAssessmentIDs map[int64]bool
	analysisTeacherResult           store.TeacherTrendResult
	analysisClassResult             store.ClassTrendResult
	analysisSubjectResult           store.SubjectComparisonResult
	analysisStudentResult           store.StudentTrackResult
	analysisComparisonResult        store.AssessmentComparisonResult
	analysisSavedViews              []store.SavedView
	analysisSelectedViewID          int64
	analysisSaveViewOpen            bool
	analysisSaveViewName            string
	analysisSaveViewError           string
	analysisDeleteViewOpen          bool
	analysisDeleteViewID            int64
	analysisDeleteViewName          string
	analysisMessage                 string
	analysisError                   string

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
	studentPage          int
	studentPageSize      int
	studentTable         ui.ListState
	classTable           ui.ListState
	teacherTable         ui.ListState
	groupTable           ui.ListState
	baselineTable        ui.ListState
	examRosterTable      ui.ListState
	examScoresTable      ui.ListState
	examTargetsTable     ui.ListState
	importsListState     ui.ListState
	qualityListState     ui.ListState
	examQualityListState ui.ListState
	wizardIssuesList     ui.ListState
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
	secrets              secretStore
	syncSettings         store.SyncSettings
	syncWebDAVPassword   string
	syncPackagePassword  string
	syncLoaded           bool
	syncBusy             bool
	syncMessage          string
	syncError            string
	syncRemoteStatus     *sync.RemoteStatus
	syncConflictOpen     bool
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
	examRosterPage       int
	examRosterPageSize   int
	examScores           []store.ExamScoreRow
	examScoresTotal      int
	examScorePage        int
	examScorePageSize    int
	examScoresStatsOpen  bool
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

	settingsWin            *mygo.Window
	settingsOpen           bool
	appearance             string
	settingsPageID         string
	mainToast              string
	settingsToast          string
	workspaceSummary       store.WorkspaceSummary
	workspaceSummaryLoaded bool
	baseDataTab            int
	importReturn           *importReturnContext
	importCenterTab        int
	importCenterScope      int
	importCenterExamID     int64
	importCenterWorkbook   *importer.Workbook
	importCenterFilePath   string
	importCenterFileName   string
	importCenterSheets     []importer.SheetClassification
	importCenterReading    bool
	importCenterError      string
	importUndoConfirmOpen  bool
	unsavedGuardOpen       bool
	pendingNavAction       func()
	sofficePath            string
}

func (a *app) notifyMain(message string) {
	if message == "" {
		return
	}
	a.mainToast = message
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) notifySettings(message string) {
	if message == "" {
		return
	}
	a.settingsToast = message
	a.invalidateSettings()
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
	if syncSet, err := opened.GetSyncSettings(ctx); err == nil {
		a.syncSettings = syncSet
	}
	a.loadSyncSecrets()
	a.syncLoaded = true
	appearance, err := opened.Appearance(ctx)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.appearance = appearance
	if soffice, err := opened.SofficePath(ctx); err == nil {
		a.sofficePath = soffice
	}
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
			BackgroundColor:      "light-dark(#faf8f5, #090706)",
			Content:              ui.View(a.view),
		})
		mygo.App.OnLocaleChanged(func(string) {
			if a.win != nil {
				a.win.SetTitle(locales.AppTitle())
			}
			if a.settingsWin != nil {
				a.settingsWin.SetTitle(locales.NavSettings())
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
