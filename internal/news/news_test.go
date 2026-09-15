package news

import "testing"

// The report holds what the candidate adds: entries the lock already has stay
// out, duplicates collapse, and the result sorts by time.
func TestDiff(t *testing.T) {
	old := Entry{Time: "2026-09-01T00:00:00+00:00", Message: "already known"}
	newer := Entry{Time: "2026-09-13T00:00:00+00:00", Message: "later change"}
	fresh := Entry{Time: "2026-09-12T00:00:00+00:00", Message: "new option"}

	got := Diff([]Entry{old}, []Entry{old, newer, fresh, fresh})
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %v", len(got), got)
	}
	if got[0] != fresh || got[1] != newer {
		t.Errorf("entries out of time order: %v", got)
	}

	if empty := Diff([]Entry{old, fresh, newer}, []Entry{old, newer, fresh}); len(empty) != 0 {
		t.Errorf("equal lists still report %v", empty)
	}
}
