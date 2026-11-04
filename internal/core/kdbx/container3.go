package kdbx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// readContainer3 reads the container of a KDBX 2.x or 3.x file (SPEC §4.5): the
// header, the key derivation, the decryption with the check of the stream start
// bytes (the only check of the credentials in this format), the hashed block
// stream and the decompression. The XML it returns still has its protected
// values encrypted by the inner stream.
func readContainer3(ctx context.Context, data []byte, composite secret.Bytes) (*container, error) {
	version, err := parseSignature(data)
	if err != nil {
		return nil, err
	}
	if version.Major != 2 && version.Major != 3 {
		return nil, errors.New("VersionUnsupported", "the file is not a KDBX 2 or 3 file", "version", version.String())
	}
	h, headerEnd, err := parseHeader3(data, version)
	if err != nil {
		return nil, err
	}
	cipher, err := crypto.LookupCipher(h.CipherID)
	if err != nil {
		return nil, err
	}
	if len(h.IV) != cipher.IVSize() {
		return nil, errors.New("FileCorrupt", "the initialization vector has the wrong length for the cipher",
			"cipher", cipher.Name(), "length", len(h.IV), "want", cipher.IVSize())
	}
	if err := h.KDF.Validate(); err != nil {
		return nil, err
	}
	transformed, err := h.KDF.Derive(ctx, composite.Reveal())
	if err != nil {
		return nil, err
	}
	defer clear(transformed)
	keyHash := sha256.New()
	keyHash.Write(h.MasterSeed)
	keyHash.Write(transformed)
	key := keyHash.Sum(nil)
	defer clear(key)

	// A wrong key shows as stream start bytes that differ (and as padding that is
	// not valid), so the start bytes are looked at before the padding.
	raw, err := cipher.DecryptRaw(key, h.IV, data[headerEnd:])
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	if len(raw) < startBytesLength || subtle.ConstantTimeCompare(raw[:startBytesLength], h.StreamStartBytes) != 1 {
		return nil, errors.New("WrongCredentials", "the password or key file is wrong")
	}
	plain, err := cipher.Unpad(raw)
	if err != nil {
		return nil, err
	}
	xml, err := readHashedBlocks(plain[startBytesLength:], maxPayload)
	if err != nil {
		return nil, err
	}
	if h.Compression == CompressionGZip {
		if xml, err = gunzip(xml, maxPayload); err != nil {
			return nil, err
		}
	}
	sum := sha256.Sum256(data[:headerEnd])
	return &container{
		Header: h.Header, XML: xml, InnerStreamID: h.InnerStreamID, InnerStreamKey: secret.New(append([]byte(nil), h.InnerStreamKey...)),
		HeaderHash: sum[:],
	}, nil
}

// hashBlockLength is the length of the SHA-256 in a block of the hashed block
// stream.
const hashBlockLength = 32

// readHashedBlocks reads the hashed block stream of KDBX 2.x and 3.x: blocks of
// an index (four bytes, counting from 0), a SHA-256 of the data (32 bytes), a
// size (four bytes) and the data, up to a block of size zero whose hash is all
// zeros. It returns the data of the blocks joined, at most max bytes of it.
func readHashedBlocks(data []byte, max int) ([]byte, error) {
	var out []byte
	for index := uint32(0); ; index++ {
		if len(data) < 40 {
			return nil, errors.New("FileCorrupt", "the file ends inside the data blocks", "block", index)
		}
		if got := binary.LittleEndian.Uint32(data[:4]); got != index {
			return nil, errors.New("FileCorrupt", "a data block has the wrong index", "block", index, "found", got)
		}
		stored := data[4 : 4+hashBlockLength]
		size := binary.LittleEndian.Uint32(data[36:40])
		if size > maxBlock || len(data) < 40+int(size) {
			return nil, errors.New("FileCorrupt", "a data block is too long or cut off", "block", index, "size", size)
		}
		block := data[40 : 40+int(size)]
		if size == 0 {
			if !bytes.Equal(stored, make([]byte, hashBlockLength)) {
				return nil, errors.New("FileCorrupt", "the last data block has a hash that is not zero", "block", index)
			}
			return out, nil
		}
		if sum := sha256.Sum256(block); !bytes.Equal(sum[:], stored) {
			return nil, errors.New("FileCorrupt", "the hash of a data block does not match: the file is damaged", "block", index)
		}
		if len(out)+int(size) > max {
			return nil, errors.New("FileCorrupt", "the data of the file is too large", "max", max)
		}
		out = append(out, block...)
		data = data[40+int(size):]
	}
}
