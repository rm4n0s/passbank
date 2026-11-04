# Passbank specification

This document is the source of truth for Passbank. Where it says **(assumed, A<n>)**, the decision is a proposal that the user has not confirmed yet; section 19 lists them. A step that depends on an assumption confirms it with the user before it starts.

## 1. Purpose

Passbank merges two KeePass databases into a new one, and lets the user choose what the new database keeps.

Passbank takes two KDBX files, **A** and **B**, of any KDBX version. It lists the differences between them: entries only in A, entries only in B, entries in both that differ, and entries one side deleted. For each difference, the user picks what goes into the result. Passbank then writes a **new** KDBX file, in KDBX 4 unless the user picks another version. A and B are never modified.

There are two applications, both built on one shared core:
- `passbank`, a command-line tool for terminals and scripts
- `passbank-gui`, a desktop application built on Glutter

Given the same inputs and the same choices, both produce the same database.

## 2. Goals and non-goals

**Goals**
- Read every KDBX version (2.x, 3.0, 3.1, 4.0, 4.1), with every standard cipher and key-derivation function
- Never lose data on the way through. Everything an input holds and the user keeps reaches the output, including history, attachments, custom icons, custom data and XML elements Passbank does not know
- Make every decision visible. Nothing that differs between A and B is resolved silently: either the user chooses, or a documented default applies and is shown
- Write every KDBX version as output (2.x, 3.0, 3.1, 4.0, 4.1), with KDBX 4 as the default
- Write output that opens in KeePassXC 2.7+, KeePass 2.48+ and other clients that read the chosen version
- Run on the Linux desktop, without cgo

**Non-goals**
- Viewing, editing or creating entries outside a merge. Passbank is not a password manager
- Merging more than two databases in one run (run it twice instead)
- KeePass 1 databases (`.kdb`). They are not KDBX; Passbank recognizes them and says so (section 15)
- Hardware keys (YubiKey challenge-response) and Windows user-account keys
- Network access of any kind, synchronization, cloud storage
- Platforms other than Linux

## 3. Terms

- **Input A, input B**: the two databases being merged. A is the *primary* input; where a rule needs a tie-breaker, A wins
- **Item**: an entry or a group
- **Change**: one difference between A and B that needs a choice (section 8)
- **Choice**: what the user decided for one change (section 9)
- **Plan**: the full set of changes with one choice each, tied to the exact bytes of A and B (section 11)
- **Result**: the database built by applying a plan; it is written to the **output** file

## 4. Reading KDBX

### 4.1 Supported formats

| File version | Written by | Outer cipher | Key derivation | Inner stream |
|---|---|---|---|---|
| 2.x, 3.0, 3.1 | KeePass 2.x before 2.35, KeePassXC (3.1) | AES-256-CBC, Twofish-CBC, ChaCha20 | AES-KDF | Salsa20, none |
| 4.0, 4.1 | KeePass 2.35+, KeePassXC 2.3+ | AES-256-CBC, Twofish-CBC, ChaCha20 | AES-KDF, Argon2d, Argon2id | ChaCha20, Salsa20, none |

Identifiers, to be checked against the fixtures:
- Ciphers: AES-256 `31c1f2e6-bf71-4350-be58-05216afc5aff`, Twofish `ad68f29f-576f-4bb9-a36a-d47af965346c`, ChaCha20 `d6038a2b-8b6f-4cb5-a524-339a31dbb59a`
- KDFs: AES-KDF `c9d9f39a-628a-4460-bf74-0d08c18a4fea`, Argon2d `ef636ddf-8c29-444b-91f7-a9a403e30a0c`, Argon2id `9e298b19-56db-4773-b23d-fc3ec6f0a1e6`
- Inner stream IDs: 0 none, 1 ArcFourVariant, 2 Salsa20, 3 ChaCha20

Format references: KeePass's "KDBX 4" and "KDBX 4.1" pages (keepass.info/help/kb/kdbx_4.html, kdbx_4.1.html) and the KeePass 2 source. KeePassXC's source is GPL: read it to understand behavior, never copy from it (Passbank is MIT).

- **Compression**: none or GZip
- **Version check**: files with major version 2 or 3 go through the KDBX3 reader, and files with major version 4 through the KDBX4 reader. A major version above 4 is rejected with `VersionUnsupported`
- **Rejected inputs**: the ArcFourVariant inner stream (`InnerStreamUnsupported`, see A9), unknown cipher, KDF or compression IDs, and KeePass 2 pre-release files (the second signature `0xB54BFB66`)

### 4.2 Credentials

A database is unlocked with a password, a key file, or both.
- **Password**: the UTF-8 bytes of what the user typed. An empty password field means "no password component", as in KeePassXC
- **Key file**: every format KeePass accepts:
  - XML key file version 1.0 (base64 `Data`)
  - XML key file version 2.0 (hex `Data` with a `Hash` attribute, which is verified: a mismatch is `KeyFileCorrupt`)
  - a 32-byte binary file
  - a 64-character hex file
  - any other file, which is used as the SHA-256 of its contents
- **Composite key**: SHA-256 of the concatenated component hashes, in KeePass's order

Wrong credentials are reported as `WrongCredentials`, never as corruption. KDBX4 detects them through the header HMAC; KDBX3 through the stream start bytes.

### 4.3 Integrity

- KDBX4: the header SHA-256, the header HMAC and the per-block HMACs are all verified
- KDBX3: the hashed-block stream hashes are verified

A failure after the credentials have been verified is `FileCorrupt`, and nothing of the file is used.

### 4.4 Preservation

The reader converts both versions into one data model (section 6) and keeps everything:
- Protected values are decrypted from the inner stream and marked as protected
- KDBX3 binaries (in `Meta/Binaries`) and KDBX4 binaries (in the inner header) become one content-addressed attachment pool
- XML elements and attributes Passbank does not model are kept as raw XML on the object they belong to, and written back unchanged
- `CustomData` (in the database, groups and entries), the `PublicCustomData` header field, and plugin data are kept

The only things dropped are those that only exist to encode the file itself:
- the KDBX3 `HeaderHash`
- seeds and IVs
- the inner stream key
- block framing

### 4.5 Differences between versions

To be written during M3: the differences between KDBX 2.x, 3.0 and 3.1 (header fields, XML elements and their formats), established from the KeePass 2 source and the fixtures. Reading these versions uses this section, and writing them (M5) must follow it exactly.

## 5. Writing the result

- **Format**: any version the reader accepts: 2.x, 3.0, 3.1, 4.0 or 4.1.
  - The default is **KDBX 4**: version 4.0, or 4.1 when the result holds data that only 4.1 can store (group tags, previous-parent-group, custom icon names or modification times, the entry quality-check flag). This matches KeePass's rule of writing the lowest version that holds the data
  - The user can pick an exact version instead: `--format` in the CLI, a version menu on the GUI's Write screen
  - The differences between 2.x, 3.0 and 3.1 that matter for writing are established from the KeePass 2 source during M3 and recorded in §4.5. Until then, writing those versions is not specified
- **Data the chosen version cannot hold**: when the result holds something the chosen version has no place for, the write is refused with `OutputVersionLosesData`. The error lists what would be lost and on which items. Examples:
  - for 3.x and 2.x: custom data on groups and entries, the `PublicCustomData` header field, and the 4.1 fields
  - for 4.0: the 4.1 fields

  The user can accept the loss explicitly: `--allow-loss` in the CLI, or a confirmation dialog in the GUI that shows the same list. Without that, nothing is written **(assumed, A11)**
- **Encryption per version**:
  - KDBX 3.x and 2.x support only AES-KDF and use the Salsa20 inner stream, with AES-256 or Twofish as the outer cipher
  - KDBX 4.x supports every cipher and KDF of §4.1 and uses the ChaCha20 inner stream
  - When the encryption settings taken from an input are not possible in the chosen version (for example Argon2 for 3.1), the write is refused with `EncryptionNotInVersion`, and the user picks the defaults or the other input's settings
- **Encryption settings** (cipher, KDF and its parameters, compression):
  - default: copied from A, if A is KDBX4
  - otherwise, the defaults: AES-256, GZip, and Argon2id with 64 MiB of memory, 2 lanes and 10 iterations for KDBX 4; AES-KDF with 2,000,000 rounds for KDBX 3.x and 2.x **(assumed, A5)**
  - the user can pick B's settings instead, or the defaults
- **Credentials**: the default is A's credentials, reused from the composite key that unlocked A, so the user doesn't type them again **(assumed, A4)**. The user can pick B's instead, or new ones (a password typed twice and/or a key file)
- **Fresh secrets**: seeds, IVs and the inner stream key are new random values from `crypto/rand` on every write
- **Safe writing**:
  - the output is written to a temporary file in the target folder with mode `0600`, synced, and renamed into place, and then the folder is synced
  - an existing output file is replaced only when the user explicitly asked for it: `--force` in the CLI, or the save dialog's overwrite confirmation in the GUI
  - the output may never be the same file as A or B (checked with `os.SameFile`, also with `--force`): `OutputIsInput`
- **Verification after writing**: Passbank reads the written file back with the same credentials and compares its model with the result it meant to write. A mismatch is `WriteVerifyFailed`; the file is removed, and the error is a bug to fix

## 6. Data model

`internal/core/model` is format-independent:
- **Database**:
  - `Meta`: name, description, default username, color, recycle-bin settings, history limits, memory protection flags, custom icons, custom data, raw extra XML
  - root group
  - deleted objects
  - attachment pool
- **Group**:
  - UUID, name, notes, icon, custom icon, times, expiry
  - auto-type and search settings, default auto-type sequence, last top visible entry
  - tags, previous parent, custom data, raw extra XML
  - child groups and entries, in order
- **Entry**:
  - UUID, icon, custom icon, colors, override URL, tags, quality check, times, expiry, previous parent
  - string fields (standard and custom, each with its protected flag)
  - attachments (name → pool reference), auto-type settings and associations
  - custom data, raw extra XML
  - history (a list of entries without history)
- **Times**: creation, last modification, last access, expiry, the expires flag, usage count, location changed. All times are UTC with second precision, which is what KDBX4 stores

## 7. Matching

Matching pairs each item of A with at most one item of B.
1. **By UUID**: items with the same UUID are a pair, wherever they are in the tree
2. **Recycle bins**: A's recycle-bin group and B's recycle-bin group are a pair even when their UUIDs differ
3. **Probable matches** (entries only) **(assumed, A2)**: after step 1, an entry of A without a pair and an entry of B without a pair are a *probable pair* when their normalized title, username and URL host are equal, and each has exactly one such candidate on the other side.
   - Normalized means: trimmed, case-folded, and the URL reduced to its host without `www.`
   - Two or more candidates on either side make no pair, so the entries stay "only in A" and "only in B"

## 8. Comparison and changes

Two paired entries are **equal** when everything compares equal:
- their string fields, with values and protected flags
- their attachments (name and content hash)
- icon, custom icon, colors, override URL, tags
- expiry and the expires flag
- auto-type settings, custom data and raw extra XML
- the UUID of their parent group

Last-access time, usage count and history are not compared. Equal pairs are not listed: the result keeps A's copy with the union of both histories (section 10.2).

Each unequal or unpaired item is one **change** of one kind:

| Kind | Meaning | Choices | Default |
|---|---|---|---|
| `only-a` | Entry or group only in A | `keep`, `drop` | `keep` |
| `only-b` | Entry or group only in B | `keep`, `drop` | `keep` |
| `differs` | Paired entries (by UUID) that are not equal | `take-a`, `take-b`, `both`, `drop` | the newer by last modification time; A on a tie |
| `probable` | A probable pair (section 7) | `take-a`, `take-b`, `both`, `drop` | the newer, as for `differs` **(assumed, A2)** |
| `deleted-in-a` | Entry in B whose UUID is in A's deleted objects | `keep`, `drop` | `drop` if the deletion is newer than B's last modification, else `keep` |
| `deleted-in-b` | The same, the other way round | `keep`, `drop` | as above |
| `group-differs` | Paired groups whose own properties differ (name, notes, icon, tags, expiry, settings, parent) | `take-a`, `take-b` | the newer |
| `settings` | The databases' `Meta` settings differ (name, description, default username, color, history limits, recycle-bin settings, memory protection) | `take-a`, `take-b` | `take-a` **(assumed, A6)** |

Choices are made per whole entry **(assumed, A3)**; choosing individual fields of an entry is not in 1.0.

What each choice means:
- **`both`** keeps both entries. A's copy keeps its UUID; B's copy gets a new UUID and is placed in the group that B had it in
- **`drop`** on a group drops the group and everything in it. It is allowed only when the user has chosen `drop` for every item under it; otherwise the plan is invalid (`PlanGroupNotEmpty`), and the GUI offers to set them all to `drop`
- **Entries only in one input, in a group that only exists in that input**: their group is kept automatically while at least one of them is kept

Changes are listed in a stable order: by kind (in the table's order), then by group path, then by title, then by UUID.

## 9. Choices and rules

A plan starts from the defaults in section 8. The user can then change choices one at a time, or apply a **rule** to many changes at once:
- `newer`: the defaults
- `a`: `take-a` for `differs`, `probable`, `group-differs` and `settings`; `keep` for `only-a`; `drop` for `only-b`; and for deletions, A's state wins (if A deleted the entry, `drop`)
- `b`: the mirror image of `a`
- `union`: `keep` for everything only in one input, `both` for `differs` and `probable`, and the defaults for the rest

The same rule definitions are used by the CLI's `--prefer` and the GUI's bulk actions.

## 10. Applying a plan

The core builds the result from A, B and a complete, valid plan. It is a pure function: same inputs and plan, same result model (random values are only added later, by the writer).

### 10.1 Tree

The result's tree starts as A's tree, with every item's position following the copy the plan chose:
- an entry taken from B goes to the group B had it in
- a group that doesn't exist in the result yet is created, with its ancestors, from the input it came from

The order of children follows the chosen input; items from the other input are appended after them in their own order.

### 10.2 History

- **Union**: the history of a kept entry is the union of both inputs' histories for that UUID, without duplicates (two versions with the same last modification time and equal content), sorted by last modification time
- **The version not chosen**: when the chosen version differs from the other one, the other version is added to the history too **(assumed, A7)**, so the merge can be undone in any KeePass client
- **No trimming**: history is not trimmed to the history limits. Clients trim on their next save

### 10.3 Deleted objects

The result's deleted-objects list is the union of both lists, minus every UUID the result contains.

### 10.4 Meta

- Settings come from the input chosen for the `settings` change
- Custom icons: the union by UUID. When both have the same UUID with different data, the newer icon wins (in 4.1), else A's
- Custom data: the union by key. When both have a key with different values, the value comes from the input that supplied the settings
- The recycle bin is A's group, holding the recycle-bin contents of both inputs as chosen

### 10.5 Attachments

The attachment pool is rebuilt from the attachments the result actually references (entries and history), with duplicates removed by content hash.

## 11. Plan file

A plan file is how the CLI saves choices and loads them back, and the GUI can load and save it too. It is UTF-8 text, line based, and easy to edit by hand:

```
passbank-plan 1
a sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 personal.kdbx
b sha256:60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752 laptop.kdbx

# only in A
keep    3f1c2a6e-5b2d-4d7e-9a3b-2c1d0e9f8a7b  # Email / Work mail
# only in B
drop    0b8e4d2c-1f3a-4c5b-8d6e-7f9a0b1c2d3e  # Games / Old forum
# differs: password, url  (A 2026-10-01 12:00, B 2026-10-30 09:12)
take-b  7d0e1c2b-3a4f-4e5d-9c8b-7a6f5e4d3c2b  # Banking / My bank
# probable match
both    1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d=9f8e7d6c-5b4a-4c3d-2e1f-0a9b8c7d6e5f  # Shopping / Store
settings take-a
```

- **Header**: the first line holds the format version. Lines `a` and `b` hold the SHA-256 of the input files' bytes and their names
- **Stale plans**: a plan whose hashes do not match the given inputs is rejected (`PlanStale`). A plan only fits the exact files it was made for
- **One line per change**: each change has one line, `<choice> <item>`. A probable pair is written as `<A UUID>=<B UUID>`. Comments (`#` to the end of the line) and blank lines are ignored
- **Errors**:
  - a missing change is `PlanIncomplete`
  - an item that is not a change of these inputs is `PlanUnknownItem`
  - a choice the kind doesn't allow is `PlanInvalidChoice`
  - a malformed line is `PlanSyntax`, with its line number
- **Secrets**: the file never contains passwords or other field values. Group paths and titles appear only in comments, and the CLI's `--no-titles` leaves them out, because titles can be sensitive

## 12. Command-line tool (`passbank`)

```
passbank diff  A B [--prefer newer|a|b|union] [-o PLAN] [--no-titles]
passbank merge A B -o OUT [--plan PLAN | --prefer newer|a|b|union]
               [--format 4|4.1|4.0|3.1|3.0|2] [--allow-loss]
               [--settings-from a|b] [--credentials a|b|new] [--encryption a|b|default] [--force]
passbank info  FILE
```

- **`diff`**: unlocks A and B and writes a plan with the defaults (or the given rule) to `PLAN`, or to stdout. The user edits the plan, then runs `merge --plan`
- **`merge`**: builds and writes the result. Without `--plan` it uses `--prefer`, which defaults to `newer`. `--format` picks the output version and defaults to `4` (4.0, or 4.1 when needed, as in §5); `--allow-loss` accepts the losses that a lower version causes. It prints a summary: the counts of each kind and choice, the output path and the format version
- **`info`**: prints a file's KDBX version, cipher, KDF and its parameters, compression, and counts of groups, entries, attachments and history items. It needs the credentials, because the counts are inside the encrypted part
- **Credentials**, for each input X (`a`, `b`) and for `out` when `--credentials new`:
  - `--keyfile-X PATH`
  - `--password-file-X PATH` reads the first line of a file. The file must be readable only by its owner, or Passbank refuses it
  - `--no-password-X` means the database has no password component
  - otherwise the password is read without echo from `/dev/tty`; a new output password is asked twice
  - if there is no terminal, the error is `PasswordNeedsTerminal`
  - passwords are never accepted as arguments or environment variables
- **Output streams**: messages go to stderr; the plan (in `diff`) and the summaries go to stdout
- **Exit codes**: `0` success, `1` failure, `2` usage error. Errors print one line with the message; `--debug` adds the route

## 13. Desktop application (`passbank-gui`)

Built on Glutter's `widgets` layer, without Material. One window, three screens:

1. **Open**: two panels, A and B. Each panel has:
   - a file chooser (`desktop.OpenFiles`, filter `*.kdbx`; dropping a file on the panel also works)
   - a password field (obscured, with a reveal button)
   - a key-file chooser and an **Unlock** button
   - after unlocking, the panel shows the file's version, cipher and counts

   **Compare** is enabled when both are unlocked; it opens the Review screen. There is also a **Swap A and B** button.
2. **Review**: on the left, the list of changes grouped by kind with counts, a filter (all, by kind, undecided only) and a search over group path and title. Each row shows the kind, group path, title and the current choice.

   On the right, the details of the selected change:
   - a side-by-side table of A's and B's fields, with differing fields highlighted
   - protected values masked, each with its own reveal toggle that hides it again when the selection changes
   - the attachments, marked same or different
   - both last modification times

   The choice is set with buttons (Take A, Take B, Keep both, Keep, Drop, enabled per kind) or the keys `A`, `B`, `K` (keep or keep both), `D` (drop) and `N` (newer). The keys work on every selected row. Bulk actions apply the rules of section 9. **Load plan** and **Save plan** use the plan file of section 11.
3. **Write**:
   - the output path (`desktop.SaveFile`)
   - the output version (default KDBX 4), the settings source, the credentials (A's, B's or new) and the encryption source
   - when the chosen version cannot hold everything, a warning that lists what would be lost; Write then asks for confirmation
   - a summary of what the result will contain
   - **Write**, which shows progress while the KDF and the write run off the UI thread, then the verification result

   **Back** returns to Review with every choice intact.

- **Debug flags**: debug builds (without `glutter_release`) accept `--dev-a PATH`, `--dev-b PATH` and `--dev-password-file PATH` to preload fixtures for display tests and proofs. Release builds don't have them, and no build accepts a password in an argument
- **State**: the GUI keeps no state between runs: no recent files, no settings files **(assumed, A8)**
- **Leaving**: closing the window or going back to Open drops every unlocked database and its secrets

## 14. Security

- **Secrets in memory**: secret values (passwords, keys, protected field values) are held in `[]byte` in `internal/core/secret` and zeroed when no longer needed, as far as Go allows. They are never converted to `string` except at the last moment of display, and never logged. Passbank does not promise that a secret never reaches swap or a core dump; this is stated in the README
- **Error metadata**: errors carry paths, UUIDs, versions and IDs, never secret values
- **No network**: the process opens no network connections
- **Concurrency**: KDF runs and file reads and writes are cancellable through `ctx` and never run on the UI thread
- **Untrusted input**: inputs are untrusted files. Every length, count and size read from a file is bounded before allocation, so a hostile file cannot exhaust memory, and the parsers are fuzzed (with the limits of CLAUDE.md)

## 15. User-facing errors

Each failure has a Tag (see CLAUDE.md, "Error handling") and a message for the user. The ones the user can meet:

| Tag | Message (sketch) |
|---|---|
| `NotKdbx` | "<file> is not a KeePass database" |
| `KeePass1NotSupported` | "<file> is a KeePass 1 database (.kdb); open it in KeePassXC and save it as KDBX first" |
| `VersionUnsupported` | "<file> uses KDBX version <v>, which Passbank does not know" |
| `CipherUnsupported`, `KdfUnsupported`, `InnerStreamUnsupported`, `CompressionUnsupported` | "<file> uses <thing> <id>, which Passbank does not support" |
| `WrongCredentials` | "The password or key file for <file> is wrong" |
| `KeyFileCorrupt` | "The key file <path> is damaged (its checksum does not match)" |
| `FileCorrupt` | "<file> is damaged: <where>" |
| `PlanStale`, `PlanIncomplete`, `PlanUnknownItem`, `PlanInvalidChoice`, `PlanSyntax`, `PlanGroupNotEmpty` | what is wrong, with the line or item |
| `OutputExists` | "<out> exists; use --force to replace it" |
| `OutputVersionLosesData` | "KDBX <v> cannot store <what> of <n> items; choose KDBX 4 or accept the loss" |
| `EncryptionNotInVersion` | "KDBX <v> does not support <cipher or KDF>; choose other encryption settings" |
| `OutputIsInput` | "The output must be a new file, not one of the inputs" |
| `PasswordNeedsTerminal`, `PasswordsDoNotMatch`, `PasswordFileTooOpen` | |
| `WriteVerifyFailed` | "The written file did not read back correctly and was removed" (a bug) |

## 16. Packages

```
cmd/passbank/              CLI main
cmd/passbank-gui/          GUI main
internal/core/secret/      secret byte buffers
internal/core/model/       data model (section 6)
internal/core/kdbx/        file container: signature, headers, block streams, compression, inner stream
internal/core/kdbx/crypto/ ciphers, KDFs, composite key, key files
internal/core/kdbx/xml/    XML <-> model, raw-XML preservation, time formats
internal/core/diff/        matching and comparison -> changes (sections 7, 8)
internal/core/plan/        choices, rules, plan file (sections 9, 11)
internal/core/merge/       applying a plan -> result model (section 10)
internal/core/output/      safe write, same-file checks, read-back verification (section 5)
internal/cli/              CLI frontend
internal/gui/app/          the application widget and screen flow
internal/gui/open/         Open screen
internal/gui/review/       Review screen
internal/gui/write/        Write screen
internal/gui/kit/          Passbank's own widgets
internal/gui/testutil/     test fonts and helpers for widget and golden tests
internal/testenv/          test isolation (XDG folders), fixture loading
internal/third_party/argon2/ copy of x/crypto/argon2 exporting Argon2d (A12)
internal/archtest/         package-boundary checks
```

Boundary rules, checked by `internal/archtest`:
- `internal/core/...` imports neither Glutter nor `internal/cli` nor `internal/gui`
- `internal/cli` imports neither Glutter nor `internal/gui`
- `cmd/passbank` does not link Glutter
- only `cmd/passbank-gui` imports `internal/gui/...`

## 17. Testing

- **KDBX fixtures**: a database for every version, cipher, KDF and inner stream the reader accepts, plus databases with attachments, history, custom icons, custom data, 4.1 fields and unknown XML. They live in `internal/core/kdbx/testdata/` with their test passwords and key files. Each fixture has a line in `testdata/FIXTURES.md` saying what made it (program and version) and what it holds
- **Round trip**: read every fixture, write it in every output version, read it back, and compare the models. A version that cannot hold the fixture's data must refuse with `OutputVersionLosesData`; with the loss accepted, everything except the listed data must survive
- **Interoperability**: tests with the build tag `passbank_keepassxc` run `keepassxc-cli` against the written files of every version, and are skipped when it is not installed
- **Diff, plan and merge**: table-driven, with databases built in the test, covering every kind, choice and rule, histories, deletions, recycle bins, probable matches and the plan-file errors
- **CLI**: tested in-process with fake stdin, stdout, stderr and terminal, against golden text files
- **GUI**: widget tests and golden images with `gluttertest`; a display test for the full flow, run in the headless compositor
- **Fuzzing**: the container parser, the XML reader and the plan parser have seed corpora and fuzz tests

## 18. Milestones

1. **M1, Project setup**: `go.mod`, the scripts named in CLAUDE.md, `internal/archtest`, `internal/testenv`, lint configuration
2. **M2, Read KDBX4**: 4.0 and 4.1, every cipher and KDF, key files, the model, `passbank info`
3. **M3, Read KDBX 2.x and 3.x**
4. **M4, Write KDBX 4**: 4.0 and 4.1, safe writing, read-back verification, round-trip and KeePassXC tests
5. **M5, Write KDBX 2.x and 3.x**: the loss check, encryption limits per version
6. **M6, Diff and plan**: matching, changes, rules, the plan file, `passbank diff`
7. **M7, Merge**: applying a plan, `passbank merge`
8. **M8, GUI**: the three screens, plan load and save, display test
9. **M9, Release**: README, release builds, a manual check against real databases

`docs/IMPLEMENTATION.md` breaks these into steps.

## 19. Assumptions to confirm

Each was decided by Claude without the user's confirmation. They stay assumptions until the user confirms or changes them. Step 1.1 of `docs/IMPLEMENTATION.md` asks the user about all of them at once and records the answers here.

- **A1, How the CLI lets you choose**: a plan file plus rules (section 11, 12). Interactive prompts in the terminal are not in 1.0
- **A2, Probable matches**: the second matching pass by title, username and URL host, and its default (the newer copy)
- **A3, Per-entry choices**: you choose per whole entry; picking individual fields is not in 1.0
- **A4, Output credentials**: A's credentials by default
- **A5, Encryption defaults**: when A's settings aren't used: AES-256 with Argon2id at 64 MiB, 2 lanes and 10 iterations for KDBX 4; AES-KDF with 2,000,000 rounds for 3.x and 2.x
- **A6, Database settings**: settings come from A by default and are one change
- **A7, History**: the version not chosen is added to the entry's history
- **A8, No stored state**: the GUI keeps no state between runs
- **A9, ArcFourVariant**: the ArcFourVariant inner stream is rejected (no known KeePass version writes it by default)
- **A10, Old fixtures**: real test fixtures for KDBX 2.x and 3.0 need an old KeePass 2.x. Passbank's own writer (M5) can produce these versions, but a reader tested only against Passbank's writer proves little, because both could share the same misunderstanding of the format. If no old KeePass is available, the 2.x and 3.0 support is marked as checked only against Passbank's own files, and at least one real 3.1 file from KeePassXC is a fixture
- **A11, Lossy output versions**: writing a version that cannot hold the result's data is refused unless the user accepts the listed losses
- **A12, Argon2d**: `golang.org/x/crypto/argon2` provides Argon2id but not Argon2d (KeePassXC's default KDF). Its code supports Argon2d internally without exporting it. Passbank keeps a copy of that package (BSD-3-Clause, license kept in `third_party/`) in `internal/third_party/argon2`, changed only to export Argon2d, and checked against the RFC 9106 test vectors. This is the only copied cryptographic code
