package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"tally/locales"
)

func TestBaseDataStudentDetailCardAndSnapshots(t *testing.T) {
	aEmpty := newTestApp(t)
	aEmpty.page = pageBaseData
	ttEmpty := ui.NewTester(aEmpty.view, 1280, 800)
	useChineseUI(t)
	ttEmpty.SetLocale("zh-Hans")
	settleTester(ttEmpty)
	captureDualSnapshot(t, ttEmpty, "view_basedata_empty")

	a := newChinesePopulatedTestApp(t)
	a.page = pageBaseData
	tt := ui.NewTester(a.view, 1280, 800)
	useChineseUI(t)
	tt.SetLocale("zh-Hans")
	settleTester(tt)

	if err := tt.Click(locales.BaseDataTabStudents()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.baseDataTab != 1 || len(a.students) == 0 {
		t.Fatalf("students tab state: tab=%d len=%d", a.baseDataTab, len(a.students))
	}

	firstStudent := a.students[0]
	if err := tt.Click(firstStudent.Name); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if !a.studentDetailOpen || a.selectedStudentID != firstStudent.ID {
		t.Fatalf("student detail panel did not open: open=%v selected=%d want=%d",
			a.studentDetailOpen, a.selectedStudentID, firstStudent.ID)
	}
	if !tt.HasText(locales.StudentDetailTitle()) || !tt.HasText(firstStudent.StudentNo) {
		t.Fatalf("student detail card missing expected text: %q", tt.Texts())
	}
	if tt.FramePending() {
		t.Fatal("student detail panel enter did not settle")
	}

	captureDualSnapshot(t, tt, "view_basedata_populated")

	if err := tt.Click(locales.StudentCloseDetail()); err != nil {
		t.Fatal(err)
	}
	settleTester(tt)
	if a.studentDetailOpen || tt.FramePending() {
		t.Fatalf("student detail panel did not close cleanly: open=%v pending=%v",
			a.studentDetailOpen, tt.FramePending())
	}
}
