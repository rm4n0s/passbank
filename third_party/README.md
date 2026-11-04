# third_party

This folder holds the licenses of the files that Passbank bundles or copies
from other projects: fonts, a copy of a library, and test files from other
KeePass implementations.

- One folder per project, named after it (for example `x-crypto/`), with the
  project's `LICENSE` file and a `README.md` line saying where the files came
  from (repository, commit or version) and which files Passbank uses.
- `NOTICE` in the repository root lists every project and the step that added
  it.
- Do not put code here that Passbank imports. A copy of a library that Passbank
  compiles lives under `internal/third_party/`, with its license kept both
  there and here.
