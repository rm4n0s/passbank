package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// The merge fixtures are the two databases of modeltest.MergePair as files: A as
// KDBX 4.0 and B as KDBX 3.1, so that every merge test mixes versions. They are
// committed (the plan files of the goldens hold the hashes of their bytes) and
// rewritten, with the goldens, by -update-goldens.
var (
	mergeFixtureA = filepath.Join("testdata", "merge", "a.kdbx")
	mergeFixtureB = filepath.Join("testdata", "merge", "b.kdbx")
)

// fixtureSpec describes how one merge fixture is stored.
type fixtureSpec struct {
	path     string
	password string
	version  kdbx.Version
	db       *model.Database
}

func mergeFixtureSpecs() []fixtureSpec {
	a, b := modeltest.MergePair()
	return []fixtureSpec{
		{mergeFixtureA, modeltest.PairPasswordA, kdbx.Version{Major: 4, Minor: 0}, a},
		{mergeFixtureB, modeltest.PairPasswordB, kdbx.Version{Major: 3, Minor: 1}, b},
	}
}

var regenerate sync.Once

// mergeFixtures returns the absolute paths of the two merge fixtures. With
// -update-goldens it first writes them again from modeltest.MergePair.
func mergeFixtures(t *testing.T) (a, b string) {
	t.Helper()
	if *updateGoldens {
		regenerate.Do(func() {
			for _, s := range mergeFixtureSpecs() {
				settings := kdbx.DefaultSettings(s.version)
				// A cheap key derivation: the fixtures protect nothing.
				settings.KDF = crypto.KDF{ID: crypto.KDFAES, Rounds: 1000}
				creds := crypto.Credentials{HasPassword: true, Password: secret.New([]byte(s.password))}
				data, err := kdbx.Encode(context.Background(), s.db, s.version, settings, creds)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(s.path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	abs := func(p string) string {
		path, err := filepath.Abs(p)
		if err != nil {
			t.Fatal(err)
		}
		return path
	}
	return abs(mergeFixtureA), abs(mergeFixtureB)
}

// passwordFile writes a password file that only its owner can read.
func passwordFile(t *testing.T, dir, name, password string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestMergeFixturesAreThePair checks that the committed files hold what
// modeltest.MergePair makes, so that the pair and the files cannot drift apart.
func TestMergeFixturesAreThePair(t *testing.T) {
	mergeFixtures(t)
	for _, s := range mergeFixtureSpecs() {
		creds := crypto.Credentials{HasPassword: true, Password: secret.New([]byte(s.password))}
		db, info, err := kdbx.Open(context.Background(), s.path, creds)
		if err != nil {
			t.Fatalf("%s: %v", s.path, err)
		}
		if info.Version != s.version {
			t.Errorf("%s is version %v, want %v", s.path, info.Version, s.version)
		}
		if d := model.Diff(db, kdbx.Fit(s.db, s.version)); len(d) > 0 {
			t.Errorf("%s differs from modeltest.MergePair: %v", s.path, d)
		}
	}
}
