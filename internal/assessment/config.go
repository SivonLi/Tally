// Package assessment 管理教学成绩考核方案的配置、校验与默认值。
package assessment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

// CurrentSchemaVersion 是当前方案配置结构的版本。
const CurrentSchemaVersion = 1

// Config 是一项考核方案的完整配置。
type Config struct {
	SchemaVersion int               `json:"schemaVersion"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	AppliesTo     AppliesTo         `json:"appliesTo"`
	Data          DataConfig        `json:"data"`
	Indicators    []IndicatorConfig `json:"indicators"`
	Tiers         []TierConfig      `json:"tiers"`
	Ranking       RankingConfig     `json:"ranking"`
	Personal      PersonalConfig    `json:"personal"`
	Programs      []ProgramConfig   `json:"programs"`
	Validation    ValidationConfig  `json:"validation"`
}

// AppliesTo 声明方案适用的年级与考试类型。
type AppliesTo struct {
	Grades    []string `json:"grades"`
	ExamTypes []string `json:"examTypes"`
}

// DataConfig 定义有效考生、剔除范围、基数来源和走班统计口径。
type DataConfig struct {
	ValidStudent   string        `json:"validStudent"`
	ExcludeFlags   []string      `json:"excludeFlags"`
	ExcludeStatus  []string      `json:"excludeStatus"`
	BaselineSource string        `json:"baselineSource"`
	GradeAverage   string        `json:"gradeAverage"`
	Walking        WalkingConfig `json:"walking"`
}

// WalkingConfig 定义走班学生的统计方式。
type WalkingConfig struct {
	CountIn               string          `json:"countIn"`
	AdminClassDoubleCount bool            `json:"adminClassDoubleCount"`
	Japanese              JapaneseConfig  `json:"japanese"`
	ArtSports             ArtSportsConfig `json:"artSports"`
}

// JapaneseConfig 定义日语考生的英语统计与排名组处理。
type JapaneseConfig struct {
	EnglishStatistics string `json:"englishStatistics"`
	TotalScore        string `json:"totalScore"`
	RankGroup         string `json:"rankGroup"`
}

// ArtSportsConfig 定义艺体考生的排名组处理。
type ArtSportsConfig struct {
	RankGroup string `json:"rankGroup"`
}

// IndicatorConfig 定义一项指标的满分、公式、封顶和合并组算法。
type IndicatorConfig struct {
	Key     string       `json:"key"`
	Points  float64      `json:"points"`
	Cap     *CapConfig   `json:"cap"`
	Formula string       `json:"formula"`
	Merge   *MergeConfig `json:"merge,omitempty"`
}

// CapConfig 定义指标得分的上下限。
type CapConfig struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

// MergeConfig 定义合并统计组的计算方式。
type MergeConfig struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
	ResultRow   string `json:"resultRow"`
	LeadClass   string `json:"leadClass"`
}

// TierConfig 定义完成率档位的代码、显示名称和权重。
type TierConfig struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
}

// RankingConfig 定义排名组、并列规则、分档边界和赋分。
type RankingConfig struct {
	GroupBy  []string       `json:"groupBy"`
	TieMode  string         `json:"tieMode"`
	Boundary BoundaryConfig `json:"boundary"`
	Points   []float64      `json:"points"`
}

// BoundaryConfig 定义分档边界的生成方式与例外。
type BoundaryConfig struct {
	Mode      string             `json:"mode"`
	First     string             `json:"first"`
	Second    string             `json:"second"`
	Overrides []BoundaryOverride `json:"overrides"`
	Manual    []ManualBoundary   `json:"manual,omitempty"`
}

// BoundaryOverride 定义某个学科和参评数量下的分档边界。
type BoundaryOverride struct {
	Subject string `json:"subject"`
	N       int    `json:"n"`
	First   int    `json:"first"`
	Second  int    `json:"second"`
}

// ManualBoundary 定义人工模式下一组学科名次区间。
type ManualBoundary struct {
	Subject string `json:"subject"`
	N       int    `json:"n,omitempty"`
	First   int    `json:"first"`
	Second  int    `json:"second"`
}

// PersonalConfig 定义教师个人得分的汇总方式。
type PersonalConfig struct {
	Mode  string `json:"mode"`
	Scope string `json:"scope"`
}

// ProgramConfig 定义按条件覆盖指标参数的专项规则。
type ProgramConfig struct {
	When               ProgramWhen                  `json:"when"`
	IndicatorOverrides map[string]IndicatorOverride `json:"indicatorOverrides"`
}

// ProgramWhen 定义专项规则的匹配条件。
type ProgramWhen struct {
	ExamType string `json:"examType,omitempty"`
	Grade    string `json:"grade,omitempty"`
	Cohort   string `json:"cohort,omitempty"`
}

// IndicatorOverride 定义专项规则对某项指标的覆盖。
type IndicatorOverride struct {
	Formula string `json:"formula"`
}

// ValidationConfig 定义阻断项、提醒项和零目标处理。
type ValidationConfig struct {
	Blocking   []string         `json:"blocking"`
	Warning    []string         `json:"warning"`
	ZeroTarget ZeroTargetConfig `json:"zeroTarget"`
}

// ZeroTargetConfig 定义目标基础分为零时的处理流程。
type ZeroTargetConfig struct {
	Action                string `json:"action"`
	AllowNotParticipating bool   `json:"allowNotParticipating"`
	RecomputeAfterFill    bool   `json:"recomputeAfterFill"`
	Audit                 bool   `json:"audit"`
	RecheckBeforeExport   bool   `json:"recheckBeforeExport"`
}

// Issue 描述一个配置校验问题。
type Issue struct {
	Field   string
	Message string
}

// ValidationError 汇总一次配置校验发现的全部问题。
type ValidationError struct {
	Issues []Issue
}

// Error 返回可直接显示的校验错误。
func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, issue.Field+": "+issue.Message)
	}
	return "方案配置校验失败: " + strings.Join(parts, "; ")
}

// Decode 严格解析并校验一个方案配置，未知字段会被拒绝。
func Decode(data []byte) (Config, error) {
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("解析方案配置: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("解析方案配置: 存在多余内容")
		}
		return Config{}, fmt.Errorf("解析方案配置: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate 检查方案配置的结构和语义。
func (c Config) Validate() error {
	var issues []Issue
	add := func(field, message string) {
		issues = append(issues, Issue{Field: field, Message: message})
	}

	if c.SchemaVersion != CurrentSchemaVersion {
		add("schemaVersion", fmt.Sprintf("必须为 %d", CurrentSchemaVersion))
	}
	if strings.TrimSpace(c.Name) == "" {
		add("name", "不能为空")
	}

	validateAppliesTo(c.AppliesTo, add)
	validateData(c.Data, add)
	validateIndicators(c.Indicators, add)
	validateTiers(c.Tiers, add)
	validateRanking(c.Ranking, add)
	validatePersonal(c.Personal, add)
	validatePrograms(c.Programs, add)
	validateValidation(c.Validation, add)

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func validateAppliesTo(a AppliesTo, add func(string, string)) {
	validGrades := stringSet("g1", "g2", "g3")
	validExamTypes := stringSet("midterm", "final", "gaokao")
	validateEnumList("appliesTo.grades", a.Grades, validGrades, true, add)
	validateEnumList("appliesTo.examTypes", a.ExamTypes, validExamTypes, true, add)
}

func validateData(d DataConfig, add func(string, string)) {
	validateEnum("data.validStudent", d.ValidStudent,
		stringSet("registered_and_scored", "registered", "scored"), add)
	validateEnumList("data.excludeFlags", d.ExcludeFlags,
		stringSet("absent", "violation", "cheating", "invalid", "not_selected"), false, add)
	validateEnumList("data.excludeStatus", d.ExcludeStatus,
		stringSet("transferred", "suspended", "borrowed", "left_school"), false, add)
	validateEnum("data.baselineSource", d.BaselineSource,
		stringSet("zhongkao", "placement", "previous_exam", "imported"), add)
	validateEnum("data.gradeAverage", d.GradeAverage,
		stringSet("total_row", "class_row"), add)
	validateEnum("data.walking.countIn", d.Walking.CountIn,
		stringSet("teaching_group", "admin_class"), add)
	validateEnum("data.walking.japanese.englishStatistics", d.Walking.Japanese.EnglishStatistics,
		stringSet("exclude", "include"), add)
	validateEnum("data.walking.japanese.totalScore", d.Walking.Japanese.TotalScore,
		stringSet("include", "exclude"), add)
	validateEnum("data.walking.japanese.rankGroup", d.Walking.Japanese.RankGroup,
		stringSet("teaching_group", "separate"), add)
	validateEnum("data.walking.artSports.rankGroup", d.Walking.ArtSports.RankGroup,
		stringSet("teaching_group", "separate"), add)
}

func validateIndicators(indicators []IndicatorConfig, add func(string, string)) {
	required := map[string]bool{
		"improvement": false,
		"pass":        false,
		"completion":  false,
	}
	total := 0.0
	for i, indicator := range indicators {
		field := fmt.Sprintf("indicators[%d]", i)
		if _, ok := required[indicator.Key]; !ok {
			add(field+".key", "必须是 improvement、pass 或 completion")
		} else if required[indicator.Key] {
			add(field+".key", "不能重复")
		} else {
			required[indicator.Key] = true
		}
		if !finitePositive(indicator.Points) {
			add(field+".points", "必须大于零")
		}
		total += indicator.Points
		if strings.TrimSpace(indicator.Formula) == "" {
			add(field+".formula", "不能为空")
		}
		validateCap(field+".cap", indicator.Cap, add)
		if indicator.Key == "pass" {
			if indicator.Merge == nil {
				add(field+".merge", "pass 指标必须配置合并组算法")
			} else {
				validateMerge(field+".merge", *indicator.Merge, add)
			}
		} else if indicator.Merge != nil {
			add(field+".merge", "只有 pass 指标可以配置合并组算法")
		}
	}
	for key, found := range required {
		if !found {
			add("indicators", "缺少指标 "+key)
		}
	}
	if math.Abs(total-100) > 1e-9 {
		add("indicators", fmt.Sprintf("指标分值之和必须为 100，当前为 %g", total))
	}
}

func validateCap(field string, cap *CapConfig, add func(string, string)) {
	if cap == nil {
		return
	}
	if cap.Min == nil && cap.Max == nil {
		add(field, "至少设置一个上下限")
		return
	}
	if cap.Min != nil && !finite(*cap.Min) {
		add(field+".min", "必须是有限数值")
	}
	if cap.Max != nil && !finite(*cap.Max) {
		add(field+".max", "必须是有限数值")
	}
	if cap.Min != nil && cap.Max != nil && *cap.Min > *cap.Max {
		add(field, "下限不能大于上限")
	}
}

func validateMerge(field string, merge MergeConfig, add func(string, string)) {
	validateEnum(field+".numerator", merge.Numerator, stringSet("sum_members"), add)
	validateEnum(field+".denominator", merge.Denominator,
		stringSet("sum_member_targets", "lead_class_target", "group_target"), add)
	validateEnum(field+".resultRow", merge.ResultRow,
		stringSet("lead_class", "each_member", "new_row"), add)
	validateEnum(field+".leadClass", merge.LeadClass,
		stringSet("designated", "largest_target", "first_by_name"), add)
}

func validateTiers(tiers []TierConfig, add func(string, string)) {
	if len(tiers) == 0 {
		add("tiers", "至少需要一个档位")
	}
	seen := map[string]bool{}
	for i, tier := range tiers {
		field := fmt.Sprintf("tiers[%d]", i)
		if strings.TrimSpace(tier.Key) == "" {
			add(field+".key", "不能为空")
		} else if seen[tier.Key] {
			add(field+".key", "不能重复")
		} else {
			seen[tier.Key] = true
		}
		if strings.TrimSpace(tier.Label) == "" {
			add(field+".label", "不能为空")
		}
		if !finitePositive(tier.Weight) {
			add(field+".weight", "必须大于零")
		}
	}
}

func validateRanking(ranking RankingConfig, add func(string, string)) {
	groupBy := strings.Join(ranking.GroupBy, ",")
	if groupBy != "grade,subject" && groupBy != "grade" && groupBy != "school" {
		add("ranking.groupBy", "必须是 [grade,subject]、[grade] 或 [school]")
	}
	validateEnum("ranking.tieMode", ranking.TieMode,
		stringSet("competition", "dense", "ordinal"), add)
	validateEnum("ranking.boundary.mode", ranking.Boundary.Mode,
		stringSet("ratio", "manual"), add)
	if ranking.Boundary.Mode == "ratio" {
		if strings.TrimSpace(ranking.Boundary.First) == "" {
			add("ranking.boundary.first", "不能为空")
		}
		if strings.TrimSpace(ranking.Boundary.Second) == "" {
			add("ranking.boundary.second", "不能为空")
		}
	}
	seenOverrides := map[string]bool{}
	for i, override := range ranking.Boundary.Overrides {
		field := fmt.Sprintf("ranking.boundary.overrides[%d]", i)
		if strings.TrimSpace(override.Subject) == "" {
			add(field+".subject", "不能为空")
		}
		key := fmt.Sprintf("%s:%d", override.Subject, override.N)
		if seenOverrides[key] {
			add(field, "同一学科和参评数量不能重复")
		}
		seenOverrides[key] = true
		if override.N <= 0 {
			add(field+".n", "必须大于零")
		}
		if override.First < 1 || override.First > override.Second || override.Second > override.N {
			add(field, "必须满足 1 <= first <= second <= n")
		}
	}
	if ranking.Boundary.Mode == "manual" {
		if len(ranking.Boundary.Manual) == 0 {
			add("ranking.boundary.manual", "人工模式至少需要一条名次区间")
		}
		seenManual := map[string]bool{}
		for i, boundary := range ranking.Boundary.Manual {
			field := fmt.Sprintf("ranking.boundary.manual[%d]", i)
			if strings.TrimSpace(boundary.Subject) == "" {
				add(field+".subject", "不能为空")
			}
			key := fmt.Sprintf("%s:%d", boundary.Subject, boundary.N)
			if seenManual[key] {
				add(field, "同一学科和参评数量不能重复")
			}
			seenManual[key] = true
			if boundary.N < 0 {
				add(field+".n", "不能小于零")
			}
			if boundary.First < 1 || boundary.First > boundary.Second {
				add(field, "必须满足 1 <= first <= second")
			}
			if boundary.N > 0 && boundary.Second > boundary.N {
				add(field+".second", "不能大于参评数量")
			}
		}
	}
	if len(ranking.Points) != 3 {
		add("ranking.points", "必须包含三档赋分")
	}
	for i, points := range ranking.Points {
		if !finitePositive(points) {
			add(fmt.Sprintf("ranking.points[%d]", i), "必须大于零")
		}
	}
}

func validatePersonal(personal PersonalConfig, add func(string, string)) {
	validateEnum("personal.mode", personal.Mode,
		stringSet("top_two_average", "weighted_by_count", "average_all", "top_one", "lowest_one"), add)
	validateEnum("personal.scope", personal.Scope, stringSet("per_grade"), add)
}

func validatePrograms(programs []ProgramConfig, add func(string, string)) {
	validIndicators := stringSet("improvement", "pass", "completion")
	for i, program := range programs {
		field := fmt.Sprintf("programs[%d]", i)
		when := program.When
		if when.ExamType == "" && when.Grade == "" && when.Cohort == "" {
			add(field+".when", "至少需要一个匹配条件")
		}
		if when.ExamType != "" {
			validateEnum(field+".when.examType", when.ExamType,
				stringSet("midterm", "final", "gaokao"), add)
		}
		if when.Grade != "" {
			validateEnum(field+".when.grade", when.Grade, stringSet("g1", "g2", "g3"), add)
		}
		if len(program.IndicatorOverrides) == 0 {
			add(field+".indicatorOverrides", "至少需要一项覆盖")
		}
		for key, override := range program.IndicatorOverrides {
			overrideField := field + ".indicatorOverrides." + key
			if !validIndicators[key] {
				add(overrideField, "指标键不存在")
			}
			if strings.TrimSpace(override.Formula) == "" {
				add(overrideField+".formula", "不能为空")
			}
		}
	}
}

func validateValidation(validation ValidationConfig, add func(string, string)) {
	validateEnumList("validation.blocking", validation.Blocking,
		stringSet("missing_required_column", "duplicate_student", "zero_target_base"), false, add)
	validateEnumList("validation.warning", validation.Warning,
		stringSet("count_mismatch", "missing_score_without_flag"), false, add)
	validateEnum("validation.zeroTarget.action", validation.ZeroTarget.Action,
		stringSet("pause_and_prompt", "pause_and_exclude"), add)
}

func validateEnum(field, value string, allowed map[string]bool, add func(string, string)) {
	if !allowed[value] {
		add(field, "取值不在允许范围内")
	}
}

func validateEnumList(field string, values []string, allowed map[string]bool, required bool, add func(string, string)) {
	if required && len(values) == 0 {
		add(field, "至少需要一项")
	}
	seen := map[string]bool{}
	for i, value := range values {
		itemField := fmt.Sprintf("%s[%d]", field, i)
		if !allowed[value] {
			add(itemField, "取值不在允许范围内")
		}
		if seen[value] {
			add(itemField, "不能重复")
		}
		seen[value] = true
	}
}

func stringSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finitePositive(value float64) bool {
	return finite(value) && value > 0
}
