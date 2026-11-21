package assessment_test

import (
	"strings"
	"testing"

	"tally/internal/assessment"
)

func TestDefaultConfigFormulasAreValid(t *testing.T) {
	if err := assessment.DefaultConfig().ValidateFormulas(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateFormulasRejectsUnknownVariable(t *testing.T) {
	config := assessment.DefaultConfig()
	config.Indicators[0].Formula = "class.missing"
	err := config.ValidateFormulas()
	if err == nil || !strings.Contains(err.Error(), "未知变量") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateFormulasRejectsMissingIndicatorReference(t *testing.T) {
	config := assessment.DefaultConfig()
	config.Indicators[2].Formula = `indicator["missing"]`
	err := config.ValidateFormulas()
	if err == nil || !strings.Contains(err.Error(), "不存在的指标") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateFormulasRejectsCycle(t *testing.T) {
	config := assessment.DefaultConfig()
	config.Indicators[0].Formula = `indicator["pass"]`
	config.Indicators[1].Formula = `indicator["improvement"]`
	err := config.ValidateFormulas()
	if err == nil || !strings.Contains(err.Error(), "循环引用") {
		t.Fatalf("err = %v", err)
	}
}
