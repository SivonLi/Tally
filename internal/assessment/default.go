package assessment

// DefaultConfig 返回系统内置的默认考核方案。
func DefaultConfig() Config {
	return Config{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "教学成绩量化考核方案",
		Description:   "学校内置默认方案，覆盖全部年级和考试类型",
		AppliesTo: AppliesTo{
			Grades:    []string{"g1", "g2", "g3"},
			ExamTypes: []string{"midterm", "final", "gaokao"},
		},
		Data: DataConfig{
			ValidStudent:   "registered_and_scored",
			ExcludeFlags:   []string{"absent", "violation", "cheating", "invalid"},
			ExcludeStatus:  []string{"transferred", "suspended", "borrowed"},
			BaselineSource: "zhongkao",
			GradeAverage:   "total_row",
			Walking: WalkingConfig{
				CountIn:               "teaching_group",
				AdminClassDoubleCount: false,
				Japanese: JapaneseConfig{
					EnglishStatistics: "exclude",
					TotalScore:        "include",
					RankGroup:         "teaching_group",
				},
				ArtSports: ArtSportsConfig{
					RankGroup: "teaching_group",
				},
			},
		},
		Indicators: []IndicatorConfig{
			{
				Key:     "improvement",
				Points:  40,
				Cap:     nil,
				Formula: "class.average[subject] / grade.average[subject] / baseline.ratio[subject]",
			},
			{
				Key:     "pass",
				Points:  40,
				Cap:     nil,
				Formula: "statGroup.passCount / statGroup.target",
				Merge: &MergeConfig{
					Numerator:   "sum_members",
					Denominator: "group_target",
					ResultRow:   "lead_class",
					LeadClass:   "designated",
				},
			},
			{
				Key:     "completion",
				Points:  20,
				Cap:     nil,
				Formula: "tier.actual / tier.base",
			},
		},
		Tiers: []TierConfig{
			{Key: "qingbei", Label: "清北", Weight: 50},
			{Key: "c9", Label: "C9", Weight: 40},
			{Key: "project985", Label: "985", Weight: 30},
			{Key: "project211", Label: "211", Weight: 20},
			{Key: "special_control", Label: "特控", Weight: 10},
			{Key: "undergraduate", Label: "本科", Weight: 8},
		},
		Ranking: RankingConfig{
			GroupBy: []string{"grade", "subject"},
			TieMode: "competition",
			Boundary: BoundaryConfig{
				Mode:   "ratio",
				First:  "floor(n / 3)",
				Second: "round(2 * n / 3)",
				Overrides: []BoundaryOverride{
					{Subject: "chinese", N: 17, First: 6, Second: 11},
				},
			},
			Points: []float64{70, 67, 64},
		},
		Personal: PersonalConfig{
			Mode:  "top_two_average",
			Scope: "per_grade",
		},
		Programs: []ProgramConfig{
			{
				When: ProgramWhen{ExamType: "gaokao"},
				IndicatorOverrides: map[string]IndicatorOverride{
					"completion": {
						Formula: "program.targetCompletion * 0.6 + program.undergraduatePass * 0.4",
					},
				},
			},
		},
		Validation: ValidationConfig{
			Blocking: []string{
				"missing_required_column",
				"duplicate_student",
				"zero_target_base",
			},
			Warning: []string{
				"count_mismatch",
				"missing_score_without_flag",
			},
			ZeroTarget: ZeroTargetConfig{
				Action:                "pause_and_prompt",
				AllowNotParticipating: true,
				RecomputeAfterFill:    true,
				Audit:                 true,
				RecheckBeforeExport:   true,
			},
		},
	}
}
