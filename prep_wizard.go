package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/theme"

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
	ui.Column(c).Fill().Gap(t.Space(5)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			back := textButton(c, locales.Back()).Padding(t.Space(1.5)).
				Radius(t.Space(1.5)).Background(ui.Transparent)
			back.Children(func() {
				ui.Icon(c, iconArrowLeft).FontSize(t.Space(4)).TextColor(p.ink)
				ui.Text(c, locales.Back()).FontSize(bodySize(t))
			})
			if back.Clicked() {
				a.prepOpen = false
			}
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Text(c, locales.PrepTitle()).FontSize(titleSize(t)).FontWeight(600).
					Role(ui.RoleHeading).Level(1)
				ui.Text(c, locales.PrepSubtitle()).FontSize(bodySize(t)).TextColor(p.muted)
			})
			ui.Textf(c, locales.ReadinessProgress(), formatInt(done), formatInt(len(steps))).FontSize(bodySize(t)).
				TextColor(p.inkSecondary).FontFeatures("tnum")
		})
		feedback.Progress(c, float32(done)/float32(len(steps))*100, feedback.ProgressProps{
			Label: locales.ReadinessTitle(),
			Size:  theme.Middle,
		}).FillWidth()
		if !editable {
			notice(c, p, noticeInfo, locales.StageReadOnly())
		}
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(0).Children(func() {
				for index, step := range steps {
					index := index
					step := step
					a.prepRow(c, p, step, index, next, editable)
				}
			})
		})
	})
}

func (a *app) prepRow(c *ui.Context, p uiPalette, step prepStep, index, next int, editable bool) {
	t := p.theme
	rule(c)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).
		Padding(t.Space(3.5), 0).Children(func() {
		ui.Textf(c, "%d", index+1).FontSize(captionSize(t)).
			TextColor(p.muted).FontFeatures("tnum").Width(t.Space(4))
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			ui.Text(c, step.Title).FontSize(bodySize(t)).FontWeight(600)
			ui.Text(c, step.Body).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
		})
		switch {
		case step.Count > 0:
			ui.Text(c, formatInt(step.Count)).FontSize(captionSize(t)).
				TextColor(p.muted).FontFeatures("tnum")
			view := plainButton(c, locales.PrepView())
			view.Label(locales.PrepView() + " " + step.Title)
			if view.Clicked() {
				a.prepOpen = false
				a.page = step.Page
			}
		case !editable:
			ui.Text(c, locales.ReadinessMissing()).FontSize(captionSize(t)).TextColor(p.muted)
		default:
			primary := index == next
			button := labelledButton(c, locales.PrepImport(), iconImports, primary)
			button.Label(locales.PrepImport() + " " + step.Title)
			if button.Clicked() {
				a.openImportWizard(step.Kind)
			}
		}
	})
}
