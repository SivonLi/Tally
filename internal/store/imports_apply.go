package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// MergeStageRoster 把学生名单合并写入指定年级阶段（兼容旧入口，不强制去重或写可撤销账本）。
func (s *Store) MergeStageRoster(ctx context.Context, cohortID, stageID int64, data ImportData, report ImportReport) (ImportSummary, error) {
	return s.applyStageImportInternal(ctx, cohortID, stageID, StageImportRequest{
		Kind: "students", Data: data, Report: report,
	}, false, true)
}

// MergeStageAssignments 把教师与教学班任课关系合并写入指定年级阶段。
func (s *Store) MergeStageAssignments(ctx context.Context, cohortID, stageID int64, data ImportData, report ImportReport) (ImportSummary, error) {
	return s.applyStageImportInternal(ctx, cohortID, stageID, StageImportRequest{
		Kind: "assignments", Data: data, Report: report,
	}, false, true)
}

// MergeStageBaselines 把班级与年级入口基数写入指定年级阶段。
func (s *Store) MergeStageBaselines(ctx context.Context, cohortID, stageID int64, data ImportData, report ImportReport) (ImportSummary, error) {
	return s.applyStageImportInternal(ctx, cohortID, stageID, StageImportRequest{
		Kind: "baselines", Data: data, Report: report,
	}, false, true)
}

// ApplyStageImport 校验并原子写入阶段基础数据导入，保存撤销所需的变更记录和校验问题。
func (s *Store) ApplyStageImport(ctx context.Context, cohortID, stageID int64, req StageImportRequest) (ImportSummary, error) {
	return s.applyStageImportInternal(ctx, cohortID, stageID, req, true, false)
}

func (s *Store) applyStageImportInternal(ctx context.Context, cohortID, stageID int64, req StageImportRequest, recordChanges, allowDuplicate bool) (ImportSummary, error) {
	cohort, phase, err := s.resolveCohortPhaseScope(ctx, cohortID, stageID)
	if err != nil {
		return ImportSummary{}, err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return ImportSummary{}, ErrCohortGraduated
	}
	resolvedStageID := EncodeStageID(cohort.ID, phase)
	preview, err := s.ValidateStageImport(ctx, cohort.ID, resolvedStageID, req)
	if err != nil {
		return ImportSummary{}, err
	}
	if preview.Blocking > 0 {
		return ImportSummary{}, fmt.Errorf("%w: %s", ErrImportBlocked, issueCodeSummary(preview.Issues))
	}

	runID, reused, err := s.beginImportRun(ctx, "cohort", resolvedStageID, req.Kind, req.Report, req.Mapping, preview, allowDuplicate)
	if err != nil {
		return ImportSummary{}, err
	}

	var changes []importChange
	switch req.Kind {
	case "classes":
		changes, err = s.applyClassImport(ctx, cohort.ID, phase, req, runID)
	case "students":
		changes, err = s.applyStudentImport(ctx, cohort.ID, phase, req, runID)
	case "teachers":
		changes, err = s.applyTeacherImport(ctx, req, runID)
	case "assignments":
		changes, err = s.applyAssignmentImport(ctx, cohort.ID, phase, req, runID)
	case "baselines":
		changes, err = s.applyBaselineImport(ctx, cohort.ID, phase, req, runID)
	default:
		err = fmt.Errorf("未知的数据项 %q", req.Kind)
	}
	if err != nil {
		if !reused {
			_ = s.deleteImportRun(ctx, runID)
		}
		return ImportSummary{}, err
	}

	now := s.now().Format(time.RFC3339Nano)
	var statements []sqlite.Statement
	if recordChanges {
		statements = append(statements, s.importChangeStatements(runID, changes)...)
	}
	statements = append(statements, s.importIssueStatements(runID, preview.Issues)...)
	statements = append(statements, s.importSheetStatements(runID, req.Report)...)
	statements = append(statements,
		bumpRevisionStatement(fmt.Sprintf("cohort:%d:%s", cohort.ID, phase), now),
		auditLogStatement("import_stage", "cohort", cohort.ID, map[string]any{
			"phase": phase, "kind": req.Kind, "run": runID,
		}, now),
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return ImportSummary{}, err
	}

	counts, err := s.StageCounts(ctx, cohort.ID, cohort.EnrollmentYear+PhaseIndex(phase)-1, phase)
	if err != nil {
		return ImportSummary{}, err
	}
	summary := ImportSummary{
		Classes:        counts.Classes,
		Students:       counts.Students,
		Teachers:       counts.Teachers,
		TeachingGroups: counts.TeachingGroups,
		Assignments:    counts.Assignments,
		Baselines:      counts.Baselines,
		Inserted:       preview.New,
		Updated:        preview.Update,
		Skipped:        preview.Skip,
	}
	for _, issue := range preview.Issues {
		if issue.Severity == "warning" {
			summary.Warnings = append(summary.Warnings, issue.Code)
		}
	}
	return summary, nil
}

func (s *Store) ensurePhaseClassWithChange(ctx context.Context, cohortID int64, phase, rawName string, runID int64, sheet string, row int, changes *[]importChange) (int64, error) {
	name := normalizeClassName(rawName)
	if name == "" {
		return 0, nil
	}
	cRows, err := s.db.Query(ctx, `SELECT id FROM classes WHERE cohort_id = ? AND phase = ? AND name = ?`, cohortID, phase, name)
	if err != nil {
		return 0, err
	}
	if len(cRows.Values) > 0 {
		return intValue(cRows.Values[0][0]), nil
	}
	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `INSERT INTO classes(
		cohort_id, phase, name, status, import_run_id, source_sheet, source_row, created_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?)`,
		cohortID, phase, name, runID, sheet, row, now)
	if err != nil {
		return 0, err
	}
	*changes = append(*changes, importChange{
		TableName: "classes",
		RowID:     res.LastInsertID,
		Action:    "insert",
		Before:    map[string]any{"cohort_id": cohortID, "phase": phase, "name": name},
	})
	return res.LastInsertID, nil
}

func (s *Store) applyClassImport(ctx context.Context, cohortID int64, phase string, req StageImportRequest, runID int64) ([]importChange, error) {
	var changes []importChange
	seen := map[string]bool{}
	for idx, raw := range req.Data.Classes {
		name := normalizeClassName(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if _, err := s.ensurePhaseClassWithChange(ctx, cohortID, phase, name, runID, "", idx+2, &changes); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

func (s *Store) applyStudentImport(ctx context.Context, cohortID int64, phase string, req StageImportRequest, runID int64) ([]importChange, error) {
	var changes []importChange
	now := s.now().Format(time.RFC3339Nano)
	seen := map[string]bool{}
	for _, st := range req.Data.Students {
		sno := strings.TrimSpace(st.StudentNo)
		cand := strings.TrimSpace(st.CandidateCode)
		name := strings.TrimSpace(st.Name)
		cname := normalizeClassName(st.ClassName)
		if name == "" || cname == "" {
			continue
		}
		key := sno
		if key == "" {
			key = cand + "\x00" + name + "\x00" + cname
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		classID, err := s.ensurePhaseClassWithChange(ctx, cohortID, phase, cname, runID, st.SourceSheet, st.SourceRow, &changes)
		if err != nil {
			return nil, err
		}
		track := normalizeTrackValue(st.Track)
		if track == "" {
			track = trackFromElective(st.Elective)
		}
		fl := "english"
		if st.Japanese {
			fl = "japanese"
		}
		status := normalizeStudentStatus(st.Status)

		existingID, _, matchErr := s.MatchStudent(ctx, cohortID, phase, sno, cand, name, cname)
		var studentID int64
		if matchErr == nil && existingID > 0 {
			studentID = existingID
			prevRows, err := s.db.Query(ctx, `SELECT COALESCE(current_class_id, 0), current_track, elective_combo, status
				FROM students WHERE id = ?`, existingID)
			if err != nil {
				return nil, err
			}
			var oldClassID int64
			var oldTrack, oldElective, oldStatus string
			if len(prevRows.Values) > 0 {
				oldClassID = intValue(prevRows.Values[0][0])
				oldTrack = stringValue(prevRows.Values[0][1])
				oldElective = stringValue(prevRows.Values[0][2])
				oldStatus = stringValue(prevRows.Values[0][3])
			}
			changes = append(changes, importChange{
				TableName: "students",
				RowID:     existingID,
				Action:    "update",
				Before: map[string]any{
					"class_id": oldClassID, "track": oldTrack, "elective": oldElective, "status": oldStatus,
				},
			})
			if _, err := s.db.Execute(ctx, `UPDATE students SET
				candidate_code = CASE WHEN ? <> '' THEN ? ELSE candidate_code END,
				name = ?,
				gender = CASE WHEN ? <> '' THEN ? ELSE gender END,
				current_class_id = ?,
				current_track = CASE WHEN ? <> '' THEN ? ELSE current_track END,
				elective_combo = CASE WHEN ? <> '' THEN ? ELSE elective_combo END,
				foreign_lang = ?,
				status = ?,
				entry_total_score = COALESCE(?, entry_total_score),
				previous_student_id = COALESCE(?, previous_student_id),
				import_run_id = ?,
				source_sheet = ?,
				source_row = ?,
				updated_at = ?
				WHERE id = ?`,
				cand, cand, name, st.Gender, st.Gender, classID, track, track,
				strings.TrimSpace(st.Elective), strings.TrimSpace(st.Elective), fl, status,
				optionalFloat(st.TotalScore), nullableID(idPtrVal(st.PreviousStudentID)),
				runID, st.SourceSheet, st.SourceRow, now, existingID); err != nil {
				return nil, err
			}
			// 记录关键属性变动事件。
			if oldClassID != classID {
				if _, err := s.db.Execute(ctx, `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
					VALUES (?, ?, ?, 'class_changed', ?, ?, '名单更新调班', ?)`,
					studentID, cohortID, phase, mustJSONString(map[string]any{"class_id": oldClassID}),
					mustJSONString(map[string]any{"class_id": classID, "class_name": cname}), now); err != nil {
					return nil, err
				}
			}
			if oldTrack != "" && track != "" && oldTrack != track {
				if _, err := s.db.Execute(ctx, `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
					VALUES (?, ?, ?, 'track_changed', ?, ?, '选科类别变更', ?)`,
					studentID, cohortID, phase, mustJSONString(map[string]any{"track": oldTrack}),
					mustJSONString(map[string]any{"track": track}), now); err != nil {
					return nil, err
				}
			}
			if oldStatus != "" && oldStatus != status {
				if _, err := s.db.Execute(ctx, `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
					VALUES (?, ?, ?, 'status_changed', ?, ?, '学籍状态变更', ?)`,
					studentID, cohortID, phase, mustJSONString(map[string]any{"status": oldStatus}),
					mustJSONString(map[string]any{"status": status}), now); err != nil {
					return nil, err
				}
			}
		} else if errors.Is(matchErr, ErrNotFound) {
			res, err := s.db.Execute(ctx, `INSERT INTO students(
				cohort_id, student_no, candidate_code, name, gender, current_class_id,
				current_track, elective_combo, foreign_lang, status, entry_total_score, previous_student_id,
				import_run_id, source_sheet, source_row, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				cohortID, sno, cand, name, strings.TrimSpace(st.Gender), classID,
				track, strings.TrimSpace(st.Elective), fl, status, optionalFloat(st.TotalScore),
				nullableID(idPtrVal(st.PreviousStudentID)), runID, st.SourceSheet, st.SourceRow, now, now)
			if err != nil {
				return nil, err
			}
			studentID = res.LastInsertID
			changes = append(changes, importChange{
				TableName: "students",
				RowID:     studentID,
				Action:    "insert",
				Before:    map[string]any{"student_no": sno, "name": name},
			})
			if _, err := s.db.Execute(ctx, `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
				VALUES (?, ?, ?, 'enrolled', '{}', ?, '名册建档入籍', ?)`,
				studentID, cohortID, phase, mustJSONString(map[string]any{"class_id": classID, "class_name": cname, "track": track}), now); err != nil {
				return nil, err
			}
		} else if matchErr != nil {
			return nil, matchErr
		}

		// 若名单中随附了单科入学/中考基数成绩，同步落入 student_entry_scores。
		if studentID > 0 && len(st.Scores) > 0 {
			for subj, sc := range st.Scores {
				if _, err := s.db.Execute(ctx, `INSERT INTO student_entry_scores(
					student_id, subject, score, import_run_id, source_sheet, source_row)
					VALUES (?, ?, ?, ?, ?, ?)
					ON CONFLICT(student_id, subject) DO UPDATE SET
						score = excluded.score, import_run_id = excluded.import_run_id`,
					studentID, subj, sc, runID, st.SourceSheet, st.SourceRow); err != nil {
					return nil, err
				}
			}
			changes = append(changes, importChange{
				TableName: "student_entry_scores", Action: "insert", Before: map[string]any{"student_id": studentID},
			})
		}
	}
	return changes, nil
}

func (s *Store) applyTeacherImport(ctx context.Context, req StageImportRequest, runID int64) ([]importChange, error) {
	var changes []importChange
	for _, t := range req.Data.TeacherList {
		if _, err := s.ensureTeacherWithChange(ctx, t.TeacherNo, t.Name, t.Subject, t.Note, runID, t.SourceSheet, t.SourceRow, &changes); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

func (s *Store) ensureTeacherWithChange(ctx context.Context, teacherNo, name, subject, note string, runID int64, sheet string, row int, changes *[]importChange) (int64, error) {
	teacherNo = strings.TrimSpace(teacherNo)
	name = strings.TrimSpace(name)
	subject = strings.TrimSpace(subject)
	if name == "" {
		return 0, fmt.Errorf("教师姓名不能为空")
	}
	now := s.now().Format(time.RFC3339Nano)
	tid := s.lookupTeacherID(ctx, teacherNo, name, subject)
	if tid > 0 {
		if subject != "" {
			if _, err := s.db.Execute(ctx, `UPDATE teachers SET
				primary_subject = CASE WHEN primary_subject = '' THEN ? ELSE primary_subject END,
				updated_at = ? WHERE id = ?`, subject, now, tid); err != nil {
				return 0, err
			}
		}
		return tid, nil
	}
	res, err := s.db.Execute(ctx, `INSERT INTO teachers(
		teacher_no, name, primary_subject, status, note, import_run_id, source_sheet, source_row, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?, ?, ?)`,
		teacherNo, name, subject, strings.TrimSpace(note), runID, sheet, row, now, now)
	if err != nil {
		return 0, err
	}
	*changes = append(*changes, importChange{
		TableName: "teachers", RowID: res.LastInsertID, Action: "insert",
		Before: map[string]any{"name": name, "teacher_no": teacherNo},
	})
	return res.LastInsertID, nil
}

func (s *Store) applyAssignmentImport(ctx context.Context, cohortID int64, phase string, req StageImportRequest, runID int64) ([]importChange, error) {
	var changes []importChange
	now := s.now().Format(time.RFC3339Nano)
	seen := map[string]bool{}
	for _, a := range req.Data.Assignments {
		cname := normalizeClassName(a.ClassName)
		tname := strings.TrimSpace(a.TeacherName)
		if cname == "" || a.Subject == "" || tname == "" {
			continue
		}
		key := a.Subject + "\x00" + cname + "\x00" + tname
		if seen[key] {
			continue
		}
		seen[key] = true

		classID, err := s.ensurePhaseClassWithChange(ctx, cohortID, phase, cname, runID, a.SourceSheet, a.SourceRow, &changes)
		if err != nil {
			return nil, err
		}
		teacherID, err := s.ensureTeacherWithChange(ctx, a.TeacherNo, tname, a.Subject, "", runID, a.SourceSheet, a.SourceRow, &changes)
		if err != nil {
			return nil, err
		}

		// 查找或创建对应的学科教学班。
		tgID := int64(0)
		tgRows, err := s.db.Query(ctx, `SELECT id FROM teaching_groups
			WHERE cohort_id = ? AND phase = ? AND subject = ? AND name = ?`, cohortID, phase, a.Subject, cname)
		if err != nil {
			return nil, err
		}
		if len(tgRows.Values) > 0 {
			tgID = intValue(tgRows.Values[0][0])
		} else {
			tgRes, err := s.db.Execute(ctx, `INSERT INTO teaching_groups(
				cohort_id, phase, subject, name, kind, admin_class_id, status, import_run_id, source_sheet, source_row, created_at)
				VALUES (?, ?, ?, ?, 'admin', ?, 'active', ?, ?, ?, ?)`,
				cohortID, phase, a.Subject, cname, classID, runID, a.SourceSheet, a.SourceRow, now)
			if err != nil {
				return nil, err
			}
			tgID = tgRes.LastInsertID
			changes = append(changes, importChange{
				TableName: "teaching_groups", RowID: tgID, Action: "insert",
				Before: map[string]any{"cohort_id": cohortID, "phase": phase, "subject": a.Subject, "name": cname},
			})
		}

		role := a.Role
		if role == "" {
			role = "primary"
		}
		exAssign, err := s.db.Query(ctx, `SELECT id FROM teaching_assignments
			WHERE teacher_id = ? AND teaching_group_id = ? AND role = ? AND status = 'active'`,
			teacherID, tgID, role)
		if err != nil {
			return nil, err
		}
		if len(exAssign.Values) == 0 {
			taRes, err := s.db.Execute(ctx, `INSERT INTO teaching_assignments(
				teacher_id, cohort_id, phase, teaching_group_id, role, status, started_at,
				import_run_id, source_sheet, source_row)
				VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?)`,
				teacherID, cohortID, phase, tgID, role, now, runID, a.SourceSheet, a.SourceRow)
			if err != nil {
				return nil, err
			}
			changes = append(changes, importChange{
				TableName: "teaching_assignments", RowID: taRes.LastInsertID, Action: "insert",
				Before: map[string]any{"teacher_id": teacherID, "teaching_group_id": tgID},
			})
		}
	}
	return changes, nil
}

func (s *Store) applyBaselineImport(ctx context.Context, cohortID int64, phase string, req StageImportRequest, runID int64) ([]importChange, error) {
	var changes []importChange
	classes, err := s.phaseClassIDMap(ctx, cohortID, phase)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, b := range req.Data.ClassBaselines {
		cname := normalizeClassName(b.ClassName)
		cid := classes[cname]
		if cid == 0 {
			continue
		}
		key := fmt.Sprintf("%d\x00%s", cid, b.Subject)
		if seen[key] {
			continue
		}
		seen[key] = true

		exRows, err := s.db.Query(ctx, `SELECT id, average, ratio FROM cohort_baseline_overrides
			WHERE cohort_id = ? AND phase = ? AND scope_type = 'class' AND scope_id = ? AND subject = ?`,
			cohortID, phase, cid, b.Subject)
		if err != nil {
			return nil, err
		}
		if len(exRows.Values) > 0 {
			ovID := intValue(exRows.Values[0][0])
			oldRatio, _ := nullableFloat(exRows.Values[0][2])
			changes = append(changes, importChange{
				TableName: "cohort_baseline_overrides", RowID: ovID, Action: "update",
				Before: map[string]any{"average": floatValue(exRows.Values[0][1]), "ratio": oldRatio},
			})
			if _, err := s.db.Execute(ctx, `UPDATE cohort_baseline_overrides
				SET average = ?, ratio = ?, import_run_id = ?, source_sheet = ?, source_row = ?
				WHERE id = ?`,
				b.Average, optionalFloat(b.Ratio), runID, b.SourceSheet, b.SourceRow, ovID); err != nil {
				return nil, err
			}
		} else {
			res, err := s.db.Execute(ctx, `INSERT INTO cohort_baseline_overrides(
				cohort_id, phase, scope_type, scope_id, subject, average, ratio, import_run_id, source_sheet, source_row)
				VALUES (?, ?, 'class', ?, ?, ?, ?, ?, ?, ?)`,
				cohortID, phase, cid, b.Subject, b.Average, optionalFloat(b.Ratio), runID, b.SourceSheet, b.SourceRow)
			if err != nil {
				return nil, err
			}
			changes = append(changes, importChange{
				TableName: "cohort_baseline_overrides", RowID: res.LastInsertID, Action: "insert",
				Before: map[string]any{"cohort_id": cohortID, "phase": phase, "scope_id": cid, "subject": b.Subject},
			})
		}
	}
	for subj, avg := range req.Data.GradeBaselines {
		exRows, err := s.db.Query(ctx, `SELECT id, average, ratio FROM cohort_baseline_overrides
			WHERE cohort_id = ? AND phase = ? AND scope_type = 'grade' AND scope_id = 0 AND subject = ?`, cohortID, phase, subj)
		if err != nil {
			return nil, err
		}
		if len(exRows.Values) > 0 {
			ovID := intValue(exRows.Values[0][0])
			changes = append(changes, importChange{
				TableName: "cohort_baseline_overrides", RowID: ovID, Action: "update",
				Before: map[string]any{"average": floatValue(exRows.Values[0][1]), "ratio": nil},
			})
			if _, err := s.db.Execute(ctx, `UPDATE cohort_baseline_overrides SET average = ?, import_run_id = ? WHERE id = ?`,
				avg, runID, ovID); err != nil {
				return nil, err
			}
		} else {
			res, err := s.db.Execute(ctx, `INSERT INTO cohort_baseline_overrides(
				cohort_id, phase, scope_type, scope_id, subject, average, import_run_id)
				VALUES (?, ?, 'grade', 0, ?, ?, ?)`, cohortID, phase, subj, avg, runID)
			if err != nil {
				return nil, err
			}
			changes = append(changes, importChange{
				TableName: "cohort_baseline_overrides", RowID: res.LastInsertID, Action: "insert",
				Before: map[string]any{"cohort_id": cohortID, "scope_type": "grade", "subject": subj},
			})
		}
	}
	return changes, nil
}

func idPtrVal(ptr *int64) int64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}
