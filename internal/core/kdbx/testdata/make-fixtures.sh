#!/usr/bin/env bash
# Makes the KDBX fixtures of this folder (see FIXTURES.md). It is not run by any test: the fixtures are
# committed, and this script is how they were made and how to make them again (the seeds are random, so
# the bytes differ every time; the content does not).
#
#   internal/core/kdbx/testdata/make-fixtures.sh [<output folder>]
#
# Two programs make the files, so that no fixture comes from Passbank itself:
#   * KeePassXC (keepassxc-cli, or the Flatpak org.keepassxc.KeePassXC) makes the KDBX 3.1 files and the key
#     files; KeePassXC 2.7 writes KDBX 3.1 from its command line.
#   * pykeepass 4.2.0 (a Python implementation of KDBX, installed into a temporary virtualenv from PyPI)
#     makes the KDBX 4 files. KeePassXC reads every one of them: this script checks that.
# Everything runs in a temporary folder with its own HOME and XDG folders; nothing of the user's is read.
# Needs python3 with venv and pip, network access for pip, and keepassxc-cli or Flatpak KeePassXC.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
out="${1:-$here}"
mkdir -p "$out"
out="$(cd "$out" && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/passbank-fixtures.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
export HOME="$tmp/home" XDG_CONFIG_HOME="$tmp/config" XDG_DATA_HOME="$tmp/data" XDG_CACHE_HOME="$tmp/cache"
mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"

work="$tmp/work"
mkdir -p "$work"

# kcli runs keepassxc-cli; the Flatpak gets access to the work folder only.
if command -v keepassxc-cli >/dev/null 2>&1; then
  kcli() { keepassxc-cli "$@"; }
elif command -v flatpak >/dev/null 2>&1 && flatpak info org.keepassxc.KeePassXC >/dev/null 2>&1; then
  kcli() { flatpak run --filesystem="$work" --command=keepassxc-cli org.keepassxc.KeePassXC "$@"; }
else
  echo "error: neither keepassxc-cli nor the Flatpak org.keepassxc.KeePassXC is installed" >&2
  exit 2
fi
echo "keepassxc-cli $(kcli --version)"

python3 -m venv "$tmp/venv"
"$tmp/venv/bin/pip" install --quiet pykeepass==4.2.0
py="$tmp/venv/bin/python"
echo "pykeepass $("$py" -c 'import pykeepass.version as v; print(v.__version__)' 2>/dev/null || echo 4.2.0)"

# The content: one XML per style, from rich.py.
python3 "$here/gen/rich.py" --protect memory --binaries meta >"$work/rich-memory.xml"
python3 "$here/gen/rich.py" --protect protected --binaries inner >"$work/rich4.xml"
python3 "$here/gen/rich.py" --protect protected --binaries inner --four-one >"$work/rich41.xml"
python3 "$here/gen/rich.py" --protect protected --binaries inner --unknown >"$work/rich-unknown.xml"

# --- KeePassXC: KDBX 3.1 and the key files -----------------------------------------------------------------
echo "key files and KDBX 3.1 (KeePassXC)"
printf 'x\nx\n' | kcli db-create -q -p --set-key-file "$work/keyfile-xml2.keyx" "$work/scratch.kdbx" >/dev/null
rm -f "$work/scratch.kdbx"
python3 -c "import sys; open(sys.argv[1], 'wb').write(bytes(range(32)))" "$work/keyfile-raw32.key"

printf 'fixture-min\nfixture-min\n' | kcli db-create -q -p -t 100 "$work/k31-empty.kdbx" >/dev/null
printf 'fixture-rich\nfixture-rich\n' | kcli import -q -p -t 100 "$work/rich-memory.xml" "$work/k31-rich.kdbx" >/dev/null
kcli db-create -q -t 100 --set-key-file "$work/keyfile-k31.keyx" "$work/k31-keyfile.kdbx" >/dev/null </dev/null
printf 'fixture-rich\nfixture-rich\n' | kcli db-create -q -p -t 100 --set-key-file "$work/keyfile-xml2.keyx" "$work/k31-password-and-keyfile.kdbx" >/dev/null

# --- pykeepass: KDBX 4 ---------------------------------------------------------------------------------------
echo "KDBX 4 (pykeepass)"
mk() { "$py" "$here/gen/make_kdbx4.py" "$@"; }
mk "$work/k4-empty.kdbx" --password fixture-min
mk "$work/k4-rich.kdbx" --password fixture-rich --content "$work/rich4.xml"
mk "$work/k4-rich-argon2id.kdbx" --password fixture-rich --content "$work/rich4.xml" --kdf argon2id
mk "$work/k4-rich-chacha20.kdbx" --password fixture-rich --content "$work/rich4.xml" --cipher chacha20
mk "$work/k4-rich-twofish-aeskdf.kdbx" --password fixture-rich --content "$work/rich4.xml" --cipher twofish --kdf aeskdf
mk "$work/k4-rich-nocompression.kdbx" --password fixture-rich --content "$work/rich4.xml" --no-compression
mk "$work/k4-keyfile-xml2.kdbx" --keyfile "$work/keyfile-xml2.keyx" --content "$work/rich4.xml"
mk "$work/k4-password-and-keyfile.kdbx" --password fixture-rich --keyfile "$work/keyfile-raw32.key" --content "$work/rich4.xml"
mk "$work/k4-public-custom-data.kdbx" --password fixture-rich --content "$work/rich4.xml" --public-custom-data
mk "$work/k41-fields.kdbx" --password fixture-rich --content "$work/rich41.xml" --minor 1
mk "$work/k4-unknown-xml.kdbx" --password fixture-rich --content "$work/rich-unknown.xml"

# --- KeePassXC must read what pykeepass wrote, and what it made itself ------------------------------------
echo "checking every fixture with KeePassXC"
check() { # check <file> <text the listing must contain> <password or ""> [key file]
  local f="$1" want="$2" pw="$3" kf="${4:-}" args=() list
  [[ -n "$kf" ]] && args+=(-k "$kf")
  [[ -z "$pw" ]] && args+=(--no-password)
  if [[ -n "$pw" ]]; then
    list="$(printf '%s\n' "$pw" | kcli ls -q -R "${args[@]}" "$f" 2>&1)"
  else
    list="$(kcli ls -q -R "${args[@]}" "$f" </dev/null 2>&1)"
  fi
  if ! grep -q -F "$want" <<<"$list"; then
    echo "error: KeePassXC cannot read $f (wanted \"$want\" in the listing):" >&2
    echo "$list" >&2
    exit 1
  fi
  echo "  $(basename "$f"): opens"
}
check "$work/k31-empty.kdbx" "[empty]" fixture-min
check "$work/k31-rich.kdbx" "Old forum" fixture-rich
check "$work/k31-keyfile.kdbx" "[empty]" "" "$work/keyfile-k31.keyx"
check "$work/k31-password-and-keyfile.kdbx" "[empty]" fixture-rich "$work/keyfile-xml2.keyx"
check "$work/k4-empty.kdbx" "[empty]" fixture-min
for f in k4-rich k4-rich-argon2id k4-rich-chacha20 k4-rich-twofish-aeskdf k4-rich-nocompression k4-public-custom-data k41-fields k4-unknown-xml; do
  check "$work/$f.kdbx" "Old forum" fixture-rich
done
check "$work/k4-keyfile-xml2.kdbx" "Old forum" "" "$work/keyfile-xml2.keyx"
check "$work/k4-password-and-keyfile.kdbx" "Old forum" fixture-rich "$work/keyfile-raw32.key"

# The rich XML is kept too: it is the description of the content that FIXTURES.md refers to.
rm -f "$out"/*.kdbx "$out"/keyfile-* # a rerun replaces the files, which KeePassXC made read-only
cp "$work"/*.kdbx "$work"/keyfile-* "$out/"
cp "$work/rich4.xml" "$out/rich.xml"
chmod 644 "$out"/*.kdbx "$out"/keyfile-* "$out/rich.xml" # KeePassXC writes them readable by the owner only
echo "wrote $(ls "$out"/*.kdbx | wc -l) databases and $(ls "$out"/keyfile-* | wc -l) key files to $out"
