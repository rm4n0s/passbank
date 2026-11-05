//go:build !glutter_release

package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// In a debug build the --dev flags exist; they go together, and none takes a
// password.
func TestDevFlagsExistInADebugBuild(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		status int
		errs   string
	}{
		{"only A", []string{"--dev-a", "x.kdbx"}, 2, "--dev-a, --dev-b and --dev-password-file go together"},
		{"A and B", []string{"--dev-a", "x.kdbx", "--dev-b", "y.kdbx"}, 2, "go together"},
		{"the help lists them", []string{"--help"}, 0, "dev-password-file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			if status := Main(context.Background(), tt.args, &out, &errs); status != tt.status {
				t.Errorf("status = %d, want %d (stderr: %s)", status, tt.status, errs.String())
			}
			if !strings.Contains(errs.String(), tt.errs) {
				t.Errorf("stderr = %q, want it to contain %q", errs.String(), tt.errs)
			}
		})
	}
	// No flag takes a password itself.
	var errs bytes.Buffer
	Main(context.Background(), []string{"--help"}, &bytes.Buffer{}, &errs)
	for _, name := range []string{"-dev-password ", "-password "} {
		if strings.Contains(errs.String(), name) {
			t.Errorf("the help shows a flag %q that would take a password", name)
		}
	}
}
