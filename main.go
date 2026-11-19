package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"tally/internal/calc"
	"tally/internal/store"
	"tally/locales"
)

type app struct {
	win            *mygo.Window
	dataDir        string
	language       string
	store          *store.Store
	plans          []store.Plan
	versions       []store.PlanVersion
	selectedPlanID string
	versionIndex   int
	sidebarWidth   float32
	versionTable   ui.ListState
	loadError      string

	tab               int
	published         []publishedVersion
	batches           []store.Batch
	selectedBatchID   string
	batchSidebarWidth float32
	batchBusy         bool
	batchMessage      string
	batchError        string
	importSummary     *store.ImportSummary
	resultRows        []store.StoredResultRow
	personalScores    []store.StoredPersonalScore
	resultTable       ui.ListState
	calculateVersion  string
	newBatchOpen      bool
	newBatchYear      string
	newBatchSemester  string
	newBatchExamType  string
	newBatchGrade     string
	newBatchError     string
	zeroOpen          bool
	zeroIssues        []calc.ZeroTargetIssue
	zeroValues        map[string]*float64
	zeroReason        string
	zeroError         string
}

func (a *app) load() {
	ctx := context.Background()
	dir := a.dataDir
	if dir == "" {
		var err error
		dir, err = mygo.App.Path(mygo.PathUserData)
		if err != nil {
			a.loadError = err.Error()
			return
		}
	}
	opened, err := store.Open(ctx, filepath.Join(dir, "tally.db"))
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.store = opened
	a.dataDir = dir
	if _, err := opened.EnsureDefaultPlan(ctx); err != nil {
		a.loadError = err.Error()
		return
	}
	a.reloadPlans(ctx)
	a.reloadBatches(ctx)
}

func (a *app) reloadPlans(ctx context.Context) {
	if a.store == nil {
		return
	}
	plans, err := a.store.ListPlans(ctx)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.plans = plans
	if len(plans) == 0 {
		a.selectedPlanID = ""
		a.versions = nil
		return
	}
	found := false
	for _, plan := range plans {
		if strconv.FormatInt(plan.ID, 10) == a.selectedPlanID {
			found = true
			break
		}
	}
	if !found {
		a.selectedPlanID = strconv.FormatInt(plans[0].ID, 10)
	}
	a.reloadVersions(ctx)
	a.reloadPublished(ctx)
}

func (a *app) reloadVersions(ctx context.Context) {
	if a.store == nil || a.selectedPlanID == "" {
		a.versions = nil
		a.versionIndex = -1
		return
	}
	planID, err := strconv.ParseInt(a.selectedPlanID, 10, 64)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	versions, err := a.store.ListVersions(ctx, planID)
	if err != nil {
		a.loadError = err.Error()
		return
	}
	a.versions = versions
	if len(versions) == 0 {
		a.versionIndex = -1
		return
	}
	if a.versionIndex < 0 || a.versionIndex >= len(versions) {
		a.versionIndex = 0
	}
}

func (a *app) selectedPlan() *store.Plan {
	if a.selectedPlanID == "" {
		return nil
	}
	planID, err := strconv.ParseInt(a.selectedPlanID, 10, 64)
	if err != nil {
		return nil
	}
	for i := range a.plans {
		if a.plans[i].ID == planID {
			return &a.plans[i]
		}
	}
	return nil
}

func (a *app) selectedVersion() *store.PlanVersion {
	if a.versionIndex < 0 || a.versionIndex >= len(a.versions) {
		return nil
	}
	return &a.versions[a.versionIndex]
}

func (a *app) publishSelected() {
	version := a.selectedVersion()
	if version == nil {
		return
	}
	ctx := context.Background()
	if err := a.store.Publish(ctx, version.ID, "local"); err != nil {
		a.loadError = err.Error()
		return
	}
	a.loadError = ""
	a.reloadVersions(ctx)
	a.reloadPublished(ctx)
}

func (a *app) retireSelected() {
	version := a.selectedVersion()
	if version == nil {
		return
	}
	ctx := context.Background()
	if err := a.store.Retire(ctx, version.ID); err != nil {
		a.loadError = err.Error()
		return
	}
	a.loadError = ""
	a.reloadVersions(ctx)
	a.reloadPublished(ctx)
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(t.Space(4)).Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(4)).Children(func() {
			ui.Text(c, locales.AppTitle()).FontSize(t.FontSize * 1.6).Bold()
			ui.Spacer(c)
			ui.RadioGroup(c, func() {
				for _, tag := range []string{"en", "zh-Hans"} {
					if ui.Radio(c, &a.language, tag, i18n.Autonym(tag)).Changed() {
						mygo.App.SetLocale(a.language)
					}
				}
			}).Label(locales.LanguageLabel())
		})
		if a.loadError != "" {
			ui.Text(c, locales.PlanLoadError()+": "+a.loadError).TextColor(t.Danger)
			return
		}
		ui.Tabs(c, &a.tab, locales.PlanLibraryTitle(), locales.BatchWorkbenchTitle())
		if a.tab == 1 {
			a.batchView(c)
			return
		}
		a.planView(c)
	})
}

func (a *app) planView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Gap(t.Space(3)).Children(func() {
		if len(a.plans) == 0 {
			ui.Text(c, locales.PlanEmpty()).TextColor(t.TextMuted)
			return
		}
		ui.Split(c, &a.sidebarWidth, func() {
			a.planSidebar(c)
		}, func() {
			a.planDetails(c)
		}).Fill()
	})
}

func (a *app) planSidebar(c *ui.Context) {
	sidebar := ui.Sidebar(c, &a.selectedPlanID, func() {
		ui.SidebarSection(c, locales.PlanListTitle(), nil, func() {
			for _, plan := range a.plans {
				ui.SidebarItem(c, strconv.FormatInt(plan.ID, 10), nil, plan.Name)
			}
		})
	}).Fill()
	if sidebar.Changed() {
		a.reloadVersions(context.Background())
	}
}

func (a *app) planDetails(c *ui.Context) {
	t := c.Theme()
	plan := a.selectedPlan()
	ui.Column(c).Fill().Padding(t.Space(3)).Gap(t.Space(3)).Children(func() {
		if plan == nil {
			ui.Text(c, locales.PlanSelectHint()).TextColor(t.TextMuted)
			return
		}
		ui.Text(c, plan.Name).FontSize(t.FontSize * 1.3).Bold()
		ui.Text(c, a.kindLabel(plan.Kind)).TextColor(t.TextMuted)
		if plan.Description != "" {
			ui.Text(c, plan.Description).TextColor(t.TextMuted)
		}
		ui.Row(c).Gap(t.Space(2)).Children(func() {
			version := a.selectedVersion()
			publish := ui.Button(c, locales.PublishVersion()).Label(locales.PublishVersion())
			publish.Disabled(version == nil || version.Status != "draft")
			if publish.Clicked() {
				a.publishSelected()
			}
			retire := ui.Button(c, locales.RetireVersion()).Label(locales.RetireVersion())
			retire.Disabled(version == nil || version.Status != "published")
			if retire.Clicked() {
				a.retireSelected()
			}
		})
		ui.Divider(c)
		ui.Text(c, locales.PlanVersionsTitle()).Bold()
		if len(a.versions) == 0 {
			ui.Text(c, locales.PlanNoVersions()).TextColor(t.TextMuted)
			return
		}
		a.versionTable.Selected = &a.versionIndex
		a.versionTable.Key = func(row int) any { return a.versions[row].ID }
		a.versionTable.Label = func(row int) string {
			return fmt.Sprintf("%s %d", locales.PlanColumnVersion(), a.versions[row].Version)
		}
		columns := []ui.TableColumn{
			{Title: locales.PlanColumnVersion(), Width: t.Space(18)},
			{Title: locales.PlanColumnStatus(), Width: t.Space(22)},
			{Title: locales.PlanColumnCreated(), Width: t.Space(38)},
			{Title: locales.PlanColumnPublished(), Width: t.Space(38)},
		}
		ui.Table(c, &a.versionTable, columns, len(a.versions), func(row, col int) {
			version := a.versions[row]
			switch col {
			case 0:
				ui.Textf(c, "%d", version.Version)
			case 1:
				ui.Text(c, a.statusLabel(version.Status))
			case 2:
				ui.Text(c, formatTime(version.CreatedAt))
			case 3:
				if version.PublishedAt == nil {
					ui.Text(c, "-")
					return
				}
				ui.Text(c, formatTime(*version.PublishedAt))
			}
		}).Grow(1).FillWidth()
	})
}

func (a *app) kindLabel(kind string) string {
	if kind == "system" {
		return locales.PlanKindSystem()
	}
	return locales.PlanKindUser()
}

func (a *app) statusLabel(status string) string {
	switch status {
	case "draft":
		return locales.PlanStatusDraft()
	case "published":
		return locales.PlanStatusPublished()
	case "retired":
		return locales.PlanStatusRetired()
	default:
		return status
	}
}

func formatTime(value time.Time) string {
	return value.Local().Format("2006-01-02 15:04")
}

func main() {
	a := &app{}
	mygo.App.WhenReady(func() {
		a.language = mygo.App.Locale()
		a.load()
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     locales.AppTitle(),
			Width:     960,
			Height:    640,
			MinWidth:  720,
			MinHeight: 480,
			StateKey:  "plans",
			Content:   ui.View(a.view),
		})
		mygo.App.OnLocaleChanged(func(string) {
			if a.win != nil {
				a.win.SetTitle(locales.AppTitle())
			}
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			log.Println(err)
		}
	}
}
