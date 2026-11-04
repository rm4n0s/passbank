package kdbx

import (
	"strconv"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// The kinds of Loss.
const (
	LossSettingsChanged    = "SettingsChanged"
	LossCustomIconName     = "CustomIconName"
	LossCustomIconModified = "CustomIconModified"
	LossCustomDataModified = "CustomDataModified"
	LossGroupTags          = "GroupTags"
	LossGroupPreviousGroup = "GroupPreviousParent"
	LossEntryPreviousGroup = "EntryPreviousParent"
	LossEntryQualityCheck  = "EntryQualityCheck"
	LossGroupCustomData    = "GroupCustomData"
	LossEntryCustomData    = "EntryCustomData"
	LossPublicCustomData   = "PublicCustomData"
)

// Loss is something a KDBX version has no place for, so that writing the database
// in that version would drop it (SPEC §5). It names the item and what is lost,
// never a value.
type Loss struct {
	// Kind is one of the Loss constants.
	Kind string
	// Item is the path of the group, or of the entry (the group's path and the
	// entry's title), or "database" for the settings of the database.
	Item string
	// UUID is the group, entry or icon that is concerned; zero for the database.
	UUID model.UUID
	// What says what is lost, in words.
	What string
}

// Losses lists what the database holds that the version cannot store, in
// document order. It is empty when nothing is lost.
func Losses(db *model.Database, version Version) []Loss {
	lim := limitsOf(version)
	var out []Loss
	add := func(kind, item string, u model.UUID, what string) { out = append(out, Loss{kind, item, u, what}) }
	m := &db.Meta
	if !lim.fields41 {
		if !m.SettingsChanged.IsZero() {
			add(LossSettingsChanged, "database", model.UUID{}, "the time the settings changed")
		}
		for _, ic := range m.CustomIcons {
			if ic.Name != "" {
				add(LossCustomIconName, "custom icon "+ic.UUID.String(), ic.UUID, "the name of a custom icon")
			}
			if !ic.LastModified.IsZero() {
				add(LossCustomIconModified, "custom icon "+ic.UUID.String(), ic.UUID, "the modification time of a custom icon")
			}
		}
		if n := countCustomTimes(m.CustomData); n > 0 {
			add(LossCustomDataModified, "database", model.UUID{}, plural(n, "modification time of a custom data item", "modification times of custom data items"))
		}
	}
	if !lim.publicCustomData && len(db.PublicCustomData) > 0 {
		add(LossPublicCustomData, "database", model.UUID{}, plural(len(db.PublicCustomData), "public custom data item", "public custom data items"))
	}
	var walk func(g *model.Group, path string)
	walk = func(g *model.Group, path string) {
		path = joinPath(path, g.Name)
		if !lim.fields41 {
			if len(g.Tags) > 0 {
				add(LossGroupTags, path, g.UUID, plural(len(g.Tags), "tag of a group", "tags of a group"))
			}
			if !g.PreviousParent.IsZero() {
				add(LossGroupPreviousGroup, path, g.UUID, "the previous parent of a group")
			}
		}
		if !lim.itemCustomData && len(g.CustomData) > 0 {
			add(LossGroupCustomData, path, g.UUID, plural(len(g.CustomData), "custom data item of a group", "custom data items of a group"))
		} else if !lim.fields41 {
			if n := countCustomTimes(g.CustomData); n > 0 {
				add(LossCustomDataModified, path, g.UUID, plural(n, "modification time of a custom data item", "modification times of custom data items"))
			}
		}
		for _, e := range g.Entries {
			entryLosses(e, joinPath(path, e.Title()), lim, add)
		}
		for _, c := range g.Groups {
			walk(c, path)
		}
	}
	if db.Root != nil {
		walk(db.Root, "")
	}
	return out
}

// entryLosses reports what an entry and its history hold that the version lacks.
func entryLosses(e *model.Entry, item string, lim limits, add func(kind, item string, u model.UUID, what string)) {
	var quality, previous, custom, customTimes int
	count := func(x *model.Entry) {
		if x.QualityCheck != nil {
			quality++
		}
		if !x.PreviousParent.IsZero() {
			previous++
		}
		custom += len(x.CustomData)
		customTimes += countCustomTimes(x.CustomData)
	}
	count(e)
	for _, h := range e.History {
		count(h)
	}
	if !lim.fields41 {
		if quality > 0 {
			add(LossEntryQualityCheck, item, e.UUID, "the quality-check flag"+inHistory(quality, e))
		}
		if previous > 0 {
			add(LossEntryPreviousGroup, item, e.UUID, "the previous parent of an entry"+inHistory(previous, e))
		}
	}
	if !lim.itemCustomData && custom > 0 {
		add(LossEntryCustomData, item, e.UUID, plural(custom, "custom data item of an entry", "custom data items of an entry")+" (history included)")
	} else if !lim.fields41 && customTimes > 0 {
		add(LossCustomDataModified, item, e.UUID, plural(customTimes, "modification time of a custom data item", "modification times of custom data items"))
	}
}

func inHistory(n int, e *model.Entry) string {
	if len(e.History) > 0 {
		return " (history included)"
	}
	return ""
}

func countCustomTimes(items []model.CustomDataItem) int {
	n := 0
	for _, it := range items {
		if !it.LastModified.IsZero() {
			n++
		}
	}
	return n
}

func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// Supports checks that the version can hold the encryption settings (SPEC §5):
// KDBX 2.x and 3.x have the AES-KDF only and the outer ciphers AES-256 and
// Twofish; KDBX 4 has every cipher and KDF Passbank knows. It is
// EncryptionNotInVersion when not.
func Supports(version Version, settings EncryptionSettings) error {
	if version.Major >= 4 {
		return nil
	}
	if settings.KDF.ID != crypto.KDFAES {
		return errors.New("EncryptionNotInVersion", "KDBX "+version.String()+" has only the AES-KDF; choose other encryption settings",
			"version", version.String(), "kdf", settings.KDF.Name())
	}
	if settings.CipherID != crypto.CipherAES256 && settings.CipherID != crypto.CipherTwofish {
		name := "unknown"
		if c, err := crypto.LookupCipher(settings.CipherID); err == nil {
			name = c.Name()
		}
		return errors.New("EncryptionNotInVersion", "KDBX "+version.String()+" has only the ciphers AES-256 and Twofish; choose other encryption settings",
			"version", version.String(), "cipher", name)
	}
	return nil
}
