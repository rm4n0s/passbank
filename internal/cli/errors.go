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
	case "PlanStale":
		return fmt.Sprintf("the plan was not made for these databases: input %v has changed since", e.Metadata["input"])
	case "PlanIncomplete":
		return fmt.Sprintf("the plan has no choice for %v changes, for instance %v", e.Metadata["missing"], e.Metadata["first"])
	case "PlanUnknownItem":
		return fmt.Sprintf("the plan, line %v: %v is not a change between these databases", e.Metadata["line"], e.Metadata["item"])
	case "PlanInvalidChoice":
		return fmt.Sprintf("the plan, line %v: %v is not allowed for a change of kind %v", e.Metadata["line"], e.Metadata["choice"], e.Metadata["kind"])
	case "PlanDuplicateItem":
		return fmt.Sprintf("the plan, line %v: %v has more than one line", e.Metadata["line"], e.Metadata["item"])
	case "PlanSyntax":
		return fmt.Sprintf("the plan, line %v: %s", e.Metadata["line"], e.Message)
	case "PlanMismatch":
		return "the plan was not made for these databases"
	case "PlanGroupNotEmpty":
		return fmt.Sprintf("the group %q is dropped, but %q in it is kept (%v items in dropped groups are kept); drop them too or keep the group",
			e.Metadata["group path"], e.Metadata["item title"], e.Metadata["conflicts"])
	case "MergeGroupCycle":
		return fmt.Sprintf("the choices would put the group %v inside itself; take the place of one of the groups from A", e.Metadata["group"])
	case "PlanFileReadFailed":
		return fmt.Sprintf("%s cannot be read: %v", file, e.OriginalErr)
	case "OutputExists":
		return fmt.Sprintf("%s exists; use --force to replace it", file)
	case "OutputIsInput":
		return "the output must be a new file, not one of the inputs"
	case "OutputVersionLosesData":
		return fmt.Sprintf("KDBX %v cannot store everything in the result (%v items would lose data); choose --format 4 or accept the loss with --allow-loss: %v",
			e.Metadata["version"], e.Metadata["count"], e.Metadata["losses"])
	case "EncryptionNotInVersion":
		return e.Message
	case "WriteVerifyFailed":
		return "the written file did not read back correctly and was removed (this is a bug in Passbank)"
	case "OutputPathInvalid", "OutputTempFailed", "OutputWriteFailed", "OutputPlaceFailed", "OutputSyncFailed":
		if e.OriginalErr == nil {
			return fmt.Sprintf("%s cannot be written: %s", file, e.Message)
		}
		return fmt.Sprintf("%s cannot be written: %v", file, e.OriginalErr)
	case "WriteCancelled":
		return "cancelled"
	case "PasswordsDoNotMatch":
		return "the two passwords are not the same"
	case "PlanFileWriteFailed":
		return fmt.Sprintf("%s cannot be written: %v", file, e.OriginalErr)
	case "KeyFileCorrupt":
		return "the key file is damaged: " + e.Message
	}
	return e.Message
}
