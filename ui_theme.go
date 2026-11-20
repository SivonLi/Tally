package main

import "github.com/egoist/mygo/ui"

// uiPalette 收起界面用到的颜色和主题值。界面统一从这里取色，
// 浅色与深色各自一套，控件仍沿用 MyGo 的主题行为。
type uiPalette struct {
	theme        ui.Theme
	canvas       ui.Color
	panel        ui.Color
	panelMuted   ui.Color
	panelHover   ui.Color
	border       ui.Color
	borderStrong ui.Color
	ink          ui.Color
	inkSecondary ui.Color
	muted        ui.Color
	accent       ui.Color
	accentText   ui.Color
	accentSoft   ui.Color
	danger       ui.Color
	dangerSoft   ui.Color
	warning      ui.Color
	warningSoft  ui.Color
	success      ui.Color
	successSoft  ui.Color
	info         ui.Color
	infoSoft     ui.Color
}

func uiPaletteFor(c *ui.Context) uiPalette {
	t := *c.Theme()
	p := uiPalette{theme: t}

	if t.Dark {
		t.Background = ui.Hex("#171512")
		t.Surface = ui.Hex("#1E1C18")
		t.SurfaceHover = ui.Hex("#2B2822")
		t.SurfacePressed = ui.Hex("#332F28")
		t.Border = ui.Hex("#3A362E")
		t.Text = ui.Hex("#EDE9E0")
		t.TextMuted = ui.Hex("#A19A8D")
		t.Accent = ui.Hex("#9DB7DC")
		t.AccentHover = ui.Hex("#B3C9E6")
		t.AccentPressed = ui.Hex("#84A2CB")
		t.AccentText = ui.Hex("#14120F")
		t.Danger = ui.Hex("#E39A90")
		t.Warning = ui.Hex("#D8B466")
		t.Success = ui.Hex("#8FC3A4")
		t.Selection = ui.Hex("#2C4160")
		t.Focus = ui.Hex("#9DB7DC")
		t.Inverse = ui.Hex("#EDE9E0")
		t.InverseText = ui.Hex("#171512")
		t.Scrollbar = ui.Hex("#4A453C")
		p.panelMuted = ui.Hex("#26231E")
		p.panelHover = ui.Hex("#2B2822")
		p.borderStrong = ui.Hex("#4E483E")
		p.inkSecondary = ui.Hex("#C7C1B5")
		p.accentSoft = ui.Hex("#293447")
		p.dangerSoft = ui.Hex("#3A2422")
		p.warningSoft = ui.Hex("#362E1C")
		p.successSoft = ui.Hex("#1F3328")
		p.info = ui.Hex("#8FB8CC")
		p.infoSoft = ui.Hex("#1E3038")
	} else {
		t.Background = ui.Hex("#F4F1E8")
		t.Surface = ui.Hex("#FFFDF8")
		t.SurfaceHover = ui.Hex("#F7F3E9")
		t.SurfacePressed = ui.Hex("#E8E2D4")
		t.Border = ui.Hex("#DCD4C4")
		t.Text = ui.Hex("#1E1C18")
		t.TextMuted = ui.Hex("#6B665C")
		t.Accent = ui.Hex("#2B4C7E")
		t.AccentHover = ui.Hex("#223E68")
		t.AccentPressed = ui.Hex("#1A3152")
		t.AccentText = ui.Hex("#FFFFFF")
		t.Danger = ui.Hex("#9D2933")
		t.Warning = ui.Hex("#7A5B12")
		t.Success = ui.Hex("#2E6B4F")
		t.Selection = ui.Hex("#D9E3F2")
		t.Focus = ui.Hex("#2B4C7E")
		t.Inverse = ui.Hex("#1E1C18")
		t.InverseText = ui.Hex("#FFFDF8")
		t.Scrollbar = ui.Hex("#B8AF9E")
		p.panelMuted = ui.Hex("#ECE7DB")
		p.panelHover = ui.Hex("#F7F3E9")
		p.borderStrong = ui.Hex("#C4B9A6")
		p.inkSecondary = ui.Hex("#4A463E")
		p.accentSoft = ui.Hex("#E4EBF5")
		p.dangerSoft = ui.Hex("#F4E3E4")
		p.warningSoft = ui.Hex("#F5EEDC")
		p.successSoft = ui.Hex("#E3EEE7")
		p.info = ui.Hex("#2A6079")
		p.infoSoft = ui.Hex("#E2EEF3")
	}

	t.FontSize = 13
	t.Spacing = 4
	t.Radius = 6
	t.FocusRingWidth = 2
	t.FocusRingOffset = 2
	t.ScrollbarWidth = 8
	c.SetTheme(&t)

	p.theme = t
	p.canvas = t.Background
	p.panel = t.Surface
	p.border = t.Border
	p.ink = t.Text
	p.muted = t.TextMuted
	p.accent = t.Accent
	p.accentText = t.AccentText
	p.danger = t.Danger
	p.warning = t.Warning
	p.success = t.Success
	return p
}

func titleSize(t ui.Theme) float32 {
	return t.Rem(24.0 / 13.0)
}

func sectionSize(t ui.Theme) float32 {
	return t.Rem(16.0 / 13.0)
}

func bodySize(t ui.Theme) float32 {
	return t.Rem(1)
}

func captionSize(t ui.Theme) float32 {
	return t.Rem(12.0 / 13.0)
}
