package merge

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

func u(n byte) model.UUID { return model.UUID{15: n} }

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func at(n int) time.Time { return epoch.Add(time.Duration(n) * time.Hour) }

// grp makes a group with the UUID n and children (groups and entries).
func grp(n byte, name string, children ...any) *model.Group {
	g := &model.Group{UUID: u(n), Name: name, Times: model.Times{LastModification: at(0)}}
	for _, c := range children {
		switch c := c.(type) {
		case *model.Group:
			g.Groups = append(g.Groups, c)
		case *model.Entry:
			g.Entries = append(g.Entries, c)
		}
	}
	return g
}

// ent makes an entry with the UUID n, a title, a password and a modification time.
func ent(n byte, title, password string, hours int) *model.Entry {
	return &model.Entry{
		UUID: u(n),
		Fields: []model.Field{
			{Key: model.FieldTitle, Value: secret.New([]byte(title))},
			{Key: model.FieldPassword, Value: secret.New([]byte(password)), Protected: true},
		},
		Times: model.Times{LastModification: at(hours)},
	}
}

// database makes a database with the root group and the UUID of the recycle bin
// (0 for none).
func database(root *model.Group, bin byte, deleted ...model.DeletedObject) *model.Database {
	db := &model.Database{Root: root, DeletedObjects: deleted, Binaries: map[model.BinaryKey]*model.Binary{}}
	if bin != 0 {
		db.Meta.RecycleBinUUID = u(bin)
		db.Meta.RecycleBinEnabled = true
	}
	return db
}

func deleted(n byte, hours int) model.DeletedObject {
	return model.DeletedObject{UUID: u(n), DeletionTime: at(hours)}
}

// render writes the tree as text: a group is `Name{...}` and an entry is
// `uuid=title/password`, the children in order.
func render(db *model.Database) string { return renderKnown(db, nil) }

// renderKnown is render with `*` instead of the UUID of an entry that is not in
// known (the copies that `both` makes); a nil known means every entry is known.
func renderKnown(db *model.Database, known map[model.UUID]bool) string {
	var b strings.Builder
	var walk func(g *model.Group)
	walk = func(g *model.Group) {
		b.WriteString(g.Name + "{")
		first := true
		sep := func() {
			if !first {
				b.WriteString(" ")
			}
			first = false
		}
		for _, e := range g.Entries {
			sep()
			pw, _ := e.Field(model.FieldPassword)
			if known != nil && !known[e.UUID] {
				fmt.Fprintf(&b, "*=%s/%s", e.Title(), pw.Text())
			} else {
				fmt.Fprintf(&b, "%d=%s/%s", e.UUID[15], e.Title(), pw.Text())
			}
		}
		for _, c := range g.Groups {
			sep()
			walk(c)
		}
		b.WriteString("}")
	}
	walk(db.Root)
	return b.String()
}

// planFor makes the plan for A and B with the choices given by UUID byte (the rest
// get the defaults); settings is the choice for the settings change.
func planFor(t *testing.T, a, b *model.Database, choices map[byte]plan.Choice) *plan.Plan {
	t.Helper()
	p := plan.New(plan.Input{Name: "a"}, plan.Input{Name: "b"}, diff.Changes(a, b))
	for i, c := range p.Changes {
		if c.Item == diff.ItemSettings {
			continue
		}
		if choice, ok := choices[c.UUID[15]]; ok {
			if !p.Set(i, choice) {
				t.Fatalf("choice %s is not allowed for %s", choice, c.Kind)
			}
		}
	}
	return p
}

func mustApply(t *testing.T, a, b *model.Database, p *plan.Plan) *model.Database {
	t.Helper()
	res, err := Apply(a, b, p)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// entryUUIDs returns the UUIDs of the entries of the databases.
func entryUUIDs(dbs ...*model.Database) map[model.UUID]bool {
	known := map[model.UUID]bool{}
	for _, db := range dbs {
		db.WalkEntries(func(_ *model.Group, e *model.Entry) bool { known[e.UUID] = true; return true })
	}
	return known
}
