package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/rm4n0s/errors"
	"golang.org/x/term"

	"github.com/rm4n0s/passbank/internal/core/secret"
)

// Terminal is the terminal the user answers on. Passwords and choices are read
// from the terminal and never from stdin, so that stdin and stdout stay free for
// pipes (SPEC §12). Tests give a fake.
type Terminal interface {
	// IsTerminal reports whether there is a terminal to ask the user on.
	IsTerminal() bool
	// ReadPassword shows prompt and reads a line without echoing it.
	ReadPassword(prompt string) (secret.Bytes, error)
	// ReadLine shows prompt and reads a line of text, echoed as it is typed, without
	// the line break. At the end of the input (Ctrl-D) it returns io.EOF.
	ReadLine(prompt string) (string, error)
	// Write shows text to the user on the terminal. The interactive choices use it,
	// so that stdout stays free for the summary.
	Write(p []byte) (int, error)
}

// ttyPath is the controlling terminal of the process.
const ttyPath = "/dev/tty"

// TTY is the Terminal on /dev/tty.
type TTY struct {
	open func() (*os.File, error)
}

// NewTTY returns the Terminal on the controlling terminal of the process.
func NewTTY() *TTY {
	return &TTY{open: func() (*os.File, error) { return os.OpenFile(ttyPath, os.O_RDWR, 0) }}
}

// IsTerminal reports whether /dev/tty can be opened and is a terminal.
func (t *TTY) IsTerminal() bool {
	f, err := t.open()
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }() // nothing was written, so there is nothing to lose
	return term.IsTerminal(int(f.Fd()))
}

// ReadPassword writes prompt to the terminal and reads a line without echo.
func (t *TTY) ReadPassword(prompt string) (secret.Bytes, error) {
	f, err := t.open()
	if err != nil {
		return secret.Bytes{}, errors.NewErr("TerminalOpenFailed", err).SetMetadata("path", ttyPath)
	}
	defer func() { _ = f.Close() }() // a failed close of a terminal changes nothing for the caller
	if _, err := fmt.Fprint(f, prompt); err != nil {
		return secret.Bytes{}, errors.NewErr("TerminalWriteFailed", err)
	}
	line, err := term.ReadPassword(int(f.Fd()))
	// The user's Enter was not echoed either; move the cursor to the next line.
	if _, werr := fmt.Fprintln(f); werr != nil && err == nil {
		err = werr
	}
	if err != nil {
		return secret.Bytes{}, errors.NewErr("PasswordReadFailed", err)
	}
	return secret.New(line), nil
}

// ReadLine writes prompt to the terminal and reads a line. It reads the terminal
// byte by byte, so that nothing after the line is consumed.
func (t *TTY) ReadLine(prompt string) (string, error) {
	f, err := t.open()
	if err != nil {
		return "", errors.NewErr("TerminalOpenFailed", err).SetMetadata("path", ttyPath)
	}
	defer func() { _ = f.Close() }() // a failed close of a terminal changes nothing for the caller
	if _, err := fmt.Fprint(f, prompt); err != nil {
		return "", errors.NewErr("TerminalWriteFailed", err)
	}
	var line []byte
	var b [1]byte
	for {
		n, err := f.Read(b[:])
		if n == 1 {
			if b[0] == '\n' {
				return string(line), nil
			}
			line = append(line, b[0])
			if len(line) > maxLineLength {
				return "", errors.New("LineTooLong", "the line is too long", "limit", maxLineLength)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) > 0 {
					return string(line), nil
				}
				return "", io.EOF
			}
			return "", errors.NewErr("LineReadFailed", err)
		}
	}
}

// maxLineLength is the longest line ReadLine accepts.
const maxLineLength = 4096

// Write writes text to the terminal.
func (t *TTY) Write(p []byte) (int, error) {
	f, err := t.open()
	if err != nil {
		return 0, errors.NewErr("TerminalOpenFailed", err).SetMetadata("path", ttyPath)
	}
	defer func() { _ = f.Close() }() // a failed close of a terminal changes nothing for the caller
	n, err := f.Write(p)
	if err != nil {
		return n, errors.NewErr("TerminalWriteFailed", err)
	}
	return n, nil
}
