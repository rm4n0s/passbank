package kdbx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/testenv"
)

// roundTripXML writes db as the version, with a new inner stream, and reads the
// XML back with the reader. It returns the model read.
func roundTripXML(t *testing.T, db *model.Database, version Version, compress bool) (*model.Database, string) {
	t.Helper()
	innerID := uint32(crypto.InnerChaCha20)
	if version.Major < 4 {
		innerID = crypto.InnerSalsa20
	}
	key := []byte("a round trip key, 32 bytes long!")
	stream, err := crypto.NewInnerStream(innerID, key)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("a header"))
	xmlBytes, binaries, err := marshalXML(db, xmlSpec{Version: version, Stream: stream, HeaderHash: hash[:], CompressBinaries: compress})
	if err != nil {
		t.Fatal(err)
	}
	c := &container{
		Header: &Header{Version: version, PublicCustomData: db.PublicCustomData}, XML: xmlBytes,
		InnerStreamID: innerID, InnerStreamKey: secret.New(key), Binaries: binaries,
	}
	if version.Major < 4 {
		c.HeaderHash = hash[:]
	}
	got, err := parseXML(context.Background(), c)
	if err != nil {
		t.Fatalf("the XML that was written does not read back: %v\n%s", err, xmlBytes)
	}
	return got, string(xmlBytes)
}

// strip41 removes the fields that only KDBX 4.1 stores.
func strip41(db *model.Database) *model.Database {
	db = db.Clone()
	db.Meta.SettingsChanged = time.Time{}
	for i := range db.Meta.CustomIcons {
		db.Meta.CustomIcons[i].Name, db.Meta.CustomIcons[i].LastModified = "", time.Time{}
	}
	for i := range db.Meta.CustomData {
		db.Meta.CustomData[i].LastModified = time.Time{}
	}
	var fixEntry func(e *model.Entry)
	fixEntry = func(e *model.Entry) {
		e.QualityCheck, e.PreviousParent = nil, model.UUID{}
		for i := range e.CustomData {
			e.CustomData[i].LastModified = time.Time{}
		}
		for _, h := range e.History {
			fixEntry(h)
		}
	}
	db.WalkGroups(func(g *model.Group) bool {
		g.Tags, g.PreviousParent = nil, model.UUID{}
		for i := range g.CustomData {
			g.CustomData[i].LastModified = time.Time{}
		}
		for _, e := range g.Entries {
			fixEntry(e)
		}
		return true
	})
	return db
}

func TestXMLWriterRoundTripOfFixtures(t *testing.T) {
	for _, tt := range []struct {
		file, password, keyFile string
	}{
		{"k4-empty.kdbx", "fixture-min", ""},
		{"k4-rich.kdbx", "fixture-rich", ""},
		{"k4-public-custom-data.kdbx", "fixture-rich", ""},
		{"k4-unknown-xml.kdbx", "fixture-rich", ""},
		{"k41-fields.kdbx", "fixture-rich", ""},
		{"k31-rich.kdbx", "fixture-rich", ""},
		{"ext/rs-password.kdbx", "demopass", ""},
		{"ext/rs-attachments.kdbx", "demopass", ""},
		{"ext/rs-chacha20-inner.kdbx", "password", ""},
		{"ext/go-protected-binary.kdbx", "123", ""},
		{"ext/web-cyrillic.kdbx", "пароль", ""},
	} {
		t.Run(tt.file, func(t *testing.T) {
			db, _, err := Open(context.Background(), testenv.Fixture(t, tt.file), credsOf(t, tt.password, tt.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Wipe()
			before := db.Clone()
			// KDBX 4.1: everything is stored.
			got, _ := roundTripXML(t, db, Version{4, 1}, false)
			if diff := model.Diff(db, got); len(diff) != 0 {
				t.Errorf("4.1 round trip:\n%s", strings.Join(diff, "\n"))
			}
			// KDBX 4.0: everything but the 4.1 fields.
			got, _ = roundTripXML(t, db, Version{4, 0}, false)
			if diff := model.Diff(strip41(db), got); len(diff) != 0 {
				t.Errorf("4.0 round trip:\n%s", strings.Join(diff, "\n"))
			}
			// KDBX 3.1 and 3.0: everything but the 4.1 fields and the custom data of
			// groups and entries (the files of the fixtures have none).
			for _, v := range []Version{{3, 1}, {3, 0}, {2, 0}} {
				for _, compress := range []bool{false, true} {
					got, _ = roundTripXML(t, db, v, compress)
					if diff := model.Diff(strip41(db), got); len(diff) != 0 {
						t.Errorf("%s (compressed %v) round trip:\n%s", v, compress, strings.Join(diff, "\n"))
					}
				}
			}
			if diff := model.Diff(before, db); len(diff) != 0 {
				t.Errorf("writing changed the database:\n%s", strings.Join(diff, "\n"))
			}
		})
	}
}

func TestXMLWriterFormats(t *testing.T) {
	db, _, err := Open(context.Background(), testenv.Fixture(t, "k4-rich.kdbx"), credsOf(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	_, x4 := roundTripXML(t, db, Version{4, 0}, false)
	_, x3 := roundTripXML(t, db, Version{3, 1}, true)
	if strings.Contains(x4, "<Binaries>") || strings.Contains(x4, "<HeaderHash>") || !strings.Contains(x4, `<Value Ref="0"/>`) {
		t.Error("KDBX 4 XML has the dialect of KDBX 3 or lacks the reference")
	}
	if !strings.Contains(x4, "<CreationTime>AAAAAAAAAAA=") && !strings.Contains(x4, "<CreationTime>") {
		t.Error("no times")
	}
	if strings.Contains(x4, "2026-01-01T00:00:01Z") {
		t.Error("KDBX 4 XML has an ISO time")
	}
	for _, want := range []string{"<HeaderHash>", `<Binary ID="0" Compressed="True" Protected="True">`, "2026-01-01T00:00:01Z", `<Value Ref="0"/>`} {
		if !strings.Contains(x3, want) {
			t.Errorf("KDBX 3 XML lacks %q", want)
		}
	}
	for _, secret := range []string{"bank-password-1", "older-bank-password", "4711", "attachment-bytes"} {
		if strings.Contains(x4, secret) || strings.Contains(x3, secret) {
			t.Errorf("the XML shows %q", secret)
		}
	}
	// Plain fields stay plain.
	if !strings.Contains(x4, "<Value>https://example.test/bank</Value>") {
		t.Error("a plain value is not plain")
	}
}

func TestXMLWriterEscaping(t *testing.T) {
	nasty := "a & b < c > d \" e ' f \n g \r h \t i ünï 鍵 ]]> <![CDATA[x]]>"
	db := &model.Database{
		Meta: model.Meta{Generator: "test", DatabaseName: nasty, Description: nasty, Raw: model.Raw{
			Attrs: []model.RawAttr{{Name: "a", Value: nasty}},
			Nodes: []*model.RawNode{{Name: "X", Attrs: []model.RawAttr{{Name: "q", Value: nasty}}, Text: secret.New([]byte(nasty)),
				Children: []*model.RawNode{{Name: "Y", Text: secret.New([]byte(nasty))}}}}}},
		Binaries: map[model.BinaryKey]*model.Binary{},
		Root: &model.Group{UUID: uuid(1), Name: nasty, Notes: nasty, Entries: []*model.Entry{{
			UUID: uuid(2), Tags: []string{"a b", "c&d"}, OverrideURL: nasty,
			Fields: []model.Field{
				{Key: nasty, Value: secret.New([]byte(nasty))},
				{Key: "Secret", Value: secret.New([]byte(nasty)), Protected: true},
				{Key: "Empty secret", Value: secret.New(nil), Protected: true},
			},
			AutoType: model.AutoType{DefaultSequence: nasty, Associations: []model.AutoTypeAssociation{{Window: nasty, Sequence: nasty}}},
		}}},
	}
	for _, v := range []Version{{4, 0}, {4, 1}, {3, 1}} {
		got, _ := roundTripXML(t, db, v, false)
		if diff := model.Diff(db, got); len(diff) != 0 {
			t.Errorf("%s:\n%s", v, strings.Join(diff, "\n"))
		}
	}
}

func TestXMLWriterTimes(t *testing.T) {
	odd := []time.Time{{}, time.Date(1, 1, 1, 0, 0, 1, 0, time.UTC), time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 5, 23, 59, 59, 0, time.UTC), time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)}
	for _, v := range []Version{{4, 0}, {3, 1}} {
		for _, ts := range odd {
			db := &model.Database{Meta: model.Meta{DatabaseNameChanged: ts}, Root: &model.Group{UUID: uuid(1), Times: model.Times{Creation: ts, Expiry: ts, Expires: true}}}
			got, _ := roundTripXML(t, db, v, false)
			if !got.Meta.DatabaseNameChanged.Equal(ts) || !got.Root.Times.Creation.Equal(ts) || !got.Root.Times.Expiry.Equal(ts) {
				t.Errorf("%s: time %v came back as %v", v, ts, got.Root.Times.Creation)
			}
		}
	}
	// A time with a zone or fractions is written as UTC with whole seconds.
	zone := time.FixedZone("x", 3600)
	db := &model.Database{Root: &model.Group{UUID: uuid(1), Times: model.Times{Creation: time.Date(2026, 1, 1, 1, 0, 0, 999, zone)}}}
	got, _ := roundTripXML(t, db, Version{4, 0}, false)
	if want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); !got.Root.Times.Creation.Equal(want) {
		t.Errorf("time = %v, want %v", got.Root.Times.Creation, want)
	}
}

func TestXMLWriterAttachments(t *testing.T) {
	mk := func(content string, protected bool) (model.BinaryKey, *model.Binary) {
		return model.BinaryKey{Hash: sha256.Sum256([]byte(content)), Protected: protected}, &model.Binary{Data: secret.New([]byte(content))}
	}
	k1, b1 := mk("one", false)
	k2, b2 := mk("two", true)
	k3, b3 := mk("never used", false)
	k4, b4 := mk("only in history", false)
	db := &model.Database{
		Binaries: map[model.BinaryKey]*model.Binary{k1: b1, k2: b2, k3: b3, k4: b4},
		Root: &model.Group{UUID: uuid(1), Entries: []*model.Entry{
			{UUID: uuid(2), Attachments: []model.Attachment{{Name: "a", Binary: k2}, {Name: "b", Binary: k1}},
				History: []*model.Entry{{UUID: uuid(2), Attachments: []model.Attachment{{Name: "old", Binary: k4}}}}},
			{UUID: uuid(3), Attachments: []model.Attachment{{Name: "c", Binary: k2}}},
		}},
	}
	stream, _ := crypto.NewInnerStream(crypto.InnerChaCha20, []byte("k"))
	_, binaries, err := marshalXML(db, xmlSpec{Version: Version{4, 0}, Stream: stream})
	if err != nil {
		t.Fatal(err)
	}
	// In order of first use; shared once; the unused one is left out.
	var got []string
	for _, b := range binaries {
		got = append(got, string(b.Data.Reveal()))
		if b.Protected != (string(b.Data.Reveal()) == "two") {
			t.Errorf("%q protected = %v", b.Data.Reveal(), b.Protected)
		}
	}
	if strings.Join(got, ",") != "two,one,only in history" {
		t.Errorf("binaries = %v", got)
	}
	// The inner header gets copies: wiping them leaves the database.
	for _, b := range binaries {
		b.Data.Wipe()
	}
	if !bytes.Equal(b2.Data.Reveal(), []byte("two")) {
		t.Error("the binaries of the inner header are not copies")
	}
	for _, v := range []Version{{4, 0}, {3, 1}} {
		back, _ := roundTripXML(t, db, v, true)
		if len(back.Binaries) != 3 {
			t.Errorf("%s: pool has %d contents, want 3", v, len(back.Binaries))
		}
	}
}

func TestXMLWriterErrors(t *testing.T) {
	stream, _ := crypto.NewInnerStream(crypto.InnerChaCha20, []byte("k"))
	spec := xmlSpec{Version: Version{4, 0}, Stream: stream}
	_, _, err := marshalXML(&model.Database{}, spec)
	assertRoute(t, err, "marshalXML.DatabaseInvalid")
	missing := model.BinaryKey{Hash: [32]byte{1}}
	db := &model.Database{Binaries: map[model.BinaryKey]*model.Binary{}, Root: &model.Group{UUID: uuid(1), Entries: []*model.Entry{
		{UUID: uuid(2), Attachments: []model.Attachment{{Name: "x", Binary: missing}}}}}}
	_, _, err = marshalXML(db, spec)
	assertRoute(t, err, "collectBinaries.func1.DatabaseInvalid")
}

func TestMinVersion(t *testing.T) {
	yes := true
	later := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	plain := func() *model.Database {
		return &model.Database{Root: &model.Group{UUID: uuid(1), Entries: []*model.Entry{{UUID: uuid(2), History: []*model.Entry{{UUID: uuid(2)}}}}}}
	}
	entry := func(db *model.Database) *model.Entry { return db.Root.Entries[0] }
	tests := []struct {
		name string
		mut  func(db *model.Database)
		want Version
	}{
		{"nothing special", func(*model.Database) {}, Version{4, 0}},
		{"custom data without a time", func(db *model.Database) {
			db.Meta.CustomData = []model.CustomDataItem{{Key: "k"}}
			db.Root.CustomData = []model.CustomDataItem{{Key: "k"}}
			entry(db).CustomData = []model.CustomDataItem{{Key: "k"}}
		}, Version{4, 0}},
		{"settings changed", func(db *model.Database) { db.Meta.SettingsChanged = later }, Version{4, 1}},
		{"icon name", func(db *model.Database) { db.Meta.CustomIcons = []model.CustomIcon{{Name: "n"}} }, Version{4, 1}},
		{"icon time", func(db *model.Database) { db.Meta.CustomIcons = []model.CustomIcon{{LastModified: later}} }, Version{4, 1}},
		{"icon without either", func(db *model.Database) { db.Meta.CustomIcons = []model.CustomIcon{{UUID: uuid(9)}} }, Version{4, 0}},
		{"database custom data time", func(db *model.Database) { db.Meta.CustomData = []model.CustomDataItem{{Key: "k", LastModified: later}} }, Version{4, 1}},
		{"group tags", func(db *model.Database) { db.Root.Tags = []string{"t"} }, Version{4, 1}},
		{"group previous parent", func(db *model.Database) { db.Root.PreviousParent = uuid(7) }, Version{4, 1}},
		{"group custom data time", func(db *model.Database) { db.Root.CustomData = []model.CustomDataItem{{LastModified: later}} }, Version{4, 1}},
		{"entry quality check", func(db *model.Database) { entry(db).QualityCheck = &yes }, Version{4, 1}},
		{"entry previous parent", func(db *model.Database) { entry(db).PreviousParent = uuid(7) }, Version{4, 1}},
		{"entry custom data time", func(db *model.Database) { entry(db).CustomData = []model.CustomDataItem{{LastModified: later}} }, Version{4, 1}},
		{"quality check in a history item", func(db *model.Database) { entry(db).History[0].QualityCheck = &yes }, Version{4, 1}},
		{"a deeper group", func(db *model.Database) {
			db.Root.Groups = []*model.Group{{UUID: uuid(3), Groups: []*model.Group{{UUID: uuid(4), Entries: []*model.Entry{{UUID: uuid(5), PreviousParent: uuid(1)}}}}}}
		}, Version{4, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := plain()
			tt.mut(db)
			if got := MinVersion(db); got != tt.want {
				t.Errorf("MinVersion = %v, want %v", got, tt.want)
			}
		})
	}
}
