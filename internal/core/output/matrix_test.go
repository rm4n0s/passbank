package output

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// matrixVersions are the versions Passbank writes.
var matrixVersions = []kdbx.Version{{Major: 4, Minor: 1}, {Major: 4}, {Major: 3, Minor: 1}, {Major: 3}, {Major: 2}}

// matrixSettings are cheap settings the version holds.
func matrixSettings(v kdbx.Version, i int) kdbx.EncryptionSettings {
	if v.Major < 4 {
		return kdbx.EncryptionSettings{CipherID: []model.UUID{crypto.CipherAES256, crypto.CipherTwofish}[i%2],
			KDF: crypto.KDF{ID: crypto.KDFAES, Rounds: 100}, Compression: kdbx.Compression(i / 2 % 2)}
	}
	kdfs := []crypto.KDF{{ID: crypto.KDFAES, Rounds: 100}, {ID: crypto.KDFArgon2d, Memory: 1 << 20, Iterations: 1, Parallelism: 1},
		{ID: crypto.KDFArgon2id, Memory: 1 << 20, Iterations: 1, Parallelism: 2}}
	return kdbx.EncryptionSettings{CipherID: []model.UUID{crypto.CipherAES256, crypto.CipherTwofish, crypto.CipherChaCha20}[i%3],
		KDF: kdfs[i/3%3], Compression: kdbx.Compression(i / 9 % 2)}
}

// checkCell writes the database in the version through Write and checks the cell of
// the matrix (SPEC §17): without AllowLoss the write is refused exactly when
// something is lost; with it, the file reads back as the database fitted to the
// version, and what differs from the original is exactly what Losses reported.
func checkCell(t *testing.T, db *model.Database, v kdbx.Version, i int) {
	t.Helper()
	losses := kdbx.Losses(db, v)
	settings := matrixSettings(v, i)
	path := filepath.Join(t.TempDir(), "out.kdbx")
	err := Write(context.Background(), path, db, v, settings, creds, Options{})
	if len(losses) > 0 {
		assertRoute(t, err, "lossesError.OutputVersionLosesData")
		if len(names(t, filepath.Dir(path))) != 0 {
			t.Errorf("%v: a refused write left %v", v, names(t, filepath.Dir(path)))
		}
	} else if err != nil {
		t.Fatalf("%v: nothing is lost, but the write failed: %v", v, err)
	}
	if err := Write(context.Background(), path, db, v, settings, creds, Options{AllowLoss: true, Overwrite: true}); err != nil {
		t.Fatalf("%v: %v", v, err)
	}
	got, info, err := kdbx.Open(context.Background(), path, creds)
	if err != nil {
		t.Fatalf("%v: %v", v, err)
	}
	defer got.Wipe()
	if info.Version != v {
		t.Errorf("version %v, want %v", info.Version, v)
	}
	if diff := model.Diff(kdbx.Fit(db, v), got); len(diff) != 0 {
		t.Errorf("%v: the file differs from the database as the version holds it:\n%s", v, strings.Join(diff[:min(len(diff), 10)], "\n"))
	}
	// What differs from the original is what Losses reported, and (the pool, the
	// generator) nothing else.
	if len(losses) == 0 {
		for _, line := range model.Diff(db, got) {
			if !strings.HasPrefix(line, "Binaries/") && !strings.HasPrefix(line, "Meta/Generator") {
				t.Errorf("%v: nothing is lost, but %s differs", v, line)
			}
		}
	}
}

func TestMatrixOfFixtures(t *testing.T) {
	for _, f := range []struct{ file, password string }{
		{"k4-empty.kdbx", "fixture-min"}, {"k4-rich.kdbx", "fixture-rich"}, {"k4-rich-argon2id.kdbx", "fixture-rich"},
		{"k4-rich-chacha20.kdbx", "fixture-rich"}, {"k4-rich-twofish-aeskdf.kdbx", "fixture-rich"}, {"k4-rich-nocompression.kdbx", "fixture-rich"},
		{"k4-public-custom-data.kdbx", "fixture-rich"}, {"k4-unknown-xml.kdbx", "fixture-rich"}, {"k41-fields.kdbx", "fixture-rich"},
		{"k31-empty.kdbx", "fixture-min"}, {"k31-rich.kdbx", "fixture-rich"},
		{"ext/rs-password.kdbx", "demopass"}, {"ext/rs-attachments.kdbx", "demopass"}, {"ext/rs-chacha20-inner.kdbx", "password"},
		{"ext/go-example.kdbx", "abcdefg12345678"}, {"ext/go-protected-binary.kdbx", "123"}, {"ext/go-chacha.kdbx", "abcdefg12345678"},
		{"ext/go-twofish.kdbx", "test1234test"}, {"ext/web-demo.kdbx", "demo"}, {"ext/web-cyrillic.kdbx", "пароль"},
	} {
		t.Run(f.file, func(t *testing.T) {
			path, err := filepath.Abs(filepath.Join("..", "kdbx", "testdata", f.file))
			if err != nil {
				t.Fatal(err)
			}
			// ext/web-demo.kdbx also needs its key file.
			c := crypto.Credentials{HasPassword: true, Password: secret.New([]byte(f.password))}
			if f.file == "ext/web-demo.kdbx" {
				c.HasKeyFile, c.KeyFile = true, secret.New(readAll(t, filepath.Join(filepath.Dir(path), "web-demo.key")))
			}
			db, _, err := kdbx.Open(context.Background(), path, c)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Wipe()
			for i, v := range matrixVersions {
				checkCell(t, db, v, i+len(f.file))
			}
		})
	}
}

func TestMatrixOfRandomDatabases(t *testing.T) {
	o := modeltest.DefaultOptions()
	for seed := uint64(1); seed <= 40; seed++ {
		o.With41 = seed%2 == 0
		o.WithItemCustomData = seed%3 != 0
		db := modeltest.Random(seed, o)
		for i, v := range matrixVersions {
			checkCell(t, db, v, int(seed)+i)
		}
	}
}
