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
