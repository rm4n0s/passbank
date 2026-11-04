package kdbx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/testenv"
)

var oldVersions = []Version{{3, 1}, {3, 0}, {2, 0}}

func aesSettings(cipher model.UUID, comp Compression) EncryptionSettings {
	return EncryptionSettings{CipherID: cipher, KDF: crypto.KDF{ID: crypto.KDFAES, Rounds: 100}, Compression: comp}
}

func TestEncode3EveryVersionCipherAndCompression(t *testing.T) {
	db := openFixture(t, "k4-rich.kdbx", "fixture-rich", "")
	for _, v := range oldVersions {
		for _, cipher := range []model.UUID{crypto.CipherAES256, crypto.CipherTwofish} {
			for _, comp := range []Compression{CompressionNone, CompressionGZip} {
				t.Run(v.String()+"/"+cipher.String()[:4]+"/"+comp.String(), func(t *testing.T) {
					data, err := Encode(context.Background(), db, v, aesSettings(cipher, comp), encodeCreds)
					if err != nil {
						t.Fatal(err)
					}
					got, info, err := OpenBytes(context.Background(), data, encodeCreds)
					if err != nil {
						t.Fatal(err)
					}
					defer got.Wipe()
					if diff := model.Diff(Fit(db, v), got); len(diff) != 0 {
						t.Errorf("the file reads back differently:\n%s", strings.Join(diff, "\n"))
					}
					if info.Version != v || info.CipherID != cipher || info.Compression != comp || info.KDF.ID != crypto.KDFAES || info.KDF.Rounds != 100 ||
						info.InnerStreamID != crypto.InnerSalsa20 {
						t.Errorf("info = %+v", info)
					}
					if _, _, err := OpenBytes(context.Background(), data, credsOf(t, "wrong", "")); err == nil {
						t.Error("a wrong password opened the file")
					}
				})
			}
		}
	}
}

func TestEncode3EveryFixture(t *testing.T) {
	var all []struct{ file, password, keyFile string }
	all = append(all, kdbx4Fixtures...)
	for _, f := range kdbx3Fixtures {
		all = append(all, struct{ file, password, keyFile string }{f.file, f.password, f.keyFile})
	}
	for _, f := range all {
		t.Run(f.file, func(t *testing.T) {
			db := openFixture(t, f.file, f.password, f.keyFile)
			for _, v := range oldVersions {
				data, err := Encode(context.Background(), db, v, aesSettings(crypto.CipherAES256, CompressionGZip), encodeCreds)
				if err != nil {
					t.Fatalf("%v: %v", v, err)
				}
				got, _, err := OpenBytes(context.Background(), data, encodeCreds)
				if err != nil {
					t.Fatalf("%v: %v", v, err)
				}
				if diff := model.Diff(Fit(db, v), got); len(diff) != 0 {
					t.Errorf("%v reads back differently:\n%s", v, strings.Join(diff, "\n"))
				}
				got.Wipe()
			}
		})
	}
}

func TestEncode3RandomDatabases(t *testing.T) {
	o := modeltest.DefaultOptions()
	o.With41 = true
	for seed := uint64(1); seed <= 40; seed++ {
		db := modeltest.Random(seed, o)
		for _, v := range oldVersions {
			cipher := []model.UUID{crypto.CipherAES256, crypto.CipherTwofish}[seed%2]
			data, err := Encode(context.Background(), db, v, aesSettings(cipher, Compression(seed/2%2)), encodeCreds)
			if err != nil {
				t.Fatalf("seed %d, %v: %v", seed, v, err)
			}
			got, _, err := OpenBytes(context.Background(), data, encodeCreds)
			if err != nil {
				t.Fatalf("seed %d, %v: the file does not read back: %v", seed, v, err)
			}
			if diff := model.Diff(Fit(db, v), got); len(diff) != 0 {
				t.Errorf("seed %d, %v:\n%s", seed, v, strings.Join(diff[:min(len(diff), 10)], "\n"))
			}
			got.Wipe()
		}
	}
}

func TestEncode3IsRandomAndHashesItsHeader(t *testing.T) {
	db := openFixture(t, "k4-rich.kdbx", "fixture-rich", "")
	var files [][]byte
	var containers []*container
	for i := 0; i < 2; i++ {
		data, err := Encode(context.Background(), db, Version{3, 1}, aesSettings(crypto.CipherAES256, CompressionGZip), encodeCreds)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, data)
		c, err := readContainer3(context.Background(), data, fixtureCredsOf(t, encodeCreds))
		if err != nil {
			t.Fatal(err)
		}
		containers = append(containers, c)
	}
	if bytes.Equal(files[0], files[1]) {
		t.Fatal("two writes gave the same bytes")
	}
	a, b := containers[0], containers[1]
	if bytes.Equal(a.Header.MasterSeed, b.Header.MasterSeed) || bytes.Equal(a.Header.IV, b.Header.IV) || bytes.Equal(a.Header.KDF.Seed, b.Header.KDF.Seed) ||
		bytes.Equal(a.InnerStreamKey.Reveal(), b.InnerStreamKey.Reveal()) {
		t.Error("a seed, the IV or the inner stream key was reused")
	}
	if len(a.Header.MasterSeed) != 32 || len(a.Header.KDF.Seed) != 32 || a.InnerStreamKey.Len() != 32 {
		t.Errorf("lengths: seed %d, KDF seed %d, inner key %d", len(a.Header.MasterSeed), len(a.Header.KDF.Seed), a.InnerStreamKey.Len())
	}
	// The XML repeats the hash of the header.
	m := headerHashPattern.FindSubmatch(a.XML)
	if m == nil || string(m[1]) != base64.StdEncoding.EncodeToString(a.HeaderHash) {
		t.Errorf("HeaderHash = %s, header hashes to %s", m, base64.StdEncoding.EncodeToString(a.HeaderHash))
	}
	sum := sha256.Sum256(files[0][:headerLength(t, files[0])])
	if !bytes.Equal(sum[:], a.HeaderHash) {
		t.Error("HeaderHash is not the hash of the header in the file")
	}
}

func TestEncode3Errors(t *testing.T) {
	db := openFixture(t, "k4-empty.kdbx", "fixture-min", "")
	good := aesSettings(crypto.CipherAES256, CompressionGZip)
	argon := EncryptionSettings{CipherID: crypto.CipherAES256, KDF: cheapKDFs["Argon2id"]}
	tests := []struct {
		name     string
		db       *model.Database
		version  Version
		settings EncryptionSettings
		creds    crypto.Credentials
		route    string
	}{
		{"version 3.2", db, Version{3, 2}, good, encodeCreds, "encode.VersionUnsupported"},
		{"version 2.1", db, Version{2, 1}, good, encodeCreds, "encode.VersionUnsupported"},
		{"version 1.0", db, Version{1, 0}, good, encodeCreds, "encode.VersionUnsupported"},
		{"a nil database", nil, Version{3, 1}, good, encodeCreds, "encode3.DatabaseInvalid"},
		{"Argon2", db, Version{3, 1}, argon, encodeCreds, "Supports.EncryptionNotInVersion"},
		{"ChaCha20", db, Version{3, 1}, aesSettings(crypto.CipherChaCha20, CompressionGZip), encodeCreds, "Supports.EncryptionNotInVersion"},
		{"an unknown compression", db, Version{3, 1}, aesSettings(crypto.CipherAES256, 5), encodeCreds, "encode3.CompressionUnsupported"},
		{"zero rounds", db, Version{3, 1}, EncryptionSettings{CipherID: crypto.CipherAES256, KDF: crypto.KDF{ID: crypto.KDFAES}}, encodeCreds, "KDF.Validate.KdfParametersRejected"},
		{"no credentials", db, Version{3, 1}, good, crypto.Credentials{}, "CompositeKey.NoCredentials"},
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
		_, err := Encode(ctx, db, Version{3, 1}, good, encodeCreds)
		assertRoute(t, err, "KdfCancelled")
	})
	t.Run("an attachment that the pool lacks", func(t *testing.T) {
		bad := &model.Database{Binaries: map[model.BinaryKey]*model.Binary{}, Root: &model.Group{UUID: uuid(1), Entries: []*model.Entry{
			{UUID: uuid(2), Attachments: []model.Attachment{{Name: "x", Binary: model.BinaryKey{Hash: [32]byte{1}}}}}}}}
		_, err := Encode(context.Background(), bad, Version{3, 1}, good, encodeCreds)
		assertRoute(t, err, "collectBinaries.func1.DatabaseInvalid")
	})
	// Master seed 32, IV 16, KDF seed 32, inner key 32, start bytes 32: fail at each draw.
	for _, n := range []int{0, 32, 48, 80, 112} {
		_, err := encode(context.Background(), db, Version{3, 1}, good, encodeCreds, &failAfter{n: n})
		assertRoute(t, err, "randomBytes.RandomFailed")
	}
	if _, err := encode(context.Background(), db, Version{3, 1}, good, encodeCreds, &failAfter{n: 144}); err != nil {
		t.Errorf("exactly enough random bytes failed: %v", err)
	}
}

func TestEncode3MatchesTheFixturesOfOtherPrograms(t *testing.T) {
	// A KDBX 3.1 file that KeePass wrote, read and written again by Passbank, keeps
	// every entry byte for byte (the same dump pykeepass made of the original).
	for _, name := range []string{"ext/rs-password.kdbx", "ext/go-example.kdbx", "ext/web-demo.kdbx", "ext/rs-attachments.kdbx"} {
		t.Run(name, func(t *testing.T) {
			var f struct{ password, keyFile string }
			for _, x := range kdbx3Fixtures {
				if x.file == name {
					f.password, f.keyFile = x.password, x.keyFile
				}
			}
			db, _, err := Open(context.Background(), testenv.Fixture(t, name), credsOf(t, f.password, f.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Wipe()
			data, err := Encode(context.Background(), db, Version{3, 1}, aesSettings(crypto.CipherAES256, CompressionGZip), encodeCreds)
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := OpenBytes(context.Background(), data, encodeCreds)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Wipe()
			if a, b := dumpEntries(db), dumpEntries(got); a != b {
				t.Errorf("the entries changed:\n%s\n%s", a, b)
			}
		})
	}
}
