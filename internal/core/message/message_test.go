package message

import (
	"io"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
)

func TestOf(t *testing.T) {
	cause := io.ErrUnexpectedEOF
	tests := []struct {
		name string
		err  error
		cli  string
		gui  string // "" when the same as the command line's
	}{
		{"not kdbx", errors.New("NotKdbx", "x"), "a.kdbx is not a KeePass database", ""},
		{"kdb", errors.New("KeePass1NotSupported", "x"), "a.kdbx is a KeePass 1 database (.kdb); open it in KeePassXC and save it as KDBX first", ""},
		{"version", errors.New("VersionUnsupported", "x", "version", "5.0"), "a.kdbx uses KDBX version 5.0, which Passbank cannot read", ""},
		{"cipher", errors.New("CipherUnsupported", "cipher 1234"), "a.kdbx uses something Passbank does not support: cipher 1234", ""},
		{"wrong credentials", errors.New("WrongCredentials", "x"), "the password or key file for a.kdbx is wrong", ""},
		{"no credentials", errors.New("NoCredentials", "x"), "a.kdbx needs a password, a key file, or both; the password is empty and no key file was given", ""},
		{"corrupt", errors.New("FileCorrupt", "bad block"), "a.kdbx is damaged: bad block", ""},
		{"too large", errors.New("FileTooLarge", "x"), "a.kdbx is larger than 2 GiB, which Passbank does not read", ""},
		{"read failed", errors.NewErr("FileReadFailed", cause), "a.kdbx cannot be read: unexpected EOF", ""},
		{"cancelled", errors.New("ReadCancelled", "x"), "cancelled", ""},
		{"key file", errors.New("KeyFileCorrupt", "checksum"), "the key file is damaged: checksum", ""},
		{"plan stale", errors.New("PlanStale", "x", "input", "A"), "the plan was not made for these databases: input A has changed since", ""},
		{"plan incomplete", errors.New("PlanIncomplete", "x", "missing", 2, "first", "u"), "the plan has no choice for 2 changes, for instance u", ""},
		{"plan unknown", errors.New("PlanUnknownItem", "x", "line", 7, "item", "u"), "the plan, line 7: u is not a change between these databases", ""},
		{"plan invalid", errors.New("PlanInvalidChoice", "x", "line", 7, "choice", "keep", "kind", "differs"), "the plan, line 7: keep is not allowed for a change of kind differs", ""},
		{"plan duplicate", errors.New("PlanDuplicateItem", "x", "line", 9, "item", "u"), "the plan, line 9: u has more than one line", ""},
		{"plan syntax", errors.New("PlanSyntax", "expected `<choice> <item>`", "line", 3), "the plan, line 3: expected `<choice> <item>`", ""},
		{"plan mismatch", errors.New("PlanMismatch", "x"), "the plan was not made for these databases", ""},
		{"group not empty", errors.New("PlanGroupNotEmpty", "x", "group path", "Games", "item title", "Forum", "conflicts", 2),
			`the group "Games" is dropped, but "Forum" in it is kept (2 items in dropped groups are kept); drop them too or keep the group`, ""},
		{"cycle", errors.New("MergeGroupCycle", "x", "group", "g"), "the choices would put the group g inside itself; take the place of one of the groups from A", ""},
		{"output exists", errors.New("OutputExists", "x"), "a.kdbx exists; use --force to replace it", "a.kdbx exists; choose another name, or confirm that it is replaced in the file dialog"},
		{"plan file exists", errors.New("PlanFileExists", "x"), "a.kdbx exists; use --force to replace it", "a.kdbx exists; confirm that it is replaced in the file dialog"},
		{"output is input", errors.New("OutputIsInput", "x"), "the output must be a new file, not one of the inputs", ""},
		{"loses data", errors.New("OutputVersionLosesData", "x", "version", "3.1", "count", 2, "losses", "GroupTags Root/Email"),
			"KDBX 3.1 cannot store everything in the result (2 items would lose data); choose --format 4 or accept the loss with --allow-loss: GroupTags Root/Email",
			"KDBX 3.1 cannot store everything in the result (2 items would lose data); choose KDBX 4 or accept the loss: GroupTags Root/Email"},
		{"encryption", errors.New("EncryptionNotInVersion", "KDBX 3.1 has only the AES-KDF; choose other encryption settings"), "KDBX 3.1 has only the AES-KDF; choose other encryption settings", ""},
		{"verify", errors.New("WriteVerifyFailed", "x"), "the written file did not read back correctly and was removed (this is a bug in Passbank)", ""},
		{"write failed", errors.NewErr("OutputWriteFailed", cause), "a.kdbx cannot be written: unexpected EOF", ""},
		{"write failed without a cause", errors.New("OutputPathInvalid", "the output path is empty"), "a.kdbx cannot be written: the output path is empty", ""},
		{"write cancelled", errors.New("WriteCancelled", "x"), "cancelled", ""},
		{"passwords", errors.New("PasswordsDoNotMatch", "x"), "the two passwords are not the same", ""},
		{"plan file", errors.NewErr("PlanFileWriteFailed", cause), "a.kdbx cannot be written: unexpected EOF", ""},
		{"a tag without a message shows its message", errors.New("SomethingElse", "what happened"), "what happened", ""},
		{"an error of another kind", io.EOF, "EOF", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Of(tt.err, "a.kdbx", CLI); got != tt.cli {
				t.Errorf("CLI: %q, want %q", got, tt.cli)
			}
			want := tt.gui
			if want == "" {
				want = tt.cli
			}
			if got := Of(tt.err, "a.kdbx", GUI); got != want {
				t.Errorf("GUI: %q, want %q", got, want)
			}
			if strings.Contains(Of(tt.err, "a.kdbx", GUI), "--") && tt.gui != "" {
				t.Error("the window's message names a command-line option")
			}
		})
	}
}
