package kdbx

import (
	"context"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
)

// randomSettings picks cheap settings by seed.
func randomSettings(seed uint64) EncryptionSettings {
	ciphers := []model.UUID{crypto.CipherAES256, crypto.CipherTwofish, crypto.CipherChaCha20}
	kdfs := []crypto.KDF{cheapKDFs["AES-KDF"], cheapKDFs["Argon2d"], cheapKDFs["Argon2id"]}
	return EncryptionSettings{CipherID: ciphers[seed%3], KDF: kdfs[(seed/3)%3], Compression: Compression((seed / 9) % 2)}
}

// TestRandomDatabasesRoundTripInKDBX4 writes random databases (SPEC §17) and reads
// them back: the model is equal, in the lowest KDBX 4 version that holds it.
func TestRandomDatabasesRoundTripInKDBX4(t *testing.T) {
	for seed := uint64(1); seed <= 60; seed++ {
		o := modeltest.DefaultOptions()
		o.With41 = seed%2 == 0
		db := modeltest.Random(seed, o)
		version := MinVersion(db)
		data, err := Encode(context.Background(), db, version, randomSettings(seed), encodeCreds)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		got, info, err := OpenBytes(context.Background(), data, encodeCreds)
		if err != nil {
			t.Fatalf("seed %d: the file does not read back: %v", seed, err)
		}
		if info.Version != version {
			t.Errorf("seed %d: version %v, want %v", seed, info.Version, version)
		}
		if diff := model.Diff(Fit(db, version), got); len(diff) != 0 {
			t.Errorf("seed %d reads back differently:\n%s", seed, strings.Join(diff[:min(len(diff), 10)], "\n"))
		}
		got.Wipe()
	}
}

// TestRandomDatabasesLoseOnlyThe41FieldsIn40 writes databases with 4.1 fields as
// 4.0: what comes back is the database without those fields.
func TestRandomDatabasesLoseOnlyThe41FieldsIn40(t *testing.T) {
	o := modeltest.DefaultOptions()
	o.With41 = true
	for seed := uint64(1); seed <= 30; seed++ {
		db := modeltest.Random(seed, o)
		data, err := Encode(context.Background(), db, Version{4, 0}, randomSettings(seed), encodeCreds)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		got, _, err := OpenBytes(context.Background(), data, encodeCreds)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if diff := model.Diff(Fit(db, Version{4, 0}), got); len(diff) != 0 {
			t.Errorf("seed %d:\n%s", seed, strings.Join(diff[:min(len(diff), 10)], "\n"))
		}
		got.Wipe()
	}
}
