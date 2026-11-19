package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"tally/internal/assessment"
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
	planError      string
	planMessage    string
	planEditOpen   bool
	planEditName   string
	planEditNotes  string
	planEditDraft  string
	planEditError  string

	tab                int
	published          []publishedVersion
	batches            []store.Batch
	selectedBatchID    string
	batchSidebarWidth  float32
	batchResultsHeight float32
	batchBusy          bool
	batchMessage       string
	batchError         string
	importSummary      *store.ImportSummary
	resultRows         []store.StoredResultRow
	personalScores     []store.StoredPersonalScore
	resultTable        ui.ListState
	calculateVersion   string
	newBatchOpen       bool
	newBatchYear       string
	newBatchSemester   string
	newBatchExamType   string
	newBatchGrade      string
	newBatchError      string
	zeroOpen           bool
	zeroIssues         []calc.ZeroTargetIssue
	zeroValues         map[string]*float64
	zeroReason         string
	zeroError          string
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
		a.planError = err.Error()
		a.planMessage = ""
		return
	}
	a.planError = ""
	a.planMessage = fmt.Sprintf(locales.PlanPublishedMessage(), version.Version)
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
		if errors.Is(err, store.ErrSystemPlanRetire) {
			a.planError = locales.PlanSystemCannotRetire()
		} else {
			a.planError = err.Error()
		}
		a.planMessage = ""
		return
	}
	a.planError = ""
	a.planMessage = fmt.Sprintf(locales.PlanRetiredMessage(), version.Version)
	a.reloadVersions(ctx)
	a.reloadPublished(ctx)
}

func (a *app) view(c *ui.Context) {
	p := uiPaletteFor(c)
	ui.Column(c).Fill().Background(p.canvas).Children(func() {
		a.appBar(c, p)
		ui.Column(c).Fill().Padding(p.theme.Space(5), p.theme.Space(7)).Children(func() {
			if a.loadError != "" {
				notice(c, p, noticeDanger, locales.PlanLoadError()+": "+a.loadError)
				return
			}
			if a.tab == 1 {
				a.batchPage(c, p)
				return
			}
			a.planPage(c, p)
		}).Role(ui.RoleMain)
	})
}

func (a *app) planPage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.sidebarWidth == 0 {
		a.sidebarWidth = t.Space(67)
	}
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.PlanLibraryTitle(), locales.PlanPageSubtitle(), nil)
		if len(a.plans) == 0 {
			emptyState(c, p, iconEmptyPlan, locales.PlanEmpty(), locales.PlanEmptyHint(), nil)
			return
		}
		ui.Split(c, &a.sidebarWidth, func() {
			a.planSidebar(c, p)
		}, func() {
			a.planDetails(c, p)
		}).Grow(1)
	})
	a.planEditModal(c, p)
}

func (a *app) planSidebar(c *ui.Context, p uiPalette) {
	changed := false
	listPanel(c, p, locales.PlanListPanelTitle(), len(a.plans), func() {
		sidebar := ui.Sidebar(c, &a.selectedPlanID, func() {
			for _, plan := range a.plans {
				ui.SidebarItem(c, strconv.FormatInt(plan.ID, 10), iconPlans, plan.Name)
			}
		}).Label(locales.PlanListTitle()).Fill()
		if sidebar.Changed() {
			changed = true
		}
	})
	if changed {
		a.planError = ""
		a.planMessage = ""
		a.reloadVersions(context.Background())
	}
}

func (a *app) planDetails(c *ui.Context, p uiPalette) {
	t := p.theme
	plan := a.selectedPlan()
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		if plan == nil {
			emptyState(c, p, iconEmptyPlan, locales.PlanSelectHint(), "", nil)
			return
		}
		a.planSummary(c, p, *plan)
		if a.planError != "" {
			notice(c, p, noticeWarning, a.planError)
		}
		if a.planMessage != "" {
			notice(c, p, noticeSuccess, a.planMessage)
		}
		panel(c, p).Grow(1).Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
			sectionHeading(c, p, locales.PlanVersionsTitle(), -1)
			a.versionsTable(c, p)
		})
	})
}

func (a *app) planSummary(c *ui.Context, p uiPalette, plan store.Plan) {
	t := p.theme
	panel(c, p).Padding(t.Space(5)).Gap(t.Space(4)).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Grow(1).Gap(t.Space(1)).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Text(c, plan.Name).FontSize(t.Rem(1.55)).Bold().LetterSpacing(-0.25).
						Role(ui.RoleHeading).Level(2)
					kindBadge(c, p, plan.Kind, a.kindLabel(plan.Kind))
				})
				if plan.Description != "" {
					ui.Text(c, plan.Description).FontSize(t.Rem(0.9)).TextColor(p.muted).Pretty()
				}
			})
			a.versionActions(c, p)
		})
		ui.Row(c).Gap(t.Space(7)).Children(func() {
			published := 0
			for _, version := range a.versions {
				if version.Status == "published" {
					published++
				}
			}
			latest := "-"
			if len(a.versions) > 0 {
				latest = fmt.Sprintf("v%d", a.versions[0].Version)
			}
			metric(c, p, fmt.Sprint(len(a.versions)), locales.PlanMetricVersions())
			metric(c, p, fmt.Sprint(published), locales.PlanMetricPublished())
			metric(c, p, latest, locales.PlanMetricLatest())
		})
	})
}

func (a *app) versionActions(c *ui.Context, p uiPalette) {
	plan := a.selectedPlan()
	version := a.selectedVersion()
	if plan == nil || version == nil {
		return
	}
	t := p.theme
	ui.Row(c).Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
		if plan.Kind != "system" {
			publish := labelledButton(c, locales.PublishVersion(), iconExport, version.Status == "draft")
			publish.Disabled(version.Status != "draft")
			if publish.Clicked() {
				a.publishSelected()
			}
		}
		update := labelledButton(c, locales.PlanUpdate(), iconEdit, plan.Kind == "system")
		if update.Clicked() {
			a.openPlanEditor()
		}
		if labelledButton(c, locales.PlanSaveAsNew(), iconCopy, false).Clicked() {
			a.openPlanEditor()
		}
		if plan.Kind != "system" {
			retire := labelledButton(c, locales.RetireVersion(), nil, false)
			retire.Disabled(version.Status != "published")
			if retire.Clicked() {
				a.retireSelected()
			}
		}
	})
}

func (a *app) openPlanEditor() {
	plan := a.selectedPlan()
	version := a.selectedVersion()
	if plan == nil || version == nil {
		return
	}
	data, err := json.MarshalIndent(version.Config, "", "  ")
	if err != nil {
		a.planError = err.Error()
		return
	}
	a.planEditOpen = true
	a.planEditName = plan.Name
	a.planEditNotes = ""
	a.planEditDraft = string(data)
	a.planEditError = ""
}

func (a *app) planEditModal(c *ui.Context, p uiPalette) {
	if !a.planEditOpen {
		return
	}
	t := p.theme
	ui.Modal(c, &a.planEditOpen, func() {
		ui.Column(c).Width(t.Space(170)).Gap(t.Space(4)).Children(func() {
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.PlanConfigTitle()).FontSize(t.Rem(1.3)).Bold().
					Role(ui.RoleHeading).Level(2)
				ui.Text(c, locales.PlanConfigSubtitle()).FontSize(t.Rem(0.9)).
					TextColor(p.muted).Pretty()
			})
			ui.Field(c, locales.PlanNameLabel(), func() {
				ui.TextInput(c, &a.planEditName)
			})
			ui.Field(c, locales.PlanConfigJSON(), func() {
				ui.TextArea(c, &a.planEditDraft).Height(t.Space(88)).FillWidth()
			})
			ui.Field(c, locales.PlanVersionNotes(), func() {
				ui.TextInput(c, &a.planEditNotes)
			})
			if a.planEditError != "" {
				notice(c, p, noticeWarning, a.planEditError)
			}
			ui.Row(c).Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if ui.Button(c, locales.PlanCancel()).Clicked() {
					a.planEditOpen = false
				}
				if ui.Button(c, locales.PlanSaveAsNew()).Clicked() {
					a.savePlanAsNew()
				}
				if ui.PrimaryButton(c, locales.PlanUpdate()).Clicked() {
					a.savePlanUpdate()
				}
			})
		})
	})
}

func (a *app) parsePlanEditConfig() (assessment.Config, bool) {
	config, err := assessment.Decode([]byte(a.planEditDraft))
	if err != nil {
		a.planEditError = locales.PlanUpdateInvalid() + " " + err.Error()
		return assessment.Config{}, false
	}
	name := strings.TrimSpace(a.planEditName)
	if name == "" {
		a.planEditError = locales.PlanNameRequired()
		return assessment.Config{}, false
	}
	config.Name = name
	return config, true
}

func (a *app) savePlanUpdate() {
	plan := a.selectedPlan()
	if plan == nil {
		return
	}
	config, ok := a.parsePlanEditConfig()
	if !ok {
		return
	}
	updated, err := a.store.UpdatePlan(context.Background(), plan.ID, config, a.planEditNotes, "local")
	if err != nil {
		a.planEditError = err.Error()
		return
	}
	a.planEditOpen = false
	a.planEditError = ""
	a.planError = ""
	a.planMessage = fmt.Sprintf(locales.PlanUpdateSaved(), updated.Version)
	a.reloadPlans(context.Background())
	a.selectPlanVersion(updated.ID)
}

func (a *app) savePlanAsNew() {
	base := a.selectedVersion()
	if base == nil {
		return
	}
	config, ok := a.parsePlanEditConfig()
	if !ok {
		return
	}
	plan, version, err := a.store.CreatePlan(context.Background(), store.CreatePlanInput{
		Name:          config.Name,
		Kind:          "user",
		Description:   config.Description,
		BaseVersionID: base.ID,
		Config:        config,
		CreatedBy:     "local",
	})
	if err != nil {
		a.planEditError = err.Error()
		return
	}
	a.planEditOpen = false
	a.planEditError = ""
	a.planError = ""
	a.planMessage = fmt.Sprintf(locales.PlanSavedAsNew(), plan.Name, version.Version)
	a.reloadPlans(context.Background())
	a.selectedPlanID = strconv.FormatInt(plan.ID, 10)
	a.reloadVersions(context.Background())
	a.reloadPublished(context.Background())
	a.selectPlanVersion(version.ID)
}

func (a *app) selectPlanVersion(id int64) {
	for index := range a.versions {
		if a.versions[index].ID == id {
			a.versionIndex = index
			return
		}
	}
}

func (a *app) versionsTable(c *ui.Context, p uiPalette) {
	t := p.theme
	if len(a.versions) == 0 {
		smallEmptyState(c, p, locales.PlanNoVersions())
		return
	}
	a.versionTable.Selected = &a.versionIndex
	a.versionTable.Key = func(row int) any { return a.versions[row].ID }
	a.versionTable.Label = func(row int) string {
		return fmt.Sprintf("%s %d", locales.PlanColumnVersion(), a.versions[row].Version)
	}
	a.versionTable.RowHeight = t.Space(8)
	columns := []ui.TableColumn{
		{Title: locales.PlanColumnVersion(), Width: t.Space(18)},
		{Title: locales.PlanColumnStatus(), Width: t.Space(24)},
		{Title: locales.PlanColumnCreated(), Width: 0},
		{Title: locales.PlanColumnPublished(), Width: 0},
	}
	ui.Table(c, &a.versionTable, columns, len(a.versions), func(row, col int) {
		version := a.versions[row]
		switch col {
		case 0:
			ui.Textf(c, "%d", version.Version)
		case 1:
			statusBadge(c, p, version.Status, a.statusLabel(version.Status))
		case 2:
			ui.Text(c, formatTime(version.CreatedAt)).TextColor(p.muted)
		case 3:
			if version.PublishedAt == nil {
				ui.Text(c, "-").TextColor(p.muted)
				return
			}
			ui.Text(c, formatTime(*version.PublishedAt)).TextColor(p.muted)
		}
	}).Label(locales.PlanVersionsTitle()).Grow(1).FillWidth()
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
			Width:     1120,
			Height:    760,
			MinWidth:  900,
			MinHeight: 620,
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
