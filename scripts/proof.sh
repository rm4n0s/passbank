#!/usr/bin/env bash
# Makes a proof screenshot of the Passbank GUI (proof method B in
# docs/IMPLEMENTATION.md).
#
# Usage: scripts/proof.sh <out.png> [--after <duration>] [-- passbank-gui args...]
#
# It builds a debug binary (so the debug-only flags work) into a temporary folder,
# and runs it, isolated (scripts/isolated.sh) and in a headless Wayland compositor
# (scripts/headless-wayland.sh), never on the user's desktop and never with the
# user's configuration. Glutter saves the first window as a PNG and the app exits.
#
#   --after <duration>  how long Glutter waits before it captures (GLUTTER_SCREENSHOT_AFTER,
#                       such as 500ms or 2s)
#
# GLUTTER_RENDERER is gles for the run unless the environment sets it (the headless compositor has no GPU, and Glutter
# would try Vulkan first). PROOF_TIMEOUT (seconds, default 90) stops a run that hangs. Needs sway: sudo dnf install sway
#
# Exit status: 0 and a non-empty PNG on success; 1 when no image was written; 2 for
# a missing tool; 64 for a usage mistake.
set -euo pipefail

usage() {
  echo "usage: $0 <out.png> [--after <duration>] [-- passbank-gui args...]" >&2
  exit 64
}

[[ $# -ge 1 ]] || usage
out="$1"
shift
[[ "$out" == *.png ]] || usage

after=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --after)
      [[ $# -ge 2 ]] || usage
      after="$2"
      shift 2
      ;;
    --)
      shift
      break
      ;;
    *) usage ;;
  esac
done
args=("$@")

# Paths given relative to where the user stands are fixed before moving to the repository.
mkdir -p "$(dirname "$out")"
out="$(cd "$(dirname "$out")" && pwd)/$(basename "$out")"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/scripts/gpu-env.sh" # never the NVIDIA card
cd "$root"

for tool in go sway; do
  command -v "$tool" >/dev/null 2>&1 || { echo "error: $tool is not installed (sudo dnf install sway)" >&2; exit 2; }
done

tmp="$(mktemp -d "${TMPDIR:-/tmp}/passbank-proof.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "proof: building"
# A debug build (-tags debug_mode): the --dev flags of the proofs exist only there.
go build -tags debug_mode -o "$tmp/passbank-gui" ./cmd/passbank-gui

rm -f "$out"

# The run must not reach the user's compositor: the headless one replaces it.
run=(env -u WAYLAND_DISPLAY -u DISPLAY)
# The headless compositor presents no GPU buffers, and Glutter's `auto` tries Vulkan first, so the run asks for GLES.
# GLUTTER_RENDERER in the environment of the caller wins.
run+=(GLUTTER_RENDERER="${GLUTTER_RENDERER:-gles}")
[[ -n "$after" ]] && run+=(GLUTTER_SCREENSHOT_AFTER="$after")

set +e
timeout -k 5 "${PROOF_TIMEOUT:-90}" "${run[@]}" GLUTTER_SCREENSHOT="$out" "$root/scripts/isolated.sh" -- \
  "$root/scripts/headless-wayland.sh" -- "$tmp/passbank-gui" "${args[@]}"
status=$?
set -e

if [[ ! -s "$out" ]]; then
  echo "error: no image was written to $out (exit status $status)" >&2
  exit 1
fi
if ((status != 0)); then
  echo "warning: the run ended with status $status, but the image was written" >&2
fi
echo "proof: wrote $out"
