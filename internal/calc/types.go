// Package calc 按考核方案计算班级指标、排名、赋分和教师个人得分。
package calc

import "tally/internal/assessment"

// Input 是一次核算需要的全部批次数据。
type Input struct {
	Grade    string
	ExamType string
	Cohort   string
	Plan     assessment.Config

	Classes          []AdminClass
	TeachingGroups   []TeachingGroup
	StatGroups       []StatGroup
	Students         []Student
	ClassTargets     []ClassTarget
	ActualTierCounts map[string]map[string]float64
	Assignments      []Assignment
	References       map[string]SubjectReference
}

// AdminClass 是一个行政班。
type AdminClass struct {
	ID    string
	Name  string
	Grade string
}

// TeachingGroup 是一个学科教学班。
type TeachingGroup struct {
	ID           string
	Name         string
	Subject      string
	AdminClassID string
}

// StatGroup 是单科达线率的计算单位，可以包含一个或者多个教学班。
type StatGroup struct {
	ID                     string
	Name                   string
	Subject                string
	LeadClassID            string
	MemberTeachingGroupIDs []string
	Target                 float64
	BaselineAverage        float64
	BaselineRatio          float64
}

// Student 是一名参加核算的学生。
type Student struct {
	ID             string
	Name           string
	AdminClassID   string
	Track          string
	Registered     bool
	Status         string
	Flags          []string
	Scores         map[string]float64
	TeachingGroups map[string]string
}

// Participant 是一名参与某统计组某学科计算的学生，随结果保存为快照。
type Participant struct {
	StudentNo string
	Name      string
}

// ClassTarget 是一个行政班的分档目标人数。
type ClassTarget struct {
	ClassID string
	Counts  map[string]*float64
}

// Assignment 是一名教师在一个教学班的任课关系。
type Assignment struct {
	TeacherID       string
	TeacherName     string
	TeachingGroupID string
}

// SubjectReference 是年级层面的学科参照值。
type SubjectReference struct {
	GradeAverage         float64
	GradeBaselineAverage float64
	Cutoff               float64
	Cutoffs              map[string]float64
}

// Options 控制核算时的特殊处理。
type Options struct {
	ExcludedCompletionClasses map[string]bool
}

// Row 是一个统计组在某一学科下的计算结果。
type Row struct {
	StatGroupID   string
	StatGroupName string
	Subject       string
	LeadClassID   string
	LeadClassName string
	TeacherIDs    []string
	TeacherNames  []string

	ClassAverage    float64
	ClassCount      int
	GradeAverage    float64
	BaselineAverage float64
	BaselineRatio   float64

	ImprovementRaw   float64
	ImprovementScore float64
	PassCount        int
	PassTarget       float64
	PassRaw          float64
	PassScore        float64

	CompletionActual        float64
	CompletionBase          float64
	CompletionRaw           float64
	CompletionScore         float64
	CompletionParticipating bool

	MemberCount     int
	Total           float64
	Rank            int
	TierIndex       int
	Points          float64
	Participants    []Participant
	IndicatorValues map[string]float64
}

// PersonalScore 是一名教师在一个年级和学科下的个人得分。
type PersonalScore struct {
	TeacherID   string
	TeacherName string
	Grade       string
	Subject     string
	GroupPoints []float64
	Score       float64
}

// Result 是一次核算的输出。
type Result struct {
	Rows                      []Row
	Personal                  []PersonalScore
	ExcludedCompletionClasses []string
}

// ZeroTargetIssue 描述一个目标基础分为零或者目标缺失的班级。
type ZeroTargetIssue struct {
	ClassID      string
	ClassName    string
	AllTiers     []string
	MissingTiers []string
	ExplicitZero bool
}

// ZeroTargetError 表示计算前需要补齐目标。
type ZeroTargetError struct {
	Issues []ZeroTargetIssue
}

// Error 返回可直接显示的零目标错误。
func (e *ZeroTargetError) Error() string {
	return "目标基础分为零或者目标缺失，需要补齐后重新计算"
}
