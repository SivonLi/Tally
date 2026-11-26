package store

import (
	"time"

	"tally/internal/assessment"
)

// 阶段与升学常量定义。
const (
	PhaseG1        = "g1"
	PhaseG2        = "g2"
	PhaseG3        = "g3"
	PhaseGraduated = "graduated"

	PromoteDirect     = "direct"
	PromoteReclassify = "reclassify"

	TeacherStatusInactive     = "inactive"
	TeacherStatusIdle         = "idle"
	TeacherStatusActiveSingle = "active_single"
	TeacherStatusActiveMulti  = "active_multi"
)

// Plan 是一套量化考核方案的元信息。
type Plan struct {
	ID                int64
	Name              string
	Kind              string
	Description       string
	BasePlanVersionID int64
	Scope             string
	IsSystem          bool
	CreatedAt         time.Time
}

// CreatePlanInput 是新建考核方案的请求参数。
type CreatePlanInput struct {
	Name          string
	Kind          string
	Description   string
	BaseVersionID int64
	Config        assessment.Config
	CreatedBy     string
}

// CreateDraftInput 是为考核方案追加草稿版本的请求参数。
type CreateDraftInput struct {
	PlanID        int64
	BaseVersionID int64
	Config        assessment.Config
	Notes         string
	CreatedBy     string
}

// PlanVersion 是考核方案的一个版本。
type PlanVersion struct {
	ID              int64
	PlanID          int64
	Version         int
	Status          string
	Config          assessment.Config
	Note            string
	CreatedBy       string
	ParentVersionID int64
	ChangeSummary   string
	ConfigHash      string
	PublishedAt     *time.Time
	CreatedAt       time.Time
}

// Subject 是一个学科设置。
type Subject struct {
	Key      string
	Name     string
	FullMark float64
	Sort     int
	Enabled  bool
}

// Track 是一个选科类别。
type Track struct {
	Key     string
	Name    string
	Sort    int
	Enabled bool
}

// Cohort 是按入学年份建立的三年制年级。
type Cohort struct {
	ID                       int64
	Name                     string
	EnrollmentYear           int
	GraduationYear           int
	CurrentPhase             string
	CurrentAcademicYearStart int
	GraduatedAt              *time.Time
	// Status 为阶段 1 界面兼容字段，由 CurrentPhase 推导：graduated 或 active。
	Status    string
	CreatedAt time.Time
}

// CohortStage 是基于 Cohort(cohort_id, phase) 衍生的阶段视图，兼容现有阶段 1 界面。
type CohortStage struct {
	ID                int64
	CohortID          int64
	AcademicYearStart int
	GradeLevel        string
	CopiedFromStageID *int64
	ReviewStatus      string
	ConfirmedAt       *time.Time
}

// StageCounts 是一个级在一个阶段中的基础数据数量。
type StageCounts struct {
	Classes        int
	Students       int
	Teachers       int
	TeachingGroups int
	Assignments    int
	Baselines      int
}

// PromoteInput 是年级升学流转的请求参数。
type PromoteInput struct {
	ToPhase             string
	ToAcademicYearStart int
	// Mode 为 PromoteDirect ("direct"，原班直升) 或 PromoteReclassify ("reclassify"，选科重分班)。
	Mode string
}

// PromoteResult 是年级升学流转的摘要结果。
type PromoteResult struct {
	CohortID              int64
	FromPhase             string
	ToPhase               string
	ToAcademicYearStart   int
	Mode                  string
	ClassesCreated        int
	TeachingGroupsCreated int
	AssignmentsCarried    int
	HeadTeachersCarried   int
	StudentsUpdated       int
	StageCopySummary      StageCopySummary
}

// StageCopySummary 兼容旧版升学/复制阶段统计。
type StageCopySummary struct {
	FromStageID    int64
	ToStageID      int64
	Classes        int
	Students       int
	TeachingGroups int
	Assignments    int
}

// GraduatePreview 是毕业归档前的只读影响摘要。
type GraduatePreview struct {
	CohortID                    int64
	EnrollmentYear              int
	GraduationYear              int
	CurrentPhase                string
	ActiveStudents              int
	ExamCount                   int
	TotalTeachersAffected       int
	ReleasedToIdleCount         int
	StillActiveCrossCohortCount int
	ReleasedTeachers            []TeacherOccupancy
	CrossCohortTeachers         []TeacherOccupancy
}

// GraduateResult 是执行毕业归档后的原子操作结果。
type GraduateResult struct {
	CohortID          int64
	GraduatedAt       time.Time
	GraduatedClasses  int
	GraduatedGroups   int
	GraduatedStudents int
	EndedAssignments  int
	EndedHeadTeachers int
	ReleasedToIdle    int
}

// TeacherActiveAssignment 描述教师当前或历史承担的一条教学班或班主任职责。
type TeacherActiveAssignment struct {
	CohortID          int64
	EnrollmentYear    int
	GraduationYear    int
	Phase             string
	ClassID           int64
	ClassName         string
	TeachingGroupID   int64
	TeachingGroupName string
	Subject           string
	Role              string // primary | assistant | head_teacher
	Status            string // active | ended
	EndReason         string
	StartedAt         string
	EndedAt           string
}

// TeacherLastRelease 描述教师最近一次结束任教的原因与时间。
type TeacherLastRelease struct {
	CohortID       int64
	EnrollmentYear int
	Phase          string
	ClassName      string
	Subject        string
	EndReason      string
	EndedAt        time.Time
}

// TeacherOccupancy 描述全校教师库中一名教师的实时占用状态。
type TeacherOccupancy struct {
	TeacherID         int64
	TeacherNo         string
	Name              string
	PrimarySubject    string
	Status            string // inactive | idle | active_single | active_multi
	ActiveCohorts     []int64
	ActiveAssignments []TeacherActiveAssignment
	LastRelease       *TeacherLastRelease
}

// CrossGradeWarning 描述教师跨年级兼任提示（非阻断）。
type CrossGradeWarning struct {
	TeacherID       int64
	TeacherNo       string
	TeacherName     string
	ExistingTargets []TeacherActiveAssignment
}

// SchoolClass 是一个级在某个阶段的行政班实例。
type SchoolClass struct {
	ID                   int64
	CohortID             int64
	Name                 string
	StageID              int64
	Stage                string // g1 | g2 | g3
	ContinuedFromClassID *int64
	TrackHint            string
	Room                 string
	Status               string // active | archived | graduated
	AcademicYear         int
	Students             int
	HeadTeacher          string
	HeadTeacherID        int64
}

// ClassChainNode 是班级跨学年传承链上的单个节点。
type ClassChainNode struct {
	ClassID              int64
	CohortID             int64
	Phase                string
	Name                 string
	ContinuedFromClassID *int64
	Status               string
	HeadTeacher          string
	StudentCount         int
}

// DataTeacher 是教师与全校占用列表项。
type DataTeacher struct {
	ID            int64
	TeacherNo     string
	Name          string
	Subject       string
	Status        string // active | inactive
	Note          string
	Occupancy     string // inactive | idle | active_single | active_multi
	ActiveSummary string
	Groups        int
	Assignments   int
	ImportRunID   int64
	SourceSheet   string
	SourceRow     int
}

// DataTeachingGroup 是教学班列表项。
type DataTeachingGroup struct {
	ID           int64
	CohortID     int64
	StageID      int64
	Phase        string
	Subject      string
	Name         string
	Kind         string
	AdminClassID int64
	Status       string
	Students     int
	Teachers     string
	ClassNames   string
	ImportRunID  int64
	SourceSheet  string
	SourceRow    int
}

// DataStudent 是数据页面显示的学生。
type DataStudent struct {
	ID                int64
	StudentNo         string
	CandidateCode     string
	Name              string
	Gender            string
	CohortID          int64
	StageID           int64
	ClassID           int64
	ClassName         string
	Track             string
	Elective          string
	ForeignLang       string
	Status            string
	Walking           bool
	Japanese          bool
	TotalScore        *float64
	PreviousStudentID *int64
	TeachingGroups    []string
	ImportRunID       int64
	SourceSheet       string
	SourceRow         int
}

// StudentFilter 是学生列表的查询条件。
type StudentFilter struct {
	CohortID int64
	StageID  int64
	Phase    string
	ClassID  int64
	Track    string
	Status   string
	Query    string
	Page     int
	PageSize int
}

// StudentEntryScore 是学生单科入学/中考基数成绩。
type StudentEntryScore struct {
	Subject     string
	Score       float64
	ImportRunID int64
	SourceSheet string
	SourceRow   int
}

// StudentEvent 是学生三年学籍与变动时间线事件。
type StudentEvent struct {
	ID            int64
	StudentID     int64
	CohortID      int64
	Phase         string
	EventType     string
	FromValueJSON string
	ToValueJSON   string
	ExamID        *int64
	Note          string
	OccurredAt    time.Time
}

// AddStudentEventInput 描述手工录入学生学籍/分班/选科异动的参数。
type AddStudentEventInput struct {
	EventType     string
	Phase         string
	ToClassID     int64
	ToTrack       string
	ToElective    string
	ToForeignLang string
	ToStatus      string
	Note          string
	OccurredAt    *time.Time
}

// StudentProfile 是阶段 2 学生个人三年完整档案视图。
type StudentProfile struct {
	Student         DataStudent
	Cohort          Cohort
	PreviousStudent *DataStudent
	PreviousCohort  *Cohort
	EntryScores     []StudentEntryScore
	Events          []StudentEvent
	Exams           []StudentTrackPoint
}

// TeacherExamHistory 是教师在单场考试下的考核得分记录。
type TeacherExamHistory struct {
	ExamID            int64
	AssessmentID      int64
	ExamName          string
	ExamType          string
	AcademicYearStart int
	Term              string
	ExamDate          string
	CohortID          int64
	EnrollmentYear    int
	PhaseAtExam       string
	Subject           string
	Score             float64
	GroupPoints       []float64
	ClassRows         []StoredResultRow
}

// TeacherCohortHistory 按届展示教师历史带班与得分记录。
type TeacherCohortHistory struct {
	CohortID       int64
	EnrollmentYear int
	GraduationYear int
	CurrentPhase   string
	Assignments    []TeacherActiveAssignment
	Exams          []TeacherExamHistory
}

// TeacherProfile 是阶段 2 教师档案与跨届表现视图。
type TeacherProfile struct {
	Teacher           DataTeacher
	Occupancy         TeacherOccupancy
	CrossCohortScores []TeacherTrendPoint
	CohortHistories   []TeacherCohortHistory
}

// BaselineRow 是入口基数汇总展示行。
type BaselineRow struct {
	BaselineSetID int64
	SetName       string
	ClassID       int64
	ClassName     string
	Subject       string
	Average       float64
	Count         int
	Ratio         float64
	ImportRunID   int64
	SourceSheet   string
	SourceRow     int
}

// ImportRun 是一条导入记录。
type ImportRun struct {
	ID           int64
	ScopeType    string
	ScopeID      int64
	DataKind     string
	FileHash     string
	StoredPath   string
	RelativePath string
	Status       string
	RowCount     int
	Inserted     int
	Updated      int
	Skipped      int
	Blocking     int
	Warnings     int
	CreatedAt    time.Time
	Sheets       []ImportSheetInfo
}

// ImportSheetInfo 是导入记录中的一个工作表。
type ImportSheetInfo struct {
	Name      string
	Role      string
	HeaderRow int
	Rows      int
	Current   bool
}

// ImportStudent 是导入时的一名学生。
type ImportStudent struct {
	StudentNo         string
	CandidateCode     string
	Name              string
	Gender            string
	ClassName         string
	Track             string
	Elective          string
	Status            string
	Japanese          bool
	Walking           bool
	TotalScore        *float64
	PreviousStudentID *int64
	Scores            map[string]float64
	Flags             map[string]string
	SourceSheet       string
	SourceRow         int
}

// ImportAssignment 是导入时的一条任课关系。
type ImportAssignment struct {
	ClassName   string
	Subject     string
	TeacherName string
	TeacherNo   string
	Role        string
	SourceSheet string
	SourceRow   int
}

// ImportTeacher 是导入时的一名教师。
type ImportTeacher struct {
	TeacherNo   string
	Name        string
	Subject     string
	Note        string
	SourceSheet string
	SourceRow   int
}

// ImportClassTarget 是一个行政班的分档目标人数。
type ImportClassTarget struct {
	ClassName    string
	Counts       map[string]*float64
	ActualCounts map[string]*float64
}

// ImportClassBaseline 是一个行政班某个学科的基数均分。
type ImportClassBaseline struct {
	ClassName   string
	Subject     string
	Average     float64
	Ratio       *float64
	SourceSheet string
	SourceRow   int
}

// ImportSubjectCutoff 是一个学科在某一类考生的分数线。
type ImportSubjectCutoff struct {
	Subject string
	Track   string
	Value   float64
}

// ImportTierCutoff 是一个总分档位在某一类考生中的分数线。
type ImportTierCutoff struct {
	TierKey string
	Track   string
	Value   float64
}

// ImportStatGroup 是显式配置的统计组，用于走班合并。
type ImportStatGroup struct {
	Name            string
	Subject         string
	LeadClassName   string
	MemberClasses   []string
	Target          float64
	BaselineAverage float64
}

// ImportRoster 是报名名单中的一行。
type ImportRoster struct {
	StudentNo   string
	Name        string
	ClassName   string
	ExamNo      string
	CandidateNo string
	Track       string
	Elective    string
	Status      string
	TotalScore  *float64
	SourceSheet string
	SourceRow   int
}

// ImportSheet 记录一个被识别的工作表。
type ImportSheet struct {
	Name      string
	Role      string
	HeaderRow int
	Rows      int
}

// ImportReport 记录一次导入的来源与识别结果。
type ImportReport struct {
	FileHash      string
	Path          string
	StoredPath    string
	ConvertedPath string
	Format        string
	Conversion    string
	RowCount      int
	Sheets        []ImportSheet
	Warnings      []string
}

// ImportData 是一次导入解析出的全部数据。
type ImportData struct {
	Classes        []string
	Students       []ImportStudent
	Roster         []ImportRoster
	Teachers       []string
	TeacherList    []ImportTeacher
	Assignments    []ImportAssignment
	ClassTargets   []ImportClassTarget
	ClassBaselines []ImportClassBaseline
	SubjectCutoffs []ImportSubjectCutoff
	TierCutoffs    []ImportTierCutoff
	StatGroups     []ImportStatGroup
	GradeBaselines map[string]float64
}

// ImportIssue 是一条导入校验问题。Code 与 Args 用于界面按语言生成文案。
type ImportIssue struct {
	Severity string
	Row      int
	Column   string
	Code     string
	Args     []string
}

// StageImportRequest 是一次阶段数据导入请求。
type StageImportRequest struct {
	Kind    string
	Data    ImportData
	Report  ImportReport
	Mapping map[string]string
	Issues  []ImportIssue
}

// ImportPreview 是写入前的校验结果。
type ImportPreview struct {
	Kind     string
	New      int
	Update   int
	Skip     int
	Issues   []ImportIssue
	Blocking int
	Warnings int
}

// ImportRunDetail 是导入记录详情。
type ImportRunDetail struct {
	Run    ImportRun
	Issues []ImportIssue
}

// ImportSummary 是导入完成后可以显示的数量摘要。
type ImportSummary struct {
	Classes        int
	Students       int
	Teachers       int
	TeachingGroups int
	StatGroups     int
	Assignments    int
	ClassTargets   int
	SubjectCutoffs int
	TierCutoffs    int
	Baselines      int
	Inserted       int
	Updated        int
	Skipped        int
	Warnings       []string
}

// Batch 描述导出或兼容口径使用的考核元信息。
type Batch struct {
	ID                   int64
	TermID               int64
	AcademicYear         string
	Semester             string
	ExamType             string
	Grade                string
	Status               string
	CurrentPlanVersionID int64
	Revision             int
	CreatedAt            time.Time
}

// Class 是一个行政班。
type Class struct {
	ID   int64
	Name string
}

// Student 是一名学生及其在考试或考核下的成绩与归属快照。
type Student struct {
	ID             int64
	StudentNo      string
	Name           string
	ClassID        int64
	Track          string
	Elective       string
	Japanese       bool
	Walking        bool
	Status         string
	TotalScore     *float64
	Scores         map[string]float64
	ScoreFlags     map[string]string
	TeachingGroups map[string]int64
}

// TeachingGroup 是一个学科教学班。
type TeachingGroup struct {
	ID      int64
	Subject string
	Name    string
	ClassID int64
}

// StatGroup 是单科达线率的计算单位。
type StatGroup struct {
	ID              int64
	Subject         string
	Name            string
	LeadClassID     int64
	MemberGroupIDs  []int64
	Target          float64
	BaselineAverage float64
	BaselineRatio   float64
}

// Assignment 是教师与教学班的任课关系。
type Assignment struct {
	TeacherID       int64
	TeacherName     string
	TeachingGroupID int64
}

// ClassTarget 是一个行政班的分档目标人数。
type ClassTarget struct {
	ClassID      int64
	Counts       map[string]*float64
	ActualCounts map[string]*float64
}

// SubjectCutoff 是一个学科在两类考生中的分数线。
type SubjectCutoff struct {
	Subject string
	Track   string
	Value   float64
	Cutoff  float64
}

// ExamSubjectCutoff 是考核与考试页使用的学科分数线别名。
type ExamSubjectCutoff = SubjectCutoff

// TierCutoff 是总分档位在两类考生中的分数线。
type TierCutoff struct {
	TierKey string
	Track   string
	Value   float64
	Cutoff  float64
}

// ExamTierCutoff 是考核与考试页使用的档位分数线别名。
type ExamTierCutoff = TierCutoff

// AssessmentClassTarget 表示单个行政班在某档位上的目标人数。
type AssessmentClassTarget struct {
	ClassID   int64
	ClassName string
	TierKey   string
	Target    float64
}

// ExamClassTargetRow 兼容别名。
type ExamClassTargetRow = AssessmentClassTarget

// AssessmentData 是一次考核读取后可以直接用于核算的数据。
type AssessmentData struct {
	Batch                     Batch
	Classes                   []Class
	Students                  []Student
	TeachingGroups            []TeachingGroup
	StatGroups                []StatGroup
	Assignments               []Assignment
	ClassTargets              []ClassTarget
	ClassBaselines            map[string]map[string]float64
	ClassBaselineRatios       map[string]map[string]float64
	GradeBaselines            map[string]float64
	SubjectCutoffs            []SubjectCutoff
	TierCutoffs               []TierCutoff
	ExcludedCompletionClasses map[int64]bool
}

// BatchData 兼容导出与管线调用方使用的 AssessmentData 别名。
type BatchData = AssessmentData

// ResultRun 是一次核算记录。
type ResultRun struct {
	ID            int64
	BatchID       int64
	PlanVersionID int64
	InputHash     string
	InputSummary  string
	IsOfficial    bool
	CreatedAt     time.Time
}

// StoredResultRow 是一条保存下来的核算结果。
type StoredResultRow struct {
	ID                    int64
	StatGroupID           int64
	StatGroupName         string
	Subject               string
	LeadClassID           int64
	LeadClassName         string
	TeacherNames          string
	TeacherIDs            []int64
	ClassCount            int
	ClassAverage          float64
	GradeAverage          float64
	BaselineAverage       float64
	BaselineRatio         float64
	ImprovementRaw        float64
	ImprovementScore      float64
	PassCount             int
	PassTarget            float64
	PassRaw               float64
	PassScore             float64
	CompletionActual      float64
	CompletionBase        float64
	CompletionRaw         float64
	CompletionParticipate bool
	CompletionScore       float64
	Total                 float64
	Rank                  int
	TierIndex             int
	Points                float64
	ParticipantsJSON      string
	IndicatorJSON         string
}

// StoredPersonalScore 是一条保存下来的教师个人得分。
type StoredPersonalScore struct {
	TeacherID   int64
	TeacherName string
	CohortID    int64
	Grade       string
	Subject     string
	GroupPoints string
	Score       float64
}

// ResultRowStudent 记录参与某条统计组结果的单个学生快照与达线明细。
type ResultRowStudent struct {
	Position  int      `json:"position,omitempty"`
	StudentID int64    `json:"studentId,omitempty"`
	StudentNo string   `json:"studentNo"`
	Name      string   `json:"name"`
	ClassName string   `json:"className,omitempty"`
	Track     string   `json:"track,omitempty"`
	Score     *float64 `json:"score,omitempty"`
	Flag      string   `json:"flag,omitempty"`
	Cutoff    float64  `json:"cutoff,omitempty"`
	Passed    bool     `json:"passed,omitempty"`
}

// -----------------------------------------------------------------------------
// Exam / Assessment / Overview / Analysis Types
// -----------------------------------------------------------------------------

const (
	ExamTermFirst  = "autumn"
	ExamTermSecond = "spring"

	ExamMonthly = "monthly"
	ExamMidterm = "midterm"
	ExamFinal   = "final"
	ExamMock    = "mock"
	ExamGaokao  = "gaokao"
)

// Exam 是一场覆盖一个或者多个级的考试。
type Exam struct {
	ID                int64
	AcademicYearStart int
	Term              string
	ExamType          string
	Name              string
	ExamDate          string
	Sequence          int
	Status            string
	CreatedAt         time.Time
	Cohorts           []ExamCohort
}

// ExamCohort 是考试中的一个级与考试当时的阶段。
type ExamCohort struct {
	ID                int64
	ExamID            int64
	CohortID          int64
	StageID           int64
	GradeLevelAtExam  string
	CohortName        string
	EnrollmentYear    int
	GraduationYear    int
	RosterCount       int
	ScoreCount        int
	SubjectCount      int
	ClassTargetCount  int
	StatGroupCount    int
	AcademicYearStart int
}

// CreateExamInput 是新建考试需要的字段。
type CreateExamInput struct {
	AcademicYearStart int
	Term              string
	ExamType          string
	Name              string
	ExamDate          string
	Sequence          int
	CohortIDs         []int64
	SubjectMarks      map[int64]map[string]float64
}

// ExamSubject 是一场考试中一个级的科目满分。
type ExamSubject struct {
	ExamID   int64   `json:"exam_id,omitempty"`
	CohortID int64   `json:"cohort_id,omitempty"`
	Subject  string  `json:"subject"`
	FullMark float64 `json:"full_mark"`
	Enabled  bool    `json:"enabled"`
}

// ExamRosterEntry 是报名名单中的一名学生。
type ExamRosterEntry struct {
	ID              int64
	StudentID       int64
	StudentNo       string
	Name            string
	ClassID         int64
	ClassName       string
	ExamNo          string
	CandidateNo     string
	Track           string
	Elective        string
	ForeignLang     string
	Status          string
	TotalScore      *float64
	GradeRank       *int
	GradeTotalCount *int
	TrackRank       *int
	TrackTotalCount *int
	ClassRank       *int
	ClassTotalCount *int
	Subjects        []string
	SourceSheet     string
	SourceRow       int
	ImportRunID     int64
}

// ExamRosterFilter 是报名名单的查询条件。
type ExamRosterFilter struct {
	ExamID   int64
	CohortID int64
	Query    string
	Page     int
	PageSize int
}

// ExamRosterSummary 是生成报名名单后的数量。
type ExamRosterSummary struct {
	Students     int
	Inserted     int
	Existing     int
	MissingTrack int
}

// ExamRosterDriftItem 表示参考名单快照与当前学生主档之间的班级或选科差异。
type ExamRosterDriftItem struct {
	StudentID     int64
	StudentNo     string
	StudentName   string
	ExamClassID   int64
	ExamClassName string
	ExamTrack     string
	ExamElective  string
	CurClassID    int64
	CurClassName  string
	CurTrack      string
	CurElective   string
	ClassChanged  bool
	TrackChanged  bool
}

// ExamScoreRow 是一条考试成绩。
type ExamScoreRow struct {
	StudentID             int64
	StudentNo             string
	Name                  string
	ClassID               int64
	ClassName             string
	Subject               string
	Score                 *float64
	Flag                  string
	TeachingGroupSnapshot *int64
	SubjectGradeRank      *int
	SubjectClassRank      *int
	SourceSheet           string
	SourceRow             int
	ImportRunID           int64
}

// ExamScoreFilter 是成绩查询条件。
type ExamScoreFilter struct {
	ExamID   int64
	CohortID int64
	Subject  string
	ClassID  int64
	Query    string
	Page     int
	PageSize int
}

// ExamScoreSummary 是一个科目已录入成绩的数量与分布。
type ExamScoreSummary struct {
	Subject string
	Count   int
	Missing int
	Average float64
	Minimum float64
	Maximum float64
}

// ExamScoreBand 是成绩分布中的一个分数区间。
type ExamScoreBand struct {
	Subject string
	Lower   float64
	Upper   float64
	Count   int
}

// ExamClassAverage 是一个行政班在某科目的均分与人数。
type ExamClassAverage struct {
	Subject   string
	ClassID   int64
	ClassName string
	Count     int
	Average   float64
}

// ExamCutoffImpact 是一条分数线达到人数与占比。
type ExamCutoffImpact struct {
	Subject      string
	Track        string
	Cutoff       float64
	Rostered     int
	Reached      int
	ReachedRatio float64
}

// ExamStatGroupMember 是一个统计组中的教学班。
type ExamStatGroupMember struct {
	TeachingGroupID int64
	Subject         string
	Name            string
	Kind            string
	StudentCount    int
	ClassNames      string
}

// ExamStatGroup 是考核下的一条统计组。
type ExamStatGroup struct {
	ID              int64
	AssessmentID    int64
	Subject         string
	Name            string
	LeadClassID     int64
	LeadClassName   string
	Target          *float64
	BaselineAverage float64
	BaselineRatio   float64
	BaselineSource  string
	Explicit        bool
	Members         []ExamStatGroupMember
}

// ExamStatGroupRow 为 ExamStatGroup 的兼容别名。
type ExamStatGroupRow = ExamStatGroup

// StatGroupInput 是新建或者编辑统计组的参数。
type StatGroupInput struct {
	ID              int64
	AssessmentID    int64
	Subject         string
	Name            string
	LeadClassID     int64
	MemberGroupIDs  []int64
	Target          *float64
	BaselineAverage float64
	BaselineRatio   float64
	BaselineSource  string
}

// ExamQualityIssue 描述一条考务或基础数据质量问题。
type ExamQualityIssue struct {
	Severity    string
	Code        string
	StudentNo   string
	Name        string
	ClassID     int64
	ClassName   string
	Subject     string
	TierKey     string
	Row         int
	ImportRunID int64
	Args        []string
}

// ExamPrepStatus 是 ExamPrepSummary 的别名。
type ExamPrepStatus = ExamPrepSummary

// ExamQualitySummary 概括一场考试中某个级的就绪状态与问题数。
type ExamQualitySummary struct {
	ExamID         int64
	ExamName       string
	ExamType       string
	Term           string
	ExamDate       string
	CohortID       int64
	CohortName     string
	GradeLevel     string
	Blocking       int
	Warnings       int
	AssessmentID   int64
	Status         string
	HasOfficialRun bool
	Issues         []ExamQualityIssue
}

// ExamPrepSummary 是考前数据检查总览。
type ExamPrepSummary struct {
	ExamID       int64
	ExamName     string
	CohortID     int64
	CohortName   string
	GradeLevel   string
	RosterCount  int
	Roster       int
	Scores       int
	Cutoffs      int
	Targets      int
	ClassCount   int
	GroupCount   int
	StatGroups   int
	TargetCount  int
	SubjectCount int
	MissingTrack int
	MissingGroup int
	ZeroTarget   int
	Issues       []ExamQualityIssue
}

// Assessment 是某场考试某个级的一次量化考核。
type Assessment struct {
	ID           int64
	ExamCohortID int64
	PlanVersion  int64
	Status       string
	CurrentRunID int64
	Revision     int
	Archived     bool
	CreatedAt    time.Time
}

// AssessmentContext 是一场考试中一个级的考核及其展示信息。
type AssessmentContext struct {
	Assessment   Assessment
	Exam         Exam
	ExamCohort   ExamCohort
	Cohort       Cohort
	Stage        CohortStage
	AcademicYear string
	Grade        string
}

// AssessmentRunSummary 描述一次核算运行摘要。
type AssessmentRunSummary struct {
	ID            int64
	PlanVersionID int64
	InputHash     string
	IsOfficial    bool
	CreatedAt     time.Time
}

// AssessmentOverview 描述列表页展示的考核摘要。
type AssessmentOverview struct {
	AssessmentID      int64
	ExamCohortID      int64
	ExamID            int64
	AcademicYearStart int
	Term              string
	ExamType          string
	ExamName          string
	ExamDate          string
	CohortID          int64
	CohortName        string
	GradeLevel        string
	Status            string
	PlanVersionID     int64
	CurrentRunID      int64
	HasOfficial       bool
	Archived          bool
	RosterCount       int
	ScoreCount        int
	SubjectCutoffCnt  int
	TierCutoffCnt     int
	CutoffCount       int
	TargetCount       int
	GroupCount        int
	StatGroupCount    int
}

// ZeroTargetBatchItem 是批量设置零目标时的单个班级配置项。
type ZeroTargetBatchItem struct {
	ClassID int64
	TierKey string
	Targets map[string]float64
	Exclude bool
	Target  *float64
	Reason  string
}

// WorkspaceAssessmentSnapshot 是总览台账上最近一次考核快照。
type WorkspaceAssessmentSnapshot struct {
	AssessmentID       int64
	ExamID             int64
	ExamName           string
	ExamType           string
	Term               string
	ExamDate           string
	Status             string
	RosterCount        int
	ScoreCount         int
	CutoffCount        int
	TargetCount        int
	GroupCount         int
	BlockingIssues     int
	WarningIssues      int
	HasOfficialRun     bool
	OfficialStale      bool
	GradeAverage       float64
	ResultRowCount     int
	PersonalScoreCount int
}

// WorkspaceExamSnapshot 是总览时间线中单次考试摘要。
type WorkspaceExamSnapshot struct {
	ExamID            int64
	AssessmentID      int64
	ExamName          string
	ExamType          string
	Term              string
	ExamDate          string
	Status            string
	RosterCount       int
	Calculated        bool
	AverageTotal      float64
	AverageCompletion float64
}

// WorkspaceSummary 聚合工作区实时台账。
type WorkspaceSummary struct {
	CohortID               int64
	StageID                int64
	AcademicYearStart      int
	GradeLevel             string
	Counts                 StageCounts
	ActiveStudents         int
	PhysicsStudents        int
	HistoryStudents        int
	InactiveStudents       int
	BaselineCoveredClasses int
	BaselineSubjectCount   int
	GradeBaselineTotal     float64
	ExamCount              int
	CalculatedExamCount    int
	BlockingIssues         int
	WarningIssues          int
	LatestAssessment       *WorkspaceAssessmentSnapshot
	Exams                  []WorkspaceExamSnapshot
}

// CohortOverviewCard 描述全校总览中单个在读年级的态势摘要卡。
type CohortOverviewCard struct {
	Cohort                 Cohort
	ClassCount             int
	ActiveStudents         int
	TeacherCount           int
	AssignmentCount        int
	BaselineCount          int
	ExamCount              int
	LatestExamID           int64
	LatestExamName         string
	LatestExamDate         string
	LatestExamStatus       string
	LatestAssessmentID     int64
	LatestAssessmentStatus string
	HasOfficialRun         bool
	BlockingIssues         int
}

// SchoolOverview 汇总全校在读年级态势、已毕业届数与全校近期考试。
type SchoolOverview struct {
	ActiveCohorts     []CohortOverviewCard
	GraduatedCount    int
	RecentExams       []Exam
	LatestAssessments []AssessmentOverview
}

// TeachingAssignmentRow 描述一条任课关系（关联教学班、行政班与教师信息）。
type TeachingAssignmentRow struct {
	ID                int64
	CohortID          int64
	Phase             string
	TeachingGroupID   int64
	TeachingGroupName string
	GroupKind         string
	AdminClassID      int64
	ClassName         string
	Subject           string
	TeacherID         int64
	TeacherNo         string
	TeacherName       string
	Role              string
	Status            string
}

// ExamImportRequest 是一次考试数据导入请求。
type ExamImportRequest struct {
	ExamID   int64
	CohortID int64
	Kind     string
	Data     ImportData
	Report   ImportReport
	Mapping  map[string]string
	Issues   []ImportIssue
}
