package modeltest

import (
	"time"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// The passwords of the two databases of MergePair, for the tests that open the
// files built from them. They protect nothing.
const (
	PairPasswordA = "merge-pair-a"
	PairPasswordB = "merge-pair-b"
)

// pairTime is a fixed time for the pair: day 1 of October 2026 plus n hours.
func pairTime(n int) time.Time {
	return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
}

func pairUUID(n byte) model.UUID { return model.UUID{0: 0xab, 1: 0xcd, 15: n} }

// pairEntry makes an entry of the pair.
func pairEntry(n byte, title, user, password, url string, modified int) *model.Entry {
	e := &model.Entry{
		UUID: pairUUID(n),
		Fields: []model.Field{
			{Key: model.FieldTitle, Value: secret.New([]byte(title))},
			{Key: model.FieldUserName, Value: secret.New([]byte(user))},
			{Key: model.FieldPassword, Value: secret.New([]byte(password)), Protected: true},
			{Key: model.FieldURL, Value: secret.New([]byte(url))},
			{Key: model.FieldNotes, Value: secret.New(nil)},
		},
		Times: model.Times{Creation: pairTime(0), LastModification: pairTime(modified), LastAccess: pairTime(modified), LocationChanged: pairTime(0)},
	}
	return e
}

func pairGroup(n byte, name, notes string, modified int, children ...any) *model.Group {
	g := &model.Group{
		UUID: pairUUID(n), Name: name, Notes: notes, IconID: 48,
		Times: model.Times{Creation: pairTime(0), LastModification: pairTime(modified), LastAccess: pairTime(modified), LocationChanged: pairTime(0)},
	}
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

func pairDatabase(name string, root *model.Group, deleted ...model.DeletedObject) *model.Database {
	db := &model.Database{
		Root:           root,
		DeletedObjects: deleted,
		Binaries:       map[model.BinaryKey]*model.Binary{},
	}
	db.Meta.DatabaseName = name
	db.Meta.DatabaseNameChanged = pairTime(0)
	db.Meta.SettingsChanged = pairTime(0)
	db.Meta.HistoryMaxItems = 10
	db.Meta.HistoryMaxSize = 6 << 20
	db.Meta.MemoryProtection = model.MemoryProtection{Password: true}
	return db
}

// MergePair returns two related databases, A and B, that came from one database
// and were edited on two devices, with one change of every kind of SPEC §8. The
// same call always gives the same databases, and the files built from them are
// committed under internal/cli/testdata/merge.
//
// The changes between them, in the order of the change list:
//
//   - only in A: the entry "Work mail" in "Email"
//   - only in B: the group "Games" and its entry "Old forum"
//   - differs: "My bank" in "Banking" (password and URL; B is newer) and
//     "Store" in "Shopping" (title; A is newer)
//   - deleted in A: "Old account" in "Social", which B still holds and A deleted
//     after B's last change (the default is to drop it)
//   - deleted in B: "Stale login" in "Social", which A still holds and B deleted
//     before A's last change (the default is to keep it)
//   - group differs: "Shopping" has other notes in B
//   - settings: the name of the database
//
// "Personal" in "Email" is the same in both.
func MergePair() (a, b *model.Database) {
	a = pairDatabase("Personal", pairGroup(1, "Root", "", 0,
		pairGroup(2, "Email", "", 0,
			pairEntry(10, "Personal", "me@example.com", "pw-personal", "https://mail.example.com", 1),
			pairEntry(11, "Work mail", "me@work.example", "pw-work-mail", "https://mail.work.example", 5),
		),
		pairGroup(3, "Banking", "", 0,
			pairEntry(12, "My bank", "me", "bank-password-a", "https://bank.example.com", 3),
		),
		pairGroup(4, "Shopping", "", 2,
			pairEntry(13, "Store", "me", "store-password", "https://store.example.com", 9),
		),
		pairGroup(5, "Social", "", 0,
			pairEntry(14, "Stale login", "me", "stale-password", "https://social.example.com", 8),
		),
	), model.DeletedObject{UUID: pairUUID(15), DeletionTime: pairTime(12)})

	b = pairDatabase("Laptop", pairGroup(1, "Root", "", 0,
		pairGroup(2, "Email", "", 0,
			pairEntry(10, "Personal", "me@example.com", "pw-personal", "https://mail.example.com", 1),
		),
		pairGroup(3, "Banking", "", 0,
			pairEntry(12, "My bank", "me", "bank-password-b", "https://bank.example.org", 20),
		),
		pairGroup(4, "Shopping", "Ship to the office", 6,
			pairEntry(13, "Store (old name)", "me", "store-password", "https://store.example.com", 4),
		),
		pairGroup(5, "Social", "", 0,
			pairEntry(15, "Old account", "me", "old-account-password", "https://old.example.com", 6),
		),
		pairGroup(6, "Games", "", 0,
			pairEntry(16, "Old forum", "gamer", "forum-password", "https://forum.example.com", 7),
		),
	), model.DeletedObject{UUID: pairUUID(14), DeletionTime: pairTime(2)})
	return a, b
}

// LargePair returns two related databases with n entries each, for tests of speed
// and size: A has n entries in groups of 100, and B is a copy edited elsewhere (one
// entry in ten has another password and is newer, one in twenty-five is deleted,
// one in thirty-three has moved to another group), with n/20 entries that only B
// has. The same n always gives the same databases.
func LargePair(n int) (a, b *model.Database) {
	root := pairGroup(1, "Root", "", 0)
	var groups []*model.Group
	for g := 0; g*100 < n; g++ {
		grp := &model.Group{
			UUID: model.UUID{0: 0xab, 1: 0xce, 2: byte(g >> 8), 3: byte(g)}, Name: "Group " + itoa(g), IconID: 48,
			Times: model.Times{Creation: pairTime(0), LastModification: pairTime(0)},
		}
		groups = append(groups, grp)
		root.Groups = append(root.Groups, grp)
	}
	entry := func(i int, password string, modified int) *model.Entry {
		e := pairEntry(0, "Entry "+itoa(i), "user"+itoa(i), password, "https://site"+itoa(i)+".example.com", modified)
		e.UUID = model.UUID{0: 0xab, 1: 0xcf, 2: byte(i >> 24), 3: byte(i >> 16), 4: byte(i >> 8), 5: byte(i)}
		return e
	}
	for i := 0; i < n; i++ {
		g := groups[i/100]
		g.Entries = append(g.Entries, entry(i, "password-"+itoa(i), 1))
	}
	a = pairDatabase("Large A", root)
	b = a.Clone()
	b.Meta.DatabaseName = "Large B"
	bGroups := b.Root.Groups
	for gi, g := range bGroups {
		var kept []*model.Entry
		for _, e := range g.Entries {
			i := gi*100 + indexIn(a.Root.Groups[gi], e.UUID)
			switch {
			case i%25 == 2:
				b.DeletedObjects = append(b.DeletedObjects, model.DeletedObject{UUID: e.UUID, DeletionTime: pairTime(5)})
				continue
			case i%33 == 3:
				other := bGroups[(gi+1)%len(bGroups)]
				other.Entries = append(other.Entries, e)
				continue
			case i%10 == 0:
				e.Fields[2].Value = secret.New([]byte("changed-" + itoa(i)))
				e.Times.LastModification = pairTime(9)
			}
			kept = append(kept, e)
		}
		g.Entries = kept
	}
	for i := 0; i < n/20; i++ {
		g := bGroups[i%len(bGroups)]
		g.Entries = append(g.Entries, entry(n+i, "new-"+itoa(i), 2))
	}
	return a, b
}

func indexIn(g *model.Group, u model.UUID) int {
	for i, e := range g.Entries {
		if e.UUID == u {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
