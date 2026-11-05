package cli

import (
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// mergeArgs returns the arguments of `passbank merge` on the merge fixtures with
// the passwords from files in dir, writing to out.
func mergeArgs(t *testing.T, dir, out string, extra ...string) []string {
	t.Helper()
	a, b := mergeFixtures(t)
	pa := passwordFile(t, dir, "pw-a", modeltest.PairPasswordA)
	pb := passwordFile(t, dir, "pw-b", modeltest.PairPasswordB)
	// The encryption of A (a cheap key derivation) keeps the tests fast; the tests of
	// --encryption say otherwise.
	return append([]string{"merge", a, b, "-o", out, "--password-file-a", pa, "--password-file-b", pb, "--encryption", "a"}, extra...)
}

// openResult opens a file the CLI wrote.
func openResult(t *testing.T, path string, creds crypto.Credentials) (*model.Database, kdbx.Info) {
	t.Helper()
	db, info, err := kdbx.Open(context.Background(), path, creds)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	t.Cleanup(db.Wipe)
	return db, info
}

func passwordCreds(password string) crypto.Credentials {
	return crypto.Credentials{HasPassword: true, Password: secret.New([]byte(password))}
}

// listing writes the groups and entries of a database, one per line, with the
// password of each entry (the test values of the fixtures), sorted by path.
func listing(db *model.Database) string {
	var lines []string
	var walk func(g *model.Group, path string)
	walk = func(g *model.Group, path string) {
		p := path
		if g != db.Root {
			p = path + g.Name + "/"
			lines = append(lines, "group "+p+"  notes="+g.Notes)
		}
		for _, e := range g.Entries {
			pw, _ := e.Field(model.FieldPassword)
			url, _ := e.Field(model.FieldURL)
			lines = append(lines, "entry "+p+e.Title()+"  password="+pw.Text()+" url="+url.Text()+" history="+itoa(len(e.History)))
		}
		for _, c := range g.Groups {
			walk(c, p)
		}
	}
	walk(db.Root, "")
	sort.Strings(lines)
	return "name=" + db.Meta.DatabaseName + "\n" + strings.Join(lines, "\n") + "\n"
}

func itoa(n int) string { return string(rune('0' + n)) }

// sumOf hashes a file.
func sumOf(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func TestMergeGolden(t *testing.T) {
	tests := []struct {
		golden string
		extra  []string
	}{
		{"merge-newer", []string{"--prefer", "newer"}},
		{"merge-prefer-a", []string{"--prefer", "a"}},
		{"merge-prefer-b", []string{"--prefer", "b"}},
		{"merge-union", []string{"--prefer", "union"}},
		{"merge-newer-settings-b", []string{"--prefer", "newer", "--settings-from", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "result.kdbx")
			a, b := mergeFixtures(t)
			beforeA, beforeB := sumOf(t, a), sumOf(t, b)
			status, stdout, stderr := run(t, &scriptedTerminal{}, mergeArgs(t, dir, out, tt.extra...)...)
			if status != ExitOK || stderr != "" {
				t.Fatalf("status %d, stderr %q", status, stderr)
			}
			text := strings.ReplaceAll(normalize(stdout, fixturesDir(t)), out, "<out>")
			checkGolden(t, tt.golden+".txt", text)
			db, info := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
			if info.Version != (kdbx.Version{Major: 4, Minor: 0}) {
				t.Errorf("version = %v, want 4.0", info.Version)
			}
			checkGolden(t, tt.golden+".entries.txt", listing(db))
			// The inputs are never modified.
			if sumOf(t, a) != beforeA || sumOf(t, b) != beforeB {
				t.Error("an input was modified")
			}
			for _, s := range pairSecrets {
				if strings.Contains(stdout, s) || strings.Contains(stderr, s) {
					t.Errorf("the output of the command holds the secret %q", s)
				}
			}
			// Only the result is in the folder (no temporary file).
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if strings.Contains(e.Name(), ".tmp") {
					t.Errorf("a temporary file is left: %s", e.Name())
				}
			}
		})
	}
}

// diff, then an edited plan, then merge: the plan decides.
func TestMergeWithAPlanFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.txt")
	status, _, errs := run(t, &scriptedTerminal{}, diffArgsTo(t, dir, planPath)...)
	if status != ExitOK {
		t.Fatalf("diff: status %d, stderr %q", status, errs)
	}
	text, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	// Unedited, the plan gives what --prefer newer gives.
	outPlan, outRule := filepath.Join(dir, "plan.kdbx"), filepath.Join(dir, "rule.kdbx")
	if status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, outPlan, "--plan", planPath)...); status != ExitOK {
		t.Fatalf("merge --plan: status %d, stderr %q", status, errs)
	}
	if status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, outRule, "--prefer", "newer")...); status != ExitOK {
		t.Fatalf("merge --prefer: status %d, stderr %q", status, errs)
	}
	dbPlan, _ := openResult(t, outPlan, passwordCreds(modeltest.PairPasswordA))
	dbRule, _ := openResult(t, outRule, passwordCreds(modeltest.PairPasswordA))
	if listing(dbPlan) != listing(dbRule) {
		t.Errorf("the plan and the rule give different results:\n%s\n---\n%s", listing(dbPlan), listing(dbRule))
	}

	// Edited by hand: keep both versions of "My bank", drop "Work mail", and take B's settings.
	edited := string(text)
	edited = replaceChoice(t, edited, "take-b", "both", "# Banking / My bank")
	edited = replaceChoice(t, edited, "keep", "drop", "# Email / Work mail")
	edited = strings.Replace(edited, "settings take-a", "settings take-b", 1)
	editedPath := filepath.Join(dir, "edited.txt")
	if err := os.WriteFile(editedPath, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	outEdited := filepath.Join(dir, "edited.kdbx")
	status, stdout, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, outEdited, "--plan", editedPath)...)
	if status != ExitOK {
		t.Fatalf("merge --plan edited: status %d, stderr %q", status, errs)
	}
	db, _ := openResult(t, outEdited, passwordCreds(modeltest.PairPasswordA))
	got := listing(db)
	for _, want := range []string{"name=Laptop", "entry Banking/My bank  password=bank-password-a", "entry Banking/My bank  password=bank-password-b"} {
		if !strings.Contains(got, want) {
			t.Errorf("the result lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Work mail") {
		t.Errorf("the dropped entry is in the result:\n%s", got)
	}
	if !strings.Contains(stdout, "both 1") || !strings.Contains(stdout, "drop 1") {
		t.Errorf("the summary does not count the edits:\n%s", stdout)
	}
}

// replaceChoice changes the choice on the line of the plan that ends with the comment.
func replaceChoice(t *testing.T, text, from, to, comment string) string {
	t.Helper()
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.HasSuffix(l, comment) && strings.HasPrefix(l, from+" ") {
			lines[i] = to + l[len(from):]
			return strings.Join(lines, "\n")
		}
	}
	t.Fatalf("no line %q with choice %q in the plan:\n%s", comment, from, text)
	return ""
}

// diffArgsTo is diffArgs writing the plan to a file.
func diffArgsTo(t *testing.T, dir, planPath string) []string {
	args, _, _ := diffArgs(t, dir, "-o", planPath)
	return args
}

func TestMergeFormats(t *testing.T) {
	tests := []struct {
		format string
		want   kdbx.Version
	}{
		{"4", kdbx.Version{Major: 4, Minor: 0}},
		{"4.0", kdbx.Version{Major: 4, Minor: 0}},
		{"4.1", kdbx.Version{Major: 4, Minor: 1}},
		{"3.1", kdbx.Version{Major: 3, Minor: 1}},
		{"3.0", kdbx.Version{Major: 3, Minor: 0}},
		{"2", kdbx.Version{Major: 2, Minor: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "out.kdbx")
			status, stdout, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, out, "--prefer", "newer", "--format", tt.format)...)
			if status != ExitOK {
				t.Fatalf("status %d, stderr %q", status, errs)
			}
			db, info := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
			if info.Version != tt.want {
				t.Errorf("version = %v, want %v", info.Version, tt.want)
			}
			if !strings.Contains(stdout, "written: "+out+" (KDBX "+tt.want.String()+")") {
				t.Errorf("summary = %q", stdout)
			}
			if got := listing(db); !strings.Contains(got, "entry Banking/My bank  password=bank-password-b") {
				t.Errorf("the result is wrong:\n%s", got)
			}
		})
	}
}

// writeWith41 writes the fixture A with a group tag, which only KDBX 4.1 stores.
func writeWith41(t *testing.T, dir string) string {
	t.Helper()
	a, _ := modeltest.MergePair()
	a.Root.Groups[0].Tags = []string{"tagged"}
	path := filepath.Join(dir, "a41.kdbx")
	settings := kdbx.DefaultSettings(kdbx.Version{Major: 4, Minor: 1})
	settings.KDF = crypto.KDF{ID: crypto.KDFAES, Rounds: 1000}
	data, err := kdbx.Encode(context.Background(), a, kdbx.Version{Major: 4, Minor: 1}, settings, passwordCreds(modeltest.PairPasswordA))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMergeLosses(t *testing.T) {
	dir := t.TempDir()
	a41 := writeWith41(t, dir)
	_, b := mergeFixtures(t)
	pa := passwordFile(t, dir, "pw-a", modeltest.PairPasswordA)
	pb := passwordFile(t, dir, "pw-b", modeltest.PairPasswordB)
	base := func(out string, extra ...string) []string {
		return append([]string{"merge", a41, b, "-o", out, "--password-file-a", pa, "--password-file-b", pb, "--prefer", "a"}, extra...)
	}

	t.Run("4 becomes 4.1 when the database needs it", func(t *testing.T) {
		out := filepath.Join(dir, "auto.kdbx")
		status, _, errs := run(t, &scriptedTerminal{}, base(out)...)
		if status != ExitOK {
			t.Fatalf("status %d, stderr %q", status, errs)
		}
		if _, info := openResult(t, out, passwordCreds(modeltest.PairPasswordA)); info.Version != (kdbx.Version{Major: 4, Minor: 1}) {
			t.Errorf("version = %v, want 4.1", info.Version)
		}
	})
	t.Run("a lower version refuses to lose data", func(t *testing.T) {
		out := filepath.Join(dir, "refused.kdbx")
		status, stdout, errs := run(t, &scriptedTerminal{}, base(out, "--format", "3.1")...)
		if status != ExitFailure || stdout != "" {
			t.Errorf("status %d, stdout %q", status, stdout)
		}
		if !strings.Contains(errs, "KDBX 3.1 cannot store everything") || !strings.Contains(errs, "GroupTags") || !strings.Contains(errs, "--allow-loss") {
			t.Errorf("stderr = %q", errs)
		}
		if _, err := os.Stat(out); err == nil {
			t.Error("a file was written")
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.Contains(e.Name(), ".tmp") {
				t.Errorf("a temporary file is left: %s", e.Name())
			}
		}
	})
	t.Run("--allow-loss writes it and lists the loss", func(t *testing.T) {
		out := filepath.Join(dir, "allowed.kdbx")
		status, stdout, errs := run(t, &scriptedTerminal{}, base(out, "--format", "3.1", "--allow-loss")...)
		if status != ExitOK {
			t.Fatalf("status %d, stderr %q", status, errs)
		}
		if !strings.Contains(stdout, "accepted losses in KDBX 3.1: 2") || !strings.Contains(stdout, "GroupTags") {
			t.Errorf("stdout = %q", stdout)
		}
		if _, info := openResult(t, out, passwordCreds(modeltest.PairPasswordA)); info.Version != (kdbx.Version{Major: 3, Minor: 1}) {
			t.Errorf("version = %v", info.Version)
		}
	})
}

func TestMergeCredentials(t *testing.T) {
	dir := t.TempDir()
	keyFile := kdbxFixture(t, "keyfile-raw32.key")
	newPassword := passwordFile(t, dir, "pw-new", "brand new password")

	tests := []struct {
		name  string
		extra []string
		term  Terminal
		open  func() crypto.Credentials
		fail  string // the tag of the failure; "" for success
	}{
		{"A's credentials by default", nil, &scriptedTerminal{}, func() crypto.Credentials { return passwordCreds(modeltest.PairPasswordA) }, ""},
		{"B's credentials", []string{"--credentials", "b"}, &scriptedTerminal{}, func() crypto.Credentials { return passwordCreds(modeltest.PairPasswordB) }, ""},
		{"a new password from a file", []string{"--credentials", "new", "--password-file-out", newPassword}, &scriptedTerminal{}, func() crypto.Credentials { return passwordCreds("brand new password") }, ""},
		{"a new password asked twice", []string{"--credentials", "new"}, &scriptedTerminal{tty: true, password: "typed twice"}, func() crypto.Credentials { return passwordCreds("typed twice") }, ""},
		{"a new key file and no password", []string{"--credentials", "new", "--keyfile-out", keyFile, "--no-password-out"}, &scriptedTerminal{}, func() crypto.Credentials {
			key, err := crypto.ReadKeyFile(context.Background(), keyFile)
			if err != nil {
				t.Fatal(err)
			}
			return crypto.Credentials{HasKeyFile: true, KeyFile: key}
		}, ""},
		{"a new password and a key file", []string{"--credentials", "new", "--keyfile-out", keyFile, "--password-file-out", newPassword}, &scriptedTerminal{}, func() crypto.Credentials {
			key, err := crypto.ReadKeyFile(context.Background(), keyFile)
			if err != nil {
				t.Fatal(err)
			}
			c := passwordCreds("brand new password")
			c.HasKeyFile, c.KeyFile = true, key
			return c
		}, ""},
		{"two different passwords", []string{"--credentials", "new"}, &differingTerminal{}, nil, "PasswordsDoNotMatch"},
		{"a new password needs a terminal", []string{"--credentials", "new"}, &scriptedTerminal{}, nil, "PasswordNeedsTerminal"},
		{"no password and no key file", []string{"--credentials", "new", "--no-password-out"}, &scriptedTerminal{}, nil, "NoCredentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.kdbx")
			args := append(mergeArgs(t, dir, out, "--prefer", "newer", "--debug"), tt.extra...)
			status, _, errs := run(t, tt.term, args...)
			if tt.fail != "" {
				if status != ExitFailure || !strings.Contains(errs, "tag:   "+tt.fail) {
					t.Errorf("status %d, stderr %q (want tag %s)", status, errs, tt.fail)
				}
				if _, err := os.Stat(out); err == nil {
					t.Error("a file was written")
				}
				return
			}
			if status != ExitOK {
				t.Fatalf("status %d, stderr %q", status, errs)
			}
			openResult(t, out, tt.open())
			// The wrong credentials do not open it.
			if _, _, err := kdbx.Open(context.Background(), out, passwordCreds("wrong")); err == nil {
				t.Error("the result opens with a wrong password")
			}
		})
	}
}

// differingTerminal answers the first prompt with one password and the second
// with another.
type differingTerminal struct{ n int }

func (d *differingTerminal) IsTerminal() bool                { return true }
func (d *differingTerminal) ReadLine(string) (string, error) { return "", io.EOF }
func (d *differingTerminal) Write(b []byte) (int, error)     { return len(b), nil }
func (d *differingTerminal) ReadPassword(string) (secret.Bytes, error) {
	d.n++
	return secret.New([]byte([]string{"", "first", "second"}[d.n])), nil
}

func TestMergeEncryption(t *testing.T) {
	dir := t.TempDir()
	a, b := mergeFixtures(t)
	_, infoA, err := kdbx.Open(context.Background(), a, passwordCreds(modeltest.PairPasswordA))
	if err != nil {
		t.Fatal(err)
	}
	_, infoB, err := kdbx.Open(context.Background(), b, passwordCreds(modeltest.PairPasswordB))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		extra      []string
		wantRounds uint64
		wantKDF    string
		fail       string
	}{
		{"A's", []string{"--encryption", "a"}, infoA.KDF.Rounds, infoA.KDF.Name(), ""},
		{"B's", []string{"--encryption", "b", "--format", "3.1"}, infoB.KDF.Rounds, infoB.KDF.Name(), ""},
		{"the defaults of KDBX 4", []string{"--encryption", "default"}, 0, "Argon2id", ""},
		{"the defaults of KDBX 3.1", []string{"--encryption", "default", "--format", "3.1"}, 2_000_000, "AES-KDF", ""},
		{"the encryption of A in a version that can hold it", []string{"--encryption", "a", "--format", "2"}, infoA.KDF.Rounds, infoA.KDF.Name(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.kdbx")
			status, _, errs := run(t, &scriptedTerminal{}, append(mergeArgs(t, dir, out, "--prefer", "newer"), tt.extra...)...)
			if status != ExitOK {
				t.Fatalf("status %d, stderr %q", status, errs)
			}
			_, info := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
			if info.KDF.Name() != tt.wantKDF || (tt.wantRounds != 0 && info.KDF.Rounds != tt.wantRounds) {
				t.Errorf("KDF = %s with %d rounds, want %s with %d", info.KDF.Name(), info.KDF.Rounds, tt.wantKDF, tt.wantRounds)
			}
		})
	}
}

func TestMergeEncryptionNotInVersion(t *testing.T) {
	// A KDBX 4 file with Argon2 as the input A, merged with A's encryption into KDBX 3.1.
	dir := t.TempDir()
	a, _ := modeltest.MergePair()
	argon := kdbx.DefaultSettings(kdbx.Version{Major: 4, Minor: 0})
	argon.KDF.Memory, argon.KDF.Iterations = 1<<20, 1
	data, err := kdbx.Encode(context.Background(), a, kdbx.Version{Major: 4, Minor: 0}, argon, passwordCreds(modeltest.PairPasswordA))
	if err != nil {
		t.Fatal(err)
	}
	argonPath := filepath.Join(dir, "argon.kdbx")
	if err := os.WriteFile(argonPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, b := mergeFixtures(t)
	pa := passwordFile(t, dir, "pw-a", modeltest.PairPasswordA)
	pb := passwordFile(t, dir, "pw-b", modeltest.PairPasswordB)
	out := filepath.Join(dir, "out.kdbx")
	status, _, errs := run(t, &scriptedTerminal{}, "merge", argonPath, b, "-o", out, "--password-file-a", pa, "--password-file-b", pb,
		"--prefer", "a", "--encryption", "a", "--format", "3.1", "--debug")
	if status != ExitFailure || !strings.Contains(errs, "KDBX 3.1 has only the AES-KDF") || !strings.Contains(errs, "tag:   EncryptionNotInVersion") {
		t.Errorf("status %d, stderr %q", status, errs)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a file was written")
	}
}

func TestMergeOutputRules(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.kdbx")
	args := mergeArgs(t, dir, out, "--prefer", "newer")

	if status, _, errs := run(t, &scriptedTerminal{}, args...); status != ExitOK {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	first := sumOf(t, out)

	// An existing file is never replaced without --force.
	status, stdout, errs := run(t, &scriptedTerminal{}, args...)
	if status != ExitFailure || stdout != "" || !strings.Contains(errs, out+" exists; use --force to replace it") {
		t.Errorf("status %d, stdout %q, stderr %q", status, stdout, errs)
	}
	if sumOf(t, out) != first {
		t.Error("the existing file was changed")
	}
	// With --force it is replaced.
	if status, _, errs := run(t, &scriptedTerminal{}, append(args, "--force")...); status != ExitOK {
		t.Fatalf("--force: status %d, stderr %q", status, errs)
	}
	if sumOf(t, out) == first {
		t.Error("--force did not write a new file (the salts must differ)")
	}
	// An input is never the output, also with --force.
	a, b := mergeFixtures(t)
	for _, in := range []string{a, b} {
		before := sumOf(t, in)
		bad := mergeArgs(t, dir, in, "--prefer", "newer", "--force")
		status, _, errs := run(t, &scriptedTerminal{}, bad...)
		if status != ExitFailure || !strings.Contains(errs, "must be a new file, not one of the inputs") {
			t.Errorf("status %d, stderr %q", status, errs)
		}
		if sumOf(t, in) != before {
			t.Errorf("%s was modified", in)
		}
	}
	// A missing folder.
	missing := filepath.Join(dir, "no", "such", "out.kdbx")
	status, _, errs = run(t, &scriptedTerminal{}, mergeArgs(t, dir, missing, "--prefer", "newer")...)
	if status != ExitFailure || !strings.Contains(errs, "cannot be written") {
		t.Errorf("status %d, stderr %q", status, errs)
	}
}

func TestMergePlanErrors(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.txt")
	if status, _, errs := run(t, &scriptedTerminal{}, diffArgsTo(t, dir, planPath)...); status != ExitOK {
		t.Fatalf("diff: %d %q", status, errs)
	}
	good, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(good), "\n")
	firstChoice := func() int {
		for i, l := range lines {
			if strings.HasPrefix(l, "keep ") || strings.HasPrefix(l, "take-") || strings.HasPrefix(l, "drop ") || strings.HasPrefix(l, "both ") {
				return i
			}
		}
		t.Fatal("no change line")
		return 0
	}()
	edit := func(f func(ls []string) []string) string {
		cp := append([]string(nil), lines...)
		return strings.Join(f(cp), "\n")
	}
	tests := []struct {
		name string
		text string
		tag  string
		msg  string
	}{
		{"a stale plan", edit(func(ls []string) []string { ls[1] = "a sha256:" + strings.Repeat("0", 64) + " a.kdbx"; return ls }), "PlanStale", "input A has changed"},
		{"a missing change", edit(func(ls []string) []string { return append(ls[:firstChoice:firstChoice], ls[firstChoice+1:]...) }), "PlanIncomplete", "no choice for 1 changes"},
		{"an unknown item", edit(func(ls []string) []string { return append(ls, "keep abcd0000-0000-0000-0000-0000000000aa") }), "PlanUnknownItem", "is not a change between these databases"},
		{"a choice the kind does not allow", edit(func(ls []string) []string {
			ls[firstChoice] = "take-a" + ls[firstChoice][strings.Index(ls[firstChoice], " "):]
			return ls
		}), "PlanInvalidChoice", "not allowed"},
		{"a duplicate", edit(func(ls []string) []string { return append(ls, ls[firstChoice]) }), "PlanDuplicateItem", "more than one line"},
		{"a syntax error", edit(func(ls []string) []string { return append(ls, "what is this") }), "PlanSyntax", "line "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := filepath.Join(dir, "bad.txt")
			if err := os.WriteFile(bad, []byte(tt.text), 0o600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "never.kdbx")
			status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, out, "--plan", bad, "--debug")...)
			if status != ExitFailure || !strings.Contains(errs, "tag:   "+tt.tag) || !strings.Contains(errs, tt.msg) {
				t.Errorf("status %d, stderr %q (want tag %s and %q)", status, errs, tt.tag, tt.msg)
			}
			if _, err := os.Stat(out); err == nil {
				t.Error("a file was written")
			}
		})
	}
	t.Run("a missing plan file", func(t *testing.T) {
		out := filepath.Join(dir, "never.kdbx")
		status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, out, "--plan", filepath.Join(dir, "missing.txt"), "--debug")...)
		if status != ExitFailure || !strings.Contains(errs, "tag:   PlanFileReadFailed") || !strings.Contains(errs, "cannot be read") {
			t.Errorf("status %d, stderr %q", status, errs)
		}
	})
}

func TestMergeUsageErrors(t *testing.T) {
	dir := t.TempDir()
	a, b := mergeFixtures(t)
	pa := passwordFile(t, dir, "pw-a", modeltest.PairPasswordA)
	out := filepath.Join(dir, "out.kdbx")
	tests := []struct {
		name string
		args []string
		msg  string
	}{
		{"no databases", []string{"merge"}, "needs two databases"},
		{"one database", []string{"merge", a, "-o", out, "--prefer", "a"}, "needs two databases"},
		{"no output", []string{"merge", a, b, "--prefer", "a"}, "-o OUT"},
		{"both plan and rule", []string{"merge", a, b, "-o", out, "--plan", "x", "--prefer", "a"}, "contradict"},
		{"a bad rule", []string{"merge", a, b, "-o", out, "--prefer", "oldest"}, "--prefer must be"},
		{"a bad format", []string{"merge", a, b, "-o", out, "--prefer", "a", "--format", "5"}, "--format must be"},
		{"a bad settings side", []string{"merge", a, b, "-o", out, "--prefer", "a", "--settings-from", "c"}, "--settings-from must be"},
		{"bad credentials", []string{"merge", a, b, "-o", out, "--prefer", "a", "--credentials", "c"}, "--credentials must be"},
		{"bad encryption", []string{"merge", a, b, "-o", out, "--prefer", "a", "--encryption", "c"}, "--encryption must be"},
		{"out options without new credentials", []string{"merge", a, b, "-o", out, "--prefer", "a", "--password-file-out", pa}, "need --credentials new"},
		{"contradicting options", []string{"merge", a, b, "-o", out, "--prefer", "a", "--no-password-a", "--password-file-a", pa}, "contradict"},
		{"an unknown option", []string{"merge", a, b, "-o", out, "--prefer", "a", "--frobnicate"}, "frobnicate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, stdout, errs := run(t, &scriptedTerminal{}, tt.args...)
			if status != ExitUsage || stdout != "" || !strings.Contains(errs, tt.msg) || !strings.Contains(errs, "usage: passbank merge") {
				t.Errorf("status %d, stdout %q, stderr %q (want %q)", status, stdout, errs, tt.msg)
			}
		})
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a file was written")
	}
}

func TestMergeWrongPasswords(t *testing.T) {
	dir := t.TempDir()
	a, b := mergeFixtures(t)
	pa := passwordFile(t, dir, "pw-a", modeltest.PairPasswordA)
	pb := passwordFile(t, dir, "pw-b", modeltest.PairPasswordB)
	wrong := passwordFile(t, dir, "wrong", "nope")
	out := filepath.Join(dir, "out.kdbx")
	for _, tt := range []struct {
		name         string
		pwA, pwB, in string
	}{
		{"A", wrong, pb, a},
		{"B", pa, wrong, b},
	} {
		status, _, errs := run(t, &scriptedTerminal{}, "merge", a, b, "-o", out, "--password-file-a", tt.pwA, "--password-file-b", tt.pwB, "--prefer", "a")
		if status != ExitFailure || !strings.Contains(errs, "the password or key file for "+tt.in+" is wrong") {
			t.Errorf("%s: status %d, stderr %q", tt.name, status, errs)
		}
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a file was written")
	}
}

func TestMergeGroupDropConflict(t *testing.T) {
	// A plan that drops the group Games but keeps its entry is refused.
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.txt")
	if status, _, errs := run(t, &scriptedTerminal{}, diffArgsTo(t, dir, planPath)...); status != ExitOK {
		t.Fatalf("diff: %d %q", status, errs)
	}
	text, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := replaceChoice(t, string(text), "keep", "drop", "# group: Games")
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.kdbx")
	status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, out, "--plan", bad, "--debug")...)
	if status != ExitFailure || !strings.Contains(errs, "tag:   PlanGroupNotEmpty") || !strings.Contains(errs, `the group "Games" is dropped, but "Old forum" in it is kept`) {
		t.Errorf("status %d, stderr %q", status, errs)
	}
	// Dropping the entry too makes the plan valid, and Games is gone.
	edited = replaceChoice(t, edited, "keep", "drop", "# Games / Old forum")
	if err := os.WriteFile(bad, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	status, _, errs = run(t, &scriptedTerminal{}, mergeArgs(t, dir, out, "--plan", bad)...)
	if status != ExitOK {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
	if got := listing(db); strings.Contains(got, "Games") || strings.Contains(got, "Old forum") {
		t.Errorf("Games is in the result:\n%s", got)
	}
}

func TestMergeRoutes(t *testing.T) {
	// Differing passwords: the route.
	c := &credentialOptions{confirm: true}
	_, err := c.load(context.Background(), env{term: &differingTerminal{}}, "out")
	appErr, ok := errors.FromError(err)
	if !ok || !appErr.HasRoute("readNewPassword.PasswordsDoNotMatch") {
		t.Errorf("err = %v", err)
	}
}
