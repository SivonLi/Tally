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

// ValidateExamImport 在写入前检查考试数据导入。
func (s *Store) ValidateExamImport(ctx context.Context, req ExamImportRequest) (ImportPreview, error) {
	ec, err := s.GetExamCohort(ctx, req.ExamID, req.CohortID)
	if err != nil {
		return ImportPreview{}, err
	}
	preview := ImportPreview{Kind: req.Kind}
	preview.Issues = append(preview.Issues, req.Issues...)
	switch req.Kind {
	case ExamImportRoster:
		err = s.validateExamRoster(ctx, ec, req, &preview)
	case ExamImportScores:
		err = s.validateExamScores(ctx, ec, req, &preview)
	case ExamImportCutoffs:
		err = s.validateExamCutoffs(ctx, ec, req, &preview)
	case ExamImportTargets:
		err = s.validateExamTargets(ctx, ec, req, &preview)
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

// ApplyExamImport 校验并原子写入考试数据（报名名单、成绩及排名预计算、分数线或班级目标），同时记录回滚账本。
func (s *Store) ApplyExamImport(ctx context.Context, req ExamImportRequest) (ImportSummary, error) {
	ec, err := s.GetExamCohort(ctx, req.ExamID, req.CohortID)
	if err != nil {
		return ImportSummary{}, err
	}
	preview, err := s.ValidateExamImport(ctx, req)
	if err != nil {
		return ImportSummary{}, err
	}
	if preview.Blocking > 0 {
		return ImportSummary{}, fmt.Errorf("%w: %s", ErrImportBlocked, issueCodeSummary(preview.Issues))
	}

	// 导入成绩前先为缺失报名记录的考生补建档案和参试快照（当成绩表中已带有学号、姓名与班级）。
	if req.Kind == ExamImportScores && len(req.Data.Students) > 0 {
		if _, _, err := s.AddExamRosterStudents(ctx, ec, req.Data.Students); err != nil {
			return ImportSummary{}, err
		}
	}

	runID, reused, err := s.beginImportRun(ctx, "exam", ec.ID, req.Kind, req.Report, req.Mapping, preview, false)
	if err != nil {
		return ImportSummary{}, err
	}

	summary, statements, changes, err := s.examImportStatements(ctx, ec, req, preview, runID)
	if err != nil {
		if !reused {
			_ = s.deleteImportRun(ctx, runID)
		}
		return ImportSummary{}, err
	}

	now := s.now().Format(time.RFC3339Nano)
	statements = append(statements, s.importChangeStatements(runID, changes)...)
	statements = append(statements, s.importIssueStatements(runID, preview.Issues)...)
	statements = append(statements, s.importSheetStatements(runID, req.Report)...)
	if req.Kind == ExamImportScores {
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE exams SET status = 'scored' WHERE id = ? AND status = 'draft'`,
			Args: []any{ec.ExamID},
		})
	}
	statements = append(statements,
		bumpRevisionStatement(fmt.Sprintf("exam:%d:%d", ec.ExamID, ec.CohortID), now),
		auditLogStatement("import_exam", "exam_cohort", ec.ID, map[string]any{
			"kind": req.Kind, "run": runID,
		}, now),
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		if !reused {
			_ = s.deleteImportRun(ctx, runID)
		}
		return ImportSummary{}, err
	}

	if req.Kind == ExamImportScores || req.Kind == ExamImportRoster {
		if err := s.recomputeExamCohortRanks(ctx, ec.ID); err != nil {
			return ImportSummary{}, err
		}
	}
	if err := s.markAssessmentStale(ctx, ec.ID); err != nil {
		return ImportSummary{}, err
	}
	for _, issue := range preview.Issues {
		if issue.Severity == "warning" {
			summary.Warnings = append(summary.Warnings, issue.Code)
		}
	}
	return summary, nil
}

func (s *Store) validateExamRoster(ctx context.Context, ec ExamCohort, req ExamImportRequest, preview *ImportPreview) error {
	phase := phaseForStage(ec.GradeLevelAtExam)
	classMap, err := s.phaseClassIDMap(ctx, ec.CohortID, phase)
	if err != nil {
		return err
	}
	existing, err := s.examRosterUserIndex(ctx, ec)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, entry := range req.Data.Roster {
		sno := strings.TrimSpace(entry.StudentNo)
		name := strings.TrimSpace(entry.Name)
		if sno == "" && strings.TrimSpace(entry.CandidateNo) == "" && name == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: entry.SourceRow, Column: "student_no", Code: "missing_student_id",
			})
			continue
		}
		key := sno
		if key == "" {
			key = strings.TrimSpace(entry.CandidateNo) + "\x00" + name + "\x00" + normalizeClassName(entry.ClassName)
		}
		if seen[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: entry.SourceRow, Column: "student_no",
				Code: "duplicate_roster_student", Args: []string{sno},
			})
			preview.Skip++
			continue
		}
		seen[key] = true
		_ = classMap
		track := normalizeTrackValue(entry.Track)
		if track == "" {
			track = trackFromElective(entry.Elective)
		}
		if _, found := existing.findMulti(sno, entry.CandidateNo, name, entry.ClassName); found {
			preview.Update++
		} else {
			preview.New++
		}
		if track == "" {
			idLabel := sno
			if idLabel == "" {
				idLabel = name
			}
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: entry.SourceRow, Column: "track",
				Code: "missing_track", Args: []string{idLabel},
			})
		}
	}
	return nil
}

func (s *Store) validateExamScores(ctx context.Context, ec ExamCohort, req ExamImportRequest, preview *ImportPreview) error {
	roster, err := s.examRosterUserIndex(ctx, ec)
	if err != nil {
		return err
	}
	subs, err := s.ListExamSubjects(ctx, ec.ExamID, ec.CohortID)
	if err != nil {
		return err
	}
	fullMarks := map[string]float64{}
	for _, sub := range subs {
		if sub.Enabled {
			fullMarks[sub.Subject] = sub.FullMark
		}
	}
	if len(fullMarks) == 0 {
		allSubs, _ := s.ListSubjects(ctx)
		for _, sub := range allSubs {
			fullMarks[sub.Key] = sub.FullMark
		}
	}

	existingScores, err := s.existingExamScoreKeys(ctx, ec.ID)
	if err != nil {
		return err
	}

	seen := map[string]bool{}
	for _, student := range req.Data.Students {
		sno := strings.TrimSpace(student.StudentNo)
		name := strings.TrimSpace(student.Name)
		if sno == "" && strings.TrimSpace(student.CandidateCode) == "" && name == "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: student.SourceRow, Column: "student_no", Code: "missing_student_id",
			})
			continue
		}
		entry, ok := roster.findMulti(sno, student.CandidateCode, name, student.ClassName)
		if !ok {
			if sno != "" && name != "" && normalizeClassName(student.ClassName) != "" {
				// 成绩表中包含足够建档信息，ApplyExamImport 会自动补建参考学生。
				for subj := range student.Scores {
					if maxScore, known := fullMarks[subj]; !known {
						preview.Issues = append(preview.Issues, ImportIssue{
							Severity: "blocking", Row: student.SourceRow, Column: "subject:" + subj,
							Code: "unknown_exam_subject", Args: []string{subj},
						})
					} else if student.Scores[subj] < 0 || student.Scores[subj] > maxScore {
						preview.Issues = append(preview.Issues, ImportIssue{
							Severity: "blocking", Row: student.SourceRow, Column: "subject:" + subj,
							Code: "score_out_of_range", Args: []string{sno, subj},
						})
					} else {
						preview.New++
					}
				}
				continue
			}
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Row: student.SourceRow, Column: "student_no",
				Code: "student_not_in_roster", Args: []string{sno},
			})
			continue
		}
		if seen[sno] && sno != "" {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "warning", Row: student.SourceRow, Column: "student_no",
				Code: "duplicate_score_student", Args: []string{sno},
			})
			preview.Skip++
			continue
		}
		if sno != "" {
			seen[sno] = true
		}
		for subj, val := range student.Scores {
			maxScore, known := fullMarks[subj]
			if !known {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: student.SourceRow, Column: "subject:" + subj,
					Code: "unknown_exam_subject", Args: []string{subj},
				})
				continue
			}
			if val < 0 || val > maxScore {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: student.SourceRow, Column: "subject:" + subj,
					Code: "score_out_of_range", Args: []string{sno, subj},
				})
				continue
			}
			if existingScores[fmt.Sprintf("%d\x00%s", entry.ID, subj)] {
				preview.Update++
			} else {
				preview.New++
			}
		}
		for subj, flag := range student.Flags {
			if flag == "zero" || subj == "total" {
				continue
			}
			if _, known := fullMarks[subj]; !known {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Row: student.SourceRow, Column: "subject:" + subj,
					Code: "unknown_exam_subject", Args: []string{subj},
				})
				continue
			}
			if existingScores[fmt.Sprintf("%d\x00%s", entry.ID, subj)] {
				preview.Update++
			} else {
				preview.New++
			}
		}
	}
	return nil
}

func (s *Store) validateExamCutoffs(ctx context.Context, ec ExamCohort, req ExamImportRequest, preview *ImportPreview) error {
	subjects, err := s.subjectFullMarksMap(ctx)
	if err != nil {
		return err
	}
	tracks, err := s.trackKeySet(ctx)
	if err != nil {
		return err
	}
	assessment, err := s.AssessmentForExamCohort(ctx, ec.ID)
	if err != nil {
		return err
	}
	tiers, err := s.assessmentTierKeys(ctx, assessment.ID)
	if err != nil {
		return err
	}
	tierSet := make(map[string]bool, len(tiers)+1)
	for _, tk := range tiers {
		tierSet[tk] = true
	}
	tierSet["single_subject"] = true

	existingCutoffs, err := s.existingCutoffMap(ctx, ec.ID)
	if err != nil {
		return err
	}
	seenSubj := map[string]bool{}
	seenTier := map[string]bool{}

	for _, c := range req.Data.SubjectCutoffs {
		if _, ok := subjects[c.Subject]; !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_subject", Code: "unknown_subject", Args: []string{c.Subject},
			})
			continue
		}
		if !tracks[c.Track] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_track", Code: "unknown_track", Args: []string{c.Track},
			})
			continue
		}
		if c.Value <= 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_value", Code: "cutoff_not_positive", Args: []string{c.Subject, c.Track},
			})
			continue
		}
		key := "subject\x00" + c.Subject + "\x00" + c.Track
		if seenSubj[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_subject", Code: "duplicate_cutoff", Args: []string{c.Subject, c.Track},
			})
			continue
		}
		seenSubj[key] = true
		if _, exists := existingCutoffs[key]; exists {
			preview.Update++
		} else {
			preview.New++
		}
	}
	for _, c := range req.Data.TierCutoffs {
		if !tierSet[c.TierKey] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_tier", Code: "unknown_tier", Args: []string{c.TierKey},
			})
			continue
		}
		if !tracks[c.Track] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_track", Code: "unknown_track", Args: []string{c.Track},
			})
			continue
		}
		if c.Value <= 0 {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_value", Code: "cutoff_not_positive", Args: []string{c.TierKey, c.Track},
			})
			continue
		}
		key := "tier\x00" + c.TierKey + "\x00" + c.Track
		if seenTier[key] {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "cutoff_tier", Code: "duplicate_cutoff", Args: []string{c.TierKey, c.Track},
			})
			continue
		}
		seenTier[key] = true
		if _, exists := existingCutoffs[key]; exists {
			preview.Update++
		} else {
			preview.New++
		}
	}
	return nil
}

func (s *Store) validateExamTargets(ctx context.Context, ec ExamCohort, req ExamImportRequest, preview *ImportPreview) error {
	phase := phaseForStage(ec.GradeLevelAtExam)
	classMap, err := s.phaseClassIDMap(ctx, ec.CohortID, phase)
	if err != nil {
		return err
	}
	assessment, err := s.AssessmentForExamCohort(ctx, ec.ID)
	if err != nil {
		return err
	}
	tiers, err := s.assessmentTierKeys(ctx, assessment.ID)
	if err != nil {
		return err
	}
	tierSet := make(map[string]bool, len(tiers)+1)
	for _, tk := range tiers {
		tierSet[tk] = true
	}
	tierSet["single_subject"] = true
	tRows, err := s.db.Query(ctx, `SELECT class_id, tier_key FROM assessment_class_targets WHERE assessment_id = ?`, assessment.ID)
	if err != nil {
		return err
	}
	existing := make(map[string]bool, len(tRows.Values))
	for _, r := range tRows.Values {
		existing[fmt.Sprintf("%d\x00%s", intValue(r[0]), stringValue(r[1]))] = true
	}
	seen := map[string]bool{}
	for _, t := range req.Data.ClassTargets {
		cid, ok := classMap[normalizeClassName(t.ClassName)]
		if !ok {
			preview.Issues = append(preview.Issues, ImportIssue{
				Severity: "blocking", Column: "class_name", Code: "unknown_class", Args: []string{t.ClassName},
			})
			continue
		}
		for tier, count := range t.Counts {
			if !tierSet[tier] {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Column: "tier:" + tier, Code: "unknown_tier", Args: []string{tier},
				})
				continue
			}
			if count == nil || *count < 0 {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Column: "tier:" + tier, Code: "negative_target", Args: []string{t.ClassName, tier},
				})
				continue
			}
			key := fmt.Sprintf("%d\x00%s", cid, tier)
			if seen[key] {
				preview.Issues = append(preview.Issues, ImportIssue{
					Severity: "blocking", Column: "tier:" + tier, Code: "duplicate_target", Args: []string{t.ClassName, tier},
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

func (s *Store) examImportStatements(ctx context.Context, ec ExamCohort, req ExamImportRequest, preview ImportPreview, runID int64) (ImportSummary, []sqlite.Statement, []importChange, error) {
	summary := ImportSummary{}
	var statements []sqlite.Statement
	var changes []importChange
	phase := phaseForStage(ec.GradeLevelAtExam)
	now := s.now().Format(time.RFC3339Nano)

	switch req.Kind {
	case ExamImportRoster:
		classes, err := s.phaseClassIDMap(ctx, ec.CohortID, phase)
		if err != nil {
			return summary, nil, nil, err
		}
		existingRoster, err := s.existingExamParticipantsMap(ctx, ec.ID)
		if err != nil {
			return summary, nil, nil, err
		}
		for _, entry := range req.Data.Roster {
			cname := normalizeClassName(entry.ClassName)
			classID := classes[cname]
			if classID == 0 && cname != "" {
				created, cErr := s.CreateClass(ctx, ec.CohortID, phase, cname, "", "", nil)
				if cErr != nil {
					return summary, nil, nil, cErr
				}
				classID = created.ID
				classes[cname] = classID
			}
			studentID, _, matchErr := s.MatchStudent(ctx, ec.CohortID, phase, entry.StudentNo, entry.CandidateNo, entry.Name, cname)
			if matchErr != nil {
				if matchErr != ErrNotFound {
					continue
				}
				// 按需在学生表补建档案。
				track := normalizeTrackValue(entry.Track)
				if track == "" {
					track = trackFromElective(entry.Elective)
				}
				status := normalizeStudentStatus(entry.Status)
				res, iErr := s.db.Execute(ctx, `INSERT INTO students(
					cohort_id, student_no, candidate_code, name, current_class_id, current_track,
					elective_combo, foreign_lang, status, import_run_id, source_sheet, source_row, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, 'english', ?, ?, ?, ?, ?, ?)`,
					ec.CohortID, strings.TrimSpace(entry.StudentNo), strings.TrimSpace(entry.CandidateNo),
					strings.TrimSpace(entry.Name), nullableID(classID), track, strings.TrimSpace(entry.Elective),
					status, runID, entry.SourceSheet, entry.SourceRow, now, now)
				if iErr != nil {
					return summary, nil, nil, iErr
				}
				studentID = res.LastInsertID
			}

			track := normalizeTrackValue(entry.Track)
			if track == "" {
				track = trackFromElective(entry.Elective)
			}
			elective := strings.TrimSpace(entry.Elective)
			status := normalizeStudentStatus(entry.Status)
			candNo := strings.TrimSpace(entry.CandidateNo)
			fl := "english"
			// 报名表未提供班级、选科、组合、考生号或外语语种时，沿用学生档案中的已有字段。
			stRows, _ := s.db.Query(ctx, `SELECT COALESCE(st.current_class_id, 0), COALESCE(c.name, ''), st.current_track, st.elective_combo, st.foreign_lang, st.status, st.candidate_code
				FROM students st LEFT JOIN classes c ON c.id = st.current_class_id WHERE st.id = ?`, studentID)
			if len(stRows.Values) > 0 {
				if classID == 0 {
					classID = intValue(stRows.Values[0][0])
				}
				if cname == "" {
					cname = stringValue(stRows.Values[0][1])
				}
				if track == "" {
					track = stringValue(stRows.Values[0][2])
				}
				if elective == "" {
					elective = stringValue(stRows.Values[0][3])
				}
				if storedFL := stringValue(stRows.Values[0][4]); storedFL != "" {
					fl = storedFL
				}
				if strings.TrimSpace(entry.Status) == "" && stringValue(stRows.Values[0][5]) != "" {
					status = stringValue(stRows.Values[0][5])
				}
				if candNo == "" {
					candNo = stringValue(stRows.Values[0][6])
				}
			}

			if before, exists := existingRoster[studentID]; exists {
				changes = append(changes, importChange{
					TableName: "exam_participants",
					RowID:     before.ID,
					Action:    "update",
					Before:    before.payload,
				})
			} else {
				changes = append(changes, importChange{
					TableName: "exam_participants",
					RowID:     studentID,
					Action:    "insert",
					Before:    map[string]any{"exam_cohort_id": ec.ID, "student_id": studentID},
				})
			}

			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_participants(
					exam_cohort_id, student_id, class_id, class_name_snapshot,
					track_snapshot, elective_snapshot, foreign_lang_snapshot, status_snapshot,
					exam_no, candidate_no, total_score, is_valid_for_calc, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
					ON CONFLICT(exam_cohort_id, student_id) DO UPDATE SET
						class_id = excluded.class_id,
						class_name_snapshot = excluded.class_name_snapshot,
						track_snapshot = CASE WHEN excluded.track_snapshot <> '' THEN excluded.track_snapshot ELSE exam_participants.track_snapshot END,
						elective_snapshot = CASE WHEN excluded.elective_snapshot <> '' THEN excluded.elective_snapshot ELSE exam_participants.elective_snapshot END,
						foreign_lang_snapshot = excluded.foreign_lang_snapshot,
						status_snapshot = excluded.status_snapshot,
						exam_no = CASE WHEN excluded.exam_no <> '' THEN excluded.exam_no ELSE exam_participants.exam_no END,
						candidate_no = CASE WHEN excluded.candidate_no <> '' THEN excluded.candidate_no ELSE exam_participants.candidate_no END,
						total_score = COALESCE(excluded.total_score, exam_participants.total_score),
						is_valid_for_calc = excluded.is_valid_for_calc,
						import_run_id = excluded.import_run_id,
						source_sheet = excluded.source_sheet,
						source_row = excluded.source_row`,
				Args: []any{ec.ID, studentID, nullableID(classID), cname, track,
					elective, fl, status, strings.TrimSpace(entry.ExamNo), candNo,
					optionalFloat(entry.TotalScore), boolInt(isParticipantValidStatus(status)),
					runID, entry.SourceSheet, entry.SourceRow},
			})
		}
		summary.Inserted = preview.New
		summary.Updated = preview.Update
		summary.Students = preview.New + preview.Update

	case ExamImportScores:
		roster, err := s.examRosterUserIndex(ctx, ec)
		if err != nil {
			return summary, nil, nil, err
		}
		existingScores, err := s.existingExamScoreKeys(ctx, ec.ID)
		if err != nil {
			return summary, nil, nil, err
		}
		seenChange := map[string]bool{}
		for _, st := range req.Data.Students {
			entry, ok := roster.findMulti(st.StudentNo, st.CandidateCode, st.Name, st.ClassName)
			if !ok {
				continue
			}
			if st.TotalScore != nil {
				statements = append(statements, sqlite.Statement{
					SQL:  `UPDATE exam_participants SET total_score = ? WHERE id = ?`,
					Args: []any{*st.TotalScore, entry.ID},
				})
			}
			writeScore := func(subj string, val *float64, flag string) {
				key := fmt.Sprintf("%d\x00%s", entry.ID, subj)
				if !seenChange[key] {
					seenChange[key] = true
					if existingScores[key] {
						changes = append(changes, importChange{
							TableName: "exam_scores", Action: "update",
							Before: map[string]any{"exam_participant_id": entry.ID, "subject": subj},
						})
					} else {
						changes = append(changes, importChange{
							TableName: "exam_scores", Action: "insert",
							Before: map[string]any{"exam_participant_id": entry.ID, "subject": subj},
						})
					}
				}
				tgSnap := s.resolveStudentSubjectTeachingGroup(ctx, ec.CohortID, phase, entry.StudentID, entry.ClassID, subj)
				statements = append(statements, sqlite.Statement{
					SQL: `INSERT INTO exam_scores(
						exam_participant_id, subject, score, flag, teaching_group_id_snapshot,
						import_run_id, source_sheet, source_row)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?)
						ON CONFLICT(exam_participant_id, subject) DO UPDATE SET
							score = excluded.score,
							flag = excluded.flag,
							teaching_group_id_snapshot = COALESCE(excluded.teaching_group_id_snapshot, exam_scores.teaching_group_id_snapshot),
							import_run_id = excluded.import_run_id,
							source_sheet = excluded.source_sheet,
							source_row = excluded.source_row`,
					Args: []any{entry.ID, subj, optionalFloat(val), flag, nullableID(tgSnap),
						runID, st.SourceSheet, st.SourceRow},
				})
			}
			for subj, sc := range st.Scores {
				v := sc
				writeScore(subj, &v, "")
			}
			for subj, fl := range st.Flags {
				if fl == "zero" || subj == "total" {
					continue
				}
				writeScore(subj, nil, fl)
			}
		}
		summary.Inserted = preview.New
		summary.Updated = preview.Update
		summary.Students = preview.New + preview.Update

	case ExamImportCutoffs:
		existing, err := s.existingCutoffMap(ctx, ec.ID)
		if err != nil {
			return summary, nil, nil, err
		}
		for _, c := range req.Data.SubjectCutoffs {
			key := "subject\x00" + c.Subject + "\x00" + c.Track
			if prev, ok := existing[key]; ok {
				changes = append(changes, importChange{
					TableName: "exam_cutoffs", Action: "update",
					Before: map[string]any{"exam_cohort_id": ec.ID, "kind": "subject", "key": c.Subject, "track": c.Track, "cutoff": prev},
				})
			} else {
				changes = append(changes, importChange{
					TableName: "exam_cutoffs", Action: "insert",
					Before: map[string]any{"exam_cohort_id": ec.ID, "kind": "subject", "key": c.Subject, "track": c.Track},
				})
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_cutoffs(exam_cohort_id, kind, "key", track, cutoff)
					VALUES (?, 'subject', ?, ?, ?)
					ON CONFLICT(exam_cohort_id, kind, "key", track) DO UPDATE SET cutoff = excluded.cutoff`,
				Args: []any{ec.ID, c.Subject, c.Track, c.Value},
			})
			summary.SubjectCutoffs++
		}
		for _, c := range req.Data.TierCutoffs {
			key := "tier\x00" + c.TierKey + "\x00" + c.Track
			if prev, ok := existing[key]; ok {
				changes = append(changes, importChange{
					TableName: "exam_cutoffs", Action: "update",
					Before: map[string]any{"exam_cohort_id": ec.ID, "kind": "tier", "key": c.TierKey, "track": c.Track, "cutoff": prev},
				})
			} else {
				changes = append(changes, importChange{
					TableName: "exam_cutoffs", Action: "insert",
					Before: map[string]any{"exam_cohort_id": ec.ID, "kind": "tier", "key": c.TierKey, "track": c.Track},
				})
			}
			statements = append(statements, sqlite.Statement{
				SQL: `INSERT INTO exam_cutoffs(exam_cohort_id, kind, "key", track, cutoff)
					VALUES (?, 'tier', ?, ?, ?)
					ON CONFLICT(exam_cohort_id, kind, "key", track) DO UPDATE SET cutoff = excluded.cutoff`,
				Args: []any{ec.ID, c.TierKey, c.Track, c.Value},
			})
			summary.TierCutoffs++
		}
		summary.Inserted = preview.New
		summary.Updated = preview.Update

	case ExamImportTargets:
		classes, err := s.phaseClassIDMap(ctx, ec.CohortID, phase)
		if err != nil {
			return summary, nil, nil, err
		}
		assessment, err := s.AssessmentForExamCohort(ctx, ec.ID)
		if err != nil {
			return summary, nil, nil, err
		}
		for _, t := range req.Data.ClassTargets {
			cid := classes[normalizeClassName(t.ClassName)]
			if cid == 0 {
				continue
			}
			for tier, cnt := range t.Counts {
				var actualArg any
				if ac := t.ActualCounts[tier]; ac != nil {
					actualArg = *ac
				}
				changes = append(changes, importChange{
					TableName: "assessment_class_targets", Action: "insert",
					Before: map[string]any{"assessment_id": assessment.ID, "class_id": cid, "tier_key": tier},
				})
				statements = append(statements, sqlite.Statement{
					SQL: `INSERT INTO assessment_class_targets(assessment_id, class_id, tier_key, target_count, actual_count)
						VALUES (?, ?, ?, ?, ?)
						ON CONFLICT(assessment_id, class_id, tier_key) DO UPDATE SET
							target_count = excluded.target_count,
							actual_count = COALESCE(excluded.actual_count, assessment_class_targets.actual_count)`,
					Args: []any{assessment.ID, cid, tier, optionalFloat(cnt), actualArg},
				})
				summary.ClassTargets++
			}
		}
		summary.Inserted = preview.New
		summary.Updated = preview.Update
	}
	return summary, statements, changes, nil
}

type existingParticipantSnap struct {
	ID      int64
	payload map[string]any
}

func (s *Store) existingExamParticipantsMap(ctx context.Context, ecID int64) (map[int64]existingParticipantSnap, error) {
	rows, err := s.db.Query(ctx, `SELECT id, student_id, COALESCE(class_id, 0), class_name_snapshot,
		track_snapshot, elective_snapshot, status_snapshot, exam_no, total_score
		FROM exam_participants WHERE exam_cohort_id = ?`, ecID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]existingParticipantSnap, len(rows.Values))
	for _, r := range rows.Values {
		sid := intValue(r[1])
		tot, _ := nullableFloat(r[8])
		out[sid] = existingParticipantSnap{
			ID: intValue(r[0]),
			payload: map[string]any{
				"exam_cohort_id": ecID,
				"student_id":     sid,
				"class_id":       intValue(r[2]),
				"class_name":     stringValue(r[3]),
				"track":          stringValue(r[4]),
				"elective":       stringValue(r[5]),
				"status":         stringValue(r[6]),
				"exam_no":        stringValue(r[7]),
				"total_score":    tot,
			},
		}
	}
	return out, nil
}

func (s *Store) existingExamScoreKeys(ctx context.Context, ecID int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT es.exam_participant_id, es.subject FROM exam_scores es
		JOIN exam_participants ep ON ep.id = es.exam_participant_id WHERE ep.exam_cohort_id = ?`, ecID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows.Values))
	for _, r := range rows.Values {
		out[fmt.Sprintf("%d\x00%s", intValue(r[0]), stringValue(r[1]))] = true
	}
	return out, nil
}

func (s *Store) existingCutoffMap(ctx context.Context, ecID int64) (map[string]float64, error) {
	rows, err := s.db.Query(ctx, `SELECT kind, "key", track, cutoff FROM exam_cutoffs WHERE exam_cohort_id = ?`, ecID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(rows.Values))
	for _, r := range rows.Values {
		out[stringValue(r[0])+"\x00"+stringValue(r[1])+"\x00"+stringValue(r[2])] = floatValue(r[3])
	}
	return out, nil
}

func (s *Store) subjectFullMarksMap(ctx context.Context) (map[string]float64, error) {
	rows, err := s.db.Query(ctx, `SELECT "key", full_mark FROM subjects WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(rows.Values))
	for _, r := range rows.Values {
		out[stringValue(r[0])] = floatValue(r[1])
	}
	return out, nil
}

func (s *Store) trackKeySet(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT "key" FROM tracks WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows.Values))
	for _, r := range rows.Values {
		out[stringValue(r[0])] = true
	}
	return out, nil
}

func mustJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
