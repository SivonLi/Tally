package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"tally/locales"
)

func subjectLabel(code string) string {
	switch code {
	case "chinese":
		return locales.SubjectChinese()
	case "math":
		return locales.SubjectMath()
	case "english":
		return locales.SubjectEnglish()
	case "physics":
		return locales.SubjectPhysics()
	case "chemistry":
		return locales.SubjectChemistry()
	case "biology":
		return locales.SubjectBiology()
	case "history":
		return locales.SubjectHistory()
	case "geography":
		return locales.SubjectGeography()
	case "politics":
		return locales.SubjectPolitics()
	case "japanese":
		return locales.SubjectJapanese()
	}
	return code
}

func levelLabel(index int) string {
	switch index {
	case 0:
		return locales.LevelFirst()
	case 1:
		return locales.LevelSecond()
	case 2:
		return locales.LevelThird()
	default:
		return fmt.Sprintf(locales.LevelOther(), index+1)
	}
}

func decodeNames(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var names []string
	if err := json.Unmarshal([]byte(value), &names); err != nil {
		return value
	}
	return strings.Join(names, "、")
}

func (a *app) semesterLabel(value string) string {
	switch value {
	case "spring":
		return locales.SemesterSpring()
	case "autumn":
		return locales.SemesterAutumn()
	default:
		return value
	}
}

func (a *app) examLabel(value string) string {
	switch value {
	case "midterm":
		return locales.ExamMidterm()
	case "final":
		return locales.ExamFinal()
	case "gaokao":
		return locales.ExamGaokao()
	default:
		return value
	}
}

func (a *app) gradeLabel(value string) string {
	switch value {
	case "g1":
		return locales.GradeG1()
	case "g2":
		return locales.GradeG2()
	case "g3":
		return locales.GradeG3()
	default:
		return value
	}
}

func semesterCode(label string) string {
	switch label {
	case locales.SemesterSpring():
		return "spring"
	case locales.SemesterAutumn():
		return "autumn"
	default:
		return label
	}
}

func examCode(label string) string {
	switch label {
	case locales.ExamMidterm():
		return "midterm"
	case locales.ExamFinal():
		return "final"
	case locales.ExamGaokao():
		return "gaokao"
	default:
		return label
	}
}

func gradeCode(label string) string {
	switch label {
	case locales.GradeG1():
		return "g1"
	case locales.GradeG2():
		return "g2"
	case locales.GradeG3():
		return "g3"
	default:
		return label
	}
}

func (a *app) tierLabel(key string) string {
	if version := a.publishedByLabel(a.calculateVersion); version != nil {
		for _, tier := range version.Version.Config.Tiers {
			if tier.Key == key {
				return tier.Label
			}
		}
	}
	switch key {
	case "qingbei":
		return locales.TierQingbei()
	case "c9":
		return locales.TierC9()
	case "project985":
		return locales.TierProject985()
	case "project211":
		return locales.TierProject211()
	case "special_control":
		return locales.TierSpecialControl()
	case "undergraduate":
		return locales.TierUndergraduate()
	}
	return key
}
