package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// brand 是应用唯一的主题。背景使用中性黑与中性灰，强调色只有深蓝一种，
// 成功、提醒和错误各自保留可辨认的颜色。
var brand = func() *theme.Config {
	light, dark := *theme.Light(), *theme.Dark()

	light.Primary = ui.Hex("#2B4FC7")
	light.PrimaryHover = ui.Hex("#355CDB")
	light.PrimaryActive = ui.Hex("#213EA3")
	light.PrimaryForeground = ui.Hex("#FFFFFF")
	light.PrimaryText = light.Primary
	light.PrimaryTextHover = light.PrimaryHover
	light.PrimaryTextActive = light.PrimaryActive
	light.PrimaryBg = ui.Hex("#EEF1FD")
	light.PrimaryBgHover = ui.Hex("#E3E9FC")
	light.PrimaryBorder = ui.Hex("#C6D0F4")
	light.PrimaryBorderHover = ui.Hex("#AABAF0")
	light.PrimaryFill = ui.RGBA(43, 79, 199, 0.36)
	light.PrimaryFillSecondary = ui.RGBA(43, 79, 199, 0.2)
	light.PrimaryFillTertiary = ui.RGBA(43, 79, 199, 0.1)
	light.PrimaryFillQuaternary = ui.RGBA(43, 79, 199, 0.04)
	light.Link = light.Primary
	light.LinkHover = light.PrimaryHover
	light.LinkActive = light.PrimaryActive
	light.Success = ui.Hex("#2F7D53")
	light.SuccessHover = ui.Hex("#389363")
	light.SuccessActive = ui.Hex("#256642")
	light.SuccessText = light.Success
	light.SuccessTextHover = light.SuccessHover
	light.SuccessTextActive = light.SuccessActive
	light.SuccessBg = ui.Hex("#EDF6F0")
	light.SuccessBgHover = ui.Hex("#E2F0E7")
	light.SuccessBorder = ui.Hex("#C8E0D2")
	light.SuccessBorderHover = ui.Hex("#AFD3BE")
	light.Warning = ui.Hex("#8A641C")
	light.WarningHover = ui.Hex("#9C7424")
	light.WarningActive = ui.Hex("#715115")
	light.WarningText = light.Warning
	light.WarningTextHover = light.WarningHover
	light.WarningTextActive = light.WarningActive
	light.WarningBg = ui.Hex("#FAF3E2")
	light.WarningBgHover = ui.Hex("#F5EBD4")
	light.WarningBorder = ui.Hex("#E7D8B4")
	light.WarningBorderHover = ui.Hex("#DAC798")
	light.Error = ui.Hex("#A4423C")
	light.ErrorHover = ui.Hex("#B5514A")
	light.ErrorActive = ui.Hex("#87332E")
	light.ErrorText = light.Error
	light.ErrorTextHover = light.ErrorHover
	light.ErrorTextActive = light.ErrorActive
	light.ErrorBg = ui.Hex("#FAECEA")
	light.ErrorBgHover = ui.Hex("#F4DFDB")
	light.ErrorBorder = ui.Hex("#E9C8C2")
	light.ErrorBorderHover = ui.Hex("#DBB0A8")
	light.Info = ui.Hex("#5F5F6A")
	light.InfoHover = ui.Hex("#71717D")
	light.InfoActive = ui.Hex("#4C4C56")
	light.InfoBg = ui.Hex("#F4F4F6")
	light.InfoBorder = ui.Hex("#E4E4E9")

	light.BgBase = ui.Hex("#FFFFFF")
	light.BgLayout = ui.Hex("#FFFFFF")
	light.BgContainer = ui.Hex("#FAFAFB")
	light.BgContainerSecondary = ui.Hex("#F4F4F6")
	light.BgElevated = ui.Hex("#FFFFFF")
	light.BgSpotlight = ui.Hex("#ECECEF")
	light.Border = ui.Hex("#E4E4E9")
	light.BorderSecondary = ui.Hex("#EFEFF2")
	light.BorderDisabled = ui.Hex("#E7E7EB")
	light.Fill = ui.RGBA(24, 24, 27, 0.12)
	light.FillSecondary = ui.RGBA(24, 24, 27, 0.06)
	light.FillTertiary = ui.RGBA(24, 24, 27, 0.035)
	light.FillQuaternary = ui.RGBA(24, 24, 27, 0.02)
	light.Text = ui.Hex("#18181B")
	light.TextHeading = ui.Hex("#111113")
	light.TextSecondary = ui.Hex("#52525B")
	light.TextTertiary = ui.Hex("#71717A")
	light.TextQuaternary = ui.Hex("#A1A1AA")
	light.TextDescription = ui.Hex("#71717A")
	light.TextLabel = ui.Hex("#52525B")
	light.TextPlaceholder = ui.Hex("#A1A1AA")
	light.TextDisabled = ui.Hex("#B4B4BC")
	light.Icon = ui.Hex("#71717A")
	light.IconHover = ui.Hex("#18181B")
	light.Split = ui.RGBA(24, 24, 27, 0.08)
	light.Highlight = light.Primary

	dark.Primary = ui.Hex("#2F58D8")
	dark.PrimaryHover = ui.Hex("#3F6AEE")
	dark.PrimaryActive = ui.Hex("#2547B8")
	dark.PrimaryForeground = ui.Hex("#FFFFFF")
	dark.PrimaryText = ui.Hex("#6E8FF5")
	dark.PrimaryTextHover = ui.Hex("#8AA4F8")
	dark.PrimaryTextActive = ui.Hex("#5879EA")
	dark.PrimaryBg = ui.Hex("#0F1835")
	dark.PrimaryBgHover = ui.Hex("#152147")
	dark.PrimaryBorder = ui.Hex("#26366B")
	dark.PrimaryBorderHover = ui.Hex("#33488A")
	dark.PrimaryFill = ui.RGBA(47, 88, 216, 0.44)
	dark.PrimaryFillSecondary = ui.RGBA(47, 88, 216, 0.24)
	dark.PrimaryFillTertiary = ui.RGBA(47, 88, 216, 0.14)
	dark.PrimaryFillQuaternary = ui.RGBA(47, 88, 216, 0.06)
	dark.Link = dark.PrimaryText
	dark.LinkHover = dark.PrimaryTextHover
	dark.LinkActive = dark.PrimaryTextActive
	dark.Success = ui.Hex("#63C08A")
	dark.SuccessHover = ui.Hex("#7BD09E")
	dark.SuccessActive = ui.Hex("#4EA675")
	dark.SuccessText = dark.Success
	dark.SuccessTextHover = dark.SuccessHover
	dark.SuccessTextActive = dark.SuccessActive
	dark.SuccessBg = ui.Hex("#0D2417")
	dark.SuccessBgHover = ui.Hex("#123020")
	dark.SuccessBorder = ui.Hex("#1F4A30")
	dark.SuccessBorderHover = ui.Hex("#2A6240")
	dark.Warning = ui.Hex("#D9B163")
	dark.WarningHover = ui.Hex("#E6C27B")
	dark.WarningActive = ui.Hex("#C29A4B")
	dark.WarningText = dark.Warning
	dark.WarningTextHover = dark.WarningHover
	dark.WarningTextActive = dark.WarningActive
	dark.WarningBg = ui.Hex("#2A2213")
	dark.WarningBgHover = ui.Hex("#372C19")
	dark.WarningBorder = ui.Hex("#4A3B1E")
	dark.WarningBorderHover = ui.Hex("#5C4A28")
	dark.Error = ui.Hex("#E07A72")
	dark.ErrorHover = ui.Hex("#EC8F87")
	dark.ErrorActive = ui.Hex("#C9655E")
	dark.ErrorText = dark.Error
	dark.ErrorTextHover = dark.ErrorHover
	dark.ErrorTextActive = dark.ErrorActive
	dark.ErrorBg = ui.Hex("#2A1513")
	dark.ErrorBgHover = ui.Hex("#381D1A")
	dark.ErrorBorder = ui.Hex("#4C2A26")
	dark.ErrorBorderHover = ui.Hex("#5E3833")
	dark.Info = ui.Hex("#9C9CA5")
	dark.InfoHover = ui.Hex("#B2B2BA")
	dark.InfoActive = ui.Hex("#85858E")
	dark.InfoBg = ui.Hex("#1A1A1E")
	dark.InfoBorder = ui.Hex("#2A2A30")

	dark.BgBase = ui.Hex("#000000")
	dark.BgLayout = ui.Hex("#000000")
	dark.BgContainer = ui.Hex("#0B0B0D")
	dark.BgContainerSecondary = ui.Hex("#121215")
	dark.BgElevated = ui.Hex("#17171A")
	dark.BgSpotlight = ui.Hex("#202024")
	dark.Border = ui.Hex("#29292F")
	dark.BorderSecondary = ui.Hex("#1C1C21")
	dark.BorderDisabled = ui.Hex("#232329")
	dark.Fill = ui.RGBA(255, 255, 255, 0.14)
	dark.FillSecondary = ui.RGBA(255, 255, 255, 0.08)
	dark.FillTertiary = ui.RGBA(255, 255, 255, 0.05)
	dark.FillQuaternary = ui.RGBA(255, 255, 255, 0.03)
	dark.Text = ui.Hex("#EDEDF0")
	dark.TextHeading = ui.Hex("#FAFAFB")
	dark.TextSecondary = ui.Hex("#A8A8B0")
	dark.TextTertiary = ui.Hex("#85858E")
	dark.TextQuaternary = ui.Hex("#62626B")
	dark.TextDescription = ui.Hex("#85858E")
	dark.TextLabel = ui.Hex("#A8A8B0")
	dark.TextPlaceholder = ui.Hex("#62626B")
	dark.TextDisabled = ui.Hex("#55555D")
	dark.Icon = ui.Hex("#85858E")
	dark.IconHover = ui.Hex("#EDEDF0")
	dark.Split = ui.RGBA(255, 255, 255, 0.08)
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
	tk := theme.TokensOf(c)
	t := *c.Theme()

	return uiPalette{
		theme:        t,
		canvas:       tk.BgLayout,
		panel:        tk.BgContainer,
		panelMuted:   tk.BgContainerSecondary,
		panelHover:   tk.FillTertiary,
		fill:         tk.FillSecondary,
		hover:        tk.FillTertiary,
		border:       tk.Border,
		borderStrong: tk.Border,
		ink:          tk.Text,
		inkSecondary: tk.TextSecondary,
		muted:        tk.TextTertiary,
		accent:       tk.Primary,
		accentText:   tk.PrimaryForeground,
		accentSoft:   tk.PrimaryBg,
		danger:       tk.Error,
		dangerSoft:   tk.ErrorBg,
		warning:      tk.Warning,
		warningSoft:  tk.WarningBg,
		success:      tk.Success,
		successSoft:  tk.SuccessBg,
		info:         tk.TextSecondary,
		infoSoft:     tk.InfoBg,
	}
}

func titleSize(t ui.Theme) float32 {
	return 28
}

func sectionSize(t ui.Theme) float32 {
	return 18
}

func bodySize(t ui.Theme) float32 {
	return 14
}

func captionSize(t ui.Theme) float32 {
	return 12
}
