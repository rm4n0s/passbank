package crypto

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/secret"
)

// The expected keys were computed with Python's hashlib, not with this code:
// SHA-256 of the SHA-256 of the password, and so on (SPEC §4.2).
const (
	wantPasswordOnly   = "4cfe2734587078f0c7ba2f9b473c1c9be68e775d6fa1517fa919a15b4b781130" // sha256(sha256("fixture-pass"))
	wantRawKeyFileOnly = "630dcd2966c4336691125448bbb25b4ff412a49c732db2c8abc1b8581bd710dd" // sha256(bytes 0..31)
	wantHashedKeyFile  = "294f27d43550c7b0b35175551ddc83143106a269c7a70cbcf8217be49d8261bd" // sha256(sha256("hello key file\n"))
	wantPasswordAndRaw = "00ab8960db065138ef591dd72be8ca5d7dbedbb763f167dc166cf0f9164f3e8b" // sha256(sha256(pw) + bytes 0..31)
	wantPasswordHashed = "36e1381c58370eefbe6303a864c4221c8f0c93973e76bfcf2635949cebba5097" // sha256(sha256(pw) + sha256("hello key file\n"))
	wantEmptyPassword  = "5df6e0e2761359d30a8275058e299fcc0381534545f55cf43e41983f5d4c9456" // sha256(sha256(""))
)

// keyBytes are the 32 bytes 00 01 02 … 1f that most key files below stand for.
var keyBytes = sequence(0, 32)

func xmlV1(data string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<KeyFile>
	<Meta>
		<Version>1.00</Version>
	</Meta>
	<Key>
		<Data>` + data + `</Data>
	</Key>
</KeyFile>`
}

func xmlV2(hash, data string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<KeyFile>
	<Meta>
		<Version>2.0</Version>
	</Meta>
	<Key>
		<Data Hash="` + hash + `">
			` + data + `
		</Data>
	</Key>
</KeyFile>`
}

func creds(password *string, keyFile []byte) Credentials {
	var c Credentials
	if password != nil {
		c.HasPassword, c.Password = true, secret.New([]byte(*password))
	}
	if keyFile != nil {
		c.HasKeyFile, c.KeyFile = true, secret.New(append([]byte(nil), keyFile...))
	}
	return c
}

func str(s string) *string { return &s }

func TestCompositeKey(t *testing.T) {
	keyHex := hex.EncodeToString(keyBytes)
	tests := []struct {
		name     string
		password *string
		keyFile  []byte
		want     string
	}{
		{"password only", str("fixture-pass"), nil, wantPasswordOnly},
		{"empty password is a component", str(""), nil, wantEmptyPassword},
		{"raw 32-byte key file", nil, keyBytes, wantRawKeyFileOnly},
		{"64 hex digits", nil, []byte(keyHex), wantRawKeyFileOnly},
		{"64 hex digits in upper case", nil, []byte(strings.ToUpper(keyHex)), wantRawKeyFileOnly},
		{"any other file is hashed", nil, []byte("hello key file\n"), wantHashedKeyFile},
		{"64 bytes that are not all hex are hashed", nil, []byte(strings.Repeat("g", 64)), ""},
		{"XML 1.0", nil, []byte(xmlV1("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")), wantRawKeyFileOnly},
		{"XML 1.0 with spaces around the data", nil, []byte(xmlV1("\n  AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=\n ")), wantRawKeyFileOnly},
		{"XML 2.0", nil, []byte(xmlV2("630DCD29", "00010203 04050607 08090A0B 0C0D0E0F\n 10111213 14151617 18191A1B 1C1D1E1F")), wantRawKeyFileOnly},
		{"XML 2.0 with a lower-case hash", nil, []byte(xmlV2("630dcd29", keyHex)), wantRawKeyFileOnly},
		{"password and a raw key file", str("fixture-pass"), keyBytes, wantPasswordAndRaw},
		{"password and a hashed key file", str("fixture-pass"), []byte("hello key file\n"), wantPasswordHashed},
		{"password and an XML 2.0 key file", str("fixture-pass"), []byte(xmlV2("630DCD29", keyHex)), wantPasswordAndRaw},
		{"an empty key file is hashed", nil, []byte{}, wantEmptyPassword}, // sha256("") hashed again is the same as an empty password
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompositeKey(creds(tt.password, tt.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			if got.Len() != 32 {
				t.Fatalf("length = %d, want 32", got.Len())
			}
			if tt.want != "" && hex.EncodeToString(got.Reveal()) != tt.want {
				t.Errorf("key = %x, want %s", got.Reveal(), tt.want)
			}
		})
	}
	// A file that is XML but has another root is read as a binary file.
	other := []byte(`<?xml version="1.0"?><Other>KeyFile</Other>`)
	a, err := CompositeKey(creds(nil, other))
	if err != nil {
		t.Fatal(err)
	}
	b, err := CompositeKey(creds(nil, append([]byte(nil), other...)))
	if err != nil || !a.Equal(b) {
		t.Errorf("another XML root must be hashed like any file (%v)", err)
	}
	// As in KeePass, a password and a key file that is hashed (not 32 bytes, not
	// hexadecimal, not XML) of the same text give one key: both components are the
	// SHA-256 of the text. The order of two components matters.
	p, _ := CompositeKey(creds(str("x"), nil))
	k, _ := CompositeKey(creds(nil, []byte("x")))
	if !p.Equal(k) {
		t.Error("a password and a hashed key file of the same text give one key in KeePass")
	}
	pk, _ := CompositeKey(creds(str("x"), []byte("y")))
	kp, _ := CompositeKey(creds(str("y"), []byte("x")))
	if pk.Equal(kp) {
		t.Error("swapping the password and the key file text must change the key")
	}
}

func TestCompositeKeyErrors(t *testing.T) {
	_, err := CompositeKey(Credentials{})
	assertRoute(t, err, "CompositeKey.NoCredentials")
	// A password that is present but not marked is no password.
	_, err = CompositeKey(Credentials{Password: secret.New([]byte("ignored"))})
	assertRoute(t, err, "CompositeKey.NoCredentials")

	keyHex := hex.EncodeToString(keyBytes)
	bad := map[string]string{
		"1.0 with bad base64":          xmlV1("***not base64***"),
		"1.0 with an empty key":        xmlV1("   "),
		"2.0 with the wrong hash":      xmlV2("DEADBEEF", keyHex),
		"2.0 with no hash":             xmlV2("", keyHex),
		"2.0 with a short hash":        xmlV2("630D", keyHex),
		"2.0 with a hash that is text": xmlV2("zzzzzzzz", keyHex),
		"2.0 with a short key":         xmlV2("630DCD29", keyHex[:62]),
		"2.0 with a key that is text":  xmlV2("630DCD29", strings.Repeat("zz", 32)),
		"unknown version":              strings.Replace(xmlV1("AAAA"), "1.00", "3.0", 1),
		"no version":                   strings.Replace(xmlV1("AAAA"), "<Version>1.00</Version>", "", 1),
		"truncated after the root":     `<?xml version="1.0"?><KeyFile><Meta><Version>2.0</Version></Meta><Key><Data Hash="630DCD29">` + keyHex,
	}
	for name, doc := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := CompositeKey(creds(str("pw"), []byte(doc)))
			assertRoute(t, err, "KeyFileCorrupt")
		})
	}
}

func TestCredentialsWipe(t *testing.T) {
	c := creds(str("pw"), keyBytes)
	c.Wipe()
	if c.Password.Len() != 0 || c.KeyFile.Len() != 0 {
		t.Error("Wipe left a secret behind")
	}
}

func TestReadKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key.bin")
	if err := os.WriteFile(path, keyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadKeyFile(context.Background(), path)
	if err != nil || hex.EncodeToString(got.Reveal()) != hex.EncodeToString(keyBytes) {
		t.Fatalf("ReadKeyFile = %x, %v", got.Reveal(), err)
	}

	t.Run("missing", func(t *testing.T) {
		_, err := ReadKeyFile(context.Background(), filepath.Join(dir, "nope"))
		assertRoute(t, err, "ReadKeyFile.KeyFileReadFailed")
	})
	t.Run("a folder", func(t *testing.T) {
		_, err := ReadKeyFile(context.Background(), dir)
		assertRoute(t, err, "ReadKeyFile.KeyFileReadFailed")
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := ReadKeyFile(ctx, path)
		assertRoute(t, err, "ReadKeyFile.KeyFileReadCancelled")
	})
	t.Run("exactly the limit and one byte over", func(t *testing.T) {
		big := filepath.Join(dir, "big")
		f, err := os.Create(big)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()                   // a sparse test file
		if err := f.Truncate(MaxKeyFileSize); err != nil { // a sparse file: no disk is used
			t.Fatal(err)
		}
		s, err := ReadKeyFile(context.Background(), big)
		if err != nil || s.Len() != MaxKeyFileSize {
			t.Fatalf("a file of the limit: %d bytes, %v", s.Len(), err)
		}
		s.Wipe()
		if err := f.Truncate(MaxKeyFileSize + 1); err != nil {
			t.Fatal(err)
		}
		_, err = ReadKeyFile(context.Background(), big)
		assertRoute(t, err, "ReadKeyFile.KeyFileTooLarge")
	})
}
