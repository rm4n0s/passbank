// Command passbank-gui is the desktop application that merges two KeePass
// databases into a new one (SPEC §13).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rm4n0s/passbank/internal/gui/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := app.Main(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
