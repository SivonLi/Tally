package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// brand 是应用唯一的主题。背景使用暖纸色，强调色只有靛青一种，
// 成功、提醒和错误各自保留可辨认的颜色。
var brand = func() *theme.Config {
	light, dark := *theme.Light(), *theme.Dark()

	light.Primary = ui.Hex("#2B4C7E")
	light.PrimaryHover = ui.Hex("#223E68")
	light.PrimaryActive = ui.Hex("#1A3152")
	light.PrimaryForeground = ui.Hex("#FFFFFF")
	light.PrimaryText = light.Primary
	light.PrimaryTextHover = light.PrimaryHover
	light.PrimaryTextActive = light.PrimaryActive
	light.PrimaryBg = ui.Hex("#E4EBF5")
	light.PrimaryBgHover = ui.Hex("#D5DFF0")
	light.PrimaryBorder = ui.Hex("#C4B9A6")
	light.PrimaryBorderHover = ui.Hex("#9DB7DC")
	light.PrimaryFill = ui.RGBA(43, 76, 126, 0.36)
	light.PrimaryFillSecondary = ui.RGBA(43, 76, 126, 0.2)
	light.PrimaryFillTertiary = ui.RGBA(43, 76, 126, 0.1)
	light.PrimaryFillQuaternary = ui.RGBA(43, 76, 126, 0.04)
	light.Link = light.Primary
	light.LinkHover = light.PrimaryHover
	light.LinkActive = light.PrimaryActive
	light.Success = ui.Hex("#2E6B4F")
	light.SuccessHover = ui.Hex("#3D7D5E")
	light.SuccessActive = ui.Hex("#245840")
	light.SuccessText = light.Success
	light.SuccessTextHover = light.SuccessHover
	light.SuccessTextActive = light.SuccessActive
	light.SuccessBg = ui.Hex("#E3EEE7")
	light.SuccessBgHover = ui.Hex("#D3E3D8")
	light.SuccessBorder = ui.Hex("#C8E0D2")
	light.SuccessBorderHover = ui.Hex("#AFD3BE")
	light.Warning = ui.Hex("#7A5B12")
	light.WarningHover = ui.Hex("#8B6B1A")
	light.WarningActive = ui.Hex("#654A0E")
	light.WarningText = light.Warning
	light.WarningTextHover = light.WarningHover
	light.WarningTextActive = light.WarningActive
	light.WarningBg = ui.Hex("#F5EEDC")
	light.WarningBgHover = ui.Hex("#E7DFC8")
	light.WarningBorder = ui.Hex("#E7D8B4")
	light.WarningBorderHover = ui.Hex("#DAC798")
	light.Error = ui.Hex("#9D2933")
	light.ErrorHover = ui.Hex("#AF3B43")
	light.ErrorActive = ui.Hex("#86222B")
	light.ErrorText = light.Error
	light.ErrorTextHover = light.ErrorHover
	light.ErrorTextActive = light.ErrorActive
	light.ErrorBg = ui.Hex("#F4E3E4")
	light.ErrorBgHover = ui.Hex("#E7CFCF")
	light.ErrorBorder = ui.Hex("#E9C8C2")
	light.ErrorBorderHover = ui.Hex("#DBB0A8")
	light.Info = ui.Hex("#5F5F6A")
	light.InfoHover = ui.Hex("#71717D")
	light.InfoActive = ui.Hex("#4C4C56")
	light.InfoBg = ui.Hex("#F4F4F6")
	light.InfoBorder = ui.Hex("#E4E4E9")

	light.BgBase = ui.Hex("#F4F1E8")
	light.BgLayout = ui.Hex("#F4F1E8")
	light.BgContainer = ui.Hex("#FFFDF8")
	light.BgContainerSecondary = ui.Hex("#ECE7DB")
	light.BgElevated = ui.Hex("#FFFDF8")
	light.BgSpotlight = ui.Hex("#F7F3E9")
	light.Border = ui.Hex("#DCD4C4")
	light.BorderSecondary = ui.Hex("#E8E1D4")
	light.BorderDisabled = ui.Hex("#DCD4C4")
	light.Fill = ui.RGBA(30, 28, 24, 0.12)
	light.FillSecondary = ui.RGBA(30, 28, 24, 0.06)
	light.FillTertiary = ui.RGBA(30, 28, 24, 0.035)
	light.FillQuaternary = ui.RGBA(30, 28, 24, 0.02)
	light.Text = ui.Hex("#1E1C18")
	light.TextHeading = ui.Hex("#1E1C18")
	light.TextSecondary = ui.Hex("#4A463E")
	light.TextTertiary = ui.Hex("#6B665C")
	light.TextQuaternary = ui.Hex("#6B665C")
	light.TextDescription = ui.Hex("#6B665C")
	light.TextLabel = ui.Hex("#4A463E")
	light.TextPlaceholder = ui.Hex("#6B665C")
	light.TextDisabled = ui.Hex("#6B665C")
	light.Icon = ui.Hex("#6B665C")
	light.IconHover = ui.Hex("#1E1C18")
	light.Split = ui.RGBA(30, 28, 24, 0.08)
	light.Highlight = light.Primary

	dark.Primary = ui.Hex("#9DB7DC")
	dark.PrimaryHover = ui.Hex("#B3C9E6")
	dark.PrimaryActive = ui.Hex("#84A2CB")
	dark.PrimaryForeground = ui.Hex("#14120F")
	dark.PrimaryText = ui.Hex("#9DB7DC")
	dark.PrimaryTextHover = ui.Hex("#B3C9E6")
	dark.PrimaryTextActive = ui.Hex("#84A2CB")
	dark.PrimaryBg = ui.Hex("#293447")
	dark.PrimaryBgHover = ui.Hex("#314059")
	dark.PrimaryBorder = ui.Hex("#4E483E")
	dark.PrimaryBorderHover = ui.Hex("#9DB7DC")
	dark.PrimaryFill = ui.RGBA(157, 183, 220, 0.36)
	dark.PrimaryFillSecondary = ui.RGBA(157, 183, 220, 0.22)
	dark.PrimaryFillTertiary = ui.RGBA(157, 183, 220, 0.12)
	dark.PrimaryFillQuaternary = ui.RGBA(157, 183, 220, 0.05)
	dark.Link = dark.PrimaryText
	dark.LinkHover = dark.PrimaryTextHover
	dark.LinkActive = dark.PrimaryTextActive
	dark.Success = ui.Hex("#8FC3A4")
	dark.SuccessHover = ui.Hex("#A5D2B6")
	dark.SuccessActive = ui.Hex("#78B292")
	dark.SuccessText = dark.Success
	dark.SuccessTextHover = dark.SuccessHover
	dark.SuccessTextActive = dark.SuccessActive
	dark.SuccessBg = ui.Hex("#1F3328")
	dark.SuccessBgHover = ui.Hex("#254232")
	dark.SuccessBorder = ui.Hex("#1F4A30")
	dark.SuccessBorderHover = ui.Hex("#2A6240")
	dark.Warning = ui.Hex("#D8B466")
	dark.WarningHover = ui.Hex("#E3C378")
	dark.WarningActive = ui.Hex("#C3A458")
	dark.WarningText = dark.Warning
	dark.WarningTextHover = dark.WarningHover
	dark.WarningTextActive = dark.WarningActive
	dark.WarningBg = ui.Hex("#362E1C")
	dark.WarningBgHover = ui.Hex("#43392B")
	dark.WarningBorder = ui.Hex("#4A3B1E")
	dark.WarningBorderHover = ui.Hex("#5C4A28")
	dark.Error = ui.Hex("#E39A90")
	dark.ErrorHover = ui.Hex("#EBAEA6")
	dark.ErrorActive = ui.Hex("#CF8379")
	dark.ErrorText = dark.Error
	dark.ErrorTextHover = dark.ErrorHover
	dark.ErrorTextActive = dark.ErrorActive
	dark.ErrorBg = ui.Hex("#3A2422")
	dark.ErrorBgHover = ui.Hex("#4A302D")
	dark.ErrorBorder = ui.Hex("#4C2A26")
	dark.ErrorBorderHover = ui.Hex("#5E3833")
	dark.Info = ui.Hex("#9C9CA5")
	dark.InfoHover = ui.Hex("#B2B2BA")
	dark.InfoActive = ui.Hex("#85858E")
	dark.InfoBg = ui.Hex("#1A1A1E")
	dark.InfoBorder = ui.Hex("#2A2A30")

	dark.BgBase = ui.Hex("#171512")
	dark.BgLayout = ui.Hex("#171512")
	dark.BgContainer = ui.Hex("#1E1C18")
	dark.BgContainerSecondary = ui.Hex("#26231E")
	dark.BgElevated = ui.Hex("#1E1C18")
	dark.BgSpotlight = ui.Hex("#2B2822")
	dark.Border = ui.Hex("#3A362E")
	dark.BorderSecondary = ui.Hex("#332F28")
	dark.BorderDisabled = ui.Hex("#3A362E")
	dark.Fill = ui.RGBA(237, 233, 224, 0.14)
	dark.FillSecondary = ui.RGBA(237, 233, 224, 0.08)
	dark.FillTertiary = ui.RGBA(237, 233, 224, 0.05)
	dark.FillQuaternary = ui.RGBA(237, 233, 224, 0.03)
	dark.Text = ui.Hex("#EDE9E0")
	dark.TextHeading = ui.Hex("#EDE9E0")
	dark.TextSecondary = ui.Hex("#C7C1B5")
	dark.TextTertiary = ui.Hex("#A19A8D")
	dark.TextQuaternary = ui.Hex("#A19A8D")
	dark.TextDescription = ui.Hex("#A19A8D")
	dark.TextLabel = ui.Hex("#C7C1B5")
	dark.TextPlaceholder = ui.Hex("#A19A8D")
	dark.TextDisabled = ui.Hex("#A19A8D")
	dark.Icon = ui.Hex("#A19A8D")
	dark.IconHover = ui.Hex("#EDE9E0")
	dark.Split = ui.RGBA(237, 233, 224, 0.08)
	dark.Highlight = dark.Primary

	return theme.NewConfig(light, dark)
}()

// uiPalette 是界面代码使用的颜色与间距，控件本身跟随 Lobe MyGo 的主题。
type uiPalette struct {
	theme        ui.Theme
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
	brand.Apply(c)
	t := *c.Theme()
	if t.Dark {
		return uiPalette{
			theme:        t,
			canvas:       ui.Hex("#171512"),
			panel:        ui.Hex("#1E1C18"),
			panelMuted:   ui.Hex("#26231E"),
			panelHover:   ui.Hex("#2B2822"),
			fill:         ui.Hex("#2B2822"),
			hover:        ui.Hex("#2B2822"),
			border:       ui.Hex("#3A362E"),
			borderStrong: ui.Hex("#4E483E"),
			ink:          ui.Hex("#EDE9E0"),
			inkSecondary: ui.Hex("#C7C1B5"),
			muted:        ui.Hex("#A19A8D"),
			accent:       ui.Hex("#9DB7DC"),
			accentText:   ui.Hex("#14120F"),
			accentSoft:   ui.Hex("#293447"),
			danger:       ui.Hex("#E39A90"),
			dangerSoft:   ui.Hex("#3A2422"),
			warning:      ui.Hex("#D8B466"),
			warningSoft:  ui.Hex("#362E1C"),
			success:      ui.Hex("#8FC3A4"),
			successSoft:  ui.Hex("#1F3328"),
			info:         ui.Hex("#8FB8CC"),
			infoSoft:     ui.Hex("#1E3038"),
		}
	}

	return uiPalette{
		theme:        t,
		canvas:       ui.Hex("#F4F1E8"),
		panel:        ui.Hex("#FFFDF8"),
		panelMuted:   ui.Hex("#ECE7DB"),
		panelHover:   ui.Hex("#F7F3E9"),
		fill:         ui.Hex("#F7F3E9"),
		hover:        ui.Hex("#F7F3E9"),
		border:       ui.Hex("#DCD4C4"),
		borderStrong: ui.Hex("#C4B9A6"),
		ink:          ui.Hex("#1E1C18"),
		inkSecondary: ui.Hex("#4A463E"),
		muted:        ui.Hex("#6B665C"),
		accent:       ui.Hex("#2B4C7E"),
		accentText:   ui.Hex("#FFFFFF"),
		accentSoft:   ui.Hex("#E4EBF5"),
		danger:       ui.Hex("#9D2933"),
		dangerSoft:   ui.Hex("#F4E3E4"),
		warning:      ui.Hex("#7A5B12"),
		warningSoft:  ui.Hex("#F5EEDC"),
		success:      ui.Hex("#2E6B4F"),
		successSoft:  ui.Hex("#E3EEE7"),
		info:         ui.Hex("#2A6079"),
		infoSoft:     ui.Hex("#E2EEF3"),
	}
}

func titleSize(t ui.Theme) float32 {
	return 24
}

func sectionSize(t ui.Theme) float32 {
	return 16
}

func bodySize(t ui.Theme) float32 {
	return 13
}

func captionSize(t ui.Theme) float32 {
	return 12
}
