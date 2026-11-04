# keepass-rs

Test databases from https://github.com/sseemayer/keepass-rs (MIT, commit 26cb076aa5cbbe36e175326dfdaf5a19799c4085),
`tests/resources/`. Passbank's tests read them and never change them; they are copied to
`internal/core/kdbx/testdata/ext/` as `rs-*`, with the names and passwords in `testdata/FIXTURES.md`.

- `test_db_with_password.kdbx` → `rs-password.kdbx` (written by KeePass 2, KDBX 3.1)
- `test_db_kdbx3_with_attachments.kdbx` → `rs-attachments.kdbx` (KeePassXC 2.7, KDBX 3.1)
- `test_db_kdbx3_with_chacha20_protected_fields.kdbx` → `rs-chacha20-inner.kdbx`
- `test_db_with_keyfile.kdbx`, `test_key.key` → `rs-keyfile.kdbx`, `rs-keyfile.key`
- `test_db_with_keyfile_xml.kdbx`, `test_key_xml.key` → `rs-keyfile-xml.kdbx`, `rs-keyfile-xml.key`
