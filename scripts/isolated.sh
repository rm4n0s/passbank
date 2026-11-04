#!/usr/bin/env bash
# Runs a command so that it can't touch the user's real configuration, state,
# cache, data or runtime folders (CLAUDE.md "Don't").
#
# Usage: scripts/isolated.sh -- <command> [args...]
#
# HOME and the XDG folders point into a fresh temporary folder, which is removed
# when the command ends.
#
# Two things stay reachable. Go keeps its caches (GOCACHE, GOMODCACHE and GOPATH
# are fixed before the folders move), so `go run` and `go build` still work. A
# Wayland compositor stays reachable: when WAYLAND_DISPLAY names a socket, a
# link to it is put in the private runtime folder.
#
# fontconfig rebuilds its cache in the temporary folder, and does not see the
# user's own fonts, so the first run is a little slower and reproducible.
#
# Exit status: the command's own; 64 for a usage mistake.
set -euo pipefail

usage() {
  echo "usage: $0 -- <command> [args...]" >&2
  exit 64
}

[[ "${1:-}" == "--" ]] || usage
shift
[[ $# -ge 1 ]] || usage

# Go's folders, while the real ones are still in force.
if command -v go >/dev/null 2>&1; then
  GOCACHE="$(go env GOCACHE)"
  GOMODCACHE="$(go env GOMODCACHE)"
  GOPATH="$(go env GOPATH)"
  export GOCACHE GOMODCACHE GOPATH
fi

# A short path: a Unix socket's path is limited to about 100 bytes.
tmp="$(mktemp -d "${TMPDIR:-/tmp}/passbank-iso.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$tmp/home" "$tmp/config" "$tmp/state" "$tmp/cache" "$tmp/data" "$tmp/run"
chmod 700 "$tmp/run"

# Keep the Wayland compositor reachable through the private runtime folder.
if [[ -n "${WAYLAND_DISPLAY:-}" ]]; then
  case "$WAYLAND_DISPLAY" in
    /*) socket="$WAYLAND_DISPLAY" ;;
    *) socket="${XDG_RUNTIME_DIR:-}/$WAYLAND_DISPLAY" ;;
  esac
  if [[ -S "$socket" ]]; then
    ln -s "$socket" "$tmp/run/$(basename "$socket")"
    export WAYLAND_DISPLAY="$(basename "$socket")"
  fi
fi

export HOME="$tmp/home"
export XDG_CONFIG_HOME="$tmp/config"
export XDG_STATE_HOME="$tmp/state"
export XDG_CACHE_HOME="$tmp/cache"
export XDG_DATA_HOME="$tmp/data"
export XDG_RUNTIME_DIR="$tmp/run"

# Not exec: the trap must run afterwards to remove the folder.
set +e
"$@"
status=$?
exit "$status"
