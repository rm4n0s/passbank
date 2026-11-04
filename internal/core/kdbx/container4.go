package kdbx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"io"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// Limits on what the decrypted parts may be, so that a hostile file cannot take
// all the memory (SPEC §14).
const (
	// maxBlock is the largest block of the HMAC block stream. KeePass writes 1 MiB.
	maxBlock = 256 << 20
	// maxPayload is the largest decompressed payload: 1 GiB.
	maxPayload = 1 << 30
)

// The IDs of the fields of the inner header of KDBX 4.
const (
	innerEnd             = 0
	innerRandomStreamID  = 1
	innerRandomStreamKey = 2
	innerBinary          = 3
)

// binaryProtected is the flag of the first byte of an inner header binary that
// says the content is protected in memory.
const binaryProtected = 0x01

// container is a KDBX 4 file taken apart: its header, its XML, and what protects
// the values of the XML.
type container struct {
	// Header is the outer header.
	Header *Header
	// XML is the decrypted, decompressed XML document. Its protected values are
	// still encrypted by the inner stream.
	XML []byte
	// InnerStreamID and InnerStreamKey say how the protected values are encrypted.
	InnerStreamID  uint32
	InnerStreamKey secret.Bytes
	// Binaries are the attachments of the inner header, in order.
	Binaries []innerBinaryData
}

// innerBinaryData is an attachment of the inner header.
type innerBinaryData struct {
	Protected bool
	Data      secret.Bytes
}

// wipe overwrites the secrets of the container.
func (c *container) wipe() {
	c.InnerStreamKey.Wipe()
	for _, b := range c.Binaries {
		b.Data.Wipe()
	}
}

// readContainer4 takes a KDBX 4 file apart. composite is the composite key of
// the credentials (crypto.CompositeKey). The order of the checks is the one of
// KeePass: what the file says about itself first, and the header hash before
// the key is derived, so that damage is found without the key-derivation cost;
// then the key, which makes a wrong password WrongCredentials; then every block
// of the payload.
func readContainer4(ctx context.Context, data []byte, composite secret.Bytes) (*container, error) {
	version, err := parseSignature(data)
	if err != nil {
		return nil, err
	}
	if version.Major != 4 {
		return nil, errors.New("VersionUnsupported", "the file is not a KDBX 4 file", "version", version.String())
	}
	header, headerEnd, err := parseHeader4(data, version)
	if err != nil {
		return nil, err
	}
	// The stored hash and HMAC follow the header.
	if headerEnd+64 > len(data) {
		return nil, errors.New("FileCorrupt", "the file ends after the header")
	}
	storedHash := data[headerEnd : headerEnd+32]
	storedHMAC := data[headerEnd+32 : headerEnd+64]
	if sum := sha256.Sum256(data[:headerEnd]); !bytes.Equal(sum[:], storedHash) {
		return nil, errors.New("FileCorrupt", "the hash of the header does not match: the file is damaged")
	}
	cipher, err := crypto.LookupCipher(header.CipherID)
	if err != nil {
		return nil, err
	}
	if len(header.IV) != cipher.IVSize() {
		return nil, errors.New("FileCorrupt", "the initialization vector has the wrong length for the cipher",
			"cipher", cipher.Name(), "length", len(header.IV), "want", cipher.IVSize())
	}
	if err := header.KDF.Validate(); err != nil {
		return nil, err
	}

	transformed, err := header.KDF.Derive(ctx, composite.Reveal())
	if err != nil {
		return nil, err
	}
	defer clear(transformed)
	hmacKey := hmacBaseKey(header.MasterSeed, transformed)
	defer clear(hmacKey)
	if !hmac.Equal(storedHMAC, headerHMAC(hmacKey, data[:headerEnd])) {
		return nil, errors.New("WrongCredentials", "the password or the key file is wrong")
	}

	ciphertext, err := readBlocks(data[headerEnd+64:], hmacKey, maxPayload)
	if err != nil {
		return nil, err
	}
	keyHash := sha256.New()
	keyHash.Write(header.MasterSeed)
	keyHash.Write(transformed)
	cipherKey := keyHash.Sum(nil)
	defer clear(cipherKey)
	plain, err := cipher.Decrypt(cipherKey, header.IV, ciphertext)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	if header.Compression == CompressionGZip {
		plain, err = gunzip(plain, maxPayload)
		if err != nil {
			return nil, err
		}
		defer clear(plain)
	}

	c := &container{Header: header}
	xmlStart, err := parseInnerHeader(plain, c)
	if err != nil {
		c.wipe()
		return nil, err
	}
	c.XML = append([]byte(nil), plain[xmlStart:]...)
	return c, nil
}

// hmacBaseKey is SHA-512 of the master seed, the transformed key and a 1.
func hmacBaseKey(masterSeed, transformed []byte) []byte {
	h := sha512.New()
	h.Write(masterSeed)
	h.Write(transformed)
	h.Write([]byte{1})
	return h.Sum(nil)
}

// blockKey is the key of the HMAC of block index: SHA-512 of the index (eight
// bytes, little-endian) and the base key. The header uses the index 2^64-1.
func blockKey(base []byte, index uint64) []byte {
	var idx [8]byte
	binary.LittleEndian.PutUint64(idx[:], index)
	h := sha512.New()
	h.Write(idx[:])
	h.Write(base)
	return h.Sum(nil)
}

// headerHMAC is the HMAC-SHA-256 of the header bytes.
func headerHMAC(base, header []byte) []byte {
	key := blockKey(base, ^uint64(0))
	defer clear(key)
	m := hmac.New(sha256.New, key)
	m.Write(header)
	return m.Sum(nil)
}

// readBlocks reads the HMAC block stream: blocks of an HMAC (32 bytes), a size
// (four bytes) and that many bytes, up to a block of size zero. It returns the
// data of the blocks joined, at most max bytes of it.
func readBlocks(data, base []byte, max int) ([]byte, error) {
	var out []byte
	for index := uint64(0); ; index++ {
		if len(data) < 36 {
			return nil, errors.New("FileCorrupt", "the file ends inside the data blocks", "block", index)
		}
		stored := data[:32]
		size := binary.LittleEndian.Uint32(data[32:36])
		if size > maxBlock || len(data) < 36+int(size) {
			return nil, errors.New("FileCorrupt", "a data block is too long or cut off", "block", index, "size", size)
		}
		block := data[36 : 36+int(size)]
		key := blockKey(base, index)
		m := hmac.New(sha256.New, key)
		var prefix [12]byte
		binary.LittleEndian.PutUint64(prefix[:8], index)
		binary.LittleEndian.PutUint32(prefix[8:], size)
		m.Write(prefix[:])
		m.Write(block)
		ok := hmac.Equal(stored, m.Sum(nil))
		clear(key)
		if !ok {
			return nil, errors.New("FileCorrupt", "the HMAC of a data block does not match: the file is damaged", "block", index)
		}
		if size == 0 {
			return out, nil
		}
		if len(out)+int(size) > max {
			return nil, errors.New("FileCorrupt", "the data of the file is too large", "max", max)
		}
		out = append(out, block...)
		data = data[36+int(size):]
	}
}

// gunzip decompresses data, at most max bytes of it.
func gunzip(data []byte, max int) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.NewErr("FileCorrupt", err).SetMetadata("part", "payload")
	}
	out, err := io.ReadAll(io.LimitReader(zr, int64(max)+1))
	if err != nil {
		return nil, errors.NewErr("FileCorrupt", err).SetMetadata("part", "payload")
	}
	if len(out) > max {
		return nil, errors.New("FileCorrupt", "the decompressed data of the file is too large", "max", max)
	}
	return out, nil
}

// parseInnerHeader reads the inner header at the start of the decrypted payload
// into c and returns the offset where the XML starts. Fields it does not know
// are skipped, as KeePass does.
func parseInnerHeader(plain []byte, c *container) (int, error) {
	off := 0
	haveStream, haveKey := false, false
	for {
		if off+5 > len(plain) {
			return 0, errors.New("FileCorrupt", "the inner header of the file is cut off")
		}
		id := plain[off]
		size := int(binary.LittleEndian.Uint32(plain[off+1 : off+5]))
		off += 5
		if size < 0 || size > len(plain)-off {
			return 0, errors.New("FileCorrupt", "a field of the inner header is too long or cut off", "field", id)
		}
		field := plain[off : off+size]
		off += size
		switch id {
		case innerEnd:
			if !haveStream || !haveKey {
				return 0, errors.New("FileCorrupt", "the inner header has no inner stream ID or key")
			}
			return off, nil
		case innerRandomStreamID:
			if len(field) != 4 {
				return 0, errors.New("FileCorrupt", "the inner stream ID has the wrong length", "length", len(field))
			}
			c.InnerStreamID = binary.LittleEndian.Uint32(field)
			haveStream = true
		case innerRandomStreamKey:
			c.InnerStreamKey = secret.New(append([]byte(nil), field...))
			haveKey = true
		case innerBinary:
			if len(field) < 1 {
				return 0, errors.New("FileCorrupt", "an attachment of the inner header is empty")
			}
			c.Binaries = append(c.Binaries, innerBinaryData{
				Protected: field[0]&binaryProtected != 0,
				Data:      secret.New(append([]byte(nil), field[1:]...)),
			})
		}
	}
}
