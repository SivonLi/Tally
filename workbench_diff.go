package main

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"

	"tally/internal/pipeline"
	"tally/locales"
)

// openRunCompare 打开正式/试算版本对比抽屉，默认以最新正式版为基准、当前查看版本或最新试算版为对比对象。
func (a *app) openRunCompare() {
	if len(a.assessmentRuns) < 2 {
		return
	}
	baseID := a.latestOfficialRun().ID
	if baseID == 0 {
		baseID = a.assessmentRuns[len(a.assessmentRuns)-1].ID
	}
	targetID := a.assessmentRunID
	if targetID == 0 || targetID == baseID {
		for _, run := range a.assessmentRuns {
			if run.ID != baseID {
				targetID = run.ID
				break
			}
		}
	}
	a.runCompareBaseID = baseID
	a.runCompareTargetID = targetID
	a.runCompareTab = 0
	a.runCompareOnlyChanged = false
	a.runCompareOpen = true
	a.reloadRunCompare()
}

func (a *app) reloadRunCompare() {
	if a.store == nil || a.runCompareBaseID == 0 || a.runCompareTargetID == 0 {
		a.runCompareDiff = nil
		return
	}
	diff, err := pipeline.DiffAssessmentRuns(context.Background(), a.store, a.selectedAssessmentID, a.runCompareBaseID, a.runCompareTargetID)
	if err != nil {
		a.runCompareError = err.Error()
		a.runCompareDiff = nil
		return
	}
	a.runCompareError = ""
	a.runCompareDiff = &diff
}

func (a *app) runCompareDrawer(c *ui.Context, p uiPalette) {
	if !a.runCompareOpen {
		return
	}
	t := p.theme
	subtitle := ""
	if ov := a.selectedAssessmentOverview(); ov != nil {
		subtitle = ov.ExamName
	}
	sideDrawer(c, &a.runCompareOpen, sideDrawerProps{
		Key:      "run-compare",
		Title:    locales.WorkbenchCompareTitle(),
		Subtitle: subtitle,
		Width:    680,
	}, func() {
		options := a.runOptions()
		baseLabel := a.runOptionLabelForID(a.runCompareBaseID)
		targetLabel := a.runOptionLabelForID(a.runCompareTargetID)
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(t.Space(3)).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(50)).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.WorkbenchCompareBase()).FontSize(captionSize(t)).TextColor(p.muted)
				nextBase := baseLabel
				selectField(c, "compare-base-run", &nextBase, options, locales.WorkbenchCompareBase()).FillWidth()
				if nextBase != baseLabel {
					if id, ok := a.runIDForLabel(nextBase); ok {
						a.runCompareBaseID = id
						a.reloadRunCompare()
					}
				}
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(50)).Gap(t.Space(1)).Children(func() {
				ui.Text(c, locales.WorkbenchCompareTarget()).FontSize(captionSize(t)).TextColor(p.muted)
				nextTarget := targetLabel
				selectField(c, "compare-target-run", &nextTarget, options, locales.WorkbenchCompareTarget()).FillWidth()
				if nextTarget != targetLabel {
					if id, ok := a.runIDForLabel(nextTarget); ok {
						a.runCompareTargetID = id
						a.reloadRunCompare()
					}
				}
			})
		})
		if a.runCompareError != "" {
			notice(c, p, noticeDanger, a.runCompareError)
			return
		}
		diff := a.runCompareDiff
		if diff == nil {
			smallEmptyState(c, p, locales.WorkbenchCompareNoDiff())
			return
		}
		summary := fmt.Sprintf(locales.WorkbenchCompareSummary(),
			formatInt(diff.ChangedRowCount), formatInt(diff.ChangedTierCount), formatInt(diff.ChangedPersonalCount))
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			ui.Text(c, summary).FontSize(captionSize(t)).TextColor(p.inkSecondary).
				FontFeatures("tnum").Grow(1).Shrink(1).MinWidth(0)
			checkboxField(c, &a.runCompareOnlyChanged, locales.WorkbenchCompareOnlyChanged())
		})
		ui.Row(c).Children(func() {
			segmentedField(c, &a.runCompareTab, []string{
				locales.AssessmentViewBySubject(),
				locales.AssessmentViewByTeacher(),
			}, locales.WorkbenchCompareTitle())
		})
		if diff.ChangedRowCount == 0 && diff.ChangedPersonalCount == 0 && a.runCompareOnlyChanged {
			smallEmptyState(c, p, locales.WorkbenchCompareNoDiff())
			return
		}
		if a.runCompareTab == 1 {
			a.runComparePersonalList(c, p, diff.Personal)
			return
		}
		a.runCompareRowList(c, p, diff.Rows)
	}, func() {
		if plainButton(c, locales.Close()).Clicked() {
			a.runCompareOpen = false
		}
	})
}

func (a *app) runCompareRowList(c *ui.Context, p uiPalette, rows []pipeline.RowDiff) {
	t := p.theme
	shown := 0
	ui.Column(c).FillWidth().Gap(0).Children(func() {
		for _, item := range rows {
			changed := math.Abs(item.TotalDelta) > 1e-6 || item.RankDelta != 0 || item.TierChanged || math.Abs(item.PointsDelta) > 1e-6 || item.Presence != "both"
			if a.runCompareOnlyChanged && !changed {
				continue
			}
			if shown > 0 {
				rule(c)
			}
			shown++
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Padding(t.Space(2), 0).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					title := item.StatGroupName + " · " + subjectLabel(item.Subject)
					ui.Text(c, title).FontSize(bodySize(t)).FontWeight(500).TextColor(p.ink).SingleLine().Tooltip(title)
					if item.TeacherNames != "" {
						ui.Text(c, decodeNames(item.TeacherNames)).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
					}
				})
				ui.Column(c).AlignItems(ui.End).Gap(1).Shrink(0).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
						ui.Textf(c, "%.2f → %.2f", item.BaseTotal, item.CompareTotal).
							FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
						deltaText(c, p, formatSignedDecimal(item.TotalDelta, 2), signOfFloat(item.TotalDelta))
					})
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						rankStr := "="
						if item.RankDelta > 0 {
							rankStr = fmt.Sprintf("↑%d", item.RankDelta)
						} else if item.RankDelta < 0 {
							rankStr = fmt.Sprintf("↓%d", -item.RankDelta)
						}
						deltaText(c, p, fmt.Sprintf("#%d→#%d (%s)", item.BaseRank, item.CompareRank, rankStr), item.RankDelta)
						if item.TierChanged {
							badge(c, p, fmt.Sprintf("%d→%d", item.BaseTier+1, item.CompareTier+1), toneWarning)
						}
						if math.Abs(item.PointsDelta) > 1e-6 {
							deltaText(c, p, formatSignedDecimal(item.PointsDelta, 1), signOfFloat(item.PointsDelta))
						}
					})
				})
			})
		}
	})
	if shown == 0 {
		smallEmptyState(c, p, locales.WorkbenchCompareNoDiff())
	}
}

func (a *app) runComparePersonalList(c *ui.Context, p uiPalette, items []pipeline.PersonalDiff) {
	t := p.theme
	shown := 0
	ui.Column(c).FillWidth().Gap(0).Children(func() {
		for _, item := range items {
			changed := math.Abs(item.ScoreDelta) > 1e-6 || item.Presence != "both"
			if a.runCompareOnlyChanged && !changed {
				continue
			}
			if shown > 0 {
				rule(c)
			}
			shown++
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Padding(t.Space(2), 0).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, item.TeacherName).FontSize(bodySize(t)).FontWeight(500).TextColor(p.ink).SingleLine()
					ui.Text(c, subjectLabel(item.Subject)).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
				})
				ui.Textf(c, "%.2f → %.2f", item.BaseScore, item.CompareScore).
					FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
				deltaText(c, p, formatSignedDecimal(item.ScoreDelta, 2), signOfFloat(item.ScoreDelta))
			})
		}
	})
	if shown == 0 {
		smallEmptyState(c, p, locales.WorkbenchCompareNoDiff())
	}
}

func formatSignedDecimal(v float64, prec int) string {
	if math.Abs(v) < 1e-6 {
		return "0"
	}
	if v > 0 {
		return "+" + strconv.FormatFloat(v, 'f', prec, 64)
	}
	return strconv.FormatFloat(v, 'f', prec, 64)
}

func signOfFloat(v float64) int {
	if v > 1e-6 {
		return 1
	}
	if v < -1e-6 {
		return -1
	}
	return 0
}
