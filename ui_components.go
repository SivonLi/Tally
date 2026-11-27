package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/locales"
)

type badgeTone int

const (
	toneNeutral badgeTone = iota
	toneAccent
	toneSuccess
	toneWarning
	toneDanger
)

type noticeTone int

const (
	noticeInfo noticeTone = iota
	noticeSuccess
	noticeWarning
	noticeDanger
)

// Sunbeam 标准动效曲线（见 DESIGN.md 与 .agents/skills/sunbeam/SKILL.md 第 8 节）。
var (
	sunbeamEaseOut    = ui.CubicBezier(0.22, 1, 0.36, 1)
	sunbeamEaseSpring = ui.CubicBezier(0.34, 1.4, 0.64, 1)
	sunbeamEaseInOut  = ui.CubicBezier(0.65, 0, 0.35, 1)
	sunbeamEaseDrawer = ui.CubicBezier(0.32, 0.72, 0, 1)

	buttonPress      = ui.Animation{Duration: sunbeam.DurationPress, Ease: sunbeamEaseOut}
	microButtonPress = ui.Animation{Duration: 100 * time.Millisecond, Ease: sunbeamEaseOut}
	selectTransition = ui.ElementTransition{Duration: sunbeam.DurationPress, Ease: sunbeamEaseOut, Colors: true}
	segmentSpring    = ui.SpringOptions{Duration: sunbeam.DurationIndicator, Bounce: 0.2}
	popupTransition  = ui.ElementTransition{
		Duration:     sunbeam.DurationPopup,
		Ease:         sunbeamEaseOut,
		ExitDuration: sunbeam.DurationPopupExit,
		ExitEase:     sunbeamEaseOut,
		Enter:        &ui.Motion{Scale: 0.96, Y: -4},
		Exit:         &ui.Motion{Scale: 0.96, Y: -4},
	}
	drawerTransition = ui.ElementTransition{
		Duration:     sunbeam.DurationDrawer,
		Ease:         sunbeamEaseDrawer,
		ExitDuration: sunbeam.DurationDrawerExit,
		ExitEase:     sunbeamEaseDrawer,
		Enter:        &ui.Motion{X: 16},
		Exit:         &ui.Motion{X: 16},
	}
	wizardDoneEnter = ui.ElementTransition{
		Duration: sunbeam.DurationDialog,
		Ease:     sunbeamEaseSpring,
		Enter:    &ui.Motion{Scale: 0.9},
	}
)

const busyDelay = 250 * time.Millisecond

// panel 是文档流内的内容卡片：遵循 Sunbeam 规范，内容卡片始终保持 Flat（白/暖墨实底 + 1px 细边，不加浮层阴影）。
func panel(c *ui.Context, p uiPalette) ui.Element {
	return ui.Column(c).
		Background(p.panel).
		Border(1, p.border).
		Radius(sunbeam.Radius2XL)
}

// section 是页面内的平面分区，通过统一的分割线与间距划分层次。
func section(c *ui.Context, p uiPalette, body func()) ui.Element {
	t := p.theme
	return ui.Column(c).FillWidth().Gap(t.Space(3)).Children(body)
}

func sectionRule(c *ui.Context, p uiPalette, body func()) ui.Element {
	t := p.theme
	return ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
		rule(c)
		body()
	})
}

func (a *app) languageSwitch(c *ui.Context) {
	tags := []string{"en", "zh-Hans"}
	chosen := 0
	if a.language == "zh-Hans" {
		chosen = 1
	}
	next := chosen
	segmentedField(c, &next, []string{i18n.Autonym("en"), i18n.Autonym("zh-Hans")},
		locales.LanguageLabel())
	if next != chosen {
		a.language = tags[next]
		mygo.App.SetLocale(a.language)
	}
}

// controlHeight 返回随系统文字缩放自适应的 Sunbeam 标准控件高度（默认 40 DIP）。
func controlHeight(t ui.Theme) float32 {
	return max(40, bodySize(t)*2.75)
}

// controlHeightLG 返回页面级主操作按钮高度，与 Sunbeam 40 DIP 胶囊体系保持一致。
func controlHeightLG(t ui.Theme) float32 {
	return controlHeight(t)
}

// tableRowHeight 返回随系统文字缩放自适应的表格行高（默认 40 DIP）。
func tableRowHeight(t ui.Theme) float32 {
	return max(40, bodySize(t)*2.75)
}

func pageHeading(c *ui.Context, p uiPalette, title, subtitle string, actions func()) {
	t := p.theme
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
				ui.Text(c, title).FontSize(titleSize(t)).FontWeight(500).
					SingleLine().MinWidth(0).Shrink(1).Tooltip(title).
					Role(ui.RoleHeading).Level(1)
				if subtitle != "" {
					ui.Text(c, subtitle).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
				}
			})
			if actions != nil {
				ui.Row(c).Wrap().Shrink(0).AlignItems(ui.Center).Gap(t.Space(2)).Children(actions)
			}
		})
	})
}

func sectionHeading(c *ui.Context, p uiPalette, title string, count int) {
	t := p.theme
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		ui.Text(c, title).FontSize(sectionSize(t)).FontWeight(500).
			SingleLine().MinWidth(0).Shrink(1).Tooltip(title)
		if count >= 0 {
			ui.Text(c, formatInt(count)).FontSize(captionSize(t)).TextColor(p.muted).
				FontFeatures("tnum").Shrink(0)
		}
	})
}

func listPanel(c *ui.Context, p uiPalette, title string, count int, body func()) {
	t := p.theme
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Role(ui.RoleComplementary).Label(title).
		Gap(t.Space(2)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, title).FontSize(bodySize(t)).FontWeight(600).
				SingleLine().MinWidth(0).Shrink(1).Tooltip(title)
			ui.Text(c, formatInt(count)).FontSize(captionSize(t)).TextColor(p.muted).
				FontFeatures("tnum").Shrink(0)
		})
		body()
	})
}

// badge 是 Sunbeam 风格的安静胶囊徽标：暖灰底、墨色字，状态色只以 6px 圆点呈现，
// 从不将主动作明黄用于徽标或正文。
func badge(c *ui.Context, p uiPalette, text string, tone badgeTone) {
	t := p.theme
	var dotColor *ui.Color
	switch tone {
	case toneAccent:
		col := p.info
		dotColor = &col
	case toneSuccess:
		col := p.success
		dotColor = &col
	case toneWarning:
		col := p.warning
		dotColor = &col
	case toneDanger:
		col := p.danger
		dotColor = &col
	}
	ui.Row(c).Height(24).PaddingX(10).Gap(6).Radius(12).
		Shrink(0).AlignItems(ui.Center).Background(p.fill).Children(func() {
		if dotColor != nil {
			ui.Box(c).Size(6, 6).Radius(3).Background(*dotColor).Shrink(0)
		}
		ui.Text(c, text).FontSize(captionSize(t)).FontWeight(500).
			TextColor(p.inkSecondary).SingleLine()
	})
}

func statusBadge(c *ui.Context, p uiPalette, status string, label string) {
	switch status {
	case "published", "calculated", "ready":
		badge(c, p, label, toneSuccess)
	case "imported", "applied", "scored", "assessed":
		badge(c, p, label, toneAccent)
	case "draft", "pending", "stale":
		badge(c, p, label, toneWarning)
	default:
		badge(c, p, label, toneNeutral)
	}
}

func kindBadge(c *ui.Context, p uiPalette, kind string, label string) {
	if kind == "system" {
		badge(c, p, label, toneAccent)
		return
	}
	badge(c, p, label, toneNeutral)
}

func metric(c *ui.Context, p uiPalette, value, label string) {
	t := p.theme
	ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, value).FontSize(titleSize(t)).FontWeight(500).FontFeatures("tnum")
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted)
	})
}

// metricInline 把一组数量排成一行，数值跟随正文字号，标签使用次要文字颜色。
func metricInline(c *ui.Context, p uiPalette, value uint64, label string) {
	t := p.theme
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
		ui.Text(c, formatCount(int64(value))).FontSize(bodySize(t)).FontWeight(600).FontFeatures("tnum")
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted)
	})
}

// notice 遵循 Sunbeam 原则：实底卡片 + 1px 细边，状态色仅用于图标，正文保持暖墨色保证 AA 对比度。
func notice(c *ui.Context, p uiPalette, tone noticeTone, text string) {
	text = localizeErrorMessage(text)
	t := p.theme
	accent, icon := p.info, iconInfo
	switch tone {
	case noticeSuccess:
		accent, icon = p.success, iconCircleCheck
	case noticeWarning:
		accent, icon = p.warning, iconAlert
	case noticeDanger:
		accent, icon = p.danger, iconCircleX
	}
	ui.Row(c).Gap(t.Space(2.5)).AlignSelf(ui.Stretch).AlignItems(ui.Start).
		Radius(sunbeam.RadiusXL).Padding(t.Space(3), t.Space(4)).
		Background(p.panel).Border(1, p.border).Children(func() {
		ui.Row(c).Height(t.Space(5.5)).AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Icon(c, icon).FontSize(t.Space(4)).TextColor(accent)
		})
		ui.Text(c, text).FontSize(bodySize(t)).TextColor(p.ink).Grow(1).Shrink(1).MinWidth(0).Pretty().Selectable()
	})
}

// errorState 在启动或存储不可用时展示带恢复动作的错误空状态，避免整页死锁或纯白屏。
func errorState(c *ui.Context, p uiPalette, title, detail string, onRetry, onSettings func()) {
	t := p.theme
	detail = localizeErrorMessage(detail)
	emptyState(c, p, iconAlert, title, detail, func() {
		ui.Row(c).Wrap().Gap(t.Space(2)).Children(func() {
			if onRetry != nil {
				if primaryButton(c, locales.RetryLoad()).Clicked() {
					onRetry()
				}
			}
			if onSettings != nil {
				if plainButton(c, locales.OpenSettings()).Clicked() {
					onSettings()
				}
			}
		})
	})
}

// cellText 渲染表格单行单元格文本：空值显示次要 "-"，非空值单行省略并提供完整 Tooltip。
func cellText(c *ui.Context, p uiPalette, value string) ui.Element {
	if strings.TrimSpace(value) == "" {
		return ui.Text(c, "-").TextColor(p.muted).SingleLine()
	}
	return ui.Text(c, value).SingleLine().Tooltip(value)
}

// mutedCellText 渲染表格次要颜色单行单元格文本：空值显示 "-"，非空值单行省略并提供 Tooltip。
func mutedCellText(c *ui.Context, p uiPalette, value string) ui.Element {
	if strings.TrimSpace(value) == "" {
		return ui.Text(c, "-").TextColor(p.muted).SingleLine()
	}
	return ui.Text(c, value).TextColor(p.muted).SingleLine().Tooltip(value)
}

func emptyState(c *ui.Context, p uiPalette, icon *ui.SVG, title, body string, action func()) {
	t := p.theme
	ui.Column(c).Grow(1).FillWidth().Center().Padding(t.Space(8)).Children(func() {
		ui.Column(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
			if icon != nil {
				ui.Icon(c, icon).FontSize(t.Space(8)).TextColor(p.muted)
			}
			if title != "" {
				ui.Text(c, title).FontSize(bodySize(t)).FontWeight(500).
					TextColor(p.ink).TextAlign(ui.Center)
			}
			if body != "" {
				ui.Text(c, body).FontSize(captionSize(t)).TextColor(p.muted).
					TextAlign(ui.Center).Pretty().MaxWidth(t.Space(170))
			}
			if action != nil {
				ui.Row(c).Margin(t.Space(2), 0, 0, 0).Children(action)
			}
		})
	})
}

func smallEmptyState(c *ui.Context, p uiPalette, body string) {
	t := p.theme
	ui.Column(c).Grow(1).FillWidth().Center().Padding(t.Space(6)).Children(func() {
		ui.Text(c, body).FontSize(captionSize(t)).TextColor(p.muted).
			TextAlign(ui.Center).Pretty().MaxWidth(t.Space(170))
	})
}

func applyButtonPress(element ui.Element) {
	pose := ui.Pose{}
	if element.Pressed() && !element.IsDisabled() {
		pose.Scale = 0.97
	}
	element.AnimatePose(pose, buttonPress)
}

func applyMicroPress(element ui.Element) {
	pose := ui.Pose{}
	if element.Pressed() && !element.IsDisabled() {
		pose.Scale = 0.94
	}
	element.AnimatePose(pose, microButtonPress)
}

func sunbeamPillBase(c *ui.Context, h float32, label string) ui.Element {
	return ui.ButtonBase(c).Height(h).MinWidth(h).Radius(h / 2).
		PaddingX(16).Gap(8).Center().Cursor(ui.CursorPointer).
		Label(label).Shrink(0)
}

func labelledButton(c *ui.Context, label string, icon *ui.SVG, primary bool) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	h := controlHeight(t)
	element := sunbeamPillBase(c, h, label)
	if primary {
		element.TextColor(sb.PrimaryForeground)
		sunbeam.PrimaryFace(c, element)
	} else {
		element.TextColor(sb.Foreground)
		sunbeam.Face(c, element)
	}
	element.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(16).Shrink(0)
		}
		ui.Text(c, label).FontSize(bodySize(t)).FontWeight(500).SingleLine()
	})
	applyButtonPress(element)
	return element
}

// plainButton 是 Sunbeam 次要胶囊按钮，自动跟随所在层的 Flat/Raised 材质。
func plainButton(c *ui.Context, label string) ui.Element {
	return labelledButton(c, label, nil, false)
}

// primaryButton 是 Sunbeam 明黄主动作胶囊按钮，一个视图只用于核心主操作。
func primaryButton(c *ui.Context, label string) ui.Element {
	return labelledButton(c, label, nil, true)
}

// textButton 是无底色胶囊幽灵按钮，悬停或按下时出现柔和暖墨填充。
func textButton(c *ui.Context, label string) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	h := controlHeight(t)
	element := sunbeamPillBase(c, h, label).Border(1, ui.Transparent)
	switch {
	case element.Pressed():
		element.Background(sb.FlatFacePressed).TextColor(sb.Foreground)
	case element.Hovered():
		element.Background(sb.FlatFaceHover).TextColor(sb.Foreground)
	default:
		element.TextColor(sb.MutedForeground)
	}
	element.Children(func() {
		ui.Text(c, label).FontSize(bodySize(t)).FontWeight(500).SingleLine()
	})
	applyButtonPress(element)
	return element
}

// dangerButton 是危险动作胶囊按钮：采用危险色细边与轻淡底色，避免大面积刺眼纯红块。
func dangerButton(c *ui.Context, label string) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	h := controlHeight(t)
	element := sunbeamPillBase(c, h, label).Transition(selectTransition)
	bg := sb.Danger.Alpha(0.08)
	border := sb.Danger.Alpha(0.32)
	fg := sb.Danger
	switch {
	case element.Pressed():
		bg = sb.Danger.Alpha(0.20)
		border = sb.Danger
	case element.Hovered():
		bg = sb.Danger.Alpha(0.14)
		border = sb.Danger.Alpha(0.55)
	}
	element.Background(bg).Border(1, border).TextColor(fg).Children(func() {
		ui.Text(c, label).FontSize(bodySize(t)).FontWeight(500).SingleLine()
	})
	applyButtonPress(element)
	return element
}

// dangerIconButton 是用于删除列表行等轻量危险动作的圆形图标按钮。
func dangerIconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	size := controlHeight(t)
	element := ui.ButtonBase(c).Size(size, size).Radius(size/2).Center().
		Cursor(ui.CursorPointer).Label(label).Tooltip(label).Shrink(0).
		Border(1, ui.Transparent).TextColor(sb.MutedForeground)
	switch {
	case element.Pressed():
		element.Background(sb.Danger.Alpha(0.18)).TextColor(sb.Danger)
	case element.Hovered():
		element.Background(sb.Danger.Alpha(0.10)).TextColor(sb.Danger)
	}
	element.Children(func() {
		ui.Icon(c, icon).Size(16, 16)
	})
	applyMicroPress(element)
	return element
}

// pageActionButton 是页面级主动作胶囊按钮。
func pageActionButton(c *ui.Context, label string) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	h := controlHeightLG(t)
	element := sunbeamPillBase(c, h, label).PaddingX(20).TextColor(sb.PrimaryForeground)
	sunbeam.PrimaryFace(c, element)
	element.Children(func() {
		ui.Text(c, label).FontSize(bodySize(t)).FontWeight(500).SingleLine()
	})
	applyButtonPress(element)
	return element
}

func linkRow(c *ui.Context, p uiPalette, label string, body func()) ui.Element {
	t := p.theme
	element := ui.ButtonBase(c).
		FillWidth().Label(label).MinHeight(t.Space(14)).
		Padding(t.Space(3), t.Space(3)).Radius(sunbeam.RadiusLG).
		Cursor(ui.CursorPointer).TextColor(p.ink)
	switch {
	case element.Pressed():
		element.Background(p.sb.FlatFacePressed)
	case element.Hovered():
		element.Background(p.hover)
	}
	element.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(body)
	})
	return element
}

// listRowButton 是用于列表项的透明背景行按钮，只设置无障碍名称，不插入重复视觉文本。
func listRowButton(c *ui.Context, label string) ui.Element {
	t := *c.Theme()
	p := uiPaletteFor(c)
	element := ui.ButtonBase(c).
		FillWidth().Label(label).FontSize(bodySize(t)).Cursor(ui.CursorPointer).
		Radius(sunbeam.RadiusLG).TextColor(p.ink)
	switch {
	case element.Pressed():
		element.Background(p.sb.FlatFacePressed)
	case element.Hovered():
		element.Background(p.hover)
	}
	return element
}

// backButton 是统一的返回胶囊按钮，带左箭头图标与返回文案。
func backButton(c *ui.Context, p uiPalette) ui.Element {
	t := p.theme
	h := controlHeight(t)
	element := sunbeamPillBase(c, h, locales.Back()).Padding(0, 12).
		Border(1, ui.Transparent).TextColor(p.muted)
	switch {
	case element.Pressed():
		element.Background(p.sb.FlatFacePressed).TextColor(p.ink)
	case element.Hovered():
		element.Background(p.hover).TextColor(p.ink)
	}
	element.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
			ui.Icon(c, iconArrowLeft).Size(16, 16)
			ui.Text(c, locales.Back()).FontSize(bodySize(t)).FontWeight(500).SingleLine()
		})
	})
	applyButtonPress(element)
	return element
}

// progressBar 是 Sunbeam 风格的进度条：凹槽轨道 + 暖墨填充（不用明黄，明黄仅留给唯一主按钮）。
func progressBar(c *ui.Context, p uiPalette, percent float32, label string) ui.Element {
	t := p.theme
	pct := min(max(percent, 0), 100)
	root := ui.Column(c).FillWidth().Gap(t.Space(1.5)).Role(ui.RoleProgress).Range(0, 100, float64(pct))
	if label != "" {
		root.Label(label)
	}
	root.Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
		}
		bar := ui.Box(c).FillWidth().Height(6).Radius(3).Clip()
		sunbeam.Well(c, bar)
		bar.Children(func() {
			if pct > 0 {
				ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).WidthPercent(pct).
					Radius(3).Background(p.ink)
			}
		})
	})
	return root
}

// showDelayedBusy 在异步任务超过 250ms 后才显示加载指示器，避免 <300ms 的本地计算闪现造成画面抖动。
func showDelayedBusy(c *ui.Context, key string, busy bool) bool {
	since := ui.Local(ui.Box(c.Key(key)).Absolute().Size(0, 0), "busy-since", func() time.Time {
		return time.Time{}
	})
	if !busy {
		*since = time.Time{}
		return false
	}
	if since.IsZero() {
		*since = c.Now()
	}
	elapsed := c.Now().Sub(*since)
	if elapsed >= busyDelay {
		return true
	}
	c.After(busyDelay - elapsed)
	return false
}

// rule 是页面里的 1px Sunbeam 横向分割线。
func rule(c *ui.Context) {
	sunbeam.Divider(c)
}

// selectField 是基于 ui.SelectBase 与 Sunbeam Field/PanelSurface 的下拉选择。
// 之所以保留薄包装而不是直接调用 sunbeam.Select，是因为 Tally 下拉需要支持：
// 1. 几十项列表时的最大高度滚动（ui.Scroll）；
// 2. 空选项禁用态与独立 Key 避免弹层切换裁切；
// 3. 无需调用点额外维护 sunbeam.SelectState 结构体字段。
func selectField(c *ui.Context, key string, value *string, labels []string, label string) ui.Element {
	empty := len(labels) == 0
	if empty {
		labels = []string{locales.SelectNoOptions()}
	}
	chosen := 0
	for index, item := range labels {
		if item == *value {
			chosen = index
		}
	}
	current := strconv.Itoa(chosen)
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	h := controlHeight(t)
	sel := ui.SelectBase(c, &current)
	trigger := sel.Trigger.Height(h).Padding(0, 12).Gap(8).
		FontSize(bodySize(t)).AlignItems(ui.Center).TextColor(sb.Foreground).
		Cursor(ui.CursorPointer).Unselectable().Transition(selectTransition).
		Label(label).Value(labels[chosen])
	sunbeam.Field(c, trigger)
	switch {
	case empty:
		trigger.Disabled(true).TextColor(sb.MutedForeground).Cursor(ui.CursorNotAllowed)
	case sel.Open():
		trigger.Border(1, sb.Ring)
	case trigger.Hovered():
		trigger.Border(1, sb.Faint)
	}
	trigger.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			text := ui.Text(c, labels[chosen]).SingleLine().MinWidth(0).Shrink(1)
			if !empty && labels[chosen] != "" {
				text.Tooltip(labels[chosen])
			}
		})
		arrow := ui.Icon(c, iconUpDown).Size(14, 14).TextColor(sb.MutedForeground).Shrink(0)
		if sel.Open() {
			arrow.Rotate(180)
		}
	})
	width := trigger.Bounds().W
	popupMaxWidth := max(width, t.Space(96))
	sel.Popup(func(panel ui.Element) {
		panel.Key(key)
		sunbeam.PanelSurface(c, panel)
		// 同心圆角：浮层圆角 RadiusXL (14) - 内边距 4 = 列表项圆角 RadiusLG (10)。
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(sunbeam.RadiusXL).
			MinWidth(max(width, 160)).MaxWidth(popupMaxWidth).
			Transition(popupTransition).AttachedOrigin()
		sunbeam.Layer(sunbeam.Raised, func() {
			ui.Scroll(c).MaxHeight(256).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					for index, item := range labels {
						index := index
						selected := index == chosen
						opt := sel.Item(strconv.Itoa(index)).Label(item).
							Padding(0, 10).Height(34).
							Gap(8).AlignItems(ui.Center).Radius(sunbeam.RadiusLG).
							AlignSelf(ui.Stretch).FontSize(bodySize(t)).TextColor(sb.Foreground)
						if opt.Highlighted() {
							opt.Background(sb.FlatFaceHover)
						}
						opt.Children(func() {
							ui.Text(c, item).Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(item)
							if selected {
								ui.Icon(c, iconCheck).Size(14, 14).TextColor(sb.Foreground).
									Shrink(0).Margin(0, 0, 0, 8)
							}
						})
					}
				})
			})
		})
	})
	if !empty {
		if index, err := strconv.Atoi(current); err == nil && index != chosen && index < len(labels) {
			*value = labels[index]
		}
	}
	return trigger
}

func inputRow(c *ui.Context) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	return ui.Row(c).Height(controlHeight(t)).PaddingX(12).
		AlignItems(ui.Center).Gap(8).
		FontSize(bodySize(t)).TextColor(sb.Foreground).Cursor(ui.CursorText)
}

// searchField 是 40 高、Sunbeam Field 外观的搜索框，保留 MyGo 原生 Esc 清空行为。
func searchField(c *ui.Context, value *string, label, placeholder string) ui.Element {
	t := *c.Theme()
	e := ui.SearchField(c, value).Label(label).Placeholder(placeholder).
		MinHeight(controlHeight(t)).FontSize(bodySize(t)).FillWidth()
	return sunbeam.Field(c, e)
}

// dateField 使用 MyGo v0.10.0 原生 ui.DateInput，并套用 Sunbeam Field 外观。
func dateField(c *ui.Context, value *time.Time, label string) ui.Element {
	t := *c.Theme()
	e := ui.DateInput(c, value).Label(label).
		Height(controlHeight(t)).FontSize(bodySize(t))
	return sunbeam.Field(c, e)
}

func styleInputRow(c *ui.Context, root ui.Element, focused bool) {
	sb := sunbeam.Colors(c)
	sunbeam.Field(c, root)
	if focused {
		root.Border(1, sb.Ring)
	}
}

// textField 是 40 高、Sunbeam Field 外观的单行输入框。
func textField(c *ui.Context, value *string, label, placeholder string) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	root := inputRow(c)
	var input ui.Element
	root.Children(func() {
		input = ui.TextInputBase(c, value).Grow(1).MinWidth(0).Height(controlHeight(t)).
			Placeholder(placeholder).PlaceholderColor(sb.MutedForeground).FocusRing(false)
	})
	styleInputRow(c, root, input.Focused())
	input.Label(label)
	return input
}

// textAreaField 是 Sunbeam Field 外观的多行输入框，行数由调用点控制。
func textAreaField(c *ui.Context, value *string, label, placeholder string, minRows, maxRows int) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	root := ui.Column(c).Padding(8, 12).
		FontSize(bodySize(t)).TextColor(sb.Foreground).Cursor(ui.CursorText)
	var area ui.Element
	root.Children(func() {
		area = ui.TextAreaBase(c, value).Lines(minRows, maxRows).FocusRing(false).
			Placeholder(placeholder).PlaceholderColor(sb.MutedForeground)
	})
	styleInputRow(c, root, area.Focused())
	area.Label(label)
	return area
}

// passwordField 是 40 高、Sunbeam Field 外观的密码输入框。
func passwordField(c *ui.Context, value *string, label, placeholder string) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	root := inputRow(c)
	visible := ui.Local(root, "visible", func() bool { return false })
	var input ui.Element
	root.Children(func() {
		input = ui.TextInputBase(c, value).Grow(1).MinWidth(0).Height(controlHeight(t)).
			Placeholder(placeholder).PlaceholderColor(sb.MutedForeground).FocusRing(false)
		if !*visible {
			input.Password()
		}
		text, glyph := locales.ShowPassword(), iconEyeOff
		if *visible {
			text, glyph = locales.HidePassword(), iconEye
		}
		toggle := ui.ButtonBase(c).Label(text).Size(28, 28).
			Radius(6).Center().Cursor(ui.CursorPointer).
			TextColor(sb.MutedForeground)
		switch {
		case toggle.Pressed():
			toggle.Background(sb.FlatFacePressed).TextColor(sb.Foreground)
		case toggle.Hovered():
			toggle.Background(sb.FlatFaceHover).TextColor(sb.Foreground)
		}
		applyMicroPress(toggle)
		if toggle.Clicked() {
			*visible = !*visible
		}
		toggle.Children(func() {
			ui.Icon(c, glyph).Size(16, 16)
		})
	})
	styleInputRow(c, root, input.Focused())
	input.Label(label)
	return input
}

type numberDraft struct {
	text    string
	focused bool
}

// numberField 是 40 高、Sunbeam Field 外观的数字输入框，使用 ui.Local 托管输入草稿，无需调用点持有状态。
func numberField(c *ui.Context, value *float64, lo, hi, step float64, label string) ui.Element {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	if step <= 0 {
		step = 1
	}
	format := func(v float64) string {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	clamp := func(v float64) float64 {
		if lo != hi {
			v = math.Min(math.Max(v, lo), hi)
		}
		return v
	}
	set := func(v float64) {
		*value = clamp(v)
	}

	root := inputRow(c)
	draft := ui.Local(root, "draft", func() numberDraft {
		return numberDraft{text: format(*value)}
	})
	commit := func() {
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(draft.text), 64); err == nil {
			set(parsed)
		}
		draft.text = format(*value)
	}

	var input ui.Element
	root.Children(func() {
		input = ui.TextInputBase(c, &draft.text).Grow(1).MinWidth(0).Height(controlHeight(t)).
			PlaceholderColor(sb.MutedForeground).FontFeatures("tnum").FocusRing(false)
		focused := input.Focused()
		switch {
		case input.Submitted(), draft.focused && !focused:
			commit()
		case input.Shortcut(0, ui.KeyUp):
			commit()
			set(*value + step)
			draft.text = format(*value)
		case input.Shortcut(0, ui.KeyDown):
			commit()
			set(*value - step)
			draft.text = format(*value)
		case !focused:
			draft.text = format(*value)
		}
		draft.focused = focused
		ui.Column(c).Shrink(0).Justify(ui.Center).Children(func() {
			if numberControl(c, iconChevronUp, locales.Increment()) {
				set(*value + step)
				draft.text = format(*value)
			}
			if numberControl(c, iconUpDown, locales.Decrement()) {
				set(*value - step)
				draft.text = format(*value)
			}
		})
	})
	styleInputRow(c, root, input.Focused())
	input.Label(label)
	return input
}

func numberControl(c *ui.Context, glyph *ui.SVG, label string) bool {
	sb := sunbeam.Colors(c)
	button := ui.ButtonBase(c).Label(label).Size(24, 17).
		Radius(4).Center().Cursor(ui.CursorPointer).
		TextColor(sb.MutedForeground)
	switch {
	case button.Pressed():
		button.Background(sb.FlatFacePressed).TextColor(sb.Foreground)
	case button.Hovered():
		button.Background(sb.FlatFaceHover).TextColor(sb.Foreground)
	}
	applyMicroPress(button)
	button.Children(func() {
		ui.Icon(c, glyph).Size(12, 12)
	})
	return button.Clicked()
}

// segmentedField 是 Sunbeam 风格的等宽分段选择器：
// 使用凹槽轨道（Well）与随弹簧滑动的选中滑块（Thumb），并保证窄容器下所有分段仍保持严格等宽且不溢出。
func segmentedField(c *ui.Context, selected *int, labels []string, label string) ui.Element {
	if len(labels) == 0 {
		return ui.Row(c)
	}
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	const wellInset float32 = 4
	const segPad float32 = 12
	trackH := controlHeight(t)
	segH := trackH - 2*wellInset

	segW := float32(0)
	for _, option := range labels {
		w, _ := c.MeasureText(0, ui.Span{Text: option, Font: t.Font, Size: bodySize(t), Weight: 500})
		segW = max(segW, w+2*segPad+2)
	}

	parts := ui.SegmentedBase(c, selected, len(labels))
	track := parts.Track.Height(trackH).MaxWidthPercent(100).
		Padding(wellInset).Gap(wellInset).Radius(trackH / 2).Label(label)
	sunbeam.Well(c, track)

	// 若多页签场景下等宽总宽度超出父容器，按已提交宽度等比收缩，保持每段依然严格等宽。
	if bounds := track.Bounds(); bounds.W > 0 {
		totalNeed := float32(len(labels))*segW + float32(len(labels)-1)*wellInset + 2*wellInset
		if totalNeed > bounds.W {
			avail := (bounds.W - 2*wellInset - float32(len(labels)-1)*wellInset) / float32(len(labels))
			if avail > 32 {
				segW = avail
			}
		}
	}

	chosen := *selected
	if chosen < 0 {
		chosen = 0
	} else if chosen >= len(labels) {
		chosen = len(labels) - 1
	}

	track.Children(func() {
		x := track.AnimateTo("thumb", float32(chosen)*(segW+wellInset), ui.Animation{Spring: segmentSpring})
		thumb := ui.Box(c).Width(segW).Height(segH).Radius(segH / 2).
			Absolute().Left(wellInset + x).Top(wellInset).PassThrough()
		sunbeam.Thumb(c, thumb)

		for index, option := range labels {
			segment := parts.Segment(index).Width(segW).Height(segH).Radius(segH/2).
				PaddingX(segPad).Center().Cursor(ui.CursorPointer).
				Border(1, ui.Transparent).ZIndex(1)
			color := sb.MutedForeground
			if index == chosen {
				color = sb.Foreground
			} else if segment.Hovered() {
				color = sb.Foreground
			}
			segment.TextColor(color)
			segment.Children(func() {
				ui.Text(c, option).FontSize(bodySize(t)).FontWeight(500).
					SingleLine().MinWidth(0).Shrink(1).Tooltip(option)
			})
		}
	})
	return track
}

// switchField 使用 Sunbeam 开关（暖墨轨道 + 弹簧滑块）。
func switchField(c *ui.Context, value *bool, label string) ui.Element {
	return sunbeam.Switch(c, value).Label(label)
}

// checkboxField 使用 Sunbeam 复选框（暖墨勾选框 + 白色对勾）。
func checkboxField(c *ui.Context, value *bool, label string) ui.Element {
	return sunbeam.Checkbox(c, value, label).Label(label)
}

// toggleCodeOption 描述多选开关组的一个选项。
type toggleCodeOption struct {
	code  string
	label string
}

// toggleCodeGroup 是多选胶囊开关组，遵循 Sunbeam「选中项用暖墨填充、未选中用柔和底色，不用明黄」原则，
// 避免 Flat 模式下 sunbeam.Toggle 的白色 FlatThumb 与白色 Card 卡片底色融为一体。
func toggleCodeGroup(c *ui.Context, values *[]string, options []toggleCodeOption, label string) ui.Element {
	t := *c.Theme()
	p := uiPaletteFor(c)
	return ui.Row(c).Wrap().Gap(8).Role(ui.RoleGroup).Label(label).Children(func() {
		for _, option := range options {
			option := option
			active := containsString(*values, option.code)
			next := active
			e := ui.ToggleBase(c, &next).Height(34).Padding(0, 14).Radius(17).
				Center().Cursor(ui.CursorPointer).Label(option.label)
			switch {
			case active:
				e.Background(p.ink).Border(1, p.ink).TextColor(p.canvas)
			case e.Pressed():
				e.Background(p.sb.FlatFacePressed).Border(1, p.border).TextColor(p.ink)
			case e.Hovered():
				e.Background(p.hover).Border(1, p.border).TextColor(p.ink)
			default:
				e.Background(p.fill).Border(1, ui.Transparent).TextColor(p.inkSecondary)
			}
			e.Children(func() {
				ui.Text(c, option.label).FontSize(bodySize(t)).FontWeight(500).SingleLine()
			})
			applyMicroPress(e)
			if next != active {
				if next {
					*values = append(*values, option.code)
				} else {
					filtered := make([]string, 0, len(*values))
					for _, existing := range *values {
						if existing != option.code {
							filtered = append(filtered, existing)
						}
					}
					*values = filtered
				}
			}
		}
	})
}

// formGroup 是替代 lobe-mygo/pkg/form.Group 的 Sunbeam 表单卡片分组。
func formGroup(c *ui.Context, p uiPalette, title, desc string, extra func(), body func()) {
	t := p.theme
	panel(c, p).FillWidth().Padding(20).Gap(16).Children(func() {
		if title != "" || desc != "" || extra != nil {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					if title != "" {
						ui.Text(c, title).FontSize(sectionSize(t)).FontWeight(500).TextColor(p.ink).SingleLine()
					}
					if desc != "" {
						ui.Text(c, desc).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
					}
				})
				if extra != nil {
					ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(t.Space(2)).Children(extra)
				}
			})
			rule(c)
		}
		ui.Column(c).FillWidth().Gap(16).Children(body)
	})
}

// formFieldItem 是表单字段行：支持垂直（标签在上）或水平（标签在左、控件在右）两种排布。
func formFieldItem(c *ui.Context, p uiPalette, label, desc string, horizontal bool, control func()) {
	t := p.theme
	if horizontal {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				if label != "" {
					ui.Text(c, label).FontSize(bodySize(t)).FontWeight(500).TextColor(p.ink)
				}
				if desc != "" {
					ui.Text(c, desc).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
				}
			})
			ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(control)
		})
		return
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(bodySize(t)).FontWeight(500).TextColor(p.ink)
		}
		control()
		if desc != "" {
			ui.Text(c, desc).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
		}
	})
}

// dialogForm 承载弹窗内的垂直表单字段集合。
func (a *app) dialogForm(c *ui.Context, body func()) {
	ui.Column(c).FillWidth().Gap(16).Children(body)
}

// dialogField 是弹窗内的垂直表单字段。
func (a *app) dialogField(c *ui.Context, _ string, label, desc string, control func()) {
	p := uiPaletteFor(c)
	formFieldItem(c, p, label, desc, false, control)
}

// stepsBar 是步骤条组件。
// 已核对 MyGo v0.10.0 官方组件清单与 sunbeam v0.2.0，两者均无内置步骤条控件，
// 因此在此作为项目唯一薄封装实现，使用暖墨序号圆点与 1px 细线连接。
func stepsBar(c *ui.Context, p uiPalette, labels []string, current int) {
	t := p.theme
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		for index, title := range labels {
			done := index < current
			active := index == current
			ui.Row(c).AlignItems(ui.Center).Gap(8).Shrink(0).Children(func() {
				dot := ui.Box(c).Size(24, 24).Radius(12).Center().Shrink(0)
				switch {
				case done:
					dot.Background(p.ink).TextColor(p.canvas)
				case active:
					dot.Background(p.ink).TextColor(p.canvas)
				default:
					dot.Background(p.fill).Border(1, p.border).TextColor(p.muted)
				}
				dot.Children(func() {
					if done {
						ui.Icon(c, iconCheck).Size(12, 12)
					} else {
						ui.Text(c, strconv.Itoa(index+1)).FontSize(12).FontWeight(600).FontFeatures("tnum")
					}
				})
				textColor := p.muted
				weight := 400
				if active || done {
					textColor = p.ink
				}
				if active {
					weight = 600
				}
				ui.Text(c, title).FontSize(captionSize(t)).FontWeight(weight).
					TextColor(textColor).SingleLine()
			})
			if index < len(labels)-1 {
				ui.Box(c).Grow(1).MinWidth(16).Height(1).Background(p.border)
			}
		}
	})
}

type actionListItem struct {
	Key         string
	Label       string
	Description string
	Extra       string
	Actions     func()
}

// actionList 是替代 lobe-mygo/pkg/display.List 的轻量操作列表。
func actionList(c *ui.Context, p uiPalette, items []actionListItem, borderless bool) string {
	t := p.theme
	clicked := ""
	renderRows := func() {
		for index, item := range items {
			item := item
			if index > 0 {
				rule(c)
			}
			if item.Actions == nil {
				btn := ui.ButtonBase(c.Key("alist-"+item.Key)).FillWidth().
					MinHeight(44).Padding(t.Space(2), t.Space(3)).Radius(sunbeam.RadiusLG).
					Cursor(ui.CursorPointer).Label(item.Label).TextColor(p.ink)
				switch {
				case btn.Pressed():
					btn.Background(p.sb.FlatFacePressed)
				case btn.Hovered():
					btn.Background(p.hover)
				}
				btn.Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, item.Label).FontSize(bodySize(t)).FontWeight(500).
								TextColor(p.ink).SingleLine().Tooltip(item.Label)
							if item.Description != "" {
								ui.Text(c, item.Description).FontSize(captionSize(t)).
									TextColor(p.muted).SingleLine().Tooltip(item.Description)
							}
						})
						if item.Extra != "" {
							ui.Text(c, item.Extra).FontSize(captionSize(t)).
								TextColor(p.muted).SingleLine().Shrink(0)
						}
					})
				})
				if btn.Clicked() {
					clicked = item.Key
				}
				continue
			}
			row := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).
				MinHeight(44).Padding(t.Space(2), t.Space(3))
			row.Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, item.Label).FontSize(bodySize(t)).FontWeight(500).
						TextColor(p.ink).SingleLine().Tooltip(item.Label)
					if item.Description != "" {
						ui.Text(c, item.Description).FontSize(captionSize(t)).
							TextColor(p.muted).SingleLine().Tooltip(item.Description)
					}
				})
				if item.Extra != "" {
					ui.Text(c, item.Extra).FontSize(captionSize(t)).
						TextColor(p.muted).SingleLine().Shrink(0)
				}
				ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(t.Space(2)).Children(item.Actions)
			})
		}
	}
	if borderless {
		ui.Column(c).FillWidth().Children(renderRows)
		return clicked
	}
	panel(c, p).FillWidth().Radius(sunbeam.RadiusXL).Padding(4).Children(renderRows)
	return clicked
}

const (
	modalWidthCompact float32 = 420
	modalDefaultWidth float32 = 520
	modalWidthWide    float32 = 680
)

type modalProps struct {
	Key        string
	Title      string
	Width      float32
	OkText     string
	CancelText string
	NoFooter   bool
}

type dialogState struct {
	shown   bool
	closing bool
	entered bool
}

// modal 使用 Sunbeam Raised 浮层材质（PanelSurface + Layer(Raised) + 暖墨 Scrim）承载弹窗，
// 保持同心圆角（RadiusTile 28px）与对称进出场动效。
func modal(c *ui.Context, open *bool, p modalProps, content func()) bool {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	key := p.Key
	if key == "" {
		key = p.Title
	}
	state := ui.Local(ui.Box(c.Key("modal-"+key)).Absolute().Size(0, 0), "dialog", func() dialogState {
		return dialogState{}
	})
	if *open {
		state.shown, state.closing = true, false
	} else if state.shown {
		state.closing = true
	}
	if !state.shown {
		return false
	}

	ok := false
	done := false
	ui.DialogBase(c, &state.shown, func(backdrop, panel ui.Element) {
		if !state.entered {
			state.entered = true
			panel.Jump("shown", 0)
			backdrop.Jump("dim", 0)
		}
		target := float32(1)
		show := ui.Animation{Duration: sunbeam.DurationDialog, Ease: sunbeamEaseOut}
		if state.closing {
			target = 0
			show = ui.Animation{Duration: sunbeam.DurationDialogExit, Ease: sunbeamEaseOut}
			backdrop.IgnoreInput(true)
		}
		shown := panel.AnimateTo("shown", target, show)
		dim := backdrop.AnimateTo("dim", target, show)
		done = state.closing && shown <= 0 && dim <= 0

		backdrop.Padding(t.Space(8), t.Space(4)).
			Background(sb.Scrim.Alpha(0.28 * dim))
		width := p.Width
		if width == 0 {
			width = modalDefaultWidth
		}
		scale := 0.96 + 0.04*shown
		sunbeam.PanelSurface(c, panel)
		panel.Width(width).MaxWidthPercent(100).MaxHeightPercent(100).
			Clip().TextColor(sb.Foreground).FontSize(bodySize(t)).
			Scale(scale, scale).Opacity(shown)

		panel.Children(func() {
			sunbeam.Layer(sunbeam.Raised, func() {
				if p.Title != "" {
					ui.Row(c).MinHeight(56).Padding(16, 24, 12, 24).
						AlignItems(ui.Center).Gap(12).Children(func() {
						ui.Text(c, p.Title).FontSize(sectionSize(t)).FontWeight(500).
							Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(p.Title)
						if modalCloseButton(c, sb).Clicked() {
							*open = false
						}
					})
				}
				ui.Scroll(c).Shrink(1).Children(func() {
					contentColumn := ui.Column(c).Padding(0, 24, 20, 24)
					if p.Title == "" {
						contentColumn.Padding(24)
					}
					contentColumn.Children(content)
				})
				if !p.NoFooter {
					ui.Row(c).Gap(8).Padding(12, 24, 20, 24).
						Justify(ui.End).Children(func() {
						if plainButton(c, textOr(p.CancelText, locales.Close())).Clicked() {
							*open = false
						}
						if primaryButton(c, textOr(p.OkText, locales.Save())).Clicked() {
							ok = true
						}
					})
				}
			})
		})
	})

	switch {
	case !state.shown:
		*open, state.shown, state.closing = false, true, true
		c.Invalidate()
	case done:
		*state = dialogState{}
		c.Invalidate()
	}
	return ok
}

// confirmDangerModal 是不可恢复操作的统一确认弹窗：
// 默认焦点放在取消按钮，危险按钮以 12px 间距与危险色描边隔离。
func confirmDangerModal(c *ui.Context, p uiPalette, key string, open *bool, title, body, cancelText, confirmText string) bool {
	t := p.theme
	confirmed := false
	modal(c, open, modalProps{
		Key:      key,
		Title:    title,
		Width:    modalWidthCompact,
		NoFooter: true,
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			if body != "" {
				ui.Text(c, body).FontSize(bodySize(t)).TextColor(p.inkSecondary).Pretty()
			}
			ui.Row(c).FillWidth().Wrap().Justify(ui.End).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				cancel := plainButton(c, textOr(cancelText, locales.PlanCancel())).AutoFocus()
				if cancel.Clicked() {
					*open = false
				}
				if dangerButton(c, confirmText).Clicked() {
					confirmed = true
					*open = false
				}
			})
		})
	})
	return confirmed
}

func modalCloseButton(c *ui.Context, sb *sunbeam.Palette) ui.Element {
	button := ui.ButtonBase(c).Label(locales.Close()).Size(32, 32).
		Shrink(0).Radius(16).Center().Cursor(ui.CursorPointer).
		TextColor(sb.MutedForeground)
	switch {
	case button.Pressed():
		button.Background(sb.FlatFacePressed).TextColor(sb.Foreground)
	case button.Hovered():
		button.Background(sb.FlatFaceHover).TextColor(sb.Foreground)
	}
	applyMicroPress(button)
	button.Children(func() {
		ui.Icon(c, iconX).Size(16, 16)
	})
	return button
}

var pageSizeOptions = []int{50, 100, 200, 500}

// normalizePageSize 返回合法的每页条数，未设置时默认 200。
func normalizePageSize(size int) int {
	for _, option := range pageSizeOptions {
		if size == option {
			return size
		}
	}
	return 200
}

// paginationBar 渲染带每页条数切换、范围统计与上一页/下一页的通用翻页栏。
func paginationBar(c *ui.Context, p uiPalette, keyPrefix string, page, pageSize *int, total int, onChange func()) {
	if total <= 0 {
		return
	}
	t := p.theme
	size := normalizePageSize(*pageSize)
	*pageSize = size
	totalPages := (total + size - 1) / size
	if totalPages < 1 {
		totalPages = 1
	}
	if *page >= totalPages {
		*page = totalPages - 1
	}
	if *page < 0 {
		*page = 0
	}
	start := *page*size + 1
	end := min((*page+1)*size, total)

	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).
		Padding(t.Space(1), 0, 0, 0).Children(func() {
		ui.Textf(c, locales.PaginationSummary(), formatInt(start), formatInt(end), formatInt(total)).
			FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").SingleLine()
		ui.Spacer(c)
		labels := make([]string, 0, len(pageSizeOptions))
		chosen := ""
		for _, opt := range pageSizeOptions {
			lbl := fmt.Sprintf(locales.PaginationPerPageOption(), formatInt(opt))
			labels = append(labels, lbl)
			if opt == size {
				chosen = lbl
			}
		}
		nextSizeLabel := chosen
		selectField(c, keyPrefix+"-page-size", &nextSizeLabel, labels, locales.PaginationPageSize()).
			Width(t.Space(40))
		if nextSizeLabel != chosen {
			for idx, lbl := range labels {
				if lbl == nextSizeLabel {
					*pageSize = pageSizeOptions[idx]
					*page = 0
					if onChange != nil {
						onChange()
					}
					break
				}
			}
		}
		prev := plainButton(c, locales.PaginationPrev())
		prev.Disabled(*page <= 0)
		if prev.Clicked() {
			*page--
			if onChange != nil {
				onChange()
			}
		}
		ui.Text(c, formatInt(*page+1)+" / "+formatInt(totalPages)).
			FontSize(captionSize(t)).TextColor(p.inkSecondary).FontFeatures("tnum").Shrink(0)
		next := plainButton(c, locales.PaginationNext())
		next.Disabled(*page >= totalPages-1)
		if next.Clicked() {
			*page++
			if onChange != nil {
				onChange()
			}
		}
	})
}

func textOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// kpiCardProps 描述桌面端总览与模块页 KPI 指标卡的展示参数。
type kpiCardProps struct {
	Key       string
	Title     string
	Value     string
	Subtitle  string
	BadgeText string
	BadgeTone badgeTone
	Clickable bool
}

// kpiCard 渲染 Sunbeam Flat 风格的数据台账指标卡。
// 已核对 MyGo v0.10.0 与 sunbeam v0.2.0，两者均无内置 KPI 卡片控件（ui.Badge 仅用于短标签、
// sunbeam.Card 为空白容器），因此在此统一封装数值等宽（tnum）、可选状态圆点徽标与整卡点击跳转。
func kpiCard(c *ui.Context, p uiPalette, props kpiCardProps) bool {
	t := p.theme
	renderBody := func() {
		ui.Text(c, props.Title).FontSize(captionSize(t)).FontWeight(500).
			TextColor(p.muted).FillWidth().SingleLine().Tooltip(props.Title)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, props.Value).FontSize(titleSize(t)).FontWeight(600).
				TextColor(p.ink).FontFeatures("tnum").SingleLine().Tooltip(props.Value).
				Grow(1).Shrink(1).MinWidth(0)
			if props.BadgeText != "" {
				badge(c, p, props.BadgeText, props.BadgeTone)
			}
		})
		if props.Subtitle != "" {
			ui.Text(c, props.Subtitle).FontSize(captionSize(t)).TextColor(p.muted).
				SingleLine().MinWidth(0).Tooltip(props.Subtitle)
		}
	}
	if !props.Clickable {
		panel(c, p).Grow(1).Shrink(1).MinWidth(0).Padding(t.Space(4), t.Space(4.5)).
			Gap(t.Space(1.5)).Children(renderBody)
		return false
	}
	key := props.Key
	if key == "" {
		key = props.Title
	}
	btn := ui.ButtonBase(c.Key("kpi-"+key)).Grow(1).Shrink(1).MinWidth(0).
		Background(p.panel).Border(1, p.border).Radius(sunbeam.Radius2XL).
		Padding(t.Space(4), t.Space(4.5)).Cursor(ui.CursorPointer).
		Label(props.Title).TextColor(p.ink)
	switch {
	case btn.Pressed():
		btn.Background(p.sb.FlatFacePressed).Border(1, p.borderStrong)
	case btn.Hovered():
		btn.Background(p.panelHover).Border(1, p.borderStrong)
	}
	btn.Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(1.5)).Children(renderBody)
	})
	return btn.Clicked()
}

// returnContextBanner 渲染独立「数据导入」页顶部的来源上下文与返回条。
// 已核对 MyGo v0.10.0（ui.Breadcrumbs 仅表达层级路径，不承载任务目标与动作返回按钮），
// 在此统一封装导入上下文说明与单次点击返回原页面的能力。
func returnContextBanner(c *ui.Context, p uiPalette, contextText, returnLabel string) bool {
	t := p.theme
	clicked := false
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).
		Background(p.panel).Border(1, p.border).Radius(sunbeam.RadiusXL).
		Padding(t.Space(2.5), t.Space(4)).Children(func() {
		ui.Icon(c, iconImport).Size(16, 16).TextColor(p.muted).Shrink(0)
		ui.Text(c, contextText).FontSize(bodySize(t)).FontWeight(500).TextColor(p.ink).
			Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(contextText)
		if returnLabel != "" {
			if labelledButton(c, returnLabel, iconArrowLeft, false).Clicked() {
				clicked = true
			}
		}
	})
	return clicked
}

// filterPillOption 描述轻量筛选胶囊条的一个可选项。
type filterPillOption struct {
	Key      string
	Label    string
	Count    int
	HasCount bool
}

// filterPillBar 渲染带可选计数的单选筛选胶囊条。
// 已核对 MyGo v0.10.0（ui.Segmented 强制等宽不适合长短不一的计数筛选、ui.ToggleGroup 为无样式组），
// 在此使用 ui.ToggleBase + Sunbeam 暖墨选中态实现零下拉单点筛选。
func filterPillBar(c *ui.Context, p uiPalette, active *string, options []filterPillOption, label string) bool {
	t := p.theme
	changed := false
	ui.Row(c).Wrap().AlignItems(ui.Center).Gap(t.Space(1.5)).
		Role(ui.RoleGroup).Label(label).Children(func() {
		for _, opt := range options {
			opt := opt
			selected := *active == opt.Key
			next := selected
			accessName := opt.Label
			if opt.HasCount {
				accessName = fmt.Sprintf("%s (%s)", opt.Label, formatInt(opt.Count))
			}
			pill := ui.ToggleBase(c.Key("fpill-"+opt.Key), &next).
				Height(34).Padding(0, 14).Radius(17).Gap(6).
				Center().Cursor(ui.CursorPointer).Label(accessName)
			switch {
			case selected:
				pill.Background(p.ink).Border(1, p.ink).TextColor(p.canvas)
			case pill.Pressed():
				pill.Background(p.sb.FlatFacePressed).Border(1, p.border).TextColor(p.ink)
			case pill.Hovered():
				pill.Background(p.hover).Border(1, p.border).TextColor(p.ink)
			default:
				pill.Background(p.fill).Border(1, ui.Transparent).TextColor(p.inkSecondary)
			}
			pill.Children(func() {
				ui.Text(c, opt.Label).FontSize(captionSize(t)).FontWeight(500).SingleLine()
				if opt.HasCount {
					countColor := p.muted
					if selected {
						countColor = p.canvas.Alpha(0.75)
					}
					ui.Text(c, formatInt(opt.Count)).FontSize(captionSize(t)).
						FontFeatures("tnum").TextColor(countColor).SingleLine()
				}
			})
			applyMicroPress(pill)
			if next && !selected {
				*active = opt.Key
				changed = true
			}
		}
	})
	return changed
}

// fileDropZoneProps 描述文件拖拽与点击选择区域的配置。
type fileDropZoneProps struct {
	Key         string
	Title       string
	ActiveTitle string
	Hint        string
	ButtonLabel string
}

// fileDropZone 渲染支持桌面文件拖拽与点击浏览的上传区，底层直接使用 MyGo v0.10.0 原生
// Element.FileDragOver 与 Element.DroppedFiles，并在拖拽悬停时展示 Sunbeam 焦点边框提示。
func fileDropZone(c *ui.Context, p uiPalette, props fileDropZoneProps) (clicked bool, dropped []string) {
	t := p.theme
	key := props.Key
	if key == "" {
		key = "file-drop-zone"
	}
	zone := ui.ButtonBase(c.Key(key)).FillWidth().MinHeight(t.Space(36)).
		Radius(sunbeam.Radius2XL).Padding(t.Space(6), t.Space(5)).
		Center().Cursor(ui.CursorPointer).Label(textOr(props.ButtonLabel, props.Title)).
		TextColor(p.ink)
	dragOver := zone.FileDragOver()
	dropped = zone.DroppedFiles()
	switch {
	case dragOver:
		zone.Background(p.accentSoft).Border(2, p.sb.Ring)
	case zone.Pressed():
		zone.Background(p.sb.FlatFacePressed).Border(1, p.borderStrong)
	case zone.Hovered():
		zone.Background(p.panelHover).Border(1, p.borderStrong)
	default:
		zone.Background(p.panel).Border(1, p.borderStrong)
	}
	zone.Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			iconColor := p.muted
			heading := props.Title
			if dragOver {
				iconColor = p.ink
				if props.ActiveTitle != "" {
					heading = props.ActiveTitle
				}
			}
			ui.Icon(c, iconUpload).Size(28, 28).TextColor(iconColor)
			ui.Text(c, heading).FontSize(bodySize(t)).FontWeight(600).
				TextColor(p.ink).TextAlign(ui.Center)
			if props.Hint != "" {
				ui.Text(c, props.Hint).FontSize(captionSize(t)).TextColor(p.muted).
					TextAlign(ui.Center).Pretty().MaxWidth(t.Space(140))
			}
			if props.ButtonLabel != "" {
				ui.Row(c).Margin(t.Space(1), 0, 0, 0).Padding(0, 16).Height(34).Radius(17).
					AlignItems(ui.Center).Gap(6).Background(p.fill).TextColor(p.ink).Children(func() {
					ui.Icon(c, iconFolder).Size(14, 14)
					ui.Text(c, props.ButtonLabel).FontSize(captionSize(t)).FontWeight(500).SingleLine()
				})
			}
		})
	})
	return zone.Clicked(), dropped
}

// detailInspectorHeader 渲染右侧分栏或抽屉顶部的统一标题、副标题与关闭按钮。
func detailInspectorHeader(c *ui.Context, p uiPalette, title, subtitle string, onClose func()) {
	t := p.theme
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(sectionSize(t)).FontWeight(600).TextColor(p.ink).
				SingleLine().Tooltip(title)
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(captionSize(t)).TextColor(p.muted).
					SingleLine().Tooltip(subtitle)
			}
		})
		if onClose != nil {
			if modalCloseButton(c, p.sb).Clicked() {
				onClose()
			}
		}
	})
}

const sideDrawerDefaultWidth float32 = 520

// sideDrawerProps 描述右侧滑出详情抽屉的属性。
type sideDrawerProps struct {
	Key      string
	Title    string
	Subtitle string
	Width    float32
}

// sideDrawer 渲染右侧滑出浮层抽屉，保持主表上下文可见。
// 已核对 MyGo v0.10.0（仅提供 DialogBase/Modal/Overlay/SplitPercent，尚无内置 Drawer/Sheet 控件），
// 因此基于 ui.DialogBase + sunbeam.PanelSurface + Layer(Raised) 实现，支持 Esc/遮罩关闭与对称滑入滑出动效。
func sideDrawer(c *ui.Context, open *bool, props sideDrawerProps, body func(), footer func()) {
	t := *c.Theme()
	sb := sunbeam.Colors(c)
	p := uiPaletteFor(c)
	key := props.Key
	if key == "" {
		key = props.Title
	}
	state := ui.Local(ui.Box(c.Key("drawer-"+key)).Absolute().Size(0, 0), "drawer", func() dialogState {
		return dialogState{}
	})
	if *open {
		state.shown, state.closing = true, false
	} else if state.shown {
		state.closing = true
	}
	if !state.shown {
		return
	}

	done := false
	ui.DialogBase(c, &state.shown, func(backdrop, drawerPanel ui.Element) {
		if !state.entered {
			state.entered = true
			drawerPanel.Jump("shown", 0)
			backdrop.Jump("dim", 0)
		}
		target := float32(1)
		show := ui.Animation{Duration: sunbeam.DurationDrawer, Ease: sunbeamEaseDrawer}
		if state.closing {
			target = 0
			show = ui.Animation{Duration: sunbeam.DurationDrawerExit, Ease: sunbeamEaseDrawer}
			backdrop.IgnoreInput(true)
		}
		shown := drawerPanel.AnimateTo("shown", target, show)
		dim := backdrop.AnimateTo("dim", target, show)
		done = state.closing && shown <= 0 && dim <= 0

		backdrop.Padding(t.Space(3)).AlignItems(ui.End).Justify(ui.Center).
			Background(sb.Scrim.Alpha(0.24 * dim))
		width := props.Width
		if width == 0 {
			width = sideDrawerDefaultWidth
		}
		sunbeam.PanelSurface(c, drawerPanel)
		drawerPanel.Width(width).MaxWidthPercent(92).FillHeight().Clip().
			TextColor(sb.Foreground).FontSize(bodySize(t)).
			Translate((1-shown)*28, 0).Opacity(shown)

		drawerPanel.Children(func() {
			sunbeam.Layer(sunbeam.Raised, func() {
				ui.Column(c).Fill().Children(func() {
					ui.Column(c).FillWidth().Padding(16, 20, 12, 20).Children(func() {
						detailInspectorHeader(c, p, props.Title, props.Subtitle, func() {
							*open = false
						})
					})
					rule(c)
					ui.Scroll(c).Grow(1).Shrink(1).MinHeight(0).Children(func() {
						ui.Column(c).FillWidth().Padding(16, 20).Gap(t.Space(3)).Children(body)
					})
					if footer != nil {
						rule(c)
						ui.Row(c).FillWidth().Wrap().Justify(ui.End).AlignItems(ui.Center).
							Gap(t.Space(2)).Padding(12, 20, 16, 20).Children(footer)
					}
				})
			})
		})
	})

	switch {
	case !state.shown:
		*open, state.shown, state.closing = false, true, true
		c.Invalidate()
	case done:
		*state = dialogState{}
		c.Invalidate()
	}
}

// deltaText 渲染带语义色与等宽数字的差值文本：direction > 0 为上升/改善（绿），< 0 为下降（红），0 为持平（次要灰）。
func deltaText(c *ui.Context, p uiPalette, text string, direction int) ui.Element {
	t := p.theme
	color := p.muted
	weight := 400
	if direction > 0 {
		color = p.success
		weight = 600
	} else if direction < 0 {
		color = p.danger
		weight = 600
	}
	return ui.Text(c, text).FontSize(bodySize(t)).FontWeight(weight).
		FontFeatures("tnum").TextColor(color).SingleLine()
}

type unsavedGuardAction int

const (
	unsavedGuardNone unsavedGuardAction = iota
	unsavedGuardStay
	unsavedGuardDiscard
	unsavedGuardSave
)

// unsavedGuardModal 是离开含未保存草稿页面时的三态保护弹窗（留在当前页 / 放弃修改 / 保存并继续）。
// 默认焦点置于「留在当前页」，防止回车误丢弃草稿。
func unsavedGuardModal(c *ui.Context, p uiPalette, key string, open *bool, title, body string) unsavedGuardAction {
	t := p.theme
	action := unsavedGuardNone
	modal(c, open, modalProps{
		Key:      key,
		Title:    textOr(title, locales.UnsavedGuardTitle()),
		Width:    modalWidthCompact,
		NoFooter: true,
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			ui.Text(c, textOr(body, locales.UnsavedGuardBody())).
				FontSize(bodySize(t)).TextColor(p.inkSecondary).Pretty()
			ui.Row(c).FillWidth().Wrap().Justify(ui.End).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				if plainButton(c, locales.UnsavedGuardStay()).AutoFocus().Clicked() {
					action = unsavedGuardStay
					*open = false
				}
				if dangerButton(c, locales.UnsavedGuardDiscard()).Clicked() {
					action = unsavedGuardDiscard
					*open = false
				}
				if primaryButton(c, locales.UnsavedGuardSave()).Clicked() {
					action = unsavedGuardSave
					*open = false
				}
			})
		})
	})
	return action
}
