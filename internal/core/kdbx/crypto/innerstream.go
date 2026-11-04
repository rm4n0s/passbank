package crypto

import (
	"crypto/sha256"
	"crypto/sha512"

	"github.com/rm4n0s/errors"
	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/salsa20/salsa" //nolint:staticcheck // KDBX uses Salsa20 with an 8-byte nonce, which only this package gives
)

// The inner stream IDs of a KDBX file: the stream that protects the values marked
// Protected in the XML (SPEC §4.1).
const (
	// InnerNone protects nothing.
	InnerNone = 0
	// InnerArcFour is the ArcFourVariant stream, which Passbank rejects (SPEC §19, A9).
	InnerArcFour = 1
	// InnerSalsa20 is the stream of KDBX 3 files.
	InnerSalsa20 = 2
	// InnerChaCha20 is the stream of KDBX 4 files.
	InnerChaCha20 = 3
)

// InnerStream is the keystream that protects values: one continuous stream across
// all the protected values of a file, in the order they appear in the XML. Reading
// and writing XOR with it, so the same call encrypts and decrypts.
type InnerStream interface {
	// XORKeyStream XORs src with the next len(src) bytes of the keystream into dst,
	// which may be src.
	XORKeyStream(dst, src []byte)
}

// NewInnerStream makes the inner stream with the ID from the inner stream key of
// the file. The key is not secret-sized: it is hashed (SHA-256 for Salsa20,
// SHA-512 for ChaCha20), so any non-empty length is fine.
func NewInnerStream(id uint32, key []byte) (InnerStream, error) {
	if id != InnerNone && len(key) == 0 {
		return nil, errors.New("InnerStreamKeyInvalid", "the key of the inner stream is empty", "stream", id)
	}
	switch id {
	case InnerNone:
		return noStream{}, nil
	case InnerSalsa20:
		return newSalsa20Stream(key), nil
	case InnerChaCha20:
		sum := sha512.Sum512(key)
		defer clear(sum[:])
		s, err := chacha20.NewUnauthenticatedCipher(sum[:32], sum[32:44])
		if err != nil {
			return nil, errors.NewErr("CipherInitFailed", err).SetMetadata("cipher", "ChaCha20")
		}
		return s, nil
	case InnerArcFour:
		return nil, errors.New("InnerStreamUnsupported", "the ArcFourVariant inner stream is not supported", "stream", id)
	}
	return nil, errors.New("InnerStreamUnsupported", "the file uses an inner stream that Passbank does not know", "stream", id)
}

// noStream leaves the data as it is.
type noStream struct{}

func (noStream) XORKeyStream(dst, src []byte) { copy(dst, src) }

// salsa20IV is the fixed nonce of KeePass's Salsa20 inner stream.
var salsa20IV = [8]byte{0xE8, 0x30, 0x09, 0x4B, 0x97, 0x20, 0x5D, 0x2A}

// salsa20Stream is Salsa20/20 that can be asked for any number of bytes at a
// time: golang.org/x/crypto/salsa20 only encrypts a whole message from block 0.
type salsa20Stream struct {
	key     [32]byte
	counter [16]byte // nonce (8 bytes), then the block number as a little-endian uint64
	block   [64]byte // the current block of keystream
	used    int      // how many bytes of block are used; 64 means none left
}

func newSalsa20Stream(key []byte) *salsa20Stream {
	s := &salsa20Stream{key: sha256.Sum256(key), used: 64}
	copy(s.counter[:8], salsa20IV[:])
	return s
}

func (s *salsa20Stream) XORKeyStream(dst, src []byte) {
	for len(src) > 0 {
		if s.used == len(s.block) {
			s.nextBlock()
		}
		n := min(len(src), len(s.block)-s.used)
		for i := range n {
			dst[i] = src[i] ^ s.block[s.used+i]
		}
		s.used += n
		dst, src = dst[n:], src[n:]
	}
}

// nextBlock makes the keystream block whose number follows the last one.
func (s *salsa20Stream) nextBlock() {
	var zero [64]byte
	c := s.counter // the library may advance its copy of the counter; this one is ours
	salsa.XORKeyStream(s.block[:], zero[:], &c, &s.key)
	// Advance the block number, a little-endian uint64 in bytes 8 to 15.
	for i := 8; i < 16; i++ {
		s.counter[i]++
		if s.counter[i] != 0 {
			break
		}
	}
	s.used = 0
}
