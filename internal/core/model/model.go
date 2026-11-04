package model

import (
	"time"

	"github.com/rm4n0s/passbank/internal/core/secret"
)

// Database is a whole KeePass database, as read from any KDBX version.
type Database struct {
	// Meta holds the settings of the database.
	Meta Meta
	// Root is the root group; it is never nil in a database that was read.
	Root *Group
	// DeletedObjects records the entries and groups that were deleted, so that a
	// merge does not bring them back.
	DeletedObjects []DeletedObject
	// Binaries is the pool of attachment contents; entries refer to it by BinaryKey.
	Binaries map[BinaryKey]*Binary
	// PublicCustomData is the header field of KDBX 4 that plugins read without the
	// key; its values are opaque.
	PublicCustomData []PublicCustomDataItem
	// Raw is what the file holds around the database that the model does not know.
	Raw Raw
}

// Meta holds the settings of a database (the Meta element of the XML).
type Meta struct {
	// Generator names the program that wrote the file.
	Generator string

	// DatabaseName and its change time.
	DatabaseName        string
	DatabaseNameChanged time.Time
	// Description and its change time.
	Description        string
	DescriptionChanged time.Time
	// DefaultUserName and its change time.
	DefaultUserName        string
	DefaultUserNameChanged time.Time
	// SettingsChanged is the change time of the settings (KDBX 4.1).
	SettingsChanged time.Time

	// MaintenanceHistoryDays is how long history is kept, in days.
	MaintenanceHistoryDays int
	// Color is the database color, "#RRGGBB" or empty.
	Color string

	// MasterKeyChanged is when the key last changed; the recommended and forced
	// change intervals are in days, -1 for never.
	MasterKeyChanged     time.Time
	MasterKeyChangeRec   int64
	MasterKeyChangeForce int64

	// MemoryProtection says which standard fields are protected by default.
	MemoryProtection MemoryProtection

	// RecycleBinEnabled, RecycleBinUUID and RecycleBinChanged describe the recycle
	// bin group.
	RecycleBinEnabled bool
	RecycleBinUUID    UUID
	RecycleBinChanged time.Time
	// EntryTemplatesGroup is the group of entry templates, and when it changed.
	EntryTemplatesGroup        UUID
	EntryTemplatesGroupChanged time.Time

	// HistoryMaxItems and HistoryMaxSize limit the history of an entry; -1 means
	// no limit.
	HistoryMaxItems int
	HistoryMaxSize  int64

	// LastSelectedGroup and LastTopVisibleGroup remember the state of the window.
	LastSelectedGroup   UUID
	LastTopVisibleGroup UUID

	// CustomIcons are the icons that entries and groups refer to by UUID.
	CustomIcons []CustomIcon
	// CustomData is the data that plugins keep in the database.
	CustomData []CustomDataItem

	// Raw is the Meta XML that the model does not know.
	Raw Raw
}

// MemoryProtection says which of the standard fields are protected by default.
type MemoryProtection struct {
	Title, UserName, Password, URL, Notes bool
}

// CustomIcon is an icon image that entries and groups can use.
type CustomIcon struct {
	UUID UUID
	// Data is the image file (PNG).
	Data []byte
	// Name and LastModified are KDBX 4.1 fields.
	Name         string
	LastModified time.Time
}

// CustomDataItem is one key and value that a plugin keeps.
type CustomDataItem struct {
	Key, Value string
	// LastModified is a KDBX 4.1 field; zero when absent.
	LastModified time.Time
}

// PublicCustomDataItem is one value of the PublicCustomData header field of KDBX
// 4. Value is the VariantDictionary value as it is stored: Type is the type byte
// and Data its bytes.
type PublicCustomDataItem struct {
	Key  string
	Type byte
	Data []byte
}

// DeletedObject records that an entry or group was deleted.
type DeletedObject struct {
	UUID         UUID
	DeletionTime time.Time
}

// TriState is a setting that a group can leave to its parent.
type TriState int8

// The values of TriState.
const (
	// Inherit leaves the setting to the parent group; it is the zero value.
	Inherit TriState = iota
	// Enabled turns the setting on.
	Enabled
	// Disabled turns the setting off.
	Disabled
)

// Times are the timestamps of an entry or a group. KDBX stores them in UTC with
// second precision, and so does the model.
type Times struct {
	Creation         time.Time
	LastModification time.Time
	LastAccess       time.Time
	// Expiry is the time the item expires, when Expires is set.
	Expiry  time.Time
	Expires bool
	// UsageCount is how often the item was used.
	UsageCount int64
	// LocationChanged is when the item was last moved to another group.
	LocationChanged time.Time
}

// Group is a folder of entries and groups.
type Group struct {
	UUID  UUID
	Name  string
	Notes string
	// IconID is the number of a standard icon; CustomIcon, when not zero, is the
	// UUID of a custom icon, which wins.
	IconID     int
	CustomIcon UUID
	Times      Times
	// IsExpanded is whether the group is open in a tree view.
	IsExpanded bool
	// DefaultAutoTypeSequence is the auto-type sequence of the group's entries.
	DefaultAutoTypeSequence string
	// EnableAutoType and EnableSearching let a group leave the setting to its parent.
	EnableAutoType  TriState
	EnableSearching TriState
	// LastTopVisibleEntry remembers the state of the window.
	LastTopVisibleEntry UUID
	// Tags and PreviousParent are KDBX 4.1 fields.
	Tags           []string
	PreviousParent UUID
	CustomData     []CustomDataItem

	// Groups and Entries are the children, in file order.
	Groups  []*Group
	Entries []*Entry

	// Raw is the XML of the group that the model does not know.
	Raw Raw
}

// Entry is one record: a title, a user name, a password and more.
type Entry struct {
	UUID UUID
	// IconID and CustomIcon are as for Group.
	IconID     int
	CustomIcon UUID
	// ForegroundColor and BackgroundColor are "#RRGGBB" or empty.
	ForegroundColor string
	BackgroundColor string
	// OverrideURL opens the entry's URL with another command.
	OverrideURL string
	// Tags are the labels of the entry.
	Tags  []string
	Times Times
	// QualityCheck is whether the password's quality is checked; nil when the file
	// does not say (KDBX 4.1 stores it only when it is off).
	QualityCheck *bool
	// PreviousParent is a KDBX 4.1 field.
	PreviousParent UUID
	// Fields are the standard fields (Title, UserName, Password, URL, Notes) and the
	// custom ones, in file order.
	Fields []Field
	// Attachments are the files attached to the entry.
	Attachments []Attachment
	AutoType    AutoType
	CustomData  []CustomDataItem
	// History holds the earlier versions of the entry, oldest first. They have no
	// history of their own.
	History []*Entry

	// Raw is the XML of the entry that the model does not know.
	Raw Raw
}

// The keys of the standard fields.
const (
	FieldTitle    = "Title"
	FieldUserName = "UserName"
	FieldPassword = "Password"
	FieldURL      = "URL"
	FieldNotes    = "Notes"
)

// Field is one string of an entry.
type Field struct {
	Key string
	// Value is the content, held as a secret whether or not the field is protected,
	// so that no value is printed or logged by accident.
	Value secret.Bytes
	// Protected is the flag the file stored: whether clients mask the value.
	Protected bool
}

// Text returns the value as a string. A protected value is revealed only when
// the user asks to see it; do not log or keep the result.
func (f Field) Text() string { return string(f.Value.Reveal()) }

// Field returns the field with the key, or false.
func (e *Entry) Field(key string) (Field, bool) {
	for _, f := range e.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// Title returns the title of the entry, "" when it has none.
func (e *Entry) Title() string {
	f, _ := e.Field(FieldTitle)
	return f.Text()
}

// Attachment is a file attached to an entry; its content is in the pool of the
// database.
type Attachment struct {
	// Name is the file name.
	Name string
	// Binary is the key of the content in Database.Binaries.
	Binary BinaryKey
}

// BinaryKey identifies a content in the pool: the SHA-256 of the bytes and the
// protection flag. Two entries that attach the same bytes share one content.
type BinaryKey struct {
	Hash      [32]byte
	Protected bool
}

// Binary is the content of an attachment.
type Binary struct {
	Data secret.Bytes
}

// AutoType holds the auto-type settings of an entry.
type AutoType struct {
	Enabled                 bool
	DataTransferObfuscation int
	DefaultSequence         string
	Associations            []AutoTypeAssociation
}

// AutoTypeAssociation links a window title pattern to a key sequence.
type AutoTypeAssociation struct {
	Window   string
	Sequence string
}

// Raw is what a file holds that the model has no field for, kept so that the
// writer can put it back (SPEC §4.4).
type Raw struct {
	// Attrs are the attributes of the object's own element that the model does not
	// know.
	Attrs []RawAttr
	// Nodes are the child elements that the model does not know, in file order.
	// Comments and processing instructions are not kept.
	Nodes []*RawNode
}

// RawAttr is one XML attribute.
type RawAttr struct {
	Name, Value string
}

// RawNode is an XML element that the model does not know. Its text is a secret
// because the element may have been protected by the inner stream, which the
// reader must decrypt to keep the stream in step and the writer must encrypt again.
type RawNode struct {
	Name  string
	Attrs []RawAttr
	// Text is the character data of the element.
	Text secret.Bytes
	// Protected is whether Text was protected by the inner stream (the
	// Protected="True" attribute, which is not repeated in Attrs).
	Protected bool
	Children  []*RawNode
}
