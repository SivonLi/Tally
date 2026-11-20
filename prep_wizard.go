package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"tally/internal/importer"
	"tally/locales"
)

type prepStep struct {
	Kind  string
	Title string
	Body  string
	Count int
	Page  pageID
}

func (a *app) prepSteps() []prepStep {
	return []prepStep{
		{Kind: importer.KindClasses, Title: locales.PrepClasses(), Body: locales.PrepClassesBody(),
			Count: a.stageCounts.Classes, Page: pageCohorts},
		{Kind: importer.KindStudents, Title: locales.PrepStudents(), Body: locales.PrepStudentsBody(),
			Count: a.stageCounts.Students, Page: pageStudents},
		{Kind: importer.KindTeachers, Title: locales.PrepTeachers(), Body: locales.PrepTeachersBody(),
			Count: a.stageCounts.Teachers, Page: pageFaculty},
		{Kind: importer.KindAssignments, Title: locales.PrepAssignments(), Body: locales.PrepAssignmentsBody(),
			Count: a.stageCounts.Assignments, Page: pageFaculty},
		{Kind: importer.KindBaselines, Title: locales.PrepBaselines(), Body: locales.PrepBaselinesBody(),
			Count: a.stageCounts.Baselines, Page: pageBaseline},
	}
}

func (a *app) prepPage(c *ui.Context, p uiPalette) {
	t := p.theme
	steps := a.prepSteps()
	done := 0
	next := -1
	for index, step := range steps {
		if step.Count > 0 {
			done++
		} else if next < 0 {
			next = index
		}
	}
	editable := a.stageEditable()
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			if ui.Button(c, "").Label(locales.Back()).Children(func() {
				ui.Icon(c, iconArrowLeft).FontSize(t.Space(4))
				ui.Text(c, locales.Back()).FontSize(bodySize(t))
			}).Clicked() {
				a.prepOpen = false
			}
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Text(c, locales.PrepTitle()).FontSize(titleSize(t)).Bold().
					Role(ui.RoleHeading).Level(1)
				ui.Text(c, locales.PrepSubtitle()).FontSize(bodySize(t)).TextColor(p.muted)
			})
			ui.Textf(c, locales.ReadinessProgress(), done, len(steps)).FontSize(bodySize(t)).
				TextColor(p.inkSecondary).FontFeatures("tnum")
		})
		ui.Progress(c, float64(done)/float64(len(steps))).
			TextColor(p.accent).Background(p.panelMuted).FillWidth().
			Label(locales.ReadinessTitle())
		if !editable {
			notice(c, p, noticeInfo, locales.StageReadOnly())
		}
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				for index, step := range steps {
					index := index
					step := step
					panel(c, p).FillWidth().Padding(t.Space(3)).Gap(t.Space(1)).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
							if step.Count > 0 {
								badge(c, p, fmtInt(index+1), toneSuccess)
							} else if index == next {
								badge(c, p, fmtInt(index+1), toneAccent)
							} else {
								badge(c, p, fmtInt(index+1), toneNeutral)
							}
							ui.Column(c).Grow(1).Gap(0).Children(func() {
								ui.Text(c, step.Title).FontSize(bodySize(t)).Bold()
								ui.Text(c, step.Body).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
							})
							if step.Count > 0 {
								badge(c, p, locales.ReadinessReady(), toneSuccess)
								view := ui.Button(c, locales.PrepView())
								view.Label(locales.PrepView() + " " + step.Title)
								if view.Clicked() {
									a.prepOpen = false
									a.page = step.Page
								}
								return
							}
							if !editable {
								badge(c, p, locales.ReadinessMissing(), toneNeutral)
								return
							}
							primary := index == next
							button := labelledButton(c, locales.PrepImport(), iconImports, primary)
							button.Label(locales.PrepImport() + " " + step.Title)
							if button.Clicked() {
								a.openImportWizard(step.Kind)
							}
						})
					})
				}
			})
		})
		if editable && next >= 0 {
			ui.Row(c).Children(func() {
				if labelledButton(c, locales.PrepContinueNext(), iconArrowRight, true).Clicked() {
					a.openImportWizard(steps[next].Kind)
				}
			})
		}
	})
}

func fmtInt(value int) string {
	return fmt.Sprintf("%d", value)
}
