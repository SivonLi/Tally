package assessment_test

import (
	"encoding/json"
	"strings"
	"testing"

	"tally/internal/assessment"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := assessment.DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config is invalid: %v", err)
	}
}

func TestDefaultConfigSurvivesStrictJSONRoundTrip(t *testing.T) {
	data, err := json.Marshal(assessment.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	got, err := assessment.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ranking.TieMode != "competition" {
		t.Fatalf("tieMode = %q", got.Ranking.TieMode)
	}
	if got.Ranking.Boundary.Overrides[0].First != 6 {
		t.Fatalf("override = %+v", got.Ranking.Boundary.Overrides[0])
	}
	if got.Indicators[1].Merge == nil || got.Indicators[1].Merge.Denominator != "group_target" {
		t.Fatalf("pass merge = %+v", got.Indicators[1].Merge)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	cfg := assessment.DefaultConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"schemaVersion":1`, `"schemaVersion":1,"unknown":true`, 1))
	if _, err := assessment.Decode(data); err == nil {
		t.Fatal("unknown field was accepted")
	}
}

func TestValidateRejectsInvalidIndicatorPoints(t *testing.T) {
	cfg := assessment.DefaultConfig()
	cfg.Indicators[0].Points = 39
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "指标分值之和") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRejectsDuplicateIndicatorKey(t *testing.T) {
	cfg := assessment.DefaultConfig()
	cfg.Indicators[1].Key = "improvement"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "不能重复") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRejectsInvalidBoundaryOverride(t *testing.T) {
	cfg := assessment.DefaultConfig()
	cfg.Ranking.Boundary.Overrides[0].Second = 18
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "1 <= first <= second <= n") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRejectsInvalidMergeDenominator(t *testing.T) {
	cfg := assessment.DefaultConfig()
	cfg.Indicators[1].Merge.Denominator = "member_sum"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "denominator") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRejectsEmptyProgramFormula(t *testing.T) {
	cfg := assessment.DefaultConfig()
	cfg.Programs[0].IndicatorOverrides["completion"] = assessment.IndicatorOverride{}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "formula") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRejectsZeroTargetAction(t *testing.T) {
	cfg := assessment.DefaultConfig()
	cfg.Validation.ZeroTarget.Action = "ignore"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "zeroTarget.action") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRejectsCapWithReversedBounds(t *testing.T) {
	cfg := assessment.DefaultConfig()
	min := 10.0
	max := 5.0
	cfg.Indicators[0].Cap = &assessment.CapConfig{Min: &min, Max: &max}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "下限不能大于上限") {
		t.Fatalf("err = %v", err)
	}
}
