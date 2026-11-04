package merge

import (
	"slices"
	"sort"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

// mergeHistories gives every entry that both inputs hold the union of their
// histories (SPEC §10.2): the versions of both, without duplicates (the same
// last modification time and the same content), oldest first. When the plan took
// one version of a pair that differs, the version it did not take is added to the
// history as well, so that the merge can be undone in any KeePass client. An
// entry that `both` keeps twice keeps the history it came with. The history is
// not trimmed.
func (bd *builder) mergeHistories(choiceOf map[model.UUID]plan.Choice) {
	inResult := map[model.UUID]*model.Entry{}
	bd.res.WalkEntries(func(_ *model.Group, e *model.Entry) bool { inResult[e.UUID] = e; return true })
	for _, pair := range diff.Match(bd.a, bd.b).Entries {
		res, ok := inResult[pair.A.UUID]
		if !ok {
			continue // dropped
		}
		choice, changed := choiceOf[pair.A.UUID]
		if changed && choice == plan.Both {
			continue
		}
		var other *model.Entry // the version the plan did not take
		if len(diff.EntryDifferences(pair.A, pair.B)) > 0 {
			switch choice {
			case plan.TakeA:
				other = pair.B
			case plan.TakeB:
				other = pair.A
			}
		}
		old := res.History
		res.History = unionHistory(res, pair.A.History, pair.B.History, other)
		for _, h := range old {
			h.Wipe() // the result's own copies, replaced by the union
		}
	}
}

// unionHistory returns the history of the entry in the result: the versions of
// both histories and the other version, if any, without duplicates, ordered by
// last modification time (versions with the same time keep A's before B's).
func unionHistory(res *model.Entry, a, b []*model.Entry, other *model.Entry) []*model.Entry {
	var out []*model.Entry
	add := func(v *model.Entry) {
		for _, have := range out {
			if have.Times.LastModification.Equal(v.Times.LastModification) && model.SameVersion(have, v) {
				return
			}
		}
		c := v.Clone()
		c.History = nil
		c.UUID = res.UUID
		out = append(out, c)
	}
	for _, v := range a {
		add(v)
	}
	for _, v := range b {
		add(v)
	}
	if other != nil {
		add(other)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Times.LastModification.Before(out[j].Times.LastModification)
	})
	return out
}

// mergeMeta sets the Meta of the result (SPEC §10.4). The result's Meta starts as
// A's; with the settings taken from B, B's settings replace A's. The custom icons
// are the union by UUID and the custom data the union by key. References to
// groups that are not in the result are cleared.
func (bd *builder) mergeMeta(settingsFromB bool) {
	m := &bd.res.Meta
	bm := &bd.b.Meta
	if settingsFromB {
		m.DatabaseName, m.DatabaseNameChanged = bm.DatabaseName, bm.DatabaseNameChanged
		m.Description, m.DescriptionChanged = bm.Description, bm.DescriptionChanged
		m.DefaultUserName, m.DefaultUserNameChanged = bm.DefaultUserName, bm.DefaultUserNameChanged
		m.Color = bm.Color
		m.MaintenanceHistoryDays = bm.MaintenanceHistoryDays
		m.HistoryMaxItems, m.HistoryMaxSize = bm.HistoryMaxItems, bm.HistoryMaxSize
		m.RecycleBinEnabled, m.RecycleBinChanged = bm.RecycleBinEnabled, bm.RecycleBinChanged
		m.MemoryProtection = bm.MemoryProtection
		m.SettingsChanged = bm.SettingsChanged
	}
	m.CustomIcons = unionIcons(m.CustomIcons, bm.CustomIcons)
	m.CustomData = unionCustomData(m.CustomData, bm.CustomData, settingsFromB)

	// When A has no recycle bin and B's was kept, it becomes the bin of the result.
	if m.RecycleBinUUID.IsZero() || !bd.hasGroup(m.RecycleBinUUID) {
		if binB := bd.tb.RecycleBin(); binB != nil {
			if g, err := bd.groupOfB(binB); err == nil {
				m.RecycleBinUUID = g.UUID
			}
		}
	}
	for _, ref := range []*model.UUID{&m.RecycleBinUUID, &m.EntryTemplatesGroup, &m.LastSelectedGroup, &m.LastTopVisibleGroup} {
		if !ref.IsZero() && !bd.hasGroup(*ref) {
			*ref = model.UUID{}
		}
	}
}

// hasGroup reports whether the result still has the group (dropped groups are
// removed from the tree but not from the index of UUIDs).
func (bd *builder) hasGroup(u model.UUID) bool {
	g := bd.groups[u]
	if g == nil {
		return false
	}
	for p := g; p != nil; p = bd.parent[p] {
		if p == bd.res.Root {
			return true
		}
	}
	return false
}

// unionIcons returns A's custom icons and then those of B that A does not have.
// An icon both have with different data comes from B when B's was modified later
// (KDBX 4.1 records the time), else from A.
func unionIcons(a, b []model.CustomIcon) []model.CustomIcon {
	out := slices.Clone(a)
	for _, ib := range b {
		i := slices.IndexFunc(out, func(x model.CustomIcon) bool { return x.UUID == ib.UUID })
		switch {
		case i < 0:
			ib.Data = slices.Clone(ib.Data)
			out = append(out, ib)
		case !slices.Equal(out[i].Data, ib.Data) && ib.LastModified.After(out[i].LastModified):
			ib.Data = slices.Clone(ib.Data)
			out[i] = ib
		}
	}
	return out
}

// unionCustomData returns A's custom data and then B's keys that A does not have.
// A key both have with different values takes B's when fromB is set, else A's.
func unionCustomData(a, b []model.CustomDataItem, fromB bool) []model.CustomDataItem {
	out := slices.Clone(a)
	for _, ib := range b {
		i := slices.IndexFunc(out, func(x model.CustomDataItem) bool { return x.Key == ib.Key })
		switch {
		case i < 0:
			out = append(out, ib)
		case fromB && out[i].Value != ib.Value:
			out[i] = ib
		}
	}
	return out
}

// mergeDeleted sets the deleted objects of the result (SPEC §10.3): the union of
// both lists, minus every UUID the result contains. A UUID in both lists keeps
// the later deletion time.
func (bd *builder) mergeDeleted() {
	present := map[model.UUID]bool{}
	bd.res.WalkGroups(func(g *model.Group) bool {
		present[g.UUID] = true
		for _, e := range g.Entries {
			present[e.UUID] = true
		}
		return true
	})
	var out []model.DeletedObject
	index := map[model.UUID]int{}
	for _, list := range [][]model.DeletedObject{bd.a.DeletedObjects, bd.b.DeletedObjects} {
		for _, d := range list {
			if present[d.UUID] {
				continue
			}
			if i, ok := index[d.UUID]; ok {
				if d.DeletionTime.After(out[i].DeletionTime) {
					out[i].DeletionTime = d.DeletionTime
				}
				continue
			}
			index[d.UUID] = len(out)
			out = append(out, d)
		}
	}
	bd.res.DeletedObjects = out
}

// rebuildPool makes the attachment pool of the result hold exactly the binaries
// that the entries and their histories reference (SPEC §10.5), each once: from A's
// pool when A has it, else from B's.
func (bd *builder) rebuildPool() {
	pool := map[model.BinaryKey]*model.Binary{}
	take := func(k model.BinaryKey) {
		if _, ok := pool[k]; ok {
			return
		}
		if bin, ok := bd.res.Binaries[k]; ok {
			pool[k] = bin
		} else if bin, ok := bd.b.Binaries[k]; ok {
			pool[k] = &model.Binary{Data: bin.Data.Clone()}
		}
	}
	bd.res.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
		for _, at := range e.Attachments {
			take(at.Binary)
		}
		for _, h := range e.History {
			for _, at := range h.Attachments {
				take(at.Binary)
			}
		}
		return true
	})
	// The binaries that are left out were cloned with the result: wipe them.
	for k, bin := range bd.res.Binaries {
		if pool[k] != bin {
			bin.Data.Wipe()
		}
	}
	bd.res.Binaries = pool
}
