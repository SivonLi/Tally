package main

import (
	"context"
	"fmt"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tally/internal/school"
	"tally/internal/store"
	"tally/locales"
)

// settingsSidebarWidth 是设置窗口左侧分类的宽度。
const settingsSidebarWidth = 200

// openSettings 打开设置窗口；已打开时把它带到前台。
func (a *app) openSettings() {
	if a.win == nil {
		a.page = pageOverview
		a.settingsPageID = "school"
		return
	}
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.settingsPageID = "school"
	if !a.syncLoaded && a.store != nil {
		if s, err := a.store.GetSyncSettings(context.Background()); err == nil {
			a.syncSettings = s
		}
		a.loadSyncSecrets()
		a.syncLoaded = true
	}
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:                locales.NavSettings(),
		Width:                760,
		Height:               620,
		MinWidth:             640,
		MinHeight:            440,
		TitleBarStyle:        mygo.TitleBarHidden,
		TitleBarHeight:       int(topStripHeight),
		TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
		StateKey:             "tally-settings",
		BackgroundColor:      "light-dark(#faf8f5, #090706)",
		DisableMaximize:      true,
		DisableMinimize:      true,
		AutoHideMenuBar:      true,
		Content:              ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
	})
}

func (a *app) settingsWindow() *mygo.Window {
	return a.settingsWin
}

func (a *app) settingsView(c *ui.Context) {
	p := uiPaletteFor(c)
	t := p.theme
	pages := a.settingsPages()
	current := pages[0]
	for _, item := range pages {
		if item.id == a.settingsPageID {
			current = item
		}
	}
	top := t.Space(4)
	if c.TitleBar().Left > 0 {
		top = topStrip(c) + 4
	}
	root := ui.Row(c).Fill().AlignItems(ui.Stretch).Background(p.canvas)
	root.Children(func() {
		side := ui.Column(c).Width(settingsSidebarWidth).Shrink(0).FillHeight().
			Background(p.panelMuted).
			Padding(top, t.Space(3), t.Space(4), t.Space(3)).Gap(2).DragWindow()
		side.Children(func() {
			for _, item := range pages {
				item := item
				a.settingsNavItem(c, p, item, item.id == current.id)
			}
		})
		ui.Box(c).Width(1).FillHeight().Background(p.border).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(p.canvas).Children(func() {
			ui.Row(c).FillWidth().Height(topStrip(c)).Shrink(0).Background(p.canvas).
				Padding(0, t.Space(7)).AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Text(c, current.title).FontSize(bodySize(t)).FontWeight(600).
					SingleLine().MinWidth(0).Shrink(1).Tooltip(current.title)
			})
			rule(c)
			ui.Scroll(c).Clip().Grow(1).Shrink(1).MinHeight(0).FillWidth().
				Padding(t.Space(5), t.Space(7), t.Space(8), t.Space(7)).
				Children(func() {
					current.build(c, p)
				})
		})
	})
	a.restoreDialog(c, p)
	a.syncConflictDialog(c, p)
	if a.settingsToast != "" {
		c.Toast(a.settingsToast)
		a.settingsToast = ""
	}
	sunbeam.Toasts(c, locales.Close())
}

type settingsPage struct {
	id    string
	title string
	icon  *ui.SVG
	build func(c *ui.Context, p uiPalette)
}

func (a *app) settingsPages() []settingsPage {
	return []settingsPage{
		{"school", locales.SettingsSchool(), iconSchool, a.settingsSchoolSection},
		{"subjects", locales.SettingsSubjects(), iconSubjects, a.settingsSubjectsSection},
		{"data", locales.SettingsData(), iconDatabase, a.settingsDataSection},
		{"sync", locales.SettingsSync(), iconSync, a.settingsSyncSection},
		{"appearance", locales.SettingsAppearance(), iconAppearance, a.settingsAppearanceSection},
	}
}

func (a *app) settingsNavItem(c *ui.Context, p uiPalette, item settingsPage, selected bool) {
	t := p.theme
	button := ui.ButtonBase(c.Key(item.id)).Label(item.title).Height(36).
		Padding(0, 10).Gap(10).Radius(sunbeam.RadiusLG).
		AlignItems(ui.Center).Justify(ui.Start).
		Cursor(ui.CursorPointer).FocusRing(false)
	switch {
	case selected:
		button.Background(p.fill)
	case button.Pressed():
		button.Background(p.sb.FlatFacePressed)
	case button.Hovered():
		button.Background(p.hover)
	}
	iconColor := p.muted
	textColor := p.inkSecondary
	weight := 400
	if selected {
		iconColor = p.ink
		textColor = p.ink
		weight = 500
	}
	button.Children(func() {
		ui.Icon(c, item.icon).Size(16, 16).TextColor(iconColor)
		ui.Text(c, item.title).FontSize(bodySize(t)).FontWeight(weight).
			TextColor(textColor).SingleLine()
	})
	if button.Clicked() {
		a.settingsPageID = item.id
	}
}

func (a *app) settingsSchoolSection(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().MaxWidth(t.Space(170)).Gap(t.Space(5)).Children(func() {
		formGroup(c, p, "", "", nil, func() {
			formFieldItem(c, p, locales.SettingsAcademicYear(), locales.SettingsAcademicYearHint(), false, func() {
				a.academicYearSelect(c, p)
			})
		})
		if a.settingsError != "" {
			notice(c, p, noticeDanger, a.settingsError)
		}
	})
}

func (a *app) settingsSubjectsSection(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().MaxWidth(t.Space(170)).Gap(t.Space(5)).Children(func() {
		formGroup(c, p, locales.SettingsSubjects(), locales.SettingsSubjectsHint(), nil, func() {
			for index := range a.subjectDrafts {
				index := index
				subject := &a.subjectDrafts[index]
				formFieldItem(c, p, a.subjectName(subject.Key, subject.Name), "", false, func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
							textField(c, &subject.Name, locales.SubjectName(), "")
						})
						numberField(c, &subject.FullMark, 0, 300, 1, locales.SubjectFullMark()).
							Width(t.Space(28)).Shrink(0)
						switchField(c, &subject.Enabled, locales.SubjectEnabled()).Shrink(0)
					})
				})
			}
		})
		formGroup(c, p, locales.SettingsTracks(), locales.SettingsTracksHint(), nil, func() {
			for index := range a.trackDrafts {
				index := index
				track := &a.trackDrafts[index]
				formFieldItem(c, p, a.trackName(track.Key, track.Name), "", false, func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
							textField(c, &track.Name, locales.TrackName(), "")
						})
						switchField(c, &track.Enabled, locales.TrackEnabled()).Shrink(0)
					})
				})
			}
		})
		ui.Row(c).Children(func() {
			if labelledButton(c, locales.SettingsSaveSchool(), nil, true).Clicked() {
				a.saveSchoolSettings()
			}
		})
	})
}

func (a *app) settingsDataSection(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().MaxWidth(t.Space(170)).Gap(t.Space(5)).Children(func() {
		formGroup(c, p, "", "", nil, func() {
			formFieldItem(c, p, locales.SettingsDataDir(), "", false, func() {
				a.settingsDataDirText(c, p)
			})
		})
		items := make([]actionListItem, 0, len(a.backups))
		for _, backup := range a.backups {
			backup := backup
			items = append(items, actionListItem{
				Key:         backup.Name,
				Label:       formatTime(backup.CreatedAt),
				Description: fmt.Sprintf(locales.SettingsBackupSize(), float64(backup.Size)/(1024*1024)),
				Actions: func() {
					if plainButton(c, locales.SettingsRestore()).Clicked() {
						a.restoreBackupName = backup.Name
						a.restoreOpen = true
						a.invalidateSettings()
					}
				},
			})
		}
		formGroup(c, p, locales.SettingsBackups(), "", func() {
			if labelledButton(c, locales.SettingsBackupNow(), iconImports, false).Clicked() {
				a.backupNow()
			}
		}, func() {
			if len(items) == 0 {
				smallEmptyState(c, p, locales.SettingsBackupsEmpty())
				return
			}
			if key := actionList(c, p, items, true); key != "" {
				a.restoreBackupName = key
				a.restoreOpen = true
				a.invalidateSettings()
			}
		})
		if a.settingsError != "" {
			notice(c, p, noticeDanger, a.settingsError)
		}
	})
}

func (a *app) settingsDataDirText(c *ui.Context, p uiPalette) {
	ui.Text(c, dashOr(a.dataDir)).FontSize(captionSize(p.theme)).TextColor(p.muted).
		Selectable().SingleLine().TruncateMiddle().MinWidth(0).Tooltip(a.dataDir)
}

func (a *app) settingsAppearanceSection(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().MaxWidth(t.Space(170)).Gap(t.Space(5)).Children(func() {
		formGroup(c, p, "", "", nil, func() {
			formFieldItem(c, p, locales.LanguageLabel(), "", true, func() {
				a.languageSwitch(c)
			})
			formFieldItem(c, p, locales.AppearanceTitle(), "", true, func() {
				values := []string{
					store.AppearanceSystem, store.AppearanceLight, store.AppearanceDark,
				}
				labels := []string{
					locales.AppearanceSystem(), locales.AppearanceLight(), locales.AppearanceDark(),
				}
				chosen := 0
				for index, value := range values {
					if value == a.appearance {
						chosen = index
					}
				}
				next := chosen
				segmentedField(c, &next, labels, locales.AppearanceTitle())
				if next != chosen {
					a.setAppearance(values[next])
				}
			})
		})
	})
}

// setAppearance 保存外观并立即应用。
func (a *app) setAppearance(value string) {
	a.appearance = value
	source := mygo.ThemeSystem
	switch value {
	case store.AppearanceLight:
		source = mygo.ThemeLight
	case store.AppearanceDark:
		source = mygo.ThemeDark
	}
	if a.win != nil {
		mygo.Theme.SetSource(source)
		a.win.Invalidate()
	}
	if a.store == nil {
		return
	}
	if err := a.store.SetAppearance(context.Background(), value); err != nil {
		a.settingsError = err.Error()
		return
	}
	a.settingsError = ""
	a.invalidateSettings()
}

func (a *app) invalidateSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

func (a *app) restoreDialog(c *ui.Context, p uiPalette) {
	if confirmDangerModal(c, p, "settings-restore", &a.restoreOpen, locales.SettingsRestoreTitle(),
		fmt.Sprintf(locales.SettingsRestoreConfirm(), a.restoreBackupName),
		locales.PlanCancel(), locales.SettingsRestore()) {
		name := a.restoreBackupName
		a.restoreBackup(name)
		a.invalidateSettings()
	}
}

func (a *app) academicYearSelect(c *ui.Context, p uiPalette) {
	t := p.theme
	years := make([]int, 0, 7)
	for year := a.systemAcademicYear - 3; year <= a.systemAcademicYear+3; year++ {
		years = append(years, year)
	}
	labels := make([]string, 0, len(years))
	chosen := a.academicYearLabel(a.academicYearStart)
	for _, year := range years {
		labels = append(labels, a.academicYearLabel(year))
	}
	if !containsString(labels, chosen) {
		labels = append(labels, chosen)
	}
	next := chosen
	selectField(c, "settings-academic-year", &next, labels, locales.SettingsAcademicYear()).Width(t.Space(56))
	if next != chosen {
		start, err := school.ParseAcademicYearStart(next)
		if err == nil {
			a.setAcademicYear(start)
		}
	}
}
