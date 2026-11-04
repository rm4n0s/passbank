# Error Tags without a route test

CLAUDE.md requires a route test for every error Tag. This file is the only exception list:
`scripts/error-tags.sh --untested` skips the Tags named here. A Tag is listed only when no test
can reach it, with the reason. Remove the entry when the code changes so that a test can.

- `CipherInitFailed`: the constructors of AES, Twofish and ChaCha20 fail only for a key or nonce of the wrong length, and every call checks the lengths first (`CipherKeyInvalid`, `KdfParametersRejected`, `InnerStreamKeyInvalid`), so the branch is a guard against a future change and cannot be reached.
