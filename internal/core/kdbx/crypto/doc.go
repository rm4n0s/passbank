// Package crypto holds the ciphers, key-derivation functions and inner streams of
// KDBX (SPEC §4.1) and, from step 2.3, the composite key and key files (§4.2).
//
// It uses the standard library and golang.org/x/crypto, and a copy of its
// argon2 package that exports Argon2d (SPEC §19, A12). It implements no
// primitive itself: every function here arranges the primitives the way the
// KDBX format says.
package crypto
