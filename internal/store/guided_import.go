package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrDuplicateImport 表示同一个文件已经导入过同一类数据。
var ErrDuplicateImport = errors.New("该文件已经导入过这类数据")

// ErrImportBlocked 表示导入存在阻断问题，不能写入。
var ErrImportBlocked = errors.New("导入存在阻断问题")

// ValidateStageImport 在写入前检查一个阶段数据导入，返回新增、更新、跳过和问题。
func (s *Store) ValidateStageImport(ctx context.Context, cohortID, stageID int64, req StageImportRequest) (ImportPreview, error) {
	stage, err := s.stageByID(ctx, cohortID, stageID)
	if err != nil {
		return ImportPreview{}, err
	}
	preview := ImportPreview{Kind: req.Kind}
	preview.Issues = append(preview.Issues, req.Issues...)
	switch req.Kind {
	case "classes":
		err = s.validateClassImport(ctx, stage, req, &preview)
	case "students":
		err = s.validateRosterImport(ctx, stage, req, &preview)
	case "teachers":
		err = s.validateTeacherImport(ctx, stage, req, &preview)
	case "assignments":
		err = s.validateAssignmentImport(ctx, stage, req, &preview)
	case "baselines":
		err = s.validateBaselineImport(ctx, stage, req, &preview)
	default:
		err = fmt.Errorf("未知的数据项 %q", req.Kind)
	}
	if err != nil {
		return ImportPreview{}, err
	}
	for _, issue := range preview.Issues {
		switch issue.Severity {
		case "blocking":
			preview.Blocking++
		case "warning":
			preview.Warnings++
		}
	}
	return preview, nil
}

func (s *Store) validateClassImport(ctx context.Context, stage migrationBatch, req StageImportRequest, preview *ImportPreview) error {
	existing, err := s.stageClassNameSet(ctx, stage.StageID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for index, raw := range req.Data.Classes {
		name := normalizeClassName(raw)
		if name == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: index + 2, Column: "class_name", Code: "missing_field",
			})
			continue
		}
		if seen[name] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: index + 2, Column: "class_name",
				Code: "duplicate_class", Args: []string{name},
			})
			preview.Skip++
			continue
		}
		seen[name] = true
		if existing[name] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	return nil
}

func (s *Store) validateRosterImport(ctx context.Context, stage migrationBatch, req StageImportRequest, preview *ImportPreview) error {
	existing, err := s.studentNoSet(ctx)
	if err != nil {
		return err
	}
	members, err := s.stageStudentNoSet(ctx, stage.StageID)
	if err != nil {
		return err
	}
	classes, err := s.stageClassNameSet(ctx, stage.StageID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	newClasses := map[string]bool{}
	for _, student := range req.Data.Students {
		switch {
		case student.StudentNo == "":
			preview.Issues = append(preview.Issues, missingIssue(student.SourceRow, "student_no"))
			continue
		case student.Name == "":
			preview.Issues = append(preview.Issues, missingIssue(student.SourceRow, "name"))
			continue
		case student.ClassName == "":
			preview.Issues = append(preview.Issues, missingIssue(student.SourceRow, "class_name"))
			continue
		}
		if !classes[student.ClassName] {
			newClasses[student.ClassName] = true
		}
		if seen[student.StudentNo] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: student.SourceRow, Column: "student_no",
				Code: "duplicate_student", Args: []string{student.StudentNo},
			})
			continue
		}
		seen[student.StudentNo] = true
		if existing[student.StudentNo] || members[student.StudentNo] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	if len(newClasses) > 0 {
		preview.Issues = append(preview.Issues, ImportIssue{
			Severity: "warning", Column: "class_name",
			Code: "classes_created", Args: []string{strconv.Itoa(len(newClasses))},
		})
	}
	return nil
}

func (s *Store) validateTeacherImport(ctx context.Context, stage migrationBatch, req StageImportRequest, preview *ImportPreview) error {
	subjects, err := s.subjectFullMarks(ctx)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `SELECT teacher_no, name, subject FROM teachers`)
	if err != nil {
		return err
	}
	existingNo := map[string]string{}
	existingName := map[string]bool{}
	for _, row := range rows.Values {
		no, _ := row[0].(string)
		name, _ := row[1].(string)
		subject, _ := row[2].(string)
		if no != "" {
			existingNo[no] = name
		} else {
			existingName[name+"\x00"+subject] = true
		}
	}
	seenNo := map[string]string{}
	seenName := map[string]bool{}
	for _, teacher := range req.Data.TeacherList {
		if teacher.Name == "" {
			preview.Issues = append(preview.Issues, missingIssue(teacher.SourceRow, "teacher_name"))
			continue
		}
		if teacher.TeacherNo == "" && teacher.Subject == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: teacher.SourceRow, Column: "teacher_no",
				Code: "teacher_identity_missing", Args: []string{teacher.Name},
			})
			continue
		}
		if teacher.Subject != "" {
			if _, ok := subjects[teacher.Subject]; !ok {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: teacher.SourceRow, Column: "subject",
					Code: "unknown_subject", Args: []string{teacher.Subject},
				})
				continue
			}
		}
		if teacher.TeacherNo != "" {
			if previous, ok := seenNo[teacher.TeacherNo]; ok && previous != teacher.Name {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: teacher.SourceRow, Column: "teacher_no",
					Code: "duplicate_teacher_no", Args: []string{teacher.TeacherNo},
				})
				continue
			}
			seenNo[teacher.TeacherNo] = teacher.Name
			if existing, ok := existingNo[teacher.TeacherNo]; ok {
				if existing != teacher.Name {
					preview.Issues = append(preview.Issues, ImportIssue{
						Severity: "blocking", Row: teacher.SourceRow, Column: "teacher_no",
						Code: "teacher_no_conflict", Args: []string{teacher.TeacherNo, existing},
					})
					continue
				}
				preview.Update++
				continue
			}
			preview.New++
			continue
		}
		key := teacher.Name + "\x00" + teacher.Subject
		if seenName[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: teacher.SourceRow, Column: "teacher_name",
				Code: "teacher_name_conflict", Args: []string{teacher.Name, teacher.Subject},
			})
			continue
		}
		seenName[key] = true
		if existingName[key] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	return nil
}

func (s *Store) validateAssignmentImport(ctx context.Context, stage migrationBatch, req StageImportRequest, preview *ImportPreview) error {
	subjects, err := s.subjectFullMarks(ctx)
	if err != nil {
		return err
	}
	classes, err := s.stageClassNameSet(ctx, stage.StageID)
	if err != nil {
		return err
	}
	existing, err := s.assignmentKeySet(ctx, stage.StageID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	createdClasses := make([]string, 0)
	seenClass := map[string]bool{}
	for _, assignment := range req.Data.Assignments {
		switch {
		case assignment.TeacherName == "":
			preview.Issues = append(preview.Issues, missingIssue(assignment.SourceRow, "teacher_name"))
			continue
		case assignment.ClassName == "":
			preview.Issues = append(preview.Issues, missingIssue(assignment.SourceRow, "class_name"))
			continue
		case assignment.Subject == "":
			preview.Issues = append(preview.Issues, missingIssue(assignment.SourceRow, "subject"))
			continue
		}
		if _, ok := subjects[assignment.Subject]; !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: assignment.SourceRow, Column: "subject",
				Code: "unknown_subject", Args: []string{assignment.Subject},
			})
			continue
		}
		className := normalizeClassName(assignment.ClassName)
		key := assignment.TeacherName + "\x00" + className + "\x00" + assignment.Subject
		if seen[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: assignment.SourceRow, Column: "teacher_name",
				Code: "duplicate_assignment",
				Args: []string{assignment.TeacherName, className, assignment.Subject},
			})
			preview.Skip++
			continue
		}
		seen[key] = true
		if existing[key] {
			preview.Skip++
			continue
		}
		if !classes[className] && !seenClass[className] {
			seenClass[className] = true
			createdClasses = append(createdClasses, className)
		}
		if !classes[className] {
			preview.New++
			continue
		}
		preview.New++
	}
	if len(createdClasses) > 0 {
		preview.Issues = append(preview.Issues, ImportIssue{
			Severity: "warning", Column: "class_name",
			Code: "classes_created", Args: []string{strconv.Itoa(len(createdClasses))},
		})
	}
	return nil
}

func (s *Store) validateBaselineImport(ctx context.Context, stage migrationBatch, req StageImportRequest, preview *ImportPreview) error {
	subjects, err := s.subjectFullMarks(ctx)
	if err != nil {
		return err
	}
	classes, err := s.stageClassNameSet(ctx, stage.StageID)
	if err != nil {
		return err
	}
	existing, err := s.baselineKeySet(ctx, stage.StageID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, baseline := range req.Data.ClassBaselines {
		className := normalizeClassName(baseline.ClassName)
		if className == "" {
			preview.Issues = append(preview.Issues, missingIssue(baseline.SourceRow, "class_name"))
			continue
		}
		if !classes[className] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: baseline.SourceRow, Column: "class_name",
				Code: "unknown_class", Args: []string{className},
			})
			continue
		}
		fullMark, ok := subjects[baseline.Subject]
		if !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: baseline.SourceRow, Column: "subject",
				Code: "unknown_subject", Args: []string{baseline.Subject},
			})
			continue
		}
		if baseline.Average < 0 || baseline.Average > fullMark {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: baseline.SourceRow, Column: "subject",
				Code: "score_out_of_range",
				Args: []string{className, baseline.Subject, formatFullMark(fullMark)},
			})
			continue
		}
		if baseline.Ratio != nil && *baseline.Ratio <= 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: baseline.SourceRow, Column: "ratio",
				Code: "ratio_not_positive",
			})
			continue
		}
		key := className + "\x00" + baseline.Subject
		if seen[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: baseline.SourceRow, Column: "subject",
				Code: "duplicate_baseline", Args: []string{className, baseline.Subject},
			})
			continue
		}
		seen[key] = true
		if existing[key] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	return nil
}

func missingIssue(row int, column string) ImportIssue {
	return ImportIssue{Severity: "blocking", Row: row, Column: column, Code: "missing_field"}
}

// formatFullMark 去掉整数满分的小数点，例如 150.0 显示为 150。
func formatFullMark(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (s *Store) stageClassNameSet(ctx context.Context, stageID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT c.name FROM classes c
		JOIN class_stage_info i ON i.class_id = c.id WHERE i.stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		result[name] = true
	}
	return result, nil
}

func (s *Store) studentNoSet(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT student_no FROM students`)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		no, _ := row[0].(string)
		result[no] = true
	}
	return result, nil
}

func (s *Store) stageStudentNoSet(ctx context.Context, stageID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT s.student_no FROM students s
		JOIN student_stage_memberships m ON m.student_id = s.id WHERE m.stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		no, _ := row[0].(string)
		result[no] = true
	}
	return result, nil
}

func (s *Store) assignmentKeySet(ctx context.Context, stageID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT t.name, g.name, g.subject FROM assignments a
		JOIN teachers t ON t.id = a.teacher_id
		JOIN teaching_groups g ON g.id = a.teaching_group_id
		WHERE g.stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		teacher, _ := row[0].(string)
		className, _ := row[1].(string)
		subject, _ := row[2].(string)
		result[teacher+"\x00"+className+"\x00"+subject] = true
	}
	return result, nil
}

func (s *Store) baselineKeySet(ctx context.Context, stageID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT c.name, a.subject FROM baseline_aggregates a
		JOIN classes c ON c.id = a.scope_id
		WHERE a.stage_id = ? AND a.scope_type = 'class'`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		subject, _ := row[1].(string)
		result[name+"\x00"+subject] = true
	}
	return result, nil
}

func (s *Store) subjectFullMarks(ctx context.Context) (map[string]float64, error) {
	rows, err := s.db.Query(ctx, `SELECT "key", full_mark FROM subjects WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	result := map[string]float64{}
	for _, row := range rows.Values {
		key, _ := row[0].(string)
		fullMark, _ := row[1].(float64)
		result[key] = fullMark
	}
	return result, nil
}

func issueCodeSummary(issues []ImportIssue) string {
	for _, issue := range issues {
		if issue.Severity == "blocking" {
			return issue.Code
		}
	}
	return strings.Join([]string{"blocking"}, "")
}
