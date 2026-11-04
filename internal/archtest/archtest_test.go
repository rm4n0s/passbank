package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
)

const fixtureModule = "example.com/passbank"

// strs formats violations as "rule file:line".
func strs(vs []Violation) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.String()
	}
	return out
}

func TestCheckFixtures(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"good", nil},
		{"bad-glutter", []string{
			"glutter-outside-gui cmd/passbank/main.go:3",
			"glutter-outside-gui internal/cli/cli.go:3",
			"glutter-outside-gui internal/core/model/model.go:3",
		}},
		{"bad-layers", []string{
			"gui-outside-gui cmd/passbank/main.go:3",
			"cli-imports-gui internal/cli/cli.go:3",
			"gui-outside-gui internal/cli/cli.go:3",
			"core-imports-frontend internal/core/diff/diff.go:3",
			"gui-outside-gui internal/core/diff/diff.go:3",
			"core-imports-frontend internal/core/model/model.go:3",
			"gui-imports-cli internal/gui/app/app.go:3",
		}},
		{"bad-imports", []string{
			"no-cgo internal/core/model/cgo.go:3",
			"no-network internal/core/model/net.go:4",
			"no-network internal/core/model/net.go:5",
			"no-network internal/core/model/net.go:6",
			"no-std-errors internal/core/model/std.go:3",
			"term-outside-cli internal/gui/app/term.go:3",
		}},
		{"bad-calls", []string{
			"no-fmt-errorf internal/core/model/a.go:6",
			"no-fmt-errorf internal/core/model/b.go:7",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Check(filepath.Join("testdata", tt.name), fixtureModule)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if !slices.Equal(strs(got), tt.want) {
				t.Errorf("violations:\n got %q\nwant %q", strs(got), tt.want)
			}
			for _, v := range got {
				if v.Message == "" {
					t.Errorf("violation %s has no message", v)
				}
			}
		})
	}
}

// TestRepository enforces the rules on Passbank itself.
func TestRepository(t *testing.T) {
	root := filepath.Join("..", "..")
	module, err := ModulePath(root)
	if err != nil {
		t.Fatalf("ModulePath: %v", err)
	}
	got, err := Check(root, module)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, v := range got {
		t.Errorf("%s: %s", v, v.Message)
	}
}

// TestCLIDoesNotLinkGlutter checks the whole dependency graph of the CLI, which
// the import rules only see one package at a time.
func TestCLIDoesNotLinkGlutter(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/passbank")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if under(pkg, glutterModule) {
			t.Errorf("cmd/passbank links %s", pkg)
		}
	}
}

func TestUnder(t *testing.T) {
	tests := []struct {
		p, dir string
		want   bool
	}{
		{"internal/core", "internal/core", true},
		{"internal/core/model", "internal/core", true},
		{"internal/corex", "internal/core", false},
		{"internal", "internal/core", false},
		{"cmd/passbank-gui", "cmd/passbank", false},
	}
	for _, tt := range tests {
		if got := under(tt.p, tt.dir); got != tt.want {
			t.Errorf("under(%q, %q) = %v, want %v", tt.p, tt.dir, got, tt.want)
		}
	}
}

func TestModulePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("// comment\nmodule \"example.com/x\"\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ModulePath(dir)
	if err != nil || got != "example.com/x" {
		t.Errorf("ModulePath = %q, %v; want example.com/x", got, err)
	}
}

// assertRoute fails the test unless err is an *errors.Error with the given route.
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

func TestErrorRoutes(t *testing.T) {
	t.Run("GoModReadFailed", func(t *testing.T) {
		_, err := ModulePath(t.TempDir())
		assertRoute(t, err, "ModulePath.GoModReadFailed")
	})
	t.Run("ModuleLineMissing", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.27\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ModulePath(dir)
		assertRoute(t, err, "ModulePath.ModuleLineMissing")
	})
	t.Run("RootWalkFailed", func(t *testing.T) {
		_, err := Check(filepath.Join(t.TempDir(), "missing"), fixtureModule)
		assertRoute(t, err, "Check->")
		assertRoute(t, err, "RootWalkFailed")
	})
	t.Run("SourceParseFailed", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package x\n\nfunc {\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := Check(dir, fixtureModule)
		assertRoute(t, err, "SourceParseFailed")
	})
	t.Run("SourceReadFailed", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can read any file")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "locked.go")
		if err := os.WriteFile(path, []byte("package x\n"), 0o000); err != nil {
			t.Fatal(err)
		}
		_, err := Check(dir, fixtureModule)
		assertRoute(t, err, "SourceReadFailed")
	})
}
