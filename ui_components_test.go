package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo/ui"

	"tally/internal/fonts"
	"tally/locales"
)

func componentFixtureView(c *ui.Context) {
	p := uiPaletteFor(c)
	text := "Ada"
	password := "secret"
	number := float64(3)
	choice := "One"
	selected := 0
	ui.Column(c).Padding(p.theme.Space(6)).Gap(p.theme.Space(3)).Children(func() {
		textField(c, &text, "Name", "")
		passwordField(c, &password, "Password", "")
		numberField(c, &number, 0, 10, 1, "Number")
		selectField(c, "component-choice", &choice, []string{"One", "Two"}, "Choice")
		segmentedField(c, &selected, []string{"One", "Two"}, "Mode")
		labelledButton(c, "Continue", nil, false)
	})
}

func TestDesignTokens(t *testing.T) {
	light, dark := sunbeam.Light(), sunbeam.Dark()
	if light.Background != ui.Hex("#faf8f5") || dark.Background != ui.Hex("#090706") {
		t.Errorf("unexpected sunbeam backgrounds: light=%v dark=%v", light.Background, dark.Background)
	}
	if light.Primary != ui.Hex("#ffcc00") || light.PrimaryForeground != ui.Hex("#14110f") {
		t.Errorf("unexpected sunbeam primary: %v / %v", light.Primary, light.PrimaryForeground)
	}

	base := ui.Theme{FontSize: sunbeam.TypeBodySM.Size}
	if controlHeight(base) != 40 || controlHeightLG(base) != 40 || tableRowHeight(base) != 40 {
		t.Errorf("heights = %v/%v/%v, want 40/40/40",
			controlHeight(base), controlHeightLG(base), tableRowHeight(base))
	}
	if bodySize(base) != 14 || captionSize(base) != 13 || sectionSize(base) != 18 || titleSize(base) != 24 {
		t.Errorf("font scale = %v/%v/%v/%v, want 14/13/18/24",
			bodySize(base), captionSize(base), sectionSize(base), titleSize(base))
	}
	scaled := ui.Theme{FontSize: 21}
	if bodySize(scaled) != 21 || captionSize(scaled) != 19.5 || sectionSize(scaled) != 27 || titleSize(scaled) != 36 {
		t.Errorf("scaled font sizes = %v/%v/%v/%v, want 21/19.5/27/36",
			bodySize(scaled), captionSize(scaled), sectionSize(scaled), titleSize(scaled))
	}

	var themeFont string
	tt := ui.NewTester(func(c *ui.Context) {
		p := uiPaletteFor(c)
		themeFont = p.theme.Font
		ui.Text(c, "Sunbeam")
	}, 200, 80)
	tt.Frame()
	if themeFont != fonts.UIFontFamily {
		t.Errorf("theme font = %q, want %q", themeFont, fonts.UIFontFamily)
	}
}

func TestComponentFixtureAtStandardSizes(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{
		{name: "components_1280", w: 1280, h: 800},
		{name: "components_1024", w: 1024, h: 640},
	} {
		tt := ui.NewTester(componentFixtureView, size.w, size.h)
		tt.SetLocale("en")
		settleTester(tt)
		if tt.FramePending() {
			t.Fatalf("%s: view did not settle", size.name)
		}
		for _, name := range []string{"Name", "Password", "Number", "Choice"} {
			info, ok := tt.Access(name)
			if !ok {
				t.Fatalf("%s: missing accessible control %q", size.name, name)
			}
			if info.Bounds.H != 40 {
				t.Errorf("%s: %s height = %v, want 40", size.name, name, info.Bounds.H)
			}
		}
		button, ok := tt.AccessByRole(ui.RoleButton, "Continue")
		if !ok {
			t.Fatalf("%s: missing Continue button", size.name)
		}
		if button.Bounds.H != 40 {
			t.Errorf("%s: Continue height = %v, want 40", size.name, button.Bounds.H)
		}
		captureDualSnapshot(t, tt, size.name)
	}
}

func TestModalUsesStandardControls(t *testing.T) {
	open := true
	view := func(c *ui.Context) {
		p := uiPaletteFor(c)
		ui.Column(c).Padding(p.theme.Space(6)).Children(func() {
			confirmed := modal(c, &open, modalProps{
				Title:      "Details",
				OkText:     "Apply",
				CancelText: "Dismiss",
			}, func() {
				ui.Text(c, "Body").FontSize(p.theme.FontSize)
			})
			if confirmed {
				open = false
			}
		})
	}

	tt := ui.NewTester(view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)
	wantHeights := map[string]float32{
		"Close":   32,
		"Dismiss": 40,
		"Apply":   40,
	}
	for name, wantH := range wantHeights {
		info, ok := tt.AccessByRole(ui.RoleButton, name)
		if !ok {
			t.Fatalf("missing modal button %q", name)
		}
		if info.Bounds.H != wantH {
			t.Errorf("%s height = %v, want %v", name, info.Bounds.H, wantH)
		}
	}
	if !tt.HasText("Details") || !tt.HasText("Body") {
		t.Fatalf("modal content missing: %q", tt.Texts())
	}
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if open || tt.HasText("Details") {
		t.Fatalf("modal did not close: open=%v texts=%q", open, tt.Texts())
	}
}

func TestThemeHoverAndActiveTokensDistinct(t *testing.T) {
	for _, tc := range []struct {
		name string
		sb   *sunbeam.Palette
	}{
		{"light", sunbeam.Light()},
		{"dark", sunbeam.Dark()},
	} {
		sb := tc.sb
		if sb.FlatFace == sb.FlatFaceHover || sb.FlatFaceHover == sb.FlatFacePressed || sb.FlatFace == sb.FlatFacePressed {
			t.Errorf("%s flat rest/hover/pressed must be distinct: %v / %v / %v",
				tc.name, sb.FlatFace, sb.FlatFaceHover, sb.FlatFacePressed)
		}
		if sb.RaisedFaceFrom == sb.RaisedFaceTo || sb.RaisedFaceTo == sb.RaisedFacePressed {
			t.Errorf("%s raised gradient/pressed must be distinct: %v / %v / %v",
				tc.name, sb.RaisedFaceFrom, sb.RaisedFaceTo, sb.RaisedFacePressed)
		}
		if sb.Primary == sb.PrimaryForeground {
			t.Errorf("%s primary and foreground must be distinct", tc.name)
		}
	}
}

func TestModalSymmetricExitAnimation(t *testing.T) {
	open := true
	view := func(c *ui.Context) {
		p := uiPaletteFor(c)
		ui.Column(c).Fill().Children(func() {
			modal(c, &open, modalProps{
				Key:        "test-exit",
				Title:      "Confirm Details",
				OkText:     "Apply",
				CancelText: "Cancel",
			}, func() {
				ui.Text(c, "Modal body").FontSize(p.theme.FontSize)
			})
		})
	}
	tt := ui.NewTester(view, 800, 600)
	tt.SetLocale("en")
	settleTester(tt)
	if tt.FramePending() {
		t.Fatal("modal enter did not settle")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if open {
		t.Fatal("expected open to become false immediately on Cancel")
	}
	// Mid-exit (40ms < 120ms): exit animation should still be running.
	tt.Advance(40 * time.Millisecond)
	if !tt.FramePending() {
		t.Fatal("expected modal exit animation to be active mid-exit")
	}
	// After full exit duration: modal should be gone and settled.
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if tt.HasText("Confirm Details") {
		t.Fatalf("modal text remained after exit: %q", tt.Texts())
	}
	if tt.FramePending() {
		t.Fatal("modal did not settle after exit")
	}
}

func TestConfirmDangerModalFocusesCancelAndConfirms(t *testing.T) {
	open := true
	confirmed := false
	view := func(c *ui.Context) {
		p := uiPaletteFor(c)
		ui.Column(c).Fill().Children(func() {
			if confirmDangerModal(c, p, "test-danger", &open, "Delete item?", "Cannot be undone.", "Cancel", "Delete item") {
				confirmed = true
			}
		})
	}
	tt := ui.NewTester(view, 800, 600)
	tt.SetLocale("en")
	settleTester(tt)

	// Pressing Enter should trigger the default-focused Cancel button, not Delete.
	tt.Key(0, ui.KeyEnter)
	settleTester(tt)
	if confirmed {
		t.Fatal("Enter triggered destructive confirm instead of Cancel")
	}
	if open {
		t.Fatal("Enter on default-focused Cancel did not close dialog")
	}

	// Reopen and explicitly click the destructive confirm button.
	open = true
	settleTester(tt)
	if err := tt.Click("Delete item"); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !confirmed || open {
		t.Fatalf("expected confirm=true open=false, got confirm=%v open=%v", confirmed, open)
	}
	if tt.FramePending() {
		t.Fatal("confirm danger modal did not settle after confirm")
	}
}

func TestDelayedBusySuppressesFastFlash(t *testing.T) {
	busy := true
	view := func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			if showDelayedBusy(c, "test-busy", busy) {
				ui.Text(c, "Working...")
			}
		})
	}
	tt := ui.NewTester(view, 400, 200)
	tt.SetLocale("en")
	tt.Frame()
	if tt.HasText("Working...") {
		t.Fatal("busy indicator flashed at t=0")
	}
	tt.Advance(100 * time.Millisecond)
	if tt.HasText("Working...") {
		t.Fatal("busy indicator flashed before 250ms threshold")
	}
	tt.Advance(200 * time.Millisecond)
	if !tt.HasText("Working...") {
		t.Fatalf("busy indicator did not appear after 300ms: %q", tt.Texts())
	}
	busy = false
	tt.Frame()
	if tt.HasText("Working...") {
		t.Fatal("busy indicator stayed visible after busy=false")
	}
}

func TestProgressBarRendersStaticallyWithoutMountAnimation(t *testing.T) {
	view := func(c *ui.Context) {
		p := uiPaletteFor(c)
		progressBar(c, p, 75, "Readiness")
	}
	tt := ui.NewTester(view, 400, 120)
	tt.SetLocale("en")
	tt.Frame()
	if tt.FramePending() {
		t.Fatal("static progressBar requested animation frames on mount")
	}
	info, ok := tt.AccessByRole(ui.RoleProgress, "Readiness")
	if !ok {
		t.Fatal("missing accessible progress bar")
	}
	if info.Value != "75" && !tt.HasText("Readiness") {
		t.Fatalf("unexpected progress bar info: %+v", info)
	}
}

func TestActionToastsOnSettingsAndMainActions(t *testing.T) {
	a := newTestApp(t)
	a.settingsPageID = "subjects"
	st := ui.NewTester(a.settingsView, 760, 620)
	st.SetLocale("en")
	settleTester(st)

	a.saveSchoolSettings()
	st.Frame()
	if !st.HasText(locales.SettingsSchoolSaved()) {
		t.Fatalf("missing school saved toast: %q", st.Texts())
	}

	a.backupNow()
	st.Frame()
	if !st.HasText(locales.SettingsBackupCreated()) {
		t.Fatalf("missing backup created toast: %q", st.Texts())
	}

	a.setAcademicYear(a.academicYearStart)
	st.Frame()
	if !st.HasText(locales.SettingsAcademicYearSaved()) {
		t.Fatalf("missing academic year toast: %q", st.Texts())
	}

	a.page = pageQuality
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)
	if err := tt.Click(locales.ExamRevalidate()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(locales.QualityRevalidatedToast()) {
		t.Fatalf("missing revalidate toast: %q", tt.Texts())
	}

	a.newCohortYear = float64(a.academicYearStart)
	a.createCohort()
	tt.Frame()
	wantCohort := fmt.Sprintf(locales.CohortCreatedToast(), a.cohortName(a.academicYearStart))
	if !tt.HasText(wantCohort) {
		t.Fatalf("missing cohort created toast %q in %q", wantCohort, tt.Texts())
	}

	a.page = pageAnalysis
	a.saveCurrentView(context.Background(), "Test View")
	tt.Frame()
	a.deleteSavedView(context.Background(), a.analysisSelectedViewID)
	tt.Frame()
	if !tt.HasText(locales.AnalysisViewDeleted()) {
		t.Fatalf("missing delete view toast: %q", tt.Texts())
	}
}

func TestStudentDetailPanelSlideTransitionSettles(t *testing.T) {
	a := newTestApp(t)
	addCohort(t, a, 2026)
	seedStageData(t, a)
	a.page = pageStudents
	tt := ui.NewTester(a.view, 1280, 800)
	tt.SetLocale("en")
	settleTester(tt)

	a.selectStudent(a.students[0].ID)
	tt.Frame()
	settleTester(tt)
	if tt.FramePending() {
		t.Fatal("student detail panel enter did not settle")
	}
	if !tt.HasText(locales.StudentDetailTitle()) {
		t.Fatalf("missing student detail title: %q", tt.Texts())
	}

	if err := tt.Click(locales.StudentCloseDetail()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.studentDetailOpen || tt.FramePending() {
		t.Fatalf("student detail panel did not close/settle: open=%v pending=%v", a.studentDetailOpen, tt.FramePending())
	}
}
