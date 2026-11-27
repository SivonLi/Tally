package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"tally/internal/store"
	"tally/locales"
)

func chartSeriesColors(p uiPalette) []ui.Color {
	return []ui.Color{p.info, p.success, p.warning, p.ink, p.danger, p.accentInk}
}

// -----------------------------------------------------------------------------
// 学生三年完整档案页
// -----------------------------------------------------------------------------

func (a *app) studentProfilePage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.studentProfileLoading && a.studentProfile.Student.ID != a.selectedStudentID {
		ui.Column(c).Fill().Center().Gap(t.Space(3)).Children(func() {
			ui.Spinner(c).Label(locales.StudentProfileLoading())
			ui.Text(c, locales.StudentProfileLoading()).FontSize(bodySize(t)).TextColor(p.muted)
		})
		return
	}
	if a.studentProfile.Student.ID != a.selectedStudentID || a.studentProfile.Student.ID <= 0 {
		detail := textOr(a.studentProfileError, a.dataError)
		if detail == "" {
			detail = locales.StudentProfileLoadErrorDetail()
		}
		ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				if backButton(c, p).Clicked() {
					a.closeStudentProfile()
					return
				}
			})
			errorState(c, p, locales.StudentProfileLoadErrorTitle(), detail, func() {
				a.openStudentProfileWithHistory(a.selectedStudentID, len(a.studentProfileStack) > 0)
			}, nil)
		})
		return
	}

	prof := a.studentProfile
	cohortName := a.cohortName(prof.Cohort.EnrollmentYear)

	ui.Scroll(c.Key("student-profile-page")).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			// 1. 面包屑导航（支持复读前溯多层返回栈）
			crumbs := make([]string, 0, 3+len(a.studentProfileStack))
			crumbs = append(crumbs, locales.CohortDetailBackToList(), cohortName)
			for _, entry := range a.studentProfileStack {
				crumbs = append(crumbs, entry.Name)
			}
			crumbs = append(crumbs, prof.Student.Name)
			chosen := len(crumbs) - 1
			if ui.Breadcrumbs(c, crumbs, &chosen).Label(locales.StudentDetailTitle()).Changed() {
				switch {
				case chosen == 0:
					a.studentProfileStack = nil
					a.studentProfileOpen = false
					a.selectedStudentID = 0
					a.cohortDetailOpen = false
					return
				case chosen == 1:
					a.studentProfileStack = nil
					a.studentProfileOpen = false
					a.selectedStudentID = 0
					return
				case chosen >= 2 && chosen-2 < len(a.studentProfileStack):
					a.popStudentProfileStack(chosen - 2)
					return
				}
			}

			// 2. 顶部名片与主操作栏
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				if backButton(c, p).Clicked() {
					a.closeStudentProfile()
					return
				}
				sunbeam.Avatar(c, prof.Student.Name)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(48)).Gap(1).Children(func() {
					ui.Row(c).Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Text(c, prof.Student.Name).FontSize(titleSize(t)).FontWeight(600).
							Role(ui.RoleHeading).Level(1).SingleLine()
						stTone := toneAccent
						switch prof.Student.Status {
						case "active":
							stTone = toneSuccess
						case "transferred", "suspended", "borrowed":
							stTone = toneWarning
						case "left_school":
							stTone = toneDanger
						case "graduated":
							stTone = toneNeutral
						}
						badge(c, p, a.statusName(prof.Student.Status), stTone)
						if prof.Student.Walking {
							badge(c, p, locales.ImportFieldWalking(), toneNeutral)
						}
					})
					metaParts := []string{
						dashOr(prof.Student.StudentNo),
						dashOr(prof.Student.ClassName),
						a.trackName(prof.Student.Track, prof.Student.Track),
						a.studentForeignLangName(prof.Student.ForeignLang),
					}
					if strings.TrimSpace(prof.Student.Elective) != "" {
						metaParts = append(metaParts, prof.Student.Elective)
					}
					if strings.TrimSpace(prof.Student.CandidateCode) != "" {
						metaParts = append(metaParts, locales.StudentProfileCandidateCode()+": "+prof.Student.CandidateCode)
					}
					ui.Text(c, strings.Join(metaParts, " · ")).
						FontSize(captionSize(t)).TextColor(p.muted).Pretty()
				})

				ui.Row(c).Wrap().Shrink(0).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					eventBtn := labelledButton(c, locales.StudentProfileRecordEvent(), iconEdit, false)
					eventBtn.Disabled(prof.Cohort.CurrentPhase == store.PhaseGraduated)
					if eventBtn.Clicked() {
						a.openStudentEventModal()
					}
					if labelledButton(c, locales.StudentProfileExportExcel(), iconExport, true).Clicked() {
						a.chooseStudentProfileExport()
					}
				})
			})

			if a.dataError != "" {
				notice(c, p, noticeDanger, a.dataError)
			}

			// 3. 复读前溯一键跳转条
			if prof.PreviousStudent != nil && prof.PreviousCohort != nil {
				linkText := fmt.Sprintf(locales.StudentProfilePreviousLink(),
					a.cohortName(prof.PreviousCohort.EnrollmentYear), prof.PreviousStudent.Name)
				row := linkRow(c, p, linkText, func() {
					ui.Icon(c, iconUndo).Size(16, 16).TextColor(p.info).Shrink(0)
					ui.Text(c, linkText).FontSize(bodySize(t)).TextColor(p.ink).Grow(1).Shrink(1).MinWidth(0)
					ui.Icon(c, iconArrowRight).Size(15, 15).TextColor(p.muted).Shrink(0)
				})
				if row.Clicked() {
					a.openStudentProfilePrevious(prof.PreviousCohort.ID, prof.PreviousStudent.ID)
					return
				}
			}

			// 4. 档案摘要 KPI 与入学基数成绩
			ui.Grid(c).FillWidth().Columns(4).Gap(t.Space(3)).Children(func() {
				entryText := "-"
				if prof.Student.TotalScore != nil {
					entryText = formatDecimal(*prof.Student.TotalScore, 1)
				}
				kpiCard(c, p, kpiCardProps{
					Key:   "sp-entry",
					Title: locales.StudentProfileEntryTotal(),
					Value: entryText,
				})
				kpiCard(c, p, kpiCardProps{
					Key:   "sp-exams",
					Title: locales.StudentProfileExamCount(),
					Value: formatInt(len(prof.Exams)),
				})
				groupSub := ""
				if len(prof.Student.TeachingGroups) > 0 {
					groupSub = strings.Join(prof.Student.TeachingGroups, locales.ListSeparator())
				}
				kpiCard(c, p, kpiCardProps{
					Key:      "sp-groups",
					Title:    locales.StudentTeachingGroups(),
					Value:    formatInt(len(prof.Student.TeachingGroups)),
					Subtitle: groupSub,
				})
				kpiCard(c, p, kpiCardProps{
					Key:      "sp-lang",
					Title:    locales.StudentProfileForeignLang(),
					Value:    a.studentForeignLangName(prof.Student.ForeignLang),
					Subtitle: strings.TrimSpace(prof.Student.Elective),
				})
			})

			if len(prof.EntryScores) > 0 {
				panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(2.5)).Children(func() {
					sectionHeading(c, p, locales.StudentProfileEntrySummaryTitle(), len(prof.EntryScores))
					ui.Row(c).FillWidth().Wrap().Gap(t.Space(2)).Children(func() {
						for _, es := range prof.EntryScores {
							ui.Box(c).Padding(t.Space(1), t.Space(2.5)).Radius(sunbeam.RadiusLG).
								Background(p.fill).Children(func() {
								ui.Textf(c, "%s: %s", a.subjectName(es.Subject, es.Subject), formatDecimal(es.Score, 1)).
									FontSize(captionSize(t)).FontWeight(500).TextColor(p.ink).FontFeatures("tnum")
							})
						}
					})
				})
			}

			// 5. 三年成绩与排名走势图（ui/chart）
			a.studentProfileChartSection(c, p, prof)

			// 6. 历次考试成绩明细表（ui.Table）
			a.studentProfileExamTableSection(c, p, prof)

			// 7. 学籍与分班异动时间线（student_events）
			a.studentProfileEventsSection(c, p, prof)
		})
	})
}

type studentTrendPoint struct {
	TickLabel string
	FullLabel string
	Value     float64
	OutOf     int
	Valid     bool
}

type studentTrendSeries struct {
	Key    string
	Label  string
	Color  ui.Color
	Points []studentTrendPoint
}

func (a *app) studentProfileChartSection(c *ui.Context, p uiPalette, prof store.StudentProfile) {
	t := p.theme
	if a.studentChartMetric < 0 || a.studentChartMetric > 3 {
		a.studentChartMetric = 0
	}

	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			sectionHeading(c, p, locales.StudentProfileTrendTitle(), -1)
		})

		prevMetric := a.studentChartMetric
		ui.Row(c).FillWidth().Children(func() {
			segmentedField(c, &a.studentChartMetric, []string{
				locales.StudentProfileMetricGradeRank(),
				locales.StudentProfileMetricTrackRank(),
				locales.StudentProfileMetricTotalScore(),
				locales.StudentProfileMetricSubject(),
			}, locales.StudentProfileTrendTitle())
		})
		if a.studentChartMetric != prevMetric {
			a.studentChartHover = -1
		}

		isRank := a.studentChartMetric == 0 || a.studentChartMetric == 1
		if isRank {
			ui.Text(c, locales.StudentProfileTrendHintRank()).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
		} else {
			ui.Text(c, locales.StudentProfileTrendHintScore()).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
		}

		availSubjects := a.studentProfileSubjects(prof)
		if a.studentChartMetric == 3 && len(availSubjects) > 0 {
			opts := make([]toggleCodeOption, 0, len(availSubjects))
			for _, subj := range availSubjects {
				opts = append(opts, toggleCodeOption{
					code:  subj,
					label: a.subjectName(subj, subj),
				})
			}
			toggleCodeGroup(c, &a.studentChartSubjects, opts, locales.StudentProfileSelectSubjects())
		}

		xLabels, seriesList := a.studentChartData(p, prof)
		hasAnyPoint := false
		for _, ser := range seriesList {
			for _, pt := range ser.Points {
				if pt.Valid {
					hasAnyPoint = true
					break
				}
			}
			if hasAnyPoint {
				break
			}
		}

		if len(xLabels) == 0 || !hasAnyPoint {
			smallEmptyState(c, p, locales.StudentProfileChartEmpty())
			return
		}

		// 极值/进退步摘要指标卡
		a.studentChartSummaryStrip(c, p, isRank, seriesList)

		// 多系列图例（单科模式）
		if len(seriesList) > 1 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				for _, ser := range seriesList {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Box(c).Size(8, 8).Radius(4).Background(ser.Color).Shrink(0)
						ui.Text(c, ser.Label).FontSize(captionSize(t)).TextColor(p.inkSecondary).SingleLine()
					})
				}
			})
		}

		a.renderStudentTrendPlot(c, p, isRank, xLabels, seriesList)
	})
}

func (a *app) studentChartData(p uiPalette, prof store.StudentProfile) ([]string, []studentTrendSeries) {
	palette := chartSeriesColors(p)
	var xLabels []string
	var fullLabels []string

	shortTick := func(phase, name string) string {
		pLabel := a.stageName(phase)
		combined := name
		if pLabel != "" {
			combined = pLabel + "·" + name
		}
		runes := []rune(combined)
		if len(runes) > 9 {
			return string(runes[:8]) + "…"
		}
		return combined
	}

	switch a.studentChartMetric {
	case 0, 1: // 年级排名 / 类别排名
		pts := make([]studentTrendPoint, 0, len(prof.Exams))
		for _, ex := range prof.Exams {
			tick := shortTick(ex.PhaseAtExam, ex.ExamName)
			xLabels = append(xLabels, tick)
			full := ex.ExamName
			if ex.PhaseAtExam != "" {
				full = a.stageName(ex.PhaseAtExam) + " · " + ex.ExamName
			}
			fullLabels = append(fullLabels, full)

			val := 0
			total := 0
			if a.studentChartMetric == 0 {
				val = ex.GradeRank
				total = ex.TotalStudents
			} else {
				val = ex.TrackRank
				total = ex.TrackTotalCount
			}
			pts = append(pts, studentTrendPoint{
				TickLabel: tick,
				FullLabel: full,
				Value:     float64(val),
				OutOf:     total,
				Valid:     val > 0,
			})
		}
		title := locales.StudentProfileMetricGradeRank()
		if a.studentChartMetric == 1 {
			title = locales.StudentProfileMetricTrackRank()
		}
		return xLabels, []studentTrendSeries{{
			Key:    "rank",
			Label:  title,
			Color:  palette[0],
			Points: pts,
		}}

	case 2: // 总分（串联入学总分锚点 + 历次考试总分）
		var pts []studentTrendPoint
		if prof.Student.TotalScore != nil {
			xLabels = append(xLabels, locales.StudentProfileEntryAnchor())
			pts = append(pts, studentTrendPoint{
				TickLabel: locales.StudentProfileEntryAnchor(),
				FullLabel: locales.StudentProfileEntryAnchor(),
				Value:     *prof.Student.TotalScore,
				Valid:     true,
			})
		}
		for _, ex := range prof.Exams {
			tick := shortTick(ex.PhaseAtExam, ex.ExamName)
			full := ex.ExamName
			if ex.PhaseAtExam != "" {
				full = a.stageName(ex.PhaseAtExam) + " · " + ex.ExamName
			}
			xLabels = append(xLabels, tick)
			pts = append(pts, studentTrendPoint{
				TickLabel: tick,
				FullLabel: full,
				Value:     ex.TotalScore,
				Valid:     ex.TotalScore > 0 || len(ex.Scores) > 0,
			})
		}
		return xLabels, []studentTrendSeries{{
			Key:    "total",
			Label:  locales.StudentProfileMetricTotalScore(),
			Color:  palette[0],
			Points: pts,
		}}

	default: // 单科成绩（多选）
		selected := a.studentChartSubjects
		if len(selected) == 0 {
			avail := a.studentProfileSubjects(prof)
			if len(avail) > 0 {
				selected = avail[:min(len(avail), 3)]
			}
		}
		entryMap := map[string]float64{}
		for _, es := range prof.EntryScores {
			entryMap[es.Subject] = es.Score
		}
		hasEntry := len(entryMap) > 0
		if hasEntry {
			xLabels = append(xLabels, locales.StudentProfileEntryAnchor())
			fullLabels = append(fullLabels, locales.StudentProfileEntryAnchor())
		}
		for _, ex := range prof.Exams {
			tick := shortTick(ex.PhaseAtExam, ex.ExamName)
			full := ex.ExamName
			if ex.PhaseAtExam != "" {
				full = a.stageName(ex.PhaseAtExam) + " · " + ex.ExamName
			}
			xLabels = append(xLabels, tick)
			fullLabels = append(fullLabels, full)
		}

		out := make([]studentTrendSeries, 0, len(selected))
		for sIdx, subj := range selected {
			pts := make([]studentTrendPoint, 0, len(xLabels))
			if hasEntry {
				val, ok := entryMap[subj]
				pts = append(pts, studentTrendPoint{
					TickLabel: locales.StudentProfileEntryAnchor(),
					FullLabel: locales.StudentProfileEntryAnchor(),
					Value:     val,
					Valid:     ok,
				})
			}
			for _, ex := range prof.Exams {
				val, ok := ex.Scores[subj]
				pts = append(pts, studentTrendPoint{
					TickLabel: shortTick(ex.PhaseAtExam, ex.ExamName),
					FullLabel: a.stageName(ex.PhaseAtExam) + " · " + ex.ExamName,
					Value:     val,
					Valid:     ok,
				})
			}
			out = append(out, studentTrendSeries{
				Key:    subj,
				Label:  a.subjectName(subj, subj),
				Color:  palette[sIdx%len(palette)],
				Points: pts,
			})
		}
		return xLabels, out
	}
}

func (a *app) studentChartSummaryStrip(c *ui.Context, p uiPalette, isRank bool, seriesList []studentTrendSeries) {
	t := p.theme
	if len(seriesList) == 0 {
		return
	}
	primary := seriesList[0]
	var validVals []studentTrendPoint
	for _, pt := range primary.Points {
		if pt.Valid {
			validVals = append(validVals, pt)
		}
	}
	if len(validVals) == 0 {
		return
	}

	ui.Grid(c).FillWidth().Columns(3).Gap(t.Space(2.5)).Children(func() {
		if isRank {
			best := validVals[0]
			for _, pt := range validVals[1:] {
				if pt.Value < best.Value {
					best = pt
				}
			}
			latest := validVals[len(validVals)-1]
			latestText := fmt.Sprintf(locales.StudentProfileRankValue(), formatInt(int(math.Round(latest.Value))))
			if latest.OutOf > 0 {
				latestText = fmt.Sprintf(locales.StudentProfileRankOutOf(), formatInt(int(math.Round(latest.Value))), formatInt(latest.OutOf))
			}

			changeVal := locales.StudentProfileChangeSame()
			changeDir := 0
			if len(validVals) >= 2 {
				prev := validVals[len(validVals)-2]
				diff := int(math.Round(prev.Value - latest.Value))
				switch {
				case diff > 0:
					changeVal = fmt.Sprintf(locales.StudentProfileChangeUp(), formatInt(diff))
					changeDir = 1
				case diff < 0:
					changeVal = fmt.Sprintf(locales.StudentProfileChangeDown(), formatInt(-diff))
					changeDir = -1
				}
			}

			kpiCard(c, p, kpiCardProps{
				Key:      "sc-best-rank",
				Title:    locales.StudentProfileBestRank(),
				Value:    fmt.Sprintf(locales.StudentProfileRankValue(), formatInt(int(math.Round(best.Value)))),
				Subtitle: best.FullLabel,
			})
			kpiCard(c, p, kpiCardProps{
				Key:      "sc-latest-rank",
				Title:    locales.StudentProfileLatestRank(),
				Value:    latestText,
				Subtitle: latest.FullLabel,
			})
			panel(c, p).Grow(1).Shrink(1).MinWidth(0).Padding(t.Space(4), t.Space(4.5)).Gap(t.Space(1.5)).Children(func() {
				ui.Text(c, locales.StudentProfileChangePrev()).FontSize(captionSize(t)).FontWeight(500).
					TextColor(p.muted).FillWidth().SingleLine()
				deltaText(c, p, changeVal, changeDir).FontSize(titleSize(t))
				ui.Text(c, primary.Label).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
			})
			return
		}

		// 分数摘要（极值 / 最新 / 均值）
		best := validVals[0]
		sum := 0.0
		for _, pt := range validVals {
			if pt.Value > best.Value {
				best = pt
			}
			sum += pt.Value
		}
		latest := validVals[len(validVals)-1]
		avg := sum / float64(len(validVals))
		kpiCard(c, p, kpiCardProps{
			Key:      "sc-best-score",
			Title:    locales.StudentProfileHighestScore(),
			Value:    formatDecimal(best.Value, 1),
			Subtitle: primary.Label + " · " + best.FullLabel,
		})
		kpiCard(c, p, kpiCardProps{
			Key:      "sc-latest-score",
			Title:    locales.StudentProfileLatestScore(),
			Value:    formatDecimal(latest.Value, 1),
			Subtitle: latest.FullLabel,
		})
		kpiCard(c, p, kpiCardProps{
			Key:      "sc-avg-score",
			Title:    locales.StudentProfileAverageScore(),
			Value:    formatDecimal(avg, 1),
			Subtitle: primary.Label,
		})
	})
}

func (a *app) renderStudentTrendPlot(c *ui.Context, p uiPalette, isRank bool, xLabels []string, seriesList []studentTrendSeries) {
	t := p.theme
	x := chart.NewBand(xLabels...).Padding(0.25, 0.12)

	var allValues []float64
	for _, ser := range seriesList {
		for _, pt := range ser.Points {
			if pt.Valid {
				allValues = append(allValues, pt.Value)
			}
		}
	}
	lo, hi := chart.Extent(allValues...)
	var y chart.Linear
	if isRank {
		topMax := max(hi*1.1, 5)
		y = chart.NewLinear(topMax, 1).Nice(4)
		if y.Hi < 1 {
			y.Hi = 1
		}
		if y.Lo <= y.Hi {
			y.Lo = y.Hi + 10
		}
		y.Format = func(v float64) string {
			if v < 1 {
				return ""
			}
			return fmt.Sprintf("#%.0f", v)
		}
	} else {
		if lo == hi {
			lo = max(0, lo-10)
			hi = hi + 10
		} else {
			pad := (hi - lo) * 0.12
			lo = max(0, lo-pad)
			hi = hi + pad
		}
		y = chart.NewLinear(lo, hi).Nice(4)
	}

	yTicks := y.Ticks(4)
	if isRank && len(yTicks) > 0 {
		filtered := make([]chart.Tick, 0, len(yTicks))
		for _, tk := range yTicks {
			if tk.Value >= 1 {
				filtered = append(filtered, tk)
			}
		}
		yTicks = filtered
	}
	xTicks := x.Ticks()

	xs := make([]float32, len(xLabels))
	for i := range xLabels {
		xs[i] = x.At(i)
	}

	axisLabel := func(_ chart.Side, tick chart.Tick) {
		ui.Text(c, tick.Label).NoWrap().FontSize(11).TextColor(p.muted)
	}

	chart.Plot(c, chart.Axes{Left: yTicks, Bottom: xTicks, Label: axisLabel}, func(area ui.Element) {
		area.Draw(func(painter *ui.Painter, r ui.Rect) {
			for _, tk := range yTicks {
				py := chart.Y(r, tk.At)
				painter.Line(r.X, py, r.X+r.W, py, 1, p.border)
			}
			for _, ser := range seriesList {
				ys := make([]float32, len(ser.Points))
				for i, pt := range ser.Points {
					if pt.Valid {
						ys[i] = y.At(pt.Value)
					} else {
						ys[i] = float32(math.NaN())
					}
				}
				if len(seriesList) == 1 {
					painter.FillPath(chart.Area(r, xs, nil, ys, chart.CurveMonotone), ser.Color.Alpha(0.14))
				}
				painter.StrokePath(chart.Line(r, xs, ys, chart.CurveMonotone), 2, ser.Color)
			}
		})

		for sIdx, ser := range seriesList {
			for i, pt := range ser.Points {
				if !pt.Valid {
					continue
				}
				py := y.At(pt.Value)
				valText := formatDecimal(pt.Value, 1)
				if isRank {
					valText = fmt.Sprintf(locales.StudentProfileRankValue(), formatInt(int(math.Round(pt.Value))))
				}
				chart.Point(c.Key(fmt.Sprintf("sp-pt-%d-%d", sIdx, i)), xs[i], py, 8, 8).
					Radius(4).Background(ser.Color).
					Tooltip(fmt.Sprintf("%s · %s: %s", pt.FullLabel, ser.Label, valText))
			}
		}

		describe := func(i int) string {
			if i < 0 || i >= len(xLabels) {
				return ""
			}
			var parts []string
			title := xLabels[i]
			for _, ser := range seriesList {
				if i < len(ser.Points) && ser.Points[i].Valid {
					pt := ser.Points[i]
					if pt.FullLabel != "" {
						title = pt.FullLabel
					}
					if isRank {
						parts = append(parts, fmt.Sprintf("%s #%s", ser.Label, formatInt(int(math.Round(pt.Value)))))
					} else {
						parts = append(parts, fmt.Sprintf("%s %s", ser.Label, formatDecimal(pt.Value, 1)))
					}
				}
			}
			if len(parts) == 0 {
				return title
			}
			return title + ": " + strings.Join(parts, " · ")
		}

		chart.Select(c, area, &a.studentChartHover, xs, describe)
		if idx := a.studentChartHover; idx >= 0 && idx < len(xs) {
			chart.RuleX(c, xs[idx], 1).Background(p.borderStrong)
			for _, ser := range seriesList {
				if idx < len(ser.Points) && ser.Points[idx].Valid {
					chart.Point(c, xs[idx], y.At(ser.Points[idx].Value), 12, 12).
						Radius(6).Background(ser.Color).Border(2, p.panel)
				}
			}
			chart.Anchor(c, xs[idx], 1, ui.Center, ui.Start, func() {
				ui.Text(c, describe(idx)).NoWrap().FontSize(11).FontWeight(500).
					TextColor(p.ink).Padding(3, 8).Radius(6).
					Background(p.panel).Border(1, p.borderStrong)
			})
		}
	}).Label(locales.StudentProfileTrendTitle()).Height(220).Gap(t.Space(2))
}

func (a *app) studentProfileExamTableSection(c *ui.Context, p uiPalette, prof store.StudentProfile) {
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.StudentProfileExamTableTitle(), len(prof.Exams))
		if len(prof.Exams) == 0 {
			smallEmptyState(c, p, locales.StudentProfileExamTableEmpty())
			return
		}

		a.studentProfileExamTable.RowHeight = tableRowHeight(t)
		selected := a.studentSelectedExamIdx
		if selected < -1 || selected >= len(prof.Exams) {
			selected = -1
		}
		a.studentProfileExamTable.Selected = &selected
		a.studentProfileExamTable.Key = func(row int) any { return prof.Exams[row].ExamID }

		columns := []ui.TableColumn{
			{Title: locales.StudentProfileColPhase(), Width: t.Space(14)},
			{Title: locales.StudentProfileColExam(), Width: t.Space(36)},
			{Title: locales.StudentProfileColClassSnapshot(), Width: t.Space(18)},
			{Title: locales.ColumnStudentTotal(), Width: t.Space(16), Align: ui.End},
			{Title: locales.StudentProfileColGradeRank(), Width: t.Space(20), Align: ui.End},
			{Title: locales.StudentProfileColTrackRank(), Width: t.Space(20), Align: ui.End},
			{Title: locales.StudentProfileColClassRank(), Width: t.Space(18), Align: ui.End},
			{Title: locales.StudentProfileColSubjects(), Width: 0},
		}

		tableH := min(float32(len(prof.Exams)+1)*tableRowHeight(t)+t.Space(2), t.Space(72))
		table := ui.Table(c, &a.studentProfileExamTable, columns, len(prof.Exams), func(row, col int) {
			ex := prof.Exams[row]
			switch col {
			case 0:
				cellText(c, p, a.stageName(ex.PhaseAtExam))
			case 1:
				label := ex.ExamName
				if ex.ExamDate != "" {
					label += " (" + ex.ExamDate + ")"
				}
				cellText(c, p, label)
			case 2:
				cellText(c, p, dashOr(ex.ClassName))
			case 3:
				ui.Text(c, formatDecimal(ex.TotalScore, 1)).FontFeatures("tnum")
			case 4:
				if ex.GradeRank > 0 {
					txt := fmt.Sprintf(locales.StudentProfileRankValue(), formatInt(ex.GradeRank))
					if ex.TotalStudents > 0 {
						txt = fmt.Sprintf(locales.StudentProfileRankOutOf(), formatInt(ex.GradeRank), formatInt(ex.TotalStudents))
					}
					ui.Text(c, txt).FontFeatures("tnum")
				} else {
					ui.Text(c, "-").TextColor(p.muted)
				}
			case 5:
				if ex.TrackRank > 0 {
					txt := fmt.Sprintf(locales.StudentProfileRankValue(), formatInt(ex.TrackRank))
					if ex.TrackTotalCount > 0 {
						txt = fmt.Sprintf(locales.StudentProfileRankOutOf(), formatInt(ex.TrackRank), formatInt(ex.TrackTotalCount))
					}
					ui.Text(c, txt).FontFeatures("tnum")
				} else {
					ui.Text(c, "-").TextColor(p.muted)
				}
			case 6:
				if ex.ClassRank > 0 {
					ui.Text(c, fmt.Sprintf(locales.StudentProfileRankValue(), formatInt(ex.ClassRank))).FontFeatures("tnum")
				} else {
					ui.Text(c, "-").TextColor(p.muted)
				}
			case 7:
				cellText(c, p, a.formatExamSubjectsBrief(ex))
			}
		}).Label(locales.StudentProfileExamTableTitle()).FillWidth().Height(tableH)

		if table.Changed() && selected >= 0 {
			a.studentSelectedExamIdx = selected
		}

		if a.studentSelectedExamIdx >= 0 && a.studentSelectedExamIdx < len(prof.Exams) {
			ex := prof.Exams[a.studentSelectedExamIdx]
			rule(c)
			ui.Column(c.Key("sp-exam-detail-"+strconv.FormatInt(ex.ExamID, 10))).
				FillWidth().Padding(t.Space(3)).Radius(sunbeam.RadiusXL).
				Background(p.panelMuted).Border(1, p.border).Gap(t.Space(2.5)).Children(func() {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Text(c, fmt.Sprintf(locales.StudentProfileExamDetailTitle(), ex.ExamName)).
						FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink).Grow(1).Shrink(1).MinWidth(0)
					if plainButton(c.Key("sp-open-exam"), locales.StudentProfileOpenExamBtn()).Clicked() {
						a.openExamForCohort(ex.ExamID, prof.Cohort.ID)
						return
					}
					if modalCloseButton(c, p.sb).Clicked() {
						a.studentSelectedExamIdx = -1
						return
					}
				})
				subjKeys := a.sortedExamSubjectKeys(ex)
				ui.Row(c).FillWidth().Wrap().Gap(t.Space(2)).Children(func() {
					for _, subj := range subjKeys {
						val, hasScore := ex.Scores[subj]
						flag := ex.Flags[subj]
						rank := ex.SubjectRanks[subj]
						ui.Column(c).Padding(t.Space(2), t.Space(3)).Radius(sunbeam.RadiusLG).
							Background(p.panel).Border(1, p.border).Gap(2).Children(func() {
							ui.Text(c, a.subjectName(subj, subj)).FontSize(captionSize(t)).TextColor(p.muted)
							if hasScore {
								ui.Text(c, formatDecimal(val, 1)).FontSize(bodySize(t)).FontWeight(600).
									TextColor(p.ink).FontFeatures("tnum")
							} else if flag != "" {
								ui.Text(c, a.examFlagName(flag)).FontSize(captionSize(t)).FontWeight(500).
									TextColor(p.danger)
							}
							if rank > 0 {
								ui.Text(c, fmt.Sprintf(locales.StudentProfileSubjectRankLabel(), formatInt(rank))).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
							}
						})
					}
				})
			})
		}
	})
}

func (a *app) sortedExamSubjectKeys(ex store.StudentTrackPoint) []string {
	seen := map[string]bool{}
	for k := range ex.Scores {
		seen[k] = true
	}
	for k := range ex.Flags {
		seen[k] = true
	}
	ordered := make([]string, 0, len(seen))
	for _, s := range a.subjects {
		if seen[s.Key] {
			ordered = append(ordered, s.Key)
			delete(seen, s.Key)
		}
	}
	var rest []string
	for k := range seen {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	return append(ordered, rest...)
}

func (a *app) formatExamSubjectsBrief(ex store.StudentTrackPoint) string {
	keys := a.sortedExamSubjectKeys(ex)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if val, ok := ex.Scores[k]; ok {
			parts = append(parts, fmt.Sprintf("%s %s", a.subjectName(k, k), formatDecimal(val, 1)))
		} else if flag := ex.Flags[k]; flag != "" {
			parts = append(parts, fmt.Sprintf("%s %s", a.subjectName(k, k), a.examFlagName(flag)))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " · ")
}

func (a *app) studentProfileEventsSection(c *ui.Context, p uiPalette, prof store.StudentProfile) {
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.StudentProfileEventsTitle(), len(prof.Events))
		if len(prof.Events) == 0 {
			smallEmptyState(c, p, locales.StudentProfileEventsEmpty())
			return
		}
		ui.Column(c).FillWidth().Gap(0).Children(func() {
			for idx, ev := range prof.Events {
				if idx > 0 {
					rule(c)
				}
				evLabel, evTone := a.studentEventTypeLabel(ev.EventType)
				transText := a.formatStudentEventTransition(ev)
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).
					Padding(t.Space(2.5), 0).Gap(t.Space(2)).Children(func() {
					badge(c, p, evLabel, evTone)
					if ev.Phase != "" {
						badge(c, p, a.stageName(ev.Phase), toneNeutral)
					}
					ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(36)).Gap(1).Children(func() {
						if transText != "" {
							ui.Text(c, transText).FontSize(bodySize(t)).FontWeight(500).TextColor(p.ink)
						}
						if strings.TrimSpace(ev.Note) != "" {
							ui.Text(c, ev.Note).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
						}
					})
					ui.Text(c, formatTime(ev.OccurredAt)).FontSize(captionSize(t)).
						TextColor(p.muted).FontFeatures("tnum").Shrink(0)
				})
			}
		})
	})
}

func (a *app) studentEventModal(c *ui.Context, p uiPalette) {
	wasOpen := a.studentEventModalOpen
	if modal(c, &a.studentEventModalOpen, modalProps{
		Key:        "student-event-modal",
		Title:      locales.StudentEventModalTitle(),
		Width:      modalDefaultWidth,
		OkText:     locales.Save(),
		CancelText: locales.PlanCancel(),
	}, func() {
		a.dialogForm(c, func() {
			typeCodes := []string{"class_changed", "track_changed", "status_changed"}
			typeLabels := []string{
				locales.StudentEventTypeClassChanged(),
				locales.StudentEventTypeTrackChanged(),
				locales.StudentEventTypeStatusChanged(),
			}
			curTypeLabel := typeLabels[0]
			for i, code := range typeCodes {
				if code == a.studentEventType {
					curTypeLabel = typeLabels[i]
					break
				}
			}
			nextTypeLabel := curTypeLabel
			a.dialogField(c, "stu-ev-type", locales.StudentEventTypeLabel(), "", func() {
				selectField(c, "stu-ev-type-sel", &nextTypeLabel, typeLabels, locales.StudentEventTypeLabel()).FillWidth()
			})
			if nextTypeLabel != curTypeLabel {
				for i, lbl := range typeLabels {
					if lbl == nextTypeLabel {
						a.studentEventType = typeCodes[i]
						break
					}
				}
			}

			switch a.studentEventType {
			case "class_changed":
				classes := a.studentEventClasses
				if len(classes) == 0 {
					classes = a.classes
				}
				classOpts := make([]string, 0, len(classes))
				for _, cls := range classes {
					classOpts = append(classOpts, cls.Name)
				}
				a.dialogField(c, "stu-ev-class", locales.StudentEventTargetClass(), "", func() {
					selectField(c, "stu-ev-class-sel", &a.studentEventClassChoice, classOpts, locales.StudentEventTargetClass()).FillWidth()
				})

			case "track_changed":
				trackCodes := []string{"physics", "history"}
				trackLabels := []string{locales.TrackPhysics(), locales.TrackHistory()}
				curTrack := trackLabels[0]
				for i, code := range trackCodes {
					if code == a.studentEventTrackChoice {
						curTrack = trackLabels[i]
						break
					}
				}
				nextTrack := curTrack
				a.dialogField(c, "stu-ev-track", locales.StudentEventTargetTrack(), "", func() {
					selectField(c, "stu-ev-track-sel", &nextTrack, trackLabels, locales.StudentEventTargetTrack()).FillWidth()
				})
				if nextTrack != curTrack {
					for i, lbl := range trackLabels {
						if lbl == nextTrack {
							a.studentEventTrackChoice = trackCodes[i]
							break
						}
					}
				}
				a.dialogField(c, "stu-ev-elective", locales.StudentEventTargetElective(), "", func() {
					textField(c, &a.studentEventElective, locales.StudentEventTargetElective(), "")
				})
				langCodes := []string{"english", "japanese"}
				langLabels := []string{locales.SubjectEnglish(), locales.SubjectJapanese()}
				curLang := langLabels[0]
				for i, code := range langCodes {
					if code == a.studentEventForeignLang {
						curLang = langLabels[i]
						break
					}
				}
				nextLang := curLang
				a.dialogField(c, "stu-ev-lang", locales.StudentEventTargetForeignLang(), "", func() {
					selectField(c, "stu-ev-lang-sel", &nextLang, langLabels, locales.StudentEventTargetForeignLang()).FillWidth()
				})
				if nextLang != curLang {
					for i, lbl := range langLabels {
						if lbl == nextLang {
							a.studentEventForeignLang = langCodes[i]
							break
						}
					}
				}

			case "status_changed":
				stCodes := []string{"active", "transferred", "suspended", "borrowed", "left_school", "graduated"}
				stLabels := make([]string, 0, len(stCodes))
				curStatus := a.statusName(a.studentEventStatusChoice)
				for _, code := range stCodes {
					stLabels = append(stLabels, a.statusName(code))
				}
				nextStatus := curStatus
				a.dialogField(c, "stu-ev-status", locales.StudentEventTargetStatus(), "", func() {
					selectField(c, "stu-ev-status-sel", &nextStatus, stLabels, locales.StudentEventTargetStatus()).FillWidth()
				})
				if nextStatus != curStatus {
					for i, lbl := range stLabels {
						if lbl == nextStatus {
							a.studentEventStatusChoice = stCodes[i]
							break
						}
					}
				}
			}

			a.dialogField(c, "stu-ev-note", locales.StudentEventNoteLabel(), "", func() {
				textAreaField(c, &a.studentEventNote, locales.StudentEventNoteLabel(), "", 2, 4)
			})
			if a.studentEventError != "" {
				notice(c, p, noticeDanger, a.studentEventError)
			}
		})
	}) {
		a.submitStudentEvent()
	} else if wasOpen && !a.studentEventModalOpen {
		a.studentEventError = ""
	}
}

// -----------------------------------------------------------------------------
// 教师执教档案详情页
// -----------------------------------------------------------------------------

func (a *app) teacherProfilePage(c *ui.Context, p uiPalette) {
	t := p.theme
	if a.teacherProfileLoading && a.teacherProfile.Teacher.ID != a.selectedTeacherID {
		ui.Column(c).Fill().Center().Gap(t.Space(3)).Children(func() {
			ui.Spinner(c).Label(locales.TeacherProfileLoading())
			ui.Text(c, locales.TeacherProfileLoading()).FontSize(bodySize(t)).TextColor(p.muted)
		})
		return
	}
	if a.teacherProfile.Teacher.ID != a.selectedTeacherID || a.teacherProfile.Teacher.ID <= 0 {
		detail := textOr(a.teacherProfileError, a.dataError)
		if detail == "" {
			detail = locales.TeacherProfileLoadErrorDetail()
		}
		ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				if backButton(c, p).Clicked() {
					a.closeTeacherProfile()
					return
				}
			})
			errorState(c, p, locales.TeacherProfileLoadErrorTitle(), detail, func() {
				a.openTeacherProfileWithReturn(a.selectedTeacherID, a.teacherProfileReturnCohort)
			}, nil)
		})
		return
	}

	prof := a.teacherProfile

	ui.Scroll(c.Key("teacher-profile-page")).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			// 1. 面包屑与返回
			if a.teacherProfileReturnCohort > 0 {
				cohortTitle := locales.NavCohorts()
				for _, co := range a.cohorts {
					if co.ID == a.teacherProfileReturnCohort {
						cohortTitle = a.cohortName(co.EnrollmentYear)
						break
					}
				}
				crumb := 2
				if ui.Breadcrumbs(c, []string{cohortTitle, locales.CohortTabAssignments(), prof.Teacher.Name}, &crumb).
					Label(locales.TeachersDirectoryTitle()).Changed() {
					if crumb < 2 {
						a.closeTeacherProfile()
						return
					}
				}
			} else {
				crumb := 1
				if ui.Breadcrumbs(c, []string{locales.TeachersDirectoryTitle(), prof.Teacher.Name}, &crumb).
					Label(locales.TeachersDirectoryTitle()).Changed() {
					if crumb == 0 {
						a.closeTeacherProfile()
						return
					}
				}
			}

			// 2. 顶部名片与编辑/停用操作
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				if backButton(c, p).Clicked() {
					a.closeTeacherProfile()
					return
				}
				sunbeam.Avatar(c, prof.Teacher.Name)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(48)).Gap(1).Children(func() {
					ui.Row(c).Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						ui.Text(c, prof.Teacher.Name).FontSize(titleSize(t)).FontWeight(600).
							Role(ui.RoleHeading).Level(1).SingleLine()
						occLabel, occTone := a.teacherOccupancyBadge(prof.Occupancy.Status)
						badge(c, p, occLabel, occTone)
					})
					metaParts := []string{
						dashOr(prof.Teacher.TeacherNo),
						a.subjectName(prof.Teacher.Subject, prof.Teacher.Subject),
					}
					if strings.TrimSpace(prof.Teacher.Note) != "" {
						metaParts = append(metaParts, prof.Teacher.Note)
					}
					ui.Text(c, strings.Join(metaParts, " · ")).
						FontSize(captionSize(t)).TextColor(p.muted).Pretty()
				})

				ui.Row(c).Wrap().Shrink(0).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					if a.teacherProfileReturnCohort > 0 {
						if plainButton(c, locales.TeacherProfileBackToCohort()).Clicked() {
							a.closeTeacherProfile()
							return
						}
					}
					if labelledButton(c, locales.TeacherProfileEditBtn(), iconEdit, false).Clicked() {
						a.openEditTeacherModal()
					}
					if labelledButton(c, locales.TeacherProfileExportExcel(), iconExport, true).Clicked() {
						a.chooseTeacherProfileExport()
					}
					toggleLbl := locales.TeachersToggleInactive()
					if prof.Teacher.Status == store.TeacherStatusInactive {
						toggleLbl = locales.TeachersToggleActive()
					}
					if plainButton(c, toggleLbl).Clicked() {
						a.toggleTeacherProfileStatus()
					}
				})
			})

			if a.dataError != "" {
				notice(c, p, noticeDanger, a.dataError)
			}

			// 3. 实时占用大徽章卡
			a.teacherProfileOccupancyCard(c, p, prof)

			// 4. 累计执教届数 / 考核次数 / 最高分 / 最近分 指标卡
			bestScore := "-"
			latestScore := "-"
			if len(prof.CrossCohortScores) > 0 {
				hi := prof.CrossCohortScores[0].PersonalScore
				for _, pt := range prof.CrossCohortScores {
					if pt.PersonalScore > hi {
						hi = pt.PersonalScore
					}
				}
				bestScore = formatDecimal(hi, 1)
				latestScore = formatDecimal(prof.CrossCohortScores[len(prof.CrossCohortScores)-1].PersonalScore, 1)
			}
			ui.Grid(c).FillWidth().Columns(4).Gap(t.Space(3)).Children(func() {
				kpiCard(c, p, kpiCardProps{
					Key:   "tp-cohorts",
					Title: locales.TeacherProfileMetricCohorts(),
					Value: formatInt(len(prof.CohortHistories)),
				})
				kpiCard(c, p, kpiCardProps{
					Key:   "tp-assessments",
					Title: locales.TeacherProfileMetricAssessments(),
					Value: formatInt(len(prof.CrossCohortScores)),
				})
				kpiCard(c, p, kpiCardProps{
					Key:   "tp-best",
					Title: locales.TeacherProfileMetricBestScore(),
					Value: bestScore,
				})
				kpiCard(c, p, kpiCardProps{
					Key:   "tp-latest",
					Title: locales.TeacherProfileMetricLatestScore(),
					Value: latestScore,
				})
			})

			// 5. 当前在任班级与最近考核表现（含班主任徽章与空闲快捷跳转）
			a.teacherProfileActiveSection(c, p, prof)

			// 6. 跨届个人得分趋势图（按届分色）
			a.teacherProfileChartSection(c, p, prof)

			// 7. 按届折叠的历史带班与历次考核履历
			a.teacherProfileHistorySection(c, p, prof)
		})
	})
}

func (a *app) teacherProfileOccupancyCard(c *ui.Context, p uiPalette, prof store.TeacherProfile) {
	t := p.theme
	occ := prof.Occupancy
	occLabel, occTone := a.teacherOccupancyBadge(occ.Status)

	cohortNamesMap := map[int64]bool{}
	var cohortNames []string
	for _, item := range occ.ActiveAssignments {
		if !cohortNamesMap[item.CohortID] {
			cohortNamesMap[item.CohortID] = true
			cohortNames = append(cohortNames, a.cohortName(item.EnrollmentYear))
		}
	}
	joinedCohorts := strings.Join(cohortNames, locales.ListSeparator())

	var desc string
	switch occ.Status {
	case store.TeacherStatusActiveSingle:
		desc = fmt.Sprintf(locales.TeacherProfileOccSingleDesc(), joinedCohorts, formatInt(len(occ.ActiveAssignments)))
	case store.TeacherStatusActiveMulti:
		desc = fmt.Sprintf(locales.TeacherProfileOccMultiDesc(), joinedCohorts)
	case store.TeacherStatusIdle:
		desc = locales.TeacherProfileOccIdleDesc()
	default:
		desc = locales.TeacherProfileOccInactiveDesc()
	}

	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(2)).Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2.5)).Children(func() {
			ui.Text(c, locales.TeacherProfileOccupancyTitle()).FontSize(sectionSize(t)).FontWeight(600).TextColor(p.ink)
			badge(c, p, occLabel, occTone)
			ui.Spacer(c)
			if occ.Status == store.TeacherStatusIdle && occ.LastRelease != nil {
				target := strings.TrimSpace(a.cohortName(occ.LastRelease.EnrollmentYear) + " " + occ.LastRelease.ClassName)
				relText := fmt.Sprintf(locales.TeacherProfileReleasedFrom(),
					target, a.teacherReleaseReasonLabel(occ.LastRelease.EndReason))
				badge(c, p, relText, toneNeutral)
			}
		})
		ui.Text(c, desc).FontSize(bodySize(t)).TextColor(p.inkSecondary).Pretty()
	})
}

func (a *app) teacherProfileActiveSection(c *ui.Context, p uiPalette, prof store.TeacherProfile) {
	t := p.theme
	occ := prof.Occupancy

	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.TeacherProfileActiveDutiesTitle(), len(occ.ActiveAssignments))
		if len(occ.ActiveAssignments) == 0 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).
				Padding(t.Space(3)).Radius(sunbeam.RadiusXL).Background(p.panelMuted).Border(1, p.border).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(t.Space(48)).Gap(1).Children(func() {
					ui.Text(c, locales.TeacherProfileIdleCardTitle()).FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink)
					ui.Text(c, locales.TeacherProfileIdleCardHint()).FontSize(captionSize(t)).TextColor(p.muted).Pretty()
				})
				if prof.Teacher.Status != store.TeacherStatusInactive {
					if labelledButton(c, locales.TeacherProfileGoAssignBtn(), iconArrowRight, true).Clicked() {
						a.teacherProfileOpen = false
						a.teacherProfileReturnCohort = 0
						for _, co := range a.cohorts {
							if co.CurrentPhase != store.PhaseGraduated {
								a.openCohortDetail(co.ID)
								a.cohortDetailTab = 3
								break
							}
						}
						a.navigateTo(pageCohorts)
					}
				}
			})
			return
		}

		// 索引各届最近一次考试表现
		latestByCohort := map[int64]*store.TeacherExamHistory{}
		for i := range prof.CohortHistories {
			ch := &prof.CohortHistories[i]
			if len(ch.Exams) > 0 {
				latestByCohort[ch.CohortID] = &ch.Exams[len(ch.Exams)-1]
			}
		}

		ui.Column(c).FillWidth().Gap(0).Children(func() {
			for idx, item := range occ.ActiveAssignments {
				if idx > 0 {
					rule(c)
				}
				title := item.TeachingGroupName
				if item.Role == "head_teacher" {
					title = item.ClassName
				} else if title == "" {
					title = item.ClassName
				}
				ui.Column(c).FillWidth().Padding(t.Space(2.5), 0).Gap(t.Space(1)).Children(func() {
					ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
						badge(c, p, a.cohortName(item.EnrollmentYear), toneNeutral)
						badge(c, p, a.stageName(item.Phase), toneNeutral)
						ui.Text(c, dashOr(title)).FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink).SingleLine()
						if item.Subject != "" {
							badge(c, p, a.subjectName(item.Subject, item.Subject), toneAccent)
						}
						if item.Role == "head_teacher" {
							badge(c, p, locales.TeacherProfileRoleHead(), toneSuccess)
						} else {
							badge(c, p, a.teacherRoleLabel(item.Role), toneNeutral)
						}
					})
					if ex := latestByCohort[item.CohortID]; ex != nil {
						var matchedRow *store.StoredResultRow
						for rIdx := range ex.ClassRows {
							r := &ex.ClassRows[rIdx]
							if (item.TeachingGroupID > 0 && r.StatGroupName == item.TeachingGroupName) ||
								(item.ClassName != "" && r.LeadClassName == item.ClassName) {
								matchedRow = r
								break
							}
						}
						if matchedRow != nil {
							ui.Text(c, fmt.Sprintf(locales.TeacherProfileLatestPerf(),
								ex.ExamName,
								formatDecimal(ex.Score, 1),
								formatDecimal(matchedRow.ClassAverage, 1),
								formatDecimal(matchedRow.PassScore, 1))).
								FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Pretty()
						}
					}
				})
			}
		})
	})
}

func (a *app) teacherProfileChartSection(c *ui.Context, p uiPalette, prof store.TeacherProfile) {
	t := p.theme
	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.TeacherProfileCrossChartTitle(), len(prof.CrossCohortScores))
		if len(prof.CrossCohortScores) == 0 {
			smallEmptyState(c, p, locales.TeacherProfileCrossChartEmpty())
			return
		}

		// 从 CohortHistories 映射每条考核记录所属的届别（EnrollmentYear）
		yearByAssessment := map[int64]int{}
		var cohortYears []int
		seenYear := map[int]bool{}
		for _, ch := range prof.CohortHistories {
			for _, ex := range ch.Exams {
				yearByAssessment[ex.AssessmentID] = ch.EnrollmentYear
				if !seenYear[ch.EnrollmentYear] {
					seenYear[ch.EnrollmentYear] = true
					cohortYears = append(cohortYears, ch.EnrollmentYear)
				}
			}
		}
		sort.Ints(cohortYears)

		palette := chartSeriesColors(p)
		colorByYear := map[int]ui.Color{}
		for idx, yr := range cohortYears {
			colorByYear[yr] = palette[idx%len(palette)]
		}

		if len(cohortYears) > 0 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				for _, yr := range cohortYears {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Box(c).Size(8, 8).Radius(4).Background(colorByYear[yr]).Shrink(0)
						ui.Text(c, a.cohortName(yr)).FontSize(captionSize(t)).TextColor(p.inkSecondary).SingleLine()
					})
				}
			})
		}

		xLabels := make([]string, len(prof.CrossCohortScores))
		vals := make([]float64, len(prof.CrossCohortScores))
		pointYears := make([]int, len(prof.CrossCohortScores))
		for i, pt := range prof.CrossCohortScores {
			yr := yearByAssessment[pt.AssessmentID]
			pointYears[i] = yr
			lbl := pt.ExamName
			if yr > 0 {
				lbl = fmt.Sprintf("%d·%s", yr%100, pt.ExamName)
			}
			runes := []rune(lbl)
			if len(runes) > 9 {
				lbl = string(runes[:8]) + "…"
			}
			xLabels[i] = lbl
			vals[i] = pt.PersonalScore
		}

		x := chart.NewBand(xLabels...).Padding(0.25, 0.12)
		lo, hi := chart.Extent(vals...)
		if lo == hi {
			lo = max(0, lo-10)
			hi = hi + 10
		} else {
			pad := (hi - lo) * 0.12
			lo = max(0, lo-pad)
			hi = hi + pad
		}
		y := chart.NewLinear(lo, hi).Nice(4)
		yTicks := y.Ticks(4)
		xTicks := x.Ticks()

		xs := make([]float32, len(prof.CrossCohortScores))
		ys := make([]float32, len(prof.CrossCohortScores))
		for i, v := range vals {
			xs[i] = x.At(i)
			ys[i] = y.At(v)
		}

		axisLabel := func(_ chart.Side, tick chart.Tick) {
			ui.Text(c, tick.Label).NoWrap().FontSize(11).TextColor(p.muted)
		}

		chart.Plot(c, chart.Axes{Left: yTicks, Bottom: xTicks, Label: axisLabel}, func(area ui.Element) {
			area.Draw(func(painter *ui.Painter, r ui.Rect) {
				for _, tk := range yTicks {
					py := chart.Y(r, tk.At)
					painter.Line(r.X, py, r.X+r.W, py, 1, p.border)
				}
				// 按届分组连线（同一届历次考试连接成折线，跨届点按色彩区分）
				for _, yr := range cohortYears {
					col := colorByYear[yr]
					var segX, segY []float32
					for i := range prof.CrossCohortScores {
						if pointYears[i] == yr {
							segX = append(segX, xs[i])
							segY = append(segY, ys[i])
						}
					}
					if len(segX) >= 2 {
						painter.StrokePath(chart.Line(r, segX, segY, chart.CurveMonotone), 2, col)
					}
				}
				if len(cohortYears) == 0 && len(xs) >= 2 {
					painter.StrokePath(chart.Line(r, xs, ys, chart.CurveMonotone), 2, p.info)
				}
			})

			for i, pt := range prof.CrossCohortScores {
				col := p.info
				if cYr, ok := colorByYear[pointYears[i]]; ok {
					col = cYr
				}
				yrPrefix := ""
				if pointYears[i] > 0 {
					yrPrefix = a.cohortName(pointYears[i]) + " · "
				}
				chart.Point(c.Key("tp-pt-"+strconv.Itoa(i)), xs[i], ys[i], 9, 9).
					Radius(4.5).Background(col).
					Tooltip(fmt.Sprintf("%s%s: %s", yrPrefix, pt.ExamName, formatDecimal(pt.PersonalScore, 1)))
			}

			describe := func(i int) string {
				if i < 0 || i >= len(prof.CrossCohortScores) {
					return ""
				}
				pt := prof.CrossCohortScores[i]
				yrPrefix := ""
				if pointYears[i] > 0 {
					yrPrefix = a.cohortName(pointYears[i]) + " · "
				}
				return fmt.Sprintf("%s%s: %s", yrPrefix, pt.ExamName, formatDecimal(pt.PersonalScore, 1))
			}
			chart.Select(c, area, &a.teacherChartHover, xs, describe)
			if idx := a.teacherChartHover; idx >= 0 && idx < len(xs) {
				col := p.info
				if cYr, ok := colorByYear[pointYears[idx]]; ok {
					col = cYr
				}
				chart.RuleX(c, xs[idx], 1).Background(p.borderStrong)
				chart.Point(c, xs[idx], ys[idx], 12, 12).Radius(6).Background(col).Border(2, p.panel)
				chart.Anchor(c, xs[idx], 1, ui.Center, ui.Start, func() {
					ui.Text(c, describe(idx)).NoWrap().FontSize(11).FontWeight(500).
						TextColor(p.ink).Padding(3, 8).Radius(6).
						Background(p.panel).Border(1, p.borderStrong)
				})
			}
		}).Label(locales.TeacherProfileCrossChartTitle()).Height(220).Gap(t.Space(2))
	})
}

func (a *app) teacherProfileHistorySection(c *ui.Context, p uiPalette, prof store.TeacherProfile) {
	t := p.theme
	if a.teacherCohortExpanded == nil {
		a.teacherCohortExpanded = map[int64]bool{}
	}

	panel(c, p).FillWidth().Padding(t.Space(4)).Gap(t.Space(3)).Children(func() {
		sectionHeading(c, p, locales.TeacherProfileHistoryTitle(), len(prof.CohortHistories))
		if len(prof.CohortHistories) == 0 {
			smallEmptyState(c, p, locales.TeacherProfileHistoryEmpty())
			return
		}

		for idx, ch := range prof.CohortHistories {
			if idx > 0 {
				rule(c)
			}
			ch := ch
			expanded, exists := a.teacherCohortExpanded[ch.CohortID]
			if !exists {
				expanded = ch.CurrentPhase != store.PhaseGraduated
			}
			statusLbl := locales.TeacherProfileHistoryCohortActive()
			if ch.CurrentPhase == store.PhaseGraduated {
				statusLbl = locales.TeacherProfileHistoryCohortGraduated()
			}
			headerTitle := fmt.Sprintf("%s · %s · %s",
				a.cohortName(ch.EnrollmentYear),
				statusLbl,
				fmt.Sprintf(locales.TeacherProfileHistorySummary(), formatInt(len(ch.Assignments)), formatInt(len(ch.Exams))),
			)

			ui.Collapsible(c.Key("tch-hist-"+strconv.FormatInt(ch.CohortID, 10)), headerTitle, &expanded, func() {
				ui.Column(c).FillWidth().Padding(t.Space(2), 0, t.Space(2), t.Space(4)).Gap(t.Space(3)).Children(func() {
					if len(ch.Assignments) > 0 {
						ui.Text(c, locales.TeacherProfileCohortDutiesTitle()).
							FontSize(captionSize(t)).FontWeight(600).TextColor(p.muted)
						ui.Column(c).FillWidth().Gap(t.Space(1.5)).Children(func() {
							for _, asg := range ch.Assignments {
								title := asg.TeachingGroupName
								if asg.Role == "head_teacher" || title == "" {
									title = asg.ClassName
								}
								ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).
									Padding(t.Space(1.5), t.Space(2.5)).Radius(sunbeam.RadiusLG).
									Background(p.panelMuted).Border(1, p.border).Children(func() {
									badge(c, p, a.stageName(asg.Phase), toneNeutral)
									ui.Text(c, dashOr(title)).FontSize(bodySize(t)).FontWeight(500).TextColor(p.ink)
									if asg.Subject != "" {
										badge(c, p, a.subjectName(asg.Subject, asg.Subject), toneAccent)
									}
									if asg.Role == "head_teacher" {
										badge(c, p, locales.TeacherProfileRoleHead(), toneSuccess)
									} else {
										badge(c, p, a.teacherRoleLabel(asg.Role), toneNeutral)
									}
									ui.Spacer(c)
									if asg.Status == "ended" {
										badge(c, p, locales.TeacherProfileDutyEnded(), toneNeutral)
										if asg.EndReason != "" {
											badge(c, p, a.teacherReleaseReasonLabel(asg.EndReason), toneWarning)
										}
									} else {
										badge(c, p, locales.TeacherProfileDutyActive(), toneSuccess)
									}
								})
							}
						})
					}

					if len(ch.Exams) > 0 {
						ui.Text(c, locales.TeacherProfileCohortExamsTitle()).
							FontSize(captionSize(t)).FontWeight(600).TextColor(p.muted)
						ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
							for _, ex := range ch.Exams {
								ui.Column(c).FillWidth().Padding(t.Space(2.5), t.Space(3)).
									Radius(sunbeam.RadiusXL).Background(p.panelMuted).Border(1, p.border).
									Gap(t.Space(1.5)).Children(func() {
									ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
										badge(c, p, a.stageName(ex.PhaseAtExam), toneNeutral)
										ui.Text(c, ex.ExamName).FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink).
											Grow(1).Shrink(1).MinWidth(0).SingleLine()
										if ex.ExamDate != "" {
											ui.Text(c, ex.ExamDate).FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
										}
										badge(c, p, a.subjectName(ex.Subject, ex.Subject), toneAccent)
										ui.Text(c, fmt.Sprintf(locales.AnalysisScorePoints(), formatDecimal(ex.Score, 1))).
											FontSize(bodySize(t)).FontWeight(600).TextColor(p.ink).FontFeatures("tnum")
									})
									if len(ex.GroupPoints) > 0 {
										pts := make([]string, 0, len(ex.GroupPoints))
										for _, gp := range ex.GroupPoints {
											pts = append(pts, formatDecimal(gp, 1))
										}
										ui.Text(c, fmt.Sprintf(locales.TeacherProfileGroupPoints(), strings.Join(pts, locales.ListSeparator()))).
											FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
									}
									for _, row := range ex.ClassRows {
										ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2.5)).Children(func() {
											ui.Text(c, dashOr(row.StatGroupName)).FontSize(captionSize(t)).FontWeight(500).TextColor(p.ink)
											ui.Textf(c, "%s: %s", locales.AnalysisClassAvg(), formatDecimal(row.ClassAverage, 1)).
												FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
											ui.Textf(c, "%s: %s", locales.AnalysisImprovementScore(), formatDecimal(row.ImprovementScore, 1)).
												FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
											ui.Textf(c, "%s: %s", locales.AnalysisPassScore(), formatDecimal(row.PassScore, 1)).
												FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
											ui.Textf(c, "%s: %s", locales.AnalysisCompletionScore(), formatDecimal(row.CompletionScore, 1)).
												FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
											ui.Textf(c, "%s #%s · %s", locales.AnalysisRank(), formatInt(row.Rank), formatDecimal(row.Total, 1)).
												FontSize(captionSize(t)).FontWeight(600).TextColor(p.ink).FontFeatures("tnum")
										})
									}
								})
							}
						})
					}
				})
			})
			a.teacherCohortExpanded[ch.CohortID] = expanded
		}
	})
}

func (a *app) editTeacherModal(c *ui.Context, p uiPalette) {
	wasOpen := a.editTeacherOpen
	if modal(c, &a.editTeacherOpen, modalProps{
		Key:        "edit-teacher-modal",
		Title:      locales.TeacherProfileEditTitle(),
		Width:      modalDefaultWidth,
		OkText:     locales.Save(),
		CancelText: locales.PlanCancel(),
	}, func() {
		a.dialogForm(c, func() {
			a.dialogField(c, "edit-t-no", locales.TeacherNo(), "", func() {
				textField(c, &a.editTeacherNo, locales.TeacherNo(), "")
			})
			a.dialogField(c, "edit-t-name", locales.ColumnName(), "", func() {
				textField(c, &a.editTeacherName, locales.ColumnName(), "")
			})
			subjLabels := make([]string, 0, len(a.subjects))
			subjCodes := make([]string, 0, len(a.subjects))
			curSubj := ""
			for _, s := range a.subjects {
				if !s.Enabled && s.Key != a.editTeacherSubject {
					continue
				}
				lbl := a.subjectName(s.Key, s.Name)
				subjLabels = append(subjLabels, lbl)
				subjCodes = append(subjCodes, s.Key)
				if s.Key == a.editTeacherSubject {
					curSubj = lbl
				}
			}
			if curSubj == "" && len(subjLabels) > 0 {
				curSubj = subjLabels[0]
				a.editTeacherSubject = subjCodes[0]
			}
			nextSubj := curSubj
			a.dialogField(c, "edit-t-subj", locales.TeacherSubject(), "", func() {
				selectField(c, "edit-t-subj-sel", &nextSubj, subjLabels, locales.TeacherSubject()).FillWidth()
			})
			if nextSubj != curSubj {
				for i, lbl := range subjLabels {
					if lbl == nextSubj {
						a.editTeacherSubject = subjCodes[i]
						break
					}
				}
			}
			a.dialogField(c, "edit-t-note", locales.TeacherProfileNoteLabel(), "", func() {
				textAreaField(c, &a.editTeacherNote, locales.TeacherProfileNoteLabel(), "", 2, 4)
			})
			if a.editTeacherError != "" {
				notice(c, p, noticeDanger, a.editTeacherError)
			}
		})
	}) {
		a.submitEditTeacher()
	} else if wasOpen && !a.editTeacherOpen {
		a.editTeacherError = ""
	}
}
