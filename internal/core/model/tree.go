package model

import "strings"

// WalkGroups calls fn for the root group and every group below it, parents
// before children, in file order. fn returns false to stop the walk.
func (d *Database) WalkGroups(fn func(g *Group) bool) {
	if d.Root != nil {
		d.Root.walkGroups(fn)
	}
}

func (g *Group) walkGroups(fn func(*Group) bool) bool {
	if !fn(g) {
		return false
	}
	for _, c := range g.Groups {
		if !c.walkGroups(fn) {
			return false
		}
	}
	return true
}

// WalkEntries calls fn for every entry of the database with the group that holds
// it, in file order (the entries of a group before its subgroups). The history of
// an entry is not walked. fn returns false to stop the walk.
func (d *Database) WalkEntries(fn func(g *Group, e *Entry) bool) {
	d.WalkGroups(func(g *Group) bool {
		for _, e := range g.Entries {
			if !fn(g, e) {
				return false
			}
		}
		return true
	})
}

// FindGroup returns the group with the UUID and its parent (nil for the root), or
// false.
func (d *Database) FindGroup(u UUID) (g, parent *Group, ok bool) {
	var walk func(cur, par *Group) bool
	walk = func(cur, par *Group) bool {
		if cur.UUID == u {
			g, parent, ok = cur, par, true
			return true
		}
		for _, c := range cur.Groups {
			if walk(c, cur) {
				return true
			}
		}
		return false
	}
	if d.Root != nil {
		walk(d.Root, nil)
	}
	return g, parent, ok
}

// FindEntry returns the entry with the UUID and the group that holds it, or false.
func (d *Database) FindEntry(u UUID) (e *Entry, parent *Group, ok bool) {
	d.WalkEntries(func(g *Group, cur *Entry) bool {
		if cur.UUID == u {
			e, parent, ok = cur, g, true
			return false
		}
		return true
	})
	return e, parent, ok
}

// GroupPath returns the names of the groups from the root down to the group with
// the UUID, joined by "/", or false when there is no such group. The root's name
// is part of the path.
func (d *Database) GroupPath(u UUID) (string, bool) {
	var names []string
	var walk func(g *Group) bool
	walk = func(g *Group) bool {
		names = append(names, g.Name)
		if g.UUID == u {
			return true
		}
		for _, c := range g.Groups {
			if walk(c) {
				return true
			}
		}
		names = names[:len(names)-1]
		return false
	}
	if d.Root == nil || !walk(d.Root) {
		return "", false
	}
	return strings.Join(names, "/"), true
}
