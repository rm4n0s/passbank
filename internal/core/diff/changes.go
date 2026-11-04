package diff

import (
	"sort"
	"time"

	"github.com/rm4n0s/passbank/internal/core/model"
)

// Kind is the kind of a change (SPEC §8). The order of the constants is the
// order in which changes are listed.
type Kind int

// The kinds of change.
const (
	// OnlyA is an entry or group only in A.
	OnlyA Kind = iota
	// OnlyB is an entry or group only in B.
	OnlyB
	// Differs is a pair of entries that are not equal.
	Differs
	// DeletedInA is an entry of B whose UUID is in A's deleted objects.
	DeletedInA
	// DeletedInB is an entry of A whose UUID is in B's deleted objects.
	DeletedInB
	// GroupDiffers is a pair of groups whose own properties differ.
	GroupDiffers
	// Settings is the databases' Meta settings differing.
	Settings
)

// Kinds lists the kinds in listing order.
var Kinds = []Kind{OnlyA, OnlyB, Differs, DeletedInA, DeletedInB, GroupDiffers, Settings}

var kindNames = [...]string{"only-a", "only-b", "differs", "deleted-in-a", "deleted-in-b", "group-differs", "settings"}

// String is the name of the kind as the plan file spells it.
func (k Kind) String() string {
	if k < 0 || int(k) >= len(kindNames) {
		return "unknown"
	}
	return kindNames[k]
}

// ParseKind returns the kind with the given name.
func ParseKind(s string) (Kind, bool) {
	for i, n := range kindNames {
		if n == s {
			return Kind(i), true
		}
	}
	return 0, false
}

// Item says what a change is about.
type Item int

// The items a change can be about.
const (
	// ItemEntry is an entry.
	ItemEntry Item = iota
	// ItemGroup is a group.
	ItemGroup
	// ItemSettings is the Meta settings of the databases.
	ItemSettings
)

// Change is one difference between A and B that the user decides about.
type Change struct {
	Kind Kind
	Item Item
	// UUID is the UUID of the entry or group; zero for Settings.
	UUID model.UUID
	// Entry holds the entry on each side, nil where it does not exist. Parent holds
	// the group that holds it on that side. Both are nil for groups and Settings.
	Entry  [2]*model.Entry
	Parent [2]*model.Group
	// Group holds the group on each side, for group changes; nil otherwise.
	Group [2]*model.Group
	// Fields names what differs (Differs, GroupDiffers, Settings); never values.
	Fields []string
	// Modified is the last modification time on each side; zero where the item does
	// not exist. For Settings it is the time the settings changed.
	Modified [2]time.Time
	// Deleted is the deletion time, for DeletedInA and DeletedInB.
	Deleted time.Time
	// Path is the path of the group that holds the item, without the root, and Title
	// its title (for a group, its name). They are for display and sorting.
	Path, Title string
}

// FieldList is the names of the differing fields, joined for display.
func (c Change) FieldList() string { return joinNames(c.Fields) }

// Changes compares A and B and lists everything the user decides about, in the
// order of SPEC §8: by kind, then group path, then title, then UUID. Equal pairs
// are not listed. It never changes a or b.
func Changes(a, b *model.Database) []Change {
	ta, tb := NewTree(a), NewTree(b)
	pairs := MatchGroups(ta, tb)
	m := match(ta, tb, pairs)
	delA, delB := deletions(a), deletions(b)

	var out []Change
	for _, l := range m.OnlyAEntries {
		c := Change{Kind: OnlyA, Item: ItemEntry, UUID: l.Entry.UUID}
		c.Entry[SideA], c.Parent[SideA] = l.Entry, l.Group
		c.Modified[SideA] = l.Entry.Times.LastModification
		if t, ok := delB[l.Entry.UUID]; ok {
			c.Kind, c.Deleted = DeletedInB, t
		}
		out = append(out, c)
	}
	for _, l := range m.OnlyBEntries {
		c := Change{Kind: OnlyB, Item: ItemEntry, UUID: l.Entry.UUID}
		c.Entry[SideB], c.Parent[SideB] = l.Entry, l.Group
		c.Modified[SideB] = l.Entry.Times.LastModification
		if t, ok := delA[l.Entry.UUID]; ok {
			c.Kind, c.Deleted = DeletedInA, t
		}
		out = append(out, c)
	}
	for _, g := range m.OnlyAGroups {
		c := Change{Kind: OnlyA, Item: ItemGroup, UUID: g.UUID}
		c.Group[SideA] = g
		c.Modified[SideA] = g.Times.LastModification
		out = append(out, c)
	}
	for _, g := range m.OnlyBGroups {
		c := Change{Kind: OnlyB, Item: ItemGroup, UUID: g.UUID}
		c.Group[SideB] = g
		c.Modified[SideB] = g.Times.LastModification
		out = append(out, c)
	}
	for _, p := range m.Entries {
		fields := EntryDifferences(p.A, p.B)
		if pairs.PartnerOf(p.GroupA, SideA) != p.GroupB {
			fields = append(fields, FieldGroup)
		}
		if len(fields) == 0 {
			continue
		}
		c := Change{Kind: Differs, Item: ItemEntry, UUID: p.A.UUID, Fields: fields}
		c.Entry = [2]*model.Entry{p.A, p.B}
		c.Parent = [2]*model.Group{p.GroupA, p.GroupB}
		c.Modified = [2]time.Time{p.A.Times.LastModification, p.B.Times.LastModification}
		out = append(out, c)
	}
	for _, p := range m.Groups {
		fields := GroupDifferences(p.A, p.B)
		if !sameParent(ta, tb, pairs, p) {
			fields = append(fields, FieldParent)
		}
		if len(fields) == 0 {
			continue
		}
		c := Change{Kind: GroupDiffers, Item: ItemGroup, UUID: p.A.UUID, Fields: fields}
		c.Group = [2]*model.Group{p.A, p.B}
		c.Modified = [2]time.Time{p.A.Times.LastModification, p.B.Times.LastModification}
		out = append(out, c)
	}
	if fields := SettingsDifferences(&a.Meta, &b.Meta); len(fields) > 0 {
		out = append(out, Change{Kind: Settings, Item: ItemSettings, Fields: fields,
			Modified: [2]time.Time{a.Meta.SettingsChanged, b.Meta.SettingsChanged}})
	}

	for i := range out {
		out[i].Path, out[i].Title = display(ta, tb, &out[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		x, y := &out[i], &out[j]
		if x.Kind != y.Kind {
			return x.Kind < y.Kind
		}
		if x.Path != y.Path {
			return x.Path < y.Path
		}
		if x.Title != y.Title {
			return x.Title < y.Title
		}
		return x.UUID.String() < y.UUID.String()
	})
	return out
}

// deletions indexes the deleted objects of a database by UUID, keeping the
// newest deletion time of each.
func deletions(db *model.Database) map[model.UUID]time.Time {
	out := make(map[model.UUID]time.Time, len(db.DeletedObjects))
	for _, d := range db.DeletedObjects {
		if t, ok := out[d.UUID]; !ok || d.DeletionTime.After(t) {
			out[d.UUID] = d.DeletionTime
		}
	}
	return out
}

// sameParent reports whether the two groups of a pair sit in groups that are a
// pair themselves (both roots count as having the same, absent, parent).
func sameParent(ta, tb *Tree, pairs *Pairs, p GroupPair) bool {
	pa, pb := ta.Parent(p.A), tb.Parent(p.B)
	if pa == nil || pb == nil {
		return pa == nil && pb == nil
	}
	return pairs.PartnerOf(pa, SideA) == pb
}

// display finds the path and title of a change: from A when A has the item,
// else from B.
func display(ta, tb *Tree, c *Change) (path, title string) {
	trees := [2]*Tree{ta, tb}
	for s := SideA; s <= SideB; s++ {
		switch c.Item {
		case ItemEntry:
			if c.Entry[s] != nil {
				return trees[s].GroupPath(c.Parent[s]), c.Entry[s].Title()
			}
		case ItemGroup:
			if g := c.Group[s]; g != nil {
				return trees[s].GroupPath(trees[s].Parent(g)), g.Name
			}
		}
	}
	return "", ""
}
