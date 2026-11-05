package modeltest

import (
	"testing"

	"github.com/rm4n0s/passbank/internal/core/model"
)

func TestMergePairIsDeterministic(t *testing.T) {
	a1, b1 := MergePair()
	a2, b2 := MergePair()
	if d := model.Diff(a1, a2); len(d) > 0 {
		t.Errorf("A differs: %v", d)
	}
	if d := model.Diff(b1, b2); len(d) > 0 {
		t.Errorf("B differs: %v", d)
	}
}

func TestLargePair(t *testing.T) {
	count := func(db *model.Database) (entries int, uuids map[model.UUID]bool) {
		uuids = map[model.UUID]bool{}
		db.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
			entries++
			if uuids[e.UUID] {
				t.Fatalf("entry %s twice", e.UUID)
			}
			uuids[e.UUID] = true
			return true
		})
		return
	}
	a, b := LargePair(1000)
	na, _ := count(a)
	nb, ub := count(b)
	if na != 1000 {
		t.Errorf("A has %d entries, want 1000", na)
	}
	// 1000 - 40 deleted + 50 new.
	if nb != 1000-40+50 {
		t.Errorf("B has %d entries, want %d", nb, 1000-40+50)
	}
	if len(b.DeletedObjects) != 40 {
		t.Errorf("B deleted %d, want 40", len(b.DeletedObjects))
	}
	for _, d := range b.DeletedObjects {
		if ub[d.UUID] {
			t.Errorf("a deleted entry %s is still in B", d.UUID)
		}
	}
	a2, b2 := LargePair(1000)
	if len(model.Diff(a, a2)) > 0 || len(model.Diff(b, b2)) > 0 {
		t.Error("LargePair is not deterministic")
	}
}
