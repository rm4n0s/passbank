//go:build debug_mode

package app

import "flag"

// addDevFlags registers the debug-only flags and returns a function that reads
// them after the parse.
func addDevFlags(fs *flag.FlagSet) func() DevOptions {
	a := fs.String("dev-a", "", "debug builds only: preload the database A (needs --dev-b and --dev-password-file)")
	b := fs.String("dev-b", "", "debug builds only: preload the database B")
	pw := fs.String("dev-password-file", "", "debug builds only: a file with the password of A on its first line and that of B on its second (test fixtures only)")
	return func() DevOptions { return DevOptions{A: *a, B: *b, PasswordFile: *pw} }
}
