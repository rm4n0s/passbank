package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/rm4n0s/errors"
)

// openPTY opens a pseudo-terminal and returns its master and slave sides; the test
// is skipped where there is none.
func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal here: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	var unlock int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); e != 0 {
		t.Skipf("cannot unlock the pseudo-terminal: %v", e)
	}
	var n uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); e != 0 {
		t.Skipf("cannot name the pseudo-terminal: %v", e)
	}
	slave, err = os.OpenFile("/dev/pts/"+itoaN(int(n)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("cannot open the pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return master, slave
}

func itoaN(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// ttyOn is a TTY whose terminal is the given file; every open gives a new
// descriptor of the same terminal, as /dev/tty does.
func ttyOn(t *testing.T, f *os.File) *TTY {
	t.Helper()
	return &TTY{open: func() (*os.File, error) {
		fd, err := syscall.Dup(int(f.Fd()))
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(fd), f.Name()), nil
	}}
}

func TestTTYOnAPseudoTerminal(t *testing.T) {
	master, slave := openPTY(t)
	tty := ttyOn(t, slave)
	if !tty.IsTerminal() {
		t.Fatal("a pseudo-terminal is a terminal")
	}
	// The user types two lines and a password; the terminal echoes them to the master.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(master, "first line\nsecond line\n")
	}()
	got1, err := tty.ReadLine("one> ")
	if err != nil || got1 != "first line" {
		t.Fatalf("ReadLine = %q, %v", got1, err)
	}
	got2, err := tty.ReadLine("two> ")
	if err != nil || got2 != "second line" {
		t.Fatalf("ReadLine = %q, %v (the first read must not take more than its line)", got2, err)
	}
	if _, err := tty.Write([]byte("shown to the user\n")); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(master, "s3cret\n")
	}()
	pw, err := tty.ReadPassword("password> ")
	if err != nil || string(pw.Reveal()) != "s3cret" {
		t.Fatalf("ReadPassword: %v", err)
	}
	// What the master saw: the prompts and the text, and the typed lines echoed, but
	// not the password. The master cannot be read with a deadline, so a goroutine
	// drains it until the test closes it.
	var mu sync.Mutex
	var seen strings.Builder
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			mu.Lock()
			seen.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{"one> ", "two> ", "shown to the user", "password> "} {
		if !strings.Contains(seen.String(), want) {
			t.Errorf("the terminal did not show %q: %q", want, seen.String())
		}
	}
	if strings.Contains(seen.String(), "s3cret") {
		t.Error("the password was echoed")
	}
}

func fileTTY(t *testing.T, content string) *TTY {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tty")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	// One descriptor, shared offset: the prompt overwrites the start of the file and
	// the line is read from what follows.
	return &TTY{open: func() (*os.File, error) {
		fd, err := syscall.Dup(int(f.Fd()))
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(fd), path), nil
	}}
}

func TestTTYReadLineEdges(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		err     string // route suffix, "" for none, "EOF" for io.EOF
	}{
		{"a partial last line", "  abc", "abc", ""},
		{"a line", "  hello\nrest", "hello", ""},
		{"an empty line", "  \nrest", "", ""},
		{"the end of the input", "  ", "", "EOF"},
		{"a line that is too long", "  " + strings.Repeat("x", maxLineLength+10), "", "LineTooLong"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fileTTY(t, tt.content).ReadLine("> ")
			switch tt.err {
			case "":
				if err != nil || got != tt.want {
					t.Errorf("ReadLine = %q, %v, want %q", got, err, tt.want)
				}
			case "EOF":
				if !errors.Is(err, io.EOF) {
					t.Errorf("err = %v, want io.EOF", err)
				}
			default:
				appErr, ok := errors.FromError(err)
				if !ok || !appErr.HasRoute("TTY.ReadLine."+tt.err) {
					t.Errorf("err = %v", err)
				}
			}
		})
	}
}

func TestTTYFailures(t *testing.T) {
	broken := &TTY{open: func() (*os.File, error) { return nil, os.ErrPermission }}
	if _, err := broken.ReadLine("> "); !routeIs(err, "TTY.ReadLine.TerminalOpenFailed") {
		t.Errorf("ReadLine: %v", err)
	}
	if _, err := broken.Write([]byte("x")); !routeIs(err, "TTY.Write.TerminalOpenFailed") {
		t.Errorf("Write: %v", err)
	}
	// A terminal that cannot be written: a file opened for reading only.
	readOnly := &TTY{open: func() (*os.File, error) { return os.Open(os.DevNull) }}
	if _, err := readOnly.ReadLine("> "); !routeIs(err, "TTY.ReadLine.TerminalWriteFailed") {
		t.Errorf("ReadLine on a read-only terminal: %v", err)
	}
	if _, err := readOnly.Write([]byte("x")); !routeIs(err, "TTY.Write.TerminalWriteFailed") {
		t.Errorf("Write on a read-only terminal: %v", err)
	}
	// A terminal that cannot be read: a file opened for writing only.
	writeOnly := &TTY{open: func() (*os.File, error) { return os.OpenFile(os.DevNull, os.O_WRONLY, 0) }}
	if _, err := writeOnly.ReadLine("> "); !routeIs(err, "TTY.ReadLine.LineReadFailed") {
		t.Errorf("ReadLine on a write-only terminal: %v", err)
	}
}

func routeIs(err error, route string) bool {
	appErr, ok := errors.FromError(err)
	return ok && appErr.HasRoute(route)
}
