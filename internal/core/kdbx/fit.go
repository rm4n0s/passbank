package kdbx

import (
	"time"

	"github.com/rm4n0s/passbank/internal/core/model"
)

// DefaultGenerator is the generator that a database without one is written with.
const DefaultGenerator = "Passbank"

// Fit returns a copy of the database as a file of the version holds it: what the
// version cannot store is removed (the 4.1 fields for KDBX 4.0) and an empty
// generator is the default. A database that is read back from the file Encode
// wrote for the version is equal to it, which is how the output is verified. It
// does not decide whether the loss is allowed (SPEC §5).
func Fit(db *model.Database, version Version) *model.Database {
	db = db.Clone()
	if db.Meta.Generator == "" {
		db.Meta.Generator = DefaultGenerator
	}
	if version.Major == 4 && version.Minor >= 1 {
		return db
	}
	db.Meta.SettingsChanged = time.Time{}
	for i := range db.Meta.CustomIcons {
		db.Meta.CustomIcons[i].Name, db.Meta.CustomIcons[i].LastModified = "", time.Time{}
	}
	clearTimes := func(items []model.CustomDataItem) {
		for i := range items {
			items[i].LastModified = time.Time{}
		}
	}
	clearTimes(db.Meta.CustomData)
	var fixEntry func(e *model.Entry)
	fixEntry = func(e *model.Entry) {
		e.QualityCheck, e.PreviousParent = nil, model.UUID{}
		clearTimes(e.CustomData)
		for _, h := range e.History {
			fixEntry(h)
		}
	}
	db.WalkGroups(func(g *model.Group) bool {
		g.Tags, g.PreviousParent = nil, model.UUID{}
		clearTimes(g.CustomData)
		for _, e := range g.Entries {
			fixEntry(e)
		}
		return true
	})
	return db
}
