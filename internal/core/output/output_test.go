package output

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// fixtureDB reads a database of the kdbx fixtures.
func fixtureDB(t *testing.T, file, password string) *model.Database {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "kdbx", "testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := kdbx.Open(context.Background(), path, crypto.Credentials{HasPassword: true, Password: secret.New([]byte(password))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Wipe)
	if file != "k41-fields.kdbx" {
		// pykeepass writes the 4.1 element SettingsChanged into its 4.0 files; the tests
		// that write 4.0 need databases that 4.0 holds.
		db.Meta.SettingsChanged = time.Time{}
	}
	return db
}

var (
	creds    = crypto.Credentials{HasPassword: true, Password: secret.New([]byte("output password"))}
	settings = kdbx.EncryptionSettings{CipherID: crypto.CipherAES256, KDF: crypto.KDF{ID: crypto.KDFAES, Rounds: 20}, Compression: kdbx.CompressionGZip}
	v40      = kdbx.Version{Major: 4}
)

func assertRoute(t *testing.T, err error, route string) {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
}

// names lists the files of a folder.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func readAll(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWriteNewFile(t *testing.T) {
	db := fixtureDB(t, "k4-rich.kdbx", "fixture-rich")
	dir := t.TempDir()
	path := filepath.Join(dir, "out.kdbx")
	if err := Write(context.Background(), path, db, v40, settings, creds, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dir); len(got) != 1 || got[0] != "out.kdbx" {
		t.Errorf("the folder holds %v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
	got, _, err := kdbx.Open(context.Background(), path, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Wipe()
	if diff := model.Diff(kdbx.Fit(db, v40), got); len(diff) != 0 {
		t.Errorf("the file differs: %v", diff)
	}
}

func TestWriteExistingFile(t *testing.T) {
	db := fixtureDB(t, "k4-empty.kdbx", "fixture-min")
	dir := t.TempDir()
	path := filepath.Join(dir, "out.kdbx")
	if err := os.WriteFile(path, []byte("precious"), 0o640); err != nil {
		t.Fatal(err)
	}
	err := Write(context.Background(), path, db, v40, settings, creds, Options{})
	assertRoute(t, err, "write.OutputExists")
	if string(readAll(t, path)) != "precious" || len(names(t, dir)) != 1 {
		t.Errorf("the existing file or the folder changed: %v", names(t, dir))
	}
	if err := Write(context.Background(), path, db, v40, settings, creds, Options{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := kdbx.Open(context.Background(), path, creds); err != nil {
		t.Errorf("the replaced file does not open: %v", err)
	}
	if len(names(t, dir)) != 1 {
		t.Errorf("the folder holds %v", names(t, dir))
	}
}

func TestWriteInputs(t *testing.T) {
	db := fixtureDB(t, "k4-empty.kdbx", "fixture-min")
	dir := t.TempDir()
	a := filepath.Join(dir, "a.kdbx")
	if err := os.WriteFile(a, []byte("input a"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.kdbx")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	hard := filepath.Join(dir, "hard.kdbx")
	if err := os.Link(a, hard); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "b.kdbx")
	if err := os.WriteFile(other, []byte("input b"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, path string
		inputs     []string
		overwrite  bool
	}{
		{"the same path", a, []string{a}, false},
		{"the same path with Overwrite", a, []string{a}, true},
		{"a spelling of the path", filepath.Join(dir, ".", "a.kdbx"), []string{a}, true},
		{"a path with ..", filepath.Join(dir, "x", "..", "a.kdbx"), []string{a}, true},
		{"a symbolic link to the input", link, []string{a}, true},
		{"a hard link to the input", hard, []string{a}, true},
		{"the input is the link", a, []string{link}, true},
		{"the second input", other, []string{a, other}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "a path with .." {
				if err := os.Mkdir(filepath.Join(dir, "x"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			err := Write(context.Background(), tt.path, db, v40, settings, creds, Options{Inputs: tt.inputs, Overwrite: tt.overwrite})
			assertRoute(t, err, "checkInputs.OutputIsInput")
			if string(readAll(t, a)) != "input a" || string(readAll(t, other)) != "input b" {
				t.Error("an input changed")
			}
		})
	}
	// Inputs that are not the output do not matter, and a missing input is not a problem.
	out := filepath.Join(dir, "new.kdbx")
	if err := Write(context.Background(), out, db, v40, settings, creds, Options{Inputs: []string{a, other, filepath.Join(dir, "missing")}}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteBadPaths(t *testing.T) {
	db := fixtureDB(t, "k4-empty.kdbx", "fixture-min")
	err := Write(context.Background(), "", db, v40, settings, creds, Options{})
	assertRoute(t, err, "write.OutputPathInvalid")
	err = Write(context.Background(), filepath.Join(t.TempDir(), "no", "such", "dir", "out.kdbx"), db, v40, settings, creds, Options{})
	assertRoute(t, err, "write.OutputTempFailed")
	// A folder at the path cannot be replaced.
	dir := t.TempDir()
	target := filepath.Join(dir, "folder")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	err = Write(context.Background(), target, db, v40, settings, creds, Options{Overwrite: true})
	assertRoute(t, err, "write.OutputPlaceFailed")
	if got := names(t, dir); len(got) != 1 {
		t.Errorf("the folder holds %v", got)
	}
}

func TestWriteEncodeErrors(t *testing.T) {
	db := fixtureDB(t, "k4-empty.kdbx", "fixture-min")
	dir := t.TempDir()
	path := filepath.Join(dir, "out.kdbx")
	err := Write(context.Background(), path, db, kdbx.Version{Major: 9}, settings, creds, Options{})
	assertRoute(t, err, "Encode->encode.VersionUnsupported")
	err = Write(context.Background(), path, db, v40, settings, crypto.Credentials{}, Options{})
	assertRoute(t, err, "NoCredentials")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = Write(ctx, path, db, v40, settings, creds, Options{})
	assertRoute(t, err, "KdfCancelled")
	if len(names(t, dir)) != 0 {
		t.Errorf("the folder holds %v", names(t, dir))
	}
}

// faultFS is the real file system with hooks that make single steps fail.
type faultFS struct {
	OS
	createTemp func() error
	wrap       func(File) File
	link       func(oldname, newname string) error
	rename     func(oldname, newname string) error
	remove     func(name string) error
	syncDir    func() error
	// onCreate runs after the temporary file is made.
	onCreate func()
}

func (f *faultFS) CreateTemp(dir, pattern string) (File, error) {
	if f.createTemp != nil {
		if err := f.createTemp(); err != nil {
			return nil, err
		}
	}
	file, err := f.OS.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	if f.onCreate != nil {
		f.onCreate()
	}
	if f.wrap != nil {
		return f.wrap(file), nil
	}
	return file, nil
}

func (f *faultFS) Link(o, n string) error {
	if f.link != nil {
		return f.link(o, n)
	}
	return f.OS.Link(o, n)
}

func (f *faultFS) Rename(o, n string) error {
	if f.rename != nil {
		return f.rename(o, n)
	}
	return f.OS.Rename(o, n)
}

func (f *faultFS) Remove(n string) error {
	if f.remove != nil {
		return f.remove(n)
	}
	return f.OS.Remove(n)
}

func (f *faultFS) SyncDir(d string) error {
	if f.syncDir != nil {
		return f.syncDir()
	}
	return f.OS.SyncDir(d)
}

// failingFile fails the step named by the hooks and otherwise passes through.
type failingFile struct {
	File
	write func([]byte) (int, error)
	sync  func() error
	close func() error
}

func (f *failingFile) Write(p []byte) (int, error) {
	if f.write != nil {
		return f.write(p)
	}
	return f.File.Write(p)
}

func (f *failingFile) Sync() error {
	if f.sync != nil {
		return f.sync()
	}
	return f.File.Sync()
}

func (f *failingFile) Close() error {
	if f.close != nil {
		cerr := f.File.Close()
		if err := f.close(); err != nil {
			return err
		}
		return cerr
	}
	return f.File.Close()
}

var errInjected = fs.ErrInvalid

func TestWriteFaults(t *testing.T) {
	db := fixtureDB(t, "k4-empty.kdbx", "fixture-min")
	tests := []struct {
		name        string
		fs          func() *faultFS
		route       string
		wantPlaced  bool // the new file is at the path in the end
		existingOld bool // an existing file at the path is still the old one
	}{
		{"the temporary file cannot be made", func() *faultFS {
			return &faultFS{createTemp: func() error { return errInjected }}
		}, "write.OutputTempFailed", false, true},
		{"a write fails", func() *faultFS {
			return &faultFS{wrap: func(f File) File {
				return &failingFile{File: f, write: func([]byte) (int, error) { return 0, errInjected }}
			}}
		}, "write.OutputWriteFailed", false, true},
		{"the sync fails", func() *faultFS {
			return &faultFS{wrap: func(f File) File { return &failingFile{File: f, sync: func() error { return errInjected }} }}
		}, "write.OutputWriteFailed", false, true},
		{"the close fails", func() *faultFS {
			return &faultFS{wrap: func(f File) File { return &failingFile{File: f, close: func() error { return errInjected }} }}
		}, "write.OutputWriteFailed", false, true},
		{"the link fails", func() *faultFS {
			return &faultFS{link: func(string, string) error { return errInjected }}
		}, "write.OutputPlaceFailed", false, true},
		{"removing the temporary name after the link fails", func() *faultFS {
			first := true
			return &faultFS{remove: func(n string) error {
				if first { // the first Remove is the one after the link
					first = false
					return errInjected
				}
				return os.Remove(n)
			}}
		}, "write.OutputPlaceFailed", true, false},
		{"the folder cannot be synced", func() *faultFS {
			return &faultFS{syncDir: func() error { return errInjected }}
		}, "write.OutputSyncFailed", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "out.kdbx")
			err := Write(context.Background(), path, db, v40, settings, creds, Options{FS: tt.fs()})
			assertRoute(t, err, tt.route)
			files := names(t, dir)
			if tt.wantPlaced {
				if _, _, err := kdbx.Open(context.Background(), path, creds); err != nil {
					t.Errorf("the file is not complete: %v", err)
				}
				for _, n := range files {
					if n != "out.kdbx" && !strings.HasPrefix(n, ".out.kdbx.tmp-") {
						t.Errorf("unexpected file %s", n)
					}
				}
			} else if len(files) != 0 {
				t.Errorf("the folder holds %v", files)
			}
		})
		t.Run(tt.name+", over an existing file", func(t *testing.T) {
			if !tt.existingOld {
				t.Skip("the new file is in place in this case")
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "out.kdbx")
			if err := os.WriteFile(path, []byte("precious"), 0o600); err != nil {
				t.Fatal(err)
			}
			f := tt.fs()
			// With Overwrite the placing step is the rename.
			if f.link != nil {
				f.rename, f.link = f.link, nil
			}
			err := Write(context.Background(), path, db, v40, settings, creds, Options{FS: f, Overwrite: true})
			assertRoute(t, err, tt.route)
			if string(readAll(t, path)) != "precious" || len(names(t, dir)) != 1 {
				t.Errorf("the existing file changed or a file was left: %v", names(t, dir))
			}
		})
	}
}

func TestWriteLosesARaceToAnotherFile(t *testing.T) {
	db := fixtureDB(t, "k4-empty.kdbx", "fixture-min")
	dir := t.TempDir()
	path := filepath.Join(dir, "out.kdbx")
	f := &faultFS{onCreate: func() {
		// Someone makes the file after the check and before the link.
		if err := os.WriteFile(path, []byte("made meanwhile"), 0o600); err != nil {
			t.Error(err)
		}
	}}
	err := Write(context.Background(), path, db, v40, settings, creds, Options{FS: f})
	assertRoute(t, err, "write.OutputExists")
	if string(readAll(t, path)) != "made meanwhile" || len(names(t, dir)) != 1 {
		t.Errorf("the other file changed or a file was left: %v", names(t, dir))
	}
}

func TestWriteVerification(t *testing.T) {
	db := fixtureDB(t, "k4-rich.kdbx", "fixture-rich")
	t.Run("a file that does not read back", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.kdbx")
		if err := os.WriteFile(path, []byte("precious"), 0o600); err != nil {
			t.Fatal(err)
		}
		flip := &faultFS{wrap: func(f File) File {
			return &failingFile{File: f, write: func(p []byte) (int, error) {
				q := bytes.Clone(p)
				q[len(q)-5] ^= 1 // a byte of the last block
				return f.Write(q)
			}}
		}}
		err := Write(context.Background(), path, db, v40, settings, creds, Options{FS: flip, Overwrite: true})
		assertRoute(t, err, "verify.WriteVerifyFailed")
		appErr, _ := errors.FromError(err)
		if appErr.Metadata["cause"] != "FileCorrupt" {
			t.Errorf("metadata = %v", appErr.Metadata)
		}
		if string(readAll(t, path)) != "precious" || len(names(t, dir)) != 1 {
			t.Errorf("the existing file changed or a file was left: %v", names(t, dir))
		}
	})
	t.Run("a model that differs", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.kdbx")
		other := fixtureDB(t, "k4-empty.kdbx", "fixture-min")
		err := write(context.Background(), path, db, v40, settings, creds, Options{}, kdbx.Fit(other, v40))
		assertRoute(t, err, "verify.WriteVerifyFailed")
		appErr, _ := errors.FromError(err)
		if n, _ := appErr.Metadata["count"].(int); n == 0 {
			t.Errorf("metadata = %v", appErr.Metadata)
		}
		// The differences name paths and UUIDs, not values.
		text, _ := appErr.Metadata["differences"].(string)
		if text == "" || strings.Contains(text, "bank-password") || strings.Contains(text, "Bank") {
			t.Errorf("differences = %q", text)
		}
		if len(names(t, dir)) != 0 {
			t.Errorf("the folder holds %v", names(t, dir))
		}
	})
	t.Run("cancelled during the verification", func(t *testing.T) {
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		f := &faultFS{onCreate: cancel}
		err := Write(ctx, filepath.Join(dir, "out.kdbx"), db, v40, settings, creds, Options{FS: f})
		assertRoute(t, err, "verify.WriteCancelled")
		if len(names(t, dir)) != 0 {
			t.Errorf("the folder holds %v", names(t, dir))
		}
	})
}

func TestWriteDoesNotChangeItsInputs(t *testing.T) {
	db := fixtureDB(t, "k41-fields.kdbx", "fixture-rich")
	before := db.Clone()
	c := creds
	path := filepath.Join(t.TempDir(), "out.kdbx")
	// The version 4.0 cannot hold the 4.1 fields; the database is not changed by that.
	if err := Write(context.Background(), path, db, v40, settings, c, Options{AllowLoss: true}); err != nil {
		t.Fatal(err)
	}
	if diff := model.Diff(before, db); len(diff) != 0 {
		t.Errorf("Write changed the database: %v", diff)
	}
	if string(c.Password.Reveal()) != "output password" {
		t.Error("Write wiped the credentials")
	}
	got, _, err := kdbx.Open(context.Background(), path, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Wipe()
	if kdbx.MinVersion(got) != v40 {
		t.Error("the 4.0 file holds 4.1 fields")
	}
}

func TestWriteRefusesALossUnlessAllowed(t *testing.T) {
	db := fixtureDB(t, "k41-fields.kdbx", "fixture-rich")
	dir := t.TempDir()
	path := filepath.Join(dir, "out.kdbx")
	err := Write(context.Background(), path, db, v40, settings, creds, Options{})
	assertRoute(t, err, "lossesError.OutputVersionLosesData")
	appErr, _ := errors.FromError(err)
	if n, _ := appErr.Metadata["count"].(int); n < 3 {
		t.Errorf("metadata = %v", appErr.Metadata)
	}
	text, _ := appErr.Metadata["losses"].(string)
	for _, want := range []string{"GroupTags Root", "EntryQualityCheck Root/Bank", "SettingsChanged database"} {
		if !strings.Contains(text, want) {
			t.Errorf("the losses lack %q: %s", want, text)
		}
	}
	for _, secret := range []string{"bank-password", "older-bank-password", "4711"} {
		if strings.Contains(text, secret) {
			t.Errorf("a value is in the losses: %s", text)
		}
	}
	if len(names(t, dir)) != 0 {
		t.Errorf("the folder holds %v", names(t, dir))
	}
	// The same database fits 4.1 and needs no permission.
	if err := Write(context.Background(), path, db, kdbx.Version{Major: 4, Minor: 1}, settings, creds, Options{}); err != nil {
		t.Fatal(err)
	}
	// An existing file is not touched by the refusal.
	if err := Write(context.Background(), path, db, v40, settings, creds, Options{Overwrite: true}); err == nil {
		t.Error("the loss was accepted without AllowLoss")
	}
	if got, _, err := kdbx.Open(context.Background(), path, creds); err != nil || kdbx.MinVersion(got) != (kdbx.Version{Major: 4, Minor: 1}) {
		t.Errorf("the 4.1 file was replaced: %v", err)
	}
}

func TestWriteRefusesEncryptionTheVersionLacks(t *testing.T) {
	db := fixtureDB(t, "k4-empty.kdbx", "fixture-min")
	dir := t.TempDir()
	argon := kdbx.EncryptionSettings{CipherID: crypto.CipherAES256, KDF: crypto.KDF{ID: crypto.KDFArgon2id, Memory: 1 << 20, Iterations: 1, Parallelism: 1}}
	err := Write(context.Background(), filepath.Join(dir, "out.kdbx"), db, kdbx.Version{Major: 3, Minor: 1}, argon, creds, Options{AllowLoss: true})
	assertRoute(t, err, "Supports.EncryptionNotInVersion")
	if len(names(t, dir)) != 0 {
		t.Errorf("the folder holds %v", names(t, dir))
	}
}
