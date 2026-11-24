package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"tally/locales"
)

const (
	// maxNameRunes 是名称类字段（考试名、方案名、合并组名、视图名等）的最大字符数上限。
	maxNameRunes = 80
	// maxDescRunes 是描述、原因类文本字段的最大字符数上限。
	maxDescRunes = 500
	// maxExportFileBaseRunes 是导出文件名主体部分的最大字符数，防止触发 Windows 路径长度上限。
	maxExportFileBaseRunes = 80
)

// validateRuneLength 检查输入字符数是否超过上限，超限时返回本地化错误文案。
func validateRuneLength(value, label string, limit int) string {
	if utf8.RuneCountInString(strings.TrimSpace(value)) > limit {
		return fmt.Sprintf(locales.ValidationNameTooLong(), label, limit)
	}
	return ""
}

// dashOr 在文本为空时统一返回 "-"，避免界面出现空白单元格或孤立占位。
func dashOr(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// pluralCount 按数量选择单数或复数格式化模板，数量本身统一经 formatInt 千位分隔。
func pluralCount(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf(singular, formatInt(count))
	}
	return fmt.Sprintf(plural, formatInt(count))
}

// sanitizeExportFilename 清洗导出文件名中的非法路径字符，并安全截断过长主体后保留扩展名。
func sanitizeExportFilename(name, fallback string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "-", "?", "",
		"\"", "", "<", "", ">", "", "|", "-", "\n", " ", "\r", " ", "\t", " ",
	)
	base = strings.TrimSpace(replacer.Replace(base))
	for strings.Contains(base, "  ") {
		base = strings.ReplaceAll(base, "  ", " ")
	}
	base = strings.Trim(base, ". ")
	if base == "" {
		return fallback
	}
	runes := []rune(base)
	if len(runes) > maxExportFileBaseRunes {
		base = strings.TrimRight(string(runes[:maxExportFileBaseRunes]), ". -")
	}
	if base == "" {
		return fallback
	}
	if ext == "" {
		ext = filepath.Ext(fallback)
	}
	return base + ext
}

// formatInt 给整数加上千位分隔，年份这类固定编号不要用它。
func formatInt(value int) string {
	return groupDigits(strconv.Itoa(value))
}

// formatCount 给计数类型的无符号整数加上千位分隔。
func formatCount(value int64) string {
	return groupDigits(strconv.FormatInt(value, 10))
}

// formatDecimal 保留固定小数位，用于分数、均分与比值。
func formatDecimal(value float64, places int) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "-"
	}
	return strconv.FormatFloat(value, 'f', places, 64)
}

// formatPercent 把 0 到 1 之间的比值写成保留一位小数的百分数。
func formatPercent(ratio float64) string {
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return "-"
	}
	ratio = max(0, min(1, ratio))
	return formatDecimal(ratio*100, 1) + "%"
}

// formatRatio 把倍数写成带乘号的形式，避免与分数混淆。
func formatRatio(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return "-"
	}
	return formatDecimal(value, 2) + "×"
}

func groupDigits(value string) string {
	sign := ""
	if strings.HasPrefix(value, "-") {
		sign = "-"
		value = value[1:]
	}
	if len(value) <= 3 {
		return sign + value
	}
	first := len(value) % 3
	if first == 0 {
		first = 3
	}
	var b strings.Builder
	b.WriteString(sign)
	b.WriteString(value[:first])
	for index := first; index < len(value); index += 3 {
		b.WriteByte(',')
		b.WriteString(value[index : index+3])
	}
	return b.String()
}
