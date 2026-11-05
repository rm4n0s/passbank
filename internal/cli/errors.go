package cli

import (
	"sort"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/message"
)

// reportError writes the error for the user on errs, as one line, and returns the
// exit status. file names the database the failure is about, when there is one.
// With debug it adds the route and the metadata of the error, which hold no
// secret (SPEC §14).
func reportError(errs *printer, command, file string, err error, debug bool) int {
	appErr, ok := errors.FromError(err)
	if !ok {
		errs.printf("passbank %s: %v\n", command, err)
		return ExitFailure
	}
	errs.printf("passbank %s: %s\n", command, message.Of(appErr, file, message.CLI))
	if debug {
		errs.printf("  tag:   %s\n  route: %s\n", appErr.Tag, appErr.Route())
		keys := make([]string, 0, len(appErr.Metadata))
		for k := range appErr.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			errs.printf("  %s: %v\n", k, appErr.Metadata[k])
		}
		if appErr.OriginalErr != nil {
			errs.printf("  cause: %v\n", appErr.OriginalErr)
		}
	}
	if appErr.Tag == "UsageInvalid" {
		return ExitUsage
	}
	return ExitFailure
}
