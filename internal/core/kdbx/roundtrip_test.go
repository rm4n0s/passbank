package kdbx

import (
	"context"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/testenv"
)

// kdbx4Fixtures are the KDBX 4 files with the credentials of FIXTURES.md.
var kdbx4Fixtures = []struct{ file, password, keyFile string }{
	{"k4-empty.kdbx", "fixture-min", ""},
	{"k4-rich.kdbx", "fixture-rich", ""},
	{"k4-rich-argon2id.kdbx", "fixture-rich", ""},
	{"k4-rich-chacha20.kdbx", "fixture-rich", ""},
	{"k4-rich-twofish-aeskdf.kdbx", "fixture-rich", ""},
	{"k4-rich-nocompression.kdbx", "fixture-rich", ""},
	{"k4-keyfile-xml2.kdbx", "", "keyfile-xml2.keyx"},
	{"k4-password-and-keyfile.kdbx", "fixture-rich", "keyfile-raw32.key"},
	{"k4-public-custom-data.kdbx", "fixture-rich", ""},
	{"k4-unknown-xml.kdbx", "fixture-rich", ""},
	{"k41-fields.kdbx", "fixture-rich", ""},
}

// TestEveryFixtureRoundTripsAsKDBX4 reads every fixture of every version, writes
// it as KDBX 4 (4.0, or 4.1 where it needs it) with the settings of the file it
// came from and with the defaults' ciphers, reads it back, and requires an equal
// model (SPEC §17).
func TestEveryFixtureRoundTripsAsKDBX4(t *testing.T) {
	type fixture struct{ file, password, keyFile string }
	var all []fixture
	for _, f := range kdbx4Fixtures {
		all = append(all, fixture(f))
	}
	for _, f := range kdbx3Fixtures {
		all = append(all, fixture{f.file, f.password, f.keyFile})
	}
	for _, f := range all {
		t.Run(f.file, func(t *testing.T) {
			db, info, err := Open(context.Background(), testenv.Fixture(t, f.file), credsOf(t, f.password, f.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Wipe()
			version := MinVersion(db)
			// The settings of the input where KDBX 4 can use them (they are cheap in the
			// fixtures); otherwise the cheap Argon2.
			settings := SettingsOf(info)
			if info.Version.Major < 4 {
				settings.KDF = cheapKDFs["Argon2id"]
			}
			creds := credsOf(t, f.password, f.keyFile)
			data, err := Encode(context.Background(), db, version, settings, creds)
			if err != nil {
				t.Fatal(err)
			}
			got, got4, err := OpenBytes(context.Background(), data, creds)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Wipe()
			if got4.Version != version {
				t.Errorf("version %v, want %v", got4.Version, version)
			}
			if diff := model.Diff(Fit(db, version), got); len(diff) != 0 {
				t.Errorf("the model changed:\n%s", strings.Join(diff, "\n"))
			}
		})
	}
}
