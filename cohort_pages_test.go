package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func TestStage2ACohortLifecyclePromoteAndGraduate(t *testing.T) {
	useChineseUI(t)
	a := newChinesePopulatedTestApp(t)
	ctx := context.Background()
	cohortID := a.cohorts[0].ID

	// 新增一名跨届兼任教师（同时带 2025 级）和一名仅带 2026 级的教师，以验证毕业释放预览与执行。
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

	a.reloadCohorts(ctx)
	a.page = pageCohorts
	a.openCohortDetail(cohortID)
	tt := newChineseTester(t, a.view, 1280, 800)

	// 验证年级详情 6 个页签与快照。
	for _, tab := range []string{
		locales.CohortTabOverview(),
		locales.CohortTabClasses(),
		locales.CohortTabStudents(),
		locales.CohortTabAssignments(),
		locales.CohortTabBaselines(),
		locales.CohortTabExams(),
	} {
		if !tt.HasText(tab) {
			t.Fatalf("missing cohort detail tab %q in %q", tab, tt.Texts())
		}
	}
	captureDualSnapshot(t, tt, "view_cohort_detail_populated")

	// 打开升级流转弹窗（模式 A 原班直升到高二）。
	if err := tt.Click(locales.CohortPromoteAction()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.promoteOpen {
		t.Fatal("promote modal did not open")
	}
	captureDualSnapshot(t, tt, "modal_promote_cohort")

	if err := tt.Click(locales.CohortPromoteConfirm()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.promoteOpen || a.selectedCohort() == nil || a.selectedCohort().CurrentPhase != store.PhaseG2 {
		t.Fatalf("promote failed: open=%v cohort=%+v err=%q", a.promoteOpen, a.selectedCohort(), a.promoteError)
	}

	// 切换到「班级与教学班」页签，查看班级继承链走势弹窗。
	if err := tt.Click(locales.CohortTabClasses()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if err := tt.Click(locales.CohortClassTrendBtn()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.classTrendOpen || len(a.classTrendNodes) < 2 {
		t.Fatalf("class trend chain did not open with 2 nodes: open=%v nodes=%+v", a.classTrendOpen, a.classTrendNodes)
	}
	if err := tt.Click(locales.Close()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)

	// 回到「概览与周期」执行毕业封存预览并确认：李老师释放回空闲，张老师因兼任 2025 级仍保持跨届/在任。
	if err := tt.Click(locales.CohortTabOverview()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if err := tt.Click(locales.CohortGraduateAction()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.graduateOpen || a.graduatePreview.ReleasedToIdleCount != 1 || a.graduatePreview.StillActiveCrossCohortCount != 1 {
		t.Fatalf("graduate preview unexpected: open=%v pv=%+v", a.graduateOpen, a.graduatePreview)
	}
	for _, want := range []string{
		locales.CohortGraduateImpactStudents(),
		locales.CohortGraduateImpactExams(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("graduate modal missing full label %q in %q", want, tt.Texts())
		}
	}
	captureDualSnapshot(t, tt, "modal_graduate_cohort")

	if err := tt.Click(locales.CohortGraduateConfirm()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.graduateOpen || a.selectedCohort().CurrentPhase != store.PhaseGraduated {
		t.Fatalf("graduate failed: open=%v phase=%q err=%q", a.graduateOpen, a.selectedCohort().CurrentPhase, a.graduateError)
	}
	if !tt.HasText(locales.CohortArchiveNotice()) {
		t.Fatalf("missing read-only archive notice after graduation: %q", tt.Texts())
	}

	// 验证回到年级列表切换到「已毕业历史届」可见 2026 级。
	if err := tt.Click(locales.Back()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.cohortDetailOpen {
		t.Fatal("cohort detail did not close via back button")
	}
	if err := tt.Click(locales.CohortFilterGraduated()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !tt.HasText(a.cohortName(2026)) {
		t.Fatalf("graduated archive missing 2026 cohort: %q", tt.Texts())
	}
}

func TestStage2ATeachersDirectoryFiltersProfileAndSnapshots(t *testing.T) {
	useChineseUI(t)
	aEmpty := newTestApp(t)
	aEmpty.page = pageFaculty
	ttEmpty := newChineseTester(t, aEmpty.view, 1280, 800)
	captureDualSnapshot(t, ttEmpty, "view_teachers_empty")

	a := newChinesePopulatedTestApp(t)
	a.page = pageFaculty
	tt := newChineseTester(t, a.view, 1280, 800)
	captureDualSnapshot(t, tt, "view_teachers_populated")

	// 新建一位空闲教师并验证筛选胶囊与搜索。
	if err := tt.Click(locales.TeachersNewTeacher()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.newTeacherOpen {
		t.Fatal("new teacher modal did not open")
	}
	a.newTeacherNo = "T99"
	a.newTeacherName = "王老师"
	a.newTeacherSubject = "english"
	if err := tt.Click(locales.Save()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.newTeacherOpen || !tt.HasText("王老师") {
		t.Fatalf("created teacher missing: open=%v err=%q texts=%q", a.newTeacherOpen, a.newTeacherError, tt.Texts())
	}

	// 点击「空闲」筛选胶囊仅展示王老师。
	idlePill := fmt.Sprintf("%s (%s)", locales.TeachersFilterIdle(), formatInt(1))
	if err := tt.Click(idlePill); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !tt.HasText("王老师") || tt.HasText("张老师") {
		t.Fatalf("idle filter unexpected: %q", tt.Texts())
	}

	// 点击王老师打开教师执教档案详情页，并切换停用状态。
	if err := tt.Click("王老师"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.teacherProfileOpen || !tt.HasText(locales.TeacherProfileOccupancyTitle()) {
		t.Fatalf("teacher profile did not open: open=%v texts=%q", a.teacherProfileOpen, tt.Texts())
	}
	if err := tt.Click(locales.TeachersToggleInactive()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.teacherProfile.Occupancy.Status != store.TeacherStatusInactive {
		t.Fatalf("status after deactivate = %q", a.teacherProfile.Occupancy.Status)
	}
}

func TestStage2AExamHubSubnavSwitching(t *testing.T) {
	a := newPopulatedTestApp(t)
	a.selectedExamID = 0
	a.page = pageExams
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	if err := tt.Click(locales.ExamHubTabWorkbench()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageAssessments {
		t.Fatalf("expected pageAssessments, got %q", a.page)
	}
	if err := tt.Click(locales.ExamHubTabQuality()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageQuality {
		t.Fatalf("expected pageQuality, got %q", a.page)
	}
	if err := tt.Click(locales.ExamHubTabAnalysis()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageAnalysis {
		t.Fatalf("expected pageAnalysis, got %q", a.page)
	}
	if err := tt.Click(locales.ExamHubTabExams()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.page != pageExams {
		t.Fatalf("expected pageExams, got %q", a.page)
	}
}

func TestStage2AWalkingStudentsModalFullCandidates(t *testing.T) {
	useChineseUI(t)
	a := newChinesePopulatedTestApp(t)
	ctx := context.Background()
	cohortID := a.cohorts[0].ID

	// 创建走班教学班，并在学生档案页施加严格搜索过滤与极小分页，使 a.students 仅剩 1 人。
	walkingGrp, err := a.store.CreateTeachingGroup(ctx, cohortID, store.PhaseG1, "japanese", "日语走班1", "walking", 0)
	if err != nil {
		t.Fatal(err)
	}
	allStudents, total, err := a.store.ListStudents(ctx, store.StudentFilter{CohortID: cohortID, Status: "active", PageSize: 200})
	if err != nil || total < 2 {
		t.Fatalf("expected >=2 active students, got total=%d err=%v", total, err)
	}

	a.page = pageCohorts
	a.openCohortDetail(cohortID)
	a.dataQuery = allStudents[0].Name
	a.studentPageSize = 1
	a.reloadSelectedCohort(ctx)
	if len(a.students) > 1 {
		t.Fatalf("expected filtered a.students <= 1, got %d", len(a.students))
	}

	// 打开走班加人弹窗，断言候选名单包含该阶段全部在读学生，不受 a.students 筛选与分页限制。
	a.openWalkingStudentsModal(store.DataTeachingGroup{
		ID:      walkingGrp.ID,
		Name:    walkingGrp.Name,
		Subject: walkingGrp.Subject,
		Kind:    walkingGrp.Kind,
	})
	if len(a.walkingCandidateStudents) != total {
		t.Fatalf("walkingCandidateStudents = %d, want full %d active students", len(a.walkingCandidateStudents), total)
	}
}
