package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"tally/locales"
)

// analysisPage 教学数据多维分析工作台（教师趋势、班级趋势、学科对比、学生跟踪、考核对比）。
func (a *app) analysisPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ctx := context.Background()

	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		cohort := a.selectedCohort()
		var actions func()
		if cohort != nil {
			actions = func() {
				ui.Row(c).Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					a.analysisSavedViewsSelector(c, p)

					if labelledButton(c, locales.AnalysisSaveCurrentView(), iconCopy, false).Clicked() {
						a.analysisSaveViewName = ""
						a.analysisSaveViewError = ""
						a.analysisSaveViewOpen = true
					}

					if a.analysisSelectedViewID > 0 {
						if labelledButton(c, locales.AnalysisDeleteView(), iconTrash, false).Clicked() {
							a.analysisDeleteViewID = a.analysisSelectedViewID
							a.analysisDeleteViewName = a.analysisSavedViewName(a.analysisSelectedViewID)
							a.analysisDeleteViewOpen = a.analysisDeleteViewName != ""
						}
					}

					exportBtn := labelledButton(c, locales.AnalysisExport(), iconExport, false)
					if exportBtn.Clicked() {
						a.chooseAnalysisExport()
					}
				})
			}
		}

		// 顶栏：标题 + 视图操作 + 导出按钮
		pageHeading(c, p, locales.NavAnalysis(), locales.AnalysisPageSubtitle(), actions)

		if cohort == nil {
			emptyState(c, p, iconCohort, locales.WorkspaceEmpty(), locales.WorkspaceEmptyHint(), func() {
				if labelledButton(c, locales.WorkspaceCreate(), iconPlus, true).Clicked() {
					a.openNewCohort()
				}
			})
			return
		}

		// 消息通知
		if a.analysisError != "" {
			notice(c, p, noticeDanger, a.analysisError)
		}
		if a.analysisMessage != "" {
			notice(c, p, noticeSuccess, a.analysisMessage)
		}

		// 维度切换卡
		tabLabels := []string{
			locales.AnalysisTabTeacherTrend(),
			locales.AnalysisTabClassTrend(),
			locales.AnalysisTabSubjectComparison(),
			locales.AnalysisTabStudentTrack(),
			locales.AnalysisTabAssessmentComparison(),
		}
		prevTab := a.analysisTabIndex
		ui.Row(c).Children(func() {
			segmentedField(c, &a.analysisTabIndex, tabLabels, locales.NavAnalysis())
		})
		if a.analysisTabIndex != prevTab {
			a.reloadAnalysis(ctx)
		}

		// 过滤工具栏
		a.analysisFilterBar(c, p)

		// 主内容区（带滚动）
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(6)).Children(func() {
				switch a.analysisTabIndex {
				case 0:
					a.analysisTeacherTrendTab(c, p)
				case 1:
					a.analysisClassTrendTab(c, p)
				case 2:
					a.analysisSubjectComparisonTab(c, p)
				case 3:
					a.analysisStudentTrackTab(c, p)
				case 4:
					a.analysisAssessmentComparisonTab(c, p)
				}
			})
		})
	})
}

func (a *app) analysisSavedViewName(id int64) string {
	for _, view := range a.analysisSavedViews {
		if view.ID == id {
			return view.Name
		}
	}
	return ""
}

func (a *app) analysisDeleteViewDialog(c *ui.Context, p uiPalette) {
	title := fmt.Sprintf(locales.AnalysisDeleteViewTitle(), a.analysisDeleteViewName)
	confirm := fmt.Sprintf(locales.AnalysisDeleteViewConfirm(), a.analysisDeleteViewName)
	if confirmDangerModal(c, p, "analysis-delete-view", &a.analysisDeleteViewOpen, title,
		locales.AnalysisDeleteViewBody(), locales.AnalysisCancelBtn(), confirm) {
		a.deleteSavedView(context.Background(), a.analysisDeleteViewID)
		a.analysisDeleteViewID = 0
	}
}

func (a *app) analysisSavedViewsSelector(c *ui.Context, p uiPalette) {
	t := p.theme
	ctx := context.Background()

	options := []string{locales.AnalysisDefaultView()}
	for _, v := range a.analysisSavedViews {
		options = append(options, v.Name)
	}

	selectedName := locales.AnalysisDefaultView()
	for _, v := range a.analysisSavedViews {
		if v.ID == a.analysisSelectedViewID {
			selectedName = v.Name
			break
		}
	}

	next := selectedName
	selectField(c, "analysis-views", &next, options, locales.AnalysisSavedViews()).Width(t.Space(40))
	if next != selectedName {
		if next == locales.AnalysisDefaultView() {
			a.analysisSelectedViewID = 0
			a.reloadAnalysis(ctx)
		} else {
			for _, v := range a.analysisSavedViews {
				if v.Name == next {
					a.analysisSelectedViewID = v.ID
					a.applySavedView(ctx, v)
					break
				}
			}
		}
	}
}

func (a *app) analysisSaveViewModal(c *ui.Context, p uiPalette) {
	t := p.theme
	ctx := context.Background()
	wasOpen := a.analysisSaveViewOpen

	if modal(c, &a.analysisSaveViewOpen, modalProps{
		Key:        "analysis-save-view",
		Title:      locales.AnalysisSaveCurrentView(),
		OkText:     locales.AnalysisSaveViewBtn(),
		CancelText: locales.AnalysisCancelBtn(),
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			ui.Text(c, locales.AnalysisSaveViewPrompt()).FontSize(captionSize(t)).TextColor(p.muted)
			textField(c, &a.analysisSaveViewName, locales.AnalysisSaveViewPrompt(), "")
			if a.analysisSaveViewError != "" {
				notice(c, p, noticeDanger, a.analysisSaveViewError)
			}
		})
	}) {
		name := strings.TrimSpace(a.analysisSaveViewName)
		if name == "" {
			a.analysisSaveViewError = locales.AnalysisViewNameRequired()
		} else if msg := validateRuneLength(name, locales.AnalysisSavedViews(), maxNameRunes); msg != "" {
			a.analysisSaveViewError = msg
		} else {
			a.analysisSaveViewError = ""
			a.saveCurrentView(ctx, name)
			a.analysisSaveViewOpen = false
		}
	}
	if wasOpen && !a.analysisSaveViewOpen {
		a.analysisSaveViewError = ""
	}
}

func (a *app) analysisFilterBar(c *ui.Context, p uiPalette) {
	t := p.theme
	ctx := context.Background()

	ui.Row(c).Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Padding(t.Space(2), 0).Children(func() {
		switch a.analysisTabIndex {
		case 0:
			// 教师筛选
			teacherNames := a.uniqueTeacherNames()
			if len(teacherNames) > 0 && a.analysisTeacherName == "" {
				a.analysisTeacherName = teacherNames[0]
				a.reloadAnalysis(ctx)
			}
			nextTeacher := a.analysisTeacherName
			selectField(c, "analysis-filter-teacher", &nextTeacher, teacherNames, locales.AnalysisFilterTeacher()).Width(t.Space(40))
			if len(teacherNames) > 0 && nextTeacher != a.analysisTeacherName {
				a.analysisTeacherName = nextTeacher
				a.reloadAnalysis(ctx)
			}

		case 1:
			// 班级筛选
			classNames := make([]string, 0, len(a.classes))
			classMap := map[string]int64{}
			for _, cls := range a.classes {
				classNames = append(classNames, cls.Name)
				classMap[cls.Name] = cls.ID
			}
			currentName := ""
			for _, cls := range a.classes {
				if cls.ID == a.analysisClassID {
					currentName = cls.Name
					break
				}
			}
			if currentName == "" && len(a.classes) > 0 {
				currentName = classNames[0]
				a.analysisClassID = classMap[currentName]
				a.reloadAnalysis(ctx)
			}
			nextClass := currentName
			selectField(c, "analysis-filter-class", &nextClass, classNames, locales.AnalysisFilterClass()).Width(t.Space(40))
			if len(classNames) > 0 && nextClass != currentName {
				a.analysisClassID = classMap[nextClass]
				a.reloadAnalysis(ctx)
			}

		case 2:
			// 学科对比筛选：选本次考核 + 学科
			a.analysisAssessmentSelect(c, p)

			subjectKeys := a.allSubjectKeys()
			subjectLabels := []string{locales.AnalysisAllSubjects()}
			for _, s := range subjectKeys {
				subjectLabels = append(subjectLabels, a.subjectName(s, s))
			}
			currentLabel := locales.AnalysisAllSubjects()
			if a.analysisSubject != "" {
				currentLabel = a.subjectName(a.analysisSubject, a.analysisSubject)
			}
			nextSubject := currentLabel
			selectField(c, "analysis-filter-subject", &nextSubject, subjectLabels, locales.AnalysisFilterSubject()).Width(t.Space(40))
			if nextSubject != currentLabel {
				if nextSubject == locales.AnalysisAllSubjects() {
					a.analysisSubject = ""
				} else {
					for _, s := range subjectKeys {
						if a.subjectName(s, s) == nextSubject {
							a.analysisSubject = s
							break
						}
					}
				}
				a.reloadAnalysis(ctx)
			}

		case 3:
			// 学生追踪筛选（支持输入搜索）
			studentNames := make([]string, 0, len(a.students))
			studentMap := map[string]int64{}
			for _, s := range a.students {
				lbl := fmt.Sprintf("%s (%s - %s)", s.Name, s.StudentNo, s.ClassName)
				studentNames = append(studentNames, lbl)
				studentMap[lbl] = s.ID
			}
			currentLabel := ""
			for _, s := range a.students {
				if s.ID == a.analysisStudentID {
					currentLabel = fmt.Sprintf("%s (%s - %s)", s.Name, s.StudentNo, s.ClassName)
					break
				}
			}
			if currentLabel == "" && len(a.students) > 0 {
				currentLabel = studentNames[0]
				a.analysisStudentID = studentMap[currentLabel]
				a.reloadAnalysis(ctx)
			}
			if len(studentNames) == 0 {
				empty := ""
				selectField(c, "analysis-filter-student", &empty, nil, locales.AnalysisFilterStudent()).Width(t.Space(56))
			} else {
				nextStudent := currentLabel
				combo := ui.Combobox(c, &nextStudent, studentNames).
					Label(locales.AnalysisFilterStudent()).Width(t.Space(72)).
					MinHeight(t.Space(9)).FontSize(bodySize(t))
				if combo.Changed() && nextStudent != currentLabel {
					if id, ok := studentMap[nextStudent]; ok {
						a.analysisStudentID = id
						a.reloadAnalysis(ctx)
					}
				}
			}

		case 4:
			// 考核对比：提示选定考核
			ui.Text(c, locales.AnalysisSelectExamHint()).FontSize(captionSize(t)).TextColor(p.muted)
		}
	})
}

// -------------------------------------------------------------
// Tab 0: 教师趋势
// -------------------------------------------------------------
func (a *app) analysisTeacherTrendTab(c *ui.Context, p uiPalette) {
	t := p.theme
	res := a.analysisTeacherResult

	if res.TeacherName == "" {
		emptyState(c, p, iconAnalysis, locales.AnalysisTabTeacherTrend(), locales.AnalysisSelectTargetHint(), nil)
		return
	}

	if res.HasMultiplePlans {
		notice(c, p, noticeWarning, locales.AnalysisPlanDifferent())
	}
	if len(res.Points) < 2 {
		notice(c, p, noticeInfo, locales.AnalysisNeedMoreExams())
	}

	// 概览指标
	sectionRule(c, p, func() {
		sectionHeading(c, p, res.TeacherName+" · "+a.subjectName(res.Subject, res.Subject), -1)
		ui.Row(c).Wrap().Gap(t.Space(8)).Children(func() {
			a.analysisMetric(c, p, locales.AnalysisLatestScore(), formatDecimal(res.LatestPersonalScore, 1))
			a.analysisMetric(c, p, locales.AnalysisAvgScore(), formatDecimal(res.AveragePersonalScore, 1))
			a.analysisMetric(c, p, locales.AnalysisExamsCount(), formatInt(len(res.Points)))
		})
	})

	// 柱状图对比
	if len(res.Points) > 0 {
		sectionRule(c, p, func() {
			ui.Text(c, locales.AnalysisTabTeacherTrend()).FontSize(sectionSize(t)).FontWeight(600)
			labels := make([]string, 0, len(res.Points))
			values := make([]float64, 0, len(res.Points))
			for _, pt := range res.Points {
				labels = append(labels, pt.ExamName)
				values = append(values, pt.PersonalScore)
			}
			a.analysisBars(c, p, labels, values, res.AveragePersonalScore)
		})
	}

	// 历次明细列表
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisHistoryDetails()).FontSize(sectionSize(t)).FontWeight(600)
		ui.Column(c).FillWidth().Gap(0).Dividers(1, p.border).Children(func() {
			for _, pt := range res.Points {
				pt := pt
				ui.Column(c).FillWidth().Padding(t.Space(3), 0).Gap(t.Space(1.5)).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Text(c, pt.ExamName).FontSize(bodySize(t)).FontWeight(600).
							Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(pt.ExamName)
						ui.Text(c, pt.ExamDate).FontSize(captionSize(t)).TextColor(p.muted).
							FontFeatures("tnum").Shrink(0)
						ui.Text(c, fmt.Sprintf(locales.AnalysisScorePoints(), formatDecimal(pt.PersonalScore, 1))).
							FontSize(bodySize(t)).FontWeight(600).FontFeatures("tnum").Shrink(0)
					})
					for _, r := range pt.Rows {
						r := r
						groupTitle := strings.TrimSpace(r.LeadClassName + " " + a.subjectName(r.Subject, r.Subject))
						ui.Column(c).FillWidth().Gap(t.Space(1)).Padding(t.Space(1), 0).Children(func() {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, dashOr(groupTitle)).FontSize(bodySize(t)).FontWeight(500).
									Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(groupTitle)
								ui.Textf(c, "%s %s", locales.AnalysisRank(), formatInt(r.Rank)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
								ui.Textf(c, "%s %s", locales.AnalysisTier(), formatInt(r.TierIndex)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
								ui.Text(c, formatDecimal(r.Total, 1)).FontSize(bodySize(t)).
									FontFeatures("tnum").FontWeight(600).Shrink(0)
							})
							ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
								ui.Textf(c, "%s: %s", locales.AnalysisClassAvg(), formatDecimal(r.ClassAverage, 1)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
								if r.BaselineRatio > 0 {
									ui.Textf(c, "%s: %s", locales.AnalysisBaselineRatio(), formatDecimal(r.BaselineRatio, 4)).
										FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
								}
								ui.Textf(c, "%s: %s", locales.AnalysisImprovementScore(), formatDecimal(r.ImprovementScore, 1)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
								ui.Textf(c, "%s: %s", locales.AnalysisPassScore(), formatDecimal(r.PassRateScore, 1)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
								ui.Textf(c, "%s: %s", locales.AnalysisCompletionScore(), formatDecimal(r.CompletionScore, 1)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
							})
						})
					}
				})
			}
		})
	})
}

// -------------------------------------------------------------
// Tab 1: 班级趋势
// -------------------------------------------------------------
func (a *app) analysisClassTrendTab(c *ui.Context, p uiPalette) {
	t := p.theme
	res := a.analysisClassResult

	if res.ClassName == "" {
		emptyState(c, p, iconAnalysis, locales.AnalysisTabClassTrend(), locales.AnalysisSelectTargetHint(), nil)
		return
	}

	if len(res.Points) < 2 {
		notice(c, p, noticeInfo, locales.AnalysisNeedMoreExams())
	}

	// 概览指标
	sectionRule(c, p, func() {
		sectionHeading(c, p, res.ClassName, -1)
		ui.Row(c).Wrap().Gap(t.Space(8)).Children(func() {
			a.analysisMetric(c, p, locales.AnalysisCompletionScore(), formatDecimal(res.AverageCompletionScore, 1))
			a.analysisMetric(c, p, locales.AnalysisExamsCount(), formatInt(len(res.Points)))
		})
	})

	// 柱状图对比（完成率）
	if len(res.Points) > 0 {
		sectionRule(c, p, func() {
			ui.Text(c, locales.AnalysisCompletionScore()).FontSize(sectionSize(t)).FontWeight(600)
			labels := make([]string, 0, len(res.Points))
			values := make([]float64, 0, len(res.Points))
			for _, pt := range res.Points {
				labels = append(labels, pt.ExamName)
				values = append(values, pt.CompletionScore)
			}
			a.analysisBars(c, p, labels, values, res.AverageCompletionScore)
		})
	}

	// 历次学科表现
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisHistoryDetails()).FontSize(sectionSize(t)).FontWeight(600)
		ui.Column(c).FillWidth().Gap(0).Dividers(1, p.border).Children(func() {
			for _, pt := range res.Points {
				pt := pt
				ui.Column(c).FillWidth().Padding(t.Space(3), 0).Gap(t.Space(1.5)).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Text(c, pt.ExamName).FontSize(bodySize(t)).FontWeight(600).
							Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(pt.ExamName)
						ui.Text(c, pt.ExamDate).FontSize(captionSize(t)).TextColor(p.muted).
							FontFeatures("tnum").Shrink(0)
						ui.Text(c, locales.AnalysisCompletionScore()+": "+formatDecimal(pt.CompletionScore, 1)).
							FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
					})
					for _, r := range pt.Rows {
						r := r
						teachers := strings.Join(r.TeacherNames, locales.ListSeparator())
						subjectText := a.subjectName(r.Subject, r.Subject)
						ui.Column(c).FillWidth().Gap(t.Space(1)).Padding(t.Space(1), 0).Children(func() {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, dashOr(subjectText)).FontSize(bodySize(t)).FontWeight(500).
									Shrink(0).SingleLine().Tooltip(subjectText)
								ui.Text(c, dashOr(teachers)).FontSize(captionSize(t)).TextColor(p.muted).
									Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(teachers)
								ui.Textf(c, "%s %s", locales.AnalysisRank(), formatInt(r.Rank)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
								ui.Text(c, formatDecimal(r.Total, 1)).FontSize(bodySize(t)).
									FontFeatures("tnum").FontWeight(600).Shrink(0)
							})
							ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
								ui.Textf(c, "%s: %s", locales.AnalysisClassAvg(), formatDecimal(r.ClassAverage, 1)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
								if r.BaselineRatio > 0 {
									ui.Textf(c, "%s: %s", locales.AnalysisBaselineRatio(), formatDecimal(r.BaselineRatio, 4)).
										FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
								}
								ui.Textf(c, "%s: %s", locales.AnalysisImprovementScore(), formatDecimal(r.ImprovementScore, 1)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
								ui.Textf(c, "%s: %s", locales.AnalysisPassScore(), formatDecimal(r.PassRateScore, 1)).
									FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
							})
						})
					}
				})
			}
		})
	})
}

// -------------------------------------------------------------
// Tab 2: 学科对比
// -------------------------------------------------------------
func (a *app) analysisSubjectComparisonTab(c *ui.Context, p uiPalette) {
	t := p.theme
	res := a.analysisSubjectResult

	if res.GroupCount == 0 {
		emptyState(c, p, iconAnalysis, locales.AnalysisTabSubjectComparison(), locales.AssessmentNoResultsHint(), nil)
		return
	}

	// 概览指标
	sectionRule(c, p, func() {
		sectionHeading(c, p, res.ExamName+" · "+a.subjectName(res.Subject, res.Subject), -1)
		ui.Row(c).Wrap().Gap(t.Space(8)).Children(func() {
			a.analysisMetric(c, p, locales.AnalysisGroupCount(), formatInt(res.GroupCount))
			a.analysisMetric(c, p, locales.AnalysisAverageScore(), formatDecimal(res.AverageScore, 1))
			a.analysisMetric(c, p, locales.AnalysisAverageImprovement(), formatDecimal(res.AverageImprovement, 1))
			a.analysisMetric(c, p, locales.AnalysisPassTotal(), formatDecimal(res.AveragePassRate, 1))
		})
	})

	// 柱状图横向对比
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisByClass()).FontSize(sectionSize(t)).FontWeight(600)
		labels := make([]string, 0, len(res.Groups))
		values := make([]float64, 0, len(res.Groups))
		for _, g := range res.Groups {
			labels = append(labels, g.LeadClassName)
			values = append(values, g.Total)
		}
		a.analysisBars(c, p, labels, values, res.AverageScore)
	})

	// 详细排名列表（两行结构：主行展示排名/班级/教师/总分，次行 Wrap 展示指标明细）
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisRankings()).FontSize(sectionSize(t)).FontWeight(600)
		ui.Column(c).FillWidth().Gap(0).Dividers(1, p.border).Children(func() {
			for _, g := range res.Groups {
				g := g
				teachers := strings.Join(g.TeacherNames, locales.ListSeparator())
				ui.Column(c).FillWidth().Padding(t.Space(2), t.Space(3)).Gap(t.Space(1)).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Text(c, "#"+formatInt(g.Rank)).FontSize(bodySize(t)).FontWeight(600).
							FontFeatures("tnum").MinWidth(t.Space(10)).Shrink(0)
						ui.Text(c, dashOr(g.LeadClassName)).FontSize(bodySize(t)).FontWeight(500).
							SingleLine().Shrink(0).Tooltip(g.LeadClassName)
						ui.Text(c, dashOr(teachers)).FontSize(captionSize(t)).TextColor(p.muted).
							Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(teachers)
						ui.Text(c, formatDecimal(g.Total, 1)).FontSize(bodySize(t)).
							FontFeatures("tnum").FontWeight(600).Shrink(0)
						ui.Text(c, a.analysisDelta(g.Total, res.AverageScore)).FontSize(captionSize(t)).
							TextColor(p.muted).FontFeatures("tnum").MinWidth(t.Space(12)).TextAlign(ui.End).Shrink(0)
					})
					ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Textf(c, "%s: %s", locales.AnalysisClassAvg(), formatDecimal(g.ClassAverage, 1)).
							FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
						if g.BaselineRatio > 0 {
							ui.Textf(c, "%s: %s", locales.AnalysisBaselineRatio(), formatDecimal(g.BaselineRatio, 4)).
								FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
						}
						ui.Textf(c, "%s: %s", locales.AnalysisPassScore(), formatDecimal(g.PassRateScore, 1)).
							FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
						ui.Textf(c, "%s: %s", locales.AnalysisCompletionScore(), formatDecimal(g.CompletionScore, 1)).
							FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum")
					})
				})
			}
		})
	})
}

// -------------------------------------------------------------
// Tab 3: 学生跟踪
// -------------------------------------------------------------
func (a *app) analysisStudentTrackTab(c *ui.Context, p uiPalette) {
	t := p.theme
	res := a.analysisStudentResult

	if res.StudentName == "" {
		emptyState(c, p, iconStudents, locales.AnalysisTabStudentTrack(), locales.AnalysisSelectTargetHint(), nil)
		return
	}

	if len(res.Points) == 0 {
		emptyState(c, p, iconStudents, locales.AnalysisTabStudentTrack(), locales.AnalysisNoStudentData(), nil)
		return
	}

	// 概览信息
	sectionRule(c, p, func() {
		trackLabel := res.Track
		if trackLabel == "physics" {
			trackLabel = locales.TrackPhysics()
		} else if trackLabel == "history" {
			trackLabel = locales.TrackHistory()
		}
		sectionHeading(c, p, res.StudentName+" ("+res.StudentNo+" · "+res.ClassName+" · "+trackLabel+")", -1)
		ui.Row(c).Wrap().Gap(t.Space(8)).Children(func() {
			a.analysisMetric(c, p, locales.AnalysisExamsCount(), formatInt(len(res.Points)))
			lastPt := res.Points[len(res.Points)-1]
			a.analysisMetric(c, p, locales.AnalysisStudentScore(), formatDecimal(lastPt.TotalScore, 1))
			a.analysisMetric(c, p, locales.AnalysisGradeRank(), formatInt(lastPt.GradeRank)+" / "+formatInt(lastPt.TotalStudents))
		})
	})

	// 柱状图：总分变化
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisStudentScore()).FontSize(sectionSize(t)).FontWeight(600)
		labels := make([]string, 0, len(res.Points))
		values := make([]float64, 0, len(res.Points))
		overall := 0.0
		for _, pt := range res.Points {
			labels = append(labels, pt.ExamName)
			values = append(values, pt.TotalScore)
			overall += pt.TotalScore
		}
		if len(res.Points) > 0 {
			overall /= float64(len(res.Points))
		}
		a.analysisBars(c, p, labels, values, overall)
	})

	// 历次学科成绩列表
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisHistoryDetails()).FontSize(sectionSize(t)).FontWeight(600)
		ui.Column(c).FillWidth().Gap(0).Dividers(1, p.border).Children(func() {
			for _, pt := range res.Points {
				pt := pt
				ui.Column(c).FillWidth().Padding(t.Space(3), 0).Gap(t.Space(1)).Children(func() {
					ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Text(c, pt.ExamName).FontSize(bodySize(t)).FontWeight(600).
							Grow(1).Shrink(1).MinWidth(0).SingleLine().Tooltip(pt.ExamName)
						ui.Text(c, pt.ExamDate).FontSize(captionSize(t)).TextColor(p.muted).
							FontFeatures("tnum").Shrink(0)
						ui.Textf(c, "%s: %s", locales.AnalysisStudentScore(), formatDecimal(pt.TotalScore, 1)).
							FontSize(bodySize(t)).FontFeatures("tnum").FontWeight(600).Shrink(0)
						ui.Textf(c, "%s: %s / %s", locales.AnalysisGradeRank(), formatInt(pt.GradeRank), formatInt(pt.TotalStudents)).
							FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Shrink(0)
					})
					// 逐科小胶囊
					subjKeys := make([]string, 0, len(pt.Scores))
					for k := range pt.Scores {
						subjKeys = append(subjKeys, k)
					}
					sort.Strings(subjKeys)
					ui.Row(c).Wrap().Gap(t.Space(2)).Children(func() {
						for _, k := range subjKeys {
							sc := pt.Scores[k]
							ui.Box(c).Padding(t.Space(0.5), t.Space(2)).Radius(t.Space(1)).Background(p.fill).Children(func() {
								ui.Textf(c, "%s: %s", a.subjectName(k, k), formatDecimal(sc, 1)).FontSize(captionSize(t)).FontFeatures("tnum")
							})
						}
					})
				})
			}
		})
	})
}

// -------------------------------------------------------------
// Tab 4: 考核对比
// -------------------------------------------------------------
func (a *app) analysisAssessmentComparisonTab(c *ui.Context, p uiPalette) {
	t := p.theme
	ctx := context.Background()
	res := a.analysisComparisonResult

	// 顶部多选框供用户勾选要对比的考核批次
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisSelectExamHint()).FontSize(sectionSize(t)).FontWeight(600)
		ui.Row(c).Wrap().Gap(t.Space(4)).Children(func() {
			for _, item := range a.assessments {
				item := item
				checked := a.analysisComparisonAssessmentIDs[item.AssessmentID]
				label := a.assessmentOverviewLabel(item)
				prevChecked := checked
				checkboxField(c, &checked, label)
				if checked != prevChecked {
					if a.analysisComparisonAssessmentIDs == nil {
						a.analysisComparisonAssessmentIDs = map[int64]bool{}
					}
					a.analysisComparisonAssessmentIDs[item.AssessmentID] = checked
					a.reloadAnalysis(ctx)
				}
			}
		})
	})

	if len(res.Items) < 2 {
		notice(c, p, noticeInfo, locales.AnalysisNeedMoreExams())
		return
	}

	if res.PlanDifferent {
		notice(c, p, noticeWarning, locales.AnalysisPlanDifferent())
	}

	// 并排卡片对比（支持多场考试自动换行，避免窄屏挤压溢出）
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisTabAssessmentComparison()).FontSize(sectionSize(t)).FontWeight(600)
		ui.Row(c).FillWidth().Wrap().Gap(t.Space(4)).Children(func() {
			for _, item := range res.Items {
				item := item
				ui.Box(c).Grow(1).Shrink(1).MinWidth(t.Space(56)).Padding(t.Space(4)).
					Radius(t.Space(2)).Border(1, p.border).Background(p.panel).Children(func() {
					ui.Column(c).FillWidth().MinWidth(0).Gap(t.Space(2)).Children(func() {
						ui.Text(c, item.ExamName).FontSize(sectionSize(t)).FontWeight(600).
							SingleLine().MinWidth(0).Shrink(1).Tooltip(item.ExamName)
						meta := item.ExamDate + " · " + item.AcademicYear + " " + item.Semester
						ui.Text(c, meta).FontSize(captionSize(t)).TextColor(p.muted).
							SingleLine().MinWidth(0).Shrink(1).Tooltip(meta)
						rule(c)
						a.analysisMetric(c, p, locales.AnalysisTotalStudents(), formatInt(item.ParticipantCount))
						a.analysisMetric(c, p, locales.AnalysisGroupCount(), formatInt(item.GroupCount))
						a.analysisMetric(c, p, locales.AnalysisAverageScore(), formatDecimal(item.AverageScore, 1))
						a.analysisMetric(c, p, locales.AnalysisAverageClass(), formatDecimal(item.AverageClass, 1))
						a.analysisMetric(c, p, locales.AnalysisAverageImprovement(), formatDecimal(item.AverageImprovement, 1))
						a.analysisMetric(c, p, locales.AnalysisPassTotal(), formatInt(item.PassTotal))
					})
				})
			}
		})
	})
}

// -------------------------------------------------------------
// 辅助渲染方法
// -------------------------------------------------------------

func (a *app) uniqueTeacherNames() []string {
	set := map[string]bool{}
	for _, t := range a.teachers {
		if t.Name != "" {
			set[t.Name] = true
		}
	}
	for _, row := range a.assessmentRows {
		for _, name := range strings.Split(decodeNames(row.TeacherNames), "、") {
			name = strings.TrimSpace(name)
			if name != "" {
				set[name] = true
			}
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (a *app) allSubjectKeys() []string {
	set := map[string]bool{}
	for _, s := range a.subjects {
		set[s.Key] = true
	}
	for _, r := range a.assessmentRows {
		if r.Subject != "" {
			set[r.Subject] = true
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (a *app) analysisAssessmentSelect(c *ui.Context, p uiPalette) {
	t := p.theme
	ctx := context.Background()
	labels := make([]string, 0, len(a.assessments))
	chosen := ""
	for _, item := range a.assessments {
		label := a.assessmentOverviewLabel(item)
		labels = append(labels, label)
		if item.AssessmentID == a.selectedAssessmentID {
			chosen = label
		}
	}
	if len(labels) == 0 {
		empty := ""
		selectField(c, "analysis-results", &empty, nil, locales.AssessmentResultsSelect()).Width(t.Space(72))
		return
	}
	next := chosen
	selectField(c, "analysis-results", &next, labels, locales.AssessmentResultsSelect()).Width(t.Space(72))
	if next != chosen {
		for _, item := range a.assessments {
			if a.assessmentOverviewLabel(item) == next {
				a.selectAssessment(item.AssessmentID)
				a.reloadAnalysis(ctx)
				break
			}
		}
	}
}

func (a *app) analysisMetric(c *ui.Context, p uiPalette, label, value string) {
	t := p.theme
	ui.Column(c).Gap(t.Space(0.5)).MinWidth(t.Space(24)).Children(func() {
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
		ui.Text(c, value).FontSize(sectionSize(t)).FontWeight(600).FontFeatures("tnum")
	})
}

func (a *app) analysisBars(c *ui.Context, p uiPalette, labels []string, values []float64, mean float64) {
	t := p.theme
	maxVal := mean
	for _, value := range values {
		if value > maxVal {
			maxVal = value
		}
	}
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		for index, label := range labels {
			value := values[index]
			width := float32(0)
			if maxVal > 0 && value > 0 {
				width = float32(value / maxVal * 100)
				if width > 100 {
					width = 100
				}
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				ui.Text(c, dashOr(label)).FontSize(bodySize(t)).Width(t.Space(28)).Shrink(0).
					SingleLine().Tooltip(label)
				ui.Box(c).Grow(1).Shrink(1).MinWidth(0).Height(t.Space(1.5)).Radius(t.Space(1)).
					Background(p.fill).Children(func() {
					ui.Box(c).WidthPercent(width).MaxWidthPercent(100).FillHeight().
						Radius(t.Space(1)).Background(p.ink)
				})
				ui.Text(c, formatDecimal(value, 1)).FontSize(captionSize(t)).
					FontFeatures("tnum").Width(t.Space(18)).Shrink(0).TextAlign(ui.End)
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
