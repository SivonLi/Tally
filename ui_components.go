package main

import (
	"fmt"

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

func panel(c *ui.Context, p uiPalette) ui.Element {
	return ui.Column(c).
		Background(p.panel).
		Border(1, p.border).
		Radius(p.theme.Space(3))
}

func (a *app) appBar(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Background(p.panel).Children(func() {
		ui.Row(c).Padding(t.Space(3), t.Space(7)).AlignItems(ui.Center).Gap(t.Space(4)).Children(func() {
			a.brand(c, p)
			ui.Spacer(c)
			a.mainNavigation(c, p)
			ui.Spacer(c)
			a.languageSwitch(c)
		})
		ui.Divider(c)
	}).Role(ui.RoleBanner)
}

func (a *app) brand(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2.5)).Children(func() {
		ui.Box(c).Size(t.Space(9), t.Space(9)).Radius(t.Space(2.5)).Background(p.accent).Center().Children(func() {
			ui.Icon(c, iconMark).FontSize(t.Space(5.5)).TextColor(p.accentText)
		})
		ui.Column(c).Gap(0).Children(func() {
			ui.Text(c, locales.AppTitle()).FontSize(t.Rem(1.25)).Bold().LetterSpacing(-0.2)
			ui.Text(c, locales.AppTagline()).FontSize(t.Rem(0.8)).TextColor(p.muted)
		})
	})
}

// mainNavigation 使用 TabsBase 自绘胶囊式页签。查过 Tabs、Segmented 与 Toolbar：
// Tabs 无法容纳图标与选中底色，Segmented 表达的是同一页内的视图切换，
// Toolbar 会在窗口变窄时把页签收进溢出菜单。
func (a *app) mainNavigation(c *ui.Context, p uiPalette) {
	t := p.theme
	items := []struct {
		label string
		icon  *ui.SVG
	}{
		{locales.PlanLibraryTitle(), iconPlans},
		{locales.BatchWorkbenchTitle(), iconBatches},
	}
	nav := ui.TabsBase(c, &a.tab, len(items))
	nav.List.Label(locales.MainNavigation()).Padding(t.Space(1)).Radius(t.Space(2.5)).
		Gap(t.Space(1)).Background(t.SurfaceHover).Children(func() {
		for index, item := range items {
			tab := nav.Tab(index).Padding(t.Space(1.5), t.Space(3)).Radius(t.Space(2)).
				Label(item.label).Background(ui.Transparent).Border(1, ui.Transparent)
			color := p.muted
			if a.tab == index {
				tab.Background(p.panel).Border(1, p.border)
				color = p.accent
			}
			tab.Children(func() {
				ui.Icon(c, item.icon).FontSize(t.Space(4)).TextColor(color)
				label := ui.Text(c, item.label).FontSize(t.Rem(0.92)).TextColor(color)
				if a.tab == index {
					label.Bold()
				}
			})
		}
	})
}

func (a *app) languageSwitch(c *ui.Context) {
	tags := []string{"en", "zh-Hans"}
	chosen := 0
	if a.language == "zh-Hans" {
		chosen = 1
	}
	next := chosen
	ui.Segmented(c, &next, i18n.Autonym("en"), i18n.Autonym("zh-Hans")).
		Label(locales.LanguageLabel())
	if next != chosen {
		a.language = tags[next]
		mygo.App.SetLocale(a.language)
	}
}

func pageHeading(c *ui.Context, p uiPalette, title, subtitle string, actions func()) {
	t := p.theme
	ui.Row(c).AlignItems(ui.End).Gap(t.Space(4)).Children(func() {
		ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
			ui.Text(c, title).FontSize(t.Rem(1.85)).Bold().LetterSpacing(-0.35).
				Role(ui.RoleHeading).Level(1)
			ui.Text(c, subtitle).FontSize(t.Rem(0.92)).TextColor(p.muted).Pretty()
		})
		if actions != nil {
			ui.Row(c).Gap(t.Space(2)).Children(actions)
		}
	})
}

func listPanel(c *ui.Context, p uiPalette, title string, count int, body func()) {
	t := p.theme
	panel(c, p).Fill().Role(ui.RoleComplementary).Label(title).
		Padding(t.Space(3)).Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, title).FontSize(t.Rem(0.95)).Bold()
			ui.Badge(c, fmt.Sprint(count)).Background(p.panelMuted).TextColor(p.muted)
		})
		ui.Divider(c)
		body()
	})
}

func sectionHeading(c *ui.Context, p uiPalette, title string, count int) {
	t := p.theme
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		ui.Text(c, title).FontSize(t.Rem(1.02)).Bold()
		if count >= 0 {
			ui.Badge(c, fmt.Sprint(count)).Background(p.panelMuted).TextColor(p.muted)
		}
	})
}

func badge(c *ui.Context, p uiPalette, text string, tone badgeTone) {
	background := p.panelMuted
	foreground := p.muted
	switch tone {
	case toneAccent:
		background = p.accentSoft
		foreground = p.accent
	case toneSuccess:
		background = p.successSoft
		foreground = p.success
	case toneWarning:
		background = p.warningSoft
		foreground = p.warning
	case toneDanger:
		background = p.dangerSoft
		foreground = p.danger
	}
	ui.Badge(c, text).Background(background).TextColor(foreground)
}

func statusBadge(c *ui.Context, p uiPalette, status string, label string) {
	switch status {
	case "published", "calculated":
		badge(c, p, label, toneSuccess)
	case "imported":
		badge(c, p, label, toneAccent)
	case "draft":
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
		ui.Text(c, value).FontSize(t.Rem(1.4)).Bold().FontFeatures("tnum")
		ui.Text(c, label).FontSize(t.Rem(0.8)).TextColor(p.muted)
	})
}

func notice(c *ui.Context, p uiPalette, tone noticeTone, text string) {
	t := p.theme
	background := p.accentSoft
	border := p.accent
	icon := iconAlert
	if tone == noticeSuccess {
		background = p.successSoft
		border = p.success
		icon = iconCheck
	} else if tone == noticeWarning {
		background = p.warningSoft
		border = p.warning
	} else if tone == noticeDanger {
		background = p.dangerSoft
		border = p.danger
	}
	ui.Row(c).AlignItems(ui.Start).Gap(t.Space(2)).Padding(t.Space(3)).
		Background(background).Border(1, border.Alpha(0.35)).Radius(t.Space(2)).
		Children(func() {
			ui.Icon(c, icon).FontSize(t.Space(4)).TextColor(border)
			ui.Text(c, text).FontSize(t.Rem(0.88)).TextColor(p.ink).Grow(1).Selectable().Pretty()
		})
}

func emptyState(c *ui.Context, p uiPalette, icon *ui.SVG, title, body string, action func()) {
	t := p.theme
	ui.Column(c).Grow(1).FillWidth().Center().Gap(t.Space(3)).Padding(t.Space(8)).Children(func() {
		ui.Box(c).Size(t.Space(14), t.Space(14)).Radius(t.Space(4)).Background(p.accentSoft).
			Center().Children(func() {
			ui.Icon(c, icon).FontSize(t.Space(6.5)).TextColor(p.accent)
		})
		ui.Text(c, title).FontSize(t.Rem(1.15)).Bold().TextAlign(ui.Center)
		if body != "" {
			ui.Text(c, body).FontSize(t.Rem(0.9)).TextColor(p.muted).TextAlign(ui.Center).
				MaxWidth(t.Space(88)).Pretty()
		}
		if action != nil {
			ui.Row(c).Children(action)
		}
	})
}

func smallEmptyState(c *ui.Context, p uiPalette, body string) {
	t := p.theme
	ui.Column(c).Grow(1).FillWidth().Center().Padding(t.Space(6)).Children(func() {
		ui.Text(c, body).FontSize(t.Rem(0.9)).TextColor(p.muted).TextAlign(ui.Center)
	})
}

func labelledButton(c *ui.Context, label string, icon *ui.SVG, primary bool) ui.Element {
	var element ui.Element
	if primary {
		element = ui.PrimaryButton(c, "")
	} else {
		element = ui.Button(c, "")
	}
	element.Label(label)
	element.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(c.Theme().Space(4))
		}
		ui.Text(c, label).SingleLine()
	})
	return element
}
