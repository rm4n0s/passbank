# Passbank: Implementation guide

This file breaks the milestones of `docs/SPEC.md` §18 into small steps. Each step is one unit of work for one coding session. Work goes strictly top to bottom, and every finished step is recorded in `docs/IMPLEMENTED.md`.

**Who this is for**: the coding agent (or human) implementing Passbank. You don't need to read the whole file: you need the Rules below and the one step you are doing.

---

## Rules

### Before you start a step

1. **Find your step.** It is the first step in this file that has no entry in `docs/IMPLEMENTED.md`. Do only that step. Never work on two steps at once, and never skip ahead.
2. **Read, in this order:**
   1. `CLAUDE.md`, all of it.
   2. The whole step in this file.
   3. Every section of `docs/SPEC.md` listed under **Read**.
   4. For format work, the sources listed under **Reference**.
   5. For UI steps, the Glutter APIs the step names, in Glutter's source in the Go module cache (`go list -m -f '{{.Dir}}' github.com/rm4n0s/glutter`), and Glutter's `docs/GUIDE.md`.
3. **Create the branch.** If `docs/IMPLEMENTATION.md` is not on `main`, stop and ask the user to commit it there first.
   ```sh
   git switch main
   git switch -c feat/step-<id>-<slug>     # e.g. feat/step-2.4-kdbx4-container
   ```

### While you work

4. **Do exactly the step's Tasks.** Don't do work that belongs to a later step, even if it looks easy. If a later step will need a hook, leave only a comment: `// TODO(step 6.3): ...`.
5. **Follow CLAUDE.md.** In particular:
   - `rm4n0s/errors` for every error, with a route test for every Tag
   - no cgo, and no Glutter import outside `internal/gui` and `cmd/passbank-gui`
   - no secret in an error, a log, a golden file or a proof image
   - isolated paths in every test and script
6. **Match KeePass.** For the file format, KeePass 2's behavior is the reference: its "KDBX 4" and "KDBX 4.1" pages and its source (`KeePassLib/Serialization/`). KeePass's and KeePassXC's sources are GPL: read them to understand behavior, never copy from them. MIT-licensed implementations (gokeepasslib, keepass-rs, kdbxweb) may be read, and their test files used with their license kept in `third_party/`. Where KeePass and KeePassXC disagree, write it down in the step's `IMPLEMENTED.md` entry and follow the spec.
7. **Glutter gaps.** When Glutter is missing something or has a bug:
   - work around it in Passbank, and mark the code with `// Glutter: <what and why>`
   - add an entry to `docs/GLUTTER-ISSUES.md`, creating the file if needed
   - mention it in the report (CLAUDE.md "Glutter")
8. **Stop and ask the user** when any of these happens:
   - the step is ambiguous
   - the spec looks wrong
   - a tool is missing (give the exact install command, e.g. `sudo dnf install keepassxc`)
   - a new dependency seems necessary
   - something needs `sudo`
   - a check still fails after a real attempt to fix it
   - the step says **ask the user**

   Never weaken or delete a test, skip a check, or mark a step done just to get unblocked. **Manual checks** don't stop the work: follow CLAUDE.md "Working through the steps".

**Processes you start.** Every compositor or GUI instance you start for a test, a proof or a check is stopped when you are done with it. Run it under `timeout -k`, in its own process group, and stop it from `t.Cleanup` or `defer` in Go code. Before you finish a step, check with `pgrep -a` that nothing you started is still running.

**Isolation.** Nothing you run may use the user's real configuration, data or databases:
- Go tests call `testenv.Isolate(t)` (step 1.3)
- scripts run the apps through `scripts/isolated.sh` or `scripts/proof.sh` (step 1.4)
- only fixtures under `testdata/` and files the tests create are opened

### Finishing a step

9. **The standard checks must all pass.** `scripts/checks.sh` runs all of the list below, the heavy ones through `scripts/heavy.sh`. By hand, from the repo root:
   ```sh
   test -z "$(gofmt -l cmd internal)"
   go vet ./...
   golangci-lint run
   GOMEMLIMIT=3GiB go test -race -p 2 ./...
   CGO_ENABLED=0 go build ./...
   CGO_ENABLED=0 go test ./...
   scripts/error-tags.sh --untested     # prints nothing (from step 1.3 on)
   ```
   - If the step lists **Display tests**, run them too: `scripts/heavy.sh --alone -- scripts/headless-wayland.sh -- go test -race -tags passbank_display <packages>`
   - If it lists **Interop tests**, run `go test -tags passbank_keepassxc <packages>`
   - Golden tests have `Golden` in their name, so `go test ./... -run Golden -update-goldens` finds them all
10. **Proof images.** Only when the step has a **Proof** item:
    - Produce exactly the images it lists, in `docs/implemented-proof-images/`, named `<id>-<name>.png` (e.g. `8.3-review.png`), with the command the step gives.
    - **Open every image with your image-viewing tool** (the Read tool shows images) and check that it shows what **Proof** describes. If it doesn't, fix the code: the step isn't done.
    - Never draw, edit, crop or generate a proof image any other way, and never reuse an older image.
    - Proof images show only fixture data, never real passwords.
11. **Record the step.** On the step branch, add an entry to `docs/IMPLEMENTED.md`, using the format at the top of that file. Embed the proof images.
12. **Commit on the step branch.** Make one commit per step, with the message `Step <id>: <imperative summary>`:
    ```sh
    git add -A && git commit -m "Step 2.4: Read the KDBX 4 container"
    ```
13. **Merge every finished step into `main`; publish to GitHub once per milestone.**
    - Merge a step only after rules 9–12 are complete: checks green, proofs verified, `IMPLEMENTED.md` entry committed. Never collect several steps into one merge, and never leave a finished step only on its branch:
      ```sh
      git switch main
      git merge --ff-only feat/step-2.4-kdbx4-container
      git tag step-2.4
      git branch -d feat/step-2.4-kdbx4-container
      ```
    - A step counts as finished when its commit is on the local `main` and tagged `step-<id>`. Start the next step from there.
    - **Publish a milestone.** When the last step of a milestone (a `## M<n>` section below) is merged, push `main` and the tags of its steps:
      ```sh
      git push origin main
      git push origin --tags
      ```
      The milestone counts as published when that commit is on `origin/main`.
    - If `--ff-only` fails, rebase the step branch onto `main`, run the standard checks again, and retry.
    - If a push is rejected or fails (authentication, network), retry a few times. If it keeps failing, stop and tell the user.
    - Don't start the next milestone's first step until the milestone's push succeeded.
14. **Report to the user.** Give:
    - the step id and a two-line summary
    - the check results and the proof image paths
    - new `docs/GLUTTER-ISSUES.md` entries and new manual checks
    - the commit hash now on `main` (and, at the end of a milestone, that it is on `origin/main`)

    Then start the next step.

**Default "Done when".** A step without a **Done when** line is done when:
- its Tasks are complete
- its **Tests** are written and pass
- its **Display tests** and **Interop tests** pass
- its **Proof** images are verified
- the standard checks pass

### Proof methods

| Method | Use it for | How |
|---|---|---|
| **A. Widget render** | a screen or widget with given state (fixture databases already unlocked, a given plan) | A widget test renders it with `internal/gui/testutil` (test fonts, isolated paths). When run with `-proof`, it writes `Tester.Screenshot()` as a PNG to `docs/implemented-proof-images/<file>.png`. Run only that package: `go test ./internal/gui/<pkg> -run <TestName> -proof` |
| **B. App self-capture** | the real window | `scripts/proof.sh docs/implemented-proof-images/<file>.png [--after <duration>] -- <passbank-gui args>`. It builds a debug binary in a temporary folder, isolates the `XDG_*` folders, runs the binary in a headless compositor with `GLUTTER_SCREENSHOT` (and `GLUTTER_SCREENSHOT_AFTER` from `--after`), and checks that the PNG was written |
| **C. Terminal capture** | CLI output | The proof is the golden text file of the CLI test, quoted in the `IMPLEMENTED.md` entry; there is no image |
| **D. User desktop** | things only a real desktop shows: the portal file dialogs, dropping a file from the file manager | A manual check: the user runs the given command and saves a screenshot at the given path. When it arrives, view it, verify it and record it |

Every **Proof** item gives its exact command.

---

## Milestone overview

| Milestone (SPEC §18) | Steps |
|---|---|
| M1 Project setup | 1.1–1.4 |
| M2 Read KDBX 4 | 2.1–2.8 |
| M3 Read KDBX 2.x and 3.x | 3.1–3.3 |
| M4 Write KDBX 4 | 4.1–4.4 |
| M5 Write KDBX 2.x and 3.x | 5.1–5.3 |
| M6 Diff and plan | 6.1–6.5 |
| M7 Merge | 7.1–7.6 |
| M8 GUI | 8.1–8.7 |
| M9 Release | 9.1–9.2 |

---

## M1: Project setup

### 1.1 Confirm the assumptions
**Goal:** every **(assumed)** decision of SPEC §19 confirmed or changed by the user, so the later steps don't stop for them.
**Read:** SPEC, all of it; §19 closely.
**Tasks:**
1. **Ask the user** about A1–A12 in one message. Give each one with a one-line explanation and the proposed answer, and ask for confirmation or a change. For A10, also ask whether the user can run an old KeePass 2.x to make KDBX 2.x and 3.0 fixtures.
2. Record the answers in the spec on this step's branch:
   - remove each **(assumed, A<n>)** marker that the user confirmed
   - rewrite the sections whose decision changed, and change the steps of this file that the change touches
   - turn §19 into a log: each assumption, the answer, and the date
3. If an answer changes a dependency, update CLAUDE.md "Go conventions" too.

**Done when:** SPEC.md has no **(assumed** marker left, §19 records every answer, and the plan agrees with the spec. There is no code and no Go module yet, so the standard checks are skipped; the `IMPLEMENTED.md` entry says so.

### 1.2 Module, package layout and the architecture checks
**Goal:** an empty but complete skeleton that passes every check.
**Read:** SPEC §16; CLAUDE.md "Layout", "Go conventions".
**Tasks:**
1. `go mod init github.com/rm4n0s/passbank`, with Go 1.27 (the installed toolchain).
2. For every package of SPEC §16, create the directory with a `doc.go` whose package comment states its purpose and the SPEC section. `cmd/passbank/main.go` and `cmd/passbank-gui/main.go` are empty `main` functions.
3. Add `.golangci.yml` (golangci-lint v2 format): the standard set, plus `revive` with `exported` and `package-comments`, and `gofmt` as formatter.
4. Add `internal/archtest`, with table tests that parse the imports and calls of every package with `go/parser` and enforce:
   - the boundary rules of SPEC §16
   - no `import "C"`
   - no standard library `errors` import, and no `fmt.Errorf` call
   - no `net`, `net/http` or `crypto/tls` anywhere (SPEC §14)
   - `golang.org/x/term` only in `internal/cli`

   The checker is a function that takes a root folder, so each rule is tested on small fixture trees under `internal/archtest/testdata/` (gofmt-clean, never built).
5. Add the "`cmd/passbank` does not link Glutter" check: a test that runs `go list -deps ./cmd/passbank` and fails if a `github.com/rm4n0s/glutter` package is listed.
6. Add `NOTICE` (Passbank, MIT) and `third_party/README.md`.

**Tests:** each archtest rule, with a fixture that violates it and is reported, and one that passes.

### 1.3 Test isolation, secrets and the error-tag script
**Goal:** `internal/testenv`, `internal/core/secret`, and `scripts/error-tags.sh`.
**Read:** SPEC §14, §15; CLAUDE.md "Error handling".
**Tasks:**
1. `github.com/rm4n0s/errors` is already in `go.mod` (step 1.2 needed it for `archtest`). Read its `AI_GUIDE.md` in the module cache.
2. `testenv.Isolate(t)`: sets every `XDG_*` variable and `HOME` to folders under `t.TempDir()` with `t.Setenv`. `testenv.Fixture(t, name)` returns the path of a file under the calling package's `testdata/`, and fails the test if it is missing.
3. `internal/core/secret`: a `Bytes` type that holds secret data, with `Wipe()` (zeroing), `Equal` (constant time, `crypto/subtle`), and a `String()` / `LogValue()` that print `[secret]`, so a secret can't be printed or logged by accident. `FromTerminal`-style constructors come later; this step needs only `New([]byte)` (which takes ownership) and `Clone`.
4. `scripts/error-tags.sh [--untested]`:
   - lists every Tag created in non-test Go code (the first string argument of `errors.New`, `errors.NewErr` and `errors.NewErrf`, and the `Tag:` of sentinel struct literals)
   - with `--untested`, prints the Tags that no `_test.go` file mentions in a `HasRoute` call, minus those listed in `docs/ERROR-TAGS-UNTESTED.md`

   Adapt Emetos's script: `git clone --depth 1 https://github.com/rm4n0s/emetos <scratch dir>` and then `git -C <scratch dir> checkout 4d0c4b55aae66ed505b018d976337330f5659555`. Emetos is the user's own MIT project; delete the clone afterwards.

**Tests:** `Isolate` sets and restores the variables; `secret.Bytes` wipes, compares, and never prints its content (with `fmt` verbs and `slog`); a test of `error-tags.sh` on a fixture tree.

### 1.4 Scripts, first window and the CLI skeleton
**Goal:** `go run ./cmd/passbank-gui` opens an empty window; `passbank --version` works; isolated runs, heavy-command slots and proofs work.
**Read:** SPEC §12, §13; CLAUDE.md "Commands", "Memory (OOM)", "Testing notes". Glutter: `widgets/run_app.go` (`RunApp`, `RunAppOpts`, `FontSource`), `README.md`, `scripts/headless-wayland.sh`, `scripts/screenshot-headless.sh`.
**Tasks:**
1. `go get github.com/rm4n0s/glutter@v1.9.9`.
2. Copy `scripts/headless-wayland.sh` and `scripts/screenshot-headless.sh` from Glutter's module folder (and `chmod +x`, as the module cache is read-only). From the Emetos clone (as in 1.3), adapt `scripts/heavy.sh`, `scripts/gpu-env.sh`, `scripts/isolated.sh` and `scripts/proof.sh`:
   - change "Emetos" to "Passbank" and `emetos` to `passbank`
   - drop the options Passbank doesn't have (`--config`, `--commands`, `--launch`, `--output`)
   - keep the NVIDIA rule of `gpu-env.sh` exactly (CLAUDE.md "Testing notes")
3. Fonts: the Go fonts (`golang.org/x/image/font/gofont/goregular`, `gobold`, `gomono`, already brought in by Glutter) are embedded through `internal/gui/kit/fonts` (`Sources()` for `RunAppOpts.Fonts`; family-name constants). The app and the tests use only these; `internal/gui/testutil` loads them in `gluttertest`.
4. `internal/gui/app.Run(ctx)`: `widgets.RunApp` with `AppID: "dev.rm4n0s.Passbank"`, title "Passbank", size 1100×720, the fonts, and a home that shows "Passbank" centered on the app's background color. `cmd/passbank-gui` parses `--version` and `--help` and calls it.
5. `internal/cli.Main(ctx, args, stdin, stdout, stderr, term) int`: command dispatch with `--version`, `--help` and usage errors (exit 2, SPEC §12). `term` is an interface for the terminal (is it one, read a line without echo), so tests use a fake. The real terminal (`golang.org/x/term` on `/dev/tty`) is implemented in `internal/cli` (archtest allows `x/term` only there) and `cmd/passbank` just calls `cli.Main` with it.

**Tests:** a widget test that pumps the home and finds the text; the fonts load; `cli.Main` for `--version`, `--help`, an unknown command (exit 2); `scripts/isolated.sh` leaves no folder behind (a test that runs it with `env`).
**Proof:** method B, `scripts/proof.sh docs/implemented-proof-images/1.4-first-window.png -- `: the window with "Passbank" in the center.

---

## M2: Read KDBX 4

### 2.1 The data model
**Goal:** `internal/core/model` as SPEC §6 describes, with deep comparison.
**Read:** SPEC §6, §4.4.
**Tasks:**
1. The types of SPEC §6:
   - `Database`, `Meta`, `Group`, `Entry`, `Times`, `StringField` (value as `secret.Bytes` when protected, plain otherwise), `AutoType`, `CustomIcon`, `CustomDataItem`, `DeletedObject`
   - the attachment pool, keyed by SHA-256, with each attachment's protected flag
   - `RawXML` for preserved unknown elements and attributes
2. `model.UUID` (16 bytes) with parsing and canonical formatting (`xxxxxxxx-xxxx-…`).
3. `model.Diff(a, b *Database) []string`: a deep comparison that returns readable paths of every difference (`Root/Email/Entry 3f1c…/Fields/Password`), or nothing when equal. It is used by the round-trip tests and the read-back verification. Secret values are compared, never printed.
4. Tree helpers: walk in document order, find by UUID, group path of an item.

**Tests:** UUID parse and format; `Diff` finds each kind of difference and prints no secret; the walk order.

### 2.2 Ciphers and key derivation
**Goal:** every cipher and KDF of SPEC §4.1 behind small interfaces in `internal/core/kdbx/crypto`.
**Read:** SPEC §4.1, §19 (A12); KeePass "KDBX 4" page.
**Reference:** RFC 9106 (Argon2), RFC 8439 (ChaCha20).
**Tasks:**
1. `go get golang.org/x/crypto@latest`.
2. Ciphers, by UUID: AES-256-CBC (stdlib), Twofish-CBC (`x/crypto/twofish` with `cipher.NewCBCDecrypter`), and ChaCha20 (`x/crypto/chacha20`, 12-byte nonce). Decrypt with PKCS#7 padding checks for the CBC ciphers, and encrypt (needed from M4).
3. KDFs:
   - AES-KDF (rounds of AES-256-ECB over the 32-byte seed, then SHA-256)
   - Argon2id (`x/crypto/argon2.IDKey`)
   - Argon2d through `internal/third_party/argon2` as A12 says: a copy of `x/crypto/argon2` at the version in `go.mod`, with Argon2d exported and the BSD license in `third_party/x-crypto/LICENSE` and `NOTICE`
4. Each KDF checks its parameters before running: memory, iterations and lanes within sane bounds (memory at most 4 GiB, iterations at most 2^32-1, lanes 1–256). This is SPEC §14's "bounded before allocation"; parameters out of bounds give `KdfParametersRejected`.
5. Inner streams: Salsa20 (`x/crypto/salsa20`, KeePass's fixed nonce) and ChaCha20 (key and nonce from SHA-512 of the stream key), each as a keystream that XORs protected values in document order.

**Tests:** known-answer tests for each cipher and KDF (the RFC vectors for Argon2d and Argon2id; vectors from KeePass's source for AES-KDF and the inner streams), padding errors, parameter bounds, route tests.

### 2.3 Key files and the composite key
**Goal:** `internal/core/kdbx/crypto` credentials, as SPEC §4.2 describes.
**Read:** SPEC §4.2; KeePass's `KcpKeyFile.cs` and `CompositeKey.cs`.
**Tasks:**
1. `Credentials{HasPassword, Password, HasKeyFile, KeyFile}` and `CompositeKey(Credentials) (secret.Bytes, error)` (no `ctx`: it does no I/O), and `ReadKeyFile(ctx, path)`, which reads a key file up to 64 MiB.
2. Key-file loading in KeePass's order:
   - XML 1.0 and 2.0 (verifying the 2.0 `Hash`: `KeyFileCorrupt`)
   - 32 raw bytes
   - 64 hex characters
   - otherwise SHA-256 of the contents

   Key files larger than 64 MiB are rejected (`KeyFileTooLarge`).

**Tests:** every key-file format, a 2.0 file with a wrong hash, an empty password versus no password, the composite key against KeePass's construction for password only, key file only, and both.

### 2.4 Fixtures
**Goal:** the KDBX fixtures of SPEC §17: KDBX 4.0 and 4.1 now, and the 3.1 files that step 3.1 will use.
**Read:** SPEC §17; `keepassxc-cli --help` and the help of `import`, `db-create` and `add`.
**Tasks:**
1. Make the fixtures with programs other than Passbank. KeePassXC's command line (`keepassxc-cli`, or the Flatpak `org.keepassxc.KeePassXC`, run with `flatpak run --filesystem=<work folder> --command=keepassxc-cli org.keepassxc.KeePassXC`) writes only KDBX 3.1, so it makes the 3.1 files and the key files; for KDBX 4 use pykeepass (pinned, in a temporary virtualenv: a development tool, not a dependency of Passbank), whose files KeePassXC must be able to read. If neither KeePassXC nor Python with `venv` and network access is available, **ask the user**.
2. `internal/core/kdbx/testdata/make-fixtures.sh` makes them in a temporary folder with its own HOME and XDG folders, checks each with KeePassXC, and copies them to `testdata/`; `gen/rich.py` writes the XML of the rich content and `gen/make_kdbx4.py` makes the KDBX 4 files (cipher, KDF, compression, key file, header variants).
3. The set: empty and rich databases; every cipher (AES-256, ChaCha20, Twofish), every KDF (AES-KDF, Argon2d, Argon2id), compression on and off, password only, key file only and both, a public-custom-data header field, a 4.1 file, a file with unknown XML, and the KDBX 3.1 versions of empty, rich, key file and both.
4. `testdata/FIXTURES.md` lists every file (program, version, cipher, KDF, credentials, content) and the facts the tests may rely on; `fixtures_test.go` checks that the folder and the list agree and that each file has the version its name says.
5. What no program available here can make goes to `docs/MANUAL-CHECKS.md` as an optional check (a KDBX 4 file from KeePassXC's own writer, a real 4.1 file); no step waits for it.

**Done when:** the fixtures are committed, `make-fixtures.sh` has opened every one with KeePassXC, FIXTURES.md lists them all, and the standard checks pass.

### 2.5 The KDBX 4 container
**Goal:** from a file's bytes and credentials to the decrypted XML, the inner header and its binaries.
**Read:** SPEC §4.1–§4.3, §14; KeePass "KDBX 4" page and `KdbxFile.Read.cs`.
**Tasks:**
1. Signature and version detection for every case of SPEC §4.1 and §15: `NotKdbx`, `KeePass1NotSupported`, `VersionUnsupported`, and the pre-release signature.
2. The outer header: the TLV fields and the `VariantDictionary` (KDF parameters, `PublicCustomData`). Every length is bounded before allocation; unknown field IDs are an error, as in KeePass.
3. Verify the header SHA-256 (`FileCorrupt`) and the HMAC (`WrongCredentials`), then the HMAC block stream, decryption and decompression. Gzip output is bounded at 1 GiB (`FileCorrupt`, "too large").
4. The inner header: the inner stream ID and key, and the binaries (with their protected flag).
5. Unknown cipher, KDF, compression or inner stream IDs give the errors of SPEC §15.

**Tests:** every 4.x fixture opens to XML; wrong password; corrupt bytes at each layer (header, HMAC, a block, the gzip stream); truncated files; route tests for every Tag.

### 2.6 The KDBX 4 XML
**Goal:** from the decrypted XML to `model.Database`, for KDBX 4.0 and 4.1.
**Read:** SPEC §4.4, §6; KeePass `KdbxFile.Read.Streamed.cs`.
**Tasks:**
1. A streaming `encoding/xml` reader that builds the model, decrypts protected values with the inner stream in document order, and resolves binary references into the attachment pool.
2. Times: base64 of int64 seconds since 0001-01-01 UTC.
3. Every element and attribute that the model has no field for is kept as `RawXML` on its parent object, in order, so the writer (4.1) can put it back.
4. Depth, count and size limits: group nesting at most 1,000, and a string value at most 256 MiB (`FileCorrupt`).

**Tests:** every 4.x fixture is read, and spot checks compare its model with what FIXTURES.md says it holds; unknown XML is kept; protected values decrypt; route tests.

### 2.7 Opening a database
**Goal:** one entry point for reading any supported file: `kdbx.Open(ctx, path, creds) (*model.Database, kdbx.Info, error)`.
**Read:** SPEC §4, §14, §15.
**Tasks:**
1. `Open` reads the file (bounded at 2 GiB) and dispatches by version. The KDBX3 branch returns `VersionUnsupported` until M3. `Open` honors `ctx` between the KDF and the parsing.
2. `Info`: version, cipher, KDF and parameters, compression, inner stream, counts of groups, entries, attachments and history items.
3. Fuzz tests for the container and the XML reader, with the fixtures as seed corpus (CLAUDE.md "Memory (OOM)" limits).

**Tests:** every fixture through `Open`; cancellation during the KDF; the fuzz seed corpus runs in the normal test run.

### 2.8 `passbank info`
**Goal:** the `info` command of SPEC §12, and the credential options used by every command.
**Read:** SPEC §12, §14.
**Tasks:**
1. `golang.org/x/term` is already in `go.mod` (step 1.4 added it for the real terminal).
2. Credential options for a named input (`--keyfile-X`, `--password-file-X`, `--no-password-X`). The terminal prompt reads without echo from the `term` interface and returns `secret.Bytes`. `--password-file-X` refuses a file readable by group or others (`PasswordFileTooOpen`). With no terminal, the error is `PasswordNeedsTerminal`. For `info`, the input's name is just `--keyfile`, `--password-file` and `--no-password`.
3. `passbank info FILE`: prints the `Info` fields, one per line, in a stable order.
4. Error output: one line with the message, and the route with `--debug`.

**Tests:** in-process CLI tests with a fake terminal, against golden text files, for fixtures of each kind, a wrong password, no terminal, and a password file that is too open.
**Proof:** method C, the golden output of `passbank info` on the richest 4.x fixture.
**Manual check:** the success path of the real terminal (`cli.TTY`, step 1.4) cannot run without a terminal. Add to `docs/MANUAL-CHECKS.md`: the user runs `go run ./cmd/passbank info internal/core/kdbx/testdata/<fixture>.kdbx` in a real terminal, types the fixture's password (FIXTURES.md), sees nothing echoed, and sees the file's summary.

---

## M3: Read KDBX 2.x and 3.x

### 3.1 Version differences
**Goal:** SPEC §4.5 written from public sources, and a decision on whether KDBX 2.x and 3.x can be built (SPEC §19, A10).
**Read:** SPEC §4.1, §4.5, §19 (A10); the sources below.
**Reference:**
- KeePass's format pages, and its source (`KeePassLib/Serialization/KdbxFile*.cs`, with the version constants and every `if (m_uFileVersion …)` branch), read for behavior only because it is GPL
- KeePassXC's C++ (`src/format/Kdbx3Reader.cpp`, `Kdbx3Writer.cpp`, `KdbxXmlReader.cpp`, `KdbxXmlWriter.cpp`), also GPL: read for behavior only
- the permissive implementations, which may be read freely: gokeepasslib (Go, MIT), keepass-rs (Rust, MIT) and kdbxweb (JavaScript, MIT). Fetch each with `git clone --depth 1` into a scratch folder, check its `LICENSE`, and delete the clone afterwards
**Tasks:**
1. Write SPEC §4.5 for 2.x, 3.0 and 3.1: the header fields, the payload framing, the XML elements and formats (times, binaries, `HeaderHash`), and how each version differs from the next. Cite the source and the condition for each difference. Write it in your own words, not translated from any GPL code.
2. Decide per version, and write the decision in §4.5 and in this step's `IMPLEMENTED.md` entry:
   - **Build it** when at least two independent sources agree on every difference, and a real file or test vector exists to check the reader against.
   - **Give it up** otherwise (the rule of A10): remove that version from SPEC §1, §2, §4.1 and §5, remove it from steps 3.2, 3.3, 5.1, 5.2 and 5.3 and from the tests of M4, and say so in the report. A version given up is rejected with `VersionUnsupported`.
3. Fixtures: the KDBX 3.1 files exist since step 2.4 (made by KeePassXC). For 2.x and 3.0 there are none (A10): use the test files of a permissive implementation if one has them (with their passwords and licenses, in `third_party/` and in FIXTURES.md), else give the version up as A10 says. Passbank's own writer (M5) is never the only check of its reader: every version built has at least one file Passbank did not write.

**Done when:** §4.5 is complete and cites sources, every version has a decision, and FIXTURES.md lists the old-version fixtures. The spec and plan changes are in this step's commit.

### 3.2 The KDBX 3 container
**Goal:** from a 2.x or 3.x file to decrypted XML.
**Read:** SPEC §4.1–§4.3, §4.5.
**Tasks:**
1. The KDBX3 header fields of §4.5, with AES-KDF (transform seed and rounds), and the master key from the master seed and the transformed key.
2. Decryption, then the stream start bytes check (`WrongCredentials`), the hashed block stream (`FileCorrupt` on a hash mismatch), and decompression.
3. Inner streams: Salsa20 and none; ArcFourVariant is handled as A9's answer says.

**Tests:** every 3.x and 2.x fixture opens to XML; wrong password; corruption at each layer; route tests.

### 3.3 The KDBX 3 XML and `Open` for every version
**Goal:** old XML into the same model; `kdbx.Open` and `passbank info` for every version.
**Read:** SPEC §4.4, §4.5.
**Tasks:**
1. The XML differences of §4.5: ISO 8601 times, `Meta/Binaries` into the attachment pool, and the `HeaderHash` check (a mismatch is `FileCorrupt`).
2. `Open` dispatches 2.x and 3.x to the new reader; the fuzz seeds include the old fixtures.

**Tests:** every fixture of every version through `Open`; the same content saved as 3.1 and as 4.0 (fixture pairs made from one source) gives equal models by `model.Diff`, apart from the fields 3.1 can't store; `passbank info` goldens for 3.x.

---

## M4: Write KDBX 4

### 4.1 The KDBX 4 XML writer
**Goal:** from `model.Database` to KDBX 4.0 or 4.1 XML.
**Read:** SPEC §5, §6; KeePass `KdbxFile.Write.cs` and its rule for the lowest version (`GetMinKdbxVersion`).
**Tasks:**
1. Write every model field, with protected values XORed by the inner stream in document order, binary references into the inner header's list, and every `RawXML` back in its place.
2. `MinVersion(db)`: 4.1 when the database holds a 4.1-only field (SPEC §5), else 4.0. When asked to write 4.0, a 4.1-only field is not written (M5's loss check decides whether that may happen; this step only provides the function and a `Lost` report of what was skipped).

**Tests:** the XML of each 4.x fixture model, written and parsed back with 2.6, gives an equal model.

### 4.2 The KDBX 4 container writer
**Goal:** encrypted KDBX 4 bytes from the XML and the encryption settings.
**Read:** SPEC §5, §14.
**Tasks:**
1. `EncryptionSettings` (cipher, KDF and parameters, compression), and `DefaultSettings(version)` with A5's values.
2. New random master seed, IV, KDF seed and inner stream key from `crypto/rand` on every write.
3. The header with its SHA-256 and HMAC, the inner header, compression, encryption and the HMAC block stream (1 MiB blocks, as KeePass).

**Tests:** written files open with `Open` (2.7) for every cipher and KDF; two writes of one database differ in bytes but give equal models.

### 4.3 Safe output
**Goal:** `internal/core/output`, as SPEC §5's "Safe writing" and "Verification after writing" describe.
**Read:** SPEC §5, §15.
**Tasks:**
1. `output.Write(ctx, path, db, version, settings, creds, opts)`:
   - writes a temporary file in the target folder (mode `0600`), syncs it, renames it into place and syncs the folder
   - `OutputExists` unless `opts.Overwrite`
   - `OutputIsInput` when `os.SameFile` matches either input (also with `Overwrite`)
2. Read-back verification: `Open` the written file with the same credentials and compare with `model.Diff`. On a mismatch, remove the file and return `WriteVerifyFailed` with the differences' paths (never values) in the metadata.
3. A failure at any point leaves no temporary file behind, and leaves an existing file at the path unchanged.

**Tests:** every error path (fault injection through a small filesystem interface); an existing file is untouched after a failed write; mode `0600`; route tests.

### 4.4 Round trip and interoperability
**Goal:** proof that what Passbank writes keeps everything and opens in KeePassXC.
**Read:** SPEC §17.
**Tasks:**
1. Round trip: every fixture (all versions) is read, written as KDBX 4 (4.0 or 4.1 by `MinVersion`) and read back, with `model.Diff` empty.
2. Interop tests (build tag `passbank_keepassxc`, skipped without `keepassxc-cli`):
   - `keepassxc-cli ls -R -f` and `keepassxc-cli show -s` on written files match the model
   - KeePassXC reads every written cipher and KDF
3. A seeded random database generator (`internal/core/model/modeltest`) for property tests: random trees, fields, attachments and histories, bounded in size, written and read back.

**Interop tests:** `go test -tags passbank_keepassxc ./internal/core/...`.
**Done when:** the default "Done when", and the interop tests pass.

---

## M5: Write KDBX 2.x and 3.x

### 5.1 The loss check and encryption limits
**Goal:** `OutputVersionLosesData` and `EncryptionNotInVersion`, as SPEC §5 describes.
**Read:** SPEC §5, §4.5, §15.
**Tasks:**
1. `kdbx.Losses(db, version) []Loss`: every item and field the version can't store (each `Loss` has an item path, a UUID and what is lost, never a value). It covers 4.0, 3.1, 3.0 and 2.x.
2. `output.Write` refuses with `OutputVersionLosesData` (with the count and the first losses in the metadata) unless `opts.AllowLoss`.
3. `kdbx.Supports(version, settings) error` gives `EncryptionNotInVersion`, and `DefaultSettings` covers 3.x and 2.x (A5).

**Tests:** a database holding each kind of 4.x-only and 4.1-only data, checked against each version; route tests.

### 5.2 The KDBX 3 writer
**Goal:** KDBX 3.1, 3.0 and 2.x files, exactly as §4.5 says.
**Read:** SPEC §4.5, §5.
**Tasks:**
1. The XML differences (times, `Meta/Binaries`, `HeaderHash`), and the container (header, AES-KDF, stream start bytes, hashed block stream, Salsa20 inner stream) for each version.
2. `output.Write` accepts every version.

**Tests:** written files of every old version open with `Open`; the 3.1 output opens in KeePassXC (interop tag).
**Interop tests:** `go test -tags passbank_keepassxc ./internal/core/...`.

### 5.3 Round trip in every version
**Goal:** the full round-trip matrix of SPEC §17.
**Tasks:**
1. Every fixture, written in every output version, is read back:
   - when the version can't hold its data, the write is refused without `AllowLoss`
   - with `AllowLoss`, `model.Diff` shows exactly the reported losses and nothing else
2. The random databases of 4.4 go through every version the same way.

**Done when:** the default "Done when", and the matrix is green.

---

## M6: Diff and plan

### 6.1 Matching by UUID
**Goal:** `internal/core/diff` pairs items by UUID, and pairs the recycle bins.
**Read:** SPEC §7, §3.
**Tasks:**
1. `diff.Match(a, b *model.Database) Matching`: entry pairs, group pairs, and the unpaired items of each side, wherever they are in the tree.
2. The recycle-bin rule: the two recycle-bin groups are a pair even with different UUIDs.

**Tests:** tables of small databases built in the test (with `modeltest` helpers), covering moved items, same UUID in different groups, and the recycle bins.

### 6.2 Comparison and changes
**Goal:** the change list of SPEC §8.
**Read:** SPEC §8.
**Tasks:**
1. Entry equality exactly as §8 lists it (ignoring last access, usage count and history).
2. Group equality for `group-differs`, and the `settings` change from the `Meta` fields §8 names.
3. Deletions: `deleted-in-a` and `deleted-in-b` from the deleted-objects lists.
4. `diff.Changes(a, b) []Change`. Each change has its kind, the items on each side, the names of the differing fields (for `differs`), both modification times, and its group path and title for display. The order is stable, as §8 defines.

**Tests:** one table per kind; field-level differences named correctly; stable order across runs; equal entries are not listed.

### 6.3 Defaults and rules
**Goal:** `internal/core/plan`: choices, defaults and rules, as SPEC §8 and §9 describe.
**Read:** SPEC §8, §9.
**Tasks:**
1. `plan.Plan`: the input hashes, the changes, and one choice each. `Allowed(kind)` gives the choices of §8's table.
2. `plan.Default(changes)` and `plan.ApplyRule(plan, rule, filter)` for `newer`, `a`, `b` and `union`. The filter selects changes, for the GUI's bulk actions.

**Tests:** every kind under every rule; ties go to A; deletion defaults by time.

### 6.4 The plan file
**Goal:** writing and reading plan files exactly as SPEC §11 shows.
**Read:** SPEC §11, §15.
**Tasks:**
1. `plan.Write(w, p, opts)` with the header, one line per change, the comments, and `NoTitles`.
2. `plan.Read(r, a, b, changes)`: every error of §11 (`PlanStale`, `PlanIncomplete`, `PlanUnknownItem`, `PlanInvalidChoice`, `PlanSyntax` with its line number).
3. Fuzz test for `Read`.

**Tests:** write then read gives the same plan; hand-written files with every error; titles with spaces, `#` and non-ASCII characters; CRLF line endings accepted.

### 6.5 `passbank diff`
**Goal:** the `diff` command of SPEC §12.
**Read:** SPEC §12.
**Tasks:**
1. `passbank diff A B [--prefer …] [-o PLAN] [--no-titles]`, with the credential options for `a` and `b`.
2. The plan goes to `-o` (refusing to overwrite unless `--force`) or to stdout; a summary of counts by kind goes to stderr.

**Tests:** CLI goldens on fixture pairs built for merging (`internal/cli/testdata/merge/`: two related databases with one change of every kind, built once by a generator in `internal/core/model/modeltest` and committed).
**Proof:** method C, the golden plan of the related pair.

---

## M7: Merge

### 7.1 Building the result tree
**Goal:** `internal/core/merge.Apply(a, b, plan) (*model.Database, error)` for the tree, as SPEC §10.1 describes.
**Read:** SPEC §8 (choice meanings), §10.1.
**Tasks:**
1. Start from a copy of A's tree, and apply every choice. Taking B moves the entry to B's group. `both` gives B's copy a new UUID. `drop` removes the item. Groups that only one input has are created with their ancestors when a kept item needs them.
2. Child order as §10.1 says.
3. `Apply` is pure: it never changes `a` or `b` (tests check this with `model.Diff` against copies).

**Tests:** one table per choice and kind; nested new groups; moved entries; inputs unchanged.

### 7.2 History, deletions, meta and attachments
**Goal:** SPEC §10.2–§10.5.
**Read:** SPEC §10.2–§10.5.
**Tasks:**
1. History union, deduplication and order, and adding the version not chosen (A7's answer).
2. The deleted-objects union minus the result's UUIDs.
3. Meta from the `settings` choice; the custom icons and custom data unions; the recycle bin.
4. Rebuild the attachment pool from the references, deduplicated by hash.

**Tests:** tables for each rule; a merged result has no dangling attachment or icon reference.

### 7.3 Plan validation
**Goal:** a plan is complete and consistent before it is applied.
**Read:** SPEC §8, §11, §15.
**Tasks:**
1. `plan.Validate(p)`: every change has an allowed choice, and `PlanGroupNotEmpty` when a dropped group still holds a kept item (naming the group and one kept item).
2. `Apply` calls `Validate` first.

**Tests:** each failure, with route tests.

### 7.4 `passbank merge`
**Goal:** the `merge` command of SPEC §12.
**Read:** SPEC §5, §12, §15.
**Tasks:**
1. `passbank merge A B -o OUT` with `--plan` or `--prefer` (without either: for now a usage error, `// TODO(step 7.5)`), plus `--format`, `--allow-loss`, `--settings-from`, `--credentials`, `--encryption` and `--force`.
2. Credentials for the output:
   - `a` and `b` reuse the composite key that unlocked that input
   - `new` asks for a password twice (`PasswordsDoNotMatch`), or takes `--keyfile-out`, `--password-file-out` or `--no-password-out`
3. Encryption: `a`, `b` or `default`, checked against the version (`EncryptionNotInVersion`). `--format 4` means 4.0 or 4.1 by `MinVersion`.
4. The summary on stdout: counts of each kind and choice, the output path, and the version written.

**Tests:** CLI goldens for every option on the merge fixture pairs; the written output opens with `Open`, and its content matches the expected merge (checked with `model.Diff` against a model built in the test).

### 7.5 Interactive choices
**Goal:** `passbank merge` without `--plan` and `--prefer`, as SPEC §12.1 describes.
**Read:** SPEC §8, §9, §12, §12.1, §14, §15.
**Tasks:**
1. `internal/cli/review`: the interactive loop over `[]plan.Change` and a `plan.Plan`, reading keys from the `term` interface (a line per answer is enough; single-key reading is not needed). It shows each change as §12.1 lists, with the non-secret standard fields side by side and the other differing fields by name only.
2. The keys of §12.1: choices per kind, Enter for the default, `v` (protected values printed for that change only, never stored or logged), `p` (previous), `r` (a rule for the rest), `?`, `q`. An invalid key asks again.
3. Dropping a group that still holds kept items asks the follow-up question of §12.1.
4. After the last change: the summary, then the "Write OUT? [y/N]" confirmation. Replace the 7.4 usage error with this path. Without a terminal, fail with `ChoicesNeedTerminal`.

**Tests:** in-process CLI tests with a fake terminal that plays scripted answers: accept all defaults (equals `--prefer newer`), every key, going back, `r`, `q` (nothing written, exit 1), the group question both ways, `v` printing protected values once and never logging them, no terminal; CLI goldens of the prompts on the merge fixture pairs.
**Proof:** method C, the golden transcript of an interactive merge of the related pair.

### 7.6 End-to-end merges
**Goal:** the whole CLI path, checked against KeePassXC.
**Tasks:**
1. End-to-end tests, each as `diff`, then an edited plan, then `merge`, then `Open`: one per rule, plus a hand-edited plan. One more test drives the interactive merge with scripted answers and compares its output with the same choices given as a plan.
2. Interop: the merged files open in KeePassXC, with the expected entries (`keepassxc-cli ls -R`, `show`).
3. Large inputs: two generated databases of 10,000 entries each. `diff` and `merge`, without the KDF, take under 2 s; record the time in `IMPLEMENTED.md`.

**Interop tests:** `go test -tags passbank_keepassxc ./internal/cli/...`.

---

## M8: GUI

### 8.1 The kit
**Goal:** Passbank's own widgets on Glutter's `widgets` layer, in `internal/gui/kit`.
**Read:** SPEC §13; CLAUDE.md "Glutter"; Glutter `docs/GUIDE.md`, `widgets/editable_text.go` (`ObscureText`), `widgets/radio_group.go`, `widgets/table.go`.
**Tasks:**
1. The theme: colors (light and dark, following the desktop's preference if Glutter exposes it), spacing and text styles, with the Go fonts.
2. Widgets, following Glutter's constructor conventions:
   - button (primary, secondary, disabled)
   - text field
   - password field with a reveal toggle (`ObscureText`)
   - file field (a path with a "Choose…" button)
   - segmented choice (for Take A / Take B / …)
   - list row with selection
   - side-by-side field table
   - progress indicator, error banner, and dialog

**Tests:** widget tests for each (states, keyboard focus, the reveal toggle); golden images of each in both themes.
**Proof:** method A, `go test ./internal/gui/kit -run TestProofKit -proof`, writing `8.1-kit.png`: every widget in both themes.

### 8.2 The Open screen
**Goal:** SPEC §13 screen 1.
**Read:** SPEC §13, §4.2; Glutter `desktop/file_chooser.go` (`OpenFiles`, `FileFilter`), `widgets/drag_drop.go`, `widgets/drag_target.go`.
**Tasks:**
1. Two panels, A and B: file chooser (portal, `*.kdbx` filter), password, key file, **Unlock**.
   - Unlocking runs `kdbx.Open` off the UI thread with a cancellable `ctx`, and hands the result back through `SetState`
   - after unlocking, the panel shows the `Info` summary
   - errors show in the panel, as the messages of SPEC §15
2. Dropping a file on a panel sets its path, if Glutter delivers file drops from the desktop. If it doesn't, that is a Glutter gap (rule 7).
3. **Swap A and B**, and **Compare** (enabled when both are unlocked).
4. `internal/gui/app` holds the session state: both databases, credentials, the plan, and the output settings. Leaving the Open screen's flow wipes the secrets (SPEC §13).

**Tests:** widget tests with fixture databases and a fake file chooser: unlock, wrong password, swap, compare enabled only when both are unlocked; secrets wiped when going back.
**Proof:** method A, `8.2-open.png`: both panels unlocked, showing their summaries. Method D (manual check): the portal file dialog opened from the panel, `docs/implemented-proof-images/8.2-portal.png`, with the command `scripts/isolated.sh -- go run ./cmd/passbank-gui`.

### 8.3 The Review screen: the list
**Goal:** the left side of SPEC §13 screen 2.
**Read:** SPEC §8, §9, §13.
**Tasks:**
1. Changes grouped by kind with counts, a filter (all, by kind, undecided only), and a search over group path and title.
2. Each row shows the kind, group path, title and choice. The keys `A`, `B`, `K`, `D` and `N` set the choice of the selected rows; arrow keys and Shift/Ctrl selection work.
3. Bulk actions from the rules of §9 on the filtered rows.
4. The list stays fast with 10,000 changes (lazy building).

**Tests:** widget tests for filtering, search, keys, multi-selection, and the bulk rules; a test pumping 10,000 changes within a time bound recorded in `IMPLEMENTED.md`.
**Proof:** method A, `8.3-review-list.png`: the merge fixture pair's changes, with mixed choices.

### 8.4 The Review screen: details and plans
**Goal:** the right side of SPEC §13 screen 2, and plan load and save.
**Read:** SPEC §11, §13.
**Tasks:**
1. The side-by-side table with differing fields highlighted; protected values masked, each with a reveal toggle that resets when the selection changes; attachments marked same or different; both modification times.
2. Choice buttons enabled per kind.
3. **Load plan** and **Save plan** through the portal dialogs and `plan.Read` / `plan.Write`, with the plan errors shown as SPEC §15 messages.
4. `PlanGroupNotEmpty` offered as a fix: "Drop everything in this group".

**Tests:** widget tests for the reveal reset, plan load errors, and the group fix; golden images of the details for each kind (fixture data only).
**Proof:** method A, `8.4-review-details.png`: a `differs` change with a password difference, masked.

### 8.5 The Write screen
**Goal:** SPEC §13 screen 3.
**Read:** SPEC §5, §13, §15.
**Tasks:**
1. The output path (`desktop.SaveFile`, whose overwrite confirmation counts as the user's consent), the version menu (default KDBX 4), the settings source, the credentials (A's, B's, new with a confirmation field, key file), and the encryption source.
2. The live checks: `EncryptionNotInVersion`, and the loss list for the chosen version, with a confirmation before writing.
3. **Write** runs `merge.Apply` and `output.Write` off the UI thread with progress and cancel, then shows the verification result and the summary.
4. **Back** keeps every choice.

**Tests:** widget tests with a fake save dialog: write success, the loss confirmation, each error; the written file opens with `Open`.
**Proof:** method A, `8.5-write.png`: the screen with the version menu and the summary.

### 8.6 The full flow
**Goal:** the three screens working together in a real window.
**Tasks:**
1. A display test (build tag `passbank_display`) that starts `passbank-gui` with debug-only flags. `--dev-a`, `--dev-b` and `--dev-password-file` preload fixture paths and the fixture password file; they exist only in debug builds (not with `glutter_release`), and never accept a password in the argument itself. The test drives Open, Review and Write through `gluttertest`-style input, and checks the written file.
2. Window title, minimum size, and keyboard focus order across the screens.

**Display tests:** `go test -race -tags passbank_display ./internal/gui/app`.
**Proof:** method B, `scripts/proof.sh docs/implemented-proof-images/8.6-flow.png --after 2s -- --dev-a internal/cli/testdata/merge/a.kdbx --dev-b internal/cli/testdata/merge/b.kdbx --dev-password-file internal/cli/testdata/merge/password.txt`: the Review screen with the fixture changes.

### 8.7 Accessibility and polish
**Goal:** the GUI works by keyboard alone and with a screen reader.
**Read:** Glutter `semantics/`, SPEC §13.
**Tasks:**
1. Semantics labels on every control (Glutter's AT-SPI2 bridge), so every action is reachable by keyboard and named for screen readers. Password values are exposed as obscured.
2. Check every screen at 100 % and 200 % scale, and with a long group path and title (ellipsis, no overflow).

**Tests:** semantics tests on every screen; goldens at 200 % scale.

---

## M9: Release

### 9.1 Release builds and the README
**Goal:** what a user needs to install and trust Passbank.
**Read:** SPEC, all of it.
**Tasks:**
1. `README.md`:
   - what Passbank does and doesn't do
   - install (`go install` for both binaries)
   - CLI examples (diff, edit, merge)
   - the GUI flow
   - the security statement of SPEC §14 (including that secrets may reach swap)
   - the KeePassXC compatibility
2. `--version` prints the module version and the commit (`debug.ReadBuildInfo`); release builds as CLAUDE.md "Commands" lists them.
3. A coverage table in `docs/IMPLEMENTED.md`: every SPEC section, the steps that built it, the main tests, and its status (done, partial with what is missing, or not done), checked against the code.

**Done when:** the default "Done when", and the coverage table shows every section done or explains why not.

### 9.2 Real databases
**Goal:** the user's check on real data.
**Tasks:**
1. A **manual check**: the user copies two of their own databases to a scratch folder, and runs `passbank diff`, `passbank merge` and the GUI on the copies. They then open the result in KeePassXC and report anything wrong. The check gives the exact commands, all on copies, never on the originals.
2. Fix what the user reports, each fix with a regression test made from fixture data (never the user's data).

**Done when:** the user confirmed the check, and every reported problem is fixed or recorded as a known issue in the README.
