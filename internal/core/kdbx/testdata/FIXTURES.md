# KDBX fixtures

Real KDBX files for the tests of `internal/core/kdbx` and everything above it. All of them are made by
`make-fixtures.sh`, which says how, and none is made by Passbank. The passwords and key files are test
values. A test never opens a file that is not listed here (`fixtures_test.go` checks that the list and
the folder agree).

**Made by:** *KeePassXC* is `keepassxc-cli` 2.7.12 (the Flatpak). From its command line KeePassXC writes
KDBX 3.1 only, so the KDBX 3.1 files and the key files come from it. *pykeepass* is pykeepass 4.2.0, a
Python implementation of KDBX that is independent of Passbank and of KeePassXC; it makes the KDBX 4 files.
`make-fixtures.sh` opens every file with KeePassXC after making it, so KeePassXC reads what pykeepass
wrote. No file here was written by KeePassXC's own KDBX 4 writer or by KeePass 2; see
`docs/MANUAL-CHECKS.md`. KDBX 2.x and 3.0 files are not here (SPEC §19, A10).

The KDF parameters are small so that tests run fast: Argon2 with 8 MiB, 2 lanes and 2 passes, AES-KDF with
6000 rounds (pykeepass) or about 100 ms of rounds (KeePassXC).

## Databases

| File | Version | Made by | Cipher | KDF | Compression | Unlocked with | Content |
|---|---|---|---|---|---|---|---|
| `k31-empty.kdbx` | 3.1 | KeePassXC | AES-256 | AES-KDF | GZip | password `fixture-min` | empty |
| `k31-rich.kdbx` | 3.1 | KeePassXC | AES-256 | AES-KDF | GZip | password `fixture-rich` | rich |
| `k31-keyfile.kdbx` | 3.1 | KeePassXC | AES-256 | AES-KDF | GZip | `keyfile-k31.keyx` only | empty |
| `k31-password-and-keyfile.kdbx` | 3.1 | KeePassXC | AES-256 | AES-KDF | GZip | password `fixture-rich` and `keyfile-xml2.keyx` | empty |
| `k4-empty.kdbx` | 4.0 | pykeepass | AES-256 | Argon2d | GZip | password `fixture-min` | empty |
| `k4-rich.kdbx` | 4.0 | pykeepass | AES-256 | Argon2d | GZip | password `fixture-rich` | rich |
| `k4-rich-argon2id.kdbx` | 4.0 | pykeepass | AES-256 | Argon2id | GZip | password `fixture-rich` | rich |
| `k4-rich-chacha20.kdbx` | 4.0 | pykeepass | ChaCha20 | Argon2d | GZip | password `fixture-rich` | rich |
| `k4-rich-twofish-aeskdf.kdbx` | 4.0 | pykeepass | Twofish | AES-KDF | GZip | password `fixture-rich` | rich |
| `k4-rich-nocompression.kdbx` | 4.0 | pykeepass | AES-256 | Argon2d | none | password `fixture-rich` | rich |
| `k4-keyfile-xml2.kdbx` | 4.0 | pykeepass | AES-256 | Argon2d | GZip | `keyfile-xml2.keyx` only | rich |
| `k4-password-and-keyfile.kdbx` | 4.0 | pykeepass | AES-256 | Argon2d | GZip | password `fixture-rich` and `keyfile-raw32.key` | rich |
| `k4-public-custom-data.kdbx` | 4.0 | pykeepass | AES-256 | Argon2d | GZip | password `fixture-rich` | rich; header field 12 (public custom data) holds `fixture` = UInt32 42 |
| `k4-unknown-xml.kdbx` | 4.0 | pykeepass | AES-256 | Argon2d | GZip | password `fixture-rich` | rich, plus XML elements that no KeePass version defines (below) |
| `k41-fields.kdbx` | 4.1 | pykeepass | AES-256 | Argon2d | GZip | password `fixture-rich` | rich, plus the elements that only KDBX 4.1 has (below) |

"Empty" is the empty database the program makes: a root group, and for KeePassXC a recycle bin setting; no entries.

## Key files

| File | Format | Made by | Key |
|---|---|---|---|
| `keyfile-xml2.keyx` | XML 2.0, hex data with a `Hash` | KeePassXC | random, 32 bytes |
| `keyfile-k31.keyx` | XML 2.0, hex data with a `Hash` | KeePassXC | random, 32 bytes |
| `keyfile-raw32.key` | raw 32 bytes (`00 01 … 1f`) | `make-fixtures.sh` | the bytes themselves |

The other formats of SPEC §4.2 (XML 1.0, 64 hexadecimal digits, any other file) are covered by the unit tests
of `internal/core/kdbx/crypto`, whose expected keys were computed with Python.

## The rich content

`rich.xml` is the XML of the rich databases as it was given to the writers (`gen/rich.py` makes it; protected
values are plain text there and encrypted in the files). KeePassXC's import (`k31-rich`) keeps what it knows;
the KDBX 4 files hold the XML as written. What the tests may rely on:

- **Meta**: name "Rich fixture", description "A database that uses most features", default user name `ada`,
  history of 365 days, color `#336699`, only the password is protected in memory, the recycle bin is enabled
  (its group is `00000000-0000-0000-0000-000000000103`), at most 10 history items and 6291456 bytes per
  entry, one custom icon (`…0301`, 24 bytes that are not a real PNG), one custom data item
  `fixture-key` = `fixture-value`.
- **Groups**: `Root` (`…0101`) with the entries Bank and Ünïcödé; `Email` (`…0102`, not expanded, auto-type off,
  searching on, default sequence `{PASSWORD}`) with the entry Mail and the empty group `Nested` (`…0104`);
  `Recycle Bin` (`…0103`) with the entry Old forum.
- **Entries**: Bank (`…0201`; user `ada`, protected password `bank-password-1`, protected custom field `PIN` =
  `4711`, plain custom field `Account number`, tags `money` and `work`, colors `#112233` / `#445566`, override
  URL `firefox %u`, the custom icon, the attachment `statement.txt`, two history versions: the password was
  `older-bank-password`, then `old-bank-password`); Ünïcödé ключ 鍵 (`…0204`; user `用户`, password
  `pässwörd-日本語`); Mail (`…0202`; `ada@example.test`, `mail-password-1`); Old forum (`…0203`; `ada`,
  `forum-password-1`).
- **Attachment**: `statement.txt`, the text `attachment-bytes-for-the-fixture` and a newline, three times.
- **Deleted objects**: one, `…0401`.
- KeePassXC (`k31-rich`) may write its own `Generator`, times and IDs of what it does not keep; tests of that
  file check the facts above, not every byte.

## `k41-fields.kdbx`

The rich content plus the KDBX 4.1 elements: `Tags` and `PreviousParentGroup` on the root group (`team;home`,
`…0103`), `QualityCheck` = False and `PreviousParentGroup` (`…0102`) on Bank, `Name` ("fixture icon") and
`LastModificationTime` on the custom icon, `SettingsChanged` in Meta and `LastModificationTime` on the custom
data item. The header says 4.1. pykeepass does not know these elements; it keeps them as they are, so the file
tests how Passbank reads the XML of 4.1, not how a 4.1 program writes it.

## `k4-unknown-xml.kdbx`

The rich content plus elements that no KeePass version defines, which Passbank must keep (SPEC §4.4):
`X-Plugin-meta` in Meta, `X-Plugin-group` in the root group, `X-Plugin-entry` in Bank (each with an attribute
`version="2"`, a `Token` element with a `Protected="True"` attribute and the text `plugin-secret-<place>`, a
`Visible` element and a nested `Nested/Deep`), and `X-TopLevel` (text `kept`, one child `Child a="1"`) after
`Root` in the file. The `Token` is **not** a `Value` element, so KDBX does not protect it: the file holds its
text as it is, and a reader must not decrypt it.
