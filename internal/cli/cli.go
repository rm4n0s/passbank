package cli

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/rm4n0s/passbank/internal/version"
)

// Exit statuses of Main (SPEC §12).
const (
	// ExitOK is success.
	ExitOK = 0
	// ExitFailure is a command that failed.
	ExitFailure = 1
	// ExitUsage is a mistake in the arguments.
	ExitUsage = 2
)

// env is what a command gets: the streams and the terminal. Commands never use
// os.Stdin, os.Stdout or the terminal directly, so that tests give fakes.
type env struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	term   Terminal
}

// command is one subcommand: its one-line description and its body, which
// receives the arguments after its name and returns the exit status.
type command struct {
	summary string
	run     func(ctx context.Context, e env, args []string) int
}

// commands returns the subcommands by name. Steps 2.8, 6.5 and 7.4 add info,
// diff and merge.
func commands() map[string]command {
	return map[string]command{}
}

// Main is the body of cmd/passbank: it runs the command named by args (the
// arguments without the program name) and returns the exit status. term is the
// terminal the user answers on; tests give a fake.
func Main(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, term Terminal) int {
	e := env{stdin: stdin, stdout: stdout, stderr: stderr, term: term}
	out, errs := &printer{w: stdout}, &printer{w: stderr}
	if len(args) == 0 {
		printUsage(errs)
		return ExitUsage
	}
	switch args[0] {
	case "--version", "-version":
		out.println("passbank", version.String())
		return out.status(errs)
	case "--help", "-help", "-h", "help":
		printUsage(out)
		return out.status(errs)
	}
	cmd, ok := commands()[args[0]]
	if !ok {
		errs.printf("passbank: unknown command %q; run passbank --help\n", args[0])
		return ExitUsage
	}
	return cmd.run(ctx, e, args[1:])
}

// printer writes to a stream and remembers the first write error, so that the
// code that prints several lines checks once at the end.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

func (p *printer) println(args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintln(p.w, args...)
	}
}

// status is ExitOK when everything printed, and ExitFailure when a write failed
// (a closed pipe, a full disk); it then says so on errs, which may fail as well,
// and there is nowhere else to say it.
func (p *printer) status(errs *printer) int {
	if p.err == nil {
		return ExitOK
	}
	errs.printf("passbank: cannot write the output: %v\n", p.err)
	return ExitFailure
}

// printUsage writes the help text.
func printUsage(w *printer) {
	w.println("passbank merges two KeePass databases into a new one.")
	w.println()
	w.println("usage: passbank [--version] [--help] <command> [arguments]")
	w.println()
	cmds := commands()
	if len(cmds) == 0 {
		w.println("No command is available yet.")
		return
	}
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)
	w.println("commands:")
	for _, name := range names {
		w.printf("  %-8s %s\n", name, cmds[name].summary)
	}
}
