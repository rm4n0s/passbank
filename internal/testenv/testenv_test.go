package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fatal is what the fake's Fatalf panics with, so that a test can see the
// failure and go on.
type fatal string

// fakeTB records what Isolate and Fixture ask of the test.
type fakeTB struct {
	dir string
	env map[string]string
}

func (f *fakeTB) Helper()         {}
func (f *fakeTB) TempDir() string { return f.dir }
func (f *fakeTB) Setenv(key, value string) {
	if f.env == nil {
		f.env = map[string]string{}
	}
	f.env[key] = value
}
func (f *fakeTB) Fatalf(format string, args ...any) { panic(fatal(fmt.Sprintf(format, args...))) }

// failure runs fn and returns the message it failed the test with, or "".
func failure(fn func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(fatal)
			if !ok {
				panic(r)
			}
			msg = string(f)
		}
	}()
	fn()
	return ""
}

func TestIsolate(t *testing.T) {
	f := &fakeTB{dir: t.TempDir()}
	env := Isolate(f)
	wantVars := map[string]string{
		"HOME":            env.Home,
		"XDG_CONFIG_HOME": env.Config,
		"XDG_STATE_HOME":  env.State,
		"XDG_CACHE_HOME":  env.Cache,
		"XDG_DATA_HOME":   env.Data,
		"XDG_RUNTIME_DIR": env.Runtime,
	}
	for key, want := range wantVars {
		if f.env[key] != want {
			t.Errorf("%s = %q, want %q", key, f.env[key], want)
		}
	}
	for _, key := range []string{"XDG_CONFIG_DIRS", "XDG_DATA_DIRS"} {
		if !strings.HasPrefix(f.env[key], f.dir) {
			t.Errorf("%s = %q, want a folder below %s", key, f.env[key], f.dir)
		}
	}
	for key, dir := range f.env {
		if !strings.HasPrefix(dir, f.dir+string(filepath.Separator)) {
			t.Errorf("%s = %q is outside the test's temporary folder", key, dir)
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("%s: %s is not a folder (%v)", key, dir, err)
		}
	}
	info, err := os.Stat(env.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("runtime folder mode = %v, want 0700", info.Mode().Perm())
	}
}

func TestIsolateSetsAndRestoresRealVariables(t *testing.T) {
	before := os.Getenv("HOME")
	var inside string
	t.Run("inner", func(t *testing.T) {
		env := Isolate(t)
		inside = os.Getenv("HOME")
		if inside != env.Home || inside == before {
			t.Errorf("HOME = %q inside the test, want %q (not %q)", inside, env.Home, before)
		}
	})
	if got := os.Getenv("HOME"); got != before {
		t.Errorf("HOME = %q after the test, want it restored to %q", got, before)
	}
}

func TestIsolateFailsWhenAFolderCannotBeMade(t *testing.T) {
	// A file where the temporary folder should be: nothing can be made below it.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	msg := failure(func() { Isolate(&fakeTB{dir: file}) })
	if !strings.Contains(msg, "cannot make") {
		t.Errorf("failure = %q, want it to say the folder cannot be made", msg)
	}
}

func TestFixture(t *testing.T) {
	path := Fixture(t, "sample.txt")
	if !filepath.IsAbs(path) || filepath.Base(path) != "sample.txt" {
		t.Errorf("Fixture = %q, want an absolute path to sample.txt", path)
	}
	tests := []struct {
		name string
		want string
	}{
		{"missing.txt", "is missing"},
		{"../testenv.go", "must stay inside testdata"},
		{"/etc/passwd", "must stay inside testdata"},
	}
	for _, tt := range tests {
		msg := failure(func() { Fixture(&fakeTB{}, tt.name) })
		if !strings.Contains(msg, tt.want) {
			t.Errorf("Fixture(%q) failed with %q, want %q", tt.name, msg, tt.want)
		}
	}
}
