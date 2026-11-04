package kdbx

import (
	"bytes"
	"compress/gzip"
	"context"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// The fuzz targets run their seeds in every `go test`. To fuzz for real, keep to
// the limits of CLAUDE.md "Memory (OOM)":
//
//	GOMEMLIMIT=2GiB go test ./internal/core/kdbx -run '^$' -fuzz FuzzParseXML4 -fuzztime 30s -parallel 1
//
// Each target checks only that the parser returns (an error or a result) and
// does not panic, hang or allocate without bound.

// fixtureHeaders are the first bytes of the KDBX 4 fixtures, which hold their headers.
func fixtureHeaders(t testing.TB) [][]byte {
	t.Helper()
	var out [][]byte
	for _, name := range []string{"k4-rich.kdbx", "k4-rich-twofish-aeskdf.kdbx", "k4-public-custom-data.kdbx", "k41-fields.kdbx"} {
		data := fixtureFile(t, name)
		out = append(out, data[:min(len(data), 400)])
	}
	return out
}

func FuzzParseHeader4(f *testing.F) {
	for _, h := range fixtureHeaders(f) {
		f.Add(h)
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, end, err := parseHeader4(data, Version{4, 0})
		if err == nil && (end < 0 || end > len(data)) {
			t.Fatalf("header end %d of %d bytes", end, len(data))
		}
	})
}

func FuzzParseVariantDictionary(f *testing.F) {
	f.Add(encodeVariant(argonParams(bytes.Repeat([]byte{1}, 16))...))
	f.Add(encodeVariant(variantItem{"a", variantString, []byte("x")}, variantItem{"b", variantUInt64, u64(7)}))
	f.Add([]byte{0, 1, 0})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		items, err := parseVariantDictionary(data)
		if err != nil {
			return
		}
		// Whatever was read can be looked up and turned into KDF parameters.
		d := newVariantDict(items)
		_, _ = kdfFromVariant(d)
	})
}

func FuzzParseInnerHeader(f *testing.F) {
	f.Add(innerHeaderBytes([]byte("0123456789abcdef"), []byte{1, 'a'}, []byte{0, 'b'}))
	f.Add(goodPayload([]byte{1, 'z'}))
	f.Add([]byte{innerBinary, 1, 0, 0, 0})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		c := &container{}
		n, err := parseInnerHeader(data, c)
		if err == nil && (n < 0 || n > len(data)) {
			t.Fatalf("inner header end %d of %d bytes", n, len(data))
		}
		c.wipe()
	})
}

func FuzzReadBlocks(f *testing.F) {
	base := bytes.Repeat([]byte{3}, 64)
	f.Add(buildBlocks(base, bytes.Repeat([]byte("d"), 100), 30))
	f.Add(buildBlocks(base, nil, 30))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if out, err := readBlocks(data, base, 1<<20); err == nil && len(out) > 1<<20 {
			t.Fatalf("read %d bytes past the limit", len(out))
		}
	})
}

func FuzzGunzip(f *testing.F) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(bytes.Repeat([]byte("zip"), 1000))
	_ = zw.Close()
	f.Add(buf.Bytes())
	f.Add(buf.Bytes()[:20])
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if out, err := gunzip(data, 1<<20); err == nil && len(out) > 1<<20 {
			t.Fatalf("read %d bytes past the limit", len(out))
		}
	})
}

func FuzzParseXML4(f *testing.F) {
	for _, name := range []string{"k4-rich.kdbx", "k4-unknown-xml.kdbx", "k41-fields.kdbx"} {
		c, err := readContainer4(context.Background(), fixtureFile(f, name), fixtureCreds(f, "fixture-rich", ""))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(c.XML)
		c.wipe()
	}
	f.Add([]byte(buildXML))
	f.Add([]byte(`<KeePassFile><Root><Group><UUID>AAAAAAAAAAAAAAAAAAAAAQ==</UUID><Entry><UUID>AAAAAAAAAAAAAAAAAAAAAg==</UUID><String><Key>k</Key><Value Protected="True">AAAA</Value></String><Binary><Key>b</Key><Value Ref="0"/></Binary></Entry></Group></Root></KeePassFile>`))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		c := &container{
			Header: &Header{Version: Version{4, 0}}, XML: data, InnerStreamID: crypto.InnerChaCha20, InnerStreamKey: secret.New([]byte("fuzz key")),
			Binaries: []innerBinaryData{{Data: secret.New([]byte("binary"))}},
		}
		db, err := parseXML(context.Background(), c)
		if err == nil {
			if db.Root == nil {
				t.Fatal("a database without a root group")
			}
			db.Wipe()
		}
		c.wipe()
	})
}

func FuzzOpenBytes(f *testing.F) {
	// A cheap file, so that a mutation that survives the header checks is not slow.
	f.Add(build4{Password: "built", Payload: goodPayload([]byte{1, 'x'}), BlockSize: 40}.build(f))
	f.Add([]byte{})
	creds := crypto.Credentials{HasPassword: true, Password: secretOf("built")}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		db, _, err := OpenBytes(context.Background(), data, creds)
		if err == nil {
			db.Wipe()
		}
	})
}

func FuzzParseHeader3(f *testing.F) {
	for _, name := range []string{"k31-rich.kdbx", "ext/go-chacha.kdbx", "ext/rs-chacha20-inner.kdbx"} {
		data := fixtureFile(f, name)
		f.Add(data[:min(len(data), 300)])
	}
	f.Add(build3{Password: "built"}.build(f)[:200])
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, end, err := parseHeader3(data, Version{3, 1})
		if err == nil && (end < 0 || end > len(data)) {
			t.Fatalf("header end %d of %d bytes", end, len(data))
		}
	})
}

func FuzzReadHashedBlocks(f *testing.F) {
	f.Add(hashedBlocks(bytes.Repeat([]byte("d"), 100), 30))
	f.Add(hashedBlocks(nil, 30))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if out, err := readHashedBlocks(data, 1<<20); err == nil && len(out) > 1<<20 {
			t.Fatalf("read %d bytes past the limit", len(out))
		}
	})
}

func FuzzOpenBytes3(f *testing.F) {
	// A cheap file of KDBX 3, so that a mutation that survives the early checks is not slow.
	f.Add(build3{Password: "built", BlockSize: 40}.build(f))
	f.Add(build3{Major: 2, Password: "built", Compression: CompressionGZip}.build(f))
	f.Add([]byte{})
	creds := crypto.Credentials{HasPassword: true, Password: secretOf("built")}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		db, _, err := OpenBytes(context.Background(), data, creds)
		if err == nil {
			db.Wipe()
		}
	})
}
