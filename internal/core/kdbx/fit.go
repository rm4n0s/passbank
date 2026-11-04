package kdbx

import (
	"time"

	"github.com/rm4n0s/passbank/internal/core/model"
)

// DefaultGenerator is the generator that a database without one is written with.
const DefaultGenerator = "Passbank"

// limits say what a KDBX version has a place for beyond what every version has
// (SPEC §5).
type limits struct {
	// fields41 are the fields that only KDBX 4.1 stores: group tags, previous parent
	// of groups and entries, names and times of custom icons, times of custom data
	// items, the settings-changed time, the quality-check flag.
	fields41 bool
	// itemCustomData is custom data on groups and entries.
	itemCustomData bool
	// publicCustomData is the PublicCustomData field of the header.
	publicCustomData bool
}

func limitsOf(v Version) limits {
	switch {
	case v.Major == 4 && v.Minor >= 1:
		return limits{fields41: true, itemCustomData: true, publicCustomData: true}
	case v.Major == 4:
		return limits{itemCustomData: true, publicCustomData: true}
	}
	return limits{}
}

// Fit returns a copy of the database as a file of the version holds it: what the
// version cannot store is removed (Losses lists it), the contents that no entry
// uses are left out of the pool (a file keeps only what entries refer to), and an
// empty generator is the default. A database that is read back from the file
// Encode wrote for the version is equal to it, which is how the output is
// verified. It does not decide whether the loss is allowed (SPEC §5).
func Fit(db *model.Database, version Version) *model.Database {
	db = db.Clone()
	if db.Meta.Generator == "" {
		db.Meta.Generator = DefaultGenerator
	}
	dropUnusedBinaries(db)
	lim := limitsOf(version)
	if !lim.publicCustomData {
		db.PublicCustomData = nil
	}
	if lim.fields41 && lim.itemCustomData {
		return db
	}
	if !lim.fields41 {
		db.Meta.SettingsChanged = time.Time{}
		for i := range db.Meta.CustomIcons {
			db.Meta.CustomIcons[i].Name, db.Meta.CustomIcons[i].LastModified = "", time.Time{}
		}
		clearCustomTimes(db.Meta.CustomData)
	}
	var fixEntry func(e *model.Entry)
	fixEntry = func(e *model.Entry) {
		if !lim.fields41 {
			e.QualityCheck, e.PreviousParent = nil, model.UUID{}
			clearCustomTimes(e.CustomData)
		}
		if !lim.itemCustomData {
			e.CustomData = nil
		}
		for _, h := range e.History {
			fixEntry(h)
		}
	}
	db.WalkGroups(func(g *model.Group) bool {
		if !lim.fields41 {
			g.Tags, g.PreviousParent = nil, model.UUID{}
			clearCustomTimes(g.CustomData)
		}
		if !lim.itemCustomData {
			g.CustomData = nil
		}
		for _, e := range g.Entries {
			fixEntry(e)
		}
		return true
	})
	return db
}

func clearCustomTimes(items []model.CustomDataItem) {
	for i := range items {
		items[i].LastModified = time.Time{}
	}
}

// dropUnusedBinaries removes the contents of the pool that no entry or history
// item refers to.
func dropUnusedBinaries(db *model.Database) {
	used := map[model.BinaryKey]bool{}
	mark := func(e *model.Entry) {
		for _, a := range e.Attachments {
			used[a.Binary] = true
		}
	}
	db.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
		mark(e)
		for _, h := range e.History {
			mark(h)
		}
		return true
	})
	for key, b := range db.Binaries {
		if !used[key] {
			b.Data.Wipe()
			delete(db.Binaries, key)
		}
	}
}
