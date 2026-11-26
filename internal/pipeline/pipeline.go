// Package pipeline 把本次考核的数据转换为计算引擎输入，并保存核算结果。
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"tally/internal/assessment"
	"tally/internal/calc"
	"tally/internal/exporter"
	"tally/internal/store"
)

// Issue 描述一个阻断核算的数据问题。
type Issue struct {
	Kind    string
	Message string
}

// ValidationError 汇总一次核算前的数据问题。
type ValidationError struct {
	Issues []Issue
}

// Error 返回可直接显示的错误。
func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, issue.Message)
	}
	return "核算前校验失败: " + strings.Join(parts, "；")
}

// BuildAssessmentInput 读取本次考核的数据并生成计算引擎输入与输入摘要。
func BuildAssessmentInput(ctx context.Context, storage *store.Store, assessmentID int64, plan assessment.Config) (calc.Input, store.BatchData, string, map[string]string, error) {
	data, err := storage.LoadAssessmentData(ctx, assessmentID)
	if err != nil {
		return calc.Input{}, store.BatchData{}, "", nil, err
	}
	input, issues := build(data, plan)
	if len(issues) > 0 {
		return calc.Input{}, data, "", nil, &ValidationError{Issues: issues}
	}
	hash, err := store.InputHash(plan, data)
	if err != nil {
		return calc.Input{}, data, "", nil, err
	}
	summary, err := store.InputSummary(plan, data)
	if err != nil {
		return calc.Input{}, data, "", nil, err
	}
	return input, data, hash, summary, nil
}

// CalculateAssessment 按选定的方案版本正式核算本次考核并保存结果。
func CalculateAssessment(ctx context.Context, storage *store.Store, assessmentID, versionID int64) (calc.Result, error) {
	return calculateAssessment(ctx, storage, assessmentID, versionID, true)
}

// TrialAssessment 按选定的方案版本试算本次考核：结果按非正式保存，
// 不改变考核状态和当前正式结果。
func TrialAssessment(ctx context.Context, storage *store.Store, assessmentID, versionID int64) (calc.Result, error) {
	return calculateAssessment(ctx, storage, assessmentID, versionID, false)
}

func calculateAssessment(ctx context.Context, storage *store.Store, assessmentID, versionID int64, official bool) (calc.Result, error) {
	if err := storage.AssertAssessmentCalculable(ctx, assessmentID); err != nil {
		return calc.Result{}, err
	}
	version, err := storage.GetVersion(ctx, versionID)
	if err != nil {
		return calc.Result{}, err
	}
	if version.Status != "published" {
		return calc.Result{}, fmt.Errorf("核算只能选择已发布的方案版本")
	}
	input, data, hash, summary, err := BuildAssessmentInput(ctx, storage, assessmentID, version.Config)
	if err != nil {
		return calc.Result{}, err
	}
	excluded := make(map[string]bool, len(data.ExcludedCompletionClasses))
	for classID := range data.ExcludedCompletionClasses {
		excluded[strconv.FormatInt(classID, 10)] = true
	}
	result, err := calc.Calculate(input, calc.Options{ExcludedCompletionClasses: excluded})
	if err != nil {
		return calc.Result{}, err
	}
	if _, err := storage.SaveAssessmentRun(ctx, assessmentID, versionID, hash, summary, result, official); err != nil {
		return calc.Result{}, err
	}
	return result, nil
}

// ExportAssessment 把本次考核最新的正式核算结果写成工作簿，并记录导出文件。
func ExportAssessment(ctx context.Context, storage *store.Store, assessmentID int64, path string) error {
	run, rows, personal, err := storage.LatestOfficialAssessmentRun(ctx, assessmentID)
	if err != nil {
		return err
	}
	context, err := storage.LoadAssessmentContext(ctx, assessmentID)
	if err != nil {
		return err
	}
	version, err := storage.GetVersion(ctx, run.PlanVersionID)
	if err != nil {
		return err
	}
	archived, err := storage.AssessmentArchived(ctx, assessmentID)
	if err != nil {
		return err
	}
	// 归档考核只有最终结果，没有可以重建的输入，导出直接使用保存的结果。
	if !archived {
		_, _, currentHash, _, err := BuildAssessmentInput(ctx, storage, assessmentID, version.Config)
		if err != nil {
			return err
		}
		if currentHash != run.InputHash {
			return fmt.Errorf("本次考核的数据或者方案已经变化，需要重新核算后才能导出")
		}
	}
	data, err := storage.LoadAssessmentData(ctx, assessmentID)
	if err != nil {
		return err
	}
	batch := store.Batch{
		AcademicYear: context.AcademicYear,
		Semester:     context.Exam.Term,
		ExamType:     context.Exam.ExamType,
		Grade:        context.Grade,
		Status:       "calculated",
		Revision:     context.Assessment.Revision,
	}
	if err := exporter.Write(exporter.Input{
		Path:        path,
		Batch:       batch,
		Plan:        version,
		Run:         run,
		Rows:        rows,
		Personal:    personal,
		Students:    data.Students,
		Classes:     data.Classes,
		GeneratedAt: time.Now(),
	}); err != nil {
		return err
	}
	hash, err := fileHash(path)
	if err != nil {
		return err
	}
	return storage.SaveExport(ctx, run.ID, path, hash)
}

func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func build(data store.BatchData, plan assessment.Config) (calc.Input, []Issue) {
	var issues []Issue
	add := func(kind, message string) {
		issues = append(issues, Issue{Kind: kind, Message: message})
	}
	if len(data.Classes) == 0 {
		add("missing_classes", "批次没有班级")
	}
	if len(data.Students) == 0 {
		add("missing_students", "批次没有学生")
	}
	if len(data.StatGroups) == 0 {
		add("missing_stat_groups", "批次没有统计组")
	}
	validStudents := make([]store.Student, 0, len(data.Students))
	for _, student := range data.Students {
		converted := toCalcStudent(student)
		if calc.IsValidStudent(converted, plan.Data) {
			validStudents = append(validStudents, student)
		}
	}
	if len(validStudents) == 0 {
		add("missing_students", "批次没有有效考生")
	}
	gradeAverages := gradeAverages(validStudents)
	subjectTracks := subjectTracks(data)
	references := map[string]calc.SubjectReference{}
	for subject, average := range gradeAverages {
		baseline := data.GradeBaselines[subject]
		if baseline <= 0 {
			baseline = weightedGradeBaseline(data, subject)
		}
		if baseline <= 0 {
			add("missing_grade_baseline", fmt.Sprintf("学科 %s 缺少年级基数均分", subject))
			continue
		}
		cutoffs := map[string]float64{}
		for _, item := range data.SubjectCutoffs {
			if item.Subject != subject {
				continue
			}
			cutoffs[item.Track] = item.Value
		}
		reference := calc.SubjectReference{
			GradeAverage:         average,
			GradeBaselineAverage: baseline,
			Cutoffs:              cutoffs,
		}
		if len(cutoffs) == 1 {
			for _, value := range cutoffs {
				reference.Cutoff = value
			}
		}
		if len(cutoffs) == 0 {
			add("missing_subject_cutoff", fmt.Sprintf("学科 %s 缺少分数线", subject))
			continue
		}
		for _, track := range subjectTracks[subject] {
			if _, ok := cutoffs[track]; !ok && len(cutoffs) == 1 {
				continue
			}
			if _, ok := cutoffs[track]; !ok {
				add("missing_subject_cutoff", fmt.Sprintf("学科 %s 缺少 %s 类分数线", subject, track))
			}
		}
		references[subject] = reference
	}
	tierCutoffs := map[string]map[string]float64{}
	for _, item := range data.TierCutoffs {
		if tierCutoffs[item.TierKey] == nil {
			tierCutoffs[item.TierKey] = map[string]float64{}
		}
		tierCutoffs[item.TierKey][item.Track] = item.Value
	}
	hasActualCounts := func(tierKey string) bool {
		for _, target := range data.ClassTargets {
			if target.ActualCounts != nil && target.ActualCounts[tierKey] != nil {
				return true
			}
		}
		return false
	}
	for _, tier := range plan.Tiers {
		if len(tierCutoffs[tier.Key]) == 0 {
			if data.Batch.ExamType == "gaokao" && hasActualCounts(tier.Key) {
				continue
			}
			add("missing_tier_cutoff", fmt.Sprintf("档位 %s 缺少总分分数线", tier.Label))
		}
	}
	studentTracks := map[string]bool{}
	for _, student := range validStudents {
		studentTracks[normalizeTrack(student)] = true
	}
	for _, tier := range plan.Tiers {
		for track := range studentTracks {
			if tierCutoffs[tier.Key][track] <= 0 {
				if data.Batch.ExamType == "gaokao" && hasActualCounts(tier.Key) {
					continue
				}
				add("missing_tier_cutoff", fmt.Sprintf("档位 %s 缺少 %s 类总分分数线", tier.Label, track))
			}
		}
	}
	actualCounts := actualTierCounts(data, plan, tierCutoffs, &issues)
	classes := make([]calc.AdminClass, 0, len(data.Classes))
	for _, class := range data.Classes {
		classes = append(classes, calc.AdminClass{
			ID:    strconv.FormatInt(class.ID, 10),
			Name:  class.Name,
			Grade: data.Batch.Grade,
		})
	}
	teachingGroups := make([]calc.TeachingGroup, 0, len(data.TeachingGroups))
	for _, group := range data.TeachingGroups {
		teachingGroups = append(teachingGroups, calc.TeachingGroup{
			ID:           strconv.FormatInt(group.ID, 10),
			Name:         group.Name,
			Subject:      group.Subject,
			AdminClassID: strconv.FormatInt(group.ClassID, 10),
		})
	}
	statGroups := make([]calc.StatGroup, 0, len(data.StatGroups))
	targetClasses := map[int64]bool{}
	for _, target := range data.ClassTargets {
		targetClasses[target.ClassID] = true
	}
	assignedGroups := map[int64]bool{}
	for _, assignment := range data.Assignments {
		assignedGroups[assignment.TeachingGroupID] = true
	}
	for _, group := range data.StatGroups {
		participates := targetClasses[group.LeadClassID]
		for _, memberID := range group.MemberGroupIDs {
			if assignedGroups[memberID] {
				participates = true
				break
			}
		}
		if !participates {
			continue
		}
		members := make([]string, 0, len(group.MemberGroupIDs))
		for _, id := range group.MemberGroupIDs {
			members = append(members, strconv.FormatInt(id, 10))
		}
		if len(members) == 0 {
			add("missing_stat_member", fmt.Sprintf("统计组 %s %s 没有成员教学班", group.Subject, group.Name))
			continue
		}
		if !statGroupHasStudents(data, group, plan.Data) {
			continue
		}
		baseline := group.BaselineAverage
		ratio := group.BaselineRatio
		if baseline <= 0 && ratio <= 0 {
			leadKey := strconv.FormatInt(group.LeadClassID, 10)
			if r, ok := data.ClassBaselineRatios[group.Subject][leadKey]; ok && r > 0 {
				ratio = r
			} else if b, ok := data.ClassBaselines[group.Subject][leadKey]; ok && b > 0 {
				baseline = b
			} else {
				baseline = statGroupBaseline(data, group)
			}
		}
		if baseline <= 0 && ratio <= 0 {
			add("missing_stat_baseline", fmt.Sprintf("统计组 %s %s 缺少基数均分", group.Subject, group.Name))
			continue
		}
		if group.Target < 0 {
			add("missing_stat_target", fmt.Sprintf("统计组 %s %s 单科目标不能为负数", group.Subject, group.Name))
			continue
		}
		statGroups = append(statGroups, calc.StatGroup{
			ID:                     strconv.FormatInt(group.ID, 10),
			Name:                   group.Name,
			Subject:                group.Subject,
			LeadClassID:            strconv.FormatInt(group.LeadClassID, 10),
			MemberTeachingGroupIDs: members,
			Target:                 group.Target,
			BaselineAverage:        baseline,
			BaselineRatio:          ratio,
		})
	}
	students := make([]calc.Student, 0, len(validStudents))
	for _, student := range validStudents {
		students = append(students, toCalcStudent(student))
	}
	targets := make([]calc.ClassTarget, 0, len(data.ClassTargets))
	for _, target := range data.ClassTargets {
		counts := make(map[string]*float64, len(target.Counts))
		for key, value := range target.Counts {
			copied := *value
			counts[key] = &copied
		}
		targets = append(targets, calc.ClassTarget{
			ClassID: strconv.FormatInt(target.ClassID, 10),
			Counts:  counts,
		})
	}
	assignments := make([]calc.Assignment, 0, len(data.Assignments))
	for _, assignment := range data.Assignments {
		assignments = append(assignments, calc.Assignment{
			TeacherID:       strconv.FormatInt(assignment.TeacherID, 10),
			TeacherName:     assignment.TeacherName,
			TeachingGroupID: strconv.FormatInt(assignment.TeachingGroupID, 10),
		})
	}
	excluded := map[string]bool{}
	for classID := range data.ExcludedCompletionClasses {
		excluded[strconv.FormatInt(classID, 10)] = true
	}
	return calc.Input{
		Grade:            data.Batch.Grade,
		ExamType:         data.Batch.ExamType,
		Plan:             plan,
		Classes:          classes,
		TeachingGroups:   teachingGroups,
		StatGroups:       statGroups,
		Students:         students,
		ClassTargets:     targets,
		ActualTierCounts: actualCounts,
		Assignments:      assignments,
		References:       references,
	}, issues
}

func toCalcStudent(student store.Student) calc.Student {
	flags := make([]string, 0, len(student.ScoreFlags))
	for _, flag := range student.ScoreFlags {
		if flag != "" {
			flags = append(flags, flag)
		}
	}
	teachingGroups := make(map[string]string, len(student.TeachingGroups))
	for subject, id := range student.TeachingGroups {
		teachingGroups[subject] = strconv.FormatInt(id, 10)
	}
	return calc.Student{
		ID:             student.StudentNo,
		Name:           student.Name,
		AdminClassID:   strconv.FormatInt(student.ClassID, 10),
		Track:          normalizeTrack(student),
		Registered:     true,
		Status:         student.Status,
		Flags:          flags,
		Scores:         student.Scores,
		TeachingGroups: teachingGroups,
	}
}

func gradeAverages(students []store.Student) map[string]float64 {
	sums := map[string]float64{}
	counts := map[string]int{}
	for _, student := range students {
		for subject, score := range student.Scores {
			if score <= 0 {
				continue
			}
			sums[subject] += score
			counts[subject]++
		}
	}
	averages := make(map[string]float64, len(sums))
	for subject, sum := range sums {
		averages[subject] = sum / float64(counts[subject])
	}
	return averages
}

func weightedGradeBaseline(data store.BatchData, subject string) float64 {
	headcounts := map[int64]int{}
	for _, student := range data.Students {
		headcounts[student.ClassID]++
	}
	total := 0.0
	count := 0
	for classID, value := range data.ClassBaselines[subject] {
		parsed, err := strconv.ParseInt(classID, 10, 64)
		if err != nil {
			continue
		}
		weight := headcounts[parsed]
		if weight == 0 {
			weight = 1
		}
		total += value * float64(weight)
		count += weight
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func statGroupBaseline(data store.BatchData, group store.StatGroup) float64 {
	total := 0.0
	count := 0
	members := map[int64]bool{}
	for _, id := range group.MemberGroupIDs {
		members[id] = true
	}
	for _, teachingGroup := range data.TeachingGroups {
		if !members[teachingGroup.ID] {
			continue
		}
		value := data.ClassBaselines[group.Subject][strconv.FormatInt(teachingGroup.ClassID, 10)]
		if value <= 0 {
			continue
		}
		total += value
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func statGroupHasStudents(data store.BatchData, group store.StatGroup, config assessment.DataConfig) bool {
	members := map[int64]bool{}
	for _, id := range group.MemberGroupIDs {
		members[id] = true
	}
	for _, student := range data.Students {
		if !calc.IsValidStudent(toCalcStudent(student), config) {
			continue
		}
		if _, ok := student.Scores[group.Subject]; !ok {
			continue
		}
		if members[student.TeachingGroups[group.Subject]] {
			return true
		}
	}
	return false
}

func subjectTracks(data store.BatchData) map[string][]string {
	tracks := map[string]map[string]bool{}
	for _, student := range data.Students {
		track := normalizeTrack(student)
		for subject := range student.Scores {
			if tracks[subject] == nil {
				tracks[subject] = map[string]bool{}
			}
			tracks[subject][track] = true
		}
	}
	result := make(map[string][]string, len(tracks))
	for subject, values := range tracks {
		list := make([]string, 0, len(values))
		for track := range values {
			list = append(list, track)
		}
		sort.Strings(list)
		result[subject] = list
	}
	return result
}

func actualTierCounts(data store.BatchData, plan assessment.Config, cutoffs map[string]map[string]float64, issues *[]Issue) map[string]map[string]float64 {
	hasExplicitActuals := false
	for _, target := range data.ClassTargets {
		if len(target.ActualCounts) > 0 {
			hasExplicitActuals = true
			break
		}
	}
	if hasExplicitActuals {
		result := map[string]map[string]float64{}
		for _, target := range data.ClassTargets {
			classID := strconv.FormatInt(target.ClassID, 10)
			if result[classID] == nil {
				result[classID] = map[string]float64{}
			}
			for tierKey, actual := range target.ActualCounts {
				if actual != nil {
					result[classID][tierKey] = *actual
				}
			}
		}
		return result
	}
	valid := map[int64]bool{}
	for _, student := range data.Students {
		if calc.IsValidStudent(toCalcStudent(student), plan.Data) {
			valid[student.ID] = true
		}
	}
	result := map[string]map[string]float64{}
	for _, student := range data.Students {
		if !valid[student.ID] || student.TotalScore == nil {
			continue
		}
		classID := strconv.FormatInt(student.ClassID, 10)
		if result[classID] == nil {
			result[classID] = map[string]float64{}
		}
		for _, tier := range plan.Tiers {
			track := normalizeTrack(student)
			cutoff := cutoffs[tier.Key][track]
			if cutoff <= 0 {
				continue
			}
			if *student.TotalScore >= cutoff {
				result[classID][tier.Key]++
				break
			}
		}
	}
	return result
}

func normalizeTrack(student store.Student) string {
	if student.Track == "physics" || student.Track == "history" {
		return student.Track
	}
	if strings.Contains(student.Elective, "历史") || strings.Contains(student.Elective, "史") || strings.Contains(student.Elective, "历") {
		return "history"
	}
	if strings.Contains(student.Elective, "物理") || strings.Contains(student.Elective, "物") {
		return "physics"
	}
	if student.Track != "" {
		return student.Track
	}
	return "physics"
}
