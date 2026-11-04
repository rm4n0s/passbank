package kdbx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// This file is a builder of KDBX 4 files for tests that need a file no fixture is:
// bad gzip data, a damaged inner header, a header that is valid but odd. It builds
// from the format of SPEC §4.1 with the primitives of the crypto package and has
// nothing to do with the writer of step 4.2. Its files are checked against the
// reader here, and the format itself is checked against real files by the fixtures.

// encodeVariant encodes a VariantDictionary.
func encodeVariant(items ...variantItem) []byte {
	out := []byte{0x00, 0x01}
	for _, it := range items {
		out = append(out, it.Type)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(it.Key)))
		out = append(out, it.Key...)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(it.Value)))
		out = append(out, it.Value...)
	}
	return append(out, 0)
}

func u32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func u64(v uint64) []byte { return binary.LittleEndian.AppendUint64(nil, v) }

// argonParams are small Argon2id parameters for built files.
func argonParams(salt []byte) []variantItem {
	return []variantItem{
		{"$UUID", variantBytes, crypto.KDFArgon2id[:]},
		{"S", variantBytes, salt},
		{"P", variantUInt32, u32(1)},
		{"M", variantUInt64, u64(8 * 1024)},
		{"I", variantUInt64, u64(1)},
		{"V", variantUInt32, u32(0x13)},
	}
}

// build4 describes a KDBX 4 file to build.
type build4 struct {
	// Password unlocks the file.
	Password string
	// Cipher is the outer cipher (AES-256 when zero).
	Cipher model.UUID
	// Compression of the payload.
	Compression Compression
	// KDF items of the header (small Argon2id when nil).
	KDF []variantItem
	// IVLength overrides the length of the IV (the cipher's when 0).
	IVLength int
	// ExtraHeader are further header fields written before the end field.
	ExtraHeader []HeaderField
	// Payload is the inner header and the XML, as bytes; Raw says it is already
	// what the compressor would give (so that it can be bad gzip).
	Payload []byte
	Raw     bool
	// BlockSize is the size of the blocks of the stream (1 MiB when 0).
	BlockSize int
}

// field encodes a header field.
func field(id byte, data []byte) []byte {
	return append(append([]byte{id}, u32(uint32(len(data)))...), data...)
}

// innerHeaderBytes encodes an inner header with the ChaCha20 stream and the
// given key, then the end field.
func innerHeaderBytes(streamKey []byte, binaries ...[]byte) []byte {
	out := field(innerRandomStreamID, u32(crypto.InnerChaCha20))
	out = append(out, field(innerRandomStreamKey, streamKey)...)
	for _, b := range binaries {
		out = append(out, field(innerBinary, b)...)
	}
	return append(out, field(innerEnd, nil)...)
}

// build makes the file.
func (b build4) build(t testing.TB) []byte {
	t.Helper()
	cipherID := b.Cipher
	if cipherID.IsZero() {
		cipherID = crypto.CipherAES256
	}
	cipher, err := crypto.LookupCipher(cipherID)
	if err != nil {
		t.Fatal(err)
	}
	masterSeed := bytes.Repeat([]byte{0x5a}, 32)
	ivLen := cipher.IVSize()
	if b.IVLength != 0 {
		ivLen = b.IVLength
	}
	iv := bytes.Repeat([]byte{0x33}, ivLen)
	salt := bytes.Repeat([]byte{0x77}, 16)
	kdfItems := b.KDF
	if kdfItems == nil {
		kdfItems = argonParams(salt)
	}
	kdfDict := encodeVariant(kdfItems...)

	header := binary.LittleEndian.AppendUint32(nil, signature1)
	header = binary.LittleEndian.AppendUint32(header, signature2KDBX)
	header = append(header, 0, 0, 4, 0) // version 4.0
	header = append(header, field(fieldCipherID, cipherID[:])...)
	header = append(header, field(fieldCompression, u32(uint32(b.Compression)))...)
	header = append(header, field(fieldMasterSeed, masterSeed)...)
	header = append(header, field(fieldEncryptionIV, iv)...)
	header = append(header, field(fieldKDFParameters, kdfDict)...)
	for _, f := range b.ExtraHeader {
		header = append(header, field(f.ID, f.Data)...)
	}
	header = append(header, field(fieldEnd, []byte("\r\n\r\n"))...)

	var c crypto.Credentials
	c.HasPassword, c.Password = true, secret.New([]byte(b.Password))
	composite, err := crypto.CompositeKey(c)
	if err != nil {
		t.Fatal(err)
	}
	// A file with parameters Passbank rejects is still built, with keys from
	// the default parameters: the reader must refuse it before using them.
	kdf, err := kdfFromVariant(newVariantDict(kdfItems))
	if err == nil {
		err = kdf.Validate()
	}
	if err != nil {
		kdf, err = kdfFromVariant(newVariantDict(argonParams(salt)))
		if err != nil {
			t.Fatal(err)
		}
	}
	transformed, err := kdf.Derive(context.Background(), composite.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	hmacKey := hmacBaseKey(masterSeed, transformed)

	payload := b.Payload
	if b.Compression == CompressionGZip && !b.Raw {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		payload = buf.Bytes()
	}
	keyHash := sha256.New()
	keyHash.Write(masterSeed)
	keyHash.Write(transformed)
	encIV := iv
	if len(iv) != cipher.IVSize() {
		encIV = bytes.Repeat([]byte{0x33}, cipher.IVSize())
	}
	ciphertext, err := cipher.Encrypt(keyHash.Sum(nil), encIV, payload)
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(header)
	out := append([]byte(nil), header...)
	out = append(out, sum[:]...)
	out = append(out, headerHMAC(hmacKey, header)...)
	blockSize := b.BlockSize
	if blockSize == 0 {
		blockSize = 1 << 20
	}
	return append(out, buildBlocks(hmacKey, ciphertext, blockSize)...)
}

// buildBlocks writes the HMAC block stream of data in blocks of at most size
// bytes, ending with the block of size zero.
func buildBlocks(base, data []byte, size int) []byte {
	var out []byte
	for index := uint64(0); ; index++ {
		n := min(size, len(data))
		block := data[:n]
		data = data[n:]
		m := hmac.New(sha256.New, blockKey(base, index))
		var prefix [12]byte
		binary.LittleEndian.PutUint64(prefix[:8], index)
		binary.LittleEndian.PutUint32(prefix[8:], uint32(n))
		m.Write(prefix[:])
		m.Write(block)
		out = append(out, m.Sum(nil)...)
		out = append(out, u32(uint32(n))...)
		out = append(out, block...)
		if n == 0 {
			return out
		}
	}
}

// rawHeader makes the bytes of an outer header from encoded fields: the
// signatures, version 4.0, the fields, and the end field.
func rawHeader(fields ...[]byte) []byte {
	out := binary.LittleEndian.AppendUint32(nil, signature1)
	out = binary.LittleEndian.AppendUint32(out, signature2KDBX)
	out = append(out, 0, 0, 4, 0)
	for _, f := range fields {
		out = append(out, f...)
	}
	return append(out, field(fieldEnd, []byte("\r\n\r\n"))...)
}
