// Package archtest checks Passbank's package boundaries and forbidden
// imports (SPEC §16, CLAUDE.md "Go conventions" and "Don't").
//
// Check takes the root folder of a module, so every rule is tested on small
// fixture trees under testdata/ as well as on the repository itself.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rm4n0s/errors"
)

// glutterModule is the import path prefix of the GUI toolkit.
const glutterModule = "github.com/rm4n0s/glutter"

// Violation is one broken rule at one place in the source.
type Violation struct {
	// Rule is the name of the broken rule.
	Rule string
	// File is the slash-separated path of the file, relative to the root.
	File string
	// Line is the line of the offending import or call.
	Line int
	// Message says what is wrong and what to do instead.
	Message string
}

// String formats the violation as "rule file:line", which is how tests compare
// violations.
func (v Violation) String() string {
	return v.Rule + " " + v.File + ":" + strconv.Itoa(v.Line)
}

// rule forbids some imports in some folders.
type rule struct {
	name    string
	message string
	// applies reports whether the rule covers files in the folder dir (a
	// slash-separated path relative to the root).
	applies func(dir string) bool
	// forbids reports whether the import path imp breaks the rule.
	forbids func(imp string) bool
}

// under reports whether p is dir or a folder below it.
func under(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// importRules returns the rules about imports for the module path module.
func importRules(module string) []rule {
	gui := module + "/internal/gui"
	cli := module + "/internal/cli"
	inGUI := func(dir string) bool { return under(dir, "internal/gui") }
	inGUIMain := func(dir string) bool { return under(dir, "cmd/passbank-gui") }
	return []rule{
		{
			name:    "glutter-outside-gui",
			message: "Glutter may only be imported by internal/gui and cmd/passbank-gui",
			applies: func(dir string) bool { return !inGUI(dir) && !inGUIMain(dir) },
			forbids: func(imp string) bool { return under(imp, glutterModule) },
		},
		{
			name:    "core-imports-frontend",
			message: "internal/core must not import internal/cli or internal/gui: the core is tested without a frontend",
			applies: func(dir string) bool { return under(dir, "internal/core") },
			forbids: func(imp string) bool { return under(imp, gui) || under(imp, cli) },
		},
		{
			name:    "cli-imports-gui",
			message: "internal/cli must not import internal/gui: the CLI never links the GUI",
			applies: func(dir string) bool { return under(dir, "internal/cli") },
			forbids: func(imp string) bool { return under(imp, gui) },
		},
		{
			name:    "gui-imports-cli",
			message: "internal/gui must not import internal/cli",
			applies: inGUI,
			forbids: func(imp string) bool { return under(imp, cli) },
		},
		{
			name:    "gui-outside-gui",
			message: "only cmd/passbank-gui may import internal/gui",
			applies: func(dir string) bool { return !inGUI(dir) && !inGUIMain(dir) },
			forbids: func(imp string) bool { return under(imp, gui) },
		},
		{
			name:    "term-outside-cli",
			message: "golang.org/x/term may only be imported by internal/cli",
			applies: func(dir string) bool { return !under(dir, "internal/cli") },
			forbids: func(imp string) bool { return imp == "golang.org/x/term" },
		},
		{
			name:    "no-cgo",
			message: `never write import "C": Passbank is cgo-free`,
			applies: func(string) bool { return true },
			forbids: func(imp string) bool { return imp == "C" },
		},
		{
			name:    "no-std-errors",
			message: `import "github.com/rm4n0s/errors" instead of the standard library errors package`,
			applies: func(string) bool { return true },
			forbids: func(imp string) bool { return imp == "errors" },
		},
		{
			name:    "no-network",
			message: "Passbank opens no network connections: net, net/* (except net/url) and crypto/tls are forbidden",
			applies: func(string) bool { return true },
			forbids: func(imp string) bool {
				if imp == "net/url" {
					return false
				}
				return imp == "net" || strings.HasPrefix(imp, "net/") || imp == "crypto/tls"
			},
		},
	}
}

// ModulePath returns the module path declared by the go.mod file in root.
func ModulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", errors.NewErr("GoModReadFailed", err).SetMetadata("root", root)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`), nil
		}
	}
	return "", errors.New("ModuleLineMissing", "go.mod has no module line", "root", root)
}

// walkGo parses every Go file below root and calls visit with the file's
// slash-separated path relative to root. Folders named testdata or vendor and
// folders that start with a dot or an underscore are skipped.
func walkGo(root string, visit func(rel string, fset *token.FileSet, file *ast.File)) error {
	fsys := os.DirFS(root)
	return fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.NewErr("RootWalkFailed", walkErr).SetMetadata("path", rel)
		}
		if d.IsDir() {
			name := d.Name()
			if rel != "." && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") {
			return nil
		}
		src, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return errors.NewErr("SourceReadFailed", err).SetMetadata("file", rel)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
		if err != nil {
			return errors.NewErr("SourceParseFailed", err).SetMetadata("file", rel)
		}
		visit(rel, fset, file)
		return nil
	})
}

// Check walks the Go files below root and returns every violation, sorted by
// file, line and rule. module is the module path of the code under root.
func Check(root, module string) ([]Violation, error) {
	rules := importRules(module)
	var found []Violation
	err := walkGo(root, func(rel string, fset *token.FileSet, file *ast.File) {
		found = append(found, checkFile(fset, file, rel, rules)...)
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Rule < b.Rule
	})
	return found, nil
}

// checkFile applies the import rules and the fmt.Errorf rule to one parsed
// file. rel is the file's slash-separated path relative to the root.
func checkFile(fset *token.FileSet, file *ast.File, rel string, rules []rule) []Violation {
	dir := "."
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		dir = rel[:i]
	}
	var found []Violation
	fmtName := ""
	for _, spec := range file.Imports {
		imp, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			// The parser accepted the file, so a path that does not unquote is not
			// an import any rule could be about.
			continue
		}
		if imp == "fmt" {
			fmtName = "fmt"
			if spec.Name != nil {
				fmtName = spec.Name.Name
			}
		}
		line := fset.Position(spec.Path.Pos()).Line
		for _, r := range rules {
			if r.applies(dir) && r.forbids(imp) {
				found = append(found, Violation{Rule: r.name, File: rel, Line: line, Message: r.message})
			}
		}
	}
	if fmtName == "" || fmtName == "_" || fmtName == "." {
		return found
	}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Errorf" {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); ok && x.Name == fmtName {
			found = append(found, Violation{
				Rule:    "no-fmt-errorf",
				File:    rel,
				Line:    fset.Position(call.Pos()).Line,
				Message: "use errors.New, errors.NewErr or errors.NewErrf from github.com/rm4n0s/errors instead of fmt.Errorf",
			})
		}
		return true
	})
	return found
}
