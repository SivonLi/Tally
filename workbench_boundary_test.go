package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

func TestWorkbenchBoundaryOverridesFlowAndLocales(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.page = pageAssessments

	tt := ui.NewTester(a.view, 1280, 820)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	// 1. 打开分档边界弹窗并验证预填
	if err := tt.Click(locales.BoundaryAdjustButton()); err != nil {
		t.Fatalf("click adjust boundaries: %v", err)
	}
	settleTester(tt)
	if !a.boundaryState().open {
		t.Fatal("expected boundary modal to open")
	}
	if len(a.boundaryState().rows) == 0 {
		t.Fatal("expected prefilled subject rows in boundary modal")
	}
	if _, ok := tt.Find(locales.BoundaryModalTitle()); !ok {
		t.Fatal("expected boundary modal title visible")
	}

	// 2. 修改语文备注并保存（当前 fixture 含 1 个参评统计组）
	st := a.boundaryState()
	st.rows[0].First = 1
	st.rows[0].Second = 1
	st.rows[0].Reason = "语文微调"
	settleTester(tt)
	if err := tt.Click(locales.BoundarySave()); err != nil {
		t.Fatalf("click save boundaries: %v", err)
	}
	settleTester(tt)
	if a.boundaryState().open {
		t.Fatalf("expected boundary modal to close after save (err=%q, rows=%+v)", a.boundaryState().errorText, a.boundaryState().rows)
	}
	if a.boundaryState().overrideCount != 1 {
		t.Fatalf("expected overrideCount = 1, got %d", a.boundaryState().overrideCount)
	}
	if _, ok := tt.Find(fmt.Sprintf(locales.BoundaryOverrideCountBadge(), 1)); !ok {
		t.Fatal("expected override count badge visible on workbench")
	}

	// 3. 正式计算并在结果详情中回显生效边界与来源
	if err := tt.Click(locales.AssessmentCalculate()); err != nil {
		t.Fatalf("click calculate: %v", err)
	}
	settleTester(tt)
	if len(a.assessmentRows) == 0 {
		t.Fatal("expected calculated assessment rows")
	}
	a.openResultDetail(a.assessmentRows[0])
	settleTester(tt)
	if _, ok := tt.Find(locales.AssessmentDetailBoundary()); !ok {
		t.Fatal("expected effective boundary row in result detail modal")
	}
	if _, ok := tt.Find(locales.AssessmentDetailBoundarySourceOverride()); !ok {
		t.Fatal("expected override source in result detail modal")
	}
	a.closeResultDetail()
	settleTester(tt)

	// 4. 切换英文语言后打开弹窗，验证单科恢复默认与工作台变为 stale
	t.Setenv("MYGO_LOCALE", "en")
	tt.SetLocale("en")
	settleTester(tt)
	if err := tt.Click(locales.BoundaryAdjustButton()); err != nil {
		t.Fatalf("click adjust boundaries in en: %v", err)
	}
	settleTester(tt)
	firstSubj := subjectLabel(a.boundaryState().rows[0].Subject)
	if err := tt.Click(firstSubj + " " + locales.BoundaryResetSubject()); err != nil {
		t.Fatalf("click single subject reset: %v", err)
	}
	settleTester(tt)
	if a.boundaryState().overrideCount != 0 {
		t.Fatalf("expected overrideCount = 0 after single reset, got %d", a.boundaryState().overrideCount)
	}
	if err := tt.Click(locales.BoundaryCancel()); err != nil {
		t.Fatalf("close boundary modal: %v", err)
	}
	settleTester(tt)
	if !a.assessmentOfficialStale {
		t.Fatal("expected assessment to be marked stale after resetting boundary override")
	}

	// 5. 全部恢复默认按钮验证
	ctx := context.Background()
	if err := a.store.SaveAssessmentBoundaryOverrides(ctx, a.selectedAssessmentID, []store.AssessmentBoundaryOverride{
		{Subject: "chinese", GroupCount: 1, FirstRank: 1, SecondRank: 1, Reason: "temp"},
	}); err != nil {
		t.Fatal(err)
	}
	a.reloadAssessments(ctx)
	tt.Frame()
	if err := tt.Click(locales.BoundaryAdjustButton()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if err := tt.Click(locales.BoundaryResetAll()); err != nil {
		t.Fatalf("click reset all: %v", err)
	}
	settleTester(tt)
	if a.boundaryState().overrideCount != 0 {
		t.Fatalf("expected overrideCount = 0 after reset all, got %d", a.boundaryState().overrideCount)
	}
}

func TestWorkbenchBoundaryStaleInterception(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	fixture := newAssessmentPageFixture(t, false)
	a := fixture.App
	a.page = pageAssessments
	ctx := context.Background()

	// 写入与当前参评组数(2)不匹配的微调(5)
	if err := a.store.SaveAssessmentBoundaryOverrides(ctx, a.selectedAssessmentID, []store.AssessmentBoundaryOverride{
		{Subject: "chinese", GroupCount: 5, FirstRank: 2, SecondRank: 4, Reason: "组数已变化"},
	}); err != nil {
		t.Fatal(err)
	}
	a.reloadAssessments(ctx)

	tt := ui.NewTester(a.view, 1280, 820)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if err := tt.Click(locales.AssessmentCalculate()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.assessmentError == "" {
		t.Fatal("expected BoundaryStaleError message on workbench")
	}
	if _, ok := tt.Find(locales.BoundaryReviewButton()); !ok {
		t.Fatal("expected review boundaries shortcut button on workbench")
	}

	// 点击复核入口打开弹窗并验证高亮状态与一键沿用
	if err := tt.Click(locales.BoundaryReviewButton()); err != nil {
		t.Fatalf("click review boundaries: %v", err)
	}
	settleTester(tt)
	if !a.boundaryState().open {
		t.Fatal("expected boundary modal to open from review shortcut")
	}
	if _, ok := tt.Find(locales.BoundaryStatusStale()); !ok {
		t.Fatal("expected stale status badge inside boundary modal")
	}
	if err := tt.Click(subjectLabel("chinese") + " " + locales.BoundaryKeepCurrentCount()); err != nil {
		t.Fatalf("click keep current count: %v", err)
	}
	settleTester(tt)
	if err := tt.Click(locales.BoundarySave()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if err := tt.Click(locales.AssessmentCalculate()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.assessmentError != "" || len(a.assessmentRows) == 0 {
		t.Fatalf("expected calculation to succeed after reviewing boundary, err=%q", a.assessmentError)
	}
}

func TestWorkbenchBoundaryModalSnapshots(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")

	// Empty 状态快照（无参评统计组）
	aEmpty := newTestApp(t)
	aEmpty.page = pageAssessments
	aEmpty.boundaryState().open = true
	ttEmpty := ui.NewTester(aEmpty.view, 1280, 820)
	ttEmpty.SetLocale("zh-Hans")
	captureDualSnapshot(t, ttEmpty, "modal_boundary_overrides_empty")

	// Populated 状态快照（有参评学科与已微调数据）
	aPop := newPopulatedTestApp(t)
	aPop.page = pageAssessments
	ctx := context.Background()
	if err := aPop.store.SaveAssessmentBoundaryOverrides(ctx, aPop.selectedAssessmentID, []store.AssessmentBoundaryOverride{
		{Subject: "chinese", GroupCount: 1, FirstRank: 1, SecondRank: 1, Reason: "前两名分差接近"},
	}); err != nil {
		t.Fatal(err)
	}
	aPop.reloadAssessments(ctx)
	aPop.openBoundaryModal()

	ttPop := ui.NewTester(aPop.view, 1280, 820)
	ttPop.SetLocale("zh-Hans")
	captureDualSnapshot(t, ttPop, "modal_boundary_overrides_populated")

	// 额外输出代表性全窗口浅/深色截图到 conversation brain 目录供验收查阅
	brainDir := "/Users/sivon/.gemini/antigravity/brain/baaf948b-3907-4337-94fb-f7280b665ad6"
	ttPop.SetDark(false)
	settleTester(ttPop)
	savePNG(t, filepath.Join(brainDir, "boundary_modal_light.png"), ttPop.Image())

	ttPop.SetDark(true)
	settleTester(ttPop)
	savePNG(t, filepath.Join(brainDir, "boundary_modal_dark.png"), ttPop.Image())
}
