package model

import (
	"encoding/hex"
	"strings"

	"github.com/rm4n0s/errors"
)

// UUID is the 16-byte identifier of an entry, a group, an icon or a deleted
// object. The zero UUID means "none".
type UUID [16]byte

// IsZero reports whether u is the zero UUID.
func (u UUID) IsZero() bool { return u == UUID{} }

// String formats u in the canonical form, "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
// in lower case.
func (u UUID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:36], u[10:16])
	return string(b[:])
}

// ParseUUID parses the canonical form of String, in either case.
func ParseUUID(s string) (UUID, error) {
	var u UUID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, errors.New("UUIDInvalid", "a UUID looks like xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", "value", s)
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil {
		return u, errors.NewErr("UUIDInvalid", err).SetMetadata("value", s)
	}
	copy(u[:], raw)
	return u, nil
}
