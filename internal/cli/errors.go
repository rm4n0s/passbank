package cli

import (
	"fmt"
	"sort"

	"github.com/rm4n0s/errors"
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
	errs.printf("passbank %s: %s\n", command, userMessage(appErr, file))
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

// userMessage is the message of SPEC §15 for an error; file is the database it is
// about. Tags that have no message there show the message of the error.
func userMessage(e *errors.Error, file string) string {
	switch e.Tag {
	case "NotKdbx":
		return fmt.Sprintf("%s is not a KeePass database", file)
	case "KeePass1NotSupported":
		return fmt.Sprintf("%s is a KeePass 1 database (.kdb); open it in KeePassXC and save it as KDBX first", file)
	case "VersionUnsupported":
		return fmt.Sprintf("%s uses KDBX version %v, which Passbank cannot read", file, e.Metadata["version"])
	case "CipherUnsupported", "KdfUnsupported", "InnerStreamUnsupported", "CompressionUnsupported":
		return fmt.Sprintf("%s uses something Passbank does not support: %s", file, e.Message)
	case "WrongCredentials":
		return fmt.Sprintf("the password or key file for %s is wrong", file)
	case "NoCredentials":
		return fmt.Sprintf("%s needs a password, a key file, or both; the password is empty and no key file was given", file)
	case "FileCorrupt":
		return fmt.Sprintf("%s is damaged: %s", file, e.Message)
	case "FileTooLarge":
		return fmt.Sprintf("%s is larger than 2 GiB, which Passbank does not read", file)
	case "FileReadFailed":
		return fmt.Sprintf("%s cannot be read: %v", file, e.OriginalErr)
	case "ReadCancelled", "KdfCancelled":
		return "cancelled"
	case "PlanFileWriteFailed":
		return fmt.Sprintf("%s cannot be written: %v", file, e.OriginalErr)
	case "KeyFileCorrupt":
		return "the key file is damaged: " + e.Message
	}
	return e.Message
}
