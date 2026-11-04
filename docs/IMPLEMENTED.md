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
