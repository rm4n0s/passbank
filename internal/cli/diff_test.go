package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// pairSecrets are the passwords of the merge fixtures' entries; none may ever
// reach the output of a command.
var pairSecrets = []string{
	"pw-personal", "pw-work-mail", "bank-password-a", "bank-password-b", "store-password",
	"stale-password", "old-account-password", "forum-password", modeltest.PairPasswordA, modeltest.PairPasswordB,
}

// diffArgs returns the arguments of `passbank diff` on the merge fixtures with
// the passwords from files in dir.
func diffArgs(t *testing.T, dir string, extra ...string) (args []string, a, b string) {
	t.Helper()
	a, b = mergeFixtures(t)
	pa := passwordFile(t, dir, "pw-a", modeltest.PairPasswordA)
	pb := passwordFile(t, dir, "pw-b", modeltest.PairPasswordB)
	args = append([]string{"diff", a, b, "--password-file-a", pa, "--password-file-b", pb}, extra...)
	return args, a, b
}

// normalize replaces the folder of the fixtures in text, which differs between
// machines, so that the golden files hold the same text everywhere.
func normalize(text, fixtures string) string {
	return strings.ReplaceAll(text, fixtures, "<fixtures>")
}

func fixturesDir(t *testing.T) string {
	t.Helper()
	a, _ := mergeFixtures(t)
	return filepath.Dir(a)
}

func TestDiffGolden(t *testing.T) {
	tests := []struct {
		golden string
		extra  []string
	}{
		{"diff-merge-newer", nil},
		{"diff-merge-prefer-a", []string{"--prefer", "a"}},
		{"diff-merge-prefer-b", []string{"--prefer", "b"}},
		{"diff-merge-union", []string{"--prefer", "union"}},
		{"diff-merge-no-titles", []string{"--no-titles"}},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			args, _, _ := diffArgs(t, t.TempDir(), tt.extra...)
			status, out, errs := run(t, &scriptedTerminal{}, args...)
			if status != ExitOK {
				t.Fatalf("status %d, stderr %q", status, errs)
			}
			checkGolden(t, tt.golden+".txt", out)
			checkGolden(t, tt.golden+".stderr.txt", normalize(errs, fixturesDir(t)))
			for _, s := range pairSecrets {
				if strings.Contains(out, s) || strings.Contains(errs, s) {
					t.Errorf("the output holds the secret %q", s)
				}
			}
		})
	}
}

// The plan that diff prints reads back for the same inputs, with the choices of
// the default.
func TestDiffPlanReadsBack(t *testing.T) {
	args, a, b := diffArgs(t, t.TempDir())
	status, out, errs := run(t, &scriptedTerminal{}, args...)
	if status != ExitOK {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	ctx := context.Background()
	dir := t.TempDir()
	open := func(path, password string) *input {
		opts := &credentialOptions{passwordFile: passwordFile(t, dir, filepath.Base(path)+".pw", password)}
		in, err := openInput(ctx, env{}, opts, path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(in.wipe)
		return in
	}
	ia, ib := open(a, modeltest.PairPasswordA), open(b, modeltest.PairPasswordB)
	changes := diff.Changes(ia.db, ib.db)
	got, err := plan.Read(strings.NewReader(out), ia.planInput(), ib.planInput(), changes)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := plan.Default(changes)
	for i := range want {
		if got.Choices[i] != want[i] {
			t.Errorf("change %d: %s, want %s", i, got.Choices[i], want[i])
		}
	}
	// One change of every kind.
	seen := map[diff.Kind]bool{}
	for _, c := range changes {
		seen[c.Kind] = true
	}
	for _, k := range diff.Kinds {
		if !seen[k] {
			t.Errorf("the merge fixtures have no change of kind %s", k)
		}
	}
}

func TestDiffPlanFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.txt")
	args, a, b := diffArgs(t, dir, "-o", planPath)

	status, out, errs := run(t, &scriptedTerminal{}, args...)
	if status != ExitOK || out != "" {
		t.Fatalf("status %d, stdout %q, stderr %q", status, out, errs)
	}
	if !strings.Contains(errs, "plan written to "+planPath) {
		t.Errorf("stderr = %q", errs)
	}
	info, err := os.Stat(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	file, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	// The same plan goes to stdout without -o.
	argsOut, _, _ := diffArgs(t, dir)
	_, stdout, _ := run(t, &scriptedTerminal{}, argsOut...)
	if string(file) != stdout {
		t.Errorf("the file and stdout differ:\n%s\n---\n%s", file, stdout)
	}

	// An existing plan file is not replaced.
	status, _, errs = run(t, &scriptedTerminal{}, args...)
	if status != ExitFailure || !strings.Contains(errs, "exists; use --force") {
		t.Errorf("status %d, stderr %q", status, errs)
	}
	if again, _ := os.ReadFile(planPath); !bytes.Equal(again, file) {
		t.Error("the existing plan file was changed")
	}
	// --force replaces it.
	if err := os.WriteFile(planPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, _, errs = run(t, &scriptedTerminal{}, append(args, "--force")...)
	if status != ExitOK {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	if again, _ := os.ReadFile(planPath); !bytes.Equal(again, file) {
		t.Errorf("--force did not write the plan: %q", again)
	}
	// Nothing else is left in the folder.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".passbank-plan-") {
			t.Errorf("a temporary file is left: %s", e.Name())
		}
	}
	// The databases are not changed (the plan path was never one of them).
	for _, p := range []string{a, b} {
		if _, err := os.Stat(p); err != nil {
			t.Error(err)
		}
	}
}

func TestDiffRefusesTheInputsAsOutput(t *testing.T) {
	for _, which := range []int{1, 2} {
		args, a, b := diffArgs(t, t.TempDir())
		before, err := os.ReadFile([]string{"", a, b}[which])
		if err != nil {
			t.Fatal(err)
		}
		status, _, errs := run(t, &scriptedTerminal{}, append(args, "-o", []string{"", a, b}[which], "--force")...)
		if status != ExitFailure || !strings.Contains(errs, "must not be one of the databases") {
			t.Errorf("status %d, stderr %q", status, errs)
		}
		after, err := os.ReadFile([]string{"", a, b}[which])
		if err != nil || !bytes.Equal(before, after) {
			t.Error("a database was changed")
		}
	}
	// Also through a link.
	dir := t.TempDir()
	args, a, _ := diffArgs(t, dir)
	link := filepath.Join(dir, "link.kdbx")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	status, _, errs := run(t, &scriptedTerminal{}, append(args, "-o", link, "--force")...)
	if status != ExitFailure || !strings.Contains(errs, "must not be one of the databases") {
		t.Errorf("through a link: status %d, stderr %q", status, errs)
	}
}

func TestDiffAsksForPasswords(t *testing.T) {
	a, b := mergeFixtures(t)
	term := &perPathTerminal{passwords: map[string]string{a: modeltest.PairPasswordA, b: modeltest.PairPasswordB}}
	status, out, errs := run(t, term, "diff", a, b)
	if status != ExitOK {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	if want := []string{"Password for " + a + ": ", "Password for " + b + ": "}; len(term.prompts) != 2 || term.prompts[0] != want[0] || term.prompts[1] != want[1] {
		t.Errorf("prompts = %q, want %q", term.prompts, want)
	}
	if !strings.HasPrefix(out, plan.FormatLine+"\n") {
		t.Errorf("stdout = %q", out)
	}
}

// perPathTerminal answers each prompt by the path it names.
type perPathTerminal struct {
	passwords map[string]string
	prompts   []string
}

func (p *perPathTerminal) IsTerminal() bool                { return true }
func (p *perPathTerminal) ReadLine(string) (string, error) { return "", io.EOF }
func (p *perPathTerminal) Write(b []byte) (int, error)     { return len(b), nil }

func (p *perPathTerminal) ReadPassword(prompt string) (secret.Bytes, error) {
	p.prompts = append(p.prompts, prompt)
	path := strings.TrimSuffix(strings.TrimPrefix(prompt, "Password for "), ": ")
	return secret.New([]byte(p.passwords[path])), nil
}

func TestDiffIdenticalDatabases(t *testing.T) {
	a, _ := mergeFixtures(t)
	dir := t.TempDir()
	pa := passwordFile(t, dir, "pw", modeltest.PairPasswordA)
	status, out, errs := run(t, &scriptedTerminal{}, "diff", a, a, "--password-file-a", pa, "--password-file-b", pa)
	if status != ExitOK {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	if !strings.Contains(errs, "no changes") {
		t.Errorf("stderr = %q", errs)
	}
	if strings.Count(out, "\n") != 3 {
		t.Errorf("a plan without changes is the three header lines:\n%s", out)
	}
}

func TestDiffErrors(t *testing.T) {
	dir := t.TempDir()
	a, b := mergeFixtures(t)
	pa := passwordFile(t, dir, "pw-a", modeltest.PairPasswordA)
	pb := passwordFile(t, dir, "pw-b", modeltest.PairPasswordB)
	wrong := passwordFile(t, dir, "wrong", "not the password")
	tests := []struct {
		name   string
		args   []string
		status int
		stderr string // a substring
		tag    string // with --debug
	}{
		{"no databases", []string{"diff"}, ExitUsage, "needs two databases", "UsageInvalid"},
		{"one database", []string{"diff", a}, ExitUsage, "needs two databases", "UsageInvalid"},
		{"three databases", []string{"diff", a, b, a}, ExitUsage, "needs two databases", "UsageInvalid"},
		{"a bad rule", []string{"diff", a, b, "--prefer", "oldest"}, ExitUsage, "--prefer must be", "UsageInvalid"},
		{"an unknown option", []string{"diff", a, b, "--frobnicate"}, ExitUsage, "frobnicate", "UsageInvalid"},
		{"contradicting options for A", []string{"diff", a, b, "--no-password-a", "--password-file-a", pa}, ExitUsage, "contradict", "UsageInvalid"},
		{"contradicting options for B", []string{"diff", a, b, "--no-password-b", "--password-file-b", pb}, ExitUsage, "contradict", "UsageInvalid"},
		{"the wrong password for A", []string{"diff", a, b, "--password-file-a", wrong, "--password-file-b", pb}, ExitFailure, "the password or key file for " + a + " is wrong", "WrongCredentials"},
		{"the wrong password for B", []string{"diff", a, b, "--password-file-a", pa, "--password-file-b", wrong}, ExitFailure, "the password or key file for " + b + " is wrong", "WrongCredentials"},
		{"a missing file", []string{"diff", a, filepath.Join(dir, "missing.kdbx"), "--password-file-a", pa, "--password-file-b", pb}, ExitFailure, "cannot be read", "FileReadFailed"},
		{"no terminal for the password", []string{"diff", a, b, "--password-file-a", pa}, ExitFailure, "no terminal", "PasswordNeedsTerminal"},
		{"a plan file in a missing folder", []string{"diff", a, b, "--password-file-a", pa, "--password-file-b", pb, "-o", filepath.Join(dir, "nope", "plan")}, ExitFailure, "cannot be written", "PlanFileWriteFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, out, errs := run(t, &scriptedTerminal{}, tt.args...)
			if status != tt.status || !strings.Contains(errs, tt.stderr) {
				t.Errorf("status %d (want %d), stderr %q (want %q)", status, tt.status, errs, tt.stderr)
			}
			if out != "" {
				t.Errorf("stdout = %q", out)
			}
			for _, s := range pairSecrets {
				if strings.Contains(errs, s) {
					t.Errorf("the error shows a secret %q", s)
				}
			}
			if tt.tag == "UsageInvalid" {
				return // a mistake in the arguments is reported before --debug is known
			}
			_, _, errs = run(t, &scriptedTerminal{}, append(tt.args, "--debug")...)
			if !strings.Contains(errs, "tag:   "+tt.tag) {
				t.Errorf("--debug does not show the tag %s:\n%s", tt.tag, errs)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(dir, "nope")); err == nil {
		t.Error("a folder was created")
	}
}
