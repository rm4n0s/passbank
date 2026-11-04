package model

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/secret"
)

// Fixed UUIDs, so that a failure names the same item every time.
var (
	uRoot   = mustUUID("00000000-0000-0000-0000-000000000001")
	uEmail  = mustUUID("00000000-0000-0000-0000-000000000002")
	uBin    = mustUUID("00000000-0000-0000-0000-000000000003")
	uBank   = mustUUID("00000000-0000-0000-0000-0000000000a1")
	uMail   = mustUUID("00000000-0000-0000-0000-0000000000a2")
	uIcon   = mustUUID("00000000-0000-0000-0000-0000000000c1")
	uDelete = mustUUID("00000000-0000-0000-0000-0000000000d1")
	uSecond = mustUUID("00000000-0000-0000-0000-0000000000a3")
)

// The secrets of the sample: none of them may ever appear in the output of Diff.
const (
	secretPassword   = "pw-SECRET-1"
	secretNotes      = "notes-SECRET-2"
	secretAttachment = "attachment-SECRET-3"
	secretRaw        = "raw-SECRET-4"
)

func mustUUID(s string) UUID {
	u, err := ParseUUID(s)
	if err != nil {
		panic(err)
	}
	return u
}

func ts(sec int) time.Time { return time.Date(2026, 1, 1, 0, 0, sec, 0, time.UTC) }

func field(key, value string, protected bool) Field {
	return Field{Key: key, Value: secret.New([]byte(value)), Protected: protected}
}

// sample returns a database that uses every part of the model.
func sample() *Database {
	attach := []byte(secretAttachment)
	key := BinaryKey{Hash: sha256.Sum256(attach), Protected: true}
	quality := false
	bank := &Entry{
		UUID: uBank, IconID: 3, CustomIcon: uIcon, ForegroundColor: "#112233", BackgroundColor: "#445566",
		OverrideURL: "firefox %1", Tags: []string{"money", "work"},
		Times: Times{
			Creation: ts(1), LastModification: ts(2), LastAccess: ts(3), Expiry: ts(4), Expires: true,
			UsageCount: 5, LocationChanged: ts(6),
		},
		QualityCheck:   &quality,
		PreviousParent: uEmail,
		Fields: []Field{
			field(FieldTitle, "My bank", false), field(FieldUserName, "ada", false),
			field(FieldPassword, secretPassword, true), field(FieldURL, "https://bank.example", false),
			field(FieldNotes, secretNotes, false), field("PIN", "1234", true),
		},
		Attachments: []Attachment{{Name: "statement.pdf", Binary: key}},
		AutoType: AutoType{
			Enabled: true, DataTransferObfuscation: 1, DefaultSequence: "{USERNAME}{TAB}{PASSWORD}{ENTER}",
			Associations: []AutoTypeAssociation{{Window: "My bank*", Sequence: "{PASSWORD}"}},
		},
		CustomData: []CustomDataItem{{Key: "k", Value: "v", LastModified: ts(7)}},
		History: []*Entry{{
			UUID: uBank, Times: Times{LastModification: ts(1)},
			Fields: []Field{field(FieldTitle, "Old bank", false), field(FieldPassword, "old-"+secretPassword, true)},
		}},
		Raw: Raw{
			Attrs: []RawAttr{{Name: "Extra", Value: "yes"}},
			Nodes: []*RawNode{{
				Name: "Plugin", Attrs: []RawAttr{{Name: "v", Value: "1"}},
				Children: []*RawNode{{Name: "Token", Text: secret.New([]byte(secretRaw))}},
			}},
		},
	}
	mail := &Entry{
		UUID: uMail, Times: Times{Creation: ts(1), LastModification: ts(2)},
		Fields: []Field{field(FieldTitle, "Mail", false), field(FieldPassword, "mail-pw", true)},
	}
	return &Database{
		Meta: Meta{
			Generator: "test", DatabaseName: "Sample", DatabaseNameChanged: ts(1), Description: "d", DescriptionChanged: ts(2),
			DefaultUserName: "ada", DefaultUserNameChanged: ts(3), SettingsChanged: ts(4), MaintenanceHistoryDays: 365,
			Color: "#ff0000", MasterKeyChanged: ts(5), MasterKeyChangeRec: -1, MasterKeyChangeForce: -1,
			MemoryProtection:  MemoryProtection{Password: true},
			RecycleBinEnabled: true, RecycleBinUUID: uBin, RecycleBinChanged: ts(6),
			EntryTemplatesGroup: uEmail, EntryTemplatesGroupChanged: ts(7),
			HistoryMaxItems: 10, HistoryMaxSize: 6 << 20, LastSelectedGroup: uEmail, LastTopVisibleGroup: uRoot,
			CustomIcons: []CustomIcon{{UUID: uIcon, Data: []byte("png"), Name: "icon", LastModified: ts(8)}},
			CustomData:  []CustomDataItem{{Key: "mk", Value: "mv", LastModified: ts(9)}},
			Raw:         Raw{Attrs: []RawAttr{{Name: "m", Value: "1"}}},
		},
		Root: &Group{
			UUID: uRoot, Name: "Root", Notes: "n", IconID: 48, Times: Times{Creation: ts(1)}, IsExpanded: true,
			DefaultAutoTypeSequence: "{PASSWORD}", EnableAutoType: Enabled, EnableSearching: Disabled,
			LastTopVisibleEntry: uBank, Tags: []string{"t"}, PreviousParent: uBin,
			CustomData: []CustomDataItem{{Key: "gk", Value: "gv"}},
			Groups: []*Group{
				{UUID: uEmail, Name: "Email", Entries: []*Entry{mail}},
				{UUID: uBin, Name: "Recycle Bin"},
			},
			Entries: []*Entry{bank, {UUID: uSecond, Fields: []Field{field(FieldTitle, "Second", false)}}},
		},
		DeletedObjects:   []DeletedObject{{UUID: uDelete, DeletionTime: ts(10)}},
		Binaries:         map[BinaryKey]*Binary{key: {Data: secret.New(attach)}},
		PublicCustomData: []PublicCustomDataItem{{Key: "p", Type: 0x0c, Data: []byte{1, 2, 3}}},
		Raw:              Raw{Nodes: []*RawNode{{Name: "Unknown", Text: secret.New([]byte("text"))}}},
	}
}

func TestParseUUID(t *testing.T) {
	good := []struct{ in, want string }{
		{"3f1c2a6e-5b2d-4d7e-9a3b-2c1d0e9f8a7b", "3f1c2a6e-5b2d-4d7e-9a3b-2c1d0e9f8a7b"},
		{"3F1C2A6E-5B2D-4D7E-9A3B-2C1D0E9F8A7B", "3f1c2a6e-5b2d-4d7e-9a3b-2c1d0e9f8a7b"},
		{"00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000"},
	}
	for _, tt := range good {
		u, err := ParseUUID(tt.in)
		if err != nil || u.String() != tt.want {
			t.Errorf("ParseUUID(%q) = %v, %v; want %q", tt.in, u, err, tt.want)
		}
	}
	bad := []string{
		"", "3f1c2a6e5b2d4d7e9a3b2c1d0e9f8a7b", "3f1c2a6e-5b2d-4d7e-9a3b-2c1d0e9f8a7", "3f1c2a6e-5b2d-4d7e-9a3b-2c1d0e9f8a7bb",
		"3f1c2a6e_5b2d_4d7e_9a3b_2c1d0e9f8a7b", "zf1c2a6e-5b2d-4d7e-9a3b-2c1d0e9f8a7b",
	}
	for _, in := range bad {
		_, err := ParseUUID(in)
		assertRoute(t, err, "ParseUUID.UUIDInvalid")
	}
	if !(UUID{}).IsZero() || mustUUID("00000000-0000-0000-0000-000000000001").IsZero() {
		t.Error("IsZero is wrong")
	}
}

// assertRoute fails the test unless err is an *errors.Error whose route contains route.
func assertRoute(t *testing.T, err error, route string) {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
}

func TestDiffOfEqualDatabases(t *testing.T) {
	a := sample()
	if lines := Diff(a, sample()); len(lines) != 0 {
		t.Errorf("two samples differ: %q", lines)
	}
	if lines := Diff(a, a.Clone()); len(lines) != 0 {
		t.Errorf("a database and its clone differ: %q", lines)
	}
	if Diff(nil, nil) != nil {
		t.Error("two nil databases are equal")
	}
	if lines := Diff(a, nil); len(lines) != 1 {
		t.Errorf("Diff(a, nil) = %q, want one line", lines)
	}
}

func TestDiffFindsEveryKindOfDifference(t *testing.T) {
	bank := func(d *Database) *Entry { return d.Root.Entries[0] }
	tests := []struct {
		name   string
		mutate func(d *Database)
		want   string // a substring of one line of the result
	}{
		{"meta name", func(d *Database) { d.Meta.DatabaseName = "x" }, "Meta/DatabaseName: differs"},
		{"meta time", func(d *Database) { d.Meta.SettingsChanged = ts(50) }, "Meta/SettingsChanged: differs"},
		{"meta protection", func(d *Database) { d.Meta.MemoryProtection.URL = true }, "Meta/MemoryProtection: differs"},
		{"meta history limit", func(d *Database) { d.Meta.HistoryMaxSize = 1 }, "Meta/HistoryMaxSize: differs"},
		{"meta icon data", func(d *Database) { d.Meta.CustomIcons[0].Data = []byte("x") }, "Meta/CustomIcons[0]/Data: differs"},
		{"meta icon count", func(d *Database) { d.Meta.CustomIcons = nil }, "Meta/CustomIcons: 1 icons and 0 icons"},
		{"meta custom data", func(d *Database) { d.Meta.CustomData[0].Value = "x" }, "Meta/CustomData[0]/Value: differs"},
		{"meta raw", func(d *Database) { d.Meta.Raw.Attrs[0].Value = "2" }, "Meta/Raw/Attrs: differs"},
		{"group name", func(d *Database) { d.Root.Groups[0].Name = "x" }, "group " + uEmail.String() + "/Name: differs"},
		{"group tri-state", func(d *Database) { d.Root.EnableSearching = Inherit }, "Root/EnableSearching: differs"},
		{"group tags", func(d *Database) { d.Root.Tags = nil }, "Root/Tags: differs"},
		{"group time", func(d *Database) { d.Root.Times.Expires = true }, "Root/Times/Expires: differs"},
		{"group removed", func(d *Database) { d.Root.Groups = d.Root.Groups[:1] }, "group " + uBin.String() + ": only in the first database"},
		{"group added", func(d *Database) {
			d.Root.Groups = append(d.Root.Groups, &Group{UUID: mustUUID("00000000-0000-0000-0000-0000000000ff")})
		}, "only in the second database"},
		{"group order", func(d *Database) { d.Root.Groups[0], d.Root.Groups[1] = d.Root.Groups[1], d.Root.Groups[0] }, "Root/group order: differs"},
		{"entry moved", func(d *Database) {
			e := d.Root.Groups[0].Entries[0]
			d.Root.Groups[0].Entries = nil
			d.Root.Entries = append(d.Root.Entries, e)
		}, "entry " + uMail.String() + ": only in the second database"},
		{"entry order", func(d *Database) {
			d.Root.Entries[0], d.Root.Entries[1] = d.Root.Entries[1], d.Root.Entries[0]
		}, "Root/entry order: differs"},
		{"entry password", func(d *Database) { bank(d).Fields[2].Value = secret.New([]byte("other")) }, "Fields/Password: differs"},
		{"entry protected flag", func(d *Database) { bank(d).Fields[2].Protected = false }, "Fields/Password protected: differs"},
		{"entry field key", func(d *Database) { bank(d).Fields[5].Key = "PIN2" }, `key differs ("PIN" and "PIN2")`},
		{"entry field count", func(d *Database) { bank(d).Fields = bank(d).Fields[:5] }, "Fields: 6 fields and 5 fields"},
		{"entry tags", func(d *Database) { bank(d).Tags = []string{"work", "money"} }, "Tags: differs"},
		{"entry colors", func(d *Database) { bank(d).BackgroundColor = "" }, "BackgroundColor: differs"},
		{"entry quality unset", func(d *Database) { bank(d).QualityCheck = nil }, "QualityCheck: differs"},
		{"entry quality value", func(d *Database) { q := true; bank(d).QualityCheck = &q }, "QualityCheck: differs"},
		{"entry times", func(d *Database) { bank(d).Times.UsageCount++ }, "Times/UsageCount: differs"},
		{"entry attachment name", func(d *Database) { bank(d).Attachments[0].Name = "x" }, "Attachments[0]/Name: differs"},
		{"entry attachment content ref", func(d *Database) { bank(d).Attachments[0].Binary.Protected = false }, "Attachments[0]/Binary: differs"},
		{"entry attachment count", func(d *Database) { bank(d).Attachments = nil }, "Attachments: 1 attachments and 0 attachments"},
		{"entry autotype", func(d *Database) { bank(d).AutoType.Associations[0].Sequence = "x" }, "AutoType: differs"},
		{"entry custom data", func(d *Database) { bank(d).CustomData = nil }, "CustomData: 1 items and 0 items"},
		{"entry raw text", func(d *Database) { bank(d).Raw.Nodes[0].Children[0].Text = secret.New([]byte("x")) }, "Raw/Nodes[0]/Children[0]/Text: differs"},
		{"entry raw child count", func(d *Database) { bank(d).Raw.Nodes[0].Children = nil }, "Raw/Nodes[0]/Children: 1 children and 0 children"},
		{"entry raw node name", func(d *Database) { bank(d).Raw.Nodes[0].Name = "x" }, "Raw/Nodes[0]/Name: differs"},
		{"history count", func(d *Database) { bank(d).History = nil }, "History: 1 versions and 0 versions"},
		{"history field", func(d *Database) { bank(d).History[0].Fields[1].Value = secret.New([]byte("x")) }, "History[0]/Fields/Password: differs"},
		{"deleted count", func(d *Database) { d.DeletedObjects = nil }, "DeletedObjects: 1 objects and 0 objects"},
		{"deleted time", func(d *Database) { d.DeletedObjects[0].DeletionTime = ts(99) }, "DeletedObjects[0]/DeletionTime: differs"},
		{"binary content", func(d *Database) {
			for _, b := range d.Binaries {
				b.Data = secret.New([]byte("x"))
			}
		}, "content differs"},
		{"binary removed", func(d *Database) { d.Binaries = nil }, "only in the first database"},
		{"binary added", func(d *Database) {
			d.Binaries[BinaryKey{Hash: [32]byte{1}}] = &Binary{Data: secret.New([]byte("x"))}
		}, "only in the second database"},
		{"public custom data", func(d *Database) { d.PublicCustomData[0].Data = []byte{9} }, "PublicCustomData[0]/Data: differs"},
		{"public custom data count", func(d *Database) { d.PublicCustomData = nil }, "PublicCustomData: 1 items and 0 items"},
		{"root raw", func(d *Database) { d.Raw.Nodes = nil }, "Raw/Nodes: 1 nodes and 0 nodes"},
		{"no root", func(d *Database) { d.Root = nil }, "only one side has the group"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := sample(), sample()
			tt.mutate(b)
			lines := Diff(a, b)
			found := false
			for _, line := range lines {
				if strings.Contains(line, tt.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("no line contains %q; got:\n%s", tt.want, strings.Join(lines, "\n"))
			}
			// Both ways round, and never a value.
			if back := Diff(b, a); len(back) == 0 {
				t.Error("the reverse comparison finds nothing")
			}
			for _, line := range append(lines, Diff(b, a)...) {
				for _, s := range []string{secretPassword, secretNotes, secretAttachment, secretRaw, "other"} {
					if strings.Contains(line, s) {
						t.Errorf("a line shows a value (%q): %s", s, line)
					}
				}
			}
		})
	}
}

func TestCloneIsDeep(t *testing.T) {
	a := sample()
	c := a.Clone()
	c.Root.Entries[0].Fields[2].Value.Wipe()
	c.Root.Entries[0].Tags[0] = "x"
	c.Root.Entries[0].History[0].Fields[0].Key = "x"
	c.Root.Groups[0].Name = "x"
	c.Meta.CustomIcons[0].Data[0] = 'x'
	c.PublicCustomData[0].Data[0] = 9
	for _, b := range c.Binaries {
		b.Data.Wipe()
	}
	*c.Root.Entries[0].QualityCheck = true
	c.Root.Entries[0].Raw.Nodes[0].Children[0].Text.Wipe()
	if lines := Diff(a, sample()); len(lines) != 0 {
		t.Errorf("changing the clone changed the original: %q", lines)
	}
	if (*Database)(nil).Clone() != nil {
		t.Error("the clone of nil is nil")
	}
}

func TestWipe(t *testing.T) {
	d := sample()
	pw := d.Root.Entries[0].Fields[2].Value
	hist := d.Root.Entries[0].History[0].Fields[1].Value
	raw := d.Root.Entries[0].Raw.Nodes[0].Children[0].Text
	var bin secret.Bytes
	for _, b := range d.Binaries {
		bin = b.Data
	}
	d.Wipe()
	for name, s := range map[string]secret.Bytes{"password": pw, "history password": hist, "raw text": raw, "attachment": bin} {
		if s.Len() != 0 {
			t.Errorf("%s was not wiped", name)
		}
	}
	(*Database)(nil).Wipe()
}

func TestTree(t *testing.T) {
	d := sample()
	var groups []string
	d.WalkGroups(func(g *Group) bool { groups = append(groups, g.Name); return true })
	if got := strings.Join(groups, ","); got != "Root,Email,Recycle Bin" {
		t.Errorf("WalkGroups order = %s", got)
	}
	var entries []string
	d.WalkEntries(func(g *Group, e *Entry) bool { entries = append(entries, g.Name+":"+e.Title()); return true })
	if got := strings.Join(entries, ","); got != "Root:My bank,Root:Second,Email:Mail" {
		t.Errorf("WalkEntries order = %s", got)
	}
	count := 0
	d.WalkGroups(func(*Group) bool { count++; return false })
	if count != 1 {
		t.Errorf("WalkGroups did not stop: %d calls", count)
	}
	count = 0
	d.WalkEntries(func(*Group, *Entry) bool { count++; return false })
	if count != 1 {
		t.Errorf("WalkEntries did not stop: %d calls", count)
	}

	if g, parent, ok := d.FindGroup(uEmail); !ok || g.Name != "Email" || parent.UUID != uRoot {
		t.Errorf("FindGroup(Email) = %v, %v, %v", g, parent, ok)
	}
	if g, parent, ok := d.FindGroup(uRoot); !ok || g.Name != "Root" || parent != nil {
		t.Errorf("FindGroup(root) = %v, %v, %v", g, parent, ok)
	}
	if _, _, ok := d.FindGroup(uDelete); ok {
		t.Error("FindGroup found a group that is not there")
	}
	if e, parent, ok := d.FindEntry(uMail); !ok || e.Title() != "Mail" || parent.UUID != uEmail {
		t.Errorf("FindEntry(mail) = %v, %v, %v", e, parent, ok)
	}
	if _, _, ok := d.FindEntry(uDelete); ok {
		t.Error("FindEntry found an entry that is not there")
	}
	if p, ok := d.GroupPath(uEmail); !ok || p != "Root/Email" {
		t.Errorf("GroupPath(Email) = %q, %v", p, ok)
	}
	if p, ok := d.GroupPath(uRoot); !ok || p != "Root" {
		t.Errorf("GroupPath(root) = %q, %v", p, ok)
	}
	if _, ok := d.GroupPath(uDelete); ok {
		t.Error("GroupPath found a group that is not there")
	}
	empty := &Database{}
	if _, _, ok := empty.FindGroup(uRoot); ok {
		t.Error("an empty database has no groups")
	}
	if _, ok := empty.GroupPath(uRoot); ok {
		t.Error("an empty database has no paths")
	}
}

func TestEntryFields(t *testing.T) {
	e := sample().Root.Entries[0]
	if f, ok := e.Field(FieldPassword); !ok || f.Text() != secretPassword || !f.Protected {
		t.Errorf("Field(Password) = %v, %v", f, ok)
	}
	if _, ok := e.Field("nope"); ok {
		t.Error("Field found a field that is not there")
	}
	if e.Title() != "My bank" || (&Entry{}).Title() != "" {
		t.Errorf("Title = %q", e.Title())
	}
	if got := fmtSecret(e.Fields[2]); strings.Contains(got, secretPassword) {
		t.Errorf("a printed field shows its value: %s", got)
	}
}

// fmtSecret prints a field with every verb that shows contents.
func fmtSecret(f Field) string {
	return strings.Join([]string{
		sprint("%v", f), sprint("%+v", f), sprint("%#v", f), sprint("%s", f), sprint("%q", f),
	}, " ")
}

func sprint(format string, v any) string { return fmt.Sprintf(format, v) }
