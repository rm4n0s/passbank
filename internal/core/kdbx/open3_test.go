package kdbx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/testenv"
)

// dumpEscape escapes a value as testdata/gen/dump_entries.py does.
func dumpEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "|", `\|`, "\n", `\n`, "\r", `\r`).Replace(s)
}

// dumpEntries writes one line per entry in the format of dump_entries.py.
func dumpEntries(db *model.Database) string {
	type line struct{ uuid, text string }
	var lines []line
	var walk func(g *model.Group, path []string)
	walk = func(g *model.Group, path []string) {
		for _, e := range g.Entries {
			get := func(key string) string {
				f, _ := e.Field(key)
				return f.Text()
			}
			var customFields []model.Field
			for _, f := range e.Fields {
				switch f.Key {
				case model.FieldTitle, model.FieldUserName, model.FieldPassword, model.FieldURL, model.FieldNotes:
				default:
					customFields = append(customFields, f)
				}
			}
			sort.Slice(customFields, func(i, j int) bool { return customFields[i].Key < customFields[j].Key })
			var custom []string
			for _, f := range customFields {
				custom = append(custom, f.Key+"="+dumpEscape(f.Text()))
			}
			var atts []string
			for _, a := range e.Attachments {
				sum := sha256.Sum256(db.Binaries[a.Binary].Data.Reveal())
				atts = append(atts, a.Name+":"+hex.EncodeToString(sum[:]))
			}
			sort.Strings(atts)
			text := strings.Join([]string{
				dumpEscape(strings.Join(path, "/")), dumpEscape(get(model.FieldTitle)), dumpEscape(get(model.FieldUserName)),
				dumpEscape(get(model.FieldPassword)), dumpEscape(get(model.FieldURL)), dumpEscape(get(model.FieldNotes)),
				strings.Join(e.Tags, ","), strings.Join(custom, ";"), strings.Join(atts, ","), fmt.Sprint(len(e.History)),
			}, "|")
			lines = append(lines, line{hex.EncodeToString(e.UUID[:]), text})
		}
		for _, c := range g.Groups {
			walk(c, append(path[:len(path):len(path)], c.Name))
		}
	}
	walk(db.Root, nil)
	sort.Slice(lines, func(i, j int) bool { return lines[i].uuid < lines[j].uuid })
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l.uuid + "|" + l.text + "\n")
	}
	return sb.String()
}

// kdbx3Fixtures are the KDBX 3.1 files with the credentials of FIXTURES.md, and the
// counts that pykeepass finds in them.
var kdbx3Fixtures = []struct {
	file, password, keyFile string
	groups, entries         int
	history, attachments    int
	dumped                  bool // pykeepass can read it, so testdata/dumps has its entries
}{
	{"k31-empty.kdbx", "fixture-min", "", 1, 0, 0, 0, true},
	{"k31-rich.kdbx", "fixture-rich", "", 4, 4, 2, 1, true},
	{"k31-keyfile.kdbx", "", "keyfile-k31.keyx", 1, 0, 0, 0, false},
	{"k31-password-and-keyfile.kdbx", "fixture-rich", "keyfile-xml2.keyx", 1, 0, 0, 0, false},
	{"ext/rs-password.kdbx", "demopass", "", 5, 6, 3, 0, true},
	{"ext/rs-attachments.kdbx", "demopass", "", 1, 2, 0, 4, true},
	{"ext/rs-chacha20-inner.kdbx", "password", "", 2, 1, 0, 0, false},
	{"ext/rs-keyfile.kdbx", "", "ext/rs-keyfile.key", 1, 1, 0, 0, true},
	{"ext/rs-keyfile-xml.kdbx", "", "ext/rs-keyfile-xml.key", 5, 6, 2, 0, true},
	{"ext/go-example.kdbx", "abcdefg12345678", "", 8, 3, 1, 1, true},
	{"ext/go-example-key.kdbx", "abcdefg12345678", "ext/go-example-key.key", 8, 3, 1, 1, true},
	{"ext/go-protected-binary.kdbx", "123", "", 4, 5, 1, 1, false},
	{"ext/go-chacha.kdbx", "abcdefg12345678", "", 8, 4, 1, 2, true},
	{"ext/go-twofish.kdbx", "test1234test", "", 7, 2, 0, 0, true},
	{"ext/web-demo.kdbx", "demo", "ext/web-demo.key", 8, 4, 2, 1, true},
	{"ext/web-cyrillic.kdbx", "пароль", "", 7, 2, 1, 0, true},
}

func TestOpenKDBX3Fixtures(t *testing.T) {
	for _, tt := range kdbx3Fixtures {
		t.Run(tt.file, func(t *testing.T) {
			db, info, err := Open(context.Background(), testenv.Fixture(t, tt.file), credsOf(t, tt.password, tt.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Wipe()
			if info.Version != (Version{3, 1}) || info.KDF.ID != crypto.KDFAES {
				t.Errorf("info = %+v", info)
			}
			if info.Groups != tt.groups || info.Entries != tt.entries || info.HistoryItems != tt.history || info.Attachments != tt.attachments {
				t.Errorf("counts = %d groups, %d entries, %d history, %d attachments; want %d, %d, %d, %d",
					info.Groups, info.Entries, info.HistoryItems, info.Attachments, tt.groups, tt.entries, tt.history, tt.attachments)
			}
			if tt.dumped {
				name := strings.ReplaceAll(strings.TrimSuffix(tt.file, ".kdbx"), "/", "-") + ".txt"
				want, err := os.ReadFile(filepath.Join("testdata", "dumps", name))
				if err != nil {
					t.Fatal(err)
				}
				if got := dumpEntries(db); got != string(want) {
					t.Errorf("the entries differ from what pykeepass reads:\n--- got\n%s--- want\n%s", got, want)
				}
			}
		})
	}
}

func TestOpenKDBX3Rich(t *testing.T) {
	db, _, err := Open(context.Background(), testenv.Fixture(t, "k31-rich.kdbx"), credsOf(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Wipe()
	bank := findEntry(db.Root, "Bank")
	if bank == nil || bank.UUID != uuid(2, 1) {
		t.Fatalf("Bank = %+v", bank)
	}
	for key, want := range map[string]string{"Password": "bank-password-1", "PIN": "4711", "Account number": "DE00 1234"} {
		if got := fieldText(t, bank, key); got != want {
			t.Errorf("Bank %s = %q, want %q", key, got, want)
		}
	}
	if len(bank.History) != 2 || fieldText(t, bank.History[0], "Password") != "older-bank-password" {
		t.Errorf("history = %d items", len(bank.History))
	}
	if len(bank.Attachments) != 1 || bank.Attachments[0].Name != "statement.txt" ||
		string(db.Binaries[bank.Attachments[0].Binary].Data.Reveal()) != string(attachment) {
		t.Errorf("attachments = %+v", bank.Attachments)
	}
	if db.Meta.DatabaseName != "Rich fixture" || db.Meta.Color != "#336699" || len(db.Meta.CustomIcons) != 1 || len(db.DeletedObjects) != 1 {
		t.Errorf("Meta = %+v", db.Meta)
	}
	if strings.Join(bank.Tags, "|") != "money|work" {
		t.Errorf("tags = %v (KeePassXC writes them with a comma)", bank.Tags)
	}
	if !bank.Times.Creation.Equal(db.Root.Entries[0].Times.Creation) && db.Root.Entries[0] == bank {
		t.Error("times")
	}
}

func TestOpenKDBX3ProtectedBinary(t *testing.T) {
	// The comment in gokeepasslib's tests lists the content of this file, which
	// kdbxweb wrote: a protected attachment in Meta/Binaries comes before the
	// protected values, so reading them right proves the stream stays in step.
	db, _, err := Open(context.Background(), testenv.Fixture(t, "ext/go-protected-binary.kdbx"), credsOf(t, "123", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Wipe()
	for title, password := range map[string]string{"e1-root": "PASS-e1-v2", "e4-root": "PASS-e4", "e5-empty": "", "e2-in-g1": "PASS-e2", "e3-in-g2": "PASS-e3"} {
		e := findEntry(db.Root, title)
		if e == nil {
			t.Fatalf("no entry %q", title)
		}
		if got := fieldText(t, e, "Password"); got != password {
			t.Errorf("%s password = %q, want %q", title, got, password)
		}
	}
	if got := fieldText(t, findEntry(db.Root, "e4-root"), "otp"); got != "JBSWY3DPEHPK3PXP" {
		t.Errorf("otp = %q", got)
	}
	if f, _ := findEntry(db.Root, "e4-root").Field("otp"); !f.Protected {
		t.Error("otp is not protected")
	}
	if h := findEntry(db.Root, "e1-root").History; len(h) != 1 || fieldText(t, h[0], "Password") != "PASS-e1" {
		t.Errorf("history of e1-root = %v", h)
	}
	e2 := findEntry(db.Root, "e2-in-g1")
	if len(e2.Attachments) != 1 || e2.Attachments[0].Name != "a.bin" {
		t.Fatalf("attachments = %+v", e2.Attachments)
	}
	if !e2.Attachments[0].Binary.Protected {
		t.Error("the attachment should be a protected binary")
	}
	if got := db.Binaries[e2.Attachments[0].Binary].Data.Reveal(); string(got) != strings.Repeat("\x07", 16) {
		t.Errorf("a.bin = %x", got)
	}
}

func TestOpenKDBX3ChaCha20InnerStream(t *testing.T) {
	db, info, err := Open(context.Background(), testenv.Fixture(t, "ext/rs-chacha20-inner.kdbx"), credsOf(t, "password", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Wipe()
	if info.InnerStreamID != crypto.InnerChaCha20 || info.InnerStream != "ChaCha20" {
		t.Errorf("inner stream = %d %s", info.InnerStreamID, info.InnerStream)
	}
	n := 0
	db.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
		n++
		if got := fieldText(t, e, "Password"); got != "admin" {
			t.Errorf("password = %q, want admin", got)
		}
		return true
	})
	if n != 1 {
		t.Errorf("%d entries", n)
	}
}

func TestOpenKDBX3Attachments(t *testing.T) {
	db, _, err := Open(context.Background(), testenv.Fixture(t, "ext/rs-attachments.kdbx"), credsOf(t, "demopass", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Wipe()
	// Four references; one binary is empty and several are shared between entries.
	contents := map[string]int{}
	for _, e := range db.Root.Entries {
		for _, a := range e.Attachments {
			contents[string(db.Binaries[a.Binary].Data.Reveal())]++
		}
	}
	empty, total := contents[""], 0
	for _, n := range contents {
		total += n
	}
	if total != 4 || empty != 1 {
		t.Errorf("contents = %v", contents)
	}
	if len(db.Binaries) != len(contents) {
		t.Errorf("pool has %d contents, entries use %d", len(db.Binaries), len(contents))
	}
}
