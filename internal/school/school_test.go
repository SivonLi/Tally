package school

import (
	"errors"
	"testing"
	"time"
)

func TestAcademicYearStart(t *testing.T) {
	cases := []struct {
		name string
		date time.Time
		want int
	}{
		{"August belongs to the previous academic year", time.Date(2026, time.August, 31, 23, 0, 0, 0, time.Local), 2025},
		{"September starts a new academic year", time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local), 2026},
		{"January stays in the same academic year", time.Date(2027, time.January, 15, 0, 0, 0, 0, time.Local), 2026},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := AcademicYearStart(item.date); got != item.want {
				t.Fatalf("got %d, want %d", got, item.want)
			}
		})
	}
}

func TestParseAcademicYearStart(t *testing.T) {
	cases := []struct {
		value string
		want  int
	}{
		{"2026-2027", 2026},
		{"2026", 2026},
		{"2026—2027", 2026},
		{" 2026-2027 ", 2026},
	}
	for _, item := range cases {
		got, err := ParseAcademicYearStart(item.value)
		if err != nil {
			t.Fatalf("%q: %v", item.value, err)
		}
		if got != item.want {
			t.Fatalf("%q: got %d, want %d", item.value, got, item.want)
		}
	}
	for _, value := range []string{"", "二〇二六", "26"} {
		if _, err := ParseAcademicYearStart(value); !errors.Is(err, ErrAcademicYear) {
			t.Fatalf("%q: error = %v", value, err)
		}
	}
}

func TestStageIndex(t *testing.T) {
	cases := []struct {
		enrollment int
		start      int
		want       int
	}{
		{2026, 2025, StageUpcoming},
		{2026, 2026, StageFirst},
		{2026, 2027, StageSecond},
		{2026, 2028, StageThird},
		{2026, 2029, StageGraduated},
		{2023, 2026, StageGraduated},
	}
	for _, item := range cases {
		if got := StageIndex(item.enrollment, item.start); got != item.want {
			t.Fatalf("%d at %d: got %d, want %d", item.enrollment, item.start, got, item.want)
		}
	}
}

func TestStageKeysRoundTrip(t *testing.T) {
	for index := StageFirst; index <= StageThird; index++ {
		key := StageKey(index)
		got, err := StageIndexForKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if got != index {
			t.Fatalf("%q: got %d, want %d", key, got, index)
		}
	}
	if _, err := StageIndexForKey("g9"); !errors.Is(err, ErrGradeLevel) {
		t.Fatalf("error = %v", err)
	}
}

func TestEnrollmentYear(t *testing.T) {
	year, err := EnrollmentYear(2026, "g2")
	if err != nil {
		t.Fatal(err)
	}
	if year != 2025 {
		t.Fatalf("year = %d", year)
	}
	if _, err := EnrollmentYear(2026, "graduated"); !errors.Is(err, ErrGradeLevel) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateEnrollmentYear(t *testing.T) {
	if err := ValidateEnrollmentYear(2026, 2026); err != nil {
		t.Fatal(err)
	}
	for _, year := range []int{1989, 2027} {
		if err := ValidateEnrollmentYear(year, 2026); !errors.Is(err, ErrEnrollmentYear) {
			t.Fatalf("%d: error = %v", year, err)
		}
	}
}
