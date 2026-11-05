package app

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/gui/session"

	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

func TestHomeIsTheOpenScreen(t *testing.T) {
	ts := testutil.New(t)
	ts.PumpWidget(NewApp())
	ts.PumpAndSettle()
	if err := gluttertest.FindsOneWidget(ts, gluttertest.ByText("Merge two KeePass databases")); err != nil {
		t.Error(err)
	}
}

func TestMainArguments(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantOut    string // a substring of stdout
		wantErr    string // a substring of stderr
	}{
		{"version", []string{"--version"}, 0, "passbank-gui ", ""},
		{"version with one dash", []string{"-version"}, 0, "passbank-gui ", ""},
		{"help", []string{"--help"}, 0, "", "usage: passbank-gui"},
		{"unknown flag", []string{"--nope"}, 2, "", "flag provided but not defined: -nope"},
		{"extra argument", []string{"merge"}, 2, "", `unexpected argument "merge"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			status := Main(context.Background(), tt.args, &out, &errs)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d (stderr: %s)", status, tt.wantStatus, errs.String())
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), tt.wantOut)
			}
			if !strings.Contains(errs.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errs.String(), tt.wantErr)
			}
		})
	}
}

// TestRunFailsWithoutADisplay needs no display: without WAYLAND_DISPLAY and
// DISPLAY the platform reports NoDisplay, and Run tags it.
func TestRunFailsWithoutADisplay(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("GLUTTER_BACKEND", "")
	err := Run(context.Background())
	if err == nil {
		t.Fatal("Run must fail when there is no display")
	}
	assertRoute(t, err, "RunWith.AppRunFailed")
}

func unlockFixture(t *testing.T, name string) *session.Source {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "core", "kdbx", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	src, err := session.Unlock(context.Background(), path, secret.New([]byte("fixture-rich")), "")
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestCompareThenBackWipesTheSession(t *testing.T) {
	ts := testutil.New(t)
	ts.PumpWidget(NewApp())
	ts.PumpAndSettle()
	st := gluttertest.StateOf[*appState](ts, gluttertest.ByType[*App]())
	a, b := unlockFixture(t, "k4-rich.kdbx"), unlockFixture(t, "k31-rich.kdbx")
	st.SetState(func() { st.sess.SetA(a); st.sess.SetB(b) })
	ts.PumpAndSettle()
	ts.Tap(gluttertest.ByText("Compare"))
	ts.PumpAndSettle()
	if st.screen != screenReview {
		t.Fatalf("screen = %v, want the Review screen", st.screen)
	}
	password := firstPassword(a)
	ts.Tap(gluttertest.ByText("Back"))
	ts.PumpAndSettle()
	if st.screen != screenOpen || st.sess.A != nil || st.sess.B != nil {
		t.Errorf("going back to Open left the session: screen %v, A %v, B %v", st.screen, st.sess.A != nil, st.sess.B != nil)
	}
	for _, c := range password.Reveal() {
		if c != 0 {
			t.Fatal("a password of A was not wiped when going back to Open")
		}
	}
}

func TestClosingTheWindowWipesTheSession(t *testing.T) {
	ts := testutil.New(t)
	ts.PumpWidget(NewApp())
	ts.PumpAndSettle()
	st := gluttertest.StateOf[*appState](ts, gluttertest.ByType[*App]())
	a := unlockFixture(t, "k4-rich.kdbx")
	st.SetState(func() { st.sess.SetA(a) })
	password := firstPassword(a)
	ts.PumpWidget(widgets.NewSizedBox(nil)) // the app leaves the tree, as when the window closes
	ts.PumpAndSettle()
	for _, c := range password.Reveal() {
		if c != 0 {
			t.Fatal("a password of A was not wiped when the app went")
		}
	}
}

func firstPassword(src *session.Source) secret.Bytes {
	var pw secret.Bytes
	src.DB.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
		if f, ok := e.Field(model.FieldPassword); ok && f.Value.Len() > 0 {
			pw = f.Value
			return false
		}
		return true
	})
	return pw
}
