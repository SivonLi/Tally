package store_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"tally/internal/calc"
	"tally/internal/store"
)

func TestSchemaVersion1AndLegacyRejection(t *testing.T) {
	if store.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", store.SchemaVersion)
	}
	s := openL1Store(t)
	ctx := context.Background()
	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatalf("CreateCohort: %v", err)
	}
	if cohort.CurrentPhase != store.PhaseG1 || cohort.GraduationYear != 2029 {
		t.Fatalf("cohort = %+v", cohort)
	}
}

func TestPromoteCohortDirectAndReclassifyAndClassChain(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	plan, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	c2026, stG1 := seedCohortBaseData(t, s, 2026, "p1")
	g1Classes, err := s.ListClasses(ctx, c2026.ID, stG1.ID)
	if err != nil || len(g1Classes) != 2 {
		t.Fatalf("g1 classes = %+v, err=%v", g1Classes, err)
	}
	// 设置班主任
	t1, err := s.CreateTeacher(ctx, store.ImportTeacher{TeacherNo: "HT01", Name: "赵班主任", Subject: "chinese"})
	if err != nil {
		t.Fatal(err)
	}
	if warn, err := s.SetClassHeadTeacher(ctx, g1Classes[0].ID, t1.ID); err != nil || warn != nil {
		t.Fatalf("SetClassHeadTeacher err=%v warn=%+v", err, warn)
	}

	// 高一考试并核算
	exam1, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "高一期中", ExamDate: "2026-11-10", CohortIDs: []int64{c2026.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ec1, err := s.GetExamCohort(ctx, exam1.ID, c2026.ID)
	if err != nil {
		t.Fatal(err)
	}
	a1, err := s.AssessmentForExamCohort(ctx, ec1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAssessmentRun(ctx, a1.ID, plan.ID, "h1", nil, calc.Result{
		Rows: []calc.Row{{
			StatGroupID: "1", StatGroupName: "1班", Subject: "chinese",
			LeadClassID: strconv.FormatInt(g1Classes[0].ID, 10), LeadClassName: "1班",
			ClassAverage: 92, GradeAverage: 88, Total: 88, Rank: 1, TierIndex: 0, Points: 70,
		}},
	}, true); err != nil {
		t.Fatal(err)
	}

	// 模式 A：原班直升 g1 -> g2
	resA, err := s.PromoteCohort(ctx, c2026.ID, store.PromoteInput{
		ToPhase:             store.PhaseG2,
		ToAcademicYearStart: 2027,
		Mode:                store.PromoteDirect,
	})
	if err != nil {
		t.Fatalf("PromoteCohort direct: %v", err)
	}
	if resA.FromPhase != store.PhaseG1 || resA.ToPhase != store.PhaseG2 || resA.ClassesCreated != 2 || resA.HeadTeachersCarried != 1 {
		t.Fatalf("resA = %+v", resA)
	}
	g2StageID := store.EncodeStageID(c2026.ID, store.PhaseG2)
	g2Classes, err := s.ListClasses(ctx, c2026.ID, g2StageID)
	if err != nil || len(g2Classes) != 2 {
		t.Fatalf("g2 classes = %+v, err=%v", g2Classes, err)
	}
	if g2Classes[0].ContinuedFromClassID == nil || *g2Classes[0].ContinuedFromClassID != g1Classes[0].ID {
		t.Fatalf("expected continued_from_class_id=%d, got %+v", g1Classes[0].ID, g2Classes[0].ContinuedFromClassID)
	}
	// 旧阶段班级归档
	g1ClassesAfter, err := s.ListClasses(ctx, c2026.ID, stG1.ID)
	if err != nil || g1ClassesAfter[0].Status != "archived" {
		t.Fatalf("expected g1 class archived, got %+v err=%v", g1ClassesAfter, err)
	}

	// 高二考试并核算
	exam2, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2027, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "高二期中", ExamDate: "2027-11-10", CohortIDs: []int64{c2026.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ec2, err := s.GetExamCohort(ctx, exam2.ID, c2026.ID)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.AssessmentForExamCohort(ctx, ec2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAssessmentRun(ctx, a2.ID, plan.ID, "h2", nil, calc.Result{
		Rows: []calc.Row{{
			StatGroupID: "2", StatGroupName: "1班", Subject: "chinese",
			LeadClassID: strconv.FormatInt(g2Classes[0].ID, 10), LeadClassName: "1班",
			ClassAverage: 96, GradeAverage: 90, Total: 91, Rank: 1, TierIndex: 0, Points: 70,
		}},
	}, true); err != nil {
		t.Fatal(err)
	}

	// 验证 QueryClassTrendChain 沿 continued_from_class_id 串联 g1 与 g2 两场考试
	chain, trend, err := s.QueryClassTrendChain(ctx, g2Classes[0].ID)
	if err != nil {
		t.Fatalf("QueryClassTrendChain: %v", err)
	}
	if len(chain) != 2 || chain[0].Phase != store.PhaseG1 || chain[1].Phase != store.PhaseG2 {
		t.Fatalf("chain = %+v", chain)
	}
	if len(trend.Points) != 2 {
		t.Fatalf("expected 2 trend points along continued chain, got %d", len(trend.Points))
	}

	// 模式 B：选科重分班 g2 -> g3
	resB, err := s.PromoteCohort(ctx, c2026.ID, store.PromoteInput{
		ToPhase:             store.PhaseG3,
		ToAcademicYearStart: 2028,
		Mode:                store.PromoteReclassify,
	})
	if err != nil {
		t.Fatalf("PromoteCohort reclassify: %v", err)
	}
	if resB.ClassesCreated != 0 {
		t.Fatalf("expected 0 classes created on reclassify, got %+v", resB)
	}
	// 新建 g3 同名"1班"（无 continued_from_class_id），验证趋势链不误连 g1/g2
	g3Class1, err := s.CreateClass(ctx, c2026.ID, store.PhaseG3, "1班", "physics", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	chainG3, trendG3, err := s.QueryClassTrendChain(ctx, g3Class1.ID)
	if err != nil {
		t.Fatalf("QueryClassTrendChain g3: %v", err)
	}
	if len(chainG3) != 1 || chainG3[0].ClassID != g3Class1.ID {
		t.Fatalf("expected isolated g3 chain of length 1, got %+v", chainG3)
	}
	if len(trendG3.Points) != 0 {
		t.Fatalf("expected 0 trend points across reclassify boundary, got %d", len(trendG3.Points))
	}
}

func TestTeacherOccupancyCrossGradeAndGraduateRelease(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	c2024, _ := seedCohortBaseData(t, s, 2024, "g24")
	c2025, _ := seedCohortBaseData(t, s, 2025, "g25")

	// 全校新建教师：停用、空闲、单届在任、跨届兼任
	inactiveT, err := s.CreateTeacher(ctx, store.ImportTeacher{TeacherNo: "T100", Name: "休假教师", Subject: "physics"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetTeacherStatus(ctx, inactiveT.ID, "inactive"); err != nil {
		t.Fatal(err)
	}
	idleT, err := s.CreateTeacher(ctx, store.ImportTeacher{TeacherNo: "T101", Name: "待命教师", Subject: "math"})
	if err != nil {
		t.Fatal(err)
	}
	crossT, err := s.CreateTeacher(ctx, store.ImportTeacher{TeacherNo: "T102", Name: "跨级教师", Subject: "geography"})
	if err != nil {
		t.Fatal(err)
	}

	g24Groups, err := s.ListTeachingGroups(ctx, store.EncodeStageID(c2024.ID, c2024.CurrentPhase), "")
	if err != nil || len(g24Groups) == 0 {
		t.Fatalf("g24 groups = %+v err=%v", g24Groups, err)
	}
	g25Groups, err := s.ListTeachingGroups(ctx, store.EncodeStageID(c2025.ID, c2025.CurrentPhase), "")
	if err != nil || len(g25Groups) == 0 {
		t.Fatalf("g25 groups = %+v err=%v", g25Groups, err)
	}

	// 仅在 2024 级任教
	if warn, err := s.AssignTeacherToGroup(ctx, idleT.ID, g24Groups[0].ID, "primary"); err != nil || warn != nil {
		t.Fatalf("assign single cohort err=%v warn=%+v", err, warn)
	}
	// 先在 2024 级任教，再到 2025 级任教应返回非阻断跨级提醒
	if warn, err := s.AssignTeacherToGroup(ctx, crossT.ID, g24Groups[0].ID, "primary"); err != nil || warn != nil {
		t.Fatalf("assign cross cohort 1 err=%v warn=%+v", err, warn)
	}
	warn, err := s.AssignTeacherToGroup(ctx, crossT.ID, g25Groups[0].ID, "primary")
	if err != nil {
		t.Fatalf("assign cross cohort 2 err=%v", err)
	}
	if warn == nil || len(warn.ExistingTargets) == 0 {
		t.Fatalf("expected non-blocking CrossGradeWarning, got nil")
	}

	// 检查全校占用聚合状态
	allOcc, err := s.ListTeachersWithOccupancy(ctx, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	occByID := map[int64]store.TeacherOccupancy{}
	for _, o := range allOcc {
		occByID[o.TeacherID] = o
	}
	if occByID[inactiveT.ID].Status != store.TeacherStatusInactive {
		t.Fatalf("inactive teacher status = %q", occByID[inactiveT.ID].Status)
	}
	if occByID[idleT.ID].Status != store.TeacherStatusActiveSingle {
		t.Fatalf("single teacher status = %q", occByID[idleT.ID].Status)
	}
	if occByID[crossT.ID].Status != store.TeacherStatusActiveMulti || len(occByID[crossT.ID].ActiveCohorts) != 2 {
		t.Fatalf("cross teacher occ = %+v", occByID[crossT.ID])
	}

	// 毕业预检（seedCohortBaseData 在 2024/2025 两届共用张老师、李老师，加上 crossT 共 3 位跨级在任，idleT 1 位释放空闲）
	preview, err := s.PreviewGraduateCohort(ctx, c2024.ID)
	if err != nil {
		t.Fatalf("PreviewGraduateCohort: %v", err)
	}
	if preview.ActiveStudents != 2 || preview.TotalTeachersAffected != 4 || preview.StillActiveCrossCohortCount != 3 || preview.ReleasedToIdleCount != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.ReleasedTeachers) != 1 || preview.ReleasedTeachers[0].TeacherID != idleT.ID {
		t.Fatalf("expected ReleasedTeachers=[idleT(%d)], got %+v", idleT.ID, preview.ReleasedTeachers)
	}
	foundCross := false
	for _, ct := range preview.CrossCohortTeachers {
		if ct.TeacherID == crossT.ID {
			foundCross = true
			break
		}
	}
	if !foundCross {
		t.Fatalf("expected crossT(%d) in CrossCohortTeachers, got %+v", crossT.ID, preview.CrossCohortTeachers)
	}

	// 执行毕业封存
	gradRes, err := s.GraduateCohort(ctx, c2024.ID)
	if err != nil {
		t.Fatalf("GraduateCohort: %v", err)
	}
	if gradRes.GraduatedStudents != 2 || gradRes.EndedAssignments == 0 {
		t.Fatalf("gradRes = %+v", gradRes)
	}
	// 毕业后禁止重复毕业或继续导入
	if _, err := s.GraduateCohort(ctx, c2024.ID); !errors.Is(err, store.ErrCohortGraduated) {
		t.Fatalf("expected ErrCohortGraduated, got %v", err)
	}

	// 验证仅教 2024 级的 idleT 恢复空闲且带 LastRelease；跨级兼任 crossT 仍保持在任
	afterOcc, err := s.ListTeachersWithOccupancy(ctx, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	afterByID := map[int64]store.TeacherOccupancy{}
	for _, o := range afterOcc {
		afterByID[o.TeacherID] = o
	}
	if afterByID[idleT.ID].Status != store.TeacherStatusIdle || afterByID[idleT.ID].LastRelease == nil || afterByID[idleT.ID].LastRelease.EndReason != "graduated" {
		t.Fatalf("expected idleT released to idle with graduated LastRelease, got %+v", afterByID[idleT.ID])
	}
	if afterByID[crossT.ID].Status != store.TeacherStatusActiveSingle || len(afterByID[crossT.ID].ActiveCohorts) != 1 || afterByID[crossT.ID].ActiveCohorts[0] != c2025.ID {
		t.Fatalf("expected crossT still active in c2025, got %+v", afterByID[crossT.ID])
	}

	// 教师个人档案与跨届履历
	plan, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	exam25, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "2025级期中", ExamDate: "2026-11-15", CohortIDs: []int64{c2025.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ec25, _ := s.GetExamCohort(ctx, exam25.ID, c2025.ID)
	a25, _ := s.AssessmentForExamCohort(ctx, ec25.ID)
	if _, err := s.SaveAssessmentRun(ctx, a25.ID, plan.ID, "h25", nil, calc.Result{
		Rows: []calc.Row{{
			StatGroupID: "1", StatGroupName: "1班", Subject: "chinese",
			LeadClassName: "1班", TeacherIDs: []string{strconv.FormatInt(crossT.ID, 10)},
			Total: 90, Rank: 1, Points: 70,
		}},
		Personal: []calc.PersonalScore{{
			TeacherID: strconv.FormatInt(crossT.ID, 10), TeacherName: crossT.Name,
			Grade: "g2", Subject: "chinese", GroupPoints: []float64{70}, Score: 70,
		}},
	}, true); err != nil {
		t.Fatal(err)
	}
	tProfile, err := s.GetTeacherProfile(ctx, crossT.ID)
	if err != nil {
		t.Fatalf("GetTeacherProfile: %v", err)
	}
	if len(tProfile.CrossCohortScores) != 1 || len(tProfile.CohortHistories) != 2 {
		t.Fatalf("teacher profile = %+v", tProfile)
	}
	foundExamClassRow := false
	for _, ch := range tProfile.CohortHistories {
		if ch.CohortID == c2025.ID {
			if len(ch.Exams) != 1 || len(ch.Exams[0].ClassRows) != 1 || ch.Exams[0].ClassRows[0].StatGroupName != "1班" {
				t.Fatalf("expected populated ClassRows on TeacherExamHistory, got %+v", ch.Exams)
			}
			foundExamClassRow = true
		}
	}
	if !foundExamClassRow {
		t.Fatalf("missing c2025 exam history in %+v", tProfile.CohortHistories)
	}
}

func TestStudentProfilePreviousLinkMergeAndWalkingSnapshot(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)
	plan, err := s.EnsureDefaultPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 2025级往届学生（用于复读前溯）
	c2025, st2025 := seedCohortBaseData(t, s, 2025, "old")
	oldStudents, _, err := s.ListStudents(ctx, store.StudentFilter{CohortID: c2025.ID, StageID: st2025.ID})
	if err != nil || len(oldStudents) == 0 {
		t.Fatalf("old students = %+v err=%v", oldStudents, err)
	}

	// 2026级复读建档 + 重复建档待合并 + 中考单科入口成绩
	c2026, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	st2026, err := s.GetCohortStage(ctx, c2026.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	ent := 555.0
	if _, err := s.ApplyStageImport(ctx, c2026.ID, st2026.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Students: []store.ImportStudent{
				{
					StudentNo: "2026001", CandidateCode: "CAND001", Name: "王复读", ClassName: "1班",
					Track: "physics", Status: "active", TotalScore: &ent,
					Scores: map[string]float64{"chinese": 108, "math": 122}, SourceRow: 2,
				},
				{
					StudentNo: "", CandidateCode: "CAND002", Name: "王复读", ClassName: "1班",
					Track: "physics", Status: "active", SourceRow: 3,
				},
			},
		},
		Report: store.ImportReport{
			FileHash: "stu-2026", Path: "stu2026.xlsx", RowCount: 2,
			Sheets: []store.ImportSheet{{Name: "学生", Role: "students", HeaderRow: 1, Rows: 2}},
		},
	}); err != nil {
		t.Fatalf("import 2026 students: %v", err)
	}

	// 多键匹配验证：student_no -> candidate_code -> 重名同班消歧报错
	idByNo, via1, err := s.MatchStudent(ctx, c2026.ID, store.PhaseG1, "2026001", "", "", "")
	if err != nil || via1 != "student_no" {
		t.Fatalf("MatchStudent student_no: id=%d via=%q err=%v", idByNo, via1, err)
	}
	idByCand, via2, err := s.MatchStudent(ctx, c2026.ID, store.PhaseG1, "", "CAND002", "", "")
	if err != nil || via2 != "candidate_code" {
		t.Fatalf("MatchStudent candidate_code: id=%d via=%q err=%v", idByCand, via2, err)
	}
	if _, _, err := s.MatchStudent(ctx, c2026.ID, store.PhaseG1, "", "", "王复读", "1班"); !errors.Is(err, store.ErrAmbiguousStudentMatch) {
		t.Fatalf("expected ErrAmbiguousStudentMatch, got %v", err)
	}

	// 合并重复学生档案并关联往届复读前档
	if err := s.MergeStudents(ctx, idByNo, idByCand); err != nil {
		t.Fatalf("MergeStudents: %v", err)
	}
	if err := s.SetStudentPreviousLink(ctx, idByNo, oldStudents[0].ID); err != nil {
		t.Fatalf("SetStudentPreviousLink: %v", err)
	}

	// 走班仅存 teaching_group_students，创建常规和走班教学班验证约束与快照固化
	classes26, err := s.ListClasses(ctx, c2026.ID, st2026.ID)
	if err != nil || len(classes26) == 0 {
		t.Fatal(err)
	}
	if _, err := s.ApplyStageImport(ctx, c2026.ID, st2026.ID, store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Assignments: []store.ImportAssignment{
				{ClassName: "1班", Subject: "chinese", TeacherName: "刘老师", SourceRow: 2},
			},
		},
		Report: store.ImportReport{FileHash: "assign-26", Path: "assign26.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatal(err)
	}
	// 创建走班教学班并绑定学生
	walkGroup, err := s.CreateTeachingGroup(ctx, c2026.ID, store.PhaseG1, "geography", "地理走班A", "walking", 0)
	if err != nil {
		t.Fatalf("CreateTeachingGroup walking: %v", err)
	}
	if err := s.AssignWalkingStudent(ctx, walkGroup.ID, idByNo); err != nil {
		t.Fatalf("AssignWalkingStudent: %v", err)
	}
	walkRes := walkGroup.ID
	if walkRes <= 0 {
		t.Fatalf("invalid walking group id %d", walkRes)
	}
	// 确认 admin 教学班不能写入 teaching_group_students
	adminGroups, err := s.ListTeachingGroups(ctx, st2026.ID, "chinese")
	if err != nil || len(adminGroups) == 0 {
		t.Fatal(err)
	}
	if err := s.AssignWalkingStudent(ctx, adminGroups[0].ID, idByNo); err == nil {
		t.Fatalf("expected error assigning student to admin group")
	}

	// 创建考试并导入成绩与分数线/目标，验证 exam_scores.teaching_group_id_snapshot 与 exam_participants 排名快照
	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "走班期中", ExamDate: "2026-11-20", CohortIDs: []int64{c2026.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GenerateExamRoster(ctx, exam.ID, c2026.ID, "physics"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: c2026.ID, Kind: store.ExamImportScores,
		Data: store.ImportData{
			Students: []store.ImportStudent{{
				StudentNo: "2026001", Name: "王复读",
				Scores:    map[string]float64{"chinese": 115, "geography": 89},
				SourceRow: 2,
			}},
		},
		Report: store.ImportReport{FileHash: "walk-scores", Path: "walk_scores.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatalf("ApplyExamImport scores: %v", err)
	}

	roster, _, err := s.ListExamRoster(ctx, store.ExamRosterFilter{ExamID: exam.ID, CohortID: c2026.ID})
	if err != nil || len(roster) != 1 {
		t.Fatalf("roster = %+v err=%v", roster, err)
	}
	if roster[0].TotalScore == nil || *roster[0].TotalScore != 204 || roster[0].GradeRank == nil || *roster[0].GradeRank != 1 {
		t.Fatalf("expected precomputed total_score=204 & grade_rank=1, got %+v", roster[0])
	}
	scores, _, err := s.ListExamScores(ctx, store.ExamScoreFilter{ExamID: exam.ID, CohortID: c2026.ID})
	if err != nil || len(scores) != 2 {
		t.Fatalf("scores = %+v err=%v", scores, err)
	}
	snapBySubj := map[string]int64{}
	for _, sc := range scores {
		if sc.TeachingGroupSnapshot != nil {
			snapBySubj[sc.Subject] = *sc.TeachingGroupSnapshot
		}
	}
	if snapBySubj["geography"] != walkRes || snapBySubj["chinese"] != adminGroups[0].ID {
		t.Fatalf("expected teaching_group_id_snapshot geo=%d chn=%d, got %+v", walkRes, adminGroups[0].ID, snapBySubj)
	}

	// 验证完整学生三年档案 GetStudentProfile
	ec, _ := s.GetExamCohort(ctx, exam.ID, c2026.ID)
	assess, _ := s.AssessmentForExamCohort(ctx, ec.ID)
	if _, err := s.SaveAssessmentRun(ctx, assess.ID, plan.ID, "h-walk", nil, calc.Result{
		Rows: []calc.Row{{
			StatGroupID: "1", StatGroupName: "1班", Subject: "chinese",
			LeadClassID: strconv.FormatInt(classes26[0].ID, 10), LeadClassName: "1班",
			Total: 90, Rank: 1, Points: 70,
			Participants: []calc.Participant{{StudentNo: "2026001", Name: "王复读"}},
		}},
	}, true); err != nil {
		t.Fatal(err)
	}
	profile, err := s.GetStudentProfile(ctx, idByNo)
	if err != nil {
		t.Fatalf("GetStudentProfile: %v", err)
	}
	if profile.PreviousStudent == nil || profile.PreviousStudent.ID != oldStudents[0].ID {
		t.Fatalf("expected previous_student link, got %+v", profile.PreviousStudent)
	}
	if len(profile.EntryScores) != 2 || len(profile.Events) < 3 || len(profile.Exams) != 1 {
		t.Fatalf("student profile counts: entry=%d events=%d exams=%d", len(profile.EntryScores), len(profile.Events), len(profile.Exams))
	}
}

func TestPromoteDirectWithPartialExistingTargetClasses(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	c2026, stG1 := seedCohortBaseData(t, s, 2026, "part")
	// 再往 2班 补一名在读学生，确保两个班均有在读生。
	if _, err := s.ApplyStageImport(ctx, c2026.ID, stG1.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Students: []store.ImportStudent{
				{StudentNo: "part04", Name: "丁", ClassName: "2班", Track: "physics", Status: "active", SourceRow: 5},
			},
		},
		Report: store.ImportReport{FileHash: "part-extra-stu", Path: "extra.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatal(err)
	}

	g1Classes, err := s.ListClasses(ctx, c2026.ID, stG1.ID)
	if err != nil || len(g1Classes) != 2 {
		t.Fatalf("g1Classes = %+v err=%v", g1Classes, err)
	}
	g1ByName := map[string]int64{}
	for _, c := range g1Classes {
		g1ByName[c.Name] = c.ID
	}

	// 设置班主任。
	ht1, err := s.CreateTeacher(ctx, store.ImportTeacher{TeacherNo: "PHT1", Name: "一班班主任", Subject: "chinese"})
	if err != nil {
		t.Fatal(err)
	}
	ht2, err := s.CreateTeacher(ctx, store.ImportTeacher{TeacherNo: "PHT2", Name: "二班班主任", Subject: "math"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClassHeadTeacher(ctx, g1ByName["1班"], ht1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClassHeadTeacher(ctx, g1ByName["2班"], ht2.ID); err != nil {
		t.Fatal(err)
	}

	// 预先在高二创建 1班（continued_from_class_id 为空），2班尚未创建。
	preG2Class1, err := s.CreateClass(ctx, c2026.ID, store.PhaseG2, "1班", "physics", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.PromoteCohort(ctx, c2026.ID, store.PromoteInput{
		ToPhase:             store.PhaseG2,
		ToAcademicYearStart: 2027,
		Mode:                store.PromoteDirect,
	})
	if err != nil {
		t.Fatalf("PromoteCohort direct with partial target classes: %v", err)
	}
	if res.ClassesCreated != 2 || res.HeadTeachersCarried != 2 {
		t.Fatalf("res = %+v", res)
	}

	g2StageID := store.EncodeStageID(c2026.ID, store.PhaseG2)
	g2Classes, err := s.ListClasses(ctx, c2026.ID, g2StageID)
	if err != nil || len(g2Classes) != 2 {
		t.Fatalf("g2Classes = %+v err=%v", g2Classes, err)
	}
	g2ByName := map[string]store.SchoolClass{}
	for _, c := range g2Classes {
		g2ByName[c.Name] = c
	}
	if g2ByName["1班"].ID != preG2Class1.ID || g2ByName["1班"].ContinuedFromClassID == nil || *g2ByName["1班"].ContinuedFromClassID != g1ByName["1班"] {
		t.Fatalf("g2 1班 = %+v, want ID=%d ContinuedFrom=%d", g2ByName["1班"], preG2Class1.ID, g1ByName["1班"])
	}
	if g2ByName["2班"].ContinuedFromClassID == nil || *g2ByName["2班"].ContinuedFromClassID != g1ByName["2班"] {
		t.Fatalf("g2 2班 = %+v, want ContinuedFrom=%d", g2ByName["2班"], g1ByName["2班"])
	}
	if g2ByName["1班"].HeadTeacherID != ht1.ID || g2ByName["2班"].HeadTeacherID != ht2.ID {
		t.Fatalf("head teachers not carried: 1班=%d 2班=%d", g2ByName["1班"].HeadTeacherID, g2ByName["2班"].HeadTeacherID)
	}

	// 验证所有在读学生均迁移至高二活跃班，无一滞留在已归档的高一旧班。
	activeStudents, _, err := s.ListStudents(ctx, store.StudentFilter{CohortID: c2026.ID, Status: "active"})
	if err != nil || len(activeStudents) != 3 {
		t.Fatalf("activeStudents = %+v err=%v", activeStudents, err)
	}
	g2ActiveIDs := map[int64]bool{g2ByName["1班"].ID: true, g2ByName["2班"].ID: true}
	for _, st := range activeStudents {
		if !g2ActiveIDs[st.ClassID] {
			t.Fatalf("student %s (%s) stranded in old class %d", st.StudentNo, st.Name, st.ClassID)
		}
	}

	// 验证教学班与活跃任课关系均已延续。
	g2Groups, err := s.ListTeachingGroups(ctx, g2StageID, "")
	if err != nil || len(g2Groups) != 2 {
		t.Fatalf("g2Groups = %+v err=%v", g2Groups, err)
	}
	for _, g := range g2Groups {
		if g.Teachers == "" {
			t.Fatalf("teaching group %s missing carried teacher", g.Name)
		}
	}
}

func TestCohortBaselineOverridesAcrossPromotion(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	c2026, _ := seedCohortBaseData(t, s, 2026, "base")
	if _, err := s.PromoteCohort(ctx, c2026.ID, store.PromoteInput{
		ToPhase: store.PhaseG2, ToAcademicYearStart: 2027, Mode: store.PromoteDirect,
	}); err != nil {
		t.Fatal(err)
	}
	g2StageID := store.EncodeStageID(c2026.ID, store.PhaseG2)
	g2Classes, err := s.ListClasses(ctx, c2026.ID, g2StageID)
	if err != nil || len(g2Classes) != 2 {
		t.Fatalf("g2Classes = %+v err=%v", g2Classes, err)
	}
	g2ByName := map[string]int64{}
	for _, c := range g2Classes {
		g2ByName[c.Name] = c.ID
	}

	exam2, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2027, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "高二期中基数测试", ExamDate: "2027-11-10", CohortIDs: []int64{c2026.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ec2, err := s.GetExamCohort(ctx, exam2.ID, c2026.ID)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.AssessmentForExamCohort(ctx, ec2.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.LoadAssessmentData(ctx, a2.ID)
	if err != nil {
		t.Fatalf("LoadAssessmentData: %v", err)
	}
	c1Key := strconv.FormatInt(g2ByName["1班"], 10)
	c2Key := strconv.FormatInt(g2ByName["2班"], 10)
	if data.ClassBaselines["chinese"][c1Key] != 95 || data.ClassBaselines["chinese"][c2Key] != 85 {
		t.Fatalf("expected g2 ClassBaselines to resolve via continued_from_class_id (%s=95, %s=85), got %+v",
			c1Key, c2Key, data.ClassBaselines["chinese"])
	}
	if data.ClassBaselineRatios["chinese"][c1Key] != 1.05 || data.ClassBaselineRatios["chinese"][c2Key] != 0.95 {
		t.Fatalf("expected g2 ClassBaselineRatios to resolve, got %+v", data.ClassBaselineRatios["chinese"])
	}

	// 验证 ListBaselines 在高二阶段也能按继承链命中新班 ID。
	bRows, err := s.ListBaselines(ctx, g2StageID, g2ByName["1班"], "")
	if err != nil || len(bRows) != 1 || bRows[0].ClassID != g2ByName["1班"] || bRows[0].Average != 95 {
		t.Fatalf("ListBaselines g2 = %+v err=%v", bRows, err)
	}

	// 选科重分班进入高三后，新建同名班不应误继承旧班覆盖基数。
	if _, err := s.PromoteCohort(ctx, c2026.ID, store.PromoteInput{
		ToPhase: store.PhaseG3, ToAcademicYearStart: 2028, Mode: store.PromoteReclassify,
	}); err != nil {
		t.Fatal(err)
	}
	g3Class1, err := s.CreateClass(ctx, c2026.ID, store.PhaseG3, "1班", "physics", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	exam3, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2028, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "高三期中重分班测试", ExamDate: "2028-11-10", CohortIDs: []int64{c2026.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ec3, _ := s.GetExamCohort(ctx, exam3.ID, c2026.ID)
	a3, _ := s.AssessmentForExamCohort(ctx, ec3.ID)
	data3, err := s.LoadAssessmentData(ctx, a3.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := data3.ClassBaselines["chinese"][strconv.FormatInt(g3Class1.ID, 10)]; leaked {
		t.Fatalf("reclassified g3 class should not inherit old g1/g2 override: %+v", data3.ClassBaselines["chinese"])
	}
}

func TestGetTeacherProfileHeadTeacherOnlyAndGraduation(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	c2024, stG3 := seedCohortBaseData(t, s, 2024, "ht")
	classes, err := s.ListClasses(ctx, c2024.ID, stG3.ID)
	if err != nil || len(classes) == 0 {
		t.Fatalf("classes = %+v err=%v", classes, err)
	}
	htOnly, err := s.CreateTeacher(ctx, store.ImportTeacher{TeacherNo: "HTONLY1", Name: "纯班主任", Subject: "history"})
	if err != nil {
		t.Fatal(err)
	}
	if warn, err := s.SetClassHeadTeacher(ctx, classes[0].ID, htOnly.ID); err != nil || warn != nil {
		t.Fatalf("SetClassHeadTeacher err=%v warn=%+v", err, warn)
	}

	profile, err := s.GetTeacherProfile(ctx, htOnly.ID)
	if err != nil {
		t.Fatalf("GetTeacherProfile: %v", err)
	}
	if profile.Occupancy.Status != store.TeacherStatusActiveSingle {
		t.Fatalf("expected active_single occupancy, got %q", profile.Occupancy.Status)
	}
	if len(profile.CohortHistories) != 1 || len(profile.CohortHistories[0].Assignments) != 1 {
		t.Fatalf("expected 1 cohort history with 1 head_teacher assignment, got %+v", profile.CohortHistories)
	}
	assign := profile.CohortHistories[0].Assignments[0]
	if assign.Role != "head_teacher" || assign.ClassID != classes[0].ID || assign.ClassName != classes[0].Name {
		t.Fatalf("unexpected head teacher assignment: %+v", assign)
	}

	if _, err := s.GraduateCohort(ctx, c2024.ID); err != nil {
		t.Fatalf("GraduateCohort: %v", err)
	}
	after, err := s.GetTeacherProfile(ctx, htOnly.ID)
	if err != nil {
		t.Fatalf("GetTeacherProfile after graduate: %v", err)
	}
	if after.Occupancy.Status != store.TeacherStatusIdle || after.Occupancy.LastRelease == nil || after.Occupancy.LastRelease.EndReason != "graduated" {
		t.Fatalf("expected idle after graduation with graduated release, got %+v", after.Occupancy)
	}
	if len(after.CohortHistories) != 1 || after.CohortHistories[0].CurrentPhase != store.PhaseGraduated ||
		len(after.CohortHistories[0].Assignments) != 1 || after.CohortHistories[0].Assignments[0].Role != "head_teacher" {
		t.Fatalf("expected graduated history with head_teacher record, got %+v", after.CohortHistories)
	}
}

func TestDeleteClassChecksAllForeignKeys(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	cohort, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := s.CreateClass(ctx, cohort.ID, store.PhaseG1, "1班", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// 1. continued_from_class_id 引用阻断
	c2, err := s.CreateClass(ctx, cohort.ID, store.PhaseG2, "1班", "", "", &c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClass(ctx, c1.ID); !errors.Is(err, store.ErrClassNotEmpty) {
		t.Fatalf("expected ErrClassNotEmpty for continued_from_class_id, got %v", err)
	}
	if err := s.DeleteClass(ctx, c2.ID); err != nil {
		t.Fatalf("DeleteClass c2: %v", err)
	}
	// 2. teaching_groups.admin_class_id 引用阻断
	if _, err := s.CreateTeachingGroup(ctx, cohort.ID, store.PhaseG1, "chinese", "1班", "admin", c1.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClass(ctx, c1.ID); !errors.Is(err, store.ErrClassNotEmpty) {
		t.Fatalf("expected ErrClassNotEmpty for teaching_groups.admin_class_id, got %v", err)
	}
	// 3. assessment_stat_groups.lead_class_id 引用阻断
	c3, err := s.CreateClass(ctx, cohort.ID, store.PhaseG1, "3班", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "FK测试", ExamDate: "2026-11-10", CohortIDs: []int64{cohort.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ec, _ := s.GetExamCohort(ctx, exam.ID, cohort.ID)
	assess, _ := s.AssessmentForExamCohort(ctx, ec.ID)
	tgt := 5.0
	if _, err := s.SaveStatGroup(ctx, store.StatGroupInput{
		AssessmentID: assess.ID, Subject: "chinese", Name: "3班合并组",
		LeadClassID: c3.ID, Target: &tgt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClass(ctx, c3.ID); !errors.Is(err, store.ErrClassNotEmpty) {
		t.Fatalf("expected ErrClassNotEmpty for lead_class_id, got %v", err)
	}
}

func TestMergeStudentsInheritsPreviousStudentID(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	c2025, _ := seedCohortBaseData(t, s, 2025, "prev")
	prevList, _, err := s.ListStudents(ctx, store.StudentFilter{CohortID: c2025.ID})
	if err != nil || len(prevList) == 0 {
		t.Fatalf("prevList = %+v err=%v", prevList, err)
	}
	prevID := prevList[0].ID

	c2026, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	st2026, _ := s.GetCohortStage(ctx, c2026.ID, 2026)
	if _, err := s.ApplyStageImport(ctx, c2026.ID, st2026.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Students: []store.ImportStudent{
				{StudentNo: "2026901", CandidateCode: "MC01", Name: "合并复读生", ClassName: "1班", Status: "active", SourceRow: 2},
				{StudentNo: "", CandidateCode: "MC02", Name: "合并复读生", ClassName: "1班", Status: "active", PreviousStudentID: &prevID, SourceRow: 3},
			},
		},
		Report: store.ImportReport{FileHash: "merge-prev", Path: "merge_prev.xlsx", RowCount: 2},
	}); err != nil {
		t.Fatal(err)
	}
	targetID, _, err := s.MatchStudent(ctx, c2026.ID, store.PhaseG1, "2026901", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sourceID, _, err := s.MatchStudent(ctx, c2026.ID, store.PhaseG1, "", "MC02", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MergeStudents(ctx, targetID, sourceID); err != nil {
		t.Fatalf("MergeStudents: %v", err)
	}
	merged, err := s.GetStudent(ctx, 0, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.PreviousStudentID == nil || *merged.PreviousStudentID != prevID {
		t.Fatalf("expected inherited previous_student_id=%d, got %+v", prevID, merged.PreviousStudentID)
	}
}

func TestTeacherDisambiguationAndCandidateNoSnapshot(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	c2026, err := s.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	st2026, _ := s.GetCohortStage(ctx, c2026.ID, 2026)

	// 无工号同名不同学科教师：应消歧为 2 条独立教师记录并各自任课。
	if _, err := s.ApplyStageImport(ctx, c2026.ID, st2026.ID, store.StageImportRequest{
		Kind: "assignments",
		Data: store.ImportData{
			Assignments: []store.ImportAssignment{
				{ClassName: "1班", Subject: "chinese", TeacherName: "王同名", SourceRow: 2},
				{ClassName: "1班", Subject: "math", TeacherName: "王同名", SourceRow: 3},
				{ClassName: "2班", Subject: "chinese", TeacherName: "王同名", SourceRow: 4},
			},
		},
		Report: store.ImportReport{FileHash: "same-name-diff-subj", Path: "assign.xlsx", RowCount: 3},
	}); err != nil {
		t.Fatalf("ApplyStageImport assignments: %v", err)
	}
	teachers, err := s.ListTeachers(ctx, 0, "王同名")
	if err != nil || len(teachers) != 2 {
		t.Fatalf("expected 2 disambiguated teachers for 王同名, got %+v err=%v", teachers, err)
	}
	// 同名同科无工号重复插入应受部分唯一索引约束。
	if _, err := s.CreateTeacher(ctx, store.ImportTeacher{Name: "王同名", Subject: "chinese"}); err == nil {
		t.Fatalf("expected unique constraint error for duplicate (name, primary_subject) without teacher_no")
	}

	// 导入学生与考务报名快照，验证 exam_participants.candidate_no 优先作为快照返回。
	if _, err := s.ApplyStageImport(ctx, c2026.ID, st2026.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Students: []store.ImportStudent{
				{StudentNo: "S100", CandidateCode: "ARCH-CAND-1", Name: "快照生", ClassName: "1班", Track: "physics", Status: "active", SourceRow: 2},
			},
		},
		Report: store.ImportReport{FileHash: "snap-stu", Path: "stu.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatal(err)
	}
	exam, err := s.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "考生号快照测试", ExamDate: "2026-11-10", CohortIDs: []int64{c2026.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyExamImport(ctx, store.ExamImportRequest{
		ExamID: exam.ID, CohortID: c2026.ID, Kind: store.ExamImportRoster,
		Data: store.ImportData{
			Roster: []store.ImportRoster{
				{StudentNo: "S100", CandidateNo: "EXAM-CAND-999", Name: "快照生", ClassName: "1班", Track: "physics", SourceRow: 2},
			},
		},
		Report: store.ImportReport{FileHash: "snap-roster", Path: "roster.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatal(err)
	}
	// 更新学生主档的 candidate_code，考务名单仍应优先返回考试当时的 candidate_no 快照。
	if _, err := s.ApplyStageImport(ctx, c2026.ID, st2026.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Students: []store.ImportStudent{
				{StudentNo: "S100", CandidateCode: "ARCH-CAND-NEW", Name: "快照生", ClassName: "1班", Track: "physics", Status: "active", SourceRow: 2},
			},
		},
		Report: store.ImportReport{FileHash: "snap-stu-upd", Path: "stu2.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatal(err)
	}
	roster, _, err := s.ListExamRoster(ctx, store.ExamRosterFilter{ExamID: exam.ID, CohortID: c2026.ID, Query: "EXAM-CAND-999"})
	if err != nil || len(roster) != 1 || roster[0].CandidateNo != "EXAM-CAND-999" {
		t.Fatalf("expected snapshot candidate_no=EXAM-CAND-999, got %+v err=%v", roster, err)
	}
}

func TestAddStudentEventAndUpdateTeacher(t *testing.T) {
	ctx := context.Background()
	s := openL1Store(t)

	c2026, stG1 := seedCohortBaseData(t, s, 2026, "ev")
	classes, err := s.ListClasses(ctx, c2026.ID, stG1.ID)
	if err != nil || len(classes) < 2 {
		t.Fatalf("classes = %+v err=%v", classes, err)
	}
	students, _, err := s.ListStudents(ctx, store.StudentFilter{CohortID: c2026.ID})
	if err != nil || len(students) == 0 {
		t.Fatalf("students = %+v err=%v", students, err)
	}
	stu := students[0]

	var targetClassID int64
	for _, cls := range classes {
		if cls.ID != stu.ClassID {
			targetClassID = cls.ID
			break
		}
	}
	if targetClassID == 0 {
		t.Fatal("missing target class")
	}

	ev1, err := s.AddStudentEvent(ctx, stu.ID, store.AddStudentEventInput{
		EventType: "class_changed",
		ToClassID: targetClassID,
		Note:      "分班微调",
	})
	if err != nil || ev1.EventType != "class_changed" {
		t.Fatalf("AddStudentEvent class_changed: %+v err=%v", ev1, err)
	}
	ev2, err := s.AddStudentEvent(ctx, stu.ID, store.AddStudentEventInput{
		EventType:     "track_changed",
		ToTrack:       "history",
		ToElective:    "史政地",
		ToForeignLang: "japanese",
		Note:          "改选历史类",
	})
	if err != nil || ev2.EventType != "track_changed" {
		t.Fatalf("AddStudentEvent track_changed: %+v err=%v", ev2, err)
	}
	ev3, err := s.AddStudentEvent(ctx, stu.ID, store.AddStudentEventInput{
		EventType: "status_changed",
		ToStatus:  "borrowed",
		Note:      "外校借读一学期",
	})
	if err != nil || ev3.EventType != "status_changed" {
		t.Fatalf("AddStudentEvent status_changed: %+v err=%v", ev3, err)
	}

	prof, err := s.GetStudentProfile(ctx, stu.ID)
	if err != nil {
		t.Fatalf("GetStudentProfile: %v", err)
	}
	if prof.Student.ClassID != targetClassID || prof.Student.Track != "history" ||
		prof.Student.Elective != "史政地" || prof.Student.ForeignLang != "japanese" || prof.Student.Status != "borrowed" {
		t.Fatalf("student profile not updated by events: %+v", prof.Student)
	}
	if len(prof.Events) < 4 {
		t.Fatalf("expected >=4 events, got %d", len(prof.Events))
	}

	teacher, err := s.CreateTeacher(ctx, store.ImportTeacher{TeacherNo: "UP01", Name: "原名教师", Subject: "chinese"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateTeacher(ctx, teacher.ID, store.ImportTeacher{
		TeacherNo: "UP01-NEW",
		Name:      "新名教师",
		Subject:   "math",
		Note:      "教研组长",
	})
	if err != nil {
		t.Fatalf("UpdateTeacher: %v", err)
	}
	if updated.TeacherNo != "UP01-NEW" || updated.Name != "新名教师" || updated.Subject != "math" || updated.Note != "教研组长" {
		t.Fatalf("updated teacher mismatch: %+v", updated)
	}
}
