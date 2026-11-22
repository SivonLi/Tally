package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/feedback"

	"tally/locales"
)

// analysisPage 教学数据多维分析工作台（教师趋势、班级趋势、学科对比、学生跟踪、考核对比）。
func (a *app) analysisPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ctx := context.Background()

	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		// 顶栏：标题 + 视图操作 + 导出按钮
		pageHeading(c, p, locales.NavAnalysis(), locales.AnalysisPageSubtitle(), func() {
			ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				a.analysisSavedViewsSelector(c, p)

				if labelledButton(c, locales.AnalysisSaveCurrentView(), iconCopy, false).Clicked() {
					a.analysisSaveViewName = ""
					a.analysisSaveViewOpen = true
				}

				if a.analysisSelectedViewID > 0 {
					if labelledButton(c, locales.AnalysisDeleteView(), iconTrash, false).Clicked() {
						a.deleteSavedView(ctx, a.analysisSelectedViewID)
					}
				}

				exportBtn := labelledButton(c, locales.AnalysisExport(), iconExport, false)
				if exportBtn.Clicked() {
					a.chooseAnalysisExport()
				}
			})
		})

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
		segmentedField(c, &a.analysisTabIndex, tabLabels, locales.NavAnalysis())
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

	// 保存视图弹窗
	a.analysisSaveViewModal(c, p)
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
	if !a.analysisSaveViewOpen {
		return
	}
	t := p.theme
	ctx := context.Background()
	open := true

	if feedback.Modal(c, &open, feedback.ModalProps{
		Title:      locales.AnalysisSaveCurrentView(),
		OkText:     locales.AnalysisSaveViewBtn(),
		CancelText: locales.AnalysisCancelBtn(),
		Width:      t.Space(120),
	}, func() {
		ui.Column(c).Width(t.Space(120)).Gap(t.Space(3)).Children(func() {
			ui.Text(c, locales.AnalysisSaveViewPrompt()).FontSize(captionSize(t)).TextColor(p.muted)
			textField(c, &a.analysisSaveViewName, locales.AnalysisSaveViewPrompt(), "")
		})
	}) {
		if strings.TrimSpace(a.analysisSaveViewName) != "" {
			a.saveCurrentView(ctx, strings.TrimSpace(a.analysisSaveViewName))
			a.analysisSaveViewOpen = false
		}
	}
	if !open {
		a.analysisSaveViewOpen = false
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
			if len(teacherNames) == 0 {
				teacherNames = []string{locales.NoDataYet()}
			} else if a.analysisTeacherName == "" {
				a.analysisTeacherName = teacherNames[0]
				a.reloadAnalysis(ctx)
			}
			nextTeacher := a.analysisTeacherName
			if nextTeacher == "" {
				nextTeacher = teacherNames[0]
			}
			selectField(c, "analysis-filter-teacher", &nextTeacher, teacherNames, locales.AnalysisFilterTeacher()).Width(t.Space(40))
			if nextTeacher != a.analysisTeacherName && nextTeacher != locales.NoDataYet() {
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
			if len(classNames) == 0 {
				classNames = []string{locales.NoDataYet()}
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
			} else if currentName == "" {
				currentName = classNames[0]
			}
			nextClass := currentName
			selectField(c, "analysis-filter-class", &nextClass, classNames, locales.AnalysisFilterClass()).Width(t.Space(40))
			if nextClass != currentName && nextClass != locales.NoDataYet() {
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
			// 学生追踪筛选
			studentNames := make([]string, 0, len(a.students))
			studentMap := map[string]int64{}
			for _, s := range a.students {
				lbl := fmt.Sprintf("%s (%s - %s)", s.Name, s.StudentNo, s.ClassName)
				studentNames = append(studentNames, lbl)
				studentMap[lbl] = s.ID
			}
			if len(studentNames) == 0 {
				studentNames = []string{locales.NoDataYet()}
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
			} else if currentLabel == "" {
				currentLabel = studentNames[0]
			}
			nextStudent := currentLabel
			selectField(c, "analysis-filter-student", &nextStudent, studentNames, locales.AnalysisFilterStudent()).Width(t.Space(56))
			if nextStudent != currentLabel && nextStudent != locales.NoDataYet() {
				a.analysisStudentID = studentMap[nextStudent]
				a.reloadAnalysis(ctx)
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
						ui.Text(c, pt.ExamName).FontSize(bodySize(t)).FontWeight(600).Grow(1)
						ui.Text(c, pt.ExamDate).FontSize(captionSize(t)).TextColor(p.muted)
						ui.Text(c, formatDecimal(pt.PersonalScore, 1)+" 分").FontSize(bodySize(t)).FontWeight(600).FontFeatures("tnum")
					})
					for _, r := range pt.Rows {
						r := r
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
							ui.Text(c, r.LeadClassName+" "+a.subjectName(r.Subject, r.Subject)).FontSize(captionSize(t)).TextColor(p.muted).Width(t.Space(28))
							ui.Textf(c, "%s: %s", locales.AnalysisClassAvg(), formatDecimal(r.ClassAverage, 1)).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Textf(c, "%s: %s", locales.AnalysisImprovementScore(), formatDecimal(r.ImprovementScore, 1)).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Textf(c, "%s: %s", locales.AnalysisPassScore(), formatDecimal(r.PassRateScore, 1)).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Textf(c, "%s: %s", locales.AnalysisCompletionScore(), formatDecimal(r.CompletionScore, 1)).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Spacer(c)
							ui.Textf(c, "%s %d", locales.AnalysisRank(), r.Rank).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Textf(c, "%s %d", locales.AnalysisTier(), r.TierIndex).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Text(c, formatDecimal(r.Total, 1)).FontSize(bodySize(t)).FontFeatures("tnum").FontWeight(600)
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
						ui.Text(c, pt.ExamName).FontSize(bodySize(t)).FontWeight(600).Grow(1)
						ui.Text(c, pt.ExamDate).FontSize(captionSize(t)).TextColor(p.muted)
						ui.Text(c, locales.AnalysisCompletionScore()+": "+formatDecimal(pt.CompletionScore, 1)).FontSize(captionSize(t)).TextColor(p.muted)
					})
					for _, r := range pt.Rows {
						r := r
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
							ui.Text(c, a.subjectName(r.Subject, r.Subject)).FontSize(bodySize(t)).Width(t.Space(20))
							ui.Text(c, strings.Join(r.TeacherNames, "、")).FontSize(captionSize(t)).TextColor(p.muted).Width(t.Space(24))
							ui.Textf(c, "%s: %s", locales.AnalysisClassAvg(), formatDecimal(r.ClassAverage, 1)).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Textf(c, "%s: %s", locales.AnalysisImprovementScore(), formatDecimal(r.ImprovementScore, 1)).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Textf(c, "%s: %s", locales.AnalysisPassScore(), formatDecimal(r.PassRateScore, 1)).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Spacer(c)
							ui.Textf(c, "%s %d", locales.AnalysisRank(), r.Rank).FontSize(captionSize(t)).TextColor(p.muted)
							ui.Text(c, formatDecimal(r.Total, 1)).FontSize(bodySize(t)).FontFeatures("tnum").FontWeight(600)
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

	// 详细排名表格
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisRankings()).FontSize(sectionSize(t)).FontWeight(600)
		ui.Column(c).FillWidth().Gap(0).Dividers(1, p.border).Children(func() {
			for _, g := range res.Groups {
				g := g
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Padding(t.Space(2.5), 0).Children(func() {
					ui.Textf(c, "#%d", g.Rank).FontSize(bodySize(t)).FontWeight(600).Width(t.Space(8))
					ui.Text(c, g.LeadClassName).FontSize(bodySize(t)).Width(t.Space(18))
					ui.Text(c, strings.Join(g.TeacherNames, "、")).FontSize(captionSize(t)).TextColor(p.muted).Grow(1)
					ui.Textf(c, "%s: %s", locales.AnalysisClassAvg(), formatDecimal(g.ClassAverage, 1)).FontSize(captionSize(t)).TextColor(p.muted)
					ui.Textf(c, "%s: %s", locales.AnalysisPassScore(), formatDecimal(g.PassRateScore, 1)).FontSize(captionSize(t)).TextColor(p.muted)
					ui.Textf(c, "%s: %s", locales.AnalysisCompletionScore(), formatDecimal(g.CompletionScore, 1)).FontSize(captionSize(t)).TextColor(p.muted)
					ui.Text(c, formatDecimal(g.Total, 1)).FontSize(bodySize(t)).FontFeatures("tnum").FontWeight(600).Width(t.Space(12)).TextAlign(ui.End)
					ui.Text(c, a.analysisDelta(g.Total, res.AverageScore)).FontSize(captionSize(t)).TextColor(p.muted).FontFeatures("tnum").Width(t.Space(10)).TextAlign(ui.End)
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
			a.analysisMetric(c, p, locales.AnalysisGradeRank(), fmt.Sprintf("%d / %d", lastPt.GradeRank, lastPt.TotalStudents))
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
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
						ui.Text(c, pt.ExamName).FontSize(bodySize(t)).FontWeight(600).Grow(1)
						ui.Text(c, pt.ExamDate).FontSize(captionSize(t)).TextColor(p.muted)
						ui.Textf(c, "%s: %s", locales.AnalysisStudentScore(), formatDecimal(pt.TotalScore, 1)).FontSize(bodySize(t)).FontFeatures("tnum").FontWeight(600)
						ui.Textf(c, "%s: %d / %d", locales.AnalysisGradeRank(), pt.GradeRank, pt.TotalStudents).FontSize(captionSize(t)).TextColor(p.muted)
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

	// 并排卡片对比
	sectionRule(c, p, func() {
		ui.Text(c, locales.AnalysisTabAssessmentComparison()).FontSize(sectionSize(t)).FontWeight(600)
		ui.Row(c).FillWidth().Gap(t.Space(4)).Children(func() {
			for _, item := range res.Items {
				item := item
				ui.Box(c).Grow(1).Padding(t.Space(4)).Radius(t.Space(2)).Border(1, p.border).Background(p.panel).Children(func() {
					ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
						ui.Text(c, item.ExamName).FontSize(sectionSize(t)).FontWeight(600)
						ui.Text(c, item.ExamDate+" · "+item.AcademicYear+" "+item.Semester).FontSize(captionSize(t)).TextColor(p.muted)
						ui.Box(c).Height(1).FillWidth().Background(p.border)
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
		for _, name := range strings.Split(row.TeacherNames, "、") {
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
		labels = []string{locales.NoDataYet()}
		chosen = labels[0]
	}
	next := chosen
	selectField(c, "analysis-results", &next, labels, locales.AssessmentResultsSelect()).Width(t.Space(64))
	if next != chosen && next != locales.NoDataYet() {
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
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		for index, label := range labels {
			value := values[index]
			width := float32(0)
			if mean > 0 {
				width = float32(value / mean * 100)
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				ui.Text(c, label).FontSize(bodySize(t)).Width(t.Space(24)).SingleLine()
				ui.Box(c).Grow(1).Height(t.Space(1.5)).Radius(t.Space(0.75)).
					Background(p.fill).Children(func() {
					ui.Box(c).WidthPercent(width).MaxWidthPercent(100).FillHeight().
						Radius(t.Space(0.75)).Background(p.accent)
				})
				ui.Text(c, formatDecimal(value, 1)).FontSize(captionSize(t)).
					FontFeatures("tnum").Width(t.Space(12)).TextAlign(ui.End)
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
