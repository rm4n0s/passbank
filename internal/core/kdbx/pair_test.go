package kdbx

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/testenv"
)

// normalizeForPair removes what two programs legitimately do differently when
// they store the same content, so that model.Diff shows only real differences:
// KeePassXC sorts the fields of an entry by key (pykeepass keeps their order),
// leaves the standard icon out when a custom icon is set, adds the
// custom data items `_LAST_MODIFIED` and `KPXC_RANDOM_SLUG`, and stores an imported attachment without
// the protected flag.
func normalizeForPair(db *model.Database) *model.Database {
	db = db.Clone()
	db.Meta.CustomData = slices.DeleteFunc(db.Meta.CustomData, func(i model.CustomDataItem) bool { return i.Key == "_LAST_MODIFIED" || i.Key == "KPXC_RANDOM_SLUG" })
	fix := func(e *model.Entry) {
		slices.SortFunc(e.Fields, func(a, b model.Field) int { return strings.Compare(a.Key, b.Key) })
		if !e.CustomIcon.IsZero() {
			e.IconID = 0
		}
		for i := range e.Attachments {
			e.Attachments[i].Binary.Protected = false
		}
	}
	db.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
		fix(e)
		for _, h := range e.History {
			fix(h)
		}
		return true
	})
	pool := map[model.BinaryKey]*model.Binary{}
	for k, b := range db.Binaries {
		k.Protected = false
		pool[k] = b
	}
	db.Binaries = pool
	return db
}

// TestKDBX3AndKDBX4SameContent opens the rich fixture made by KeePassXC as KDBX
// 3.1 and the one made by pykeepass as KDBX 4.0, both from the same source XML,
// and requires equal models: the two readers, with their different framing,
// times, binaries and inner streams, give the same database.
func TestKDBX3AndKDBX4SameContent(t *testing.T) {
	a, _, err := Open(context.Background(), testenv.Fixture(t, "k31-rich.kdbx"), credsOf(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := Open(context.Background(), testenv.Fixture(t, "k4-rich.kdbx"), credsOf(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	if diff := model.Diff(normalizeForPair(a), normalizeForPair(b)); len(diff) != 0 {
		t.Errorf("the KDBX 3.1 and the KDBX 4.0 fixture differ:\n%s", strings.Join(diff, "\n"))
	}
}
