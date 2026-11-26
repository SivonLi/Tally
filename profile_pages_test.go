package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/calc"
	"tally/internal/store"
	"tally/locales"
)

func seedThreeYearStudentFixture(t *testing.T, a *app) (prevCohortID, prevStuID, curCohortID, curStuID int64) {
	t.Helper()
	ctx := context.Background()

	c2025, err := a.store.CreateCohort(ctx, 2025, 2026)
	if err != nil {
		t.Fatal(err)
	}
	st2025, err := a.store.GetCohortStage(ctx, c2025.ID, 2025)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ApplyStageImport(ctx, c2025.ID, st2025.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班"},
			Students: []store.ImportStudent{
				{StudentNo: "2025009", Name: "林往届", ClassName: "1班", Track: "physics", Status: "active", SourceRow: 2},
			},
		},
		Report: store.ImportReport{FileHash: "p25-stu", Path: "p25.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatal(err)
	}
	prevList, _, err := a.store.ListStudents(ctx, store.StudentFilter{CohortID: c2025.ID, StageID: st2025.ID})
	if err != nil || len(prevList) == 0 {
		t.Fatalf("prevList=%+v err=%v", prevList, err)
	}
	prevStuID = prevList[0].ID

	c2026, err := a.store.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	st2026, err := a.store.GetCohortStage(ctx, c2026.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	entryTotal := 565.0
	if _, err := a.store.ApplyStageImport(ctx, c2026.ID, st2026.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班", "2班"},
			Students: []store.ImportStudent{
				{
					StudentNo: "2026001", CandidateCode: "CAND2601", Name: "陈远航",
					ClassName: "1班", Track: "physics", Elective: "物化生", Status: "active",
					TotalScore: &entryTotal,
					Scores:     map[string]float64{"chinese": 112, "math": 128, "english": 120},
					SourceRow:  2,
				},
				{
					StudentNo: "2026002", Name: "周同学",
					ClassName: "2班", Track: "physics", Status: "active", SourceRow: 3,
				},
			},
		},
		Report: store.ImportReport{FileHash: "p26-stu", Path: "p26.xlsx", RowCount: 2},
	}); err != nil {
		t.Fatal(err)
	}
	curList, _, err := a.store.ListStudents(ctx, store.StudentFilter{CohortID: c2026.ID, StageID: st2026.ID, Query: "2026001"})
	if err != nil || len(curList) == 0 {
		t.Fatalf("curList=%+v err=%v", curList, err)
	}
	curStuID = curList[0].ID
	if err := a.store.SetStudentPreviousLink(ctx, curStuID, prevStuID); err != nil {
		t.Fatal(err)
	}

	// 连续创建高一、高二（升级）、高三（升级）三场考试
	type examSpec struct {
		year    int
		term    string
		name    string
		date    string
		chinese float64
		math    float64
		english float64
		peerChn float64
		peerMat float64
		peerEng float64
	}
	specs := []examSpec{
		{year: 2026, term: store.ExamTermFirst, name: "高一期中", date: "2026-11-12", chinese: 114, math: 130, english: 118, peerChn: 118, peerMat: 135, peerEng: 122},
		{year: 2027, term: store.ExamTermFirst, name: "高二期中", date: "2027-11-10", chinese: 119, math: 138, english: 125, peerChn: 115, peerMat: 130, peerEng: 120},
		{year: 2028, term: store.ExamTermFirst, name: "高三一模", date: "2028-11-08", chinese: 124, math: 142, english: 131, peerChn: 112, peerMat: 128, peerEng: 118},
	}

	for idx, sp := range specs {
		if idx == 1 {
			if _, err := a.store.PromoteCohort(ctx, c2026.ID, store.PromoteInput{
				ToPhase: store.PhaseG2, ToAcademicYearStart: 2027, Mode: store.PromoteDirect,
			}); err != nil {
				t.Fatal(err)
			}
		} else if idx == 2 {
			if _, err := a.store.PromoteCohort(ctx, c2026.ID, store.PromoteInput{
				ToPhase: store.PhaseG3, ToAcademicYearStart: 2028, Mode: store.PromoteDirect,
			}); err != nil {
				t.Fatal(err)
			}
		}
		ex, err := a.store.CreateExam(ctx, store.CreateExamInput{
			AcademicYearStart: sp.year,
			Term:              sp.term,
			ExamType:          store.ExamMidterm,
			Name:              sp.name,
			ExamDate:          sp.date,
			CohortIDs:         []int64{c2026.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.store.GenerateExamRoster(ctx, ex.ID, c2026.ID, "physics"); err != nil {
			t.Fatal(err)
		}
		if _, err := a.store.ApplyExamImport(ctx, store.ExamImportRequest{
			ExamID:   ex.ID,
			CohortID: c2026.ID,
			Kind:     store.ExamImportScores,
			Data: store.ImportData{
				Students: []store.ImportStudent{
					{
						StudentNo: "2026001", Name: "陈远航",
						Scores:    map[string]float64{"chinese": sp.chinese, "math": sp.math, "english": sp.english},
						SourceRow: 2,
					},
					{
						StudentNo: "2026002", Name: "周同学",
						Scores:    map[string]float64{"chinese": sp.peerChn, "math": sp.peerMat, "english": sp.peerEng},
						SourceRow: 3,
					},
				},
			},
			Report: store.ImportReport{FileHash: "sc-" + strconv.Itoa(idx), Path: "scores.xlsx", RowCount: 2},
		}); err != nil {
			t.Fatal(err)
		}
	}

	a.reloadCohorts(ctx)
	return c2025.ID, prevStuID, c2026.ID, curStuID
}

func TestProfilePages_StudentProfile_EmptyAndPopulated(t *testing.T) {
	useChineseUI(t)
	ctx := context.Background()

	// 1. Empty state: 学生无任何入口单科、无考试记录、无异动事件
	aEmpty := newTestApp(t)
	cEmpty, err := aEmpty.store.CreateCohort(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	stEmpty, err := aEmpty.store.GetCohortStage(ctx, cEmpty.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := aEmpty.store.ApplyStageImport(ctx, cEmpty.ID, stEmpty.ID, store.StageImportRequest{
		Kind: "students",
		Data: store.ImportData{
			Classes: []string{"1班"},
			Students: []store.ImportStudent{
				{StudentNo: "E001", Name: "空档案生", ClassName: "1班", Track: "physics", Status: "active", SourceRow: 2},
			},
		},
		Report: store.ImportReport{FileHash: "empty-stu", Path: "empty.xlsx", RowCount: 1},
	}); err != nil {
		t.Fatal(err)
	}
	aEmpty.reloadCohorts(ctx)
	aEmpty.page = pageCohorts
	aEmpty.openCohortDetail(cEmpty.ID)
	aEmpty.cohortDetailTab = 2
	ttEmpty := newChineseTester(t, aEmpty.view, 1280, 800)
	if err := ttEmpty.Click("空档案生"); err != nil {
		t.Fatal(err)
	}
	settleTester(ttEmpty)
	if !aEmpty.studentProfileOpen {
		t.Fatal("empty student profile did not open")
	}
	for _, want := range []string{
		locales.StudentProfileChartEmpty(),
		locales.StudentProfileExamTableEmpty(),
		locales.StudentEventTypeEnrolled(),
	} {
		if !ttEmpty.HasText(want) {
			t.Fatalf("missing empty state %q in %q", want, ttEmpty.Texts())
		}
	}
	captureDualSnapshot(t, ttEmpty, "view_student_profile_empty")

	// 2. Populated state: 三年多考 + 中考入口锚点 + 复读前溯 + 图表指标切换 + 记录异动弹窗 + 导出
	a := newTestApp(t)
	prevCohortID, prevStuID, curCohortID, curStuID := seedThreeYearStudentFixture(t, a)

	a.page = pageCohorts
	a.openCohortDetail(curCohortID)
	a.cohortDetailTab = 2
	tt := newChineseTester(t, a.view, 1280, 800)

	if err := tt.Click("陈远航"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.studentProfileOpen || a.selectedStudentID != curStuID {
		t.Fatalf("student profile did not open: open=%v id=%d want=%d", a.studentProfileOpen, a.selectedStudentID, curStuID)
	}
	if len(a.studentProfile.Exams) != 3 {
		t.Fatalf("expected 3 exams across G1-G3, got %d", len(a.studentProfile.Exams))
	}

	// 验证默认年级排名模式与极值摘要卡
	if !tt.HasText(locales.StudentProfileTrendTitle()) || !tt.HasText(locales.StudentProfileBestRank()) {
		t.Fatalf("missing chart or best rank card: %q", tt.Texts())
	}
	captureDualSnapshot(t, tt, "view_student_profile_populated")

	// 切换走势图指标：类别排名 -> 总分（含中考入口锚点） -> 单科成绩
	if err := tt.Click(locales.StudentProfileMetricTrackRank()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.studentChartMetric != 1 {
		t.Fatalf("expected studentChartMetric=1, got %d", a.studentChartMetric)
	}

	if err := tt.Click(locales.StudentProfileMetricTotalScore()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.studentChartMetric != 2 || !tt.HasText(locales.StudentProfileEntryAnchor()) || !tt.HasText(locales.StudentProfileHighestScore()) {
		t.Fatalf("total score chart state unexpected: metric=%d texts=%q", a.studentChartMetric, tt.Texts())
	}

	if err := tt.Click(locales.StudentProfileMetricSubject()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.studentChartMetric != 3 || !tt.HasText(locales.StudentProfileSelectSubjects()) {
		t.Fatalf("subject chart state unexpected: metric=%d texts=%q", a.studentChartMetric, tt.Texts())
	}

	// 向下滚动并点击历次考试行展开单科详情
	tt.Scroll(600, 400, 0, 600)
	settleTester(tt)
	if err := tt.Click("高三一模 (2028-11-08)"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.studentSelectedExamIdx < 0 || !tt.HasText(locales.StudentProfileOpenExamBtn()) {
		t.Fatalf("expected exam row detail expansion, idx=%d texts=%q", a.studentSelectedExamIdx, tt.Texts())
	}

	// 滚回顶部验证复读前溯跳转与多层返回栈
	tt.Scroll(600, 400, 0, -600)
	settleTester(tt)
	prevLink := "曾就读于 " + a.cohortName(2025) + " · 原档案（林往届）"
	if err := tt.Click(prevLink); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.selectedStudentID != prevStuID || len(a.studentProfileStack) != 1 {
		t.Fatalf("previous student jump failed: selected=%d want=%d stack=%+v", a.selectedStudentID, prevStuID, a.studentProfileStack)
	}
	if a.selectedCohortID != strconv.FormatInt(prevCohortID, 10) {
		t.Fatalf("expected cohort switched to %d, got %q", prevCohortID, a.selectedCohortID)
	}
	// 点击返回回到陈远航档案
	if err := tt.Click(locales.Back()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.selectedStudentID != curStuID || len(a.studentProfileStack) != 0 {
		t.Fatalf("back stack pop failed: selected=%d want=%d stack=%+v", a.selectedStudentID, curStuID, a.studentProfileStack)
	}

	// 打开「记录异动」弹窗，抓取快照并提交转班异动
	if err := tt.Click(locales.StudentProfileRecordEvent()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.studentEventModalOpen {
		t.Fatal("student event modal did not open")
	}
	a.studentEventClassChoice = "2班"
	a.studentEventNote = "高三冲刺转入2班"
	settleTester(tt)
	captureDualSnapshot(t, tt, "modal_student_event_populated")

	if err := tt.Click(locales.Save()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.studentEventModalOpen || a.studentEventError != "" {
		t.Fatalf("student event submit failed: open=%v err=%q", a.studentEventModalOpen, a.studentEventError)
	}
	if !tt.HasText("高三冲刺转入2班") {
		t.Fatalf("expected new student event note in timeline: %q", tt.Texts())
	}

	// 验证学生档案 Excel 导出
	a.chooseStudentProfileExport()
	if a.dataError != "" {
		t.Fatalf("export student profile error: %s", a.dataError)
	}
	entries, err := os.ReadDir(a.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	foundXLSX := false
	for _, ent := range entries {
		if filepath.Ext(ent.Name()) == ".xlsx" {
			foundXLSX = true
			break
		}
	}
	if !foundXLSX {
		t.Fatalf("expected exported xlsx in %s", a.dataDir)
	}

	// 验证 1024×640 最小窗口无溢出且状态稳定
	a.mainToast = ""
	a.studentChartMetric = 0
	ttMin := newChineseTester(t, a.view, 1024, 640)
	assertAccessibleWithin(t, ttMin, 1024, 640)
	if ttMin.FramePending() {
		t.Fatal("student profile 1024x640 did not settle")
	}
	captureDualSnapshot(t, ttMin, "view_student_profile_1024")
}

func TestProfilePages_TeacherProfile_EmptyIdleAndPopulated(t *testing.T) {
	useChineseUI(t)
	ctx := context.Background()

	// 1. Empty / Idle（已毕业届释放回空闲）状态
	aIdle := newTestApp(t)
	c2024, err := aIdle.store.CreateCohort(ctx, 2024, 2026)
	if err != nil {
		t.Fatal(err)
	}
	st2024, err := aIdle.store.GetCohortStage(ctx, c2024.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	cls2024, err := aIdle.store.CreateClass(ctx, c2024.ID, store.PhaseG3, "1班", "physics", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := aIdle.store.CreateCohort(ctx, 2026, 2026); err != nil {
		t.Fatal(err)
	}
	idleTeacher, err := aIdle.store.CreateTeacher(ctx, store.ImportTeacher{
		TeacherNo: "T-IDLE", Name: "周闲置", Subject: "physics",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := aIdle.store.SetClassHeadTeacher(ctx, cls2024.ID, idleTeacher.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := aIdle.store.GraduateCohort(ctx, c2024.ID); err != nil {
		t.Fatal(err)
	}
	_ = st2024
	aIdle.reloadCohorts(ctx)
	aIdle.page = pageFaculty
	ttIdle := newChineseTester(t, aIdle.view, 1280, 800)
	if err := ttIdle.Click("周闲置"); err != nil {
		t.Fatal(err)
	}
	settleTester(ttIdle)
	if !aIdle.teacherProfileOpen || aIdle.teacherProfile.Occupancy.Status != store.TeacherStatusIdle {
		t.Fatalf("expected idle teacher profile, open=%v occ=%+v", aIdle.teacherProfileOpen, aIdle.teacherProfile.Occupancy)
	}
	if !ttIdle.HasText(locales.TeacherProfileIdleCardTitle()) || !ttIdle.HasText(locales.TeacherProfileCrossChartEmpty()) {
		t.Fatalf("missing idle teacher sections: %q", ttIdle.Texts())
	}
	captureDualSnapshot(t, ttIdle, "view_teacher_profile_empty")

	// 点击「去年级分配任课」快捷跳转
	if err := ttIdle.Click(locales.TeacherProfileGoAssignBtn()); err != nil {
		t.Fatal(err)
	}
	settleTester(ttIdle)
	if aIdle.page != pageCohorts || !aIdle.cohortDetailOpen || aIdle.cohortDetailTab != 3 {
		t.Fatalf("expected jump to cohort assignments tab, got page=%q detail=%v tab=%d",
			aIdle.page, aIdle.cohortDetailOpen, aIdle.cohortDetailTab)
	}

	// 2. Populated 状态：跨届兼任 + 班主任 + 跨届个人得分曲线 + 折叠履历 + 任课矩阵入口 + 编辑弹窗
	a := newChinesePopulatedTestApp(t)
	c2025, err := a.store.CreateCohort(ctx, 2025, 2026)
	if err != nil {
		t.Fatal(err)
	}
	cls2025, err := a.store.CreateClass(ctx, c2025.ID, store.PhaseG2, "1班", "physics", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	grp2025, err := a.store.CreateTeachingGroup(ctx, c2025.ID, store.PhaseG2, "chinese", "1班语文", "admin", cls2025.ID)
	if err != nil {
		t.Fatal(err)
	}
	a.reloadTeacherDirectory(ctx)
	var zhangID int64
	for _, occ := range a.teacherOccupancy {
		if occ.Name == "张老师" {
			zhangID = occ.TeacherID
			break
		}
	}
	if zhangID == 0 {
		t.Fatal("missing 张老师")
	}
	if _, err := a.store.AssignTeacherToGroup(ctx, zhangID, grp2025.ID, "primary"); err != nil {
		t.Fatal(err)
	}
	// 为 2026 级 1班 设置班主任为张老师，同时为 2025 级增加一次正式考核记录以形成跨届双色得分点
	if len(a.classes) > 0 {
		if _, err := a.store.SetClassHeadTeacher(ctx, a.classes[0].ID, zhangID); err != nil {
			t.Fatal(err)
		}
	}
	ex2025, err := a.store.CreateExam(ctx, store.CreateExamInput{
		AcademicYearStart: 2026, Term: store.ExamTermFirst, ExamType: store.ExamMidterm,
		Name: "2025级高二期中", ExamDate: "2026-11-18", CohortIDs: []int64{c2025.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ec2025, err := a.store.GetExamCohort(ctx, ex2025.ID, c2025.ID)
	if err != nil {
		t.Fatal(err)
	}
	assess2025, err := a.store.AssessmentForExamCohort(ctx, ec2025.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.SaveAssessmentRun(ctx, assess2025.ID, a.versions[0].ID, "h-2025", nil, calc.Result{
		Rows: []calc.Row{{
			StatGroupID:      strconv.FormatInt(grp2025.ID, 10),
			StatGroupName:    "1班语文",
			Subject:          "chinese",
			LeadClassID:      strconv.FormatInt(cls2025.ID, 10),
			LeadClassName:    "1班",
			TeacherIDs:       []string{strconv.FormatInt(zhangID, 10)},
			TeacherNames:     []string{"张老师"},
			ClassAverage:     108.5,
			GradeAverage:     102.0,
			ImprovementScore: 103.0,
			PassScore:        96.0,
			CompletionScore:  92.0,
			Total:            97.8,
			Rank:             1,
			TierIndex:        1,
			Points:           96.5,
		}},
		Personal: []calc.PersonalScore{{
			TeacherID:   strconv.FormatInt(zhangID, 10),
			TeacherName: "张老师",
			Subject:     "chinese",
			GroupPoints: []float64{96.5},
			Score:       96.5,
		}},
	}, true); err != nil {
		t.Fatal(err)
	}

	a.reloadCohorts(ctx)

	// 验证从年级「任课安排」矩阵点击教师姓名进入教师执教档案，并能一键返回年级任课
	c2026ID := a.cohorts[0].ID
	if a.cohorts[0].EnrollmentYear != 2026 {
		for _, co := range a.cohorts {
			if co.EnrollmentYear == 2026 {
				c2026ID = co.ID
				break
			}
		}
	}
	a.page = pageCohorts
	a.openCohortDetail(c2026ID)
	a.cohortDetailTab = 3
	tt := newChineseTester(t, a.view, 1280, 800)
	if err := tt.Click("张老师"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.teacherProfileOpen || a.selectedTeacherID != zhangID || a.teacherProfileReturnCohort != c2026ID {
		t.Fatalf("assignment matrix click failed: open=%v tid=%d ret=%d",
			a.teacherProfileOpen, a.selectedTeacherID, a.teacherProfileReturnCohort)
	}
	if !tt.HasText(locales.TeacherProfileBackToCohort()) {
		t.Fatalf("missing back to cohort button: %q", tt.Texts())
	}
	if err := tt.Click(locales.TeacherProfileBackToCohort()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.teacherProfileOpen || a.page != pageCohorts || a.cohortDetailTab != 3 {
		t.Fatalf("back to cohort failed: open=%v page=%q tab=%d", a.teacherProfileOpen, a.page, a.cohortDetailTab)
	}

	// 从教师档案目录点击打开张老师完整档案
	a.page = pageFaculty
	ttDir := newChineseTester(t, a.view, 1280, 800)
	if err := ttDir.Click("张老师"); err != nil {
		t.Fatal(err)
	}
	settleTester(ttDir)
	if !a.teacherProfileOpen || a.teacherProfile.Occupancy.Status != store.TeacherStatusActiveMulti {
		t.Fatalf("expected active_multi occupancy, got %+v", a.teacherProfile.Occupancy)
	}
	if !ttDir.HasText(locales.TeacherProfileRoleHead()) || !ttDir.HasText(locales.TeacherProfileCrossChartTitle()) {
		t.Fatalf("missing head teacher badge or cross chart: %q", ttDir.Texts())
	}
	captureDualSnapshot(t, ttDir, "view_teacher_profile_populated")

	// 向下滚动并测试折叠与展开某届历史履历
	ttDir.Scroll(600, 400, 0, 700)
	settleTester(ttDir)
	beforeExpanded := a.teacherCohortExpanded[c2025.ID]
	summary2025 := a.cohortName(2025) + " · " + locales.TeacherProfileHistoryCohortActive()
	for _, info := range ttDir.AccessAll() {
		if info.Role == ui.RoleDisclosure || info.Role == ui.RoleButton {
			if len(info.Name) >= len(summary2025) && info.Name[:len(summary2025)] == summary2025 {
				if err := ttDir.Click(info.Name); err != nil {
					t.Fatal(err)
				}
				break
			}
		}
	}
	settleTester(ttDir)
	if a.teacherCohortExpanded[c2025.ID] == beforeExpanded {
		t.Fatalf("expected cohort history collapsible state to toggle")
	}

	// 滚回顶部并测试「编辑基本信息」弹窗
	ttDir.Scroll(600, 400, 0, -700)
	settleTester(ttDir)
	if err := ttDir.Click(locales.TeacherProfileEditBtn()); err != nil {
		t.Fatal(err)
	}
	settleTester(ttDir)
	if !a.editTeacherOpen {
		t.Fatal("edit teacher modal did not open")
	}
	a.editTeacherNote = "高三语文备课组长"
	settleTester(ttDir)
	captureDualSnapshot(t, ttDir, "modal_edit_teacher_populated")

	if err := ttDir.Click(locales.Save()); err != nil {
		t.Fatal(err)
	}
	settleTester(ttDir)
	if a.editTeacherOpen || a.teacherProfile.Teacher.Note != "高三语文备课组长" {
		t.Fatalf("edit teacher submit failed: open=%v note=%q err=%q",
			a.editTeacherOpen, a.teacherProfile.Teacher.Note, a.editTeacherError)
	}

	// 验证教师执教档案 Excel 导出
	a.chooseTeacherProfileExport()
	if a.dataError != "" {
		t.Fatalf("export teacher profile error: %s", a.dataError)
	}
	entries, err := os.ReadDir(a.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	foundXLSX := false
	for _, ent := range entries {
		if filepath.Ext(ent.Name()) == ".xlsx" {
			foundXLSX = true
			break
		}
	}
	if !foundXLSX {
		t.Fatalf("expected exported xlsx in %s", a.dataDir)
	}

	// 验证 1024×640 最小窗口无溢出且状态稳定
	ttMin := newChineseTester(t, a.view, 1024, 640)
	assertAccessibleWithin(t, ttMin, 1024, 640)
	if ttMin.FramePending() {
		t.Fatal("teacher profile 1024x640 did not settle")
	}
	captureDualSnapshot(t, ttMin, "view_teacher_profile_1024")
}

func TestProfilePages_AsyncLoadErrorShowsErrorStateNotStaleOrZero(t *testing.T) {
	useChineseUI(t)
	a := newChinesePopulatedTestApp(t)
	ctx := context.Background()
	cohortID := a.cohorts[0].ID

	// 1. 先打开一个真实学生，再切换到不存在的学生 ID，断言显示错误态而非旧缓存或「0级」。
	students, _, err := a.store.ListStudents(ctx, store.StudentFilter{CohortID: cohortID, PageSize: 10})
	if err != nil || len(students) == 0 {
		t.Fatalf("ListStudents: %+v err=%v", students, err)
	}
	prevStudentName := students[0].Name
	a.page = pageCohorts
	a.openCohortDetail(cohortID)
	a.openStudentProfile(students[0].ID)
	tt := newChineseTester(t, a.view, 1280, 800)
	if !tt.HasText(prevStudentName) {
		t.Fatalf("expected valid student %q loaded first, got %q", prevStudentName, tt.Texts())
	}

	a.openStudentProfile(999999)
	settleTester(tt)
	if !tt.HasText(locales.StudentProfileLoadErrorTitle()) {
		t.Fatalf("expected student load error title %q, got %q", locales.StudentProfileLoadErrorTitle(), tt.Texts())
	}
	if tt.HasText("0级") || tt.HasText(prevStudentName) {
		t.Fatalf("student error state leaked zero-value or stale profile: %q", tt.Texts())
	}

	// 2. 先打开一个真实教师，再切换到不存在的教师 ID，断言显示错误态而非旧缓存。
	a.reloadTeacherDirectory(ctx)
	if len(a.teacherOccupancy) == 0 {
		t.Fatal("expected at least one teacher")
	}
	prevTeacherName := a.teacherOccupancy[0].Name
	a.page = pageFaculty
	a.openTeacherProfile(a.teacherOccupancy[0].TeacherID)
	settleTester(tt)
	if !tt.HasText(prevTeacherName) {
		t.Fatalf("expected valid teacher %q loaded first, got %q", prevTeacherName, tt.Texts())
	}

	a.openTeacherProfile(999999)
	settleTester(tt)
	if !tt.HasText(locales.TeacherProfileLoadErrorTitle()) {
		t.Fatalf("expected teacher load error title %q, got %q", locales.TeacherProfileLoadErrorTitle(), tt.Texts())
	}
	if tt.HasText(prevTeacherName) {
		t.Fatalf("teacher error state leaked stale profile %q: %q", prevTeacherName, tt.Texts())
	}
}
