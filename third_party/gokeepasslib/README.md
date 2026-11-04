# gokeepasslib

Test databases from https://github.com/tobischo/gokeepasslib (MIT, commit 7b40897fc44c9b7cbbb87ca681582fb1c4a8a85c),
`tests/kdbx3/` and `tests/kdbx4/` (two of the latter are KDBX 3.1 files in spite of the folder). Passbank's tests read
them and never change them; they are copied to `internal/core/kdbx/testdata/ext/` as `go-*`, with the names and
passwords in `testdata/FIXTURES.md`.

- `tests/kdbx3/example.kdbx` → `go-example.kdbx`
- `tests/kdbx3/example-key.kdbx`, `example-key.key` → `go-example-key.kdbx`, `go-example-key.key`
- `tests/kdbx3/protected-binary.kdbx` → `go-protected-binary.kdbx` (made with kdbxweb 1.14.4)
- `tests/kdbx4/example-chacha.kdbx` → `go-chacha.kdbx` (KDBX 3.1 with the ChaCha20 outer cipher)
- `tests/kdbx4/example-twofish.kdbx` → `go-twofish.kdbx` (KDBX 3.1 with Twofish)
