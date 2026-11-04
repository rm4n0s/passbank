package archtest

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestErrorTagsScript runs scripts/error-tags.sh on fixture trees, the way a
// person runs it on the repository.
func TestErrorTagsScript(t *testing.T) {
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "error-tags.sh"))
	if err != nil {
		t.Fatal(err)
	}
	clean, err := filepath.Abs(filepath.Join("testdata", "tags-clean"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := filepath.Abs(tagsFixture)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantOut    string
	}{
		{"nothing untested", []string{"--untested", clean}, 0, ""},
		{"untested Tags", []string{"--untested", fixture}, 1, "NeverTested"},
		{"every Tag", []string{clean}, 0, "OpenFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("bash", append([]string{script}, tt.args...)...)
			out, err := cmd.Output()
			status := 0
			if ee, ok := err.(*exec.ExitError); ok {
				status = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("running the script: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if tt.wantOut == "" && len(out) != 0 {
				t.Errorf("stdout = %q, want nothing", out)
			}
			if !strings.Contains(string(out), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", out, tt.wantOut)
			}
		})
	}
}
