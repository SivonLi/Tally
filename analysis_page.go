package main

import (
	"sort"

	"github.com/egoist/mygo/ui"

	"tally/internal/store"
	"tally/locales"
)

// analysisPage 用当前考核的结果对比学科与班级，帮助读者看出差异在哪。
func (a *app) analysisPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.NavAnalysis(), locales.AnalysisPageSubtitle(), nil)
		if a.assessmentError != "" {
			notice(c, p, noticeDanger, a.assessmentError)
		}
		if len(a.assessments) == 0 {
			emptyState(c, p, iconAnalysis, locales.AnalysisEmpty(), locales.AnalysisEmptyHint(), func() {
				if labelledButton(c, locales.AssessmentNewExam(), iconPlus, true).Clicked() {
					a.page = pageExams
					a.openNewExam()
				}
			})
			return
		}
		a.analysisAssessmentSelect(c, p)
		if len(a.assessmentRows) == 0 {
			emptyState(c, p, iconAnalysis, locales.AssessmentNoResults(), locales.AssessmentNoResultsHint(), func() {
				if labelledButton(c, locales.AssessmentOpenAssessment(), iconBatches, false).Clicked() {
					a.page = pageAssessments
				}
			})
			return
		}
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(6)).Children(func() {
				a.analysisOverview(c, p)
				a.analysisByClass(c, p)
				a.analysisBySubject(c, p)
				a.analysisByTeacher(c, p)
			})
		})
	})
}

func (a *app) analysisAssessmentSelect(c *ui.Context, p uiPalette) {
	t := p.theme
	labels := make([]string, 0, len(a.assessments))
	chosen := ""
	for _, item := range a.assessments {
		label := a.assessmentOverviewLabel(item)
		labels = append(labels, label)
		if item.AssessmentID == a.selectedAssessmentID {
			chosen = label
		}
	}
	next := chosen
	selectField(c, "analysis-results", &next, labels, locales.AssessmentResultsSelect()).Width(t.Space(72))
	if next != chosen {
		for _, item := range a.assessments {
			if a.assessmentOverviewLabel(item) == next {
				a.selectAssessment(item.AssessmentID)
				break
			}
		}
	}
}

func (a *app) analysisOverview(c *ui.Context, p uiPalette) {
	t := p.theme
	rows := a.assessmentRows
	scoreMean := 0.0
	classMean := 0.0
	improvementMean := 0.0
	passTotal := 0
	for _, row := range rows {
		scoreMean += row.Total
		classMean += row.ClassAverage
		improvementMean += row.ImprovementScore
		passTotal += row.PassCount
	}
	count := float64(len(rows))
	scoreMean = scoreMean / count
	classMean = classMean / count
	improvementMean = improvementMean / count
	sectionRule(c, p, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.AnalysisOverview(), -1)
			ui.Spacer(c)
			if a.assessmentRun.ID != 0 {
				ui.Textf(c, locales.AssessmentRunMeta(), formatTime(a.assessmentRun.CreatedAt)).
					FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
			}
		})
		ui.Row(c).Wrap().Gap(t.Space(8)).Children(func() {
			a.analysisMetric(c, p, locales.AnalysisGroupCount(), formatInt(len(rows)))
			a.analysisMetric(c, p, locales.AnalysisAverageScore(), formatDecimal(scoreMean, 1))
			a.analysisMetric(c, p, locales.AnalysisAverageClass(), formatDecimal(classMean, 1))
			a.analysisMetric(c, p, locales.AnalysisAverageImprovement(),
				formatDecimal(improvementMean, 1))
			a.analysisMetric(c, p, locales.AnalysisPassTotal(), formatInt(passTotal))
		})
	})
}

func (a *app) analysisMetric(c *ui.Context, p uiPalette, label, value string) {
	t := p.theme
	ui.Column(c).Gap(t.Space(0.5)).MinWidth(t.Space(28)).Children(func() {
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
		ui.Text(c, value).FontSize(sectionSize(t)).FontWeight(600).FontFeatures("tnum")
	})
}

// analysisByClass 按教学量化成绩排列班级，显示与全体均值的差距。
func (a *app) analysisByClass(c *ui.Context, p uiPalette) {
	t := p.theme
	type classSummary struct {
		id     int64
		name   string
		total  float64
		groups int
	}
	items := make([]classSummary, 0, len(a.assessmentRows))
	index := map[int64]int{}
	for _, row := range a.assessmentRows {
		at, ok := index[row.LeadClassID]
		if !ok {
			at = len(items)
			index[row.LeadClassID] = at
			items = append(items, classSummary{id: row.LeadClassID, name: row.LeadClassName})
		}
		items[at].total += row.Total
		items[at].groups++
	}
	if len(items) == 0 {
		return
	}
	overall := 0.0
	for at := range items {
		items[at].total /= float64(items[at].groups)
		overall += items[at].total
	}
	overall /= float64(len(items))
	sort.Slice(items, func(i, j int) bool { return items[i].total > items[j].total })
	labels := make([]string, 0, len(items))
	values := make([]float64, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.name)
		values = append(values, item.total)
	}
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisByClass()).FontSize(sectionSize(t)).FontWeight(600)
		a.analysisBars(c, p, labels, values, overall)
		ui.Column(c).FillWidth().Gap(0).Dividers(1, p.border).Children(func() {
			for _, item := range items {
				item := item
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).
					Padding(t.Space(2.5), 0).Children(func() {
					ui.Text(c, item.name).FontSize(bodySize(t)).Grow(1).SingleLine().
						Shrink(1).MinWidth(0)
					ui.Textf(c, locales.AnalysisGroupCountValue(), formatInt(item.groups)).
						FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
					ui.Text(c, formatDecimal(item.total, 1)).FontSize(bodySize(t)).
						FontWeight(600).FontFeatures("tnum").Width(t.Space(12)).
						TextAlign(ui.End)
					ui.Text(c, a.analysisDelta(item.total, overall)).FontSize(captionSize(t)).
						TextColor(p.muted).FontFeatures("tnum").Width(t.Space(12)).
						TextAlign(ui.End)
				})
			}
		})
	})
}

// analysisByTeacher 按教师个人得分排列，展示本次考核中教师的相对位置。
func (a *app) analysisByTeacher(c *ui.Context, p uiPalette) {
	t := p.theme
	scores := a.assessmentPersonal
	if len(scores) == 0 {
		return
	}
	type teacherSummary struct {
		name     string
		total    float64
		count    int
		subjects map[string]bool
	}
	items := make([]teacherSummary, 0, len(scores))
	index := map[int64]int{}
	for _, score := range scores {
		at, ok := index[score.TeacherID]
		if !ok {
			at = len(items)
			index[score.TeacherID] = at
			items = append(items, teacherSummary{
				name: score.TeacherName, subjects: map[string]bool{},
			})
		}
		items[at].total += score.Score
		items[at].count++
		if score.Subject != "" {
			items[at].subjects[score.Subject] = true
		}
	}
	overall := 0.0
	for at := range items {
		overall += items[at].total / float64(items[at].count)
	}
	overall /= float64(len(items))
	sort.Slice(items, func(i, j int) bool {
		return items[i].total/float64(items[i].count) > items[j].total/float64(items[j].count)
	})
	labels := make([]string, 0, len(items))
	values := make([]float64, 0, len(items))
	for at := range items {
		labels = append(labels, items[at].name)
		values = append(values, items[at].total/float64(items[at].count))
	}
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisByTeacher()).FontSize(sectionSize(t)).FontWeight(600)
		a.analysisBars(c, p, labels, values, overall)
		ui.Column(c).FillWidth().Gap(0).Dividers(1, p.border).Children(func() {
			for _, item := range items {
				item := item
				value := item.total / float64(item.count)
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).
					Padding(t.Space(2.5), 0).Children(func() {
					ui.Text(c, item.name).FontSize(bodySize(t)).Grow(1).SingleLine().
						Shrink(1).MinWidth(0)
					ui.Textf(c, locales.AnalysisTeacherMeta(), len(item.subjects)).
						FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
					ui.Text(c, formatDecimal(value, 1)).FontSize(bodySize(t)).
						FontWeight(600).FontFeatures("tnum").Width(t.Space(12)).
						TextAlign(ui.End)
					ui.Text(c, a.analysisDelta(value, overall)).FontSize(captionSize(t)).
						TextColor(p.muted).FontFeatures("tnum").Width(t.Space(12)).
						TextAlign(ui.End)
				})
			}
		})
	})
}

// analysisBySubject 按学科汇总均分与班级数，显示学科之间的差异。
func (a *app) analysisBySubject(c *ui.Context, p uiPalette) {
	t := p.theme
	groups := map[string][]store.StoredResultRow{}
	for _, row := range a.assessmentRows {
		groups[row.Subject] = append(groups[row.Subject], row)
	}
	if len(groups) == 0 {
		return
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return a.subjectName(keys[i], keys[i]) < a.subjectName(keys[j], keys[j])
	})
	labels := make([]string, 0, len(keys))
	values := make([]float64, 0, len(keys))
	overall := 0.0
	weight := 0
	for _, key := range keys {
		rows := groups[key]
		sum := 0.0
		for _, row := range rows {
			sum += row.Total
		}
		labels = append(labels, a.subjectName(key, key))
		values = append(values, sum/float64(len(rows)))
		overall += sum
		weight += len(rows)
	}
	overall /= float64(weight)
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisBySubject()).FontSize(sectionSize(t)).FontWeight(600)
		a.analysisBars(c, p, labels, values, overall)
	})
}

// analysisBars 用同一组柱子展示分布，基线为全体均值。
func (a *app) analysisBars(c *ui.Context, p uiPalette, labels []string, values []float64,
	mean float64) {
	t := p.theme
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		for index, label := range labels {
			value := values[index]
			width := float32(0)
			if mean > 0 {
				width = float32(value / mean * 100)
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				ui.Text(c, label).FontSize(bodySize(t)).Width(t.Space(20)).SingleLine()
				ui.Box(c).Grow(1).Height(t.Space(1.5)).Radius(t.Space(0.75)).
					Background(p.fill).Children(func() {
					ui.Box(c).WidthPercent(width).MaxWidthPercent(100).FillHeight().
						Radius(t.Space(0.75)).Background(p.accent)
				})
				ui.Text(c, formatDecimal(value, 1)).FontSize(captionSize(t)).
					FontFeatures("tnum").Width(t.Space(10)).TextAlign(ui.End)
			})
		}
	})
}

func (a *app) analysisDelta(value, mean float64) string {
	delta := value - mean
	if delta > 0.05 {
		return "+" + formatDecimal(delta, 1)
	}
	if delta < -0.05 {
		return formatDecimal(delta, 1)
	}
	return formatDecimal(0, 1)
}
