# Manual checks

Checks that need the user (CLAUDE.md "Working through the steps"). Each has the step that raised it, what
to do and what to report. They never stop the work: the step's `IMPLEMENTED.md` entry says "pending" until
the answer is recorded, and the item is removed from this file then.

## 2.4: a KDBX 4 file written by KeePassXC itself (optional, helps steps 2.5 and 2.6)

KeePassXC 2.7's command line writes only KDBX 3.1, so every KDBX 4 fixture was made by pykeepass (which
KeePassXC reads without a problem). A file from KeePassXC's own KDBX 4 writer would check the reader
against the program most users have.

1. In the KeePassXC window: Database → New Database. Name it anything; in the encryption settings keep
   the defaults (AES-256, Argon2d). Password `fixture-min`.
2. Add one entry with title `Example`, user name `ada`, password `example-password`, and save as
   `/home/dev/…/passbank/internal/core/kdbx/testdata/k4-keepassxc.kdbx` (any folder works; copy it there).
3. Tell the agent the file is in place. It adds the file to `FIXTURES.md` and to the tests.

Nothing in the plan waits for this check.

## 2.4: a KDBX 4.1 file from KeePassXC or KeePass (optional)

`k41-fields.kdbx` has the XML elements of 4.1 but was not written by a program that knows them. A real
one: in KeePassXC 2.7 or KeePass 2.5x, make a database, add an entry, give a group a tag (KeePass: group
properties) or move an entry to another group, and save. Copy it to `testdata/k41-keepassxc.kdbx`, with the
password `fixture-min`, and tell the agent.

## 2.8 and 7.5: the password prompt and the interactive choices on a real terminal

The prompt (`cli.TTY`) reads from `/dev/tty` and cannot run in a test. In a real terminal, from the
project folder:

    go run ./cmd/passbank info internal/core/kdbx/testdata/k4-rich.kdbx

Type the password `fixture-rich` (nothing should appear while you type), press Enter, and check that the
summary appears (version 4.0, cipher AES-256, kdf Argon2d, 4 groups, 4 entries, 1 attachment, 2 history
items). Run it again, type a wrong password, and check that you get one line saying the password or key
file is wrong. Tell the agent what you saw.


For the interactive choices (step 7.5), in a real terminal from the project folder (the merge fixtures'
passwords are `merge-pair-a` for A and `merge-pair-b` for B; the output goes to a new folder):

    mkdir -p /tmp/pb-check && go run ./cmd/passbank merge internal/cli/testdata/merge/a.kdbx internal/cli/testdata/merge/b.kdbx -o /tmp/pb-check/out.kdbx

Type the two passwords, then Enter on each change (nine of them), `v` on the "My bank" change to see the
passwords, and `y` at the end. Check that `go run ./cmd/passbank info /tmp/pb-check/out.kdbx` (password
`merge-pair-a`) shows 6 entries. Try `q` once and check that no file is written. Tell the agent what you saw.

## 8.2: the file dialog of the desktop

The Open screen's **Choose…** button opens the portal's file dialog, which a test cannot open. In a real
desktop session, from the project folder:

    scripts/isolated.sh -- go run ./cmd/passbank-gui

Press **Choose…** beside the file of A, check that the dialog shows only `.kdbx` files by default and that
the file you pick appears in the field. Take a screenshot of the dialog and tell the agent what you saw (it
goes to `docs/implemented-proof-images/8.2-portal.png`).

## 8.7: the GUI with a screen reader (Orca)

The semantics of every screen are tested (labels, obscured passwords, no secret in any name), but what Orca
says is not. With Orca running (Super+Alt+S on GNOME): start `scripts/isolated.sh -- go run ./cmd/passbank-gui`,
and with the keyboard only (Tab, Shift+Tab, Space, Enter) unlock the two fixture databases
(`internal/cli/testdata/merge/a.kdbx` with the password `merge-pair-a` and `b.kdbx` with `merge-pair-b`),
compare, choose in the list, open the details of "Banking / My bank" and write a new database to a scratch folder.
Report any control that Orca reads without a name or twice, and any value that it reads aloud although it
should be hidden (the password fields must say "password" and read nothing, and the table must never read the
passwords).
