package testutil

import (
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/passbank/internal/gui/kit/fonts"
	"github.com/rm4n0s/passbank/internal/testenv"
)

// proofFlag turns on writing proof images (docs/IMPLEMENTATION.md, proof method
// A): go test ./internal/gui/<pkg> -run <TestName> -proof
var proofFlag = flag.Bool("proof", false, "write the proof images of the tests to docs/implemented-proof-images")

// New returns a widget tester that lays text out with the bundled test fonts
// (the fonts of the app, never the system's) and runs with isolated paths. Like
// testenv.Isolate it uses Setenv, so the test cannot run in parallel.
func New(t *testing.T, opts ...gluttertest.Opts) *gluttertest.Tester {
	t.Helper()
	testenv.Isolate(t)
	var o gluttertest.Opts
	if len(opts) > 0 {
		o = opts[0]
	}
	o.Fonts = append(Fonts(), o.Fonts...)
	return gluttertest.New(t, o)
}

// Fonts returns the fonts of the app as gluttertest fonts.
func Fonts() []gluttertest.Font {
	faces := fonts.Faces()
	out := make([]gluttertest.Font, len(faces))
	for i, f := range faces {
		out[i] = gluttertest.Font{Family: f.Family, Data: f.Data}
	}
	return out
}

// Proof writes the last frame of ts to docs/implemented-proof-images/<name>.png
// when the test runs with -proof, and does nothing otherwise. name has no
// extension. Proof images show fixture data only, never a real password.
func Proof(t *testing.T, ts *gluttertest.Tester, name string) {
	t.Helper()
	if !*proofFlag {
		return
	}
	dir := filepath.Join(repoRoot(t), "docs", "implemented-proof-images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("testutil: cannot make %s: %v", dir, err)
	}
	path := filepath.Join(dir, name+".png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("testutil: cannot create %s: %v", path, err)
	}
	if err := png.Encode(f, ts.Screenshot()); err != nil {
		_ = f.Close() // the write error is the one to report
		t.Fatalf("testutil: cannot write %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("testutil: cannot close %s: %v", path, err)
	}
	t.Logf("proof image written to %s", path)
}

// repoRoot returns the folder with go.mod above the test's folder.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("testutil: cannot get the working folder: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("testutil: no go.mod above the working folder")
		}
		dir = parent
	}
}
