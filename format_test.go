package main

import (
	"math"
	"testing"
)

func TestFormatPercentValidatesInput(t *testing.T) {
	cases := []struct {
		ratio float64
		want  string
	}{
		{0, "0.0%"},
		{0.12345, "12.3%"},
		{1, "100.0%"},
		{-0.2, "0.0%"},
		{1.2, "100.0%"},
		{math.NaN(), "-"},
	}
	for _, item := range cases {
		if got := formatPercent(item.ratio); got != item.want {
			t.Fatalf("formatPercent(%v) = %q, want %q", item.ratio, got, item.want)
		}
	}
}

func TestFormatRatioValidatesInput(t *testing.T) {
	cases := []struct {
		ratio float64
		want  string
	}{
		{1.2, "1.20×"},
		{0, "-"},
		{-1, "-"},
		{math.Inf(1), "-"},
		{math.NaN(), "-"},
	}
	for _, item := range cases {
		if got := formatRatio(item.ratio); got != item.want {
			t.Fatalf("formatRatio(%v) = %q, want %q", item.ratio, got, item.want)
		}
	}
}
