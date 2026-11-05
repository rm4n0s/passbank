//go:build glutter_release

package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// A release build does not have the --dev flags: the flag package refuses them.
// Run with: go test -tags glutter_release ./internal/gui/app
func TestDevFlagsAreAbsentInAReleaseBuild(t *testing.T) {
	for _, flagName := range []string{"--dev-a", "--dev-b", "--dev-password-file"} {
		var out, errs bytes.Buffer
		if status := Main(context.Background(), []string{flagName, "x"}, &out, &errs); status != 2 {
			t.Errorf("%s: status = %d, want 2", flagName, status)
		}
		if !strings.Contains(errs.String(), "flag provided but not defined") {
			t.Errorf("%s: stderr = %q", flagName, errs.String())
		}
	}
	var errs bytes.Buffer
	Main(context.Background(), []string{"--help"}, &bytes.Buffer{}, &errs)
	if strings.Contains(errs.String(), "dev-") {
		t.Errorf("the help of a release build shows a dev flag:\n%s", errs.String())
	}
}
