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

// ApplyExamImport 写入考试数据并记录可撤销的变更明细，阻断问题存在时返回 ErrImportBlocked。
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
	runID, reused, err := s.beginExamImportRun(ctx, examCohort, req, preview)
	if err != nil {
		return ImportSummary{}, err
	}
	summary, statements, changes, err := s.examImportStatements(ctx, examCohort, req, preview, runID)
	if err != nil {
		if !reused {
			s.deleteImportRun(ctx, runID)
		}
		return ImportSummary{}, err
	}
	for _, sheet := range req.Report.Sheets {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_sheets(import_run_id, sheet_name, role, header_row, data_range, is_current)
				VALUES (?, ?, ?, ?, ?, 1)
				ON CONFLICT(import_run_id, sheet_name) DO UPDATE SET role = excluded.role,
					header_row = excluded.header_row, data_range = excluded.data_range`,
			Args: []any{runID, sheet.Name, sheet.Role, sheet.HeaderRow, fmt.Sprintf("%d rows", sheet.Rows)},
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
				WHERE s.import_run_id = ? AND s.sheet_name = ?
				ON CONFLICT(import_sheet_id, source_column) DO UPDATE SET standard_field = excluded.standard_field`,
			Args: []any{column, field, runID, sheetName},
		})
	}
	for _, issue := range preview.Issues {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO import_issues(import_run_id, severity, row_number, column_name, code, message, resolved)
				VALUES (?, ?, ?, ?, ?, ?, 0)`,
			Args: []any{runID, issue.Severity, issue.Row, issue.Column, issue.Code,
				encodeIssueArgs(issue.Args)},
		})
	}
	statements = append(statements, s.importChangeStatements(runID, changes)...)
	now := s.now().Format(time.RFC3339Nano)
	statements = append(statements, sqlite.Statement{
		SQL: `INSERT INTO audit_log(action, entity, entity_id, detail_json, created_at, operator)
			VALUES ('import', 'exam', ?, ?, ?, 'local')`,
		Args: []any{examCohort.ID, fmt.Sprintf(`{"kind":%q,"run":%d}`, req.Kind, runID), now},
	})
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		if !reused {
			s.deleteImportRun(ctx, runID)
		}
		return ImportSummary{}, err
	}
	_, err = s.db.Execute(ctx, `UPDATE exams SET status = 'imported' WHERE id = ? AND status = 'draft'`,
		examCohort.ExamID)
	if err != nil {
		return ImportSummary{}, err
	}
	return summary, nil
}

func (s *Store) beginExamImportRun(ctx context.Context, examCohort ExamCohort, req ExamImportRequest, preview ImportPreview) (int64, bool, error) {
	rows, err := s.db.Query(ctx, `SELECT id, status FROM import_runs
		WHERE scope_type = 'exam' AND scope_id = ? AND data_kind = ? AND file_hash = ?`,
		examCohort.ID, req.Kind, req.Report.FileHash)
	if err != nil {
		return 0, false, err
	}
	now := s.now().Format(time.RFC3339Nano)
	if len(rows.Values) > 0 {
		id := intValue(rows.Values[0][0])
		status := stringValue(rows.Values[0][1])
		if status == "imported" {
			return 0, false, ErrDuplicateImport
		}
		if _, err := s.db.Transaction(ctx, []sqlite.Statement{
			{
				SQL: `UPDATE import_runs SET status = 'imported', stored_path = ?,
					relative_path = ?, converted_path = ?, mapping_json = ?, row_count = ?,
					error_count = ?, warning_count = ?, created_at = ? WHERE id = ?`,
				Args: []any{req.Report.StoredPath, req.Report.Path, req.Report.ConvertedPath,
					encodeMapping(req.Mapping), req.Report.RowCount, preview.Blocking,
					preview.Warnings, now, id},
			},
			{SQL: `DELETE FROM import_changes WHERE import_run_id = ?`, Args: []any{id}},
			{SQL: `DELETE FROM import_issues WHERE import_run_id = ?`, Args: []any{id}},
			{SQL: `DELETE FROM import_sheets WHERE import_run_id = ?`, Args: []any{id}},
		}); err != nil {
			return 0, false, err
		}
		return id, true, nil
	}
	result, err := s.db.Execute(ctx, `INSERT INTO import_runs(scope_type, scope_id, data_kind, file_hash, stored_path,
		relative_path, converted_path, mapping_json, status, row_count, error_count, warning_count, created_at)
		VALUES ('exam', ?, ?, ?, ?, ?, ?, ?, 'imported', ?, ?, ?, ?)`,
		examCohort.ID, req.Kind, req.Report.FileHash, req.Report.StoredPath,
		req.Report.Path, req.Report.ConvertedPath, encodeMapping(req.Mapping), req.Report.RowCount,
		preview.Blocking, preview.Warnings, now)
	if err != nil {
		return 0, false, err
	}
	return result.LastInsertID, false, nil
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

func (s *Store) examImportStatements(ctx context.Context, examCohort ExamCohort, req ExamImportRequest, preview ImportPreview, runID int64) (ImportSummary, []sqlite.Statement, []importChange, error) {
	summary := ImportSummary{}
	var statements []sqlite.Statement
	var changes []importChange
	switch req.Kind {
	case ExamImportRoster:
		students, err := s.stageStudentIndex(ctx, examCohort.StageID)
		if err != nil {
			return summary, nil, nil, err
		}
		classIDs, err := s.stageClassIDSet(ctx, examCohort.StageID)
		if err != nil {
			return summary, nil, nil, err
		}
		existingRoster, err := s.existingExamRosterMap(ctx, examCohort.ExamID, examCohort.CohortID)
		if err != nil {
			return summary, nil, nil, err
		}
		for _, entry := range req.Data.Roster {
			student, ok := students[entry.StudentNo]
			if !ok {
				continue
			}
			classID := student.ClassID
			if entry.ClassName != "" {
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
				return summary, nil, nil, err
			}
			if before, exists := existingRoster[student.ID]; exists {
				changes = append(changes, importChange{
					Entity: "exam_roster",
					Action: "update",
					Before: before,
				})
			} else {
				changes = append(changes, importChange{
					Entity: "exam_roster",
					Action: "insert",
					Before: map[string]any{
						"exam_id":    examCohort.ExamID,
						"cohort_id":  examCohort.CohortID,
						"student_id": student.ID,
					},
				})
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_rosters(exam_id, cohort_id, student_id, class_id, exam_no,
					candidate_no, track, total_score, status, subjects_json, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
					ON CONFLICT(exam_id, cohort_id, student_id) DO UPDATE SET class_id = excluded.class_id,
						exam_no = excluded.exam_no, candidate_no = excluded.candidate_no, track = excluded.track,
						total_score = excluded.total_score, status = excluded.status,
						subjects_json = excluded.subjects_json, import_run_id = excluded.import_run_id,
						source_sheet = excluded.source_sheet, source_row = excluded.source_row`,
				Args: []any{examCohort.ExamID, examCohort.CohortID, student.ID, classID, entry.ExamNo,
					entry.CandidateNo, track, optionalFloat(entry.TotalScore), status, string(subjectsJSON),
					runID, entry.SourceSheet, entry.SourceRow},
			})
		}
		summary.Students = preview.New + preview.Update
		summary.Inserted = preview.New
		summary.Updated = preview.Update
	case ExamImportScores:
		roster, err := s.examRosterUserIndex(ctx, examCohort)
		if err != nil {
			return summary, nil, nil, err
		}
		existingScores, err := s.existingExamScoreMap(ctx, examCohort.ExamID, examCohort.CohortID)
		if err != nil {
			return summary, nil, nil, err
		}
		recorded := map[string]bool{}
		recordScoreChange := func(studentID int64, subject string) {
			key := fmt.Sprintf("%d\x00%s", studentID, subject)
			if recorded[key] {
				return
			}
			recorded[key] = true
			if before, exists := existingScores[key]; exists {
				changes = append(changes, importChange{
					Entity: "exam_score",
					Action: "update",
					Before: before,
				})
			} else {
				changes = append(changes, importChange{
					Entity: "exam_score",
					Action: "insert",
					Before: map[string]any{
						"exam_id":    examCohort.ExamID,
						"cohort_id":  examCohort.CohortID,
						"student_id": studentID,
						"subject":    subject,
					},
				})
			}
		}
		for _, student := range req.Data.Students {
			entry, ok := roster.find(student.StudentNo)
			if !ok {
				continue
			}
			for subject, score := range student.Scores {
				value := score
				recordScoreChange(entry.StudentID, subject)
				statements = append(statements, examScoreStatement(examCohort, runID, entry, student, subject, &value, ""))
			}
			for subject, flag := range student.Flags {
				if flag == "zero" || subject == "total" {
					continue
				}
				recordScoreChange(entry.StudentID, subject)
				statements = append(statements, examScoreStatement(examCohort, runID, entry, student, subject, nil, flag))
			}
		}
		summary.Inserted = preview.New
		summary.Updated = preview.Update
		summary.Students = preview.New + preview.Update
	case ExamImportCutoffs:
		existingSubjects, err := s.existingSubjectCutoffMap(ctx, examCohort.ExamID, examCohort.CohortID)
		if err != nil {
			return summary, nil, nil, err
		}
		existingTiers, err := s.existingTierCutoffMap(ctx, examCohort.ExamID, examCohort.CohortID)
		if err != nil {
			return summary, nil, nil, err
		}
		for _, cutoff := range req.Data.SubjectCutoffs {
			key := cutoff.Subject + "\x00" + cutoff.Track
			if beforeVal, exists := existingSubjects[key]; exists {
				changes = append(changes, importChange{
					Entity: "exam_subject_cutoff",
					Action: "update",
					Before: map[string]any{
						"exam_id":   examCohort.ExamID,
						"cohort_id": examCohort.CohortID,
						"subject":   cutoff.Subject,
						"track":     cutoff.Track,
						"cutoff":    beforeVal,
					},
				})
			} else {
				changes = append(changes, importChange{
					Entity: "exam_subject_cutoff",
					Action: "insert",
					Before: map[string]any{
						"exam_id":   examCohort.ExamID,
						"cohort_id": examCohort.CohortID,
						"subject":   cutoff.Subject,
						"track":     cutoff.Track,
					},
				})
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_subject_cutoffs(exam_id, cohort_id, subject, track, cutoff)
					VALUES (?, ?, ?, ?, ?) ON CONFLICT(exam_id, cohort_id, subject, track)
					DO UPDATE SET cutoff = excluded.cutoff`,
				Args: []any{examCohort.ExamID, examCohort.CohortID, cutoff.Subject, cutoff.Track, cutoff.Value},
			})
		}
		for _, cutoff := range req.Data.TierCutoffs {
			key := cutoff.TierKey + "\x00" + cutoff.Track
			if beforeVal, exists := existingTiers[key]; exists {
				changes = append(changes, importChange{
					Entity: "exam_tier_cutoff",
					Action: "update",
					Before: map[string]any{
						"exam_id":   examCohort.ExamID,
						"cohort_id": examCohort.CohortID,
						"tier_key":  cutoff.TierKey,
						"track":     cutoff.Track,
						"cutoff":    beforeVal,
					},
				})
			} else {
				changes = append(changes, importChange{
					Entity: "exam_tier_cutoff",
					Action: "insert",
					Before: map[string]any{
						"exam_id":   examCohort.ExamID,
						"cohort_id": examCohort.CohortID,
						"tier_key":  cutoff.TierKey,
						"track":     cutoff.Track,
					},
				})
			}
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
			return summary, nil, nil, err
		}
		assessment, err := s.AssessmentForExamCohort(ctx, examCohort.ID)
		if err != nil {
			return summary, nil, nil, err
		}
		existingTargets, err := s.existingClassTargetMap(ctx, assessment.ID)
		if err != nil {
			return summary, nil, nil, err
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
				key := fmt.Sprintf("%d\x00%s", classID, tier)
				if beforeVal, exists := existingTargets[key]; exists {
					changes = append(changes, importChange{
						Entity: "exam_class_target",
						Action: "update",
						Before: map[string]any{
							"assessment_id": assessment.ID,
							"class_id":      classID,
							"tier_key":      tier,
							"target_count":  beforeVal,
						},
					})
				} else {
					changes = append(changes, importChange{
						Entity: "exam_class_target",
						Action: "insert",
						Before: map[string]any{
							"assessment_id": assessment.ID,
							"class_id":      classID,
							"tier_key":      tier,
						},
					})
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
	return summary, statements, changes, nil
}

func (s *Store) existingExamRosterMap(ctx context.Context, examID, cohortID int64) (map[int64]map[string]any, error) {
	rows, err := s.db.Query(ctx, `SELECT student_id, class_id, exam_no, candidate_no, track,
		total_score, status, subjects_json, COALESCE(import_run_id, 0), source_sheet, source_row
		FROM exam_rosters WHERE exam_id = ? AND cohort_id = ?`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]map[string]any, len(rows.Values))
	for _, row := range rows.Values {
		studentID := intValue(row[0])
		result[studentID] = map[string]any{
			"exam_id":       examID,
			"cohort_id":     cohortID,
			"student_id":    studentID,
			"class_id":      intValue(row[1]),
			"exam_no":       stringValue(row[2]),
			"candidate_no":  stringValue(row[3]),
			"track":         stringValue(row[4]),
			"total_score":   row[5],
			"status":        stringValue(row[6]),
			"subjects_json": stringValue(row[7]),
			"import_run_id": intValue(row[8]),
			"source_sheet":  stringValue(row[9]),
			"source_row":    intValue(row[10]),
		}
	}
	return result, nil
}

func (s *Store) existingExamScoreMap(ctx context.Context, examID, cohortID int64) (map[string]map[string]any, error) {
	rows, err := s.db.Query(ctx, `SELECT student_id, subject, score, flag,
		COALESCE(import_run_id, 0), source_sheet, source_row
		FROM exam_scores WHERE exam_id = ? AND cohort_id = ?`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]map[string]any, len(rows.Values))
	for _, row := range rows.Values {
		studentID := intValue(row[0])
		subject := stringValue(row[1])
		key := fmt.Sprintf("%d\x00%s", studentID, subject)
		result[key] = map[string]any{
			"exam_id":       examID,
			"cohort_id":     cohortID,
			"student_id":    studentID,
			"subject":       subject,
			"score":         row[2],
			"flag":          stringValue(row[3]),
			"import_run_id": intValue(row[4]),
			"source_sheet":  stringValue(row[5]),
			"source_row":    intValue(row[6]),
		}
	}
	return result, nil
}

func (s *Store) existingSubjectCutoffMap(ctx context.Context, examID, cohortID int64) (map[string]float64, error) {
	rows, err := s.db.Query(ctx, `SELECT subject, track, cutoff FROM exam_subject_cutoffs
		WHERE exam_id = ? AND cohort_id = ?`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]float64, len(rows.Values))
	for _, row := range rows.Values {
		result[stringValue(row[0])+"\x00"+stringValue(row[1])] = floatValue(row[2])
	}
	return result, nil
}

func (s *Store) existingTierCutoffMap(ctx context.Context, examID, cohortID int64) (map[string]float64, error) {
	rows, err := s.db.Query(ctx, `SELECT tier_key, track, cutoff FROM exam_tier_cutoffs
		WHERE exam_id = ? AND cohort_id = ?`, examID, cohortID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]float64, len(rows.Values))
	for _, row := range rows.Values {
		result[stringValue(row[0])+"\x00"+stringValue(row[1])] = floatValue(row[2])
	}
	return result, nil
}

func (s *Store) existingClassTargetMap(ctx context.Context, assessmentID int64) (map[string]float64, error) {
	rows, err := s.db.Query(ctx, `SELECT class_id, tier_key, target_count FROM assessment_class_targets
		WHERE assessment_id = ?`, assessmentID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]float64, len(rows.Values))
	for _, row := range rows.Values {
		key := fmt.Sprintf("%d\x00%s", intValue(row[0]), stringValue(row[1]))
		result[key] = floatValue(row[2])
	}
	return result, nil
}

func examScoreStatement(examCohort ExamCohort, runID int64, entry ExamRosterEntry,
	student ImportStudent, subject string, score *float64, flag string) sqlite.Statement {
	return sqlite.Statement{
		SQL: `INSERT INTO exam_scores(exam_id, cohort_id, student_id, subject, score, flag,
			import_run_id, source_sheet, source_row)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(exam_id, cohort_id, student_id, subject) DO UPDATE SET score = excluded.score,
				flag = excluded.flag, import_run_id = excluded.import_run_id,
				source_sheet = excluded.source_sheet, source_row = excluded.source_row`,
		Args: []any{examCohort.ExamID, examCohort.CohortID, entry.StudentID, subject, optionalFloat(score), flag,
			runID, student.SourceSheet, student.SourceRow},
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
