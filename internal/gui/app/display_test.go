//go:build passbank_display

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/testenv"
)

// These tests start the real application in a real window of a headless Wayland
// compositor. Run them alone, inside the compositor:
//
//	scripts/heavy.sh --alone -- scripts/headless-wayland.sh -- go test -race -tags passbank_display ./internal/gui/app
//
// The window is driven by the debug flags (--dev-a, --dev-b, --dev-password-file),
// which preload the fixtures; what the three screens do with input is tested with
// gluttertest in dev_test.go, because no tool that sends input to a Wayland window
// is installed (and Glutter has none). Nothing here uses a GPU: the renderer is
// GLES on the compositor's software path.

// build compiles the debug binary of the application.
func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "passbank-gui")
	cmd := exec.Command("go", "build", "-tags", "debug_mode", "-o", bin, "./cmd/passbank-gui")
	cmd.Dir = filepath.Join("..", "..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// start runs the application with the fixtures preloaded, in its own process
// group, and stops it when the test ends.
func start(t *testing.T, bin string, env ...string) *exec.Cmd {
	t.Helper()
	// The compositor's socket is in the runtime folder, which the isolation replaces: the
	// application gets the compositor's folder back and keeps the isolated config, data
	// and cache folders.
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	testenv.Isolate(t)
	dev := mergeFixtures(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	cmd := exec.CommandContext(ctx, bin, "--dev-a", dev.A, "--dev-b", dev.B, "--dev-password-file", dev.PasswordFile)
	cmd.Env = append(os.Environ(), append([]string{"GLUTTER_RENDERER=gles", "XDG_RUNTIME_DIR=" + runtime}, env...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) // a clean exit first
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cancel()
			<-done
		}
		cancel()
		if t.Failed() {
			t.Logf("the application printed:\n%s", out.String())
		}
	})
	return cmd
}

func needCompositor(t *testing.T) {
	t.Helper()
	if os.Getenv("WAYLAND_DISPLAY") == "" || os.Getenv("SWAYSOCK") == "" {
		t.Skip("needs the headless sway compositor of scripts/headless-wayland.sh")
	}
}

// windowOf finds the window of the application in the compositor's tree.
func windowOf(t *testing.T) (title string, width, height float64, ok bool) {
	t.Helper()
	out, err := exec.Command("swaymsg", "-t", "get_tree").Output()
	if err != nil {
		t.Fatalf("swaymsg: %v", err)
	}
	var tree map[string]any
	if err := json.Unmarshal(out, &tree); err != nil {
		t.Fatal(err)
	}
	var walk func(n map[string]any) bool
	walk = func(n map[string]any) bool {
		if n["app_id"] == AppID {
			title, _ = n["name"].(string)
			if r, isMap := n["window_rect"].(map[string]any); isMap {
				width, _ = r["width"].(float64)
				height, _ = r["height"].(float64)
			}
			return true
		}
		for _, key := range []string{"nodes", "floating_nodes"} {
			kids, _ := n[key].([]any)
			for _, k := range kids {
				if m, isMap := k.(map[string]any); isMap && walk(m) {
					return true
				}
			}
		}
		return false
	}
	return title, width, height, walk(tree)
}

func TestRealWindow(t *testing.T) {
	needCompositor(t)
	start(t, build(t))
	var title string
	var w, h float64
	deadline := time.Now().Add(60 * time.Second)
	for {
		var ok bool
		if title, w, h, ok = windowOf(t); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the window did not appear")
		}
		time.Sleep(250 * time.Millisecond)
	}
	if title != Title {
		t.Errorf("window title %q, want %q", title, Title)
	}
	min := windowConfig().MinSize
	if w < min.Width || h < min.Height {
		t.Errorf("window %vx%v is smaller than the minimum %vx%v", w, h, min.Width, min.Height)
	}
}

// TestRealWindowScreenshot has Glutter save the first window after the fixtures
// were preloaded: it must be the Review screen, not blank.
func TestRealWindowScreenshot(t *testing.T) {
	needCompositor(t)
	shot := filepath.Join(t.TempDir(), "window.png")
	cmd := start(t, build(t), "GLUTTER_SCREENSHOT="+shot, "GLUTTER_SCREENSHOT_AFTER=2s")
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("the application did not save the screenshot and exit")
	}
	data, err := os.ReadFile(shot)
	if err != nil || len(data) == 0 {
		t.Fatalf("no screenshot: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() < 640 || b.Dy() < 480 {
		t.Errorf("screenshot is %dx%d", b.Dx(), b.Dy())
	}
	// A blank window is one colour.
	first := img.At(b.Min.X, b.Min.Y)
	different := 0
	for y := b.Min.Y; y < b.Max.Y; y += 7 {
		for x := b.Min.X; x < b.Max.X; x += 7 {
			if img.At(x, y) != first {
				different++
			}
		}
	}
	if different < 200 {
		t.Errorf("the screenshot looks blank (%d points differ from the corner)", different)
	}
}
