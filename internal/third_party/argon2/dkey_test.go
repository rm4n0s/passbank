package argon2

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestDKey checks the one function Passbank added: it must give the Argon2d
// results of the reference implementation (the Argon2d entries of testVectors),
// and no other variant's.
func TestDKey(t *testing.T) {
	password, salt := []byte("password"), []byte("somesalt")
	checked := 0
	for i, v := range testVectors {
		if v.mode != argon2d {
			continue
		}
		want, err := hex.DecodeString(v.hash)
		if err != nil {
			t.Fatalf("vector %d: %v", i, err)
		}
		if got := DKey(password, salt, v.time, v.memory, v.threads, uint32(len(want))); !bytes.Equal(got, want) {
			t.Errorf("vector %d: DKey = %x, want %x", i, got, want)
		}
		if bytes.Equal(IDKey(password, salt, v.time, v.memory, v.threads, uint32(len(want))), want) ||
			bytes.Equal(Key(password, salt, v.time, v.memory, v.threads, uint32(len(want))), want) {
			t.Errorf("vector %d: Argon2id or Argon2i gives the Argon2d result", i)
		}
		checked++
	}
	if checked < 3 {
		t.Fatalf("only %d Argon2d vectors checked", checked)
	}
}
