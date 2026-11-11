# CLAUDE.md

Guidance for Claude Code when working in this repository (Go, hosted on GitHub).

## Project
Passbank merges KeePass databases, written in Go. It reads two KDBX databases of any version (KDBX 2.x, 3.0, 3.1, 4.0 and 4.1, in any combination) and writes a **new** KDBX database with the merged result, in any KDBX version (KDBX 4 by default); the two inputs are never modified. Nothing an input holds may be lost silently: when the chosen output version cannot store something, Passbank refuses to write unless the user accepts the listed losses, as `docs/SPEC.md` defines. It is not a password manager: viewing and editing entries is left to KeePassXC and other KeePass clients, and the databases Passbank writes must open in them.

The user decides what the result keeps: Passbank lists the entries that exist in only one input and the entries that exist in both but differ, and for each one the user picks what goes into the new database. The exact matching, comparison and merge rules are defined in `docs/SPEC.md`.

There are two applications on one shared core:
- **`passbank`**, the command-line tool. It never imports Glutter
- **`passbank-gui`**, the desktop application. Its user interface is built on [Glutter](https://github.com/rm4n0s/glutter), a cgo-free GUI toolkit for Go with Flutter's framework API (Wayland/X11, Vulkan with an OpenGL ES 3.0 fallback)

Everything that decides the result (reading and writing KDBX, matching, comparing, applying the user's choices) lives in the core, so both applications produce the same database from the same inputs and choices. Passbank targets Linux only.

**`docs/SPEC.md` is the source of truth.** Read the sections that cover a task before starting it. If the code and the spec disagree, or a task needs a decision the spec doesn't make, ask instead of guessing. When a decision changes, update the spec on the same branch.

## Commands
- Build: `go build ./...`. Passbank is cgo-free; `CGO_ENABLED=0 go build ./...` must always work
- Test all: `go test -race -tags debug_mode ./...` (the race detector needs cgo, so this one needs a C compiler). `-tags debug_mode` makes a debug build of Glutter and of the GUI's debug flags; since Glutter 1.17 a build without tags is a release build, which the cgo-free run below tests
- Test the cgo-free path: `CGO_ENABLED=0 go test ./...`
- Test one package: `go test ./internal/core/<pkg> -run TestName -v`
- Tests that need a real display/GPU: `go test -race -tags passbank_display ./...`
- Update golden images: `go test ./... -run Golden -update-goldens`, then review every changed PNG
- Heavy commands (full test runs, lint, release builds, display tests, measurements): through `scripts/heavy.sh` (`--alone` for display tests and measurements), see "Memory (OOM)"
- Vet: `go vet ./...`
- Lint: `golangci-lint run`
- Format: `gofmt -w cmd internal`
- Generate: `go generate ./...`
- Run the CLI: `go run ./cmd/passbank <args>`
- Run the GUI: `go run -tags debug_mode ./cmd/passbank-gui` (debug build; it uses your real configuration)
- Run either isolated from your configuration: `scripts/isolated.sh -- go run ./cmd/<passbank|passbank-gui> <args>`
- Make a proof screenshot of the GUI: `scripts/proof.sh <out.png> [--after <duration>] -- <passbank-gui args>` (docs/IMPLEMENTATION.md "Proof methods")
- Release builds: `go build ./cmd/passbank` and `go build ./cmd/passbank-gui` (a build without tags is a release build of Glutter)
- After changing dependencies: `go mod tidy`

## Definition of done
Before reporting a task as finished, run `gofmt` (on `cmd` and `internal`), `go vet ./...`, `golangci-lint run` and `go test -race -tags debug_mode ./...`, and fix anything they report. Say so explicitly if any step was skipped or failed. Also:
- `CGO_ENABLED=0 go build ./...` and `CGO_ENABLED=0 go test ./...` pass (proves the app stays cgo-free)
- `scripts/error-tags.sh --untested` prints nothing (every error Tag has a test that asserts its route; `docs/ERROR-TAGS-UNTESTED.md` is the only exception list)
- Every new exported identifier has a doc comment
- Every changed golden image has been looked at, not just regenerated

## Layout
- `cmd/passbank/` is the CLI's `main` package; `cmd/passbank-gui/` is the GUI's `main` package. Both stay thin: they parse arguments and start their frontend
- `internal/core/...` is the shared core (KDBX reading and writing, the data model, matching, comparing, merge choices and their application, settings). It never imports Glutter, `internal/gui` or `internal/cli`, so it is tested without a UI
- `internal/cli/...` is the command-line frontend. It imports the core, never Glutter or `internal/gui`
- `internal/gui/...` is the UI on Glutter: the widget kit, screens, views and dialogs. It imports the core, never `internal/cli`, and nothing imports it but `cmd/passbank-gui`
- `internal/archtest/` checks the package boundaries and forbidden imports, including that `cmd/passbank` does not link Glutter
- The UI is built on Glutter's `widgets` layer; no Material widgets
- `third_party/` holds the licenses of bundled assets (fonts, icons); `NOTICE` lists them
- `docs/SPEC.md` is the specification; `docs/IMPLEMENTATION.md` is the step-by-step plan; `docs/IMPLEMENTED.md` logs finished steps; `docs/implemented-proof-images/` holds each step's proof screenshots
- `scripts/` holds helper scripts
- Tests live next to the code (`foo_test.go`); golden images live in `testdata/goldens/` of their package
- The exact package list is defined in `docs/SPEC.md`

## Go conventions
- Never ignore a returned error; handle it or return it
- Pass `context.Context` as the first parameter of anything that does I/O (files, processes, network). Name it `ctx`. Name a Glutter `BuildContext` `bc`
- Tests are table-driven
- No global mutable state and no singletons. Package-level read-only values are fine and are never reassigned
- Prefer the standard library. Allowed dependencies: `github.com/rm4n0s/glutter` (v1.9.9 or newer), `github.com/rm4n0s/errors`, and what Glutter brings in (`golang.org/x/text`, `golang.org/x/image`, `golang.org/x/sys`, `github.com/go-text/typesetting`, `github.com/godbus/dbus/v5`), `golang.org/x/term` for reading passwords without echo in the CLI, and `golang.org/x/crypto` for the KDBX primitives the standard library lacks (Argon2, ChaCha20, Salsa20, Twofish). Never write a cryptographic primitive yourself. A step that first imports one of them adds it with `go get <module>@<version>`. Ask before adding any other
- **No cgo**: never write `import "C"`. If a C library is ever needed, ask first
- Only Glutter's UI thread touches widgets and render objects. Other goroutines (file watchers, I/O, background work) hand work over through `SetState` or the binding's `Post`
- Long-running work never blocks the UI thread, and takes a `ctx` so it can be cancelled

## Glutter
- Follow Glutter's API conventions: `NewX(required..., opts ...XOpts) *X` constructors, options structs, `foundation.Opt[T]` for nullable options. Passbank's own widgets follow the same conventions
- When Glutter is missing something or has a bug, work around it in Passbank without stopping, and mark the workaround with a `// Glutter:` comment that says what it works around and what Glutter should provide instead
- Keep a list of these workarounds in `docs/GLUTTER-ISSUES.md` (what is missing or broken, where the workaround is, and what the fix in Glutter would be), and mention new entries in the report at the end of the task, so they can be fixed in Glutter later. Create the file with the first entry; remove an entry when the fix is in the Glutter version Passbank uses, and delete the file when no entry is left
- Read Glutter's source in the Go module cache when its behavior is unclear; its `docs/SPEC.md` describes its design

## Error handling
Use `github.com/rm4n0s/errors` for all errors. Never use `fmt.Errorf`, and never import the standard library `errors` package: this package re-exports `errors.Is`, `errors.As` and `errors.Unwrap`, so a single import covers everything. Read `AI_GUIDE.md` of github.com/rm4n0s/errors (in the Go module cache: `go list -m -f '{{.Dir}}' github.com/rm4n0s/errors`) before writing error code.

**Creating errors**
- New failure with a fixed message: `errors.New("Tag", "message", "key", value, ...)` (all metadata in this one call, never `New(...).SetMetadata(...)`)
  - Tag: short PascalCase name of the failure, e.g. `FileOpenFailed`
  - Trailing arguments are key/value metadata pairs, e.g. `"path", path`
- New failure with a formatted message: `errors.NewErrf("Tag", "format %s", arg)`
- Wrapping an error that is NOT already an `*errors.Error` (stdlib, third-party, Glutter errors that are not from this package): `errors.NewErr("Tag", err)`. Add metadata with one `.SetMetadata("key", value, "key2", value2)` call (a later call merges into the metadata, it does not replace it, but use one call unless the data only becomes available later)
- Every `New`, `NewErr` and `NewErrf` call captures the stack at that point
- Construct the error in the function that detects the failure, never in a helper that only builds it (`errNoWindow()`, `fail(tag, msg)`): the route is the stack at the constructor, so a helper puts its own name in every route. The tag is a string literal at the call
- Sentinels are struct literals, `&errors.Error{Tag: "X"}`, never a package-level `errors.New`: its stack is `init`, so every route through it is wrong
- Never put secrets (passwords, keys, tokens, decrypted data) in an error's message or metadata: errors are logged with all their metadata

**Propagating errors**
- If the error already came from this package, return it untouched: `return err`. Re-wrapping it would replace the Tag and the stack and break route tests
- Functions return the plain `error` interface, never `*errors.Error` (avoids the non-nil-interface-holding-nil-pointer bug)
- `errors.FromError(err)` is a plain type assertion. It fails if something else wrapped the error, so don't wrap these errors with anything

**Inspecting errors**
- `appErr, ok := errors.FromError(err)`, then use `appErr.Tag`, `appErr.Metadata`, `appErr.Message`
- Use `errors.Is` / `errors.As` / `errors.Unwrap` for the underlying cause stored in `OriginalErr`

**Testing errors**
- Every new failure case gets its own unique Tag and a test that asserts the route, not just that an error occurred:
  ```go
  appErr, ok := errors.FromError(err)
  if !ok {
      t.Fatalf("expected *errors.Error, got %T", err)
  }
  if !appErr.HasRoute("Vault.Open->Store.Load.FileOpenFailed") {
      t.Errorf("unexpected failure path: %s", appErr.Route())
  }
  ```
- A route lists the call chain from outermost to innermost (`Type.Method` or `Func`, joined by `->`) and ends with the Tag. `HasRoute` matches by substring
- Log errors with `slog`, passing the error as an attribute (`l.Error("failed", "err", err)`): `*errors.Error` is a `slog.LogValuer`, so the log gets the Tag, Route, Metadata and stack frames, and the logged object can be replayed in a test via `errors.ErrorJson` and `errors.FromJson`. Do not call `ToJson` or `json.Marshal` on an error yourself

## Git and GitHub
- Never commit or push directly to `main`; work on a branch named `feat/...` or `fix/...`
- **Exception:** if `docs/IMPLEMENTATION.md` exists, follow its Rules section for the steps it lists. Each step is done on its own branch, one at a time. A finished step (all four Definition-of-done checks passing, its entry added to `docs/IMPLEMENTED.md` on that branch) is fast-forward merged into the local `main`, without a PR, and tagged `step-<id>` locally. **GitHub gets milestones, not steps:** `main` and the tags are pushed once, when the last step of a milestone (a `## M<n>` section of `docs/IMPLEMENTATION.md`) is merged. Never commit work for a step directly on `main`. Everything not covered by that file (other changes, fixes outside the listed steps) still follows the rule above
- Commit messages: imperative mood, e.g. "Add retry to client"
- PRs: short description, link the issue (`Closes #123`), keep them small
- No `.github/` directory and no GitHub Actions workflows: never add them. The four Definition-of-done checks are run locally

## Working through the steps
- **Never stop until every step in `docs/IMPLEMENTATION.md` is implemented** (through the last one in the file; as the plan grows, the last step moves). A finished step is never the end of a turn: the turn's last action is a tool call that works on the next step, not a summary. Do not write "starting the next step now" and then end the turn; start it. Report a finished step (and a published milestone) in one or two lines in the middle of the work, and keep working
- Don't stop to ask for confirmation, and don't end a turn with a question such as "Should I start the next step?", until every step in `docs/IMPLEMENTATION.md` is finished (through the last one). After a step is merged (or a milestone published), report it briefly and start the next step in the same turn
- The only reasons to stop are the ones `docs/IMPLEMENTATION.md` names (an ambiguous step, a wrong spec, a missing tool, a new dependency, `sudo`, a check that keeps failing), a push of a milestone that keeps failing, and a step that says **ask the user**. Then stop, ask, and continue with the same step after the answer
- **Manual checks** don't stop the work: add them to `docs/MANUAL-CHECKS.md` (create it if needed) with the step id and the exact command for the user, write "pending, see docs/MANUAL-CHECKS.md" in the step's `IMPLEMENTED.md` entry, and go on. When the user answers, record the answer in that entry and remove the item from the list (delete the file when it is empty)
- A request such as "publish to main" that is already satisfied gets a one-line answer, then work continues with the next step

## Don't
- Don't edit generated files (`*_gen.go`); regenerate them with `go generate ./...`
- Don't commit secrets or `.env` files
- Don't log secrets, and don't let them reach golden images, proof screenshots or test fixtures other than throwaway test values
- Don't let a Glutter type appear in the core's or the CLI's packages
- Don't put merge logic in a frontend: if the CLI and the GUI would both need it, it belongs in the core
- Never modify an input database; write the result to a new file, and never overwrite an existing file unless the user asked for it explicitly
- Never take a password as a command-line argument or an environment variable (other processes can read them): read it from the terminal without echo, or from a file or file descriptor the user names
- Don't leave processes running: every CLI or GUI instance or compositor started for a test, a proof or a check is stopped when it is no longer needed (run it under `timeout -k`, in its own process group), and `pgrep -a` shows nothing left before a task is reported done
- Never look for, list, open or read files outside this project folder (no home-directory databases, configs, logs or other projects). If something outside it is needed, ask the user to provide it. Exceptions: dependency source in the Go module cache (including Glutter's), and temporary folders your own tests and scripts create
- Never let a test, proof or check use the user's real Passbank configuration, data, state, cache or socket: Go tests isolate the `XDG_*` folders (`internal/testenv`), and scripts run the app through `scripts/isolated.sh` or `scripts/proof.sh`

## Memory (OOM)
A run that uses too much memory is killed by the OS (exit 137) and takes the session with it. Be careful:
- Run the race tests with `GOMEMLIMIT=3GiB go test -race -p 2 -tags debug_mode ./...`. At most two heavy test or build commands run at the same time on this machine (30 GB, 32 CPUs), counting those of every other session on this machine; real-input display tests and timing measurements run alone. `scripts/heavy.sh -- <command>` enforces it: it waits for one of two machine-wide slots, and `scripts/heavy.sh --alone -- <command>` takes both. Run every full test run (`./...`), `golangci-lint run`, release build and display or timing run through it
- Never run Go fuzzing (`-fuzz`) without `-parallel=1`, a short `-fuzztime` (30s at most) and a memory limit (`GOMEMLIMIT=2GiB`, `ulimit -v`); each fuzz worker grows its own memory. Prefer the seed corpus and seeded random tests
- Run the tests of one package while working; run `./...` only for the final checks
- Tests, proofs and scripts that start compositors or the app run under `timeout -k`, and a test that builds big inputs keeps them bounded
- If a command is killed (137), find out why before running it again: it may be a real memory bug (unbounded growth, a pathological input) that needs a fix and a regression test

## Testing notes
- The core is tested with plain Go tests; no UI needed. The KDBX reader has fixture databases for every version, cipher (AES, ChaCha20, Twofish) and key-derivation function (AES-KDF, Argon2d, Argon2id) it accepts, created with KeePassXC or KeePass and committed under `testdata/` with their test passwords. Merge tests build their input databases in the test (or use small fixtures in `testdata/`), and check that the written database reads back and opens in a second, independent read
- The CLI is tested in-process through `internal/cli` with fake stdin, stdout and terminal; its output is checked against golden text files
- Widget tests use `gluttertest` (`PumpWidget`, `Pump`, `Tap`, finders) with a fake clock and the fake platform; no display needed
- Golden tests use `gluttertest`'s `MatchesGolden`, which renders through Glutter's software renderer, so they are deterministic and need no GPU
- Tests that open real windows carry the `passbank_display` build tag and are skipped by the default test run; proofs and display tests run in a headless compositor, never on the user's desktop unless a step says so
- **Never the NVIDIA card** (the user: "dont use my nvidia card to test"): nothing started for a test, a proof, a check or a measurement may open it. `scripts/gpu-env.sh` (sourced by the headless-compositor, proof and real-input scripts) and the display-test helper load Mesa only for EGL and GLX and hide NVIDIA's Vulkan driver; anything new that starts a GPU client uses one of them
- UI tests use bundled test fonts through `internal/gui/testutil`, never system fonts
- Every error Tag has a route test
