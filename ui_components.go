package main

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/form"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/motion"
	"github.com/lobehub/lobe-mygo/pkg/theme"

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

func panel(c *ui.Context, p uiPalette) ui.Element {
	return ui.Column(c).
		Background(p.panel).
		Border(1, p.border).
		Radius(p.theme.Space(2))
}

// section 是页面内的一个平面分区。通过统一的分割线与间距划分层次，
// 不再给每个分区套一层卡片边框。
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

func pageHeading(c *ui.Context, p uiPalette, title, subtitle string, actions func()) {
	t := p.theme
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.End).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Text(c, title).FontSize(titleSize(t)).FontWeight(600).LetterSpacing(-0.2).
					Role(ui.RoleHeading).Level(1)
				if subtitle != "" {
					ui.Text(c, subtitle).FontSize(bodySize(t)).TextColor(p.muted).Pretty()
				}
			})
			if actions != nil {
				ui.Row(c).Gap(t.Space(2)).Children(actions)
			}
		})
	})
}

func sectionHeading(c *ui.Context, p uiPalette, title string, count int) {
	t := p.theme
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		ui.Text(c, title).FontSize(sectionSize(t)).FontWeight(600).LetterSpacing(-0.2)
		if count >= 0 {
			ui.Textf(c, "%d", count).FontSize(captionSize(t)).TextColor(p.muted).
				FontFeatures("tnum")
		}
	})
}

func listPanel(c *ui.Context, p uiPalette, title string, count int, body func()) {
	t := p.theme
	ui.Column(c).Fill().Role(ui.RoleComplementary).Label(title).
		Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, title).FontSize(bodySize(t)).FontWeight(600)
			ui.Textf(c, "%d", count).FontSize(captionSize(t)).TextColor(p.muted).
				FontFeatures("tnum")
		})
		body()
	})
}

func badge(c *ui.Context, p uiPalette, text string, tone badgeTone) {
	t := p.theme
	accent, soft := p.inkSecondary, p.panelMuted
	switch tone {
	case toneAccent:
		accent, soft = p.accent, p.accentSoft
	case toneSuccess:
		accent, soft = p.success, p.successSoft
	case toneWarning:
		accent, soft = p.warning, p.warningSoft
	case toneDanger:
		accent, soft = p.danger, p.dangerSoft
	}
	ui.Row(c).Height(t.Space(5)).PaddingX(t.Space(2)).Radius(t.Space(1)).
		AlignItems(ui.Center).Background(soft).Children(func() {
		ui.Text(c, text).FontSize(captionSize(t)).FontWeight(500).
			TextColor(accent).SingleLine()
	})
}

func statusBadge(c *ui.Context, p uiPalette, status string, label string) {
	switch status {
	case "published", "calculated", "ready":
		badge(c, p, label, toneSuccess)
	case "imported":
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
	ui.Column(c).Gap(0).Children(func() {
		ui.Text(c, value).FontSize(titleSize(t)).FontWeight(600).FontFeatures("tnum")
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted)
	})
}

// metricInline 把一组数量排成一行，数值跟随正文字号，标签使用次要文字颜色。
func metricInline(c *ui.Context, p uiPalette, value uint64, label string) {
	t := p.theme
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
		ui.Textf(c, "%d", value).FontSize(bodySize(t)).FontWeight(600).FontFeatures("tnum")
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted)
	})
}

func notice(c *ui.Context, p uiPalette, tone noticeTone, text string) {
	t := p.theme
	accent, icon, background := p.info, lucide.Info, p.infoSoft
	switch tone {
	case noticeSuccess:
		accent, icon, background = p.success, lucide.CircleCheck, p.successSoft
	case noticeWarning:
		accent, icon, background = p.warning, lucide.TriangleAlert, p.warningSoft
	case noticeDanger:
		accent, icon, background = p.danger, lucide.CircleX, p.dangerSoft
	}
	ui.Row(c).Gap(t.Space(2)).AlignSelf(ui.Stretch).AlignItems(ui.Start).
		Radius(t.Space(2)).Padding(t.Space(3), t.Space(4)).
		Background(background).Border(1, accent.Alpha(0.18)).Children(func() {
		ui.Row(c).Height(t.Space(6)).AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Icon(c, icon()).FontSize(t.Space(4)).TextColor(accent)
		})
		ui.Text(c, text).FontSize(bodySize(t)).TextColor(p.ink).Pretty().Selectable()
	})
}

func emptyState(c *ui.Context, p uiPalette, icon *ui.SVG, title, body string, action func()) {
	t := p.theme
	ui.Column(c).Grow(1).FillWidth().Center().Padding(t.Space(8)).Children(func() {
		ui.Column(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
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

func labelledButton(c *ui.Context, label string, icon *ui.SVG, primary bool) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	props := action.ButtonProps{Variant: action.Default}
	if primary {
		props.Variant = action.Primary
	}
	element := action.StyleButton(c, ui.ButtonBase(c), "", props)
	element.Height(tk.ControlHeight).FontSize(bodySize(t)).Label(label).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(t.Space(4))
		}
		ui.Text(c, label).FontSize(bodySize(t)).SingleLine()
	})
	return element
}

// plainButton 是 Lobe MyGo 的次要按钮，带可访问名称。
func plainButton(c *ui.Context, label string) ui.Element {
	return standardButton(c, label, action.Default)
}

// primaryButton 是 Lobe MyGo 的主按钮，带可访问名称。
func primaryButton(c *ui.Context, label string) ui.Element {
	return standardButton(c, label, action.Primary)
}

// textButton 是无底色的 Lobe MyGo 按钮，用于列表行和返回这类轻量动作。
func textButton(c *ui.Context, label string) ui.Element {
	return standardButton(c, label, action.Text)
}

func standardButton(c *ui.Context, label string, variant action.ButtonVariant) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	element := action.StyleButton(c, ui.ButtonBase(c), "", action.ButtonProps{Variant: variant})
	element.Height(tk.ControlHeight).FontSize(bodySize(t)).Label(label).Children(func() {
		ui.Text(c, label).FontSize(bodySize(t)).SingleLine()
	})
	return element
}

// pageActionButton 是 44 高、16 字号的页面级主按钮。
func pageActionButton(c *ui.Context, label string) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	element := action.StyleButton(c, ui.ButtonBase(c), "", action.ButtonProps{Variant: action.Primary})
	element.Height(tk.ControlHeightLG).FontSize(bodySize(t)).Label(label).Children(func() {
		ui.Text(c, label).FontSize(bodySize(t)).SingleLine()
	})
	return element
}

func linkRow(c *ui.Context, p uiPalette, label string, body func()) ui.Element {
	t := p.theme
	element := action.StyleButton(c, ui.ButtonBase(c), "", action.ButtonProps{Variant: action.Text}).
		FillWidth().Label(label).MinHeight(t.Space(9)).
		Padding(t.Space(2), t.Space(3)).Radius(t.Space(1.5))
	if element.Hovered() {
		element.Background(p.hover)
	}
	element.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(body)
	})
	return element
}

// rule 是页面里的横向分割线，统一使用 Lobe MyGo 的分割线颜色。
func rule(c *ui.Context) {
	layout.Divider(c, layout.DividerProps{})
}

// selectField 是 Lobe MyGo 的下拉选择，选项文字同时作为取值。
// label 是下拉的控件级名称，读屏软件随当前选中的选项一起读出。
// key 必须全应用唯一：弹窗 overlay 节点按位置继承保留状态，后打开的下拉会占用
// 先关闭下拉留下的 panel 节点，带着它正在进行的过渡和上次的尺寸，把选项裁掉。
// 给 panel 上独立的 Key 后每个下拉的弹窗是全新节点，按自己的内容布局。
func selectField(c *ui.Context, key string, value *string, labels []string, label string) ui.Element {
	if len(labels) == 0 {
		labels = []string{""}
	}
	chosen := 0
	for index, item := range labels {
		if item == *value {
			chosen = index
		}
	}
	current := strconv.Itoa(chosen)
	t := *c.Theme()
	tk := theme.TokensOf(c)
	sel := ui.SelectBase(c, &current)
	trigger := sel.Trigger.Height(tk.ControlHeight).Padding(t.Space(1), t.Space(3)).
		Gap(t.Space(2)).Radius(tk.BorderRadius).FontSize(bodySize(t)).
		AlignItems(ui.Center).TextColor(tk.Text).Cursor(ui.CursorPointer).Unselectable().
		Transition(selectTransition).Background(tk.BgContainer).Border(1, ui.Transparent).
		Label(label).Value(labels[chosen])
	trigger.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, labels[chosen]).SingleLine()
		})
		turn := float32(0)
		if sel.Open() {
			turn = 180
		}
		arrow := ui.Icon(c, lucide.ChevronDown()).FontSize(t.Space(4)).TextColor(tk.TextSecondary).Shrink(0)
		arrow.Rotate(arrow.AnimateTo("turn", turn, selectArrowTurn))
	})
	width := trigger.Bounds().W
	sel.Popup(func(panel ui.Element) {
		panel.Key(key)
		panel.Margin(t.Space(1), 0, 0, 0).Padding(t.Space(1)).Radius(tk.BorderRadius).MinWidth(width).
			Background(tk.BgElevated).Transition(motion.Popup()).AttachedOrigin()
		tk.Elevate(panel, theme.BoxShadowSecondary)
		// 选项多时滚动，避免列表超出窗口。
		ui.Scroll(c).MaxHeight(t.Space(64)).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for index, item := range labels {
					index := index
					selected := index == chosen
					opt := sel.Item(strconv.Itoa(index)).Label(item).
						Padding(t.Space(1.5), t.Space(3)).Height(tk.ControlHeight).
						Gap(t.Space(2)).AlignItems(ui.Center).Radius(tk.BorderRadiusSM).
						AlignSelf(ui.Stretch).FontSize(bodySize(t)).TextColor(tk.Text)
					if opt.Highlighted() {
						opt.Background(tk.FillTertiary)
					}
					opt.Children(func() {
						ui.Text(c, item).Grow(1).SingleLine()
						if selected {
							ui.Icon(c, lucide.Check()).FontSize(t.Space(4)).TextColor(tk.Primary).
								Margin(0, 0, 0, t.Space(2))
						}
					})
				}
			})
		})
	})
	if index, err := strconv.Atoi(current); err == nil && index != chosen && index < len(labels) {
		*value = labels[index]
	}
	return trigger
}

var (
	selectTransition = ui.ElementTransition{Duration: 150 * time.Millisecond, Ease: motion.EaseOut, Colors: true}
	selectArrowTurn  = ui.Animation{Duration: 150 * time.Millisecond, Ease: motion.EaseOut}
	inputTransition  = ui.ElementTransition{Duration: 150 * time.Millisecond, Ease: motion.EaseOut, Colors: true}
	segmentSlide     = ui.ElementTransition{Duration: 240 * time.Millisecond, Ease: motion.EaseOut}
	segmentPress     = ui.Animation{Duration: 120 * time.Millisecond, Ease: motion.EaseOut}
)

func inputRow(c *ui.Context) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	return ui.Row(c).Height(tk.ControlHeight).PaddingX(t.Space(3)).
		Radius(tk.BorderRadius).AlignItems(ui.Center).Gap(t.Space(2)).
		FontSize(bodySize(t)).TextColor(tk.Text).Cursor(ui.CursorText).
		Transition(inputTransition)
}

// searchField 是 36 高、16 字号的搜索框。
func searchField(c *ui.Context, value *string, label, placeholder string) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	return ui.SearchField(c, value).Label(label).Placeholder(placeholder).
		MinHeight(tk.ControlHeight).FontSize(bodySize(t))
}

// dateField 是 36 高、16 字号的日期选择框。
func dateField(c *ui.Context, value *time.Time, label string, props field.DatePickerProps) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	return field.DatePicker(c, value, props).Label(label).
		Height(tk.ControlHeight).FontSize(bodySize(t)).Radius(tk.BorderRadius)
}

func styleInputRow(root ui.Element, tk *theme.Tokens, focused bool) {
	hovered := root.Hovered() && !focused && !root.IsDisabled()
	border := tk.BorderSecondary
	if hovered {
		border = tk.Border
	}
	if focused {
		border = tk.Primary
	}
	root.Background(tk.BgContainer).Border(1, border)
}

// textField 是 36 高、16 字号的单行输入框。
func textField(c *ui.Context, value *string, label, placeholder string) ui.Element {
	tk := theme.TokensOf(c)
	root := inputRow(c)
	var input ui.Element
	root.Children(func() {
		input = ui.TextInputBase(c, value).Grow(1).Height(tk.ControlHeight).Placeholder(placeholder).
			PlaceholderColor(tk.TextPlaceholder)
	})
	styleInputRow(root, tk, input.Focused())
	input.Label(label)
	return input
}

// textAreaField 是 16 字号的多行输入框，行数由调用点控制。
func textAreaField(c *ui.Context, value *string, label, placeholder string, minRows, maxRows int) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	root := ui.Column(c).Padding(t.Space(2), t.Space(3)).Radius(tk.BorderRadius).
		FontSize(bodySize(t)).TextColor(tk.Text).Cursor(ui.CursorText).
		Transition(inputTransition)
	var area ui.Element
	root.Children(func() {
		area = ui.TextAreaBase(c, value).Lines(minRows, maxRows).FocusRing(false).
			Placeholder(placeholder).PlaceholderColor(tk.TextPlaceholder)
	})
	styleInputRow(root, tk, area.Focused())
	area.Label(label)
	return area
}

// passwordField 是 36 高、16 字号的密码输入框。
func passwordField(c *ui.Context, value *string, label, placeholder string) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	root := inputRow(c)
	visible := ui.Local(root, "visible", func() bool { return false })
	var input ui.Element
	root.Children(func() {
		input = ui.TextInputBase(c, value).Grow(1).MinWidth(0).Height(tk.ControlHeight).
			Placeholder(placeholder).PlaceholderColor(tk.TextPlaceholder)
		if !*visible {
			input.Password()
		}
		text, glyph := locales.ShowPassword(), lucide.EyeOff()
		if *visible {
			text, glyph = locales.HidePassword(), lucide.Eye()
		}
		toggle := ui.ButtonBase(c).Label(text).Size(t.Space(7), t.Space(7)).
			Radius(t.Space(1.5)).Center().Cursor(ui.CursorPointer).
			TextColor(tk.TextTertiary)
		if toggle.Hovered() {
			toggle.TextColor(tk.Text)
		}
		if toggle.Clicked() {
			*visible = !*visible
		}
		toggle.Children(func() {
			ui.Icon(c, glyph).FontSize(t.Space(4))
		})
	})
	styleInputRow(root, tk, input.Focused())
	input.Label(label)
	return input
}

type numberDraft struct {
	text    string
	focused bool
}

// numberField 是 36 高、16 字号的数字输入框。
func numberField(c *ui.Context, value *float64, lo, hi, step float64, label string) ui.Element {
	tk := theme.TokensOf(c)
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
		input = ui.TextInputBase(c, &draft.text).Grow(1).MinWidth(0).Height(tk.ControlHeight).
			PlaceholderColor(tk.TextPlaceholder).FontFeatures("tnum")
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
			if numberControl(c, lucide.ChevronUp(), locales.Increment()) {
				set(*value + step)
				draft.text = format(*value)
			}
			if numberControl(c, lucide.ChevronDown(), locales.Decrement()) {
				set(*value - step)
				draft.text = format(*value)
			}
		})
	})
	styleInputRow(root, tk, input.Focused())
	input.Label(label)
	return input
}

func numberControl(c *ui.Context, glyph *ui.SVG, label string) bool {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	button := ui.ButtonBase(c).Label(label).Size(t.Space(7), t.Space(4.5)).
		Radius(t.Space(1)).Center().Cursor(ui.CursorPointer).
		TextColor(tk.TextTertiary)
	if button.Hovered() {
		button.Background(tk.FillSecondary).TextColor(tk.Text)
	}
	button.Children(func() {
		ui.Icon(c, glyph).FontSize(t.Space(4))
	})
	return button.Clicked()
}

// segmentedField 是 36 高、16 字号的分段选择。
func segmentedField(c *ui.Context, selected *int, labels []string, label string) ui.Element {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	parts := ui.SegmentedBase(c, selected, len(labels))
	track := parts.Track.Height(tk.ControlHeight).Padding(t.Space(1)).
		Gap(t.Space(1)).Radius(tk.BorderRadiusLG).
		Background(tk.BgLayout).Border(1, tk.FillQuaternary)
	track.Label(label)
	track.Children(func() {
		for index, option := range labels {
			segment := parts.Segment(index).FillHeight().PaddingX(t.Space(4)).
				Radius(tk.BorderRadius).FontSize(bodySize(t)).FontWeight(500).
				Cursor(ui.CursorPointer)
			color := tk.TextSecondary
			if index == *selected || segment.Hovered() {
				color = tk.Text
			}
			pose := ui.Pose{}
			if segment.Pressed() {
				pose.Scale = 0.98
			}
			segment.TextColor(color).AnimatePose(pose, segmentPress)
			segment.Children(func() {
				if index == *selected {
					ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).
						Radius(tk.BorderRadius).Background(tk.BgElevated).
						ZIndex(-1).Transition(segmentSlide)
				}
				ui.Text(c, option).SingleLine()
			})
		}
	})
	return track
}

// switchField 是 Lobe MyGo 的开关。
func switchField(c *ui.Context, value *bool, label string) ui.Element {
	return choice.Switch(c, value, choice.SwitchProps{}).Label(label)
}

// checkboxField 是 Lobe MyGo 的复选框。
func checkboxField(c *ui.Context, value *bool, label string) ui.Element {
	return choice.Checkbox(c, value, label, choice.CheckboxProps{}).Label(label)
}

// dialogFormState 初始化弹窗表单的布局状态。
func (a *app) dialogFormState() *form.Form[struct{}] {
	if a.dialogFormHost == nil {
		a.dialogFormHost = form.New(struct{}{})
	}
	return a.dialogFormHost
}

// dialogForm 用 Lobe 的表单视图承载弹窗里的字段，标签在控件上方。
func (a *app) dialogForm(c *ui.Context, body func()) {
	form.View(c, a.dialogFormState(), form.Props[struct{}]{
		Variant: form.Borderless,
		Layout:  form.Vertical,
	}, body)
}

// dialogField 是弹窗表单里的一个字段，标签、说明与控件都使用 Lobe 的排版。
// name 用来区分同一个表单里的字段状态。
func (a *app) dialogField(c *ui.Context, name, label, desc string, control func()) {
	form.Field(c, a.dialogFormState(), form.FieldProps{
		Name: name, Label: label, Desc: desc, Value: label, Layout: form.Vertical,
	}, control)
}

const (
	modalWidthCompact float32 = 400
	modalDefaultWidth float32 = 520
	modalWidthWide    float32 = 680
)

type modalProps struct {
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

func modal(c *ui.Context, open *bool, p modalProps, content func()) bool {
	t := *c.Theme()
	tk := theme.TokensOf(c)
	enter := motion.Modal()
	fade := motion.Backdrop()
	state := ui.Local(ui.Box(c).Absolute().Size(0, 0), "dialog", func() dialogState {
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
		show := ui.Animation{Duration: enter.Duration, Ease: enter.Ease}
		if state.closing {
			target = 0
			show = ui.Animation{Duration: enter.ExitDuration, Ease: enter.ExitEase}
			backdrop.IgnoreInput(true)
		}
		shown := panel.AnimateTo("shown", target, show)
		dim := backdrop.AnimateTo("dim", target, ui.Animation{Duration: fade.Duration, Ease: fade.Ease})
		done = state.closing && shown <= 0 && dim <= 0

		backdrop.Padding(t.Space(8), t.Space(4)).
			Background(tk.BgContainer.Alpha(0.6 * dim))
		width := p.Width
		if width == 0 {
			width = modalDefaultWidth
		}
		from := enter.Enter.Scale
		if state.closing {
			from = enter.Exit.Scale
		}
		scale := from + (1-from)*shown
		panel.Width(width).MaxWidthPercent(100).MaxHeightPercent(100).
			Radius(t.Space(3)).Clip().Background(tk.BgElevated).
			TextColor(tk.Text).FontSize(bodySize(t)).
			Scale(scale, scale).Opacity(shown)
		tk.Elevate(panel, theme.BoxShadow)

		panel.Children(func() {
			if p.Title != "" {
				ui.Row(c).MinHeight(t.Space(10)).Padding(t.Space(2), t.Space(4)).
					AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Text(c, p.Title).FontSize(sectionSize(t)).FontWeight(600).
						Grow(1).SingleLine()
					if modalCloseButton(c, tk).Clicked() {
						*open = false
					}
				})
			}
			ui.Scroll(c).Shrink(1).Children(func() {
				contentColumn := ui.Column(c).
					Padding(0, t.Space(4), t.Space(4), t.Space(4))
				if p.Title == "" {
					contentColumn.Padding(t.Space(4))
				}
				contentColumn.Children(content)
			})
			if !p.NoFooter {
				ui.Row(c).Gap(t.Space(2)).Padding(t.Space(3), t.Space(4)).
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

func modalCloseButton(c *ui.Context, tk *theme.Tokens) ui.Element {
	t := *c.Theme()
	button := ui.ButtonBase(c).Label(locales.Close()).Size(t.Space(9), t.Space(9)).
		Radius(t.Space(2)).Center().Cursor(ui.CursorPointer).
		TextColor(tk.TextTertiary)
	if button.Hovered() {
		button.Background(tk.FillSecondary).TextColor(tk.Text)
	}
	button.Children(func() {
		ui.Icon(c, lucide.X()).FontSize(t.Space(4))
	})
	return button
}

func textOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
