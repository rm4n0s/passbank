package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
)

// runInfo is `passbank info FILE`: it opens the database and prints how it is
// stored and how much it holds, one fact per line in a fixed order.
func runInfo(ctx context.Context, e env, args []string) int {
	out, errs := &printer{w: e.stdout}, &printer{w: e.stderr}
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := addCredentialFlags(fs, "")
	debug := fs.Bool("debug", false, "show the route of an error")
	files, err := parseInterleaved(fs, args)
	if err == nil && len(files) != 1 {
		err = errors.New("UsageInvalid", "info needs exactly one file")
	}
	if err == nil {
		err = opts.check()
	}
	if err != nil {
		reportError(errs, "info", "", err, false)
		errs.println("usage: passbank info [--keyfile PATH] [--password-file PATH] [--no-password] [--debug] FILE")
		return ExitUsage
	}
	file := files[0]

	creds, err := opts.load(ctx, e, file)
	if err != nil {
		return reportError(errs, "info", file, err, *debug)
	}
	defer creds.Wipe()
	db, info, err := kdbx.Open(ctx, file, creds)
	if err != nil {
		return reportError(errs, "info", file, err, *debug)
	}
	db.Wipe()

	printInfo(out, info)
	return out.status(errs)
}

// printInfo writes the facts of an Info, one per line.
func printInfo(out *printer, info kdbx.Info) {
	line := func(key string, value any) { out.printf("%-16s %v\n", key, value) }
	line("version", info.Version)
	line("cipher", info.Cipher)
	line("kdf", info.KDF.Name())
	if info.KDF.ID == crypto.KDFAES {
		line("kdf-rounds", info.KDF.Rounds)
	} else {
		line("kdf-memory", formatBytes(info.KDF.Memory))
		line("kdf-iterations", info.KDF.Iterations)
		line("kdf-parallelism", info.KDF.Parallelism)
	}
	line("compression", info.Compression)
	line("inner-stream", info.InnerStream)
	line("groups", info.Groups)
	line("entries", info.Entries)
	line("attachments", info.Attachments)
	line("history", info.HistoryItems)
}

// formatBytes writes a size in the largest binary unit that divides it.
func formatBytes(n uint64) string {
	switch {
	case n != 0 && n%(1<<30) == 0:
		return fmt.Sprintf("%d GiB", n>>30)
	case n != 0 && n%(1<<20) == 0:
		return fmt.Sprintf("%d MiB", n>>20)
	case n != 0 && n%(1<<10) == 0:
		return fmt.Sprintf("%d KiB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}
