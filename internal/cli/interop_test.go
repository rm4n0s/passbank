//go:build passbank_keepassxc

package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
)

// These tests open the files that `passbank merge` writes in KeePassXC, an
// independent reader:
//
//	go test -tags passbank_keepassxc ./internal/cli
//
// They are skipped when keepassxc-cli is neither in the PATH nor installed as the Flatpak.

func keepassxc(t *testing.T, dir, password string, args ...string) (string, error) {
	t.Helper()
	var cmd *exec.Cmd
	if path, err := exec.LookPath("keepassxc-cli"); err == nil {
		cmd = exec.Command(path, args...)
	} else if _, err := exec.LookPath("flatpak"); err == nil && exec.Command("flatpak", "info", "org.keepassxc.KeePassXC").Run() == nil {
		cmd = exec.Command("flatpak", append([]string{"run", "--filesystem=" + dir, "--command=keepassxc-cli", "org.keepassxc.KeePassXC"}, args...)...)
	} else {
		t.Skip("keepassxc-cli is not installed")
	}
	cmd.Stdin = strings.NewReader(password + "\n")
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return out.String(), errors.NewErr("KeePassXCFailed", err).SetMetadata("stderr", strings.TrimSpace(errs.String()))
		}
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatalf("keepassxc-cli %v timed out", args)
	}
	return out.String(), nil
}

func TestMergedFilesOpenInKeePassXC(t *testing.T) {
	tests := []struct {
		rule, format string
		want         []string // the entries KeePassXC must list, and the password of "My bank"
		bankPassword string
	}{
		{"newer", "4", []string{"Banking/My bank", "Email/Personal", "Email/Work mail", "Games/Old forum", "Shopping/Store", "Social/Stale login"}, "bank-password-b"},
		{"a", "4.1", []string{"Banking/My bank", "Email/Personal", "Email/Work mail", "Shopping/Store", "Social/Stale login"}, "bank-password-a"},
		{"b", "3.1", []string{"Banking/My bank", "Email/Personal", "Games/Old forum", "Shopping/Store (old name)", "Social/Old account"}, "bank-password-b"},
		{"union", "2", []string{"Banking/My bank", "Banking/My bank", "Shopping/Store (old name)", "Email/Personal", "Email/Work mail", "Games/Old forum", "Shopping/Store", "Social/Stale login"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.rule+" as "+tt.format, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "merged.kdbx")
			// The default encryption of the format: Argon2id for 4 and the AES-KDF for the older ones.
			status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, out, "--prefer", tt.rule, "--format", tt.format, "--encryption", "default")...)
			if status != ExitOK {
				t.Fatalf("merge: %d %q", status, errs)
			}
			listing, err := keepassxc(t, dir, modeltest.PairPasswordA, "ls", "-R", "-f", "-q", out)
			if err != nil {
				t.Fatalf("KeePassXC cannot open the file: %v", err)
			}
			var listed []string
			for _, l := range strings.Split(listing, "\n") {
				if l = strings.TrimSpace(l); l != "" && !strings.HasSuffix(l, "/") {
					listed = append(listed, l)
				}
			}
			sort.Strings(listed)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if strings.Join(listed, "\n") != strings.Join(want, "\n") {
				t.Errorf("KeePassXC lists\n%s\nwant\n%s", strings.Join(listed, "\n"), strings.Join(want, "\n"))
			}
			if tt.bankPassword != "" {
				show, err := keepassxc(t, dir, modeltest.PairPasswordA, "show", "-s", "-q", out, "Banking/My bank")
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(show, "Password: "+tt.bankPassword) {
					t.Errorf("KeePassXC shows\n%s\nwant the password %s", show, tt.bankPassword)
				}
			}
		})
	}
}
