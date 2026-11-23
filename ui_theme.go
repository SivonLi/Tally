package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// brand 是应用唯一的主题。
// 遵循 Tally 调色板体系，强调色为深蓝/冰蓝，状态色保留清晰辨识度。
var brand = func() *theme.Config {
	light, dark := *theme.Light(), *theme.Dark()

	light.ControlHeight = 36
	light.ControlHeightSM = 28
	light.ControlHeightLG = 44
	light.FontSize = 16
	light.FontSizeSM = 14
	light.FontSizeLG = 18
	light.FontSizeXL = 28

	light.Primary = ui.Hex("#2672AF")
	light.PrimaryHover = light.Primary
	light.PrimaryActive = light.Primary
	light.PrimaryForeground = ui.Hex("#FFFFFF")
	light.PrimaryText = light.Primary
	light.PrimaryTextHover = light.Primary
	light.PrimaryTextActive = light.Primary
	light.PrimaryBg = ui.Hex("#E4ECF2")
	light.PrimaryBgHover = ui.Hex("#D8E4EE")
	light.PrimaryBorder = ui.Hex("#828C99")
	light.PrimaryBorderHover = light.Primary
	light.PrimaryFill = ui.RGBA(38, 114, 175, 0.36)
	light.PrimaryFillSecondary = ui.RGBA(38, 114, 175, 0.2)
	light.PrimaryFillTertiary = ui.RGBA(38, 114, 175, 0.1)
	light.PrimaryFillQuaternary = ui.RGBA(38, 114, 175, 0.04)
	light.Link = light.Primary
	light.LinkHover = light.Primary
	light.LinkActive = light.Primary
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
	light.Warning = ui.Hex("#FE9A00")
	light.WarningHover = light.Warning
	light.WarningActive = light.Warning
	light.WarningText = light.Warning
	light.WarningTextHover = light.Warning
	light.WarningTextActive = light.Warning
	light.WarningBg = ui.Hex("#F5EEDC")
	light.WarningBgHover = ui.Hex("#E7DFC8")
	light.WarningBorder = ui.Hex("#E7D8B4")
	light.WarningBorderHover = ui.Hex("#DAC798")
	light.Error = ui.Hex("#FB2C36")
	light.ErrorHover = light.Error
	light.ErrorActive = light.Error
	light.ErrorText = light.Error
	light.ErrorTextHover = light.Error
	light.ErrorTextActive = light.Error
	light.ErrorBg = ui.Hex("#F4E3E4")
	light.ErrorBgHover = ui.Hex("#E7CFCF")
	light.ErrorBorder = ui.Hex("#E9C8C2")
	light.ErrorBorderHover = ui.Hex("#DBB0A8")
	light.Info = ui.Hex("#5F5F6A")
	light.InfoHover = ui.Hex("#71717D")
	light.InfoActive = ui.Hex("#4C4C56")
	light.InfoBg = ui.Hex("#F4F4F6")
	light.InfoBorder = ui.Hex("#E4E4E9")

	light.BgBase = ui.Hex("#F5F7F8")
	light.BgLayout = ui.Hex("#F5F7F8")
	light.BgContainer = ui.Hex("#EDEFF1")
	light.BgContainerSecondary = ui.Hex("#E4ECF2")
	light.BgElevated = ui.Hex("#EDEFF1")
	light.BgSpotlight = ui.Hex("#E8E9EB")
	light.Border = ui.Hex("#828C99")
	light.BorderSecondary = ui.Hex("#828C99")
	light.BorderDisabled = ui.Hex("#828C99")
	light.Fill = ui.RGBA(36, 21, 35, 0.12)
	light.FillSecondary = ui.RGBA(36, 21, 35, 0.06)
	light.FillTertiary = ui.RGBA(36, 21, 35, 0.035)
	light.FillQuaternary = ui.RGBA(36, 21, 35, 0.02)
	light.Text = ui.Hex("#241523")
	light.TextHeading = ui.Hex("#241523")
	light.TextSecondary = ui.Hex("#6F6873")
	light.TextTertiary = ui.Hex("#6F6873")
	light.TextQuaternary = ui.Hex("#6F6873")
	light.TextDescription = ui.Hex("#6F6873")
	light.TextLabel = ui.Hex("#6F6873")
	light.TextPlaceholder = ui.Hex("#6F6873")
	light.TextDisabled = ui.Hex("#6F6873")
	light.Icon = ui.Hex("#6F6873")
	light.IconHover = ui.Hex("#241523")
	light.Split = ui.RGBA(36, 21, 35, 0.08)
	light.Highlight = light.Primary

	dark.ControlHeight = 36
	dark.ControlHeightSM = 28
	dark.ControlHeightLG = 44
	dark.FontSize = 16
	dark.FontSizeSM = 14
	dark.FontSizeLG = 18
	dark.FontSizeXL = 28

	dark.Primary = ui.Hex("#70B9EE")
	dark.PrimaryHover = dark.Primary
	dark.PrimaryActive = dark.Primary
	dark.PrimaryForeground = ui.Hex("#17212B")
	dark.PrimaryText = dark.Primary
	dark.PrimaryTextHover = dark.Primary
	dark.PrimaryTextActive = dark.Primary
	dark.PrimaryBg = ui.Hex("#293F52")
	dark.PrimaryBgHover = ui.Hex("#324E66")
	dark.PrimaryBorder = ui.Hex("#405567")
	dark.PrimaryBorderHover = dark.Primary
	dark.PrimaryFill = ui.RGBA(112, 185, 238, 0.36)
	dark.PrimaryFillSecondary = ui.RGBA(112, 185, 238, 0.22)
	dark.PrimaryFillTertiary = ui.RGBA(112, 185, 238, 0.12)
	dark.PrimaryFillQuaternary = ui.RGBA(112, 185, 238, 0.05)
	dark.Link = dark.PrimaryText
	dark.LinkHover = dark.PrimaryText
	dark.LinkActive = dark.PrimaryText
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
	dark.Warning = ui.Hex("#FE9A00")
	dark.WarningHover = dark.Warning
	dark.WarningActive = dark.Warning
	dark.WarningText = dark.Warning
	dark.WarningTextHover = dark.Warning
	dark.WarningTextActive = dark.Warning
	dark.WarningBg = ui.Hex("#362E1C")
	dark.WarningBgHover = ui.Hex("#43392B")
	dark.WarningBorder = ui.Hex("#4A3B1E")
	dark.WarningBorderHover = ui.Hex("#5C4A28")
	dark.Error = ui.Hex("#FB414A")
	dark.ErrorHover = dark.Error
	dark.ErrorActive = dark.Error
	dark.ErrorText = dark.Error
	dark.ErrorTextHover = dark.Error
	dark.ErrorTextActive = dark.Error
	dark.ErrorBg = ui.Hex("#3A2422")
	dark.ErrorBgHover = ui.Hex("#4A302D")
	dark.ErrorBorder = ui.Hex("#4C2A26")
	dark.ErrorBorderHover = ui.Hex("#5E3833")
	dark.Info = ui.Hex("#9C9CA5")
	dark.InfoHover = ui.Hex("#B2B2BA")
	dark.InfoActive = ui.Hex("#85858E")
	dark.InfoBg = ui.Hex("#1A1A1E")
	dark.InfoBorder = ui.Hex("#2A2A30")

	dark.BgBase = ui.Hex("#17212B")
	dark.BgLayout = ui.Hex("#17212B")
	dark.BgContainer = ui.Hex("#333B45")
	dark.BgContainerSecondary = ui.Hex("#1E2D3B")
	dark.BgElevated = ui.Hex("#333B45")
	dark.BgSpotlight = ui.Hex("#414851")
	dark.Border = ui.Hex("#405567")
	dark.BorderSecondary = ui.Hex("#405567")
	dark.BorderDisabled = ui.Hex("#405567")
	dark.Fill = ui.RGBA(255, 250, 255, 0.14)
	dark.FillSecondary = ui.RGBA(255, 250, 255, 0.08)
	dark.FillTertiary = ui.RGBA(255, 250, 255, 0.05)
	dark.FillQuaternary = ui.RGBA(255, 250, 255, 0.03)
	dark.Text = ui.Hex("#FFFAFF")
	dark.TextHeading = ui.Hex("#FFFAFF")
	dark.TextSecondary = ui.Hex("#969CA6")
	dark.TextTertiary = ui.Hex("#969CA6")
	dark.TextQuaternary = ui.Hex("#969CA6")
	dark.TextDescription = ui.Hex("#969CA6")
	dark.TextLabel = ui.Hex("#969CA6")
	dark.TextPlaceholder = ui.Hex("#969CA6")
	dark.TextDisabled = ui.Hex("#969CA6")
	dark.Icon = ui.Hex("#969CA6")
	dark.IconHover = ui.Hex("#FFFAFF")
	dark.Split = ui.RGBA(255, 250, 255, 0.08)
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
	tk := theme.TokensOf(c)
	if t.Dark {
		return uiPalette{
			theme:        t,
			canvas:       tk.BgBase,
			panel:        tk.BgContainer,
			panelMuted:   tk.BgContainerSecondary,
			panelHover:   tk.PrimaryBgHover,
			fill:         ui.Hex("#324F66"),
			hover:        ui.Hex("#424E5A"),
			border:       tk.Border,
			borderStrong: ui.Hex("#4F677B"),
			ink:          tk.Text,
			inkSecondary: tk.TextSecondary,
			muted:        tk.TextSecondary,
			accent:       tk.Primary,
			accentText:   tk.PrimaryForeground,
			accentSoft:   tk.PrimaryBg,
			danger:       tk.Error,
			dangerSoft:   tk.ErrorBg,
			warning:      tk.Warning,
			warningSoft:  tk.WarningBg,
			success:      tk.Success,
			successSoft:  tk.SuccessBg,
			info:         tk.Info,
			infoSoft:     tk.InfoBg,
		}
	}

	return uiPalette{
		theme:        t,
		canvas:       tk.BgBase,
		panel:        tk.BgContainer,
		panelMuted:   tk.BgContainerSecondary,
		panelHover:   tk.PrimaryBgHover,
		fill:         ui.Hex("#B7CFE2"),
		hover:        ui.Hex("#D5DBE2"),
		border:       tk.Border,
		borderStrong: ui.Hex("#7D8794"),
		ink:          tk.Text,
		inkSecondary: tk.TextSecondary,
		muted:        tk.TextSecondary,
		accent:       tk.Primary,
		accentText:   tk.PrimaryForeground,
		accentSoft:   tk.PrimaryBg,
		danger:       tk.Error,
		dangerSoft:   tk.ErrorBg,
		warning:      tk.Warning,
		warningSoft:  tk.WarningBg,
		success:      tk.Success,
		successSoft:  tk.SuccessBg,
		info:         tk.Info,
		infoSoft:     tk.InfoBg,
	}
}

func titleSize(t ui.Theme) float32 {
	return t.FontSize * 28 / 16
}

func sectionSize(t ui.Theme) float32 {
	return t.FontSize * 18 / 16
}

func bodySize(t ui.Theme) float32 {
	return t.FontSize
}

func captionSize(t ui.Theme) float32 {
	return t.FontSize * 14 / 16
}
