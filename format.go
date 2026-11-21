package main

import (
	"math"
	"strconv"
	"strings"
)

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
