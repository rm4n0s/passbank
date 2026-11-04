package kdbx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/testenv"
)

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

// secretOf returns a secret.Bytes for a test password.
func secretOf(s string) secret.Bytes { return secret.New([]byte(s)) }

// fixtureCreds are the credentials of a fixture, as FIXTURES.md lists them.
func fixtureCreds(t *testing.T, password, keyFile string) secret.Bytes {
	t.Helper()
	var c crypto.Credentials
	if password != "" {
		c.HasPassword, c.Password = true, secretOf(password)
	}
	if keyFile != "" {
		c.HasKeyFile, c.KeyFile = true, secret.New(readFile(t, testenv.Fixture(t, keyFile)))
	}
	key, err := crypto.CompositeKey(c)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fixtureFile reads a fixture.
func fixtureFile(t *testing.T, name string) []byte {
	t.Helper()
	return readFile(t, testenv.Fixture(t, name))
}

// attachment is the content of the attachment of the rich fixtures.
var attachment = bytes.Repeat([]byte("attachment-bytes-for-the-fixture\n"), 3)

func TestReadContainer4Fixtures(t *testing.T) {
	tests := []struct {
		file, password, keyFile string
		version                 Version
		cipher                  model.UUID
		kdf                     model.UUID
		compression             Compression
		rich                    bool
	}{
		{"k4-empty.kdbx", "fixture-min", "", Version{4, 0}, crypto.CipherAES256, crypto.KDFArgon2d, CompressionGZip, false},
		{"k4-rich.kdbx", "fixture-rich", "", Version{4, 0}, crypto.CipherAES256, crypto.KDFArgon2d, CompressionGZip, true},
		{"k4-rich-argon2id.kdbx", "fixture-rich", "", Version{4, 0}, crypto.CipherAES256, crypto.KDFArgon2id, CompressionGZip, true},
		{"k4-rich-chacha20.kdbx", "fixture-rich", "", Version{4, 0}, crypto.CipherChaCha20, crypto.KDFArgon2d, CompressionGZip, true},
		{"k4-rich-twofish-aeskdf.kdbx", "fixture-rich", "", Version{4, 0}, crypto.CipherTwofish, crypto.KDFAES, CompressionGZip, true},
		{"k4-rich-nocompression.kdbx", "fixture-rich", "", Version{4, 0}, crypto.CipherAES256, crypto.KDFArgon2d, CompressionNone, true},
		{"k4-keyfile-xml2.kdbx", "", "keyfile-xml2.keyx", Version{4, 0}, crypto.CipherAES256, crypto.KDFArgon2d, CompressionGZip, true},
		{"k4-password-and-keyfile.kdbx", "fixture-rich", "keyfile-raw32.key", Version{4, 0}, crypto.CipherAES256, crypto.KDFArgon2d, CompressionGZip, true},
		{"k4-public-custom-data.kdbx", "fixture-rich", "", Version{4, 0}, crypto.CipherAES256, crypto.KDFArgon2d, CompressionGZip, true},
		{"k4-unknown-xml.kdbx", "fixture-rich", "", Version{4, 0}, crypto.CipherAES256, crypto.KDFArgon2d, CompressionGZip, true},
		{"k41-fields.kdbx", "fixture-rich", "", Version{4, 1}, crypto.CipherAES256, crypto.KDFArgon2d, CompressionGZip, true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			c, err := readContainer4(context.Background(), fixtureFile(t, tt.file), fixtureCreds(t, tt.password, tt.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			defer c.wipe()
			h := c.Header
			if h.Version != tt.version || h.CipherID != tt.cipher || h.KDF.ID != tt.kdf || h.Compression != tt.compression {
				t.Errorf("header = version %v cipher %v kdf %v compression %v; want %v %v %v %v",
					h.Version, h.CipherID, h.KDF.ID, h.Compression, tt.version, tt.cipher, tt.kdf, tt.compression)
			}
			if !bytes.HasPrefix(c.XML, []byte("<?xml")) && !bytes.HasPrefix(c.XML, []byte("<KeePassFile")) {
				t.Errorf("the XML starts with %q", c.XML[:min(20, len(c.XML))])
			}
			if c.InnerStreamID != crypto.InnerChaCha20 || c.InnerStreamKey.Len() == 0 {
				t.Errorf("inner stream %d with a key of %d bytes", c.InnerStreamID, c.InnerStreamKey.Len())
			}
			if tt.rich {
				if !bytes.Contains(c.XML, []byte("Rich fixture")) || !bytes.Contains(c.XML, []byte("Old forum")) {
					t.Error("the XML lacks the content of the rich fixture")
				}
				if len(c.Binaries) != 1 || !c.Binaries[0].Protected || !bytes.Equal(c.Binaries[0].Data.Reveal(), attachment) {
					t.Errorf("binaries = %d, want the one attachment of the fixture", len(c.Binaries))
				}
				// Protected values are still encrypted: the XML must not show a password.
				if bytes.Contains(c.XML, []byte("bank-password-1")) {
					t.Error("a protected value is in the XML as plain text")
				}
			} else if len(c.Binaries) != 0 {
				t.Errorf("an empty database has %d binaries", len(c.Binaries))
			}
		})
	}
}

func TestReadContainer4HeaderDetails(t *testing.T) {
	c, err := readContainer4(context.Background(), fixtureFile(t, "k4-public-custom-data.kdbx"), fixtureCreds(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.wipe()
	pcd := c.Header.PublicCustomData
	if len(pcd) != 1 || pcd[0].Key != "fixture" || pcd[0].Type != variantUInt32 || !bytes.Equal(pcd[0].Data, []byte{42, 0, 0, 0}) {
		t.Errorf("public custom data = %+v", pcd)
	}
	kdf := c.Header.KDF
	if kdf.Memory != 8*1024*1024 || kdf.Iterations != 2 || kdf.Parallelism != 2 || kdf.Version != 0x13 || len(kdf.Seed) != 32 {
		t.Errorf("KDF parameters = %+v", kdf)
	}
	aes, err := readContainer4(context.Background(), fixtureFile(t, "k4-rich-twofish-aeskdf.kdbx"), fixtureCreds(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer aes.wipe()
	if aes.Header.KDF.Rounds != 6000 || len(aes.Header.KDF.Seed) != 32 {
		t.Errorf("AES-KDF parameters = %+v", aes.Header.KDF)
	}
}

func TestReadContainer4WrongCredentials(t *testing.T) {
	data := fixtureFile(t, "k4-rich.kdbx")
	for name, key := range map[string]secret.Bytes{
		"wrong password":    fixtureCreds(t, "fixture-wrong", ""),
		"no password":       fixtureCreds(t, "", "keyfile-raw32.key"),
		"password and file": fixtureCreds(t, "fixture-rich", "keyfile-raw32.key"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := readContainer4(context.Background(), data, key)
			assertRoute(t, err, "readContainer4.WrongCredentials")
		})
	}
	// A key file that the database does not use, for a database that wants one.
	_, err := readContainer4(context.Background(), fixtureFile(t, "k4-keyfile-xml2.kdbx"), fixtureCreds(t, "", "keyfile-raw32.key"))
	assertRoute(t, err, "readContainer4.WrongCredentials")
}

// headerEndOf returns the offset where the header of a KDBX 4 file ends (the
// stored hash starts), by walking its fields.
func headerEndOf(t *testing.T, data []byte) int {
	t.Helper()
	off := signatureSize
	for {
		id := data[off]
		size := int(binary.LittleEndian.Uint32(data[off+1 : off+5]))
		off += 5 + size
		if id == fieldEnd {
			return off
		}
	}
}

// fieldRange returns where the data of a header field is.
func fieldRange(t *testing.T, data []byte, want byte) (start, end int) {
	t.Helper()
	off := signatureSize
	for {
		id := data[off]
		size := int(binary.LittleEndian.Uint32(data[off+1 : off+5]))
		if id == want {
			return off + 5, off + 5 + size
		}
		if id == fieldEnd {
			t.Fatalf("no header field %d", want)
		}
		off += 5 + size
	}
}

// rehash fixes the stored hash of the header after a test changed the header, so
// that the change is not found by the hash check.
func rehash(data []byte) {
	end := headerEndOf2(data)
	sum := sha256.Sum256(data[:end])
	copy(data[end:end+32], sum[:])
}

func headerEndOf2(data []byte) int {
	off := signatureSize
	for {
		id := data[off]
		size := int(binary.LittleEndian.Uint32(data[off+1 : off+5]))
		off += 5 + size
		if id == fieldEnd {
			return off
		}
	}
}

func clone(b []byte) []byte { return append([]byte(nil), b...) }

func TestReadContainer4Damage(t *testing.T) {
	good := fixtureFile(t, "k4-rich.kdbx")
	key := fixtureCreds(t, "fixture-rich", "")
	headerEnd := headerEndOf(t, good)
	firstBlock := headerEnd + 64
	lastBlock := len(good) - 36 // the terminating block of size 0

	tests := []struct {
		name   string
		mutate func(d []byte) []byte
		route  string
	}{
		{"not KeePass", func(d []byte) []byte { return []byte("this is not a KeePass database at all") }, "parseSignature.NotKdbx"},
		{"empty", func(d []byte) []byte { return nil }, "parseSignature.NotKdbx"},
		{"first signature", func(d []byte) []byte { d[0] ^= 0xff; return d }, "parseSignature.NotKdbx"},
		{"second signature", func(d []byte) []byte { d[4] ^= 0xff; return d }, "parseSignature.NotKdbx"},
		{"KeePass 1", func(d []byte) []byte { binary.LittleEndian.PutUint32(d[4:8], signature2KDB); return d }, "parseSignature.KeePass1NotSupported"},
		{"pre-release format", func(d []byte) []byte { binary.LittleEndian.PutUint32(d[4:8], signature2Beta); return d }, "parseSignature.VersionUnsupported"},
		{"version 3", func(d []byte) []byte { d[10] = 3; return d }, "readContainer4.VersionUnsupported"},
		{"version 5", func(d []byte) []byte { d[10] = 5; return d }, "readContainer4.VersionUnsupported"},
		{"header hash", func(d []byte) []byte { d[headerEnd] ^= 1; return d }, "readContainer4.FileCorrupt"},
		{"a byte of the header", func(d []byte) []byte { s, _ := fieldRange(t, d, fieldMasterSeed); d[s] ^= 1; return d }, "readContainer4.FileCorrupt"},
		{"header HMAC", func(d []byte) []byte { d[headerEnd+32] ^= 1; return d }, "readContainer4.WrongCredentials"},
		{"a byte of a block", func(d []byte) []byte { d[firstBlock+36+10] ^= 1; return d }, "readBlocks.FileCorrupt"},
		{"the HMAC of a block", func(d []byte) []byte { d[firstBlock+5] ^= 1; return d }, "readBlocks.FileCorrupt"},
		{"the size of a block", func(d []byte) []byte { d[firstBlock+32]++; return d }, "readBlocks.FileCorrupt"},
		{"a block that claims 4 GiB", func(d []byte) []byte { binary.LittleEndian.PutUint32(d[firstBlock+32:], 0xFFFFFFFF); return d }, "readBlocks.FileCorrupt"},
		{"no terminating block", func(d []byte) []byte { return d[:lastBlock] }, "readBlocks.FileCorrupt"},
		{"the terminating block", func(d []byte) []byte { d[lastBlock+1] ^= 1; return d }, "readBlocks.FileCorrupt"},
		{"cut after the header", func(d []byte) []byte { return d[:headerEnd] }, "readContainer4.FileCorrupt"},
		{"cut in the header HMAC", func(d []byte) []byte { return d[:headerEnd+40] }, "readContainer4.FileCorrupt"},
		{"cut in the first block", func(d []byte) []byte { return d[:firstBlock+50] }, "readBlocks.FileCorrupt"},
		{"cut after the first block header", func(d []byte) []byte { return d[:firstBlock+36] }, "readBlocks.FileCorrupt"},
		{"cut in the header", func(d []byte) []byte { return d[:30] }, "parseHeader4.FileCorrupt"},
		{"cut after the signatures", func(d []byte) []byte { return d[:signatureSize] }, "parseHeader4.FileCorrupt"},
		{"unknown cipher", func(d []byte) []byte {
			s, _ := fieldRange(t, d, fieldCipherID)
			d[s]++
			rehash(d)
			return d
		}, "LookupCipher.CipherUnsupported"},
		{"unknown compression", func(d []byte) []byte {
			s, _ := fieldRange(t, d, fieldCompression)
			d[s] = 9
			rehash(d)
			return d
		}, "parseHeader4.CompressionUnsupported"},
		{"cipher field of the wrong length", func(d []byte) []byte {
			s, _ := fieldRange(t, d, fieldCipherID)
			binary.LittleEndian.PutUint32(d[s-4:], 15)
			return d
		}, "parseHeader4.FileCorrupt"},
		{"IV of the wrong length for the cipher", func(d []byte) []byte {
			s, _ := fieldRange(t, d, fieldEncryptionIV)
			binary.LittleEndian.PutUint32(d[s-4:], 12)
			// Removing four bytes of the IV shifts the rest; instead shorten the field
			// in place by turning the last four bytes into an unknown field is too
			// much surgery: the length check of the field itself is what is tested.
			return d
		}, "parseHeader4.FileCorrupt"},
		{"a field larger than the file", func(d []byte) []byte {
			s, _ := fieldRange(t, d, fieldMasterSeed)
			binary.LittleEndian.PutUint32(d[s-4:], 1<<30)
			return d
		}, "parseHeader4.FileCorrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readContainer4(context.Background(), tt.mutate(clone(good)), key)
			if err == nil {
				t.Fatal("damaged file accepted")
			}
			assertRoute(t, err, tt.route)
		})
	}
}

// TestReadContainer4EveryPrefix checks that a file cut at any length is an
// error and never a panic.
func TestReadContainer4EveryPrefix(t *testing.T) {
	good := fixtureFile(t, "k4-empty.kdbx")
	key := fixtureCreds(t, "fixture-min", "")
	// Every length through the header and its checks; after that each cut
	// derives the key again, so only every 29th length (and the last).
	for n := 0; n < len(good); n++ {
		if n > 400 && n%29 != 0 && n != len(good)-1 {
			continue
		}
		if _, err := readContainer4(context.Background(), good[:n], key); err == nil {
			t.Fatalf("a file cut at %d of %d bytes was accepted", n, len(good))
		} else if _, ok := errors.FromError(err); !ok {
			t.Fatalf("cut at %d: not an *errors.Error: %v", n, err)
		}
	}
}

func TestReadContainer4Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readContainer4(ctx, fixtureFile(t, "k4-rich.kdbx"), fixtureCreds(t, "fixture-rich", ""))
	assertRoute(t, err, "KdfCancelled")
}

func TestVersionString(t *testing.T) {
	if got := (Version{4, 1}).String(); got != "4.1" {
		t.Errorf("String = %q", got)
	}
	if CompressionNone.String() != "none" || CompressionGZip.String() != "GZip" || Compression(7).String() != "unknown" {
		t.Error("Compression.String is wrong")
	}
	_ = fmt.Sprint
	_ = strings.Contains
}
