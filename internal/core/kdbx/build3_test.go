package kdbx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// This file is a builder of KDBX 2.x and 3.x files for tests (SPEC §4.5), built
// from the format and the primitives of the crypto package, apart from the
// writer of M5. The reader is checked against real files by the fixtures; this
// makes the files no fixture is: other versions, damage at one layer, odd headers.

// build3 describes a file to build. The zero value is a valid KDBX 3.1 file.
type build3 struct {
	// Major and Minor are the version written (3.1 when both are 0).
	Major, Minor uint16
	// Password of the file.
	Password string
	// Cipher is the outer cipher (AES-256 when zero).
	Cipher model.UUID
	// Compression of the payload.
	Compression Compression
	// Rounds of the AES-KDF (10 when 0).
	Rounds uint64
	// InnerStream is the inner stream ID (Salsa20 when 0 and NoInnerStream is false).
	InnerStream uint32
	// Skip lists header field IDs to leave out; Extra are further header fields
	// written before the end field.
	Skip  []byte
	Extra []HeaderField
	// Override replaces the data of known header fields.
	Override map[byte][]byte
	// StartBytes overrides the stream start bytes written in the payload.
	StartBytes []byte
	// XML is the payload (a small database when empty).
	XML []byte
	// BlockSize is the size of the blocks of the stream (1 MiB when 0).
	BlockSize int
	// Blocks, when not nil, replaces the hashed block stream (it is then not built
	// from the XML); the start bytes still come first.
	Blocks []byte
	// RawPayload, when not nil, is encrypted as the whole payload (start bytes
	// included), for a payload without the stream.
	RawPayload []byte
}

// header3Field encodes a field of the old header: ID, two bytes of length, data.
func header3Field(id byte, data []byte) []byte {
	return append(append([]byte{id}, byte(len(data)), byte(len(data)>>8)), data...)
}

// hashedBlocks builds a hashed block stream of KDBX 3.
func hashedBlocks(data []byte, size int) []byte {
	var out []byte
	for index := uint32(0); ; index++ {
		n := min(size, len(data))
		block := data[:n]
		data = data[n:]
		out = binary.LittleEndian.AppendUint32(out, index)
		if n == 0 {
			out = append(out, make([]byte, 32)...)
		} else {
			sum := sha256.Sum256(block)
			out = append(out, sum[:]...)
		}
		out = binary.LittleEndian.AppendUint32(out, uint32(n))
		out = append(out, block...)
		if n == 0 {
			return out
		}
	}
}

// build makes the file.
func (b build3) build(t testing.TB) []byte {
	t.Helper()
	major, minor := b.Major, b.Minor
	if major == 0 {
		major, minor = 3, 1
	}
	cipherID := b.Cipher
	if cipherID.IsZero() {
		cipherID = crypto.CipherAES256
	}
	cipher, err := crypto.LookupCipher(cipherID)
	if err != nil {
		t.Fatal(err)
	}
	rounds := b.Rounds
	if rounds == 0 {
		rounds = 10
	}
	inner := b.InnerStream
	if inner == 0 {
		inner = crypto.InnerSalsa20
	}
	masterSeed := bytes.Repeat([]byte{0x11}, 32)
	transformSeed := bytes.Repeat([]byte{0x22}, 32)
	iv := bytes.Repeat([]byte{0x33}, cipher.IVSize())
	streamKey := bytes.Repeat([]byte{0x44}, 32)
	start := bytes.Repeat([]byte{0x55}, 32)

	fields := map[byte][]byte{
		fieldCipherID:           cipherID[:],
		fieldCompression:        u32(uint32(b.Compression)),
		fieldMasterSeed:         masterSeed,
		fieldTransformSeed:      transformSeed,
		fieldTransformRounds:    u64(rounds),
		fieldEncryptionIV:       iv,
		fieldProtectedStreamKey: streamKey,
		fieldStreamStartBytes:   start,
		fieldInnerRandomStream:  u32(inner),
	}
	for id, data := range b.Override {
		fields[id] = data
	}
	header := binary.LittleEndian.AppendUint32(nil, signature1)
	header = binary.LittleEndian.AppendUint32(header, signature2KDBX)
	header = append(header, byte(minor), byte(minor>>8), byte(major), byte(major>>8))
	for _, id := range []byte{fieldCipherID, fieldCompression, fieldMasterSeed, fieldTransformSeed, fieldTransformRounds, fieldEncryptionIV, fieldProtectedStreamKey, fieldStreamStartBytes, fieldInnerRandomStream} {
		if !bytes.Contains(b.Skip, []byte{id}) {
			header = append(header, header3Field(id, fields[id])...)
		}
	}
	for _, f := range b.Extra {
		header = append(header, header3Field(f.ID, f.Data)...)
	}
	header = append(header, header3Field(fieldEnd, []byte("\r\n\r\n"))...)

	composite, err := crypto.CompositeKey(crypto.Credentials{HasPassword: true, Password: secretOf(b.Password)})
	if err != nil {
		t.Fatal(err)
	}
	kdf := crypto.KDF{ID: crypto.KDFAES, Seed: transformSeed, Rounds: rounds}
	transformed, err := kdf.Derive(context.Background(), composite.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	keyHash := sha256.New()
	keyHash.Write(masterSeed)
	keyHash.Write(transformed)

	payload := b.RawPayload
	if payload == nil {
		startWritten := b.StartBytes
		if startWritten == nil {
			startWritten = start
		}
		blocks := b.Blocks
		if blocks == nil {
			xml := b.XML
			if xml == nil {
				xml = []byte(buildXML)
			}
			if b.Compression == CompressionGZip {
				var buf bytes.Buffer
				zw := gzip.NewWriter(&buf)
				if _, err := zw.Write(xml); err != nil {
					t.Fatal(err)
				}
				if err := zw.Close(); err != nil {
					t.Fatal(err)
				}
				xml = buf.Bytes()
			}
			size := b.BlockSize
			if size == 0 {
				size = 1 << 20
			}
			blocks = hashedBlocks(xml, size)
		}
		payload = append(append([]byte(nil), startWritten...), blocks...)
	}
	ciphertext, err := cipher.Encrypt(keyHash.Sum(nil), iv, payload)
	if err != nil {
		t.Fatal(err)
	}
	return append(header, ciphertext...)
}
