package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

const (
	ExamImportRoster  = "exam_roster"
	ExamImportScores  = "exam_scores"
	ExamImportCutoffs = "exam_cutoffs"
	ExamImportTargets = "exam_targets"

	ExamDataRoster  = "roster"
	ExamDataScores  = "scores"
	ExamDataCutoffs = "cutoffs"
	ExamDataTargets = "targets"
	ExamDataGroups  = "groups"
)

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

// ValidateExamImport 在写入前检查考试数据导入。
func (s *Store) ValidateExamImport(ctx context.Context, req ExamImportRequest) (ImportPreview, error) {
	examCohort, err := s.GetExamCohort(ctx, req.ExamID, req.CohortID)
	if err != nil {
		return ImportPreview{}, err
	}
	preview := ImportPreview{Kind: req.Kind}
	preview.Issues = append(preview.Issues, req.Issues...)
	switch req.Kind {
	case ExamImportRoster:
		err = s.validateExamRoster(ctx, examCohort, req, &preview)
	case ExamImportScores:
		err = s.validateExamScores(ctx, examCohort, req, &preview)
	case ExamImportCutoffs:
		err = s.validateExamCutoffs(ctx, examCohort, req, &preview)
	case ExamImportTargets:
		err = s.validateExamTargets(ctx, examCohort, req, &preview)
	default:
		err = fmt.Errorf("未知的考试数据项 %q", req.Kind)
	}
	if err != nil {
		return ImportPreview{}, err
	}
	for _, issue := range preview.Issues {
		if issue.Severity == "blocking" {
			preview.Blocking++
		} else {
			preview.Warnings++
		}
	}
	return preview, nil
}

// AddExamRosterStudents 把成绩单里不在报名名单的考生补建为学生并加入本场考试报名名单。
// 已有阶段归属的学生沿用原有班级与类别；返回新增人数与因缺少班级或姓名跳过的人数。
func (s *Store) AddExamRosterStudents(ctx context.Context, examCohort ExamCohort, entries []ImportStudent) (int, int, error) {
	roster, err := s.examRosterUserIndex(ctx, examCohort)
	if err != nil {
		return 0, 0, err
	}
	classes, err := s.stageClassMap(ctx, examCohort.StageID)
	if err != nil {
		return 0, 0, err
	}
	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	added := 0
	skipped := 0
	createdClass := map[string]bool{}
	for _, entry := range entries {
		if entry.StudentNo == "" || entry.Name == "" {
			skipped++
			continue
		}
		if _, ok := roster.byStudentNo[entry.StudentNo]; ok {
			continue
		}
		className := normalizeClassName(entry.ClassName)
		if className == "" {
			skipped++
			continue
		}
		membership, err := s.stageMembershipRow(ctx, examCohort.StageID, entry.StudentNo)
		if err != nil {
			return 0, 0, err
		}
		if membership != nil {
			statements = append(statements, s.rosterAddExistingStudentStatements(examCohort, entry, membership)...)
			added++
			continue
		}
		if id, ok := classes[className]; !ok || id == 0 {
			if !createdClass[className] {
				statements = append(statements,
					sqlite.Statement{
						SQL: `INSERT INTO classes(cohort_id, name, created_at)
							VALUES (?, ?, ?)`,
						Args: []any{examCohort.CohortID, className, now},
					},
					sqlite.Statement{
						SQL: `INSERT INTO class_stage_info(class_id, stage_id, status)
							SELECT id, ?, 'active' FROM classes WHERE cohort_id = ? AND name = ?`,
						Args: []any{examCohort.StageID, examCohort.CohortID, className},
					})
				createdClass[className] = true
			}
		}
		statements = append(statements, s.rosterAddNewStudentStatements(examCohort, entry, className, now)...)
		added++
	}
	if len(statements) > 0 {
		if _, err := s.db.Transaction(ctx, statements); err != nil {
			return 0, 0, err
		}
	}
	return added, skipped, nil
}

type stageMembershipRow struct {
	Track    string
	Elective string
}

// stageMembershipRow 返回学生在当前阶段的归属，不存在时返回 nil。
func (s *Store) stageMembershipRow(ctx context.Context, stageID int64, studentNo string) (*stageMembershipRow, error) {
	rows, err := s.db.Query(ctx, `SELECT m.track, m.elective FROM student_stage_memberships m
		JOIN students s ON s.id = m.student_id WHERE s.student_no = ? AND m.stage_id = ?`,
		studentNo, stageID)
	if err != nil {
		return nil, err
	}
	if len(rows.Values) == 0 {
		return nil, nil
	}
	track, _ := rows.Values[0][0].(string)
	elective, _ := rows.Values[0][1].(string)
	return &stageMembershipRow{Track: track, Elective: elective}, nil
}

// rosterAddExistingStudentStatements 为已有阶段归属的考生生成报名名单语句，沿用原班级与类别。
func (s *Store) rosterAddExistingStudentStatements(examCohort ExamCohort, entry ImportStudent, membership *stageMembershipRow) []sqlite.Statement {
	subjects := electiveSubjects(membership.Elective)
	subjectsJSON, _ := json.Marshal(subjects)
	return []sqlite.Statement{
		{
			SQL: `INSERT INTO exam_rosters(exam_id, cohort_id, student_id, class_id, track, status, subjects_json, source_sheet, source_row)
				SELECT ?, ?, m.student_id, m.class_id, m.track, 'active', ?, ?, ?
				FROM student_stage_memberships m JOIN students s ON s.id = m.student_id
				WHERE s.student_no = ? AND m.stage_id = ?`,
			Args: []any{examCohort.ExamID, examCohort.CohortID, string(subjectsJSON),
				entry.SourceSheet, entry.SourceRow, entry.StudentNo, examCohort.StageID},
		},
	}
}

// rosterAddNewStudentStatements 为不在学生名单的考生生成建学生、归属与报名名单的语句。
func (s *Store) rosterAddNewStudentStatements(examCohort ExamCohort, entry ImportStudent, className, now string) []sqlite.Statement {
	subjects := electiveSubjects(entry.Elective)
	subjectsJSON, _ := json.Marshal(subjects)
	return []sqlite.Statement{
		{
			SQL: `INSERT INTO students(student_no, name, created_at, source_sheet, source_row)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(student_no) DO UPDATE SET name = excluded.name`,
			Args: []any{entry.StudentNo, entry.Name, now, entry.SourceSheet, entry.SourceRow},
		},
		{
			SQL: `INSERT INTO student_cohorts(student_id, cohort_id, status)
				SELECT id, ?, 'active' FROM students WHERE student_no = ?
				ON CONFLICT(student_id, cohort_id) DO NOTHING`,
			Args: []any{examCohort.CohortID, entry.StudentNo},
		},
		{
			SQL: `INSERT INTO student_stage_memberships(student_id, stage_id, class_id, status, track, elective)
				SELECT s.id, ?, c.id, 'active', ?, ? FROM students s, classes c
				WHERE s.student_no = ? AND c.cohort_id = ? AND c.name = ?
				ON CONFLICT(student_id, stage_id) DO NOTHING`,
			Args: []any{examCohort.StageID, entry.Track, entry.Elective,
				entry.StudentNo, examCohort.CohortID, className},
		},
		{
			SQL: `INSERT INTO exam_rosters(exam_id, cohort_id, student_id, class_id, track, status, subjects_json, source_sheet, source_row)
				SELECT ?, ?, s.id, c.id, ?, 'active', ?, ?, ? FROM students s, classes c
				WHERE s.student_no = ? AND c.cohort_id = ? AND c.name = ?`,
			Args: []any{examCohort.ExamID, examCohort.CohortID, entry.Track,
				string(subjectsJSON), entry.SourceSheet, entry.SourceRow,
				entry.StudentNo, examCohort.CohortID, className},
		},
	}
}

// ApplyExamImport 写入考试数据，阻断问题存在时返回 ErrImportBlocked。
func (s *Store) ApplyExamImport(ctx context.Context, req ExamImportRequest) (ImportSummary, error) {
	preview, err := s.ValidateExamImport(ctx, req)
	if err != nil {
		return ImportSummary{}, err
	}
	if preview.Blocking > 0 {
		return ImportSummary{}, ErrImportBlocked
	}
	examCohort, err := s.GetExamCohort(ctx, req.ExamID, req.CohortID)
	if err != nil {
		return ImportSummary{}, err
	}
	if req.Report.FileHash == "" {
		return ImportSummary{}, fmt.Errorf("导入文件缺少内容哈希")
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM import_runs WHERE scope_type = 'exam' AND scope_id = ?
		AND data_kind = ? AND file_hash = ?`, examCohort.ID, req.Kind, req.Report.FileHash)
	if err != nil {
		return ImportSummary{}, err
	}
	if len(rows.Values) > 0 {
		return ImportSummary{}, ErrDuplicateImport
	}
	summary, statements, err := s.examImportStatements(ctx, examCohort, req, preview)
	if err != nil {
		return ImportSummary{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	statements = append([]sqlite.Statement{{
		SQL: `INSERT INTO import_runs(scope_type, scope_id, data_kind, file_hash, stored_path,
			relative_path, converted_path, mapping_json, status, row_count, error_count, warning_count, created_at)
			VALUES ('exam', ?, ?, ?, ?, ?, ?, ?, 'imported', ?, ?, ?, ?)`,
		Args: []any{examCohort.ID, req.Kind, req.Report.FileHash, req.Report.StoredPath,
			req.Report.Path, req.Report.ConvertedPath, encodeMapping(req.Mapping), req.Report.RowCount,
			preview.Blocking, preview.Warnings, now},
	}}, statements...)
	for _, sheet := range req.Report.Sheets {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_sheets(import_run_id, sheet_name, role, header_row, data_range, is_current)
				SELECT id, ?, ?, ?, ?, 1 FROM import_runs
				WHERE scope_type = 'exam' AND scope_id = ? AND data_kind = ? AND file_hash = ?
				ON CONFLICT(import_run_id, sheet_name) DO UPDATE SET role = excluded.role,
					header_row = excluded.header_row, data_range = excluded.data_range`,
			Args: []any{sheet.Name, sheet.Role, sheet.HeaderRow, fmt.Sprintf("%d rows", sheet.Rows),
				examCohort.ID, req.Kind, req.Report.FileHash},
		})
	}
	sheetName := ""
	if len(req.Report.Sheets) > 0 {
		sheetName = req.Report.Sheets[0].Name
	}
	for field, column := range req.Mapping {
		if strings.TrimSpace(column) == "" {
			continue
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_columns(import_sheet_id, source_column, standard_field, merged_header,
				is_user_selected)
				SELECT s.id, ?, ?, '', 1 FROM import_sheets s
				JOIN import_runs r ON r.id = s.import_run_id
				WHERE r.scope_type = 'exam' AND r.scope_id = ? AND r.data_kind = ? AND r.file_hash = ?
				AND s.sheet_name = ?
				ON CONFLICT(import_sheet_id, source_column) DO UPDATE SET standard_field = excluded.standard_field`,
			Args: []any{column, field, examCohort.ID, req.Kind, req.Report.FileHash, sheetName},
		})
	}
	for _, issue := range preview.Issues {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_issues(import_run_id, severity, row_number, column_name, code, message, resolved)
				SELECT id, ?, ?, ?, ?, ?, 0 FROM import_runs
				WHERE scope_type = 'exam' AND scope_id = ? AND data_kind = ? AND file_hash = ?`,
			Args: []any{issue.Severity, issue.Row, issue.Column, issue.Code,
				encodeIssueArgs(issue.Args), examCohort.ID, req.Kind, req.Report.FileHash},
		})
	}
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return ImportSummary{}, err
	}
	_, err = s.db.Execute(ctx, `UPDATE exams SET status = 'imported' WHERE id = ? AND status = 'draft'`,
		examCohort.ExamID)
	if err != nil {
		return ImportSummary{}, err
	}
	return summary, nil
}

func (s *Store) validateExamRoster(ctx context.Context, examCohort ExamCohort, req ExamImportRequest, preview *ImportPreview) error {
	students, err := s.stageStudentIndex(ctx, examCohort.StageID)
	if err != nil {
		return err
	}
	existing, err := s.examRosterStudentSet(ctx, examCohort.ID)
	if err != nil {
		return err
	}
	examNos, err := s.examRosterNoMap(ctx, examCohort.ExamID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, entry := range req.Data.Roster {
		if entry.StudentNo == "" {
			preview.Issues = append(preview.Issues, missingIssue(entry.SourceRow, "student_no"))
			continue
		}
		if entry.Name == "" {
			preview.Issues = append(preview.Issues, missingIssue(entry.SourceRow, "name"))
			continue
		}
		if seen[entry.StudentNo] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: entry.SourceRow, Column: "student_no",
				Code: "duplicate_student", Args: []string{entry.StudentNo},
			})
			continue
		}
		seen[entry.StudentNo] = true
		student, ok := students[entry.StudentNo]
		if !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: entry.SourceRow, Column: "student_no",
				Code: "unknown_student", Args: []string{entry.StudentNo},
			})
			continue
		}
		if entry.ClassName != "" {
			if !student.Classes[normalizeClassName(entry.ClassName)] {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: entry.SourceRow, Column: "class_name",
					Code: "unknown_class", Args: []string{entry.ClassName},
				})
				continue
			}
		}
		if entry.ExamNo != "" {
			if previous, ok := examNos[entry.ExamNo]; ok && previous != student.ID {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: entry.SourceRow, Column: "exam_no",
					Code: "duplicate_exam_no", Args: []string{entry.ExamNo},
				})
				continue
			}
		}
		if entry.Track == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: entry.SourceRow, Column: "track",
				Code: "missing_track", Args: []string{entry.StudentNo},
			})
		}
		if existing[student.ID] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	return nil
}

func (s *Store) validateExamScores(ctx context.Context, examCohort ExamCohort, req ExamImportRequest, preview *ImportPreview) error {
	roster, err := s.examRosterUserIndex(ctx, examCohort)
	if err != nil {
		return err
	}
	subjects, err := s.ListExamSubjects(ctx, examCohort.ExamID, examCohort.CohortID)
	if err != nil {
		return err
	}
	fullMarks := map[string]ExamSubject{}
	for _, subject := range subjects {
		if subject.Enabled {
			fullMarks[subject.Subject] = subject
		}
	}
	existing, err := s.examScoreKeySet(ctx, examCohort.ExamID, examCohort.CohortID)
	if err != nil {
		return err
	}
	for _, student := range req.Data.Students {
		entry, ok := roster.find(student.StudentNo)
		if !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: student.SourceRow, Column: "student_no",
				Code: "unknown_roster", Args: []string{student.StudentNo},
			})
			continue
		}
		if len(student.Scores) == 0 && len(student.Flags) == 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: student.SourceRow, Column: "student_no",
				Code: "no_scores", Args: []string{student.StudentNo},
			})
			continue
		}
		for subject, score := range student.Scores {
			item, ok := fullMarks[subject]
			if !ok {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: student.SourceRow, Column: "subject:" + subject,
					Code: "unknown_exam_subject", Args: []string{subject},
				})
				continue
			}
			if score > item.FullMark {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: student.SourceRow, Column: "subject:" + subject,
					Code: "score_out_of_range", Args: []string{student.StudentNo, subject},
				})
				continue
			}
			if existing[fmt.Sprintf("%d\x00%s", entry.StudentID, subject)] {
				preview.Update++
			} else {
				preview.New++
			}
		}
		for subject, flag := range student.Flags {
			if flag == "zero" {
				continue
			}
			if _, ok := fullMarks[subject]; !ok && subject != "total" {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: student.SourceRow, Column: "subject:" + subject,
					Code: "unknown_exam_subject", Args: []string{subject},
				})
				continue
			}
			if subject == "total" {
				continue
			}
			if existing[fmt.Sprintf("%d\x00%s", entry.StudentID, subject)] {
				preview.Update++
			} else {
				preview.New++
			}
		}
	}
	return nil
}

func (s *Store) validateExamCutoffs(ctx context.Context, examCohort ExamCohort, req ExamImportRequest, preview *ImportPreview) error {
	subjects, err := s.subjectFullMarks(ctx)
	if err != nil {
		return err
	}
	tracks, err := s.trackKeys(ctx)
	if err != nil {
		return err
	}
	assessment, err := s.AssessmentForExamCohort(ctx, examCohort.ID)
	if err != nil {
		return err
	}
	tiers, err := s.assessmentTierKeys(ctx, assessment.ID)
	if err != nil {
		return err
	}
	tierSet := map[string]bool{}
	for _, tier := range tiers {
		tierSet[tier] = true
	}
	subjectSeen := map[string]bool{}
	tierSeen := map[string]bool{}
	existingSubjects, err := s.examSubjectCutoffSet(ctx, examCohort.ExamID, examCohort.CohortID)
	if err != nil {
		return err
	}
	existingTiers, err := s.examTierCutoffSet(ctx, examCohort.ExamID, examCohort.CohortID)
	if err != nil {
		return err
	}
	for _, cutoff := range req.Data.SubjectCutoffs {
		if _, ok := subjects[cutoff.Subject]; !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_subject", Code: "unknown_subject",
				Args: []string{cutoff.Subject},
			})
			continue
		}
		if !tracks[cutoff.Track] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_track", Code: "unknown_track",
				Args: []string{cutoff.Track},
			})
			continue
		}
		if cutoff.Value <= 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_value", Code: "cutoff_not_positive",
				Args: []string{cutoff.Subject, cutoff.Track},
			})
			continue
		}
		key := cutoff.Subject + "\x00" + cutoff.Track
		if subjectSeen[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_subject", Code: "duplicate_cutoff",
				Args: []string{cutoff.Subject, cutoff.Track},
			})
			continue
		}
		subjectSeen[key] = true
		if existingSubjects[key] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	for _, cutoff := range req.Data.TierCutoffs {
		if !tierSet[cutoff.TierKey] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_tier", Code: "unknown_tier",
				Args: []string{cutoff.TierKey},
			})
			continue
		}
		if !tracks[cutoff.Track] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_track", Code: "unknown_track",
				Args: []string{cutoff.Track},
			})
			continue
		}
		if cutoff.Value <= 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_value", Code: "cutoff_not_positive",
				Args: []string{cutoff.TierKey, cutoff.Track},
			})
			continue
		}
		key := cutoff.TierKey + "\x00" + cutoff.Track
		if tierSeen[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_tier", Code: "duplicate_cutoff",
				Args: []string{cutoff.TierKey, cutoff.Track},
			})
			continue
		}
		tierSeen[key] = true
		if existingTiers[key] {
			preview.Update++
		} else {
			preview.New++
		}
	}
	return nil
}

func (s *Store) validateExamTargets(ctx context.Context, examCohort ExamCohort, req ExamImportRequest, preview *ImportPreview) error {
	classIDs, err := s.stageClassIDSet(ctx, examCohort.StageID)
	if err != nil {
		return err
	}
	assessment, err := s.AssessmentForExamCohort(ctx, examCohort.ID)
	if err != nil {
		return err
	}
	tiers, err := s.assessmentTierKeys(ctx, assessment.ID)
	if err != nil {
		return err
	}
	tierSet := map[string]bool{}
	for _, tier := range tiers {
		tierSet[tier] = true
	}
	existing, err := s.classTargetSet(ctx, assessment.ID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, target := range req.Data.ClassTargets {
		classID, ok := classIDs[normalizeClassName(target.ClassName)]
		if !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "class_name", Code: "unknown_class",
				Args: []string{target.ClassName},
			})
			continue
		}
		for tier, count := range target.Counts {
			if !tierSet[tier] {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Column: "tier:" + tier, Code: "unknown_tier",
					Args: []string{tier},
				})
				continue
			}
			if count == nil || *count < 0 {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Column: "tier:" + tier, Code: "negative_target",
					Args: []string{target.ClassName, tier},
				})
				continue
			}
			key := fmt.Sprintf("%d\x00%s", classID, tier)
			if seen[key] {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Column: "tier:" + tier, Code: "duplicate_target",
					Args: []string{target.ClassName, tier},
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
	}
	return nil
}

type examRosterIndex struct {
	byStudentNo   map[string]ExamRosterEntry
	byExamNo      map[string]ExamRosterEntry
	byCandidateNo map[string]ExamRosterEntry
}

func (i examRosterIndex) find(value string) (ExamRosterEntry, bool) {
	if entry, ok := i.byStudentNo[value]; ok {
		return entry, true
	}
	if entry, ok := i.byExamNo[value]; ok {
		return entry, true
	}
	entry, ok := i.byCandidateNo[value]
	return entry, ok
}

type stageStudent struct {
	ID        int64
	ClassID   int64
	ClassName string
	Track     string
	Classes   map[string]bool
}

func (s *Store) stageStudentIndex(ctx context.Context, stageID int64) (map[string]stageStudent, error) {
	rows, err := s.db.Query(ctx, `SELECT st.student_no, st.id, m.class_id, c.name, m.track
		FROM student_stage_memberships m JOIN students st ON st.id = m.student_id
		JOIN classes c ON c.id = m.class_id WHERE m.stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	classNames, err := s.stageClassNameSet(ctx, stageID)
	if err != nil {
		return nil, err
	}
	classes := make([]string, 0, len(classNames))
	for name := range classNames {
		classes = append(classes, name)
	}
	classList := map[string]bool{}
	for _, name := range classes {
		classList[name] = true
	}
	result := map[string]stageStudent{}
	for _, row := range rows.Values {
		studentNo, _ := row[0].(string)
		id, _ := row[1].(int64)
		classID, _ := row[2].(int64)
		className, _ := row[3].(string)
		track, _ := row[4].(string)
		result[studentNo] = stageStudent{
			ID: id, ClassID: classID, ClassName: className, Track: track, Classes: classList,
		}
	}
	return result, nil
}

func (s *Store) examRosterStudentSet(ctx context.Context, examCohortID int64) (map[int64]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT student_id FROM exam_rosters WHERE id IN
		(SELECT id FROM exam_rosters WHERE exam_id = (SELECT exam_id FROM exam_cohorts WHERE id = ?)
		AND cohort_id = (SELECT cohort_id FROM exam_cohorts WHERE id = ?))`, examCohortID, examCohortID)
	if err != nil {
		return nil, err
	}
	result := map[int64]bool{}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		result[id] = true
	}
	return result, nil
}

func (s *Store) examRosterNoMap(ctx context.Context, examID int64) (map[string]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT exam_no, student_id FROM exam_rosters
		WHERE exam_id = ? AND exam_no <> ''`, examID)
	if err != nil {
		return nil, err
	}
	result := map[string]int64{}
	for _, row := range rows.Values {
		number, _ := row[0].(string)
		id, _ := row[1].(int64)
		result[number] = id
	}
	return result, nil
}

func (s *Store) examRosterUserIndex(ctx context.Context, examCohort ExamCohort) (examRosterIndex, error) {
	entries, _, err := s.ListExamRoster(ctx, ExamRosterFilter{
		ExamID: examCohort.ExamID, CohortID: examCohort.CohortID, PageSize: 100000,
	})
	if err != nil {
		return examRosterIndex{}, err
	}
	index := examRosterIndex{
		byStudentNo: map[string]ExamRosterEntry{}, byExamNo: map[string]ExamRosterEntry{},
		byCandidateNo: map[string]ExamRosterEntry{},
	}
	for _, entry := range entries {
		index.byStudentNo[entry.StudentNo] = entry
		if entry.ExamNo != "" {
			index.byExamNo[entry.ExamNo] = entry
		}
		if entry.CandidateNo != "" {
			index.byCandidateNo[entry.CandidateNo] = entry
		}
	}
	return index, nil
}

func (s *Store) examScoreKeySet(ctx context.Context, examID, cohortID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT student_id, subject FROM exam_scores WHERE exam_id = ? AND cohort_id = ?`,
		examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		subject, _ := row[1].(string)
		result[fmt.Sprintf("%d\x00%s", id, subject)] = true
	}
	return result, nil
}

func (s *Store) examSubjectCutoffSet(ctx context.Context, examID, cohortID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT subject, track FROM exam_subject_cutoffs WHERE exam_id = ? AND cohort_id = ?`,
		examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		subject, _ := row[0].(string)
		track, _ := row[1].(string)
		result[subject+"\x00"+track] = true
	}
	return result, nil
}

func (s *Store) examTierCutoffSet(ctx context.Context, examID, cohortID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT tier_key, track FROM exam_tier_cutoffs WHERE exam_id = ? AND cohort_id = ?`,
		examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		tier, _ := row[0].(string)
		track, _ := row[1].(string)
		result[tier+"\x00"+track] = true
	}
	return result, nil
}

func (s *Store) stageClassIDSet(ctx context.Context, stageID int64) (map[string]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT c.name, c.id FROM classes c JOIN class_stage_info i ON i.class_id = c.id
		WHERE i.stage_id = ?`, stageID)
	if err != nil {
		return nil, err
	}
	result := map[string]int64{}
	for _, row := range rows.Values {
		name, _ := row[0].(string)
		id, _ := row[1].(int64)
		result[normalizeClassName(name)] = id
	}
	return result, nil
}

func (s *Store) classTargetSet(ctx context.Context, assessmentID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT class_id, tier_key FROM assessment_class_targets WHERE assessment_id = ?`,
		assessmentID)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		tier, _ := row[1].(string)
		result[fmt.Sprintf("%d\x00%s", id, tier)] = true
	}
	return result, nil
}

func (s *Store) trackKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT "key" FROM tracks WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, row := range rows.Values {
		key, _ := row[0].(string)
		result[key] = true
	}
	return result, nil
}

func (s *Store) examImportStatements(ctx context.Context, examCohort ExamCohort, req ExamImportRequest, preview ImportPreview) (ImportSummary, []sqlite.Statement, error) {
	summary := ImportSummary{}
	var statements []sqlite.Statement
	switch req.Kind {
	case ExamImportRoster:
		students, err := s.stageStudentIndex(ctx, examCohort.StageID)
		if err != nil {
			return summary, nil, err
		}
		for _, entry := range req.Data.Roster {
			student, ok := students[entry.StudentNo]
			if !ok {
				continue
			}
			classID := student.ClassID
			if entry.ClassName != "" {
				classIDs, err := s.stageClassIDSet(ctx, examCohort.StageID)
				if err != nil {
					return summary, nil, err
				}
				if id, ok := classIDs[normalizeClassName(entry.ClassName)]; ok {
					classID = id
				}
			}
			track := entry.Track
			if track == "" {
				track = student.Track
			}
			status := entry.Status
			if status == "" {
				status = "active"
			}
			subjects := electiveSubjects(entry.Elective)
			subjectsJSON, err := json.Marshal(subjects)
			if err != nil {
				return summary, nil, err
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_rosters(exam_id, cohort_id, student_id, class_id, exam_no,
					candidate_no, track, total_score, status, subjects_json, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, (SELECT id FROM import_runs WHERE scope_type = 'exam'
						AND scope_id = ? AND data_kind = ? AND file_hash = ?), ?, ?)
					ON CONFLICT(exam_id, cohort_id, student_id) DO UPDATE SET class_id = excluded.class_id,
						exam_no = excluded.exam_no, candidate_no = excluded.candidate_no, track = excluded.track,
						total_score = excluded.total_score, status = excluded.status,
						subjects_json = excluded.subjects_json, import_run_id = excluded.import_run_id,
						source_sheet = excluded.source_sheet, source_row = excluded.source_row`,
				Args: []any{examCohort.ExamID, examCohort.CohortID, student.ID, classID, entry.ExamNo,
					entry.CandidateNo, track, optionalFloat(entry.TotalScore), status, string(subjectsJSON),
					examCohort.ID, req.Kind, req.Report.FileHash,
					entry.SourceSheet, entry.SourceRow},
			})
		}
		summary.Students = preview.New + preview.Update
		summary.Inserted = preview.New
		summary.Updated = preview.Update
	case ExamImportScores:
		roster, err := s.examRosterUserIndex(ctx, examCohort)
		if err != nil {
			return summary, nil, err
		}
		for _, student := range req.Data.Students {
			entry, ok := roster.find(student.StudentNo)
			if !ok {
				continue
			}
			for subject, score := range student.Scores {
				value := score
				statements = append(statements, examScoreStatement(examCohort, req, entry, student, subject, &value, ""))
			}
			for subject, flag := range student.Flags {
				if flag == "zero" || subject == "total" {
					continue
				}
				statements = append(statements, examScoreStatement(examCohort, req, entry, student, subject, nil, flag))
			}
		}
		summary.Inserted = preview.New
		summary.Updated = preview.Update
		summary.Students = preview.New + preview.Update
	case ExamImportCutoffs:
		for _, cutoff := range req.Data.SubjectCutoffs {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_subject_cutoffs(exam_id, cohort_id, subject, track, cutoff)
					VALUES (?, ?, ?, ?, ?) ON CONFLICT(exam_id, cohort_id, subject, track)
					DO UPDATE SET cutoff = excluded.cutoff`,
				Args: []any{examCohort.ExamID, examCohort.CohortID, cutoff.Subject, cutoff.Track, cutoff.Value},
			})
		}
		for _, cutoff := range req.Data.TierCutoffs {
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_tier_cutoffs(exam_id, cohort_id, tier_key, track, cutoff)
					VALUES (?, ?, ?, ?, ?) ON CONFLICT(exam_id, cohort_id, tier_key, track)
					DO UPDATE SET cutoff = excluded.cutoff`,
				Args: []any{examCohort.ExamID, examCohort.CohortID, cutoff.TierKey, cutoff.Track, cutoff.Value},
			})
		}
		summary.SubjectCutoffs = len(req.Data.SubjectCutoffs)
		summary.TierCutoffs = len(req.Data.TierCutoffs)
		summary.Inserted = preview.New
		summary.Updated = preview.Update
	case ExamImportTargets:
		classIDs, err := s.stageClassIDSet(ctx, examCohort.StageID)
		if err != nil {
			return summary, nil, err
		}
		assessment, err := s.AssessmentForExamCohort(ctx, examCohort.ID)
		if err != nil {
			return summary, nil, err
		}
		for _, target := range req.Data.ClassTargets {
			classID, ok := classIDs[normalizeClassName(target.ClassName)]
			if !ok {
				continue
			}
			for tier, count := range target.Counts {
				if count == nil {
					continue
				}
				statements = append(statements, sqlite.Statement{
					SQL: `INSERT INTO assessment_class_targets(assessment_id, class_id, tier_key, target_count)
						VALUES (?, ?, ?, ?) ON CONFLICT(assessment_id, class_id, tier_key)
						DO UPDATE SET target_count = excluded.target_count`,
					Args: []any{assessment.ID, classID, tier, *count},
				})
			}
		}
		summary.ClassTargets = preview.New + preview.Update
		summary.Inserted = preview.New
		summary.Updated = preview.Update
	}
	statements = append(statements, sqlite.Statement{
		SQL: `INSERT INTO data_revisions(scope_type, scope_id, revision, updated_at)
			VALUES ('exam', ?, 1, ?) ON CONFLICT(scope_type, scope_id)
			DO UPDATE SET revision = revision + 1, updated_at = excluded.updated_at`,
		Args: []any{examCohort.ExamID, s.now().Format(time.RFC3339Nano)},
	})
	return summary, statements, nil
}

func examScoreStatement(examCohort ExamCohort, req ExamImportRequest, entry ExamRosterEntry,
	student ImportStudent, subject string, score *float64, flag string) sqlite.Statement {
	return sqlite.Statement{
		SQL: `INSERT INTO exam_scores(exam_id, cohort_id, student_id, subject, score, flag,
			import_run_id, source_sheet, source_row)
			VALUES (?, ?, ?, ?, ?, ?, (SELECT id FROM import_runs WHERE scope_type = 'exam'
				AND scope_id = ? AND data_kind = ? AND file_hash = ?), ?, ?)
			ON CONFLICT(exam_id, cohort_id, student_id, subject) DO UPDATE SET score = excluded.score,
				flag = excluded.flag, import_run_id = excluded.import_run_id,
				source_sheet = excluded.source_sheet, source_row = excluded.source_row`,
		Args: []any{examCohort.ExamID, examCohort.CohortID, entry.StudentID, subject, optionalFloat(score), flag,
			examCohort.ID, req.Kind, req.Report.FileHash, student.SourceSheet, student.SourceRow},
	}
}

func electiveSubjects(elective string) []string {
	text := strings.TrimSpace(elective)
	if text == "" {
		return []string{}
	}
	replacer := strings.NewReplacer("，", "、", ",", "、", "/", "、", "\\", "、")
	parts := strings.Split(replacer.Replace(text), "、")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func encodeMapping(mapping map[string]string) string {
	if len(mapping) == 0 {
		return "{}"
	}
	data, err := json.Marshal(mapping)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func encodeIssueArgs(args []string) string {
	encoded, err := json.Marshal(args)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
