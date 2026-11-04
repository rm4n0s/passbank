package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

func TestHomeShowsTheName(t *testing.T) {
	ts := testutil.New(t)
	ts.PumpWidget(NewApp())
	ts.PumpAndSettle()
	if err := gluttertest.FindsOneWidget(ts, gluttertest.ByText(Title)); err != nil {
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
	assertRoute(t, err, "Run.AppRunFailed")
}
