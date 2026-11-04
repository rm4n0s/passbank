# golang.org/x/crypto

The module is a dependency (`go.mod` pins the version): Passbank uses its
`chacha20`, `salsa20`, `twofish` and `blake2b` packages as they are.

`internal/third_party/argon2` is a **copy** of `golang.org/x/crypto/argon2` at
v0.58.0, with one change: `DKey` exports Argon2d, which the upstream package
computes inside but does not export. KeePassXC's default key-derivation function
is Argon2d (SPEC §19, A12). `LICENSE` and `PATENTS` here are the module's. The
copy keeps the upstream tests, which include the RFC 9106 test vectors of Argon2d,
Argon2i and Argon2id.
