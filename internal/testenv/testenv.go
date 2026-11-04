package testenv

import (
	"os"
	"path/filepath"
)

// TB is the part of testing.TB that this package uses; *testing.T and
// *testing.B satisfy it, and tests of this package use a fake.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
	TempDir() string
	Setenv(key, value string)
}

// Env holds the folders Isolate made, all below the test's temporary folder.
type Env struct {
	// Home is the HOME folder.
	Home string
	// Config is XDG_CONFIG_HOME.
	Config string
	// State is XDG_STATE_HOME.
	State string
	// Cache is XDG_CACHE_HOME.
	Cache string
	// Data is XDG_DATA_HOME.
	Data string
	// Runtime is XDG_RUNTIME_DIR; its mode is 0700, as the XDG specification
	// requires.
	Runtime string
}

// Isolate points HOME and every XDG variable at fresh folders below
// t.TempDir(), so that nothing the test (or a program it starts) reads or writes
// belongs to the user. t restores the variables when the test ends. Because it
// uses Setenv, a test that calls Isolate cannot run in parallel.
func Isolate(t TB) Env {
	t.Helper()
	root := t.TempDir()
	env := Env{
		Home:    filepath.Join(root, "home"),
		Config:  filepath.Join(root, "config"),
		State:   filepath.Join(root, "state"),
		Cache:   filepath.Join(root, "cache"),
		Data:    filepath.Join(root, "data"),
		Runtime: filepath.Join(root, "runtime"),
	}
	// The system-wide search paths are emptied too, so that nothing outside the
	// test's folder is searched.
	configDirs := filepath.Join(root, "etc-xdg")
	dataDirs := filepath.Join(root, "share")
	for _, dir := range []string{env.Home, env.Config, env.State, env.Cache, env.Data, env.Runtime, configDirs, dataDirs} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("testenv: cannot make %s: %v", dir, err)
		}
	}
	vars := []struct{ key, value string }{
		{"HOME", env.Home},
		{"XDG_CONFIG_HOME", env.Config},
		{"XDG_STATE_HOME", env.State},
		{"XDG_CACHE_HOME", env.Cache},
		{"XDG_DATA_HOME", env.Data},
		{"XDG_RUNTIME_DIR", env.Runtime},
		{"XDG_CONFIG_DIRS", configDirs},
		{"XDG_DATA_DIRS", dataDirs},
	}
	for _, v := range vars {
		t.Setenv(v.key, v.value)
	}
	return env
}

// Fixture returns the absolute path of the file name below the testdata folder
// of the package under test (tests run in their package's folder), and fails the
// test if the file does not exist or name leaves testdata.
func Fixture(t TB, name string) string {
	t.Helper()
	if !filepath.IsLocal(name) {
		t.Fatalf("testenv: fixture name %q must stay inside testdata", name)
	}
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("testenv: cannot resolve fixture %q: %v", name, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("testenv: fixture %q is missing: %v", name, err)
	}
	return path
}
