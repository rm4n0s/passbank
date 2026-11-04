package kdbx

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// readFixtureDB reads a fixture of KDBX 4 to the model.
func readFixtureDB(t *testing.T, file, password, keyFile string) *model.Database {
	t.Helper()
	c, err := readContainer4(context.Background(), fixtureFile(t, file), fixtureCreds(t, password, keyFile))
	if err != nil {
		t.Fatal(err)
	}
	defer c.wipe()
	db, err := parseXML(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func uuid(last ...byte) model.UUID {
	var u model.UUID
	copy(u[len(u)-len(last):], last)
	return u
}

func findGroup(g *model.Group, name string) *model.Group {
	if g.Name == name {
		return g
	}
	for _, c := range g.Groups {
		if f := findGroup(c, name); f != nil {
			return f
		}
	}
	return nil
}

func findEntry(g *model.Group, title string) *model.Entry {
	for _, e := range g.Entries {
		if e.Title() == title {
			return e
		}
	}
	for _, c := range g.Groups {
		if e := findEntry(c, title); e != nil {
			return e
		}
	}
	return nil
}

func fieldText(t *testing.T, e *model.Entry, key string) string {
	t.Helper()
	f, ok := e.Field(key)
	if !ok {
		t.Fatalf("entry %q has no field %q", e.Title(), key)
	}
	return f.Text()
}

func TestParseXML4RichFixtures(t *testing.T) {
	for _, tt := range []struct {
		file, keyFile, password string
	}{
		{"k4-rich.kdbx", "", "fixture-rich"},
		{"k4-rich-argon2id.kdbx", "", "fixture-rich"},
		{"k4-rich-chacha20.kdbx", "", "fixture-rich"},
		{"k4-rich-twofish-aeskdf.kdbx", "", "fixture-rich"},
		{"k4-rich-nocompression.kdbx", "", "fixture-rich"},
		{"k4-keyfile-xml2.kdbx", "keyfile-xml2.keyx", ""},
		{"k4-password-and-keyfile.kdbx", "keyfile-raw32.key", "fixture-rich"},
		{"k4-public-custom-data.kdbx", "", "fixture-rich"},
		{"k4-unknown-xml.kdbx", "", "fixture-rich"},
		{"k41-fields.kdbx", "", "fixture-rich"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			db := readFixtureDB(t, tt.file, tt.password, tt.keyFile)
			checkRich(t, db)
		})
	}
}

// checkRich checks the facts of FIXTURES.md ("The rich content").
func checkRich(t *testing.T, db *model.Database) {
	t.Helper()
	m := db.Meta
	if m.DatabaseName != "Rich fixture" || m.Description != "A database that uses most features" || m.DefaultUserName != "ada" ||
		m.MaintenanceHistoryDays != 365 || m.Color != "#336699" || m.HistoryMaxItems != 10 || m.HistoryMaxSize != 6291456 ||
		!m.RecycleBinEnabled || m.RecycleBinUUID != uuid(1, 3) || m.MasterKeyChangeRec != -1 || m.MasterKeyChangeForce != -1 {
		t.Errorf("Meta = %+v", m)
	}
	if want := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC); !m.DatabaseNameChanged.Equal(want) {
		t.Errorf("DatabaseNameChanged = %v", m.DatabaseNameChanged)
	}
	if m.MemoryProtection != (model.MemoryProtection{Password: true}) {
		t.Errorf("MemoryProtection = %+v", m.MemoryProtection)
	}
	if len(m.CustomIcons) != 1 || m.CustomIcons[0].UUID != uuid(3, 1) || len(m.CustomIcons[0].Data) != 24 {
		t.Errorf("CustomIcons = %+v", m.CustomIcons)
	}
	if len(m.CustomData) != 1 || m.CustomData[0].Key != "fixture-key" || m.CustomData[0].Value != "fixture-value" {
		t.Errorf("CustomData = %+v", m.CustomData)
	}
	if len(db.DeletedObjects) != 1 || db.DeletedObjects[0].UUID != uuid(4, 1) {
		t.Errorf("DeletedObjects = %+v", db.DeletedObjects)
	}

	root := db.Root
	if root == nil || root.UUID != uuid(1, 1) || root.Name != "Root" || root.Notes != "root notes" || root.IconID != 48 || !root.IsExpanded ||
		root.EnableAutoType != model.Inherit || root.EnableSearching != model.Inherit || root.LastTopVisibleEntry != uuid(2, 1) {
		t.Fatalf("root = %+v", root)
	}
	if root.Times.UsageCount != 3 || root.Times.Expires || !root.Times.Creation.Equal(time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)) {
		t.Errorf("root times = %+v", root.Times)
	}
	email := findGroup(root, "Email")
	if email == nil || email.UUID != uuid(1, 2) || email.IsExpanded || email.EnableAutoType != model.Disabled ||
		email.EnableSearching != model.Enabled || email.DefaultAutoTypeSequence != "{PASSWORD}" {
		t.Errorf("Email = %+v", email)
	}
	if nested := findGroup(root, "Nested"); nested == nil || nested.UUID != uuid(1, 4) || len(nested.Entries) != 0 {
		t.Errorf("Nested = %+v", nested)
	}
	if bin := findGroup(root, "Recycle Bin"); bin == nil || bin.UUID != uuid(1, 3) || len(bin.Entries) != 1 {
		t.Errorf("Recycle Bin = %+v", bin)
	}

	bank := findEntry(root, "Bank")
	if bank == nil || bank.UUID != uuid(2, 1) {
		t.Fatalf("Bank = %+v", bank)
	}
	for key, want := range map[string]string{"UserName": "ada", "Password": "bank-password-1", "URL": "https://example.test/bank",
		"Notes": "notes of Bank", "PIN": "4711", "Account number": "DE00 1234"} {
		if got := fieldText(t, bank, key); got != want {
			t.Errorf("Bank %s = %q, want %q", key, got, want)
		}
	}
	for key, want := range map[string]bool{"Title": false, "Password": true, "PIN": true, "Account number": false} {
		if f, _ := bank.Field(key); f.Protected != want {
			t.Errorf("Bank %s protected = %v", key, f.Protected)
		}
	}
	if bank.ForegroundColor != "#112233" || bank.BackgroundColor != "#445566" || bank.OverrideURL != "firefox %u" ||
		strings.Join(bank.Tags, "|") != "money|work" || bank.CustomIcon != uuid(3, 1) || bank.IconID != 33 {
		t.Errorf("Bank = %+v", bank)
	}
	if !bank.AutoType.Enabled || bank.AutoType.DefaultSequence != "{USERNAME}{TAB}{PASSWORD}{ENTER}" {
		t.Errorf("Bank auto-type = %+v", bank.AutoType)
	}
	if len(bank.History) != 2 || fieldText(t, bank.History[0], "Password") != "older-bank-password" || fieldText(t, bank.History[1], "Password") != "old-bank-password" {
		t.Fatalf("Bank history has %d items", len(bank.History))
	}
	for _, h := range bank.History {
		if len(h.History) != 0 || h.UUID != bank.UUID {
			t.Errorf("a history item has %d items of history and UUID %v", len(h.History), h.UUID)
		}
	}
	if len(bank.Attachments) != 1 || bank.Attachments[0].Name != "statement.txt" {
		t.Fatalf("Bank attachments = %+v", bank.Attachments)
	}
	key := bank.Attachments[0].Binary
	if key.Hash != sha256.Sum256(attachment) || !key.Protected {
		t.Errorf("attachment key = %+v", key)
	}
	if b := db.Binaries[key]; b == nil || string(b.Data.Reveal()) != string(attachment) || len(db.Binaries) != 1 {
		t.Errorf("binaries = %d", len(db.Binaries))
	}

	uni := findEntry(root, "Ünïcödé ключ 鍵")
	if uni == nil || uni.UUID != uuid(2, 4) || fieldText(t, uni, "UserName") != "用户" || fieldText(t, uni, "Password") != "pässwörd-日本語" {
		t.Errorf("Ünïcödé = %+v", uni)
	}
	if mail := findEntry(root, "Mail"); mail == nil || fieldText(t, mail, "Password") != "mail-password-1" || fieldText(t, mail, "UserName") != "ada@example.test" {
		t.Errorf("Mail = %+v", mail)
	}
	if forum := findEntry(root, "Old forum"); forum == nil || fieldText(t, forum, "Password") != "forum-password-1" {
		t.Errorf("Old forum = %+v", forum)
	}
}

func TestParseXML4EmptyFixtures(t *testing.T) {
	for _, file := range []string{"k4-empty.kdbx"} {
		db := readFixtureDB(t, file, "fixture-min", "")
		if db.Root == nil || len(db.Root.Entries) != 0 || len(db.Binaries) != 0 || len(db.DeletedObjects) != 0 {
			t.Errorf("%s: root %+v", file, db.Root)
		}
	}
}

func TestParseXML4PublicCustomData(t *testing.T) {
	db := readFixtureDB(t, "k4-public-custom-data.kdbx", "fixture-rich", "")
	if len(db.PublicCustomData) != 1 || db.PublicCustomData[0].Key != "fixture" || db.PublicCustomData[0].Type != variantUInt32 {
		t.Errorf("public custom data = %+v", db.PublicCustomData)
	}
}

func TestParseXML4Fields41(t *testing.T) {
	db := readFixtureDB(t, "k41-fields.kdbx", "fixture-rich", "")
	if strings.Join(db.Root.Tags, "|") != "team|home" || db.Root.PreviousParent != uuid(1, 3) {
		t.Errorf("root tags %v, previous parent %v", db.Root.Tags, db.Root.PreviousParent)
	}
	bank := findEntry(db.Root, "Bank")
	if bank.QualityCheck == nil || *bank.QualityCheck || bank.PreviousParent != uuid(1, 2) {
		t.Errorf("Bank quality check %v, previous parent %v", bank.QualityCheck, bank.PreviousParent)
	}
	if mail := findEntry(db.Root, "Mail"); mail.QualityCheck != nil {
		t.Error("an entry that says nothing has a quality check")
	}
	icon := db.Meta.CustomIcons[0]
	if icon.Name != "fixture icon" || !icon.LastModified.Equal(time.Date(2026, 1, 1, 0, 0, 8, 0, time.UTC)) {
		t.Errorf("icon = %+v", icon)
	}
	if !db.Meta.SettingsChanged.Equal(time.Date(2026, 1, 1, 0, 0, 9, 0, time.UTC)) {
		t.Errorf("SettingsChanged = %v", db.Meta.SettingsChanged)
	}
	if got := db.Meta.CustomData[0].LastModified; !got.Equal(time.Date(2026, 1, 1, 0, 0, 10, 0, time.UTC)) {
		t.Errorf("custom data modified = %v", got)
	}
}

func TestParseXML4UnknownXML(t *testing.T) {
	db := readFixtureDB(t, "k4-unknown-xml.kdbx", "fixture-rich", "")
	check := func(where string, raw model.Raw, secretText string) {
		t.Helper()
		if len(raw.Nodes) != 1 {
			t.Fatalf("%s: %d unknown nodes", where, len(raw.Nodes))
		}
		n := raw.Nodes[0]
		if !strings.HasPrefix(n.Name, "X-Plugin-") || len(n.Attrs) != 1 || n.Attrs[0] != (model.RawAttr{Name: "version", Value: "2"}) || len(n.Children) != 3 {
			t.Fatalf("%s: node = %+v", where, n)
		}
		token := n.Children[0]
		// The Token has a Protected attribute but is not a Value: its text is not encrypted.
		if token.Name != "Token" || string(token.Text.Reveal()) != secretText || len(token.Attrs) != 1 || token.Attrs[0] != (model.RawAttr{Name: "Protected", Value: "True"}) {
			t.Errorf("%s: token = %+v", where, token)
		}
		if v := n.Children[1]; v.Name != "Visible" || string(v.Text.Reveal()) != "yes" {
			t.Errorf("%s: visible = %+v", where, v)
		}
		if c := n.Children[2]; c.Name != "Nested" || len(c.Children) != 1 || c.Children[0].Name != "Deep" || string(c.Children[0].Text.Reveal()) != "text" || len(c.Text.Reveal()) != 0 {
			t.Errorf("%s: nested = %+v", where, c)
		}
	}
	check("Meta", db.Meta.Raw, "plugin-secret-meta")
	check("root group", db.Root.Raw, "plugin-secret-group")
	check("Bank", findEntry(db.Root, "Bank").Raw, "plugin-secret-entry")
	if len(db.Raw.Nodes) != 1 {
		t.Fatalf("top level: %d nodes", len(db.Raw.Nodes))
	}
	top := db.Raw.Nodes[0]
	if top.Name != "X-TopLevel" || string(top.Text.Reveal()) != "kept" || len(top.Children) != 1 || top.Children[0].Name != "Child" ||
		top.Children[0].Attrs[0] != (model.RawAttr{Name: "a", Value: "1"}) {
		t.Errorf("top level node = %+v", top)
	}
	// The plain rich fixture has none.
	rich := readFixtureDB(t, "k4-rich.kdbx", "fixture-rich", "")
	if len(rich.Raw.Nodes)+len(rich.Meta.Raw.Nodes)+len(rich.Root.Raw.Nodes) != 0 {
		t.Error("the rich fixture has unknown XML")
	}
}

// --- hand-made XML ---

const innerKey = "an inner stream key"

// xmlContainer makes a container around an XML, with the ChaCha20 stream.
func xmlContainer(xmlText string, binaries ...innerBinaryData) *container {
	return &container{
		Header: &Header{Version: Version{4, 0}}, XML: []byte(xmlText), InnerStreamID: crypto.InnerChaCha20,
		InnerStreamKey: secret.New([]byte(innerKey)), Binaries: binaries,
	}
}

// protectedValues encrypts the texts with the inner stream in order and returns
// them as the base64 that the XML holds.
func protectedValues(t *testing.T, texts ...string) []string {
	t.Helper()
	s, err := crypto.NewInnerStream(crypto.InnerChaCha20, []byte(innerKey))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(texts))
	for i, text := range texts {
		b := []byte(text)
		s.XORKeyStream(b, b)
		out[i] = base64.StdEncoding.EncodeToString(b)
	}
	return out
}

func b64uuid(last ...byte) string {
	u := uuid(last...)
	return base64.StdEncoding.EncodeToString(u[:])
}

// xmlDoc wraps a body in a database: the root group has the UUID 1 and holds body.
func xmlDoc(meta, body string) string {
	return fmt.Sprintf(`<?xml version="1.0"?><KeePassFile><Meta>%s</Meta><Root><Group><UUID>%s</UUID><Name>R</Name>%s</Group></Root></KeePassFile>`, meta, b64uuid(1), body)
}

func entryXML(uuidLast byte, extra string) string {
	return fmt.Sprintf(`<Entry><UUID>%s</UUID>%s</Entry>`, b64uuid(uuidLast), extra)
}

func parse(t *testing.T, xmlText string, binaries ...innerBinaryData) (*model.Database, error) {
	t.Helper()
	return parseXML(context.Background(), xmlContainer(xmlText, binaries...))
}

func TestParseXML4ProtectedValuesAreDecryptedInOrder(t *testing.T) {
	v := protectedValues(t, "first", "second", "", "third", "fourth")
	body := entryXML(2, fmt.Sprintf(`<String><Key>A</Key><Value Protected="True">%s</Value></String>
<String><Key>B</Key><Value>plain</Value></String>
<String><Key>C</Key><Value Protected="True">%s</Value></String>
<String><Key>D</Key><Value Protected="True">%s</Value></String>
<History>%s</History>`, v[0], v[1], v[2],
		entryXML(2, fmt.Sprintf(`<String><Key>E</Key><Value Protected="True">%s</Value></String>`, v[3]))))
	body += entryXML(3, fmt.Sprintf(`<String><Key>F</Key><Value Protected="True">%s</Value></String><String><Key>G</Key><Value ProtectInMemory="True">visible</Value></String>`, v[4]))
	db, err := parse(t, xmlDoc("", body))
	if err != nil {
		t.Fatal(err)
	}
	e := db.Root.Entries[0]
	for key, want := range map[string]string{"A": "first", "B": "plain", "C": "second", "D": ""} {
		if got := fieldText(t, e, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := fieldText(t, e.History[0], "E"); got != "third" {
		t.Errorf("history E = %q", got)
	}
	f := db.Root.Entries[1]
	if got := fieldText(t, f, "F"); got != "fourth" {
		t.Errorf("F = %q", got)
	}
	if g, _ := f.Field("G"); g.Text() != "visible" || !g.Protected {
		t.Errorf("a ProtectInMemory value = %q protected %v", g.Text(), g.Protected)
	}
	if a, _ := e.Field("A"); !a.Protected {
		t.Error("a protected value is not marked protected")
	}
	if b, _ := e.Field("B"); b.Protected {
		t.Error("a plain value is marked protected")
	}
}

func TestParseXML4Attachments(t *testing.T) {
	plain := innerBinaryData{Data: secret.New([]byte("plain bytes"))}
	prot := innerBinaryData{Protected: true, Data: secret.New([]byte("plain bytes"))}
	unused := innerBinaryData{Data: secret.New([]byte("never used"))}
	ref := func(name string, n int) string {
		return fmt.Sprintf(`<Binary><Key>%s</Key><Value Ref="%d"/></Binary>`, name, n)
	}
	body := entryXML(2, ref("a.txt", 0)+ref("b.txt", 1)+ref("again.txt", 0)) + entryXML(3, ref("c.txt", 0))
	c := xmlContainer(xmlDoc("", body), plain, prot, unused)
	db, err := parseXML(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("plain bytes"))
	a := db.Root.Entries[0].Attachments
	if len(a) != 3 || a[0].Name != "a.txt" || a[0].Binary != (model.BinaryKey{Hash: hash}) || a[1].Binary != (model.BinaryKey{Hash: hash, Protected: true}) || a[2].Binary != a[0].Binary {
		t.Errorf("attachments = %+v", a)
	}
	if db.Root.Entries[1].Attachments[0].Binary != a[0].Binary {
		t.Error("two entries with the same bytes do not share one content")
	}
	if len(db.Binaries) != 2 {
		t.Errorf("pool has %d contents, want 2 (the unused one is dropped)", len(db.Binaries))
	}
	// The database owns its copies: wiping the container leaves them.
	c.wipe()
	if string(db.Binaries[a[0].Binary].Data.Reveal()) != "plain bytes" {
		t.Error("wiping the container wiped the pool")
	}
}

func TestParseXML4Times(t *testing.T) {
	base64Time := func(secs int64) string {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], uint64(secs))
		return base64.StdEncoding.EncodeToString(b[:])
	}
	tests := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"2026-01-01T00:00:01Z", time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC), false},
		{"2026-01-01T02:00:01+02:00", time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC), false},
		{"2026-01-01T00:00:01.987Z", time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC), false},
		{"  2026-01-01T00:00:01Z\n", time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC), false},
		{"", time.Time{}, false},
		{base64Time(0), time.Time{}, false},
		{base64Time(time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC).Unix() + secondsToUnixEpoch), time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC), false},
		{base64Time(maxKDBXSeconds), time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), false},
		{base64Time(maxKDBXSeconds + 1), time.Time{}, true},
		{base64Time(-1), time.Time{}, true},
		{"AAAA", time.Time{}, true},
		{"not a time", time.Time{}, true},
		{"2026-13-01T00:00:01Z", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			db, err := parse(t, xmlDoc(fmt.Sprintf(`<DatabaseNameChanged>%s</DatabaseNameChanged>`, tt.in), ""))
			if tt.wantErr {
				assertRoute(t, err, "parseXMLTime.FileCorrupt")
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := db.Meta.DatabaseNameChanged; !got.Equal(tt.want) || got.Location() != time.UTC {
				t.Errorf("time = %v (%v), want %v", got, got.Location(), tt.want)
			}
		})
	}
}

func TestParseXML4Tags(t *testing.T) {
	for in, want := range map[string]string{"": "", "a": "a", "a;b": "a|b", "a, b ;c,,": "a|b|c", " ; ": ""} {
		db, err := parse(t, xmlDoc("", entryXML(2, `<Tags>`+in+`</Tags>`)))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(db.Root.Entries[0].Tags, "|"); got != want {
			t.Errorf("tags %q = %q, want %q", in, got, want)
		}
	}
}

func TestParseXML4LeafWithAttributesIsKept(t *testing.T) {
	db, err := parse(t, strings.Replace(xmlDoc("", ""), "<Name>R</Name>", `<Name lang="el">Ρίζα</Name><Notes>n</Notes>`, 1))
	if err != nil {
		t.Fatal(err)
	}
	g := db.Root
	if g.Name != "" || g.Notes != "n" || len(g.Raw.Nodes) != 1 || g.Raw.Nodes[0].Name != "Name" || string(g.Raw.Nodes[0].Text.Reveal()) != "Ρίζα" ||
		g.Raw.Nodes[0].Attrs[0] != (model.RawAttr{Name: "lang", Value: "el"}) {
		t.Errorf("group = %+v, raw = %+v", g, g.Raw.Nodes)
	}
}

func TestParseXML4AttributesOfObjectsAreKept(t *testing.T) {
	doc := strings.Replace(xmlDoc("", entryXML(2, "")), "<Entry>", `<Entry x="1">`, 1)
	doc = strings.Replace(doc, "<KeePassFile>", `<KeePassFile y="2">`, 1)
	db, err := parse(t, doc)
	if err != nil {
		t.Fatal(err)
	}
	if got := db.Root.Entries[0].Raw.Attrs; len(got) != 1 || got[0] != (model.RawAttr{Name: "x", Value: "1"}) {
		t.Errorf("entry attributes = %+v", got)
	}
	if got := db.Raw.Attrs; len(got) != 1 || got[0] != (model.RawAttr{Name: "y", Value: "2"}) {
		t.Errorf("file attributes = %+v", got)
	}
}

func TestParseXML4MetaDefaults(t *testing.T) {
	db, err := parse(t, `<KeePassFile><Root><Group><UUID>`+b64uuid(1)+`</UUID></Group></Root></KeePassFile>`)
	if err != nil {
		t.Fatal(err)
	}
	m := db.Meta
	if m.HistoryMaxItems != 10 || m.HistoryMaxSize != 6<<20 || m.MaintenanceHistoryDays != 365 || m.MasterKeyChangeRec != -1 ||
		m.MasterKeyChangeForce != -1 || !m.RecycleBinEnabled || !m.MemoryProtection.Password || m.MemoryProtection.Title {
		t.Errorf("Meta = %+v", m)
	}
}

func TestParseXML4Errors(t *testing.T) {
	deepGroups := func(n int) string {
		var sb strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&sb, "<Group><UUID>%s</UUID>", b64uuid(1))
		}
		sb.WriteString(strings.Repeat("</Group>", n))
		return `<KeePassFile><Root>` + sb.String() + `</Root></KeePassFile>`
	}
	deepRaw := func(n int) string {
		return xmlDoc("", strings.Repeat("<X>", n)+strings.Repeat("</X>", n))
	}
	badValue := func(inner string) string {
		return xmlDoc("", entryXML(2, inner))
	}
	tests := []struct {
		name  string
		xml   string
		route string
	}{
		{"not XML", "this is not xml", "next.FileCorrupt"},
		{"empty", "", "next.FileCorrupt"},
		{"cut in an element", `<KeePassFile><Meta>`, "next.FileCorrupt"},
		{"mismatched tags", `<KeePassFile><Meta></Root></KeePassFile>`, "next.FileCorrupt"},
		{"another root element", `<Other/>`, "document.FileCorrupt"},
		{"no root group", `<KeePassFile><Meta/><Root/></KeePassFile>`, "document.FileCorrupt"},
		{"no Root", `<KeePassFile/>`, "document.FileCorrupt"},
		{"two root groups", xmlDoc("", "")[:len(xmlDoc("", ""))-len("</Root></KeePassFile>")] + `<Group><UUID>` + b64uuid(9) + `</UUID></Group></Root></KeePassFile>`, "root.1.FileCorrupt"},
		{"a group without a UUID", `<KeePassFile><Root><Group><Name>x</Name></Group></Root></KeePassFile>`, "group.FileCorrupt"},
		{"an entry without a UUID", xmlDoc("", `<Entry><String><Key>a</Key><Value>b</Value></String></Entry>`), "entry.FileCorrupt"},
		{"a UUID that is not base64", badValue(`<IconID>1</IconID><CustomIconUUID>!!!!</CustomIconUUID>`), "parseXMLUUID.FileCorrupt"},
		{"a UUID of 15 bytes", badValue(`<CustomIconUUID>` + base64.StdEncoding.EncodeToString(make([]byte, 15)) + `</CustomIconUUID>`), "parseXMLUUID.FileCorrupt"},
		{"a number that is not one", badValue(`<IconID>five</IconID>`), "parseXMLInt.FileCorrupt"},
		{"a number too large", badValue(`<IconID>99999999999999999999</IconID>`), "parseXMLInt.FileCorrupt"},
		{"a bool that is neither", badValue(`<QualityCheck>maybe</QualityCheck>`), "parseXMLBool.FileCorrupt"},
		{"a tri-state that is none", strings.Replace(xmlDoc("", ""), "<Name>R</Name>", "<EnableAutoType>maybe</EnableAutoType>", 1), "setTriState.1.FileCorrupt"},
		{"bad base64 in a protected value", badValue(`<String><Key>a</Key><Value Protected="True">***</Value></String>`), "value.FileCorrupt"},
		{"a reference that is not a number", badValue(`<Binary><Key>a</Key><Value Ref="x"/></Binary>`), "value.FileCorrupt"},
		{"a negative reference", badValue(`<Binary><Key>a</Key><Value Ref="-1"/></Binary>`), "value.FileCorrupt"},
		{"a reference to nothing", badValue(`<Binary><Key>a</Key><Value Ref="0"/></Binary>`), "attachment.FileCorrupt"},
		{"an attachment without a reference", badValue(`<Binary><Key>a</Key><Value>inline</Value></Binary>`), "attachment.FileCorrupt"},
		{"an element inside a value", badValue(`<String><Key>a</Key><Value>x<b/></Value></String>`), "textBytes.FileCorrupt"},
		{"an element inside a number", badValue(`<IconID><b/></IconID>`), "textBytes.FileCorrupt"},
		{"a bad icon", xmlDoc(`<CustomIcons><Icon><UUID>`+b64uuid(3)+`</UUID><Data>***</Data></Icon></CustomIcons>`, ""), "customIcons.1.1.1.FileCorrupt"},
		{"a deleted object with a bad time", xmlDoc("", "")[:len(xmlDoc("", ""))-len("</Root></KeePassFile>")] + `<DeletedObjects><DeletedObject><UUID>` + b64uuid(4) + `</UUID><DeletionTime>x</DeletionTime></DeletedObject></DeletedObjects></Root></KeePassFile>`, "parseXMLTime.FileCorrupt"},
		{"groups nested 1001 deep", deepGroups(maxGroupDepth + 1), "group.FileCorrupt"},
		{"unknown elements nested too deep", deepRaw(maxElementDepth + 1), "rawNode.FileCorrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(t, tt.xml)
			if err == nil {
				t.Fatal("accepted")
			}
			assertRoute(t, err, tt.route)
		})
	}
}

func TestParseXML4DeepestGroupsAreAccepted(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < maxGroupDepth; i++ {
		fmt.Fprintf(&sb, "<Group><UUID>%s</UUID>", b64uuid(1))
	}
	sb.WriteString(strings.Repeat("</Group>", maxGroupDepth))
	if _, err := parse(t, `<KeePassFile><Root>`+sb.String()+`</Root></KeePassFile>`); err != nil {
		t.Fatal(err)
	}
}

func TestParseXML4TextLimit(t *testing.T) {
	// The base64 UUIDs are 24 characters, so that is the smallest limit.
	const limit = 24
	at, over := strings.Repeat("a", limit), strings.Repeat("a", limit+1)
	doc := xmlDoc("", entryXML(2, `<String><Key>a</Key><Value>`+at+`</Value></String><OverrideURL>`+at+`</OverrideURL>`)+`<X>`+at+`</X>`)
	if _, err := parseXMLLimit(context.Background(), xmlContainer(doc), limit); err != nil {
		t.Fatalf("a text at the limit: %v", err)
	}
	for name, text := range map[string]string{
		"a value":        entryXML(2, `<String><Key>a</Key><Value>`+over+`</Value></String>`),
		"a plain leaf":   entryXML(2, `<OverrideURL>`+over+`</OverrideURL>`),
		"an unknown one": `<X>` + over + `</X>`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseXMLLimit(context.Background(), xmlContainer(xmlDoc("", text)), limit)
			if name == "an unknown one" {
				assertRoute(t, err, "rawNode.FileCorrupt")
			} else {
				assertRoute(t, err, "textBytes.FileCorrupt")
			}
		})
	}
}

func TestParseXML4Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := strings.Repeat(entryXML(2, `<IconID>1</IconID>`), cancelEvery)
	_, err := parseXML(ctx, xmlContainer(xmlDoc("", body)))
	assertRoute(t, err, "next.ReadCancelled")
}

func TestParseXML4BadInnerStream(t *testing.T) {
	c := xmlContainer(xmlDoc("", ""))
	c.InnerStreamID = 77
	_, err := parseXML(context.Background(), c)
	assertRoute(t, err, "NewInnerStream.InnerStreamUnsupported")
}

func TestParseXML4NeverShowsSecrets(t *testing.T) {
	v := protectedValues(t, "very-secret-value")
	_, err := parse(t, xmlDoc("", entryXML(2, fmt.Sprintf(`<String><Key>a</Key><Value Protected="True">%s</Value></String><IconID>bad</IconID>`, v[0]))))
	if err == nil {
		t.Fatal("accepted")
	}
	if s := fmt.Sprintf("%v %+v", err, err); strings.Contains(s, "very-secret-value") || strings.Contains(s, v[0]) {
		t.Errorf("the error shows a secret: %s", s)
	}
}
