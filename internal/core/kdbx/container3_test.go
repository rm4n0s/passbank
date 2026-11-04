package kdbx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"regexp"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

var headerHashPattern = regexp.MustCompile(`<HeaderHash>([^<]*)</HeaderHash>`)

func TestReadContainer3Fixtures(t *testing.T) {
	tests := []struct {
		file, password, keyFile string
		cipher                  model.UUID
		rounds                  uint64 // 0: not checked
		compression             Compression
		inner                   uint32
	}{
		{"k31-empty.kdbx", "fixture-min", "", crypto.CipherAES256, 0, CompressionGZip, crypto.InnerSalsa20},
		{"k31-rich.kdbx", "fixture-rich", "", crypto.CipherAES256, 0, CompressionGZip, crypto.InnerSalsa20},
		{"k31-keyfile.kdbx", "", "keyfile-k31.keyx", crypto.CipherAES256, 0, CompressionGZip, crypto.InnerSalsa20},
		{"k31-password-and-keyfile.kdbx", "fixture-rich", "keyfile-xml2.keyx", crypto.CipherAES256, 0, CompressionGZip, crypto.InnerSalsa20},
		{"ext/rs-password.kdbx", "demopass", "", crypto.CipherAES256, 6000, CompressionGZip, crypto.InnerSalsa20},
		{"ext/rs-attachments.kdbx", "demopass", "", crypto.CipherAES256, 1000000, CompressionGZip, crypto.InnerSalsa20},
		{"ext/rs-chacha20-inner.kdbx", "password", "", crypto.CipherAES256, 6000, CompressionGZip, crypto.InnerChaCha20},
		{"ext/rs-keyfile.kdbx", "", "ext/rs-keyfile.key", crypto.CipherAES256, 100, CompressionGZip, crypto.InnerSalsa20},
		{"ext/rs-keyfile-xml.kdbx", "", "ext/rs-keyfile-xml.key", crypto.CipherAES256, 100, CompressionGZip, crypto.InnerSalsa20},
		{"ext/go-example.kdbx", "abcdefg12345678", "", crypto.CipherAES256, 6000, CompressionGZip, crypto.InnerSalsa20},
		{"ext/go-example-key.kdbx", "abcdefg12345678", "ext/go-example-key.key", crypto.CipherAES256, 6000, CompressionGZip, crypto.InnerSalsa20},
		{"ext/go-protected-binary.kdbx", "123", "", crypto.CipherAES256, 300000, CompressionGZip, crypto.InnerSalsa20},
		{"ext/go-chacha.kdbx", "abcdefg12345678", "", crypto.CipherChaCha20, 60000, CompressionGZip, crypto.InnerSalsa20},
		{"ext/go-twofish.kdbx", "test1234test", "", crypto.CipherTwofish, 600000, CompressionGZip, crypto.InnerSalsa20},
		{"ext/web-demo.kdbx", "demo", "ext/web-demo.key", crypto.CipherAES256, 6000, CompressionGZip, crypto.InnerSalsa20},
		{"ext/web-cyrillic.kdbx", "пароль", "", crypto.CipherAES256, 100, CompressionNone, crypto.InnerSalsa20},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			c, err := readContainer3(context.Background(), fixtureFile(t, tt.file), fixtureCreds(t, tt.password, tt.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			defer c.wipe()
			h := c.Header
			if h.Version != (Version{3, 1}) || h.CipherID != tt.cipher || h.Compression != tt.compression || h.KDF.ID != crypto.KDFAES ||
				(tt.rounds != 0 && h.KDF.Rounds != tt.rounds) || len(h.KDF.Seed) != 32 || len(h.MasterSeed) != 32 {
				t.Errorf("header = %+v", h)
			}
			if c.InnerStreamID != tt.inner || c.InnerStreamKey.Len() == 0 {
				t.Errorf("inner stream %d with a key of %d bytes", c.InnerStreamID, c.InnerStreamKey.Len())
			}
			if !bytes.Contains(c.XML, []byte("<KeePassFile")) || !bytes.Contains(c.XML, []byte("</KeePassFile>")) {
				t.Fatalf("the XML starts with %q", c.XML[:min(40, len(c.XML))])
			}
			// The hash of the header that Passbank computed is the one the file carries.
			m := headerHashPattern.FindSubmatch(c.XML)
			if m == nil {
				t.Fatal("the XML has no HeaderHash")
			}
			if want := base64.StdEncoding.EncodeToString(c.HeaderHash); string(m[1]) != want {
				t.Errorf("HeaderHash in the XML is %s, the header hashes to %s", m[1], want)
			}
			if len(c.Binaries) != 0 {
				t.Errorf("a KDBX 3 container has %d inner-header binaries", len(c.Binaries))
			}
		})
	}
}

func TestReadContainer3Rich(t *testing.T) {
	c, err := readContainer3(context.Background(), fixtureFile(t, "k31-rich.kdbx"), fixtureCreds(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.wipe()
	for _, want := range []string{"Rich fixture", "Old forum", "Ünïcödé ключ 鍵", "<Binaries>"} {
		if !bytes.Contains(c.XML, []byte(want)) {
			t.Errorf("the XML lacks %q", want)
		}
	}
	// Protected values are still encrypted.
	if bytes.Contains(c.XML, []byte("bank-password-1")) {
		t.Error("a protected value is in the XML as plain text")
	}
}

func TestReadContainer3WrongCredentials(t *testing.T) {
	for _, tt := range []struct{ file, password, keyFile string }{
		{"k31-rich.kdbx", "wrong", ""},
		{"k31-rich.kdbx", "", "keyfile-k31.keyx"},
		{"k31-keyfile.kdbx", "fixture-rich", "keyfile-k31.keyx"},
		{"k31-keyfile.kdbx", "", "keyfile-xml2.keyx"},
		{"ext/go-twofish.kdbx", "abcdefg12345678", ""},
		{"ext/go-chacha.kdbx", "wrong", ""},
		{"ext/rs-password.kdbx", "demopasss", ""},
	} {
		_, err := readContainer3(context.Background(), fixtureFile(t, tt.file), fixtureCreds(t, tt.password, tt.keyFile))
		assertRoute(t, err, "readContainer3.WrongCredentials")
	}
}

func TestReadContainer3Built(t *testing.T) {
	for _, v := range []Version{{2, 0}, {2, 1}, {3, 0}, {3, 1}, {3, 7}} {
		for _, cipher := range []model.UUID{crypto.CipherAES256, crypto.CipherTwofish, crypto.CipherChaCha20} {
			for _, comp := range []Compression{CompressionNone, CompressionGZip} {
				for _, blockSize := range []int{5, 1 << 20} {
					name := v.String() + "/" + cipher.String()[:4] + "/" + comp.String() + "/" + string(rune('0'+blockSize%10))
					t.Run(name, func(t *testing.T) {
						file := build3{Major: v.Major, Minor: v.Minor, Password: "built", Cipher: cipher, Compression: comp, BlockSize: blockSize}.build(t)
						c, err := readContainer3(context.Background(), file, buildKey(t, "built"))
						if err != nil {
							t.Fatal(err)
						}
						defer c.wipe()
						if string(c.XML) != buildXML {
							t.Errorf("XML = %q", c.XML)
						}
						if c.Header.Version != v || c.Header.CipherID != cipher || c.Header.Compression != comp || c.Header.KDF.Rounds != 10 {
							t.Errorf("header = %+v", c.Header)
						}
						if c.InnerStreamID != crypto.InnerSalsa20 || len(c.InnerStreamKey.Reveal()) != 32 {
							t.Errorf("inner stream %d, key %d bytes", c.InnerStreamID, len(c.InnerStreamKey.Reveal()))
						}
						want := sha256.Sum256(file[:headerLength(t, file)])
						if !bytes.Equal(c.HeaderHash, want[:]) {
							t.Error("HeaderHash is not the hash of the header")
						}
						if _, err := readContainer3(context.Background(), file, buildKey(t, "wrong")); err == nil {
							t.Error("a wrong password opened the file")
						}
					})
				}
			}
		}
	}
}

// headerLength returns the length of the header of a built KDBX 3 file.
func headerLength(t *testing.T, file []byte) int {
	t.Helper()
	off := signatureSize
	for {
		id := file[off]
		size := int(binary.LittleEndian.Uint16(file[off+1:]))
		off += 3 + size
		if id == fieldEnd {
			return off
		}
	}
}

func TestReadContainer3InnerStreamIDs(t *testing.T) {
	for _, id := range []uint32{crypto.InnerSalsa20, crypto.InnerChaCha20, crypto.InnerArcFour, 77} {
		file := build3{Password: "built", InnerStream: id}.build(t)
		c, err := readContainer3(context.Background(), file, buildKey(t, "built"))
		if err != nil {
			t.Fatalf("inner stream %d: %v", id, err)
		}
		// The container reports the ID; the XML reader refuses the ones it cannot do.
		if c.InnerStreamID != id {
			t.Errorf("inner stream = %d, want %d", c.InnerStreamID, id)
		}
		c.wipe()
	}
}

func TestReadContainer3Header(t *testing.T) {
	extra := []HeaderField{{ID: fieldComment, Data: []byte("a comment")}, {ID: 99, Data: []byte("unknown field")}, {ID: 100}}
	file := build3{Password: "built", Extra: extra}.build(t)
	c, err := readContainer3(context.Background(), file, buildKey(t, "built"))
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
	// The AES-KDF IDs of KDBX 3 and the fields' order do not matter: the fixtures
	// have other orders than the builder.
}

func TestReadContainer3Damage(t *testing.T) {
	bad := func(id byte, data []byte) map[byte][]byte { return map[byte][]byte{id: data} }
	badBlocks := func(mut func([]byte) []byte) []byte {
		return mut(hashedBlocks([]byte(buildXML), 20))
	}
	tests := []struct {
		name  string
		build build3
		route string
	}{
		{"cipher of 15 bytes", build3{Override: bad(fieldCipherID, make([]byte, 15))}, "parseHeader3.FileCorrupt"},
		{"compression of 3 bytes", build3{Override: bad(fieldCompression, make([]byte, 3))}, "parseHeader3.FileCorrupt"},
		{"compression 2", build3{Override: bad(fieldCompression, u32(2))}, "parseHeader3.CompressionUnsupported"},
		{"master seed of 31 bytes", build3{Override: bad(fieldMasterSeed, make([]byte, 31))}, "parseHeader3.FileCorrupt"},
		{"transform seed of 31 bytes", build3{Override: bad(fieldTransformSeed, make([]byte, 31))}, "parseHeader3.FileCorrupt"},
		{"rounds of 4 bytes", build3{Override: bad(fieldTransformRounds, u32(5))}, "parseHeader3.FileCorrupt"},
		{"empty inner stream key", build3{Override: bad(fieldProtectedStreamKey, nil)}, "parseHeader3.FileCorrupt"},
		{"start bytes of 31 bytes", build3{Override: bad(fieldStreamStartBytes, make([]byte, 31))}, "parseHeader3.FileCorrupt"},
		{"inner stream ID of 2 bytes", build3{Override: bad(fieldInnerRandomStream, []byte{1, 0})}, "parseHeader3.FileCorrupt"},
		{"a field twice", build3{Extra: []HeaderField{{ID: fieldMasterSeed, Data: make([]byte, 32)}}}, "parseHeader3.FileCorrupt"},
		{"an unknown cipher", build3{Override: bad(fieldCipherID, bytes.Repeat([]byte{9}, 16))}, "LookupCipher.CipherUnsupported"},
		{"an IV of the wrong length", build3{Override: bad(fieldEncryptionIV, make([]byte, 12))}, "readContainer3.FileCorrupt"},
		{"zero rounds", build3{Override: bad(fieldTransformRounds, u64(0))}, "KDF.Validate.KdfParametersRejected"},
		{"too many rounds", build3{Override: bad(fieldTransformRounds, u64(1<<40))}, "KDF.Validate.KdfParametersRejected"},
		{"wrong start bytes", build3{StartBytes: bytes.Repeat([]byte{1}, 32)}, "readContainer3.WrongCredentials"},
		{"a payload shorter than the start bytes", build3{RawPayload: []byte("short")}, "readContainer3.WrongCredentials"},
		{"a wrong block index", build3{Blocks: badBlocks(func(b []byte) []byte { b[0] = 5; return b })}, "readHashedBlocks.FileCorrupt"},
		{"a wrong block hash", build3{Blocks: badBlocks(func(b []byte) []byte { b[10] ^= 1; return b })}, "readHashedBlocks.FileCorrupt"},
		{"a damaged block", build3{Blocks: badBlocks(func(b []byte) []byte { b[41] ^= 1; return b })}, "readHashedBlocks.FileCorrupt"},
		{"a last block with a hash", build3{Blocks: badBlocks(func(b []byte) []byte { b[len(b)-10] = 1; return b })}, "readHashedBlocks.FileCorrupt"},
		{"no last block", build3{Blocks: badBlocks(func(b []byte) []byte { return b[:len(b)-40] })}, "readHashedBlocks.FileCorrupt"},
		{"a block cut off", build3{Blocks: badBlocks(func(b []byte) []byte { return b[:60] })}, "readHashedBlocks.FileCorrupt"},
		{"bad gzip", build3{Compression: CompressionGZip, Blocks: hashedBlocks([]byte("this is not gzip data"), 100)}, "gunzip.FileCorrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.build.Password = "built"
			_, err := readContainer3(context.Background(), tt.build.build(t), buildKey(t, "built"))
			if err == nil {
				t.Fatal("the file was accepted")
			}
			assertRoute(t, err, tt.route)
		})
	}

	t.Run("missing fields", func(t *testing.T) {
		for _, id := range []byte{fieldCipherID, fieldCompression, fieldMasterSeed, fieldTransformSeed, fieldTransformRounds, fieldEncryptionIV, fieldProtectedStreamKey, fieldStreamStartBytes, fieldInnerRandomStream} {
			_, err := readContainer3(context.Background(), build3{Password: "built", Skip: []byte{id}}.build(t), buildKey(t, "built"))
			assertRoute(t, err, "parseHeader3.FileCorrupt")
		}
	})
}

func TestReadContainer3DamageOfBytes(t *testing.T) {
	good := build3{Password: "built", BlockSize: 20}.build(t)
	key := buildKey(t, "built")
	with := func(mut func([]byte) []byte) []byte { return mut(bytes.Clone(good)) }
	tests := []struct {
		name  string
		data  []byte
		route string
	}{
		{"empty", nil, "readContainer3->parseSignature.NotKdbx"},
		{"not KeePass", []byte("this is a text file, not a database"), "parseSignature.NotKdbx"},
		{"a KDBX 4 file", with(func(b []byte) []byte { b[10] = 4; return b }), "readContainer3.VersionUnsupported"},
		{"a KDBX 1 file", with(func(b []byte) []byte { b[10] = 1; return b }), "readContainer3.VersionUnsupported"},
		{"a header cut inside a field", good[:30], "parseHeader3.FileCorrupt"},
		{"a header cut inside a field length", good[:13], "parseHeader3.FileCorrupt"},
		{"a header that never ends", good[:headerLength(t, good)-5], "parseHeader3.FileCorrupt"},
		{"nothing after the header", good[:headerLength(t, good)], "readContainer3->Cipher.DecryptRaw.CiphertextInvalid"},
		{"a payload cut by a byte", good[:len(good)-1], "Cipher.DecryptRaw.CiphertextInvalid"},
		{"a payload cut by a block", good[:len(good)-16], "Cipher.Unpad.PaddingInvalid"},
		{"a flipped bit in the cipher ID", with(func(b []byte) []byte { b[30] ^= 1; return b }), "LookupCipher.CipherUnsupported"},
		{"a flipped bit in the first cipher block", with(func(b []byte) []byte { b[headerLength(t, b)+3] ^= 1; return b }), "readContainer3.WrongCredentials"},
		{"a flipped bit in the middle of the payload", with(func(b []byte) []byte { b[headerLength(t, b)+60] ^= 1; return b }), "readHashedBlocks.FileCorrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readContainer3(context.Background(), tt.data, key)
			if err == nil {
				t.Fatal("the file was accepted")
			}
			assertRoute(t, err, tt.route)
		})
	}
	// The last block's ciphertext decides the padding: a wrong one is PaddingInvalid
	// (or, one time in 256, a padding that is valid by chance and a damaged block).
	damaged := with(func(b []byte) []byte { b[len(b)-1] ^= 1; return b })
	if _, err := readContainer3(context.Background(), damaged, key); err == nil {
		t.Error("a damaged last block was accepted")
	} else {
		assertRoute(t, err, "Cipher.Unpad.PaddingInvalid")
	}
}

func TestReadContainer3EveryPrefix(t *testing.T) {
	good := build3{Password: "built", BlockSize: 30}.build(t)
	key := buildKey(t, "built")
	for n := 0; n < len(good); n++ {
		if _, err := readContainer3(context.Background(), good[:n], key); err == nil {
			t.Fatalf("a file cut at %d of %d bytes was accepted", n, len(good))
		}
	}
}

func TestReadContainer3Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readContainer3(ctx, fixtureFile(t, "k31-rich.kdbx"), fixtureCreds(t, "fixture-rich", ""))
	assertRoute(t, err, "KdfCancelled")
}

func TestReadHashedBlocks(t *testing.T) {
	data := bytes.Repeat([]byte("d"), 100)
	stream := hashedBlocks(data, 30)
	if out, err := readHashedBlocks(stream, 100); err != nil || !bytes.Equal(out, data) {
		t.Errorf("at the limit: %d bytes, %v", len(out), err)
	}
	_, err := readHashedBlocks(stream, 99)
	assertRoute(t, err, "readHashedBlocks.FileCorrupt")
	if out, err := readHashedBlocks(hashedBlocks(nil, 30), 100); err != nil || len(out) != 0 {
		t.Errorf("an empty stream: %d bytes, %v", len(out), err)
	}
	// A block that claims to be larger than the limit of a block.
	huge := hashedBlocks(data, 30)
	binary.LittleEndian.PutUint32(huge[36:40], maxBlock+1)
	_, err = readHashedBlocks(huge, 1<<30)
	assertRoute(t, err, "readHashedBlocks.FileCorrupt")
}

func TestReadContainer4AcceptsBothAESKDFIDs(t *testing.T) {
	salt := bytes.Repeat([]byte{7}, 32)
	for name, id := range map[string]model.UUID{"KDBX 3 ID": crypto.KDFAES, "KDBX 4 ID": crypto.KDFAESKdbx4} {
		t.Run(name, func(t *testing.T) {
			kdf := []variantItem{{"$UUID", variantBytes, id[:]}, {"S", variantBytes, salt}, {"R", variantUInt64, u64(1000)}}
			file := build4{Password: "built", Payload: goodPayload(), KDF: kdf}.build(t)
			c, err := readContainer4(context.Background(), file, buildKey(t, "built"))
			if err != nil {
				t.Fatal(err)
			}
			defer c.wipe()
			if c.Header.KDF.ID != crypto.KDFAES || c.Header.KDF.Rounds != 1000 || c.Header.KDF.Name() != "AES-KDF" {
				t.Errorf("KDF = %+v", c.Header.KDF)
			}
		})
	}
}
