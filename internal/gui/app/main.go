package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"

	"github.com/rm4n0s/passbank/internal/version"
)

// Main is the body of cmd/passbank-gui: it reads the arguments (without the
// program name), runs the application, and returns the exit status: 0 on
// success, 1 when the application failed, 2 for a usage mistake. --version and
// --help print and return without opening a window.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("passbank-gui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() {
		// The help goes to stderr, where a failure to write has nowhere else to be reported.
		_, _ = fmt.Fprint(stderr, "passbank-gui merges two KeePass databases into a new one.\n\nusage: passbank-gui [--version] [--help]\n")
		fs.PrintDefaults()
	}
	switch err := fs.Parse(args); {
	case err == flag.ErrHelp:
		return 0
	case err != nil:
		return 2
	}
	if fs.NArg() > 0 {
		// A failure to write the message has nowhere else to be reported.
		_, _ = fmt.Fprintf(stderr, "passbank-gui: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *showVersion {
		if _, err := fmt.Fprintln(stdout, "passbank-gui", version.String()); err != nil {
			_, _ = fmt.Fprintf(stderr, "passbank-gui: cannot write the output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := Run(ctx); err != nil {
		slog.New(slog.NewTextHandler(stderr, nil)).Error("passbank-gui failed", "err", err)
		return 1
	}
	return 0
}
