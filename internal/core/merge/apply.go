package merge

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

// Apply builds the merged database from A, B and a plan made for them (SPEC §10).
// It is a pure function: the same inputs and plan always give the same result,
// and a and b are not changed. The result shares nothing with them, so wiping it
// never wipes an input.
//
// The plan must be valid (see Validate): a group that is dropped holds nothing
// that is kept.
func Apply(a, b *model.Database, p *plan.Plan) (*model.Database, error) {
	return apply(a, b, p)
}

// apply is Apply without the validation, so that a test can give it a plan that
// Validate would refuse.
func apply(a, b *model.Database, p *plan.Plan) (*model.Database, error) {
	changes := diff.Changes(a, b)
	if err := sameChanges(changes, p); err != nil {
		return nil, err
	}
	bd := newBuilder(a, b)
	if err := bd.run(changes, p.Choices); err != nil {
		bd.res.Wipe()
		return nil, err
	}
	return bd.res, nil
}

// sameChanges checks that the plan was made for the changes between these two
// databases: the same changes in the same order, each with a choice.
func sameChanges(changes []diff.Change, p *plan.Plan) error {
	if len(p.Changes) != len(changes) || len(p.Choices) != len(changes) {
		return errors.New("PlanMismatch", "the plan was not made for these databases",
			"changes", len(changes), "plan changes", len(p.Changes), "plan choices", len(p.Choices))
	}
	for i, c := range changes {
		pc := p.Changes[i]
		if pc.Kind != c.Kind || pc.Item != c.Item || pc.UUID != c.UUID {
			return errors.New("PlanMismatch", "the plan was not made for these databases", "index", i)
		}
	}
	return nil
}

// builder holds the result while the choices are applied to it.
type builder struct {
	a, b   *model.Database
	ta, tb *diff.Tree
	pairs  *diff.Pairs
	res    *model.Database

	// The groups of the result by UUID, and the parent of each.
	groups map[model.UUID]*model.Group
	parent map[*model.Group]*model.Group
	// The group of the result that holds each entry.
	entryIn map[model.UUID]*model.Group
	// The groups of B that exist only there, and were created in the result.
	created map[*model.Group]*model.Group
	// used holds every UUID of the inputs and the result, for the new UUIDs of
	// the copies that `both` makes.
	used map[model.UUID]bool
}

func newBuilder(a, b *model.Database) *builder {
	bd := &builder{
		a: a, b: b, ta: diff.NewTree(a), tb: diff.NewTree(b),
		res:     a.Clone(),
		groups:  map[model.UUID]*model.Group{},
		parent:  map[*model.Group]*model.Group{},
		entryIn: map[model.UUID]*model.Group{},
		created: map[*model.Group]*model.Group{},
		used:    map[model.UUID]bool{},
	}
	bd.pairs = diff.MatchGroups(bd.ta, bd.tb)
	var walk func(g, parent *model.Group)
	walk = func(g, parent *model.Group) {
		bd.groups[g.UUID] = g
		bd.parent[g] = parent
		for _, e := range g.Entries {
			bd.entryIn[e.UUID] = g
		}
		for _, c := range g.Groups {
			walk(c, g)
		}
	}
	if bd.res.Root != nil {
		walk(bd.res.Root, nil)
	}
	for _, db := range []*model.Database{a, b} {
		db.WalkGroups(func(g *model.Group) bool {
			bd.used[g.UUID] = true
			for _, e := range g.Entries {
				bd.used[e.UUID] = true
			}
			return true
		})
		for _, d := range db.DeletedObjects {
			bd.used[d.UUID] = true
		}
	}
	return bd
}

// run applies every choice: first the groups that only B has, then the changes
// to the groups that both have, then the entries, and last the groups that are
// dropped.
func (bd *builder) run(changes []diff.Change, choices []plan.Choice) error {
	choiceOf := make(map[model.UUID]plan.Choice, len(changes))
	settingsFromB := false
	for i, c := range changes {
		if c.Item == diff.ItemSettings {
			settingsFromB = choices[i] == plan.TakeB
		} else {
			choiceOf[c.UUID] = choices[i]
		}
	}
	if err := bd.createGroups(choiceOf); err != nil {
		return err
	}
	for i, c := range changes {
		if c.Item == diff.ItemGroup && c.Kind == diff.GroupDiffers && choices[i] == plan.TakeB {
			if err := bd.takeGroupB(c); err != nil {
				return err
			}
		}
	}
	for i, c := range changes {
		if c.Item != diff.ItemEntry {
			continue
		}
		if err := bd.entry(c, choices[i]); err != nil {
			return err
		}
	}
	for i, c := range changes {
		if c.Item == diff.ItemGroup && c.Kind == diff.OnlyA && choices[i] == plan.Drop {
			bd.removeGroup(c.Group[diff.SideA].UUID)
		}
	}
	bd.mergeHistories(choiceOf)
	bd.mergeMeta(settingsFromB)
	bd.mergeDeleted()
	bd.rebuildPool()
	return nil
}

// createGroups creates, below their parents, the groups that only B has and the
// plan keeps. B's tree is walked parents first, so a parent exists before its
// children.
func (bd *builder) createGroups(choiceOf map[model.UUID]plan.Choice) error {
	var err error
	bd.b.WalkGroups(func(g *model.Group) bool {
		if bd.pairs.PartnerOf(g, diff.SideB) != nil || choiceOf[g.UUID] != plan.Keep {
			return true
		}
		parent, perr := bd.groupOfB(bd.tb.Parent(g))
		if perr != nil {
			err = perr
			return false
		}
		c := groupOnly(g)
		parent.Groups = append(parent.Groups, c)
		bd.created[g] = c
		bd.groups[c.UUID] = c
		bd.parent[c] = parent
		return true
	})
	return err
}

// groupOfB returns the group of the result that stands for a group of B: A's
// partner of the group, or the group that was created for it. A group that is
// not in the result is PlanGroupNotEmpty: something kept needs a group the plan
// drops.
func (bd *builder) groupOfB(g *model.Group) (*model.Group, error) {
	if g == nil {
		return bd.res.Root, nil
	}
	if partner := bd.pairs.PartnerOf(g, diff.SideB); partner != nil {
		return bd.groups[partner.UUID], nil
	}
	if c, ok := bd.created[g]; ok {
		return c, nil
	}
	return nil, errors.New("PlanGroupNotEmpty", "something that is kept needs a group that the plan drops", "group", g.UUID.String())
}

// groupOnly returns a copy of a group with its own properties and without its
// subgroups and entries.
func groupOnly(g *model.Group) *model.Group {
	tmp := *g
	tmp.Groups, tmp.Entries = nil, nil
	return tmp.Clone()
}

// takeGroupB gives the group of the result B's properties and, when the parent
// differs, B's parent.
func (bd *builder) takeGroupB(c diff.Change) error {
	gb := c.Group[diff.SideB]
	dst, err := bd.groupOfB(gb)
	if err != nil {
		return err
	}
	// dst is the result's group for gb: the partner of gb.
	props := groupOnly(gb)
	props.UUID = dst.UUID
	props.Groups, props.Entries = dst.Groups, dst.Entries
	*dst = *props
	if !slices.Contains(c.Fields, diff.FieldParent) {
		return nil
	}
	newParent, err := bd.groupOfB(bd.tb.Parent(gb))
	if err != nil {
		return err
	}
	return bd.moveGroup(dst, newParent)
}

// moveGroup moves a group of the result below another group; a move below the
// group itself or one of its descendants is MergeGroupCycle.
func (bd *builder) moveGroup(g, newParent *model.Group) error {
	for p := newParent; p != nil; p = bd.parent[p] {
		if p == g {
			return errors.New("MergeGroupCycle", "the choices would put a group inside itself", "group", g.UUID.String())
		}
	}
	old := bd.parent[g]
	if old == newParent {
		return nil
	}
	if old != nil {
		old.Groups = slices.DeleteFunc(old.Groups, func(x *model.Group) bool { return x == g })
	}
	newParent.Groups = append(newParent.Groups, g)
	bd.parent[g] = newParent
	return nil
}

// removeGroup removes a group of the result with everything in it.
func (bd *builder) removeGroup(u model.UUID) {
	g := bd.groups[u]
	if g == nil {
		return
	}
	if p := bd.parent[g]; p != nil {
		p.Groups = slices.DeleteFunc(p.Groups, func(x *model.Group) bool { return x == g })
	}
	delete(bd.groups, u)
	delete(bd.parent, g) // the group and what is in it are no longer below the root
	g.Wipe()
}

// entry applies the choice for one entry change.
func (bd *builder) entry(c diff.Change, choice plan.Choice) error {
	uuid := c.UUID
	switch c.Kind {
	case diff.OnlyA, diff.DeletedInB:
		if choice == plan.Drop {
			bd.removeEntry(uuid)
		}
	case diff.OnlyB, diff.DeletedInA:
		if choice == plan.Keep {
			_, err := bd.addFromB(c.Entry[diff.SideB], c.Parent[diff.SideB], uuid)
			return err
		}
	case diff.Differs:
		switch choice {
		case plan.Drop:
			bd.removeEntry(uuid)
		case plan.TakeB:
			return bd.replaceWithB(c)
		case plan.Both:
			copyUUID := bd.newUUID(uuid)
			_, err := bd.addFromB(c.Entry[diff.SideB], c.Parent[diff.SideB], copyUUID)
			return err
		}
	}
	return nil
}

// addFromB puts a copy of B's entry, with the given UUID, in the result's group
// for B's group, and returns the copy.
func (bd *builder) addFromB(e *model.Entry, parentB *model.Group, uuid model.UUID) (*model.Entry, error) {
	dst, err := bd.groupOfB(parentB)
	if err != nil {
		return nil, err
	}
	cp := e.Clone()
	setUUID(cp, uuid)
	dst.Entries = append(dst.Entries, cp)
	bd.entryIn[uuid] = dst
	return cp, nil
}

// replaceWithB puts B's version of an entry in place of A's: in the same place
// when the group is the same, else appended to B's group.
func (bd *builder) replaceWithB(c diff.Change) error {
	uuid := c.UUID
	dst, err := bd.groupOfB(c.Parent[diff.SideB])
	if err != nil {
		return err
	}
	cur := bd.entryIn[uuid]
	cp := c.Entry[diff.SideB].Clone()
	if cur == dst {
		i := slices.IndexFunc(cur.Entries, func(e *model.Entry) bool { return e.UUID == uuid })
		if i >= 0 {
			cur.Entries[i].Wipe()
			cur.Entries[i] = cp
			return nil
		}
	}
	bd.removeEntry(uuid)
	dst.Entries = append(dst.Entries, cp)
	bd.entryIn[uuid] = dst
	return nil
}

// removeEntry removes an entry from the result.
func (bd *builder) removeEntry(u model.UUID) {
	g := bd.entryIn[u]
	if g == nil {
		return
	}
	g.Entries = slices.DeleteFunc(g.Entries, func(e *model.Entry) bool {
		if e.UUID != u {
			return false
		}
		e.Wipe() // the result's own copy
		return true
	})
	delete(bd.entryIn, u)
}

// setUUID gives an entry and the versions in its history a UUID.
func setUUID(e *model.Entry, u model.UUID) {
	e.UUID = u
	for _, h := range e.History {
		h.UUID = u
	}
}

// newUUID makes the UUID of the copy that `both` keeps, from the UUID of the
// entry, so that the same merge always gives the same result. It looks like a
// version 4 UUID and is not any UUID of the inputs or the result so far.
func (bd *builder) newUUID(of model.UUID) model.UUID {
	for n := uint32(0); ; n++ {
		h := sha256.New()
		h.Write([]byte("passbank-both"))
		h.Write(of[:])
		var count [4]byte
		binary.BigEndian.PutUint32(count[:], n)
		h.Write(count[:])
		var u model.UUID
		copy(u[:], h.Sum(nil))
		u[6] = u[6]&0x0f | 0x40
		u[8] = u[8]&0x3f | 0x80
		if !bd.used[u] {
			bd.used[u] = true
			return u
		}
	}
}
