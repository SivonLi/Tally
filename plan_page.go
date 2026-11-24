package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/AstraFoundry/design/packages/mygo/sunbeam"
	"github.com/egoist/mygo/ui"

	"tally/internal/assessment"
	"tally/internal/store"
	"tally/locales"
)

type planSaveChoice int

const (
	planSaveNone planSaveChoice = iota
	planSaveAsking
	planSaveUpdate
	planSaveNew
	planSaveCancel
)

type planOption struct {
	code  string
	label string
}

// 选项文字每帧重新读取，语言切换后立即生效。
func planGradeChoices() []planOption {
	return []planOption{
		{"g1", locales.GradeG1()},
		{"g2", locales.GradeG2()},
		{"g3", locales.GradeG3()},
	}
}

func planExamTypeChoices() []planOption {
	return []planOption{
		{"midterm", locales.ExamMidterm()},
		{"final", locales.ExamFinal()},
		{"gaokao", locales.ExamGaokao()},
	}
}

func planSubjectChoices() []planOption {
	return []planOption{
		{"chinese", locales.SubjectChinese()},
		{"math", locales.SubjectMath()},
		{"english", locales.SubjectEnglish()},
		{"physics", locales.SubjectPhysics()},
		{"chemistry", locales.SubjectChemistry()},
		{"biology", locales.SubjectBiology()},
		{"history", locales.SubjectHistory()},
		{"geography", locales.SubjectGeography()},
		{"politics", locales.SubjectPolitics()},
		{"japanese", locales.SubjectJapanese()},
	}
}

func planIndicatorLabel(key string) string {
	switch key {
	case "improvement":
		return locales.PlanIndicatorImprovement()
	case "pass":
		return locales.PlanIndicatorPass()
	case "completion":
		return locales.PlanIndicatorCompletion()
	}
	return key
}

// planFormulaShapeOther 是公式形状下拉里的自定义形状代码。
const planFormulaShapeOther = "other"

// planFormulaShapeLabel 是公式形状在下拉与预览里的文字。
func (a *app) planFormulaShapeLabel(key, code string) string {
	switch key + "." + code {
	case "improvement.class_grade_baseline":
		return locales.PlanFormulaClassGradeBaseline()
	case "improvement.class_grade":
		return locales.PlanFormulaClassGrade()
	case "improvement.class_baseline":
		return locales.PlanFormulaClassBaseline()
	case "pass.pass_target":
		return locales.PlanFormulaPassTarget()
	case "pass.pass_members":
		return locales.PlanFormulaPassMembers()
	case "completion.tier_weighted":
		return locales.PlanFormulaTierWeighted()
	}
	return code
}

// planFormulaShapeField 是指标公式的选项表单：
// 下拉选择公式形状，公式文本由形状生成；
// 自定义表达式只读回显，重选形状后覆盖。
func (a *app) planFormulaShapeField(c *ui.Context, p uiPalette, key string, formula *string) {
	t := p.theme
	shapes := assessment.IndicatorFormulaShapes(key)
	code := assessment.FormulaShapeFor(key, *formula)
	options := make([]planOption, 0, len(shapes)+1)
	for _, shape := range shapes {
		options = append(options, planOption{code: shape.Code, label: a.planFormulaShapeLabel(key, shape.Code)})
	}
	if code == "" {
		options = append(options, planOption{code: planFormulaShapeOther, label: locales.PlanFormulaCustom()})
		code = planFormulaShapeOther
	}
	a.codeSelect(c, &code, options, locales.PlanFormulaLabel(), func(value string) {
		if value == planFormulaShapeOther {
			return
		}
		if text := assessment.FormulaForShape(key, value); text != "" {
			*formula = text
		}
	})
	if code == planFormulaShapeOther {
		ui.Text(c, *formula).FontSize(captionSize(t)).TextColor(p.muted).SingleLine().Tooltip(*formula)
		return
	}
	ui.Text(c, a.planFormulaShapeLabel(key, code)).FontSize(captionSize(t)).TextColor(p.muted)
}

// planPage 是方案页：顶部的配置切换，下面是全部可编辑的配置项，末尾一个保存按钮。
func (a *app) planPage(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Column(c).Fill().Gap(t.Space(4)).Children(func() {
		pageHeading(c, p, locales.PlanLibraryTitle(), locales.PlanPageSubtitle(), nil)
		if len(a.plans) == 0 {
			emptyState(c, p, iconEmptyPlan, locales.PlanEmpty(), locales.PlanEmptyHint(), nil)
			return
		}
		if version := a.selectedVersion(); version == nil {
			a.planConfigVersionID = 0
		} else if a.planConfigVersionID != version.ID || a.planConfigBaseline != a.selectedPlanID {
			a.loadPlanConfig()
		}
		ui.Column(c).Grow(1).FillWidth().Gap(t.Space(3)).Children(func() {
			a.planConfigBar(c, p)
			if a.planError != "" {
				notice(c, p, noticeWarning, a.planError)
			}
			// 负边距抵消滚动条占位，左右补偿统一为 8。
			ui.Scroll(c.Key("plan-config")).Grow(1).MinHeight(0).
				Margin(0, -t.Space(2)).Padding(0, t.Space(2)).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space(5)).Padding(0, 0, t.Space(3), 0).Children(func() {
					a.planFieldSeq = 0
					a.planBasicsSection(c, p)
					a.planScopeSection(c, p)
					a.planDataSection(c, p)
					a.planWalkingSection(c, p)
					a.planIndicatorsSection(c, p)
					a.planTiersSection(c, p)
					a.planRankingSection(c, p)
					a.planPersonalSection(c, p)
					a.planProgramsSection(c, p)
					a.planValidationSection(c, p)
				})
			})
			a.planSaveBar(c, p)
		})
	})
}

// loadPlanConfig 把当前方案的版本读入可编辑配置。
func (a *app) loadPlanConfig() {
	version := a.selectedVersion()
	if version == nil {
		a.planConfigVersionID = 0
		return
	}
	a.planConfig = version.Config
	a.planConfigVersionID = version.ID
	a.planConfigBaseline = a.selectedPlanID
}

// switchPlan 切换当前编辑的方案，并读出它最新的版本。
func (a *app) switchPlan(id string) {
	if id == a.selectedPlanID {
		return
	}
	a.selectedPlanID = id
	a.versionIndex = -1
	a.planError = ""
	a.planMessage = ""
	a.reloadVersions(context.Background())
	a.loadPlanConfig()
}

func (a *app) planConfigBar(c *ui.Context, p uiPalette) {
	t := p.theme
	plan := a.selectedPlan()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
		ui.Column(c).Width(t.Space(80)).Shrink(0).Children(func() {
			a.codeSelect(c, &a.selectedPlanID, a.planChoices(), locales.PlanSelectorLabel(), func(value string) {
				a.switchPlan(value)
			})
		})
		if plan != nil {
			kindBadge(c, p, plan.Kind, a.kindLabel(plan.Kind))
		}
	})
}

func (a *app) planChoices() []planOption {
	options := make([]planOption, 0, len(a.plans))
	for _, plan := range a.plans {
		options = append(options, planOption{strconv.FormatInt(plan.ID, 10), plan.Name})
	}
	return options
}

// planSection 用 Sunbeam 表单分组卡片显示方案的一个分区。
func (a *app) planSection(c *ui.Context, p uiPalette, title, desc string, body func()) {
	formGroup(c, p, title, desc, nil, body)
}

// codeSelect 是保存代码的下拉框，选项文字跟随语言。
func (a *app) codeSelect(c *ui.Context, value *string, options []planOption, label string, onChange func(string)) {
	a.planFieldSeq++
	labels := make([]string, 0, len(options))
	chosenLabel := ""
	for _, option := range options {
		labels = append(labels, option.label)
		if option.code == *value {
			chosenLabel = option.label
		}
	}
	if chosenLabel == "" && len(labels) > 0 {
		chosenLabel = labels[0]
	}
	current := chosenLabel
	selectField(c, "plan-code-"+strconv.Itoa(a.planFieldSeq), &current, labels, label).FillWidth()
	if current != chosenLabel {
		for _, option := range options {
			if option.label == current {
				if onChange != nil {
					onChange(option.code)
				} else {
					*value = option.code
				}
				break
			}
		}
	}
}

func (a *app) codeField(c *ui.Context, value *string, options []planOption, label string) {
	a.codeSelect(c, value, options, label, nil)
}

// toggleCodes 是多选开关组，直接读写配置里的代码列表。
func (a *app) toggleCodes(c *ui.Context, values *[]string, options []planOption, label string) {
	items := make([]toggleCodeOption, 0, len(options))
	for _, option := range options {
		items = append(items, toggleCodeOption{code: option.code, label: option.label})
	}
	toggleCodeGroup(c, values, items, label)
}

func (a *app) planField(c *ui.Context, label, desc string, control func()) {
	a.planFieldSeq++
	p := uiPaletteFor(c)
	formFieldItem(c, p, label, desc, false, control)
}

func (a *app) planBasicsSection(c *ui.Context, p uiPalette) {
	a.planSection(c, p, locales.PlanSectionBasics(), locales.PlanSectionBasicsHint(), func() {
		a.planField(c, locales.PlanNameLabel(), "", func() {
			textField(c, &a.planConfig.Name, locales.PlanNameLabel(), "")
		})
		a.planField(c, locales.PlanDescriptionLabel(), "", func() {
			textAreaField(c, &a.planConfig.Description, locales.PlanDescriptionLabel(), "", 2, 4)
		})
	})
}

func (a *app) planScopeSection(c *ui.Context, p uiPalette) {
	a.planSection(c, p, locales.PlanSectionScope(), "", func() {
		a.planField(c, locales.PlanGradesLabel(), "", func() {
			a.toggleCodes(c, &a.planConfig.AppliesTo.Grades, planGradeChoices(), locales.PlanGradesLabel())
		})
		a.planField(c, locales.PlanExamTypesLabel(), "", func() {
			a.toggleCodes(c, &a.planConfig.AppliesTo.ExamTypes, planExamTypeChoices(), locales.PlanExamTypesLabel())
		})
	})
}

func (a *app) planDataSection(c *ui.Context, p uiPalette) {
	t := p.theme
	a.planSection(c, p, locales.PlanSectionData(), "", func() {
		ui.Grid(c).ColumnTracks(ui.Fr(1).Min(t.Space(60)), ui.Fr(1).Min(t.Space(60))).
			Gap(t.Space(4)).Children(func() {
			a.planField(c, locales.PlanValidStudentLabel(), "", func() {
				a.codeField(c, &a.planConfig.Data.ValidStudent, []planOption{
					{"registered_and_scored", locales.PlanValidRegisteredAndScored()},
					{"registered", locales.PlanValidRegistered()},
					{"scored", locales.PlanValidScored()},
				}, locales.PlanValidStudentLabel())
			})
			a.planField(c, locales.PlanBaselineSourceLabel(), "", func() {
				a.codeField(c, &a.planConfig.Data.BaselineSource, []planOption{
					{"zhongkao", locales.PlanBaselineZhongkao()},
					{"placement", locales.PlanBaselinePlacement()},
					{"previous_exam", locales.PlanBaselinePreviousExam()},
					{"imported", locales.PlanBaselineImported()},
				}, locales.PlanBaselineSourceLabel())
			})
			a.planField(c, locales.PlanGradeAverageLabel(), "", func() {
				a.codeField(c, &a.planConfig.Data.GradeAverage, []planOption{
					{"total_row", locales.PlanGradeAverageTotalRow()},
					{"class_row", locales.PlanGradeAverageClassRow()},
				}, locales.PlanGradeAverageLabel())
			})
		})
		a.planField(c, locales.PlanExcludeFlagsLabel(), "", func() {
			a.toggleCodes(c, &a.planConfig.Data.ExcludeFlags, []planOption{
				{"absent", locales.PlanFlagAbsent()},
				{"violation", locales.PlanFlagViolation()},
				{"cheating", locales.PlanFlagCheating()},
				{"invalid", locales.PlanFlagInvalid()},
				{"not_selected", locales.PlanFlagNotSelected()},
			}, locales.PlanExcludeFlagsLabel())
		})
		a.planField(c, locales.PlanExcludeStatusLabel(), "", func() {
			a.toggleCodes(c, &a.planConfig.Data.ExcludeStatus, []planOption{
				{"transferred", locales.PlanStatusTransferred()},
				{"suspended", locales.PlanStatusSuspended()},
				{"borrowed", locales.PlanStatusBorrowed()},
				{"left_school", locales.PlanStatusLeftSchool()},
			}, locales.PlanExcludeStatusLabel())
		})
	})
}

func (a *app) planWalkingSection(c *ui.Context, p uiPalette) {
	t := p.theme
	a.planSection(c, p, locales.PlanSectionWalking(), "", func() {
		ui.Grid(c).ColumnTracks(ui.Fr(1).Min(t.Space(60)), ui.Fr(1).Min(t.Space(60))).
			Gap(t.Space(4)).Children(func() {
			a.planField(c, locales.PlanWalkingCountInLabel(), "", func() {
				a.codeField(c, &a.planConfig.Data.Walking.CountIn, []planOption{
					{"teaching_group", locales.PlanWalkingTeachingGroup()},
					{"admin_class", locales.PlanWalkingAdminClass()},
				}, locales.PlanWalkingCountInLabel())
			})
			a.planField(c, locales.PlanWalkingDoubleCount(), "", func() {
				switchField(c, &a.planConfig.Data.Walking.AdminClassDoubleCount, locales.PlanWalkingDoubleCount())
			})
			a.planField(c, locales.PlanJapaneseEnglishLabel(), "", func() {
				a.codeField(c, &a.planConfig.Data.Walking.Japanese.EnglishStatistics, []planOption{
					{"exclude", locales.PlanJapaneseExclude()},
					{"include", locales.PlanJapaneseInclude()},
				}, locales.PlanJapaneseEnglishLabel())
			})
			a.planField(c, locales.PlanJapaneseTotalLabel(), "", func() {
				a.codeField(c, &a.planConfig.Data.Walking.Japanese.TotalScore, []planOption{
					{"include", locales.PlanJapaneseInclude()},
					{"exclude", locales.PlanJapaneseExclude()},
				}, locales.PlanJapaneseTotalLabel())
			})
			a.planField(c, locales.PlanJapaneseRankLabel(), "", func() {
				a.codeField(c, &a.planConfig.Data.Walking.Japanese.RankGroup, []planOption{
					{"teaching_group", locales.PlanRankTeachingGroup()},
					{"separate", locales.PlanRankSeparate()},
				}, locales.PlanJapaneseRankLabel())
			})
			a.planField(c, locales.PlanArtSportsRankLabel(), "", func() {
				a.codeField(c, &a.planConfig.Data.Walking.ArtSports.RankGroup, []planOption{
					{"teaching_group", locales.PlanRankTeachingGroup()},
					{"separate", locales.PlanRankSeparate()},
				}, locales.PlanArtSportsRankLabel())
			})
		})
	})
}

func (a *app) planIndicatorsSection(c *ui.Context, p uiPalette) {
	t := p.theme
	a.planSection(c, p, locales.PlanSectionIndicators(), locales.PlanSectionIndicatorsHint(), func() {
		for index := range a.planConfig.Indicators {
			indicator := &a.planConfig.Indicators[index]
			key := indicator.Key
			ui.Column(c).Background(p.panelMuted).Radius(sunbeam.RadiusXL).
				FillWidth().Padding(16).Gap(t.Space(3)).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Text(c, planIndicatorLabel(key)).FontSize(bodySize(t)).FontWeight(600)
				})
				ui.Grid(c).ColumnTracks(ui.Fixed(t.Space(36)), ui.Fr(1).Min(t.Space(60))).
					Gap(t.Space(4)).Children(func() {
					a.planField(c, locales.PlanPointsLabel(), "", func() {
						numberField(c, &indicator.Points, 0, 100, 1, locales.PlanPointsLabel())
					})
					a.planField(c, locales.PlanFormulaLabel(), "", func() {
						a.planFormulaShapeField(c, p, key, &indicator.Formula)
					})
				})
				if indicator.Merge != nil {
					a.planMergeFields(c, p, indicator.Merge)
				}
				a.planCapFields(c, p, indicator)
			})
		}
	})
}

func (a *app) planMergeFields(c *ui.Context, p uiPalette, merge *assessment.MergeConfig) {
	t := p.theme
	ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
		ui.Text(c, locales.PlanMergeTitle()).FontSize(captionSize(t)).FontWeight(600).TextColor(p.muted)
		ui.Grid(c).ColumnTracks(ui.Fr(1).Min(t.Space(50)), ui.Fr(1).Min(t.Space(50))).
			Gap(t.Space(4)).Children(func() {
			a.planField(c, locales.PlanMergeNumerator(), "", func() {
				a.codeField(c, &merge.Numerator, []planOption{
					{"sum_members", locales.PlanMergeSumMembers()},
				}, locales.PlanMergeNumerator())
			})
			a.planField(c, locales.PlanMergeDenominator(), "", func() {
				a.codeField(c, &merge.Denominator, []planOption{
					{"group_target", locales.PlanMergeGroupTarget()},
					{"sum_member_targets", locales.PlanMergeSumMemberTargets()},
					{"lead_class_target", locales.PlanMergeLeadClassTarget()},
				}, locales.PlanMergeDenominator())
			})
			a.planField(c, locales.PlanMergeResultRow(), "", func() {
				a.codeField(c, &merge.ResultRow, []planOption{
					{"lead_class", locales.PlanMergeLeadClass()},
					{"each_member", locales.PlanMergeEachMember()},
					{"new_row", locales.PlanMergeNewRow()},
				}, locales.PlanMergeResultRow())
			})
			a.planField(c, locales.PlanMergeLeadClassField(), "", func() {
				a.codeField(c, &merge.LeadClass, []planOption{
					{"designated", locales.PlanMergeLeadDesignated()},
					{"largest_target", locales.PlanMergeLeadLargest()},
					{"first_by_name", locales.PlanMergeLeadFirst()},
				}, locales.PlanMergeLeadClassField())
			})
		})
	})
}

func (a *app) planCapFields(c *ui.Context, p uiPalette, indicator *assessment.IndicatorConfig) {
	t := p.theme
	on := indicator.Cap != nil
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			switchField(c, &on, locales.PlanCapLabel())
			ui.Text(c, locales.PlanCapLabel()).FontSize(bodySize(t))
		})
		switch {
		case !on && indicator.Cap != nil:
			indicator.Cap = nil
		case on && indicator.Cap == nil:
			indicator.Cap = &assessment.CapConfig{Max: &indicator.Points}
		}
		if indicator.Cap == nil {
			return
		}
		if indicator.Cap.Min == nil {
			lo := float64(0)
			indicator.Cap.Min = &lo
		}
		if indicator.Cap.Max == nil {
			hi := indicator.Points
			indicator.Cap.Max = &hi
		}
		ui.Grid(c).ColumnTracks(ui.Fixed(t.Space(36)), ui.Fixed(t.Space(36))).
			Gap(t.Space(3)).Children(func() {
			a.planField(c, locales.PlanCapMinLabel(), "", func() {
				numberField(c, indicator.Cap.Min, math.Inf(-1), math.Inf(1), 1, locales.PlanCapMinLabel())
			})
			a.planField(c, locales.PlanCapMaxLabel(), "", func() {
				numberField(c, indicator.Cap.Max, math.Inf(-1), math.Inf(1), 1, locales.PlanCapMaxLabel())
			})
		})
	})
}

func (a *app) planTiersSection(c *ui.Context, p uiPalette) {
	t := p.theme
	removed := -1
	a.planSection(c, p, locales.PlanSectionTiers(), locales.PlanSectionTiersHint(), func() {
		for index := range a.planConfig.Tiers {
			index := index
			tier := &a.planConfig.Tiers[index]
			ui.Row(c).FillWidth().AlignItems(ui.End).Gap(t.Space(4)).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					a.planField(c, locales.PlanTierLabel(), "", func() {
						textField(c, &tier.Label, locales.PlanTierLabel(), "")
					})
				})
				ui.Column(c).Width(t.Space(30)).Shrink(0).Children(func() {
					a.planField(c, locales.PlanTierWeightLabel(), "", func() {
						a.planFieldNumber(c, &tier.Weight, locales.PlanTierWeightLabel())
					})
				})
				remove := dangerIconButton(c, iconTrash, locales.PlanRemoveRow())
				if remove.Clicked() {
					removed = index
				}
			})
		}
		if plainButton(c, locales.PlanAddTier()).Clicked() {
			a.planConfig.Tiers = append(a.planConfig.Tiers, assessment.TierConfig{
				Key:    fmt.Sprintf("tier_%d", len(a.planConfig.Tiers)+1),
				Label:  fmt.Sprintf("%s %d", locales.PlanTierLabel(), len(a.planConfig.Tiers)+1),
				Weight: 10,
			})
		}
	})
	if removed >= 0 && len(a.planConfig.Tiers) > 1 {
		a.planConfig.Tiers = append(a.planConfig.Tiers[:removed], a.planConfig.Tiers[removed+1:]...)
	}
}

func (a *app) planRankingSection(c *ui.Context, p uiPalette) {
	t := p.theme
	a.planSection(c, p, locales.PlanSectionRanking(), "", func() {
		ui.Grid(c).ColumnTracks(ui.Fr(1).Min(t.Space(55)), ui.Fr(1).Min(t.Space(55))).
			Gap(t.Space(4)).Children(func() {
			a.planField(c, locales.PlanRankingGroupLabel(), "", func() {
				group := ""
				switch strings.Join(a.planConfig.Ranking.GroupBy, ",") {
				case "grade,subject":
					group = "grade_subject"
				case "grade":
					group = "grade"
				case "school":
					group = "school"
				}
				a.codeSelect(c, &group, []planOption{
					{"grade_subject", locales.PlanRankingGradeSubject()},
					{"grade", locales.PlanRankingGrade()},
					{"school", locales.PlanRankingSchool()},
				}, locales.PlanRankingGroupLabel(), func(value string) {
					switch value {
					case "grade_subject":
						a.planConfig.Ranking.GroupBy = []string{"grade", "subject"}
					case "grade":
						a.planConfig.Ranking.GroupBy = []string{"grade"}
					case "school":
						a.planConfig.Ranking.GroupBy = []string{"school"}
					}
				})
			})
			a.planField(c, locales.PlanTieModeLabel(), "", func() {
				a.codeField(c, &a.planConfig.Ranking.TieMode, []planOption{
					{"competition", locales.PlanTieCompetition()},
					{"dense", locales.PlanTieDense()},
					{"ordinal", locales.PlanTieOrdinal()},
				}, locales.PlanTieModeLabel())
			})
			a.planField(c, locales.PlanBoundaryModeLabel(), "", func() {
				a.codeField(c, &a.planConfig.Ranking.Boundary.Mode, []planOption{
					{"ratio", locales.PlanBoundaryRatio()},
					{"manual", locales.PlanBoundaryManual()},
				}, locales.PlanBoundaryModeLabel())
			})
		})
		if a.planConfig.Ranking.Boundary.Mode == "ratio" {
			a.planBoundaryRatioField(c, p)
			a.planOverrides(c, p)
		} else {
			a.planManualBoundaries(c, p)
		}
		ui.Text(c, locales.PlanTierPointsHint()).FontSize(captionSize(t)).TextColor(p.muted)
		ui.Grid(c).ColumnTracks(ui.Fixed(t.Space(32)), ui.Fixed(t.Space(32)), ui.Fixed(t.Space(32))).
			Gap(t.Space(3)).Children(func() {
			for index := range a.planConfig.Ranking.Points {
				index := index
				a.planField(c, fmt.Sprintf(locales.PlanTierPointsItem(), index+1), "", func() {
					numberField(c, &a.planConfig.Ranking.Points[index], 0, math.Inf(1), 1,
						fmt.Sprintf(locales.PlanTierPointsItem(), index+1))
				})
			}
		})
	})
}

// planBoundaryRatioField 是比例模式分档边界的选项表单：
// 输入均等份数生成一档与二档边界表达式；
// 自定义表达式只读回显，改动份数后覆盖。
func (a *app) planBoundaryRatioField(c *ui.Context, p uiPalette) {
	t := p.theme
	boundary := &a.planConfig.Ranking.Boundary
	matched := assessment.BoundaryRatioParts(boundary.First, boundary.Second)
	display := matched
	if display == 0 {
		display = 3
	}
	if matched == 0 {
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			ui.Text(c, locales.PlanBoundaryCustomEcho()).FontSize(captionSize(t)).TextColor(p.muted)
			ui.Row(c).Gap(t.Space(3)).Children(func() {
				ui.Text(c, boundary.First).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
				ui.Text(c, boundary.Second).FontSize(captionSize(t)).TextColor(p.muted).SingleLine()
			})
		})
	}
	a.planField(c, locales.PlanBoundaryPartsLabel(), "", func() {
		original := float64(display)
		value := original
		numberField(c, &value, 2, 10, 1, locales.PlanBoundaryPartsLabel())
		if value != original {
			boundary.First, boundary.Second = assessment.BoundaryRatioFormulas(int(value))
		}
	})
	ui.Text(c, fmt.Sprintf(locales.PlanBoundaryPartsPreview(), display, display, display-1, display)).
		FontSize(captionSize(t)).TextColor(p.muted)
}

func (a *app) planOverrides(c *ui.Context, p uiPalette) {
	t := p.theme
	removed := -1
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Text(c, locales.PlanOverridesLabel()).FontSize(captionSize(t)).FontWeight(600).TextColor(p.muted)
		for index := range a.planConfig.Ranking.Boundary.Overrides {
			index := index
			row := &a.planConfig.Ranking.Boundary.Overrides[index]
			ui.Row(c).FillWidth().AlignItems(ui.End).Gap(t.Space(4)).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					a.planField(c, locales.PlanOverrideSubject(), "", func() {
						a.codeField(c, &row.Subject, planSubjectChoices(), locales.PlanOverrideSubject())
					})
				})
				ui.Column(c).Width(t.Space(24)).Shrink(0).Children(func() {
					a.planField(c, locales.PlanOverrideCount(), "", func() {
						count := float64(row.N)
						a.planFieldNumber(c, &count, locales.PlanOverrideCount())
						row.N = int(count)
					})
				})
				ui.Column(c).Width(t.Space(24)).Shrink(0).Children(func() {
					a.planField(c, locales.PlanOverrideFirst(), "", func() {
						first := float64(row.First)
						a.planFieldNumber(c, &first, locales.PlanOverrideFirst())
						row.First = int(first)
					})
				})
				ui.Column(c).Width(t.Space(24)).Shrink(0).Children(func() {
					a.planField(c, locales.PlanOverrideSecond(), "", func() {
						second := float64(row.Second)
						a.planFieldNumber(c, &second, locales.PlanOverrideSecond())
						row.Second = int(second)
					})
				})
				if dangerIconButton(c, iconTrash, locales.PlanRemoveRow()).Clicked() {
					removed = index
				}
			})
		}
		if plainButton(c, locales.PlanAddOverride()).Clicked() {
			a.planConfig.Ranking.Boundary.Overrides = append(a.planConfig.Ranking.Boundary.Overrides,
				assessment.BoundaryOverride{Subject: "chinese", N: 1, First: 1, Second: 1})
		}
	})
	if removed >= 0 {
		a.planConfig.Ranking.Boundary.Overrides = append(
			a.planConfig.Ranking.Boundary.Overrides[:removed],
			a.planConfig.Ranking.Boundary.Overrides[removed+1:]...)
	}
}

func (a *app) planManualBoundaries(c *ui.Context, p uiPalette) {
	t := p.theme
	removed := -1
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Text(c, locales.PlanManualLabel()).FontSize(captionSize(t)).FontWeight(600).TextColor(p.muted)
		for index := range a.planConfig.Ranking.Boundary.Manual {
			index := index
			row := &a.planConfig.Ranking.Boundary.Manual[index]
			ui.Row(c).FillWidth().AlignItems(ui.End).Gap(t.Space(4)).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					a.planField(c, locales.PlanOverrideSubject(), "", func() {
						a.codeField(c, &row.Subject, planSubjectChoices(), locales.PlanOverrideSubject())
					})
				})
				ui.Column(c).Width(t.Space(24)).Shrink(0).Children(func() {
					a.planField(c, locales.PlanOverrideFirst(), "", func() {
						first := float64(row.First)
						a.planFieldNumber(c, &first, locales.PlanOverrideFirst())
						row.First = int(first)
					})
				})
				ui.Column(c).Width(t.Space(24)).Shrink(0).Children(func() {
					a.planField(c, locales.PlanOverrideSecond(), "", func() {
						second := float64(row.Second)
						a.planFieldNumber(c, &second, locales.PlanOverrideSecond())
						row.Second = int(second)
					})
				})
				if dangerIconButton(c, iconTrash, locales.PlanRemoveRow()).Clicked() {
					removed = index
				}
			})
		}
		if plainButton(c, locales.PlanAddManual()).Clicked() {
			a.planConfig.Ranking.Boundary.Manual = append(a.planConfig.Ranking.Boundary.Manual,
				assessment.ManualBoundary{Subject: "chinese", First: 1, Second: 1})
		}
	})
	if removed >= 0 {
		a.planConfig.Ranking.Boundary.Manual = append(
			a.planConfig.Ranking.Boundary.Manual[:removed],
			a.planConfig.Ranking.Boundary.Manual[removed+1:]...)
	}
}

func (a *app) planFieldNumber(c *ui.Context, value *float64, label string) {
	numberField(c, value, 0, math.Inf(1), 1, label)
}

func (a *app) planPersonalSection(c *ui.Context, p uiPalette) {
	t := p.theme
	a.planSection(c, p, locales.PlanSectionPersonal(), "", func() {
		ui.Grid(c).ColumnTracks(ui.Fr(1).Min(t.Space(60)), ui.Fr(1).Min(t.Space(60))).
			Gap(t.Space(4)).Children(func() {
			a.planField(c, locales.PlanPersonalModeLabel(), "", func() {
				a.codeField(c, &a.planConfig.Personal.Mode, []planOption{
					{"top_two_average", locales.PlanPersonalTopTwo()},
					{"weighted_by_count", locales.PlanPersonalWeighted()},
					{"average_all", locales.PlanPersonalAverageAll()},
					{"top_one", locales.PlanPersonalTopOne()},
					{"lowest_one", locales.PlanPersonalLowestOne()},
				}, locales.PlanPersonalModeLabel())
			})
			a.planField(c, locales.PlanPersonalScopeLabel(), "", func() {
				a.codeField(c, &a.planConfig.Personal.Scope, []planOption{
					{"per_grade", locales.PlanPersonalScopePerGrade()},
				}, locales.PlanPersonalScopeLabel())
			})
		})
	})
}

func (a *app) planProgramsSection(c *ui.Context, p uiPalette) {
	t := p.theme
	removed := -1
	a.planSection(c, p, locales.PlanSectionPrograms(), locales.PlanSectionProgramsHint(), func() {
		for index := range a.planConfig.Programs {
			index := index
			program := &a.planConfig.Programs[index]
			ui.Column(c).Background(p.panelMuted).Radius(sunbeam.RadiusXL).
				FillWidth().Padding(16).Gap(t.Space(3)).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					ui.Textf(c, "%s %d", locales.PlanProgramTitle(), index+1).
						FontSize(bodySize(t)).FontWeight(600)
					ui.Spacer(c)
					if dangerIconButton(c, iconTrash, locales.PlanRemoveRow()).Clicked() {
						removed = index
					}
				})
				ui.Grid(c).ColumnTracks(ui.Fr(1).Min(t.Space(45)), ui.Fr(1).Min(t.Space(45)), ui.Fr(1).Min(t.Space(45))).
					Gap(t.Space(3)).Children(func() {
					a.planProgramExamType(c, p, program)
					a.planProgramGrade(c, p, program)
					a.planField(c, locales.PlanProgramCohort(), "", func() {
						cohort := program.When.Cohort
						textField(c, &cohort, locales.PlanProgramCohort(), locales.PlanProgramAny())
						program.When.Cohort = cohort
					})
				})
				ui.Text(c, locales.PlanProgramOverrides()).FontSize(captionSize(t)).
					FontWeight(600).TextColor(p.muted)
				for _, key := range []string{"improvement", "pass", "completion"} {
					key := key
					a.planField(c, planIndicatorLabel(key), "", func() {
						if key == "completion" {
							a.planCompletionOverrideField(c, p, program)
							return
						}
						a.planProgramOverrideField(c, program, key)
					})
				}
			})
		}
		if plainButton(c, locales.PlanAddProgram()).Clicked() {
			a.planConfig.Programs = append(a.planConfig.Programs, assessment.ProgramConfig{
				When:               assessment.ProgramWhen{ExamType: "gaokao"},
				IndicatorOverrides: map[string]assessment.IndicatorOverride{},
			})
		}
	})
	if removed >= 0 {
		a.planConfig.Programs = append(a.planConfig.Programs[:removed], a.planConfig.Programs[removed+1:]...)
	}
}

func (a *app) planProgramExamType(c *ui.Context, p uiPalette, program *assessment.ProgramConfig) {
	options := append([]planOption{{"", locales.PlanProgramAny()}}, planExamTypeChoices()...)
	a.planField(c, locales.PlanProgramExamType(), "", func() {
		a.codeField(c, &program.When.ExamType, options, locales.PlanProgramExamType())
	})
}

// planProgramOverrideField 是提升率与达线率覆盖表达式的原始输入。
func (a *app) planProgramOverrideField(c *ui.Context, program *assessment.ProgramConfig, key string) {
	formula := ""
	if program.IndicatorOverrides != nil {
		formula = program.IndicatorOverrides[key].Formula
	}
	textField(c, &formula, planIndicatorLabel(key), "")
	a.setProgramOverride(program, key, formula)
}

// setProgramOverride 把覆盖表达式写入专项规则；空表达式移除该指标的覆盖。
func (a *app) setProgramOverride(program *assessment.ProgramConfig, key, formula string) {
	formula = strings.TrimSpace(formula)
	switch {
	case formula == "" && program.IndicatorOverrides != nil:
		delete(program.IndicatorOverrides, key)
	case formula != "":
		if program.IndicatorOverrides == nil {
			program.IndicatorOverrides = map[string]assessment.IndicatorOverride{}
		}
		program.IndicatorOverrides[key] = assessment.IndicatorOverride{Formula: formula}
	}
}

// planCompletionOverrideField 是完成率覆盖表达式的选项表单：
// 目标完成率与本科达线率按权重合成；
// 自定义表达式只读回显，重选形状后覆盖。
func (a *app) planCompletionOverrideField(c *ui.Context, p uiPalette, program *assessment.ProgramConfig) {
	t := p.theme
	formula := ""
	if program.IndicatorOverrides != nil {
		formula = program.IndicatorOverrides["completion"].Formula
	}
	weights, weighted := assessment.ParseCompletionWeights(formula)
	code := "none"
	switch {
	case formula != "" && weighted:
		code = "weighted"
	case formula != "":
		code = planFormulaShapeOther
	}
	options := []planOption{
		{code: "none", label: locales.PlanProgramOverrideNone()},
		{code: "weighted", label: locales.PlanCompletionWeightedLabel()},
	}
	if code == planFormulaShapeOther {
		options = append(options, planOption{code: planFormulaShapeOther, label: locales.PlanFormulaCustom()})
	}
	ui.Column(c).FillWidth().Gap(t.Space(1.5)).Children(func() {
		a.codeSelect(c, &code, options, planIndicatorLabel("completion"), func(value string) {
			switch value {
			case "none":
				a.setProgramOverride(program, "completion", "")
			case "weighted":
				a.setProgramOverride(program, "completion", assessment.CompletionWeightsFormula(0.6, 0.4))
			}
		})
		if code == planFormulaShapeOther {
			ui.Text(c, formula).FontSize(captionSize(t)).TextColor(p.muted).SingleLine().Tooltip(formula)
			return
		}
		if code != "weighted" {
			return
		}
		originalTarget := weights.TargetCompletion
		originalUndergraduate := weights.Undergraduate
		targetWeight := originalTarget
		undergraduateWeight := originalUndergraduate
		ui.Grid(c).ColumnTracks(ui.Fr(1).Min(t.Space(28)), ui.Fr(1).Min(t.Space(28))).
			Gap(t.Space(3)).Children(func() {
			a.planWeightInput(c, p, locales.PlanCompletionWeightCompletion(), &targetWeight)
			a.planWeightInput(c, p, locales.PlanCompletionWeightUndergraduate(), &undergraduateWeight)
		})
		if targetWeight != originalTarget || undergraduateWeight != originalUndergraduate {
			a.setProgramOverride(program, "completion",
				assessment.CompletionWeightsFormula(targetWeight, undergraduateWeight))
		}
		ui.Text(c, fmt.Sprintf(locales.PlanCompletionWeightedPreview(),
			assessment.FormatWeight(targetWeight), assessment.FormatWeight(undergraduateWeight))).
			FontSize(captionSize(t)).TextColor(p.muted)
	})
}

// planWeightInput 是 0 到 1 的权重输入，标签在控件上方。
func (a *app) planWeightInput(c *ui.Context, p uiPalette, label string, value *float64) {
	t := p.theme
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		ui.Text(c, label).FontSize(captionSize(t)).TextColor(p.muted)
		numberField(c, value, 0, 1, 0.05, label)
	})
}

func (a *app) planProgramGrade(c *ui.Context, p uiPalette, program *assessment.ProgramConfig) {
	options := append([]planOption{{"", locales.PlanProgramAny()}}, planGradeChoices()...)
	a.planField(c, locales.PlanProgramGrade(), "", func() {
		a.codeField(c, &program.When.Grade, options, locales.PlanProgramGrade())
	})
}

func (a *app) planValidationSection(c *ui.Context, p uiPalette) {
	t := p.theme
	a.planSection(c, p, locales.PlanSectionValidation(), "", func() {
		a.planField(c, locales.PlanValidationBlockingLabel(), "", func() {
			a.toggleCodes(c, &a.planConfig.Validation.Blocking, []planOption{
				{"missing_required_column", locales.PlanIssueMissingColumn()},
				{"duplicate_student", locales.PlanIssueDuplicateStudent()},
				{"zero_target_base", locales.PlanIssueZeroTarget()},
			}, locales.PlanValidationBlockingLabel())
		})
		a.planField(c, locales.PlanValidationWarningLabel(), "", func() {
			a.toggleCodes(c, &a.planConfig.Validation.Warning, []planOption{
				{"count_mismatch", locales.PlanIssueCountMismatch()},
				{"missing_score_without_flag", locales.PlanIssueMissingScore()},
			}, locales.PlanValidationWarningLabel())
		})
		a.planField(c, locales.PlanZeroTargetActionLabel(), "", func() {
			a.codeField(c, &a.planConfig.Validation.ZeroTarget.Action, []planOption{
				{"pause_and_prompt", locales.PlanZeroPausePrompt()},
				{"pause_and_exclude", locales.PlanZeroPauseExclude()},
			}, locales.PlanZeroTargetActionLabel())
		})
		ui.Grid(c).ColumnTracks(ui.Fr(1).Min(t.Space(55)), ui.Fr(1).Min(t.Space(55))).
			Gap(t.Space(2)).Children(func() {
			a.planSwitch(c, &a.planConfig.Validation.ZeroTarget.AllowNotParticipating, locales.PlanZeroAllowSkip())
			a.planSwitch(c, &a.planConfig.Validation.ZeroTarget.RecomputeAfterFill, locales.PlanZeroRecompute())
			a.planSwitch(c, &a.planConfig.Validation.ZeroTarget.Audit, locales.PlanZeroAudit())
			a.planSwitch(c, &a.planConfig.Validation.ZeroTarget.RecheckBeforeExport, locales.PlanZeroRecheck())
		})
	})
}

func (a *app) planSwitch(c *ui.Context, value *bool, label string) {
	t := *c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		switchField(c, value, label)
		ui.Text(c, label).FontSize(bodySize(t))
	})
}

func (a *app) planSaveBar(c *ui.Context, p uiPalette) {
	t := p.theme
	ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.End).Gap(t.Space(2)).
		Padding(t.Space(2), 0, 0, 0).Children(func() {
		ui.Text(c, locales.PlanSaveHint()).FontSize(captionSize(t)).TextColor(p.muted).
			Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if pageActionButton(c, locales.PlanSave()).Clicked() {
			a.planError = ""
			a.planMessage = ""
			a.planSaveChoice = planSaveAsking
		}
	})
}

func (a *app) planSaveDialog(c *ui.Context, p uiPalette) {
	t := p.theme
	chosen := planSaveNone
	open := a.planSaveChoice == planSaveAsking
	modal(c, &open, modalProps{
		Key:      "plan-save",
		Title:    locales.PlanSaveTitle(),
		NoFooter: true,
		Width:    modalWidthCompact,
	}, func() {
		ui.Column(c).FillWidth().Gap(t.Space(3)).Children(func() {
			ui.Text(c, locales.PlanSaveMessage()).FontSize(bodySize(t)).TextColor(p.inkSecondary).
				Pretty()
			ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
				if primaryButton(c, locales.PlanUpdate()).Clicked() {
					chosen = planSaveUpdate
					open = false
				}
				if plainButton(c, locales.PlanSaveAsNew()).Clicked() {
					chosen = planSaveNew
					open = false
				}
				if textButton(c, locales.PlanCancel()).Clicked() {
					chosen = planSaveCancel
					open = false
				}
			})
		})
	})
	if chosen == planSaveNone {
		if !open && a.planSaveChoice == planSaveAsking {
			a.planSaveChoice = planSaveNone
		}
		return
	}
	a.planSaveChoice = planSaveNone
	switch chosen {
	case planSaveNew:
		a.savePlan(false)
	case planSaveUpdate:
		a.savePlan(true)
	}
	if a.planError == "" && a.planMessage != "" {
		c.Toast(a.planMessage)
		a.planMessage = ""
	}
}

// savePlan 保存可视化配置。update 为真时更新当前方案，否则保存成新的方案。
// 两种保存都会发布结果，核算页可以直接选用。
func (a *app) savePlan(update bool) {
	a.planConfig.Name = strings.TrimSpace(a.planConfig.Name)
	if a.planConfig.Name == "" {
		a.planError = locales.PlanNameRequired()
		return
	}
	if msg := validateRuneLength(a.planConfig.Name, locales.PlanNameLabel(), maxNameRunes); msg != "" {
		a.planError = msg
		return
	}
	if msg := validateRuneLength(a.planConfig.Description, locales.PlanDescriptionLabel(), maxDescRunes); msg != "" {
		a.planError = msg
		return
	}
	ctx := context.Background()
	current := a.selectedPlan()
	var version store.PlanVersion
	if update {
		if current == nil {
			a.planError = locales.PlanSelectHint()
			return
		}
		saved, err := a.store.UpdatePlan(ctx, current.ID, a.planConfig, "", "local")
		if err != nil {
			a.planError = err.Error()
			return
		}
		version = saved
	} else {
		name := a.planConfig.Name
		for _, plan := range a.plans {
			if plan.Name == name {
				a.planError = locales.PlanNameTaken()
				return
			}
		}
		config := a.planConfig
		config.Name = name
		plan, saved, err := a.store.CreatePlan(ctx, store.CreatePlanInput{
			Name:          name,
			Kind:          "user",
			Description:   config.Description,
			BaseVersionID: a.planConfigVersionID,
			Config:        config,
			CreatedBy:     "local",
		})
		if err != nil {
			a.planError = err.Error()
			return
		}
		version = saved
		a.reloadPlans(ctx)
		a.selectedPlanID = strconv.FormatInt(plan.ID, 10)
		a.versionIndex = -1
		a.reloadVersions(ctx)
	}
	if version.Status == "draft" {
		if err := a.store.Publish(ctx, version.ID, "local"); err != nil {
			a.planError = err.Error()
			return
		}
	}
	a.planError = ""
	a.reloadPlans(ctx)
	a.versionIndex = -1
	a.reloadVersions(ctx)
	if update {
		a.planMessage = locales.PlanSavedToast()
	} else {
		a.planMessage = fmt.Sprintf(locales.PlanSavedNewToast(), a.selectedPlanName())
	}
	a.loadPlanConfig()
}

func (a *app) selectedPlanName() string {
	if plan := a.selectedPlan(); plan != nil {
		return plan.Name
	}
	return a.planConfig.Name
}
