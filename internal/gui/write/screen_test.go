package write

import (
	"context"
	"crypto/sha256"
	"github.com/rm4n0s/errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/session"
	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

// The merge fixtures of the CLI: A is a KDBX 4.0 file, B a KDBX 3.1 file.
func mergeFixture(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "cli", "testdata", "merge", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// newSession unlocks the two fixtures and makes the plan with the defaults, as the
// Review screen leaves it.
func newSession(t *testing.T, edit ...func(a, b *session.Source)) *session.Session {
	t.Helper()
	sess := session.New()
	t.Cleanup(sess.Wipe)
	a, err := session.Unlock(context.Background(), mergeFixture(t, "a.kdbx"), secret.New([]byte(modeltest.PairPasswordA)), "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := session.Unlock(context.Background(), mergeFixture(t, "b.kdbx"), secret.New([]byte(modeltest.PairPasswordB)), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range edit {
		f(a, b) // before the plan, which is made for the databases as they are
	}
	sess.SetA(a)
	sess.SetB(b)
	sess.Plan = plan.New(a.PlanInput(), b.PlanInput(), diff.Changes(a.DB, b.DB))
	sess.Decided = make([]bool, len(sess.Plan.Changes))
	return sess
}

type harness struct {
	t    *testing.T
	ts   *gluttertest.Tester
	fd   *gluttertest.FakeDesktop
	sess *session.Session
	back int
}

func newHarness(t *testing.T, theme kit.Theme, sess *session.Session) *harness {
	t.Helper()
	h := &harness{t: t, sess: sess}
	h.ts = testutil.New(t)
	h.fd = h.ts.FakeDesktop()
	h.ts.SetViewSize(ui.Size{Width: 1000, Height: 1500})
	h.show(theme)
	return h
}

func (h *harness) show(theme kit.Theme) {
	h.t.Helper()
	h.ts.PumpWidget(widgets.NewDirectionality(ui.TextDirectionLTR, kit.NewRoot(theme, NewScreen(h.sess, func() { h.back++ }))))
	h.ts.PumpAndSettle()
	h.waitFor("the result", func() bool { return h.state().result() != nil })
}

func (h *harness) state() *screenState {
	return gluttertest.StateOf[*screenState](h.ts, gluttertest.ByType[*Screen]())
}

func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	h.fd.WaitFor(what, func() bool {
		h.ts.PumpAndSettle()
		return cond()
	})
}

func (h *harness) button(label string) *kit.Button {
	h.t.Helper()
	for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Button]()) {
		if b := e.Widget().(*kit.Button); b.Label() == label {
			return b
		}
	}
	h.t.Fatalf("no button %q", label)
	return nil
}

func (h *harness) press(label string) {
	h.t.Helper()
	h.ts.Tap(gluttertest.ByWidget(h.button(label)))
	h.ts.PumpAndSettle()
}

// choose presses the segment of the control (a kit.Segmented named group).
func (h *harness) choose(group, label string) {
	h.t.Helper()
	for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Segmented]()) {
		if e.Widget().(*kit.Segmented).Label() == group {
			h.ts.Tap(gluttertest.Descendant(gluttertest.ByWidget(e.Widget()), gluttertest.ByText(label)))
			h.ts.PumpAndSettle()
			return
		}
	}
	h.t.Fatalf("no control %q", group)
}

func (h *harness) setPath(path string) {
	h.t.Helper()
	h.state().path.SetText(path)
	h.ts.PumpAndSettle()
}

func (h *harness) written() string {
	h.t.Helper()
	for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Banner]()) {
		if m := e.Widget().(*kit.Banner).Message(); strings.HasPrefix(m, "Written and verified") {
			return m
		}
	}
	return ""
}

func (h *harness) banner(prefix string) string {
	h.t.Helper()
	for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Banner]()) {
		if m := e.Widget().(*kit.Banner).Message(); strings.HasPrefix(m, prefix) {
			return m
		}
	}
	return ""
}

// waitWritten waits until the last write has ended.
func (h *harness) waitWritten() {
	h.t.Helper()
	h.waitFor("the write to end", func() bool { return !h.state().writing && h.state().outcome != nil })
}

func sum(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func open(t *testing.T, path string, password string) (*model.Database, kdbx.Info) {
	t.Helper()
	db, info, err := kdbx.Open(context.Background(), path, crypto.Credentials{HasPassword: true, Password: secret.New([]byte(password))})
	if err != nil {
		t.Fatalf("%s does not open: %v", path, err)
	}
	t.Cleanup(db.Wipe)
	return db, info
}

func titles(db *model.Database) []string {
	var out []string
	db.WalkGroups(func(g *model.Group) bool {
		for _, e := range g.Entries {
			out = append(out, e.Title())
		}
		return true
	})
	return out
}

func TestScreenShowsTheSummaryAndStartsDisabled(t *testing.T) {
	h := newHarness(t, kit.LightTheme(), newSession(t))
	for _, want := range []string{"Write the new database", "What the new database will contain", "9 changes between A and B:"} {
		if !h.ts.Exists(gluttertest.ByText(want)) {
			t.Errorf("the screen does not show %q", want)
		}
	}
	if h.button("Write").Enabled() {
		t.Error("Write is enabled without a path")
	}
	r := h.state().result()
	if r.err != nil || r.entries == 0 || r.groups == 0 {
		t.Fatalf("result = %+v", r)
	}
	// The default is KDBX 4, and the credentials and encryption are those of A.
	seg := func(group string) string {
		for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Segmented]()) {
			if s := e.Widget().(*kit.Segmented); s.Label() == group {
				return s.Selected()
			}
		}
		return "?"
	}
	if seg("Version") != "4" || seg("Credentials") != SourceA || seg("Encryption") != EncryptionDefault {
		t.Errorf("version %q, credentials %q, encryption %q", seg("Version"), seg("Credentials"), seg("Encryption"))
	}
}

func TestWriteDefaults(t *testing.T) {
	sess := newSession(t)
	h := newHarness(t, kit.LightTheme(), sess)
	hashA, hashB := sum(t, sess.A.Path), sum(t, sess.B.Path)
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	h.setPath(out)
	h.press("Write")
	h.waitWritten()
	if want := "Written and verified: merged.kdbx (KDBX 4.0)."; h.written() != want {
		t.Fatalf("banner %q, want %q (outcome %+v)", h.written(), want, h.state().outcome)
	}
	// The file opens with A's password, as KDBX 4.0, and holds what the plan says.
	db, info := open(t, out, modeltest.PairPasswordA)
	if info.Version != (kdbx.Version{Major: 4, Minor: 0}) {
		t.Errorf("version %v", info.Version)
	}
	got := strings.Join(titles(db), "|")
	for _, want := range []string{"Work mail", "Old forum", "My bank"} {
		if !strings.Contains(got, want) {
			t.Errorf("the result lacks %q: %s", want, got)
		}
	}
	if sum(t, sess.A.Path) != hashA || sum(t, sess.B.Path) != hashB {
		t.Error("an input was changed")
	}
	// The session remembers the path for the way back.
	if sess.Output.Path != out {
		t.Errorf("remembered path %q", sess.Output.Path)
	}
}

func TestWriteEveryVersion(t *testing.T) {
	tests := []struct {
		label   string
		version kdbx.Version
	}{
		{"KDBX 4", kdbx.Version{Major: 4, Minor: 0}},
		{"4.1", kdbx.Version{Major: 4, Minor: 1}},
		{"4.0", kdbx.Version{Major: 4, Minor: 0}},
		{"3.1", kdbx.Version{Major: 3, Minor: 1}},
		{"3.0", kdbx.Version{Major: 3, Minor: 0}},
		{"2.0", kdbx.Version{Major: 2, Minor: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			h := newHarness(t, kit.LightTheme(), newSession(t))
			out := filepath.Join(t.TempDir(), "merged.kdbx")
			h.setPath(out)
			h.choose("Version", tt.label)
			h.choose("Encryption", "Defaults of the version")
			h.press("Write")
			h.waitWritten()
			if o := h.state().outcome; o.err != nil {
				t.Fatalf("write failed: %v", o.err)
			}
			if _, info := open(t, out, modeltest.PairPasswordA); info.Version != tt.version {
				t.Errorf("the file is KDBX %v, want %v", info.Version, tt.version)
			}
		})
	}
}

func TestEncryptionNotInVersion(t *testing.T) {
	// Make A a KDBX 4 file with Argon2; KDBX 3.1 cannot hold that.
	h := newHarness(t, kit.LightTheme(), newSession(t, func(a, _ *session.Source) {
		a.Info.KDF = crypto.KDF{ID: crypto.KDFArgon2id, Memory: 8 << 20, Iterations: 2, Parallelism: 1, Version: crypto.Argon2Version}
	}))
	h.setPath(filepath.Join(t.TempDir(), "merged.kdbx"))
	h.choose("Encryption", "As A")
	h.choose("Version", "3.1")
	if h.banner("KDBX 3.1 has only the AES-KDF") == "" {
		t.Fatal("no warning about the encryption")
	}
	if h.button("Write").Enabled() {
		t.Error("Write is enabled with an encryption the version cannot hold")
	}
	h.choose("Encryption", "As B") // B is a KDBX 3.1 file
	if h.banner("KDBX 3.1 has only the AES-KDF") != "" || !h.button("Write").Enabled() {
		t.Error("B's encryption fits KDBX 3.1, but the warning stays")
	}
	h.choose("Version", "KDBX 4")
	h.choose("Encryption", "As A")
	if !h.button("Write").Enabled() {
		t.Error("A's encryption fits KDBX 4, but Write is disabled")
	}
}

// withGroupTags gives a group of A tags, which KDBX 3 cannot store.
func withGroupTags(a, _ *session.Source) { a.DB.Root.Groups[0].Tags = []string{"private"} }

func TestLossesNeedConfirmation(t *testing.T) {
	h := newHarness(t, kit.LightTheme(), newSession(t, withGroupTags))
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	h.setPath(out)
	h.choose("Encryption", "Defaults of the version")
	if h.banner("KDBX 4.0 cannot store") != "" {
		t.Fatal("KDBX 4 reports a loss")
	}
	h.choose("Version", "3.1")
	if h.banner("KDBX 3.1 cannot store everything in the result: 1 thing would be lost.") == "" {
		t.Fatalf("no loss warning; banners: %v", h.state().losses)
	}
	if !h.ts.Exists(gluttertest.ByTextContaining("tags")) && len(h.state().losses) == 0 {
		t.Error("the loss is not listed")
	}

	// Cancel in the dialog: nothing is written.
	h.press("Write")
	if !h.ts.Exists(gluttertest.ByText("Write although data is lost?")) {
		t.Fatal("no confirmation dialog")
	}
	h.press("Cancel")
	if _, err := os.Stat(out); err == nil {
		t.Error("the file was written although the user cancelled")
	}
	if h.state().confirming {
		t.Error("the dialog stays open")
	}

	// Confirm: written, and the banner says that something was left out.
	h.press("Write")
	h.press("Write and lose it")
	h.waitWritten()
	if o := h.state().outcome; o.err != nil || o.losses != 1 {
		t.Fatalf("outcome %+v", o)
	}
	if !h.ts.Exists(gluttertest.ByTextContaining("1 thing was left out, as you confirmed")) {
		t.Error("the banner does not say what was accepted")
	}
	if _, info := open(t, out, modeltest.PairPasswordA); info.Version != (kdbx.Version{Major: 3, Minor: 1}) {
		t.Errorf("version %v", info.Version)
	}
}

func TestWriteRefusesFilesThatExist(t *testing.T) {
	sess := newSession(t)
	h := newHarness(t, kit.LightTheme(), sess)
	dir := t.TempDir()
	existing := filepath.Join(dir, "old.kdbx")
	if err := os.WriteFile(existing, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Typed: never replaced.
	h.setPath(existing)
	h.press("Write")
	h.waitWritten()
	if want := "The database was not written: old.kdbx exists; choose another name, or confirm that it is replaced in the file dialog"; h.banner("The database was not written") != want {
		t.Errorf("banner %q", h.banner("The database was not written"))
	}
	if b, _ := os.ReadFile(existing); string(b) != "keep me" {
		t.Errorf("the file was changed: %q", b)
	}

	// Chosen in the dialog, which asks the user to confirm replacing: replaced.
	h.press("Choose…")
	h.waitFor("the dialog", func() bool { return h.fd.PendingFileChoosers() == 1 })
	h.fd.AnswerFileChooser(existing)
	h.waitFor("the path", func() bool { return h.state().path.Text() == existing && h.fd.PendingFileChoosers() == 0 })
	h.press("Write")
	h.waitFor("the write", func() bool { return !h.state().writing && h.state().outcome != nil && h.state().outcome.err == nil })
	open(t, existing, modeltest.PairPasswordA)

	// The input databases are never the output, also when the dialog chose them.
	for _, in := range []string{sess.A.Path, sess.B.Path} {
		before := sum(t, in)
		h.setPath(in)
		h.state().chosenPath = in
		h.press("Write")
		h.waitFor("the refusal", func() bool { return !h.state().writing && h.state().outcome != nil && h.state().outcome.err != nil })
		if !h.ts.Exists(gluttertest.ByTextContaining("the output must be a new file, not one of the inputs")) {
			t.Errorf("no refusal for %s", filepath.Base(in))
		}
		if sum(t, in) != before {
			t.Errorf("%s was changed", filepath.Base(in))
		}
	}
}

func TestWriteInAMissingFolder(t *testing.T) {
	h := newHarness(t, kit.LightTheme(), newSession(t))
	h.setPath(filepath.Join(t.TempDir(), "no", "such", "merged.kdbx"))
	h.press("Write")
	h.waitWritten()
	if !h.ts.Exists(gluttertest.ByTextContaining("merged.kdbx cannot be written")) {
		t.Errorf("banner: %q", h.banner("The database was not written"))
	}
	// The write runs in a goroutine; errors.Async, called by begin on the UI thread,
	// starts the route there.
	assertRoute(t, h.state().outcome.err, "screenState.startWrite->screenState.begin->screenState.begin.func2->Write->write.OutputTempFailed")
}

func TestWriteWithAMissingKeyFile(t *testing.T) {
	h := newHarness(t, kit.LightTheme(), newSession(t))
	h.setPath(filepath.Join(t.TempDir(), "merged.kdbx"))
	h.choose("Credentials", "New")
	h.state().keyFile.SetText(filepath.Join(t.TempDir(), "missing.key"))
	h.ts.PumpAndSettle()
	h.press("Write")
	h.waitWritten()
	assertRoute(t, h.state().outcome.err, "screenState.startWrite->screenState.begin->screenState.begin.func2->ReadKeyFile.KeyFileReadFailed")
}

func TestPlanThatCannotBeApplied(t *testing.T) {
	sess := newSession(t)
	sess.Plan.Choices[0] = plan.Choice("no-such-choice")
	h := newHarness(t, kit.LightTheme(), sess)
	r := h.state().result()
	if r.db != nil {
		t.Fatal("a plan that cannot be applied gave a result")
	}
	if h.button("Write").Enabled() {
		t.Error("Write is enabled without a result")
	}
	// merge.Apply runs in a goroutine; errors.Async, called by build on the UI thread,
	// starts the route there.
	assertRoute(t, r.err, "screenState.InitState->screenState.build->screenState.build.func1->Apply->Validate.PlanInvalidChoice")
}

func TestFileDialogsWithoutAFileChooser(t *testing.T) {
	tests := []struct {
		name string
		// choose is which Choose… button is pressed: 0 for the new database, 1 for the
		// key file of the new credentials.
		choose int
		// route is the whole route across the goroutine of the dialog: errors.Async,
		// called on the UI thread, starts it at the function that opens the dialog.
		route string
	}{
		{"the new database", 0, "screenState.choosePath->screenState.choosePath.func1->SaveFile->call.chooseFiles.FileChooserUnavailable"},
		{"the key file", 1, "screenState.chooseKeyFile->screenState.chooseKeyFile.func1->OpenFiles->call.chooseFiles.FileChooserUnavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, kit.LightTheme(), newSession(t))
			h.choose("Credentials", "New")
			h.fd.DisableFileChooser()
			var buttons []*kit.Button
			for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Button]()) {
				if b := e.Widget().(*kit.Button); b.Label() == "Choose…" {
					buttons = append(buttons, b)
				}
			}
			if len(buttons) != 2 {
				t.Fatalf("%d Choose… buttons, want 2", len(buttons))
			}
			h.ts.Tap(gluttertest.ByWidget(buttons[tt.choose]))
			h.ts.PumpAndSettle()
			h.waitFor("an error", func() bool { return h.state().err != nil })
			assertRoute(t, h.state().err, tt.route)
		})
	}
}

func TestNewCredentials(t *testing.T) {
	h := newHarness(t, kit.LightTheme(), newSession(t))
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	h.setPath(out)
	h.choose("Credentials", "New")
	if h.button("Write").Enabled() {
		t.Error("Write is enabled with no new password and no key file")
	}
	if h.banner("A database needs a password, a key file, or both.") == "" {
		t.Error("no warning about empty credentials")
	}
	h.state().password.SetText("new-secret")
	h.state().confirm.SetText("other")
	h.ts.PumpAndSettle()
	if h.banner("The two passwords are not the same.") == "" || h.button("Write").Enabled() {
		t.Error("different passwords are accepted")
	}
	h.state().confirm.SetText("new-secret")
	h.ts.PumpAndSettle()
	h.press("Write")
	h.waitWritten()
	if o := h.state().outcome; o.err != nil {
		t.Fatalf("write failed: %v", o.err)
	}
	open(t, out, "new-secret")
	if _, _, err := kdbx.Open(context.Background(), out, crypto.Credentials{HasPassword: true, Password: secret.New([]byte(modeltest.PairPasswordA))}); err == nil {
		t.Error("the file opens with A's password")
	}
}

func TestNewCredentialsWithAKeyFile(t *testing.T) {
	h := newHarness(t, kit.LightTheme(), newSession(t))
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	key, err := filepath.Abs(filepath.Join("..", "..", "core", "kdbx", "testdata", "keyfile-xml2.keyx"))
	if err != nil {
		t.Fatal(err)
	}
	h.setPath(out)
	h.choose("Credentials", "New")
	h.state().keyFile.SetText(key)
	h.ts.PumpAndSettle()
	if !h.button("Write").Enabled() {
		t.Fatal("a key file alone is a valid credential")
	}
	h.press("Write")
	h.waitWritten()
	if o := h.state().outcome; o.err != nil {
		t.Fatalf("write failed: %v", o.err)
	}
	k, err := crypto.ReadKeyFile(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := kdbx.Open(context.Background(), out, crypto.Credentials{HasKeyFile: true, KeyFile: k})
	if err != nil {
		t.Fatalf("the file does not open with the key file: %v", err)
	}
	db.Wipe()
	if sess := h.sess; sess.Output.KeyFile != key {
		t.Errorf("remembered key file %q", sess.Output.KeyFile)
	}
}

func TestCredentialsOfB(t *testing.T) {
	h := newHarness(t, kit.LightTheme(), newSession(t))
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	h.setPath(out)
	h.choose("Credentials", "As B")
	h.press("Write")
	h.waitWritten()
	open(t, out, modeltest.PairPasswordB)
}

func TestSettingsSource(t *testing.T) {
	sess := newSession(t)
	h := newHarness(t, kit.LightTheme(), sess)
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	h.setPath(out)
	h.choose("Database settings from", "From B")
	h.waitFor("the new result", func() bool { return h.state().result() != nil })
	if i := h.state().settingsIdx; sess.Plan.Choices[i] != plan.TakeB || !sess.Decided[i] {
		t.Fatalf("the settings choice is %q", sess.Plan.Choices[i])
	}
	h.press("Write")
	h.waitWritten()
	db, _ := open(t, out, modeltest.PairPasswordA)
	if db.Meta.DatabaseName != sess.B.DB.Meta.DatabaseName {
		t.Errorf("database name %q, want B's %q", db.Meta.DatabaseName, sess.B.DB.Meta.DatabaseName)
	}
}

func TestBackKeepsTheOptions(t *testing.T) {
	sess := newSession(t)
	h := newHarness(t, kit.LightTheme(), sess)
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	h.setPath(out)
	h.choose("Version", "3.1")
	h.choose("Credentials", "As B")
	h.choose("Encryption", "Defaults of the version")
	choices := append([]plan.Choice(nil), sess.Plan.Choices...)
	h.press("Back")
	if h.back != 1 {
		t.Fatal("Back did nothing")
	}
	// The screen comes back: the options are as they were, the plan too.
	h.ts.PumpWidget(widgets.NewSizedBox(nil))
	h.ts.PumpAndSettle()
	h.show(kit.LightTheme())
	if h.state().path.Text() != out || LookupVersion(sess.Output.Version).Value != "3.1" || CredentialsSource(sess.Output) != SourceB {
		t.Errorf("options after coming back: %+v", sess.Output)
	}
	for i, c := range sess.Plan.Choices {
		if c != choices[i] {
			t.Errorf("choice %d changed", i)
		}
	}
}

func TestCancelWhileWriting(t *testing.T) {
	// A key derivation that takes seconds, so that Cancel arrives in the middle of it.
	h := newHarness(t, kit.LightTheme(), newSession(t, func(a, _ *session.Source) {
		a.Info.KDF = crypto.KDF{ID: crypto.KDFArgon2id, Memory: 256 << 20, Iterations: 400, Parallelism: 1, Version: crypto.Argon2Version}
	}))
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	h.setPath(out)
	h.choose("Encryption", "As A")
	h.press("Write")
	h.waitFor("the write to start", func() bool { return h.state().writing })
	if !h.ts.Exists(gluttertest.ByTextContaining("Writing and verifying")) {
		t.Error("no progress while writing")
	}
	h.press("Cancel")
	h.waitFor("the write to stop", func() bool { return !h.state().writing })
	if h.state().outcome != nil {
		t.Errorf("a cancelled write reports %+v", h.state().outcome)
	}
	if entries, _ := os.ReadDir(filepath.Dir(out)); len(entries) != 0 {
		t.Errorf("%d files left in the folder after Cancel", len(entries))
	}
	if !h.button("Write").Enabled() {
		t.Error("Write stays disabled after Cancel")
	}
}

func TestCancelledErrors(t *testing.T) {
	for _, tag := range []string{"WriteCancelled", "KdfCancelled", "ReadCancelled"} {
		if !cancelled(errorWithTag(tag)) {
			t.Errorf("%s is not a cancellation", tag)
		}
	}
	if cancelled(errorWithTag("OutputExists")) || cancelled(nil) {
		t.Error("another error is a cancellation")
	}
}

func TestScreenWithoutAPlan(t *testing.T) {
	sess := session.New()
	t.Cleanup(sess.Wipe)
	h := &harness{t: t, sess: sess}
	h.ts = testutil.New(t)
	h.ts.SetViewSize(ui.Size{Width: 1000, Height: 900})
	h.ts.PumpWidget(widgets.NewDirectionality(ui.TextDirectionLTR, kit.NewRoot(kit.LightTheme(), NewScreen(sess, func() {}))))
	h.ts.PumpAndSettle()
	if h.banner("Compare the two databases first.") == "" {
		t.Error("no hint that there is nothing to write")
	}
	if h.button("Write").Enabled() {
		t.Error("Write is enabled with nothing to write")
	}
}

func TestScreenGolden(t *testing.T) {
	for _, th := range []struct {
		name  string
		theme kit.Theme
	}{{"light", kit.LightTheme()}, {"dark", kit.DarkTheme()}} {
		t.Run(th.name, func(t *testing.T) {
			h := newHarness(t, th.theme, newSession(t, withGroupTags))
			h.ts.SetViewSize(ui.Size{Width: 1000, Height: 1180})
			h.ts.PumpAndSettle()
			h.setPath("/home/me/merged.kdbx")
			h.choose("Version", "3.1")
			h.choose("Encryption", "Defaults of the version")
			h.ts.MatchesGolden("write-" + th.name)
			if th.name == "light" {
				testutil.Proof(t, h.ts, "8.5-write")
			}
		})
	}
}

func TestSuccessGolden(t *testing.T) {
	h := newHarness(t, kit.LightTheme(), newSession(t))
	h.ts.SetViewSize(ui.Size{Width: 1000, Height: 1280})
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	h.setPath(out)
	h.choose("Credentials", "New")
	h.state().password.SetText("new-secret")
	h.state().confirm.SetText("new-secret")
	h.ts.PumpAndSettle()
	h.press("Write")
	h.waitWritten()
	// The temporary folder differs between runs: show a fixed path instead.
	done := h.state().outcome
	h.state().path.SetText("/home/me/merged.kdbx") // this clears the outcome
	h.state().outcome = done
	h.ts.PumpAndSettle()
	h.ts.MatchesGolden("write-done")
}

func errorWithTag(tag string) error { return errors.New(tag, "x") }

// assertRoute fails the test unless err is an *errors.Error whose route contains route.
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
