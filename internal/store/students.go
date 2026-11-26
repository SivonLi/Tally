package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// ErrAmbiguousStudentMatch 表示仅按姓名和班级匹配到多名同名同班学生，需要人工确认。
var ErrAmbiguousStudentMatch = errors.New("姓名与班级存在重名，无法唯一匹配")

// ListStudents 按条件分页查询学生名单与当前所在班级、走班归属。
func (s *Store) ListStudents(ctx context.Context, filter StudentFilter) ([]DataStudent, int, error) {
	if filter.PageSize <= 0 {
		filter.PageSize = 200
	}
	if filter.Page < 0 {
		filter.Page = 0
	}
	var cohortID int64
	var phase string
	if filter.StageID > 0 {
		cohort, ph, err := s.resolveCohortPhaseScope(ctx, filter.CohortID, filter.StageID)
		if err != nil {
			return nil, 0, err
		}
		cohortID = cohort.ID
		phase = ph
	} else if filter.CohortID > 0 {
		cohort, err := s.GetCohort(ctx, filter.CohortID)
		if err != nil {
			return nil, 0, err
		}
		cohortID = cohort.ID
		phase = filter.Phase
		if phase == "" {
			phase = cohort.CurrentPhase
			if phase == PhaseGraduated {
				phase = PhaseG3
			}
		}
	} else {
		return nil, 0, fmt.Errorf("查询学生需要指定年级或阶段")
	}

	where := []string{"st.cohort_id = ?"}
	args := []any{cohortID}
	if filter.ClassID != 0 {
		where = append(where, `(st.current_class_id = ? OR EXISTS (
			SELECT 1 FROM exam_participants ep
			JOIN exam_cohorts ec ON ec.id = ep.exam_cohort_id
			WHERE ep.student_id = st.id AND ec.cohort_id = st.cohort_id AND ec.phase_at_exam = ? AND ep.class_id = ?))`)
		args = append(args, filter.ClassID, phase, filter.ClassID)
	}
	if filter.Track != "" {
		where = append(where, "st.current_track = ?")
		args = append(args, filter.Track)
	}
	if filter.Status != "" {
		where = append(where, "st.status = ?")
		args = append(args, filter.Status)
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		where = append(where, "(st.student_no LIKE ? OR st.candidate_code LIKE ? OR st.name LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}

	condition := strings.Join(where, " AND ")
	total, err := s.countQuery(ctx, `SELECT COUNT(*) FROM students st WHERE `+condition, args...)
	if err != nil {
		return nil, 0, err
	}

	queryArgs := append(append([]any{}, args...), filter.PageSize, filter.Page*filter.PageSize)
	rows, err := s.db.Query(ctx, `SELECT st.id, st.student_no, st.candidate_code, st.name, st.gender,
		st.cohort_id, COALESCE(st.current_class_id, 0), COALESCE(c.name, ''),
		st.current_track, st.elective_combo, st.foreign_lang, st.status,
		st.entry_total_score, st.previous_student_id,
		COALESCE(st.import_run_id, 0), st.source_sheet, st.source_row
		FROM students st
		LEFT JOIN classes c ON c.id = st.current_class_id
		WHERE `+condition+` ORDER BY st.student_no, st.id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}

	stageID := EncodeStageID(cohortID, phase)
	students := make([]DataStudent, 0, len(rows.Values))
	for _, row := range rows.Values {
		ds := dataStudentFromRow(row, stageID)
		students = append(students, ds)
	}
	if err := s.attachStudentTeachingGroups(ctx, cohortID, phase, students); err != nil {
		return nil, 0, err
	}
	return students, total, nil
}

// GetStudent 读取单名学生的详细档案及当前教学班归属。
func (s *Store) GetStudent(ctx context.Context, stageID, studentID int64) (DataStudent, error) {
	rows, err := s.db.Query(ctx, `SELECT st.id, st.student_no, st.candidate_code, st.name, st.gender,
		st.cohort_id, COALESCE(st.current_class_id, 0), COALESCE(c.name, ''),
		st.current_track, st.elective_combo, st.foreign_lang, st.status,
		st.entry_total_score, st.previous_student_id,
		COALESCE(st.import_run_id, 0), st.source_sheet, st.source_row
		FROM students st
		LEFT JOIN classes c ON c.id = st.current_class_id
		WHERE st.id = ?`, studentID)
	if err != nil {
		return DataStudent{}, err
	}
	if len(rows.Values) == 0 {
		return DataStudent{}, ErrNotFound
	}
	ds := dataStudentFromRow(rows.Values[0], stageID)
	phase := PhaseG1
	if _, ph, ok := DecodeStageID(stageID); ok {
		phase = ph
	} else if cohort, err := s.GetCohort(ctx, ds.CohortID); err == nil {
		phase = cohort.CurrentPhase
		if phase == PhaseGraduated {
			phase = PhaseG3
		}
		ds.StageID = EncodeStageID(ds.CohortID, phase)
	}
	slice := []DataStudent{ds}
	if err := s.attachStudentTeachingGroups(ctx, ds.CohortID, phase, slice); err != nil {
		return DataStudent{}, err
	}
	return slice[0], nil
}

func dataStudentFromRow(row []any, stageID int64) DataStudent {
	total, _ := nullableFloat(row[12])
	var prevID *int64
	if pv, ok := nullableInt64(row[13]); ok {
		prevID = &pv
	}
	foreignLang := stringValue(row[10])
	if foreignLang == "" {
		foreignLang = "english"
	}
	return DataStudent{
		ID:                intValue(row[0]),
		StudentNo:         stringValue(row[1]),
		CandidateCode:     stringValue(row[2]),
		Name:              stringValue(row[3]),
		Gender:            stringValue(row[4]),
		CohortID:          intValue(row[5]),
		StageID:           stageID,
		ClassID:           intValue(row[6]),
		ClassName:         stringValue(row[7]),
		Track:             stringValue(row[8]),
		Elective:          stringValue(row[9]),
		ForeignLang:       foreignLang,
		Status:            stringValue(row[11]),
		Japanese:          foreignLang == "japanese",
		TotalScore:        total,
		PreviousStudentID: prevID,
		ImportRunID:       intValue(row[14]),
		SourceSheet:       stringValue(row[15]),
		SourceRow:         int(intValue(row[16])),
	}
}

func (s *Store) attachStudentTeachingGroups(ctx context.Context, cohortID int64, phase string, students []DataStudent) error {
	if len(students) == 0 || !ValidSchoolPhase(phase) {
		return nil
	}
	// 查询本阶段行政教学班（按 admin_class_id 索引）和学生显式走班记录。
	adminRows, err := s.db.Query(ctx, `SELECT COALESCE(admin_class_id, 0), subject, name
		FROM teaching_groups WHERE cohort_id = ? AND phase = ? AND kind = 'admin' AND admin_class_id IS NOT NULL`,
		cohortID, phase)
	if err != nil {
		return err
	}
	adminByClass := map[int64]map[string]string{}
	for _, row := range adminRows.Values {
		cid := intValue(row[0])
		if adminByClass[cid] == nil {
			adminByClass[cid] = map[string]string{}
		}
		adminByClass[cid][stringValue(row[1])] = stringValue(row[2])
	}

	ids := make([]any, len(students))
	indexByID := make(map[int64]int, len(students))
	for i, st := range students {
		ids[i] = st.ID
		indexByID[st.ID] = i
	}
	walkRows, err := s.db.Query(ctx, `SELECT tgs.student_id, g.subject, g.name
		FROM teaching_group_students tgs
		JOIN teaching_groups g ON g.id = tgs.teaching_group_id
		WHERE tgs.cohort_id = ? AND tgs.phase = ? AND tgs.ended_at IS NULL
		AND tgs.student_id IN (`+placeholders(len(ids))+`)`,
		append([]any{cohortID, phase}, ids...)...)
	if err != nil {
		return err
	}
	walkingByStudent := map[int64]map[string]string{}
	for _, row := range walkRows.Values {
		sid := intValue(row[0])
		if walkingByStudent[sid] == nil {
			walkingByStudent[sid] = map[string]string{}
		}
		walkingByStudent[sid][stringValue(row[1])] = stringValue(row[2])
	}

	for i := range students {
		merged := map[string]string{}
		if adminMap := adminByClass[students[i].ClassID]; adminMap != nil {
			for subj, name := range adminMap {
				merged[subj] = name
			}
		}
		if walkMap := walkingByStudent[students[i].ID]; walkMap != nil {
			students[i].Walking = len(walkMap) > 0
			for subj, name := range walkMap {
				merged[subj] = name
			}
		}
		if len(merged) == 0 {
			continue
		}
		subjects := make([]string, 0, len(merged))
		for subj := range merged {
			subjects = append(subjects, subj)
		}
		sort.Strings(subjects)
		groups := make([]string, 0, len(subjects))
		for _, subj := range subjects {
			groups = append(groups, subj+":"+merged[subj])
		}
		students[i].TeachingGroups = groups
	}
	return nil
}

// MatchStudent 按 student_no > candidate_code > name+class_name 优先级在指定年级中唯一匹配学生。
func (s *Store) MatchStudent(ctx context.Context, cohortID int64, phase, studentNo, candidateCode, name, className string) (int64, string, error) {
	studentNo = strings.TrimSpace(studentNo)
	candidateCode = strings.TrimSpace(candidateCode)
	name = strings.TrimSpace(name)
	normClass := normalizeClassName(className)

	if studentNo != "" {
		rows, err := s.db.Query(ctx, `SELECT id FROM students WHERE cohort_id = ? AND student_no = ? LIMIT 1`,
			cohortID, studentNo)
		if err != nil {
			return 0, "", err
		}
		if len(rows.Values) > 0 {
			return intValue(rows.Values[0][0]), "student_no", nil
		}
	}
	if candidateCode != "" {
		rows, err := s.db.Query(ctx, `SELECT id FROM students WHERE cohort_id = ? AND candidate_code = ?`,
			cohortID, candidateCode)
		if err != nil {
			return 0, "", err
		}
		if len(rows.Values) == 1 {
			return intValue(rows.Values[0][0]), "candidate_code", nil
		}
		if len(rows.Values) > 1 {
			return 0, "", fmt.Errorf("%w: 考生号 %s 存在多条记录", ErrAmbiguousStudentMatch, candidateCode)
		}
	}
	if name != "" && normClass != "" {
		rows, err := s.db.Query(ctx, `SELECT st.id, st.student_no, st.candidate_code FROM students st
			LEFT JOIN classes c ON c.id = st.current_class_id
			WHERE st.cohort_id = ? AND st.name = ? AND (
				c.name = ? OR EXISTS (
					SELECT 1 FROM exam_participants ep
					JOIN exam_cohorts ec ON ec.id = ep.exam_cohort_id
					WHERE ep.student_id = st.id AND ec.cohort_id = st.cohort_id
					AND (? = '' OR ec.phase_at_exam = ?) AND ep.class_name_snapshot = ?
				)
			)`, cohortID, name, normClass, phase, phase, normClass)
		if err != nil {
			return 0, "", err
		}
		var compatible []int64
		for _, row := range rows.Values {
			existingNo := strings.TrimSpace(stringValue(row[1]))
			existingCand := strings.TrimSpace(stringValue(row[2]))
			if studentNo != "" && existingNo != "" && existingNo != studentNo {
				continue
			}
			if candidateCode != "" && existingCand != "" && existingCand != candidateCode {
				continue
			}
			compatible = append(compatible, intValue(row[0]))
		}
		if len(compatible) == 1 {
			return compatible[0], "name_class", nil
		}
		if len(compatible) > 1 {
			return 0, "", fmt.Errorf("%w: %s (%s)", ErrAmbiguousStudentMatch, name, normClass)
		}
	}
	return 0, "", ErrNotFound
}

// SetStudentPreviousLink 为复读生/留级生关联其前一届的学生档案 ID（previous_student_id）。
func (s *Store) SetStudentPreviousLink(ctx context.Context, studentID, previousStudentID int64) error {
	if studentID <= 0 || previousStudentID <= 0 || studentID == previousStudentID {
		return ErrStudentMerge
	}
	rows, err := s.db.Query(ctx, `SELECT id, cohort_id FROM students WHERE id IN (?, ?)`, studentID, previousStudentID)
	if err != nil {
		return err
	}
	if len(rows.Values) < 2 {
		return ErrNotFound
	}
	now := s.now().Format(time.RFC3339Nano)
	cohortID := intValue(rows.Values[0][1])
	for _, r := range rows.Values {
		if intValue(r[0]) == studentID {
			cohortID = intValue(r[1])
		}
	}
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return err
	}
	phase := cohort.CurrentPhase
	if phase == PhaseGraduated {
		phase = PhaseG3
	}
	_, err = s.db.Transaction(ctx, []sqlite.Statement{
		{
			SQL:  `UPDATE students SET previous_student_id = ?, updated_at = ? WHERE id = ?`,
			Args: []any{previousStudentID, now, studentID},
		},
		{
			SQL: `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
				VALUES (?, ?, ?, 'enrolled', '{}', ?, '关联往届复读/留级前档', ?)`,
			Args: []any{studentID, cohortID, phase, fmt.Sprintf(`{"previous_student_id":%d}`, previousStudentID), now},
		},
	})
	return err
}

// MergeStudents 在同一年级内把重复学生档案 sourceStudentID 合并入 targetStudentID。
func (s *Store) MergeStudents(ctx context.Context, targetStudentID, sourceStudentID int64) error {
	if targetStudentID <= 0 || sourceStudentID <= 0 || targetStudentID == sourceStudentID {
		return ErrStudentMerge
	}
	tRows, err := s.db.Query(ctx, `SELECT id, cohort_id, student_no, candidate_code, name, gender, entry_total_score, previous_student_id
		FROM students WHERE id IN (?, ?)`, targetStudentID, sourceStudentID)
	if err != nil {
		return err
	}
	if len(tRows.Values) != 2 {
		return ErrNotFound
	}
	type snap struct {
		cohortID          int64
		studentNo         string
		candidateCode     string
		name              string
		gender            string
		entryTotal        *float64
		previousStudentID *int64
	}
	var target, source snap
	for _, row := range tRows.Values {
		id := intValue(row[0])
		et, _ := nullableFloat(row[6])
		var prevID *int64
		if pv, ok := nullableInt64(row[7]); ok && pv > 0 {
			prevID = &pv
		}
		item := snap{
			cohortID:          intValue(row[1]),
			studentNo:         stringValue(row[2]),
			candidateCode:     stringValue(row[3]),
			name:              stringValue(row[4]),
			gender:            stringValue(row[5]),
			entryTotal:        et,
			previousStudentID: prevID,
		}
		if id == targetStudentID {
			target = item
		} else {
			source = item
		}
	}
	if target.cohortID != source.cohortID {
		return fmt.Errorf("%w: 仅支持同一年级内的重复档案合并", ErrStudentMerge)
	}
	cohort, err := s.GetCohort(ctx, target.cohortID)
	if err != nil {
		return err
	}
	phase := cohort.CurrentPhase
	if phase == PhaseGraduated {
		phase = PhaseG3
	}
	now := s.now().Format(time.RFC3339Nano)

	var statements []sqlite.Statement
	// 1. 补齐空缺基础信息（若目标学号为空且源学号非空，先清空源学号避免唯一索引冲突）。
	newStudentNo := target.studentNo
	if newStudentNo == "" && source.studentNo != "" {
		newStudentNo = source.studentNo
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE students SET student_no = '', updated_at = ? WHERE id = ?`,
			Args: []any{now, sourceStudentID},
		})
	}
	newCandidate := target.candidateCode
	if newCandidate == "" {
		newCandidate = source.candidateCode
	}
	newGender := target.gender
	if newGender == "" {
		newGender = source.gender
	}
	newEntryTotal := target.entryTotal
	if newEntryTotal == nil {
		newEntryTotal = source.entryTotal
	}
	newPrevStudentID := target.previousStudentID
	if newPrevStudentID == nil && source.previousStudentID != nil && *source.previousStudentID > 0 && *source.previousStudentID != targetStudentID {
		newPrevStudentID = source.previousStudentID
	}
	statements = append(statements, sqlite.Statement{
		SQL: `UPDATE students SET student_no = ?, candidate_code = ?, gender = ?, entry_total_score = ?, previous_student_id = ?, updated_at = ?
			WHERE id = ?`,
		Args: []any{newStudentNo, newCandidate, newGender, optionalFloat(newEntryTotal), nullableID(idPtrVal(newPrevStudentID)), now, targetStudentID},
	})

	// 2. 迁移入学基数成绩与事件日志。
	statements = append(statements,
		sqlite.Statement{
			SQL: `INSERT OR IGNORE INTO student_entry_scores(student_id, subject, score, import_run_id, source_sheet, source_row)
				SELECT ?, subject, score, import_run_id, source_sheet, source_row
				FROM student_entry_scores WHERE student_id = ?`,
			Args: []any{targetStudentID, sourceStudentID},
		},
		sqlite.Statement{
			SQL: `INSERT OR IGNORE INTO teaching_group_students(teaching_group_id, student_id, cohort_id, phase, subject, started_at, ended_at)
				SELECT teaching_group_id, ?, cohort_id, phase, subject, started_at, ended_at
				FROM teaching_group_students WHERE student_id = ? AND NOT EXISTS (
					SELECT 1 FROM teaching_group_students existing
					WHERE existing.student_id = ? AND existing.cohort_id = teaching_group_students.cohort_id
					AND existing.phase = teaching_group_students.phase AND existing.subject = teaching_group_students.subject
					AND existing.ended_at IS NULL
				)`,
			Args: []any{targetStudentID, sourceStudentID, targetStudentID},
		},
	)

	// 3. 处理 exam_participants：目标已参考的场次保留目标，未参考的场次改挂到目标学生。
	statements = append(statements,
		sqlite.Statement{
			SQL: `DELETE FROM exam_participants WHERE student_id = ? AND exam_cohort_id IN (
				SELECT exam_cohort_id FROM exam_participants WHERE student_id = ?
			)`,
			Args: []any{sourceStudentID, targetStudentID},
		},
		sqlite.Statement{
			SQL:  `UPDATE exam_participants SET student_id = ? WHERE student_id = ?`,
			Args: []any{targetStudentID, sourceStudentID},
		},
		sqlite.Statement{
			SQL:  `UPDATE student_events SET student_id = ? WHERE student_id = ?`,
			Args: []any{targetStudentID, sourceStudentID},
		},
		sqlite.Statement{
			SQL:  `UPDATE students SET previous_student_id = ? WHERE previous_student_id = ?`,
			Args: []any{targetStudentID, sourceStudentID},
		},
		sqlite.Statement{
			SQL: `INSERT INTO student_events(student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
				VALUES (?, ?, ?, 'merged', ?, ?, '合并重复学生档案', ?)`,
			Args: []any{
				targetStudentID, target.cohortID, phase,
				mustJSON(map[string]any{"source_student_id": sourceStudentID, "student_no": source.studentNo, "name": source.name}),
				mustJSON(map[string]any{"target_student_id": targetStudentID, "student_no": newStudentNo, "name": target.name}),
				now,
			},
		},
		sqlite.Statement{
			SQL:  `DELETE FROM students WHERE id = ?`,
			Args: []any{sourceStudentID},
		},
		bumpRevisionStatement(fmt.Sprintf("cohort:%d", target.cohortID), now),
	)
	_, err = s.db.Transaction(ctx, statements)
	return err
}

// ListBaselines 返回按班级和学科聚合的入学/中考基数视图。
func (s *Store) ListBaselines(ctx context.Context, stageID, classID int64, query string) ([]BaselineRow, error) {
	cohort, phase, err := s.resolveCohortPhaseScope(ctx, 0, stageID)
	if err != nil {
		return nil, err
	}
	classRows, err := s.db.Query(ctx, `SELECT id, name FROM classes WHERE cohort_id = ? AND phase = ? ORDER BY name`,
		cohort.ID, phase)
	if err != nil {
		return nil, err
	}
	classNames := map[int64]string{}
	for _, r := range classRows.Values {
		classNames[intValue(r[0])] = stringValue(r[1])
	}

	// 读取年级平均作比值分母（先查 override，否则按学生入学成绩总平均计算）。
	gradeAvgs := map[string]float64{}
	ovGradeRows, err := s.db.Query(ctx, `SELECT subject, average, phase FROM cohort_baseline_overrides
		WHERE cohort_id = ? AND scope_type = 'grade'
		ORDER BY CASE phase WHEN '' THEN 0 WHEN 'g1' THEN 1 WHEN 'g2' THEN 2 WHEN 'g3' THEN 3 ELSE 4 END ASC, id ASC`, cohort.ID)
	if err != nil {
		return nil, err
	}
	targetPhaseIdx := PhaseIndex(phase)
	for _, r := range ovGradeRows.Values {
		ovPhase := stringValue(r[2])
		if ovPhase != "" && targetPhaseIdx > 0 && PhaseIndex(ovPhase) > targetPhaseIdx {
			continue
		}
		gradeAvgs[stringValue(r[0])] = floatValue(r[1])
	}
	stuGradeRows, err := s.db.Query(ctx, `SELECT es.subject, AVG(es.score)
		FROM student_entry_scores es
		JOIN students st ON st.id = es.student_id
		WHERE st.cohort_id = ? AND st.status IN ('active','graduated')
		GROUP BY es.subject`, cohort.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range stuGradeRows.Values {
		subj := stringValue(r[0])
		if _, ok := gradeAvgs[subj]; !ok {
			gradeAvgs[subj] = floatValue(r[1])
		}
	}

	// 1. 聚合当前/对应阶段班级学生入学成绩。
	itemsByKey := map[string]BaselineRow{}
	aggRows, err := s.db.Query(ctx, `SELECT c.id, c.name, es.subject, AVG(es.score), COUNT(*),
		COALESCE(MAX(es.import_run_id), 0), COALESCE(MAX(es.source_sheet), ''), COALESCE(MAX(es.source_row), 0)
		FROM student_entry_scores es
		JOIN students st ON st.id = es.student_id
		JOIN classes c ON c.id = st.current_class_id
		WHERE st.cohort_id = ? AND c.phase = ?
		GROUP BY c.id, c.name, es.subject`, cohort.ID, phase)
	if err != nil {
		return nil, err
	}
	for _, r := range aggRows.Values {
		cid := intValue(r[0])
		cname := stringValue(r[1])
		subj := stringValue(r[2])
		avg := floatValue(r[3])
		cnt := int(intValue(r[4]))
		ratio := 0.0
		if ga := gradeAvgs[subj]; ga > 0 {
			ratio = avg / ga
		}
		key := fmt.Sprintf("%d\x00%s", cid, subj)
		itemsByKey[key] = BaselineRow{
			BaselineSetID: cohort.ID,
			SetName:       formatCohortLabel(cohort.EnrollmentYear) + "基数",
			ClassID:       cid,
			ClassName:     cname,
			Subject:       subj,
			Average:       avg,
			Count:         cnt,
			Ratio:         ratio,
			ImportRunID:   intValue(r[5]),
			SourceSheet:   stringValue(r[6]),
			SourceRow:     int(intValue(r[7])),
		}
	}

	// 2. 应用班级手工/导入覆盖值（cohort_baseline_overrides，沿 continued_from_class_id 解析到目标阶段）。
	classResolve, err := s.resolveCohortClassToPhaseMap(ctx, cohort.ID, phase)
	if err != nil {
		return nil, err
	}
	ovRows, err := s.db.Query(ctx, `SELECT ov.scope_id, ov.subject, ov.average, ov.ratio,
		COALESCE(ov.import_run_id, 0), ov.source_sheet, ov.source_row, ov.phase
		FROM cohort_baseline_overrides ov
		WHERE ov.cohort_id = ? AND ov.scope_type = 'class'
		ORDER BY CASE ov.phase WHEN '' THEN 0 WHEN 'g1' THEN 1 WHEN 'g2' THEN 2 WHEN 'g3' THEN 3 ELSE 4 END ASC, ov.id ASC`, cohort.ID)
	if err != nil {
		return nil, err
	}
	directApplied := map[string]bool{}
	for _, r := range ovRows.Values {
		ovPhase := stringValue(r[7])
		if ovPhase != "" && targetPhaseIdx > 0 && PhaseIndex(ovPhase) > targetPhaseIdx {
			continue
		}
		origCID := intValue(r[0])
		cid, ok := classResolve[origCID]
		if !ok || cid <= 0 {
			continue
		}
		cname := classNames[cid]
		subj := stringValue(r[1])
		key := fmt.Sprintf("%d\x00%s", cid, subj)
		isDirect := origCID == cid
		if !isDirect && directApplied[key] {
			continue
		}
		if isDirect {
			directApplied[key] = true
		}
		avg := floatValue(r[2])
		ratioPtr, hasRatio := nullableFloat(r[3])
		ratio := 0.0
		if hasRatio && ratioPtr != nil {
			ratio = *ratioPtr
		} else if ga := gradeAvgs[subj]; ga > 0 {
			ratio = avg / ga
		}
		existing := itemsByKey[key]
		cnt := existing.Count
		if cnt == 0 && cid > 0 {
			cnt, _ = s.countQuery(ctx, `SELECT COUNT(*) FROM students WHERE current_class_id = ?`, cid)
		}
		itemsByKey[key] = BaselineRow{
			BaselineSetID: cohort.ID,
			SetName:       formatCohortLabel(cohort.EnrollmentYear) + "基数",
			ClassID:       cid,
			ClassName:     cname,
			Subject:       subj,
			Average:       avg,
			Count:         cnt,
			Ratio:         ratio,
			ImportRunID:   intValue(r[4]),
			SourceSheet:   stringValue(r[5]),
			SourceRow:     int(intValue(r[6])),
		}
	}

	like := strings.ToLower(strings.TrimSpace(query))
	result := make([]BaselineRow, 0, len(itemsByKey))
	for _, item := range itemsByKey {
		if classID != 0 && item.ClassID != classID {
			continue
		}
		if like != "" && !strings.Contains(strings.ToLower(item.ClassName), like) &&
			!strings.Contains(strings.ToLower(item.Subject), like) &&
			!strings.Contains(strings.ToLower(item.SetName), like) {
			continue
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ClassName != result[j].ClassName {
			return result[i].ClassName < result[j].ClassName
		}
		return result[i].Subject < result[j].Subject
	})
	return result, nil
}

// CreateStudent 在指定年级下手工新增一名学生并写入初始学籍事件。
func (s *Store) CreateStudent(ctx context.Context, cohortID int64, input ImportStudent) (DataStudent, error) {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return DataStudent{}, err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return DataStudent{}, ErrCohortGraduated
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return DataStudent{}, fmt.Errorf("学生姓名不能为空")
	}
	studentNo := strings.TrimSpace(input.StudentNo)
	var classIDArg any
	var classID int64
	className := normalizeClassName(input.ClassName)
	if className != "" {
		cRows, cErr := s.db.Query(ctx, `SELECT id FROM classes WHERE cohort_id = ? AND phase = ? AND name = ? LIMIT 1`,
			cohortID, cohort.CurrentPhase, className)
		if cErr != nil {
			return DataStudent{}, cErr
		}
		if len(cRows.Values) > 0 {
			classID = intValue(cRows.Values[0][0])
			classIDArg = classID
		}
	}
	track := normalizeTrackValue(input.Track)
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "active"
	}
	foreignLang := "english"
	if input.Japanese {
		foreignLang = "japanese"
	}
	now := s.now().Format(time.RFC3339Nano)
	res, err := s.db.Execute(ctx, `INSERT INTO students(
		cohort_id, student_no, candidate_code, name, gender, current_class_id,
		current_track, elective_combo, foreign_lang, status, entry_total_score,
		previous_student_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cohortID, studentNo, strings.TrimSpace(input.CandidateCode), name, strings.TrimSpace(input.Gender),
		classIDArg, track, strings.TrimSpace(input.Elective), foreignLang, status,
		optionalFloat(input.TotalScore), nullableID(idPtrVal(input.PreviousStudentID)), now, now)
	if err != nil {
		return DataStudent{}, err
	}
	sid := res.LastInsertID
	if _, err := s.db.Execute(ctx, `INSERT INTO student_events(
		student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
		VALUES (?, ?, ?, 'enrolled', '{}', ?, '手工建档', ?)`,
		sid, cohortID, cohort.CurrentPhase, mustJSON(map[string]any{"class_id": classID, "class_name": className}), now); err != nil {
		return DataStudent{}, err
	}
	return s.GetStudent(ctx, EncodeStageID(cohortID, cohort.CurrentPhase), sid)
}

// UpsertBaselineOverride 新增或更新年级/班级的入口基数覆盖值。
func (s *Store) UpsertBaselineOverride(ctx context.Context, cohortID int64, phase, scopeType string, scopeID int64, subject string, average float64, ratio *float64) error {
	cohort, err := s.GetCohort(ctx, cohortID)
	if err != nil {
		return err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return ErrCohortGraduated
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return fmt.Errorf("学科不能为空")
	}
	if scopeType != "class" && scopeType != "grade" {
		return fmt.Errorf("无效基数作用域 %q", scopeType)
	}
	if scopeType == "grade" {
		scopeID = 0
	}
	if phase == "" {
		phase = cohort.CurrentPhase
	}
	var ratioArg any
	if ratio != nil && *ratio > 0 {
		ratioArg = *ratio
	}
	_, err = s.db.Execute(ctx, `INSERT INTO cohort_baseline_overrides(
		cohort_id, phase, scope_type, scope_id, subject, average, ratio, source_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, '手工编辑')
		ON CONFLICT(cohort_id, phase, scope_type, scope_id, subject) DO UPDATE SET
			average = excluded.average,
			ratio = excluded.ratio,
			source_name = excluded.source_name`,
		cohortID, phase, scopeType, scopeID, subject, average, ratioArg)
	return err
}

// AddStudentEvent 为学生记录学籍、分班、选科或状态异动事件，并同步更新学生当前档案状态。
func (s *Store) AddStudentEvent(ctx context.Context, studentID int64, input AddStudentEventInput) (StudentEvent, error) {
	st, err := s.GetStudent(ctx, 0, studentID)
	if err != nil {
		return StudentEvent{}, err
	}
	cohort, err := s.GetCohort(ctx, st.CohortID)
	if err != nil {
		return StudentEvent{}, err
	}
	if cohort.CurrentPhase == PhaseGraduated {
		return StudentEvent{}, ErrCohortGraduated
	}
	phase := strings.TrimSpace(input.Phase)
	if phase == "" {
		phase = cohort.CurrentPhase
	}
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		return StudentEvent{}, fmt.Errorf("异动类型不能为空")
	}
	occurredAt := s.now()
	if input.OccurredAt != nil && !input.OccurredAt.IsZero() {
		occurredAt = *input.OccurredAt
	}
	occurredStr := occurredAt.Format(time.RFC3339Nano)
	nowStr := s.now().Format(time.RFC3339Nano)
	note := strings.TrimSpace(input.Note)

	var fromJSON, toJSON string
	var statements []sqlite.Statement
	switch eventType {
	case "class_changed":
		if input.ToClassID <= 0 {
			return StudentEvent{}, fmt.Errorf("请选择目标班级")
		}
		cRows, cErr := s.db.Query(ctx, `SELECT id, name FROM classes WHERE id = ? AND cohort_id = ? LIMIT 1`,
			input.ToClassID, st.CohortID)
		if cErr != nil {
			return StudentEvent{}, cErr
		}
		if len(cRows.Values) == 0 {
			return StudentEvent{}, fmt.Errorf("目标班级不存在或不属于当前年级")
		}
		toClassName := stringValue(cRows.Values[0][1])
		fromJSON = mustJSON(map[string]any{"class_id": st.ClassID, "class_name": st.ClassName})
		toJSON = mustJSON(map[string]any{"class_id": input.ToClassID, "class_name": toClassName})
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE students SET current_class_id = ?, updated_at = ? WHERE id = ?`,
			Args: []any{input.ToClassID, nowStr, studentID},
		})
	case "track_changed":
		toTrack := normalizeTrackValue(input.ToTrack)
		toElective := strings.TrimSpace(input.ToElective)
		toLang := strings.TrimSpace(input.ToForeignLang)
		if toLang == "" {
			toLang = st.ForeignLang
		}
		if toTrack == "" && toElective == "" && toLang == st.ForeignLang {
			return StudentEvent{}, fmt.Errorf("请选择变更后的选科方向或外语语种")
		}
		if toTrack == "" {
			toTrack = st.Track
		}
		fromJSON = mustJSON(map[string]any{"track": st.Track, "elective": st.Elective, "foreign_lang": st.ForeignLang})
		toJSON = mustJSON(map[string]any{"track": toTrack, "elective": toElective, "foreign_lang": toLang})
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE students SET current_track = ?, elective_combo = ?, foreign_lang = ?, updated_at = ? WHERE id = ?`,
			Args: []any{toTrack, toElective, toLang, nowStr, studentID},
		})
	case "status_changed":
		toStatus := strings.TrimSpace(input.ToStatus)
		switch toStatus {
		case "active", "suspended", "transferred", "transferred_out", "borrowed", "left_school", "graduated":
		default:
			return StudentEvent{}, fmt.Errorf("未知学生状态 %q", toStatus)
		}
		fromJSON = mustJSON(map[string]any{"status": st.Status})
		toJSON = mustJSON(map[string]any{"status": toStatus})
		statements = append(statements, sqlite.Statement{
			SQL:  `UPDATE students SET status = ?, updated_at = ? WHERE id = ?`,
			Args: []any{toStatus, nowStr, studentID},
		})
	case "enrolled", "promoted", "merged", "graduated":
		fromJSON = "{}"
		toJSON = "{}"
	default:
		return StudentEvent{}, fmt.Errorf("未知异动类型 %q", eventType)
	}

	statements = append(statements,
		sqlite.Statement{
			SQL: `INSERT INTO student_events(
				student_id, cohort_id, phase, event_type, from_value_json, to_value_json, note, occurred_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{studentID, st.CohortID, phase, eventType, fromJSON, toJSON, note, occurredStr},
		},
		bumpRevisionStatement(fmt.Sprintf("cohort:%d", st.CohortID), nowStr),
	)
	if _, err := s.db.Transaction(ctx, statements); err != nil {
		return StudentEvent{}, err
	}
	evRows, err := s.db.Query(ctx, `SELECT id, student_id, cohort_id, phase, event_type,
		from_value_json, to_value_json, exam_id, note, occurred_at
		FROM student_events WHERE student_id = ? ORDER BY id DESC LIMIT 1`, studentID)
	if err != nil {
		return StudentEvent{}, err
	}
	if len(evRows.Values) == 0 {
		return StudentEvent{}, ErrNotFound
	}
	r := evRows.Values[0]
	tVal, err := parseTime(r[9])
	if err != nil {
		tVal = occurredAt
	}
	return StudentEvent{
		ID:            intValue(r[0]),
		StudentID:     intValue(r[1]),
		CohortID:      intValue(r[2]),
		Phase:         stringValue(r[3]),
		EventType:     stringValue(r[4]),
		FromValueJSON: stringValue(r[5]),
		ToValueJSON:   stringValue(r[6]),
		Note:          stringValue(r[8]),
		OccurredAt:    tVal,
	}, nil
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
