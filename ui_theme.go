package main

import "github.com/egoist/mygo/ui"

// uiPalette 收起界面用到的颜色和主题值。界面统一从这里取色，
// 浅色与深色各自一套，控件仍沿用 MyGo 的主题行为。
type uiPalette struct {
	theme       ui.Theme
	canvas      ui.Color
	panel       ui.Color
	panelMuted  ui.Color
	border      ui.Color
	ink         ui.Color
	muted       ui.Color
	accent      ui.Color
	accentText  ui.Color
	accentSoft  ui.Color
	danger      ui.Color
	dangerSoft  ui.Color
	warning     ui.Color
	warningSoft ui.Color
	success     ui.Color
	successSoft ui.Color
}

func uiPaletteFor(c *ui.Context) uiPalette {
	t := *c.Theme()
	p := uiPalette{theme: t}

	if t.Dark {
		t.Background = ui.Hex("#141715")
		t.Surface = ui.Hex("#1C211E")
		t.SurfaceHover = ui.Hex("#232A26")
		t.SurfacePressed = ui.Hex("#2A322D")
		t.Border = ui.Hex("#3A433D")
		t.Text = ui.Hex("#ECF0EB")
		t.TextMuted = ui.Hex("#A1ABA4")
		t.Accent = ui.Hex("#73C4A3")
		t.AccentHover = ui.Hex("#8AD3B5")
		t.AccentPressed = ui.Hex("#5BAF8F")
		t.AccentText = ui.Hex("#0F1C16")
		t.Danger = ui.Hex("#E48278")
		t.Warning = ui.Hex("#D9A45F")
		t.Success = ui.Hex("#7FC9A1")
		t.Selection = ui.Hex("#2A4638")
		t.Focus = ui.Hex("#73C4A3")
		t.Inverse = ui.Hex("#ECF0EB")
		t.InverseText = ui.Hex("#141715")
		t.Scrollbar = ui.Hex("#4A554E")
		p.panelMuted = ui.Hex("#202622")
		p.accentSoft = ui.RGBA(115, 196, 163, 0.16)
		p.dangerSoft = ui.RGBA(228, 130, 120, 0.16)
		p.warningSoft = ui.RGBA(217, 164, 95, 0.16)
		p.successSoft = ui.RGBA(127, 201, 161, 0.16)
	} else {
		t.Background = ui.Hex("#F4F1EA")
		t.Surface = ui.Hex("#FFFFFF")
		t.SurfaceHover = ui.Hex("#F7F4ED")
		t.SurfacePressed = ui.Hex("#EFEAE0")
		t.Border = ui.Hex("#DED7C8")
		t.Text = ui.Hex("#191C1A")
		t.TextMuted = ui.Hex("#626862")
		t.Accent = ui.Hex("#1F5C48")
		t.AccentHover = ui.Hex("#184A3A")
		t.AccentPressed = ui.Hex("#123A2D")
		t.AccentText = ui.Hex("#FFFFFF")
		t.Danger = ui.Hex("#A23B32")
		t.Warning = ui.Hex("#97601A")
		t.Success = ui.Hex("#2D6A4F")
		t.Selection = ui.Hex("#D8E9DF")
		t.Focus = ui.Hex("#1F5C48")
		t.Inverse = ui.Hex("#191C1A")
		t.InverseText = ui.Hex("#FAF8F3")
		t.Scrollbar = ui.Hex("#B9B1A2")
		p.panelMuted = ui.Hex("#F7F4ED")
		p.accentSoft = ui.RGBA(31, 92, 72, 0.10)
		p.dangerSoft = ui.RGBA(162, 59, 50, 0.10)
		p.warningSoft = ui.RGBA(151, 96, 26, 0.12)
		p.successSoft = ui.RGBA(45, 106, 79, 0.10)
	}

	t.Radius = 8
	t.Spacing = 4
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
