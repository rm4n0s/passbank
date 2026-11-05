package cli

import (
	"bytes"
	"context"
	"flag"
	"io"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

const diffUsage = "usage: passbank diff [--prefer newer|a|b|union] [-o PLAN] [--force] [--no-titles] [--debug]\n" +
	"                     [--keyfile-a PATH] [--password-file-a PATH] [--no-password-a]\n" +
	"                     [--keyfile-b PATH] [--password-file-b PATH] [--no-password-b] A B"

// runDiff is `passbank diff A B`: it opens both databases, compares them and
// writes the plan with the defaults (or the choices of a rule) to the file given
// with -o, or to stdout. A summary of the changes goes to stderr.
func runDiff(ctx context.Context, e env, args []string) int {
	out, errs := &printer{w: e.stdout}, &printer{w: e.stderr}
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	credsA := addCredentialFlags(fs, "-a")
	credsB := addCredentialFlags(fs, "-b")
	prefer := fs.String("prefer", string(plan.Newer), "the rule that sets the choices: newer, a, b or union")
	planPath := fs.String("o", "", "write the plan to this file instead of stdout")
	force := fs.Bool("force", false, "replace the plan file if it exists")
	noTitles := fs.Bool("no-titles", false, "leave the group paths and titles out of the plan's comments")
	debug := fs.Bool("debug", false, "show the route of an error")
	files, err := parseInterleaved(fs, args)
	if err == nil && len(files) != 2 {
		err = errors.New("UsageInvalid", "diff needs two databases, A and B")
	}
	rule, ok := plan.ParseRule(*prefer)
	if err == nil && !ok {
		err = errors.New("UsageInvalid", "--prefer must be newer, a, b or union", "value", *prefer)
	}
	if err == nil {
		err = credsA.check()
	}
	if err == nil {
		err = credsB.check()
	}
	if err != nil {
		reportError(errs, "diff", "", err, false)
		errs.println(diffUsage)
		return ExitUsage
	}

	if *planPath != "" {
		if err := plan.CheckPath(*planPath, files...); err != nil {
			return reportError(errs, "diff", *planPath, err, *debug)
		}
	}

	a, err := openInput(ctx, e, credsA, files[0])
	if err != nil {
		return reportError(errs, "diff", files[0], err, *debug)
	}
	defer a.wipe()
	b, err := openInput(ctx, e, credsB, files[1])
	if err != nil {
		return reportError(errs, "diff", files[1], err, *debug)
	}
	defer b.wipe()

	changes := diff.Changes(a.db, b.db)
	p := plan.New(a.planInput(), b.planInput(), changes)
	plan.ApplyRule(p, rule, nil)

	var text bytes.Buffer
	if err := plan.Write(&text, p, plan.WriteOptions{NoTitles: *noTitles}); err != nil {
		return reportError(errs, "diff", "", err, *debug)
	}
	if *planPath == "" {
		out.printf("%s", text.Bytes())
	} else if err := plan.WriteFile(ctx, *planPath, text.Bytes(), *force); err != nil {
		return reportError(errs, "diff", *planPath, err, *debug)
	}
	printChangeSummary(errs, a, b, changes)
	if *planPath != "" {
		errs.printf("plan written to %s\n", *planPath)
	}
	return out.status(errs)
}

// printChangeSummary says how many changes of each kind there are.
func printChangeSummary(w *printer, a, b *input, changes []diff.Change) {
	w.printf("A: %s\nB: %s\n", a.path, b.path)
	if len(changes) == 0 {
		w.println("no changes: the databases hold the same")
		return
	}
	counts := map[diff.Kind]int{}
	for _, c := range changes {
		counts[c.Kind]++
	}
	w.printf("changes: %d\n", len(changes))
	for _, k := range diff.Kinds {
		if counts[k] > 0 {
			w.printf("  %-14s %d\n", k, counts[k])
		}
	}
}
