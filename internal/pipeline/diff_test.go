package pipeline

import (
	"testing"

	"tally/internal/store"
)

func TestDiffRuns(t *testing.T) {
	baseRows := []store.StoredResultRow{
		{StatGroupID: 1, StatGroupName: "1班", Subject: "chinese", LeadClassName: "1班", Total: 90, Rank: 2, TierIndex: 1, Points: 67},
		{StatGroupID: 2, StatGroupName: "2班", Subject: "chinese", LeadClassName: "2班", Total: 95, Rank: 1, TierIndex: 0, Points: 70},
		{StatGroupID: 3, StatGroupName: "3班", Subject: "chinese", LeadClassName: "3班", Total: 80, Rank: 3, TierIndex: 2, Points: 64},
	}
	compareRows := []store.StoredResultRow{
		{StatGroupID: 1, StatGroupName: "1班", Subject: "chinese", LeadClassName: "1班", Total: 96, Rank: 1, TierIndex: 0, Points: 70},
		{StatGroupID: 2, StatGroupName: "2班", Subject: "chinese", LeadClassName: "2班", Total: 95, Rank: 2, TierIndex: 1, Points: 67},
		{StatGroupID: 4, StatGroupName: "4班", Subject: "chinese", LeadClassName: "4班", Total: 82, Rank: 3, TierIndex: 2, Points: 64},
	}
	basePersonal := []store.StoredPersonalScore{
		{TeacherID: 1, TeacherName: "张老师", Grade: "g1", Subject: "chinese", Score: 67},
		{TeacherID: 2, TeacherName: "李老师", Grade: "g1", Subject: "chinese", Score: 70},
	}
	comparePersonal := []store.StoredPersonalScore{
		{TeacherID: 1, TeacherName: "张老师", Grade: "g1", Subject: "chinese", Score: 70},
		{TeacherID: 2, TeacherName: "李老师", Grade: "g1", Subject: "chinese", Score: 70},
	}

	diff := DiffRuns(store.ResultRun{ID: 1, IsOfficial: true}, baseRows, basePersonal,
		store.ResultRun{ID: 2, IsOfficial: false}, compareRows, comparePersonal)

	if len(diff.Rows) != 4 {
		t.Fatalf("expected 4 diff rows, got %d", len(diff.Rows))
	}
	if diff.ChangedRowCount != 4 || diff.ChangedTierCount != 2 || diff.ChangedPersonalCount != 1 {
		t.Fatalf("unexpected diff counts: rows=%d tiers=%d personal=%d",
			diff.ChangedRowCount, diff.ChangedTierCount, diff.ChangedPersonalCount)
	}
	// 1班从第2名升到第1名 -> RankDelta = +1, TierChanged = true, PointsDelta = +3
	if diff.Rows[0].StatGroupID != 1 || diff.Rows[0].RankDelta != 1 || !diff.Rows[0].TierChanged || diff.Rows[0].PointsDelta != 3 {
		t.Fatalf("unexpected first diff row: %+v", diff.Rows[0])
	}
	if diff.Rows[3].Presence != "only_base" || diff.Rows[3].StatGroupID != 3 {
		t.Fatalf("expected only_base row 3 last, got %+v", diff.Rows[3])
	}
}
