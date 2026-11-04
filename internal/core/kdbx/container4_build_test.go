package kdbx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// buildKey is the composite key of the password of built files.
func buildKey(t *testing.T, password string) secret.Bytes {
	t.Helper()
	key, err := crypto.CompositeKey(crypto.Credentials{HasPassword: true, Password: secret.New([]byte(password))})
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// buildXML is a database with a root group and nothing else.
const buildXML = `<?xml version="1.0"?><KeePassFile><Meta/><Root><Group><UUID>AAAAAAAAAAAAAAAAAAAAAQ==</UUID></Group></Root></KeePassFile>`

// goodPayload is an inner header with the given binaries and a small XML.
func goodPayload(binaries ...[]byte) []byte {
	return append(innerHeaderBytes([]byte("0123456789abcdef"), binaries...), buildXML...)
}

func TestBuiltFilesOpen(t *testing.T) {
	bins := [][]byte{append([]byte{1}, "protected attachment"...), append([]byte{0}, "plain attachment"...)}
	for _, cipher := range []model.UUID{crypto.CipherAES256, crypto.CipherTwofish, crypto.CipherChaCha20} {
		for _, comp := range []Compression{CompressionNone, CompressionGZip} {
			for _, blockSize := range []int{7, 64, 1 << 20} {
				name := strings.Join([]string{cipher.String()[:4], comp.String(), string(rune('a' + blockSize%26))}, "-")
				t.Run(name, func(t *testing.T) {
					file := build4{Password: "built", Cipher: cipher, Compression: comp, Payload: goodPayload(bins...), BlockSize: blockSize}.build(t)
					c, err := readContainer4(context.Background(), file, buildKey(t, "built"))
					if err != nil {
						t.Fatal(err)
					}
					defer c.wipe()
					if string(c.XML) != buildXML {
						t.Errorf("XML = %q", c.XML)
					}
					if c.InnerStreamID != crypto.InnerChaCha20 || string(c.InnerStreamKey.Reveal()) != "0123456789abcdef" {
						t.Errorf("inner stream %d %q", c.InnerStreamID, c.InnerStreamKey.Reveal())
					}
					if len(c.Binaries) != 2 || !c.Binaries[0].Protected || c.Binaries[1].Protected ||
						string(c.Binaries[0].Data.Reveal()) != "protected attachment" || string(c.Binaries[1].Data.Reveal()) != "plain attachment" {
						t.Errorf("binaries = %+v", c.Binaries)
					}
					if c.Header.Compression != comp || c.Header.CipherID != cipher {
						t.Errorf("header = %+v", c.Header)
					}
					if _, err := readContainer4(context.Background(), file, buildKey(t, "wrong")); err == nil {
						t.Error("a wrong password opened the file")
					}
				})
			}
		}
	}
}

func TestBuiltFilesWithOddHeaders(t *testing.T) {
	extra := []HeaderField{{ID: fieldComment, Data: []byte("a comment")}, {ID: 99, Data: []byte("unknown field")}, {ID: 100, Data: nil}}
	file := build4{Password: "built", Payload: goodPayload(), ExtraHeader: extra}.build(t)
	c, err := readContainer4(context.Background(), file, buildKey(t, "built"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.wipe()
	if string(c.Header.Comment) != "a comment" {
		t.Errorf("comment = %q", c.Header.Comment)
	}
	if len(c.Header.Unknown) != 2 || c.Header.Unknown[0].ID != 99 || string(c.Header.Unknown[0].Data) != "unknown field" || c.Header.Unknown[1].ID != 100 {
		t.Errorf("unknown fields = %+v", c.Header.Unknown)
	}
}

func TestBuiltFilesWithBadParts(t *testing.T) {
	salt := bytes.Repeat([]byte{7}, 16)
	argon := func(mut func(items []variantItem) []variantItem) []variantItem { return mut(argonParams(salt)) }
	replace := func(key string, value []byte) func([]variantItem) []variantItem {
		return func(items []variantItem) []variantItem {
			for i := range items {
				if items[i].Key == key {
					items[i].Value = value
				}
			}
			return items
		}
	}
	drop := func(key string) func([]variantItem) []variantItem {
		return func(items []variantItem) []variantItem {
			return slices.DeleteFunc(items, func(it variantItem) bool { return it.Key == key })
		}
	}
	tests := []struct {
		name  string
		build build4
		route string
	}{
		{"the IV has the wrong length", build4{IVLength: 12, Payload: goodPayload()}, "readContainer4.FileCorrupt"},
		{"an unknown KDF", build4{Payload: goodPayload(), KDF: []variantItem{{"$UUID", variantBytes, bytes.Repeat([]byte{9}, 16)}}}, "KDF.Validate.KdfUnsupported"},
		{"Argon2 memory of 5 GiB", build4{Payload: goodPayload(), KDF: argon(replace("M", u64(5<<30)))}, "KDF.Validate.KdfParametersRejected"},
		{"Argon2 version 1.0", build4{Payload: goodPayload(), KDF: argon(replace("V", u32(0x10)))}, "KDF.Validate.KdfParametersRejected"},
		{"Argon2 without memory", build4{Payload: goodPayload(), KDF: argon(drop("M"))}, "kdfFromVariant.KdfParametersRejected"},
		{"Argon2 with a secret key", build4{Payload: goodPayload(), KDF: append(argonParams(salt), variantItem{"K", variantBytes, []byte("secret key")})}, "kdfFromVariant.KdfParametersRejected"},
		{"AES-KDF without rounds", build4{Payload: goodPayload(), KDF: []variantItem{{"$UUID", variantBytes, crypto.KDFAES[:]}, {"S", variantBytes, bytes.Repeat([]byte{1}, 32)}}}, "kdfFromVariant.KdfParametersRejected"},
		{"a KDF without an ID", build4{Payload: goodPayload(), KDF: argon(drop("$UUID"))}, "kdfFromVariant.FileCorrupt"},
		{"bad gzip", build4{Compression: CompressionGZip, Raw: true, Payload: []byte("this is not gzip data at all")}, "gunzip.FileCorrupt"},
		{"truncated gzip", build4{Compression: CompressionGZip, Payload: nil, Raw: true}, "gunzip.FileCorrupt"},
		{"an inner header that is cut off", build4{Payload: []byte{innerRandomStreamID, 4}}, "parseInnerHeader.FileCorrupt"},
		{"an inner field longer than the data", build4{Payload: append([]byte{innerRandomStreamKey}, u32(1000)...)}, "parseInnerHeader.FileCorrupt"},
		{"an inner stream ID of the wrong length", build4{Payload: append(field(innerRandomStreamID, []byte{1, 2}), field(innerEnd, nil)...)}, "parseInnerHeader.FileCorrupt"},
		{"no inner stream key", build4{Payload: append(field(innerRandomStreamID, u32(3)), field(innerEnd, nil)...)}, "parseInnerHeader.FileCorrupt"},
		{"no inner stream ID", build4{Payload: append(field(innerRandomStreamKey, []byte("k")), field(innerEnd, nil)...)}, "parseInnerHeader.FileCorrupt"},
		{"an empty attachment field", build4{Payload: append(append(field(innerRandomStreamID, u32(3)), field(innerRandomStreamKey, []byte("k"))...), append(field(innerBinary, nil), field(innerEnd, nil)...)...)}, "parseInnerHeader.FileCorrupt"},
		{"no end of the inner header", build4{Payload: append(field(innerRandomStreamID, u32(3)), field(innerRandomStreamKey, []byte("k"))...)}, "parseInnerHeader.FileCorrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.build.Password = "built"
			if tt.build.Compression == CompressionGZip && tt.name == "truncated gzip" {
				// a gzip stream cut after its header
				var buf bytes.Buffer
				zw := gzip.NewWriter(&buf)
				zw.Write(bytes.Repeat([]byte("x"), 1000)) //nolint:errcheck // a buffer cannot fail
				zw.Close()                                //nolint:errcheck
				tt.build.Payload = buf.Bytes()[:20]
			}
			_, err := readContainer4(context.Background(), tt.build.build(t), buildKey(t, "built"))
			if err == nil {
				t.Fatal("the file was accepted")
			}
			assertRoute(t, err, tt.route)
		})
	}
}

func TestGunzipLimit(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(bytes.Repeat([]byte("z"), 1000)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if out, err := gunzip(buf.Bytes(), 1000); err != nil || len(out) != 1000 {
		t.Errorf("at the limit: %d bytes, %v", len(out), err)
	}
	_, err := gunzip(buf.Bytes(), 999)
	assertRoute(t, err, "gunzip.FileCorrupt")
}

func TestReadBlocksLimit(t *testing.T) {
	base := bytes.Repeat([]byte{3}, 64)
	stream := writeBlocks(base, bytes.Repeat([]byte("d"), 100), 30) // 4 blocks and the end block
	if out, err := readBlocks(stream, base, 100); err != nil || len(out) != 100 {
		t.Errorf("at the limit: %d bytes, %v", len(out), err)
	}
	_, err := readBlocks(stream, base, 99)
	assertRoute(t, err, "readBlocks.FileCorrupt")
}

func TestParseHeader4Errors(t *testing.T) {
	cipher := field(fieldCipherID, crypto.CipherAES256[:])
	comp := field(fieldCompression, u32(1))
	seed := field(fieldMasterSeed, bytes.Repeat([]byte{1}, 32))
	iv := field(fieldEncryptionIV, bytes.Repeat([]byte{2}, 16))
	kdf := field(fieldKDFParameters, encodeVariant(argonParams(bytes.Repeat([]byte{3}, 16))...))
	full := [][]byte{cipher, comp, seed, iv, kdf}
	without := func(i int) [][]byte { return slices.Concat(full[:i], full[i+1:]) }
	version := Version{4, 0}

	if _, end, err := parseHeader4(rawHeader(full...), version); err != nil || end != len(rawHeader(full...)) {
		t.Fatalf("a complete header: end %d, %v", end, err)
	}
	for i, name := range []string{"cipher", "compression", "master seed", "initialization vector", "key-derivation parameters"} {
		_, _, err := parseHeader4(rawHeader(without(i)...), version)
		assertRoute(t, err, "parseHeader4.FileCorrupt")
		if err != nil && !strings.Contains(err.Error(), name) {
			t.Errorf("the error for a missing %s says %q", name, err)
		}
	}
	bad := map[string][]byte{
		"a cipher of 15 bytes":        field(fieldCipherID, make([]byte, 15)),
		"compression of 3 bytes":      field(fieldCompression, make([]byte, 3)),
		"a master seed of 31 bytes":   field(fieldMasterSeed, make([]byte, 31)),
		"a bad KDF dictionary":        field(fieldKDFParameters, []byte{0, 1}),
		"a bad public custom data":    field(fieldPublicCustomData, []byte{9, 9, 9}),
		"a field that is cut off":     append([]byte{fieldComment}, u32(100)...),
		"a field of more than 4 MiB":  field(200, make([]byte, maxHeaderField+1)),
		"a header that is cut off":    {fieldComment, 1},
		"the end field is missing":    nil,
		"unknown compression":         field(fieldCompression, u32(5)),
		"a header of more than 8 MiB": slices.Concat(field(200, make([]byte, 3<<20)), field(201, make([]byte, 3<<20)), field(202, make([]byte, 3<<20))),
	}
	for name, f := range bad {
		t.Run(name, func(t *testing.T) {
			data := rawHeader(cipher, comp, seed, iv, kdf, f) // the field is last, so that a good one before it does not hide it
			if name == "the end field is missing" {
				data = data[:len(data)-len(field(fieldEnd, []byte("\r\n\r\n")))]
			}
			if name == "unknown compression" || name == "a cipher of 15 bytes" || name == "compression of 3 bytes" || name == "a master seed of 31 bytes" {
				data = rawHeader(f) // the bad field alone, before the others
			}
			_, _, err := parseHeader4(data, version)
			if err == nil {
				t.Fatal("accepted")
			}
			switch name {
			case "unknown compression":
				assertRoute(t, err, "parseHeader4.CompressionUnsupported")
			case "a bad KDF dictionary", "a bad public custom data":
				assertRoute(t, err, "parseHeader4->parseVariantDictionary.FileCorrupt")
			default:
				assertRoute(t, err, "parseHeader4.FileCorrupt")
			}
		})
	}
	// A Public custom data field with several items.
	pcd := field(fieldPublicCustomData, encodeVariant(variantItem{"a", variantString, []byte("x")}, variantItem{"b", variantUInt64, u64(7)}))
	h, _, err := parseHeader4(rawHeader(append(slices.Clone(full), pcd)...), version)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.PublicCustomData) != 2 || h.PublicCustomData[0].Key != "a" || h.PublicCustomData[1].Type != variantUInt64 {
		t.Errorf("public custom data = %+v", h.PublicCustomData)
	}
}

func TestParseVariantDictionary(t *testing.T) {
	good := encodeVariant(
		variantItem{"u32", variantUInt32, u32(7)}, variantItem{"u64", variantUInt64, u64(1 << 40)}, variantItem{"bool", variantBool, []byte{1}},
		variantItem{"i32", variantInt32, u32(0xFFFFFFFF)}, variantItem{"i64", variantInt64, u64(5)},
		variantItem{"str", variantString, []byte("héllo")}, variantItem{"bytes", variantBytes, []byte{1, 2, 3}},
	)
	items, err := parseVariantDictionary(good)
	if err != nil || len(items) != 7 || items[5].Key != "str" || string(items[5].Value) != "héllo" {
		t.Fatalf("good dictionary: %d items, %v", len(items), err)
	}
	d := newVariantDict(items)
	if v, ok := d.uint32Of("u32"); !ok || v != 7 {
		t.Errorf("uint32Of = %d, %v", v, ok)
	}
	if v, ok := d.uint64Of("u64"); !ok || v != 1<<40 {
		t.Errorf("uint64Of = %d, %v", v, ok)
	}
	if v, ok := d.bytesOf("bytes"); !ok || !bytes.Equal(v, []byte{1, 2, 3}) {
		t.Errorf("bytesOf = %v, %v", v, ok)
	}
	if _, ok := d.uint32Of("u64"); ok {
		t.Error("uint32Of must not read a UInt64")
	}
	if _, ok := d.uint64Of("missing"); ok {
		t.Error("uint64Of found a missing key")
	}
	if _, ok := d.bytesOf("str"); ok {
		t.Error("bytesOf must not read a string")
	}

	many := make([]variantItem, maxVariantItems+1)
	for i := range many {
		many[i] = variantItem{"k", variantBool, []byte{0}}
	}
	longKey := append([]byte{0, 1, variantBool}, u32(maxVariantKey+1)...)
	longValue := append(append([]byte{0, 1, variantBytes}, u32(1)...), 'k')
	longValue = append(longValue, u32(maxVariantValue+1)...)
	bad := map[string]struct {
		data  []byte
		route string
	}{
		"too short":                      {[]byte{0, 1}, "parseVariantDictionary.FileCorrupt"},
		"an unknown version":             {[]byte{0, 2, 0}, "parseVariantDictionary.FileCorrupt"},
		"no end":                         {good[:len(good)-1], "parseVariantDictionary.FileCorrupt"},
		"cut in a key":                   {good[:8], "parseVariantDictionary.FileCorrupt"},
		"cut in a value":                 {good[:16], "parseVariantDictionary.FileCorrupt"},
		"a key of more than 1 KiB":       {longKey, "parseVariantDictionary.FileCorrupt"},
		"a value of more than 1 MiB":     {longValue, "parseVariantDictionary.FileCorrupt"},
		"more than 1024 items":           {encodeVariant(many...), "parseVariantDictionary.FileCorrupt"},
		"an unknown type":                {encodeVariant(variantItem{"k", 0x77, []byte{1}}), "checkVariantSize.FileCorrupt"},
		"a UInt32 of 3 bytes":            {encodeVariant(variantItem{"k", variantUInt32, []byte{1, 2, 3}}), "checkVariantSize.FileCorrupt"},
		"a UInt64 of 4 bytes":            {encodeVariant(variantItem{"k", variantUInt64, u32(1)}), "checkVariantSize.FileCorrupt"},
		"a Bool of 2 bytes":              {encodeVariant(variantItem{"k", variantBool, []byte{1, 1}}), "checkVariantSize.FileCorrupt"},
		"an Int32 of 8 bytes":            {encodeVariant(variantItem{"k", variantInt32, u64(1)}), "checkVariantSize.FileCorrupt"},
		"a key length that is cut off":   {[]byte{0, 1, variantBool, 1, 0}, "parseVariantDictionary.FileCorrupt"},
		"a value length that is cut off": {append([]byte{0, 1, variantBool}, append(u32(1), 'k', 1, 0)...), "parseVariantDictionary.FileCorrupt"},
	}
	for name, tt := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := parseVariantDictionary(tt.data)
			if err == nil {
				t.Fatal("accepted")
			}
			assertRoute(t, err, tt.route)
		})
	}
}

func TestKDFFromVariantUnknownFunction(t *testing.T) {
	d := newVariantDict([]variantItem{{"$UUID", variantBytes, bytes.Repeat([]byte{9}, 16)}})
	k, err := kdfFromVariant(d)
	if err != nil || k.ID[0] != 9 {
		t.Fatalf("kdfFromVariant = %+v, %v", k, err)
	}
	assertRoute(t, k.Validate(), "KDF.Validate.KdfUnsupported")
	short := newVariantDict([]variantItem{{"$UUID", variantBytes, []byte{1, 2, 3}}})
	_, err = kdfFromVariant(short)
	assertRoute(t, err, "kdfFromVariant.FileCorrupt")
	_ = sha256.Size
}
