package main

import (
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/form"
	"github.com/lobehub/lobe-mygo/pkg/layout"

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
		Radius(p.theme.Space(2.5))
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
	color := display.TagGray
	switch tone {
	case toneAccent:
		color = display.TagProcessing
	case toneSuccess:
		color = display.TagSuccess
	case toneWarning:
		color = display.TagWarning
	case toneDanger:
		color = display.TagError
	}
	display.Tag(c, text, display.TagProps{Color: color})
}

func statusBadge(c *ui.Context, p uiPalette, status string, label string) {
	switch status {
	case "published", "calculated", "ready":
		badge(c, p, label, toneSuccess)
	case "imported":
		badge(c, p, label, toneAccent)
	case "draft", "pending":
		badge(c, p, label, toneWarning)
	default:
		badge(c, p, label, toneNeutral)
	}
}

func kindBadge(c *ui.Context, p uiPalette, kind string, label string) {
	if kind == "system" {
		tint := ui.Hex("#DDE5FF")
		text := ui.Hex("#1B3390")
		if p.theme.Dark {
			tint = ui.Hex("#25345F")
			text = ui.Hex("#C7D4FE")
		}
		ui.Row(c).Height(p.theme.Space(5)).PaddingX(p.theme.Space(2)).Radius(p.theme.Space(1)).
			AlignItems(ui.Center).Background(tint).Children(func() {
			ui.Text(c, label).FontSize(captionSize(p.theme)).FontWeight(500).TextColor(text).SingleLine()
		})
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
	ui.Row(c).AlignItems(ui.Center).Gap(0.75).Children(func() {
		ui.Textf(c, "%d", value).FontSize(bodySize(t)).FontWeight(600).FontFeatures("tnum")
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted)
	})
}

func notice(c *ui.Context, p uiPalette, tone noticeTone, text string) {
	kind := feedback.Info
	switch tone {
	case noticeSuccess:
		kind = feedback.Success
	case noticeWarning:
		kind = feedback.Warning
	case noticeDanger:
		kind = feedback.Error
	}
	feedback.Alert(c, text, feedback.AlertProps{Type: kind, Selectable: true})
}

func emptyState(c *ui.Context, p uiPalette, icon *ui.SVG, title, body string, action func()) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(p.theme.Space(8)).Children(func() {
		display.Empty(c, display.EmptyProps{
			Icon:        icon,
			Title:       title,
			Description: body,
			Action:      action,
		})
	})
}

func smallEmptyState(c *ui.Context, p uiPalette, body string) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(p.theme.Space(6)).Children(func() {
		display.Empty(c, display.EmptyProps{Title: body})
	})
}

func labelledButton(c *ui.Context, label string, icon *ui.SVG, primary bool) ui.Element {
	props := action.ButtonProps{Variant: action.Default}
	if primary {
		props.Variant = action.Primary
	}
	element := action.StyleButton(c, ui.ButtonBase(c), "", props)
	element.Label(label).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(c.Theme().Space(4))
		}
		ui.Text(c, label).FontSize(bodySize(*c.Theme())).SingleLine()
	})
	return element
}

// plainButton 是 Lobe MyGo 的次要按钮，带可访问名称。
func plainButton(c *ui.Context, label string) ui.Element {
	return action.Button(c, label, action.ButtonProps{}).Label(label)
}

// primaryButton 是 Lobe MyGo 的主按钮，带可访问名称。
func primaryButton(c *ui.Context, label string) ui.Element {
	return action.Button(c, label, action.ButtonProps{Variant: action.Primary}).Label(label)
}

// textButton 是无底色的 Lobe MyGo 按钮，用于列表行和返回这类轻量动作。
func textButton(c *ui.Context, label string) ui.Element {
	return action.StyleButton(c, ui.ButtonBase(c), "", action.ButtonProps{Variant: action.Text}).Label(label)
}

func linkRow(c *ui.Context, p uiPalette, label string, body func()) ui.Element {
	t := p.theme
	element := action.StyleButton(c, ui.ButtonBase(c), "", action.ButtonProps{Variant: action.Text}).
		FillWidth().Label(label).Padding(t.Space(2.5), t.Space(3)).Radius(t.Space(1.5))
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
func selectField(c *ui.Context, value *string, labels []string, label string) ui.Element {
	chosen := 0
	for index, item := range labels {
		if item == *value {
			chosen = index
		}
	}
	current := strconv.Itoa(chosen)
	options := make([]field.SelectOption, 0, len(labels))
	for index, item := range labels {
		options = append(options, field.SelectOption{Value: strconv.Itoa(index), Label: item})
	}
	trigger := field.Select(c, &current, field.SelectProps{Options: options}).Label(label)
	if index, err := strconv.Atoi(current); err == nil && index != chosen && index < len(labels) {
		*value = labels[index]
	}
	return trigger
}

// textField 是 Lobe MyGo 的单行输入框。
func textField(c *ui.Context, value *string, label, placeholder string) ui.Element {
	return field.Input(c, value, field.InputProps{Placeholder: placeholder}).Label(label)
}

// numberField 是 Lobe MyGo 的数字输入框。
func numberField(c *ui.Context, value *float64, lo, hi, step float64, label string) ui.Element {
	return field.InputNumber(c, value, field.InputNumberProps{
		Min: lo, Max: hi, Step: step,
	}).Label(label)
}

// segmentedField 是 Lobe MyGo 的分段选择。
func segmentedField(c *ui.Context, selected *int, labels []string, label string) ui.Element {
	return action.Segmented(c, selected, labels, action.SegmentedProps{}).Label(label)
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
