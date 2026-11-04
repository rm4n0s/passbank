package cli

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/secret"
)

var updateGoldens = flag.Bool("update-goldens", false, "rewrite the golden files of the tests")

// kdbxFixture returns the path of a fixture database of the core, which holds
// the fixtures.
func kdbxFixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "core", "kdbx", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return path
}

// scriptedTerminal is a Terminal that answers with a fixed password and
// remembers what it was asked.
type scriptedTerminal struct {
	tty      bool
	password string
	err      error
	prompts  []string
}

func (s *scriptedTerminal) IsTerminal() bool { return s.tty }

func (s *scriptedTerminal) ReadPassword(prompt string) (secret.Bytes, error) {
	s.prompts = append(s.prompts, prompt)
	if s.err != nil {
		return secret.Bytes{}, s.err
	}
	return secret.New([]byte(s.password)), nil
}

// run runs the CLI in-process and returns the status, stdout and stderr.
func run(t *testing.T, term Terminal, args ...string) (int, string, string) {
	t.Helper()
	var out, errs bytes.Buffer
	status := Main(context.Background(), args, strings.NewReader(""), &out, &errs, term)
	return status, out.String(), errs.String()
}

// checkGolden compares got with testdata/golden/name, or rewrites it with -update-goldens.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *updateGoldens {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the golden file:\n--- got\n%s--- want\n%s", name, got, want)
	}
}

func TestInfoGolden(t *testing.T) {
	tests := []struct {
		golden, file string
		args         []string
		password     string
	}{
		{"info-k4-rich.txt", "k4-rich.kdbx", nil, "fixture-rich"},
		{"info-k4-rich-twofish-aeskdf.txt", "k4-rich-twofish-aeskdf.kdbx", nil, "fixture-rich"},
		{"info-k4-rich-argon2id.txt", "k4-rich-argon2id.kdbx", nil, "fixture-rich"},
		{"info-k4-rich-chacha20.txt", "k4-rich-chacha20.kdbx", nil, "fixture-rich"},
		{"info-k41-fields.txt", "k41-fields.kdbx", nil, "fixture-rich"},
		{"info-k4-empty.txt", "k4-empty.kdbx", nil, "fixture-min"},
		{"info-k4-rich-nocompression.txt", "k4-rich-nocompression.kdbx", nil, "fixture-rich"},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			term := &scriptedTerminal{tty: true, password: tt.password}
			path := kdbxFixture(t, tt.file)
			status, out, errs := run(t, term, append([]string{"info", path}, tt.args...)...)
			if status != ExitOK || errs != "" {
				t.Fatalf("status %d, stderr %q", status, errs)
			}
			checkGolden(t, tt.golden, out)
			if len(term.prompts) != 1 || term.prompts[0] != "Password for "+path+": " {
				t.Errorf("prompts = %q", term.prompts)
			}
		})
	}
}

func TestInfoCredentialOptions(t *testing.T) {
	dir := t.TempDir()
	writePasswordFile := func(name, content string, mode os.FileMode) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	passwordFile := writePasswordFile("pw", "fixture-rich\nsecond line is ignored\n", 0o600)
	crlfFile := writePasswordFile("pw-crlf", "fixture-rich\r\n", 0o400)
	noNewline := writePasswordFile("pw-nonl", "fixture-rich", 0o600)
	noTerm := &scriptedTerminal{tty: false}

	tests := []struct {
		name string
		file string
		args []string
	}{
		{"a password file", "k4-rich.kdbx", []string{"--password-file", passwordFile}},
		{"a password file with CRLF, read-only", "k4-rich.kdbx", []string{"--password-file", crlfFile}},
		{"a password file without a newline", "k4-rich.kdbx", []string{"--password-file", noNewline}},
		{"options before the file", "k4-rich.kdbx", []string{"--password-file", passwordFile, "--debug"}},
		{"a key file and no password", "k4-keyfile-xml2.kdbx", []string{"--keyfile", kdbxFixture(t, "keyfile-xml2.keyx"), "--no-password"}},
		{"a key file and a password file", "k4-password-and-keyfile.kdbx", []string{"--keyfile", kdbxFixture(t, "keyfile-raw32.key"), "--password-file", passwordFile}},
	}
	for _, tt := range tests {
		for _, before := range []bool{false, true} {
			name := tt.name
			if before {
				name += ", options first"
			}
			t.Run(name, func(t *testing.T) {
				path := kdbxFixture(t, tt.file)
				args := append([]string{"info"}, tt.args...)
				if before {
					args = append(args, path)
				} else {
					args = append([]string{"info", path}, tt.args...)
				}
				status, out, errs := run(t, noTerm, args...)
				if status != ExitOK || errs != "" {
					t.Fatalf("status %d, stderr %q", status, errs)
				}
				if !strings.Contains(out, "entries          4\n") {
					t.Errorf("stdout = %q", out)
				}
				if len(noTerm.prompts) != 0 {
					t.Errorf("asked on the terminal: %q", noTerm.prompts)
				}
			})
		}
	}
}

func TestInfoFailures(t *testing.T) {
	dir := t.TempDir()
	openFile := filepath.Join(dir, "open-pw")
	if err := os.WriteFile(openFile, []byte("fixture-rich\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(openFile, 0o644); err != nil {
		t.Fatal(err)
	}
	notKdbx := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(notKdbx, []byte("not a database at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	rich := kdbxFixture(t, "k4-rich.kdbx")
	tests := []struct {
		name       string
		term       *scriptedTerminal
		args       []string
		wantStatus int
		wantErr    string // stderr with the paths replaced by FILE and DIR
	}{
		{"a wrong password", &scriptedTerminal{tty: true, password: "wrong"}, []string{"info", rich}, ExitFailure,
			"passbank info: the password or key file for FILE is wrong\n"},
		{"no terminal and no option", &scriptedTerminal{tty: false}, []string{"info", rich}, ExitFailure,
			"passbank info: there is no terminal to ask for the password; use --password-file or --no-password\n"},
		{"an empty password and no key file", &scriptedTerminal{tty: true, password: ""}, []string{"info", rich}, ExitFailure,
			"passbank info: FILE needs a password, a key file, or both; the password is empty and no key file was given\n"},
		{"the terminal fails", &scriptedTerminal{tty: true, err: errors.New("PasswordReadFailed", "the terminal went away")}, []string{"info", rich}, ExitFailure,
			"passbank info: the terminal went away\n"},
		{"a password file that others can read", &scriptedTerminal{}, []string{"info", rich, "--password-file", openFile}, ExitFailure,
			"passbank info: the password file can be read by others; make it readable only by you (chmod 600)\n"},
		{"a missing password file", &scriptedTerminal{}, []string{"info", rich, "--password-file", filepath.Join(dir, "none")}, ExitFailure,
			"passbank info: open DIR/none: no such file or directory\n"},
		{"a missing key file", &scriptedTerminal{}, []string{"info", rich, "--keyfile", filepath.Join(dir, "none.key"), "--no-password"}, ExitFailure,
			"passbank info: open DIR/none.key: no such file or directory\n"},
		{"a missing database", &scriptedTerminal{tty: true, password: "x"}, []string{"info", filepath.Join(dir, "none.kdbx")}, ExitFailure,
			"passbank info: DIR/none.kdbx cannot be read: open DIR/none.kdbx: no such file or directory\n"},
		{"not a database", &scriptedTerminal{tty: true, password: "x"}, []string{"info", notKdbx}, ExitFailure,
			"passbank info: DIR/text.txt is not a KeePass database\n"},
		{"a KDBX 3 file", &scriptedTerminal{tty: true, password: "fixture-rich"}, []string{"info", kdbxFixture(t, "k31-rich.kdbx")}, ExitFailure,
			"passbank info: FILE uses KDBX version 3.1, which Passbank cannot read\n"},
		{"no file", &scriptedTerminal{}, []string{"info"}, ExitUsage,
			"passbank info: info needs exactly one file\nusage: passbank info [--keyfile PATH] [--password-file PATH] [--no-password] [--debug] FILE\n"},
		{"two files", &scriptedTerminal{}, []string{"info", rich, rich}, ExitUsage,
			"passbank info: info needs exactly one file\nusage: passbank info [--keyfile PATH] [--password-file PATH] [--no-password] [--debug] FILE\n"},
		{"an unknown option", &scriptedTerminal{}, []string{"info", "--password", "x", rich}, ExitUsage,
			"passbank info: flag provided but not defined: -password\nusage: passbank info [--keyfile PATH] [--password-file PATH] [--no-password] [--debug] FILE\n"},
		{"contradicting options", &scriptedTerminal{}, []string{"info", rich, "--no-password", "--password-file", "x"}, ExitUsage,
			"passbank info: --no-password and --password-file contradict each other\nusage: passbank info [--keyfile PATH] [--password-file PATH] [--no-password] [--debug] FILE\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, out, errs := run(t, tt.term, tt.args...)
			if status != tt.wantStatus || out != "" {
				t.Errorf("status %d (want %d), stdout %q", status, tt.wantStatus, out)
			}
			got := strings.ReplaceAll(strings.ReplaceAll(errs, rich, "FILE"), kdbxFixture(t, "k31-rich.kdbx"), "FILE")
			got = strings.ReplaceAll(got, dir, "DIR")
			if got != tt.wantErr {
				t.Errorf("stderr = %q\nwant     %q", got, tt.wantErr)
			}
		})
	}
}

func TestInfoDebugShowsTheRoute(t *testing.T) {
	rich := kdbxFixture(t, "k4-rich.kdbx")
	status, _, errs := run(t, &scriptedTerminal{tty: true, password: "wrong"}, "info", "--debug", rich)
	if status != ExitFailure {
		t.Fatalf("status = %d", status)
	}
	for _, want := range []string{"  tag:   WrongCredentials\n", "  route: ", "Open->OpenBytes->readContainer4.WrongCredentials"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errs)
		}
	}
}

func TestInfoWriteFailure(t *testing.T) {
	var errs bytes.Buffer
	status := Main(context.Background(), []string{"info", kdbxFixture(t, "k4-empty.kdbx"), "--debug"}, strings.NewReader(""), failingWriter{}, &errs,
		&scriptedTerminal{tty: true, password: "fixture-min"})
	if status != ExitFailure || !strings.Contains(errs.String(), "cannot write the output") {
		t.Errorf("status %d, stderr %q", status, errs.String())
	}
}

func TestInfoCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errs bytes.Buffer
	status := Main(ctx, []string{"info", kdbxFixture(t, "k4-rich.kdbx")}, strings.NewReader(""), &out, &errs, &scriptedTerminal{tty: true, password: "fixture-rich"})
	if status != ExitFailure || errs.String() != "passbank info: cancelled\n" {
		t.Errorf("status %d, stderr %q", status, errs.String())
	}
}

func TestHelpListsInfo(t *testing.T) {
	status, out, _ := run(t, &scriptedTerminal{}, "--help")
	if status != ExitOK || !strings.Contains(out, "info") || strings.Contains(out, "No command is available") {
		t.Errorf("help = %q", out)
	}
}

func TestFormatBytes(t *testing.T) {
	for n, want := range map[uint64]string{0: "0 B", 1: "1 B", 1000: "1000 B", 1 << 10: "1 KiB", 3 << 20: "3 MiB", 8 << 20: "8 MiB", 1 << 30: "1 GiB", 1<<20 + 1: "1048577 B"} {
		if got := formatBytes(n); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPasswordFileRoutes(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte, mode os.FileMode) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	tests := []struct {
		name  string
		path  string
		route string
	}{
		{"missing", filepath.Join(dir, "none"), "readPasswordFile.PasswordFileReadFailed"},
		{"a folder", dir, "readPasswordFile.PasswordFileReadFailed"},
		{"readable by the group", write("g", []byte("x"), 0o640), "readPasswordFile.PasswordFileTooOpen"},
		{"readable by others", write("o", []byte("x"), 0o604), "readPasswordFile.PasswordFileTooOpen"},
		{"writable by the group", write("w", []byte("x"), 0o620), "readPasswordFile.PasswordFileTooOpen"},
		{"too large", write("big", bytes.Repeat([]byte("a"), maxPasswordFile+1), 0o600), "readPasswordFile.PasswordFileReadFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readPasswordFile(context.Background(), tt.path)
			assertRoute(t, err, tt.route)
		})
	}
	t.Run("at the size limit", func(t *testing.T) {
		pw, err := readPasswordFile(context.Background(), write("max", bytes.Repeat([]byte("a"), maxPasswordFile), 0o600))
		if err != nil || pw.Len() != maxPasswordFile {
			t.Errorf("%d bytes, %v", pw.Len(), err)
		}
	})
	t.Run("only the first line", func(t *testing.T) {
		pw, err := readPasswordFile(context.Background(), write("lines", []byte("one\r\ntwo\n"), 0o600))
		if err != nil || string(pw.Reveal()) != "one" {
			t.Errorf("%q, %v", pw.Reveal(), err)
		}
	})
	t.Run("an empty file", func(t *testing.T) {
		pw, err := readPasswordFile(context.Background(), write("empty", nil, 0o600))
		if err != nil || pw.Len() != 0 {
			t.Errorf("%q, %v", pw.Reveal(), err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := readPasswordFile(ctx, write("c", []byte("x"), 0o600))
		assertRoute(t, err, "readPasswordFile.ReadCancelled")
	})
}

func TestCredentialRoutes(t *testing.T) {
	t.Run("PasswordNeedsTerminal", func(t *testing.T) {
		_, err := (&credentialOptions{}).load(context.Background(), env{term: &scriptedTerminal{}}, "db")
		assertRoute(t, err, "load.PasswordNeedsTerminal")
	})
	t.Run("PasswordNeedsTerminal without any terminal value", func(t *testing.T) {
		_, err := (&credentialOptions{}).load(context.Background(), env{}, "db")
		assertRoute(t, err, "load.PasswordNeedsTerminal")
	})
	t.Run("UsageInvalid", func(t *testing.T) {
		assertRoute(t, (&credentialOptions{noPassword: true, passwordFile: "x", suffix: "-a"}).check(), "check.UsageInvalid")
		if err := (&credentialOptions{noPassword: true}).check(); err != nil {
			t.Error(err)
		}
	})
	t.Run("a key file that is missing is not read before the terminal", func(t *testing.T) {
		_, err := (&credentialOptions{keyFile: filepath.Join(t.TempDir(), "none")}).load(context.Background(), env{term: &scriptedTerminal{tty: true}}, "db")
		if err == nil {
			t.Error("a missing key file was accepted")
		}
	})
}
