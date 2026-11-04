package kdbx

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/testenv"
)

// cheapKDFs are KDF settings that are fast enough for many writes.
var cheapKDFs = map[string]crypto.KDF{
	"AES-KDF":  {ID: crypto.KDFAES, Rounds: 50},
	"Argon2d":  {ID: crypto.KDFArgon2d, Memory: 1 << 20, Iterations: 1, Parallelism: 1},
	"Argon2id": {ID: crypto.KDFArgon2id, Memory: 1 << 20, Iterations: 2, Parallelism: 2},
}

func openFixture(t *testing.T, file, password, keyFile string) *model.Database {
	t.Helper()
	db, _, err := Open(context.Background(), testenv.Fixture(t, file), credsOf(t, password, keyFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Wipe)
	return db
}

var encodeCreds = crypto.Credentials{HasPassword: true, Password: secret.New([]byte("a new password"))}

func TestEncodeEveryCipherAndKDF(t *testing.T) {
	db := openFixture(t, "k4-rich.kdbx", "fixture-rich", "")
	for _, cipher := range []model.UUID{crypto.CipherAES256, crypto.CipherTwofish, crypto.CipherChaCha20} {
		for name, kdf := range cheapKDFs {
			for _, comp := range []Compression{CompressionNone, CompressionGZip} {
				t.Run(cipher.String()[:4]+"/"+name+"/"+comp.String(), func(t *testing.T) {
					data, err := Encode(context.Background(), db, Version{4, 0}, EncryptionSettings{CipherID: cipher, KDF: kdf, Compression: comp}, encodeCreds)
					if err != nil {
						t.Fatal(err)
					}
					got, info, err := OpenBytes(context.Background(), data, encodeCreds)
					if err != nil {
						t.Fatal(err)
					}
					defer got.Wipe()
					if diff := model.Diff(db, got); len(diff) != 0 {
						t.Errorf("the file reads back differently:\n%s", strings.Join(diff, "\n"))
					}
					if info.Version != (Version{4, 0}) || info.CipherID != cipher || info.Compression != comp || info.KDF.ID != kdf.ID ||
						info.InnerStreamID != crypto.InnerChaCha20 {
						t.Errorf("info = %+v", info)
					}
					if kdf.ID == crypto.KDFAES && info.KDF.Rounds != kdf.Rounds {
						t.Errorf("rounds = %d", info.KDF.Rounds)
					}
					if kdf.ID != crypto.KDFAES && (info.KDF.Memory != kdf.Memory || info.KDF.Iterations != kdf.Iterations || info.KDF.Parallelism != kdf.Parallelism || info.KDF.Version != 0x13) {
						t.Errorf("KDF = %+v", info.KDF)
					}
					if _, _, err := OpenBytes(context.Background(), data, credsOf(t, "wrong", "")); err == nil {
						t.Error("a wrong password opened the file")
					}
				})
			}
		}
	}
}

func TestEncodeEveryFixtureAndVersion(t *testing.T) {
	for _, tt := range []struct{ file, password string }{
		{"k4-empty.kdbx", "fixture-min"}, {"k4-rich.kdbx", "fixture-rich"}, {"k4-public-custom-data.kdbx", "fixture-rich"},
		{"k4-unknown-xml.kdbx", "fixture-rich"}, {"k41-fields.kdbx", "fixture-rich"}, {"k31-rich.kdbx", "fixture-rich"},
		{"ext/rs-attachments.kdbx", "demopass"}, {"ext/go-protected-binary.kdbx", "123"}, {"ext/web-cyrillic.kdbx", "пароль"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			db := openFixture(t, tt.file, tt.password, "")
			version := MinVersion(db)
			data, err := Encode(context.Background(), db, version, EncryptionSettings{CipherID: crypto.CipherChaCha20, KDF: cheapKDFs["Argon2id"], Compression: CompressionGZip}, encodeCreds)
			if err != nil {
				t.Fatal(err)
			}
			got, info, err := OpenBytes(context.Background(), data, encodeCreds)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Wipe()
			if info.Version != version {
				t.Errorf("version = %v, want %v", info.Version, version)
			}
			if diff := model.Diff(db, got); len(diff) != 0 {
				t.Errorf("the file reads back differently:\n%s", strings.Join(diff, "\n"))
			}
		})
	}
}

func TestEncodeVersionsDecideTheFields(t *testing.T) {
	db := openFixture(t, "k41-fields.kdbx", "fixture-rich", "")
	if MinVersion(db) != (Version{4, 1}) {
		t.Fatal("the fixture should need 4.1")
	}
	settings := EncryptionSettings{CipherID: crypto.CipherAES256, KDF: cheapKDFs["Argon2d"], Compression: CompressionGZip}
	for version, want := range map[Version]bool{{4, 0}: false, {4, 1}: true} {
		data, err := Encode(context.Background(), db, version, settings, encodeCreds)
		if err != nil {
			t.Fatal(err)
		}
		got, info, err := OpenBytes(context.Background(), data, encodeCreds)
		if err != nil {
			t.Fatal(err)
		}
		if info.Version != version {
			t.Errorf("version = %v, want %v", info.Version, version)
		}
		if has := len(got.Root.Tags) > 0; has != want {
			t.Errorf("%v: root tags present = %v, want %v", version, has, want)
		}
		if has := MinVersion(got) == (Version{4, 1}); has != want {
			t.Errorf("%v: 4.1 data present = %v, want %v", version, has, want)
		}
		got.Wipe()
	}
}

func TestEncodeIsRandom(t *testing.T) {
	db := openFixture(t, "k4-rich.kdbx", "fixture-rich", "")
	settings := EncryptionSettings{CipherID: crypto.CipherAES256, KDF: cheapKDFs["Argon2id"], Compression: CompressionGZip}
	var files [][]byte
	var containers []*container
	for i := 0; i < 2; i++ {
		data, err := Encode(context.Background(), db, Version{4, 0}, settings, encodeCreds)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, data)
		composite := fixtureCredsOf(t, encodeCreds)
		c, err := readContainer4(context.Background(), data, composite)
		if err != nil {
			t.Fatal(err)
		}
		containers = append(containers, c)
	}
	if bytes.Equal(files[0], files[1]) {
		t.Fatal("two writes gave the same bytes")
	}
	a, b := containers[0], containers[1]
	if bytes.Equal(a.Header.MasterSeed, b.Header.MasterSeed) || bytes.Equal(a.Header.IV, b.Header.IV) ||
		bytes.Equal(a.Header.KDF.Seed, b.Header.KDF.Seed) || bytes.Equal(a.InnerStreamKey.Reveal(), b.InnerStreamKey.Reveal()) {
		t.Error("a seed, the IV or the inner stream key was reused")
	}
	if len(a.Header.MasterSeed) != 32 || len(a.Header.KDF.Seed) != 32 || a.InnerStreamKey.Len() != 64 {
		t.Errorf("lengths: seed %d, KDF seed %d, inner key %d", len(a.Header.MasterSeed), len(a.Header.KDF.Seed), a.InnerStreamKey.Len())
	}
	if bytes.Equal(a.XML, b.XML) {
		t.Error("the protected values were encrypted with the same stream twice")
	}
	// Equal models, though.
	da, _, _ := OpenBytes(context.Background(), files[0], encodeCreds)
	db2, _, _ := OpenBytes(context.Background(), files[1], encodeCreds)
	if diff := model.Diff(da, db2); len(diff) != 0 {
		t.Errorf("models differ: %v", diff)
	}
}

func fixtureCredsOf(t *testing.T, c crypto.Credentials) secret.Bytes {
	t.Helper()
	key, err := crypto.CompositeKey(c)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncodeLargeAttachmentSpansBlocks(t *testing.T) {
	content := make([]byte, 3*blockSize4+12345)
	if _, err := io.ReadFull(rand.Reader, content); err != nil { // incompressible, so the ciphertext is as large
		t.Fatal(err)
	}
	key, bin := bigBinary(content)
	db := &model.Database{
		Meta:     model.Meta{Generator: "test"},
		Binaries: map[model.BinaryKey]*model.Binary{key: bin},
		Root:     &model.Group{UUID: uuid(1), Entries: []*model.Entry{{UUID: uuid(2), Attachments: []model.Attachment{{Name: "big", Binary: key}}}}},
	}
	settings := EncryptionSettings{CipherID: crypto.CipherTwofish, KDF: cheapKDFs["AES-KDF"], Compression: CompressionGZip}
	data, err := Encode(context.Background(), db, Version{4, 0}, settings, encodeCreds)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 3*blockSize4 {
		t.Fatalf("file has %d bytes", len(data))
	}
	got, _, err := OpenBytes(context.Background(), data, encodeCreds)
	if err != nil {
		t.Fatal(err)
	}
	if diff := model.Diff(db, got); len(diff) != 0 {
		t.Errorf("reads back differently: %v", diff)
	}
}

func TestEncodeCredentials(t *testing.T) {
	db := openFixture(t, "k4-empty.kdbx", "fixture-min", "")
	settings := EncryptionSettings{CipherID: crypto.CipherAES256, KDF: cheapKDFs["Argon2id"], Compression: CompressionGZip}
	for name, creds := range map[string]crypto.Credentials{
		"key file only":     credsOf(t, "", "keyfile-xml2.keyx"),
		"password and file": credsOf(t, "pw", "keyfile-raw32.key"),
		"unicode password":  {HasPassword: true, Password: secretOf("пароль 鍵 ü")},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := Encode(context.Background(), db, Version{4, 0}, settings, creds)
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := OpenBytes(context.Background(), data, creds)
			if err != nil {
				t.Fatal(err)
			}
			got.Wipe()
		})
	}
	// The caller's credentials are not wiped by Encode.
	creds := credsOf(t, "kept", "")
	if _, err := Encode(context.Background(), db, Version{4, 0}, settings, creds); err != nil {
		t.Fatal(err)
	}
	if string(creds.Password.Reveal()) != "kept" {
		t.Error("Encode wiped the credentials")
	}
}

func TestDefaultSettings(t *testing.T) {
	v4 := DefaultSettings(Version{4, 0})
	if v4.CipherID != crypto.CipherAES256 || v4.Compression != CompressionGZip || v4.KDF.ID != crypto.KDFArgon2id ||
		v4.KDF.Memory != 64<<20 || v4.KDF.Iterations != 10 || v4.KDF.Parallelism != 2 || v4.KDF.Version != 0x13 {
		t.Errorf("KDBX 4 defaults = %+v", v4)
	}
	for _, v := range []Version{{3, 1}, {3, 0}, {2, 0}} {
		s := DefaultSettings(v)
		if s.CipherID != crypto.CipherAES256 || s.Compression != CompressionGZip || s.KDF.ID != crypto.KDFAES || s.KDF.Rounds != 2_000_000 {
			t.Errorf("%v defaults = %+v", v, s)
		}
	}
	if err := func() error {
		s := DefaultSettings(Version{4, 1})
		s.KDF.Seed = make([]byte, 32)
		return s.KDF.Validate()
	}(); err != nil {
		t.Errorf("the default KDF is invalid: %v", err)
	}
	// The defaults write a file that reads.
	db := openFixture(t, "k4-empty.kdbx", "fixture-min", "")
	data, err := Encode(context.Background(), db, Version{4, 1}, DefaultSettings(Version{4, 1}), encodeCreds)
	if err != nil {
		t.Fatal(err)
	}
	got, info, err := OpenBytes(context.Background(), data, encodeCreds)
	if err != nil {
		t.Fatal(err)
	}
	got.Wipe()
	if info.KDF.Memory != 64<<20 || info.KDF.Iterations != 10 {
		t.Errorf("KDF = %+v", info.KDF)
	}
}

func TestSettingsOf(t *testing.T) {
	_, info, err := Open(context.Background(), testenv.Fixture(t, "k4-rich-twofish-aeskdf.kdbx"), credsOf(t, "fixture-rich", ""))
	if err != nil {
		t.Fatal(err)
	}
	s := SettingsOf(info)
	if s.CipherID != crypto.CipherTwofish || s.KDF.ID != crypto.KDFAES || s.KDF.Rounds != 6000 || s.KDF.Seed != nil || s.Compression != CompressionGZip {
		t.Errorf("settings = %+v", s)
	}
	db := openFixture(t, "k4-rich-twofish-aeskdf.kdbx", "fixture-rich", "")
	data, err := Encode(context.Background(), db, Version{4, 0}, s, encodeCreds)
	if err != nil {
		t.Fatal(err)
	}
	if _, info2, err := OpenBytes(context.Background(), data, encodeCreds); err != nil || info2.Cipher != "Twofish" || info2.KDF.Rounds != 6000 {
		t.Errorf("the settings were not kept: %+v, %v", info2, err)
	}
}

func TestEncodeErrors(t *testing.T) {
	db := openFixture(t, "k4-empty.kdbx", "fixture-min", "")
	good := EncryptionSettings{CipherID: crypto.CipherAES256, KDF: cheapKDFs["AES-KDF"], Compression: CompressionGZip}
	with := func(f func(s *EncryptionSettings)) EncryptionSettings { s := good; f(&s); return s }
	tests := []struct {
		name     string
		db       *model.Database
		version  Version
		settings EncryptionSettings
		creds    crypto.Credentials
		route    string
	}{
		{"version 5", db, Version{5, 0}, good, encodeCreds, "encode.VersionUnsupported"},
		{"version 4.2", db, Version{4, 2}, good, encodeCreds, "encode.VersionUnsupported"},
		{"a nil database", nil, Version{4, 0}, good, encodeCreds, "encode.DatabaseInvalid"},
		{"no root group", &model.Database{}, Version{4, 0}, good, encodeCreds, "encode.DatabaseInvalid"},
		{"an unknown cipher", db, Version{4, 0}, with(func(s *EncryptionSettings) { s.CipherID = model.UUID{1} }), encodeCreds, "LookupCipher.CipherUnsupported"},
		{"an unknown compression", db, Version{4, 0}, with(func(s *EncryptionSettings) { s.Compression = 7 }), encodeCreds, "encode.CompressionUnsupported"},
		{"an unknown KDF", db, Version{4, 0}, with(func(s *EncryptionSettings) { s.KDF = crypto.KDF{ID: model.UUID{2}} }), encodeCreds, "KDF.Validate.KdfUnsupported"},
		{"Argon2 without memory", db, Version{4, 0}, with(func(s *EncryptionSettings) { s.KDF = crypto.KDF{ID: crypto.KDFArgon2id, Iterations: 1, Parallelism: 1} }), encodeCreds, "KDF.Validate.KdfParametersRejected"},
		{"AES-KDF without rounds", db, Version{4, 0}, with(func(s *EncryptionSettings) { s.KDF = crypto.KDF{ID: crypto.KDFAES} }), encodeCreds, "KDF.Validate.KdfParametersRejected"},
		{"no credentials", db, Version{4, 0}, good, crypto.Credentials{}, "CompositeKey.NoCredentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Encode(context.Background(), tt.db, tt.version, tt.settings, tt.creds)
			assertRoute(t, err, tt.route)
		})
	}
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Encode(ctx, db, Version{4, 0}, good, encodeCreds)
		assertRoute(t, err, "KdfCancelled")
	})
	t.Run("an attachment that the pool lacks", func(t *testing.T) {
		bad := &model.Database{Binaries: map[model.BinaryKey]*model.Binary{}, Root: &model.Group{UUID: uuid(1), Entries: []*model.Entry{
			{UUID: uuid(2), Attachments: []model.Attachment{{Name: "x", Binary: model.BinaryKey{Hash: [32]byte{1}}}}}}}}
		_, err := Encode(context.Background(), bad, Version{4, 0}, good, encodeCreds)
		assertRoute(t, err, "collectBinaries.func1.DatabaseInvalid")
	})
}

// failAfter is a reader that gives n bytes and then fails.
type failAfter struct{ n int }

func (f *failAfter) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := min(len(p), f.n)
	f.n -= n
	return n, nil
}

func TestEncodeRandomFailure(t *testing.T) {
	db := openFixture(t, "k4-empty.kdbx", "fixture-min", "")
	good := EncryptionSettings{CipherID: crypto.CipherAES256, KDF: cheapKDFs["AES-KDF"], Compression: CompressionGZip}
	// Master seed 32, IV 16, KDF seed 32, inner key 64: fail at each.
	for _, n := range []int{0, 32, 48, 80} {
		_, err := encode(context.Background(), db, Version{4, 0}, good, encodeCreds, &failAfter{n: n})
		assertRoute(t, err, "randomBytes.RandomFailed")
	}
	if _, err := encode(context.Background(), db, Version{4, 0}, good, encodeCreds, &failAfter{n: 144}); err != nil {
		t.Errorf("exactly enough random bytes failed: %v", err)
	}
}

// bigBinary makes a pool entry for a content.
func bigBinary(content []byte) (model.BinaryKey, *model.Binary) {
	return model.BinaryKey{Hash: sha256Sum(content)}, &model.Binary{Data: secret.New(content)}
}

func sha256Sum(b []byte) [32]byte { return sha256.Sum256(b) }
