package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/secret"
)

// fakeTerminal is the Terminal of the tests.
type fakeTerminal struct{}

func (fakeTerminal) IsTerminal() bool                { return false }
func (fakeTerminal) ReadLine(string) (string, error) { return "", io.EOF }
func (fakeTerminal) Write(p []byte) (int, error)     { return len(p), nil }
func (fakeTerminal) ReadPassword(string) (secret.Bytes, error) {
	return secret.Bytes{}, nil
}

func TestMainDispatch(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantOut    string // a substring of stdout
		wantErr    string // a substring of stderr
	}{
		{"no arguments", nil, ExitUsage, "", "usage: passbank"},
		{"version", []string{"--version"}, ExitOK, "passbank ", ""},
		{"version with one dash", []string{"-version"}, ExitOK, "passbank ", ""},
		{"help", []string{"--help"}, ExitOK, "usage: passbank", ""},
		{"help command", []string{"help"}, ExitOK, "merges two KeePass databases", ""},
		{"short help", []string{"-h"}, ExitOK, "usage: passbank", ""},
		{"unknown command", []string{"frobnicate"}, ExitUsage, "", `unknown command "frobnicate"`},
		{"unknown flag", []string{"--frobnicate"}, ExitUsage, "", `unknown command "--frobnicate"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			status := Main(context.Background(), tt.args, strings.NewReader(""), &out, &errs, fakeTerminal{})
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
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

// failingWriter fails every write, like a closed pipe.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestMainWriteFailure(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"--help"}} {
		var errs bytes.Buffer
		status := Main(context.Background(), args, strings.NewReader(""), failingWriter{}, &errs, fakeTerminal{})
		if status != ExitFailure {
			t.Errorf("%v: status = %d, want %d when stdout cannot be written", args, status, ExitFailure)
		}
		if !strings.Contains(errs.String(), "cannot write the output") {
			t.Errorf("%v: stderr = %q, want it to say the output cannot be written", args, errs.String())
		}
	}
}

// assertRoute fails the test unless err is an *errors.Error whose route contains route.
func assertRoute(t *testing.T, err error, route string) {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
}

// pipeTTY returns a TTY whose "terminal" is the read end of a pipe, which is a
// file that is not a terminal.
func pipeTTY(t *testing.T) *TTY {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return &TTY{open: func() (*os.File, error) { return r, nil }}
}

func TestTTYWithoutATerminal(t *testing.T) {
	if pipeTTY(t).IsTerminal() {
		t.Error("a pipe is not a terminal")
	}
	missing := &TTY{open: func() (*os.File, error) { return nil, os.ErrNotExist }}
	if missing.IsTerminal() {
		t.Error("IsTerminal must be false when /dev/tty cannot be opened")
	}
}

func TestTTYRoutes(t *testing.T) {
	t.Run("TerminalOpenFailed", func(t *testing.T) {
		tty := &TTY{open: func() (*os.File, error) { return nil, os.ErrPermission }}
		_, err := tty.ReadPassword("Password: ")
		assertRoute(t, err, "TTY.ReadPassword.TerminalOpenFailed")
	})
	t.Run("TerminalWriteFailed", func(t *testing.T) {
		// A file opened for reading only: the prompt cannot be written to it.
		tty := &TTY{open: func() (*os.File, error) { return os.Open(os.DevNull) }}
		_, err := tty.ReadPassword("Password: ")
		assertRoute(t, err, "TTY.ReadPassword.TerminalWriteFailed")
	})
	t.Run("PasswordReadFailed", func(t *testing.T) {
		// /dev/null accepts the prompt but is not a terminal, so there is nothing to
		// read without echo from.
		tty := &TTY{open: func() (*os.File, error) { return os.OpenFile(os.DevNull, os.O_RDWR, 0) }}
		_, err := tty.ReadPassword("Password: ")
		assertRoute(t, err, "TTY.ReadPassword.PasswordReadFailed")
	})
}
