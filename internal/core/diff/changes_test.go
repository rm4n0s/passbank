package diff

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// summary renders a change as "kind uuid-byte path/title [fields]".
func summary(c Change) string {
	s := fmt.Sprintf("%s %d %q %q", c.Kind, c.UUID[15], c.Path, c.Title)
	if len(c.Fields) > 0 {
		s += " [" + c.FieldList() + "]"
	}
	return s
}

func summaries(cs []Change) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = summary(c)
	}
	return out
}

func withField(e *model.Entry, key, value string, protected bool) *model.Entry {
	e = e.Clone()
	for i := range e.Fields {
		if e.Fields[i].Key == key {
			e.Fields[i].Value = secret.New([]byte(value))
			e.Fields[i].Protected = protected
			return e
		}
	}
	e.Fields = append(e.Fields, model.Field{Key: key, Value: secret.New([]byte(value)), Protected: protected})
	return e
}

func TestChanges(t *testing.T) {
	base := ent(11, "Bank", "pw", 5)
	hash := [32]byte{1}
	tests := []struct {
		name string
		a, b *model.Database
		want []string
	}{
		{
			name: "equal databases have no changes",
			a:    database(grp(1, "Root", grp(2, "Mail", base.Clone())), 0),
			b:    database(grp(1, "Root", grp(2, "Mail", base.Clone())), 0),
		},
		{
			name: "last access, usage count and history are not compared",
			a:    database(grp(1, "Root", base.Clone()), 0),
			b: database(grp(1, "Root", func() *model.Entry {
				e := base.Clone()
				e.Times.LastAccess = at(99)
				e.Times.UsageCount = 7
				e.History = []*model.Entry{ent(11, "Old", "x", 1)}
				return e
			}()), 0),
		},
		{
			name: "only-a and only-b, entries and groups, anywhere",
			a:    database(grp(1, "Root", grp(2, "Mail", ent(11, "Zeta", "p", 0), ent(12, "Alpha", "p", 0)), grp(3, "Games")), 0),
			b:    database(grp(1, "Root", grp(4, "Work", ent(21, "Job", "p", 0)), ent(22, "Top", "p", 0)), 0),
			want: []string{
				`only-a 3 "" "Games"`, `only-a 2 "" "Mail"`, `only-a 12 "Mail" "Alpha"`, `only-a 11 "Mail" "Zeta"`,
				`only-b 22 "" "Top"`, `only-b 4 "" "Work"`, `only-b 21 "Work" "Job"`,
			},
		},
		{
			name: "differs names the fields, never values",
			a:    database(grp(1, "Root", ent(11, "Bank", "old", 1)), 0),
			b:    database(grp(1, "Root", withField(ent(11, "Bank", "new", 2), model.FieldURL, "https://x", false)), 0),
			want: []string{`differs 11 "" "Bank" [password, url]`},
		},
		{
			name: "a protected flag alone differs",
			a:    database(grp(1, "Root", ent(11, "Bank", "pw", 1)), 0),
			b:    database(grp(1, "Root", withField(ent(11, "Bank", "pw", 1), model.FieldPassword, "pw", false)), 0),
			want: []string{`differs 11 "" "Bank" [password]`},
		},
		{
			name: "a custom field is named with its key",
			a:    database(grp(1, "Root", withField(ent(11, "Bank", "pw", 1), "PIN", "1234", true)), 0),
			b:    database(grp(1, "Root", withField(ent(11, "Bank", "pw", 1), "PIN", "4321", true)), 0),
			want: []string{`differs 11 "" "Bank" [field PIN]`},
		},
		{
			name: "field order does not count, a missing field does",
			a: database(grp(1, "Root", func() *model.Entry {
				e := ent(11, "Bank", "pw", 1)
				e.Fields[0], e.Fields[2] = e.Fields[2], e.Fields[0]
				return e
			}()), 0),
			b:    database(grp(1, "Root", ent(11, "Bank", "pw", 1)), 0),
			want: nil,
		},
		{
			name: "every other entry property",
			a:    database(grp(1, "Root", ent(11, "Bank", "pw", 1)), 0),
			b: database(grp(1, "Root", func() *model.Entry {
				e := ent(11, "Bank", "pw", 1)
				e.IconID = 5
				e.ForegroundColor = "#ff0000"
				e.OverrideURL = "cmd://x"
				e.Tags = []string{"a"}
				e.Times.Expires = true
				e.Times.Expiry = at(50)
				e.AutoType.Enabled = true
				e.AutoType.DefaultSequence = "{USERNAME}"
				e.CustomData = []model.CustomDataItem{{Key: "k", Value: "v"}}
				e.Attachments = []model.Attachment{{Name: "f.txt", Binary: model.BinaryKey{Hash: hash}}}
				e.Raw.Nodes = []*model.RawNode{{Name: "Extra"}}
				return e
			}()), 0),
			want: []string{`differs 11 "" "Bank" [attachments, icon, colors, override-url, tags, expiry, auto-type, custom-data, extra-xml]`},
		},
		{
			name: "tags in another order, custom data in another order and with another time are equal",
			a: database(grp(1, "Root", func() *model.Entry {
				e := ent(11, "Bank", "pw", 1)
				e.Tags = []string{"a", "b"}
				e.CustomData = []model.CustomDataItem{{Key: "x", Value: "1", LastModified: at(1)}, {Key: "y", Value: "2"}}
				return e
			}()), 0),
			b: database(grp(1, "Root", func() *model.Entry {
				e := ent(11, "Bank", "pw", 1)
				e.Tags = []string{"b", "a"}
				e.CustomData = []model.CustomDataItem{{Key: "y", Value: "2"}, {Key: "x", Value: "1", LastModified: at(9)}}
				return e
			}()), 0),
		},
		{
			name: "the expiry time does not count when the entry does not expire",
			a: database(grp(1, "Root", func() *model.Entry {
				e := ent(11, "Bank", "pw", 1)
				e.Times.Expiry = at(3)
				return e
			}()), 0),
			b: database(grp(1, "Root", ent(11, "Bank", "pw", 1)), 0),
		},
		{
			name: "an attachment with the same name and another hash differs; the same in another order does not",
			a: database(grp(1, "Root", func() *model.Entry {
				e := ent(11, "Bank", "pw", 1)
				e.Attachments = []model.Attachment{{Name: "a", Binary: model.BinaryKey{Hash: hash}}, {Name: "b", Binary: model.BinaryKey{Hash: [32]byte{2}}}}
				return e
			}()), 0),
			b: database(grp(1, "Root", func() *model.Entry {
				e := ent(11, "Bank", "pw", 1)
				e.Attachments = []model.Attachment{{Name: "b", Binary: model.BinaryKey{Hash: [32]byte{2}}}, {Name: "a", Binary: model.BinaryKey{Hash: [32]byte{3}}}}
				return e
			}()), 0),
			want: []string{`differs 11 "" "Bank" [attachments]`},
		},
		{
			name: "an entry in another group differs by group",
			a:    database(grp(1, "Root", grp(2, "Mail", ent(11, "Bank", "pw", 1)), grp(3, "Work")), 0),
			b:    database(grp(1, "Root", grp(2, "Mail"), grp(3, "Work", ent(11, "Bank", "pw", 1))), 0),
			want: []string{`differs 11 "Mail" "Bank" [group]`},
		},
		{
			name: "entries in paired groups with different UUIDs (roots, bins) are in the same group",
			a:    database(grp(1, "Root", ent(10, "Top", "pw", 1), grp(8, "Bin", ent(11, "Gone", "pw", 1))), 8),
			b:    database(grp(2, "Root", ent(10, "Top", "pw", 1), grp(9, "Bin", ent(11, "Gone", "pw", 1))), 9),
		},
		{
			name: "deleted-in-a and deleted-in-b",
			a:    database(grp(1, "Root", grp(2, "Mail", ent(11, "Kept in A", "p", 1))), 0, deleted(21, 7)),
			b:    database(grp(1, "Root", grp(2, "Mail", ent(21, "Deleted in A", "p", 1))), 0, deleted(11, 8)),
			want: []string{`deleted-in-a 21 "Mail" "Deleted in A"`, `deleted-in-b 11 "Mail" "Kept in A"`},
		},
		{
			name: "a deleted entry that exists on both sides is a pair, not a deletion",
			a:    database(grp(1, "Root", ent(11, "Bank", "pw", 1)), 0, deleted(11, 7)),
			b:    database(grp(1, "Root", ent(11, "Bank", "pw", 1)), 0),
		},
		{
			name: "an entry only in one side that the other deleted is not only-a",
			a:    database(grp(1, "Root", ent(11, "X", "p", 1)), 0),
			b:    database(grp(1, "Root"), 0, deleted(11, 3), deleted(11, 4)),
			want: []string{`deleted-in-b 11 "" "X"`},
		},
		{
			name: "group-differs: name, notes, tags, parent",
			a:    database(grp(1, "Root", grp(2, "Mail", grp(3, "Deep")), grp(4, "Work")), 0),
			b:    database(grp(1, "Root", grp(2, "E-mail", func() any { g := grp(4, "Work", grp(3, "Deep")); g.Notes = "n"; g.Tags = []string{"t"}; return g }())), 0),
			want: []string{
				`group-differs 2 "" "Mail" [name]`, `group-differs 4 "" "Work" [notes, tags, parent]`, `group-differs 3 "Mail" "Deep" [parent]`,
			},
		},
		{
			name: "group-differs: settings, expiry, icon, custom data; interface state is ignored",
			a:    database(grp(1, "Root", grp(2, "G")), 0),
			b: database(grp(1, "Root", func() any {
				g := grp(2, "G")
				g.IsExpanded = true
				g.LastTopVisibleEntry = u(77)
				return g
			}()), 0),
		},
		{
			name: "group-differs lists all the properties",
			a:    database(grp(1, "Root", grp(2, "G")), 0),
			b: database(grp(1, "Root", func() any {
				g := grp(2, "G")
				g.IconID = 4
				g.Times.Expires = true
				g.Times.Expiry = at(2)
				g.EnableSearching = model.Disabled
				g.CustomData = []model.CustomDataItem{{Key: "k", Value: "v"}}
				g.Raw.Nodes = []*model.RawNode{{Name: "X"}}
				return g
			}()), 0),
			want: []string{`group-differs 2 "" "G" [icon, expiry, settings, custom-data, extra-xml]`},
		},
		{
			name: "the roots with different UUIDs and names are a group-differs",
			a:    database(grp(1, "Root"), 0),
			b:    database(grp(2, "My database"), 0),
			want: []string{`group-differs 1 "" "Root" [name]`},
		},
		{
			name: "settings",
			a:    database(grp(1, "Root"), 0),
			b: func() *model.Database {
				db := database(grp(1, "Root"), 0)
				db.Meta.DatabaseName = "Mine"
				db.Meta.Description = "d"
				db.Meta.DefaultUserName = "me"
				db.Meta.Color = "#00ff00"
				db.Meta.HistoryMaxItems = 5
				db.Meta.RecycleBinEnabled = true
				db.Meta.MemoryProtection.Password = true
				return db
			}(),
			want: []string{`settings 0 "" "" [name, description, default-username, color, history-limits, recycle-bin, memory-protection]`},
		},
		{
			name: "the settings of equal databases and times that changed are equal",
			a: func() *model.Database {
				db := database(grp(1, "Root"), 0)
				db.Meta.DatabaseName = "X"
				db.Meta.DatabaseNameChanged = at(1)
				db.Meta.SettingsChanged = at(1)
				return db
			}(),
			b: func() *model.Database {
				db := database(grp(1, "Root"), 0)
				db.Meta.DatabaseName = "X"
				db.Meta.DatabaseNameChanged = at(2)
				db.Meta.SettingsChanged = at(2)
				return db
			}(),
		},
		{
			name: "kinds come in the order of the table",
			a: func() *model.Database {
				db := database(grp(1, "Root", grp(5, "OnlyA"), ent(10, "Same", "old", 1), grp(6, "G")), 0, deleted(31, 1))
				db.Meta.DatabaseName = "A"
				return db
			}(),
			b: func() *model.Database {
				db := database(grp(1, "Root", grp(7, "OnlyB"), ent(10, "Same", "new", 2), grp(6, "G2"), ent(31, "Gone", "p", 0)), 0)
				db.Meta.DatabaseName = "B"
				return db
			}(),
			want: []string{
				`only-a 5 "" "OnlyA"`, `only-b 7 "" "OnlyB"`, `differs 10 "" "Same" [password]`,
				`deleted-in-a 31 "" "Gone"`, `group-differs 6 "" "G" [name]`, `settings 0 "" "" [name]`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summaries(Changes(tt.a, tt.b))
			if !slices.Equal(got, tt.want) {
				t.Errorf("changes:\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestChangeDetails(t *testing.T) {
	a := database(grp(1, "Root", grp(2, "Mail", ent(11, "Bank", "old", 3))), 0, deleted(21, 9))
	b := database(grp(1, "Root", grp(2, "Mail", ent(11, "Bank", "new", 4), ent(21, "Dead", "p", 5))), 0)
	cs := Changes(a, b)
	if len(cs) != 2 {
		t.Fatalf("changes = %q", summaries(cs))
	}
	d := cs[0]
	if d.Kind != Differs || d.Item != ItemEntry || d.Entry[SideA] == nil || d.Entry[SideB] == nil || d.Parent[SideA].UUID != u(2) ||
		!d.Modified[SideA].Equal(at(3)) || !d.Modified[SideB].Equal(at(4)) {
		t.Errorf("differs = %+v", d)
	}
	del := cs[1]
	if del.Kind != DeletedInA || del.Entry[SideA] != nil || del.Entry[SideB] == nil || !del.Deleted.Equal(at(9)) || !del.Modified[SideB].Equal(at(5)) {
		t.Errorf("deleted = %+v", del)
	}
}

func TestChangesDoNotShowValues(t *testing.T) {
	a := database(grp(1, "Root", withField(ent(11, "Bank", "SECRET-OLD", 1), "PIN", "SECRET-PIN", true)), 0)
	b := database(grp(1, "Root", withField(ent(11, "Bank", "SECRET-NEW", 2), "PIN", "SECRET-PIN2", true)), 0)
	for _, c := range Changes(a, b) {
		s := fmt.Sprintf("%+v %s", c.Fields, summary(c))
		if strings.Contains(s, "SECRET") {
			t.Errorf("a value is in %s", s)
		}
	}
}

func TestChangesDoNotChangeTheInputs(t *testing.T) {
	o := modeltest.DefaultOptions()
	for seed := uint64(1); seed <= 20; seed++ {
		a, b := modeltest.Random(seed, o), modeltest.Random(seed+500, o)
		ca, cb := a.Clone(), b.Clone()
		Changes(a, b)
		if d := model.Diff(a, ca); len(d) > 0 {
			t.Fatalf("seed %d: A changed: %v", seed, d)
		}
		if d := model.Diff(b, cb); len(d) > 0 {
			t.Fatalf("seed %d: B changed: %v", seed, d)
		}
	}
}

func TestChangesStableOrder(t *testing.T) {
	o := modeltest.DefaultOptions()
	for seed := uint64(1); seed <= 20; seed++ {
		a := modeltest.Random(seed, o)
		// B is a related database: a clone with edits.
		b := a.Clone()
		n := 0
		b.WalkGroups(func(g *model.Group) bool {
			for _, e := range g.Entries {
				if n++; n%3 == 0 {
					*e = *withField(e, model.FieldPassword, fmt.Sprintf("changed-%d", n), true)
				}
			}
			return true
		})
		first := summaries(Changes(a, b))
		for i := 0; i < 5; i++ {
			if got := summaries(Changes(a, b)); !slices.Equal(got, first) {
				t.Fatalf("seed %d: the order changed", seed)
			}
		}
		// Swapping the inputs keeps the same entries but the other way round.
		sw := Changes(b, a)
		if len(sw) != len(first) {
			t.Fatalf("seed %d: %d changes and %d swapped", seed, len(first), len(sw))
		}
		// Sorted: kinds never go backwards.
		cs := Changes(a, b)
		for i := 1; i < len(cs); i++ {
			if cs[i].Kind < cs[i-1].Kind {
				t.Fatalf("seed %d: kinds out of order", seed)
			}
		}
	}
}

func TestKindNames(t *testing.T) {
	for _, k := range Kinds {
		got, ok := ParseKind(k.String())
		if !ok || got != k {
			t.Errorf("%v does not parse back", k)
		}
	}
	if _, ok := ParseKind("nope"); ok {
		t.Error("parsed a bad kind")
	}
	if Kind(99).String() != "unknown" {
		t.Error("unknown kind")
	}
}
