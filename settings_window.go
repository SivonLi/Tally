package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/form"

	"tally/internal/school"
	"tally/internal/store"
	"tally/locales"
)

// settingsSidebarWidth 是设置窗口左侧分类的宽度。
const settingsSidebarWidth = 200

// settingsFormValues 是设置窗口里可编辑的取值，与保存到数据库的草稿分开。
type settingsFormValues struct {
	Subjects []store.Subject
	Tracks   []store.Track
}

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
	a.settingsForm = form.New(a.settingsFormValuesFromDrafts())
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
		BackgroundColor:      "light-dark(#ffffff, #000000)",
		DisableMaximize:      true,
		DisableMinimize:      true,
		AutoHideMenuBar:      true,
		Vibrancy:             mygo.VibrancySidebar,
		Content:              ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
		a.settingsForm = nil
	})
}

// settingsFormValuesFromDrafts 把当前的科目与选科草稿复制成表单初始值。
func (a *app) settingsFormValuesFromDrafts() settingsFormValues {
	return settingsFormValues{
		Subjects: append([]store.Subject(nil), a.subjectDrafts...),
		Tracks:   append([]store.Track(nil), a.trackDrafts...),
	}
}

// syncSettingsForm 把表单的当前值写回保存用的草稿，并返回设置页共同的根节点。
func (a *app) syncSettingsForm(c *ui.Context, content func(values *settingsFormValues)) ui.Element {
	if a.settingsForm == nil {
		a.settingsForm = form.New(a.settingsFormValuesFromDrafts())
	}
	return form.View(c, a.settingsForm, form.Props[settingsFormValues]{
		Variant: form.Borderless,
		Layout:  form.Vertical,
		Gap:     c.Theme().Space(6),
		OnValuesChange: func(values settingsFormValues) {
			a.subjectDrafts = append([]store.Subject(nil), values.Subjects...)
			a.trackDrafts = append([]store.Track(nil), values.Tracks...)
		},
	}, func() {
		content(&a.settingsForm.Values)
	})
}

func (a *app) settingsWindow() *mygo.Window {
	return a.settingsWin
}

func (a *app) settingsView(c *ui.Context) {
	p := uiPaletteFor(c)
	t := p.theme
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
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
	root := ui.Row(c).Fill().AlignItems(ui.Stretch)
	if !c.Vibrancy() {
		root.Background(p.canvas)
	}
	root.Children(func() {
		side := ui.Column(c).Width(settingsSidebarWidth).Shrink(0).FillHeight().
			Padding(top, t.Space(3), t.Space(4), t.Space(3)).Gap(t.Space(0.5)).DragWindow()
		if !c.Vibrancy() {
			side.Background(p.panelMuted)
		}
		side.Children(func() {
			for _, item := range pages {
				item := item
				a.settingsNavItem(c, p, item, item.id == current.id)
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(p.canvas).Children(func() {
			ui.Row(c).FillWidth().Height(topStrip(c)).Shrink(0).
				Padding(0, t.Space(7)).AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Text(c, current.title).FontSize(bodySize(t)).FontWeight(600)
			})
			ui.Scroll(c).Clip().Grow(1).Shrink(1).MinHeight(0).FillWidth().
				Padding(t.Space(4), t.Space(7), t.Space(8), t.Space(7)).
				Children(func() {
					current.build(c, p)
				})
		})
	})
	a.restoreDialog(c, p)
	a.syncConflictDialog(c, p)
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
	button := ui.ButtonBase(c.Key(item.id)).Label(item.title).Height(t.Space(9)).
		Padding(0, t.Space(2)).Gap(t.Space(2)).Radius(t.Space(2)).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	switch {
	case selected:
		button.Background(p.panelHover)
	case button.Hovered():
		button.Background(p.panelHover)
	}
	color := p.muted
	if selected {
		color = p.ink
	}
	button.Children(func() {
		ui.Icon(c, item.icon).FontSize(t.Space(4)).TextColor(color)
		ui.Text(c, item.title).FontSize(bodySize(t)).FontWeight(500).
			TextColor(color).SingleLine()
	})
	if button.Clicked() {
		a.settingsPageID = item.id
	}
}

func (a *app) settingsSchoolSection(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().MaxWidth(t.Space(170)).Gap(t.Space(5)).Children(func() {
		a.syncSettingsForm(c, func(*settingsFormValues) {
			form.Group(c, a.settingsForm, form.GroupProps{Fixed: true}, func() {
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:  "academic-year",
					Label: locales.SettingsAcademicYear(),
					Desc:  locales.SettingsAcademicYearHint(),
					Value: a.academicYearStart,
				}, func() {
					a.academicYearSelect(c, p)
				})
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
		a.syncSettingsForm(c, func(values *settingsFormValues) {
			form.Group(c, a.settingsForm, form.GroupProps{
				Title: locales.SettingsSubjects(),
				Desc:  locales.SettingsSubjectsHint(),
				Fixed: true,
			}, func() {
				for index := range values.Subjects {
					index := index
					subject := &values.Subjects[index]
					form.Field(c, a.settingsForm, form.FieldProps{
						Name:   "subject-" + subject.Key,
						Label:  a.subjectName(subject.Key, subject.Name),
						Layout: form.Horizontal,
						Value:  subject.Name,
					}, func() {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
								textField(c, &subject.Name, locales.SubjectName(), "")
							})
							numberField(c, &subject.FullMark, 0, 300, 1, locales.SubjectFullMark()).
								Width(t.Space(28))
							switchField(c, &subject.Enabled, locales.SubjectEnabled())
						})
					})
				}
			})
			form.Group(c, a.settingsForm, form.GroupProps{
				Title: locales.SettingsTracks(),
				Desc:  locales.SettingsTracksHint(),
				Fixed: true,
			}, func() {
				for index := range values.Tracks {
					index := index
					track := &values.Tracks[index]
					form.Field(c, a.settingsForm, form.FieldProps{
						Name:   "track-" + track.Key,
						Label:  a.trackName(track.Key, track.Name),
						Layout: form.Horizontal,
						Value:  track.Name,
					}, func() {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
								textField(c, &track.Name, locales.TrackName(), "")
							})
							switchField(c, &track.Enabled, locales.TrackEnabled())
						})
					})
				}
			})
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
		a.syncSettingsForm(c, func(*settingsFormValues) {
			form.Group(c, a.settingsForm, form.GroupProps{Fixed: true}, func() {
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:  "data-dir",
					Label: locales.SettingsDataDir(),
					Desc:  a.dataDir,
					Value: a.dataDir,
				}, func() {
					a.settingsDataDirText(c, p)
				})
			})
			items := make([]display.ListItem, 0, len(a.backups))
			for _, backup := range a.backups {
				backup := backup
				items = append(items, display.ListItem{
					Key:         backup.Name,
					Label:       formatTime(backup.CreatedAt),
					Description: fmt.Sprintf(locales.SettingsBackupSize(), float64(backup.Size)/(1024*1024)),
					Extra:       locales.SettingsRestore(),
					Actions: func() {
						plainButton(c, locales.SettingsRestore())
					},
				})
			}
			form.Group(c, a.settingsForm, form.GroupProps{
				Title: locales.SettingsBackups(),
				Fixed: true,
				Extra: func() {
					if labelledButton(c, locales.SettingsBackupNow(), iconImports, false).Clicked() {
						a.backupNow()
					}
				},
			}, func() {
				if len(items) == 0 {
					smallEmptyState(c, p, locales.SettingsBackupsEmpty())
					return
				}
				if key := display.List(c, display.ListProps{Items: items, Variant: display.ListBorderless}); key != "" {
					a.restoreBackupName = key
					a.restoreOpen = true
					a.invalidateSettings()
				}
			})
		})
		if a.settingsError != "" {
			notice(c, p, noticeDanger, a.settingsError)
		}
	})
}

func (a *app) settingsDataDirText(c *ui.Context, p uiPalette) {
	ui.Text(c, a.dataDir).FontSize(captionSize(p.theme)).TextColor(p.muted).
		Selectable().SingleLine()
}

func (a *app) settingsAppearanceSection(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).FillWidth().MaxWidth(t.Space(170)).Gap(t.Space(5)).Children(func() {
		a.syncSettingsForm(c, func(*settingsFormValues) {
			form.Group(c, a.settingsForm, form.GroupProps{Fixed: true}, func() {
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "language",
					Label:  locales.LanguageLabel(),
					Layout: form.Horizontal,
					Value:  a.language,
				}, func() {
					a.languageSwitch(c)
				})
				form.Field(c, a.settingsForm, form.FieldProps{
					Name:   "appearance",
					Label:  locales.AppearanceTitle(),
					Layout: form.Horizontal,
					Value:  a.appearance,
				}, func() {
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
	if ui.AlertDialog(c, &a.restoreOpen, locales.SettingsRestoreTitle(),
		fmt.Sprintf(locales.SettingsRestoreConfirm(), a.restoreBackupName),
		locales.PlanCancel(), locales.SettingsRestore()) == 1 {
		name := a.restoreBackupName
		a.restoreBackupName = ""
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
