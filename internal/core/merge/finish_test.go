package merge

import (
	"crypto/sha256"
	"slices"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// version is a history version of entry n: the title and password, modified at the hour.
func version(n byte, title, password string, hours int) *model.Entry {
	return ent(n, title, password, hours)
}

func withHistory(e *model.Entry, versions ...*model.Entry) *model.Entry {
	e.History = versions
	return e
}

func historyOf(t *testing.T, db *model.Database, n byte) []string {
	t.Helper()
	e, _, ok := db.FindEntry(u(n))
	if !ok {
		t.Fatalf("entry %d is not in the result", n)
	}
	var out []string
	for _, h := range e.History {
		pw, _ := h.Field(model.FieldPassword)
		out = append(out, h.Title()+"/"+pw.Text())
		if h.UUID != e.UUID || len(h.History) != 0 {
			t.Errorf("a history version has the UUID %s or a history of its own", h.UUID)
		}
	}
	return out
}

func TestHistories(t *testing.T) {
	tests := []struct {
		name    string
		a, b    *model.Entry
		choice  plan.Choice // 0 means the entries are equal, so there is no change
		want    []string
		wantNil bool
	}{
		{
			name: "equal entries: the union of the histories, oldest first, without duplicates",
			a:    withHistory(ent(11, "X", "now", 9), version(11, "X", "v1", 1), version(11, "X", "v3", 3)),
			b:    withHistory(ent(11, "X", "now", 9), version(11, "X", "v1", 1), version(11, "X", "v2", 2)),
			want: []string{"X/v1", "X/v2", "X/v3"},
		},
		{
			name: "the same time with other content are two versions",
			a:    withHistory(ent(11, "X", "now", 9), version(11, "X", "a", 1)),
			b:    withHistory(ent(11, "X", "now", 9), version(11, "X", "b", 1)),
			want: []string{"X/a", "X/b"},
		},
		{
			name:   "take-a adds B's current version",
			a:      withHistory(ent(11, "X", "a-now", 5), version(11, "X", "a-old", 1)),
			b:      withHistory(ent(11, "X", "b-now", 7), version(11, "X", "b-old", 2)),
			choice: plan.TakeA,
			want:   []string{"X/a-old", "X/b-old", "X/b-now"},
		},
		{
			name:   "take-b adds A's current version",
			a:      withHistory(ent(11, "X", "a-now", 5), version(11, "X", "a-old", 1)),
			b:      withHistory(ent(11, "X", "b-now", 7), version(11, "X", "b-old", 2)),
			choice: plan.TakeB,
			want:   []string{"X/a-old", "X/b-old", "X/a-now"},
		},
		{
			name:   "the other version is not added twice when it is in the history already",
			a:      withHistory(ent(11, "X", "a-now", 5), version(11, "X", "a-old", 1)),
			b:      withHistory(ent(11, "X", "b-now", 7), version(11, "X", "a-now", 5)),
			choice: plan.TakeB,
			want:   []string{"X/a-old", "X/a-now"},
		},
		{
			name:   "both keeps the history each copy came with",
			a:      withHistory(ent(11, "X", "a-now", 5), version(11, "X", "a-old", 1)),
			b:      withHistory(ent(11, "X", "b-now", 7), version(11, "X", "b-old", 2)),
			choice: plan.Both,
			want:   []string{"X/a-old"},
		},
		{
			name:   "no history on either side",
			a:      ent(11, "X", "a", 5),
			b:      ent(11, "X", "b", 7),
			choice: plan.TakeB,
			want:   []string{"X/a"},
		},
		{
			name:    "equal entries without history have none",
			a:       ent(11, "X", "a", 5),
			b:       ent(11, "X", "a", 5),
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := database(grp(1, "Root", tt.a), 0)
			b := database(grp(1, "Root", tt.b), 0)
			var choices map[byte]plan.Choice
			if tt.choice != "" {
				choices = map[byte]plan.Choice{11: tt.choice}
			}
			res := mustApply(t, a, b, planFor(t, a, b, choices))
			got := historyOf(t, res, 11)
			if tt.wantNil {
				if len(got) != 0 {
					t.Errorf("history = %v", got)
				}
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("history = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHistoryIsNotTrimmed(t *testing.T) {
	var av, bv []*model.Entry
	for i := 0; i < 30; i++ {
		av = append(av, version(11, "X", "a", i))
		bv = append(bv, version(11, "X", "b", i))
	}
	a := database(grp(1, "Root", withHistory(ent(11, "X", "now", 99), av...)), 0)
	b := database(grp(1, "Root", withHistory(ent(11, "X", "now", 99), bv...)), 0)
	a.Meta.HistoryMaxItems = 10
	res := mustApply(t, a, b, planFor(t, a, b, nil))
	if got := len(historyOf(t, res, 11)); got != 60 {
		t.Errorf("%d versions, want 60 (history is not trimmed)", got)
	}
}

func TestDeletedObjects(t *testing.T) {
	// A deleted 21 (B has it) and 22; B deleted 23 and 22 (later).
	a := database(grp(1, "Root", ent(11, "keep", "p", 1), ent(23, "B deleted it", "p", 1)), 0, deleted(21, 5), deleted(22, 3), deleted(31, 1))
	b := database(grp(1, "Root", ent(11, "keep", "p", 1), ent(21, "A deleted it", "p", 1)), 0, deleted(23, 6), deleted(22, 8), deleted(31, 2))
	tests := []struct {
		name    string
		choices map[byte]plan.Choice
		want    map[byte]int // UUID byte -> deletion hour
	}{
		{
			name:    "the entries kept are not deleted; the others stay recorded",
			choices: map[byte]plan.Choice{21: plan.Keep, 23: plan.Keep},
			want:    map[byte]int{22: 8, 31: 2},
		},
		{
			name:    "the entries dropped stay deleted",
			choices: map[byte]plan.Choice{21: plan.Drop, 23: plan.Drop},
			want:    map[byte]int{21: 5, 23: 6, 22: 8, 31: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := mustApply(t, a, b, planFor(t, a, b, tt.choices))
			got := map[byte]int{}
			for _, d := range res.DeletedObjects {
				if _, twice := got[d.UUID[15]]; twice {
					t.Errorf("%s is deleted twice", d.UUID)
				}
				got[d.UUID[15]] = int(d.DeletionTime.Sub(at(0)) / time.Hour)
			}
			if len(got) != len(tt.want) {
				t.Errorf("deleted = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("deleted = %v, want %v", got, tt.want)
				}
			}
		})
	}
	// A's records come first, in A's order.
	res := mustApply(t, a, b, planFor(t, a, b, map[byte]plan.Choice{21: plan.Drop, 23: plan.Drop}))
	var order []byte
	for _, d := range res.DeletedObjects {
		order = append(order, d.UUID[15])
	}
	if !slices.Equal(order, []byte{21, 22, 31, 23}) {
		t.Errorf("order = %v", order)
	}
}

func TestMeta(t *testing.T) {
	icon := func(n byte, data string, hours int) model.CustomIcon {
		return model.CustomIcon{UUID: u(n), Data: []byte(data), LastModified: at(hours)}
	}
	newA := func() *model.Database {
		db := database(grp(1, "Root", grp(2, "Mail")), 0)
		db.Meta.DatabaseName = "A name"
		db.Meta.DatabaseNameChanged = at(1)
		db.Meta.Description = "A description"
		db.Meta.DefaultUserName = "me-a"
		db.Meta.Color = "#aaaaaa"
		db.Meta.HistoryMaxItems = 10
		db.Meta.HistoryMaxSize = 100
		db.Meta.MaintenanceHistoryDays = 365
		db.Meta.MemoryProtection = model.MemoryProtection{Password: true}
		db.Meta.CustomIcons = []model.CustomIcon{icon(1, "same", 1), icon(2, "a-data", 5), icon(3, "a-only", 1)}
		db.Meta.CustomData = []model.CustomDataItem{{Key: "shared", Value: "a"}, {Key: "a-only", Value: "1"}}
		return db
	}
	newB := func() *model.Database {
		db := database(grp(1, "Root", grp(2, "Mail")), 0)
		db.Meta.DatabaseName = "B name"
		db.Meta.DatabaseNameChanged = at(9)
		db.Meta.Description = "B description"
		db.Meta.DefaultUserName = "me-b"
		db.Meta.Color = "#bbbbbb"
		db.Meta.HistoryMaxItems = 20
		db.Meta.HistoryMaxSize = 200
		db.Meta.MaintenanceHistoryDays = 30
		db.Meta.RecycleBinEnabled = true
		db.Meta.MemoryProtection = model.MemoryProtection{Notes: true}
		db.Meta.CustomIcons = []model.CustomIcon{icon(1, "same", 1), icon(2, "b-data", 8), icon(4, "b-only", 1)}
		db.Meta.CustomData = []model.CustomDataItem{{Key: "shared", Value: "b"}, {Key: "b-only", Value: "2"}}
		return db
	}
	t.Run("settings from A", func(t *testing.T) {
		a, b := newA(), newB()
		res := mustApply(t, a, b, planFor(t, a, b, nil))
		m := res.Meta
		if m.DatabaseName != "A name" || m.Description != "A description" || m.DefaultUserName != "me-a" || m.Color != "#aaaaaa" ||
			m.HistoryMaxItems != 10 || m.HistoryMaxSize != 100 || m.MaintenanceHistoryDays != 365 || m.RecycleBinEnabled || !m.MemoryProtection.Password {
			t.Errorf("meta = %+v", m)
		}
		if m.CustomData[0].Value != "a" {
			t.Errorf("the shared key comes from %q, want A's", m.CustomData[0].Value)
		}
	})
	t.Run("settings from B", func(t *testing.T) {
		a, b := newA(), newB()
		p := planFor(t, a, b, nil)
		for i, c := range p.Changes {
			if c.Kind.String() == "settings" {
				p.Set(i, plan.TakeB)
			}
		}
		res := mustApply(t, a, b, p)
		m := res.Meta
		if m.DatabaseName != "B name" || !m.DatabaseNameChanged.Equal(at(9)) || m.Description != "B description" || m.DefaultUserName != "me-b" || m.Color != "#bbbbbb" ||
			m.HistoryMaxItems != 20 || m.HistoryMaxSize != 200 || m.MaintenanceHistoryDays != 30 || !m.RecycleBinEnabled || !m.MemoryProtection.Notes || m.MemoryProtection.Password {
			t.Errorf("meta = %+v", m)
		}
		// The custom data conflict is decided by the input that gave the settings.
		got := map[string]string{}
		for _, it := range m.CustomData {
			got[it.Key] = it.Value
		}
		if got["shared"] != "b" || got["a-only"] != "1" || got["b-only"] != "2" || len(got) != 3 {
			t.Errorf("custom data = %v", got)
		}
	})
	t.Run("custom icons are the union; the newer of a conflict wins", func(t *testing.T) {
		a, b := newA(), newB()
		res := mustApply(t, a, b, planFor(t, a, b, nil))
		var got []string
		for _, ic := range res.Meta.CustomIcons {
			got = append(got, string(ic.Data))
		}
		// 2 conflicts: B's was modified later (hour 8 > 5).
		if want := []string{"same", "b-data", "a-only", "b-only"}; !slices.Equal(got, want) {
			t.Errorf("icons = %v, want %v", got, want)
		}
		// With the times the other way round, A's stays.
		a.Meta.CustomIcons[1].LastModified = at(20)
		res = mustApply(t, a, b, planFor(t, a, b, nil))
		if string(res.Meta.CustomIcons[1].Data) != "a-data" {
			t.Errorf("icon 2 = %q, want A's", res.Meta.CustomIcons[1].Data)
		}
		// Without times (before KDBX 4.1) A's stays too.
		a.Meta.CustomIcons[1].LastModified = time.Time{}
		b.Meta.CustomIcons[1].LastModified = time.Time{}
		res = mustApply(t, a, b, planFor(t, a, b, nil))
		if string(res.Meta.CustomIcons[1].Data) != "a-data" {
			t.Errorf("icon 2 without times = %q, want A's", res.Meta.CustomIcons[1].Data)
		}
	})
	t.Run("the result's icons are copies", func(t *testing.T) {
		a, b := newA(), newB()
		res := mustApply(t, a, b, planFor(t, a, b, nil))
		res.Meta.CustomIcons[3].Data[0] = 'X'
		if string(b.Meta.CustomIcons[2].Data) != "b-only" {
			t.Error("the result shares an icon with B")
		}
	})
}

func TestRecycleBinAndReferences(t *testing.T) {
	t.Run("A has no bin and B's is kept: it becomes the bin", func(t *testing.T) {
		a := database(grp(1, "Root"), 0)
		b := database(grp(1, "Root", grp(9, "Bin", ent(21, "gone", "p", 1))), 9)
		res := mustApply(t, a, b, planFor(t, a, b, nil))
		if res.Meta.RecycleBinUUID != u(9) {
			t.Errorf("bin = %s, want %s", res.Meta.RecycleBinUUID, u(9))
		}
	})
	t.Run("A has no bin and B's is dropped: there is none", func(t *testing.T) {
		a := database(grp(1, "Root"), 0)
		b := database(grp(1, "Root", grp(9, "Bin", ent(21, "gone", "p", 1))), 9)
		res := mustApply(t, a, b, planFor(t, a, b, map[byte]plan.Choice{9: plan.Drop, 21: plan.Drop}))
		if !res.Meta.RecycleBinUUID.IsZero() {
			t.Errorf("bin = %s", res.Meta.RecycleBinUUID)
		}
	})
	t.Run("A's bin stays A's, with B's differing UUID", func(t *testing.T) {
		a := database(grp(1, "Root", grp(8, "Bin")), 8)
		b := database(grp(1, "Root", grp(9, "Bin", ent(21, "gone", "p", 1))), 9)
		res := mustApply(t, a, b, planFor(t, a, b, nil))
		if res.Meta.RecycleBinUUID != u(8) {
			t.Errorf("bin = %s", res.Meta.RecycleBinUUID)
		}
	})
	t.Run("references to dropped groups are cleared", func(t *testing.T) {
		a := database(grp(1, "Root", grp(2, "Templates"), grp(3, "Selected"), grp(4, "Kept")), 0)
		a.Meta.EntryTemplatesGroup = u(2)
		a.Meta.LastSelectedGroup = u(3)
		a.Meta.LastTopVisibleGroup = u(4)
		b := database(grp(1, "Root", grp(4, "Kept")), 0)
		res := mustApply(t, a, b, planFor(t, a, b, map[byte]plan.Choice{2: plan.Drop, 3: plan.Drop}))
		m := res.Meta
		if !m.EntryTemplatesGroup.IsZero() || !m.LastSelectedGroup.IsZero() || m.LastTopVisibleGroup != u(4) {
			t.Errorf("references = %s %s %s", m.EntryTemplatesGroup, m.LastSelectedGroup, m.LastTopVisibleGroup)
		}
	})
	t.Run("a reference below a dropped group is cleared", func(t *testing.T) {
		a := database(grp(1, "Root", grp(2, "Outer", grp(3, "Inner"))), 0)
		a.Meta.LastSelectedGroup = u(3)
		b := database(grp(1, "Root"), 0)
		res := mustApply(t, a, b, planFor(t, a, b, map[byte]plan.Choice{2: plan.Drop, 3: plan.Drop}))
		if !res.Meta.LastSelectedGroup.IsZero() {
			t.Errorf("reference = %s", res.Meta.LastSelectedGroup)
		}
	})
}

func blob(data string) (model.BinaryKey, *model.Binary) {
	return model.BinaryKey{Hash: sha256.Sum256([]byte(data))}, &model.Binary{Data: secret.New([]byte(data))}
}

func TestAttachmentPool(t *testing.T) {
	ka, ba := blob("only in A")
	kb, bb := blob("only in B")
	ks, bsA := blob("shared")
	_, bsB := blob("shared")
	ku, bu := blob("unused")
	kh, bh := blob("history only")

	a := database(grp(1, "Root",
		func() *model.Entry {
			e := ent(11, "has A", "p", 1)
			e.Attachments = []model.Attachment{{Name: "a.txt", Binary: ka}, {Name: "s.txt", Binary: ks}}
			e.History = []*model.Entry{func() *model.Entry {
				h := ent(11, "old", "p", 0)
				h.Attachments = []model.Attachment{{Name: "h.txt", Binary: kh}}
				return h
			}()}
			return e
		}(),
		ent(12, "dropped", "p", 1)), 0)
	a.Binaries = map[model.BinaryKey]*model.Binary{ka: ba, ks: bsA, ku: bu, kh: bh}
	b := database(grp(1, "Root", func() *model.Entry {
		e := ent(21, "has B", "p", 1)
		e.Attachments = []model.Attachment{{Name: "b.txt", Binary: kb}, {Name: "s2.txt", Binary: ks}}
		return e
	}()), 0)
	b.Binaries = map[model.BinaryKey]*model.Binary{kb: bb, ks: bsB}

	res := mustApply(t, a, b, planFor(t, a, b, map[byte]plan.Choice{12: plan.Drop}))
	var got []string
	for k, bin := range res.Binaries {
		got = append(got, string(bin.Data.Reveal()))
		if k.Hash != sha256.Sum256(bin.Data.Reveal()) {
			t.Errorf("binary %q is under the wrong key", bin.Data.Reveal())
		}
	}
	slices.Sort(got)
	want := []string{"history only", "only in A", "only in B", "shared"}
	if !slices.Equal(got, want) {
		t.Errorf("pool = %v, want %v (no unused binary, the shared one once)", got, want)
	}
	// The pool holds copies of B's binaries.
	res.Binaries[kb].Data.Wipe()
	if string(bb.Data.Reveal()) != "only in B" {
		t.Error("the pool shares a binary with B")
	}
}

// Every reference of the result resolves: attachments to the pool, custom icons to
// the icons of the Meta, groups of the Meta to groups of the tree.
func TestNoDanglingReferences(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		a, b := related(seed)
		for _, rule := range plan.Rules {
			p := planFor(t, a, b, nil)
			plan.ApplyRule(p, rule, nil)
			res := mustApply(t, a, b, p)
			icons := map[model.UUID]bool{}
			for _, ic := range res.Meta.CustomIcons {
				icons[ic.UUID] = true
			}
			check := func(e *model.Entry) {
				for _, at := range e.Attachments {
					if _, ok := res.Binaries[at.Binary]; !ok {
						t.Fatalf("seed %d, rule %s: entry %s references a binary the pool lacks", seed, rule, e.UUID)
					}
				}
				if !e.CustomIcon.IsZero() && !icons[e.CustomIcon] {
					t.Fatalf("seed %d, rule %s: entry %s references the icon %s, which the result lacks", seed, rule, e.UUID, e.CustomIcon)
				}
			}
			res.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
				check(e)
				for _, h := range e.History {
					check(h)
				}
				return true
			})
			for _, ref := range []model.UUID{res.Meta.RecycleBinUUID, res.Meta.EntryTemplatesGroup, res.Meta.LastSelectedGroup, res.Meta.LastTopVisibleGroup} {
				if ref.IsZero() {
					continue
				}
				if _, _, ok := res.FindGroup(ref); !ok {
					t.Fatalf("seed %d, rule %s: the Meta refers to the group %s, which is not in the tree", seed, rule, ref)
				}
			}
		}
	}
}
