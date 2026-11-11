//go:build !debug_mode

package app

import "flag"

// addDevFlags registers nothing in a release build: the debug-only flags do not
// exist there.
func addDevFlags(*flag.FlagSet) func() DevOptions { return func() DevOptions { return DevOptions{} } }
