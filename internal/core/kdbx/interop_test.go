//go:build passbank_keepassxc

package kdbx

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// This file checks Passbank's output against KeePassXC, an independent reader. It
// runs with `go test -tags passbank_keepassxc ./internal/core/kdbx` and is skipped
// when keepassxc-cli is neither in the PATH nor installed as the Flatpak.

// keepassxc runs keepassxc-cli with the given arguments; password is given on
// standard input. dir is a folder the Flatpak may read.
func keepassxc(t *testing.T, dir, password string, args ...string) (string, error) {
	t.Helper()
	var cmd *exec.Cmd
	if path, err := exec.LookPath("keepassxc-cli"); err == nil {
		cmd = exec.Command(path, args...)
	} else if _, err := exec.LookPath("flatpak"); err == nil && exec.Command("flatpak", "info", "org.keepassxc.KeePassXC").Run() == nil {
		cmd = exec.Command("flatpak", append([]string{"run", "--filesystem=" + dir, "--command=keepassxc-cli", "org.keepassxc.KeePassXC"}, args...)...)
	} else {
		t.Skip("keepassxc-cli is not installed")
	}
	cmd.Stdin = strings.NewReader(password + "\n")
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return out.String(), errors.NewErr("KeePassXCFailed", err).SetMetadata("stderr", strings.TrimSpace(errs.String()))
		}
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatalf("keepassxc-cli %v timed out", args)
	}
	return out.String(), nil
}

// standardFields are what `keepassxc-cli show -s` prints for the five standard fields.
func showEntry(t *testing.T, dir, file, password, entryPath string) map[string]string {
	t.Helper()
	out, err := keepassxc(t, dir, password, "show", "-s", "-q", file, entryPath)
	if err != nil {
		t.Fatalf("show %s: %v", entryPath, err)
	}
	fields := map[string]string{}
	var key string
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(line, ": "); ok && (k == "Title" || k == "UserName" || k == "Password" || k == "URL" || k == "Notes" || k == "Uuid" || k == "Tags") {
			key = k
			fields[k] = v
		} else if key == "Notes" && line != "" {
			fields[key] += "\n" + line // notes may span lines
		}
	}
	return fields
}

// entryPaths returns the paths of the entries, as keepassxc-cli names them (without the root group).
func entryPaths(db *model.Database) map[string]*model.Entry {
	paths := map[string]*model.Entry{}
	var walk func(g *model.Group, prefix string)
	walk = func(g *model.Group, prefix string) {
		for _, e := range g.Entries {
			if e.Title() == "" {
				continue // keepassxc-cli cannot name an entry without a title
			}
			paths[prefix+e.Title()] = e
		}
		for _, c := range g.Groups {
			walk(c, prefix+c.Name+"/")
		}
	}
	walk(db.Root, "")
	return paths
}

// checkAgainstKeePassXC requires KeePassXC to list the entries of the model and to
// show the standard fields as the model has them.
func checkAgainstKeePassXC(t *testing.T, dir, file, password string, db *model.Database) {
	t.Helper()
	listing, err := keepassxc(t, dir, password, "ls", "-R", "-f", "-q", file)
	if err != nil {
		t.Fatalf("KeePassXC cannot open the file: %v", err)
	}
	var listed []string
	for _, l := range strings.Split(listing, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasSuffix(l, "/") && !strings.HasSuffix(l, "[empty]") {
			listed = append(listed, l)
		}
	}
	paths := entryPaths(db)
	var want []string
	for p := range paths {
		want = append(want, p)
	}
	sort.Strings(listed)
	sort.Strings(want)
	if strings.Join(listed, "\n") != strings.Join(want, "\n") {
		t.Errorf("KeePassXC lists\n%s\nthe model has\n%s", strings.Join(listed, "\n"), strings.Join(want, "\n"))
	}
	for p, e := range paths {
		got := showEntry(t, dir, file, password, p)
		for _, key := range []string{model.FieldTitle, model.FieldUserName, model.FieldPassword, model.FieldURL, model.FieldNotes} {
			f, _ := e.Field(key)
			if got[key] != f.Text() {
				t.Errorf("%s: %s is %q in KeePassXC and %q in the model", p, key, got[key], f.Text())
			}
		}
	}
}

func TestKeePassXCReadsEveryCipherAndKDF(t *testing.T) {
	db := openFixture(t, "k4-rich.kdbx", "fixture-rich", "")
	dir := t.TempDir()
	creds := crypto.Credentials{HasPassword: true, Password: secret.New([]byte("interop password"))}
	for _, cipher := range []model.UUID{crypto.CipherAES256, crypto.CipherTwofish, crypto.CipherChaCha20} {
		for name, kdf := range map[string]crypto.KDF{
			"AES-KDF":  {ID: crypto.KDFAES, Rounds: 1000},
			"Argon2d":  {ID: crypto.KDFArgon2d, Memory: 8 << 20, Iterations: 2, Parallelism: 2},
			"Argon2id": {ID: crypto.KDFArgon2id, Memory: 8 << 20, Iterations: 2, Parallelism: 2},
		} {
			for _, comp := range []Compression{CompressionNone, CompressionGZip} {
				t.Run(cipher.String()[:4]+"/"+name+"/"+comp.String(), func(t *testing.T) {
					data, err := Encode(context.Background(), db, Version{4, 0}, EncryptionSettings{CipherID: cipher, KDF: kdf, Compression: comp}, creds)
					if err != nil {
						t.Fatal(err)
					}
					file := filepath.Join(dir, strings.NewReplacer("/", "-", " ", "").Replace(t.Name())+".kdbx")
					if err := os.WriteFile(file, data, 0o600); err != nil {
						t.Fatal(err)
					}
					checkAgainstKeePassXC(t, dir, file, "interop password", db)
				})
			}
		}
	}
}

func TestKeePassXCReadsEveryFixtureWePass(t *testing.T) {
	dir := t.TempDir()
	creds := crypto.Credentials{HasPassword: true, Password: secret.New([]byte("interop password"))}
	for _, tt := range []struct{ file, password string }{
		{"k4-empty.kdbx", "fixture-min"}, {"k4-rich.kdbx", "fixture-rich"}, {"k4-unknown-xml.kdbx", "fixture-rich"},
		{"k41-fields.kdbx", "fixture-rich"}, {"k31-rich.kdbx", "fixture-rich"}, {"ext/rs-password.kdbx", "demopass"},
		{"ext/rs-attachments.kdbx", "demopass"}, {"ext/go-protected-binary.kdbx", "123"}, {"ext/web-cyrillic.kdbx", "пароль"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			db := openFixture(t, tt.file, tt.password, "")
			version := MinVersion(db)
			data, err := Encode(context.Background(), db, version, DefaultSettingsFast(version), creds)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, strings.NewReplacer("/", "-", " ", "").Replace(t.Name())+".kdbx")
			if err := os.WriteFile(file, data, 0o600); err != nil {
				t.Fatal(err)
			}
			checkAgainstKeePassXC(t, dir, file, "interop password", db)
		})
	}
}

// DefaultSettingsFast is the defaults with a KDF cheap enough for many files.
func DefaultSettingsFast(version Version) EncryptionSettings {
	s := DefaultSettings(version)
	if s.KDF.ID == crypto.KDFAES {
		s.KDF.Rounds = 1000
	} else {
		s.KDF.Memory, s.KDF.Iterations = 8<<20, 2
	}
	return s
}

func TestKeePassXCRejectsAWrongPassword(t *testing.T) {
	db := openFixture(t, "k4-empty.kdbx", "fixture-min", "")
	dir := t.TempDir()
	creds := crypto.Credentials{HasPassword: true, Password: secret.New([]byte("right"))}
	data, err := Encode(context.Background(), db, Version{4, 0}, DefaultSettingsFast(Version{4, 0}), creds)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "x.kdbx")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := keepassxc(t, dir, "wrong", "ls", "-q", file); err == nil {
		t.Error("KeePassXC opened the file with a wrong password")
	}
	if _, err := keepassxc(t, dir, "right", "ls", "-q", file); err != nil {
		t.Errorf("KeePassXC cannot open the file with the right one: %v", err)
	}
}
