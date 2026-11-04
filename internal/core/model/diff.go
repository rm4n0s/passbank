package model

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// Diff compares two databases deeply and returns one line for each difference:
// the path of the item and what differs, such as
//
//	group 3f1c…/entry 7d0e…: Fields/Password differs
//
// It returns nothing when the databases are equal. Values are compared, never
// printed: the lines name properties, UUIDs and counts, so the result is safe to
// log and to show in a test failure. The tests of the readers and writers and
// the read-back check of the output use it.
//
// Children are compared by UUID, then their order; the entries of a history are
// compared by position, because they all share the UUID of their entry.
func Diff(a, b *Database) []string {
	d := &differ{}
	switch {
	case a == nil && b == nil:
		return nil
	case a == nil || b == nil:
		return []string{"database: only one side exists"}
	}
	d.meta("Meta", &a.Meta, &b.Meta)
	d.group("Root", a.Root, b.Root)
	d.deleted(a.DeletedObjects, b.DeletedObjects)
	d.binaries(a.Binaries, b.Binaries)
	d.publicCustomData(a.PublicCustomData, b.PublicCustomData)
	d.raw("Raw", a.Raw, b.Raw)
	return d.lines
}

// differ collects the differences.
type differ struct {
	lines []string
}

func (d *differ) add(path, format string, args ...any) {
	d.lines = append(d.lines, path+": "+fmt.Sprintf(format, args...))
}

// check records "path differs" unless same.
func (d *differ) check(path string, same bool) {
	if !same {
		d.add(path, "differs")
	}
}

func (d *differ) time(path string, a, b time.Time) { d.check(path, a.Equal(b)) }

func (d *differ) meta(path string, a, b *Meta) {
	d.check(path+"/Generator", a.Generator == b.Generator)
	d.check(path+"/DatabaseName", a.DatabaseName == b.DatabaseName)
	d.time(path+"/DatabaseNameChanged", a.DatabaseNameChanged, b.DatabaseNameChanged)
	d.check(path+"/Description", a.Description == b.Description)
	d.time(path+"/DescriptionChanged", a.DescriptionChanged, b.DescriptionChanged)
	d.check(path+"/DefaultUserName", a.DefaultUserName == b.DefaultUserName)
	d.time(path+"/DefaultUserNameChanged", a.DefaultUserNameChanged, b.DefaultUserNameChanged)
	d.time(path+"/SettingsChanged", a.SettingsChanged, b.SettingsChanged)
	d.check(path+"/MaintenanceHistoryDays", a.MaintenanceHistoryDays == b.MaintenanceHistoryDays)
	d.check(path+"/Color", a.Color == b.Color)
	d.time(path+"/MasterKeyChanged", a.MasterKeyChanged, b.MasterKeyChanged)
	d.check(path+"/MasterKeyChangeRec", a.MasterKeyChangeRec == b.MasterKeyChangeRec)
	d.check(path+"/MasterKeyChangeForce", a.MasterKeyChangeForce == b.MasterKeyChangeForce)
	d.check(path+"/MemoryProtection", a.MemoryProtection == b.MemoryProtection)
	d.check(path+"/RecycleBinEnabled", a.RecycleBinEnabled == b.RecycleBinEnabled)
	d.check(path+"/RecycleBinUUID", a.RecycleBinUUID == b.RecycleBinUUID)
	d.time(path+"/RecycleBinChanged", a.RecycleBinChanged, b.RecycleBinChanged)
	d.check(path+"/EntryTemplatesGroup", a.EntryTemplatesGroup == b.EntryTemplatesGroup)
	d.time(path+"/EntryTemplatesGroupChanged", a.EntryTemplatesGroupChanged, b.EntryTemplatesGroupChanged)
	d.check(path+"/HistoryMaxItems", a.HistoryMaxItems == b.HistoryMaxItems)
	d.check(path+"/HistoryMaxSize", a.HistoryMaxSize == b.HistoryMaxSize)
	d.check(path+"/LastSelectedGroup", a.LastSelectedGroup == b.LastSelectedGroup)
	d.check(path+"/LastTopVisibleGroup", a.LastTopVisibleGroup == b.LastTopVisibleGroup)
	d.customIcons(path+"/CustomIcons", a.CustomIcons, b.CustomIcons)
	d.customData(path+"/CustomData", a.CustomData, b.CustomData)
	d.raw(path+"/Raw", a.Raw, b.Raw)
}

func (d *differ) customIcons(path string, a, b []CustomIcon) {
	if len(a) != len(b) {
		d.add(path, "%d icons and %d icons", len(a), len(b))
		return
	}
	for i := range a {
		p := fmt.Sprintf("%s[%d]", path, i)
		d.check(p+"/UUID", a[i].UUID == b[i].UUID)
		d.check(p+"/Data", slices.Equal(a[i].Data, b[i].Data))
		d.check(p+"/Name", a[i].Name == b[i].Name)
		d.time(p+"/LastModified", a[i].LastModified, b[i].LastModified)
	}
}

func (d *differ) customData(path string, a, b []CustomDataItem) {
	if len(a) != len(b) {
		d.add(path, "%d items and %d items", len(a), len(b))
		return
	}
	for i := range a {
		p := fmt.Sprintf("%s[%d]", path, i)
		d.check(p+"/Key", a[i].Key == b[i].Key)
		d.check(p+"/Value", a[i].Value == b[i].Value)
		d.time(p+"/LastModified", a[i].LastModified, b[i].LastModified)
	}
}

func (d *differ) deleted(a, b []DeletedObject) {
	if len(a) != len(b) {
		d.add("DeletedObjects", "%d objects and %d objects", len(a), len(b))
		return
	}
	for i := range a {
		p := fmt.Sprintf("DeletedObjects[%d]", i)
		d.check(p+"/UUID", a[i].UUID == b[i].UUID)
		d.time(p+"/DeletionTime", a[i].DeletionTime, b[i].DeletionTime)
	}
}

func (d *differ) publicCustomData(a, b []PublicCustomDataItem) {
	if len(a) != len(b) {
		d.add("PublicCustomData", "%d items and %d items", len(a), len(b))
		return
	}
	for i := range a {
		p := fmt.Sprintf("PublicCustomData[%d]", i)
		d.check(p+"/Key", a[i].Key == b[i].Key)
		d.check(p+"/Type", a[i].Type == b[i].Type)
		d.check(p+"/Data", slices.Equal(a[i].Data, b[i].Data))
	}
}

func (d *differ) binaries(a, b map[BinaryKey]*Binary) {
	keys := map[BinaryKey]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	ordered := make([]BinaryKey, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Slice(ordered, func(i, j int) bool {
		x, y := ordered[i], ordered[j]
		if c := slices.Compare(x.Hash[:], y.Hash[:]); c != 0 {
			return c < 0
		}
		return !x.Protected && y.Protected
	})
	for _, k := range ordered {
		p := fmt.Sprintf("Binaries/%x(protected=%v)", k.Hash[:6], k.Protected)
		ba, inA := a[k]
		bb, inB := b[k]
		switch {
		case inA && !inB:
			d.add(p, "only in the first database")
		case !inA && inB:
			d.add(p, "only in the second database")
		case !ba.Data.Equal(bb.Data):
			d.add(p, "content differs")
		}
	}
}

func (d *differ) times(path string, a, b Times) {
	d.time(path+"/Creation", a.Creation, b.Creation)
	d.time(path+"/LastModification", a.LastModification, b.LastModification)
	d.time(path+"/LastAccess", a.LastAccess, b.LastAccess)
	d.time(path+"/Expiry", a.Expiry, b.Expiry)
	d.check(path+"/Expires", a.Expires == b.Expires)
	d.check(path+"/UsageCount", a.UsageCount == b.UsageCount)
	d.time(path+"/LocationChanged", a.LocationChanged, b.LocationChanged)
}

func (d *differ) group(path string, a, b *Group) {
	switch {
	case a == nil && b == nil:
		return
	case a == nil || b == nil:
		d.add(path, "only one side has the group")
		return
	}
	d.check(path+"/UUID", a.UUID == b.UUID)
	d.check(path+"/Name", a.Name == b.Name)
	d.check(path+"/Notes", a.Notes == b.Notes)
	d.check(path+"/IconID", a.IconID == b.IconID)
	d.check(path+"/CustomIcon", a.CustomIcon == b.CustomIcon)
	d.times(path+"/Times", a.Times, b.Times)
	d.check(path+"/IsExpanded", a.IsExpanded == b.IsExpanded)
	d.check(path+"/DefaultAutoTypeSequence", a.DefaultAutoTypeSequence == b.DefaultAutoTypeSequence)
	d.check(path+"/EnableAutoType", a.EnableAutoType == b.EnableAutoType)
	d.check(path+"/EnableSearching", a.EnableSearching == b.EnableSearching)
	d.check(path+"/LastTopVisibleEntry", a.LastTopVisibleEntry == b.LastTopVisibleEntry)
	d.check(path+"/Tags", slices.Equal(a.Tags, b.Tags))
	d.check(path+"/PreviousParent", a.PreviousParent == b.PreviousParent)
	d.customData(path+"/CustomData", a.CustomData, b.CustomData)
	d.raw(path+"/Raw", a.Raw, b.Raw)

	// Subgroups by UUID, then their order.
	subA, subB := map[UUID]*Group{}, map[UUID]*Group{}
	for _, g := range a.Groups {
		subA[g.UUID] = g
	}
	for _, g := range b.Groups {
		subB[g.UUID] = g
	}
	var common []UUID
	for _, g := range a.Groups {
		p := path + "/group " + g.UUID.String()
		if o, ok := subB[g.UUID]; ok {
			common = append(common, g.UUID)
			d.group(p, g, o)
		} else {
			d.add(p, "only in the first database")
		}
	}
	for _, g := range b.Groups {
		if _, ok := subA[g.UUID]; !ok {
			d.add(path+"/group "+g.UUID.String(), "only in the second database")
		}
	}
	d.check(path+"/group order", slices.Equal(common, commonOrder(b.Groups, subA, func(g *Group) UUID { return g.UUID })))

	// Entries likewise.
	entA, entB := map[UUID]*Entry{}, map[UUID]*Entry{}
	for _, e := range a.Entries {
		entA[e.UUID] = e
	}
	for _, e := range b.Entries {
		entB[e.UUID] = e
	}
	var commonE []UUID
	for _, e := range a.Entries {
		p := path + "/entry " + e.UUID.String()
		if o, ok := entB[e.UUID]; ok {
			commonE = append(commonE, e.UUID)
			d.entry(p, e, o)
		} else {
			d.add(p, "only in the first database")
		}
	}
	for _, e := range b.Entries {
		if _, ok := entA[e.UUID]; !ok {
			d.add(path+"/entry "+e.UUID.String(), "only in the second database")
		}
	}
	d.check(path+"/entry order", slices.Equal(commonE, commonOrder(b.Entries, entA, func(e *Entry) UUID { return e.UUID })))
}

// commonOrder returns the UUIDs of items that are also in other, in the order of
// items.
func commonOrder[T any](items []T, other map[UUID]T, uuid func(T) UUID) []UUID {
	var out []UUID
	for _, it := range items {
		if _, ok := other[uuid(it)]; ok {
			out = append(out, uuid(it))
		}
	}
	return out
}

func (d *differ) entry(path string, a, b *Entry) {
	d.entryBody(path, a, b)
	if len(a.History) != len(b.History) {
		d.add(path+"/History", "%d versions and %d versions", len(a.History), len(b.History))
		return
	}
	for i := range a.History {
		d.entryBody(fmt.Sprintf("%s/History[%d]", path, i), a.History[i], b.History[i])
	}
}

// entryBody compares everything of an entry but its history.
func (d *differ) entryBody(path string, a, b *Entry) {
	d.check(path+"/UUID", a.UUID == b.UUID)
	d.check(path+"/IconID", a.IconID == b.IconID)
	d.check(path+"/CustomIcon", a.CustomIcon == b.CustomIcon)
	d.check(path+"/ForegroundColor", a.ForegroundColor == b.ForegroundColor)
	d.check(path+"/BackgroundColor", a.BackgroundColor == b.BackgroundColor)
	d.check(path+"/OverrideURL", a.OverrideURL == b.OverrideURL)
	d.check(path+"/Tags", slices.Equal(a.Tags, b.Tags))
	d.times(path+"/Times", a.Times, b.Times)
	d.check(path+"/QualityCheck", (a.QualityCheck == nil) == (b.QualityCheck == nil) &&
		(a.QualityCheck == nil || *a.QualityCheck == *b.QualityCheck))
	d.check(path+"/PreviousParent", a.PreviousParent == b.PreviousParent)
	d.fields(path+"/Fields", a.Fields, b.Fields)
	if len(a.Attachments) != len(b.Attachments) {
		d.add(path+"/Attachments", "%d attachments and %d attachments", len(a.Attachments), len(b.Attachments))
	} else {
		for i := range a.Attachments {
			p := fmt.Sprintf("%s/Attachments[%d]", path, i)
			d.check(p+"/Name", a.Attachments[i].Name == b.Attachments[i].Name)
			d.check(p+"/Binary", a.Attachments[i].Binary == b.Attachments[i].Binary)
		}
	}
	d.check(path+"/AutoType", autoTypeEqual(a.AutoType, b.AutoType))
	d.customData(path+"/CustomData", a.CustomData, b.CustomData)
	d.raw(path+"/Raw", a.Raw, b.Raw)
}

func autoTypeEqual(a, b AutoType) bool {
	return a.Enabled == b.Enabled && a.DataTransferObfuscation == b.DataTransferObfuscation &&
		a.DefaultSequence == b.DefaultSequence && slices.Equal(a.Associations, b.Associations)
}

func (d *differ) fields(path string, a, b []Field) {
	if len(a) != len(b) {
		d.add(path, "%d fields and %d fields", len(a), len(b))
		return
	}
	for i := range a {
		if a[i].Key != b[i].Key {
			d.add(fmt.Sprintf("%s[%d]", path, i), "key differs (%q and %q)", a[i].Key, b[i].Key)
			continue
		}
		p := path + "/" + a[i].Key
		d.check(p+" protected", a[i].Protected == b[i].Protected)
		d.check(p, a[i].Value.Equal(b[i].Value))
	}
}

func (d *differ) raw(path string, a, b Raw) {
	d.check(path+"/Attrs", slices.Equal(a.Attrs, b.Attrs))
	if len(a.Nodes) != len(b.Nodes) {
		d.add(path+"/Nodes", "%d nodes and %d nodes", len(a.Nodes), len(b.Nodes))
		return
	}
	for i := range a.Nodes {
		d.rawNode(fmt.Sprintf("%s/Nodes[%d]", path, i), a.Nodes[i], b.Nodes[i])
	}
}

func (d *differ) rawNode(path string, a, b *RawNode) {
	d.check(path+"/Name", a.Name == b.Name)
	d.check(path+"/Attrs", slices.Equal(a.Attrs, b.Attrs))
	d.check(path+"/Protected", a.Protected == b.Protected)
	d.check(path+"/Text", a.Text.Equal(b.Text))
	if len(a.Children) != len(b.Children) {
		d.add(path+"/Children", "%d children and %d children", len(a.Children), len(b.Children))
		return
	}
	for i := range a.Children {
		d.rawNode(fmt.Sprintf("%s/Children[%d]", path, i), a.Children[i], b.Children[i])
	}
}
