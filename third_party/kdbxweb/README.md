# kdbxweb

Test databases from https://github.com/keeweb/kdbxweb (MIT, commit 7601e45d2a437f35a7fa3d767164eab60bc05d42),
`resources/`. Passbank's tests read them and never change them; they are copied to
`internal/core/kdbx/testdata/ext/` as `web-*`, with the names and passwords in `testdata/FIXTURES.md`.

- `demo.kdbx`, `demo.key` → `web-demo.kdbx`, `web-demo.key`
- `cyrillic.kdbx` → `web-cyrillic.kdbx` (not compressed)
