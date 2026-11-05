package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

// mergeFixtures are the databases of the CLI's merge tests and their password file.
func mergeFixtures(t *testing.T) DevOptions {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "cli", "testdata", "merge"))
	if err != nil {
		t.Fatal(err)
	}
	return DevOptions{A: filepath.Join(dir, "a.kdbx"), B: filepath.Join(dir, "b.kdbx"), PasswordFile: filepath.Join(dir, "password.txt")}
}

func TestReadDevPasswords(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name, text string
		a, b       string
		route      string
	}{
		{"two lines", "one\ntwo\n", "one", "two", ""},
		{"one line is for both", "same\n", "same", "same", ""},
		{"CRLF", "one\r\ntwo\r\n", "one", "two", ""},
		{"more lines are ignored", "one\ntwo\nthree\n", "one", "two", ""},
		{"no newline at the end", "one\ntwo", "one", "two", ""},
		{"empty", "", "", "", "readDevPasswords.DevPasswordFileFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b, err := readDevPasswords(write("pw", tt.text))
			if tt.route != "" {
				appErr, ok := errors.FromError(err)
				if !ok || !appErr.HasRoute(tt.route) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || a != tt.a || b != tt.b {
				t.Errorf("got %q, %q, %v; want %q, %q", a, b, err, tt.a, tt.b)
			}
		})
	}
	_, _, err := readDevPasswords(filepath.Join(dir, "missing"))
	if appErr, ok := errors.FromError(err); !ok || !appErr.HasRoute("readDevPasswords.DevPasswordFileFailed") {
		t.Errorf("a missing file: err = %v", err)
	}
}

func TestDevOptions(t *testing.T) {
	tests := []struct {
		dev              DevOptions
		active, complete bool
	}{
		{DevOptions{}, false, false},
		{DevOptions{A: "a"}, true, false},
		{DevOptions{A: "a", B: "b"}, true, false},
		{DevOptions{A: "a", B: "b", PasswordFile: "p"}, true, true},
	}
	for _, tt := range tests {
		if tt.dev.Active() != tt.active || tt.dev.Complete() != tt.complete {
			t.Errorf("%+v: active %v, complete %v", tt.dev, tt.dev.Active(), tt.dev.Complete())
		}
	}
}

// pumpWith shows the application with the databases preloaded and waits for Review.
func pumpWith(t *testing.T, dev DevOptions) (*gluttertest.Tester, *gluttertest.FakeDesktop, *appState) {
	t.Helper()
	ts := testutil.New(t)
	fd := ts.FakeDesktop()
	ts.SetViewSize(ui.Size{Width: 1200, Height: 800})
	ts.PumpWidget(NewAppWith(dev))
	ts.PumpAndSettle()
	st := gluttertest.StateOf[*appState](ts, gluttertest.ByType[*App]())
	t.Cleanup(st.sess.Wipe)
	fd.WaitFor("the Review screen", func() bool {
		ts.PumpAndSettle()
		return st.screen == screenReview && ts.Exists(gluttertest.ByText("Banking / My bank"))
	})
	return ts, fd, st
}

func TestPreloadStartsOnReview(t *testing.T) {
	ts, _, st := pumpWith(t, mergeFixtures(t))
	if !st.sess.Ready() {
		t.Fatal("the databases are not in the session")
	}
	if !ts.Exists(gluttertest.ByText("A: a.kdbx    B: b.kdbx")) {
		t.Error("Review does not name the files")
	}
}

func TestPreloadFailureStaysOnOpen(t *testing.T) {
	dev := mergeFixtures(t)
	dev.PasswordFile = filepath.Join(t.TempDir(), "missing")
	ts := testutil.New(t)
	ts.PumpWidget(NewAppWith(dev))
	ts.PumpAndSettle()
	st := gluttertest.StateOf[*appState](ts, gluttertest.ByType[*App]())
	if st.screen != screenOpen || st.sess.Ready() {
		t.Errorf("screen %v, ready %v after a failed preload", st.screen, st.sess.Ready())
	}
	// A wrong password for one of them: nothing is kept.
	dev = mergeFixtures(t)
	bad := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(bad, []byte("wrong\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dev.PasswordFile = bad
	ts2 := testutil.New(t)
	fd := ts2.FakeDesktop()
	ts2.PumpWidget(NewAppWith(dev))
	ts2.PumpAndSettle()
	st2 := gluttertest.StateOf[*appState](ts2, gluttertest.ByType[*App]())
	fd.WaitFor("the failed unlock", func() bool { ts2.PumpAndSettle(); return true })
	if st2.screen != screenOpen || st2.sess.A != nil || st2.sess.B != nil {
		t.Errorf("a failed unlock left screen %v, A %v, B %v", st2.screen, st2.sess.A != nil, st2.sess.B != nil)
	}
}

// TestFullFlow drives the three screens with taps and keys: it chooses, goes to
// Write, writes and checks the file, then goes back.
func TestFullFlow(t *testing.T) {
	ts, fd, st := pumpWith(t, mergeFixtures(t))
	dev := mergeFixtures(t)
	tapRow := func(text string) {
		f := gluttertest.Descendant(gluttertest.ByType[*kit.ListRow](), gluttertest.ByText(text))
		ts.Tap(f)
		ts.PumpAndSettle()
	}

	// Review: take B's version of "My bank" (its password differs).
	tapRow("Banking / My bank")
	ts.SendKeyEvent(services.LogicalKeyboardKeyKeyB)
	ts.PumpAndSettle()
	if !ts.Exists(gluttertest.ByText("Undecided 8")) {
		t.Error("the choice did not count as decided")
	}

	// Write.
	ts.Tap(gluttertest.ByText("Next: write the new database"))
	ts.PumpAndSettle()
	if st.screen != screenWrite {
		t.Fatalf("screen = %v, want Write", st.screen)
	}
	out := filepath.Join(t.TempDir(), "merged.kdbx")
	ts.EnterText(gluttertest.ByType[*widgets.EditableText](), out)
	ts.PumpAndSettle()
	fd.WaitFor("the result", func() bool {
		ts.PumpAndSettle()
		return ts.Exists(gluttertest.ByTextContaining("The new database holds"))
	})
	ts.Tap(gluttertest.ByText("Write"))
	fd.WaitFor("the verified file", func() bool {
		ts.PumpAndSettle()
		return ts.Exists(gluttertest.ByTextContaining("Written and verified: merged.kdbx"))
	})
	db, _, err := kdbx.Open(context.Background(), out, crypto.Credentials{HasPassword: true, Password: secret.New([]byte(modeltest.PairPasswordA))})
	if err != nil {
		t.Fatalf("the written file does not open: %v", err)
	}
	t.Cleanup(db.Wipe)
	found := false
	db.WalkGroups(func(g *model.Group) bool {
		for _, e := range g.Entries {
			if e.Title() == "My bank" {
				f, _ := e.Field("Password")
				found = f.Text() == "bank-password-b"
			}
		}
		return true
	})
	if !found {
		t.Error("the entry My bank does not have B's password, as chosen")
	}
	for _, in := range []string{dev.A, dev.B} {
		if _, err := os.Stat(in); err != nil {
			t.Errorf("input %s: %v", in, err)
		}
	}

	// Back to Review: the choice is still there; Back again drops the secrets.
	ts.Tap(gluttertest.ByText("Back"))
	ts.PumpAndSettle()
	if st.screen != screenReview || !ts.Exists(gluttertest.ByText("Undecided 8")) {
		t.Fatalf("screen %v; the choices are not intact", st.screen)
	}
	ts.Tap(gluttertest.ByText("Back"))
	ts.PumpAndSettle()
	if st.screen != screenOpen || st.sess.A != nil || st.sess.B != nil {
		t.Errorf("screen %v; the databases are still in the session", st.screen)
	}
	if !strings.Contains(out, "merged.kdbx") {
		t.Fatal("unreachable")
	}
}
