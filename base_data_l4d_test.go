package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/locales"
)

func TestBaseDataStudentDetailCardAndSnapshots(t *testing.T) {
	aEmpty := newTestApp(t)
	aEmpty.page = pageCohorts
	ttEmpty := ui.NewTester(aEmpty.view, 1280, 800)
	useChineseUI(t)
	ttEmpty.SetLocale("zh-Hans")
	settleTester(ttEmpty)
	captureDualSnapshot(t, ttEmpty, "view_basedata_empty")

	a := newChinesePopulatedTestApp(t)
	a.page = pageCohorts
	a.openCohortDetail(a.cohorts[0].ID)
	tt := ui.NewTester(a.view, 1280, 800)
	useChineseUI(t)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if err := tt.Click(locales.CohortTabStudents()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.cohortDetailTab != 2 || len(a.students) == 0 {
		t.Fatalf("students tab state: tab=%d len=%d", a.cohortDetailTab, len(a.students))
	}

	firstStudent := a.students[0]
	if err := tt.Click(firstStudent.Name); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.studentProfileOpen || a.selectedStudentID != firstStudent.ID {
		t.Fatalf("student profile shell did not open: open=%v selected=%d want=%d",
			a.studentProfileOpen, a.selectedStudentID, firstStudent.ID)
	}
	if !tt.HasText(firstStudent.Name) || !tt.HasText(firstStudent.StudentNo) {
		t.Fatalf("student profile shell missing expected text: %q", tt.Texts())
	}
	if tt.FramePending() {
		t.Fatal("student profile shell did not settle")
	}

	captureDualSnapshot(t, tt, "view_basedata_populated")

	if err := tt.Click(locales.Back()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.studentProfileOpen || tt.FramePending() {
		t.Fatalf("student profile shell did not close cleanly: open=%v pending=%v",
			a.studentProfileOpen, tt.FramePending())
	}
}
