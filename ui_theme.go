package main

import (
	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo/ui"

	"tally/internal/fonts"
)

func init() {
	fonts.Register()
}

// applySunbeamTheme 为当前上下文注入 Sunbeam 主题与自定义字体族（Allo + 霞鹜文楷屏幕阅读版）。
// 所有颜色与语义均来自 Sunbeam token，组件内不写物理色值或 dark 分支。
func applySunbeamTheme(c *ui.Context) {
	fonts.Register()
	isDark := c.Theme().Dark
	t := sunbeam.Theme(isDark)
	p := sunbeam.Light()
	if isDark {
		p = sunbeam.Dark()
	}
	t.Font = fonts.UIFontFamily
	t.FontSize = sunbeam.TypeBodySM.Size
	t.Spacing = sunbeam.SpaceUnit
	t.Radius = sunbeam.RadiusLG
	t.Warning = p.AccentForeground
	t.Scrollbar = p.Border
	t.FocusRingWidth = 2
	t.FocusRingOffset = 2
	c.SetTheme(t)
}

// uiPalette 是界面代码使用的 Sunbeam 语义调色板。
// 浅色与深色完全由 Sunbeam token 层承载，组件本身不包含深浅分支。
type uiPalette struct {
	theme        ui.Theme
	sb           *sunbeam.Palette
	canvas       ui.Color
	panel        ui.Color
	panelMuted   ui.Color
	panelHover   ui.Color
	fill         ui.Color
	hover        ui.Color
	border       ui.Color
	borderStrong ui.Color
	ink          ui.Color
	inkSecondary ui.Color
	muted        ui.Color
	faint        ui.Color
	accent       ui.Color
	accentText   ui.Color
	accentSoft   ui.Color
	accentInk    ui.Color
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
	applySunbeamTheme(c)
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	return uiPalette{
		theme:        t,
		sb:           sb,
		canvas:       sb.Background,
		panel:        sb.Card,
		panelMuted:   sb.Canvas,
		panelHover:   sb.FlatFaceHover,
		fill:         sb.Secondary,
		hover:        sb.FlatFaceHover,
		border:       sb.Border,
		borderStrong: sb.Input,
		ink:          sb.Foreground,
		inkSecondary: sb.SecondaryForeground,
		muted:        sb.MutedForeground,
		faint:        sb.Faint,
		accent:       sb.Primary,
		accentText:   sb.PrimaryForeground,
		accentSoft:   sb.Accent,
		accentInk:    sb.AccentForeground,
		danger:       sb.Danger,
		dangerSoft:   sb.Danger.Alpha(0.10),
		warning:      sb.AccentForeground,
		warningSoft:  sb.Accent,
		success:      sb.Success,
		successSoft:  sb.Success.Alpha(0.10),
		info:         sb.Info,
		infoSoft:     sb.Info.Alpha(0.10),
	}
}

// textScale 返回相对 Sunbeam body-sm (14 DIP) 基准字号的缩放比例，跟随系统文字缩放。
func textScale(t ui.Theme) float32 {
	if t.FontSize <= 0 {
		return 1
	}
	return t.FontSize / sunbeam.TypeBodySM.Size
}

func titleSize(t ui.Theme) float32 {
	return sunbeam.TypeHeadlineMD.Size * textScale(t)
}

func sectionSize(t ui.Theme) float32 {
	return sunbeam.TypeBodyLG.Size * textScale(t)
}

func bodySize(t ui.Theme) float32 {
	if t.FontSize <= 0 {
		return sunbeam.TypeBodySM.Size
	}
	return t.FontSize
}

func captionSize(t ui.Theme) float32 {
	return sunbeam.TypeCaption.Size * textScale(t)
}
