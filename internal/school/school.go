// Package school 根据入学年份、当前学年和年级阶段推导级、届与阶段。
package school

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 阶段序号。1 到 3 对应高一、高二、高三。
const (
	StageUpcoming  = 0
	StageFirst     = 1
	StageSecond    = 2
	StageThird     = 3
	StageGraduated = 4
)

var (
	// ErrAcademicYear 表示学年写法无法解析。
	ErrAcademicYear = errors.New("学年写法无法解析")
	// ErrGradeLevel 表示年级阶段写法无法识别。
	ErrGradeLevel = errors.New("年级阶段无法识别")
	// ErrEnrollmentYear 表示入学年份超出允许范围。
	ErrEnrollmentYear = errors.New("入学年份超出允许范围")
)

// AcademicYearStart 返回日期所属学年的起始年份，9 月 1 日进入新学年。
func AcademicYearStart(at time.Time) int {
	if at.Month() >= time.September {
		return at.Year()
	}
	return at.Year() - 1
}

// AcademicYearLabel 返回学年的显示写法，例如 2026-2027。
func AcademicYearLabel(start int) string {
	return fmt.Sprintf("%d-%d", start, start+1)
}

// ParseAcademicYearStart 读取学年起始年份，兼容 2026-2027 与 2026 两种写法。
func ParseAcademicYearStart(value string) (int, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, ErrAcademicYear
	}
	if index := strings.IndexAny(text, "-—–~"); index >= 0 {
		text = strings.TrimSpace(text[:index])
	}
	year, err := strconv.Atoi(text)
	if err != nil || year < 1900 || year > 3000 {
		return 0, fmt.Errorf("%w: %q", ErrAcademicYear, value)
	}
	return year, nil
}

// GraduationYear 返回默认毕业年份。
func GraduationYear(enrollment int) int {
	return enrollment + 3
}

// StageIndex 返回级在指定学年的阶段序号。
func StageIndex(enrollment, academicYearStart int) int {
	index := academicYearStart - enrollment + 1
	switch {
	case index < StageFirst:
		return StageUpcoming
	case index > StageThird:
		return StageGraduated
	default:
		return index
	}
}

// StageKey 返回阶段在数据库中的写法。
func StageKey(index int) string {
	switch index {
	case StageFirst:
		return "g1"
	case StageSecond:
		return "g2"
	case StageThird:
		return "g3"
	case StageGraduated:
		return "graduated"
	default:
		return "upcoming"
	}
}

// StageIndexForKey 读取阶段序号。
func StageIndexForKey(key string) (int, error) {
	switch key {
	case "g1":
		return StageFirst, nil
	case "g2":
		return StageSecond, nil
	case "g3":
		return StageThird, nil
	case "graduated":
		return StageGraduated, nil
	case "upcoming":
		return StageUpcoming, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrGradeLevel, key)
	}
}

// EnrollmentYear 根据学年和阶段反推入学年份。
func EnrollmentYear(academicYearStart int, gradeKey string) (int, error) {
	index, err := StageIndexForKey(gradeKey)
	if err != nil {
		return 0, err
	}
	if index < StageFirst || index > StageThird {
		return 0, fmt.Errorf("%w: %q", ErrGradeLevel, gradeKey)
	}
	return academicYearStart - index + 1, nil
}

// ValidateEnrollmentYear 检查入学年份是否落在可用范围。
func ValidateEnrollmentYear(year, currentAcademicYearStart int) error {
	if year < 1990 || year > currentAcademicYearStart {
		return fmt.Errorf("%w: %d", ErrEnrollmentYear, year)
	}
	return nil
}
