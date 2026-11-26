package app

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/gui/session"
)

// DevOptions preload the two databases for display tests and proof screenshots. The
// flags that fill them (--dev-a, --dev-b, --dev-password-file) exist only in debug
// builds (-tags debug_mode), never in a release build, and no build takes a
// password as an argument: the password file holds the passwords of test
// fixtures.
type DevOptions struct {
	// A and B are the paths of the databases.
	A, B string
	// PasswordFile holds the password of A on its first line and that of B on its
	// second; with one line only, both have that password.
	PasswordFile string
}

// Active says whether any option is set.
func (d DevOptions) Active() bool { return d.A != "" || d.B != "" || d.PasswordFile != "" }

// Complete says whether every option is set, which preloading needs.
func (d DevOptions) Complete() bool { return d.A != "" && d.B != "" && d.PasswordFile != "" }

// maxDevPasswordFile bounds the file that is read.
const maxDevPasswordFile = 64 << 10

// readDevPasswords reads the passwords of A and B from the file.
func readDevPasswords(path string) (a, b string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", errors.NewErr("DevPasswordFileFailed", err).SetMetadata("path", path)
	}
	defer func() { _ = f.Close() }() // the file was only read
	var lines []string
	sc := bufio.NewScanner(io.LimitReader(f, maxDevPasswordFile))
	for sc.Scan() && len(lines) < 2 {
		lines = append(lines, strings.TrimSuffix(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return "", "", errors.NewErr("DevPasswordFileFailed", err).SetMetadata("path", path)
	}
	switch len(lines) {
	case 0:
		return "", "", errors.New("DevPasswordFileFailed", "the password file is empty", "path", path)
	case 1:
		return lines[0], lines[0], nil
	}
	return lines[0], lines[1], nil
}

// preload unlocks the two databases off the UI thread and, when both open, puts
// them in the session and shows the Review screen. A failure is logged, and the
// application stays on the Open screen.
func (s *appState) preload(dev DevOptions) {
	pa, pb, err := readDevPasswords(dev.PasswordFile)
	if err != nil {
		slog.Error("passbank-gui: cannot preload the databases", "err", err)
		return
	}
	// GoErr hands the unlocking off through errors.Async on the UI thread, so the
	// routes of its errors start at preload. Both failures are returned, joined, and
	// each is logged. A failure wipes both sources inside the work, which also runs
	// when the state was disposed and done is skipped.
	var a, b *session.Source
	s.GoErr(func(ctx context.Context) error {
		var errA, errB error
		a, errA = session.Unlock(ctx, dev.A, secret.New([]byte(pa)), "")
		b, errB = session.Unlock(ctx, dev.B, secret.New([]byte(pb)), "")
		err := errors.Join(errA, errB)
		if err != nil {
			a.Wipe()
			b.Wipe()
		}
		return err
	}, func(err error) {
		if err != nil {
			for _, e := range errors.All(err) {
				slog.Error("passbank-gui: cannot preload a database", "err", e)
			}
			return
		}
		s.SetState(func() {
			s.sess.SetA(a)
			s.sess.SetB(b)
			s.screen = screenReview
		})
	})
}
