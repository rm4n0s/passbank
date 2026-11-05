# Passbank

Passbank merges two KeePass databases into a **new** one, and lets you choose what the new database keeps.

You give it two KDBX files, **A** and **B**, of any KDBX version (2.x, 3.0, 3.1, 4.0 and 4.1, in any combination). It lists what differs: entries only in A, entries only in B, entries in both that differ, and entries that one side deleted. For each difference you pick what goes into the result. Then it writes a new file, in KDBX 4 unless you pick another version. A and B are never modified.

There are two programs on one shared core, so both produce the same database from the same inputs and choices:

- `passbank`, a command-line tool for terminals and scripts
- `passbank-gui`, a desktop application (built on [Glutter](https://github.com/rm4n0s/glutter), no cgo)

## What Passbank is not

- It is **not a password manager**: it does not view, edit or create entries outside a merge. Use KeePassXC or another KeePass client for that. The databases Passbank writes open in them.
- It does not merge more than two databases in one run (run it twice), does not read KeePass 1 (`.kdb`) files, and does not support hardware keys (YubiKey) or Windows account keys.
- It uses no network and no cloud. It runs on Linux only.

## Install

Passbank needs Go 1.27 or later and no C compiler (it is cgo-free).

```
go install github.com/rm4n0s/passbank/cmd/passbank@latest
go install -tags glutter_release github.com/rm4n0s/passbank/cmd/passbank-gui@latest
```

The GUI needs a Wayland or X11 desktop with Vulkan or OpenGL ES 3.0 drivers. `passbank --version` and `passbank-gui --version` print the module version and the commit the binary was built from.

## The command line

```
passbank info  FILE
passbank diff  A B [--prefer newer|a|b|union] [-o PLAN] [--no-titles]
passbank merge A B -o OUT [--plan PLAN | --prefer newer|a|b|union]
                          [--format 4|4.1|4.0|3.1|3.0|2] [--allow-loss]
                          [--settings-from a|b] [--credentials a|b|new] [--encryption a|b|default] [--force]
```

Passwords are never taken as arguments or from the environment (other processes can read both). Each command asks for them on the terminal without echo, or reads them from a file that only you can read (`--password-file-a PATH`), and takes key files with `--keyfile-a PATH` (the same for `b`, and for the new database `--keyfile-out`, `--password-file-out`).

**Look at a file**

```
$ passbank info personal.kdbx
```

**See what differs, and decide in a file**

```
$ passbank diff personal.kdbx laptop.kdbx -o plan.txt
```

`plan.txt` lists every change with its default choice (the newer version of what differs, everything that exists on one side only is kept). Edit the choices you disagree with; the file has one line per change, with a comment that shows what it is. The plan never holds a password or any other value of a field. It names the two inputs by their SHA-256, so a plan made for other files is refused.

**Merge with the plan**

```
$ passbank merge personal.kdbx laptop.kdbx --plan plan.txt -o merged.kdbx
```

**Merge by a rule, without questions**

```
$ passbank merge personal.kdbx laptop.kdbx --prefer union -o merged.kdbx
```

`newer` takes the newer version of what differs, `a` and `b` let one side win everywhere, `union` keeps everything that exists on one side only and both versions of every pair that differs.

**Merge and decide one change at a time**

```
$ passbank merge personal.kdbx laptop.kdbx -o merged.kdbx
```

With neither `--plan` nor `--prefer`, Passbank goes through the changes on the terminal: it shows the group path, title, both modification times and the fields that differ, and asks for a choice with one key (`a`, `b`, `t` both, `k` keep, `d` drop, Enter for the default, `?` for help). At the end it shows a summary and asks before it writes.

**Choose the version, the credentials and the encryption**

```
$ passbank merge a.kdbx b.kdbx --prefer newer -o old.kdbx --format 3.1 --encryption default
```

`--format` defaults to `4`: version 4.0, or 4.1 when the result holds data only 4.1 stores. When the chosen version cannot hold something in the result, Passbank **refuses to write** and lists what would be lost, unless you accept the loss with `--allow-loss`. `--credentials` says whose password and key file the new database gets (`a` by default, `b`, or `new`), `--encryption` whose cipher and key derivation (`default` is AES-256 with Argon2id for KDBX 4, the AES-KDF for older versions). The output must be a new file: an existing one is replaced only with `--force`, and an input is never replaced.

Exit status is 0 on success, 1 on failure and 2 for a mistake in the arguments. `--debug` shows the route of an error (never a secret).

## The desktop application

```
passbank-gui
```

One window, three screens:

1. **Open**: two panels, A and B. Choose a file (or drop it on the panel), type the password, choose a key file if there is one, and press **Unlock**. The panel then shows the version, cipher and counts. **Swap A and B** exchanges the sides; **Compare** opens the next screen.
2. **Review**: on the left, the changes grouped by kind with counts, a filter (all, one kind, undecided only) and a search over group path and title. Select rows with the mouse (Shift and Ctrl) or the arrow keys and set the choice with the buttons or the keys `A` (A wins), `B` (B wins), `K` (keep, or keep both), `D` (drop) and `N` (the newer). The four rule buttons apply a rule to the rows shown. On the right, the two versions side by side with the fields that differ highlighted, protected values masked (each with its own **Show**), attachments marked same or different, and both modification times. **Load plan** and **Save plan** use the same plan file as the command line.
3. **Write**: the file to write, the version (KDBX 4 by default), where the database settings, the credentials and the encryption come from, and a warning that lists what the chosen version cannot store (Write then asks you to confirm). **Write** runs in the background with a progress bar and a Cancel button, then says that the file was read back and verified.

The application keeps no state between runs: no recent files, no settings. Closing the window, or going back to the first screen, drops the unlocked databases and their secrets. It works by keyboard alone and with a screen reader.

## What gets merged

Entries and groups are matched by their **UUID**, never by title. For a pair that differs you take A's, take B's, or keep both (B's copy gets a new UUID). Passbank keeps everything an input holds and you keep: history (the two histories are united), attachments, custom icons, custom data, auto-type settings, and XML elements it does not know. Entries one side deleted are dropped or kept as you decide (by default the deletion wins when it is newer than the entry). The database settings (name, description, history limits, recycle bin and so on) come from A or from B, as you choose.

The exact rules are in [docs/SPEC.md](docs/SPEC.md) (sections 7 to 10).

## Compatibility

- **Reads** KDBX 2.x, 3.0, 3.1, 4.0 and 4.1 with AES-256, Twofish and ChaCha20, the AES-KDF, Argon2d and Argon2id, password, key file (XML 1.0 and 2.0, raw 32 bytes, hex, or any file) or both, compressed or not.
- **Writes** every one of those versions. KDBX 4 files use ChaCha20 for the protected values, older files Salsa20; AES-256 or Twofish for the database.
- **KeePassXC**: the files Passbank writes open in KeePassXC 2.7.12 (tested with `keepassxc-cli` for every combination of cipher, key derivation and compression, and for a file of every kind of change). Reading was checked against files from KeePassXC, KeePass, pykeepass, kdbxweb, gokeepasslib and keepass-rs. KDBX 2.x and 3.0 files from other programs were not available to test; they are read by the same code as 3.1.
- Every write is verified: Passbank reads the new file back with the same credentials, before it puts it in place, and compares it with what it meant to write. A file that fails the check never reaches the output path.

## Security

- **Secrets in memory** (passwords, keys, protected field values) are held as bytes and overwritten when they are no longer needed, as far as Go allows. They are never written to a log, an error message, a plan file or a screenshot. **Passbank does not promise that a secret never reaches swap or a core dump**: Go's runtime can copy memory, and the operating system can write it to disk. If that matters to you, use encrypted swap and disable core dumps. The GUI's text fields hold what you type as ordinary strings until you unlock or write, and clear them then.
- **Your databases are never changed.** The result goes to a new file, written to a temporary file in the target folder (readable only by you), synced, verified and put in place; an existing file is never replaced unless you ask (`--force`, or the confirmation of the save dialog), and an input is never the output.
- **No network**: the process opens no network connection.
- **Untrusted input**: the two files are treated as untrusted. Every length and count read from a file is bounded before memory is allocated, and the parsers are fuzzed.
- **No passwords in arguments or the environment**: other processes can read both.
- Errors show paths, UUIDs and versions, never secret values.

## Development

```
go build ./...                   # CGO_ENABLED=0 go build ./... must work too
go vet ./...
golangci-lint run
GOMEMLIMIT=3GiB go test -race -p 2 ./...
scripts/checks.sh                # all the checks, through scripts/heavy.sh
```

Tests that need KeePassXC (`-tags passbank_keepassxc`), a display (`-tags passbank_display`) or a long run (`-tags passbank_large`) are described in [docs/IMPLEMENTATION.md](docs/IMPLEMENTATION.md). The specification is [docs/SPEC.md](docs/SPEC.md), the plan [docs/IMPLEMENTATION.md](docs/IMPLEMENTATION.md) and what was built, step by step, [docs/IMPLEMENTED.md](docs/IMPLEMENTED.md).

Release builds: `go build ./cmd/passbank` and `go build -tags glutter_release ./cmd/passbank-gui`. The debug build of the GUI has three flags (`--dev-a`, `--dev-b`, `--dev-password-file`) that preload test fixtures for display tests; a release build does not have them.

## License

MIT, see [LICENSE](LICENSE). The licenses of what Passbank bundles are in [NOTICE](NOTICE) and `third_party/`.
