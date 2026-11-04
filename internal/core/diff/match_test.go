package diff

import (
	"testing"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
)

func uuids(gs []*model.Group) []byte {
	var out []byte
	for _, g := range gs {
		out = append(out, g.UUID[15])
	}
	return out
}

func entryUUIDs(ls []Located) []byte {
	var out []byte
	for _, l := range ls {
		out = append(out, l.Entry.UUID[15])
	}
	return out
}

func eq(a, b []byte) bool { return string(a) == string(b) }

func TestMatch(t *testing.T) {
	tests := []struct {
		name                       string
		a, b                       *model.Database
		entryPairs                 []byte
		groupPairs                 [][2]byte
		onlyAEntries, onlyBEntries []byte
		onlyAGroups, onlyBGroups   []byte
	}{
		{
			name:       "equal databases",
			a:          database(grp(1, "Root", grp(2, "Mail", ent(11, "m", "p", 0)), ent(10, "r", "p", 0)), 0),
			b:          database(grp(1, "Root", grp(2, "Mail", ent(11, "m", "p", 0)), ent(10, "r", "p", 0)), 0),
			entryPairs: []byte{10, 11}, groupPairs: [][2]byte{{1, 1}, {2, 2}},
		},
		{
			name:         "unrelated databases: only the roots pair",
			a:            database(grp(1, "Root", grp(2, "Mail", ent(11, "m", "p", 0))), 0),
			b:            database(grp(5, "Root", grp(6, "Work", ent(21, "w", "p", 0))), 0),
			groupPairs:   [][2]byte{{1, 5}},
			onlyAEntries: []byte{11}, onlyBEntries: []byte{21}, onlyAGroups: []byte{2}, onlyBGroups: []byte{6},
		},
		{
			name:       "an entry moved to another group is still a pair",
			a:          database(grp(1, "Root", grp(2, "Mail", ent(11, "m", "p", 0)), grp(3, "Work")), 0),
			b:          database(grp(1, "Root", grp(2, "Mail"), grp(3, "Work", ent(11, "m", "p", 0))), 0),
			entryPairs: []byte{11}, groupPairs: [][2]byte{{1, 1}, {2, 2}, {3, 3}},
		},
		{
			name:       "a group renamed or moved is still a pair",
			a:          database(grp(1, "Root", grp(2, "Old name", grp(3, "Inner"))), 0),
			b:          database(grp(1, "Root", grp(3, "Inner"), grp(2, "New name")), 0),
			groupPairs: [][2]byte{{1, 1}, {2, 2}, {3, 3}},
		},
		{
			name:       "items only in one side, anywhere in the tree",
			a:          database(grp(1, "Root", grp(2, "A only", ent(11, "x", "p", 0)), ent(10, "common", "p", 0)), 0),
			b:          database(grp(1, "Root", grp(3, "B only", grp(4, "Deep", ent(21, "y", "p", 0))), ent(10, "common", "p", 0)), 0),
			entryPairs: []byte{10}, groupPairs: [][2]byte{{1, 1}},
			onlyAEntries: []byte{11}, onlyBEntries: []byte{21}, onlyAGroups: []byte{2}, onlyBGroups: []byte{3, 4},
		},
		{
			name:       "recycle bins with different UUIDs are a pair",
			a:          database(grp(1, "Root", grp(8, "Recycle Bin", ent(11, "gone a", "p", 0))), 8),
			b:          database(grp(1, "Root", grp(9, "Recycle Bin", ent(21, "gone b", "p", 0))), 9),
			groupPairs: [][2]byte{{1, 1}, {8, 9}}, onlyAEntries: []byte{11}, onlyBEntries: []byte{21},
		},
		{
			name:       "recycle bins with the same UUID pair by UUID",
			a:          database(grp(1, "Root", grp(8, "Recycle Bin")), 8),
			b:          database(grp(1, "Root", grp(8, "Papierkorb")), 8),
			groupPairs: [][2]byte{{1, 1}, {8, 8}},
		},
		{
			name:       "no bin on one side",
			a:          database(grp(1, "Root", grp(8, "Recycle Bin")), 8),
			b:          database(grp(1, "Root", grp(9, "Recycle Bin")), 0),
			groupPairs: [][2]byte{{1, 1}}, onlyAGroups: []byte{8}, onlyBGroups: []byte{9},
		},
		{
			name:       "a bin UUID without a group is no bin",
			a:          database(grp(1, "Root", grp(8, "Recycle Bin")), 8),
			b:          database(grp(1, "Root", grp(9, "Other")), 77),
			groupPairs: [][2]byte{{1, 1}}, onlyAGroups: []byte{8}, onlyBGroups: []byte{9},
		},
		{
			name:       "a bin that already has a partner by UUID does not pair again",
			a:          database(grp(1, "Root", grp(8, "Recycle Bin"), grp(5, "Shared")), 8),
			b:          database(grp(1, "Root", grp(8, "Not the bin"), grp(9, "B bin")), 9),
			groupPairs: [][2]byte{{1, 1}, {8, 8}}, onlyAGroups: []byte{5}, onlyBGroups: []byte{9},
		},
		{
			name:       "roots with different UUIDs pair",
			a:          database(grp(1, "Root", ent(10, "x", "p", 0)), 0),
			b:          database(grp(2, "Root", ent(10, "x", "p", 0)), 0),
			entryPairs: []byte{10}, groupPairs: [][2]byte{{1, 2}},
		},
		{
			name: "databases without a root",
			a:    &model.Database{},
			b:    &model.Database{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Match(tt.a, tt.b)
			var ep []byte
			for _, p := range m.Entries {
				if p.A.UUID != p.B.UUID {
					t.Errorf("entry pair with different UUIDs")
				}
				ep = append(ep, p.A.UUID[15])
			}
			if !eq(ep, tt.entryPairs) {
				t.Errorf("entry pairs = %v, want %v", ep, tt.entryPairs)
			}
			var gp [][2]byte
			for _, p := range m.Groups {
				gp = append(gp, [2]byte{p.A.UUID[15], p.B.UUID[15]})
			}
			if len(gp) != len(tt.groupPairs) {
				t.Fatalf("group pairs = %v, want %v", gp, tt.groupPairs)
			}
			for i := range gp {
				if gp[i] != tt.groupPairs[i] {
					t.Errorf("group pairs = %v, want %v", gp, tt.groupPairs)
				}
			}
			if got := entryUUIDs(m.OnlyAEntries); !eq(got, tt.onlyAEntries) {
				t.Errorf("only in A (entries) = %v, want %v", got, tt.onlyAEntries)
			}
			if got := entryUUIDs(m.OnlyBEntries); !eq(got, tt.onlyBEntries) {
				t.Errorf("only in B (entries) = %v, want %v", got, tt.onlyBEntries)
			}
			if got := uuids(m.OnlyAGroups); !eq(got, tt.onlyAGroups) {
				t.Errorf("only in A (groups) = %v, want %v", got, tt.onlyAGroups)
			}
			if got := uuids(m.OnlyBGroups); !eq(got, tt.onlyBGroups) {
				t.Errorf("only in B (groups) = %v, want %v", got, tt.onlyBGroups)
			}
		})
	}
}

func TestMatchGroupsOfPairedEntries(t *testing.T) {
	a := database(grp(1, "Root", grp(2, "Mail", ent(11, "m", "p", 0)), grp(3, "Work")), 0)
	b := database(grp(1, "Root", grp(2, "Mail"), grp(3, "Work", ent(11, "m", "p", 0))), 0)
	m := Match(a, b)
	if len(m.Entries) != 1 || m.Entries[0].GroupA.UUID != u(2) || m.Entries[0].GroupB.UUID != u(3) {
		t.Errorf("pair = %+v", m.Entries)
	}
}

func TestMatchRandomDatabases(t *testing.T) {
	o := modeltest.DefaultOptions()
	count := func(db *model.Database) (groups, entries int) {
		db.WalkGroups(func(g *model.Group) bool { groups++; entries += len(g.Entries); return true })
		return
	}
	for seed := uint64(1); seed <= 40; seed++ {
		a := modeltest.Random(seed, o)
		same := a.Clone()
		m := Match(a, same)
		ga, ea := count(a)
		if len(m.Entries) != ea || len(m.Groups) != ga || len(m.OnlyAEntries)+len(m.OnlyBEntries)+len(m.OnlyAGroups)+len(m.OnlyBGroups) != 0 {
			t.Fatalf("seed %d: a clone is not all pairs: %d/%d entries, %d/%d groups", seed, len(m.Entries), ea, len(m.Groups), ga)
		}
		b := modeltest.Random(seed+1000, o)
		gb, eb := count(b)
		m = Match(a, b)
		if len(m.Entries)+len(m.OnlyAEntries) != ea || len(m.Entries)+len(m.OnlyBEntries) != eb {
			t.Fatalf("seed %d: entries do not add up", seed)
		}
		if len(m.Groups)+len(m.OnlyAGroups) != ga || len(m.Groups)+len(m.OnlyBGroups) != gb {
			t.Fatalf("seed %d: groups do not add up", seed)
		}
		// Independent databases share no UUIDs: only the roots (and maybe the bins) pair.
		if len(m.Entries) != 0 || len(m.Groups) < 1 || len(m.Groups) > 2 {
			t.Fatalf("seed %d: %d entry pairs and %d group pairs of unrelated databases", seed, len(m.Entries), len(m.Groups))
		}
	}
}

func TestTreeHelpers(t *testing.T) {
	db := database(grp(1, "Root", grp(2, "Mail", grp(3, "Deep", ent(11, "e", "p", 0))), grp(8, "Recycle Bin")), 8)
	tr := NewTree(db)
	deep := tr.Group(u(3))
	if tr.GroupPath(deep) != "Mail / Deep" || tr.GroupPath(db.Root) != "" || tr.GroupPath(tr.Group(u(2))) != "Mail" {
		t.Errorf("paths = %q %q", tr.GroupPath(deep), tr.GroupPath(db.Root))
	}
	if got := tr.Ancestors(deep); len(got) != 3 || got[0] != u(1) || got[1] != u(2) || got[2] != u(3) {
		t.Errorf("ancestors = %v", got)
	}
	if tr.Parent(deep).UUID != u(2) || tr.Parent(db.Root) != nil {
		t.Error("parents are wrong")
	}
	if l, ok := tr.Entry(u(11)); !ok || l.Group != deep {
		t.Errorf("entry = %+v %v", l, ok)
	}
	if _, ok := tr.Entry(u(99)); ok {
		t.Error("found an entry that is not there")
	}
	if tr.RecycleBin() == nil || tr.RecycleBin().UUID != u(8) {
		t.Error("recycle bin")
	}
	if NewTree(database(grp(1, "Root"), 0)).RecycleBin() != nil {
		t.Error("a bin without a UUID")
	}
	if SideA.String() != "A" || SideB.String() != "B" || SideA.Other() != SideB || SideB.Other() != SideA {
		t.Error("sides")
	}
	p := MatchGroups(tr, NewTree(db.Clone()))
	if p.PartnerOf(deep, SideA) == nil || p.PartnerOf(deep, SideB) != nil && false {
		t.Error("partner")
	}
}
