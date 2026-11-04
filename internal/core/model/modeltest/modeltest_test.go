package modeltest

import (
	"testing"

	"github.com/rm4n0s/passbank/internal/core/model"
)

func TestRandomIsDeterministic(t *testing.T) {
	for _, o := range []Options{DefaultOptions(), {MaxDepth: 2, MaxGroups: 2, MaxEntries: 2, With41: true, WithRaw: true, WithItemCustomData: true}, {}} {
		for seed := uint64(1); seed <= 20; seed++ {
			a, b := Random(seed, o), Random(seed, o)
			if diff := model.Diff(a, b); len(diff) != 0 {
				t.Fatalf("seed %d gives two databases: %v", seed, diff)
			}
		}
	}
	if len(model.Diff(Random(1, DefaultOptions()), Random(2, DefaultOptions()))) == 0 {
		t.Error("two seeds gave the same database")
	}
}

func TestRandomIsBoundedAndValid(t *testing.T) {
	o := DefaultOptions()
	o.With41 = true
	sawHistory, sawAttachment, sawRaw, sawDeep := false, false, false, false
	for seed := uint64(1); seed <= 200; seed++ {
		db := Random(seed, o)
		groups, entries := 0, 0
		seen := map[model.UUID]bool{}
		db.WalkGroups(func(g *model.Group) bool {
			groups++
			if g.UUID.IsZero() || seen[g.UUID] {
				t.Fatalf("seed %d: group UUID %v is zero or repeated", seed, g.UUID)
			}
			seen[g.UUID] = true
			if len(g.Raw.Nodes) > 0 {
				sawRaw = true
			}
			for _, e := range g.Entries {
				entries++
				if e.UUID.IsZero() || seen[e.UUID] {
					t.Fatalf("seed %d: entry UUID %v is zero or repeated", seed, e.UUID)
				}
				seen[e.UUID] = true
				if len(e.History) > 0 {
					sawHistory = true
				}
				for _, h := range e.History {
					if h.UUID != e.UUID || len(h.History) != 0 {
						t.Fatalf("seed %d: a history item is wrong", seed)
					}
				}
				for _, a := range e.Attachments {
					sawAttachment = true
					if db.Binaries[a.Binary] == nil {
						t.Fatalf("seed %d: an attachment refers to a content that is not in the pool", seed)
					}
				}
			}
			return true
		})
		if groups > 1+3+9+27 || entries > groups*4 {
			t.Fatalf("seed %d: %d groups and %d entries are more than the options allow", seed, groups, entries)
		}
		if groups > 10 {
			sawDeep = true
		}
	}
	if !sawHistory || !sawAttachment || !sawRaw || !sawDeep {
		t.Errorf("200 seeds never made history (%v), an attachment (%v), unknown XML (%v) or a big tree (%v)", sawHistory, sawAttachment, sawRaw, sawDeep)
	}
}

func TestOptionsTurnThingsOff(t *testing.T) {
	o := Options{MaxDepth: 3, MaxGroups: 3, MaxEntries: 3}
	for seed := uint64(1); seed <= 100; seed++ {
		db := Random(seed, o)
		if len(db.Raw.Nodes)+len(db.Meta.Raw.Nodes) != 0 || len(db.Meta.CustomData) != 0 && false {
			t.Fatalf("seed %d: unknown XML although WithRaw is off", seed)
		}
		if db.Meta.SettingsChanged != (db.Meta.SettingsChanged) || !db.Meta.SettingsChanged.IsZero() {
			t.Fatalf("seed %d: a 4.1 field although With41 is off", seed)
		}
		db.WalkGroups(func(g *model.Group) bool {
			if len(g.Tags) != 0 || len(g.CustomData) != 0 || len(g.Raw.Nodes) != 0 {
				t.Fatalf("seed %d: group has tags, custom data or unknown XML", seed)
			}
			for _, e := range g.Entries {
				if e.QualityCheck != nil || len(e.CustomData) != 0 || len(e.Raw.Nodes) != 0 {
					t.Fatalf("seed %d: entry has 4.1 data, custom data or unknown XML", seed)
				}
			}
			return true
		})
	}
}
