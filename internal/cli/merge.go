package cli

import (
	"context"
	"flag"
	"io"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/cli/review"
	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/merge"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/output"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

const mergeUsage = "usage: passbank merge A B -o OUT [--plan PLAN | --prefer newer|a|b|union]\n" +
	"                     [--format 4|4.1|4.0|3.1|3.0|2] [--allow-loss] [--force]\n" +
	"                     [--settings-from a|b] [--credentials a|b|new] [--encryption a|b|default] [--debug]\n" +
	"                     [--keyfile-a PATH] [--password-file-a PATH] [--no-password-a]\n" +
	"                     [--keyfile-b PATH] [--password-file-b PATH] [--no-password-b]\n" +
	"                     [--keyfile-out PATH] [--password-file-out PATH] [--no-password-out]"

// mergeOptions are the parsed options of `passbank merge`.
type mergeOptions struct {
	files        []string
	out          string
	planPath     string
	prefer       plan.Rule
	hasPrefer    bool
	format       string
	allowLoss    bool
	force        bool
	settingsFrom string
	credentials  string
	encryption   string
	debug        bool
	credsA       *credentialOptions
	credsB       *credentialOptions
	credsOut     *credentialOptions
}

// parseMergeFlags parses the arguments of merge. A mistake is a UsageInvalid error.
func parseMergeFlags(args []string) (*mergeOptions, error) {
	o := &mergeOptions{}
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	o.credsA = addCredentialFlags(fs, "-a")
	o.credsB = addCredentialFlags(fs, "-b")
	o.credsOut = addCredentialFlags(fs, "-out")
	o.credsOut.confirm = true
	prefer := fs.String("prefer", "", "the rule that sets the choices: newer, a, b or union")
	fs.StringVar(&o.out, "o", "", "the new database to write")
	fs.StringVar(&o.planPath, "plan", "", "a plan file with the choices")
	fs.StringVar(&o.format, "format", "4", "the KDBX version to write: 4, 4.1, 4.0, 3.1, 3.0 or 2")
	fs.BoolVar(&o.allowLoss, "allow-loss", false, "write the version even when it cannot store everything")
	fs.BoolVar(&o.force, "force", false, "replace the output file if it exists")
	fs.StringVar(&o.settingsFrom, "settings-from", "", "take the database settings from a or b (default: as the plan says)")
	fs.StringVar(&o.credentials, "credentials", "a", "the credentials of the new database: a, b or new")
	fs.StringVar(&o.encryption, "encryption", "default", "the encryption of the new database: a, b or default")
	fs.BoolVar(&o.debug, "debug", false, "show the route of an error")
	files, err := parseInterleaved(fs, args)
	if err != nil {
		return nil, err
	}
	o.files = files
	switch {
	case len(files) != 2:
		return nil, errors.New("UsageInvalid", "merge needs two databases, A and B")
	case o.out == "":
		return nil, errors.New("UsageInvalid", "merge needs the new database to write: -o OUT")
	case o.planPath != "" && *prefer != "":
		return nil, errors.New("UsageInvalid", "--plan and --prefer contradict each other")
	}
	if *prefer != "" {
		rule, ok := plan.ParseRule(*prefer)
		if !ok {
			return nil, errors.New("UsageInvalid", "--prefer must be newer, a, b or union", "value", *prefer)
		}
		o.prefer, o.hasPrefer = rule, true
	}
	if !oneOf(o.settingsFrom, "", "a", "b") {
		return nil, errors.New("UsageInvalid", "--settings-from must be a or b", "value", o.settingsFrom)
	}
	if !oneOf(o.credentials, "a", "b", "new") {
		return nil, errors.New("UsageInvalid", "--credentials must be a, b or new", "value", o.credentials)
	}
	if !oneOf(o.encryption, "a", "b", "default") {
		return nil, errors.New("UsageInvalid", "--encryption must be a, b or default", "value", o.encryption)
	}
	if _, _, err := parseFormat(o.format); err != nil {
		return nil, err
	}
	for _, c := range []*credentialOptions{o.credsA, o.credsB, o.credsOut} {
		if err := c.check(); err != nil {
			return nil, err
		}
	}
	if o.credentials != "new" && (o.credsOut.keyFile != "" || o.credsOut.passwordFile != "" || o.credsOut.noPassword) {
		return nil, errors.New("UsageInvalid", "--keyfile-out, --password-file-out and --no-password-out need --credentials new")
	}
	return o, nil
}

func oneOf(s string, options ...string) bool {
	for _, o := range options {
		if s == o {
			return true
		}
	}
	return false
}

// parseFormat reads the --format value: the version, and whether it is "4", which
// means 4.0, or 4.1 when the database needs it (SPEC §5).
func parseFormat(s string) (v kdbx.Version, auto bool, err error) {
	switch s {
	case "4":
		return kdbx.Version{Major: 4, Minor: 0}, true, nil
	case "4.0":
		return kdbx.Version{Major: 4, Minor: 0}, false, nil
	case "4.1":
		return kdbx.Version{Major: 4, Minor: 1}, false, nil
	case "3.1":
		return kdbx.Version{Major: 3, Minor: 1}, false, nil
	case "3.0":
		return kdbx.Version{Major: 3, Minor: 0}, false, nil
	case "2", "2.0":
		return kdbx.Version{Major: 2, Minor: 0}, false, nil
	}
	return kdbx.Version{}, false, errors.New("UsageInvalid", "--format must be 4, 4.1, 4.0, 3.1, 3.0 or 2", "value", s)
}

// runMerge is `passbank merge A B -o OUT`: it opens both databases, takes the
// choices from a plan file or a rule, builds the merged database and writes it as
// a new file.
func runMerge(ctx context.Context, e env, args []string) int {
	out, errs := &printer{w: e.stdout}, &printer{w: e.stderr}
	o, err := parseMergeFlags(args)
	if err != nil {
		reportError(errs, "merge", "", err, false)
		errs.println(mergeUsage)
		return ExitUsage
	}
	interactive := o.planPath == "" && !o.hasPrefer
	if interactive {
		if err := needTerminal(e); err != nil {
			return reportError(errs, "merge", "", err, o.debug)
		}
	}

	a, err := openInput(ctx, e, o.credsA, o.files[0])
	if err != nil {
		return reportError(errs, "merge", o.files[0], err, o.debug)
	}
	defer a.wipe()
	b, err := openInput(ctx, e, o.credsB, o.files[1])
	if err != nil {
		return reportError(errs, "merge", o.files[1], err, o.debug)
	}
	defer b.wipe()

	changes := diff.Changes(a.db, b.db)
	p, err := choicesFor(ctx, o, a, b, changes)
	if err != nil {
		return reportError(errs, "merge", o.planPath, err, o.debug)
	}
	if interactive {
		outcome, err := review.Run(e.term, p, review.Options{Out: o.out})
		if err != nil {
			return reportError(errs, "merge", "", err, o.debug)
		}
		if outcome == review.Quit {
			errs.println("passbank merge: stopped; nothing was written")
			return ExitFailure
		}
	}
	if o.settingsFrom != "" {
		setSettings(p, o.settingsFrom)
	}
	return finishMerge(ctx, e, o, a, b, p, out, errs)
}

// needTerminal fails with ChoicesNeedTerminal when there is no terminal to ask on.
func needTerminal(e env) error {
	if e.term == nil || !e.term.IsTerminal() {
		return errors.New("ChoicesNeedTerminal", "there is no terminal to ask for choices; use --plan or --prefer")
	}
	return nil
}

// choicesFor makes the plan: read from the plan file, or from the rule. Without
// either it holds the defaults, for the interactive review to change.
func choicesFor(ctx context.Context, o *mergeOptions, a, b *input, changes []diff.Change) (*plan.Plan, error) {
	if o.planPath == "" {
		p := plan.New(a.planInput(), b.planInput(), changes)
		if o.hasPrefer {
			plan.ApplyRule(p, o.prefer, nil)
		}
		return p, nil
	}
	return plan.ReadFile(ctx, o.planPath, a.planInput(), b.planInput(), changes)
}

// setSettings sets the choice of the settings change to the side given.
func setSettings(p *plan.Plan, from string) {
	choice := plan.TakeA
	if from == "b" {
		choice = plan.TakeB
	}
	for i, c := range p.Changes {
		if c.Kind == diff.Settings {
			p.Choices[i] = choice
		}
	}
}

// finishMerge applies the plan and writes the result.
func finishMerge(ctx context.Context, e env, o *mergeOptions, a, b *input, p *plan.Plan, out, errs *printer) int {
	res, err := merge.Apply(a.db, b.db, p)
	if err != nil {
		return reportError(errs, "merge", "", err, o.debug)
	}
	defer res.Wipe()

	version, auto, _ := parseFormat(o.format) // checked by parseMergeFlags
	if auto {
		version = kdbx.MinVersion(res)
	}
	settings := outputSettings(o.encryption, a, b, version)
	creds, err := outputCredentials(ctx, e, o, a, b)
	if err != nil {
		return reportError(errs, "merge", o.out, err, o.debug)
	}
	if o.credentials == "new" {
		defer creds.Wipe()
	}
	err = output.Write(ctx, o.out, res, version, settings, creds, output.Options{AllowLoss: o.allowLoss, Overwrite: o.force, Inputs: o.files})
	if err != nil {
		return reportError(errs, "merge", o.out, err, o.debug)
	}
	printMergeSummary(out, o, a, b, p, res, version)
	return out.status(errs)
}

// outputSettings picks the encryption of the new database: that of A or B (when
// the version can hold it, checked by the writer) or the defaults of the version.
func outputSettings(choice string, a, b *input, version kdbx.Version) kdbx.EncryptionSettings {
	switch choice {
	case "a":
		return kdbx.SettingsOf(a.info)
	case "b":
		return kdbx.SettingsOf(b.info)
	}
	return kdbx.DefaultSettings(version)
}

// outputCredentials returns the credentials of the new database: those that
// unlocked A or B, or new ones.
func outputCredentials(ctx context.Context, e env, o *mergeOptions, a, b *input) (crypto.Credentials, error) {
	switch o.credentials {
	case "a":
		return a.creds, nil
	case "b":
		return b.creds, nil
	}
	return o.credsOut.load(ctx, e, o.out)
}

// printMergeSummary writes the summary of a merge: the counts of each kind and
// choice, the result and the file written.
func printMergeSummary(out *printer, o *mergeOptions, a, b *input, p *plan.Plan, res *model.Database, version kdbx.Version) {
	out.printf("A: %s\nB: %s\n", a.path, b.path)
	for _, line := range p.Summary() {
		out.println(line)
	}
	groups, entries := 0, 0
	res.WalkGroups(func(g *model.Group) bool { groups++; entries += len(g.Entries); return true })
	out.printf("result: %d groups, %d entries\n", groups, entries)
	if losses := kdbx.Losses(res, version); len(losses) > 0 {
		out.printf("accepted losses in KDBX %s: %d\n", version, len(losses))
		for _, l := range losses {
			out.printf("  %s: %s (%s)\n", l.Kind, l.Item, l.What)
		}
	}
	out.printf("written: %s (KDBX %s)\n", o.out, version)
}
