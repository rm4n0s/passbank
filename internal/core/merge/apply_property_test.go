package merge

import (
	"fmt"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// related returns A (random) and B, a copy of A that was edited elsewhere: some
// entries changed, deleted or moved, and new entries and groups added. The edits
// depend on the seed.
func related(seed uint64) (a, b *model.Database) {
	a = modeltest.Random(seed, modeltest.DefaultOptions())
	b = a.Clone()
	n := 0
	var groups []*model.Group
	b.WalkGroups(func(g *model.Group) bool { groups = append(groups, g); return true })
	for _, g := range groups {
		var kept []*model.Entry
		for _, e := range g.Entries {
			n++
			switch {
			case n%7 == 0: // deleted in B
				b.DeletedObjects = append(b.DeletedObjects, model.DeletedObject{UUID: e.UUID, DeletionTime: e.Times.LastModification.Add(1)})
				continue
			case n%5 == 0: // password changed in B
				for i := range e.Fields {
					if e.Fields[i].Key == model.FieldPassword {
						e.Fields[i].Value = secret.New([]byte(fmt.Sprintf("changed-%d", n)))
					}
				}
				e.Times.LastModification = e.Times.LastModification.Add(48 * 3600e9)
			case n%11 == 0 && len(groups) > 1: // moved in B
				other := groups[(n/11)%len(groups)]
				if other != g {
					other.Entries = append(other.Entries, e)
					continue
				}
			}
			kept = append(kept, e)
		}
		g.Entries = kept
		if n%3 == 0 {
			g.Name += " (renamed)"
		}
	}
	// New in B: an entry in an existing group, and a new group with an entry.
	extra := modeltest.Random(seed+9000, modeltest.DefaultOptions())
	var fromExtra []*model.Entry
	extra.WalkEntries(func(_ *model.Group, e *model.Entry) bool { fromExtra = append(fromExtra, e); return len(fromExtra) < 3 })
	// The new entries bring their binaries and icons with them.
	bring := func(e *model.Entry) {
		take := func(e *model.Entry) {
			for _, at := range e.Attachments {
				if _, ok := b.Binaries[at.Binary]; !ok {
					b.Binaries[at.Binary] = &model.Binary{Data: extra.Binaries[at.Binary].Data.Clone()}
				}
			}
			if !e.CustomIcon.IsZero() && !slices.ContainsFunc(b.Meta.CustomIcons, func(ic model.CustomIcon) bool { return ic.UUID == e.CustomIcon }) {
				for _, ic := range extra.Meta.CustomIcons {
					if ic.UUID == e.CustomIcon {
						b.Meta.CustomIcons = append(b.Meta.CustomIcons, ic)
					}
				}
			}
		}
		take(e)
		for _, h := range e.History {
			take(h)
		}
	}
	for _, e := range fromExtra {
		bring(e)
	}
	if len(fromExtra) > 0 {
		b.Root.Entries = append(b.Root.Entries, fromExtra[0])
		if seed%2 == 0 { // A deleted it, before or after B's last change
			a.DeletedObjects = append(a.DeletedObjects, model.DeletedObject{UUID: fromExtra[0].UUID, DeletionTime: fromExtra[0].Times.LastModification.Add(time.Duration(seed%4-1) * time.Hour)})
		}
	}
	if seed%3 == 0 {
		b.Meta.DatabaseName = "Edited in B"
		b.Meta.Color = "#112233"
	}
	if len(fromExtra) > 1 {
		b.Root.Groups = append(b.Root.Groups, &model.Group{UUID: model.UUID{0: 0xee, 15: byte(seed)}, Name: "New in B", Entries: fromExtra[1:]})
	}
	// Entries only in A.
	if len(a.Root.Groups) > 0 && len(b.Root.Groups) > 0 {
		b.Root.Groups = b.Root.Groups[:len(b.Root.Groups)-1]
	}
	return a, b
}

// shape lists the groups and entries of a database as "group-uuid > parent-uuid" and
// "entry-uuid in group-uuid", sorted, so that two trees compare regardless of order.
func shape(db *model.Database) []string {
	var out []string
	var walk func(g, parent *model.Group)
	walk = func(g, parent *model.Group) {
		p := ""
		if parent != nil {
			p = parent.UUID.String()
		}
		out = append(out, "group "+g.UUID.String()+" in "+p)
		for _, e := range g.Entries {
			out = append(out, "entry "+e.UUID.String()+" in "+g.UUID.String())
		}
		for _, c := range g.Groups {
			walk(c, g)
		}
	}
	walk(db.Root, nil)
	sort.Strings(out)
	return out
}

func TestApplyRulesOnRelatedDatabases(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		a, b := related(seed)
		// Rule a: the result has A's groups and entries, each where A has it.
		// Rule b: the same for B.
		for _, tt := range []struct {
			rule plan.Rule
			want *model.Database
		}{{plan.PreferA, a}, {plan.PreferB, b}} {
			p := planFor(t, a, b, nil)
			plan.ApplyRule(p, tt.rule, nil)
			res := mustApply(t, a, b, p)
			if got, want := shape(res), shape(tt.want); !slices.Equal(got, want) {
				t.Fatalf("seed %d, rule %s: the tree differs from the preferred input:\n got %v\nwant %v", seed, tt.rule, got, want)
			}
		}
	}
}

func TestApplyUnionAndNewer(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		a, b := related(seed)
		for _, rule := range []plan.Rule{plan.Newer, plan.Union} {
			p := planFor(t, a, b, nil)
			plan.ApplyRule(p, rule, nil)
			res := mustApply(t, a, b, p)
			// Every UUID of a group or entry in the result is in an input, except the
			// copies that `both` makes (union), and nothing is there twice.
			known := entryUUIDs(a, b)
			seen := map[model.UUID]bool{}
			res.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
				if seen[e.UUID] {
					t.Fatalf("seed %d, rule %s: entry %s twice", seed, rule, e.UUID)
				}
				seen[e.UUID] = true
				if !known[e.UUID] && rule != plan.Union {
					t.Fatalf("seed %d, rule %s: entry %s is new", seed, rule, e.UUID)
				}
				return true
			})
			groups := map[model.UUID]bool{}
			res.WalkGroups(func(g *model.Group) bool {
				if groups[g.UUID] {
					t.Fatalf("seed %d, rule %s: group %s twice", seed, rule, g.UUID)
				}
				groups[g.UUID] = true
				return true
			})
			if rule == plan.Union {
				// Union keeps everything that exists on either side.
				for u := range known {
					if !seen[u] {
						// ... except an entry that the other input deleted and the default drops.
						if !deletedIn(a, b, u) {
							t.Fatalf("seed %d: union lost entry %s", seed, u)
						}
					}
				}
			}
		}
	}
}

func deletedIn(a, b *model.Database, u model.UUID) bool {
	for _, db := range []*model.Database{a, b} {
		for _, d := range db.DeletedObjects {
			if d.UUID == u {
				return true
			}
		}
	}
	return false
}

func TestApplyDoesNotChangeTheInputs(t *testing.T) {
	for seed := uint64(1); seed <= 30; seed++ {
		a, b := related(seed)
		ca, cb := a.Clone(), b.Clone()
		for _, rule := range plan.Rules {
			p := planFor(t, a, b, nil)
			plan.ApplyRule(p, rule, nil)
			res := mustApply(t, a, b, p)
			// Wiping the result must not touch the inputs.
			res.Wipe()
		}
		if d := model.Diff(a, ca); len(d) > 0 {
			t.Fatalf("seed %d: A changed: %v", seed, d)
		}
		if d := model.Diff(b, cb); len(d) > 0 {
			t.Fatalf("seed %d: B changed: %v", seed, d)
		}
	}
}

func TestApplyIsDeterministic(t *testing.T) {
	for seed := uint64(1); seed <= 30; seed++ {
		a, b := related(seed)
		p := planFor(t, a, b, nil)
		plan.ApplyRule(p, plan.Union, nil)
		first := mustApply(t, a, b, p)
		second := mustApply(t, a, b, p)
		if d := model.Diff(first, second); len(d) > 0 {
			t.Fatalf("seed %d: two applications differ: %v", seed, d)
		}
	}
}

func TestBothMakesACopyWithANewUUID(t *testing.T) {
	a := database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1))), 0)
	b := database(grp(1, "Root", grp(2, "Mail"), grp(3, "Work", func() *model.Entry {
		e := ent(11, "X", "b", 2)
		e.History = []*model.Entry{ent(11, "X old", "b0", 1)}
		return e
	}())), 0)
	p := planFor(t, a, b, map[byte]plan.Choice{11: plan.Both})
	res := mustApply(t, a, b, p)
	mail, work := res.Root.Groups[0], res.Root.Groups[1]
	if len(mail.Entries) != 1 || mail.Entries[0].UUID != u(11) || len(work.Entries) != 1 {
		t.Fatalf("tree = %s", render(res))
	}
	cp := work.Entries[0]
	if cp.UUID == u(11) || cp.UUID.IsZero() {
		t.Errorf("the copy has the UUID %s", cp.UUID)
	}
	// It looks like a random (version 4) UUID.
	if cp.UUID[6]>>4 != 4 || cp.UUID[8]>>6 != 2 {
		t.Errorf("the copy's UUID %s is not version 4", cp.UUID)
	}
	if len(cp.History) != 1 || cp.History[0].UUID != cp.UUID {
		t.Errorf("the history of the copy has the UUID %v, want %s", cp.History[0].UUID, cp.UUID)
	}
	// The copy is a copy: wiping it leaves B alone.
	res.Wipe()
	if pw, _ := b.Root.Groups[1].Entries[0].Field(model.FieldPassword); pw.Text() != "b" {
		t.Error("wiping the result wiped B")
	}
	// The same merge gives the same UUID.
	again := mustApply(t, a, b, p)
	if again.Root.Groups[1].Entries[0].UUID != cp.UUID {
		t.Error("the UUID of the copy changes between runs")
	}
}

func TestNewUUIDAvoidsUsedOnes(t *testing.T) {
	a := database(grp(1, "Root", ent(11, "X", "a", 1)), 0)
	bd := newBuilder(a, a)
	first := bd.newUUID(u(11))
	// Mark the next candidate as used: the next call must skip it.
	bd2 := newBuilder(a, a)
	bd2.used[first] = true
	second := bd2.newUUID(u(11))
	if second == first || bd2.used[second] != true {
		t.Errorf("the UUID %s was not avoided: %s", first, second)
	}
}
