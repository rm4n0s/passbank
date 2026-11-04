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
