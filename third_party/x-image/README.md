# golang.org/x/image

The Go fonts (`golang.org/x/image/font/gofont/*`), embedded by
`internal/gui/kit/fonts` (step 1.4). They were made by the Bigelow & Holmes
foundry for the Go project and are licensed under the same license as the rest of
Go (`LICENSE` here, BSD-3-Clause). Passbank uses the regular, bold, italic and
bold italic faces of Go and Go Mono. The module is a dependency, not a copy:
`go.mod` pins the version.
