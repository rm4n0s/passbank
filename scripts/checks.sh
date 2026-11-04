#!/usr/bin/env bash
# Runs the standard checks of docs/IMPLEMENTATION.md rule 9 (CLAUDE.md "Definition of done") and prints one line
# for each: gofmt, go vet, golangci-lint, go test -race, the cgo-free build and tests, and the error-tag check. The
# heavy ones run through scripts/heavy.sh. The exit status is 0 only when all pass; the output of a failed check follows
# its line.
#
#   scripts/checks.sh
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
failed=0

# check <name> <command...>: runs the command, prints "ok" or "FAILED" and its output when it failed.
check() {
  local name="$1" out
  shift
  if out="$("$@" 2>&1)"; then
    printf '%-28s ok\n' "$name"
  else
    printf '%-28s FAILED\n%s\n' "$name" "$out"
    failed=1
  fi
}

unformatted() { test -z "$(gofmt -l cmd internal)"; }

check "gofmt" unformatted
check "go vet" go vet ./...
check "golangci-lint" scripts/heavy.sh -- golangci-lint run
check "go test -race" scripts/heavy.sh -- env GOMEMLIMIT=3GiB go test -race -p 2 ./...
check "CGO_ENABLED=0 build" env CGO_ENABLED=0 go build ./...
check "CGO_ENABLED=0 test" scripts/heavy.sh -- env CGO_ENABLED=0 go test ./...
check "error-tags --untested" bash -c 'out="$(scripts/error-tags.sh --untested)" && test -z "$out"'
exit "$failed"
