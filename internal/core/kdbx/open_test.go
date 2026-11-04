package kdbx

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/testenv"
)

// credsOf makes the credentials of a fixture.
func credsOf(t *testing.T, password, keyFile string) crypto.Credentials {
	t.Helper()
	var c crypto.Credentials
	if password != "" {
		c.HasPassword, c.Password = true, secretOf(password)
	}
	if keyFile != "" {
		c.HasKeyFile, c.KeyFile = true, secret.New(readFile(t, testenv.Fixture(t, keyFile)))
	}
	return c
}

func TestOpenFixtures(t *testing.T) {
	tests := []struct {
		file, password, keyFile string
		version                 Version
		cipher, kdf             string
		compression             Compression
		rich                    bool
	}{
		{"k4-empty.kdbx", "fixture-min", "", Version{4, 0}, "AES-256", "Argon2d", CompressionGZip, false},
		{"k4-rich.kdbx", "fixture-rich", "", Version{4, 0}, "AES-256", "Argon2d", CompressionGZip, true},
		{"k4-rich-argon2id.kdbx", "fixture-rich", "", Version{4, 0}, "AES-256", "Argon2id", CompressionGZip, true},
		{"k4-rich-chacha20.kdbx", "fixture-rich", "", Version{4, 0}, "ChaCha20", "Argon2d", CompressionGZip, true},
		{"k4-rich-twofish-aeskdf.kdbx", "fixture-rich", "", Version{4, 0}, "Twofish", "AES-KDF", CompressionGZip, true},
		{"k4-rich-nocompression.kdbx", "fixture-rich", "", Version{4, 0}, "AES-256", "Argon2d", CompressionNone, true},
		{"k4-keyfile-xml2.kdbx", "", "keyfile-xml2.keyx", Version{4, 0}, "AES-256", "Argon2d", CompressionGZip, true},
		{"k4-password-and-keyfile.kdbx", "fixture-rich", "keyfile-raw32.key", Version{4, 0}, "AES-256", "Argon2d", CompressionGZip, true},
		{"k4-public-custom-data.kdbx", "fixture-rich", "", Version{4, 0}, "AES-256", "Argon2d", CompressionGZip, true},
		{"k4-unknown-xml.kdbx", "fixture-rich", "", Version{4, 0}, "AES-256", "Argon2d", CompressionGZip, true},
		{"k41-fields.kdbx", "fixture-rich", "", Version{4, 1}, "AES-256", "Argon2d", CompressionGZip, true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			db, info, err := Open(context.Background(), testenv.Fixture(t, tt.file), credsOf(t, tt.password, tt.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Wipe()
			if info.Version != tt.version || info.Cipher != tt.cipher || info.KDF.Name() != tt.kdf || info.Compression != tt.compression ||
				info.InnerStreamID != crypto.InnerChaCha20 || info.InnerStream != "ChaCha20" {
				t.Errorf("info = %+v", info)
			}
			if tt.rich {
				// Groups: Root, Email, Nested, Recycle Bin. Entries: Bank, Ünïcödé, Mail, Old forum.
				if info.Groups != 4 || info.Entries != 4 || info.HistoryItems != 2 || info.Attachments != 1 {
					t.Errorf("counts = %d groups, %d entries, %d history, %d attachments", info.Groups, info.Entries, info.HistoryItems, info.Attachments)
				}
				checkRich(t, db)
			} else if info.Groups != 1 || info.Entries != 0 {
				t.Errorf("counts of an empty database = %d groups, %d entries", info.Groups, info.Entries)
			}
		})
	}
}

func TestOpenKDFParameters(t *testing.T) {
	_, info, err := Open(context.Background(), testenv.Fixture(t, "k4-rich.kdbx"), credsOf(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	if k := info.KDF; k.Memory != 8*1024*1024 || k.Iterations != 2 || k.Parallelism != 2 {
		t.Errorf("KDF = %+v", k)
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()
	notKdbx := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(notKdbx, []byte("just some text, not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		path  string
		creds crypto.Credentials
		route string
	}{
		{"a missing file", filepath.Join(dir, "missing.kdbx"), credsOf(t, "x", ""), "Open->readDatabaseFile.FileReadFailed"},
		{"a folder", dir, credsOf(t, "x", ""), "Open->readDatabaseFile.FileReadFailed"},
		{"not a database", notKdbx, credsOf(t, "x", ""), "OpenBytes->parseSignature.NotKdbx"},
		{"a wrong password", testenv.Fixture(t, "k4-rich.kdbx"), credsOf(t, "wrong", ""), "WrongCredentials"},
		{"no credentials", testenv.Fixture(t, "k4-rich.kdbx"), crypto.Credentials{}, "OpenBytes->CompositeKey.NoCredentials"},
		{"a missing key file", testenv.Fixture(t, "k4-password-and-keyfile.kdbx"), credsOf(t, "fixture-rich", ""), "WrongCredentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, err := Open(context.Background(), tt.path, tt.creds)
			if err == nil {
				db.Wipe()
				t.Fatal("opened")
			}
			assertRoute(t, err, tt.route)
		})
	}
}

func TestOpenDoesNotChangeTheFile(t *testing.T) {
	path := testenv.Fixture(t, "k4-rich.kdbx")
	before := readFile(t, path)
	infoBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := Open(context.Background(), path, credsOf(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	db.Wipe()
	after := readFile(t, path)
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Error("Open changed the file")
	}
}

func TestOpenSignatureErrors(t *testing.T) {
	good := fixtureFile(t, "k4-rich.kdbx")
	creds := credsOf(t, "fixture-rich", "")
	tests := []struct {
		name  string
		data  []byte
		route string
	}{
		{"empty", nil, "OpenBytes->parseSignature.NotKdbx"},
		{"KeePass 1", append([]byte{0x03, 0xd9, 0xa2, 0x9a, 0x65, 0xfb, 0x4b, 0xb5, 0, 0, 3, 0}, make([]byte, 100)...), "parseSignature.KeePass1NotSupported"},
		{"the pre-release format", append([]byte{0x03, 0xd9, 0xa2, 0x9a, 0x66, 0xfb, 0x4b, 0xb5, 0, 0, 3, 0}, make([]byte, 100)...), "parseSignature.VersionUnsupported"},
		{"KDBX 5", func() []byte { d := append([]byte(nil), good...); d[10], d[11] = 5, 0; return d }(), "OpenBytes.VersionUnsupported"},
		{"KDBX 1", func() []byte { d := append([]byte(nil), good...); d[10], d[11] = 1, 0; return d }(), "OpenBytes.VersionUnsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := OpenBytes(context.Background(), tt.data, creds)
			assertRoute(t, err, tt.route)
		})
	}
}

func TestReadDatabaseFileLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(path, make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := readDatabaseFile(context.Background(), path, 100); err != nil || len(data) != 100 {
		t.Errorf("at the limit: %d bytes, %v", len(data), err)
	}
	_, err := readDatabaseFile(context.Background(), path, 99)
	assertRoute(t, err, "readDatabaseFile.FileTooLarge")
}

func TestOpenCancelledBeforeReading(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := Open(ctx, testenv.Fixture(t, "k4-rich.kdbx"), credsOf(t, "fixture-rich", ""))
	assertRoute(t, err, "readDatabaseFile.ReadCancelled")
}

// countingContext lets Err report "not cancelled" for the first n calls and
// "cancelled" for the rest, so that a test can cancel at every look.
type countingContext struct {
	context.Context
	allowed int64
	calls   atomic.Int64
}

func (c *countingContext) Err() error {
	if c.calls.Add(1) > c.allowed {
		return context.Canceled
	}
	return nil
}

// TestOpenCancelledAtEveryLook cancels at each place that Open looks at its
// context: every cancellation is a cancellation error, none is a success or
// another failure, and the one between the key derivation and the parsing is
// among them.
func TestOpenCancelledAtEveryLook(t *testing.T) {
	file := build4{Password: "built", Payload: goodPayload()}.build(t)
	creds := crypto.Credentials{HasPassword: true, Password: secretOf("built")}

	probe := &countingContext{Context: context.Background(), allowed: 1 << 30}
	if _, _, err := OpenBytes(probe, file, creds); err != nil {
		t.Fatal(err)
	}
	looks := probe.calls.Load()
	if looks < 2 {
		t.Fatalf("Open looked at its context %d times", looks)
	}
	routes := map[string]bool{}
	for allowed := int64(0); allowed < looks; allowed++ {
		ctx := &countingContext{Context: context.Background(), allowed: allowed}
		db, _, err := OpenBytes(ctx, file, creds)
		if err == nil {
			db.Wipe()
			t.Fatalf("allowed %d of %d looks: opened", allowed, looks)
		}
		appErr, ok := errors.FromError(err)
		if !ok || (appErr.Tag != "KdfCancelled" && appErr.Tag != "ReadCancelled") {
			t.Fatalf("allowed %d of %d looks: %v", allowed, looks, err)
		}
		routes[appErr.Tag] = true
	}
	if !routes["KdfCancelled"] || !routes["ReadCancelled"] {
		t.Errorf("cancellations seen: %v", routes)
	}
}

func TestInfoNames(t *testing.T) {
	for id, want := range map[uint32]string{crypto.InnerNone: "none", crypto.InnerArcFour: "ArcFourVariant", crypto.InnerSalsa20: "Salsa20", crypto.InnerChaCha20: "ChaCha20", 9: "unknown"} {
		if got := InnerStreamName(id); got != want {
			t.Errorf("InnerStreamName(%d) = %q, want %q", id, got, want)
		}
	}
	_ = model.UUID{}
}
