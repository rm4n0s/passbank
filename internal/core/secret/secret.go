package secret

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
)

// redacted is what a Bytes prints as, in every format.
const redacted = "[secret]"

// Bytes is a secret byte string. The zero value is an empty secret. Copies of a
// Bytes share one buffer: wiping any of them wipes all of them. Use Clone for an
// independent copy.
//
// The buffer is reached through a func because of how fmt prints a struct field
// it cannot call methods on (an unexported one): it prints a pointer to a struct
// or a slice by its contents, even with a verb that does not apply, but it
// prints a func by its address. A pointer to a struct here would leak the bytes
// through "%s" or "%q" of any struct that holds a Bytes in an unexported field.
// The func makes Bytes not comparable with ==, which is intended: use Equal.
type Bytes struct {
	buf func() *[]byte
}

// New returns a Bytes that holds b. It takes ownership of b: the caller must not
// use or keep the slice afterwards.
func New(b []byte) Bytes {
	p := &b
	return Bytes{buf: func() *[]byte { return p }}
}

// Len returns the number of bytes in the secret.
func (s Bytes) Len() int {
	return len(s.Reveal())
}

// Reveal returns the secret's bytes, not a copy. The caller must not modify the
// slice or keep it after the next Wipe.
func (s Bytes) Reveal() []byte {
	if s.buf == nil {
		return nil
	}
	return *s.buf()
}

// Clone returns a Bytes with its own copy of the bytes, which Wipe of the
// original does not touch.
func (s Bytes) Clone() Bytes {
	if s.buf == nil {
		return Bytes{}
	}
	return New(append([]byte(nil), s.Reveal()...))
}

// Wipe overwrites the bytes with zeros and empties the secret, for this Bytes
// and every copy of it.
func (s Bytes) Wipe() {
	if s.buf == nil {
		return
	}
	p := s.buf()
	clear(*p)
	*p = nil
}

// Equal reports whether s and other hold the same bytes, in time that does not
// depend on where they first differ. The lengths are not secret.
func (s Bytes) Equal(other Bytes) bool {
	return subtle.ConstantTimeCompare(s.Reveal(), other.Reveal()) == 1
}

// String returns "[secret]", so that a Bytes in a message or a log line shows
// nothing.
func (s Bytes) String() string { return redacted }

// Format prints "[secret]" for every verb, including %#v, %x and %q.
func (s Bytes) Format(f fmt.State, _ rune) {
	// A formatter has no way to report a failed write, and there is nothing else
	// to do with it.
	_, _ = f.Write([]byte(redacted))
}

// LogValue makes slog print "[secret]".
func (s Bytes) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalText makes encoding/json and other encoders write "[secret]" instead
// of the bytes.
func (s Bytes) MarshalText() ([]byte, error) { return []byte(redacted), nil }
