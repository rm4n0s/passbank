# Implemented steps

This is the log of finished steps from `docs/IMPLEMENTATION.md`, newest at the bottom. Each entry is added on the step's branch, as part of the step's commit (`docs/IMPLEMENTATION.md`, rules 11–13). The next step to do is the first step in `docs/IMPLEMENTATION.md` that has no entry here.

## Entry format

Copy this block, fill it in, and keep every heading. Write "none" where something doesn't apply.

```markdown
## <id> <step title>

- **Date:** YYYY-MM-DD
- **Branch:** feat/step-<id>-<slug>
- **Summary:** 2–4 sentences on what now exists and works.
- **Main files:** the packages or files created or changed.
- **Tests:** what was added, including fixtures and fuzz seeds.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · CGO_ENABLED=0 build ✅ · CGO_ENABLED=0 test ✅ · error-tags ✅ · display tests ✅/none · interop tests ✅/none
- **Proof:**
  ![<id> <name>](implemented-proof-images/<id>-<name>.png)
  One sentence describing what the image shows, as you verified it by viewing it (or the golden output, for method C).
- **Manual checks:** the user's answers to the step's manual checks, "pending, see docs/MANUAL-CHECKS.md", or "none".
- **Glutter issues:** entries added to docs/GLUTTER-ISSUES.md, or "none".
- **Deviations and follow-ups:** differences from SPEC or KeePass, and TODOs left for later steps.
```

---

## 1.1 Confirm the assumptions

- **Date:** 2026-11-04
- **Branch:** feat/step-1.1-confirm-assumptions
- **Summary:** The user answered the assumptions of SPEC §19. A3–A9, A11 and A12 were confirmed as proposed. Three changed: A1 (the CLI is interactive), A2 (entries pair by UUID only) and A10 (KDBX 2.x and 3.x are built by Passbank from public sources, or given up if that proves impossible). SPEC §19 is now a decisions log, the "probable match" kind and plan-file form are gone, and the plan has a new step 7.5 (interactive choices); M6 is 6.1–6.5 and M7 is 7.1–7.6.
- **Main files:** docs/SPEC.md (§4.1, §7, §8, §9, §11, §12, §12.1, §15, §17, §18, §19), docs/IMPLEMENTATION.md (rule 6, steps 2.4, 3.1, 6.x, 7.x), docs/IMPLEMENTED.md.
- **Tests:** none (no code yet).
- **Checks:** skipped: there is no Go module yet, so gofmt, vet, lint, tests and the cgo-free checks have nothing to run.
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:** the user said to base the 2.x/3.x code on C++ code if any exists. KeePassXC's C++ is GPL and Passbank is MIT, so step 3.1 reads it (and KeePass's C#) for behavior only and writes the code independently, using MIT-licensed implementations (gokeepasslib, keepass-rs, kdbxweb) as the copyable references.

## 1.2 Module, package layout and the architecture checks

- **Date:** 2026-11-04
- **Branch:** feat/step-1.2-module-layout
- **Summary:** The Go module `github.com/rm4n0s/passbank` (Go 1.27.3) exists with every package of SPEC §16 as a documented empty package, and two empty `main` packages. `internal/archtest` parses all Go files and enforces ten rules: Glutter only in `internal/gui` and `cmd/passbank-gui`; core, CLI and GUI layering; `golang.org/x/term` only in `internal/cli`; no `import "C"`; no standard-library `errors`; no `fmt.Errorf`; no `net`/`net/*` (except `net/url`) or `crypto/tls`. A separate test checks that `go list -deps ./cmd/passbank` lists no Glutter package.
- **Main files:** go.mod, .golangci.yml, NOTICE, third_party/README.md, cmd/*/main.go, internal/*/doc.go, internal/archtest/archtest.go, internal/archtest/archtest_test.go, internal/archtest/testdata/.
- **Tests:** each rule has a fixture tree that breaks it (bad-glutter, bad-layers, bad-imports, bad-calls) and a `good` tree that passes, including a `testdata` folder that must be skipped; `TestRepository` runs the rules on Passbank itself (checked by planting a bad import: it failed as it should); route tests for the five error Tags (`GoModReadFailed`, `ModuleLineMissing`, `RootWalkFailed`, `SourceReadFailed`, `SourceParseFailed`). The syntax-error case is built in a temporary folder because a broken file cannot be gofmt-clean.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ (0 issues, v2.15.0) · go test -race ✅ · CGO_ENABLED=0 build ✅ · CGO_ENABLED=0 test ✅ · error-tags: script arrives in step 1.3 · display tests none · interop tests none
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:** `github.com/rm4n0s/errors` was added here (not in 1.3) because archtest needs it; step 1.3 is adjusted. `internal/third_party/argon2` and the fixture-only packages `modeltest` and `cli/review` are not created yet; their steps (2.2, 4.4, 7.5) do it. `NOTICE` lists the planned components with the step that adds each.

## 1.3 Test isolation, secrets and the error-tag script

- **Date:** 2026-11-04
- **Branch:** feat/step-1.3-isolation-secrets-tags
- **Summary:** `testenv.Isolate(t)` points HOME and every XDG variable at fresh folders under `t.TempDir()` (the runtime folder is mode 0700), and `testenv.Fixture` returns a path under the package's `testdata/` or fails the test. `secret.Bytes` holds secrets: Wipe zeroes every copy, Equal is constant-time, and every fmt verb, slog and text encoder print `[secret]`. `scripts/error-tags.sh` lists every error Tag and, with `--untested`, the Tags no test asserts.
- **Main files:** internal/testenv/, internal/core/secret/, scripts/error-tags.sh, internal/archtest/errortags.go, internal/archtest/cmd/errortags/, internal/archtest/archtest.go (shared `walkGo`).
- **Tests:** testenv with a fake `TB` (variables set, all inside the temp folder, restored after a real subtest, failure paths); secret (wipe shared by copies, clone independent, Equal table, no leak with ten fmt verbs on values, pointers, structs with exported and unexported fields, slices and maps, nor in slog text/JSON nor JSON); error tags on fixture trees (literal, table, helper, `.Tag` comparison, `errors.Is` sentinel, comment-only and test-fake Tags not counted, non-literal Tag, exceptions file), the command's output and exit status, and the shell script on fixtures. `TestRepositoryErrorTags` runs the same check on Passbank itself on every `go test`. Route test for the new Tag `ErrorTagsExceptionsReadFailed`.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · CGO_ENABLED=0 build ✅ · CGO_ENABLED=0 test ✅ · error-tags ✅ (prints nothing) · display tests none · interop tests none
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:** the first version of `secret.Bytes` held its bytes behind a pointer to a struct, and the leak test found that `fmt` prints such a field by its contents with `%s` or `%q` when the field is unexported (`%!s(*secret.buffer=&{[104 117 ...]})`). It now holds them behind a func, which fmt prints as an address; the doc comment says why, and a `Bytes` is no longer comparable with `==` (use `Equal`). The error-tag scanner is a trimmed port of Emetos's (the user's MIT project, commit 4d0c4b5); its extra rule file is not ported because no step needs it. The Go program is in `internal/archtest/cmd/errortags` and the script wraps it, as in Emetos. `docs/ERROR-TAGS-UNTESTED.md` does not exist yet (an empty list).

## 1.4 Scripts, first window and the CLI skeleton

- **Date:** 2026-11-04
- **Branch:** feat/step-1.4-scripts-first-window
- **Summary:** `go run ./cmd/passbank-gui` opens a 1100×720 Glutter window (Wayland/X11, `WidgetsApp` without Material) that shows "Passbank" centered on the app's background, with the Go fonts bundled. `passbank --version`, `--help` and usage errors work through `cli.Main`, which takes the terminal as an interface (`cli.TTY` is the real one, on `/dev/tty`). The scripts `heavy.sh`, `gpu-env.sh`, `isolated.sh`, `proof.sh`, `headless-wayland.sh` and `screenshot-headless.sh` exist, and `proof.sh` makes a screenshot in headless sway on Mesa's software renderer, never the NVIDIA card.
- **Main files:** go.mod (Glutter v1.9.9, x/image, x/term), cmd/passbank, cmd/passbank-gui, internal/cli, internal/gui/app, internal/gui/kit/fonts, internal/gui/testutil, internal/version, scripts/, third_party/x-image, NOTICE.
- **Tests:** the home shows the name (widget test); `app.Main` and `cli.Main` for every flag and mistake, and a failing stdout (exit 1); both font families load (monospace gives "iiii" and "WWWW" one width, Go does not); `cli.TTY` route tests for `TerminalOpenFailed`, `TerminalWriteFailed` and `PasswordReadFailed`; `app.Run` without a display (`AppRunFailed`); `version.describe`; script tests: `isolated.sh` moves HOME and every XDG variable, passes the status through, rejects bad usage, and leaves no folder behind; `heavy.sh` passes assignments and status through and makes a third command wait while two others hold the slots; `gpu-env.sh` leaves no NVIDIA driver visible.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · CGO_ENABLED=0 build ✅ · CGO_ENABLED=0 test ✅ · error-tags ✅ · display tests none · interop tests none
- **Proof:**
  ![1.4 first window](implemented-proof-images/1.4-first-window.png)
  The 1100×720 window shows "Passbank" in bold, centered on the near-white background (checked by viewing the image; made with `scripts/proof.sh docs/implemented-proof-images/1.4-first-window.png`).
- **Manual checks:** the success path of `cli.TTY` (a real password prompt without echo) needs a terminal; it is added to step 2.8 as a manual check, where `passbank info` first uses it.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - `WidgetsApp`'s `color` is only the app's primary color (as in Flutter), not a background: the first proof showed the engine's purple, so the page paints its own `ColoredBox`.
  - `heavy.sh` uses the same lock files as Emetos's (`emetos-heavy.*`) on purpose, so Passbank and Emetos sessions on this machine share the two slots. The proof then waited behind the Emetos session's runs; a `--alone` request would block that session's new commands while it waits, so the proof took a normal slot (CLAUDE.md names real-input display tests and timing measurements for `--alone`, not proofs).
  - The real terminal moved into `internal/cli` (archtest allows `x/term` only there), so `cmd/passbank` only calls `cli.Main`; `internal/version` is new (both programs print it).
  - `scripts/headless-wayland.sh` and `screenshot-headless.sh` are Emetos's adaptations of Glutter's (BSD-3-Clause, noted in the header); `proof.sh` lost the options this project doesn't have.
  - The display-test helper that sets the GPU environment for programs the Go tests start is not written: step 8.6 adds it (`gpu-env.sh` says so).

## 2.1 The data model

- **Date:** 2026-11-05
- **Branch:** feat/step-2.1-data-model
- **Summary:** `internal/core/model` holds the format-independent database: `Database`, `Meta`, `Group`, `Entry`, `Field`, attachments with a content pool keyed by SHA-256 and the protected flag, auto-type, history, custom icons and data, deleted objects, the KDBX 4 public custom data, and `Raw` (unknown XML as a tree whose text is a secret, so protected unknown values can be decrypted and encrypted in stream order). `model.Diff` compares two databases deeply and reports paths, never values; `Clone` and `Wipe` copy and zero everything secret.
- **Main files:** internal/core/model/{model,uuid,tree,clone,diff}.go, scripts/checks.sh.
- **Tests:** UUID parse and format (valid, upper case, six malformed forms, route test for `UUIDInvalid`); Diff on equal databases, a clone and nil; 40 table cases that each change one property and expect its path (and in the reverse direction), checking that no line shows any of the five secrets of the sample; Clone is deep (nine kinds of change to the clone leave the original equal to a fresh sample); Wipe reaches field values, history, raw text and attachments; tree walks, stops, `FindGroup`, `FindEntry`, `GroupPath`; printing a `Field` with five verbs shows no value.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · CGO_ENABLED=0 build ✅ · CGO_ENABLED=0 test ✅ · error-tags ✅ · display tests none · interop tests none
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - Every field value is a `secret.Bytes`, protected or not, so that no title or user name is printed or logged by accident; `Field.Text()` reveals one on purpose.
  - `Raw` keeps unknown elements as a node tree (name, attributes, text, protected flag, children), not as raw bytes: the inner stream protects values in document order, so step 2.6 must decrypt the protected text of unknown elements to keep the stream in step. Comments and processing instructions are not kept, and an unknown element is written after the known ones of its parent, not at its original position.
  - `Diff` compares children by UUID and then their order; history entries by position. Times are compared with `Equal`.
  - `scripts/checks.sh` is new (not in the plan): it runs the standard checks of rule 9 through `heavy.sh`.

## 2.2 Ciphers and key derivation

- **Date:** 2026-11-05
- **Branch:** feat/step-2.2-ciphers-kdf
- **Summary:** `internal/core/kdbx/crypto` has the three outer ciphers (AES-256-CBC, Twofish-CBC with PKCS#7, ChaCha20), the three KDFs (AES-KDF, Argon2d, Argon2id) with bounded parameters and cancellation, and the two inner streams (Salsa20 and ChaCha20, as one continuous keystream) plus "none". `internal/third_party/argon2` is the copy of `x/crypto/argon2` v0.58.0 that exports Argon2d (SPEC A12).
- **Main files:** internal/core/kdbx/crypto/{cipher,kdf,innerstream}.go, internal/third_party/argon2/ (upstream files, `DKey` and `dkey_test.go` added), third_party/x-crypto/, docs/ERROR-TAGS-UNTESTED.md, .golangci.yml (copied code is not linted).
- **Tests:** the six UUIDs against the strings of SPEC §4.1; known answers from independent sources: AES-256-CBC and the ChaCha20 inner stream from OpenSSL, AES-KDF (1, 2, 10 rounds) from OpenSSL and Python, Twofish from the Twofish paper (all-zero 256-bit key), ChaCha20 from RFC 8439 A.1; Salsa20 inner stream equal to `x/crypto/salsa20` over seven different splits of 1000 bytes; round trips for every length 0–70; every error route (`CipherUnsupported`, `CipherKeyInvalid`, `CiphertextInvalid`, `PaddingInvalid` incl. four forged paddings, `KdfParametersRejected` for 13 cases, `KdfUnsupported`, `KdfKeyInvalid`, `KdfCancelled` before, in AES-KDF and in Argon2, `InnerStreamUnsupported`, `InnerStreamKeyInvalid`); the upstream argon2 tests, which include the RFC 9106 vectors of Argon2d, Argon2i and Argon2id, run in the copy, and `DKey` is checked against the Argon2d vectors of the reference CLI.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - Argon2 version 1.0 (0x10) is rejected (`KdfParametersRejected`): the library has only 1.3. At most 255 lanes (the library takes a byte), not 256 as the plan said. Argon2 salt 8 bytes to 64 KiB, memory up to 4 GiB, AES-KDF rounds up to 2³²−1.
  - `CipherInitFailed` is listed in docs/ERROR-TAGS-UNTESTED.md: the sizes are checked before every constructor, so it cannot be reached.
  - Argon2 cannot be interrupted: on cancellation the call returns at once and the computation finishes in the background (at most a few seconds, 4 GiB at the very most).
  - The AES-KDF and Argon2 results are checked against independent code for the algorithms, but the full path (a real KDBX file) is checked only when the fixtures of step 2.4 exist.

## 2.3 Key files and the composite key

- **Date:** 2026-11-05
- **Branch:** feat/step-2.3-key-files
- **Summary:** `crypto.CompositeKey` makes the 32-byte composite key from a password, a key file, or both, as KeePass does (SHA-256 of the 32-byte components in that order). Key files are read in KeePass's order: XML 1.x (base64) and XML 2.x (64 hex digits, whitespace allowed, the 4-byte `Hash` checked), then a 32-byte file as it is, a 64-hex-digit file as the key it spells, and any other file hashed. `crypto.ReadKeyFile` reads a key file from disk, at most 64 MiB.
- **Main files:** internal/core/kdbx/crypto/{credentials,keyfile}.go.
- **Tests:** expected keys computed with Python's hashlib (password only, empty password, raw key file, hex key file in both cases, hashed key file, password plus each kind, XML 1.0 and 2.0 in several spellings, another XML root read as a plain file); eleven damaged XML key files, each `KeyFileCorrupt` (bad base64, empty key, wrong, missing, short and non-hex hash, short and non-hex key, unknown and missing version, truncated document); `NoCredentials`; `ReadKeyFile` for a missing file, a folder, a cancelled context, a file of exactly 64 MiB (a sparse file) and one byte more; Wipe.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - `CompositeKey` takes no `ctx` (the plan said it would): it does no I/O. `Credentials` has explicit `HasPassword` and `HasKeyFile` flags, so an empty password can be a component (KeePass) or absent (KeePassXC's reading of an empty field); the callers choose.
  - As in KeePass, a password and a hashed key file with the same text give the same key; the test says so.
  - An XML key file whose root is `KeyFile` and which is damaged inside is an error, not a hashed file, because no other program would read it as one; a document with another root, or a file that is not XML, follows the binary rules. XML in other encodings than UTF-8 is read as a binary file.
  - The behavior of KeePass for an XML 1.0 key whose base64 does not decode to 32 bytes is not documented; Passbank uses the decoded bytes as they are, like KeePass's reader does, and rejects only an empty or invalid one. A real KeePass-made file will confirm this in step 2.5's tests.

## 2.4 Fixtures

- **Date:** 2026-11-05
- **Branch:** feat/step-2.4-fixtures
- **Summary:** 15 KDBX files and 3 key files, none made by Passbank. KeePassXC 2.7.12 (the Flatpak's `keepassxc-cli`) made the four KDBX 3.1 files and the key files; pykeepass 4.2.0 made the KDBX 4 files: AES-256, ChaCha20 and Twofish; AES-KDF, Argon2d and Argon2id; with and without compression; password, key file, or both; a public-custom-data header field; a 4.1 file; a file with unknown XML. `make-fixtures.sh` opens every file with KeePassXC, which reads all of them.
- **Main files:** internal/core/kdbx/testdata/ (make-fixtures.sh, gen/rich.py, gen/make_kdbx4.py, FIXTURES.md, rich.xml, the fixtures), internal/core/kdbx/fixtures_test.go, docs/MANUAL-CHECKS.md.
- **Tests:** `TestFixturesAreListed`: the folder and FIXTURES.md list the same files, every database starts with the KDBX signature, and its version (3.1, 4.0, 4.1) is the one its name says.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** two optional checks in docs/MANUAL-CHECKS.md (a KDBX 4 file from KeePassXC's own writer, a real 4.1 file); no step waits for them.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - `keepassxc-cli` is not installed here; the Flatpak KeePassXC was used. It writes only KDBX 3.1 from its command line (`db-create` and `import` both), so the plan's "make every fixture keepassxc-cli can make" gave 3.1 files, and the KDBX 4 files had to come from pykeepass, installed in a temporary virtualenv from PyPI by the script (a development tool, not a dependency of Passbank). The generator is independent of Passbank, and KeePassXC reading the pykeepass files is checked on every run.
  - KeePassXC's XML import crashes (segfault) on `Protected="True"` values; `rich.py` writes `ProtectInMemory="True"` for it, as a plain KeePass export does.
  - The `Token` element of the unknown-XML fixture has a `Protected="True"` attribute but is not a `Value` element, so it is stored as plain text: step 2.6 must decrypt only the text of `Value` elements.
  - The Flatpak keeps its own configuration folder, which the script cannot move; `keepassxc-cli` used for these commands reads no setting that matters.
  - pykeepass does not know the 4.1 elements; `k41-fields.kdbx` was not written by a program that does. A real 4.1 file is the optional manual check.

## 2.5 The KDBX 4 container

- **Date:** 2026-11-05
- **Branch:** feat/step-2.5-kdbx4-container
- **Summary:** `readContainer4` takes the bytes of a KDBX 4.x file and the composite key and returns the header, the decrypted and decompressed XML, the inner stream (ID and key) and the binaries with their protected flag. Order of the checks: signature and version, outer header (TLV and VariantDictionary, every length bounded before allocation), header SHA-256 (`FileCorrupt`), cipher and IV length, KDF parameters, key derivation, header HMAC (`WrongCredentials`), HMAC block stream, decryption, gzip (bounded at 1 GiB), inner header. Derived keys are wiped when the function returns.
- **Main files:** internal/core/kdbx/{file,variant,header4,container4}.go.
- **Tests:** every KDBX 4 fixture (all ciphers, KDFs, compression on and off, key files, public custom data, unknown XML) reads to the expected XML; wrong password, wrong key file, password missing; about 30 damaged files (a flipped byte or a cut in each layer: signature, version, header fields, header hash, header HMAC, block HMAC, block length, ciphertext, gzip, inner header), each with its route; every file cut at any length through the header and at every 29th length after it; a cancelled context. A test-only KDBX 4 writer (`build_test.go`) makes files with exact parts for the cases no fixture has: unknown header fields, comment field, a wrong IV length, an unknown KDF, Argon2 and AES-KDF parameters that are out of range, missing or of the wrong type, VariantDictionary limits and bad types, bad gzip, a gzip stream over the limit, a block stream over the limit, and every inner-header error.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - The plan said unknown outer-header field IDs are an error "as in KeePass". They are kept in `Header.Unknown` instead, so that a file from a newer KeePass still opens and the writer can put them back.
  - The error Tag `FileCorrupt` is shared by all the damage sites; each site has its own route test (the route ends in the function name that found it).
  - `PaddingInvalid` and `CiphertextInvalid` from the cipher layer reach the caller as they are; SPEC §15 should list them under file damage (not changed yet).

## 2.6 The KDBX 4 XML

- **Date:** 2026-11-05
- **Branch:** feat/step-2.6-kdbx4-xml
- **Summary:** `parseXML4` turns the decrypted XML of a container into a `model.Database`: Meta (with KeePass's defaults for what a file leaves out), custom icons, custom data, the group tree, entries with their string fields, attachments, auto-type, history, times, deleted objects, and the 4.1 elements. Protected `Value` elements are decrypted with the inner stream in document order; attachments are resolved into the content-addressed pool. What the model has no field for is kept as raw XML on the file, `Meta`, group or entry (SPEC §4.4 now lists the rules). Limits: groups nest at most 1,000 deep, elements at most 1,100, one text at most 256 MiB; the reader looks at `ctx` every 1,024 tokens.
- **Main files:** internal/core/kdbx/xmlread.go; model.RawNode lost its `Protected` field.
- **Tests:** every KDBX 4 rich fixture is read and compared with the facts of FIXTURES.md (Meta, groups, all entries and their fields, protected flags, history, attachment, deleted object), plus the 4.1 fields, the public custom data, and the unknown XML of `k4-unknown-xml` (kept in Meta, the root group, Bank and at the top level; the `Token` element with a `Protected` attribute is kept as it is, and the values after it still decrypt, which proves the stream stays in step); hand-made documents for decryption order (history, `ProtectInMemory`, empty values), the attachment pool (sharing, the protected flag, unused contents dropped, copies independent of the container), ISO and base64 times and their limits, tags, a leaf element with attributes, attributes of objects, Meta defaults; about 25 damaged documents each with its route (not XML, cut, wrong root, no root group, two root groups, missing UUIDs, bad UUID, number, bool, tri-state, base64, reference, time, deleted object, 1,001 nested groups, too deep unknown elements); the text limit at, and one over, a small limit; cancellation; an unknown inner stream; no secret in an error.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - `RawNode.Protected` is removed: the fixture shows (and KeePass does the same) that an unknown element with a `Protected` attribute is not encrypted, so the reader never decrypts raw nodes, and the attribute stays in `Attrs`.
  - FIXTURES.md said the custom icon has 26 bytes; it has 24 (corrected).
  - Namespaces are not supported: an element's local name is kept and its prefix is lost. KeePass files use none.
  - Unknown elements inside `Times`, `MemoryProtection`, `AutoType`, icons, custom data items, `String` and deleted objects are dropped (no raw XML on those parts of the model); SPEC §4.4 says so.
  - The order of groups relative to entries inside a group is not kept (the model keeps two lists); KeePass writes entries first.

## 2.7 Opening a database

- **Date:** 2026-11-05
- **Branch:** feat/step-2.7-open
- **Summary:** `kdbx.Open(ctx, path, creds)` and `kdbx.OpenBytes(ctx, data, creds)` read a database file to a `model.Database` and an `Info` (version, cipher, KDF with its parameters, compression, inner stream, counts of groups, entries, history items and attachments). The file is read once, at most 2 GiB. KDBX 2.x and 3.x give `VersionUnsupported` until milestone 3. `ctx` is looked at before reading, in the key derivation, between the derivation and the parsing, and every 1,024 XML tokens. Seven fuzz targets (header, VariantDictionary, inner header, block stream, gzip, XML, and `OpenBytes`) run their seeds in every test run.
- **Main files:** internal/core/kdbx/open.go, fuzz_test.go.
- **Tests:** every KDBX 4 fixture through `Open` with the expected `Info` and the rich content; the KDF parameters; a missing file, a folder, a text file, a wrong password, no credentials, a key file missing, a KDBX 3 file; KDBX 1, 5, KeePass 1 and pre-release signatures; the file is unchanged after `Open`; the file size limit; cancellation before reading and at every look at the context (`KdfCancelled` and `ReadCancelled` both occur, nothing else); about 12 s of fuzzing per target (`-parallel 1`, `GOMEMLIMIT=2GiB`) found nothing.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - SPEC §16 listed a package `internal/core/kdbx/xml`; the XML code is in `internal/core/kdbx` because it needs the container's types, and the empty package was removed. SPEC §16 is changed.
  - New tags `FileReadFailed`, `FileTooLarge` and `ReadCancelled` (SPEC §15), each with a route test.
  - `Open` takes `crypto.Credentials` (the key file content as a secret, not a path); the CLI reads the key file with `crypto.ReadKeyFile`.
  - Go's fuzzer spends most of the first half minute minimizing the 10 KB XML seeds of `FuzzParseXML4`; use `-fuzzminimizetime 2s` for short runs.

## 2.8 `passbank info`

- **Date:** 2026-11-05
- **Branch:** feat/step-2.8-cli-info
- **Summary:** `passbank info FILE` opens a database and prints, one fact per line in a fixed order: version, cipher, kdf (with memory, iterations and parallelism, or rounds), compression, inner stream, and the counts of groups, entries, attachments and history items. The credential options (`--keyfile`, `--password-file`, `--no-password`, with a `-a`/`-b` suffix for the commands with two inputs) are in `internal/cli/credentials.go`: the password comes from the option's file (first line; the file must be a regular file that group and others cannot access, checked on the open file) or from the terminal without echo, never from an argument or the environment. Options may come before or after the file. A failure prints one line with the message of SPEC §15 and exits 1; a mistake in the arguments exits 2 with the usage line; `--debug` adds tag, route and metadata.
- **Main files:** internal/cli/{info,credentials,errors}.go, internal/cli/testdata/golden/.
- **Tests:** golden output of `info` for seven fixtures (Argon2d, Argon2id, AES-KDF, all three ciphers, 4.0 and 4.1, with and without compression, empty); the prompt text; a password file (LF, CRLF, no newline, read-only 0400, only the first line, a file at the size limit, an empty one); key file with `--no-password`; key file with a password file; options first and last; every failure message (wrong password, no terminal, empty password, terminal error, file readable by others, missing password file / key file / database, not a database, a KDBX 3 file, no file, two files, unknown option, contradicting options); the `--debug` route; a write failure on stdout; cancellation; route tests for `PasswordFileTooOpen`, `PasswordFileReadFailed`, `PasswordNeedsTerminal`, `UsageInvalid`, `ReadCancelled`.
- **Checks:** (filled in when the checks pass)
- **Proof:** method C: the golden output of `passbank info` on the richest 4.x fixture, run with `go test ./internal/cli -run TestInfoGolden`:

```
version          4.0
cipher           AES-256
kdf              Argon2d
kdf-memory       8 MiB
kdf-iterations   2
kdf-parallelism  2
compression      GZip
inner-stream     ChaCha20
groups           4
entries          4
attachments      1
history          2
```

- **Manual checks:** pending, see docs/MANUAL-CHECKS.md (the prompt on a real terminal).
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - The plan named `PasswordFileTooOpen` only; reading problems of the password file are a new tag, `PasswordFileReadFailed`, and mistakes in the arguments are `UsageInvalid` (exit status 2).
  - An empty password is no password component (SPEC §12 now says so).
  - `--debug` is an option of every command; the other commands get it with their steps.

## 3.1 Version differences

- **Date:** 2026-11-05
- **Branch:** feat/step-3.1-version-differences
- **Summary:** SPEC §4.5 now describes KDBX 2.x, 3.0 and 3.1 completely (header fields and IDs, key derivation, stream start bytes, the hashed block stream, GZip, the inner streams, and every XML difference from KDBX 4: ISO times, `Meta/Binaries`, `Meta/HeaderHash`, no 4.1 elements, tags separated by commas) and records the decision. Sources: KeePassXC's `Kdbx3Reader.cpp`, `Kdbx3Writer.cpp` and `KeePass2.h` (GPL, read for behavior and put in own words), the three MIT implementations (gokeepasslib, keepass-rs, kdbxweb), and real files: every statement about the XML was checked on files from KeePass 2 and KeePassXC 2.7.
- **Decision (A10):** nothing is given up. The sources dispatch on the major version only; the only difference between 2.x, 3.0 and 3.1 is that `HeaderHash` is required from 3.1 (KeePassXC). 3.1 has twelve real files from other projects plus the four KeePassXC fixtures; 2.x and 3.0 have none anywhere Passbank may use, so they are verified in M5 by KeePassXC reading the files Passbank writes (SPEC §4.5 says what happens if it does not).
- **Main files:** docs/SPEC.md §4.5, internal/core/kdbx/testdata/ext/ (16 files), third_party/{keepass-rs,gokeepasslib,kdbxweb}/, NOTICE, testdata/FIXTURES.md, internal/core/kdbx/fixtures_test.go.
- **Tests:** `TestFixturesAreListed` also covers `ext/` (listed, and version 3.1).
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - **Found a bug in step 2.5** while reading the sources: KDBX 4 files from KeePassXC may carry the AES-KDF ID `7c02bb82-79a7-4ac0-927d-114a00648238` instead of `c9d9f39a-…` (keepass-rs reads both as the same function and writes the second one "for compatibility"). Passbank rejected the first as an unknown KDF. The fix (accept both, write `c9d9f39a-…`) is made in step 3.2's branch with a test.
  - KDBX 3.1 files exist with the ChaCha20 outer cipher (gokeepasslib, kdbxweb) and with the ChaCha20 inner stream (keepass-rs); the reader accepts them. The writer (M5) writes only what KeePass writes (AES-256 or Twofish with Salsa20).
  - KeePassXC writes entry tags in 3.1 files separated by `,`; the reader splits at `;` and `,` (already so in step 2.6).

## 3.2 The KDBX 3 container

- **Date:** 2026-11-05
- **Branch:** feat/step-3.2-kdbx3-container
- **Summary:** `readContainer3` reads the container of a KDBX 2.x or 3.x file: signature and version (major 2 or 3, any minor), the header (SPEC §4.5; unknown fields kept, a known field twice or a required one missing is `FileCorrupt`), AES-KDF key derivation, the cipher key SHA-256(master seed || transformed key), decryption with the check of the stream start bytes before the padding (`WrongCredentials`), the hashed block stream, GZip, and the SHA-256 of the header (for the `Meta/HeaderHash` check of step 3.3). All three outer ciphers work. `crypto.Cipher.Decrypt` was split into `DecryptRaw` and `Unpad` for this: a wrong key makes the padding invalid, which must not hide behind a `PaddingInvalid` error.
- **Main files:** internal/core/kdbx/{header3,container3}.go, crypto/cipher.go.
- **Tests:** all sixteen KDBX 3.1 files (the four from KeePassXC and twelve from KeePass, KeePassXC, kdbxweb, gokeepasslib and keepass-rs) open to XML; for each, the SHA-256 that Passbank computes of the header equals the `HeaderHash` in the file's XML, which checks the header bytes, the key derivation, the cipher, the block stream and the decompression at once; seven wrong-credential cases on real files; hand-built files of versions 2.0, 2.1, 3.0, 3.1 and 3.7 with each cipher, both compressions and tiny blocks; the inner stream IDs; unknown and comment header fields; 23 damaged-header and damaged-stream cases and 13 damaged-bytes cases, each with its route; every prefix of a file; cancellation; `readHashedBlocks` at its limit; and KDBX 4 files with either AES-KDF ID.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - Fixes a bug of step 2.5 found in step 3.1: a KDBX 4 header with the AES-KDF ID `7c02bb82-…` (KeePassXC) is now read as AES-KDF (`crypto.KDFAESKdbx4`, mapped to `KDFAES`); both IDs are tested.
  - The `PaddingInvalid` and `CiphertextInvalid` errors of the outer cipher still reach the caller as they are (they are damage, not wrong credentials, once the start bytes match).

## 3.3 The KDBX 3 XML and `Open` for every version

- **Date:** 2026-11-05
- **Branch:** feat/step-3.3-kdbx3-xml
- **Summary:** `kdbx.Open` and `kdbx.OpenBytes` now read every version: major 2 and 3 go through `readContainer3`, major 4 through `readContainer4`, any other major is `VersionUnsupported`. The XML reader (`parseXML`, formerly `parseXML4`) handles the old dialect: `Meta/HeaderHash` is checked against the header's SHA-256 (a mismatch is `FileCorrupt`; a missing one is an error from 3.1 on), `Meta/Binaries` becomes the attachment pool (base64, GZip when `Compressed`, decrypted with the inner stream when `Protected`, in document order), entries refer to it by ID, and content inside an entry (`<Value Compressed=…>`) is read too. `passbank info` works for every version.
- **Main files:** internal/core/kdbx/{xmlread,open}.go, testdata/dumps/, testdata/gen/dump_entries.py.
- **Tests:** the sixteen KDBX 3.1 fixtures through `Open` with their counts; thirteen of them compared line by line (title, user, password, URL, notes, tags, custom fields, attachment hashes, history counts) with dumps made by pykeepass, which is independent of Passbank; the rest with known values (the kdbxweb file with a protected attachment, whose later values prove the inner stream stays in step; the ChaCha20 inner stream; the shared and empty binaries of KeePassXC 2.7); the rich fixture made by KeePassXC as 3.1 and the one made by pykeepass as 4.0 from the same source give equal models by `model.Diff` after removing what the two programs do differently (field order, standard icon with a custom icon, KeePassXC's own custom data, the protected flag of an imported attachment); `HeaderHash` cases for 2.0, 3.0, 3.1, 3.2; binaries in `Meta/Binaries` and inline, every error with its route; built files of versions 2.0, 3.0 and 3.1 with the hash of their header in the XML, and a damaged header found through it; `passbank info` goldens for five KDBX 3.1 files; three more fuzz targets (header, block stream, `OpenBytes` on built KDBX 3 files), 10 s each, nothing found.
- **Checks:** (filled in when the checks pass)
- **Proof:** method C: `internal/cli/testdata/golden/info-k31-rich.txt` and the four `info-ext-*.txt` files.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - pykeepass is wrong about `ext/go-protected-binary.kdbx` (it does not decrypt a protected attachment of `Meta/Binaries`); Passbank reads the passwords the project documents.
  - A version-4 file whose XML has a `HeaderHash` element keeps it as unknown XML; only KDBX 2.x and 3.x files check it.
  - `Info.Attachments` counts the attachments of entries, not the pool.

## 4.1 The XML writer

- **Date:** 2026-11-05
- **Branch:** feat/step-4.1-xml-writer
- **Summary:** `marshalXML` writes a `model.Database` as the XML of any KDBX version: KDBX 4.0 and 4.1 (base64 times, attachments by index into the inner header, 4.1 elements for 4.1 only), and the old dialect of 2.x and 3.x (ISO times, `Meta/Binaries` compressed on request and protected by the inner stream, `Meta/HeaderHash`). Protected values are encrypted with the inner stream in document order; every unknown element and attribute is written back as it was read, without layout; attachments are numbered in order of first use (history included), shared ones once, unused pool entries left out. `MinVersion` gives 4.1 when the database holds a 4.1-only field.
- **Main files:** internal/core/kdbx/xmlwrite.go.
- **Tests:** eleven fixtures (4.0, 4.1, 3.1, with unknown XML, public custom data, a protected attachment, the ChaCha20 inner stream, Cyrillic) are written as 4.1, 4.0, 3.1, 3.0 and 2.0, with and without compressed binaries, and read back by the reader of 2.6/3.3: `model.Diff` is empty (for 4.0 and older after removing the 4.1 fields); the database is not changed by writing; the XML has the dialect of its version and shows no secret; escaping of `& < > " '`, line breaks, tabs, `]]>` and CDATA in names, text, attributes, tags, raw XML and protected values; times at the limits (year 1, 1970, 9999, a zone and fractions); the attachment pool (order, sharing, the protected flag, copies for the inner header, history-only attachments); a database without a root group and an attachment missing from the pool; `MinVersion` for every 4.1 field, in a deep group and in a history item.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - **Found and fixed a bug of step 2.6** through the round trip: the reader took a base64 time for an ISO time when the base64 contained a capital `T` (it looked for `T`, `:` or `-`). It now looks for `:` or `-`, which base64 never has. The test has a time whose base64 contains a `T`.
  - The plan's `Lost` report is not made here: `kdbx.Losses` of step 5.1 reports what a version cannot store, for every version at once.
  - `MinVersion` also returns 4.1 for modification times of custom data items and for the settings-changed time, which SPEC §5 now lists.
  - Raw XML is written without layout, because the text of a node is data; the rest of the XML is written with a line break after each element, which the reader ignores.

## 4.2 The KDBX 4 container writer

- **Date:** 2026-11-05
- **Branch:** feat/step-4.2-container-writer
- **Summary:** `kdbx.Encode(ctx, db, version, settings, creds)` returns the bytes of a KDBX 4.0 or 4.1 file. `EncryptionSettings` (cipher, KDF with parameters, compression), `DefaultSettings(version)` (A5: AES-256, GZip, Argon2id 64 MiB / 2 lanes / 10 iterations for 4.x; AES-KDF with 2,000,000 rounds for 3.x and 2.x) and `SettingsOf(info)` (the settings of a file that was read). Every write draws a new master seed, IV, KDF seed and inner stream key (64 bytes, ChaCha20) from `crypto/rand`; the header carries its SHA-256 and HMAC, the inner header the attachments, and the encrypted payload is split into 1 MiB HMAC blocks. The AES-KDF is always written with the ID `c9d9f39a-…`. The helpers the test builder had (`encodeVariant`, `field`, `writeBlocks`) moved to `wire.go`.
- **Main files:** internal/core/kdbx/{encode,wire}.go.
- **Tests:** every cipher × KDF × compression writes a file that `Open` reads back to an equal model with the settings that were asked for, and a wrong password does not open it; nine fixtures (4.0, 4.1, 3.1, unknown XML, public custom data, attachments, Cyrillic) written at `MinVersion` and read back equal; 4.0 leaves out the 4.1 fields and 4.1 keeps them; two writes differ in every seed, in the IV and in the encrypted values, and give equal models; a 3 MiB incompressible attachment spans four blocks; key file only, password and key file, a Unicode password; the caller's credentials are not wiped; the defaults and `SettingsOf` (a Twofish/AES-KDF file is written again with its settings); eleven error cases with routes (version 5 and 4.2, no database, an unknown cipher, compression and KDF, Argon2 without memory, AES-KDF without rounds, no credentials, cancellation, a missing attachment) and a failing random source at each of the four draws.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none (KeePassXC opens the files in step 4.4's interop tests).
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - `Encode` writes only KDBX 4 until step 5.2 adds the old container; other versions give `VersionUnsupported`.
  - A database with an empty `Meta.Generator` is written with the generator `Passbank`, so it does not read back equal; the merge sets the generator.

## 4.3 Safe output

- **Date:** 2026-11-05
- **Branch:** feat/step-4.3-safe-output
- **Summary:** `output.Write(ctx, path, db, version, settings, creds, opts)` encodes the database, writes it to a temporary file in the folder of the path (mode 0600, synced), reads that file back with the same credentials and requires it to equal the database as the version holds it (`kdbx.Fit`), and only then puts it in place, by a hard link that fails if the path exists (or by a rename with `Overwrite`), and syncs the folder. `OutputExists` without `Overwrite`; `OutputIsInput` when the path is any of `Inputs` by `os.SameFile`, so also through links and other spellings, also with `Overwrite`. No temporary file is left behind and an existing file is unchanged by any failure before it is replaced. The file system is an interface (`output.FS`) so that every step can be made to fail.
- **Main files:** internal/core/output/{output,fs}.go, internal/core/kdbx/fit.go (`Fit`: the database as a version holds it).
- **Tests:** a new file (content, mode 0600, no other file in the folder); an existing file is refused and unchanged, then replaced with `Overwrite`; eight cases of `OutputIsInput` (same path, spelling, `..`, symbolic and hard links, either input, with and without `Overwrite`) and inputs that do not matter; empty path, missing folder, a folder at the path; encoding errors; seven injected faults (create, write, sync, close, link, removing the temporary name, syncing the folder) each with its route, with the folder checked and, for the six that fail before the file is in place, the same fault over an existing file leaving it unchanged; a file made by someone else between the check and the link survives; verification: a flipped bit in what is written, and a database that differs (the metadata names paths and UUIDs only, never values), cancellation during the verification; `Write` does not change the database or the credentials, and a 4.0 file of a 4.1 database holds no 4.1 field.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - The plan verified the file at its final path and removed it on a mismatch; this verifies the temporary file before it is placed, so a bad file never appears at the path and an existing file is never replaced by one.
  - `Write` reports `WriteCancelled` for a cancellation during the verification, not `WriteVerifyFailed`.
  - After `OutputSyncFailed` the file is in place and complete (SPEC §15 says so).

## 4.4 Round trip and interoperability

- **Date:** 2026-11-05
- **Branch:** feat/step-4.4-round-trip
- **Summary:** proof that what Passbank writes keeps everything and opens in KeePassXC. `internal/core/model/modeltest.Random(seed, options)` makes random databases (trees up to depth 3 with several groups and entries each, standard and custom and protected fields with awkward characters, shared and empty and protected attachments, histories, custom data, custom icons, tri-states, auto-type, unknown XML on the file, Meta, groups and entries, public custom data, 4.1 fields on request), determined by the seed. `kdbx.Fit` also leaves out of the pool the contents that no entry uses, as a file does.
- **Main files:** internal/core/model/modeltest/, internal/core/kdbx/{roundtrip,property,interop}_test.go.
- **Tests:** all 27 fixtures of every version (11 KDBX 4, 16 KDBX 3.1) are read, written as KDBX 4 (4.0 or 4.1 by `MinVersion`) and read back, with `model.Diff` empty; 60 random databases (every second with 4.1 fields) through random cipher × KDF × compression equal after the round trip, and in the version `MinVersion` says; 30 random databases with 4.1 fields written as 4.0 come back equal to `Fit(db, 4.0)`; the generator is deterministic, bounded, has unique UUIDs, valid references, and its options turn things off.
- **Interop (build tag `passbank_keepassxc`, KeePassXC 2.7.12 from the Flatpak):** `go test -tags passbank_keepassxc ./internal/core/kdbx` — KeePassXC opens files of all 18 combinations of cipher (AES-256, Twofish, ChaCha20), KDF (AES-KDF, Argon2d, Argon2id) and compression, and also the nine fixtures written by Passbank (one per kind), lists exactly the entries of the model (`ls -R -f`) and shows the title, user name, password, URL and notes of every entry as the model has them (`show -s`); it rejects a wrong password and accepts the right one. All pass.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - `keepassxc-cli` shows custom fields and attachments differently from the model, so the interop tests compare the five standard fields and the entry lists; the rest is covered by the round trip with Passbank's own reader, which two independent readers (pykeepass, KeePassXC) agree with on the fixtures.
  - Entries without a title cannot be named to `keepassxc-cli`, so the interop tests skip them.

## 5.1 The loss check and encryption limits

- **Date:** 2026-11-05
- **Branch:** feat/step-5.1-losses
- **Summary:** `kdbx.Losses(db, version)` lists, in document order, what the database holds that the version has no place for, as `Loss{Kind, Item, UUID, What}` (item path and UUID, never a value): for 4.0 the 4.1 fields (group tags, previous parent of groups and entries, custom icon names and times, times of custom data items, the settings-changed time, the quality-check flag); for 3.1, 3.0 and 2.x also the custom data of groups and entries (with their history) and the public custom data. `kdbx.Supports(version, settings)` is `EncryptionNotInVersion` for 2.x and 3.x with anything but the AES-KDF and AES-256 or Twofish. `Fit` now follows the same table of limits, so `Losses(Fit(db, v), v)` is always empty. `output.Write` refuses with `EncryptionNotInVersion` or `OutputVersionLosesData` (count and the first ten losses in the metadata) unless `Options.AllowLoss`, before anything is written.
- **Main files:** internal/core/kdbx/{losses,fit}.go, internal/core/output/output.go.
- **Tests:** a database with every kind of data checked against 4.1, 4.0, 3.1, 3.0 and 2.0 (the kinds lost in each), the items and wording of the losses with no value in them, history items, plurals, a database without a root group; 40 random databases × 5 versions: `Fit` leaves nothing to lose, is idempotent, and changes nothing when no loss is reported (but unused pool contents); databases without 4.1 fields lose nothing in 4.0; `Supports` for eight combinations with routes; `output.Write` refuses a loss without `AllowLoss`, names the items, leaves the folder and an existing file alone, and writes the same database as 4.1; it refuses Argon2 for 3.1.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:**
  - The fixtures made by pykeepass carry the 4.1 element `SettingsChanged` in files marked 4.0; writing such a file as 4.0 is therefore a (small) loss, and `MinVersion` returns 4.1 for it. The output tests clear that field where they write 4.0.
  - One `Loss` per item and kind, not per field: an entry whose history has three quality-check flags gives one `EntryQualityCheck`.

## 5.2 The KDBX 3 writer

- **Date:** 2026-11-05
- **Branch:** feat/step-5.2-kdbx3-writer
- **Summary:** `kdbx.Encode` writes KDBX 3.1, 3.0 and 2.0 as well: the header of SPEC §4.5 with new random seeds, IV, inner stream key and stream start bytes on every write, AES-KDF, the cipher key SHA-256(master seed || transformed key), the XML in the old dialect with `Meta/HeaderHash` (the SHA-256 of the header), the Salsa20 inner stream, GZip or none, the hashed block stream in 1 MiB blocks, and AES-256 or Twofish. `Supports` is checked first. `output.Write` accepts every version (nothing else in it changed).
- **Main files:** internal/core/kdbx/encode3.go (the header and block helpers are in encode3.go and wire.go).
- **Tests:** versions 3.1, 3.0 and 2.0 × AES-256 and Twofish × both compressions read back equal to `Fit(db, version)` with the settings that were asked for, and a wrong password does not open them; all 27 fixtures (KDBX 4 and 3.1) written in all three versions read back equal; 40 random databases with 4.1 fields and item custom data in all three versions; two writes differ in every seed, the IV and the inner key; the `HeaderHash` in the XML is the hash of the header of the file; a KDBX 3.1 file that KeePass or KeePassXC wrote, read and written again keeps every entry as pykeepass dumped it; nine error cases with routes (versions 3.2, 2.1 and 1.0, no database, Argon2, ChaCha20, an unknown compression, zero rounds, no credentials), cancellation, a missing attachment, and a failing random source at each of the five draws.
- **Interop:** `go test -tags passbank_keepassxc ./internal/core/kdbx` — KeePassXC 2.7.12 opens 120 files (3.1, 3.0 and 2.0 × AES-256 and Twofish × both compressions × five databases including attachments and a Cyrillic password), lists exactly the entries of the model and shows the five standard fields as the model has them. **KDBX 2.x writing is therefore kept** (SPEC §4.5).
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:** Passbank writes the versions 2.0, 3.0 and 3.1 only (other minors of 2 and 3 are `VersionUnsupported`), and its 4.x files are 4.0 or 4.1.

## 5.3 Round trip in every version

- **Date:** 2026-11-05
- **Branch:** feat/step-5.3-round-trip-matrix
- **Summary:** the round-trip matrix of SPEC §17, through `output.Write`: every database is written in 4.1, 4.0, 3.1, 3.0 and 2.0 with settings the version holds (all ciphers and KDFs in 4.x, AES-256 and Twofish with the AES-KDF in the others, both compressions) and read back.
- **Main files:** internal/core/output/matrix_test.go.
- **Tests:** 20 fixtures (KDBX 4 and 3.1, every cipher and KDF, unknown XML, public custom data, 4.1 fields, attachments, a protected attachment, ChaCha20 as inner stream, Cyrillic) and 40 random databases (half with 4.1 fields, a third without custom data on groups and entries) × 5 versions = 300 cells. In each: without `AllowLoss` the write is refused with `OutputVersionLosesData` exactly when `Losses` lists something, and a refused write leaves nothing in the folder; with `AllowLoss` the file reads back equal to the database as the version holds it (`Fit`); when nothing is lost the file reads back equal to the original, except the contents the entries do not use. All green.
- **Checks:** (filled in when the checks pass)
- **Proof:** none.
- **Manual checks:** none.
- **Glutter issues:** none.
- **Deviations and follow-ups:** the matrix checks that what differs is "exactly what `Losses` reports" in two halves, because `Diff` and `Losses` speak different languages: `Fit(db, v)` is what the version holds (and `Losses(Fit(db, v), v)` is empty, step 5.1), and the file equals it.

