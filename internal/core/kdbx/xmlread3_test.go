package kdbx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// xmlContainer3 makes a container of a KDBX 3 file around an XML. The inner
// stream is ChaCha20, as in xmlContainer.
func xmlContainer3(version Version, headerHash []byte, xmlText string) *container {
	c := xmlContainer(xmlText)
	c.Header = &Header{Version: version}
	c.HeaderHash = headerHash
	return c
}

func gz(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// metaWith is a database whose Meta holds meta and whose root group holds body.
func metaWith(meta, body string) string { return xmlDoc(meta, body) }

func TestParseXML3HeaderHash(t *testing.T) {
	hash := sha256.Sum256([]byte("the header"))
	good := fmt.Sprintf(`<HeaderHash>%s</HeaderHash>`, b64(hash[:]))
	other := sha256.Sum256([]byte("another header"))
	tests := []struct {
		name    string
		version Version
		meta    string
		route   string // "" when the file must be accepted
	}{
		{"3.1 with the right hash", Version{3, 1}, good, ""},
		{"3.1 with a wrong hash", Version{3, 1}, fmt.Sprintf(`<HeaderHash>%s</HeaderHash>`, b64(other[:])), "setHeaderHash.FileCorrupt"},
		{"3.1 with a hash that is not base64", Version{3, 1}, `<HeaderHash>***</HeaderHash>`, "setHeaderHash.FileCorrupt"},
		{"3.1 with an empty hash", Version{3, 1}, `<HeaderHash></HeaderHash>`, "setHeaderHash.FileCorrupt"},
		{"3.1 without a hash", Version{3, 1}, "", "parseXMLLimit.FileCorrupt"},
		{"3.2 without a hash", Version{3, 2}, "", "parseXMLLimit.FileCorrupt"},
		{"3.0 without a hash", Version{3, 0}, "", ""},
		{"2.0 without a hash", Version{2, 0}, "", ""},
		{"3.0 with the right hash", Version{3, 0}, good, ""},
		{"3.0 with a wrong hash", Version{3, 0}, fmt.Sprintf(`<HeaderHash>%s</HeaderHash>`, b64(other[:])), "setHeaderHash.FileCorrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := parseXML(context.Background(), xmlContainer3(tt.version, hash[:], metaWith(tt.meta, "")))
			if tt.route == "" {
				if err != nil {
					t.Fatal(err)
				}
				db.Wipe()
				return
			}
			assertRoute(t, err, tt.route)
		})
	}
	t.Run("KDBX 4 keeps a HeaderHash as unknown XML", func(t *testing.T) {
		db, err := parseXML(context.Background(), xmlContainer(metaWith(good, "")))
		if err != nil {
			t.Fatal(err)
		}
		if len(db.Meta.Raw.Nodes) != 1 || db.Meta.Raw.Nodes[0].Name != "HeaderHash" {
			t.Errorf("raw = %+v", db.Meta.Raw.Nodes)
		}
	})
}

func TestParseXML3Binaries(t *testing.T) {
	hash := sha256.Sum256(nil)
	plain := []byte("plain content")
	zipped := []byte("zipped content, zipped content, zipped content")
	protect := newProtector(t)
	prot := protect([]byte("protected content"))
	protZipped := protect(gz(t, []byte("protected and zipped")))
	meta := fmt.Sprintf(`<HeaderHash>%s</HeaderHash><Binaries>
<Binary ID="0">%s</Binary>
<Binary ID="1" Compressed="True">%s</Binary>
<Binary ID="2" Compressed="True"/>
<Binary ID="5" Protected="True">%s</Binary>
<Binary ID="6" Compressed="True" Protected="True">%s</Binary>
<Binary ID="9">%s</Binary></Binaries>`, b64(hash[:]), b64(plain), b64(gz(t, zipped)), prot, protZipped, b64(plain))
	ref := func(name string, id int) string {
		return fmt.Sprintf(`<Binary><Key>%s</Key><Value Ref="%d"/></Binary>`, name, id)
	}
	body := entryXML(2, ref("plain", 0)+ref("zipped", 1)+ref("empty", 2)+ref("prot", 5)+ref("protzip", 6)+ref("dup", 9))
	db, err := parseXML(context.Background(), xmlContainer3(Version{3, 1}, hash[:], metaWith(meta, body)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Wipe()
	content := func(name string) (string, bool) {
		for _, a := range db.Root.Entries[0].Attachments {
			if a.Name == name {
				return string(db.Binaries[a.Binary].Data.Reveal()), a.Binary.Protected
			}
		}
		t.Fatalf("no attachment %q", name)
		return "", false
	}
	for name, want := range map[string]struct {
		content   string
		protected bool
	}{
		"plain": {"plain content", false}, "zipped": {string(zipped), false}, "empty": {"", false},
		"prot": {"protected content", true}, "protzip": {"protected and zipped", true}, "dup": {"plain content", false},
	} {
		if got, protected := content(name); got != want.content || protected != want.protected {
			t.Errorf("%s = %q (protected %v), want %q (%v)", name, got, protected, want.content, want.protected)
		}
	}
	// Binaries 0 and 9 have the same bytes and flag: one content in the pool.
	if len(db.Binaries) != 5 {
		t.Errorf("pool has %d contents, want 5", len(db.Binaries))
	}
	if len(db.Meta.Raw.Nodes) != 0 {
		t.Errorf("Binaries was kept as unknown XML: %+v", db.Meta.Raw.Nodes)
	}
}

// newProtector returns a function that encrypts data with the inner stream of the
// test containers and returns it as base64. Each call goes on where the last
// stopped, so call it in document order.
func newProtector(t *testing.T) func(data []byte) string {
	t.Helper()
	s, err := crypto.NewInnerStream(crypto.InnerChaCha20, []byte(innerKey))
	if err != nil {
		t.Fatal(err)
	}
	return func(data []byte) string {
		b := bytes.Clone(data)
		s.XORKeyStream(b, b)
		return b64(b)
	}
}

func TestParseXML3InlineBinaries(t *testing.T) {
	hash := sha256.Sum256(nil)
	content := []byte("inline content")
	protect := newProtector(t)
	inline := func(name, attrs, text string) string {
		return fmt.Sprintf(`<Binary><Key>%s</Key><Value %s>%s</Value></Binary>`, name, attrs, text)
	}
	body := entryXML(2,
		inline("plain", "", b64(content))+
			inline("zipped", `Compressed="True"`, b64(gz(t, content)))+
			inline("prot", `Protected="True"`, protect(content))+
			inline("protzip", `Protected="True" Compressed="True"`, protect(gz(t, content)))+
			inline("empty", "", ""))
	meta := fmt.Sprintf(`<HeaderHash>%s</HeaderHash>`, b64(hash[:]))
	db, err := parseXML(context.Background(), xmlContainer3(Version{3, 1}, hash[:], metaWith(meta, body)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Wipe()
	atts := db.Root.Entries[0].Attachments
	if len(atts) != 5 {
		t.Fatalf("attachments = %+v", atts)
	}
	for i, a := range atts {
		want := string(content)
		if a.Name == "empty" {
			want = ""
		}
		if got := string(db.Binaries[a.Binary].Data.Reveal()); got != want {
			t.Errorf("%d %s = %q, want %q", i, a.Name, got, want)
		}
	}
	if atts[0].Binary != atts[1].Binary || atts[2].Binary.Protected != true || atts[0].Binary.Protected {
		t.Errorf("keys = %+v", atts)
	}
	// KDBX 4 does not know inline content.
	_, err = parseXML(context.Background(), xmlContainer(xmlDoc("", entryXML(2, inline("x", "", b64(content))))))
	assertRoute(t, err, "attachment.FileCorrupt")
}

func TestParseXML3BinariesErrors(t *testing.T) {
	hash := sha256.Sum256(nil)
	tests := []struct {
		name, binaries, route string
	}{
		{"an ID that is not a number", `<Binary ID="x">AA==</Binary>`, "binaries.1.FileCorrupt"},
		{"a negative ID", `<Binary ID="-1">AA==</Binary>`, "binaries.1.FileCorrupt"},
		{"no ID", `<Binary>AA==</Binary>`, "binaries.1.FileCorrupt"},
		{"the same ID twice", `<Binary ID="1">AA==</Binary><Binary ID="1">AA==</Binary>`, "binaries.1.FileCorrupt"},
		{"content that is not base64", `<Binary ID="1">***</Binary>`, "binaries.1.FileCorrupt"},
		{"compressed content that is not GZip", `<Binary ID="1" Compressed="True">AAAA</Binary>`, "gunzip.FileCorrupt"},
		{"an element inside", `<Binary ID="1">AA==<x/></Binary>`, "textBytes.FileCorrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := fmt.Sprintf(`<HeaderHash>%s</HeaderHash><Binaries>%s</Binaries>`, b64(hash[:]), tt.binaries)
			_, err := parseXML(context.Background(), xmlContainer3(Version{3, 1}, hash[:], metaWith(meta, "")))
			assertRoute(t, err, tt.route)
		})
	}
	t.Run("a reference to a binary that is not there", func(t *testing.T) {
		meta := fmt.Sprintf(`<HeaderHash>%s</HeaderHash><Binaries><Binary ID="1">AA==</Binary></Binaries>`, b64(hash[:]))
		body := entryXML(2, `<Binary><Key>a</Key><Value Ref="2"/></Binary>`)
		_, err := parseXML(context.Background(), xmlContainer3(Version{3, 1}, hash[:], metaWith(meta, body)))
		assertRoute(t, err, "attachment.FileCorrupt")
	})
	t.Run("inline content that is not base64 or GZip", func(t *testing.T) {
		meta := fmt.Sprintf(`<HeaderHash>%s</HeaderHash>`, b64(hash[:]))
		for _, value := range []string{`<Value>***</Value>`, `<Value Compressed="True">AAAA</Value>`} {
			_, err := parseXML(context.Background(), xmlContainer3(Version{3, 1}, hash[:], metaWith(meta, entryXML(2, `<Binary><Key>a</Key>`+value+`</Binary>`))))
			if err == nil {
				t.Errorf("%s accepted", value)
			}
		}
	})
	t.Run("protected inline content that is not base64", func(t *testing.T) {
		meta := fmt.Sprintf(`<HeaderHash>%s</HeaderHash>`, b64(hash[:]))
		_, err := parseXML(context.Background(), xmlContainer3(Version{3, 1}, hash[:], metaWith(meta, entryXML(2, `<Binary><Key>a</Key><Value Protected="True">***</Value></Binary>`))))
		assertRoute(t, err, "value.FileCorrupt")
	})
}

// TestOpenBytesKDBX3Built opens built files of every version: the XML carries the
// hash of the header that the builder wrote, and a damaged header is found by it.
func TestOpenBytesKDBX3Built(t *testing.T) {
	creds := crypto.Credentials{HasPassword: true, Password: secret.New([]byte("built"))}
	xmlWith := func(hash []byte) []byte {
		return []byte(fmt.Sprintf(`<KeePassFile><Meta><HeaderHash>%s</HeaderHash><DatabaseName>built %s</DatabaseName></Meta><Root><Group><UUID>%s</UUID><Name>R</Name></Group></Root></KeePassFile>`,
			b64(hash), "x", b64uuid(1)))
	}
	for _, v := range []Version{{2, 0}, {3, 0}, {3, 1}} {
		for _, cipher := range []model.UUID{crypto.CipherAES256, crypto.CipherTwofish} {
			t.Run(v.String()+"/"+cipher.String()[:4], func(t *testing.T) {
				b := build3{Major: v.Major, Minor: v.Minor, Password: "built", Cipher: cipher, Compression: CompressionGZip}
				header := b.build(t)
				sum := sha256.Sum256(header[:headerLength(t, header)])
				b.XML = xmlWith(sum[:])
				file := b.build(t)
				db, info, err := OpenBytes(context.Background(), file, creds)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Wipe()
				if info.Version != v || info.Cipher == "" || db.Meta.DatabaseName != "built x" || info.KDF.Rounds != 10 {
					t.Errorf("info = %+v, name %q", info, db.Meta.DatabaseName)
				}
				// A different comment field changes the header and so its hash.
				b.Extra = []HeaderField{{ID: fieldComment, Data: []byte("changed")}}
				_, _, err = OpenBytes(context.Background(), b.build(t), creds)
				assertRoute(t, err, "setHeaderHash.FileCorrupt")
			})
		}
	}
	t.Run("a bad XML in a good container", func(t *testing.T) {
		_, _, err := OpenBytes(context.Background(), build3{Password: "built", XML: []byte("<KeePassFile>")}.build(t), creds)
		assertRoute(t, err, "next.FileCorrupt")
	})
	t.Run("an inner stream that is not supported", func(t *testing.T) {
		_, _, err := OpenBytes(context.Background(), build3{Password: "built", InnerStream: crypto.InnerArcFour}.build(t), creds)
		assertRoute(t, err, "NewInnerStream.InnerStreamUnsupported")
	})
}

func TestOpenBytesVersions(t *testing.T) {
	creds := crypto.Credentials{HasPassword: true, Password: secret.New([]byte("built"))}
	for _, major := range []uint16{0, 1, 5, 99} {
		file := build3{Major: major, Minor: 0, Password: "built"}.build(t)
		if major == 0 {
			file[10], file[11] = 0, 0 // the builder turns 0 into 3
		}
		_, _, err := OpenBytes(context.Background(), file, creds)
		assertRoute(t, err, "OpenBytes.VersionUnsupported")
	}
}
