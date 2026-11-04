package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
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

// scriptPath returns the absolute path of a script of the repository.
func scriptPath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "scripts", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// runScript runs a script and returns its stdout, stderr and exit status. env
// is added to the environment of the test.
func runScript(t *testing.T, env []string, script string, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{scriptPath(t, script)}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	var out, errs strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		status = ee.ExitCode()
	case err != nil:
		t.Fatalf("running %s: %v", script, err)
	}
	return out.String(), errs.String(), status
}

// TestIsolatedScript checks that scripts/isolated.sh moves HOME and the XDG
// folders into a temporary folder, passes the exit status through, and removes the
// folder afterwards.
func TestIsolatedScript(t *testing.T) {
	tmp := t.TempDir()
	env := []string{"TMPDIR=" + tmp, "HOME=/home/someone-else", "XDG_CONFIG_HOME=/real/config"}
	out, _, status := runScript(t, env, "isolated.sh", "--", "env")
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	vars := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			vars[k] = v
		}
	}
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR"} {
		if !strings.HasPrefix(vars[k], tmp+"/passbank-iso.") {
			t.Errorf("%s = %q, want a folder below %s/passbank-iso.*", k, vars[k], tmp)
		}
	}
	if vars["GOCACHE"] == "" || strings.HasPrefix(vars["GOCACHE"], tmp) {
		t.Errorf("GOCACHE = %q, want Go's real cache folder", vars["GOCACHE"])
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the temporary folder was not removed: %v", entries)
	}

	if _, _, status := runScript(t, env, "isolated.sh", "--", "sh", "-c", "exit 7"); status != 7 {
		t.Errorf("status = %d, want the command's 7", status)
	}
	for _, args := range [][]string{{}, {"env"}, {"--"}} {
		if _, _, status := runScript(t, env, "isolated.sh", args...); status != 64 {
			t.Errorf("isolated.sh %v: status = %d, want 64 (usage)", args, status)
		}
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("a failed run left %v behind", entries)
	}
}

// TestHeavyScript checks scripts/heavy.sh: the exit status and the VAR=value
// assignments pass through, and a third command waits while two others hold the
// two slots.
func TestHeavyScript(t *testing.T) {
	run := t.TempDir()
	env := []string{"XDG_RUNTIME_DIR=" + run}
	out, _, status := runScript(t, env, "heavy.sh", "--", "FOO=bar", "sh", "-c", "echo $FOO")
	if status != 0 || strings.TrimSpace(out) != "bar" {
		t.Errorf("assignment: status %d, stdout %q", status, out)
	}
	if _, _, status := runScript(t, env, "heavy.sh", "--", "sh", "-c", "exit 5"); status != 5 {
		t.Errorf("status = %d, want the command's 5", status)
	}
	if _, _, status := runScript(t, env, "heavy.sh", "--alone", "--", "true"); status != 0 {
		t.Errorf("--alone: status = %d", status)
	}
	if _, _, status := runScript(t, env, "heavy.sh"); status != 2 {
		t.Errorf("no command: status = %d, want 2 (usage)", status)
	}

	// Two commands hold the two slots for a second; a third has to wait for one.
	var holders []*exec.Cmd
	for range 2 {
		cmd := exec.Command("bash", scriptPath(t, "heavy.sh"), "--", "sleep", "1.5")
		cmd.Env = append(os.Environ(), env...)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		holders = append(holders, cmd)
	}
	time.Sleep(500 * time.Millisecond) // let both take their slot
	start := time.Now()
	_, stderr, status := runScript(t, env, "heavy.sh", "--", "true")
	waited := time.Since(start)
	for _, cmd := range holders {
		if err := cmd.Wait(); err != nil {
			t.Error(err)
		}
	}
	if status != 0 || waited < 700*time.Millisecond || !strings.Contains(stderr, "waiting for a free slot") {
		t.Errorf("third command: status %d after %v, stderr %q; want it to wait about a second and say so", status, waited, stderr)
	}
}

// TestGPUEnvScript checks that scripts/gpu-env.sh keeps the NVIDIA card out.
func TestGPUEnvScript(t *testing.T) {
	cmd := exec.Command("bash", "-c", `source "$1"; echo "$__GLX_VENDOR_LIBRARY_NAME|$VK_DRIVER_FILES|$__EGL_VENDOR_LIBRARY_FILENAMES"`, "bash", scriptPath(t, "gpu-env.sh"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) != 3 || parts[0] != "mesa" {
		t.Fatalf("output %q, want mesa as the GLX vendor", out)
	}
	if strings.Contains(strings.ToLower(parts[1]), "nvidia") || strings.Contains(strings.ToLower(parts[2]), "nvidia") {
		t.Errorf("an NVIDIA driver is still visible: %q", out)
	}
}
