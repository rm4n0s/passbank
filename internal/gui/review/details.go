package review

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/gui/kit"
)

// Details is what the right side of the Review screen shows for one change: the
// rows of the side-by-side table, and the names of the differences that the table
// has no row for.
type Details struct {
	// Rows are the rows of the table: A's and B's values, the differing ones marked.
	Rows []kit.FieldRow
	// Also names what else differs (the auto-type settings, custom data, ...), as the
	// plan file names it; never values.
	Also []string
}

// timeFormat shows a time the same way everywhere.
const timeFormat = "2006-01-02 15:04:05 UTC"

// unknownTime is shown for a time that is not known.
const unknownTime = "(unknown)"

// DetailsOf builds the details of a change between the databases a and b.
func DetailsOf(c diff.Change, a, b *model.Database) Details {
	d := &detailsBuilder{c: c, dbs: [2]*model.Database{a, b}, covered: map[string]bool{}}
	switch c.Item {
	case diff.ItemSettings:
		d.settings()
	case diff.ItemGroup:
		d.group()
	default:
		d.entry()
	}
	d.times()
	for _, f := range c.Fields {
		if !d.covered[f] {
			d.out.Also = append(d.out.Also, f)
		}
	}
	return d.out
}

type detailsBuilder struct {
	c       diff.Change
	dbs     [2]*model.Database
	out     Details
	covered map[string]bool
}

// differs says whether the plan names the field as one that differs.
func (d *detailsBuilder) differs(names ...string) bool {
	for _, n := range names {
		if slices.Contains(d.c.Fields, n) {
			return true
		}
	}
	return false
}

// add appends a row whose two values are text. covers lists the names of the
// difference that the row shows.
func (d *detailsBuilder) add(name string, a, b string, missing [2]bool, differs bool, covers ...string) {
	for _, n := range covers {
		d.covered[n] = true
	}
	d.out.Rows = append(d.out.Rows, kit.FieldRow{
		Name: name, A: a, B: b, MissingA: missing[0], MissingB: missing[1], Differs: differs,
	})
}

// entry builds the rows of an entry change.
func (d *detailsBuilder) entry() {
	c := d.c
	e := c.Entry
	missing := [2]bool{e[0] == nil, e[1] == nil}

	groupName := func(g *model.Group) string {
		if g == nil {
			return ""
		}
		return g.Name
	}
	d.add("Group", groupName(c.Parent[0]), groupName(c.Parent[1]), [2]bool{c.Parent[0] == nil, c.Parent[1] == nil},
		d.differs(diff.FieldGroup, diff.FieldParent), diff.FieldGroup, diff.FieldParent)

	standard := []struct{ key, name, label string }{
		{model.FieldTitle, "title", "Title"},
		{model.FieldUserName, "username", "User name"},
		{model.FieldPassword, "password", "Password"},
		{model.FieldURL, "url", "URL"},
		{model.FieldNotes, "notes", "Notes"},
	}
	keys := make([]string, 0, len(standard))
	for _, s := range standard {
		keys = append(keys, s.key)
	}
	var custom []string
	for side := 0; side < 2; side++ {
		if e[side] == nil {
			continue
		}
		for _, f := range e[side].Fields {
			if !slices.Contains(keys, f.Key) && !slices.Contains(custom, f.Key) {
				custom = append(custom, f.Key)
			}
		}
	}
	rows := make([]struct{ key, label, name string }, 0, len(keys)+len(custom))
	for _, s := range standard {
		rows = append(rows, struct{ key, label, name string }{s.key, s.label, s.name})
	}
	for _, k := range custom {
		rows = append(rows, struct{ key, label, name string }{k, k, "field " + k})
	}
	for _, r := range rows {
		var text [2]string
		var absent [2]bool
		var field [2]model.Field
		protected := r.key == model.FieldPassword
		for side := 0; side < 2; side++ {
			if e[side] == nil {
				absent[side] = true
				continue
			}
			f, ok := e[side].Field(r.key)
			if !ok {
				absent[side] = true
				continue
			}
			field[side], text[side] = f, f.Text()
			protected = protected || f.Protected
		}
		differs := absent[0] != absent[1] || (!absent[0] && (field[0].Protected != field[1].Protected || !bytes.Equal(field[0].Value.Reveal(), field[1].Value.Reveal())))
		if missing[0] || missing[1] {
			differs = false // a side that has no entry at all is not a difference in a field
		}
		d.covered[r.name] = true
		d.out.Rows = append(d.out.Rows, kit.FieldRow{
			Name: r.label, A: text[0], B: text[1], MissingA: absent[0], MissingB: absent[1], Differs: differs, Protected: protected,
		})
	}

	d.attachments()
	d.covered[diff.FieldTags], d.covered[diff.FieldExpiry] = true, true
	if tags := [2]string{tagsOf(e[0]), tagsOf(e[1])}; d.differs(diff.FieldTags) || tags[0] != "" || tags[1] != "" {
		d.add("Tags", tags[0], tags[1], missing, d.differs(diff.FieldTags), diff.FieldTags)
	}
	if exp := [2]string{expiryOf(e[0]), expiryOf(e[1])}; d.differs(diff.FieldExpiry) || (exp[0] != "never" && exp[0] != "") || (exp[1] != "never" && exp[1] != "") {
		d.add("Expires", exp[0], exp[1], missing, d.differs(diff.FieldExpiry), diff.FieldExpiry)
	}
	if d.differs(diff.FieldIcon) {
		d.add("Icon", iconOf(e[0]), iconOf(e[1]), missing, true, diff.FieldIcon)
	}
	if d.differs(diff.FieldColors) {
		d.add("Colors", colorsOf(e[0]), colorsOf(e[1]), missing, true, diff.FieldColors)
	}
	if d.differs(diff.FieldOverrideURL) {
		d.add("Override URL", overrideOf(e[0]), overrideOf(e[1]), missing, true, diff.FieldOverrideURL)
	}
}

// attachments adds one row for each attached file, marked as the same or as
// different.
func (d *detailsBuilder) attachments() {
	e := d.c.Entry
	var names []string
	for side := 0; side < 2; side++ {
		if e[side] == nil {
			continue
		}
		for _, at := range e[side].Attachments {
			if !slices.Contains(names, at.Name) {
				names = append(names, at.Name)
			}
		}
	}
	d.covered[diff.FieldAttachments] = true
	if len(names) == 0 {
		return
	}
	for _, name := range names {
		var text [2]string
		var absent [2]bool
		var key [2]model.BinaryKey
		for side := 0; side < 2; side++ {
			absent[side] = true
			if e[side] == nil {
				continue
			}
			for _, at := range e[side].Attachments {
				if at.Name == name {
					absent[side] = false
					key[side] = at.Binary
					text[side] = name + " (" + d.size(side, at.Binary) + ")"
					break
				}
			}
		}
		state := "same"
		differs := false
		switch {
		case e[0] == nil || e[1] == nil:
			state = ""
		case absent[0] || absent[1] || key[0] != key[1]:
			state, differs = "different", true
		}
		label := "Attachment"
		if state != "" {
			label += " (" + state + ")"
		}
		d.out.Rows = append(d.out.Rows, kit.FieldRow{Name: label, A: text[0], B: text[1], MissingA: absent[0], MissingB: absent[1], Differs: differs})
	}
}

// size is the size of an attachment of the database of a side, for display.
func (d *detailsBuilder) size(side int, key model.BinaryKey) string {
	db := d.dbs[side]
	if db == nil || db.Binaries[key] == nil {
		return "size unknown"
	}
	n := db.Binaries[key].Data.Len()
	if n == 1 {
		return "1 byte"
	}
	return strconv.Itoa(n) + " bytes"
}

// group builds the rows of a group change.
func (d *detailsBuilder) group() {
	c := d.c
	g := c.Group
	missing := [2]bool{g[0] == nil, g[1] == nil}
	name := func(g *model.Group) string {
		if g == nil {
			return ""
		}
		return g.Name
	}
	notes := func(g *model.Group) string {
		if g == nil {
			return ""
		}
		return g.Notes
	}
	d.add("Name", name(g[0]), name(g[1]), missing, d.differs(diff.FieldName), diff.FieldName)
	d.add("Notes", notes(g[0]), notes(g[1]), missing, d.differs(diff.FieldNotes), diff.FieldNotes)
	d.add("Parent group", name(c.Parent[0]), name(c.Parent[1]), [2]bool{c.Parent[0] == nil, c.Parent[1] == nil},
		d.differs(diff.FieldParent), diff.FieldParent)
	if d.differs(diff.FieldIcon) {
		d.add("Icon", groupIcon(g[0]), groupIcon(g[1]), missing, true, diff.FieldIcon)
	}
	if d.differs(diff.FieldTags) {
		d.add("Tags", groupTags(g[0]), groupTags(g[1]), missing, true, diff.FieldTags)
	}
}

// settings builds the rows of the settings change from the Meta of the databases.
func (d *detailsBuilder) settings() {
	var m [2]*model.Meta
	var missing [2]bool
	for i, db := range d.dbs {
		if db == nil {
			missing[i] = true
			continue
		}
		m[i] = &db.Meta
	}
	row := func(label, field string, f func(*model.Meta) string) {
		var t [2]string
		for i := range m {
			if m[i] != nil {
				t[i] = f(m[i])
			}
		}
		d.add(label, t[0], t[1], missing, d.differs(field), field)
	}
	row("Database name", diff.FieldName, func(m *model.Meta) string { return m.DatabaseName })
	row("Description", diff.FieldDescription, func(m *model.Meta) string { return m.Description })
	row("Default user name", diff.FieldDefaultUserName, func(m *model.Meta) string { return m.DefaultUserName })
	row("Color", diff.FieldColor, func(m *model.Meta) string { return m.Color })
	row("History limits", diff.FieldHistoryLimits, func(m *model.Meta) string {
		return strconv.Itoa(m.HistoryMaxItems) + " items, " + strconv.FormatInt(m.HistoryMaxSize, 10) + " bytes"
	})
	row("Recycle bin", diff.FieldRecycleBin, func(m *model.Meta) string { return onOff(m.RecycleBinEnabled) })
	row("Protected fields", diff.FieldMemoryProtection, func(m *model.Meta) string {
		p := m.MemoryProtection
		var on []string
		for _, f := range []struct {
			name string
			on   bool
		}{{"title", p.Title}, {"user name", p.UserName}, {"password", p.Password}, {"URL", p.URL}, {"notes", p.Notes}} {
			if f.on {
				on = append(on, f.name)
			}
		}
		if len(on) == 0 {
			return "none"
		}
		return strings.Join(on, ", ")
	})
}

// times adds the modification times of both sides and, for a deletion, when it
// happened.
func (d *detailsBuilder) times() {
	c := d.c
	show := func(t time.Time, exists bool) (string, bool) {
		if !exists {
			return "", true
		}
		if t.IsZero() {
			return unknownTime, false
		}
		return t.UTC().Format(timeFormat), false
	}
	var exists [2]bool
	switch c.Item {
	case diff.ItemGroup:
		exists = [2]bool{c.Group[0] != nil, c.Group[1] != nil}
	case diff.ItemSettings:
		exists = [2]bool{d.dbs[0] != nil, d.dbs[1] != nil}
	default:
		exists = [2]bool{c.Entry[0] != nil, c.Entry[1] != nil}
	}
	a, ma := show(c.Modified[0], exists[0])
	b, mb := show(c.Modified[1], exists[1])
	label := "Modified"
	if c.Item == diff.ItemSettings {
		label = "Settings changed"
	}
	d.out.Rows = append(d.out.Rows, kit.FieldRow{Name: label, A: a, B: b, MissingA: ma, MissingB: mb})
	if c.Kind == diff.DeletedInA || c.Kind == diff.DeletedInB {
		del := [2]string{unknownTime, unknownTime}
		if !c.Deleted.IsZero() {
			del[0] = c.Deleted.UTC().Format(timeFormat)
			del[1] = del[0]
		}
		row := kit.FieldRow{Name: "Deleted"}
		if c.Kind == diff.DeletedInA {
			row.A, row.MissingB = del[0], true
		} else {
			row.B, row.MissingA = del[1], true
		}
		d.out.Rows = append(d.out.Rows, row)
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func tagsOf(e *model.Entry) string {
	if e == nil {
		return ""
	}
	return strings.Join(e.Tags, ", ")
}

func groupTags(g *model.Group) string {
	if g == nil {
		return ""
	}
	return strings.Join(g.Tags, ", ")
}

func expiryOf(e *model.Entry) string {
	switch {
	case e == nil:
		return ""
	case !e.Times.Expires:
		return "never"
	}
	return e.Times.Expiry.UTC().Format(timeFormat)
}

func iconOf(e *model.Entry) string {
	if e == nil {
		return ""
	}
	if e.CustomIcon != (model.UUID{}) {
		return "custom icon " + e.CustomIcon.String()
	}
	return "icon " + strconv.Itoa(e.IconID)
}

func groupIcon(g *model.Group) string {
	if g == nil {
		return ""
	}
	if g.CustomIcon != (model.UUID{}) {
		return "custom icon " + g.CustomIcon.String()
	}
	return "icon " + strconv.Itoa(g.IconID)
}

func colorsOf(e *model.Entry) string {
	if e == nil {
		return ""
	}
	return "text " + orNone(e.ForegroundColor) + ", background " + orNone(e.BackgroundColor)
}

func overrideOf(e *model.Entry) string {
	if e == nil {
		return ""
	}
	return orNone(e.OverrideURL)
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
