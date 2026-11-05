package app

import (
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

// scales are the text scales every screen is tried at: the default and 200 %.
var scales = []struct {
	name   string
	factor float64
}{{"100", 1}, {"200", 2}}

func setScale(ts *gluttertest.Tester, factor float64) {
	ts.SetPlatformConfiguration(func(c *ui.PlatformConfiguration) { c.TextScaleFactor = factor })
	ts.PumpAndSettle()
}

// pumpSemantic shows the app with semantics on.
func pumpSemantic(t *testing.T, dev DevOptions) (*gluttertest.Tester, *gluttertest.FakeDesktop, *appState) {
	t.Helper()
	ts := testutil.New(t)
	ts.EnsureSemantics()
	fd := ts.FakeDesktop()
	ts.SetViewSize(ui.Size{Width: 1100, Height: 720})
	ts.PumpWidget(NewAppWith(dev))
	ts.PumpAndSettle()
	st := gluttertest.StateOf[*appState](ts, gluttertest.ByType[*App]())
	t.Cleanup(st.sess.Wipe)
	if dev.Complete() {
		fd.WaitFor("the Review screen", func() bool {
			ts.PumpAndSettle()
			return st.screen == screenReview && ts.Exists(gluttertest.ByText("Banking / My bank"))
		})
	}
	return ts, fd, st
}

// The secrets that no label, value or hint may hold: the passwords of the fixtures
// and of the tests.
var secretsOfTheTests = []string{
	"bank-password-a", "bank-password-b", "store-password", "pw-personal", "forum-password", "old-account-password",
	"merge-pair-a", "merge-pair-b", "typed-secret", "new-secret",
}

func TestSemanticsOfTheOpenScreen(t *testing.T) {
	for _, sc := range scales {
		t.Run(sc.name, func(t *testing.T) {
			ts, _, _ := pumpSemantic(t, DevOptions{})
			setScale(ts, sc.factor)
			ts.EnterText(gluttertest.At(gluttertest.ByType[*widgets.EditableText](), 1), "typed-secret") // the password field of A
			ts.PumpAndSettle()
			nodes := testutil.CheckSemantics(t, ts, secretsOfTheTests...)
			for _, label := range []string{
				"Database A file", "Database B file", "Password of database A", "Password of database B",
				"Key file of database A", "Key file of database B", "Swap A and B", "Compare", "Unlock database A", "Unlock database B",
				"Choose… Database A file", "Show the Password of database A",
			} {
				if _, ok := testutil.FindNode(nodes, label); !ok {
					t.Errorf("no semantics node named %q", label)
				}
			}
			for _, label := range []string{"Password of database A", "Password of database B"} {
				if n, ok := testutil.FindTextField(nodes, label); !ok || !n.Flags.IsObscured {
					t.Errorf("%q is not a text field marked as obscured", label)
				}
			}
			if n, ok := testutil.FindNode(nodes, "Compare"); ok && n.Flags.IsEnabled != ui.TristateIsFalse {
				t.Errorf("Compare is not disabled before both databases are unlocked: %v", n.Flags.IsEnabled)
			}
		})
	}
}

func TestSemanticsOfTheReviewScreen(t *testing.T) {
	for _, sc := range scales {
		t.Run(sc.name, func(t *testing.T) {
			ts, _, _ := pumpSemantic(t, mergeFixtures(t))
			setScale(ts, sc.factor)
			ts.Tap(gluttertest.Descendant(gluttertest.ByType[*kit.ListRow](), gluttertest.ByText("Banking / My bank")))
			ts.PumpAndSettle()
			nodes := testutil.CheckSemantics(t, ts, secretsOfTheTests...)
			for _, label := range []string{
				"Load plan…", "Save plan…", "Search group path and title", "Back", "Next: write the new database", "Newer wins",
				"All 9", "Differs 2", "Undecided 9", "Changes",
			} {
				if _, ok := testutil.FindNode(nodes, label); !ok {
					t.Errorf("no semantics node named %q", label)
				}
			}
			// A row says what it is, its choice and whether it is decided.
			if _, ok := testutil.FindNode(nodes, "Differs, Banking / My bank, Take B, default"); !ok {
				t.Error("the row of the selected change is not described")
			}
			// The table says which fields differ, never their values. (At 200 % it is below
			// the fold of the scrolling page, where semantics do not reach.)
			if _, ok := testutil.FindNode(nodes, "Password, differs between A and B. The values are hidden."); !ok && sc.factor == 1 {
				t.Error("the password row of the table is not described")
			}
		})
	}
}

func TestSemanticsOfTheWriteScreen(t *testing.T) {
	for _, sc := range scales {
		t.Run(sc.name, func(t *testing.T) {
			ts, fd, st := pumpSemantic(t, mergeFixtures(t))
			setScale(ts, sc.factor)
			st.SetState(func() { st.screen = screenWrite })
			fd.WaitFor("the Write screen", func() bool { ts.PumpAndSettle(); return ts.Exists(gluttertest.ByText("Write the new database")) })
			st.sess.Output.Credentials = "new"
			st.SetState(func() {})
			ts.PumpAndSettle()
			editables := ts.Elements(gluttertest.ByType[*widgets.EditableText]())
			if len(editables) < 3 {
				t.Fatalf("%d text fields on the Write screen with new credentials", len(editables))
			}
			ts.EnterText(gluttertest.At(gluttertest.ByType[*widgets.EditableText](), 1), "typed-secret")
			ts.PumpAndSettle()
			nodes := testutil.CheckSemantics(t, ts, secretsOfTheTests...)
			for _, label := range []string{
				"New database", "Choose… New database", "New password", "Repeat the new password", "Key file of the new database",
				"Version", "Credentials", "Encryption", "Back", "Write",
			} {
				if _, ok := testutil.FindNode(nodes, label); !ok {
					t.Errorf("no semantics node named %q", label)
				}
			}
			for _, label := range []string{"New password", "Repeat the new password"} {
				if n, ok := testutil.FindTextField(nodes, label); !ok || !n.Flags.IsObscured {
					t.Errorf("%q is not a text field marked as obscured", label)
				}
			}
		})
	}
}

// Every screen at 200 % text size: nothing overflows (the tester fails on an
// overflow), and the golden images show what the user sees.
func TestGoldensAtTwiceTheTextSize(t *testing.T) {
	t.Run("open", func(t *testing.T) {
		ts, _, _ := pumpSemantic(t, DevOptions{})
		setScale(ts, 2)
		ts.MatchesGolden("scale200-open")
	})
	t.Run("review", func(t *testing.T) {
		ts, _, _ := pumpSemantic(t, mergeFixtures(t))
		setScale(ts, 2)
		ts.Tap(gluttertest.Descendant(gluttertest.ByType[*kit.ListRow](), gluttertest.ByText("Banking / My bank")))
		ts.PumpAndSettle()
		ts.MatchesGolden("scale200-review")
	})
	t.Run("write", func(t *testing.T) {
		ts, fd, st := pumpSemantic(t, mergeFixtures(t))
		setScale(ts, 2)
		st.SetState(func() { st.screen = screenWrite })
		fd.WaitFor("the Write screen", func() bool { ts.PumpAndSettle(); return ts.Exists(gluttertest.ByText("Write the new database")) })
		ts.MatchesGolden("scale200-write")
	})
}
