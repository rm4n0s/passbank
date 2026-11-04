// Command passbank is the command-line tool that merges two KeePass databases
// into a new one (SPEC §12).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rm4n0s/passbank/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Main(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cli.NewTTY())
	stop()
	os.Exit(code)
}
