package open

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/session"
	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "core", "kdbx", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// harness is the screen in a window, with its session.
type harness struct {
	t        *testing.T
	ts       *gluttertest.Tester
	fd       *gluttertest.FakeDesktop
	sess     *session.Session
	compared int
}

func newHarness(t *testing.T, theme kit.Theme) *harness {
	t.Helper()
	h := &harness{t: t, sess: session.New()}
	t.Cleanup(h.sess.Wipe)
	h.ts = testutil.New(t)
	// Each call of FakeDesktop makes a new fake, so there is one; it answers the file
	// dialogs and must exist before the widget is shown.
	h.fd = h.ts.FakeDesktop()
	h.ts.SetViewSize(ui.Size{Width: 1100, Height: 760})
	h.ts.PumpWidget(widgets.NewDirectionality(ui.TextDirectionLTR,
		kit.NewRoot(theme, NewScreen(h.sess, func() { h.compared++ }))))
	h.ts.PumpAndSettle()
	return h
}

func (h *harness) state() *screenState {
	return gluttertest.StateOf[*screenState](h.ts, gluttertest.ByType[*Screen]())
}

// waitFor pumps the UI thread until cond holds.
func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	h.fd.WaitFor(what, func() bool {
		h.ts.PumpAndSettle()
		return cond()
	})
}

// fill types a path, a password and a key file into a panel.
func (h *harness) fill(i int, path, password, key string) {
	p := h.state().panels[i]
	p.path.SetText(path)
	p.password.SetText(password)
	p.key.SetText(key)
	h.ts.PumpAndSettle()
}

// button finds the i-th button whose label starts with the prefix.
func (h *harness) button(prefix string, i int) *kit.Button {
	h.t.Helper()
	var found []widgets.Element
	for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Button]()) {
		if strings.HasPrefix(e.Widget().(*kit.Button).Label(), prefix) {
			found = append(found, e)
		}
	}
	if i >= len(found) {
		h.t.Fatalf("no button %d starting with %q", i, prefix)
	}
	return found[i].Widget().(*kit.Button)
}

func (h *harness) tap(prefix string, i int) {
	h.t.Helper()
	b := h.button(prefix, i)
	h.ts.Tap(gluttertest.ByWidget(b))
	h.ts.PumpAndSettle()
}

func (h *harness) unlock(i int, file, password, key string) {
	h.t.Helper()
	h.fill(i, fixture(h.t, file), password, key)
	h.tap("Unlock", i)
}

func TestUnlockShowsTheSummary(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	h.unlock(0, "k4-rich.kdbx", "fixture-rich", "")
	h.waitFor("A is unlocked", func() bool { return h.sess.A != nil })
	if h.sess.B != nil {
		t.Error("B was unlocked too")
	}
	for _, want := range []string{"Unlocked", "KDBX 4.0", "AES-256", "Argon2d", "4 groups, 4 entries, 1 attachment, 2 history items"} {
		if !h.ts.Exists(gluttertest.ByText(want)) {
			t.Errorf("the panel does not show %q", want)
		}
	}
	// The password is gone from the field once it is in the session.
	if got := h.state().panels[0].password.Text(); got != "" {
		t.Errorf("the password field still holds %d characters", len(got))
	}
	if h.button("Compare", 0).Enabled() {
		t.Error("Compare is enabled with one side unlocked")
	}
}

func TestUnlockWithAKeyFile(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	h.unlock(1, "k4-keyfile-xml2.kdbx", "", fixture(t, "keyfile-xml2.keyx"))
	h.waitFor("B is unlocked", func() bool { return h.sess.B != nil })
	if h.sess.A != nil {
		t.Error("A was unlocked")
	}
}

func TestUnlockErrors(t *testing.T) {
	tests := []struct {
		name, file, password, key, want string
	}{
		{"a wrong password", "k4-rich.kdbx", "wrong", "", "the password or key file for"},
		{"no credentials", "k4-rich.kdbx", "", "", "needs a password, a key file, or both"},
		{"a file that is no database", "keyfile-raw32.key", "x", "", "is not a KeePass database"},
		{"a missing file", "missing.kdbx", "x", "", "cannot be read"},
		{"a missing key file", "k4-rich.kdbx", "fixture-rich", "/no/such/key", "key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, kit.LightTheme())
			h.unlock(0, tt.file, tt.password, tt.key)
			h.waitFor("an error", func() bool { return h.state().panels[0].err != nil })
			if !h.ts.Exists(gluttertest.ByTextContaining(tt.want)) {
				t.Errorf("the panel does not say %q", tt.want)
			}
			if h.sess.A != nil {
				t.Error("A was unlocked")
			}
			// A failed unlock leaves the password for another try.
			if tt.password != "" && h.state().panels[0].password.Text() != tt.password {
				t.Error("the password was cleared by a failure")
			}
		})
	}
}

func TestCompareNeedsBothSides(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	h.unlock(0, "k4-rich.kdbx", "fixture-rich", "")
	h.waitFor("A", func() bool { return h.sess.A != nil })
	h.unlock(1, "k31-rich.kdbx", "fixture-rich", "")
	h.waitFor("B", func() bool { return h.sess.B != nil })
	if !h.button("Compare", 0).Enabled() {
		t.Fatal("Compare is disabled with both sides unlocked")
	}
	h.tap("Compare", 0)
	if h.compared != 1 {
		t.Errorf("Compare called onCompare %d times", h.compared)
	}
}

func TestSwap(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	h.unlock(0, "k4-rich.kdbx", "fixture-rich", "")
	h.waitFor("A", func() bool { return h.sess.A != nil })
	h.unlock(1, "k31-rich.kdbx", "fixture-rich", "")
	h.waitFor("B", func() bool { return h.sess.B != nil })
	a, b := h.sess.A, h.sess.B
	pathA, pathB := h.state().panels[0].path.Text(), h.state().panels[1].path.Text()
	h.tap("Swap", 0)
	if h.sess.A != b || h.sess.B != a {
		t.Error("the sources were not swapped")
	}
	if h.state().panels[0].path.Text() != pathB || h.state().panels[1].path.Text() != pathA {
		t.Error("the paths were not swapped")
	}
	if !h.button("Compare", 0).Enabled() {
		t.Error("Compare is disabled after the swap")
	}
}

func TestChooseWithTheFileDialog(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	fd := h.fd
	chosen := fixture(t, "k4-rich.kdbx")
	h.tap("Choose", 0) // the file of A
	h.waitFor("a dialog", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.AnswerFileChooser(chosen)
	h.waitFor("the path", func() bool { return h.state().panels[0].path.Text() == chosen })
}

func TestCancellingTheFileDialogChangesNothing(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	fd := h.fd
	h.tap("Choose", 3) // the key file of B
	h.waitFor("a dialog", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.CancelFileChooser()
	h.waitFor("the dialog to close", func() bool { return fd.PendingFileChoosers() == 0 })
	if got := h.state().panels[1].key.Text(); got != "" {
		t.Errorf("a cancelled dialog set the key file to %q", got)
	}
}

func TestChooseWithoutAFileChooser(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	h.fd.DisableFileChooser()
	h.tap("Choose", 0)
	h.waitFor("an error", func() bool { return h.state().panels[0].err != nil })
	if h.sess.A != nil || h.state().panels[0].path.Text() != "" {
		t.Error("the panel changed")
	}
}

func TestEditingThePathLocksTheSideAgain(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	h.unlock(0, "k4-rich.kdbx", "fixture-rich", "")
	h.waitFor("A", func() bool { return h.sess.A != nil })
	h.state().panels[0].path.SetText(fixture(t, "k4-empty.kdbx"))
	h.ts.PumpAndSettle()
	if h.sess.A != nil {
		t.Error("a side stays unlocked with a path that is not the file that was opened")
	}
	if !h.ts.Exists(gluttertest.ByText("Unlock")) {
		t.Error("the button does not say Unlock")
	}
}

func TestDroppingAFileSetsThePath(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	path := fixture(t, "k4-rich.kdbx")
	at := h.ts.Center(gluttertest.ByText("Database A"))
	drop := ui.DropEvent{Types: []string{"text/uri-list"}, Actions: ui.DropActionCopy, URIs: []string{"file://" + path}}
	view := h.ts.View.ViewID()
	h.ts.Drop(view, ui.DropPhaseEnter, at, drop)
	h.ts.Drop(view, ui.DropPhaseDrop, at, drop)
	h.ts.PumpAndSettle()
	if got := h.state().panels[0].path.Text(); got != path {
		t.Errorf("the path of A is %q, want the dropped file %q", got, path)
	}
	if got := h.state().panels[1].path.Text(); got != "" {
		t.Errorf("the path of B is %q", got)
	}
}

func TestPathOfURI(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"file:///home/me/a.kdbx", "/home/me/a.kdbx", true},
		{"file:///home/me/a%20b.kdbx", "/home/me/a b.kdbx", true},
		{"  file:///x  ", "/x", true},
		{"http://example.com/a.kdbx", "", false},
		{"/plain/path", "", false},
		{"file://", "", false},
		{"%zz", "", false},
	}
	for _, tt := range tests {
		got, ok := pathOfURI(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("pathOfURI(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestCancelWhileUnlocking(t *testing.T) {
	h := newHarness(t, kit.LightTheme())
	called := false
	p := h.state().panels[0]
	h.state().SetState(func() { p.unlocking, p.cancel = true, func() { called = true } })
	h.ts.PumpAndSettle()
	if !h.ts.Exists(gluttertest.ByTextContaining("Unlocking A")) {
		t.Error("the panel does not say it is unlocking")
	}
	if h.button("Swap", 0).Enabled() {
		t.Error("Swap is enabled while a side is being unlocked")
	}
	h.tap("Cancel", 0)
	if !called {
		t.Error("Cancel did not cancel")
	}
	h.state().SetState(func() { p.unlocking, p.cancel = false, nil })
}

func TestGolden(t *testing.T) {
	for _, th := range []struct {
		name  string
		theme kit.Theme
	}{{"light", kit.LightTheme()}, {"dark", kit.DarkTheme()}} {
		t.Run(th.name, func(t *testing.T) {
			h := newHarness(t, th.theme)
			h.unlock(0, "k4-rich.kdbx", "fixture-rich", "")
			h.waitFor("A", func() bool { return h.sess.A != nil })
			h.unlock(1, "k31-rich.kdbx", "wrong password", "")
			h.waitFor("an error", func() bool { return h.state().panels[1].err != nil })
			// The paths of the fixtures depend on the machine; the golden shows others.
			h.sess.A.Path = "/home/me/personal.kdbx"
			h.state().panels[0].path.SetText(h.sess.A.Path)
			h.state().panels[1].path.SetText("/home/me/laptop.kdbx")
			h.ts.PumpAndSettle()
			h.ts.MatchesGolden("open-" + th.name)
			if th.name == "light" {
				testutil.Proof(t, h.ts, "8.2-open")
			}
		})
	}
}
