// Package secret holds secret byte buffers that are wiped after use and never
// printed (SPEC §14).
//
// Passwords, keys and protected field values live in a Bytes. A Bytes cannot be
// printed, logged or marshalled by accident: every formatting verb, slog and
// encoding/text give "[secret]". The bytes are reached only through Reveal, and
// the code that calls it keeps the slice no longer than it needs.
//
// Go gives no guarantee that a secret never reaches swap, a core dump or a copy
// the runtime made while growing a slice. Wipe zeroes the buffer Passbank holds;
// it cannot reach copies Passbank did not make.
package secret
