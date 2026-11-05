package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
)

// The whole path of the command line: diff writes a plan, the plan is edited, merge
// reads it, and the file it writes opens.

func TestEndToEndEveryRule(t *testing.T) {
	for _, rule := range []string{"newer", "a", "b", "union"} {
		t.Run(rule, func(t *testing.T) {
			dir := t.TempDir()
			planPath := filepath.Join(dir, "plan.txt")
			args, _, _ := diffArgs(t, dir, "--prefer", rule, "-o", planPath)
			if status, _, errs := run(t, &scriptedTerminal{}, args...); status != ExitOK {
				t.Fatalf("diff: %d %q", status, errs)
			}
			viaPlan, viaRule := filepath.Join(dir, "plan.kdbx"), filepath.Join(dir, "rule.kdbx")
			if status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, viaPlan, "--plan", planPath)...); status != ExitOK {
				t.Fatalf("merge --plan: %d %q", status, errs)
			}
			if status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, viaRule, "--prefer", rule)...); status != ExitOK {
				t.Fatalf("merge --prefer: %d %q", status, errs)
			}
			p, _ := openResult(t, viaPlan, passwordCreds(modeltest.PairPasswordA))
			r, _ := openResult(t, viaRule, passwordCreds(modeltest.PairPasswordA))
			if listing(p) != listing(r) {
				t.Errorf("the plan and the rule differ:\n%s\n---\n%s", listing(p), listing(r))
			}
			// The model of the two databases read back is equal too (not only the listing),
			// except for what a write sets at random.
			if d := model.Diff(p, r); len(d) > 0 {
				t.Errorf("model.Diff: %v", d)
			}
		})
	}
}

// An interactive session and a plan file with the same choices give the same file.
func TestEndToEndInteractiveEqualsPlan(t *testing.T) {
	answers := []string{"d", "", "k", "t", "b", "k", "d", "a", "b", "y"}
	status, _, stderr, transcript, outInteractive := interactive(t, answers)
	if status != ExitOK {
		t.Fatalf("interactive: %d %q\n%s", status, stderr, transcript)
	}

	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.txt")
	args, _, _ := diffArgs(t, dir, "-o", planPath)
	if status, _, errs := run(t, &scriptedTerminal{}, args...); status != ExitOK {
		t.Fatalf("diff: %d %q", status, errs)
	}
	text, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := string(text)
	for _, e := range []struct{ from, to, comment string }{
		{"keep", "drop", "# Email / Work mail"},
		{"take-b", "both", "# Banking / My bank"},
		{"take-a", "take-b", "# Shopping / Store"},
		{"drop", "keep", "# Social / Old account"},
		{"keep", "drop", "# Social / Stale login"},
		{"take-b", "take-a", "# group: Shopping"},
	} {
		edited = replaceChoice(t, edited, e.from, e.to, e.comment)
	}
	edited = strings.Replace(edited, "settings take-a", "settings take-b", 1)
	if err := os.WriteFile(planPath, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	outPlan := filepath.Join(dir, "plan.kdbx")
	if status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, outPlan, "--plan", planPath)...); status != ExitOK {
		t.Fatalf("merge --plan: %d %q", status, errs)
	}
	i, _ := openResult(t, outInteractive, passwordCreds(modeltest.PairPasswordA))
	p, _ := openResult(t, outPlan, passwordCreds(modeltest.PairPasswordA))
	if d := model.Diff(i, p); len(d) > 0 {
		t.Errorf("the interactive result differs from the plan's: %v\n%s\n---\n%s", d, listing(i), listing(p))
	}
}

// Two databases of 2,000 entries each, written in different versions, through the
// whole path. The timed run with 10,000 entries is in large_test.go.
func TestEndToEndManyEntries(t *testing.T) {
	dir := t.TempDir()
	a, b := writeLarge(t, dir, 2000)
	pa := passwordFile(t, dir, "pw", modeltest.PairPasswordA)
	planPath := filepath.Join(dir, "plan.txt")
	status, _, errs := run(t, &scriptedTerminal{}, "diff", a, b, "--password-file-a", pa, "--password-file-b", pa, "-o", planPath)
	if status != ExitOK {
		t.Fatalf("diff: %d %q", status, errs)
	}
	for _, want := range []string{"only-b", "differs", "deleted-in-b", "settings"} {
		if !strings.Contains(errs, want) {
			t.Errorf("the summary lacks %q:\n%s", want, errs)
		}
	}
	out := filepath.Join(dir, "out.kdbx")
	status, stdout, errs := run(t, &scriptedTerminal{}, "merge", a, b, "-o", out, "--password-file-a", pa, "--password-file-b", pa, "--plan", planPath, "--encryption", "a")
	if status != ExitOK {
		t.Fatalf("merge: %d %q", status, errs)
	}
	db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
	entries := 0
	db.WalkEntries(func(_ *model.Group, _ *model.Entry) bool { entries++; return true })
	// 2000 in A, 100 only in B; of A's, 80 were deleted by B (the default keeps them
	// when A changed them later, or drops them: the deletion is newer than A's
	// version, so the default drops).
	if entries != 2000-80+100 {
		t.Errorf("%d entries, want %d\n%s", entries, 2000-80+100, stdout)
	}
}

// writeLarge writes the two databases of modeltest.LargePair(n): A as KDBX 4 and
// B as KDBX 3.1, with a cheap key derivation, both with the password of A.
func writeLarge(t testing.TB, dir string, n int) (a, b string) {
	t.Helper()
	da, db := modeltest.LargePair(n)
	write := func(name string, d *model.Database, v kdbx.Version) string {
		settings := kdbx.DefaultSettings(v)
		settings.KDF = crypto.KDF{ID: crypto.KDFAES, Rounds: 1000}
		data, err := kdbx.Encode(context.Background(), d, v, settings, passwordCreds(modeltest.PairPasswordA))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	return write("large-a.kdbx", da, kdbx.Version{Major: 4}), write("large-b.kdbx", db, kdbx.Version{Major: 3, Minor: 1})
}
