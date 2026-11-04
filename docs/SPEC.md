# Passbank specification

This document is the source of truth for Passbank. Section 19 logs the decisions the user confirmed or changed on 2026-11-04.

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

Format references: KeePass's "KDBX 4" and "KDBX 4.1" pages (keepass.info/help/kb/kdbx_4.html, kdbx_4.1.html) and the KeePass 2 source. KeePass's and KeePassXC's sources are GPL: read them to understand behavior, never copy from them (Passbank is MIT). MIT-licensed implementations may also be read, and their test files used with their license kept in `third_party/`: gokeepasslib (Go), keepass-rs (Rust) and kdbxweb (JavaScript).

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

How the reader treats the XML:
- Only the `Value` of a `String` is decrypted from the inner stream, in document order (history entries included). A `Protected` attribute on any other element is an ordinary attribute and is kept with the unknown XML, and its text is not decrypted; this is what KeePass and KeePassXC do
- An element of a known object (the file, `Meta`, a group, an entry) that the model has no field for, or a field element that carries attributes, is kept whole as raw XML on that object. Unknown elements inside the small parts that have no raw XML of their own (`Times`, `MemoryProtection`, `AutoType`, an icon, a custom data item, a `String`, a deleted object) are dropped, as they are by KeePass
- Times are read in both forms KDBX has, ISO 8601 and base64 of the seconds since 0001-01-01, in any version
- An attachment that no entry refers to is not carried over, as in KeePass: it is invisible to the user
- Tags are split at `;` and `,`
- Settings that a file leaves out take KeePass's defaults (history of 10 items and 6 MiB, 365 days, the recycle bin on, the password protected in memory)

The only things dropped are those that only exist to encode the file itself:
- the KDBX3 `HeaderHash`
- seeds and IVs
- the inner stream key
- block framing

### 4.5 KDBX 2.x and 3.x, and how they differ from KDBX 4

Established in step 3.1 from the sources below, in Passbank's own words. Behavior only was taken from the GPL sources (KeePassXC's `Kdbx3Reader.cpp`, `Kdbx3Writer.cpp`, `KeePass2.h`); the permissively licensed ones (gokeepasslib, keepass-rs, kdbxweb) agree with them where they overlap, and real files from KeePass and KeePassXC were opened to check every statement marked (file).

**One container for 2.x, 3.0 and 3.1.** Every source dispatches on the major version only: KeePassXC reads any file whose major version is 3 or lower with one reader (its minimum is 2.0), keepass-rs and gokeepasslib treat major 3 alike, kdbxweb accepts 3.0 and 3.1. Passbank reads major versions 2 and 3, any minor, with one reader. The only differences between 2.x, 3.0 and 3.1 are in the XML: `Meta/HeaderHash` must be present from 3.1 on and may be missing before (KeePassXC); nothing else is conditioned on the minor version in any source.

**Layout** (after the 12 bytes of signatures and version):
1. The header: fields of 1 byte ID, 2 bytes length (little endian), data. IDs: 0 end (data `\r\n\r\n`), 1 comment, 2 cipher ID (16 bytes), 3 compression (u32: 0 none, 1 GZip), 4 master seed (32), 5 transform seed (32), 6 transform rounds (u64), 7 initialization vector (the cipher's size), 8 inner stream key (32), 9 stream start bytes (32), 10 inner stream ID (u32: 0 none, 1 ArcFourVariant (rejected, A9), 2 Salsa20, 3 ChaCha20 (a keepass-rs test file uses it; accepted)). IDs 11 and 12 (KDF parameters, public custom data) do not exist; an unknown ID is skipped, not an error (KeePassXC warns and continues). Required: cipher, master seed, transform seed, rounds, IV, inner stream key, start bytes. A missing one is `FileCorrupt`
2. The header is not authenticated by a hash or an HMAC after it. Its SHA-256 is stored in the XML (`Meta/HeaderHash`, below), over the bytes from the first signature to the end of the end-of-header field
3. The payload, to the end of the file, is encrypted with the outer cipher (AES-256, Twofish, or ChaCha20 with a 12-byte IV, which kdbxweb and gokeepasslib files use even in version 3.1; the reader accepts all three). Key derivation: the composite key is transformed with AES-KDF (the transform seed as the AES key, the rounds as the count: the same function as KDBX 4's, `crypto.KDF`), then the cipher key is SHA-256(master seed || transformed key). There is no challenge-response part (Passbank does not support it)
4. The decrypted payload starts with the 32 stream start bytes; if they differ from the header's, the credentials are wrong (`WrongCredentials`), the only check of the credentials in this format (KeePassXC; file)
5. Then the hashed block stream: blocks of u32 index (0, 1, 2, …), 32 bytes SHA-256 of the block's data, u32 size, the data. The last block has size 0 and a hash of 32 zero bytes. A wrong index or hash is `FileCorrupt`; a missing last block is `FileCorrupt`
6. Then, if compression is 1, GZip. The result is the XML

**XML differences from KDBX 4** (the model is the same; the reader of §4.4 handles both):
- **Times** are text, `2006-01-02T15:04:05Z` (UTC), not base64 seconds (file). Passbank's XML reader accepts both forms in any version
- **Attachments** are in `Meta/Binaries` as `<Binary ID="n" Compressed="True|False" Protected="True|False">` elements holding base64 (GZip of the content first when Compressed; XOR with the inner stream when Protected, in document order with the other protected values); an entry refers to one with `<Binary><Key>name</Key><Value Ref="n"/></Binary>`. Several entries may refer to one; an empty content is a self-closing element (file: KeePassXC 2.7). The pool of §4.4 is built from them. An inline `<Value Compressed=… >content</Value>` in an entry, which older programs wrote, is read the same way
- **`Meta/HeaderHash`**: base64 of the SHA-256 of the header. A mismatch is `FileCorrupt`; a missing element is an error only from 3.1 on
- **No 4.1 elements**: group tags and previous-parent-group, entry quality-check, previous-parent-group, `SettingsChanged`, custom icon names and modification times, and the modification time of custom data items do not exist; `CustomData` exists in `Meta` only (files from KeePass and KeePassXC have none on groups and entries). `Tags` exists on entries; KeePassXC writes them separated by `,`, KDBX 4 files by `;`, and the reader splits at both
- There is no `PublicCustomData`, no KDF parameter dictionary, no inner header, and the binary pool is not in the file's header

**Writing** a 2.x, 3.0 or 3.1 file is the reverse: the same header, AES-KDF, outer cipher AES-256 or Twofish, Salsa20 inner stream, GZip or none, and the XML with ISO times, `Meta/Binaries`, and `HeaderHash`. The version written is the one asked for; the layout is the same for all three. KDBX 3 AES-KDF uses the KDF ID `c9d9f39a-…` in KDBX 4 files too (keepass-rs does this "for compatibility"); KDBX 4 files from KeePassXC may carry `7c02bb82-79a7-4ac0-927d-114a00648238` for the AES-KDF, which the reader accepts as the same function.

**Decision (A10).** All of 2.x, 3.0 and 3.1 are built, because the format is established by four independent sources and is the same for the three. The 3.1 reader is checked against real files from KeePass (keepass-rs `test_db_with_password.kdbx`), KeePassXC 2.7 (the fixtures and keepass-rs `test_db_kdbx3_with_attachments.kdbx`) and kdbxweb (several, with a key file, ChaCha20, Cyrillic, empty password). No permissive repository has a 2.x or 3.0 file; those versions are checked by KeePassXC reading what Passbank writes (M5) and Passbank reading it back. If KeePassXC does not open a written 2.0 file, 2.x writing is given up at that step (and recorded).

## 5. Writing the result

- **Format**: any version the reader accepts: 2.x, 3.0, 3.1, 4.0 or 4.1.
  - The default is **KDBX 4**: version 4.0, or 4.1 when the result holds data that only 4.1 can store (group tags, previous-parent-group, custom icon names or modification times, modification times of custom data items, the settings-changed time, the entry quality-check flag). This matches KeePass's rule of writing the lowest version that holds the data
  - The user can pick an exact version instead: `--format` in the CLI, a version menu on the GUI's Write screen
  - 2.x, 3.0 and 3.1 are written with one layout, differing in the version number (§4.5)
- **Data the chosen version cannot hold**: when the result holds something the chosen version has no place for, the write is refused with `OutputVersionLosesData`. The error lists what would be lost and on which items. Examples:
  - for 3.x and 2.x: custom data on groups and entries, the `PublicCustomData` header field, and the 4.1 fields
  - for 4.0: the 4.1 fields

  The user can accept the loss explicitly: `--allow-loss` in the CLI, or a confirmation dialog in the GUI that shows the same list. Without that, nothing is written
- **Encryption per version**:
  - KDBX 3.x and 2.x support only AES-KDF and use the Salsa20 inner stream, with AES-256 or Twofish as the outer cipher
  - KDBX 4.x supports every cipher and KDF of §4.1 and uses the ChaCha20 inner stream
  - When the encryption settings taken from an input are not possible in the chosen version (for example Argon2 for 3.1), the write is refused with `EncryptionNotInVersion`, and the user picks the defaults or the other input's settings
- **Encryption settings** (cipher, KDF and its parameters, compression):
  - default: copied from A, if A is KDBX4
  - otherwise, the defaults: AES-256, GZip, and Argon2id with 64 MiB of memory, 2 lanes and 10 iterations for KDBX 4; AES-KDF with 2,000,000 rounds for KDBX 3.x and 2.x
  - the user can pick B's settings instead, or the defaults
- **Credentials**: the default is A's credentials, reused from the composite key that unlocked A, so the user doesn't type them again. The user can pick B's instead, or new ones (a password typed twice and/or a key file)
- **Fresh secrets**: seeds, IVs and the inner stream key are new random values from `crypto/rand` on every write
- **Safe writing**:
  - the output is written to a temporary file in the target folder with mode `0600`, synced, and renamed into place, and then the folder is synced
  - without that request the file is put in place by a hard link that fails if the path exists, so that a file made by someone else in the meantime is never replaced
  - an existing output file is replaced only when the user explicitly asked for it: `--force` in the CLI, or the save dialog's overwrite confirmation in the GUI
  - the output may never be the same file as A or B (checked with `os.SameFile`, also with `--force`): `OutputIsInput`
- **Verification after writing**: Passbank reads the temporary file back with the same credentials, before it is put in place, and compares its model with the result it meant to write (as the chosen version holds it). A file that fails the check never reaches the output path, and an existing file there is untouched. A mismatch is `WriteVerifyFailed`; the temporary file is removed, and the error is a bug to fix

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

Matching is by UUID only. Two databases created separately share no UUIDs, so every entry of each is "only in" its input, and Passbank never guesses that two entries with the same title are the same account. Databases that come from one another (copies, backups, the same database edited on two devices) keep their UUIDs and pair up.

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
| `deleted-in-a` | Entry in B whose UUID is in A's deleted objects | `keep`, `drop` | `drop` if the deletion is newer than B's last modification, else `keep` |
| `deleted-in-b` | The same, the other way round | `keep`, `drop` | as above |
| `group-differs` | Paired groups whose own properties differ (name, notes, icon, tags, expiry, settings, parent) | `take-a`, `take-b` | the newer |
| `settings` | The databases' `Meta` settings differ (name, description, default username, color, history limits, recycle-bin settings, memory protection) | `take-a`, `take-b` | `take-a` |

Choices are made per whole entry; choosing individual fields of an entry is not in 1.0.

What each choice means:
- **`both`** keeps both entries. A's copy keeps its UUID; B's copy gets a new UUID and is placed in the group that B had it in
- **`drop`** on a group drops the group and everything in it. It is allowed only when the user has chosen `drop` for every item under it; otherwise the plan is invalid (`PlanGroupNotEmpty`), and the GUI offers to set them all to `drop`
- **Entries only in one input, in a group that only exists in that input**: their group is kept automatically while at least one of them is kept

Changes are listed in a stable order: by kind (in the table's order), then by group path, then by title, then by UUID.

## 9. Choices and rules

A plan starts from the defaults in section 8. The user can then change choices one at a time, or apply a **rule** to many changes at once:
- `newer`: the defaults
- `a`: `take-a` for `differs`, `group-differs` and `settings`; `keep` for `only-a`; `drop` for `only-b`; and for deletions, A's state wins (if A deleted the entry, `drop`)
- `b`: the mirror image of `a`
- `union`: `keep` for everything only in one input, `both` for `differs`, and the defaults for the rest

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
- **The version not chosen**: when the chosen version differs from the other one, the other version is added to the history too, so the merge can be undone in any KeePass client
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
# differs: title
both    1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d  # Shopping / Store
settings take-a
```

- **Header**: the first line holds the format version. Lines `a` and `b` hold the SHA-256 of the input files' bytes and their names
- **Stale plans**: a plan whose hashes do not match the given inputs is rejected (`PlanStale`). A plan only fits the exact files it was made for
- **One line per change**: each change has one line, `<choice> <item>`. Comments (`#` to the end of the line) and blank lines are ignored
- **Errors**:
  - a missing change is `PlanIncomplete`
  - an item that is not a change of these inputs is `PlanUnknownItem`
  - a choice the kind doesn't allow is `PlanInvalidChoice`
  - a malformed line is `PlanSyntax`, with its line number
- **Secrets**: the file never contains passwords or other field values. Group paths and titles appear only in comments, and the CLI's `--no-titles` leaves them out, because titles can be sensitive

## 12. Command-line tool (`passbank`)

```
passbank diff  A B [--prefer newer|a|b|union] [-o PLAN] [--no-titles]
passbank merge A B -o OUT [--plan PLAN | --prefer newer|a|b|union]   (neither: interactive)
               [--format 4|4.1|4.0|3.1|3.0|2] [--allow-loss]
               [--settings-from a|b] [--credentials a|b|new] [--encryption a|b|default] [--force]
passbank info  FILE
```

- **`diff`**: unlocks A and B and writes a plan with the defaults (or the given rule) to `PLAN`, or to stdout. The user edits the plan, then runs `merge --plan`
- **`merge`**: builds and writes the result. With `--plan` or `--prefer` it runs without questions. With neither, it is **interactive** (section 12.1). `--format` picks the output version and defaults to `4` (4.0, or 4.1 when needed, as in §5); `--allow-loss` accepts the losses that a lower version causes. It prints a summary: the counts of each kind and choice, the output path and the format version
- **`info`**: prints a file's KDBX version, cipher, KDF and its parameters, compression, and counts of groups, entries, attachments and history items. It needs the credentials, because the counts are inside the encrypted part
- **Credentials**, for each input X (`a`, `b`) and for `out` when `--credentials new`:
  - `--keyfile-X PATH`
  - `--password-file-X PATH` reads the first line of a file. The file must be readable only by its owner, or Passbank refuses it
  - `--no-password-X` means the database has no password component
  - otherwise the password is read without echo from `/dev/tty`; a new output password is asked twice
  - an empty password (typed, or the first line of the password file) is not a password component, as in KeePassXC; with no key file either, the command fails with `NoCredentials`
  - `--no-password-X` together with `--password-file-X` is a mistake in the arguments (`UsageInvalid`, exit status 2)
- **Errors**: a failed command prints one line, `passbank <command>: <message>` (the messages of section 15), and exits with status 1; a mistake in the arguments exits with 2 and adds the usage line. `--debug` adds the tag, the route and the metadata of the error (never a secret)
  - if there is no terminal, the error is `PasswordNeedsTerminal`
  - passwords are never accepted as arguments or environment variables
- **Interactive input**: the password prompts and the interactive choices read from `/dev/tty` (the same terminal handle), so stdin and stdout stay free for pipes
- **Output streams**: messages go to stderr; the plan (in `diff`) and the summaries go to stdout
- **Exit codes**: `0` success, `1` failure, `2` usage error. Errors print one line with the message; `--debug` adds the route

### 12.1 Interactive merge

`passbank merge A B -o OUT` with neither `--plan` nor `--prefer` unlocks both inputs, prints the count of changes of each kind, and then goes through the changes one by one, in the stable order of section 8. For each change it shows:
- its position (`[3/17]`), kind, group path and title
- both modification times
- the names of the fields that differ
- the values of the non-secret standard fields that differ (Title, UserName, URL), side by side. Notes, custom fields, passwords and every other protected value are shown by name only

Then it asks for a choice. The prompt lists only the choices the kind allows, one key each, with the default marked:

| Key | Choice |
|---|---|
| `a` / `b` | take A / take B |
| `t` | both (keep two) |
| `k` | keep |
| `d` | drop |
| Enter | the default of section 8 |
| `v` | show the protected values that differ, for this change only; they are printed once and kept nowhere |
| `p` | go back to the previous change |
| `r` | apply a rule of section 9 to this change and all the remaining ones |
| `?` | help |
| `q` | quit; nothing is written (exit code 1) |

Dropping a group that still holds kept items asks "also drop the N items inside it? [y/N]"; answering no keeps the group. After the last change it prints the summary of section 12 and asks "Write OUT? [y/N]" before writing. Without a terminal, the command fails with `ChoicesNeedTerminal`, which tells the user to use `--plan` or `--prefer`.

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
- **State**: the GUI keeps no state between runs: no recent files, no settings files
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
| `FileReadFailed`, `FileTooLarge`, `ReadCancelled` | "<file> cannot be read: <cause>"; "<file> is larger than 2 GiB"; "reading was cancelled" |
| `PlanStale`, `PlanIncomplete`, `PlanUnknownItem`, `PlanInvalidChoice`, `PlanSyntax`, `PlanGroupNotEmpty` | what is wrong, with the line or item |
| `ChoicesNeedTerminal` | "There is no terminal to ask for choices; use --plan or --prefer" |
| `OutputExists` | "<out> exists; use --force to replace it" |
| `OutputVersionLosesData` | "KDBX <v> cannot store <what> of <n> items; choose KDBX 4 or accept the loss" |
| `EncryptionNotInVersion` | "KDBX <v> does not support <cipher or KDF>; choose other encryption settings" |
| `OutputIsInput` | "The output must be a new file, not one of the inputs" |
| `PasswordNeedsTerminal`, `PasswordsDoNotMatch`, `PasswordFileTooOpen` | |
| `WriteVerifyFailed` | "The written file did not read back correctly and was removed" (a bug) |
| `OutputPathInvalid`, `OutputTempFailed`, `OutputWriteFailed`, `OutputPlaceFailed`, `OutputSyncFailed`, `WriteCancelled` | "<out> cannot be written: <cause>"; nothing is left behind, except that after `OutputSyncFailed` the file is in place and complete but may not survive a crash |

## 16. Packages

```
cmd/passbank/              CLI main
cmd/passbank-gui/          GUI main
internal/core/secret/      secret byte buffers
internal/core/model/       data model (section 6)
internal/core/kdbx/        file container: signature, headers, block streams, compression, inner stream; XML <-> model, raw-XML preservation, time formats
internal/core/kdbx/crypto/ ciphers, KDFs, composite key, key files
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
internal/version/          the version both programs print for --version
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
- **Diff, plan and merge**: table-driven, with databases built in the test, covering every kind, choice and rule, histories, deletions, recycle bins and the plan-file errors
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
7. **M7, Merge**: applying a plan, `passbank merge`, the interactive choices
8. **M8, GUI**: the three screens, plan load and save, display test
9. **M9, Release**: README, release builds, a manual check against real databases

`docs/IMPLEMENTATION.md` breaks these into steps.

## 19. Decisions log

These decisions were proposed by Claude and confirmed or changed by the user on 2026-11-04 (step 1.1 of `docs/IMPLEMENTATION.md`).

- **A1, How the CLI lets you choose**: **changed by the user.** The CLI is **interactive**: `passbank merge` asks about each change on the terminal (section 12.1). The plan file and `--prefer` rules stay, as the way to run without questions and in scripts
- **A2, Matching**: **changed by the user.** Entries and groups pair **by UUID only** (section 7). There is no matching by title, username or URL
- **A3, Per-entry choices**: confirmed. You choose per whole entry; picking individual fields is not in 1.0
- **A4, Output credentials**: confirmed. A's credentials by default
- **A5, Encryption defaults**: confirmed. When A's settings aren't used: AES-256 with Argon2id at 64 MiB, 2 lanes and 10 iterations for KDBX 4; AES-KDF with 2,000,000 rounds for 3.x and 2.x
- **A6, Database settings**: confirmed. Settings come from A by default and are one change
- **A7, History**: confirmed. The version not chosen is added to the entry's history
- **A8, No stored state**: confirmed. The GUI keeps no state between runs
- **A9, ArcFourVariant**: confirmed. The ArcFourVariant inner stream is rejected
- **A10, Old versions**: **changed by the user.** KDBX 2.x and 3.x are implemented by Passbank itself, from the format documentation and from the permissively licensed implementations of section 4.1, with no old KeePass needed to make fixtures (3.1 fixtures come from KeePassXC if it can make them, and from the test files of the permissively licensed implementations otherwise). If step 3.1 finds that the format of a version cannot be established with confidence from those sources, that version is **given up**: it is removed from this spec and from the plan, and the removal is recorded in `docs/IMPLEMENTED.md` **Outcome of step 3.1 (2026-11-05):** nothing is given up. The three versions share one container and one XML dialect (section 4.5), four independent sources agree, and 3.1 has real files from KeePass and KeePassXC; 2.x and 3.0 are verified by KeePassXC reading what Passbank writes (M5)
- **A11, Lossy output versions**: confirmed. Writing a version that cannot hold the result's data is refused unless the user accepts the listed losses
- **A12, Argon2d**: confirmed. `golang.org/x/crypto/argon2` provides Argon2id but not Argon2d (KeePassXC's default KDF). Passbank keeps a copy of that package (BSD-3-Clause, license kept in `third_party/`) in `internal/third_party/argon2`, changed only to export Argon2d, and checked against the RFC 9106 test vectors. This is the only copied cryptographic code
