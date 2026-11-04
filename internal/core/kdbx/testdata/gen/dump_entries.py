"""Writes dumps/<name>.txt: one line per entry of a KDBX 3.1 fixture, made with pykeepass (independent of Passbank).

Usage: python -I dump_entries.py <venv with pykeepass> ... is not needed; run it with the python of a virtualenv
that has pykeepass installed, from the testdata folder:  python -I gen/dump_entries.py

The line is: group path | title | user name | password | URL | notes | tags | custom fields (key=value, sorted by key) |
attachments (name:sha256 of the content, sorted) | number of history items. Entries are sorted by UUID.
pykeepass cannot read ext/go-protected-binary.kdbx (it does not decrypt a protected binary of Meta/Binaries, which
shifts the stream for every later value) and ext/rs-chacha20-inner.kdbx (ChaCha20 inner stream), so those two have
tests with known values instead. The Go test (open3_test.go) writes the same lines from Passbank's model and compares.
"""
import hashlib
import os
import sys

from pykeepass import PyKeePass

CASES = [
    ("k31-empty.kdbx", "fixture-min", None),
    ("k31-rich.kdbx", "fixture-rich", None),
    ("ext/rs-password.kdbx", "demopass", None),
    ("ext/rs-attachments.kdbx", "demopass", None),
    ("ext/rs-keyfile.kdbx", None, "ext/rs-keyfile.key"),
    ("ext/rs-keyfile-xml.kdbx", None, "ext/rs-keyfile-xml.key"),
    ("ext/go-example.kdbx", "abcdefg12345678", None),
    ("ext/go-example-key.kdbx", "abcdefg12345678", "ext/go-example-key.key"),
    ("ext/go-chacha.kdbx", "abcdefg12345678", None),
    ("ext/go-twofish.kdbx", "test1234test", None),
    ("ext/web-demo.kdbx", "demo", "ext/web-demo.key"),
    ("ext/web-cyrillic.kdbx", "пароль", None),
]


def clean(s):
    return (s or "").replace("\\", "\\\\").replace("|", "\\|").replace("\n", "\\n").replace("\r", "\\r")


def main():
    os.makedirs("dumps", exist_ok=True)
    for name, password, keyfile in CASES:
        kp = PyKeePass(name, password=password, keyfile=keyfile)
        lines = []
        for e in kp.entries:
            path = "/".join(e.parentgroup.path) if e.parentgroup.path else ""
            custom = ";".join(f"{k}={clean(v)}" for k, v in sorted(e.custom_properties.items()))
            atts = ",".join(sorted(f"{a.filename}:{hashlib.sha256(a.data).hexdigest()}" for a in e.attachments))
            lines.append((e.uuid.hex, "|".join([
                clean(path), clean(e.title), clean(e.username), clean(e.password), clean(e.url), clean(e.notes),
                ",".join(e.tags or []), custom, atts, str(len(e.history))])))
        lines.sort()
        out = os.path.join("dumps", name.replace("/", "-").replace(".kdbx", ".txt"))
        with open(out, "w", encoding="utf-8") as f:
            for uid, line in lines:
                f.write(uid + "|" + line + "\n")
        print(out, len(lines))


if __name__ == "__main__":
    sys.exit(main())
