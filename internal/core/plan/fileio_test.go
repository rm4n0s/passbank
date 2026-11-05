package plan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/diff"
)

func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestWriteFileRoutes(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "plan")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		path    string
		replace bool
		ctx     context.Context
		route   string
	}{
		{"exists", existing, false, context.Background(), "WriteFile.PlanFileExists"},
		{"missing folder", filepath.Join(dir, "no", "plan"), false, context.Background(), "WriteFile.PlanFileWriteFailed"},
		{"missing folder with replace", filepath.Join(dir, "no", "plan"), true, context.Background(), "WriteFile.PlanFileWriteFailed"},
		{"cancelled", filepath.Join(dir, "new"), false, cancelled(), "WriteFile.WriteCancelled"},
		{"a folder with replace", dir, true, context.Background(), "WriteFile.PlanFileWriteFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteFile(tt.ctx, tt.path, []byte("data"), tt.replace)
			appErr, ok := errors.FromError(err)
			if !ok {
				t.Fatalf("expected *errors.Error, got %T (%v)", err, err)
			}
			if !appErr.HasRoute(tt.route) {
				t.Errorf("route = %s, want %s", appErr.Route(), tt.route)
			}
		})
	}
	if b, _ := os.ReadFile(existing); string(b) != "x" {
		t.Errorf("the existing file was changed: %q", b)
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan")
	if err := WriteFile(context.Background(), path, []byte("one"), false); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("stat = %v, %v; the file must be readable by its owner only", fi, err)
	}
	if err := WriteFile(context.Background(), path, []byte("two"), true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "two" {
		t.Errorf("the file holds %q after a replace", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("%d files in the folder; no temporary file may stay", len(entries))
	}
}

func TestReadFile(t *testing.T) {
	changes := sample()
	p := New(inA, inB, changes)
	var text strings.Builder
	if err := Write(&text, p, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan")
	if err := WriteFile(context.Background(), path, []byte(text.String()), false); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(context.Background(), path, inA, inB, changes)
	if err != nil {
		t.Fatal(err)
	}
	for i := range p.Choices {
		if got.Choices[i] != p.Choices[i] {
			t.Errorf("choice %d = %q, want %q", i, got.Choices[i], p.Choices[i])
		}
	}
}

func TestReadFileRoutes(t *testing.T) {
	_, err := ReadFile(context.Background(), filepath.Join(t.TempDir(), "missing"), Input{}, Input{}, nil)
	appErr, ok := errors.FromError(err)
	if !ok || !appErr.HasRoute("ReadFile.PlanFileReadFailed") {
		t.Errorf("err = %v", err)
	}
	_, err = ReadFile(cancelled(), "x", Input{}, Input{}, nil)
	appErr, ok = errors.FromError(err)
	if !ok || !appErr.HasRoute("ReadFile.ReadCancelled") {
		t.Errorf("err = %v", err)
	}
	// A file that is not a plan fails in Read, through ReadFile.
	bad := filepath.Join(t.TempDir(), "bad")
	if err := os.WriteFile(bad, []byte("not a plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ReadFile(context.Background(), bad, Input{}, Input{}, []diff.Change(nil))
	appErr, ok = errors.FromError(err)
	if !ok || !appErr.HasRoute("ReadFile->Read.PlanSyntax") {
		t.Errorf("err = %v", err)
	}
}

func TestCheckPathRoute(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.kdbx"), filepath.Join(dir, "b.kdbx")
	for _, f := range []string{a, b} {
		if err := os.WriteFile(f, []byte("db"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{a, b, link} {
		err := CheckPath(path, a, b)
		appErr, ok := errors.FromError(err)
		if !ok || !appErr.HasRoute("CheckPath.PlanOutputIsInput") {
			t.Errorf("%s: err = %v", path, err)
		}
	}
	if err := CheckPath(filepath.Join(dir, "plan"), a, b); err != nil {
		t.Errorf("a new file: %v", err)
	}
}
