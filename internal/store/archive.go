package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// 结果归档相关的常量。
const (
	// ArchivedInputSummaryMode 是归档核算记录写入 input_summary_json 的模式标记。
	ArchivedInputSummaryMode = "archived"
	// ExamImportArchivedResults 是结果归档导入在导入记录中的数据类型。
	ExamImportArchivedResults = "exam_archived_results"
)

// ErrAssessmentArchived 表示归档考核只读，不允许试算或重新计算。
var ErrAssessmentArchived = errors.New("该考核以结果归档方式导入，只读保存，不能试算或重新计算")

// ImportArchivedResultRow 是从结果归档工作簿读出的一条量化结果。
type ImportArchivedResultRow struct {
	ClassName        string
	Subject          string
	StatGroupName    string
	TeacherNames     []string
	ClassCount       int
	ClassAverage     float64
	GradeAverage     float64
	BaselineAverage  float64
	BaselineRatio    float64
	ImprovementRaw   float64
	ImprovementScore float64
	PassCount        int
	PassTarget       float64
	PassRaw          float64
	PassScore        float64
	// CompletionParticipate 表示工作簿给出了班级完成率，班级总分需要计入这一项。
	CompletionParticipate bool
	CompletionScore       float64
	Total                 float64
	Rank                  int
	// TierIndex 是按零起算的档次序号，nil 表示工作簿没有档次列。
	TierIndex *int
	Points    float64
	SourceRow int
}

// ImportArchivedPersonalScore 是从结果归档工作簿读出的一条教师个人得分。
type ImportArchivedPersonalScore struct {
	TeacherName string
	Subject     string
	Score       float64
	GroupPoints []float64
	SourceRow   int
}

// ArchivedResultData 是一次结果归档导入解析出的全部数据。
type ArchivedResultData struct {
	Rows     []ImportArchivedResultRow
	Personal []ImportArchivedPersonalScore
}

// ArchivedResultImportRequest 是一次结果归档导入请求。
type ArchivedResultImportRequest struct {
	AssessmentID int64
	Data         ArchivedResultData
	Report       ImportReport
	Mapping      map[string]string
	Issues       []ImportIssue
}

// ArchivedImportSummary 是一次结果归档导入写入后的数量摘要。
type ArchivedImportSummary struct {
	ResultRows     int
	PersonalScores int
	Teachers       int
	Classes        int
	ImportRunID    int64
}

// ArchivedInputHash 计算归档数据内容的确定性摘要，用于记录本次归档来源。
func ArchivedInputHash(data ArchivedResultData) string {
	payload := struct {
		Rows     []ImportArchivedResultRow     `json:"rows"`
		Personal []ImportArchivedPersonalScore `json:"personal"`
	}{Rows: data.Rows, Personal: data.Personal}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// AssessmentArchived 判断一次考核是否以结果归档方式导入。
func (s *Store) AssessmentArchived(ctx context.Context, assessmentID int64) (bool, error) {
	rows, err := s.db.Query(ctx, `SELECT 1 FROM archived_assessments WHERE assessment_id = ?`, assessmentID)
	if err != nil {
		return false, err
	}
	return len(rows.Values) > 0, nil
}

// AssertAssessmentCalculable 在试算与正式计算前确认考核不是归档记录。
func (s *Store) AssertAssessmentCalculable(ctx context.Context, assessmentID int64) error {
	archived, err := s.AssessmentArchived(ctx, assessmentID)
	if err != nil {
		return err
	}
	if archived {
		return ErrAssessmentArchived
	}
	return nil
}

// ValidateArchivedResultImport 在写入前校验归档结果，返回可写入数量与问题清单。
func (s *Store) ValidateArchivedResultImport(ctx context.Context, req ArchivedResultImportRequest) (ImportPreview, error) {
	scope, err := s.loadArchivedAssessmentScope(ctx, req.AssessmentID)
	if err != nil {
		return ImportPreview{}, err
	}
	return s.validateArchivedResultImportWithScope(ctx, scope.CohortID, scope.Phase, req)
}

// ValidateArchivedResultImportForExamCohort 只读校验考试下某个级的归档结果，不提前创建考核记录。
func (s *Store) ValidateArchivedResultImportForExamCohort(ctx context.Context, examID, cohortID int64, req ArchivedResultImportRequest) (ImportPreview, error) {
	rows, err := s.db.Query(ctx, `SELECT cohort_id, phase_at_exam FROM exam_cohorts WHERE exam_id = ? AND cohort_id = ?`,
		examID, cohortID)
	if err != nil {
		return ImportPreview{}, err
	}
	if len(rows.Values) == 0 {
		return ImportPreview{}, ErrNotFound
	}
	return s.validateArchivedResultImportWithScope(ctx, intValue(rows.Values[0][0]),
		phaseForStage(stringValue(rows.Values[0][1])), req)
}

func (s *Store) validateArchivedResultImportWithScope(ctx context.Context, cohortID int64, phase string, req ArchivedResultImportRequest) (ImportPreview, error) {
	subjects, err := s.subjectKeySet(ctx)
	if err != nil {
		return ImportPreview{}, err
	}
	preview := ImportPreview{Kind: ExamImportArchivedResults}
	preview.Issues = append(preview.Issues, req.Issues...)

	newClasses := map[string]bool{}
	seenRows := map[string]bool{}
	for _, row := range req.Data.Rows {
		className := normalizeClassName(row.ClassName)
		switch {
		case className == "":
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: row.SourceRow, Column: "class_name", Code: "missing_field",
			})
			continue
		case !subjects[row.Subject]:
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: row.SourceRow, Column: "subject",
				Code: "unknown_subject", Args: []string{row.Subject},
			})
			continue
		}
		if row.Rank <= 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: row.SourceRow, Column: "rank", Code: "archive_missing_rank",
				Args: []string{row.ClassName, row.Subject},
			})
		}
		key := strings.Join([]string{className, row.Subject, normalizeClassName(row.StatGroupName)}, "\x00")
		if seenRows[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: row.SourceRow, Column: "stat_group",
				Code: "archive_stat_group_duplicate", Args: []string{row.ClassName, row.Subject},
			})
			continue
		}
		seenRows[key] = true
		newClasses[className] = true
		preview.New++
	}

	existingClasses, err := s.phaseClassIDMap(ctx, cohortID, phase)
	if err != nil {
		return ImportPreview{}, err
	}
	created := 0
	for name := range newClasses {
		if existingClasses[name] == 0 {
			created++
		}
	}
	if created > 0 {
		preview.Issues = append(preview.Issues, ImportIssue{
			Severity: "warning", Column: "class_name", Code: "classes_created",
			Args: []string{fmt.Sprintf("%d", created)},
		})
	}

	seenPersonal := map[string]bool{}
	for _, item := range req.Data.Personal {
		name := strings.TrimSpace(item.TeacherName)
		switch {
		case name == "":
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: item.SourceRow, Column: "teacher_name", Code: "missing_field",
			})
			continue
		case !subjects[item.Subject]:
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: item.SourceRow, Column: "subject",
				Code: "unknown_subject", Args: []string{item.Subject},
			})
			continue
		}
		key := name + "\x00" + item.Subject
		if seenPersonal[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: item.SourceRow, Column: "teacher_name",
				Code: "archive_personal_duplicate", Args: []string{item.TeacherName, item.Subject},
			})
			continue
		}
		seenPersonal[key] = true
		preview.New++
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

// ApplyArchivedResultImport 校验并以一个事务写入归档结果：核算记录、结果行、教师关联、
// 个人得分与归档标记，同时登记导入账本、撤销账本与审计日志。
func (s *Store) ApplyArchivedResultImport(ctx context.Context, req ArchivedResultImportRequest) (ArchivedImportSummary, error) {
	if req.AssessmentID <= 0 {
		return ArchivedImportSummary{}, ErrNotFound
	}
	if len(req.Data.Rows) == 0 && len(req.Data.Personal) == 0 {
		return ArchivedImportSummary{}, fmt.Errorf("归档数据为空，没有可以写入的结果")
	}
	scope, err := s.loadArchivedAssessmentScope(ctx, req.AssessmentID)
	if err != nil {
		return ArchivedImportSummary{}, err
	}
	preview, err := s.validateArchivedResultImportWithScope(ctx, scope.CohortID, scope.Phase, req)
	if err != nil {
		return ArchivedImportSummary{}, err
	}
	if preview.Blocking > 0 {
		return ArchivedImportSummary{}, fmt.Errorf("%w: %s", ErrImportBlocked, issueCodeSummary(preview.Issues))
	}

	// 在登记 import_runs 之前完成所有前置查询与序列化，确保任何前置失败都不会残留孤儿导入记录。
	existingClasses, err := s.phaseClassIDMap(ctx, scope.CohortID, scope.Phase)
	if err != nil {
		return ArchivedImportSummary{}, err
	}
	prevAssessRows, err := s.db.Query(ctx, `SELECT status, COALESCE(current_run_id, 0) FROM assessments WHERE id = ?`, req.AssessmentID)
	if err != nil {
		return ArchivedImportSummary{}, err
	}
	if len(prevAssessRows.Values) == 0 {
		return ArchivedImportSummary{}, ErrNotFound
	}
	prevAssessStatus := stringValue(prevAssessRows.Values[0][0])
	prevCurrentRunID := intValue(prevAssessRows.Values[0][1])

	prevExamRows, err := s.db.Query(ctx, `SELECT status FROM exams WHERE id = ?`, scope.ExamID)
	if err != nil {
		return ArchivedImportSummary{}, err
	}
	prevExamStatus := ""
	if len(prevExamRows.Values) > 0 {
		prevExamStatus = stringValue(prevExamRows.Values[0][0])
	}

	previousOfficial := int64(0)
	prevRows, err := s.db.Query(ctx, `SELECT id FROM result_runs WHERE assessment_id = ? AND is_official = 1 ORDER BY id DESC LIMIT 1`,
		req.AssessmentID)
	if err != nil {
		return ArchivedImportSummary{}, err
	}
	if len(prevRows.Values) > 0 {
		previousOfficial = intValue(prevRows.Values[0][0])
	}

	hadArchive := false
	prevArchiveImportRunID := int64(0)
	prevArchiveCreatedAt := ""
	archRows, err := s.db.Query(ctx, `SELECT import_run_id, created_at FROM archived_assessments WHERE assessment_id = ?`, req.AssessmentID)
	if err != nil {
		return ArchivedImportSummary{}, err
	}
	if len(archRows.Values) > 0 {
		hadArchive = true
		prevArchiveImportRunID = intValue(archRows.Values[0][0])
		prevArchiveCreatedAt = stringValue(archRows.Values[0][1])
	}

	classNames := make([]string, 0, len(req.Data.Rows))
	newClassNames := make([]string, 0)
	seenClass := map[string]bool{}
	for _, row := range req.Data.Rows {
		name := normalizeClassName(row.ClassName)
		if seenClass[name] {
			continue
		}
		seenClass[name] = true
		classNames = append(classNames, name)
		if existingClasses[name] == 0 {
			newClassNames = append(newClassNames, name)
		}
	}

	type newTeacherEntry struct {
		Name    string
		Subject string
		Row     int
	}
	teacherKeys := map[string]bool{}
	var newTeachers []newTeacherEntry
	recordTeacher := func(name, subject string, row int) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := name + "\x00" + subject
		if teacherKeys[key] {
			return
		}
		teacherKeys[key] = true
		if s.lookupTeacherID(ctx, "", name, subject) > 0 {
			return
		}
		newTeachers = append(newTeachers, newTeacherEntry{Name: name, Subject: subject, Row: row})
	}
	for _, row := range req.Data.Rows {
		for _, name := range row.TeacherNames {
			recordTeacher(name, row.Subject, row.SourceRow)
		}
	}
	for _, item := range req.Data.Personal {
		recordTeacher(item.TeacherName, item.Subject, item.SourceRow)
	}

	personalJSONs := make([]string, len(req.Data.Personal))
	for i, item := range req.Data.Personal {
		groupPoints := item.GroupPoints
		if len(groupPoints) == 0 {
			groupPoints = []float64{item.Score}
		}
		gpJSON, marshalErr := json.Marshal(groupPoints)
		if marshalErr != nil {
			return ArchivedImportSummary{}, marshalErr
		}
		personalJSONs[i] = string(gpJSON)
	}
	inputHash := ArchivedInputHash(req.Data)

	runID, reused, err := s.beginImportRun(ctx, "exam", scope.ExamCohortID, ExamImportArchivedResults,
		req.Report, req.Mapping, preview, false)
	if err != nil {
		return ArchivedImportSummary{}, err
	}

	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	var changes []importChange

	// 归档结果涉及的班级与教师在工作簿里只有名称，需要先补建档案再按名称回填关联。
	for _, name := range newClassNames {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO classes(cohort_id, phase, name, status, import_run_id, created_at)
				VALUES (?, ?, ?, 'active', ?, ?)
				ON CONFLICT(cohort_id, phase, name) DO NOTHING`,
			Args: []any{scope.CohortID, scope.Phase, name, runID, now},
		})
		changes = append(changes, importChange{
			TableName: "classes", Action: "insert",
			Before: map[string]any{"cohort_id": scope.CohortID, "phase": scope.Phase, "name": name},
		})
	}
	for _, teacher := range newTeachers {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO teachers(teacher_no, name, primary_subject, status, note, import_run_id, source_sheet, source_row, created_at, updated_at)
				VALUES ('', ?, ?, 'active', '', ?, ?, ?, ?, ?)
				ON CONFLICT(name, primary_subject) WHERE teacher_no = '' DO NOTHING`,
			Args: []any{teacher.Name, teacher.Subject, runID, req.Report.Path, teacher.Row, now, now},
		})
		changes = append(changes, importChange{
			TableName: "teachers", Action: "insert",
			Before: map[string]any{"name": teacher.Name, "primary_subject": teacher.Subject},
		})
	}

	// 归档结果替换此前正式版本，保持同一考核只有一条正式记录。
	inputSummaryJSON := fmt.Sprintf(`{"mode":%q,"import_run_id":%d}`, ArchivedInputSummaryMode, runID)
	statements = append(statements, sqlite.Statement{
		SQL:  `UPDATE result_runs SET is_official = 0 WHERE assessment_id = ? AND is_official = 1`,
		Args: []any{req.AssessmentID},
	})
	statements = append(statements, sqlite.Statement{
		SQL: `INSERT INTO result_runs(assessment_id, plan_version_id, input_hash, input_summary_json, is_official, created_at)
			VALUES (?, ?, ?, ?, 1, ?)`,
		Args: []any{req.AssessmentID, scope.PlanVersionID, inputHash, inputSummaryJSON, now},
	})

	for _, row := range req.Data.Rows {
		statements = append(statements, archivedResultRowStatement(scope, row))
		for _, name := range row.TeacherNames {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			statements = append(statements, archivedRowTeacherStatement(name, row.Subject))
		}
	}
	for i, item := range req.Data.Personal {
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO personal_scores(run_id, teacher_id, teacher_name_snapshot, cohort_id, phase_at_exam, subject, group_points_json, score)
				VALUES ((SELECT MAX(id) FROM result_runs WHERE assessment_id = ?), ` + archivedTeacherSubquery() + `, ?, ?, ?, ?, ?, ?)`,
			Args: []any{req.AssessmentID, strings.TrimSpace(item.TeacherName), item.Subject,
				strings.TrimSpace(item.TeacherName), scope.CohortID, scope.Phase, item.Subject, personalJSONs[i], item.Score},
		})
	}

	// 将 archived_assessments 放在 classes/teachers 之后追加，UndoImportRun 按 id DESC 逆序执行时会先删除 result_runs 解除引用再清理新建班级与教师。
	changes = append(changes, importChange{
		TableName: "archived_assessments", Action: "insert",
		Before: map[string]any{
			"assessment_id":              req.AssessmentID,
			"exam_id":                    scope.ExamID,
			"prev_assessment_status":     prevAssessStatus,
			"prev_current_run_id":        prevCurrentRunID,
			"prev_exam_status":           prevExamStatus,
			"previous_official_run_id":   previousOfficial,
			"had_archive":                hadArchive,
			"prev_archive_import_run_id": prevArchiveImportRunID,
			"prev_archive_created_at":    prevArchiveCreatedAt,
			"input_summary_json":         inputSummaryJSON,
		},
	})
	statements = append(statements,
		sqlite.Statement{
			SQL: `INSERT INTO archived_assessments(assessment_id, import_run_id, created_at)
				VALUES (?, ?, ?)
				ON CONFLICT(assessment_id) DO UPDATE SET import_run_id = excluded.import_run_id, created_at = excluded.created_at`,
			Args: []any{req.AssessmentID, runID, now},
		},
		sqlite.Statement{
			SQL: `UPDATE assessments SET status = 'calculated',
				current_run_id = (SELECT MAX(id) FROM result_runs WHERE assessment_id = ?), revision = revision + 1
				WHERE id = ?`,
			Args: []any{req.AssessmentID, req.AssessmentID},
		},
		sqlite.Statement{
			SQL:  `UPDATE exams SET status = 'assessed' WHERE id = ? AND status <> 'archived'`,
			Args: []any{scope.ExamID},
		},
		bumpRevisionStatement(fmt.Sprintf("assessment:%d", req.AssessmentID), now),
		auditLogStatement("import_archived_results", "assessment", req.AssessmentID, map[string]any{
			"run": runID, "rows": len(req.Data.Rows), "personal": len(req.Data.Personal),
		}, now),
	)

	statements = append(statements, s.importChangeStatements(runID, changes)...)
	statements = append(statements, s.importIssueStatements(runID, preview.Issues)...)
	statements = append(statements, s.importSheetStatements(runID, req.Report)...)

	if _, err := s.db.Transaction(ctx, statements); err != nil {
		if !reused {
			if delErr := s.deleteImportRun(ctx, runID); delErr != nil {
				return ArchivedImportSummary{}, fmt.Errorf("%w（清理导入记录失败：%v）", err, delErr)
			}
		} else if _, updErr := s.db.Execute(ctx, `UPDATE import_runs SET status = 'failed' WHERE id = ?`, runID); updErr != nil {
			return ArchivedImportSummary{}, fmt.Errorf("%w（更新导入记录状态失败：%v）", err, updErr)
		}
		return ArchivedImportSummary{}, err
	}

	return ArchivedImportSummary{
		ResultRows:     len(req.Data.Rows),
		PersonalScores: len(req.Data.Personal),
		Teachers:       len(teacherKeys),
		Classes:        len(classNames),
		ImportRunID:    runID,
	}, nil
}

// archivedTeacherSubquery 返回按姓名与学科定位教师标识的子查询语句片段。
func archivedTeacherSubquery() string {
	return `(SELECT id FROM teachers WHERE name = ? ORDER BY (primary_subject = ?) DESC, id LIMIT 1)`
}

// archivedResultRowStatement 生成一条归档结果行语句，关联刚写入的核算记录。
func archivedResultRowStatement(scope archivedAssessmentScope, row ImportArchivedResultRow) sqlite.Statement {
	className := normalizeClassName(row.ClassName)
	groupName := normalizeClassName(row.StatGroupName)
	if groupName == "" {
		groupName = className
	}
	tierIndex := 0
	if row.TierIndex != nil {
		tierIndex = *row.TierIndex
	}
	return sqlite.Statement{
		SQL: `INSERT INTO result_rows(
			run_id, stat_group_id, stat_group_name, subject, lead_class_id, lead_class_name,
			class_count, class_average, grade_average, baseline_average, baseline_ratio,
			improvement_raw, improvement_score, pass_count, pass_target, pass_raw, pass_score,
			completion_actual, completion_base, completion_raw, completion_participate, completion_score,
			total_score, rank, tier_index, points, participants_json, indicator_values_json)
			VALUES ((SELECT MAX(id) FROM result_runs WHERE assessment_id = ?),
			0, ?, ?, (SELECT id FROM classes WHERE cohort_id = ? AND phase = ? AND name = ?), ?,
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, ?, ?, ?, ?, ?, ?, '[]', '{}')`,
		Args: []any{
			scope.ID, groupName, row.Subject, scope.CohortID, scope.Phase, className, className,
			row.ClassCount, row.ClassAverage, row.GradeAverage, row.BaselineAverage, row.BaselineRatio,
			row.ImprovementRaw, row.ImprovementScore, row.PassCount, row.PassTarget, row.PassRaw, row.PassScore,
			boolInt(row.CompletionParticipate), row.CompletionScore, row.Total, row.Rank, tierIndex, row.Points,
		},
	}
}

// archivedRowTeacherStatement 把刚写入的归档结果行关联到教师。
func archivedRowTeacherStatement(name, subject string) sqlite.Statement {
	return sqlite.Statement{
		SQL: `INSERT INTO result_row_teachers(result_row_id, teacher_id, teacher_name_snapshot)
			VALUES ((SELECT MAX(id) FROM result_rows), ` + archivedTeacherSubquery() + `, ?)
			ON CONFLICT(result_row_id, teacher_id) DO NOTHING`,
		Args: []any{name, subject, name},
	}
}

// archivedAssessmentScope 是归档导入需要的最小考核范围信息。
type archivedAssessmentScope struct {
	ID            int64
	ExamID        int64
	ExamCohortID  int64
	CohortID      int64
	Phase         string
	PlanVersionID int64
}

func (s *Store) loadArchivedAssessmentScope(ctx context.Context, assessmentID int64) (archivedAssessmentScope, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id, ec.exam_id, ec.id, ec.cohort_id, ec.phase_at_exam,
		COALESCE(a.plan_version_id, 0)
		FROM assessments a
		JOIN exam_cohorts ec ON ec.id = a.exam_cohort_id
		WHERE a.id = ?`, assessmentID)
	if err != nil {
		return archivedAssessmentScope{}, err
	}
	if len(rows.Values) == 0 {
		return archivedAssessmentScope{}, ErrNotFound
	}
	r := rows.Values[0]
	scope := archivedAssessmentScope{
		ID:            intValue(r[0]),
		ExamID:        intValue(r[1]),
		ExamCohortID:  intValue(r[2]),
		CohortID:      intValue(r[3]),
		Phase:         phaseForStage(stringValue(r[4])),
		PlanVersionID: intValue(r[5]),
	}
	if scope.PlanVersionID == 0 {
		plan, err := s.EnsureDefaultPlan(ctx)
		if err != nil {
			return archivedAssessmentScope{}, err
		}
		scope.PlanVersionID = plan.ID
	}
	return scope, nil
}

func (s *Store) subjectKeySet(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT "key" FROM subjects`)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows.Values))
	for _, r := range rows.Values {
		out[stringValue(r[0])] = true
	}
	return out, nil
}
