package diff

import (
	"time"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// u makes a UUID from a number.
func u(n byte) model.UUID { return model.UUID{15: n} }

// grp makes a group with the UUID n and children (groups and entries).
func grp(n byte, name string, children ...any) *model.Group {
	g := &model.Group{UUID: u(n), Name: name}
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

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// at is epoch plus n hours.
func at(n int) time.Time { return epoch.Add(time.Duration(n) * time.Hour) }

// ent makes an entry with the UUID n, a title, a password and a modification time.
func ent(n byte, title, password string, hours int) *model.Entry {
	return &model.Entry{
		UUID: u(n),
		Fields: []model.Field{
			{Key: model.FieldTitle, Value: secret.New([]byte(title))},
			{Key: model.FieldUserName, Value: secret.New([]byte("user"))},
			{Key: model.FieldPassword, Value: secret.New([]byte(password)), Protected: true},
		},
		Times: model.Times{LastModification: at(hours)},
	}
}

// database makes a database with the root group and the recycle bin UUID.
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
