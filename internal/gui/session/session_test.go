package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "core", "kdbx", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUnlock(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		password string
		key      string
		route    string // "" for success
	}{
		{"a password", "k4-rich.kdbx", "fixture-rich", "", ""},
		{"a key file", "k4-keyfile-xml2.kdbx", "", "keyfile-xml2.keyx", ""},
		{"a password and a key file", "k4-password-and-keyfile.kdbx", "fixture-rich", "keyfile-raw32.key", ""},
		{"a KDBX 3.1 file", "k31-rich.kdbx", "fixture-rich", "", ""},
		{"a wrong password", "k4-rich.kdbx", "wrong", "", "Unlock->OpenBytes"},
		{"no credentials", "k4-rich.kdbx", "", "", "NoCredentials"},
		{"a missing file", "missing.kdbx", "x", "", "Unlock->ReadFile"},
		{"a missing key file", "k4-rich.kdbx", "fixture-rich", "missing.key", "Unlock->ReadKeyFile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.key
			if key != "" && tt.route == "" {
				key = fixture(t, key)
			}
			src, err := Unlock(context.Background(), fixture(t, tt.file), secret.New([]byte(tt.password)), key)
			if tt.route != "" {
				appErr, ok := errors.FromError(err)
				if !ok || !appErr.HasRoute(tt.route) {
					t.Fatalf("err = %v, want route %s", err, tt.route)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer src.Wipe()
			if src.Info.Entries == 0 || src.Hash == ([32]byte{}) || src.PlanInput().Name != tt.file {
				t.Errorf("source = %+v", src.Info)
			}
		})
	}
}

func TestSession(t *testing.T) {
	open := func(name string) *Source {
		src, err := Unlock(context.Background(), fixture(t, name), secret.New([]byte("fixture-rich")), "")
		if err != nil {
			t.Fatal(err)
		}
		return src
	}
	s := New()
	if s.Ready() {
		t.Fatal("an empty session is ready")
	}
	a, b := open("k4-rich.kdbx"), open("k31-rich.kdbx")
	s.SetA(a)
	s.SetB(b)
	if !s.Ready() {
		t.Fatal("the session is not ready with both sides")
	}
	s.Swap()
	if s.A != b || s.B != a {
		t.Error("Swap did not exchange the sides")
	}
	// Unlocking a side again wipes the source it replaces.
	c := open("k4-rich.kdbx")
	s.SetA(c)
	if s.A != c {
		t.Error("SetA did not replace the source")
	}
	s.Wipe()
	if s.A != nil || s.B != nil || s.Plan != nil || s.Ready() {
		t.Error("Wipe left something")
	}
	// Wiping an empty session or a nil source is allowed.
	s.Wipe()
	(*Source)(nil).Wipe()
}

func TestWipeOverwritesTheSecrets(t *testing.T) {
	src, err := Unlock(context.Background(), fixture(t, "k4-rich.kdbx"), secret.New([]byte("fixture-rich")), "")
	if err != nil {
		t.Fatal(err)
	}
	var password secret.Bytes
	src.DB.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
		if f, ok := e.Field(model.FieldPassword); ok && f.Value.Len() > 0 {
			password = f.Value
			return false
		}
		return true
	})
	if password.Len() == 0 {
		t.Fatal("the fixture has no password to look at")
	}
	creds := src.Creds.Password
	src.Wipe()
	for _, b := range password.Reveal() {
		if b != 0 {
			t.Fatal("an entry's password was not wiped")
		}
	}
	for _, b := range creds.Reveal() {
		if b != 0 {
			t.Fatal("the master password was not wiped")
		}
	}
}
