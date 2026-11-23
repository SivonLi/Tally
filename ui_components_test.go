package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
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
	for _, dark := range []bool{false, true} {
		tokens := brand.Tokens(dark)
		if tokens.ControlHeight != 36 || tokens.ControlHeightSM != 28 || tokens.ControlHeightLG != 44 {
			t.Errorf("dark=%v control heights = %v/%v/%v, want 36/28/44",
				dark, tokens.ControlHeight, tokens.ControlHeightSM, tokens.ControlHeightLG)
		}
		if tokens.FontSize != 16 || tokens.FontSizeSM != 14 || tokens.FontSizeLG != 18 || tokens.FontSizeXL != 28 {
			t.Errorf("dark=%v font sizes = %v/%v/%v/%v, want 16/14/18/28",
				dark, tokens.FontSize, tokens.FontSizeSM, tokens.FontSizeLG, tokens.FontSizeXL)
		}
	}

	base := ui.Theme{FontSize: 16}
	if bodySize(base) != 16 || captionSize(base) != 14 || sectionSize(base) != 18 || titleSize(base) != 28 {
		t.Errorf("font scale = %v/%v/%v/%v, want 16/14/18/28",
			bodySize(base), captionSize(base), sectionSize(base), titleSize(base))
	}
	scaled := ui.Theme{FontSize: 24}
	if bodySize(scaled) != 24 || captionSize(scaled) != 21 || sectionSize(scaled) != 27 || titleSize(scaled) != 42 {
		t.Errorf("scaled font sizes = %v/%v/%v/%v, want 24/21/27/42",
			bodySize(scaled), captionSize(scaled), sectionSize(scaled), titleSize(scaled))
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
			if info.Bounds.H != 36 {
				t.Errorf("%s: %s height = %v, want 36", size.name, name, info.Bounds.H)
			}
		}
		button, ok := tt.AccessByRole(ui.RoleButton, "Continue")
		if !ok {
			t.Fatalf("%s: missing Continue button", size.name)
		}
		if button.Bounds.H != 36 {
			t.Errorf("%s: Continue height = %v, want 36", size.name, button.Bounds.H)
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
	for _, name := range []string{"Close", "Dismiss", "Apply"} {
		info, ok := tt.AccessByRole(ui.RoleButton, name)
		if !ok {
			t.Fatalf("missing modal button %q", name)
		}
		if info.Bounds.H != 36 {
			t.Errorf("%s height = %v, want 36", name, info.Bounds.H)
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
