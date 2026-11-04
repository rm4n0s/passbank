package diff

import (
	"strings"

	"github.com/rm4n0s/passbank/internal/core/model"
)

// Side is one of the two inputs.
type Side int

// The two inputs.
const (
	// SideA is the primary input.
	SideA Side = iota
	// SideB is the other input.
	SideB
)

// String is "A" or "B".
func (s Side) String() string {
	if s == SideA {
		return "A"
	}
	return "B"
}

// Other returns the other side.
func (s Side) Other() Side { return 1 - s }

// Located is an entry with the group that holds it.
type Located struct {
	Entry *model.Entry
	Group *model.Group
}

// EntryPair is an entry of A and the entry of B with the same UUID.
type EntryPair struct {
	A, B           *model.Entry
	GroupA, GroupB *model.Group
}

// GroupPair is a group of A and the group of B that is the same group: the same
// UUID, the roots, or the recycle bins (SPEC §7).
type GroupPair struct {
	A, B *model.Group
}

// Matching pairs the items of two databases (SPEC §7). Every item of A is in a
// pair or in its OnlyA list, and likewise for B.
type Matching struct {
	Entries []EntryPair
	Groups  []GroupPair

	OnlyAEntries, OnlyBEntries []Located
	OnlyAGroups, OnlyBGroups   []*model.Group
}

// Tree knows where the groups and entries of one database are.
type Tree struct {
	DB     *model.Database
	parent map[*model.Group]*model.Group
	byUUID map[model.UUID]*model.Group
	// entries by UUID, and the group of each entry.
	entries map[model.UUID]Located
}

// NewTree indexes a database.
func NewTree(db *model.Database) *Tree {
	t := &Tree{DB: db, parent: map[*model.Group]*model.Group{}, byUUID: map[model.UUID]*model.Group{}, entries: map[model.UUID]Located{}}
	var walk func(g, parent *model.Group)
	walk = func(g, parent *model.Group) {
		t.parent[g] = parent
		t.byUUID[g.UUID] = g
		for _, e := range g.Entries {
			t.entries[e.UUID] = Located{Entry: e, Group: g}
		}
		for _, c := range g.Groups {
			walk(c, g)
		}
	}
	if db.Root != nil {
		walk(db.Root, nil)
	}
	return t
}

// Parent returns the parent of a group, nil for the root.
func (t *Tree) Parent(g *model.Group) *model.Group { return t.parent[g] }

// Group returns the group with the UUID, or nil.
func (t *Tree) Group(u model.UUID) *model.Group { return t.byUUID[u] }

// Entry returns the entry with the UUID and its group, or false.
func (t *Tree) Entry(u model.UUID) (Located, bool) {
	l, ok := t.entries[u]
	return l, ok
}

// RecycleBin returns the recycle-bin group, or nil when the database has none.
func (t *Tree) RecycleBin() *model.Group {
	if t.DB.Meta.RecycleBinUUID.IsZero() {
		return nil
	}
	return t.byUUID[t.DB.Meta.RecycleBinUUID]
}

// Ancestors returns the UUIDs of the groups above g, root first, g itself last.
func (t *Tree) Ancestors(g *model.Group) []model.UUID {
	var rev []model.UUID
	for ; g != nil; g = t.parent[g] {
		rev = append(rev, g.UUID)
	}
	out := make([]model.UUID, len(rev))
	for i, u := range rev {
		out[len(rev)-1-i] = u
	}
	return out
}

// GroupPath returns the names of the groups above the group and the group itself,
// without the root, joined by " / ". The root's path is empty.
func (t *Tree) GroupPath(g *model.Group) string {
	var names []string
	for ; g != nil && t.parent[g] != nil; g = t.parent[g] {
		names = append(names, g.Name)
	}
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	return strings.Join(names, " / ")
}

// Pairs are the matching of the groups of two trees, with lookups both ways.
type Pairs struct {
	toB map[*model.Group]*model.Group
	toA map[*model.Group]*model.Group
}

// PartnerOf returns the group of the other side that is the same group as g, or nil.
func (p *Pairs) PartnerOf(g *model.Group, side Side) *model.Group {
	if side == SideA {
		return p.toB[g]
	}
	return p.toA[g]
}

// MatchGroups pairs the groups of A and B: by UUID, the two roots, and the two
// recycle bins (when both exist and neither has a partner by UUID).
func MatchGroups(a, b *Tree) *Pairs {
	p := &Pairs{toB: map[*model.Group]*model.Group{}, toA: map[*model.Group]*model.Group{}}
	pair := func(ga, gb *model.Group) {
		p.toB[ga], p.toA[gb] = gb, ga
	}
	for u, ga := range a.byUUID {
		if gb, ok := b.byUUID[u]; ok {
			pair(ga, gb)
		}
	}
	if a.DB.Root != nil && b.DB.Root != nil {
		_, aDone := p.toB[a.DB.Root]
		_, bDone := p.toA[b.DB.Root]
		if !aDone && !bDone {
			pair(a.DB.Root, b.DB.Root)
		}
	}
	binA, binB := a.RecycleBin(), b.RecycleBin()
	if binA != nil && binB != nil && p.toB[binA] == nil && p.toA[binB] == nil {
		pair(binA, binB)
	}
	return p
}

// Match pairs the items of A and B (SPEC §7): entries by UUID wherever they are in
// the tree; groups by UUID, plus the two roots and the two recycle bins. The lists
// are in the order of the trees (A's tree, then B's).
func Match(a, b *model.Database) Matching {
	ta, tb := NewTree(a), NewTree(b)
	return match(ta, tb, MatchGroups(ta, tb))
}

func match(ta, tb *Tree, pairs *Pairs) Matching {
	var m Matching
	walkGroups(ta.DB, func(g *model.Group) {
		if partner := pairs.toB[g]; partner != nil {
			m.Groups = append(m.Groups, GroupPair{A: g, B: partner})
		} else {
			m.OnlyAGroups = append(m.OnlyAGroups, g)
		}
		for _, e := range g.Entries {
			if lb, ok := tb.entries[e.UUID]; ok {
				m.Entries = append(m.Entries, EntryPair{A: e, B: lb.Entry, GroupA: g, GroupB: lb.Group})
			} else {
				m.OnlyAEntries = append(m.OnlyAEntries, Located{e, g})
			}
		}
	})
	walkGroups(tb.DB, func(g *model.Group) {
		if pairs.toA[g] == nil {
			m.OnlyBGroups = append(m.OnlyBGroups, g)
		}
		for _, e := range g.Entries {
			if _, ok := ta.entries[e.UUID]; !ok {
				m.OnlyBEntries = append(m.OnlyBEntries, Located{e, g})
			}
		}
	})
	return m
}

// walkGroups visits the root and every group below it, parents first.
func walkGroups(db *model.Database, fn func(*model.Group)) {
	db.WalkGroups(func(g *model.Group) bool { fn(g); return true })
}
