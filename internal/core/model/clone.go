package model

import "slices"

// Clone returns a deep copy of the database: changing the copy never changes the
// original, and wiping the copy's secrets never wipes the original's.
func (d *Database) Clone() *Database {
	if d == nil {
		return nil
	}
	c := &Database{
		Meta:             d.Meta.clone(),
		Root:             d.Root.clone(),
		DeletedObjects:   slices.Clone(d.DeletedObjects),
		PublicCustomData: make([]PublicCustomDataItem, len(d.PublicCustomData)),
		Raw:              d.Raw.clone(),
	}
	for i, p := range d.PublicCustomData {
		p.Data = slices.Clone(p.Data)
		c.PublicCustomData[i] = p
	}
	if d.PublicCustomData == nil {
		c.PublicCustomData = nil
	}
	if d.Binaries != nil {
		c.Binaries = make(map[BinaryKey]*Binary, len(d.Binaries))
		for k, b := range d.Binaries {
			c.Binaries[k] = &Binary{Data: b.Data.Clone()}
		}
	}
	return c
}

func (m Meta) clone() Meta {
	m.CustomIcons = slices.Clone(m.CustomIcons)
	for i := range m.CustomIcons {
		m.CustomIcons[i].Data = slices.Clone(m.CustomIcons[i].Data)
	}
	m.CustomData = slices.Clone(m.CustomData)
	m.Raw = m.Raw.clone()
	return m
}

func (g *Group) clone() *Group {
	if g == nil {
		return nil
	}
	c := *g
	c.Tags = slices.Clone(g.Tags)
	c.CustomData = slices.Clone(g.CustomData)
	c.Raw = g.Raw.clone()
	if g.Groups != nil {
		c.Groups = make([]*Group, len(g.Groups))
		for i, child := range g.Groups {
			c.Groups[i] = child.clone()
		}
	}
	if g.Entries != nil {
		c.Entries = make([]*Entry, len(g.Entries))
		for i, e := range g.Entries {
			c.Entries[i] = e.clone()
		}
	}
	return &c
}

func (e *Entry) clone() *Entry {
	if e == nil {
		return nil
	}
	c := *e
	c.Tags = slices.Clone(e.Tags)
	if e.QualityCheck != nil {
		q := *e.QualityCheck
		c.QualityCheck = &q
	}
	if e.Fields != nil {
		c.Fields = make([]Field, len(e.Fields))
		for i, f := range e.Fields {
			f.Value = f.Value.Clone()
			c.Fields[i] = f
		}
	}
	c.Attachments = slices.Clone(e.Attachments)
	c.AutoType.Associations = slices.Clone(e.AutoType.Associations)
	c.CustomData = slices.Clone(e.CustomData)
	c.Raw = e.Raw.clone()
	if e.History != nil {
		c.History = make([]*Entry, len(e.History))
		for i, h := range e.History {
			c.History[i] = h.clone()
		}
	}
	return &c
}

func (r Raw) clone() Raw {
	r.Attrs = slices.Clone(r.Attrs)
	if r.Nodes != nil {
		nodes := make([]*RawNode, len(r.Nodes))
		for i, n := range r.Nodes {
			nodes[i] = n.clone()
		}
		r.Nodes = nodes
	}
	return r
}

func (n *RawNode) clone() *RawNode {
	if n == nil {
		return nil
	}
	c := *n
	c.Attrs = slices.Clone(n.Attrs)
	c.Text = n.Text.Clone()
	if n.Children != nil {
		c.Children = make([]*RawNode, len(n.Children))
		for i, ch := range n.Children {
			c.Children[i] = ch.clone()
		}
	}
	return &c
}

// Wipe overwrites every secret of the database with zeros: field values,
// attachment contents and the text of raw nodes. The database must not be used
// afterwards.
func (d *Database) Wipe() {
	if d == nil {
		return
	}
	for _, b := range d.Binaries {
		b.Data.Wipe()
	}
	wipeRaw(d.Raw)
	wipeRaw(d.Meta.Raw)
	d.WalkGroups(func(g *Group) bool {
		wipeRaw(g.Raw)
		for _, e := range g.Entries {
			e.wipe()
		}
		return true
	})
}

func (e *Entry) wipe() {
	for _, f := range e.Fields {
		f.Value.Wipe()
	}
	wipeRaw(e.Raw)
	for _, h := range e.History {
		h.wipe()
	}
}

func wipeRaw(r Raw) {
	var wipe func(*RawNode)
	wipe = func(n *RawNode) {
		n.Text.Wipe()
		for _, c := range n.Children {
			wipe(c)
		}
	}
	for _, n := range r.Nodes {
		wipe(n)
	}
}
